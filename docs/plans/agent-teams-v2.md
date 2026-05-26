# CHROTE Agent Teams v2 — Architecture Plan

> **Status:** Ready for review  
> **Branch:** `feature/agent-teams`  
> **Reference:** [the-verifier-agent](https://github.com/disler/the-verifier-agent) (disler)

---

## 1. Problem Statement

The current implementation treats agent teams as a **sequential pipeline**: builder must exit before verifier spawns, verifier must exit before builder receives feedback. This is broken because:

1. **Nudging dead sessions fails.** When the verifier loop tries to send feedback back to the builder, the builder's tmux session was killed in step 1.
2. **Agents are not processes — they are team members.** A real team doesn't kill the developer to let the reviewer start.
3. **The harness engine became a message broker** mediating every round-trip between agents, adding latency and fragility.

The reference implementation (`the-verifier-agent`) shows the correct model: builder and verifier are **siblings**, both alive simultaneously, communicating through a shared seam (unix socket + session JSONL). The verifier is an **observer** — the builder doesn't know it exists.

---

## 2. Core Insight

> An agent team is a **shared workspace with concurrent tmux sessions**. The harness engine is a **supervisor**, not a message broker. Agents communicate through the filesystem directly. The engine spawns, monitors, and cleans up.

---

## 3. Design Principles

1. **Concurrent by default** — All initial agents start together and stay alive
2. **Filesystem is the seam** — Agents read/write files in the team workspace; no sockets, no REST APIs between agents
3. **Engine is a supervisor** — Spawns, pipes output, monitors health, handles cleanup. Does NOT mediate every round-trip.
4. **Nudge is for humans only** — `tmux send-keys` notifies a human in the loop. Automated agents watch files themselves.
5. **Minimal changes** — Reuse existing engine, store, and API. Add one action (`pipe`) and fix start semantics.

---

## 4. What Changes

### 4.1 Concurrent Start

**Current (broken):**
```json
{"trigger": "start", "action": "spawn", "role": "builder"},
{"trigger": "stop:builder", "action": "spawn", "role": "verifier"}
```

**New:**
```json
{"trigger": "start", "action": "spawn", "role": "builder"},
{"trigger": "start", "action": "spawn", "role": "verifier"}
```

The engine already iterates all flow steps matching a trigger. The fix is **conceptual**: stop using `stop:<role>` as a workflow sequencing mechanism. `start` spawns everyone who needs to exist.

### 4.2 New Action: `pipe`

After spawning a session, stream its pane output to a log file so other agents can read it.

```json
{"trigger": "start", "action": "pipe", "role": "builder", "value": "builder.log"}
```

Engine runs:
```bash
tmux pipe-pane -t <session> "cat > <workspace>/builder.log"
```

This is the IPC layer. The verifier watches `builder.log` (via `tail -F` in its own session). Zero engine involvement after the initial pipe setup.

### 4.3 `file:*` Triggers (Optional, Human-in-the-Loop)

For cases where the engine should react to files (e.g., notify a human builder that feedback is ready):

```json
{"trigger": "file:feedback.json", "action": "nudge", "role": "builder"}
```

**Implementation:** The engine polls the workspace alongside tmux sessions (same 5s tick, or reduce to 1s). When `feedback.json` appears, the engine reads it and nudges the builder's session. This is OPTIONAL — automated agents don't need it.

**Why polling is acceptable here:** The engine is no longer in the critical path of automated loops. It's only for human notification and status tracking. A 1–5s delay for "your review is ready" is fine.

### 4.4 Condition Syntax (Simplified)

Keep flat string conditions. No expression parsing.

| Condition | Meaning |
|-----------|---------|
| `feedback.exists` | `feedback.json` exists and is non-empty |
| `feedback.perfect` | `feedback.json` has `{"grade":"PERFECT"}` |
| `feedback.verified` | `feedback.json` has `{"grade":"VERIFIED"}` |
| `status.success` | `status.json` has `{"status":"success"}` |
| `status.failure` | `status.json` has `{"status":"failure"}` |
| `ready.exists` | `ready` file exists (generic signal) |

### 4.5 `stop:<role>` Is for Cleanup Only

```json
{"trigger": "stop:builder", "action": "status", "value": "error"},
{"trigger": "stop:verifier", "action": "status", "value": "error"}
```

Session death means something went wrong (agent crashed or was killed). The engine marks the team as error. It does NOT respawn or sequence other agents.

### 4.6 Implicit Roles (Human Builders)

A role with no `spawnCmd` is **implicit** — the user provides the session. At team creation, the user can optionally specify:

```json
POST /api/teams
{
  "harnessId": "ai-collab",
  "name": "My Review",
  "sessions": {
    "builder": "my-tmux-session-name"
  }
}
```

The engine registers this session as the builder member without spawning it. `actionNudge` and `actionPipe` work normally.

If no session is provided for an implicit role, the engine skips spawn and the role is unmanaged. `actionNudge` to an implicit role with no registered session returns a clear error.

---

## 5. Redesigned Harnesses

### 5.1 ai-collab (Builder + Verifier Loop)

```json
{
  "id": "ai-collab",
  "name": "AI Collaboration",
  "description": "Builder works, verifier reviews. Concurrent agents with file-based feedback loop.",
  "roles": [
    {
      "id": "builder",
      "label": "Builder",
      "prefix": "bld",
      "color": "#4a9eff",
      "spawnCmd": "bash"
    },
    {
      "id": "verifier",
      "label": "Verifier",
      "prefix": "vrf",
      "color": "#ff9f4a",
      "spawnCmd": "bash"
    }
  ],
  "flow": [
    {"trigger": "start", "action": "spawn", "role": "builder"},
    {"trigger": "start", "action": "spawn", "role": "verifier"},
    {"trigger": "start", "action": "pipe", "role": "builder", "value": "builder.log"},
    {"trigger": "start", "action": "pipe", "role": "verifier", "value": "verifier.log"},
    {"trigger": "file:feedback.json", "action": "nudge", "role": "builder"},
    {"trigger": "stop:builder", "action": "status", "value": "error"},
    {"trigger": "stop:verifier", "action": "status", "value": "error"}
  ]
}
```

**How it works:**
1. Engine spawns builder and verifier bash sessions, pipes both to log files.
2. Builder works. When ready, the builder (human or script) signals by creating a file or the verifier simply watches `builder.log`.
3. Verifier watches `builder.log` (via `tail -F` in its own session). When it sees the builder finish, it runs its review.
4. Verifier writes `feedback.json` in the workspace.
5. Engine detects `feedback.json` (via workspace polling) and nudges the builder's session with the feedback text.
6. Builder fixes, loop repeats.
7. When verifier decides there are no issues, it writes `status.json` with `{"status":"success"}` and exits.
8. Engine sees verifier stop + success status, marks team complete.

**For a human builder:** The builder role has no `spawnCmd` (implicit). The user works in their normal terminal. They click a dashboard **"Request Review"** button that creates `ready` in the workspace. The verifier (already running) sees it and starts reviewing. When feedback is ready, the engine nudges the user's tmux session.

### 5.2 verifier-loop (Generic)

Same as `ai-collab` but without assumptions about spawn commands.

### 5.3 pipeline (ETL — Sequential by Design)

ETL is inherently sequential, but the coordination belongs in the agent scripts, not the engine:

```json
{
  "id": "pipeline",
  "name": "ETL Pipeline",
  "description": "Extract → Transform → Load. Agents wait for predecessors via file watchers.",
  "roles": [
    {"id": "extract", "label": "Extract", "prefix": "ext", "color": "#4aff9e", "spawnCmd": "bash -c 'run-extract && touch extract-done'"},
    {"id": "transform", "label": "Transform", "prefix": "trn", "color": "#ffeb3b", "spawnCmd": "bash -c 'while [ ! -f extract-done ]; do sleep 1; done; run-transform && touch transform-done'"},
    {"id": "load", "label": "Load", "prefix": "ld", "color": "#ff5252", "spawnCmd": "bash -c 'while [ ! -f transform-done ]; do sleep 1; done; run-load && touch load-done'"}
  ],
  "flow": [
    {"trigger": "start", "action": "spawn", "role": "extract"},
    {"trigger": "start", "action": "spawn", "role": "transform"},
    {"trigger": "start", "action": "spawn", "role": "load"},
    {"trigger": "file:load-done", "action": "status", "value": "complete"},
    {"trigger": "stop:extract", "action": "status", "if": "!status.success", "value": "error"},
    {"trigger": "stop:transform", "action": "status", "if": "!status.success", "value": "error"},
    {"trigger": "stop:load", "action": "status", "if": "!status.success", "value": "error"}
  ]
}
```

All three spawn concurrently, but the transform and load agents block until their predecessor's done-file appears. The engine does not sequence them.

### 5.4 pair-programming (Already Correct)

No changes needed. Both agents spawn on start and work simultaneously.

---

## 6. Engine Changes

### 6.1 Add `actionPipe`

```go
func (e *Engine) actionPipe(team *Team, harness *Harness, step FlowStep) error {
    role := harness.findRole(step.Role)
    if role == nil {
        return fmt.Errorf("harness %q has no role %q", harness.ID, step.Role)
    }
    // Find the member's session
    var sessionName string
    for _, m := range team.Members {
        if m.RoleID == role.ID && m.SessionName != "" {
            sessionName = m.SessionName
            break
        }
    }
    if sessionName == "" {
        return fmt.Errorf("role %q has no session to pipe", step.Role)
    }
    logFile := step.Value
    if logFile == "" {
        logFile = fmt.Sprintf("%s.log", role.ID)
    }
    logPath := filepath.Join(e.store.TeamWorkspace(team.ID), logFile)
    _, err := e.runner.RunTmux("pipe-pane", "-t", sessionName, fmt.Sprintf("cat > %s", logPath))
    return err
}
```

### 6.2 Support Implicit Roles

In `StartTeam`, after loading the harness, check if any role has no `spawnCmd`. If the team has a pre-registered session for that role, create the `TeamMember` without spawning:

```go
// In StartTeam or a new RegisterMember method
for _, role := range harness.Roles {
    if role.SpawnCmd == "" {
        if sessionName, ok := team.SessionMap[role.ID]; ok {
            team.Members = append(team.Members, TeamMember{
                RoleID:      role.ID,
                SessionName: sessionName,
                Status:      MemberStatusRunning,
            })
        }
    }
}
```

Add `SessionMap map[string]string` to `Team` (optional sessions provided at creation).

### 6.3 Add `file:*` Trigger Detection

In `tick()`, after checking tmux sessions, check the workspace for new files:

```go
func (e *Engine) checkFileTriggers(team *Team, harness *Harness, knownFiles map[string]bool) error {
    ws := e.store.TeamWorkspace(team.ID)
    for _, step := range harness.Flow {
        if !strings.HasPrefix(step.Trigger, "file:") {
            continue
        }
        fileName := strings.TrimPrefix(step.Trigger, "file:")
        filePath := filepath.Join(ws, fileName)
        exists := fileExists(filePath)
        wasKnown := knownFiles[filePath]
        knownFiles[filePath] = exists
        if exists && !wasKnown {
            if err := e.executeStep(team, harness, step); err != nil {
                return err
            }
        }
    }
    return nil
}
```

Store `knownFiles` per team in `teamPoller`.

### 6.4 Update `actionNudge` to Read `with` Field

Currently hardcoded to `feedback.json`. Read the file specified in `step.With`:

```go
var text string
if step.With != "" {
    data, err := os.ReadFile(filepath.Join(ws, step.With))
    if err == nil {
        text = string(data)
    }
}
if text == "" {
    text = "[nudge] please review and continue"
}
```

### 6.5 Reduce Poll Interval

Change `pollTick` from `5 * time.Second` to `1 * time.Second` for faster file trigger detection. Tmux session health checks are cheap.

---

## 7. API Changes

### 7.1 Create Team with Explicit Sessions

```go
type createTeamReq struct {
    HarnessID string            `json:"harnessId"`
    Name      string            `json:"name"`
    Sessions  map[string]string `json:"sessions,omitempty"` // role -> tmux session name
}
```

### 7.2 No New Signal Endpoint

Delete the draft's `POST /api/teams/{id}/signal`. Agents signal by writing files. The dashboard can have a "Request Review" button that `touch`es `ready` in the workspace (via the existing files API).

---

## 8. Workspace Layout

```
.chrote/teams/<team-id>/
├── team.json              # persisted team state (existing)
├── builder.log            # piped tmux output from builder
├── verifier.log           # piped tmux output from verifier
├── feedback.json          # structured verifier output
│   {"grade":"PERFECT", "message":"...", "details": [...]}
├── status.json            # loop termination sentinel
│   {"status":"success"}
├── ready                  # generic "I'm done" signal (optional)
├── extract-done           # pipeline sentinel (optional)
├── transform-done         # pipeline sentinel (optional)
├── load-done              # pipeline sentinel (optional)
├── mail-builder.jsonl     # harness messages to builder
├── mail-verifier.jsonl    # harness messages to verifier
└── output/                # shared artifacts directory
```

---

## 9. Implementation Order

1. **Add `actionPipe`** — stream tmux output to files
2. **Support implicit roles** — `SessionMap` in Team, register without spawn
3. **Add `file:*` triggers** — workspace file detection in tick loop
4. **Update `actionNudge`** — read `step.With` file instead of hardcoded `feedback.json`
5. **Fix built-in harnesses** — concurrent start, pipe actions, file triggers
6. **Update tests** — concurrent start, pipe output, file triggers, implicit roles
7. **Reduce pollTick** — 5s → 1s
8. **Update API** — `sessions` field in create team request

---

## 10. Open Questions (For Your Review)

1. **Should we add `fsnotify` instead of file polling?** It eliminates the 1s latency but adds a dependency. Since the engine is no longer in the critical path for automated loops, polling may be sufficient.

2. **Should `pipe-pane` be automatic?** Instead of an explicit `pipe` action, should the engine automatically pipe every spawned session to `<role>.log`? This would simplify harnesses but remove flexibility.

3. **How does the verifier know WHAT to review?** In the reference, the verifier reads the builder's session JSONL. Here, the verifier reads `builder.log`. Should the engine also capture the current working directory or git diff and write it to a known file? Or is that the verifier agent's responsibility?

4. **Dashboard integration?** Should the dashboard show:
   - A "Request Review" button that creates `ready`?
   - Live feedback.json preview?
   - Grade indicator (green/orange/red)?

5. **Max loops / escalation?** The reference caps at 3 attempts. Should the harness support `maxIterations` or should this be left to the agents?

---

## 11. Success Criteria

- [ ] `ai-collab` team starts with both builder and verifier sessions alive
- [ ] Both sessions stream output to `.log` files via `pipe-pane`
- [ ] Verifier can read `builder.log` without engine involvement
- [ ] Engine detects `feedback.json` and nudges builder (human notification)
- [ ] No session depends on another session's death to function
- [ ] Implicit roles work (human builder with pre-registered session)
- [ ] Pipeline harness uses file-based agent coordination, not `stop:*` sequencing
- [ ] All existing tests pass
- [ ] New tests cover pipe action, file triggers, and implicit roles
- [ ] `go test ./...` and `go build` are green
