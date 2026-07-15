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
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "live data"):
            smoke_disposable.assert_no_forbidden_references(["/srv/data/chrote/session-bank/sessions.json"])
        with self.assertRaisesRegex(smoke_disposable.SmokeFailure, "Tavern"):
            smoke_disposable.assert_no_forbidden_references(["/tmp/tavern.sock"])

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
