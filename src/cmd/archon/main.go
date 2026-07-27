package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
	"github.com/chrote/server/internal/formations"
)

type tmuxRunner interface {
	LiveSessions() ([]formations.LiveAgentSession, error)
	Spawn(name, command string) error
	Attach(name string) error
}

type realTmuxRunner struct{}

type archonConfig struct {
	Workspace string
}

type archonBoardIdentity struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Rev   int    `json:"rev"`
	ETag  string `json:"etag"`
}

type archonMissionListResponse struct {
	Board    archonBoardIdentity      `json:"board"`
	Missions []formations.MissionNode `json:"missions"`
}

type archonMissionInspectResponse struct {
	Board       archonBoardIdentity          `json:"board"`
	Mission     formations.MissionNode       `json:"mission"`
	Chain       []archonMissionChainNode     `json:"chain"`
	Connections []formations.BoardConnection `json:"connections"`
}

type archonMissionChainNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Type  string `json:"type,omitempty"`
	Depth int    `json:"depth"`
}

type archonErrorResponse struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Boundary string `json:"boundary"`
	Selector string `json:"selector"`
}

type archonRunAskResponse struct {
	RunID           string                          `json:"runId"`
	Question        string                          `json:"question,omitempty"`
	Status          *formations.RunStatusProjection `json:"status"`
	Answer          string                          `json:"answer"`
	CompletedNodes  []archonRunCompletedNode        `json:"completedNodes"`
	ProducedOutputs []archonRunProducedOutput       `json:"producedOutputs"`
	OpenEscalations []formations.OpenEscalation     `json:"openEscalations"`
	WaitingGates    []archonRunWaitingGate          `json:"waitingGates"`
	BlockedReasons  []archonRunBlockedReason        `json:"blockedReasons"`
	EvidenceSeqs    []int                           `json:"evidenceSeqs"`
	MissingEvidence []string                        `json:"missingEvidence,omitempty"`
}

type archonRunCompletedNode struct {
	NodeID    string `json:"nodeId"`
	Status    string `json:"status"`
	OutputSeq int    `json:"outputSeq"`
	ReportRef string `json:"reportRef,omitempty"`
	Text      string `json:"text,omitempty"`
}

type archonRunProducedOutput struct {
	NodeID    string `json:"nodeId"`
	OutputSeq int    `json:"outputSeq"`
	ReportRef string `json:"reportRef,omitempty"`
	Text      string `json:"text,omitempty"`
}

type archonRunWaitingGate struct {
	GateID       string   `json:"gateId"`
	NodeID       string   `json:"nodeId"`
	Prompt       string   `json:"prompt,omitempty"`
	Choices      []string `json:"choices,omitempty"`
	RequestedSeq int      `json:"requestedSeq"`
}

type archonRunBlockedReason struct {
	Seq           int    `json:"seq"`
	NodeID        string `json:"nodeId,omitempty"`
	GateID        string `json:"gateId,omitempty"`
	Reason        string `json:"reason"`
	Code          string `json:"code,omitempty"`
	Boundary      string `json:"boundary,omitempty"`
	ResumeAllowed bool   `json:"resumeAllowed"`
}

type archonStreamError struct {
	Type  string              `json:"type"`
	Error archonErrorResponse `json:"error"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, realTmuxRunner{}))
}

func run(args []string, stdout, stderr io.Writer, runner tmuxRunner) int {
	return runWithRuntimeStoreFactory(args, stdout, stderr, runner, func(workspace string) *formations.Store {
		return formations.NewRuntimeStore(workspace, "")
	})
}

func runWithRuntimeStoreFactory(args []string, stdout, stderr io.Writer, runner tmuxRunner, runtimeStore func(string) *formations.Store) int {
	config, args, ok := parseGlobalArgs(args, stderr)
	if !ok {
		return 2
	}
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: archon <agent|board|formation|gate|mission|tool|run> <command>")
		return 2
	}
	switch args[0] {
	case "agent":
		store := formations.NewPersonaStore(formations.DefaultAgentsDir())
		switch args[1] {
		case "list":
			return runAgentList(store, args[2:], stdout, stderr, runner)
		case "inspect":
			return runAgentInspect(store, args[2:], stdout, stderr)
		case "new":
			return runAgentNew(store, args[2:], stdout, stderr)
		case "edit":
			return runAgentEdit(store, args[2:], stdout, stderr)
		case "spawn":
			return runAgentSpawn(store, args[2:], stdout, stderr, runner)
		case "attach":
			return runAgentAttach(store, args[2:], stdout, stderr, runner)
		case "retire":
			return runAgentRetire(store, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown agent command %q\n", args[1])
			return 2
		}
	case "board":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "new":
			return runBoardNew(store, args[2:], stdout, stderr)
		case "list":
			return runBoardList(store, args[2:], stdout, stderr)
		case "inspect":
			return runBoardInspect(store, args[2:], stdout, stderr)
		case "validate":
			return runBoardValidate(store, args[2:], stdout, stderr)
		case "arrange":
			return runBoardArrange(store, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown board command %q\n", args[1])
			return 2
		}
	case "formation":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "create":
			return runFormationCreate(store, args[2:], stdout, stderr)
		case "list":
			return runFormationList(store, args[2:], stdout, stderr)
		case "inspect":
			return runFormationInspect(store, args[2:], stdout, stderr)
		case "assign":
			return runFormationAssign(store, args[2:], stdout, stderr)
		case "unassign":
			return runFormationUnassign(store, args[2:], stdout, stderr)
		case "set-brief":
			return runFormationSetBrief(store, args[2:], stdout, stderr)
		case "remove-verification":
			return runFormationRemoveVerification(store, args[2:], stdout, stderr)
		case "add-input":
			return runFormationAddPort(store, args[2:], stdout, stderr, formations.FormationPortInput)
		case "add-output":
			return runFormationAddPort(store, args[2:], stdout, stderr, formations.FormationPortOutput)
		case "wire":
			return runFormationWire(store, args[2:], stdout, stderr, false)
		case "unwire":
			return runFormationWire(store, args[2:], stdout, stderr, true)
		case "run":
			return runFormationRun(runtimeStore(config.Workspace), args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown formation command %q\n", args[1])
			return 2
		}
	case "gate":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "create":
			return runGateCreate(store, args[2:], stdout, stderr)
		case "update":
			return runGateUpdate(store, args[2:], stdout, stderr)
		case "judge":
			return runGateJudge(store, args[2:], stdout, stderr)
		case "approve":
			return runGateVerdict(runtimeStore(config.Workspace), args[2:], stdout, stderr, "pass")
		case "reject":
			return runGateVerdict(runtimeStore(config.Workspace), args[2:], stdout, stderr, "fail")
		default:
			fmt.Fprintf(stderr, "unknown gate command %q\n", args[1])
			return 2
		}
	case "mission":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "create":
			return runMissionCreate(store, args[2:], stdout, stderr)
		case "list":
			return runMissionList(store, args[2:], stdout, stderr)
		case "inspect":
			return runMissionInspect(store, args[2:], stdout, stderr)
		case "wire":
			return runMissionWire(store, args[2:], stdout, stderr)
		case "run":
			return runMissionRun(runtimeStore(config.Workspace), args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown mission command %q\n", args[1])
			return 2
		}
	case "tool":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "create":
			return runToolCreate(store, args[2:], stdout, stderr)
		case "update":
			return runToolUpdate(store, args[2:], stdout, stderr)
		case "delete":
			return runToolDelete(store, args[2:], stdout, stderr)
		case "inspect":
			return runToolInspect(store, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown tool command %q\n", args[1])
			return 2
		}
	case "run":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "list":
			return runList(store, args[2:], stdout, stderr)
		case "status":
			return runStatus(store, args[2:], stdout, stderr)
		case "logs":
			return runLogs(store, args[2:], stdout, stderr)
		case "follow":
			return runFollow(store, args[2:], stdout, stderr)
		case "resume":
			return runResume(runtimeStore(config.Workspace), args[2:], stdout, stderr)
		case "abort":
			return runAbort(runtimeStore(config.Workspace), args[2:], stdout, stderr)
		case "ask":
			return runAsk(store, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown run command %q\n", args[1])
			return 2
		}
	default:
		fmt.Fprintf(stderr, "unknown archon noun %q\n", args[0])
		return 2
	}
}

func newArchonRunEngine(store *formations.Store, personas *formations.PersonaStore, boundary string) *formations.RunEngine {
	engine := formations.NewRunEngine(store, personas, formations.NewConfiguredFormationExecutorFromEnv(store, personas, boundary))
	engine.SetGateEvaluator(formations.NewCodeGateEvaluator())
	return engine
}

func parseGlobalArgs(args []string, stderr io.Writer) (archonConfig, []string, bool) {
	config := archonConfig{Workspace: core.GetWorkDir()}
	for len(args) > 0 {
		switch args[0] {
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(stderr, "--workspace requires a path")
				return config, args, false
			}
			config.Workspace = args[1]
			args = args[2:]
		default:
			return config, args, true
		}
	}
	return config, args, true
}

func runAgentList(store *formations.PersonaStore, args []string, stdout, stderr io.Writer, runner tmuxRunner) int {
	fs := flag.NewFlagSet("agent list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	capable := fs.String("capable", "", "filter by bare capability")
	assignable := fs.Bool("assignable", false, "show assignable agents only")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true, "assignable": true})); err != nil {
		return 2
	}
	cards, err := store.ListPersonas()
	if err != nil {
		return fail(stderr, err)
	}
	live, err := liveFromRunner(runner)
	if err != nil {
		return fail(stderr, err)
	}
	roster, err := formations.ProjectAgentRoster(cards, live, formations.AgentRosterFilter{
		Capable:        *capable,
		AssignableOnly: *assignable,
	})
	if err != nil {
		return fail(stderr, err)
	}
	archonExposeTmuxTargetSessions(&roster)
	if *jsonOut {
		return writeJSON(stdout, roster)
	}
	for _, agent := range roster.Agents {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", agent.ID, agent.Kind, agent.Liveness, strings.Join(agent.Tags, ","))
	}
	return 0
}

func runAgentInspect(store *formations.PersonaStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent inspect <id> [--json]")
		return 2
	}
	card, err := store.ReadPersona(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	card.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, card)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\n", card.ID, card.Kind, strings.Join(card.Tags, ","))
	return 0
}

func runAgentNew(store *formations.PersonaStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kind := fs.String("kind", "", "agent kind")
	harness := fs.String("harness", "", "default harness")
	capable := fs.String("capable", "", "comma-separated bare capabilities")
	personality := fs.String("personality", "", "personality facet")
	from := fs.String("from", "", "source config path")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent new <id> --kind <kind> [--harness <h>] [--from <path>]")
		return 2
	}
	card, err := store.CreatePersona(formations.CreatePersonaRequest{
		ID:           fs.Arg(0),
		Kind:         *kind,
		Harness:      *harness,
		Capabilities: splitCSV(*capable),
		Personality:  *personality,
		Source:       *from,
	})
	if err != nil {
		return fail(stderr, err)
	}
	card.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, card)
	}
	fmt.Fprintf(stdout, "created %s\n", card.ID)
	return 0
}

func runAgentEdit(store *formations.PersonaStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent edit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addCapability := fs.String("add-capability", "", "add bare capability")
	removeCapability := fs.String("remove-capability", "", "remove bare capability")
	addHarness := fs.String("add-harness", "", "add harness variant")
	sessionStem := fs.String("session-stem", "", "session stem for added harness")
	note := fs.String("note", "", "append note")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent edit <id> [--add-capability t|--remove-capability t|--add-harness h --session-stem s|--note text]")
		return 2
	}
	before, err := store.ReadPersona(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	card, err := store.EditPersona(fs.Arg(0), formations.EditPersonaRequest{
		AddCapability:    *addCapability,
		RemoveCapability: *removeCapability,
		AddHarness:       *addHarness,
		SessionStem:      *sessionStem,
		Note:             *note,
		ExpectedETag:     before.ETag,
	})
	if err != nil {
		return fail(stderr, err)
	}
	card.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, card)
	}
	fmt.Fprintf(stdout, "updated %s\n", card.ID)
	return 0
}

func runAgentSpawn(store *formations.PersonaStore, args []string, stdout, stderr io.Writer, runner tmuxRunner) int {
	fs := flag.NewFlagSet("agent spawn", flag.ContinueOnError)
	fs.SetOutput(stderr)
	harness := fs.String("harness", "", "harness variant")
	if err := fs.Parse(reorderFlags(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent spawn <id> [--harness <h>]")
		return 2
	}
	card, err := store.ReadPersona(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	variant, err := card.SelectHarnessVariant(*harness)
	if err != nil {
		return fail(stderr, err)
	}
	live, err := liveForCard(*card, runner)
	if err != nil {
		return fail(stderr, err)
	}
	if binding, err := formations.ResolveAgentSession(*card, live, *harness); err == nil {
		fmt.Fprintf(stdout, "%s already live as %s\n", card.ID, archonTmuxTargetSessionName(binding.SessionStem))
		return 0
	} else if !errors.Is(err, formations.ErrAgentSessionOffline) {
		return fail(stderr, err)
	}
	if variant.SessionStem == "" {
		return fail(stderr, fmt.Errorf("%w: agent %q harness %q has no session_stem", formations.ErrAgentSessionOffline, card.ID, variant.ID))
	}
	targetSession := archonTmuxTargetSessionName(variant.SessionStem)
	if err := runner.Spawn(targetSession, variant.Launch); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "spawned %s as %s\n", card.ID, targetSession)
	return 0
}

func runAgentAttach(store *formations.PersonaStore, args []string, stdout, stderr io.Writer, runner tmuxRunner) int {
	fs := flag.NewFlagSet("agent attach", flag.ContinueOnError)
	fs.SetOutput(stderr)
	harness := fs.String("harness", "", "harness variant")
	if err := fs.Parse(reorderFlags(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent attach <id> [--harness <h>]")
		return 2
	}
	card, err := store.ReadPersona(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	live, err := liveForCard(*card, runner)
	if err != nil {
		return fail(stderr, err)
	}
	binding, err := formations.ResolveAgentSession(*card, live, *harness)
	if err != nil {
		return fail(stderr, err)
	}
	if err := runner.Attach(archonTmuxTargetSessionName(binding.SessionStem)); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func runAgentRetire(store *formations.PersonaStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent retire", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "retire even if future reference scans warn")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"force": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent retire <id> [--force]")
		return 2
	}
	if !*force {
		fmt.Fprintln(stderr, "warning: S1 cannot scan future formation references yet; use --force to retire")
		return 1
	}
	before, err := store.ReadPersona(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	card, err := store.EditPersona(fs.Arg(0), formations.EditPersonaRequest{Retire: true, ExpectedETag: before.ETag})
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "retired %s\n", card.ID)
	return 0
}

func runFormationCreate(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "formation title")
	x := fs.Int("x", 120, "layout x")
	y := fs.Int("y", 120, "layout y")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon formation create <board> <type> --title <title> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	createX, createY, err := resolveCreateCoordinates(store, slug, fs, *x, *y)
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result, err := store.CreateFormation(slug, formations.FormationCreateRequest{
		Type:      fs.Arg(1),
		Title:     *title,
		X:         createX,
		Y:         createY,
		UpdatedBy: *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result.Board.TOML = ""
	result.Layout.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "created %s\n", result.Formation.ID)
	return 0
}

func resolveCreateCoordinates(store *formations.Store, slug string, fs *flag.FlagSet, x, y int) (int, int, error) {
	explicit := false
	fs.Visit(func(current *flag.Flag) {
		if current.Name == "x" || current.Name == "y" {
			explicit = true
		}
	})
	if explicit {
		return x, y, nil
	}
	position, err := store.FindFreeLayoutPosition(slug, x, y)
	if err != nil {
		return 0, 0, err
	}
	return position.X, position.Y, nil
}

func runFormationAssign(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation assign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	slotID := fs.String("slot", "", "slot id")
	agentID := fs.String("agent", "", "persona id")
	harness := fs.String("harness", "", "harness variant")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 || *slotID == "" || *agentID == "" {
		fmt.Fprintln(stderr, "usage: archon formation assign <board> <formation> --slot <slot> --agent <agent> [--harness <h>] [--json]")
		return 2
	}
	slug, board, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result, err := store.AssignFormationSlot(slug, formations.FormationSlotAssignmentRequest{
		FormationID: formationID,
		SlotID:      *slotID,
		AgentID:     *agentID,
		Harness:     *harness,
		UpdatedBy:   *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "assigned %s to %s\n", *agentID, *slotID)
	return 0
}

func runFormationUnassign(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation unassign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	slotID := fs.String("slot", "", "slot id")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 || *slotID == "" {
		fmt.Fprintln(stderr, "usage: archon formation unassign <board> <formation> --slot <slot> [--json]")
		return 2
	}
	slug, board, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result, err := store.AssignFormationSlot(slug, formations.FormationSlotAssignmentRequest{
		FormationID: formationID,
		SlotID:      *slotID,
		UpdatedBy:   *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "unassigned %s from %s\n", *slotID, formationID)
	return 0
}

func runFormationSetBrief(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation set-brief", flag.ContinueOnError)
	fs.SetOutput(stderr)
	goal := fs.String("goal", "", "brief goal")
	beadID := fs.String("bead", "", "project Beads id")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	var files stringList
	var links stringList
	fs.Var(&files, "file", "file reference")
	fs.Var(&links, "link", "link reference")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon formation set-brief <board> <formation> --goal <goal> [--bead <beads-id>] [--file <path>] [--link <url>] [--json]")
		return 2
	}
	slug, board, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result, err := store.SetFormationBrief(slug, formations.FormationBriefRequest{
		FormationID: formationID,
		Goal:        *goal,
		BeadID:      *beadID,
		Files:       files,
		Links:       links,
		UpdatedBy:   *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "updated brief for %s\n", formationID)
	return 0
}

func runFormationRemoveVerification(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation remove-verification", flag.ContinueOnError)
	fs.SetOutput(stderr)
	replacementGate := fs.String("replacement-gate", "", "explicit Gate already wired from the Formation")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon formation remove-verification <board> <formation> --replacement-gate <gate> [--json]")
		return 2
	}
	slug, board, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result, err := store.RemoveFormationVerification(slug, formations.FormationVerificationRemovalRequest{
		FormationID:       formationID,
		ReplacementGateID: *replacementGate,
		UpdatedBy:         *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "formation", formationID)
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "removed legacy inline verification from %s\n", formationID)
	return 0
}

func runFormationAddPort(store *formations.Store, args []string, stdout, stderr io.Writer, direction string) int {
	name := "formation add-input"
	if direction == formations.FormationPortOutput {
		name = "formation add-output"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	label := fs.String("label", "", "port label")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintf(stderr, "usage: archon %s <board> <formation> --label <label> [--json]\n", name)
		return 2
	}
	slug, board, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result, err := store.AddFormationPort(slug, formations.FormationPortRequest{
		FormationID: formationID,
		Direction:   direction,
		Label:       *label,
		UpdatedBy:   *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "added %s to %s\n", direction, formationID)
	return 0
}

func runFormationWire(store *formations.Store, args []string, stdout, stderr io.Writer, remove bool) int {
	name := "formation wire"
	if remove {
		name = "formation unwire"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 3 {
		fmt.Fprintf(stderr, "usage: archon %s <board> <from-node:port> <to-node:port> [--json]\n", name)
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	request := formations.FormationWireRequest{
		From:      fs.Arg(1),
		To:        fs.Arg(2),
		UpdatedBy: *updatedBy,
	}
	var result *formations.BoardDocument
	if remove {
		result, err = store.UnwireFormationPorts(slug, request, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	} else {
		result, err = store.WireFormationPorts(slug, request, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	}
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	if remove {
		fmt.Fprintf(stdout, "removed connection %s -> %s\n", request.From, request.To)
	} else {
		fmt.Fprintf(stdout, "wired %s -> %s\n", request.From, request.To)
	}
	return 0
}

func runGateCreate(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gate create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "Review gate", "gate title")
	kinds := fs.String("kinds", "code", "comma-separated gate kinds")
	criterion := fs.String("criterion", "", "gate criterion")
	check := fs.String("check", "", "registered code Gate profile id")
	checkVersion := fs.String("check-version", "", "exact code Gate profile version")
	checkValue := fs.String("check-value", "", "code Gate profile value parameter")
	command := fs.String("command", "", "retired legacy Gate field; new writes fail with a migration error")
	commandArgv := fs.String("command-argv", "", "retired legacy Gate argv; new writes fail with a migration error")
	commandCWD := fs.String("command-cwd", "", "retired legacy Gate cwd; new writes fail with a migration error")
	commandShell := fs.String("command-shell", "", "retired legacy Gate shell command; new writes fail with a migration error")
	x := fs.Int("x", 0, "layout x coordinate")
	y := fs.Int("y", 0, "layout y coordinate")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon gate create <board> [--kinds code,human] [--criterion text] [--check id --check-version version --check-value value] [--x n] [--y n] [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	createX, createY, err := resolveCreateCoordinates(store, slug, fs, *x, *y)
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result, err := store.CreateGate(slug, formations.GateCreateRequest{
		Title:                      *title,
		Kinds:                      splitCSV(*kinds),
		Criterion:                  *criterion,
		Check:                      *check,
		CheckVersion:               *checkVersion,
		CheckValue:                 *checkValue,
		Command:                    *command,
		CommandArgv:                splitCSV(*commandArgv),
		CommandCWD:                 *commandCWD,
		CommandShell:               *commandShell,
		LegacyCommandFieldsPresent: legacyGateCommandFlagPresent(fs),
		X:                          createX,
		Y:                          createY,
		UpdatedBy:                  *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result.Board.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result.Board)
	}
	fmt.Fprintln(stdout, "created gate")
	return 0
}

func runGateUpdate(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gate update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "gate title")
	kinds := fs.String("kinds", "", "comma-separated gate kinds")
	criterion := fs.String("criterion", "", "gate criterion")
	check := fs.String("check", "", "registered code Gate profile id")
	checkVersion := fs.String("check-version", "", "exact code Gate profile version")
	checkValue := fs.String("check-value", "", "code Gate profile value parameter")
	command := fs.String("command", "", "retired legacy Gate field; new writes fail with a migration error")
	commandArgv := fs.String("command-argv", "", "retired legacy Gate argv; new writes fail with a migration error")
	commandCWD := fs.String("command-cwd", "", "retired legacy Gate cwd; new writes fail with a migration error")
	commandShell := fs.String("command-shell", "", "retired legacy Gate shell command; new writes fail with a migration error")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon gate update <board> <gate> [--title text] [--kinds code,human] [--criterion text] [--check id --check-version version --check-value value] [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	gateID, err := resolveGateSelector(board, fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "gate", fs.Arg(1))
	}
	var updateKinds []string
	if strings.TrimSpace(*kinds) != "" {
		updateKinds = splitCSV(*kinds)
	}
	result, err := store.UpdateGate(slug, formations.GateUpdateRequest{
		GateID:                     gateID,
		Title:                      *title,
		Kinds:                      updateKinds,
		Criterion:                  *criterion,
		Check:                      *check,
		CheckVersion:               *checkVersion,
		CheckValue:                 *checkValue,
		Command:                    *command,
		CommandArgv:                splitCSV(*commandArgv),
		CommandCWD:                 *commandCWD,
		CommandShell:               *commandShell,
		LegacyCommandFieldsPresent: legacyGateCommandFlagPresent(fs),
		UpdatedBy:                  *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintln(stdout, "updated gate")
	return 0
}

func legacyGateCommandFlagPresent(fs *flag.FlagSet) bool {
	present := false
	fs.Visit(func(current *flag.Flag) {
		switch current.Name {
		case "command", "command-argv", "command-cwd", "command-shell":
			present = true
		}
	})
	return present
}

func runGateJudge(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gate judge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	chain := fs.String("chain", "", "comma-separated formation chain")
	detach := fs.Bool("detach", false, "detach judge")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true, "detach": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 || (!*detach && *chain == "") {
		fmt.Fprintln(stderr, "usage: archon gate judge <board> <gate> --chain f1,f2 | --detach [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	gateID, err := resolveGateSelector(board, fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "gate", fs.Arg(1))
	}
	request := formations.GateJudgeRequest{
		GateID:    gateID,
		Chain:     splitCSV(*chain),
		UpdatedBy: *updatedBy,
	}
	var result *formations.BoardDocument
	if *detach {
		result, err = store.DetachGateJudge(slug, request, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	} else {
		result, err = store.SetGateJudgeChain(slug, request, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	}
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	if *detach {
		fmt.Fprintf(stdout, "detached judge from %s\n", gateID)
	} else {
		fmt.Fprintf(stdout, "updated judge for %s\n", gateID)
	}
	return 0
}

func runGateVerdict(store *formations.Store, args []string, stdout, stderr io.Writer, verdict string) int {
	fs := flag.NewFlagSet("gate verdict", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reason := fs.String("reason", "", "verdict reason")
	actor := fs.String("actor", "human:operator", "deciding actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon gate approve|reject <runId> <gateId> [--reason text] [--json]")
		return 2
	}
	if err := store.RequireRuntimeAuthority(); err != nil {
		return failJSON(stderr, err, *jsonOut, "run", fs.Arg(0))
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := newArchonRunEngine(store, personas, "archon")
	status, err := engine.RecordHumanGateVerdict(fs.Arg(0), formations.HumanGateVerdictRequest{
		GateID:  fs.Arg(1),
		Verdict: verdict,
		Reason:  *reason,
		Actor:   *actor,
	})
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", fs.Arg(0))
	}
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "%s\t%s\n", status.RunID, status.Status)
	return 0
}

func runMissionCreate(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mission create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "mission title")
	goal := fs.String("goal", "", "mission goal")
	beadID := fs.String("bead", "", "project Beads id")
	x := fs.Int("x", 0, "layout x coordinate")
	y := fs.Int("y", 0, "layout y coordinate")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon mission create <board> --title <title> --goal <goal> --bead <beads-id> [--x n] [--y n] [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	createX, createY, err := resolveCreateCoordinates(store, slug, fs, *x, *y)
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result, err := store.CreateMission(slug, formations.MissionCreateRequest{
		Title:     *title,
		Goal:      *goal,
		BeadID:    *beadID,
		X:         createX,
		Y:         createY,
		UpdatedBy: *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result.Board.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result.Board)
	}
	fmt.Fprintln(stdout, "created mission")
	return 0
}

func runMissionList(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mission list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon mission list <board> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	response := archonMissionListResponse{
		Board:    identityFromBoard(board),
		Missions: board.Missions,
	}
	if *jsonOut {
		return writeJSON(stdout, response)
	}
	for _, mission := range response.Missions {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", mission.ID, mission.Title, mission.Goal, mission.BeadID)
	}
	return 0
}

func runMissionInspect(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mission inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon mission inspect <board> <mission> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	missionID, err := resolveMissionSelector(board, fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "mission", fs.Arg(1))
	}
	mission, ok := missionByID(board, missionID)
	if !ok {
		return failSelector(stderr, fmt.Errorf("%w: mission %q", formations.ErrNotFound, missionID), *jsonOut, "mission", missionID)
	}
	chain, connections, err := missionReachableChain(board, missionID)
	if err != nil {
		return fail(stderr, err)
	}
	response := archonMissionInspectResponse{
		Board:       identityFromBoard(board),
		Mission:     mission,
		Chain:       chain,
		Connections: connections,
	}
	if *jsonOut {
		return writeJSON(stdout, response)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%d reachable nodes\n", mission.ID, mission.Title, len(chain))
	return 0
}

func runMissionWire(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mission wire", flag.ContinueOnError)
	fs.SetOutput(stderr)
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 3 {
		fmt.Fprintln(stderr, "usage: archon mission wire <board> <mission> <to-node:port> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	missionID, err := resolveMissionSelector(board, fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "mission", fs.Arg(1))
	}
	result, err := store.WireFormationPorts(slug, formations.FormationWireRequest{
		From:      missionID + ":out",
		To:        fs.Arg(2),
		UpdatedBy: *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "wired mission %s -> %s\n", missionID, fs.Arg(2))
	return 0
}

func runMissionRun(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mission run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	missionSelector := fs.String("mission", "", "mission id or title")
	actor := fs.String("actor", "agent:archon", "run actor")
	maxDispatch := fs.Int("max-dispatch", 0, "maximum dispatches")
	maxAttempts := fs.Int("max-attempts", 0, "maximum attempts per formation")
	wallClockSeconds := fs.Int("wall-clock-seconds", 0, "wall clock limit in seconds")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon mission run <board> [--mission <mission>] [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	missionID := *missionSelector
	if missionID == "" {
		switch len(board.Missions) {
		case 0:
			return failJSON(stderr, fmt.Errorf("%w: board %q has no mission", formations.ErrNotFound, slug), *jsonOut, "mission", "")
		case 1:
			missionID = board.Missions[0].ID
		default:
			return failJSON(stderr, fmt.Errorf("%w: board %q has multiple missions; pass --mission", formations.ErrConflict, slug), *jsonOut, "mission", "")
		}
	} else if resolved, err := resolveMissionSelector(board, missionID); err != nil {
		return failSelector(stderr, err, *jsonOut, "mission", missionID)
	} else {
		missionID = resolved
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := newArchonRunEngine(store, personas, "archon")
	status, err := engine.RunMission(slug, formations.RunStartRequest{
		MissionID:         missionID,
		Actor:             *actor,
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits: formations.RunLimits{
			MaxDispatch:      *maxDispatch,
			MaxAttempts:      *maxAttempts,
			WallClockSeconds: *wallClockSeconds,
		},
	})
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", missionID)
	}
	return writeRunCommandResponse(stdout, status, *jsonOut)
}

func runFormationRun(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	actor := fs.String("actor", "agent:archon", "run actor")
	maxDispatch := fs.Int("max-dispatch", 0, "maximum dispatches")
	maxAttempts := fs.Int("max-attempts", 0, "maximum attempts per formation")
	wallClockSeconds := fs.Int("wall-clock-seconds", 0, "wall clock limit in seconds")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon formation run <board> <formation> [--json]")
		return 2
	}
	slug, _, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := newArchonRunEngine(store, personas, "archon")
	status, err := engine.RunFormation(slug, formationID, formations.FormationRunRequest{
		Actor:    *actor,
		Personas: personas,
		Limits: formations.RunLimits{
			MaxDispatch:      *maxDispatch,
			MaxAttempts:      *maxAttempts,
			WallClockSeconds: *wallClockSeconds,
		},
	})
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", formationID)
	}
	return writeRunCommandResponse(stdout, status, *jsonOut)
}

func runList(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	boardSelector := fs.String("board", "", "board selector filter")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: archon run list [--board <board>] [--json]")
		return 2
	}
	boardSlug := ""
	if *boardSelector != "" {
		resolved, err := store.ResolveBoardSelector(*boardSelector)
		if err != nil {
			return failSelector(stderr, err, *jsonOut, "board", *boardSelector)
		}
		boardSlug = resolved
	}
	runs, err := store.ListRuns(formations.RunListFilter{BoardSlug: boardSlug})
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", "")
	}
	if *jsonOut {
		return writeJSON(stdout, map[string]any{"runs": runs})
	}
	for _, run := range runs {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%d events\n", run.RunID, run.Status, run.BoardSlug, run.EventCount)
	}
	return 0
}

func runStatus(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon run status <runId> [--json]")
		return 2
	}
	status, err := store.ProjectRun(fs.Arg(0))
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", fs.Arg(0))
	}
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%d events\n", status.RunID, status.Status, status.BoardSlug, status.EventCount)
	return 0
}

func runLogs(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run logs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	nodeID := fs.String("node", "", "node id filter")
	follow := fs.Bool("follow", false, "follow the ledger")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true, "follow": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon run logs <runId> [--node <id>] [--follow] [--json]")
		return 2
	}
	runID := fs.Arg(0)
	events, err := store.ReadRunEvents(runID)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", runID)
	}
	filtered := filterRunEvents(events, *nodeID)
	if *follow && *jsonOut {
		for {
			status, err := store.ProjectRun(runID)
			if err != nil {
				return failJSON(stderr, err, *jsonOut, "run", runID)
			}
			if status.Final || isBlockedResumable(status) {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		events, err = store.ReadRunEvents(runID)
		if err != nil {
			return failJSON(stderr, err, *jsonOut, "run", runID)
		}
		filtered = filterRunEvents(events, *nodeID)
	}
	if *jsonOut {
		return writeJSON(stdout, filtered)
	}
	writeRunEventsText(stdout, filtered)
	if *follow {
		lastSeq := lastRunSeq(events)
		for {
			time.Sleep(250 * time.Millisecond)
			nextEvents, err := store.ReadRunEvents(runID)
			if err != nil {
				return failJSON(stderr, err, *jsonOut, "run", runID)
			}
			var newEvents []formations.RunEvent
			for _, event := range nextEvents {
				if event.Seq > lastSeq {
					newEvents = append(newEvents, event)
				}
			}
			writeRunEventsText(stdout, filterRunEvents(newEvents, *nodeID))
			lastSeq = lastRunSeq(nextEvents)
			status, err := store.ProjectRun(runID)
			if err != nil {
				return failJSON(stderr, err, *jsonOut, "run", runID)
			}
			if status.Final {
				return 0
			}
		}
	}
	return 0
}

func runFollow(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run follow", flag.ContinueOnError)
	fs.SetOutput(stderr)
	nodeID := fs.String("node", "", "node id filter")
	since := fs.Int("since", 0, "only emit events with seq greater than this value")
	jsonOut := fs.Bool("json", false, "write NDJSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon run follow <runId> [--since <seq>] [--node <id>] [--json]")
		return 2
	}
	if *since < 0 {
		return failJSON(stderr, fmt.Errorf("%w: --since must be non-negative", formations.ErrInvalidSlug), *jsonOut, "run", fs.Arg(0))
	}
	runID := fs.Arg(0)
	lastSeq := *since
	for {
		events, err := store.ReadRunEvents(runID)
		if err != nil {
			return failRunStreamError(stdout, stderr, err, *jsonOut, "run", runID)
		}
		for _, event := range events {
			if event.Seq <= lastSeq || !runEventReferencesNode(event, *nodeID) {
				continue
			}
			if *jsonOut {
				if err := writeNDJSON(stdout, event); err != nil {
					return 1
				}
			} else {
				writeRunEventsText(stdout, []formations.RunEvent{event})
			}
		}
		if ledgerLast := lastRunSeq(events); ledgerLast > lastSeq {
			lastSeq = ledgerLast
		}
		status, err := store.ProjectRun(runID)
		if err != nil {
			return failRunStreamError(stdout, stderr, err, *jsonOut, "run", runID)
		}
		if status.Final {
			return 0
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func isBlockedResumable(status *formations.RunStatusProjection) bool {
	return status != nil && status.Status == formations.RunStatusBlocked && status.ResumeAllowed
}

func runResume(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run resume", flag.ContinueOnError)
	fs.SetOutput(stderr)
	actor := fs.String("actor", "agent:archon", "resume actor")
	mode := fs.String("mode", "reattach", "resume mode")
	reason := fs.String("reason", "", "resume reason")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon run resume <runId> [--reason text] [--json]")
		return 2
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := newArchonRunEngine(store, personas, "archon")
	status, err := engine.ResumeRun(fs.Arg(0), formations.RunResumeRequest{
		Actor:  *actor,
		Mode:   *mode,
		Reason: *reason,
	})
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", fs.Arg(0))
	}
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%d\n", status.RunID, status.Status, status.Epoch)
	return 0
}

func runAbort(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run abort", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reason := fs.String("reason", "operator abort", "abort reason")
	requestedBy := fs.String("requested-by", "agent:archon", "requesting actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon run abort <runId> [--reason <reason>] [--requested-by <actor>] [--json]")
		return 2
	}
	runID := fs.Arg(0)
	if err := store.AppendRunEvent(runID, formations.RunEvent{
		Type:  formations.RunEventCanceled,
		Actor: *requestedBy,
		Data: map[string]any{
			"reason":      *reason,
			"requestedBy": *requestedBy,
			"final":       true,
		},
	}); err != nil {
		return failJSON(stderr, err, *jsonOut, "run", runID)
	}
	status, err := store.ProjectRun(runID)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", runID)
	}
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "%s\t%s\n", status.RunID, status.Status)
	return 0
}

func runAsk(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run ask", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "usage: archon run ask <runId> [question]")
		return 2
	}
	runID := fs.Arg(0)
	question := strings.Join(fs.Args()[1:], " ")
	status, err := store.ProjectRun(runID)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", runID)
	}
	events, err := store.ReadRunEvents(runID)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", runID)
	}
	escalations, err := store.ProjectOpenEscalations(runID)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "run", runID)
	}
	response := buildRunAskResponse(runID, question, status, events, escalations)
	if *jsonOut {
		return writeJSON(stdout, response)
	}
	fmt.Fprintln(stdout, response.Answer)
	return 0
}

func writeRunCommandResponse(stdout io.Writer, status *formations.RunStatusProjection, jsonOut bool) int {
	if jsonOut {
		return writeJSON(stdout, struct {
			RunID  string                          `json:"runId"`
			Status *formations.RunStatusProjection `json:"status"`
		}{
			RunID:  status.RunID,
			Status: status,
		})
	}
	fmt.Fprintf(stdout, "%s	%s	%s\n", status.RunID, status.Status, status.BoardSlug)
	return 0
}

func buildRunAskResponse(runID, question string, status *formations.RunStatusProjection, events []formations.RunEvent, escalations []formations.OpenEscalation) archonRunAskResponse {
	response := archonRunAskResponse{
		RunID:           runID,
		Question:        question,
		Status:          status,
		OpenEscalations: escalations,
	}
	waitingBySeq := map[int]archonRunWaitingGate{}
	waitingOrder := []int{}
	seenEvidence := map[int]bool{}
	addEvidence := func(seq int) {
		if seq <= 0 || seenEvidence[seq] {
			return
		}
		seenEvidence[seq] = true
		response.EvidenceSeqs = append(response.EvidenceSeqs, seq)
	}
	for _, escalation := range escalations {
		addEvidence(escalation.Seq)
	}
	for _, event := range events {
		switch event.Type {
		case formations.RunEventNodeOutput:
			statusText := stringFromMap(event.Data, "status")
			if statusText == "" {
				statusText = "done"
			}
			completed := archonRunCompletedNode{
				NodeID:    event.NodeID,
				Status:    statusText,
				OutputSeq: event.Seq,
				ReportRef: stringFromMap(event.Data, "reportRef"),
				Text:      stringFromMap(event.Data, "text"),
			}
			response.CompletedNodes = append(response.CompletedNodes, completed)
			response.ProducedOutputs = append(response.ProducedOutputs, archonRunProducedOutput{
				NodeID:    completed.NodeID,
				OutputSeq: completed.OutputSeq,
				ReportRef: completed.ReportRef,
				Text:      completed.Text,
			})
			addEvidence(event.Seq)
		case formations.RunEventHumanInputRequested:
			waiting := archonRunWaitingGate{
				GateID:       event.GateID,
				NodeID:       event.NodeID,
				Prompt:       stringFromMap(event.Data, "prompt"),
				Choices:      stringSliceFromMap(event.Data, "choices"),
				RequestedSeq: event.Seq,
			}
			waitingBySeq[event.Seq] = waiting
			waitingOrder = append(waitingOrder, event.Seq)
			addEvidence(event.Seq)
		case formations.RunEventHumanVerdictRecorded:
			requestedSeq := intFromMap(event.Data, "requestedSeq")
			if requestedSeq > 0 {
				delete(waitingBySeq, requestedSeq)
				continue
			}
			for seq, waiting := range waitingBySeq {
				if waiting.GateID == event.GateID {
					delete(waitingBySeq, seq)
				}
			}
		case formations.RunEventBlocked:
			response.BlockedReasons = append(response.BlockedReasons, archonRunBlockedReason{
				Seq:           event.Seq,
				NodeID:        firstNonEmpty(event.NodeID, stringFromMap(event.Data, "blockedNodeId")),
				GateID:        firstNonEmpty(event.GateID, stringFromMap(event.Data, "blockedGateId")),
				Reason:        stringFromMap(event.Data, "reason"),
				Code:          stringFromMap(event.Data, "code"),
				Boundary:      stringFromMap(event.Data, "boundary"),
				ResumeAllowed: boolFromMap(event.Data, "resumeAllowed"),
			})
			addEvidence(event.Seq)
		}
	}
	for _, seq := range waitingOrder {
		if waiting, ok := waitingBySeq[seq]; ok {
			response.WaitingGates = append(response.WaitingGates, waiting)
		}
	}
	if len(response.CompletedNodes) == 0 {
		response.MissingEvidence = append(response.MissingEvidence, "no node_output events are present in the durable ledger")
	}
	if len(response.ProducedOutputs) == 0 {
		response.MissingEvidence = append(response.MissingEvidence, "no produced output text or report references are present in the durable ledger")
	}
	response.Answer = buildRunAskAnswer(response)
	return response
}

func buildRunAskAnswer(response archonRunAskResponse) string {
	parts := []string{}
	if response.Status != nil {
		parts = append(parts, fmt.Sprintf("Run %s is %s with %d ledger events", response.RunID, response.Status.Status, response.Status.EventCount))
	} else {
		parts = append(parts, fmt.Sprintf("Run %s has no status projection", response.RunID))
	}
	if len(response.CompletedNodes) > 0 {
		nodes := make([]string, 0, len(response.CompletedNodes))
		for _, node := range response.CompletedNodes {
			nodes = append(nodes, node.NodeID)
		}
		parts = append(parts, fmt.Sprintf("completed nodes: %s", strings.Join(nodes, ", ")))
	} else {
		parts = append(parts, "no completed node_output evidence yet")
	}
	if len(response.ProducedOutputs) > 0 {
		latest := response.ProducedOutputs[len(response.ProducedOutputs)-1]
		detail := latest.ReportRef
		if latest.Text != "" {
			detail = clipText(latest.Text, 160)
		}
		if detail != "" {
			parts = append(parts, fmt.Sprintf("latest output from %s: %s", latest.NodeID, detail))
		}
	}
	if len(response.WaitingGates) > 0 {
		gates := make([]string, 0, len(response.WaitingGates))
		for _, gate := range response.WaitingGates {
			gates = append(gates, gate.GateID)
		}
		parts = append(parts, fmt.Sprintf("waiting gates: %s", strings.Join(gates, ", ")))
	}
	if len(response.BlockedReasons) > 0 {
		latest := response.BlockedReasons[len(response.BlockedReasons)-1]
		parts = append(parts, fmt.Sprintf("latest block: %s", latest.Reason))
	}
	if len(response.OpenEscalations) > 0 {
		escalations := make([]string, 0, len(response.OpenEscalations))
		for _, escalation := range response.OpenEscalations {
			where := firstNonEmpty(escalation.NodeID, escalation.GateID)
			if where == "" {
				where = "run"
			}
			escalations = append(escalations, where+": "+escalation.Reason)
		}
		parts = append(parts, fmt.Sprintf("open escalations: %s", strings.Join(escalations, "; ")))
	}
	if len(response.MissingEvidence) > 0 {
		parts = append(parts, "missing evidence: "+strings.Join(response.MissingEvidence, "; "))
	}
	return strings.Join(parts, ". ") + "."
}

func filterRunEvents(events []formations.RunEvent, nodeID string) []formations.RunEvent {
	filtered := make([]formations.RunEvent, 0, len(events))
	for _, event := range events {
		if !runEventReferencesNode(event, nodeID) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func runEventReferencesNode(event formations.RunEvent, nodeID string) bool {
	if nodeID == "" || event.NodeID == nodeID {
		return true
	}
	if event.Data == nil {
		return false
	}
	for _, key := range []string{"nodeId", "blockedNodeId", "fromNodeId", "toNodeId"} {
		if stringFromMap(event.Data, key) == nodeID {
			return true
		}
	}
	return false
}

func writeRunEventsText(stdout io.Writer, events []formations.RunEvent) {
	for _, event := range events {
		fmt.Fprintf(stdout, "%d\t%s\t%s\n", event.Seq, event.Type, event.NodeID)
	}
}

func lastRunSeq(events []formations.RunEvent) int {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}

func identityFromBoard(board *formations.BoardDocument) archonBoardIdentity {
	return archonBoardIdentity{
		ID:    board.ID,
		Slug:  board.Slug,
		Title: board.Title,
		Rev:   board.Rev,
		ETag:  board.ETag,
	}
}

func runBoardNew(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("board new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "board title")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 || strings.TrimSpace(*title) == "" {
		fmt.Fprintln(stderr, "usage: archon board new <slug> --title <title> [--json]")
		return 2
	}
	board, err := store.CreateBoard(formations.BoardCreateRequest{
		Slug:      fs.Arg(0),
		Title:     *title,
		UpdatedBy: *updatedBy,
	})
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, board)
	}
	fmt.Fprintf(stdout, "created %s\n", board.Slug)
	return 0
}

func runBoardList(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("board list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	boards, err := store.ListBoards()
	if err != nil {
		return fail(stderr, err)
	}
	if *jsonOut {
		return writeJSON(stdout, map[string]interface{}{"boards": boards})
	}
	for _, board := range boards {
		fmt.Fprintf(stdout, "%s\t%s\t%d\n", board.Slug, board.Title, board.Rev)
	}
	return 0
}

func runBoardInspect(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("board inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon board inspect <board> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	board.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, board)
	}
	fmt.Fprintf(stdout, "%s	%s	%d	%d formations\n", board.Slug, board.Title, board.Rev, len(board.Formations))
	return 0
}

func runBoardValidate(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("board validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon board validate <board> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	report := formations.ValidateBoard(board)
	if *jsonOut {
		code := writeJSON(stdout, map[string]interface{}{
			"board":    identityFromBoard(board),
			"errors":   report.Errors,
			"warnings": report.Warnings,
		})
		if code != 0 {
			return code
		}
	} else {
		fmt.Fprintf(stdout, "%s	%d errors	%d warnings\n", board.Slug, len(report.Errors), len(report.Warnings))
		for _, finding := range report.Errors {
			fmt.Fprintf(stdout, "ERROR	%s	%s	%s\n", finding.Code, finding.NodeID, finding.Message)
		}
		for _, finding := range report.Warnings {
			fmt.Fprintf(stdout, "WARN	%s	%s	%s\n", finding.Code, finding.NodeID, finding.Message)
		}
	}
	if len(report.Errors) > 0 {
		return 1
	}
	return 0
}

func runBoardArrange(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("board arrange", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon board arrange <board> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	expectedETag := "*"
	if current, err := store.ReadLayout(slug); err == nil {
		expectedETag = current.ETag
	} else if !errors.Is(err, formations.ErrNotFound) {
		return fail(stderr, err)
	}
	layout, err := store.ArrangeLayout(slug, formations.WriteOptions{ExpectedETag: expectedETag})
	if err != nil {
		return failDefinitionWrite(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	layout.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, layout)
	}
	fmt.Fprintf(stdout, "arranged %s\n", slug)
	return 0
}

func runFormationList(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	boards, err := store.ListBoards()
	if err != nil {
		return fail(stderr, err)
	}
	if *jsonOut {
		return writeJSON(stdout, map[string]interface{}{"boards": boards})
	}
	for _, board := range boards {
		fmt.Fprintf(stdout, "%s\t%s\t%d\n", board.Slug, board.Title, board.Rev)
	}
	return 0
}

func runFormationInspect(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon formation inspect <board> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	board.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, board)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%d\t%d formations\n", board.Slug, board.Title, board.Rev, len(board.Formations))
	return 0
}

func liveFromRunner(runner tmuxRunner) ([]formations.LiveAgentSession, error) {
	if runner == nil {
		return nil, nil
	}
	live, err := runner.LiveSessions()
	if err != nil {
		return nil, err
	}
	return archonLogicalTmuxSessions(live), nil
}

func liveForCard(card formations.PersonaCard, runner tmuxRunner) ([]formations.LiveAgentSession, error) {
	live, err := liveFromRunner(runner)
	if err != nil {
		return nil, err
	}
	stems := map[string]bool{}
	for _, variant := range card.HarnessVariants {
		if variant.SessionStem == "" && variant.ID == card.HarnessDefault {
			variant.SessionStem = card.ID
		}
		if variant.SessionStem != "" {
			stems[variant.SessionStem] = true
		}
	}
	filtered := make([]formations.LiveAgentSession, 0, len(live))
	for _, session := range live {
		if stems[session.Name] {
			filtered = append(filtered, session)
		}
	}
	return filtered, nil
}

func archonTmuxSessionPrefix() string {
	return strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_SESSION_PREFIX"))
}

func archonTmuxTargetSessionName(stem string) string {
	return archonTmuxSessionPrefix() + stem
}

func archonLogicalTmuxSessions(live []formations.LiveAgentSession) []formations.LiveAgentSession {
	prefix := archonTmuxSessionPrefix()
	if prefix == "" {
		return live
	}
	logical := make([]formations.LiveAgentSession, 0, len(live))
	for _, session := range live {
		stem, ok := strings.CutPrefix(session.Name, prefix)
		if !ok || stem == "" {
			continue
		}
		session.Name = stem
		logical = append(logical, session)
	}
	return logical
}

func archonExposeTmuxTargetSessions(roster *formations.AgentRoster) {
	if roster == nil || archonTmuxSessionPrefix() == "" {
		return
	}
	for i := range roster.Agents {
		if roster.Agents[i].SessionID != "" {
			roster.Agents[i].SessionID = archonTmuxTargetSessionName(roster.Agents[i].SessionID)
		}
	}
}

func (realTmuxRunner) LiveSessions() ([]formations.LiveAgentSession, error) {
	cmd := exec.Command(core.TmuxBin(), archonTmuxArgs("list-sessions", "-F", "#{session_name}:#{session_attached}")...)
	cmd.Env = archonTmuxEnv()
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "no server running") || strings.Contains(stderr, "No such file or directory") || strings.Contains(stderr, "server exited unexpectedly") {
				return nil, nil
			}
			return nil, fmt.Errorf("%s: %s", err.Error(), stderr)
		}
		return nil, err
	}
	live := []formations.LiveAgentSession{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		session := formations.LiveAgentSession{Name: parts[0], Status: "live"}
		if len(parts) == 2 {
			session.Attached = parts[1] == "1"
		}
		live = append(live, session)
	}
	return live, nil
}

func (realTmuxRunner) Spawn(name, command string) error {
	args := []string{"new-session", "-d", "-s", name}
	if command != "" {
		args = append(args, command)
	}
	cmd := exec.Command(core.TmuxBin(), archonTmuxArgs(args...)...)
	cmd.Env = archonTmuxEnv()
	return cmd.Run()
}

func (realTmuxRunner) Attach(name string) error {
	cmd := exec.Command(core.TmuxBin(), archonTmuxArgs("attach-session", "-t", name)...)
	cmd.Env = archonTmuxEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func archonTmuxArgs(args ...string) []string {
	socket := strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_SOCKET"))
	if socket == "" {
		return append([]string(nil), args...)
	}
	allArgs := []string{"-S", socket}
	allArgs = append(allArgs, args...)
	return allArgs
}

func archonTmuxEnv() []string {
	base := core.GetTmuxEnv()
	env := make([]string, 0, len(base))
	for _, item := range base {
		if strings.HasPrefix(item, "TMUX=") {
			continue
		}
		env = append(env, item)
	}
	return env
}

func writeJSON(w io.Writer, value interface{}) int {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return 1
	}
	return 0
}

func writeNDJSON(w io.Writer, value interface{}) error {
	return json.NewEncoder(w).Encode(value)
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, archonErrorMessage(err))
	return 1
}

func failJSON(stderr io.Writer, err error, jsonOut bool, boundary, selector string) int {
	if !jsonOut {
		return fail(stderr, err)
	}
	if code := writeJSON(stderr, archonErrorFromError(err, boundary, selector)); code != 0 {
		return code
	}
	return 1
}

func failDefinitionWrite(stderr io.Writer, err error, jsonOut bool, boundary, selector string) int {
	if errors.Is(err, formations.ErrInvalidDefinitionSource) {
		return failJSON(stderr, err, jsonOut, boundary, selector)
	}
	return fail(stderr, err)
}

func failRunStreamError(stdout, stderr io.Writer, err error, jsonOut bool, boundary, selector string) int {
	if !jsonOut {
		return fail(stderr, err)
	}
	if err := writeNDJSON(stdout, archonStreamError{Type: "stream_error", Error: archonErrorFromError(err, boundary, selector)}); err != nil {
		return 1
	}
	return 1
}

func archonErrorFromError(err error, boundary, selector string) archonErrorResponse {
	return archonErrorResponse{
		Code:     archonErrorCode(err),
		Message:  archonErrorMessage(err),
		Boundary: boundary,
		Selector: selector,
	}
}

func archonErrorMessage(err error) string {
	if errors.Is(err, formations.ErrDefinitionPublicationUncertain) {
		return "Reload both board and layout before any explicit retry"
	}
	return err.Error()
}

func archonErrorCode(err error) string {
	switch {
	case errors.Is(err, formations.ErrDefinitionPublicationUncertain):
		return "definition_publication_uncertain"
	case errors.Is(err, formations.ErrInvalidToolMutation):
		return "invalid_tool_mutation"
	case errors.Is(err, formations.ErrInvalidDefinitionSource):
		return formations.InvalidDefinitionSourceCode
	case errors.Is(err, formations.ErrToolExecutionUnavailable):
		return formations.ToolExecutionUnavailableCode
	case errors.Is(err, formations.ErrRuntimeAuthorityNonAuthorizing):
		return "runtime_authority_non_authorizing"
	case errors.Is(err, formations.ErrAmbiguousSelector):
		return "ambiguous_selector"
	case errors.Is(err, formations.ErrNotFound):
		return "not_found"
	case errors.Is(err, formations.ErrAlreadyExists):
		return "conflict"
	case errors.Is(err, formations.ErrConflict):
		return "conflict"
	case errors.Is(err, formations.ErrInvalidSlug):
		return "invalid_selector"
	case errors.Is(err, formations.ErrPreconditionRequired):
		return "precondition_required"
	case errors.Is(err, formations.ErrUnsupportedSchema):
		return "unsupported_schema"
	case errors.Is(err, formations.ErrLegacyScriptGateRequiresFencedMigration):
		return formations.LegacyScriptGateMigrationCode
	case errors.Is(err, formations.ErrLegacyInlineVerificationRequiresMigration):
		return formations.LegacyInlineVerificationMigrationCode
	case errors.Is(err, formations.ErrRunFinal):
		return "run_final"
	case errors.Is(err, formations.ErrRunLedgerInvalid):
		return "run_ledger_invalid"
	case errors.Is(err, formations.ErrRunResumeNotAllowed):
		return "run_resume_not_allowed"
	case errors.Is(err, formations.ErrRunEpochBlocked):
		return "run_epoch_blocked"
	default:
		return "error"
	}
}

func failSelector(stderr io.Writer, err error, jsonOut bool, boundary, selector string) int {
	return failJSON(stderr, err, jsonOut, boundary, selector)
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func boolFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, ok := values[key].(bool)
	return ok && value
}

func intFromMap(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		return 0
	}
}

func stringSliceFromMap(values map[string]any, key string) []string {
	if values == nil {
		return nil
	}
	switch raw := values[key].(type) {
	case []string:
		return append([]string(nil), raw...)
	case []any:
		items := make([]string, 0, len(raw))
		for _, item := range raw {
			if text, ok := item.(string); ok && text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func clipText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func resolveFormationCommandTarget(store *formations.Store, boardSelector, formationSelector string) (string, *formations.BoardDocument, string, error) {
	slug, err := store.ResolveBoardSelector(boardSelector)
	if err != nil {
		return "", nil, "", err
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return "", nil, "", err
	}
	formationID, err := resolveFormationSelector(board, formationSelector)
	if err != nil {
		return "", nil, "", err
	}
	return slug, board, formationID, nil
}

func resolveFormationSelector(board *formations.BoardDocument, selector string) (string, error) {
	candidates := make([]graphSelectorCandidate, 0, len(board.Formations))
	for _, formation := range board.Formations {
		candidates = append(candidates, graphSelectorCandidate{
			ID:    formation.ID,
			Title: formation.Title,
		})
	}
	return resolveGraphSelector("formation", selector, candidates)
}

func resolveGateSelector(board *formations.BoardDocument, selector string) (string, error) {
	candidates := make([]graphSelectorCandidate, 0, len(board.Gates))
	for _, gate := range board.Gates {
		candidates = append(candidates, graphSelectorCandidate{
			ID:    gate.ID,
			Title: gate.Title,
		})
	}
	return resolveGraphSelector("gate", selector, candidates)
}

func resolveMissionSelector(board *formations.BoardDocument, selector string) (string, error) {
	candidates := make([]graphSelectorCandidate, 0, len(board.Missions))
	for _, mission := range board.Missions {
		candidates = append(candidates, graphSelectorCandidate{
			ID:    mission.ID,
			Title: mission.Title,
		})
	}
	return resolveGraphSelector("mission", selector, candidates)
}

type graphSelectorCandidate struct {
	ID    string
	Title string
}

func resolveGraphSelector(kind, selector string, candidates []graphSelectorCandidate) (string, error) {
	matches := map[string]graphSelectorCandidate{}
	for _, candidate := range candidates {
		if candidate.ID == selector || candidate.Title == selector || slugKey(candidate.Title) == selector {
			matches[candidate.ID] = candidate
		}
	}
	if len(matches) == 1 {
		for id := range matches {
			return id, nil
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%w: %s %q matched %d objects", formations.ErrAmbiguousSelector, kind, selector, len(matches))
	}
	return "", fmt.Errorf("%w: %s %q", formations.ErrNotFound, kind, selector)
}

func missionByID(board *formations.BoardDocument, missionID string) (formations.MissionNode, bool) {
	for _, mission := range board.Missions {
		if mission.ID == missionID {
			return mission, true
		}
	}
	return formations.MissionNode{}, false
}

func missionReachableChain(board *formations.BoardDocument, missionID string) ([]archonMissionChainNode, []formations.BoardConnection, error) {
	type queueItem struct {
		nodeID string
		depth  int
	}
	seen := map[string]bool{missionID: true}
	queue := []queueItem{{nodeID: missionID}}
	chain := []archonMissionChainNode{}
	connections := []formations.BoardConnection{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, connection := range board.Connections {
			fromNode, ok := endpointNodeID(connection.From)
			if !ok || fromNode != current.nodeID {
				continue
			}
			toNode, ok := endpointNodeID(connection.To)
			if !ok {
				return nil, nil, fmt.Errorf("%w: malformed connection target %q", formations.ErrNotFound, connection.To)
			}
			connections = append(connections, connection)
			if seen[toNode] {
				continue
			}
			node, ok := chainNodeByID(board, toNode, current.depth+1)
			if !ok {
				return nil, nil, fmt.Errorf("%w: reachable node %q", formations.ErrNotFound, toNode)
			}
			seen[toNode] = true
			chain = append(chain, node)
			queue = append(queue, queueItem{nodeID: toNode, depth: current.depth + 1})
		}
	}
	return chain, connections, nil
}

func chainNodeByID(board *formations.BoardDocument, nodeID string, depth int) (archonMissionChainNode, bool) {
	for _, formation := range board.Formations {
		if formation.ID == nodeID {
			return archonMissionChainNode{
				ID:    formation.ID,
				Kind:  "formation",
				Title: formation.Title,
				Type:  formation.Type,
				Depth: depth,
			}, true
		}
	}
	for _, gate := range board.Gates {
		if gate.ID == nodeID {
			return archonMissionChainNode{
				ID:    gate.ID,
				Kind:  "gate",
				Title: gate.Title,
				Depth: depth,
			}, true
		}
	}
	for _, mission := range board.Missions {
		if mission.ID == nodeID {
			return archonMissionChainNode{
				ID:    mission.ID,
				Kind:  "mission",
				Title: mission.Title,
				Depth: depth,
			}, true
		}
	}
	for _, tool := range board.Tools {
		if tool.ID == nodeID {
			return archonMissionChainNode{
				ID:    tool.ID,
				Kind:  "tool",
				Title: tool.Title,
				Depth: depth,
			}, true
		}
	}
	return archonMissionChainNode{}, false
}

func endpointNodeID(endpoint string) (string, bool) {
	node, _, ok := strings.Cut(endpoint, ":")
	if !ok || node == "" {
		return "", false
	}
	return node, true
}

func slugKey(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func reorderFlags(args []string, boolFlags map[string]bool) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			continue
		}
		if boolFlags[name] {
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, positionals...)
}
