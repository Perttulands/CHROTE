package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/chrote/server/internal/formations"
)

type archonToolStore interface {
	ResolveBoardSelector(string) (string, error)
	ReadBoard(string) (*formations.BoardDocument, error)
	ReadLayout(string) (*formations.LayoutDocument, error)
	CreateTool(string, formations.ToolCreateRequest, formations.ToolWriteOptions) (*formations.ToolCreateResult, error)
	UpdateTool(string, formations.ToolUpdateRequest, formations.ToolWriteOptions) (*formations.ToolUpdateResult, error)
	DeleteTool(string, formations.ToolDeleteRequest, formations.ToolWriteOptions) (*formations.ToolDeleteResult, error)
}

type archonToolWriteSnapshot struct {
	slug  string
	board *formations.BoardDocument
	opts  formations.ToolWriteOptions
}

type archonToolInspectResponse struct {
	Board archonBoardIdentity `json:"board"`
	Tool  formations.ToolNode `json:"tool"`
}

func runToolCreate(store archonToolStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tool create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profileID := fs.String("profile-id", "", "exact host Tool profile id")
	profileVersion := fs.String("profile-version", "", "exact host Tool profile version")
	title := fs.String("title", "", "Tool title")
	paramsJSON := fs.String("params-json", "", "complete Tool parameter JSON object")
	x := fs.Int("x", 0, "exact layout x coordinate")
	y := fs.Int("y", 0, "exact layout y coordinate")
	predecessorNodeID := fs.String("predecessor-node-id", "", "exact predecessor node id placement hint")
	successorNodeID := fs.String("successor-node-id", "", "exact successor node id placement hint")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: archon tool create <board> --profile-id <id> --profile-version <version> --title <title> --params-json <object> [--x n --y n|--predecessor-node-id <id>|--successor-node-id <id>] [--json]")
		return 2
	}

	selector := archonToolProfileSelector(*profileID, *profileVersion)
	var parameters map[string]any
	if archonToolFlagPresent(fs, "params-json") {
		var err error
		parameters, err = parseArchonToolParametersJSON(*paramsJSON)
		if err != nil {
			return failJSON(stderr, err, *jsonOut, "tool", selector)
		}
	}
	placement, err := archonToolPlacementFromFlags(fs, *x, *y, *predecessorNodeID, *successorNodeID)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "tool", selector)
	}
	snapshot, err := readArchonToolWriteSnapshot(store, fs.Arg(0))
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	result, err := store.CreateTool(snapshot.slug, formations.ToolCreateRequest{
		ProfileID:      *profileID,
		ProfileVersion: *profileVersion,
		Title:          *title,
		Params:         parameters,
		Placement:      placement,
		UpdatedBy:      *updatedBy,
	}, snapshot.opts)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "tool", selector)
	}
	clearArchonToolResultSource(result.Board, result.Layout)
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "created %s\n", result.Tool.ID)
	return 0
}

func runToolUpdate(store archonToolStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tool update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "replacement Tool title")
	paramsJSON := fs.String("params-json", "", "complete replacement Tool parameter JSON object")
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon tool update <board> <tool> [--title <title>] [--params-json <object>] [--json]")
		return 2
	}

	var titlePatch *string
	if archonToolFlagPresent(fs, "title") {
		titlePatch = title
	}
	var parameters *map[string]any
	if archonToolFlagPresent(fs, "params-json") {
		parsed, err := parseArchonToolParametersJSON(*paramsJSON)
		if err != nil {
			return failJSON(stderr, err, *jsonOut, "tool", fs.Arg(1))
		}
		parameters = &parsed
	}
	snapshot, err := readArchonToolWriteSnapshot(store, fs.Arg(0))
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	toolID, err := resolveToolSelector(snapshot.board, fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "tool", fs.Arg(1))
	}
	result, err := store.UpdateTool(snapshot.slug, formations.ToolUpdateRequest{
		ToolID:    toolID,
		Title:     titlePatch,
		Params:    parameters,
		UpdatedBy: *updatedBy,
	}, snapshot.opts)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "tool", fs.Arg(1))
	}
	clearArchonToolResultSource(result.Board, result.Layout)
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "updated %s\n", result.Tool.ID)
	return 0
}

func runToolDelete(store archonToolStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tool delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	updatedBy := fs.String("updated-by", "agent:archon", "update actor")
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon tool delete <board> <tool> [--json]")
		return 2
	}
	snapshot, err := readArchonToolWriteSnapshot(store, fs.Arg(0))
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "board", fs.Arg(0))
	}
	toolID, err := resolveToolSelector(snapshot.board, fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "tool", fs.Arg(1))
	}
	result, err := store.DeleteTool(snapshot.slug, formations.ToolDeleteRequest{
		ID:        toolID,
		UpdatedBy: *updatedBy,
	}, snapshot.opts)
	if err != nil {
		return failJSON(stderr, err, *jsonOut, "tool", fs.Arg(1))
	}
	clearArchonToolResultSource(result.Board, result.Layout)
	if *jsonOut {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "deleted %s\n", result.ToolID)
	return 0
}

func runToolInspect(store archonToolStore, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tool inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"json": true})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: archon tool inspect <board> <tool> [--json]")
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
	toolID, err := resolveToolSelector(board, fs.Arg(1))
	if err != nil {
		return failSelector(stderr, err, *jsonOut, "tool", fs.Arg(1))
	}
	tool, ok := toolByID(board, toolID)
	if !ok {
		return failSelector(stderr, fmt.Errorf("%w: tool %q", formations.ErrNotFound, fs.Arg(1)), *jsonOut, "tool", fs.Arg(1))
	}
	if *jsonOut {
		return writeJSON(stdout, archonToolInspectResponse{Board: identityFromBoard(board), Tool: tool})
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s@%s\n", tool.ID, tool.Title, tool.ProfileID, tool.ProfileVersion)
	return 0
}

func readArchonToolWriteSnapshot(store archonToolStore, boardSelector string) (archonToolWriteSnapshot, error) {
	slug, err := store.ResolveBoardSelector(boardSelector)
	if err != nil {
		return archonToolWriteSnapshot{}, err
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		return archonToolWriteSnapshot{}, err
	}
	layoutExpectation := &formations.LayoutWriteExpectation{State: formations.LayoutWriteAbsent}
	layout, err := store.ReadLayout(slug)
	if err == nil {
		layoutExpectation.State = formations.LayoutWritePresent
		layoutExpectation.ETag = layout.ETag
	} else if !errors.Is(err, formations.ErrNotFound) {
		return archonToolWriteSnapshot{}, err
	}
	return archonToolWriteSnapshot{
		slug:  slug,
		board: board,
		opts: formations.ToolWriteOptions{
			Board:  formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev},
			Layout: layoutExpectation,
		},
	}, nil
}

func archonToolPlacementFromFlags(fs *flag.FlagSet, x, y int, predecessorNodeID, successorNodeID string) (formations.ToolPlacement, error) {
	var placement formations.ToolPlacement
	if archonToolFlagPresent(fs, "x") {
		placement.X = &x
	}
	if archonToolFlagPresent(fs, "y") {
		placement.Y = &y
	}
	if archonToolFlagPresent(fs, "predecessor-node-id") {
		if predecessorNodeID == "" {
			return formations.ToolPlacement{}, fmt.Errorf("%w: Tool predecessor placement hint must not be empty", formations.ErrInvalidToolMutation)
		}
		placement.PredecessorNodeID = predecessorNodeID
	}
	if archonToolFlagPresent(fs, "successor-node-id") {
		if successorNodeID == "" {
			return formations.ToolPlacement{}, fmt.Errorf("%w: Tool successor placement hint must not be empty", formations.ErrInvalidToolMutation)
		}
		placement.SuccessorNodeID = successorNodeID
	}
	return placement, nil
}

func archonToolFlagPresent(fs *flag.FlagSet, name string) bool {
	present := false
	fs.Visit(func(current *flag.Flag) {
		if current.Name == name {
			present = true
		}
	})
	return present
}

func clearArchonToolResultSource(board *formations.BoardDocument, layout *formations.LayoutDocument) {
	if board != nil {
		board.TOML = ""
	}
	if layout != nil {
		layout.TOML = ""
	}
}

func resolveToolSelector(board *formations.BoardDocument, selector string) (string, error) {
	candidates := make([]graphSelectorCandidate, 0, len(board.Tools))
	for _, tool := range board.Tools {
		candidates = append(candidates, graphSelectorCandidate{ID: tool.ID, Title: tool.Title})
	}
	return resolveGraphSelector("tool", selector, candidates)
}

func toolByID(board *formations.BoardDocument, toolID string) (formations.ToolNode, bool) {
	for _, tool := range board.Tools {
		if tool.ID == toolID {
			return tool, true
		}
	}
	return formations.ToolNode{}, false
}

func archonToolProfileSelector(profileID, profileVersion string) string {
	return profileID + "@" + profileVersion
}

func parseArchonToolParametersJSON(raw string) (map[string]any, error) {
	values, err := formations.ParseToolParametersJSON([]byte(raw))
	if err != nil {
		return nil, invalidArchonToolParameters("must be one duplicate-free JSON object of string, boolean, or signed 64-bit integer values")
	}
	return values, nil
}

func invalidArchonToolParameters(message string) error {
	return fmt.Errorf("%w: Tool params JSON %s", formations.ErrInvalidToolMutation, message)
}
