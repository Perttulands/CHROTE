package formations

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
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

func TestDefinitionPairBuilderReadsPostWaitPairWithBothPeerProcessFlocksHeld(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-builder-post-wait"
	initialBoard := pairBoardFixture(slug, 1, "Initial")
	initialLayout := pairLayoutFixture(1, "initial")
	postWaitBoard := pairBoardFixture(slug, 2, "Cooperating writer")
	postWaitLayout := pairLayoutFixture(2, "cooperating-writer")
	newBoard := pairBoardFixture(slug, 3, "Builder")
	newLayout := pairLayoutFixture(3, "builder")
	writeFixture(t, store.BoardPath(slug), string(initialBoard))
	writeFixture(t, store.LayoutPath(slug), string(initialLayout))

	request := definitionPairRequestForTest(postWaitBoard, postWaitLayout, nil, pairAbsentContentForTest())
	request.build = func(current definitionPairState) (definitionPairState, error) {
		if string(current.board) != string(postWaitBoard) || !current.layout.present || string(current.layout.raw) != string(postWaitLayout) {
			return definitionPairState{}, fmt.Errorf("builder current pair = %#v, want cooperating writer's post-wait bytes", current)
		}
		assertPeerProcessDefinitionFlocksBlockedForTest(
			t,
			store.BoardPath(slug)+".lock",
			store.LayoutPath(slug)+".lock",
		)
		return definitionPairState{
			board:  append([]byte(nil), newBoard...),
			layout: pairPresentContentForTest(newLayout),
		}, nil
	}

	done := make(chan error, 1)
	err := store.withBoardDefinitionLock(slug, func(board *definitionFile) error {
		return store.withLayoutDefinitionLock(slug, func(layout *definitionFile) error {
			go runDefinitionPairBuilderPublicationForTest(store, slug, request, done)
			waitForDefinitionPairGoroutineBlockForTest(
				t,
				"runDefinitionPairBuilderPublicationForTest",
				"[sync.Mutex.Lock]",
				"sync.(*Mutex).Lock",
			)
			if err := board.writeAtomic(postWaitBoard); err != nil {
				return fmt.Errorf("publish cooperating board: %w", err)
			}
			if err := layout.writeAtomic(postWaitLayout); err != nil {
				return fmt.Errorf("publish cooperating layout: %w", err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("publish cooperating pair while builder waits: %v", err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatalf("publish builder-derived post-wait pair: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for post-wait builder publication")
	}
	assertPairFilesForTest(t, store, slug, newBoard, pairPresentContentForTest(newLayout))
}

func TestDefinitionPairRejectsCompetingSuppliedAndBuiltCandidates(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-builder-source-union"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	suppliedBoard := pairBoardFixture(slug, 2, "Supplied")
	suppliedLayout := pairLayoutFixture(2, "supplied")
	builtBoard := pairBoardFixture(slug, 2, "Built")
	builtLayout := pairLayoutFixture(2, "built")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	built := false
	request := definitionPairRequestForTest(oldBoard, oldLayout, suppliedBoard, pairPresentContentForTest(suppliedLayout))
	request.build = func(current definitionPairState) (definitionPairState, error) {
		built = true
		return definitionPairState{board: builtBoard, layout: pairPresentContentForTest(builtLayout)}, nil
	}
	err := store.publishDefinitionPair(slug, request, nil)
	if err == nil || !strings.Contains(err.Error(), "exactly one candidate source") {
		t.Errorf("competing candidate sources error = %v, want closed-source-union rejection", err)
	}
	if built {
		t.Error("competing candidate sources reached the builder")
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
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

//go:noinline
func runDefinitionPairBuilderPublicationForTest(
	store *Store,
	slug string,
	request definitionPairPublicationRequest,
	done chan<- error,
) {
	done <- store.publishDefinitionPair(slug, request, nil)
}
