# Deepens what agents.feature introduces. Covers the agent LIFECYCLE: create variants, evolve,
# harness variants, live-session spawn/attach, retirement, and the forward-compatible hooks for
# evaluation-informed tuning. See ../../DECISIONS-LOCKED.md (D3: factory is first-class and early).

Feature: Build, evolve, and retire agents
  As the Archon
  I need to create agents when none fit, grow them over time, and bring them online
  So that the organization can expand and improve without Perttu hand-editing config

  Background:
    Given a workspace at "/workspace/chrote"
    And the central persona home "~/agents/"
    And the harnesses available are "claude-code", "openai-codex", and "hermes"

  # ── Creation paths ──────────────────────────────────────────────────────────

  @cli @file
  Scenario: Create a minimal agent with only the required identity
    When I run "archon agent new scout --kind specialist --harness claude-code"
    Then "~/agents/scout.toml" exists
    And it has id "scout", kind "specialist", harness default "claude-code"
    And it has an empty tags = [] list using bare capabilities plus namespaced facets
    And it has a schema version at the top of the file
    And the card id "scout" is simultaneously its filename stem and default sessionStem
    # The one-id spine: card id -> default sessionStem -> slot agentId -> ledger persona key.

  @cli @file
  Scenario Outline: Kinds are open tags, not a closed enum
    When I run "archon agent new <id> --kind <kind>"
    Then "~/agents/<id>.toml" records kind "<kind>" without rejecting it as unknown
    Examples:
      | id        | kind        |
      | archon2   | archon      |
      | leadx     | leader      |
      | susie3    | specialist  |
      | red       | reviewer    |
      | janitor   | maintainer  |
      | throwaway | disposable  |
    # The system does not police the kind vocabulary; new kinds need no code change.

  @cli @file
  Scenario: Bootstrap a card by introspecting an existing harness config
    Given an existing config at "~/.codex/config.toml"
    When I run "archon agent new codex --from ~/.codex/config.toml"
    Then the card records a source pointer to that path
    And it does NOT inline the config contents
    And the introspected harness is detected as "openai-codex"

  @cli @file
  Scenario: Bootstrap from a Hermes profile carries the profile reference
    Given a Hermes profile directory "~/.hermes/profiles/archon/"
    When I run "archon agent new archon --from ~/.hermes/profiles/archon/"
    Then the card's harness default is "hermes"
    And a source pointer references the profile directory
    And the card's launch reference is populated so the engine can bring it online

  @cli @file
  Scenario: Creating an agent with a taken id fails loud and changes nothing
    Given a card "~/agents/scout.toml" already exists
    When I run "archon agent new scout --kind specialist"
    Then the command exits non-zero with a clear "id already exists" message
    And "~/agents/scout.toml" is byte-for-byte unchanged

  @ui @file
  Scenario: Author an agent from the UI through the same writer
    Given the always-available Formations surface is open
    When I create an agent "writer" with capabilities "writing, voice" in the Agents view
    Then "~/agents/writer.toml" is written through the shared formations package
    And "archon agent inspect writer --json" returns exactly the fields the UI submitted
    # One writer, two clients — no JSON-vs-TOML drift between UI and CLI.

  # ── Evolution ───────────────────────────────────────────────────────────────

  @cli @file
  Scenario: Add a capability without disturbing unrelated fields
    Given susie's card lists bare capability tags "design, react"
    And susie's card has an agent-authored key 'reviewerNotes = "prefers tight grids"'
    When I run "archon agent edit susie --add-capability tailwind"
    Then susie's bare capability tags are "design, react, tailwind"
    And 'reviewerNotes = "prefers tight grids"' survives byte-for-byte
    And the diff is a single added line

  @cli @file
  Scenario: Removing a capability is explicit and idempotent
    Given susie's card lists bare capability tags "design, react, tailwind"
    When I run "archon agent edit susie --remove-capability tailwind"
    And I run "archon agent edit susie --remove-capability tailwind"
    Then both runs leave bare capability tags as "design, react"
    And the second run reports no change rather than erroring

  @cli @file
  Scenario: A persona spans multiple harnesses as variants under one identity
    Given susie's card has harness default "claude-code"
    When I run "archon agent edit susie --add-harness hermes --session-stem hermes-susie"
    Then susie has harness variants "claude-code" and "hermes"
    And the "hermes" variant declares sessionStem "hermes-susie"
    And her id remains "susie" (one identity, multiple bindings)
    # Q6f resolved: one card with harness variants, not two cards.

  @cli @file
  Scenario: Evaluation hints are forward-compatible, not blocking
    When I run 'archon agent edit susie --note "react quality improved over sprint 3"'
    Then the note is appended to susie's card without imposing an evaluation schema
    # Phase-3 evaluation can later read these; the factory does not require it now.

  # ── Self-knowledge ──────────────────────────────────────────────────────────

  @cli
  Scenario: An agent can read its own card
    Given a live session bound to "susie"
    When susie runs "archon agent inspect susie --json"
    Then she receives her own capability tags, personality facets, and source pointers
    # The agent is a first-class entity that can introspect itself.

  # ── Bringing agents online ──────────────────────────────────────────────────

  @cli
  Scenario: Spawn a live session for an agent from its card
    Given the card "scout" with harness default "claude-code" and no live session
    When I run "archon agent spawn scout"
    Then a tmux session whose stem is the selected variant's sessionStem is launched via the card's launch reference
    And "archon agent list --json" shows "scout" as live
    And no existing session was disturbed

  @cli
  Scenario: Spawning an already-live agent is a no-op, not a second session
    Given a live session bound to "scout"
    When I run "archon agent spawn scout"
    Then the command reports the session is already live
    And exactly one session whose stem is "scout" exists

  @cli
  Scenario: Attach surfaces the live session for the human to take over
    Given a live session bound to "scout"
    When I run "archon agent attach scout"
    Then I am connected to scout's tmux session
    And detaching leaves the session running (golden rule: never disrupt sessions)

  @cli @ui
  Scenario: Spawn a disambiguated session when the stem is ambiguous
    Given susie's "claude-code" variant declares sessionStem "claude-susie"
    And susie's "openai-codex" variant declares sessionStem "codex-susie"
    And live sessions exist for both "claude-susie" and "codex-susie"
    When I resolve agent "susie" for the "claude-code" harness
    Then it binds to "claude-susie"
    And resolving for "openai-codex" binds to "codex-susie"
    # Human-meaningful id + harness suffix on collision (F4 binding model).

  @human @cli
  Scenario: Implicit harness-prefix extraction is not guessed by S1
    Given live sessions exist for both "claude-susie" and "codex-susie"
    And susie's card does not declare either sessionStem
    When I resolve agent "susie"
    Then resolution fails loud with an ambiguity error
    And the operator must add explicit variant sessionStems or choose a future prefix-extraction rule

  # ── Retirement ──────────────────────────────────────────────────────────────

  @cli @file
  Scenario: Retiring an agent is explicit and warns about live references
    Given "scout" is referenced by a slot in formation "research-huddle"
    When I run "archon agent retire scout"
    Then the command warns that "research-huddle" still references "scout"
    And it requires "--force" to proceed
    And without "--force" the card is left in place
    # Fail loud rather than silently orphan a formation slot.
