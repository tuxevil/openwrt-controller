import os
import glob
import re

handlers = glob.glob("internal/api/handlers/*.go")

for file in handlers:
    with open(file, "r") as f:
        content = f.read()

    # Replace database.DB.Query(...) with database.Query(r.Context(), ...)
    content = content.replace("database.DB.Query(", "database.Tx(r.Context()).Query(")
    content = content.replace("database.DB.QueryRow(", "database.Tx(r.Context()).QueryRow(")
    content = content.replace("database.DB.Exec(", "database.Tx(r.Context()).Exec(")
    
    with open(file, "w") as f:
        f.write(content)
