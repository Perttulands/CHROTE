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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, realTmuxRunner{}))
}

func run(args []string, stdout, stderr io.Writer, runner tmuxRunner) int {
	config, args, ok := parseGlobalArgs(args, stderr)
	if !ok {
		return 2
	}
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: archon <agent|formation|gate|mission|run> <command>")
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
		case "approve":
			return runGateVerdict(store, args[2:], stdout, stderr, "pass")
		case "reject":
			return runGateVerdict(store, args[2:], stdout, stderr, "fail")
		default:
			fmt.Fprintf(stderr, "unknown gate command %q\n", args[1])
			return 2
		}
	case "mission":
		store := formations.NewStore(config.Workspace)
		switch args[1] {
		case "create":
			return runMissionCreate(store, args[2:], stdout, stderr)
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
		case "status":
			return runStatus(store, args[2:], stdout, stderr)
		case "logs":
			return runLogs(store, args[2:], stdout, stderr)
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
		fmt.Fprintf(stdout, "%s already live as %s\n", card.ID, binding.Session.Name)
		return 0
	} else if !errors.Is(err, formations.ErrAgentSessionOffline) {
		return fail(stderr, err)
	}
	if variant.SessionStem == "" {
		return fail(stderr, fmt.Errorf("%w: agent %q harness %q has no session_stem", formations.ErrAgentSessionOffline, card.ID, variant.ID))
	}
	if err := runner.Spawn(variant.SessionStem, variant.Launch); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "spawned %s as %s\n", card.ID, variant.SessionStem)
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
	if err := runner.Attach(binding.Session.Name); err != nil {
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
		return fail(stderr, err)
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
		return fail(stderr, err)
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

func runFormationSetBrief(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("formation set-brief", flag.ContinueOnError)
	fs.SetOutput(stderr)
	goal := fs.String("goal", "", "brief goal")
	beadID := fs.String("bead", "", "home- Beads id")
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
		fmt.Fprintln(stderr, "usage: archon formation set-brief <board> <formation> --goal <goal> [--bead <home-id>] [--file <path>] [--link <url>] [--json]")
		return 2
	}
	slug, board, formationID, err := resolveFormationCommandTarget(store, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return fail(stderr, err)
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
		return fail(stderr, err)
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
		return fail(stderr, err)
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
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon gate create <board> [--kinds code,human] [--criterion text] [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	result, err := store.CreateGate(slug, formations.GateCreateRequest{
		Title:     *title,
		Kinds:     splitCSV(*kinds),
		Criterion: *criterion,
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
		return fail(stderr, err)
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	gateID, err := resolveGateSelector(board, fs.Arg(1))
	if err != nil {
		return fail(stderr, err)
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
	engine := formations.NewRunEngine(store, formations.NewPersonaStore(formations.DefaultAgentsDir()), formations.NewUnavailableFormationExecutor("archon"))
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

func runMissionCreate(store *formations.Store, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mission create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "mission title")
	goal := fs.String("goal", "", "mission goal")
	beadID := fs.String("bead", "", "home- Beads id")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon mission create <board> --title <title> --goal <goal> --bead <home-id> [--json]")
		return 2
	}
	slug, err := store.ResolveBoardSelector(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	result, err := store.CreateMission(slug, formations.MissionCreateRequest{
		Title:     *title,
		Goal:      *goal,
		BeadID:    *beadID,
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
		return fail(stderr, err)
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	missionID, err := resolveMissionSelector(board, fs.Arg(1))
	if err != nil {
		return fail(stderr, err)
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
		return fail(stderr, err)
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return fail(stderr, err)
	}
	missionID := *missionSelector
	if missionID == "" {
		switch len(board.Missions) {
		case 0:
			return fail(stderr, fmt.Errorf("%w: board %q has no mission", formations.ErrNotFound, slug))
		case 1:
			missionID = board.Missions[0].ID
		default:
			return fail(stderr, fmt.Errorf("%w: board %q has multiple missions; pass --mission", formations.ErrConflict, slug))
		}
	} else if resolved, err := resolveMissionSelector(board, missionID); err != nil {
		return fail(stderr, err)
	} else {
		missionID = resolved
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := formations.NewRunEngine(store, personas, formations.NewUnavailableFormationExecutor("archon"))
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
		return fail(stderr, err)
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
		return fail(stderr, err)
	}
	personas := formations.NewPersonaStore(formations.DefaultAgentsDir())
	engine := formations.NewRunEngine(store, personas, formations.NewUnavailableFormationExecutor("archon"))
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
		return fail(stderr, err)
	}
	return writeRunCommandResponse(stdout, status, *jsonOut)
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
		return fail(stderr, err)
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
		return fail(stderr, err)
	}
	filtered := filterRunEvents(events, *nodeID)
	if *follow && *jsonOut {
		for {
			status, err := store.ProjectRun(runID)
			if err != nil {
				return fail(stderr, err)
			}
			if status.Final || isBlockedResumable(status) {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		events, err = store.ReadRunEvents(runID)
		if err != nil {
			return fail(stderr, err)
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
				return fail(stderr, err)
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
				return fail(stderr, err)
			}
			if status.Final {
				return 0
			}
		}
	}
	return 0
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
	engine := formations.NewRunEngine(store, formations.NewPersonaStore(formations.DefaultAgentsDir()), formations.NewUnavailableFormationExecutor("archon"))
	status, err := engine.ResumeRun(fs.Arg(0), formations.RunResumeRequest{
		Actor:  *actor,
		Mode:   *mode,
		Reason: *reason,
	})
	if err != nil {
		return fail(stderr, err)
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
		return fail(stderr, err)
	}
	status, err := store.ProjectRun(runID)
	if err != nil {
		return fail(stderr, err)
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
	escalations, err := store.ProjectOpenEscalations(runID)
	if err != nil {
		return fail(stderr, err)
	}
	if *jsonOut {
		return writeJSON(stdout, map[string]any{"runId": runID, "escalations": escalations})
	}
	if len(escalations) == 0 {
		fmt.Fprintf(stdout, "%s: nothing needs you right now\n", runID)
		return 0
	}
	for _, escalation := range escalations {
		fmt.Fprintf(stdout, "%s needs attention at %s: %s\n", escalation.RunID, escalation.NodeID, escalation.Reason)
	}
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
	fmt.Fprintf(stdout, "%s\t%s\t%s\n", status.RunID, status.Status, status.BoardSlug)
	return 0
}

func filterRunEvents(events []formations.RunEvent, nodeID string) []formations.RunEvent {
	filtered := make([]formations.RunEvent, 0, len(events))
	for _, event := range events {
		if nodeID != "" && event.NodeID != nodeID {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
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
		return fail(stderr, err)
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
	return runner.LiveSessions()
}

func liveForCard(card formations.PersonaCard, runner tmuxRunner) ([]formations.LiveAgentSession, error) {
	if runner == nil {
		return nil, nil
	}
	live, err := runner.LiveSessions()
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

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, err)
	return 1
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
	for _, formation := range board.Formations {
		if formation.ID == selector || formation.Title == selector || slugKey(formation.Title) == selector {
			return formation.ID, nil
		}
	}
	return "", fmt.Errorf("%w: formation %q", formations.ErrNotFound, selector)
}

func resolveGateSelector(board *formations.BoardDocument, selector string) (string, error) {
	for _, gate := range board.Gates {
		if gate.ID == selector || gate.Title == selector || slugKey(gate.Title) == selector {
			return gate.ID, nil
		}
	}
	return "", fmt.Errorf("%w: gate %q", formations.ErrNotFound, selector)
}

func resolveMissionSelector(board *formations.BoardDocument, selector string) (string, error) {
	for _, mission := range board.Missions {
		if mission.ID == selector || mission.Title == selector || slugKey(mission.Title) == selector {
			return mission.ID, nil
		}
	}
	return "", fmt.Errorf("%w: mission %q", formations.ErrNotFound, selector)
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
