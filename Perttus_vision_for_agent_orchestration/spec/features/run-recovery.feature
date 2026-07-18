# Captures recovery + fail-loud behaviors implied by the run model and DECISIONS-LOCKED (ledger is
# canonical; status is projected; recovery replays; binding/sentinel failures are loud, never silent).
# The prototype mocks runs, so these are real-engine requirements the spec must pin.

Feature: Run recovery and fail-loud failure modes
  As the engine and its operators
  I need runs to survive disconnects and to fail loudly, never silently
  So that "what happened?" is always answerable and nothing hangs unnoticed

  Background:
    Given a board "session-search" whose runs write an append-only NDJSON ledger
    And run status is projected from the ledger, not stored separately
    And one CHROTE server coordinator holds the current workspace lease and writer fence
    And unsupported registry, bootstrap, workspace-authority, admission-policy, or ledger schema is strictly read-only before fence acquisition
    And every "run_failed" exact-names one prior unique "run_failure_reconciliation_started" through "failureReconciliationSeq"
    And that start projects non-final "failing", freezes the failure header and complete open-resource snapshots, and permits reconciliation only
    And the final failure byte-matches that header and exactly disposes those snapshots

  # ── Recovery ────────────────────────────────────────────────────────────────

  @cli
  Scenario: A client disconnect does not change run ownership
    Given a run is durably admitted and mid-cascade
    When the CLI or browser disconnects
    Then the same coordinator continues the run independently
    And no owner takeover, replay, redispatch, or cancellation occurs because of the client disconnect

  @file @security
  Scenario: A replacement coordinator acquires a newer fence before recovery
    Given the prior coordinator crashes while admitted runs and private obligations remain
    When a replacement acquires the private workspace lock
    Then it validates the immutable supported bootstrap, mutable workspace authority-schema high-water mark, and complete hash-matched current admission-policy chain to revision 1 before mutation
    And it advances/fsyncs the fence counter before publishing a strictly newer owner fence, allowing gaps but no reuse
    And it reconstructs queued runs by immutable workspace admission sequence
    And it strict-validates every retained policy revision/hash while reconstructing only current admission counts and FIFO from run ledgers
    But it does not claim a workspace-global historical admission order that schema 2 does not persist
    And it reconciles cancellation, cleanup, and recovery before fresh dispatch
    And it continues only when every required execution-authoritative input is available
    And no completed node or turn is re-run
    And the stale prior owner cannot append, send, spawn, interrupt, clean, quarantine, or finalize
    And historical event/obligation origin fences remain valid monotonic prefixes while recovery transitions carry the new state fence

  @file
  Scenario Outline: Command recovery returns one semantic effect
    Given command "<state>" is durable for command id C and hash H
    When the current fenced owner recovers the command journal
    Then it "<outcome>"
    And retrying C/H returns the same durable receipt and run/effect identity
    And no second command effect occurs
    Examples:
      | state                                                   | outcome                                                                  |
      | pending start before run admission                      | resumes bounded admission from stored payload or records rejection       |
      | pending resume before run_resumed                       | rechecks blocked seq and applies once or records stale rejection          |
      | pending cancel before run_cancel_requested              | rechecks run/cancel snapshot and applies once or records stable rejection |
      | pending verdict before human_verdict_recorded           | rechecks requested seq and applies once or records stale rejection        |
      | run_started durable but receipt is missing              | repairs the receipt for the same run and schedules it once                |
      | run_resumed durable but receipt is missing              | repairs the applied receipt without opening another epoch                 |
      | run_cancel_requested durable but receipt is missing     | repairs the applied receipt and resumes cancellation only                 |
      | human_verdict_recorded durable but receipt is missing   | repairs the applied receipt without another verdict                       |

  @file @security
  Scenario: Takeover receipt repair separates outcome and publication fences
    Given a command effect is durable under writer fence F1 while its command record remains pending
    And a replacement coordinator owns current writer fence F2
    When the replacement repairs the terminal applied command record
    Then "stateWriterFence" is F2 and immutable "outcomeWriterFence" is F1
    And the API receipt preserves F1 while no second command effect occurs

  @file @security
  Scenario Outline: Result recovery uses immutable result authority
    Given "<durable>" is durable and "<missing>" is absent after a coordinator crash
    When the current fenced owner replays the run
    Then it "<recovery>" exactly once
    And it neither reparses mutable capture nor redispatches completed work
    Examples:
      | durable                              | missing          | recovery                                                        |
      | successful terminal slot-turn result | formation_result | derives the result from the fixed formation-type rule/envelopes |
      | first non-ok deciding slot-turn result | formation_result | derives the failed/review result and dispatches no later phase  |
      | formation_result or tool_result      | node_output      | materializes the exact hash-bound output                        |
      | node_output                          | routing/finality | continues only the missing delivery and finalization            |

  @cli @ui
  Scenario: Status is identical whether read from CLI or UI after reconnect
    When I reconnect mid-run
    Then "archon run status run_01J9" and the Formations tab show the same projected state
    And both consume the coordinator's sanitized projection
    And neither reads the private ledger or falls back to a local engine

  @cli
  Scenario: Two runs of the same mission have independent ledgers
    When I run the mission twice
    Then each run has its own ledger, run id, and workflow projection
    But the host-wide target arbiter prevents both runs from dispatching concurrently to one pane

  # ── Binding failures (fail loud) ────────────────────────────────────────────

  @cli
  Scenario: Run preflight rejects an agent with no live session
    Given a slot references "scout" and no live session matches
    When I request a run
    Then SlotResolution reports "unavailable" with a stable reason
    And no binding snapshot or "run_started" event is written

  @cli
  Scenario: Run preflight rejects a session whose pane is already dead
    Given "scout" has a session whose process has exited
    When I request a run
    Then SlotResolution reports "unavailable" with reason "pane_dead"
    And no binding snapshot or "run_started" event is written

  @cli
  Scenario: An ambiguous agent id is disambiguated or fails loud
    Given scout's card has "claude-code" sessionStem "claude-scout"
    And scout's card has "openai-codex" sessionStem "codex-scout"
    And both "claude-scout" and "codex-scout" are live for agent "scout"
    Then dispatch resolves by the slot's harness
    But if the slot has no harness and the card default does not resolve uniquely
    Then preflight reports "ambiguous" and writes no binding snapshot or "run_started"

  @cli @security
  Scenario: A frozen target that dies after run start never falls back by name
    Given "run_started" froze scout's exact binding and opaque session target
    And that pane becomes dead or stale before its slot dispatch completes
    When runtime reconciliation checks the binding
    Then "slot_binding_observed" records unavailable or stale for that exact binding/target
    And an "error" with "errorScope=slot" names its node, slot, binding, and target
    And the engine never re-resolves the agent id, session stem, or same-named session
    And no prompt or soft interrupt is sent to an unproven target

  @cli
  Scenario: A proven explicit reattach continues without redispatch
    Given the ledger contains "slot_dispatch" for "slot_peer_a" with no matching "slot_result"
    When the recovery reconciler proves the current workspace fence and replays the ledger
    Then the engine re-attaches capture only if the same unresolved attempt, qualified session target, exact chained interaction journal, and continuous client/input audit are proven live
    And run-bound Peek input remains suspended until recovered capability and monitor state exact-match the current fence
    And it records a loud "error" if capture cannot be re-attached
    And the first block permits only one bounded "reattach_only" resume with the exact same open dispatch
    And while that block is current it rejects late "slot_result", "node_output", and routing
    When I explicitly resume and capture proves the exact original sentinel plus certified closed-turn and client-audit boundaries
    Then that resume sends no prompt and creates no "slot_dispatch"
    And it drains input and fsyncs irreversible Peek capability revocation before result closure
    And it installs the closure mutation/input barrier before appending "slot_result" with the original turn identity, exact audit proof, and hashed immutable turn envelope
    And occupancy becomes a durable "result_committed" release receipt carrying the exact turn-closure proof, audit proof, and "closure_barrier_held" releaseProof
    And no result is consumable before that receipt fsync
    And ordinary graph execution continues with no second block

  @cli
  Scenario: A second unproven reattach blocks without superseding the lease
    Given one unmatched dispatch reached a current "reattach_only" block
    When I explicitly resume its exact open-dispatch set
    And the bounded no-prompt pass still cannot prove its result
    Then no prompt or new "slot_dispatch" is created
    And a second block uses "resumePolicy=new_run_required" with no "nextEpoch"
    And that block rejects late "slot_result", "node_output", and routing until canonical cancel
    And a separately started run is independent and does not close or supersede the old lease
    And no same-run action supersedes the lease or sends the original prompt again

  @cli @security @recovery
  Scenario: Recovery never trusts a stale result-to-receipt barrier
    Given "slot_result" and its exact private audit proof are durable
    But no "result_committed" receipt exists and closure-barrier continuity is lost
    When recovery reconciles the target
    Then the immutable result stays unconsumable in result-closed quarantine
    And recovery does not invent "closure_barrier_held" from matching hashes
    When certified observation proves the exact old pane incarnation is gone
    Then the receipt may use only "post_result_pane_incarnation_gone" releaseProof
    And recovery never reparses, changes, or redispatches the result

  @cli @security
  Scenario: Reattach reconciles all unmatched dispatches and retains only the unresolved subset
    Given dispatch A for node A and dispatch B for node B are both unmatched
    And the bounded automatic capture pass proves neither result
    When recovery reaches quiescence
    Then one "reattach_only" block contains A then B in stable dispatch sequence order
    And it has "blockScope=run" with no blocked node or Gate id
    And the current block rejects late results, outputs, and routing for both dispatches
    When I explicitly resume that exact open-dispatch set
    Then the reattach epoch sends no prompt and creates no new dispatch
    When A's exact sentinel and certified closed-turn/client-audit boundaries are proven but B remains unproven before the bounded pass ends
    Then A's Peek capability is durably revoked with every steering generation closed
    And A's closure barrier remains continuous while its original closed-turn "slot_result" is fsynced and occupancy becomes its durable "result_committed" receipt with exact audit/release proof
    And the next "new_run_required" block contains only B
    And that block derives "blockScope=node" for node B with no "nextEpoch"
    And it cannot add or change a dispatch identity from the preceding set
    And B rejects late result, output, and routing until canonical cancel

  @cli @security
  Scenario: Unmatched-dispatch recovery precedes graph-semantic blockers
    Given one dispatch is unmatched when independent branches also leave an unwired Gate FAIL and a partial JOIN
    When recovery reaches quiescence
    Then the complete-set "reattach_only" block is selected first to preserve open authority
    And no semantic block or retry dispatch is appended
    When exact reattach closes the dispatch and recovery recomputes quiescence
    Then the earliest stable non-resumable Gate/JOIN candidate selects exactly one block
    And every other blocker remains inspectable durable evidence

  @cli @security
  Scenario: A classified root-derived input remains replayable under redaction
    Given a Redact=true run has a Mission-out delivery or isolated Formation seed
    And its durable projection is classified "authored_config" and exact-matches "run_started.rootInputProjection" in source role, encoding, media, hash, and text
    And both exact-match the corresponding private "authoredConfigManifest" entry
    When recovery validates that root-derived input
    Then it may reuse those exact board-authored bytes without treating them as discarded runtime input
    And it rejects a generic unclassified copy or any mismatch before dispatch

  @file @security
  Scenario: Redacted replay fails when execution-authoritative input was discarded
    Given a Redact=true run needs a prior raw node output for its next dispatch
    And durable evidence contains only a redaction marker, hash, summary, or sanitized ref
    When recovery cannot re-attach the exact unresolved attempt
    Then the ledger records "run_failed" with code and reason "redacted_input_unavailable"
    And the event records "unrecoverable=true" and "final=true"
    And "relatedSeq" identifies the source event whose raw value was required but does not select a failed attempt
    And "failureCause" is "kind=none"
    And "nodeAttemptDispositions" exactly closes every node attempt still open at failure, possibly none
    And "slotDispatchDispositions" exactly closes every slot dispatch still open at failure, possibly none
    And "toolLeaseDispositions" exactly closes every Tool lease still open at failure, possibly none
    And the run does not record "run_blocked" or open a new epoch
    And resume is rejected and no evidence value is dispatched as graph input

  @cli @security
  Scenario: Capture cleanup ownership survives a crash and retry
    Given a Redact=true attempt will write raw capture bytes to a persistent path
    And its exact pending-redaction obligation is written and fsynced before capture begins
    When raw capture is written and the process crashes before replacement
    Then a supported current owner validates the obligation's workspace, command, and historical origin fence
    And it claims cleanup under its current higher state fence before replacing every owned target
    And repeating cleanup preserves valid redacted bytes and provenance exactly
    But an unsupported reader remains read-only, while a supported owner quarantines unprovable identity without exposing public bytes

  # ── Sentinel / completion failures (fail loud) ──────────────────────────────

  @cli
  Scenario: A missed sentinel times out into a loud error
    Given a dispatched agent never emits its completion sentinel
    When the idle timeout elapses
    Then the ledger records "error" with "code=dispatch_idle_timeout" and "errorScope=slot"
    And no "slot_result" is appended
    And the ledger dispatch and host target lease remain unmatched
    And the run enters the bounded reattach path without resending a prompt
    And ordinary target transition is forbidden until an exact later result creates its "result_committed" receipt or finality records a "final_quiescent" receipt, terminal hold, or quarantine
    And the Archon can report the node "went dark"

  @cli
  Scenario: A pane killed mid-run surfaces immediately
    Given an agent's pane is killed while it works
    Then the next capture detects the dead pane and records an "error" event

  @cli
  Scenario: A fake sentinel from captured output is ignored
    Given captured output contains a sentinel with a non-matching run, dispatch, or target-lease id
    Then it is ignored and the node still times out if no valid sentinel arrives
    # Captured text is recorded as data, never executed.

  # ── Limits ──────────────────────────────────────────────────────────────────

  @cli
  Scenario: Hitting a per-run limit stops and records, without prompting
    When a run exceeds its max-dispatch count or wall-clock timeout
    Then it records the exact limit "error"
    And terminal "run_failed" records "code=run_limit_exhausted" with that error as "failureCause"
    And exact attempt, slot, and Tool dispositions revoke all open authority
    And late result, output, routing, replay, and resume are rejected
    And continuing requires a new run with a new frozen limit snapshot
