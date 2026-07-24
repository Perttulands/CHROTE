# Spec: Grab the wheel — peek and steer a live agent session from the board

- Bead: chrote-a7k (parent epic chrote-z0q)
- Date: 2026-07-24
- Branch/worktree: feat/formations-peek @ -worktrees/peek
- Status: design → RED → GREEN (SDD)

## 1. Feature

Functionality 6 of the "usable solo agent-workflow cockpit" epic. From the Formations
board, the operator opens a **running** node's run-evidence inspector and clicks **Peek /
grab the wheel**. CHROTE attaches interactively to that node's live agent tmux session:
the operator watches the agent work, can take the keyboard to steer, and hands control
back by closing the window. Cooperative-correctness for trusted agents: attached/live
state is surfaced loudly; peek never kills, renames, resizes, or steals a session, and
never touches any session other than the one node's own.

## 2. The hard constraint: ephemeral sessions

Formations runs execute on the **shared cockpit tmux pool**
(`CHROTE_FORMATIONS_TMUX_SOCKET`). The tmux executor
(`src/internal/formations/tmux_executor.go`) spawns its **own** uniquely-named session
per slot on demand — `provisionOwnedSession` creates
`<SessionPrefix><runId>-<slotId>-<nonce>` via `CreateSession`, runs the step, and
`teardownOwnedSessions` **kills it** when the run/step finishes. The session therefore
exists **only while that step is executing**. There is nothing to attach to before the
step starts or after it is torn down.

Consequences the design must honor:

- Peek is a **live window during execution only**. It is not history and not replay.
- "Available" must mean *actually attachable right now*, not merely "this node ran once".
- After teardown, an attach attempt must **fail loud and honest** ("session ended"),
  never silently reconnect-loop onto a dead name or a foreign session that later reuses
  the name (the nonce makes reuse astronomically unlikely, but honesty is the rule).

## 3. What already exists (reuse, do not rebuild)

The whole attach path is already in the product. Peek is wiring, not new terminal
infrastructure.

- **Session name is already on the wire.** Every dispatch records
  `sessionRef: "tmux:<sessionName>"` in the `slot_dispatch` run event
  (`run_dispatch.go:88`, produced at `tmux_executor.go:740`). `GET
  /api/formations/runs/{id}/events` returns it verbatim (no redaction —
  `redaction.go` only scrubs credential-shaped text and does not run on event output).
- **Frontend already parses and renders it.** `projectNodeEvidence`
  (`dashboard/src/components/formationsRunState.ts:186`) exposes
  `NodeEvidenceDispatch.sessionRef` per attempt/dispatch, already displayed in the
  inspector at `FormationsCockpit.tsx:2171` (`.node-dispatch-session`).
- **Per-node running state is already derived.** `projectNodeStates`
  (`formationsRunState.ts:84`) maps `node_started` / `slot_dispatch` /
  `gate_evaluating` → `'running'`; a terminal `node_output` / `gate_verdict` flips it to
  `done`/`failed`. `NodeEvidence.state === 'running'` is the honest "executing now"
  signal.
- **The interactive terminal already exists.** `FloatingModal.tsx` renders a draggable,
  resizable terminal iframe for a session via `/terminal/?arg=<session>[&arg=<user>]`,
  opened through `openFloatingModal(sessionName)` on the shared session context. The
  ttyd proxy (`src/internal/proxy/terminal.go`) plus `terminal-launch.sh` do the actual
  `tmux -S <socket> attach-session -t <session>`, and `terminal-launch.sh:74-79`
  **already fails loud** when the session is not on the socket. This attach is a real
  interactive attach (ttyd `-W`), so "watch" and "take the keyboard" are the *same*
  attached terminal — steering is just typing.
- **The board already holds the context.** `FormationsCockpit` already calls
  `useSessionOptional()` (`FormationsCockpit.tsx:126`), so `openFloatingModal` and the
  live `sessions` registry are already reachable from the inspector.

### Socket resolution (load-bearing)

A mission session lives on `CHROTE_FORMATIONS_TMUX_SOCKET`. The terminal attach resolves
its socket like this:

- No `unixUser` arg → `terminal-launch.sh` attaches on `CHROTE_TMUX_SOCKET`, which the
  proxy sets from `CHROTE_DEFAULT_TMUX_SOCKET` (`terminal.go:70,84`).
- With a `unixUser` arg → per-user socket from `CHROTE_TERMINAL_USER_SOCKETS`.

In the **shared cockpit production topology these point at the same socket** (Formations
and the Terminal tab share the cockpit pool — see chrote-ui-work). Therefore, while a
step runs, its session is enumerated by `GET /api/tmux/sessions` on the default socket
(`tmux.go:1338` lists the default target with `UnixUser=""`), so it appears in the
frontend `sessions` registry with a bare key. `openFloatingModal(<sessionName>)` then
resolves the correct user automatically (`FloatingModal.tsx:144-150`) and attaches to the
right socket. **No new backend, no new socket plumbing.**

This alignment (`CHROTE_DEFAULT_TMUX_SOCKET` == `CHROTE_FORMATIONS_TMUX_SOCKET`, or a
matching `CHROTE_TERMINAL_USER_SOCKETS` entry) is a **deployment invariant peek relies
on**. It is the one thing that genuinely needs a live run to confirm end to end (§9).

## 4. Design

### 4.1 The honest gate

Peek is offered from the node inspector only when it is **truly attachable**:

```
peekTarget(evidence, liveSessionNames) =
  evidence.state === 'running'
  AND latest dispatch has a non-empty sessionRef (strip "tmux:")
  AND that sessionName is currently in the live sessions registry
  → { sessionName }   otherwise null
```

Both truth signals must hold:

1. `state === 'running'` — the step is executing (from the event stream).
2. the exact session name is present in `sessions` — there is a live session to attach
   to *right now*.

This is a pure function, `peekTargetForEvidence(evidence, liveSessionNames)`, added next
to `projectNodeEvidence` in `formationsRunState.ts`. It picks the **latest attempt's
latest dispatch** that carries a sessionRef (so a re-dispatched/retried node peeks the
current session, not a stale one).

### 4.2 Lifecycle-honest UX (before / during / after)

Inside the existing run-evidence inspector (`data-testid="node-inspector"`), add a small
monochrome **Peek** region, rendered only while `evidence.state === 'running'`:

- **Before the step / node not running:** no peek region at all (the inspector is
  history-only, as today).
- **During the step, session live (peekTarget non-null):** an enabled **Peek / grab the
  wheel** button plus one honest caption: *"Attaches to the live agent session. Type to
  steer; close to hand back. It ends when the step finishes."* Clicking calls
  `session.openFloatingModal(peekTarget.sessionName)` → the existing FloatingModal
  attaches interactively.
- **During the step, session not (yet/no longer) attachable (running but peekTarget
  null):** a disabled button + caption *"Live session isn't attachable right now."* This
  covers the poll-lag windows at both ends of the ephemeral lifecycle without lying.
- **After teardown while a peek window is open:** the attach drops when tmux tears the
  session down; ttyd closes the socket and the FloatingModal shows the terminal ending.
  We do **not** add reconnect logic for peek — a dropped attach means the step ended,
  which is the truth. (FloatingModal already prunes `floatingSession` when the session
  leaves the registry — `SessionContext.tsx:773`.)

### 4.3 Cooperative / grab-the-wheel semantics

- The attach is **cooperative by construction**: tmux attach shares the agent's own
  pane. The executor keeps driving that same pane with `send-keys`/`capture-pane`; the
  operator's keystrokes go to the **same one agent**. That coexistence *is* grab-the-
  wheel — the operator intervenes in a real decision on that agent, then detaches.
- **Safety invariant (absolute):** peek issues **no lifecycle operations** — no
  new-session, kill, kill-server, rename, resize, or respawn. It only opens an attach to
  one named session and closes it. It can never touch a sibling or foreign session. This
  mirrors the executor's own narrow surface (`tmux_executor.go:77-95`), which also has no
  attach/kill-server/rename.
- **Hand back** = close the FloatingModal (detach). The tmux session keeps running under
  the executor; nothing is killed.
- **Known, accepted trade-off (trusted solo model):** because the pane is shared,
  operator input can change what the agent does mid-step (that is the point) and could in
  principle perturb the executor's completion-sentinel wait. The completion sentinel is a
  specific `<<<CHROTE-DONE run-id=...>>>` line no operator types by accident; steering is
  the operator's informed choice. Documented, not guarded against — guarding would defeat
  the feature.

### 4.4 Visual language

Monochrome, per CHROTE house style (no circus colors, no LEDs). Reuse existing inspector
tokens (`--ink2`, `--dim`, `--dimmer`, `--line`, `--paper2`); the button uses the same
restrained treatment as other inspector controls. Attached/live state is conveyed as
**text**, not a colored dot. One calm accent (`--terra`) at most, consistent with the
existing `.pop-body` focus treatment.

## 5. No backend change (justification)

Everything peek needs is already served: the session name (`sessionRef` on
`slot_dispatch` events), the running state (event stream), the live-session registry
(`/api/tmux/sessions`), and the attach path (ttyd proxy + `terminal-launch.sh`). Adding a
backend endpoint or a new projection field would duplicate data the API already returns.
Peek is therefore **frontend-only**: a pure gate helper, an inspector control, and CSS.
If §9 live verification shows the mission session is *not* enumerable with a bare key
under some deployment shape, the smallest follow-up is to teach the peek open path an
explicit `{name, unixUser}` attach target rather than to add backend plumbing — tracked
as discovered work if it materializes.

## 6. Files to change (GREEN)

- `dashboard/src/components/formationsRunState.ts` — add `peekTargetForEvidence` (pure).
- `dashboard/src/components/FormationsCockpit.tsx` — in the node inspector, compute the
  live-session set from `session?.sessions`, compute `peekTarget`, render the Peek region
  and wire the click to `session.openFloatingModal(peekTarget.sessionName)`.
- `dashboard/src/styles/formations-d7.css` — `.node-peek` region styling (monochrome).

Test IDs: `peek-node-<nodeId>` (button), `node-peek` (region). Copy contains "grab the
wheel".

## 7. Test plan (RED first)

Unit (`formationsPeek.test.ts`), pure `peekTargetForEvidence`:

- running node + latest dispatch `sessionRef: "tmux:mission-x"` + `mission-x` in live set
  → returns `{ sessionName: "mission-x" }`.
- running but session **not** in live set → null (ephemeral: torn down / not yet seen).
- `state !== 'running'` (done/failed/'') even with a sessionRef in the live set → null.
- running but no dispatch/sessionRef → null.
- multiple attempts → picks the **latest** attempt's session name.
- strips the `tmux:` prefix; a bare/blank sessionRef → null.

Component (`FormationsCockpit.peek.test.tsx`, mocks `useSessionOptional`):

- running node (node_started + slot_dispatch with `sessionRef`, no node_output) with the
  session in the mocked registry → inspector shows an **enabled** `peek-node-*` button;
  clicking calls `openFloatingModal("mission-...")` (prefix stripped).
- finished node (node_output present) → **no** peek button.
- running but session absent from registry → button present but **disabled**, honest
  caption shown.

Playwright (`dashboard/tests/formations/`), if it fits the existing mock-api fixture:
drive a run whose node is running with a live session, open the inspector, click peek,
assert a floating terminal iframe whose `src` contains `/terminal/?arg=mission-...`
appears. Added only if the fixture supports run events + `/api/tmux/sessions` without new
harness scaffolding; otherwise the unit + component RED tests carry the behavior and the
end-to-end attach is covered by §9 live verification.

## 8. Gates (before done)

- `cd dashboard && npm run lint && npx vitest run && npm run build`
- Formations Playwright suite (`PATH=/usr/local/go/bin:$PATH`).
- No Go touched (frontend-only); if that changes: `cd src && go test ./... && go vet ./...
  && gofmt -l .` empty.
- `git diff --check` clean.

## 9. Needs a live run to fully verify

- **Socket alignment (§3):** confirm a running mission session is enumerated by `GET
  /api/tmux/sessions` and that `/terminal/?arg=<missionSession>` attaches to it on the
  live cockpit lane. This is the one behavior that cannot be proven from unit/component
  tests because it depends on deployment env (`CHROTE_DEFAULT_TMUX_SOCKET` ==
  `CHROTE_FORMATIONS_TMUX_SOCKET`).
- **Teardown drop:** confirm that when the step finishes, an open peek window shows the
  terminal ending rather than reconnect-looping.

## 10. Out of scope (follow-ups)

- Free-tiles / multi-session simultaneous focus (multiple peeks at once).
- Peeking historical/torn-down sessions (would require a durable transcript capture, a
  different feature).
- Any executor/lifecycle change — the executor is deliberately untouched.

## Notes / drift found

`.env.example:82-90` still describes the tmux executor as one that "never creates or kills
sessions" and requires pre-existing sessions named `SESSION_PREFIX + sessionStem`. The
code now spawns and tears down on-demand `<prefix><runId>-<slotId>-<nonce>` sessions
(`tmux_executor.go:818-851`, `teardownOwnedSessions`). Stale doc, not in this bead's
scope — flagged for a docs follow-up.
