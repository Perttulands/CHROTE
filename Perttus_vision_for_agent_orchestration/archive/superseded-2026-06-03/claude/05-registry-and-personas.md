# 05 · Agent Registry, Personas & Persistent Identity

The org's "who." A persona is a durable identity; a session is a running instance. The registry lets
agents discover collaborators with progressive disclosure.

> **Decisions applied (Perttu — see [08](08-open-questions.md)):** discovery/launch verbs are
> **`archon agent list | spawn <harness> --name <n> | attach <n> | inspect <n>`**; `archon agent spawn`
> is the launch primitive the "launching a persona" section describes.

---

## 1. Don't reinvent identity — wrap what exists

Perttu already has persistent named agents: **Hermes profiles** in `~/.hermes/profiles/<name>/`
(verified: `archon`, `athena`, `chiron`, `hestia`, `hierophant`, `plutus`), each with a `SOUL.md`
(self-evolving identity/voice) and config (model, provider, toolsets). **There is already a profile
named `archon`.** A second competing persona system would violate simplicity and "surface conflicts,
don't blend."

So: **a persona = a small CHROTE-owned TOML "card" that _points at_ the real definition** (a Hermes
profile, a `CLAUDE.md`, a skill, or a raw launch command). The card holds only what discovery and
reference need; the heavy stuff stays in its native home and is read only on deep disclosure.

---

## 2. Persona card schema

TOML, one file per persona, at **`~/agents/<id>.toml`** (central — identities span projects; the
architect must hold the golden thread across weeks). The card is split so listing the roster parses
only the cheap `[card]` stanza.

```toml
[card]                          # cheap; always loaded for roster + tag search
id           = "susie"          # THE id: == tmux session stem == slot.agentId == team ref == ledger key
display_name = "Susie"
role         = "Designer"
summary      = "UI & web-experience designer. Holds the visual voice."
tags         = ["design","ui","web","taste:visual"]   # namespaced; capability/focus/personality
persistent   = true             # default true (a card exists ⇒ persistent)
status       = "active"         # active | dormant | retired (intent, NOT live liveness)

[harness]                       # how to instantiate (the run engine consumes this)
kind         = "hermes"         # hermes | claude-code | codex | pi | opencode | tmux
profile      = "susie"          # for hermes: ~/.hermes/profiles/<profile>
launch       = "hermes-run --profile susie --session susie"
session_stem = "susie"          # tmux session name Oracle binds to (default = id)

[identity]                      # deep; pointers, not inlined prose
soul   = "~/.hermes/profiles/susie/SOUL.md"
skills = ["humanizer","prototype-it"]
voice  = "Warm, opinionated about whitespace and hierarchy. Hates hedging."

[standards]                     # only for taste/voice/standards/architecture holders
holds       = "visual-voice"
review_lens = "Legible at a glance? Hierarchy matches importance? On-brand?"

[relationships]
reports_to   = "archon"
member_of    = ["design-team"]

[eval]
ledger_key   = "susie"          # filter interactions.jsonl by this; default = id
```

**The Archon** (`~/agents/archon.toml`): `kind=hermes`, `profile=archon` (the existing one — do not
duplicate its SOUL), `oversees=["*"]` by convention (the one persona that enumerates/assembles the
whole org). **Senior architect** (e.g. `atlas`): `kind=claude-code`, `agent_md=/workspace/chrote/CLAUDE.md`,
`[standards].holds="chrote-architecture"` — proves the schema is harness-agnostic. **A disposable
helper has no card at all** — it's just a `claude-pico`/`codex-1` tmux session Oracle reports.

---

## 3. The progressive-disclosure registry

**The registry _is_ the directory `~/agents/`** — derived, not a separate index (nothing to keep in
sync, because the files are the registry). Layers map to "read more of the card" + the `archon agent` verbs:

| Layer | Exposes | Source |
|---|---|---|
| 0 Liveness | live sessions, status, contextPct, beadId | Oracle (`/api/oracle/agents`) — exists |
| 1 Roster | id, display_name, role, status | `[card]` |
| 2 Tags | capability/personality/focus tags + summary | `[card].tags` |
| 3 Org | relationships, team membership | `[relationships]`, team files |
| 4 Deep | harness binding, model, prompt/soul/skill pointers, standards | rest of card + follow pointers |
| 5 Track record | per-persona history/outputs | ledger (`interactions.jsonl`), design-only |
| 6 Gap analysis | "what's missing / what should we build?" | computed over layers 1–3 |

**Minimal registry (stage 1):** one Go function `LoadPersonas(dir)` that globs `~/agents/*.toml` and
parses `[card]`+`[harness]`. That's it.

---

## 4. Persona ↔ live session binding (a naming convention)

Deliberately boring: `[harness].session_stem` (default = `id`) **is** the tmux session name (or
prefix). Binding is computed at read time: `oracleSession.Name == session_stem` (or `HasPrefix`).
**Nothing is written to establish a binding — running the right session name _is_ the binding.** It
survives crashes (next launch with the same name re-binds).

- **Dormant** ("registered, not running") is the default and normal: layer-1 roster shows every
  persona; the layer-0 join finds no session → `live:false`. Two axes kept distinct: `[card].status`
  (intent) vs the join's live/dormant (fact).
- **Launching** a persona = run `[harness].launch` on the CHROTE tmux socket with session name
  `session_stem`. The card *declares* the launch method; the run engine *executes* it (the seam to
  [doc 04](04-run-engine-and-adapters.md)). Respects the golden rule (named, deliberate sessions;
  never touches existing shells).
- An optional lease file (`~/agents/.sessions/<id>.json`) disambiguates the rare same-persona-two-
  sessions case. **Not in stage 1.**

---

## 5. Relationship to the existing Oracle

**A thin new layer beside Oracle, not inside it.** A new `RegistryHandler` (`/api/agents`) loads
persona cards and **left-joins** them onto Oracle's existing `getAgentSessions()`/`enrichAgent()` by
session-stem. Why beside:

- Oracle must stay orchestrator-neutral and "absence of agents is normal" (PRD). Teaching it personas
  would make it opinionated about an orchestration model it's meant to be neutral about.
- The two have different liveness: Oracle = only live sessions (correctly empty when nothing runs);
  the registry = mostly-dormant durable identities. Forcing dormant personas into Oracle would break
  its "live sessions" contract and its SSE diff logic.

The Agents view (and `archon agent list`) then shows persistent personas (running/dormant) **and** the raw
disposable sessions Oracle sees. Oracle's `/api/oracle/*` endpoints stay byte-for-byte unchanged. One
small shared helper (`isAgentSession`/`agentPrefixes`) may move to `core`.

---

## 6. Persistent vs disposable

**The distinguishing bit: a card exists ⇒ persistent; a live session with no card ⇒ disposable.** No
flag to forget, no DB column. "Make this disposable agent permanent" becomes a concrete, reviewable
act: write a card.

A persona earns a card if it holds ≥1 of: **taste/voice** (Susie), **standards/domain stewardship**
(integrity-maintainer over skill files), **architectural/project continuity** (the architect's golden
thread), **relationship/touchpoint** (Archon, team leads), or **a characteristic way of thinking worth
reusing**. Purely mechanical execution ("implement this function") → no card. This makes the vision's
"integrity maintainer ≠ generic cron job" real: the maintainer has a card (`holds`, `review_lens`), so
it appears in the org, can be evaluated, and reasons about its domain; a cron job has none of that.

---

## 7. Teams

Teams are files too — `~/agents/teams/<id>.toml` — a leader + members + a charter, referencing
personas **by id**:

```toml
[team]
id      = "design-team"
name    = "Design Team"
leader  = "susie"                 # the single touchpoint (vision §3/§7)
members = ["pixel","atlas"]       # persona ids; same persona may sit in many teams
charter = "Produce 2-3 directions before converging. Escalate on scope > 1 week or brand conflict."
beads_workspace = "/workspace/chrote"   # where this team tracks work
```

- **Preconfigured team** = such a file (durable, reusable — "talk to a leader who already has a group").
- **Dynamically-assembled team** = the same format, written at assembly time by `archon team assemble`.
  Recommendation: ephemeral by default (held in run state), `archon team save` promotes it to a durable
  file. Keeps `~/agents/teams/` curated, not littered with one-shots ([08 Q]).

A team is the durable org-chart; a formation is a runtime wiring. Both reference personas by the same
`id` — the universal join key.

---

## 8. The Archon

The Archon is just the root persona (`~/agents/archon.toml`), bound to the existing Hermes `archon`
profile — so "talk to the Archon" is *already real* outside CHROTE (it has a `channel_directory.json`,
a voice presence). Inside CHROTE the convention is a pinned session named `archon` the Agents view
surfaces first and can launch on demand. It has no privileged API — it calls the same `archon` everyone
does (`archon agent list`, `archon team assemble`). What makes it special is its role/SOUL (framing, assembly,
escalation, gap-reasoning), not a special mechanism. → [08 Q: auto-launch vs show-and-launch].

---

## 9. Evaluation seam (design-only)

**Reuse `bd audit` → `.beads/interactions.jsonl`** (verified: append-only, git-versioned, built for
"why did the agent do that?" + SFT/RL datasets). The registry contributes one thing: a **stable key**.
Reserve a `persona` field on each interaction; the run-ledger writer stamps `persona=<id>`. Then "how
is Susie doing?" = filter the ledger by `persona==susie`; improve by editing her SOUL / swapping her
`[harness].model` / adding a skill / trying a different `kind`. The run-ledger owns the writer; the
registry reserves the field. **Build no dashboards/scorecards now** (vision §15).

---

## 10. Gap analysis ("what should we build that doesn't exist?")

Emergent and free from tagged cards + the CLI: union all `tags` + `[standards].holds` → the org's
current coverage; a needed capability with no holder = a gap; the same disposable session spun up
repeatedly (visible in the ledger) = a candidate for promotion to a persona. Stage-1 form: the Archon
reasons in natural language over `archon agent list --tags`. The only schema guarantee needed: tags and `holds`
are present and namespaced (`taste:visual`, `domain:architecture`) so coverage is queryable.

---

## 11. Minimal first slice

1. Define `~/agents/` + the `[card]`/`[harness]` schema. Hand-author **three real cards**: `archon`
   (→ existing Hermes profile), one `design-lead`, one specialist.
2. `LoadPersonas(dir)` (glob + parse `[card]`+`[harness]`) — this *is* the registry.
3. `GET /api/agents`: load personas, call Oracle's `getAgentSessions()`, **left-join by session_stem**,
   return cards + `live:running|dormant` + the unmatched ephemeral sessions. Touch `oracle.go` only to
   export the prefix helper.
4. Bind by convention, dormant by default. Launch a session named `susie` → it lights up; kill it →
   dormant. No lease file.
5. Reserve the `persona` field in the ledger (coordinate with [doc 04](04-run-engine-and-adapters.md)).
   No eval UI.

**The whole bet:** one string (`id`) unifies card ⇄ session ⇄ slot ⇄ team ⇄ ledger; the registry is a
directory of small TOML cards left-joined onto the Oracle CHROTE already has; persistence is "a card
exists"; everything richer is more stanzas read only when disclosed.
