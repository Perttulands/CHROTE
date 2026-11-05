---
title: The launcher and agent events
nav: Launcher and agent events
summary: Name the harnesses, flags and folders a new session may start with, and let launched agents report when they finish.
sources: docs/installation.md, DESIGN-SYSTEM.md, ARCHITECTURE.md, SECURITY.md
---

# The launcher and agent events

## The launch configuration

`CHROTE_LAUNCH_CONFIG` names a JSON file listing what a new session may start,
with which flags, and where. Unset, the launcher offers only `Shell` in `~`.

```json
{
  "harnesses": [
    {
      "id": "example-agent",
      "label": "Example Agent",
      "command": "example-agent",
      "defaultFlags": "--flag",
      "flags": [
        {"name": "--flag", "description": "What the flag does"},
        {"name": "--model", "short": "-m", "value": "<model>", "description": "Model to use"},
        {"name": "--mode", "value": "<MODE>", "values": ["a", "b"], "description": "One of two modes"}
      ]
    },
    {"id": "shell", "label": "Shell", "command": ""}
  ],
  "folders": ["/absolute/path/to/project", "~"]
}
```

| Field | Meaning |
| --- | --- |
| `id` | Unique, `[a-z0-9-]+`. `claude-code` and `codex` get completion hooks wired automatically |
| `label` | What the launcher shows |
| `command` | The binary. An entry with flags inside `command` is split on load: first word binary, the rest default flags |
| `defaultFlags` | The flag line the launcher prefills; you can edit it before launching |
| `flags` | Optional catalogue the launcher's flag panel lists and searches. Only `name` and `description` are required |
| `folders` | Pinned folders: absolute, or starting with `~`, which resolves against the target Unix user's home |

`shell`, the bare login shell, is offered whether or not the file lists it.

When you launch, the browser sends a harness id and an optional one-line flag
string, never a command. The server types `binary flags` into the new session,
refusing control characters in the flags. A preview in the launcher shows the
exact line before it runs.

Set but unreadable or invalid, the file stops the server at startup with the
reason in the log, rather than running a launcher that cannot launch. Check what
the server read with:

```bash
curl http://127.0.0.1:8094/api/launch
```

> [!NOTE]
> The launcher adds no capability a terminal did not already give: anyone at
> the dashboard can type any command into a shell. The configuration exists to
> make the right start quick, not to restrict.

## Agent events

CHROTE learns that an agent finished a turn, or is waiting for you, from the
harness's own completion hook, never from guessing at terminal output.

The hook is `chrote-agent-event`, a POSIX `sh` script the installer puts beside
`chrote-server`. It asks tmux for its session's name and posts
`POST /api/agent/event` with `{ session, unixUser, event, summary? }`, where
`event` is `finished` or `needs-input`. It gives up after two seconds and always
exits 0, so a notification can never fail the harness.

With **Notify on completion** on in the launcher (the default), each launch
wires the hook through the harness's own flags:

| Harness id | How the hook is installed |
| --- | --- |
| `claude-code` | `--settings <file>`, a generated settings file whose `Stop` hook reports `finished` and whose `Notification` hook reports `needs-input` |
| `codex` | `-c notify=[…]` naming the script |
| anything else | Launched as it is, with no hook |

The server keeps each session's last event in memory and forgets it with the
session. Nothing is persisted and nothing polls the harness.

### What each device does with it

Under **Settings → Agent events**, per device:

| Setting | Default |
| --- | --- |
| Mark the session's row and the tab holding it | On |
| Name the session in a toast | On |
| A short tone per kind: finished is two falling notes, needs-input one higher note | Off |
| A browser notification while the tab is hidden | Off; needs a secure origin, and the setting shows what the browser answered |

The status line keeps the last report whatever is on. Focusing the session marks
it seen on every device.

### Wiring a hook by hand

For a session CHROTE did not create, set `CHROTE_AGENT_EVENT_URL` to
`http://127.0.0.1:<port>/api/agent/event` in that session and have the
harness's own hook run `chrote-agent-event finished` or
`chrote-agent-event needs-input` inside tmux. The script needs `tmux` and
`curl` on its `PATH`. A report about a session that
does not exist on that user's socket is answered `404`.
