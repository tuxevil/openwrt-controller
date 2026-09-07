package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

type sentinelRowScanner interface {
	Scan(dest ...any) error
}

func ensureSentinelLearningTables(schema string) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	// Learning state is deliberately separate from sentinel_cases (episodic
	// memory) and from all authoritative OMEGA/controller state. None of these
	// tables are consulted as World State.
	// #nosec G201 -- safeSchema is validated by sentinelSchema; no data values are interpolated.
	_, err = database.DB.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.sentinel_learned_memories (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		source_case_id UUID NOT NULL REFERENCES %s.sentinel_cases(id) ON DELETE CASCADE,
		statement TEXT NOT NULL,
		scope_site_id UUID REFERENCES %s.sites(id) ON DELETE CASCADE,
		scope_device_id VARCHAR(50) REFERENCES %s.devices(id) ON DELETE CASCADE,
		confidence DOUBLE PRECISION NOT NULL,
		validation_state VARCHAR(20) NOT NULL DEFAULT 'CANDIDATE',
		evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
		counter_evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
		provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_by VARCHAR(100) NOT NULL DEFAULT 'system',
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		validated_at TIMESTAMP WITH TIME ZONE,
		expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
		superseded_by UUID REFERENCES %s.sentinel_learned_memories(id),
		CHECK (confidence >= 0 AND confidence <= 1),
		CHECK (validation_state IN ('CANDIDATE','VALIDATED','REJECTED','SUPERSEDED','EXPIRED'))
	);
	CREATE INDEX IF NOT EXISTS idx_sentinel_learned_memories_active
		ON %s.sentinel_learned_memories(validation_state, expires_at, scope_site_id, scope_device_id);

	CREATE TABLE IF NOT EXISTS %s.sentinel_skills (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		source_case_id UUID NOT NULL REFERENCES %s.sentinel_cases(id) ON DELETE CASCADE,
		scope_site_id UUID REFERENCES %s.sites(id) ON DELETE CASCADE,
		scope_device_id VARCHAR(50) REFERENCES %s.devices(id) ON DELETE CASCADE,
		state VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
		spec JSONB NOT NULL,
		provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
		static_validation JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_by VARCHAR(100) NOT NULL DEFAULT 'system',
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		validated_at TIMESTAMP WITH TIME ZONE,
		shadow_started_at TIMESTAMP WITH TIME ZONE,
		trusted_at TIMESTAMP WITH TIME ZONE,
		deprecated_at TIMESTAMP WITH TIME ZONE,
		revoked_at TIMESTAMP WITH TIME ZONE,
		CHECK (state IN ('DRAFT','VALIDATED','SHADOW','TRUSTED','DEPRECATED','REVOKED'))
	);
	CREATE INDEX IF NOT EXISTS idx_sentinel_skills_state_scope
		ON %s.sentinel_skills(state, scope_site_id, scope_device_id);

	CREATE TABLE IF NOT EXISTS %s.sentinel_skill_evaluations (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		skill_id UUID NOT NULL REFERENCES %s.sentinel_skills(id) ON DELETE CASCADE,
		case_id UUID NOT NULL REFERENCES %s.sentinel_cases(id) ON DELETE CASCADE,
		mode VARCHAR(16) NOT NULL,
		as_of TIMESTAMP WITH TIME ZONE NOT NULL,
		matched BOOLEAN NOT NULL,
		steps_total INT NOT NULL,
		steps_satisfied INT NOT NULL,
		unsafe BOOLEAN NOT NULL DEFAULT false,
		score DOUBLE PRECISION NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CHECK (mode IN ('REPLAY','SHADOW')),
		CHECK (steps_total >= 0),
		CHECK (steps_satisfied >= 0 AND steps_satisfied <= steps_total),
		CHECK (score >= 0 AND score <= 1)
	);
	CREATE INDEX IF NOT EXISTS idx_sentinel_skill_evaluations_skill
		ON %s.sentinel_skill_evaluations(skill_id, mode, created_at DESC);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_sentinel_skill_replay_once
		ON %s.sentinel_skill_evaluations(skill_id, case_id, mode, as_of)
		WHERE mode = 'REPLAY';`,
		safeSchema, safeSchema, safeSchema, safeSchema, safeSchema, safeSchema,
		safeSchema, safeSchema, safeSchema, safeSchema, safeSchema, safeSchema,
		safeSchema, safeSchema, safeSchema, safeSchema))
	return err
}

func encodeLearningRefs(refs []SentinelLearningEvidenceRef) ([]byte, error) {
	if refs == nil {
		refs = []SentinelLearningEvidenceRef{}
	}
	return json.Marshal(refs)
}

func decodeLearningRefs(raw []byte) []SentinelLearningEvidenceRef {
	refs := []SentinelLearningEvidenceRef{}
	_ = json.Unmarshal(raw, &refs)
	return refs
}

func scanSentinelLearnedMemory(scanner sentinelRowScanner) (SentinelLearnedMemory, error) {
	var item SentinelLearnedMemory
	var siteID, deviceID, supersededBy sql.NullString
	var evidenceRaw, counterRaw, provenanceRaw []byte
	err := scanner.Scan(
		&item.ID, &item.SourceCaseID, &item.Statement, &siteID, &deviceID, &item.Confidence,
		&item.ValidationState, &evidenceRaw, &counterRaw, &provenanceRaw, &item.CreatedBy,
		&item.CreatedAt, &item.ValidatedAt, &item.ExpiresAt, &supersededBy,
	)
	if err != nil {
		return item, err
	}
	item.Scope = SentinelLearningScope{SiteID: siteID.String, DeviceID: deviceID.String}
	item.EvidenceRefs = decodeLearningRefs(evidenceRaw)
	item.CounterEvidenceRefs = decodeLearningRefs(counterRaw)
	item.Provenance = append(json.RawMessage(nil), provenanceRaw...)
	item.SupersededBy = supersededBy.String
	item.Statement = redactSentinelSecrets(item.Statement)
	return item, nil
}

func sentinelMemorySelect(safeSchema string) string {
	// #nosec G201 -- callers pass a schema already validated by sentinelSchema.
	return fmt.Sprintf(`SELECT id::text, source_case_id::text, statement, COALESCE(scope_site_id::text,''),
		COALESCE(scope_device_id,''), confidence, validation_state, evidence_refs, counter_evidence_refs,
		provenance, created_by, created_at, validated_at, expires_at, COALESCE(superseded_by::text,'')
		FROM %s.sentinel_learned_memories`, safeSchema)
}

func GetSentinelLearnedMemory(schema, memoryID string) (SentinelLearnedMemory, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return SentinelLearnedMemory{}, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	return scanSentinelLearnedMemory(database.DB.QueryRow(sentinelMemorySelect(safeSchema)+" WHERE id = $1", memoryID))
}

func ListSentinelLearnedMemories(schema, state string, limit int) ([]SentinelLearnedMemory, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return nil, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := sentinelMemorySelect(safeSchema)
	args := []any{}
	if state != "" {
		args = append(args, strings.ToUpper(strings.TrimSpace(state)))
		query += " WHERE validation_state = $1"
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))
	rows, err := database.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelLearnedMemory{}
	for rows.Next() {
		item, err := scanSentinelLearnedMemory(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func insertSentinelLearnedMemoryCandidate(schema string, input SentinelMemoryCandidateInput, scope SentinelLearningScope, expiresAt time.Time, createdBy string) (SentinelLearnedMemory, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	evidenceRaw, err := encodeLearningRefs(input.EvidenceRefs)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	counterRaw, err := encodeLearningRefs(input.CounterEvidenceRefs)
	if err != nil {
		return SentinelLearnedMemory{}, err
	}
	provenance := input.Provenance
	if len(provenance) == 0 || !json.Valid(provenance) {
		provenance = json.RawMessage(`{}`)
	}
	// #nosec G201 -- safeSchema is validated; all data values are parameterized.
	row := database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_learned_memories
		(source_case_id, statement, scope_site_id, scope_device_id, confidence, validation_state,
		 evidence_refs, counter_evidence_refs, provenance, created_by, expires_at)
		VALUES ($1::uuid, $2, NULLIF($3,'')::uuid, NULLIF($4,''), $5, 'CANDIDATE', $6, $7, $8, $9, $10)
		RETURNING id::text, source_case_id::text, statement, COALESCE(scope_site_id::text,''),
		COALESCE(scope_device_id,''), confidence, validation_state, evidence_refs, counter_evidence_refs,
		provenance, created_by, created_at, validated_at, expires_at, COALESCE(superseded_by::text,'')`, safeSchema),
		input.SourceCaseID, input.Statement, scope.SiteID, scope.DeviceID, input.Confidence,
		evidenceRaw, counterRaw, provenance, createdBy, expiresAt)
	return scanSentinelLearnedMemory(row)
}

func updateSentinelMemoryState(schema, memoryID, fromState, toState string) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	validatedExpr := "validated_at"
	if toState == SentinelMemoryStateValidated {
		validatedExpr = "CURRENT_TIMESTAMP"
	}
	// #nosec G201 -- safeSchema and validatedExpr are controller-owned constants; data is parameterized.
	result, err := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_learned_memories
		SET validation_state = $1, validated_at = %s
		WHERE id = $2::uuid AND validation_state = $3`, safeSchema, validatedExpr), toState, memoryID, fromState)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("memory %s is not in state %s", memoryID, fromState)
	}
	return nil
}

func listActiveSentinelMemoriesForCase(schema string, item SentinelCase, limit int) ([]SentinelLearnedMemory, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return nil, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 20 {
		limit = 8
	}
	// A memory can be global-to-tenant (NULL scope), site scoped, or device
	// scoped. More specific scopes never leak into a different Case.
	// #nosec G201 -- safeSchema is validated; all case values are parameterized.
	rows, err := database.DB.Query(sentinelMemorySelect(safeSchema)+fmt.Sprintf(` WHERE validation_state = 'VALIDATED'
		AND expires_at > CURRENT_TIMESTAMP AND superseded_by IS NULL
		AND (scope_site_id IS NULL OR scope_site_id::text = $1)
		AND (scope_device_id IS NULL OR scope_device_id = $2)
		ORDER BY confidence DESC, validated_at DESC NULLS LAST LIMIT $3`), item.SiteID, item.DeviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelLearnedMemory{}
	for rows.Next() {
		memory, err := scanSentinelLearnedMemory(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, memory)
	}
	return result, rows.Err()
}

func expireSentinelLearnedMemories(schema string) (int64, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return 0, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return 0, err
	}
	// #nosec G201 -- safeSchema is validated; no data is interpolated.
	result, err := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_learned_memories
		SET validation_state = 'EXPIRED' WHERE validation_state IN ('CANDIDATE','VALIDATED') AND expires_at <= CURRENT_TIMESTAMP`, safeSchema))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func scanSentinelSkill(scanner sentinelRowScanner) (SentinelSkill, error) {
	var item SentinelSkill
	var siteID, deviceID sql.NullString
	var specRaw, provenanceRaw, validationRaw []byte
	err := scanner.Scan(
		&item.ID, &item.SourceCaseID, &siteID, &deviceID, &item.State, &specRaw,
		&provenanceRaw, &validationRaw, &item.CreatedBy, &item.CreatedAt, &item.ValidatedAt,
		&item.ShadowStartedAt, &item.TrustedAt, &item.DeprecatedAt, &item.RevokedAt,
	)
	if err != nil {
		return item, err
	}
	item.Scope = SentinelLearningScope{SiteID: siteID.String, DeviceID: deviceID.String}
	_ = json.Unmarshal(specRaw, &item.Spec)
	item.Provenance = append(json.RawMessage(nil), provenanceRaw...)
	item.StaticValidation = append(json.RawMessage(nil), validationRaw...)
	return item, nil
}

func sentinelSkillSelect(safeSchema string) string {
	// #nosec G201 -- callers pass a schema already validated by sentinelSchema.
	return fmt.Sprintf(`SELECT id::text, source_case_id::text, COALESCE(scope_site_id::text,''), COALESCE(scope_device_id,''),
		state, spec, provenance, static_validation, created_by, created_at, validated_at, shadow_started_at,
		trusted_at, deprecated_at, revoked_at FROM %s.sentinel_skills`, safeSchema)
}

func GetSentinelSkill(schema, skillID string) (SentinelSkill, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return SentinelSkill{}, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelSkill{}, err
	}
	return scanSentinelSkill(database.DB.QueryRow(sentinelSkillSelect(safeSchema)+" WHERE id = $1", skillID))
}

func ListSentinelSkills(schema, state string, limit int) ([]SentinelSkill, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return nil, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := sentinelSkillSelect(safeSchema)
	args := []any{}
	if state != "" {
		args = append(args, strings.ToUpper(strings.TrimSpace(state)))
		query += " WHERE state = $1"
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))
	rows, err := database.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelSkill{}
	for rows.Next() {
		item, err := scanSentinelSkill(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func insertSentinelSkillDraft(schema string, input SentinelSkillDraftInput, scope SentinelLearningScope, createdBy string) (SentinelSkill, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelSkill{}, err
	}
	specRaw, err := json.Marshal(input.Spec)
	if err != nil {
		return SentinelSkill{}, err
	}
	provenance := input.Provenance
	if len(provenance) == 0 || !json.Valid(provenance) {
		provenance = json.RawMessage(`{}`)
	}
	// #nosec G201 -- safeSchema is validated; data is parameterized.
	row := database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_skills
		(source_case_id, scope_site_id, scope_device_id, state, spec, provenance, created_by)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, NULLIF($3,''), 'DRAFT', $4, $5, $6)
		RETURNING id::text, source_case_id::text, COALESCE(scope_site_id::text,''), COALESCE(scope_device_id,''),
		state, spec, provenance, static_validation, created_by, created_at, validated_at, shadow_started_at,
		trusted_at, deprecated_at, revoked_at`, safeSchema), input.SourceCaseID, scope.SiteID, scope.DeviceID, specRaw, provenance, createdBy)
	return scanSentinelSkill(row)
}

func storeSentinelSkillValidation(schema, skillID string, validation SentinelSkillValidation, promote bool) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(validation)
	if promote {
		// #nosec G201 -- safeSchema is validated; data is parameterized.
		result, err := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_skills
			SET state = 'VALIDATED', static_validation = $1, validated_at = CURRENT_TIMESTAMP
			WHERE id = $2::uuid AND state = 'DRAFT'`, safeSchema), raw, skillID)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			return fmt.Errorf("skill %s is not in DRAFT state", skillID)
		}
		return nil
	}
	// #nosec G201 -- safeSchema is validated; data is parameterized.
	_, err = database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_skills SET static_validation = $1 WHERE id = $2::uuid AND state = 'DRAFT'`, safeSchema), raw, skillID)
	return err
}

func recordSentinelSkillEvaluation(schema string, evaluation SentinelSkillEvaluation) (SentinelSkillEvaluation, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelSkillEvaluation{}, err
	}
	// #nosec G201 -- safeSchema is validated; all data is parameterized.
	err = database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_skill_evaluations
		(skill_id, case_id, mode, as_of, matched, steps_total, steps_satisfied, unsafe, score)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING
		RETURNING id::text, skill_id::text, case_id::text, mode, as_of, matched, steps_total, steps_satisfied, unsafe, score, created_at`, safeSchema),
		evaluation.SkillID, evaluation.CaseID, evaluation.Mode, evaluation.AsOf, evaluation.Matched,
		evaluation.StepsTotal, evaluation.StepsSatisfied, evaluation.Unsafe, evaluation.Score).
		Scan(&evaluation.ID, &evaluation.SkillID, &evaluation.CaseID, &evaluation.Mode, &evaluation.AsOf,
			&evaluation.Matched, &evaluation.StepsTotal, &evaluation.StepsSatisfied, &evaluation.Unsafe, &evaluation.Score, &evaluation.CreatedAt)
	if err == sql.ErrNoRows && evaluation.Mode == SentinelSkillEvalReplay {
		// Deterministic replay is idempotent. Return the already persisted row.
		// #nosec G201 -- safeSchema is validated; data is parameterized.
		err = database.DB.QueryRow(fmt.Sprintf(`SELECT id::text, skill_id::text, case_id::text, mode, as_of, matched,
			steps_total, steps_satisfied, unsafe, score, created_at FROM %s.sentinel_skill_evaluations
			WHERE skill_id = $1::uuid AND case_id = $2::uuid AND mode = 'REPLAY' AND as_of = $3`, safeSchema),
			evaluation.SkillID, evaluation.CaseID, evaluation.AsOf).
			Scan(&evaluation.ID, &evaluation.SkillID, &evaluation.CaseID, &evaluation.Mode, &evaluation.AsOf,
				&evaluation.Matched, &evaluation.StepsTotal, &evaluation.StepsSatisfied, &evaluation.Unsafe, &evaluation.Score, &evaluation.CreatedAt)
	}
	return evaluation, err
}

func sentinelSkillPromotionStats(schema, skillID string) (SentinelSkillPromotionStats, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return SentinelSkillPromotionStats{}, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelSkillPromotionStats{}, err
	}
	var stats SentinelSkillPromotionStats
	// Only matched evaluations count toward promotion. A skill that did not
	// apply to a Case neither helps nor hurts its score.
	// #nosec G201 -- safeSchema is validated; skillID is parameterized.
	err = database.DB.QueryRow(fmt.Sprintf(`SELECT
		COUNT(*) FILTER (WHERE mode = 'REPLAY' AND matched),
		COALESCE(AVG(score) FILTER (WHERE mode = 'REPLAY' AND matched), 0),
		COUNT(*) FILTER (WHERE mode = 'SHADOW' AND matched),
		COALESCE(AVG(score) FILTER (WHERE mode = 'SHADOW' AND matched), 0),
		COUNT(*) FILTER (WHERE unsafe)
		FROM %s.sentinel_skill_evaluations WHERE skill_id = $1::uuid`, safeSchema), skillID).
		Scan(&stats.ReplayCount, &stats.ReplaySuccessRate, &stats.ShadowCount, &stats.ShadowSuccessRate, &stats.UnsafeCount)
	return stats, err
}

func transitionSentinelSkillState(schema, skillID, fromState, toState, timestampColumn string) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	allowedColumns := map[string]bool{
		"shadow_started_at": true, "trusted_at": true, "deprecated_at": true, "revoked_at": true,
	}
	if !allowedColumns[timestampColumn] {
		return fmt.Errorf("invalid skill transition timestamp column")
	}
	// #nosec G201 -- safeSchema is validated and timestampColumn is allowlisted; values are parameterized.
	result, err := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_skills SET state = $1, %s = CURRENT_TIMESTAMP
		WHERE id = $2::uuid AND state = $3`, safeSchema, timestampColumn), toState, skillID, fromState)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("skill %s is not in state %s", skillID, fromState)
	}
	return nil
}

func listSentinelSkillsForStateAndCase(schema, state string, item SentinelCase, limit int) ([]SentinelSkill, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return nil, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	// #nosec G201 -- safeSchema is validated; case scope is parameterized.
	rows, err := database.DB.Query(sentinelSkillSelect(safeSchema)+fmt.Sprintf(` WHERE state = $1
		AND (scope_site_id IS NULL OR scope_site_id::text = $2)
		AND (scope_device_id IS NULL OR scope_device_id = $3)
		ORDER BY COALESCE(trusted_at, shadow_started_at, validated_at, created_at) DESC LIMIT $4`),
		state, item.SiteID, item.DeviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelSkill{}
	for rows.Next() {
		skill, err := scanSentinelSkill(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, skill)
	}
	return result, rows.Err()
}

func listSentinelSkillEvaluations(schema, skillID, mode string, limit int) ([]SentinelSkillEvaluation, error) {
	if err := ensureSentinelLearningTables(schema); err != nil {
		return nil, err
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	args := []any{skillID}
	where := "skill_id = $1::uuid"
	if mode != "" {
		args = append(args, strings.ToUpper(mode))
		where += " AND mode = $2"
	}
	args = append(args, limit)
	// #nosec G201 -- safeSchema is validated and where contains only controller-owned SQL fragments.
	rows, err := database.DB.Query(fmt.Sprintf(`SELECT id::text, skill_id::text, case_id::text, mode, as_of, matched,
		steps_total, steps_satisfied, unsafe, score, created_at FROM %s.sentinel_skill_evaluations
		WHERE %s ORDER BY created_at DESC LIMIT $%d`, safeSchema, where, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelSkillEvaluation{}
	for rows.Next() {
		var item SentinelSkillEvaluation
		if err := rows.Scan(&item.ID, &item.SkillID, &item.CaseID, &item.Mode, &item.AsOf, &item.Matched,
			&item.StepsTotal, &item.StepsSatisfied, &item.Unsafe, &item.Score, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func uniqueLearningRefs(refs []SentinelLearningEvidenceRef) []SentinelLearningEvidenceRef {
	seen := map[string]bool{}
	result := make([]SentinelLearningEvidenceRef, 0, len(refs))
	for _, ref := range refs {
		key := ref.CaseID + ":" + ref.EvidenceID
		if ref.CaseID == "" || ref.EvidenceID == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CaseID == result[j].CaseID {
			return result[i].EvidenceID < result[j].EvidenceID
		}
		return result[i].CaseID < result[j].CaseID
	})
	return result
}
