# ADR-0002: Formations File Write Contract

## Status
Accepted

## Context
Formations definitions are durable local files edited by both the CHROTE server
and future `archon` CLI verbs. They must preserve human-authored TOML comments
and unknown fields and reject stale edits. Code rollback must preserve both the
workspace `.formations/` tree and host-private canonical run authority under the
Formations host-authority root. Retention, migration, or deletion is a separate
explicit operator action, never part of rollback.

The main alternatives were:
- let each client parse and rewrite TOML independently;
- serialize through the running CHROTE server only;
- keep a shared local writer with file locks and optimistic revisions.

## Decision
All board, layout, and future persona-card definition writes go through the
shared Go `internal/formations` package. The server and future `archon` are peer
clients of that package rather than owning separate persistence models.

The writer serializes each file's read-check-mutate-write sequence with an
advisory lock next to the target file. It checks the caller's expected ETag and,
for structural board writes, expected revision while holding the lock. Stale
inputs fail loudly with a conflict instead of retrying or merging silently.
Missing preconditions fail before any write; bootstrap/create operations need
their own explicit no-overwrite paths rather than reusing stale-client updates.

Writes use a temporary file in the target directory, fsync the file, rename over
the target, and fsync the directory where the platform allows it. Board and
layout files use separate paths and locks; any future operation that needs both
must take locks in deterministic board-then-layout order.

## Consequences
The feature has one persistence contract for UI and CLI callers, and readers can
trust that comments, unknown keys, and rollback boundaries are not accidental.
Concurrent edits produce visible conflicts instead of lost updates.
Reverting code does not delete or rewrite definition, artifact, ledger, binding,
or private run-authority data.

The trade-off is that the writer is more careful than a normal TOML
marshal/unmarshal path. Future schema work must extend the shared patching
logic rather than bypassing it, and multi-file operations need explicit lock
ordering to avoid deadlocks.
