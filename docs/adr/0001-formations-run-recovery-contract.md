# ADR-0001: Formations Run Recovery Contract

## Status
Accepted

## Context
Formations runs are append-only NDJSON ledgers driven through tmux sessions that
do not provide a native acknowledgement. Once runs exist, changing resume or
redispatch semantics would break old ledgers and make recovery hard to trust.

The main alternatives were:
- resend any prompt whose result is missing after replay;
- make every blocked resume create a new run id;
- keep one run id and use epochs to record explicit resumed attempts.

## Decision
Use one `run_started` event at `seq=1` for the run and keep `seq` monotonic for
the whole ledger. `epoch` starts at `0` and increments only when an operator
explicitly resumes a blocked run. A `run_blocked` event stops dispatch for the
current epoch but leaves the run resumable. `run_succeeded`, `run_failed`, and
`run_canceled` are final for the whole run.

An accepted resume appends `run_resumed` as the first event in the next epoch and
records the `run_blocked` sequence it resumes from. Process reconnects or replay
reattach attempts do not advance the epoch by themselves; they append proof
events or block loudly when recovery cannot be proven.

At run start, write immutable board and resolved persona binding snapshots before
`run_started`. Resume and dispatch use those snapshots, not current board,
layout, or persona files.

Record `slot_dispatch` before sending to a tmux pane. On replay, a dispatch
without a matching `slot_result` is a lease to reattach capture or record a loud
error. The engine never blindly sends the same prompt again.

Status projection is ledger-only. Live tmux inspection belongs to the recovery
reconciler, which changes status by appending events to the ledger.

## Consequences
This keeps ledgers replayable, makes "what happened?" answerable from disk, and
prevents duplicate agent work after crashes. It also means resume logic is more
explicit: blocked runs need an operator action, event projection must understand
epochs, and the first S4 implementation must test unresolved dispatch leases
before shipping real tmux dispatch.
