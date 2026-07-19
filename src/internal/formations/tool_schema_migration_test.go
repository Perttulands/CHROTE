package formations

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const toolSchemaMigrationJudgeSendBlock = `[[connection]]
id = "edge_judge_send"
from = "gate_review:judge"
to = "fmn_judge_a:port_judge_a_in"
`

const toolSchemaMigrationJudgeMidBlock = `[[connection]]
id = "edge_judge_mid"
from = "fmn_judge_a:port_judge_a_out"
to = "fmn_judge_b:port_judge_b_in"
`

const toolSchemaMigrationJudgeReturnBlock = `[[connection]]
id = "edge_judge_return"
from = "fmn_judge_b:port_judge_b_out"
to = "gate_review:judge"
`

func TestToolSchemaMigrationNormalizesSafeLegacyGraphOnceWithoutOwningRevision(t *testing.T) {
	raw := []byte(toolSchemaMigrationLegacyFixture())
	wantSource := append([]byte(nil), raw...)
	source, err := parseBoard(raw)
	if err != nil {
		t.Fatalf("parse safe schema-1 source: %v", err)
	}
	if report := ValidateBoard(source); len(report.Errors) != 0 {
		t.Fatalf("safe schema-1 source has unrelated structural errors: %+v", report.Errors)
	}
	judgeChain := judgeChainForGate(source, "gate_review")
	if len(judgeChain) != 2 || judgeChain[0].ID != "fmn_judge_a" || judgeChain[1].ID != "fmn_judge_b" {
		t.Fatalf("safe schema-1 judge chain = %+v, want exact A then B linear chain", judgeChain)
	}

	migrated, err := migrateBoardToToolSchema(raw)
	if err != nil {
		t.Fatalf("migrate safe schema-1 board: %v", err)
	}
	if bytes.Equal(migrated, raw) {
		t.Fatal("safe schema-1 migration returned the unchanged source")
	}
	if !bytes.Equal(raw, wantSource) {
		t.Fatal("pure schema migration mutated its source buffer")
	}

	doc := parseTOMLDocument(migrated)
	if got := doc.intValue("schema"); got != CurrentBoardSchema {
		t.Fatalf("migrated schema = %d, want %d", got, CurrentBoardSchema)
	}
	// The later first-Tool writer owns the operation's single revision bump.
	// This pure prerequisite must not consume or duplicate it.
	if got := doc.intValue("rev"); got != 7 {
		t.Fatalf("pure migration changed board revision = %d, want preserved 7", got)
	}

	for _, port := range []struct {
		id        string
		direction string
	}{
		{id: "port_work_in", direction: FormationPortInput},
		{id: "port_work_out", direction: FormationPortOutput},
		{id: "port_feedback_in", direction: FormationPortInput},
		{id: "port_feedback_out", direction: FormationPortOutput},
		{id: "port_judge_a_in", direction: FormationPortInput},
		{id: "port_judge_a_out", direction: FormationPortOutput},
		{id: "port_judge_b_in", direction: FormationPortInput},
		{id: "port_judge_b_out", direction: FormationPortOutput},
	} {
		assertToolSchemaMigrationPortDefaults(t, migrated, port.id, port.direction)
	}

	for _, connection := range []struct {
		id      string
		channel string
	}{
		{id: "edge_start", channel: "workflow"},
		{id: "edge_review", channel: "workflow"},
		{id: "edge_judge_send", channel: "judge"},
		{id: "edge_judge_mid", channel: "judge"},
		{id: "edge_judge_return", channel: "judge"},
	} {
		assertToolSchemaMigrationConnectionChannel(t, migrated, connection.id, connection.channel)
	}

	if got := toolSchemaMigrationWithoutOwnedFields(migrated, renderInt(NewBoardSchema)); !bytes.Equal(got, wantSource) {
		t.Fatalf("schema migration rewrote bytes outside schema/typed-port/channel fields:\n got %q\nwant %q", got, wantSource)
	}
	board, err := parseBoard(migrated)
	if err != nil {
		t.Fatalf("parse migrated board: %v", err)
	}
	if len(board.Tools) != 0 {
		t.Fatalf("pure schema migration authored Tools = %+v, want none", board.Tools)
	}

	again, err := migrateBoardToToolSchema(migrated)
	if err != nil {
		t.Fatalf("repeat schema migration: %v", err)
	}
	if !bytes.Equal(again, migrated) {
		t.Fatalf("repeat schema migration was not byte-idempotent:\n got %q\nwant %q", again, migrated)
	}
	if err := toolSchemaMigrationReadAtSchemaCeiling(migrated, NewBoardSchema); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("schema-1 reader error = %v, want ErrUnsupportedSchema for migrated schema 2", err)
	}
}

func TestToolSchemaMigrationKeepsCanonicalSchemaTwoByteStable(t *testing.T) {
	raw := []byte(toolSchemaMigrationCanonicalFixture())
	wantSource := append([]byte(nil), raw...)

	migrated, err := migrateBoardToToolSchema(raw)
	if err != nil {
		t.Fatalf("accept canonical schema-2 board: %v", err)
	}
	if !bytes.Equal(migrated, wantSource) {
		t.Fatalf("canonical schema-2 migration was not an exact no-op:\n got %q\nwant %q", migrated, wantSource)
	}
	if !bytes.Equal(raw, wantSource) {
		t.Fatal("canonical schema-2 check mutated its source buffer")
	}
}

func TestToolSchemaMigrationAcceptsValidSignedSchemaIntegers(t *testing.T) {
	t.Run("+1 migrates", func(t *testing.T) {
		base := toolSchemaMigrationLegacyFixture()
		plusOne := replaceToolSchemaMigrationFixture(t, base, `schema = 1 # compatibility schema`, `schema = +1 # compatibility schema`)
		migrated, err := migrateBoardToToolSchema([]byte(plusOne))
		if err != nil {
			t.Fatalf("migrate valid TOML +1 schema: %v", err)
		}
		if got := parseTOMLDocument(migrated).intValue("schema"); got != CurrentBoardSchema {
			t.Fatalf("migrated +1 schema = %d, want %d", got, CurrentBoardSchema)
		}
		assertToolSchemaMigrationPortDefaults(t, migrated, "port_work_in", FormationPortInput)
		assertToolSchemaMigrationPortDefaults(t, migrated, "port_work_out", FormationPortOutput)
		assertToolSchemaMigrationConnectionChannel(t, migrated, "edge_start", "workflow")
		assertToolSchemaMigrationConnectionChannel(t, migrated, "edge_judge_send", "judge")
		assertToolSchemaMigrationConnectionChannel(t, migrated, "edge_judge_return", "judge")
		if got := toolSchemaMigrationWithoutOwnedFields(migrated, "+1"); !bytes.Equal(got, []byte(plusOne)) {
			t.Fatalf("+1 migration rewrote bytes outside owned fields:\n got %q\nwant %q", got, plusOne)
		}
	})

	t.Run("+2 canonical no-op", func(t *testing.T) {
		plusTwo := replaceToolSchemaMigrationFixture(
			t,
			toolSchemaMigrationCanonicalFixture(),
			`schema = 2 # canonical schema`,
			`schema = +2 # canonical schema`,
		)
		stable, err := migrateBoardToToolSchema([]byte(plusTwo))
		if err != nil {
			t.Fatalf("accept valid TOML +2 schema: %v", err)
		}
		if !bytes.Equal(stable, []byte(plusTwo)) {
			t.Fatalf("canonical +2 schema was not byte-stable:\n got %q\nwant %q", stable, plusTwo)
		}
	})
}

func TestToolSchemaMigrationAcceptsOnlyExactSchemaOneOrCanonicalSchemaTwo(t *testing.T) {
	base := toolSchemaMigrationLegacyFixture()
	schemaLine := `schema = 1 # compatibility schema`
	tests := []struct {
		name      string
		raw       string
		wantError error
	}{
		{name: "schema zero", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine, `schema = 0`)},
		{name: "negative schema", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine, `schema = -1`)},
		{name: "missing schema", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine+"\n", "")},
		{name: "leading-zero schema one", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine, `schema = 01`)},
		{name: "quoted schema one", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine, `schema = "1"`)},
		{name: "floating schema one", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine, `schema = 1.0`)},
		{name: "duplicate schema", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine, "schema = 1\nschema = 1")},
		{name: "schema two missing canonical owned fields", raw: replaceToolSchemaMigrationFixture(t, base, schemaLine, `schema = 2`)},
		{
			name:      "future schema",
			raw:       replaceToolSchemaMigrationFixture(t, base, schemaLine, `schema = 3`),
			wantError: ErrUnsupportedSchema,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolSchemaMigrationRejected(t, tt.raw, tt.wantError, "")
		})
	}
}

func TestToolSchemaMigrationAcceptsHumanOnlyAndSingleFormationJudgeTopologies(t *testing.T) {
	base := toolSchemaMigrationLegacyFixture()
	humanOnly := removeToolSchemaMigrationFixtureBlocks(
		t,
		base,
		toolSchemaMigrationJudgeSendBlock,
		toolSchemaMigrationJudgeMidBlock,
		toolSchemaMigrationJudgeReturnBlock,
	)
	humanOnly = replaceToolSchemaMigrationFixture(t, humanOnly, `kinds = ["human", "formation"]`, `kinds = ["human"]`)

	singleJudge := removeToolSchemaMigrationFixtureBlocks(
		t,
		base,
		toolSchemaMigrationJudgeMidBlock,
		toolSchemaMigrationJudgeReturnBlock,
	) + `
[[connection]]
id = "edge_judge_single_return"
from = "fmn_judge_a:port_judge_a_out"
to = "gate_review:judge"
`

	tests := []struct {
		name          string
		raw           string
		judgeChain    []string
		judgeEdgeIDs  []string
		workflowEdges []string
	}{
		{
			name:          "human-only Gate without judge edges",
			raw:           humanOnly,
			workflowEdges: []string{"edge_start", "edge_review"},
		},
		{
			name:          "one-Formation judge send and return",
			raw:           singleJudge,
			judgeChain:    []string{"fmn_judge_a"},
			judgeEdgeIDs:  []string{"edge_judge_send", "edge_judge_single_return"},
			workflowEdges: []string{"edge_start", "edge_review"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := parseBoard([]byte(tt.raw))
			if err != nil {
				t.Fatalf("parse safe source: %v", err)
			}
			if report := ValidateBoard(source); len(report.Errors) != 0 {
				t.Fatalf("safe source has structural errors: %+v", report.Errors)
			}
			chain := judgeChainForGate(source, "gate_review")
			if len(chain) != len(tt.judgeChain) {
				t.Fatalf("source judge chain = %+v, want ids %v", chain, tt.judgeChain)
			}
			for i, wantID := range tt.judgeChain {
				if chain[i].ID != wantID {
					t.Fatalf("source judge chain[%d] = %q, want %q", i, chain[i].ID, wantID)
				}
			}

			migrated, err := migrateBoardToToolSchema([]byte(tt.raw))
			if err != nil {
				t.Fatalf("migrate safe topology: %v", err)
			}
			for _, edgeID := range tt.workflowEdges {
				assertToolSchemaMigrationConnectionChannel(t, migrated, edgeID, "workflow")
			}
			for _, edgeID := range tt.judgeEdgeIDs {
				assertToolSchemaMigrationConnectionChannel(t, migrated, edgeID, "judge")
			}
			again, err := migrateBoardToToolSchema(migrated)
			if err != nil {
				t.Fatalf("repeat safe topology migration: %v", err)
			}
			if !bytes.Equal(again, migrated) {
				t.Fatal("safe topology migration was not byte-idempotent")
			}
		})
	}
}

func TestToolSchemaMigrationReusesOnlyExactOwnedFieldsAndRejectsCollisions(t *testing.T) {
	base := toolSchemaMigrationLegacyFixture()
	inputLabel := `label = "Work input"`
	outputLabel := `label = "Work result" # output label`
	edgeStart := `[[connection]]
id = "edge_start"
from = "mis_main:out"`
	edgeJudgeSend := `[[connection]]
id = "edge_judge_send"
from = "gate_review:judge"`

	prefilledInput := inputLabel + `
direction = "input"
kind = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]
required = true
role = "data"`
	prefilledOutput := outputLabel + `
direction = "output"
kind = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]`
	prefilled := replaceToolSchemaMigrationFixture(t, base, inputLabel, prefilledInput)
	prefilled = replaceToolSchemaMigrationFixture(t, prefilled, outputLabel, prefilledOutput)
	prefilled = replaceToolSchemaMigrationFixture(t, prefilled, edgeStart, `[[connection]]
id = "edge_start"
channel = "workflow"
from = "mis_main:out"`)
	prefilled = replaceToolSchemaMigrationFixture(t, prefilled, edgeJudgeSend, `[[connection]]
id = "edge_judge_send"
channel = "judge"
from = "gate_review:judge"`)

	migrated, err := migrateBoardToToolSchema([]byte(prefilled))
	if err != nil {
		t.Fatalf("migrate exact prefilled schema-1 fields: %v", err)
	}
	assertToolSchemaMigrationPortDefaults(t, migrated, "port_work_in", FormationPortInput)
	assertToolSchemaMigrationPortDefaults(t, migrated, "port_work_out", FormationPortOutput)
	assertToolSchemaMigrationConnectionChannel(t, migrated, "edge_start", "workflow")
	assertToolSchemaMigrationConnectionChannel(t, migrated, "edge_judge_send", "judge")
	for _, fragment := range []string{
		prefilledInput,
		prefilledOutput,
		"id = \"edge_start\"\nchannel = \"workflow\"\nfrom = \"mis_main:out\"",
		"id = \"edge_judge_send\"\nchannel = \"judge\"\nfrom = \"gate_review:judge\"",
	} {
		if !strings.Contains(string(migrated), fragment) {
			t.Fatalf("exact prefilled migration field bytes were rewritten or duplicated; missing %q", fragment)
		}
	}
	again, err := migrateBoardToToolSchema(migrated)
	if err != nil {
		t.Fatalf("repeat exact-prefilled migration: %v", err)
	}
	if !bytes.Equal(again, migrated) {
		t.Fatalf("exact-prefilled migration was not byte-idempotent:\n got %q\nwant %q", again, migrated)
	}

	withInputFields := func(fields string) string {
		return replaceToolSchemaMigrationFixture(t, base, inputLabel, inputLabel+"\n"+fields)
	}
	withOutputFields := func(fields string) string {
		return replaceToolSchemaMigrationFixture(t, base, outputLabel, outputLabel+"\n"+fields)
	}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "conflicting input direction", raw: withInputFields(`direction = "output"`)},
		{name: "conflicting input kind", raw: withInputFields(`kind = "gate_feedback"`)},
		{name: "conflicting input media", raw: withInputFields(`acceptedMediaTypes = ["application/json"]`)},
		{name: "conflicting input required", raw: withInputFields(`required = false`)},
		{name: "conflicting input role", raw: withInputFields(`role = "retry_control"`)},
		{name: "duplicate input direction", raw: withInputFields("direction = \"input\"\ndirection = \"input\"")},
		{name: "duplicate input kind", raw: withInputFields("kind = \"work\"\nkind = \"work\"")},
		{name: "duplicate input media", raw: withInputFields("acceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]\nacceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]")},
		{name: "duplicate input required", raw: withInputFields("required = true\nrequired = true")},
		{name: "duplicate input role", raw: withInputFields("role = \"data\"\nrole = \"data\"")},
		{name: "conflicting output direction", raw: withOutputFields(`direction = "input"`)},
		{name: "conflicting output kind", raw: withOutputFields(`kind = "gate_feedback"`)},
		{name: "conflicting output media", raw: withOutputFields(`acceptedMediaTypes = ["application/json"]`)},
		{name: "output carries input-only required", raw: withOutputFields(`required = true`)},
		{name: "output carries input-only role", raw: withOutputFields(`role = "data"`)},
		{name: "duplicate output direction", raw: withOutputFields("direction = \"output\"\ndirection = \"output\"")},
		{name: "duplicate output kind", raw: withOutputFields("kind = \"work\"\nkind = \"work\"")},
		{name: "duplicate output media", raw: withOutputFields("acceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]\nacceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]")},
		{
			name: "workflow edge claims judge channel",
			raw: replaceToolSchemaMigrationFixture(t, base, edgeStart, `[[connection]]
id = "edge_start"
channel = "judge"
from = "mis_main:out"`),
		},
		{
			name: "judge edge claims workflow channel",
			raw: replaceToolSchemaMigrationFixture(t, base, edgeJudgeSend, `[[connection]]
id = "edge_judge_send"
channel = "workflow"
from = "gate_review:judge"`),
		},
		{
			name: "duplicate connection channel",
			raw: replaceToolSchemaMigrationFixture(t, base, edgeStart, `[[connection]]
id = "edge_start"
channel = "workflow"
channel = "workflow"
from = "mis_main:out"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolSchemaMigrationRejected(t, tt.raw, nil, "")
		})
	}
}

func TestToolSchemaMigrationRejectsEveryOwnedFieldCollisionOnCanonicalSchemaTwo(t *testing.T) {
	base := toolSchemaMigrationCanonicalFixture()
	inputField := func(key, replacement string) string {
		return replaceToolSchemaMigrationPortField(t, base, "port_work_in", key, replacement)
	}
	outputField := func(key, replacement string) string {
		return replaceToolSchemaMigrationPortField(t, base, "port_work_out", key, replacement)
	}
	outputFieldsAfterLabel := func(fields string) string {
		return outputField("label", "label = \"Work output\"\n"+fields)
	}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "wrong input direction", raw: inputField("direction", `direction = "output"`)},
		{name: "wrong input kind", raw: inputField("kind", `kind = "gate_feedback"`)},
		{name: "wrong input media", raw: inputField("acceptedMediaTypes", `acceptedMediaTypes = ["application/json"]`)},
		{name: "wrong input required", raw: inputField("required", `required = false`)},
		{name: "wrong input role", raw: inputField("role", `role = "retry_control"`)},
		{name: "duplicate input direction", raw: inputField("direction", "direction = \"input\"\ndirection = \"input\"")},
		{name: "duplicate input kind", raw: inputField("kind", "kind = \"work\"\nkind = \"work\"")},
		{name: "duplicate input media", raw: inputField("acceptedMediaTypes", "acceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]\nacceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]")},
		{name: "duplicate input required", raw: inputField("required", "required = true\nrequired = true")},
		{name: "duplicate input role", raw: inputField("role", "role = \"data\"\nrole = \"data\"")},
		{name: "wrong output direction", raw: outputField("direction", `direction = "input"`)},
		{name: "wrong output kind", raw: outputField("kind", `kind = "gate_feedback"`)},
		{name: "wrong output media", raw: outputField("acceptedMediaTypes", `acceptedMediaTypes = ["application/json"]`)},
		{name: "output carries required", raw: outputFieldsAfterLabel(`required = true`)},
		{name: "output carries role", raw: outputFieldsAfterLabel(`role = "data"`)},
		{name: "duplicate output direction", raw: outputField("direction", "direction = \"output\"\ndirection = \"output\"")},
		{name: "duplicate output kind", raw: outputField("kind", "kind = \"work\"\nkind = \"work\"")},
		{name: "duplicate output media", raw: outputField("acceptedMediaTypes", "acceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]\nacceptedMediaTypes = [\"text/plain\", \"text/markdown\", \"application/json\"]")},
		{name: "duplicate output required", raw: outputFieldsAfterLabel("required = true\nrequired = true")},
		{name: "duplicate output role", raw: outputFieldsAfterLabel("role = \"data\"\nrole = \"data\"")},
		{
			name: "wrong connection channel",
			raw:  replaceToolSchemaMigrationFixture(t, base, `channel = "workflow"`, `channel = "judge"`),
		},
		{
			name: "duplicate connection channel",
			raw:  replaceToolSchemaMigrationFixture(t, base, `channel = "workflow"`, "channel = \"workflow\"\nchannel = \"workflow\""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolSchemaMigrationRejected(t, tt.raw, nil, "")
		})
	}
}

func TestToolSchemaMigrationRejectsUnsafeLegacyShapesBeforeProducingCandidate(t *testing.T) {
	base := toolSchemaMigrationLegacyFixture()
	gateCriterion := `criterion = "Review the work" # criterion comment`
	formationBoundary := `label = "Work result" # output label

[[formation]]
id = "fmn_feedback"`
	withoutJudgeBlocks := func(blocks ...string) string {
		return removeToolSchemaMigrationFixtureBlocks(t, base, blocks...)
	}
	humanOnlyNoJudge := withoutJudgeBlocks(
		toolSchemaMigrationJudgeSendBlock,
		toolSchemaMigrationJudgeMidBlock,
		toolSchemaMigrationJudgeReturnBlock,
	)
	humanOnlyNoJudge = replaceToolSchemaMigrationFixture(t, humanOnlyNoJudge, `kinds = ["human", "formation"]`, `kinds = ["human"]`)

	tests := []struct {
		name      string
		raw       string
		wantError error
		wantCode  string
	}{
		{
			name:      "legacy command presence",
			raw:       replaceToolSchemaMigrationFixture(t, base, gateCriterion, gateCriterion+"\ncommand = \"printf unsafe\""),
			wantError: ErrLegacyScriptGateRequiresFencedMigration,
		},
		{
			name:      "legacy argv presence even empty",
			raw:       replaceToolSchemaMigrationFixture(t, base, gateCriterion, gateCriterion+"\ncommandArgv = []"),
			wantError: ErrLegacyScriptGateRequiresFencedMigration,
		},
		{
			name:      "legacy cwd presence even empty",
			raw:       replaceToolSchemaMigrationFixture(t, base, gateCriterion, gateCriterion+"\ncommandCwd = \"\""),
			wantError: ErrLegacyScriptGateRequiresFencedMigration,
		},
		{
			name:      "legacy shell presence even empty",
			raw:       replaceToolSchemaMigrationFixture(t, base, gateCriterion, gateCriterion+"\ncommandShell = \"\""),
			wantError: ErrLegacyScriptGateRequiresFencedMigration,
		},
		{
			name: "retired inline verification",
			raw: replaceToolSchemaMigrationFixture(t, base, formationBoundary, `label = "Work result" # output label

[formation.verification]
id = "ver_legacy"
kinds = ["human"]
criterion = "Legacy hidden check"
onFail = "pushback"

[[formation]]
id = "fmn_feedback"`),
			wantError: ErrLegacyInlineVerificationRequiresMigration,
		},
		{
			name: "Gate fail into legacy work input",
			raw: base + `
[[connection]]
id = "edge_legacy_fail"
from = "gate_review:fail"
to = "fmn_feedback:port_feedback_in"
`,
			wantCode: "legacy_fail_route_requires_migration",
		},
		{
			name:     "unpaired Gate judge send path missing return",
			raw:      replaceToolSchemaMigrationFixture(t, base, toolSchemaMigrationJudgeReturnBlock, ""),
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name:     "unpaired Gate judge return",
			raw:      withoutJudgeBlocks(toolSchemaMigrationJudgeSendBlock, toolSchemaMigrationJudgeMidBlock),
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name:     "judge chain lacks formation kind",
			raw:      replaceToolSchemaMigrationFixture(t, base, `kinds = ["human", "formation"]`, `kinds = ["human"]`),
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "formation kind lacks complete judge chain",
			raw: withoutJudgeBlocks(
				toolSchemaMigrationJudgeSendBlock,
				toolSchemaMigrationJudgeMidBlock,
				toolSchemaMigrationJudgeReturnBlock,
			),
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "malformed later Gate formation discriminator",
			raw: humanOnlyNoJudge + `
[[gate]]
id = "gate_later_malformed"
title = "Malformed later Gate"
kinds = "formation"
criterion = "Must not borrow the prior Gate discriminator"

[[connection]]
id = "edge_later_judge_send"
from = "gate_later_malformed:judge"
to = "fmn_judge_a:port_judge_a_in"

[[connection]]
id = "edge_later_judge_return"
from = "fmn_judge_a:port_judge_a_out"
to = "gate_later_malformed:judge"
`,
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "multiple Gate judge sends",
			raw: base + `
[[connection]]
id = "edge_judge_second_send"
from = "gate_review:judge"
to = "fmn_feedback:port_feedback_in"
`,
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "multiple Gate judge returns",
			raw: base + `
[[connection]]
id = "edge_judge_second_return"
from = "fmn_feedback:port_feedback_out"
to = "gate_review:judge"
`,
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "non-Formation judge hop",
			raw: withoutJudgeBlocks(toolSchemaMigrationJudgeSendBlock, toolSchemaMigrationJudgeMidBlock, toolSchemaMigrationJudgeReturnBlock) + `
[[gate]]
id = "gate_nonformation_hop"
title = "Not a Formation judge"
kinds = ["human"]
criterion = "Must not enter a judge chain"

[[connection]]
id = "edge_judge_nonformation_send"
from = "gate_review:judge"
to = "gate_nonformation_hop:in"

[[connection]]
id = "edge_judge_nonformation_return"
from = "gate_nonformation_hop:pass"
to = "gate_review:judge"
`,
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name:     "disconnected judge chain",
			raw:      replaceToolSchemaMigrationFixture(t, base, toolSchemaMigrationJudgeMidBlock, ""),
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "cyclic judge chain",
			raw: base + `
[[connection]]
id = "edge_judge_cycle"
from = "fmn_judge_b:port_judge_b_out"
to = "fmn_judge_a:port_judge_a_in"
`,
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "output-side judge endpoint cross-use",
			raw: base + `
[[connection]]
id = "edge_judge_cross_use"
from = "fmn_judge_a:port_judge_a_out"
to = "fmn_feedback:port_feedback_in"
`,
			wantCode: "legacy_judge_channel_requires_migration",
		},
		{
			name: "input-side judge endpoint cross-use",
			raw: base + `
[[connection]]
id = "edge_judge_side_entry"
from = "fmn_feedback:port_feedback_out"
to = "fmn_judge_b:port_judge_b_in"
`,
			wantCode: "legacy_judge_channel_requires_migration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolSchemaMigrationRejected(t, tt.raw, tt.wantError, tt.wantCode)
		})
	}
}

func assertToolSchemaMigrationRejected(t *testing.T, source string, wantError error, wantCode string) {
	t.Helper()
	raw := []byte(source)
	wantSource := append([]byte(nil), raw...)
	candidate, err := migrateBoardToToolSchema(raw)
	if err == nil {
		t.Fatalf("unsafe board produced %d candidate bytes", len(candidate))
	}
	if wantError != nil && !errors.Is(err, wantError) {
		t.Fatalf("migration error = %v, want %v", err, wantError)
	}
	if wantCode != "" && !strings.Contains(err.Error(), wantCode) {
		t.Fatalf("migration error = %v, want stable code %q", err, wantCode)
	}
	if !bytes.Equal(raw, wantSource) {
		t.Fatal("rejected migration mutated its source buffer")
	}
	if candidate != nil && !bytes.Equal(candidate, wantSource) {
		t.Fatalf("rejected migration exposed changed candidate bytes %q", candidate)
	}
}

func assertToolSchemaMigrationPortDefaults(t *testing.T, raw []byte, portID, direction string) {
	t.Helper()
	lines := splitLines(raw)
	var start, end int
	matches := 0
	for i := range lines {
		section, ok := tomlLineSectionName(lines[i])
		if !ok || section != "formation."+direction {
			continue
		}
		blockEnd := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, blockEnd, "id") == portID {
			start, end = i+1, blockEnd
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("Formation port %q blocks = %d, want exactly 1", portID, matches)
	}

	values := toolSchemaMigrationBlockValues(lines, start, end)
	assertToolSchemaMigrationScalar(t, values, "direction", direction)
	assertToolSchemaMigrationScalar(t, values, "kind", "work")
	mediaValues := values["acceptedMediaTypes"]
	if len(mediaValues) != 1 {
		t.Fatalf("Formation port %q acceptedMediaTypes fields = %v, want exactly 1", portID, mediaValues)
	}
	wantMedia := []string{"text/plain", "text/markdown", "application/json"}
	if got := parseStringArray(mediaValues[0]); !equalToolStrings(got, wantMedia) {
		t.Fatalf("Formation port %q media = %v, want %v", portID, got, wantMedia)
	}
	if direction == FormationPortInput {
		assertToolSchemaMigrationScalar(t, values, "required", "true")
		assertToolSchemaMigrationScalar(t, values, "role", "data")
		return
	}
	if len(values["required"]) != 0 || len(values["role"]) != 0 {
		t.Fatalf("Formation output %q gained input-only fields: required=%v role=%v", portID, values["required"], values["role"])
	}
}

func assertToolSchemaMigrationConnectionChannel(t *testing.T, raw []byte, connectionID, wantChannel string) {
	t.Helper()
	lines := splitLines(raw)
	matches := 0
	for i := range lines {
		section, ok := tomlLineSectionName(lines[i])
		if !ok || section != "connection" {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") != connectionID {
			continue
		}
		matches++
		values := toolSchemaMigrationBlockValues(lines, i+1, end)
		assertToolSchemaMigrationScalar(t, values, "channel", wantChannel)
	}
	if matches != 1 {
		t.Fatalf("connection %q blocks = %d, want exactly 1", connectionID, matches)
	}
}

func assertToolSchemaMigrationScalar(t *testing.T, values map[string][]string, key, want string) {
	t.Helper()
	if got := values[key]; len(got) != 1 || got[0] != want {
		t.Fatalf("field %q values = %v, want exactly [%q]", key, got, want)
	}
}

func toolSchemaMigrationBlockValues(lines []tomlLine, start, end int) map[string][]string {
	values := make(map[string][]string)
	for i := start; i < end && i < len(lines); i++ {
		if lines[i].valueContinuation {
			continue
		}
		key, value, ok := tomlKeyValue(lines[i].body)
		if ok {
			values[key] = append(values[key], value)
		}
	}
	return values
}

func toolSchemaMigrationWithoutOwnedFields(raw []byte, sourceSchema string) []byte {
	lines := splitLines(raw)
	filtered := make([]tomlLine, 0, len(lines))
	activeSection := ""
	for _, line := range lines {
		if section, ok := tomlLineSectionName(line); ok {
			activeSection = section
			filtered = append(filtered, line)
			continue
		}
		if isTOMLHeader(line) {
			activeSection = ""
			filtered = append(filtered, line)
			continue
		}
		key, _, ok := tomlKeyValue(line.body)
		if ok && activeSection == "" && key == "schema" {
			line.body = replaceScalarValue(line.body, sourceSchema)
		}
		if ok && (activeSection == "formation.input" || activeSection == "formation.output") {
			switch key {
			case "direction", "kind", "acceptedMediaTypes", "required", "role":
				continue
			}
		}
		if ok && activeSection == "connection" && key == "channel" {
			continue
		}
		filtered = append(filtered, line)
	}
	return renderTOMLLines(filtered)
}

func toolSchemaMigrationReadAtSchemaCeiling(raw []byte, ceiling int) error {
	schema := parseTOMLDocument(raw).intValue("schema")
	if schema > ceiling {
		return fmt.Errorf("%w: schema %d", ErrUnsupportedSchema, schema)
	}
	_, err := parseBoard(raw)
	return err
}

func replaceToolSchemaMigrationFixture(t *testing.T, raw, old, replacement string) string {
	t.Helper()
	if count := strings.Count(raw, old); count != 1 {
		t.Fatalf("migration fixture replacement target %q count = %d, want 1", old, count)
	}
	return strings.Replace(raw, old, replacement, 1)
}

func replaceToolSchemaMigrationPortField(t *testing.T, raw, portID, key, replacement string) string {
	t.Helper()
	lines := splitLines([]byte(raw))
	portMatches := 0
	fieldIndex := -1
	for i := range lines {
		section, ok := tomlLineSectionName(lines[i])
		if !ok || (section != "formation.input" && section != "formation.output") {
			continue
		}
		end := tomlBlockEnd(lines, i)
		if scalarInBlock(lines, i+1, end, "id") != portID {
			continue
		}
		portMatches++
		for j := i + 1; j < end; j++ {
			fieldKey, _, ok := tomlKeyValue(lines[j].body)
			if !ok || fieldKey != key {
				continue
			}
			if fieldIndex >= 0 {
				t.Fatalf("port %q has duplicate source field %q", portID, key)
			}
			fieldIndex = j
		}
	}
	if portMatches != 1 || fieldIndex < 0 {
		t.Fatalf("port %q matches = %d and field %q index = %d, want one existing field", portID, portMatches, key, fieldIndex)
	}

	bodies := strings.Split(replacement, "\n")
	replacementLines := make([]tomlLine, len(bodies))
	for i, body := range bodies {
		newline := "\n"
		if i == len(bodies)-1 {
			newline = lines[fieldIndex].newline
		}
		replacementLines[i] = tomlLine{body: body, newline: newline}
	}
	next := make([]tomlLine, 0, len(lines)-1+len(replacementLines))
	next = append(next, lines[:fieldIndex]...)
	next = append(next, replacementLines...)
	next = append(next, lines[fieldIndex+1:]...)
	return string(renderTOMLLines(next))
}

func removeToolSchemaMigrationFixtureBlocks(t *testing.T, raw string, blocks ...string) string {
	t.Helper()
	for _, block := range blocks {
		raw = replaceToolSchemaMigrationFixture(t, raw, block, "")
	}
	return raw
}

func toolSchemaMigrationLegacyFixture() string {
	return `schema = 1 # compatibility schema
id = "brd_tool_migration" # stable board id
slug = "tool-migration"
title = "Tool migration fixture"
rev = 7
updatedBy = "agent:test"
updatedAt = "2026-07-19T12:00:00Z"
x_owner = "keep" # unknown top-level field

[[mission]]
id = "mis_main"
title = "Main"
goal = "Review the work"
beadId = "ctx-test"
x_mission_note = "keep"

[[formation]]
id = "fmn_work" # stable Formation id
type = "solo"
title = "Worker"

[[formation.input]]
id = "port_work_in" # stable input id
label = "Work input"
x_port_hint = "keep" # unknown safe port field

[[formation.output]]
id = "port_work_out"
label = "Work result" # output label

[[formation]]
id = "fmn_feedback"
type = "solo"
title = "Unwired legacy target"

[[formation.input]]
id = "port_feedback_in"
label = "Feedback candidate"

[[formation.output]]
id = "port_feedback_out"
label = "Feedback result"

[[formation]]
id = "fmn_judge_a"
type = "solo"
title = "Judge A"

[[formation.input]]
id = "port_judge_a_in"
label = "Judge A input"

[[formation.output]]
id = "port_judge_a_out"
label = "Judge A output"

[[formation]]
id = "fmn_judge_b"
type = "solo"
title = "Judge B"

[[formation.input]]
id = "port_judge_b_in"
label = "Judge B input"

[[formation.output]]
id = "port_judge_b_out"
label = "Judge B output"

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human", "formation"]
criterion = "Review the work" # criterion comment

[[connection]]
id = "edge_start"
from = "mis_main:out"
to = "fmn_work:port_work_in"
x_route_note = "keep" # unknown safe connection field

[[connection]]
id = "edge_review"
from = "fmn_work:port_work_out"
to = "gate_review:in"

[[connection]]
id = "edge_judge_send"
from = "gate_review:judge"
to = "fmn_judge_a:port_judge_a_in"

[[connection]]
id = "edge_judge_mid"
from = "fmn_judge_a:port_judge_a_out"
to = "fmn_judge_b:port_judge_b_in"

[[connection]]
id = "edge_judge_return"
from = "fmn_judge_b:port_judge_b_out"
to = "gate_review:judge"

[x_extension]
note = "keep this table byte-for-byte"
`
}

func toolSchemaMigrationCanonicalFixture() string {
	return `schema = 2 # canonical schema
id = "brd_tool_schema_two"
slug = "tool-schema-two"
title = "Canonical schema two"
rev = 9
x_owner = "keep" # canonical extension

[[mission]]
id = "mis_main"
title = "Main"
goal = "Keep canonical bytes"
beadId = "ctx-test"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Worker"

[[formation.input]]
id = "port_work_in"
label = "Work input"
direction = "input"
kind = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]
required = true
role = "data"
x_port_hint = "keep"

[[formation.output]]
id = "port_work_out"
label = "Work output"
direction = "output"
kind = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]

[[connection]]
id = "edge_start"
channel = "workflow"
from = "mis_main:out"
to = "fmn_work:port_work_in"

[x_extension]
note = "already canonical"
`
}
