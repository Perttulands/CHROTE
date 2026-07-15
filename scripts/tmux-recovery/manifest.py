#!/usr/bin/env python3
"""Manifest helpers for CHROTE tmux recovery operator tools."""

from __future__ import annotations

from copy import deepcopy
from dataclasses import dataclass
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
from typing import Any
from urllib.parse import urlparse


SCHEMA_VERSION = "chrote.tmux-recovery.manifest.v1"
TIMESTAMP_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$")
UUID_RE = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
NATIVE_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
PROFILE_RE = re.compile(r"^[a-z][a-z0-9_-]{0,63}$")
UNIT_RE = re.compile(r"^[A-Za-z0-9_.@:-]+\.service$")
OWNER_REF_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,239}$")
SAFE_SESSION_NAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._@+-]{0,239}$")
SAFE_TMUX_ID_RE = re.compile(r"^[A-Za-z0-9%$._:-]{0,80}$")
SAFE_STATES = {"active", "inactive", "failed", "activating", "deactivating", "reloading", "maintenance"}
LOOPBACK_HOSTS = {"127.0.0.1", "localhost", "::1"}
UNRESOLVED_REASONS = {
    "unknown_process",
    "ambiguous_candidates",
    "unsafe_evidence",
    "unsupported_workload",
    "missing_evidence",
    "conflicting_owners",
    "conflicting_evidence",
}
MAX_DOCUMENT_CHARS = 512 * 1024
MAX_SESSIONS = 256
MAX_DESCRIPTORS = 256
MAX_STRING_CHARS = 8192

SESSION_KNOWN_KEYS = {
    "sessionName",
    "unixUser",
    "owner",
    "managerKind",
    "managerRef",
    "restartAllowed",
    "statusProbe",
    "descriptors",
    "verification",
    "allowExtraPanes",
}
DESCRIPTOR_KNOWN_KEYS = {
    "mode",
    "owner",
    "topology",
    "workloadKind",
    "agent",
    "command",
    "evidenceSource",
    "confidence",
    "unresolvedReason",
}
TOP_LEVEL_KNOWN_KEYS = {"schemaVersion", "createdAt", "source", "sessions"}
SOURCE_KNOWN_KEYS = {"apiUrl"}
OWNER_KNOWN_KEYS = {"kind", "ref", "mayRestart"}
TOPOLOGY_KNOWN_KEYS = {"sessionName", "sessionId", "windowIndex", "windowName", "windowLayout", "paneIndex", "paneId", "paneCurrentPath"}
AGENT_KNOWN_KEYS = {"kind", "nativeSessionId", "hermesProfile"}
COMMAND_KNOWN_KEYS = {"kind", "pythonHTTPServer"}
PYTHON_HTTP_SERVER_KNOWN_KEYS = {"bind", "port", "directory"}
PANE_VERIFICATION_KNOWN_KEYS = {"target", "paneStatus", "helperEndpoint"}
PANE_TARGET_KNOWN_KEYS = {"windowIndex", "paneIndex", "paneId"}
PANE_STATUS_KNOWN_KEYS = {"dead", "currentCommand", "cwd"}
HELPER_ENDPOINT_KNOWN_KEYS = {"kind", "url", "expectStatus", "timeoutSeconds"}
STATUS_PROBE_KNOWN_KEYS = {"kind", "unit", "expectActiveState"}


class ManifestError(ValueError):
    pass


@dataclass
class ManifestPath:
    path: Path


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).strftime("%Y-%m-%dT%H:%M:%SZ")


def new_manifest(sessions: list[dict[str, Any]], now: str | None = None, source: dict[str, Any] | None = None) -> dict[str, Any]:
    normalized_sessions = deepcopy(sessions)
    for session in normalized_sessions:
        if isinstance(session, dict):
            _normalize_managed_defaults(session)
    doc: dict[str, Any] = {
        "schemaVersion": SCHEMA_VERSION,
        "createdAt": now or utc_now(),
        "sessions": normalized_sessions,
    }
    if source:
        doc["source"] = deepcopy(source)
    validate_manifest(doc)
    return doc


def load_manifest(path: str | Path) -> dict[str, Any]:
    raw = Path(path).read_text(encoding="utf-8")
    doc = json.loads(raw)
    validate_manifest(doc)
    return doc


def write_timestamped_manifest(doc: dict[str, Any], output_dir: str | Path, now: str | None = None) -> Path:
    validate_manifest(doc)
    output = Path(output_dir)
    output.mkdir(parents=True, exist_ok=True)
    stamp = _timestamp_slug(now or doc.get("createdAt") or utc_now())
    path = output / f"chrote-tmux-recovery-{stamp}.json"
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(doc, handle, indent=2, sort_keys=True)
            handle.write("\n")
    except Exception:
        try:
            path.unlink()
        finally:
            raise
    return path


def merge_preserving_extras(current: dict[str, Any], baseline: dict[str, Any] | None) -> dict[str, Any]:
    validate_manifest(current)
    if baseline is not None:
        validate_manifest(baseline)
    merged = deepcopy(current)
    if baseline is None:
        return merged
    current_keys = {_session_key(session) for session in merged.get("sessions", []) if isinstance(session, dict)}
    for session in baseline.get("sessions", []):
        if not isinstance(session, dict):
            continue
        if _session_key(session) not in current_keys:
            merged["sessions"].append(deepcopy(session))
    validate_manifest(merged)
    return merged


def is_session_bank_session(session: dict[str, Any]) -> bool:
    descriptors = session.get("descriptors")
    if not isinstance(descriptors, list) or not descriptors:
        return False
    for desc in descriptors:
        if not isinstance(desc, dict):
            return False
        owner = desc.get("owner") if isinstance(desc.get("owner"), dict) else {}
        if owner.get("kind") != "session_bank":
            return False
    return True


def is_managed_session(session: dict[str, Any]) -> bool:
    owner = session.get("owner") if isinstance(session.get("owner"), dict) else {}
    if owner.get("kind") == "external_manager":
        return True
    return str(session.get("managerKind", "")).strip() != ""


def validate_manifest(doc: dict[str, Any]) -> None:
    if not isinstance(doc, dict):
        raise ManifestError("manifest must be an object")
    _validate_bounds(doc)
    _reject_unknown_keys(doc, TOP_LEVEL_KNOWN_KEYS, "$")
    if doc.get("schemaVersion") != SCHEMA_VERSION:
        raise ManifestError(f"schemaVersion must be {SCHEMA_VERSION}")
    created = doc.get("createdAt")
    if not isinstance(created, str) or not TIMESTAMP_RE.match(created):
        raise ManifestError("createdAt must be an RFC3339 UTC second timestamp")
    if "source" in doc:
        _validate_source(doc["source"], "$.source")
    sessions = doc.get("sessions")
    if not isinstance(sessions, list):
        raise ManifestError("sessions must be a list")
    if len(sessions) > MAX_SESSIONS:
        raise ManifestError("too many sessions")
    seen_sessions: set[tuple[str, str]] = set()
    for index, session in enumerate(sessions):
        if isinstance(session, dict):
            key = _session_key(session)
            if key in seen_sessions:
                raise ManifestError(f"duplicate session key {key}")
            seen_sessions.add(key)
        _validate_session(session, index)


def _validate_session(session: Any, index: int) -> None:
    if not isinstance(session, dict):
        raise ManifestError(f"sessions[{index}] must be an object")
    _reject_unknown_keys(session, SESSION_KNOWN_KEYS, f"sessions[{index}]")
    if not str(session.get("sessionName", "")).strip():
        raise ManifestError(f"sessions[{index}].sessionName is required")
    if "allowExtraPanes" in session and not isinstance(session["allowExtraPanes"], bool):
        raise ManifestError(f"sessions[{index}].allowExtraPanes must be boolean")
    session_owner = session.get("owner") if isinstance(session.get("owner"), dict) else None
    if session_owner is not None:
        _validate_owner(session_owner, f"sessions[{index}].owner")
    if is_managed_session(session):
        if session.get("managerKind") != "systemd-user":
            raise ManifestError(f"sessions[{index}].managerKind must be systemd-user")
        manager_ref = str(session.get("managerRef", "")).strip()
        if not _valid_unit(manager_ref):
            raise ManifestError(f"sessions[{index}].managerRef is invalid")
        if session.get("restartAllowed", False) is not False:
            raise ManifestError(f"sessions[{index}].restartAllowed must be false for managed records")
        _validate_status_probe(session.get("statusProbe"), f"sessions[{index}].statusProbe", manager_ref)
        if session_owner is not None and (
            session_owner.get("kind") != "external_manager" or session_owner.get("mayRestart") is not False
        ):
            raise ManifestError(f"sessions[{index}].owner conflicts with managed session")
    descriptors = session.get("descriptors", [])
    if not isinstance(descriptors, list):
        raise ManifestError(f"sessions[{index}].descriptors must be a list")
    if len(descriptors) > MAX_DESCRIPTORS:
        raise ManifestError(f"sessions[{index}] has too many descriptors")
    seen_targets: set[tuple[Any, ...]] = set()
    seen_pane_ids: set[str] = set()
    for desc_index, desc in enumerate(descriptors):
        if isinstance(desc, dict):
            target_key = _pane_target_key(session, desc)
            if target_key in seen_targets:
                raise ManifestError(f"sessions[{index}] has duplicate pane target {target_key}")
            seen_targets.add(target_key)
            pane_id = _pane_id(desc)
            if pane_id:
                if pane_id in seen_pane_ids:
                    raise ManifestError(f"sessions[{index}] has duplicate pane id {pane_id}")
                seen_pane_ids.add(pane_id)
            if session_owner is not None:
                desc_owner = desc.get("owner") if isinstance(desc.get("owner"), dict) else {}
                if session_owner != desc_owner:
                    raise ManifestError(f"sessions[{index}].descriptors[{desc_index}] owner conflicts with session owner")
        _validate_descriptor(desc, index, desc_index)
    verification = session.get("verification", [])
    if not isinstance(verification, list):
        raise ManifestError(f"sessions[{index}].verification must be a list")
    if len(verification) > MAX_DESCRIPTORS:
        raise ManifestError(f"sessions[{index}] has too many verification records")
    descriptor_keys = {_descriptor_verification_key(desc) for desc in descriptors if isinstance(desc, dict) and desc.get("mode") != "managed"}
    seen_verification: set[tuple[Any, ...]] = set()
    for verify_index, record in enumerate(verification):
        key = _validate_pane_verification(record, index, verify_index)
        if key in seen_verification:
            raise ManifestError(f"sessions[{index}] has duplicate verification target {key}")
        seen_verification.add(key)
        if descriptor_keys and key not in descriptor_keys:
            raise ManifestError(f"sessions[{index}].verification[{verify_index}] target has no descriptor")


def _normalize_managed_defaults(session: dict[str, Any]) -> None:
    if not is_managed_session(session):
        return
    session.setdefault("restartAllowed", False)
    if session.get("managerKind") == "systemd-user" and str(session.get("managerRef", "")).strip():
        session.setdefault(
            "statusProbe",
            {
                "kind": "systemd-user",
                "unit": str(session["managerRef"]).strip(),
                "expectActiveState": "active",
            },
        )


def _validate_descriptor(desc: Any, session_index: int, desc_index: int) -> None:
    if not isinstance(desc, dict):
        raise ManifestError(f"sessions[{session_index}].descriptors[{desc_index}] must be an object")
    path = f"sessions[{session_index}].descriptors[{desc_index}]"
    _reject_unknown_keys(desc, DESCRIPTOR_KNOWN_KEYS, path)
    mode = desc.get("mode")
    if mode not in {"topology", "agent", "command", "managed", "unresolved"}:
        raise ManifestError(f"{path}.mode is unsupported")
    owner = desc.get("owner")
    _validate_owner(owner, f"{path}.owner")
    _validate_topology(desc.get("topology"), f"{path}.topology")
    if mode == "managed" and (owner.get("kind") != "external_manager" or owner.get("mayRestart") is not False):
        raise ManifestError(f"{path} managed owner must be non-restarting external_manager")
    if mode == "agent" and (owner.get("kind") not in {"session_bank", "persistent_agent"} or owner.get("mayRestart") is not True):
        raise ManifestError(f"{path} agent owner must be restartable")
    if mode in {"command", "topology"} and (owner.get("kind") != "session_bank" or owner.get("mayRestart") is not True):
        raise ManifestError(f"{path} {mode} owner must be restartable session_bank")
    if mode == "unresolved" and owner.get("mayRestart") is True:
        raise ManifestError(f"{path} unresolved owner cannot restart")
    if mode == "agent":
        _forbid_descriptor_keys(desc, {"command", "unresolvedReason"}, path, mode)
        agent_kind = _validate_agent(desc.get("agent"), f"{path}.agent")
        _validate_workload_kind(desc, agent_kind, path)
    elif mode == "command":
        _forbid_descriptor_keys(desc, {"agent", "unresolvedReason"}, path, mode)
        command_kind = _validate_command(desc.get("command"), f"{path}.command")
        _validate_workload_kind(desc, command_kind, path)
    elif mode == "topology":
        _forbid_descriptor_keys(desc, {"agent", "command", "unresolvedReason"}, path, mode)
        _validate_workload_kind(desc, "shell", path)
    elif mode == "managed":
        _forbid_descriptor_keys(desc, {"agent", "command", "unresolvedReason"}, path, mode)
        _validate_workload_kind(desc, "managed", path)
    elif mode == "unresolved":
        _forbid_descriptor_keys(desc, {"agent", "command"}, path, mode)
        _validate_workload_kind(desc, "unknown", path)
        reason = desc.get("unresolvedReason")
        if reason not in UNRESOLVED_REASONS:
            raise ManifestError(f"{path}.unresolvedReason is invalid")


def _validate_workload_kind(desc: dict[str, Any], expected: str, path: str) -> None:
    if desc.get("workloadKind") != expected:
        raise ManifestError(f"{path}.workloadKind must be {expected}")


def _forbid_descriptor_keys(desc: dict[str, Any], keys: set[str], path: str, mode: str) -> None:
    for key in sorted(keys):
        if key in desc:
            raise ManifestError(f"{path}.{key} is forbidden for {mode} descriptors")


def _reject_unknown_keys(value: dict[str, Any], allowed: set[str], path: str) -> None:
    unknown_keys = sorted(key for key in value if key not in allowed)
    if unknown_keys:
        raise ManifestError(f"{path}.{unknown_keys[0]} is not an allowed key")


def _validate_source(source: Any, path: str) -> None:
    if not isinstance(source, dict):
        raise ManifestError(f"{path} must be an object")
    _reject_unknown_keys(source, SOURCE_KNOWN_KEYS, path)
    if "apiUrl" in source:
        _validate_control_free_string(source["apiUrl"], f"{path}.apiUrl", MAX_STRING_CHARS)
        try:
            from client import normalize_api_base_url

            normalized = normalize_api_base_url(source["apiUrl"])
        except Exception as exc:
            raise ManifestError(f"{path}.apiUrl is invalid") from exc
        if source["apiUrl"] != normalized:
            raise ManifestError(f"{path}.apiUrl must be canonical api origin")


def _validate_bounds(value: Any) -> None:
    try:
        encoded = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    except (TypeError, ValueError) as exc:
        raise ManifestError("manifest must be JSON serializable") from exc
    if len(encoded) > MAX_DOCUMENT_CHARS:
        raise ManifestError("manifest document is too long")
    _validate_value_bounds(value, "$")


def _validate_value_bounds(value: Any, path: str) -> None:
    if isinstance(value, str):
        if len(value) > MAX_STRING_CHARS:
            raise ManifestError(f"{path} is too long")
        if "\x00" in value:
            raise ManifestError(f"{path} contains invalid NUL")
        return
    if isinstance(value, dict):
        for key, child in value.items():
            if not isinstance(key, str):
                raise ManifestError(f"{path} has a non-string key")
            if len(key) > 128:
                raise ManifestError(f"{path} key is too long")
            _validate_value_bounds(child, f"{path}.{key}")
        return
    if isinstance(value, list):
        for index, child in enumerate(value):
            _validate_value_bounds(child, f"{path}[{index}]")


def _validate_owner(owner: Any, path: str) -> None:
    if not isinstance(owner, dict):
        raise ManifestError(f"{path} is required")
    _reject_unknown_keys(owner, OWNER_KNOWN_KEYS, path)
    if owner.get("kind") not in {"session_bank", "persistent_agent", "external_manager"}:
        raise ManifestError(f"{path}.kind is invalid")
    if not isinstance(owner.get("ref"), str) or not OWNER_REF_RE.match(owner["ref"].strip()):
        raise ManifestError(f"{path}.ref is malformed")
    if not isinstance(owner.get("mayRestart"), bool):
        raise ManifestError(f"{path}.mayRestart must be boolean")


def _validate_topology(topology: Any, path: str) -> None:
    if not isinstance(topology, dict):
        raise ManifestError(f"{path} is required")
    _reject_unknown_keys(topology, TOPOLOGY_KNOWN_KEYS, path)
    for key in ("windowIndex", "paneIndex"):
        if not isinstance(topology.get(key), int) or topology.get(key) < 0:
            raise ManifestError(f"{path}.{key} must be a non-negative integer")
    session_name = topology.get("sessionName")
    if session_name not in (None, ""):
        _validate_safe_name(session_name, f"{path}.sessionName", SAFE_SESSION_NAME_RE, 240)
    session_id = topology.get("sessionId")
    if session_id not in (None, ""):
        _validate_safe_name(session_id, f"{path}.sessionId", SAFE_TMUX_ID_RE, 80)
    window_name = topology.get("windowName")
    if window_name not in (None, ""):
        _validate_control_free_string(window_name, f"{path}.windowName", 240)
    layout = topology.get("windowLayout")
    if layout not in (None, ""):
        _validate_control_free_string(layout, f"{path}.windowLayout", 4096)
    pane_id = topology.get("paneId")
    if pane_id not in (None, ""):
        _validate_safe_name(pane_id, f"{path}.paneId", SAFE_TMUX_ID_RE, 80)
    pane_current_path = topology.get("paneCurrentPath")
    if pane_current_path not in (None, ""):
        _validate_abs_path(pane_current_path, f"{path}.paneCurrentPath")


def _validate_agent(agent: Any, path: str) -> str:
    if not isinstance(agent, dict):
        raise ManifestError(f"{path} is required")
    _reject_unknown_keys(agent, AGENT_KNOWN_KEYS, path)
    kind = agent.get("kind")
    session_id = str(agent.get("nativeSessionId", "")).strip()
    if kind in {"codex", "claude"}:
        if not UUID_RE.match(session_id):
            raise ManifestError(f"{path}.nativeSessionId must be a uuid")
        return kind
    if kind == "hermes":
        if not NATIVE_ID_RE.match(session_id):
            raise ManifestError(f"{path}.nativeSessionId is malformed")
        profile = str(agent.get("hermesProfile", "")).strip()
        if not PROFILE_RE.match(profile):
            raise ManifestError(f"{path}.hermesProfile is malformed")
        return kind
    raise ManifestError(f"{path}.kind is invalid")


def _validate_command(command: Any, path: str) -> str:
    if not isinstance(command, dict) or command.get("kind") != "python-http-server":
        raise ManifestError(f"{path} must be typed python-http-server")
    _reject_unknown_keys(command, COMMAND_KNOWN_KEYS, path)
    server = command.get("pythonHTTPServer")
    if not isinstance(server, dict):
        raise ManifestError(f"{path}.pythonHTTPServer is required")
    _reject_unknown_keys(server, PYTHON_HTTP_SERVER_KNOWN_KEYS, f"{path}.pythonHTTPServer")
    if server.get("bind") not in LOOPBACK_HOSTS:
        raise ManifestError(f"{path}.pythonHTTPServer bind must be loopback")
    port = server.get("port")
    if not isinstance(port, int) or port < 1 or port > 65535:
        raise ManifestError(f"{path}.pythonHTTPServer port is invalid")
    directory = server.get("directory")
    if not isinstance(directory, str) or not directory.startswith("/"):
        raise ManifestError(f"{path}.pythonHTTPServer directory must be absolute")
    _validate_abs_path(directory, f"{path}.pythonHTTPServer directory")
    return "python-http-server"


def _validate_helper_endpoint(probe: Any, path: str) -> None:
    if not isinstance(probe, dict) or probe.get("kind") != "http-get":
        raise ManifestError(f"{path} must be typed http-get")
    _reject_unknown_keys(probe, HELPER_ENDPOINT_KNOWN_KEYS, path)
    parsed = urlparse(str(probe.get("url", "")).strip())
    if parsed.scheme != "http" or parsed.hostname not in LOOPBACK_HOSTS:
        raise ManifestError(f"{path}.url must be loopback http")
    if "expectStatus" in probe and not isinstance(probe["expectStatus"], int):
        raise ManifestError(f"{path}.expectStatus must be integer")
    if "expectStatus" in probe and (probe["expectStatus"] < 100 or probe["expectStatus"] > 599):
        raise ManifestError(f"{path}.expectStatus is invalid")
    if "timeoutSeconds" in probe and (
        not isinstance(probe["timeoutSeconds"], (int, float)) or probe["timeoutSeconds"] <= 0 or probe["timeoutSeconds"] > 30
    ):
        raise ManifestError(f"{path}.timeoutSeconds is invalid")


def _validate_pane_status(status: Any, path: str) -> None:
    if not isinstance(status, dict):
        raise ManifestError(f"{path} must be an object")
    _reject_unknown_keys(status, PANE_STATUS_KNOWN_KEYS, path)
    for key in ("dead", "currentCommand", "cwd"):
        if key not in status:
            raise ManifestError(f"{path}.{key} is required")
    if "dead" in status and not isinstance(status["dead"], bool):
        raise ManifestError(f"{path}.dead must be boolean")
    if "currentCommand" in status and not isinstance(status["currentCommand"], str):
        raise ManifestError(f"{path}.currentCommand must be string")
    if "cwd" in status and not isinstance(status["cwd"], str):
        raise ManifestError(f"{path}.cwd must be string")
    if isinstance(status.get("cwd"), str) and status["cwd"]:
        _validate_abs_path(status["cwd"], f"{path}.cwd")


def _validate_pane_verification(record: Any, session_index: int, verify_index: int) -> tuple[Any, ...]:
    path = f"sessions[{session_index}].verification[{verify_index}]"
    if not isinstance(record, dict):
        raise ManifestError(f"{path} must be an object")
    _reject_unknown_keys(record, PANE_VERIFICATION_KNOWN_KEYS, path)
    key = _validate_pane_target(record.get("target"), f"{path}.target")
    _validate_pane_status(record.get("paneStatus"), f"{path}.paneStatus")
    if "helperEndpoint" in record:
        _validate_helper_endpoint(record["helperEndpoint"], f"{path}.helperEndpoint")
    return key


def _validate_pane_target(target: Any, path: str) -> tuple[Any, ...]:
    if not isinstance(target, dict):
        raise ManifestError(f"{path} is required")
    _reject_unknown_keys(target, PANE_TARGET_KNOWN_KEYS, path)
    for key in ("windowIndex", "paneIndex"):
        if not isinstance(target.get(key), int) or target.get(key) < 0:
            raise ManifestError(f"{path}.{key} must be a non-negative integer")
    pane_id = target.get("paneId")
    if not isinstance(pane_id, str) or not pane_id.strip():
        raise ManifestError(f"{path}.paneId is required")
    _validate_safe_name(pane_id, f"{path}.paneId", SAFE_TMUX_ID_RE, 80)
    return (target.get("windowIndex"), target.get("paneIndex"), pane_id)


def _validate_status_probe(probe: Any, path: str, manager_ref: str) -> None:
    if not isinstance(probe, dict):
        raise ManifestError(f"{path} must be a systemd-user probe")
    _reject_unknown_keys(probe, STATUS_PROBE_KNOWN_KEYS, path)
    if probe.get("kind") != "systemd-user" or not _valid_unit(str(probe.get("unit", "")).strip()):
        raise ManifestError(f"{path} must be a systemd-user probe")
    if str(probe.get("unit", "")).strip() != manager_ref:
        raise ManifestError(f"{path} unit must match managerRef")
    expect_state = str(probe.get("expectActiveState", "active")).strip()
    if expect_state not in SAFE_STATES:
        raise ManifestError(f"{path} expectActiveState is invalid")


def _validate_control_free_string(value: Any, path: str, max_length: int) -> None:
    if not isinstance(value, str):
        raise ManifestError(f"{path} must be string")
    if len(value) > max_length:
        raise ManifestError(f"{path} is too long")
    if any(char in value for char in "\x00\n\r"):
        raise ManifestError(f"{path} contains control characters")


def _validate_safe_name(value: Any, path: str, pattern: re.Pattern[str], max_length: int) -> None:
    _validate_control_free_string(value, path, max_length)
    if not pattern.match(value):
        raise ManifestError(f"{path} is malformed")


def _validate_abs_path(value: str, path: str) -> None:
    _validate_control_free_string(value, path, MAX_STRING_CHARS)
    if "#" in value:
        raise ManifestError(f"{path} contains unsafe characters")
    if not os.path.isabs(value):
        raise ManifestError(f"{path} must be absolute")


def _valid_unit(value: str) -> bool:
    return bool(UNIT_RE.match(value))


def _timestamp_slug(value: str) -> str:
    text = str(value).strip()
    if not TIMESTAMP_RE.match(text):
        raise ManifestError("timestamp must be an RFC3339 UTC second timestamp")
    return text.replace("-", "").replace(":", "").replace("Z", "Z")


def _session_key(session: dict[str, Any]) -> tuple[str, str]:
    return (str(session.get("unixUser", "")).strip(), str(session.get("sessionName", "")).strip())


def _pane_target_key(session: dict[str, Any], desc: dict[str, Any]) -> tuple[Any, ...]:
    topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
    return (
        topology.get("sessionName") or session.get("sessionName"),
        topology.get("windowIndex"),
        topology.get("paneIndex"),
    )


def _pane_id(desc: dict[str, Any]) -> str:
    topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
    return str(topology.get("paneId", "")).strip()


def _descriptor_verification_key(desc: dict[str, Any]) -> tuple[Any, ...]:
    topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
    return (topology.get("windowIndex"), topology.get("paneIndex"), str(topology.get("paneId", "")).strip())
