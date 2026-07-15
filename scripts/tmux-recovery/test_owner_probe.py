#!/usr/bin/env python3
from __future__ import annotations

import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import owner_probe


FIXTURES = Path(__file__).resolve().parent / "fixtures"


def load_fixture(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def assert_subset(test: unittest.TestCase, got, want, path: str = "$") -> None:
    if isinstance(want, dict):
        test.assertIsInstance(got, dict, path)
        for key, value in want.items():
            test.assertIn(key, got, f"{path}.{key}")
            assert_subset(test, got[key], value, f"{path}.{key}")
        return
    if isinstance(want, list):
        test.assertEqual(got, want, path)
        return
    test.assertEqual(got, want, path)


def collect_keys(value) -> set[str]:
    if isinstance(value, dict):
        keys = set(value.keys())
        for child in value.values():
            keys.update(collect_keys(child))
        return keys
    if isinstance(value, list):
        keys: set[str] = set()
        for child in value:
            keys.update(collect_keys(child))
        return keys
    return set()


def contains_value(value, needle: str) -> bool:
    if isinstance(value, dict):
        return any(contains_value(child, needle) for child in value.values())
    if isinstance(value, list):
        return any(contains_value(child, needle) for child in value)
    return value == needle


class OwnerProbeFixtureTest(unittest.TestCase):
    def test_classifies_fixture_inputs_without_raw_argv_or_env(self) -> None:
        fixture_paths = sorted(FIXTURES.glob("*.json"))
        self.assertGreater(len(fixture_paths), 0, "expected fixture files")
        for path in fixture_paths:
            with self.subTest(path=path.name):
                fixture = load_fixture(path)
                original_input = copy.deepcopy(fixture["input"])
                got = owner_probe.classify_pane(fixture["input"])
                self.assertEqual(fixture["input"], original_input, f"{path.name} input was mutated")
                assert_subset(self, got, fixture["want"])
                if got.get("mode") == "unresolved":
                    self.assertFalse(got["owner"]["mayRestart"], f"{path.name} unresolved output must not restart: {got}")
                keys = collect_keys(got)
                self.assertNotIn("argv", keys)
                self.assertNotIn("env", keys)
                for forbidden in fixture.get("forbiddenValues", []):
                    self.assertFalse(contains_value(got, forbidden), f"{path.name} emitted forbidden value {forbidden!r}: {got}")


if __name__ == "__main__":
    unittest.main()
