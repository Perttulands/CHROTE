# Executor-as-perttu design (chrote-jkk)

Status: DESIGN ONLY. No live infrastructure change is authorized by this
document. The cutover is executed later by the orchestrator with explicit owner
sign-off. Date: 2026-07-24. Branch: `design/executor-as-perttu`.

Golden rule that binds every option below: **do not disrupt running shells or
tmux sessions.** The operator's live cockpit sessions (e.g. the `Sol` codex/claude
sessions on `/run/user/1000/chrote-tmux/tmux-1000/default`) must never be
touched by executor churn.

---

## 1. The problem, confirmed from the live system (read-only)

The Formations executor launches a `claude`/`codex` agent by shelling out
`tmux -S <socket> new-session -d ... <launch>` and then driving the pane with
`send-keys`/`load-buffer`/`paste-buffer` (`src/internal/formations/tmux_executor.go`,
`realTmuxHarnessClient`). The agent process therefore runs **as the owner of the
tmux server that hosts the pane** — not as the user that issued the tmux command.
The executor's own comment in `.env.example` states this plainly: *"They do not
sandbox the agent process; tmux agents still have the host Unix user's own
filesystem permissions."*

Today the service that hosts the executor runs as `chrote`:

- `systemctl cat chrote-srv.service` → `User=chrote`, `Group=chrote`,
  `EnvironmentFile=/etc/chrote/chrote-srv.env`, `RuntimeDirectory=chrote`,
  `KillMode=control-group`.
- `id chrote` → `uid=999(chrote) gid=989(chrote) groups=989(chrote)` — a
  dedicated, low-privilege, isolated user. No `sudo`, no `docker`.
- `id perttu` → `uid=1000(perttu) ... groups=1000(perttu),27(sudo),989(chrote),1001(docker)`
  — the owner: **member of `sudo` and `docker`, and already a member of the
  `chrote` group.**

Two hard walls make an agent launched *as chrote* fail:

1. **Credentials are perttu-only, and the wall is real.** `claude` reads
   `/home/perttu/.claude/.credentials.json`. `getfacl` shows an ACL entry
   `user:chrote:rwx` on that file — but `mask::---`, so the **effective**
   permission is `---`. Someone already tried to grant chrote and the mask
   nullifies it. Even if the mask were widened, `claude` rewrites this file
   `0600 perttu:perttu` on every OAuth token refresh, so any group/ACL grant is
   fragile by construction. chrote cannot durably authenticate as a claude agent.

2. **The agent must run as perttu to be perttu.** Because the pane runs as the
   tmux *server* owner, the only way to get a perttu-authenticated agent out of
   this executor is for the pane's server to be perttu's.

### What already works (the bridge to reuse)

The interactive Terminal cockpit already drives perttu's tmux sessions *from the
chrote service*. The mechanism is `chrote-tmux-grants.sh`
(`/usr/local/libexec/chrote/chrote-tmux-grants.sh`, run as an `ExecStartPre=+`
root step). For each `owner=socket` mapping in `CHROTE_TERMINAL_USER_SOCKETS`,
running **as the socket owner (perttu, via `runuser`)** it:

- `setfacl` grants `chrote` traverse (`--x`) down `/run/user/1000/...` and `rw`
  on the socket file, plus a default ACL on the socket dir; and
- `tmux -S <socket> server-access -a chrote` grants chrote client access to
  perttu's tmux server.

Confirmed live: `getfacl /run/user/1000/chrote-tmux/tmux-1000` shows
`user:chrote:--x` (traverse) with a `default:user:chrote:rwx`; the cockpit
`default` socket exists and is perttu-owned. This is the exact bridge that lets a
chrote-hosted service send input into perttu-owned panes today. **The executor
can reuse it — the missing piece is a perttu-owned tmux server for the executor
to drive, plus reconciling which socket that is.**

### Why the executor's socket is currently wrong

Runtime dirs on the live host:

- `/run/chrote/formations-tmux/` — **chrote-owned, empty** (service runtime).
- `/run/chrote/tmux/tmux-999/` — chrote's own terminal server (uid 999).
- `/run/user/1000/chrote-formations-tmux/` — **perttu-owned, empty** (provisioned,
  unused).
- `/run/user/1000/chrote-tmux/tmux-1000/default` — perttu cockpit pool (live
  operator sessions).

`CHROTE_FORMATIONS_TMUX_SOCKET` is set in the root-owned `/etc/chrote/chrote-srv.env`
(mode `0600 root:chrote`, unreadable to this investigation and injected by systemd
as root — so the *service user never needs to read it*; that is not a blocker for
any option). Whatever value it holds, the failure mode is identical: if it points
at a **chrote-owned** socket, the executor's `ensureServer()` lazy-starts a
**chrote-owned** server (`StartKeeper` runs `tmux new-session` as the service
user), the pane runs as chrote, and `claude` cannot authenticate. That is the
last-mile gap.

### Deployed code state (relevant to every option)

The three "fix layers" named in the bead are on this branch's history:

- `3c19333 Let Formations tmux executor dispatch to the production socket`
- `8969b51 Share the cockpit tmux socket and spawn owned sessions on demand`
- `5b8c433 Support any tmux socket via lazy server start`

Their net effect in `tmux_executor.go` today:

- The executor **creates and kills its own ephemeral sessions** (`CreateSession`
  / `KillSession`, `ownedSessions` teardown). It only ever tears down names it
  recorded, and the client interface has **no** `kill-server`, `attach`,
  `rename`, or `resize` — so it cannot disrupt a foreign session by construction.
- The executor **lazy-starts a server** if none is running (`ensureServer` →
  `StartKeeper`, a `SessionPrefix+"keeper"` holding session running
  `exec sleep 2147483647` under `SHELL=/bin/bash`).
- The old temp-root / `PROD_SMOKE` / `DEDICATED` opt-in guard is **gone from
  production code** (only referenced in `tmux_executor_test.go`). Executor
  selection is now purely "are any `CHROTE_FORMATIONS_TMUX_HARNESSES` configured"
  (`NewConfiguredFormationExecutorFromEnv`).
- The `session_target_attachment_audit_unavailable` error still exists but is
  **repurposed**: `pinTmuxSocketIdentity` / `validatePinnedTmuxSocket` pin the
  socket's inode at run start and revalidate before every op, failing loud if the
  socket path/identity changes mid-run. It no longer refuses a cockpit socket.
- `RequireRuntimeAuthority()` now `return nil` (the "authority guard neutralized"
  layer, `runtime_authority.go`).

> **Cross-cutting flag (needs owner decision).** These layers moved *ahead of the
> root specs.* `FORMATIONS.md` §"Execution environments" still documents the
> superseded contract — that a configured cockpit target fails closed with
> `session_target_attachment_audit_unavailable`, that production Formations must
> consume the *same* Terminal session pool, and that a *"Formations-only
> production socket" must not be reintroduced* (ADR-0009). `ARCHON.md` still says
> "Schema-2 runtime remains disabled" and the runtime verbs are "non-authorizing,"
> which the neutralized authority guard contradicts. **Whichever executor option
> is chosen, `FORMATIONS.md` §Execution-environments and ADR-0007/0009 must be
> updated (a superseding ADR) to match the deployed reality, or the proving run
> is blocked on that reconciliation.** This is not optional cleanup: the accepted
> contract currently *forbids* the very socket topology the fix layers rely on.

---

## 2. Data ownership map (read-only, for the migration analysis)

`/srv/data/chrote` is `chrote:chrote`, setgid (`-s-`), with a default ACL
`default:user:chrote:rwx` + `default:group::rwx`. Because perttu is in the
`chrote` group, perttu already has group access to most subtrees:

| Subdir | Mode | perttu (as uid 1000) access today |
| --- | --- | --- |
| `.` (root), `workspace`, `evidence`, `assets`, `audits`, `qa`, `releases` | `drwxrwsr-x+` group `chrote` rwx | **rwx via group** |
| `agents` | `drwxrws---+` group rwx | rwx via group |
| `persistent-agents`, `scheduled-tasks`, `session-bank` | `drwxrwx---+` group rwx | rwx via group |
| `session-drops` | `drwx--x---+`, ACL `group::---`, only `user:tavern:--x` | **NONE** — no perttu entry, `other::---` |

So a service running as perttu would reach every state dir **except
`session-drops`** through the `chrote` group. `session-drops` is the one concrete
break. New files perttu writes inherit group `chrote` (setgid) and
`default:user:chrote:rwx`, so chrote-side readers keep working after a perttu
write. This makes the Option A migration far smaller than "re-chown everything."

Host-owned state paths (`CHROTE_SESSION_DROPS_DIR`, `CHROTE_SESSION_BANK_PATH`,
`CHROTE_PERSISTENT_AGENTS_PATH`, `CHROTE_SCHEDULED_TASKS_DIR`, etc.) are
configurable in the env; the migration can also point them at perttu-owned paths
instead of re-permissioning the chrote tree.

---

## 3. Options

### Option A — Run the whole `chrote-srv` service as `User=perttu`

Change the unit to `User=perttu`, `Group=perttu` (or keep `Group=chrote` to
preserve group reads of the data tree — recommended sub-choice), and let the
executor drive a perttu-owned socket natively. Two sub-variants for *which*
socket:

- **A1 — drive the shared cockpit pool** (`/run/user/1000/chrote-tmux/tmux-1000/default`).
  Closest to the `FORMATIONS.md` "same session pool as Terminal tabs" intent. But
  the executor creates/kills sessions *in the operator's live pool*; even though
  it only kills its own names, its churn shares a server with `Sol` and every real
  operator session. Golden-rule risk is highest here.
- **A2 — drive a dedicated perttu formations socket**
  (`/run/user/1000/chrote-formations-tmux/default`). Isolated from live sessions.
  This is a "Formations-only production socket," which the current accepted
  contract forbids (see cross-cutting flag).

**Exact steps (A, sketch):**
1. `Group=chrote` retained; add `SupplementaryGroups=` as needed. Set
   `User=perttu`.
2. Re-home `RuntimeDirectory` (currently `chrote` → `/run/chrote` owned by the
   service user). As perttu this becomes `/run/chrote` owned `perttu`. The
   `ExecStartPre` `install -o chrote -g chrote` grant lines and
   `chrote-tmux-grants.sh` (`SERVICE_USER=chrote`) must be re-pointed to
   `perttu`, OR made no-ops (perttu driving perttu's own sockets needs no ACL
   grant / `server-access`).
3. Grant perttu the one missing data dir: `setfacl -m u:perttu:rwx
   /srv/data/chrote/session-drops` (and its default ACL), **or** repoint
   `CHROTE_SESSION_DROPS_DIR` to a perttu-owned path. Audit the other
   `drwxrwx---`/`drwxrws---` dirs — group access covers them, but confirm no file
   *inside* is `chrote:chrote 0600` owner-only.
4. Point `CHROTE_FORMATIONS_TMUX_SOCKET` at the chosen perttu socket; ensure a
   perttu server is up (A1: the cockpit server already runs; A2: bootstrap one).
5. `systemctl daemon-reload`, restart the lane.

**Data-ownership plan:** minimal file moves (group already covers most);
`session-drops` is the lone exception. Prefer ACL-grant over chown to avoid
disturbing chrote-side readers. New writes stay group-`chrote` via setgid.

**Security-posture change (the real cost):** the network-facing HTTP service
(8095, and via Tailscale Serve) would run as **perttu — a member of `sudo` and
`docker`.** `docker` group membership is root-equivalent (trivial container
escape to host root). A compromise of the web surface today lands on an isolated
uid-999 account with no such powers; under Option A it lands on the owner
account. **This is the single biggest downside and the primary owner decision.**

**What breaks + fixes:**
- `chrote-tmux-grants.sh` and `CHROTE_TERMINAL_USER_SOCKETS` (grant chrote → perttu
  sockets) become self-grants / no-ops. Cross-user terminal tabs to *other* users
  (e.g. a `build` user) would now need perttu-side grants. → Re-parameterize
  `CHROTE_TMUX_GRANT_USER=perttu`.
- `session-drops` inaccessible → ACL grant or repoint (step 3).
- Any file the service *created as chrote* that is `0600 chrote:chrote` becomes
  read-only-denied to perttu → audit + widen (rare; most dirs are group-shared).
- The chrote user's own terminal server `/run/chrote/tmux/tmux-999` and RuntimeDir
  ownership semantics shift → verify Terminal tabs that target the service-user
  socket still resolve.

**Rollback:** revert the unit to `User=chrote` + `daemon-reload` + restart. ACL
*additions* (perttu grants) are harmless to leave or can be removed with
`setfacl -x`. No data was moved if the ACL-grant path was taken, so rollback is
config-only and fast. **Live tmux sessions are never touched by an A rollback.**

**Risk to live tmux:** A1 shares the operator pool → highest. A2 isolated →
low. Service restart itself: `KillMode=control-group` means restarting the
service kills the service cgroup, but the operator's tmux servers are **not** in
the service cgroup (they are perttu-login / `--user` scoped), so a restart does
not kill live agents — confirmed by the existing lane's behavior. Verify anyway.

**Verify:** `id` of the running service (`systemctl show -p MainPID` →
`/proc/<pid>/status` Uid); launch a smoke agent and confirm the pane process is
perttu and `claude` authenticates; confirm `session-drops`-backed features work;
confirm live cockpit sessions untouched (`tmux -S .../default list-sessions`
before/after).

---

### Option B — Keep `chrote-srv` as `chrote`; run only a dedicated **perttu-owned formations tmux server** that chrote drives (RECOMMENDED)

Do **not** move the network service. Leave the HTTP surface on the isolated
uid-999 `chrote` user. Instead, run one thing as perttu: a **dedicated formations
tmux server** on `/run/user/1000/chrote-formations-tmux/default` (the
already-provisioned perttu dir), supervised by a perttu `systemd --user` unit —
mirroring the existing `chrote-tmux-watchdog-guard.service` that already keeps the
cockpit server alive (perttu has `Linger=yes`, confirmed). Grant chrote
`server-access` + socket ACL to it using the **already-deployed
`chrote-tmux-grants.sh` mechanism** (it already handles arbitrary
`owner=/run/user/<uid>/...` sockets). Point `CHROTE_FORMATIONS_TMUX_SOCKET` at it
and configure `CHROTE_FORMATIONS_TMUX_HARNESSES`.

The executor (still chrote) then does exactly what the Terminal cockpit already
does — issues tmux commands to a perttu server it has been granted access to —
and every session it creates has a **perttu-owned pane**, so `claude`
authenticates. The delegation boundary is the tmux `server-access` surface, which
is already trusted and in production for the cockpit.

**Why this is the recommendation:**
- **Security posture preserved.** The internet-reachable service stays uid-999,
  no `sudo`, no `docker`. This is the property Option A gives up.
- **Marginal privilege cost ≈ zero.** chrote *already* has `server-access` to
  perttu's cockpit tmux, rwx on perttu's tmux socket dirs, and read/traverse into
  `/home/perttu` (getfacl: `user:chrote:rwx` effective on `/home/perttu`). Adding
  a second perttu server chrote may drive does not expand chrote's reach.
- **Zero data migration.** `/srv/data/chrote` stays chrote-owned; no
  `session-drops` problem.
- **Golden rule honored.** Executor churn lives on a *dedicated* formations
  server, isolated from the operator's live cockpit `Sol` sessions.
- **Reuses proven, deployed plumbing** (`chrote-tmux-grants.sh`, the exact
  cockpit bridge).

**The delegation boundary, precisely:** chrote → perttu is the tmux
`server-access` ACL on one socket. chrote can `new-session`, `send-keys`,
`load-buffer`, `paste-buffer`, `capture-pane`, `list-sessions`, `kill-session`
(its own names) on that server. It cannot `kill-server`/`attach`/`rename`/`resize`
(the client interface omits them). Panes run as perttu with perttu's HOME and
credentials.

**Exact steps (B):**
1. Author a perttu `systemd --user` unit — `chrote-formations-tmux.service` —
   `Type=forking`/`oneshot` that starts the server + a keeper:
   `tmux -S /run/user/1000/chrote-formations-tmux/default new-session -d -s
   <prefix>keeper 'exec sleep 2147483647'` under `SHELL=/bin/bash`, with a
   `Restart=`/watchdog sibling like `chrote-tmux-watchdog-guard`. `enable
   --now`; linger already on.
2. Add `perttu=/run/user/1000/chrote-formations-tmux/default` to the grants
   mechanism (extend `CHROTE_TERMINAL_USER_SOCKETS` semantics or add a
   formations-specific mapping) so the root `ExecStartPre` grants chrote
   `server-access` + socket rw. Runs as perttu via `runuser`; no chrote-side
   privilege needed.
3. Set `CHROTE_FORMATIONS_TMUX_SOCKET` to that path,
   `CHROTE_FORMATIONS_TMUX_HARNESSES=<claude variants>`,
   `CHROTE_FORMATIONS_TMUX_CWD` + `..._ROOTS` to a shared workspace both users can
   reach (e.g. `/srv/data/chrote/workspace`, group-writable + default
   `user:chrote:rwx`), and `CHROTE_FORMATIONS_TMUX_SESSION_PREFIX`.
4. Restart the chrote-srv lane. No data move.

**The one code-level gotcha to close (must-fix for B):** the executor's
`ensureServer()` lazy-start runs `StartKeeper` **as chrote**. If the perttu
formations server is ever *down* when the executor touches the socket, tmux will
find a stale/absent socket and start a **chrote-owned** server at that path —
silently reverting agents to chrote and defeating the whole design. Mitigations,
in order of strength:
   - (a) The perttu `--user` watchdog keeps the server up (necessary but not
     sufficient against a race).
   - (b) **Add an ownership assertion to the executor**: before use, `stat` the
     socket and refuse (fail loud) if its owner uid ≠ the expected agent uid.
     This is a small, high-value hardening — recommend filing it as
     discovered-from `chrote-jkk`. It converts a silent-wrong-identity failure
     into a loud, safe one.
   - (c) Alternatively, run the keeper start itself through the grant path so a
     missing server is (re)started as perttu, not chrote.

**Data-ownership plan:** none for the service. Only the shared workspace/roots
need to be reachable by both uids — `/srv/data/chrote/workspace` already is
(group + default ACL). The agent writes deliverables as perttu (group `chrote`
via setgid); chrote hydrates artifact refs by reading them via group. Verify the
roots chosen satisfy both.

**Rollback:** clear `CHROTE_FORMATIONS_TMUX_*` (or repoint to the lab executor),
`systemctl --user disable --now chrote-formations-tmux.service`, remove the grant
mapping, restart the lane. The perttu formations server is *separate* from the
operator cockpit server, so tearing it down **cannot touch live operator
sessions.** Config-only; seconds to revert.

**Risk to live tmux:** lowest of all options — a dedicated server, never shared
with the operator pool, and the executor's client interface cannot issue
disruptive verbs. The residual risk is the lazy-start-as-chrote race (mitigated
above).

**Verify:** perttu server up (`systemctl --user status`); chrote can
`tmux -S <sock> list-sessions` (grant works); launch a smoke agent, confirm pane
owner is perttu and `claude` authenticates; confirm the operator cockpit server
is a *different* server (different socket, unaffected); kill/restart the perttu
formations server and confirm the executor fails **loud** (identity pin), not
silently as chrote.

---

### Option C — Perttu-side launch helper (RPC delegation)

A small perttu `--user` service exposes a localhost/Unix-socket RPC
("launch agent <card> in <workspace> on session <name>", "send prompt",
"capture", "teardown"); the chrote service calls it instead of driving tmux
directly. The helper does all tmux work as perttu.

- **Pro:** the cleanest *conceptual* boundary (a typed contract, not raw tmux
  ACLs); the helper can enforce policy (allowed workspaces, session naming) and
  own the ADR-0007 attachment/mutation/closure journal that the accepted contract
  actually asks for.
- **Con:** most new code and moving parts — a new service, a wire protocol, auth
  between the two daemons, and a re-plumb of the executor away from
  `realTmuxHarnessClient` to an RPC client. It duplicates what `server-access`
  already gives Option B for free.
- **When to choose it:** if the owner wants to *satisfy* (not supersede)
  ADR-0007's journal requirement and treat the executor→perttu boundary as a
  first-class, auditable contract for a future multi-user posture. Otherwise it
  is over-engineering relative to B.

---

## 4. Recommendation

**Recommend Option B** (dedicated perttu-owned formations tmux server, driven by
the unchanged chrote service through the already-deployed grant bridge), with the
executor ownership-assertion hardening (§3B gotcha, mitigation b) as a required
companion fix.

Reasoning: it delivers the required outcome — agents launch **authenticated as
perttu** and the tmux drive works — while (1) keeping the network-facing service
on the isolated uid-999 user, avoiding the `sudo`/`docker` blast-radius
escalation that is Option A's defining cost; (2) requiring **zero data-ownership
migration**; (3) keeping executor session churn off the operator's live cockpit
pool (golden rule); and (4) reusing plumbing already proven in production for the
Terminal cockpit. Its marginal privilege grant to chrote is ~nil because chrote
already drives perttu tmux for the cockpit.

Option A is the owner's original lean (the bead frames it as the service-user
change). It is conceptually simplest and its data migration turns out small
(group access already covers all but `session-drops`), but it trades away the
core reason `chrote` exists — an isolated, unprivileged identity for the
network-facing surface. **If the owner values uniform identity / accepts running
the web surface as a `sudo`+`docker` account, A2 (dedicated socket) is the
fallback; A1 (shared pool) should be avoided for golden-rule reasons.**

**Owner decisions required:**
1. **Security posture vs. uniformity:** accept Option A's web-surface-as-perttu
   (`sudo`+`docker`) escalation, or take Option B's narrow split. *(Recommend B.)*
2. **Contract reconciliation:** both B and A2 introduce/keep a "Formations-only
   production socket," which `FORMATIONS.md` §Execution-environments and ADR-0009
   currently forbid. This needs a **superseding ADR** (and doc update) before the
   proving run — or explicit acknowledgment that the deployed fix layers already
   broke that contract and the docs are simply catching up. Either way the owner
   must bless the topology.
3. **Executor ownership assertion:** approve adding a fail-loud "socket owner must
   be the expected agent uid" check (converts silent-wrong-identity into a safe
   stop). *(Recommend yes; file discovered-from `chrote-jkk`.)*

---

## 5. Staged, reversible cutover plan (for the recommended Option B)

Each stage is independently reversible and ends in a verification gate. Nothing
touches live operator sessions.

**Stage 0 — Pre-flight (read-only).** Snapshot current live cockpit sessions
(`tmux -S /run/user/1000/chrote-tmux/tmux-1000/default list-sessions`), record
the running service uid, and capture the current `CHROTE_FORMATIONS_TMUX_*` env
values for rollback. Reconcile the doc contract (owner decision #2) — land the
superseding ADR *or* explicit sign-off first. *No system change.*

**Stage 1 — Perttu formations server (additive, isolated).** Install + `enable
--now` the perttu `--user` `chrote-formations-tmux.service` + watchdog. Verify the
server + keeper are up on the dedicated socket and that it is a *distinct* server
from the cockpit pool. Rollback: `disable --now`. *Cannot affect operator
sessions — different server.*

**Stage 2 — Grant + code hardening.** Land the executor ownership-assertion
(mitigation b) in a normal build/deploy of the chrote binary (its own test
gate). Add the perttu formations socket to the grant mapping; confirm chrote can
`list-sessions` on the perttu server. Rollback: remove mapping / revert binary.

**Stage 3 — Point the executor at the perttu socket.** Set
`CHROTE_FORMATIONS_TMUX_SOCKET` + `..._HARNESSES` + `..._CWD`/`..._ROOTS` +
`..._SESSION_PREFIX`; restart the chrote-srv lane. Verify: launch a single smoke
agent through Formations, confirm the pane owner is perttu, `claude`
authenticates, `chrote-outputs` + `CHROTE-DONE` sentinel round-trip, and the
operator cockpit sessions are untouched. Rollback: clear the env vars (fall back
to lab executor or unavailable), restart. *Config-only.*

**Stage 4 — Fault-injection verification.** Kill the perttu formations server
mid-idle and confirm the executor fails **loud** (identity pin / ownership
assertion) rather than silently lazy-starting a chrote server; confirm the
watchdog restarts it; confirm a mid-run server restart aborts the run cleanly
(fail-loud ledger event) without disrupting anything else. Only after this passes
is the executor considered production-ready for the proving missions
(`chrote-3af`).

Throughout: the operator's live cockpit pool is never a target of any executor or
cutover step. Any anomaly → Stage-N rollback (all config- or additive-service
level; no data was moved).
