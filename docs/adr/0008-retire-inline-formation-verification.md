# ADR-0008: Retire Inline Formation Verification

## Status

Accepted 2026-07-18

## Context

Schema-1 Formations can embed `[formation.verification]` inside a Formation.
The evaluator receives mutable result text and may append
`verification_verdict`, but that record does not identify the exact Formation
attempt, named output or safe ref/hash it evaluated. It also has no unique
closer for block or pushback and no replay-safe identity for opening the next
attempt.

Keeping this hidden check would create a second verification lifecycle beside
the explicit Gate node. The canvas could not explain which output was checked,
which evaluator decided, or which route and revision the decision authorized.

## Decision

Inline Formation verification is retired. Explicit Gate nodes are the only
workflow verification primitive.

- Schema-1 `[formation.verification]` remains parseable and visible for
  compatibility inspection. Reading it never rewrites the board.
- Validation, Mission start, isolated Formation start and resume fail
  `legacy_inline_verification_requires_migration` before a snapshot, binding,
  ledger event, evaluator, dispatch or revision attempt is created.
- New inline-verification definitions and new `verification_verdict` events are
  rejected. The injected `VerificationEvaluator` execution path is removed.
- Historical schema-1 `verification_verdict` records remain readable evidence.
  They never route work, resume a run or open another attempt.
- Migration is explicit. The operator or author creates a replacement Gate and
  wires a named output of the Formation to that Gate's input. Only then may the
  author remove the legacy block, and the removal request must name that
  existing Gate as `replacementGateId`. CHROTE verifies the existing
  Formation-output-to-Gate connection but never guesses an input, creates a
  Gate, rewires outputs or translates `pushback` automatically.
- The Formations UI shows the legacy kinds, criterion and failure policy with
  migration guidance. It offers no Add, Configure or Save action. Archon exposes
  the same data through inspection and provides the explicit
  `formation remove-verification --replacement-gate <gate>` compatibility
  command.

Retirement adds no execution, closer or revision authority, so it does not
require an ADR-0007 authority-schema bump.

## Enforcement

- The shared writer accepts compatibility removal only when `formationId` and
  `replacementGateId` resolve on the same current board and an existing
  connection runs from one named output of that Formation to the replacement
  Gate's input. A missing, stale, wrong-kind or unwired replacement fails
  `legacy_inline_verification_requires_migration` without changing board bytes,
  revision, layout, Gate definitions or connections.
- `PATCH /api/formations/boards/{board}` carries
  `removeVerification: {formationId, replacementGateId}`. Archon requires
  `formation remove-verification <board> <formation> --replacement-gate <gate>`.
  Both use the shared writer and the same ETag/revision preconditions and stable
  failure code.
- The UI keeps legacy fields read-only. Its removal action requires the user to
  choose an eligible already-wired replacement Gate and submits that exact Gate
  id; it does not close the migration view as successful until the shared write
  succeeds.
- Validation, Mission start, isolated Formation start, resume and human verdict
  entry continue to reject a snapshot containing inline verification before
  run artifacts, ledger mutation, evaluator work, dispatch or revision. New
  `verification_verdict` appends remain rejected, while historical verdicts
  remain readable and non-authorizing.
- Fail-closed retirement does not strand historical runs. Inspection and
  terminal containment remain available: cancellation or failure may append
  the normal terminal `run_canceled` or `run_failed` event without evaluating,
  routing, resuming or dispatching legacy inline verification.
- Tests cover the shared writer plus API, Archon and UI parity; exact no-mutation
  rejection; no inferred Gate or rewiring; start/resume/verdict rejection;
  historical ledger projection; and terminal cancellation/failure. Go, race,
  dashboard, browser, doc-lint, diff and independent exact-candidate review
  remain certification gates.

## Alternatives Considered

- **Retain inline verification with exact attempt/output/evaluator identity.**
  Rejected. It duplicates Gate lifecycle, recovery, redaction and projection
  semantics while keeping the check visually subordinate to a Formation.
- **Automatically convert it to a Gate.** Rejected. The legacy shape does not
  say which named output to evaluate, which judge/profile to bind, how to map a
  multi-output Formation, or how `pushback` should wire typed feedback.
- **Keep executing schema-1 behavior until schema 2 ships.** Rejected. A known
  replay-ambiguous authority path should fail before durable work, not remain a
  temporary exception.

## Consequences

Old boards and ledgers stay legible, but a board containing inline verification
cannot run until its author explicitly models the check as a Gate, wires a
Formation output to it and names that Gate while removing the legacy block.
This is intentionally fail-loud. Historical runs may still be canceled or
failed without reopening execution. The result is one visible, testable
verification model across files, Archon, API, runtime and canvas.

Implementation and certification are owned by `ctx-ug7.17`. ADR-0006 remains
the node and Gate contract; ADR-0007 remains the execution-authority contract.
