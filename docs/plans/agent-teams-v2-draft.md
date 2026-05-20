# CHROTE Agent Teams v2 — Architecture Plan (DRAFT)

## Problem Statement

The current agent team implementation treats collaboration as a **sequential pipeline** where agents must die before the next one can spawn. This is fundamentally wrong:

- `ai-collab` harness: builder must exit → verifier spawns → verifier must exit → builder gets nudged with feedback
- If the builder session is killed, nudging it fails because the session no longer exists
- Agents are tmux sessions — they should coexist like a real team in a shared workspace

The reference implementation (`the-verifier-agent`) shows the correct model: **builder and verifier are siblings**, both alive simultaneously, communicating through a shared seam (unix socket + session JSONL). The verifier is an **observer** — the builder doesn't know it exists.

## Core Insight

> An agent team is a **shared workspace with concurrent tmux sessions**, not a sequential process pipeline. Agents communicate through the filesystem. The harness **coordinates**, it does not sequence kills and spawns.

## Design Principles

1. **Concurrent by default** — All initial agents start together and stay alive
2. **Signal-based coordination** — Agents signal readiness via files/API, not death
3. **Nudge living sessions only** — Never depend on respawn for communication
4. **Workspace is the seam** — Files in `.chrote/teams/<team-id>/` are the IPC layer
5. **Minimal changes** — Reuse existing engine, store, and API structure

---

## Detailed Changes

### 1. New Trigger Model

| Trigger | Semantics | Use Case |
|---------|-----------|----------|
| `start` | Fires once when `POST /api/teams/{id}/start` is called. **All matching steps execute concurrently.** | Spawn initial agents |
| `signal:<role>` | Fires when a signal file appears in the team workspace | Agent signals "I'm done, next step" |
| `stop:<role>` | Fires when a tmux session exits | Cleanup, error handling, optional respawn |

**Key change:** `start` now executes ALL matching steps, not just the first one. Steps with the same trigger have no ordering guarantees — they are independent.

### 2. Signal Mechanism

**Signal files:** `.chrote/teams/<team-id>/.signal-<role>-<action>`

Example: `.signal-builder-ready` means "builder is ready for review"

**New API endpoint:**
```
POST /api/teams/{id}/signal
Content-Type: application/json

{"role": "builder", "action": "ready"}
```

This creates `.signal-builder-ready` in the team workspace. The engine detects it on the next tick, fires the `signal:builder` trigger, and deletes the signal file.

**Who calls the signal API?**
- Agents running in tmux sessions: `curl -X POST http://localhost:8080/api/teams/{id}/signal -d '{"role":"builder","action":"ready"}'`
- Users via dashboard button or CLI
- Scripts (e.g., git hooks, build scripts)

### 3. Concurrent Start

Current behavior:
```
start → spawn builder
stop:builder → spawn verifier  // sequential, dependent on death
```

New behavior:
```
start → spawn builder
start → spawn verifier         // both spawn simultaneously
signal:builder → nudge verifier
signal:verifier → nudge builder (if feedback)
signal:verifier → status complete (if !feedback)
```

**Engine change:** `handleTrigger("start")` iterates through ALL flow steps where `Trigger == "start"` and executes each. No dependency chain.

### 4. Nudge to Living Sessions Only

`actionNudge` already checks `has-session` before sending keys. This stays. The key difference: **the harness designer ensures the target session stays alive.**

If a nudge target is dead, the engine sets the team to `error` with a clear message:
> "Cannot nudge role 'builder': tmux session '...' no longer exists. The harness expects this role to stay alive. Check that the role's spawnCmd starts a long-running process (e.g., `bash`, not a one-shot script)."

### 5. Workspace Communication

The team workspace (`.chrote/teams/<team-id>/`) is the shared seam:

| File | Writer | Reader | Purpose |
|------|--------|--------|---------|
| `feedback.json` | Verifier | Harness, Builder | Structured review feedback |
| `status.json` | Any agent | Harness | Success/failure sentinel |
| `mail-<role>.jsonl` | Harness | Agent | Append-only message log |
| `output/` | Any agent | Any agent | Shared artifacts directory |
| `.signal-<role>-<action>` | Agent/User | Harness | Trigger signals |

**Condition evaluation:**
- `feedback.exists` → `feedback.json` exists and is non-empty
- `feedback.grade` → read `feedback.json` and check `grade` field (PERFECT, VERIFIED, PARTIAL, FEEDBACK, FAILED)
- `success` / `failure` → `status.json` exists with matching status

### 6. Role Spawn Model

```go
type HarnessRole struct {
    ID        string `json:"id"`
    Label     string `json:"label"`
    Prefix    string `json:"prefix"`
    Color     string `json:"color,omitempty"`
    SpawnCmd  string `json:"spawnCmd,omitempty"`   // empty = not auto-spawned (user/external)
    // Optional: restart policy
    Restart   string `json:"restart,omitempty"`    // "no", "on-failure", "always" (default: "no")
}
```

A role with no `spawnCmd` is **implicit** — the user or an external process provides it. Example: the builder could be the user's current OpenCode terminal, not a spawned tmux session.

### 7. Health Monitoring

The engine already polls tmux sessions every 5s. Add:

- Detect session death → mark member as `stopped`
- If a harness step depends on a stopped session → error
- Optional `respawn` action (future):
  ```json
  {"trigger": "stop:builder", "action": "respawn", "role": "builder"}
  ```

### 8. Redesigned ai-collab Harness

```json
{
  "id": "ai-collab",
  "name": "AI Collaboration",
  "description": "Builder works, verifier reviews. Concurrent agents with signal-based feedback loop.",
  "roles": [
    {
      "id": "builder",
      "label": "Builder (OpenCode)",
      "prefix": "bld",
      "color": "#4a9eff"
    },
    {
      "id": "verifier",
      "label": "Verifier (Claude Code)",
      "prefix": "vrf",
      "color": "#ff9f4a",
      "spawnCmd": "bash"
    }
  ],
  "flow": [
    {"trigger": "start", "action": "spawn", "role": "builder"},
    {"trigger": "start", "action": "spawn", "role": "verifier"},
    {"trigger": "signal:builder", "action": "nudge", "role": "verifier", "with": "builder.output"},
    {"trigger": "signal:verifier", "action": "nudge", "role": "builder", "if": "feedback.exists"},
    {"trigger": "signal:verifier", "action": "status", "if": "!feedback.exists", "value": "complete"}
  ]
}
```

**How it works:**
1. `start` spawns both builder and verifier sessions
2. Builder works in its tmux session. When ready, it signals: `curl -X POST .../signal -d '{"role":"builder","action":"ready"}'`
3. Engine detects signal → fires `signal:builder` → nudges verifier with review request text
4. Verifier (Claude Code in its session) receives the text, reviews the code, writes `feedback.json`, then signals: `curl -X POST .../signal -d '{"role":"verifier","action":"done"}'`
5. Engine detects signal → fires `signal:verifier`
   - If `feedback.json` exists → nudge builder with feedback text → loop back to step 2
   - If no feedback → mark team `complete`

### 9. Updated verifier-loop Harness

```json
{
  "id": "verifier-loop",
  "name": "Verifier Loop",
  "description": "Builder writes, verifier reviews, loop until clean. Concurrent agents.",
  "roles": [
    {"id": "builder", "label": "Builder", "prefix": "bld", "color": "#4a9eff"},
    {"id": "verifier", "label": "Verifier", "prefix": "vrf", "color": "#ff9f4a"}
  ],
  "flow": [
    {"trigger": "start", "action": "spawn", "role": "builder"},
    {"trigger": "start", "action": "spawn", "role": "verifier"},
    {"trigger": "signal:builder", "action": "nudge", "role": "verifier", "with": "builder.output"},
    {"trigger": "signal:verifier", "action": "nudge", "role": "builder", "if": "feedback.exists"},
    {"trigger": "signal:verifier", "action": "status", "if": "feedback.grade == PERFECT", "value": "complete"},
    {"trigger": "signal:verifier", "action": "status", "if": "feedback.grade == VERIFIED", "value": "complete"}
  ]
}
```

### 10. Implementation Order

1. **Add signal support to engine**
   - `handleSignal(team, harness, role, action)` method
   - Poll workspace for `.signal-*` files in `tick()`
   - Delete signal files after processing

2. **Change `start` trigger semantics**
   - Execute ALL matching flow steps concurrently
   - No ordering guarantees

3. **Add signal API endpoint**
   - `POST /api/teams/{id}/signal`
   - Validate role exists in harness
   - Create signal file in workspace

4. **Update condition evaluation**
   - Support `feedback.grade == X` syntax
   - Support `feedback.exists`

5. **Update built-in harnesses**
   - `verifier-loop`: concurrent start, signal-based
   - `pipeline`: keep sequential for ETL use case (spawn on signal, not stop)
   - `pair-programming`: already concurrent, minor updates
   - `ai-collab`: new design as above

6. **Update tests**
   - Test concurrent start spawns multiple sessions
   - Test signal detection and trigger firing
   - Test nudge to living session
   - Test error on nudge to dead session

7. **Update documentation**
   - Harness authoring guide
   - Signal API docs
   - Example workflows

---

## Open Questions

1. **Should we support role dependencies?** E.g., "spawn verifier only after builder has written its first output." This could be handled by conditions: `{"trigger": "start", "action": "spawn", "role": "verifier", "if": "builder.output.exists"}`

2. **Should nudge include structured data?** Currently nudge sends raw text. Should it support JSON envelopes so the receiver knows it's a harness message vs. user input?

3. **How does the verifier read builder output?** The reference uses session JSONL. We could capture tmux pane output to a file: `tmux capture-pane -t <session> -p > builder-output.txt` as part of the nudge action.

4. **Rate limiting / max loops?** The reference caps at 3 attempts then escalates to human. Should the harness support `max_iterations` or similar?

5. **Dashboard integration?** A "Signal" button in the dashboard for each role? Auto-signal detection based on output patterns?

## Risks

- **Signal file races:** Two agents signal simultaneously. Mitigation: signal files include timestamp (`touch .signal-builder-ready-$(date +%s)`), engine processes all and deduplicates by (role, action).
- **Signal spam:** An agent signals repeatedly. Mitigation: engine deletes signal file immediately after reading; only processes each file once.
- **Tmux session bloat:** Concurrent agents mean more sessions. Mitigation: `ResetTeam` and `DestroyTeam` already kill all sessions. Add TTL or auto-cleanup for old teams.
- **Feedback loop hangs:** Builder and verifier keep signaling each other forever. Mitigation: `max_loops` field on harness or team.

## Success Criteria

- [ ] `ai-collab` team starts with both builder and verifier sessions alive
- [ ] Builder signals → verifier is nudged → verifier reviews → signals back
- [ ] Feedback loop works: builder receives feedback, fixes, signals again
- [ ] Loop terminates when verifier writes no feedback or gives PERFECT grade
- [ ] All existing tests pass
- [ ] New tests cover signal mechanism and concurrent start
- [ ] No session depends on another session's death to function
