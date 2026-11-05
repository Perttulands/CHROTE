# CHROTE documentation site

A static, browsable documentation site for CHROTE: guide pages for installing,
using, configuring, operating and trusting it, beside the canonical documents
rendered as they are. It is built from this repository with the Python standard
library and needs no other dependency. Nothing here deploys or hosts it.

## Build and preview

From the repository root:

```bash
python3 scripts/build-docs-site.py --serve
```

That builds into `docs/site/_site/` and serves it on <http://127.0.0.1:8000>
until `Ctrl+C`. `--port` picks another port. Without `--serve`, the build only
writes the directory; its pages use relative links, so `docs/site/_site/index.html`
also opens straight from disk.

```bash
python3 scripts/build-docs-site.py --check
```

builds into a temporary directory and fails if any internal link, image or
anchor does not resolve. CI runs it in the `docs` job. `docs/site/_site/` is
ignored by git.

`--out PATH` builds elsewhere. Every build happens in a staging directory beside
the target and replaces it only after its link check passes, so a failed build
leaves the previous site in place. The builder replaces only a directory that is
absent, empty, or marked `.chrote-docs-site` by an earlier build; it refuses
anything else, such as `--out docs`.

## Layout

| Path | Holds |
| --- | --- |
| [`pages/`](pages/index.md) | Guide pages in Markdown, each opening with `title`, `nav`, `summary` and `sources` |
| [`nav.json`](nav.json) | Sections and page order |
| [`template.html`](template.html) | The page frame |
| [`static/`](static/site.css) | Styles and the script for the menu, search, copy buttons and outline |
| [`../../scripts/build-docs-site.py`](../../scripts/build-docs-site.py) | The builder; its `SOURCES` list names the canonical documents it renders |

The fonts are copied from `dashboard/public/fonts` and the favicon from
`dashboard/public`, so the site wears the product's own face.

## Writing pages

- Guide pages explain and link; the canonical documents decide. When a guide and
  a canonical document disagree, the canonical document wins, and the fix
  belongs wherever the wrong statement is. The `sources` line names the
  documents a page answers to, and the site prints them at its foot.
- Link to other pages and to repository files by their relative repository
  path, as GitHub would, such as `install.md` or `../../../SECURITY.md`. The
  builder turns links to rendered documents into site links and others into
  GitHub links.
- Supported Markdown: headings, paragraphs, lists, tables, fenced code,
  blockquotes, GitHub alerts (`> [!NOTE]`, `> [!WARNING]` …), links, images and
  raw HTML blocks. Code spans that read as a chord, such as `Alt+S`, are drawn
  as keys.
- The look follows [`DESIGN-SYSTEM.md`](../../DESIGN-SYSTEM.md): the
  `chrote-violet` palette, one typeface, and the accent only where it marks
  where you are, where input goes, or the one action.
- Keep pages host-neutral, as every public document must be.
