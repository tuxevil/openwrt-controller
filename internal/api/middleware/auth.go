package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"openwrt-controller/internal/api/handlers"
	"openwrt-controller/internal/database"
)

type contextKey string

const claimsKey = contextKey("jwt_claims")
const tenantSchemaKey = contextKey("tenant_schema")

// WithAuth wraps a handler requiring a valid JWT Bearer token
func WithAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string

		// 1. Try Authorization header
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// 2. Try query parameter (for WebSockets)
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
			return handlers.JWTSecret(), nil
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
				// Set LOCAL search_path for this request's transaction queries
				tx.Exec(fmt.Sprintf("SET LOCAL search_path TO %s, public", fullSchema))
				ctx = context.WithValue(ctx, tenantSchemaKey, fullSchema)
			}
		} else {
			tx.Exec("SET LOCAL search_path TO public")
		}

		ctx = context.WithValue(ctx, database.TxKey, tx)

		next(w, r.WithContext(ctx))
		tx.Commit()
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
