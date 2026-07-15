#!/usr/bin/env python3
"""Snapshot CHROTE tmux recovery descriptors into an immutable manifest."""

from __future__ import annotations

import argparse
from dataclasses import dataclass
import json
from pathlib import Path
from typing import Any

from client import ChroteClient, normalize_api_base_url
import collector
import manifest as manifest_lib
import owner_probe

LOOPBACK_BINDS = {"127.0.0.1", "localhost", "::1"}


@dataclass
class SnapshotResult:
    path: Path
    manifest: dict[str, Any]
    posted_sessions: list[tuple[str, str]]
    post_results: list[dict[str, Any]]


def sessions_from_collected_evidence(evidence_items: list[dict[str, Any]], *, unix_user: str = "") -> list[dict[str, Any]]:
    grouped: dict[tuple[str, str], dict[str, Any]] = {}
    for evidence in evidence_items:
        desc = owner_probe.classify_pane(evidence)
        verification = _verification_from_descriptor(desc)
        topology = desc.get("topology", {})
        session_name = str(topology.get("sessionName") or evidence.get("sessionName") or "").strip()
        if not session_name:
            continue
        session_unix_user = str(evidence.get("unixUser") or unix_user or "").strip()
        key = (session_unix_user, session_name)
        if key not in grouped:
            grouped[key] = {
                "sessionName": session_name,
                "unixUser": session_unix_user,
                "descriptors": [],
                "verification": [],
            }
        grouped[key]["descriptors"].append(desc)
        if verification is not None:
            grouped[key]["verification"].append(verification)
    return list(grouped.values())


def _verification_from_descriptor(desc: dict[str, Any]) -> dict[str, Any] | None:
    topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
    pane_status = desc.pop("paneStatus", None)
    helper = desc.pop("helperEndpoint", None)
    derived_helper = _derived_helper_probe(desc)
    if derived_helper is not None:
        helper = derived_helper
    if not isinstance(pane_status, dict):
        return None
    pane_id = str(topology.get("paneId", "")).strip()
    if not pane_id:
        return None
    record: dict[str, Any] = {
        "target": {
            "windowIndex": topology.get("windowIndex"),
            "paneIndex": topology.get("paneIndex"),
            "paneId": pane_id,
        },
        "paneStatus": pane_status,
    }
    if helper is not None:
        record["helperEndpoint"] = helper
    return record


def _derived_helper_probe(desc: dict[str, Any]) -> dict[str, Any] | None:
    if desc.get("mode") != "command" or desc.get("workloadKind") != "python-http-server":
        return None
    command = desc.get("command") if isinstance(desc.get("command"), dict) else {}
    server = command.get("pythonHTTPServer") if isinstance(command.get("pythonHTTPServer"), dict) else {}
    bind = str(server.get("bind", "")).strip()
    port = server.get("port")
    if bind not in LOOPBACK_BINDS or not isinstance(port, int) or port < 1 or port > 65535:
        return None
    host = "[::1]" if bind == "::1" else bind
    return {"kind": "http-get", "url": f"http://{host}:{port}/", "expectStatus": 200}


def create_snapshot(
    client: Any,
    collected_sessions: list[dict[str, Any]],
    output_dir: str | Path,
    *,
    accepted_baseline_path: str | Path | None = None,
    now: str | None = None,
    source: dict[str, Any] | None = None,
) -> SnapshotResult:
    baseline = None
    if accepted_baseline_path is not None:
        baseline = manifest_lib.load_manifest(accepted_baseline_path)
    doc = manifest_lib.new_manifest(collected_sessions, now=now, source=source)
    doc = manifest_lib.merge_preserving_extras(doc, baseline)

    try:
        staged = manifest_lib.stage_pending_manifest(doc, output_dir, now=doc["createdAt"])
    except Exception as exc:
        raise RuntimeError(f"failed to stage manifest before API posts: {exc}") from exc

    try:
        previous_bank = _previous_bank_by_key(client)
    except Exception as exc:
        raise RuntimeError(f"snapshot publish failed before API posts: {exc}; pending manifest left at {staged.pending_path}") from exc
    posted: list[tuple[str, str]] = []
    post_results: list[dict[str, Any]] = []
    try:
        for session in doc["sessions"]:
            if not manifest_lib.is_session_bank_session(session):
                post_results.append(
                    {"sessionName": session["sessionName"], "unixUser": session.get("unixUser", ""), "posted": False, "reason": "manifest-only"}
                )
                continue
            name = session["sessionName"]
            unix_user = session.get("unixUser", "")
            response = client.update_session_recovery(name, unix_user, session["descriptors"])
            posted.append((name, unix_user))
            post_results.append({"sessionName": name, "unixUser": unix_user, "posted": True, "response": response})
        path = manifest_lib.accept_pending_manifest(staged)
    except Exception as exc:
        rollback_errors = _rollback_snapshot_posts(client, posted, previous_bank)
        if rollback_errors:
            details = "; ".join(rollback_errors)
            raise RuntimeError(f"snapshot publish failed: {exc}; rollback failures: {details}; pending manifest left at {staged.pending_path}") from exc
        raise RuntimeError(f"snapshot publish failed: {exc}; pending manifest left at {staged.pending_path}") from exc
    return SnapshotResult(path=path, manifest=doc, posted_sessions=posted, post_results=post_results)


def _previous_bank_by_key(client: Any) -> dict[tuple[str, str], dict[str, Any]]:
    response = client.get_sessions()
    banked = response.get("banked", []) if isinstance(response, dict) else []
    result: dict[tuple[str, str], dict[str, Any]] = {}
    if not isinstance(banked, list):
        return result
    for entry in banked:
        if not isinstance(entry, dict):
            continue
        name = str(entry.get("name") or "").strip()
        if not name:
            continue
        unix_user = str(entry.get("unixUser") or "").strip()
        result[(name, unix_user)] = dict(entry)
    return result


def _rollback_snapshot_posts(
    client: Any,
    posted: list[tuple[str, str]],
    previous_bank: dict[tuple[str, str], dict[str, Any]],
) -> list[str]:
    failures: list[str] = []
    for name, unix_user in reversed(posted):
        previous = previous_bank.get((name, unix_user))
        try:
            if previous is None:
                client.forget_session_bank(name, unix_user)
            else:
                client.restore_session_bank_entry(name, unix_user, previous)
        except Exception as rollback_exc:
            failures.append(f"{unix_user or 'default'}/{name}: {rollback_exc}")
    return failures


def main(
    argv: list[str] | None = None,
    *,
    client: Any | None = None,
    command_runner: Any | None = None,
    proc_reader: Any | None = None,
    current_user: str | None = None,
) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-url", required=True, help="CHROTE API base URL, for example http://127.0.0.1:8095")
    parser.add_argument("--input", help="Explicit fixture/import JSON containing manifest session records")
    parser.add_argument("--socket", help="Owner tmux socket path for evidence collection")
    parser.add_argument("--unix-user", help="Unix owner whose tmux socket is being collected")
    parser.add_argument("--owner-home", help="Unix owner home directory")
    parser.add_argument("--owner-kind", choices=["session_bank", "persistent_agent", "external_manager"])
    parser.add_argument("--owner-ref")
    parser.add_argument("--owner-may-restart", action="store_true")
    parser.add_argument("--session-name", help="Required filter for persistent/external owner collection")
    parser.add_argument("--managed-records", help="Optional JSON file of typed managed session records to include manifest-only")
    parser.add_argument("--output-dir", required=True, help="Directory for the immutable timestamped manifest")
    parser.add_argument("--accepted-baseline", help="Explicit prior manifest whose operator extras should be preserved")
    args = parser.parse_args(argv)

    sessions: list[dict[str, Any]]
    if args.input:
        sessions = _load_sessions_file(args.input)
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
            parser.error("normal snapshot collection requires " + ", ".join(missing))
        if args.owner_kind != "session_bank" and (not args.owner_ref or not args.session_name):
            parser.error("persistent/external snapshot collection requires --owner-ref and --session-name")
        actual_user = current_user if current_user is not None else collector.current_effective_user()
        if actual_user != args.unix_user:
            parser.error(f"collector must run as requested unix user {args.unix_user}; current user is {actual_user}")
        evidence = collector.collect_tmux_evidence(
            socket=args.socket,
            owner={"kind": args.owner_kind, "ref": args.owner_ref or "", "mayRestart": bool(args.owner_may_restart)},
            owner_home=args.owner_home,
            unix_user=args.unix_user,
            session_filter=args.session_name,
            command_runner=command_runner,
            proc_reader=proc_reader,
        )
        sessions = sessions_from_collected_evidence(evidence, unix_user=args.unix_user)
    if args.managed_records:
        sessions.extend(_load_sessions_file(args.managed_records))
    api_url = normalize_api_base_url(args.api_url)
    api_client = client or ChroteClient(api_url)
    result = create_snapshot(
        api_client,
        sessions,
        Path(args.output_dir),
        accepted_baseline_path=args.accepted_baseline,
        source={"apiUrl": api_url},
    )
    print(
        json.dumps(
            {
                "ok": True,
                "manifestPath": str(result.path),
                "postedSessions": result.posted_sessions,
                "postResults": result.post_results,
            },
            sort_keys=True,
        )
    )
    return 0


def _load_sessions_file(path: str | Path) -> list[dict[str, Any]]:
    raw = json.loads(Path(path).read_text(encoding="utf-8"))
    sessions = raw.get("sessions", raw) if isinstance(raw, dict) else raw
    if not isinstance(sessions, list):
        raise manifest_lib.ManifestError("session input must be a list or manifest object")
    return sessions


if __name__ == "__main__":
    raise SystemExit(main())
