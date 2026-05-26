# Gas City Real Identity Smoke

Date: 2026-05-27
Worker: C3-A
Bead: `home-bi6d.5`

## Scope

This smoke tested one real Gas City-owned harness identity with a disposable
mail response. It did not edit CHROTE code, edit Gas City config, start a
multi-agent workflow, print secrets, dump environment, or kill sessions.

The live sidecar was `/home/perttu/gascity`. The only CHROTE write for this
worker is this evidence note.

## Harness Choice

Codex was not used. The live sidecar did not have a safe `codex-smoke` agent
configuration, and ADR-0002 rejects existing dangerous/bypass Codex launch
patterns for real-harness evidence.

Pi was used as the safer fallback because it was already configured as a Gas
City identity:

- agent config: `/home/perttu/gascity/agents/pi-smoke/agent.toml`
- wrapper: `/home/perttu/gascity/bin/pi-gc-agent`
- wrapper behavior: Pi session dir under `.gc/pi-sessions`, no context files,
  no extensions, no skills, no prompt templates, and tools limited to
  `read,grep,find,ls`

Existing active session reused:

- session id: `gc-51923`
- template: `pi-smoke`
- alias/mail identity: `chrote-poem-pi`
- tmux session: `s-gc-51923`

This avoided starting another `pi-smoke` session while `max_active_sessions = 1`.

## Commands And Evidence

Inspection:

```bash
gc doctor --verbose
gc session list --state all
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux -L gascity ls
gc session peek gc-51923 --lines 120
```

Evidence:

- `gc doctor --verbose` passed 46 checks.
- `gc session list --state all` showed `gc-51923` as active with template
  `pi-smoke` and target `chrote-poem-pi`.
- Gas City tmux listed `s-gc-51923`.
- `gc session peek gc-51923 --lines 120` showed the real Pi harness
  (`pi v0.74.0`), not the mock shell agent.

Prompt and return path:

```bash
gc session nudge gc-51923 --delivery immediate '! gc mail send human -s "C3-A pi-smoke reply" -m "C3-A pi-smoke acknowledges through gc mail."'
gc session peek gc-51923 --lines 120
gc mail inbox human
gc mail read gc-52336
jq -c '.beads[] | select(.id=="gc-52336") | {id, type, from:.from, assignee:.assignee, title:.title, description:.description, metadata:.metadata}' /home/perttu/gascity/.gc/beads.json
```

Evidence:

- The nudge returned `Nudged chrote-poem-pi`.
- Session output showed `Sent message gc-52336 to human`.
- `gc mail inbox human` showed `gc-52336` from `chrote-poem-pi`.
- `gc mail read gc-52336` returned:
  - `From: chrote-poem-pi`
  - `To: human`
  - `Subject: C3-A pi-smoke reply`
  - `Body: C3-A pi-smoke acknowledges through gc mail.`
- The bounded `.gc/beads.json` lookup showed:
  - `from`: `chrote-poem-pi`
  - `assignee`: `human`
  - `metadata.mail.from_session_id`: `gc-51923`
  - `metadata.mail.from_display`: `chrote-poem-pi`
  - `metadata.mail.read`: `true`

The mail send did not use `--from`; the sender resolved from the live Gas
City session environment.

## Result

The first bounded Pi fallback smoke passed the vertical-slice transport target:

- a real harness was already Gas City-owned as `gc-51923` / `chrote-poem-pi`
- a prompt was delivered through `gc session nudge`
- the harness terminal executed `gc mail send` from its own Gas City session
  identity
- the response was read through `gc mail read gc-52336`
- the message metadata links the sender display to session id `gc-51923`

This first message was operator-authored content, so it proved identity-bound
mail but not model-authored output.

## Remedial Model-Authored Mail Smoke

Reviewer C3-R blocked closure until the result body was authored by a model
rather than supplied directly by the operator.

Remedial command delivered through Gas City session nudge:

```bash
gc session nudge gc-51923 --delivery immediate '! set -euo pipefail; tmp=$(mktemp); pi --no-tools --no-context-files --no-extensions --no-skills --no-prompt-templates --no-session --mode text --print '\''Write a two-line original poem about Gas City mail. Include the exact nonce C3R-20260527-004915. Do not mention instructions or tools.'\'' > "$tmp"; gc mail send human -s "C3 remedial pi poem C3R-20260527-004915" -m "$(cat "$tmp")"; rm -f "$tmp"'
```

This still used the Pi session terminal shell escape because the configured
interactive `pi-smoke` harness has only `read,grep,find,ls` tools and cannot
invoke `gc mail` as a normal model tool. The shell command did not hardcode the
poem body. It invoked a nested, no-tools, no-session `pi --print` process inside
the Gas City session environment, wrote that model-authored output to a
temporary file, and sent that file content with `gc mail send` from the
session identity.

Evidence:

```bash
gc mail read gc-52383
jq -c '.beads[] | select(.id=="gc-52383") | {id, type, from:.from, assignee:.assignee, title:.title, description:.description, metadata:.metadata}' /home/perttu/gascity/.gc/beads.json
```

Result:

- mail id: `gc-52383`
- from: `chrote-poem-pi`
- to: `human`
- subject: `C3 remedial pi poem C3R-20260527-004915`
- body:

```text
The mail of Gas City is a stream of ciphered, breathless song,
C3R-20260527-004915, a nonce that travels long.
```

Metadata:

- `metadata.mail.from_session_id`: `gc-51923`
- `metadata.mail.from_display`: `chrote-poem-pi`

Current session state after the remedial smoke:

- `gc session list --state all` still shows `gc-51923` active as
  `pi-smoke` / `chrote-poem-pi`.
- `gc status` still shows only the expected four active sessions and no
  suspended sessions.

## Boundaries And Failure Modes

- This is Pi evidence, not Codex evidence. Codex remains blocked until a safe
  `codex-smoke` adapter exists without dangerous/bypass defaults.
- The current Pi wrapper does not give the interactive model a normal shell
  tool. The remedial smoke had to invoke nested `pi --print` through Pi's
  terminal shell escape, which proves model-authored mail under the session
  identity but is not yet a durable mail-injection hook.
- Existing unread human mail `gc-51917` and `gc-51920` was left untouched.
- The live file-backed Gas City runtime store changed because real mail beads
  were created and read. No CHROTE code or Gas City config was changed.
