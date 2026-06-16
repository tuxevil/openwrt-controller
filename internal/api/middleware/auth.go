package middleware

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/secrets"
)

type contextKey string

const claimsKey = contextKey("jwt_claims")
const tenantSchemaKey = contextKey("tenant_schema")

// WithAuth wraps a handler requiring a valid JWT Bearer token
func WithAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string

		// 1. Try Authorization header (preferred path).
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else if os.Getenv("WS_ALLOW_QUERY_TOKEN") == "true" {
			// 2. Query parameter fallback for WebSockets. Disabled by
			//    default to avoid leaking JWTs into access logs / Referer
			//    headers. Opt in only when running behind a trusted proxy
			//    that strips the query string from logs.
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"UNAUTHORIZED: missing bearer token"}`))
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secrets.JWTSecret(), nil
		})

		if err != nil || !token.Valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"UNAUTHORIZED: invalid token"}`))
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"UNAUTHORIZED: invalid claims"}`))
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)

		// ── Tenant Schema Resolution ─────────────────────────────────
		// Priority: 1) X-Tenant-Schema header (SuperAdmin assuming identity)
		//           2) schema_alias from JWT claims (tenant-scoped user)
		tenantSchema := r.Header.Get("X-Tenant-Schema")
		if tenantSchema == "" {
			if sa, ok := claims["schema_alias"].(string); ok && sa != "" {
				tenantSchema = sa
			}
		}

		if tenantSchema == "" {
			// If no tenant schema is specified (e.g. SuperAdmin on default login),
			// check if there is an active tenant schema we can default to,
			// to avoid querying the empty public schema.
			var defaultAlias string
			err := database.DB.QueryRow(
				"SELECT schema_alias FROM tenants WHERE is_active = true ORDER BY created_at ASC LIMIT 1",
			).Scan(&defaultAlias)
			if err == nil && defaultAlias != "" {
				tenantSchema = defaultAlias
			}
		}

		tx, err := database.DB.BeginTx(r.Context(), nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		if tenantSchema != "" {
			// Validate against tenants whitelist
			var count int
			err := tx.QueryRow(
				"SELECT COUNT(*) FROM tenants WHERE schema_alias = $1 AND is_active = true",
				tenantSchema,
			).Scan(&count)
			if err == nil && count > 0 {
				fullSchema := "tenant_" + tenantSchema
				// Set LOCAL search_path for this request's transaction queries.
				// We check the error here so a SET failure surfaces as a 500
				// rather than silently targeting the public schema (which
				// would leak data across tenants).
				if _, spErr := tx.Exec(fmt.Sprintf("SET LOCAL search_path TO %s, public", fullSchema)); spErr != nil {
					log.Printf("[auth] SET LOCAL search_path failed for %q: %v", fullSchema, spErr)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				ctx = context.WithValue(ctx, tenantSchemaKey, fullSchema)
			}
		} else {
			if _, spErr := tx.Exec("SET LOCAL search_path TO public"); spErr != nil {
				log.Printf("[auth] SET LOCAL search_path public failed: %v", spErr)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		ctx = context.WithValue(ctx, database.TxKey, tx)

		next(w, r.WithContext(ctx))
		// Check Commit error so callers learn about constraint violations
		// / deadlocks even though we've already written the response body.
		// The defer Rollback() above is a no-op once Commit succeeds.
		if cerr := tx.Commit(); cerr != nil && cerr != sql.ErrTxDone {
			log.Printf("[auth] tx.Commit failed: %v", cerr)
		}
	}
}

// RequireAdmin enforces that the JWT claims contain role == "ADMIN" or "SUPERADMIN"
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsKey).(jwt.MapClaims)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"FORBIDDEN: missing claims"}`))
			return
		}

		role, ok := claims["role"].(string)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"ACCESS_DENIED"}`))
			return
		}

		upperRole := strings.ToUpper(role)
		if upperRole != "ADMIN" && upperRole != "SUPERADMIN" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"ACCESS_DENIED"}`))
			return
		}

		next(w, r)
	}
}

// RequireSuperAdmin enforces that the JWT claims contain role == "SUPERADMIN".
// Only SUPERADMIN users can access landlord-level operations.
func RequireSuperAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(claimsKey).(jwt.MapClaims)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"FORBIDDEN: missing claims"}`))
			return
		}

		role, ok := claims["role"].(string)
		if !ok || strings.ToUpper(role) != "SUPERADMIN" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"ACCESS_DENIED: SUPERADMIN clearance required"}`))
			return
		}

		next(w, r)
	}
}

// GetTenantSchema extracts the resolved tenant schema from the request context.
func GetTenantSchema(r *http.Request) string {
	if schema, ok := r.Context().Value(tenantSchemaKey).(string); ok {
		return schema
	}
	return ""
}
