# Gas City Codex Parity — Smoke Evidence and Safety Posture

This document preserves the Codex Gas City parity evidence: the first paid,
credentialed harness built on the reusable adapter mold from `home-7jk3`. It is
the durable record for Beads `home-piis`. The shared mold contract and the
OpenCode reference live in `README.md` and `docs/gascity-opencode-parity.md`.

## Codex parity smoke (2026-05-27)

Outcome: **Codex runs as a valid Gas City-owned identity and returns mail under
an immutable session id, with no sender spoofing, read-only and sandboxed.**

- Environment: `codex` resolves at `/home/linuxbrew/.linuxbrew/bin/codex`;
  `codex --version` = `codex-cli 0.130.0`. Supervisor up on `127.0.0.1:8372`
  (localhost-only, unchanged; PID 3826452).
- Auth: `codex login status` = "Logged in using ChatGPT". A minimal read-only
  `codex exec` probe returned a model response (exit 0), so **Codex CLI auth
  works** — the `home-8y8` HTTP 401 (recorded 2026-05-23) no longer reproduces.
  No credential values were read, printed, or stored.
- Safe launch: the wrapper drives `codex exec` with
  `--sandbox read-only --skip-git-repo-check --ephemeral --ignore-user-config
  --color never -C <workdir> -o <last-file>`, stdin from `/dev/null`. It does
  **not** use `--dangerously-bypass-approvals-and-sandbox` or any full-access /
  bypass flag. `--ignore-user-config` scrubs ambient `~/.codex/config.toml`
  (model/effort/project state) so the smoke is self-contained; auth still uses
  `CODEX_HOME`. The workdir carries an `AGENTS.md` that instructs the agent it
  is read-only and **denied** edit/shell/network/tool use (Codex reads and
  honors `AGENTS.md`; verified by probe), layered on top of the sandbox.
- Identity / mail:
  - Disposable Gas City session **`gc-56218`** (template `codex-smoke`) ran the
    wrapper. Gas City set the immutable `GC_SESSION_ID`; the wrapper's identity
    gate (exit 64 if unset) means mail could only be sent because a real session
    id was present.
  - Codex produced a two-line poem containing the exact nonce
    `CX-GC-20260527-120737`; wrapper recorded `codex_status=0`.
  - The wrapper sent mail via `gc mail send human --from "$GC_SESSION_ID"`,
    producing mail **`gc-56220`** (`issue_type=message`, `From: codex-smoke`,
    `To: human`).
  - Authoritative identity check from the file-backed bead store
    (`.gc/beads.json`, provider = `file`): the `gc-56220` metadata is
    `{"mail.from_display":"codex-smoke","mail.from_session_id":"gc-56218"}`.
    **`mail.from_session_id = gc-56218`** confirms the sender identity came from
    the real Gas City session, not a spoofed `GC_ALIAS`.
- Cleanup: the disposable session `gc-56218` was unpinned and closed. The live
  registry retains only the intended `codex-smoke` agent template (one added
  agent; pre-existing `dog`, `opencode-smoke`, `pi-smoke`, `planner`,
  `reviewer-a`, `reviewer-b` and their sessions unchanged). Runtime run-state
  markers under `.gc/codex-smoke-runs/` are not source and are left in place
  (analogous to `opencode-smoke-runs`).

### How to read `mail.from_session_id`

This city's beads provider is `file` (`city.toml [beads] provider = "file"`),
so mail is **not** in the embedded Dolt databases (those are unused leftovers
for this provider). `gc mail` CLI commands do not expose `from_session_id`, and
the supervisor buffers the store in memory. The authoritative record is the
flushed file store:

```bash
python3 - <<'PY'
import json
d = json.load(open('/home/perttu/gascity/.gc/beads.json'))
b = next(x for x in (d['beads'].values() if isinstance(d['beads'], dict) else d['beads'])
         if x.get('id') == 'gc-56220')
print(b['issue_type'], b['from'], b['metadata'])
PY
# -> message codex-smoke {'mail.from_display': 'codex-smoke', 'mail.from_session_id': 'gc-56218'}
```

## Safety posture (home-4xv.5 boundary)

- **Immutable session id required** — wrapper refuses to mail without
  `GC_SESSION_ID` (exit 64); `GC_ALIAS` alone cannot spoof a sender.
- **Tool default-deny** — enforced by `codex exec --sandbox read-only` (no
  writes/network; model-run shell is sandboxed read-only) plus the `AGENTS.md`
  deny instructions.
- **No dangerous-skip flags** — the wrapper never uses
  `--dangerously-bypass-approvals-and-sandbox` / `danger-full-access` / `--yolo`.
  `install-adapter` enforces the `forbidden_substrings` list and refuses to
  write if any appears.
- **Env scrubbed** — `--ignore-user-config` keeps the run self-contained; no
  credential values appear in logs, docs, beads, run-state, or diffs.

The `install-adapter` safety gate was exercised both positively (codex-smoke
cleared 4 forbidden-substring checks + the session-id and tool-deny invariants)
and negatively (a throwaway adapter containing `--dangerously-bypass` and no
session gate was refused with violations before any file was written, target
city left empty).

## Adapter source and install

Package: `chrote/docs/gascity-harness-adapters/` (this repo).

| tracked source                                          | live city dest                                  |
|---------------------------------------------------------|-------------------------------------------------|
| `adapters/codex-smoke/bin/codex-gc-agent`               | `/home/perttu/gascity/bin/codex-gc-agent`       |
| `adapters/codex-smoke/agent/agent.toml`                 | `/home/perttu/gascity/agents/codex-smoke/agent.toml` |
| `adapters/codex-smoke/workdir/AGENTS.md`                | `/home/perttu/gascity/codex-smoke/AGENTS.md`    |

```bash
PKG=/home/perttu/chrote/docs/gascity-harness-adapters
"$PKG/bin/install-adapter" codex-smoke            # sync into live city ($GC_CITY|/home/perttu/gascity)
"$PKG/bin/install-adapter" codex-smoke --verify   # confirm city matches source
gc --city /home/perttu/gascity reload
gc config explain --agent codex-smoke

# One disposable smoke under a real Gas City identity:
gc session new codex-smoke --alias codex-smoke --title "Codex smoke" --no-attach
gc session pin codex-smoke && gc session wake codex-smoke
# Gas City sets GC_SESSION_ID; the wrapper runs `codex exec --sandbox read-only`
# once and mails human the poem from the session identity.
gc mail inbox human
gc session unpin codex-smoke && gc session close codex-smoke   # disposable
```
