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
