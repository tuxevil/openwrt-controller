package services

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"openwrt-controller/internal/database"
)

// SentinelReasoningClass is a workload class, not a provider or model name.
// Sentinel domain code selects one of these classes and the routing layer maps
// it to an OpenAI-compatible endpoint/model alias.
type SentinelReasoningClass string

const (
	SentinelReasoningRoutine            SentinelReasoningClass = "routine"
	SentinelReasoningDeepRCA            SentinelReasoningClass = "deep_rca"
	SentinelReasoningCuration           SentinelReasoningClass = "curation"
	SentinelReasoningFrontierEscalation SentinelReasoningClass = "frontier_escalation"
)

type SentinelModelRoute struct {
	Class   SentinelReasoningClass `json:"class"`
	BaseURL string                 `json:"-"`
	Model   string                 `json:"model"`
	APIKey  string                 `json:"-"`
	Local   bool                   `json:"local"`
}

type SentinelModelExecution struct {
	RequestedClass SentinelReasoningClass `json:"requested_class"`
	RouteClass     SentinelReasoningClass `json:"route_class"`
	Model          string                 `json:"model,omitempty"`
	Local          bool                   `json:"local"`
}

func sentinelReasoningEnvPrefix(class SentinelReasoningClass) (string, error) {
	switch class {
	case SentinelReasoningRoutine:
		return "SENTINEL_ROUTINE", nil
	case SentinelReasoningDeepRCA:
		return "SENTINEL_DEEP_RCA", nil
	case SentinelReasoningCuration:
		return "SENTINEL_CURATION", nil
	case SentinelReasoningFrontierEscalation:
		return "SENTINEL_FRONTIER", nil
	default:
		return "", fmt.Errorf("unknown Sentinel reasoning class %q", class)
	}
}

func sentinelBoolEnv(getenv func(string) string, key string, fallback bool) bool {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func sentinelURLIsLocal(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return false
	}
	if host == "localhost" || host == "::1" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	// Single-label hostnames are common for Docker/Kubernetes service names.
	return !strings.Contains(host, ".")
}

func resolveSentinelModelRouteWithEnv(class SentinelReasoningClass, settings database.PlatformSettings, getenv func(string) string) (SentinelModelRoute, error) {
	prefix, err := sentinelReasoningEnvPrefix(class)
	if err != nil {
		return SentinelModelRoute{}, err
	}

	baseURLOverride := strings.TrimRight(strings.TrimSpace(getenv(prefix+"_BASE_URL")), "/")
	baseURL := baseURLOverride
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(settings.AIEngineBaseURL), "/")
	}
	model := strings.TrimSpace(getenv(prefix + "_MODEL"))
	apiKey := strings.TrimSpace(getenv(prefix + "_API_KEY"))
	// The platform key belongs to the platform endpoint. Never forward it to a
	// per-class endpoint override: local routes can be keyless, while remote
	// overrides must provide their own credential explicitly.
	if apiKey == "" && baseURLOverride == "" {
		apiKey = openAIKey(settings.AIEngineAPIKey)
	}

	switch class {
	case SentinelReasoningRoutine:
		if model == "" {
			model = strings.TrimSpace(settings.AIEngineModel)
		}
	case SentinelReasoningDeepRCA:
		if model == "" {
			model = strings.TrimSpace(getenv("SENTINEL_ROUTINE_MODEL"))
		}
		if model == "" {
			model = strings.TrimSpace(settings.AIEngineModel)
		}
	case SentinelReasoningCuration, SentinelReasoningFrontierEscalation:
		// Optional classes must be configured explicitly so normal Sentinel
		// operation can never silently start exporting Case context to a
		// remote endpoint.
		if model == "" {
			return SentinelModelRoute{}, fmt.Errorf("%s model route is not configured", class)
		}
	}
	if baseURL == "" || model == "" {
		return SentinelModelRoute{}, fmt.Errorf("%s model route is incomplete", class)
	}

	local := sentinelURLIsLocal(baseURL)
	local = sentinelBoolEnv(getenv, prefix+"_LOCAL", local)
	if (class == SentinelReasoningRoutine || class == SentinelReasoningDeepRCA) && !local {
		return SentinelModelRoute{}, fmt.Errorf("%s requires a local model route; %s is not local", class, baseURL)
	}
	if !local && apiKey == "" {
		return SentinelModelRoute{}, fmt.Errorf("%s remote model route has no API key", class)
	}
	return SentinelModelRoute{Class: class, BaseURL: baseURL, Model: model, APIKey: apiKey, Local: local}, nil
}

func ResolveSentinelModelRoute(class SentinelReasoningClass) (SentinelModelRoute, error) {
	return resolveSentinelModelRouteWithEnv(class, database.GetPlatformSettings(), os.Getenv)
}

func sentinelReasoningRouteChain(class SentinelReasoningClass) []SentinelReasoningClass {
	if class == SentinelReasoningDeepRCA {
		return []SentinelReasoningClass{SentinelReasoningDeepRCA, SentinelReasoningRoutine}
	}
	return []SentinelReasoningClass{class}
}

func completeSentinelReasoningContext(ctx context.Context, class SentinelReasoningClass, systemPrompt, userPrompt string, jsonMode bool) (string, SentinelModelExecution, int, error) {
	settings := database.GetPlatformSettings()
	failures := make([]string, 0, 2)
	for _, candidate := range sentinelReasoningRouteChain(class) {
		route, err := resolveSentinelModelRouteWithEnv(candidate, settings, os.Getenv)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		content, model, tokens, err := completeAIEndpointContext(ctx, route.BaseURL, route.Model, route.APIKey, systemPrompt, userPrompt, jsonMode)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		return content, SentinelModelExecution{
			RequestedClass: class,
			RouteClass:     candidate,
			Model:          model,
			Local:          route.Local,
		}, tokens, nil
	}
	return "", SentinelModelExecution{RequestedClass: class}, 0, fmt.Errorf("no usable Sentinel route for %s: %s", class, strings.Join(failures, "; "))
}

// ClassifySentinelReasoning deliberately maps operator/automation work only to
// local classes. Curation/frontier are controller-owned optional workloads and
// are never selected because user/network text asked for them.
func ClassifySentinelReasoning(item SentinelCase, query string) SentinelReasoningClass {
	severity := strings.ToUpper(strings.TrimSpace(item.Severity))
	if severity == "HIGH" || severity == "CRITICAL" {
		return SentinelReasoningDeepRCA
	}
	normalized := normalizeSentinelQuery(strings.Join([]string{query, item.Source, item.Title}, " "))
	deepTerms := []string{
		"root cause", "causa raiz", "why", "por que", "correlate", "correlacion", "cascade", "cascada",
		"intermittent", "intermitente", "regression", "regresion", "rollback", "plan", "multi device", "multiple devices",
	}
	for _, term := range deepTerms {
		if strings.Contains(" "+normalized+" ", " "+normalizeSentinelQuery(term)+" ") {
			return SentinelReasoningDeepRCA
		}
	}
	return SentinelReasoningRoutine
}
