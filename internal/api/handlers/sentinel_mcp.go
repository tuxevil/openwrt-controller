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

func NewSentinelMCPHandler(identitiesPath string) http.Handler {
	if identitiesPath == "" {
		return http.NotFoundHandler()
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "nerve-center-sentinel", Version: "1.0.0"}, nil)
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
		Name: "get_site_health", Description: "Read the telemetry freshness summary of your assigned site.",
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
			SiteID     string    `json:"site_id"`
			Devices    int       `json:"devices"`
			Fresh      int       `json:"fresh_within_two_minutes"`
			ObservedAt time.Time `json:"observed_at"`
		}
		snapshot.SiteID = identity.SiteID
		err = database.DB.QueryRowContext(ctx, fmt.Sprintf(`
			SELECT count(d.id), count(d.id) FILTER (WHERE d.last_seen_at > CURRENT_TIMESTAMP - INTERVAL '2 minutes'),
			       CURRENT_TIMESTAMP
			FROM %[1]s.sites s LEFT JOIN %[1]s.devices d ON d.site_id = s.id
			WHERE s.id = $1 GROUP BY s.id`, schema), identity.SiteID).
			Scan(&snapshot.Devices, &snapshot.Fresh, &snapshot.ObservedAt)
		slog.Info("Sentinel MCP site health", "client", identity.Name, "site", identity.SiteID, "found", err == nil)
		if err != nil {
			return nil, nil, errors.New("site unavailable")
		}
		return nil, snapshot, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 16384,
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
