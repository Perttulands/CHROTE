# Hermes → Gas City Ingress Bridge (v0)

Tracked source for the **Hermes ingress adapter** that lets a Hermes/Discord
request enter the *same* Gas City workflow model as CHROTE 3.0 work, instead of
living in a separate communication world.

This package realizes Beads `home-qnzi` (productize the bridge), the follow-up
of `home-5qmz` (prototype, recorded in
`/home/perttu/chrote-3.0-gascity/docs/gascity-hermes-bridge-prototype.md`).

## This is an INGRESS adapter, NOT the harness-wrapper mold

The sibling `../../adapters/` package is the **harness-wrapper mold**: it wraps
a CLI (OpenCode, Codex, Claude Code) as a long-lived *Gas City-owned session
identity* that returns mail. That shape answers "run a harness inside Gas City".

This package answers a **different question**: "a request arrived from outside
(Discord, via Hermes) — turn it into native Gas City artifacts". It is:

- a sanitized **request envelope** (`gascity.hermes_bridge.request.v0`) in,
- native `gc` primitives called with **argv arrays**,
- an **artifact receipt** (`gascity.hermes_bridge.result.v0`) out.

It does **not** register a gc agent, does **not** wrap a CLI, and does **not**
hold a session. There is no `adapter.toml` and `install-adapter` does not touch
it — that mold is for CLI-as-session adapters only. Forcing this into that mold
would be wrong.

## Files

```
ingress/hermes/
  README.md                     this file
  bin/hermes_bridge.py          validate → normalize → dispatch (the adapter)
  tests/test_hermes_bridge.py   offline validation tests (no gc call)
  examples/request.v0.example.json   a synthetic envelope
  schema/request.v0.schema.json      JSON Schema for the request envelope
  schema/result.v0.schema.json       JSON Schema for the result receipt
```

Python 3 stdlib only (no third-party deps). Python was chosen over Bash for the
adapter because the safety logic — recursive token-shape scanning, argv-array
construction, and the five validation rules — is far more testable and auditable
in Python, and `subprocess.run([...], shell=False)` gives the required
"argv arrays, never shell-interpolated raw Discord text" guarantee directly.

## Identity / writer-principal decision (the anti-spoofing gate)

The prototype proved, and this package re-verified live, that
`gc mail send --from <arbitrary>` is **rejected** because `--from` must be a
valid Gas City *session* identity:

```
$ gc --city /home/perttu/gascity mail send planner --from "discord-user-666999" -s ... -m ...
gc mail send: invalid sender "": session not found: "discord-user-666999"   (exit 1)
```

**Decision:** the bridge writes to Gas City as a **fixed local writer
principal** (`DEFAULT_WRITER_PRINCIPAL = "human"`, the city's default sender),
and the request envelope can **never** set the Gas City sender. Discord
user/channel/message ids travel only as **body metadata**.

Why a fixed local writer principal instead of a standing `hermes-bridge` Gas
City identity, for v0:

- Gas City identities are **sessions** (`gc mail send` resolves the sender from
  `$GC_SESSION_ID` / `$GC_ALIAS` / `$GC_AGENT`, else `"human"`). A standing
  `hermes-bridge` identity would require an always-running agent session — more
  surface, no v0 benefit.
- The load-bearing safety property is "**cannot spoof an arbitrary Discord user
  via `--from`**". The bridge guarantees this by construction: it **never
  appends `--from` from envelope data**, and it **rejects** any envelope that
  carries `request.from`, `request.gc_from`, `request.sender_identity`, or
  `auth.from`. The spoofing surface does not exist.
- This matches what the smoke actually did (mail `gc-52691` was `from: human`)
  and is fully reversible.

An operator may later point `DEFAULT_WRITER_PRINCIPAL` at a real, existing Gas
City `hermes-bridge` session identity. The spoofing surface stays closed either
way, because the value is **configuration, never envelope-controlled**.

## v0 request envelope contract

`gascity.hermes_bridge.request.v0` (full schema in
`schema/request.v0.schema.json`; synthetic example in
`examples/request.v0.example.json`):

| field                | required | notes |
|----------------------|----------|-------|
| `schema_version`     | yes      | must equal `gascity.hermes_bridge.request.v0` |
| `nonce`              | yes      | caller-supplied idempotency/correlation token |
| `origin.kind`        | yes      | e.g. `discord_message` — external attribution only |
| `origin.*`           | no       | sanitized Discord/Hermes attribution (never tokens) |
| `request.title`      | yes      | becomes mail subject / task title |
| `request.body`       | no       | becomes mail/task body |
| `request.intent`     | yes      | `mail_only` or `route_task` |
| `request.target`     | no       | `human` (default) or `planner`; others need approval |
| `request.home_bead_id` | no     | links to canonical `/home/perttu` Beads work |
| `request.notify`     | no       | **defaults false**; `true` requires approval |
| `approval`           | no       | `{operator_approved: true, approved_action: "..."}` |

**Rejected, before any gc call:** missing required fields; any token-shaped key
or value (JWTs, `Bearer ...`, Discord/Slack token shapes, PEM keys) anywhere in
the tree; raw Discord gateway frames (`op`/`d`/heartbeat fields); off-allowlist
targets; unsupported intents; any attempt to set a Gas City sender.

## v0 result receipt

`gascity.hermes_bridge.result.v0` (schema in `schema/result.v0.schema.json`):

```json
{
  "schema_version": "gascity.hermes_bridge.result.v0",
  "nonce": "...",
  "city": "/home/perttu/gascity",
  "artifacts": { "mail_id": "gc-XXXX", "sling_bead_id": "gc-YYYY" },
  "sender_principal": "human",
  "commands": ["gc --city ... sling planner ...", "gc --city ... mail send ..."],
  "notes": [ "No Discord token ...", "Sender pinned ...", "notify requested: false." ]
}
```

The receipt is for later CHROTE display or Beads notes. It is **not** a
source-of-truth update by itself; `/home/perttu` Beads stays canonical.

## Operator approval gates

Default-deny for anything beyond durable mail/task creation:

| action                                   | v0 policy |
|------------------------------------------|-----------|
| send mail receipt to `human`/`planner`   | allowed (default) |
| route a task bead (`sling`) to `human`/`planner` | allowed (default) |
| **live notification (`notify=true`)**    | **requires `approval.operator_approved=true`** |
| route to a non-allowlisted target (e.g. a paid harness) | rejected in v0 |
| real harness start, file mutation, package install, service/process change, network exposure | **out of scope for the bridge; never performed here** |

Without an explicit `approval` block, a `notify=true` envelope is refused with
exit 77 and **no gc call happens** (proven by
`test_notify_true_without_approval_blocks_dispatch`).

## Usage (localhost-only)

The bridge is invoked locally; it never opens a network listener itself, and the
Gas City supervisor stays bound to `127.0.0.1` (unchanged by this package). A
Hermes integration would pipe a sanitized envelope to stdin:

```bash
BIN=/home/perttu/chrote/docs/gascity-harness-adapters/ingress/hermes/bin/hermes_bridge.py

# Validate + show the plan WITHOUT calling gc (safe to run anywhere):
python3 "$BIN" --validate-only < examples/request.v0.example.json

# Dispatch for real (creates a Gas City mail + routed task in the live city):
python3 "$BIN" --city /home/perttu/gascity < envelope.json
```

Exit codes: `0` ok; `64` invalid JSON; `65` envelope rejected; `70` gc dispatch
failed; `77` operator approval required.

## Tests

Offline, no gc call, no third-party deps:

```bash
cd /home/perttu/chrote/docs/gascity-harness-adapters/ingress/hermes
python3 tests/test_hermes_bridge.py        # 24/24 passed, exit 0
# or, if pytest is installed:
python3 -m pytest tests/ -q
```

Coverage maps 1:1 to the bead acceptance: missing fields, token-shaped fields,
unsupported target, notify default false, and sender-identity (anti-spoofing)
handling — plus raw-gateway rejection, argv-array (no shell interpolation)
proof, and the happy-path receipt shape.

## Rollback and cleanup

**Code rollback** (this package is uncommitted source; nothing is wired into a
running service):

- This package adds **no** gc agent, **no** `city.toml` entry, and **no**
  service. To "disable" the bridge, simply do not invoke `hermes_bridge.py`
  and/or remove the Hermes route that pipes envelopes to it.
- Removing the directory `ingress/hermes/` fully removes the bridge. Nothing
  else references it; `install-adapter` ignores it.

**Artifact cleanup** (when a *live dispatch* created Gas City artifacts):

- A dispatch creates a `gc mail` message and (for `route_task`) a routed task
  bead in `/home/perttu/gascity`. These are **disposable runtime evidence**.
  Leave them unless the operator asks for cleanup.
- Do **not** hand-edit `/home/perttu/gascity/.gc/beads.json` or rewrite
  `.gc/events.jsonl`.
- To hide a noisy mail receipt, use a supported command only after deciding the
  evidence can leave the inbox:
  ```bash
  gc --city /home/perttu/gascity mail archive <gc-id>
  ```
- There is no safe generic close command for the routed task bead in the
  file-backed sidecar; treat it as historical evidence or clean it up later
  through a supported Gas City workflow. (Same limitation the prototype
  recorded.)

**Product rollback** (if a future Hermes route is wired):

- Stop the local caller / remove the Hermes route to the bridge.
- Keep the Gas City supervisor bound to `127.0.0.1`.
- Disable only the bridge config, not unrelated Gas City sessions.
- Do not delete canonical Beads or Context Citadel context.
- Preserve any `gc-*` ids in Beads notes if they became part of a real work
  trail.

## Safety invariants (enforced or asserted)

- **No tokens, ever.** No Discord/Hermes token is read, logged, persisted, or
  forwarded. Token-shaped keys/values are rejected before any gc call. Verify:
  ```bash
  grep -rniE 'discord.*token|bot[- ]?token|bearer [a-z0-9]|xox[baprs]-|BEGIN .*PRIVATE KEY' \
    ingress/hermes/   # only detector regexes + test fixtures, no real secrets
  ```
- **No sender spoofing.** `--from` is never set from envelope data; envelope
  sender fields are rejected.
- **Localhost-only.** The bridge opens no listener; the supervisor stays on
  `127.0.0.1:8372`.
- **Default-deny actions.** notify=false default; explicit approval required for
  notify; harness starts / file mutation / service changes are out of scope and
  never performed.
- **argv arrays.** gc is called via `subprocess.run([...], shell=False)`; raw
  Discord text is a single list element, never shell-interpolated.

## Verification evidence

See `docs/gascity-hermes-ingress.md` (sibling of the OpenCode parity doc) for
the recorded live `--from` spoof-rejection check and the test run.
