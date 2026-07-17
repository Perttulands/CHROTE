#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from io import BytesIO, StringIO
from contextlib import redirect_stdout
from urllib import error as urllib_error

sys.path.insert(0, str(Path(__file__).resolve().parent))

import client as client_lib
import collector
import manifest
import restore
import snapshot
import verify


SCRIPT_DIR = Path(__file__).resolve().parent
FIXTURES = SCRIPT_DIR / "fixtures" / "ops"


CODEX_ID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
CLAUDE_ID = "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4"
HERMES_ID = "hermes-session-20260715T100000Z"


def session_bank_descriptor(session: str, pane_id: str = "%1") -> dict:
    return {
        "mode": "agent",
        "owner": {"kind": "session_bank", "ref": f"alice/{session}", "mayRestart": True},
        "topology": {
            "sessionName": session,
            "windowIndex": 0,
            "windowName": "agent",
            "windowLayout": "b25f,80x24,0,0",
            "paneIndex": 0,
            "paneId": pane_id,
            "paneCurrentPath": "/home/alice/project",
        },
        "workloadKind": "codex",
        "agent": {"kind": "codex", "nativeSessionId": CODEX_ID},
        "evidenceSource": "argv",
        "confidence": "high",
    }


def hermes_descriptor(session: str, profile: str = "scout") -> dict:
    return {
        "mode": "agent",
        "owner": {"kind": "persistent_agent", "ref": f"persistent:alice/{session}", "mayRestart": True},
        "topology": {
            "sessionName": session,
            "windowIndex": 0,
            "windowName": "agent",
            "windowLayout": "b25f,80x24,0,0",
            "paneIndex": 1,
            "paneId": "%2",
            "paneCurrentPath": "/home/alice/project",
        },
        "workloadKind": "hermes",
        "agent": {"kind": "hermes", "nativeSessionId": HERMES_ID, "hermesProfile": profile},
        "evidenceSource": "state_db",
        "confidence": "medium",
    }


def python_descriptor(session: str) -> dict:
    return {
        "mode": "command",
        "owner": {"kind": "session_bank", "ref": f"alice/{session}", "mayRestart": True},
        "topology": {
            "sessionName": session,
            "windowIndex": 1,
            "windowName": "server",
            "windowLayout": "7f91,80x24,0,0",
            "paneIndex": 0,
            "paneId": "%3",
            "paneCurrentPath": "/home/alice/project/public",
        },
        "workloadKind": "python-http-server",
        "command": {
            "kind": "python-http-server",
            "pythonHTTPServer": {"bind": "127.0.0.1", "port": 8088, "directory": "/home/alice/project/public"},
        },
        "evidenceSource": "argv",
        "confidence": "high",
    }


def verification_for_descriptor(desc: dict, *, helper: dict | None = None, current_command: str | None = None) -> dict:
    topology = desc["topology"]
    command = current_command
    if command is None:
        if desc.get("mode") == "command":
            command = "python3"
        elif desc.get("workloadKind") == "hermes":
            command = "python"
        elif desc.get("workloadKind") == "shell":
            command = "bash"
        else:
            command = str(desc.get("workloadKind") or "sh")
    record = {
        "target": {
            "windowIndex": topology["windowIndex"],
            "paneIndex": topology["paneIndex"],
            "paneId": topology.get("paneId", ""),
        },
        "paneStatus": {
            "dead": False,
            "currentCommand": command,
            "cwd": topology.get("paneCurrentPath", ""),
        },
    }
    if helper is not None:
        record["helperEndpoint"] = helper
    return record


def session_record(session_name: str, descriptors: list[dict], *, unix_user: str = "alice", verification: list[dict] | None = None) -> dict:
    record = {"sessionName": session_name, "unixUser": unix_user, "descriptors": descriptors}
    if verification is not None:
        record["verification"] = verification
    return record


def live_session_record(session_name: str, descriptors: list[dict], *, unix_user: str = "alice") -> dict:
    return session_record(
        session_name,
        descriptors,
        unix_user=unix_user,
        verification=[verification_for_descriptor(desc) for desc in descriptors if desc.get("mode") != "managed"],
    )


def retarget_descriptor(
    desc: dict,
    *,
    session_id: str,
    window_index: int,
    window_name: str,
    window_layout: str,
    pane_index: int,
    pane_id: str,
    cwd: str,
) -> dict:
    result = json.loads(json.dumps(desc))
    result["topology"].update(
        {
            "sessionId": session_id,
            "windowIndex": window_index,
            "windowName": window_name,
            "windowLayout": window_layout,
            "paneIndex": pane_index,
            "paneId": pane_id,
            "paneCurrentPath": cwd,
        }
    )
    return result


def reallocated_verify_sessions() -> tuple[dict, dict, dict]:
    helper = {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}
    expected_agent_layout = "aaaa,160x40,0,0[160x20,0,0,1,160x19,0,21,2]"
    observed_agent_layout = "bbbb,160x40,0,0[160x20,0,0,10,160x19,0,21,11]"
    expected_server_layout = "cccc,80x24,0,0,3"
    observed_server_layout = "dddd,80x24,0,0,12"
    expected_codex = retarget_descriptor(
        session_bank_descriptor("velis"),
        session_id="$old",
        window_index=4,
        window_name="agent",
        window_layout=expected_agent_layout,
        pane_index=7,
        pane_id="%old-codex",
        cwd="/home/alice/project",
    )
    expected_hermes = retarget_descriptor(
        hermes_descriptor("velis"),
        session_id="$old",
        window_index=4,
        window_name="agent",
        window_layout=expected_agent_layout,
        pane_index=8,
        pane_id="%old-hermes",
        cwd="/home/alice/project",
    )
    expected_python = retarget_descriptor(
        python_descriptor("velis"),
        session_id="$old",
        window_index=5,
        window_name="server",
        window_layout=expected_server_layout,
        pane_index=7,
        pane_id="%old-http",
        cwd="/home/alice/project/public",
    )
    observed_codex = retarget_descriptor(
        session_bank_descriptor("velis", pane_id="%new-codex"),
        session_id="$new",
        window_index=0,
        window_name="agent",
        window_layout=observed_agent_layout,
        pane_index=0,
        pane_id="%new-codex",
        cwd="/home/alice/project",
    )
    observed_hermes = retarget_descriptor(
        hermes_descriptor("velis"),
        session_id="$new",
        window_index=0,
        window_name="agent",
        window_layout=observed_agent_layout,
        pane_index=1,
        pane_id="%new-hermes",
        cwd="/home/alice/project",
    )
    observed_python = retarget_descriptor(
        python_descriptor("velis"),
        session_id="$new",
        window_index=1,
        window_name="server",
        window_layout=observed_server_layout,
        pane_index=0,
        pane_id="%new-http",
        cwd="/home/alice/project/public",
    )
    expected = session_record(
        "velis",
        [expected_codex, expected_hermes, expected_python],
        verification=[
            verification_for_descriptor(expected_codex),
            verification_for_descriptor(expected_hermes),
            verification_for_descriptor(expected_python, helper=helper),
        ],
    )
    observed = session_record(
        "velis",
        [observed_codex, observed_hermes, observed_python],
        verification=[
            verification_for_descriptor(observed_codex),
            verification_for_descriptor(observed_hermes),
            verification_for_descriptor(observed_python, helper=helper),
        ],
    )
    return expected, observed, helper


def managed_session(name: str, unit: str) -> dict:
    return {
        "sessionName": name,
        "unixUser": "alice",
        "owner": {"kind": "external_manager", "ref": f"systemd:user/{unit}", "mayRestart": False},
        "managerKind": "systemd-user",
        "managerRef": unit,
        "restartAllowed": False,
        "statusProbe": {"kind": "systemd-user", "unit": unit, "expectActiveState": "active"},
        "descriptors": [
            {
                "mode": "managed",
                "owner": {"kind": "external_manager", "ref": f"systemd:user/{unit}", "mayRestart": False},
                "topology": {
                    "sessionName": name,
                    "windowIndex": 0,
                    "windowName": "service",
                    "windowLayout": "b25f,80x24,0,0",
                    "paneIndex": 0,
                    "paneId": "%10",
                    "paneCurrentPath": "/home/alice/service",
                },
                "workloadKind": "managed",
                "evidenceSource": "manager",
                "confidence": "high",
            }
        ],
    }


def topology_descriptor(session: str) -> dict:
    desc = session_bank_descriptor(session)
    desc.update({"mode": "topology", "workloadKind": "shell", "evidenceSource": "topology", "confidence": "medium"})
    desc.pop("agent", None)
    return desc


def unresolved_descriptor(session: str, reason: str = "unknown_process") -> dict:
    desc = session_bank_descriptor(session)
    desc.update(
        {
            "mode": "unresolved",
            "owner": {"kind": "session_bank", "ref": f"alice/{session}", "mayRestart": False},
            "workloadKind": "unknown",
            "unresolvedReason": reason,
            "evidenceSource": "process",
            "confidence": "low",
        }
    )
    desc.pop("agent", None)
    desc.pop("command", None)
    desc.pop("helperEndpoint", None)
    return desc


class RecordingClient:
    def __init__(self) -> None:
        self.updates: list[tuple[str, str, dict]] = []
        self.entry_updates: list[tuple[str, str, dict]] = []
        self.deletes: list[tuple[str, str]] = []
        self.recovers: list[tuple[str, str, dict]] = []
        self.restored_entries: list[tuple[str, str, dict]] = []
        self.recover_responses: dict[str, dict] = {}
        self.banked: list[dict] = []
        self.session_reads = 0

    def get_sessions(self) -> dict:
        self.session_reads += 1
        return {"banked": list(self.banked)}

    def update_session_recovery_entry(self, name: str, unix_user: str, body: dict) -> dict:
        self.entry_updates.append((name, unix_user, body))
        if "recoveryPlan" in body:
            self.updates.append((name, unix_user, body))
        return {"success": True, "session": name}

    def update_session_recovery(self, name: str, unix_user: str, recovery_plan: list[dict]) -> dict:
        body = {"recoveryPlan": recovery_plan}
        return self.update_session_recovery_entry(name, unix_user, body)

    def forget_session_bank(self, name: str, unix_user: str) -> dict:
        self.deletes.append((name, unix_user))
        return {"success": True, "removed": True, "session": name}

    def restore_session_bank_entry(self, name: str, unix_user: str, entry: dict) -> dict:
        self.restored_entries.append((name, unix_user, json.loads(json.dumps(entry))))
        return {"success": True, "session": name}

    def recover_session(self, name: str, unix_user: str, body: dict) -> dict:
        self.recovers.append((name, unix_user, body))
        return self.recover_responses.get(name, {"success": True, "action": "recovered", "session": name})


class FailingUpdateClient(RecordingClient):
    def update_session_recovery(self, name: str, unix_user: str, recovery_plan: list[dict]) -> dict:
        super().update_session_recovery(name, unix_user, recovery_plan)
        raise RuntimeError("api unavailable")


class RaisingRecoverClient(RecordingClient):
    def recover_session(self, name: str, unix_user: str, body: dict) -> dict:
        self.recovers.append((name, unix_user, body))
        if name == "velis":
            raise RuntimeError("recover failed")
        return {"success": True, "action": "recovered", "session": name}


class RecordingStatusRunner:
    def __init__(self) -> None:
        self.calls: list[dict] = []

    def check(self, probe: dict, unix_user: str = "") -> dict:
        self.calls.append({"probe": probe, "unixUser": unix_user})
        return {"ok": True, "activeState": probe.get("expectActiveState", "active")}


class RecordingHTTPRunner:
    def __init__(self) -> None:
        self.calls: list[dict] = []

    def check(self, probe: dict) -> dict:
        self.calls.append(probe)
        return {"ok": True, "status": probe.get("expectStatus", 200)}


class SequencedHTTPRunner:
    def __init__(self, statuses: list[bool]) -> None:
        self.statuses = list(statuses)
        self.calls: list[dict] = []

    def check(self, probe: dict) -> dict:
        self.calls.append(probe)
        ok = self.statuses.pop(0) if self.statuses else False
        return {"ok": ok, "status": probe.get("expectStatus", 200) if ok else 503}


class ClientTest(unittest.TestCase):
    def test_api_base_url_is_canonicalized_and_rejects_credentials_paths_and_insecure_non_loopback(self) -> None:
        cases = {
            "http://localhost:8095/": "http://localhost:8095",
            "http://127.0.0.1:8095": "http://127.0.0.1:8095",
            "http://[::1]:8095": "http://[::1]:8095",
            "https://example.com/": "https://example.com",
        }
        for raw, want in cases.items():
            with self.subTest(raw=raw):
                self.assertEqual(client_lib.normalize_api_base_url(raw), want)

        bad_urls = [
            "http://user:pass@127.0.0.1:8095",
            "http://127.0.0.1:8095/api",
            "http://127.0.0.1:8095/?token=secret",
            "http://127.0.0.1:8095/#fragment",
            "http://example.com:8095",
            "https://example.com/api",
            "ftp://127.0.0.1:8095",
        ]
        for raw in bad_urls:
            with self.subTest(raw=raw):
                with self.assertRaises(client_lib.ChroteClientError):
                    client_lib.normalize_api_base_url(raw)

    def test_chrote_client_does_not_follow_redirects_or_send_auth_to_location(self) -> None:
        location = "https://redirect-target.example/steal"

        class RedirectingOpener:
            def __init__(self) -> None:
                self.calls: list[dict] = []

            def open(self, req: object, timeout: float) -> object:
                headers = dict(getattr(req, "header_items")())
                self.calls.append(
                    {
                        "url": getattr(req, "full_url", str(req)),
                        "method": getattr(req, "get_method")(),
                        "headers": headers,
                    }
                )
                raise urllib_error.HTTPError(getattr(req, "full_url", ""), 302, "Found", {"Location": location}, BytesIO(b"redirect"))

        cases = [
            ("GET token", "GET", "secret-token"),
            ("GET no token", "GET", None),
            ("POST token", "POST", "secret-token"),
            ("POST no token", "POST", None),
        ]
        for name, method, token in cases:
            with self.subTest(name=name):
                opener = RedirectingOpener()
                api = client_lib.ChroteClient("https://chrote.example", token=token, opener=opener)
                with self.assertRaisesRegex(client_lib.ChroteClientError, "HTTP 302"):
                    if method == "GET":
                        api.get_sessions()
                    else:
                        api.update_session_recovery("velis", "alice", [session_bank_descriptor("velis")])
                self.assertEqual(len(opener.calls), 1)
                self.assertEqual(opener.calls[0]["method"], method)
                self.assertNotEqual(opener.calls[0]["url"], location)
                if token:
                    self.assertEqual(opener.calls[0]["headers"].get("Authorization"), "Bearer secret-token")
                else:
                    self.assertNotIn("Authorization", opener.calls[0]["headers"])

    def test_snapshot_source_persists_only_sanitized_api_origin(self) -> None:
        session_path = write_json_temp([live_session_record("velis", [session_bank_descriptor("velis")])])
        try:
            with tempfile.TemporaryDirectory() as tmp:
                code = snapshot.main(
                    [
                        "--api-url",
                        "http://localhost:8095/",
                        "--input",
                        str(session_path),
                        "--output-dir",
                        tmp,
                    ],
                    client=RecordingClient(),
                )
                self.assertEqual(code, 0)
                manifest_path = next(Path(tmp).glob("*.json"))
                doc = json.loads(manifest_path.read_text(encoding="utf-8"))
                self.assertEqual(doc["source"]["apiUrl"], "http://localhost:8095")
        finally:
            session_path.unlink(missing_ok=True)


class ManifestTest(unittest.TestCase):
    def test_managed_records_default_restart_false_and_typed_status_probe(self) -> None:
        doc = manifest.new_manifest(
            [
                {
                    "sessionName": "bridge",
                    "unixUser": "alice",
                    "managerKind": "systemd-user",
                    "managerRef": "bridge.service",
                    "descriptors": [
                        {
                            "mode": "managed",
                            "owner": {"kind": "external_manager", "ref": "systemd:user/bridge.service", "mayRestart": False},
                            "topology": {"sessionName": "bridge", "windowIndex": 0, "paneIndex": 0},
                            "workloadKind": "managed",
                            "evidenceSource": "manager",
                            "confidence": "high",
                        }
                    ],
                }
            ],
            now="2026-07-15T10:00:00Z",
        )
        session = doc["sessions"][0]
        self.assertIs(session["restartAllowed"], False)
        self.assertEqual(session["statusProbe"], {"kind": "systemd-user", "unit": "bridge.service", "expectActiveState": "active"})

    def test_writes_timestamped_mode_0600_manifest_and_refuses_overwrite(self) -> None:
        doc = manifest.new_manifest([{"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis")]}], now="2026-07-15T10:00:00Z")
        with tempfile.TemporaryDirectory() as tmp:
            path = manifest.write_timestamped_manifest(doc, Path(tmp), now="2026-07-15T10:00:00Z")
            self.assertEqual(path.name, "chrote-tmux-recovery-20260715T100000Z.json")
            mode = stat.S_IMODE(path.stat().st_mode)
            self.assertEqual(mode, 0o600)
            with self.assertRaises(FileExistsError):
                manifest.write_timestamped_manifest(doc, Path(tmp), now="2026-07-15T10:00:00Z")

    def test_explicit_accepted_baseline_preserves_valid_baseline_only_sessions_and_current_replacements(self) -> None:
        baseline = manifest.new_manifest(
            [
                live_session_record("velis", [session_bank_descriptor("velis")]),
                managed_session("bridge", "bridge.service"),
                live_session_record("offline-bank", [session_bank_descriptor("offline-bank")]),
            ],
            now="2026-07-15T09:00:00Z",
        )
        current = manifest.new_manifest(
            [
                live_session_record("velis", [session_bank_descriptor("velis", "%99")]),
                live_session_record("post-snapshot-extra", [session_bank_descriptor("post-snapshot-extra")]),
            ],
            now="2026-07-15T10:00:00Z",
        )
        merged = manifest.merge_preserving_extras(current, baseline)
        self.assertEqual([item["sessionName"] for item in merged["sessions"]], ["velis", "post-snapshot-extra", "bridge", "offline-bank"])
        self.assertEqual(merged["sessions"][0]["descriptors"][0]["topology"]["paneId"], "%99")
        self.assertEqual(merged["sessions"][2]["owner"]["kind"], "external_manager")
        self.assertEqual(merged["sessions"][3]["descriptors"][0]["owner"]["kind"], "session_bank")

        stale_metadata = json.loads(json.dumps(baseline))
        stale_metadata["postSnapshotReview"] = {"operatorNote": "must not survive"}
        with self.assertRaisesRegex(manifest.ManifestError, "postSnapshotReview"):
            manifest.merge_preserving_extras(current, stale_metadata)

        result = verify.verify_manifest(
            manifest.new_manifest(
                [live_session_record("velis", [session_bank_descriptor("velis", "%99")])],
                now="2026-07-15T10:00:00Z",
            ),
            observed_sessions=merged["sessions"],
            status_runner=RecordingStatusRunner(),
            http_runner=RecordingHTTPRunner(),
            stability_seconds=0,
        )
        self.assertTrue(result["ok"], result)

    def test_manifest_rejects_unknown_keys_at_every_object_depth(self) -> None:
        def base_doc() -> dict:
            helper = {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}
            codex = session_bank_descriptor("velis")
            py = python_descriptor("velis")
            return manifest.new_manifest(
                [
                    session_record(
                        "velis",
                        [codex, py],
                        verification=[
                            verification_for_descriptor(codex),
                            verification_for_descriptor(py, helper=helper),
                        ],
                    ),
                    managed_session("bridge", "bridge.service"),
                ],
                now="2026-07-15T10:00:00Z",
                source={"apiUrl": "http://127.0.0.1:8095"},
            )

        cases = [
            ("top level", lambda doc: doc.update({"postSnapshotReview": {"acceptedBy": "operator"}}), "postSnapshotReview"),
            ("source", lambda doc: doc["source"].update({"token": "secret"}), "token"),
            ("session", lambda doc: doc["sessions"][0].update({"operatorNote": "wrong depth"}), "operatorNote"),
            ("owner", lambda doc: doc["sessions"][0]["descriptors"][0]["owner"].update({"token": "secret"}), "token"),
            ("topology", lambda doc: doc["sessions"][0]["descriptors"][0]["topology"].update({"customExtra": True}), "customExtra"),
            ("agent", lambda doc: doc["sessions"][0]["descriptors"][0]["agent"].update({"rawArgv": ["codex"]}), "rawArgv"),
            ("command", lambda doc: doc["sessions"][0]["descriptors"][1]["command"].update({"environment": {"KEY": "secret"}}), "environment"),
            ("pythonHTTPServer", lambda doc: doc["sessions"][0]["descriptors"][1]["command"]["pythonHTTPServer"].update({"token": "secret"}), "token"),
            ("verification", lambda doc: doc["sessions"][0]["verification"][0].update({"rawArgv": ["codex"]}), "rawArgv"),
            ("verification target", lambda doc: doc["sessions"][0]["verification"][0]["target"].update({"customExtra": True}), "customExtra"),
            ("paneStatus", lambda doc: doc["sessions"][0]["verification"][0]["paneStatus"].update({"customExtra": True}), "customExtra"),
            ("helperEndpoint", lambda doc: doc["sessions"][0]["verification"][1]["helperEndpoint"].update({"token": "secret"}), "token"),
            ("statusProbe", lambda doc: doc["sessions"][1]["statusProbe"].update({"environment": {"KEY": "secret"}}), "environment"),
        ]
        for name, mutate, want in cases:
            with self.subTest(name=name):
                doc = base_doc()
                mutate(doc)
                with self.assertRaisesRegex(manifest.ManifestError, want):
                    manifest.validate_manifest(doc)

    def test_schema_key_sets_match_strict_manifest_objects(self) -> None:
        schema = json.loads((SCRIPT_DIR / "recovery-manifest.schema.json").read_text(encoding="utf-8"))
        expectations = [
            ("$", schema, {"schemaVersion", "createdAt", "source", "sessions"}),
            ("source", schema["$defs"].get("source", {}), {"apiUrl"}),
            ("session", schema["$defs"]["session"], {"sessionName", "unixUser", "owner", "managerKind", "managerRef", "restartAllowed", "statusProbe", "allowExtraPanes", "descriptors", "verification"}),
            ("owner", schema["$defs"]["owner"], {"kind", "ref", "mayRestart"}),
            ("topology", schema["$defs"]["topology"], {"sessionName", "sessionId", "windowIndex", "windowName", "windowLayout", "paneIndex", "paneId", "paneCurrentPath"}),
            ("descriptor", schema["$defs"]["descriptor"], {"mode", "owner", "topology", "workloadKind", "agent", "command", "evidenceSource", "confidence", "unresolvedReason"}),
            ("agent", schema["$defs"]["agent"], {"kind", "nativeSessionId", "hermesProfile"}),
            ("command", schema["$defs"]["command"], {"kind", "pythonHTTPServer"}),
            (
                "pythonHTTPServer",
                schema["$defs"]["command"]["properties"]["pythonHTTPServer"],
                {"bind", "port", "directory"},
            ),
            ("paneVerification", schema["$defs"]["paneVerification"], {"target", "paneStatus", "helperEndpoint"}),
            ("paneTarget", schema["$defs"]["paneTarget"], {"windowIndex", "paneIndex", "paneId"}),
            ("paneStatus", schema["$defs"]["paneStatus"], {"dead", "currentCommand", "cwd"}),
            ("helperEndpoint", schema["$defs"]["helperEndpoint"], {"kind", "url", "expectStatus", "timeoutSeconds"}),
            ("statusProbe", schema["$defs"]["statusProbe"], {"kind", "unit", "expectActiveState"}),
        ]
        for name, node, keys in expectations:
            with self.subTest(name=name):
                self.assertIn("properties", node)
                self.assertEqual(set(node["properties"]), keys)
                self.assertIs(node["additionalProperties"], False)

    def test_strict_manifest_rejects_duplicates_conflicts_unsafe_helpers_and_bounds(self) -> None:
        cases = [
            {
                "name": "duplicate session key",
                "sessions": [
                    {"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis")]},
                    {"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis", "%2")]},
                ],
                "want": "duplicate session",
            },
            {
                "name": "duplicate pane key",
                "sessions": [
                    {"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis"), session_bank_descriptor("velis", "%2")]}
                ],
                "want": "duplicate pane",
            },
            {
                "name": "duplicate nonempty pane id",
                "sessions": [
                    {
                        "sessionName": "velis",
                        "unixUser": "alice",
                        "descriptors": [
                            session_bank_descriptor("velis"),
                            {
                                **python_descriptor("velis"),
                                "topology": {**python_descriptor("velis")["topology"], "paneId": "%1"},
                            },
                        ],
                    }
                ],
                "want": "duplicate pane id",
            },
            {
                "name": "unsafe topology",
                "sessions": [
                    {
                        "sessionName": "velis",
                        "unixUser": "alice",
                        "descriptors": [
                            {
                                **session_bank_descriptor("velis"),
                                "topology": {
                                    **session_bank_descriptor("velis")["topology"],
                                    "paneId": "%1;bad",
                                    "paneCurrentPath": "relative/path",
                                },
                            }
                        ],
                    }
                ],
                "want": "topology",
            },
            {
                "name": "malformed codex id",
                "sessions": [
                    {
                        "sessionName": "velis",
                        "unixUser": "alice",
                        "descriptors": [
                            {**session_bank_descriptor("velis"), "agent": {"kind": "codex", "nativeSessionId": "not-a-uuid"}}
                        ],
                    }
                ],
                "want": "uuid",
            },
            {
                "name": "non loopback helper",
                "sessions": [
                    session_record(
                        "velis",
                        [python_descriptor("velis")],
                        verification=[
                            verification_for_descriptor(
                                python_descriptor("velis"),
                                helper={"kind": "http-get", "url": "http://10.0.0.2/health", "expectStatus": 200},
                            )
                        ],
                    )
                ],
                "want": "loopback",
            },
            {
                "name": "invalid mode owner",
                "sessions": [
                    {
                        "sessionName": "velis",
                        "unixUser": "alice",
                        "descriptors": [
                            {
                                **session_bank_descriptor("velis"),
                                "mode": "managed",
                                "owner": {"kind": "session_bank", "ref": "alice/velis", "mayRestart": True},
                            }
                        ],
                    }
                ],
                "want": "managed",
            },
            {
                "name": "oversized allowed descriptor string",
                "sessions": [
                    {
                        "sessionName": "velis",
                        "unixUser": "alice",
                        "descriptors": [
                            {
                                **session_bank_descriptor("velis"),
                                "topology": {**session_bank_descriptor("velis")["topology"], "windowLayout": "x" * 9000},
                            }
                        ],
                    }
                ],
                "want": "too long",
            },
        ]
        for case in cases:
            with self.subTest(case=case["name"]):
                with self.assertRaisesRegex(manifest.ManifestError, case["want"]):
                    manifest.new_manifest(case["sessions"], now="2026-07-15T10:00:00Z")

    def test_descriptor_mode_payload_parity_matches_go_canonicalization(self) -> None:
        helper = {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}
        cases = [
            {
                "name": "review repro agent workloadKind must equal agent kind",
                "desc": {**session_bank_descriptor("velis"), "workloadKind": "claude"},
                "want": "workloadKind",
            },
            {
                "name": "review repro command workloadKind must be python-http-server",
                "desc": {**python_descriptor("velis"), "workloadKind": "shell"},
                "want": "workloadKind",
            },
            {
                "name": "review repro unresolved cannot carry stale agent payload",
                "desc": {**unresolved_descriptor("velis"), "agent": session_bank_descriptor("velis")["agent"]},
                "want": "agent",
            },
            {
                "name": "agent forbids command payload",
                "desc": {**session_bank_descriptor("velis"), "command": python_descriptor("velis")["command"]},
                "want": "command",
            },
            {
                "name": "agent forbids unresolved reason",
                "desc": {**session_bank_descriptor("velis"), "unresolvedReason": "unknown_process"},
                "want": "unresolvedReason",
            },
            {
                "name": "agent forbids helper endpoint",
                "desc": {**session_bank_descriptor("velis"), "helperEndpoint": helper},
                "want": "helperEndpoint",
            },
            {
                "name": "command forbids agent payload",
                "desc": {**python_descriptor("velis"), "agent": session_bank_descriptor("velis")["agent"]},
                "want": "agent",
            },
            {
                "name": "command forbids unresolved reason",
                "desc": {**python_descriptor("velis"), "unresolvedReason": "unsupported_workload"},
                "want": "unresolvedReason",
            },
            {
                "name": "topology workloadKind must be shell",
                "desc": {**topology_descriptor("velis"), "workloadKind": "unknown"},
                "want": "workloadKind",
            },
            {
                "name": "topology forbids command payload",
                "desc": {**topology_descriptor("velis"), "command": python_descriptor("velis")["command"]},
                "want": "command",
            },
            {
                "name": "topology forbids helper endpoint",
                "desc": {**topology_descriptor("velis"), "helperEndpoint": helper},
                "want": "helperEndpoint",
            },
            {
                "name": "managed workloadKind must be managed",
                "desc": {**managed_session("bridge", "bridge.service")["descriptors"][0], "workloadKind": "shell"},
                "sessionName": "bridge",
                "want": "workloadKind",
            },
            {
                "name": "managed forbids command payload",
                "desc": {**managed_session("bridge", "bridge.service")["descriptors"][0], "command": python_descriptor("velis")["command"]},
                "sessionName": "bridge",
                "want": "command",
            },
            {
                "name": "managed forbids helper endpoint",
                "desc": {**managed_session("bridge", "bridge.service")["descriptors"][0], "helperEndpoint": helper},
                "sessionName": "bridge",
                "want": "helperEndpoint",
            },
            {
                "name": "unresolved workloadKind must be unknown",
                "desc": {**unresolved_descriptor("velis"), "workloadKind": "shell"},
                "want": "workloadKind",
            },
            {
                "name": "unresolved requires reason",
                "desc": {key: value for key, value in unresolved_descriptor("velis").items() if key != "unresolvedReason"},
                "want": "unresolvedReason",
            },
            {
                "name": "unresolved reason must be allowlisted",
                "desc": {**unresolved_descriptor("velis"), "unresolvedReason": "node --token secret"},
                "want": "unresolvedReason",
            },
            {
                "name": "unresolved forbids command payload",
                "desc": {**unresolved_descriptor("velis"), "command": python_descriptor("velis")["command"]},
                "want": "command",
            },
            {
                "name": "unresolved forbids helper endpoint",
                "desc": {**unresolved_descriptor("velis"), "helperEndpoint": helper},
                "want": "helperEndpoint",
            },
            {
                "name": "session-level helper endpoint is not a command descriptor helper",
                "session": {
                    "sessionName": "velis",
                    "unixUser": "alice",
                    "helperEndpoint": helper,
                    "descriptors": [session_bank_descriptor("velis")],
                },
                "want": "helperEndpoint",
            },
        ]
        for case in cases:
            with self.subTest(case=case["name"]):
                if "session" in case:
                    session = case["session"]
                else:
                    session = {"sessionName": case.get("sessionName", "velis"), "unixUser": "alice", "descriptors": [case["desc"]]}
                with self.assertRaisesRegex(manifest.ManifestError, case["want"]):
                    manifest.new_manifest([session], now="2026-07-15T10:00:00Z")

    def test_descriptor_rejects_raw_or_unknown_extras_for_every_mode(self) -> None:
        modes = [
            ("agent", "velis", session_bank_descriptor("velis")),
            ("command", "velis", python_descriptor("velis")),
            ("topology", "velis", topology_descriptor("velis")),
            ("managed", "bridge", managed_session("bridge", "bridge.service")["descriptors"][0]),
            ("unresolved", "velis", unresolved_descriptor("velis")),
        ]
        extra_fields = [
            ("rawArgv", ["codex", "resume", CODEX_ID]),
            ("environment", {"OPENAI_API_KEY": "secret"}),
            ("token", "secret"),
            ("operatorNote", "descriptor notes are forbidden"),
            ("customExtra", {"anything": True}),
        ]
        for mode, session_name, base_desc in modes:
            for key, value in extra_fields:
                with self.subTest(mode=mode, key=key):
                    desc = {**base_desc, key: value}
                    with self.assertRaisesRegex(manifest.ManifestError, key):
                        manifest.new_manifest(
                            [{"sessionName": session_name, "unixUser": "alice", "descriptors": [desc]}],
                            now="2026-07-15T10:00:00Z",
                        )


class SnapshotTest(unittest.TestCase):
    def test_snapshot_groups_owner_probe_evidence_into_session_bank_descriptor_plan(self) -> None:
        fixture = json.loads((SCRIPT_DIR / "fixtures" / "codex_argv.json").read_text(encoding="utf-8"))
        sessions = snapshot.sessions_from_collected_evidence([fixture["input"]], unix_user="alice")
        self.assertEqual(len(sessions), 1)
        self.assertEqual(sessions[0]["sessionName"], "codex-alpha")
        self.assertEqual(sessions[0]["unixUser"], "alice")
        self.assertEqual(sessions[0]["descriptors"][0]["agent"]["nativeSessionId"], CODEX_ID)

    def test_python_http_snapshot_derives_loopback_helper_probe_from_typed_command(self) -> None:
        evidence = python_http_collected_evidence()
        sessions = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")
        desc = sessions[0]["descriptors"][0]
        self.assertNotIn("helperEndpoint", desc)
        self.assertNotIn("paneStatus", desc)
        self.assertEqual(
            sessions[0]["verification"][0]["helperEndpoint"],
            {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200},
        )
        doc = manifest.new_manifest(sessions, now="2026-07-15T10:00:00Z")
        self.assertEqual(
            doc["sessions"][0]["verification"][0]["helperEndpoint"],
            {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200},
        )

    def test_python_http_snapshot_does_not_derive_non_loopback_helper_probe(self) -> None:
        evidence = python_http_collected_evidence(bind="0.0.0.0")
        sessions = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")
        self.assertNotIn("helperEndpoint", sessions[0]["descriptors"][0])
        self.assertNotIn("helperEndpoint", sessions[0]["verification"][0])

    def test_snapshot_posts_go_compatible_descriptors_and_retains_operator_verification(self) -> None:
        sessions = snapshot.sessions_from_collected_evidence([python_http_collected_evidence()], unix_user="alice")
        with tempfile.TemporaryDirectory() as tmp:
            client = RecordingClient()
            result = snapshot.create_snapshot(client, sessions, Path(tmp), now="2026-07-15T10:00:00Z")
        posted_descriptor = client.updates[0][2]["recoveryPlan"][0]
        self.assertEqual(
            set(posted_descriptor),
            {"mode", "owner", "topology", "workloadKind", "command", "evidenceSource", "confidence"},
        )
        self.assertNotIn("paneStatus", posted_descriptor)
        self.assertNotIn("helperEndpoint", posted_descriptor)
        self.assertEqual(
            result.manifest["sessions"][0]["verification"][0]["helperEndpoint"],
            {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200},
        )

    def test_snapshot_posts_only_session_bank_plans_and_keeps_managed_manifest_only(self) -> None:
        client = RecordingClient()
        sessions = [
            {"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis")]},
            managed_session("bridge", "bridge.service"),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            result = snapshot.create_snapshot(client, sessions, Path(tmp), now="2026-07-15T10:00:00Z")
        self.assertEqual([(name, user) for name, user, _ in client.updates], [("velis", "alice")])
        self.assertNotIn("bridge", [name for name, _, _ in client.updates])
        self.assertTrue(result.path.name.endswith(".json"))
        managed = [item for item in result.manifest["sessions"] if item["sessionName"] == "bridge"][0]
        self.assertFalse(managed["restartAllowed"])
        self.assertEqual(managed["managerKind"], "systemd-user")

    def test_snapshot_stages_manifest_before_any_api_post(self) -> None:
        sessions = [{"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis")]}]
        with tempfile.TemporaryDirectory() as tmp:
            output_dir = Path(tmp) / "not-a-directory"
            output_dir.write_text("blocking file", encoding="utf-8")
            client = RecordingClient()
            with self.assertRaises(RuntimeError):
                snapshot.create_snapshot(client, sessions, output_dir, now="2026-07-15T10:00:00Z")
            self.assertEqual(client.session_reads, 0)
            self.assertEqual(client.updates, [])

    def test_snapshot_second_post_failure_rolls_back_previous_bank_state_without_final_manifest(self) -> None:
        old_desc = session_bank_descriptor("velis", pane_id="%old")
        new_desc = session_bank_descriptor("velis", pane_id="%new")
        sessions = [
            {"sessionName": "velis", "unixUser": "alice", "descriptors": [new_desc]},
            {"sessionName": "bridge", "unixUser": "alice", "descriptors": [session_bank_descriptor("bridge")]},
        ]

        class FailingSecondUpdateClient(RecordingClient):
            def update_session_recovery(self, name: str, unix_user: str, recovery_plan: list[dict]) -> dict:
                result = super().update_session_recovery(name, unix_user, recovery_plan)
                if len(self.updates) == 2:
                    raise RuntimeError("second post failed")
                return result

        client = FailingSecondUpdateClient()
        client.banked = [{"name": "velis", "unixUser": "alice", "recoveryPlan": [old_desc], "live": False}]
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaisesRegex(RuntimeError, "second post failed"):
                snapshot.create_snapshot(client, sessions, Path(tmp), now="2026-07-15T10:00:00Z")
            self.assertEqual(list(Path(tmp).glob("*.json")), [])
            self.assertEqual(len(list(Path(tmp).glob("*.pending"))), 1)
        self.assertEqual(client.session_reads, 1)
        self.assertEqual(client.restored_entries, [("velis", "alice", {"name": "velis", "unixUser": "alice", "recoveryPlan": [old_desc], "live": False})])

    def test_snapshot_second_post_failure_restores_existing_legacy_absent_plan_exactly(self) -> None:
        previous = {
            "name": "legacy-agent",
            "unixUser": "alice",
            "group": "codex",
            "windows": 1,
            "attached": False,
            "live": False,
            "firstSeen": "2026-07-09T00:00:00Z",
            "lastSeen": "2026-07-09T00:00:00Z",
            "recoveryKind": "agent",
            "agentKind": "codex",
            "agentSessionId": CODEX_ID,
            "resumeCommand": f"codex resume {CODEX_ID}",
            "cwd": "/home/alice/project",
        }
        sessions = [
            {"sessionName": "legacy-agent", "unixUser": "alice", "descriptors": [session_bank_descriptor("legacy-agent")]},
            {"sessionName": "bridge", "unixUser": "alice", "descriptors": [session_bank_descriptor("bridge")]},
        ]

        class FailingSecondUpdateClient(RecordingClient):
            def update_session_recovery(self, name: str, unix_user: str, recovery_plan: list[dict]) -> dict:
                result = super().update_session_recovery(name, unix_user, recovery_plan)
                if len(self.updates) == 2:
                    raise RuntimeError("second post failed")
                return result

        client = FailingSecondUpdateClient()
        client.banked = [json.loads(json.dumps(previous))]
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaisesRegex(RuntimeError, "second post failed"):
                snapshot.create_snapshot(client, sessions, Path(tmp), now="2026-07-15T10:00:00Z")
            self.assertEqual(list(Path(tmp).glob("*.json")), [])
            self.assertEqual(len(list(Path(tmp).glob("*.pending"))), 1)
        self.assertEqual(client.restored_entries, [("legacy-agent", "alice", previous)])
        self.assertEqual(client.deletes, [])

    def test_snapshot_final_rename_failure_rolls_back_new_bank_posts(self) -> None:
        sessions = [
            {"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis")]},
            {"sessionName": "bridge", "unixUser": "alice", "descriptors": [session_bank_descriptor("bridge")]},
        ]
        client = RecordingClient()
        original_replace = manifest.os.replace

        def failing_replace(src: str | os.PathLike[str], dst: str | os.PathLike[str]) -> None:
            if str(src).endswith(".pending"):
                raise OSError("final rename failed")
            original_replace(src, dst)

        manifest.os.replace = failing_replace
        try:
            with tempfile.TemporaryDirectory() as tmp:
                with self.assertRaisesRegex(RuntimeError, "final rename failed"):
                    snapshot.create_snapshot(client, sessions, Path(tmp), now="2026-07-15T10:00:00Z")
                self.assertEqual(list(Path(tmp).glob("*.json")), [])
                self.assertEqual(len(list(Path(tmp).glob("*.pending"))), 1)
        finally:
            manifest.os.replace = original_replace
        self.assertEqual(client.deletes, [("bridge", "alice"), ("velis", "alice")])

    def test_snapshot_final_rename_failure_restores_present_empty_entry_and_deletes_only_new_posts(self) -> None:
        previous = {
            "name": "unsafe-empty",
            "unixUser": "alice",
            "group": "codex",
            "windows": 1,
            "attached": False,
            "live": False,
            "firstSeen": "2026-07-09T00:00:00Z",
            "lastSeen": "2026-07-09T00:00:00Z",
            "recoveryKind": "unresolved",
            "recoveryPlan": [],
        }
        sessions = [
            {"sessionName": "unsafe-empty", "unixUser": "alice", "descriptors": [session_bank_descriptor("unsafe-empty")]},
            {"sessionName": "new-bank", "unixUser": "alice", "descriptors": [session_bank_descriptor("new-bank")]},
        ]
        client = RecordingClient()
        client.banked = [json.loads(json.dumps(previous))]
        original_replace = manifest.os.replace

        def failing_replace(src: str | os.PathLike[str], dst: str | os.PathLike[str]) -> None:
            if str(src).endswith(".pending"):
                raise OSError("final rename failed")
            original_replace(src, dst)

        manifest.os.replace = failing_replace
        try:
            with tempfile.TemporaryDirectory() as tmp:
                with self.assertRaisesRegex(RuntimeError, "final rename failed"):
                    snapshot.create_snapshot(client, sessions, Path(tmp), now="2026-07-15T10:00:00Z")
                self.assertEqual(list(Path(tmp).glob("*.json")), [])
                self.assertEqual(len(list(Path(tmp).glob("*.pending"))), 1)
        finally:
            manifest.os.replace = original_replace
        self.assertEqual(client.restored_entries, [("unsafe-empty", "alice", previous)])
        self.assertEqual(client.deletes, [("new-bank", "alice")])

    def test_snapshot_success_accepts_manifest_after_all_posts(self) -> None:
        sessions = [{"sessionName": "velis", "unixUser": "alice", "descriptors": [session_bank_descriptor("velis")]}]
        client = RecordingClient()
        with tempfile.TemporaryDirectory() as tmp:
            result = snapshot.create_snapshot(client, sessions, Path(tmp), now="2026-07-15T10:00:00Z")
            self.assertTrue(result.path.exists())
            self.assertEqual(result.path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(list(Path(tmp).glob("*.pending")), [])
        self.assertEqual([(name, user) for name, user, _ in client.updates], [("velis", "alice")])
        self.assertEqual(client.deletes, [])

    def test_snapshot_cli_collects_owner_evidence_and_allows_typed_managed_records(self) -> None:
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z")
            }
        )
        runner = collector.RecordingCommandRunner(
            {
                ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "velis\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\tcodex\t0\n"
            }
        )
        managed_path = write_json_temp([managed_session("bridge", "bridge.service")])
        try:
            with tempfile.TemporaryDirectory() as tmp:
                client = RecordingClient()
                code = snapshot.main(
                    [
                        "--api-url",
                        "http://127.0.0.1:8095",
                        "--socket",
                        "/tmp/test.sock",
                        "--unix-user",
                        "alice",
                        "--owner-home",
                        "/home/alice",
                        "--owner-kind",
                        "session_bank",
                        "--owner-ref",
                        "alice/velis",
                        "--owner-may-restart",
                        "--managed-records",
                        str(managed_path),
                        "--output-dir",
                        tmp,
                    ],
                    client=client,
                    command_runner=runner,
                    proc_reader=proc,
                    current_user="alice",
                )
                self.assertEqual(code, 0)
                self.assertEqual([(name, user) for name, user, _ in client.updates], [("velis", "alice")])
                self.assertEqual(len(list(Path(tmp).glob("*.json"))), 1)
        finally:
            managed_path.unlink(missing_ok=True)

    def test_session_bank_snapshot_derives_owner_ref_per_session_before_api_posts(self) -> None:
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/one", started_at="2026-07-15T10:00:00Z"),
                "52": ProcEntry(ppid="1", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/two", started_at="2026-07-15T10:00:00Z"),
            }
        )
        runner = collector.RecordingCommandRunner(
            {
                ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "\n".join(
                    [
                        "one\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/one\t42\tcodex\t0",
                        "two\t0\tagent\tb25f,80x24,0,0\t0\t%2\t/home/alice/two\t52\tcodex\t0",
                    ]
                )
                + "\n"
            }
        )
        with tempfile.TemporaryDirectory() as tmp:
            client = RecordingClient()
            code = snapshot.main(
                [
                    "--api-url",
                    "http://127.0.0.1:8095",
                    "--socket",
                    "/tmp/test.sock",
                    "--unix-user",
                    "alice",
                    "--owner-home",
                    "/home/alice",
                    "--owner-kind",
                    "session_bank",
                    "--owner-ref",
                    "alice/wrong",
                    "--owner-may-restart",
                    "--output-dir",
                    tmp,
                ],
                client=client,
                command_runner=runner,
                proc_reader=proc,
                current_user="alice",
            )
        self.assertEqual(code, 0)
        posted_refs = [body["recoveryPlan"][0]["owner"]["ref"] for _, _, body in client.updates]
        self.assertEqual(posted_refs, ["alice/one", "alice/two"])

    def test_persistent_collection_requires_explicit_session_filter_or_owner_map(self) -> None:
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="python", argv=["python"], cwd="/home/alice/one", started_at="2026-07-15T10:00:00Z"),
                "52": ProcEntry(ppid="1", comm="python", argv=["python"], cwd="/home/alice/two", started_at="2026-07-15T10:00:00Z"),
            }
        )
        runner = collector.RecordingCommandRunner(
            {
                ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "\n".join(
                    [
                        "one\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/one\t42\tpython\t0",
                        "two\t0\tagent\tb25f,80x24,0,0\t0\t%2\t/home/alice/two\t52\tpython\t0",
                    ]
                )
                + "\n"
            }
        )
        with self.assertRaisesRegex(collector.CollectorError, "explicit"):
            collector.collect_tmux_evidence(
                socket="/tmp/test.sock",
                owner={"kind": "persistent_agent", "ref": "persistent:alice/one", "mayRestart": True},
                owner_home="/home/alice",
                unix_user="alice",
                command_runner=runner,
                proc_reader=proc,
            )


class RestoreTest(unittest.TestCase):
    def test_restore_delegates_recovery_to_backend_and_topology_only_is_explicit(self) -> None:
        client = RecordingClient()
        status = RecordingStatusRunner()
        doc = manifest.new_manifest(
            [
                live_session_record("velis", [session_bank_descriptor("velis")]),
                managed_session("bridge", "bridge.service"),
            ],
            now="2026-07-15T10:00:00Z",
        )
        result = restore.restore_manifest(doc, client, status_runner=status, verifier=lambda **_: {"ok": True, "sessions": []})
        self.assertEqual(client.recovers, [("velis", "alice", {})])
        self.assertEqual(status.calls, [{"probe": {"kind": "systemd-user", "unit": "bridge.service", "expectActiveState": "active"}, "unixUser": "alice"}])
        self.assertTrue(result["ok"])

        client = RecordingClient()
        restore.restore_manifest(doc, client, status_runner=RecordingStatusRunner(), topology_only=True, verifier=lambda **_: {"ok": True, "sessions": []})
        self.assertEqual(client.recovers, [("velis", "alice", {"topologyOnly": True})])

    def test_restore_writes_managed_status_registry_from_actual_status_probe(self) -> None:
        doc = manifest.new_manifest([managed_session("bridge", "bridge.service")], now="2026-07-15T10:00:00Z")
        status = RecordingStatusRunner()
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "managed-status.json"
            result = restore.restore_manifest(
                doc,
                RecordingClient(),
                status_runner=status,
                verifier=lambda **_: {"ok": True, "sessions": []},
                managed_status_output=output,
            )
            self.assertTrue(result["ok"], result)
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            entries = json.loads(output.read_text(encoding="utf-8"))
        self.assertEqual(status.calls, [{"probe": {"kind": "systemd-user", "unit": "bridge.service", "expectActiveState": "active"}, "unixUser": "alice"}])
        self.assertEqual(len(entries), 1)
        entry = entries[0]
        self.assertEqual(entry["name"], "bridge")
        self.assertEqual(entry["sessionName"], "bridge")
        self.assertEqual(entry["unixUser"], "alice")
        self.assertEqual(entry["owner"], {"kind": "external_manager", "ref": "systemd:user/bridge.service", "mayRestart": False})
        self.assertEqual(entry["managerKind"], "systemd-user")
        self.assertEqual(entry["managerRef"], "bridge.service")
        self.assertEqual(entry["status"]["activeState"], "active")
        self.assertTrue(entry["status"]["ok"])
        self.assertEqual(entry["storageKind"], "managed-status")
        self.assertEqual(entry["sourceKind"], "restore")
        self.assertNotIn("descriptors", entry)
        self.assertNotIn("statusProbe", entry)
        self.assertNotIn("restartAllowed", entry)

    def test_restore_managed_status_output_validation_matches_go_reader_strictness(self) -> None:
        base = {
            "name": "bridge",
            "sessionName": "bridge",
            "unixUser": "alice",
            "owner": {"kind": "external_manager", "ref": "systemd:user/bridge.service", "mayRestart": False},
            "managerKind": "systemd-user",
            "managerRef": "bridge.service",
            "status": {"ok": True, "activeState": "active", "checkedAt": "2026-07-15T10:00:00Z"},
            "storageKind": "managed-status",
            "sourceKind": "restore",
        }
        restore._validate_managed_status_output_entry(json.loads(json.dumps(base)))
        cases = {
            "missing unix user": lambda entry: entry.pop("unixUser"),
            "unsafe unix user": lambda entry: entry.update({"unixUser": "Alice"}),
            "dotted session name": lambda entry: entry.update({"name": "bridge.worker", "sessionName": "bridge.worker"}),
            "plus session name": lambda entry: entry.update({"name": "bridge+worker", "sessionName": "bridge+worker"}),
            "overlong session name": lambda entry: entry.update({"name": "a" * 51, "sessionName": "a" * 51}),
            "owner may restart": lambda entry: entry["owner"].update({"mayRestart": True}),
            "owner ref mismatch": lambda entry: entry["owner"].update({"ref": "systemd:user/other.service"}),
            "ok activeState rejected": lambda entry: entry["status"].update({"activeState": "ok"}),
            "inactive cannot be ok": lambda entry: entry["status"].update({"activeState": "inactive"}),
            "wrong storage kind": lambda entry: entry.update({"storageKind": "banked"}),
            "wrong source kind": lambda entry: entry.update({"sourceKind": "manual"}),
        }
        for name, mutate in cases.items():
            with self.subTest(name=name):
                entry = json.loads(json.dumps(base))
                mutate(entry)
                with self.assertRaises(ValueError):
                    restore._validate_managed_status_output_entry(entry)

    def test_managed_status_atomic_write_uses_unique_temp_files_under_concurrency(self) -> None:
        def entry(index: int) -> dict:
            unit = f"bridge-{index}.service"
            return {
                "name": f"bridge-{index}",
                "sessionName": f"bridge-{index}",
                "unixUser": "alice",
                "owner": {"kind": "external_manager", "ref": f"systemd:user/{unit}", "mayRestart": False},
                "managerKind": "systemd-user",
                "managerRef": unit,
                "status": {"ok": True, "activeState": "active", "checkedAt": "2026-07-15T10:00:00Z"},
                "storageKind": "managed-status",
                "sourceKind": "restore",
            }

        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "managed-status.json"
            values = [[entry(index)] for index in range(24)]
            with ThreadPoolExecutor(max_workers=8) as pool:
                list(pool.map(lambda value: restore._atomic_write_json_0600(output, value), values))
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            final = json.loads(output.read_text(encoding="utf-8"))
            self.assertIn(final, values)
            self.assertEqual(list(Path(tmp).glob("managed-status.json.tmp")), [])
            for leftover in Path(tmp).glob("managed-status.json.*.tmp"):
                self.fail(f"leftover managed status temp file: {leftover}")

    def test_skip_live_backend_response_is_not_success_without_verification_health(self) -> None:
        client = RecordingClient()
        client.recover_responses["velis"] = {"success": True, "action": "skip-live", "session": "velis"}
        doc = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        result = restore.restore_manifest(doc, client, status_runner=RecordingStatusRunner(), verifier=lambda **_: {"ok": False, "sessions": [{"sessionName": "velis", "ok": False, "errors": ["missing agent identity"]}]})
        self.assertFalse(result["ok"])
        self.assertEqual(result["sessions"][0]["restore"]["action"], "skip-live")

    def test_restore_collects_per_session_errors_and_continues(self) -> None:
        doc = manifest.new_manifest(
            [
                live_session_record("velis", [session_bank_descriptor("velis")]),
                live_session_record("other", [session_bank_descriptor("other")]),
            ],
            now="2026-07-15T10:00:00Z",
        )
        result = restore.restore_manifest(doc, RaisingRecoverClient(), status_runner=RecordingStatusRunner(), verifier=lambda **_: {"ok": True, "sessions": []})
        self.assertFalse(result["ok"])
        self.assertEqual([item["restore"]["action"] for item in result["sessions"]], ["error", "recovered"])
        self.assertIn("recover failed", result["sessions"][0]["errors"][0])

    def test_restore_recollects_after_backend_calls_for_normal_verification(self) -> None:
        client = RecordingClient()
        observed = [live_session_record("velis", [session_bank_descriptor("velis")])]
        calls: list[str] = []

        def provider() -> list[dict]:
            calls.append("recollected")
            self.assertEqual(client.recovers, [("velis", "alice", {})])
            return observed

        doc = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        result = restore.restore_manifest(doc, client, status_runner=RecordingStatusRunner(), observed_provider=provider)
        self.assertTrue(result["ok"], result)
        self.assertEqual(calls, ["recollected"])

    def test_restore_passes_bounded_readiness_to_verifier(self) -> None:
        captured: dict[str, Any] = {}

        def verifier(**kwargs: Any) -> dict[str, Any]:
            captured.update(kwargs)
            return {"ok": True, "sessions": [{"sessionName": "velis", "unixUser": "alice", "ok": True, "errors": []}]}

        doc = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        result = restore.restore_manifest(
            doc,
            RecordingClient(),
            status_runner=RecordingStatusRunner(),
            verifier=verifier,
            readiness_seconds=10,
            readiness_interval_seconds=0.5,
            stability_seconds=2,
        )
        self.assertTrue(result["ok"], result)
        self.assertEqual(captured["readiness_seconds"], 10)
        self.assertEqual(captured["readiness_interval_seconds"], 0.5)
        self.assertEqual(captured["stability_seconds"], 2)

    def test_restore_result_preserves_readiness_and_stability_evidence(self) -> None:
        doc = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        result = restore.restore_manifest(
            doc,
            RecordingClient(),
            status_runner=RecordingStatusRunner(),
            verifier=lambda **_: {
                "ok": True,
                "sessions": [{"sessionName": "velis", "unixUser": "alice", "ok": True, "errors": []}],
                "readinessSeconds": 5,
                "readinessSamples": [{"ok": True, "elapsedSeconds": 0}],
                "stabilitySeconds": 2,
                "stabilitySamples": [{"ok": True, "elapsedSeconds": 1}],
            },
        )
        self.assertEqual(result["readinessSeconds"], 5)
        self.assertEqual(result["readinessSamples"], [{"ok": True, "elapsedSeconds": 0}])
        self.assertEqual(result["stabilitySeconds"], 2)
        self.assertEqual(result["stabilitySamples"], [{"ok": True, "elapsedSeconds": 1}])

    def test_restore_verification_uses_derived_python_http_helper_probe(self) -> None:
        sessions = snapshot.sessions_from_collected_evidence([python_http_collected_evidence()], unix_user="alice")
        doc = manifest.new_manifest(sessions, now="2026-07-15T10:00:00Z")
        observed = json.loads(json.dumps(sessions))
        http = RecordingHTTPRunner()
        result = restore.restore_manifest(
            doc,
            RecordingClient(),
            status_runner=RecordingStatusRunner(),
            observed_sessions=observed,
            http_runner=http,
            stability_seconds=0,
        )
        self.assertTrue(result["ok"], result)
        self.assertEqual(http.calls, [{"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}])


    def test_managed_fixtures_never_post_recovery_or_read_environment(self) -> None:
        for fixture in sorted(FIXTURES.glob("*_managed_manifest.json")):
            with self.subTest(fixture=fixture.name):
                client = RecordingClient()
                status = RecordingStatusRunner()
                doc = json.loads(fixture.read_text(encoding="utf-8"))
                result = restore.restore_manifest(doc, client, status_runner=status, verifier=lambda **_: {"ok": True, "sessions": []})
                self.assertEqual(client.updates, [])
                self.assertEqual(client.recovers, [])
                self.assertGreater(len(status.calls), 0)
                self.assertTrue(result["ok"])
                for call in status.calls:
                    self.assertEqual(call["probe"]["kind"], "systemd-user")
                    self.assertEqual(call["unixUser"], "alice")


class VerifyTest(unittest.TestCase):
    def test_helper_http_runner_does_not_follow_redirects_to_non_loopback_targets(self) -> None:
        class RedirectingOpener:
            def __init__(self) -> None:
                self.calls: list[str] = []

            def open(self, req: object, timeout: float) -> object:
                url = getattr(req, "full_url", str(req))
                self.calls.append(url)
                raise urllib_error.HTTPError(url, 302, "Found", {"Location": "http://example.com/secret"}, None)

        opener = RedirectingOpener()
        runner = verify.HTTPRunner(opener=opener)
        result = runner.check({"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200})
        self.assertFalse(result["ok"])
        self.assertEqual(opener.calls, ["http://127.0.0.1:8088/"])

    def test_readiness_polls_helper_until_manifest_is_fully_healthy(self) -> None:
        helper = {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}
        python = python_descriptor("velis")
        expected = manifest.new_manifest(
            [
                session_record(
                    "velis",
                    [python],
                    verification=[verification_for_descriptor(python, helper=helper)],
                )
            ],
            now="2026-07-15T10:00:00Z",
        )
        sleeps: list[float] = []
        http = SequencedHTTPRunner([False, False, True])
        result = verify.verify_manifest(
            expected,
            observed_sessions=[session_record("velis", [python], verification=[verification_for_descriptor(python, helper=helper)])],
            http_runner=http,
            readiness_seconds=2,
            readiness_interval_seconds=0.5,
            sleep=sleeps.append,
        )
        self.assertTrue(result["ok"], result)
        self.assertEqual(sleeps, [0.5, 0.5])
        self.assertEqual(len(http.calls), 3)
        self.assertEqual(result["readinessSamples"], 3)

    def test_readiness_polls_until_descriptor_appears(self) -> None:
        expected = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        observed = [live_session_record("velis", [session_bank_descriptor("velis")])]
        samples = iter([[], [], observed])
        provider_calls: list[str] = []

        def provider() -> list[dict]:
            provider_calls.append("sample")
            return next(samples)

        sleeps: list[float] = []
        result = verify.verify_manifest(
            expected,
            observed_provider=provider,
            readiness_seconds=2,
            readiness_interval_seconds=0.5,
            sleep=sleeps.append,
        )
        self.assertTrue(result["ok"], result)
        self.assertEqual(provider_calls, ["sample", "sample", "sample"])
        self.assertEqual(sleeps, [0.5, 0.5])

    def test_readiness_times_out_permanent_descriptor_mismatch_without_stability_sleep(self) -> None:
        expected = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        wrong = session_bank_descriptor("velis")
        wrong["agent"]["nativeSessionId"] = CLAUDE_ID
        observed = [live_session_record("velis", [wrong])]
        sleeps: list[float] = []
        result = verify.verify_manifest(
            expected,
            observed_provider=lambda: observed,
            readiness_seconds=1,
            readiness_interval_seconds=0.5,
            stability_seconds=3,
            sleep=sleeps.append,
        )
        self.assertFalse(result["ok"], result)
        self.assertEqual(sleeps, [0.5, 0.5])
        self.assertEqual(result["readinessSamples"], 3)
        self.assertEqual(result["stabilitySamples"], 1)
        self.assertIn("missing expected descriptor", " ".join(result["sessions"][0]["errors"]))

    def test_readiness_and_stability_are_independent_samples(self) -> None:
        expected = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        observed = [live_session_record("velis", [session_bank_descriptor("velis")])]
        samples = iter([[], observed, observed])
        calls: list[str] = []

        def provider() -> list[dict]:
            calls.append("sample")
            return next(samples)

        sleeps: list[float] = []
        result = verify.verify_manifest(
            expected,
            observed_provider=provider,
            readiness_seconds=2,
            readiness_interval_seconds=0.5,
            stability_seconds=3,
            sleep=sleeps.append,
        )
        self.assertTrue(result["ok"], result)
        self.assertEqual(calls, ["sample", "sample", "sample"])
        self.assertEqual(sleeps, [0.5, 3])
        self.assertEqual(result["readinessSamples"], 2)
        self.assertEqual(result["stabilitySamples"], 2)

    def test_tmux_window_layout_matches_semantically_after_reallocated_pane_ids(self) -> None:
        def topology(layout: str) -> dict:
            return {
                "sessionName": "velis",
                "windowName": "agent",
                "windowLayout": layout,
                "paneCurrentPath": "/home/alice/project",
            }

        same_layout_pairs = [
            ("b25d,80x24,0,0,0", "91aa,80x24,0,0,4"),
            (
                "aaaa,160x40,0,0{80x40,0,0,1,79x40,81,0[79x20,81,0,2,79x19,81,21,3]}",
                "bbbb,160x40,0,0{80x40,0,0,10,79x40,81,0[79x20,81,0,11,79x19,81,21,12]}",
            ),
        ]
        for old_layout, new_layout in same_layout_pairs:
            with self.subTest(old=old_layout, new=new_layout):
                self.assertTrue(verify._topology_matches(topology(old_layout), topology(new_layout)))

        different_layout_pairs = [
            ("b25d,80x24,0,0,0", "91aa,81x24,0,0,4"),
            ("aaaa,80x24,0,0[80x12,0,0,1,80x11,0,13,2]", "bbbb,80x24,0,0{80x12,0,0,3,80x11,0,13,4}"),
            ("aaaa,80x24,0,0[80x12,0,0,1,80x11,0,13,2]", "bbbb,80x24,0,0[80x11,0,13,4,80x12,0,0,3]"),
            ("aaaa,80x24,0,0[80x12,0,0,1,80x11,0,13,2]", "bbbb,80x24,0,0[80x12,0,0,3]"),
            ("not-a-layout", "91aa,80x24,0,0,4"),
        ]
        for old_layout, new_layout in different_layout_pairs:
            with self.subTest(old=old_layout, new=new_layout):
                self.assertFalse(verify._topology_matches(topology(old_layout), topology(new_layout)))

    def test_verify_requires_exact_agents_pane_health_helper_endpoint_and_managed_status(self) -> None:
        helper = {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}
        codex = session_bank_descriptor("velis")
        hermes = hermes_descriptor("velis")
        python = python_descriptor("velis")
        expected = manifest.new_manifest(
            [
                session_record(
                    "velis",
                    [codex, hermes, python],
                    verification=[
                        verification_for_descriptor(codex),
                        verification_for_descriptor(hermes),
                        verification_for_descriptor(python, helper=helper),
                    ],
                ),
                managed_session("bridge", "bridge.service"),
            ],
            now="2026-07-15T10:00:00Z",
        )
        observed = [
            session_record(
                "velis",
                [session_bank_descriptor("velis"), hermes_descriptor("velis"), python_descriptor("velis")],
                verification=[
                    verification_for_descriptor(session_bank_descriptor("velis")),
                    verification_for_descriptor(hermes_descriptor("velis")),
                    verification_for_descriptor(python_descriptor("velis"), helper=helper),
                ],
            )
        ]
        status = RecordingStatusRunner()
        http = RecordingHTTPRunner()
        result = verify.verify_manifest(expected, observed_sessions=observed, status_runner=status, http_runner=http, stability_seconds=0)
        self.assertTrue(result["ok"], result)
        self.assertEqual(http.calls, [{"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}])

        bad_observed = [
            session_record(
                "velis",
                [session_bank_descriptor("velis"), hermes_descriptor("velis", profile="wrong"), python_descriptor("velis")],
                verification=[
                    verification_for_descriptor(session_bank_descriptor("velis")),
                    verification_for_descriptor(hermes_descriptor("velis", profile="wrong")),
                    verification_for_descriptor(python_descriptor("velis"), helper=helper),
                ],
            )
        ]
        bad = verify.verify_manifest(expected, observed_sessions=bad_observed, status_runner=status, http_runner=http, stability_seconds=0)
        self.assertFalse(bad["ok"])
        self.assertIn("missing expected descriptor", " ".join(bad["sessions"][0]["errors"]))

    def test_verify_matches_reallocated_single_pane_windows_by_semantic_layout(self) -> None:
        helper = {"kind": "http-get", "url": "http://127.0.0.1:8088/", "expectStatus": 200}
        expected_hermes = retarget_descriptor(
            hermes_descriptor("ctx-sh7-smoke"),
            session_id="$old",
            window_index=0,
            window_name="agent",
            window_layout="b25d,80x24,0,0,0",
            pane_index=0,
            pane_id="%0",
            cwd="/home/alice/project",
        )
        expected_python = retarget_descriptor(
            python_descriptor("ctx-sh7-smoke"),
            session_id="$old",
            window_index=1,
            window_name="server",
            window_layout="aaaa,80x24,0,0,1",
            pane_index=0,
            pane_id="%1",
            cwd="/home/alice/project/public",
        )
        observed_hermes = retarget_descriptor(
            hermes_descriptor("ctx-sh7-smoke"),
            session_id="$new",
            window_index=0,
            window_name="agent",
            window_layout="91aa,80x24,0,0,4",
            pane_index=0,
            pane_id="%4",
            cwd="/home/alice/project",
        )
        observed_python = retarget_descriptor(
            python_descriptor("ctx-sh7-smoke"),
            session_id="$new",
            window_index=1,
            window_name="server",
            window_layout="4488,80x24,0,0,5",
            pane_index=0,
            pane_id="%5",
            cwd="/home/alice/project/public",
        )
        expected = manifest.new_manifest(
            [
                session_record(
                    "ctx-sh7-smoke",
                    [expected_hermes, expected_python],
                    verification=[
                        verification_for_descriptor(expected_hermes),
                        verification_for_descriptor(expected_python, helper=helper),
                    ],
                )
            ],
            now="2026-07-15T10:00:00Z",
        )
        observed = session_record(
            "ctx-sh7-smoke",
            [observed_hermes, observed_python],
            verification=[
                verification_for_descriptor(observed_hermes),
                verification_for_descriptor(observed_python, helper=helper),
            ],
        )
        result = verify.verify_manifest(expected, observed_sessions=[observed], http_runner=RecordingHTTPRunner(), stability_seconds=0)
        self.assertTrue(result["ok"], result)

        bad_observed = json.loads(json.dumps(observed))
        bad_observed["descriptors"][0]["topology"]["windowLayout"] = "91aa,81x24,0,0,4"
        bad = verify.verify_manifest(expected, observed_sessions=[bad_observed], http_runner=RecordingHTTPRunner(), stability_seconds=0)
        self.assertFalse(bad["ok"], bad)
        self.assertIn("windowLayout", " ".join(bad["sessions"][0]["errors"]))

    def test_verify_matches_reallocated_tmux_ids_by_logical_window_and_pane_ordinals(self) -> None:
        expected_session, observed_session, helper = reallocated_verify_sessions()
        expected = manifest.new_manifest([expected_session], now="2026-07-15T10:00:00Z")
        http = RecordingHTTPRunner()
        result = verify.verify_manifest(expected, observed_sessions=[observed_session], http_runner=http, stability_seconds=0)
        self.assertTrue(result["ok"], result)
        self.assertEqual(http.calls, [helper])

        cases = [
            (
                "window name",
                lambda session: session["descriptors"][0]["topology"].update({"windowName": "renamed"}),
                "missing expected descriptor",
            ),
            (
                "window layout",
                lambda session: session["descriptors"][1]["topology"].update({"windowLayout": "changed-layout"}),
                "missing expected descriptor",
            ),
            (
                "cwd",
                lambda session: session["verification"][2]["paneStatus"].update({"cwd": "/home/alice/wrong"}),
                "pane health mismatch",
            ),
            (
                "missing pane",
                lambda session: (session["descriptors"].pop(1), session["verification"].pop(1)),
                "missing expected pane",
            ),
            (
                "wrong agent id",
                lambda session: session["descriptors"][0]["agent"].update({"nativeSessionId": CLAUDE_ID}),
                "missing expected descriptor",
            ),
            (
                "reordered panes",
                lambda session: (
                    session["descriptors"][0]["topology"].update({"paneIndex": 1}),
                    session["descriptors"][1]["topology"].update({"paneIndex": 0}),
                    session["verification"][0]["target"].update({"paneIndex": 1}),
                    session["verification"][1]["target"].update({"paneIndex": 0}),
                ),
                "missing expected descriptor",
            ),
        ]
        for name, mutate, want in cases:
            with self.subTest(name=name):
                _, bad_observed, _ = reallocated_verify_sessions()
                mutate(bad_observed)
                bad = verify.verify_manifest(expected, observed_sessions=[bad_observed], http_runner=RecordingHTTPRunner(), stability_seconds=0)
                self.assertFalse(bad["ok"], bad)
                self.assertIn(want, " ".join(bad["sessions"][0]["errors"]))

    def test_stability_interval_samples_twice(self) -> None:
        expected = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        observed = [live_session_record("velis", [session_bank_descriptor("velis")])]
        sleeps: list[float] = []
        result = verify.verify_manifest(
            expected,
            observed_provider=lambda: observed,
            status_runner=RecordingStatusRunner(),
            http_runner=RecordingHTTPRunner(),
            stability_seconds=3,
            sleep=sleeps.append,
        )
        self.assertTrue(result["ok"], result)
        self.assertEqual(sleeps, [3])

    def test_verify_rejects_extra_panes_unless_allowed_and_topology_only_relaxes_identity(self) -> None:
        expected = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        extra_pane = session_bank_descriptor("velis", pane_id="%2")
        extra_pane["topology"]["paneIndex"] = 1
        observed_extra = [
            live_session_record("velis", [session_bank_descriptor("velis"), extra_pane])
        ]
        result = verify.verify_manifest(expected, observed_sessions=observed_extra, status_runner=RecordingStatusRunner(), http_runner=RecordingHTTPRunner())
        self.assertFalse(result["ok"])
        self.assertIn("extra pane", " ".join(result["sessions"][0]["errors"]))

        expected["sessions"][0]["allowExtraPanes"] = True
        allowed = verify.verify_manifest(expected, observed_sessions=observed_extra, status_runner=RecordingStatusRunner(), http_runner=RecordingHTTPRunner())
        self.assertTrue(allowed["ok"], allowed)

        topology = {
            "mode": "topology",
            "owner": {"kind": "session_bank", "ref": "alice/velis", "mayRestart": True},
            "topology": session_bank_descriptor("velis")["topology"],
            "workloadKind": "shell",
            "evidenceSource": "topology",
            "confidence": "medium",
        }
        observed_topology = [live_session_record("velis", [topology])]
        topology_only = verify.verify_manifest(expected, observed_sessions=observed_topology, topology_only=True)
        self.assertTrue(topology_only["ok"], topology_only)
        self.assertEqual(topology_only["mode"], "topology-only")

    def test_managed_status_runner_fails_wrong_unix_user_without_invoking_systemctl(self) -> None:
        runner = verify.SystemdUserStatusRunner(current_user="bob", command_runner=collector.RecordingCommandRunner({}))
        result = runner.check({"kind": "systemd-user", "unit": "bridge.service", "expectActiveState": "active"}, unix_user="alice")
        self.assertFalse(result["ok"])
        self.assertIn("current user bob", result["error"])

    def test_verify_cli_requires_observed_fixture_opt_in_and_defaults_to_thirty_second_stability(self) -> None:
        expected = manifest.new_manifest(
            [live_session_record("velis", [session_bank_descriptor("velis")])],
            now="2026-07-15T10:00:00Z",
        )
        observed = [live_session_record("velis", [session_bank_descriptor("velis")])]
        manifest_path = write_json_temp(expected)
        observed_path = write_json_temp(observed)
        try:
            with self.assertRaises(SystemExit):
                verify.main(["--manifest", str(manifest_path), "--observed", str(observed_path)])
            sleeps: list[float] = []
            stdout = StringIO()
            with redirect_stdout(stdout):
                code = verify.main(
                    ["--manifest", str(manifest_path), "--observed", str(observed_path), "--allow-test-observed"],
                    sleep=sleeps.append,
                )
            self.assertEqual(code, 0)
            self.assertEqual(sleeps, [30])
            result = json.loads(stdout.getvalue())
            self.assertEqual(result["readinessSeconds"], 30)
        finally:
            manifest_path.unlink(missing_ok=True)
            observed_path.unlink(missing_ok=True)


class CollectorTest(unittest.TestCase):
    def test_collector_emits_only_bounded_probe_evidence_and_never_reads_env_or_history(self) -> None:
        runner = collector.RecordingCommandRunner(
            {
                ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "velis\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\tcodex\t0\n",
            }
        )
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z")
            }
        )
        evidence = collector.collect_tmux_evidence(
            socket="/tmp/test.sock",
            owner={"kind": "session_bank", "ref": "alice/velis", "mayRestart": True},
            owner_home="/home/alice",
            command_runner=runner,
            proc_reader=proc,
        )
        self.assertEqual(len(evidence), 1)
        keys = json.dumps(evidence)
        self.assertNotIn("env", keys)
        self.assertNotIn("transcript", keys)
        invoked = " ".join(" ".join(call) for call in runner.calls)
        self.assertNotIn("env", invoked)
        self.assertNotIn("printenv", invoked)
        self.assertNotIn("history", invoked)
        self.assertTrue(all("environ" not in path for path in proc.read_paths))

    def test_collector_includes_pane_status_for_live_and_dead_panes(self) -> None:
        runner = collector.RecordingCommandRunner(
            {
                ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "\n".join(
                    [
                        "velis\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\tcodex\t0",
                        "velis\t0\tagent\tb25f,80x24,0,0\t1\t%2\t/home/alice/project\t43\tbash\t1",
                    ]
                )
                + "\n"
            }
        )
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z"),
                "43": ProcEntry(ppid="1", comm="bash", argv=["bash"], cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z"),
            }
        )
        evidence = collector.collect_tmux_evidence(
            socket="/tmp/test.sock",
            owner={"kind": "session_bank", "ref": "alice/velis", "mayRestart": True},
            owner_home="/home/alice",
            command_runner=runner,
            proc_reader=proc,
        )
        self.assertEqual(evidence[0]["paneStatus"], {"dead": False, "currentCommand": "codex", "cwd": "/home/alice/project"})
        self.assertEqual(evidence[1]["paneStatus"], {"dead": True, "currentCommand": "bash", "cwd": "/home/alice/project"})
        session = snapshot.sessions_from_collected_evidence([evidence[0]], unix_user="alice")[0]
        self.assertNotIn("paneStatus", session["descriptors"][0])
        self.assertEqual(session["verification"][0]["paneStatus"], {"dead": False, "currentCommand": "codex", "cwd": "/home/alice/project"})

    def test_proc_tree_redacts_unknown_argv_classifies_descendant_and_rejects_conflicts(self) -> None:
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="bash", argv=["bash"], cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z"),
                "43": ProcEntry(ppid="42", comm="node", argv=["node", "server.js", "--token", "secret"], cwd="/home/alice/project", started_at="2026-07-15T10:00:01Z"),
                "44": ProcEntry(ppid="43", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/project", started_at="2026-07-15T10:00:02Z"),
            }
        )
        runner = collector.RecordingCommandRunner(
            {
                ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "velis\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\tbash\t0\n"
            }
        )
        evidence = collector.collect_tmux_evidence(
            socket="/tmp/test.sock",
            owner={"kind": "session_bank", "ref": "alice/velis", "mayRestart": True},
            owner_home="/home/alice",
            command_runner=runner,
            proc_reader=proc,
        )[0]
        self.assertNotIn("secret", json.dumps(evidence))
        desc = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")[0]["descriptors"][0]
        self.assertEqual(desc["agent"]["nativeSessionId"], CODEX_ID)

        conflict = dict(evidence)
        conflict["processTree"] = evidence["processTree"] + [
            {"comm": "claude", "argv": ["claude", "--resume", CLAUDE_ID], "cwd": "/home/alice/project", "startedAt": "2026-07-15T10:00:03Z"}
        ]
        conflict_desc = snapshot.sessions_from_collected_evidence([conflict], unix_user="alice")[0]["descriptors"][0]
        self.assertEqual(conflict_desc["mode"], "unresolved")
        self.assertEqual(conflict_desc["unresolvedReason"], "conflicting_evidence")

    def test_recognized_commands_with_secret_like_extra_options_are_redacted_and_unresolved(self) -> None:
        cases = [
            ("codex-unsafe", "codex", ["codex", "resume", CODEX_ID, "--token", "leak-value"]),
            ("claude-unsafe", "claude", ["claude", "--resume", CLAUDE_ID, "--api-key", "leak-value"]),
            (
                "hermes-unsafe",
                "python",
                [
                    "/home/alice/.hermes/hermes-agent-current/venv/bin/python",
                    "-m",
                    "hermes_cli.main",
                    "--profile",
                    "scout",
                    "--resume",
                    HERMES_ID,
                    "--token",
                    "leak-value",
                ],
            ),
        ]
        for session_name, comm, argv in cases:
            with self.subTest(session=session_name):
                proc = FakeProcReader(
                    {
                        "42": ProcEntry(ppid="1", comm=comm, argv=argv, cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z")
                    }
                )
                runner = collector.RecordingCommandRunner(
                    {
                        ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): f"{session_name}\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\t{comm}\t0\n"
                    }
                )
                evidence = collector.collect_tmux_evidence(
                    socket="/tmp/test.sock",
                    owner={"kind": "session_bank", "ref": "alice/wrong", "mayRestart": True},
                    owner_home="/home/alice",
                    unix_user="alice",
                    command_runner=runner,
                    proc_reader=proc,
                )[0]
                self.assertNotIn("leak-value", json.dumps(evidence))
                desc = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")[0]["descriptors"][0]
                self.assertEqual(desc["mode"], "unresolved")

    def test_proc_tree_reads_cmdline_and_cwd_only_for_pane_ancestry(self) -> None:
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="bash", argv=["bash"], cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z"),
                "43": ProcEntry(ppid="42", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/project", started_at="2026-07-15T10:00:01Z"),
                "99": ProcEntry(ppid="1", comm="node", argv=["node", "--token", "sibling-secret"], cwd="/home/alice/other", started_at="2026-07-15T10:00:01Z"),
            }
        )
        tree = collector.collect_process_tree("42", "/home/alice", proc_reader=proc)
        self.assertEqual([item["pid"] for item in tree], ["42", "43"])
        self.assertIn("/proc/99/stat", proc.read_paths)
        self.assertNotIn("/proc/99/cmdline", proc.read_paths)
        self.assertNotIn("/proc/99/cwd", proc.read_paths)

    def test_proc_tree_rechecks_pid_identity_before_cmdline_to_avoid_reuse_leaks(self) -> None:
        proc = ReusingProcReader(
            stable=ProcEntry(
                ppid="1",
                comm="bash",
                argv=["bash"],
                cwd="/home/alice/project",
                started_at="2026-07-15T10:00:00Z",
                start_ticks="100",
            ),
            reused=ProcEntry(
                ppid="1",
                comm="codex",
                argv=["codex", "resume", CODEX_ID, "--token", "leak-value"],
                cwd="/home/alice/project",
                started_at="2026-07-15T10:00:03Z",
                start_ticks="999",
            ),
        )
        tree = collector.collect_process_tree("42", "/home/alice", proc_reader=proc)
        self.assertNotIn("/proc/42/cmdline", proc.read_paths)
        self.assertNotIn("leak-value", json.dumps(tree))
        evidence = {
            "owner": {"kind": "session_bank", "ref": "alice/velis", "mayRestart": True},
            "ownerHome": "/home/alice",
            "sessionName": "velis",
            "unixUser": "alice",
            "pane": {
                "windowIndex": 0,
                "windowName": "agent",
                "windowLayout": "b25f,80x24,0,0",
                "paneIndex": 0,
                "paneId": "%1",
                "cwd": "/home/alice/project",
            },
            "paneStatus": {"dead": False, "currentCommand": "bash", "cwd": "/home/alice/project"},
            "process": tree[0] if tree else {},
            "processTree": tree,
        }
        desc = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")[0]["descriptors"][0]
        self.assertEqual(desc["mode"], "unresolved")

    def test_proc_tree_discards_cmdline_when_pid_identity_changes_after_read(self) -> None:
        proc = ReusingAfterReadProcReader(
            stable=ProcEntry(
                ppid="1",
                comm="bash",
                argv=["bash"],
                cwd="/home/alice/project",
                started_at="2026-07-15T10:00:00Z",
                start_ticks="100",
            ),
            reused=ProcEntry(
                ppid="1",
                comm="codex",
                argv=["codex", "resume", CODEX_ID, "--token", "leak-value"],
                cwd="/home/alice/project",
                started_at="2026-07-15T10:00:03Z",
                start_ticks="999",
            ),
        )
        tree = collector.collect_process_tree("42", "/home/alice", proc_reader=proc)
        self.assertIn("/proc/42/cmdline", proc.read_paths)
        self.assertNotIn("leak-value", json.dumps(tree))
        evidence = {
            "owner": {"kind": "session_bank", "ref": "alice/velis", "mayRestart": True},
            "ownerHome": "/home/alice",
            "sessionName": "velis",
            "unixUser": "alice",
            "pane": {
                "windowIndex": 0,
                "windowName": "agent",
                "windowLayout": "b25f,80x24,0,0",
                "paneIndex": 0,
                "paneId": "%1",
                "cwd": "/home/alice/project",
            },
            "paneStatus": {"dead": False, "currentCommand": "bash", "cwd": "/home/alice/project"},
            "process": tree[0] if tree else {},
            "processTree": tree,
        }
        desc = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")[0]["descriptors"][0]
        self.assertEqual(desc["mode"], "unresolved")

    def test_hermes_state_db_requires_unique_candidate_without_latest_guessing(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            db_dir = home / ".hermes" / "profiles" / "scout"
            db_dir.mkdir(parents=True)
            db_path = db_dir / "state.db"
            collector.initialize_test_state_db(
                db_path,
                [
                    ("hermes-old", "/home/alice/project", "2026-07-15T09:59:59Z"),
                    ("hermes-new", "/home/alice/project", "2026-07-15T10:00:01Z"),
                ],
            )
            proc = FakeProcReader(
                {
                    "42": ProcEntry(
                        ppid="1",
                        comm="python",
                        argv=[str(home / ".hermes" / "hermes-agent-current" / "venv" / "bin" / "python"), "-m", "hermes_cli.main", "--profile", "scout", "--tui", "--yolo"],
                        cwd="/home/alice/project",
                        started_at="2026-07-15T10:00:00Z",
                    )
                }
            )
            runner = collector.RecordingCommandRunner(
                {
                    ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "hermes-scout\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\tpython\t0\n"
                }
            )
            evidence = collector.collect_tmux_evidence(
                socket="/tmp/test.sock",
                owner={"kind": "persistent_agent", "ref": "persistent:alice/hermes-scout", "mayRestart": True},
                owner_home=str(home),
                session_filter="hermes-scout",
                command_runner=runner,
                proc_reader=proc,
            )[0]
            self.assertEqual(len(evidence["stateDbCandidates"]), 2)
            desc = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")[0]["descriptors"][0]
            self.assertEqual(desc["mode"], "unresolved")
            self.assertEqual(desc["unresolvedReason"], "ambiguous_candidates")

    def test_hermes_state_db_incomplete_bounded_scan_never_selects_single_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            db_dir = home / ".hermes" / "profiles" / "scout"
            db_dir.mkdir(parents=True)
            collector.initialize_test_state_db(
                db_dir / "state.db",
                [
                    ("hermes-match", "/home/alice/project", "2026-07-15T10:00:00Z"),
                    ("hermes-hidden", "/home/alice/project", "2026-07-15T10:00:01Z"),
                ],
            )
            proc = FakeProcReader(
                {
                    "42": ProcEntry(
                        ppid="1",
                        comm="python",
                        argv=[
                            str(home / ".hermes" / "hermes-agent-current" / "venv" / "bin" / "python"),
                            "-m",
                            "hermes_cli.main",
                            "--profile=scout",
                            "--tui",
                            "--yolo",
                        ],
                        cwd="/home/alice/project",
                        started_at="2026-07-15T10:00:00Z",
                    )
                }
            )
            runner = collector.RecordingCommandRunner(
                {
                    ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "hermes-scout\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\tpython\t0\n"
                }
            )
            evidence = collector.collect_tmux_evidence(
                socket="/tmp/test.sock",
                owner={"kind": "persistent_agent", "ref": "persistent:alice/hermes-scout", "mayRestart": True},
                owner_home=str(home),
                session_filter="hermes-scout",
                command_runner=runner,
                proc_reader=proc,
                state_db_scan_cap=1,
            )[0]
            self.assertTrue(evidence["stateDbIncomplete"])
            desc = snapshot.sessions_from_collected_evidence([evidence], unix_user="alice")[0]["descriptors"][0]
            self.assertEqual(desc["mode"], "unresolved")
            self.assertEqual(desc["unresolvedReason"], "missing_evidence")

    def test_collector_cli_requires_matching_owner_and_writes_json(self) -> None:
        proc = FakeProcReader(
            {
                "42": ProcEntry(ppid="1", comm="codex", argv=["codex", "resume", CODEX_ID], cwd="/home/alice/project", started_at="2026-07-15T10:00:00Z")
            }
        )
        runner = collector.RecordingCommandRunner(
            {
                ("tmux", "-S", "/tmp/test.sock", "list-panes", "-a", "-F", collector.TMUX_PANE_FORMAT): "velis\t0\tagent\tb25f,80x24,0,0\t0\t%1\t/home/alice/project\t42\tcodex\t0\n"
            }
        )
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "evidence.json"
            code = collector.main(
                [
                    "--socket",
                    "/tmp/test.sock",
                    "--unix-user",
                    "alice",
                    "--owner-home",
                    "/home/alice",
                    "--owner-kind",
                    "session_bank",
                    "--owner-ref",
                    "alice/velis",
                    "--owner-may-restart",
                    "--output",
                    str(out),
                ],
                command_runner=runner,
                proc_reader=proc,
                current_user="alice",
            )
            self.assertEqual(code, 0)
            self.assertEqual(json.loads(out.read_text(encoding="utf-8"))[0]["sessionName"], "velis")
        mismatch = collector.main(
            [
                "--socket",
                "/tmp/test.sock",
                "--unix-user",
                "alice",
                "--owner-home",
                "/home/alice",
                "--owner-kind",
                "session_bank",
                "--owner-ref",
                "alice/velis",
            ],
            command_runner=runner,
            proc_reader=proc,
            current_user="bob",
        )
        self.assertNotEqual(mismatch, 0)


class InstallScriptTest(unittest.TestCase):
    def test_install_script_honors_prefix_and_installs_modes_without_runtime_mutation(self) -> None:
        script = SCRIPT_DIR / "install.sh"
        with tempfile.TemporaryDirectory() as tmp:
            subprocess.run(["bash", str(script), "--prefix", tmp], check=True, text=True, capture_output=True)
            prefix = Path(tmp)
            snapshot_bin = prefix / "bin" / "chrote-tmux-recovery-snapshot"
            module = prefix / "lib" / "chrote" / "tmux-recovery" / "manifest.py"
            readme = prefix / "share" / "doc" / "chrote" / "tmux-recovery" / "README.md"
            schema = prefix / "share" / "doc" / "chrote" / "tmux-recovery" / "recovery-manifest.schema.json"
            self.assertTrue(snapshot_bin.exists())
            self.assertTrue(module.exists())
            self.assertTrue(readme.exists())
            self.assertTrue(schema.exists())
            self.assertEqual(stat.S_IMODE(snapshot_bin.stat().st_mode), 0o755)
            self.assertEqual(stat.S_IMODE(module.stat().st_mode), 0o644)
            self.assertEqual(stat.S_IMODE(readme.stat().st_mode), 0o644)
            self.assertEqual(stat.S_IMODE(schema.stat().st_mode), 0o644)
            for wrapper in [
                "chrote-tmux-recovery-snapshot",
                "chrote-tmux-recovery-restore",
                "chrote-tmux-recovery-verify",
                "chrote-tmux-recovery-collector",
            ]:
                subprocess.run([str(prefix / "bin" / wrapper), "--help"], check=True, text=True, capture_output=True)
        text = script.read_text(encoding="utf-8")
        forbidden = ["systemctl", "sudo", "~/.local"]
        for token in forbidden:
            self.assertNotIn(token, text)
        self.assertNotRegex(text, r"/home/[^/]+")


class ProcEntry:
    def __init__(self, *, ppid: str, comm: str, argv: list[str], cwd: str, started_at: str, start_ticks: str = "100") -> None:
        self.ppid = ppid
        self.comm = comm
        self.argv = argv
        self.cwd = cwd
        self.started_at = started_at
        self.start_ticks = start_ticks


class FakeProcReader:
    def __init__(self, entries: dict[str, ProcEntry]) -> None:
        self.entries = entries
        self.read_paths: list[str] = []

    def collect(self, pane_pid: str, owner_home: str) -> list[dict]:
        return collector.collect_process_tree(pane_pid, owner_home, proc_reader=self)

    def list_pids(self) -> list[str]:
        return sorted(self.entries)

    def stat_info(self, pid: str) -> dict:
        self.read_paths.append(f"/proc/{pid}/stat")
        entry = self.entries[pid]
        return {
            "pid": pid,
            "ppid": entry.ppid,
            "comm": entry.comm,
            "startedAt": entry.started_at,
            "startTicks": entry.start_ticks,
        }

    def process_info(self, pid: str) -> dict:
        self.read_paths.extend([f"/proc/{pid}/cmdline", f"/proc/{pid}/cwd"])
        entry = self.entries[pid]
        return {
            "pid": pid,
            "ppid": entry.ppid,
            "comm": entry.comm,
            "argv": entry.argv,
            "cwd": entry.cwd,
            "startedAt": entry.started_at,
            "startTicks": entry.start_ticks,
        }


class ReusingProcReader(FakeProcReader):
    def __init__(self, *, stable: ProcEntry, reused: ProcEntry) -> None:
        super().__init__({"42": stable})
        self.stable = stable
        self.reused = reused
        self.stat_calls = 0

    def stat_info(self, pid: str) -> dict:
        self.read_paths.append(f"/proc/{pid}/stat")
        self.stat_calls += 1
        entry = self.stable if self.stat_calls == 1 else self.reused
        return {
            "pid": pid,
            "ppid": entry.ppid,
            "comm": entry.comm,
            "startedAt": entry.started_at,
            "startTicks": entry.start_ticks,
        }

    def process_info(self, pid: str) -> dict:
        self.read_paths.extend([f"/proc/{pid}/cmdline", f"/proc/{pid}/cwd"])
        return {
            "pid": pid,
            "ppid": self.reused.ppid,
            "comm": self.reused.comm,
            "argv": self.reused.argv,
            "cwd": self.reused.cwd,
            "startedAt": self.reused.started_at,
            "startTicks": self.reused.start_ticks,
        }


class ReusingAfterReadProcReader(ReusingProcReader):
    def stat_info(self, pid: str) -> dict:
        self.read_paths.append(f"/proc/{pid}/stat")
        self.stat_calls += 1
        entry = self.stable if self.stat_calls <= 2 else self.reused
        return {
            "pid": pid,
            "ppid": entry.ppid,
            "comm": entry.comm,
            "startedAt": entry.started_at,
            "startTicks": entry.start_ticks,
        }


def write_json_temp(value: object) -> Path:
    handle = tempfile.NamedTemporaryFile("w", encoding="utf-8", delete=False)
    with handle:
        json.dump(value, handle)
    return Path(handle.name)


def python_http_collected_evidence(bind: str = "127.0.0.1") -> dict:
    return {
        "owner": {"kind": "session_bank", "ref": "alice/static-site", "mayRestart": True},
        "ownerHome": "/home/alice",
        "sessionName": "static-site",
        "unixUser": "alice",
        "pane": {
            "windowIndex": 1,
            "windowName": "server",
            "windowLayout": "7f91,80x24,0,0",
            "paneIndex": 0,
            "paneId": "%3",
            "cwd": "/home/alice/project/public",
        },
        "paneStatus": {"dead": False, "currentCommand": "python3", "cwd": "/home/alice/project/public"},
        "process": {
            "pid": "42",
            "ppid": "1",
            "comm": "python3",
            "argv": ["python3", "-m", "http.server", "8088", "--bind", bind, "--directory", "."],
            "cwd": "/home/alice/project/public",
            "startedAt": "2026-07-15T10:00:00Z",
        },
        "processTree": [
            {
                "pid": "42",
                "ppid": "1",
                "comm": "python3",
                "argv": ["python3", "-m", "http.server", "8088", "--bind", bind, "--directory", "."],
                "cwd": "/home/alice/project/public",
                "startedAt": "2026-07-15T10:00:00Z",
            }
        ],
    }


if __name__ == "__main__":
    unittest.main()
