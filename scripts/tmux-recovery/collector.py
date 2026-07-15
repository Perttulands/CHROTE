#!/usr/bin/env python3
"""Bounded owner-side evidence collection for owner_probe.py.

The collector emits only tmux topology, pane status, process tree argv/cwd
metadata, and optional Hermes state-db candidate metadata. It never reads
environments, transcript contents, or shell history.
"""

from __future__ import annotations

import argparse
from dataclasses import dataclass, field
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import pwd
import re
import sqlite3
import subprocess
import sys
import time
from typing import Any


TMUX_PANE_FORMAT = "#{session_name}\t#{window_index}\t#{window_name}\t#{window_layout}\t#{pane_index}\t#{pane_id}\t#{pane_current_path}\t#{pane_pid}\t#{pane_current_command}\t#{pane_dead}"

UUID_RE = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
NATIVE_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
PROFILE_RE = re.compile(r"^[a-z][a-z0-9_-]{0,63}$")
LOOPBACK_BINDS = {"127.0.0.1", "localhost", "::1"}

MAX_PROCESS_TREE = 64
MAX_ARGV_TOKENS = 64
MAX_ARG_TOKEN_CHARS = 512
MAX_STATE_DB_CANDIDATES = 16
MAX_STATE_DB_SCAN_ROWS = 4096


class CollectorError(RuntimeError):
    pass


@dataclass
class RecordingCommandRunner:
    outputs: dict[tuple[str, ...], str]
    calls: list[tuple[str, ...]] = field(default_factory=list)

    def run(self, argv: list[str]) -> str:
        key = tuple(argv)
        self.calls.append(key)
        if key not in self.outputs:
            raise CollectorError(f"unexpected command: {argv!r}")
        return self.outputs[key]


class ShellCommandRunner:
    def run(self, argv: list[str]) -> str:
        proc = subprocess.run(argv, text=True, capture_output=True, check=False)
        if proc.returncode != 0:
            raise CollectorError(proc.stderr.strip() or f"{argv[0]} exited {proc.returncode}")
        return proc.stdout


class ProcfsReader:
    def __init__(self, root: str | Path = "/proc") -> None:
        self.root = Path(root)
        self._boot_time = self._read_boot_time()
        try:
            self._ticks_per_second = os.sysconf(os.sysconf_names["SC_CLK_TCK"])
        except (KeyError, ValueError, OSError):
            self._ticks_per_second = 100

    def list_pids(self) -> list[str]:
        return sorted(path.name for path in self.root.iterdir() if path.name.isdigit())

    def process_info(self, pid: str) -> dict[str, Any]:
        stat_info = self.stat_info(pid)
        proc_dir = self.root / pid
        cmdline = (proc_dir / "cmdline").read_bytes()
        argv = [part.decode("utf-8", errors="replace") for part in cmdline.split(b"\x00") if part]
        cwd = ""
        try:
            cwd = str((proc_dir / "cwd").readlink())
        except OSError:
            cwd = ""
        return {
            "pid": pid,
            "ppid": stat_info.get("ppid", ""),
            "comm": stat_info.get("comm", ""),
            "argv": argv,
            "cwd": cwd,
            "startedAt": stat_info.get("startedAt", ""),
            "startTicks": stat_info.get("startTicks", ""),
        }

    def stat_info(self, pid: str) -> dict[str, Any]:
        proc_dir = self.root / pid
        stat_text = (proc_dir / "stat").read_text(encoding="utf-8", errors="replace")
        stat_info = _parse_proc_stat(stat_text)
        started_at = ""
        if self._boot_time and stat_info.get("startTicks") is not None:
            started_at = datetime.fromtimestamp(
                self._boot_time + (int(stat_info["startTicks"]) / self._ticks_per_second),
                tz=timezone.utc,
            ).strftime("%Y-%m-%dT%H:%M:%SZ")
        return {
            "pid": pid,
            "ppid": stat_info.get("ppid", ""),
            "comm": stat_info.get("comm", ""),
            "startedAt": started_at,
            "startTicks": str(stat_info.get("startTicks", "")),
        }

    def _read_boot_time(self) -> int:
        try:
            for line in (self.root / "stat").read_text(encoding="utf-8", errors="replace").splitlines():
                if line.startswith("btime "):
                    return int(line.split()[1])
        except (OSError, ValueError, IndexError):
            return 0
        return 0


def collect_tmux_evidence(
    *,
    socket: str,
    owner: dict[str, Any],
    owner_home: str,
    command_runner: Any | None = None,
    proc_reader: Any | None = None,
    state_db_tolerance_seconds: int = 5,
    state_db_scan_cap: int = MAX_STATE_DB_SCAN_ROWS,
    unix_user: str = "",
    session_filter: str | None = None,
    owner_map: dict[str, dict[str, Any]] | None = None,
) -> list[dict[str, Any]]:
    runner = command_runner or ShellCommandRunner()
    proc_reader = proc_reader or ProcfsReader()
    panes_raw = runner.run(["tmux", "-S", socket, "list-panes", "-a", "-F", TMUX_PANE_FORMAT])
    evidence: list[dict[str, Any]] = []
    seen_panes: set[tuple[str, int, int, str]] = set()
    for pane in _parse_panes(panes_raw):
        if session_filter and pane["sessionName"] != session_filter:
            continue
        key = (pane["sessionName"], pane["pane"]["windowIndex"], pane["pane"]["paneIndex"], pane["pane"]["paneId"])
        if key in seen_panes:
            raise CollectorError(f"duplicate pane evidence for {key}")
        seen_panes.add(key)
        pane_owner = _owner_for_session(
            owner,
            session_name=pane["sessionName"],
            unix_user=unix_user,
            session_filter=session_filter,
            owner_map=owner_map,
        )
        pane_pid = str(pane.pop("pid", "")).strip()
        process_tree = collect_process_tree(pane_pid, owner_home, proc_reader=proc_reader)
        root_process = process_tree[0] if process_tree else {}
        item = {
            "owner": pane_owner,
            "ownerHome": str(owner_home).strip(),
            "sessionName": pane["sessionName"],
            "unixUser": str(unix_user).strip(),
            "pane": pane["pane"],
            "paneStatus": pane["paneStatus"],
            "process": root_process,
            "processTree": process_tree,
        }
        candidates, incomplete = collect_hermes_candidates_for_tree(
            process_tree,
            owner_home,
            state_db_tolerance_seconds,
            scan_cap=state_db_scan_cap,
        )
        if candidates:
            item["stateDbCandidates"] = candidates
            item["stateDbToleranceSeconds"] = state_db_tolerance_seconds
        if incomplete:
            item["stateDbIncomplete"] = True
        evidence.append(item)
    return evidence


def collect_process_tree(pane_pid: str, owner_home: str, *, proc_reader: Any | None = None) -> list[dict[str, Any]]:
    pane_pid = str(pane_pid).strip()
    if not pane_pid:
        return []
    proc_reader = proc_reader or ProcfsReader()
    raw_stats: dict[str, dict[str, Any]] = {}
    children: dict[str, list[str]] = {}
    stat_reader = getattr(proc_reader, "stat_info", None)
    if stat_reader is None:
        return []
    for pid in proc_reader.list_pids():
        try:
            info = stat_reader(pid)
        except (OSError, CollectorError, KeyError):
            continue
        raw_stats[pid] = info
        children.setdefault(str(info.get("ppid", "")), []).append(pid)

    result: list[dict[str, Any]] = []
    queue = [pane_pid]
    seen: set[str] = set()
    while queue and len(result) < MAX_PROCESS_TREE:
        pid = queue.pop(0)
        if pid in seen:
            continue
        seen.add(pid)
        if pid in raw_stats:
            raw = raw_stats[pid]
            try:
                before = proc_reader.stat_info(pid)
            except (OSError, CollectorError, KeyError):
                before = {}
            if not _same_pid_identity(raw, before):
                result.append(_bounded_process_info(_unsafe_process_info(raw, "pid_reused"), owner_home))
                continue
            try:
                info = proc_reader.process_info(pid)
            except (OSError, CollectorError, KeyError):
                info = raw
            try:
                after = proc_reader.stat_info(pid)
            except (OSError, CollectorError, KeyError):
                after = {}
            if not _same_pid_identity(raw, after):
                result.append(_bounded_process_info(_unsafe_process_info(raw, "pid_reused"), owner_home))
                continue
            result.append(_bounded_process_info(info, owner_home))
        queue.extend(children.get(pid, []))
    return result


def collect_hermes_candidates_for_tree(
    process_tree: list[dict[str, Any]],
    owner_home: str,
    tolerance_seconds: int,
    *,
    scan_cap: int = MAX_STATE_DB_SCAN_ROWS,
) -> tuple[list[dict[str, Any]], bool]:
    candidates: list[dict[str, Any]] = []
    incomplete = False
    for process in process_tree:
        argv = process.get("argv")
        if not isinstance(argv, list) or not _looks_like_hermes_argv(argv, owner_home):
            continue
        profile = _option_value(argv, "--profile")
        if not profile or not PROFILE_RE.match(profile):
            continue
        if _option_value(argv, "--resume"):
            continue
        found, partial = collect_hermes_state_candidates(
            owner_home,
            profile,
            str(process.get("cwd", "")),
            str(process.get("startedAt", "")),
            tolerance_seconds=tolerance_seconds,
            scan_cap=scan_cap,
        )
        candidates.extend(found)
        incomplete = incomplete or partial
        if len(candidates) >= 2:
            return candidates[:MAX_STATE_DB_CANDIDATES], False
    return candidates[:MAX_STATE_DB_CANDIDATES], incomplete


def collect_hermes_state_candidates(
    owner_home: str,
    profile: str,
    process_cwd: str,
    process_started: str,
    *,
    tolerance_seconds: int = 5,
    scan_cap: int = MAX_STATE_DB_SCAN_ROWS,
) -> tuple[list[dict[str, Any]], bool]:
    if not PROFILE_RE.match(profile):
        return [], False
    db_path = Path(owner_home) / ".hermes" / "profiles" / profile / "state.db"
    if not db_path.exists():
        return [], False
    process_time = _parse_time(process_started)
    if process_time is None:
        return [], False
    uri = "file:" + str(db_path) + "?mode=ro"
    candidates: list[dict[str, Any]] = []
    scanned = 0
    incomplete = False
    try:
        with sqlite3.connect(uri, uri=True) as conn:
            table_names = {
                row[0]
                for row in conn.execute("SELECT name FROM sqlite_master WHERE type='table'")
                if isinstance(row[0], str)
            }
            for table in sorted(table_names):
                if table.lower() == "messages":
                    continue
                columns = [row[1] for row in conn.execute(f"PRAGMA table_info({_quote_identifier(table)})")]
                id_col = _first_present(columns, ["session_id", "sessionId", "id", "native_session_id"])
                cwd_col = _first_present(columns, ["cwd", "working_directory", "current_workdir"])
                started_col = _first_present(columns, ["started_at", "startedAt", "created_at", "start_time"])
                if not id_col or not cwd_col or not started_col:
                    continue
                query = (
                    f"SELECT {_quote_identifier(id_col)}, {_quote_identifier(cwd_col)}, {_quote_identifier(started_col)} "
                    f"FROM {_quote_identifier(table)}"
                )
                for row in conn.execute(query):
                    if scanned >= scan_cap:
                        incomplete = True
                        break
                    scanned += 1
                    candidate = _state_db_candidate_from_row(row, profile, process_cwd, process_time, tolerance_seconds)
                    if candidate is not None:
                        candidates.append(candidate)
                        if len(candidates) >= 2:
                            return candidates[:MAX_STATE_DB_CANDIDATES], False
                if incomplete:
                    break
    except sqlite3.Error:
        return [], False

    return candidates[:MAX_STATE_DB_CANDIDATES], incomplete


def initialize_test_state_db(path: Path, rows: list[tuple[str, str, str]]) -> None:
    with sqlite3.connect(path) as conn:
        conn.execute("CREATE TABLE sessions (session_id TEXT, cwd TEXT, started_at TEXT)")
        conn.executemany("INSERT INTO sessions (session_id, cwd, started_at) VALUES (?, ?, ?)", rows)


def main(
    argv: list[str] | None = None,
    *,
    command_runner: Any | None = None,
    proc_reader: Any | None = None,
    current_user: str | None = None,
) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--socket", required=True)
    parser.add_argument("--unix-user", required=True)
    parser.add_argument("--owner-home", required=True)
    parser.add_argument("--owner-kind", required=True, choices=["session_bank", "persistent_agent", "external_manager"])
    parser.add_argument("--owner-ref")
    parser.add_argument("--owner-may-restart", action="store_true")
    parser.add_argument("--session-name")
    parser.add_argument("--output")
    args = parser.parse_args(argv)

    actual_user = current_user if current_user is not None else current_effective_user()
    if actual_user != args.unix_user:
        print(f"collector must run as requested unix user {args.unix_user}; current user is {actual_user}", file=sys.stderr)
        return 2
    if args.owner_kind != "session_bank" and (not args.owner_ref or not args.session_name):
        print("persistent/external collection requires explicit --owner-ref and --session-name", file=sys.stderr)
        return 2
    evidence = collect_tmux_evidence(
        socket=args.socket,
        owner={"kind": args.owner_kind, "ref": args.owner_ref or "", "mayRestart": bool(args.owner_may_restart)},
        owner_home=args.owner_home,
        unix_user=args.unix_user,
        session_filter=args.session_name,
        command_runner=command_runner,
        proc_reader=proc_reader,
    )
    raw = json.dumps(evidence, indent=2, sort_keys=True) + "\n"
    if args.output:
        Path(args.output).write_text(raw, encoding="utf-8")
    else:
        print(raw, end="")
    return 0


def current_effective_user() -> str:
    return pwd.getpwuid(os.geteuid()).pw_name


def _parse_panes(raw: str) -> list[dict[str, Any]]:
    panes: list[dict[str, Any]] = []
    for line in raw.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) != 10:
            raise CollectorError("tmux pane output did not match expected bounded format")
        session_name, window_index, window_name, layout, pane_index, pane_id, cwd, pid, current_command, pane_dead = parts
        dead = pane_dead.strip() in {"1", "true", "yes"}
        panes.append(
            {
                "sessionName": session_name.strip(),
                "pid": pid.strip(),
                "pane": {
                    "windowIndex": int(window_index),
                    "windowName": window_name.strip(),
                    "windowLayout": layout.strip(),
                    "paneIndex": int(pane_index),
                    "paneId": pane_id.strip(),
                    "cwd": cwd.strip(),
                },
                "paneStatus": {
                    "dead": dead,
                    "currentCommand": current_command.strip(),
                    "cwd": cwd.strip(),
                },
            }
        )
    return panes


def _bounded_process_info(info: dict[str, Any], owner_home: str) -> dict[str, Any]:
    argv = [str(token) for token in info.get("argv", []) if str(token)]
    bounded = {
        "pid": str(info.get("pid", "")),
        "ppid": str(info.get("ppid", "")),
        "comm": _bounded_token(str(info.get("comm", ""))),
        "cwd": _bounded_token(str(info.get("cwd", ""))),
        "startedAt": _bounded_token(str(info.get("startedAt", ""))),
        "startTicks": _bounded_token(str(info.get("startTicks", ""))),
    }
    canonical = _canonical_safe_argv(argv, owner_home)
    if canonical is not None:
        bounded["argv"] = [_bounded_token(token) for token in canonical[:MAX_ARGV_TOKENS]]
    else:
        bounded["argvRedacted"] = True
    if info.get("unsafeEvidence"):
        bounded["unsafeEvidence"] = _bounded_token(str(info["unsafeEvidence"]))
    return bounded


def _owner_for_session(
    owner: dict[str, Any],
    *,
    session_name: str,
    unix_user: str,
    session_filter: str | None,
    owner_map: dict[str, dict[str, Any]] | None,
) -> dict[str, Any]:
    if owner_map is not None:
        mapped = owner_map.get(session_name)
        if not isinstance(mapped, dict):
            raise CollectorError(f"missing explicit owner mapping for session {session_name}")
        return dict(mapped)
    kind = str(owner.get("kind", "")).strip()
    if kind == "session_bank":
        user = str(unix_user).strip()
        if user:
            return {"kind": "session_bank", "ref": f"{user}/{session_name}", "mayRestart": bool(owner.get("mayRestart", False))}
        return dict(owner)
    if not session_filter:
        raise CollectorError("persistent/external collection requires explicit session filter or owner map")
    return dict(owner)


def _state_db_candidate_from_row(
    row: Any,
    profile: str,
    process_cwd: str,
    process_time: datetime,
    tolerance_seconds: int,
) -> dict[str, Any] | None:
    session_id, cwd, started_at = str(row[0]), str(row[1]), str(row[2])
    if not NATIVE_ID_RE.match(session_id):
        return None
    if os.path.normpath(cwd) != os.path.normpath(process_cwd):
        return None
    candidate_time = _parse_time(started_at)
    if candidate_time is None:
        return None
    if abs((candidate_time - process_time).total_seconds()) <= tolerance_seconds:
        return {"profile": profile, "sessionId": session_id, "cwd": os.path.normpath(cwd), "startedAt": _format_time(candidate_time)}
    return None


def _same_pid_identity(expected: dict[str, Any], actual: dict[str, Any]) -> bool:
    expected_ticks = str(expected.get("startTicks", "")).strip()
    actual_ticks = str(actual.get("startTicks", "")).strip()
    if not expected_ticks or not actual_ticks:
        return False
    return (
        str(expected.get("pid", "")).strip() == str(actual.get("pid", "")).strip()
        and str(expected.get("ppid", "")).strip() == str(actual.get("ppid", "")).strip()
        and expected_ticks == actual_ticks
    )


def _unsafe_process_info(info: dict[str, Any], reason: str) -> dict[str, Any]:
    redacted = dict(info)
    redacted["unsafeEvidence"] = reason
    redacted.pop("argv", None)
    return redacted


def _canonical_safe_argv(argv: list[str], owner_home: str) -> list[str] | None:
    if not argv:
        return None
    name = os.path.basename(argv[0]).lower()
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
    python_http = _canonical_python_http_argv(argv)
    if python_http is not None:
        return python_http
    return None


def _canonical_hermes_argv(argv: list[str], owner_home: str) -> list[str] | None:
    if len(argv) not in {4, 5, 6, 7, 8, 9}:
        return None
    expected_python = os.path.normpath(os.path.join(owner_home, ".hermes", "hermes-agent-current", "venv", "bin", "python"))
    if os.path.normpath(argv[0]) != expected_python:
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
    if bind not in LOOPBACK_BINDS:
        return None
    if len(positionals) > 1:
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


def _looks_like_hermes_argv(argv: list[str], owner_home: str) -> bool:
    return _canonical_hermes_argv(argv, owner_home) is not None


def _looks_like_python_http_server(argv: list[str]) -> bool:
    return _canonical_python_http_argv(argv) is not None


def _option_value(argv: list[str], option: str) -> str | None:
    for i, token in enumerate(argv):
        if token.startswith(option + "="):
            return token.split("=", 1)[1].strip()
        if token == option and i + 1 < len(argv):
            return argv[i + 1].strip()
    return None


def _bounded_token(value: str) -> str:
    value = str(value).replace("\x00", "").replace("\n", " ").replace("\r", " ").strip()
    if len(value) > MAX_ARG_TOKEN_CHARS:
        return value[:MAX_ARG_TOKEN_CHARS]
    return value


def _parse_proc_stat(text: str) -> dict[str, Any]:
    start = text.find("(")
    end = text.rfind(")")
    if start == -1 or end == -1 or end <= start:
        return {}
    comm = text[start + 1 : end]
    rest = text[end + 2 :].split()
    if len(rest) < 20:
        return {"comm": comm}
    return {"comm": comm, "ppid": rest[1], "startTicks": int(rest[19])}


def _parse_time(value: str) -> datetime | None:
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


def _format_time(value: datetime) -> str:
    return value.astimezone(timezone.utc).replace(microsecond=0).strftime("%Y-%m-%dT%H:%M:%SZ")


def _first_present(columns: list[str], names: list[str]) -> str:
    lower = {column.lower(): column for column in columns}
    for name in names:
        if name.lower() in lower:
            return lower[name.lower()]
    return ""


def _quote_identifier(value: str) -> str:
    return '"' + value.replace('"', '""') + '"'


if __name__ == "__main__":
    raise SystemExit(main())
