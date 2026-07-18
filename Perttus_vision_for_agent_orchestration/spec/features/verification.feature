# Schema-1 compatibility reference only. ADR-0006 rejects inline verification
# from schema-2 execution with legacy_inline_verification_requires_migration
# until ctx-ug7.17 defines exact replay-safe semantics or retires it.
# Captures in-formation verification (03-formations.js: makeVerification, verifyBandHTML,
# openGateConfig with isVerify, verifyThenFinish). A verification is a gate's check kinds MINUS its
# own routing — it runs at the END of a formation's work and either blocks or pushes back.

Feature: Legacy inline verification is inspection-only until explicitly resolved
  As an operator opening older Formations evidence
  I need schema-1 inline verification to remain legible without gaining runtime authority
  So that schema-2 cannot silently invent attempt, evaluator, or revision semantics

  Background:
    Given a schema-1 board contains formation "research" with inline verification

  @ui @file
  Scenario: Legacy configuration remains inspectable
    When I inspect the formation and its verification band
    Then I can read its kinds, criterion, and block or pushback policy
    But Add, Configure, and Remove verification actions are disabled
    And inspection does not rewrite the board

  @cli @file
  Scenario: Schema-2 authoring fails closed
    When a CLI or UI mutation attempts to add, configure, or remove inline verification
    Then it fails "legacy_inline_verification_requires_migration"
    And no schema-2 board revision is written

  @cli @file
  Scenario: Schema-2 validation rejects a legacy inline verification
    When schema-2 preflight reads the board
    Then it fails "legacy_inline_verification_requires_migration" before "run_started"
    And no node attempt, evaluator, dispatch, or revision cycle opens

  @file
  Scenario: Historical verdicts remain non-authorizing evidence
    Given a schema-1 run ledger contains "verification_verdict"
    When schema-2 inspection projects that ledger
    Then the verdict remains visible as legacy evidence
    But it cannot route output, resume the run, or open a revision attempt
