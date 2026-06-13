# 06 · Shared State — Beads, Notice Board & Escalation

Where each kind of state lives, how teams stay aware without Perttu choreographing it, and how
meaningful interrupts actually reach him.

> **DEFERRED (Perttu): the notice board is "a very different system" — out of the near-term plan.**
> Stage-1 communication is the brief (inline) + the run ledger + conversational status via the Archon;
> escalation stays minimal. This doc is kept as the design for when the board *is* built — including
> why it must be file-native NDJSON, not messages-as-beads (`bd mail` delegates to Gastown, forbidden
> here). The Beads-vs-board boundary and the "missions are bead-backed" integration ([03 §4](03-cli-surface.md))
> still hold.

---

## 1. The boundary (resolving the vision's open question)

**Beads = work (commitments). Notice board = communication (awareness). Run ledger = run mechanics.
They do not overlap.** A board entry is a *speech act* ("I'm asking X", "heads up", "I'm blocked")
whose lifetime is the mission; a bead is a *commitment* with a state machine. Posting a question
creates awareness, not work — if it *becomes* work, that spawns a bead.

| Thing | Lives in | Why |
|---|---|---|
| A unit of work | **Beads** | bd has the state machine (open→in_progress→closed), deps, priority. |
| A blocker | **Beads** (`blocked` + dep) **+ a board post** | The *fact* is work state (bd); the *announcement* "who can unblock me?" is awareness (board, linking the bead). Two facets, not duplication. |
| A question to another agent | **Notice board** | Communication, not a commitment. Questions in bd would pollute `bd ready`. |
| A team status update | **Notice board** (`status`) | Ephemeral awareness; lets the Archon synthesize without choreography. |
| A design decision | **Beads** (`--type=decision`) — the record; board announces it | bd has native `decision`; decisions are durable and belong with the work. The deliberation is board + transcript. |
| A cross-team notice | **Notice board** (global, `notice`) | Pure cross-boundary awareness. |
| An escalation to Perttu | **Notice board** (`escalation`, source of truth) → bd event/wisp → TTS | §3. |
| An agent's reasoning/output | **Run ledger** (report/diffs/artifacts/verdict); raw thinking = transcript | The prototype's output object *is* the ledger; owned by the run engine, not bd, not the board. |
| Ephemeral chatter ("on it", "lgtm") | **Transcript-only** | No recipient, no decision, no work → no durable home. Don't let the board fill with acks. |

Two rules fall out: **the board never holds a commitment; bd never holds a conversation.** And posts
*reference* beads/ledger outputs by id (`beadRef`, `outputRef`) but are never their source of truth.

---

## 2. Notice board — file-native NDJSON (not messages-as-beads)

**This overrides the prior "beadmail is the strongest candidate" framing, for a concrete reason:**
`bd mail` in this install **delegates to an external provider** (`BEADS_MAIL_DELEGATE` /
`BD_MAIL_DELEGATE`, i.e. Gastown's `gt`) — verified. CHROTE's `CLAUDE.md` forbids assuming Gastown is
installed. So messages-as-beads means either a forbidden dependency or cramming messages into
`--type=event`/custom types — the "second tracker" anti-pattern. We keep beadmail's *semantics*
(sender/recipient/subject/body/thread/read/priority) and drop its *substrate*.

A board is an **append-only NDJSON file** — `cat`-able, dependency-free, durable, conflict-free under
concurrent line-appends. Addressing: **one board per mission** (`<mission>/board.ndjson`, the common
case — naturally scopes awareness, archives with the mission) **plus one global board**
(`.formations/board.ndjson`) for cross-mission `notice`s and all `escalation`s. No per-team boards (a
mission already scopes the audience).

```jsonc
{ "id":"post-7f3a","ts":"…","mission":"improve-session-search",
  "from":"design-lead","to":["arch-lead"],          // ["*"] = broadcast within the mission
  "kind":"question",                                 // question|answer|status|notice|decision|escalation
  "subject":"Caching strategy for the search index?","body":"…",
  "priority":2,                                       // 0=urgent(escalation) … 3=fyi
  "thread":"post-7f3a","replyTo":null,                // reply sets thread=root, replyTo=parent
  "beadRef":"chrote-42","outputRef":null,             // optional links out
  "resolved":false }
```

**Never mutate a line.** "Read"/"resolved" is an appended update post; a reader folds the log per
`thread` to compute current state. This keeps an immutable audit trail, `tail`-able and append-safe.
Semantics (owned here; verb names owned by [doc 03](03-cli-surface.md)): **post** = append;
**read** = fold + filter (`to==me || "*"`, `resolved`, `since`); **reply** = append with thread/replyTo
(also marks seen); **escalate** = post `escalation`/`priority:0` to the global board (triggers §3).

---

## 3. Escalation channel to Perttu

An escalation must be a communication, be durable/queryable, and actually reach Perttu — so it's the
one deliberate fan-out. The run engine (or a lead) *emits* it ([doc 04 §5](04-run-engine-and-adapters.md));
this is where it lands:

```
escalate  →  1. SOURCE OF TRUTH: append {kind:escalation, priority:0} to the GLOBAL board.ndjson
             2. DURABLE/QUERYABLE: bd create --type=event --event-category=escalation
                                   --wisp-type=escalation  (TTL-compacted; `bd list --wisp-type=escalation`)
             3. INTERRUPT: POST http://127.0.0.1:3100/v1/tts/enqueue {text:"<subject>. <one line>"}
```

Rationale: the board post means the *same* read that gives conversational visibility (§4) surfaces
escalations; the bd wisp gives "what has the team escalated lately?" without polluting `bd ready`
(this is the one place bd is used for an escalation — as an *event*, not a work item or a message); the
TTS gateway (exists at `:3100`, localhost, async, web player) is the minimum real interrupt — no
Discord bot needed for slice 1. **No gate**: the lead's judgment decides what's escalation-worthy; the
channel just carries it loudly.

**Round-trip:** Perttu hears the TTS line / sees the CHROTE "Needs you" strip, and responds by
**talking to the Archon** (conversational — he doesn't operate a console). The Archon records the
resolution by appending an `answer`/`status` post (`resolved:true`) and, if it implies work, creating
a bead (the wisp can be `promote`d to a permanent bead). CHROTE never becomes the router.

→ [08 Q]: TTS-only for slice 1, or add a Discord/push mirror for AFK escalations (recommend TTS-only).

---

## 4. Shared context during a mission (no choreography)

The scene "design lead + arch lead remain active and proactively review; developers surface questions
and get input" needs four surfaces with assigned roles:

| Surface | Role | Who leans on it |
|---|---|---|
| **Beads** | The shared plan of record (work items, blockers, decisions). | Everyone reads `bd ready`; leads sequence. |
| **Notice board (mission)** | The shared conversation: devs post questions → leads answer; leads post status/reviews. | Devs ask; leads answer + proactively post. |
| **Run ledger** | The shared evidence: the actual draft/diff/PR/report + verdict. | Leads review; engine writes. |
| **Context Citadel** (`:3200`) | The long-term lens: standards, preferences, the architect's golden thread. | Persistent agents read; rarely written mid-mission. |

**The mechanism that removes choreography:** every agent's loop ends with a cheap **read of the
mission board since-last-seen** (+ `bd ready`). A dev posts `question, to:[arch-lead]`; the arch-lead,
reading the board, replies — **no socket-hunting by Perttu.** Boundary with the ledger: *if a human or
agent typed it as a message, it's the board; if a run produced it, it's the ledger.* A review comment
is a board post referencing a ledger `outputRef`.

---

## 5. Conversational visibility ("ask the agents what's happening")

The strict boundary exists so any agent (esp. the Archon) can answer "what's going on?" from **three
cheap reads**, no dashboard, no precomputed index:

1. `bd list --json` / `bd ready` → **work state** (open/in-progress/blocked, who's assigned).
2. fold `<mission>/board.ndjson` → **live conversation** (open questions, latest status, escalations).
3. the run ledger → **evidence** (what executed, verdicts, latest outputs).

The Archon synthesizes on demand: *"3 beads in progress, 1 blocked on the caching decision; design-lead
asked arch-lead about index persistence 20 min ago, unanswered; last run produced a draft, verdict
needs-review; no open escalations; nothing needs you right now."* The three sources are deliberately
disjoint (work / talk / evidence) so synthesis is a join, not a dedupe. This is exactly "explainable on
request, not observable by default."

---

## 6. What CHROTE shows (read-mostly)

CHROTE is observer, not operator. Add (small, read-only): `GET /api/board?mission=…` (folds the NDJSON
into threads — like `beads.go`'s read pattern), `GET /api/escalations` (`bd list --wisp-type=escalation`
+ unresolved board escalations → a small "Needs you" strip), and extend the existing Oracle poller to
tail the global board for new `escalation` lines → emit an SSE event + fire the TTS enqueue (the one
place CHROTE *acts* — a notification relay, not orchestration). **No POST endpoint** — posting is
CLI/agent-only, which is what keeps CHROTE from becoming a chat control plane. A read-only board pane
sits beside the existing Agents/Beads views, linked by `beadRef`. Per vision §15, CHROTE *can* show the
board but doesn't *have* to for the system to work — agents answer by reading files; the surface is a
convenience, secondary to "ask the Archon."

---

## 7. Minimal first slice

The board may be **deferred to Phase 1.5** (a linear Archon→lead→specialist handoff can pass the brief
inline and report via the ledger — see [08 Q4](08-open-questions.md)). If built in slice 1:

1. Board file convention + fold-by-thread reader (~30 lines, no service).
2. Two agent verbs (`board post|read|reply`, `escalate`) — pure file append/read.
3. Escalation fan-out: append-to-global-board + `bd create --type=event --wisp-type=escalation` +
   `curl :3100/v1/tts/enqueue` — one shell function; all targets already exist.
4. CHROTE read-only: `GET /api/board`, `GET /api/escalations`; Oracle poller relays escalations to
   SSE+TTS. **No POST.**
5. Document the Archon's 3-read status synthesis (§5).

**Success:** a dev posts a question; the arch-lead (different harness/session) reads it on its next
loop and replies — *zero socket-hunting by Perttu*; a lead escalates and Perttu hears it via TTS + sees
it in the "Needs you" strip; the Archon, asked "where are we?", produces the §5 status. All without
Perttu choreographing anything.
