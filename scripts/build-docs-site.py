#!/usr/bin/env python3
"""Build the CHROTE documentation site from Markdown, with the standard library.

The guide pages live in docs/site/pages/. The canonical documents (README,
PRD, SECURITY, docs/installation.md and the rest) are rendered beside them,
unchanged, so the site links to the source of truth instead of copying it.

    python3 scripts/build-docs-site.py             # writes docs/site/_site/
    python3 scripts/build-docs-site.py --serve     # builds, then serves on 127.0.0.1:8000
    python3 scripts/build-docs-site.py --check     # builds into a temporary directory and
                                                   # fails on a broken internal link or anchor

The output is static and uses relative links only, so it opens from disk and
can be published under any path later.
"""

from __future__ import annotations

import argparse
import functools
import html
import http.server
import json
import re
import shutil
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path, PurePosixPath
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
SITE = ROOT / "docs" / "site"
PAGES = SITE / "pages"
STATIC = SITE / "static"
DEFAULT_OUT = SITE / "_site"
MARKER = ".chrote-docs-site"
REPOSITORY = "https://github.com/Perttulands/CHROTE"
BLOB = f"{REPOSITORY}/blob/main/"
FONTS = ["JetBrainsMono-Regular.woff2", "JetBrainsMono-Bold.woff2", "chrote-term-symbols.woff2"]

# Canonical documents rendered under source/. Order is the order of the index.
SOURCES = [
    ("README.md", "readme", "README", "The project landing page"),
    ("VISION.md", "vision", "Vision", "Why CHROTE exists"),
    ("PRD.md", "prd", "Product requirements", "The durable product contract and non-goals"),
    ("ARCHITECTURE.md", "architecture", "Architecture", "Runtime shape, state ownership and boundaries"),
    ("SECURITY.md", "security", "Security policy", "The trust model and vulnerability reporting"),
    ("DESIGN-SYSTEM.md", "design-system", "Design system", "Visual and interaction contract"),
    ("COMPONENTS.md", "components", "Components", "Runtime component and integration map"),
    ("docs/journeys.md", "journeys", "Operator journeys", "The seven journeys every control answers to"),
    ("docs/installation.md", "installation", "Installation reference", "Install, environment and upgrade contract"),
    ("docs/troubleshooting.md", "troubleshooting", "Troubleshooting reference", "Boundary-by-boundary diagnosis"),
    ("docs/scheduled-tasks.md", "scheduled-tasks", "Scheduled tasks API", "Task schema, API and safety contract"),
    ("CONTRIBUTING.md", "contributing", "Contributing", "Setup, gates and pull requests"),
    ("docs/TEST_STRATEGY.md", "test-strategy", "Test strategy", "Test layers and CI routing"),
    ("dashboard/README.md", "dashboard-readme", "Dashboard README", "Dashboard contributor map"),
    ("docs/source-truth-index.md", "source-truth-index", "Source-truth index", "Which document wins a conflict"),
    ("docs/private-beads-sidecar.md", "private-beads-sidecar", "Private Beads sidecar", "Private Beads transport contract"),
    ("CHANGELOG.md", "changelog", "Changelog", "What changed, release by release"),
]
ADR_DIR = "docs/adr"

ALERTS = {"NOTE": "Note", "TIP": "Tip", "IMPORTANT": "Important", "WARNING": "Warning", "CAUTION": "Caution"}
INLINE_TAGS = re.compile(r"</?(?:kbd|br|sup|sub|abbr)(?:\s+title=\"[^\"<>]*\")?\s*/?>", re.IGNORECASE)
KEY_PATTERN = re.compile(
    r"^(?:(?:Alt|Ctrl|Shift|Cmd|Leader)\+)+(?:[A-Za-z0-9/?=+\-]|Plus|Minus|Space|Enter|Tab|Escape|Esc)"
    r"(?:\s*(?:…|\.\.\.)\s*(?:Alt\+)?[0-9A-Z])?$|^(?:Escape|Esc|Enter|Tab|Alt|AltGr|Ctrl|Shift)$"
)


def slugify(text: str) -> str:
    """GitHub's heading anchors, so links written for GitHub land here too."""
    text = re.sub(r"<[^>]+>", "", text).strip().lower()
    text = re.sub(r"[^\w\- ]", "", text)
    return text.replace(" ", "-")


# --------------------------------------------------------------------------- #
# Markdown                                                                     #
# --------------------------------------------------------------------------- #


@dataclass
class Rendered:
    html: str
    title: str
    headings: list[tuple[int, str, str]] = field(default_factory=list)  # level, id, text
    text: str = ""


class Markdown:
    """A Markdown subset: the constructs CHROTE's documents use, GitHub-flavoured."""

    def __init__(self, link: callable):
        self.link = link  # (raw href) -> (href, is_external)
        self.headings: list[tuple[int, str, str]] = []
        self.slugs: dict[str, int] = {}
        self.refs: dict[str, str] = {}

    # ---- inline -------------------------------------------------------- #

    def inline(self, text: str) -> str:
        """Placeholders share one stash across nested calls, so a link label can
        hold code that was set aside before the link was found."""
        self.depth = getattr(self, "depth", 0) + 1
        if self.depth == 1:
            self.stash = []
        try:
            return self._inline(text)
        finally:
            self.depth -= 1

    def _inline(self, text: str) -> str:
        stash = self.stash

        def keep(value: str) -> str:
            stash.append(value)
            return f"\x00{len(stash) - 1}\x00"

        def code(match: re.Match) -> str:
            body = match.group(2).strip() if match.group(2).strip() else match.group(2)
            cls = ' class="key"' if KEY_PATTERN.match(body) else ""
            return keep(f"<code{cls}>{html.escape(body)}</code>")

        text = re.sub(r"(`+)(.+?)\1", code, text)
        text = re.sub(r"\\([\\`*_{}\[\]()#+\-.!|<>~])", lambda m: keep(html.escape(m.group(1))), text)
        text = re.sub(r"<(https?://[^>\s]+)>", lambda m: keep(self.anchor(m.group(1), html.escape(m.group(1)))), text)
        text = INLINE_TAGS.sub(lambda m: keep(m.group(0)), text)

        def image(match: re.Match) -> str:
            href, _ = self.link(match.group(2))
            alt = html.escape(match.group(1), quote=True)
            return keep(f'<img src="{html.escape(href, quote=True)}" alt="{alt}" loading="lazy">')

        text = re.sub(r"!\[([^\]]*)\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)", image, text)

        def link(match: re.Match) -> str:
            label = self.inline(match.group(1))
            return keep(self.anchor(match.group(2), label))

        text = re.sub(r"\[((?:[^\[\]]|\[[^\[\]]*\])+)\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)", link, text)

        def reference(match: re.Match) -> str:
            key = match.group(1).lower()
            if key not in self.refs:
                return match.group(0)
            return keep(self.anchor(self.refs[key], self.inline(match.group(1))))

        text = re.sub(r"\[([^\[\]]+)\](?!\()", reference, text)
        text = html.escape(text, quote=False)
        text = re.sub(r"\*\*(?=\S)(.+?)(?<=\S)\*\*", r"<strong>\1</strong>", text, flags=re.S)
        text = re.sub(r"(?<![\w*])\*(?=[^\s*])(.+?)(?<=[^\s*])\*(?![\w*])", r"<em>\1</em>", text, flags=re.S)
        text = re.sub(r"(?<![\w])_(?=\S)(.+?)(?<=\S)_(?![\w])", r"<em>\1</em>", text, flags=re.S)
        if self.depth == 1:
            while "\x00" in text:
                text = re.sub(r"\x00(\d+)\x00", lambda m: stash[int(m.group(1))], text)
        return text

    def anchor(self, raw: str, label: str) -> str:
        href, external = self.link(raw.strip("<>"))
        attrs = ' rel="noopener"' if external else ""
        return f'<a href="{html.escape(href, quote=True)}"{attrs}>{label}</a>'

    # ---- blocks -------------------------------------------------------- #

    def render(self, source: str) -> str:
        lines = source.replace("\t", "    ").splitlines()
        kept = []
        for line in lines:
            match = re.match(r"^\[([^\]]+)\]:\s+(\S+)\s*$", line)
            if match:
                self.refs[match.group(1).lower()] = match.group(2)
            else:
                kept.append(line)
        return self.blocks(kept)

    def blocks(self, lines: list[str]) -> str:
        out: list[str] = []
        i = 0
        while i < len(lines):
            line = lines[i]
            stripped = line.strip()
            if not stripped:
                i += 1
                continue
            fence = re.match(r"^(\s*)(`{3,}|~{3,})\s*([\w+-]*)", line)
            if fence:
                indent, marker, lang = len(fence.group(1)), fence.group(2), fence.group(3)
                body = []
                i += 1
                while i < len(lines) and not lines[i].strip().startswith(marker):
                    body.append(lines[i][indent:] if lines[i][:indent].strip() == "" else lines[i])
                    i += 1
                i += 1
                out.append(self.code_block("\n".join(body), lang))
                continue
            heading = re.match(r"^(#{1,6})\s+(.*?)\s*#*\s*$", line)
            if heading:
                out.append(self.heading(len(heading.group(1)), heading.group(2)))
                i += 1
                continue
            if re.match(r"^\s*([-*_])(\s*\1){2,}\s*$", line):
                out.append("<hr>")
                i += 1
                continue
            if stripped.startswith(">"):
                quote = []
                while i < len(lines) and lines[i].strip().startswith(">"):
                    quote.append(re.sub(r"^\s*>\s?", "", lines[i]))
                    i += 1
                out.append(self.blockquote(quote))
                continue
            if stripped.startswith("|") and i + 1 < len(lines) and re.match(r"^\s*\|?\s*:?-{2,}", lines[i + 1]):
                table = []
                while i < len(lines) and lines[i].strip().startswith("|"):
                    table.append(lines[i])
                    i += 1
                out.append(self.table(table))
                continue
            if re.match(r"^<(div|section|figure|nav|aside|table|details|ol|ul|p|a|span|pre)\b", stripped):
                raw = []
                while i < len(lines) and lines[i].strip():
                    raw.append(lines[i])
                    i += 1
                out.append(self.raw_html("\n".join(raw)))
                continue
            if re.match(r"^\s*(?:[-*+]|\d+[.)])\s+", line):
                consumed, rendered = self.list_block(lines, i)
                out.append(rendered)
                i = consumed
                continue
            para = [stripped]
            i += 1
            while i < len(lines):
                nxt = lines[i]
                if (not nxt.strip() or re.match(r"^(#{1,6}\s|```|~~~|>|\s*\|)", nxt.strip())
                        or re.match(r"^\s*(?:[-*+]|\d+[.)])\s+", nxt)):
                    break
                para.append(nxt.strip())
                i += 1
            text = "\n".join(para)
            out.append(f"<p>{self.inline(text)}</p>")
        return "\n".join(out)

    def raw_html(self, block: str) -> str:
        def rewrite(match: re.Match) -> str:
            href, _ = self.link(match.group(2))
            return f'{match.group(1)}="{html.escape(href, quote=True)}"'

        return re.sub(r'\b(href|src)="([^"]+)"', rewrite, block)

    def heading(self, level: int, text: str) -> str:
        explicit = re.search(r"\s*\{#([\w-]+)\}\s*$", text)
        if explicit:
            text = text[: explicit.start()]
        base = explicit.group(1) if explicit else slugify(text)
        count = self.slugs.get(base, 0)
        self.slugs[base] = count + 1
        anchor = base if count == 0 else f"{base}-{count}"
        rendered = self.inline(text)
        plain = re.sub(r"<[^>]+>", "", rendered)
        self.headings.append((level, anchor, html.unescape(plain)))
        if level == 1:
            return f'<h1 id="{anchor}">{rendered}</h1>'
        return (f'<h{level} id="{anchor}">{rendered}'
                f'<a class="anchor" href="#{anchor}" aria-label="Link to this section">#</a></h{level}>')

    def code_block(self, body: str, lang: str) -> str:
        escaped = html.escape(body)
        if lang in {"bash", "sh", "shell", "console", "ini", "env", "text", "dotenv", ""}:
            escaped = re.sub(r"(?m)^(\s*#.*)$", r'<span class="c">\1</span>', escaped)
        label = {"bash": "shell", "sh": "shell", "": "text"}.get(lang, lang)
        return (f'<div class="code" data-lang="{html.escape(label)}"><div class="code-head">'
                f'<span class="code-lang">{html.escape(label)}</span>'
                f'<button type="button" class="copy" data-copy>Copy</button></div>'
                f"<pre><code>{escaped}</code></pre></div>")

    def blockquote(self, lines: list[str]) -> str:
        first = lines[0].strip() if lines else ""
        alert = re.match(r"^\[!(\w+)\]\s*$", first)
        if alert and alert.group(1).upper() in ALERTS:
            kind = alert.group(1).upper()
            body = self.blocks(lines[1:])
            return (f'<aside class="callout callout-{kind.lower()}" role="note">'
                    f'<p class="callout-label">{ALERTS[kind]}</p>{body}</aside>')
        return f"<blockquote>{self.blocks(lines)}</blockquote>"

    def table(self, rows: list[str]) -> str:
        def cells(row: str) -> list[str]:
            row = row.strip()
            row = row[1:] if row.startswith("|") else row
            row = row[:-1] if row.endswith("|") and not row.endswith("\\|") else row
            return [c.strip().replace("\\|", "|") for c in re.split(r"(?<!\\)\|", row)]

        head, align_row, body = cells(rows[0]), cells(rows[1]), [cells(r) for r in rows[2:]]
        aligns = []
        for spec in align_row:
            if spec.startswith(":") and spec.endswith(":"):
                aligns.append("center")
            elif spec.endswith(":"):
                aligns.append("right")
            else:
                aligns.append("")

        def style(index: int) -> str:
            value = aligns[index] if index < len(aligns) else ""
            return f' style="text-align:{value}"' if value else ""

        parts = ['<div class="table-wrap" tabindex="0"><table><thead><tr>']
        parts += [f"<th scope=\"col\"{style(n)}>{self.inline(c)}</th>" for n, c in enumerate(head)]
        parts.append("</tr></thead><tbody>")
        for row in body:
            parts.append("<tr>" + "".join(f"<td{style(n)}>{self.inline(c)}</td>" for n, c in enumerate(row)) + "</tr>")
        parts.append("</tbody></table></div>")
        return "".join(parts)

    def list_block(self, lines: list[str], start: int) -> tuple[int, str]:
        first = re.match(r"^(\s*)([-*+]|\d+[.)])\s+", lines[start])
        base_indent = len(first.group(1))
        ordered = first.group(2)[0].isdigit()
        items: list[list[str]] = []
        loose = False
        i = start
        content_indent = len(first.group(0))
        while i < len(lines):
            line = lines[i]
            marker = re.match(r"^(\s*)([-*+]|\d+[.)])\s+", line)
            if marker and len(marker.group(1)) == base_indent and marker.group(2)[0].isdigit() == ordered:
                content_indent = len(marker.group(0))
                items.append([line[content_indent:]])
                i += 1
                continue
            if not line.strip():
                # A blank line continues the list only if indented content follows.
                j = i + 1
                while j < len(lines) and not lines[j].strip():
                    j += 1
                if j < len(lines):
                    nxt = lines[j]
                    indent = len(nxt) - len(nxt.lstrip())
                    sibling = re.match(r"^(\s*)([-*+]|\d+[.)])\s+", nxt)
                    if indent >= content_indent or (sibling and len(sibling.group(1)) == base_indent
                                                    and sibling.group(2)[0].isdigit() == ordered):
                        loose = loose or not sibling or indent >= content_indent
                        items[-1].append("")
                        i += 1
                        continue
                break
            indent = len(line) - len(line.lstrip())
            if indent >= min(content_indent, base_indent + 2):
                items[-1].append(line[min(indent, content_indent):])
                i += 1
                continue
            if marker and len(marker.group(1)) < base_indent:
                break
            if re.match(r"^(#{1,6}\s|```|>|\|)", line.strip()) or marker:
                break
            items[-1].append(line.strip())  # lazy continuation
            i += 1
        tag = "ol" if ordered else "ul"
        start_attr = ""
        if ordered:
            number = int(re.match(r"\s*(\d+)", lines[start]).group(1))
            start_attr = f' start="{number}"' if number != 1 else ""
        rendered = []
        for item in items:
            while item and not item[-1].strip():
                item.pop()
            inner = self.blocks(item)
            if not loose and inner.count("<p>") == 1 and inner.startswith("<p>"):
                inner = inner.replace("<p>", "", 1).replace("</p>", "", 1)
            rendered.append(f"<li>{inner}</li>")
        return i, f"<{tag}{start_attr}>" + "".join(rendered) + f"</{tag}>"


def split_front(text: str) -> tuple[dict[str, str], str]:
    if not text.startswith("---\n"):
        return {}, text
    end = text.find("\n---\n", 4)
    if end < 0:
        return {}, text
    meta = {}
    for line in text[4:end].splitlines():
        if ":" in line:
            key, value = line.split(":", 1)
            meta[key.strip()] = value.strip()
    return meta, text[end + 5:]


# --------------------------------------------------------------------------- #
# Site                                                                         #
# --------------------------------------------------------------------------- #


@dataclass
class Page:
    source: Path          # repository file
    url: str              # site path, e.g. "install.html" or "source/prd.html"
    title: str
    nav: str
    section: str
    summary: str = ""
    sources: list[str] = field(default_factory=list)
    canonical: bool = False
    rendered: Rendered | None = None


def load_nav() -> list[dict]:
    return json.loads((SITE / "nav.json").read_text(encoding="utf-8"))


def collect() -> tuple[list[Page], dict[Path, Page]]:
    pages: list[Page] = []
    for section in load_nav():
        for name in section["pages"]:
            path = PAGES / f"{name}.md"
            meta, _ = split_front(path.read_text(encoding="utf-8"))
            pages.append(Page(
                source=path,
                url=f"{name}.html",
                title=meta.get("title", name),
                nav=meta.get("nav", meta.get("title", name)),
                section=section["section"],
                summary=meta.get("summary", ""),
                sources=[s.strip() for s in meta.get("sources", "").split(",") if s.strip()],
            ))
    for rel, slug, title, summary in SOURCES:
        pages.append(Page(ROOT / rel, f"source/{slug}.html", title, title, "Source documents", summary, canonical=True))
    for adr in sorted((ROOT / ADR_DIR).glob("*.md")):
        first = adr.read_text(encoding="utf-8").splitlines()[0].lstrip("# ").strip()
        number = adr.name.split("-", 1)[0]
        pages.append(Page(adr, f"source/adr-{adr.stem}.html", first, f"ADR {number}", "Decisions",
                          "Accepted architecture decision", canonical=True))
    by_source = {page.source.resolve(): page for page in pages}
    return pages, by_source


def relative_url(from_url: str, to_url: str) -> str:
    depth = from_url.count("/")
    return "../" * depth + to_url


class Linker:
    """Rewrites a link written against the repository into one that works in the site."""

    def __init__(self, page: Page, by_source: dict[Path, Page], assets: dict[str, str]):
        self.page = page
        self.by_source = by_source
        self.assets = assets  # repo-relative path -> site path, filled as images are met

    def __call__(self, raw: str) -> tuple[str, bool]:
        if re.match(r"^[a-z][a-z0-9+.-]*:", raw, re.IGNORECASE) or raw.startswith("//"):
            return raw, True
        if raw.startswith("#"):
            return raw, False
        target, _, fragment = raw.partition("#")
        fragment = f"#{fragment}" if fragment else ""
        if target.startswith("/"):  # an application route such as /?files=, not a file
            return raw, False
        resolved = (self.page.source.parent / unquote(target)).resolve()
        if resolved in self.by_source:
            return relative_url(self.page.url, self.by_source[resolved].url) + fragment, False
        try:
            rel = resolved.relative_to(ROOT).as_posix()
        except ValueError:
            return raw, False
        if resolved.suffix.lower() in {".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg"} and resolved.is_file():
            site_path = "media/" + rel.replace("/", "-")
            self.assets[rel] = site_path
            return relative_url(self.page.url, site_path), False
        kind = "tree" if resolved.is_dir() else "blob"
        return f"{REPOSITORY}/{kind}/main/{rel}{fragment}", True


def render_page(page: Page, by_source: dict[Path, Page], assets: dict[str, str]) -> Rendered:
    meta, body = split_front(page.source.read_text(encoding="utf-8"))
    md = Markdown(Linker(page, by_source, assets))
    content = md.render(body)
    title = page.title
    if not page.canonical:
        title = meta.get("title", page.title)
    else:
        h1 = next((h for h in md.headings if h[0] == 1), None)
        if h1:
            title = h1[2]
    text = re.sub(r'<a class="anchor"[^>]*>#</a>|<div class="code-head">.*?</div>', " ", content)
    text = re.sub(r"</?(?:a|code|strong|em|kbd)\b[^>]*>", "", text)
    text = html.unescape(re.sub(r"<[^>]+>", " ", text))
    text = re.sub(r"\s+([.,;:)])", r"\1", re.sub(r"\s+", " ", text)).strip()
    return Rendered(content, title, md.headings, text)


def nav_html(pages: list[Page], current: Page) -> str:
    sections: dict[str, list[Page]] = {}
    for page in pages:
        sections.setdefault(page.section, []).append(page)
    out = []
    for section, members in sections.items():
        canonical = members[0].canonical
        open_attr = " open" if (not canonical or current.section == section) else ""
        rows = []
        for page in members:
            here = ' aria-current="page"' if page is current else ""
            rows.append(f'<li><a href="{relative_url(current.url, page.url)}"{here}>{html.escape(page.nav)}</a></li>')
        if canonical:
            out.append(f'<details class="nav-group"{open_attr}><summary class="label">{html.escape(section)}</summary>'
                       f'<ul>{"".join(rows)}</ul></details>')
        else:
            out.append(f'<div class="nav-group"><p class="label">{html.escape(section)}</p><ul>{"".join(rows)}</ul></div>')
    return "\n".join(out)


def toc_html(rendered: Rendered) -> str:
    items = [h for h in rendered.headings if h[0] in (2, 3)]
    if len(items) < 2:
        return ""
    rows = "".join(
        f'<li class="toc-{level}"><a href="#{anchor}">{html.escape(text)}</a></li>' for level, anchor, text in items
    )
    return f'<nav class="toc" aria-label="On this page"><p class="label">On this page</p><ul>{rows}</ul></nav>'


def toc_inline_html(rendered: Rendered) -> str:
    """The same outline, folded under the title, for screens without the aside."""
    items = [h for h in rendered.headings if h[0] == 2]
    if len(items) < 3:
        return ""
    rows = "".join(f'<li><a href="#{anchor}">{html.escape(text)}</a></li>' for _, anchor, text in items)
    return f'<details class="toc-inline"><summary>On this page</summary><ul>{rows}</ul></details>'


def pager_html(pages: list[Page], current: Page) -> str:
    guides = [p for p in pages if not p.canonical]
    if current not in guides:
        return ""
    index = guides.index(current)
    cells = []
    if index > 0:
        prev = guides[index - 1]
        cells.append(f'<a class="pager-prev" href="{relative_url(current.url, prev.url)}" rel="prev">'
                     f'<span class="label">Previous</span><span>{html.escape(prev.nav)}</span></a>')
    else:
        cells.append("<span></span>")
    if index + 1 < len(guides):
        nxt = guides[index + 1]
        cells.append(f'<a class="pager-next" href="{relative_url(current.url, nxt.url)}" rel="next">'
                     f'<span class="label">Next</span><span>{html.escape(nxt.nav)}</span></a>')
    return f'<nav class="pager" aria-label="Previous and next">{"".join(cells)}</nav>'


def sources_html(page: Page, by_source: dict[Path, Page]) -> str:
    if page.canonical:
        rel = page.source.relative_to(ROOT).as_posix()
        return (f'<p class="provenance">Rendered from <a href="{BLOB}{rel}" rel="noopener"><code>{rel}</code></a>, '
                f"a canonical document. Where a guide page and this page disagree, this page wins.</p>")
    if not page.sources:
        return ""
    links = []
    for rel in page.sources:
        target = by_source.get((ROOT / rel).resolve())
        href = relative_url(page.url, target.url) if target else f"{BLOB}{rel}"
        links.append(f'<a href="{href}"><code>{html.escape(rel)}</code></a>')
    return f'<p class="provenance">Canonical sources: {", ".join(links)}. Where this page and a source disagree, the source wins.</p>'


def render_site(out: Path) -> list[str]:
    pages, by_source = collect()
    assets: dict[str, str] = {}
    template = (SITE / "template.html").read_text(encoding="utf-8")
    version = (ROOT / "VERSION").read_text(encoding="utf-8").strip()
    for page in pages:
        page.rendered = render_page(page, by_source, assets)
        if page.canonical:
            page.title = page.rendered.title

    (out / "source").mkdir(parents=True)
    (out / MARKER).write_text("Built by scripts/build-docs-site.py; safe for it to replace.\n", encoding="utf-8")
    (out / "assets" / "fonts").mkdir(parents=True)
    (out / "media").mkdir()
    for item in STATIC.iterdir():
        shutil.copy2(item, out / "assets" / item.name)
    shutil.copy2(ROOT / "dashboard" / "public" / "favicon.png", out / "assets" / "favicon.png")
    for font in FONTS:
        shutil.copy2(ROOT / "dashboard" / "public" / "fonts" / font, out / "assets" / "fonts" / font)
    for rel, site_path in assets.items():
        shutil.copy2(ROOT / rel, out / site_path)

    index = []
    for page in pages:
        rendered = page.rendered
        root = relative_url(page.url, "")
        body_class = "canonical" if page.canonical else ("home" if page.url == "index.html" else "guide")
        lede = ""
        if page.summary and not page.canonical and page.url != "index.html":
            lede = f'<p class="lede">{html.escape(page.summary)}</p>'
        content = rendered.html
        if lede:
            content = re.sub(r"(</h1>)", r"\1" + lede.replace("\\", "\\\\"), content, count=1)
        inline_toc = toc_inline_html(rendered) if page.url != "index.html" else ""
        if inline_toc:
            # After the lede when there is one, else straight after the title.
            anchor = r'(<p class="lede">.*?</p>)' if lede else r"(</h1>)"
            content = re.sub(anchor, lambda m: m.group(1) + inline_toc, content, count=1, flags=re.S)
        replacements = {
            "{{title}}": html.escape(rendered.title if page.url != "index.html" else "CHROTE documentation"),
            "{{description}}": html.escape(page.summary or "CHROTE documentation", quote=True),
            "{{root}}": root,
            "{{version}}": html.escape(version),
            "{{nav}}": nav_html(pages, page),
            "{{toc}}": toc_html(rendered) if page.url != "index.html" else "",
            "{{content}}": content,
            "{{provenance}}": sources_html(page, by_source),
            "{{pager}}": pager_html(pages, page),
            "{{section}}": html.escape(page.section),
            "{{body_class}}": body_class,
            "{{repository}}": REPOSITORY,
        }
        document = template
        for key, value in replacements.items():
            document = document.replace(key, value)
        (out / page.url).write_text(document, encoding="utf-8")
        sections = [{"id": anchor, "text": text} for level, anchor, text in rendered.headings if level in (2, 3)]
        index.append({
            "url": page.url,
            "title": rendered.title,
            "section": page.section,
            "summary": page.summary,
            "headings": sections,
            "text": re.sub(r"\s+", " ", rendered.text)[:6000],
        })
    (out / "assets" / "search-index.js").write_text(
        "window.CHROTE_SEARCH=" + json.dumps(index, ensure_ascii=False, separators=(",", ":")) + ";\n",
        encoding="utf-8",
    )
    return [page.url for page in pages]


def check_links(out: Path) -> list[str]:
    """Every relative link resolves to a file, and every fragment to an id in it."""
    errors = []
    ids: dict[Path, set[str]] = {}

    def anchors(path: Path) -> set[str]:
        if path not in ids:
            ids[path] = set(re.findall(r'\bid="([^"]+)"', path.read_text(encoding="utf-8")))
        return ids[path]

    for path in out.rglob("*.html"):
        text = path.read_text(encoding="utf-8")
        for attr, raw in re.findall(r'\b(href|src)="([^"]+)"', text):
            if re.match(r"^[a-z][a-z0-9+.-]*:", raw, re.IGNORECASE) or raw.startswith("/"):
                continue
            target, _, fragment = html.unescape(raw).partition("#")
            resolved = (path.parent / target).resolve() if target else path
            where = path.relative_to(out).as_posix()
            if not resolved.exists():
                errors.append(f"{where}: missing {raw}")
            elif fragment and resolved.suffix == ".html" and fragment not in anchors(resolved):
                errors.append(f"{where}: no anchor {raw}")
    return errors


def serve(out: Path, port: int) -> None:
    handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(out))
    with http.server.ThreadingHTTPServer(("127.0.0.1", port), handler) as server:
        print(f"Serving {out.relative_to(ROOT) if out.is_relative_to(ROOT) else out} at http://127.0.0.1:{port}/ (Ctrl+C stops)")
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT, help="output directory (default: docs/site/_site)")
    parser.add_argument("--serve", action="store_true", help="serve the built site on 127.0.0.1 after building")
    parser.add_argument("--port", type=int, default=8000, help="port for --serve (default: 8000)")
    parser.add_argument("--check", action="store_true", help="build to a temporary directory and check links")
    args = parser.parse_args()

    if args.check:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "site"
            built = render_site(out)
            errors = check_links(out)
        if errors:
            print("docs-site: FAIL")
            for error in errors:
                print(f"- {error}")
            return 1
        print(f"docs-site: PASS ({len(built)} pages, internal links and anchors)")
        return 0

    out = args.out.resolve()
    refusal = replaceable(out)
    if refusal:
        print(f"docs-site: refusing --out {out}: {refusal}", file=sys.stderr)
        return 2
    # Build beside the target and swap only a build that passed, so a failure
    # leaves the previous site exactly as it was.
    out.parent.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix=f".{out.name}.", dir=out.parent))
    try:
        built = render_site(staging / "site")
        errors = check_links(staging / "site")
        if errors:
            print("docs-site: FAIL; the previous build, if any, is unchanged")
            for error in errors:
                print(f"- {error}")
            return 1
        if out.exists():
            out.rename(staging / "previous")
        (staging / "site").rename(out)
    finally:
        shutil.rmtree(staging, ignore_errors=True)
    print(f"Built {len(built)} pages into {out}")
    if args.serve:
        serve(out, args.port)
    return 0


def replaceable(out: Path) -> str:
    """Why OUT must not be replaced, or "" when it may: absent, empty, or an earlier build."""
    if not out.exists():
        return ""
    if not out.is_dir():
        return "it exists and is not a directory"
    if (out / MARKER).is_file() or not any(out.iterdir()):
        return ""
    return f"it is a non-empty directory this builder did not create (no {MARKER})"


if __name__ == "__main__":
    raise SystemExit(main())
