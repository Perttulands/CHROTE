#!/usr/bin/env python3
"""Contract tests for scripts/chrote-agentctl (ADR-0014 decision 4).

This helper is the one privileged step in the whole feature: it crosses a unix
account boundary. The owner has approved that crossing as policy, so these tests
are not about whether it may cross -- they are about whether the crossing can be
made to do more than intended. Every test drives the argument parser only; none
reaches setpriv or systemctl, because the refusals all happen before the exec.
"""
from __future__ import annotations

import os
import subprocess
import unittest
from pathlib import Path

HELPER = Path(__file__).resolve().parents[1] / "scripts" / "chrote-agentctl"
REPO_ROOT = HELPER.parents[1]
SUDOERS = REPO_ROOT / "services" / "chrote-agentctl.sudoers"
SERVER_UNIT = REPO_ROOT / "services" / "chrote-srv.service"
CONTROLLER = REPO_ROOT / "src" / "internal" / "api" / "agent_units.go"

# Refusals exit 64 and never exec. A test that asserts "non-zero" would also pass
# if the helper crashed after doing the dangerous thing, so the exact code
# matters.
REFUSED = 64


def run(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["bash", str(HELPER), *args],
        env={"PATH": os.environ.get("PATH", "/usr/bin:/bin")},
        text=True,
        capture_output=True,
        check=False,
        timeout=30,
    )


class UnitScopeTests(unittest.TestCase):
    def test_only_units_from_the_chrote_template_are_reachable(self) -> None:
        foreign = [
            "codex-minerva-telegram.service",
            "chrote-srv.service",
            "sshd.service",
            "chrote-agent@x.timer",
            "chrote-agent@x.socket",
            "chrote-agentXx.service",
            "chrote-agent@.service",
            "chrote-agent@a b.service",
            "chrote-agent@a;b.service",
            "chrote-agent@../../etc/passwd.service",
            "chrote-agent@" + "a" * 51 + ".service",
            "chrote-agent@-flag.service",
            "*",
            "chrote-agent@*.service",
        ]
        for unit in foreign:
            with self.subTest(unit=unit):
                result = run("alice", "--user", "enable", "--now", unit)
                self.assertEqual(
                    REFUSED,
                    result.returncode,
                    f"unit {unit!r} must be refused before any privileged call",
                )

    def test_a_second_unit_cannot_be_smuggled_in(self) -> None:
        result = run("alice", "--user", "enable", "--now",
                     "chrote-agent@ok.service", "chrote-agent@also.service")
        self.assertEqual(REFUSED, result.returncode)
        self.assertIn("exactly one unit", result.stderr)


class VerbScopeTests(unittest.TestCase):
    def test_only_the_lock_and_read_verbs_are_permitted(self) -> None:
        for verb in ("start", "stop", "restart", "kill", "mask", "unmask",
                     "daemon-reload", "set-property", "edit", "link"):
            with self.subTest(verb=verb):
                result = run("alice", "--user", verb, "chrote-agent@ok.service")
                self.assertEqual(REFUSED, result.returncode, f"verb {verb} must be refused")

    def test_system_manager_is_unreachable(self) -> None:
        for scope in ("--system", "--global", "--machine=other"):
            with self.subTest(scope=scope):
                result = run("alice", scope, "enable", "chrote-agent@ok.service")
                self.assertEqual(REFUSED, result.returncode)


class OptionScopeTests(unittest.TestCase):
    def test_unknown_options_are_refused_rather_than_forwarded(self) -> None:
        for option in ("--force", "--root=/", "-f", "--runtime", "--no-block",
                       "--property=Exec Start", "--property=", "--global"):
            with self.subTest(option=option):
                result = run("alice", "--user", "enable", option, "chrote-agent@ok.service")
                self.assertEqual(
                    REFUSED,
                    result.returncode,
                    f"option {option!r} must not be forwarded to systemctl",
                )


class UserScopeTests(unittest.TestCase):
    def test_hostile_account_names_are_refused(self) -> None:
        for user in ("alice; id", "--user", "-", "root/../alice", "a" * 33,
                     "Alice", "al ice", ""):
            with self.subTest(user=user):
                result = run(user, "--user", "enable", "chrote-agent@ok.service")
                self.assertEqual(REFUSED, result.returncode, f"user {user!r} must be refused")

    def test_arguments_must_arrive_in_the_expected_shape(self) -> None:
        self.assertEqual(REFUSED, run("alice").returncode)
        self.assertEqual(REFUSED, run("alice", "--user").returncode)
        self.assertEqual(REFUSED, run("alice", "enable", "chrote-agent@ok.service").returncode)


class SourceContractTests(unittest.TestCase):
    """Properties easier to assert on the text than to provoke at runtime."""

    def setUp(self) -> None:
        self.text = HELPER.read_text()

    def test_helper_never_builds_a_shell_command(self) -> None:
        for pattern in ("eval ", "bash -c", "sh -c", "$(systemctl"):
            self.assertNotIn(pattern, self.text)

    def test_final_call_is_an_exec_with_an_argv(self) -> None:
        self.assertIn("exec /usr/bin/setpriv", self.text)

    def test_start_and_stop_are_not_grantable(self) -> None:
        # A verb that starts a unit without enabling it would create a lock that
        # does not survive reboot -- the promise the lock exists to make.
        allowed = next(line for line in self.text.splitlines() if "ALLOWED_VERBS=" in line)
        for verb in (" start ", " stop ", " restart "):
            self.assertNotIn(verb, allowed)

    def test_privileged_executables_are_absolute(self) -> None:
        self.assertTrue(self.text.startswith("#!/usr/bin/bash\n"))
        for path in ("/usr/bin/id", "/usr/bin/setpriv", "/usr/bin/env", "/usr/bin/systemctl"):
            self.assertIn(path, self.text)

    def test_host_service_installs_the_reviewed_helper_and_grant(self) -> None:
        sudoers = SUDOERS.read_text()
        unit = SERVER_UNIT.read_text()
        controller = CONTROLLER.read_text()
        self.assertIn("/usr/local/libexec/chrote/chrote-agentctl", sudoers)
        self.assertIn("chrote-agentctl *", sudoers)
        self.assertIn('agentUnitSudoBinary   = "/usr/bin/sudo"', controller)
        self.assertIn(
            'agentUnitHelperBinary = "/usr/local/libexec/chrote/chrote-agentctl"',
            controller,
        )
        self.assertIn("scripts/chrote-agentctl", unit)
        self.assertIn("services/chrote-agentctl.sudoers", unit)
        self.assertIn("/usr/sbin/visudo -cf", unit)


if __name__ == "__main__":
    unittest.main()


class EnvironmentIsolationTests(unittest.TestCase):
    """The helper crosses an account boundary; the caller's environment must not.

    The caller is the CHROTE web server, whose environment holds API_AUTH_TOKEN
    and service credentials. Bare `env` ADDS to that environment rather than
    replacing it, so the target account -- which can read its own
    /proc/<pid>/environ -- would receive them on every invocation.
    """

    def setUp(self) -> None:
        self.text = HELPER.read_text()

    def test_environment_is_replaced_not_extended(self) -> None:
        self.assertIn("env -i", self.text)

    def test_no_bare_env_invocation_survives(self) -> None:
        for line in self.text.splitlines():
            stripped = line.strip()
            if stripped.startswith("#"):
                continue
            self.assertNotRegex(
                stripped,
                r"(^|\s)env\s+(?!-i)[A-Z]",
                f"bare env forwards the caller's environment: {stripped!r}",
            )
