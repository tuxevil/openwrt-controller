import os

filepath = 'internal/api/handlers/vault.go'
with open(filepath, 'r') as f:
    content = f.read()

sema = """
// Limit concurrent firmware uploads to prevent OOM
var uploadSemaphore = make(chan struct{}, 5)
"""

if "uploadSemaphore" not in content:
    content = content.replace('func UploadFirmwareHandler(w http.ResponseWriter, r *http.Request) {', sema + '\nfunc UploadFirmwareHandler(w http.ResponseWriter, r *http.Request) {\n\tuploadSemaphore <- struct{}{}\n\tdefer func() { <-uploadSemaphore }()\n')

with open(filepath, 'w') as f:
    f.write(content)
