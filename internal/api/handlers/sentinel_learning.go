package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"openwrt-controller/internal/services"
)

type sentinelCounterEvidenceRequest struct {
	EvidenceRefs []services.SentinelLearningEvidenceRef `json:"evidence_refs"`
}

type sentinelSupersedeMemoryRequest struct {
	NewMemoryID string `json:"new_memory_id"`
}

type sentinelReplaySkillRequest struct {
	CaseID string `json:"case_id"`
	AsOf   string `json:"as_of,omitempty"`
}

func decodeSentinelLearningJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func ListSentinelLearnedMemoriesHandler(w http.ResponseWriter, r *http.Request) {
	items, err := services.ListSentinelLearnedMemories(sentinelSchemaFromRequest(r), r.URL.Query().Get("state"), parseSentinelLimit(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list Sentinel learned memories")
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]any{"data": items})
}

func CreateSentinelLearnedMemoryCandidateHandler(w http.ResponseWriter, r *http.Request) {
	var req services.SentinelMemoryCandidateInput
	if err := decodeSentinelLearningJSON(r, &req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid learned-memory candidate payload")
		return
	}
	if !parseSentinelID(w, req.SourceCaseID) {
		return
	}
	item, err := services.CreateSentinelLearnedMemoryCandidate(sentinelSchemaFromRequest(r), req, GetUsernameFromReq(r))
	if err != nil {
		writeSentinelError(w, http.StatusBadRequest, "CANDIDATE_REJECTED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusCreated, item)
}

func ValidateSentinelLearnedMemoryHandler(w http.ResponseWriter, r *http.Request) {
	memoryID := r.PathValue("memory_id")
	if !parseSentinelID(w, memoryID) {
		return
	}
	item, err := services.ValidateSentinelLearnedMemory(sentinelSchemaFromRequest(r), memoryID)
	if err != nil {
		writeSentinelError(w, http.StatusConflict, "VALIDATION_FAILED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, item)
}

func RejectSentinelLearnedMemoryHandler(w http.ResponseWriter, r *http.Request) {
	memoryID := r.PathValue("memory_id")
	if !parseSentinelID(w, memoryID) {
		return
	}
	if err := services.RejectSentinelLearnedMemory(sentinelSchemaFromRequest(r), memoryID); err != nil {
		writeSentinelError(w, http.StatusConflict, "REJECTION_FAILED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]string{"status": services.SentinelMemoryStateRejected, "memory_id": memoryID})
}

func AddSentinelMemoryCounterEvidenceHandler(w http.ResponseWriter, r *http.Request) {
	memoryID := r.PathValue("memory_id")
	if !parseSentinelID(w, memoryID) {
		return
	}
	var req sentinelCounterEvidenceRequest
	if err := decodeSentinelLearningJSON(r, &req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid counter-evidence payload")
		return
	}
	item, err := services.RecordSentinelMemoryCounterEvidence(sentinelSchemaFromRequest(r), memoryID, req.EvidenceRefs)
	if err != nil {
		writeSentinelError(w, http.StatusConflict, "COUNTER_EVIDENCE_REJECTED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, item)
}

func SupersedeSentinelLearnedMemoryHandler(w http.ResponseWriter, r *http.Request) {
	memoryID := r.PathValue("memory_id")
	if !parseSentinelID(w, memoryID) {
		return
	}
	var req sentinelSupersedeMemoryRequest
	if err := decodeSentinelLearningJSON(r, &req); err != nil || !parseSentinelID(w, req.NewMemoryID) {
		if err != nil {
			writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid supersession payload")
		}
		return
	}
	if err := services.SupersedeSentinelLearnedMemory(sentinelSchemaFromRequest(r), memoryID, req.NewMemoryID); err != nil {
		writeSentinelError(w, http.StatusConflict, "SUPERSESSION_FAILED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]string{"status": services.SentinelMemoryStateSuperseded, "memory_id": memoryID, "superseded_by": req.NewMemoryID})
}

func ListSentinelSkillsHandler(w http.ResponseWriter, r *http.Request) {
	items, err := services.ListSentinelSkills(sentinelSchemaFromRequest(r), r.URL.Query().Get("state"), parseSentinelLimit(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list Sentinel skills")
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]any{"data": items})
}

func CreateSentinelSkillDraftHandler(w http.ResponseWriter, r *http.Request) {
	var req services.SentinelSkillDraftInput
	if err := decodeSentinelLearningJSON(r, &req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid skill draft payload")
		return
	}
	if !parseSentinelID(w, req.SourceCaseID) {
		return
	}
	item, err := services.CreateSentinelSkillDraft(sentinelSchemaFromRequest(r), req, GetUsernameFromReq(r))
	if err != nil {
		writeSentinelError(w, http.StatusBadRequest, "DRAFT_REJECTED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusCreated, item)
}

func ValidateSentinelSkillHandler(w http.ResponseWriter, r *http.Request) {
	skillID := r.PathValue("skill_id")
	if !parseSentinelID(w, skillID) {
		return
	}
	validation, skill, err := services.ValidateSentinelSkill(sentinelSchemaFromRequest(r), skillID)
	if err != nil {
		writeSentinelJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "validation": validation, "skill": skill})
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]any{"validation": validation, "skill": skill})
}

func ReplaySentinelSkillHandler(w http.ResponseWriter, r *http.Request) {
	skillID := r.PathValue("skill_id")
	if !parseSentinelID(w, skillID) {
		return
	}
	var req sentinelReplaySkillRequest
	if err := decodeSentinelLearningJSON(r, &req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid replay payload")
		return
	}
	if !parseSentinelID(w, req.CaseID) {
		return
	}
	var asOf time.Time
	if strings.TrimSpace(req.AsOf) != "" {
		parsed, err := time.Parse(time.RFC3339, req.AsOf)
		if err != nil {
			writeSentinelError(w, http.StatusBadRequest, "INVALID_AS_OF", "as_of must be RFC3339")
			return
		}
		asOf = parsed
	}
	evaluation, err := services.ReplaySentinelSkill(sentinelSchemaFromRequest(r), skillID, req.CaseID, asOf)
	if err != nil {
		writeSentinelError(w, http.StatusConflict, "REPLAY_FAILED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, evaluation)
}

func StartSentinelSkillShadowHandler(w http.ResponseWriter, r *http.Request) {
	skillID := r.PathValue("skill_id")
	if !parseSentinelID(w, skillID) {
		return
	}
	skill, stats, err := services.StartSentinelSkillShadow(sentinelSchemaFromRequest(r), skillID)
	if err != nil {
		writeSentinelJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "stats": stats, "skill": skill})
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]any{"skill": skill, "stats": stats})
}

func PromoteSentinelSkillHandler(w http.ResponseWriter, r *http.Request) {
	skillID := r.PathValue("skill_id")
	if !parseSentinelID(w, skillID) {
		return
	}
	skill, stats, err := services.PromoteSentinelSkill(sentinelSchemaFromRequest(r), skillID)
	if err != nil {
		writeSentinelJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "stats": stats, "skill": skill})
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]any{"skill": skill, "stats": stats})
}

func DeprecateSentinelSkillHandler(w http.ResponseWriter, r *http.Request) {
	skillID := r.PathValue("skill_id")
	if !parseSentinelID(w, skillID) {
		return
	}
	if err := services.DeprecateSentinelSkill(sentinelSchemaFromRequest(r), skillID); err != nil {
		writeSentinelError(w, http.StatusConflict, "DEPRECATION_FAILED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]string{"status": services.SentinelSkillStateDeprecated, "skill_id": skillID})
}

func RevokeSentinelSkillHandler(w http.ResponseWriter, r *http.Request) {
	skillID := r.PathValue("skill_id")
	if !parseSentinelID(w, skillID) {
		return
	}
	if err := services.RevokeSentinelSkill(sentinelSchemaFromRequest(r), skillID); err != nil {
		writeSentinelError(w, http.StatusConflict, "REVOCATION_FAILED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]string{"status": services.SentinelSkillStateRevoked, "skill_id": skillID})
}

func GetSentinelSkillEvaluationsHandler(w http.ResponseWriter, r *http.Request) {
	skillID := r.PathValue("skill_id")
	if !parseSentinelID(w, skillID) {
		return
	}
	items, err := services.ListSentinelSkillEvaluations(sentinelSchemaFromRequest(r), skillID, r.URL.Query().Get("mode"), parseSentinelLimit(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list Sentinel skill evaluations")
		return
	}
	stats, _ := services.SentinelSkillStats(sentinelSchemaFromRequest(r), skillID)
	writeSentinelJSON(w, http.StatusOK, map[string]any{"data": items, "stats": stats})
}

func QueueSentinelCaseCurationHandler(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("case_id")
	if !parseSentinelID(w, caseID) {
		return
	}
	task, err := services.QueueSentinelCaseCuration(sentinelSchemaFromRequest(r), caseID)
	if err != nil {
		writeSentinelError(w, http.StatusConflict, "CURATION_NOT_QUEUED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusAccepted, task)
}
