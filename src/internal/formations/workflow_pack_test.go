package formations

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstantiateWorkflowPackCopiesAssetsAndFreshensGraphIdentity(t *testing.T) {
	workspace := t.TempDir()
	packDir := writeWorkflowPackFixture(t)
	store := NewStore(workspace)
	store.Now = fixedClock()

	result, err := store.InstantiateWorkflowPack(packDir, WorkflowInstantiateRequest{
		Slug:      "landing-page",
		Title:     "Landing page design",
		Goal:      "Design a launch page for a local-first agent cockpit.",
		UpdatedBy: "agent:test",
	})
	if err != nil {
		t.Fatalf("instantiate workflow pack: %v", err)
	}
	if result.Pack.ID != "test-design" || result.Pack.Version != "0.1.0" {
		t.Fatalf("pack = %+v, want test-design 0.1.0", result.Pack)
	}
	if result.Board.Slug != "landing-page" || result.Board.Title != "Landing page design" || result.Board.Rev != 1 {
		t.Fatalf("board identity = %+v", result.Board)
	}
	if result.Board.ID == "brd_template" || !strings.HasPrefix(result.Board.ID, "brd_") {
		t.Fatalf("board id = %q, want fresh brd_ id", result.Board.ID)
	}
	if result.Board.WorkflowPackID != "test-design" || result.Board.WorkflowPackVersion != "0.1.0" || result.Board.WorkflowPackDigest == "" {
		t.Fatalf("workflow provenance = %q %q %q", result.Board.WorkflowPackID, result.Board.WorkflowPackVersion, result.Board.WorkflowPackDigest)
	}
	if len(result.Board.Missions) != 1 || result.Board.Missions[0].ID == "mis_template" || result.Board.Missions[0].Goal != "Design a launch page for a local-first agent cockpit." {
		t.Fatalf("mission = %+v, want fresh id and requested goal", result.Board.Missions)
	}
	if len(result.Board.Formations) != 1 || result.Board.Formations[0].ID == "fmn_designer" {
		t.Fatalf("formations = %+v, want fresh formation id", result.Board.Formations)
	}
	if result.Board.Formations[0].Title != "Designer mentions fmn_designer literally" {
		t.Fatalf("formation title = %q, want human text preserved", result.Board.Formations[0].Title)
	}
	if len(result.Board.Gates) != 1 || result.Board.Gates[0].ID == "gate_quality" {
		t.Fatalf("gates = %+v, want fresh gate id", result.Board.Gates)
	}
	if len(result.Board.Connections) != 3 {
		t.Fatalf("connections = %+v, want copied graph", result.Board.Connections)
	}
	for _, connection := range result.Board.Connections {
		if strings.Contains(connection.From, "template") || strings.Contains(connection.To, "template") || connection.ID == "edge_mission_designer" {
			t.Fatalf("connection was not remapped: %+v", connection)
		}
	}
	if result.Layout == nil || result.Layout.BoardID != result.Board.ID || result.Layout.BoardRev != 1 {
		t.Fatalf("layout = %+v, want remapped board identity", result.Layout)
	}
	if len(result.Layout.Nodes) != 3 || len(result.Layout.Edges) != 3 {
		t.Fatalf("layout shape = %+v, want three nodes and edges", result.Layout)
	}

	installedRole := filepath.Join(workspace, filepath.FromSlash(result.InstalledRoot), "roles", "designer.md")
	if got, err := os.ReadFile(installedRole); err != nil || !strings.Contains(string(got), "rendered evidence") {
		t.Fatalf("installed role = %q err=%v", string(got), err)
	}
	brief := result.Board.Formations[0].Brief
	if brief == nil || len(brief.Files) != 1 || brief.Files[0] != filepath.ToSlash(filepath.Join(result.InstalledRoot, "roles", "designer.md")) {
		t.Fatalf("brief files = %+v, want installed pack-relative role", brief)
	}

	sourceBoard, err := os.ReadFile(filepath.Join(packDir, "design.formation.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sourceBoard), `id = "brd_template"`) || !strings.Contains(string(sourceBoard), "{{packRoot}}") {
		t.Fatalf("source template was mutated:\n%s", sourceBoard)
	}

	report := ValidateBoard(result.Board)
	if len(report.Errors) != 0 {
		t.Fatalf("instantiated board validation errors = %+v", report.Errors)
	}
}

func TestInstantiateWorkflowPackReusesIdenticalInstalledPackAndRejectsDrift(t *testing.T) {
	workspace := t.TempDir()
	packDir := writeWorkflowPackFixture(t)
	store := NewStore(workspace)

	first, err := store.InstantiateWorkflowPack(packDir, WorkflowInstantiateRequest{Slug: "one", Title: "One", Goal: "One", UpdatedBy: "agent:test"})
	if err != nil {
		t.Fatalf("first instantiate: %v", err)
	}
	if _, err := store.InstantiateWorkflowPack(packDir, WorkflowInstantiateRequest{Slug: "two", Title: "Two", Goal: "Two", UpdatedBy: "agent:test"}); err != nil {
		t.Fatalf("reuse identical installed pack: %v", err)
	}

	installedRole := filepath.Join(workspace, filepath.FromSlash(first.InstalledRoot), "roles", "designer.md")
	if err := os.WriteFile(installedRole, []byte("drift"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = store.InstantiateWorkflowPack(packDir, WorkflowInstantiateRequest{Slug: "three", Title: "Three", Goal: "Three", UpdatedBy: "agent:test"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("drifted installed pack error = %v, want ErrConflict", err)
	}
	if _, statErr := os.Stat(store.BoardPath("three")); !os.IsNotExist(statErr) {
		t.Fatalf("drift failure created a board: %v", statErr)
	}
}

func TestInstallWorkflowPackRejectsSourceDriftDuringStaging(t *testing.T) {
	packDir := writeWorkflowPackFixture(t)
	pack, err := LoadWorkflowPack(packDir)
	if err != nil {
		t.Fatalf("load workflow pack: %v", err)
	}
	writeFixture(t, filepath.Join(packDir, "roles", "designer.md"), "changed after validation\n")
	destination := filepath.Join(t.TempDir(), "installed")
	if err := installWorkflowPack(pack, destination); !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "changed while staging") {
		t.Fatalf("install drift error = %v, want staging conflict", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("drifted destination exists or stat failed unexpectedly: %v", err)
	}
}

func TestInstantiateWorkflowPackRejectsSymlinkedWorkspaceDestinations(t *testing.T) {
	packDir := writeWorkflowPackFixture(t)
	for _, tc := range []struct {
		name    string
		linkRel string
		leakRel string
	}{
		{name: "pack directory", linkRel: filepath.Join(".formations", "packs", "test-design"), leakRel: "0.1.0"},
		{name: "board directory", linkRel: filepath.Join(".formations", "boards"), leakRel: "symlinked.formation.toml"},
		{name: "layout directory", linkRel: filepath.Join(".formations", "layout"), leakRel: "symlinked.layout.toml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			external := t.TempDir()
			link := filepath.Join(workspace, tc.linkRel)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, link); err != nil {
				t.Fatal(err)
			}
			_, err := NewStore(workspace).InstantiateWorkflowPack(packDir, WorkflowInstantiateRequest{
				Slug: "symlinked", Title: "Symlinked", Goal: "Stay inside workspace", UpdatedBy: "agent:test",
			})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink") {
				t.Fatalf("instantiate error = %v, want symlink rejection", err)
			}
			if _, statErr := os.Stat(filepath.Join(external, tc.leakRel)); !os.IsNotExist(statErr) {
				t.Fatalf("external write leaked through symlink: %v", statErr)
			}
		})
	}
}

func TestInstantiateWorkflowPackRetainsLayoutAfterBoardPublicationSyncFailure(t *testing.T) {
	workspace := t.TempDir()
	packDir := writeWorkflowPackFixture(t)
	boardDir := filepath.Join(workspace, ".formations", "boards")
	boom := errors.New("board directory sync failed")
	originalSyncDirectory := syncDirectory
	syncDirectory = func(dir string) error {
		if filepath.Clean(dir) == filepath.Clean(boardDir) {
			return boom
		}
		return fsyncDir(dir)
	}
	t.Cleanup(func() { syncDirectory = originalSyncDirectory })

	_, err := NewStore(workspace).InstantiateWorkflowPack(packDir, WorkflowInstantiateRequest{
		Slug: "sync-failure", Title: "Sync failure", Goal: "Keep publication consistent", UpdatedBy: "agent:test",
	})
	if !errors.Is(err, boom) {
		t.Fatalf("instantiate error = %v, want board sync failure", err)
	}
	for _, path := range []string{
		filepath.Join(boardDir, "sync-failure.formation.toml"),
		filepath.Join(workspace, ".formations", "layout", "sync-failure.layout.toml"),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("published instance component %q missing after post-link failure: %v", path, statErr)
		}
	}
}

func TestInstallWorkflowPackSyncsStagedTreeBeforePublication(t *testing.T) {
	packDir := writeWorkflowPackFixture(t)
	pack, err := LoadWorkflowPack(packDir)
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	destination := filepath.Join(parent, "installed")
	boom := errors.New("staged tree sync failed")
	originalSyncDirectory := syncDirectory
	syncDirectory = func(dir string) error {
		if strings.HasPrefix(filepath.Base(dir), ".pack-stage-") {
			return boom
		}
		return fsyncDir(dir)
	}
	t.Cleanup(func() { syncDirectory = originalSyncDirectory })

	if err := installWorkflowPack(pack, destination); !errors.Is(err, boom) {
		t.Fatalf("install error = %v, want staged sync failure", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("unsynced pack was published: %v", err)
	}
}

func TestInstallWorkflowPackSyncsParentAfterPublication(t *testing.T) {
	packDir := writeWorkflowPackFixture(t)
	pack, err := LoadWorkflowPack(packDir)
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	destination := filepath.Join(parent, "installed")
	boom := errors.New("pack parent sync failed")
	originalSyncDirectory := syncDirectory
	syncDirectory = func(dir string) error {
		if filepath.Clean(dir) == filepath.Clean(parent) {
			if _, err := os.Stat(destination); err != nil {
				t.Fatalf("pack parent synchronized before publication: %v", err)
			}
			return boom
		}
		return fsyncDir(dir)
	}
	t.Cleanup(func() { syncDirectory = originalSyncDirectory })

	if err := installWorkflowPack(pack, destination); !errors.Is(err, boom) {
		t.Fatalf("install error = %v, want parent sync failure", err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("published pack missing after post-rename sync failure: %v", err)
	}
}

func TestPublishWorkflowPackDirectoryNeverReplacesExistingDestination(t *testing.T) {
	parent := t.TempDir()
	source := filepath.Join(parent, "source")
	destination := filepath.Join(parent, "destination")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "owner")
	if err := os.WriteFile(marker, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishDirectoryNoReplace(source, destination); err == nil {
		t.Fatal("publish replaced existing destination")
	}
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "existing" {
		t.Fatalf("existing destination changed: raw=%q err=%v", raw, err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("failed publish removed source: %v", err)
	}
}

func TestRequireWorkflowPackDigestRejectsInstalledMutation(t *testing.T) {
	packDir := writeWorkflowPackFixture(t)
	pack, err := LoadWorkflowPack(packDir)
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(t.TempDir(), "installed")
	if err := copyWorkflowPackTree(pack.Root, installed); err != nil {
		t.Fatal(err)
	}
	if err := requireWorkflowPackDigest(installed, pack.Digest); err != nil {
		t.Fatalf("initial digest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installed, "roles", "designer.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := requireWorkflowPackDigest(installed, pack.Digest); err == nil || !strings.Contains(err.Error(), "changed during instantiation") {
		t.Fatalf("mutation error = %v, want provenance conflict", err)
	}
}

func TestLoadWorkflowPackRejectsEscapesAndMalformedTemplates(t *testing.T) {
	t.Run("manifest path escape", func(t *testing.T) {
		packDir := t.TempDir()
		writeFixture(t, filepath.Join(packDir, "pack.toml"), `schema = 1
id = "bad-pack"
version = "1.0.0"
title = "Bad"
board = "../outside.toml"
missionId = "mis_template"
license = "Apache-2.0"
`)
		if _, err := LoadWorkflowPack(packDir); err == nil || !strings.Contains(err.Error(), "outside pack") {
			t.Fatalf("escape error = %v, want outside pack", err)
		}
	})

	t.Run("missing license file", func(t *testing.T) {
		packDir := writeWorkflowPackFixture(t)
		if err := os.Remove(filepath.Join(packDir, "LICENSE")); err != nil {
			t.Fatalf("remove fixture license: %v", err)
		}
		if _, err := LoadWorkflowPack(packDir); err == nil || !strings.Contains(err.Error(), "license file") {
			t.Fatalf("missing license error = %v, want license file", err)
		}
	})

	t.Run("dangling board", func(t *testing.T) {
		packDir := writeWorkflowPackFixture(t)
		path := filepath.Join(packDir, "design.formation.toml")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.Replace(string(raw), `to = "fmn_designer:port_designer_in"`, `to = "missing:in"`, 1))
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadWorkflowPack(packDir); err == nil || !strings.Contains(err.Error(), FindingDanglingConnection) {
			t.Fatalf("dangling template error = %v, want %s", err, FindingDanglingConnection)
		}
	})
}

func writeWorkflowPackFixture(t *testing.T) string {
	t.Helper()
	packDir := t.TempDir()
	writeFixture(t, filepath.Join(packDir, "pack.toml"), `schema = 1
id = "test-design"
version = "0.1.0"
title = "Test design workflow"
board = "design.formation.toml"
layout = "design.layout.toml"
missionId = "mis_template"
license = "Apache-2.0"
`)
	writeFixture(t, filepath.Join(packDir, "LICENSE"), "Apache License 2.0 fixture\n")
	writeFixture(t, filepath.Join(packDir, "roles", "designer.md"), "Create the artifact and cite rendered evidence.\n")
	writeFixture(t, filepath.Join(packDir, "design.formation.toml"), `schema = 1
id = "brd_template"
slug = "design-template"
title = "Design template"
rev = 1
updatedBy = "pack:test"
updatedAt = "2026-07-15T00:00:00Z"

[[mission]]
id = "mis_template"
title = "Design artifact"
goal = "Replaced when instantiated"

[[formation]]
id = "fmn_designer"
type = "solo"
title = "Designer mentions fmn_designer literally"

[formation.brief]
goal = "Create a real artifact and return its reference."
files = ["{{packRoot}}/roles/designer.md"]

[[formation.input]]
id = "port_designer_in"
label = "Brief"

[[formation.output]]
id = "port_designer_out"
label = "Artifact"

[[formation.slot]]
id = "slot_designer"
label = "Designer"
controller = false

[[gate]]
id = "gate_quality"
title = "Quality score"
kinds = ["scorecard"]
criterion = "Weighted score meets the configured threshold with no must-fix findings."
scoreThreshold = 8.0
requireNoMustFix = true
requiredReviewers = ["critic"]
reviewerWeights = ["critic=1.0"]

[[connection]]
id = "edge_mission_designer"
from = "mis_template:out"
to = "fmn_designer:port_designer_in"

[[connection]]
id = "edge_designer_quality"
from = "fmn_designer:port_designer_out"
to = "gate_quality:in"

[[connection]]
id = "edge_quality_designer"
from = "gate_quality:pass"
to = "fmn_designer:port_designer_in"
`)
	writeFixture(t, filepath.Join(packDir, "design.layout.toml"), `schema = 1
boardId = "brd_template"
boardRev = 1
updatedAt = "2026-07-15T00:00:00Z"

[[node]]
id = "mis_template"
x = 40
y = 120

[[node]]
id = "fmn_designer"
x = 300
y = 120

[[node]]
id = "gate_quality"
x = 560
y = 120

[[edge]]
id = "edge_mission_designer"
lane = "main"

[[edge]]
id = "edge_designer_quality"
lane = "main"

[[edge]]
id = "edge_quality_designer"
lane = "pass"
`)
	return packDir
}
