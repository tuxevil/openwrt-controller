# Sentinel Architecture

## Status

Design specification. This document defines the target architecture for Sentinel, OMEGA's native OpenWrt network operator. It is intentionally implementation-oriented but does not require immediate refactoring of the existing Sentinel PoC.

## Product intent

Sentinel is not a chatbot and it is not a general-purpose agent.

Sentinel is a persistent, evidence-driven OpenWrt network operator embedded in the OMEGA control plane. Its long-term objective is full operational autonomy over OpenWrt fleets while preserving a hard separation between model intelligence and execution authority.

The operating principle is:

> Sentinel may become fully autonomous, but it must never become sovereign over the mechanisms that constrain its authority.

OMEGA remains the authority for identity, policy, desired and observed state, compilation, rollout, rollback, audit, and recovery. Sentinel reasons over that control plane and requests typed network intents. It never receives unrestricted shell, SQL, filesystem, SSH, or arbitrary command execution capabilities.

## Scope

Sentinel is OpenWrt-only by design.

This restriction is deliberate. The goal is not to build a generic network automation framework that abstracts JunOS, IOS, RouterOS, Aruba, UniFi, and OpenWrt into a lowest-common-denominator interface. The goal is to understand OpenWrt deeply enough to reason safely about UCI, ubus, netifd, procd, rpcd, hostapd, wpa_supplicant, usteer, fw4/nftables, dnsmasq, odhcpd, SQM/CAKE, WireGuard, packages, board capabilities, and OpenWrt version differences.

The resulting specialization should reduce model burden and allow small local models to perform useful operational reasoning with strongly structured context and tools.

## Design goals

1. **Offline-first operation.** Normal monitoring, investigation, reasoning, memory recall, skill execution, proposal generation, policy evaluation, execution, verification, and rollback must work without Internet access.
2. **Local-model-first reasoning.** Routine operation should be viable with small local models in roughly the 4B-12B dense and <=35B MoE range. Frontier models are optional teachers and curators, not online dependencies.
3. **Full eventual autonomy.** Sentinel should be able to progress from read-only investigation to guarded and eventually autonomous operation for all ordinary OpenWrt network changes.
4. **Deterministic authority boundary.** LLM output must never directly decide authorization, mutate trust roots, expand tools, alter policy, bypass rollout safeguards, or execute arbitrary code.
5. **Evidence-driven conclusions.** Important conclusions must be traceable to current state, telemetry, logs, incidents, changes, probes, prior episodes, or explicitly identified operator knowledge.
6. **Typed intent instead of commands.** The model expresses semantic network intent. OMEGA compiles it into desired-state changes and device operations.
7. **Learning with provenance.** Persistent knowledge must retain source, evidence, confidence, age, scope, and validation state.
8. **Self-learning without self-escalation.** Sentinel may autonomously create memories and procedural skills, but learned artifacts cannot grant new authority or arbitrary execution capabilities.
9. **Reversible-by-default actions.** Any mutable operation should have an explicit verification target and recovery strategy.
10. **Fail useful without AI.** Loss of the LLM must not disable telemetry, deterministic health checks, policy enforcement, rollback, adoption, state reconciliation, or already-authorized recovery mechanisms.

## Non-goals

Sentinel is not intended to:

- become a general-purpose personal assistant;
- expose bash, SSH, raw SQL, arbitrary HTTP, or unrestricted filesystem tools to a model;
- replace deterministic controller logic with LLM reasoning;
- store live network state in LLM memory;
- require a frontier cloud model for normal operation;
- permit generated code as a learned skill;
- allow an agent to modify its own authorization boundary;
- infer support for non-OpenWrt network operating systems.

## Trust model

Assume all of the following can fail or become hostile:

- the active LLM;
- model output parsing;
- a prompt or conversation;
- operator-provided notes;
- logs and telemetry strings;
- SSIDs, hostnames, DHCP options, mDNS names, LLDP descriptions, DNS names, HTTP metadata, and other device/client-controlled strings;
- retrieved memory;
- retrieved skills;
- a frontier-model response;
- a learned artifact;
- a model provider.

The system must remain safe if Sentinel is prompt-injected or produces malicious output.

The following are separate trust anchors and are not writable by Sentinel:

- authentication and device identity;
- controller and agent trust roots;
- the Safety Kernel;
- policy definitions that govern Sentinel's own authority;
- invariant implementation;
- the operation executor;
- rollback/recovery machinery;
- the tool registry implementation;
- the skill DSL interpreter;
- signed built-in recovery procedures.

## High-level architecture

```text
                      operator / event
                            |
                            v
                    +---------------+
                    | Sentinel Case |
                    +-------+-------+
                            |
              +-------------+-------------+
              |             |             |
              v             v             v
       Context Compiler  Memory Recall  Skill Router
              |             |             |
              +-------------+-------------+
                            |
                            v
                    +---------------+
                    | Sentinel Brain|
                    | local LLM     |
                    +-------+-------+
                            |
                 evidence / hypotheses /
                    semantic intent
                            |
                            v
        =================================================
        |              OMEGA SAFETY KERNEL              |
        |                                               |
        | identity  policy  risk  invariants  compiler |
        | rollout   audit   limits  authorization       |
        |                                               |
        |          deterministic / non-LLM              |
        =================================================
                            |
                      typed operation
                            |
                            v
                    OpenWrt device agent
                            |
             journal -> apply -> restart/verify
                            |
                     commit / rollback
                            |
                            v
                    post-change verifier
                            |
                            v
                    episode + learning
```

The line between the Sentinel Brain and the Safety Kernel is the primary security boundary.

## Core components

### Sentinel Case

A Case is the durable unit of operational reasoning. Conversations are one possible interface to a Case, but a Case can also originate from automation.

Examples of case origins:

- operator question;
- WAN packet loss;
- device offline transition;
- excessive wireless disconnects;
- configuration drift;
- rollout failure;
- unknown client appearance;
- DNS health degradation;
- controller-generated anomaly;
- synthetic probe failure.

A Case should retain:

- scope (tenant/site/device/client/resource);
- trigger;
- timestamps;
- structured evidence references;
- hypotheses;
- conclusions;
- proposed intents;
- operations executed;
- expected outcomes;
- observed outcomes;
- rollback history;
- models and skills used;
- policy decisions;
- final resolution.

A chat conversation may attach to a Case, and multiple conversations may refer to the same Case.

### Context Compiler

The model should not receive an unrestricted database dump.

The Context Compiler builds the smallest authoritative context appropriate for the current Case or operator intent.

Example:

```text
Operator: "why does my laptop keep disconnecting?"

Context Compiler ->
  site summary
  resolved client identity
  current association
  recent association history
  radio health
  relevant channel metrics
  recent site/device changes
  active incidents
  matching memories
  matching skills
```

The compiler distinguishes:

- authoritative state;
- measurements;
- operator assertions;
- learned knowledge;
- untrusted observations.

Untrusted strings are always structurally labeled as data and are never concatenated into privileged instructions.

### Tool Registry

Sentinel tools are domain-specific, typed, bounded, and capability-aware.

A tool declaration should eventually include at least:

```text
name
category
input schema
output schema
side-effect class
risk class
required scope
required device capability
timeout
cost hint
result trust class
rate limit
```

Examples of read tools:

```text
inventory.get_device
inventory.list_devices
clients.find
clients.get_experience
wifi.get_radio_health
wifi.get_associations
wifi.get_roaming_history
wifi.get_channel_environment
wan.get_status
wan.get_latency_history
dhcp.get_lease
dns.get_health
topology.get_path
telemetry.query_metric
config.get_desired
config.get_observed
config.get_drift
changes.get_recent
incidents.get
logs.search
```

Examples of bounded diagnostic tools:

```text
probe.ping
probe.dns
probe.http
probe.traceroute
probe.iperf
```

A diagnostic tool does not expose a shell command. For example, `probe.ping` accepts a typed request with source, target, count, and timeout constraints; the controller or device agent decides how to execute it.

Tools with side effects should normally be represented as NetworkIntents rather than direct tool calls.

The model never receives `bash`, `ssh`, `sql`, `curl`, or arbitrary executable tools.

### Skill Router

Do not expose every skill or every tool to every model invocation.

The Skill Router maps a Case or operator intent to a small specialist domain and loads only relevant procedural knowledge and tool schemas.

Initial specialist domains may include:

- Wireless;
- WAN;
- LAN / DHCP / DNS;
- Firewall / security;
- Device / firmware;
- Change / rollout / recovery.

These are logical specializations, not necessarily independent agents. The preferred initial architecture is workflow-first rather than a multi-agent swarm.

A single local model may assume different specialist roles by receiving different system instructions, tools, skills, and context.

### Sentinel Brain

The Brain is responsible for:

- interpreting the operator goal or Case trigger;
- identifying missing evidence;
- selecting read and diagnostic tools;
- forming and ranking hypotheses;
- deciding whether enough evidence exists;
- selecting a procedural skill;
- producing explanations;
- generating typed NetworkIntents;
- defining expected outcomes for proposed actions;
- identifying unresolved uncertainty.

The Brain is not responsible for:

- authorization;
- compiling OpenWrt configuration;
- validating invariants;
- deciding whether policy permits an action;
- bypassing rollout strategy;
- committing device state;
- handling rollback mechanics.

### Safety Kernel

The Safety Kernel is deterministic and non-LLM.

It evaluates every potentially mutating intent against:

- authenticated principal;
- tenant/site/device scope;
- target capabilities;
- current desired and observed state;
- risk classification;
- autonomy policy;
- explicit invariants;
- blast radius;
- change frequency and anomaly limits;
- rollback availability;
- rollout strategy;
- management-path safety;
- required approval state;
- controller/device health prerequisites.

It returns a decision such as:

```text
ALLOW
DENY
REQUIRE_APPROVAL
REQUIRE_CANARY
REQUIRE_STRONGER_VERIFICATION
```

Sentinel cannot modify the Safety Kernel or the rules that determine Sentinel's own authority.

### NetworkIntent

The model should not produce UCI as the stable long-term interface.

A NetworkIntent is a typed semantic request. Example:

```json
{
  "kind": "wifi.channel.change",
  "target": {
    "device_id": "AP-LIVING",
    "radio": "2.4GHz"
  },
  "parameters": {
    "channel": 11
  },
  "reason": {
    "case_id": "...",
    "evidence_ids": ["...", "..."]
  },
  "expected_outcome": {
    "metric": "wifi.client_retry_rate",
    "direction": "decrease",
    "verification_window": "15m"
  }
}
```

The controller resolves the semantic target against hardware capabilities and current state, updates or derives desired state, and compiles an exact immutable plan.

This keeps the model independent from low-level identifiers such as anonymous UCI section numbers, radio numbering conventions, DSA details, or OpenWrt-version-specific rendering.

### OpenWrt Semantic Compiler

The compiler translates a validated NetworkIntent into the exact desired-state delta and device operation plan.

Responsibilities include:

- resolve logical resources to concrete OpenWrt resources;
- check capability compatibility;
- render exact UCI/ubus/package/service actions;
- produce a deterministic plan hash;
- bind the plan to a rollout generation;
- attach required service restarts;
- attach deterministic health and verification checks;
- produce rollback metadata.

The same semantic intent may compile differently for different devices while preserving the same operator meaning.

### Invariants

Invariants are deterministic properties that must remain true regardless of model reasoning.

Initial examples:

```text
controller management path remains reachable
LAN networks do not overlap where policy forbids overlap
DHCP pools remain inside their configured subnet
default route requirements remain satisfied
management SSH is not unexpectedly exposed to WAN
critical WLAN minimum availability is preserved
adopted devices retain valid device identity
required resolver paths remain available
an operation cannot silently expand beyond its authorized scope
```

Invariant evaluation occurs after intent compilation and before mutation.

### Risk and autonomy policy

Risk level and autonomy level are separate concepts.

Suggested risk classes:

| Risk | Typical actions | Initial behavior | Long-term behavior |
| --- | --- | --- | --- |
| R0 | state, telemetry, logs, topology | automatic | automatic |
| R1 | ping, DNS probe, bounded diagnostics | automatic | automatic |
| R2 | service restart, non-critical Wi-Fi tuning | approval or guarded | automatic when trusted |
| R3 | firewall policy, critical WLAN, WAN, SQM | approval | guarded autonomy |
| R4 | LAN addressing, management path, fleet firmware, major routing | strong approval | tightly guarded autonomy |
| R5 | identity roots, Sentinel authority, Safety Kernel, policy roots, executor trust | impossible for Sentinel | impossible for Sentinel |

R5 remains outside Sentinel permanently. This does not prevent full operational autonomy over ordinary network administration; it prevents the agent from granting itself additional authority.

Autonomy modes may evolve per site, device, resource, or action kind:

```text
OBSERVE   -> investigate and explain only
ASSIST    -> investigate and propose
GUARDED   -> execute pre-authorized low-risk actions
AUTONOMOUS -> execute allowed actions under policy and verification
```

Autonomy should be earned by evidence and policy, not by changing model prompts.

### Rollout and blast-radius control

Fleet-wide intents must not translate directly into fleet-wide simultaneous execution.

The rollout engine determines staging independently of Sentinel, for example:

```text
canary
 -> verify
5 percent
 -> verify
25 percent
 -> verify
remaining fleet
```

Sentinel may request a fleet intent. OMEGA controls concurrency, canaries, phases, stop conditions, and rollback.

### Post-change verifier

A successful command or UCI commit does not complete an AI-driven action.

Each mutable intent must include an expected observable result.

Example:

```text
hypothesis:
  channel congestion causes client instability

action:
  change radio channel

expected:
  retry rate decreases
  disconnect frequency decreases

window:
  15 minutes
```

The verifier compares the post-change state against:

- the explicit expected outcome;
- pre-change baseline;
- safety health checks;
- management reachability;
- unrelated regressions.

Possible results:

```text
SUCCESS
PARTIAL
NEUTRAL
REGRESSION
INCONCLUSIVE
```

A regression may trigger deterministic rollback if policy and rollback guarantees allow it.

The outcome becomes part of the Case and learning dataset.

## Memory model

Sentinel memory is not a generic transcript store and is not a substitute for controller state.

Four distinct classes must remain separate.

### World State

Current truth belongs to OMEGA databases and telemetry systems.

Examples:

```text
AP3 is on channel 11
client X is currently associated to AP2
WAN is down
OpenWrt version is 25.x
desired generation is 48
```

Sentinel queries World State when needed. It does not persist these facts as learned memory merely to avoid querying the controller.

### Episodic Memory

An episode records what happened in a specific operational event.

Example:

```text
14:02 client begins disconnecting
14:03 RSSI remains healthy
14:04 retry rate rises sharply
14:05 channel utilization reaches 92 percent
14:07 Sentinel proposes channel change
14:09 change applied
14:24 retries normalize
outcome: success
```

Episodes should primarily derive from Cases and immutable operation/evidence records.

### Learned Memory

Learned Memory captures a generalization supported by evidence.

A learned-memory record should eventually include:

```text
id
kind
scope
statement
confidence
evidence references
counter-evidence references
source class
created_at
last_used_at
last_verified_at
expires_at
superseded_by
validation state
```

Example:

```text
scope: hardware:model/WNDR3800CH
statement: management recovery after some network rollbacks may exceed the nominal rollback timeout
confidence: 0.82
evidence: cases 31, 44, 61
```

Learned statements are never silently promoted to authoritative live state.

### Procedural Skills

Skills represent how Sentinel investigates or handles a class of problem.

Skills are declarative data, not executable code.

Sentinel-generated skills must not contain shell, Python, Lua, Go, SQL, arbitrary HTTP, or arbitrary commands.

A future skill DSL may resemble:

```yaml
name: investigate-wifi-roaming-failure
version: 7

trigger:
  intents:
    - client_wifi_disconnect
    - roaming_failure

requires:
  - wifi.association_history
  - wifi.radio_health

tools:
  - clients.find
  - wifi.get_associations
  - wifi.get_roaming_history
  - wifi.get_radio_health
  - wifi.get_channel_environment
  - changes.get_recent

procedure:
  - identify affected client
  - determine whether failure is client-only or site-wide
  - inspect RSSI, SNR, retry rate, and disconnect timing
  - inspect recent roaming transitions
  - inspect channel congestion
  - correlate with recent configuration changes

possible_intents:
  - wifi.channel.change
  - wifi.tx_power.adjust
  - wlan.fast_transition.disable

verification:
  window: 15m
  metrics:
    - client.disconnect_rate
    - client.retry_rate
```

The DSL interpreter is trusted controller code. Sentinel may create data consumed by the interpreter but may not extend interpreter capabilities.

## Autonomous learning lifecycle

Self-learning should be autonomous, but learning is not equivalent to immediate trust.

Suggested lifecycle:

```text
episode(s)
   |
   v
pattern detection
   |
   +-------------------+
   |                   |
   v                   v
candidate memory   candidate skill
   |                   |
   v                   v
schema validation  static validation
   |                   |
contradiction      historical replay
analysis               |
   |                   v
   +---------------> SHADOW
                       |
                 live evaluation
                       |
                       v
                    TRUSTED
```

Recommended states:

```text
DRAFT
VALIDATED
SHADOW
TRUSTED
DEPRECATED
REVOKED
```

A learned artifact cannot alter the Safety Kernel, introduce arbitrary tool capabilities, or grant permissions.

### Historical replay

Candidate skills should be evaluated against prior Cases using only evidence that existed at the historical timestamp.

Useful evaluation metrics include:

- diagnostic precision;
- evidence sufficiency;
- false-positive rate;
- unsafe-proposal rate;
- tool-call cost;
- time to diagnosis;
- successful resolution rate;
- post-change regression rate.

### Shadow mode

A candidate skill in SHADOW may run against live Cases and produce conclusions or proposed intents that are recorded but do not influence the operational path.

Promotion to TRUSTED may be automatic if deterministic acceptance thresholds are satisfied.

## Memory-poisoning controls

No tool result or LLM output writes directly to trusted persistent memory.

The minimum pipeline is:

```text
CandidateMemory
  -> schema validation
  -> source/trust classification
  -> evidence binding
  -> contradiction search
  -> confidence calculation
  -> quarantine/shadow
  -> trusted learned memory
```

Evidence from attacker-controlled sources such as client-provided hostnames, SSIDs, log message payloads, or network metadata receives lower trust than operator declarations or controller-generated measurements.

Contradictions must be retained rather than overwritten silently.

## Frontier model role

Frontier cloud models are optional teachers and curators.

Suitable frontier tasks:

- synthesize candidate skills from successful episodes;
- review or rewrite procedural skills;
- generate additional replay/eval cases;
- consolidate learned memories;
- detect contradictions and duplicate knowledge;
- analyze hard unresolved Cases;
- improve taxonomies and specialist procedures.

Frontier models are not required to execute live network changes.

The controller should sanitize exported context and avoid sending credentials, raw secrets, unnecessary MAC addresses, public IPs, hostnames, or full configurations when a structurally equivalent representation is sufficient.

If Internet is unavailable, local Sentinel must continue operating. Frontier curation jobs remain queued until connectivity returns.

## Model routing

The harness should classify reasoning requirements rather than bind Sentinel to one model.

Illustrative routing:

```text
small local model (4B-8B)
  intent routing
  entity extraction
  log classification
  context summarization
  skill retrieval
  candidate-memory extraction

strong local model (12B dense / <=35B MoE)
  investigation
  root-cause analysis
  hypothesis generation
  planning
  counterfactual reasoning
  intent generation

frontier model
  skill synthesis and review
  memory consolidation
  hard unresolved RCA
  eval generation
  meta-learning
```

The exact models are configuration, not architecture.

The Safety Kernel behaves identically regardless of model class or provider.

## Prompt-injection posture

All network-derived text is untrusted evidence.

Examples include:

- SSID names;
- DHCP hostnames and options;
- mDNS names;
- DNS records;
- LLDP descriptions;
- syslog payloads;
- HTTP metadata;
- user-controlled device labels;
- retrieved notes unless their provenance establishes operator authority.

The Context Compiler must preserve structural boundaries and trust labels. Tool results are data, never instructions.

Prompt defenses are defense-in-depth only. The primary protection is that model output cannot bypass the Safety Kernel or obtain arbitrary execution capability.

## Sentinel Guard

Sentinel Guard is a deterministic behavioral circuit breaker, not another LLM.

It watches the agent itself for anomalous behavior such as:

- unusual tool-call bursts;
- repeated denied operations;
- rapid scope expansion;
- attempts to target many devices unexpectedly;
- simultaneous changes to unrelated sensitive resources;
- memory-write bursts;
- skill-generation bursts;
- repeated requests involving management reachability;
- configuration churn exceeding policy thresholds;
- abnormal deviation from historical action patterns.

Guard responses may include:

```text
RATE_LIMIT
PAUSE_MUTATIONS
REQUIRE_APPROVAL
DROP_TO_OBSERVE
REVOKE_ACTIVE_AUTONOMY_LEASE
```

The operator must retain an out-of-band method to disable Sentinel mutation authority without depending on Sentinel itself.

## Capability leases

For higher-risk future autonomy, consider short-lived capability leases instead of permanent broad authorization.

Example:

```text
principal: sentinel
site: mountain-home
allowed intents:
  wifi.channel.change
  service.restart
risk <= R2
expires: 6h
max operations: 10
max devices per operation: 2
```

The policy engine, not the model, issues and validates the lease.

This is optional for the first implementation but fits the long-term threat model better than static broad permission.

## Audit requirements

Every model-assisted mutation must be reconstructable.

Record at minimum:

- Case ID;
- model identity/version;
- model-provider class (local/frontier);
- prompt/context artifact hashes where practical;
- skill IDs and versions;
- memory IDs retrieved;
- evidence references;
- hypotheses;
- generated NetworkIntent;
- policy decision;
- invariant results;
- compiled plan hash;
- rollout/operation IDs;
- exact target set;
- approval identity when applicable;
- pre-change state reference;
- post-change verification;
- rollback result;
- final outcome.

Audit records must not depend solely on model-authored summaries.

## Built-in recovery skills

OMEGA should eventually ship signed, immutable built-in operational procedures for critical recovery paths.

Examples:

- controller reachability investigation;
- device reconnect after failed rollout;
- DNS service recovery;
- WAN interface recovery;
- wireless service recovery;
- safe rollback verification.

Sentinel may learn alternative or improved procedures, but it cannot remove or rewrite the trusted built-in recovery baseline.

## Synthetic Sentinel Probes

Long-term, OMEGA device agents can provide bounded synthetic experience tests so Sentinel can observe the network even without active users.

Candidate probes:

- controller reachability;
- default gateway reachability;
- DNS resolution;
- HTTPS Internet reachability;
- latency and packet loss;
- NTP;
- WireGuard peer reachability;
- WAN failover validation;
- DHCP server responsiveness where safe.

The deterministic controller schedules and bounds these probes. Sentinel consumes their results as evidence.

## Existing Sentinel implementation

The current Sentinel PoC already contains several patterns that should be retained:

- bounded investigation rounds and tool calls;
- explicit tool allow-listing;
- site scoping;
- persisted conversations and runs;
- persisted evidence;
- secret redaction;
- Cases and Notes;
- proposals separated from investigations;
- explicit proposal approval;
- a system-prompt rule that logs, notes, and tool results are evidence rather than instructions.

These are compatible with the target architecture.

The primary changes required over time are architectural rather than a rewrite of all existing behavior:

1. extract the current hard-coded tool allow-list into a typed Tool Registry;
2. make Case the durable operational unit and conversation a UI/interface surface;
3. introduce the Context Compiler;
4. introduce skill retrieval and a declarative skill format;
5. replace model-authored UCI proposals with NetworkIntent;
6. route NetworkIntent through the same immutable, generation-bound operation path used by the controller;
7. add explicit post-change verification;
8. add episodic and learned memory with provenance;
9. add candidate-skill replay and shadow evaluation;
10. add deterministic Sentinel Guard behavior.

No mutating Sentinel architecture should bypass completion of the core controller work required for safe durable operations.

## Dependency on controller core hardening

Sentinel autonomy depends on a reliable control plane. Before Sentinel receives meaningful mutation authority, the controller should provide:

- durable per-device operation execution;
- separate unique operation attempt IDs and content plan hashes;
- immutable preview/apply binding;
- device-local transactional apply/rollback;
- controller-visible operation state;
- generation-bound reconciliation;
- canary and staged rollout behavior;
- capability-aware rendering;
- strong device identity and authenticated transport;
- management-path-aware verification;
- a single canonical mutation path rather than parallel SSH and agent writers.

Until these guarantees exist, Sentinel should remain primarily investigative and proposal-oriented.

## Proposed implementation phases

### Phase S0 - Preserve and stabilize the PoC

- keep current read-only investigation behavior;
- keep proposal/execution separation;
- fix current core-controller reliability issues before expanding mutation authority;
- avoid adding arbitrary execution tools.

### Phase S1 - Native harness foundation

- Tool Registry;
- Context Compiler;
- Case-first orchestration;
- specialist/skill routing;
- structured evidence references;
- model router with local-first policy.

Success criterion: useful investigations with a small local model and bounded relevant context.

### Phase S2 - Memory

- episodic Case representation;
- learned-memory schema;
- provenance, confidence, contradiction, TTL, and supersession;
- local memory retrieval;
- autonomous candidate-memory generation;
- frontier memory-curation queue.

Success criterion: Sentinel can use prior experience without confusing it with current network truth.

### Phase S3 - Skills

- declarative skill DSL;
- built-in signed skills;
- retrieval and execution;
- autonomous candidate-skill generation;
- historical replay;
- SHADOW and TRUSTED promotion states;
- frontier skill-curation queue.

Success criterion: newly learned procedures improve measured investigation quality without introducing code execution.

### Phase S4 - Semantic actions

- NetworkIntent schema registry;
- OpenWrt semantic compiler;
- risk classes;
- invariants;
- policy evaluation;
- immutable plan linkage;
- expected-outcome definitions.

Success criterion: the model never needs to emit raw UCI for supported actions.

### Phase S5 - Closed-loop guarded operation

- post-change verifier;
- automatic rollback on regression where safe;
- Sentinel Guard;
- R2 guarded autonomy;
- canary/staged fleet actions;
- full audit chain.

Success criterion: selected low-risk incidents can be resolved end-to-end without operator intervention and with reproducible evidence.

### Phase S6 - Proactive Sentinel

- event-triggered Cases;
- anomaly-triggered investigation;
- synthetic probes;
- autonomous investigation in shadow mode;
- proactive operator summaries.

Success criterion: Sentinel discovers and diagnoses meaningful issues before an operator asks.

### Phase S7 - Broad guarded autonomy

- policy-driven R3/R4 autonomy;
- short-lived capability leases if adopted;
- learned skill promotion based on outcome history;
- automatic blast-radius adaptation based on confidence and health;
- emergency operator kill switch and autonomy downgrade.

Success criterion: OMEGA can operate an OpenWrt fleet for extended periods with minimal human intervention while preserving deterministic authority boundaries.

## Architectural invariants for Sentinel itself

These requirements should be treated as non-negotiable unless this architecture document is deliberately superseded by an operator-reviewed design change:

1. Sentinel never receives unrestricted arbitrary code execution.
2. Sentinel cannot modify the system that determines Sentinel's own authority.
3. Current network truth comes from OMEGA state, not learned memory.
4. Learned persistent knowledge carries provenance.
5. Learned skills are declarative and cannot introduce new primitive capabilities.
6. Every mutating model decision crosses a deterministic policy boundary.
7. Every autonomous mutation has an expected observable outcome.
8. High-blast-radius operations are staged independently of the model.
9. A model or Internet outage cannot disable deterministic recovery and controller operation.
10. Every autonomous change is auditable from evidence through verified outcome.

## Near-term repository direction

The immediate engineering priority remains controller-core consolidation. Sentinel should not force premature execution features while the controller still has competing mutation paths or incomplete rollout guarantees.

In parallel, new Sentinel work should prefer changes that move the PoC toward this architecture without increasing mutation risk:

- typed Tool Registry;
- Case-first orchestration;
- structured evidence;
- Context Compiler;
- model routing;
- read-only skills;
- episodic memory foundations.

Raw command execution, broad package-management tools, unconstrained HTTP, shell, direct SSH mutation, and automatically trusted generated skills are explicitly out of scope.
