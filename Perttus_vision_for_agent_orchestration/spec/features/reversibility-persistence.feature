# Captures the file contract (F1) + reversibility acceptance (DECISIONS-LOCKED §2, master-plan §10).
# Definitions are TOML and runtime ledgers are NDJSON; the shared formations package serializes
# definitions while one fenced coordinator writes runtime authority; rollback preserves evidence.

Feature: Reversibility and the file contract — safe, durable, and recoverable
  As the operator of a CHROTE install
  I need the system to be file-canonical, single-writer, and code-rollback-safe
  So that it never corrupts state and "go back" is trivial

  Background:
    Given definitions live in "<workspace>/.formations/" (sibling of ".beads/")
    And canonical run authority lives under one "<formations-host-authority-root>/" outside generic Files roots
    And persona cards live centrally in "~/agents/"
    And the shared formations package is the only serializer of definition files
    And the current fenced CHROTE server coordinator is the sole semantic writer of schema-2 runtime authority

  # ── Rollback boundary ───────────────────────────────────────────────────────

  @ui
  Scenario: The UI is not run-lifecycle authority
    Given no browser has the Formations tab open
    Then admitted runs continue under the coordinator
    And CLI or later UI inspection uses only its sanitized projection
    And neither client reads the private ledger or gains lifecycle authority

  @cli @file
  Scenario: Archon offline access ends at workflow definitions
    Given the workspace definitions are readable but the coordinator is unavailable
    Then Archon can author, validate, and inspect workflow definitions through the shared serializer
    But run start, resume, cancel, verdict, list, status, logs, and follow fail loud before mutation or external effect
    And Archon has no local-engine or private-ledger fallback

  @cli
  Scenario: A certified guarded rollback version does not delete durable evidence
    Given Formations has produced evidence under ".formations/"
    And Formations has produced canonical run authority under "<formations-host-authority-root>/"
    And the rollback binary reports the non-authorizing "formations.runtime-authority-read-guard.v1" capability
    When the operator deploys that certified prior CHROTE code version
    Then existing ".formations/" data is preserved byte-for-byte
    And existing "<formations-host-authority-root>/" data is preserved byte-for-byte
    And tmux sessions and Beads are untouched

  @file
  Scenario: Rollback never treats canonical data as disposable cache
    When a code rollback is requested
    Then no workflow deletes or rewrites "<workspace>/.formations/"
    And no workflow deletes or rewrites "<formations-host-authority-root>/"
    And explicit evidence retention or migration remains a separate operator decision

  @file @security
  Scenario: An older reader fails closed on unsupported runtime authority schema
    Given durable runtime authority uses an unsupported authority schema
    And the older binary belongs to the certified post-guard rollback set
    When it opens the immutable workspace bootstrap, mutable workspace authority high-water mark, and complete hash-matched current admission-policy chain under the owner lock
    Then it may expose only a separately certified sanitized inspection projection
    But it does not allocate a fence, clean, quarantine, adopt authority, dispatch, accept a result, mutate execution state, cancel, or finalize
    And the durable bytes remain preserved for a compatible reader or explicit migration

  @file @security
  Scenario: Pre-guard binaries are not schema-2 rollback targets
    Given a historical binary can still run the legacy Archon or API local engine
    Then schema-2 admission remains disabled while that binary is runnable as rollback
    And enabling admission requires a certified inventory in which every active and rollback binary honors the bootstrap/workspace-authority guard
    And an arbitrary pre-guard prior version is prohibited from runtime use against that workspace

  # ── File contract (F1) ──────────────────────────────────────────────────────

  @file
  Scenario: Writes are atomic
    When a definition file is written
    Then it is written via temp-file + rename so a crash never leaves a partial file

  @file @security
  Scenario: Mutable runtime authority publishes one crash-safe generation
    Given the supported current owner holds the governing registry or workspace authority lock
    When it updates the registry, workspace counters/current policy ref, owner lease, or one command record and its terminal receipt fields
    Then each closed record increments its revision and writes a same-directory generation-checked temp, fsyncs it, atomically renames it, and fsyncs the parent
    And registry generations use only the shared host registry lock while workspace-local generations use only the owner lock
    And each mutable record exact-binds its expected previous revision and same-encoding SHA-256 through required "priorGeneration"
    And a permissively decoded legacy record missing that binding is non-authorizing and never auto-upgraded
    And every revision, fence, and admission sequence stays within the JSON-safe positive-integer range or fails before mutation
    And immutable authority is fully written and fsynced in same-directory staging before atomic no-replace canonical install and parent fsync
    And an exact canonical immutable file makes retry idempotent while conflicting bytes are never replaced
    And a leftover staging file or torn/conflicting mutable generation never authorizes execution

  @file @security @recovery
  Scenario: A crash cannot leave a partial canonical immutable file
    Given immutable authority bytes are being published under the governing registry or owner lock
    When the process crashes before staging fsync, after staging fsync, after atomic no-replace install, or before parent fsync
    Then the canonical path is absent or contains only the complete exact bytes
    And recovery may discard only a validated unreferenced staging file
    And exact installed bytes complete the parent fsync and return the original success while a conflict fails loud

  @file @security
  Scenario: Admission policy history is immutable and hash-linked
    Given each policy revision is a closed canonical file in one complete contiguous chain to revision 1
    When the owner changes configured limits or disabled state
    Then it stages/fsyncs and atomically no-replace-installs the next immutable policy before atomically replacing the current workspace revision/hash ref
    And generations referenced by terminal start decisions, run admission, or activation remain byte-preserved and hash-verifiable
    And retry completes an exact installed-but-unreferenced generation or returns success for the exact already-current generation
    But a missing generation, discontinuity, cycle, stale ref, or byte/hash conflict authorizes no mutation

  @file
  Scenario: Unknown fields survive edits
    Given a board file with an agent-authored key the UI does not model
    When a human edit (rename/rewire) is saved through the package
    Then the unknown key survives byte-for-byte and the diff is minimal

  @file
  Scenario: There is one definition serializer — no format drift
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
    And canonical run events live in the writer-only per-run NDJSON ledger under "<formations-host-authority-root>/" outside generic Files roots
    And only the current workspace lease/fence holder may append or perform a runtime side effect
    And the workspace exposes only definitions, layout, and authorized sanitized artifacts
    And a run finishing or a node being dragged never dirties the board definition
