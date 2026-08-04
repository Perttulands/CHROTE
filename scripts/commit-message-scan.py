#!/usr/bin/env python3
"""CHROTE commit-message credential scan.

Detection, not enforcement: this scans git commit messages for pasted environment
blocks and for assignments to high-signal credential names, then reports the
offending commit SHAs and the variable NAMES that triggered — never the values.

Usage: commit-message-scan.py <BASE..HEAD | REVISION>

The single argument is either a revision range ("BASE..HEAD", scanning every
commit `git rev-list BASE..HEAD` yields) or one revision (scanning exactly that
commit). Git runs in the current working directory. An invalid, empty, or
unresolvable revision argument is a hard failure — there are deliberately no
fallback ranges, so a scan that cannot resolve its input fails loudly instead of
quietly scanning something else (or nothing).
"""

from __future__ import annotations

import re
import subprocess
import sys

# A pasted environment block: uppercase-only names, so mixed-case systemd
# directives such as ExecStart= or RestartSec= deliberately do not match.
ENV_LINE = re.compile(r"^[A-Z_][A-Z0-9_]*=.+$")
ENV_RUN_THRESHOLD = 3

# Any NAME=value occurrence in a line, however the line is prefixed (bullet,
# indentation, `export`, URL query), as long as a value follows the equals sign.
ASSIGNMENT = re.compile(r"(?<![\w.-])([A-Za-z_][A-Za-z0-9_]*)=(?!\s|$)")
HIGH_SIGNAL_SUFFIXES = ("_token", "_secret", "_key", "_password")
HIGH_SIGNAL_NAMES = {"API_AUTH_TOKEN", "CHROTE_CREATION_TOKEN"}


def fail(errors: list[str], message: str) -> None:
    errors.append(message)


def run_git(args: list[str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], text=True, capture_output=True)


def resolve_commits(revisions: str) -> tuple[list[str], str | None]:
    """Resolve the argument to full commit SHAs, oldest first.

    Returns (shas, error). Every resolution problem is an error; nothing here
    falls back to a different range.
    """
    if not revisions.strip():
        return [], "empty revision argument"
    if revisions.startswith("-"):
        return [], f"revision argument must not start with '-': {revisions!r}"
    if ".." in revisions:
        proc = run_git(["rev-list", "--reverse", revisions])
        if proc.returncode != 0:
            return [], f"cannot resolve revision range {revisions!r}: {proc.stderr.strip()}"
        shas = [line.strip() for line in proc.stdout.splitlines() if line.strip()]
        if not shas:
            return [], f"revision range {revisions!r} contains no commits"
        return shas, None
    proc = run_git(["rev-parse", "--verify", "--quiet", f"{revisions}^{{commit}}"])
    if proc.returncode != 0:
        detail = proc.stderr.strip()
        suffix = f": {detail}" if detail else ""
        return [], f"cannot resolve revision {revisions!r}{suffix}"
    return [proc.stdout.strip()], None


def commit_message(sha: str) -> tuple[str, str | None]:
    proc = run_git(["show", "-s", "--format=%B", sha])
    if proc.returncode != 0:
        return "", f"cannot read commit message for {sha}: {proc.stderr.strip()}"
    return proc.stdout, None


def scan_message(message: str) -> list[str]:
    """Return findings for one commit message. Variable names only, never values."""
    findings: list[str] = []
    lines = message.splitlines()

    run_names: list[str] = []

    def flush_run() -> None:
        if len(run_names) >= ENV_RUN_THRESHOLD:
            findings.append(
                f"{len(run_names)} consecutive environment-style assignment lines "
                f"({', '.join(run_names)})"
            )
        run_names.clear()

    for line in lines:
        if ENV_LINE.match(line):
            run_names.append(line.split("=", 1)[0])
        else:
            flush_run()
    flush_run()

    seen: set[str] = set()
    for line in lines:
        for match in ASSIGNMENT.finditer(line):
            name = match.group(1)
            if name in seen:
                continue
            if name.lower().endswith(HIGH_SIGNAL_SUFFIXES) or name in HIGH_SIGNAL_NAMES:
                seen.add(name)
                findings.append(f"high-signal name assigned a value: {name}")
    return findings


def main(argv: list[str]) -> int:
    if len(argv) != 1:
        print("commit-message-scan: FAIL")
        print("- usage: commit-message-scan.py <BASE..HEAD | REVISION>")
        return 1

    revisions = argv[0]
    shas, resolve_error = resolve_commits(revisions)
    if resolve_error:
        print("commit-message-scan: FAIL")
        print(f"- {resolve_error}")
        return 1

    errors: list[str] = []
    for sha in shas:
        message, read_error = commit_message(sha)
        if read_error:
            fail(errors, read_error)
            continue
        for finding in scan_message(message):
            fail(errors, f"{sha}: {finding}")

    if errors:
        print("commit-message-scan: FAIL")
        for error in errors:
            print(f"- {error}")
        return 1
    print(f"commit-message-scan: PASS ({len(shas)} commit(s) scanned for {revisions})")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
