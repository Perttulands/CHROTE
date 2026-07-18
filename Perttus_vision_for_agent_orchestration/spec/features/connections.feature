# Captures the prototype's connection model (03-formations.js: startWire / startReconnect /
# startWireDrag / drawWires). Governing principle (D7): compatible connections just work.
# Connections are STRUCTURE (round-trip files↔CLI↔UI); a hand-routed lane is LAYOUT only.

Feature: Connect nodes — compatible outputs to declared inputs
  As an agent authoring a graph (or a human tweaking one)
  I need to wire compatible outputs to declared inputs, reconnect, branch, join, and unwire
  So that a mission's execution graph reflects exactly the intended flow

  Background:
    Given a board "session-search" containing:
      | node     | kind        | ports                         |
      | mission  | mission     | out                           |
      | frame    | formation   | in[0], out[0]                 |
      | research | formation   | in[0], feedback, out[0]       |
      | gate     | gate        | in, pass, fail, judge         |
      | ship     | formation   | in[0], out[0]                 |
    And every connection is addressed as "<nodeId>:<portId> -> <nodeId>:<portId>"
    And "in[N]" and "out[N]" are test aliases resolved once to stable "port_..." ids before persistence
    And "research:feedback" is an optional "gate_feedback" input with role "retry_control"

  # ── The core rule: output → input ───────────────────────────────────────────

  @ui @file
  Scenario: Drag from any output port to any input port creates a connection
    When I drag from "frame:out[0]" and release on "research:in[0]"
    Then a connection "frame:out[0] -> research:in[0]" is persisted
    And the connection has a stable id that survives reload
    And the persisted endpoints use stable port ids, not the aliases "out[0]" or "in[0]"

  @cli @file
  Scenario: The same wiring is expressible from the CLI
    When I run "archon formation wire session-search frame:out[0] research:in[0]"
    Then the identical connection exists
    And the UI renders it without a structural diff
    # UI gesture and CLI verb are two clients of one writer (D1).

  @ui
  Scenario: Pressing an INPUT port does not start a new wire — it grabs the existing one
    Given a connection "frame:out[0] -> research:in[0]" exists
    When I press "research:in[0]" and drag
    Then I am reconnecting the existing wire's target end, not drawing a new wire
    And releasing on "ship:in[0]" repoints it to "frame:out[0] -> ship:in[0]"

  @ui
  Scenario: Pressing an empty input port does nothing (you wire from an output instead)
    Given no connection lands on "ship:in[0]"
    When I press "ship:in[0]" and release
    Then no wire is created and nothing changes

  # ── "Just works" guards ─────────────────────────────────────────────────────

  @ui @file
  Scenario: Duplicate connections are refused silently
    Given a connection "frame:out[0] -> research:in[0]" exists
    When I drag from "frame:out[0]" and release on "research:in[0]" again
    Then no second connection is created

  @ui
  Scenario: A node cannot connect to itself
    When I drag from "frame:out[0]" and release on "frame:in[0]"
    Then no connection is created

  @ui
  Scenario: Releasing a wire on empty canvas or a non-port reverts with no change
    When I drag from "frame:out[0]" and release on empty canvas
    Then no connection is created and the temp wire disappears

  @file @security
  Scenario Outline: Only a Gate fail port can produce gate feedback
    Given a hand-edited board declares "<producer>" as a "gate_feedback" output
    When shared structural validation reads the board
    Then it rejects the producer before execution
    And no ordinary node can mint Gate verdict or retry authority
    Examples:
      | producer         |
      | mission:out      |
      | frame:out[0]     |
      | normalize:out[0] |

  # ── Reconnect & re-route ────────────────────────────────────────────────────

  @ui @file
  Scenario: Reconnect the source end of a committed wire
    Given a connection "frame:out[0] -> research:in[0]" exists
    When I grab the wire near its source end and release on "mission:out"
    Then the connection becomes "mission:out -> research:in[0]"

  @ui @file @layout
  Scenario: Hand-routing the middle of a wire changes its lane, not its structure
    Given a connection "frame:out[0] -> research:in[0]" exists
    When I grab the middle of the wire and drag it to a new lane
    Then the connection's endpoints are unchanged in the board file
    And the lane is stored in the layout sidecar, not the definition
    And "archon formation inspect session-search" shows no structural change

  @ui @file @layout
  Scenario: Reset routing clears a hand-routed lane
    Given a connection with a hand-routed lane
    When I right-click the wire and choose "Reset routing"
    Then the lane is removed from the layout sidecar
    And the connection auto-routes again

  @ui @file
  Scenario: Right-click a wire to remove the connection
    Given a connection "frame:out[0] -> research:in[0]" exists
    When I right-click the wire and choose "Remove connection"
    Then the connection is deleted from the board file

  @cli @file
  Scenario: Unwiring from the CLI matches the UI removal
    When I run "archon formation unwire session-search frame:out[0] research:in[0]"
    Then the connection no longer exists

  # ── Branching: gate pass/fail and mission start ─────────────────────────────

  @ui @file
  Scenario: A gate routes from distinct pass and fail outputs
    Given "research:out[0] -> gate:in" is research's only connected workflow-output edge
    When I drag from "gate:pass" to "ship:in[0]"
    And I drag from "gate:fail" to "research:feedback"
    Then "gate:pass -> ship:in[0]" and "gate:fail -> research:feedback" both exist
    And the fail wire carries typed gate feedback through a declared control port
    And the wire pointing back to "research" is the gate's only fail edge
    And this direct-source pushback loop is valid while the retry-control edge stays outside the data DAG

  @ui
  Scenario: An unwired gate fail output means "block" at run time
    Given "gate:fail" has no outgoing connection
    Then reaching the gate on a FAIL verdict blocks the run (see run-execution.feature)

  # ── JOIN: multiple inputs ───────────────────────────────────────────────────

  @ui @file
  Scenario: A node joins multiple distinct required input ports
    Given "ship:frame_input" and "ship:research_input" are distinct required ports
    When I wire "frame:out[0] -> ship:frame_input"
    And I wire "research:out[0] -> ship:research_input"
    Then each required port has exactly one producer
    And at run time it must not start until both inputs have arrived (see run-execution.feature)

  @file
  Scenario: A quiescent partial JOIN blocks loudly instead of waiting forever
    Given "ship" received one required input but another required producer can no longer route
    And every in-flight and independent attempt has settled
    When the graph quiesces before "ship" starts
    Then "error" records "code=unsatisfied_required_input" and names "ship" plus its waiting sequence
    And "run_blocked" uses "blockScope=node", "blockedNodeId=ship", and "reason=unsatisfied_required_input"
    And it has empty "openDispatches" and "retryTargets", "resumeAllowed=false", "resumePolicy=new_run_required", and no "nextEpoch"
    And "ship" projects "blocked" while its readiness counts remain inspection evidence

  @cli @file
  Scenario: Adding an input port is a structural edit with a stable new port id
    When I run "archon formation add-input session-search ship --label 'Second input'"
    Then "ship" gains an input port with a new stable id
    And existing connections are untouched

  @ui @file
  Scenario: Removing an input port by id also removes connections landing on it
    Given "ship" has input ports "in[0]" as "port_ship_a" and "in[1]" as "port_ship_b"
    And a connection lands on "port_ship_b"
    When I remove input port "port_ship_b" from "ship"
    Then "port_ship_b" and its connection are both gone
    And "port_ship_a" and its connection remain

  @file
  Scenario: Removing a middle port does not renumber surviving stable ids
    Given "ship" has input ports:
      | alias | id          |
      | in[0] | port_ship_a |
      | in[1] | port_ship_b |
      | in[2] | port_ship_c |
    And connections land on "port_ship_a" and "port_ship_c"
    When I remove input port "port_ship_b"
    Then no surviving port id is regenerated
    And the connection that displayed as "ship:in[2]" still references "ship:port_ship_c"
    And the UI may redisplay "port_ship_c" as "in[1]" only as a fresh alias

  @file
  Scenario: JOIN fan-in is explicit by input port
    Given a connection already lands on "ship:in[0]"
    When I try to wire "research:out[0] -> ship:in[0]"
    Then the writer rejects the second edge to that occupied input port with a conflict
    But adding "ship:in[1]" and wiring to that port is accepted as a JOIN input

  @file
  Scenario: A hand-edited duplicate producer fails on read and run preflight
    Given the board file contains two edges targeting the same input port
    When the shared reader validates the board
    Then it rejects the board as an invalid duplicate producer
    And run preflight rejects it before any dispatch

  @ui @file
  Scenario: Ordinary workflow cycles are rejected instead of deadlocking a JOIN
    Given distinct required ports form "mission -> A:seed", "B:out -> A:join", and "A:out -> B:in"
    When I create the last edge through the UI or hand-edit it into the board
    Then shared validation rejects the workflow-channel cycle on mutation and read
    And run preflight writes no "run_started" or dispatch
    But a separately validated direct-source Gate fail edge to "retry_control" remains outside this DAG check

  # ── Fan-out: multiple outputs ───────────────────────────────────────────────

  @ui @file
  Scenario: A formation fans out distinct results from multiple output ports
    Given "research" is not the source of a pushback loop
    When I add a second output port to "research"
    And I wire "research:out[0] -> gate:in" and "research:out[1] -> ship:in[0]"
    Then both downstream paths receive work when "research" finishes

  @file
  Scenario: Work ports declare a bounded media contract
    Given a work input accepts "text/markdown" and "application/json"
    When upstream offers "text/plain"
    Then validation rejects the incompatible connection or delivery before attempt start
    And a work payload contains exactly one of bounded text or one safe artifact under its single media type
    And a payload containing both representations is invalid
    And full JSON Schema remains deferred

  # ── Round-trip integrity ────────────────────────────────────────────────────

  @file
  Scenario: Every node, port, and edge has a stable id that round-trips
    When an agent edits the board file by hand and the UI reloads it
    Then no ids are regenerated
    And a UI rename or rewire produces a minimal diff against existing ids, not a full rewrite
