package formations

import (
	"errors"
	"fmt"
)

const (
	layoutPlacementGrid        = 28
	layoutPlacementMin         = layoutPlacementGrid * 4
	layoutPlacementWidth       = 308
	layoutPlacementHeight      = 280
	layoutPlacementStep        = 336
	layoutPlacementWrapX       = 1900
	layoutPlacementMaxAttempts = 24
)

// FindFreeLayoutPosition returns a deterministic grid position without
// changing authored layout. Board nodes missing from the layout still occupy
// their display fallback so an agent-created node does not cover them.
func (s *Store) FindFreeLayoutPosition(slug string, desiredX, desiredY int) (LayoutNode, error) {
	board, err := s.readBoardDefinitionForWrite(slug)
	if err != nil {
		return LayoutNode{}, err
	}

	persisted := map[string]LayoutNode{}
	layout, err := s.readLayoutDefinitionForWrite(slug)
	switch {
	case err == nil:
		if layout.BoardID != board.ID {
			return LayoutNode{}, fmt.Errorf("%w: layout board %q does not match %q", ErrConflict, layout.BoardID, board.ID)
		}
		for _, node := range layout.Nodes {
			persisted[node.ID] = node
		}
	case errors.Is(err, ErrNotFound):
	default:
		return LayoutNode{}, err
	}

	occupied := make([]LayoutNode, 0, len(board.Missions)+len(board.Formations)+len(board.Gates)+len(board.Tools))
	index := 0
	appendPosition := func(id string) {
		position, ok := persisted[id]
		if !ok {
			position = LayoutNode{ID: id, X: 140 + index*308, Y: 168 + (index%2)*196}
		}
		occupied = append(occupied, position)
		index++
	}
	for _, mission := range board.Missions {
		appendPosition(mission.ID)
	}
	for _, formation := range board.Formations {
		appendPosition(formation.ID)
	}
	for _, gate := range board.Gates {
		appendPosition(gate.ID)
	}
	for _, tool := range board.Tools {
		appendPosition(tool.ID)
	}

	x := maxInt(layoutPlacementMin, snapLayoutPosition(desiredX))
	y := maxInt(layoutPlacementMin, snapLayoutPosition(desiredY))
	for attempt := 0; attempt < layoutPlacementMaxAttempts; attempt++ {
		if !layoutPositionCollides(x, y, occupied) {
			return LayoutNode{X: x, Y: y}, nil
		}
		x += layoutPlacementStep
		if x > layoutPlacementWrapX {
			x = layoutPlacementMin
			y += layoutPlacementStep
		}
	}
	return LayoutNode{}, fmt.Errorf("%w: no free layout position within bounded search", ErrConflict)
}

func layoutPositionCollides(x, y int, occupied []LayoutNode) bool {
	for _, node := range occupied {
		if absLayoutPosition(node.X-x) < layoutPlacementWidth && absLayoutPosition(node.Y-y) < layoutPlacementHeight {
			return true
		}
	}
	return false
}

func snapLayoutPosition(value int) int {
	if value >= 0 {
		return ((value + layoutPlacementGrid/2) / layoutPlacementGrid) * layoutPlacementGrid
	}
	return -snapLayoutPosition(-value)
}

func absLayoutPosition(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
