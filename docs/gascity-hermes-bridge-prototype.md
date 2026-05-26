# Hermes To Gas City Bridge Prototype

Date: 2026-05-27
Worker: Chunk 6 worker A
Bead: `home-5qmz`

## Purpose

Prototype the smallest bridge shape that lets a Hermes/Discord-originated
request enter the same Gas City workflow world as CHROTE 3.0 work.

This is a bridge contract and live disposable smoke, not production automation.
It does not use Discord tokens, send Discord messages, mutate Hermes/Discord
configuration, start a new real harness, expose the Gas City supervisor, or
make Gas City the durable memory source.

Current source-of-truth split remains:

- `/home/perttu` Beads is canonical for CHROTE work, including `home-5qmz`.
- Context Citadel is canonical for durable personal/project context.
- Gas City owns runtime orchestration artifacts: mail, routed task beads,
  sessions, formulas, molecules, nudges, and events.
- CHROTE owns authenticated operator access and policy. The raw Gas City
  supervisor stays localhost-only.

## Assumptions

- Hermes/Discord can authenticate the real Discord event before this bridge is
  called.
- The bridge receives a sanitized request envelope, not a raw Discord gateway
  event and not token material.
- A Discord user, channel, message id, or Hermes profile name is external
  attribution, not a Gas City sender identity.
- The first useful bridge does not need a long-running service. A small local
  adapter can call native `gc` commands and return exact artifact ids.

## Auth And Attribution Boundary

Authentication happens before Gas City.

Hermes owns Discord authentication and should pass only an already-validated
request envelope to the local bridge. The bridge must never receive, log,
persist, or forward Discord bot tokens, Hermes profile tokens, owner tokens,
Context Citadel tokens, shell history, SSH material, or raw environment dumps.

Gas City attribution is separate:

- `origin.*` fields record where the request came from.
- `bridge_actor` records which local bridge principal wrote to Gas City.
- `home_bead_id` links to canonical Beads work when the request relates to an
  existing work item.
- Gas City `from` is reserved for valid Gas City identities.

The live smoke proved this matters: the attempted
`gc mail send --from hermes-bridge-smoke ...` command failed because
`hermes-bridge-smoke` was not a valid Gas City session identity. That is the
right failure mode for production.
Productization should create a real `hermes-bridge` Gas City identity or run
the bridge as an authenticated local writer with explicit body metadata, not
spoof Discord users through `--from`.

## Bridge Input Shape

Version `gascity.hermes_bridge.request.v0`:

```json
{
  "schema_version": "gascity.hermes_bridge.request.v0",
  "nonce": "HGB-A-20260527-001",
  "origin": {
    "kind": "discord_message",
    "hermes_profile": "hermes-smoke",
    "discord_guild_id": "synthetic-guild",
    "discord_channel_id": "synthetic-channel",
    "discord_message_id": "synthetic-message-HGB-A-20260527-001",
    "discord_user_id": "synthetic-user"
  },
  "auth": {
    "verified_by": "hermes",
    "bridge_actor": "local-hermes-bridge",
    "verified_at": "2026-05-27T01:57:38+03:00"
  },
  "request": {
    "title": "Hermes bridge smoke routed request HGB-A-20260527-001",
    "body": "Synthetic Hermes/Discord-originated request smoke.",
    "intent": "route_task",
    "target": "planner",
    "home_bead_id": "home-5qmz",
    "notify": false
  },
  "safety": {
    "synthetic": true,
    "allow_real_harness_start": false,
    "allow_file_mutation": false,
    "allow_notify": false
  }
}
```

Required validation:

- reject missing `schema_version`, `nonce`, `origin.kind`, `request.title`, or
  `request.intent`;
- reject raw token-shaped fields and raw Discord gateway payload dumps;
- allow only explicit targets such as `human`, `planner`, or a configured
  Gas City bridge/dispatcher identity;
- default `notify` to `false`;
- require human approval before any real harness start, file mutation, package
  install, service change, process kill/restart, or network exposure.

## Bridge Output Shape

Version `gascity.hermes_bridge.result.v0`:

```json
{
  "schema_version": "gascity.hermes_bridge.result.v0",
  "nonce": "HGB-A-20260527-001",
  "city": "/home/perttu/gascity",
  "artifacts": {
    "mail_id": "gc-52691",
    "mail_thread_id": "thread-00c50d390d8c",
    "sling_bead_id": "gc-52688",
    "formula_root_id": null,
    "event_seq": 105339
  },
  "commands": [
    "gc sling planner --stdin --json --no-convoy --no-formula",
    "gc mail send planner -s \"Hermes bridge smoke HGB-A-20260527-001\" -m \"...\""
  ],
  "notes": [
    "No Discord token or real Discord message was used.",
    "No notification was requested.",
    "Mail sender is human in the smoke because a synthetic sender was rejected."
  ]
}
```

The output is a receipt for later CHROTE display or Beads notes. It is not a
source-of-truth update by itself.

## First Implementation Path

1. Add a local bridge entry point that Hermes can call without exposing Gas
   City beyond localhost. A Unix-domain socket or localhost-only HTTP endpoint
   is enough for the first version.
2. Validate and normalize the request envelope above.
3. Write a Gas City mail artifact for operator or dispatcher visibility:

   ```bash
   gc mail send planner \
     -s "<request title>" \
     -m "<sanitized origin summary and body>"
   ```

4. For actionable work, route a task bead through sling without notification:

   ```bash
   gc sling planner --stdin --json --no-convoy --no-formula
   ```

5. For multi-step workflows only after the single-task path is boring, cook a
   formula with explicit metadata:

   ```bash
   gc formula cook plan-review-synthesis \
     --title "<workflow title>" \
     --var topic="<sanitized topic>" \
     --meta home_bead=home-5qmz \
     --meta origin=hermes-discord
   ```

6. Return the bridge output receipt with exact `gc-*` ids.

The first implementation should not use real Discord event replay, real harness
launch, `--notify`, or broad formula cooking by default.

## Live Smoke Evidence

Smoke nonce: `HGB-A-20260527-001`

Safety checks:

```bash
curl -sS http://127.0.0.1:8372/health
ss -ltnp | rg '127\.0\.0\.1:8372|0\.0\.0\.0:8372|\[::\]:8372|:8372'
gc status
gc session list --state all
```

Evidence:

- Supervisor health returned `{"status":"ok",...,"cities_running":1,...}`.
- `ss` showed `gc` listening on `127.0.0.1:8372` only.
- `gc status` showed three running mock agents: `planner`, `reviewer-a`, and
  `reviewer-b`.
- A pre-existing real Pi proof session, `gc-51923` / `chrote-poem-pi`, was
  active from the earlier vertical slice. The Hermes bridge smoke did not
  launch or notify it.
- The configured `pi-smoke` agent template remained stopped/asleep in
  `gc status`; no new real harness was launched for this smoke.

Boundary check that failed safely:

```bash
gc mail send planner \
  --from hermes-bridge-smoke \
  -s "Hermes bridge smoke HGB-A-20260527-001" \
  -m "Synthetic Hermes/Discord-originated request smoke..."
```

Result:

```text
gc mail send: invalid sender "": session not found: "hermes-bridge-smoke"
```

That blocks sender spoofing as the first production path. Use a valid Gas City
bridge identity later; until then, store external origin as payload metadata.

Successful routed task artifact:

```bash
gc sling planner --stdin --json --no-convoy --no-formula <<'EOF'
Hermes bridge smoke routed request HGB-A-20260527-001
Synthetic Hermes/Discord-originated request smoke.

Input envelope:
origin_kind=discord_message
origin_profile=hermes-smoke
source_channel_id=synthetic-channel
source_message_id=synthetic-message-HGB-A-20260527-001
nonce=HGB-A-20260527-001
canonical_home_bead=home-5qmz
requested_artifact=gas-city-sling-task

No real Discord token or message was used. Do not execute; this bead exists only as bridge shape evidence.
EOF
```

Result:

```json
{
  "schema_version": "1",
  "success": true,
  "target": "planner",
  "method": "bead",
  "bead_id": "gc-52688",
  "routed": true,
  "queued": false,
  "dry_run": false
}
```

Bounded runtime lookup:

```bash
jq -c '.beads[] | select(.id=="gc-52688") | {id,type,status,title,description,from,assignee,labels,metadata}' .gc/beads.json
```

Observed:

- `id`: `gc-52688`
- `status`: `open`
- `title`: `Hermes bridge smoke routed request HGB-A-20260527-001`
- `metadata.gc.routed_to`: `planner`

Successful mail artifact using local smoke actor:

```bash
gc mail send planner \
  -s "Hermes bridge smoke HGB-A-20260527-001" \
  -m "Synthetic Hermes/Discord-originated request smoke. nonce=HGB-A-20260527-001; bridge_actor=local-human-cli-smoke; origin_kind=discord_message; origin_profile=hermes-smoke; source_channel_id=synthetic-channel; source_message_id=synthetic-message-HGB-A-20260527-001; requested_artifact=gas-city-mail; canonical_home_bead=home-5qmz; no real Discord token or message was used; no notification requested."
```

Result:

```text
Sent message gc-52691 to planner
```

Verification:

```bash
gc mail peek gc-52691
jq -c '.beads[] | select(.id=="gc-52691") | {id,type,status,title,description,from,assignee,labels,metadata}' .gc/beads.json
rg -n "HGB-A-20260527-001|gc-52688|gc-52691" .gc/events.jsonl
```

Observed:

- mail id: `gc-52691`
- from: `human`
- to/assignee: `planner`
- subject/title: `Hermes bridge smoke HGB-A-20260527-001`
- thread label: `thread:thread-00c50d390d8c`
- event seq: `105339`
- event type: `mail.sent`
- event actor: `human`
- event payload `read`: `false`

The smoke created Gas City runtime artifacts only. It did not close or mutate
canonical `home-*` Beads, notify agents, launch real Discord/Hermes traffic,
or expose the supervisor.

## Rollback And Untangle Notes

For this smoke:

- Leave `gc-52688` and `gc-52691` as disposable runtime evidence unless Perttu
  explicitly asks for cleanup.
- Do not hand-edit `/home/perttu/gascity/.gc/beads.json`.
- Do not rewrite `.gc/events.jsonl`.
- If the mail artifact becomes noisy, use a supported mail command such as
  `gc mail archive gc-52691` only after deciding the evidence can be hidden
  from the inbox.
- There is no safe generic close command for the routed task in the current
  file-backed sidecar surfaced by `gc beads`; treat it as historical evidence
  or clean it up later through a supported Gas City workflow.

For product rollback:

- stop the local bridge process or remove the Hermes route to it;
- keep the Gas City supervisor bound to `127.0.0.1`;
- do not delete canonical Beads or Context Citadel context;
- disable only the bridge identity/config, not unrelated Gas City sessions;
- preserve mail/task ids in Beads notes if they became part of a real work
  trail.

## Follow-Up Work For Productization

- Create a valid `hermes-bridge` Gas City identity or a documented local writer
  principal. The sender-spoof failure above should remain a gate.
- Add a tiny bridge adapter only after the schema is accepted. It should read a
  sanitized envelope and call `gc` with argv arrays, not shell-interpolated raw
  Discord text.
- Add synthetic tests for validation: missing fields, token-shaped fields,
  unsupported target, notify defaulting to false, and sender identity handling.
- Decide where CHROTE displays bridge receipts: likely Gas City mail/task ids
  in the read-only Gas City observer, with `/home/perttu` Beads remaining
  canonical.
- Add an explicit approval path before `--notify`, formula cooking, real harness
  starts, file mutations, or service/process changes.
- Add a cleanup/archival command path for disposable bridge artifacts if Gas
  City exposes one for file-backed task beads.
