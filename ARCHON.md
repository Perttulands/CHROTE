---
type: spec
status: active
authority: source-of-truth
workspace: chrote
enforced_by: scripts/doc-lint.py
---

# Archon CLI Spec

Status: **Active core source of truth**.

`archon` is the command surface for CHROTE Formations. It is how humans, agents,
scripts, and higher-level coordinators author, inspect, run, recover, and decide
against the same model the dashboard renders.

## Purpose

The CLI exists so Formations is not trapped in the browser. Every important UI
operation should have an equivalent or composable `archon` operation, and every
`archon` mutation should be visible in CHROTE without semantic drift.

## CLI invariants

Items 1-8 describe current constraints. Items 9-11 are ADR-0006 accepted-target
constraints and must remain labeled target until their implementation gates
close.

1. Use the shared Go formations package for validation, writes, and run behavior.
2. Emit JSON when `--json` is requested.
3. Fail loud on missing boards, agents, sessions, ports, gates, cwd, checks, or
   ambiguous harness resolution.
4. Do not mutate browser-only state.
5. Do not silently invent agents or sessions.
6. Keep ids stable and visible.
7. Prefer exact commands over conversational interpretation.
8. Never turn free-text criteria into implicit shell commands. Legacy Gate
   command flags remain parseable compatibility inputs, but every authored
   command field is rejected before mutation or execution.
9. Reject a second producer for any input port. Joins use distinct stable
   required ports and do not imply a merge order.
10. Preserve the evaluated exact authorized payload/provenance on Gate pass and
    keep verdict or feedback separate from work output. One failed Gate sequence creates one
    stable feedback object; pushback is a route action to an exact source
    attempt, not another verdict.
11. Gates remain pure evaluators. Gate-owned argv/shell process execution is
    retired. Deterministic process-backed transformation belongs to a future
    Tool step using a host-owned profile whose exact version/hash, parameters,
    policy, and content-addressed execution bundle are frozen for the run.

## Current command groups

```text
archon agent      list | inspect | new | edit | spawn | attach | retire
archon board      new | list | inspect | validate | arrange
archon formation  create | list | inspect | assign | unassign | set-brief | remove-verification | add-input | add-output | wire | unwire | run
archon gate       create | update | judge | approve | reject
archon mission    create | list | inspect | wire | run
archon tool       create | update | delete | inspect
archon run        list | status | logs | follow | resume | abort | ask
```

This list describes the current binary. `board export`, Gate inspection/routing
helpers, `verify`, and `doctor` remain directional command ideas, not available
verbs.

`archon board arrange` is the explicit whole-board layout mutation and uses the
same server-owned deterministic operation as the UI Arrange action. No
authoring, validation, inspection, save, runtime, replay, or reconnect command
may rearrange existing coordinates. When neither coordinate is supplied,
current Formation, Gate, and Mission create verbs use one shared, bounded
free-space grid heuristic to place only the new node; explicit `--x` or `--y`
keeps the supplied coordinates exact. Tool create accepts either an exact
`--x`/`--y` pair or exact predecessor/successor node-id hints for
connection-aware free-space placement. That heuristic writes only the new Tool;
it never moves or reflows an existing node.

Current Archon keeps schema-1 definition authoring and inspection for Tool-free
boards. The first Tool create is the only schema-1-to-2 definition-authoring
migration; Tool update/delete require and preserve schema 2. Local schema-1
`run list`, `status`, `logs`, `follow`, and `ask` remain inspection-only.
Runtime mutation verbs (`mission run`, `formation run`, `gate approve`,
`gate reject`, `run resume`, and `run abort`) are deliberately
non-authorizing: they fail at the runtime-authority boundary before board or run
reads, artifact writes, dispatch, or tmux effects. They do not fall back to the
local engine or private schema-2 ledgers.

Schema-1 inline Formation verification is retired by ADR-0008. Archon inspection
keeps legacy blocks legible, but authoring, run start, resume, and verdict entry
fail `legacy_inline_verification_requires_migration` before artifacts or work.
After an author explicitly creates a replacement Gate and wires a named output
of the Formation to it,
`formation remove-verification <board> <formation> --replacement-gate <gate>`
is the compatibility-only removal operation. The named Gate must already exist
and have that input connection; Archon never converts, creates or rewires the
legacy block automatically. Historical schema-1 runs remain inspectable through
the read-only run commands. Current Archon cannot resume or terminate them;
those verbs fail at the same non-authorizing runtime boundary.

ADR-0007 accepts a different runtime boundary. Archon continues to author,
validate, and inspect workflow definitions offline through the shared package.
Run start, resume, cancel, verdict, list, status, logs, and follow instead use the
fenced CHROTE coordinator and its sanitized projection; coordinator unavailability
fails loud with no local engine or private-ledger fallback. Runtime mutations
carry a stable `commandId` matching
`^cmd_[0-7][0-9A-HJKMNP-TV-Z]{25}$` and a stored canonical payload/hash. Archon validates that id before
transport/path use and canonicalizes resume modes to `reattach` or
`retry-failed-producer`. The same id/hash returns
the original durable receipt, while another payload conflicts without effect.
One command record holds the request and becomes the closed terminal receipt;
its immutable effect/decision fence remains distinct from any later takeover
publication fence, and a terminal start receipt binds the exact immutable
admission-policy revision/hash used. There is no second actor or receipt file.
Start returns an admitted run id after `run_started` fsync and before long-running
execution finishes.

Schema-2 runtime remains disabled until the non-authorizing registry/bootstrap/
workspace-authority guard, complete safe projection/coordinator capability set, and guarded rollback
inventory are certified. A matching schema number does not authorize Archon to
fall back or treat a private ledger as readable runtime state.

Canonical target spelling is `archon run cancel`. Current `archon run abort`, and
any compatibility `stop` spelling, normalize to `cancel` before command hashing
and cannot create another request snapshot, event, or lifecycle state. Exact
target flags and transport wiring land with `ctx-ug7.6`; this current-command
inventory remains unchanged until then.

ADR-0006 accepts agent-first authoring for a mixed Mission → Formation → Tool →
Gate workflow. Current Archon provides the narrow non-executing definition
surface:

```text
archon tool create <board> --profile-id <id> --profile-version <version> --title <title> --params-json '<object>' [--x n --y n | [--predecessor-node-id <id>] [--successor-node-id <id>]] [--updated-by <actor>] [--json]
archon tool update <board> <tool> [--title <title>] [--params-json '<complete object>'] [--updated-by <actor>] [--json]
archon tool delete <board> <tool> [--updated-by <actor>] [--json]
archon tool inspect <board> <tool> [--json]
```

Board selectors accept a stable id or slug. Tool selectors accept a stable id or
an unambiguous exact/slugged title. Tool graph validation, wiring, unwiring, and
whole-board layout continue through the existing `board validate`,
`formation wire`, `formation unwire`, and `board arrange` verbs; there is no
separate `tool wire`, `tool arrange`, or `tool run`. Archon reads the current
board/layout identity immediately before a Tool mutation and supplies the
shared revision/ETag CAS internally; it exposes no public CAS-bypass flag. The
closed initial profile catalog contains only `json.normalize@1`, and
`--params-json` accepts one duplicate-free object whose values are strings,
booleans, or signed 64-bit integers.

Starting a Mission whose selected graph reaches a valid Tool fails with
`tool_execution_unavailable` before snapshot, binding, ledger, dispatch,
process, tmux, or artifact mutation. There is no isolated Tool-run endpoint;
isolated Formation runs remain singleton roots and do not traverse downstream
Tools. The authoritative Mission Store evaluates revision/ETag CAS first, so a
stale caller still receives the existing conflict rather than the Tool
sentinel. Certified host-private implementation packaging and execution remain
owned by `ctx-ug7.8`; `ctx-ug7.11` and `ctx-ug7.13` own the broader
agent-authoring UX. This command group grants no Tool execution authority.

Legacy `command`, `commandArgv`, `commandShell`, and `commandCwd` Gate fields are
schema-1 inspection and migration-plan inputs only, including authored empty
values. Gate-owned process execution is retired. Board validation reports
`legacy_script_gate_requires_fenced_migration`; a selected Mission start or
resume reports the same error before any run mutation when its executable root
contains such a Gate. Unreachable Gates and Gates outside an isolated Formation
root remain board-validation errors but do not block that selected run.

Before Tool profiles land, the migration projection is deliberately
non-authorizing: `ready=false`, `applySupported=false`, and no raw command value,
resolved executable/cwd, generated Tool id, or inferred profile appears in it.
`ctx-ug7.8.1` owns non-executing Tool definitions, registry descriptors, and
board authoring. `ctx-ug7.8` owns certified host-private implementations and
runtime execution. `ctx-ug7.30` owns pure code-Gate profiles and the later
explicit Tool-plus-pure-Gate apply path; this spec does not claim any of those
command surfaces ship.

## Output modes

- Human text for direct operator use.
- JSON for agents and scripts.
- NDJSON for run-follow streams.
- Non-zero exit codes for failures.

## Agent/persona resolution

Current main stores stable agent ids and harness/session-stem compatibility
evidence, then re-reads mutable persona data and resolves again at dispatch. It
does not yet consume a frozen exact binding.

In the accepted target, assignment is staffing intent, not runnable proof.
Before run start, each declared slot in the selected `runRoot` executable
subgraph has a resolution state: unresolved, runnable, ambiguous, or unavailable.
Production resolution calls the same Terminal-session resolver and configured
inventory as cockpit Terminal tabs; the inventory may contain several explicit
user/socket sources, but Archon never invents a Formations-only source. Reusing
accumulated session context is intentional. The same persona stem in more than
one source is ambiguous. A matching candidate is runnable only when unleased,
unattached, and certified closed/ready for the exact fingerprint through the
harness adapter's non-pane channel. Active work reports
`session_target_harness_busy`; missing/ambiguous proof reports
`session_target_readiness_unknown`; quiet pane text is never readiness, and
incomplete client/input monitoring is
`session_target_attachment_audit_unavailable`. All,
and any connected hidden CHROTE Terminal iframe, are unavailable and fail
loudly rather than being detached, stolen, or replaced. A user may explicitly
disconnect a CHROTE-owned presentation client before retrying; binding itself
never does so.
Stock tmux on an owner-accessible raw socket is therefore unavailable until
`ctx-ug7.21` selects, `ctx-ug7.22` implements, and `ctx-ug7.23` certifies an
enforceable same-session-pool boundary; an Archon/CHROTE mutex alone is not
certification.
Only a runnable resolution becomes one host-private immutable `RunSlotBinding`
plus a hash-linked safe projection with server-issued `sessionTargetId`. The
private record freezes persona-card hash, exact tmux server/session/window/pane,
cwd/root, harness/process-start identity, and target fingerprint. Immediately
before send the acquired lease must still exact-match that fingerprint. After
start, projected binding health is
runnable, unavailable, or stale without changing that identity. A multi-slot
attempt can expose several targets. Archon inspection and terminal Peek must use
the target for the exact slot dispatch, never a fresh same-name lookup. A run
never rebinds a slot to a different pane; replacement requires a new run.
Each dispatch records a journal-drained one-send input barrier, its bound ready
proof, and the closed `tmux-pane-history-baseline-v1` token/hash before send.
The baseline binds the exact pane fingerprint, capture-continuity
epoch, byte offset, and frozen terminal grid without pane bytes. Trim/reset,
resize/reflow, restart without proven continuity, or ambiguity fails closed; the
safe projection exposes only encoding/hash/validation state. An authorized Peek
is a full interactive user attach and may send literal control input to steer or
interrupt the exact agent. Only an exact CHROTE-issued capability after durable
target occupancy may send. Its input generation is durable before forwarding,
must close before result acceptance, and is bound into non-pane-forgeable turn
closure proof. Archon inspection marks operator influence but exposes no raw
keystrokes. Ordinary Terminal attachment is denied while occupied; a certified
audit covers client, resize/reflow, history, pane-lifecycle/topology/other
mutation, and input-capable tmux command/control routes, so any foreign
attachment/mutation/input or lost continuity revokes Peek and fails closed
without a result. Peek attach metadata is durable before attach.
Only the latest fsynced issuance is valid; a newer issuance drains prior clients
and invalidates every older token/generation before exposure.
Result/cancel/failure closure drains input, closes steering, and durably revokes
the capability. A coordinator interrupt has its own exact no-resend durable
permit; terminal holds are
non-interactive and no run-bound capability survives finality. This does not
grant Archon or the browser a second automatic
dispatch path. Tile movement and resizing are viewport-only while the dispatch
is active; they send no tmux resize.
Current main resolves by agent/harness/session stem and does not yet expose this
exact target or freeze it for dispatch.

Resolution must surface:

- no matching agent;
- no compatible harness;
- multiple compatible live sessions without a disambiguator;
- stale session references;
- unavailable cwd or file root;
- missing permissions or a disabled executor adapter.

## Mutation rules

- Structural mutations update board definitions.
- Layout mutations update layout sidecars.
- New-element creation may add only that element's heuristic coordinates.
  Existing coordinates change through direct user moves or explicit
  `archon board arrange`, never as an implicit side effect of another verb.
- In the ADR-0006/0007 accepted target, run events append through the sole writer
  to host-private ledgers outside generic Files roots; Archon reads sanitized
  projections, never raw authority. Current main still uses workspace run files,
  so that security boundary must land before this target is claimed implemented.
- Undoable operations must record enough inverse intent to reverse the mutation
  without whole-board hacks.
- Definition writes go through the shared package writer so UI/API/CLI cannot
  diverge. Runtime command/effect writes go only through the current fenced
  coordinator; a package file lock is not execution authority.

## Retired Script-gate command flags

`archon gate create` and `archon gate update` still recognize the historical
command flags so old automation fails deterministically instead of being
misparsed. They are rejected compatibility inputs, not executable forms:

- `--command-argv npm,run,lint`, `--command-shell 'npm run lint'`, legacy
  `--command 'npm run lint'`, and `--command-cwd dashboard` each fail with
  `legacy_script_gate_requires_fenced_migration` before board or layout mutation.
- An explicit empty value still counts as authored legacy field presence.
- Mutually inconsistent legacy modes remain invalid; Archon never tokenizes a
  legacy command, parses a shell string into argv, resolves an executable/cwd,
  or infers a host profile.
- `archon board inspect --json` remains the authorized source-definition view.
  `archon board validate --json` exposes the non-mutating migration inspection,
  including source field names and affected edge ids but no raw values.

The intended future shape is a host-profile Tool whose declared output feeds a
pure code Gate. The Gate evaluates and passes through that exact Tool output, not
the pre-Tool payload. Future migration must therefore validate the explicit
profile, parameter, port, media, and downstream mapping or leave the legacy
board byte-identical and fail loud.

## Examples

```bash
archon board list --json
archon agent list --assignable --json
archon formation create default peer --title "review pair" --json
archon formation assign default fmn_review_pair --slot slot_reviewer --agent codex-reviewer --json
archon mission wire default mis_home_vdki fmn_review_pair:port_review_in --json
archon formation wire default fmn_review_pair:port_review_out gate_final_review:in --json
archon gate create default --kinds human --criterion "review passes" --json
archon mission run default --mission mis_home_vdki --json
archon run status run_20260605_001 --json
archon gate approve run_20260605_001 gate_final_review --reason "passes owner check" --json
```

## Relationship to dashboard

The dashboard is the spatial cockpit. `archon` is the exact command surface.
Neither is secondary; both are projections over the same file-backed system.
