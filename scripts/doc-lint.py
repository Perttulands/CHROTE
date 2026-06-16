#!/usr/bin/env python3
"""Doc lint: keep the ARCHON.md command surface honest against the real CLI.

The root spec docs declare `enforced_by: scripts/doc-lint.py`. This is that
enforcement. Its core job is the ARCHON.md invariant the spec promises: every
documented `archon <noun> <verb>` maps to a real handler in
`src/cmd/archon/main.go`, and every implemented verb is documented. A drift in
either direction fails loudly with the exact offending pairs.

It is intentionally narrow and dependency-free (stdlib only) so it runs anywhere
Python 3 runs, including CI, without a venv.

Checks:
  1. The four root specs carry the expected front-matter (type/status/authority,
     and the `enforced_by: scripts/doc-lint.py` claim this script makes true).
  2. ARCHON.md's "Command groups" block and main.go's dispatch switch describe
     exactly the same set of `noun verb` pairs (the doctor noun is matched on its
     own subcommands).

Exit code is non-zero on any failure, with a precise report of what diverged.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
ARCHON_MD = REPO_ROOT / "ARCHON.md"
MAIN_GO = REPO_ROOT / "src" / "cmd" / "archon" / "main.go"

# Root specs that must carry the enforced-by front matter. doc-lint asserts the
# claim it itself makes is present and correctly pointed at this script.
ENFORCED_SPECS = ["ARCHON.md", "FORMATIONS.md", "DATA-MODEL.md", "DESIGN-SYSTEM.md"]


def fail(messages: list[str]) -> None:
    print("doc-lint: FAIL", file=sys.stderr)
    for message in messages:
        print("  - " + message, file=sys.stderr)
    sys.exit(1)


def parse_front_matter(text: str) -> dict[str, str]:
    if not text.startswith("---"):
        return {}
    end = text.find("\n---", 3)
    if end == -1:
        return {}
    block = text[3:end]
    matter: dict[str, str] = {}
    for line in block.splitlines():
        if ":" in line:
            key, _, value = line.partition(":")
            matter[key.strip()] = value.strip()
    return matter


def check_front_matter(problems: list[str]) -> None:
    for name in ENFORCED_SPECS:
        path = REPO_ROOT / name
        if not path.exists():
            problems.append(f"{name}: spec file is missing")
            continue
        matter = parse_front_matter(path.read_text())
        if matter.get("type") != "spec":
            problems.append(f"{name}: front matter type must be 'spec'")
        if matter.get("status") != "active":
            problems.append(f"{name}: front matter status must be 'active'")
        if matter.get("enforced_by") != "scripts/doc-lint.py":
            problems.append(
                f"{name}: front matter must declare 'enforced_by: scripts/doc-lint.py'"
            )


def documented_pairs(problems: list[str]) -> set[tuple[str, str]]:
    """Parse ARCHON.md's ```text Command groups block into (noun, verb) pairs."""
    text = ARCHON_MD.read_text()
    section = re.search(r"## Command groups\s*```text\n(.*?)```", text, re.DOTALL)
    if not section:
        problems.append("ARCHON.md: could not find the '## Command groups' text block")
        return set()
    pairs: set[tuple[str, str]] = set()
    for raw in section.group(1).splitlines():
        line = raw.strip()
        if not line.startswith("archon "):
            continue
        body = line[len("archon "):].strip()
        # noun is the first token; the rest is `verb | verb | verb`.
        parts = body.split(None, 1)
        if len(parts) != 2:
            problems.append(f"ARCHON.md: command line has no verbs: {line!r}")
            continue
        noun, verbs_blob = parts[0], parts[1]
        for verb in verbs_blob.split("|"):
            verb = verb.strip()
            if verb:
                pairs.add((noun, verb))
    return pairs


def implemented_pairs(problems: list[str]) -> set[tuple[str, str]]:
    """Parse main.go's `run()` dispatch switch into (noun, verb) pairs.

    The dispatch is `switch args[0]` (noun) -> `switch args[1]` (verb). The
    doctor noun is dispatched separately to runDoctor, whose subcommands are read
    from runDoctor's own switch.
    """
    text = MAIN_GO.read_text()
    pairs: set[tuple[str, str]] = set()

    run_match = re.search(r"\n\tswitch args\[0\] \{\n(.*?)\n\tdefault:", text, re.DOTALL)
    if not run_match:
        problems.append("main.go: could not locate the top-level dispatch switch")
        return set()
    body = run_match.group(1)

    noun = None
    in_inner = False
    for line in body.splitlines():
        outer = re.match(r'\tcase "([^"]+)":', line)
        if outer:
            noun = outer.group(1)
            in_inner = False
            continue
        if re.match(r"\t\tswitch args\[1\] \{", line):
            in_inner = True
            continue
        if re.match(r"\t\tdefault:", line):
            in_inner = False
            continue
        if in_inner and noun:
            inner = re.match(r'\t\tcase (.+):', line)
            if inner:
                for token in inner.group(1).split(","):
                    verb = token.strip().strip('"')
                    if verb:
                        pairs.add((noun, verb))

    # doctor is dispatched to runDoctor; read its own subcommand switch.
    doctor_match = re.search(
        r"func runDoctor\(.*?\n\tswitch sub \{\n(.*?)\n\tdefault:", text, re.DOTALL
    )
    if not doctor_match:
        problems.append("main.go: could not locate runDoctor's subcommand switch")
    else:
        for line in doctor_match.group(1).splitlines():
            inner = re.match(r'\tcase "([^"]+)":', line)
            if inner:
                pairs.add(("doctor", inner.group(1)))

    return pairs


def check_command_surface(problems: list[str]) -> None:
    documented = documented_pairs(problems)
    implemented = implemented_pairs(problems)
    if not documented or not implemented:
        return

    undocumented = sorted(implemented - documented)
    unimplemented = sorted(documented - implemented)

    for noun, verb in undocumented:
        problems.append(
            f"main.go implements `archon {noun} {verb}` but ARCHON.md does not document it"
        )
    for noun, verb in unimplemented:
        problems.append(
            f"ARCHON.md documents `archon {noun} {verb}` but main.go has no handler for it"
        )


def main() -> None:
    problems: list[str] = []
    check_front_matter(problems)
    check_command_surface(problems)
    if problems:
        fail(problems)
    print("doc-lint: OK (command surface and spec front matter are consistent)")


if __name__ == "__main__":
    main()
