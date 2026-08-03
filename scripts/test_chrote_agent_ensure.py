#!/usr/bin/env python3
"""Contract tests for scripts/chrote-agent-ensure.sh (ADR-0014).

The launcher is what systemd supervises for a locked agent, so the properties
tested here are the ones a broken launcher would violate silently:

* it never creates a tmux server (the fork-into-our-cgroup hazard),
* it stays attached and exits non-zero when the agent dies (so Restart= fires),
* it renders the resume argv from typed fields rather than trusting a string,
* it refuses malformed config loudly instead of guessing.

tmux is a fake binary that records its argv, so no test touches a real server.
"""
from __future__ import annotations

import os
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = REPO_ROOT / "scripts" / "chrote-agent-ensure.sh"
UNIT = REPO_ROOT / "services" / "chrote-agent@.service"

SESSION_ID = "019f4baa-e368-7ea0-8912-fb2c6f99785c"

# A fake tmux. Behaviour is steered by files in $FAKE_STATE so a single test can
# make the server die between calls. Every invocation is appended to the log.
FAKE_TMUX = r"""#!/usr/bin/env bash
state="${FAKE_STATE:?}"
printf '%s\n' "$*" >>"$state/calls.log"
# Strip the socket selector the launcher always injects.
args=("$@")
if [[ "${args[0]}" == "-S" ]]; then
  printf '%s\n' "${args[1]}" >>"$state/sockets.log"
  args=("${args[@]:2}")
fi
command="${args[0]:-}"
case "$command" in
  list-sessions)
    [[ -f "$state/server_dead" ]] && exit 1
    exit 0
    ;;
  has-session)
    [[ -f "$state/server_dead" ]] && exit 1
    [[ -f "$state/session_exists" ]] || exit 1
    # A session that vanishes mid-watch: after the launcher has adopted it and
    # asked about it $vanish_after times, report it gone. Simulating this by
    # deleting the marker up front would not work -- the launcher would simply
    # create the session again, which is correct behaviour, not a vanish.
    if [[ -f "$state/vanish_after" ]]; then
      seen=$(cat "$state/has_session_calls" 2>/dev/null || printf 0)
      seen=$((seen + 1))
      printf '%s' "$seen" >"$state/has_session_calls"
      (( seen > $(cat "$state/vanish_after") )) && exit 1
    fi
    exit 0
    ;;
  new-session)
    [[ -f "$state/server_dead" ]] && exit 1
    touch "$state/session_exists"
    exit 0
    ;;
  display-message)
    [[ -f "$state/server_dead" ]] && exit 1
    [[ -f "$state/session_exists" ]] || exit 1
    cat "$state/pane_pid"
    exit 0
    ;;
  send-keys)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
"""


class LauncherFixture:
    """A disposable HOME-ish tree with a fake tmux and a typed config file."""

    def __init__(self, **overrides: str) -> None:
        self.root = Path(tempfile.mkdtemp(prefix="chrote-agent-ensure-"))
        self.state = self.root / "state"
        self.state.mkdir()
        self.workdir = self.root / "workspace"
        self.workdir.mkdir()

        self.tmux = self.root / "tmux"
        self.tmux.write_text(FAKE_TMUX)
        self.tmux.chmod(0o755)

        self.agent_bin = self.root / "agent-bin"
        self.agent_bin.write_text("#!/usr/bin/env bash\nexit 0\n")
        self.agent_bin.chmod(0o755)

        # A live process to stand in for the pane's own process. Its own PID is
        # always alive, so "pane still running" is true until a test says
        # otherwise by pointing pane_pid at a dead one.
        (self.state / "pane_pid").write_text(f"{os.getpid()}\n")

        self.receipt = self.root / "receipt.json"
        self.values = {
            "CHROTE_AGENT_SESSION": "agent-under-test",
            "CHROTE_AGENT_TMUX_BIN": str(self.tmux),
            "CHROTE_AGENT_TMUX_SOCKET": str(self.root / "tmux.sock"),
            "CHROTE_AGENT_WORKDIR": str(self.workdir),
            "CHROTE_AGENT_KIND": "codex",
            "CHROTE_AGENT_SESSION_ID": SESSION_ID,
            "CHROTE_AGENT_BIN": str(self.agent_bin),
            "CHROTE_AGENT_RECEIPT_PATH": str(self.receipt),
            "CHROTE_AGENT_WATCH_INTERVAL": "1",
            "CHROTE_AGENT_TMUX_KEEPER_UNIT": "cockpit-tmux.service",
        }
        self.values.update(overrides)
        self.config = self.root / "agent.conf"
        self.write_config()

    def write_config(self, *, drop: str | None = None) -> None:
        lines = [f"{key}={value}" for key, value in self.values.items() if key != drop]
        self.config.write_text("\n".join(lines) + "\n")
        self.config.chmod(0o600)

    def session_exists(self, exists: bool = True) -> None:
        marker = self.state / "session_exists"
        marker.touch() if exists else marker.unlink(missing_ok=True)

    def kill_server(self) -> None:
        (self.state / "server_dead").touch()

    def vanish_session_after(self, calls: int) -> None:
        (self.state / "vanish_after").write_text(str(calls))

    def set_pane_pid(self, pid: int) -> None:
        (self.state / "pane_pid").write_text(f"{pid}\n")

    def run(self, *args: str, timeout: int = 30) -> subprocess.CompletedProcess:
        env = {
            "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
            "HOME": str(self.root),
            "FAKE_STATE": str(self.state),
        }
        return subprocess.run(
            ["bash", str(LAUNCHER), "--config", str(self.config), *args],
            env=env,
            text=True,
            capture_output=True,
            check=False,
            timeout=timeout,
        )

    def calls(self) -> list[str]:
        log = self.state / "calls.log"
        return log.read_text().splitlines() if log.exists() else []

    def sockets(self) -> list[str]:
        log = self.state / "sockets.log"
        return log.read_text().splitlines() if log.exists() else []


class SocketKeeperContractTests(unittest.TestCase):
    def test_dead_server_is_refused_and_never_revived(self) -> None:
        fixture = LauncherFixture()
        fixture.kill_server()
        result = fixture.run("--once")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn(
            "new-session",
            " ".join(fixture.calls()),
            "the launcher must never create a tmux server on a dead socket",
        )
        self.assertIn("refusing to create one", result.stderr)
        self.assertIn(
            "cockpit-tmux.service",
            result.stderr,
            "the failure must name the keeper unit so the operator knows what to start",
        )

    def test_socket_is_always_selected_explicitly(self) -> None:
        fixture = LauncherFixture()
        fixture.run("--once")
        sockets = fixture.sockets()
        self.assertTrue(sockets, "every tmux call must pass -S <socket>")
        self.assertEqual(
            {fixture.values["CHROTE_AGENT_TMUX_SOCKET"]},
            set(sockets),
            "no tmux call may reach a socket other than the configured one",
        )


class SessionLifecycleTests(unittest.TestCase):
    def test_missing_session_is_created_and_resumed_by_native_id(self) -> None:
        fixture = LauncherFixture()
        result = fixture.run("--once")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        calls = fixture.calls()
        joined = "\n".join(calls)
        self.assertIn("new-session -d -s agent-under-test", joined)
        self.assertIn(f"send-keys -t =agent-under-test -l -- {fixture.agent_bin} resume {SESSION_ID}", joined)
        self.assertIn("send-keys -t =agent-under-test Enter", joined)
        create_index = next(i for i, call in enumerate(calls) if call.startswith("-S") and "new-session" in call)
        liveness_index = next(i for i, call in enumerate(calls) if "list-sessions" in call)
        self.assertLess(
            liveness_index, create_index, "liveness must be proven before creating anything"
        )

    def test_existing_session_is_adopted_without_sending_keys(self) -> None:
        fixture = LauncherFixture()
        fixture.session_exists(True)
        result = fixture.run("--once")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        joined = "\n".join(fixture.calls())
        self.assertNotIn("new-session", joined)
        self.assertNotIn(
            "send-keys",
            joined,
            "adopting a live session must never type into someone's running agent",
        )

    def test_claude_kind_renders_its_own_resume_flag(self) -> None:
        fixture = LauncherFixture(CHROTE_AGENT_KIND="claude")
        fixture.run("--once")
        self.assertIn(f"--resume {SESSION_ID}", "\n".join(fixture.calls()))

    def test_hermes_kind_renders_module_profile_and_resume(self) -> None:
        fixture = LauncherFixture(
            CHROTE_AGENT_KIND="hermes", CHROTE_AGENT_HERMES_PROFILE="research"
        )
        result = fixture.run("--once")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(
            f"-m hermes_cli.main --profile research --resume {SESSION_ID}",
            "\n".join(fixture.calls()),
        )

    def test_hermes_without_a_profile_fails_before_touching_tmux(self) -> None:
        fixture = LauncherFixture(CHROTE_AGENT_KIND="hermes")
        result = fixture.run("--once")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("new-session", "\n".join(fixture.calls()))

    def test_receipt_records_the_resumed_identity(self) -> None:
        fixture = LauncherFixture()
        result = fixture.run("--once")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(fixture.receipt.exists(), "a receipt is what proves WHICH transcript resumed")
        receipt = fixture.receipt.read_text()
        self.assertIn(SESSION_ID, receipt)
        self.assertIn("agent-under-test", receipt)
        self.assertEqual(0o600, fixture.receipt.stat().st_mode & 0o777)


class WatchContractTests(unittest.TestCase):
    """The property that makes Restart= mean anything."""

    def test_launcher_exits_nonzero_when_the_session_disappears(self) -> None:
        fixture = LauncherFixture()
        fixture.session_exists(True)
        fixture.vanish_session_after(1)
        result = fixture.run(timeout=30)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("disappeared", result.stderr)

    def test_launcher_exits_nonzero_when_the_pane_process_dies(self) -> None:
        fixture = LauncherFixture()
        fixture.session_exists(True)
        # PID 2 is the kernel's kthreadd on Linux; signalling it as an ordinary
        # user fails, which is exactly the "process is not ours / not alive"
        # signal the launcher checks for. A never-allocated PID would be racy.
        fixture.set_pane_pid(2**22 - 1)
        result = fixture.run(timeout=30)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exited", result.stderr)

    def test_launcher_exits_nonzero_when_the_server_stops_answering(self) -> None:
        fixture = LauncherFixture()
        fixture.session_exists(True)
        result = fixture.run("--once")
        self.assertEqual(result.returncode, 0, result.stderr)
        fixture.kill_server()
        result = fixture.run(timeout=30)
        self.assertNotEqual(result.returncode, 0)


class ConfigValidationTests(unittest.TestCase):
    def test_config_is_never_sourced(self) -> None:
        fixture = LauncherFixture()
        canary = fixture.root / "canary"
        fixture.config.write_text(
            fixture.config.read_text() + f"\n$(touch {canary})\n`touch {canary}`\n"
        )
        fixture.run("--once")
        self.assertFalse(
            canary.exists(),
            "a config file must never be executed; sourcing it would run whatever it contains",
        )

    def test_symlinked_config_is_refused(self) -> None:
        fixture = LauncherFixture()
        real = fixture.config
        link = fixture.root / "link.conf"
        link.symlink_to(real)
        env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "FAKE_STATE": str(fixture.state)}
        result = subprocess.run(
            ["bash", str(LAUNCHER), "--config", str(link), "--once"],
            env=env, text=True, capture_output=True, check=False, timeout=30,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("symlink", result.stderr)

    def test_missing_required_keys_fail_loud(self) -> None:
        for key in (
            "CHROTE_AGENT_SESSION",
            "CHROTE_AGENT_TMUX_SOCKET",
            "CHROTE_AGENT_KIND",
            "CHROTE_AGENT_SESSION_ID",
            "CHROTE_AGENT_BIN",
        ):
            with self.subTest(key=key):
                fixture = LauncherFixture()
                fixture.write_config(drop=key)
                result = fixture.run("--once")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(key, result.stderr)

    def test_malformed_values_are_refused(self) -> None:
        cases = {
            "CHROTE_AGENT_SESSION": "bad name; rm -rf /",
            "CHROTE_AGENT_KIND": "bash",
            "CHROTE_AGENT_SESSION_ID": "--last",
            "CHROTE_AGENT_TMUX_SOCKET": "relative/path",
            "CHROTE_AGENT_WATCH_INTERVAL": "0",
        }
        for key, value in cases.items():
            with self.subTest(key=key):
                fixture = LauncherFixture(**{key: value})
                result = fixture.run("--once")
                self.assertNotEqual(result.returncode, 0, f"{key}={value!r} should be refused")

    def test_unsupported_agent_kind_never_reaches_tmux(self) -> None:
        fixture = LauncherFixture(CHROTE_AGENT_KIND="shell")
        result = fixture.run("--once")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("new-session", "\n".join(fixture.calls()))


class UnitFileTests(unittest.TestCase):
    """The unit is a contract too: three directives carry ADR-0014 decisions."""

    def setUp(self) -> None:
        self.text = UNIT.read_text()

    def test_type_is_simple_so_active_means_the_launcher_is_running(self) -> None:
        self.assertIn("Type=simple", self.text)
        self.assertNotIn(
            "RemainAfterExit",
            self.text,
            "RemainAfterExit would make 'active' mean 'we once ran the launcher'",
        )

    def test_restart_policy_has_a_reachable_burst_window(self) -> None:
        self.assertIn("Restart=on-failure", self.text)
        restart_sec = int(self._directive("RestartSec"))
        burst = int(self._directive("StartLimitBurst"))
        interval = int(self._directive("StartLimitIntervalSec"))
        self.assertGreater(
            interval,
            restart_sec * burst,
            "StartLimitIntervalSec must exceed RestartSec x StartLimitBurst or the burst is unreachable",
        )

    def test_killmode_process_keeps_the_agent_alive_when_the_unit_stops(self) -> None:
        self.assertIn("KillMode=process", self.text)

    def test_execstart_is_argv_only(self) -> None:
        exec_start = self._directive("ExecStart")
        for metacharacter in ("&&", "||", ";", "|", "$(", "`"):
            self.assertNotIn(
                metacharacter,
                exec_start,
                "ExecStart must be a plain argv, never a shell fragment",
            )

    def _directive(self, key: str) -> str:
        for line in self.text.splitlines():
            if line.startswith(f"{key}="):
                return line.split("=", 1)[1].strip()
        raise AssertionError(f"unit does not set {key}")


class LauncherHygieneTests(unittest.TestCase):
    def test_launcher_is_executable_and_shellcheck_clean_enough_to_parse(self) -> None:
        self.assertTrue(os.access(LAUNCHER, os.X_OK), "launcher must be executable")
        parsed = subprocess.run(
            ["bash", "-n", str(LAUNCHER)], text=True, capture_output=True, check=False
        )
        self.assertEqual(parsed.returncode, 0, parsed.stderr)

    def test_launcher_hardcodes_no_host_specifics(self) -> None:
        text = LAUNCHER.read_text()
        for token in ("/run/user/", "/home/", "/srv/data", "linuxbrew"):
            self.assertNotIn(
                token,
                text,
                "the launcher must take every host-specific path from config",
            )


if __name__ == "__main__":
    unittest.main()
