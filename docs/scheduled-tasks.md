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
- target `unixUser / sessionName`;
- schedule and timezone;
- enabled/paused state;
- next run and last status;
- `createdBy` / `updatedBy` metadata.

Users can edit, pause/resume, run-now, duplicate, or delete agent-created tasks from the same UI.

## Safety contract

The API fails closed for:

- missing `X-Chrote-Intent: scheduled-task` on mutation requests;
- non-JSON create/update requests;
- empty prompts;
- invalid interval or cron schedules;
- invalid or unknown targets when target validation is enabled;
- user-supplied `socket` fields;
- session/user names outside CHROTE's safe target grammar.

Prompt text is sent to tmux with argv-only `tmux send-keys -l -- <prompt>` followed by `Enter`; prompt content is not shell-interpolated by CHROTE.
