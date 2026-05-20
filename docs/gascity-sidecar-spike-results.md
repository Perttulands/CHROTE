# Gas City Sidecar Spike Results

Date: 2026-05-18

## Setup

The isolated Gas City sidecar is installed at `<workspace-root>/gascity`.
It does not replace or mutate the CHROTE service under `/path/to/chrote`.

Installed components:

- `gc`: `<workspace-root>/.local/bin/gc`
- Gas City source: `<workspace-root>/research/upstreams/gascity`
- Gas City source commit: `37824e0 fix(cmd/gc/bd): surface bd silent on-disk fallback as loud error (#2080, #2079) (#2327)`
- `gc version`: `dev`
- Required host package added: `lsof`
- Supervisor service: `gascity-supervisor.service`

Runtime choices:

- Beads backend: file-backed via `[beads] provider = "file"`
- Dolt: intentionally skipped for this spike with `GC_DOLT=skip`
- Session runtime: tmux socket `gascity`
- Supervisor API: `http://127.0.0.1:8372`
- No public bind or Tailscale exposure was added
- No real AI harness was started

The spike has three harmless shell agents:

- `planner`
- `reviewer-a`
- `reviewer-b`

They run `./bin/mock-agent <role>` and write plain logs under
`<workspace-root>/gascity/logs/`.

## Verification

`gc doctor --verbose` passed after pinning planner and reviewer sessions:

```text
46 passed
```

Supervisor API health worked:

```text
GET /health -> status ok, build_id 37824e047312...
GET /v0/cities -> gascity running=true
GET /v0/events -> event cursor plus city event history
```

Session lifecycle worked:

```text
gc session new planner --alias plan-demo --title "CHROTE meta-harness plan demo" --no-attach
gc session list
gc session peek plan-demo
gc session nudge plan-demo "Plan the next CHROTE meta-harness slice and list acceptance criteria."
gc session submit planner "Submit-path validation for CHROTE meta-harness spike." --intent follow_up
tmux -S /run/user/1000/chrote-tmux/tmux-1000/gascity list-sessions
```

Result:

- session `gc-16` was created
- alias `plan-demo` is active
- tmux session `s-gc-16` exists
- `gc session peek` shows live session output
- nudge delivery was observed in the session output
- submit/follow-up delivery was observed in the session output
- mock-agent durable log exists at `logs/planner.log`

Gas City later drained the ad hoc `plan-demo` session after its wake reason
cleared. That behavior is useful for resource control but is not the desired
"always inspectable live cockpit" default. A canonical named session was then
pinned:

```text
gc session pin planner
gc session wake planner
```

Result:

- session `gc-36` is active with wake reason `session,config,pin`
- sessions `gc-76` and `gc-77` are active for `reviewer-a` and `reviewer-b`
- tmux sessions `planner`, `reviewer-a`, and `reviewer-b` are present on socket `gascity`
- `gc session peek planner` shows live output

Mail worked:

```text
gc mail send reviewer-a "Review the current CHROTE meta-harness plan from implementation risk perspective."
gc mail inbox reviewer-a
gc mail read gc-18
```

Result:

- message `gc-18` was delivered to `reviewer-a`
- `gc mail read gc-18` marks it read; `gc mail inbox reviewer-a` now reports no unread messages

Sling worked:

```text
gc sling planner "Demo task routed to planner from setup validation" --json
```

Result:

- bead `gc-29` was created and routed to `planner`
- convoy `gc-30` was created

Formula cook worked for a local minimal plan-review-synthesis formula:

```text
gc formula cook plan-review-synthesis --title "Demo CHROTE plan review" --var topic="CHROTE meta-harness"
```

Result:

- root `gc-21`
- steps `gc-22` through `gc-25`

Formula sling / wisp worked:

```text
gc sling planner plan-review-synthesis --formula --title "Wisp demo CHROTE review" --var topic="CHROTE meta-harness" --json
```

Result:

- wisp root `gc-31` was created and routed to `planner`

Core `mol-review-quorum` is available through `gc formula list`, but
`gc formula cook mol-review-quorum` exited with status 1 and no diagnostic
output in this environment. For this spike, the local
`plan-review-synthesis` formula satisfies the plan/two-reviewers/synthesis
acceptance path. The silent `mol-review-quorum` failure should be investigated
before relying on the upstream formula directly.

## Integration Notes

The supervisor owns a single API on `127.0.0.1:8372`. A per-city `[api]`
section was removed because Gas City logs that per-city API ports are ignored
under supervisor mode.

The supervisor API should not be exposed directly through Tailscale or the
public web. Mutation endpoints use `X-GC-Request` as CSRF friction, not user
authentication. CHROTE should proxy this API through its existing authenticated
surface and start read-only.

The spike directory is isolated, but the supervisor is a user-global service:

- service file: `<workspace-root>/.local/share/systemd/user/gascity-supervisor.service`
- registry: `<workspace-root>/.gc/cities.toml`

That is acceptable for this local spike, but a production CHROTE integration
should make the service boundary explicit.

Two default maintenance orders, `mol-dog-jsonl` and `mol-dog-reaper`, attempted
to resolve a Dolt runtime even in the file-backed spike. They are skipped in
`city.toml` for this city:

```toml
[orders]
skip = ["mol-dog-jsonl", "mol-dog-reaper"]
```

`gc session logs` did not find a provider-native transcript for the tmux-backed
mock session. For this spike, recoverable output is available through
`gc session peek` and the explicit mock-agent logs under `logs/`. This should
be treated as an adapter concern before using Gas City as the only transcript
source for CHROTE.

Real AI harnesses are not yet safe to attach. The mock agents run as the normal
`perttu` user, and Gas City settings include dangerous-mode prompt skipping for
managed sessions. Before connecting Codex, Claude Code, OpenCode, Pi, or Gemini,
CHROTE needs explicit adapter boundaries, environment scrubbing, transcript
policy, and approval rules.

File-backed Beads are acceptable for this spike but are not a production
durability proof. Dolt-backed behavior still needs separate validation.

## Recommendation

Use Gas City as a sidecar orchestration runtime for the next CHROTE
meta-harness slice.

The sidecar path is currently the best fit because it gives CHROTE durable
sessions, mail, sling, formulas, molecules, wisps, convoys, and a typed
HTTP/SSE control plane without requiring CHROTE to reimplement those primitives
first.

The next slice should not connect paid AI harnesses immediately. It should first
build a read-only CHROTE panel that queries the supervisor API and displays:

- running cities
- sessions
- mail counts
- routed work
- formula/molecule/wisp state
- recent events

After that panel is stable, add one real harness adapter at a time.

Follow-up Beads:

- `home-4xv.2`: Build CHROTE read-only Gas City observer
- `home-4xv.3`: Fix Gas City transcript recovery for CHROTE
- `home-4xv.4`: Validate Dolt-backed Gas City and mol-review-quorum
- `home-4xv.5`: Define Gas City real-harness safety boundary
