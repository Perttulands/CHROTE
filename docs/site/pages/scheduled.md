---
title: Scheduled
nav: Scheduled
summary: Host-owned tasks that type a prompt into named tmux sessions on a schedule, with a delivery record for every target.
sources: docs/scheduled-tasks.md, SECURITY.md
---

# Scheduled

A scheduled task sends a prompt, verbatim, into one or more named tmux sessions
on an interval or a cron schedule. Tasks are stored by the CHROTE server in its
state directory (`CHROTE_SCHEDULED_TASKS_DIR`), not in the browser and not in
the host's cron.

## In the tab

Each task shows its prompt, every target session, its schedule in plain words
with its timezone, whether it is enabled or paused, the next run with a
countdown, the last status, and recent runs with a result per target.

- **Create or edit**: schedules are built from **Every**, **Daily**, **Weekly**
  and **Cron** presets; a raw cron expression is only needed for what the
  presets do not cover. The timezone defaults to the browser's.
- **Targets**: drag sessions from the Sessions sidecar onto a task's **Send to**
  zone, or pick them in the editor. The Unix user comes from the session.
- **Act**: edit, pause and resume, run now, duplicate, delete.

From a session row's menu, **Schedule task…** opens the Send drawer with that
session as the fixed target. **Create task** saves directly. **Have agent
refine** instead sends your draft, with CHROTE's scheduler API instructions, to
an agent through Send to Session, and the task the agent creates appears here.

## What a run promises

A run is recorded per target. A missing session fails only its own target and
never blocks the others; a run whose targets did not all succeed is `partial`.
`submitKeyDispatched: true` is a tmux transport receipt: CHROTE pasted the
prompt into the verified pane and dispatched `Enter`. It does not claim the
program accepted or began processing it.

If the pane changed between resolution and paste, or tmux does not confirm the
paste, the run fails for that target rather than being retried blindly. Two
mutations of one task never overlap: the second gets `409 CONFLICT`.

## From an agent or a script

The API at `/api/scheduled-tasks` creates, lists, updates, pauses, resumes, runs
and deletes tasks. Every mutation must carry `X-Chrote-Intent: scheduled-task`.
Targets are always `unixUser` plus `sessionName`; a client never sends a socket
path, because CHROTE resolves sockets itself. The schema, every request, and
the full safety contract are in the
[scheduled tasks reference](../../scheduled-tasks.md).

```bash
curl -fsS http://127.0.0.1:8094/api/scheduled-tasks
```

## If a task looks stuck

Read the run history in the tab and the service log first. CHROTE may reclaim a
stale lock under its contract; it will not double-run a task holding a fresh one.
Do not delete lock or state files until you know whether another process is
still running.
