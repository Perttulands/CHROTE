# CHROTE scheduled tmux tasks

Scheduled tasks are CHROTE-owned persisted jobs that send a prompt literally into a configured tmux session. They are stored by the CHROTE service under its data lane, not in browser local state, host cron, or Hermes cron.

Default API base on the `/srv` lane:

```text
http://127.0.0.1:8095
```

## Task schema

```json
{
  "id": "tsk_...",
  "name": "Daily standup nudge",
  "prompt": "Write the standup summary now.",
  "targets": [
    {"unixUser": "alice", "sessionName": "planner"},
    {"unixUser": "alice", "sessionName": "planner-2"}
  ],
  "target": {
    "unixUser": "alice",
    "sessionName": "planner"
  },
  "schedule": {
    "type": "interval",
    "everyMinutes": 60,
    "timezone": "UTC"
  },
  "enabled": true,
  "paused": false,
  "nextRun": "2026-06-27T15:00:00Z",
  "lastRun": "2026-06-27T14:00:00Z",
  "lastStatus": "success",
  "createdBy": "agent:athena",
  "updatedBy": "agent:athena"
}
```

Targets are always `unixUser + sessionName`. Do **not** send socket paths: CHROTE resolves allowed tmux sockets server-side from its terminal configuration.

One task may fan the same prompt out to several sessions. Send `targets` as an array;
the single `target` object is still accepted on create/update and is mirrored back in
responses as the first target so older clients keep working. A `PATCH` carrying
`targets` replaces the whole list.

Targets are independent at fire time: a session that is gone is recorded against that
target only and never blocks delivery to the healthy ones. A run whose targets did not
all succeed is stored with status `partial`, and every run entry carries per-target
results:

```json
{
  "id": "run_...",
  "trigger": "scheduled",
  "status": "partial",
  "message": "alice/planner-2: scheduled task target not found",
  "targets": [
    {"sessionName": "planner", "unixUser": "alice", "status": "success", "pane": "%7", "submitKeyDispatched": true},
    {"sessionName": "planner-2", "unixUser": "alice", "status": "error", "message": "scheduled task target not found"}
  ]
}
```

`submitKeyDispatched: true` is a tmux transport receipt only. It confirms that
CHROTE dispatched the submit key to the verified pane; it does not claim that
the terminal application accepted or began processing the prompt.

## List tasks

```bash
curl -fsS http://127.0.0.1:8095/api/scheduled-tasks | jq .
```

## Create an interval task

```bash
curl -fsS -X POST http://127.0.0.1:8095/api/scheduled-tasks \
  -H 'Content-Type: application/json' \
  -H 'X-Chrote-Intent: scheduled-task' \
  -d '{
    "name": "Hourly planner nudge",
    "prompt": "Review current plan and post blockers.",
    "target": {"unixUser": "alice", "sessionName": "planner"},
    "schedule": {"type": "interval", "everyMinutes": 60, "timezone": "UTC"},
    "enabled": true,
    "paused": false,
    "createdBy": "agent:athena",
    "updatedBy": "agent:athena"
  }' | jq .
```

The response includes `data.task.id`; keep that ID for updates/actions.

## Create a task that targets several sessions

```bash
curl -fsS -X POST http://127.0.0.1:8095/api/scheduled-tasks \
  -H 'Content-Type: application/json' \
  -H 'X-Chrote-Intent: scheduled-task' \
  -d '{
    "name": "Continue work",
    "prompt": "Continue if work is clear, keep things moving.",
    "targets": [
      {"unixUser": "alice", "sessionName": "worker-1"},
      {"unixUser": "alice", "sessionName": "worker-2"}
    ],
    "schedule": {"type": "cron", "expression": "0 16 * * *", "timezone": "Europe/Helsinki"},
    "createdBy": "agent:athena",
    "updatedBy": "agent:athena"
  }' | jq .
```

## Create a cron task

Cron schedules use five fields: minute, hour, day-of-month, month, day-of-week.

```bash
curl -fsS -X POST http://127.0.0.1:8095/api/scheduled-tasks \
  -H 'Content-Type: application/json' \
  -H 'X-Chrote-Intent: scheduled-task' \
  -d '{
    "name": "Weekday morning review",
    "prompt": "Check overnight CHROTE runs and summarize anything stuck.",
    "target": {"unixUser": "alice", "sessionName": "planner"},
    "schedule": {"type": "cron", "expression": "0 9 * * 1-5", "timezone": "Europe/Helsinki"},
    "enabled": true,
    "paused": false,
    "createdBy": "agent:athena",
    "updatedBy": "agent:athena"
  }' | jq .
```

## Pause and resume

```bash
TASK_ID=tsk_example
curl -fsS -X POST "http://127.0.0.1:8095/api/scheduled-tasks/${TASK_ID}/pause" \
  -H 'Content-Type: application/json' \
  -H 'X-Chrote-Intent: scheduled-task' \
  -d '{"updatedBy":"agent:athena"}' | jq .

curl -fsS -X POST "http://127.0.0.1:8095/api/scheduled-tasks/${TASK_ID}/resume" \
  -H 'Content-Type: application/json' \
  -H 'X-Chrote-Intent: scheduled-task' \
  -d '{"updatedBy":"agent:athena"}' | jq .
```

## Run now

`run-now` sends the prompt immediately and records a run entry. It does not create a one-shot schedule.

```bash
TASK_ID=tsk_example
curl -fsS -X POST "http://127.0.0.1:8095/api/scheduled-tasks/${TASK_ID}/run-now" \
  -H 'Content-Type: application/json' \
  -H 'X-Chrote-Intent: scheduled-task' \
  -d '{"updatedBy":"agent:athena"}' | jq .
```

## Delete

```bash
TASK_ID=tsk_example
curl -fsS -X DELETE "http://127.0.0.1:8095/api/scheduled-tasks/${TASK_ID}" \
  -H 'X-Chrote-Intent: scheduled-task' | jq .
```

## Dashboard readback

Open the dashboard at `http://127.0.0.1:8095`, then use the **Scheduled** tab. Agent-created tasks are user-visible there and show:

- task ID;
- prompt text;
- every target session;
- schedule in plain language plus its timezone;
- enabled/paused state;
- next run (with countdown) and last status;
- recent runs with per-target delivery results.

The tab carries the same Sessions sidecar as the terminal workspaces: sessions can be
dragged from it onto a task's **Send to** zone, or picked from the session list in the
editor. Schedules are built from **Every / Daily / Weekly / Cron** presets, so a raw cron
string is only needed for expressions the presets do not cover. The timezone defaults to
the browser's IANA zone; the unix user is taken from the selected session rather than
typed.

Users can edit, pause/resume, run-now, duplicate, or delete agent-created tasks from the same UI.

## Safety contract

The API fails closed for:

- missing `X-Chrote-Intent: scheduled-task` on mutation requests;
- non-JSON create/update requests;
- empty prompts;
- invalid interval or cron schedules;
- an empty target list, or more than 32 targets;
- invalid or unknown targets when target validation is enabled;
- user-supplied `socket` fields;
- session/user names outside CHROTE's safe target grammar; and
- a concurrent mutation of the same task. One task mutation owns the complete
  read/send/save operation; overlapping patch, pause/resume, delete, run-now, or
  scheduler work receives `409 CONFLICT` instead of dispatching twice or overwriting state.

Prompts are delivered through the same guarded path as **Send to Session**: the prompt is
staged in a private file, loaded into a private tmux buffer, and pasted only while the
resolved pane's `session_id`/`pane_id`/`pane_pid`/`server pid` generation still matches,
followed by an `Enter` key dispatch. That dispatch is a tmux transport receipt only; it does
not prove that the terminal application accepted or began processing the prompt. A successful
paste consumes its run-unique tmux buffer. Failure paths reserve part of the same delivery
deadline for synchronous buffer deletion; if tmux cannot confirm that cleanup, the persisted
run reports the cleanup failure. The staged file is always removed. Prompt text never reaches
a shell or a tmux argv, and nothing is appended to it. If the pane generation changed, or tmux
does not confirm the paste, the run is recorded as failed for that target instead of being
retried blindly.

For an unattended send CHROTE resolves the target session's active pane (a session with a
single pane resolves to that pane). Interactive sends still pin an exact pane, because a
human is there to disambiguate.
