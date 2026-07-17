# ADR-0005: Redacted Runs Fail When Replay Truth Is Unavailable

## Status
Accepted

## Context
Formations routes agent and tool output between nodes and records enough evidence
to explain and recover a run. A `Redact=true` run makes a stronger promise: raw
execution values must not become durable evidence merely because the ledger,
verifier, capture, report, or artifact pipeline observed them.

That promise conflicts with ordinary replay. A later node may need the raw value
that a prior node produced, while the durable ledger contains only redacted
evidence. Treating a redaction marker or display summary as the original value
would execute different work. Persisting a hidden raw copy would instead create
a secret store with new key, access, backup, rotation, and erasure obligations.

Executor capture also creates a crash boundary. Raw bytes may exist briefly
while a live attempt is being collected. If the process stops between recording
the node result and replacing those bytes, recovery must still know which paths
it owns and must finish redaction before the run can succeed.

## Decision
Separate **execution-authoritative values** from **durable evidence**.

For a fresh `Redact=true` execution, the engine may route an authoritative raw
value only from the live attempt's ephemeral in-memory state. It does not persist
that value for later dispatch. Redaction markers, display summaries, hashes, and
safe reference metadata are evidence only and must never be dispatched as a
substitute for the raw value.

The redaction boundary includes every run-owned durable surface: ledger payloads,
verifier and per-kind feedback, captures, reports, artifact contents, output
references and their targets, derived evidence, and error text. Reference
metadata may remain only when its target is already sanitized or replaced,
resolves inside an authorized root, and contains no raw value covered by the
run's redaction policy.

Raw executor capture may exist only as cleanup-owned transient material, not as
accepted durable evidence or a replay store. Before any persistent path can
contain raw capture bytes, the engine writes and fsyncs a durable
pending-redaction obligation that owns that exact target. Its internal cleanup
locator is the only metadata allowed to point temporarily at an unsanitized
target; it is never exposed as an output ref or graph input. Recovery completes
the obligation idempotently. Reprocessing valid redacted evidence preserves its
bytes and provenance rather than hashing or wrapping it again. A run cannot
append `run_succeeded` while any run-owned capture, report, or artifact is
pending redaction.

Recovery may reattach an unresolved dispatch only when it proves the same
attempt and qualified session target and sends no new prompt. It may also finish
pending redaction or other reconciliation that needs no discarded raw value.
After replay, a new dispatch is allowed only when every required input is
available under the ordinary non-secret durable-input contract. A pending
cleanup target, capture, report, artifact, marker, hash, or display summary is
never an allowed source for reconstructing graph input.

If a required value was intentionally discarded and cannot be proven available,
the engine appends an event with `type=run_failed`,
`data.code=redacted_input_unavailable`,
`data.reason=redacted_input_unavailable`, `data.unrecoverable=true`, and
`data.final=true`. `data.relatedSeq` names the exact source event whose
execution-authoritative value was required but unavailable. This is terminal for
the run: resume is rejected, no new epoch is opened, and no marker or summary is
dispatched. Retrying the work requires a new run with newly supplied
authoritative input.

## Rationale
A typed terminal failure preserves both promises: redaction remains meaningful,
and replay never pretends evidence is execution truth. Allowing exact reattach
and cleanup still recovers work that can be proven safe without making every
interrupted redacted run fail.

## Alternatives Considered
- **Persist encrypted raw payloads for replay:** rejected for this phase. It
  needs a separate security design for key lifecycle, authorization, backup,
  rotation, audit, and erasure.
- **Dispatch redaction markers or display summaries:** rejected. They are
  evidence, not the value the producer emitted.
- **Use `run_blocked` and invite resume:** rejected. A blocked run implies a
  recovery path inside the same run; discarded authoritative input has none.
- **Fail every interrupted redacted run:** rejected. Exact reattach, pending
  cleanup, and reconciliation that needs no discarded value can remain safe.
- **Exclude referenced files from redaction:** rejected. A sanitized ledger that
  points to raw content still leaks the content.

## Consequences
Some redacted runs that an unredacted run could replay now end in a clear final
failure and must be started again. Operators receive less raw debugging evidence,
and producers of reports or artifacts must participate in redaction finalization.
The engine also needs durable cleanup ownership, an idempotent recovery scan, and
an exact check before final success or resumed dispatch.

In return, run history stays honest: durable evidence can explain that data was
redacted without becoming a hidden source of execution input. The UI and CLI can
surface one stable reason code instead of presenting a resumable state that can
never succeed safely.

## Enforcement
- `ctx-phz` owns failing-first regressions for marker routing, verifier echo,
  capture ownership on every exit, the node-output-to-capture crash window, and
  idempotent `CHROTE-CAPTURE-REDACTED-V1` cleanup.
- The regressions scan the ledger and every run-owned capture, report, artifact,
  output target, derived field, and error for a generated raw fixture secret.
- `ctx-ug7.5` certifies the stabilized foundation, and `ctx-ug7.15` repeats the
  boundary against the exact completed feature candidate.
- `FORMATIONS.md` and `DATA-MODEL.md` carry the same replay and durable-evidence
  vocabulary; `scripts/doc-lint.py` remains the active-spec hygiene gate.
