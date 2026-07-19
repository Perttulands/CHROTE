package formations

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const (
	pairStepPublishLayoutFileSyncForTest = "publish:layout:file-sync"
	pairStepPublishLayoutDirSyncForTest  = "publish:layout:dir-sync"
	pairStepPublishBoardFileSyncForTest  = "publish:board:file-sync"
	pairStepPublishBoardDirSyncForTest   = "publish:board:dir-sync"
)

func TestDefinitionPairPublishesDurableLayoutBeforeBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-durable-order"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	if err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		return nil
	}); err != nil {
		t.Fatalf("publish pair: %v", err)
	}

	assertPairPartialOrderForTest(t, steps,
		pairStepPublishLayoutRenameForTest,
		pairStepPublishLayoutFileSyncForTest,
		pairStepPublishLayoutDirSyncForTest,
		pairStepPublishBoardRenameForTest,
		pairStepPublishBoardFileSyncForTest,
		pairStepPublishBoardDirSyncForTest,
	)
	assertPairFilesForTest(t, store, slug, newBoard, pairPresentContentForTest(newLayout))
}

func TestDefinitionPairPublishesLayoutAbsenceDurablyBeforeBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-durable-absence"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairAbsentContentForTest())
	if err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		return nil
	}); err != nil {
		t.Fatalf("publish absent-layout pair: %v", err)
	}

	assertPairPartialOrderForTest(t, steps,
		pairStepPublishLayoutUnlinkForTest,
		pairStepPublishLayoutAbsentForTest,
		pairStepPublishLayoutDirSyncForTest,
		pairStepPublishBoardRenameForTest,
		pairStepPublishBoardFileSyncForTest,
		pairStepPublishBoardDirSyncForTest,
	)
	if pairStepObservedForTest(steps, pairStepPublishLayoutFileSyncForTest) {
		t.Fatalf("absent candidate layout was file-synced; steps=%v", steps)
	}
	assertPairFilesForTest(t, store, slug, newBoard, pairAbsentContentForTest())
}

func TestDefinitionPairCrashPointsExposeOnlyContractedBytePairs(t *testing.T) {
	tests := []struct {
		name       string
		crashStep  string
		wantBoard  string
		wantLayout string
	}{
		{name: "during staging is old old", crashStep: pairStepStageNewLayoutFileSyncForTest, wantBoard: "old", wantLayout: "old"},
		{name: "before first rename is old old", crashStep: pairStepPublishLayoutRenameForTest, wantBoard: "old", wantLayout: "old"},
		{name: "after layout rename is layout new board old", crashStep: pairStepPublishLayoutFileSyncForTest, wantBoard: "old", wantLayout: "new"},
		{name: "before board rename is layout new board old", crashStep: pairStepPublishBoardRenameForTest, wantBoard: "old", wantLayout: "new"},
		{name: "after board rename is new new", crashStep: pairStepPublishBoardFileSyncForTest, wantBoard: "new", wantLayout: "new"},
		{name: "before final board parent sync is new new", crashStep: pairStepPublishBoardDirSyncForTest, wantBoard: "new", wantLayout: "new"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-crash-state"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			crash := &pairCrashForTest{step: test.crashStep}
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			func() {
				defer func() {
					if recovered := recover(); recovered != crash {
						t.Fatalf("recovered panic = %#v, want injected crash %#v", recovered, crash)
					}
				}()
				_ = store.publishDefinitionPair(slug, request, func(step string) error {
					if step == crash.step {
						panic(crash)
					}
					return nil
				})
			}()

			gotBoard := readFile(t, store.BoardPath(slug))
			gotLayout := readFile(t, store.LayoutPath(slug))
			assertPairCrashMemberForTest(t, "board", gotBoard, test.wantBoard, string(oldBoard), string(newBoard))
			assertPairCrashMemberForTest(t, "layout", gotLayout, test.wantLayout, string(oldLayout), string(newLayout))
			if gotBoard == string(newBoard) && gotLayout == string(oldLayout) {
				t.Fatal("protocol exposed forbidden board-new/layout-old crash state")
			}
		})
	}
}

func TestDefinitionPairPreFirstRenameIOFailureReturnsOrdinaryErrorWithoutReconciliation(t *testing.T) {
	for _, failStep := range []string{
		pairStepStageOldBoardFileSyncForTest,
		pairStepStageOldLayoutFileSyncForTest,
		pairStepStageNewBoardFileSyncForTest,
		pairStepStageNewLayoutFileSyncForTest,
		pairStepPublishLayoutRenameForTest,
	} {
		t.Run(failStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-pre-rename-error"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			injected := errors.New("injected before first rename")
			failed := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == failStep && !failed {
					failed = true
					return injected
				}
				return nil
			})
			if !errors.Is(err, injected) {
				t.Fatalf("publication error = %v, want ordinary injected error", err)
			}
			if !failed {
				t.Fatalf("failure point %q not reached; steps=%v", failStep, steps)
			}
			for _, step := range steps {
				if strings.HasPrefix(step, "reconcile:") {
					t.Fatalf("pre-first-rename failure entered reconciliation at %q; steps=%v", step, steps)
				}
			}
			assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
		})
	}
}

func TestDefinitionPairAbsentLayoutCrashNeverExposesBoardNewWithLayoutOld(t *testing.T) {
	tests := []struct {
		crashStep  string
		wantBoard  []byte
		wantAbsent bool
	}{
		{crashStep: pairStepPublishLayoutUnlinkForTest, wantBoard: nil, wantAbsent: false},
		{crashStep: pairStepPublishLayoutAbsentForTest, wantBoard: nil, wantAbsent: true},
		{crashStep: pairStepPublishBoardRenameForTest, wantBoard: nil, wantAbsent: true},
		{crashStep: pairStepPublishBoardFileSyncForTest, wantBoard: []byte("new"), wantAbsent: true},
	}
	for _, test := range tests {
		t.Run(test.crashStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-absence-crash"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			crash := &pairCrashForTest{step: test.crashStep}
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairAbsentContentForTest())
			func() {
				defer func() {
					if recovered := recover(); recovered != crash {
						t.Fatalf("recovered panic = %#v, want %#v", recovered, crash)
					}
				}()
				_ = store.publishDefinitionPair(slug, request, func(step string) error {
					if step == crash.step {
						panic(crash)
					}
					return nil
				})
			}()

			wantBoard := oldBoard
			if test.wantBoard != nil {
				wantBoard = newBoard
			}
			if got := readFile(t, store.BoardPath(slug)); got != string(wantBoard) {
				t.Fatalf("board at crash = %q, want %q", got, wantBoard)
			}
			_, layoutErr := os.Lstat(store.LayoutPath(slug))
			if gotAbsent := errors.Is(layoutErr, os.ErrNotExist); gotAbsent != test.wantAbsent {
				t.Fatalf("layout absent at crash = %t, want %t (err=%v)", gotAbsent, test.wantAbsent, layoutErr)
			}
			if string(wantBoard) == string(newBoard) && !test.wantAbsent {
				t.Fatal("protocol exposed board-new/layout-old during absence publication")
			}
		})
	}
}

type pairCrashForTest struct {
	step string
}

func assertPairCrashMemberForTest(t *testing.T, member, got, wantState, oldRaw, newRaw string) {
	t.Helper()
	want := oldRaw
	if wantState == "new" {
		want = newRaw
	}
	if got != want {
		t.Fatalf("%s bytes at crash = %q, want %s bytes %q", member, got, wantState, want)
	}
}

func assertPairPartialOrderForTest(t *testing.T, steps []string, ordered ...string) {
	t.Helper()
	previous := -1
	for _, step := range ordered {
		index := pairStepIndexAfterForTest(steps, step, previous+1)
		if index < 0 {
			t.Fatalf("publication steps omitted %q after index %d: %v", step, previous, steps)
		}
		previous = index
	}
}

func pairStepIndexAfterForTest(steps []string, want string, start int) int {
	for index := start; index < len(steps); index++ {
		if steps[index] == want {
			return index
		}
	}
	return -1
}
