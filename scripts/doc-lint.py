#!/usr/bin/env python3
"""CHROTE docs/source-truth lint.

This intentionally starts narrow. It enforces claims the current docs already make
without dragging dirty Archon/Formations implementation work into doc cleanup.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

ACTIVE_SPECS = [
    "ARCHON.md",
    "FORMATIONS.md",
    "DATA-MODEL.md",
    "DESIGN-SYSTEM.md",
]
REQUIRED_FRONTMATTER = {
    "type": "spec",
    "status": "active",
    "authority": "source-of-truth",
    "workspace": "chrote",
    "enforced_by": "scripts/doc-lint.py",
}
INDEX_PATH = Path("docs/source-truth-index.md")
LANE_AWARE_DOCS = [
    "README.md",
    "SECURITY.md",
    "CONTRIBUTING.md",
    "PRD.md",
    "COMPONENTS.md",
    "CLAUDE.md",
    "AGENTS.md",
    "docs/troubleshooting.md",
    "docs/TEST_STRATEGY.md",
    "dashboard/README.md",
]
LANE_TOKENS = {
    "srv": [
        "/srv/chrote",
        "/srv/data/chrote",
        "chrote-srv.service",
        "8095",
        "7686",
    ],
    "legacy": [
        "/home/perttu/chrote",
        "chrote.service",
        "8094",
        "7683",
    ],
}
INTENTIONALLY_ABSENT = [
    "CHROTE.md",
    "SPEC-CHANGELOG.md",
    "ARCHON_BDD.md",
    "FORMATIONS_BDD.md",
    "scripts/dead-link-check.py",
    "scripts/agent-check",
]


def rel(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def fail(errors: list[str], message: str) -> None:
    errors.append(message)


def parse_frontmatter(path: Path, errors: list[str]) -> dict[str, str]:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---\n"):
        fail(errors, f"{rel(path)}: missing YAML frontmatter")
        return {}
    end = text.find("\n---\n", 4)
    if end == -1:
        fail(errors, f"{rel(path)}: unterminated YAML frontmatter")
        return {}
    frontmatter: dict[str, str] = {}
    for line_no, raw in enumerate(text[4:end].splitlines(), start=2):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if ":" not in line:
            fail(errors, f"{rel(path)}:{line_no}: invalid frontmatter line {raw!r}")
            continue
        key, value = line.split(":", 1)
        frontmatter[key.strip()] = value.strip().strip('"\'')
    return frontmatter


def tracked_markdown_files() -> list[Path]:
    # Use git if available so generated/vendor markdown does not become a noisy gate.
    import subprocess

    try:
        proc = subprocess.run(
            ["git", "ls-files", "*.md"],
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=True,
        )
    except Exception:
        return sorted(p for p in ROOT.rglob("*.md") if ".git" not in p.parts and "node_modules" not in p.parts)
    return [ROOT / line for line in proc.stdout.splitlines() if line.strip()]


def check_active_spec_frontmatter(errors: list[str]) -> None:
    for spec in ACTIVE_SPECS:
        path = ROOT / spec
        if not path.exists():
            fail(errors, f"missing active source-truth spec: {spec}")
            continue
        frontmatter = parse_frontmatter(path, errors)
        for key, expected in REQUIRED_FRONTMATTER.items():
            actual = frontmatter.get(key)
            if actual != expected:
                fail(errors, f"{spec}: frontmatter {key!r} is {actual!r}, expected {expected!r}")


def check_enforced_by_paths(errors: list[str]) -> None:
    pattern = re.compile(r"^\s*enforced_by\s*:\s*(.+?)\s*$")
    for path in tracked_markdown_files():
        try:
            lines = path.read_text(encoding="utf-8").splitlines()
        except UnicodeDecodeError:
            continue
        for line_no, line in enumerate(lines, start=1):
            match = pattern.match(line)
            if not match:
                continue
            target = match.group(1).strip().strip('"\'')
            target_path = ROOT / target
            if not target_path.exists():
                fail(errors, f"{rel(path)}:{line_no}: enforced_by target does not exist: {target}")


def check_source_truth_index(errors: list[str]) -> None:
    path = ROOT / INDEX_PATH
    if not path.exists():
        fail(errors, f"missing {INDEX_PATH.as_posix()}")
        return
    text = path.read_text(encoding="utf-8")

    required_mentions = [
        "FORMATIONS.md",
        "ARCHON.md",
        "DATA-MODEL.md",
        "DESIGN-SYSTEM.md",
        "PRD.md",
        "SECURITY.md",
        "docs/archive/",
        "Perttus_vision_for_agent_orchestration/spec/",
        "scripts/doc-lint.py",
    ]
    for mention in required_mentions:
        if mention not in text:
            fail(errors, f"{INDEX_PATH.as_posix()}: missing required mention {mention!r}")

    for missing in INTENTIONALLY_ABSENT:
        if missing not in text:
            fail(errors, f"{INDEX_PATH.as_posix()}: does not explicitly classify absent doc/tool {missing}")

    link_pattern = re.compile(r"\[[^\]]+\]\(([^)]+)\)")
    for match in link_pattern.finditer(text):
        raw = match.group(1).split("#", 1)[0]
        if not raw or raw.startswith(("http://", "https://", "mailto:")):
            continue
        target = (path.parent / raw).resolve()
        try:
            target.relative_to(ROOT.resolve())
        except ValueError:
            fail(errors, f"{INDEX_PATH.as_posix()}: link escapes repo: {match.group(1)}")
            continue
        if not target.exists():
            fail(errors, f"{INDEX_PATH.as_posix()}: broken local link: {match.group(1)}")


def theme_ids_from_types(errors: list[str]) -> list[str]:
    path = ROOT / "dashboard/src/types.ts"
    if not path.exists():
        fail(errors, "dashboard/src/types.ts missing; cannot validate theme ids")
        return []
    text = path.read_text(encoding="utf-8")
    match = re.search(r"theme:\s*([^\n]+)// Color theme", text)
    if not match:
        fail(errors, "dashboard/src/types.ts: cannot find UserSettings theme union")
        return []
    return re.findall(r"'([^']+)'", match.group(1))


def check_theme_docs(errors: list[str]) -> None:
    ids = theme_ids_from_types(errors)
    if not ids:
        return
    expected_block = "\n".join(ids)
    data_model = (ROOT / "DATA-MODEL.md").read_text(encoding="utf-8")
    design_system = (ROOT / "DESIGN-SYSTEM.md").read_text(encoding="utf-8")
    if expected_block not in data_model:
        fail(errors, "DATA-MODEL.md: current theme id block does not match dashboard/src/types.ts")
    table_ids = re.findall(r"\| `([^`]+)` \|", design_system)
    missing = [theme for theme in ids if theme not in table_ids]
    extra = [theme for theme in table_ids if theme not in ids]
    if missing or extra:
        fail(errors, f"DESIGN-SYSTEM.md: theme table mismatch; missing={missing}, extra={extra}")


def check_security_runtime_facts(errors: list[str]) -> None:
    path = ROOT / "SECURITY.md"
    if not path.exists():
        fail(errors, "SECURITY.md missing")
        return
    text = path.read_text(encoding="utf-8")
    required = [
        "127.0.0.1",
        "8094",
        "HOST",
        "PORT",
        "TTYD_PORT",
        "API_AUTH_TOKEN",
        "CORS_ORIGINS",
        "CHROTE_ROOTS",
    ]
    for token in required:
        if token not in text:
            fail(errors, f"SECURITY.md: missing current runtime/security token {token!r}")
    stale = [
        "CHROTE_HOST",
        "CHROTE_PORT",
        "default: 8080",
        "default: 0.0.0.0",
        "The server binds to `0.0.0.0`",
        "No built-in authentication is provided",
    ]
    for token in stale:
        if token in text:
            fail(errors, f"SECURITY.md: stale runtime/security text remains: {token!r}")


def check_service_lane_docs(errors: list[str]) -> None:
    for rel_path in LANE_AWARE_DOCS:
        path = ROOT / rel_path
        if not path.exists():
            fail(errors, f"missing lane-aware doc: {rel_path}")
            continue
        text = path.read_text(encoding="utf-8")
        missing_srv = [token for token in LANE_TOKENS["srv"] if token not in text]
        if missing_srv:
            fail(errors, f"{rel_path}: missing /srv proving lane facts {missing_srv}")
        legacy_present = any(token in text for token in LANE_TOKENS["legacy"])
        if legacy_present and "legacy rollback" not in text:
            fail(errors, f"{rel_path}: legacy runtime facts must be labeled as legacy rollback")


def main() -> int:
    errors: list[str] = []
    check_active_spec_frontmatter(errors)
    check_enforced_by_paths(errors)
    check_source_truth_index(errors)
    check_theme_docs(errors)
    check_security_runtime_facts(errors)
    check_service_lane_docs(errors)

    if errors:
        print("doc-lint: FAIL")
        for error in errors:
            print(f"- {error}")
        return 1
    print("doc-lint: PASS")
    print(f"checked active specs: {', '.join(ACTIVE_SPECS)}")
    print(f"checked source-truth index: {INDEX_PATH.as_posix()}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
