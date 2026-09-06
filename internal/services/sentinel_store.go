package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"openwrt-controller/internal/database"
)

var (
	sentinelCaseRunMu sync.Mutex
	sentinelCaseRuns  = map[string]time.Time{}
)

type SentinelConversation struct {
	ID        string                `json:"id"`
	Title     string                `json:"title"`
	Status    string                `json:"status"`
	CreatedBy string                `json:"created_by"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	Messages  []SentinelMessageView `json:"messages,omitempty"`
}

type SentinelMessageView struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type SentinelCase struct {
	ID          string          `json:"id"`
	Fingerprint string          `json:"fingerprint"`
	Source      string          `json:"source"`
	SiteID      string          `json:"site_id,omitempty"`
	DeviceID    string          `json:"device_id,omitempty"`
	Severity    string          `json:"severity"`
	Status      string          `json:"status"`
	Title       string          `json:"title"`
	Summary     string          `json:"summary"`
	Evidence    json.RawMessage `json:"evidence,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
}

type SentinelNote struct {
	ID        string    `json:"id"`
	SiteID    string    `json:"site_id,omitempty"`
	DeviceID  string    `json:"device_id,omitempty"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SentinelProposal struct {
	ID             string          `json:"id"`
	CaseID         string          `json:"case_id,omitempty"`
	ConversationID string          `json:"conversation_id,omitempty"`
	DeviceID       string          `json:"device_id"`
	Config         string          `json:"config"`
	Summary        string          `json:"summary"`
	Plan           json.RawMessage `json:"plan"`
	Status         string          `json:"status"`
	BlockedReason  string          `json:"blocked_reason,omitempty"`
	CreatedBy      string          `json:"created_by"`
	ApprovedBy     string          `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time      `json:"approved_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	ExpiresAt      time.Time       `json:"expires_at"`
}

// ProcessSentinelMessage persists the operator message, runs the bounded
// read-only investigation, persists the answer, and records any proposal. It
// intentionally keeps execution separate from proposal creation.
func ProcessSentinelMessage(schema, conversationID, query, createdBy string) (SentinelInvestigationResult, *SentinelProposal, error) {
	if strings.TrimSpace(query) == "" || len(query) > 8000 {
		return SentinelInvestigationResult{}, nil, fmt.Errorf("query must contain between 1 and 8000 characters")
	}
	if _, err := GetSentinelConversation(schema, conversationID); err != nil {
		return SentinelInvestigationResult{}, nil, err
	}
	history, err := loadSentinelHistory(schema, conversationID)
	if err != nil {
		return SentinelInvestigationResult{}, nil, err
	}
	if err := appendSentinelMessage(schema, conversationID, "user", query, nil); err != nil {
		return SentinelInvestigationResult{}, nil, err
	}
	result, err := runSentinelInvestigation(schema, history, query)
	if err != nil {
		return result, nil, err
	}
	metadata, _ := json.Marshal(map[string]interface{}{
		"evidence":    result.Evidence,
		"tool_calls":  result.ToolCalls,
		"rounds":      result.Rounds,
		"llm_model":   result.LLMModel,
		"tokens_used": result.TokensUsed,
	})
	if err := appendSentinelMessage(schema, conversationID, "assistant", result.Answer, metadata); err != nil {
		return result, nil, err
	}
	if result.Proposal == nil {
		return result, nil, nil
	}
	proposal, err := CreateSentinelProposal(schema, conversationID, "", createdBy, result.Proposal)
	if err != nil {
		return result, nil, err
	}
	return result, &proposal, nil
}

func sentinelSchema(schema string) (string, error) {
	return database.SafeSchemaIdent(schema)
}

func CreateSentinelConversation(schema, title, createdBy string) (SentinelConversation, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelConversation{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Sentinel investigation"
	}
	if len(title) > 200 {
		title = title[:200]
	}
	var conversation SentinelConversation
	err = database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_conversations (title, created_by)
        VALUES ($1, $2) RETURNING id::text, title, status, created_by, created_at, updated_at`, safeSchema), title, createdBy).
		Scan(&conversation.ID, &conversation.Title, &conversation.Status, &conversation.CreatedBy, &conversation.CreatedAt, &conversation.UpdatedAt)
	return conversation, err
}

func ListSentinelConversations(schema string, limit int) ([]SentinelConversation, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := database.DB.Query(fmt.Sprintf(`SELECT id::text, title, status, created_by, created_at, updated_at
        FROM %s.sentinel_conversations ORDER BY updated_at DESC LIMIT $1`, safeSchema), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelConversation{}
	for rows.Next() {
		var item SentinelConversation
		if err := rows.Scan(&item.ID, &item.Title, &item.Status, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func GetSentinelConversation(schema, conversationID string) (SentinelConversation, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelConversation{}, err
	}
	var conversation SentinelConversation
	err = database.DB.QueryRow(fmt.Sprintf(`SELECT id::text, title, status, created_by, created_at, updated_at
        FROM %s.sentinel_conversations WHERE id = $1`, safeSchema), conversationID).
		Scan(&conversation.ID, &conversation.Title, &conversation.Status, &conversation.CreatedBy, &conversation.CreatedAt, &conversation.UpdatedAt)
	if err != nil {
		return conversation, err
	}
	conversation.Messages, err = listSentinelMessageViews(safeSchema, conversationID)
	return conversation, err
}

func listSentinelMessageViews(safeSchema, conversationID string) ([]SentinelMessageView, error) {
	rows, err := database.DB.Query(fmt.Sprintf(`SELECT id::text, role, content, metadata, created_at
        FROM %s.sentinel_messages WHERE conversation_id = $1 ORDER BY created_at ASC`, safeSchema), conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelMessageView{}
	for rows.Next() {
		var item SentinelMessageView
		if err := rows.Scan(&item.ID, &item.Role, &item.Content, &item.Metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Content = redactSentinelSecrets(item.Content)
		result = append(result, item)
	}
	return result, rows.Err()
}

func loadSentinelHistory(schema, conversationID string) ([]SentinelStoredMessage, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	rows, err := database.DB.Query(fmt.Sprintf(`SELECT role, content FROM %s.sentinel_messages
        WHERE conversation_id = $1 ORDER BY created_at ASC`, safeSchema), conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelStoredMessage{}
	for rows.Next() {
		var item SentinelStoredMessage
		if err := rows.Scan(&item.Role, &item.Content); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func appendSentinelMessage(schema, conversationID, role, content string, metadata interface{}) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	if role != "user" && role != "assistant" && role != "system" && role != "tool" {
		return fmt.Errorf("invalid sentinel message role")
	}
	if strings.TrimSpace(content) == "" || len(content) > 50000 {
		return fmt.Errorf("invalid sentinel message content")
	}
	_, err = database.DB.Exec(fmt.Sprintf(`INSERT INTO %s.sentinel_messages (conversation_id, role, content, metadata)
        VALUES ($1, $2, $3, COALESCE($4::jsonb, '{}'::jsonb))`, safeSchema), conversationID, role, redactSentinelSecrets(content), metadata)
	if err != nil {
		return err
	}
	_, err = database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_conversations SET updated_at = CURRENT_TIMESTAMP WHERE id = $1`, safeSchema), conversationID)
	return err
}

func OpenSentinelCase(schema, source, siteID, deviceID, severity, title, summary string, evidence interface{}) (string, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return "", err
	}
	fingerprint := strings.Join([]string{source, siteID, deviceID, title}, ":")
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	var existing string
	err = database.DB.QueryRow(fmt.Sprintf(`SELECT id::text FROM %s.sentinel_cases
        WHERE fingerprint = $1 AND status IN ('OPEN','INVESTIGATING') ORDER BY updated_at DESC LIMIT 1`, safeSchema), fingerprint).Scan(&existing)
	if err == nil {
		_, updateErr := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_cases SET severity = $1, summary = $2,
            evidence = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $4`, safeSchema), severity, summary, evidenceJSON, existing)
		return existing, updateErr
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	var id string
	err = database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_cases
        (fingerprint, source, site_id, device_id, severity, status, title, summary, evidence)
        VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, ''), $5, 'OPEN', $6, $7, $8)
        RETURNING id::text`, safeSchema), fingerprint, source, siteID, deviceID, severity, title, summary, evidenceJSON).Scan(&id)
	return id, err
}

func InvestigateSentinelCase(schema, caseID string) {
	if database.DB == nil {
		return
	}
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		logSentinelInvestigationError(caseID, err)
		return
	}
	_, _ = database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_cases SET status = 'INVESTIGATING', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, safeSchema), caseID)
	var title, summary string
	var evidence []byte
	if err := database.DB.QueryRow(fmt.Sprintf(`SELECT title, summary, evidence FROM %s.sentinel_cases WHERE id = $1`, safeSchema), caseID).Scan(&title, &summary, &evidence); err != nil {
		logSentinelInvestigationError(caseID, err)
		return
	}
	query := fmt.Sprintf("Investigate proactive case %s. Event title: %s\nInitial summary: %s\nEvidence: %s", caseID, title, summary, redactSentinelSecrets(string(evidence)))
	result, err := runSentinelInvestigation(schema, nil, query)
	if err != nil {
		_, _ = database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_cases SET status = 'OPEN', summary = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, safeSchema), "Investigation failed: "+err.Error(), caseID)
		logSentinelInvestigationError(caseID, err)
		return
	}
	resultEvidence, _ := json.Marshal(result.Evidence)
	_, updateErr := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_cases SET status = 'OPEN', summary = $1,
        evidence = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`, safeSchema), result.Answer, resultEvidence, caseID)
	if updateErr != nil {
		logSentinelInvestigationError(caseID, updateErr)
	}
	if result.Proposal != nil {
		if _, err := CreateSentinelProposal(schema, "", caseID, "system", result.Proposal); err != nil {
			logSentinelInvestigationError(caseID, err)
		}
	}
}

// QueueSentinelCaseInvestigation coalesces repeated telemetry/log triggers so
// one noisy device cannot consume the entire model budget.
func QueueSentinelCaseInvestigation(schema, caseID string) {
	key := schema + ":" + caseID
	sentinelCaseRunMu.Lock()
	last, seen := sentinelCaseRuns[key]
	if seen && time.Since(last) < 5*time.Minute {
		sentinelCaseRunMu.Unlock()
		return
	}
	sentinelCaseRuns[key] = time.Now()
	sentinelCaseRunMu.Unlock()
	go InvestigateSentinelCase(schema, caseID)
}

func ListSentinelCases(schema string, limit int) ([]SentinelCase, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := database.DB.Query(fmt.Sprintf(`SELECT id::text, fingerprint, source, COALESCE(site_id::text,''),
        COALESCE(device_id,''), severity, status, title, summary, evidence, created_at, updated_at, resolved_at
        FROM %s.sentinel_cases ORDER BY updated_at DESC LIMIT $1`, safeSchema), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelCase{}
	for rows.Next() {
		var item SentinelCase
		if err := rows.Scan(&item.ID, &item.Fingerprint, &item.Source, &item.SiteID, &item.DeviceID, &item.Severity, &item.Status, &item.Title, &item.Summary, &item.Evidence, &item.CreatedAt, &item.UpdatedAt, &item.ResolvedAt); err != nil {
			return nil, err
		}
		item.Summary = redactSentinelSecrets(item.Summary)
		result = append(result, item)
	}
	return result, rows.Err()
}

func GetSentinelCase(schema, caseID string) (SentinelCase, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelCase{}, err
	}
	var item SentinelCase
	err = database.DB.QueryRow(fmt.Sprintf(`SELECT id::text, fingerprint, source, COALESCE(site_id::text,''),
        COALESCE(device_id,''), severity, status, title, summary, evidence, created_at, updated_at, resolved_at
        FROM %s.sentinel_cases WHERE id = $1`, safeSchema), caseID).Scan(&item.ID, &item.Fingerprint, &item.Source, &item.SiteID, &item.DeviceID, &item.Severity, &item.Status, &item.Title, &item.Summary, &item.Evidence, &item.CreatedAt, &item.UpdatedAt, &item.ResolvedAt)
	if err != nil {
		return item, err
	}
	item.Summary = redactSentinelSecrets(item.Summary)
	return item, nil
}

func CreateSentinelNote(schema, siteID, deviceID, title, content, createdBy string) (SentinelNote, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelNote{}, err
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" || len(content) > 20000 {
		return SentinelNote{}, fmt.Errorf("note title and content are required")
	}
	var note SentinelNote
	err = database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_notes (site_id, device_id, title, content, created_by)
        VALUES (NULLIF($1, '')::uuid, NULLIF($2, ''), $3, $4, $5)
        RETURNING id::text, COALESCE(site_id::text,''), COALESCE(device_id,''), title, content, created_by, created_at, updated_at`, safeSchema), siteID, deviceID, title, content, createdBy).
		Scan(&note.ID, &note.SiteID, &note.DeviceID, &note.Title, &note.Content, &note.CreatedBy, &note.CreatedAt, &note.UpdatedAt)
	return note, err
}

func ListSentinelNotes(schema, siteID, deviceID string, limit int) ([]SentinelNote, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := fmt.Sprintf(`SELECT id::text, COALESCE(site_id::text,''), COALESCE(device_id,''), title, content, created_by, created_at, updated_at
        FROM %s.sentinel_notes WHERE 1=1`, safeSchema)
	args := []interface{}{}
	if siteID != "" {
		args = append(args, siteID)
		query += fmt.Sprintf(" AND site_id = $%d", len(args))
	}
	if deviceID != "" {
		args = append(args, deviceID)
		query += fmt.Sprintf(" AND device_id = $%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", len(args))
	rows, err := database.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelNote{}
	for rows.Next() {
		var note SentinelNote
		if err := rows.Scan(&note.ID, &note.SiteID, &note.DeviceID, &note.Title, &note.Content, &note.CreatedBy, &note.CreatedAt, &note.UpdatedAt); err != nil {
			return nil, err
		}
		note.Content = redactSentinelSecrets(note.Content)
		result = append(result, note)
	}
	return result, rows.Err()
}

func DeleteSentinelNote(schema, noteID string) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	_, err = database.DB.Exec(fmt.Sprintf("DELETE FROM %s.sentinel_notes WHERE id = $1", safeSchema), noteID)
	return err
}

func UpdateSentinelNote(schema, noteID, title, content string) (SentinelNote, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelNote{}, err
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" || len(content) > 20000 {
		return SentinelNote{}, fmt.Errorf("note title and content are required")
	}
	var note SentinelNote
	err = database.DB.QueryRow(fmt.Sprintf(`UPDATE %s.sentinel_notes SET title = $1, content = $2,
        updated_at = CURRENT_TIMESTAMP WHERE id = $3
        RETURNING id::text, COALESCE(site_id::text,''), COALESCE(device_id,''), title, content, created_by, created_at, updated_at`, safeSchema), title, content, noteID).
		Scan(&note.ID, &note.SiteID, &note.DeviceID, &note.Title, &note.Content, &note.CreatedBy, &note.CreatedAt, &note.UpdatedAt)
	return note, err
}

func CreateSentinelProposal(schema, conversationID, caseID, createdBy string, draft *SentinelProposalDraft) (SentinelProposal, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return SentinelProposal{}, err
	}
	if draft == nil {
		return SentinelProposal{}, fmt.Errorf("proposal is required")
	}
	status := "PENDING"
	blockedReason := draft.BlockedReason
	var plan []byte
	if err := validateSentinelProposal(draft); err != nil {
		blockedReason = err.Error()
		status = "BLOCKED"
		plan, _ = json.Marshal(draft)
	} else {
		operation, err := NewDeviceOperationPlan(draft.Config, draft.Commands, draft.HealthChecks, false)
		if err != nil {
			return SentinelProposal{}, err
		}
		plan, err = json.Marshal(operation)
		if err != nil {
			return SentinelProposal{}, err
		}
	}
	var proposal SentinelProposal
	err = database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_proposals
        (conversation_id, case_id, device_id, config, summary, plan, status, blocked_reason, created_by, expires_at)
        VALUES (NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, $3, $4, $5, $6, $7, NULLIF($8,''), $9, CURRENT_TIMESTAMP + INTERVAL '30 minutes')
        RETURNING id::text, COALESCE(case_id::text,''), COALESCE(conversation_id::text,''), device_id, config, summary, plan, status,
        COALESCE(blocked_reason,''), created_by, COALESCE(approved_by,''), approved_at, created_at, expires_at`, safeSchema), conversationID, caseID, draft.DeviceID, draft.Config, draft.Summary, plan, status, blockedReason, createdBy).
		Scan(&proposal.ID, &proposal.CaseID, &proposal.ConversationID, &proposal.DeviceID, &proposal.Config, &proposal.Summary, &proposal.Plan, &proposal.Status, &proposal.BlockedReason, &proposal.CreatedBy, &proposal.ApprovedBy, &proposal.ApprovedAt, &proposal.CreatedAt, &proposal.ExpiresAt)
	return proposal, err
}

func ListSentinelProposals(schema, status string, limit int) ([]SentinelProposal, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := fmt.Sprintf(`SELECT id::text, COALESCE(case_id::text,''), COALESCE(conversation_id::text,''), device_id, config, summary, plan, status,
        COALESCE(blocked_reason,''), created_by, COALESCE(approved_by,''), approved_at, created_at, expires_at FROM %s.sentinel_proposals`, safeSchema)
	args := []interface{}{}
	if status != "" {
		args = append(args, status)
		query += " WHERE status = $1"
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))
	rows, err := database.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SentinelProposal{}
	for rows.Next() {
		var item SentinelProposal
		if err := rows.Scan(&item.ID, &item.CaseID, &item.ConversationID, &item.DeviceID, &item.Config, &item.Summary, &item.Plan, &item.Status, &item.BlockedReason, &item.CreatedBy, &item.ApprovedBy, &item.ApprovedAt, &item.CreatedAt, &item.ExpiresAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func ApproveSentinelProposal(ctx context.Context, schema, proposalID, username string) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	var deviceID, status string
	var plan []byte
	err = database.DB.QueryRow(fmt.Sprintf(`UPDATE %s.sentinel_proposals SET status = 'APPROVING'
        WHERE id = $1 AND status = 'PENDING' AND expires_at > CURRENT_TIMESTAMP
        RETURNING device_id, status, plan`, safeSchema), proposalID).Scan(&deviceID, &status, &plan)
	if err == sql.ErrNoRows {
		return fmt.Errorf("proposal is no longer pending, expired, or blocked")
	}
	if err != nil {
		return err
	}
	if err := database.QueueDeviceOperation(ctx, schema, deviceID, plan); err != nil {
		_, _ = database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_proposals SET status = 'FAILED', blocked_reason = $1 WHERE id = $2`, safeSchema), err.Error(), proposalID)
		return err
	}
	_, err = database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_proposals SET status = 'APPROVED', approved_by = $1,
        approved_at = CURRENT_TIMESTAMP WHERE id = $2`, safeSchema), username, proposalID)
	return err
}

func RejectSentinelProposal(schema, proposalID, username, reason string) error {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		reason = "Rejected by operator"
	}
	result, err := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_proposals SET status = 'REJECTED', approved_by = $1,
        blocked_reason = $2 WHERE id = $3 AND status = 'PENDING'`, safeSchema), username, reason, proposalID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("proposal is no longer pending")
	}
	return nil
}

// StartSentinelCaseRecovery retries investigations that were interrupted by a
// controller restart. Cases are persisted before the model call, so an
// in-flight case can be resumed without losing the original trigger.
func StartSentinelCaseRecovery(stopCh <-chan struct{}) {
	go func() {
		recoverSentinelCases()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				recoverSentinelCases()
			}
		}
	}()
}

func recoverSentinelCases() {
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
		rows, err := database.DB.Query(fmt.Sprintf("SELECT id::text FROM %s.sentinel_cases WHERE status = 'INVESTIGATING' ORDER BY updated_at ASC LIMIT 10", schema))
		if err != nil {
			continue
		}
		var ids []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			QueueSentinelCaseInvestigation(schema, id)
		}
	}
}
