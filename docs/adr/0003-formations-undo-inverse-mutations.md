# ADR-0003: Formations Undo Uses Inverse Mutations

## Status
Accepted

## Context
S3 adds undo for board and layout authoring: slot assignment, wiring, gates,
missions, briefs, verification, and hand-routed wire lanes. Formations files can
also be edited by `archon`, the CHROTE API, and humans between the original
action and the undo.

The main alternatives were:
- keep an in-memory UI snapshot stack and restore whole board or layout files;
- store full pre-change TOML snapshots in the API and restore them on undo;
- record inverse mutations and apply them through the shared writer with fresh
  ETag and board revision checks.

Whole-board snapshot restore matches the vanilla prototype, but it would bypass
the file write contract's stale-edit protection and could silently erase changes
made by another client after the original action.

## Decision
Undo for Formations authoring uses inverse mutations, not whole-board snapshot
restore.

Each mutating UI action records the stable ids and previous modeled values needed
to ask the shared writer for the inverse operation. Structural undo calls the
board PATCH path with the current board ETag and expected revision. Layout undo
calls the layout PATCH path with the current layout ETag. If either ETag is
stale, undo fails loudly and the board is reloaded instead of restoring an old
serialized file image.

Undo covers board and layout authoring state only. It does not undo run ledger
output, terminal window state, tmux session state, or Beads records.

## Consequences
This keeps undo aligned with the single-writer contract, preserves comments and
unknown fields through the same patch paths as normal authoring, and prevents a
stale browser from clobbering newer CLI or file edits.

It also makes undo more explicit than the prototype snapshot stack. Every new
S3 mutation needs a tested inverse, and some undo actions can now conflict when
another writer has changed the same board or layout. The UI must surface those
conflicts clearly rather than pretending undo always succeeds.
