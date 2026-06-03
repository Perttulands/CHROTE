# Captures in-formation verification (03-formations.js: makeVerification, verifyBandHTML,
# openGateConfig with isVerify, verifyThenFinish). A verification is a gate's check kinds MINUS its
# own routing — it runs at the END of a formation's work and either blocks or pushes back.

Feature: Verification — an in-formation quality check before output flows on
  As an agent wiring quality into a single formation
  I need a check that runs after the formation's work and gates its own output
  So that a formation doesn't emit work that fails its own bar

  Background:
    Given a formation "research" on board "session-search"

  @ui @file
  Scenario: Add a verification to a formation
    When I click the formation's "+ verify" band (or right-click → Add verification)
    Then a verification is attached with default kind "code"
    And its config opens for editing

  @cli @file
  Scenario: A verification is authorable from the CLI
    When I run "archon verification add session-search research --kinds code --criterion 'both reads converge on a recommendation'"
    Then "research" has a verification with kind "code" and that criterion

  @ui @file
  Scenario: A verification combines the same kinds as a gate, except formation-judge routing
    When I configure the verification
    Then I can combine "code" and "human" checks
    But it has no pass/fail output ports of its own (it is in-formation, not a routing node)

  @ui @file
  Scenario Outline: The fail policy is block or pushback
    When I set the verification's onFail to "<policy>"
    Then a failing verification "<effect>"
    Examples:
      | policy   | effect                                                        |
      | block    | stops the run at this formation (work goes no further)        |
      | pushback | returns to the formation's own agents with feedback to revise |

  @ui
  Scenario: Verification runs at the end of the formation's work
    When "research" finishes its agents' work
    Then the verification evaluates before any output flows downstream
    And the ledger records "verification_verdict"

  @ui
  Scenario: A passing verification lets the output finalize and flow on
    Given the verification passes
    Then "research" output is finalized and cascades to its downstream wires

  @ui @file
  Scenario: Remove a verification
    When I right-click the verification band and choose "Remove verification"
    Then the formation has no verification and its output is no longer gated by one

  @ui @file
  Scenario: Verification config and badge reflect the fail policy at a glance
    Then the verification band shows its kinds, criterion, and a block/pushback badge
