#!/usr/bin/env python3
"""Fail if tracked files leak this host's deployment specifics.

CHROTE is a public repository describing a private cockpit. The product must be reproducible by
someone else, which means no real operator usernames, uids, socket paths, install paths, systemd unit
names, or retired-lane ports in tracked files. Deployment specifics belong in untracked operator
configuration (/etc/chrote/chrote-srv.env on this host), never here.

AGENTS.md and SECURITY.md already state that contract. Before this script it was enforced only over
markdown, via `git ls-files '*.md'`, so real usernames and socket paths sat in tracked Go tests and
shipped scripts while doc-lint reported PASS. The lint's own scope encoded the leak.

Run:  python3 scripts/host-neutrality.py [--all]
Exit: 0 clean, 1 leaks found.

By default only TIER 1 is enforced — values that identify a specific human or machine. --all also
reports TIER 2 (install paths), which is advisory while the docs still use concrete examples.
"""
from __future__ import annotations
import re
import subprocess
import sys

# Historical by design: archives and dated plans record what WAS true and are not a contract.
EXEMPT_PREFIXES = (
    'docs/archive/',
    'docs/plans/',
    'Perttus_vision_for_agent_orchestration/archive/',
)
# This file necessarily contains the patterns it forbids.
EXEMPT_FILES = {'scripts/host-neutrality.py'}

# Suffixed forms matter: a fixture named `tavern1` leaks just as much as `tavern`, and a
# word-boundary-only rule silently permits them (it did — a sweep left `tavern1` behind).
ALLOW_LINE = re.compile(r'perttus_vision_for_agent_teams_and_orchestration')

TIER1 = [
    (r'\bperttu\w*', 'real operator username'),
    (r'\btavern\w*', 'real secondary username'),
    (r'/home/perttu\b', 'real home directory'),
    (r'/home/tavern\b', 'real home directory'),
    (r'/run/user/1000\b', "real operator uid in a socket path"),
    (r'/tmp/tmux-100[0-9]\b', 'real uid-scoped tmux socket path'),
    (r'\btail1f2f3b\b', 'real tailnet name'),
    (r'\bnew-chrote\b', 'real host name'),
    (r'chrote-cockpit-tmux\.service', 'host-only systemd unit (not tracked here)'),
]
# NOT tier-1, deliberately:
#   8094 / 7683 are the PRODUCT's compiled defaults (src/cmd/server/main.go:34-35). Docs naming them are
#     correct. It is 8095/7686 that are this host's overrides, passed as unit flags — so those are the
#     host-specific values, which is the opposite of what it looks like from the running service.
#   services/chrote-srv.service is a tracked, parameterised product artifact ($CHROTE_SERVICE_ROOT, ${PORT}),
#     so naming it is not a leak.
TIER2 = [
    (r'/srv/chrote\b', 'this host install path'),
    (r'/srv/data/chrote\b', 'this host data path'),
    (r'\b8095\b|\b7686\b', "this host's port override (product default is 8094/7683)"),
]


def tracked() -> list[str]:
    out = subprocess.run(['git', 'ls-files'], capture_output=True, text=True, check=True).stdout
    return [p for p in out.splitlines() if p]


def scan(paths, rules):
    hits = []
    for path in paths:
        if path in EXEMPT_FILES or path.startswith(EXEMPT_PREFIXES):
            continue
        try:
            with open(path, 'r', encoding='utf-8', errors='strict') as fh:
                lines = fh.readlines()
        except (UnicodeDecodeError, FileNotFoundError, IsADirectoryError):
            continue  # binary or vanished; nothing textual to leak
        for n, line in enumerate(lines, 1):
            if ALLOW_LINE.search(line):
                continue  # the vision design-doc filename is the owner's own naming, not host config
            for pattern, why in rules:
                if re.search(pattern, line):
                    hits.append((path, n, why, line.strip()[:110]))
                    break
    return hits


def main() -> int:
    show_all = '--all' in sys.argv
    paths = tracked()
    t1 = scan(paths, TIER1)

    if t1:
        print(f'host-neutrality: FAIL — {len(t1)} tier-1 leak(s) in tracked files\n')
        by_file: dict[str, list] = {}
        for path, n, why, text in t1:
            by_file.setdefault(path, []).append((n, why, text))
        for path in sorted(by_file):
            print(f'  {path}')
            for n, why, text in by_file[path][:6]:
                print(f'    :{n}  {why}\n        {text}')
            if len(by_file[path]) > 6:
                print(f'    ... and {len(by_file[path]) - 6} more in this file')
        print('\nDeployment specifics belong in untracked operator configuration, not the public repo.')
        print('Use neutral fixtures instead: usernames alice/build, sockets /run/user/<uid>/... or')
        print('/tmp/tmux-<uid>/..., the compiled defaults 8094/7683 for ports, and describe')
        print('host-only systemd units generically rather than by name.')
    else:
        print(f'host-neutrality: PASS — no tier-1 leaks across {len(paths)} tracked files')

    if show_all:
        t2 = scan(paths, TIER2)
        print(f'\ntier-2 (advisory, install paths): {len(t2)} occurrence(s)')
        for path, n, why, _ in t2[:20]:
            print(f'  {path}:{n}  {why}')
        if len(t2) > 20:
            print(f'  ... and {len(t2) - 20} more')

    return 1 if t1 else 0


if __name__ == '__main__':
    raise SystemExit(main())
