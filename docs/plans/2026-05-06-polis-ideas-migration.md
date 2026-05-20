# Bringing Polis Ideas to CHROTE — v2 (Jobs-First)

**Date:** 2026-05-06
**Mode:** prescriptive
**Supersedes:** the v1 of this file (which catalogued polis's solutions and tried to phase them in). v1 was the wrong altitude. This is the rewrite.

---

## 0. The framing correction

The first version of this plan went shopping in polis's toolbox: relay, senate, squire, argus, learning-loop, memory hierarchy, mythology — phase them all in over six weeks. That is exactly the move that killed polis.

Polis died of unnecessary complexity. The exodus is the evidence. So the question isn't "which polis tools should chrote import?" It's: **what jobs does the operator actually have, and what is the smallest thing that closes each gap?** Most of polis's machinery was an answer to a job polis invented for itself. Chrote should not inherit those jobs.

---

## 1. The disease, named

The pattern that killed polis (visible in `master-plan-v2.md`'s problem register):

1. Every job got its own tool. (work, gate, relay, senate, argus, oathkeeper, learning-loop, truthsayer, ubs, polis-command, polis-monitor, polis-status, br, bv …)
2. Every tool got its own contract. JSON shapes, CLI surfaces, configs.
3. Contracts drifted. Master-plan-v2's *first goal* was "make 'pass' mean pass" — the system was lying to itself.
4. Drift required new tools to fix. (parser contract tests, dual-write parity checkers, restore drills, smoke workflows.)
5. The new tools also drifted.

The tell: when a multi-agent OS needs a master plan to fix its master plan, the cause is rarely a missing feature. It's that *adding more was the default response to every problem*.

**Chrote's defense:** treat every new endpoint, every new daemon, every new file convention as expensive. Default to no.

---

## 2. The operator's actual jobs

Six jobs, in priority order. For each: when it bites, what polis built for it, what's wrong with copying that, and the smallest answer that closes the gap. "Smallest" is the load-bearing word.

### Job 1 — Triage attention across the fleet

> "Where do I look first?"

**When it bites:** every time the operator opens chrote or pings clawdbot. Linearly with fleet size.

**What polis built:** five overlapping systems — `polis-monitor`, `agent-status`, `relay status`, `work status`, `polis-status.sh`. Master-plan-v2 lists "deduplicate status surfaces" as a phase 4–5 problem.

**Why copying is wrong:** chrote already shows sessions, beads, files. Adding a sixth surface called "Citizens" does not solve triage — it adds a sixth thing to look at.

**Smallest answer:** one **attention feed** at the top of the dashboard. Pulled from signals chrote already has — idle time per pane, `bd ready`, journalctl errors, recent stop-hook failures. Sorted by "needs me." No new daemon. No new schema. Possibly no new tab — slot it into the existing OracleView.

The polis idea this *does* borrow: the principle that **the system reports what is wrong, not what is healthy** (Argus's discipline, not Argus's binary).

---

### Job 2 — Direct one agent without setting up infrastructure

> "Tell session-X to do Y."

**When it bites:** any time the operator wants to act on a specific session, especially from the phone.

**What polis built:** `relay` — typed envelopes (`type | from | to | thread_id | payload`), threading, file reservations, wake guardrails, budgets, throttles, broadcast policy.

**Why copying is wrong:** chrote runs one operator and a small fleet. Reservations, wake budgets, and broadcast policy solve concurrency problems chrote does not have. The 100-agent shape was a polis problem.

**Smallest answer:** a single endpoint — `POST /api/sessions/:name/send {prompt}` — that does `tmux load-buffer | paste-buffer | send-keys Enter`. From clawdbot, `"tell hq-main: write the README"` lowers to that one call. The session name *is* the address. The thread *is* the tmux history.

If contention emerges later (two operators? scheduled tasks colliding?), promote to a real envelope. Not before.

---

### Job 3 — Trust that "done" means done

> "When this agent says it's finished, it actually is."

**When it bites:** every closeout. Agents over-claim. Operator either trusts and ships broken work, or hand-verifies everything.

**What polis built:** four overlapping systems — `squire` (stop-hook), `gate` (tests + lint + truthsayer + ubs), `oathkeeper` (commitment tracker), `aletheia/truthsayer` (anti-pattern scanner).

**Why copying is wrong:** chrote is not the place to enforce code quality. The harness (Claude Code, Pi, Codex) and the project's own CI are. Chrote's leverage is *the moment of closeout* — that's it.

**Smallest answer:** a closeout checklist that lives in the harness's stop-hook config and runs in the agent's own session. One shell script in `scripts/closeout.sh`. Default checks: tests pass if any code changed; the claimed bd issue is actually closed; no `2>/dev/null` was added on tmux commands. ~30 lines. Owned by chrote because chrote ships the harness config templates, not because chrote runs a "squire service."

What chrote does *not* do: scan for anti-patterns, track commitments across sessions, or run a compliance daemon. Those are the polis spirals.

---

### Job 4 — Carry context forward

> "Last week I figured out X. Don't make me figure it out again."

**When it bites:** every new session on a project chrote has touched before.

**What polis built:** the most over-engineered area in the city. `learning-loop` binary with SQLite, `work feed` JSONL emitter, daily `memory/YYYY-MM-DD.md` files per agent, `MEMORY.md` curated layer, `memory-curator` skill, `sleep` skill, archive/wakeup/USER files, the Hierophant role guarding canon. A whole governance layer for context.

**Why copying is wrong:** in a 5-agent fleet, the curation overhead exceeds the curation benefit. Polis itself flags `learning-loop integration` as "code exists, not wired" in master-plan-v2 — they built the machine but never closed the loop.

**Smallest answer:** **one file per project: `/code/<project>/HANDOFF.md`**. Sessions read it on boot. Sessions append to it on close (a one-line entry: date, what was done, what was learned, what's next). The operator edits it by hand when they want. That's it. No daemon. No schema. No timer.

If volume becomes unmanageable, *then* talk about pattern extraction. Until then, a markdown file beats a binary every time.

---

### Job 5 — Survive my absence

> "If I'm gone for 4 hours, I want to come back to a system, not a mess."

**When it bites:** overnight, on weekends, during meetings.

**What polis built:** `argus` watchdog with action set (restart_service / kill_pid / kill_tmux / clean_disk / alert / log), boot grace, atomic state writes, fallback drain queues, beads escalation, plus `oathkeeper` for promise tracking, plus heartbeats per agent.

**Why copying *some* is right, copying *all* is wrong:** the underlying job — "tell me when something is wrong, don't act unless I asked you to" — is real. But the elaborate action machinery is what made argus a maintenance burden in polis (master-plan-v2's "valuable systems implemented but not actually operating" row). Auto-remediation is where complexity accrues fastest.

**Smallest answer:** a 10-minute systemd timer that runs a check script and, *if anything looks wrong*, posts a single message to clawdbot (or relay-style, an inbox file). No auto-restart. No auto-kill. Just **observe and notify**. The operator decides whether to act.

The polis lesson worth keeping is the **closed action set as a discipline**: a watchdog should never have unbounded permissions. For now, chrote's watchdog has *zero* actions. Add them one at a time, only when the operator's manual response has been the same three times in a row.

---

### Job 6 — Absorb new tools without breaking

> "A new framework drops every two months. I want to evaluate and integrate without rewriting chrote."

**When it bites:** every time something new shows up (which is constantly).

**What polis built:** phase-gated planning, contract change rules, controlled deletion lifecycle, master plans v1 and v2, four design packets per phase. Industrial-grade.

**Why copying is wrong:** chrote is a single-operator project. Master-plan ceremony is the *exact* shape of complexity polis chose to die on.

**Smallest answer:** chrote already has `docs/TOOL_ABSORPTION_PROCESS.md` — it's lighter than polis's framework and works for one operator. Promote it from "a doc" to "the gate." Anything in this plan that gets built must pass Phase 3 (Challenge) of the absorption process first, including the items above.

The polis idea worth keeping: **rollback before forward-fix**. If a new feature regresses something, revert before debugging.

---

## 3. The budget

To prevent this plan from accreting into polis-shaped sprawl, commit to a budget *now*:

| Resource | Budget for everything in this plan |
|---|---|
| New API endpoints | **3** (Job 2: send; Job 5: notify; one slack/discretion) |
| New systemd units | **1** (Job 5: the watchdog timer) |
| New daemons (long-running processes) | **0** |
| New file conventions | **2** (`HANDOFF.md` per project, `closeout.sh` per harness config) |
| New dashboard tabs | **0** (use the existing surface; OracleView gets the attention feed) |
| New binaries shipped with chrote | **0** |
| New docs | **3** (this plan; an updated CHROTE-VISION footnote on the JTBD framing; a one-page CLOSEOUT.md) |

If a future change wants more, it must justify why this budget is wrong. The default answer is "no."

---

## 4. What is explicitly cut from v1

The previous version of this plan proposed seven phases. Here's what I'm cutting and why:

| v1 proposal | Why cut |
|---|---|
| Constitutional layer (Golden Truths, Document Modes, chrote Mythology) | Solving for "doctrine drift" — a polis problem at polis scale. One operator does not need a constitution; chrote already has CLAUDE.md and a vision. |
| Agent workspace convention (`SOUL.md` / `MEMORY.md` / `IDENTITY.md` / `HEARTBEAT.md` / `inbox/` / `outbox/`) | Convention serves polis's identity-driven agents. Chrote hosts whatever harness the operator throws at it; do not impose six files on every fleet. |
| Spawn primitive (pantheon pattern as a chrote API) | Already partially exists in `internal/teams/engine.go`. Generalizing was the right instinct, but doing it as a "phase" implied building beyond what's needed. |
| Relay (typed envelopes, threading, reservations, WebSocket stream) | See Job 2. Replaced by one endpoint. |
| Squire as a service | See Job 3. Replaced by one shell script. |
| Learning loop + memory curator | See Job 4. Replaced by one markdown file. |
| Argus watchdog with action set | See Job 5. Replaced by an observe-only timer. |
| Senate (deliberation API + UI) | Solving "decisions need precedent" — a polis problem driven by multiple agents disagreeing. One operator + N agents is not a polis. Cut entirely. |
| Phase-to-beads conversion before commit, contract change rule, etc. | Polis ceremony. Replaced by the existing Tool Absorption Process. |

What survives: a handful of *principles* (fail closed, observe-don't-act, the agent is the reader, length is diagnostic). Principles are cheap; mechanisms are expensive.

---

## 5. What to do this week vs defer

**This week, if any of it:**

1. Write `HANDOFF.md` for one active project. Use it for one session. See if the next session benefits. (Job 4 — the cheapest experiment, highest leverage.)
2. Write `scripts/closeout.sh` and wire it into the Claude Code stop hook for one new session. Watch what it catches over the next 10 sessions. (Job 3.)

That's it for this week. Two changes. Both reversible in 5 minutes.

**Next, if those work:**

3. Add `POST /api/sessions/:name/send`. (Job 2.) Wire clawdbot to it. Don't envelope-ify.
4. Add the 10-minute watchdog timer. Observe-only. Output → clawdbot. (Job 5.)
5. Add the attention feed to OracleView, sourced from existing signals. (Job 1.)

**Defer indefinitely until evidence demands them:**

- Anything resembling polis's relay, senate, learning-loop daemon, or memory hierarchy.
- Auto-remediation in the watchdog.
- A new dashboard tab.
- A new binary shipped with chrote.

The discipline is not "build less." The discipline is: **build the smallest thing, watch it for two weeks, then decide whether the next thing is actually needed.** Polis built the next thing first.

---

## 6. The one sentence to remember

> Polis built solutions in search of jobs. Chrote names jobs in search of the smallest answer.

If a future change to chrote does not begin with "the operator's job is X, the current gap is Y," it is probably importing the polis disease. Stop and ask which job it serves. If the answer takes more than a sentence, the answer is no.

---

## Appendix — what polis got right that survives the cut

These are *principles*, not tools. They cost nothing to adopt:

- **Fail closed at boundaries.** Missing dependency → 503 with a named cause, not soft-degraded silence.
- **The agent is the reader.** Names, comments, error messages must make sense to a fresh session.
- **Observe, don't act.** A monitor that creates work is cheaper than a monitor that takes action and gets it wrong.
- **Length is diagnostic.** A long doc is a question: why? A long endpoint is a question: why?
- **Rollback before forward-fix.** If a new feature regresses something, revert first, debug second.
- **Sessions are sacred.** Chrote's existing golden rule, reinforced.

These six lines can sit at the top of CLAUDE.md. That's the entire constitutional layer chrote needs.
