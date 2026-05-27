import os
import re

def patch_file(filepath, replacements):
    with open(filepath, 'r') as f:
        content = f.read()
    for old, new in replacements:
        content = content.replace(old, new)
    with open(filepath, 'w') as f:
        f.write(content)

patch_file('internal/api/handlers/devices.go', [
    ('query := `SELECT id, site_id, name, model, status, last_seen_at FROM devices`', 
     'query := `SELECT id, site_id, name, model, status, last_seen_at FROM devices LIMIT 1000`'),
    ('query += ` WHERE site_id IS NULL`', 
     'query = `SELECT id, site_id, name, model, status, last_seen_at FROM devices WHERE site_id IS NULL LIMIT 1000`')
])

patch_file('internal/api/handlers/users.go', [
    ('SELECT id, username, role, created_at FROM users ORDER BY created_at ASC', 
     'SELECT id, username, role, created_at FROM users ORDER BY created_at ASC LIMIT 1000')
])
