import re

def fix(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    # For agent_mgmt.go
    content = content.replace('func resolveSiteByKey(siteKey string) (string, error) {\n\tvar siteID string\n\terr := database.Tx(r.Context()).QueryRow(', 
                              'func resolveSiteByKey(siteKey string) (string, error) {\n\tvar siteID string\n\terr := database.DB.QueryRow(')
    
    # For edge_api.go
    # edge_api.go:38, 52, 66
    content = content.replace('database.Tx(r.Context())', 'database.DB')

    with open(filepath, 'w') as f:
        f.write(content)

fix('internal/api/handlers/agent_mgmt.go')
fix('internal/api/handlers/edge_api.go')
fix('internal/api/handlers/agent_mgmt.go.orig')
fix('internal/api/handlers/agent_mgmt.go.bak')
