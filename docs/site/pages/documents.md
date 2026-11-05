---
title: Source documents
nav: Source documents
summary: The canonical documents this site is built on, rendered as they are in the repository.
sources: docs/source-truth-index.md
---

# Source documents

The guide pages explain; these documents decide. When CHROTE documents
disagree, [the source-truth index](../../source-truth-index.md) says which one
wins, and a canonical document always outranks a guide page.

## Product and design

<ul class="rows">
<li><a href="../../../VISION.md"><span class="row-title">Vision</span><span class="row-text">Why CHROTE exists and what should remain true.</span></a></li>
<li><a href="../../../PRD.md"><span class="row-title">Product requirements</span><span class="row-text">The durable product contract, state ownership and non-goals.</span></a></li>
<li><a href="../../journeys.md"><span class="row-title">Operator journeys</span><span class="row-text">The seven journeys every surface and control answers to.</span></a></li>
<li><a href="../../../DESIGN-SYSTEM.md"><span class="row-title">Design system</span><span class="row-text">Principles, doctrine, the colour rule, the accent, the keyboard and the theme contract.</span></a></li>
</ul>

## Runtime and trust

<ul class="rows">
<li><a href="../../../ARCHITECTURE.md"><span class="row-title">Architecture</span><span class="row-text">Runtime shape, sources of truth, the terminal path and failure behaviour.</span></a></li>
<li><a href="../../../COMPONENTS.md"><span class="row-title">Components</span><span class="row-text">The runtime component and optional-integration map.</span></a></li>
<li><a href="../../../SECURITY.md"><span class="row-title">Security policy</span><span class="row-text">The trust model, browser origins, terminal and filesystem boundaries, reporting.</span></a></li>
</ul>

## Installation and operation

<ul class="rows">
<li><a href="../../installation.md"><span class="row-title">Installation reference</span><span class="row-text">Install, environment, agent events, the Library, residents, upgrade and uninstall.</span></a></li>
<li><a href="../../troubleshooting.md"><span class="row-title">Troubleshooting reference</span><span class="row-text">Boundary-by-boundary diagnosis of a supported install.</span></a></li>
<li><a href="../../scheduled-tasks.md"><span class="row-title">Scheduled tasks API</span><span class="row-text">Task schema, every request, and the delivery safety contract.</span></a></li>
<li><a href="../../private-beads-sidecar.md"><span class="row-title">Private Beads sidecar</span><span class="row-text">The host-neutral contract for private Beads transport.</span></a></li>
</ul>

## Contributing

<ul class="rows">
<li><a href="../../../CONTRIBUTING.md"><span class="row-title">Contributing</span><span class="row-text">Setup, the gates CI runs, documentation rules and pull requests.</span></a></li>
<li><a href="../../TEST_STRATEGY.md"><span class="row-title">Test strategy</span><span class="row-text">Which layer owns which behaviour, and CI routing.</span></a></li>
<li><a href="../../../dashboard/README.md"><span class="row-title">Dashboard README</span><span class="row-text">The dashboard's map and local commands.</span></a></li>
<li><a href="../../../CHANGELOG.md"><span class="row-title">Changelog</span><span class="row-text">What changed, release by release.</span></a></li>
</ul>

## Decisions

<ul class="rows">
<li><a href="../../adr/0013-ttyd-restart-lifecycle-and-orphan-reaping.md"><span class="row-title">ADR 0013</span><span class="row-text">Browser terminal attaches do not survive a server restart.</span></a></li>
<li><a href="../../adr/0015-access-first-non-interference.md"><span class="row-title">ADR 0015</span><span class="row-text">CHROTE preserves access but does not own workload durability.</span></a></li>
<li><a href="../../adr/0016-core-boundary-and-formations-extraction.md"><span class="row-title">ADR 0016</span><span class="row-text">Keep the core small and extract agent formations.</span></a></li>
<li><a href="../../adr/0017-terminal-viewing-model.md"><span class="row-title">ADR 0017</span><span class="row-text">The terminal viewing model: sizing, bindings and Peek.</span></a></li>
<li><a href="../../adr/0018-terminal-transport-ownership.md"><span class="row-title">ADR 0018</span><span class="row-text">CHROTE owns the terminal transport.</span></a></li>
</ul>

The [README](../../../README.md) is the project's landing page on GitHub.
