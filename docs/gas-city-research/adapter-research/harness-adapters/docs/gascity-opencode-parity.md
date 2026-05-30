# Gas City OpenCode Parity — Smoke Evidence and Package Verification

This document preserves the OpenCode Gas City parity evidence and the
verification that the tracked `gascity-harness-adapters` package recreates the
live city configuration. It is the durable record for Beads `home-7jk3` and the
shared adapter mold reused by `home-piis` (Codex) and `home-5v4k` (Claude Code).

The broader, longer experiment narrative (Pi + OpenCode, run gates, setup steps)
lives at `/home/perttu/gascity/docs/pi-opencode-harness-experiment.md`. This file
is the focused parity + packaging record.

## OpenCode parity smoke (chunk 6, 2026-05-26/27)

Outcome: **OpenCode runs as a valid Gas City-owned identity and returns mail.**

- Environment: OpenCode CLI installed user-locally (npm prefix
  `/home/perttu/.npm-global`); `command -v opencode` resolves from
  `/home/perttu/gascity`; `opencode --version` = `1.15.10`.
- Safe launch: the smoke used `opencode run --pure` against the contained
  `gas-city-smoke` agent, which **denies** `bash`, `edit`, `webfetch`, `task`,
  `todowrite`, `websearch`, `lsp`, and `skill`. It did **not** use
  `--dangerously-skip-permissions` or any broad-permission bypass.
- Identity / mail:
  - Gas City session **`gc-52763`** (template `opencode-smoke`) sent mail
    **`gc-52766`** to `human` from `opencode-smoke` with
    `mail.from_session_id = gc-52763`.
  - A `peek` replay produced **`gc-52767`** before the disposable session was
    closed.
- Hardening applied after review: the wrapper requires an **immutable
  `GC_SESSION_ID`** before sending mail (not `GC_ALIAS` alone), so a launched
  process cannot spoof a sender identity.

These ids and findings are also recorded in the `home-7jk3` / `home-9o5` Beads
notes.

## What the tracked package contains

Package: `chrote/docs/gas-city-research/adapter-research/harness-adapters/` (this repo).

The three live runtime files are mirrored byte-for-byte as adapter source and
installed back into the city by `bin/install-adapter` per the adapter manifest:

| tracked source                                                      | live city dest                                       |
|---------------------------------------------------------------------|------------------------------------------------------|
| `adapters/opencode-smoke/bin/opencode-gc-agent`                     | `/home/perttu/gascity/bin/opencode-gc-agent`         |
| `adapters/opencode-smoke/agent/agent.toml`                          | `/home/perttu/gascity/agents/opencode-smoke/agent.toml` |
| `adapters/opencode-smoke/workdir/.opencode/agent/gas-city-smoke.md` | `/home/perttu/gascity/opencode-smoke/.opencode/agent/gas-city-smoke.md` |

Not packaged / never copied: `.gc/` runtime state, `controller.token`, sockets,
`node_modules/`, and `*.done` run markers.

## Package verification (this session, 2026-05-27)

All commands run from a non-Hermes Ubuntu shell with the Gas City supervisor up
on `127.0.0.1:8372` (localhost-only, unchanged).

1. **Source matches live city** — byte-identical, all three files:
   ```
   install-adapter opencode-smoke --verify
   = bin/opencode-gc-agent (matches source)
   = agents/opencode-smoke/agent.toml (matches source)
   = opencode-smoke/.opencode/agent/gas-city-smoke.md (matches source)
   verify OK: live city matches tracked source   (exit 0)
   ```

2. **Recreates config in a disposable city** — installed into a fresh
   `mktemp -d` city root; resulting files were byte-identical (`diff`) to the
   live `/home/perttu/gascity` files, with correct modes (`0755` wrapper,
   `0644` configs); `--verify` against the temp city returned exit 0; temp city
   removed afterward. The live city was not modified by this test.

3. **Safety contract fails loud** — a throwaway adapter containing
   `--dangerously-skip-permissions`, no `GC_SESSION_ID` gate, and no tool-deny
   block was **refused** before any file was written:
   ```
   SAFETY: forbidden substring '--dangerously-skip-permissions' found ...
   SAFETY: requires_immutable_session_id=true but no GC_SESSION_ID gate in bin/
   SAFETY: tool_default_deny=true but no deny/allowlist found in agent config
   error: 3 safety violation(s); refusing to install.   (exit 1)
   ```
   The target city remained empty. Negative-test artifacts were removed.

## Reproducing the OpenCode smoke from tracked source

```bash
PKG=/home/perttu/chrote/docs/gas-city-research/adapter-research/harness-adapters

# 1. Sync the adapter into the live city from tracked source.
"$PKG/bin/install-adapter" opencode-smoke
"$PKG/bin/install-adapter" opencode-smoke --verify

# 2. Reload Gas City and confirm the agent resolves.
gc --city /home/perttu/gascity reload
gc config explain --agent opencode-smoke

# 3. Run one disposable smoke under a real Gas City identity.
gc session new opencode-smoke --alias opencode-smoke --title "OpenCode smoke" --no-attach
gc session pin opencode-smoke
gc session wake opencode-smoke
# Gas City sets the immutable GC_SESSION_ID for the session; the wrapper runs
# `opencode run --pure --agent gas-city-smoke` once and mails human the poem.

# 4. Confirm mail returned under a valid identity, then clean up.
gc mail inbox human
gc session close opencode-smoke   # disposable
```

The wrapper refuses to send mail if `GC_SESSION_ID` is unset (exit 64), so a
manual `./bin/opencode-gc-agent` outside a Gas City session cannot spoof mail.
