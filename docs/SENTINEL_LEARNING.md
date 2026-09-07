# Sentinel Learning

Sentinel learning is designed to improve investigations without creating a second source of truth or allowing learned content to expand controller authority.

## Memory planes

Sentinel keeps four concerns separate:

1. **Authoritative World State** — current OMEGA/controller state queried from trusted controller data sources. Learned content never writes this plane.
2. **Episodic memory** — `sentinel_cases`, their evidence ledger, outcomes, conversations, and timestamps.
3. **Learned memory** — reusable generalizations stored in `sentinel_learned_memories` with evidence provenance, confidence, scope, validation state, counter-evidence, TTL, and supersession metadata.
4. **Procedural skills** — declarative investigation recipes in `sentinel_skills`.

Only `VALIDATED` learned memories and `TRUSTED` skills are exposed to the Context Compiler. They are serialized as `learned_knowledge`, never as `authoritative_state` and never as instructions.

## Hard trust boundary

The write path is intentionally one-way:

```text
model/tool output
    ↓
CANDIDATE memory / DRAFT skill
    ↓
controller validation
    ↓
historical replay
    ↓
live SHADOW evaluation
    ↓
TRUSTED artifact
```

A model response or tool result cannot write directly to trusted memory or create an immediately trusted skill.

Learning code cannot modify the Tool Registry, Safety Kernel, policy/trust roots, proposal approval boundary, executor, verification, or rollback mechanisms.

## Learned memory lifecycle

States:

```text
CANDIDATE → VALIDATED → SUPERSEDED
    │           │
    ├→ REJECTED └→ CANDIDATE  (new counter-evidence)
    └→ EXPIRED
```

A learned-memory candidate contains:

- source Case;
- Case/site/device scope;
- statement;
- confidence in `[0,1]`;
- evidence references into Case evidence ledgers;
- counter-evidence references;
- provenance;
- creator/timestamps;
- validation state;
- TTL / expiry;
- optional superseding memory.

The default TTL is 30 days and the controller maximum is 365 days. An hourly maintenance sweep expires stale candidate/validated memories.

Adding valid counter-evidence to a `VALIDATED` memory immediately demotes it to `CANDIDATE`, removing it from model context until it is replaced or deliberately resolved. A superseding memory must itself be validated and have the same scope.

The normal Sentinel history-retention sweep preserves a resolved source Case while that Case is still required by an unexpired candidate/validated memory or an active skill. This keeps provenance available for the lifetime of learned knowledge.

## Skill format

Skills are data interpreted by trusted controller code. Schema version 1 contains only:

```json
{
  "schema_version": 1,
  "name": "wan-loss-rca",
  "description": "Collect the read-only evidence normally useful for WAN loss cases.",
  "match": {
    "sources": ["log_anomaly"],
    "severities": ["high", "critical"],
    "keywords": ["packet loss"]
  },
  "evidence_tools": [
    "get_device_status",
    "get_incidents"
  ]
}
```

Unknown JSON fields are rejected. A skill cannot contain shell, Python, Lua, Go, SQL, HTTP requests, UCI commands, arbitrary command strings, credentials, policies, prompts that override controller policy, or executable code.

`evidence_tools` may reference only tools that already exist in the trusted Tool Registry and whose side-effect class is `none`. Learning therefore cannot manufacture a new primitive capability.

Even for a trusted skill, arguments are not stored in the skill. The Context Compiler reconstructs arguments from the active Case site/device scope and applies its normal global tool budget. A trusted skill can therefore suggest *which existing read-only evidence source to inspect*, not where or how to execute arbitrary work.

## Skill lifecycle

```text
DRAFT → VALIDATED → SHADOW → TRUSTED → DEPRECATED
  │         │          │         │
  └─────────┴──────────┴─────────┴→ REVOKED
```

### DRAFT

A human or optional curator may create a draft. Draft creation grants no runtime effect.

### VALIDATED

Static controller validation requires:

- schema version 1;
- valid bounded selectors;
- 1–4 evidence tools;
- every tool present in the trusted Tool Registry;
- every tool strictly read-only;
- no unknown executable fields.

### Historical replay

A validated skill can be replayed against past Cases. Replay reads only the persisted Case evidence ledger and ignores evidence whose `captured_at` is later than the selected historical `as_of` timestamp. Replay never executes a live tool.

A skill needs at least 3 matching historical Cases with an aggregate success ratio of at least 0.80 and zero unsafe evaluations before it may enter `SHADOW`.

Promotion accounting is based on **distinct Cases**, using the latest evaluation per `(mode, case)`. Replaying the same Case repeatedly cannot inflate promotion thresholds.

### SHADOW

Shadow skills are evaluated on live Cases only after the production investigation has completed and its evidence has been persisted. Shadow evaluation compares the skill recipe with evidence already collected by the production path:

- it does not request extra tools;
- it does not change compiled context;
- it does not change the answer;
- it does not create proposals;
- it has no execution authority.

A skill requires at least 5 matching shadow Cases, a shadow success ratio of at least 0.80, the replay threshold above, and zero unsafe evaluations before it can be promoted.

### TRUSTED

Trusted skills may contribute their already-validated read-only tool names to the Context Compiler. The compiler still enforces its global evidence-tool budget and Case-scoped argument construction.

Static safety validation is re-run when a trusted skill is loaded. If its referenced Tool Registry contract is no longer safe/available, it stops affecting the investigation even before an operator revokes it.

## Optional model curation

Issue #20 provides the optional `curation` reasoning class. #21 uses it as a teacher only, never as a trust authority.

Manual curation can be queued through:

```text
POST /api/sentinel/cases/{case_id}/curate
```

Automatic curation is disabled by default and is enabled with:

```text
SENTINEL_AUTO_CURATION=true
```

A curation route must also be explicitly configured through the model router. Because optional model tasks use the durable queue from #20, Internet/cloud unavailability does not block normal local Sentinel operation; the optional task is retried later.

The curator must return strict JSON containing at most five memory candidates and five skill drafts. It may cite only evidence IDs already present in the source Case evidence ledger. The controller independently verifies every cited evidence ID and clamps the artifact scope to the source Case before persisting it.

Imported artifacts always start as:

- memory: `CANDIDATE`;
- skill: `DRAFT`.

The curator has no API or internal path that can mark them `VALIDATED`, `SHADOW`, or `TRUSTED`.

## Context Compiler integration

For every Case investigation the compiler can include:

- unexpired `VALIDATED` memories that match tenant/site/device scope;
- `TRUSTED` skills whose declarative selectors match the Case.

Both are emitted under the `learned_knowledge` trust class with provenance and identifiers. They never overwrite the Case scope or authoritative OMEGA observations.

Trusted skills are merged into the normal prefetch plan only until the existing Context Compiler maximum of four read-only prefetches is reached. The model tool-call budget and Safety Kernel remain unchanged.

## API authorization

Read endpoints for memories, skills, and evaluation history require normal authenticated access.

Creating candidates/drafts, validating/rejecting/superseding memories, replaying or changing skill lifecycle state, and queueing curation require `ADMIN`.

## Safety invariants

The following must remain true in future changes:

- World State is never populated from learned memory.
- Model/tool output cannot bypass candidate/draft state.
- Learned memory cannot change its own validation state.
- Skills cannot add Tool Registry entries or side-effecting primitives.
- Skills cannot supply arbitrary tool arguments.
- Historical replay cannot see future evidence.
- Shadow mode cannot affect production behavior.
- Promotion thresholds are deterministic and based on distinct Cases.
- Counter-evidence removes contradicted learned memory from trusted context.
- Revocation is terminal and available from every non-revoked skill state.
- Optional cloud curation is not required for normal Sentinel operation.
