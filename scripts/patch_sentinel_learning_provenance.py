from pathlib import Path

store = Path("internal/services/sentinel_store.go")
text = store.read_text()
start_marker = '\t\tresult, err = database.DB.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s.sentinel_cases c\n'
end_marker = '`, schema, schema, schema), cutoff)'
new = '''\t\tresult, err = database.DB.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s.sentinel_cases c
            WHERE c.resolved_at IS NOT NULL AND c.resolved_at < $1
            AND NOT EXISTS (
                SELECT 1 FROM %s.sentinel_learned_memories m
                WHERE m.validation_state IN ('CANDIDATE','VALIDATED')
                AND m.expires_at > CURRENT_TIMESTAMP
                AND (
                    m.source_case_id = c.id
                    OR EXISTS (SELECT 1 FROM jsonb_array_elements(m.evidence_refs) e WHERE e->>'case_id' = c.id::text)
                    OR EXISTS (SELECT 1 FROM jsonb_array_elements(m.counter_evidence_refs) e WHERE e->>'case_id' = c.id::text)
                )
            )
            AND NOT EXISTS (
                SELECT 1 FROM %s.sentinel_skills s
                WHERE s.source_case_id = c.id
                AND s.state IN ('DRAFT','VALIDATED','SHADOW','TRUSTED')
            )`, schema, schema, schema), cutoff)'''

if new not in text:
    start = text.find(start_marker)
    if start < 0:
        raise SystemExit("Sentinel retention start marker not found")
    end = text.find(end_marker, start)
    if end < 0:
        raise SystemExit("Sentinel retention end marker not found")
    end += len(end_marker)
    store.write_text(text[:start] + new + text[end:])

env_file = Path(".env.example")
env = env_file.read_text()
if "SENTINEL_AUTO_CURATION" not in env:
    marker = '''# ── AI engine ─────────────────────────────────────────────────────────────────
# Passphrase used to encrypt the provider API key in platform_settings.
# Keep it stable; changing it makes existing AI keys unreadable.
# AI_ENGINE_ENCRYPTION_KEY=
'''
    addition = marker + '''
# ── Sentinel learning ─────────────────────────────────────────────────────────
# Optional asynchronous curation of Case evidence into CANDIDATE memories and
# DRAFT skills. Disabled by default. Enabling it also requires an explicit
# curation route configured through the Sentinel model router (#20).
# SENTINEL_AUTO_CURATION=false
'''
    if marker not in env:
        raise SystemExit("AI engine env marker not found")
    env_file.write_text(env.replace(marker, addition, 1))
