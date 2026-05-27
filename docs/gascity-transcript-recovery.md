# Gas City Transcript Recovery

CHROTE recovers recent Gas City session output through a narrow server-side
adapter, backed by a bounded CHROTE-owned archive so a transcript survives a
supervisor restart (home-5ubb):

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

The line count is bounded, terminal output is sanitized (ANSI/control bytes
stripped), and the returned text is capped. The response identifies the source
and includes the resolved session id, template, alias, state, city, requested
line bound, and returned line count.

The dashboard Gas City sessions panel calls this route from the per-session
transcript action. The browser sends only the immutable `gc-*` id from the
observer model; aliases such as `planner` are display labels only.

## Why A Single Live Peek Is Not Enough

Two facts make a single live `gc session peek` insufficient for "boring"
recovery after a supervisor restart, verified locally on 2026-05-27:

- `gc session peek` reads **volatile tmux pane scrollback**. The Gas City
  supervisor recreates session tmux panes on restart (in the live city all
  panes — `planner`, `reviewer-a/b`, `s-gc-51923` — were recreated at the last
  restart timestamp). Pre-restart pane output is therefore gone after a restart.
- `gc session logs` reads provider-native structured transcript files from
  `~/.claude/projects/` plus any `[daemon] observe_paths` in `city.toml`. In the
  live city no session carries a `session_key` and no `observe_paths` are
  configured, so `gc session logs <gc-id>` fails for mock sessions
  ("no session_key and workdir fallback is ambiguous") and returns nothing even
  for the Pi session. It also has no JSON output mode.

So neither gc command alone gives CHROTE a transcript that survives a restart.

## CHROTE-Owned Transcript Archive

Per the substrate map, CHROTE owns recovery. CHROTE therefore archives every
successful sanitized peek to a bounded on-disk store keyed by the immutable
`gc-*` session id:

- On a successful, non-empty live peek, CHROTE writes the sanitized snapshot to
  the archive and returns it with `source: gc-session-peek`, `stale: false`.
- On a later request where the supervisor cannot resolve the session (restarted,
  down, or pruned), or the peek command fails, or the peek returns an empty pane
  (recreated empty after restart), CHROTE serves the last archived snapshot with
  `source: chrote-archive`, `stale: true`, and a `capturedAt` timestamp.

This turns a restart-volatile live peek into a retrievable last-known
transcript, so an operator can recover a Gas City-owned session transcript after
a supervisor restart, beyond a single bounded peek. The dashboard shows a clear
"Recovered from CHROTE archive" banner when the response is stale.

The archive is **not** a durable memory source and does not change source-of-
truth boundaries: Context Citadel remains durable-context truth and Gas City
remains orchestration truth. The archive is an operator-recovery cache only.

### Storage, Configuration, And Retention (Bounded)

- Location: `$XDG_STATE_HOME/chrote/gascity-transcripts` (default
  `~/.local/state/chrote/gascity-transcripts`), outside the Gas City runtime
  tree. Override with `CHROTE_GASCITY_TRANSCRIPT_DIR=<dir>`. Set
  `CHROTE_GASCITY_TRANSCRIPT_DIR=off` to disable archiving (live peek only).
- One snapshot file per session id (`<gc-id>.json`); each new successful peek
  atomically replaces the previous snapshot for that session.
- Per-snapshot size is bounded by the existing transcript output cap
  (`gasCityTranscriptOutputLimit`, 64 KiB) applied before archiving.
- At most `gasCityArchiveMaxSessions` (64) distinct sessions retain a snapshot;
  the oldest-captured snapshots are evicted (LRU by file mtime) past that cap.
- Files are written `0600` in a `0700` directory (user-private).
- Only already-sanitized transcript text is archived; no credentials, tokens, or
  raw control bytes are written. The archive write is best-effort and never
  fails or blocks the live response.

## Residual Limitations (Explicit)

- The archive only contains what CHROTE has peeked. A session that is never
  peeked while live has no archived transcript to recover after a restart. This
  is acceptable: recovery covers transcripts an operator has actually viewed
  through CHROTE.
- It is a bounded recent-output snapshot, not a full provider transcript. It
  does not change the real-harness transcript retention boundary from ADR-0002.
- It is a snapshot, not a live follow/stream.
- Provider-native structured transcripts do exist on disk for some harnesses
  (for example `.gc/pi-sessions/*.jsonl`), but they are keyed by the provider's
  own UUID, not the `gc-*` id, and gc exposes no `gc-id → transcript-file`
  bridge here. If a session type later exposes provider-native logs addressable
  by `gc-*` id, CHROTE can add a separate explicit source instead of silently
  changing this adapter.

## Disposable-City Restart Safety Note

Restart-recovery behavior must only be exercised in a disposable, `GC_HOME`-
isolated city per the "Safe Disposable City Proof Path" in
`chrote-gascity-substrate-map.md`. The live `/home/perttu/gascity` supervisor
must never be restarted for testing. The archive recovery path itself is fully
covered by unit tests (`gascity_test.go`) using fake/scripted runners and an
httptest supervisor, with no live restart required.
