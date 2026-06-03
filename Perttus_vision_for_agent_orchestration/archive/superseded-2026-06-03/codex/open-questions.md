# Open Questions Before Implementation

Status: decision checklist

Recommended defaults are included so implementation can proceed if Perttu accepts them.

## Architecture

1. Should v0 be CHROTE-native or Gas City-backed?
   - Recommended: CHROTE-native file/CLI/run-ledger contract first; Gas City only as later adapter or reference.

2. Where should canonical files live?
   - Recommended: `.chrote/orchestration/` under the active workspace.

3. Should Context Citadel store orchestration state?
   - Recommended: no. Use Citadel for context retrieval/contribution, not mission/run storage.

4. Should Beads store missions?
   - Recommended: no. Link missions to Beads; use Beads for work items and blockers.

5. Should a notice board exist in v0?
   - Recommended: yes, but only as file-backed JSONL messages under `.chrote/orchestration/notices/`.

## Data Model

6. Does "formation" mean a concrete mission node, a reusable template, or both?
   - Recommended: v0 formation means concrete mission node. Add `formation_template` later if real reuse appears.

7. Should v0 use JSON, YAML, or TOML?
   - Recommended: JSON for v0 because Go supports it natively and agents can generate it reliably.

8. How strict should schema validation be?
   - Recommended: strict for IDs, ports, node references, root paths, and enum values; tolerant for unknown fields so agent-authored detail survives UI edits.

9. Where do outputs, reports, diffs, and artifacts live?
   - Recommended: run directories, not mission JSON. Keep graph definitions small.

10. How are transcripts retained?
    - Recommended: do not promise transcript storage in v0. Link to existing tmux capture where available and design retention before saving full transcripts.

## Agent Registry

11. How are durable agents declared?
    - Recommended: `agents/*.json` profile files plus live tmux/Oracle session matching.

12. Does persistent identity require a live session?
    - Recommended: no. Profiles are durable; sessions are live bindings.

13. How should agents discover capabilities?
    - Recommended: progressive disclosure: list IDs/tags first, inspect profile/source files only when needed.

14. Are Archon and team leaders special runtime types?
    - Recommended: no special scheduler behavior in v0. They are agent profiles and formation slots with stronger prompts/responsibilities.

## CLI And API

15. CLI name?
    - Recommended: `chrotectl` unless a broader `chrote` CLI exists by implementation time.

16. Should browser edits be allowed in v0?
    - Recommended: read-only first. Later allow small patch operations with revision checks.

17. Should the API start runs?
    - Recommended: not in Phase 2. Add run start/cancel behind backend mutating mode after the run ledger exists.

18. Should there be arbitrary command execution for gates?
    - Recommended: no generic command endpoint. Code gates must use explicit configured commands in mission files and a server-side allow/safety model.

19. Should agents be able to nudge other agents automatically?
    - Recommended: not in v0. Add explicit audited nudge only after adapter and audit boundaries are proven.

## UI

20. Should the Formations tab default on?
    - Recommended: no. Default off behind `chrote-formations-tab`.

21. Should the tab copy the prototype visuals directly?
    - Recommended: preserve the concepts, not the CSS wholesale. Real CHROTE UI should stay dense, operational, and consistent with current dashboard tabs.

22. Should the canvas be the main authoring tool?
    - Recommended: no. It is inspection plus light-tweak only.

23. Which actions are allowed from the tab first?
    - Recommended: inspect mission, inspect node, open terminal/session, open Bead, open artifact, validate. Add start/cancel/verdict later.

## Safety And Rollout

24. What flags are required?
    - Recommended: frontend flags for tab/control visibility; backend env mode for API/mutation behavior.

25. Should backend orchestration be available when the UI flag is off?
    - Recommended: read-only API can exist when server mode is read-only; mutating API must require server mode plus UI/action flag.

26. Should implementation wait for current CHROTE security beads?
    - Recommended: avoid Beads mailbox and broad mutation until Beads argv confinement is fixed. Do not mix this work with current safety-spine fixes.

27. What deployment rule protects live sessions?
    - Recommended: build first, no service restart until scheduled, smoke after restart, never kill tmux sessions.

28. What is the first real mission to test?
    - Recommended: a small plan-review-synthesis mission linked to an existing Bead, with one planner, two reviewers, and a synthesis formation.

## Product Boundary

29. Is the main outcome "watch agents work"?
    - Recommended: no. The outcome is access to agent teams through coherent agent touchpoints. The tab exists to inspect and recover state.

30. What should cause interruption?
    - Recommended: blocked work, decision needing taste/judgment, team disagreement, architectural drift, cost/risk concerns, surprising opportunity, better direction found, or a leader deciding work should stop.
