package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/services"
)

type sentinelConversationRequest struct {
	Title string `json:"title"`
}

type sentinelMessageRequest struct {
	Query  string `json:"query"`
	SiteID string `json:"site_id"`
	CaseID string `json:"case_id,omitempty"`
}

type sentinelNoteRequest struct {
	SiteID   string `json:"site_id"`
	DeviceID string `json:"device_id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

type sentinelRejectRequest struct {
	Reason string `json:"reason"`
}

func sentinelSchemaFromRequest(r *http.Request) string {
	return middleware.GetTenantSchema(r)
}

func writeSentinelJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSentinelError(w http.ResponseWriter, status int, code, message string) {
	writeSentinelJSON(w, status, map[string]interface{}{"error": map[string]string{"code": code, "message": message}})
}

func writeApprovedSentinelProposalResponse(w http.ResponseWriter, proposalID string, operation services.DeviceOperationPlan) {
	writeSentinelJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "APPROVED",
		"proposal_id":  proposalID,
		"operation_id": operation.OperationID,
		"plan_hash":    operation.PlanHash,
		"generation":   operation.Generation,
	})
}

func parseSentinelID(w http.ResponseWriter, raw string) bool {
	if _, err := uuid.Parse(raw); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_ID", "invalid Sentinel resource id")
		return false
	}
	return true
}

func parseOptionalSentinelSiteID(w http.ResponseWriter, raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	if _, err := uuid.Parse(raw); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_SITE_ID", "invalid Sentinel site_id")
		return false
	}
	return true
}

func parseOptionalSentinelCaseID(w http.ResponseWriter, raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	if _, err := uuid.Parse(raw); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_CASE_ID", "invalid Sentinel case_id")
		return false
	}
	return true
}

func parseSentinelLimit(r *http.Request) int {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		return 50
	}
	return limit
}

func CreateSentinelConversationHandler(w http.ResponseWriter, r *http.Request) {
	var req sentinelConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid conversation payload")
		return
	}
	conversation, err := services.CreateSentinelConversation(sentinelSchemaFromRequest(r), req.Title, GetUsernameFromReq(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "CREATE_FAILED", "could not create Sentinel conversation")
		return
	}
	writeSentinelJSON(w, http.StatusCreated, conversation)
}

func ListSentinelConversationsHandler(w http.ResponseWriter, r *http.Request) {
	conversations, err := services.ListSentinelConversations(sentinelSchemaFromRequest(r), parseSentinelLimit(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list Sentinel conversations")
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]interface{}{"data": conversations})
}

func GetSentinelConversationHandler(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("conversation_id")
	if !parseSentinelID(w, conversationID) {
		return
	}
	conversation, err := services.GetSentinelConversation(sentinelSchemaFromRequest(r), conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		writeSentinelError(w, http.StatusNotFound, "NOT_FOUND", "Sentinel conversation not found")
		return
	}
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "GET_FAILED", "could not load Sentinel conversation")
		return
	}
	writeSentinelJSON(w, http.StatusOK, conversation)
}

func PostSentinelMessageHandler(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("conversation_id")
	if !parseSentinelID(w, conversationID) {
		return
	}
	var req sentinelMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid Sentinel message payload")
		return
	}
	if !parseOptionalSentinelSiteID(w, req.SiteID) || !parseOptionalSentinelCaseID(w, req.CaseID) {
		return
	}
	if strings.TrimSpace(req.Query) == "" || len(req.Query) > 8000 {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_QUERY", "query must contain between 1 and 8000 characters")
		return
	}
	schema := sentinelSchemaFromRequest(r)
	if r.URL.Query().Get("async") == "true" {
		run, err := services.QueueSentinelCaseMessageForSite(schema, conversationID, req.Query, req.SiteID, req.CaseID, GetUsernameFromReq(r))
		if errors.Is(err, sql.ErrNoRows) {
			writeSentinelError(w, http.StatusNotFound, "NOT_FOUND", "Sentinel conversation or Case not found")
			return
		}
		if err != nil {
			writeSentinelError(w, http.StatusBadRequest, "QUEUE_FAILED", err.Error())
			return
		}
		go services.RunSentinelCaseMessage(context.WithoutCancel(r.Context()), schema, run.ID)
		writeSentinelJSON(w, http.StatusAccepted, run)
		return
	}
	result, proposal, caseID, err := services.ProcessSentinelCaseMessageForSite(r.Context(), schema, conversationID, req.Query, req.SiteID, req.CaseID, GetUsernameFromReq(r))
	if errors.Is(err, sql.ErrNoRows) {
		writeSentinelError(w, http.StatusNotFound, "NOT_FOUND", "Sentinel conversation or Case not found")
		return
	}
	if err != nil {
		writeSentinelError(w, http.StatusBadGateway, "INVESTIGATION_FAILED", "Sentinel investigation failed")
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]interface{}{
		"conversation_id": conversationID,
		"case_id":         caseID,
		"site_id":         req.SiteID,
		"answer":          result.Answer,
		"evidence":        result.Evidence,
		"proposal":        proposal,
		"tool_calls":      result.ToolCalls,
		"rounds":          result.Rounds,
		"llm_model":       result.LLMModel,
		"tokens_used":     result.TokensUsed,
	})
}

func GetSentinelRunHandler(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if !parseSentinelID(w, runID) {
		return
	}
	run, err := services.GetSentinelRun(sentinelSchemaFromRequest(r), runID)
	if errors.Is(err, sql.ErrNoRows) {
		writeSentinelError(w, http.StatusNotFound, "NOT_FOUND", "Sentinel run not found")
		return
	}
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "GET_FAILED", "could not load Sentinel run")
		return
	}
	writeSentinelJSON(w, http.StatusOK, services.SentinelRunViewFor(run))
}

func ListSentinelCasesHandler(w http.ResponseWriter, r *http.Request) {
	cases, err := services.ListSentinelCases(sentinelSchemaFromRequest(r), parseSentinelLimit(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list Sentinel cases")
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]interface{}{"data": cases})
}

func GetSentinelCaseHandler(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("case_id")
	if !parseSentinelID(w, caseID) {
		return
	}
	item, err := services.GetSentinelCase(sentinelSchemaFromRequest(r), caseID)
	if errors.Is(err, sql.ErrNoRows) {
		writeSentinelError(w, http.StatusNotFound, "NOT_FOUND", "Sentinel case not found")
		return
	}
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "GET_FAILED", "could not load Sentinel case")
		return
	}
	writeSentinelJSON(w, http.StatusOK, item)
}

func ListSentinelNotesHandler(w http.ResponseWriter, r *http.Request) {
	notes, err := services.ListSentinelNotes(sentinelSchemaFromRequest(r), r.URL.Query().Get("site_id"), r.URL.Query().Get("device_id"), parseSentinelLimit(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list Sentinel notes")
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]interface{}{"data": notes})
}

func CreateSentinelNoteHandler(w http.ResponseWriter, r *http.Request) {
	var req sentinelNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid Sentinel note payload")
		return
	}
	note, err := services.CreateSentinelNote(sentinelSchemaFromRequest(r), req.SiteID, req.DeviceID, req.Title, req.Content, GetUsernameFromReq(r))
	if err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_NOTE", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusCreated, note)
}

func DeleteSentinelNoteHandler(w http.ResponseWriter, r *http.Request) {
	noteID := r.PathValue("note_id")
	if !parseSentinelID(w, noteID) {
		return
	}
	if err := services.DeleteSentinelNote(sentinelSchemaFromRequest(r), noteID); err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "DELETE_FAILED", "could not delete Sentinel note")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func UpdateSentinelNoteHandler(w http.ResponseWriter, r *http.Request) {
	noteID := r.PathValue("note_id")
	if !parseSentinelID(w, noteID) {
		return
	}
	var req sentinelNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_JSON", "invalid Sentinel note payload")
		return
	}
	note, err := services.UpdateSentinelNote(sentinelSchemaFromRequest(r), noteID, req.Title, req.Content)
	if errors.Is(err, sql.ErrNoRows) {
		writeSentinelError(w, http.StatusNotFound, "NOT_FOUND", "Sentinel note not found")
		return
	}
	if err != nil {
		writeSentinelError(w, http.StatusBadRequest, "INVALID_NOTE", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, note)
}

func ListSentinelProposalsHandler(w http.ResponseWriter, r *http.Request) {
	proposals, err := services.ListSentinelProposals(sentinelSchemaFromRequest(r), r.URL.Query().Get("status"), parseSentinelLimit(r))
	if err != nil {
		writeSentinelError(w, http.StatusInternalServerError, "LIST_FAILED", "could not list Sentinel proposals")
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]interface{}{"data": proposals})
}

func ApproveSentinelProposalHandler(w http.ResponseWriter, r *http.Request) {
	proposalID := r.PathValue("proposal_id")
	if !parseSentinelID(w, proposalID) {
		return
	}
	operation, err := services.ApproveSentinelProposal(r.Context(), sentinelSchemaFromRequest(r), proposalID, GetUsernameFromReq(r))
	if err != nil {
		writeSentinelError(w, http.StatusConflict, "APPROVAL_FAILED", err.Error())
		return
	}
	writeApprovedSentinelProposalResponse(w, proposalID, operation)
}

func RejectSentinelProposalHandler(w http.ResponseWriter, r *http.Request) {
	proposalID := r.PathValue("proposal_id")
	if !parseSentinelID(w, proposalID) {
		return
	}
	var req sentinelRejectRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if err := services.RejectSentinelProposal(sentinelSchemaFromRequest(r), proposalID, GetUsernameFromReq(r), req.Reason); err != nil {
		writeSentinelError(w, http.StatusConflict, "REJECTION_FAILED", err.Error())
		return
	}
	writeSentinelJSON(w, http.StatusOK, map[string]string{"status": "REJECTED", "proposal_id": proposalID})
}
