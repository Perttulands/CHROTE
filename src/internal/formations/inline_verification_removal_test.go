package formations

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRemoveFormationVerificationRejectionMatrixPreservesDefinitions(t *testing.T) {
	const slug = "verification-rejection"
	const boardRaw = `schema = 1
id = "brd_verification_rejection"
slug = "verification-rejection"
title = "Verification rejection"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.output]]
id = "port_work_out"
label = "Output"

[formation.verification]
id = "ver_work"
kinds = ["code"]
criterion = "Check the work"
onFail = "block"

[[gate]]
id = "gate_wired"
title = "Wired review"
kinds = ["human"]
criterion = "Review the work"

[[gate]]
id = "gate_unwired"
title = "Unwired review"
kinds = ["human"]
criterion = "Review something else"

[[connection]]
id = "edge_work_review"
from = "fmn_work:port_work_out"
to = "gate_wired:in"
`
	const layoutRaw = `schema = 1
boardId = "brd_verification_rejection"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "fmn_work"
x = 120
y = 80

[[node]]
id = "gate_wired"
x = 440
y = 80

[[node]]
id = "gate_unwired"
x = 440
y = 240

[[edge]]
id = "edge_work_review"
lane = "220"
`

	tests := []struct {
		name              string
		replacementGateID string
		staleETag         bool
		staleRevision     bool
		wantErr           error
	}{
		{name: "stale ETag", replacementGateID: "gate_wired", staleETag: true, wantErr: ErrConflict},
		{name: "stale revision", replacementGateID: "gate_wired", staleRevision: true, wantErr: ErrConflict},
		{name: "missing replacement Gate", replacementGateID: "gate_missing", wantErr: ErrLegacyInlineVerificationRequiresMigration},
		{name: "non-Gate replacement id", replacementGateID: "fmn_work", wantErr: ErrLegacyInlineVerificationRequiresMigration},
		{name: "existing unwired Gate", replacementGateID: "gate_unwired", wantErr: ErrLegacyInlineVerificationRequiresMigration},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)

			beforeBoard, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			beforeLayout, err := store.ReadLayout(slug)
			if err != nil {
				t.Fatalf("read layout: %v", err)
			}
			beforeBoardBytes := readFile(t, store.BoardPath(slug))
			beforeLayoutBytes := readFile(t, store.LayoutPath(slug))
			opts := WriteOptions{ExpectedETag: beforeBoard.ETag, ExpectedRev: beforeBoard.Rev}
			if test.staleETag {
				opts.ExpectedETag = "stale"
			}
			if test.staleRevision {
				opts.ExpectedRev++
			}

			_, err = store.RemoveFormationVerification(slug, FormationVerificationRemovalRequest{
				FormationID:       "fmn_work",
				ReplacementGateID: test.replacementGateID,
				UpdatedBy:         "agent:test",
			}, opts)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("remove verification error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == ErrConflict && err.Error() != ErrConflict.Error() {
				t.Fatalf("remove verification error = %q, want stable conflict error %q", err, ErrConflict)
			}
			if errors.Is(test.wantErr, ErrLegacyInlineVerificationRequiresMigration) && !strings.Contains(err.Error(), LegacyInlineVerificationMigrationCode) {
				t.Fatalf("remove verification error = %v, want stable code %q", err, LegacyInlineVerificationMigrationCode)
			}
			if after := readFile(t, store.BoardPath(slug)); after != beforeBoardBytes {
				t.Fatalf("rejected removal changed board bytes\nbefore:\n%s\nafter:\n%s", beforeBoardBytes, after)
			}
			if after := readFile(t, store.LayoutPath(slug)); after != beforeLayoutBytes {
				t.Fatalf("rejected removal changed layout bytes\nbefore:\n%s\nafter:\n%s", beforeLayoutBytes, after)
			}
			afterBoard, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read board after rejection: %v", err)
			}
			afterLayout, err := store.ReadLayout(slug)
			if err != nil {
				t.Fatalf("read layout after rejection: %v", err)
			}
			if afterBoard.Rev != beforeBoard.Rev {
				t.Fatalf("board revision after rejection = %d, want %d", afterBoard.Rev, beforeBoard.Rev)
			}
			if !reflect.DeepEqual(afterBoard.Gates, beforeBoard.Gates) {
				t.Fatalf("Gate definitions changed after rejection\nbefore: %+v\nafter: %+v", beforeBoard.Gates, afterBoard.Gates)
			}
			if !reflect.DeepEqual(afterBoard.Connections, beforeBoard.Connections) {
				t.Fatalf("connections changed after rejection\nbefore: %+v\nafter: %+v", beforeBoard.Connections, afterBoard.Connections)
			}
			if !reflect.DeepEqual(afterLayout, beforeLayout) {
				t.Fatalf("layout changed after rejection\nbefore: %+v\nafter: %+v", beforeLayout, afterLayout)
			}
		})
	}
}

func TestRemoveFormationVerificationMixedSectionAndInlineRepresentationRejectsBeforeWrite(t *testing.T) {
	const slug = "verification-mixed"
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath(slug), `schema = 1
id = "brd_verification_mixed"
slug = "verification-mixed"
title = "Verification mixed"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"
verification = { id = "ver_work", kinds = ["code"], criterion = "Check the work", onFail = "block" }

[[formation.output]]
id = "port_work_out"
label = "Output"

[formation.verification.extra]
futureField = "remove the descendant but retain the fence"

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
criterion = "Review the work"

[[connection]]
id = "edge_work_review"
from = "fmn_work:port_work_out"
to = "gate_review:in"
`)
	writeFixture(t, store.LayoutPath(slug), `schema = 1
boardId = "brd_verification_mixed"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "fmn_work"
x = 120
y = 80

[[node]]
id = "gate_review"
x = 440
y = 80

[[edge]]
id = "edge_work_review"
lane = "220"
`)

	beforeBoard, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	beforeLayout, err := store.ReadLayout(slug)
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	beforeBoardBytes := readFile(t, store.BoardPath(slug))
	beforeLayoutBytes := readFile(t, store.LayoutPath(slug))
	formation, ok := findFormation(beforeBoard.Formations, "fmn_work")
	if !ok || formation.Verification == nil {
		t.Fatalf("mixed representation fixture formation = %+v, want parsed inline verification", formation)
	}
	if !strings.Contains(beforeBoardBytes, `verification = {`) || !strings.Contains(beforeBoardBytes, `[formation.verification.extra]`) {
		t.Fatalf("mixed representation fixture must contain both inline and descendant-table verification\n%s", beforeBoardBytes)
	}

	_, err = store.RemoveFormationVerification(slug, FormationVerificationRemovalRequest{
		FormationID:       "fmn_work",
		ReplacementGateID: "gate_review",
		UpdatedBy:         "agent:test",
	}, WriteOptions{ExpectedETag: beforeBoard.ETag, ExpectedRev: beforeBoard.Rev})
	if !errors.Is(err, ErrLegacyInlineVerificationRequiresMigration) || !strings.Contains(err.Error(), LegacyInlineVerificationMigrationCode) {
		t.Fatalf("mixed representation removal error = %v, want stable post-parse migration rejection", err)
	}
	if !strings.Contains(err.Error(), "still contains inline verification after removal") {
		t.Fatalf("mixed representation removal error = %v, want post-parse fence rejection", err)
	}
	if after := readFile(t, store.BoardPath(slug)); after != beforeBoardBytes {
		t.Fatalf("mixed representation rejection changed board bytes\nbefore:\n%s\nafter:\n%s", beforeBoardBytes, after)
	}
	if after := readFile(t, store.LayoutPath(slug)); after != beforeLayoutBytes {
		t.Fatalf("mixed representation rejection changed layout bytes\nbefore:\n%s\nafter:\n%s", beforeLayoutBytes, after)
	}
	afterBoard, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board after rejection: %v", err)
	}
	afterLayout, err := store.ReadLayout(slug)
	if err != nil {
		t.Fatalf("read layout after rejection: %v", err)
	}
	if afterBoard.Rev != beforeBoard.Rev {
		t.Fatalf("board revision after mixed representation rejection = %d, want %d", afterBoard.Rev, beforeBoard.Rev)
	}
	if !reflect.DeepEqual(afterBoard.Gates, beforeBoard.Gates) {
		t.Fatalf("Gate definitions changed after mixed representation rejection\nbefore: %+v\nafter: %+v", beforeBoard.Gates, afterBoard.Gates)
	}
	if !reflect.DeepEqual(afterBoard.Connections, beforeBoard.Connections) {
		t.Fatalf("connections changed after mixed representation rejection\nbefore: %+v\nafter: %+v", beforeBoard.Connections, afterBoard.Connections)
	}
	if !reflect.DeepEqual(afterLayout, beforeLayout) {
		t.Fatalf("mixed representation rejection changed layout\nbefore: %+v\nafter: %+v", beforeLayout, afterLayout)
	}
}

func TestRemoveFormationVerificationAcceptsExplicitWiredGate(t *testing.T) {
	const slug = "verification-valid"
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	raw := s4VerificationBoardFixture("block") + `
[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
criterion = "Review the work"

[[connection]]
id = "edge_work_review"
from = "fmn_work:port_work_out"
to = "gate_review:in"
`
	raw = strings.Replace(raw, `slug = "session-search"`, `slug = "verification-valid"`, 1)
	writeFixture(t, store.BoardPath(slug), raw)
	before, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	after, err := store.RemoveFormationVerification(slug, FormationVerificationRemovalRequest{
		FormationID:       "fmn_work",
		ReplacementGateID: "gate_review",
		UpdatedBy:         "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("remove verification through explicit wired Gate: %v", err)
	}
	formation, ok := findFormation(after.Formations, "fmn_work")
	if !ok || formation.Verification != nil {
		t.Fatalf("formation after valid removal = %+v, want legacy verification removed", formation)
	}
	if after.Rev != before.Rev+1 {
		t.Fatalf("board revision after valid removal = %d, want %d", after.Rev, before.Rev+1)
	}
	if !reflect.DeepEqual(after.Gates, before.Gates) || !reflect.DeepEqual(after.Connections, before.Connections) {
		t.Fatalf("valid removal changed Gate definitions or connections\nbefore: %+v\nafter: %+v", before, after)
	}
}
