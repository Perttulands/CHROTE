#!/usr/bin/env python3
"""Verify tmux recovery outcomes against an accepted manifest."""

from __future__ import annotations

import argparse
from copy import deepcopy
import json
import os
import pwd
import re
import subprocess
import time
from pathlib import Path
from typing import Any, Callable
from urllib import error, request

import collector
import manifest as manifest_lib
import snapshot


UNIT_RE = re.compile(r"^[A-Za-z0-9_.@:-]+\.service$")
DEFAULT_STABILITY_SECONDS = 30


class SystemdUserStatusRunner:
    def __init__(self, *, current_user: str | None = None, command_runner: Any | None = None) -> None:
        self.current_user = current_user
        self.command_runner = command_runner

    def check(self, probe: dict[str, Any], unix_user: str = "") -> dict[str, Any]:
        if probe.get("kind") != "systemd-user":
            return {"ok": False, "error": "unsupported status probe"}
        actual_user = self.current_user if self.current_user is not None else pwd.getpwuid(os.geteuid()).pw_name
        if unix_user and actual_user != unix_user:
            return {"ok": False, "error": f"systemd status must run as requested unix user {unix_user}; current user {actual_user}"}
        unit = str(probe.get("unit", "")).strip()
        if not UNIT_RE.match(unit):
            return {"ok": False, "error": "missing systemd unit"}
        argv = ["systemctl", "--user", "show", unit, "--property=ActiveState", "--value"]
        if self.command_runner is not None:
            try:
                active_state = self.command_runner.run(argv).strip()
            except Exception as exc:
                return {"ok": False, "error": str(exc)}
        else:
            proc = subprocess.run(
                argv,
                text=True,
                capture_output=True,
                check=False,
            )
            if proc.returncode != 0:
                return {"ok": False, "error": proc.stderr.strip() or proc.stdout.strip()}
            active_state = proc.stdout.strip()
        return {"ok": active_state == probe.get("expectActiveState", "active"), "activeState": active_state}


class HTTPRunner:
    def __init__(self, opener: Any | None = None) -> None:
        self.opener = opener or request.build_opener(_NoRedirect)

    def check(self, probe: dict[str, Any]) -> dict[str, Any]:
        if probe.get("kind") != "http-get":
            return {"ok": False, "error": "unsupported helper endpoint probe"}
        expected_status = int(probe.get("expectStatus", 200))
        try:
            req = request.Request(str(probe.get("url")), method="GET")
            with self.opener.open(req, timeout=float(probe.get("timeoutSeconds", 5))) as resp:
                status = resp.status
        except error.HTTPError as exc:
            return {"ok": exc.code == expected_status, "status": exc.code, "error": str(exc)}
        except error.URLError as exc:
            return {"ok": False, "error": str(exc.reason)}
        return {"ok": status == expected_status, "status": status}


class _NoRedirect(request.HTTPRedirectHandler):
    def redirect_request(self, req: Any, fp: Any, code: int, msg: str, headers: Any, newurl: str) -> None:
        return None


def verify_manifest(
    manifest_doc: dict[str, Any],
    *,
    observed_sessions: list[dict[str, Any]] | None = None,
    observed_provider: Callable[[], list[dict[str, Any]]] | None = None,
    status_runner: Any | None = None,
    http_runner: Any | None = None,
    stability_seconds: float = 0,
    sleep: Callable[[float], Any] = time.sleep,
    topology_only: bool = False,
) -> dict[str, Any]:
    manifest_lib.validate_manifest(manifest_doc)
    if observed_provider is None:
        first_observed = observed_sessions or []
    else:
        first_observed = observed_provider()
    first = _verify_once(manifest_doc, first_observed, status_runner=status_runner, http_runner=http_runner, topology_only=topology_only)
    if stability_seconds and first["ok"]:
        sleep(stability_seconds)
        second_observed = observed_provider() if observed_provider is not None else observed_sessions or []
        second = _verify_once(manifest_doc, second_observed, status_runner=status_runner, http_runner=http_runner, topology_only=topology_only)
        second["stabilitySamples"] = 2
        second["stabilitySeconds"] = stability_seconds
        if topology_only:
            second["mode"] = "topology-only"
        return second
    first["stabilitySamples"] = 1
    first["stabilitySeconds"] = stability_seconds
    if topology_only:
        first["mode"] = "topology-only"
    return first


def _verify_once(
    manifest_doc: dict[str, Any],
    observed_sessions: list[dict[str, Any]],
    *,
    status_runner: Any | None,
    http_runner: Any | None,
    topology_only: bool,
) -> dict[str, Any]:
    observed_by_key = {_session_key(item): item for item in observed_sessions if isinstance(item, dict)}
    results: list[dict[str, Any]] = []
    for expected in manifest_doc["sessions"]:
        result = _verify_session(
            expected,
            observed_by_key.get(_session_key(expected)),
            status_runner=status_runner,
            http_runner=http_runner,
            topology_only=topology_only,
        )
        results.append(result)
    return {"ok": all(item["ok"] for item in results), "sessions": results}


def _verify_session(
    expected: dict[str, Any],
    observed: dict[str, Any] | None,
    *,
    status_runner: Any | None,
    http_runner: Any | None,
    topology_only: bool,
) -> dict[str, Any]:
    name = expected["sessionName"]
    result = {"sessionName": name, "unixUser": expected.get("unixUser", ""), "ok": True, "errors": []}
    errors: list[str] = result["errors"]

    if manifest_lib.is_managed_session(expected):
        runner = status_runner or SystemdUserStatusRunner()
        status = runner.check(expected["statusProbe"], unix_user=expected.get("unixUser", ""))
        result["managedStatus"] = status
        if not status.get("ok"):
            errors.append("managed status probe failed")

    expected_descriptors = expected.get("descriptors", [])
    if expected_descriptors:
        if observed is None and not manifest_lib.is_managed_session(expected):
            errors.append("missing live session evidence")
        observed_descriptors = observed.get("descriptors", []) if isinstance(observed, dict) else []
        expected_index = _logical_pane_index(expected_descriptors)
        observed_index = _logical_pane_index(observed_descriptors)
        _check_pane_set(expected, expected_index, observed_index, errors)
        for logical_key, desc in expected_index["by_logical"].items():
            if desc.get("mode") == "managed":
                continue
            candidate = observed_index["by_logical"].get(logical_key)
            if candidate is None or not _descriptor_matches(desc, candidate, topology_only=topology_only):
                errors.append(f"missing expected descriptor {desc.get('mode')}/{desc.get('workloadKind')} pane {desc.get('topology', {}).get('paneId', '')}")
    else:
        expected_index = _logical_pane_index([])
        observed_index = _logical_pane_index([])
    _check_verification_records(expected, observed, http_runner, errors, topology_only=topology_only, expected_index=expected_index, observed_index=observed_index)

    result["ok"] = not errors
    return result


def _check_helper(probe: dict[str, Any], http_runner: Any | None, errors: list[str]) -> None:
    runner = http_runner or HTTPRunner()
    status = runner.check(probe)
    if not status.get("ok"):
        errors.append("helper endpoint probe failed")


def _check_verification_records(
    expected_session: dict[str, Any],
    observed_session: dict[str, Any] | None,
    http_runner: Any | None,
    errors: list[str],
    *,
    topology_only: bool,
    expected_index: dict[str, Any],
    observed_index: dict[str, Any],
) -> None:
    expected_records = expected_session.get("verification", [])
    if not expected_records:
        return
    observed_records = observed_session.get("verification", []) if isinstance(observed_session, dict) else []
    observed_by_logical: dict[tuple[Any, ...], dict[str, Any]] = {}
    for item in observed_records:
        if not isinstance(item, dict):
            continue
        observed_logical = observed_index["by_provenance"].get(_verification_key(item))
        if observed_logical is not None:
            observed_by_logical[observed_logical] = item
    for record in expected_records:
        if not isinstance(record, dict):
            continue
        provenance_key = _verification_key(record)
        logical_key = expected_index["by_provenance"].get(provenance_key)
        if logical_key is None:
            errors.append(f"missing pane verification descriptor {provenance_key}")
            continue
        observed = observed_by_logical.get(logical_key)
        if observed is None:
            errors.append(f"missing pane verification {logical_key}")
            continue
        if not _pane_status_record_healthy(record, observed, topology_only=topology_only):
            errors.append(f"pane health mismatch {logical_key}")
        if "helperEndpoint" in record:
            _check_helper(record["helperEndpoint"], http_runner, errors)


def _check_pane_set(expected_session: dict[str, Any], expected_index: dict[str, Any], observed_index: dict[str, Any], errors: list[str]) -> None:
    expected_keys = set(expected_index["by_logical"])
    observed_keys = set(observed_index["by_logical"])
    if observed_index["duplicates"]:
        errors.append("duplicate observed pane " + ", ".join(str(item) for item in sorted(observed_index["duplicates"])))
    missing = expected_keys - observed_keys
    if missing:
        errors.append("missing expected pane " + ", ".join(str(item) for item in sorted(missing)))
    if not expected_session.get("allowExtraPanes", False):
        extra = observed_keys - expected_keys
        if extra:
            errors.append("extra pane " + ", ".join(str(item) for item in sorted(extra)))


def _descriptor_matches(expected: dict[str, Any], observed: Any, *, topology_only: bool = False) -> bool:
    if not isinstance(observed, dict):
        return False
    if topology_only:
        return _topology_matches(expected.get("topology"), observed.get("topology"))
    for key in ("mode", "workloadKind", "unresolvedReason", "evidenceSource", "confidence"):
        if key in expected and observed.get(key) != expected.get(key):
            return False
    for key in ("owner", "agent", "command"):
        if key in expected and not _is_subset(expected[key], observed.get(key)):
            return False
    return _topology_matches(expected.get("topology"), observed.get("topology"))


def _topology_matches(expected: Any, observed: Any) -> bool:
    if not isinstance(expected, dict) or not isinstance(observed, dict):
        return False
    for key in ("sessionName", "windowName", "windowLayout", "paneCurrentPath"):
        if key in expected and observed.get(key) != expected.get(key):
            return False
    return True


def _pane_status_record_healthy(expected: dict[str, Any], observed: dict[str, Any], *, topology_only: bool) -> bool:
    status = observed.get("paneStatus")
    if not isinstance(status, dict):
        return False
    if status.get("dead") is not False:
        return False
    expected_status = expected.get("paneStatus") if isinstance(expected.get("paneStatus"), dict) else {}
    expected_cwd = expected_status.get("cwd")
    if expected_cwd and status.get("cwd") != expected_cwd:
        return False
    if topology_only:
        return True
    if not status.get("currentCommand"):
        return False
    if expected_status.get("currentCommand") and status.get("currentCommand") != expected_status.get("currentCommand"):
        return False
    return True


def _is_subset(expected: Any, observed: Any) -> bool:
    if isinstance(expected, dict):
        if not isinstance(observed, dict):
            return False
        for key, value in expected.items():
            if key not in observed or not _is_subset(value, observed[key]):
                return False
        return True
    if isinstance(expected, list):
        return expected == observed
    return expected == observed


def _session_key(session: dict[str, Any]) -> tuple[str, str]:
    return (str(session.get("unixUser", "")).strip(), str(session.get("sessionName", "")).strip())


def _pane_key(desc: dict[str, Any]) -> tuple[Any, ...]:
    topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
    return (
        topology.get("windowIndex"),
        topology.get("paneIndex"),
        topology.get("paneId"),
    )


def _logical_pane_index(descriptors: list[Any]) -> dict[str, Any]:
    valid = [desc for desc in descriptors if isinstance(desc, dict) and desc.get("mode") != "managed"]
    window_indexes = sorted({_topology_value(desc, "windowIndex") for desc in valid})
    window_ordinals = {value: index for index, value in enumerate(window_indexes)}
    pane_ordinals: dict[Any, dict[Any, int]] = {}
    for window_index in window_indexes:
        pane_indexes = sorted({_topology_value(desc, "paneIndex") for desc in valid if _topology_value(desc, "windowIndex") == window_index})
        pane_ordinals[window_index] = {value: index for index, value in enumerate(pane_indexes)}
    by_logical: dict[tuple[Any, ...], dict[str, Any]] = {}
    by_provenance: dict[tuple[Any, ...], tuple[Any, ...]] = {}
    duplicates: set[tuple[Any, ...]] = set()
    for desc in valid:
        topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
        window_index = topology.get("windowIndex")
        pane_index = topology.get("paneIndex")
        logical = (window_ordinals.get(window_index), pane_ordinals.get(window_index, {}).get(pane_index))
        if logical in by_logical:
            duplicates.add(logical)
        else:
            by_logical[logical] = desc
        by_provenance[_pane_key(desc)] = logical
    return {"by_logical": by_logical, "by_provenance": by_provenance, "duplicates": duplicates}


def _topology_value(desc: dict[str, Any], key: str) -> Any:
    topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
    return topology.get(key)


def _verification_key(record: dict[str, Any]) -> tuple[Any, ...]:
    target = record.get("target") if isinstance(record.get("target"), dict) else {}
    return (
        target.get("windowIndex"),
        target.get("paneIndex"),
        target.get("paneId"),
    )


def main(
    argv: list[str] | None = None,
    *,
    command_runner: Any | None = None,
    proc_reader: Any | None = None,
    current_user: str | None = None,
    sleep: Callable[[float], Any] = time.sleep,
) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, help="Accepted recovery manifest path")
    parser.add_argument("--observed", help="Test/import-only JSON file containing observed session records")
    parser.add_argument("--allow-test-observed", action="store_true", help="Allow --observed fixture input instead of live recollection")
    parser.add_argument("--socket", help="Owner tmux socket path for live evidence collection")
    parser.add_argument("--unix-user", help="Unix owner whose tmux socket is being collected")
    parser.add_argument("--owner-home", help="Unix owner home directory")
    parser.add_argument("--owner-kind", choices=["session_bank", "persistent_agent", "external_manager"])
    parser.add_argument("--owner-ref")
    parser.add_argument("--owner-may-restart", action="store_true")
    parser.add_argument("--session-name", help="Required filter for persistent/external owner collection")
    parser.add_argument("--stability-seconds", type=float, default=DEFAULT_STABILITY_SECONDS)
    parser.add_argument("--topology-only", action="store_true")
    args = parser.parse_args(argv)
    doc = manifest_lib.load_manifest(args.manifest)
    observed = None
    observed_provider = None
    if args.observed:
        if not args.allow_test_observed:
            parser.error("--observed is test/import-only; pass --allow-test-observed to use it")
        observed_raw = json.loads(Path(args.observed).read_text(encoding="utf-8"))
        observed = observed_raw.get("sessions", observed_raw) if isinstance(observed_raw, dict) else observed_raw
    else:
        missing = [
            name
            for name, value in {
                "--socket": args.socket,
                "--unix-user": args.unix_user,
                "--owner-home": args.owner_home,
                "--owner-kind": args.owner_kind,
            }.items()
            if not value
        ]
        if missing:
            parser.error("normal verification requires " + ", ".join(missing) + " or explicit --observed")
        if args.owner_kind != "session_bank" and (not args.owner_ref or not args.session_name):
            parser.error("persistent/external verification collection requires --owner-ref and --session-name")
        actual_user = current_user if current_user is not None else collector.current_effective_user()
        if actual_user != args.unix_user:
            parser.error(f"collector must run as requested unix user {args.unix_user}; current user is {actual_user}")

        def observed_provider() -> list[dict[str, Any]]:
            evidence = collector.collect_tmux_evidence(
                socket=args.socket,
                owner={"kind": args.owner_kind, "ref": args.owner_ref or "", "mayRestart": bool(args.owner_may_restart)},
                owner_home=args.owner_home,
                unix_user=args.unix_user,
                session_filter=args.session_name,
                command_runner=command_runner,
                proc_reader=proc_reader,
            )
            return snapshot.sessions_from_collected_evidence(evidence, unix_user=args.unix_user)
    result = verify_manifest(
        deepcopy(doc),
        observed_sessions=observed,
        observed_provider=observed_provider,
        stability_seconds=args.stability_seconds,
        sleep=sleep,
        topology_only=args.topology_only,
    )
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0 if result["ok"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
