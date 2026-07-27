#!/usr/bin/env python3
from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

import smoke_disposable


class DisposableSmokeHelpersTest(unittest.TestCase):
    def test_hermes_resume_argv_uses_exact_noninteractive_shape(self) -> None:
        argv = smoke_disposable.hermes_resume_argv("/tmp/fake-home", "smoke", "hermes-session-test")
        self.assertEqual(
            argv,
            [
                "/tmp/fake-home/.hermes/hermes-agent-current/venv/bin/python",
                "-m",
                "hermes_cli.main",
                "--profile",
                "smoke",
                "--resume",
                "hermes-session-test",
            ],
        )
        self.assertNotIn("--tui", argv)
        self.assertNotIn("--yolo", argv)

    def test_forbidden_resource_guard_rejects_live_paths_and_ports(self) -> None:
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "live 8095"):
            smoke_disposable.assert_no_forbidden_references(["http://127.0.0.1:8095"])
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "live 7686"):
            smoke_disposable.assert_no_forbidden_references(["http://localhost:7686/ws"])
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "live data"):
            smoke_disposable.assert_no_forbidden_references(["/srv/data/chrote/session-bank/sessions.json"])
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "live agent session"):
            smoke_disposable.assert_no_forbidden_references(["claude-worker-1"])

    def test_forbidden_resource_guard_rejects_live_tmux_sockets_by_shape(self) -> None:
        # Any uid, not just this host's. Each of these once required a hardcoded literal to catch.
        for live in [
            "/run/user/4242/chrote-tmux/tmux-4242/default",
            "/run/user/5150/chrote-formations-tmux/default",
            "/tmp/tmux-4242/default",
            "/tmp/tmux-31337",
            "/run/chrote/formations-tmux/default",
        ]:
            with self.subTest(live=live):
                with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "live tmux socket"):
                    smoke_disposable.assert_no_forbidden_references([{"socket": live}])

    def test_forbidden_resource_guard_accepts_the_smokes_own_disposable_resources(self) -> None:
        # Regression: a neutrality sweep once rewrote the guard's literals to the fixture values,
        # which made the smoke's own `go build` argv trip it. The guard must pass its own record.
        temp_root = "/tmp/ctx-sh7-5-0123456789ab"
        smoke_disposable.assert_no_forbidden_references(
            [
                [["go", "build", "-o", f"{temp_root}/bin/chrote-server", "./cmd/server"]],
                {"tempRoot": temp_root, "tmuxSocket": f"{temp_root}/tmux/ctx-sh7-5.sock"},
                {"sessionName": "ctxsh75_deadbeef", "apiUrl": "http://127.0.0.1:41234"},
                {"fakeOwnerHome": f"{temp_root}/fake-home"},
            ],
            temp_root=temp_root,
        )

    def test_forbidden_resource_guard_scopes_socket_allowance_to_its_own_temp_root(self) -> None:
        # A disposable socket under someone else's temp root is still not ours.
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "live tmux socket"):
            smoke_disposable.assert_no_forbidden_references(
                [{"socket": "/tmp/tmux-4242/default"}],
                temp_root="/tmp/ctx-sh7-5-0123456789ab",
            )

    def test_forbidden_resource_guard_names_no_operator_or_uid(self) -> None:
        # The guard must stay host-neutral by construction, so no future sweep has literals to rewrite.
        source = Path(smoke_disposable.__file__).read_text(encoding="utf-8")
        guard = source.split("LIVE_TMUX_SOCKET_PATTERNS")[1].split("def _stringify")[0]
        self.assertNotRegex(guard, r"/run/user/\d")
        self.assertNotRegex(guard, r"/tmp/tmux-\d")

    def test_live_pane_pids_reads_pids_from_tmux_for_one_session(self) -> None:
        seen: list[list[str]] = []

        def fake_run_command(argv, **kwargs):
            seen.append(argv)
            return smoke_disposable.CommandResult(argv=argv, returncode=0, stdout="4242\n4243\n\n", stderr="")

        original = smoke_disposable.run_command
        smoke_disposable.run_command = fake_run_command
        try:
            pids = smoke_disposable.live_pane_pids(Path("/tmp/x/t.sock"), "ctxsh75_abc", "/usr/bin/tmux")
        finally:
            smoke_disposable.run_command = original

        self.assertEqual(pids, {"4242", "4243"})  # blank line dropped
        self.assertEqual(
            seen[0],
            ["/usr/bin/tmux", "-S", "/tmp/x/t.sock", "list-panes", "-t", "ctxsh75_abc", "-F", "#{pane_pid}"],
        )

    def test_restored_panes_are_judged_by_pid_not_by_pane_id(self) -> None:
        # tmux allocates pane ids monotonically PER SERVER and restore rebuilds on a fresh server,
        # so a snapshotted session holding the first panes gets the same ids back and an
        # id-inequality check fires on a correct restore. Pids do not depend on creation order.
        replaced = smoke_disposable.assert_pane_processes_recreated(
            {"9001", "9002"},
            {"9101", "9102"},
        )
        self.assertEqual(replaced, 4)
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "pre-kill processes.*9002"):
            smoke_disposable.assert_pane_processes_recreated(
                {"9001", "9002"},
                {"9002", "9103"},
            )

    def test_tmux_server_pid_is_read_from_the_exact_disposable_socket(self) -> None:
        seen: list[list[str]] = []

        def fake_run_command(argv, **kwargs):
            seen.append(argv)
            return smoke_disposable.CommandResult(argv=argv, returncode=0, stdout="4242\n", stderr="")

        with mock.patch.object(smoke_disposable, "run_command", side_effect=fake_run_command):
            pid = smoke_disposable.tmux_server_pid(Path("/tmp/x/t.sock"), "/usr/bin/tmux")

        self.assertEqual(pid, 4242)
        self.assertEqual(
            seen,
            [["/usr/bin/tmux", "-S", "/tmp/x/t.sock", "display-message", "-p", "#{pid}"]],
        )

    def test_smoke_never_invokes_tmux_kill_server(self) -> None:
        source = Path(smoke_disposable.__file__).read_text(encoding="utf-8")
        self.assertNotRegex(source, r'tmux\(\s*"kill-server"')

    def test_stop_tmux_server_refuses_a_pid_not_owned_by_the_disposable_socket(self) -> None:
        with (
            mock.patch.object(smoke_disposable, "tmux_server_pid", return_value=5000) as read_pid,
            mock.patch.object(smoke_disposable.os, "kill") as send_signal,
        ):
            with self.assertRaisesRegex(
                smoke_disposable.SmokeFailure,
                "disposable socket reports pid 5000, not recorded pid 4242",
            ):
                smoke_disposable.stop_tmux_server(Path("/tmp/x/t.sock"), 4242, "/usr/bin/tmux")

        read_pid.assert_called_once_with(Path("/tmp/x/t.sock"), "/usr/bin/tmux", None)
        send_signal.assert_not_called()

    def test_stop_tmux_server_signals_only_the_recorded_pid(self) -> None:
        with tempfile.TemporaryDirectory() as raw_dir:
            socket_path = Path(raw_dir) / "tmux.sock"
            socket_path.touch()
            signal_calls: list[tuple[int, int]] = []
            alive = {4242}

            def fake_kill(pid: int, sig: int) -> None:
                signal_calls.append((pid, sig))
                if sig == smoke_disposable.signal.SIGTERM:
                    alive.discard(pid)
                    socket_path.unlink()
                elif pid not in alive:
                    raise ProcessLookupError

            with (
                mock.patch.object(smoke_disposable, "tmux_server_pid", return_value=4242),
                mock.patch.object(smoke_disposable.os, "kill", side_effect=fake_kill),
            ):
                smoke_disposable.stop_tmux_server(socket_path, 4242, "/usr/bin/tmux")

        self.assertEqual({pid for pid, _sig in signal_calls}, {4242})
        self.assertEqual(signal_calls.count((4242, smoke_disposable.signal.SIGTERM)), 1)

    def test_stop_tmux_server_removes_its_stale_socket_only_after_the_pid_dies(self) -> None:
        with tempfile.TemporaryDirectory() as raw_dir:
            socket_path = Path(raw_dir) / "tmux.sock"
            socket_path.touch()
            alive = {4242}

            def fake_kill(pid: int, sig: int) -> None:
                if sig == smoke_disposable.signal.SIGTERM:
                    alive.discard(pid)
                elif pid not in alive:
                    raise ProcessLookupError

            with (
                mock.patch.object(smoke_disposable, "tmux_server_pid", return_value=4242),
                mock.patch.object(smoke_disposable.os, "kill", side_effect=fake_kill),
                mock.patch.object(smoke_disposable.time, "monotonic", side_effect=[10.0, 10.0, 10.25]),
                mock.patch.object(smoke_disposable.time, "sleep"),
            ):
                smoke_disposable.stop_tmux_server(
                    socket_path,
                    4242,
                    "/usr/bin/tmux",
                    timeout=0.2,
                    poll_interval=0.05,
                )

            self.assertFalse(socket_path.exists())

    def test_stop_tmux_server_uses_a_bounded_timeout_and_fails_if_the_pid_survives(self) -> None:
        with tempfile.TemporaryDirectory() as raw_dir:
            socket_path = Path(raw_dir) / "tmux.sock"
            socket_path.touch()
            signal_calls: list[tuple[int, int]] = []

            def fake_kill(pid: int, sig: int) -> None:
                signal_calls.append((pid, sig))

            with (
                mock.patch.object(smoke_disposable, "tmux_server_pid", return_value=4242),
                mock.patch.object(smoke_disposable.os, "kill", side_effect=fake_kill),
                mock.patch.object(smoke_disposable.time, "monotonic", side_effect=[10.0, 10.0, 10.25]) as clock,
                mock.patch.object(smoke_disposable.time, "sleep") as sleep,
            ):
                with self.assertRaisesRegex(
                    smoke_disposable.SmokeFailure,
                    "pid 4242 survived SIGTERM after 0.200s",
                ):
                    smoke_disposable.stop_tmux_server(
                        socket_path,
                        4242,
                        "/usr/bin/tmux",
                        timeout=0.2,
                        poll_interval=0.05,
                    )

        self.assertEqual({pid for pid, _sig in signal_calls}, {4242})
        self.assertEqual(signal_calls.count((4242, smoke_disposable.signal.SIGTERM)), 1)
        self.assertEqual(clock.call_count, 3)
        sleep.assert_called_once_with(0.05)

    def test_cleanup_preserves_the_socket_root_if_the_recorded_pid_survives(self) -> None:
        with tempfile.TemporaryDirectory() as raw_dir:
            temp_root = Path(raw_dir) / "smoke"
            socket_path = temp_root / "tmux" / "tmux.sock"
            socket_path.parent.mkdir(parents=True)
            socket_path.touch()
            server = smoke_disposable.DisposableTmuxServer(
                socket_path=socket_path,
                tmux_bin="/usr/bin/tmux",
            )
            server.pid = 4242
            cleanup = smoke_disposable.CleanupStack()
            cleanup.add(lambda: server.remove_temp_root(temp_root))
            cleanup.add(server.stop)

            with mock.patch.object(
                smoke_disposable,
                "stop_tmux_server",
                side_effect=smoke_disposable.SmokeFailure("pid 4242 survived SIGTERM"),
            ):
                with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "survived SIGTERM"):
                    cleanup.close()

            self.assertTrue(socket_path.exists(), "a live server must keep its socket reachable")

    def test_cleanup_preserves_the_socket_root_if_started_server_pid_capture_fails(self) -> None:
        with tempfile.TemporaryDirectory() as raw_dir:
            temp_root = Path(raw_dir) / "smoke"
            socket_path = temp_root / "tmux" / "tmux.sock"
            socket_path.parent.mkdir(parents=True)
            socket_path.touch()
            server = smoke_disposable.DisposableTmuxServer(
                socket_path=socket_path,
                tmux_bin="/usr/bin/tmux",
            )
            cleanup = smoke_disposable.CleanupStack()
            cleanup.add(lambda: server.remove_temp_root(temp_root))
            cleanup.add(server.stop)

            with mock.patch.object(
                smoke_disposable,
                "tmux_server_pid",
                side_effect=smoke_disposable.SmokeFailure("pid query failed"),
            ):
                with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "pid query failed"):
                    server.capture()
            with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "pid was not recorded"):
                cleanup.close()

            self.assertTrue(socket_path.exists(), "an untracked started server must keep its socket reachable")

    def test_loopback_port_allocation_reports_environment_blocker(self) -> None:
        def denied_socket(*_args, **_kwargs):
            raise PermissionError(1, "Operation not permitted")

        with self.assertRaisesRegex(smoke_disposable.SmokeBlocker, "loopback sockets"):
            smoke_disposable.choose_loopback_port(socket_factory=denied_socket)

    def test_cleanup_stack_runs_all_callbacks_in_reverse_order(self) -> None:
        calls: list[str] = []
        stack = smoke_disposable.CleanupStack()
        stack.add(lambda: calls.append("first"))

        def failing() -> None:
            calls.append("second")
            raise RuntimeError("cleanup failed")

        stack.add(failing)
        stack.add(lambda: calls.append("third"))

        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "cleanup failed"):
            stack.close()
        self.assertEqual(calls, ["third", "second", "first"])

    def test_managed_record_is_manifest_only_non_restarting_systemd_user(self) -> None:
        record = smoke_disposable.managed_systemd_record(
            session_name="ctxsh75-managed",
            unix_user="alice",
            unit="ctx-sh7-5-demo.service",
            owner_home="/tmp/fake-home",
        )
        self.assertEqual(record["managerKind"], "systemd-user")
        self.assertFalse(record["restartAllowed"])
        self.assertEqual(record["owner"], {"kind": "external_manager", "ref": "systemd:user/ctx-sh7-5-demo.service", "mayRestart": False})
        self.assertEqual(record["statusProbe"], {"kind": "systemd-user", "unit": "ctx-sh7-5-demo.service", "expectActiveState": "active"})
        self.assertEqual(record["descriptors"][0]["mode"], "managed")
        self.assertEqual(record["descriptors"][0]["owner"], record["owner"])


if __name__ == "__main__":
    unittest.main()
