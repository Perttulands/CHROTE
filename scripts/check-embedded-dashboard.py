#!/usr/bin/env python3
"""Verify the embedded dashboard stamp matches current tracked dashboard source."""

from __future__ import annotations

import argparse
import hashlib
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_EMBED_DIR = ROOT / "src/internal/dashboard/dist"
DEFAULT_STAMP = ROOT / "src/internal/dashboard/dist.stamp"


def tracked_dashboard_files() -> list[str]:
    result = subprocess.run(
        ["git", "-C", str(ROOT), "ls-files", "dashboard"],
        capture_output=True,
        text=True,
        check=True,
    )
    return sorted(path for path in result.stdout.splitlines() if path)


def source_fingerprint() -> tuple[str, int]:
    digest = hashlib.sha256()
    files = tracked_dashboard_files()
    for name in files:
        path = ROOT / name
        digest.update(name.encode() + b"\0")
        digest.update(path.read_bytes() if path.is_file() else b"<deleted>")
        digest.update(b"\0")
    return digest.hexdigest(), len(files)


def read_stamp(path: Path) -> dict[str, str]:
    values = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if line and not line.startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip()
    return values


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--stamp", type=Path, default=DEFAULT_STAMP)
    parser.add_argument("--embed-dir", type=Path, default=DEFAULT_EMBED_DIR)
    parser.add_argument("--print-fingerprint", action="store_true")
    args = parser.parse_args()
    actual, count = source_fingerprint()
    if args.print_fingerprint:
        print(actual)
        return 0
    if not args.embed_dir.is_dir() or not args.stamp.is_file():
        print("embedded-dashboard: FAIL - bundle or stamp missing; run build-embedded-dashboard.sh")
        return 1
    stamp = read_stamp(args.stamp)
    if stamp.get("source_sha256") != actual:
        print("embedded-dashboard: FAIL - embedded bundle is stale")
        print(f"stamp={stamp.get('source_sha256', '<missing>')}")
        print(f"tree={actual} ({count} tracked dashboard files)")
        return 1
    print(f"embedded-dashboard: PASS - bundle matches {count} tracked dashboard files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
