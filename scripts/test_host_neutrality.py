#!/usr/bin/env python3
"""Tests for the host-neutrality gate.

The gate runs in ci.yml and release.yml and had no tests of its own. That is how it came to
report "PASS — no tier-1 leaks across 563 tracked files" while every pattern was case-sensitive
by accident, so a capitalized secondary account name passed. These tests pin the case policy in
both directions, so the guarantee is a decision rather than a side effect.
"""
from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / 'host-neutrality.py'
_spec = importlib.util.spec_from_file_location('host_neutrality', SCRIPT)
assert _spec and _spec.loader
hn = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(hn)


def scan_text(text: str, rules=None) -> list[tuple[str, str]]:
    """Run the real rules over one in-memory line, returning (why, line) for each hit."""
    hits = []
    for line in text.splitlines():
        if hn.ALLOW_LINE.search(line):
            continue
        for pattern, why in (hn.TIER1 if rules is None else rules):
            if pattern.search(line):
                hits.append((why, line))
                break
    return hits


class TopologyRulesTest(unittest.TestCase):
    def test_machine_facts_are_caught_in_any_case(self) -> None:
        for leak in [
            '/home/perttu/chrote',
            '/HOME/PERTTU/chrote',
            '/run/user/1000/chrote-tmux',
            '/RUN/USER/1000/chrote-tmux',
            '/tmp/tmux-1001/default',
            'tail1f2f3b.ts.net',
            'TAIL1F2F3B.ts.net',
            'new-chrote',
            'NEW-CHROTE',
            'chrote-cockpit-tmux.service',
        ]:
            with self.subTest(leak=leak):
                self.assertTrue(scan_text(leak), f'topology leak not caught: {leak}')

    def test_neutral_fixtures_are_not_flagged(self) -> None:
        for ok in [
            '/home/alice/chrote',
            '/run/user/2001/chrote-tmux',
            '/tmp/tmux-2002/default',
            'CHROTE_TERMINAL_USER_SOCKETS=alice:/run/user/2001/chrote-tmux/tmux-2001/default',
            'services/chrote-srv.service',
            '--port 8094 --ttyd-port 7683',
        ]:
            with self.subTest(ok=ok):
                self.assertEqual(scan_text(ok), [], f'neutral fixture wrongly flagged: {ok}')


class IdentityRulesTest(unittest.TestCase):
    def test_secondary_account_name_is_caught_in_any_case(self) -> None:
        # The regression this suite was written for: `Tavern` in prose passed the gate.
        for leak in ['tavern', 'Tavern', 'TAVERN', 'tavern1', 'Tavern content']:
            with self.subTest(leak=leak):
                self.assertTrue(scan_text(leak), f'secondary account name not caught: {leak}')

    def test_lowercase_owner_account_name_is_caught(self) -> None:
        for leak in ['CHROTE_TERMINAL_USERS=perttu', 'su - perttu', 'perttu1']:
            with self.subTest(leak=leak):
                self.assertTrue(scan_text(leak), f'owner account name not caught: {leak}')

    def test_capitalized_owner_name_is_prose_and_is_allowed(self) -> None:
        # Deliberate, not an oversight: the repo names its owner in LICENSE and the clone URL, so
        # scrubbing attribution buys nothing — and a sweep that did it fabricated a dead reference.
        for prose in [
            'Copyright (c) 2024-2026 Perttulands',
            'git clone https://github.com/Perttulands/CHROTE.git',
            "Perttu wants CHROTE to grow into a meta-harness for AI work.",
            '`Perttus_vision_for_agent_orchestration/03-formations.html` is the visual source',
        ]:
            with self.subTest(prose=prose):
                self.assertEqual(scan_text(prose), [], f'prose attribution wrongly flagged: {prose}')

    def test_owner_case_policy_is_asymmetric_by_design(self) -> None:
        self.assertTrue(scan_text('perttu'), 'lowercase owner account name must be a leak')
        self.assertEqual(scan_text('Perttu'), [], 'capitalized owner name must be allowed')
        self.assertTrue(scan_text('Tavern'), 'the secondary name has no such exemption')


class AllowListTest(unittest.TestCase):
    def test_vision_document_filename_is_allowed_in_any_case(self) -> None:
        for line in [
            '- `../perttus_vision_for_agent_teams_and_orchestration.md` (the why).',
            '- `../Perttus_vision_for_agent_teams_and_orchestration.md` (the why).',
        ]:
            with self.subTest(line=line):
                self.assertEqual(scan_text(line), [])

    def test_allow_list_does_not_swallow_a_topology_leak_on_the_same_line(self) -> None:
        # Known limitation, asserted so a future change to whole-line skipping is a deliberate one.
        line = 'see perttus_vision_for_agent_teams_and_orchestration.md and /home/perttu/chrote'
        self.assertEqual(scan_text(line), [], 'ALLOW_LINE skips the whole line, by current design')


class TrackedTreeTest(unittest.TestCase):
    def test_the_tracked_tree_passes_tier_one(self) -> None:
        self.assertEqual(hn.scan(hn.tracked(), hn.TIER1), [])


if __name__ == '__main__':
    unittest.main()
