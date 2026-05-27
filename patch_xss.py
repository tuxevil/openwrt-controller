import os

filepath = 'web/src/views/SecurityMatrix.vue'
with open(filepath, 'r') as f:
    content = f.read()

content = content.replace("import { marked } from 'marked';", "import { marked } from 'marked';\nimport DOMPurify from 'dompurify';")
content = content.replace("return marked.parse(text);", "return DOMPurify.sanitize(marked.parse(text));")

with open(filepath, 'w') as f:
    f.write(content)
