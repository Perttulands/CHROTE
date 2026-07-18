# Captures the file contract (F1) + reversibility acceptance (DECISIONS-LOCKED §2, master-plan §10).
# Definitions are TOML, ledger/board NDJSON; the shared formations package is the single writer;
# rollback preserves evidence; unknown fields survive; schema is versioned.

Feature: Reversibility and the file contract — safe, durable, and recoverable
  As the operator of a CHROTE install
  I need the system to be file-canonical, single-writer, and code-rollback-safe
  So that it never corrupts state and "go back" is trivial

  Background:
    Given definitions live in "<workspace>/.formations/" (sibling of ".beads/")
    And canonical run authority lives under "<chrote-data>/formations/" outside generic Files roots
    And persona cards live centrally in "~/agents/"
    And the shared formations package is the only writer of definition files

  # ── Rollback boundary ───────────────────────────────────────────────────────

  @ui
  Scenario: The UI is not run-lifecycle authority
    Given no browser has the Formations tab open
    Then admitted runs and file-backed evidence remain available to the coordinator and CLI

  @cli
  Scenario: A prior code version does not delete durable evidence
    Given Formations has produced evidence under ".formations/"
    And Formations has produced canonical run authority under "<chrote-data>/formations/"
    When the operator deploys or reverts to a prior CHROTE code version
    Then existing ".formations/" data is preserved byte-for-byte
    And existing "<chrote-data>/formations/" data is preserved byte-for-byte
    And tmux sessions and Beads are untouched

  @file
  Scenario: Rollback never treats canonical data as disposable cache
    When a code rollback is requested
    Then no workflow deletes or rewrites "<workspace>/.formations/"
    And no workflow deletes or rewrites "<chrote-data>/formations/"
    And explicit evidence retention or migration remains a separate operator decision

  # ── File contract (F1) ──────────────────────────────────────────────────────

  @file
  Scenario: Writes are atomic
    When a definition file is written
    Then it is written via temp-file + rename so a crash never leaves a partial file

  @file
  Scenario: Unknown fields survive edits
    Given a board file with an agent-authored key the UI does not model
    When a human edit (rename/rewire) is saved through the package
    Then the unknown key survives byte-for-byte and the diff is minimal

  @file
  Scenario: There is a single writer — no format drift
    When the same edit is made via the UI and via "archon"
    Then both go through the shared package
    And the serialized result is identical (no JSON-vs-TOML or key-order drift)

    @file
    Scenario: Concurrent edits are reconciled, not clobbered
      Given two writers attempt to change the same board
      Then one wins and the other gets a conflict (optimistic revision/etag), never silent loss

    @file
    Scenario: Structural writes increment the board revision
      Given a board file has rev 7
      When the shared writer adds a slot or rewires an edge
      Then the saved board has rev 8 and a new updatedAt
      And the prior ETag no longer matches for writes

  @file
  Scenario: External edits are detected and reloaded
    When an external process modifies a board file
    Then the change is detected (mtime/fsnotify) and surfaced as a reload signal

  @file
  Scenario Outline: Schema is versioned — refuse newer, up-migrate older
    Given a file with schema version "<v>"
    Then the package "<behavior>"
    Examples:
      | v       | behavior                                            |
      | current | reads normally                                      |
      | older   | up-migrates on read, preserving content             |
      | newer   | refuses to load and fails loud (never silent guess) |

  # ── Separation of concerns ──────────────────────────────────────────────────

  @file
  Scenario: Definition, layout, and run state are separate
    Then structure lives in the board definition
    And node x/y and wire lanes live in the layout sidecar
    And canonical run events live in the writer-only per-run NDJSON ledger under the CHROTE data root outside generic Files roots
    And the workspace exposes only definitions, layout, and authorized sanitized artifacts
    And a run finishing or a node being dragged never dirties the board definition
