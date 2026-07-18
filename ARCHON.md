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
8. Never turn free-text criteria into implicit shell commands. Script gates use
   `--command-argv` by default; `--command-shell` is the explicit shell opt-in.
9. Reject a second producer for any input port. Joins use distinct stable
   required ports and do not imply a merge order.
10. Preserve the evaluated exact authorized payload/provenance on Gate pass and
    keep verdict or feedback separate from work output. One failed Gate sequence creates one
    stable feedback object; pushback is a route action to an exact source
    attempt, not another verdict.
11. Treat Script Gates as evaluators. Deterministic Tool transformation uses a
    pure host-owned profile whose exact version/hash, parameters, policy, and
    content-addressed execution bundle are frozen for the run; it is not an
    inline board command.

## Current command groups

```text
archon agent      list | inspect | new | edit | spawn | attach | retire
archon board      new | list | inspect | validate | arrange
archon formation  create | list | inspect | assign | unassign | set-brief | add-input | add-output | wire | unwire | run
archon gate       create | update | judge | approve | reject
archon mission    create | list | inspect | wire | run
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
keeps the supplied coordinates exact. Connection-aware neighbor placement
remains target behavior and is not yet implemented.

Current main also executes `mission run`, `formation run`, and `run resume`
synchronously in an Archon-local engine, reads workspace run files directly,
and implements `run abort` by appending final cancellation. That is compatibility
behavior, not the accepted schema-2 ownership contract.

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
Gate workflow. There is no `archon tool` group today. Tool profile authoring,
validation, packaging, and the exact command spelling belong to `ctx-ug7.8`,
`ctx-ug7.11`, and `ctx-ug7.13`; this spec must not imply they already ship.
Likewise, current argv/shell Script Gates are schema-1 compatibility behavior.
Schema-2 code Gates use frozen pure in-process evaluator profiles; migrating a
command-backed Gate fails loud until the process-fence or retirement decision in
`ctx-ug7.16` lands.

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

## Script-gate command flags

`archon gate create` and `archon gate update` use the same command-mode contract
as the board model:

- `--command-argv npm,run,lint` is the default executable form. CHROTE passes the
  argv literally and does not insert a shell.
- `--command-cwd dashboard` optionally sets the command working directory. It
  must resolve under the Formations workspace. This is a cwd guard, not a
  filesystem sandbox.
- `--command-shell 'npm run lint'` is the explicit shell opt-in.
- `--command 'npm run lint'` stores legacy compatibility metadata only; script
  gates do not execute legacy command strings.
- `--command-argv`, `--command-shell`, and legacy `--command` are mutually
  exclusive.

Script Gates return verdict, reason, and evidence metadata only. They do not
emit transformed workflow output and must not be used as a substitute for the
future host-profile Tool step.

## Examples

```bash
archon board list --json
archon agent list --assignable --json
archon formation create default peer --title "review pair" --json
archon formation assign default fmn_review_pair --slot slot_reviewer --agent codex-reviewer --json
archon mission wire default mis_home_vdki fmn_review_pair:port_review_in --json
archon formation wire default fmn_review_pair:port_review_out gate_final_review:in --json
archon gate create default --kinds lint --criterion "lint passes" --command-argv npm,run,lint --command-cwd dashboard --json
archon mission run default --mission mis_home_vdki --json
archon run status run_20260605_001 --json
archon gate approve run_20260605_001 gate_final_review --reason "passes owner check" --json
```

## Relationship to dashboard

The dashboard is the spatial cockpit. `archon` is the exact command surface.
Neither is secondary; both are projections over the same file-backed system.
