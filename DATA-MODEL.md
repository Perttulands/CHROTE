---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# CHROTE Data Model Spec

Status: **Active core source of truth**.

This document defines the durable state CHROTE owns or projects. It covers the
cockpit, Services, Beads, and Formations. Implementation details may live in code
or narrower docs, but they should not contradict this file.

## State ownership

| State | Owner | Notes |
| --- | --- | --- |
| tmux sessions | tmux socket | CHROTE observes and attaches; tmux owns lifecycle |
| terminal panes/layout | dashboard local state + CHROTE APIs | Browser layout is preference, not work truth |
| workspace files | configured host roots | Server enforces root restrictions |
| Beads issues | `bd` workspaces | CHROTE renders JSON output and performs bounded writes |
| service credentials | host runtime config | Never sent to browser |
| service data | upstream local services | CHROTE proxies and displays selected operations |
| formation boards | `.formations/boards/` | TOML structural definitions |
| formation layout | `.formations/layout/` | TOML sidecars for visual placement/routing |
| agent personas | configured persona roots | TOML cards with stable ids and harness variants |
| run history | `.formations/runs/` | Append-only NDJSON ledgers |

## Dashboard settings

User settings are browser-local preferences unless explicitly backed by server
state.

Current theme ids:

```text
matrix
dark
gastown
```

Host tmux appearance presets may share these ids but apply to tmux status/pane
colors on the host. Treat that as a host-side effect, not cosmetic browser-only
state.

## Session model

```ts
Session {
  name: string
  windows: number
  attached: boolean
  group: string
}
```

Grouping is derived from configurable naming rules. Group labels are display
helpers only; they are not proof that any particular harness or orchestrator is
installed.

## Beads model

CHROTE consumes `bd --json` and keeps Beads as the task source of truth.
Dashboard writes must be bounded operations such as comments or state transitions
that `bd` supports. CHROTE must not fork a second issue database.

## Service adapter model

```text
Browser -> CHROTE route -> localhost upstream service
```

The browser never receives upstream bearer tokens. Missing tokens or unavailable
upstreams produce degraded states.

## Formations files

Recommended layout:

```text
.formations/
  boards/
    <slug>.formation.toml
  layout/
    <slug>.layout.toml
  runs/
    <slug>/
      <run-id>.ndjson
      <run-id>.snapshot.toml
      <run-id>.bindings.toml
  artifacts/
    <run-id>/
      <agent-created output files>
agents/ or CHROTE_AGENTS_DIR/
  <agent-id>.toml
```

### Board definition

A board definition contains structural state:

- board id/name/version;
- missions;
- formations;
- gates, including `kinds`, `criterion`, and optional structured script gate
  command fields (`commandArgv` or explicit `commandShell`, plus optional
  `commandCwd`; legacy `command` is parseable compatibility metadata, not the
  executable contract);
- slots;
- ports;
- connections;
- verification/check specs.

It does not contain pixel positions, viewport state, or run event history.

### Layout sidecar

A layout sidecar contains visual state:

- node positions;
- collapsed/expanded local UI flags that are safe to persist;
- hand-routed wire lanes;
- canvas grouping hints.

It does not define graph semantics.

### Persona card

A persona card contains stable assignment identity:

- agent id;
- display name;
- tags/capabilities;
- harness variants;
- default cwd/root constraints;
- optional safety notes.

### Run ledger

Run ledgers are append-only NDJSON. Event payloads are versioned and should be
sufficient to reconstruct projected state and recovery handles. `node_output`
records display text plus `outputs[portId]` payloads. A `FormationOutputPayload`
may carry short inline `text`, an artifact `ref`, `reportRef`, and `artifactRef`.
Short inline `text` preserves the existing routing path. For tmux-backed long
outputs, local file `ref` values are canonicalized under configured roots and
hydrated into routed text before the payload is written to the ledger; invalid
refs block instead of becoming arbitrary file reads.

Representative event kinds:

```text
run_started
run_resumed
node_waiting
node_started
slot_dispatch
slot_result
node_output
gate_evaluating
gate_verdict
verification_verdict
artifact_attached
escalation_raised
human_input_requested
human_verdict_recorded
error
run_blocked
run_canceled
run_failed
run_succeeded
```

### Redacted-run evidence and recovery

`Redact=true` distinguishes an execution-authoritative value from durable run
evidence. A fresh attempt may hold and route the authoritative raw value only in
live ephemeral memory; the value is not persisted for replay. Redaction markers,
hashes, display summaries, and safe ref metadata are evidence and never valid
substitutes for a graph input.

The durable redaction boundary covers ledger fields, verifier and per-kind
feedback, captures, reports, artifact contents, output refs and their targets,
derived evidence, and errors. Ref metadata may persist only when its target has
already been sanitized or replaced and resolves inside an authorized root. Raw
executor capture is cleanup-owned transient material, not accepted evidence.
Before any persistent path can contain raw bytes, a durable pending-redaction
record is written and fsynced with ownership of that exact target. Its internal
cleanup locator is the only metadata allowed to point temporarily at an
unsanitized target and is never exposed as an output ref or graph input.
Recovery replaces each target idempotently and preserves valid redacted evidence
byte-for-byte. `run_succeeded` is invalid while any run-owned target remains
pending.

Recovery may reattach the exact unresolved attempt when the qualified session
target is proven and no prompt is resent. It may also finish pending cleanup or
reconcile work that needs no discarded raw value. A pending cleanup target or
other durable evidence cannot reconstruct graph input. If a new dispatch
requires an authoritative value that redaction discarded, the next terminal
event is:

```text
type: run_failed
data.code: redacted_input_unavailable
data.reason: redacted_input_unavailable
data.unrecoverable: true
data.relatedSeq: <source event sequence whose raw value was required>
data.final: true
```

That run cannot resume and cannot increment its epoch. A retry creates a new run
with newly supplied authoritative input. See ADR-0005 for the decision and
enforcement boundary.

## Revision and concurrency

- APIs should expose revision/etag values for mutable resources.
- Clients must send the revision they edited from.
- Conflicting writes fail loudly and require reload/merge.
- The shared writer owns normalization and validation.

## Id rules

- Ids are stable, lowercase, and safe for filenames or explicit encoded paths.
- Display names are not ids.
- Generated ids should include enough entropy to avoid collisions without hiding
  their noun type.
- Existing ids should not change during rename operations.
