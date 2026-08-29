#!/usr/bin/env python3
"""Reject tracked host identities and topology; report install paths with --all."""

from __future__ import annotations

import re
import subprocess
import sys

EXEMPT_PREFIXES = ("docs/archive/", "docs/plans/")
EXEMPT_FILES = {"scripts/host-neutrality.py"}
ALLOW_LINE = re.compile(r"perttus_vision_for_agent_teams_and_orchestration", re.IGNORECASE)


def rule(pattern: str, reason: str, *, ignore_case: bool = True):
    flags = re.IGNORECASE if ignore_case else 0
    return re.compile(pattern, flags), reason


TIER1 = [
    rule(r"/home/perttu\b", "real home directory"),
    rule(r"/home/tavern\b", "real home directory"),
    rule(r"/run/user/1000\b", "real operator uid in a socket path"),
    rule(r"/tmp/tmux-100[0-9]\b", "real uid-scoped tmux socket path"),
    rule(r"\btail1f2f3b\b", "real tailnet name"),
    rule(r"\bnew-chrote\b", "real host name"),
    rule(r"chrote-cockpit-tmux\.service", "host-only systemd unit"),
    rule(r"\bperttu\w*", "real operator account name", ignore_case=False),
    rule(r"\btavern\w*", "real secondary account name"),
]
TIER2 = [
    rule(r"/srv/chrote\b", "this host install path"),
    rule(r"/srv/data/chrote\b", "this host data path"),
    rule(r"\b8095\b|\b7686\b", "this host port override"),
]


def tracked() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files"], capture_output=True, text=True, check=True
    )
    return [path for path in result.stdout.splitlines() if path]


def scan(paths: list[str], rules) -> list[tuple[str, int, str, str]]:
    hits = []
    for path in paths:
        if path in EXEMPT_FILES or path.startswith(EXEMPT_PREFIXES):
            continue
        try:
            lines = open(path, encoding="utf-8", errors="strict").readlines()
        except (UnicodeDecodeError, FileNotFoundError, IsADirectoryError):
            continue
        for number, line in enumerate(lines, 1):
            if ALLOW_LINE.search(line):
                continue
            for pattern, reason in rules:
                if pattern.search(line):
                    hits.append((path, number, reason, line.strip()[:110]))
                    break
    return hits


def print_hits(hits: list[tuple[str, int, str, str]]) -> None:
    grouped: dict[str, list[tuple[int, str, str]]] = {}
    for path, number, reason, text in hits:
        grouped.setdefault(path, []).append((number, reason, text))
    for path in sorted(grouped):
        print(f"  {path}")
        for number, reason, text in grouped[path][:6]:
            print(f"    :{number}  {reason}\n        {text}")
        if len(grouped[path]) > 6:
            print(f"    ... and {len(grouped[path]) - 6} more")


def main() -> int:
    paths = tracked()
    leaks = scan(paths, TIER1)
    if leaks:
        print(f"host-neutrality: FAIL - {len(leaks)} tier-1 leak(s)")
        print_hits(leaks)
        print("Use neutral fixtures such as alice/build and /run/user/<uid>/...")
    else:
        print(f"host-neutrality: PASS - no tier-1 leaks across {len(paths)} files")
    if "--all" in sys.argv:
        advisory = scan(paths, TIER2)
        print(f"tier-2 advisory occurrences: {len(advisory)}")
        print_hits(advisory[:20])
    return 1 if leaks else 0


if __name__ == "__main__":
    raise SystemExit(main())
