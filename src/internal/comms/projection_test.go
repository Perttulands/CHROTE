package comms

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestProjectRoomBuildsCanonicalProjectionFromLedger(t *testing.T) {
	workspace := t.TempDir()
	writeRoomFixture(t, workspace, "project", "dogfood", []map[string]any{
		{"seq": 1, "kind": "room_initialized", "actor": "room-system", "text": "Build the cockpit", "timestamp": "2026-07-04T00:00:01Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"schema": "prototype"}},
		{"seq": 2, "kind": "boundary_pinned", "actor": "Perttu", "text": "Use structured claims before writing files.", "timestamp": "2026-07-04T00:00:02Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"pinned": true}},
		{"seq": 3, "kind": "task_claimed", "actor": "Builder", "text": "Build cockpit shell", "timestamp": "2026-07-04T00:00:03Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"claim_status": "claimed", "category": "ui", "reservations": []string{"prototype/structured-room-cockpit/index.html"}, "expected_artifacts": []string{"prototype/structured-room-cockpit/index.html"}, "verification_command": "node --test prototype/structured-room-cockpit/test/*.mjs"}},
		{"seq": 4, "kind": "room_post", "actor": "Builder", "text": "@Reviewer please check the boundary.", "timestamp": "2026-07-04T00:00:04Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{}},
		{"seq": 5, "kind": "passive_mention", "actor": "room-system", "text": "Passive mention for Reviewer from event #4", "timestamp": "2026-07-04T00:00:05Z", "visible_to": []string{"Reviewer"}, "to": []string{"Reviewer"}, "metadata": map[string]any{"mentioned": "Reviewer", "source_seq": 4, "passive": true, "tmux_injection": false}},
		{"seq": 6, "kind": "artifact_recorded", "actor": "Builder", "text": "Cockpit shell", "timestamp": "2026-07-04T00:00:06Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"claim_seq": 3, "path": "/tmp/cockpit/index.html"}},
		{"seq": 7, "kind": "task_done", "actor": "Builder", "text": "Shell verified.", "timestamp": "2026-07-04T00:00:07Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"claim_seq": 3, "claim_status": "done", "verification": "node --test prototype/structured-room-cockpit/test/*.mjs"}},
		{"seq": 8, "kind": "task_claimed", "actor": "CodexC", "text": "Worker may hang", "timestamp": "2026-07-04T00:00:08Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"claim_status": "claimed", "reservations": []string{"stale/output.md"}}},
		{"seq": 9, "kind": "task_claim_resolved", "actor": "Perttu", "text": "Cancelled stale claim after worker hung.", "timestamp": "2026-07-04T00:00:09Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "to": []string{"CodexC"}, "metadata": map[string]any{"claim_seq": 8, "claim_owner": "CodexC", "claim_status": "cancelled", "resolved_by": "Perttu"}},
		{"seq": 10, "kind": "artifact_salvaged", "actor": "Reviewer", "text": "Salvaged useful output.", "timestamp": "2026-07-04T00:00:10Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"claim_seq": 8, "claim_owner": "CodexC", "path": "/tmp/stale/output.md", "salvaged": true, "salvaged_by": "Reviewer", "verified_by": "Reviewer", "verification": "manual review"}},
		{"seq": 11, "kind": "task_claimed", "actor": "Reviewer", "text": "Reserve the whole cockpit tree", "timestamp": "2026-07-04T00:00:11Z", "visible_to": []string{"Perttu", "Builder", "Reviewer", "CodexC"}, "metadata": map[string]any{"claim_status": "claimed", "reservations": []string{"prototype/structured-room-cockpit"}, "reservation_warnings": []map[string]any{{"type": "broad-directory-reservation", "severity": "medium", "path": "prototype/structured-room-cockpit"}}}},
	})

	projection, err := NewStore(workspace).ProjectRoom("project:dogfood", ProjectionOptions{})
	if err != nil {
		t.Fatalf("ProjectRoom: %v", err)
	}

	if projection.Schema != "mission-room.projection.v1" {
		t.Fatalf("schema = %q", projection.Schema)
	}
	if projection.RoomRef != "project:dogfood" || projection.RoomKind != "project" || projection.RoomID != "dogfood" {
		t.Fatalf("room identity = %+v", projection)
	}
	if projection.Summary.EventCount != 10 {
		t.Fatalf("visible event count = %d, want private mention filtered out", projection.Summary.EventCount)
	}
	if projection.Summary.ClaimCount != 3 || projection.Summary.DoneClaimCount != 1 || projection.Summary.OpenClaimCount != 1 {
		t.Fatalf("claim summary = %+v", projection.Summary)
	}
	if projection.LatestBoundary == nil || projection.LatestBoundary.Seq != 2 || projection.LatestBoundary.Text == "" {
		t.Fatalf("latest boundary = %+v", projection.LatestBoundary)
	}
	if len(projection.Mentions) != 0 {
		t.Fatalf("default projection leaked private mention: %+v", projection.Mentions)
	}
	if projection.Claims[0].ArtifactSeqs[0] != 6 || projection.Claims[1].SalvagedArtifactSeqs[0] != 10 {
		t.Fatalf("claim artifact links = %+v", projection.Claims)
	}
	if projection.Claims[1].Status != "cancelled" || projection.Claims[1].ResolvedBy != "Perttu" {
		t.Fatalf("cancelled claim = %+v", projection.Claims[1])
	}
	if len(projection.Risks) != 1 || projection.Risks[0].Type != "broad-active-reservation" || projection.Risks[0].ClaimSeq != 11 {
		t.Fatalf("risks = %+v", projection.Risks)
	}
}

func TestProjectRoomPrivateInboxRequiresExplicitActor(t *testing.T) {
	workspace := t.TempDir()
	writeRoomFixture(t, workspace, "project", "dogfood", []map[string]any{
		{"seq": 1, "kind": "room_initialized", "actor": "room-system", "text": "Brief", "timestamp": "2026-07-04T00:00:01Z", "visible_to": []string{"Perttu", "Reviewer"}, "metadata": map[string]any{}},
		{"seq": 2, "kind": "passive_mention", "actor": "room-system", "text": "Private mention", "timestamp": "2026-07-04T00:00:02Z", "visible_to": []string{"Reviewer"}, "to": []string{"Reviewer"}, "metadata": map[string]any{"mentioned": "Reviewer", "source_seq": 1, "passive": true}},
	})

	publicProjection, err := NewStore(workspace).ProjectRoom("project:dogfood", ProjectionOptions{})
	if err != nil {
		t.Fatalf("ProjectRoom public: %v", err)
	}
	if len(publicProjection.Messages) != 1 || len(publicProjection.Mentions) != 0 {
		t.Fatalf("public projection leaked private inbox: messages=%+v mentions=%+v", publicProjection.Messages, publicProjection.Mentions)
	}

	reviewerProjection, err := NewStore(workspace).ProjectRoom("project:dogfood", ProjectionOptions{IncludePrivateFor: "Reviewer"})
	if err != nil {
		t.Fatalf("ProjectRoom reviewer: %v", err)
	}
	if len(reviewerProjection.Messages) != 2 || len(reviewerProjection.Mentions) != 1 {
		t.Fatalf("reviewer projection missing private inbox: messages=%+v mentions=%+v", reviewerProjection.Messages, reviewerProjection.Mentions)
	}
}

func TestProjectRunRoomProjectsFinalFormationsLedgerAsReadOnlySource(t *testing.T) {
	workspace := t.TempDir()
	writeRunLedgerFixture(t, workspace, "demo", "run_abc123", []map[string]any{
		{"seq": 1, "type": "run_started", "actor": "Perttu", "ts": "2026-07-04T01:00:01Z", "runId": "run_abc123", "boardId": "brd_demo", "boardRev": 3, "missionId": "mission_ui", "beadId": "ctx-q8x.2", "data": map[string]any{"boardSlug": "demo", "objective": "Render projection"}},
		{"seq": 2, "type": "node_output", "actor": "CodexA", "ts": "2026-07-04T01:00:02Z", "runId": "run_abc123", "nodeId": "fmn_ui", "data": map[string]any{"text": "projection rendered", "outputPort": "done"}},
		{"seq": 3, "type": "run_succeeded", "actor": "room-system", "ts": "2026-07-04T01:00:03Z", "runId": "run_abc123", "data": map[string]any{"summary": "green"}},
	})

	projection, err := NewStore(workspace).ProjectRoom("run:run_abc123", ProjectionOptions{})
	if err != nil {
		t.Fatalf("ProjectRoom run: %v", err)
	}

	if projection.RoomRef != "run:run_abc123" || projection.RoomKind != "run" || projection.RoomID != "run_abc123" {
		t.Fatalf("run room identity = %+v", projection)
	}
	if projection.Source.Kind != "formations-run-ledger" || !projection.Source.ReadOnly || !projection.Source.RunFinal || projection.Source.RunStatus != "succeeded" {
		t.Fatalf("run room source = %+v", projection.Source)
	}
	if projection.Summary.EventCount != 3 {
		t.Fatalf("event count = %d, want 3", projection.Summary.EventCount)
	}
	if projection.Messages[1].Type != "node_output" || projection.Messages[1].Metadata["nodeId"] != "fmn_ui" {
		t.Fatalf("node output message = %+v", projection.Messages[1])
	}
}

func TestProjectRunRoomDoesNotAdoptSchema2WorkspaceLedger(t *testing.T) {
	workspace := t.TempDir()
	runID := "run_01KXNP6VY3227H78329V52CKF8"
	writeRunLedgerFixture(t, workspace, "demo", runID, []map[string]any{
		{"schema": 2, "authoritySchema": 2, "writerFence": 1, "seq": 1, "type": "run_started", "actor": "agent:test", "ts": "2026-07-18T00:00:00Z", "runId": runID, "data": map[string]any{"boardSlug": "demo"}},
	})
	store := NewStoreWithFormations(workspace, formations.NewStore(workspace))

	projection, err := store.ProjectRoom("run:"+runID, ProjectionOptions{})
	if !errors.Is(err, formations.ErrRuntimeAuthorityNonAuthorizing) {
		t.Fatalf("schema-2 run projection = %+v err=%v, want typed non-authorizing rejection", projection, err)
	}
}

func TestProjectRoomRejectsUnsafeRefs(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	if _, err := store.ProjectRoom("project:../evil", ProjectionOptions{}); err == nil {
		t.Fatal("unsafe room ref accepted")
	}
}

func TestNonRunRoomReadersRejectExternalLedgerAliases(t *testing.T) {
	for _, attack := range []string{"parent symlink", "final symlink", "hard link"} {
		t.Run(attack, func(t *testing.T) {
			base := t.TempDir()
			workspace := filepath.Join(base, "workspace")
			outside := filepath.Join(base, "outside")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatal(err)
			}
			externalLedger := filepath.Join(outside, "private.ndjson")
			writeLedgerFixture(t, externalLedger, []map[string]any{
				{"seq": 1, "kind": "room_post", "actor": "private", "text": "must not escape", "timestamp": "2026-07-19T00:00:00Z"},
			})

			ledgerDirectory := filepath.Join(workspace, ".formations", "comms", "project")
			switch attack {
			case "parent symlink":
				externalComms := filepath.Join(outside, "comms")
				writeLedgerFixture(t, filepath.Join(externalComms, "project", "dogfood.ndjson"), []map[string]any{
					{"seq": 1, "kind": "room_post", "actor": "private", "text": "must not escape", "timestamp": "2026-07-19T00:00:00Z"},
				})
				if err := os.MkdirAll(filepath.Join(workspace, ".formations"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(externalComms, filepath.Join(workspace, ".formations", "comms")); err != nil {
					t.Fatal(err)
				}
			case "final symlink":
				if err := os.MkdirAll(ledgerDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(externalLedger, filepath.Join(ledgerDirectory, "dogfood.ndjson")); err != nil {
					t.Fatal(err)
				}
			case "hard link":
				if err := os.MkdirAll(ledgerDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(externalLedger, filepath.Join(ledgerDirectory, "dogfood.ndjson")); err != nil {
					t.Fatal(err)
				}
			}

			store := NewStore(workspace)
			if events, err := store.readEvents("project", "dogfood"); err == nil || len(events) != 0 {
				t.Fatalf("readEvents consumed external ledger: events=%+v err=%v", events, err)
			}
			if messages, err := store.Messages("project:dogfood", MessageOptions{}); err == nil || len(messages.Messages) != 0 {
				t.Fatalf("Messages consumed external ledger: messages=%+v err=%v", messages, err)
			}
			if export, err := store.Export("project:dogfood", "ndjson", ""); err == nil || len(export.Events) != 0 || export.Markdown != "" {
				t.Fatalf("Export consumed external ledger: export=%+v err=%v", export, err)
			}
		})
	}
}

func TestNonRunRoomReaderAllowsConfiguredWorkspaceRootSymlink(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	writeRoomFixture(t, workspace, "project", "dogfood", []map[string]any{
		{"seq": 1, "kind": "room_post", "actor": "Builder", "text": "workspace alias is valid", "timestamp": "2026-07-19T00:00:00Z"},
	})
	workspaceAlias := filepath.Join(base, "workspace-alias")
	if err := os.Symlink(workspace, workspaceAlias); err != nil {
		t.Fatal(err)
	}

	projection, err := NewStore(workspaceAlias).ProjectRoom("project:dogfood", ProjectionOptions{})
	if err != nil {
		t.Fatalf("ProjectRoom through configured workspace root symlink: %v", err)
	}
	if len(projection.Messages) != 1 || projection.Messages[0].Text != "workspace alias is valid" {
		t.Fatalf("projection through workspace alias = %+v", projection.Messages)
	}
}

func writeRoomFixture(t *testing.T, workspace, kind, id string, events []map[string]any) {
	t.Helper()
	path := filepath.Join(workspace, ".formations", "comms", kind, id+".ndjson")
	writeLedgerFixture(t, path, events)
}

func writeRunLedgerFixture(t *testing.T, workspace, boardSlug, runID string, events []map[string]any) {
	t.Helper()
	path := filepath.Join(workspace, ".formations", "runs", boardSlug, runID+".ndjson")
	writeLedgerFixture(t, path, events)
}

func writeLedgerFixture(t *testing.T, path string, events []map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
}
