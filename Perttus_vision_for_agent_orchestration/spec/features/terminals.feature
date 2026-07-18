# Captures the on-board live terminals (03-formations.js: openTerm, feed, MOCK #2 → real
# ttyd/websocket stream). The 2026-07-17 owner decision supersedes the old
# watch/focus-only S0 boundary: authorized Peek is a full interactive attach. The
# roster, liveness, and production target pool come from the same configured
# Terminal-session resolver and inventory used by Terminal tabs.

Feature: Live agent terminals on the canvas
  As a human supervising agents
  I need to open a live terminal for any agent right on the board
  So that I can watch and steer a session without manual socket/session hunting

  Background:
    Given a board with a staffed formation and a live agent roster

  @ui
  Scenario: Open a terminal from an agent or a slot
    When I click a roster agent without dragging, or choose "Open terminal" on an agent or filled slot
    Then a terminal popup opens for that agent on the board

  @ui
  Scenario: Terminals live in world space
    When the canvas pans or zooms
    Then open terminals pan and zoom with the world
    And a terminal can be dragged by its header and resized from its corner

  @ui
  Scenario: Re-opening an agent focuses its existing terminal
    Given a terminal for "scout" is open
    When I open "scout" again
    Then the existing terminal is focused and highlighted, not duplicated

    @cli @ui
    Scenario: The terminal streams the real session, not a mock
      When a terminal is open for a live agent
      Then it streams that agent's session output (ttyd/websocket)
      And it reflects the agent's state (attached/idle/busy/dead) from the session source

    @ui
    Scenario: Canvas Peek is a full interactive attach
      Given the exact live target is authorized for Peek
      When I focus the terminal popup and type
      Then input reaches that exact tmux session so I can steer it
      And the interaction does not become an automatic workflow dispatch
      And automatic prompt sends, retries, and interrupts remain coordinator-only

    @runtime @security
    Scenario: Production slot binding borrows from the cockpit session pool
      Given Terminal tabs and Formations use the same configured Terminal-session resolver and inventory
      And that inventory may unite several explicitly configured user and socket sources
      When a slot resolves an existing agent session
      Then accumulated context in that session is intentionally preserved
      And the ledger records the exact pane incarnation and pane/history baseline before dispatch
      But the same persona stem in more than one source fails as ambiguous
      And only certified non-pane closed/ready evidence for the exact fingerprint makes an unattached candidate runnable
      And certified active work fails as "session_target_harness_busy"
      And missing or non-unique readiness evidence fails as "session_target_readiness_unknown"
      And incomplete client/input monitoring fails as "session_target_attachment_audit_unavailable"
      And an attached candidate, including a connected hidden Terminal iframe, fails loudly and is never silently stolen
      And final atomic acquisition repeats lease, attachment, fingerprint, and certified-readiness checks before send
      And a disposable inventory proves topology only when both consumers use it through that shared resolver

    @runtime @security
    Scenario: Only durable run ownership authorizes interactive Peek
      Given a target binding exists but target-registry occupancy is not yet durable
      Then every attached client is competing and acquisition fails loud
      When exact occupancy and slot dispatch are durable
      Then "slot_peek_capability_issued" fsyncs safe metadata before the matching run, dispatch, target lease, binding, and fingerprint may attach as run-owned
      And a generic Terminal attach path rejects the occupied target unless converted to that exact Peek capability
      When the coordinator restarts
      Then Peek input is suspended until capability, recovered occupancy, and client-audit continuity are revalidated

    @runtime @security @recovery
    Scenario: A newer Peek issuance invalidates every older token
      Given one durable Peek capability issuance has no attached client or open input channel
      When the coordinator fsyncs a higher capability generation whose "priorIssuedSeq" exact-names that latest issuance
      Then every earlier token and generation is invalid before the new token is exposed
      And only the latest issued sequence and generation may attach or start steering
      And a superseded attach or input reaching the target boundary is classified foreign and latches the dispatch

    @runtime @security @recovery
    Scenario: A foreign client race invalidates the dispatch
      Given exact target occupancy and its certified client-attachment monitor are durable
      When an external or unregistered client attaches after occupancy and detaches before result validation
      Or a raw command/control client selects the pane or sends input outside the steering-generation gate
      Then "session_target_foreign_attachment" or "session_target_foreign_input" is durably recorded
      And Peek input is revoked and drained
      And the foreign bytes are never represented as a steering generation
      And no slot_result or ordinary target release is accepted
      And the target remains held or quarantined until exact non-authorizing quiescence reconciliation

    @runtime @security @recovery
    Scenario: Interactive steering is serialized with result closure
      Given a run-owned Peek is authorized for an active dispatch
      When I send the first terminal input
      Then a new steering generation is fsynced before those bytes are forwarded
      And no raw keystroke is durable
      And journal identity/state hashes contain no input bytes, content digest, client id, capability token, or guessable derivative
      And the dispatch cannot close while that generation remains open
      When I release the input channel
      Then the generation closes before a fresh turn-closure proof is accepted
      And that proof binds the baseline hash and latest generation through evidence user-writable pane bytes cannot forge
      And a sentinel typed through Peek cannot close the turn by itself
      And result closure durably revokes the capability before accepting that proof
      And the proof also binds continuous accounting of every client attach and detach
      And later input invalidates an earlier proof before result or is rejected after capability revocation

    @runtime @security @recovery
    Scenario: Cancel and finality close all Peek input authority
      Given a run-owned Peek has an open steering generation
      When cancel or failure reconciliation begins
      Then new capability issuance and input stop under the target occupancy
      And the input channel drains before "slot_steering_ended" records "capability_revoked"
      And "slot_peek_capability_revoked" is fsynced before final proof validation
      And any coordinator Ctrl-C requires its own durable one-shot reconciliation-interrupt permit and is never retried after uncertain send
      And an execution-final event is rejected while any capability, channel, or generation remains open
      And a terminal hold after finality is non-interactive run evidence

    @runtime @security @recovery
    Scenario: Pane history baseline fails closed when continuity is lost
      Given slot dispatch durably records one tmux-pane-history-baseline-v1 token and hash
      When history is trimmed past the boundary, cleared, reset, resized or reflowed
      Or the pane is replaced or restart cannot prove the same cursor epoch
      Then capture fails with capture_baseline_unavailable
      And no slot_result or ordinary target release is recorded
      And sanitized clients receive only baseline encoding, hash, and validation state

    @ui @runtime
    Scenario: Peek tile geometry cannot resize an active pane
      Given a run-owned Peek is open after the dispatch baseline
      When I move or resize its board tile
      Then only the browser viewport changes
      And no tmux resize or SIGWINCH is sent while the dispatch is active

  @ui
  Scenario: A terminal for a dead or missing session is shown as such
    Given an agent whose session has exited
    When I open its terminal
    Then it indicates the session is not live rather than appearing connected
    # Fail loud: never present a dead session as attached.

  @ui @security
  Scenario: Run-bound Peek never shows newer work as an old attempt
    Given an old slot dispatch records binding, target lease, opaque target, and frozen fingerprint
    When that exact dispatch still owns active unmatched occupancy in a non-final run before reconciliation
    Then run-bound Peek may attach interactively with the exact attempt identity
    When occupancy becomes a terminal hold, release receipt, quarantine, or a later run reuses the target
    Then the old attempt shows captured history, unavailable, or "pane_moved_on"
    And a terminal hold permits no run-bound input
    And it never labels current live bytes as evidence from the old run
    And "Open current session" is a separate explicitly non-run action

  @ui
  Scenario: Closing a terminal never disrupts the underlying session
    When I close a terminal popup
    Then only the popup closes
    And the agent's tmux session keeps running (CHROTE golden rule)
