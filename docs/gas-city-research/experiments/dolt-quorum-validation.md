# Gas City Dolt And Quorum Validation

Inspected: 2026-05-27 01:32 Europe/Helsinki.

Related beads:

- `home-4xv.4`: Validate Dolt-backed Gas City and mol-review-quorum.
- `home-d0lv`: Add offline no-register Gas City init path for disposable proofs.

## Recommendation

This does not block CHROTE central adoption as a reversible Gas City workflow
substrate, as long as `/home/perttu` Beads remains canonical work truth and Gas
City remains an orchestration/runtime projection.

It does block relying on either of these as production-ready CHROTE 3.0
reliability primitives:

- live Dolt-backed Gas City durability for `/home/perttu/gascity`;
- `mol-review-quorum` as the reusable review-quorum workflow.

Use the local `plan-review-synthesis` formula or a CHROTE-owned projection for
near-term central workflow work. Do not migrate the live sidecar from
file-backed state or route live quorum reviews until the no-register disposable
proof path in `home-d0lv` exists and `mol-review-quorum` compiles with
diagnostics.

## Proven

- Dolt is installed at `/home/linuxbrew/.linuxbrew/bin/dolt`.
- `dolt --version` does not work with this Dolt CLI; it exits with
  `unknown option "version"`.
- `dolt version` reports `dolt version 2.0.6` and warns that 2.0.7 is newer.
- `gc version` reports `dev`.
- The live Gas City workspace is `/home/perttu/gascity`.
- `city.toml` sets `[beads] provider = "file"`.
- `gc config show` resolves the live provider as `file` and includes the core
  and maintenance packs.
- `.beads/metadata.json` exists and names an embedded Dolt Beads database, but
  the live Gas City provider is still `file`; the Dolt-backed workspace marker
  is not the active Gas City bead provider.
- `gc doctor --verbose` passes 46 checks. Its Dolt checks are skipped because
  the live city is file-backed or Dolt is skipped.
- `gc beads health` reports `Beads provider: healthy`.
- `gc bd where` refuses the live city with `only supported for bd-backed beads
  providers (resolved "file" for /home/perttu/gascity)`.
- `gc formula list` includes both `plan-review-synthesis` and
  `mol-review-quorum`.
- `plan-review-synthesis` exists as a local city formula at
  `/home/perttu/gascity/formulas/plan-review-synthesis.toml`.
- `gc formula show plan-review-synthesis --var topic="CHROTE meta-harness"`
  succeeds and shows the four expected steps: plan, review-a, review-b, and
  synthesis.
- `mol-review-quorum` exists locally through the installed core pack at
  `/home/perttu/gascity/.gc/system/packs/core/formulas/mol-review-quorum.toml`.
  It is not only an upstream-only formula.
- `mol-review-quorum.toml` parses as TOML, declares `formula =
  "mol-review-quorum"`, `version = 2`, `contract = "graph.v2"`, 11 variables,
  and 3 steps.

## Blocked

Safe read-only CLI validation of `mol-review-quorum` is currently blocked:

- `gc formula show mol-review-quorum` exits 1 with no diagnostic output.
- The same command exits 1 with no diagnostic output even when all declared
  required variables are supplied.
- `gc formula show mol-scoped-work --var issue=home-4xv.4` also exits 1 with
  no diagnostic output. That suggests the failure is likely in the current
  graph.v2 formula show/compile path, not only in the review-quorum formula
  file.

I did not run `gc formula cook mol-review-quorum`. `gc formula cook` creates
real bead records in the current store, so running it in `/home/perttu/gascity`
would mutate the live file-backed sidecar. A sling or smoke run would be even
more mutating because it can route work and wake sessions.

The Dolt-backed proof remains blocked for repeatable validation because the
previous positive disposable proof used `gc init`, and that path registered or
started through the live supervisor. `home-d0lv` is the correct follow-up for a
safe offline/no-register proof.

## Live State Note

I took hashes around the read-only inspection:

```text
before .gc/beads.json:  ddfe32b1bcd17b61e2b2f9309d9300422dc1e2e46311b4ee8d27dacfebebe8c9
after  .gc/beads.json:  129d7035e4f378ee5dbd6981fc199ad43483de048ceafba3c4492caf66af3ecb
before .gc/events.jsonl: 1c89df8e80d65aa54cee586194f78510db120a0c4846bb63146730567b8dae0b
after  .gc/events.jsonl: b19ed315f1f3b3a4f442b8f964d020187c06777b243a00393d9bb995dbacf803
```

The live controller was running scheduled orders during the inspection. The
event tail showed `beads-health`, `gate-sweep`, and `order-tracking-sweep`
fire/complete events, plus `session.woke` records during the same window. I did
not run register, start, stop, init, cook, sling, `gc bd update`, or direct file
edits in `/home/perttu/gascity`, but the live runtime files were not
byte-stable. Future strict no-mutation proofs should use a stopped or
disposable city, not the live sidecar.

## Safe Next Sequence

Do not run this against `/home/perttu/gascity`. Run it only after `home-d0lv`
provides a verified no-register/no-start city scaffold:

```bash
dolt version
gc config show
gc doctor --verbose
gc bd where
gc formula show mol-review-quorum \
  --var subject=home-4xv.4 \
  --var lane_one_id=codex-doc-review \
  --var lane_one_provider=codex \
  --var lane_one_model=gpt-5 \
  --var lane_one_target=reviewer-a \
  --var lane_two_id=pi-doc-review \
  --var lane_two_provider=pi \
  --var lane_two_model=default \
  --var lane_two_target=reviewer-b \
  --var synthesis_target=planner
```

Only after the preview succeeds in that disposable bd-backed city, run the
matching `gc formula cook mol-review-quorum ...` there and inspect the created
records. Do not count a live cook, a file-backed cook, or a graph.v2 silent
failure as validation.

## CHROTE Decision

Proceed with CHROTE 3.0 central workflow adoption only under the existing
bounded framing:

- Beads remains canonical for work truth.
- Context Citadel remains canonical for durable context.
- Gas City owns runtime orchestration, mail, formulas, events, and session
  lifecycle only.
- CHROTE exposes a read-only or explicitly gated operator surface.

Dolt and `mol-review-quorum` should stay reliability follow-up work, not a
gate for continuing the read-only observer, projection schema, or first real
identity slices.
