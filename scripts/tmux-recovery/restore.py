#!/usr/bin/env python3
"""Restore from a CHROTE tmux recovery manifest by delegating to CHROTE APIs."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import tempfile
import time
from typing import Any, Callable

from client import ChroteClient
import collector
import manifest as manifest_lib
import snapshot
import verify as verify_lib


def restore_manifest(
    manifest_doc: dict[str, Any],
    client: Any,
    *,
    status_runner: Any | None = None,
    topology_only: bool = False,
    verifier: Callable[..., dict[str, Any]] | None = None,
    observed_sessions: list[dict[str, Any]] | None = None,
    observed_provider: Callable[[], list[dict[str, Any]]] | None = None,
    http_runner: Any | None = None,
    readiness_seconds: float = verify_lib.DEFAULT_READINESS_SECONDS,
    readiness_interval_seconds: float = verify_lib.DEFAULT_READINESS_INTERVAL_SECONDS,
    stability_seconds: float = 0,
    managed_status_output: str | Path | None = None,
    sleep: Callable[[float], Any] = time.sleep,
) -> dict[str, Any]:
    manifest_lib.validate_manifest(manifest_doc)
    restore_results: list[dict[str, Any]] = []
    for session in manifest_doc["sessions"]:
        restore_results.append(_restore_one(session, client, status_runner=status_runner, topology_only=topology_only))

    verifier = verifier or verify_lib.verify_manifest
    verification = verifier(
        manifest_doc=manifest_doc,
        observed_sessions=observed_sessions or [],
        observed_provider=observed_provider,
        status_runner=status_runner,
        http_runner=http_runner,
        readiness_seconds=readiness_seconds,
        readiness_interval_seconds=readiness_interval_seconds,
        stability_seconds=stability_seconds,
        sleep=sleep,
        topology_only=topology_only,
    )
    verification_by_key = {
        (item.get("unixUser", ""), item.get("sessionName", "")): item
        for item in verification.get("sessions", [])
        if isinstance(item, dict)
    }
    combined: list[dict[str, Any]] = []
    for restore_result in restore_results:
        key = (restore_result.get("unixUser", ""), restore_result.get("sessionName", ""))
        verify_result = verification_by_key.get(key, {"ok": verification.get("ok", True), "errors": []})
        item = dict(verify_result)
        item["sessionName"] = restore_result.get("sessionName", item.get("sessionName", ""))
        item["unixUser"] = restore_result.get("unixUser", item.get("unixUser", ""))
        item["restore"] = restore_result
        errors = list(item.get("errors", []))
        errors.extend(restore_result.get("errors", []))
        item["errors"] = errors
        item["ok"] = bool(restore_result.get("ok")) and bool(verify_result.get("ok", True))
        combined.append(item)
    if managed_status_output is not None:
        _write_managed_status_output(manifest_doc, restore_results, Path(managed_status_output))
    result = {"ok": all(item["ok"] for item in combined), "sessions": combined}
    for key in ("readinessSeconds", "readinessSamples", "readinessElapsedSeconds", "stabilitySeconds", "stabilitySamples"):
        if key in verification:
            result[key] = verification[key]
    return result


def _restore_one(session: dict[str, Any], client: Any, *, status_runner: Any | None, topology_only: bool) -> dict[str, Any]:
    name = session["sessionName"]
    unix_user = session.get("unixUser", "")
    if manifest_lib.is_session_bank_session(session):
        body: dict[str, Any] = {}
        if topology_only:
            body["topologyOnly"] = True
        try:
            response = client.recover_session(name, unix_user, body)
            return {
                "sessionName": name,
                "unixUser": unix_user,
                "ok": bool(response.get("success")),
                "action": response.get("action", "recovered"),
                "response": response,
                "errors": [] if response.get("success") else ["backend recovery did not report success"],
            }
        except Exception as exc:
            return {
                "sessionName": name,
                "unixUser": unix_user,
                "ok": False,
                "action": "error",
                "errors": [str(exc)],
            }
    if manifest_lib.is_managed_session(session):
        runner = status_runner or verify_lib.SystemdUserStatusRunner()
        try:
            status = runner.check(session["statusProbe"], unix_user=unix_user)
        except Exception as exc:
            status = {"ok": False, "error": str(exc)}
        return {
            "sessionName": name,
            "unixUser": unix_user,
            "ok": bool(status.get("ok")),
            "action": "managed-health-check",
            "managedStatus": status,
            "errors": [] if status.get("ok") else [status.get("error", "managed status probe failed")],
        }
    return {
        "sessionName": name,
        "unixUser": unix_user,
        "ok": True,
        "action": "observe-only",
        "errors": [],
    }


MANAGED_STATUS_ENTRY_KEYS = {"name", "sessionName", "unixUser", "owner", "managerKind", "managerRef", "status", "storageKind", "sourceKind"}
MANAGED_STATUS_STATUS_KEYS = {"ok", "activeState", "checkedAt"}
MANAGED_STATUS_SOURCE_KINDS = {"restore", "snapshot"}
MANAGED_STATUS_SESSION_RE = re.compile(r"^[A-Za-z0-9_-]{1,50}$")
MANAGED_STATUS_UNIX_USER_RE = re.compile(r"^[a-z_][a-z0-9_-]{0,31}$")


def _write_managed_status_output(manifest_doc: dict[str, Any], restore_results: list[dict[str, Any]], path: Path) -> None:
    result_by_key = {
        (item.get("unixUser", ""), item.get("sessionName", "")): item
        for item in restore_results
        if isinstance(item, dict)
    }
    checked_at = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    entries: list[dict[str, Any]] = []
    for session in manifest_doc["sessions"]:
        if not manifest_lib.is_managed_session(session):
            continue
        name = str(session["sessionName"]).strip()
        unix_user = str(session.get("unixUser", "")).strip()
        status = result_by_key.get((unix_user, name), {}).get("managedStatus")
        if not isinstance(status, dict):
            status = {"ok": False, "activeState": "unknown"}
        entry = {
            "name": name,
            "sessionName": name,
            "unixUser": unix_user,
            "owner": session["owner"],
            "managerKind": session["managerKind"],
            "managerRef": session["managerRef"],
            "status": {
                "ok": bool(status.get("ok")),
                "activeState": str(status.get("activeState") or "unknown").strip() or "unknown",
                "checkedAt": str(status.get("checkedAt") or checked_at),
            },
            "storageKind": "managed-status",
            "sourceKind": "restore",
        }
        _validate_managed_status_output_entry(entry)
        entries.append(entry)
    _atomic_write_json_0600(path, entries)


def _validate_managed_status_output_entry(entry: dict[str, Any]) -> None:
    if set(entry) != MANAGED_STATUS_ENTRY_KEYS:
        raise ValueError("managed status output entry has unexpected keys")
    status = entry.get("status")
    if not isinstance(status, dict) or set(status) != MANAGED_STATUS_STATUS_KEYS:
        raise ValueError("managed status output status has unexpected keys")
    if not MANAGED_STATUS_SESSION_RE.fullmatch(str(entry.get("name") or "")):
        raise ValueError("managed status output session name is invalid")
    if entry.get("name") != entry.get("sessionName"):
        raise ValueError("managed status output name/sessionName mismatch")
    if not MANAGED_STATUS_UNIX_USER_RE.match(str(entry.get("unixUser") or "")):
        raise ValueError("managed status output unixUser is invalid")
    owner = entry.get("owner")
    if not isinstance(owner, dict) or owner.get("kind") != "external_manager" or owner.get("mayRestart") is not False:
        raise ValueError("managed status output owner must be read-only external_manager")
    if entry.get("managerKind") != "systemd-user" or not manifest_lib.UNIT_RE.match(str(entry.get("managerRef") or "")):
        raise ValueError("managed status output manager is invalid")
    if owner.get("ref") != f"systemd:user/{entry.get('managerRef')}":
        raise ValueError("managed status output owner ref does not match managerRef")
    if status.get("activeState") not in (manifest_lib.SAFE_STATES | {"unknown"}):
        raise ValueError("managed status output activeState is invalid")
    if not isinstance(status.get("ok"), bool):
        raise ValueError("managed status output ok must be boolean")
    if status.get("ok") is not (status.get("activeState") == "active"):
        raise ValueError("managed status output ok contradicts activeState")
    if not manifest_lib.TIMESTAMP_RE.match(str(status.get("checkedAt") or "")):
        raise ValueError("managed status output checkedAt is invalid")
    if entry.get("storageKind") != "managed-status":
        raise ValueError("managed status output storageKind must be managed-status")
    if entry.get("sourceKind") not in MANAGED_STATUS_SOURCE_KINDS:
        raise ValueError("managed status output sourceKind is invalid")


def _atomic_write_json_0600(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
    tmp = Path(tmp_name)
    try:
        os.chmod(tmp, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(value, handle, indent=2, sort_keys=True)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(tmp, path)
        tmp = None
        os.chmod(path, 0o600)
        _fsync_dir(path.parent)
    except Exception:
        if tmp is not None:
            tmp.unlink(missing_ok=True)
        raise


def _fsync_dir(path: Path) -> None:
    fd = os.open(path, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def main(
    argv: list[str] | None = None,
    *,
    client: Any | None = None,
    command_runner: Any | None = None,
    proc_reader: Any | None = None,
    current_user: str | None = None,
    sleep: Callable[[float], Any] = time.sleep,
) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-url", required=True, help="CHROTE API base URL, for example http://127.0.0.1:8095")
    parser.add_argument("--manifest", required=True, help="Accepted recovery manifest path")
    parser.add_argument("--observed", help="Test/import-only observed session JSON for post-restore verification")
    parser.add_argument("--allow-test-observed", action="store_true", help="Allow --observed fixture input instead of live recollection")
    parser.add_argument("--socket", help="Owner tmux socket path for post-restore evidence collection")
    parser.add_argument("--unix-user", help="Unix owner whose tmux socket is being collected")
    parser.add_argument("--owner-home", help="Unix owner home directory")
    parser.add_argument("--owner-kind", choices=["session_bank", "persistent_agent", "external_manager"])
    parser.add_argument("--owner-ref")
    parser.add_argument("--owner-may-restart", action="store_true")
    parser.add_argument("--session-name", help="Required filter for persistent/external owner collection")
    parser.add_argument("--topology-only", action="store_true", help="Ask CHROTE to recreate topology without launching typed workloads")
    parser.add_argument("--managed-status-output", help="Optional managed-status registry JSON path to atomically replace")
    parser.add_argument("--stability-seconds", type=float, default=30)
    parser.add_argument("--readiness-seconds", type=float, default=verify_lib.DEFAULT_READINESS_SECONDS)
    parser.add_argument("--readiness-interval-seconds", type=float, default=verify_lib.DEFAULT_READINESS_INTERVAL_SECONDS)
    args = parser.parse_args(argv)

    doc = manifest_lib.load_manifest(args.manifest)
    observed = []
    if args.observed:
        if not args.allow_test_observed:
            parser.error("--observed is test/import-only; pass --allow-test-observed to use it")
        raw = json.loads(Path(args.observed).read_text(encoding="utf-8"))
        observed = raw.get("sessions", raw) if isinstance(raw, dict) else raw
        observed_provider = None
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
            parser.error("normal restore verification requires " + ", ".join(missing) + " or explicit --observed")
        if args.owner_kind != "session_bank" and (not args.owner_ref or not args.session_name):
            parser.error("persistent/external restore collection requires --owner-ref and --session-name")
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
    result = restore_manifest(
        doc,
        client or ChroteClient(args.api_url),
        topology_only=args.topology_only,
        observed_sessions=observed,
        observed_provider=observed_provider,
        readiness_seconds=args.readiness_seconds,
        readiness_interval_seconds=args.readiness_interval_seconds,
        stability_seconds=args.stability_seconds,
        managed_status_output=args.managed_status_output,
        sleep=sleep,
    )
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0 if result["ok"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
