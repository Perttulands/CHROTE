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
- `since` is the last-consumed cursor and therefore an exclusive lower bound: eligible events satisfy `seq > since`. Accept `0..9007199254740991`. API requests without `limit` use `200`; explicit limits outside `1..200` fail. The limit counts canonical slots scanned, including omitted projection-only slots. The complete encoded `RunEventPage` is at most 1 MiB.
- Sanitization is projection-time allowlisting. Unknown or unsafe event types and fields fail closed unless the design explicitly marks them projection-only. Never recursively copy raw maps.
- Schema 2 remains non-authorizing, and this branch must leave `RuntimeAuthorityCapability.SemanticProjection` false. A later integration may enable it only after the complete guarded `ctx-ug7.6.1` provider binds this exact projector and all other required authority checks pass. Never use schema 1 as a fallback after a schema-2 claim.
- A `RunCommandReceipt` comes only from a terminal command receipt provider. Schema-1 mutations return an explicitly labeled compatibility result and must never manufacture or imply a receipt.
- Artifact serving is by stable `(runId, artifactId)` identity through the canonical verified-artifact seam. Never expose stored paths and never reopen a pathname after validation.
- Do not add visual redesign, new orchestration behavior, migrations, feature flags, speculative abstractions, or an ADR. The approved design records no new architectural decision.
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

### Archon and Comms

- Create `src/cmd/archon/run_projection_test.go` and modify `src/cmd/archon/main.go` plus its focused tests.
- Create `src/internal/comms/run_projection_test.go` and modify `src/internal/comms/projection.go` plus `projection_test.go`.

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
| Bounded `RunEventPage` | `ctx-7i1.1` | cursor, scan, limit, 1 MiB tests |
| Authority, privacy, sessions, recovery, artifacts | `ctx-7i1.2` | adversarial security tests |
| Run list/detail/status/receipts/escalations API | `ctx-7i1.3` | HTTP contract tests |
| Events, SSE, artifact-open API | `ctx-7i1.4` | HTTP/SSE/descriptor tests |
| Archon canonical consumer | `ctx-7i1.5` | CLI contract and no-raw-read tests |
| Comms canonical consumer | `ctx-7i1.6` | room/message projection tests |
| Shared dashboard controller | `ctx-7i1.7` | unit, component, and Playwright tests |

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
	RunPageDefaultLimit = 200
	RunPageMaximumLimit = 200
	RunPageMaximumBytes = 1 << 20
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
	Source CanonicalRunSource
	Record []byte
}

type CanonicalRunAuthorityReader interface {
	ReadRun(runID string) (CanonicalRunReadInput, error)
	ListRuns(filter RunListFilter) ([]CanonicalRunReadInput, error)
	ReadCommand(commandID string) (CanonicalCommandReadInput, error)
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
func (s *Store) ListRunViews(filter RunListFilter) ([]RunView, error)
```

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
- [ ] Prove `ProjectRunEventPage` returns ascending safe events, treats `since` as last-consumed, advances across projection-only omissions, reports `hasMore` from the immutable projection, and never mutates its input.
- [ ] Request a `since` greater than the immutable projection's latest sequence and assert an empty page with `cursor == since` and `hasMore:false`; the selector must not regress to `latestSeq`.
- [ ] Prove `limit` values `0` and `201` fail, `1` and `200` work, and `since > MaxJSONSafeInteger` fails.
- [ ] Build complete `RunEventPage` candidates whose exact JSON encodings are one byte below/at/above the 1 MiB boundary. Assert a nonempty page stops before the next event without consuming it, while one individually oversized safe event returns the typed resource-limit error.
- [ ] Prove `ReadCanonicalRun` invokes `CanonicalRunAuthorityReader.ReadRun` exactly once by injecting a counting reader seam; the reader may perform its required bounded OS reads, while both selectors over the returned projection perform zero reader or filesystem calls.
- [ ] Add exact applied and rejected `CanonicalCommandReadInput` fixtures for start/resume/cancel/verdict. Prove `ProjectCommandReceipt` preserves the frozen two-arm union, accepts a rejected start without a run, rejects `pending`, and rejects any required/forbidden-field mismatch.
- [ ] Add schema-1/schema-2 source-selection fixtures. A claimed-but-invalid schema-2 input fails without consulting schema 1. Assert `SemanticProjection` remains false even when projector fixtures pass.

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

- [ ] Define the wire structs exactly from the approved design: `RunView`, `RunIdentity`, `RunNodeView`, `RunAttemptView`, `RunGateView`, `RunOutputView`, `ArtifactProjection`, `RunBlockView`, `RunEscalationView`, `RunSessionView`, `RunAction`, `CoordinatorReconcileCondition`, `RunAudit`, `SafeRunEvent`, `RunEventPage`, and the exact two-arm `RunCommandReceipt`.
- [ ] Preserve JSON-safe sequence values as `uint64` in Go but validate before projection and serialize them as ordinary JSON numbers only after validation.
- [ ] Sort and validate the immutable event input once. Reduce every semantic field in the single `ProjectCanonicalRun` switch. Materialize the final structural view and sanitized event stream into the private result.
- [ ] During whole-projection validation, encode every emitted safe event in a singleton complete `RunEventPage` with its real run ID/cursor and `hasMore:false`. Reject any singleton over 1 MiB before returning `CanonicalRunProjection`. This is the one no-partial-response guarantee used by every adapter.
- [ ] Make `ProjectRunView` a defensive-copy selector only. It must contain no switch on event type and no file access.
- [ ] Make `ProjectRunEventPage` a cursor/limit/encoded-byte selector only. It must contain no semantic event interpretation.
- [ ] Count the exact `encoding/json` bytes of the complete candidate `RunEventPage`, including schema, run ID, cursor, `hasMore`, and events; do not estimate from raw records or encode only the array. Reject an individual safe event that cannot fit in an otherwise empty complete page.
- [ ] Set `cursor` to the greatest canonical sequence scanned, including projection-only omissions. Stop after `limit` canonical candidates, not `limit` emitted events. If the page stops before a candidate for byte reasons, do not consume that candidate sequence. Set `hasMore` from remaining canonical sequences.
- [ ] Implement `CanonicalRunAuthorityReader` as the immutable-read seam. Every `CanonicalInputDocument` owns a copied byte slice, has a closed role, and carries its verified SHA-256; the reader performs physical/integrity checks, while the sole reducer performs semantic decoding. `ReadCanonicalRun` invokes the reader then `ProjectCanonicalRun` exactly once. The pure reducer has no clock input.
- [ ] Re-implement `ProjectRun`, `ListRuns`, `ProjectRunNodeReport`, and `ProjectOpenEscalations` as narrow schema-1 compatibility adapters over `RunView`. Do not leave their old event reducers in place.
- [ ] Implement `ProjectCommandReceipt` as a separate pure terminal-record decoder. It has no run read, never accepts `pending`, and shares no status inference with `ProjectCanonicalRun`.

Implement the page loop in this exact order: validate `since` and `limit`; initialize schema/run id/cursor=`since`/`hasMore` from the projection; skip stored candidates with `scanSeq <= since` without counting them; before each eligible candidate stop if `scanned == limit`; form a defensive-copy candidate event list (unchanged for an omitted event); form the complete candidate page with cursor at that candidate and `hasMore` based on any later canonical sequence; encode that complete page; on overflow return the typed resource-limit error only when the candidate is a safe event and the accepted event list is empty, otherwise stop without accepting/counting/advancing the candidate; on success accept the list/cursor and increment `scanned` even when omitted; finally return the last accepted complete page. Do not default `limit == 0` inside the selector; the HTTP parser owns the absent-query default and passes `200` explicitly.

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
- Modify: `src/internal/formations/run_projection.go`
- Modify: `src/internal/formations/run_projection_events.go`
- Modify: `src/internal/formations/run_artifacts.go`
- Modify: `src/internal/formations/runtime_authority.go`
- Modify: `src/internal/formations/authority_guard.go`
- Modify: `src/internal/formations/authority_guard_test.go`
- Modify: `src/internal/formations/run_artifact_security_test.go`

**Interfaces fixed by this task:**

```go
const RunArtifactOpenMaximumBytes = uint64(64 << 20)

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

`VerifiedRunArtifact` is safe response material, not a path container. It must not expose `rootId`, `ref`, a file descriptor, socket data, or a private authority locator.

### Step 2.1: Write adversarial authority and privacy tests

- [ ] Construct schema-2 authority fixtures for every required private input role. Prove projector-readiness checks can pass while the public capability and runtime store remain non-authorizing with `SemanticProjection:false`.
- [ ] Prove a schema-2 claim with a missing, duplicate, mislinked, stale-fence, oversized, invalid-JSON, or wrong-run input returns the exact typed guard/projection error and never falls back to schema 1.
- [ ] Prove schema-1 is selected only when no canonical schema-2 claim exists, or when the test explicitly constructs the offline compatibility store.
- [ ] For each schema-2 public event decoder, add one accepted exact payload, one unexpected top-level field, one unexpected `data` field, one invalid enum/id/hash, and one over-bound string. Unexpected authority-bearing material must reject the whole projection.
- [ ] Add registered projection-only fixtures and prove they affect only scan count/cursor, never structural fields, actions, artifacts, bindings, or event output.
- [ ] Assert `RunSessionView` JSON omits socket/server routes, `targetKey`, paths, raw session lookup identity, prompt/capture/pane/input bytes, exact history/baseline tokens, and capabilities. Also assert the required hashes, closed states, and opaque `sessionTargetId` remain.
- [ ] Assert actions arise only from their ledgered preconditions. A status label, recovery state, local value, persona, or observed tmux session must not create cancel/resume/verdict/peek.
- [ ] Add all five interrupted-finalization fixtures and an unresolved-redaction fixture. Assert precedence is `pending-redaction`, then `interrupted-finalization`, then `live`; only non-live states get `coordinator-reconcile`; neither state creates an action.
- [ ] Register an artifact as available, cite it from an early event, later revoke/redact/expire it, then assert both structural and historical event occurrences expose only the latest unavailable projection.
- [ ] In the artifact-open fixture, replace/revoke the descriptor after the first projection and after the one allowed open. Assert the second projection rejects and the code never reopens the path. Also cover symlink, non-regular file, media mismatch, size mismatch, hash mismatch, and over-bound bytes.
- [ ] Treat the 64 MiB buffered-open ceiling as a new engineering plan-review proposal for bounded memory in this simple verified-buffer implementation, not as a frozen artifact-contract value or an existing Files-read policy. Test exact-size success at the ceiling and typed resource-limit rejection above it before allocation/response. If the Task 2 RED reviewer requires the same verified handle to be seeked/streamed instead, amend/re-review the design and plan before production code.

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
go test ./internal/formations -run 'TestRunProjectionAuthority|TestRunProjectionSanitization|TestRunProjectionSession|TestRunProjectionAction|TestRecoveryState|TestArtifactHydration|TestOpenVerifiedRunArtifact' -count=1
```

Expected RED: the projector currently accepts unsafe/private fixtures or lacks the verified-open surface. Commit tests only, then obtain `APPROVED_RED`:

```bash
git add src/internal/formations/run_projection_security_test.go src/internal/formations/authority_guard_test.go src/internal/formations/run_artifact_security_test.go
git commit -m "test(formations): specify run projection security"
```

### Step 2.2: Bind authority and implement fail-closed sanitization

- [ ] Complete the schema-1 production reader and the injectable schema-2 fixture reader against the closed role/cardinality table. Preserve descriptor-relative/no-follow reads, per-record/per-event 1 MiB guards, and the JSON-safe maximum. The production runtime boundary may inspect/guard a schema-2 claim but must return typed non-authorizing/unavailable instead of constructing a complete schema-2 projection input until `ctx-ug7.6.1` binds the missing provider.
- [ ] Keep `RuntimeAuthorityCapability.SemanticProjection` false in this branch under every fixture, including complete projector fixtures. Expose an internal fixture-testable projector-readiness result, but do not wire it into capability derivation. Only the later integration of the complete `ctx-ug7.6.1` guarded provider plus this exact projector may enable the existing capability.
- [ ] Keep `RequireRuntimeAuthority` typed and non-authorizing. A schema-2 claim failure must surface its exact safe code; it must not call the schema-1 reader.
- [ ] Decode schema-2 event envelopes directly from the reader's immutable `CanonicalInputDocument.Bytes`, with payload retained as `json.RawMessage`. Apply `json.Decoder.DisallowUnknownFields` at both envelope and event-specific payload decode. Never decode schema 2 through legacy `RunEvent.Data map[string]any`, because unknown keys would already be lost or weakly typed.
- [ ] Register projection-only types in a separate table that records their redaction class and permits no semantic reducer callback.
- [ ] Apply field-specific length, enum, identifier, hash, and fixed-template validation before values enter the private projection. Replace raw adapter errors with registered safe codes.
- [ ] Derive sessions and actions from verified canonical binding/occupancy/capability state. Live tmux is not an input to this task.
- [ ] Implement recovery predicates from replay state in the sole reducer. Evaluate pending redaction last in code but apply it as the final highest-precedence override so the precedence is unambiguous and tested.
- [ ] Hydrate every artifact occurrence after reduction from a map keyed by stable `artifactId`; never retain an earlier readable descriptor in an event.

- [ ] Represent public sanitizers and projection-only classifications as two explicit closed registries. The public registry contains exactly these frozen event types, each mapped to one typed decoder: `run_started`, `run_activated`, `run_resumed`, `node_waiting`, `node_input_ignored`, `node_started`, `slot_binding_observed`, `slot_dispatch`, `slot_peek_capability_issued`, `slot_steering_started`, `slot_steering_ended`, `slot_peek_capability_revoked`, `slot_reconciliation_interrupt`, `slot_reconciliation_interrupt_outcome`, `slot_result`, `formation_result`, `tool_dispatch`, `tool_process_launch`, `tool_result`, `node_output`, `gate_evaluating`, `gate_kind_result`, `judge_result`, `judge_attempt_failed`, `gate_verdict`, `artifact_attached`, `artifact_observed`, `escalation_raised`, `human_input_requested`, `human_verdict_recorded`, `error`, `run_blocked`, `run_cancel_requested`, `run_canceled`, `run_failure_reconciliation_started`, `run_failed`, and `run_succeeded`.
- [ ] Projection-only types are accepted only through a closed code-owned registry selected by a supported authority schema; event bytes cannot self-register. Production has no extension entry in this branch. Tests inject the exact test-only entry `test_projection_only_redacted` with omit-all redaction classification from `_test.go`, and production code rejects the reserved `test_` prefix. There is no default sanitizer or name-only exemption. The reducer checks the selected projection-only registry first, the exact public registry second, and otherwise returns the typed unknown-authority-event error.

### Step 2.3: Implement optimistic artifact opening

- [ ] Project C1 and select the exact available `ArtifactProjection` by `artifactId`; require equal top-level and nested IDs.
- [ ] Open its descriptor exactly once using the existing root-relative no-follow primitives. Reject `sizeBytes > RunArtifactOpenMaximumBytes`; validate regular identity, media type, and exact stat size; read at most `sizeBytes+1` bytes from that same handle; require exactly `sizeBytes`; and validate SHA-256 over those bytes.
- [ ] Project C2 from current authority. Require P2 to be field-for-field equal to P1, including the entire `SafeArtifactRef`.
- [ ] Return only the already verified bytes and safe metadata after C2 succeeds. On any error or mismatch, discard the buffer. Never reopen the descriptor and never append an observation from the read path.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/formations/run_projection.go internal/formations/run_projection_events.go internal/formations/run_projection_artifacts.go internal/formations/run_projection_security_test.go internal/formations/run_artifacts.go internal/formations/runtime_authority.go internal/formations/authority_guard.go internal/formations/authority_guard_test.go internal/formations/run_artifact_security_test.go
go test ./internal/formations -run 'TestRunProjectionAuthority|TestRunProjectionSanitization|TestRunProjectionSession|TestRunProjectionAction|TestRecoveryState|TestArtifactHydration|TestOpenVerifiedRunArtifact|TestGuardRuntimeAuthority' -count=1
go test -race ./internal/formations -run 'TestRunProjection|TestRecoveryState|TestOpenVerifiedRunArtifact|TestGuardRuntimeAuthority' -count=1
```

Expected GREEN: all authority, privacy, and race tests pass; no schema-2 fixture reaches schema 1. Commit production separately:

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
- Modify: `src/internal/formations/run_projection.go`
- Modify: `src/internal/formations/run_projection_test.go`

**Preserved mutation response shapes:**

```text
schema-1 start:                  data = { runId, status: RunView }
schema-1 resume/cancel/verdict: data = { status: RunView }
schema-2 terminal command:      data = { receipt: RunCommandReceipt }
```

This branch does not invent another public schema or union. Schema-1 compatibility is explicitly and canonically labeled by `status.source.compatibility === true` in the preserved response shape. Schema-2 receipt serving remains unwired until the independent provider binds; its adapter is fixture-tested only.

### Step 3.1: Write HTTP and terminal-receipt adapter tests

- [ ] Assert `GET /api/formations/runs` returns the existing success envelope with `data.runs` as `RunView[]`, stable order, no raw statuses, and no event history.
- [ ] Assert `GET /api/formations/runs/{runId}` preserves the existing success envelope and `data.status` key, with one canonical `RunView` as that value.
- [ ] Assert `GET /api/formations/runs/{runId}/escalations` returns `data.escalations` copied from that same view, with no independent ledger scan.
- [ ] Inject the terminal command-reader seam for applied start/resume/cancel/verdict and rejected command fixtures. Assert the adapter would return exactly `data.receipt: RunCommandReceipt`; keep that production seam unwired and capability-disabled.
- [ ] Inject a schema-2 `pending` outcome and assert the endpoint returns the registered typed not-terminal error, not a receipt or status guess.
- [ ] Run each schema-1 mutation and assert the response preserves the current `runId`/`status` keys, replaces the old status value with `RunView`, and has `status.source.compatibility === true`. Assert no `receipt` key is present.
- [ ] Assert a schema-1 error never becomes a rejected receipt. Assert a schema-2 rejected-start receipt has no `runId` or `effectSeq`.
- [ ] Assert unsafe projection errors use `writeFormationsError`, return no partial `RunView`, and preserve the registered HTTP status/code without raw paths or adapter errors.
- [ ] Add a spy canonical store to prove list/detail/escalations call `ListRunViews` or `ReadRunView`, never `ProjectRun`, `ReadRunEvents`, or `ProjectOpenEscalations`.

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
	if response.Error.Code != "formations_command_not_terminal" { t.Fatalf("code = %q", response.Error.Code) }
	if strings.Contains(response.Body.String(), `"receipt"`) { t.Fatalf("pending became receipt: %s", response.Body.String()) }
}
```

Run RED:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
go test ./internal/api ./internal/formations -run 'Test(Formations.*RunView|ListRunsCanonical|GetRunCanonical|RunEscalationsCanonical|.*Run.*Receipt|.*Run.*Compatibility|.*Run.*Pending)' -count=1
```

Expected RED: old endpoints expose `RunStatusProjection`, direct escalation scans remain, and the terminal-receipt adapter is absent. Commit tests only and obtain `APPROVED_RED`:

```bash
git add src/internal/api/formations_run_projection_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go src/internal/api/formations_runtime_authority_test.go src/internal/formations/run_projection_test.go
git commit -m "test(api): specify canonical run responses"
```

### Step 3.2: Route structural endpoints through the canonical view

- [ ] Add exactly `GET /api/formations/runs`. Accept at most one optional `board` query value, resolve it with the existing board-selector resolver, reject empty/duplicate/unknown query keys, and call `ListRunViews(RunListFilter{BoardSlug: resolved})`; an absent query lists all canonical runs.
- [ ] Change detail to `ReadRunView`. Change escalations to select `view.Escalations`. Remove handler-level event reduction.
- [ ] Preserve the repository's `core.WriteSuccess` envelope rather than adding a second response envelope. List uses `map[string]any{"runs": views}`; detail preserves `map[string]any{"status": view}`; escalations preserve `map[string]any{"escalations": view.Escalations}`.
- [ ] Map projection guard, invalid-input, not-found, resource-limit, and authority errors through `writeFormationsError`. Do not return a partial list if any claimed run fails projection.

### Step 3.3: Bind terminal receipt input without fabricating it

- [ ] Define a narrow private terminal command reader at the mutation adapter boundary. Its input is the normalized submitted command identity; its output is `CanonicalCommandReadInput` containing the already durable terminal record.
- [ ] Invoke Task 1's `ProjectCommandReceipt` and wrap the result only as `map[string]any{"receipt": receipt}`. Fixture-test command kinds, canonical hashes, JSON-safe sequences/fence, required/forbidden fields per union arm, start-only policy reference, and rejected start without a run.
- [ ] Keep the production reader nil/unavailable and schema-2 receipt serving disabled in this branch. Missing provider returns the typed authority-unavailable error; `pending` returns the typed not-terminal error.
- [ ] For schema 1, read a fresh canonical view after mutation and preserve the existing keys: start returns `map[string]any{"runId": view.RunID, "status": view}`; resume/cancel/verdict return `map[string]any{"status": view}`. Assert `view.Source.Compatibility` is true and never add a `receipt` member.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/api/formations.go internal/api/formations_run_projection_test.go internal/api/formations_test.go internal/api/formations_acceptance_test.go internal/api/formations_runtime_authority_test.go internal/formations/run_projection.go internal/formations/run_projection_test.go
go test ./internal/api ./internal/formations -run 'Test(Formations.*RunView|ListRunsCanonical|GetRunCanonical|RunEscalationsCanonical|.*Run.*Receipt|.*Run.*Compatibility|.*Run.*Pending)' -count=1
go test -race ./internal/api ./internal/formations -run 'Test(Formations.*Run|.*Run.*Receipt|.*Run.*Compatibility)' -count=1
```

Expected GREEN: all canonical endpoint and compatibility-shape tests pass, schema-2 adapter fixtures pass, and live schema-2 receipt serving remains disabled without the independent provider. Commit production separately:

```bash
git add src/internal/api/formations.go src/internal/api/formations_run_projection_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go src/internal/api/formations_runtime_authority_test.go src/internal/formations/run_projection.go src/internal/formations/run_projection_test.go
git commit -m "feat(api): serve canonical formations run views"
```

## Task 4 (Milestone 3): `ctx-7i1.4` — Bounded events, replay-only SSE, and artifact-open API

**Depends on:** `ctx-7i1.2`, `ctx-7i1.3`

**Files:**

- Create: `src/internal/api/formations_run_events_test.go`
- Modify: `src/internal/api/formations.go`
- Modify: `src/internal/api/formations_test.go`
- Modify: `src/internal/api/formations_acceptance_test.go`
- Modify: `src/internal/formations/run_projection.go`
- Modify: `src/internal/formations/run_projection_events.go`

**Routes:**

```text
GET /api/formations/runs/{runId}/events?since=<JsonSafeInteger>&limit=<1..200>
GET /api/formations/runs/{runId}/stream
GET /api/formations/runs/{runId}/artifacts/{artifactId}
```

The events endpoint returns one `RunEventPage` inside the existing success envelope. SSE is a finite replay from one immutable `CanonicalRunProjection`, then a cursor control event, then EOF. Artifact open returns only the already verified bytes from `OpenVerifiedRunArtifact`.

**Engineering plan-review decision:** the existing SSE route `/api/formations/runs/{runId}/stream` is preserved. No artifact-read route or artifact response-header policy exists today, and the canonical contracts freeze only `(runId, artifactId)` identity, not HTTP spelling. This plan proposes `GET /api/formations/runs/{runId}/artifacts/{artifactId}`; success uses the second verified projection's media type as `Content-Type`, sets exact `Content-Length`, and deliberately omits `Content-Disposition`. The Task 4 RED reviewer must explicitly accept or reject this route/header shape before production code; rejection requires a design/plan amendment, not an improvised alias.

### Step 4.1: Write paging, SSE, and artifact HTTP tests

- [ ] Test query absence (`since=0`, `limit=200`), minimum/maximum values, negative text, signs, decimals, overflow, duplicate values, empty values, and explicit `limit=0/201`. Parse with unsigned integer functions, never float JSON logic.
- [ ] Assert one events request causes one `ReadCanonicalRun`, returns exactly one `RunEventPage`, and cannot exceed 1 MiB before the fixed success wrapper.
- [ ] Assert a projection-only tail advances the returned cursor and can produce an empty page with `hasMore:false`.
- [ ] Assert an individually oversized sanitized event maps to the registered resource-limit HTTP error with no partial page.
- [ ] For SSE, inject more than 200 events plus a projection-only tail. Assert the store is read once; events remain ascending and sanitized; each event's `id` equals its canonical sequence; no full replay array is constructed; the final frame is `event: cursor` with equal id/data high water; and the response closes.
- [ ] Preserve existing resume precedence: a present `Last-Event-ID` overrides query `since`. Test the combination exactly once and reject malformed/unsafe cursor values before writing SSE headers.
- [ ] Send a JSON-safe `Last-Event-ID` greater than the snapshot's latest sequence. Assert no safe event frame, then exactly one final cursor control frame whose id and one-field data both retain that future value; never regress it to the snapshot latest sequence.
- [ ] Assert SSE never loops, sleeps, polls a second snapshot, or watches tmux. Use a reader spy and a response recorder; no timing assertion.
- [ ] For artifact open, assert exact available bytes, `Content-Type` from the second verified projection, exact `Content-Length`, and no `Content-Disposition` header. Assert unavailable/redacted/expired/missing/mutated/symlink/hash/size failures return no bytes and neither successful nor failed responses leak a stored path.

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
```

Expected RED: the old events endpoint and SSE read an unbounded raw-event slice, and no canonical artifact-open route exists. Commit tests only and obtain `APPROVED_RED`:

```bash
git add src/internal/api/formations_run_events_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go
git commit -m "test(api): specify bounded run event transport"
```

### Step 4.2: Implement strict event query parsing and one-page response

- [ ] Add `parseRunEventPageRequest(*http.Request) (since uint64, limit int, err error)`. Accept each parameter at most once. Default an absent `since` to `0` and absent `limit` to `RunPageDefaultLimit`; reject present empty values.
- [ ] Parse both values with the digits-only `strconv.ParseUint` helper below. Enforce `since <= MaxJSONSafeInteger` and `limit` in `1..RunPageMaximumLimit` before converting limit to `int`.
- [ ] Call `ReadRunEventPage` once and send the result through `core.WriteSuccess`. Preserve typed error mapping.

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
- [ ] Loop `ProjectRunEventPage(projection, cursor, 200)`. Write each event immediately as one SSE frame and check every write/flush error; do not append it to an all-events slice.
- [ ] When a page is empty but advances across omissions, continue from its cursor. Add a progress guard: if `hasMore` is true and cursor did not advance, return the registered internal projection error.
- [ ] After `hasMore:false`, emit exactly one transport-only frame with `event: cursor`, `id: <cursor>`, and data containing exactly `{"cursor":<cursor>}` with no other members; flush and return. Do not start a timer or second read.

The Task 1 reducer's singleton-full-page validation guarantees that an oversized individual event fails before this adapter writes SSE headers. Do not add a second transport preflight or retain all pages.

### Step 4.4: Add optimistic artifact-open adapter

- [ ] Register exactly `GET /api/formations/runs/{runId}/artifacts/{artifactId}` and validate both IDs with existing safe-ID rules.
- [ ] Call `OpenVerifiedRunArtifact` once. Set `Content-Type` from the media type field authorized by the successful second projection, set exact `Content-Length`, and emit no `Content-Disposition` header in this slice.
- [ ] Write only `VerifiedRunArtifact.Bytes`. Never resolve a path in the API package and never call `os.Open` from the handler.
- [ ] Map authorization change, unavailable state, invalid descriptor, verification mismatch, not found, and resource limit through `writeFormationsError` before writing a body.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/api/formations.go internal/api/formations_run_events_test.go internal/api/formations_test.go internal/api/formations_acceptance_test.go internal/formations/run_projection.go internal/formations/run_projection_events.go
go test ./internal/api -run 'Test(GetRunEventsPage|GetRunEventsQuery|StreamRunEvents|OpenRunArtifact)' -count=1
go test -race ./internal/api ./internal/formations -run 'Test(GetRunEvents|StreamRunEvents|OpenRunArtifact|ProjectRunEventPage|OpenVerifiedRunArtifact)' -count=1
```

Expected GREEN: all event, SSE, and artifact route tests pass without sleeps or retries. Commit production separately:

```bash
git add src/internal/api/formations.go src/internal/api/formations_run_events_test.go src/internal/api/formations_test.go src/internal/api/formations_acceptance_test.go src/internal/formations/run_projection.go src/internal/formations/run_projection_events.go
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

**Engineering plan-review decision:** today `run logs --json` writes one JSON array of raw events, while `run follow --json` writes one raw event per NDJSON line. This plan deliberately changes both to one complete versioned `RunEventPage` per NDJSON line so each transferred unit is bounded and carries cursor/`hasMore` semantics. That is a breaking public agent/CLI wire change, not an internal refactor or visual UX change. The Task 5 RED reviewer must explicitly accept this before tests freeze it; rejection requires a design/plan amendment or a separately versioned CLI mode, not a silent compatibility wrapper that rebuilds an unbounded array.

**Command contract:**

- `run list --json` writes `{runs: RunView[]}`; `run status --json` writes one `RunView`.
- `run logs --json` and `run logs --follow --json` write one complete `RunEventPage` per NDJSON line. Each line independently preserves `formations.run-events.v1`; no all-events JSON array is rebuilt.
- `run follow --json` likewise writes one `RunEventPage` per NDJSON line. Text modes stream the safe events within each page.
- Schema-1 mutation JSON preserves existing keys with a canonical compatibility `RunView` status. A bound canonical provider writes `{receipt: RunCommandReceipt}`. This branch fixture-tests the latter but leaves it unavailable without `ctx-ug7.6.1`.

### Step 5.1: Write CLI contract and paging tests

- [ ] Add list/status JSON golden fixtures using the exact `RunView` contract and text fixtures using safe identity/audit fields.
- [ ] Add 201-event logs/follow fixtures with a projection-only slot at a page edge. Assert every JSON line decodes as `RunEventPage`, each page scans no more than 200 canonical slots, cursors strictly advance, omitted slots still advance, and concatenated visible event sequences are ordered without duplicates.
- [ ] Apply `--node` filtering after page selection. Assert a page whose visible events are all filtered still advances the canonical cursor and cannot stall follow.
- [ ] Assert `--since` rejects negative, signed, decimal, overflow, and above-JSON-safe values; accept `0` and the maximum. Change the flag storage to an exact unsigned-decimal string parser rather than `flag.Int`.
- [ ] Prove each follow poll calls `ReadCanonicalRun` once, consumes all bounded pages from that snapshot, checks finality from its `RunView`, and only then sleeps before a later snapshot. Use an injected sleeper; tests must not wait on wall time.
- [ ] Assert `run ask` uses `RunView.Nodes`, `Attempts`, `Gates`, `Outputs`, `Blocks`, `Escalations`, and `Artifacts`; it must not call an event or escalation ledger reader.
- [ ] Assert schema-1 start/resume/abort/verdict JSON retains its current `runId`/`status` keys, the status is a `RunView` with `source.compatibility:true`, and no receipt appears.
- [ ] Feed applied/rejected `CanonicalCommandReadInput` through the Archon receipt formatting adapter. Assert exact receipt JSON, including rejected start without run. Assert the production runtime returns authority unavailable while the provider is unbound.
- [ ] Assert all errors use existing `failJSON`/stream-safe handling and contain no raw path, private authority, tmux route, or raw payload.

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
	for {
		page, err := formations.ProjectRunEventPage(projection, cursor, formations.RunPageMaximumLimit)
		if err != nil { return cursor, err }
		if err := visit(page); err != nil { return cursor, err }
		if page.HasMore && page.Cursor == cursor { return cursor, errRunPageCursorStalled }
		cursor = page.Cursor
		if !page.HasMore { return cursor, nil }
	}
}
```

Define `errRunPageCursorStalled` as a static safe error in `main.go` and map it through the existing CLI error path.

### Step 5.3: Migrate list, status, logs, follow, and ask

- [ ] Change list to `ListRunViews` and status to `ReadRunView`. Text output reads board slug from `view.Identity`, count from `view.Audit`, and resumability from `view.Actions`; it does not infer state from event types.
- [ ] For non-follow logs, call `ReadCanonicalRun` once and pass it to the iterator. JSON visits write each whole page with `writeNDJSON`. Text visits filter and render only `SafeRunEvent` values while retaining the page cursor.
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

This task is independent of Task 5 and may be implemented/reviewed in parallel only when separate agents and non-overlapping files are available. It must not wait on an Archon commit to claim a false dependency.

**Engineering plan-review decision:** existing Formations `ProjectRoom` materializes the complete raw run ledger, and `RoomMessages` has no `hasMore`. This plan bounds `ProjectRoom.Messages` to the first canonical 200-slot page and adds the backward-compatible JSON member `hasMore` to paged `RoomMessages`; run-room export preserves its existing effective 200-message cap. The Task 6 RED reviewer must explicitly accept this truncation/paging contract before freezing tests. Non-run rooms and their export behavior remain unchanged.

### Step 6.1: Write run-room canonical source and cursor tests

- [ ] Build one canonical run fixture containing safe messages, a projection-only gap, a blocked node, an escalation, an available artifact later revoked, and private raw event keys.
- [ ] Assert `ProjectRoom("run:<id>")` identifies `Source.Kind` as `formations-run-view`, derives status/finality/board/Mission/Bead/event count from `RunView`, and includes only the first bounded canonical page of safe messages.
- [ ] Assert run-room claims, summary, artifacts, and risks derive from typed view/page members. The unavailable latest artifact appears at most as unavailable metadata; its earlier readable ref and private raw metadata never reappear.
- [ ] Assert `Messages` with `Since` and `Limit` invokes `ProjectRunEventPage` semantics directly, returns `NextSince == page.Cursor`, exposes `HasMore`, and advances when a page emits no message because every scanned slot is omitted or filtered.
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
	if page.NextSince != 2 || !page.HasMore {
		t.Fatalf("cursor = %d, hasMore = %v", page.NextSince, page.HasMore)
	}
}
```

Run RED:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
go test ./internal/comms -run 'TestRun(Room|Messages|Artifacts|Risks|Projection).*Canonical|TestRunMessagesAdvance' -count=1
```

Expected RED: `projectRunRoom` reads raw events and legacy status, and `Messages` pages an already materialized unbounded message slice. Commit tests only and obtain `APPROVED_RED`:

```bash
git add src/internal/comms/run_projection_test.go src/internal/comms/projection_test.go
git commit -m "test(comms): specify canonical formations run rooms"
```

### Step 6.2: Split run-room reads from non-run ledgers

- [ ] At `ProjectRoom`, preserve every non-run branch exactly. Route only `run:<runId>` to a canonical helper.
- [ ] The run helper calls `ReadCanonicalRun` once, selects `RunView`, and selects one page with `since=0`, `limit=200`. It never calls the generic `readEvents` function.
- [ ] Set source fields from typed view fields: status/final directly; board/Mission/Bead from `Identity`; count from `Audit.ConsumedEventCount`; `Kind:"formations-run-view"`; `ReadOnly:true`.
- [ ] Convert messages only from `SafeRunEvent` variants. Use per-variant safe fields; no fallback formatting of arbitrary `data`, `fmt.Sprint(map)`, or raw JSON.
- [ ] Derive artifacts from latest `view.Artifacts`. Derive risks from `view.Blocks` and `view.Escalations`. Derive summary from view/node/Gate finality plus those typed room values.

### Step 6.3: Page run messages at the canonical source

- [ ] Change `RoomMessage.Seq`, `MessageOptions.Since`, `RoomMessages.NextSince`, `RoomSource.EventCount`, and `RoomSummary.EventCount` from `int` to `uint64` so canonical JSON-safe sequences/counts never narrow. For non-run raw events, reject a negative sequence before checked conversion; convert existing non-negative `len` counts explicitly; JSON output for valid existing fixtures is unchanged.
- [ ] Add `HasMore bool \`json:"hasMore"\`` to `RoomMessages`. Preserve all existing members.
- [ ] For a run room, validate `Since <= MaxJSONSafeInteger` and `Limit` as `1..200`; use `200` only when the caller leaves limit zero under the existing default convention.
- [ ] Read one `CanonicalRunProjection`, select exactly one page, map its safe events to messages, and return `NextSince: page.Cursor` plus `HasMore: page.HasMore`. Message filtering cannot change the cursor.
- [ ] Preserve the existing `ProjectRoom`-based path for non-run rooms. Do not change their limit/default behavior.
- [ ] Preserve the current export's effective 200-message cap: for a run room, call canonical `Messages` once with `Limit:200` instead of the current out-of-range `Limit:1000`. Export only that bounded page in the existing `RoomExport` shape; do not add an unbounded page accumulator in this task.

Run GREEN:

```bash
cd /srv/chrote-worktrees/formations-run-view/src
gofmt -w internal/comms/projection.go internal/comms/run_projection_test.go internal/comms/projection_test.go
go test ./internal/comms -run 'TestRun(Room|Messages|Artifacts|Risks|Projection).*Canonical|TestRunMessagesAdvance' -count=1
go test -race ./internal/comms -run 'TestRun(Room|Messages|Projection)|TestProjectRoom|TestMessages|TestExport' -count=1
go test ./internal/comms -count=1
```

Expected GREEN: focused, race, and full Comms tests pass; non-run fixtures are byte-for-byte unchanged. Commit production separately:

```bash
git add src/internal/comms/projection.go src/internal/comms/run_projection_test.go src/internal/comms/projection_test.go
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

### Step 7.1: Replace legacy TypeScript contracts with exact canonical types

- [ ] Add `JsonSafeInteger = number` plus runtime validation requiring an integer in `0..Number.MAX_SAFE_INTEGER`.
- [ ] Mirror every exact `RunView` structural type and ordering-bearing collection from the design: source, identity, audit, nodes, attempts, Gates, outputs, artifacts, blocks, escalations, sessions, actions, recovery state, and reconcile condition. Do not use `Record<string, unknown>` for typed public payloads.
- [ ] Define `SafeRunEvent` as a discriminated union over the exact 37 public event types enumerated in Task 2, with a closed safe data interface for each variant. Private fields such as paths, targets, tokens, prompt/capture/pane/input bytes, and exact baselines do not exist in the TS types.
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
- [ ] Validate page schema, run ID, ascending sequences, `seq > requestedSince`, `seq <= page.cursor`, JSON-safe integers, and cursor progress before committing the candidate page.
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
  page: RunEventPage,
): { events: SafeRunEvent[]; cursor: JsonSafeInteger; hasMore: boolean } {
  if (page.schema !== 'formations.run-events.v1' || page.runId !== expectedRunId) {
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
  return {
    events: [...candidate.values()].sort((left, right) => left.seq - right.seq),
    cursor: page.cursor,
    hasMore: page.hasMore,
  }
}
```

Tests must take a before-state deep copy and prove every thrown branch leaves it byte-equal. Include an empty omitted page that advances, an equal repeat with different object-key order, and a same-sequence payload mismatch.

### Step 7.3: Specify API calls and controller state transitions

- [ ] Change `fetchRunStatus` to `fetchRunView`, decoding `data.status`. Change `fetchRunEvents` to `fetchRunEventPage(runId, since, limit=200)`, decoding the page directly. Encode both numeric query values with `String` after runtime integer validation.
- [ ] Mutation API functions return the exact response unions above. Receipt guards validate every arm and forbidden field. Compatibility guards require a canonical `RunView` whose source marks compatibility.
- [ ] `restore(boardSlug)` reads only the run ID string from `activeRunStorageKey`. It treats that string as a hint, fetches a fresh view and pages from cursor zero, and never creates status/actions from storage.
- [ ] `refresh(boardSlug)` coalesces concurrent requests by run ID. It fetches a fresh `RunView`, then catches up pages from the stored event cursor until `hasMore:false`; each page is committed atomically. A generation/abort guard discards responses for a run that is no longer current.
- [ ] A failed view/page/contract read retains the complete last-good view/events/cursor, sets `freshness:'stale'` and the safe error code, and disables all submission. It never clears into a misleading empty/live state.
- [ ] The provider maintains one 1200 ms poll per non-final run ID while at least one board entry references it. Polling stops on a fresh final view or unmount. Tests inject fake timers and prove two consumers do not create two polls.
- [ ] After any mutation response, validate the response arm but do not apply optimistic status. For a receipt, store `lastReceipt`; for compatibility, require the response's `status.source.compatibility`. Then perform a fresh view/page reconciliation. If reconciliation fails, mark the old last-good state stale and keep actions disabled.
- [ ] On a validated schema-1 start response, persist only `runId` as the hint before reconciliation. Remove the hint only after a fresh final view. Schema-2 rejected start has no run ID and writes nothing.
- [ ] `canSubmit` returns true only when freshness is `fresh`, no mutation is pending, and the exact action exists in current `view.actions`. This also governs Peek metadata/input entry points; local storage, labels, or stale state cannot enable them.

Add controller transition tests for: cold restore, corrupt hint, duplicate observers, paginated catch-up, omitted empty page, mismatch rollback, transient stale/read recovery, final cleanup, schema-1 compatibility, applied receipt, rejected start receipt, pending/unavailable receipt, and mutation-success/reconciliation-failure.

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
- [ ] Update mock endpoints to emit `data.status: RunView` and `data: RunEventPage`, including cursors and `hasMore`. Mutation mocks use existing schema-1 keys with compatibility source; do not fabricate receipts.
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
go build -o /tmp/chrote-formations-run-view-server ./cmd/server

cd /srv/chrote-worktrees/formations-run-view
if ! CHROTE_SERVER_BINARY=/tmp/chrote-formations-run-view-server \
  scripts/test-built-server-contract.sh \
  > /tmp/ctx-7i1-built-server-contract.log 2>&1; then
  sed -n '1,240p' /tmp/ctx-7i1-built-server-contract.log
  exit 1
fi
test -s /tmp/ctx-7i1-built-server-contract.log
sed -n '1,240p' /tmp/ctx-7i1-built-server-contract.log
```

Retain both `/tmp/chrote-formations-run-view-server` and `/tmp/ctx-7i1-built-server-contract.log` as final-review evidence. `scripts/test-built-server-contract.sh` is the mandatory allowed isolated contract lane: it runs the supplied binary as a disposable temporary-root child on a random port with ttyd and persistent agents disabled and with explicit empty temporary tmux sockets/harnesses. It is not a deploy, live-service, or interactive-tmux action. Do not run another live-backend suite, start either system service, restart `chrote-srv.service`, touch legacy `chrote.service`, or alter an interactive tmux session. The generated embed directory is ignored build output unless the repository state at implementation time says otherwise; inspect `git status --short` rather than assuming it belongs in a commit.

### Step F.4: Run contract, hygiene, and deletion scans

```bash
cd /srv/chrote-worktrees/formations-run-view
python3 scripts/doc-lint.py
git diff --check 884deeec2c4d4ec2e220b7450dccdd6a10238ef5..HEAD

rg -n 'ReadRunEvents|ProjectRun\(|ListRuns\(|ProjectOpenEscalations' src/internal/api src/internal/comms src/cmd/archon dashboard/src
rg -n 'RunStatusProjection|RunStatusResult|fetchRunStatus|fetchRunEvents|statusFromRunEvent|runEventResumeAllowed' dashboard/src
rg -n 'json:"(path|socket|targetKey|token|prompt|capture|pane|baseline)"' src/internal/formations/run_projection*.go
rg -n 't\.Skip\(|describe\.skip|it\.skip|test\.skip' src/internal/formations src/internal/api src/internal/comms src/cmd/archon dashboard/src dashboard/tests
```

- [ ] The first two scans must have no production consumer hit. Compatibility definitions/tests may remain only when their adapter is derived from `RunView` and the review package explains the hit.
- [ ] The private-field scan must have no public projection JSON tag. Safe hashes/encodings are allowed only under their exact approved names.
- [ ] For the skip scan, compare against certified base `884deeec2c4d4ec2e220b7450dccdd6a10238ef5`. A new skip in relevant coverage blocks completion; pre-existing unrelated skips must be listed, not silently called passing.
- [ ] Search the branch diff for event reducers. The only semantic event-type switch is inside `ProjectCanonicalRun`; sanitizer variant dispatch and presentation-only safe-event formatting are not reducers and must not mutate status/actions/artifacts.
- [ ] Confirm `RuntimeAuthorityCapability.SemanticProjection` is still false and schema-2 receipt serving is unbound. Confirm schema-1 response keys remain preserved and compatibility is conveyed only by `RunView.source.compatibility`.
- [ ] Confirm no new ADR exists and the approved design still states that no new architectural decision is recorded.

### Step F.5: Obtain independent whole-branch review

- [ ] Generate the final whole-slice package with `/home/perttu/skills/subagent-driven-development/scripts/review-package 884deeec2c4d4ec2e220b7450dccdd6a10238ef5 HEAD`. Optionally generate a second package from the recorded pre-implementation plan HEAD, but never use it instead of the certified-base package.
- [ ] Dispatch the most capable available independent reviewer using `requesting-code-review`. Give it the approved design, this plan, `.superpowers/sdd/progress.md`, the printed package, child review/evidence paths, and exact gate outputs.
- [ ] Require review of: sole-reducer architecture; source precedence/no fallback; `SemanticProjection:false`; exact receipt-vs-compatibility behavior; full-page byte/scan/cursor logic; SSE one-snapshot closure; raw-message sanitization; session/action/recovery privacy; optimistic artifact-open linearization; no raw consumer scans; controller concurrency/stale/mismatch behavior; and scope/no-UX drift.
- [ ] If findings exist, dispatch one fresh fixer with the complete Critical/Important list, named covering tests, and one appended report. Re-run covering gates and send the updated package to an independent re-review. Do not split findings across agents or dismiss a plan conflict without user resolution.
- [ ] Record every resolved finding and final approval in `.superpowers/sdd/progress.md`. All Minor findings must be explicitly fixed or accepted with rationale before closure.

### Step F.6: Attach final evidence and close only the umbrella

- [ ] Write `/tmp/ctx-7i1-final-evidence.md` with child IDs/commits/reviews, exact gate commands and outputs, final review disposition, changed-file inventory, `SemanticProjection:false` proof, receipt-provider-unbound proof, and any accepted Minor item.
- [ ] Attach it with `bd comments add ctx-7i1 -f /tmp/ctx-7i1-final-evidence.md`.
- [ ] Close the umbrella only now:

```bash
bd close ctx-7i1 --reason "All seven canonical run-view checkpoints completed RED-first; focused, race, dashboard, contract, and independent whole-branch review evidence attached."
```

- [ ] Run `bd show ctx-7i1 --json` once more and record the closed state in `.superpowers/sdd/progress.md` with `apply_patch`.
- [ ] Stop. Do not merge, push, deploy, restart, or clean another worktree. Use `finishing-a-development-branch` only when the user separately authorizes an integration choice.
