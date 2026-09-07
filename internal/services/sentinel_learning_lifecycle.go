package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

func normalizeSentinelLearningScope(source SentinelCase, requested SentinelLearningScope) (SentinelLearningScope, error) {
	scope := SentinelLearningScope{SiteID: source.SiteID, DeviceID: source.DeviceID}
	if requested.SiteID != "" && requested.SiteID != source.SiteID {
		return SentinelLearningScope{}, fmt.Errorf("learning scope cannot escape the source Case site")
	}
	if requested.DeviceID != "" && requested.DeviceID != source.DeviceID {
		return SentinelLearningScope{}, fmt.Errorf("learning scope cannot escape the source Case device")
	}
	// A model-created artifact cannot widen a scoped Case to tenant-global, nor
	// can it manufacture a narrower device scope that was not present in the
	// source Case. Controller-owned future workflows may add explicit audited
	// re-scoping without changing this invariant.
	return scope, nil
}

func sentinelCaseEvidenceIDs(item SentinelCase) map[string]bool {
	ledger := decodeSentinelCaseEvidenceLedger(item.Evidence)
	ids := map[string]bool{}
	for _, run := range ledger.Runs {
		for _, record := range run.Records {
			if record.ID != "" {
				ids[record.ID] = true
			}
		}
	}
	return ids
}

func validateSentinelLearningEvidenceRefs(schema string, scope SentinelLearningScope, refs []SentinelLearningEvidenceRef, require bool) ([]SentinelLearningEvidenceRef, error) {
	refs = uniqueLearningRefs(refs)
	if require && len(refs) == 0 {
		return nil, fmt.Errorf("at least one evidence reference is required")
	}
	caseCache := map[string]SentinelCase{}
	idCache := map[string]map[string]bool{}
	for _, ref := range refs {
		item, ok := caseCache[ref.CaseID]
		if !ok {
			loaded, err := GetSentinelCase(schema, ref.CaseID)
			if err != nil {
				return nil, fmt.Errorf("evidence Case %s: %w", ref.CaseID, err)
			}
			item = loaded
			caseCache[ref.CaseID] = item
			idCache[ref.CaseID] = sentinelCaseEvidenceIDs(item)
		}
		if scope.SiteID != "" && item.SiteID != scope.SiteID {
			return nil, fmt.Errorf("evidence %s belongs to a different site", ref.EvidenceID)
		}
		if scope.DeviceID != "" && item.DeviceID != "" && item.DeviceID != scope.DeviceID {
			return nil, fmt.Errorf("evidence %s belongs to a different device", ref.EvidenceID)
		}
		if !idCache[ref.CaseID][ref.EvidenceID] {
			return nil, fmt.Errorf("evidence id %s is not present in Case %s", ref.EvidenceID, ref.CaseID)
		}
	}
	return refs, nil
}

func CreateSentinelLearnedMemoryCandidate(schema string, input SentinelMemoryCandidateInput, createdBy string) (SentinelLearnedMemory, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return SentinelLearnedMemory{}, err
	}
	input.Statement = strings.TrimSpace(redactSentinelSecrets(input.Statement))
	if input.Statement == "" || len(input.Statement) > 4000 {
		return SentinelLearnedMemory{}, fmt.Errorf("memory statement must contain between 1 and 4000 characters")
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return SentinelLearnedMemory{}, fmt.Errorf("memory confidence must be between 0 and 1")
	}
	source, err := GetSentinelCase(schema, input.SourceCaseID)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	scope, err := normalizeSentinelLearningScope(source, input.Scope)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	input.EvidenceRefs, err = validateSentinelLearningEvidenceRefs(schema, scope, input.EvidenceRefs, true)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	input.CounterEvidenceRefs, err = validateSentinelLearningEvidenceRefs(schema, scope, input.CounterEvidenceRefs, false)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	ttl, err := sentinelLearningTTL(input.TTLHours)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	if strings.TrimSpace(createdBy) == "" {
		createdBy = "system"
	}

	// Retry-safe curation: a model task may be retried after persistence but
	// before its queue row is acknowledged. Reuse an equivalent active artifact
	// rather than multiplying candidates.
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	// #nosec G201 -- safeSchema is validated; data is parameterized.
	var existingID string
	err = database.DB.QueryRow(fmt.Sprintf(`SELECT id::text FROM %s.sentinel_learned_memories
		WHERE source_case_id = $1::uuid AND statement = $2
		AND validation_state IN ('CANDIDATE','VALIDATED') AND expires_at > CURRENT_TIMESTAMP
		ORDER BY created_at DESC LIMIT 1`, safeSchema), input.SourceCaseID, input.Statement).Scan(&existingID)
	if err == nil {
		return GetSentinelLearnedMemory(schema, existingID)
	}
	if err != sql.ErrNoRows {
		return SentinelLearnedMemory{}, err
	}
	return insertSentinelLearnedMemoryCandidate(schema, input, scope, time.Now().UTC().Add(ttl), createdBy)
}

func ValidateSentinelLearnedMemory(schema, memoryID string) (SentinelLearnedMemory, error) {
	item, err := GetSentinelLearnedMemory(schema, memoryID)
	if err != nil {
		return item, err
	}
	if item.ValidationState != SentinelMemoryStateCandidate {
		return item, fmt.Errorf("memory %s is not a CANDIDATE", memoryID)
	}
	if !item.ExpiresAt.After(time.Now().UTC()) {
		_ = updateSentinelMemoryState(schema, memoryID, SentinelMemoryStateCandidate, SentinelMemoryStateExpired)
		return item, fmt.Errorf("memory candidate has expired")
	}
	if len(item.CounterEvidenceRefs) > 0 {
		return item, fmt.Errorf("memory candidate has unresolved counter-evidence")
	}
	if _, err := validateSentinelLearningEvidenceRefs(schema, item.Scope, item.EvidenceRefs, true); err != nil {
		return item, err
	}
	if err := updateSentinelMemoryState(schema, memoryID, SentinelMemoryStateCandidate, SentinelMemoryStateValidated); err != nil {
		return item, err
	}
	return GetSentinelLearnedMemory(schema, memoryID)
}

func RejectSentinelLearnedMemory(schema, memoryID string) error {
	return updateSentinelMemoryState(schema, memoryID, SentinelMemoryStateCandidate, SentinelMemoryStateRejected)
}

func RecordSentinelMemoryCounterEvidence(schema, memoryID string, refs []SentinelLearningEvidenceRef) (SentinelLearnedMemory, error) {
	item, err := GetSentinelLearnedMemory(schema, memoryID)
	if err != nil {
		return item, err
	}
	if item.ValidationState != SentinelMemoryStateCandidate && item.ValidationState != SentinelMemoryStateValidated {
		return item, fmt.Errorf("counter-evidence cannot be added to memory in state %s", item.ValidationState)
	}
	refs, err = validateSentinelLearningEvidenceRefs(schema, item.Scope, refs, true)
	if err != nil {
		return item, err
	}
	item.CounterEvidenceRefs = uniqueLearningRefs(append(item.CounterEvidenceRefs, refs...))
	raw, _ := encodeLearningRefs(item.CounterEvidenceRefs)
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return item, err
	}
	// Contradiction immediately removes a validated memory from trusted model
	// context. It must be re-authored/revalidated or superseded explicitly.
	// #nosec G201 -- safeSchema is validated; data is parameterized.
	_, err = database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_learned_memories SET counter_evidence_refs = $1,
		validation_state = CASE WHEN validation_state = 'VALIDATED' THEN 'CANDIDATE' ELSE validation_state END,
		validated_at = CASE WHEN validation_state = 'VALIDATED' THEN NULL ELSE validated_at END
		WHERE id = $2::uuid`, safeSchema), raw, memoryID)
	if err != nil {
		return item, err
	}
	return GetSentinelLearnedMemory(schema, memoryID)
}

func SupersedeSentinelLearnedMemory(schema, oldMemoryID, newMemoryID string) error {
	oldItem, err := GetSentinelLearnedMemory(schema, oldMemoryID)
	if err != nil {
		return err
	}
	newItem, err := GetSentinelLearnedMemory(schema, newMemoryID)
	if err != nil {
		return err
	}
	if oldItem.ValidationState != SentinelMemoryStateValidated || newItem.ValidationState != SentinelMemoryStateValidated {
		return fmt.Errorf("both memories must be VALIDATED before supersession")
	}
	if oldMemoryID == newMemoryID {
		return fmt.Errorf("a memory cannot supersede itself")
	}
	if oldItem.Scope != newItem.Scope {
		return fmt.Errorf("superseding memory must have the same scope")
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	// #nosec G201 -- safeSchema is validated; IDs are parameterized.
	result, err := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_learned_memories
		SET validation_state = 'SUPERSEDED', superseded_by = $1::uuid
		WHERE id = $2::uuid AND validation_state = 'VALIDATED' AND superseded_by IS NULL`, safeSchema), newMemoryID, oldMemoryID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return fmt.Errorf("memory %s could not be superseded", oldMemoryID)
	}
	return nil
}

func CreateSentinelSkillDraft(schema string, input SentinelSkillDraftInput, createdBy string) (SentinelSkill, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return SentinelSkill{}, err
	}
	source, err := GetSentinelCase(schema, input.SourceCaseID)
	if err != nil {
		return SentinelSkill{}, err
	}
	scope, err := normalizeSentinelLearningScope(source, input.Scope)
	if err != nil {
		return SentinelSkill{}, err
	}
	if strings.TrimSpace(createdBy) == "" {
		createdBy = "system"
	}
	input.Spec.Name = strings.TrimSpace(input.Spec.Name)
	if len(input.Spec.Description) > 4000 || len(input.Spec.EvidenceTools) > 16 {
		return SentinelSkill{}, fmt.Errorf("skill draft exceeds structural limits")
	}

	// DRAFT creation intentionally does not imply validation. Still dedupe an
	// equivalent active draft produced by a retried curator task.
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelSkill{}, err
	}
	// #nosec G201 -- safeSchema is validated; data is parameterized.
	var existingID string
	err = database.DB.QueryRow(fmt.Sprintf(`SELECT id::text FROM %s.sentinel_skills
		WHERE source_case_id = $1::uuid AND spec->>'name' = $2 AND state IN ('DRAFT','VALIDATED','SHADOW','TRUSTED')
		ORDER BY created_at DESC LIMIT 1`, safeSchema), input.SourceCaseID, input.Spec.Name).Scan(&existingID)
	if err == nil {
		return GetSentinelSkill(schema, existingID)
	}
	if err != sql.ErrNoRows {
		return SentinelSkill{}, err
	}
	return insertSentinelSkillDraft(schema, input, scope, createdBy)
}

func ValidateSentinelSkill(schema, skillID string) (SentinelSkillValidation, SentinelSkill, error) {
	skill, err := GetSentinelSkill(schema, skillID)
	if err != nil {
		return SentinelSkillValidation{}, skill, err
	}
	if skill.State != SentinelSkillStateDraft {
		return SentinelSkillValidation{}, skill, fmt.Errorf("skill %s is not in DRAFT state", skillID)
	}
	validation := ValidateSentinelSkillSpec(skill.Spec)
	if err := storeSentinelSkillValidation(schema, skillID, validation, validation.Valid); err != nil {
		return validation, skill, err
	}
	if !validation.Valid {
		return validation, skill, fmt.Errorf("skill failed static validation")
	}
	skill, err = GetSentinelSkill(schema, skillID)
	return validation, skill, err
}

func sentinelHistoricalEvidenceToolsAt(ledger SentinelCaseEvidenceLedger, asOf time.Time) map[string]bool {
	available := map[string]bool{}
	for _, run := range ledger.Runs {
		for _, record := range run.Records {
			capturedAt, err := time.Parse(time.RFC3339Nano, record.CapturedAt)
			if err != nil {
				capturedAt, err = time.Parse(time.RFC3339, record.CapturedAt)
			}
			if err != nil || capturedAt.After(asOf) {
				continue
			}
			available[record.Tool] = true
		}
	}
	return available
}

func ReplaySentinelSkill(schema, skillID, caseID string, asOf time.Time) (SentinelSkillEvaluation, error) {
	skill, err := GetSentinelSkill(schema, skillID)
	if err != nil {
		return SentinelSkillEvaluation{}, err
	}
	if skill.State != SentinelSkillStateValidated && skill.State != SentinelSkillStateShadow {
		return SentinelSkillEvaluation{}, fmt.Errorf("skill must be VALIDATED or SHADOW for historical replay")
	}
	item, err := GetSentinelCase(schema, caseID)
	if err != nil {
		return SentinelSkillEvaluation{}, err
	}
	if skill.Scope.SiteID != "" && item.SiteID != skill.Scope.SiteID {
		return SentinelSkillEvaluation{}, fmt.Errorf("replay Case is outside skill site scope")
	}
	if skill.Scope.DeviceID != "" && item.DeviceID != skill.Scope.DeviceID {
		return SentinelSkillEvaluation{}, fmt.Errorf("replay Case is outside skill device scope")
	}
	if asOf.IsZero() {
		asOf = item.UpdatedAt.UTC()
		if item.ResolvedAt != nil {
			asOf = item.ResolvedAt.UTC()
		}
	}
	validation := ValidateSentinelSkillSpec(skill.Spec)
	matched := validation.Valid && sentinelSkillMatches(skill.Spec, item, "")
	available := sentinelHistoricalEvidenceToolsAt(decodeSentinelCaseEvidenceLedger(item.Evidence), asOf)
	satisfied := 0
	if matched {
		for _, toolName := range validation.ValidatedTools {
			if available[toolName] {
				satisfied++
			}
		}
	}
	total := len(validation.ValidatedTools)
	evaluation := SentinelSkillEvaluation{
		SkillID: skill.ID, CaseID: item.ID, Mode: SentinelSkillEvalReplay, AsOf: asOf,
		Matched: matched, StepsTotal: total, StepsSatisfied: satisfied, Unsafe: validation.Unsafe,
		Score: sentinelSkillScore(total, satisfied, validation.Unsafe),
	}
	return recordSentinelSkillEvaluation(schema, evaluation)
}

func StartSentinelSkillShadow(schema, skillID string) (SentinelSkill, SentinelSkillPromotionStats, error) {
	skill, err := GetSentinelSkill(schema, skillID)
	if err != nil {
		return skill, SentinelSkillPromotionStats{}, err
	}
	if skill.State != SentinelSkillStateValidated {
		return skill, SentinelSkillPromotionStats{}, fmt.Errorf("skill must be VALIDATED before SHADOW")
	}
	stats, err := sentinelSkillPromotionStats(schema, skillID)
	if err != nil {
		return skill, stats, err
	}
	if !sentinelSkillCanEnterShadow(stats) {
		return skill, stats, fmt.Errorf("skill has not met deterministic replay thresholds")
	}
	if err := transitionSentinelSkillState(schema, skillID, SentinelSkillStateValidated, SentinelSkillStateShadow, "shadow_started_at"); err != nil {
		return skill, stats, err
	}
	skill, err = GetSentinelSkill(schema, skillID)
	return skill, stats, err
}

func sentinelEvidenceToolSet(evidence []SentinelEvidence) map[string]bool {
	result := map[string]bool{}
	for _, item := range evidence {
		if item.Tool != "" {
			result[item.Tool] = true
		}
	}
	return result
}

// EvaluateSentinelShadowSkills evaluates SHADOW recipes against evidence the
// production investigation already collected. Shadow skills do not request
// additional tools and do not alter the model context or answer.
func EvaluateSentinelShadowSkills(schema string, item SentinelCase, query string, evidence []SentinelEvidence) {
	skills, err := listSentinelSkillsForStateAndCase(schema, SentinelSkillStateShadow, item, 20)
	if err != nil {
		log.Printf("[SENTINEL_LEARNING] list shadow skills failed: %v", err)
		return
	}
	available := sentinelEvidenceToolSet(evidence)
	for _, skill := range skills {
		validation := ValidateSentinelSkillSpec(skill.Spec)
		matched := validation.Valid && sentinelSkillMatches(skill.Spec, item, query)
		satisfied := 0
		if matched {
			for _, toolName := range validation.ValidatedTools {
				if available[toolName] {
					satisfied++
				}
			}
		}
		total := len(validation.ValidatedTools)
		_, err := recordSentinelSkillEvaluation(schema, SentinelSkillEvaluation{
			SkillID: skill.ID, CaseID: item.ID, Mode: SentinelSkillEvalShadow, AsOf: time.Now().UTC(),
			Matched: matched, StepsTotal: total, StepsSatisfied: satisfied, Unsafe: validation.Unsafe,
			Score: sentinelSkillScore(total, satisfied, validation.Unsafe),
		})
		if err != nil {
			log.Printf("[SENTINEL_LEARNING] shadow evaluation for skill %s failed: %v", skill.ID, err)
		}
	}
}

func PromoteSentinelSkill(schema, skillID string) (SentinelSkill, SentinelSkillPromotionStats, error) {
	skill, err := GetSentinelSkill(schema, skillID)
	if err != nil {
		return skill, SentinelSkillPromotionStats{}, err
	}
	if skill.State != SentinelSkillStateShadow {
		return skill, SentinelSkillPromotionStats{}, fmt.Errorf("skill must be in SHADOW before promotion")
	}
	validation := ValidateSentinelSkillSpec(skill.Spec)
	if !validation.Valid || validation.Unsafe {
		return skill, SentinelSkillPromotionStats{}, fmt.Errorf("skill no longer passes static safety validation")
	}
	stats, err := sentinelSkillPromotionStats(schema, skillID)
	if err != nil {
		return skill, stats, err
	}
	if !sentinelSkillCanPromote(stats) {
		return skill, stats, fmt.Errorf("skill has not met deterministic promotion thresholds")
	}
	if err := transitionSentinelSkillState(schema, skillID, SentinelSkillStateShadow, SentinelSkillStateTrusted, "trusted_at"); err != nil {
		return skill, stats, err
	}
	skill, err = GetSentinelSkill(schema, skillID)
	return skill, stats, err
}

func DeprecateSentinelSkill(schema, skillID string) error {
	return transitionSentinelSkillState(schema, skillID, SentinelSkillStateTrusted, SentinelSkillStateDeprecated, "deprecated_at")
}

func RevokeSentinelSkill(schema, skillID string) error {
	skill, err := GetSentinelSkill(schema, skillID)
	if err != nil {
		return err
	}
	if skill.State == SentinelSkillStateRevoked {
		return nil
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	// Revocation is an emergency terminal transition and is therefore allowed
	// from every non-revoked lifecycle state.
	// #nosec G201 -- safeSchema is validated; skillID is parameterized.
	result, err := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_skills SET state = 'REVOKED', revoked_at = CURRENT_TIMESTAMP
		WHERE id = $1::uuid AND state <> 'REVOKED'`, safeSchema), skillID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return fmt.Errorf("skill %s could not be revoked", skillID)
	}
	return nil
}

func SentinelSkillStats(schema, skillID string) (SentinelSkillPromotionStats, error) {
	return sentinelSkillPromotionStats(schema, skillID)
}

func ListSentinelSkillEvaluations(schema, skillID, mode string, limit int) ([]SentinelSkillEvaluation, error) {
	return listSentinelSkillEvaluations(schema, skillID, mode, limit)
}

func sentinelLearningContext(schema string, item SentinelCase, query string) ([]SentinelContextItem, []string, error) {
	memories, err := listActiveSentinelMemoriesForCase(schema, item, 8)
	if err != nil {
		return nil, nil, err
	}
	contextItems := make([]SentinelContextItem, 0, len(memories)+4)
	for _, memory := range memories {
		contextItems = append(contextItems, SentinelContextItem{
			Kind:       "learned_memory",
			TrustClass: SentinelContextLearnedKnowledge,
			Source:     "sentinel_learning:validated_memory",
			ObservedAt: memory.CreatedAt.UTC().Format(time.RFC3339Nano),
			Data: sentinelJSONData(map[string]any{
				"memory_id": memory.ID, "statement": memory.Statement, "confidence": memory.Confidence,
				"scope": memory.Scope, "validation_state": memory.ValidationState, "evidence_refs": memory.EvidenceRefs,
				"counter_evidence_refs": memory.CounterEvidenceRefs, "validated_at": memory.ValidatedAt,
				"expires_at": memory.ExpiresAt, "provenance": memory.Provenance,
			}),
		})
	}

	skills, err := listSentinelSkillsForStateAndCase(schema, SentinelSkillStateTrusted, item, 10)
	if err != nil {
		return nil, nil, err
	}
	tools := []string{}
	seen := map[string]bool{}
	for _, skill := range skills {
		validation := ValidateSentinelSkillSpec(skill.Spec)
		if !validation.Valid || validation.Unsafe || !sentinelSkillMatches(skill.Spec, item, query) {
			continue
		}
		contextItems = append(contextItems, SentinelContextItem{
			Kind:       "trusted_skill",
			TrustClass: SentinelContextLearnedKnowledge,
			Source:     "sentinel_learning:trusted_skill",
			ObservedAt: skill.CreatedAt.UTC().Format(time.RFC3339Nano),
			Data: sentinelJSONData(map[string]any{
				"skill_id": skill.ID, "state": skill.State, "scope": skill.Scope, "spec": skill.Spec,
				"provenance": skill.Provenance, "static_validation": skill.StaticValidation,
			}),
		})
		for _, toolName := range validation.ValidatedTools {
			if !seen[toolName] {
				seen[toolName] = true
				tools = append(tools, toolName)
			}
		}
	}
	sort.Strings(tools)
	return contextItems, tools, nil
}

type sentinelCuratedMemoryDraft struct {
	Statement          string   `json:"statement"`
	Confidence         float64  `json:"confidence"`
	TTLHours           int      `json:"ttl_hours,omitempty"`
	EvidenceIDs        []string `json:"evidence_ids"`
	CounterEvidenceIDs []string `json:"counter_evidence_ids,omitempty"`
}

type sentinelCuratedSkillDraft struct {
	Spec SentinelSkillSpec `json:"spec"`
}

type sentinelCurationEnvelope struct {
	Memories []sentinelCuratedMemoryDraft `json:"memories"`
	Skills   []sentinelCuratedSkillDraft  `json:"skills"`
}

const sentinelLearningCurationSystemPrompt = `You are an optional Sentinel learning curator with NO execution or trust authority.
Return ONLY one JSON object with exactly two arrays: "memories" and "skills".
A memory has: statement, confidence (0..1), ttl_hours, evidence_ids, optional counter_evidence_ids.
A skill has exactly: {"spec":{"schema_version":1,"name":"lowercase-id","description":"...","match":{"sources":[],"severities":[],"keywords":[]},"evidence_tools":[...]}}.
Skills are declarative investigation recipes. evidence_tools may name only read-only tools already shown in controller context. Never emit shell, Python, Lua, Go, SQL, HTTP requests, UCI commands, command strings, code, policies, credentials, or execution instructions.
Evidence IDs must come from CASE_CONTEXT_JSON evidence_references. If evidence is insufficient, return empty arrays.
Your output creates untrusted CANDIDATE/DRAFT artifacts only; the controller independently validates replay/shadow thresholds before any learned artifact can affect investigations.`

func importSentinelCurationCandidates(schema, taskID, caseID, raw string) error {
	var envelope sentinelCurationEnvelope
	if err := decodeStrictJSON([]byte(strings.TrimSpace(raw)), &envelope); err != nil {
		return fmt.Errorf("decode curation result: %w", err)
	}
	if len(envelope.Memories) > 5 || len(envelope.Skills) > 5 {
		return fmt.Errorf("curation result exceeds candidate limits")
	}
	item, err := GetSentinelCase(schema, caseID)
	if err != nil {
		return err
	}
	knownEvidence := sentinelCaseEvidenceIDs(item)
	provenance := sentinelJSONData(map[string]any{
		"source": "optional_model_curator", "model_task_id": taskID, "case_id": caseID,
		"authority": "candidate_only",
	})
	for _, draft := range envelope.Memories {
		refs := make([]SentinelLearningEvidenceRef, 0, len(draft.EvidenceIDs))
		for _, evidenceID := range draft.EvidenceIDs {
			if !knownEvidence[evidenceID] {
				return fmt.Errorf("curator referenced unknown evidence id %s", evidenceID)
			}
			refs = append(refs, SentinelLearningEvidenceRef{CaseID: caseID, EvidenceID: evidenceID})
		}
		counter := make([]SentinelLearningEvidenceRef, 0, len(draft.CounterEvidenceIDs))
		for _, evidenceID := range draft.CounterEvidenceIDs {
			if !knownEvidence[evidenceID] {
				return fmt.Errorf("curator referenced unknown counter-evidence id %s", evidenceID)
			}
			counter = append(counter, SentinelLearningEvidenceRef{CaseID: caseID, EvidenceID: evidenceID})
		}
		_, err := CreateSentinelLearnedMemoryCandidate(schema, SentinelMemoryCandidateInput{
			SourceCaseID: caseID, Statement: draft.Statement, Confidence: draft.Confidence,
			TTLHours: draft.TTLHours, EvidenceRefs: refs, CounterEvidenceRefs: counter, Provenance: provenance,
		}, "model-curator")
		if err != nil {
			return err
		}
	}
	for _, draft := range envelope.Skills {
		_, err := CreateSentinelSkillDraft(schema, SentinelSkillDraftInput{
			SourceCaseID: caseID, Spec: draft.Spec, Provenance: provenance,
		}, "model-curator")
		if err != nil {
			return err
		}
	}
	return nil
}

func MaybeQueueSentinelLearningCuration(schema, caseID string) {
	if !sentinelBoolEnv(os.Getenv, "SENTINEL_AUTO_CURATION", false) {
		return
	}
	if _, err := ResolveSentinelModelRoute(SentinelReasoningCuration); err != nil {
		return
	}
	prompt := "Curate reusable candidate memories and declarative read-only investigation skills from this Case. Cite only evidence IDs present in the compiled Case context. Return empty arrays rather than guessing."
	if _, err := QueueSentinelCurationTask(schema, caseID, prompt); err != nil {
		log.Printf("[SENTINEL_LEARNING] queue curation for Case %s failed: %v", caseID, err)
	}
}

func StartSentinelLearningMaintenance(stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		run := func() {
			if database.DB == nil {
				return
			}
			tenants, err := ListTenants()
			if err != nil {
				return
			}
			for _, tenant := range tenants {
				schema, err := database.SafeTenantSchema(tenant.SchemaAlias)
				if err != nil {
					continue
				}
				if _, err := expireSentinelLearnedMemories(schema); err != nil {
					log.Printf("[SENTINEL_LEARNING] expiry sweep for %s failed: %v", schema, err)
				}
			}
		}
		run()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// Keep context imported here so future lifecycle extensions can use bounded
// cancellation without adding execution authority. The current implementation
// intentionally performs replay from stored evidence only.
var _ = context.Canceled
