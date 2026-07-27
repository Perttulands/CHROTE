# ADR-0012: The Gate-Fail Pushback Edge Is the Retry Mechanism

## Status

Accepted 2026-07-27 (owner ruling, Model A). Supersedes FORMATIONS.md's
separate-correction-node pushback model and the `retry_control` port role it
required; amends invariant 11's one-producer rule with the pushback exception.

## Context

Two pushback models competed, and the spec described the one that was never
built:

- **Correction-node model (spec'd, unimplemented):** a separate correction node
  with distinct work and feedback ports; the Gate's fail frontier had to be the
  single edge into its `retry_control` input. No supporting code exists:
  `FormationPort` has no role field, `retry_control` appears in non-test source
  nowhere, and the Tool schema layer actively rejects `role = "retry_control"`.
- **Direct pushback edge (shipped):** the Gate's `fail` edge wired straight
  back into the work input of the Formation whose output the Gate evaluated.
  The run engine implements it and
  `TestS5HumanGateFailPushbackResumeReDispatchesWork` proves the loop: fail
  verdict → resumable block → resume → bounded re-dispatch of the evaluated
  producer.

The authoring layer, however, enforced a blind one-connection-per-input rule at
three sites (wire mutation, connection-candidate validation, and board
inspection's duplicate-producer scan), so the topology the engine runs could
only be produced by hand-editing TOML — the defect chrt-00kt records. Proving
mission (a) — a code change looping through lint and review gates until green —
is unauthorable without resolving this.

## Decision

1. The typed gate-fail pushback edge is *the* pushback mechanism: a connection
   from a Gate's `fail` port into the work input of the Formation whose output
   that Gate evaluates. It delivers the typed `gate_feedback` object, which
   triggers the bounded in-graph next attempt (FORMATIONS.md invariant 12).
   Feedback never masquerades as work.
2. Invariant 11 is amended: every input port accepts at most one incoming
   **non-pushback** edge. Gate-fail edges are exempt from the one-producer
   conflict at all three enforcement sites, in either authoring order. A second
   non-pushback producer into an occupied input remains a typed conflict.
   Multiple gates may push back into the same work input (mission (a) needs a
   lint gate and a review gate feeding one worker).
3. The separate-correction-node model and the `retry_control` port role are
   rejected. `retry_control` remains invalid port vocabulary; reintroducing a
   correction-node mechanism requires a new ADR, not a revival of the old text.
4. Acyclicity is defined net of pushback: after removing validated gate-fail
   pushback edges, the workflow-channel graph must be acyclic — the pushback
   edge is the one sanctioned cycle.

## Consequences

- FORMATIONS.md amended in the same change (invariants 11/12, payload-kind
  vocabulary, the fail-edge-into-work paragraph, the pushback definition, and
  the isolated-root scope list).
- The three authoring enforcement sites gain the pushback exemption with tests
  covering the authorable loop, the still-rejected double producer, and board
  validation of the pushback board; the engine's pushback semantics are
  untouched.
- The S5 pushback topology becomes authorable end-to-end via the archon CLI
  with no TOML surgery, unblocking chrote-3af mission (a).
- Authoring does not verify at wire time that the pushback target is the
  evaluated producer's input — that relationship is normative (this ADR,
  invariant 12) and enforced by the run engine's traversal rules, not by a new
  authoring-time graph analysis.
