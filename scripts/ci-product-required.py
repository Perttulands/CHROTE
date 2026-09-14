#!/usr/bin/env python3
"""Print false only when the complete commit diff contains documentation alone."""

import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys


ROOT_DOCS = {
    "ARCHITECTURE.md", "CHANGELOG.md", "COMPONENTS.md", "CONTRIBUTING.md",
    "DESIGN-SYSTEM.md", "PRD.md", "README.md", "SECURITY.md", "VISION.md",
}


def documentation(path):
    name = PurePosixPath(path)
    if name.name in {"AGENTS.md", "CLAUDE.md", "SKILL.md"}:
        return False
    return path in ROOT_DOCS or (
        path.startswith("docs/") and (
            name.suffix == ".md" or (
                path.startswith(("docs/assets/", "docs/images/"))
                and name.suffix in {".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg"}
            )
        )
    )


def product_required():
    event_name = os.environ["GITHUB_EVENT_NAME"]
    # Manual runs always prove the full product, even for documentation commits.
    if event_name == "workflow_dispatch":
        return True
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    if event_name == "push":
        base = event["before"]
    elif event_name == "pull_request":
        # Checkout and GITHUB_SHA identify the proposed merge, not the PR head.
        # Compare that tree with the base it will merge into.
        base = event["pull_request"]["base"]["sha"]
    else:
        return True
    head = os.environ["GITHUB_SHA"]
    for revision in (base, head):
        if not re.fullmatch(r"[0-9a-f]{40}", revision) or set(revision) == {"0"}:
            return True
    # No rename detection: both the removed and added paths must be documentation.
    # NUL delimiters preserve unusual file names, including embedded newlines.
    changed = subprocess.check_output([
        "git", "diff", "--no-renames", "--raw", "-z", base, head, "--",
    ]).decode("utf-8").split("\0")
    records = changed[:-1]
    if not records or len(records) % 2:
        return True
    for metadata, path in zip(records[::2], records[1::2]):
        old_mode, new_mode, _old_hash, _new_hash, status = metadata.split()
        # Executables, symlinks, submodules and type changes need full checks.
        if old_mode[1:] not in {"000000", "100644"} or new_mode not in {"000000", "100644"}:
            return True
        if status not in {"A", "M", "D"} or not documentation(path):
            return True
    return False


if __name__ == "__main__":
    try:
        required = product_required()
    except (KeyError, ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Cannot classify changes; running full product checks: {error}", file=sys.stderr)
        required = True
    print(str(required).lower())
