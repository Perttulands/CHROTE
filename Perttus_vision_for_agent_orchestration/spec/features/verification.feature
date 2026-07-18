# Schema-1 compatibility reference only. ADR-0008 retires inline verification
# in favor of explicit Gate nodes and rejects it from every new execution path
# with legacy_inline_verification_requires_migration.
# Captures in-formation verification (03-formations.js: makeVerification, verifyBandHTML,
# openGateConfig with isVerify, verifyThenFinish). A verification is a gate's check kinds MINUS its
# own routing — it runs at the END of a formation's work and either blocks or pushes back.

Feature: Legacy inline verification is retired in favor of explicit Gates
  As an operator opening older Formations evidence
  I need schema-1 inline verification to remain legible without gaining runtime authority
  So that schema-2 cannot silently invent attempt, evaluator, or revision semantics

  Background:
    Given a schema-1 board contains formation "research" with inline verification

  @ui @file
  Scenario: Legacy configuration remains inspectable
    When I inspect the formation and its verification band
    Then I can read its kinds, criterion, and block or pushback policy
    But Add, Configure, and Save verification actions are absent
    And inspection does not rewrite the board

  @cli @file
  Scenario: Schema-2 authoring fails closed
    When a CLI or UI mutation attempts to add or configure inline verification
    Then it fails "legacy_inline_verification_requires_migration"
    And no schema-2 board revision is written

  @cli @ui @api @file
  Scenario: Legacy removal is an explicit compatibility migration step
    Given I have created replacement Gate "review"
    And a named output of formation "research" is already wired to the input of Gate "review"
    When I explicitly remove the legacy inline verification with replacementGateId "review"
    Then only the legacy block is removed through the shared writer
    And CHROTE does not infer, create, or rewire any Gate

  @cli @ui @api @file
  Scenario: Removal without the named wired replacement fails closed
    When I remove the legacy inline verification without naming an existing Gate already wired from a named output of formation "research"
    Then it fails "legacy_inline_verification_requires_migration"
    And board bytes, revision, layout, Gates, and connections remain unchanged

  @cli @file
  Scenario: Execution rejects a legacy inline verification before authority
    When validation, Mission start, isolated Formation start, or resume reads the board snapshot
    Then it fails "legacy_inline_verification_requires_migration" before "run_started"
    And no node attempt, evaluator, dispatch, or revision cycle opens

  @api @ledger
  Scenario: Terminal containment remains available for a historical run
    Given a historical run snapshot contains inline verification
    When the run is canceled or failed without resuming
    Then the normal terminal event is appended
    But no evaluator, route, resume, dispatch, or revision cycle opens

  @file
  Scenario: Historical verdicts remain non-authorizing evidence
    Given a schema-1 run ledger contains "verification_verdict"
    When schema-2 inspection projects that ledger
    Then the verdict remains visible as legacy evidence
    But it cannot route output, resume the run, or open a revision attempt
