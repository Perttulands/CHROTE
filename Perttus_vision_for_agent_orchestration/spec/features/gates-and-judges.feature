# Captures the prototype's gate + judge model (03-formations.js: makeGateNode, GATE_KINDS,
# attachJudge / setJudgeReturn / judgeEntry / syncJudgeKind, startJudgeWire, openJudgePicker,
# evalGate, followBranch). The headline (D7): a judge can be ONE formation (a loop) or SEVERAL
# wired in sequence, and the gate actually RUNS that judge chain to decide.
# Prototype `gate.verdict` is a mock value only. Real gate definitions do not store verdicts or route
# from remembered/default verdict state.

Feature: Gates and judges — checkpoints that route work and can be judged by formations
  As an agent wiring quality checkpoints into a mission
  I need gates that combine code/human/formation checks and route pass vs fail
  So that work only proceeds when it should, and loops back when it shouldn't

  Background:
    Given a board "session-search"
    And a gate "gate" with input "in", outputs "pass" and "fail", and a "judge" socket

  # ── Creating gates ──────────────────────────────────────────────────────────

  @ui @file
  Scenario Outline: A gate can be created from any of the prototype's affordances
    When I create a gate via "<gesture>"
    Then a gate node is persisted with default kind "code" and no stored verdict
    And fail behavior is determined by the gate's fail-port wiring
    Examples:
      | gesture                                   |
      | dragging the topbar gate token to canvas  |
      | right-clicking the board and choosing Gate |
      | the New menu's Gate item                   |

  @cli @file
  Scenario: A gate is created from the CLI with explicit checks
    When I run "archon gate create session-search --kinds code,human --criterion 'research is sound and safe to build'"
    Then the gate has kinds "code" and "human"
    And its criterion is "research is sound and safe to build"

  # ── Kinds combine ───────────────────────────────────────────────────────────

  @ui @file
  Scenario: Check kinds are additive — code, human, and formation can be combined
    When I enable "code" and "human" on the gate
    Then the gate's label reads "Code · Human"
    And at least one kind is always present (removing the last one is refused)

  @ui @file
  Scenario: Selecting the "formation" kind prompts for a judge
    When I enable the "formation" kind on a gate that has no judge
    Then the judge picker opens
    And cancelling leaves the gate without the "formation" kind
    # syncJudgeKind keeps the "formation" kind in lockstep with whether a judge is wired.

  # ── Routing: pass / fail / block / pushback ─────────────────────────────────

  @ui @file
  Scenario: Pass routes forward, fail routes to a fallback
    Given "gate:pass -> ship:in[0]" and "gate:fail -> frame:in[0]"
    When the gate evaluates to PASS at run time
    Then work flows down the pass wire to "ship"
    But not down the fail wire

  @ui
  Scenario: An unwired fail output blocks the run
    Given "gate:fail" has no outgoing connection
    When the gate evaluates to FAIL
    Then the run blocks and the upstream formation is marked blocked
    And the block is recorded as a loud event, not a silent stop

  @ui @file
  Scenario: A fail wire back to an earlier formation is a pushback/revise loop
    Given "gate:fail -> frame:in[0]" (a backward wire)
    When the gate evaluates to FAIL
    Then "frame" receives a revise brief annotated with the gate's criterion
    And the engine tolerates this cycle via the explicit wire
    And the automatic-route graph itself remains acyclic

  # ── The judge: single formation (the classic loop) ──────────────────────────

  @ui @file
  Scenario: Attach a single-formation judge by dragging the gate's judge socket onto a formation
    When I drag the gate's "judge" socket onto formation "review"
    Then two judge connections exist: "gate:judge -> review:in[0]" and "review:out[0] -> gate:judge"
    And the gate gains the "formation" kind
    And the gate card shows it has a judge

  @ui @file
  Scenario: Dropping the judge wire on empty canvas spawns a judge formation in place
    When I drag the gate's "judge" socket onto empty canvas
    Then a new solo formation "Judge" is created at the drop point
    And it is wired as the gate's judge loop
    # "Just works": the missing piece is created rather than failing.

  @ui @file
  Scenario: Dropping a formation's output onto the judge socket sets the judge return
    Given the gate has no judge yet
    When I drag from "review:out[0]" and release on "gate:judge"
    Then a judge send "gate:judge -> review:in[0]" is auto-created
    And a judge return "review:out[0] -> gate:judge" is created
    And together they form the loop

  @ui
  Scenario: Clicking the judge socket opens the judge picker
    When I click the gate's "judge" socket without dragging
    Then I can pick a NEW judge (solo / peer / orchestrated), an EXISTING formation, or detach

  # ── The judge: a CHAIN of multiple formations (the headline) ────────────────

  @ui @file
  Scenario: A judge can be several formations wired in sequence
    Given a gate with judge send "gate:judge -> j1:in[0]"
    When I wire "j1:out[0] -> j2:in[0]" and "j2:out[0] -> j3:in[0]" normally
    And I set the judge return from "j3:out[0]" onto "gate:judge"
    Then the judge is the chain "j1 → j2 → j3" with "j3" returning the verdict
    And the gate still shows a single "formation" check whose entry is "j1"

  @ui @file
  Scenario: Re-pointing the judge return moves the chain's exit without breaking the entry
    Given a judge chain "gate:judge -> j1 -> j2 -> gate:judge"
    When I drag from "j3:out[0]" onto "gate:judge"
    Then the return becomes "j3:out[0] -> gate:judge"
    And the entry "gate:judge -> j1:in[0]" is preserved

  @cli @file
  Scenario: The CLI expresses single and chained judges explicitly
    When I run "archon gate judge session-search gate --chain j1,j2,j3"
    Then the judge send targets "j1", the chain is wired in order, and "j3" returns to the gate
    And "archon gate judge session-search gate --detach" removes all judge connections and the "formation" kind

  # ── The judge runs to decide ────────────────────────────────────────────────

  @ui
  Scenario: A gate with a staffed judge actually runs the judge chain to produce the verdict
    Given the judge chain's formations have agents assigned
    When the run reaches the gate
    Then the gate enters the "evaluating" state
    And the judge chain executes from its entry
    And when the exit formation finishes, its result becomes the gate's verdict
    And the verdict routes pass/fail as usual

  @ui
  Scenario: A formation check with no staffed judge fails loud, never routes from a stored verdict
    Given the gate has the "formation" kind but its judge has no agents
    When the run reaches the gate
    Then the ledger records an "error" event for the unstaffed judge
    And the run records "run_blocked"
    And no pass or fail wire is routed from a stored or default verdict

  # ── Gate lifecycle ──────────────────────────────────────────────────────────

  @ui @file
  Scenario: Duplicate a gate carries its kinds and criterion but no run verdict
    When I duplicate the gate
    Then the copy has the same kinds and criterion, with a fresh id and offset position
    And it has no stored verdict

  @ui @file
  Scenario: Deleting a gate removes its wires too
    When I delete the gate
    Then the gate and all connections touching it (including judge connections) are gone
