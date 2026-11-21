# Sentinel local-first model routing

Sentinel selects a **reasoning class**, never a provider. The routing layer maps that class to an OpenAI-compatible endpoint and model alias.

## Reasoning classes

| Class | Purpose | Normal-path policy |
| --- | --- | --- |
| `routine` | extraction, classification, summaries, simple investigations | local only |
| `deep_rca` | harder RCA, hypothesis generation, planning | local only; falls back to `routine` |
| `curation` | optional skill/memory/eval consolidation | explicit route; asynchronous |
| `frontier_escalation` | optional unresolved-case analysis | explicit route; asynchronous |

Operator/network text can select only `routine` or `deep_rca`. `curation` and `frontier_escalation` are controller-owned workload types; prompt text cannot select them.

## Configuration

The existing platform AI engine remains the common OpenAI-compatible gateway/base configuration. Per-class environment variables can override it:

```text
SENTINEL_ROUTINE_BASE_URL
SENTINEL_ROUTINE_MODEL
SENTINEL_ROUTINE_API_KEY
SENTINEL_ROUTINE_LOCAL

SENTINEL_DEEP_RCA_BASE_URL
SENTINEL_DEEP_RCA_MODEL
SENTINEL_DEEP_RCA_API_KEY
SENTINEL_DEEP_RCA_LOCAL

SENTINEL_CURATION_BASE_URL
SENTINEL_CURATION_MODEL
SENTINEL_CURATION_API_KEY
SENTINEL_CURATION_LOCAL

SENTINEL_FRONTIER_BASE_URL
SENTINEL_FRONTIER_MODEL
SENTINEL_FRONTIER_API_KEY
SENTINEL_FRONTIER_LOCAL
```

`routine` defaults to the platform AI engine model. `deep_rca` defaults to the routine model unless a stronger local alias is configured. `curation` and `frontier_escalation` require an explicit model alias, so enabling a general AI engine cannot silently cause Case context to be sent to a remote model.

Local endpoints may be keyless. Remote optional routes require an API key. Locality is inferred from loopback/private/link-local addresses, `.local`/`.internal` names, and single-label service names. `*_LOCAL=true|false` is an explicit operator override for deployments where DNS does not reveal topology.

Example using a local LiteLLM gateway:

```text
AI engine base URL: http://litellm:4000/v1
AI engine model: sentinel-routine
SENTINEL_DEEP_RCA_MODEL=sentinel-deep-rca
```

The gateway aliases can point to Ollama, llama.cpp, vLLM, MLX, or any other OpenAI-compatible local backend without Sentinel knowing which provider/runtime serves them.

Optional remote routes can use the same gateway or a separate OpenAI-compatible endpoint:

```text
SENTINEL_FRONTIER_BASE_URL=https://ai-gateway.example.com/v1
SENTINEL_FRONTIER_MODEL=sentinel-frontier
SENTINEL_FRONTIER_API_KEY=...
```

## Offline behavior

Normal Case investigation uses only `routine`/`deep_rca`; those classes reject non-local routes. Tool Registry access, policy checks, proposals, approval, execution, verification, and rollback remain unchanged by model routing.

Optional `curation` and `frontier_escalation` work is stored in the tenant-local `sentinel_model_tasks` queue. Tasks are claimed transactionally and requeued with bounded exponential backoff when the route is unavailable. Controller restart does not make the task part of World State or give it execution authority.

Automatic frontier escalation is disabled by default. It can be enabled explicitly with:

```text
SENTINEL_AUTO_FRONTIER_ESCALATION=true
```

When enabled, an unresolved local investigation can enqueue a `frontier_escalation` artifact. The operator-facing local investigation remains complete independently of whether that optional task can reach its route.

## Trust boundary

Routing changes model selection only. It does **not** change:

- Tool Registry capabilities or budgets;
- Case/site/device scope injection;
- Context Compiler provenance classes;
- proposal validation;
- approval requirements;
- the device-operation executor or rollback path.

A stronger or remote model therefore has exactly the same Safety Kernel boundaries as a small local model.

## Modo off-grid de Sentinel

Configura `SENTINEL_OFFGRID=true`, `SENTINEL_ROUTINE_MODEL=gemma4:e2b` y `SENTINEL_ROUTINE_BASE_URL=http://127.0.0.1:11434/v1` si Ollama comparte el host. Para un proveedor LAN con DNS y HTTPS, conserva su URL y configura `SENTINEL_ROUTINE_LOCAL=true` si el nombre tiene un dominio público. La conexión usa solo direcciones privadas o de loopback y TLS valida el hostname original. El transporte de inferencia descarta proxies y redirecciones HTTP en este modo. El monitor conserva incidentes y casos localmente y omite Telegram y los webhooks de creación de incidentes.

La conversación aparece en **Sentinel AI** del dashboard. Usa las ejecuciones asíncronas existentes y muestra la respuesta, el identificador del caso y la evidencia devuelta. El monitor de alertas existente ejecuta el barrido aunque el navegador esté cerrado. El modo off-grid se aplica a la inferencia y a la entrega de incidentes. El aislamiento de red del despliegue completo sigue siendo una prueba independiente.

## Contrato de respuesta para investigaciones

El modelo devuelve un objeto JSON con `message`, `tool_calls` y una
`proposal` opcional. Sentinel acepta el objeto directamente o dentro de
un único bloque Markdown `json`. Una respuesta vacía, truncada o con texto
fuera del objeto produce un error de investigación. Sentinel valida el
turno antes de enviar llamadas al Tool Registry. Conserva la evidencia
recopilada hasta ese punto en el resultado de la investigación.

## Tool calling nativo por ruta

El flujo por Case usa `legacy_json` por defecto. Configura
`SENTINEL_ROUTINE_TOOL_MODE=native_tools` o
`SENTINEL_DEEP_RCA_TOOL_MODE=native_tools` para enviar los esquemas
del registro como funciones OpenAI-compatible. El modo nativo conserva
los mensajes assistant y devuelve cada resultado en un mensaje tool con
el identificador de la llamada. El modelo devuelve la conclusión en
`content` como JSON con `message` y, si procede, una `proposal` sujeta
a la aprobación habitual. La ruta nativa queda fijada durante el Case.

Prueba tu modelo y servidor con una consulta que requiera herramientas.
Algunos servidores anuncian compatibilidad OpenAI pero implementan un
subconjunto del formato. Para volver al contrato anterior, establece
`legacy_json` en la ruta y reinicia el controlador. Puedes cambiar el
modelo o endpoint mediante las variables de la misma ruta.
