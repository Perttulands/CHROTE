---
title: Agents
nav: Agents
summary: Read exactly what a harness loads before it starts work in a folder: its instruction stack, its skills and its memories.
sources: PRD.md, docs/journeys.md
---

# Agents

> "Understanding what the agents load with and what they see is perhaps the most
> critical thing of all."

Before you trust or correct an agent, know what it read. The Agents tab answers
one question for any folder on the host: **what does an agent started here,
under this harness, as this user, already have in its head before you type a
word?**

## What it shows

Three layers, each row naming where it came from:

1. **The instruction stack**, in the order the harness loads it: managed
   policy, your user-level files, files in the folders above, the project's
   own files and rules, and the harness's settings.
2. **The skills** the agent can reach.
3. **The memories** written for that folder.

CHROTE resolves this for **Claude Code** and **Codex**. It reads as its own Unix
account and says so: a file it cannot read is listed as unreadable rather than
dropped, because an instruction you cannot see is still an instruction the agent
loaded. Nothing is cached; the tab asks when you look.

## Ways in

- **The Agents tab** (`Alt+G`): pick any workspace and harness from the rail.
- **What this agent sees**, from a tile's or a session row's menu: the same view
  for a running session, on the table.

## Fixing what you find

Open a layer in the editor shared with Files and correct the wording yourself,
or hand the curation to the **tender**, the resident agent of this tab, behind
the spine at its right edge. `Alt+S` pastes the chosen workspace and harness
into the tender's prompt; `Alt+Enter` focuses its drawer. Send to Session
carries the same reference to any other session.

The tender's session, folder and Beads project are configured with
`CHROTE_TENDER_SESSION`, `CHROTE_TENDER_FOLDER` and `CHROTE_TENDER_BEADS`; see
[Libraries and residents](integrations.md#residents).
