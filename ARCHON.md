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

1. Use the shared Go formations package for validation, writes, and run behavior.
2. Emit JSON when `--json` is requested.
3. Fail loud on missing boards, agents, sessions, ports, gates, cwd, checks, or
   ambiguous harness resolution.
4. Do not mutate browser-only state.
5. Do not silently invent agents or sessions.
6. Keep ids stable and visible.
7. Prefer exact commands over conversational interpretation.
8. Never turn free-text criteria into implicit shell commands.

## Command groups

```text
archon agent      list | inspect | new | edit | spawn | attach | retire
archon board      list | inspect | new | validate | export
archon formation  create | list | inspect | assign | unassign | wire | unwire | add-input | add-output | set-brief | run
archon mission    create | list | inspect | set-goal | wire | run
archon gate       create | inspect | judge | approve | reject | route
archon run        list | status | logs | follow | resume | abort | ask
archon doctor     env | files | sessions | checks
```

## Command notes

These notes pin the verbs whose behavior is not obvious from the name. Every
verb above maps to a real handler; `scripts/doc-lint.py` fails the build if this
group and the CLI dispatch drift apart.

- `board new <slug> --title <t> --goal <g> [--bead <id>]` creates a **Mission
  Board**: the board and its single mission in one atomic write. A board's
  identity is its mission, so the goal is required at create time and there is
  never an empty board; edit the mission later with `mission set-goal`.
- `board validate` runs a read-only structural integrity check and exits non-zero
  when the board has blocking errors. The `mission_count` finding fires when a
  board does not have exactly one mission (the Mission Board invariant).
  `board export` emits the canonical board definition plus its layout sidecar as
  JSON.
- `formation list` lists every formation on a board with its type, slot, and
  assigned-agent counts. `formation inspect` reports one formation's ports,
  slots, assignments, and the connections touching it.
- `mission list` and `mission inspect` are mission-level: `inspect` walks the
  reachable run chain from a mission. `mission set-goal` edits one mission's
  title/goal/bead in place. `mission wire` connects a mission's `out` port to a
  downstream node input (it is the mission-rooted form of `formation wire`).
- `gate inspect` reports a gate's authoring shape (kind, judge chain, script
  config, pass/fail wiring) without touching a run. `gate route` records a
  run-time verdict and is the canonical routing verb; `approve`/`reject` are
  conveniences for `--verdict pass`/`--verdict fail`. The verdict must be exactly
  `pass` or `fail` (see FORMATIONS.md's strict verdict contract).
- `gate judge <board> <gate> --chain f1,f2` attaches an in-board judge formation
  chain to a gate; `gate judge <board> <gate> --detach` removes the attached
  judge chain so the gate falls back to its script or human decision.
- `run ask <runId> [question]` answers a question about a run strictly from its
  durable ledger evidence (completed nodes, produced outputs, waiting gates,
  blocks, escalations). It never invents state; when evidence is missing it says
  so under `missingEvidence`.
- `doctor` is a read-only operator diagnostics surface. It never sends keys,
  spawns, kills, or mutates anything.
  - `doctor env` reports the resolved workspace and allowed roots, and which
    Formations executor the env ladder would select (`lab`,
    `tmux-dedicated-required`, `dedicated-tmux`, or `unavailable`) from the
    `CHROTE_FORMATIONS_LAB_*` / `CHROTE_FORMATIONS_TMUX_*` variables.
    `dedicated-tmux` is selected when `CHROTE_FORMATIONS_TMUX_DEDICATED=1`
    points execution at the dedicated formations socket; without that opt-in,
    tmux execution is reported as dedicated-required. The cockpit socket is
    never an execution target.
  - `doctor files` reports `.formations` health: the boards/layout/runs dirs, a
    board count, per-board `ValidateBoard` error/warning counts, and any
    unreadable or corrupt board file named loudly.
  - `doctor sessions` lists live tmux sessions on the configured socket (the same
    resolution `agent list` uses), each session's pane cwd, and whether that cwd
    is inside an allowed root. It is list/read only.
  - `doctor checks` reports executable-prerequisite readiness: tmux on PATH, the
    tmux socket path, workspace present and writable, and required dirs.
  - A bare `doctor` runs all four sections. doctor exits non-zero when it finds a
    HARD problem (missing/unwritable workspace, unreadable board, or a board with
    validation errors); warnings alone exit zero.

## Output modes

- Human text for direct operator use.
- JSON for agents and scripts.
- NDJSON for run-follow streams.
- Non-zero exit codes for failures.

## Agent/persona resolution

Slot assignment stores stable agent ids. Run dispatch resolves those ids to a
selected harness/session at run time.

Resolution must surface:

- no matching agent;
- no compatible harness;
- multiple compatible live sessions without a disambiguator;
- stale session references;
- unavailable cwd or file root;
- missing permissions or feature flags.

## Mutation rules

- Structural mutations update board definitions.
- Layout mutations update layout sidecars.
- Run events append to run ledgers.
- Undoable operations must record enough inverse intent to reverse the mutation
  without whole-board hacks.
- All writes go through the shared writer so UI/API/CLI cannot diverge.

## Examples

```bash
archon board list --json
archon agent list --assignable --json
archon formation create default peer --title "review pair" --json
archon formation assign default fmn_review_pair --slot slot_reviewer --agent codex-reviewer --json
archon mission wire default mis_home_vdki fmn_review_pair:port_review_in --json
archon formation wire default fmn_review_pair:port_review_out gate_final_review:in --json
archon mission run default --mission mis_home_vdki --json
archon run status run_20260605_001 --json
archon gate route run_20260605_001 gate_final_review --verdict pass --reason "passes owner check" --json
archon doctor --json
```

## Relationship to dashboard

The dashboard is the spatial cockpit. `archon` is the exact command surface.
Neither is secondary; both are projections over the same file-backed system.
