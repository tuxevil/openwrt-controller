import os

filepath = 'internal/api/middleware/auth.go'
with open(filepath, 'r') as f:
    content = f.read()

# Replace DB.Exec("SET search_path") with Tx
old_block = """
		if tenantSchema != "" {
			// Validate against tenants whitelist
			var count int
			err := database.DB.QueryRow(
				"SELECT COUNT(*) FROM tenants WHERE schema_alias = $1 AND is_active = true",
				tenantSchema,
			).Scan(&count)
			if err == nil && count > 0 {
				fullSchema := "tenant_" + tenantSchema
				// Set search_path for this request's queries
				database.DB.Exec(fmt.Sprintf("SET search_path TO %s, public", fullSchema))
				ctx = context.WithValue(ctx, tenantSchemaKey, fullSchema)
			}
		} else {
			// Reset search_path to public for SuperAdmin queries to avoid dirty connections!
			database.DB.Exec("SET search_path TO public")
		}

		next(w, r.WithContext(ctx))
"""

new_block = """
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
"""

content = content.replace(old_block, new_block)

with open(filepath, 'w') as f:
    f.write(content)
