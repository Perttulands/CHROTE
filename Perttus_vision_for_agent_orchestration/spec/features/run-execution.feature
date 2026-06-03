# Captures the RUN MODEL (03-formations.js header + runMission/flowFrom/followBranch/evalGate/
# runFormation/inputsReady). The prototype mocks execution with setTimeout; this spec defines the
# real engine behavior: cascade along wires, JOIN readiness, gate routing, judge execution,
# verification, fail-loud limits, and the append-only ledger that makes runs explainable.

Feature: Run a mission — cascade work along the wires with gates, joins, and judges
  As the engine driving a formation graph
  I need to dispatch work to live agents, gather results, and route by verdict
  So that a mission produces real artifacts and the run is fully auditable

  Background:
    Given a board "session-search" with mission -> frame -> research -> gate -> ship
    And the formations are staffed with live agent sessions
    And each run writes an append-only NDJSON ledger that status is projected from
    And ledger events use the envelope defined in "spec/contracts.md"

  # ── The cascade ─────────────────────────────────────────────────────────────

  @ui @cli
  Scenario: Starting a mission cascades the whole reachable chain
    When I run "archon mission run session-search" (or press the mission's start)
    Then the engine resolves the sub-graph reachable from the mission
    And the mission objective is the seed input to the first step
    And as each node finishes, its output flows along every outgoing wire
    And the ledger records "run_started" with run id, board id, board rev, mission id, actor, and seq 1
    And then per-node "node_started"/"node_output"

  @ui @file
  Scenario: A successful reachable chain records run_succeeded
    Given every reachable node finishes and no gate blocks
    When the final downstream node finishes
    Then the ledger records "run_succeeded" as the terminal event
    And no further node or gate events are appended for that run

  @ui
  Scenario: Starting a mission with no outgoing wire fails loud
    Given the mission has no connection from its output
    When I start it
    Then it reports "wire the mission to a step" and does not start

  @cli
  Scenario: A single node can be run in isolation for testing
    When I run "archon formation run session-search research"
    Then only that formation runs, seeded by its brief
    And its output is finalized in the ledger

  # ── Dispatch to live sessions (cross-harness is just dispatch — D4) ──────────

  @cli
  Scenario: A slot is dispatched to its bound tmux session via the adapter
    When the engine dispatches a node's work to slot agent "codex"
    Then it resolves "codex" to a live session and sends the brief
    And it records "slot_dispatch" in the ledger
    And a Claude Code slot and a Codex slot are dispatched the same way (no special path)

  @cli
  Scenario: Completion is detected by a sentinel line carrying the run id
    When an agent finishes and emits "<<<CHROTE-DONE run-id=... status=ok artifact=...>>>"
    Then the engine records "slot_result" with the artifact reference
    And a sentinel whose run-id does not match is ignored (prompt-injection safe)

  # ── JOIN readiness ──────────────────────────────────────────────────────────

  @ui
  Scenario: A node with multiple inputs waits for all of them
    Given "ship" has inputs from both "frame" and "research"
    When only "frame" has delivered
    Then "ship" shows "waiting · 1/2 inputs" and does not start
    When "research" also delivers
    Then "ship" becomes runnable and is dispatched

  # ── Gate routing at run time ────────────────────────────────────────────────

  @ui
  Scenario: A gate evaluates then routes pass vs fail
    When the run reaches the gate
    Then the ledger records "gate_evaluating" then "gate_verdict"
    And a PASS sends work down the pass wire(s)
    And a FAIL sends work down the fail wire(s)

  @ui
  Scenario: An unwired fail output blocks and marks the upstream formation
    Given "gate:fail" is unwired and the verdict is FAIL
    Then the ledger records "run_blocked"
    And the feeding formation is marked blocked
    And the block is surfaced, never silent

  @ui
  Scenario: A fail wire to an earlier formation pushes back with revise feedback
    Given "gate:fail -> frame:in[0]"
    When the verdict is FAIL
    Then "frame" is re-dispatched with a brief annotated "revise — <gate criterion>"
    And the ledger records a new attempt for "frame"
    And the cycle proceeds via the explicit wire without the auto-route graph being cyclic

  @ui
  Scenario: A pushback loop stops loudly at the attempt limit
    Given "gate:fail -> frame:in[0]" and max attempts for "frame" is 3
    When the third revised attempt fails the gate again
    Then the ledger records "error" with reason "revise loop exhausted"
    And the run records "run_blocked"
    And "frame" is not dispatched a fourth time

  # ── Judge execution ─────────────────────────────────────────────────────────

  @ui
  Scenario: A gate with a staffed judge runs the judge chain and uses its result as the verdict
    Given the gate's judge is the chain "j1 -> j2 -> j3"
    When the run reaches the gate
    Then "gate_evaluating" is recorded and the judge chain executes from "j1"
    And when "j3" finishes, its result becomes the gate verdict and routes pass/fail

  # ── In-formation verification ───────────────────────────────────────────────

  @ui
  Scenario: A formation's verification runs after its work finishes
    Given "research" has a verification with onFail "block"
    When "research" finishes its work
    Then the verification evaluates and records "verification_verdict"
    And on PASS the output is finalized and flows on
    And on FAIL with onFail "block" the run blocks here

  @ui
  Scenario: A verification with onFail "pushback" returns to the formation's own agents
    Given "research" has a verification with onFail "pushback"
    When the verification FAILS
    Then "research" is marked needs-review and re-engaged with feedback

  # ── Outputs are produced by the run, never authored ─────────────────────────

  @ui @file
  Scenario: A finished node has a produced Output with status, report, artifacts, and diffs
    When a formation finishes
    Then its Output has a status in {done, needs-review, blocked}
    And a human-readable report, plus any artifacts and diffs
    And the Output lives in run state, never in the board definition

  # ── Liveness, cancel, and fail-loud limits ──────────────────────────────────

  @ui @cli
  Scenario: A run streams live and can be watched from either surface
    When a run is in progress
    Then the UI receives events over SSE and the CLI "archon run logs --follow" tails the ledger
    And both show the same truth projected from the ledger

  @cli
  Scenario: A run can be cancelled
    When I run "archon run abort run_01J9"
    Then "run_canceled" is recorded as the last accepted event
    And no further node events are appended

  @cli
  Scenario: An unrecoverable engine error records run_failed
    Given the adapter returns an unrecoverable dispatch error before any slot receives work
    When the engine cannot continue the run
    Then the ledger records "error" naming the failing adapter boundary
    And "run_failed" is recorded as the terminal event

  @cli
  Scenario: Per-run fail-loud limits stop runaway loops without asking permission
    Given a per-run max-dispatch count and wall-clock timeout
    When either limit is exceeded
    Then the run records the limit hit as "error"
    And records "run_blocked" as the terminal event (record-and-stop, not approval)
    # Honors "no safeguards" (no gating) while preventing the runaway-cost failure mode.
