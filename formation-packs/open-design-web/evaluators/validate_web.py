#!/usr/bin/env python3
"""Deterministic structural integrity gate for a routed web prototype."""
from __future__ import annotations

import json
import re
import sys
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlparse

MAX_HTML_BYTES = 2_000_000
PLACEHOLDERS = ("lorem ipsum", "todo:", "placeholder text", "replace me")


class AuditParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.lang = ""
        self.in_title = False
        self.title_parts: list[str] = []
        self.has_viewport = False
        self.main_count = 0
        self.h1_count = 0
        self.ids: list[str] = []
        self.labels_for: set[str] = set()
        self.inputs: list[dict[str, str]] = []
        self.images: list[dict[str, str]] = []
        self.buttons: list[tuple[dict[str, str], list[str]]] = []
        self.current_button: list[str] | None = None
        self.links: list[str] = []
        self.local_assets: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        values = {key.lower(): (value or "") for key, value in attrs}
        tag = tag.lower()
        if tag == "html":
            self.lang = values.get("lang", "").strip()
        elif tag == "title":
            self.in_title = True
        elif tag == "main":
            self.main_count += 1
        elif tag == "h1":
            self.h1_count += 1
        elif tag == "meta" and values.get("name", "").lower() == "viewport" and values.get("content", "").strip():
            self.has_viewport = True
        elif tag == "label" and values.get("for"):
            self.labels_for.add(values["for"])
        elif tag in {"input", "select", "textarea"}:
            self.inputs.append(values)
        elif tag == "img":
            self.images.append(values)
        elif tag == "button":
            self.current_button = []
            self.buttons.append((values, self.current_button))
        elif tag == "a":
            self.links.append(values.get("href", ""))
        if values.get("id"):
            self.ids.append(values["id"])
        asset_ref = ""
        if tag in {"img", "script", "source", "video", "audio"}:
            asset_ref = values.get("src", "").strip()
        elif tag == "link":
            asset_ref = values.get("href", "").strip()
        if asset_ref and is_local_ref(asset_ref):
            self.local_assets.append(asset_ref)

    def handle_endtag(self, tag: str) -> None:
        if tag.lower() == "title":
            self.in_title = False
        elif tag.lower() == "button":
            self.current_button = None

    def handle_data(self, data: str) -> None:
        if self.in_title:
            self.title_parts.append(data)
        if self.current_button is not None:
            self.current_button.append(data)


def is_local_ref(ref: str) -> bool:
    parsed = urlparse(ref)
    return not parsed.scheme and not parsed.netloc and not ref.startswith(("#", "data:", "mailto:", "tel:"))


def inside(path: Path, root: Path) -> bool:
    try:
        path.relative_to(root)
        return True
    except ValueError:
        return False


def main() -> int:
    if len(sys.argv) != 2:
        print(json.dumps({"ok": False, "errors": ["usage: validate_web.py <workspace-relative-index.html>"]}))
        return 2
    root = Path.cwd().resolve()
    supplied = Path(sys.argv[1])
    artifact = (supplied if supplied.is_absolute() else root / supplied).resolve()
    errors: list[str] = []
    warnings: list[str] = []
    if not inside(artifact, root):
        errors.append("artifact escapes workspace")
    elif artifact.suffix.lower() != ".html":
        errors.append("artifactRef must point to an HTML file")
    elif not artifact.is_file():
        errors.append("artifact file does not exist")
    elif artifact.stat().st_size == 0 or artifact.stat().st_size > MAX_HTML_BYTES:
        errors.append("artifact HTML is empty or exceeds size limit")
    if errors:
        print(json.dumps({"ok": False, "artifact": str(artifact), "errors": errors, "warnings": warnings}, sort_keys=True))
        return 1

    text = artifact.read_text(encoding="utf-8")
    lower = text.lower()
    parser = AuditParser()
    try:
        parser.feed(text)
    except Exception as exc:
        errors.append(f"HTML parser failed: {exc}")
    if "<!doctype html" not in lower:
        errors.append("missing HTML doctype")
    if not parser.lang:
        errors.append("html lang attribute is required")
    if not "".join(parser.title_parts).strip():
        errors.append("non-empty title is required")
    if parser.main_count != 1:
        errors.append("exactly one main landmark is required")
    if parser.h1_count != 1:
        errors.append("exactly one h1 is required")
    duplicates = sorted({value for value in parser.ids if parser.ids.count(value) > 1})
    if duplicates:
        errors.append("duplicate ids: " + ", ".join(duplicates))
    for control in parser.inputs:
        control_id = control.get("id", "")
        if not control.get("aria-label") and not control.get("aria-labelledby") and (not control_id or control_id not in parser.labels_for):
            errors.append("form control lacks an accessible label")
    for image in parser.images:
        if "alt" not in image:
            errors.append("image lacks alt attribute")
    for attrs, text_parts in parser.buttons:
        visible = "".join(text_parts).strip()
        if not visible and not attrs.get("aria-label") and not attrs.get("aria-labelledby"):
            errors.append("button lacks an accessible name")
    if any(token in lower for token in PLACEHOLDERS):
        errors.append("artifact contains placeholder content")
    if not parser.has_viewport:
        errors.append("responsive viewport meta tag is required")
    if not re.search(r":focus(?:-visible)?[\s,{]", text, re.IGNORECASE):
        warnings.append("no explicit focus style found in primary HTML")
    if ("animation:" in lower or "transition:" in lower) and "prefers-reduced-motion" not in lower:
        warnings.append("motion exists without an evident reduced-motion rule")
    for ref in parser.local_assets:
        target = (artifact.parent / urlparse(ref).path).resolve()
        if not inside(target, root):
            errors.append(f"local asset escapes workspace: {ref}")
        elif not target.is_file():
            errors.append(f"local asset is missing: {ref}")

    result = {
        "ok": not errors,
        "artifact": str(artifact.relative_to(root)),
        "checks": {"main": parser.main_count, "h1": parser.h1_count, "ids": len(parser.ids), "localAssets": len(parser.local_assets)},
        "errors": sorted(set(errors)),
        "warnings": sorted(set(warnings)),
    }
    print(json.dumps(result, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
