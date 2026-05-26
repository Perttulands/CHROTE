# Gas City Transcript Recovery

CHROTE recovers recent Gas City session output through a narrow server-side
adapter:

```text
GET /api/gascity/sessions/{id}/transcript?lines=120
```

The route accepts only stable Gas City session ids such as `gc-4171`. It does
not accept aliases, configured agent names, or raw tmux session names. The
server resolves the id through the configured localhost Gas City supervisor,
checks that the configured city is running, then runs:

```bash
gc --city <cityDir> session peek <session-id> --lines <n>
```

The line count is bounded, terminal output is sanitized, and the returned text
is capped. The response identifies `source: gc-session-peek` and includes the
resolved session id, template, alias, state, city, requested line bound, and
returned line count.

The dashboard Gas City sessions panel calls this route from the per-session
transcript action. The browser sends only the immutable `gc-*` id from the
observer model; aliases such as `planner` are display labels only.

## Why Not `gc session logs`

`gc session logs` reads provider-native structured transcript files, including
configured observed transcript paths. In the local CHROTE/Gas City spike it did
not recover transcript data for tmux-backed mock sessions, while
`gc session peek` could recover the recent pane output.

For CHROTE 3.0, `gc session peek` is therefore the browser recovery adapter for
recent output. It is not a durable transcript archive and it does not change the
real-harness transcript retention boundary from ADR-0002.

## Limitations

- Snapshot only; it does not follow output streams.
- It relies on Gas City being able to peek the resolved session.
- It returns bounded recent output, not full private harness transcripts.
- If provider-native structured logs become available for a session type,
  CHROTE can add a separate explicit source instead of silently changing this
  adapter.
