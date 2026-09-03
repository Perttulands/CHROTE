#!/usr/bin/env python3
"""Check CHROTE source-truth frontmatter, shipped views, and local links."""

from __future__ import annotations

import re
import subprocess
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
ACTIVE_SPECS = ["DESIGN-SYSTEM.md"]
REQUIRED_FRONTMATTER = {
    "type": "spec",
    "status": "active",
    "authority": "source-of-truth",
    "workspace": "chrote",
    "enforced_by": "scripts/doc-lint.py",
}
SHIPPED_VIEWS = [
    "Terminal 1",
    "Terminal 2",
    "Terminal 3",
    "Files",
    "Beads",
    "Library",
    "Scheduled",
    "Server",
    "Settings",
]
LINK_PATTERN = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")


def relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def tracked_markdown() -> list[Path]:
    result = subprocess.run(
        ["git", "ls-files", "*.md"],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=True,
    )
    return [ROOT / name for name in result.stdout.splitlines() if name]


def frontmatter(path: Path, errors: list[str]) -> dict[str, str]:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---\n"):
        errors.append(f"{relative(path)}: missing YAML frontmatter")
        return {}
    end = text.find("\n---\n", 4)
    if end < 0:
        errors.append(f"{relative(path)}: unterminated YAML frontmatter")
        return {}
    values: dict[str, str] = {}
    for line in text[4:end].splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if ":" not in line:
            errors.append(f"{relative(path)}: invalid frontmatter line {line!r}")
            continue
        key, value = line.split(":", 1)
        values[key.strip()] = value.strip().strip("\"'")
    return values


def check_frontmatter(errors: list[str]) -> None:
    for name in ACTIVE_SPECS:
        path = ROOT / name
        if not path.is_file():
            errors.append(f"missing active source-truth spec: {name}")
            continue
        values = frontmatter(path, errors)
        for key, expected in REQUIRED_FRONTMATTER.items():
            if values.get(key) != expected:
                errors.append(f"{name}: frontmatter {key!r} must be {expected!r}")


def check_views(errors: list[str]) -> None:
    text = (ROOT / "PRD.md").read_text(encoding="utf-8")
    missing = [view for view in SHIPPED_VIEWS if f"| {view} |" not in text]
    if missing:
        errors.append(f"PRD.md: current view table is missing shipped views {missing}")


def check_links(errors: list[str]) -> None:
    for path in tracked_markdown():
        if not path.is_file():
            continue
        parts = path.relative_to(ROOT).parts
        if "archive" in parts or "plans" in parts or parts[:2] == ("docs", "adr"):
            continue
        for match in LINK_PATTERN.finditer(path.read_text(encoding="utf-8", errors="replace")):
            raw = match.group(1).strip()
            target = raw.split("#", 1)[0].strip().strip("<>")
            if not target or target.startswith(("http://", "https://", "mailto:")):
                continue
            resolved = (path.parent / unquote(target)).resolve()
            try:
                resolved.relative_to(ROOT.resolve())
            except ValueError:
                errors.append(f"{relative(path)}: local link escapes repository: {raw}")
                continue
            if not resolved.exists():
                errors.append(f"{relative(path)}: broken local link: {raw}")


def main() -> int:
    errors: list[str] = []
    check_frontmatter(errors)
    check_views(errors)
    check_links(errors)
    if errors:
        print("doc-lint: FAIL")
        for error in errors:
            print(f"- {error}")
        return 1
    print("doc-lint: PASS (frontmatter, PRD views, local links)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
