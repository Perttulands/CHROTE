#!/usr/bin/env python3
"""Disposable real tmux/user-systemd acceptance for ADR-0014.

The default lane uses a private socket and a uniquely named transient unit in
the current user's manager. Cross-user acceptance runs only when explicitly
named and when the installed template, helper, grant, state roots, and root
fixture authority are all present. Missing prerequisites are reported as named
GATED scenarios; --require-cross-user turns that gate into a non-zero result.

No CHROTE service is started or restarted. Cleanup kills only the two uniquely
named tmux sessions; it never invokes tmux kill-server.
"""

from __future__ import annotations

import argparse
from dataclasses import asdict, dataclass
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
from typing import Callable
import uuid


REPO_ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = REPO_ROOT / "scripts" / "chrote-agent-ensure.sh"
HELPER_SOURCE = REPO_ROOT / "scripts" / "chrote-agentctl"
SUDOERS_SOURCE = REPO_ROOT / "services" / "chrote-agentctl.sudoers"
UNIT_SOURCE = REPO_ROOT / "services" / "chrote-agent@.service"
INSTALLED_LAUNCHER = Path("/usr/local/lib/chrote/chrote-agent-ensure.sh")
INSTALLED_HELPER = Path("/usr/local/libexec/chrote/chrote-agentctl")
INSTALLED_SUDOERS = Path("/etc/sudoers.d/chrote-agentctl")
STATE_ENV = Path("/etc/chrote/chrote-agent-state.env")
SESSION_ID = "019f4baa-e368-7ea0-8912-fb2c6f99785c"
SAFE_PATH = re.compile(r"^/[A-Za-z0-9._/-]+$")
ROOT_PREFIX = "chrote-persistence-v2-"
TRUSTED_TMUX = Path("/usr/bin/tmux")


class AcceptanceFailure(RuntimeError):
    pass


@dataclass(frozen=True)
class ScenarioResult:
    name: str
    status: str
    detail: str
    evidence: dict[str, object] | None = None


@dataclass(frozen=True)
class CommandResult:
    argv: list[str]
    returncode: int
    stdout: str
    stderr: str
    elapsed_seconds: float


def run(
    argv: list[str],
    *,
    env: dict[str, str] | None = None,
    timeout: float = 20,
    check: bool = True,
) -> CommandResult:
    started = time.monotonic()
    completed = subprocess.run(
        argv,
        env=env,
        text=True,
        capture_output=True,
        check=False,
        timeout=timeout,
    )
    result = CommandResult(
        argv=list(argv),
        returncode=completed.returncode,
        stdout=completed.stdout,
        stderr=completed.stderr,
        elapsed_seconds=round(time.monotonic() - started, 3),
    )
    if check and result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        raise AcceptanceFailure(f"command failed ({result.returncode}): {argv!r}: {detail}")
    return result


def wait_for(description: str, predicate: Callable[[], bool], timeout: float = 20) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if predicate():
            return
        time.sleep(0.1)
    raise AcceptanceFailure(f"timed out waiting for {description}")


def validate_safe_path(path: Path, label: str) -> None:
    raw = str(path)
    if not path.is_absolute() or not SAFE_PATH.fullmatch(raw) or Path(os.path.normpath(raw)) != path:
        raise AcceptanceFailure(f"{label} is not a canonical safe absolute path: {raw}")


def validate_cleanup_root(root: Path, *, expected_uid: int | None = None) -> None:
    if root.is_symlink():
        raise AcceptanceFailure(f"cleanup root must not be a symlink: {root}")
    try:
        resolved = root.resolve(strict=True)
    except FileNotFoundError as exc:
        raise AcceptanceFailure(f"cleanup root does not exist: {root}") from exc
    if not resolved.name.startswith(ROOT_PREFIX):
        raise AcceptanceFailure(f"cleanup root is not harness-owned: {resolved}")
    if resolved == Path("/tmp") or Path("/tmp") not in resolved.parents:
        raise AcceptanceFailure(f"cleanup root is outside /tmp: {resolved}")
    info = resolved.stat()
    owner = os.geteuid() if expected_uid is None else expected_uid
    if info.st_uid != owner or not stat.S_ISDIR(info.st_mode):
        raise AcceptanceFailure(f"cleanup root ownership/type is unsafe: {resolved}")


def tmux_cleanup_argv(tmux: str, socket: str, session: str) -> list[str]:
    return [tmux, "-S", socket, "kill-session", "-t", f"={session}"]


def trusted_tmux() -> str:
    if not TRUSTED_TMUX.is_file():
        raise AcceptanceFailure(f"trusted tmux binary is unavailable: {TRUSTED_TMUX}")
    return str(TRUSTED_TMUX)


def dead_manager_environment(root: Path) -> dict[str, str]:
    runtime = root / "dead-runtime"
    return {
        **os.environ,
        "XDG_RUNTIME_DIR": str(runtime),
        "DBUS_SESSION_BUS_ADDRESS": f"unix:path={runtime / 'bus'}",
    }


def result_exit_code(results: list[ScenarioResult], *, require_cross_user: bool) -> int:
    if any(result.status == "FAIL" for result in results):
        return 1
    if require_cross_user and any(
        result.name == "cross_user_lock" and result.status == "GATED" for result in results
    ):
        return 2
    return 0


def cross_user_prerequisite_result(
    *,
    target_user: str | None,
    helper: Path,
    sudoers: Path,
    template_loaded: bool,
    is_root: bool,
) -> ScenarioResult:
    blockers: list[str] = []
    if not target_user:
        blockers.append("pass --cross-user USER")
    if not is_root:
        blockers.append("run as root to create and clean a private target-owned fixture")
    if not helper.is_file():
        blockers.append(f"installed helper missing: {helper}")
    if not sudoers.is_file():
        blockers.append(f"installed sudoers grant missing: {sudoers}")
    if not template_loaded:
        blockers.append("installed chrote-agent@.service template is not loaded")
    if blockers:
        return ScenarioResult("cross_user_lock", "GATED", "; ".join(blockers))
    return ScenarioResult("cross_user_lock", "READY", "cross-user prerequisites are present")


def read_json_lines(path: Path) -> list[dict[str, object]]:
    if not path.exists():
        return []
    records: list[dict[str, object]] = []
    for line in path.read_text().splitlines():
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            records.append(value)
    return records


def read_receipt(path: Path) -> dict[str, object]:
    try:
        value = json.loads(path.read_text())
    except (FileNotFoundError, json.JSONDecodeError):
        return {}
    return value if isinstance(value, dict) else {}


def process_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except (ProcessLookupError, PermissionError):
        return False
    return True


def write_agent_fixture(root: Path) -> tuple[Path, Path]:
    launch_log = root / "agent-launches.jsonl"
    agent = root / "agent-bin"
    agent.write_text(
        "#!/usr/bin/python3\n"
        "import json, os, signal, sys, time\n"
        f"log = {str(launch_log)!r}\n"
        "with open(log, 'a', encoding='utf-8') as handle:\n"
        "    handle.write(json.dumps({'pid': os.getpid(), 'argv': sys.argv[1:]}) + '\\n')\n"
        "    handle.flush()\n"
        "def stop(_signum, _frame):\n"
        "    raise SystemExit(0)\n"
        "signal.signal(signal.SIGTERM, stop)\n"
        "signal.signal(signal.SIGINT, stop)\n"
        "while True:\n"
        "    time.sleep(1)\n"
    )
    agent.chmod(0o755)
    return agent, launch_log


def write_agent_config(
    path: Path,
    *,
    session: str,
    tmux: str,
    socket: Path,
    workdir: Path,
    agent: Path,
    receipt: Path,
) -> None:
    values = {
        "CHROTE_AGENT_SESSION": session,
        "CHROTE_AGENT_KIND": "codex",
        "CHROTE_AGENT_SESSION_ID": SESSION_ID,
        "CHROTE_AGENT_BIN": str(agent),
        "CHROTE_AGENT_TMUX_BIN": tmux,
        "CHROTE_AGENT_TMUX_SOCKET": str(socket),
        "CHROTE_AGENT_WORKDIR": str(workdir),
        "CHROTE_AGENT_RECEIPT_PATH": str(receipt),
        "CHROTE_AGENT_WATCH_INTERVAL": "1",
        "CHROTE_AGENT_TMUX_KEEPER_UNIT": "disposable-acceptance-keeper.service",
    }
    for key, value in values.items():
        if "\n" in value or "\r" in value or (key.endswith(("BIN", "SOCKET", "WORKDIR", "PATH")) and not SAFE_PATH.fullmatch(value)):
            raise AcceptanceFailure(f"unsafe fixture value for {key}")
    path.write_text("".join(f"{key}={value}\n" for key, value in values.items()))
    path.chmod(0o640)


def tmux_session_exists(tmux: str, socket: Path, session: str) -> bool:
    return run(
        [tmux, "-S", str(socket), "has-session", "-t", f"={session}"],
        check=False,
        timeout=3,
    ).returncode == 0


def tmux_server_running(tmux: str, socket: Path) -> bool:
    return run(
        [tmux, "-S", str(socket), "list-sessions"],
        check=False,
        timeout=3,
    ).returncode == 0


def start_keeper(tmux: str, socket: Path, keeper: str, workdir: Path, *, prefix: list[str] | None = None) -> None:
    argv = [tmux, "-S", str(socket), "new-session", "-d", "-s", keeper, "-c", str(workdir), "/usr/bin/sleep", "600"]
    run([*(prefix or []), *argv])


def stop_user_unit(unit: str) -> None:
    run(["/usr/bin/systemctl", "--user", "stop", unit], check=False, timeout=10)


def same_user_scenarios(root: Path) -> list[ScenarioResult]:
    tmux = trusted_tmux()
    validate_safe_path(Path(tmux), "tmux binary")
    validate_safe_path(LAUNCHER, "launcher")
    manager = run(["/usr/bin/systemctl", "--user", "is-system-running"], check=False, timeout=5)
    if manager.stdout.strip() not in {"running", "degraded"}:
        raise AcceptanceFailure(f"current user manager is unavailable: {(manager.stderr or manager.stdout).strip()}")
    if not Path("/usr/bin/systemd-notify").is_file():
        raise AcceptanceFailure("/usr/bin/systemd-notify is unavailable")

    suffix = root.name.removeprefix(ROOT_PREFIX)
    session = f"accept-agent-{suffix}"
    keeper = f"accept-keeper-{suffix}"
    unit = f"chrote-agent-accept-{suffix}.service"
    socket = root / "tmux.sock"
    receipt = root / "agent.receipt.json"
    config = root / "agent.conf"
    agent, launch_log = write_agent_fixture(root)
    write_agent_config(
        config,
        session=session,
        tmux=tmux,
        socket=socket,
        workdir=root,
        agent=agent,
        receipt=receipt,
    )
    unit_start_argv = [
        "/usr/bin/systemd-run", "--user", f"--unit={unit}",
        "--property=Type=notify", "--property=NotifyAccess=all",
        "--property=TimeoutStartSec=15s", "--property=Restart=on-failure",
        "--property=RestartSec=1s", "--property=StartLimitIntervalSec=30s",
        "--property=StartLimitBurst=10", "--property=KillMode=process",
        str(LAUNCHER), "--config", str(config),
    ]

    start_keeper(tmux, socket, keeper, root)
    try:
        run(unit_start_argv, timeout=20)
        wait_for("initial agent and receipt", lambda: len(read_json_lines(launch_log)) >= 1 and bool(read_receipt(receipt)))
        first = read_json_lines(launch_log)[0]
        first_receipt = read_receipt(receipt)
        if first.get("argv") != ["resume", SESSION_ID] or not tmux_session_exists(tmux, socket, session):
            raise AcceptanceFailure(f"initial same-user resume is wrong: {first}")

        first_pid = int(first["pid"])
        os.kill(first_pid, signal.SIGKILL)
        wait_for("agent-only death restart", lambda: len(read_json_lines(launch_log)) >= 2, timeout=15)
        second = read_json_lines(launch_log)[1]
        wait_for(
            "fresh receipt after agent death",
            lambda: read_receipt(receipt).get("invocationId") not in {None, first_receipt.get("invocationId")},
            timeout=15,
        )
        if second.get("argv") != ["resume", SESSION_ID] or int(second["pid"]) == first_pid:
            raise AcceptanceFailure(f"agent death did not resume the same native session: {second}")

        run(tmux_cleanup_argv(tmux, str(socket), session))
        wait_for("session-loss restart", lambda: len(read_json_lines(launch_log)) >= 3, timeout=15)
        third = read_json_lines(launch_log)[2]
        if third.get("argv") != ["resume", SESSION_ID] or not tmux_session_exists(tmux, socket, session):
            raise AcceptanceFailure(f"session loss did not resume the same native session: {third}")

        stop_user_unit(unit)
        third_pid = int(third["pid"])
        wait_for("unit stop", lambda: run(
            ["/usr/bin/systemctl", "--user", "is-active", unit], check=False, timeout=3
        ).returncode != 0)
        if not process_alive(third_pid) or not tmux_session_exists(tmux, socket, session):
            raise AcceptanceFailure("explicit unit stop killed the agent or tmux session")

        # Simulated reboot: the unit is stopped, both disposable sessions are
        # removed individually so their server exits naturally, the keeper is
        # restored first, and the same unit is started again.
        run(tmux_cleanup_argv(tmux, str(socket), session))
        run(tmux_cleanup_argv(tmux, str(socket), keeper))
        wait_for("private tmux server exit", lambda: not tmux_server_running(tmux, socket))
        start_keeper(tmux, socket, keeper, root)
        run(unit_start_argv, timeout=20)
        wait_for("simulated reboot resume", lambda: len(read_json_lines(launch_log)) >= 4, timeout=15)
        fourth = read_json_lines(launch_log)[3]
        if fourth.get("argv") != ["resume", SESSION_ID] or not tmux_session_exists(tmux, socket, session):
            raise AcceptanceFailure(f"simulated reboot resumed the wrong native session: {fourth}")
        state = run([
            "/usr/bin/systemctl", "--user", "show", unit,
            "--property=ActiveState", "--property=SubState", "--property=NRestarts",
        ])
        evidence = {
            "unit": unit,
            "session": session,
            "nativeSessionId": SESSION_ID,
            "launchPids": [int(record["pid"]) for record in read_json_lines(launch_log)[:4]],
            "unitState": state.stdout.strip().splitlines(),
        }
        return [
            ScenarioResult("same_user_lock", "PASS", "real user unit confirmed launcher readiness", evidence),
            ScenarioResult("agent_process_death", "PASS", "killing only pane_pid relaunched the same native session", evidence),
            ScenarioResult("session_loss", "PASS", "removing only the agent session triggered recreation", evidence),
            ScenarioResult("unlock_leaves_agent", "PASS", "explicit unit stop left the pane process alive", {"pid": third_pid}),
            ScenarioResult("simulated_reboot", "PASS", "keeper-first restart resumed the same native session", evidence),
        ]
    finally:
        stop_user_unit(unit)
        run(tmux_cleanup_argv(tmux, str(socket), session), check=False, timeout=5)
        run(tmux_cleanup_argv(tmux, str(socket), keeper), check=False, timeout=5)
        run(["/usr/bin/systemctl", "--user", "reset-failed", unit], check=False, timeout=5)


def dead_manager_scenarios(root: Path) -> list[ScenarioResult]:
    runtime = root / "dead-runtime"
    runtime.mkdir(mode=0o700)
    env = dead_manager_environment(root)
    unit = f"chrote-agent@dead-{root.name[-8:]}.service"
    show = run(
        ["/usr/bin/systemctl", "--user", "show", "--property=LoadState", unit],
        env=env,
        check=False,
        timeout=3,
    )
    disable = run(
        ["/usr/bin/systemctl", "--user", "disable", "--now", unit],
        env=env,
        check=False,
        timeout=3,
    )
    if show.returncode == 0:
        raise AcceptanceFailure("dead-user-manager status probe unexpectedly succeeded")
    if disable.returncode == 0:
        raise AcceptanceFailure("disable against a dead user manager unexpectedly succeeded")
    return [
        ScenarioResult(
            "dead_user_manager", "PASS", "real systemctl failed against an isolated absent bus",
            {"returnCode": show.returncode, "elapsedSeconds": show.elapsed_seconds},
        ),
        ScenarioResult(
            "failed_disable", "PASS", "real disable returned non-zero against the absent bus",
            {"returnCode": disable.returncode, "elapsedSeconds": disable.elapsed_seconds},
        ),
    ]


def template_loaded_for(target_user: str | None) -> bool:
    if not target_user or not INSTALLED_HELPER.is_file():
        return False
    result = run(
        [str(INSTALLED_HELPER), target_user, "--user", "show", "--property=LoadState", "chrote-agent@chrote-preflight.service"],
        check=False,
        timeout=5,
    )
    return result.returncode == 0 and "LoadState=loaded" in result.stdout


def run_cross_user(target_user: str) -> ScenarioResult:
    # The complete cross-user fixture deliberately requires root: only root can
    # create a private target-owned tmux keeper without expanding the shipped
    # sudoers grant beyond chrote-agent@ unit control.
    account = pwd.getpwnam(target_user)
    service = pwd.getpwnam("chrote")
    for installed, source in (
        (INSTALLED_HELPER, HELPER_SOURCE),
        (INSTALLED_SUDOERS, SUDOERS_SOURCE),
        (INSTALLED_LAUNCHER, LAUNCHER),
    ):
        if installed.read_bytes() != source.read_bytes():
            raise AcceptanceFailure(f"installed artifact differs from reviewed source: {installed}")
    if not STATE_ENV.is_file():
        raise AcceptanceFailure(f"shared state configuration is missing: {STATE_ENV}")
    state: dict[str, str] = {}
    for line in STATE_ENV.read_text().splitlines():
        key, separator, value = line.partition("=")
        if separator and key in {"CHROTE_AGENT_UNITS_DIR", "CHROTE_AGENT_RECEIPTS_DIR"}:
            state[key] = value.strip()
    units_root = Path(state.get("CHROTE_AGENT_UNITS_DIR", "/srv/data/chrote/agent-units"))
    receipts_root = Path(state.get("CHROTE_AGENT_RECEIPTS_DIR", "/srv/data/chrote/agent-receipts"))
    for path in (units_root, receipts_root):
        validate_safe_path(path, "installed state root")

    suffix = uuid.uuid4().hex[:8]
    root = Path(tempfile.mkdtemp(prefix=ROOT_PREFIX + "cross-" + suffix + "-", dir="/tmp"))
    validate_cleanup_root(root)
    session = f"accept-cross-{suffix}"
    keeper = f"accept-keeper-{suffix}"
    unit = f"chrote-agent@{session}.service"
    socket = root / "tmux.sock"
    config = units_root / target_user / f"{session}.conf"
    receipt = receipts_root / target_user / f"{session}.receipt.json"
    prefix = [
        "/usr/bin/setpriv", "--reuid", target_user, "--regid", target_user,
        "--init-groups", "--inh-caps=-all", "--", "/usr/bin/env", "-i",
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        f"HOME={account.pw_dir}", f"XDG_RUNTIME_DIR=/run/user/{account.pw_uid}",
    ]
    control_prefix = [
        "/usr/bin/setpriv", "--reuid", service.pw_name, "--regid", service.pw_name,
        "--init-groups", "--inh-caps=-all", "--", "/usr/bin/sudo", "-n", "--",
        str(INSTALLED_HELPER), target_user, "--user",
    ]
    try:
        os.chown(root, account.pw_uid, account.pw_gid)
        agent, launches = write_agent_fixture(root)
        os.chown(agent, account.pw_uid, account.pw_gid)
        start_keeper("/usr/bin/tmux", socket, keeper, root, prefix=prefix)
        config.parent.mkdir(parents=True, exist_ok=True)
        write_agent_config(
            config, session=session, tmux="/usr/bin/tmux", socket=socket,
            workdir=root, agent=agent, receipt=receipt,
        )
        os.chown(config, service.pw_uid, service.pw_gid)
        run([*control_prefix, "enable", "--now", unit], timeout=25)
        wait_for("cross-user agent launch", lambda: len(read_json_lines(launches)) >= 1 and bool(read_receipt(receipt)), timeout=20)
        first = read_json_lines(launches)[0]
        if first.get("argv") != ["resume", SESSION_ID]:
            raise AcceptanceFailure(f"cross-user unit resumed the wrong native session: {first}")
        os.kill(int(first["pid"]), signal.SIGKILL)
        wait_for("cross-user agent restart", lambda: len(read_json_lines(launches)) >= 2, timeout=20)
        second = read_json_lines(launches)[1]
        if second.get("argv") != ["resume", SESSION_ID]:
            raise AcceptanceFailure(f"cross-user restart resumed the wrong native session: {second}")
        run([*control_prefix, "disable", "--now", unit], timeout=20)
        if not process_alive(int(second["pid"])):
            raise AcceptanceFailure("cross-user disable killed the running agent")
        return ScenarioResult(
            "cross_user_lock", "PASS", "shipped sudoers/helper controlled the target user manager",
            {"targetUser": target_user, "unit": unit, "nativeSessionId": SESSION_ID},
        )
    finally:
        run([*control_prefix, "disable", "--now", unit], check=False, timeout=10)
        run([*prefix, *tmux_cleanup_argv("/usr/bin/tmux", str(socket), session)], check=False, timeout=5)
        run([*prefix, *tmux_cleanup_argv("/usr/bin/tmux", str(socket), keeper)], check=False, timeout=5)
        for path in (receipt, config):
            try:
                path.unlink()
            except FileNotFoundError:
                pass
        validate_cleanup_root(root, expected_uid=account.pw_uid)
        shutil.rmtree(root)


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cross-user", help="target account for installed cross-user acceptance")
    parser.add_argument("--require-cross-user", action="store_true", help="fail if cross-user acceptance is gated")
    parser.add_argument("--output", type=Path, help="optional JSON evidence path")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    root = Path(tempfile.mkdtemp(prefix=ROOT_PREFIX, dir="/tmp"))
    validate_cleanup_root(root)
    results: list[ScenarioResult] = []
    try:
        try:
            results.extend(same_user_scenarios(root))
            results.extend(dead_manager_scenarios(root))
        except Exception as exc:
            results.append(ScenarioResult("same_user_acceptance", "FAIL", str(exc)))

        template_loaded = template_loaded_for(args.cross_user)
        prerequisite = cross_user_prerequisite_result(
            target_user=args.cross_user,
            helper=INSTALLED_HELPER,
            sudoers=INSTALLED_SUDOERS,
            template_loaded=template_loaded,
            is_root=os.geteuid() == 0,
        )
        if prerequisite.status == "READY" and args.cross_user:
            try:
                results.append(run_cross_user(args.cross_user))
            except Exception as exc:
                results.append(ScenarioResult("cross_user_lock", "FAIL", str(exc)))
        else:
            results.append(prerequisite)
    finally:
        validate_cleanup_root(root)
        shutil.rmtree(root)

    payload = {
        "ok": result_exit_code(results, require_cross_user=args.require_cross_user) == 0,
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "results": [asdict(result) for result in results],
    }
    encoded = json.dumps(payload, indent=2, sort_keys=True)
    print(encoded)
    if args.output:
        args.output.write_text(encoded + "\n")
    return result_exit_code(results, require_cross_user=args.require_cross_user)


if __name__ == "__main__":
    raise SystemExit(main())
