package formations

import (
	"errors"
	"os"
	"sort"
	"strings"
)

const formationLayoutGrid = 28

type arrangementItem struct {
	id    string
	kind  string
	slots int
}

// ArrangeLayout is the one explicit whole-board layout operation used by both
// the API/UI and Archon. Loading, rendering, running, and reconnecting never
// call it.
func (s *Store) ArrangeLayout(slug string, opts WriteOptions) (*LayoutDocument, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if opts.ExpectedETag == "" {
		return nil, ErrPreconditionRequired
	}
	path := s.BoardPath(slug)
	var arranged *LayoutDocument
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		board, err := parseBoard(raw)
		if err != nil {
			return err
		}
		layout, err := s.ReadLayout(slug)
		if err != nil {
			if !errors.Is(err, ErrNotFound) || opts.ExpectedETag != "*" {
				return err
			}
		} else if layout.BoardID != board.ID {
			return ErrConflict
		}
		arranged, err = s.updateLayoutNodes(slug, arrangedLayoutNodes(board), board, opts)
		return err
	})
	if err != nil {
		return nil, err
	}
	return arranged, nil
}

func arrangedLayoutNodes(board *BoardDocument) []LayoutNode {
	items := make([]arrangementItem, 0, len(board.Missions)+len(board.Formations)+len(board.Gates))
	for _, mission := range board.Missions {
		items = append(items, arrangementItem{id: mission.ID, kind: "mission"})
	}
	for _, formation := range board.Formations {
		items = append(items, arrangementItem{id: formation.ID, kind: formation.Type, slots: len(formation.Slots)})
	}
	for _, gate := range board.Gates {
		items = append(items, arrangementItem{id: gate.ID, kind: "gate"})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].id < items[j].id })
	if len(items) == 0 {
		return nil
	}

	known := make(map[string]bool, len(items))
	for _, item := range items {
		known[item.id] = true
	}
	incoming := make(map[string][]string)
	for _, connection := range board.Connections {
		from := arrangementEndpointNode(connection.From)
		to := arrangementEndpointNode(connection.To)
		if from == to || !known[from] || !known[to] {
			continue
		}
		incoming[to] = append(incoming[to], from)
	}
	for id := range incoming {
		sort.Strings(incoming[id])
	}

	depths := make(map[string]int, len(items))
	visiting := make(map[string]bool, len(items))
	var depthOf func(string) int
	depthOf = func(id string) int {
		if depth, ok := depths[id]; ok {
			return depth
		}
		if visiting[id] {
			return 0
		}
		visiting[id] = true
		depth := 0
		for _, predecessor := range incoming[id] {
			candidate := depthOf(predecessor) + 1
			if candidate > depth {
				depth = candidate
			}
		}
		delete(visiting, id)
		depths[id] = depth
		return depth
	}

	columns := make(map[int][]arrangementItem)
	columnOrder := []int{}
	for _, item := range items {
		depth := depthOf(item.id)
		if _, ok := columns[depth]; !ok {
			columnOrder = append(columnOrder, depth)
		}
		columns[depth] = append(columns[depth], item)
	}
	sort.Ints(columnOrder)

	nodes := make([]LayoutNode, 0, len(items))
	x := formationLayoutGrid * 4
	for _, depth := range columnOrder {
		column := columns[depth]
		sort.Slice(column, func(i, j int) bool { return column[i].id < column[j].id })
		y := formationLayoutGrid * 4
		width := 0
		for _, item := range column {
			itemWidth, itemHeight := arrangementItemSize(item)
			nodes = append(nodes, LayoutNode{ID: item.id, X: x, Y: y})
			y = snapLayoutUp(y + itemHeight + 48)
			if itemWidth > width {
				width = itemWidth
			}
		}
		x = snapLayoutUp(x + width + 84)
	}
	return nodes
}

func arrangementEndpointNode(endpoint string) string {
	if node, _, ok := strings.Cut(endpoint, ":"); ok {
		return node
	}
	return endpoint
}

func arrangementItemSize(item arrangementItem) (int, int) {
	switch item.kind {
	case "mission":
		return 236, 144
	case "gate":
		return 300, 124
	case FormationTypeFlow:
		width := 120 + item.slots*84
		if width < 300 {
			width = 300
		}
		if width > 560 {
			width = 560
		}
		return width, 300
	case FormationTypePeer:
		return 330, 286
	case FormationTypeOrchestrated:
		return 320, 372
	default:
		return 300, 270
	}
}

func snapLayoutUp(value int) int {
	return ((value + formationLayoutGrid - 1) / formationLayoutGrid) * formationLayoutGrid
}
