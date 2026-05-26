# Gas City OpenCode Parity Validation

Date: 2026-05-27
Worker: chunk 6 worker B
Bead: `home-9o5`

## Scope

This validation checked whether OpenCode is ready to follow the same Gas
City-owned identity and mail pattern proven by `home-bi6d.5`.

The first pass found a CLI availability blocker and did not install OpenCode,
edit shell profiles, launch an OpenCode session, start broad harness sessions,
print API keys, dump environment variables, or mutate Gas City config. The
follow-up pass resolved the CLI blocker with a user-local npm install, reviewed
safe OpenCode flags, and ran one contained Gas City-owned OpenCode smoke. The
live Gas City city inspected and extended was `/home/perttu/gascity`.

## Follow-Up Execution

Bead `home-p7jn` resolved the CLI availability blocker.

Commands and facts:

- Official OpenCode docs list `npm install -g opencode-ai` as a supported
  Node.js install path.
- `npm config get prefix` returned `/home/perttu/.npm-global`, so the install
  was user-local rather than system-wide.
- `npm install -g opencode-ai` completed and added the CLI.
- `command -v opencode` now resolves to
  `/home/perttu/.npm-global/bin/opencode` from `/home/perttu/gascity`.
- `opencode --version` reports `1.15.10`.
- The normalized Gas City wrapper PATH also resolves the same executable.

Safe flag review:

- `opencode run --help` shows `--pure`, `--agent`, `--format`, `--title`, and
  `--dir`.
- `opencode run --help` also shows
  `--dangerously-skip-permissions`; the smoke did not use it.
- `opencode agent create --help` shows explicit tool permissions, including
  `bash`, `read`, `edit`, `glob`, `grep`, `webfetch`, `task`, `todowrite`,
  `websearch`, `lsp`, and `skill`.

Gas City smoke files added under `/home/perttu/gascity`:

- `bin/opencode-gc-agent`
- `agents/opencode-smoke/agent.toml`
- `opencode-smoke/.opencode/agent/gas-city-smoke.md`

The OpenCode agent denies `bash`, `edit`, `webfetch`, `task`, `todowrite`,
`websearch`, `lsp`, and `skill`. It allows only the read-only tools left
available by omission, and the smoke prompt did not require tool use. The Gas
City wrapper refuses to send mail unless immutable `GC_SESSION_ID` is set, so
`GC_ALIAS` alone is not enough to spoof the `opencode-smoke` sender from a
normal shell.

Live Gas City smoke:

```bash
gc session new opencode-smoke \
  --alias opencode-smoke \
  --title "OpenCode Gas City smoke OC-GC-20260527-012" \
  --no-attach
```

Results:

- session `gc-52763` was created from template `opencode-smoke`;
- OpenCode ran successfully with `opencode_status=0`;
- mail `gc-52766` was sent to `human` from `opencode-smoke`;
- mail metadata includes `mail.from_session_id=gc-52763`;
- event `105487` records `mail.sent` by actor `opencode-smoke`;
- the message body contains a model-authored two-line poem with nonce
  `OC-GC-20260526-231358`.

Operational caveat:

`gc session peek gc-52763` woke the one-shot smoke session once more before it
was closed, producing a second successful mail, `gc-52767`, with nonce
`OC-GC-20260526-231421`. The disposable session was then closed with
`gc session close gc-52763`. The wrapper has since been tightened to record
per-session completion under `.gc/opencode-smoke-runs/` for future sessions,
but that marker was added after the duplicate replay. Closure of `gc-52763`
is the verified cleanup for this smoke.

## First Identity Proof

`home-bi6d.5` is closed and usable as the first Gas City-owned real harness
identity proof.

Evidence checked:

- `bd show home-bi6d.5 --json` reports status `closed`.
- Close reason says the Pi fallback vertical slice proved
  `gc-51923` / `chrote-poem-pi` could receive a bounded prompt through
  `gc session nudge` and return model-authored content through Gas City mail
  `gc-52383` without sender spoofing.
- `docs/gascity-real-identity-smoke.md` records the same proof.
- Live Gas City still reports `gc-51923` active as template `pi-smoke` with
  target `chrote-poem-pi`.
- The live mail record `gc-52383` has `from=chrote-poem-pi`,
  `assignee=human`, title `C3 remedial pi poem C3R-20260527-004915`, and flat
  metadata keys `mail.from_session_id=gc-51923`,
  `mail.from_display=chrote-poem-pi`, and `mail.read=true`.

This proves the pattern but only for Pi. It does not prove OpenCode is
available or safe to launch.

## Initial OpenCode Availability Blocker

The initial validation found that OpenCode was not available as a CLI in the
relevant CHROTE/Gas City terminal environment.

Commands checked:

```bash
command -v opencode
type -a opencode
cd /home/perttu/gascity
command -v opencode
PATH="$HOME/.local/bin:$HOME/bin:$HOME/.npm-global/bin:$HOME/node_modules/.bin:$HOME/.bun/bin:$HOME/go/bin:/usr/local/go/bin:$PATH" command -v opencode
PATH="$HOME/.local/bin:$HOME/bin:$HOME/.npm-global/bin:$HOME/node_modules/.bin:$HOME/.bun/bin:$HOME/go/bin:/usr/local/go/bin:$PATH" type -a opencode
```

Result:

- `command -v opencode` returned no path from both
  `/home/perttu/chrote-3.0-gascity` and `/home/perttu/gascity`.
- The normalized Gas City wrapper PATH still did not resolve `opencode`.
- `type -a opencode` returned `opencode: not found`.
- `gc config explain --agent opencode-smoke` failed with
  `no agents match filters`, so there is no configured OpenCode smoke agent.

Known local hints:

- `/home/perttu/.config/opencode/package.json` exists and lists
  `@opencode-ai/plugin@1.14.48`.
- `/home/perttu/.config/opencode/node_modules/@opencode-ai/sdk/package.json`
  and `@opencode-ai/plugin/package.json` exist at version `1.14.48`.
- `/home/perttu/.local/share/opencode/opencode.db` and cache directories exist.
- No executable `opencode` was found in common local binary paths:
  `/home/perttu/.local/bin`, `/home/perttu/bin`,
  `/home/perttu/.npm-global/bin`, `/home/perttu/.bun/bin`,
  `/home/perttu/.cargo/bin`, `/usr/local/bin`, or `/usr/bin`.
- `/home/perttu/.config/opencode/node_modules/.bin` exists, but contains no
  `opencode` executable.
- Global npm packages are `@beads/bd`, `@earendil-works/pi-coding-agent`, and
  `agent-browser`; no global OpenCode CLI package is installed.

Interpretation: there is local OpenCode state and SDK/plugin material, but not
an executable CLI on PATH that Gas City can launch.

## Initial Safe Flags Blocker

`opencode --help` was not run because `opencode` does not resolve and no local
OpenCode CLI executable was found. Running `npx`, `npm exec`, `bunx`, curl
installers, or package installation was intentionally avoided because that
could fetch/install software outside this validation scope.

The exact blocker is:

```text
OpenCode CLI is not installed or not exposed on PATH for the CHROTE/Gas City
terminal environment. Existing ~/.config/opencode plugin/sdk state is not
enough to launch a Gas City-owned OpenCode harness.
```

Because help output could not be inspected, no read-only, no-edit, session, or
permission flags can be accepted yet. An OpenCode smoke is not safe to run.

## Required Gas City Pattern For OpenCode

OpenCode must follow the same identity/mail pattern as the first Pi proof:

- Gas City owns the process through a configured agent such as
  `opencode-smoke`.
- The wrapper starts from a minimal, reviewed environment and prints only
  non-secret diagnostics such as the resolved command path, working directory,
  session id, and policy mode.
- The wrapper must not print full environment variables, API keys, auth
  material, shell history, Context Citadel tokens, SSH material, or raw private
  transcripts.
- The first OpenCode launch must use reviewed read-only or permission-restricted
  flags from `opencode --help`; unrestricted `exec opencode` is not acceptable.
- The first session must be disposable, max one active session, and targeted by
  immutable Gas City session id plus expected alias, not raw tmux names or
  ambiguous aliases.
- Prompt delivery must use a documented Gas City primitive such as
  `gc session submit`, `gc session nudge`, or an explicit mail-injection bridge.
- Result evidence must come back through `gc mail` from the valid OpenCode Gas
  City identity. Sender spoofing with arbitrary `--from` does not count.
- Mail storage, mail injection, and harness reaction are separate checks.
  Seeing mail in an inbox does not prove OpenCode reacted to it.
- Gas City supervisor access remains localhost-only; CHROTE remains the
  authenticated cockpit and policy layer.

Expected initial wrapper shape, after the CLI and safe flags are known:

```bash
#!/usr/bin/env bash
set -euo pipefail

export PATH="$HOME/.local/bin:$HOME/bin:$HOME/.npm-global/bin:$HOME/node_modules/.bin:$HOME/.bun/bin:$HOME/go/bin:/usr/local/go/bin:$PATH"

echo "[opencode-gc-agent] opencode=$(command -v opencode || true)"
echo "[opencode-gc-agent] mode=read-only-smoke"

exec opencode <reviewed-read-only-or-permission-restricted-flags>
```

Expected Gas City agent shape:

```toml
description = "Contained smoke-test OpenCode harness agent."
work_dir = "."
start_command = "./bin/opencode-gc-agent"
prompt_mode = "none"
nudge = "ready"
max_active_sessions = 1
min_active_sessions = 0
```

## Initial Smoke Evidence Or Blocker

Read-only evidence gathered:

```bash
gc doctor --verbose
gc status
gc session list --state all
gc config explain --agent pi-smoke
gc config explain --agent opencode-smoke
```

Result:

- `gc doctor --verbose` passed 46 checks.
- `gc status` showed the live city supervisor-managed, not suspended, with four
  active sessions and zero suspended sessions.
- `gc session list --state all` showed the active Pi proof session
  `gc-51923` / `chrote-poem-pi`.
- `gc config explain --agent pi-smoke` resolves to the contained Pi wrapper.
- `gc config explain --agent opencode-smoke` fails because no OpenCode smoke
  agent exists.

No OpenCode smoke was run. The blocker is CLI availability plus unreviewed
OpenCode permission/session flags.

## No-Secret Checks

Initial validation checks:

- Did not print PATH wholesale, environment variables, config auth files, token
  values, shell history, or `.env` contents.
- Inspected only package metadata and file names under `~/.config/opencode`;
  did not inspect OpenCode databases or auth material.
- Did not run OpenCode, `npx`, `npm exec`, `bunx`, curl installers, package
  installation, or shell profile edits.
- Did not add or mutate Gas City OpenCode wrapper/config files.
- No broad harness sessions were launched.

Follow-up checks:

- Installed only the user-local `opencode-ai` package through npm.
- Did not print OpenCode auth files, provider tokens, shell history, `.env`
  contents, or full environment dumps.
- Did not use `--dangerously-skip-permissions`.
- Did not change shell profiles or system package state.
- The live smoke used a contained Gas City identity and read-only OpenCode
  agent permissions.

## Recommendation

`home-9o5` is closeable with a caveat. OpenCode parity now has:

- a resolved CLI in the CHROTE/Gas City terminal environment;
- reviewed safe flags and an explicit no-dangerous-permissions boundary;
- a contained Gas City `opencode-smoke` identity;
- successful Gas City mail return from `opencode-smoke` to `human` with
  `mail.from_session_id=gc-52763`;
- no shell profile changes and no printed auth material.

The caveat is lifecycle shape: this is a one-shot smoke wrapper, not a durable
interactive OpenCode harness. The one-shot session was closed after the smoke.
Future production work should decide whether OpenCode should run as a
persistent TUI/server/ACP harness or stay as a one-shot task runner.
