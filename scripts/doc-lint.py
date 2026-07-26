#!/usr/bin/env python3
"""CHROTE docs/source-truth lint.

This intentionally starts narrow. It enforces claims the current docs already make
without dragging dirty Archon/Formations implementation work into doc cleanup.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from urllib.parse import unquote

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
PUBLIC_PRODUCT_DOCS = [
    "README.md",
    "AGENTS.md",
    "CLAUDE.md",
    "SECURITY.md",
    "CONTRIBUTING.md",
    "PRD.md",
    "COMPONENTS.md",
    "docs/CHROTE_VISION.md",
    "docs/TEST_STRATEGY.md",
    "docs/installation.md",
    "docs/troubleshooting.md",
    "docs/adr/0004-mission-rooms-agent-team-ledgers.md",
    "dashboard/README.md",
    "scripts/tmux-recovery/README.md",
    "Perttus_vision_for_agent_orchestration/spec/contracts.md",
]
HOST_LOCAL_EXEMPT = {
    "DATA-MODEL.md",
    "FORMATIONS.md",
    "Perttus_vision_for_agent_orchestration/spec/contracts.md",
}
HOST_LOCAL_TOKENS = [
    "/srv/chrote",
    "/srv/data/chrote",
    "chrote-srv.service",
    "proving lane",
    "legacy rollback lane",
]
HOST_LOCAL_PATTERNS = [
    (
        re.compile(r"/home/(?!chrote(?:/|$)|operator(?:/|$)|secondary(?:/|$))[^/\s]+/chrote"),
        "/home/<private-user>/chrote",
    ),
]
SHIPPED_VIEW_LABELS = [
    "Formations",
    "Terminal 1",
    "Terminal 2",
    "Terminal 3",
    "Files",
    "Agents",
    "Beads",
    "Services",
    "Scheduled",
    "Server",
    "Settings",
]
EXPERIMENTAL_VIEW_LABELS = []
README_ASSETS = [
    "docs/assets/readme/terminal-agents.png",
    "docs/assets/readme/beads-kanban.png",
    "docs/assets/readme/attach-hermes-workflow.mp4",
]
STALE_ROOT_MEDIA = [
    "screenshot 1.png",
    "file system.png",
    "kanban.png",
    "BV_insession.png",
    "Themes.png",
    "chat_new.png",
]
GO_BASELINE = "1.23"
NODE_BASELINE_DOC = "Node.js 20.19+ or 22.12+"
NODE_ENGINE_RANGE = "^20.19.0 || >=22.12.0"
GOVULNCHECK_VERSION = "v1.6.0"
DEVELOPMENT_VERSION = "2.0.0-alpha.2-dev"
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
        if not path.exists():
            continue
        if rel(path) in HOST_LOCAL_EXEMPT or rel(path).startswith("docs/adr/"):
            continue
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


def check_active_local_links(errors: list[str]) -> None:
    link_pattern = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")
    skipped_dirs = {"archive", "plans"}
    for path in tracked_markdown_files():
        if not path.exists():
            continue
        if rel(path) in HOST_LOCAL_EXEMPT or rel(path).startswith("docs/adr/"):
            continue
        rel_path = path.relative_to(ROOT)
        if any(part in skipped_dirs for part in rel_path.parts):
            continue
        text = path.read_text(encoding="utf-8", errors="replace")
        for match in link_pattern.finditer(text):
            raw = match.group(1).strip()
            target_text = raw.split("#", 1)[0].strip()
            if not target_text or target_text.startswith(("http://", "https://", "mailto:")):
                continue
            if target_text.startswith("<") and target_text.endswith(">"):
                target_text = target_text[1:-1]
            target = (path.parent / unquote(target_text)).resolve()
            try:
                target.relative_to(ROOT.resolve())
            except ValueError:
                fail(errors, f"{rel(path)}: local link escapes repository: {raw}")
                continue
            if not target.exists():
                fail(errors, f"{rel(path)}: broken local link: {raw}")


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


def compiled_default_ports() -> dict[str, int]:
    """Read the server's compiled default ports out of the code.

    These are the product's defaults, and the only port values docs may present as
    such. Three port stories exist in this repo and get confused for one another:
    the compiled defaults here, the container's explicit 8080/7681 in src/Dockerfile
    and src/deploy.sh, and whatever an operator passes with --port. A deployment's
    port must never be documented as the product's, and vice versa — that inversion
    has already sent work at correcting accurate documentation.
    """
    text = (ROOT / "src/cmd/server/main.go").read_text(encoding="utf-8")
    ports: dict[str, int] = {}
    for name in ("defaultServerPort", "defaultTtydPort"):
        match = re.search(rf"^\s*{name}\s*=\s*(\d+)\s*$", text, re.MULTILINE)
        if not match:
            raise SystemExit(f"doc-lint: cannot read {name} from src/cmd/server/main.go")
        ports[name] = int(match.group(1))
    return ports


def check_documented_ports_match_code(errors: list[str]) -> None:
    """Fail when a doc's stated default port disagrees with the compiled one.

    Derived from the code rather than duplicated, so changing main.go without
    updating the docs fails here instead of drifting silently.
    """
    ports = compiled_default_ports()
    server = str(ports["defaultServerPort"])
    ttyd = str(ports["defaultTtydPort"])

    for rel_path in ["README.md", "SECURITY.md", "docs/installation.md", "docs/troubleshooting.md"]:
        path = ROOT / rel_path
        if not path.exists():
            fail(errors, f"missing public product doc: {rel_path}")
            continue
        if server not in path.read_text(encoding="utf-8"):
            fail(
                errors,
                f"{rel_path}: does not name the compiled default server port {server} "
                f"(src/cmd/server/main.go defaultServerPort)",
            )

    installation = (ROOT / "docs/installation.md").read_text(encoding="utf-8")
    if ttyd not in installation:
        fail(
            errors,
            f"docs/installation.md: does not name the compiled default ttyd port {ttyd} "
            f"(src/cmd/server/main.go defaultTtydPort)",
        )


def check_security_runtime_facts(errors: list[str]) -> None:
    path = ROOT / "SECURITY.md"
    if not path.exists():
        fail(errors, "SECURITY.md missing")
        return
    text = path.read_text(encoding="utf-8")
    required = [
        "127.0.0.1",
        str(compiled_default_ports()["defaultServerPort"]),
        "HOST",
        "PORT",
        "TTYD_PORT",
        "CORS_ORIGINS",
        "CHROTE_ROOTS",
        "no built-in application login",
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


def check_public_docs_are_host_neutral(errors: list[str]) -> None:
    for rel_path in PUBLIC_PRODUCT_DOCS:
        if not (ROOT / rel_path).exists():
            fail(errors, f"missing public product doc: {rel_path}")

    for path in tracked_markdown_files():
        if not path.exists():
            continue
        if rel(path) in HOST_LOCAL_EXEMPT or rel(path).startswith("docs/adr/"):
            continue
        text = path.read_text(encoding="utf-8", errors="replace")
        for token in HOST_LOCAL_TOKENS:
            if token in text:
                fail(errors, f"{rel(path)}: contains host-local operator detail {token!r}")
        for pattern, label in HOST_LOCAL_PATTERNS:
            if pattern.search(text):
                fail(errors, f"{rel(path)}: contains host-local operator path matching {label}")


def check_current_product_views(errors: list[str]) -> None:
    path = ROOT / "PRD.md"
    if not path.exists():
        fail(errors, "PRD.md missing")
        return
    text = path.read_text(encoding="utf-8")
    missing_shipped = [label for label in SHIPPED_VIEW_LABELS if f"| {label} |" not in text]
    if missing_shipped:
        fail(errors, f"PRD.md: current view table is missing shipped views {missing_shipped}")
    missing_experimental = [
        label for label in EXPERIMENTAL_VIEW_LABELS if f"| {label} |" not in text
    ]
    if missing_experimental:
        fail(errors, f"PRD.md: current view table is missing experimental labels {missing_experimental}")



def check_readme_media(errors: list[str]) -> None:
    path = ROOT / "README.md"
    if not path.exists():
        fail(errors, "README.md missing")
        return
    text = path.read_text(encoding="utf-8")
    for rel_path in README_ASSETS:
        asset = ROOT / rel_path
        if rel_path not in text:
            fail(errors, f"README.md: missing approved media reference {rel_path}")
        if not asset.is_file() or asset.stat().st_size == 0:
            fail(errors, f"missing or empty approved README media: {rel_path}")
    remaining = [rel_path for rel_path in STALE_ROOT_MEDIA if (ROOT / rel_path).exists()]
    if remaining:
        fail(errors, f"stale root media should remain removed: {remaining}")


def check_repository_links(errors: list[str]) -> None:
    path = ROOT / "CHANGELOG.md"
    if not path.exists():
        fail(errors, "CHANGELOG.md missing")
        return
    if "github.com/user/chrote" in path.read_text(encoding="utf-8"):
        fail(errors, "CHANGELOG.md: placeholder github.com/user/chrote links remain")


def check_toolchain_contract(errors: list[str]) -> None:
    required_tokens = {
        "src/go.mod": f"go {GO_BASELINE}",
        ".github/workflows/ci.yml": f"go-version: '{GO_BASELINE}'",
        ".github/workflows/release.yml": f"go-version: '{GO_BASELINE}'",
        "CONTRIBUTING.md": f"Go {GO_BASELINE}+",
        "docs/TEST_STRATEGY.md": f"Go {GO_BASELINE}",
    }
    for rel_path, token in required_tokens.items():
        path = ROOT / rel_path
        if not path.exists():
            fail(errors, f"missing toolchain contract file: {rel_path}")
            continue
        if token not in path.read_text(encoding="utf-8"):
            fail(errors, f"{rel_path}: missing patched Go baseline token {token!r}")

    for rel_path in ["README.md", "CONTRIBUTING.md", "docs/installation.md", "docs/TEST_STRATEGY.md"]:
        text = (ROOT / rel_path).read_text(encoding="utf-8")
        if NODE_BASELINE_DOC not in text:
            fail(errors, f"{rel_path}: missing Node baseline token {NODE_BASELINE_DOC!r}")

    scanner = f"golang.org/x/vuln/cmd/govulncheck@{GOVULNCHECK_VERSION}"
    for rel_path in [".github/workflows/release.yml"]:
        text = (ROOT / rel_path).read_text(encoding="utf-8")
        if scanner not in text:
            fail(errors, f"{rel_path}: missing pinned vulnerability scanner {scanner}")
        if "-mode=binary" not in text:
            fail(errors, f"{rel_path}: missing release-binary vulnerability scan")


def check_version_contract(errors: list[str]) -> None:
    required_tokens = {
        "VERSION": DEVELOPMENT_VERSION,
        "src/cmd/server/main.go": f'var Version = "{DEVELOPMENT_VERSION}"',
        "src/internal/api/health.go": f'version: "{DEVELOPMENT_VERSION}"',
        ".github/workflows/release.yml": "-X main.Version=$release_version",
    }
    for rel_path, token in required_tokens.items():
        path = ROOT / rel_path
        if token not in path.read_text(encoding="utf-8"):
            fail(errors, f"{rel_path}: missing version contract token {token!r}")

    dashboard_package = json.loads((ROOT / "dashboard/package.json").read_text(encoding="utf-8"))
    dashboard_lock = json.loads((ROOT / "dashboard/package-lock.json").read_text(encoding="utf-8"))
    if dashboard_package.get("version") != DEVELOPMENT_VERSION:
        fail(errors, "dashboard/package.json: version must match VERSION")
    if dashboard_package.get("private") is not True:
        fail(errors, "dashboard/package.json: embedded dashboard package must remain private")
    if dashboard_package.get("engines", {}).get("node") != NODE_ENGINE_RANGE:
        fail(errors, "dashboard/package.json: Node engine range must match the supported baseline")
    if dashboard_lock.get("version") != DEVELOPMENT_VERSION:
        fail(errors, "dashboard/package-lock.json: root version must match VERSION")
    lock_package = dashboard_lock.get("packages", {}).get("", {})
    if lock_package.get("version") != DEVELOPMENT_VERSION:
        fail(errors, "dashboard/package-lock.json: package metadata version must match VERSION")
    if lock_package.get("engines", {}).get("node") != NODE_ENGINE_RANGE:
        fail(errors, "dashboard/package-lock.json: Node engine range must match package.json")

    release = (ROOT / ".github/workflows/release.yml").read_text(encoding="utf-8")
    if 'release_version="${GITHUB_REF_NAME#v}"' not in release:
        fail(errors, ".github/workflows/release.yml: release version must derive from the pushed tag")
    if 'source_version="$(tr -d \'\\r\\n\' < VERSION)"' not in release or 'if [ "$source_version" != "$release_version" ]' not in release:
        fail(errors, ".github/workflows/release.yml: pushed tag must match VERSION before publishing")
    if "dist/SHA256SUMS" not in release or "sha256sum chrote-server-linux-amd64 chrote-server-linux-arm64" not in release:
        fail(errors, ".github/workflows/release.yml: release binaries must publish a SHA256SUMS file")
    if re.search(r"^\s+(?:install|uninstall)\.sh\s*$", release, re.MULTILINE):
        fail(errors, ".github/workflows/release.yml: checkout-dependent scripts must not be standalone release assets")


def main() -> int:
    errors: list[str] = []
    check_active_spec_frontmatter(errors)
    check_enforced_by_paths(errors)
    check_source_truth_index(errors)
    check_active_local_links(errors)
    check_theme_docs(errors)
    check_documented_ports_match_code(errors)
    check_security_runtime_facts(errors)
    check_public_docs_are_host_neutral(errors)
    check_current_product_views(errors)
    check_readme_media(errors)
    check_repository_links(errors)
    check_toolchain_contract(errors)
    check_version_contract(errors)

    if errors:
        print("doc-lint: FAIL")
        for error in errors:
            print(f"- {error}")
        return 1
    print("doc-lint: PASS")
    print(f"checked active specs: {', '.join(ACTIVE_SPECS)}")
    print(f"checked source-truth index: {INDEX_PATH.as_posix()}")
    print("checked active local Markdown links and approved README media")
    return 0


if __name__ == "__main__":
    sys.exit(main())
