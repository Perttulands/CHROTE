# Captures the brief/input editor and the output report viewer (03-formations.js: openInputEditor,
# fileChip/bindChips, briefLabelHTML, openReport, diffHTML, mockReport, inputArrived). Briefs are
# authored (definition); outputs are produced by runs (run state). Bead resolution is a backend point.

Feature: Briefs and outputs — the manual input to a formation and the produced result
  As an agent or human setting up and reading work
  I need to give a formation a goal/bead/files and to read what a run produced
  So that each step knows its task and its result is inspectable

  Background:
    Given a formation "ship" on board "session-search"

  # ── The brief (manual input — definition) ───────────────────────────────────

  @ui @file
  Scenario: Edit a brief with a goal, a bead, and file/link references
    When I open the input editor and set a goal, bead "bd-204", and add "src/SessionPanel.tsx" and "https://example.com/spec"
    Then the brief stores the goal, the bead id, and both refs
    And file refs and link refs are distinguished in the UI

  @ui @file
  Scenario: The brief input row reflects what is set
    When the brief has only a bead
    Then the input row shows the bead chip
    And when the brief is empty it shows a "set a goal or input…" placeholder

  @cli @file
  Scenario: The brief is authorable and editable from the CLI
    When I run "archon formation set-brief session-search ship --goal 'Ship the change' --bead bd-204 --file src/SessionPanel.tsx"
    Then the brief persists those fields
    And re-running with "--remove-file src/SessionPanel.tsx" removes only that ref

  @cli
  Scenario: A bead reference resolves at run time and results write back
    Given the brief references bead "bd-204"
    When the formation runs
    Then the engine reads the bead as part of the task input
    And the produced result can be written back to the bead
    # Bead resolution is a backend integration point; Beads stays work-only (not the graph store).

  # ── Arrived inputs (run state) ──────────────────────────────────────────────

  @ui
  Scenario: A wired input shows waiting until its source delivers
    Given "frame:out[0] -> ship:in[0]"
    Then "ship" shows the input as "waiting" until "frame" produces output
    And as "ready" once it has (a gate input is ready when the gate passed)

  # ── The output / report (produced by a run) ─────────────────────────────────

  @ui
  Scenario: A finished formation's output row opens a report
    Given "ship" has finished with status "done"
    When I click the output row (or right-click → Open report)
    Then a report popup shows the status, a human-readable report, artifacts, and diffs

  @ui
  Scenario: Diffs render as unified patches and artifacts as typed references
    Then each artifact shows a name and type (doc/link/log/data)
    And each diff shows a file and its added/removed lines

  @ui @file
  Scenario: Clearing an output removes only run state, never the definition
    When I clear the formation's output
    Then the produced output is gone
    But the formation's slots, brief, and verification are untouched

    @cli
    Scenario: The same report is readable from the CLI
      When I run "archon run logs run_01J9 --node ship"
      Then I get the same status, report, artifacts, and diffs from the ledger
