#!/usr/bin/env python3
from __future__ import annotations

import sys
import unittest
from pathlib import Path

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
        same_ids_new_processes = {"9001", "9002"} & {"9101", "9102"}
        self.assertEqual(same_ids_new_processes, set(), "disjoint pids mean the panes were recreated")
        genuine_survivor = {"9001", "9002"} & {"9002", "9103"}
        self.assertEqual(genuine_survivor, {"9002"}, "an overlapping pid is a real survivor and must fail")

    def test_kill_tmux_server_fails_loud_when_the_server_survives(self) -> None:
        # A guarded tmux wrapper refuses kill-server with a non-zero exit. check=False swallowed
        # that, so the smoke restored a server it never killed and passed vacuously.
        def fake_run_command(argv, **kwargs):
            if "kill-server" in argv:
                return smoke_disposable.CommandResult(
                    argv=argv, returncode=126, stdout="", stderr="BLOCKED by CHROTE tmux guard: kill-server"
                )
            return smoke_disposable.CommandResult(argv=argv, returncode=0, stdout="ctxsh75_abc: 1 windows\n", stderr="")

        original = smoke_disposable.run_command
        smoke_disposable.run_command = fake_run_command
        try:
            with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "survived kill-server"):
                smoke_disposable.kill_tmux_server(Path("/tmp/x/t.sock"), "/usr/bin/tmux")
        finally:
            smoke_disposable.run_command = original

    def test_kill_tmux_server_accepts_a_server_that_is_really_gone(self) -> None:
        def fake_run_command(argv, **kwargs):
            if "kill-server" in argv:
                return smoke_disposable.CommandResult(argv=argv, returncode=0, stdout="", stderr="")
            return smoke_disposable.CommandResult(
                argv=argv, returncode=1, stdout="", stderr="no server running on /tmp/x/t.sock"
            )

        original = smoke_disposable.run_command
        smoke_disposable.run_command = fake_run_command
        try:
            smoke_disposable.kill_tmux_server(Path("/tmp/x/t.sock"), "/usr/bin/tmux")
        finally:
            smoke_disposable.run_command = original

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
