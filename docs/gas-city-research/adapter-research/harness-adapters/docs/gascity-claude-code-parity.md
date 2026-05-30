# Gas City Claude Code Parity — Smoke Evidence and Safety Posture

This document preserves the Claude Code Gas City parity evidence: the second
paid, credentialed harness built on the reusable adapter mold from `home-7jk3`,
after Codex (`home-piis`). It is the durable record for Beads `home-5v4k`. The
shared mold contract and the other references live in `README.md`,
`gascity-opencode-parity.md`, and `gascity-codex-parity.md`.

## Claude Code parity smoke (2026-05-27)

Outcome: **Claude Code runs as a valid Gas City-owned identity and returns mail
under an immutable session id, with no sender spoofing, read-only and
no-edit.**

- Environment: `claude` resolves at `/home/perttu/.local/bin/claude`;
  `claude --version` = `2.1.152 (Claude Code)`. Supervisor up on
  `127.0.0.1` (localhost-only, unchanged; PID 3826452 = `gc supervisor run`).
- Auth: `claude auth status` reports `loggedIn: true`, `authMethod: claude.ai`,
  `apiProvider: firstParty`, subscription `max`. No credential values were read,
  printed, or stored; `~/.claude/.credentials.json` (mode 600) was never opened.
- Safe launch: the wrapper drives Claude Code non-interactively in print mode:
  the prompt is piped on **stdin** to `claude -p` with
  `--tools Read,Grep,Glob` (the enforced read-only built-in-set allowlist),
  a belt-and-braces `--disallowedTools Bash,Edit,Write,WebFetch,WebSearch,Task,
  TodoWrite,<mcp OAuth stubs>`, `--permission-mode default`,
  `--no-session-persistence` (disposable), `--setting-sources ""` (no
  user/project settings), and `--add-dir <workdir>`. The prompt is piped on
  stdin because Claude Code's variadic flags would otherwise swallow a
  positional prompt. It does **not** use either Claude Code permission-bypass
  flag and does **not** use the permission mode that bypasses checks. The
  wrapper scrubs `CLAUDE_CODE_*` env vars so a nested launch never inherits a
  parent Claude session's runtime. The workdir carries a `CLAUDE.md` that
  instructs the agent it is read-only and denied edit/shell/network/sub-task use,
  layered on top of the allowlist.
- Read-only enforcement was probe-verified before the smoke: with
  `--tools Read,Grep,Glob`, a prompt asking Claude to create a file reported
  `BLOCKED` ("the Write tool exists but is not enabled here") and **no file was
  created** (canary check passed).
- Identity / mail:
  - Disposable Gas City session **`gc-56329`** (template `claude-code`) ran the
    wrapper. Gas City set the immutable `GC_SESSION_ID`; the wrapper's identity
    gate (exit 64 if unset) means mail could only be sent because a real session
    id was present.
  - Claude Code produced a two-line poem containing the exact nonce
    `CC-GC-20260527-123127`; wrapper recorded `claude_status=0`.
  - The wrapper sent mail via `gc mail send human --from "$GC_SESSION_ID"`,
    producing mail **`gc-56330`** (`issue_type=message`, `From: claude-code`,
    `To: human`).
  - Authoritative identity check from the file-backed bead store
    (`.gc/beads.json`, provider = `file`): the `gc-56330` metadata is
    `{"mail.from_display":"claude-code","mail.from_session_id":"gc-56329"}`.
    **`mail.from_session_id = gc-56329`** confirms the sender identity came from
    the real Gas City session, not a spoofed `GC_ALIAS`.
- Cleanup: the disposable session `gc-56329` was unpinned and closed. The live
  registry retains only the intended `claude-code` agent template (one added
  agent; pre-existing `dog`, `codex-smoke`, `opencode-smoke`, `pi-smoke`,
  `planner`, `reviewer-a`, `reviewer-b` and their sessions unchanged). Runtime
  run-state markers under `.gc/claude-code-runs/` are not source and are left in
  place (analogous to `opencode-smoke-runs` / `codex-smoke-runs`).

### How to read `mail.from_session_id`

This city's beads provider is `file` (`city.toml [beads] provider = "file"`),
so mail is **not** in the embedded Dolt databases (those are unused leftovers
for this provider). `gc mail` CLI commands do not expose `from_session_id`. The
authoritative record is the flushed file store:

```bash
python3 - <<'PY'
import json
d = json.load(open('/home/perttu/gascity/.gc/beads.json'))
b = next(x for x in (d['beads'].values() if isinstance(d['beads'], dict) else d['beads'])
         if x.get('id') == 'gc-56330')
print(b['issue_type'], b['from'], b['metadata'])
PY
# -> message claude-code {'mail.from_display': 'claude-code', 'mail.from_session_id': 'gc-56329'}
```

## Safety posture (home-4xv.5 boundary)

- **Immutable session id required** — wrapper refuses to mail without
  `GC_SESSION_ID` (exit 64); `GC_ALIAS` alone cannot spoof a sender.
- **Tool default-deny (`tool_deny_mechanism = "config"`)** — enforced by the
  built-in-set allowlist `--tools Read,Grep,Glob` in the wrapper: Bash, Edit,
  Write, WebFetch, WebSearch, Task, and TodoWrite are not in the allowed set, so
  the model cannot mutate, run shell, or hit the web. `--disallowedTools` and
  the workdir `CLAUDE.md` are belt-and-braces on top.
- **No dangerous-skip flags** — the wrapper never uses
  `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions`, the
  bypass permission mode, or `--yolo`. `install-adapter` enforces the
  `forbidden_substrings` list (which for this adapter explicitly includes both
  Claude Code permission-bypass flags and the bypass mode) and refuses to write
  if any appears.
- **Env scrubbed** — `--setting-sources ""` and the `CLAUDE_CODE_*` env scrub
  keep the run self-contained; no credential values appear in logs, docs, beads,
  run-state, or diffs.

The `install-adapter` safety gate was exercised both positively and negatively,
and — per the `home-5v4k` honesty requirement — the pass is **non-incidental**:

- Positive: `claude-code` cleared 6 forbidden-substring checks plus the
  session-id and tool-deny invariants; live `--verify` reports the city matches
  source.
- Negative (mechanism honesty): a throwaway copy with the **real**
  `--tools Read,Grep,Glob` flag removed (and the wrapper's prose comments
  de-substringed so no comment could fake the match) was **refused** —
  `"tool_deny_mechanism=config but no structural '<tool>: deny' block in
  workdir/ and no '--tools <allowlist>' in bin/"`, exit 1, target city left
  empty. This proves the gate passes because the real enforced allowlist is
  present, not because a comment mentions `--tools`.

> Mold-honesty note (feedback for `home-7jk3`): the `install-adapter` `config`
> validator's `--tools <allowlist>` branch matches the regex
> `--tools[[:space:]]+[A-Za-z]` anywhere in `bin/`, so a **prose comment** like
> `# the --tools allowlist` also satisfies it. This adapter is honest because
> its only such match is the real flag (verified by the negative test above),
> but the mold could be hardened to require the `--tools` match on a
> non-comment line (or to only count it inside the actual harness invocation).

## Why `--bare` is NOT used

Claude Code's `--bare` flag forces Anthropic auth to be strictly
`ANTHROPIC_API_KEY` / `apiKeyHelper` and never reads OAuth/keychain. This host
authenticates via `claude.ai` OAuth (subscription auth), so `--bare` would break
auth. The smoke therefore uses plain `--print` with the existing auth, plus
`--setting-sources ""` and the `CLAUDE_CODE_*` env scrub for isolation.

## Adapter source and install

Package: `chrote/docs/gas-city-research/adapter-research/harness-adapters/` (this repo).

| tracked source                                          | live city dest                                  |
|---------------------------------------------------------|-------------------------------------------------|
| `adapters/claude-code/bin/claude-gc-agent`              | `/home/perttu/gascity/bin/claude-gc-agent`      |
| `adapters/claude-code/agent/agent.toml`                 | `/home/perttu/gascity/agents/claude-code/agent.toml` |
| `adapters/claude-code/workdir/CLAUDE.md`                | `/home/perttu/gascity/claude-code/CLAUDE.md`    |

```bash
PKG=/home/perttu/chrote/docs/gas-city-research/adapter-research/harness-adapters
"$PKG/bin/install-adapter" claude-code            # sync into live city ($GC_CITY|/home/perttu/gascity)
"$PKG/bin/install-adapter" claude-code --verify   # confirm city matches source
gc --city /home/perttu/gascity reload
gc config explain --agent claude-code

# One disposable smoke under a real Gas City identity:
gc session new claude-code --title "claude-code-smoke" --no-attach
gc session pin gc-<id> && gc session wake gc-<id>
# Gas City sets GC_SESSION_ID; the wrapper runs `claude -p` (read-only allowlist)
# once and mails human the poem from the session identity.
gc mail inbox human
gc session unpin gc-<id> && gc session close gc-<id>   # disposable
```
