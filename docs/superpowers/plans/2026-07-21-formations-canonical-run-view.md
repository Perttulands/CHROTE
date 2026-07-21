# Formations Canonical Run View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Replace every direct Formations run-ledger consumer with one authority-gated, sanitized, bounded canonical read model while preserving explicit schema-1 compatibility and keeping schema-2 receipt serving disabled until a real receipt provider is bound.

**Architecture:** `ProjectCanonicalRun(input)` is the only semantic reducer. It produces a private validated `CanonicalRunProjection`; `ProjectRunView(projection)` and `ProjectRunEventPage(projection, since, limit)` are pure selectors. Store adapters obtain one immutable authority read, project once, and hand selectors to API, Archon, Comms, and the dashboard controller. Command receipts are a separate terminal-command output and are never inferred from status or events.

**Tech Stack:** Go 1.23 (`internal/formations`, `internal/api`, `internal/comms`, `cmd/archon`), React 19 and TypeScript 5, Vitest, Playwright, Go race detector, existing descriptor-relative Linux file APIs.

## Global Constraints

- Work only in `/srv/chrote-worktrees/formations-run-view`. Do not deploy, restart a service, alter a tmux session, push, or merge while executing this plan.
- The certified Tool candidate base is exactly `884deeec2c4d4ec2e220b7450dccdd6a10238ef5`. Before Task 1, require `git merge-base --is-ancestor 884deeec2c4d4ec2e220b7450dccdd6a10238ef5 HEAD` and record both that certified base and the pre-implementation plan HEAD. Whole-slice review always diffs from the certified base; implementation-only evidence may additionally diff from the recorded plan HEAD.
- The approved contract is `docs/superpowers/specs/2026-07-20-formations-canonical-run-view-design.md`. If implementation pressure reveals a contract conflict, stop and amend/re-review that document before coding through the conflict.
- Implement children in dependency order: `ctx-7i1.1`; `ctx-7i1.2`; `ctx-7i1.3`; `ctx-7i1.4`; `ctx-7i1.5` and `ctx-7i1.6`; then `ctx-7i1.7`. `.6` is independent of `.5`; do not serialize them for a nonexistent dependency.
- Every child uses RED-first development: dispatch a fresh implementer, commit tests while RED, obtain independent RED review, implement GREEN, run focused and race gates, then obtain independent GREEN review. A reviewer never reviews their own work.
- `ProjectCanonicalRun(CanonicalRunReadInput) (CanonicalRunProjection, error)` is the only semantic reducer. Selectors cannot re-read files, reinterpret event meaning, or rebuild state.
- `RunView` has no event-history member. Event history is available only through bounded `RunEventPage` values.
- Run listing is one `formations.run-list.v1` page: ascending canonical run ID, exclusive last-scanned `after`, exactly 50 default/maximum scanned candidates, and a complete encoded-page cap of 4 MiB. Filtering consumes selected slots and advances the cursor. No layer collects every run identity or immutable run input.
- A selected list candidate with a guarded schema-2 claim and non-authorizing post-writer/pre-activation capability fails the whole page as HTTP 503 `RUNTIME_AUTHORITY_NON_AUTHORIZING`. Never skip/filter/fallback or return candidates projected before that failure.
- `since` is the last-consumed cursor and therefore an exclusive lower bound: eligible events satisfy `seq > since`. Accept `0..9007199254740991`. API requests without `limit` use `200`; explicit limits outside `1..200` fail. The limit counts canonical slots scanned, including omitted projection-only slots. The complete encoded `RunEventPage`, including mandatory run-incarnation generation and source, is at most 1 MiB.
- Before either source returns a ledger document, enforce the code-owned `RunLedgerReadMaximumBytes = 64 << 20` aggregate raw-ledger ceiling through its descriptor/no-follow reader. This 64 MiB implementation limit is separate from the 1 MiB public event-page transfer cap; an over-limit read yields no partial input or projection.
- Sanitization is projection-time allowlisting. Unknown or unsafe event types and fields fail closed unless the design explicitly marks them projection-only. Never recursively copy raw maps.
- Every page/view source equality guard compares event schema, compatibility, and optional authority-schema presence/value. It never relies on Go pointer identity or JSON member order. Page generation compares the exact lowercase 64-hex value.
- The design appendix's public HTTP/code matrix is closed. `writeFormationsError`, run-room parsing, artifact paths, and pre-header SSE failures must use those exact statuses/codes and fixed safe messages; no adapter invents another mapping.
- Schema 2 remains non-authorizing, and this branch must leave `RuntimeAuthorityCapability.SemanticProjection` false. A later integration may enable it only after the complete guarded `ctx-ug7.6.1` provider binds this exact projector and all other required authority checks pass. Never use schema 1 as a fallback after a schema-2 claim.
- Recovery is source-scoped: schema 1 always projects `recoveryState:"live"` with `reconcileCondition:null`; only schema 2 evaluates pending-redaction and interrupted-finalization. A deciding schema-2 `slot_result` is the first non-`ok` result or the `ok` result that completes the frozen current Formation schedule.
- Schema-1 compatibility is the minimum exact frozen 21-row registry. Add no compatibility event or unrelated legacy hardening; `ctx-4dr9` owns its later deletion.
- A `RunCommandReceipt` comes only from a terminal command receipt provider. Schema-1 mutations return an explicitly labeled compatibility result and must never manufacture or imply a receipt.
- Receipt projection exact-binds the durable record to private `SubmittedCommandIdentity{commandId, normalized commandKind, canonical commandPayloadSha256}` constructed at the normalized submission boundary. Wrong id, kind, or hash is never a receipt.
- Artifact serving is by stable `(runId, artifactId)` identity through the canonical verified-artifact seam. `SafeArtifactRef.ref` is the sole contract-sanctioned root-relative path, resolved only beneath `rootId` with no-follow; it is not a generic JSON member named `path`. Never expose any other stored path and never reopen a pathname after validation.
- The compiled artifact fixture's only shutdown surface is exact nonce-protected `POST /api/__formations_contract/shutdown`. Both normal production-binary invocations must return the existing API-fallback 404 for that exact method/path; no production route is added.
- The no-fallback transport proof requires both observations over one unchanged generated tree: the tagged read-only test probe must directly accept it with nil guard error, ledger counts `0/1`, and every capability false including `SemanticProjection:false`; then the separate normal binary must return same-run 503 with no schema-1 compatibility data. The production API does not expose the internal guard reason.
- Do not add visual redesign, new orchestration behavior, migrations, feature flags, speculative abstractions, or an ADR. A new ADR for the deliberate Archon page-NDJSON break is explicitly declined; the approved design, plan, owner ruling, and tests are its record and enforcement.
- The work is complete only when all seven children are independently reviewed and closed with evidence, the umbrella is closed after them, every repository gate passes, and no legacy run-ledger consumer remains outside the canonical implementation and its tests.

## Intended file structure

### Canonical kernel and authority

- Create `src/internal/formations/run_projection.go`: public contract structs, private validated projection, the sole reducer, structural selector, schema-1 compatibility adapters, and store read adapters.
- Create `src/internal/formations/run_projection_events.go`: event allowlist/sanitizers and the bounded page selector.
- Create `src/internal/formations/run_projection_artifacts.go`: verified artifact identity/read handle and artifact hydration.
- Create `src/internal/formations/run_projection_test.go`: reducer, selector, cursor, byte-cap, and compatibility tests.
- Create `src/internal/formations/run_projection_security_test.go`: authority, allowlist, privacy, artifact race, and fail-closed tests.
- Modify `src/internal/formations/store.go`, `runtime_authority.go`, `authority_guard.go`, `run_ledger.go`, `run_artifacts.go`, and `run_escalation.go` only where necessary to route through the new kernel.
- Modify focused existing tests beside those files to assert schema-1 compatibility and schema-2 non-authorization.

### HTTP API

- Create `src/internal/api/formations_run_projection_test.go`: list/detail/mutation/escalation contract tests.
- Create `src/internal/api/formations_run_events_test.go`: bounded page, SSE snapshot, cursor, artifact-open, and error tests.
- Modify `src/internal/api/formations.go`, `formations_test.go`, `formations_acceptance_test.go`, and `formations_runtime_authority_test.go`.
- Modify `dashboard/tests/contract/built-server.spec.ts` and `scripts/test-built-server-contract.sh`; add only temporary-root run fixture files required by the built-server test.
- Create exact immutable production-lane fixture sources `dashboard/tests/contract/fixtures/run-view-schema1.ndjson`, `run-view-schema1.snapshot.toml`, and `run-view-schema1.bindings.toml`. The script copies them into `$artifact_root/workspace/.formations/runs/ci-contract/` as `run_01J9C100000000000000000000.ndjson` plus matching `.snapshot.toml` and `.bindings.toml`. Schema 1 cannot authorize an artifact identity, so this fixture contains no artifact-success file or claim. It never points a test at a repository or live data root.
- Create exact no-fallback schema-1 fixture sources `dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.ndjson`, `run-view-schema2-claim-schema1-fallback.snapshot.toml`, and `run-view-schema2-claim-schema1-fallback.bindings.toml`. A second isolated normal-server invocation copies them as `run_01KXNP6VY3227H78329V52CKF8.ndjson` plus matching snapshots, then creates a guarded schema-2 claim for that same run ID under its separate temporary authority root.
- Create `src/internal/formations/authority_guard_contract_test.go`: build-tagged, compiled-test-only, read-only acceptance probe for the exact script-generated no-fallback authority root and configured workspace. It calls the real guard directly and contributes no production route or capability.
- Create `src/internal/api/formations_run_contract_server_test.go`: compiled-test-only HTTP artifact fixture using the real Formations artifact handler with injected in-memory canonical-reader and verified-artifact-opener seams. It is absent from production builds and cannot read an authority root.

### Archon and Comms

- Create `src/cmd/archon/run_projection_test.go` and modify `src/cmd/archon/main.go` plus its focused tests.
- Create `src/internal/comms/run_projection_test.go` and modify `src/internal/comms/projection.go` plus `projection_test.go`.
- Modify `src/internal/api/comms.go` and its focused tests for strict run-room query parsing while preserving the non-run parser contract.

### Dashboard

- Modify `dashboard/src/components/formationsTypes.ts` and `formationsApi.ts` for exact wire contracts.
- Create `dashboard/src/components/formationsRunController.tsx` and `formationsRunController.test.tsx` for the single polling/cursor/action owner.
- Modify `dashboard/src/App.tsx`, `components/FormationsCockpit.tsx`, `components/AgentsView.tsx`, `components/formationsRunState.ts`, and their focused tests to consume the controller.
- Modify `dashboard/tests/mock-api.ts`, `dashboard/tests/formations/formations-cockpit.spec.ts`, and `formations-smoke.spec.ts` for the public contract.

## Contract-to-task coverage

The approved design retains four SDD milestones. The seven Beads children are the implementation/review packets inside those milestones, not a replacement seven-milestone model:

| Approved milestone | Child packets | Milestone exit |
| --- | --- | --- |
| M1 — pure projector plus schema-1 parity | `ctx-7i1.1` | projector, receipt projector, source selection, and bounded event page independently GREEN-reviewed |
| M2 — authority/security/artifact authorization | `ctx-7i1.2` | fail-closed authority, privacy, recovery, action, session, and artifact tests independently GREEN-reviewed |
| M3 — consumer migration | `ctx-7i1.3`, `.4`, `.5`, `.6` | API structural/receipt adapter, API events/SSE/artifacts, Archon, and Comms all independently GREEN-reviewed; `.5` and `.6` remain siblings |
| M4 — shared dashboard controller | `ctx-7i1.7` | canonical controller and unchanged presentation independently GREEN-reviewed |

The final whole-branch review occurs only after M1–M4 have exited. Do not call a child closure a replacement milestone.

| Design requirement | Owning child | Primary proof |
| --- | --- | --- |
| Sole reducer and structural `RunView` | `ctx-7i1.1` | reducer/selector tests and duplicate-reducer scan |
| Bounded `RunEventPage` | `ctx-7i1.1` | generation/source identity, cursor, scan, limit, 1 MiB tests |
| Closed 41-type event union and schema-1 parity | `ctx-7i1.1`, `ctx-7i1.2` | 21 schema-1 and 37 schema-2 exact fixtures, including conditional start roots and source-selected open dispatches |
| Authority, privacy, sessions, source-scoped recovery, ledger bytes, artifacts | `ctx-7i1.2` | adversarial security, 64 MiB reader-boundary, and recovery tests |
| Bounded run list/detail/status/receipts/escalations API | `ctx-7i1.3` | identity-page, 4 MiB, whole-page non-authorizing 503, provenance, tagged guard acceptance plus same-run schema-2 no-fallback, and HTTP contract tests |
| Events, SSE, artifact-open API | `ctx-7i1.4` | HTTP/SSE/descriptor plus TestMain-contained compiled-server tests |
| Archon canonical consumer | `ctx-7i1.5` | CLI contract and no-raw-read tests |
| Comms canonical consumer | `ctx-7i1.6` | room/message projection tests |
| Shared bounded/coherent dashboard controller | `ctx-7i1.7` | fixed read/window budgets, atomic rollback, unit/component/Playwright tests |

## Per-child subagent protocol

Run this protocol for every numbered task below. The plan deliberately records Beads commands for the later implementation session; the documentation session that authored this plan must not execute them.

1. Confirm the exact dependency state with `bd show <child> --json`, then claim only that child with `bd update <child> --claim`.
2. Generate the task brief with `/home/perttu/skills/subagent-driven-development/scripts/task-brief docs/superpowers/plans/2026-07-21-formations-canonical-run-view.md <task-number>`. Use the printed per-worktree path, do not run two implementers for the same task concurrently, and pass a distinct report path such as `.superpowers/sdd/task-<task-number>-report.md`; the script's default brief path is stable rather than unique. Do not hand-copy the task into another brief.
3. Dispatch a fresh implementer using `superpowers:subagent-driven-development`. Provide the generated brief path, interfaces already landed by dependencies, exact report path, and no accumulated session narrative. The implementer writes the report and returns the tests-only commit before production edits.
4. Record the pre-task base commit. Package RED evidence with `/home/perttu/skills/subagent-driven-development/scripts/review-package <base> <tests-commit>` and give the independent reviewer the generated brief, implementer report, printed review-package path, failing command/output, and why the failure demonstrates missing production behavior. Fix weak tests and repeat until the reviewer records `APPROVED_RED`.
5. Let the implementer add the smallest production change, run the task's focused gates plus race gate, and commit it separately.
6. Package GREEN evidence with `/home/perttu/skills/subagent-driven-development/scripts/review-package <base> <production-head>`. Give a different independent reviewer the same brief, updated report, printed package, exact test outputs, and concerns. Resolve every Critical/Important finding with one scoped fixer and re-review; record Minor findings for final triage.
7. Attach the report and both independent review results with `bd comments add <child> -f <evidence-file>`. Close only that child with `bd close <child> --reason "Implemented RED-first; focused, race, and independent GREEN review evidence attached."`.
8. Update `.superpowers/sdd/progress.md` with `apply_patch` before starting another child, recording child, base/head commits, `APPROVED_RED`, GREEN review disposition, commands, and unresolved Minor findings. This durable ledger is required for crash-resilient resumption.

## Task 1 (Milestone 1): `ctx-7i1.1` — Pure projector kernel and bounded event page

**Files:**

- Create: `src/internal/formations/run_projection.go`
- Create: `src/internal/formations/run_projection_events.go`
- Create: `src/internal/formations/run_projection_test.go`
- Modify: `src/internal/formations/store.go`
- Modify: `src/internal/formations/run_ledger.go`
- Modify: `src/internal/formations/run_escalation.go`

**Interfaces fixed by this task:**

```go
const (
	RunViewSchema       = "formations.run-view.v1"
	RunEventPageSchema  = "formations.run-events.v1"
	RunListPageSchema   = "formations.run-list.v1"
	RunPageDefaultLimit = 200
	RunPageMaximumLimit = 200
	RunPageMaximumBytes = 1 << 20
	RunListPageLimit    = 50
	RunListMaximumBytes = 4 << 20
	MaxJSONSafeInteger  = uint64(9007199254740991)
)

type CanonicalRunSource string

const (
	CanonicalRunSourceSchema1 CanonicalRunSource = "schema-1"
	CanonicalRunSourceSchema2 CanonicalRunSource = "schema-2"
)

type CanonicalInputRole string

const (
	CanonicalInputRoleSchema1Ledger           CanonicalInputRole = "schema-1-ledger"
	CanonicalInputRoleSchema1GraphSnapshot    CanonicalInputRole = "schema-1-graph-snapshot"
	CanonicalInputRoleSchema1BindingsSnapshot CanonicalInputRole = "schema-1-bindings-snapshot"

	CanonicalInputRoleSchema2WorkspaceRegistry  CanonicalInputRole = "schema-2-workspace-registry"
	CanonicalInputRoleSchema2WorkspaceBootstrap CanonicalInputRole = "schema-2-workspace-bootstrap"
	CanonicalInputRoleSchema2WorkspaceAuthority CanonicalInputRole = "schema-2-workspace-authority"
	CanonicalInputRoleSchema2AdmissionPolicy    CanonicalInputRole = "schema-2-admission-policy"
	CanonicalInputRoleSchema2RunBootstrap       CanonicalInputRole = "schema-2-run-bootstrap"
	CanonicalInputRoleSchema2GraphSnapshot      CanonicalInputRole = "schema-2-graph-snapshot"
	CanonicalInputRoleSchema2PrivateBindings    CanonicalInputRole = "schema-2-private-bindings"
	CanonicalInputRoleSchema2Ledger             CanonicalInputRole = "schema-2-ledger"
	CanonicalInputRoleSchema2CommandRecord      CanonicalInputRole = "schema-2-command-record"
	CanonicalInputRoleSchema2RunPrivateState    CanonicalInputRole = "schema-2-run-private-state"
)

type CanonicalInputDocument struct {
	Role   CanonicalInputRole
	Bytes  []byte
	SHA256 string
}

type CanonicalRunReadInput struct {
	RunID     string
	Source    CanonicalRunSource
	Documents []CanonicalInputDocument
}

type CanonicalCommandReadInput struct {
	Source    CanonicalRunSource
	Submitted SubmittedCommandIdentity
	Record    []byte
}

type SubmittedCommandIdentity struct {
	CommandID            string
	CommandKind          string
	CommandPayloadSHA256 string
}

type RunIdentityPageRequest struct {
	After string
	Limit int
}

type RunIdentityPage struct {
	RunIDs  []string
	Cursor  string
	HasMore bool
}

type RunListPage struct {
	Schema  string    `json:"schema"`
	Runs    []RunView `json:"runs"`
	Cursor  string    `json:"cursor"`
	HasMore bool      `json:"hasMore"`
}

type RunEventPage struct {
	Schema     string                       `json:"schema"`
	RunID      string                       `json:"runId"`
	Generation string                       `json:"generation"`
	Source     CanonicalRunSourceProjection `json:"source"`
	Cursor     uint64                       `json:"cursor"`
	HasMore    bool                         `json:"hasMore"`
	Events     []SafeRunEvent               `json:"events"`
}

type SafeSchema1OpenDispatch struct {
	DispatchID  string  `json:"dispatchId"`
	NodeID      string  `json:"nodeId"`
	SlotID      string  `json:"slotId"`
	DispatchSeq *uint64 `json:"dispatchSeq,omitempty"`
}

type SafeSchema2OpenDispatch struct {
	DispatchID                string  `json:"dispatchId"`
	TargetLeaseID             string  `json:"targetLeaseId"`
	NodeID                    string  `json:"nodeId"`
	Attempt                   uint64  `json:"attempt"`
	SlotID                    string  `json:"slotId"`
	AgentID                   string  `json:"agentId"`
	BindingID                 string  `json:"bindingId"`
	SessionTargetID           string  `json:"sessionTargetId"`
	TargetFingerprint         string  `json:"targetFingerprint"`
	DispatchSeq               uint64  `json:"dispatchSeq"`
	PeekCapabilityState       string  `json:"peekCapabilityState"`
	LatestCapabilityGeneration string `json:"latestCapabilityGeneration"`
	LatestCapabilityIssuedSeq uint64  `json:"latestCapabilityIssuedSeq"`
	LatestSteeringGeneration  string  `json:"latestSteeringGeneration"`
	OpenSteeringStartedSeq    *uint64 `json:"openSteeringStartedSeq,omitempty"`
	PeekCapabilityRevokedSeq  *uint64 `json:"peekCapabilityRevokedSeq,omitempty"`
	InterruptState            string  `json:"interruptState"`
	InterruptRequestedSeq     *uint64 `json:"interruptRequestedSeq,omitempty"`
	InterruptOutcomeSeq       *uint64 `json:"interruptOutcomeSeq,omitempty"`
}

type SafeOpenDispatch interface { isSafeOpenDispatch() }

type CanonicalRunAuthorityReader interface {
	ReadRun(runID string) (CanonicalRunReadInput, error)
	ListRunIdentities(request RunIdentityPageRequest) (RunIdentityPage, error)
	ReadCommand(submitted SubmittedCommandIdentity) (CanonicalCommandReadInput, error)
}

type CanonicalRunProjection struct {
	view      RunView
	events    []projectedEvent
	latestSeq uint64
}

type projectedEvent struct {
	scanSeq uint64
	omitted bool
	safe    SafeRunEvent
}

func ProjectCanonicalRun(input CanonicalRunReadInput) (CanonicalRunProjection, error)
func ProjectRunView(projection CanonicalRunProjection) RunView
func ProjectRunEventPage(projection CanonicalRunProjection, since uint64, limit int) (RunEventPage, error)
func ProjectCommandReceipt(input CanonicalCommandReadInput) (RunCommandReceipt, error)

func (s *Store) ReadCanonicalRun(runID string) (CanonicalRunProjection, error)
func (s *Store) ReadRunView(runID string) (RunView, error)
func (s *Store) ReadRunEventPage(runID string, since uint64, limit int) (RunEventPage, error)
func (s *Store) ListRunViews(filter RunListFilter, after string, limit int) (RunListPage, error)
```

`ListRunIdentities` returns ascending canonical run IDs after the exclusive
`After` value. The only accepted limit is `1..50`; public absence is normalized
to 50 before this seam. The production enumerator scans directory entries while
retaining only the lexicographically next 51 validated IDs, then calls the
existing unique-run resolver for each selected ID. It does not retain every ID
or return any ledger/document bytes. `ListRunViews` reads and projects selected
IDs one at a time, applies `RunListFilter` after selection, counts filtered IDs
as scanned, and encodes each complete candidate `RunListPage` before accepting
it. An overflow candidate is not consumed; an oversized singleton returns the
typed resource-limit error. Projection precedes filtering: a selected
non-authorizing schema-2 claim aborts the complete page with the fixed authority
error, even if the filter would exclude it, and no earlier candidate is
published.

The input role/cardinality validator is part of `ProjectCanonicalRun` and runs before any semantic decoder:

| Source | Role | Cardinality and identity rule |
| --- | --- | --- |
| schema 1 | ledger | exactly one complete schema-1 run ledger |
| schema 1 | graph snapshot | exactly one existing `<runId>.snapshot.toml` board snapshot |
| schema 1 | bindings snapshot | exactly one existing `<runId>.bindings.toml` snapshot, including an empty binding set |
| schema 2 | workspace registry | exactly one guarded `registry.private.json` generation |
| schema 2 | workspace bootstrap | exactly one selected `workspace.bootstrap.json` |
| schema 2 | workspace authority | exactly one selected current `workspace.private.json` generation |
| schema 2 | admission policy | one or more; the complete unique revision/hash chain required by current authority and every run event |
| schema 2 | run bootstrap | exactly one selected `run.bootstrap.json` |
| schema 2 | graph snapshot | exactly one hash-selected `graph.snapshot.toml`; its frozen `authoredConfigManifest` is embedded here, so there is no invented separate manifest role |
| schema 2 | private bindings | exactly one hash-selected `bindings.private.toml` |
| schema 2 | ledger | exactly one complete `events.ndjson` immutable snapshot |
| schema 2 | command record | one or more; exactly the unique complete command records referenced by the run ledger, including the admission command |
| schema 2 | run private state | zero or more; the complete unique registered records selected under the frozen run-private `refs/` authority |

Schema 1 rejects every schema-2 role; schema 2 rejects every schema-1 role. Both reject an unknown role, a duplicate singleton, a missing required role, duplicate revision/command/private-state identity, an unreferenced extra policy/command/private-state record, an incomplete referenced set, a SHA-256 mismatch, or a mutable byte slice after reader return. `CanonicalInputRoleSchema2RunPrivateState` is the fixture seam for the already-frozen run-private `refs/` family; it does not define a new authority document. Because its complete production provider is owned by independent `ctx-ug7.6.1`, this branch can certify fixture inputs but keeps the production schema-2 reader unavailable and `SemanticProjection:false`.

The reader performs physical/integrity checks and returns defensive copies. One `ReadRun` invocation may perform many bounded descriptor-relative OS reads to construct that one immutable snapshot; “one read” below always means one reader invocation, never one syscall.

Keep `CanonicalRunProjection` fields private. Consumers may pass the opaque value to selectors but cannot make a second semantic reducer from its contents. Use a `[]projectedEvent` internally if additional scan metadata is needed; do not publish raw ledger fields.

### Step 1.1: Write reducer and selector contract tests

- [ ] Add table-driven fixtures for every structural reducer transition named in the design: start/activate, node waiting/start/terminal, gate open/verdict, escalation open/resolve, run blocked/resumed/cancelled/failed/succeeded, artifact available/revoked, and audit counts.
- [ ] Assert shuffled or duplicate sequence numbers, sequence zero, a sequence above `MaxJSONSafeInteger`, run-ID mismatch, and a post-terminal mutation all return typed projection errors.
- [ ] Prove `ProjectRunView` is deterministic, returns defensive copies, and has no event-history field in its JSON.
- [ ] Prove `RunView.generation` is the exact immutable-incarnation hash from the design appendix: it is stable across appended ledger events but changes when the schema-1 first event/snapshot/bindings or schema-2 run-authority identity tuple changes.
- [ ] Prove `ProjectRunEventPage` returns ascending safe events, treats `since` as last-consumed, advances across projection-only omissions, reports `hasMore` from the immutable projection, and never mutates its input.
- [ ] Assert every page, including an empty page, carries the exact lowercase 64-hex generation and source classification from its projection and rejects any adapter attempt to substitute generation/source/run identity. The generation exact-equals `ProjectRunView(projection).Generation`.
- [ ] Request a `since` greater than the immutable projection's latest sequence and assert an empty page with `cursor == since` and `hasMore:false`; the selector must not regress to `latestSeq`.
- [ ] Prove `limit` values `0` and `201` fail, `1` and `200` work, and `since > MaxJSONSafeInteger` fails.
- [ ] Build complete `RunEventPage` candidates whose exact JSON encodings are one byte below/at/above the 1 MiB boundary. Assert a nonempty page stops before the next event without consuming it, while one individually oversized safe event returns the typed resource-limit error.
- [ ] Prove `ReadCanonicalRun` invokes `CanonicalRunAuthorityReader.ReadRun` exactly once by injecting a counting reader seam; the reader may perform its required bounded OS reads, while both selectors over the returned projection perform zero reader or filesystem calls.
- [ ] Add exact applied and rejected `CanonicalCommandReadInput` fixtures for start/resume/cancel/verdict. Prove `ProjectCommandReceipt` preserves the frozen two-arm union, accepts a rejected start without a run, rejects `pending`, and rejects any required/forbidden-field mismatch. Add wrong submitted ID, right ID/wrong payload hash, cross-kind, stale/pending, substituted-record, and rejected-start fixtures; no mismatch returns a receipt.
- [ ] Add schema-1/schema-2 source-selection fixtures. A claimed-but-invalid schema-2 input fails without consulting schema 1. Assert `SemanticProjection` remains false even when projector fixtures pass.
- [ ] Add a table with all 21 schema-1 constants. Each fixture proves its exact safe allowlist and exact private omission set from the design appendix, and unknown envelope/data keys or types reject the whole projection. Include readable but non-authorizing `verification_verdict` and the three other compatibility-only arms. The `slot_result` fixture must prove `sentinel.artifact` is omitted and the public sentinel is exactly `{runId,status}`.
- [ ] Add separate `run_started` parity fixtures for the current Mission-root writer and the exact current `startFormationRun` writer. The Mission fixture rejects either `mode` or `formationId` and derives `{kind:"mission",nodeId:missionId}`. The isolated-Formation fixture requires and publicly preserves `mode:"formation"` plus nonempty safe `formationId`, derives `{kind:"formation",nodeId:formationId}`, and rejects missing/empty/mismatched discriminants or any unknown key. Do not weaken the registry-wide unknown-key rule.
- [ ] Add schema-1 `run_blocked` fixtures for both current `SafeSchema1OpenDispatch` forms: three required ids only, and the same ids plus present `dispatchSeq:0`. Prove required-id/grammar validation, optional JSON-safe sequence including zero, source-order preservation, unknown nested-key rejection, and duplicate-`dispatchId` rejection. Project the matching `run_resumed` raw carry and require its public `openDispatches` JSON bytes to equal the blocked event's public array bytes, including order and optional-member presence.
- [ ] Add schema-2 `SafeSchema2OpenDispatch` parity from the frozen anchor and prove source selection: a schema-1 dispatch never acquires target lease, capability, steering, interrupt, attempt, agent, binding, target fingerprint, or session-target members; a schema-2 dispatch never decodes through the schema-1 arm.
- [ ] Assert `SafeRunEvent` JSON admits the exact 41 type discriminants: 37 schema-2 plus four schema-1-only. The 17 shared literals use source-selected schema-specific payload structs. It must not expose a raw map fallback.

Use focused tests with assertions shaped like these:

```go
func TestProjectRunEventPageCountsProjectionOnlySlot(t *testing.T) {
	projection := mustProjectFixture(t,
		event(1, "run_started"),
		projectionOnlyEvent(2, "test_projection_only_redacted"),
		event(3, "node_started"),
	)

	page, err := ProjectRunEventPage(projection, 1, 1)
	if err != nil { t.Fatal(err) }
	if page.Schema != RunEventPageSchema { t.Fatalf("schema = %q", page.Schema) }
	if page.Generation != ProjectRunView(projection).Generation { t.Fatalf("generation = %q", page.Generation) }
	if page.Cursor != 2 { t.Fatalf("cursor = %d, want 2", page.Cursor) }
	if !page.HasMore { t.Fatal("hasMore = false, want true") }
	if len(page.Events) != 0 { t.Fatalf("events = %v, want none", page.Events) }
}

func TestProjectRunViewDoesNotEmbedHistory(t *testing.T) {
	view := ProjectRunView(mustProjectFixture(t, event(1, "run_started")))
	raw, err := json.Marshal(view)
	if err != nil { t.Fatal(err) }
	if bytes.Contains(raw, []byte(`"events"`)) {
		t.Fatalf("RunView embeds event history: %s", raw)
	}
}
```

Run:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
go test ./internal/formations -run 'TestProjectCanonicalRun|TestProjectRunView|TestProjectRunEventPage|TestProjectCommandReceipt|TestCanonicalRunSourceSelection|TestStoreReadCanonicalRun' -count=1
```

Expected RED: compile failures for the new types/functions, followed by behavioral failures as the surface is introduced. Commit only the tests:

```bash
git add src/internal/formations/run_projection_test.go
git commit -m "test(formations): specify canonical run projection"
```

Obtain `APPROVED_RED` using the per-child protocol before adding production code.

### Step 1.2: Implement the one semantic reducer and pure selectors

- [ ] Define the wire structs exactly from the approved design appendix: `RunView`, every nested projection/ref/state type, source-selected `SafeSchema1OpenDispatch`/`SafeSchema2OpenDispatch`, `RunListPage`, `RunEventPage` with mandatory generation and source, the 41-type `SafeRunEvent` with source-selected payload variants (including the conditional schema-1 `run_started` union), and the exact two-arm `RunCommandReceipt`. Go JSON tags and TypeScript names are fixed there; no implementation-local field is added.
- [ ] Preserve JSON-safe sequence values as `uint64` in Go but validate before projection and serialize them as ordinary JSON numbers only after validation.
- [ ] Sort and validate the immutable event input once. Reduce every semantic field in the single `ProjectCanonicalRun` switch. Materialize the final structural view and sanitized event stream into the private result.
- [ ] During whole-projection validation, encode every emitted safe event in a singleton complete `RunEventPage` with its real run ID/generation/source/cursor and `hasMore:false`. Reject any singleton over 1 MiB before returning `CanonicalRunProjection`. This is the one no-partial-response guarantee used by every adapter.
- [ ] Make `ProjectRunView` a defensive-copy selector only. It must contain no switch on event type and no file access.
- [ ] Make `ProjectRunEventPage` a cursor/limit/encoded-byte selector only. It must contain no semantic event interpretation.
- [ ] Count the exact `encoding/json` bytes of the complete candidate `RunEventPage`, including schema, run ID, generation, source, cursor, `hasMore`, and events; do not estimate from raw records or encode only the array. Reject an individual safe event that cannot fit in an otherwise empty complete page.
- [ ] Set `cursor` to the greatest canonical sequence scanned, including projection-only omissions. Stop after `limit` canonical candidates, not `limit` emitted events. If the page stops before a candidate for byte reasons, do not consume that candidate sequence. Set `hasMore` from remaining canonical sequences.
- [ ] In the schema-1 reducer, decode `run_started` through the exact Mission/isolated-Formation conditional union before deriving `RunIdentity.runRoot`. Decode `run_blocked`/`run_resumed` open dispatches only as `SafeSchema1OpenDispatch`, preserving semantic source order and optional-member presence. Reject duplicate dispatch ids and require resumed public bytes to carry the prior blocked array unchanged. Schema 2 uses only `SafeSchema2OpenDispatch` and its frozen ordering/invariants.
- [ ] Implement `CanonicalRunAuthorityReader` as the immutable-read seam. Every `CanonicalInputDocument` owns a copied byte slice, has a closed role, and carries its verified SHA-256; the reader performs physical/integrity checks, while the sole reducer performs semantic decoding. `ReadCanonicalRun` invokes the reader then `ProjectCanonicalRun` exactly once. Define and fixture-test bounded identity paging here, but leave replacement of the current production all-ID enumerator to Task 3's API/list migration; no Task 1 adapter may expose the old unbounded result. The pure reducer has no clock input.
- [ ] Re-implement `ProjectRun`, `ListRuns`, `ProjectRunNodeReport`, and `ProjectOpenEscalations` as narrow schema-1 compatibility adapters over `RunView`/`RunListPage`. Do not leave their old event reducers or all-run accumulators in place.
- [ ] Implement `ProjectCommandReceipt` as a separate pure terminal-record decoder. It has no run read, never accepts `pending`, shares no status inference with `ProjectCanonicalRun`, and compares durable ID, normalized kind, and canonical payload SHA-256 to `SubmittedCommandIdentity` before constructing either union arm.

Implement the page loop in this exact order: validate `since` and `limit`; initialize schema/run id/generation/source/cursor=`since`/`hasMore` from the projection; skip stored candidates with `scanSeq <= since` without counting them; before each eligible candidate stop if `scanned == limit`; form a defensive-copy candidate event list (unchanged for an omitted event); form the complete candidate page with cursor at that candidate and `hasMore` based on any later canonical sequence; encode that complete page; on overflow return the typed resource-limit error only when the candidate is a safe event and the accepted event list is empty, otherwise stop without accepting/counting/advancing the candidate; on success accept the list/cursor and increment `scanned` even when omitted; finally return the last accepted complete page. Do not default `limit == 0` inside the selector; the HTTP parser owns the absent-query default and passes `200` explicitly.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/formations/run_projection.go internal/formations/run_projection_events.go internal/formations/run_projection_test.go internal/formations/store.go internal/formations/run_ledger.go internal/formations/run_escalation.go
go test ./internal/formations -run 'TestProjectCanonicalRun|TestProjectRunView|TestProjectRunEventPage|TestProjectCommandReceipt|TestCanonicalRunSourceSelection|TestStoreReadCanonicalRun|TestProjectRun|TestProjectOpenEscalations' -count=1
go test -race ./internal/formations -run 'TestProjectCanonicalRun|TestProjectRunEventPage|TestProjectCommandReceipt|TestStoreReadCanonicalRun' -count=1
```

Expected GREEN: all focused and race tests pass with no skips. Commit production separately:

```bash
git add src/internal/formations/run_projection.go src/internal/formations/run_projection_events.go src/internal/formations/run_projection_test.go src/internal/formations/store.go src/internal/formations/run_ledger.go src/internal/formations/run_escalation.go
git commit -m "feat(formations): add canonical run projection kernel"
```

Before closing `.1`, run and attach this duplicate-reducer scan. Every hit outside the canonical reducer, compatibility adapters, and tests must be explained or removed:

```bash
cd /srv/chrote-worktrees/formations-run-view
rg -n 'ReadRunEvents|ProjectRun\(|ProjectOpenEscalations|case "(run_|node_|gate_|artifact_|escalation_)' src/internal/formations
```

## Task 2 (Milestone 2): `ctx-7i1.2` — Authority, sanitization, sessions, actions, recovery, and artifact open

**Depends on:** `ctx-7i1.1`

**Files:**

- Create: `src/internal/formations/run_projection_artifacts.go`
- Create: `src/internal/formations/run_projection_security_test.go`
- Modify after Task 1: `src/internal/formations/run_projection.go`
- Modify after Task 1: `src/internal/formations/run_projection_events.go`
- Modify: `src/internal/formations/run_artifacts.go`
- Modify: `src/internal/formations/runtime_authority.go`
- Modify: `src/internal/formations/authority_guard.go`
- Modify: `src/internal/formations/authority_guard_test.go`
- Modify: `src/internal/formations/run_artifact_security_test.go`

**Interfaces fixed by this task:**

```go
const (
	RunLedgerReadMaximumBytes   = uint64(64 << 20)
	RunArtifactOpenMaximumBytes = uint64(64 << 20)
)

type VerifiedRunArtifact struct {
	RunID       string
	ArtifactID  string
	Name        string
	MediaType   string
	SizeBytes   uint64
	SHA256      string
	Bytes       []byte
}

func (s *Store) OpenVerifiedRunArtifact(runID, artifactID string) (VerifiedRunArtifact, error)
```

`RunLedgerReadMaximumBytes` is an aggregate raw-ledger implementation ceiling
shared by the schema-1 and schema-2 readers. It is enforced before the ledger
document enters `CanonicalRunReadInput`; it is not a public transfer contract or
an event-page bound.

`VerifiedRunArtifact` is safe response material, not a path container. It must not expose `rootId`, `ref`, a file descriptor, socket data, or a private authority locator.

### Step 2.1: Write adversarial authority and privacy tests

- [ ] Construct schema-2 authority fixtures for every required private input role. Prove projector-readiness checks can pass while the public capability and runtime store remain non-authorizing with `SemanticProjection:false`.
- [ ] Prove a schema-2 claim with a missing, duplicate, mislinked, stale-fence, oversized, invalid-JSON, or wrong-run input returns the exact typed guard/projection error and never falls back to schema 1.
- [ ] Prove schema-1 is selected only when no canonical schema-2 claim exists, or when the test explicitly constructs the offline compatibility store.
- [ ] Add `TestRunLedgerReadMaximumBytesAtLimit` and `TestRunLedgerReadMaximumBytesOverLimit` as table-driven schema-1/schema-2 reader-boundary tests. A known stat size of exactly `RunLedgerReadMaximumBytes` reaches the bounded reader; `+1` returns `ErrRunProjectionResourceLimit` before allocation or a read. For an unknown size or a file that grows after stat, stream at most `RunLedgerReadMaximumBytes+1`, detect the extra byte, discard the candidate document, and return the same typed error. Use a deterministic chunk-generating reader/temporary descriptor rather than a checked-in 64 MiB fixture or `bytes.Repeat`: this performs one bounded at-limit allocation while isolating the physical byte guard from semantic replay. Separate small valid-ledger fixtures prove both source adapters route through the helper. On every over-limit path assert no `CanonicalInputDocument.Bytes`, no partial `CanonicalRunReadInput`, and a zero/unavailable `CanonicalRunProjection`.
- [ ] For each schema-2 public event decoder, add one accepted exact payload, one unexpected top-level field, one unexpected `data` field, one invalid enum/id/hash, and one over-bound string. Unexpected authority-bearing material must reject the whole projection.
- [ ] Add registered projection-only fixtures and prove they affect only scan count/cursor, never structural fields, actions, artifacts, bindings, or event output.
- [ ] Assert `RunSessionView` JSON omits socket/server routes, `targetKey`, paths, raw session lookup identity, prompt/capture/pane/input bytes, exact history/baseline tokens, and capabilities. Also assert the required hashes, closed states, and opaque `sessionTargetId` remain.
- [ ] Assert actions arise only from their ledgered preconditions. A status label, recovery state, local value, persona, or observed tmux session must not create cancel/resume/verdict/peek.
- [ ] Add a complete final schema-1 `run_succeeded` fixture containing its ordinary legacy `slot_result` but no schema-2-only `formation_result`. Assert exactly `recoveryState:"live"` and `reconcileCondition:null`; schema 1 never evaluates a pending-redaction or interrupted-finalization predicate.
- [ ] Add exact schema-2 deciding-result fixtures: the first non-`ok` `slot_result` without `formation_result` is interrupted; the `ok` result that completes the frozen current Formation schedule without `formation_result` is interrupted; an intermediate `ok` result with later scheduled turns is not deciding and remains live when no other gap exists; and either deciding case with its matching `formation_result` no longer has that gap. Add the other four interrupted-finalization predicates plus an unresolved-redaction fixture. Assert precedence is schema-2 `pending-redaction`, then schema-2 `interrupted-finalization`, then `live`; only non-live schema-2 states get `coordinator-reconcile`; neither state creates an action.
- [ ] Register an artifact as available, cite it from an early event, later revoke/redact/expire it, then assert both structural and historical event occurrences expose only the latest unavailable projection.
- [ ] Assert an available artifact exposes the exact `SafeArtifactRef.ref` member as a validated root-relative value bound to `rootId`, never a generic `path` member. Reject absolute, escaping, symlink-following, or wrong-root refs without weakening the global forbidden-member scan.
- [ ] In the artifact-open fixture, replace/revoke the descriptor after the first projection and after the one allowed open. Assert the second projection rejects and the code never reopens the path. Also cover symlink, non-regular file, media mismatch, size mismatch, hash mismatch, and over-bound bytes.
- [ ] Treat the accepted 64 MiB buffered-open ceiling as an implementation resource bound for this verified-buffer algorithm, not as a frozen artifact contract or existing Files-read policy. Test exact-size success at the ceiling and typed resource-limit rejection above it before allocation/response. A later same-handle streaming design requires design/plan review because it changes the optimistic linearization algorithm.

Use a descriptor-race seam, not timing:

```go
func TestOpenVerifiedRunArtifactRejectsAuthorityChangeWithoutReopen(t *testing.T) {
	store, probe := artifactOpenFixture(t)
	probe.AfterFirstOpen = func() { revokeArtifactAuthority(t, store, "artifact-1") }

	_, err := store.OpenVerifiedRunArtifact("run-1", "artifact-1")
	if !errors.Is(err, ErrRunArtifactAuthorizationChanged) {
		t.Fatalf("error = %v, want authorization changed", err)
	}
	if probe.OpenCount != 1 { t.Fatalf("opens = %d, want 1", probe.OpenCount) }
	if probe.ProjectionCount != 2 { t.Fatalf("projections = %d, want 2", probe.ProjectionCount) }
}

func TestRecoveryStatePendingRedactionWins(t *testing.T) {
	projection := mustProjectPrivateFixture(t, interruptedFinalizationEvents(), unresolvedRedaction())
	view := ProjectRunView(projection)
	if view.RecoveryState != RecoveryPendingRedaction {
		t.Fatalf("recovery = %q", view.RecoveryState)
	}
	if view.ReconcileCondition == nil || view.ReconcileCondition.Kind != "coordinator-reconcile" || view.ReconcileCondition.State != RecoveryPendingRedaction {
		t.Fatalf("condition = %#v", view.ReconcileCondition)
	}
	if slices.Contains(actionKinds(view.Actions), "resume") {
		t.Fatalf("recovery fabricated resume action: %#v", view.Actions)
	}
}
```

Run RED:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
go test ./internal/formations -run 'TestRunProjectionAuthority|TestRunLedgerReadMaximumBytes|TestRunProjectionSanitization|TestRunProjectionSession|TestRunProjectionAction|TestRecoveryState|TestArtifactHydration|TestOpenVerifiedRunArtifact' -count=1
```

Expected RED: the reader lacks the aggregate ledger bound, source-scoped
recovery assertions fail or the projector accepts unsafe/private fixtures, and
the verified-open surface is absent. The generated at-limit stream is test
construction, not a checked-in binary fixture. Commit tests only, then obtain
`APPROVED_RED`:

```bash
git add src/internal/formations/run_projection_security_test.go src/internal/formations/authority_guard_test.go src/internal/formations/run_artifact_security_test.go
git commit -m "test(formations): specify run projection security"
```

### Step 2.2: Bind authority and implement fail-closed sanitization

- [ ] Complete the schema-1 production reader and the injectable schema-2 fixture reader against the closed role/cardinality table. Preserve descriptor-relative/no-follow reads, per-record/per-event 1 MiB guards, and the JSON-safe maximum. The production runtime boundary may inspect/guard a schema-2 claim but must return typed non-authorizing/unavailable instead of constructing a complete schema-2 projection input until `ctx-ug7.6.1` binds the missing provider.
- [ ] Route both ledger roles through one bounded physical reader before constructing `CanonicalInputDocument`. After descriptor-relative no-follow open, reject a known stat size above `RunLedgerReadMaximumBytes` before allocation, then stream no more than `RunLedgerReadMaximumBytes+1` bytes from that same handle to catch unknown size or post-stat growth. On an extra byte, discard the buffer and return `ErrRunProjectionResourceLimit`; never return a partially populated document/input/projection. Keep this aggregate 64 MiB ceiling independent of the existing per-record checks and the public 1 MiB page encoder.
- [ ] Keep `RuntimeAuthorityCapability.SemanticProjection` false in this branch under every fixture, including complete projector fixtures. Expose an internal fixture-testable projector-readiness result, but do not wire it into capability derivation. Only the later integration of the complete `ctx-ug7.6.1` guarded provider plus this exact projector may enable the existing capability.
- [ ] Keep `RequireRuntimeAuthority` typed and non-authorizing. A schema-2 claim failure must surface its exact safe code; it must not call the schema-1 reader.
- [ ] Decode schema-2 event envelopes directly from the reader's immutable `CanonicalInputDocument.Bytes`, with payload retained as `json.RawMessage`. Apply `json.Decoder.DisallowUnknownFields` at both envelope and event-specific payload decode. Never decode schema 2 through legacy `RunEvent.Data map[string]any`, because unknown keys would already be lost or weakly typed.
- [ ] Register projection-only types in a separate table that records their redaction class and permits no semantic reducer callback.
- [ ] Apply field-specific length, enum, identifier, hash, and fixed-template validation before values enter the private projection. Replace raw adapter errors with registered safe codes.
- [ ] Derive sessions and actions from verified canonical binding/occupancy/capability state. Live tmux is not an input to this task.
- [ ] Branch recovery derivation on the selected source. Schema 1 unconditionally sets `RecoveryState=RecoveryLive` and `ReconcileCondition=nil`. Schema 2 alone evaluates incomplete transitions. For a Formation, mark `slot_result` deciding exactly when it is the first non-`ok` result or the `ok` result that completes the frozen current schedule; only then is missing `formation_result` a gap. Evaluate pending redaction last in code but apply it as the final highest-precedence schema-2 override so the precedence is unambiguous and tested.
- [ ] Hydrate every artifact occurrence after reduction from a map keyed by stable `artifactId`; never retain an earlier readable descriptor in an event.

- [ ] Represent schema-2 public sanitizers, schema-1 compatibility sanitizers, and projection-only classifications as three explicit closed registries. The schema-2 registry contains exactly the 37 frozen types and exact per-type public key tables in the design appendix. The schema-1 registry contains the minimum exact 21 current constants with the appendix's exact allowlist/recognized-private omission set; add no compatibility event or unrelated legacy hardening because `ctx-4dr9` owns later deletion. Their public union has 41 discriminants because `orchestration_team`, `peer_plane`, `adapter_send`, and `verification_verdict` are compatibility-only. `verification_verdict` is display evidence only and cannot update Gate/status/action/routing/recovery/receipt state.
- [ ] Projection-only types are accepted only through a closed code-owned registry selected by a supported authority schema; event bytes cannot self-register. Production has no extension entry in this branch. Tests inject the exact test-only entry `test_projection_only_redacted` with omit-all redaction classification from `_test.go`, and production code rejects the reserved `test_` prefix. There is no default sanitizer or name-only exemption. The reducer checks the selected projection-only registry first, the exact public registry second, and otherwise returns the typed unknown-authority-event error.
- [ ] For all sanitizers, use exact typed envelope/data decoders. A recognized-private schema-1 key may be omitted only where the appendix names it. Any other unknown key or type rejects the complete projection. Add 21 schema-1 parity fixtures and 37 schema-2 exact fixtures; no recursive copy, `map[string]any`, or name-only compatibility exemption survives.

### Step 2.3: Implement optimistic artifact opening

- [ ] Project C1 and select the exact available `ArtifactProjection` by `artifactId`; require equal top-level and nested IDs.
- [ ] Treat `SafeArtifactRef.ref` as the one contract-sanctioned root-relative path: resolve it only beneath its `rootId` through the existing descriptor-relative no-follow primitives. It is not a generic public member named `path`. Open its descriptor exactly once. Reject `sizeBytes > RunArtifactOpenMaximumBytes`; validate regular identity, media type, and exact stat size; read at most `sizeBytes+1` bytes from that same handle; require exactly `sizeBytes`; and validate SHA-256 over those bytes.
- [ ] Project C2 from current authority. Require P2 to be field-for-field equal to P1, including the entire `SafeArtifactRef`.
- [ ] Return only the already verified bytes and safe metadata after C2 succeeds. On any error or mismatch, discard the buffer. Never reopen the descriptor and never append an observation from the read path.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/formations/run_projection.go internal/formations/run_projection_events.go internal/formations/run_projection_artifacts.go internal/formations/run_projection_security_test.go internal/formations/run_artifacts.go internal/formations/runtime_authority.go internal/formations/authority_guard.go internal/formations/authority_guard_test.go internal/formations/run_artifact_security_test.go
go test ./internal/formations -run 'TestRunProjectionAuthority|TestRunLedgerReadMaximumBytes|TestRunProjectionSanitization|TestRunProjectionSession|TestRunProjectionAction|TestRecoveryState|TestArtifactHydration|TestOpenVerifiedRunArtifact|TestGuardRuntimeAuthority' -count=1
go test -race ./internal/formations -run 'TestRunProjection|TestRunLedgerReadMaximumBytes|TestRecoveryState|TestOpenVerifiedRunArtifact|TestGuardRuntimeAuthority' -count=1
```

Expected GREEN: all authority, ledger-bound, source-scoped recovery, privacy,
and race tests pass; no over-limit reader returns partial bytes/view and no
schema-2 fixture reaches schema 1. Commit production separately:

```bash
git add src/internal/formations/run_projection.go src/internal/formations/run_projection_events.go src/internal/formations/run_projection_artifacts.go src/internal/formations/run_projection_security_test.go src/internal/formations/run_artifacts.go src/internal/formations/runtime_authority.go src/internal/formations/authority_guard.go src/internal/formations/authority_guard_test.go src/internal/formations/run_artifact_security_test.go
git commit -m "feat(formations): secure canonical run projection"
```

## Task 3 (Milestone 3): `ctx-7i1.3` — Run list, detail, receipt adapter, and escalation API

**Depends on:** `ctx-7i1.2`

**Files:**

- Create: `src/internal/api/formations_run_projection_test.go`
- Modify: `src/internal/api/formations.go`
- Modify: `src/internal/api/formations_test.go`
- Modify: `src/internal/api/formations_acceptance_test.go`
- Modify: `src/internal/api/formations_runtime_authority_test.go`
- Modify after Task 1: `src/internal/formations/run_projection.go`
- Modify after Task 1: `src/internal/formations/run_projection_test.go`
- Modify: `src/internal/formations/run_artifacts.go`
- Modify: `src/internal/formations/store.go`
- Modify: `dashboard/tests/contract/built-server.spec.ts`
- Modify: `scripts/test-built-server-contract.sh`
- Create: `src/internal/formations/authority_guard_contract_test.go`
- Create: `dashboard/tests/contract/fixtures/run-view-schema1.ndjson`
- Create: `dashboard/tests/contract/fixtures/run-view-schema1.snapshot.toml`
- Create: `dashboard/tests/contract/fixtures/run-view-schema1.bindings.toml`
- Create: `dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.ndjson`
- Create: `dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.snapshot.toml`
- Create: `dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.bindings.toml`

**Preserved mutation response shapes:**

```text
schema-1 start:                  data = { runId, status: RunView }
schema-1 resume/cancel/verdict: data = { status: RunView }
schema-2 terminal command:      data = { receipt: RunCommandReceipt }
```

This branch does not invent another public schema or union. Schema-1 compatibility is explicitly and canonically labeled by `status.source.compatibility === true` in the preserved response shape. Schema-2 receipt serving remains unwired until the independent provider binds; its adapter is fixture-tested only.

**Executable normal-server no-fallback fixture:** the contract script first
creates and affirmatively guard-probes the separate tree under
`$artifact_root/no-fallback/`. It then completes the ordinary schema-1
normal-server assertions, stops that disposable process, and starts a second
isolated invocation of the same supplied production binary against the
already-probed no-fallback tree. The tree contains:

```text
workspace/.formations/runs/schema2-claim/run_01KXNP6VY3227H78329V52CKF8.ndjson
workspace/.formations/runs/schema2-claim/run_01KXNP6VY3227H78329V52CKF8.snapshot.toml
workspace/.formations/runs/schema2-claim/run_01KXNP6VY3227H78329V52CKF8.bindings.toml
formations-data/workspaces/registry.private.json
formations-data/workspaces/wsa_01KXNP6VY3227H78329V52CKF8/workspace.bootstrap.json
formations-data/workspaces/wsa_01KXNP6VY3227H78329V52CKF8/workspace.private.json
formations-data/workspaces/wsa_01KXNP6VY3227H78329V52CKF8/admission-policies/1.json
formations-data/workspaces/wsa_01KXNP6VY3227H78329V52CKF8/runs/run_01KXNP6VY3227H78329V52CKF8/events.ndjson
```

The first three are copied from the exact immutable
`run-view-schema2-claim-schema1-fallback.*` sources. The script derives the
authority records exactly as
`bindRuntimeAuthorityFixtureToOpenedWorkspace`/`newRuntimeAuthorityFixture` in
`src/internal/formations/authority_guard_test.go`: open/stat the configured
workspace, encode device and inode as base-10 strings, resolve symlinks, apply
`filepath.Clean` then `filepath.ToSlash` to configured/resolved paths, and hash
the exact UTF-8 bytes of
`{"configuredPath":<Go-%q>,"device":<Go-%q>,"inode":<Go-%q>,"resolvedPath":<Go-%q>}`.
It writes the registry/bootstrap/workspace-authority JSON with the same key
sets, key order, compact encoding, and values as that fixture, using authority id
`wsa_01KXNP6VY3227H78329V52CKF8`, root encoding
`workspace-root-identity-v1`, record/policy revision 1, next writer fence 2,
and next admission sequence 1. The exact no-newline templates are:

```text
registry.private.json = {"registrySchema":1,"recordRev":1,"entries":[{"workspaceAuthorityId":"wsa_01KXNP6VY3227H78329V52CKF8","configuredPath":$configuredPathJSON,"device":$deviceJSON,"inode":$inodeJSON,"workspaceRootIdentitySha256":"$rootHash"}]}
workspace.bootstrap.json = {"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"wsa_01KXNP6VY3227H78329V52CKF8","workspaceRootIdentitySha256":"$rootHash"}
workspace.private.json = {"recordRev":1,"authoritySchema":2,"workspaceAuthorityId":"wsa_01KXNP6VY3227H78329V52CKF8","rootIdentityEncoding":"workspace-root-identity-v1","workspaceRootIdentitySha256":"$rootHash","nextWriterFence":2,"nextAdmissionSeq":1,"admissionPolicyRef":{"policyRev":1,"policySha256":"$policyHash"}}
```

Here `$configuredPathJSON`, `$deviceJSON`, and `$inodeJSON` are the exact Go
`%q` JSON string tokens used in the root-identity computation, not shell-quoted
approximations. Policy bytes are exactly
`{"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}`
with no newline; `policySha256` is the lowercase SHA-256 of those exact bytes.
The guarded ledger bytes are exactly one newline-terminated record:
`{"schema":2,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T00:00:00Z","runId":"run_01KXNP6VY3227H78329V52CKF8","seq":1,"type":"run_started","actor":"agent:contract","data":{}}\n`.
This is a valid guarded schema-2 claim envelope under the current guard fixture
rules; it is intentionally not treated as semantically projectable while the
capability remains disabled.

As the preflight before either normal process, the script must affirm that those
exact generated bytes are guard-accepted rather than merely capable of
producing the same public 503 as a rejection. Create this exact test-only source:

```go
//go:build formations_guard_contract

package formations

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	guardContractDataRootEnv = "CHROTE_FORMATIONS_GUARD_CONTRACT_DATA_ROOT"
	guardContractWorkspaceEnv = "CHROTE_FORMATIONS_GUARD_CONTRACT_WORKSPACE"
	guardContractAcceptedMarker = "FORMATIONS_GUARD_CONTRACT_ACCEPTED schema1Inspection=0 schema2Guarded=1 semanticProjection=false"
)

func TestFormationsRunNoFallbackAuthorityGuardContract(t *testing.T) {
	allowed := map[string]bool{
		guardContractDataRootEnv: true,
		guardContractWorkspaceEnv: true,
	}
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok && strings.HasPrefix(key, "CHROTE_") && !allowed[key] {
			t.Fatalf("unexpected CHROTE environment key %q", key)
		}
	}

	formationsDataRoot := os.Getenv(guardContractDataRootEnv)
	configuredWorkspace := os.Getenv(guardContractWorkspaceEnv)
	if formationsDataRoot == "" || !filepath.IsAbs(formationsDataRoot) || filepath.Clean(formationsDataRoot) != formationsDataRoot {
		t.Fatalf("%s must be a nonempty clean absolute path", guardContractDataRootEnv)
	}
	if configuredWorkspace == "" || !filepath.IsAbs(configuredWorkspace) || filepath.Clean(configuredWorkspace) != configuredWorkspace {
		t.Fatalf("%s must be a nonempty clean absolute path", guardContractWorkspaceEnv)
	}

	before := snapshotRuntimeAuthorityFixture(t, formationsDataRoot, configuredWorkspace)
	result, err := GuardRuntimeWorkspaceAuthorityV1(formationsDataRoot, configuredWorkspace)
	if err != nil {
		t.Fatalf("guard exact no-fallback fixture: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	if result.Ledgers.Schema1Inspection != 0 || result.Ledgers.Schema2Guarded != 1 {
		t.Fatalf("ledger classifications = %+v, want schema1Inspection=0 schema2Guarded=1", result.Ledgers)
	}
	if after := snapshotRuntimeAuthorityFixture(t, formationsDataRoot, configuredWorkspace); !reflect.DeepEqual(after, before) {
		t.Fatalf("guard probe mutated exact no-fallback fixture\nbefore: %#v\nafter:  %#v", before, after)
	}
	t.Log(guardContractAcceptedMarker)
}
```

The script compiles and executes only that opt-in test as a fixture preflight
before starting either normal server:

```bash
guard_contract_dir="$artifact_root/authority-guard-contract"
mkdir -p "$guard_contract_dir/home" "$guard_contract_dir/tmp"
chmod 0700 "$guard_contract_dir/home" "$guard_contract_dir/tmp"
(
  cd "$repo_root/src"
  go test -c -tags formations_guard_contract \
    -o "$guard_contract_dir/formations-authority-guard-contract.test" \
    ./internal/formations
)
env -i \
  PATH="$PATH" \
  HOME="$guard_contract_dir/home" \
  LANG=C.UTF-8 \
  TMPDIR="$guard_contract_dir/tmp" \
  CHROTE_FORMATIONS_GUARD_CONTRACT_DATA_ROOT="$artifact_root/no-fallback/formations-data" \
  CHROTE_FORMATIONS_GUARD_CONTRACT_WORKSPACE="$artifact_root/no-fallback/workspace" \
  "$guard_contract_dir/formations-authority-guard-contract.test" \
    -test.run '^TestFormationsRunNoFallbackAuthorityGuardContract$' \
    -test.count=1 -test.v \
    >"$artifact_root/authority-guard-probe.log" 2>&1
test -s "$artifact_root/authority-guard-probe.log"
grep -Fq \
  'FORMATIONS_GUARD_CONTRACT_ACCEPTED schema1Inspection=0 schema2Guarded=1 semanticProjection=false' \
  "$artifact_root/authority-guard-probe.log"
```

The probe is a non-server compiled test process, so the built contract still has
two server kinds and three server invocations. It runs after the complete
no-fallback tree is written and before any later write to either exact input;
the script performs no such later write. A probe failure aborts before either
normal server starts. It exposes no HTTP route, cannot change
`RuntimeAuthorityCapability`, and does not reveal guard reasons through the
production API.

After that affirmative read-only probe succeeds, the second process uses its
own random port and the same temporary workspace/runtime roots, with
`CHROTE_FORMATIONS_DATA_ROOT` set to the no-fallback `formations-data`
directory. Its exact assertions are:

- `GET /api/formations/runs/run_01KXNP6VY3227H78329V52CKF8` -> HTTP 503 with
  public code `RUNTIME_AUTHORITY_NON_AUTHORIZING`, no `data.status`, no
  `source.compatibility:true`, and no schema-1 fixture member; and
- `GET /api/formations/runs?limit=50` -> the same HTTP 503/code, with no
  `data`, `runs`, cursor, or candidate projected before the claimed run. It
  never returns an empty/filtered compatibility page.

These are the executable claimed-run no-fallback and mixed-window whole-page
fail-loud proofs; neither the guard probe nor a capability endpoint substitutes
for the separate normal-binary assertions. The first and second normal-process
logs are respectively `normal-schema1.log` and
`normal-schema2-no-fallback.log`; the probe evidence is
`authority-guard-probe.log`, all under `$artifact_root`.

**Task 3 script phase boundary:** the Task 3 revision of
`scripts/test-built-server-contract.sh` ends after the guard preflight and the
two normal production-binary invocations. Task 3 requires exactly
`authority-guard-probe.log`, `normal-schema1.log`, and
`normal-schema2-no-fallback.log`; it does not compile
`src/internal/api/formations_run_contract_server_test.go`, set
`CHROTE_FORMATIONS_ARTIFACT_CONTRACT_URL`, start the artifact contract server,
or require `artifact-contract.log`. Task 4 extends this same script with that
third server invocation and its log. Therefore Task 3 GREEN does not depend on
a Task 4 fixture, while the final contract remains two server kinds and three
server invocations plus the non-server guard probe.

### Step 3.1: Write HTTP and terminal-receipt adapter tests

- [ ] Assert `GET /api/formations/runs` returns the existing success envelope with `data` as one `formations.run-list.v1` page, ascending canonical run-id order, no raw statuses, and no event history. Cover absent/explicit `after`, absent/explicit `limit`, duplicate/empty/unknown query keys, filtering after selection, and cursor advancement across filtered candidates.
- [ ] Build one selected list window with a valid schema-1 candidate followed by a guarded schema-2 claim whose post-writer/pre-activation capability is non-authorizing. Assert the complete request fails HTTP 503 `RUNTIME_AUTHORITY_NON_AUTHORIZING` with no `data`/`runs`/cursor, even when the public filter would exclude the claimed candidate. Prove it is never skipped, filtered, downgraded to its schema-1 fallback, or returned as a partial page after the earlier valid candidate.
- [ ] Seed more than 50 identities and prove the reader retains only the next 51 IDs, reads/projects no more than 50 selected candidates one at a time, and returns `hasMore`. Build exact complete-page encodings one byte below/at/above 4 MiB; an overflow candidate is not consumed and an oversized singleton returns the typed 413 resource-limit error.
- [ ] Assert `GET /api/formations/runs/{runId}` preserves the existing success envelope and `data.status` key, with one canonical `RunView` as that value.
- [ ] Assert `GET /api/formations/runs/{runId}/escalations` returns `data.escalations` copied from that same view, with no independent ledger scan.
- [ ] Construct `SubmittedCommandIdentity` at the normalized mutation boundary after aliases/defaults are frozen and before private terminal read. Inject the terminal reader for applied start/resume/cancel/verdict and rejected command fixtures. Assert it receives the exact id/kind/hash and the adapter returns exactly `data.receipt: RunCommandReceipt`; keep that production seam unwired and capability-disabled.
- [ ] Add wrong submitted ID, right ID/wrong hash, cross-kind, stale/pending, substituted-record, duplicate-id/different-payload, and rejected-start fixtures. Each mismatch returns the fixed typed error and no receipt/status/run id.
- [ ] Inject a schema-2 `pending` outcome and assert the endpoint returns the registered typed not-terminal error, not a receipt or status guess.
- [ ] Run each schema-1 mutation and assert the response preserves the current `runId`/`status` keys, replaces the old status value with `RunView`, and has `status.source.compatibility === true`. Assert no `receipt` key is present.
- [ ] Assert a schema-1 error never becomes a rejected receipt. Assert a schema-2 rejected-start receipt has no `runId` or `effectSeq`.
- [ ] Assert unsafe projection errors use `writeFormationsError`, return no partial `RunView`, and preserve the registered HTTP status/code without raw paths or adapter errors.
- [ ] Inject `ErrRunProjectionResourceLimit` from the aggregate ledger reader into list and detail. Assert fixed HTTP 413 `FORMATIONS_RUN_RESOURCE_LIMIT`, no partial list/view, and no raw byte count or path in the error body.
- [ ] Add a spy canonical store to prove list/detail/escalations call `ListRunViews` or `ReadRunView`, never `ProjectRun`, `ReadRunEvents`, or `ProjectOpenEscalations`.
- [ ] In the built-server contract, generate the complete no-fallback tree first, compile `src/internal/formations/authority_guard_contract_test.go` with `-tags formations_guard_contract`, and run only `TestFormationsRunNoFallbackAuthorityGuardContract` under the exact two-variable `env -i` contract above. Require nonempty `authority-guard-probe.log`, its fixed acceptance marker, nil guard error, ledger counts `0/1`, the complete all-false capability including `SemanticProjection:false`, and identical before/after snapshots. After that preflight, prove the ordinary schema-1 list/detail assertions and stop the first normal process. Only then start the second normal process. Assert the same run ID exists in both schema-1 and guarded schema-2 locations; both detail and a list window selecting it return only 503 `RUNTIME_AUTHORITY_NON_AUTHORIZING`, with no compatibility fallback, skipped candidate, empty 200 page, or partial page. The private guard reason remains absent from both HTTP responses.

Representative assertions:

```go
func TestResumeRunSchema1ReturnsCompatibilityNotReceipt(t *testing.T) {
	response := performSchema1Resume(t)
	if response.Data.Receipt != nil { t.Fatalf("unexpected receipt: %#v", response.Data.Receipt) }
	if response.Data.Status.Schema != formations.RunViewSchema {
		t.Fatalf("view schema = %q", response.Data.Status.Schema)
	}
	if !response.Data.Status.Source.Compatibility {
		t.Fatalf("source = %#v, want compatibility", response.Data.Status.Source)
	}
}

func TestStartRunPendingTerminalOutcomeIsNotAReceipt(t *testing.T) {
	response := performSchema2StartWithTerminalFixture(t, "pending")
	if response.Code != http.StatusConflict { t.Fatalf("status = %d", response.Code) }
	if response.Error.Code != "FORMATIONS_COMMAND_NOT_TERMINAL" { t.Fatalf("code = %q", response.Error.Code) }
	if strings.Contains(response.Body.String(), `"receipt"`) { t.Fatalf("pending became receipt: %s", response.Body.String()) }
}
```

Run RED:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/formations/authority_guard_contract_test.go
go test ./internal/api ./internal/formations -run 'Test(Formations.*RunView|ListRunsCanonical|GetRunCanonical|RunEscalationsCanonical|.*Run.*Receipt|.*Run.*Compatibility|.*Run.*Pending)' -count=1
task3_contract_dir="$(mktemp -d "${TMPDIR:-/tmp}/ctx-7i1-task3-red.XXXXXX")"
go build -o "$task3_contract_dir/server" ./cmd/server
cd ..
CHROTE_SERVER_BINARY="$task3_contract_dir/server" CHROTE_CONTRACT_ARTIFACT_DIR="$task3_contract_dir/artifacts" scripts/test-built-server-contract.sh >"$task3_contract_dir/contract.log" 2>&1
```

Expected RED: focused tests and the built-server list/detail assertion fail because old endpoints expose `RunStatusProjection`, direct escalation scans remain, the list is unbounded, and the terminal-receipt adapter is absent. The tagged guard probe itself must already compile, run against the exact script-generated no-fallback roots, and write the affirmative marker; a malformed-fixture probe failure is not an acceptable RED and must be repaired before `APPROVED_RED`. Preserve the exact temporary evidence path/output. Commit tests only and obtain `APPROVED_RED`:

```bash
git add src/internal/api/formations_run_projection_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go src/internal/api/formations_runtime_authority_test.go src/internal/formations/run_projection_test.go src/internal/formations/authority_guard_contract_test.go dashboard/tests/contract/built-server.spec.ts scripts/test-built-server-contract.sh dashboard/tests/contract/fixtures/run-view-schema1.ndjson dashboard/tests/contract/fixtures/run-view-schema1.snapshot.toml dashboard/tests/contract/fixtures/run-view-schema1.bindings.toml dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.ndjson dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.snapshot.toml dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.bindings.toml
git commit -m "test(api): specify canonical run responses"
```

### Step 3.2: Route bounded structural endpoints through the canonical view

- [ ] Add exactly `GET /api/formations/runs`. Accept each optional `board`, `after`, and `limit` value at most once and reject empty/duplicate/unknown query keys. Resolve board with the existing selector resolver. Validate `after` with the canonical run-id grammar. Absent limit becomes 50; explicit limit is digits-only `1..50`. Call `ListRunViews(filter, after, limit)` exactly once and return its one bounded page. An absent board selects all candidate identities but does not remove the page bounds.
- [ ] Replace the existing all-ID map/sort path in `run_artifacts.go` with the bounded next-51 selector. Retain only safe IDs greater than `after`, preserve duplicate detection for any selected ID through `openRunLedger`, and read/project at most one selected run at a time. Add a probe proving retained identity count never exceeds 51 when the directory contains more than 50 candidates.
- [ ] Change detail to `ReadRunView`. Change escalations to select `view.Escalations`. Remove handler-level event reduction.
- [ ] Preserve the repository's `core.WriteSuccess` envelope rather than adding a second response envelope. List uses the `RunListPage` directly as `data`; detail preserves `map[string]any{"status": view}`; escalations preserve `map[string]any{"escalations": view.Escalations}`.
- [ ] Map projection guard, invalid-input, not-found, resource-limit, and authority errors through `writeFormationsError`. `ErrRunProjectionResourceLimit` maps to fixed 413 `FORMATIONS_RUN_RESOURCE_LIMIT`. A non-authorizing selected list candidate aborts the whole page as fixed 503 `RUNTIME_AUTHORITY_NON_AUTHORIZING` before success-envelope publication, regardless of filtering; do not return a partial list if any claimed run fails projection.
- [ ] Build the exact valid no-fallback authority files above and run the tagged read-only guard probe to its fixed acceptance marker as the contract preflight. Then extend the disposable built-server fixture and Playwright contract to cover schema-1 bounded list/detail plus `source.compatibility:true` and exact cursor/order in the first normal process, followed by the second normal process proving that both detail and the list window selecting the same-run schema-2 claim return 503 rather than the co-located schema-1 fallback or an empty/partial page. Both normal invocations return 404 for exact `POST /api/__formations_contract/shutdown` through the existing API fallback and the supplied binary omits the artifact fixture marker. Schema 1 cannot prove artifact success, and neither production-wiring invocation may synthesize artifact identity. The test uses temporary roots and no live provider/service/tmux.

### Step 3.3: Bind terminal receipt input without fabricating it

- [ ] Define a narrow private terminal command reader at the mutation adapter boundary. Its input is exact `SubmittedCommandIdentity`; its output is `CanonicalCommandReadInput` containing that identity and the already durable terminal record. The read is by client-stable id and the decoder exact-matches id, normalized kind, and canonical payload SHA-256.
- [ ] Invoke Task 1's `ProjectCommandReceipt` and wrap the result only as `map[string]any{"receipt": receipt}`. Fixture-test all command kinds, canonical hashes, JSON-safe sequences/fence, required/forbidden fields per union arm, start-only policy reference, rejected start without a run, and every provenance mismatch.
- [ ] Keep the production reader nil/unavailable and schema-2 receipt serving disabled in this branch. Missing provider returns the typed authority-unavailable error; `pending` returns the typed not-terminal error.
- [ ] For schema 1, read a fresh canonical view after mutation and preserve the existing keys: start returns `map[string]any{"runId": view.RunID, "status": view}`; resume/cancel/verdict return `map[string]any{"status": view}`. Assert `view.Source.Compatibility` is true and never add a `receipt` member.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/api/formations.go internal/api/formations_run_projection_test.go internal/api/formations_test.go internal/api/formations_acceptance_test.go internal/api/formations_runtime_authority_test.go internal/formations/authority_guard_contract_test.go internal/formations/run_projection.go internal/formations/run_projection_test.go internal/formations/run_artifacts.go internal/formations/store.go
go test ./internal/api ./internal/formations -run 'Test(Formations.*RunView|ListRunsCanonical|GetRunCanonical|RunEscalationsCanonical|.*Run.*Receipt|.*Run.*Compatibility|.*Run.*Pending)' -count=1
go test -race ./internal/api ./internal/formations -run 'Test(Formations.*Run|.*Run.*Receipt|.*Run.*Compatibility)' -count=1
task3_contract_dir="$(mktemp -d "${TMPDIR:-/tmp}/ctx-7i1-task3-green.XXXXXX")"
go build -o "$task3_contract_dir/server" ./cmd/server
cd ..
CHROTE_SERVER_BINARY="$task3_contract_dir/server" CHROTE_CONTRACT_ARTIFACT_DIR="$task3_contract_dir/artifacts" scripts/test-built-server-contract.sh >"$task3_contract_dir/contract.log" 2>&1
sed -n '1,240p' "$task3_contract_dir/contract.log"
```

Expected GREEN: all canonical endpoint and compatibility-shape tests pass; the
separately compiled read-only guard probe accepts the exact no-fallback fixture
with counts `0/1`, the complete disabled capability, unchanged snapshots, and
its fixed marker; the second normal-server invocation then proves that both
detail and list fail 503 for the same valid schema-2 claim without schema-1
fallback, skip, filter, or partial page; schema-2 adapter fixtures pass; and live
schema-2 receipt serving remains disabled without the independent provider. The
guard probe source remains in the tests-only RED commit; no production guard
code is added for this proof. Re-run the unchanged probe through
`scripts/test-built-server-contract.sh` at GREEN. Commit production separately:

```bash
git add src/internal/api/formations.go src/internal/api/formations_run_projection_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go src/internal/api/formations_runtime_authority_test.go src/internal/formations/run_projection.go src/internal/formations/run_projection_test.go src/internal/formations/run_artifacts.go src/internal/formations/store.go dashboard/tests/contract/built-server.spec.ts scripts/test-built-server-contract.sh dashboard/tests/contract/fixtures/run-view-schema1.ndjson dashboard/tests/contract/fixtures/run-view-schema1.snapshot.toml dashboard/tests/contract/fixtures/run-view-schema1.bindings.toml dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.ndjson dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.snapshot.toml dashboard/tests/contract/fixtures/run-view-schema2-claim-schema1-fallback.bindings.toml
git commit -m "feat(api): serve canonical formations run views"
```

## Task 4 (Milestone 3): `ctx-7i1.4` — Bounded events, replay-only SSE, and artifact-open API

**Depends on:** `ctx-7i1.2`, `ctx-7i1.3`

**Files:**

- Create: `src/internal/api/formations_run_events_test.go`
- Modify: `src/internal/api/formations.go`
- Modify: `src/internal/api/formations_test.go`
- Modify: `src/internal/api/formations_acceptance_test.go`
- Create: `src/internal/api/formations_run_contract_server_test.go`
- Read-only harness dependency: `src/internal/api/tmux_test.go` (`TestMain` owns the one permitted session-bank environment variable; do not modify it)
- Read-only no-fallback dependency: `src/internal/formations/authority_guard_contract_test.go` (Task 3's tagged direct-guard probe; do not duplicate or move it into the API package)
- Modify after Task 1: `src/internal/formations/run_projection.go`
- Modify after Task 1: `src/internal/formations/run_projection_events.go`
- Modify: `dashboard/tests/contract/built-server.spec.ts`
- Modify: `scripts/test-built-server-contract.sh`

**Routes:**

```text
GET /api/formations/runs/{runId}/events?since=<JsonSafeInteger>&limit=<1..200>
GET /api/formations/runs/{runId}/stream
GET /api/formations/runs/{runId}/artifacts/{artifactId}
```

The events endpoint returns one `RunEventPage` with mandatory generation/source inside the existing success envelope. SSE is a finite replay from one immutable `CanonicalRunProjection`, then a cursor control event, then EOF. Artifact open returns only the already verified bytes from `OpenVerifiedRunArtifact`.

**Accepted engineering decision:** preserve `/api/formations/runs/{runId}/stream` and add exactly `GET /api/formations/runs/{runId}/artifacts/{artifactId}`. Every artifact response path sets `Cache-Control: no-store`. Success also sets `X-Content-Type-Options: nosniff`, the second verified projection's `Content-Type`, and exact `Content-Length`; it sends no `Content-Disposition`, ETag, Last-Modified, or other validator. The Task 4 RED review must verify this sealed shape; it is no longer an open route/header choice.

**Two-server-kind built-contract lane:** the normally built server remains the
only production-wiring subject and is invoked twice. Its first temporary
schema-1 process proves bounded list/detail/events, finite SSE, Comms, and
assets. After it exits, the second isolated process runs Task 3's exact
same-run schema-1-fallback/schema-2-claim fixture and proves 503
non-authorization with `SemanticProjection:false`. Schema 1 has no artifact
registration or public artifact identity, so neither normal process can
truthfully prove artifact success or mint an identity for it.

Task 4 starts from Task 3's already-GREEN script lane: the direct guard
preflight plus the two normal invocations and their three evidence logs. It
must preserve those assertions and extends the script only with the compiled
artifact-contract server, `CHROTE_FORMATIONS_ARTIFACT_CONTRACT_URL`, and
`artifact-contract.log` described below.

Artifact success is instead proved through the real HTTP handler in a compiled
test-only `src/internal/api` server:

- `formations_run_contract_server_test.go` defines
  `TestFormationsRunArtifactContractServer`, an in-memory implementation of the
  canonical reader and verified-artifact opener, and a fixture marker. It
  registers the production Formations artifact handler; it does not duplicate
  route or header logic.
- `scripts/test-built-server-contract.sh` runs
  with `contract_dir="$artifact_root/artifact-contract"`, then runs
  `go test -c -o "$contract_dir/formations-api-contract.test" ./internal/api`
  from `src`; creates `$contract_dir/home` and `$contract_dir/tmp` with mode
  0700; and launches that binary exactly under `env -i` with `PATH`,
  `HOME=$contract_dir/home`, `LANG=C.UTF-8`, `TMPDIR=$contract_dir/tmp`,
  `CHROTE_FORMATIONS_CONTRACT_LISTEN=127.0.0.1:<artifact-random-port>`, and
  `CHROTE_FORMATIONS_CONTRACT_SHUTDOWN_NONCE=<per-run-nonce>`, followed by
  `-test.run '^TestFormationsRunArtifactContractServer$' -test.count=1`.
  It passes the corresponding base URL to Playwright only as
  `CHROTE_FORMATIONS_ARTIFACT_CONTRACT_URL`. `CHROTE_TEST_URL` continues to
  identify the currently selected normal-server invocation.
- Before the selected test runs, existing `src/internal/api/tmux_test.go`
  `TestMain` creates `TMPDIR/chrote-tmux-tests-*/` and sets
  `CHROTE_SESSION_BANK_PATH` to its `sessions.json`. The fixture permits exactly
  that one additional `CHROTE_*` variable, resolves the parent and requires it
  to be a direct `chrote-tmux-tests-*` child of resolved `TMPDIR`, requires the
  exact `sessions.json` basename, and never opens or reads the path. It rejects
  every other `CHROTE_*` variable, thereby rejecting every authority, root,
  provider, and runtime input. The supplied nonce protects its exact test-only
  `POST /api/__formations_contract/shutdown` route. After Playwright, the script
  calls that exact route, waits for `http.Server.Shutdown` and a zero
  test-process exit, and treats forced termination as failure. Its failure trap
  may kill the disposable child only after recording that graceful shutdown did
  not occur.
- The normal server must return 404 for that exact API shutdown route through
  the existing production API fallback. The script scans
  the supplied production binary for the fixture marker and fails if found;
  tests also prove no production route/constructor exposes the test fixture and
  `RuntimeAuthorityCapability.SemanticProjection` remains false. The `_test.go`
  file is therefore compiled only by `go test -c`, never into `chrote-server`.
- Per-process stdout/stderr is retained exactly as
  `$artifact_root/normal-schema1.log`,
  `$artifact_root/normal-schema2-no-fallback.log`, and
  `$artifact_root/artifact-contract.log`; every successful contract run requires
  all three server logs to exist and be nonempty. Task 3's non-server direct
  guard probe additionally retains exact output as
  `$artifact_root/authority-guard-probe.log`; success requires that file to be
  nonempty and contain its frozen acceptance marker before either normal
  process starts.
- All three server invocations, the one non-server guard-probe invocation,
  TestMain state, logs, binaries, and copied fixtures live under the step's
  `mktemp -d` evidence directory. No lane touches a live service, provider,
  authority root, or tmux server.

### Step 4.1: Write paging, SSE, and artifact HTTP tests

- [ ] Test query absence (`since=0`, `limit=200`), minimum/maximum values, negative text, signs, decimals, overflow, duplicate values, empty values, and explicit `limit=0/201`. Parse with unsigned integer functions, never float JSON logic.
- [ ] Assert one events request causes one `ReadCanonicalRun`, returns exactly one `RunEventPage` whose run ID, lowercase 64-hex generation, and source exact-match `ProjectRunView` from that same projection, and cannot exceed 1 MiB before the fixed success wrapper. Mismatched/invalid generation rejects the whole response.
- [ ] Assert a projection-only tail advances the returned cursor and can produce an empty page with `hasMore:false`.
- [ ] Assert an individually oversized sanitized event maps to the registered resource-limit HTTP error with no partial page.
- [ ] For SSE, inject more than 200 events plus a projection-only tail. Assert the store is read once; events remain ascending and sanitized; each event's `id` equals its canonical sequence; no full replay array is constructed; the final frame is `event: cursor` with equal id/data high water; and the response closes.
- [ ] Preserve existing resume precedence: a present `Last-Event-ID` overrides query `since`. Test the combination exactly once and reject malformed/unsafe cursor values before writing SSE headers.
- [ ] Send a JSON-safe `Last-Event-ID` greater than the snapshot's latest sequence. Assert no safe event frame, then exactly one final cursor control frame whose id and one-field data both retain that future value; never regress it to the snapshot latest sequence.
- [ ] Assert SSE never loops, sleeps, polls a second snapshot, or watches tmux. Use a reader spy and a response recorder; no timing assertion.
- [ ] For artifact open, assert exact available bytes, `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Content-Type` from the second verified projection, exact `Content-Length`, and absence of `Content-Disposition` and validators. Assert every unavailable/redacted/expired/missing/mutated/symlink/hash/size failure sets `Cache-Control: no-store`, returns no artifact bytes, and leaks no stored path/descriptor.
- [ ] In `formations_run_contract_server_test.go`, specify the test-only server lifecycle, in-memory reader/opener call counts, exact fixture bytes, fixture marker, nonce-protected exact `POST /api/__formations_contract/shutdown`, and clean zero exit. Assert its `CHROTE_*` environment is exactly the two contract variables plus the validated TestMain-owned session-bank variable; it never reads that path and rejects every other CHROTE input. Add built-contract assertions that both normal-server invocations return 404 for that exact method/path through the existing API fallback, the binary omits the marker, and only `CHROTE_FORMATIONS_ARTIFACT_CONTRACT_URL` reaches the artifact-success fixture. Re-run Task 3's tagged direct-guard probe against the same generated root as the preflight before either normal server; do not add an API reason field or another guard probe.

Representative SSE assertion:

```go
func TestStreamRunEventsUsesOneSnapshotAndEndsWithCursor(t *testing.T) {
	store := newCountingCanonicalStore(eventsFixture(205, projectionOnlyTail(206)))
	response := performSSE(t, store, "/api/formations/runs/run-1/stream")

	if got := store.ReadCanonicalRunCalls(); got != 1 { t.Fatalf("reads = %d", got) }
	if got := response.LastEventID(); got != 206 { t.Fatalf("last id = %d", got) }
	if got := response.LastEvent().Name; got != "cursor" { t.Fatalf("last event = %q", got) }
	var cursor struct { Cursor uint64 `json:"cursor"` }
	if err := json.Unmarshal([]byte(response.LastEvent().Data), &cursor); err != nil { t.Fatal(err) }
	if cursor.Cursor != 206 { t.Fatalf("cursor data = %d", cursor.Cursor) }
	if !response.Closed { t.Fatal("SSE response did not close") }
}
```

Run RED:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
go test ./internal/api -run 'Test(GetRunEventsPage|GetRunEventsQuery|StreamRunEvents|OpenRunArtifact)' -count=1
task4_contract_dir="$(mktemp -d "${TMPDIR:-/tmp}/ctx-7i1-task4-red.XXXXXX")"
go build -o "$task4_contract_dir/server" ./cmd/server
cd ..
CHROTE_SERVER_BINARY="$task4_contract_dir/server" CHROTE_CONTRACT_ARTIFACT_DIR="$task4_contract_dir/artifacts" scripts/test-built-server-contract.sh >"$task4_contract_dir/contract.log" 2>&1
```

Expected RED: focused and built-server transport assertions fail because the old events endpoint/SSE read an unbounded raw-event slice, no canonical artifact-open route or header policy exists, and the compiled test-only handler fixture is not yet bindable. A compile failure caused only by the named missing reader/opener injection seam is valid RED; unrelated compile failures are not. Preserve the exact temporary evidence path/output. Commit tests only and obtain `APPROVED_RED`:

```bash
git add src/internal/api/formations_run_events_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go src/internal/api/formations_run_contract_server_test.go dashboard/tests/contract/built-server.spec.ts scripts/test-built-server-contract.sh
git commit -m "test(api): specify bounded run event transport"
```

### Step 4.2: Implement strict event query parsing and one-page response

- [ ] Add `parseRunEventPageRequest(*http.Request) (since uint64, limit int, err error)`. Accept each parameter at most once. Default an absent `since` to `0` and absent `limit` to `RunPageDefaultLimit`; reject present empty values.
- [ ] Parse both values with the digits-only `strconv.ParseUint` helper below. Enforce `since <= MaxJSONSafeInteger` and `limit` in `1..RunPageMaximumLimit` before converting limit to `int`.
- [ ] Call `ReadCanonicalRun` once, select both `RunView` and `RunEventPage` from that same opaque projection, and require exact run ID/generation/source equality before `core.WriteSuccess`. Preserve typed error mapping. Do not call `ReadRunView` or perform a second authority read merely to check parity.

```go
var errInvalidRunEventQuery = errors.New("invalid formations run event query")

type runEventQueryError struct {
	Field string
}

func (e *runEventQueryError) Error() string { return errInvalidRunEventQuery.Error() }
func (e *runEventQueryError) Unwrap() error { return errInvalidRunEventQuery }

func parseRunEventPageRequest(r *http.Request) (uint64, int, error) {
	query := r.URL.Query()
	sinceText, hasSince, err := singleRunEventQueryValue(query, "since")
	if err != nil {
		return 0, 0, err
	}
	limitText, hasLimit, err := singleRunEventQueryValue(query, "limit")
	if err != nil {
		return 0, 0, err
	}

	since := uint64(0)
	if hasSince {
		since, err = parseRunEventUint("since", sinceText, formations.MaxJSONSafeInteger)
		if err != nil {
			return 0, 0, err
		}
	}

	limit := uint64(formations.RunPageDefaultLimit)
	if hasLimit {
		limit, err = parseRunEventUint("limit", limitText, formations.RunPageMaximumLimit)
		if err != nil || limit == 0 {
			return 0, 0, &runEventQueryError{Field: "limit"}
		}
	}
	return since, int(limit), nil
}

func singleRunEventQueryValue(query url.Values, field string) (string, bool, error) {
	values, ok := query[field]
	if !ok {
		return "", false, nil
	}
	if len(values) != 1 || values[0] == "" {
		return "", false, &runEventQueryError{Field: field}
	}
	return values[0], true, nil
}

func parseRunEventUint(field, value string, maximum uint64) (uint64, error) {
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return 0, &runEventQueryError{Field: field}
		}
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed > maximum {
		return 0, &runEventQueryError{Field: field}
	}
	return parsed, nil
}
```

### Step 4.3: Implement finite single-snapshot SSE

- [ ] Resolve the resume cursor before headers: use `Last-Event-ID` when present, otherwise query `since`, otherwise `0`. Apply the same digits-only and JSON-safe validation. Read one `CanonicalRunProjection` and reuse it for all internal pages.
- [ ] Before SSE headers, select the structural view and first page from that projection and require every internally selected page to exact-match its run ID/generation/source. A mismatch is the registered projection error; no page from another run incarnation is streamed.
- [ ] Loop `ProjectRunEventPage(projection, cursor, 200)`. Write each event immediately as one SSE frame and check every write/flush error; do not append it to an all-events slice.
- [ ] When a page is empty but advances across omissions, continue from its cursor. Add a progress guard: if `hasMore` is true and cursor did not advance, return the registered internal projection error.
- [ ] After `hasMore:false`, emit exactly one transport-only frame with `event: cursor`, `id: <cursor>`, and data containing exactly `{"cursor":<cursor>}` with no other members; flush and return. Do not start a timer or second read.

The Task 1 reducer's singleton-full-page validation guarantees that an oversized individual event fails before this adapter writes SSE headers. Do not add a second transport preflight or retain all pages.

### Step 4.4: Add optimistic artifact-open adapter

- [ ] Register exactly `GET /api/formations/runs/{runId}/artifacts/{artifactId}` and validate both IDs with existing safe-ID rules.
- [ ] Give the handler narrow canonical-run-reader and verified-artifact-opener dependencies. The production constructor binds the existing `Store`; the test-only `_test.go` server injects in-memory implementations. Keep route registration and response logic in production `formations.go`; do not add an environment hook, production fixture constructor, second reducer, or capability change.
- [ ] Call `OpenVerifiedRunArtifact` once. Set `Content-Type` from the media type field authorized by the successful second projection, set exact `Content-Length`, and emit no `Content-Disposition` header in this slice.
- [ ] Set `Cache-Control: no-store` before any artifact validation/error branch. On success additionally set `X-Content-Type-Options: nosniff`; do not set validators. Preserve those headers when the response writer emits the verified bytes.
- [ ] Write only `VerifiedRunArtifact.Bytes`. Never resolve a path in the API package and never call `os.Open` from the handler.
- [ ] Map authorization change, unavailable state, invalid descriptor, verification mismatch, not found, and resource limit through `writeFormationsError` before writing a body.
- [ ] Extend the normal disposable built-server fixture to exercise one bounded schema-1 events page with exact generation/source/cursor, including generation equality with detail, and a finite SSE replay with final cursor frame. Exercise verified artifact bytes plus every accepted success/error header only through the separate compiled-test server URL. Prove its nonce shutdown exits cleanly and that the supplied normal binary excludes the fixture. No live provider/service/tmux is used.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/api/formations.go internal/api/formations_run_events_test.go internal/api/formations_test.go internal/api/formations_acceptance_test.go internal/api/formations_run_contract_server_test.go internal/formations/run_projection.go internal/formations/run_projection_events.go
go test ./internal/api -run 'Test(GetRunEventsPage|GetRunEventsQuery|StreamRunEvents|OpenRunArtifact)' -count=1
go test -race ./internal/api ./internal/formations -run 'Test(GetRunEvents|StreamRunEvents|OpenRunArtifact|ProjectRunEventPage|OpenVerifiedRunArtifact)' -count=1
task4_contract_dir="$(mktemp -d "${TMPDIR:-/tmp}/ctx-7i1-task4-green.XXXXXX")"
go build -o "$task4_contract_dir/server" ./cmd/server
cd ..
CHROTE_SERVER_BINARY="$task4_contract_dir/server" CHROTE_CONTRACT_ARTIFACT_DIR="$task4_contract_dir/artifacts" scripts/test-built-server-contract.sh >"$task4_contract_dir/contract.log" 2>&1
sed -n '1,240p' "$task4_contract_dir/contract.log"
```

Expected GREEN: all event, SSE, and artifact route tests pass without sleeps or retries; the compiled test server accepts only the two contract variables plus its verified evidence-rooted TestMain session path, shuts down cleanly through exact `POST /api/__formations_contract/shutdown`, and leaves no state outside the evidence root; both normal servers return 404 for that method/path through the API fallback; the direct guard probe has already accepted the same no-fallback tree; and the production binary contains neither the fixture marker nor the test-only route. Commit production separately:

```bash
git add src/internal/api/formations.go src/internal/api/formations_run_events_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go src/internal/api/formations_run_contract_server_test.go src/internal/formations/run_projection.go src/internal/formations/run_projection_events.go dashboard/tests/contract/built-server.spec.ts scripts/test-built-server-contract.sh
git commit -m "feat(api): add bounded formations run event transport"
```

Before closing `.4`, scan for prohibited transport behavior:

```bash
cd /srv/chrote-worktrees/formations-run-view
rg -n 'ReadRunEvents|time\.(Sleep|Ticker|NewTicker)|for \{.*ReadCanonicalRun|os\.Open' src/internal/api/formations.go
```

The allowed result is no raw event read, no SSE polling timer, no repeated canonical read, and no API-level file open.

## Task 5 (Milestone 3): `ctx-7i1.5` — Migrate Archon run commands

**Depends on:** `ctx-7i1.2`

**Files:**

- Create: `src/cmd/archon/run_projection_test.go`
- Modify: `src/cmd/archon/main.go`
- Modify: `src/cmd/archon/main_test.go`
- Modify: `src/cmd/archon/runtime_authority_test.go`

**Accepted engineering decision:** today `run logs --json` writes one JSON array of raw events, while `run follow --json` writes one raw event per NDJSON line. This plan deliberately changes both to one complete versioned `RunEventPage` per NDJSON line so each transferred unit is bounded and carries generation/source/cursor/`hasMore` semantics. That is a breaking public agent/CLI wire change, not an internal refactor or visual UX change. The Task 5 RED review must verify the sealed page-NDJSON, JSON+node usage-error, and stderr-only stream-failure semantics; it does not choose a compatibility wrapper.

A new ADR for this break is explicitly `DECLINED`. Do not create one or add a
compatibility/legacy-hardening side path; the owner ruling, approved design,
this plan, and the RED/GREEN contract tests are the durable decision record and
enforcement.

**Command contract:**

- `run list --json` writes one `RunListPage`; `--after` and `--limit` use the same strict bounded list contract as the API. `run status --json` writes one `RunView`.
- `run logs --json` and `run logs --follow --json` write one complete `RunEventPage` per NDJSON line. Each line independently preserves `formations.run-events.v1` and the same immutable generation/source as its structural view; no all-events JSON array is rebuilt.
- `run follow --json` likewise writes one `RunEventPage` per NDJSON line. Text modes stream the safe events within each page.
- `--node` is text-only. Combining `--node` with `--json` returns usage exit 2 before a read or stdout byte. A mid-stream JSON failure writes only a safe error to stderr, exits nonzero, and never appends a different JSON shape to stdout.
- Schema-1 mutation JSON preserves existing keys with a canonical compatibility `RunView` status. A bound canonical provider writes `{receipt: RunCommandReceipt}`. This branch fixture-tests the latter but leaves it unavailable without `ctx-ug7.6.1`.

### Step 5.1: Write CLI contract and paging tests

- [ ] Add list/status JSON golden fixtures using the exact `RunListPage`/`RunView` contracts and text fixtures using safe identity/audit fields. Cover ascending run-id order, exclusive `after`, 50 maximum candidates, filtering cursor behavior, and the 4 MiB boundary without accumulating all pages.
- [ ] Add 201-event logs/follow fixtures with a projection-only slot at a page edge. Assert every JSON line decodes as `RunEventPage`, every page exact-matches the snapshot view's run ID/generation/source, each page scans no more than 200 canonical slots, cursors strictly advance, omitted slots still advance, and concatenated visible event sequences are ordered without duplicates. Inject a substituted generation and require fail-closed stderr/nonzero behavior with no substituted page on stdout.
- [ ] For text mode, apply `--node` filtering only after page selection and preserve the canonical cursor when every visible event is filtered. For JSON, assert `--node` is rejected with usage exit 2 before a canonical read or stdout write; every successful stdout line remains a complete unfiltered page.
- [ ] Assert `--since` rejects negative, signed, decimal, overflow, and above-JSON-safe values; accept `0` and the maximum. Change the flag storage to an exact unsigned-decimal string parser rather than `flag.Int`.
- [ ] Prove each follow poll calls `ReadCanonicalRun` once, consumes all bounded pages from that snapshot, checks finality from its `RunView`, and only then sleeps before a later snapshot. Use an injected sleeper; tests must not wait on wall time.
- [ ] Assert `run ask` uses `RunView.Nodes`, `Attempts`, `Gates`, `Outputs`, `Blocks`, `Escalations`, and `Artifacts`; it must not call an event or escalation ledger reader.
- [ ] Assert schema-1 start/resume/abort/verdict JSON retains its current `runId`/`status` keys, the status is a `RunView` with `source.compatibility:true`, and no receipt appears.
- [ ] Construct the same exact `SubmittedCommandIdentity` after Archon alias/default normalization and feed matching applied/rejected `CanonicalCommandReadInput` through the receipt adapter. Assert exact receipt JSON, including rejected start without run, and reject wrong-id/hash/kind substitutions. Assert the production runtime returns authority unavailable while the provider is unbound.
- [ ] Assert all pre-stream errors use existing `failJSON`/usage handling and contain no raw path, private authority, tmux route, or raw payload. Inject a failure after one JSON page and prove stdout contains only that complete page, the safe error is stderr-only, and exit is nonzero.

Use a store spy to make the prohibited path executable evidence:

```go
func TestRunAskUsesCanonicalViewOnly(t *testing.T) {
	store := newArchonCanonicalStoreSpy(runAskViewFixture())
	code := runAsk(store.Store(), []string{"run_test", "what is blocked?"}, &stdout, &stderr)
	if code != 0 { t.Fatalf("code = %d, stderr = %s", code, stderr.String()) }
	if store.ReadRunViewCalls() != 1 { t.Fatalf("view reads = %d", store.ReadRunViewCalls()) }
	if store.RawLedgerCalls() != 0 { t.Fatalf("raw ledger reads = %d", store.RawLedgerCalls()) }
}
```

Run RED:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
go test ./cmd/archon -run 'TestRun(List|Status|Logs|Follow|Ask|Mutation).*Canonical|TestRun.*Compatibility|TestRun.*Receipt' -count=1
```

Expected RED: commands still call `ListRuns`, `ProjectRun`, `ReadRunEvents`, and `ProjectOpenEscalations`, and JSON logs are unbounded raw arrays. Commit tests only and obtain `APPROVED_RED`:

```bash
git add src/cmd/archon/run_projection_test.go src/cmd/archon/main_test.go src/cmd/archon/runtime_authority_test.go
git commit -m "test(archon): specify canonical run commands"
```

### Step 5.2: Add one reusable bounded-page iterator

- [ ] Add a private Archon helper that accepts one opaque `CanonicalRunProjection`, starting cursor, and callback. It repeatedly selects pages at limit 200, invokes the callback once per page, advances through empty omitted pages, and fails on a cursor stall.
- [ ] The iterator performs no read and no sleep. The caller decides whether to obtain another snapshot for live CLI follow.

```go
func forEachRunEventPage(
	projection formations.CanonicalRunProjection,
	since uint64,
	visit func(formations.RunEventPage) error,
) (uint64, error) {
	cursor := since
	view := formations.ProjectRunView(projection)
	for {
		page, err := formations.ProjectRunEventPage(projection, cursor, formations.RunPageMaximumLimit)
		if err != nil { return cursor, err }
		if page.RunID != view.RunID || page.Generation != view.Generation || !sameCanonicalRunSourceValue(page.Source, view.Source) {
			return cursor, errRunPageIdentityMismatch
		}
		if err := visit(page); err != nil { return cursor, err }
		if page.HasMore && page.Cursor == cursor { return cursor, errRunPageCursorStalled }
		cursor = page.Cursor
		if !page.HasMore { return cursor, nil }
	}
}
```

Define `errRunPageCursorStalled` and `errRunPageIdentityMismatch` as static safe errors in `main.go` and map them through the existing CLI error path. The private `sameCanonicalRunSourceValue` compares event schema, compatibility, and optional authority-schema nil/value semantics; it never compares Go pointer identity.

### Step 5.3: Migrate list, status, logs, follow, and ask

- [ ] Change list to one `ListRunViews` page and status to `ReadRunView`. Parse `--after` and `--limit` with the API's exact rules; do not loop or rebuild all list pages. Text output reads board slug from `view.Identity`, count from `view.Audit`, and resumability from `view.Actions`; it does not infer state from event types.
- [ ] For non-follow logs, call `ReadCanonicalRun` once and pass it to the iterator. JSON visits write each whole unfiltered page with `writeNDJSON`; reject JSON+node before entering the iterator. Text visits may filter and render only `SafeRunEvent` values while retaining the page cursor.
- [ ] Make the JSON stream writer sticky-fail: after the first encode/write error it writes no further stdout shape, reports one safe stderr error through the caller, and exits nonzero. Update help and goldens to state page-NDJSON and the JSON+node usage restriction.
- [ ] For logs-follow and follow, each poll calls `ReadCanonicalRun` once, selects `RunView` and all new pages from that same value, then exits on `view.Final`; otherwise use the existing interval through an injectable sleeper. Do not turn Archon follow into the API replay-only SSE behavior.
- [ ] Parse `--since` with the same digits-only, JSON-safe helper semantics as the API. Store cursor as `uint64`; never cast through `int`.
- [ ] Replace `buildRunAskResponse` inputs with one `RunView`. Resolve waiting Gates, evidence, output projections, blocks, escalations, and latest artifact state from its typed fields. Remove event switches and `ProjectOpenEscalations` calls.
- [ ] Delete or narrow `filterRunEvents`, `lastRunSeq`, `runEventReferencesNode`, and old raw-event text helpers after the new safe equivalents are covered. Do not retain two paths.

### Step 5.4: Migrate mutation output without receipt synthesis

- [ ] Schema-1 engine methods remain compatibility writes. Immediately read the resulting `RunView`, require `Source.Compatibility`, and preserve the existing JSON keys and legible text fields.
- [ ] Add a private formatter for a real `RunCommandReceipt` returned by the canonical command adapter. Wire it only to the injected fixture provider. Production `NewRuntimeStore` keeps it unavailable because `ctx-ug7.6.1` is absent and `SemanticProjection` is false.
- [ ] Never derive command ID/hash/fence/effect/rejection fields from the schema-1 status, latest event, CLI arguments, or an error.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w cmd/archon/main.go cmd/archon/run_projection_test.go cmd/archon/main_test.go cmd/archon/runtime_authority_test.go
go test ./cmd/archon -run 'TestRun(List|Status|Logs|Follow|Ask|Mutation).*Canonical|TestRun.*Compatibility|TestRun.*Receipt' -count=1
go test -race ./cmd/archon -run 'TestRun(List|Status|Logs|Follow|Ask|Mutation)' -count=1
```

Expected GREEN: focused and race tests pass with no real sleep and no raw-ledger call. Commit production separately:

```bash
git add src/cmd/archon/main.go src/cmd/archon/run_projection_test.go src/cmd/archon/main_test.go src/cmd/archon/runtime_authority_test.go
git commit -m "feat(archon): consume canonical run projection"
```

Before closing `.5`, require this scan to contain no production call in `cmd/archon`:

```bash
cd /srv/chrote-worktrees/formations-run-view
rg -n 'ReadRunEvents|ProjectRun\(|ListRuns\(|ProjectOpenEscalations|\[\]formations\.RunEvent' src/cmd/archon
```

## Task 6 (Milestone 3): `ctx-7i1.6` — Migrate Formations-backed Comms rooms

**Depends on:** `ctx-7i1.2`

**Files:**

- Create: `src/internal/comms/run_projection_test.go`
- Modify: `src/internal/comms/projection.go`
- Modify: `src/internal/comms/projection_test.go`
- Modify: `src/internal/api/comms.go`
- Modify: `src/internal/api/comms_test.go`
- Modify: `dashboard/tests/contract/built-server.spec.ts`
- Modify: `scripts/test-built-server-contract.sh`

This task is independent of Task 5 and may be implemented/reviewed in parallel only when separate agents and non-overlapping files are available. It must not wait on an Archon commit to claim a false dependency.

**Accepted engineering decision:** existing Formations `ProjectRoom` must stop materializing the complete raw ledger. A run-room preview is one canonical 200-scanned-slot page and is honest about completeness: `RoomProjection` adds `messagesCursor`/`messagesHasMore`, `RoomMessages` adds `hasMore`, and run export adds `eventsCursor`/`eventsHasMore` while retaining the 200-message cap. Sequence/cursor/count members become JSON-safe `uint64`. Every completeness field is pointer-backed where omission preserves an existing non-run response shape: run rooms set non-nil values even for zero/false, while non-run rooms keep nil and omit the new fields. The Task 6 RED review verifies this sealed contract rather than choosing a truncation policy.

Exact JSON additions:

```go
type RoomProjection struct {
	// existing members unchanged
	MessagesCursor  *uint64 `json:"messagesCursor,omitempty"`
	MessagesHasMore *bool   `json:"messagesHasMore,omitempty"`
}

type RoomMessages struct {
	// existing members unchanged
	NextSince uint64 `json:"nextSince"`
	HasMore   *bool  `json:"hasMore,omitempty"`
}

type RoomExport struct {
	// existing members unchanged
	EventsCursor  *uint64 `json:"eventsCursor,omitempty"`
	EventsHasMore *bool   `json:"eventsHasMore,omitempty"`
}
```

### Step 6.1: Write run-room canonical source and cursor tests

- [ ] Build one canonical run fixture containing safe messages, a projection-only gap, a blocked node, an escalation, an available artifact later revoked, and private raw event keys.
- [ ] Assert `ProjectRoom("run:<id>")` identifies `Source.Kind` as `formations-run-view`, derives status/finality/board/Mission/Bead/event count from `RunView`, includes only the first bounded canonical page of safe messages, and exposes that page's exact cursor/`hasMore` as `messagesCursor`/`messagesHasMore`.
- [ ] Assert run-room claims, summary, artifacts, and risks derive from typed view/page members. The unavailable latest artifact appears at most as unavailable metadata; its earlier readable ref and private raw metadata never reappear.
- [ ] Assert `Messages` with `Since` and `Limit` invokes `ProjectRunEventPage` semantics directly, requires the page's run ID/generation/source to exact-match the view from the same projection, returns `NextSince == page.Cursor`, exposes non-nil `HasMore` even when false, and advances when a page emits no message because every scanned slot is omitted or filtered.
- [ ] Through `src/internal/api/comms.go`, test `run:` query absence, `since=0`, maximum safe integer, and `limit=1/200`; reject negative, sign-prefixed, decimal, overflow, above-safe, duplicate, and empty values with HTTP 400 `FORMATIONS_RUN_QUERY_INVALID`. Each field is parsed at most once.
- [ ] Freeze non-run API parity with existing fixtures: its parser/default behavior and valid response JSON are byte-for-byte unchanged, including omission of `RoomMessages.hasMore` through a nil pointer. Select the run-only strict parser only after the room id is known to have the `run:` prefix.
- [ ] Assert run export keeps at most 200 messages and reports exact `eventsCursor`/`eventsHasMore`; non-run exports omit those new members.
- [ ] Assert `IncludePrivateFor` has no effect on a Formations run room. Canonical event/page output is already the complete allowed public surface.
- [ ] Add a counting reader fixture proving each run-room operation performs one immutable canonical read and zero `ReadRunEvents`/`ProjectRun` calls.
- [ ] Run the full existing non-run Comms suite unchanged to prove mission/other room ledgers retain their current behavior.

Representative cursor proof:

```go
func TestRunMessagesAdvanceAcrossOmittedCanonicalSlot(t *testing.T) {
	store := newRunRoomFixture(t, publicEvent(1), projectionOnlyEvent(2, "test_projection_only_redacted"), publicEvent(3))
	page, err := store.Messages("run:run_test", MessageOptions{Since: 1, Limit: 1})
	if err != nil { t.Fatal(err) }
	if len(page.Messages) != 0 { t.Fatalf("messages = %#v", page.Messages) }
	if page.NextSince != 2 || page.HasMore == nil || !*page.HasMore {
		t.Fatalf("cursor = %d, hasMore = %#v", page.NextSince, page.HasMore)
	}
}
```

Run RED:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
go test ./internal/comms ./internal/api -run 'TestRun(Room|Messages|Artifacts|Risks|Projection|Export|CommsQuery).*Canonical|TestRunMessagesAdvance' -count=1
task6_contract_dir="$(mktemp -d "${TMPDIR:-/tmp}/ctx-7i1-task6-red.XXXXXX")"
go build -o "$task6_contract_dir/server" ./cmd/server
cd ..
CHROTE_SERVER_BINARY="$task6_contract_dir/server" CHROTE_CONTRACT_ARTIFACT_DIR="$task6_contract_dir/artifacts" scripts/test-built-server-contract.sh >"$task6_contract_dir/contract.log" 2>&1
```

Expected RED: focused and built-server Comms assertions fail because `projectRunRoom` reads raw events/legacy status, paging operates on an unbounded materialized slice, strict run parsing is absent, and completeness metadata is missing. Preserve the exact temporary evidence path/output. Commit tests only and obtain `APPROVED_RED`:

```bash
git add src/internal/comms/run_projection_test.go src/internal/comms/projection_test.go src/internal/api/comms_test.go dashboard/tests/contract/built-server.spec.ts scripts/test-built-server-contract.sh
git commit -m "test(comms): specify canonical formations run rooms"
```

### Step 6.2: Split run-room reads from non-run ledgers

- [ ] At `ProjectRoom`, preserve every non-run branch exactly. Route only `run:<runId>` to a canonical helper.
- [ ] The run helper calls `ReadCanonicalRun` once, selects `RunView`, and selects one page with `since=0`, `limit=200`. It requires page run ID/generation/source to exact-match that view before mapping any message. It never calls the generic `readEvents` function.
- [ ] Copy the selected page cursor/completeness into `RoomProjection.MessagesCursor` and `MessagesHasMore`; an empty-but-advanced page is distinguishable from a complete empty transcript.
- [ ] Set source fields from typed view fields: status/final directly; board/Mission/Bead from `Identity`; count from `Audit.ConsumedEventCount`; `Kind:"formations-run-view"`; `ReadOnly:true`.
- [ ] Convert messages only from `SafeRunEvent` variants. Use per-variant safe fields; no fallback formatting of arbitrary `data`, `fmt.Sprint(map)`, or raw JSON.
- [ ] Derive artifacts from latest `view.Artifacts`. Derive risks from `view.Blocks` and `view.Escalations`. Derive summary from view/node/Gate finality plus those typed room values.

### Step 6.3: Page run messages at the canonical source

- [ ] Change `RoomMessage.Seq`, `MessageOptions.Since`, `RoomMessages.NextSince`, `RoomSource.EventCount`, and `RoomSummary.EventCount` from `int` to `uint64` so canonical JSON-safe sequences/counts never narrow. For non-run raw events, reject a negative sequence before checked conversion; convert existing non-negative `len` counts explicitly; JSON output for valid existing fixtures is unchanged.
- [ ] Add `MessagesCursor *uint64 \`json:"messagesCursor,omitempty"\`` and `MessagesHasMore *bool \`json:"messagesHasMore,omitempty"\`` to `RoomProjection`; add `HasMore *bool \`json:"hasMore,omitempty"\`` to `RoomMessages`; add `EventsCursor *uint64 \`json:"eventsCursor,omitempty"\`` and `EventsHasMore *bool \`json:"eventsHasMore,omitempty"\`` to `RoomExport`. Run rooms set every completeness pointer non-nil even for zero/false; non-run rooms leave them nil. Preserve all existing members and exact non-run JSON shape.
- [ ] For a run room, validate `Since <= MaxJSONSafeInteger` and `Limit` as `1..200`; use `200` only when the caller leaves limit zero under the existing default convention.
- [ ] Read one `CanonicalRunProjection`, select its view and exactly one page, exact-match run ID/generation/source, map its safe events to messages, and return `NextSince: page.Cursor` plus `HasMore: pointerTo(page.HasMore)`. Message filtering cannot change the cursor.
- [ ] Preserve the existing `ProjectRoom`-based path for non-run rooms. Do not change their limit/default behavior.
- [ ] In `src/internal/api/comms.go`, parse each run-room `since`/`limit` at most once using the shared digits-only unsigned helper: `since` is `0..MaxJSONSafeInteger`; absent limit is 200; explicit limit is `1..200`; malformed/negative/signed/decimal/overflow/duplicate/empty values return typed 400. Preserve the existing non-run path unchanged.
- [ ] Preserve the current export's effective 200-message cap: for a run room, call canonical `Messages` once with `Limit:200` instead of the current out-of-range `Limit:1000`. Set `RoomExport.EventsCursor/EventsHasMore` from that page. Do not add an unbounded page accumulator. Leave the new fields omitted for non-run valid JSON.
- [ ] Extend the disposable built-server fixture to request a paged `run:` room and export, proving page generation/source parity, strict parsing, non-nil run false completeness, omitted non-run completeness, the 200 cap, and no raw private fields. It uses the same temporary schema-1 fixture and no live provider/service/tmux.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/comms/projection.go internal/comms/run_projection_test.go internal/comms/projection_test.go internal/api/comms.go internal/api/comms_test.go
go test ./internal/comms ./internal/api -run 'TestRun(Room|Messages|Artifacts|Risks|Projection|Export|CommsQuery).*Canonical|TestRunMessagesAdvance' -count=1
go test -race ./internal/comms ./internal/api -run 'TestRun(Room|Messages|Projection|Export|CommsQuery)|TestProjectRoom|TestMessages|TestExport' -count=1
go test ./internal/comms ./internal/api -count=1
task6_contract_dir="$(mktemp -d "${TMPDIR:-/tmp}/ctx-7i1-task6-green.XXXXXX")"
go build -o "$task6_contract_dir/server" ./cmd/server
cd ..
CHROTE_SERVER_BINARY="$task6_contract_dir/server" CHROTE_CONTRACT_ARTIFACT_DIR="$task6_contract_dir/artifacts" scripts/test-built-server-contract.sh >"$task6_contract_dir/contract.log" 2>&1
sed -n '1,240p' "$task6_contract_dir/contract.log"
```

Expected GREEN: focused, race, and full Comms tests pass; non-run fixtures are byte-for-byte unchanged. Commit production separately:

```bash
git add src/internal/comms/projection.go src/internal/comms/run_projection_test.go src/internal/comms/projection_test.go src/internal/api/comms.go src/internal/api/comms_test.go dashboard/tests/contract/built-server.spec.ts scripts/test-built-server-contract.sh
git commit -m "feat(comms): project formations rooms canonically"
```

Before closing `.6`, require this scan to show raw reads only in the non-run ledger helper and its tests, never the Formations run branch:

```bash
cd /srv/chrote-worktrees/formations-run-view
rg -n 'ReadRunEvents|ProjectRun\(|messageFromRunEvent|formations-run-ledger' src/internal/comms
```

## Task 7 (Milestone 4): `ctx-7i1.7` — One dashboard run-data controller for Agents and Cockpit

**Depends on:** `ctx-7i1.3`, `ctx-7i1.4`

**Files:**

- Modify: `dashboard/src/components/formationsTypes.ts`
- Modify: `dashboard/src/components/formationsApi.ts`
- Modify: `dashboard/src/components/formationsApi.test.ts`
- Modify: `dashboard/src/components/formationsRunState.ts`
- Modify: `dashboard/src/components/formationsRunState.test.ts`
- Create: `dashboard/src/components/formationsRunController.tsx`
- Create: `dashboard/src/components/formationsRunController.test.tsx`
- Modify: `dashboard/src/components/FormationsCockpit.tsx`
- Modify: `dashboard/src/components/FormationsCockpit.test.tsx`
- Modify: `dashboard/src/components/AgentsView.tsx`
- Modify: `dashboard/src/components/AgentsView.test.tsx`
- Modify: `dashboard/src/App.tsx`
- Modify: `dashboard/src/App.test.tsx`
- Modify: `dashboard/tests/mock-api.ts`
- Modify: `dashboard/tests/formations/formations-cockpit.spec.ts`
- Modify: `dashboard/tests/formations/formations-smoke.spec.ts`

**Controller contract:**

```ts
export type RunDataFreshness = 'idle' | 'loading' | 'fresh' | 'stale'
export const RUN_EVENT_WINDOW_MAXIMUM = 400

export interface FormationsRunDataState {
  boardSlug: string
  runId: string | null
  view: RunView | null
  events: SafeRunEvent[]
  eventCursor: JsonSafeInteger
  eventHasMore: boolean
  freshness: RunDataFreshness
  errorCode: string
  mutationPending: boolean
  lastReceipt: RunCommandReceipt | null
}

export interface FormationsRunControllerValue {
  stateFor(boardSlug: string): FormationsRunDataState
  restore(boardSlug: string): Promise<void>
  refresh(boardSlug: string): Promise<void>
  start(boardSlug: string, etag: string, input: RunStartInput): Promise<void>
  resume(boardSlug: string, input: RunResumeInput): Promise<void>
  cancel(boardSlug: string, input: RunCancelInput): Promise<void>
  verdict(boardSlug: string, gateId: string, input: RunVerdictInput): Promise<void>
  canSubmit(boardSlug: string, action: RunAction['kind']): boolean
}

export function FormationsRunControllerProvider(props: React.PropsWithChildren): JSX.Element
export function useFormationsRunController(boardSlug: string): FormationsRunControllerValue
```

The provider stores entries by board slug so the keep-alive Cockpit and conditionally mounted Agents view can observe the same run without fighting over one global selected board. It owns at most one polling job per `runId`, even when both consumers are mounted.
Each entry retains at most the 400 highest-sequence events. `eventCursor` is the
highest canonical sequence consumed even after older display events are evicted.

### Step 7.1: Replace legacy TypeScript contracts with exact canonical types

- [ ] Add `JsonSafeInteger = number` plus runtime validation requiring an integer in `0..Number.MAX_SAFE_INTEGER`.
- [ ] Mirror every exact `RunView` structural type and ordering-bearing collection from the design: source, identity, audit, nodes, attempts, Gates, outputs, artifacts, blocks, escalations, sessions, actions, recovery state, and reconcile condition. Do not use `Record<string, unknown>` for typed public payloads.
- [ ] Define `SafeRunEvent` with the exact 41 type literals from the design appendix: 37 schema-2 types plus the four schema-1 compatibility-only types. Define source-selected closed data interfaces for the 17 shared literals and one for every source-only variant; no `Record<string, unknown>` payload. Private fields such as paths, routes, tokens, prompt/capture/pane/input bytes, and exact baselines do not exist in the TS types.
- [ ] Include mandatory `generation` on both `RunView` and `RunEventPage`, plus mandatory page source. Runtime guards validate each as lowercase 64-hex SHA-256 and the exact source union; a page guard requires page generation to exact-match its candidate view before exposing events.
- [ ] Define the exact two-arm `RunCommandReceipt` union. Define response unions that preserve server shapes without inventing a schema:

```ts
export type Schema1RunStartResponse = {
  runId: string
  status: RunView
  receipt?: never
}

export type Schema1RunStatusResponse = {
  status: RunView
  receipt?: never
}

export type CanonicalRunReceiptResponse = {
  receipt: RunCommandReceipt
  runId?: never
  status?: never
}

export type RunStartResponse = Schema1RunStartResponse | CanonicalRunReceiptResponse
export type RunMutationResponse = Schema1RunStatusResponse | CanonicalRunReceiptResponse
```

- [ ] Remove `RunStatusProjection`, raw `RunEvent`, `RunStatusResult`, and the legacy response normalizer once all call sites compile against the canonical types.
- [ ] Add API fixture tests that reject a status response unless `status.schema === 'formations.run-view.v1'`, reject compatibility status unless `status.source.compatibility === true`, and reject any response containing both `receipt` and `status`.

Run the type/API RED subset:

```bash
cd /srv/chrote-worktrees/formations-run-view/dashboard
npm run test:unit -- src/components/formationsApi.test.ts src/components/formationsRunState.test.ts
```

Keep these failing tests in the Task 7 tests-only commit described in Step 7.4; do not commit production types yet.

### Step 7.2: Specify atomic event-page merge and presentation selectors

- [ ] Replace overwrite-on-same-sequence `upsertRunEvent`. A repeated `(runId, seq)` must be canonically deep-equal; otherwise reject the complete candidate page without changing the prior events/cursor.
- [ ] Validate page schema, run ID, lowercase 64-hex generation, ascending sequences, `seq > requestedSince`, `seq <= page.cursor`, JSON-safe integers, and cursor progress before committing the candidate page.
- [ ] Validate page generation and source exact-equal the candidate view generation/source. Merge then retains only the 400 highest-sequence events while returning the consumed page cursor unchanged; eviction must not rewind or derive structural state.
- [ ] Canonical equality recursively sorts object keys because JSON member order is not semantic. Array order remains semantic.
- [ ] Replace event-derived node/status/action logic: node state comes from `RunView.nodes`; waiting human Gate comes from `RunView.gates` plus a matching `verdict` action; cancel/resume/peek availability comes only from `RunView.actions`. Keep safe event text rendering only for the existing history presentation.

Implement and test this complete pure merge shape:

```ts
export class RunDataContractError extends Error {
  readonly code: string

  constructor(code: string) {
    super(code)
    this.name = 'RunDataContractError'
    this.code = code
  }
}

export function assertJsonSafeInteger(value: number, field: string): asserts value is JsonSafeInteger {
  if (!Number.isSafeInteger(value) || value < 0) {
    throw new RunDataContractError(`invalid_${field}`)
  }
}

function canonicalJSON(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalJSON)
  if (value !== null && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, child]) => [key, canonicalJSON(child)]),
    )
  }
  return value
}

function sameSafeEvent(left: SafeRunEvent, right: SafeRunEvent): boolean {
  return JSON.stringify(canonicalJSON(left)) === JSON.stringify(canonicalJSON(right))
}

export function mergeRunEventPage(
  current: SafeRunEvent[],
  currentCursor: JsonSafeInteger,
  requestedSince: JsonSafeInteger,
  expectedRunId: string,
  expectedGeneration: string,
  expectedSource: CanonicalRunSourceProjection,
  page: RunEventPage,
): { events: SafeRunEvent[]; cursor: JsonSafeInteger; hasMore: boolean } {
  if (
    page.schema !== 'formations.run-events.v1' ||
    page.runId !== expectedRunId ||
    !/^[0-9a-f]{64}$/.test(page.generation) ||
    page.generation !== expectedGeneration ||
    JSON.stringify(canonicalJSON(page.source)) !== JSON.stringify(canonicalJSON(expectedSource))
  ) {
    throw new RunDataContractError('run_event_page_identity')
  }
  assertJsonSafeInteger(page.cursor, 'cursor')
  if (page.cursor < currentCursor || page.cursor < requestedSince) {
    throw new RunDataContractError('run_event_cursor_regressed')
  }
  const candidate = new Map(current.map(event => [event.seq, event]))
  let priorSeq = requestedSince
  for (const event of page.events) {
    assertJsonSafeInteger(event.seq, 'event.seq')
    if (event.runId !== expectedRunId || event.seq <= requestedSince || event.seq <= priorSeq || event.seq > page.cursor) {
      throw new RunDataContractError('run_event_sequence')
    }
    const existing = candidate.get(event.seq)
    if (existing && !sameSafeEvent(existing, event)) {
      throw new RunDataContractError('run_event_sequence_mismatch')
    }
    candidate.set(event.seq, event)
    priorSeq = event.seq
  }
  if (page.hasMore && page.cursor === requestedSince) {
    throw new RunDataContractError('run_event_cursor_stalled')
  }
  const retained = [...candidate.values()]
    .sort((left, right) => left.seq - right.seq)
    .slice(-RUN_EVENT_WINDOW_MAXIMUM)
  return {
    events: retained,
    cursor: page.cursor,
    hasMore: page.hasMore,
  }
}
```

Tests must take a before-state deep copy and prove every thrown branch leaves it byte-equal. Include an empty omitted page that advances, invalid/substituted page generation, an equal repeat with different object-key order, and a same-sequence payload mismatch.
Also insert 401 ordered events and prove only sequences 2..401 remain while the
consumed cursor stays 401.

### Step 7.3: Specify API calls and controller state transitions

- [ ] Change `fetchRunStatus` to `fetchRunView`, decoding `data.status`. Change `fetchRunEvents` to `fetchRunEventPage(runId, since, limit=200)`, decoding the page only after validating mandatory generation/source and all page members. Encode both numeric query values with `String` after runtime integer validation.
- [ ] Mutation API functions return the exact response unions above. Receipt guards validate every arm and forbidden field. Compatibility guards require a canonical `RunView` whose source marks compatibility.
- [ ] `restore(boardSlug)` reads only the run ID string from `activeRunStorageKey`. It treats that string as a hint, initializes retained cursor `0`, then runs the same bounded refresh protocol below. It never creates status/actions from storage or loops to historical completion.
- [ ] `refresh(boardSlug)` coalesces concurrent requests by run ID and has a fixed budget: at most three `fetchRunView` calls and exactly zero or one `fetchRunEventPage(runId, retainedCursor, 200)` call. It never loops over `hasMore` or chases the writer's newest future state.
- [ ] First read a candidate view. Require exact run id/generation/source match to the retained view when one exists and require `candidate.cursor >= retainedEventCursor`. Read one event page from `retainedEventCursor`, require `page.runId/page.generation/page.source` to exact-match that candidate, and merge it into a tentative 400-event window. If `page.cursor > candidate.cursor`, re-read the view, up to the three-view total, until one matching run ID/generation/source view satisfies `view.cursor >= page.cursor`. A later writer cursor beyond that selected page does not require another page read.
- [ ] Atomically commit the candidate view, tentative page/window, consumed cursor, and `eventHasMore` only when `retainedEventCursor <= page.cursor <= committedView.cursor`, page and committed view run ID/generation/source are exact, merge checks pass, and the run did not change generation. Set `eventHasMore = page.hasMore || page.cursor < committedView.cursor`.
- [ ] Any view/page/contract failure, non-convergence within the fixed budget, cursor regression, or generation/source replacement retains the complete previous last-good view/events/cursor byte-identically, sets `freshness:'stale'` and the safe error code, and disables every mutation/Peek submission. It never partially commits a view or page and never clears into a misleading empty/live state.
- [ ] The provider maintains one 1200 ms poll per non-final run ID while at least one board entry references it. Polling stops on a fresh final view or unmount. Tests inject fake timers and prove two consumers do not create two polls.
- [ ] After any mutation response, validate the response arm but do not apply optimistic status. For a receipt, store `lastReceipt`; for compatibility, require the response's `status.source.compatibility`. Then perform a fresh view/page reconciliation. If reconciliation fails, mark the old last-good state stale and keep actions disabled.
- [ ] On a validated schema-1 start response, persist only `runId` as the hint before reconciliation. Remove the hint only after a fresh final view. Schema-2 rejected start has no run ID and writes nothing.
- [ ] `canSubmit` returns true only when freshness is `fresh`, no mutation is pending, and the exact action exists in current `view.actions`. This also governs Peek metadata/input entry points; local storage, labels, or stale state cannot enable them.

Add controller transition tests for: cold restore, corrupt hint, duplicate observers,
one-page advancement, omitted empty page, view regression, substituted/invalid page generation, page newer than first
view, convergence on the second and third view read, non-convergence under
continuous writer churn, generation/source replacement, final view with lagging
history, 400-event eviction with cursor preservation, fixed page/view budgets,
byte-identical rollback, transient stale/read recovery, final cleanup, schema-1
compatibility, applied receipt, rejected start receipt, pending/unavailable
receipt, and mutation-success/reconciliation-failure.

### Step 7.4: Commit and review the complete RED checkpoint

Run:

```bash
cd /srv/chrote-worktrees/formations-run-view/dashboard
npm run test:unit -- src/components/formationsApi.test.ts src/components/formationsRunState.test.ts src/components/formationsRunController.test.tsx src/components/FormationsCockpit.test.tsx src/components/AgentsView.test.tsx src/App.test.tsx
```

Expected RED: canonical types/controller do not exist and both views still own independent restore/poll/event/action state. Commit all Task 7 tests, fixtures, and mock expectations only:

```bash
git add dashboard/src/components/formationsApi.test.ts dashboard/src/components/formationsRunState.test.ts dashboard/src/components/formationsRunController.test.tsx dashboard/src/components/FormationsCockpit.test.tsx dashboard/src/components/AgentsView.test.tsx dashboard/src/App.test.tsx dashboard/tests/mock-api.ts dashboard/tests/formations/formations-cockpit.spec.ts dashboard/tests/formations/formations-smoke.spec.ts
git commit -m "test(dashboard): specify shared canonical run controller"
```

Obtain `APPROVED_RED` before adding any production TypeScript.

### Step 7.5: Implement the provider and API layer

- [ ] Add the exact types and runtime guards, then implement the API functions and pure state helpers covered above.
- [ ] Implement provider state as an immutable board-slug map plus private in-flight/poll refs. Provider callbacks are stable and all updates use functional state transitions.
- [ ] Mount `FormationsRunControllerProvider` in `App` around `DashboardContent`, inside `SessionProvider` and outside both Cockpit and Agents. This placement survives tab switches and gives both views the same controller instance.

```tsx
function App() {
  return (
    <SessionProvider>
      <FormationsRunControllerProvider>
        <IframePoolProvider>
          <DashboardContent />
        </IframePoolProvider>
      </FormationsRunControllerProvider>
    </SessionProvider>
  )
}
```

- [ ] Use an observer/reference key in the hook so the kept-alive Cockpit and Agents can subscribe to the same slug without duplicating timers. Cleanup removes only that observer; it does not discard last-good state needed by the other view.

### Step 7.6: Remove duplicate consumer state without changing presentation

- [ ] In `FormationsCockpit`, remove local `activeRun`, `runEvents`, restore effect, run polling effect, refresh helper, local-storage writes, and direct mutation API calls. Read controller state and invoke controller actions instead.
- [ ] In `AgentsView`, remove the corresponding duplicate state/effects/calls. Both components select node state and open Gate from `RunView`, not event replay.
- [ ] Preserve existing labels, hierarchy, action placement, artifact reader, interactive Peek presentation, and multi-session visuals. Add only disabled-state wiring already required for stale/mutation-pending states; do not introduce new copy or CSS.
- [ ] Preserve existing history rendering using the controller's separate `SafeRunEvent[]`. Replace any lookup of raw `data.text`, `data.prompt`, `data.error`, or `reportRef` with exhaustive safe-variant selectors.
- [ ] Update mock endpoints to emit `data.status: RunView` and `data: RunEventPage`, including identical run-incarnation generation, source, cursors, and `hasMore`. Mutation mocks use existing schema-1 keys with compatibility source; do not fabricate receipts.
- [ ] Update Playwright assertions to prove one status poll and bounded page sequence serve both views across a tab switch, a stale response disables actions without erasing the view, and the visible layout/copy assertions remain unchanged.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/dashboard
npm run test:unit -- src/components/formationsApi.test.ts src/components/formationsRunState.test.ts src/components/formationsRunController.test.tsx src/components/FormationsCockpit.test.tsx src/components/AgentsView.test.tsx src/App.test.tsx
npm run lint
npm run build
npm run test:formations -- tests/formations/formations-cockpit.spec.ts tests/formations/formations-smoke.spec.ts
```

Expected GREEN: unit/component tests, lint, TypeScript/Vite build, and both Formations Playwright specs pass with no skipped relevant assertion. Commit production separately:

```bash
git add dashboard/src/components/formationsTypes.ts dashboard/src/components/formationsApi.ts dashboard/src/components/formationsApi.test.ts dashboard/src/components/formationsRunState.ts dashboard/src/components/formationsRunState.test.ts dashboard/src/components/formationsRunController.tsx dashboard/src/components/formationsRunController.test.tsx dashboard/src/components/FormationsCockpit.tsx dashboard/src/components/FormationsCockpit.test.tsx dashboard/src/components/AgentsView.tsx dashboard/src/components/AgentsView.test.tsx dashboard/src/App.tsx dashboard/src/App.test.tsx dashboard/tests/mock-api.ts dashboard/tests/formations/formations-cockpit.spec.ts dashboard/tests/formations/formations-smoke.spec.ts
git commit -m "feat(dashboard): share canonical run controller"
```

Before closing `.7`, require both scans to be empty outside tests/type declarations:

```bash
cd /srv/chrote-worktrees/formations-run-view
rg -n 'RunStatusProjection|RunStatusResult|fetchRunStatus|fetchRunEvents|statusFromRunEvent|runEventResumeAllowed|openHumanGateId\(' dashboard/src
rg -n 'setInterval\(.*1200|chrote-formations-active-run-' dashboard/src/components/FormationsCockpit.tsx dashboard/src/components/AgentsView.tsx
```

## Final integration review and umbrella closure

Do not begin this section until `.1` through `.7` are closed and `.superpowers/sdd/progress.md` names their reviewed commit ranges. The umbrella `ctx-7i1` remains open until every check and the whole-branch review below is clean.

### Step F.1: Audit exact scope and dependency completion

- [ ] Run `bd show ctx-7i1 --json` and confirm all seven named children are closed, `.5` and `.6` were treated as siblings, and no unrelated Bead was modified.
- [ ] Verify the certified Tool base `884deeec2c4d4ec2e220b7450dccdd6a10238ef5` is an ancestor. Use that exact commit for the whole-slice log/diff/review; do not substitute `git merge-base main HEAD`. Also retain the pre-implementation plan HEAD recorded before Task 1 for an implementation-only supplemental diff.
- [ ] Confirm the branch contains separate tests-only and production commits for each child. Review/fix commits may follow them, but no child may lack its RED artifact.
- [ ] Confirm no files under `/srv/chrote`, the certified Tool worktree, service units, tmux sockets, live data roots, board/persona stores, or local-storage fixtures were mutated during implementation.

Use:

```bash
cd /srv/chrote-worktrees/formations-run-view
formations_certified_base="884deeec2c4d4ec2e220b7450dccdd6a10238ef5"
git merge-base --is-ancestor "$formations_certified_base" HEAD
git log --oneline "$formations_certified_base"..HEAD
git diff --stat "$formations_certified_base"..HEAD
git status --short
```

### Step F.2: Run the complete backend gates

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -l internal/formations internal/api internal/comms cmd/archon
go test ./... -count=1
go test -race ./internal/formations ./internal/api ./internal/comms ./cmd/archon -count=1
go vet ./...
```

Expected `gofmt -l` output is empty. Format only named changed Go files if it reports a hit; do not mechanically rewrite unrelated files. Any failing or skipped relevant test blocks completion.

### Step F.3: Run the complete dashboard and built-server contract gates, then refresh the embed

```bash
cd /srv/chrote-worktrees/formations-run-view/dashboard
npm run test:unit
npm run lint
npm run build
npm run test:formations

cd /srv/chrote-worktrees/formations-run-view
scripts/build-embedded-dashboard.sh

cd /srv/chrote-worktrees/formations-run-view/src
ctx_7i1_evidence_dir="$(mktemp -d "${TMPDIR:-/tmp}/ctx-7i1-final.XXXXXX")"
printf '%s\n' "$ctx_7i1_evidence_dir"
go build -o "$ctx_7i1_evidence_dir/chrote-formations-run-view-server" ./cmd/server

cd /srv/chrote-worktrees/formations-run-view
if ! CHROTE_SERVER_BINARY="$ctx_7i1_evidence_dir/chrote-formations-run-view-server" \
  CHROTE_CONTRACT_ARTIFACT_DIR="$ctx_7i1_evidence_dir/contract-artifacts" \
  scripts/test-built-server-contract.sh \
  > "$ctx_7i1_evidence_dir/built-server-contract.log" 2>&1; then
  sed -n '1,240p' "$ctx_7i1_evidence_dir/built-server-contract.log"
  exit 1
fi
test -s "$ctx_7i1_evidence_dir/built-server-contract.log"
for server_log in \
  normal-schema1.log \
  normal-schema2-no-fallback.log \
  artifact-contract.log; do
  test -s "$ctx_7i1_evidence_dir/contract-artifacts/$server_log" || {
    echo "missing contract server log: $server_log" >&2
    exit 1
  }
done
test -s "$ctx_7i1_evidence_dir/contract-artifacts/authority-guard-probe.log" || {
  echo "missing authority guard probe log" >&2
  exit 1
}
grep -Fq \
  'FORMATIONS_GUARD_CONTRACT_ACCEPTED schema1Inspection=0 schema2Guarded=1 semanticProjection=false' \
  "$ctx_7i1_evidence_dir/contract-artifacts/authority-guard-probe.log"
sed -n '1,240p' "$ctx_7i1_evidence_dir/built-server-contract.log"
```

Record the exact `ctx_7i1_evidence_dir` in the progress ledger and final evidence;
keep the production binary, compiled test binaries, all three per-process logs,
the separate guard-probe binary/log, fixture copies, TestMain temporary state,
and contract artifacts only inside it.
`scripts/test-built-server-contract.sh` is the mandatory allowed isolated
two-server-kind, three-server-invocation contract lane, with one additional
non-server direct-guard probe:

1. It writes Task 3's exact same-run fallback/claim authority files, compiles
   `src/internal/formations/authority_guard_contract_test.go` only with
   `-tags formations_guard_contract`, and runs its one named test under the
   exact evidence-rooted `env -i` contract. The probe must call
   `GuardRuntimeWorkspaceAuthorityV1` over the same generated
   `formations-data` root/workspace, return nil, classify ledgers `0/1`, retain
   the complete all-false capability including `SemanticProjection:false`,
   leave both input snapshots unchanged, and write the frozen marker to
   `authority-guard-probe.log`. It is not a server or production API surface.
2. Only after that probe succeeds, it runs the supplied normally built binary
   against only the ordinary schema-1 fixture. That isolated process covers
   bounded list/detail, one event page with generation/source/cursor, finite SSE
   final cursor, paged Comms run room/export, and embedded assets, then exits.
3. It then invokes the same normal binary again with
   those separate workspace/runtime roots. Detail for
   `run_01KXNP6VY3227H78329V52CKF8` must return 503
   `RUNTIME_AUTHORITY_NON_AUTHORIZING` without schema-1 data. A list window that
   selects that candidate must fail the whole page with the same 503/code and no
   `data`, skipped/filtered candidate, empty success page, or partial runs.
   Both normal processes return 404 for exact
   `POST /api/__formations_contract/shutdown` through the existing API fallback;
   the production binary omits the test marker and never claims schema-1
   artifact success. The response does not expose the internal guard reason.
4. Under the same evidence directory, the script compiles
   `src/internal/api/formations_run_contract_server_test.go` with `go test -c`,
   launches only `TestFormationsRunArtifactContractServer` under the exact
   `env -i`/evidence-rooted HOME/TMPDIR contract from Task 4, and passes that URL to Playwright as
   `CHROTE_FORMATIONS_ARTIFACT_CONTRACT_URL`. This server registers the real
   production artifact handler against an in-memory reader/opener and covers
   verified artifact bytes plus every accepted success/error header. The script
   invokes exact nonce-protected
   `POST /api/__formations_contract/shutdown`, waits for graceful zero exit,
   verifies the TestMain session path remained below evidence-rooted TMPDIR,
   and fails on forced termination or fixture leakage into the normal binary.

Neither server lane is a deploy, live-service, authority-root, or
interactive-tmux action. Do not run another live-backend suite, start either
system service, restart `chrote-srv.service`, touch legacy `chrote.service`, or
alter an interactive tmux session. The generated embed directory is ignored
build output unless repository state says otherwise; inspect
`git status --short`.

### Step F.4: Run contract, hygiene, and deletion scans

```bash
cd /srv/chrote-worktrees/formations-run-view
python3 scripts/doc-lint.py
git diff --check 884deeec2c4d4ec2e220b7450dccdd6a10238ef5..HEAD

rg -n 'ReadRunEvents|ProjectRun\(|ListRuns\(|ProjectOpenEscalations' src/internal/api src/internal/comms src/cmd/archon dashboard/src
rg -n 'RunStatusProjection|RunStatusResult|fetchRunStatus|fetchRunEvents|statusFromRunEvent|runEventResumeAllowed' dashboard/src
rg -n 'ListRunIdentities\([^)]*\).*\[\]|\[\]CanonicalRunReadInput|while \(.*hasMore|while \(.*eventHasMore' src dashboard/src
rg -n 'json:"(path|socket|targetKey|token|prompt|capture|pane|baseline)"' src/internal/formations/run_projection*.go
rg -n 't\.Skip\(|describe\.skip|it\.skip|test\.skip' src/internal/formations src/internal/api src/internal/comms src/cmd/archon dashboard/src dashboard/tests
```

- [ ] The first two scans must have no production consumer hit. Compatibility definitions/tests may remain only when their adapter is derived from `RunView` and the review package explains the hit.
- [ ] The private-field scan must have no public projection JSON tag. Safe hashes/encodings are allowed only under their exact approved names.
- [ ] Confirm both ledger roles pass through the one
  `RunLedgerReadMaximumBytes` guard before `CanonicalInputDocument` creation,
  with stat-over-limit rejection before allocation and a same-handle
  limit-plus-one growth check. The focused at-limit/plus-one tests must prove no
  partial input/view and `ErrRunProjectionResourceLimit`/fixed 413 mapping.
- [ ] Confirm schema-1 recovery is unconditionally `live`/nil and that only
  schema 2 tests the exact deciding-`slot_result` and other interrupted
  predicates. Confirm a selected non-authorizing schema-2 list candidate aborts
  the whole page with fixed 503 before filtering or success-envelope output.
- [ ] `SafeArtifactRef.ref` is the sole allowed root-relative path-bearing
  value and remains resolved under `rootId` with no-follow. It does not permit a
  JSON member named `path`; every hit from the private-field scan still blocks.
- [ ] For the skip scan, compare against certified base `884deeec2c4d4ec2e220b7450dccdd6a10238ef5`. A new skip in relevant coverage blocks completion; pre-existing unrelated skips must be listed, not silently called passing.
- [ ] Search the branch diff for event reducers. The only semantic event-type switch is inside `ProjectCanonicalRun`; sanitizer variant dispatch and presentation-only safe-event formatting are not reducers and must not mutate status/actions/artifacts.
- [ ] Confirm `RuntimeAuthorityCapability.SemanticProjection` is still false and schema-2 receipt serving is unbound. Confirm schema-1 response keys remain preserved and compatibility is conveyed only by `RunView.source.compatibility`.
- [ ] Confirm no new ADR exists and the approved design/Task 5 still record the
  explicit `DECLINED` ruling for an Archon page-NDJSON ADR.

### Step F.5: Obtain independent whole-branch review

- [ ] Generate the final whole-slice package with `/home/perttu/skills/subagent-driven-development/scripts/review-package 884deeec2c4d4ec2e220b7450dccdd6a10238ef5 HEAD`. Optionally generate a second package from the recorded pre-implementation plan HEAD, but never use it instead of the certified-base package.
- [ ] Dispatch the most capable available independent reviewer using `requesting-code-review`. Give it the approved design, this plan, `.superpowers/sdd/progress.md`, the printed package, child review/evidence paths, and exact gate outputs.
- [ ] Require review of: sole-reducer architecture; source precedence/no fallback; `SemanticProjection:false`; aggregate 64 MiB schema-1/schema-2 ledger reads with stat-before-allocation plus limit-plus-one growth detection and no partial input/view; the complete 41-type public union and minimum exact 21/37 parity including the conditional Mission/isolated-Formation start and schema-1 sentinel artifact omission; schema-1 recovery always `live`/nil and exact schema-2 deciding-result/interrupted/redaction predicates; source-selected schema-1/schema-2 open-dispatch shapes, order, duplicates, and resumed carry; bounded 50-candidate/4 MiB run listing, exact order, and whole-page 503 on a selected non-authorizing claim; submitted receipt id/kind/hash binding; event-page generation/source identity; SSE one-snapshot closure; raw-message sanitization; session/action privacy; `SafeArtifactRef.ref` as the sole root-relative path-bearing exception under `rootId`/no-follow; optimistic artifact-open linearization plus `no-store`/`nosniff`; Archon page-NDJSON filter/error semantics and explicit no-ADR ruling; honest pointer-backed Comms completeness and strict run parser; the built contract's separate schema-1 and same-run no-fallback normal invocations including detail/list 503, exact authority hashes/paths, the tagged same-root direct-guard acceptance probe with nil/`0/1`/all-false/unchanged assertions, TestMain-safe `env -i`, exact test-only `POST /api/__formations_contract/shutdown` with normal-server API-fallback 404, test-only artifact fixture exclusion, and graceful shutdown; controller one-page/three-view/400-event atomic refresh; no raw consumer scans; and scope/no-UX drift.
- [ ] If findings exist, dispatch one fresh fixer with the complete Critical/Important list, named covering tests, and one appended report. Re-run covering gates and send the updated package to an independent re-review. Do not split findings across agents or dismiss a plan conflict without user resolution.
- [ ] Record every resolved finding and final approval in `.superpowers/sdd/progress.md`. All Minor findings must be explicitly fixed or accepted with rationale before closure.

### Step F.6: Attach final evidence and close only the umbrella

- [ ] Write `$ctx_7i1_evidence_dir/ctx-7i1-final-evidence.md` with the recorded evidence-directory path, child IDs/commits/reviews, exact gate commands and outputs, aggregate-ledger exact-limit/plus-one/no-partial evidence, schema-scoped recovery fixtures, whole-list-page 503 evidence, all three server invocation results/logs, `authority-guard-probe.log` and its exact nil/`0/1`/all-false/unchanged proof, same-run no-fallback authority-path/hash proof, exact shutdown method/path plus both normal-server API-fallback 404s, compiled-test-only/TestMain containment and production-binary exclusion proof, graceful shutdown proof, final review disposition, changed-file inventory, `SemanticProjection:false` proof, receipt-provider-unbound proof, explicit Archon no-ADR proof, and any accepted Minor item.
- [ ] Attach it with `bd comments add ctx-7i1 -f "$ctx_7i1_evidence_dir/ctx-7i1-final-evidence.md"`.
- [ ] Close the umbrella only now:

```bash
bd close ctx-7i1 --reason "All seven canonical run-view checkpoints completed RED-first; focused, race, dashboard, contract, and independent whole-branch review evidence attached."
```

- [ ] Run `bd show ctx-7i1 --json` once more and record the closed state in `.superpowers/sdd/progress.md` with `apply_patch`.
- [ ] Stop. Do not merge, push, deploy, restart, or clean another worktree. Use `finishing-a-development-branch` only when the user separately authorizes an integration choice.
