# Config-driven Formations agent-user (no hardcoded user)

Status: CODE + CONFIG ONLY. This spec covers the executor code and the one
configuration knob. No live infrastructure change is authorized here: provisioning
the correctly-owned tmux server for a split install and the `/srv` cutover are
executed separately by the orchestrator with explicit owner sign-off.
Date: 2026-07-24. Beads: chrote-ejp (implements), chrote-jkk (parent), chrote-fjy
(subsumed). Design reference: `design/executor-as-perttu` branch,
`docs/superpowers/specs/2026-07-24-executor-as-perttu-design.md`.

Golden rule that still binds: **do not disrupt running shells or tmux sessions.**
The executor only ever creates and tears down its own uniquely-named sessions and
never issues `kill-server` — that invariant is unchanged and must remain true.

---

## 1. Problem

A Formations run's agent process runs as the **owner of the tmux server** the
executor drives, because the executor shells `tmux -S <socket> new-session` and
the pane inherits the server owner's Unix identity — not the identity of the
process that issued the tmux command. A `claude`/`codex` agent must run as the
OPERATOR user (whoever owns the agent credentials in their home).

- **Single-user install** (the CHROTE service and the operator are the same user):
  agents already run as the operator. No cross-user problem, zero configuration.
- **Split install** (isolated service user + separate operator, like `/srv`): the
  service user cannot authenticate as the operator, so agents must run on a tmux
  server owned by the operator/agent-user. If the executor lazy-starts a server on
  the configured socket, it starts one owned by the *service* user — silently
  reverting agents to the wrong identity (chrote-fjy).

Owner hard constraint (msg 239): CHROTE supports ANY user; Formations must too.
NOTHING may hardcode `perttu` or any specific user. The agent-user is
CONFIGURATION.

## 2. Design

One config-driven code path serves both installs. The install chooses via a single
knob; the default makes single-user installs work with zero configuration.

### 2.1 Config knob

`CHROTE_FORMATIONS_AGENT_USER` — the Unix user the Formations tmux executor
expects to own the tmux server it drives (and therefore the user agents run as).

- **Empty / unset ⇒ default to the service user** (the user the CHROTE process
  runs as, resolved via `os/user.Current()`). Single-user installs need no config.
- Set to a username ⇒ that user is the expected agent-user (split install).
- Nothing is hardcoded to a specific username; `perttu` never appears in code.

Stored as `TmuxExecutorConfig.AgentUser`; read in `TmuxExecutorConfigFromEnv`.

### 2.2 Expected agent-user resolution

`resolveExpectedAgentUser()` returns `{uid, self}`:

- Resolve the service uid (`formationServiceUID`, default `os/user.Current()`).
- Empty `AgentUser` ⇒ `{serviceUID, self=true}`.
- Non-empty ⇒ resolve the named user's uid (`formationLookupUID`, default
  `os/user.Lookup`); `self = (agentUID == serviceUID)`.

The self/other decision is purely uid-based, so an explicitly configured name that
happens to equal the service user behaves exactly like the default (self).

### 2.3 Ownership verification + lazy-start gating (in `ensureServer`)

Before the executor pins the socket and dispatches:

1. Resolve `{uid, self}`.
2. Probe whether a tmux server is running on the socket (read-only `HasServer`).
3. If no server is running:
   - **self**: lazy-start a keeper (unchanged behavior) — the server we start is
     owned by the service user, which IS the expected agent-user, so this is safe.
   - **other** (agent-user ≠ service user): DO NOT lazy-start. Fail loud with
     `agent_user_server_absent`. Lazy-starting would create a server owned by the
     wrong (service) user; the correctly-owned server must be provisioned out of
     band (infra, done at cutover).
4. Whether the server pre-existed or was just lazy-started, verify the socket
   owner uid equals the expected agent-user uid (`formationSocketOwnerUID`, default
   `os.Stat` of the socket → `Stat_t.Uid`). Mismatch ⇒ fail loud with
   `agent_user_owner_mismatch`. This subsumes chrote-fjy: a server owned by anyone
   other than the configured agent-user is refused loudly instead of silently
   running agents under the wrong identity — including the self case where a
   foreign server is already running on the socket.

### 2.4 Typed errors

All failures are `RunExecutionError` (boundary `executor`) with codes:

- `agent_user_unresolved` — service user or configured user name could not resolve.
- `agent_user_server_absent` — other-user install, no correctly-owned server
  present, and lazy-start is refused.
- `agent_user_owner_mismatch` — socket owner uid ≠ expected agent-user uid.
- `agent_user_owner_unverified` — socket owner could not be determined.

### 2.5 Preserved safety

- The `tmuxHarnessClient` interface is unchanged: still no `kill-server`, `attach`,
  `rename`, or `resize`. Only uniquely-named owned sessions are created/torn down.
- Socket identity pinning (`pinTmuxSocketIdentity` / `validatePinnedTmuxSocket`)
  is unchanged.
- The any-socket lazy-start is preserved but now gated: it fires only in the self
  case.

## 3. Test plan (fake lookups; NO real tmux, NO real uid changes)

Injected package vars (`formationServiceUID`, `formationLookupUID`,
`formationSocketOwnerUID`) let tests pin identities without touching the OS.

1. default-to-self, server running ⇒ accept, no lazy-start.
2. self + no server ⇒ lazy-start keeper, then accept.
3. configured other-user, socket owned by that user ⇒ accept, no lazy-start.
4. configured other-user, socket owned by service user ⇒ `agent_user_owner_mismatch`.
5. configured other-user + no server ⇒ `agent_user_server_absent`, NO lazy-start,
   owner lookup never consulted.
6. only-own-sessions safety still holds through a full run in other-user mode
   (exactly one owned session created/killed, no foreign session touched, no
   `kill-server`).

## 4. Out of scope (infra, gated on owner go)

Provisioning the operator-owned formations tmux server (a `--user` unit + watchdog),
the `server-access` grant bridge, and the `/srv` cutover (set
`CHROTE_FORMATIONS_AGENT_USER` + point the socket at the operator server). The code
must correctly DRIVE a socket whose server is owned by the configured user and
REFUSE a mismatch; standing up that server is a separate, reversible cutover step.
