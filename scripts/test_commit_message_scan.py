#!/usr/bin/env python3
"""Tests for the commit-message credential scanner.

Range semantics are proven against real throwaway git repositories built in temp
dirs, so BASE..HEAD exclusivity comes from git itself rather than from a
reimplementation of it. Every fixture value is fake, and one test asserts the
scanner's failure output never echoes those fake values — names only.
"""
from __future__ import annotations

import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "commit-message-scan.py"

CLEAN_MESSAGE = "Routine fixture change\n\nNothing sensitive here.\n"
ENV_BLOCK_MESSAGE = (
    "Add deploy notes\n"
    "\n"
    "FOO=fixture-value-alpha\n"
    "BAR=fixture-value-bravo\n"
    "BAZ=fixture-value-charlie\n"
)
SYSTEMD_MESSAGE = (
    "Document the service unit\n"
    "\n"
    "ExecStart=/usr/local/bin/fixture-daemon --flag\n"
    "Restart=always\n"
    "RestartSec=2\n"
)
TOKEN_MESSAGE = "Configure auth\n\nFOO_TOKEN=deadbeefcafefeed\n"
FAKE_VALUES = [
    "fixture-value-alpha",
    "fixture-value-bravo",
    "fixture-value-charlie",
    "deadbeefcafefeed",
]


def hermetic_env() -> dict[str, str]:
    env = dict(os.environ)
    env.update(
        {
            "GIT_CONFIG_GLOBAL": "/dev/null",
            "GIT_CONFIG_SYSTEM": "/dev/null",
            "GIT_AUTHOR_NAME": "Fixture",
            "GIT_AUTHOR_EMAIL": "fixture@example.invalid",
            "GIT_COMMITTER_NAME": "Fixture",
            "GIT_COMMITTER_EMAIL": "fixture@example.invalid",
        }
    )
    return env


class ScannerRepoTest(unittest.TestCase):
    def setUp(self) -> None:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.repo = Path(tmp.name)
        self.env = hermetic_env()
        self.git("init", "-q")

    def git(self, *args: str) -> str:
        proc = subprocess.run(
            ["git", "-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false", *args],
            cwd=self.repo,
            env=self.env,
            text=True,
            capture_output=True,
        )
        self.assertEqual(proc.returncode, 0, f"git {' '.join(args)} failed: {proc.stderr}")
        return proc.stdout.strip()

    def commit(self, message: str) -> str:
        """Create an empty commit with the given message; return its SHA."""
        message_file = self.repo / ".git" / "fixture-message"
        message_file.write_text(message, encoding="utf-8")
        self.git("commit", "--allow-empty", "-F", str(message_file))
        return self.git("rev-parse", "HEAD")

    def scan(self, revisions: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), revisions],
            cwd=self.repo,
            env=self.env,
            text=True,
            capture_output=True,
        )

    def test_bad_commit_inside_range_fails(self) -> None:
        base = self.commit(CLEAN_MESSAGE)
        bad = self.commit(ENV_BLOCK_MESSAGE)
        result = self.scan(f"{base}..HEAD")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn(bad, result.stdout, "failure output must name the offending SHA")

    def test_same_bad_commit_outside_range_passes(self) -> None:
        self.commit(CLEAN_MESSAGE)
        bad = self.commit(ENV_BLOCK_MESSAGE)
        self.commit(CLEAN_MESSAGE)
        # BASE..HEAD excludes BASE itself, so the bad commit sits just outside.
        result = self.scan(f"{bad}..HEAD")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_three_consecutive_uppercase_env_lines_fail(self) -> None:
        bad = self.commit(ENV_BLOCK_MESSAGE)
        result = self.scan(bad)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        for name in ("FOO", "BAR", "BAZ"):
            self.assertIn(name, result.stdout)

    def test_three_consecutive_systemd_directives_pass(self) -> None:
        sha = self.commit(SYSTEMD_MESSAGE)
        result = self.scan(sha)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_high_signal_token_fails_and_names_the_variable(self) -> None:
        bad = self.commit(TOKEN_MESSAGE)
        result = self.scan(bad)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("FOO_TOKEN", result.stdout)

    def test_failure_output_never_contains_the_values(self) -> None:
        base = self.commit(CLEAN_MESSAGE)
        self.commit(ENV_BLOCK_MESSAGE)
        self.commit(TOKEN_MESSAGE)
        result = self.scan(f"{base}..HEAD")
        self.assertNotEqual(result.returncode, 0, result.stdout)
        output = result.stdout + result.stderr
        for value in FAKE_VALUES:
            self.assertNotIn(value, output, "scanner output must never echo values")

    def test_invalid_revision_is_a_hard_failure(self) -> None:
        self.commit(CLEAN_MESSAGE)
        for revisions in (
            "no-such-ref-fixture..HEAD",
            "no-such-ref-fixture",
            "",
        ):
            with self.subTest(revisions=revisions):
                result = self.scan(revisions)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn("FAIL", result.stdout)


if __name__ == "__main__":
    unittest.main()
