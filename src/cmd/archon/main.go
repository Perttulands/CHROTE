package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

type archonFormationListResponse struct {
	Board      archonBoardIdentity        `json:"board"`
	Formations []formations.FormationNode `json:"formations"`
}

type archonFormationInspectResponse struct {
	Board       archonBoardIdentity          `json:"board"`
	Formation   formations.FormationNode     `json:"formation"`
	Connections []formations.BoardConnection `json:"connections"`
}

type archonGateChainNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type archonGateInspectResponse struct {
	Board      archonBoardIdentity          `json:"board"`
	Gate       formations.GateNode          `json:"gate"`
	Kind       string                       `json:"kind"`
	JudgeChain []archonGateChainNode        `json:"judgeChain"`
	Script     *formations.GateScriptConfig `json:"script,omitempty"`
	Pass       []formations.BoardConnection `json:"pass"`
	Fail       []formations.BoardConnection `json:"fail"`
}

type archonAgentRetireResponse struct {
	Retired              string                      `json:"retired"`
	OverriddenReferences []formations.AgentReference `json:"overriddenReferences,omitempty"`
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
	config, args, ok := parseGlobalArgs(args, stderr)
	if !ok {
		return 2
	}
	if len(args) == 1 && args[0] == "doctor" {
		return runDoctor(config, nil, stdout, stderr, runner)
	}
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: archon <agent|board|doctor|formation|gate|mission|run> <command>")
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
			return runAgentRetire(store, formations.NewStore(config.Workspace), args[2:], stdout, stderr)
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
		case "export":
			return runBoardExport(store, args[2:], stdout, stderr)
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
		case "add-input":
			return runFormationAddPort(store, args[2:], stdout, stderr, formations.FormationPortInput)
		case "add-output":
			return runFormationAddPort(store, args[2:], stdout, stderr, formations.FormationPortOutput)
		case "wire":
			return runFormationWire(store, args[2:], stdout, stderr, false)
		case "unwire":
			return runFormationWire(store, args[2:], stdout, stderr, true)
		case "run":
			return runFormationRun(store, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown formation command %q\n", args[1])
			return 2
		}
	case "gate":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "create":
			return runGateCreate(store, args[2:], stdout, stderr)
		case "judge":
			return runGateJudge(store, args[2:], stdout, stderr)
		case "inspect":
			return runGateInspect(store, args[2:], stdout, stderr)
		case "approve":
			return runGateVerdict(store, args[2:], stdout, stderr, "pass")
		case "reject":
			return runGateVerdict(store, args[2:], stdout, stderr, "fail")
		case "route":
			return runGateRoute(store, args[2:], stdout, stderr)
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
		case "set-goal":
			return runMissionSetGoal(store, args[2:], stdout, stderr)
		case "wire":
			return runMissionWire(store, args[2:], stdout, stderr)
		case "run":
			return runMissionRun(store, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown mission command %q\n", args[1])
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
			return runResume(store, args[2:], stdout, stderr)
		case "abort":
			return runAbort(store, args[2:], stdout, stderr)
		case "ask":
			return runAsk(store, args[2:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown run command %q\n", args[1])
			return 2
		}
	case "doctor":
		return runDoctor(config, args[1:], stdout, stderr, runner)
	default:
		fmt.Fprintf(stderr, "unknown archon noun %q\n", args[0])
		return 2
	}
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
	launch := fs.String("launch", "", "harness launch command")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent new <id> --kind <kind> [--harness <h>] [--launch <cmd>] [--from <path>]")
		return 2
	}
	card, err := store.CreatePersona(formations.CreatePersonaRequest{
		ID:           fs.Arg(0),
		Kind:         *kind,
		Harness:      *harness,
		Capabilities: splitCSV(*capable),
		Personality:  *personality,
		Source:       *from,
		Launch:       *launch,
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
	launch := fs.String("launch", "", "launch command for new/edited harness")
	note := fs.String("note", "", "append note")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent edit <id> [--add-capability t|--remove-capability t|--add-harness h --session-stem s --launch cmd|--note text]")
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
		Launch:           *launch,
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

// runAgentRetire retires a persona after a real cross-board reference scan.
// If the agent is still assigned to any slot and --force is not set, it refuses
// and lists every reference (board/formation/slot) so the caller can unassign
// first. With --force it retires anyway but reports exactly which references it
// overrode. With no references it retires cleanly without requiring --force.
func runAgentRetire(store *formations.PersonaStore, boards *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent retire", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "retire even if the agent is still assigned to slots")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"force": true, "json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon agent retire <id> [--force] [--json]")
		return 2
	}
	id := fs.Arg(0)
	refs, err := boards.ScanAgentReferences(id)
	if err != nil {
		return fail(stderr, err)
	}
	if len(refs) > 0 && !*force {
		fmt.Fprintf(stderr, "refusing to retire %s: still assigned to %d slot(s); unassign or rerun with --force\n", id, len(refs))
		for _, ref := range refs {
			fmt.Fprintf(stderr, "  board %s formation %s slot %s\n", ref.BoardSlug, ref.FormationID, ref.SlotID)
		}
		return 1
	}
	before, err := store.ReadPersona(id)
	if err != nil {
		return fail(stderr, err)
	}
	card, err := store.EditPersona(id, formations.EditPersonaRequest{Retire: true, ExpectedETag: before.ETag})
	if err != nil {
		return fail(stderr, err)
	}
	response := archonAgentRetireResponse{Retired: card.ID, OverriddenReferences: refs}
	if *jsonOut {
		return writeJSON(stdout, response)
	}
	fmt.Fprintf(stdout, "retired %s\n", card.ID)
	for _, ref := range refs {
		fmt.Fprintf(stdout, "  overrode assignment: board %s formation %s slot %s\n", ref.BoardSlug, ref.FormationID, ref.SlotID)
	}
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
	result, err := store.CreateFormation(slug, formations.FormationCreateRequest{
		Type:      fs.Arg(1),
		Title:     *title,
		X:         *x,
		Y:         *y,
		UpdatedBy: *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return fail(stderr, err)
	}
	result.Board.TOML = ""
	result.Layout.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "created %s\n", result.Formation.ID)
	return 0
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
		return fail(stderr, err)
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
		return fail(stderr, err)
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
		return fail(stderr, err)
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "updated brief for %s\n", formationID)
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
		return fail(stderr, err)
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
		return fail(stderr, err)
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
		return fail(stderr, err)
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
	scriptRoot := fs.String("script-root", "", "script gate root directory under workspace")
	scriptCwd := fs.String("script-cwd", ".", "script gate working directory under script root")
	var scriptArgs stringList
	fs.Var(&scriptArgs, "script-arg", "script gate command argv part; repeat for each argv piece")
	scriptTimeoutSeconds := fs.Int("script-timeout-seconds", 0, "script gate timeout in seconds")
	scriptOutputLimitBytes := fs.Int("script-output-limit-bytes", 0, "script gate output limit in bytes")
	x := fs.Int("x", 0, "layout x coordinate")
	y := fs.Int("y", 0, "layout y coordinate")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon gate create <board> [--kinds code,human] [--criterion text] [--script-root dir --script-arg argv ... --script-timeout-seconds n --script-output-limit-bytes n] [--x n] [--y n] [--json]")
		return 2
	}
	script := gateScriptConfigFromFlags(fs, *scriptRoot, *scriptCwd, scriptArgs, *scriptTimeoutSeconds, *scriptOutputLimitBytes)
	gateKinds := splitCSV(*kinds)
	if script != nil {
		if !flagWasPassed(fs, "kinds") && *kinds == "code" {
			gateKinds = []string{"script"}
		}
		if !containsString(gateKinds, "script") {
			return fail(stderr, fmt.Errorf("%w: script config requires script gate kind", formations.ErrInvalidSlug))
		}
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	result, err := store.CreateGate(slug, formations.GateCreateRequest{
		Title:     *title,
		Kinds:     gateKinds,
		Criterion: *criterion,
		Script:    script,
		X:         *x,
		Y:         *y,
		UpdatedBy: *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return fail(stderr, err)
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintln(stdout, "created gate")
	return 0
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
		return fail(stderr, err)
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
		return fail(stderr, err)
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
	actor := fs.String("actor", "human:perttu", "deciding actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon gate approve|reject <runId> <gateId> [--reason text] [--json]")
		return 2
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := formations.NewRunEngine(store, personas, formations.NewConfiguredFormationExecutorFromEnv(store, personas, "archon"))
	status, err := engine.RecordHumanGateVerdict(fs.Arg(0), formations.HumanGateVerdictRequest{
		GateID:  fs.Arg(1),
		Verdict: verdict,
		Reason:  *reason,
		Actor:   *actor,
	})
	if err != nil {
		return fail(stderr, err)
	}
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "%s\t%s\n", status.RunID, status.Status)
	return 0
}

// runGateInspect reports a gate's authoring shape: its kind (script/judge/human),
// its judge chain (if any), its script-gate config (if any), and its pass/fail
// output wiring. It is read-only and operates on the board definition, not a run.
func runGateInspect(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gate inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon gate inspect <board> <gate> [--json]")
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
	gate, ok := gateByID(board, gateID)
	if !ok {
		return failSelector(stderr, fmt.Errorf("%w: gate %q", formations.ErrNotFound, gateID), *jsonOut, "gate", gateID)
	}
	chain := gateJudgeChain(board, gateID)
	response := archonGateInspectResponse{
		Board:      identityFromBoard(board),
		Gate:       gate,
		Kind:       gateKind(gate, len(chain) > 0),
		JudgeChain: chain,
		Script:     gate.Script,
		Pass:       gateConnectionsFromPort(board, gateID, "pass"),
		Fail:       gateConnectionsFromPort(board, gateID, "fail"),
	}
	if *jsonOut {
		return writeJSON(stdout, response)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%d judge\t%d pass\t%d fail\n",
		gate.ID, gate.Title, response.Kind, len(chain), len(response.Pass), len(response.Fail))
	return 0
}

// runGateRoute is the canonical run-verdict routing verb. It shares the engine
// path of approve/reject (RecordHumanGateVerdict); approve and reject are just
// conveniences for pass and fail. The verdict must be exactly pass or fail,
// matching the engine's strict-verdict contract; anything else is rejected here
// at the CLI with a clear error rather than reaching the engine.
func runGateRoute(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gate route", flag.ContinueOnError)
	fs.SetOutput(stderr)
	verdict := fs.String("verdict", "", "gate verdict; must be exactly pass or fail")
	reason := fs.String("reason", "", "verdict reason")
	actor := fs.String("actor", "human:perttu", "deciding actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon gate route <runId> <gateId> --verdict <pass|fail> [--reason text] [--json]")
		return 2
	}
	if *verdict != "pass" && *verdict != "fail" {
		fmt.Fprintf(stderr, "gate route requires --verdict to be exactly \"pass\" or \"fail\"; got %q\n", *verdict)
		return 2
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := formations.NewRunEngine(store, personas, formations.NewConfiguredFormationExecutorFromEnv(store, personas, "archon"))
	status, err := engine.RecordHumanGateVerdict(fs.Arg(0), formations.HumanGateVerdictRequest{
		GateID:  fs.Arg(1),
		Verdict: *verdict,
		Reason:  *reason,
		Actor:   *actor,
	})
	if err != nil {
		return fail(stderr, err)
	}
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	fmt.Fprintf(stdout, "%s\t%s\n", status.RunID, status.Status)
	return 0
}

func gateByID(board *formations.BoardDocument, gateID string) (formations.GateNode, bool) {
	for _, gate := range board.Gates {
		if gate.ID == gateID {
			return gate, true
		}
	}
	return formations.GateNode{}, false
}

// gateKind reports the gate's effective routing kind. A script config makes it a
// script gate; an attached judge chain makes it a judge gate; otherwise it is a
// human gate (the default human-decision gate). This mirrors how the run engine
// decides a gate's verdict source.
func gateKind(gate formations.GateNode, hasJudgeChain bool) string {
	if gate.Script != nil {
		return "script"
	}
	if hasJudgeChain {
		return "judge"
	}
	return "human"
}

// gateJudgeChain walks the gate's judge wiring the same way the engine's
// judgeChainForGate does: it follows the gate's "judge" output to the first
// formation, then follows formation outputs forward until the chain loops back
// to the gate's judge port or runs out. It is reimplemented here because the
// engine helper is unexported.
func gateJudgeChain(board *formations.BoardDocument, gateID string) []archonGateChainNode {
	entries := gateConnectionsFromPort(board, gateID, "judge")
	if len(entries) == 0 {
		return []archonGateChainNode{}
	}
	formationByID := map[string]formations.FormationNode{}
	for _, formation := range board.Formations {
		formationByID[formation.ID] = formation
	}
	currentNode, _ := endpointNodeID(entries[0].To)
	visited := map[string]bool{}
	chain := []archonGateChainNode{}
	for currentNode != "" && !visited[currentNode] {
		visited[currentNode] = true
		formation, ok := formationByID[currentNode]
		if !ok {
			return chain
		}
		chain = append(chain, archonGateChainNode{ID: formation.ID, Title: formation.Title})
		nextNode := ""
		for _, connection := range board.Connections {
			fromNode, _ := endpointNodeID(connection.From)
			if fromNode != currentNode {
				continue
			}
			toNode, _ := endpointNodeID(connection.To)
			if toNode == gateID && strings.HasSuffix(connection.To, ":judge") {
				return chain
			}
			if _, ok := formationByID[toNode]; ok && nextNode == "" {
				nextNode = toNode
			}
		}
		currentNode = nextNode
	}
	return chain
}

// gateConnectionsFromPort returns the gate's outgoing connections from a named
// port (e.g. "pass", "fail", "judge"), used to surface a gate's verdict wiring.
func gateConnectionsFromPort(board *formations.BoardDocument, gateID, port string) []formations.BoardConnection {
	out := []formations.BoardConnection{}
	want := gateID + ":" + port
	for _, connection := range board.Connections {
		if connection.From == want {
			out = append(out, connection)
		}
	}
	return out
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
	result, err := store.CreateMission(slug, formations.MissionCreateRequest{
		Title:     *title,
		Goal:      *goal,
		BeadID:    *beadID,
		X:         *x,
		Y:         *y,
		UpdatedBy: *updatedBy,
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return fail(stderr, err)
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
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

// runMissionSetGoal reconfigures one mission's title/goal/bead. store.UpdateMission
// is FULL-REPLACE (it writes title, goal and beadId together), so to honor a
// partial edit like `mission set-goal b m --goal x` without clobbering the
// untouched fields we read the current mission first and seed the request with
// its existing title/goal/bead, then override only the flags the user actually
// passed. A passed-but-empty value (e.g. --bead "") is an explicit clear.
func runMissionSetGoal(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mission set-goal", flag.ContinueOnError)
	fs.SetOutput(stderr)
	goal := fs.String("goal", "", "mission goal")
	title := fs.String("title", "", "mission title")
	beadID := fs.String("bead", "", "project Beads id (\"\" clears)")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon mission set-goal <board> <mission> [--goal text] [--title text] [--bead beads-id] [--json]")
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
	// Seed the full-replace request with the mission's current values so unset
	// flags are preserved, then override only what the user passed.
	req := formations.MissionUpdateRequest{
		MissionID: missionID,
		Title:     mission.Title,
		Goal:      mission.Goal,
		BeadID:    mission.BeadID,
		UpdatedBy: *updatedBy,
	}
	if flagWasPassed(fs, "goal") {
		req.Goal = *goal
	}
	if flagWasPassed(fs, "title") {
		req.Title = *title
	}
	if flagWasPassed(fs, "bead") {
		req.BeadID = *beadID
	}
	result, err := store.UpdateMission(slug, req, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "mission", missionID)
	}
	result.TOML = ""
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "updated mission %s\n", missionID)
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
		return fail(stderr, err)
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
	engine := formations.NewRunEngine(store, personas, formations.NewConfiguredFormationExecutorFromEnv(store, personas, "archon"))
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
	engine := formations.NewRunEngine(store, personas, formations.NewConfiguredFormationExecutorFromEnv(store, personas, "archon"))
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
	engine := formations.NewRunEngine(store, personas, formations.NewConfiguredFormationExecutorFromEnv(store, personas, "archon"))
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

// ---- doctor: read-only operator diagnostics --------------------------------

// archonDoctorReport is the aggregate result of one or more doctor sections. It
// is structured for `--json` consumers and rendered as grouped ✓/✗ lines for
// humans. ok is false when any section reports a HARD problem.
type archonDoctorReport struct {
	OK       bool                   `json:"ok"`
	Sections []archonDoctorSection  `json:"sections"`
	Env      *archonDoctorEnv       `json:"env,omitempty"`
	Files    *archonDoctorFiles     `json:"files,omitempty"`
	Sessions *archonDoctorSessions  `json:"sessions,omitempty"`
	Checks   *archonDoctorCheckList `json:"checks,omitempty"`
}

// archonDoctorSection names a section and whether it is healthy, so the bare
// `archon doctor` JSON has a stable top-level shape regardless of which detail
// payloads are attached.
type archonDoctorSection struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

// archonDoctorCheck is a single ok/warn/problem readiness line with a precise,
// actionable message. status is exactly one of "ok", "warn", or "problem".
type archonDoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (c archonDoctorCheck) hardProblem() bool { return c.Status == doctorStatusProblem }

const (
	doctorStatusOK      = "ok"
	doctorStatusWarn    = "warn"
	doctorStatusProblem = "problem"
)

type archonDoctorEnv struct {
	OK               bool                `json:"ok"`
	Workspace        string              `json:"workspace"`
	AllowedRoots     []string            `json:"allowedRoots"`
	SelectedExecutor string              `json:"selectedExecutor"`
	SessionPrefix    string              `json:"sessionPrefix"`
	Lab              archonDoctorLabEnv  `json:"lab"`
	Tmux             archonDoctorTmuxEnv `json:"tmux"`
	Checks           []archonDoctorCheck `json:"checks"`
}

type archonDoctorLabEnv struct {
	Harnesses []string `json:"harnesses"`
	Cwd       string   `json:"cwd"`
	Roots     []string `json:"roots"`
}

type archonDoctorTmuxEnv struct {
	Harnesses     []string `json:"harnesses"`
	Socket        string   `json:"socket"`
	Cwd           string   `json:"cwd"`
	Roots         []string `json:"roots"`
	SessionPrefix string   `json:"sessionPrefix"`
	Dedicated     bool     `json:"dedicated"`
}

type archonDoctorFiles struct {
	OK                    bool                     `json:"ok"`
	Workspace             string                   `json:"workspace"`
	FormationsTreePresent bool                     `json:"formationsTreePresent"`
	BoardsDirPresent      bool                     `json:"boardsDirPresent"`
	LayoutDirPresent      bool                     `json:"layoutDirPresent"`
	RunsDirPresent        bool                     `json:"runsDirPresent"`
	BoardCount            int                      `json:"boardCount"`
	Boards                []archonDoctorBoardCheck `json:"boards"`
	Unreadable            []archonDoctorBoardCheck `json:"unreadable"`
	StaleLocks            []string                 `json:"staleLocks"`
	Checks                []archonDoctorCheck      `json:"checks"`
}

type archonDoctorBoardCheck struct {
	Slug         string `json:"slug"`
	ErrorCount   int    `json:"errorCount"`
	WarningCount int    `json:"warningCount"`
	Error        string `json:"error,omitempty"`
}

type archonDoctorSessions struct {
	OK            bool                       `json:"ok"`
	Socket        string                     `json:"socket"`
	SessionPrefix string                     `json:"sessionPrefix"`
	AllowedRoots  []string                   `json:"allowedRoots"`
	SessionCount  int                        `json:"sessionCount"`
	Sessions      []archonDoctorSessionEntry `json:"sessions"`
	Checks        []archonDoctorCheck        `json:"checks"`
}

type archonDoctorSessionEntry struct {
	Name        string `json:"name"`
	Cwd         string `json:"cwd"`
	CwdResolved bool   `json:"cwdResolved"`
	InAllowed   bool   `json:"inAllowedRoot"`
}

type archonDoctorCheckList struct {
	OK     bool                `json:"ok"`
	Checks []archonDoctorCheck `json:"checks"`
}

// runDoctor dispatches the read-only operator diagnostics surface. A bare
// `archon doctor` runs all four sections; a named subcommand runs just that one.
func runDoctor(config archonConfig, args []string, stdout, stderr io.Writer, runner tmuxRunner) int {
	// A leading flag (or no args) means bare doctor: run every section. Bare
	// doctor always renders structured JSON, so `--json` is accepted but other
	// unknown flags are rejected by the bare-doctor flag set.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		fs.SetOutput(stderr)
		fs.Bool("json", false, "write JSON")
		if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprintln(stderr, "usage: archon doctor [env|files|sessions|checks] [--json]")
			return 2
		}
		return runDoctorAll(config, stdout, stderr, runner)
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "env":
		return runDoctorEnv(config, rest, stdout, stderr)
	case "files":
		return runDoctorFiles(config, rest, stdout, stderr)
	case "sessions":
		return runDoctorSessions(config, rest, stdout, stderr, runner)
	case "checks":
		return runDoctorChecks(config, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown doctor command %q\n", sub)
		return 2
	}
}

// doctorJSONFlag parses the shared --json flag for a doctor subcommand. Doctor
// subcommands take no positional args, so any positional is a usage error.
func doctorJSONFlag(name string, args []string, stderr io.Writer) (bool, bool) {
	fs := flag.NewFlagSet("doctor "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return false, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "usage: archon doctor %s [--json]\n", name)
		return false, false
	}
	return *jsonOut, true
}

func runDoctorEnv(config archonConfig, args []string, stdout, stderr io.Writer) int {
	jsonOut, ok := doctorJSONFlag("env", args, stderr)
	if !ok {
		return 2
	}
	env := buildDoctorEnv(config)
	env.OK = !doctorHasHardProblem(env.Checks)
	if jsonOut {
		if code := writeJSON(stdout, env); code != 0 {
			return code
		}
	} else {
		writeDoctorEnvText(stdout, env)
	}
	return doctorExitCode(env.Checks)
}

func runDoctorFiles(config archonConfig, args []string, stdout, stderr io.Writer) int {
	jsonOut, ok := doctorJSONFlag("files", args, stderr)
	if !ok {
		return 2
	}
	files := buildDoctorFiles(config)
	files.OK = !doctorHasHardProblem(files.Checks)
	if jsonOut {
		if code := writeJSON(stdout, files); code != 0 {
			return code
		}
	} else {
		writeDoctorFilesText(stdout, files)
	}
	return doctorExitCode(files.Checks)
}

func runDoctorSessions(config archonConfig, args []string, stdout, stderr io.Writer, runner tmuxRunner) int {
	jsonOut, ok := doctorJSONFlag("sessions", args, stderr)
	if !ok {
		return 2
	}
	sessions, err := buildDoctorSessions(config, runner, realDoctorPaneCwd)
	if err != nil {
		return fail(stderr, err)
	}
	sessions.OK = !doctorHasHardProblem(sessions.Checks)
	if jsonOut {
		if code := writeJSON(stdout, sessions); code != 0 {
			return code
		}
	} else {
		writeDoctorSessionsText(stdout, sessions)
	}
	return doctorExitCode(sessions.Checks)
}

func runDoctorChecks(config archonConfig, args []string, stdout, stderr io.Writer) int {
	jsonOut, ok := doctorJSONFlag("checks", args, stderr)
	if !ok {
		return 2
	}
	checkList := buildDoctorChecks(config)
	checks := archonDoctorCheckList{OK: !doctorHasHardProblem(checkList), Checks: checkList}
	if jsonOut {
		if code := writeJSON(stdout, checks); code != 0 {
			return code
		}
	} else {
		writeDoctorChecksText(stdout, checks.Checks)
	}
	return doctorExitCode(checks.Checks)
}

// runDoctorAll runs every section and aggregates them into a single report. The
// exit code is non-zero if any section found a HARD problem.
func runDoctorAll(config archonConfig, stdout, stderr io.Writer, runner tmuxRunner) int {
	env := buildDoctorEnv(config)
	files := buildDoctorFiles(config)
	sessions, err := buildDoctorSessions(config, runner, realDoctorPaneCwd)
	if err != nil {
		return fail(stderr, err)
	}
	checks := buildDoctorChecks(config)

	envOK := !doctorHasHardProblem(env.Checks)
	filesOK := !doctorHasHardProblem(files.Checks)
	sessionsOK := !doctorHasHardProblem(sessions.Checks)
	checksOK := !doctorHasHardProblem(checks)
	env.OK = envOK
	files.OK = filesOK
	sessions.OK = sessionsOK

	report := archonDoctorReport{
		Env:      &env,
		Files:    &files,
		Sessions: &sessions,
		Checks:   &archonDoctorCheckList{OK: checksOK, Checks: checks},
	}
	report.Sections = []archonDoctorSection{
		{Name: "env", OK: envOK},
		{Name: "files", OK: filesOK},
		{Name: "sessions", OK: sessionsOK},
		{Name: "checks", OK: checksOK},
	}
	report.OK = envOK && filesOK && sessionsOK && checksOK

	// Bare doctor always renders structured JSON: it is an aggregate report whose
	// scannable form is the grouped sections, and a stable machine shape lets
	// operators script readiness gating.
	if code := writeJSON(stdout, report); code != 0 {
		return code
	}
	if !report.OK {
		return 1
	}
	return 0
}

// buildDoctorEnv reports the resolved workspace, allowed roots, and the executor
// env ladder, including WHICH executor NewConfiguredFormationExecutorFromEnv
// would select. It reads the configured state only; it never instantiates an
// executor that could mutate anything.
func buildDoctorEnv(config archonConfig) archonDoctorEnv {
	labConfig := formations.LabExecutorConfigFromEnv()
	tmuxConfig := formations.TmuxExecutorConfigFromEnv()
	selected := doctorSelectedExecutor(labConfig, tmuxConfig)

	env := archonDoctorEnv{
		Workspace:        config.Workspace,
		AllowedRoots:     core.GetAllowedRoots(),
		SelectedExecutor: selected,
		SessionPrefix:    archonTmuxSessionPrefix(),
		Lab: archonDoctorLabEnv{
			Harnesses: labConfig.Harnesses,
			Cwd:       labConfig.Cwd,
			Roots:     labConfig.Roots,
		},
		Tmux: archonDoctorTmuxEnv{
			Harnesses:     tmuxConfig.Harnesses,
			Socket:        tmuxConfig.Socket,
			Cwd:           tmuxConfig.Cwd,
			Roots:         tmuxConfig.Roots,
			SessionPrefix: tmuxConfig.SessionPrefix,
			Dedicated:     tmuxConfig.Dedicated,
		},
	}
	switch selected {
	case "lab":
		env.Checks = append(env.Checks, archonDoctorCheck{Name: "executor_selected", Status: doctorStatusOK, Message: "lab executor is configured (deterministic, no tmux); it takes precedence over tmux"})
	case "tmux-dedicated-required":
		env.Checks = append(env.Checks, archonDoctorCheck{Name: "executor_selected", Status: doctorStatusWarn, Message: "tmux harnesses are configured but mission execution requires CHROTE_FORMATIONS_TMUX_DEDICATED=1 and a dedicated formations socket"})
	case "dedicated-tmux":
		env.Checks = append(env.Checks, archonDoctorCheck{Name: "executor_selected", Status: doctorStatusOK, Message: "dedicated-tmux executor is configured: the tmux executor targets the dedicated formations socket with real roots; sessions must already exist. The cockpit socket is always refused"})
	default:
		env.Checks = append(env.Checks, archonDoctorCheck{Name: "executor_selected", Status: doctorStatusWarn, Message: "no executor is configured; set CHROTE_FORMATIONS_LAB_HARNESSES or CHROTE_FORMATIONS_TMUX_HARNESSES before running formations"})
	}
	return env
}

// doctorSelectedExecutor mirrors NewConfiguredFormationExecutorFromEnv's
// precedence exactly: lab harnesses win, then dedicated tmux, then a warning
// state for tmux harnesses that are configured without the dedicated opt-in. It
// does not instantiate anything.
func doctorSelectedExecutor(labConfig formations.LabExecutorConfig, tmuxConfig formations.TmuxExecutorConfig) string {
	if len(labConfig.Harnesses) != 0 {
		return "lab"
	}
	if len(tmuxConfig.Harnesses) != 0 {
		if tmuxConfig.Dedicated {
			return "dedicated-tmux"
		}
		return "tmux-dedicated-required"
	}
	return "unavailable"
}

// buildDoctorFiles reports .formations health under the workspace. It enumerates
// board files directly rather than via ListBoards so a single corrupt/unreadable
// board is named loudly as its own HARD problem instead of aborting the whole
// report. Each readable board is run through ValidateBoard; boards with Errors
// are flagged as HARD problems.
func buildDoctorFiles(config archonConfig) archonDoctorFiles {
	store := formations.NewStore(config.Workspace)
	formationsDir := filepath.Join(config.Workspace, ".formations")
	boardsDir := filepath.Join(formationsDir, "boards")
	layoutDir := filepath.Join(formationsDir, "layout")
	runsDir := filepath.Join(formationsDir, "runs")

	files := archonDoctorFiles{
		Workspace:             config.Workspace,
		FormationsTreePresent: dirExists(formationsDir),
		BoardsDirPresent:      dirExists(boardsDir),
		LayoutDirPresent:      dirExists(layoutDir),
		RunsDirPresent:        dirExists(runsDir),
		Boards:                []archonDoctorBoardCheck{},
		Unreadable:            []archonDoctorBoardCheck{},
		StaleLocks:            []string{},
	}

	if !files.FormationsTreePresent {
		files.Checks = append(files.Checks, archonDoctorCheck{
			Name:    "formations_tree",
			Status:  doctorStatusWarn,
			Message: "no .formations directory under " + config.Workspace + "; there are no boards, layout, or runs yet",
		})
		return files
	}
	files.Checks = append(files.Checks, archonDoctorCheck{Name: "formations_tree", Status: doctorStatusOK, Message: ".formations directory is present"})

	entries, err := os.ReadDir(boardsDir)
	if err != nil {
		if os.IsNotExist(err) {
			files.Checks = append(files.Checks, archonDoctorCheck{Name: "boards_dir", Status: doctorStatusWarn, Message: "no .formations/boards directory; no boards are defined"})
			return files
		}
		files.Checks = append(files.Checks, archonDoctorCheck{Name: "boards_dir", Status: doctorStatusProblem, Message: "cannot read .formations/boards: " + err.Error()})
		return files
	}

	const suffix = ".formation.toml"
	hardProblems := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".lock") {
			files.StaleLocks = append(files.StaleLocks, name)
			continue
		}
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		slug := strings.TrimSuffix(name, suffix)
		files.BoardCount++
		board, err := store.ReadBoard(slug)
		if err != nil {
			files.Unreadable = append(files.Unreadable, archonDoctorBoardCheck{Slug: slug, Error: err.Error()})
			files.Checks = append(files.Checks, archonDoctorCheck{
				Name:    "board:" + slug,
				Status:  doctorStatusProblem,
				Message: "board file " + name + " is unreadable or corrupt: " + err.Error(),
			})
			hardProblems++
			continue
		}
		// The formations store parses TOML leniently (it preserves comments and
		// unknown fields), so a malformed file does not error on read; it parses to
		// a degenerate board with no id/slug. A board missing both its id and slug
		// did not parse as an intelligible board, which is a HARD corruption signal
		// we must name loudly rather than treat as valid.
		if strings.TrimSpace(board.ID) == "" && strings.TrimSpace(board.Slug) == "" {
			files.Unreadable = append(files.Unreadable, archonDoctorBoardCheck{Slug: slug, Error: "parsed board has no id and no slug"})
			files.Checks = append(files.Checks, archonDoctorCheck{
				Name:    "board:" + slug,
				Status:  doctorStatusProblem,
				Message: "board file " + name + " did not parse as a valid board (no id and no slug were recovered); the file is likely corrupt",
			})
			hardProblems++
			continue
		}
		report := formations.ValidateBoard(board)
		check := archonDoctorBoardCheck{
			Slug:         slug,
			ErrorCount:   len(report.Errors),
			WarningCount: len(report.Warnings),
		}
		files.Boards = append(files.Boards, check)
		switch {
		case len(report.Errors) > 0:
			files.Checks = append(files.Checks, archonDoctorCheck{
				Name:    "board:" + slug,
				Status:  doctorStatusProblem,
				Message: fmt.Sprintf("board %q has %d validation error(s) and %d warning(s); run `archon board validate %s` for detail", slug, len(report.Errors), len(report.Warnings), slug),
			})
			hardProblems++
		case len(report.Warnings) > 0:
			files.Checks = append(files.Checks, archonDoctorCheck{
				Name:    "board:" + slug,
				Status:  doctorStatusWarn,
				Message: fmt.Sprintf("board %q validates with %d warning(s); run `archon board validate %s` for detail", slug, len(report.Warnings), slug),
			})
		default:
			files.Checks = append(files.Checks, archonDoctorCheck{Name: "board:" + slug, Status: doctorStatusOK, Message: "board " + slug + " validates clean"})
		}
	}
	if files.BoardCount == 0 && hardProblems == 0 {
		files.Checks = append(files.Checks, archonDoctorCheck{Name: "boards", Status: doctorStatusWarn, Message: "no board files under .formations/boards"})
	}
	if len(files.StaleLocks) > 0 {
		files.Checks = append(files.Checks, archonDoctorCheck{
			Name:    "stale_locks",
			Status:  doctorStatusWarn,
			Message: fmt.Sprintf("%d advisory lock file(s) present under .formations/boards: %s; left over locks can indicate an interrupted writer", len(files.StaleLocks), strings.Join(files.StaleLocks, ", ")),
		})
	}
	return files
}

// doctorPaneCwd resolves a tmux session's active pane cwd on a socket. It is
// injectable so the unit test never depends on a live tmux server; when there
// are no live sessions it is never called.
type doctorPaneCwd func(socket, session string) (string, bool)

// realDoctorPaneCwd asks tmux for one session's active pane cwd on the configured
// socket. It is read-only (display-message only — never send-keys, create, or
// kill). A failure returns (", false) so doctor reports the cwd as undetermined
// precisely instead of guessing.
func realDoctorPaneCwd(socket, session string) (string, bool) {
	cmd := exec.Command("tmux", "display-message", "-p", "-t", session, "#{pane_current_path}")
	if socket != "" {
		cmd.Args = append([]string{"tmux", "-S", socket}, cmd.Args[1:]...)
	}
	cmd.Env = core.GetTmuxEnv()
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	cwd := strings.TrimSpace(string(output))
	if cwd == "" {
		return "", false
	}
	return cwd, true
}

// buildDoctorSessions reports live tmux sessions using the SAME resolution
// `agent list` uses (runner.LiveSessions filtered through the configured
// session prefix), then reports each session's pane cwd and whether that cwd is
// inside an allowed root. It is strictly read-only: it lists sessions and reads
// pane cwd; it never sends keys or changes session state.
func buildDoctorSessions(config archonConfig, runner tmuxRunner, paneCwd doctorPaneCwd) (archonDoctorSessions, error) {
	prefix := archonTmuxSessionPrefix()
	roots := core.GetAllowedRoots()
	sessions := archonDoctorSessions{
		Socket:        filepath.Join(core.GetTmuxTmpdir(), "default"),
		SessionPrefix: prefix,
		AllowedRoots:  roots,
		Sessions:      []archonDoctorSessionEntry{},
	}
	live, err := liveFromRunner(runner)
	if err != nil {
		return archonDoctorSessions{}, err
	}
	sessions.SessionCount = len(live)
	anyUndetermined := false
	anyOutside := false
	for _, session := range live {
		// liveFromRunner already strips the configured prefix to the logical stem;
		// the real tmux session name re-applies it so the cwd lookup targets the
		// actual session.
		targetName := archonTmuxTargetSessionName(session.Name)
		entry := archonDoctorSessionEntry{Name: session.Name}
		if cwd, ok := paneCwd(sessions.Socket, targetName); ok {
			entry.Cwd = cwd
			entry.CwdResolved = true
			entry.InAllowed = pathInAnyRoot(cwd, roots)
			if !entry.InAllowed {
				anyOutside = true
			}
		} else {
			anyUndetermined = true
		}
		sessions.Sessions = append(sessions.Sessions, entry)
	}
	if sessions.SessionCount == 0 {
		sessions.Checks = append(sessions.Checks, archonDoctorCheck{Name: "live_sessions", Status: doctorStatusOK, Message: "no live tmux sessions on the configured socket"})
		return sessions, nil
	}
	sessions.Checks = append(sessions.Checks, archonDoctorCheck{Name: "live_sessions", Status: doctorStatusOK, Message: fmt.Sprintf("%d live tmux session(s) on the configured socket", sessions.SessionCount)})
	if anyUndetermined {
		sessions.Checks = append(sessions.Checks, archonDoctorCheck{Name: "session_cwd", Status: doctorStatusWarn, Message: "could not determine the pane cwd for one or more sessions; tmux did not return a pane_current_path"})
	}
	if anyOutside {
		sessions.Checks = append(sessions.Checks, archonDoctorCheck{Name: "session_cwd_root", Status: doctorStatusWarn, Message: "one or more session pane cwds are outside the allowed roots; dispatch into those sessions would be refused"})
	}
	return sessions, nil
}

// buildDoctorChecks reports executable-prerequisite readiness with precise
// per-check messages: tmux on PATH, the configured tmux socket path, the
// workspace present and writable, and the .formations dirs present.
func buildDoctorChecks(config archonConfig) []archonDoctorCheck {
	checks := []archonDoctorCheck{}

	if path, err := exec.LookPath("tmux"); err == nil {
		checks = append(checks, archonDoctorCheck{Name: "tmux_on_path", Status: doctorStatusOK, Message: "tmux found at " + path})
	} else {
		checks = append(checks, archonDoctorCheck{Name: "tmux_on_path", Status: doctorStatusProblem, Message: "tmux is not on PATH; agent spawn/attach and tmux execution will fail"})
	}

	socket := filepath.Join(core.GetTmuxTmpdir(), "default")
	if _, err := os.Stat(socket); err == nil {
		checks = append(checks, archonDoctorCheck{Name: "tmux_socket", Status: doctorStatusOK, Message: "tmux socket exists at " + socket})
	} else if os.IsNotExist(err) {
		checks = append(checks, archonDoctorCheck{Name: "tmux_socket", Status: doctorStatusWarn, Message: "tmux socket " + socket + " does not exist yet; it is created when the first session starts"})
	} else {
		checks = append(checks, archonDoctorCheck{Name: "tmux_socket", Status: doctorStatusProblem, Message: "cannot stat tmux socket " + socket + ": " + err.Error()})
	}

	info, err := os.Stat(config.Workspace)
	switch {
	case err != nil && os.IsNotExist(err):
		checks = append(checks, archonDoctorCheck{Name: "workspace_present", Status: doctorStatusProblem, Message: "workspace " + config.Workspace + " does not exist"})
	case err != nil:
		checks = append(checks, archonDoctorCheck{Name: "workspace_present", Status: doctorStatusProblem, Message: "cannot stat workspace " + config.Workspace + ": " + err.Error()})
	case !info.IsDir():
		checks = append(checks, archonDoctorCheck{Name: "workspace_present", Status: doctorStatusProblem, Message: "workspace path " + config.Workspace + " is not a directory"})
	default:
		checks = append(checks, archonDoctorCheck{Name: "workspace_present", Status: doctorStatusOK, Message: "workspace directory exists at " + config.Workspace})
		if err := workspaceWritable(config.Workspace); err != nil {
			checks = append(checks, archonDoctorCheck{Name: "workspace_writable", Status: doctorStatusProblem, Message: "workspace " + config.Workspace + " is not writable: " + err.Error()})
		} else {
			checks = append(checks, archonDoctorCheck{Name: "workspace_writable", Status: doctorStatusOK, Message: "workspace is writable"})
		}
		formationsDir := filepath.Join(config.Workspace, ".formations")
		if dirExists(formationsDir) {
			checks = append(checks, archonDoctorCheck{Name: "formations_dir", Status: doctorStatusOK, Message: ".formations directory is present"})
		} else {
			checks = append(checks, archonDoctorCheck{Name: "formations_dir", Status: doctorStatusWarn, Message: "no .formations directory yet; it is created on the first board write"})
		}
	}
	return checks
}

// workspaceWritable proves the workspace is writable by creating and removing a
// uniquely named probe file. It reports the real os error so the message is
// actionable rather than guessed.
func workspaceWritable(workspace string) error {
	probe := filepath.Join(workspace, fmt.Sprintf(".archon-doctor-write-probe-%d", os.Getpid()))
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	closeErr := f.Close()
	removeErr := os.Remove(probe)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func pathInAnyRoot(path string, roots []string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if abs == absRoot || strings.HasPrefix(abs, absRoot+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func doctorHasHardProblem(checks []archonDoctorCheck) bool {
	for _, check := range checks {
		if check.hardProblem() {
			return true
		}
	}
	return false
}

func doctorExitCode(checks []archonDoctorCheck) int {
	if doctorHasHardProblem(checks) {
		return 1
	}
	return 0
}

func doctorStatusMark(status string) string {
	switch status {
	case doctorStatusOK:
		return "✓"
	case doctorStatusWarn:
		return "!"
	default:
		return "✗"
	}
}

func writeDoctorChecksGroup(stdout io.Writer, title string, checks []archonDoctorCheck) {
	fmt.Fprintf(stdout, "%s\n", title)
	for _, check := range checks {
		fmt.Fprintf(stdout, "  %s %s: %s\n", doctorStatusMark(check.Status), check.Name, check.Message)
	}
}

func writeDoctorEnvText(stdout io.Writer, env archonDoctorEnv) {
	fmt.Fprintf(stdout, "env\n")
	fmt.Fprintf(stdout, "  workspace: %s\n", env.Workspace)
	fmt.Fprintf(stdout, "  allowed roots: %s\n", strings.Join(env.AllowedRoots, ", "))
	fmt.Fprintf(stdout, "  selected executor: %s\n", env.SelectedExecutor)
	writeDoctorChecksGroup(stdout, "checks", env.Checks)
}

func writeDoctorFilesText(stdout io.Writer, files archonDoctorFiles) {
	fmt.Fprintf(stdout, "files\n")
	fmt.Fprintf(stdout, "  workspace: %s\n", files.Workspace)
	fmt.Fprintf(stdout, "  boards: %d\n", files.BoardCount)
	writeDoctorChecksGroup(stdout, "checks", files.Checks)
}

func writeDoctorSessionsText(stdout io.Writer, sessions archonDoctorSessions) {
	fmt.Fprintf(stdout, "sessions\n")
	fmt.Fprintf(stdout, "  socket: %s\n", sessions.Socket)
	fmt.Fprintf(stdout, "  live sessions: %d\n", sessions.SessionCount)
	for _, session := range sessions.Sessions {
		cwd := session.Cwd
		if !session.CwdResolved {
			cwd = "(cwd undetermined)"
		}
		fmt.Fprintf(stdout, "  - %s\tcwd=%s\tinAllowedRoot=%t\n", session.Name, cwd, session.InAllowed)
	}
	writeDoctorChecksGroup(stdout, "checks", sessions.Checks)
}

func writeDoctorChecksText(stdout io.Writer, checks []archonDoctorCheck) {
	writeDoctorChecksGroup(stdout, "checks", checks)
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

// runBoardNew creates a Mission Board: the board AND its single mission in one
// atomic write. A board's identity IS its mission, so the goal is required at
// create time and there is never an empty board. The mission can be edited later
// with `mission set-goal`.
func runBoardNew(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("board new", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "board title")
	goal := fs.String("goal", "", "mission goal")
	missionTitle := fs.String("mission-title", "", "mission title (defaults to \"Mission\")")
	beadID := fs.String("bead", "", "project Beads id")
	x := fs.Int("x", 0, "mission layout x coordinate")
	y := fs.Int("y", 0, "mission layout y coordinate")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 || strings.TrimSpace(*title) == "" || strings.TrimSpace(*goal) == "" {
		fmt.Fprintln(stderr, "usage: archon board new <slug> --title <title> --goal <goal> [--bead <beads-id>] [--mission-title <title>] [--x n --y n] [--json]")
		return 2
	}
	board, err := store.CreateMissionBoard(fs.Arg(0), *title, formations.MissionCreateRequest{
		Title:     *missionTitle,
		Goal:      *goal,
		BeadID:    *beadID,
		X:         *x,
		Y:         *y,
		UpdatedBy: *updatedBy,
	}, formations.WriteOptions{})
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
	fmt.Fprintf(stdout, "%s\t%s\t%d\t%d formations\n", board.Slug, board.Title, board.Rev, len(board.Formations))
	return 0
}

// runBoardValidate runs a read-only structural integrity check and reports the
// findings. It exits non-zero (1) when there are any blocking Errors so the
// failure is loud and scriptable; warnings alone exit 0.
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
		if code := writeJSON(stdout, report); code != 0 {
			return code
		}
	} else {
		fmt.Fprintf(stdout, "%s\t%d errors\t%d warnings\n", slug, len(report.Errors), len(report.Warnings))
		for _, finding := range report.Errors {
			fmt.Fprintf(stdout, "error\t%s\t%s\t%s\n", finding.Code, finding.NodeID, finding.Message)
		}
		for _, finding := range report.Warnings {
			fmt.Fprintf(stdout, "warning\t%s\t%s\t%s\n", finding.Code, finding.NodeID, finding.Message)
		}
	}
	if len(report.Errors) > 0 {
		return 1
	}
	return 0
}

// runBoardExport emits the canonical, portable export of a board (definition plus
// layout sidecar). The export is inherently structured, so it always writes JSON.
// The board's raw TOML field is cleared to match inspect's no-raw-leak convention.
func runBoardExport(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("board export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", true, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon board export <board>")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	export, err := store.ExportBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	if export.Board != nil {
		export.Board.TOML = ""
	}
	if export.Layout != nil {
		export.Layout.TOML = ""
	}
	return writeJSON(stdout, export)
}

func runFormationList(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon formation list <board> [--json]")
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
	response := archonFormationListResponse{
		Board:      identityFromBoard(board),
		Formations: board.Formations,
	}
	if *jsonOut {
		return writeJSON(stdout, response)
	}
	for _, formation := range board.Formations {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%d slots\t%d agents\n",
			formation.ID, formation.Type, formation.Title,
			len(formation.Slots), formationAssignedAgentCount(formation))
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
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon formation inspect <board> <formation> [--json]")
		return 2
	}
	slug, board, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "formation", fs.Arg(1))
	}
	_ = slug
	formation, ok := formationByID(board, formationID)
	if !ok {
		return failSelector(stderr, fmt.Errorf("%w: formation %q", formations.ErrNotFound, formationID), *jsonOut, "formation", formationID)
	}
	response := archonFormationInspectResponse{
		Board:       identityFromBoard(board),
		Formation:   formation,
		Connections: connectionsTouchingNode(board, formationID),
	}
	if *jsonOut {
		return writeJSON(stdout, response)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%d inputs\t%d outputs\t%d slots\t%d agents\t%d connections\n",
		formation.ID, formation.Type, formation.Title,
		len(formation.Inputs), len(formation.Outputs), len(formation.Slots),
		formationAssignedAgentCount(formation), len(response.Connections))
	return 0
}

func formationByID(board *formations.BoardDocument, formationID string) (formations.FormationNode, bool) {
	for _, formation := range board.Formations {
		if formation.ID == formationID {
			return formation, true
		}
	}
	return formations.FormationNode{}, false
}

func formationAssignedAgentCount(formation formations.FormationNode) int {
	count := 0
	for _, slot := range formation.Slots {
		if slot.AgentID != "" {
			count++
		}
	}
	return count
}

// connectionsTouchingNode returns every board connection whose from or to
// endpoint references nodeID, so a formation/gate inspector shows exactly the
// wiring attached to that node rather than the whole board graph.
func connectionsTouchingNode(board *formations.BoardDocument, nodeID string) []formations.BoardConnection {
	out := []formations.BoardConnection{}
	for _, connection := range board.Connections {
		from, _ := endpointNodeID(connection.From)
		to, _ := endpointNodeID(connection.To)
		if from == nodeID || to == nodeID {
			out = append(out, connection)
		}
	}
	return out
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
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_attached}")
	cmd.Env = core.GetTmuxEnv()
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
	cmd := exec.Command("tmux", args...)
	cmd.Env = core.GetTmuxEnv()
	return cmd.Run()
}

func (realTmuxRunner) Attach(name string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Env = core.GetTmuxEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	fmt.Fprintln(stderr, err)
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
		Message:  err.Error(),
		Boundary: boundary,
		Selector: selector,
	}
}

func archonErrorCode(err error) string {
	switch {
	case errors.Is(err, formations.ErrAmbiguousSelector):
		return "ambiguous_selector"
	case errors.Is(err, formations.ErrNotFound):
		return "not_found"
	case errors.Is(err, formations.ErrAlreadyExists):
		return "conflict"
	case errors.Is(err, formations.ErrConflict):
		return "conflict"
	case errors.Is(err, formations.ErrInvalidBeadID):
		return "invalid_bead_id"
	case errors.Is(err, formations.ErrInvalidSlug):
		return "invalid_selector"
	case errors.Is(err, formations.ErrPreconditionRequired):
		return "precondition_required"
	case errors.Is(err, formations.ErrUnsupportedSchema):
		return "unsupported_schema"
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

func gateScriptConfigFromFlags(fs *flag.FlagSet, root, cwd string, command []string, timeoutSeconds, outputLimitBytes int) *formations.GateScriptConfig {
	if !anyFlagWasPassed(fs, "script-root", "script-cwd", "script-arg", "script-timeout-seconds", "script-output-limit-bytes") {
		return nil
	}
	return &formations.GateScriptConfig{
		Root:             root,
		Cwd:              cwd,
		Command:          append([]string(nil), command...),
		TimeoutSeconds:   timeoutSeconds,
		OutputLimitBytes: outputLimitBytes,
	}
}

func anyFlagWasPassed(fs *flag.FlagSet, names ...string) bool {
	for _, name := range names {
		if flagWasPassed(fs, name) {
			return true
		}
	}
	return false
}

func flagWasPassed(fs *flag.FlagSet, name string) bool {
	passed := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			passed = true
		}
	})
	return passed
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
