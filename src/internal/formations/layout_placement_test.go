package formations

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestFindFreeLayoutPositionUsesPersistedAndFallbackNodePositions(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("placement"), `schema = 1
id = "brd_placement"
slug = "placement"
title = "Placement"
rev = 3

[[mission]]
id = "mis_brief"
title = "Brief"

[[formation]]
id = "fmn_build"
type = "solo"
title = "Build"

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
`)
	writeFixture(t, store.LayoutPath("placement"), `schema = 1
boardId = "brd_placement"
boardRev = 3

[[node]]
id = "fmn_build"
x = 448
y = 112
`)

	position, err := store.FindFreeLayoutPosition("placement", 112, 112)
	if err != nil {
		t.Fatalf("find free layout position: %v", err)
	}
	if position.X != 1120 || position.Y != 112 {
		t.Fatalf("position = %+v, want first free grid slot at 1120,112", position)
	}
}

func TestFindFreeLayoutPositionRejectsLayoutForAnotherBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("placement"), minimalBoard("placement", 3))
	writeFixture(t, store.LayoutPath("placement"), `schema = 1
boardId = "brd_other"
boardRev = 3
`)

	if _, err := store.FindFreeLayoutPosition("placement", 112, 112); !errors.Is(err, ErrConflict) {
		t.Fatalf("mismatched layout error = %v, want ErrConflict", err)
	}
}

func TestFindFreeLayoutPositionFailsWhenBoundedSearchIsFull(t *testing.T) {
	store := NewStore(t.TempDir())
	var board strings.Builder
	board.WriteString("schema = 1\nid = \"brd_full\"\nslug = \"full\"\ntitle = \"Full\"\nrev = 1\n")
	var layout strings.Builder
	layout.WriteString("schema = 1\nboardId = \"brd_full\"\nboardRev = 1\n")
	x, y := 112, 112
	for index := 0; index < 24; index++ {
		id := fmt.Sprintf("gate_%02d", index)
		fmt.Fprintf(&board, "\n[[gate]]\nid = %q\ntitle = %q\nkinds = [\"human\"]\n", id, id)
		fmt.Fprintf(&layout, "\n[[node]]\nid = %q\nx = %d\ny = %d\n", id, x, y)
		x += 336
		if x > 1900 {
			x = 112
			y += 336
		}
	}
	writeFixture(t, store.BoardPath("full"), board.String())
	writeFixture(t, store.LayoutPath("full"), layout.String())

	if _, err := store.FindFreeLayoutPosition("full", 112, 112); !errors.Is(err, ErrConflict) {
		t.Fatalf("full placement error = %v, want ErrConflict", err)
	}
}
