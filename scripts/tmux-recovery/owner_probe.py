#!/usr/bin/env python3
"""Typed owner-probe primitives for tmux workload recovery.

This module classifies already-collected pane evidence. It does not snapshot,
restore, read live tmux, or persist raw argv/environment data.
"""

from __future__ import annotations

from datetime import datetime, timezone
from pathlib import Path
import os
import re
from typing import Any


UUID_RE = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
NATIVE_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
PROFILE_RE = re.compile(r"^[a-z][a-z0-9_-]{0,63}$")
LOOPBACK_BINDS = {"127.0.0.1", "localhost", "::1"}
SHELLS = {"sh", "bash", "zsh", "fish", "pwsh", "powershell"}


def classify_pane(evidence: dict[str, Any]) -> dict[str, Any]:
    owner_home = _clean_abs_path(evidence.get("ownerHome", ""))
    owner = _owner(evidence)
    topology = _topology(evidence)
    process_tree = evidence.get("processTree")
    if isinstance(process_tree, list) and process_tree:
        return _attach_pane_status(
            _classify_process_tree(evidence, process_tree, owner, topology, owner_home),
            evidence,
        )
    process = evidence.get("process") if isinstance(evidence.get("process"), dict) else {}
    return _attach_pane_status(
        _classify_single_process(evidence, process, owner, topology, owner_home, allow_transcript=True),
        evidence,
    )


def _classify_process_tree(
    evidence: dict[str, Any],
    process_tree: list[Any],
    owner: dict[str, Any],
    topology: dict[str, Any],
    owner_home: str,
) -> dict[str, Any]:
    identities: dict[tuple[Any, ...], dict[str, Any]] = {}
    fallbacks: list[dict[str, Any]] = []
    for process in process_tree:
        if not isinstance(process, dict):
            continue
        child_evidence = dict(evidence)
        child_evidence["process"] = process
        desc = _classify_single_process(child_evidence, process, owner, topology, owner_home, allow_transcript=False)
        key = _identity_key(desc)
        if key is not None:
            identities[key] = desc
        else:
            fallbacks.append(desc)
    if len(identities) == 1:
        return next(iter(identities.values()))
    if len(identities) > 1:
        return _unresolved(owner, topology, "conflicting_evidence", "process", "low")
    for desc in fallbacks:
        if desc.get("mode") == "unresolved" and desc.get("unresolvedReason") != "unknown_process":
            return desc
    for desc in fallbacks:
        if desc.get("mode") == "topology":
            return desc
    if fallbacks:
        return fallbacks[0]
    return _unresolved(owner, topology, "unknown_process", "process", "low")


def _classify_single_process(
    evidence: dict[str, Any],
    process: dict[str, Any],
    owner: dict[str, Any],
    topology: dict[str, Any],
    owner_home: str,
    *,
    allow_transcript: bool,
) -> dict[str, Any]:
    if process.get("unsafeEvidence") == "pid_reused":
        return _unresolved(owner, topology, "unsafe_evidence", "process", "low")
    raw_argv = _argv(process)
    argv = _canonical_safe_argv(raw_argv, owner_home) or []
    comm = _process_name(process, raw_argv)
    if raw_argv and not argv and _looks_like_known_unsafe_argv(raw_argv, owner_home):
        return _unresolved(owner, topology, "unsafe_evidence", "argv", "low")

    transcript_agent = _agent_from_transcript(evidence.get("openTranscript")) if allow_transcript else None
    argv_agent = _agent_from_codex_claude_argv(argv)
    if transcript_agent is not None:
        live_kind = _live_workload_kind(argv, comm, owner_home)
        if _transcript_conflicts(transcript_agent, argv_agent, live_kind):
            return _unresolved(owner, topology, "conflicting_evidence", "transcript", "low")
        return _agent_descriptor(owner, topology, transcript_agent, "transcript", "high")
    if argv_agent is not None:
        return _agent_descriptor(owner, topology, argv_agent, "argv", "high")

    if _looks_like_hermes_argv(argv, owner_home):
        hermes = _hermes_from_process(evidence, argv, owner_home, topology)
        if hermes is not None:
            return hermes

    if _looks_like_python_http_server(argv):
        python = _python_http_server_descriptor(evidence, argv, owner, topology, owner_home)
        if python is not None:
            return python
        return _unresolved(owner, topology, "unsafe_evidence", "argv", "low")

    if comm in SHELLS:
        return _descriptor(owner, topology, "topology", "shell", "process", "medium")

    return _unresolved(owner, topology, "unknown_process", "process", "low")


def _identity_key(desc: dict[str, Any]) -> tuple[Any, ...] | None:
    mode = desc.get("mode")
    if mode == "agent":
        agent = desc.get("agent") if isinstance(desc.get("agent"), dict) else {}
        return (
            "agent",
            agent.get("kind"),
            agent.get("nativeSessionId"),
            agent.get("hermesProfile", ""),
        )
    if mode == "command":
        command = desc.get("command") if isinstance(desc.get("command"), dict) else {}
        python = command.get("pythonHTTPServer") if isinstance(command.get("pythonHTTPServer"), dict) else {}
        return ("command", command.get("kind"), python.get("bind"), python.get("port"), python.get("directory"))
    return None


def _attach_pane_status(desc: dict[str, Any], evidence: dict[str, Any]) -> dict[str, Any]:
    status = evidence.get("paneStatus")
    if not isinstance(status, dict):
        return desc
    cwd = _clean_abs_path(status.get("cwd") or evidence.get("pane", {}).get("cwd", ""))
    desc["paneStatus"] = {
        "dead": bool(status.get("dead", False)),
        "currentCommand": str(status.get("currentCommand", "")).strip(),
        "cwd": cwd,
    }
    return desc


def _agent_from_codex_claude_argv(argv: list[str]) -> dict[str, str] | None:
    if not argv:
        return None
    name = _basename(argv[0])
    if name == "codex":
        for i, token in enumerate(argv[:-1]):
            if token == "resume" and UUID_RE.match(argv[i + 1]):
                return {"kind": "codex", "nativeSessionId": argv[i + 1]}
    if name == "claude":
        for i, token in enumerate(argv):
            if token.startswith("--resume="):
                session_id = token.split("=", 1)[1]
                if UUID_RE.match(session_id):
                    return {"kind": "claude", "nativeSessionId": session_id}
            if token == "--resume" and i + 1 < len(argv) and UUID_RE.match(argv[i + 1]):
                return {"kind": "claude", "nativeSessionId": argv[i + 1]}
    return None


def _canonical_safe_argv(argv: list[str], owner_home: str) -> list[str] | None:
    if not argv:
        return None
    name = _basename(argv[0])
    if name == "codex" and len(argv) == 3 and argv[1] == "resume" and UUID_RE.match(argv[2]):
        return ["codex", "resume", argv[2]]
    if name == "claude":
        if len(argv) == 3 and argv[1] == "--resume" and UUID_RE.match(argv[2]):
            return ["claude", "--resume", argv[2]]
        if len(argv) == 2 and argv[1].startswith("--resume="):
            session_id = argv[1].split("=", 1)[1]
            if UUID_RE.match(session_id):
                return ["claude", "--resume", session_id]
    hermes = _canonical_hermes_argv(argv, owner_home)
    if hermes is not None:
        return hermes
    python = _canonical_python_http_argv(argv)
    if python is not None:
        return python
    return None


def _looks_like_known_unsafe_argv(argv: list[str], owner_home: str) -> bool:
    if not argv:
        return False
    name = _basename(argv[0])
    if name in {"codex", "claude"}:
        return True
    expected_python = os.path.normpath(os.path.join(owner_home, ".hermes", "hermes-agent-current", "venv", "bin", "python"))
    if os.path.normpath(argv[0]) == expected_python:
        return True
    return any(token == "-m" and i + 1 < len(argv) and argv[i + 1] == "http.server" for i, token in enumerate(argv))


def _canonical_hermes_argv(argv: list[str], owner_home: str) -> list[str] | None:
    if len(argv) not in {4, 5, 6, 7, 8, 9}:
        return None
    expected_python = os.path.normpath(os.path.join(owner_home, ".hermes", "hermes-agent-current", "venv", "bin", "python"))
    if os.path.normpath(argv[0]) != expected_python or not _path_under(argv[0], owner_home):
        return None
    if argv[1:3] != ["-m", "hermes_cli.main"]:
        return None
    index = 3
    profile = ""
    if index < len(argv) and argv[index] == "--profile" and index + 1 < len(argv):
        profile = argv[index + 1]
        index += 2
    elif index < len(argv) and argv[index].startswith("--profile="):
        profile = argv[index].split("=", 1)[1]
        index += 1
    if not PROFILE_RE.match(profile):
        return None
    resume = ""
    if index < len(argv) and argv[index] == "--resume" and index + 1 < len(argv):
        resume = argv[index + 1]
        index += 2
    elif index < len(argv) and argv[index].startswith("--resume="):
        resume = argv[index].split("=", 1)[1]
        index += 1
    if resume and not NATIVE_ID_RE.match(resume):
        return None
    tail = argv[index:]
    if tail not in ([], ["--tui", "--yolo"]):
        return None
    canonical = [expected_python, "-m", "hermes_cli.main", "--profile", profile]
    if resume:
        canonical.extend(["--resume", resume])
    return canonical


def _canonical_python_http_argv(argv: list[str]) -> list[str] | None:
    if len(argv) < 3 or argv[1:3] != ["-m", "http.server"]:
        return None
    name = os.path.basename(argv[0])
    if name not in {"python", "python3"}:
        return None
    bind = ""
    directory = ""
    positionals: list[str] = []
    index = 3
    while index < len(argv):
        token = argv[index]
        if token in {"--bind", "-b"} and index + 1 < len(argv):
            bind = argv[index + 1]
            index += 2
            continue
        if token.startswith("--bind="):
            bind = token.split("=", 1)[1]
            index += 1
            continue
        if token in {"--directory", "-d"} and index + 1 < len(argv):
            directory = argv[index + 1]
            index += 2
            continue
        if token.startswith("--directory="):
            directory = token.split("=", 1)[1]
            index += 1
            continue
        if token.startswith("-"):
            return None
        positionals.append(token)
        index += 1
    if bind not in LOOPBACK_BINDS or len(positionals) > 1:
        return None
    port = "8000" if not positionals else positionals[0]
    try:
        port_int = int(port)
    except ValueError:
        return None
    if port_int < 1 or port_int > 65535:
        return None
    canonical = [name, "-m", "http.server", str(port_int), "--bind", bind]
    if directory:
        canonical.extend(["--directory", directory])
    return canonical


def _agent_from_transcript(transcript: Any) -> dict[str, str] | None:
    if not isinstance(transcript, dict):
        return None
    kind = str(transcript.get("kind", "")).strip().lower()
    session_id = str(transcript.get("sessionId", "")).strip()
    if kind in {"codex", "claude"} and UUID_RE.match(session_id):
        return {"kind": kind, "nativeSessionId": session_id}
    return None


def _live_workload_kind(argv: list[str], comm: str, owner_home: str) -> str:
    agent = _agent_from_codex_claude_argv(argv)
    if agent is not None:
        return agent["kind"]
    if comm in {"codex", "claude"}:
        return comm
    if argv and _basename(argv[0]) in {"codex", "claude"}:
        return _basename(argv[0])
    if _looks_like_hermes_argv(argv, owner_home):
        return "hermes"
    if _looks_like_python_http_server(argv):
        return "python-http-server"
    if comm in SHELLS:
        return "shell"
    if comm or argv:
        return "unknown"
    return ""


def _transcript_conflicts(transcript_agent: dict[str, str], argv_agent: dict[str, str] | None, live_kind: str) -> bool:
    if argv_agent is not None:
        return (
            argv_agent["kind"] != transcript_agent["kind"]
            or argv_agent.get("nativeSessionId") != transcript_agent.get("nativeSessionId")
        )
    if live_kind in {"codex", "claude"}:
        return live_kind != transcript_agent["kind"]
    return live_kind not in {"", transcript_agent["kind"]}


def _hermes_from_process(evidence: dict[str, Any], argv: list[str], owner_home: str, topology: dict[str, Any]) -> dict[str, Any] | None:
    owner = _owner(evidence)
    if not _looks_like_hermes_argv(argv, owner_home):
        return _unresolved(owner, topology, "unsafe_evidence", "argv", "low")
    profile = _option_value(argv, "--profile")
    if profile is None or not PROFILE_RE.match(profile):
        return _unresolved(owner, topology, "unsafe_evidence", "argv", "low")
    resume = _option_value(argv, "--resume")
    if resume:
        if not NATIVE_ID_RE.match(resume):
            return _unresolved(owner, topology, "unsafe_evidence", "argv", "low")
        return _agent_descriptor(
            owner,
            topology,
            {"kind": "hermes", "nativeSessionId": resume, "hermesProfile": profile},
            "argv",
            "high",
        )

    if evidence.get("stateDbIncomplete") is True:
        return _unresolved(owner, topology, "missing_evidence", "state_db", "low")
    candidates = _matching_state_db_candidates(evidence, profile)
    if len(candidates) == 1:
        session_id = str(candidates[0].get("sessionId", "")).strip()
        if not NATIVE_ID_RE.match(session_id):
            return _unresolved(owner, topology, "unsafe_evidence", "state_db", "low")
        return _agent_descriptor(
            owner,
            topology,
            {"kind": "hermes", "nativeSessionId": session_id, "hermesProfile": profile},
            "state_db",
            "medium",
        )
    if len(candidates) > 1:
        return _unresolved(owner, topology, "ambiguous_candidates", "state_db", "low")
    return _unresolved(owner, topology, "missing_evidence", "state_db", "low")


def _looks_like_hermes_argv(argv: list[str], owner_home: str) -> bool:
    return _canonical_hermes_argv(argv, owner_home) is not None


def _matching_state_db_candidates(evidence: dict[str, Any], profile: str) -> list[dict[str, Any]]:
    process = evidence.get("process") if isinstance(evidence.get("process"), dict) else {}
    process_cwd = _clean_abs_path(process.get("cwd") or evidence.get("pane", {}).get("cwd", ""))
    process_started = _parse_time(process.get("startedAt"))
    tolerance = int(evidence.get("stateDbToleranceSeconds", 5))
    candidates = []
    for candidate in evidence.get("stateDbCandidates", []):
        if not isinstance(candidate, dict):
            continue
        if str(candidate.get("profile", "")).strip() != profile:
            continue
        if _clean_abs_path(candidate.get("cwd", "")) != process_cwd:
            continue
        candidate_started = _parse_time(candidate.get("startedAt"))
        if process_started is None or candidate_started is None:
            continue
        if abs((candidate_started - process_started).total_seconds()) <= tolerance:
            candidates.append(candidate)
    return candidates


def _python_http_server_descriptor(
    evidence: dict[str, Any],
    argv: list[str],
    owner: dict[str, Any],
    topology: dict[str, Any],
    owner_home: str,
) -> dict[str, Any] | None:
    bind = _option_value(argv, "--bind")
    if bind not in LOOPBACK_BINDS:
        return None
    port = _python_http_server_port(argv)
    if port is None or port < 1 or port > 65535:
        return None
    directory = _option_value(argv, "--directory")
    if directory is None:
        directory = str(evidence.get("pane", {}).get("cwd", ""))
    pane_cwd = str(evidence.get("pane", {}).get("cwd", ""))
    resolved = _resolve_owner_path(directory, pane_cwd, owner_home)
    if resolved is None:
        return None
    desc = _descriptor(owner, topology, "command", "python-http-server", "argv", "high")
    desc["command"] = {
        "kind": "python-http-server",
        "pythonHTTPServer": {"bind": bind, "port": port, "directory": resolved},
    }
    return desc


def _looks_like_python_http_server(argv: list[str]) -> bool:
    return _canonical_python_http_argv(argv) is not None


def _python_http_server_port(argv: list[str]) -> int | None:
    try:
        module_index = argv.index("http.server")
    except ValueError:
        return None
    positionals: list[str] = []
    i = module_index + 1
    while i < len(argv):
        token = argv[i]
        if token == "--":
            positionals.extend(argv[i + 1 :])
            break
        if token in {"--bind", "-b", "--directory", "-d", "--protocol", "-p"}:
            if i + 1 >= len(argv):
                return None
            i += 2
            continue
        if token.startswith(("--bind=", "--directory=", "--protocol=")):
            i += 1
            continue
        if token.startswith("-"):
            return None
        positionals.append(token)
        i += 1
    if len(positionals) == 0:
        return 8000
    if len(positionals) != 1:
        return None
    try:
        return int(positionals[0])
    except ValueError:
        return None


def _option_value(argv: list[str], option: str) -> str | None:
    for i, token in enumerate(argv):
        if token.startswith(option + "="):
            return token.split("=", 1)[1].strip()
        if token == option and i + 1 < len(argv):
            return argv[i + 1].strip()
    return None


def _descriptor(
    owner: dict[str, Any],
    topology: dict[str, Any],
    mode: str,
    workload_kind: str,
    evidence_source: str,
    confidence: str,
) -> dict[str, Any]:
    return {
        "mode": mode,
        "owner": owner,
        "topology": topology,
        "workloadKind": workload_kind,
        "evidenceSource": evidence_source,
        "confidence": confidence,
    }


def _agent_descriptor(
    owner: dict[str, Any],
    topology: dict[str, Any],
    agent: dict[str, str],
    evidence_source: str,
    confidence: str,
) -> dict[str, Any]:
    desc = _descriptor(owner, topology, "agent", agent["kind"], evidence_source, confidence)
    desc["agent"] = agent
    return desc


def _unresolved(
    owner: dict[str, Any],
    topology: dict[str, Any],
    reason: str,
    evidence_source: str,
    confidence: str,
) -> dict[str, Any]:
    owner = dict(owner)
    owner["mayRestart"] = False
    desc = _descriptor(owner, topology, "unresolved", "unknown", evidence_source, confidence)
    desc["unresolvedReason"] = reason
    return desc


def _owner(evidence: dict[str, Any]) -> dict[str, Any]:
    owner = evidence.get("owner") if isinstance(evidence.get("owner"), dict) else {}
    return {
        "kind": str(owner.get("kind", "session_bank")).strip(),
        "ref": str(owner.get("ref", "")).strip(),
        "mayRestart": bool(owner.get("mayRestart", False)),
    }


def _topology(evidence: dict[str, Any]) -> dict[str, Any]:
    pane = evidence.get("pane") if isinstance(evidence.get("pane"), dict) else {}
    process = evidence.get("process") if isinstance(evidence.get("process"), dict) else {}
    topology = {
        "sessionName": str(evidence.get("sessionName", "")).strip(),
        "windowIndex": int(pane.get("windowIndex", 0)),
        "windowName": str(pane.get("windowName", "")).strip(),
        "paneIndex": int(pane.get("paneIndex", 0)),
        "paneId": str(pane.get("paneId", "")).strip(),
        "paneCurrentPath": _clean_abs_path(pane.get("cwd") or process.get("cwd") or ""),
    }
    layout = _safe_layout(pane.get("windowLayout", ""))
    if layout:
        topology["windowLayout"] = layout
    return topology


def _argv(process: dict[str, Any]) -> list[str]:
    argv = process.get("argv")
    if not isinstance(argv, list):
        return []
    return [str(token).strip() for token in argv if str(token).strip()]


def _process_name(process: dict[str, Any], argv: list[str]) -> str:
    comm = str(process.get("comm", "")).strip().lower()
    if comm:
        return _basename(comm)
    if argv:
        return _basename(argv[0])
    return ""


def _basename(value: str) -> str:
    return os.path.basename(value).strip().lower()


def _clean_abs_path(value: Any) -> str:
    text = str(value or "").strip()
    if not text:
        return ""
    return os.path.normpath(text)


def _safe_layout(value: Any) -> str:
    text = str(value or "").strip()
    if not text or len(text) > 4096:
        return ""
    if "\x00" in text or "\n" in text or "\r" in text:
        return ""
    return text


def _resolve_owner_path(value: str, pane_cwd: str, owner_home: str) -> str | None:
    if "\x00" in value or "\n" in value or "\r" in value or "#" in value:
        return None
    path = value if os.path.isabs(value) else os.path.join(pane_cwd, value)
    path = os.path.normpath(path)
    if not _path_under(path, owner_home):
        return None
    return path


def _path_under(path: str, owner_home: str) -> bool:
    if not path or not owner_home or not os.path.isabs(path) or not os.path.isabs(owner_home):
        return False
    try:
        Path(path).resolve(strict=False).relative_to(Path(owner_home).resolve(strict=False))
        return True
    except ValueError:
        return False


def _parse_time(value: Any) -> datetime | None:
    text = str(value or "").strip()
    if not text:
        return None
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)
