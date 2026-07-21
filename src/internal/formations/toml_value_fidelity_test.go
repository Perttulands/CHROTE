package formations

import (
	"strings"
	"testing"
)

func TestTOMLInspectionUsesDecodedValuesForSupportedBoardFields(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := `schema = 1
id = 'brd_value_faithful'
slug = "value-faithful"
"ti\u0074le" = 'Coverage #1 board'
rev = 7
updatedBy = 'agent:#reader'
updatedAt = "2026-07-21T12:00:00Z"

[[mission]]
id = 'mis_main'
title = "Mission \"quoted\""
goal = """
Line one
Line \u0023 two
"""
beadId = 'ctx-ug7.31'

[[formation]]
id = 'fmn_work'
type = 'solo'
title = '''Formation #1
keeps \slashes'''

[formation.brief]
goal = 'Coverage #1 passes'
beadId = 'ctx-ug7.31'
files = [
  "notes/#1.md",
  'literal\path.md',
]
links = ['https://example.test/#proof', "escaped-\u0023-link"]

[formation.verification]
id = 'ver_work'
kinds = ['code', "human"]
criterion = '''
First # criterion line
Second criterion line
'''
"on\u0046ail" = 'block'

[[gate]]
id = 'gate_review'
title = 'Review #1'
kinds = ['human', "code"]
criterion = 'Coverage #1 passes'
`
	writeFixture(t, store.BoardPath("value-faithful"), raw)

	board, err := store.ReadBoard("value-faithful")
	if err != nil {
		t.Fatalf("read value-faithful board: %v", err)
	}
	if board.ID != "brd_value_faithful" || board.Title != "Coverage #1 board" || board.UpdatedBy != "agent:#reader" {
		t.Fatalf("decoded board metadata = id %q title %q updatedBy %q", board.ID, board.Title, board.UpdatedBy)
	}
	if len(board.Missions) != 1 || board.Missions[0].Goal != "Line one\nLine # two\n" {
		t.Fatalf("decoded mission = %+v, want multiline basic-string goal", board.Missions)
	}
	if len(board.Formations) != 1 {
		t.Fatalf("formations = %+v, want one", board.Formations)
	}
	formation := board.Formations[0]
	if formation.Title != "Formation #1\nkeeps \\slashes" {
		t.Fatalf("formation title = %q, want multiline literal value", formation.Title)
	}
	if formation.Brief == nil || formation.Brief.Goal != "Coverage #1 passes" {
		t.Fatalf("formation brief = %+v, want literal # preserved", formation.Brief)
	}
	wantFiles := []string{"notes/#1.md", `literal\path.md`}
	if !equalStrings(formation.Brief.Files, wantFiles) {
		t.Fatalf("brief files = %#v, want %#v", formation.Brief.Files, wantFiles)
	}
	wantLinks := []string{"https://example.test/#proof", "escaped-#-link"}
	if !equalStrings(formation.Brief.Links, wantLinks) {
		t.Fatalf("brief links = %#v, want %#v", formation.Brief.Links, wantLinks)
	}
	if formation.Verification == nil || formation.Verification.Criterion != "First # criterion line\nSecond criterion line\n" || formation.Verification.OnFail != "block" {
		t.Fatalf("verification = %+v, want TOML-decoded multiline fields", formation.Verification)
	}
	if !equalStrings(formation.Verification.Kinds, []string{"code", "human"}) {
		t.Fatalf("verification kinds = %#v", formation.Verification.Kinds)
	}
	if len(board.Gates) != 1 || board.Gates[0].Criterion != "Coverage #1 passes" || !equalStrings(board.Gates[0].Kinds, []string{"human", "code"}) {
		t.Fatalf("decoded gates = %+v", board.Gates)
	}
	if board.TOML != raw || board.ETag != etag([]byte(raw)) {
		t.Fatal("inspection changed raw TOML identity")
	}
}

func TestTOMLInspectionUsesDecodedValuesForSupportedLayoutFields(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	const unknownLayout = `futureLayout = '''
layout # future
stays byte exact
'''
`
	raw := `schema = 1
boardId = 'brd_layout_#1'
boardRev = 7
updatedAt = '2026-07-21T12:00:00Z#source'
` + unknownLayout + `

[["no\u0064e"]]
id = 'node_#1'
x = 10
y = -20

[[edge]]
id = "edge_\u00231"
lane = '''work # lane
continued'''
`
	writeFixture(t, store.LayoutPath("value-layout"), raw)

	layout, err := store.ReadLayout("value-layout")
	if err != nil {
		t.Fatalf("read value-faithful layout: %v", err)
	}
	if layout.BoardID != "brd_layout_#1" || layout.UpdatedAt != "2026-07-21T12:00:00Z#source" {
		t.Fatalf("decoded layout metadata = boardId %q updatedAt %q", layout.BoardID, layout.UpdatedAt)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0] != (LayoutNode{ID: "node_#1", X: 10, Y: -20}) {
		t.Fatalf("decoded layout nodes = %+v", layout.Nodes)
	}
	if len(layout.Edges) != 1 || layout.Edges[0] != (LayoutEdge{ID: "edge_#1", Lane: "work # lane\ncontinued"}) {
		t.Fatalf("decoded layout edges = %+v", layout.Edges)
	}
	if layout.TOML != raw || layout.ETag != etag([]byte(raw)) {
		t.Fatal("layout inspection changed raw TOML identity")
	}

	layout, err = store.UpdateLayoutMetadata("value-layout", LayoutMetadataPatch{}, WriteOptions{ExpectedETag: layout.ETag})
	if err != nil {
		t.Fatalf("update value-faithful layout metadata: %v", err)
	}
	if layout.UpdatedAt != fixedClock()().Format("2006-01-02T15:04:05Z07:00") || !strings.Contains(layout.TOML, unknownLayout) {
		t.Fatalf("layout update changed decoded metadata or unknown multiline bytes:\n%s", layout.TOML)
	}
	if len(layout.Edges) != 1 || layout.Edges[0].Lane != "work # lane\ncontinued" {
		t.Fatalf("layout update changed multiline edge lane = %+v", layout.Edges)
	}
}

func TestTOMLWriterConsumesKnownMultilineRangesAndPreservesUnknownValues(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	const unknownRoot = `futureRoot = '''
root # unknown
stays byte exact
'''
`
	const unknownSlot = `futureSlot = '''
slot # unknown
stays byte exact
'''
`
	const unknownGate = `futureGate = """
gate # unknown
stays byte exact
"""
`
	const unknownSection = `[future.section]
message = '''
section # unknown
stays byte exact
'''
`
	raw := `schema = 1
id = "brd_value_write"
slug = "value-write"
"title" = """
OLD_TITLE_CONTINUATION_SHOULD_GO
old # title
""" # title note belongs to the replaced field
rev = 7
updatedBy = "agent:old"
updatedAt = "2026-07-21T12:00:00Z"
` + unknownRoot + `
[[formation]]
id = 'fmn_work'
type = 'orchestrated'
title = "Work"

[[formation.slot]]
id = 'slot_worker'
label = "Worker"
agentId = """
agent:old
OLD_AGENT_CONTINUATION_SHOULD_GO
"""
harness = '''
claude-code
OLD_HARNESS_CONTINUATION_SHOULD_GO
'''
` + unknownSlot + `
[[gate]]
id = 'gate_review'
title = "Review"
kinds = [
  'human',
  "OLD_KIND_CONTINUATION_SHOULD_GO",
]
'criterion' = '''
old # criterion
OLD_CRITERION_CONTINUATION_SHOULD_GO
''' # criterion note belongs to the replaced field
` + unknownGate + `
` + unknownSection
	writeFixture(t, store.BoardPath("value-write"), raw)

	board, err := store.ReadBoard("value-write")
	if err != nil {
		t.Fatalf("read board before writes: %v", err)
	}
	board, err = store.UpdateBoardMetadata("value-write", BoardMetadataPatch{
		Title:     stringPtr("Replacement title"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		t.Fatalf("replace multiline board title: %v", err)
	}
	if board.Title != "Replacement title" {
		t.Fatalf("updated title = %q", board.Title)
	}

	board, err = store.AssignFormationSlot("value-write", FormationSlotAssignmentRequest{
		FormationID: "fmn_work",
		SlotID:      "slot_worker",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		t.Fatalf("remove multiline slot assignment: %v", err)
	}
	if len(board.Formations) != 1 || len(board.Formations[0].Slots) != 1 || board.Formations[0].Slots[0].AgentID != "" || board.Formations[0].Slots[0].Harness != "" {
		t.Fatalf("slot after assignment removal = %+v", board.Formations)
	}

	board, err = store.UpdateGate("value-write", GateUpdateRequest{
		GateID:    "gate_review",
		Kinds:     []string{"human"},
		Criterion: "Replacement # criterion",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		t.Fatalf("replace multiline Gate fields: %v", err)
	}
	if len(board.Gates) != 1 || board.Gates[0].Criterion != "Replacement # criterion" || !equalStrings(board.Gates[0].Kinds, []string{"human"}) {
		t.Fatalf("updated Gate = %+v", board.Gates)
	}

	got := readFile(t, store.BoardPath("value-write"))
	for _, removed := range []string{
		"OLD_TITLE_CONTINUATION_SHOULD_GO",
		"OLD_AGENT_CONTINUATION_SHOULD_GO",
		"OLD_HARNESS_CONTINUATION_SHOULD_GO",
		"OLD_KIND_CONTINUATION_SHOULD_GO",
		"OLD_CRITERION_CONTINUATION_SHOULD_GO",
	} {
		if strings.Contains(got, removed) {
			t.Fatalf("known multiline update left continuation %q:\n%s", removed, got)
		}
	}
	for _, preserved := range []string{unknownRoot, unknownSlot, unknownGate, unknownSection} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("surgical writes changed unknown multiline bytes %q:\n%s", preserved, got)
		}
	}
}

func TestTOMLBoardWritesRejectInvalidDefinitionSourceWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		tail string
	}{
		{
			name: "duplicate decoded key identity",
			tail: "title = \"First\"\n\"ti\\u0074le\" = 'Second'\n",
		},
		{
			name: "unsupported known value type",
			tail: "[[gate]]\nid = \"gate_review\"\ntitle = \"Review\"\nkinds = [\"human\"]\ncriterion = 42\n",
		},
		{
			name: "malformed unknown multiline value",
			tail: "future = \"\"\"unterminated\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			raw := "schema = 1\nid = \"brd_invalid\"\nslug = \"invalid-source\"\nrev = 7\nupdatedAt = \"2026-07-21T12:00:00Z\"\n" + test.tail
			path := store.BoardPath("invalid-source")
			writeFixture(t, path, raw)

			_, err := store.UpdateBoardMetadata("invalid-source", BoardMetadataPatch{
				Title:     stringPtr("must not persist"),
				UpdatedBy: "agent:test",
			}, WriteOptions{ExpectedETag: etag([]byte(raw)), ExpectedRev: 7})
			if err == nil || !strings.HasPrefix(err.Error(), "invalid_definition_source:") {
				t.Fatalf("invalid source error = %v, want stable invalid_definition_source", err)
			}
			if got := readFile(t, path); got != raw {
				t.Fatalf("rejected board write changed bytes:\n got %q\nwant %q", got, raw)
			}
		})
	}
}

func TestTOMLPairedAndLayoutWritesRejectBeforeEitherDefinitionChanges(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	boardRaw := `schema = 1
id = "brd_invalid_pair"
slug = "invalid-pair"
title = "Invalid pair"
rev = 7
updatedAt = "2026-07-21T12:00:00Z"

[[gate]]
id = "gate_existing"
title = "Existing"
kinds = ["human"]
criterion = 42
`
	layoutRaw := `schema = 1
boardId = "brd_invalid_pair"
boardRev = 7
updatedAt = "2026-07-21T12:00:00Z"
future = '''layout # future'''
`
	writeFixture(t, store.BoardPath("invalid-pair"), boardRaw)
	writeFixture(t, store.LayoutPath("invalid-pair"), layoutRaw)

	_, err := store.CreateGate("invalid-pair", GateCreateRequest{
		Title:     "Must not persist",
		Kinds:     []string{"human"},
		Criterion: "Never written",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: etag([]byte(boardRaw)), ExpectedRev: 7})
	if err == nil || !strings.HasPrefix(err.Error(), "invalid_definition_source:") {
		t.Fatalf("invalid paired source error = %v, want stable invalid_definition_source", err)
	}
	if got := readFile(t, store.BoardPath("invalid-pair")); got != boardRaw {
		t.Fatalf("rejected paired write changed board:\n%s", got)
	}
	if got := readFile(t, store.LayoutPath("invalid-pair")); got != layoutRaw {
		t.Fatalf("rejected paired write changed layout:\n%s", got)
	}

	invalidLayout := `schema = 1
boardId = "brd_invalid_layout"
"board\u0049d" = 'brd_duplicate'
boardRev = 7
updatedAt = "2026-07-21T12:00:00Z"
`
	writeFixture(t, store.LayoutPath("invalid-layout"), invalidLayout)
	_, err = store.UpdateLayoutMetadata("invalid-layout", LayoutMetadataPatch{}, WriteOptions{ExpectedETag: etag([]byte(invalidLayout))})
	if err == nil || !strings.HasPrefix(err.Error(), "invalid_definition_source:") {
		t.Fatalf("invalid layout source error = %v, want stable invalid_definition_source", err)
	}
	if got := readFile(t, store.LayoutPath("invalid-layout")); got != invalidLayout {
		t.Fatalf("rejected layout write changed bytes:\n%s", got)
	}
}

func TestTOMLPairedWritesRejectInvalidLayoutBeforeBoardMutation(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Store, WriteOptions) error
	}{
		{
			name: "create Formation",
			run: func(store *Store, opts WriteOptions) error {
				_, err := store.CreateFormation("invalid-pair", FormationCreateRequest{Type: FormationTypeSolo, Title: "Must not persist"}, opts)
				return err
			},
		},
		{
			name: "delete Formation",
			run: func(store *Store, opts WriteOptions) error {
				_, err := store.DeleteFormation("invalid-pair", FormationDeleteRequest{ID: "fmn_existing"}, opts)
				return err
			},
		},
		{
			name: "create Gate",
			run: func(store *Store, opts WriteOptions) error {
				_, err := store.CreateGate("invalid-pair", GateCreateRequest{Title: "Must not persist", Kinds: []string{"human"}}, opts)
				return err
			},
		},
		{
			name: "delete Gate",
			run: func(store *Store, opts WriteOptions) error {
				_, err := store.DeleteGate("invalid-pair", GateDeleteRequest{ID: "gate_existing"}, opts)
				return err
			},
		},
		{
			name: "create Mission",
			run: func(store *Store, opts WriteOptions) error {
				_, err := store.CreateMission("invalid-pair", MissionCreateRequest{Title: "Must not persist", BeadID: "ctx-ug7.31"}, opts)
				return err
			},
		},
		{
			name: "delete Mission",
			run: func(store *Store, opts WriteOptions) error {
				_, err := store.DeleteMission("invalid-pair", MissionDeleteRequest{ID: "mis_existing"}, opts)
				return err
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			boardRaw := `schema = 1
id = "brd_invalid_pair"
slug = "invalid-pair"
title = "Invalid pair"
rev = 7
updatedAt = "2026-07-21T12:00:00Z"

[[mission]]
id = "mis_existing"
title = "Existing Mission"
goal = "Stay"
beadId = "ctx-ug7.31"

[[formation]]
id = "fmn_existing"
type = "solo"
title = "Existing Formation"

[[gate]]
id = "gate_existing"
title = "Existing Gate"
kinds = ["human"]
criterion = "Stay"
`
			layoutRaw := `schema = 1
boardId = "brd_invalid_pair"
"board\u0049d" = 'brd_duplicate'
boardRev = 7
updatedAt = "2026-07-21T12:00:00Z"
`
			writeFixture(t, store.BoardPath("invalid-pair"), boardRaw)
			writeFixture(t, store.LayoutPath("invalid-pair"), layoutRaw)

			err := operation.run(store, WriteOptions{ExpectedETag: etag([]byte(boardRaw)), ExpectedRev: 7})
			if err == nil || !strings.HasPrefix(err.Error(), "invalid_definition_source:") {
				t.Fatalf("paired write error = %v, want stable invalid_definition_source", err)
			}
			if got := readFile(t, store.BoardPath("invalid-pair")); got != boardRaw {
				t.Fatalf("rejected paired write changed board:\n got %q\nwant %q", got, boardRaw)
			}
			if got := readFile(t, store.LayoutPath("invalid-pair")); got != layoutRaw {
				t.Fatalf("rejected paired write changed layout:\n got %q\nwant %q", got, layoutRaw)
			}
		})
	}
}

func TestTOMLPairedCreatesValidateGeneratedCandidatesBeforePublication(t *testing.T) {
	tests := []struct {
		name       string
		boardTail  string
		layoutTail string
		create     func(*Store, WriteOptions) error
	}{
		{
			name:      "Formation table-array collision",
			boardTail: "formation = []\n",
			create: func(store *Store, opts WriteOptions) error {
				_, err := store.CreateFormation("candidate-validation", FormationCreateRequest{Type: FormationTypeSolo, Title: "Must not persist"}, opts)
				return err
			},
		},
		{
			name:      "Gate table-array collision",
			boardTail: "gate = []\n",
			create: func(store *Store, opts WriteOptions) error {
				_, err := store.CreateGate("candidate-validation", GateCreateRequest{Title: "Must not persist", Kinds: []string{"human"}}, opts)
				return err
			},
		},
		{
			name:      "Mission table-array collision",
			boardTail: "mission = []\n",
			create: func(store *Store, opts WriteOptions) error {
				_, err := store.CreateMission("candidate-validation", MissionCreateRequest{Title: "Must not persist", BeadID: "ctx-ug7.31"}, opts)
				return err
			},
		},
		{
			name:       "layout node table-array collision",
			layoutTail: "node = []\n",
			create: func(store *Store, opts WriteOptions) error {
				_, err := store.CreateFormation("candidate-validation", FormationCreateRequest{Type: FormationTypeSolo, Title: "Must not persist"}, opts)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			boardRaw := `schema = 1
id = "brd_candidate_validation"
slug = "candidate-validation"
title = "Candidate validation"
rev = 7
updatedAt = "2026-07-21T12:00:00Z"
` + test.boardTail
			layoutRaw := `schema = 1
boardId = "brd_candidate_validation"
boardRev = 7
updatedAt = "2026-07-21T12:00:00Z"
` + test.layoutTail
			writeFixture(t, store.BoardPath("candidate-validation"), boardRaw)
			writeFixture(t, store.LayoutPath("candidate-validation"), layoutRaw)

			err := test.create(store, WriteOptions{ExpectedETag: etag([]byte(boardRaw)), ExpectedRev: 7})
			if err == nil || !strings.HasPrefix(err.Error(), "invalid_definition_source:") {
				t.Fatalf("candidate validation error = %v, want stable invalid_definition_source", err)
			}
			if got := readFile(t, store.BoardPath("candidate-validation")); got != boardRaw {
				t.Fatalf("rejected create changed board:\n got %q\nwant %q", got, boardRaw)
			}
			if got := readFile(t, store.LayoutPath("candidate-validation")); got != layoutRaw {
				t.Fatalf("rejected create changed layout:\n got %q\nwant %q", got, layoutRaw)
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
