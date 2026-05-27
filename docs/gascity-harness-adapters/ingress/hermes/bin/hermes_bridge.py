#!/usr/bin/env python3
"""Hermes -> Gas City ingress bridge adapter (v0).

This is an INGRESS adapter: it accepts a sanitized Hermes/Discord request
envelope (NOT a raw Discord gateway event, NOT token material) and turns it
into native Gas City artifacts (mail and, for actionable work, a routed task
bead) by calling the `gc` CLI with ARGV ARRAYS.

It is deliberately NOT the harness-wrapper mold in ../../adapters/: it does not
register a long-lived gc-owned session that wraps a CLI. It is a tiny
validate -> normalize -> dispatch function plus a localhost-only entry point.

Safety invariants enforced here (see also the README + prototype doc):

* Sender identity is PINNED to a fixed local writer principal (default
  "human"). The envelope can NEVER set the Gas City `--from`. Discord
  user/channel/message ids are carried as body metadata only. This is what
  prevents spoofing an arbitrary Discord user through `--from`: the spoofing
  surface does not exist.
* Token-shaped fields and raw gateway payload dumps are REJECTED before any
  gc call.
* `notify` defaults to false; notification and every action beyond
  mail/task creation require an explicit operator approval token.
* gc is invoked with argv arrays via subprocess (shell=False). Raw Discord
  text is passed as a single list element, never interpolated into a shell
  string.

No Discord/Hermes tokens are read, logged, persisted, or forwarded.
"""

from __future__ import annotations

import json
import re
import subprocess
from dataclasses import dataclass, field
from typing import Any

REQUEST_SCHEMA = "gascity.hermes_bridge.request.v0"
RESULT_SCHEMA = "gascity.hermes_bridge.result.v0"

# The fixed local writer principal. The bridge writes to Gas City AS this
# identity. It is intentionally NOT derived from the envelope so external
# Discord users can never become a Gas City sender. "human" is the city's
# default sender (the smoke proved mail from this principal succeeds while an
# arbitrary --from is rejected). An operator may later point this at a real
# Gas City `hermes-bridge` session identity; the spoofing surface stays closed
# either way because the value is config, never envelope-controlled.
DEFAULT_WRITER_PRINCIPAL = "human"

# Targets the bridge may route to without operator approval. "human" and
# "planner" are operator/dispatcher inboxes; anything else (e.g. a real paid
# harness alias) requires explicit approval to avoid silently dispatching work
# to a credentialed session.
ALLOWED_TARGETS = frozenset({"human", "planner"})

# Intents the v0 bridge understands.
SUPPORTED_INTENTS = frozenset({"mail_only", "route_task"})

# Required envelope fields (dotted paths).
REQUIRED_FIELDS = (
    "schema_version",
    "nonce",
    "origin.kind",
    "request.title",
    "request.intent",
)

# Heuristics for token-shaped values. The bridge must reject anything that
# smells like a credential or a raw gateway dump rather than risk logging it.
_TOKEN_KEY_RE = re.compile(
    r"(token|secret|password|passwd|api[-_]?key|authorization|bearer|"
    r"client[-_]?secret|private[-_]?key|ssh[-_]?key|cookie|session[-_]?token)",
    re.IGNORECASE,
)
# Value shapes that look like real secrets regardless of key name.
_TOKEN_VALUE_RES = (
    re.compile(r"^[A-Za-z0-9_-]{2,}\.[A-Za-z0-9_-]{2,}\.[A-Za-z0-9_-]{2,}$"),  # JWT-ish
    re.compile(r"\b(?:Bot|Bearer)\s+[A-Za-z0-9._-]{10,}", re.IGNORECASE),       # Discord/HTTP auth header
    re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{10,}"),                              # slack-ish
    re.compile(r"\b[A-Za-z0-9_-]{24,}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}"), # Discord bot token shape
    re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),                         # PEM key
)
# Keys that indicate a raw Discord gateway payload dump rather than a sanitized
# envelope. Their presence is a hard reject.
_RAW_GATEWAY_KEYS = frozenset(
    {"op", "d", "s", "t", "heartbeat_interval", "session_id", "_trace",
     "gateway", "raw", "payload", "intents"}
)


class EnvelopeError(ValueError):
    """Raised when the request envelope is invalid or unsafe."""


@dataclass
class NormalizedRequest:
    """A validated, normalized, safe-to-dispatch request."""

    nonce: str
    title: str
    body: str
    intent: str
    target: str
    notify: bool
    writer_principal: str
    origin_summary: str
    home_bead_id: str | None
    approval: dict[str, Any] = field(default_factory=dict)


# --------------------------------------------------------------------------
# Pure validation / normalization (no gc calls; fully unit-testable offline)
# --------------------------------------------------------------------------

def _get(d: dict, dotted: str):
    cur: Any = d
    for part in dotted.split("."):
        if not isinstance(cur, dict) or part not in cur:
            return None
        cur = cur[part]
    return cur


def _looks_like_token_value(value: str) -> bool:
    return any(rx.search(value) for rx in _TOKEN_VALUE_RES)


def _scan_for_tokens(node: Any, path: str = "") -> None:
    """Walk the envelope and reject token-shaped keys/values anywhere."""
    if isinstance(node, dict):
        for k, v in node.items():
            if _TOKEN_KEY_RE.search(str(k)):
                raise EnvelopeError(
                    f"token-shaped field name rejected at {path}{k!r}"
                )
            _scan_for_tokens(v, f"{path}{k}.")
    elif isinstance(node, list):
        for i, v in enumerate(node):
            _scan_for_tokens(v, f"{path}{i}.")
    elif isinstance(node, str):
        if _looks_like_token_value(node):
            raise EnvelopeError(
                f"token-shaped value rejected at {path.rstrip('.')!r}"
            )


def _reject_raw_gateway(envelope: dict) -> None:
    """Reject envelopes that carry a raw Discord gateway payload dump."""
    origin = envelope.get("origin")
    if isinstance(origin, dict):
        leaked = sorted(set(origin) & _RAW_GATEWAY_KEYS)
        if leaked:
            raise EnvelopeError(
                f"raw Discord gateway payload fields rejected in origin: {leaked}"
            )
    # A `d`+`op` pair at the top level is the canonical raw gateway frame.
    if {"op", "d"} <= set(envelope):
        raise EnvelopeError("raw Discord gateway frame (op/d) rejected")


def validate_and_normalize(envelope: Any) -> NormalizedRequest:
    """Validate a v0 request envelope and return a NormalizedRequest.

    Raises EnvelopeError on any missing field, token-shaped field, raw
    gateway dump, unsupported target, or unsupported intent.
    """
    if not isinstance(envelope, dict):
        raise EnvelopeError("envelope must be a JSON object")

    # 1. Reject raw gateway dumps first (cheapest hard-reject).
    _reject_raw_gateway(envelope)

    # 2. Reject token-shaped fields anywhere in the tree.
    _scan_for_tokens(envelope)

    # 3. Schema version must match exactly.
    if envelope.get("schema_version") != REQUEST_SCHEMA:
        raise EnvelopeError(
            f"unsupported schema_version "
            f"{envelope.get('schema_version')!r}; expected {REQUEST_SCHEMA!r}"
        )

    # 4. Required fields must be present and non-empty.
    missing = [
        f for f in REQUIRED_FIELDS
        if _get(envelope, f) in (None, "")
    ]
    if missing:
        raise EnvelopeError(f"missing required field(s): {missing}")

    # 5. The envelope must NOT try to set a Gas City sender identity. This is
    #    the anti-spoofing gate: external attribution can never become --from.
    for forbidden in ("from", "gc_from", "sender_identity"):
        if forbidden in envelope.get("request", {}):
            raise EnvelopeError(
                f"request.{forbidden} is not allowed: the bridge pins the "
                f"Gas City sender; Discord identity is body metadata only"
            )
    if "from" in envelope.get("auth", {}):
        raise EnvelopeError(
            "auth.from is not allowed: the bridge pins the Gas City sender"
        )

    intent = _get(envelope, "request.intent")
    if intent not in SUPPORTED_INTENTS:
        raise EnvelopeError(
            f"unsupported intent {intent!r}; "
            f"expected one of {sorted(SUPPORTED_INTENTS)}"
        )

    # 6. Target validation. mail_only defaults to the operator inbox; an
    #    explicit target must be on the allowlist. Off-allowlist targets need
    #    operator approval (handled in the approval gate, but rejected here for
    #    v0 to keep the dispatch path tiny and safe).
    target = _get(envelope, "request.target") or "human"
    if target not in ALLOWED_TARGETS:
        raise EnvelopeError(
            f"unsupported target {target!r}; "
            f"allowed without approval: {sorted(ALLOWED_TARGETS)}"
        )

    # 7. notify defaults to false. Anything truthy requires approval (checked
    #    by the approval gate in dispatch()).
    notify = bool(_get(envelope, "request.notify") or False)

    # 8. Build a sanitized one-line origin summary for the mail body. Only the
    #    sanitized attribution fields are echoed; nothing token-shaped survived
    #    the scan above.
    origin = envelope.get("origin", {})
    origin_summary = "; ".join(
        f"{k}={origin.get(k)}"
        for k in ("kind", "hermes_profile", "discord_guild_id",
                  "discord_channel_id", "discord_message_id", "discord_user_id")
        if origin.get(k)
    )

    return NormalizedRequest(
        nonce=str(_get(envelope, "nonce")),
        title=str(_get(envelope, "request.title")),
        body=str(_get(envelope, "request.body") or ""),
        intent=intent,
        target=target,
        notify=notify,
        writer_principal=DEFAULT_WRITER_PRINCIPAL,
        origin_summary=origin_summary,
        home_bead_id=_get(envelope, "request.home_bead_id"),
        approval=envelope.get("approval", {}) or {},
    )


# --------------------------------------------------------------------------
# Approval gate
# --------------------------------------------------------------------------

def requires_approval(req: NormalizedRequest) -> str | None:
    """Return a reason string if the request needs operator approval, else None.

    v0 policy: mail/task creation is allowed by default. Notification (a live
    nudge) and any future action beyond mail/task creation require an explicit
    operator approval token in the envelope's `approval` block.
    """
    if req.notify:
        return "notify=true requires operator approval (live nudge is an action beyond mail/task creation)"
    return None


def _approval_granted(req: NormalizedRequest, reason: str) -> bool:
    """Check the envelope carries an explicit operator approval token."""
    appr = req.approval
    return bool(
        isinstance(appr, dict)
        and appr.get("operator_approved") is True
        and isinstance(appr.get("approved_action"), str)
        and appr.get("approved_action")
    )


# --------------------------------------------------------------------------
# Dispatch (calls gc with argv arrays; the only side-effecting path)
# --------------------------------------------------------------------------

def build_sling_argv(city: str, req: NormalizedRequest) -> list[str]:
    """argv for routing a task bead (text from stdin, never shell-interpolated)."""
    return [
        "gc", "--city", city, "sling", req.target,
        "--stdin", "--json", "--no-convoy", "--no-formula",
    ]


def build_mail_argv(city: str, req: NormalizedRequest, body: str) -> list[str]:
    """argv for sending mail. Sender is PINNED (no --from from the envelope)."""
    argv = [
        "gc", "--city", city, "mail", "send", req.target,
        "-s", req.title,
        "-m", body,
    ]
    # The writer principal is the city default ("human"); gc uses that when
    # --from is omitted. We deliberately do NOT append --from at all, so there
    # is no envelope-controlled path to a spoofed sender. Only if an operator
    # later configures a real, existing Gas City identity AND notify-style
    # approval is granted would --from be set, and even then to the configured
    # principal, never to envelope data.
    return argv


def _sling_stdin(req: NormalizedRequest) -> str:
    """The task bead text: title line, then sanitized body + origin metadata."""
    lines = [req.title, req.body, ""]
    lines.append(f"nonce={req.nonce}")
    if req.origin_summary:
        lines.append(f"origin: {req.origin_summary}")
    lines.append(f"bridge_writer={req.writer_principal}")
    if req.home_bead_id:
        lines.append(f"home_bead={req.home_bead_id}")
    lines.append("Source: Hermes ingress bridge (sanitized envelope; no Discord token used).")
    return "\n".join(lines)


def _mail_body(req: NormalizedRequest, sling_bead_id: str | None) -> str:
    parts = [
        req.body or req.title,
        f"nonce={req.nonce}",
        f"bridge_writer={req.writer_principal}",
    ]
    if req.origin_summary:
        parts.append(f"origin: {req.origin_summary}")
    if req.home_bead_id:
        parts.append(f"home_bead={req.home_bead_id}")
    if sling_bead_id:
        parts.append(f"routed_task={sling_bead_id}")
    parts.append("No Discord token or live notification was used.")
    return " | ".join(parts)


def dispatch(
    envelope: Any,
    city: str,
    *,
    runner=subprocess.run,
) -> dict[str, Any]:
    """Validate + dispatch an envelope, returning a v0 result receipt.

    `runner` is injectable for tests so no real gc call happens offline.
    Raises EnvelopeError (validation) or PermissionError (approval gate).
    """
    req = validate_and_normalize(envelope)

    reason = requires_approval(req)
    if reason and not _approval_granted(req, reason):
        raise PermissionError(reason)

    commands: list[str] = []
    sling_bead_id: str | None = None

    # Route a task bead first (so its id can be referenced in the mail body).
    if req.intent == "route_task":
        sling_argv = build_sling_argv(city, req)
        commands.append(" ".join(sling_argv))
        proc = runner(
            sling_argv,
            input=_sling_stdin(req),
            capture_output=True,
            text=True,
            check=False,
        )
        if proc.returncode != 0:
            raise RuntimeError(
                f"gc sling failed (exit {proc.returncode}): {proc.stderr.strip()}"
            )
        try:
            sling_bead_id = json.loads(proc.stdout).get("bead_id")
        except (ValueError, AttributeError):
            sling_bead_id = None

    # Always send a mail receipt for operator/dispatcher visibility.
    mail_argv = build_mail_argv(city, req, _mail_body(req, sling_bead_id))
    commands.append(" ".join(mail_argv))
    mproc = runner(mail_argv, capture_output=True, text=True, check=False)
    if mproc.returncode != 0:
        raise RuntimeError(
            f"gc mail send failed (exit {mproc.returncode}): {mproc.stderr.strip()}"
        )
    # gc prints "Sent message gc-XXXX to <target>"; extract the id.
    m = re.search(r"\b(gc-\d+)\b", mproc.stdout)
    mail_id = m.group(1) if m else None

    return {
        "schema_version": RESULT_SCHEMA,
        "nonce": req.nonce,
        "city": city,
        "artifacts": {
            "mail_id": mail_id,
            "sling_bead_id": sling_bead_id,
        },
        "sender_principal": req.writer_principal,
        "commands": commands,
        "notes": [
            "No Discord token or raw Discord message was used.",
            "Sender pinned to local writer principal; --from never set from envelope.",
            f"notify requested: {req.notify}.",
        ],
    }


# --------------------------------------------------------------------------
# Localhost-only entry point
# --------------------------------------------------------------------------

def main(argv: list[str] | None = None) -> int:
    import argparse
    import sys

    p = argparse.ArgumentParser(
        description="Hermes -> Gas City ingress bridge (v0). Reads a sanitized "
                    "request envelope on stdin, writes a v0 result receipt on stdout."
    )
    p.add_argument("--city", default="/home/perttu/gascity",
                   help="Gas City city root (localhost-only; default %(default)s)")
    p.add_argument("--validate-only", action="store_true",
                   help="validate + normalize the envelope and print the plan; do NOT call gc")
    args = p.parse_args(argv)

    try:
        envelope = json.load(sys.stdin)
    except ValueError as e:
        print(json.dumps({"error": f"invalid JSON envelope: {e}"}), file=sys.stderr)
        return 64

    try:
        if args.validate_only:
            req = validate_and_normalize(envelope)
            reason = requires_approval(req)
            print(json.dumps({
                "ok": True,
                "intent": req.intent,
                "target": req.target,
                "notify": req.notify,
                "sender_principal": req.writer_principal,
                "needs_approval": reason,
            }, indent=2))
            return 0
        receipt = dispatch(envelope, args.city)
        print(json.dumps(receipt, indent=2))
        return 0
    except EnvelopeError as e:
        print(json.dumps({"error": "envelope rejected", "detail": str(e)}), file=sys.stderr)
        return 65
    except PermissionError as e:
        print(json.dumps({"error": "operator approval required", "detail": str(e)}), file=sys.stderr)
        return 77
    except RuntimeError as e:
        print(json.dumps({"error": "gc dispatch failed", "detail": str(e)}), file=sys.stderr)
        return 70


if __name__ == "__main__":
    raise SystemExit(main())
