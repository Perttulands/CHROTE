package formations

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDefinitionPairBuilderSeesExactPinnedPairUnderBothLocks(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-builder"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	built := false
	request := definitionPairRequestForTest(oldBoard, oldLayout, nil, pairAbsentContentForTest())
	request.build = func(current definitionPairState) (definitionPairState, error) {
		built = true
		if string(current.board) != string(oldBoard) || !current.layout.present || string(current.layout.raw) != string(oldLayout) {
			return definitionPairState{}, fmt.Errorf("builder current pair = %#v, want exact canonical bytes", current)
		}
		assertPairMutexHeldForTest(t, store.BoardPath(slug)+".lock", "board at builder")
		assertPairMutexHeldForTest(t, store.LayoutPath(slug)+".lock", "layout at builder")
		return definitionPairState{
			board:  append([]byte(nil), newBoard...),
			layout: pairPresentContentForTest(newLayout),
		}, nil
	}
	request.validate = func(current, candidate definitionPairState) error {
		if string(candidate.board) != string(newBoard) || !candidate.layout.present || string(candidate.layout.raw) != string(newLayout) {
			return fmt.Errorf("validator candidate pair = %#v, want exact builder bytes", candidate)
		}
		return nil
	}

	if err := store.publishDefinitionPair(slug, request, nil); err != nil {
		t.Fatalf("publish builder-derived pair: %v", err)
	}
	if !built {
		t.Fatal("pair publication skipped candidate builder")
	}
	assertPairFilesForTest(t, store, slug, newBoard, pairPresentContentForTest(newLayout))
}

func TestDefinitionPairBuilderFailureCannotReachStagingOrCanonicalMutation(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-builder-failure"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	injected := errors.New("candidate derivation rejected")
	request := definitionPairRequestForTest(oldBoard, oldLayout, nil, pairAbsentContentForTest())
	request.build = func(current definitionPairState) (definitionPairState, error) {
		assertPairMutexHeldForTest(t, store.BoardPath(slug)+".lock", "board at failing builder")
		assertPairMutexHeldForTest(t, store.LayoutPath(slug)+".lock", "layout at failing builder")
		return definitionPairState{}, injected
	}
	validated := false
	request.validate = func(current, candidate definitionPairState) error {
		validated = true
		return nil
	}
	var steps []string
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("builder failure error = %v, want injected error", err)
	}
	if validated {
		t.Fatal("builder failure reached candidate validation")
	}
	for _, step := range steps {
		if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
			t.Fatalf("builder failure reached %q; steps=%v", step, steps)
		}
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
}

func TestDefinitionPairBuilderCannotAliasPinnedOrPublishedPairBuffers(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-builder-clones"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	returnedBoard := append([]byte(nil), newBoard...)
	returnedLayout := append([]byte(nil), newLayout...)
	request := definitionPairRequestForTest(oldBoard, oldLayout, nil, pairAbsentContentForTest())
	request.build = func(current definitionPairState) (definitionPairState, error) {
		// The builder owns its input buffers. Mutating them must not change the
		// pinned current pair used for validation, CAS, or rollback.
		current.board[0] = 'X'
		current.layout.raw[0] = 'Y'
		return definitionPairState{
			board:  returnedBoard,
			layout: pairPresentContentForTest(returnedLayout),
		}, nil
	}
	request.validate = func(current, candidate definitionPairState) error {
		if string(current.board) != string(oldBoard) || !current.layout.present || string(current.layout.raw) != string(oldLayout) {
			return fmt.Errorf("validator current pair aliases builder input: %#v", current)
		}
		if string(candidate.board) != string(newBoard) || !candidate.layout.present || string(candidate.layout.raw) != string(newLayout) {
			return fmt.Errorf("validator candidate pair = %#v, want exact builder output", candidate)
		}

		// The publication kernel must own the returned candidate too. Neither a
		// retained builder buffer nor the validator's clone may mutate staging.
		returnedBoard[0] = 'Z'
		returnedLayout[0] = 'W'
		candidate.board[0] = 'Q'
		candidate.layout.raw[0] = 'R'
		return nil
	}

	if err := store.publishDefinitionPair(slug, request, nil); err != nil {
		t.Fatalf("publish builder-derived pair through clone boundaries: %v", err)
	}
	assertPairFilesForTest(t, store, slug, newBoard, pairPresentContentForTest(newLayout))
}
