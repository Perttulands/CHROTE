#!/usr/bin/env python3
"""Disposable end-to-end smoke for CHROTE tmux recovery.

The smoke uses only unique resources under /tmp plus an optional current-user
systemd transient unit. It builds and starts the current CHROTE server, snapshots
real tmux evidence through the existing operator CLI/API path, kills only the
disposable tmux server, restores through the existing CLI/API path, verifies live
workloads, then removes everything it created.
"""

from __future__ import annotations

import argparse
import copy
from dataclasses import dataclass, field
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import shlex
import signal
import socket
import stat
import subprocess
import sys
import time
from typing import Any, Callable
from urllib import error, parse, request
import uuid

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parents[1]
SRC_DIR = REPO_ROOT / "src"

sys.path.insert(0, str(SCRIPT_DIR))

import collector  # noqa: E402
import manifest as manifest_lib  # noqa: E402
import snapshot as snapshot_lib  # noqa: E402


class SmokeFailure(RuntimeError):
    pass


class SmokeBlocker(SmokeFailure):
    pass


class CleanupStack:
    def __init__(self) -> None:
        self._callbacks: list[Callable[[], None]] = []
        self._closed = False

    def add(self, callback: Callable[[], None]) -> None:
        if self._closed:
            raise SmokeFailure("cleanup stack already closed")
        self._callbacks.append(callback)

    def close(self) -> None:
        if self._closed:
            return
        self._closed = True
        errors: list[str] = []
        while self._callbacks:
            callback = self._callbacks.pop()
            try:
                callback()
            except Exception as exc:  # pragma: no cover - exercised by helper test
                errors.append(str(exc))
        if errors:
            raise SmokeFailure("; ".join(errors))


@dataclass
class CommandResult:
    argv: list[str]
    returncode: int
    stdout: str
    stderr: str


@dataclass
class SmokeState:
    checks: dict[str, Any] = field(default_factory=dict)
    skips: dict[str, str] = field(default_factory=dict)
    resources: dict[str, Any] = field(default_factory=dict)
    command_log: list[list[str]] = field(default_factory=list)


def hermes_resume_argv(owner_home: str | Path, profile: str, native_id: str) -> list[str]:
    owner = Path(owner_home)
    return [
        str(owner / ".hermes" / "hermes-agent-current" / "venv" / "bin" / "python"),
        "-m",
        "hermes_cli.main",
        "--profile",
        profile,
        "--resume",
        native_id,
    ]


def managed_systemd_record(*, session_name: str, unix_user: str, unit: str, owner_home: str | Path) -> dict[str, Any]:
    owner = {"kind": "external_manager", "ref": f"systemd:user/{unit}", "mayRestart": False}
    return {
        "sessionName": session_name,
        "unixUser": unix_user,
        "owner": owner,
        "managerKind": "systemd-user",
        "managerRef": unit,
        "restartAllowed": False,
        "statusProbe": {"kind": "systemd-user", "unit": unit, "expectActiveState": "active"},
        "descriptors": [
            {
                "mode": "managed",
                "owner": owner,
                "topology": {
                    "sessionName": session_name,
                    "windowIndex": 0,
                    "windowName": "managed",
                    "paneIndex": 0,
                    "paneId": "%managed",
                    "paneCurrentPath": str(Path(owner_home)),
                },
                "workloadKind": "managed",
                "evidenceSource": "manager",
                "confidence": "high",
            }
        ],
    }


# Live tmux sockets are matched by SHAPE, not by this host's names. The previous version listed
# one operator's username and uid literally, which failed twice over: a repo-wide neutrality sweep
# rewrote those literals to the neutral fixture values `build` and uid 2001, so the guard stopped
# catching any live socket AND started tripping on the smoke's own `go build` argv in command_log.
# A shape rule cannot be broken that way, and it catches every uid rather than one.
LIVE_TMUX_SOCKET_PATTERNS = (
    r"/run/user/\d+/[\w.-]*tmux[\w.-]*(?:/[\w.-]+)*",
    r"/tmp/tmux-\d+(?:/[\w.-]+)*",
    r"/run/chrote/[\w.-]*tmux[\w.-]*(?:/[\w.-]+)*",
)
# The product's compiled defaults plus this deployment's overrides. The smoke allocates its own
# ports and choose_loopback_port() refuses to hand back any of these.
LIVE_PORTS = (8094, 8095, 7683, 7686)
LIVE_DATA_PATHS = ("/srv/data/chrote",)


def assert_no_forbidden_references(values: list[Any], *, temp_root: str | Path | None = None) -> None:
    """Fail if the smoke's own record mentions a resource it must never have touched.

    Everything the smoke legitimately owns lives under `temp_root`, so a uid-scoped tmux socket
    path outside that root means the disposable run reached into live infrastructure.
    """
    text = "\n".join(_stringify(value) for value in values)
    lower = text.lower()
    root = str(Path(temp_root)) if temp_root is not None else None

    for port in LIVE_PORTS:
        for needle in (f"127.0.0.1:{port}", f"localhost:{port}", f":{port}"):
            if needle in lower:
                raise SmokeFailure(f"forbidden live {port} reference detected: {needle}")
    for needle in LIVE_DATA_PATHS:
        if needle in lower:
            raise SmokeFailure(f"forbidden live data reference detected: {needle}")
    for pattern in LIVE_TMUX_SOCKET_PATTERNS:
        for match in re.finditer(pattern, text):
            found = match.group(0)
            if root is not None and found.startswith(root):
                continue  # the smoke's own disposable socket, not a live one
            raise SmokeFailure(f"forbidden live tmux socket reference detected: {found}")
    if "claude" in lower:
        raise SmokeFailure("forbidden live agent session reference detected: claude")


def _stringify(value: Any) -> str:
    if isinstance(value, str):
        return value
    return json.dumps(value, sort_keys=True, default=str)


def run_command(
    argv: list[str],
    *,
    cwd: str | Path | None = None,
    env: dict[str, str] | None = None,
    timeout: float = 30,
    check: bool = True,
    state: SmokeState | None = None,
) -> CommandResult:
    if state is not None:
        state.command_log.append(list(argv))
    proc = subprocess.run(
        argv,
        cwd=str(cwd) if cwd is not None else None,
        env=env,
        text=True,
        capture_output=True,
        timeout=timeout,
        check=False,
    )
    result = CommandResult(argv=list(argv), returncode=proc.returncode, stdout=proc.stdout, stderr=proc.stderr)
    if check and proc.returncode != 0:
        raise SmokeFailure(
            f"command failed ({proc.returncode}): {shlex.join(argv)}\nstdout={proc.stdout[-10000:]}\nstderr={proc.stderr[-4000:]}"
        )
    return result


def choose_loopback_port(socket_factory: Any = socket.socket) -> int:
    forbidden = {8094, 8095, 7683, 7686}
    for _ in range(50):
        try:
            with socket_factory(socket.AF_INET, socket.SOCK_STREAM) as sock:
                sock.bind(("127.0.0.1", 0))
                port = int(sock.getsockname()[1])
        except PermissionError as exc:
            raise SmokeBlocker("loopback sockets are blocked by this execution environment") from exc
        if port not in forbidden:
            return port
    raise SmokeFailure("could not allocate non-live loopback port")


def find_required_tool(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise SmokeBlocker(f"required tool is missing: {name}")
    return path


def compile_hermes_fixture(owner_home: Path, *, state: SmokeState) -> Path:
    cc = shutil.which("cc") or shutil.which("gcc") or shutil.which("clang")
    if not cc:
        raise SmokeBlocker("compiled Hermes fixture requires cc, gcc, or clang")
    exe = owner_home / ".hermes" / "hermes-agent-current" / "venv" / "bin" / "python"
    exe.parent.mkdir(parents=True, exist_ok=True)
    source = owner_home / ".hermes" / "hermes_fixture.c"
    source.write_text(
        "\n".join(
            [
                "#include <signal.h>",
                "#include <unistd.h>",
                "static volatile sig_atomic_t keep_running = 1;",
                "static void stop(int sig) { (void)sig; keep_running = 0; }",
                "int main(int argc, char **argv) {",
                "  (void)argc;",
                "  (void)argv;",
                "  signal(SIGTERM, stop);",
                "  signal(SIGINT, stop);",
                "  while (keep_running) sleep(1);",
                "  return 0;",
                "}",
                "",
            ]
        ),
        encoding="utf-8",
    )
    run_command([cc, "-O2", str(source), "-o", str(exe)], timeout=30, state=state)
    exe.chmod(0o755)
    return exe


def build_server(temp_root: Path, *, state: SmokeState) -> Path:
    binary = temp_root / "bin" / "chrote-server"
    binary.parent.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["GOCACHE"] = str(temp_root / "go-cache")
    run_command(["go", "build", "-o", str(binary), "./cmd/server"], cwd=SRC_DIR, env=env, timeout=120, state=state)
    return binary


def start_server(binary: Path, env: dict[str, str], log_path: Path, cleanup: CleanupStack) -> subprocess.Popen[str]:
    handle = log_path.open("w", encoding="utf-8")
    proc = subprocess.Popen(
        [str(binary), "-host", "127.0.0.1", "-port", env["PORT"], "-ttyd-port", env["TTYD_PORT"], "-start-ttyd=false"],
        cwd=str(SRC_DIR),
        env=env,
        text=True,
        stdout=handle,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )

    def stop() -> None:
        handle.close()
        if proc.poll() is not None:
            return
        try:
            os.killpg(proc.pid, signal.SIGTERM)
        except ProcessLookupError:
            return
        try:
            proc.wait(timeout=8)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL)
            proc.wait(timeout=5)

    cleanup.add(stop)
    return proc


def http_json(url: str, *, method: str = "GET", body: dict[str, Any] | None = None, timeout: float = 5) -> dict[str, Any]:
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body, separators=(",", ":")).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = request.Request(url, data=data, method=method, headers=headers)
    with request.urlopen(req, timeout=timeout) as resp:
        raw = resp.read().decode("utf-8")
    return json.loads(raw or "{}")


def wait_health(api_url: str, proc: subprocess.Popen[str]) -> None:
    deadline = time.time() + 20
    last = ""
    while time.time() < deadline:
        if proc.poll() is not None:
            raise SmokeFailure(f"server exited before health check: {proc.returncode}")
        try:
            payload = http_json(api_url + "/api/health", timeout=1)
            if payload.get("status") == "ok":
                return
            last = json.dumps(payload)
        except Exception as exc:
            last = str(exc)
        time.sleep(0.2)
    raise SmokeFailure(f"server health check timed out: {last}")


def wait_http_ok(url: str, timeout: float = 10) -> None:
    deadline = time.time() + timeout
    last = ""
    while time.time() < deadline:
        try:
            req = request.Request(url, method="GET")
            with request.urlopen(req, timeout=1) as resp:
                if resp.status == 200:
                    return
                last = f"status {resp.status}"
        except error.URLError as exc:
            last = str(exc.reason)
        time.sleep(0.2)
    raise SmokeFailure(f"helper HTTP probe timed out for {url}: {last}")


def collect_sessions(socket_path: Path, unix_user: str, owner_home: Path) -> list[dict[str, Any]]:
    evidence = collector.collect_tmux_evidence(
        socket=str(socket_path),
        owner={"kind": "session_bank", "ref": "", "mayRestart": True},
        owner_home=str(owner_home),
        unix_user=unix_user,
    )
    return snapshot_lib.sessions_from_collected_evidence(evidence, unix_user=unix_user)


def wait_exact_hermes_argv(socket_path: Path, owner_home: Path, hermes_argv: list[str], timeout: float = 10) -> None:
    deadline = time.time() + timeout
    last: list[dict[str, Any]] = []
    while time.time() < deadline:
        evidence = collector.collect_tmux_evidence(
            socket=str(socket_path),
            owner={"kind": "session_bank", "ref": "", "mayRestart": True},
            owner_home=str(owner_home),
            unix_user=pwd.getpwuid(os.geteuid()).pw_name,
        )
        last = evidence
        for item in evidence:
            for process in item.get("processTree", []):
                if process.get("argv") == hermes_argv:
                    return
        time.sleep(0.2)
    raise SmokeFailure(f"Hermes fixture argv not observed exactly; last evidence={last}")


def tmux(*args: str, socket_path: Path, tmux_bin: str = "tmux", state: SmokeState | None = None, check: bool = True) -> CommandResult:
    return run_command([tmux_bin, "-S", str(socket_path), *args], timeout=30, check=check, state=state)


def tmux_server_pid(socket_path: Path, tmux_bin: str, state: SmokeState | None = None) -> int:
    """Read the server pid through the exact disposable socket."""
    result = tmux(
        "display-message",
        "-p",
        "#{pid}",
        socket_path=socket_path,
        tmux_bin=tmux_bin,
        state=state,
    )
    raw = result.stdout.strip()
    if not re.fullmatch(r"[1-9]\d*", raw):
        raise SmokeFailure(f"disposable tmux socket {socket_path} returned invalid server pid: {raw!r}")
    return int(raw)


def _pid_is_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def stop_tmux_server(
    socket_path: Path,
    recorded_pid: int,
    tmux_bin: str,
    state: SmokeState | None = None,
    *,
    timeout: float = 2.0,
    poll_interval: float = 0.05,
) -> None:
    """Stop only the recorded disposable server pid and prove it and its socket disappeared."""
    current_pid = tmux_server_pid(socket_path, tmux_bin, state)
    if current_pid != recorded_pid:
        raise SmokeFailure(
            f"disposable socket reports pid {current_pid}, not recorded pid {recorded_pid}; "
            "refusing to signal an unproved process"
        )

    os.kill(recorded_pid, signal.SIGTERM)
    deadline = time.monotonic() + timeout
    while True:
        alive = _pid_is_alive(recorded_pid)
        socket_exists = socket_path.exists()
        if not alive:
            if socket_exists:
                try:
                    socket_path.unlink()
                except FileNotFoundError:
                    pass
                except OSError as exc:
                    raise SmokeFailure(
                        f"disposable tmux server pid {recorded_pid} exited but its stale "
                        f"socket {socket_path} could not be removed: {exc}"
                    ) from exc
            if not socket_path.exists():
                return
            raise SmokeFailure(
                f"disposable tmux server pid {recorded_pid} exited but socket {socket_path} remained"
            )
        now = time.monotonic()
        if now >= deadline:
            break
        time.sleep(min(poll_interval, deadline - now))

    if alive:
        raise SmokeFailure(
            f"disposable tmux server pid {recorded_pid} survived SIGTERM after {timeout:.3f}s; "
            "the smoke would otherwise verify a recovery that never happened"
        )


@dataclass
class DisposableTmuxServer:
    socket_path: Path
    tmux_bin: str
    state: SmokeState | None = None
    pid: int | None = None
    _may_be_running: bool = False

    def capture(self) -> int:
        self._may_be_running = True
        self.pid = tmux_server_pid(self.socket_path, self.tmux_bin, self.state)
        return self.pid

    def stop(self) -> None:
        if self.pid is None:
            if self._may_be_running:
                raise SmokeFailure(
                    f"disposable tmux server started at {self.socket_path}, but its pid was not recorded; "
                    "preserving the socket root for fail-loud recovery"
                )
            return
        stop_tmux_server(self.socket_path, self.pid, self.tmux_bin, self.state)
        self.pid = None
        self._may_be_running = False

    def remove_temp_root(self, temp_root: Path) -> None:
        if self.pid is None and not self._may_be_running:
            shutil.rmtree(temp_root, ignore_errors=True)


def start_initial_tmux_topology(
    *,
    server: DisposableTmuxServer,
    session_name: str,
    hermes_argv: list[str],
    hermes_cwd: Path,
    http_cwd: Path,
    site_dir: Path,
    helper_port: int,
    state: SmokeState,
) -> None:
    server.socket_path.parent.mkdir(parents=True, exist_ok=True)
    tmux(
        "new-session",
        "-d",
        "-s",
        session_name,
        "-n",
        "agents",
        "-c",
        str(hermes_cwd),
        shlex.join(hermes_argv),
        socket_path=server.socket_path,
        tmux_bin=server.tmux_bin,
        state=state,
    )
    server.capture()
    tmux(
        "new-window",
        "-d",
        "-t",
        session_name,
        "-n",
        "web",
        "-c",
        str(http_cwd),
        shlex.join(["python3", "-m", "http.server", str(helper_port), "--bind", "127.0.0.1", "--directory", str(site_dir)]),
        socket_path=server.socket_path,
        tmux_bin=server.tmux_bin,
        state=state,
    )


def start_extra_tmux_session(*, server: DisposableTmuxServer, session_name: str, cwd: Path, state: SmokeState) -> None:
    tmux(
        "new-session",
        "-d",
        "-s",
        session_name,
        "-n",
        "keep",
        "-c",
        str(cwd),
        "sleep 300",
        socket_path=server.socket_path,
        tmux_bin=server.tmux_bin,
        state=state,
    )
    server.capture()


def parse_cli_json(result: CommandResult) -> dict[str, Any]:
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise SmokeFailure(f"CLI returned invalid JSON: {result.stdout[-1000:]}") from exc


def session_by_name(sessions: list[dict[str, Any]], name: str) -> dict[str, Any]:
    for session in sessions:
        if session.get("sessionName") == name:
            return session
    raise SmokeFailure(f"missing session {name!r}")


def descriptor_by_kind(session: dict[str, Any], kind: str) -> dict[str, Any]:
    for desc in session.get("descriptors", []):
        if desc.get("workloadKind") == kind:
            return desc
    raise SmokeFailure(f"missing descriptor kind {kind!r} in {session.get('sessionName')!r}")


def pane_ids(session: dict[str, Any]) -> list[str]:
    ids = []
    for desc in session.get("descriptors", []):
        topology = desc.get("topology") if isinstance(desc.get("topology"), dict) else {}
        ids.append(str(topology.get("paneId", "")))
    return ids


def live_pane_pids(socket_path: Path, session_name: str, tmux_bin: str, state: SmokeState | None = None) -> set[str]:
    """OS pids backing a session's panes, straight from tmux.

    Pane *ids* cannot answer "are these new processes?". tmux allocates them monotonically per
    server and restore rebuilds on a fresh server, so the counter restarts at 0 and old and new
    ids collide whenever the snapshotted session happened to hold the first panes. Pids are the
    thing the check actually means, and they do not depend on creation order.
    """
    result = tmux(
        "list-panes",
        "-t",
        session_name,
        "-F",
        "#{pane_pid}",
        socket_path=socket_path,
        tmux_bin=tmux_bin,
        state=state,
    )
    return {line.strip() for line in result.stdout.splitlines() if line.strip()}


def assert_pane_processes_recreated(before: set[str], after: set[str]) -> int:
    survivors = before & after
    if survivors:
        raise SmokeFailure(f"restored panes reuse pre-kill processes: {sorted(survivors)}")
    return len(before | after)


def find_manifest_path(snapshot_result: dict[str, Any]) -> Path:
    raw = snapshot_result.get("manifestPath")
    if not isinstance(raw, str) or not raw:
        raise SmokeFailure(f"snapshot result missing manifestPath: {snapshot_result}")
    return Path(raw)


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def maybe_start_systemd_unit(
    *,
    unit_base: str,
    unix_user: str,
    owner_home: Path,
    cleanup: CleanupStack,
    state: SmokeState,
) -> dict[str, Any] | None:
    if not shutil.which("systemd-run") or not shutil.which("systemctl"):
        state.skips["managed_systemd"] = "systemd-run or systemctl is unavailable"
        return None
    unit = unit_base + ".service"
    cmd = [
        "systemd-run",
        "--user",
        f"--unit={unit_base}",
        "--property=RuntimeMaxSec=180",
        "--collect",
        "sleep",
        "300",
    ]
    started = run_command(cmd, timeout=15, check=False, state=state)
    if started.returncode != 0:
        state.skips["managed_systemd"] = (started.stderr or started.stdout or "systemd-run failed").strip()[:240]
        return None

    def stop_unit() -> None:
        run_command(["systemctl", "--user", "stop", unit], timeout=15, check=False, state=state)
        run_command(["systemctl", "--user", "reset-failed", unit], timeout=15, check=False, state=state)

    cleanup.add(stop_unit)
    deadline = time.time() + 10
    last = ""
    while time.time() < deadline:
        status = run_command(
            ["systemctl", "--user", "show", unit, "--property=ActiveState", "--value"],
            timeout=5,
            check=False,
            state=state,
        )
        last = (status.stdout or status.stderr).strip()
        if status.returncode == 0 and last == "active":
            state.checks["managed_systemd_started"] = {"status": "pass", "unit": unit}
            return managed_systemd_record(session_name=unit_base.replace("-", "_"), unix_user=unix_user, unit=unit, owner_home=owner_home)
        time.sleep(0.2)
    state.skips["managed_systemd"] = f"transient unit did not become active: {last}"
    stop_unit()
    return None


def verify_api_persisted_manifest(api_url: str, unix_user: str, session_name: str, manifest_doc: dict[str, Any]) -> dict[str, Any]:
    payload = http_json(api_url + "/api/tmux/sessions?" + parse.urlencode({"unixUser": unix_user}))
    banked = payload.get("banked", [])
    api_entry = None
    for entry in banked:
        if entry.get("name") == session_name and entry.get("unixUser", "") == unix_user:
            api_entry = entry
            break
    if api_entry is None:
        raise SmokeFailure(f"API did not return banked session {session_name!r}")
    manifest_session = session_by_name(manifest_doc["sessions"], session_name)
    api_plan = api_entry.get("recoveryPlan")
    if api_plan != manifest_session.get("descriptors"):
        raise SmokeFailure("API recoveryPlan does not match manifest descriptors")
    return api_entry


def verify_api_managed_status(api_url: str, unix_user: str, managed_record: dict[str, Any], managed_status_path: Path) -> dict[str, Any]:
    mode = stat.S_IMODE(managed_status_path.stat().st_mode)
    if mode != 0o600:
        raise SmokeFailure(f"managed status mode = {oct(mode)}, want 0o600")
    payload = http_json(api_url + "/api/tmux/sessions?" + parse.urlencode({"unixUser": unix_user}))
    name = managed_record["sessionName"]
    for bank_entry in payload.get("banked", []):
        if bank_entry.get("name") == name and bank_entry.get("unixUser") == unix_user:
            raise SmokeFailure("managed registry entry leaked into Session Bank")
    for entry in payload.get("managed", []):
        if entry.get("name") != name or entry.get("unixUser") != unix_user:
            continue
        forbidden_keys = {"descriptors", "statusProbe", "restartAllowed", "recoveryPlan", "argv", "env"}
        if forbidden_keys.intersection(entry):
            raise SmokeFailure(f"managed status exposed forbidden keys: {entry}")
        status = entry.get("status", {})
        if status.get("ok") is not True or status.get("activeState") != "active":
            raise SmokeFailure(f"managed status not active: {entry}")
        if entry.get("owner") != managed_record.get("owner"):
            raise SmokeFailure(f"managed owner mismatch: {entry}")
        if entry.get("managerRef") != managed_record.get("managerRef") or entry.get("managerKind") != managed_record.get("managerKind"):
            raise SmokeFailure(f"managed manager mismatch: {entry}")
        if entry.get("storageKind") != "managed-status" or entry.get("sourceKind") != "restore":
            raise SmokeFailure(f"managed source metadata mismatch: {entry}")
        return {"name": name, "status": status.get("activeState"), "mode": "0600"}
    raise SmokeFailure(f"API did not return managed registry entry {name!r}: {payload}")


def manifest_has_separate_verification(manifest_doc: dict[str, Any], session_name: str) -> dict[str, Any]:
    session = session_by_name(manifest_doc["sessions"], session_name)
    verification = session.get("verification", [])
    if len(verification) < 2:
        raise SmokeFailure("manifest missing separate pane verification records")
    if not any(record.get("helperEndpoint", {}).get("kind") == "http-get" for record in verification if isinstance(record, dict)):
        raise SmokeFailure("manifest missing loopback helper probe")
    for desc in session.get("descriptors", []):
        if "paneStatus" in desc or "helperEndpoint" in desc:
            raise SmokeFailure("operator verification leaked into Go descriptors")
    return {"records": len(verification), "helperProbe": True}


def mutate_wrong_hermes_identity(manifest_doc: dict[str, Any], session_name: str, wrong_native_id: str) -> dict[str, Any]:
    mutated = copy.deepcopy(manifest_doc)
    session = session_by_name(mutated["sessions"], session_name)
    hermes = descriptor_by_kind(session, "hermes")
    hermes["agent"]["nativeSessionId"] = wrong_native_id
    manifest_lib.validate_manifest(mutated)
    return mutated


def run_smoke(args: argparse.Namespace) -> SmokeState:
    state = SmokeState()
    cleanup = CleanupStack()
    temp_root = Path(args.temp_root) if args.temp_root else Path("/tmp") / f"ctx-sh7-5-{uuid.uuid4().hex[:12]}"
    if temp_root.exists():
        raise SmokeFailure(f"temp root already exists: {temp_root}")
    temp_root.mkdir(parents=True, mode=0o700)
    disposable_tmux_server: DisposableTmuxServer | None = None

    def remove_temp_root() -> None:
        if disposable_tmux_server is None:
            shutil.rmtree(temp_root, ignore_errors=True)
            return
        disposable_tmux_server.remove_temp_root(temp_root)

    cleanup.add(remove_temp_root)
    try:
        unix_user = pwd.getpwuid(os.geteuid()).pw_name
        tmux_bin = find_required_tool("tmux")
        find_required_tool("go")
        find_required_tool("python3")

        suffix = uuid.uuid4().hex[:8]
        session_name = f"ctxsh75_{suffix}"
        extra_session = f"ctxsh75_extra_{suffix}"
        unit_base = f"ctx-sh7-5-{suffix}"
        fake_home = temp_root / "fake-home"
        work_dir = fake_home / "work"
        hermes_cwd = work_dir / "hermes"
        http_cwd = work_dir / "web"
        site_dir = fake_home / "site"
        tmux_socket = temp_root / "tmux" / "ctx-sh7-5.sock"
        disposable_tmux_server = DisposableTmuxServer(tmux_socket, tmux_bin, state)
        manifest_dir = temp_root / "manifests"
        managed_records_path = temp_root / "managed-records.json"
        managed_status_path = temp_root / "data" / "tmux-recovery" / "managed-status.json"
        server_log = temp_root / "chrote-server.log"
        api_port = choose_loopback_port()
        ttyd_port = choose_loopback_port()
        helper_port = choose_loopback_port()
        native_id = f"hermes-smoke-{suffix}"
        wrong_native_id = f"hermes-wrong-{suffix}"
        api_url = f"http://127.0.0.1:{api_port}"

        for path in [fake_home, work_dir, hermes_cwd, http_cwd, site_dir, manifest_dir]:
            path.mkdir(parents=True, exist_ok=True)
        (site_dir / "index.html").write_text("ctx-sh7.5 helper ok\n", encoding="utf-8")
        hermes_argv = hermes_resume_argv(fake_home, "smoke", native_id)
        compile_hermes_fixture(fake_home, state=state)

        state.resources.update(
            {
                "tempRoot": str(temp_root),
                "tmuxSocket": str(tmux_socket),
                "apiUrl": api_url,
                "ttydPort": ttyd_port,
                "helperPort": helper_port,
                "managedStatusPath": str(managed_status_path),
                "sessionName": session_name,
                "extraSessionName": extra_session,
                "hermesNativeId": native_id,
                "wrongHermesNativeId": wrong_native_id,
                "fakeOwnerHome": str(fake_home),
            }
        )

        binary = build_server(temp_root, state=state)
        server_env = os.environ.copy()
        server_env.update(
            {
                "HOME": str(fake_home),
                "HOST": "127.0.0.1",
                "PORT": str(api_port),
                "TTYD_PORT": str(ttyd_port),
                "CHROTE_ROOTS": f"{fake_home},{work_dir}",
                "CHROTE_WORKDIR": str(work_dir),
                "CHROTE_DEFAULT_TMUX_SOCKET": str(tmux_socket),
                "CHROTE_DEFAULT_TMUX_WORKDIR": str(work_dir),
                "CHROTE_TERMINAL_USERS": unix_user,
                "CHROTE_TERMINAL_USER_SOCKETS": f"{unix_user}={tmux_socket}",
                "CHROTE_TERMINAL_USER_WORKDIRS": f"{unix_user}={work_dir}",
                "CHROTE_TERMINAL_USER_HOMES": f"{unix_user}={fake_home}",
                "CHROTE_SESSION_BANK_PATH": str(temp_root / "data" / "session-bank" / "sessions.json"),
                "CHROTE_MANAGED_RECOVERY_STATUS_PATH": str(managed_status_path),
                "CHROTE_PERSISTENT_AGENTS_PATH": str(temp_root / "data" / "persistent-agents" / "agents.json"),
                "CHROTE_SESSION_DROPS_DIR": str(temp_root / "data" / "session-drops"),
                "CHROTE_PERSISTENT_AGENTS_DISABLE": "true",
            }
        )
        proc = start_server(binary, server_env, server_log, cleanup)
        wait_health(api_url, proc)
        state.checks["feature_server_health"] = {"status": "pass", "url": api_url + "/api/health"}

        cleanup.add(disposable_tmux_server.stop)
        start_initial_tmux_topology(
            server=disposable_tmux_server,
            session_name=session_name,
            hermes_argv=hermes_argv,
            hermes_cwd=hermes_cwd,
            http_cwd=http_cwd,
            site_dir=site_dir,
            helper_port=helper_port,
            state=state,
        )
        state.resources["initialTmuxServerPid"] = disposable_tmux_server.pid
        wait_exact_hermes_argv(tmux_socket, fake_home, hermes_argv)
        wait_http_ok(f"http://127.0.0.1:{helper_port}/")
        state.checks["initial_tmux_workloads"] = {"status": "pass", "hermesArgv": hermes_argv, "helperHTTP": 200}

        managed_record = maybe_start_systemd_unit(
            unit_base=unit_base,
            unix_user=unix_user,
            owner_home=fake_home,
            cleanup=cleanup,
            state=state,
        )
        snapshot_argv = [
            sys.executable,
            str(SCRIPT_DIR / "snapshot.py"),
            "--api-url",
            api_url,
            "--socket",
            str(tmux_socket),
            "--unix-user",
            unix_user,
            "--owner-home",
            str(fake_home),
            "--owner-kind",
            "session_bank",
            "--owner-may-restart",
            "--output-dir",
            str(manifest_dir),
        ]
        if managed_record is not None:
            write_json(managed_records_path, [managed_record])
            snapshot_argv.extend(["--managed-records", str(managed_records_path)])
        snapshot_result = parse_cli_json(run_command(snapshot_argv, cwd=REPO_ROOT, timeout=60, state=state))
        manifest_path = find_manifest_path(snapshot_result)
        manifest_doc = read_json(manifest_path)
        manifest_mode = stat.S_IMODE(manifest_path.stat().st_mode)
        if manifest_mode != 0o600:
            raise SmokeFailure(f"manifest mode = {oct(manifest_mode)}, want 0o600")
        api_entry = verify_api_persisted_manifest(api_url, unix_user, session_name, manifest_doc)
        verification_summary = manifest_has_separate_verification(manifest_doc, session_name)
        expected_session = session_by_name(manifest_doc["sessions"], session_name)
        state.checks["snapshot_api_manifest"] = {
            "status": "pass",
            "manifestPath": str(manifest_path),
            "manifestMode": "0600",
            "postedSessions": snapshot_result.get("postedSessions"),
            "apiRecoveryKind": api_entry.get("recoveryKind"),
            "verification": verification_summary,
        }

        pane_pids_before = live_pane_pids(tmux_socket, session_name, tmux_bin, state)
        if not pane_pids_before:
            raise SmokeFailure(f"no pane pids observed for {session_name!r} before the kill")

        stopped_pid = disposable_tmux_server.pid
        disposable_tmux_server.stop()
        state.checks["disposable_tmux_server_stopped"] = {
            "status": "pass",
            "pid": stopped_pid,
            "signal": "SIGTERM",
            "timeoutSeconds": 2,
        }
        start_extra_tmux_session(
            server=disposable_tmux_server,
            session_name=extra_session,
            cwd=work_dir,
            state=state,
        )
        state.resources["recoveryTmuxServerPid"] = disposable_tmux_server.pid
        state.checks["post_snapshot_extra_session"] = {"status": "pass", "sessionName": extra_session}

        restore_result = run_command(
            [
                sys.executable,
                str(SCRIPT_DIR / "restore.py"),
                "--api-url",
                api_url,
                "--manifest",
                str(manifest_path),
                "--socket",
                str(tmux_socket),
                "--unix-user",
                unix_user,
                "--owner-home",
                str(fake_home),
                "--owner-kind",
                "session_bank",
                "--owner-may-restart",
                "--readiness-seconds",
                "10",
                "--stability-seconds",
                "2",
                "--managed-status-output",
                str(managed_status_path),
            ],
            cwd=REPO_ROOT,
            timeout=90,
            state=state,
        )
        restore_payload = parse_cli_json(restore_result)
        if restore_payload.get("ok") is not True:
            raise SmokeFailure(f"restore failed: {restore_payload}")
        for key in ("readinessSeconds", "readinessSamples", "stabilitySeconds", "stabilitySamples"):
            if restore_payload.get(key) is None:
                raise SmokeFailure(f"restore result missing {key}: {restore_payload}")
        state.checks["restore_cli_api_stability"] = {
            "status": "pass",
            "readinessSeconds": restore_payload.get("readinessSeconds"),
            "readinessSamples": restore_payload.get("readinessSamples"),
            "stabilitySeconds": restore_payload.get("stabilitySeconds"),
            "stabilitySamples": restore_payload.get("stabilitySamples"),
            "sessions": [item.get("sessionName") for item in restore_payload.get("sessions", [])],
        }

        observed = collect_sessions(tmux_socket, unix_user, fake_home)
        observed_session = session_by_name(observed, session_name)
        observed_hermes = descriptor_by_kind(observed_session, "hermes")
        observed_python = descriptor_by_kind(observed_session, "python-http-server")
        agent = observed_hermes.get("agent", {})
        if agent.get("nativeSessionId") != native_id or agent.get("hermesProfile") != "smoke":
            raise SmokeFailure(f"restored Hermes identity mismatch: {agent}")
        if observed_python.get("command", {}).get("pythonHTTPServer", {}).get("port") != helper_port:
            raise SmokeFailure(f"restored helper descriptor mismatch: {observed_python}")
        wait_http_ok(f"http://127.0.0.1:{helper_port}/")
        sessions_after = tmux("list-sessions", "-F", "#{session_name}", socket_path=tmux_socket, tmux_bin=tmux_bin, state=state).stdout.splitlines()
        if extra_session not in sessions_after:
            raise SmokeFailure(f"extra session was not preserved: {sessions_after}")
        if managed_record is not None and managed_record["sessionName"] in sessions_after:
            raise SmokeFailure("managed owner was recreated as a tmux session")
        if managed_record is not None:
            state.checks["managed_status_registry"] = {
                "status": "pass",
                **verify_api_managed_status(api_url, unix_user, managed_record, managed_status_path),
            }
        old_panes = pane_ids(expected_session)
        new_panes = pane_ids(observed_session)
        pane_pids_after = live_pane_pids(tmux_socket, session_name, tmux_bin, state)
        pane_pids_replaced = assert_pane_processes_recreated(pane_pids_before, pane_pids_after)
        state.checks["restored_topology_workloads"] = {
            "status": "pass",
            "oldPaneIds": old_panes,
            "newPaneIds": new_panes,
            "panePidsReplaced": pane_pids_replaced,
            "hermesProfile": agent.get("hermesProfile"),
            "helperHTTP": 200,
            "extraSessionPreserved": True,
            "managedNotRecreated": managed_record is not None,
        }

        wrong_manifest = mutate_wrong_hermes_identity(manifest_doc, session_name, wrong_native_id)
        wrong_manifest_path = temp_root / "wrong-identity-manifest.json"
        write_json(wrong_manifest_path, wrong_manifest)
        negative = run_command(
            [
                sys.executable,
                str(SCRIPT_DIR / "verify.py"),
                "--manifest",
                str(wrong_manifest_path),
                "--socket",
                str(tmux_socket),
                "--unix-user",
                unix_user,
                "--owner-home",
                str(fake_home),
                "--owner-kind",
                "session_bank",
                "--owner-may-restart",
                "--readiness-seconds",
                "0",
                "--stability-seconds",
                "0",
            ],
            cwd=REPO_ROOT,
            timeout=60,
            check=False,
            state=state,
        )
        if negative.returncode == 0:
            raise SmokeFailure("wrong valid Hermes identity unexpectedly verified")
        negative_text = negative.stdout + negative.stderr
        if "missing expected descriptor" not in negative_text:
            raise SmokeFailure(f"wrong identity failure did not mention descriptor mismatch: {negative_text[-1000:]}")
        state.checks["negative_wrong_hermes_identity"] = {"status": "pass", "returnCode": negative.returncode}

        assert_no_forbidden_references(
            [
                state.resources,
                state.checks,
                state.skips,
                state.command_log,
                manifest_doc,
                restore_payload,
                negative.stdout,
                negative.stderr,
                {
                    "sessionBank": server_env["CHROTE_SESSION_BANK_PATH"],
                    "managedStatus": server_env["CHROTE_MANAGED_RECOVERY_STATUS_PATH"],
                    "persistentAgents": server_env["CHROTE_PERSISTENT_AGENTS_PATH"],
                    "sessionDrops": server_env["CHROTE_SESSION_DROPS_DIR"],
                },
            ],
            temp_root=temp_root,
        )
        state.checks["forbidden_live_references"] = {"status": "pass"}
        return state
    finally:
        cleanup.close()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--temp-root", help="Optional unique /tmp root for diagnostics; removed on cleanup")
    args = parser.parse_args(argv)
    started = time.time()
    payload: dict[str, Any] = {"ok": False, "checks": {}, "skips": {}, "resources": {}}
    try:
        state = run_smoke(args)
        payload.update(
            {
                "ok": True,
                "checks": state.checks,
                "skips": state.skips,
                "resources": state.resources,
                "durationSeconds": round(time.time() - started, 3),
            }
        )
        code = 0
    except SmokeBlocker as exc:
        payload.update({"ok": False, "blocker": str(exc), "durationSeconds": round(time.time() - started, 3)})
        code = 2
    except Exception as exc:
        payload.update({"ok": False, "error": str(exc), "durationSeconds": round(time.time() - started, 3)})
        code = 1
    print(json.dumps(payload, sort_keys=True))
    return code


if __name__ == "__main__":
    raise SystemExit(main())
