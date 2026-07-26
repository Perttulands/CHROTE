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

TIER 1 splits into TOPOLOGY (machine facts: paths, uids, host and tailnet names) and IDENTITY
(account names). Matching is case-insensitive except where a rule says otherwise, and every
exception is pinned by scripts/test_host_neutrality.py. That test exists because the case policy
used to be accidental: nothing set IGNORECASE, so this gate reported "PASS — no tier-1 leaks"
while a capitalized secondary account name sat in CONTRIBUTING.md. A gate whose guarantee is a
side effect of how its regexes happened to be written is worse than no gate.
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
# These two necessarily contain the patterns they forbid: the rules themselves, and the test that
# proves each rule fires. Keep this set to files whose SUBJECT is the rules — not files that merely
# find the rules inconvenient.
EXEMPT_FILES = {'scripts/host-neutrality.py', 'scripts/test_host_neutrality.py'}

# Suffixed forms matter: a fixture named `tavern1` leaks just as much as `tavern`, and a
# word-boundary-only rule silently permits them (it did — a sweep left `tavern1` behind).
ALLOW_LINE = re.compile(r'perttus_vision_for_agent_teams_and_orchestration', re.IGNORECASE)


def rule(pattern: str, why: str, *, ignore_case: bool = True):
    return (re.compile(pattern, re.IGNORECASE if ignore_case else 0), why)


# TOPOLOGY — where this machine keeps things. Case-insensitive: `/HOME/PERTTU` is the same leak as
# `/home/perttu`, and a path that reaches a reader in either case is equally useful for recon. These
# rules currently match nothing, which is the point: they exist to stop a regression, not to find one.
TOPOLOGY = [
    rule(r'/home/perttu\b', 'real home directory'),
    rule(r'/home/tavern\b', 'real home directory'),
    rule(r'/run/user/1000\b', 'real operator uid in a socket path'),
    rule(r'/tmp/tmux-100[0-9]\b', 'real uid-scoped tmux socket path'),
    rule(r'\btail1f2f3b\b', 'real tailnet name'),
    rule(r'\bnew-chrote\b', 'real host name'),
    rule(r'chrote-cockpit-tmux\.service', 'host-only systemd unit (not tracked here)'),
]

# IDENTITY — who runs this machine. The two names are NOT symmetric, and the case rules below encode
# that difference deliberately. Before, both were case-sensitive by accident, which silently let
# `Tavern` through in prose (it did — CONTRIBUTING.md named it while this gate reported PASS).
IDENTITY = [
    # Unix account names are lowercase, so lowercase `perttu` is the ACCOUNT — a host fact worth
    # scrubbing. Capitalized `Perttu` is prose: authorship, quotes, decision attribution. Scrubbing
    # that would buy nothing (LICENSE and the clone URL in CONTRIBUTING.md name the owner publicly)
    # and would cost truthful design records — a blind sweep already fabricated one dead reference
    # that way. Case-sensitive ON PURPOSE; see test_host_neutrality.py, which pins both directions.
    rule(r'\bperttu\w*', 'real operator account name (lowercase = the Unix account)', ignore_case=False),
    # The secondary account has no public identity to be consistent with, so any case is a leak.
    rule(r'\btavern\w*', 'real secondary account name'),
]

TIER1 = TOPOLOGY + IDENTITY
# NOT tier-1, deliberately:
#   8094 / 7683 are the PRODUCT's compiled defaults (src/cmd/server/main.go:34-35). Docs naming them are
#     correct. It is 8095/7686 that are this host's overrides, passed as unit flags — so those are the
#     host-specific values, which is the opposite of what it looks like from the running service.
#   services/chrote-srv.service is a tracked, parameterised product artifact ($CHROTE_SERVICE_ROOT, ${PORT}),
#     so naming it is not a leak.
TIER2 = [
    rule(r'/srv/chrote\b', 'this host install path'),
    rule(r'/srv/data/chrote\b', 'this host data path'),
    rule(r'\b8095\b|\b7686\b', "this host's port override (product default is 8094/7683)"),
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
                if pattern.search(line):
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
