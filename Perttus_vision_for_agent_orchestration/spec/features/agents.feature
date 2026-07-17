Feature: Agents exist, are discoverable, and can be live-bound
  As the Archon (or a team leader)
  I need to find the right agent for a job and create new agents when none fit
  So that I can staff formations without Perttu naming sockets or hand-writing config

  Background:
    Given a workspace at "/workspace/chrote"
    And the central persona home "~/agents/" contains the cards:
      | id     | kind       | tags                        | default harness | default sessionStem |
      | archon | archon     | framing, routing            | hermes          | archon              |
      | susie  | specialist | design, react, taste:visual | claude-code     | susie               |
      | codex  | specialist | typescript, go, fast        | openai-codex    | codex               |

  # ── Discovery: progressive disclosure (vision §12) ──────────────────────────

  @cli @file
  Scenario: Roster lists who exists without dumping their internals
    When I run "archon agent list --json"
    Then the result includes "archon", "susie", and "codex"
    And each entry shows only id, displayName, kind, and tags
    But no entry includes the system prompt, source file contents, or skill bodies
    # Progressive disclosure: the roster is for candidate selection, not full config.

  @cli @file
  Scenario: Deeper inspection is a deliberate second step
    When I run "archon agent inspect susie --json"
    Then the result includes susie's harness binding and source-file pointers
    And the source pointers reference real paths (a CLAUDE.md / profile), not inlined copies
    # "Go deeper only when needed" — the card points at the source of truth, never duplicates it.

  @cli
  Scenario: Find a candidate by capability, not by name
    When I run "archon agent list --capable react --json"
    Then "susie" is listed
    And "codex" is not listed
    # The Archon answers "who can do this?" without Perttu knowing the roster.

  @ui
  Scenario: The Agents surface mirrors the roster
    Given the chrote-formations flag is on
    When I open the Agents view
    Then I see a card per agent showing id, kind, and tags
    And clicking a card reveals its harness binding and source pointers
    And the same data is served by "GET /api/agents" (one system, two clients)

  # ── Liveness: cards are durable, sessions are live bindings ─────────────────

  @cli @file
  Scenario: A persona is persistent whether or not it is currently running
    Given no tmux session matches stem "susie"
    When I run "archon agent list --json"
    Then "susie" is listed with liveness "offline"
    # Persistence ⇔ a card exists. Liveness is a separate, live-joined fact.

  @cli
  Scenario: Liveness is left-joined from Oracle by session stem
    Given susie's default harness variant declares sessionStem "susie"
    And a live tmux session whose stem is "susie"
    When I run "archon agent list --json"
    Then "susie" is listed with liveness "live"
    And the entry references the detected session id

  @cli
  Scenario: An unbound live session is visible but not assignable
    Given a live tmux session whose stem is "scratch" with no matching card
    When I run "archon agent list --json"
    Then a "scratch" entry appears flagged "unbound"
    And it is excluded from "archon agent list --assignable"
    # Visible but not silently staffable — fail loud rather than guess an identity.
