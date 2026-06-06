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
archon formation  create | inspect | assign | unassign | wire | unwire | add-input | add-output | set-brief | run
archon mission    create | inspect | list | run
archon gate       create | inspect | judge | approve | reject | route
archon verify     add | config | remove | run
archon run        list | status | logs | follow | resume | abort
archon doctor     env | files | sessions | checks
```

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
archon gate approve run_20260605_001 gate_final_review --reason "passes owner check" --json
```

## Relationship to dashboard

The dashboard is the spatial cockpit. `archon` is the exact command surface.
Neither is secondary; both are projections over the same file-backed system.
