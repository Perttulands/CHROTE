#!/usr/bin/env python3
"""Fail if the embedded dashboard bundle was not built from the current dashboard source.

`//go:embed dist/*` in src/internal/dashboard/embed.go means a MISSING bundle fails the
build loudly, but a STALE one does not. That asymmetry is the whole defect: on 2026-07-24
the running server served an 8-day-old UI built from an abandoned branch, and because both
dist directories are gitignored, git could never notice. A change that was already
committed looked lost.

The gate cannot rely on tree cleanliness (gitignored) or on rebuilding and comparing bytes
(vite output is not guaranteed byte-identical across environments). Instead the build script
records a stamp naming the exact dashboard source it built from, and this script recomputes
that fingerprint and compares.

The fingerprint covers the WORKING TREE contents of every tracked file under dashboard/,
not the index or HEAD, because an uncommitted edit makes the bundle just as stale as a
missing rebuild does.

Run:  python3 scripts/check-embedded-dashboard.py [--stamp PATH] [--embed-dir PATH]
Exit: 0 fresh, 1 stale or unverifiable.
"""
from __future__ import annotations

import argparse
import hashlib
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_EMBED_DIR = ROOT / "src" / "internal" / "dashboard" / "dist"
DEFAULT_STAMP = ROOT / "src" / "internal" / "dashboard" / "dist.stamp"

# node_modules and dist are untracked, so `git ls-files` excludes them for free.
# package.json and package-lock.json ARE tracked and do change the bundle.
SOURCE_ROOT = "dashboard"


def tracked_dashboard_files() -> list[str]:
    out = subprocess.run(
        ["git", "-C", str(ROOT), "ls-files", SOURCE_ROOT],
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    return sorted(p for p in out.splitlines() if p)


def source_fingerprint() -> tuple[str, int]:
    """sha256 over (path, content) of every tracked dashboard file, in sorted order."""
    digest = hashlib.sha256()
    counted = 0
    for rel in tracked_dashboard_files():
        path = ROOT / rel
        if not path.is_file():
            # Tracked but deleted in the working tree: that is itself a source change.
            digest.update(rel.encode() + b"\0<deleted>\0")
            counted += 1
            continue
        digest.update(rel.encode() + b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")
        counted += 1
    return digest.hexdigest(), counted


def read_stamp(stamp: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line in stamp.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--stamp", type=Path, default=DEFAULT_STAMP)
    parser.add_argument("--embed-dir", type=Path, default=DEFAULT_EMBED_DIR)
    parser.add_argument(
        "--print-fingerprint",
        action="store_true",
        help="print the current source fingerprint and exit; used by the build script to write the stamp",
    )
    args = parser.parse_args()

    if args.print_fingerprint:
        print(source_fingerprint()[0])
        return 0

    if not args.embed_dir.is_dir():
        print(
            f"embedded-dashboard: FAIL — no embedded bundle at {args.embed_dir}\n"
            "  Run: scripts/build-embedded-dashboard.sh",
            file=sys.stderr,
        )
        return 1

    if not args.stamp.is_file():
        print(
            f"embedded-dashboard: FAIL — no build stamp at {args.stamp}, so the bundle's\n"
            "  provenance cannot be verified. An unstamped bundle is exactly the state that\n"
            "  let an 8-day-old UI ship unnoticed on 2026-07-24.\n"
            "  Run: scripts/build-embedded-dashboard.sh",
            file=sys.stderr,
        )
        return 1

    stamp = read_stamp(args.stamp)
    recorded = stamp.get("source_sha256", "")
    actual, counted = source_fingerprint()

    if not recorded:
        print(
            f"embedded-dashboard: FAIL — {args.stamp} has no source_sha256 entry",
            file=sys.stderr,
        )
        return 1

    if recorded != actual:
        print(
            "embedded-dashboard: FAIL — the embedded bundle was built from different\n"
            "  dashboard source than the working tree contains.\n"
            f"    stamp says : {recorded}\n"
            f"    tree is    : {actual}   ({counted} tracked files under {SOURCE_ROOT}/)\n"
            f"    built at   : {stamp.get('built_at', 'unknown')}\n"
            f"    built from : {stamp.get('commit', 'unknown')}\n"
            "  Both dist directories are gitignored, so git cannot catch this and a stale\n"
            "  bundle embeds silently (go:embed only fails on a MISSING one).\n"
            "  Run: scripts/build-embedded-dashboard.sh",
            file=sys.stderr,
        )
        return 1

    print(
        f"embedded-dashboard: PASS — bundle matches {counted} tracked dashboard source files "
        f"(built {stamp.get('built_at', 'unknown')} from {stamp.get('commit', 'unknown')})"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
