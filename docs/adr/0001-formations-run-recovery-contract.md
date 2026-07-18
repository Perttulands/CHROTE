# ADR-0001: Formations Run Recovery Contract

## Status
Accepted; amended by ADR-0006

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
current epoch. ADR-0006 narrows the original blanket-resume rule: only a block
with `resumeAllowed=true` is resumable and carries `nextEpoch`; a non-resumable
block omits `nextEpoch` and requires a new run. `run_succeeded`, `run_failed`,
and `run_canceled` are final for execution. Non-authorizing binding and artifact
observation events may still follow for inspection, but cannot change outcome,
open an epoch, or authorize dispatch.
ADR-0006 adds fsynced `run_cancel_requested` as a non-final `canceling` state that
snapshots every open node attempt, slot dispatch, and Tool lease and forbids new
execution authority while Tool scopes are fenced. Its final cancellation, or a
terminal failure reached instead, disposes every open attempt so no final run
still projects active evaluation or human wait. ADR-0006 also overlays
never-started nodes in the frozen run root as canceled/abandoned, or `not_run`
on a valid success when they were provably unreached; `node_waiting` remains
inspection evidence rather than a post-final active state. It also permits
private Tool fencing and cleanup to continue after a terminal process-fence
failure, without ledger execution events and without promotion, result, or rerun
authority.

An accepted resume appends `run_resumed` as the first event in the next epoch and
records the `run_blocked` sequence it resumes from. Process reconnects or replay
reattach attempts do not advance the epoch by themselves; they append proof
events or block loudly when recovery cannot be proven.

At run start, write immutable board and resolved persona binding snapshots before
`run_started`. Resume and dispatch use those snapshots, not current board,
layout, or persona files.

Record `slot_dispatch` before sending to a tmux pane. On replay, a dispatch
without a matching `slot_result` is a lease to reattach capture or record a loud
error. ADR-0006 permits one bounded explicit `reattach_only` resume that
preserves the complete stable set of unmatched leases and sends no prompt. The
current block rejects results/output/routing until exact resume; that epoch may
prove and release individual results. If any remain unproven, a non-resumable
block keeps only that subset; late results/output/routing are rejected and the
run stays blocked until aborted. Node scope is valid only when the set belongs
to one node; otherwise recovery uses run scope. A separately started run is
independent and does not close or supersede an old lease. The engine never
supersedes an unmatched lease or sends the same prompt again.

Status projection is ledger-only. Live tmux inspection belongs to the recovery
reconciler, which changes status by appending events to the ledger.

## Consequences
This keeps ledgers replayable, makes "what happened?" answerable from disk, and
prevents duplicate agent work after crashes. It also means resume logic is more
explicit: resumable blocked runs need an operator action, non-resumable blocks
need a new run, event projection must understand epochs, and the first S4
implementation must test unresolved dispatch leases before shipping real tmux
dispatch.
