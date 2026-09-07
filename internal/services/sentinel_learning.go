package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	SentinelMemoryStateCandidate  = "CANDIDATE"
	SentinelMemoryStateValidated  = "VALIDATED"
	SentinelMemoryStateRejected   = "REJECTED"
	SentinelMemoryStateSuperseded = "SUPERSEDED"
	SentinelMemoryStateExpired    = "EXPIRED"

	SentinelSkillStateDraft      = "DRAFT"
	SentinelSkillStateValidated  = "VALIDATED"
	SentinelSkillStateShadow     = "SHADOW"
	SentinelSkillStateTrusted    = "TRUSTED"
	SentinelSkillStateDeprecated = "DEPRECATED"
	SentinelSkillStateRevoked    = "REVOKED"

	SentinelSkillEvalReplay = "REPLAY"
	SentinelSkillEvalShadow = "SHADOW"

	SentinelSkillPromotionMinReplay       = 3
	SentinelSkillPromotionMinShadow       = 5
	SentinelSkillPromotionMinSuccessRatio = 0.80
	SentinelLearningMaxTTL                = 365 * 24 * time.Hour
	SentinelLearningDefaultTTL            = 30 * 24 * time.Hour
)

var sentinelSkillNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)

type SentinelLearningEvidenceRef struct {
	CaseID     string `json:"case_id"`
	EvidenceID string `json:"evidence_id"`
}

type SentinelLearningScope struct {
	SiteID   string `json:"site_id,omitempty"`
	DeviceID string `json:"device_id,omitempty"`
}

type SentinelLearnedMemory struct {
	ID                  string                        `json:"id"`
	SourceCaseID        string                        `json:"source_case_id"`
	Statement           string                        `json:"statement"`
	Scope               SentinelLearningScope         `json:"scope"`
	Confidence          float64                       `json:"confidence"`
	ValidationState     string                        `json:"validation_state"`
	EvidenceRefs        []SentinelLearningEvidenceRef `json:"evidence_refs"`
	CounterEvidenceRefs []SentinelLearningEvidenceRef `json:"counter_evidence_refs"`
	Provenance          json.RawMessage               `json:"provenance,omitempty"`
	CreatedBy           string                        `json:"created_by"`
	CreatedAt           time.Time                     `json:"created_at"`
	ValidatedAt         *time.Time                    `json:"validated_at,omitempty"`
	ExpiresAt           time.Time                     `json:"expires_at"`
	SupersededBy        string                        `json:"superseded_by,omitempty"`
}

type SentinelMemoryCandidateInput struct {
	SourceCaseID        string                        `json:"source_case_id"`
	Statement           string                        `json:"statement"`
	Scope               SentinelLearningScope         `json:"scope"`
	Confidence          float64                       `json:"confidence"`
	TTLHours            int                           `json:"ttl_hours,omitempty"`
	EvidenceRefs        []SentinelLearningEvidenceRef `json:"evidence_refs"`
	CounterEvidenceRefs []SentinelLearningEvidenceRef `json:"counter_evidence_refs,omitempty"`
	Provenance          json.RawMessage               `json:"provenance,omitempty"`
}

type SentinelSkillMatch struct {
	Sources    []string `json:"sources,omitempty"`
	Severities []string `json:"severities,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
}

type SentinelSkillSpec struct {
	SchemaVersion int                `json:"schema_version"`
	Name          string             `json:"name"`
	Description   string             `json:"description,omitempty"`
	Match         SentinelSkillMatch `json:"match,omitempty"`
	EvidenceTools []string           `json:"evidence_tools"`
}

type SentinelSkill struct {
	ID               string                `json:"id"`
	SourceCaseID     string                `json:"source_case_id"`
	Scope            SentinelLearningScope `json:"scope"`
	State            string                `json:"state"`
	Spec             SentinelSkillSpec     `json:"spec"`
	Provenance       json.RawMessage       `json:"provenance,omitempty"`
	StaticValidation json.RawMessage       `json:"static_validation,omitempty"`
	CreatedBy        string                `json:"created_by"`
	CreatedAt        time.Time             `json:"created_at"`
	ValidatedAt      *time.Time            `json:"validated_at,omitempty"`
	ShadowStartedAt  *time.Time            `json:"shadow_started_at,omitempty"`
	TrustedAt        *time.Time            `json:"trusted_at,omitempty"`
	DeprecatedAt     *time.Time            `json:"deprecated_at,omitempty"`
	RevokedAt        *time.Time            `json:"revoked_at,omitempty"`
}

type SentinelSkillDraftInput struct {
	SourceCaseID string                `json:"source_case_id"`
	Scope        SentinelLearningScope `json:"scope"`
	Spec         SentinelSkillSpec     `json:"spec"`
	Provenance   json.RawMessage       `json:"provenance,omitempty"`
}

type SentinelSkillValidation struct {
	Valid          bool     `json:"valid"`
	Unsafe         bool     `json:"unsafe"`
	Errors         []string `json:"errors,omitempty"`
	ValidatedTools []string `json:"validated_tools,omitempty"`
}

type SentinelSkillEvaluation struct {
	ID             string    `json:"id"`
	SkillID        string    `json:"skill_id"`
	CaseID         string    `json:"case_id"`
	Mode           string    `json:"mode"`
	AsOf           time.Time `json:"as_of"`
	Matched        bool      `json:"matched"`
	StepsTotal     int       `json:"steps_total"`
	StepsSatisfied int       `json:"steps_satisfied"`
	Unsafe         bool      `json:"unsafe"`
	Score          float64   `json:"score"`
	CreatedAt      time.Time `json:"created_at"`
}

type SentinelSkillPromotionStats struct {
	ReplayCount       int     `json:"replay_count"`
	ReplaySuccessRate float64 `json:"replay_success_rate"`
	ShadowCount       int     `json:"shadow_count"`
	ShadowSuccessRate float64 `json:"shadow_success_rate"`
	UnsafeCount       int     `json:"unsafe_count"`
}

func decodeStrictJSON(raw []byte, dst interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func normalizeSentinelStringSet(values []string, maxItems, maxLen int) ([]string, error) {
	if len(values) > maxItems {
		return nil, fmt.Errorf("too many values; maximum is %d", maxItems)
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if len(value) > maxLen {
			return nil, fmt.Errorf("value exceeds %d characters", maxLen)
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func ValidateSentinelSkillSpec(spec SentinelSkillSpec) SentinelSkillValidation {
	validation := SentinelSkillValidation{Valid: true, Errors: []string{}, ValidatedTools: []string{}}
	if spec.SchemaVersion != 1 {
		validation.Errors = append(validation.Errors, "schema_version must equal 1")
	}
	if !sentinelSkillNamePattern.MatchString(strings.TrimSpace(spec.Name)) {
		validation.Errors = append(validation.Errors, "name must match ^[a-z][a-z0-9_-]{2,63}$")
	}
	if len(spec.Description) > 1000 {
		validation.Errors = append(validation.Errors, "description exceeds 1000 characters")
	}
	if len(spec.EvidenceTools) == 0 || len(spec.EvidenceTools) > 4 {
		validation.Errors = append(validation.Errors, "evidence_tools must contain between 1 and 4 tools")
	}

	seenTools := map[string]bool{}
	for _, toolName := range spec.EvidenceTools {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" || seenTools[toolName] {
			continue
		}
		seenTools[toolName] = true
		descriptor, ok := sentinelToolRegistry.Descriptor(toolName)
		if !ok {
			validation.Errors = append(validation.Errors, "unknown tool: "+toolName)
			validation.Unsafe = true
			continue
		}
		if descriptor.SideEffectClass != SentinelToolSideEffectNone {
			validation.Errors = append(validation.Errors, "skill tool is not read-only: "+toolName)
			validation.Unsafe = true
			continue
		}
		validation.ValidatedTools = append(validation.ValidatedTools, toolName)
	}

	var err error
	if spec.Match.Sources, err = normalizeSentinelStringSet(spec.Match.Sources, 8, 64); err != nil {
		validation.Errors = append(validation.Errors, "invalid match.sources: "+err.Error())
	}
	if spec.Match.Keywords, err = normalizeSentinelStringSet(spec.Match.Keywords, 12, 64); err != nil {
		validation.Errors = append(validation.Errors, "invalid match.keywords: "+err.Error())
	}
	severities, err := normalizeSentinelStringSet(spec.Match.Severities, 5, 16)
	if err != nil {
		validation.Errors = append(validation.Errors, "invalid match.severities: "+err.Error())
	} else {
		allowed := map[string]bool{"info": true, "low": true, "medium": true, "high": true, "critical": true}
		for _, severity := range severities {
			if !allowed[severity] {
				validation.Errors = append(validation.Errors, "invalid severity: "+severity)
			}
		}
	}
	validation.Valid = len(validation.Errors) == 0 && !validation.Unsafe
	sort.Strings(validation.ValidatedTools)
	return validation
}

func decodeSentinelSkillSpec(raw json.RawMessage) (SentinelSkillSpec, error) {
	var spec SentinelSkillSpec
	if err := decodeStrictJSON(raw, &spec); err != nil {
		return SentinelSkillSpec{}, fmt.Errorf("decode skill spec: %w", err)
	}
	validation := ValidateSentinelSkillSpec(spec)
	if !validation.Valid {
		return spec, fmt.Errorf("skill spec failed validation: %s", strings.Join(validation.Errors, "; "))
	}
	return spec, nil
}

func sentinelSkillMatches(spec SentinelSkillSpec, item SentinelCase, query string) bool {
	match := spec.Match
	if len(match.Sources) > 0 {
		source := strings.ToLower(strings.TrimSpace(item.Source))
		found := false
		for _, candidate := range match.Sources {
			if strings.EqualFold(candidate, source) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(match.Severities) > 0 {
		severity := strings.ToLower(strings.TrimSpace(item.Severity))
		found := false
		for _, candidate := range match.Severities {
			if strings.EqualFold(candidate, severity) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(match.Keywords) > 0 {
		haystack := " " + normalizeSentinelQuery(strings.Join([]string{item.Title, item.Source, query}, " ")) + " "
		found := false
		for _, keyword := range match.Keywords {
			if strings.Contains(haystack, " "+normalizeSentinelQuery(keyword)+" ") {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func sentinelSkillScore(total, satisfied int, unsafe bool) float64 {
	if unsafe || total <= 0 || satisfied < 0 {
		return 0
	}
	if satisfied > total {
		satisfied = total
	}
	return float64(satisfied) / float64(total)
}

func sentinelSkillCanEnterShadow(stats SentinelSkillPromotionStats) bool {
	return stats.ReplayCount >= SentinelSkillPromotionMinReplay &&
		stats.ReplaySuccessRate >= SentinelSkillPromotionMinSuccessRatio &&
		stats.UnsafeCount == 0
}

func sentinelSkillCanPromote(stats SentinelSkillPromotionStats) bool {
	return sentinelSkillCanEnterShadow(stats) &&
		stats.ShadowCount >= SentinelSkillPromotionMinShadow &&
		stats.ShadowSuccessRate >= SentinelSkillPromotionMinSuccessRatio
}

func sentinelLearningTTL(hours int) (time.Duration, error) {
	if hours <= 0 {
		return SentinelLearningDefaultTTL, nil
	}
	ttl := time.Duration(hours) * time.Hour
	if ttl > SentinelLearningMaxTTL {
		return 0, fmt.Errorf("ttl exceeds maximum of %d hours", int(SentinelLearningMaxTTL/time.Hour))
	}
	return ttl, nil
}
