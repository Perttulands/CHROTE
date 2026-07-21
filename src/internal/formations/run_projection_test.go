package formations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	projectionTestRunID       = "run_01KXNP6VY3227H78329V52CKF8"
	projectionTestOtherRunID  = "run_01KXNP6VY3227H78329V52CKF9"
	projectionTestWorkspaceID = "wsa_01KXNP6VY3227H78329V52CKF8"
	projectionTestAuthorityID = "auth_01KXNP6VY3227H78329V52CKF8"
	projectionTestCommandID   = "cmd_01KXNP6VY3227H78329V52CKF8"
	projectionTestOtherCmdID  = "cmd_01KXNP6VY3227H78329V52CKF9"
	projectionTestBoardID     = "brd_projection"
	projectionTestBoardSlug   = "projection"
	projectionTestMissionID   = "mis_root"
	projectionTestFormationID = "fmn_work"
	projectionTestGateID      = "gate_review"
)

var lowercaseSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestProjectCanonicalRunStructuralTransitions(t *testing.T) {
	tests := []struct {
		name             string
		events           []map[string]any
		wantStatus       string
		wantFinal        bool
		wantNodeStatus   string
		wantGateStatus   string
		wantBlocks       int
		wantEscalations  int
		wantConsumed     uint64
		wantCurrentEpoch uint64
	}{
		{
			name:         "start",
			events:       []map[string]any{schema1StartedEvent(projectionTestRunID)},
			wantStatus:   "running",
			wantConsumed: 1,
		},
		{
			name: "node waiting",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventNodeWaiting, map[string]any{
					"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"},
				}),
			},
			wantStatus:     "running",
			wantNodeStatus: "waiting",
			wantConsumed:   2,
		},
		{
			name: "node start",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1NodeStartedEvent(projectionTestRunID, 2),
			},
			wantStatus:     "running",
			wantNodeStatus: "running",
			wantConsumed:   2,
		},
		{
			name: "node terminal",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1NodeStartedEvent(projectionTestRunID, 2),
				schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
			},
			wantStatus:     "running",
			wantNodeStatus: "done",
			wantConsumed:   3,
		},
		{
			name: "gate open",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1GateEvaluatingEvent(projectionTestRunID, 2),
			},
			wantStatus:     "running",
			wantGateStatus: "evaluating",
			wantConsumed:   2,
		},
		{
			name: "gate verdict",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1GateEvaluatingEvent(projectionTestRunID, 2),
				schema1GateVerdictEvent(projectionTestRunID, 3, "pass"),
			},
			wantStatus:     "running",
			wantGateStatus: "passed",
			wantConsumed:   3,
		},
		{
			name: "escalation open",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1EscalationEvent(projectionTestRunID, 2, true),
				schema1BlockedEvent(projectionTestRunID, 3, true, []any{}),
			},
			wantStatus:      "blocked",
			wantBlocks:      1,
			wantEscalations: 1,
			wantConsumed:    3,
		},
		{
			name: "escalation block resolved by resume while evidence remains",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1EscalationEvent(projectionTestRunID, 2, true),
				schema1BlockedEvent(projectionTestRunID, 3, true, []any{}),
				schema1ResumedEvent(projectionTestRunID, 4, 3, []any{}),
			},
			wantStatus:       "running",
			wantBlocks:       1,
			wantEscalations:  1,
			wantConsumed:     4,
			wantCurrentEpoch: 1,
		},
		{
			name: "run canceled",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventCanceled, map[string]any{
					"reason": "operator stop", "requestedBy": "human:test", "softInterruptedSlots": []string{}, "final": true,
				}),
			},
			wantStatus:   "canceled",
			wantFinal:    true,
			wantConsumed: 2,
		},
		{
			name: "run failed",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventFailed, map[string]any{
					"code": "projection_fixture", "reason": "failed", "boundary": "engine", "recoverable": false, "relatedSeq": 1, "final": true,
				}),
			},
			wantStatus:   "failed",
			wantFinal:    true,
			wantConsumed: 2,
		},
		{
			name: "run succeeded",
			events: []map[string]any{
				schema1StartedEvent(projectionTestRunID),
				schema1Event(projectionTestRunID, 2, RunEventSucceeded, map[string]any{"final": true}),
			},
			wantStatus:   "succeeded",
			wantFinal:    true,
			wantConsumed: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t, test.events...))
			view := ProjectRunView(projection)
			if view.Status != test.wantStatus || view.Final != test.wantFinal {
				t.Fatalf("status/final = %q/%v, want %q/%v", view.Status, view.Final, test.wantStatus, test.wantFinal)
			}
			if view.Cursor != test.wantConsumed || view.Audit.ConsumedEventCount != test.wantConsumed {
				t.Fatalf("cursor/audit count = %d/%d, want %d", view.Cursor, view.Audit.ConsumedEventCount, test.wantConsumed)
			}
			if view.Audit.StartSeq != 1 || view.Audit.EventSchema != 1 {
				t.Fatalf("audit = %+v, want schema 1 start sequence 1", view.Audit)
			}
			if view.Identity.Epoch != test.wantCurrentEpoch {
				t.Fatalf("epoch = %d, want %d", view.Identity.Epoch, test.wantCurrentEpoch)
			}
			if test.wantNodeStatus != "" {
				node := findProjectedNode(t, view, projectionTestFormationID)
				if node.Status != test.wantNodeStatus {
					t.Fatalf("node status = %q, want %q", node.Status, test.wantNodeStatus)
				}
			}
			if test.wantGateStatus != "" {
				gate := findProjectedGate(t, view, projectionTestGateID)
				if gate.Status != test.wantGateStatus {
					t.Fatalf("gate status = %q, want %q", gate.Status, test.wantGateStatus)
				}
			}
			if len(view.Blocks) != test.wantBlocks || len(view.Escalations) != test.wantEscalations {
				t.Fatalf("blocks/escalations = %d/%d, want %d/%d", len(view.Blocks), len(view.Escalations), test.wantBlocks, test.wantEscalations)
			}
		})
	}
}

func TestProjectCanonicalRunSchema2ActivateAndArtifactTransitions(t *testing.T) {
	queued := mustProjectCanonicalFixture(t, schema2ProjectionInput(t, false))
	queuedView := ProjectRunView(queued)
	if queuedView.Status != "queued" || queuedView.Final {
		t.Fatalf("start-only schema-2 status/final = %q/%v, want queued/false", queuedView.Status, queuedView.Final)
	}

	activatedInput := schema2ProjectionInput(t, true)
	activated := mustProjectCanonicalFixture(t, activatedInput)
	if got := ProjectRunView(activated).Status; got != "running" {
		t.Fatalf("activated status = %q, want running", got)
	}

	available := map[string]any{
		"artifactId":   "art_01KXNP6VY3227H78329V52CKF8",
		"availability": "available",
		"name":         "report",
		"artifact": map[string]any{
			"artifactId": "art_01KXNP6VY3227H78329V52CKF8",
			"rootId":     "root_workspace",
			"ref":        "artifacts/report.json",
			"mediaType":  "application/json",
			"sizeBytes":  2,
			"sha256":     projectionSHA256([]byte("{}")),
		},
	}
	extra := []map[string]any{
		schema2Event(projectionTestRunID, 3, "artifact_attached", map[string]any{
			"artifactProjection": available,
			"source":             map[string]any{"kind": "system"},
		}),
		schema2Event(projectionTestRunID, 4, "artifact_observed", map[string]any{
			"artifactId":   "art_01KXNP6VY3227H78329V52CKF8",
			"availability": "redacted",
			"errorCode":    "policy_redacted",
			"observedAt":   "2026-07-20T10:00:03Z",
			"relatedSeq":   3,
		}),
	}
	projection := mustProjectCanonicalFixture(t, schema2ProjectionInput(t, true, extra...))
	view := ProjectRunView(projection)
	if len(view.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one latest projection", view.Artifacts)
	}
	raw := mustMarshalJSON(t, view.Artifacts[0])
	if !bytes.Contains(raw, []byte(`"availability":"redacted"`)) || bytes.Contains(raw, []byte(`"ref"`)) {
		t.Fatalf("latest artifact did not revoke historical readability: %s", raw)
	}
	page := mustProjectEventPage(t, projection, 0, RunPageMaximumLimit)
	for _, event := range page.Events {
		eventRaw := mustMarshalJSON(t, event)
		if bytes.Contains(eventRaw, []byte(`"availability":"available"`)) {
			t.Fatalf("historical event retained superseded available artifact: %s", eventRaw)
		}
	}
}

func TestProjectCanonicalRunRejectsInvalidSequencesAndFinality(t *testing.T) {
	started := schema1StartedEvent(projectionTestRunID)
	validTerminal := schema1Event(projectionTestRunID, 2, RunEventSucceeded, map[string]any{"final": true})
	tests := []struct {
		name   string
		events []map[string]any
	}{
		{
			name: "shuffled",
			events: []map[string]any{
				started,
				schema1Event(projectionTestRunID, 3, RunEventNodeWaiting, map[string]any{"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"}}),
				schema1NodeStartedEvent(projectionTestRunID, 2),
			},
		},
		{
			name: "duplicate sequence",
			events: []map[string]any{
				started,
				schema1NodeStartedEvent(projectionTestRunID, 2),
				schema1Event(projectionTestRunID, 2, RunEventError, map[string]any{"code": "duplicate", "message": "duplicate", "boundary": "schema", "recoverable": false, "relatedSeq": 1}),
			},
		},
		{
			name: "sequence zero",
			events: []map[string]any{
				withEventSequence(started, uint64(0)),
			},
		},
		{
			name: "sequence above JSON safe integer",
			events: []map[string]any{
				started,
				withEventSequence(schema1NodeStartedEvent(projectionTestRunID, 2), MaxJSONSafeInteger+1),
			},
		},
		{
			name: "run id mismatch",
			events: []map[string]any{
				started,
				schema1Event(projectionTestOtherRunID, 2, RunEventNodeWaiting, map[string]any{"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"}}),
			},
		},
		{
			name: "post-terminal mutation",
			events: []map[string]any{
				started,
				validTerminal,
				schema1NodeStartedEvent(projectionTestRunID, 3),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ProjectCanonicalRun(schema1ProjectionInput(t, test.events...))
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}
}

func TestProjectCanonicalRunValidatesInputRolesAndOwnsBytes(t *testing.T) {
	schema1 := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
	for _, role := range []CanonicalInputRole{
		CanonicalInputRoleSchema1Ledger,
		CanonicalInputRoleSchema1GraphSnapshot,
		CanonicalInputRoleSchema1BindingsSnapshot,
	} {
		t.Run("schema 1 missing "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema1)
			input.Documents = removeCanonicalRole(input.Documents, role)
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
		t.Run("schema 1 duplicate "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema1)
			input.Documents = append(input.Documents, canonicalDocumentByRole(t, input, role))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	t.Run("schema 1 rejects schema 2 role", func(t *testing.T) {
		input := cloneCanonicalInput(schema1)
		input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2RunBootstrap, []byte(`{}`)))
		_, err := ProjectCanonicalRun(input)
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	})

	schema2 := schema2ProjectionInput(t, true)
	for _, role := range []CanonicalInputRole{
		CanonicalInputRoleSchema2WorkspaceRegistry,
		CanonicalInputRoleSchema2WorkspaceBootstrap,
		CanonicalInputRoleSchema2WorkspaceAuthority,
		CanonicalInputRoleSchema2RunBootstrap,
		CanonicalInputRoleSchema2GraphSnapshot,
		CanonicalInputRoleSchema2PrivateBindings,
		CanonicalInputRoleSchema2Ledger,
	} {
		t.Run("schema 2 missing "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema2)
			input.Documents = removeCanonicalRole(input.Documents, role)
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
		t.Run("schema 2 duplicate "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema2)
			input.Documents = append(input.Documents, canonicalDocumentByRole(t, input, role))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(*CanonicalRunReadInput)
	}{
		{
			name: "missing admission policy chain",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = removeCanonicalRole(input.Documents, CanonicalInputRoleSchema2AdmissionPolicy)
			},
		},
		{
			name: "duplicate admission policy identity",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalDocumentByRole(t, *input, CanonicalInputRoleSchema2AdmissionPolicy))
			},
		},
		{
			name: "missing referenced command",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = removeCanonicalRole(input.Documents, CanonicalInputRoleSchema2CommandRecord)
			},
		},
		{
			name: "duplicate command identity",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalDocumentByRole(t, *input, CanonicalInputRoleSchema2CommandRecord))
			},
		},
		{
			name: "unreferenced extra command",
			mutate: func(input *CanonicalRunReadInput) {
				extra := schema2CommandRecord(t, projectionTestOtherCmdID, "start", "applied", projectionTestRunID)
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2CommandRecord, extra))
			},
		},
		{
			name: "unreferenced private state",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema2RunPrivateState, canonicalJSON(t, map[string]any{
					"recordSchema": 1, "privateStateId": "state_01KXNP6VY3227H78329V52CKF8",
				})))
			},
		},
		{
			name: "schema 2 rejects schema 1 role",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRoleSchema1Ledger, []byte("{}\n")))
			},
		},
		{
			name: "unknown role",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents = append(input.Documents, canonicalInputDocument(CanonicalInputRole("projection-test-unknown"), []byte("{}")))
			},
		},
		{
			name: "SHA-256 mismatch",
			mutate: func(input *CanonicalRunReadInput) {
				input.Documents[0].SHA256 = strings.Repeat("f", 64)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := cloneCanonicalInput(schema2)
			test.mutate(&input)
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}

	t.Run("projection owns mutable reader bytes", func(t *testing.T) {
		input := cloneCanonicalInput(schema1)
		projection := mustProjectCanonicalFixture(t, input)
		before := mustMarshalJSON(t, ProjectRunView(projection))
		for index := range input.Documents {
			for byteIndex := range input.Documents[index].Bytes {
				input.Documents[index].Bytes[byteIndex] = 'x'
			}
		}
		after := mustMarshalJSON(t, ProjectRunView(projection))
		if !bytes.Equal(after, before) {
			t.Fatalf("projection retained mutable input aliases\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

func TestProjectRunViewIsDeterministicDefensiveAndHistoryFree(t *testing.T) {
	projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1Event(projectionTestRunID, 2, RunEventNodeWaiting, map[string]any{
			"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"},
		}),
	))

	first := ProjectRunView(projection)
	second := ProjectRunView(projection)
	firstRaw := mustMarshalJSON(t, first)
	secondRaw := mustMarshalJSON(t, second)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatalf("ProjectRunView is nondeterministic\nfirst:  %s\nsecond: %s", firstRaw, secondRaw)
	}
	if bytes.Contains(firstRaw, []byte(`"events"`)) {
		t.Fatalf("RunView embeds event history: %s", firstRaw)
	}

	if len(first.Nodes) == 0 {
		t.Fatal("fixture projected no nodes")
	}
	first.Nodes[0].Status = "failed"
	if len(first.Nodes[0].Readiness.WaitingFor) > 0 {
		first.Nodes[0].Readiness.WaitingFor[0] = "mutated"
	}
	first.Nodes = append(first.Nodes, first.Nodes[0])
	third := ProjectRunView(projection)
	thirdRaw := mustMarshalJSON(t, third)
	if !bytes.Equal(thirdRaw, secondRaw) {
		t.Fatalf("ProjectRunView returned aliased state\nbefore mutation: %s\nafter mutation:  %s", secondRaw, thirdRaw)
	}
}

func TestProjectRunViewGenerationTracksImmutableIncarnation(t *testing.T) {
	baseInput := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
	baseProjection := mustProjectCanonicalFixture(t, baseInput)
	base := ProjectRunView(baseProjection)
	if !lowercaseSHA256Pattern.MatchString(base.Generation) {
		t.Fatalf("generation = %q, want exact lowercase 64-hex SHA-256", base.Generation)
	}

	appended := cloneCanonicalInput(baseInput)
	appended = replaceCanonicalDocument(t, appended, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t,
		schema1StartedEvent(projectionTestRunID),
		schema1Event(projectionTestRunID, 2, RunEventError, map[string]any{
			"code": "display_only", "message": "tail", "reason": "tail", "boundary": "schema", "nodeId": "", "gateId": "", "slotId": "", "dispatchId": "", "recoverable": true, "relatedSeq": 1,
		}),
	))
	if got := ProjectRunView(mustProjectCanonicalFixture(t, appended)).Generation; got != base.Generation {
		t.Fatalf("generation changed across an appended ledger tail: got %q want %q", got, base.Generation)
	}

	firstChanged := cloneCanonicalInput(baseInput)
	changedStart := schema1StartedEvent(projectionTestRunID)
	changedStart["actor"] = "agent:other"
	firstChanged = replaceCanonicalDocument(t, firstChanged, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, changedStart))
	snapshotChanged := replaceCanonicalDocument(t, cloneCanonicalInput(baseInput), CanonicalInputRoleSchema1GraphSnapshot, append([]byte(schema1ProjectionSnapshot), '\n'))
	bindingsChanged := replaceCanonicalDocument(t, cloneCanonicalInput(baseInput), CanonicalInputRoleSchema1BindingsSnapshot, append([]byte(schema1ProjectionBindings), '\n'))
	for _, test := range []struct {
		name  string
		input CanonicalRunReadInput
	}{
		{name: "first event", input: firstChanged},
		{name: "snapshot", input: snapshotChanged},
		{name: "bindings", input: bindingsChanged},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ProjectRunView(mustProjectCanonicalFixture(t, test.input)).Generation
			if got == base.Generation || !lowercaseSHA256Pattern.MatchString(got) {
				t.Fatalf("generation after %s change = %q, base %q", test.name, got, base.Generation)
			}
		})
	}

	schema2Base := schema2ProjectionInput(t, true)
	schema2Generation := ProjectRunView(mustProjectCanonicalFixture(t, schema2Base)).Generation
	if !lowercaseSHA256Pattern.MatchString(schema2Generation) {
		t.Fatalf("schema-2 generation = %q, want lowercase SHA-256", schema2Generation)
	}
	for _, role := range []CanonicalInputRole{
		CanonicalInputRoleSchema2RunBootstrap,
		CanonicalInputRoleSchema2GraphSnapshot,
		CanonicalInputRoleSchema2PrivateBindings,
		CanonicalInputRoleSchema2Ledger,
	} {
		t.Run("schema 2 tuple changes with "+string(role), func(t *testing.T) {
			input := cloneCanonicalInput(schema2Base)
			document := canonicalDocumentByRole(t, input, role)
			switch role {
			case CanonicalInputRoleSchema2RunBootstrap:
				document.Bytes = bytes.Replace(document.Bytes, []byte(projectionTestAuthorityID), []byte("auth_01KXNP6VY3227H78329V52CKF9"), 1)
			case CanonicalInputRoleSchema2GraphSnapshot:
				document.Bytes = append(document.Bytes, '\n')
			case CanonicalInputRoleSchema2PrivateBindings:
				document.Bytes = append(document.Bytes, '\n')
			case CanonicalInputRoleSchema2Ledger:
				// The immutable tuple includes the admission command id from the first event,
				// but excludes an ordinary advancing tail.
				document.Bytes = bytes.Replace(document.Bytes, []byte(projectionTestCommandID), []byte(projectionTestOtherCmdID), 1)
			}
			document.SHA256 = projectionSHA256(document.Bytes)
			input = replaceCanonicalDocumentObject(input, role, document)
			projection, err := ProjectCanonicalRun(input)
			if err != nil {
				// Cross-document parity may reject a one-document substitution. That is
				// still the required fail-closed result, never reuse of the old generation.
				return
			}
			if got := ProjectRunView(projection).Generation; got == schema2Generation {
				t.Fatalf("schema-2 generation did not change after immutable tuple substitution in %s", role)
			}
		})
	}
}

func TestProjectRunEventPageCountsProjectionOnlySlot(t *testing.T) {
	projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1NodeStartedEvent(projectionTestRunID, 2),
	))
	projection.events[1] = projectedEvent{scanSeq: 2, omitted: true}
	projection.events = append(projection.events, projectedEvent{
		scanSeq: 3,
		safe:    cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, projection, 0), 3),
	})
	projection.latestSeq = 3
	projection.view.Cursor = 3

	before := projectionFingerprint(t, projection)
	page, err := ProjectRunEventPage(projection, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Schema != RunEventPageSchema {
		t.Fatalf("schema = %q, want %q", page.Schema, RunEventPageSchema)
	}
	if page.Generation != ProjectRunView(projection).Generation {
		t.Fatalf("generation = %q, want projection generation", page.Generation)
	}
	if page.Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", page.Cursor)
	}
	if !page.HasMore {
		t.Fatal("hasMore = false, want true")
	}
	if len(page.Events) != 0 {
		t.Fatalf("events = %v, want none", page.Events)
	}
	if after := projectionFingerprint(t, projection); after != before {
		t.Fatalf("event selector mutated projection\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestProjectRunEventPageIdentityCursorAndOrdering(t *testing.T) {
	var selector func(CanonicalRunProjection, uint64, int) (RunEventPage, error) = ProjectRunEventPage
	_ = selector // The fixed selector signature has no adapter-supplied identity fields.

	projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1NodeStartedEvent(projectionTestRunID, 2),
		schema1NodeOutputEvent(projectionTestRunID, 3, "done"),
	))
	view := ProjectRunView(projection)
	page := mustProjectEventPage(t, projection, 0, RunPageMaximumLimit)
	pageRaw := mustMarshalJSON(t, page)
	var decoded struct {
		Schema     string          `json:"schema"`
		RunID      string          `json:"runId"`
		Generation string          `json:"generation"`
		Source     json.RawMessage `json:"source"`
		Cursor     uint64          `json:"cursor"`
		HasMore    bool            `json:"hasMore"`
		Events     []struct {
			Seq uint64 `json:"seq"`
		} `json:"events"`
	}
	if err := json.Unmarshal(pageRaw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != RunEventPageSchema || decoded.RunID != view.RunID || decoded.Generation != view.Generation {
		t.Fatalf("page identity = %+v, want schema/run/generation from view %+v", decoded, view)
	}
	if !lowercaseSHA256Pattern.MatchString(decoded.Generation) {
		t.Fatalf("generation = %q, want lowercase SHA-256", decoded.Generation)
	}
	if !bytes.Equal(decoded.Source, mustMarshalJSON(t, view.Source)) {
		t.Fatalf("page source = %s, want exact view source %s", decoded.Source, mustMarshalJSON(t, view.Source))
	}
	for index := 1; index < len(decoded.Events); index++ {
		if decoded.Events[index-1].Seq >= decoded.Events[index].Seq {
			t.Fatalf("events not strictly ascending: %+v", decoded.Events)
		}
	}

	for _, since := range []uint64{4, 41, MaxJSONSafeInteger} {
		empty := mustProjectEventPage(t, projection, since, 1)
		if empty.Schema != RunEventPageSchema || empty.RunID != view.RunID || empty.Generation != view.Generation {
			t.Fatalf("empty page lost identity at since %d: %+v", since, empty)
		}
		if !reflect.DeepEqual(empty.Source, view.Source) || empty.Cursor != since || empty.HasMore || len(empty.Events) != 0 {
			t.Fatalf("empty page at since %d = %+v, want echoed cursor, no events, hasMore false", since, empty)
		}
	}
}

func TestProjectRunEventPageLimitAndExactByteBoundary(t *testing.T) {
	base := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1Event(projectionTestRunID, 2, RunEventError, map[string]any{
			"code": "page_fixture", "message": "x", "reason": "x", "boundary": "schema", "nodeId": "", "gateId": "", "slotId": "", "dispatchId": "", "recoverable": true, "relatedSeq": 1,
		}),
	))

	for _, invalidLimit := range []int{0, RunPageMaximumLimit + 1} {
		_, err := ProjectRunEventPage(base, 0, invalidLimit)
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}
	if _, err := ProjectRunEventPage(base, MaxJSONSafeInteger+1, 1); err == nil {
		t.Fatal("since above MaxJSONSafeInteger succeeded")
	} else {
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}

	many := projectionWithRepeatedSafeEvents(t, base, 201)
	one := mustProjectEventPage(t, many, 0, 1)
	if one.Cursor != 1 || len(one.Events) != 1 || !one.HasMore {
		t.Fatalf("limit 1 page = %+v", one)
	}
	twoHundred := mustProjectEventPage(t, many, 0, RunPageMaximumLimit)
	if twoHundred.Cursor != RunPageMaximumLimit || len(twoHundred.Events) != RunPageMaximumLimit || !twoHundred.HasMore {
		t.Fatalf("limit 200 page cursor/events/hasMore = %d/%d/%v", twoHundred.Cursor, len(twoHundred.Events), twoHundred.HasMore)
	}

	for _, target := range []int{RunPageMaximumBytes - 1, RunPageMaximumBytes} {
		projection := projectionWithCompletePageSize(t, base, target)
		page := mustProjectEventPage(t, projection, 1, 1)
		if got := len(mustMarshalJSON(t, page)); got != target {
			t.Fatalf("encoded page bytes = %d, want %d", got, target)
		}
	}

	oversized := projectionWithCompletePageSize(t, base, RunPageMaximumBytes+1)
	_, err := ProjectRunEventPage(oversized, 1, 1)
	requireProjectionError(t, err, ErrRunProjectionResourceLimit)

	firstSafe := cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, base, 1), 2)
	secondProjection := projectionWithCompletePageSize(t, base, RunPageMaximumBytes)
	secondSafe := cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, secondProjection, 1), 3)
	bounded := base
	bounded.events = []projectedEvent{
		{scanSeq: 1, safe: cloneSafeEventWithSequence(t, firstSafe, 1)},
		{scanSeq: 2, safe: firstSafe},
		{scanSeq: 3, safe: secondSafe},
	}
	bounded.latestSeq = 3
	bounded.view.Cursor = 3
	page := mustProjectEventPage(t, bounded, 1, RunPageMaximumLimit)
	if page.Cursor != 2 || len(page.Events) != 1 || !page.HasMore {
		t.Fatalf("byte-capped nonempty page = cursor %d events %d hasMore %v, want 2/1/true", page.Cursor, len(page.Events), page.HasMore)
	}
}

type countingCanonicalRunReader struct {
	input        CanonicalRunReadInput
	readRunCalls int
	readIDs      []string
}

func (r *countingCanonicalRunReader) ReadRun(runID string) (CanonicalRunReadInput, error) {
	r.readRunCalls++
	r.readIDs = append(r.readIDs, runID)
	return cloneCanonicalInput(r.input), nil
}

func (r *countingCanonicalRunReader) ListRunIdentities(RunIdentityPageRequest) (RunIdentityPage, error) {
	panic("ListRunIdentities must not be called by ReadCanonicalRun")
}

func (r *countingCanonicalRunReader) ReadCommand(SubmittedCommandIdentity) (CanonicalCommandReadInput, error) {
	panic("ReadCommand must not be called by ReadCanonicalRun")
}

func TestStoreReadCanonicalRunReadsOnceAndSelectorsDoNotRead(t *testing.T) {
	reader := &countingCanonicalRunReader{
		input: schema1ProjectionInput(t,
			schema1StartedEvent(projectionTestRunID),
			schema1NodeStartedEvent(projectionTestRunID, 2),
		),
	}
	store := NewStore(t.TempDir())
	store.canonicalRunAuthorityReader = reader

	projection, err := store.ReadCanonicalRun(projectionTestRunID)
	if err != nil {
		t.Fatal(err)
	}
	if reader.readRunCalls != 1 || !reflect.DeepEqual(reader.readIDs, []string{projectionTestRunID}) {
		t.Fatalf("ReadRun calls/ids = %d/%v, want one exact read", reader.readRunCalls, reader.readIDs)
	}
	_ = ProjectRunView(projection)
	if _, err := ProjectRunEventPage(projection, 0, RunPageMaximumLimit); err != nil {
		t.Fatal(err)
	}
	if reader.readRunCalls != 1 {
		t.Fatalf("pure selectors caused %d reader calls, want one total", reader.readRunCalls)
	}
}

func TestProjectCommandReceiptPreservesTerminalUnion(t *testing.T) {
	for _, commandKind := range []string{"start", "resume", "cancel", "verdict"} {
		for _, state := range []string{"applied", "rejected"} {
			t.Run(commandKind+"/"+state, func(t *testing.T) {
				input := canonicalCommandInput(t, projectionTestCommandID, commandKind, state)
				receipt, err := ProjectCommandReceipt(input)
				if err != nil {
					t.Fatal(err)
				}
				var got map[string]any
				if err := json.Unmarshal(mustMarshalJSON(t, receipt), &got); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"commandId", "commandPayloadSha256", "commandKind", "outcomeWriterFence", "state", "decisionAdmissionPolicyRef"} {
					if _, ok := got[key]; !ok {
						t.Fatalf("receipt missing %q: %#v", key, got)
					}
				}
				if got["commandId"] != projectionTestCommandID || got["commandKind"] != commandKind || got["state"] != state {
					t.Fatalf("receipt identity/state = %#v", got)
				}
				if got["outcomeWriterFence"] != "9" {
					t.Fatalf("outcomeWriterFence = %#v, want canonical unsigned decimal string", got["outcomeWriterFence"])
				}
				if state == "applied" {
					if got["runId"] != projectionTestRunID || got["effectSeq"] != float64(7) {
						t.Fatalf("applied arm = %#v", got)
					}
					if _, exists := got["rejectionCode"]; exists {
						t.Fatalf("applied arm exposed rejectionCode: %#v", got)
					}
				} else {
					if got["rejectionCode"] != "fixture_rejected" {
						t.Fatalf("rejected arm = %#v", got)
					}
					if _, exists := got["runId"]; exists {
						t.Fatalf("rejected arm acquired runId: %#v", got)
					}
					if _, exists := got["effectSeq"]; exists {
						t.Fatalf("rejected arm acquired effectSeq: %#v", got)
					}
				}
				if commandKind == "start" {
					if got["decisionAdmissionPolicyRef"] == nil {
						t.Fatalf("start receipt lost decision policy ref: %#v", got)
					}
				} else if got["decisionAdmissionPolicyRef"] != nil {
					t.Fatalf("non-start receipt invented decision policy ref: %#v", got)
				}
			})
		}
	}
}

func TestProjectCommandReceiptRejectsPendingMismatchAndSubstitution(t *testing.T) {
	valid := canonicalCommandInput(t, projectionTestCommandID, "start", "applied")
	tests := []struct {
		name   string
		mutate func(*CanonicalCommandReadInput)
	}{
		{name: "wrong submitted id", mutate: func(input *CanonicalCommandReadInput) { input.Submitted.CommandID = projectionTestOtherCmdID }},
		{name: "right id wrong submitted payload hash", mutate: func(input *CanonicalCommandReadInput) { input.Submitted.CommandPayloadSHA256 = strings.Repeat("f", 64) }},
		{name: "cross kind", mutate: func(input *CanonicalCommandReadInput) { input.Submitted.CommandKind = "resume" }},
		{name: "pending", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) {
				record["state"] = "pending"
				delete(record, "runId")
				delete(record, "effectSeq")
				delete(record, "outcomeWriterFence")
				delete(record, "decisionAdmissionPolicyRef")
			})
		}},
		{name: "stale writer state", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["stateWriterFence"] = 1; record["admittedWriterFence"] = 2 })
		}},
		{name: "substituted record id", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["commandId"] = projectionTestOtherCmdID })
		}},
		{name: "substituted embedded payload", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["commandPayload"] = map[string]any{"kind": "start"} })
		}},
		{name: "applied missing run id", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, "runId") })
		}},
		{name: "applied missing effect sequence", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { delete(record, "effectSeq") })
		}},
		{name: "applied contains rejection field", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["rejectionCode"] = "forbidden" })
		}},
		{name: "start missing decision policy", mutate: func(input *CanonicalCommandReadInput) {
			input.Record = mutateCommandRecord(t, input.Record, func(record map[string]any) { record["decisionAdmissionPolicyRef"] = nil })
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := cloneCanonicalCommandInput(valid)
			test.mutate(&input)
			if receipt, err := ProjectCommandReceipt(input); err == nil {
				t.Fatalf("mismatch returned receipt: %#v", receipt)
			} else {
				requireProjectionError(t, err, ErrRunCommandNotTerminal)
			}
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "rejected missing code", mutate: func(record map[string]any) { delete(record, "rejectionCode") }},
		{name: "rejected contains run id", mutate: func(record map[string]any) { record["runId"] = projectionTestRunID }},
		{name: "rejected contains effect sequence", mutate: func(record map[string]any) { record["effectSeq"] = 7 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := canonicalCommandInput(t, projectionTestCommandID, "start", "rejected")
			input.Record = mutateCommandRecord(t, input.Record, test.mutate)
			if receipt, err := ProjectCommandReceipt(input); err == nil {
				t.Fatalf("invalid rejected arm returned receipt: %#v", receipt)
			} else {
				requireProjectionError(t, err, ErrRunCommandNotTerminal)
			}
		})
	}
}

func TestCanonicalRunSourceSelectionIsClosedAndNonAuthorizing(t *testing.T) {
	schema1 := mustProjectCanonicalFixture(t, schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID)))
	schema1View := ProjectRunView(schema1)
	var schema1Source map[string]any
	if err := json.Unmarshal(mustMarshalJSON(t, schema1View.Source), &schema1Source); err != nil {
		t.Fatal(err)
	}
	if schema1Source["eventSchema"] != float64(1) || schema1Source["compatibility"] != true {
		t.Fatalf("schema-1 source = %#v", schema1Source)
	}
	if _, exists := schema1Source["authoritySchema"]; exists {
		t.Fatalf("schema-1 source invented authority schema: %#v", schema1Source)
	}

	schema2Input := schema2ProjectionInput(t, true)
	schema2 := mustProjectCanonicalFixture(t, schema2Input)
	schema2View := ProjectRunView(schema2)
	var schema2Source map[string]any
	if err := json.Unmarshal(mustMarshalJSON(t, schema2View.Source), &schema2Source); err != nil {
		t.Fatal(err)
	}
	if schema2Source["eventSchema"] != float64(2) || schema2Source["authoritySchema"] != float64(2) || schema2Source["compatibility"] != false {
		t.Fatalf("schema-2 source = %#v", schema2Source)
	}

	claimedInvalid := cloneCanonicalInput(schema2Input)
	claimedInvalid.Documents = removeCanonicalRole(claimedInvalid.Documents, CanonicalInputRoleSchema2RunBootstrap)
	for _, document := range schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID)).Documents {
		claimedInvalid.Documents = append(claimedInvalid.Documents, document)
	}
	if projection, err := ProjectCanonicalRun(claimedInvalid); err == nil {
		t.Fatalf("invalid claimed schema 2 fell back to schema 1: %+v", ProjectRunView(projection))
	} else {
		requireProjectionError(t, err, ErrRunProjectionInvalid)
	}

	capability := disabledRuntimeAuthorityCapability()
	if capability.SemanticProjection {
		t.Fatalf("projector fixtures self-authorized semantic projection: %+v", capability)
	}
}

type schema1SafeRegistryCase struct {
	eventType   string
	safeData    map[string]any
	privateKeys []string
}

func TestProjectCanonicalRunSchema1SafeEventRegistry(t *testing.T) {
	cases := schema1SafeRegistryCases()
	if len(cases) != 21 {
		t.Fatalf("schema-1 registry cases = %d, want all 21 constants", len(cases))
	}
	registered := map[string]bool{}
	for _, eventType := range []string{
		RunEventStarted, RunEventResumed, RunEventNodeWaiting, RunEventNodeStarted,
		RunEventOrchestrationTeam, RunEventPeerPlane, RunEventSlotDispatch,
		RunEventAdapterSend, RunEventSlotResult, RunEventNodeOutput,
		RunEventGateEvaluating, RunEventGateVerdict, RunEventVerificationVerdict,
		RunEventEscalationRaised, RunEventHumanInputRequested,
		RunEventHumanVerdictRecorded, RunEventError, RunEventBlocked,
		RunEventCanceled, RunEventFailed, RunEventSucceeded,
	} {
		registered[eventType] = true
	}

	for _, test := range cases {
		t.Run(test.eventType, func(t *testing.T) {
			delete(registered, test.eventType)
			input, targetSeq := schema1RegistryFixture(t, test)
			projection := mustProjectCanonicalFixture(t, input)
			page := mustProjectEventPage(t, projection, targetSeq-1, 1)
			if len(page.Events) != 1 {
				t.Fatalf("sanitized target event count = %d, want 1", len(page.Events))
			}
			var event struct {
				Type string                     `json:"type"`
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(mustMarshalJSON(t, page.Events[0]), &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != test.eventType {
				t.Fatalf("event type = %q, want %q", event.Type, test.eventType)
			}
			if got, want := sortedRawKeys(event.Data), sortedAnyKeys(test.safeData); !reflect.DeepEqual(got, want) {
				t.Fatalf("safe data keys = %v, want exact allowlist %v; event=%s", got, want, mustMarshalJSON(t, page.Events[0]))
			}
			allKeys := recursiveJSONKeys(t, mustMarshalJSON(t, page.Events[0]))
			for _, key := range test.privateKeys {
				if allKeys[key] {
					t.Fatalf("private key %q survived sanitizer: %s", key, mustMarshalJSON(t, page.Events[0]))
				}
			}

			for _, mutation := range []struct {
				name   string
				mutate func([]map[string]any)
			}{
				{
					name: "unknown envelope key",
					mutate: func(events []map[string]any) {
						events[len(events)-1]["privateRoute"] = "/secret"
					},
				},
				{
					name: "unknown data key",
					mutate: func(events []map[string]any) {
						events[len(events)-1]["data"].(map[string]any)["projectionUnknown"] = true
					},
				},
				{
					name: "wrong safe key type",
					mutate: func(events []map[string]any) {
						data := events[len(events)-1]["data"].(map[string]any)
						keys := sortedAnyKeys(data)
						for _, key := range keys {
							if !containsString(test.privateKeys, key) {
								data[key] = map[string]any{"wrong": true}
								return
							}
						}
					},
				},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					events := canonicalLedgerEvents(t, input)
					mutation.mutate(events)
					mutated := replaceCanonicalDocument(t, cloneCanonicalInput(input), CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...))
					_, err := ProjectCanonicalRun(mutated)
					requireProjectionError(t, err, ErrRunEventUnknown)
				})
			}
		})
	}
	if len(registered) != 0 {
		t.Fatalf("schema-1 constants missing parity fixtures: %v", sortedBoolKeys(registered))
	}
}

func TestProjectCanonicalRunSchema1RunStartedWriterParity(t *testing.T) {
	mission := schema1ProjectionInput(t, schema1StartedEvent(projectionTestRunID))
	missionView := ProjectRunView(mustProjectCanonicalFixture(t, mission))
	assertRunRootJSON(t, missionView, `{"kind":"mission","nodeId":"`+projectionTestMissionID+`"}`)

	for _, mutation := range []struct {
		name  string
		key   string
		value any
	}{
		{name: "mission rejects mode", key: "mode", value: "formation"},
		{name: "mission rejects formation id", key: "formationId", value: projectionTestFormationID},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			input := cloneCanonicalInput(mission)
			events := canonicalLedgerEvents(t, input)
			events[0]["data"].(map[string]any)[mutation.key] = mutation.value
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunEventUnknown)
		})
	}

	formationStart := schema1StartedEvent(projectionTestRunID)
	formationStart["missionId"] = "single_" + projectionTestFormationID
	formationStart["beadId"] = ""
	formationData := formationStart["data"].(map[string]any)
	formationData["missionId"] = "single_" + projectionTestFormationID
	formationData["beadId"] = ""
	formationData["mode"] = "formation"
	formationData["formationId"] = projectionTestFormationID
	formation := schema1ProjectionInput(t, formationStart)
	formationView := ProjectRunView(mustProjectCanonicalFixture(t, formation))
	assertRunRootJSON(t, formationView, `{"kind":"formation","nodeId":"`+projectionTestFormationID+`"}`)
	formationPage := mustProjectEventPage(t, mustProjectCanonicalFixture(t, formation), 0, 1)
	formationRaw := mustMarshalJSON(t, formationPage.Events[0])
	if !bytes.Contains(formationRaw, []byte(`"mode":"formation"`)) || !bytes.Contains(formationRaw, []byte(`"formationId":"`+projectionTestFormationID+`"`)) {
		t.Fatalf("isolated-Formation start lost public discriminants: %s", formationRaw)
	}

	for _, mutation := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing mode", mutate: func(data map[string]any) { delete(data, "mode") }},
		{name: "missing formation id", mutate: func(data map[string]any) { delete(data, "formationId") }},
		{name: "empty formation id", mutate: func(data map[string]any) { data["formationId"] = "" }},
		{name: "invalid formation id", mutate: func(data map[string]any) { data["formationId"] = "../work" }},
		{name: "mismatched formation id", mutate: func(data map[string]any) { data["formationId"] = "fmn_other" }},
		{name: "wrong mode", mutate: func(data map[string]any) { data["mode"] = "mission" }},
		{name: "unknown key", mutate: func(data map[string]any) { data["formationPath"] = "/private" }},
	} {
		t.Run("formation rejects "+mutation.name, func(t *testing.T) {
			input := cloneCanonicalInput(formation)
			events := canonicalLedgerEvents(t, input)
			mutation.mutate(events[0]["data"].(map[string]any))
			input = replaceCanonicalDocument(t, input, CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...))
			_, err := ProjectCanonicalRun(input)
			requireProjectionError(t, err, ErrRunEventUnknown)
		})
	}
}

func TestProjectCanonicalRunSchema1OpenDispatchParity(t *testing.T) {
	variants := []struct {
		name       string
		dispatches []any
	}{
		{
			name: "three required ids",
			dispatches: []any{
				map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker"},
				map[string]any{"dispatchId": "dispatch-b", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer"},
			},
		},
		{
			name: "present zero dispatch sequence",
			dispatches: []any{
				map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "dispatchSeq": 0},
				map[string]any{"dispatchId": "dispatch-b", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer"},
			},
		},
	}
	for _, test := range variants {
		t.Run(test.name, func(t *testing.T) {
			projection := mustProjectCanonicalFixture(t, schema1ProjectionInput(t,
				schema1StartedEvent(projectionTestRunID),
				schema1BlockedEvent(projectionTestRunID, 2, true, test.dispatches),
				schema1ResumedEvent(projectionTestRunID, 3, 2, test.dispatches),
			))
			page := mustProjectEventPage(t, projection, 1, 2)
			if len(page.Events) != 2 {
				t.Fatalf("blocked/resumed page events = %d, want 2", len(page.Events))
			}
			blocked := eventDataMember(t, page.Events[0], "openDispatches")
			resumed := eventDataMember(t, page.Events[1], "openDispatches")
			if !bytes.Equal(blocked, resumed) {
				t.Fatalf("resumed carry changed blocked dispatch bytes\nblocked: %s\nresumed: %s", blocked, resumed)
			}
			if !bytes.Equal(blocked, mustMarshalJSON(t, test.dispatches)) {
				t.Fatalf("source order/optional presence changed: got %s want %s", blocked, mustMarshalJSON(t, test.dispatches))
			}
		})
	}

	for _, test := range []struct {
		name       string
		dispatches []any
	}{
		{name: "missing dispatch id", dispatches: []any{map[string]any{"nodeId": projectionTestFormationID, "slotId": "slot_worker"}}},
		{name: "missing node id", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "slotId": "slot_worker"}}},
		{name: "missing slot id", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID}}},
		{name: "invalid id grammar", dispatches: []any{map[string]any{"dispatchId": "../dispatch", "nodeId": projectionTestFormationID, "slotId": "slot_worker"}}},
		{name: "unsafe dispatch sequence", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "dispatchSeq": MaxJSONSafeInteger + 1}}},
		{name: "unknown nested key", dispatches: []any{map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "targetLeaseId": "lease-private"}}},
		{name: "duplicate dispatch id", dispatches: []any{
			map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker"},
			map[string]any{"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_reviewer"},
		}},
	} {
		t.Run("rejects "+test.name, func(t *testing.T) {
			_, err := ProjectCanonicalRun(schema1ProjectionInput(t,
				schema1StartedEvent(projectionTestRunID),
				schema1BlockedEvent(projectionTestRunID, 2, true, test.dispatches),
			))
			requireProjectionError(t, err, ErrRunProjectionInvalid)
		})
	}
}

func TestProjectCanonicalRunSchema2OpenDispatchIsSourceSelected(t *testing.T) {
	schema1 := SafeSchema1OpenDispatch{
		DispatchID: "dispatch-a", NodeID: projectionTestFormationID, SlotID: "slot_worker",
	}
	schema2 := SafeSchema2OpenDispatch{
		DispatchID:                 "dsp_01KXNP6VY3227H78329V52CKF8",
		TargetLeaseID:              "lease_01KXNP6VY3227H78329V52CKF8",
		NodeID:                     projectionTestFormationID,
		Attempt:                    1,
		SlotID:                     "slot_worker",
		AgentID:                    "agent_worker",
		BindingID:                  "binding_worker",
		SessionTargetID:            "target_opaque",
		TargetFingerprint:          strings.Repeat("a", 64),
		DispatchSeq:                3,
		PeekCapabilityState:        "none",
		LatestCapabilityGeneration: "0",
		LatestCapabilityIssuedSeq:  0,
		LatestSteeringGeneration:   "0",
		InterruptState:             "none",
	}
	var schema1Arm SafeOpenDispatch = schema1
	var schema2Arm SafeOpenDispatch = schema2
	schema1Raw := mustMarshalJSON(t, schema1Arm)
	schema2Raw := mustMarshalJSON(t, schema2Arm)
	for _, forbidden := range []string{"targetLeaseId", "attempt", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "peekCapabilityState", "latestCapabilityGeneration", "latestSteeringGeneration", "interruptState"} {
		if bytes.Contains(schema1Raw, []byte(`"`+forbidden+`"`)) {
			t.Fatalf("schema-1 dispatch acquired schema-2 member %q: %s", forbidden, schema1Raw)
		}
	}
	for _, required := range []string{"targetLeaseId", "attempt", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "peekCapabilityState", "latestCapabilityGeneration", "latestSteeringGeneration", "interruptState"} {
		if !bytes.Contains(schema2Raw, []byte(`"`+required+`"`)) {
			t.Fatalf("schema-2 dispatch lost member %q: %s", required, schema2Raw)
		}
	}

	wrongForSchema1 := map[string]any{}
	if err := json.Unmarshal(schema2Raw, &wrongForSchema1); err != nil {
		t.Fatal(err)
	}
	_, err := ProjectCanonicalRun(schema1ProjectionInput(t,
		schema1StartedEvent(projectionTestRunID),
		schema1BlockedEvent(projectionTestRunID, 2, true, []any{wrongForSchema1}),
	))
	requireProjectionError(t, err, ErrRunProjectionInvalid)

	wrongForSchema2 := map[string]any{}
	if err := json.Unmarshal(schema1Raw, &wrongForSchema2); err != nil {
		t.Fatal(err)
	}
	schema2Block := schema2Event(projectionTestRunID, 3, "run_blocked", map[string]any{
		"reason": "blocked", "blockScope": "run", "resumeAllowed": true,
		"resumePolicy": "reattach_only", "openDispatches": []any{wrongForSchema2},
		"retryTargets": []any{}, "nextEpoch": 1,
	})
	_, err = ProjectCanonicalRun(schema2ProjectionInput(t, true, schema2Block))
	requireProjectionError(t, err, ErrRunProjectionInvalid)
}

func TestProjectCanonicalRunSafeRunEventHasExactDiscriminants(t *testing.T) {
	type eventTypeCase struct {
		source  int
		literal string
		typeOf  reflect.Type
	}
	tests := append(schema2SafeEventTypes(), schema1SafeEventTypes()...)
	if len(tests) != 58 {
		t.Fatalf("source-specific safe event arms = %d, want 37 schema-2 + 21 schema-1", len(tests))
	}
	byLiteral := map[string][]int{}
	safeInterface := reflect.TypeOf((*SafeRunEvent)(nil)).Elem()
	if safeInterface.Kind() != reflect.Interface || safeInterface.NumMethod() == 0 {
		t.Fatalf("SafeRunEvent = %v, want closed marker interface with no raw map fallback", safeInterface)
	}
	if reflect.TypeOf(map[string]any{}).Implements(safeInterface) {
		t.Fatal("map[string]any implements SafeRunEvent raw fallback")
	}
	for _, test := range tests {
		byLiteral[test.literal] = append(byLiteral[test.literal], test.source)
		value := reflect.New(test.typeOf)
		field := value.Elem().FieldByName("Type")
		if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.String {
			t.Fatalf("%s has no exported string Type discriminant", test.typeOf)
		}
		field.SetString(test.literal)
		event, ok := value.Interface().(SafeRunEvent)
		if !ok {
			t.Fatalf("%s does not implement SafeRunEvent", test.typeOf)
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(mustMarshalJSON(t, event), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Type != test.literal {
			t.Fatalf("%s JSON type = %q, want %q", test.typeOf, envelope.Type, test.literal)
		}
	}
	if len(byLiteral) != 41 {
		t.Fatalf("safe event discriminants = %d, want exactly 41: %v", len(byLiteral), sortedIntSliceKeys(byLiteral))
	}
	for literal, sources := range byLiteral {
		sort.Ints(sources)
		if isSchema1OnlyEvent(literal) {
			if !reflect.DeepEqual(sources, []int{1}) {
				t.Fatalf("schema-1-only %q sources = %v, want [1]", literal, sources)
			}
			continue
		}
		if isSharedSafeEvent(literal) {
			if !reflect.DeepEqual(sources, []int{1, 2}) {
				t.Fatalf("shared %q sources = %v, want schema-specific [1 2]", literal, sources)
			}
			continue
		}
		if !reflect.DeepEqual(sources, []int{2}) {
			t.Fatalf("schema-2-only %q sources = %v, want [2]", literal, sources)
		}
	}
}

func schema1StartedEvent(runID string) map[string]any {
	return map[string]any{
		"ts":        "2026-07-20T10:00:00Z",
		"runId":     runID,
		"seq":       uint64(1),
		"type":      RunEventStarted,
		"actor":     "agent:test",
		"boardId":   projectionTestBoardID,
		"boardRev":  uint64(7),
		"missionId": projectionTestMissionID,
		"beadId":    "ctx-7i1.1",
		"epoch":     uint64(0),
		"attempt":   uint64(0),
		"data": map[string]any{
			"boardSlug":        projectionTestBoardSlug,
			"boardPath":        ".formations/boards/projection.formation.toml",
			"boardRev":         uint64(7),
			"snapshot":         ".formations/runs/projection/" + runID + ".snapshot.toml",
			"bindingsSnapshot": ".formations/runs/projection/" + runID + ".bindings.toml",
			"missionId":        projectionTestMissionID,
			"beadId":           "ctx-7i1.1",
			"objective":        "Project the run",
			"limits": map[string]any{
				"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false,
			},
		},
	}
}

func schema1Event(runID string, sequence uint64, eventType string, data map[string]any) map[string]any {
	event := map[string]any{
		"ts":        fmt.Sprintf("2026-07-20T10:00:%02dZ", sequence-1),
		"runId":     runID,
		"seq":       sequence,
		"type":      eventType,
		"actor":     "agent:test",
		"boardId":   projectionTestBoardID,
		"boardRev":  uint64(7),
		"missionId": projectionTestMissionID,
		"beadId":    "ctx-7i1.1",
		"epoch":     uint64(0),
		"attempt":   uint64(1),
		"data":      cloneStringAnyMap(data),
	}
	switch eventType {
	case RunEventNodeWaiting, RunEventNodeStarted, RunEventSlotDispatch, RunEventAdapterSend, RunEventSlotResult, RunEventNodeOutput:
		event["nodeId"] = projectionTestFormationID
	case RunEventGateEvaluating, RunEventGateVerdict, RunEventHumanInputRequested, RunEventHumanVerdictRecorded:
		event["nodeId"] = projectionTestFormationID
		event["gateId"] = projectionTestGateID
	case RunEventEscalationRaised, RunEventBlocked:
		event["nodeId"] = projectionTestFormationID
	}
	if eventType == RunEventResumed {
		event["epoch"] = uint64(1)
	}
	return event
}

func schema2Event(runID string, sequence uint64, eventType string, data map[string]any) map[string]any {
	event := schema1Event(runID, sequence, eventType, data)
	event["schema"] = uint64(2)
	event["authoritySchema"] = uint64(2)
	event["writerFence"] = uint64(1)
	return event
}

func schema1NodeStartedEvent(runID string, sequence uint64) map[string]any {
	return schema1Event(runID, sequence, RunEventNodeStarted, map[string]any{
		"nodeKind": "formation",
		"inputRefs": []any{
			map[string]any{"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out", "toPortId": "port_in", "outputSeq": 1},
		},
		"reason": "initial",
	})
}

func schema1NodeOutputEvent(runID string, sequence uint64, status string) map[string]any {
	return schema1Event(runID, sequence, RunEventNodeOutput, map[string]any{
		"status": status,
		"text":   "done",
		"outputs": map[string]any{
			"port_out": map[string]any{"text": "done"},
		},
		"reason": "completed",
	})
}

func schema1GateEvaluatingEvent(runID string, sequence uint64) map[string]any {
	return schema1Event(runID, sequence, RunEventGateEvaluating, map[string]any{
		"kinds":     []string{"human"},
		"criterion": "Approve the result",
		"inputRef": map[string]any{
			"edgeId": "edge_work_gate", "fromNodeId": projectionTestFormationID, "fromPortId": "port_out", "toPortId": "in", "outputSeq": 1,
		},
		"judgeChain": []string{},
	})
}

func schema1GateVerdictEvent(runID string, sequence uint64, verdict string) map[string]any {
	return schema1Event(runID, sequence, RunEventGateVerdict, map[string]any{
		"verdict":     verdict,
		"perKind":     map[string]any{"human": verdict},
		"routePort":   verdict,
		"routedEdges": []string{},
		"reason":      "reviewed",
		"inputRef": map[string]any{
			"edgeId": "edge_work_gate", "fromNodeId": projectionTestFormationID, "fromPortId": "port_out", "toPortId": "in", "outputSeq": 1,
		},
	})
}

func schema1EscalationEvent(runID string, sequence uint64, blocks bool) map[string]any {
	return schema1Event(runID, sequence, RunEventEscalationRaised, map[string]any{
		"trigger": "sentinel", "severity": "needs-attention", "reason": "operator review",
		"source": "agent", "nodeId": projectionTestFormationID, "gateId": "", "blocks": blocks,
	})
}

func schema1BlockedEvent(runID string, sequence uint64, resumeAllowed bool, dispatches []any) map[string]any {
	return schema1Event(runID, sequence, RunEventBlocked, map[string]any{
		"reason": "blocked", "code": "operator_review", "boundary": "engine",
		"blockedNodeId": projectionTestFormationID, "blockedGateId": "", "waitingNodes": []string{},
		"recoverable": resumeAllowed, "resumeAllowed": resumeAllowed, "resumePolicy": "explicit",
		"openDispatches": dispatches, "nextEpoch": uint64(1),
	})
}

func schema1ResumedEvent(runID string, sequence, blockedSequence uint64, dispatches []any) map[string]any {
	return schema1Event(runID, sequence, RunEventResumed, map[string]any{
		"resumedFromSeq": blockedSequence, "resumedBy": "human:test", "resumeMode": "reattach",
		"reason": "continue", "openDispatches": dispatches,
	})
}

const schema1ProjectionSnapshot = `schema = 1
id = "brd_projection"
slug = "projection"
title = "Projection fixture"
rev = 7

[[mission]]
id = "mis_root"
title = "Root"
goal = "Project the run"
beadId = "ctx-7i1.1"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_in"
label = "Input"

[[formation.output]]
id = "port_out"
label = "Output"

[[formation.slot]]
id = "slot_worker"
label = "Worker"
agentId = "worker"
harness = "codex"
controller = true

[[formation.slot]]
id = "slot_reviewer"
label = "Reviewer"
agentId = "reviewer"
harness = "codex"
controller = false

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
criterion = "Approve the result"

[[connection]]
id = "edge_root_work"
from = "mis_root:out"
to = "fmn_work:port_in"

[[connection]]
id = "edge_work_gate"
from = "fmn_work:port_out"
to = "gate_review:in"
`

const schema1ProjectionBindings = `schema = 1
runId = "run_01KXNP6VY3227H78329V52CKF8"
boardId = "brd_projection"
boardSlug = "projection"
boardRev = 7
missionId = "mis_root"

[[binding]]
nodeId = "fmn_work"
slotId = "slot_worker"
agentId = "worker"
harness = "codex"
sessionStem = "worker"
cardPath = "/private/worker.toml"
cardSha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

[[binding]]
nodeId = "fmn_work"
slotId = "slot_reviewer"
agentId = "reviewer"
harness = "codex"
sessionStem = "reviewer"
cardPath = "/private/reviewer.toml"
cardSha256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
`

func schema1ProjectionInput(t *testing.T, events ...map[string]any) CanonicalRunReadInput {
	t.Helper()
	return CanonicalRunReadInput{
		RunID:  projectionTestRunID,
		Source: CanonicalRunSourceSchema1,
		Documents: []CanonicalInputDocument{
			canonicalInputDocument(CanonicalInputRoleSchema1Ledger, marshalProjectionLedger(t, events...)),
			canonicalInputDocument(CanonicalInputRoleSchema1GraphSnapshot, []byte(schema1ProjectionSnapshot)),
			canonicalInputDocument(CanonicalInputRoleSchema1BindingsSnapshot, []byte(schema1ProjectionBindings)),
		},
	}
}

func schema2ProjectionInput(t *testing.T, activated bool, extra ...map[string]any) CanonicalRunReadInput {
	t.Helper()
	graph := []byte(`schema = 2
id = "brd_projection"
slug = "projection"
title = "Projection fixture"
rev = 7

[[mission]]
id = "mis_root"
title = "Root"
goal = "Project the run"
beadId = "ctx-7i1.1"

[[authoredConfigManifest]]
classification = "authored_config"
sourceKind = "mission_objective"
nodeId = "mis_root"
encoding = "mission-objective-utf8-v1"
mediaType = "text/markdown"
sha256 = "` + projectionSHA256([]byte("Project the run")) + `"
`)
	bindings := []byte(`schema = 2
runId = "` + projectionTestRunID + `"
boardId = "` + projectionTestBoardID + `"
boardRev = 7
`)
	graphHash := projectionSHA256(graph)
	bindingsHash := projectionSHA256(bindings)
	policy := canonicalJSON(t, map[string]any{
		"policySchema": 1, "policyRev": 1, "priorPolicySha256": "", "state": "configured",
		"maxActiveRuns": 1, "maxQueuedRuns": 1,
	})
	policyHash := projectionSHA256(policy)
	commandRecord := schema2CommandRecord(t, projectionTestCommandID, "start", "applied", projectionTestRunID)
	var command map[string]any
	if err := json.Unmarshal(commandRecord, &command); err != nil {
		t.Fatal(err)
	}
	commandPayloadHash := command["commandPayloadSha256"].(string)
	rootHash := strings.Repeat("1", 64)
	runBootstrap := canonicalJSON(t, map[string]any{
		"runBootstrapSchema":      1,
		"workspaceAuthorityId":    projectionTestWorkspaceID,
		"runId":                   projectionTestRunID,
		"runAuthorityId":          projectionTestAuthorityID,
		"graphSnapshotEncoding":   "run-graph-snapshot-toml-v1",
		"graphSnapshotSha256":     graphHash,
		"privateBindingsEncoding": "run-private-bindings-toml-v1",
		"privateBindingsSha256":   bindingsHash,
	})
	started := schema2Event(projectionTestRunID, 1, "run_started", map[string]any{
		"workspaceAuthorityId":    projectionTestWorkspaceID,
		"workspaceAdmissionSeq":   1,
		"admissionPolicyRev":      1,
		"admissionPolicySha256":   policyHash,
		"admissionCommandId":      projectionTestCommandID,
		"commandPayloadSha256":    commandPayloadHash,
		"boardSlug":               projectionTestBoardSlug,
		"boardPath":               ".formations/boards/projection.formation.toml",
		"sourceBoardSchema":       2,
		"snapshotSchema":          2,
		"runAuthorityId":          projectionTestAuthorityID,
		"graphSnapshotSha256":     graphHash,
		"privateBindingsSha256":   bindingsHash,
		"bindingProjectionSha256": strings.Repeat("2", 64),
		"runRoot":                 map[string]any{"kind": "mission", "nodeId": projectionTestMissionID},
		"rootInputProjection": map[string]any{
			"classification": "authored_config", "sourceKind": "mission_objective",
			"encoding": "mission-objective-utf8-v1", "mediaType": "text/markdown",
			"sha256": projectionSHA256([]byte("Project the run")), "text": "Project the run",
		},
		"limits": map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false},
	})
	events := []map[string]any{started}
	if activated {
		events = append(events, schema2Event(projectionTestRunID, 2, "run_activated", map[string]any{
			"workspaceAdmissionSeq": 1, "admissionPolicyRev": 1,
			"admissionPolicySha256": policyHash, "reason": "immediate",
		}))
	}
	events = append(events, extra...)
	return CanonicalRunReadInput{
		RunID:  projectionTestRunID,
		Source: CanonicalRunSourceSchema2,
		Documents: []CanonicalInputDocument{
			canonicalInputDocument(CanonicalInputRoleSchema2WorkspaceRegistry, canonicalJSON(t, map[string]any{
				"registrySchema": 1, "recordRev": 1, "priorGeneration": nil,
				"entries": []any{map[string]any{
					"workspaceAuthorityId": projectionTestWorkspaceID, "configuredPath": "/workspace",
					"device": "1", "inode": "2", "workspaceRootIdentitySha256": rootHash,
				}},
			})),
			canonicalInputDocument(CanonicalInputRoleSchema2WorkspaceBootstrap, canonicalJSON(t, map[string]any{
				"bootstrapSchema": 1, "workspaceAuthorityId": projectionTestWorkspaceID,
				"rootIdentityEncoding": "workspace-root-identity-v1", "workspaceRootIdentitySha256": rootHash,
			})),
			canonicalInputDocument(CanonicalInputRoleSchema2WorkspaceAuthority, canonicalJSON(t, map[string]any{
				"recordRev": 1, "priorGeneration": nil, "authoritySchema": 2,
				"workspaceAuthorityId": projectionTestWorkspaceID,
				"rootIdentityEncoding": "workspace-root-identity-v1", "workspaceRootIdentitySha256": rootHash,
				"nextWriterFence": 2, "nextAdmissionSeq": 2,
				"admissionPolicyRef": map[string]any{"policyRev": 1, "policySha256": policyHash},
			})),
			canonicalInputDocument(CanonicalInputRoleSchema2AdmissionPolicy, policy),
			canonicalInputDocument(CanonicalInputRoleSchema2RunBootstrap, runBootstrap),
			canonicalInputDocument(CanonicalInputRoleSchema2GraphSnapshot, graph),
			canonicalInputDocument(CanonicalInputRoleSchema2PrivateBindings, bindings),
			canonicalInputDocument(CanonicalInputRoleSchema2Ledger, marshalProjectionLedger(t, events...)),
			canonicalInputDocument(CanonicalInputRoleSchema2CommandRecord, commandRecord),
		},
	}
}

func schema2CommandRecord(t *testing.T, commandID, kind, state, runID string) []byte {
	t.Helper()
	payload := canonicalCommandPayload(kind, runID)
	payloadRaw := canonicalJSON(t, payload)
	record := map[string]any{
		"commandSchema":        1,
		"recordRev":            1,
		"priorGeneration":      nil,
		"commandEncoding":      "run-command-jcs-v1",
		"commandId":            commandID,
		"commandKind":          kind,
		"commandPayload":       payload,
		"commandPayloadSha256": projectionSHA256(payloadRaw),
		"admittedWriterFence":  1,
		"stateWriterFence":     9,
		"state":                state,
	}
	switch state {
	case "applied":
		record["runId"] = runID
		record["effectSeq"] = 7
		record["outcomeWriterFence"] = 9
		if kind == "start" {
			record["decisionAdmissionPolicyRef"] = map[string]any{"policyRev": 1, "policySha256": strings.Repeat("a", 64)}
		} else {
			record["decisionAdmissionPolicyRef"] = nil
		}
	case "rejected":
		record["rejectionCode"] = "fixture_rejected"
		record["outcomeWriterFence"] = 9
		if kind == "start" {
			record["decisionAdmissionPolicyRef"] = map[string]any{"policyRev": 1, "policySha256": strings.Repeat("a", 64)}
		} else {
			record["decisionAdmissionPolicyRef"] = nil
		}
	}
	return canonicalJSON(t, record)
}

func canonicalCommandPayload(kind, runID string) map[string]any {
	base := map[string]any{
		"kind": kind, "authoritySchema": 2, "actor": "human:test", "workspaceAuthorityId": projectionTestWorkspaceID,
	}
	switch kind {
	case "start":
		base["boardId"] = projectionTestBoardID
		base["runRoot"] = map[string]any{"kind": "mission", "nodeId": projectionTestMissionID}
		base["expectedBoardRev"] = 7
		base["expectedBoardETag"] = strings.Repeat("b", 64)
		base["limits"] = map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false}
	case "resume":
		base["runId"] = runID
		base["blockedSeq"] = 3
		base["resumeMode"] = "reattach"
		base["reason"] = "continue"
	case "cancel":
		base["runId"] = runID
		base["expectedLastSeq"] = 3
		base["reason"] = "stop"
	case "verdict":
		base["runId"] = runID
		base["gateId"] = projectionTestGateID
		base["requestedSeq"] = 3
		base["verdict"] = "pass"
		base["reason"] = "approved"
	}
	return base
}

func canonicalCommandInput(t *testing.T, commandID, kind, state string) CanonicalCommandReadInput {
	t.Helper()
	record := schema2CommandRecord(t, commandID, kind, state, projectionTestRunID)
	var decoded map[string]any
	if err := json.Unmarshal(record, &decoded); err != nil {
		t.Fatal(err)
	}
	return CanonicalCommandReadInput{
		Source: CanonicalRunSourceSchema2,
		Submitted: SubmittedCommandIdentity{
			CommandID: commandID, CommandKind: kind, CommandPayloadSHA256: decoded["commandPayloadSha256"].(string),
		},
		Record: record,
	}
}

func schema1SafeRegistryCases() []schema1SafeRegistryCase {
	inputRef := map[string]any{
		"edgeId": "edge_root_work", "fromNodeId": projectionTestMissionID, "fromPortId": "out",
		"toPortId": "port_in", "outputSeq": 1,
		"ref": "/private/input", "text": "private input", "reportRef": "/private/report", "artifactRef": "/private/artifact",
	}
	return []schema1SafeRegistryCase{
		{
			eventType: RunEventStarted,
			safeData: map[string]any{
				"boardSlug": projectionTestBoardSlug, "boardRev": 7, "missionId": projectionTestMissionID,
				"beadId": "ctx-7i1.1", "limits": map[string]any{"maxDispatch": 20, "maxAttempts": 3, "wallClockSeconds": 1800, "redact": false},
			},
			privateKeys: []string{"boardPath", "snapshot", "bindingsSnapshot", "objective"},
		},
		{
			eventType: RunEventResumed,
			safeData: map[string]any{
				"resumedFromSeq": 2, "resumedBy": "human:test", "resumeMode": "reattach", "reason": "continue", "openDispatches": []any{},
			},
		},
		{eventType: RunEventNodeWaiting, safeData: map[string]any{"neededInputs": 1, "readyInputs": 0, "totalInputs": 1, "waitingFor": []string{"edge_root_work"}}},
		{
			eventType:   RunEventNodeStarted,
			safeData:    map[string]any{"nodeKind": "formation", "inputRefs": []any{inputRef}, "reason": "initial", "brief": map[string]any{"goal": "private"}},
			privateKeys: []string{"brief", "ref", "text", "reportRef", "artifactRef"},
		},
		{
			eventType: RunEventOrchestrationTeam,
			safeData: map[string]any{
				"mode": "orchestrated", "controllerSlot": "slot_worker",
				"controller": map[string]any{"slotId": "slot_worker", "label": "Worker", "agentId": "worker", "harness": "codex", "sessionStem": "private", "sessionRef": "private"},
				"workers":    []any{map[string]any{"slotId": "slot_reviewer", "label": "Reviewer", "agentId": "reviewer", "harness": "codex", "sessionStem": "private", "sessionRef": "private"}},
				"socket":     "/private/socket", "cwd": "/private/cwd",
			},
			privateKeys: []string{"socket", "cwd", "sessionStem", "sessionRef"},
		},
		{
			eventType: RunEventPeerPlane,
			safeData: map[string]any{
				"mode": "peer", "peers": []any{map[string]any{"slotId": "slot_worker", "label": "Worker", "agentId": "worker", "harness": "codex", "sessionStem": "private", "sessionRef": "private"}},
				"path": "/private", "socket": "/private/socket", "cwd": "/private/cwd",
			},
			privateKeys: []string{"path", "socket", "cwd", "sessionStem", "sessionRef"},
		},
		{
			eventType: RunEventSlotDispatch,
			safeData: map[string]any{
				"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "agentId": "worker", "harness": "codex",
				"phase": "solo", "promptSha256": strings.Repeat("a", 64), "nativeAck": true, "recordedBeforeSend": true,
				"sessionStem": "private", "sessionRef": "private", "promptRef": "private",
			},
			privateKeys: []string{"sessionStem", "sessionRef", "promptRef"},
		},
		{
			eventType: RunEventAdapterSend,
			safeData: map[string]any{
				"adapter": "tmux", "dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "phase": "solo",
				"socketSha256": strings.Repeat("b", 64), "promptSha256": strings.Repeat("a", 64), "sent": true, "sessionRef": "private",
			},
			privateKeys: []string{"sessionRef"},
		},
		{
			eventType: RunEventSlotResult,
			safeData: map[string]any{
				"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "status": "ok",
				"sentinel": map[string]any{"runId": projectionTestRunID, "status": "done", "artifact": "/private/artifact"},
			},
			privateKeys: []string{"artifact"},
		},
		{
			eventType: RunEventNodeOutput,
			safeData: map[string]any{
				"status": "done", "text": "done", "reason": "completed", "reportRef": "/private/report",
				"outputs": map[string]any{"port_out": map[string]any{"text": "done", "ref": "/private", "reportRef": "/private/report", "artifactRef": "/private/artifact"}},
			},
			privateKeys: []string{"reportRef", "ref", "artifactRef"},
		},
		{
			eventType:   RunEventGateEvaluating,
			safeData:    map[string]any{"kinds": []string{"human"}, "criterion": "Approve", "inputRef": inputRef, "judgeChain": []string{}},
			privateKeys: []string{"ref", "text", "reportRef", "artifactRef"},
		},
		{
			eventType: RunEventGateVerdict,
			safeData: map[string]any{
				"verdict": "pass", "perKind": map[string]any{"human": "pass"}, "routePort": "pass", "routedEdges": []string{}, "reason": "approved", "inputRef": inputRef,
			},
			privateKeys: []string{"ref", "text", "reportRef", "artifactRef"},
		},
		{eventType: RunEventVerificationVerdict, safeData: map[string]any{"verificationId": "verification-1", "verdict": "pass"}},
		{eventType: RunEventEscalationRaised, safeData: map[string]any{"trigger": "sentinel", "severity": "needs-attention", "reason": "review", "source": "agent", "nodeId": projectionTestFormationID, "gateId": "", "blocks": false}},
		{
			eventType: RunEventHumanInputRequested,
			safeData: map[string]any{
				"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "choices": []string{"pass", "fail"}, "requestedBy": "agent:test", "inputRef": inputRef,
				"codeVerdict": "pass", "codeReason": "checks pass", "codePerKind": map[string]any{"code": "pass"}, "timeoutSeconds": 300, "prompt": "private prompt",
			},
			privateKeys: []string{"prompt", "ref", "text", "reportRef", "artifactRef"},
		},
		{eventType: RunEventHumanVerdictRecorded, safeData: map[string]any{"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "verdict": "pass", "reason": "approved", "requestedSeq": 2, "decidedBy": "human:test"}},
		{eventType: RunEventError, safeData: map[string]any{"code": "fixture", "message": "safe message", "reason": "safe reason", "boundary": "schema", "nodeId": projectionTestFormationID, "gateId": "", "slotId": "", "dispatchId": "", "recoverable": true, "relatedSeq": 1}},
		{eventType: RunEventBlocked, safeData: map[string]any{"reason": "blocked", "code": "fixture", "boundary": "engine", "blockedNodeId": projectionTestFormationID, "blockedGateId": "", "waitingNodes": []string{}, "recoverable": true, "resumeAllowed": true, "resumePolicy": "explicit", "openDispatches": []any{}, "nextEpoch": 1}},
		{eventType: RunEventCanceled, safeData: map[string]any{"reason": "stop", "requestedBy": "human:test", "softInterruptedSlots": []string{}, "final": true}},
		{eventType: RunEventFailed, safeData: map[string]any{"code": "fixture", "reason": "failed", "boundary": "engine", "recoverable": false, "relatedSeq": 1, "final": true}},
		{eventType: RunEventSucceeded, safeData: map[string]any{"final": true, "mode": "mission", "formationId": "", "missionId": projectionTestMissionID, "reason": "done", "summaryRef": "/private/summary", "outputRefs": []string{"/private/output"}, "artifactRefs": []string{"/private/artifact"}}, privateKeys: []string{"summaryRef", "outputRefs", "artifactRefs"}},
	}
}

func schema1RegistryFixture(t *testing.T, test schema1SafeRegistryCase) (CanonicalRunReadInput, uint64) {
	t.Helper()
	if test.eventType == RunEventStarted {
		started := schema1StartedEvent(projectionTestRunID)
		data := started["data"].(map[string]any)
		for key, value := range test.safeData {
			data[key] = cloneAny(value)
		}
		return schema1ProjectionInput(t, started), 1
	}
	events := []map[string]any{schema1StartedEvent(projectionTestRunID)}
	appendEvent := func(event map[string]any) {
		event["seq"] = uint64(len(events) + 1)
		events = append(events, event)
	}
	switch test.eventType {
	case RunEventResumed:
		appendEvent(schema1BlockedEvent(projectionTestRunID, 2, true, []any{}))
	case RunEventSlotDispatch:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
	case RunEventAdapterSend:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
		appendEvent(schema1Event(projectionTestRunID, 3, RunEventSlotDispatch, map[string]any{
			"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "agentId": "worker", "harness": "codex", "phase": "solo",
			"promptSha256": strings.Repeat("a", 64), "nativeAck": true, "recordedBeforeSend": true,
		}))
	case RunEventSlotResult:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
		appendEvent(schema1Event(projectionTestRunID, 3, RunEventSlotDispatch, map[string]any{
			"dispatchId": "dispatch-a", "nodeId": projectionTestFormationID, "slotId": "slot_worker", "agentId": "worker", "harness": "codex", "phase": "solo",
			"promptSha256": strings.Repeat("a", 64), "nativeAck": true, "recordedBeforeSend": true,
		}))
	case RunEventNodeOutput:
		appendEvent(schema1NodeStartedEvent(projectionTestRunID, 2))
	case RunEventGateVerdict:
		appendEvent(schema1GateEvaluatingEvent(projectionTestRunID, 2))
	case RunEventHumanInputRequested:
		appendEvent(schema1GateEvaluatingEvent(projectionTestRunID, 2))
	case RunEventHumanVerdictRecorded:
		appendEvent(schema1GateEvaluatingEvent(projectionTestRunID, 2))
		appendEvent(schema1Event(projectionTestRunID, 3, RunEventHumanInputRequested, map[string]any{
			"gateId": projectionTestGateID, "nodeId": projectionTestFormationID, "choices": []string{"pass", "fail"}, "requestedBy": "agent:test",
			"inputRef":    map[string]any{"edgeId": "edge_work_gate", "fromNodeId": projectionTestFormationID, "fromPortId": "port_out", "toPortId": "in", "outputSeq": 1},
			"codeVerdict": "pass", "codeReason": "checks pass", "codePerKind": map[string]any{"code": "pass"}, "timeoutSeconds": 300,
		}))
	}
	target := schema1Event(projectionTestRunID, uint64(len(events)+1), test.eventType, test.safeData)
	appendEvent(target)
	return schema1ProjectionInput(t, events...), uint64(len(events))
}

func schema2SafeEventTypes() []struct {
	source  int
	literal string
	typeOf  reflect.Type
} {
	return []struct {
		source  int
		literal string
		typeOf  reflect.Type
	}{
		{2, "run_started", reflect.TypeOf(SafeSchema2RunStartedEvent{})},
		{2, "run_activated", reflect.TypeOf(SafeSchema2RunActivatedEvent{})},
		{2, "run_resumed", reflect.TypeOf(SafeSchema2RunResumedEvent{})},
		{2, "node_waiting", reflect.TypeOf(SafeSchema2NodeWaitingEvent{})},
		{2, "node_input_ignored", reflect.TypeOf(SafeSchema2NodeInputIgnoredEvent{})},
		{2, "node_started", reflect.TypeOf(SafeSchema2NodeStartedEvent{})},
		{2, "slot_binding_observed", reflect.TypeOf(SafeSchema2SlotBindingObservedEvent{})},
		{2, "slot_dispatch", reflect.TypeOf(SafeSchema2SlotDispatchEvent{})},
		{2, "slot_peek_capability_issued", reflect.TypeOf(SafeSchema2SlotPeekCapabilityIssuedEvent{})},
		{2, "slot_steering_started", reflect.TypeOf(SafeSchema2SlotSteeringStartedEvent{})},
		{2, "slot_steering_ended", reflect.TypeOf(SafeSchema2SlotSteeringEndedEvent{})},
		{2, "slot_peek_capability_revoked", reflect.TypeOf(SafeSchema2SlotPeekCapabilityRevokedEvent{})},
		{2, "slot_reconciliation_interrupt", reflect.TypeOf(SafeSchema2SlotReconciliationInterruptEvent{})},
		{2, "slot_reconciliation_interrupt_outcome", reflect.TypeOf(SafeSchema2SlotReconciliationInterruptOutcomeEvent{})},
		{2, "slot_result", reflect.TypeOf(SafeSchema2SlotResultEvent{})},
		{2, "formation_result", reflect.TypeOf(SafeSchema2FormationResultEvent{})},
		{2, "tool_dispatch", reflect.TypeOf(SafeSchema2ToolDispatchEvent{})},
		{2, "tool_process_launch", reflect.TypeOf(SafeSchema2ToolProcessLaunchEvent{})},
		{2, "tool_result", reflect.TypeOf(SafeSchema2ToolResultEvent{})},
		{2, "node_output", reflect.TypeOf(SafeSchema2NodeOutputEvent{})},
		{2, "gate_evaluating", reflect.TypeOf(SafeSchema2GateEvaluatingEvent{})},
		{2, "gate_kind_result", reflect.TypeOf(SafeSchema2GateKindResultEvent{})},
		{2, "judge_result", reflect.TypeOf(SafeSchema2JudgeResultEvent{})},
		{2, "judge_attempt_failed", reflect.TypeOf(SafeSchema2JudgeAttemptFailedEvent{})},
		{2, "gate_verdict", reflect.TypeOf(SafeSchema2GateVerdictEvent{})},
		{2, "artifact_attached", reflect.TypeOf(SafeSchema2ArtifactAttachedEvent{})},
		{2, "artifact_observed", reflect.TypeOf(SafeSchema2ArtifactObservedEvent{})},
		{2, "escalation_raised", reflect.TypeOf(SafeSchema2EscalationRaisedEvent{})},
		{2, "human_input_requested", reflect.TypeOf(SafeSchema2HumanInputRequestedEvent{})},
		{2, "human_verdict_recorded", reflect.TypeOf(SafeSchema2HumanVerdictRecordedEvent{})},
		{2, "error", reflect.TypeOf(SafeSchema2ErrorEvent{})},
		{2, "run_blocked", reflect.TypeOf(SafeSchema2RunBlockedEvent{})},
		{2, "run_cancel_requested", reflect.TypeOf(SafeSchema2RunCancelRequestedEvent{})},
		{2, "run_canceled", reflect.TypeOf(SafeSchema2RunCanceledEvent{})},
		{2, "run_failure_reconciliation_started", reflect.TypeOf(SafeSchema2RunFailureReconciliationStartedEvent{})},
		{2, "run_failed", reflect.TypeOf(SafeSchema2RunFailedEvent{})},
		{2, "run_succeeded", reflect.TypeOf(SafeSchema2RunSucceededEvent{})},
	}
}

func schema1SafeEventTypes() []struct {
	source  int
	literal string
	typeOf  reflect.Type
} {
	return []struct {
		source  int
		literal string
		typeOf  reflect.Type
	}{
		{1, "run_started", reflect.TypeOf(SafeSchema1RunStartedEvent{})},
		{1, "run_resumed", reflect.TypeOf(SafeSchema1RunResumedEvent{})},
		{1, "node_waiting", reflect.TypeOf(SafeSchema1NodeWaitingEvent{})},
		{1, "node_started", reflect.TypeOf(SafeSchema1NodeStartedEvent{})},
		{1, "orchestration_team", reflect.TypeOf(SafeSchema1OrchestrationTeamEvent{})},
		{1, "peer_plane", reflect.TypeOf(SafeSchema1PeerPlaneEvent{})},
		{1, "slot_dispatch", reflect.TypeOf(SafeSchema1SlotDispatchEvent{})},
		{1, "adapter_send", reflect.TypeOf(SafeSchema1AdapterSendEvent{})},
		{1, "slot_result", reflect.TypeOf(SafeSchema1SlotResultEvent{})},
		{1, "node_output", reflect.TypeOf(SafeSchema1NodeOutputEvent{})},
		{1, "gate_evaluating", reflect.TypeOf(SafeSchema1GateEvaluatingEvent{})},
		{1, "gate_verdict", reflect.TypeOf(SafeSchema1GateVerdictEvent{})},
		{1, "verification_verdict", reflect.TypeOf(SafeSchema1VerificationVerdictEvent{})},
		{1, "escalation_raised", reflect.TypeOf(SafeSchema1EscalationRaisedEvent{})},
		{1, "human_input_requested", reflect.TypeOf(SafeSchema1HumanInputRequestedEvent{})},
		{1, "human_verdict_recorded", reflect.TypeOf(SafeSchema1HumanVerdictRecordedEvent{})},
		{1, "error", reflect.TypeOf(SafeSchema1ErrorEvent{})},
		{1, "run_blocked", reflect.TypeOf(SafeSchema1RunBlockedEvent{})},
		{1, "run_canceled", reflect.TypeOf(SafeSchema1RunCanceledEvent{})},
		{1, "run_failed", reflect.TypeOf(SafeSchema1RunFailedEvent{})},
		{1, "run_succeeded", reflect.TypeOf(SafeSchema1RunSucceededEvent{})},
	}
}

func isSchema1OnlyEvent(eventType string) bool {
	switch eventType {
	case "orchestration_team", "peer_plane", "adapter_send", "verification_verdict":
		return true
	default:
		return false
	}
}

func isSharedSafeEvent(eventType string) bool {
	switch eventType {
	case "run_started", "run_resumed", "node_waiting", "node_started", "slot_dispatch", "slot_result", "node_output", "gate_evaluating", "gate_verdict", "escalation_raised", "human_input_requested", "human_verdict_recorded", "error", "run_blocked", "run_canceled", "run_failed", "run_succeeded":
		return true
	default:
		return false
	}
}

func mustProjectCanonicalFixture(t *testing.T, input CanonicalRunReadInput) CanonicalRunProjection {
	t.Helper()
	projection, err := ProjectCanonicalRun(input)
	if err != nil {
		t.Fatalf("project canonical fixture: %v", err)
	}
	return projection
}

func mustProjectEventPage(t *testing.T, projection CanonicalRunProjection, since uint64, limit int) RunEventPage {
	t.Helper()
	page, err := ProjectRunEventPage(projection, since, limit)
	if err != nil {
		t.Fatalf("project event page: %v", err)
	}
	return page
}

func findProjectedNode(t *testing.T, view RunView, nodeID string) RunNodeView {
	t.Helper()
	for _, node := range view.Nodes {
		if node.NodeID == nodeID {
			return node
		}
	}
	t.Fatalf("node %q absent from projection: %+v", nodeID, view.Nodes)
	return RunNodeView{}
}

func findProjectedGate(t *testing.T, view RunView, gateID string) RunGateView {
	t.Helper()
	for _, gate := range view.Gates {
		if gate.GateID == gateID {
			return gate
		}
	}
	t.Fatalf("gate %q absent from projection: %+v", gateID, view.Gates)
	return RunGateView{}
}

func requireProjectionError(t *testing.T, err, target error) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want typed %v", target)
	}
	if !errors.Is(err, target) {
		t.Fatalf("error = %T %v, want errors.Is(..., %v)", err, err, target)
	}
}

func canonicalInputDocument(role CanonicalInputRole, raw []byte) CanonicalInputDocument {
	owned := append([]byte(nil), raw...)
	return CanonicalInputDocument{Role: role, Bytes: owned, SHA256: projectionSHA256(owned)}
}

func projectionSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func marshalProjectionLedger(t *testing.T, events ...map[string]any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	for _, event := range events {
		buffer.Write(canonicalJSON(t, event))
		buffer.WriteByte('\n')
	}
	return buffer.Bytes()
}

func canonicalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustMarshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneCanonicalInput(input CanonicalRunReadInput) CanonicalRunReadInput {
	clone := input
	clone.Documents = make([]CanonicalInputDocument, len(input.Documents))
	for index, document := range input.Documents {
		clone.Documents[index] = document
		clone.Documents[index].Bytes = append([]byte(nil), document.Bytes...)
	}
	return clone
}

func cloneCanonicalCommandInput(input CanonicalCommandReadInput) CanonicalCommandReadInput {
	clone := input
	clone.Record = append([]byte(nil), input.Record...)
	return clone
}

func canonicalDocumentByRole(t *testing.T, input CanonicalRunReadInput, role CanonicalInputRole) CanonicalInputDocument {
	t.Helper()
	for _, document := range input.Documents {
		if document.Role == role {
			clone := document
			clone.Bytes = append([]byte(nil), document.Bytes...)
			return clone
		}
	}
	t.Fatalf("canonical input has no %s document", role)
	return CanonicalInputDocument{}
}

func removeCanonicalRole(documents []CanonicalInputDocument, role CanonicalInputRole) []CanonicalInputDocument {
	filtered := make([]CanonicalInputDocument, 0, len(documents))
	for _, document := range documents {
		if document.Role != role {
			filtered = append(filtered, document)
		}
	}
	return filtered
}

func replaceCanonicalDocument(t *testing.T, input CanonicalRunReadInput, role CanonicalInputRole, raw []byte) CanonicalRunReadInput {
	t.Helper()
	return replaceCanonicalDocumentObject(input, role, canonicalInputDocument(role, raw))
}

func replaceCanonicalDocumentObject(input CanonicalRunReadInput, role CanonicalInputRole, replacement CanonicalInputDocument) CanonicalRunReadInput {
	for index := range input.Documents {
		if input.Documents[index].Role == role {
			input.Documents[index] = replacement
			return input
		}
	}
	input.Documents = append(input.Documents, replacement)
	return input
}

func canonicalLedgerEvents(t *testing.T, input CanonicalRunReadInput) []map[string]any {
	t.Helper()
	var raw []byte
	for _, document := range input.Documents {
		if document.Role == CanonicalInputRoleSchema1Ledger || document.Role == CanonicalInputRoleSchema2Ledger {
			raw = document.Bytes
			break
		}
	}
	if raw == nil {
		t.Fatal("canonical input has no ledger")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var events []map[string]any
	for decoder.More() {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			t.Fatalf("decode canonical ledger: %v", err)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		t.Fatal("canonical ledger contains no events")
	}
	return events
}

func withEventSequence(event map[string]any, sequence uint64) map[string]any {
	clone := cloneStringAnyMap(event)
	clone["seq"] = sequence
	return clone
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	clone := make(map[string]any, len(input))
	for key, value := range input {
		clone[key] = cloneAny(value)
	}
	return clone
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneAny(item)
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	case []map[string]any:
		clone := make([]map[string]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneStringAnyMap(item)
		}
		return clone
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return value
	}
}

func mutateCommandRecord(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var record map[string]any
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	mutate(record)
	return canonicalJSON(t, record)
}

func assertRunRootJSON(t *testing.T, view RunView, want string) {
	t.Helper()
	got := mustMarshalJSON(t, view.Identity.RunRoot)
	if string(got) != want {
		t.Fatalf("runRoot = %s, want %s", got, want)
	}
}

func eventDataMember(t *testing.T, event SafeRunEvent, member string) json.RawMessage {
	t.Helper()
	var decoded struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(mustMarshalJSON(t, event), &decoded); err != nil {
		t.Fatal(err)
	}
	value, ok := decoded.Data[member]
	if !ok {
		t.Fatalf("event data has no %q: %s", member, mustMarshalJSON(t, event))
	}
	return value
}

func mustSafeProjectedEvent(t *testing.T, projection CanonicalRunProjection, index int) SafeRunEvent {
	t.Helper()
	if index < 0 || index >= len(projection.events) || projection.events[index].omitted {
		t.Fatalf("projected safe event index %d invalid", index)
	}
	return projection.events[index].safe
}

func cloneSafeEventWithSequence(t *testing.T, event SafeRunEvent, sequence uint64) SafeRunEvent {
	t.Helper()
	value := reflect.ValueOf(event)
	pointer := value.Kind() == reflect.Pointer
	if pointer {
		value = value.Elem()
	}
	clone := reflect.New(value.Type()).Elem()
	clone.Set(value)
	sequenceField := clone.FieldByName("Seq")
	if !sequenceField.IsValid() || !sequenceField.CanSet() {
		t.Fatalf("safe event %T has no settable Seq field", event)
	}
	switch sequenceField.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		sequenceField.SetUint(sequence)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sequenceField.SetInt(int64(sequence))
	default:
		t.Fatalf("safe event %T Seq kind = %s", event, sequenceField.Kind())
	}
	var result any
	if pointer {
		copyPointer := reflect.New(clone.Type())
		copyPointer.Elem().Set(clone)
		result = copyPointer.Interface()
	} else {
		result = clone.Interface()
	}
	safe, ok := result.(SafeRunEvent)
	if !ok {
		t.Fatalf("cloned %T does not implement SafeRunEvent", result)
	}
	return safe
}

func cloneSafeEventWithMessage(t *testing.T, event SafeRunEvent, message string) SafeRunEvent {
	t.Helper()
	value := reflect.ValueOf(event)
	pointer := value.Kind() == reflect.Pointer
	if pointer {
		value = value.Elem()
	}
	clone := reflect.New(value.Type()).Elem()
	clone.Set(value)
	data := clone.FieldByName("Data")
	if !data.IsValid() {
		t.Fatalf("safe error event %T has no Data field", event)
	}
	if data.Kind() == reflect.Pointer {
		dataClone := reflect.New(data.Elem().Type())
		dataClone.Elem().Set(data.Elem())
		data.Set(dataClone)
		data = data.Elem()
	}
	messageField := data.FieldByName("Message")
	if !messageField.IsValid() || !messageField.CanSet() || messageField.Kind() != reflect.String {
		t.Fatalf("safe error event %T has no settable Data.Message", event)
	}
	messageField.SetString(message)
	var result any
	if pointer {
		copyPointer := reflect.New(clone.Type())
		copyPointer.Elem().Set(clone)
		result = copyPointer.Interface()
	} else {
		result = clone.Interface()
	}
	safe, ok := result.(SafeRunEvent)
	if !ok {
		t.Fatalf("cloned %T does not implement SafeRunEvent", result)
	}
	return safe
}

func projectionWithRepeatedSafeEvents(t *testing.T, base CanonicalRunProjection, count int) CanonicalRunProjection {
	t.Helper()
	safe := mustSafeProjectedEvent(t, base, 0)
	projection := base
	projection.events = make([]projectedEvent, count)
	for index := range projection.events {
		sequence := uint64(index + 1)
		projection.events[index] = projectedEvent{scanSeq: sequence, safe: cloneSafeEventWithSequence(t, safe, sequence)}
	}
	projection.latestSeq = uint64(count)
	projection.view.Cursor = uint64(count)
	return projection
}

func projectionWithCompletePageSize(t *testing.T, base CanonicalRunProjection, target int) CanonicalRunProjection {
	t.Helper()
	projection := base
	safe := cloneSafeEventWithSequence(t, mustSafeProjectedEvent(t, base, 1), 2)
	view := ProjectRunView(base)
	candidate := RunEventPage{
		Schema: RunEventPageSchema, RunID: view.RunID, Generation: view.Generation, Source: view.Source,
		Cursor: 2, HasMore: false, Events: []SafeRunEvent{safe},
	}
	baseSize := len(mustMarshalJSON(t, candidate))
	if target < baseSize {
		t.Fatalf("target page size %d smaller than fixed envelope %d", target, baseSize)
	}
	safe = cloneSafeEventWithMessage(t, safe, strings.Repeat("x", target-baseSize+1))
	candidate.Events = []SafeRunEvent{safe}
	actual := len(mustMarshalJSON(t, candidate))
	difference := actual - target
	if difference < 0 || difference > target-baseSize+1 {
		t.Fatalf("unable to size complete page: target=%d actual=%d", target, actual)
	}
	safe = cloneSafeEventWithMessage(t, safe, strings.Repeat("x", target-baseSize+1-difference))
	candidate.Events = []SafeRunEvent{safe}
	if got := len(mustMarshalJSON(t, candidate)); got != target {
		t.Fatalf("complete candidate bytes = %d, want %d", got, target)
	}
	projection.events = append([]projectedEvent(nil), base.events...)
	projection.events[1] = projectedEvent{scanSeq: 2, safe: safe}
	projection.latestSeq = 2
	projection.view.Cursor = 2
	return projection
}

func projectionFingerprint(t *testing.T, projection CanonicalRunProjection) string {
	t.Helper()
	var buffer strings.Builder
	buffer.Write(mustMarshalJSON(t, projection.view))
	fmt.Fprintf(&buffer, "|latest=%d", projection.latestSeq)
	for _, event := range projection.events {
		fmt.Fprintf(&buffer, "|seq=%d|omitted=%v|", event.scanSeq, event.omitted)
		if !event.omitted {
			buffer.Write(mustMarshalJSON(t, event.safe))
		}
	}
	return buffer.String()
}

func recursiveJSONKeys(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				keys[key] = true
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return keys
}

func sortedRawKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAnyKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedIntSliceKeys(values map[string][]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
