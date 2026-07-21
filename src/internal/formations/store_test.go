package formations

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestBoardWriterPreservesUnknownFieldsAndCommentsWithMinimalDiff(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `# human note stays put
schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search" # inline title note
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "do not touch"

[[formation]]
id = "fmn_01J9_research"
reviewerNotes = "preserve me"
`)

	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	after, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("Improve session search quickly"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("update board: %v", err)
	}

	if after.Rev != 8 {
		t.Fatalf("rev = %d, want 8", after.Rev)
	}
	got := readFile(t, store.BoardPath("session-search"))
	for _, want := range []string{
		"# human note stays put",
		`title = "Improve session search quickly" # inline title note`,
		`customFuture = "do not touch"`,
		`reviewerNotes = "preserve me"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated TOML lost %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "2026-06-03T16:00:00Z") {
		t.Fatalf("updatedAt was not refreshed:\n%s", got)
	}
	if !strings.Contains(got, `updatedAt = "2026-06-03T17:00:00Z"`) {
		t.Fatalf("updatedAt did not use writer clock:\n%s", got)
	}
	if after.ETag == before.ETag {
		t.Fatal("ETag did not change after structural write")
	}
}

func TestBoardWriterUsesDecodedIdentityForQuotedRootKeys(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"),
		"schema = 1\n"+
			"id = \"brd_01J9_sesssearch\"\n"+
			"slug = \"session-search\"\n"+
			"title = \"Improve session search\"\n"+
			"rev = 7\n"+
			"updatedBy = \"agent:archon\"\n"+
			"\"updatedAt\" = \"2026-06-03T16:00:00Z\"\n",
	)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	after, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("Improve session search quickly"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("update board: %v", err)
	}
	if after.UpdatedAt != "2026-06-03T17:00:00Z" {
		t.Fatalf("updatedAt = %q, want fixed update time through quoted key identity", after.UpdatedAt)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Count(raw, "updatedAt") != 1 {
		t.Fatalf("writer created duplicate semantic updatedAt keys:\n%s", raw)
	}
	if !strings.Contains(raw, "\"updatedAt\" = \"2026-06-03T17:00:00Z\"") {
		t.Fatalf("writer did not update the existing quoted root key:\n%s", raw)
	}
}

func TestBoardStructuralWriteRequiresMatchingETagAndBumpsRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if _, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("blind write"),
		UpdatedBy: "agent:test",
	}, WriteOptions{}); !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("missing precondition error = %v, want ErrPreconditionRequired", err)
	}
	if _, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("stale write"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: "stale", ExpectedRev: board.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale ETag error = %v, want ErrConflict", err)
	}

	after, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("fresh write"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		t.Fatalf("fresh update: %v", err)
	}
	if after.Rev != 8 {
		t.Fatalf("rev = %d, want 8", after.Rev)
	}

	if _, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("old rev"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: after.ETag, ExpectedRev: board.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale rev error = %v, want ErrConflict", err)
	}
}

func TestLayoutWriteDoesNotChangeBoardBytesOrBoardRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "fmn_01J9_research"
x = 420
y = 160
`)

	boardBefore := readFile(t, store.BoardPath("session-search"))
	layoutBefore, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if _, err := store.UpdateLayoutMetadata("session-search", LayoutMetadataPatch{}, WriteOptions{}); !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("missing layout precondition error = %v, want ErrPreconditionRequired", err)
	}

	layoutAfter, err := store.UpdateLayoutMetadata("session-search", LayoutMetadataPatch{
		UpdatedAt: time.Date(2026, 6, 3, 17, 5, 0, 0, time.UTC),
	}, WriteOptions{ExpectedETag: layoutBefore.ETag})
	if err != nil {
		t.Fatalf("update layout: %v", err)
	}

	if got := readFile(t, store.BoardPath("session-search")); got != boardBefore {
		t.Fatalf("layout write changed board definition:\n%s", got)
	}
	boardAfter, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after layout write: %v", err)
	}
	if boardAfter.Rev != 7 {
		t.Fatalf("board rev after layout write = %d, want 7", boardAfter.Rev)
	}
	if layoutAfter.ETag == layoutBefore.ETag {
		t.Fatal("layout ETag did not change after layout write")
	}
}

func TestConcurrentBoardWritesReturnConflictNotClobber(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, title := range []string{"first", "second"} {
		title := title
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
				Title:     &title,
				UpdatedBy: "agent:test",
			}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent write error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one of each", successes, conflicts)
	}

	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after concurrent writes: %v", err)
	}
	if after.Rev != 8 {
		t.Fatalf("rev after concurrent writes = %d, want exactly 8", after.Rev)
	}
}

func TestCreateBoardWritesMinimalDurableBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()

	board, err := store.CreateBoard(BoardCreateRequest{
		Slug:      "poems",
		Title:     "Poems",
		UpdatedBy: "agent:test",
	})
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	if board.Schema != NewBoardSchema || board.Slug != "poems" || board.Title != "Poems" || board.Rev != 1 || !strings.HasPrefix(board.ID, "brd_") || board.ETag == "" {
		t.Fatalf("created board = %+v, want durable board identity", board)
	}
	if board.UpdatedBy != "agent:test" || board.UpdatedAt != "2026-06-03T17:00:00Z" {
		t.Fatalf("created board update metadata = %q/%q", board.UpdatedBy, board.UpdatedAt)
	}
	raw := readFile(t, store.BoardPath("poems"))
	for _, want := range []string{`schema = 1`, `id = "brd_`, `slug = "poems"`, `title = "Poems"`, `rev = 1`, `updatedBy = "agent:test"`, `updatedAt = "2026-06-03T17:00:00Z"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("created board file missing %q:\n%s", want, raw)
		}
	}
	if strings.Contains(raw, "[[node]]") || strings.Contains(raw, "\nx = ") || strings.Contains(raw, "\ny = ") {
		t.Fatalf("created board leaked layout sidecar data:\n%s", raw)
	}
}

func TestCreateBoardRefusesDuplicateSlugWithoutClobber(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("poems"), minimalBoard("poems", 7))
	before := readFile(t, store.BoardPath("poems"))

	_, err := store.CreateBoard(BoardCreateRequest{Slug: "poems", Title: "Different"})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate board create error = %v, want ErrAlreadyExists", err)
	}
	after := readFile(t, store.BoardPath("poems"))
	if after != before {
		t.Fatalf("duplicate board create changed board:\n%s", after)
	}
}

func TestCreateBoardValidatesSlugAndTitle(t *testing.T) {
	store := NewStore(t.TempDir())

	if _, err := store.CreateBoard(BoardCreateRequest{Slug: "bad/path", Title: "Bad"}); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("invalid slug error = %v, want ErrInvalidSlug", err)
	}
	if _, err := store.CreateBoard(BoardCreateRequest{Slug: "poems"}); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("missing title error = %v, want ErrInvalidSlug", err)
	}
}

func TestDefinitionSchemaLifecyclesAreIndependent(t *testing.T) {
	if CurrentBoardSchema != 2 {
		t.Fatalf("CurrentBoardSchema = %d, want 2 for Tool-capable boards", CurrentBoardSchema)
	}
	if CurrentLayoutSchema != 1 {
		t.Fatalf("CurrentLayoutSchema = %d, want 1 because Tool support does not migrate layout", CurrentLayoutSchema)
	}
	if NewBoardSchema != 1 {
		t.Fatalf("NewBoardSchema = %d, want 1 so Tool-free boards stay on the compatibility schema", NewBoardSchema)
	}
}

func TestToolFreeSchemaTwoBoardRemainsValidAndReadOnly(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := `schema = 2
id = "brd_tool_deleted"
slug = "tool-deleted"
title = "Tool deleted"
rev = 4
updatedBy = "agent:test"
updatedAt = "2026-07-19T08:00:00Z"
`
	path := store.BoardPath("tool-deleted")
	writeFixture(t, path, raw)
	wantETag := etag([]byte(raw))
	wantIdentity := operativeFileIdentityForTest(t, path)

	board, err := store.ReadBoard("tool-deleted")
	if err != nil {
		t.Fatalf("read Tool-free schema-2 board: %v", err)
	}
	if board.Schema != CurrentBoardSchema {
		t.Errorf("Tool-free board schema = %d, want monotonic schema %d", board.Schema, CurrentBoardSchema)
	}
	if board.ETag != wantETag || board.TOML != raw {
		t.Errorf("schema-2 projection changed source identity: ETag=%q want %q TOML=%q want %q", board.ETag, wantETag, board.TOML, raw)
	}
	if got := readFile(t, path); got != raw {
		t.Errorf("schema-2 inspection changed canonical bytes:\n got %q\nwant %q", got, raw)
	}
	if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
		t.Errorf("schema-2 inspection replaced operative file identity = %v, want %v", got, wantIdentity)
	}
}

func TestSchemaZeroBoardInspectionNeverPublishesMigration(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := `schema = 0
id = "brd_schema_zero"
slug = "schema-zero"
title = "Schema zero"
rev = 1
legacyKey = "keep"
`
	path := store.BoardPath("schema-zero")
	writeFixture(t, path, raw)
	wantETag := etag([]byte(raw))
	wantIdentity := operativeFileIdentityForTest(t, path)

	board, err := store.ReadBoard("schema-zero")
	if err != nil {
		t.Fatalf("inspect accepted schema-0 board: %v", err)
	}
	if board.Schema != 0 {
		t.Errorf("schema-0 board inspection returned schema = %d, want source schema 0", board.Schema)
	}
	if board.ETag != wantETag || board.TOML != raw {
		t.Errorf("schema-0 board projection changed source identity: ETag=%q want %q TOML=%q want %q", board.ETag, wantETag, board.TOML, raw)
	}
	if got := readFile(t, path); got != raw {
		t.Errorf("schema-0 board inspection published a migration:\n got %q\nwant %q", got, raw)
	}
	if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
		t.Errorf("schema-0 board inspection replaced operative file identity = %v, want %v", got, wantIdentity)
	}
}

func TestSchemaZeroLayoutInspectionNeverPublishesMigration(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := `schema = 0
boardId = "brd_schema_zero"
boardRev = 1
updatedAt = "2026-06-03T16:02:00Z"
layoutNote = "keep"
`
	path := store.LayoutPath("schema-zero")
	writeFixture(t, path, raw)
	wantETag := etag([]byte(raw))
	wantIdentity := operativeFileIdentityForTest(t, path)

	layout, err := store.ReadLayout("schema-zero")
	if err != nil {
		t.Fatalf("inspect accepted schema-0 layout: %v", err)
	}
	if layout.Schema != 0 {
		t.Errorf("schema-0 layout inspection returned schema = %d, want source schema 0", layout.Schema)
	}
	if layout.ETag != wantETag || layout.TOML != raw {
		t.Errorf("schema-0 layout projection changed source identity: ETag=%q want %q TOML=%q want %q", layout.ETag, wantETag, layout.TOML, raw)
	}
	if got := readFile(t, path); got != raw {
		t.Errorf("schema-0 layout inspection published a migration:\n got %q\nwant %q", got, raw)
	}
	if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
		t.Errorf("schema-0 layout inspection replaced operative file identity = %v, want %v", got, wantIdentity)
	}
}

func TestSchemaOneBoardInspectionNeverPublishesMigration(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := minimalBoard("inspect-only", 1)
	path := store.BoardPath("inspect-only")
	writeFixture(t, path, raw)
	wantETag := etag([]byte(raw))
	wantIdentity := operativeFileIdentityForTest(t, path)

	board, err := store.ReadBoard("inspect-only")
	if err != nil {
		t.Fatalf("read schema-1 board: %v", err)
	}
	if board.Schema != NewBoardSchema {
		t.Fatalf("read board schema = %d, want stored schema %d", board.Schema, NewBoardSchema)
	}
	if board.ETag != wantETag || board.TOML != raw {
		t.Fatalf("inspection projection changed source identity: ETag=%q want %q TOML=%q want %q", board.ETag, wantETag, board.TOML, raw)
	}

	_ = ValidateBoard(board)
	if board.ETag != wantETag {
		t.Fatalf("ValidateBoard changed board ETag = %q, want %q", board.ETag, wantETag)
	}
	if got := readFile(t, path); got != raw {
		t.Fatalf("ReadBoard/ValidateBoard published an implicit migration:\n got %q\nwant %q", got, raw)
	}
	if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
		t.Fatalf("ReadBoard/ValidateBoard replaced operative file identity = %v, want %v", got, wantIdentity)
	}
}

func TestSchemaOneLayoutInspectionNeverPublishesMigration(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := `schema = 1
boardId = "brd_inspect_only"
boardRev = 1
updatedAt = "2026-06-03T16:02:00Z"
layoutNote = "keep"
`
	path := store.LayoutPath("inspect-only")
	writeFixture(t, path, raw)
	wantETag := etag([]byte(raw))
	wantIdentity := operativeFileIdentityForTest(t, path)

	layout, err := store.ReadLayout("inspect-only")
	if err != nil {
		t.Fatalf("read schema-1 layout: %v", err)
	}
	if layout.Schema != CurrentLayoutSchema {
		t.Fatalf("read layout schema = %d, want %d", layout.Schema, CurrentLayoutSchema)
	}
	if layout.ETag != wantETag || layout.TOML != raw {
		t.Fatalf("layout inspection changed source identity: ETag=%q want %q TOML=%q want %q", layout.ETag, wantETag, layout.TOML, raw)
	}
	if got := readFile(t, path); got != raw {
		t.Fatalf("ReadLayout published an implicit migration:\n got %q\nwant %q", got, raw)
	}
	if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
		t.Fatalf("ReadLayout replaced operative file identity = %v, want %v", got, wantIdentity)
	}
}

func TestLayoutCoordinateProjectionUsesHostIntegerBoundsNotToolParameterBounds(t *testing.T) {
	wide := maxToolParameterInteger + 1
	projected, ok := parseLayoutCoordinate(strconv.FormatInt(wide, 10))
	if strconv.IntSize == 64 {
		if !ok || int64(projected) != wide {
			t.Fatalf("wide host-int layout coordinate projection = %d, %v; want %d, true", projected, ok, wide)
		}
		return
	}
	if ok {
		t.Fatalf("out-of-range host-int layout coordinate unexpectedly projected as %d", projected)
	}
}

func TestDefinitionReadersRejectOnlyVersionsNewerThanTheirOwnSchema(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("future-board"), `schema = 3
id = "brd_future"
slug = "future-board"
title = "Future board"
rev = 1
`)
	if _, err := store.ReadBoard("future-board"); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("schema-3 board error = %v, want ErrUnsupportedSchema above CurrentBoardSchema", err)
	}

	writeFixture(t, store.LayoutPath("future-layout"), `schema = 2
boardId = "brd_future"
boardRev = 1
updatedAt = "2026-06-03T16:02:00Z"
`)
	if _, err := store.ReadLayout("future-layout"); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("schema-2 layout error = %v, want ErrUnsupportedSchema above CurrentLayoutSchema", err)
	}
}

func TestOrdinaryBoardMetadataWritePreservesSchemaOne(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("metadata-only"), minimalBoard("metadata-only", 1))

	before, err := store.ReadBoard("metadata-only")
	if err != nil {
		t.Fatalf("read schema-1 board: %v", err)
	}
	after, err := store.UpdateBoardMetadata("metadata-only", BoardMetadataPatch{
		Title:     stringPtr("Metadata changed"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("update schema-1 board metadata: %v", err)
	}
	if after.Schema != NewBoardSchema {
		t.Fatalf("ordinary metadata write changed schema = %d, want preserved schema %d", after.Schema, NewBoardSchema)
	}
	if got := readFile(t, store.BoardPath("metadata-only")); !strings.Contains(got, "schema = 1\n") || strings.Contains(got, "schema = 2\n") {
		t.Fatalf("ordinary metadata write migrated Tool-free board:\n%s", got)
	}
}

func TestOrdinaryNonToolWritersPreserveSchemaOne(t *testing.T) {
	const slug = "schema-one-writers"
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath(slug), minimalBoard(slug, 1))

	assertSchemaOne := func(step string, board *BoardDocument) {
		t.Helper()
		if board.Schema != 1 {
			t.Fatalf("%s returned schema = %d, want preserved schema 1", step, board.Schema)
		}
		persisted, err := parseBoard([]byte(readFile(t, store.BoardPath(slug))))
		if err != nil {
			t.Fatalf("%s parse persisted board: %v", step, err)
		}
		if persisted.Schema != 1 {
			t.Fatalf("%s persisted schema = %d, want preserved schema 1", step, persisted.Schema)
		}
	}

	current, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	formation, err := store.CreateFormation(slug, FormationCreateRequest{
		Type: FormationTypeSolo, X: 100, Y: 100, UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: current.ETag, ExpectedRev: current.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}
	current = formation.Board
	assertSchemaOne("create formation", current)

	gate, err := store.CreateGate(slug, GateCreateRequest{
		Kinds: []string{"human"}, X: 300, Y: 100, UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: current.ETag, ExpectedRev: current.Rev})
	if err != nil {
		t.Fatalf("create gate: %v", err)
	}
	current = gate.Board
	assertSchemaOne("create gate", current)

	mission, err := store.CreateMission(slug, MissionCreateRequest{
		Goal: "Exercise schema preservation", BeadID: "home-7kc4.5", X: 500, Y: 100, UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: current.ETag, ExpectedRev: current.Rev})
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}
	current = mission.Board
	assertSchemaOne("create mission", current)

	current, err = store.WireFormationPorts(slug, FormationWireRequest{
		From: formation.Formation.ID + ":" + formation.Formation.Outputs[0].ID,
		To:   gate.Gate.ID + ":in", UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: current.ETag, ExpectedRev: current.Rev})
	if err != nil {
		t.Fatalf("wire formation to gate: %v", err)
	}
	assertSchemaOne("shared wire update", current)

	deletedMission, err := store.DeleteMission(slug, MissionDeleteRequest{
		ID: mission.Mission.ID, UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: current.ETag, ExpectedRev: current.Rev})
	if err != nil {
		t.Fatalf("delete mission: %v", err)
	}
	current = deletedMission.Board
	assertSchemaOne("delete mission", current)

	deletedGate, err := store.DeleteGate(slug, GateDeleteRequest{
		ID: gate.Gate.ID, UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: current.ETag, ExpectedRev: current.Rev})
	if err != nil {
		t.Fatalf("delete gate: %v", err)
	}
	current = deletedGate.Board
	assertSchemaOne("delete gate", current)

	deletedFormation, err := store.DeleteFormation(slug, FormationDeleteRequest{
		ID: formation.Formation.ID, UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: current.ETag, ExpectedRev: current.Rev})
	if err != nil {
		t.Fatalf("delete formation: %v", err)
	}
	assertSchemaOne("delete formation", deletedFormation.Board)
}

func TestBoardChangeSignalDetectsExternalEdit(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	writeFixture(t, store.BoardPath("session-search"), strings.Replace(minimalBoard("session-search", 7), "Improve session search", "Externally edited", 1))

	signal, err := store.BoardChangeSince("session-search", board.ETag)
	if err != nil {
		t.Fatalf("detect board change: %v", err)
	}
	if !signal.Changed {
		t.Fatalf("Changed = false, want true")
	}
	if signal.Signal != "board.changed" {
		t.Fatalf("Signal = %q, want board.changed", signal.Signal)
	}
	if signal.ETag == board.ETag {
		t.Fatal("change signal ETag did not change")
	}
	if signal.ModifiedAt == "" {
		t.Fatal("change signal missing ModifiedAt")
	}
}

func TestCreateFormationPersistsDefaultSlotsAndLayoutOnlyCoordinates(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `# board comments stay
schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"
`)

	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	result, err := store.CreateFormation("session-search", FormationCreateRequest{
		Type:      FormationTypePeer,
		Title:     "Research huddle",
		X:         840,
		Y:         135,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}

	if result.Board.Rev != 8 {
		t.Fatalf("board rev = %d, want 8", result.Board.Rev)
	}
	if result.Formation.ID == "" || !strings.HasPrefix(result.Formation.ID, "fmn_") {
		t.Fatalf("formation id = %q, want fmn_ prefix", result.Formation.ID)
	}
	if result.Formation.Type != FormationTypePeer || result.Formation.Title != "Research huddle" {
		t.Fatalf("formation = %+v, want peer Research huddle", result.Formation)
	}
	if len(result.Formation.Inputs) != 1 || !strings.HasPrefix(result.Formation.Inputs[0].ID, "port_") {
		t.Fatalf("inputs = %+v, want one stable port", result.Formation.Inputs)
	}
	if len(result.Formation.Outputs) != 1 || !strings.HasPrefix(result.Formation.Outputs[0].ID, "port_") {
		t.Fatalf("outputs = %+v, want one stable port", result.Formation.Outputs)
	}
	if len(result.Formation.Slots) != 2 {
		t.Fatalf("slots len = %d, want two peer slots", len(result.Formation.Slots))
	}
	for _, slot := range result.Formation.Slots {
		if !strings.HasPrefix(slot.ID, "slot_") {
			t.Fatalf("slot id = %q, want slot_ prefix", slot.ID)
		}
		if slot.AgentID != "" || slot.Harness != "" {
			t.Fatalf("S2 create stored staffing fields too early: %+v", slot)
		}
		if slot.Controller {
			t.Fatalf("peer slot unexpectedly controller: %+v", slot)
		}
	}

	boardTOML := readFile(t, store.BoardPath("session-search"))
	for _, want := range []string{
		"# board comments stay",
		`customFuture = "keep me"`,
		`[[formation]]`,
		`type = "peer"`,
		`title = "Research huddle"`,
		`label = "Peer"`,
	} {
		if !strings.Contains(boardTOML, want) {
			t.Fatalf("board TOML missing %q:\n%s", want, boardTOML)
		}
	}
	if strings.Contains(boardTOML, "x = 840") || strings.Contains(boardTOML, "y = 135") {
		t.Fatalf("board definition contains layout coordinates:\n%s", boardTOML)
	}

	layoutTOML := readFile(t, store.LayoutPath("session-search"))
	for _, want := range []string{
		`boardId = "brd_01J9_sesssearch"`,
		`boardRev = 8`,
		`[[node]]`,
		`x = 840`,
		`y = 135`,
	} {
		if !strings.Contains(layoutTOML, want) {
			t.Fatalf("layout TOML missing %q:\n%s", want, layoutTOML)
		}
	}
	if result.Layout.BoardRev != 8 {
		t.Fatalf("layout boardRev = %d, want 8", result.Layout.BoardRev)
	}
	if len(result.Layout.Nodes) != 1 || result.Layout.Nodes[0].ID != result.Formation.ID {
		t.Fatalf("layout nodes = %+v, want created formation node", result.Layout.Nodes)
	}

	reread, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("reread board: %v", err)
	}
	if len(reread.Formations) != 1 || reread.Formations[0].ID != result.Formation.ID {
		t.Fatalf("reread formations = %+v, want created formation", reread.Formations)
	}
}

func TestCreateFormationProvidesAllFourDefaultShapes(t *testing.T) {
	tests := []struct {
		formationType string
		wantSlots     int
		wantControl   int
	}{
		{FormationTypeSolo, 1, 0},
		{FormationTypePeer, 2, 0},
		{FormationTypeFlow, 3, 0},
		{FormationTypeOrchestrated, 3, 1},
	}

	for _, tt := range tests {
		t.Run(tt.formationType, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
			before, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read board: %v", err)
			}

			result, err := store.CreateFormation("session-search", FormationCreateRequest{
				Type:      tt.formationType,
				Title:     "New " + tt.formationType,
				X:         100,
				Y:         200,
				UpdatedBy: "agent:test",
			}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
			if err != nil {
				t.Fatalf("create formation: %v", err)
			}
			if len(result.Formation.Inputs) != 1 || len(result.Formation.Outputs) != 1 {
				t.Fatalf("ports = in:%+v out:%+v, want one input and one output", result.Formation.Inputs, result.Formation.Outputs)
			}
			if len(result.Formation.Slots) != tt.wantSlots {
				t.Fatalf("slots len = %d, want %d", len(result.Formation.Slots), tt.wantSlots)
			}
			var controllers int
			for _, slot := range result.Formation.Slots {
				if slot.Controller {
					controllers++
				}
			}
			if controllers != tt.wantControl {
				t.Fatalf("controllers = %d, want %d in %+v", controllers, tt.wantControl, result.Formation.Slots)
			}
		})
	}
}

func TestBoardReadsPersistedConnectionsWithStableEndpoints(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.output]]
id = "port_frame_out"
label = "Output"

[[formation]]
id = "fmn_research"
type = "peer"
title = "Research"

[[formation.input]]
id = "port_research_in"
label = "Input"

[[connection]]
id = "edge_frame_research"
from = "fmn_frame:port_frame_out"
to = "fmn_research:port_research_in"
`)

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if len(board.Connections) != 1 {
		t.Fatalf("connections len = %d, want 1", len(board.Connections))
	}
	connection := board.Connections[0]
	if connection.ID != "edge_frame_research" || connection.From != "fmn_frame:port_frame_out" || connection.To != "fmn_research:port_research_in" {
		t.Fatalf("connection = %+v, want stable edge endpoints from board TOML", connection)
	}
}

func TestUpdateLayoutNodesRecreatesDeletedLayoutSidecarWithoutDirtyingBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_research"
type = "peer"
title = "Research"
`)
	boardBefore := readFile(t, store.BoardPath("session-search"))

	layout, err := store.UpdateLayoutNodes("session-search", []LayoutNode{{ID: "fmn_research", X: 320, Y: 240}}, WriteOptions{ExpectedETag: "*"})
	if err != nil {
		t.Fatalf("recreate layout sidecar: %v", err)
	}
	if layout.BoardID != "brd_01J9_sesssearch" || layout.BoardRev != 7 {
		t.Fatalf("layout board ref = %s rev %d, want board id/rev from definition", layout.BoardID, layout.BoardRev)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0].X != 320 || layout.Nodes[0].Y != 240 {
		t.Fatalf("layout nodes = %+v, want recreated node position", layout.Nodes)
	}
	if got := readFile(t, store.BoardPath("session-search")); got != boardBefore {
		t.Fatalf("layout recreation dirtied board definition:\n%s", got)
	}
	if _, err := os.Stat(store.LayoutPath("session-search")); err != nil {
		t.Fatalf("layout sidecar was not recreated: %v", err)
	}
}

func TestArrangeLayoutUsesGraphDepthWithoutChangingBoardDefinition(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("arrange"), `schema = 1
id = "brd_arrange"
slug = "arrange"
title = "Arrange"
rev = 4

[[mission]]
id = "mis_start"
title = "Start"

[[formation]]
id = "fmn_build"
type = "solo"
title = "Build"

[[gate]]
id = "gate_check"
title = "Check"
kinds = ["human"]

[[connection]]
id = "edge_start_build"
from = "mis_start:out"
to = "fmn_build:in"

[[connection]]
id = "edge_build_check"
from = "fmn_build:out"
to = "gate_check:in"
`)
	writeFixture(t, store.LayoutPath("arrange"), `schema = 1
boardId = "brd_arrange"
boardRev = 4

[[node]]
id = "mis_start"
x = 500
y = 500

[[node]]
id = "fmn_build"
x = 100
y = 100

[[node]]
id = "gate_check"
x = 300
y = 300
`)
	boardBefore := readFile(t, store.BoardPath("arrange"))
	layoutBefore, err := store.ReadLayout("arrange")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}

	arranged, err := store.ArrangeLayout("arrange", WriteOptions{ExpectedETag: layoutBefore.ETag})
	if err != nil {
		t.Fatalf("arrange layout: %v", err)
	}
	byID := map[string]LayoutNode{}
	for _, node := range arranged.Nodes {
		byID[node.ID] = node
		if node.X%28 != 0 || node.Y%28 != 0 {
			t.Fatalf("node is not grid aligned: %+v", node)
		}
	}
	if !(byID["mis_start"].X < byID["fmn_build"].X && byID["fmn_build"].X < byID["gate_check"].X) {
		t.Fatalf("arranged graph depth is not left-to-right: %+v", byID)
	}
	if got := readFile(t, store.BoardPath("arrange")); got != boardBefore {
		t.Fatalf("arrange changed board definition:\n%s", got)
	}

	again, err := store.ArrangeLayout("arrange", WriteOptions{ExpectedETag: arranged.ETag})
	if err != nil {
		t.Fatalf("repeat arrange: %v", err)
	}
	if len(again.Nodes) != len(arranged.Nodes) {
		t.Fatalf("repeat arrange node count = %d, want %d", len(again.Nodes), len(arranged.Nodes))
	}
	for index := range arranged.Nodes {
		if again.Nodes[index] != arranged.Nodes[index] {
			t.Fatalf("repeat arrange changed node %d: first=%+v second=%+v", index, arranged.Nodes[index], again.Nodes[index])
		}
	}
}

func TestArrangeLayoutIncludesToolsInExplicitGraphProjection(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	boardRaw := `schema = 2
id = "brd_arrange_tool"
slug = "arrange-tool"
title = "Arrange Tool"
rev = 4

[[mission]]
id = "mis_start"
title = "Start"

` + toolStructuralJSONNormalizeToolBlock("tool_normalize", "Normalize report", "port_tool_in", "port_tool_out") + `
[[formation]]
id = "fmn_finish"
type = "solo"
title = "Finish"

[[formation.input]]
id = "in"
label = "Input"

[[formation.output]]
id = "out"
label = "Output"

[[connection]]
id = "edge_start_tool"
from = "mis_start:out"
to = "tool_normalize:port_tool_in"

[[connection]]
id = "edge_tool_finish"
from = "tool_normalize:port_tool_out"
to = "fmn_finish:in"
`
	layoutRaw := `schema = 1
boardId = "brd_arrange_tool"
boardRev = 4

[[node]]
id = "mis_start"
x = 700
y = 500

[[node]]
id = "fmn_finish"
x = 300
y = 100
`
	writeFixture(t, store.BoardPath("arrange-tool"), boardRaw)
	writeFixture(t, store.LayoutPath("arrange-tool"), layoutRaw)
	layoutBefore, err := store.ReadLayout("arrange-tool")
	if err != nil {
		t.Fatalf("read Tool layout: %v", err)
	}

	// Arrangement is a read-only legibility projection of authored graph shape;
	// execution compatibility is validated by a different boundary.
	arranged, err := store.ArrangeLayout("arrange-tool", WriteOptions{ExpectedETag: layoutBefore.ETag})
	if err != nil {
		t.Fatalf("arrange Tool layout: %v", err)
	}
	byID := make(map[string]LayoutNode, len(arranged.Nodes))
	for _, node := range arranged.Nodes {
		byID[node.ID] = node
		if node.X%formationLayoutGrid != 0 || node.Y%formationLayoutGrid != 0 {
			t.Fatalf("arranged node is not grid aligned: %+v", node)
		}
	}
	mission, missionOK := byID["mis_start"]
	tool, toolOK := byID["tool_normalize"]
	formation, formationOK := byID["fmn_finish"]
	if !missionOK || !toolOK || !formationOK {
		t.Fatalf("arranged board inventory = %+v, want Mission, Tool, and Formation", byID)
	}
	if !(mission.X < tool.X && tool.X < formation.X) {
		t.Fatalf("arranged Tool graph depth is not left-to-right: %+v", byID)
	}
	if got := readFile(t, store.BoardPath("arrange-tool")); got != boardRaw {
		t.Fatalf("Tool arrangement changed board bytes:\n%s", got)
	}

	again, err := store.ArrangeLayout("arrange-tool", WriteOptions{ExpectedETag: arranged.ETag})
	if err != nil {
		t.Fatalf("repeat Tool arrangement: %v", err)
	}
	if len(again.Nodes) != len(arranged.Nodes) {
		t.Fatalf("repeat Tool arrangement node count = %d, want %d", len(again.Nodes), len(arranged.Nodes))
	}
	for index := range arranged.Nodes {
		if again.Nodes[index] != arranged.Nodes[index] {
			t.Fatalf("repeat Tool arrangement changed node %d: first=%+v second=%+v", index, arranged.Nodes[index], again.Nodes[index])
		}
	}
	if got := readFile(t, store.BoardPath("arrange-tool")); got != boardRaw {
		t.Fatalf("repeat Tool arrangement changed board bytes:\n%s", got)
	}
}

func TestArrangeLayoutRefreshesStaleBoardRevisionAfterDefinitionEdit(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("arrange-stale"), `schema = 1
id = "brd_arrange_stale"
slug = "arrange-stale"
title = "Arrange stale"
rev = 4

[[gate]]
id = "gate_check"
title = "Check"
kinds = ["human"]
`)
	writeFixture(t, store.LayoutPath("arrange-stale"), `schema = 1
boardId = "brd_arrange_stale"
boardRev = 4

[[node]]
id = "gate_check"
x = 300
y = 300
`)

	boardBefore, err := store.ReadBoard("arrange-stale")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	boardAfter, err := store.UpdateGate("arrange-stale", GateUpdateRequest{
		GateID: "gate_check",
		Title:  "Check updated",
	}, WriteOptions{ExpectedETag: boardBefore.ETag, ExpectedRev: boardBefore.Rev})
	if err != nil {
		t.Fatalf("update gate definition: %v", err)
	}
	staleLayout, err := store.ReadLayout("arrange-stale")
	if err != nil {
		t.Fatalf("read stale layout: %v", err)
	}
	if staleLayout.BoardRev == boardAfter.Rev {
		t.Fatalf("definition edit unexpectedly refreshed layout boardRev = %d", staleLayout.BoardRev)
	}

	arranged, err := store.ArrangeLayout("arrange-stale", WriteOptions{ExpectedETag: staleLayout.ETag})
	if err != nil {
		t.Fatalf("arrange stale layout: %v", err)
	}
	if arranged.BoardID != boardAfter.ID || arranged.BoardRev != boardAfter.Rev {
		t.Fatalf("arranged layout board ref = %s rev %d, want %s rev %d", arranged.BoardID, arranged.BoardRev, boardAfter.ID, boardAfter.Rev)
	}
}

func TestArrangeLayoutSerializesBoardEditsThroughLayoutPersistence(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("arrange-serial"), `schema = 1
id = "brd_arrange_serial"
slug = "arrange-serial"
title = "Arrange serial"
rev = 3

[[formation]]
id = "fmn_first"
type = "solo"
title = "First"

[[formation.input]]
id = "in"
label = "Input"

[[formation.output]]
id = "out"
label = "Output"

[[formation]]
id = "fmn_second"
type = "solo"
title = "Second"

[[formation.input]]
id = "in"
label = "Input"

[[formation.output]]
id = "out"
label = "Output"
`)
	writeFixture(t, store.LayoutPath("arrange-serial"), `schema = 1
boardId = "brd_arrange_serial"
boardRev = 3

[[node]]
id = "fmn_first"
x = 500
y = 500

[[node]]
id = "fmn_second"
x = 100
y = 100
`)
	boardBefore, err := store.ReadBoard("arrange-serial")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	layoutBefore, err := store.ReadLayout("arrange-serial")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}

	layoutLocked := make(chan struct{})
	releaseLayout := make(chan struct{})
	layoutLockDone := make(chan error, 1)
	go func() {
		layoutLockDone <- withFileLock(store.LayoutPath("arrange-serial"), func() error {
			close(layoutLocked)
			<-releaseLayout
			return nil
		})
	}()
	<-layoutLocked

	type arrangeOutcome struct {
		layout *LayoutDocument
		err    error
	}
	arrangeDone := make(chan arrangeOutcome, 1)
	go func() {
		layout, err := store.ArrangeLayout("arrange-serial", WriteOptions{ExpectedETag: layoutBefore.ETag})
		arrangeDone <- arrangeOutcome{layout: layout, err: err}
	}()

	boardLock := mutexFor(store.BoardPath("arrange-serial") + ".lock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if !boardLock.TryLock() {
			break
		}
		boardLock.Unlock()
		if time.Now().After(deadline) {
			close(releaseLayout)
			outcome := <-arrangeDone
			t.Fatalf("ArrangeLayout did not hold the board lock while waiting to persist layout: %v", outcome.err)
		}
		time.Sleep(time.Millisecond)
	}

	type wireOutcome struct {
		board *BoardDocument
		err   error
	}
	wireDone := make(chan wireOutcome, 1)
	go func() {
		board, err := store.WireFormationPorts("arrange-serial", FormationWireRequest{
			From:      "fmn_first:out",
			To:        "fmn_second:in",
			UpdatedBy: "agent:test",
		}, WriteOptions{ExpectedETag: boardBefore.ETag, ExpectedRev: boardBefore.Rev})
		wireDone <- wireOutcome{board: board, err: err}
	}()
	select {
	case outcome := <-wireDone:
		close(releaseLayout)
		<-arrangeDone
		t.Fatalf("concurrent wire completed inside Arrange critical section: %v", outcome.err)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseLayout)
	arranged := <-arrangeDone
	if err := <-layoutLockDone; err != nil {
		t.Fatalf("hold layout lock: %v", err)
	}
	if arranged.err != nil {
		t.Fatalf("arrange layout: %v", arranged.err)
	}
	if arranged.layout.BoardRev != boardBefore.Rev {
		t.Fatalf("arranged boardRev = %d, want serialized rev %d", arranged.layout.BoardRev, boardBefore.Rev)
	}
	wired := <-wireDone
	if wired.err != nil {
		t.Fatalf("wire after arrange: %v", wired.err)
	}
	if wired.board.Rev != boardBefore.Rev+1 {
		t.Fatalf("wired board rev = %d, want %d", wired.board.Rev, boardBefore.Rev+1)
	}

	staleLayout, err := store.ReadLayout("arrange-serial")
	if err != nil {
		t.Fatalf("read stale layout: %v", err)
	}
	rearranged, err := store.ArrangeLayout("arrange-serial", WriteOptions{ExpectedETag: staleLayout.ETag})
	if err != nil {
		t.Fatalf("arrange wired graph: %v", err)
	}
	byID := map[string]LayoutNode{}
	for _, node := range rearranged.Nodes {
		byID[node.ID] = node
	}
	if rearranged.BoardRev != wired.board.Rev || byID["fmn_first"].X >= byID["fmn_second"].X {
		t.Fatalf("rearranged wired graph = rev %d nodes %+v, want current rev and left-to-right depth", rearranged.BoardRev, byID)
	}
}

func TestUpdateLayoutNodesWildcardOnlyRecreatesMissingLayoutSidecar(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_session-search"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "fmn_research"
x = 120
y = 80
`)
	before := readFile(t, store.LayoutPath("session-search"))

	if _, err := store.UpdateLayoutNodes("session-search", []LayoutNode{{ID: "fmn_research", X: 320, Y: 240}}, WriteOptions{ExpectedETag: "*"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("wildcard existing layout error = %v, want ErrConflict", err)
	}
	if got := readFile(t, store.LayoutPath("session-search")); got != before {
		t.Fatalf("wildcard clobbered existing layout:\n%s", got)
	}
}

func TestS3InverseMutationsRequireFreshETags(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	created, err := store.CreateFormation("session-search", FormationCreateRequest{
		Type:      FormationTypeSolo,
		Title:     "Undo me",
		X:         300,
		Y:         180,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}

	if _, err := store.DeleteFormation("session-search", FormationDeleteRequest{
		ID:        created.Formation.ID,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale inverse delete error = %v, want ErrConflict", err)
	}

	result, err := store.DeleteFormation("session-search", FormationDeleteRequest{
		ID:        created.Formation.ID,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: created.Board.ETag, ExpectedRev: created.Board.Rev})
	if err != nil {
		t.Fatalf("fresh inverse delete: %v", err)
	}
	if result.Board.Rev != 9 {
		t.Fatalf("board rev = %d, want 9", result.Board.Rev)
	}
	if len(result.Board.Formations) != 0 {
		t.Fatalf("formations = %+v, want deleted formation gone", result.Board.Formations)
	}
	if len(result.Layout.Nodes) != 0 {
		t.Fatalf("layout nodes = %+v, want deleted formation node gone", result.Layout.Nodes)
	}
	if got := readFile(t, store.BoardPath("session-search")); strings.Contains(got, created.Formation.ID) {
		t.Fatalf("board still contains deleted formation id:\n%s", got)
	}
}

func TestS3UndoNeverRestoresWholeBoardSnapshots(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	created, err := store.CreateFormation("session-search", FormationCreateRequest{
		Type:      FormationTypePeer,
		Title:     "Peer review",
		X:         120,
		Y:         80,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}
	externalTitle := "Externally renamed board"
	external, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     &externalTitle,
		UpdatedBy: "agent:external",
	}, WriteOptions{ExpectedETag: created.Board.ETag, ExpectedRev: created.Board.Rev})
	if err != nil {
		t.Fatalf("external edit: %v", err)
	}

	if _, err := store.DeleteFormation("session-search", FormationDeleteRequest{
		ID:        created.Formation.ID,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: created.Board.ETag, ExpectedRev: created.Board.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("undo with stale create snapshot error = %v, want ErrConflict", err)
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read after stale undo: %v", err)
	}
	if after.Title != external.Title || after.Rev != external.Rev {
		t.Fatalf("stale undo restored an old board snapshot: after=%+v external=%+v", after, external)
	}
	if got := readFile(t, store.BoardPath("session-search")); !strings.Contains(got, `customFuture = "keep me"`) {
		t.Fatalf("undo path lost unknown fields:\n%s", got)
	}
}

func TestS3InversePrimitivesCoverBoardAndLayoutMutations(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Build the page"
beadId = "home-7kc4.5"

[[formation]]
id = "fmn_frame"
type = "orchestrated"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"

[[formation.output]]
id = "port_frame_out"
label = "Output"

[[formation.slot]]
id = "slot_lead"
label = "Lead"
controller = true
agentId = "mason"
harness = "codex"

[[formation.slot]]
id = "slot_worker"
label = "Worker"
controller = false

[[formation.brief]]
goal = "Frame it"
beadId = "home-7kc4.1"
files = ["README.md"]
links = ["https://example.invalid"]

[formation.verification]
id = "ver_frame"
kinds = ["code"]
criterion = "Tests pass"
onFail = "block"

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["code", "formation"]
criterion = "Review the frame"

[[connection]]
id = "edge_mission_frame"
from = "mis_showcase:out"
to = "fmn_frame:port_frame_in"

[[connection]]
id = "edge_frame_gate"
from = "fmn_frame:port_frame_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_judge"
from = "gate_review:judge"
to = "fmn_frame:port_frame_in"
`)
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "mis_showcase"
x = 80
y = -120

[[node]]
id = "fmn_frame"
x = 120
y = 80

[[node]]
id = "gate_review"
x = 440
y = 80

[[edge]]
id = "edge_frame_gate"
lane = "220"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	clearedBrief, err := store.ClearFormationBrief("session-search", FormationBriefClearRequest{
		FormationID: "fmn_frame",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("clear brief inverse: %v", err)
	}
	if clearedBrief.Formations[0].Brief != nil {
		t.Fatalf("brief after clear = %+v, want removed", clearedBrief.Formations[0].Brief)
	}
	if got := readFile(t, store.BoardPath("session-search")); strings.Contains(got, "[[formation.brief]]") {
		t.Fatalf("brief block still present after clear:\n%s", got)
	}

	rawBeforeRemoval := readFile(t, store.BoardPath("session-search"))
	_, err = store.RemoveFormationVerification("session-search", FormationVerificationRemovalRequest{
		FormationID: "fmn_frame",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: clearedBrief.ETag, ExpectedRev: clearedBrief.Rev})
	if err == nil || !strings.Contains(err.Error(), "legacy_inline_verification_requires_migration") {
		t.Fatalf("remove without replacement Gate error = %v, want migration rejection", err)
	}
	if rawAfterRejection := readFile(t, store.BoardPath("session-search")); rawAfterRejection != rawBeforeRemoval {
		t.Fatalf("rejected compatibility removal changed board\nbefore:\n%s\nafter:\n%s", rawBeforeRemoval, rawAfterRejection)
	}

	removedVerification, err := store.RemoveFormationVerification("session-search", FormationVerificationRemovalRequest{
		FormationID:       "fmn_frame",
		ReplacementGateID: "gate_review",
		UpdatedBy:         "agent:test",
	}, WriteOptions{ExpectedETag: clearedBrief.ETag, ExpectedRev: clearedBrief.Rev})
	if err != nil {
		t.Fatalf("remove verification inverse: %v", err)
	}
	if removedVerification.Formations[0].Verification != nil {
		t.Fatalf("verification after removal = %+v, want removed", removedVerification.Formations[0].Verification)
	}

	deletedGate, err := store.DeleteGate("session-search", GateDeleteRequest{
		ID:        "gate_review",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: removedVerification.ETag, ExpectedRev: removedVerification.Rev})
	if err != nil {
		t.Fatalf("delete gate inverse: %v", err)
	}
	if len(deletedGate.Board.Gates) != 0 {
		t.Fatalf("gates after delete = %+v, want none", deletedGate.Board.Gates)
	}
	for _, connection := range deletedGate.Board.Connections {
		if strings.Contains(connection.From, "gate_review") || strings.Contains(connection.To, "gate_review") {
			t.Fatalf("gate connection survived delete: %+v", connection)
		}
	}
	for _, node := range deletedGate.Layout.Nodes {
		if node.ID == "gate_review" {
			t.Fatalf("gate layout node survived delete: %+v", deletedGate.Layout.Nodes)
		}
	}

	deletedMission, err := store.DeleteMission("session-search", MissionDeleteRequest{
		ID:        "mis_showcase",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: deletedGate.Board.ETag, ExpectedRev: deletedGate.Board.Rev})
	if err != nil {
		t.Fatalf("delete mission inverse: %v", err)
	}
	if len(deletedMission.Board.Missions) != 0 {
		t.Fatalf("missions after delete = %+v, want none", deletedMission.Board.Missions)
	}
	if len(deletedMission.Board.Connections) != 0 {
		t.Fatalf("connections after deleting mission/gate = %+v, want all touched wires pruned", deletedMission.Board.Connections)
	}
	for _, node := range deletedMission.Layout.Nodes {
		if node.ID == "mis_showcase" {
			t.Fatalf("mission layout node survived delete: %+v", deletedMission.Layout.Nodes)
		}
	}
	if got := readFile(t, store.BoardPath("session-search")); !strings.Contains(got, `customFuture = "keep me"`) {
		t.Fatalf("inverse primitives lost unknown fields:\n%s", got)
	}
}

func TestS3SlotAssignmentUsesPersonaIDsWithoutSessionNames(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "peer"
title = "Frame"

[[formation.slot]]
id = "slot_peer_a"
label = "Peer A"
controller = false

[[formation.slot]]
id = "slot_peer_b"
label = "Peer B"
controller = false
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	assigned, err := store.AssignFormationSlot("session-search", FormationSlotAssignmentRequest{
		FormationID: "fmn_frame",
		SlotID:      "slot_peer_a",
		AgentID:     "susie",
		Harness:     "openai-codex",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("assign slot: %v", err)
	}
	assignedAgain, err := store.AssignFormationSlot("session-search", FormationSlotAssignmentRequest{
		FormationID: "fmn_frame",
		SlotID:      "slot_peer_b",
		AgentID:     "susie",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: assigned.ETag, ExpectedRev: assigned.Rev})
	if err != nil {
		t.Fatalf("assign same persona to another slot: %v", err)
	}

	slots := assignedAgain.Formations[0].Slots
	if slots[0].AgentID != "susie" || slots[0].Harness != "openai-codex" {
		t.Fatalf("slot A = %+v, want susie/openai-codex", slots[0])
	}
	if slots[1].AgentID != "susie" {
		t.Fatalf("slot B = %+v, want same persona reference", slots[1])
	}
	raw := readFile(t, store.BoardPath("session-search"))
	for _, forbidden := range []string{"sessionName", "sessionStem", "tmux"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("slot assignment stored runtime session field %q:\n%s", forbidden, raw)
		}
	}
}

func TestS3OrchestratedControllerIsExactlyOne(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_orch"
type = "orchestrated"
title = "Orchestrate"

[[formation.slot]]
id = "slot_controller"
label = "Controller"
controller = true

[[formation.slot]]
id = "slot_worker"
label = "Worker"
controller = false
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	after, err := store.SetFormationController("session-search", FormationControllerRequest{
		FormationID: "fmn_orch",
		SlotID:      "slot_worker",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("make controller: %v", err)
	}
	var controllers int
	var workerController, previousController bool
	for _, slot := range after.Formations[0].Slots {
		if slot.Controller {
			controllers++
		}
		if slot.ID == "slot_worker" {
			workerController = slot.Controller
		}
		if slot.ID == "slot_controller" {
			previousController = slot.Controller
		}
	}
	if controllers != 1 || !workerController || previousController {
		t.Fatalf("controller slots = %+v, want only slot_worker controller", after.Formations[0].Slots)
	}
}

func TestS3BriefPersistsAndInlineVerificationWriterFailsWithoutMutation(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	withBrief, err := store.SetFormationBrief("session-search", FormationBriefRequest{
		FormationID: "fmn_ship",
		Goal:        "Ship the change",
		BeadID:      "srv-abc.2",
		Files:       []string{"src/SessionPanel.tsx"},
		Links:       []string{"https://example.com/spec"},
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("set brief: %v", err)
	}
	rawBeforeVerification := readFile(t, store.BoardPath("session-search"))
	_, err = store.SetFormationVerification("session-search", FormationVerificationRequest{
		FormationID: "fmn_ship",
		Kinds:       []string{"code", "human"},
		Criterion:   "Tests pass and the handoff is clear.",
		OnFail:      "pushback",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: withBrief.ETag, ExpectedRev: withBrief.Rev})
	if err == nil || !strings.Contains(err.Error(), "legacy_inline_verification_requires_migration") {
		t.Fatalf("set verification error = %v, want stable migration rejection", err)
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after rejected verification: %v", err)
	}
	formation := after.Formations[0]
	if formation.Brief == nil || formation.Brief.Goal != "Ship the change" || formation.Brief.BeadID != "srv-abc.2" {
		t.Fatalf("brief = %+v, want goal and project bead", formation.Brief)
	}
	if formation.Verification != nil {
		t.Fatalf("verification = %+v, want retired writer to leave it absent", formation.Verification)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if raw != rawBeforeVerification {
		t.Fatalf("rejected verification write changed board\nbefore:\n%s\nafter:\n%s", rawBeforeVerification, raw)
	}
	for _, want := range []string{
		`customFuture = "keep me"`,
		`[formation.brief]`,
		`beadId = "srv-abc.2"`,
		`files = ["src/SessionPanel.tsx"]`,
		`links = ["https://example.com/spec"]`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("board TOML missing %q:\n%s", want, raw)
		}
	}
}

func TestRemoveFormationVerificationRejectsDuplicateLegacySectionsWithoutMutation(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("duplicate-verification"), `schema = 1
id = "brd_duplicate_verification"
slug = "duplicate-verification"
title = "Duplicate verification"
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
id = "ver_first"
kinds = ["code"]
criterion = "First check"
onFail = "block"

[formation.verification]
id = "ver_second"
kinds = ["human"]
criterion = "Second check"
onFail = "pushback"

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
	before, err := store.ReadBoard("duplicate-verification")
	if err != nil {
		t.Fatalf("read duplicate verification board: %v", err)
	}
	rawBefore := readFile(t, store.BoardPath("duplicate-verification"))

	_, err = store.RemoveFormationVerification("duplicate-verification", FormationVerificationRemovalRequest{
		FormationID:       "fmn_work",
		ReplacementGateID: "gate_review",
		UpdatedBy:         "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err == nil || !strings.Contains(err.Error(), LegacyInlineVerificationMigrationCode) {
		t.Fatalf("duplicate verification removal error = %v, want migration rejection", err)
	}
	if rawAfter := readFile(t, store.BoardPath("duplicate-verification")); rawAfter != rawBefore {
		t.Fatalf("duplicate verification rejection changed board\nbefore:\n%s\nafter:\n%s", rawBefore, rawAfter)
	}
}

func TestRemoveFormationVerificationDeletesSemanticDescendantTables(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("verification-descendant"), `schema = 1
id = "brd_verification_descendant"
slug = "verification-descendant"
title = "Verification descendant"
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

[formation.brief]
goal = "Preserve this sibling section"

[formation.verification.extra]
futureField = "must leave with its retired parent"

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
	before, err := store.ReadBoard("verification-descendant")
	if err != nil {
		t.Fatalf("read verification descendant board: %v", err)
	}
	if verification := before.Formations[0].Verification; verification == nil || verification.ID != "ver_work" || verification.Criterion != "Check the work" {
		t.Fatalf("legacy verification inspection = %+v, want populated parent fields preserved", verification)
	}

	after, err := store.RemoveFormationVerification("verification-descendant", FormationVerificationRemovalRequest{
		FormationID:       "fmn_work",
		ReplacementGateID: "gate_review",
		UpdatedBy:         "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("remove verification with descendant table: %v", err)
	}
	if after.Formations[0].Verification != nil {
		t.Fatalf("verification after removal = %+v, want removed", after.Formations[0].Verification)
	}
	raw := readFile(t, store.BoardPath("verification-descendant"))
	if strings.Contains(raw, "formation.verification") || strings.Contains(raw, "futureField") {
		t.Fatalf("retired verification descendant survived explicit removal:\n%s", raw)
	}
	if !strings.Contains(raw, `id = "gate_review"`) {
		t.Fatalf("replacement Gate was not preserved:\n%s", raw)
	}
	if !strings.Contains(raw, `goal = "Preserve this sibling section"`) {
		t.Fatalf("unrelated Formation section was not preserved:\n%s", raw)
	}
}

func TestRemoveFormationVerificationMigratesDescendantOnlyRepresentation(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	raw := strings.Replace(
		s4VerificationBoardFixture("block"),
		"[formation.verification]",
		"[formation.verification.extra]",
		1,
	)
	raw += `
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
	writeFixture(t, store.BoardPath("session-search"), raw)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read descendant-only verification board: %v", err)
	}
	formation, ok := findFormation(before.Formations, "fmn_work")
	if !ok || formation.Verification == nil {
		t.Fatalf("descendant-only verification inspection = %+v, want visible migration fence", formation.Verification)
	}

	after, err := store.RemoveFormationVerification("session-search", FormationVerificationRemovalRequest{
		FormationID:       "fmn_work",
		ReplacementGateID: "gate_review",
		UpdatedBy:         "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("remove descendant-only verification: %v", err)
	}
	afterFormation, ok := findFormation(after.Formations, "fmn_work")
	if !ok || afterFormation.Verification != nil {
		t.Fatalf("verification after descendant-only removal = %+v, want removed", afterFormation.Verification)
	}
	afterRaw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(afterRaw, "formation.verification") {
		t.Fatalf("descendant-only verification survived explicit migration:\n%s", afterRaw)
	}
	if !strings.Contains(afterRaw, `id = "gate_review"`) {
		t.Fatalf("replacement Gate was not preserved:\n%s", afterRaw)
	}
}

func TestS3FormationBriefRejectsUnsafeNonEmptyBeadID(t *testing.T) {
	for _, beadID := range []string{"nohyphen", "Home-123", "chlab/123", "../home-pfyv", "home-pfyv\n"} {
		t.Run(beadID, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"
`)
			before, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			if _, err := store.SetFormationBrief("session-search", FormationBriefRequest{
				FormationID: "fmn_ship",
				Goal:        "Ship the change",
				BeadID:      beadID,
				UpdatedBy:   "agent:test",
			}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}); !errors.Is(err, ErrInvalidSlug) {
				t.Fatalf("set brief beadId %q error = %v, want ErrInvalidSlug", beadID, err)
			}
		})
	}
}

func TestS3DynamicPortsKeepStableIDsAndPruneEdges(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_a"
label = "Input A"

[[formation.input]]
id = "port_ship_b"
label = "Input B"

[[formation.input]]
id = "port_ship_c"
label = "Input C"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[connection]]
id = "edge_keep"
from = "fmn_ship:port_ship_out"
to = "fmn_ship:port_ship_a"

[[connection]]
id = "edge_prune"
from = "fmn_ship:port_ship_out"
to = "fmn_ship:port_ship_b"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	withPort, err := store.AddFormationPort("session-search", FormationPortRequest{
		FormationID: "fmn_ship",
		Direction:   FormationPortInput,
		Label:       "Second input",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("add input: %v", err)
	}
	added := withPort.Formations[0].Inputs[len(withPort.Formations[0].Inputs)-1]
	if !strings.HasPrefix(added.ID, "port_") || added.ID == "port_ship_a" || added.ID == "port_ship_b" || added.ID == "port_ship_c" {
		t.Fatalf("added input id = %q, want fresh stable port_", added.ID)
	}
	after, err := store.RemoveFormationPort("session-search", FormationPortRemovalRequest{
		FormationID: "fmn_ship",
		PortID:      "port_ship_b",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: withPort.ETag, ExpectedRev: withPort.Rev})
	if err != nil {
		t.Fatalf("remove input: %v", err)
	}
	if len(after.Connections) != 1 || after.Connections[0].ID != "edge_keep" {
		t.Fatalf("connections after removing port = %+v, want only edge_keep", after.Connections)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "port_ship_b") || !strings.Contains(raw, "port_ship_c") || !strings.Contains(raw, added.ID) {
		t.Fatalf("port removal renumbered or failed to prune correctly:\n%s", raw)
	}
}

func TestS3WireRejectsSelfDuplicateAndSecondInput(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3ConnectionsBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	wired, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_frame:port_frame_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("wire ports: %v", err)
	}
	if len(wired.Connections) != 1 || !strings.HasPrefix(wired.Connections[0].ID, "edge_") {
		t.Fatalf("connections = %+v, want one stable edge", wired.Connections)
	}
	if _, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_frame:port_frame_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: wired.ETag, ExpectedRev: wired.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate wire error = %v, want ErrConflict", err)
	}
	if _, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_research:port_research_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: wired.ETag, ExpectedRev: wired.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("self wire error = %v, want ErrConflict", err)
	}
	if _, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_ship:port_ship_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: wired.ETag, ExpectedRev: wired.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second input wire error = %v, want ErrConflict", err)
	}
}

func TestS3RewireRejectsOccupiedTargetWithoutDroppingOriginal(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3ConnectionsBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	first, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_frame:port_frame_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("wire first: %v", err)
	}
	second, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_research:port_research_out",
		To:        "fmn_ship:port_ship_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: first.ETag, ExpectedRev: first.Rev})
	if err != nil {
		t.Fatalf("wire second: %v", err)
	}
	if _, err := store.RewireFormationTarget("session-search", FormationRewireRequest{
		From:       "fmn_frame:port_frame_out",
		PreviousTo: "fmn_research:port_research_in",
		To:         "fmn_ship:port_ship_in",
		UpdatedBy:  "agent:test",
	}, WriteOptions{ExpectedETag: second.ETag, ExpectedRev: second.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("rewire occupied target error = %v, want ErrConflict", err)
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read after failed rewire: %v", err)
	}
	if !hasConnection(after.Connections, "fmn_frame:port_frame_out", "fmn_research:port_research_in") {
		t.Fatalf("failed rewire dropped original connection: %+v", after.Connections)
	}
	if !hasConnection(after.Connections, "fmn_research:port_research_out", "fmn_ship:port_ship_in") {
		t.Fatalf("failed rewire changed occupied target connection: %+v", after.Connections)
	}
}

func TestS3HandRouteWritesLayoutOnly(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3ConnectionsBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	wired, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_frame:port_frame_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("wire ports: %v", err)
	}
	boardBeforeLane := readFile(t, store.BoardPath("session-search"))
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 8
updatedAt = "2026-06-03T16:02:00Z"
`)
	layout, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	routed, err := store.UpdateLayoutEdges("session-search", []LayoutEdge{{ID: wired.Connections[0].ID, Lane: "mid-2"}}, WriteOptions{ExpectedETag: layout.ETag})
	if err != nil {
		t.Fatalf("hand route edge: %v", err)
	}
	if got := readFile(t, store.BoardPath("session-search")); got != boardBeforeLane {
		t.Fatalf("hand route changed board definition:\n%s", got)
	}
	if len(routed.Edges) != 1 || routed.Edges[0].Lane != "mid-2" {
		t.Fatalf("layout edges = %+v, want hand-routed lane", routed.Edges)
	}
}

func TestS3GatePersistsKindsCriterionWithoutVerdictOrOnFail(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	result, err := store.CreateGate("session-search", GateCreateRequest{
		Title:     "Review gate",
		Kinds:     []string{"code", "human"},
		Criterion: "Research is sound and safe to build.",
		X:         410,
		Y:         220,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create gate: %v", err)
	}
	after := result.Board
	if len(after.Gates) != 1 || after.Gates[0].Title != "Review gate" || strings.Join(after.Gates[0].Kinds, ",") != "code,human" {
		t.Fatalf("gates = %+v, want review gate with code,human", after.Gates)
	}
	if result.Gate.ID != after.Gates[0].ID {
		t.Fatalf("created gate = %+v, want exact board gate %+v", result.Gate, after.Gates[0])
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "verdict") || strings.Contains(raw, "onFail") {
		t.Fatalf("gate persisted runtime verdict/onFail fields:\n%s", raw)
	}
	if strings.Contains(raw, "x = 410") || strings.Contains(raw, "y = 220") {
		t.Fatalf("gate layout coordinates leaked into board definition:\n%s", raw)
	}
	if strings.Contains(raw, "command") {
		t.Fatalf("pure gate unexpectedly persisted legacy command fields:\n%s", raw)
	}
	layout := result.Layout
	if layout.BoardRev != after.Rev {
		t.Fatalf("layout boardRev = %d, want %d", layout.BoardRev, after.Rev)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0].ID != after.Gates[0].ID || layout.Nodes[0].X != 410 || layout.Nodes[0].Y != 220 {
		t.Fatalf("layout nodes = %+v, want created gate at 410,220", layout.Nodes)
	}
}

func TestCreateGateHoldsBoardLockUntilCoherentLayoutResult(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	layoutLocked := make(chan struct{})
	releaseLayout := make(chan struct{})
	layoutLockDone := make(chan error, 1)
	go func() {
		layoutLockDone <- withFileLock(store.LayoutPath("session-search"), func() error {
			close(layoutLocked)
			<-releaseLayout
			return nil
		})
	}()
	<-layoutLocked

	type createOutcome struct {
		result *GateCreateResult
		err    error
	}
	createDone := make(chan createOutcome, 1)
	go func() {
		result, err := store.CreateGate("session-search", GateCreateRequest{
			Title:     "Coherent gate",
			Kinds:     []string{"code"},
			X:         448,
			Y:         280,
			UpdatedBy: "agent:test",
		}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
		createDone <- createOutcome{result: result, err: err}
	}()

	// The pair publisher stages the layout overlay before the board definition,
	// both under the board lock. While the layout lock is held the create must
	// block holding the board lock, and the authoritative board definition must
	// not advance -- so a reader can never observe a board node before its
	// layout placement is durable.
	boardLock := mutexFor(store.BoardPath("session-search") + ".lock")
	deadline := time.Now().Add(2 * time.Second)
	boardLockHeld := false
	for {
		if !boardLock.TryLock() {
			boardLockHeld = true
			break
		}
		boardLock.Unlock()
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !boardLockHeld {
		close(releaseLayout)
		outcome := <-createDone
		<-layoutLockDone
		t.Fatalf("CreateGate never held the board lock while its layout write was blocked: createErr=%v", outcome.err)
	}

	blocked, readErr := store.ReadBoard("session-search")
	if readErr != nil {
		close(releaseLayout)
		<-createDone
		<-layoutLockDone
		t.Fatalf("read board while create blocked: %v", readErr)
	}
	if blocked.Rev != before.Rev {
		close(releaseLayout)
		<-createDone
		<-layoutLockDone
		t.Fatalf("board advanced to rev %d while its layout write was blocked; the definition must not commit before the layout pair", blocked.Rev)
	}

	close(releaseLayout)
	outcome := <-createDone
	if err := <-layoutLockDone; err != nil {
		t.Fatalf("hold layout lock: %v", err)
	}
	if outcome.err != nil {
		t.Fatalf("create gate: %v", outcome.err)
	}
	if outcome.result.Board.Rev != before.Rev+1 {
		t.Fatalf("committed board rev = %d, want %d", outcome.result.Board.Rev, before.Rev+1)
	}
	if outcome.result.Board.Rev != outcome.result.Layout.BoardRev {
		t.Fatalf("returned board rev %d and layout boardRev %d are incoherent", outcome.result.Board.Rev, outcome.result.Layout.BoardRev)
	}
	if len(outcome.result.Layout.Nodes) != 1 || outcome.result.Layout.Nodes[0].ID != outcome.result.Gate.ID {
		t.Fatalf("returned layout nodes = %+v, want exact created gate %q", outcome.result.Layout.Nodes, outcome.result.Gate.ID)
	}
}

func TestS3GateFailBehaviorIsOnlyWiring(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3ConnectionsBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	withGateResult, err := store.CreateGate("session-search", GateCreateRequest{
		Title:     "Review gate",
		Kinds:     []string{"code"},
		Criterion: "Check it.",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create gate: %v", err)
	}
	withGate := withGateResult.Board
	gateID := withGate.Gates[0].ID
	after, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      gateID + ":fail",
		To:        "fmn_frame:port_frame_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: withGate.ETag, ExpectedRev: withGate.Rev})
	if err != nil {
		t.Fatalf("wire fail pushback: %v", err)
	}
	if len(after.Connections) != 1 || after.Connections[0].From != gateID+":fail" {
		t.Fatalf("connections = %+v, want fail wire as behavior", after.Connections)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "onFail") || strings.Contains(raw, "verdict") {
		t.Fatalf("gate fail behavior stored as field instead of wiring:\n%s", raw)
	}
}

func TestS3JudgeLoopAndChainUseConnectionsWithSingleSendAndReturn(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3JudgeBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	after, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1", "fmn_j2"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("set judge chain: %v", err)
	}
	var sends, returns int
	for _, connection := range after.Connections {
		if connection.From == "gate_review:judge" {
			sends++
		}
		if connection.To == "gate_review:judge" {
			returns++
		}
	}
	if sends != 1 || returns != 1 || len(after.Connections) != 3 {
		t.Fatalf("judge connections = %+v, want one send, one mid, one return", after.Connections)
	}
	if !containsString(after.Gates[0].Kinds, "formation") {
		t.Fatalf("gate kinds = %+v, want formation kind while judge attached", after.Gates[0].Kinds)
	}
}

func TestS3JudgeReturnCanMoveWithoutBreakingEntry(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3JudgeBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	first, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1", "fmn_j2"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("set first judge chain: %v", err)
	}
	after, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1", "fmn_j2", "fmn_j3"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: first.ETag, ExpectedRev: first.Rev})
	if err != nil {
		t.Fatalf("move judge return: %v", err)
	}
	if !hasConnection(after.Connections, "gate_review:judge", "fmn_j1:port_j1_in") {
		t.Fatalf("judge entry was not preserved: %+v", after.Connections)
	}
	if !hasConnection(after.Connections, "fmn_j3:port_j3_out", "gate_review:judge") {
		t.Fatalf("judge return did not move to j3: %+v", after.Connections)
	}
}

func TestS3JudgeChainRejectsSecondIncomingInputLikeNormalConnections(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3JudgeBoardFixture()+`
[[connection]]
id = "edge_existing_input"
from = "fmn_j2:port_j2_out"
to = "fmn_j1:port_j1_in"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if _, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("judge chain second input error = %v, want ErrConflict", err)
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after conflict: %v", err)
	}
	if after.Rev != before.Rev || len(after.Connections) != 1 {
		t.Fatalf("conflicting judge chain mutated board: before rev %d after %+v", before.Rev, after)
	}
}

func TestS3MissionCreateAcceptsProjectBeadIDAndSingleOut(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	result, err := store.CreateMission("session-search", MissionCreateRequest{
		Title:     "Showcase site",
		Goal:      "Build the showcase",
		BeadID:    "home-vdki.34.1",
		X:         150,
		Y:         90,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}
	after := result.Board
	if len(after.Missions) != 1 || !strings.HasPrefix(after.Missions[0].ID, "mis_") || after.Missions[0].BeadID != "home-vdki.34.1" {
		t.Fatalf("missions = %+v, want one project-backed mission", after.Missions)
	}
	if result.Mission.ID != after.Missions[0].ID {
		t.Fatalf("created mission = %+v, want exact board mission %+v", result.Mission, after.Missions[0])
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "chain") || strings.Count(raw, "out") != 0 {
		t.Fatalf("mission stored a chain or explicit dynamic port instead of fixed out:\n%s", raw)
	}
	if strings.Contains(raw, "x = 150") || strings.Contains(raw, "y = 90") {
		t.Fatalf("mission layout coordinates leaked into board definition:\n%s", raw)
	}
	layout := result.Layout
	if layout.BoardRev != after.Rev {
		t.Fatalf("layout boardRev = %d, want %d", layout.BoardRev, after.Rev)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0].ID != after.Missions[0].ID || layout.Nodes[0].X != 150 || layout.Nodes[0].Y != 90 {
		t.Fatalf("layout nodes = %+v, want created mission at 150,90", layout.Nodes)
	}
}

func TestS3MissionCreateRejectsUnsafeBeadID(t *testing.T) {
	for _, beadID := range []string{"", "nohyphen", "Home-123", "chlab/123", "../home-pfyv", "home-pfyv\n"} {
		t.Run(beadID, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
			before, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			if _, err := store.CreateMission("session-search", MissionCreateRequest{
				Title:     "Showcase site",
				Goal:      "Build the showcase",
				BeadID:    beadID,
				UpdatedBy: "agent:test",
			}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}); !errors.Is(err, ErrInvalidSlug) {
				t.Fatalf("create mission beadId %q error = %v, want ErrInvalidSlug", beadID, err)
			}
		})
	}
}

func TestS3MissionWireIsDerivedConnectionNoStoredChain(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Build it"
beadId = "home-7kc4.5"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	after, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "mis_showcase:out",
		To:        "fmn_frame:port_frame_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("wire mission: %v", err)
	}
	if len(after.Connections) != 1 || after.Connections[0].From != "mis_showcase:out" {
		t.Fatalf("connections = %+v, want mission out connection", after.Connections)
	}
	if got := readFile(t, store.BoardPath("session-search")); strings.Contains(got, "chain") {
		t.Fatalf("mission wire stored a chain instead of a connection:\n%s", got)
	}
}

func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 6, 3, 17, 0, 0, 0, time.UTC)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func operativeFileIdentityForTest(t *testing.T, path string) [2]uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat type for %s = %T, want *syscall.Stat_t", path, info.Sys())
	}
	return [2]uint64{uint64(stat.Dev), stat.Ino}
}

func minimalBoard(slug string, rev int) string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "` + slug + `"
title = "Improve session search"
rev = ` + strconv.Itoa(rev) + `
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"
`
}

func stringPtr(v string) *string {
	return &v
}

func s3ConnectionsBoardFixture() string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"

[[formation.output]]
id = "port_frame_out"
label = "Output"

[[formation]]
id = "fmn_research"
type = "solo"
title = "Research"

[[formation.input]]
id = "port_research_in"
label = "Input"

[[formation.output]]
id = "port_research_out"
label = "Output"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"
`
}

func s3JudgeBoardFixture() string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["code"]
criterion = "Check it."

[[formation]]
id = "fmn_j1"
type = "solo"
title = "Judge 1"

[[formation.input]]
id = "port_j1_in"
label = "Input"

[[formation.output]]
id = "port_j1_out"
label = "Output"

[[formation]]
id = "fmn_j2"
type = "solo"
title = "Judge 2"

[[formation.input]]
id = "port_j2_in"
label = "Input"

[[formation.output]]
id = "port_j2_out"
label = "Output"

[[formation]]
id = "fmn_j3"
type = "solo"
title = "Judge 3"

[[formation.input]]
id = "port_j3_in"
label = "Input"

[[formation.output]]
id = "port_j3_out"
label = "Output"
`
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasConnection(connections []BoardConnection, from, to string) bool {
	for _, connection := range connections {
		if connection.From == from && connection.To == to {
			return true
		}
	}
	return false
}
