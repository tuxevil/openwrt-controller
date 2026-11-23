package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

type sentinelMCPIdentity struct {
	Name         string `json:"name"`
	TokenSHA256  string `json:"token_sha256"`
	TenantSchema string `json:"tenant_schema"`
	SiteID       string `json:"site_id"`
}

func verifySentinelMCPIdentity(path, token string) (sentinelMCPIdentity, error) {
	digest := sha256.Sum256([]byte(token))
	var match sentinelMCPIdentity
	matches := 0
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return match, auth.ErrInvalidToken
	}
	data, err := os.ReadFile(path)
	var identities []sentinelMCPIdentity
	if err != nil || json.Unmarshal(data, &identities) != nil || len(token) < 32 {
		return match, auth.ErrInvalidToken
	}
	for _, identity := range identities {
		expected, err := hex.DecodeString(identity.TokenSHA256)
		if err == nil && subtle.ConstantTimeCompare(expected, digest[:]) == 1 {
			match = identity
			matches++
		}
	}
	site, siteErr := uuid.Parse(match.SiteID)
	_, schemaErr := database.SafeSQLSchemaIdent(match.TenantSchema)
	if matches != 1 || match.Name == "" || siteErr != nil || schemaErr != nil || !strings.HasPrefix(match.TenantSchema, "tenant_") {
		return sentinelMCPIdentity{}, auth.ErrInvalidToken
	}
	match.SiteID = site.String()
	return match, nil
}

type sentinelMCPCaseInput struct {
	CaseID string `json:"case_id" jsonschema:"Sentinel case UUID"`
}

type sentinelMCPCase struct {
	ID        string    `json:"id"`
	SiteID    string    `json:"site_id"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	UpdatedAt time.Time `json:"updated_at"`
}

func sentinelMCPCaller(req *mcp.CallToolRequest) (sentinelMCPIdentity, bool) {
	if req.Extra == nil || req.Extra.TokenInfo == nil {
		return sentinelMCPIdentity{}, false
	}
	identity, ok := req.Extra.TokenInfo.Extra["identity"].(sentinelMCPIdentity)
	return identity, ok
}

type sentinelMCPListCasesInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum cases to return (1-50, default 20)"`
}

type sentinelMCPAskInput struct {
	Query string `json:"query" jsonschema:"Natural-language question for Sentinel about your site"`
}

type sentinelMCPAskOutput struct {
	Answer         string   `json:"answer"`
	CaseID         string   `json:"case_id"`
	ConversationID string   `json:"conversation_id"`
	LLMModel       string   `json:"llm_model"`
	Rounds         int      `json:"rounds"`
	ToolCalls      int      `json:"tool_calls"`
	TokensUsed     int      `json:"tokens_used"`
	ToolsUsed      []string `json:"tools_used"`
}

func sentinelMCPToolSchema(descriptor services.SentinelToolDescriptor) map[string]any {
	properties := make(map[string]any, len(descriptor.InputSchema.Properties))
	for name, prop := range descriptor.InputSchema.Properties {
		// site_id is always injected from the verified Bearer token identity.
		if name == "site_id" {
			continue
		}
		schema := map[string]any{
			"type":        prop.Type,
			"description": prop.Description,
		}
		if prop.MaxLength > 0 {
			schema["maxLength"] = prop.MaxLength
		}
		if prop.Minimum != nil {
			schema["minimum"] = *prop.Minimum
		}
		if prop.Maximum != nil {
			schema["maximum"] = *prop.Maximum
		}
		properties[name] = schema
	}
	out := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	var required []string
	for _, reqField := range descriptor.InputSchema.Required {
		if reqField != "site_id" {
			required = append(required, reqField)
		}
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func NewSentinelMCPHandler(identitiesPath string) http.Handler {
	if identitiesPath == "" {
		return http.NotFoundHandler()
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "nerve-center-sentinel", Version: "2.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_case", Description: "Read the summary of a Sentinel case in your assigned site.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input sentinelMCPCaseInput) (*mcp.CallToolResult, any, error) {
		identity, ok := sentinelMCPCaller(req)
		if !ok {
			return nil, nil, errors.New("authentication required")
		}
		caseID, err := uuid.Parse(input.CaseID)
		if err != nil {
			return nil, nil, errors.New("invalid case ID")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		item, err := services.GetSentinelCaseForSite(ctx, identity.TenantSchema, identity.SiteID, caseID.String())
		slog.Info("Sentinel MCP case lookup", "client", identity.Name, "site", identity.SiteID, "case", caseID.String(), "found", err == nil)
		if err != nil {
			return nil, nil, errors.New("case unavailable")
		}
		return nil, sentinelMCPCase{ID: item.ID, SiteID: item.SiteID, Severity: item.Severity, Status: item.Status, Title: item.Title, Summary: item.Summary, UpdatedAt: item.UpdatedAt}, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_cases", Description: "List recent Sentinel cases in your assigned site.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input sentinelMCPListCasesInput) (*mcp.CallToolResult, any, error) {
		identity, ok := sentinelMCPCaller(req)
		if !ok {
			return nil, nil, errors.New("authentication required")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		items, err := services.ListSentinelCasesForSite(ctx, identity.TenantSchema, identity.SiteID, input.Limit)
		slog.Info("Sentinel MCP list cases", "client", identity.Name, "site", identity.SiteID, "count", len(items), "ok", err == nil)
		if err != nil {
			return nil, nil, errors.New("cases unavailable")
		}
		out := make([]sentinelMCPCase, 0, len(items))
		for _, item := range items {
			out = append(out, sentinelMCPCase{ID: item.ID, SiteID: item.SiteID, Severity: item.Severity, Status: item.Status, Title: item.Title, Summary: item.Summary, UpdatedAt: item.UpdatedAt})
		}
		return nil, map[string]any{"site_id": identity.SiteID, "count": len(out), "cases": out}, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_site_health", Description: "Read telemetry freshness and the last 24 hours of incident counts for your assigned site.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		identity, ok := sentinelMCPCaller(req)
		if !ok {
			return nil, nil, errors.New("authentication required")
		}
		schema, err := database.SafeSQLSchemaIdent(identity.TenantSchema)
		if err != nil {
			return nil, nil, errors.New("site unavailable")
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var snapshot struct {
			SiteID              string    `json:"site_id"`
			Devices             int       `json:"devices"`
			Fresh               int       `json:"fresh_within_two_minutes"`
			ObservedAt          time.Time `json:"observed_at"`
			OpenIncidents       int       `json:"open_incidents"`
			OpenedLast24Hours   int       `json:"opened_last_24_hours"`
			ResolvedLast24Hours int       `json:"resolved_last_24_hours"`
		}
		snapshot.SiteID = identity.SiteID
		err = database.DB.QueryRowContext(ctx, fmt.Sprintf(`
			SELECT count(d.id), count(d.id) FILTER (WHERE d.last_seen_at > CURRENT_TIMESTAMP - INTERVAL '2 minutes'),
			       CURRENT_TIMESTAMP,
			       (SELECT count(*) FROM %[1]s.incidents WHERE site_id = $1 AND status = 'OPEN'),
			       (SELECT count(*) FROM %[1]s.incidents WHERE site_id = $1 AND created_at > CURRENT_TIMESTAMP - INTERVAL '24 hours'),
			       (SELECT count(*) FROM %[1]s.incidents WHERE site_id = $1 AND resolved_at > CURRENT_TIMESTAMP - INTERVAL '24 hours')
			FROM %[1]s.sites s LEFT JOIN %[1]s.devices d ON d.site_id = s.id
			WHERE s.id = $1 GROUP BY s.id`, schema), identity.SiteID).
			Scan(&snapshot.Devices, &snapshot.Fresh, &snapshot.ObservedAt,
				&snapshot.OpenIncidents, &snapshot.OpenedLast24Hours, &snapshot.ResolvedLast24Hours)
		slog.Info("Sentinel MCP site health", "client", identity.Name, "site", identity.SiteID, "found", err == nil)
		if err != nil {
			return nil, nil, errors.New("site unavailable")
		}
		return nil, snapshot, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "ask_sentinel", Description: "Ask Nerve Center's local Sentinel AI a natural-language question about your assigned site (for direct structured data with zero local LLM latency, prefer the specific read-only tools).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input sentinelMCPAskInput) (*mcp.CallToolResult, any, error) {
		identity, ok := sentinelMCPCaller(req)
		if !ok {
			return nil, nil, errors.New("authentication required")
		}
		q := strings.TrimSpace(input.Query)
		if q == "" || len(q) > 4000 {
			return nil, nil, errors.New("query must contain between 1 and 4000 characters")
		}
		createdBy := "mcp:" + identity.Name
		conv, err := services.CreateSentinelConversation(identity.TenantSchema, "MCP "+identity.Name, createdBy)
		if err != nil {
			return nil, nil, errors.New("could not start Sentinel conversation")
		}
		res, _, caseID, err := services.ProcessSentinelCaseMessageForSite(ctx, identity.TenantSchema, conv.ID, q, identity.SiteID, "", createdBy)
		slog.Info("Sentinel MCP ask", "client", identity.Name, "site", identity.SiteID, "case", caseID, "ok", err == nil, "rounds", res.Rounds, "tool_calls", res.ToolCalls)
		if err != nil {
			return nil, nil, errors.New("Sentinel investigation failed")
		}
		toolsUsed := make([]string, 0, len(res.Evidence))
		for _, ev := range res.Evidence {
			toolsUsed = append(toolsUsed, ev.Tool)
		}
		return nil, sentinelMCPAskOutput{
			Answer:         res.Answer,
			CaseID:         caseID,
			ConversationID: conv.ID,
			LLMModel:       res.LLMModel,
			Rounds:         res.Rounds,
			ToolCalls:      res.ToolCalls,
			TokensUsed:     res.TokensUsed,
			ToolsUsed:      toolsUsed,
		}, nil
	})

	// Bridge every registered read-only Sentinel tool directly to MCP so external
	// AI agents (OpenClaw, Hermes, etc.) can query Nerve Center, routers,
	// AdGuardHome, Starlink/WireGuard, firewall, and probes with zero LLM hops.
	for _, descriptor := range services.SentinelToolDescriptors() {
		if descriptor.SideEffectClass != services.SentinelToolSideEffectNone {
			continue
		}
		toolName := descriptor.Name
		server.AddTool(&mcp.Tool{
			Name:        toolName,
			Description: descriptor.Description,
			InputSchema: sentinelMCPToolSchema(descriptor),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			identity, ok := sentinelMCPCaller(req)
			if !ok {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "authentication required"}},
				}, nil
			}
			var rawArgs json.RawMessage
			if req.Params != nil {
				rawArgs = req.Params.Arguments
			}
			result, err := services.ExecuteSentinelMCPTool(ctx, toolName, identity.TenantSchema, identity.SiteID, rawArgs)
			slog.Info("Sentinel MCP tool call", "client", identity.Name, "site", identity.SiteID, "tool", toolName, "ok", err == nil)
			if err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				}, nil
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "failed to encode tool result"}},
				}, nil
			}
			out := &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
			}
			if obj, ok := result.(map[string]any); ok {
				out.StructuredContent = obj
			}
			return out, nil
		})
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 65536,
	})
	protected := auth.RequireBearerToken(func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		identity, err := verifySentinelMCPIdentity(identitiesPath, token)
		if err != nil {
			return nil, err
		}
		return &auth.TokenInfo{UserID: identity.Name, Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"identity": identity}}, nil
	}, nil)(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.Header.Get("Origin") != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		protected.ServeHTTP(w, r)
	})
}
