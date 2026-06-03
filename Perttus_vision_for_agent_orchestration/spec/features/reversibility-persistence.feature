# Captures the file contract (F1) + reversibility acceptance (DECISIONS-LOCKED §2, master-plan §10).
# Definitions are TOML, ledger/board NDJSON; the shared formations package is the single writer;
# every layer has a kill switch; unknown fields survive; schema is versioned.

Feature: Reversibility and the file contract — safe, durable, and fully removable
  As the operator of a CHROTE install
  I need the system to be file-canonical, single-writer, and one-flag-removable
  So that it never corrupts state and "go back" is trivial

  Background:
    Given definitions live in "<workspace>/.formations/" (sibling of ".beads/")
    And persona cards live centrally in "~/agents/"
    And the shared formations package is the only writer of definition files

  # ── Kill switches ───────────────────────────────────────────────────────────

  @ui
  Scenario: The UI tab is gated by a default-off flag
    Given "chrote-formations" is at its default (off)
    Then the Formations tab does not mount and the dashboard behaves exactly as today

  @cli
  Scenario: The backend is gated by an env switch
    Given "CHROTE_FORMATIONS=off" and the server restarted
    Then "/api/formations/*" is not registered and no formations goroutine starts
    And "go test ./..." is green, "/api/health" is OK, and all existing PRD acceptance still passes

  @file
  Scenario: Removing the data is clean
    When I "rm -rf <workspace>/.formations/"
    Then the workspace, code, tmux sessions, and ".beads/" are untouched
    And nothing about CHROTE's existing behavior changes

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
    And run events live in the per-run NDJSON ledger
    And a run finishing or a node being dragged never dirties the board definition
