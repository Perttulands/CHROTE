#!/usr/bin/env python3
"""Validation tests for the Hermes -> Gas City ingress bridge (v0).

These are offline tests: gc is never called. The side-effecting dispatch path
is exercised through an injected fake runner so we can prove the bridge builds
the right argv arrays and pins the sender, without touching the live city.

Each test encodes WHY the behavior matters (Rule 7), not just what it does.

Required coverage (from bead home-qnzi acceptance):
  1. missing fields
  2. token-shaped fields
  3. unsupported target
  4. notify default false
  5. sender identity handling (anti-spoofing)

Run: python3 -m pytest tests/  OR  python3 tests/test_hermes_bridge.py
(no third-party deps required; falls back to a tiny runner if pytest absent).
"""

import importlib.util
import os
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
_BIN = os.path.join(_HERE, os.pardir, "bin", "hermes_bridge.py")
_spec = importlib.util.spec_from_file_location("hermes_bridge", _BIN)
hb = importlib.util.module_from_spec(_spec)
# Register before exec so dataclasses can resolve the module for field() types.
sys.modules["hermes_bridge"] = hb
_spec.loader.exec_module(hb)


# A canonical valid v0 envelope (synthetic; no real Discord/Hermes data).
def valid_envelope():
    return {
        "schema_version": "gascity.hermes_bridge.request.v0",
        "nonce": "HGB-TEST-0001",
        "origin": {
            "kind": "discord_message",
            "hermes_profile": "hermes-smoke",
            "discord_guild_id": "synthetic-guild",
            "discord_channel_id": "synthetic-channel",
            "discord_message_id": "synthetic-message-0001",
            "discord_user_id": "synthetic-user",
        },
        "auth": {
            "verified_by": "hermes",
            "bridge_actor": "local-hermes-bridge",
        },
        "request": {
            "title": "Synthetic ingress request HGB-TEST-0001",
            "body": "Synthetic Hermes/Discord-originated request.",
            "intent": "route_task",
            "target": "planner",
            "home_bead_id": "home-5qmz",
            "notify": False,
        },
        "safety": {"synthetic": True},
    }


class FakeRunner:
    """Records argv arrays and returns canned successful gc output."""

    def __init__(self):
        self.calls = []

    def __call__(self, argv, input=None, capture_output=False, text=False, check=False):
        self.calls.append({"argv": argv, "input": input})

        class R:
            pass

        r = R()
        r.returncode = 0
        r.stderr = ""
        if argv[:1] == ["gc"] and "sling" in argv:
            r.stdout = '{"success": true, "bead_id": "gc-99001", "routed": true}'
        elif "mail" in argv and "send" in argv:
            r.stdout = "Sent message gc-99002 to planner"
        else:
            r.stdout = ""
        return r


def expect_envelope_error(env, needle):
    try:
        hb.validate_and_normalize(env)
    except hb.EnvelopeError as e:
        assert needle in str(e), f"expected {needle!r} in error, got: {e}"
        return
    raise AssertionError(f"expected EnvelopeError containing {needle!r}, none raised")


# --- 1. MISSING FIELDS -----------------------------------------------------
# WHY: a request with no title/intent/nonce/origin.kind cannot be safely
# turned into an attributable artifact; we must reject rather than guess.

def test_missing_required_field_rejected():
    # These are reported by the missing-fields gate once schema_version is OK.
    for path in ("nonce", "request.title", "request.intent", "origin.kind"):
        env = valid_envelope()
        parts = path.split(".")
        node = env
        for p in parts[:-1]:
            node = node[p]
        del node[parts[-1]]
        expect_envelope_error(env, "missing required field")


def test_missing_schema_version_rejected():
    # A missing schema_version is caught by the (more specific) version gate,
    # which runs first. WHY: an envelope with no version cannot be trusted to
    # match the v0 contract at all, so the version error is the right signal.
    env = valid_envelope()
    del env["schema_version"]
    expect_envelope_error(env, "unsupported schema_version")


def test_empty_required_field_is_treated_as_missing():
    # WHY: an empty title is as useless as a missing one; blank != present.
    env = valid_envelope()
    env["request"]["title"] = ""
    expect_envelope_error(env, "missing required field")


# --- 2. TOKEN-SHAPED FIELDS ------------------------------------------------
# WHY: the bridge must never receive/log/forward credential material. A
# token-shaped key OR value anywhere in the tree is a hard reject BEFORE any
# gc call, so secrets cannot leak into mail bodies, logs, or beads.

def test_token_shaped_key_rejected():
    env = valid_envelope()
    env["auth"]["discord_bot_token"] = "redacted-but-the-key-name-is-the-problem"
    expect_envelope_error(env, "token-shaped field name")


def test_token_shaped_value_rejected_even_with_innocent_key():
    env = valid_envelope()
    # An obviously-fake three-segment (JWT/Discord-token) SHAPE under an
    # innocuous key name. Not a real credential; chosen to be self-evidently
    # synthetic so secret scanners do not flag it.
    env["request"]["body"] = "SYNTHETIC-NOT-A-REAL-TOKEN.aaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbb"
    expect_envelope_error(env, "token-shaped value")


def test_authorization_bearer_header_rejected():
    env = valid_envelope()
    env["origin"]["note"] = "Authorization: Bearer SYNTHETIC-NOT-A-REAL-BEARER-TOKEN"
    expect_envelope_error(env, "token-shaped value")


def test_raw_gateway_payload_rejected():
    # WHY: a raw Discord gateway frame (op/d) is the opposite of a sanitized
    # envelope; accepting it would mean Hermes never did the sanitization.
    env = {"op": 0, "d": {"content": "hi"}, "s": 5, "t": "MESSAGE_CREATE"}
    expect_envelope_error(env, "raw Discord gateway")


def test_raw_gateway_keys_in_origin_rejected():
    env = valid_envelope()
    env["origin"]["heartbeat_interval"] = 41250
    expect_envelope_error(env, "raw Discord gateway payload fields")


# --- 3. UNSUPPORTED TARGET -------------------------------------------------
# WHY: routing to an arbitrary alias could silently dispatch work to a
# credentialed/paid harness. Only operator/dispatcher inboxes are allowed
# without explicit approval.

def test_unsupported_target_rejected():
    env = valid_envelope()
    env["request"]["target"] = "codex-smoke"  # a real harness alias
    expect_envelope_error(env, "unsupported target")


def test_allowed_targets_accepted():
    for t in ("human", "planner"):
        env = valid_envelope()
        env["request"]["target"] = t
        req = hb.validate_and_normalize(env)
        assert req.target == t


def test_missing_target_defaults_to_human():
    env = valid_envelope()
    del env["request"]["target"]
    req = hb.validate_and_normalize(env)
    assert req.target == "human", "absent target must default to the operator inbox"


# --- 4. NOTIFY DEFAULT FALSE -----------------------------------------------
# WHY: a live nudge is an action beyond durable mail/task creation. Default
# must be silent; turning notify on must require explicit operator approval.

def test_notify_defaults_false_when_absent():
    env = valid_envelope()
    del env["request"]["notify"]
    req = hb.validate_and_normalize(env)
    assert req.notify is False
    assert hb.requires_approval(req) is None, "silent default needs no approval"


def test_notify_true_requires_approval():
    env = valid_envelope()
    env["request"]["notify"] = True
    req = hb.validate_and_normalize(env)
    assert req.notify is True
    assert hb.requires_approval(req) is not None, "notify=true must gate on approval"


def test_notify_true_without_approval_blocks_dispatch():
    env = valid_envelope()
    env["request"]["notify"] = True  # no approval block
    runner = FakeRunner()
    try:
        hb.dispatch(env, "/tmp/fake-city", runner=runner)
    except PermissionError:
        assert runner.calls == [], "no gc call may happen when approval is denied"
        return
    raise AssertionError("expected PermissionError for notify=true without approval")


def test_notify_true_with_explicit_approval_proceeds():
    env = valid_envelope()
    env["request"]["notify"] = True
    env["approval"] = {"operator_approved": True, "approved_action": "notify"}
    runner = FakeRunner()
    receipt = hb.dispatch(env, "/tmp/fake-city", runner=runner)
    assert receipt["artifacts"]["mail_id"] == "gc-99002"


# --- 5. SENDER IDENTITY HANDLING (anti-spoofing) ---------------------------
# WHY: the load-bearing safety property. External Discord identity must NEVER
# become the Gas City sender. The envelope cannot set --from, and dispatch
# must never put --from in the argv from envelope data.

def test_envelope_cannot_set_request_from():
    env = valid_envelope()
    env["request"]["from"] = "discord-user-666"
    expect_envelope_error(env, "not allowed")


def test_envelope_cannot_set_sender_identity_alias():
    for key in ("gc_from", "sender_identity"):
        env = valid_envelope()
        env["request"][key] = "spoofed"
        expect_envelope_error(env, "not allowed")


def test_envelope_cannot_set_auth_from():
    env = valid_envelope()
    env["auth"]["from"] = "discord-user-666"
    expect_envelope_error(env, "not allowed")


def test_dispatch_never_passes_from_in_argv():
    env = valid_envelope()
    runner = FakeRunner()
    hb.dispatch(env, "/tmp/fake-city", runner=runner)
    for call in runner.calls:
        assert "--from" not in call["argv"], (
            "the bridge must never set --from from envelope data; "
            f"argv leaked it: {call['argv']}"
        )


def test_sender_principal_is_pinned_local_writer():
    env = valid_envelope()
    req = hb.validate_and_normalize(env)
    assert req.writer_principal == hb.DEFAULT_WRITER_PRINCIPAL
    # And it does NOT come from any Discord field.
    assert req.writer_principal != env["origin"]["discord_user_id"]


def test_dispatch_uses_argv_arrays_not_shell_strings():
    # WHY: raw Discord text must never be shell-interpolated. The title may
    # contain shell metacharacters; it must arrive as one argv element.
    env = valid_envelope()
    env["request"]["title"] = "danger; rm -rf / `whoami` $(id)"
    runner = FakeRunner()
    hb.dispatch(env, "/tmp/fake-city", runner=runner)
    mail_call = [c for c in runner.calls if "mail" in c["argv"]][0]
    # The exact dangerous string is a single list element after -s.
    s_idx = mail_call["argv"].index("-s")
    assert mail_call["argv"][s_idx + 1] == "danger; rm -rf / `whoami` $(id)"
    # argv is a list, never a single shell string.
    assert isinstance(mail_call["argv"], list)


# --- happy-path receipt shape ----------------------------------------------

def test_route_task_returns_receipt_with_gc_ids():
    env = valid_envelope()
    runner = FakeRunner()
    receipt = hb.dispatch(env, "/tmp/fake-city", runner=runner)
    assert receipt["schema_version"] == "gascity.hermes_bridge.result.v0"
    assert receipt["nonce"] == "HGB-TEST-0001"
    assert receipt["artifacts"]["sling_bead_id"] == "gc-99001"
    assert receipt["artifacts"]["mail_id"] == "gc-99002"
    # sling routed first, then mail, both via argv arrays.
    assert "sling" in runner.calls[0]["argv"]
    assert "send" in runner.calls[1]["argv"]


def test_mail_only_intent_skips_sling():
    env = valid_envelope()
    env["request"]["intent"] = "mail_only"
    runner = FakeRunner()
    receipt = hb.dispatch(env, "/tmp/fake-city", runner=runner)
    assert receipt["artifacts"]["sling_bead_id"] is None
    assert all("sling" not in c["argv"] for c in runner.calls)
    assert receipt["artifacts"]["mail_id"] == "gc-99002"


def test_unsupported_intent_rejected():
    env = valid_envelope()
    env["request"]["intent"] = "launch_real_harness"
    expect_envelope_error(env, "unsupported intent")


# --- tiny no-dep runner ----------------------------------------------------

def _run_without_pytest():
    fns = [v for k, v in sorted(globals().items())
           if k.startswith("test_") and callable(v)]
    failures = 0
    for fn in fns:
        try:
            fn()
            print(f"PASS {fn.__name__}")
        except AssertionError as e:
            failures += 1
            print(f"FAIL {fn.__name__}: {e}")
        except Exception as e:  # noqa: BLE001
            failures += 1
            print(f"ERROR {fn.__name__}: {type(e).__name__}: {e}")
    print(f"\n{len(fns) - failures}/{len(fns)} passed")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(_run_without_pytest())
