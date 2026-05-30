# Gas City Hermes Ingress — Verification Evidence

Durable record for Beads `home-qnzi` (productize the Hermes → Gas City bridge),
the follow-up of `home-5qmz`. The bridge source lives at
`chrote/docs/gascity-harness-adapters/ingress/hermes/`; this is the focused
verification + rationale record (sibling of `gascity-opencode-parity.md`).

The bridge is an **ingress adapter** (sanitized envelope → native `gc`
primitives), deliberately a *different shape* from the harness-wrapper mold in
`../adapters/`. See `../ingress/hermes/README.md` for the full contract.

## Identity / writer-principal approach

The productization gate from `home-5qmz` was: a valid `hermes-bridge` Gas City
identity **or** a documented local writer principal that **cannot** spoof
arbitrary Discord users via `--from`.

**Chosen:** a documented **fixed local writer principal** (`"human"`, the city
default sender). The envelope can never set the Gas City sender; Discord
attribution is body metadata only. Rationale and the upgrade path to a real
`hermes-bridge` session identity are in the README. The anti-spoofing property
holds by construction: the bridge never appends `--from` from envelope data and
rejects any envelope sender field.

## Live re-verification of the `--from` spoof rejection (2026-05-27)

Run from a non-Hermes Ubuntu shell, supervisor up on `127.0.0.1:8372`
(localhost-only, unchanged):

```
$ gc --city /home/perttu/gascity mail send planner \
    --from "discord-user-666999" \
    -s "SYNTHETIC spoof attempt HGB-PROD-VERIFY" \
    -m "synthetic; should be rejected"
gc mail send: invalid sender "": session not found: "discord-user-666999"
exit=1
```

This reproduces the prototype's documented failure and confirms the gate is
real and current. No artifact was created (the command failed before writing).
The string was synthetic; no real Discord user or token was used.

## Validation tests (offline, no gc call)

`ingress/hermes/tests/test_hermes_bridge.py`, Python 3 stdlib only:

```
$ python3 tests/test_hermes_bridge.py
... (24 tests) ...
24/24 passed        (exit 0)
```

The five bead-required cases, each asserting WHY (not just what):

| acceptance case        | tests |
|------------------------|-------|
| missing fields         | `test_missing_required_field_rejected`, `test_missing_schema_version_rejected`, `test_empty_required_field_is_treated_as_missing` |
| token-shaped fields    | `test_token_shaped_key_rejected`, `test_token_shaped_value_rejected_even_with_innocent_key`, `test_authorization_bearer_header_rejected`, `test_raw_gateway_payload_rejected`, `test_raw_gateway_keys_in_origin_rejected` |
| unsupported target     | `test_unsupported_target_rejected`, `test_allowed_targets_accepted`, `test_missing_target_defaults_to_human` |
| notify default false   | `test_notify_defaults_false_when_absent`, `test_notify_true_requires_approval`, `test_notify_true_without_approval_blocks_dispatch`, `test_notify_true_with_explicit_approval_proceeds` |
| sender identity        | `test_envelope_cannot_set_request_from`, `test_envelope_cannot_set_sender_identity_alias`, `test_envelope_cannot_set_auth_from`, `test_dispatch_never_passes_from_in_argv`, `test_sender_principal_is_pinned_local_writer` |

Plus argv-array proof (`test_dispatch_uses_argv_arrays_not_shell_strings`:
a title containing `; rm -rf / \`whoami\` $(id)` arrives as a single argv
element, never shell-interpolated) and receipt-shape tests.

## CLI reject-path checks (no gc call, exit codes)

```
$ python3 bin/hermes_bridge.py --validate-only < examples/request.v0.example.json
{ "ok": true, "intent": "route_task", "target": "planner",
  "notify": false, "sender_principal": "human", "needs_approval": null }   exit 0

# token-shaped field  -> exit 65 "envelope rejected"
# unsupported target  -> exit 65 "envelope rejected"
# notify=true, no approval, real dispatch -> exit 77 "operator approval required"
#   (bailed BEFORE any gc call; the target city was never touched)
```

## Safety posture (verified)

- **No real Discord/Hermes token** appears in code, tests, docs, examples,
  schema, Beads, logs, or diffs. The only token-shaped string in the tree is the
  synthetic negative-test fixtures (e.g. `"...Bearer SYNTHETIC-NOT-A-REAL-..."`),
  used to prove the detector rejects token-shaped input.
- **No `--from` from envelope data**; envelope sender fields rejected.
- **Localhost-only**: the bridge opens no listener; the supervisor stays on
  `127.0.0.1:8372`.
- **Default-deny**: notify defaults false; notify=true and off-allowlist targets
  require explicit operator approval; real harness starts / file mutation /
  service changes are out of scope and never performed by the bridge.
- **No durable-memory writes**: the bridge writes only disposable Gas City
  runtime artifacts (mail + routed task). `/home/perttu` Beads stays canonical.

## What was NOT done (deliberately)

- **No live dispatch run** that creates new `gc-*` mail/task artifacts. The
  offline tests + the live spoof-rejection check fully exercise the contract
  without adding runtime noise to the city; a live dispatch needs only the
  documented one-liner and explicit operator go-ahead.
- **No `hermes-bridge` standing session identity** created (v0 uses the fixed
  local writer principal; upgrade path documented).
- **No Hermes-side wiring / network listener** (out of scope; the bridge is a
  local stdin→stdout function for now).
- **No commit** (all CHROTE 3.0 adapter work stays staged for a single later
  commit per `home-qnzi` placement note).
```
