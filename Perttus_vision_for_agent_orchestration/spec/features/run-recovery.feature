# Captures recovery + fail-loud behaviors implied by the run model and DECISIONS-LOCKED (ledger is
# canonical; status is projected; recovery replays; binding/sentinel failures are loud, never silent).
# The prototype mocks runs, so these are real-engine requirements the spec must pin.

Feature: Run recovery and fail-loud failure modes
  As the engine and its operators
  I need runs to survive disconnects and to fail loudly, never silently
  So that "what happened?" is always answerable and nothing hangs unnoticed

  Background:
    Given a board "session-search" whose runs write an append-only NDJSON ledger
    And run status is projected from the ledger, not stored separately

  # ── Recovery ────────────────────────────────────────────────────────────────

  @cli
  Scenario: A run recovers after a disconnect by replaying the ledger
    Given a run is mid-cascade when the client (or server) disconnects
    When the engine reconnects and replays the ledger
    Then the engine replays the ledger to the last consistent point and continues
    And no completed node is re-run

  @cli @ui
  Scenario: Status is identical whether read from CLI or UI after reconnect
    When I reconnect mid-run
    Then "archon run status run_01J9" and the Formations tab show the same projected state

  @cli
  Scenario: Two runs of the same mission have independent ledgers
    When I run the mission twice
    Then each run has its own ledger and run id with no shared mutable state

  # ── Binding failures (fail loud) ────────────────────────────────────────────

  @cli
  Scenario: Dispatch to an agent with no live session records a clear error
    Given a slot references "scout" and no live session matches
    When the engine tries to dispatch to it
    Then the ledger records an "error" event naming the unresolved agent
    And the run does not hang silently

  @cli
  Scenario: A session that exists but whose pane is dead is detected
    Given "scout" has a session whose process has exited
    When the engine dispatches to it
    Then it records a dead-session error rather than sending into a dead pane

  @cli
  Scenario: An ambiguous agent id is disambiguated or fails loud
    Given scout's card has "claude-code" sessionStem "claude-scout"
    And scout's card has "openai-codex" sessionStem "codex-scout"
    And both "claude-scout" and "codex-scout" are live for agent "scout"
    Then dispatch resolves by the slot's harness
    But if the slot has no harness and the card default does not resolve uniquely
    Then the ledger records an ambiguity "error" and does not dispatch

  @cli
  Scenario: Replay never blindly re-dispatches a prompt
    Given the ledger contains "slot_dispatch" for "slot_peer_a" with no matching "slot_result"
    When the recovery reconciler replays the ledger
    Then the engine re-attaches capture for that slot if its session is still live
    And it records a loud "error" if capture cannot be re-attached
    But it never sends the original prompt a second time without an explicit blocked-run resume

  # ── Sentinel / completion failures (fail loud) ──────────────────────────────

  @cli
  Scenario: A missed sentinel times out into a loud error
    Given a dispatched agent never emits its completion sentinel
    When the idle timeout elapses
    Then the ledger records a timeout "error" for that node
    And the Archon can report the node "went dark"

  @cli
  Scenario: A pane killed mid-run surfaces immediately
    Given an agent's pane is killed while it works
    Then the next capture detects the dead pane and records an "error" event

  @cli
  Scenario: A fake sentinel from captured output is ignored
    Given captured output contains a sentinel with a non-matching run id
    Then it is ignored and the node still times out if no valid sentinel arrives
    # Captured text is recorded as data, never executed.

  # ── Limits ──────────────────────────────────────────────────────────────────

  @cli
  Scenario: Hitting a per-run limit stops and records, without prompting
    When a run exceeds its max-dispatch count or wall-clock timeout
    Then it records the limit event as "error" and stops with "run_blocked"
    And resuming requires an explicit new "archon run resume run_01J9"
