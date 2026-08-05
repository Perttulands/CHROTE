#!/usr/bin/env python3
"""Unit tests for the disposable Persistence v2 acceptance harness."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parent / "persistence_v2_acceptance.py"
SPEC = importlib.util.spec_from_file_location("persistence_v2_acceptance", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
acceptance = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = acceptance
SPEC.loader.exec_module(acceptance)


class PersistenceV2AcceptanceTests(unittest.TestCase):
    def test_harness_bypasses_the_production_tmux_guard_for_private_socket(self) -> None:
        self.assertEqual(acceptance.trusted_tmux(), "/usr/bin/tmux")

    def test_tmux_cleanup_targets_sessions_never_the_server(self) -> None:
        argv = acceptance.tmux_cleanup_argv(
            "/usr/bin/tmux", "/tmp/chrote-persistence-v2-deadbeef/tmux.sock", "accept-deadbeef"
        )
        self.assertEqual(
            argv,
            [
                "/usr/bin/tmux", "-S", "/tmp/chrote-persistence-v2-deadbeef/tmux.sock",
                "kill-session", "-t", "=accept-deadbeef",
            ],
        )
        self.assertNotIn("kill-server", argv)

    def test_cleanup_root_must_be_exactly_owned_and_narrow(self) -> None:
        with tempfile.TemporaryDirectory(prefix="chrote-persistence-v2-test-") as parent:
            root = Path(parent) / "chrote-persistence-v2-deadbeef"
            root.mkdir()
            acceptance.validate_cleanup_root(root)
            with self.assertRaises(acceptance.AcceptanceFailure):
                acceptance.validate_cleanup_root(Path("/tmp"))
            with self.assertRaises(acceptance.AcceptanceFailure):
                acceptance.validate_cleanup_root(Path(parent) / "unrelated")

    def test_cross_user_prerequisites_become_named_gate_not_skip(self) -> None:
        result = acceptance.cross_user_prerequisite_result(
            target_user=None,
            helper=Path("/definitely/missing/helper"),
            sudoers=Path("/definitely/missing/sudoers"),
            template_loaded=False,
            is_root=False,
        )
        self.assertEqual(result.name, "cross_user_lock")
        self.assertEqual(result.status, "GATED")
        self.assertIn("--cross-user", result.detail)
        self.assertNotIn("skip", result.detail.lower())

    def test_required_gate_makes_the_harness_nonzero(self) -> None:
        results = [
            acceptance.ScenarioResult("same_user_lock", "PASS", "ok"),
            acceptance.ScenarioResult("cross_user_lock", "GATED", "grant unavailable"),
        ]
        self.assertEqual(acceptance.result_exit_code(results, require_cross_user=False), 0)
        self.assertNotEqual(acceptance.result_exit_code(results, require_cross_user=True), 0)

    def test_dead_manager_probe_uses_only_a_test_owned_bus_path(self) -> None:
        root = Path("/tmp/chrote-persistence-v2-deadbeef")
        env = acceptance.dead_manager_environment(root)
        self.assertEqual(env["XDG_RUNTIME_DIR"], str(root / "dead-runtime"))
        self.assertEqual(
            env["DBUS_SESSION_BUS_ADDRESS"],
            f"unix:path={root / 'dead-runtime' / 'bus'}",
        )


if __name__ == "__main__":
    unittest.main()
