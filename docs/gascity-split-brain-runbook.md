# Gas City Split-Brain Reconciliation Runbook

Date: 2026-05-27

This runbook covers mismatch cases between `/home/perttu` Beads, Gas City
runtime records, and Context Citadel. It is read-only by default. Any destructive
repair requires explicit Perttu approval.

## Sources

- Work truth: `/home/perttu` Beads.
- Runtime truth: Gas City supervisor and `.gc` runtime records.
- Durable context truth: Context Citadel.

## Read-Only Inspection

```bash
cd /home/perttu
bd show <home-id>
bd list --status=open --json

cd /home/perttu/gascity
gc status
gc beads health
gc mail list --json
gc event list --json
gc session list --json
```

Do not hand-edit `.gc/beads.json`, `.gc/events.jsonl`, `.beads/issues.jsonl`, or
Context Citadel data files as a repair step.

## Cases

### Gas City Open, Beads Closed

Symptom: a `gc-*` workflow/session/mail thread remains active after the linked
`home-*` bead is closed.

Rule: Beads wins for work lifecycle.

Read-only response:

1. Read the bead close reason.
2. Read the Gas City mail/session/event record.
3. Record whether the Gas City record is only historical evidence or still
   attempting live work.

Repair requires approval if stopping, archiving, or deleting runtime state.

### Beads Open, Gas City Missing

Symptom: a `home-*` bead expects Gas City orchestration, but no matching
`gc_workflow_id`, session, mail, or event exists.

Rule: Gas City projection is missing; the bead remains canonical.

Response:

1. Continue tracking work in Beads.
2. Recreate or relink Gas City runtime only if the bead still needs
   orchestration.
3. Add the new `gc-*` id to Beads metadata or notes.

### Both Open, Fields Disagree

Symptom: Beads owner/status/priority differs from Gas City routed target/status.

Rule: Beads wins for work truth; Gas City explains runtime state only.

Response:

1. Use Beads to decide the intended owner/status.
2. Use Gas City events to understand what actually happened.
3. Do not retarget a live paid/credentialed harness without the accepted safety
   boundary and explicit approval when the action could mutate files or spend
   money.

### Gas City Mail Without Beads Task

Symptom: useful result exists in Gas City mail, but no canonical work bead links
to it.

Rule: preserve mail as runtime evidence; create/link Beads before treating it as
durable work state.

Response:

1. Record the mail id/thread id.
2. Create a Beads issue or append a note to an existing bead.
3. Use Context Citadel only for durable context claims, not raw transcripts.

### Beads Unavailable, Gas City Running

Symptom: `bd` commands fail but Gas City still runs.

Rule: freeze canonical work mutations.

Allowed:

- observe Gas City status;
- read mail/events/sessions;
- preserve runtime evidence.

Not allowed without recovery:

- create/close/claim canonical work through Gas City alone;
- treat Gas City records as replacement Beads truth.

### Gas City Unavailable, Beads Healthy

Symptom: supervisor/city is down, but Beads works.

Rule: keep working through Beads and CHROTE; rebuild projection later.

Response:

1. Continue canonical work tracking in Beads.
2. File or update a recovery bead.
3. After Gas City recovers, relink runtime records from Beads and `.gc` evidence.

### Context Mismatch

Symptom: Gas City mail or agent output contradicts Context Citadel.

Rule: Context Citadel wins for durable context until updated through approved
append/contribution paths.

Response:

1. Ask/retrieve from Context Citadel for current context.
2. Save new durable claims through Context Citadel contribution or append paths.
3. Link the context update back to the relevant Beads issue.

## Approval Gates

Require explicit approval before:

- deleting or rewriting `.gc` runtime files;
- stopping live non-disposable harness sessions;
- exposing the supervisor beyond localhost;
- changing Beads status based only on Gas City runtime state;
- storing raw private transcripts in docs, Beads, or Context Citadel.
