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

	pairStepReconcileNewLayoutAbsentForTest    = "reconcile:new:layout:absence-check"
	pairStepReconcileNewLayoutFileSyncForTest  = "reconcile:new:layout:file-sync"
	pairStepReconcileNewLayoutDirSyncForTest   = "reconcile:new:layout:dir-sync"
	pairStepReconcileNewBoardRenameForTest     = "reconcile:new:board:rename"
	pairStepReconcileNewBoardFileSyncForTest   = "reconcile:new:board:file-sync"
	pairStepReconcileNewBoardDirSyncForTest    = "reconcile:new:board:dir-sync"
	pairStepReconcileNewBoardDirSyncedForTest  = "reconcile:new:board:dir-synced"
	pairStepReconcileOldBoardRenameForTest     = "reconcile:old:board:rename"
	pairStepReconcileOldBoardFileSyncForTest   = "reconcile:old:board:file-sync"
	pairStepReconcileOldBoardDirSyncForTest    = "reconcile:old:board:dir-sync"
	pairStepReconcileOldLayoutRenameForTest    = "reconcile:old:layout:rename"
	pairStepReconcileOldLayoutUnlinkForTest    = "reconcile:old:layout:unlink"
	pairStepReconcileOldLayoutAbsentForTest    = "reconcile:old:layout:absence-check"
	pairStepReconcileOldLayoutFileSyncForTest  = "reconcile:old:layout:file-sync"
	pairStepReconcileOldLayoutDirSyncForTest   = "reconcile:old:layout:dir-sync"
	pairStepReconcileOldLayoutDirSyncedForTest = "reconcile:old:layout:dir-synced"
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
		pairStepPublishBoardDirSyncedForTest,
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
		pairStepPublishBoardDirSyncedForTest,
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

func TestDefinitionPairPostRenameFailureReconcilesToOneExactDurablePair(t *testing.T) {
	for _, failStep := range []string{
		pairStepPublishLayoutFileSyncForTest,
		pairStepPublishLayoutDirSyncForTest,
		pairStepPublishBoardRenameForTest,
		pairStepPublishBoardFileSyncForTest,
		pairStepPublishBoardDirSyncForTest,
	} {
		t.Run(failStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-post-rename-error"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			injected := errors.New("injected after first rename")
			failed := false
			var steps []string
			locksHeldThroughout := true
			reconciled := false
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			request.validate = func(current, candidate definitionPairState) error {
				locksHeldThroughout = locksHeldThroughout &&
					pairMutexHeldForTest(store.BoardPath(slug)+".lock") &&
					pairMutexHeldForTest(store.LayoutPath(slug)+".lock")
				return nil
			}
			request.cas = func(current definitionPairState) error {
				locksHeldThroughout = locksHeldThroughout &&
					pairMutexHeldForTest(store.BoardPath(slug)+".lock") &&
					pairMutexHeldForTest(store.LayoutPath(slug)+".lock")
				return nil
			}
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				locksHeldThroughout = locksHeldThroughout &&
					pairMutexHeldForTest(store.BoardPath(slug)+".lock") &&
					pairMutexHeldForTest(store.LayoutPath(slug)+".lock")
				if step == failStep && !failed {
					failed = true
					return injected
				}
				if strings.HasPrefix(step, "reconcile:") {
					reconciled = true
				}
				return nil
			})
			if !failed {
				t.Fatalf("post-rename failure point %q was not reached; steps=%v", failStep, steps)
			}
			if !reconciled {
				t.Fatalf("post-rename failure returned without synchronous reconciliation; steps=%v", steps)
			}
			if !locksHeldThroughout {
				t.Fatalf("publication or reconciliation released a definition lock before return; steps=%v", steps)
			}

			gotBoard := readFile(t, store.BoardPath(slug))
			gotLayout := readFile(t, store.LayoutPath(slug))
			switch {
			case gotBoard == string(oldBoard) && gotLayout == string(oldLayout):
				if !errors.Is(err, injected) {
					t.Fatalf("durable old/old error = %v, want original failure", err)
				}
				assertPairReconciliationDurabilityForTest(t, steps, "old")
			case gotBoard == string(newBoard) && gotLayout == string(newLayout):
				if err != nil {
					t.Fatalf("durable new/new returned error %v, want success", err)
				}
				assertPairReconciliationDurabilityForTest(t, steps, "new")
			default:
				t.Fatalf("reconciliation returned from mixed/unknown pair: board=%q layout=%q error=%v steps=%v", gotBoard, gotLayout, err, steps)
			}
		})
	}
}

func TestDefinitionPairLayoutUnlinkFailureBeforeFirstMutationPreservesOldPair(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-unlink-failure"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	injected := errors.New("layout unlink failed")
	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairAbsentContentForTest())
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		if step == pairStepPublishLayoutUnlinkForTest {
			return injected
		}
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("layout unlink failure error = %v, want ordinary injected error", err)
	}
	for _, step := range steps {
		if strings.HasPrefix(step, "reconcile:") {
			t.Fatalf("failed pre-mutation unlink entered reconciliation at %q; steps=%v", step, steps)
		}
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
}

func TestDefinitionPairPostUnlinkFailureReconcilesToDurablePresentOldOrAbsentNewLayout(t *testing.T) {
	for _, failStep := range []string{
		pairStepPublishLayoutAbsentForTest,
		pairStepPublishLayoutDirSyncForTest,
		pairStepPublishBoardRenameForTest,
		pairStepPublishBoardFileSyncForTest,
		pairStepPublishBoardDirSyncForTest,
	} {
		t.Run(failStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-post-unlink-error"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			injected := errors.New("injected after layout unlink")
			failed := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairAbsentContentForTest())
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == failStep && !failed {
					failed = true
					return injected
				}
				return nil
			})
			if !failed {
				t.Fatalf("post-unlink failure point %q was not reached; steps=%v", failStep, steps)
			}

			gotBoard := readFile(t, store.BoardPath(slug))
			layoutRaw, layoutErr := os.ReadFile(store.LayoutPath(slug))
			switch {
			case gotBoard == string(oldBoard) && layoutErr == nil && string(layoutRaw) == string(oldLayout):
				if !errors.Is(err, injected) {
					t.Fatalf("durable present old layout error = %v, want original failure", err)
				}
				assertPairReconciliationDurabilityForTest(t, steps, "old")
			case gotBoard == string(newBoard) && errors.Is(layoutErr, os.ErrNotExist):
				if err != nil {
					t.Fatalf("durable absent new layout returned error %v, want success", err)
				}
				assertPairPartialOrderForTest(t, steps,
					pairStepReconcileNewLayoutAbsentForTest,
					pairStepReconcileNewLayoutDirSyncForTest,
					pairStepReconcileNewBoardFileSyncForTest,
					pairStepReconcileNewBoardDirSyncForTest,
					pairStepReconcileNewBoardDirSyncedForTest,
				)
			default:
				t.Fatalf("post-unlink reconciliation returned mixed/unknown pair: board=%q layout=%q layoutErr=%v error=%v steps=%v", gotBoard, layoutRaw, layoutErr, err, steps)
			}
		})
	}
}

func TestDefinitionPairRollbackAfterBoardPublicationRestoresBoardBeforeLayout(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-reverse-rollback"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	injected := errors.New("board publication durability failed")
	rollForwardUnavailable := errors.New("forced rollback path")
	failed := false
	probedTerminalDurability := false
	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		if step == pairStepPublishBoardFileSyncForTest && !failed {
			failed = true
			return injected
		}
		if strings.HasPrefix(step, "reconcile:new:") {
			return rollForwardUnavailable
		}
		if step == pairStepReconcileOldLayoutDirSyncedForTest {
			probedTerminalDurability = true
			assertPeerProcessDefinitionFlocksBlockedForTest(
				t,
				store.BoardPath(slug)+".lock",
				store.LayoutPath(slug)+".lock",
			)
		}
		return nil
	})
	if !failed {
		t.Fatalf("board publication failure was not reached; steps=%v", steps)
	}
	if !errors.Is(err, injected) {
		t.Fatalf("durable rollback error = %v, want original publication error", err)
	}
	if !probedTerminalDurability {
		t.Fatal("rollback returned without a post-old-layout-parent-fsync milestone")
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
	assertPairPartialOrderForTest(t, steps,
		pairStepReconcileOldBoardRenameForTest,
		pairStepReconcileOldBoardFileSyncForTest,
		pairStepReconcileOldBoardDirSyncForTest,
		pairStepReconcileOldLayoutRenameForTest,
		pairStepReconcileOldLayoutFileSyncForTest,
		pairStepReconcileOldLayoutDirSyncForTest,
		pairStepReconcileOldLayoutDirSyncedForTest,
	)
}

func TestDefinitionPairRollbackRestoresAbsentOldLayoutAfterOldBoardIsDurable(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-rollback-absence"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))

	injected := errors.New("board durability failed after layout creation")
	failed := false
	var steps []string
	request := definitionPairRequestForTest(oldBoard, nil, newBoard, pairPresentContentForTest(newLayout))
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		if step == pairStepPublishBoardFileSyncForTest && !failed {
			failed = true
			return injected
		}
		if strings.HasPrefix(step, "reconcile:new:") {
			return errors.New("force absent-layout rollback")
		}
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("absent-layout rollback error = %v, want original publication error", err)
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairAbsentContentForTest())
	assertPairPartialOrderForTest(t, steps,
		pairStepReconcileOldBoardRenameForTest,
		pairStepReconcileOldBoardFileSyncForTest,
		pairStepReconcileOldBoardDirSyncForTest,
		pairStepReconcileOldLayoutUnlinkForTest,
		pairStepReconcileOldLayoutAbsentForTest,
		pairStepReconcileOldLayoutDirSyncForTest,
		pairStepReconcileOldLayoutDirSyncedForTest,
	)
	if pairStepObservedForTest(steps, pairStepReconcileOldLayoutFileSyncForTest) {
		t.Fatalf("absent predecessor layout was file-synced; steps=%v", steps)
	}
}

func TestDefinitionPairRollForwardReportsSuccessOnlyAfterNewPairIsDurable(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-roll-forward"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	injected := errors.New("board durability failed")
	failed := false
	probedTerminalDurability := false
	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		if step == pairStepPublishBoardFileSyncForTest && !failed {
			failed = true
			return injected
		}
		if strings.HasPrefix(step, "reconcile:old:") {
			return errors.New("force roll-forward path")
		}
		if step == pairStepReconcileNewBoardDirSyncedForTest {
			probedTerminalDurability = true
			assertPeerProcessDefinitionFlocksBlockedForTest(
				t,
				store.BoardPath(slug)+".lock",
				store.LayoutPath(slug)+".lock",
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("durable roll-forward returned error: %v", err)
	}
	if !probedTerminalDurability {
		t.Fatal("roll-forward returned without a post-new-board-parent-fsync milestone")
	}
	assertPairFilesForTest(t, store, slug, newBoard, pairPresentContentForTest(newLayout))
	assertPairReconciliationDurabilityForTest(t, steps, "new")
}

func TestDefinitionPairHoldsContinuousMutexOwnerEpochThroughReconciliation(t *testing.T) {
	tests := []struct {
		name         string
		forcedPrefix string
		terminalStep string
		wantOriginal bool
	}{
		{
			name:         "rollback old old",
			forcedPrefix: "reconcile:new:",
			terminalStep: pairStepReconcileOldLayoutDirSyncedForTest,
			wantOriginal: true,
		},
		{
			name:         "roll forward new new",
			forcedPrefix: "reconcile:old:",
			terminalStep: pairStepReconcileNewBoardDirSyncedForTest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-reconciliation-owner-epoch"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			contenders := newDefinitionPairOwnerEpochContendersForTest(
				t,
				store.BoardPath(slug)+".lock",
				store.LayoutPath(slug)+".lock",
			)
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			request.validate = func(current, candidate definitionPairState) error {
				return contenders.arm()
			}
			request.cas = func(current definitionPairState) error {
				return contenders.requireBlocked("cas")
			}
			injected := errors.New("force reconciliation owner-epoch proof")
			failed := false
			terminalDurability := false
			done := make(chan error, 1)
			go func() {
				done <- store.publishDefinitionPair(slug, request, func(step string) error {
					if err := contenders.requireBlocked(step); err != nil {
						return err
					}
					if step == pairStepPublishBoardFileSyncForTest && !failed {
						failed = true
						return injected
					}
					if strings.HasPrefix(step, test.forcedPrefix) {
						return errors.New("force opposite reconciliation outcome")
					}
					if step == test.terminalStep {
						terminalDurability = true
					}
					return nil
				})
			}()

			err := waitForDefinitionPairPublicationForTest(t, done, contenders)
			contenders.releaseAndRequireEntry(t)
			if !failed {
				t.Fatal("owner-epoch proof never entered reconciliation")
			}
			if !terminalDurability {
				t.Fatalf("reconciliation returned without terminal durability milestone %q", test.terminalStep)
			}
			if test.wantOriginal {
				if !errors.Is(err, injected) {
					t.Fatalf("rollback error = %v, want original failure", err)
				}
			} else if err != nil {
				t.Fatalf("roll-forward error = %v, want success", err)
			}
		})
	}
}

func TestDefinitionPairRollForwardConfirmsAbsentNewLayoutBeforeBoardDurability(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-roll-forward-absence"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	injected := errors.New("board durability failed after layout removal")
	failed := false
	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairAbsentContentForTest())
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		if step == pairStepPublishBoardFileSyncForTest && !failed {
			failed = true
			return injected
		}
		if strings.HasPrefix(step, "reconcile:old:") {
			return errors.New("force absent-layout roll-forward")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("durable absent-layout roll-forward returned error: %v", err)
	}
	assertPairFilesForTest(t, store, slug, newBoard, pairAbsentContentForTest())
	assertPairPartialOrderForTest(t, steps,
		pairStepReconcileNewLayoutAbsentForTest,
		pairStepReconcileNewLayoutDirSyncForTest,
		pairStepReconcileNewBoardFileSyncForTest,
		pairStepReconcileNewBoardDirSyncForTest,
		pairStepReconcileNewBoardDirSyncedForTest,
	)
	if pairStepObservedForTest(steps, pairStepReconcileNewLayoutFileSyncForTest) {
		t.Fatalf("absent new layout was file-synced during reconciliation; steps=%v", steps)
	}
}

func TestDefinitionPairFailedRollbackIONeverReturnsOrdinaryFailure(t *testing.T) {
	for _, failStep := range []string{
		pairStepReconcileOldBoardRenameForTest,
		pairStepReconcileOldBoardFileSyncForTest,
		pairStepReconcileOldBoardDirSyncForTest,
		pairStepReconcileOldLayoutRenameForTest,
		pairStepReconcileOldLayoutFileSyncForTest,
		pairStepReconcileOldLayoutDirSyncForTest,
	} {
		t.Run(failStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-rollback-sync-failure"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			initialFailure := errors.New("initial board publication failure")
			failedInitial := false
			failedSync := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == pairStepPublishBoardFileSyncForTest && !failedInitial {
					failedInitial = true
					return initialFailure
				}
				if strings.HasPrefix(step, "reconcile:new:") {
					return errors.New("roll-forward unavailable")
				}
				if step == failStep {
					failedSync = true
					return errors.New("rollback durability unavailable")
				}
				return nil
			})
			if !failedSync {
				t.Fatalf("rollback sync failure %q was not reached; steps=%v", failStep, steps)
			}
			assertDefinitionPublicationUncertainForTest(t, err)
			if failStep == pairStepReconcileOldLayoutFileSyncForTest || failStep == pairStepReconcileOldLayoutDirSyncForTest {
				// Both canonical hashes are already old here. The failed durability
				// proof must still prevent an ordinary rollback result.
				assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
			}
		})
	}
}

func TestDefinitionPairFailedReconciliationSyncNeverReturnsSuccessForNewBytes(t *testing.T) {
	for _, failStep := range []string{
		pairStepReconcileNewLayoutFileSyncForTest,
		pairStepReconcileNewLayoutDirSyncForTest,
		pairStepReconcileNewBoardFileSyncForTest,
		pairStepReconcileNewBoardDirSyncForTest,
	} {
		t.Run(failStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-roll-forward-sync-failure"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			initialFailure := errors.New("initial board publication failure")
			failedInitial := false
			failedSync := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == pairStepPublishBoardFileSyncForTest && !failedInitial {
					failedInitial = true
					return initialFailure
				}
				if strings.HasPrefix(step, "reconcile:old:") {
					return errors.New("rollback unavailable")
				}
				if step == failStep {
					failedSync = true
					return errors.New("roll-forward durability unavailable")
				}
				return nil
			})
			if !failedSync {
				t.Fatalf("roll-forward sync failure %q was not reached; steps=%v", failStep, steps)
			}
			assertDefinitionPublicationUncertainForTest(t, err)
			// Publication already installed both new byte images before its first
			// failed board sync. Exact hashes alone cannot turn them into success.
			assertPairFilesForTest(t, store, slug, newBoard, pairPresentContentForTest(newLayout))
		})
	}
}

func TestDefinitionPairFailedNewBoardRenameCannotReturnFromMixedPair(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-new-board-rename-failure"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	failedPublish := false
	failedReconcileRename := false
	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		if step == pairStepPublishBoardRenameForTest && !failedPublish {
			failedPublish = true
			return errors.New("initial board rename unavailable")
		}
		if strings.HasPrefix(step, "reconcile:old:") {
			return errors.New("rollback unavailable")
		}
		if step == pairStepReconcileNewBoardRenameForTest {
			failedReconcileRename = true
			return errors.New("reconciled board rename unavailable")
		}
		return nil
	})
	if !failedReconcileRename {
		t.Fatalf("new-board reconciliation rename was not reached; steps=%v", steps)
	}
	assertDefinitionPublicationUncertainForTest(t, err)
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(newLayout))
}

func TestDefinitionPairAbsentOldLayoutRollbackIOFailureIsUncertain(t *testing.T) {
	for _, failStep := range []string{
		pairStepReconcileOldLayoutUnlinkForTest,
		pairStepReconcileOldLayoutAbsentForTest,
		pairStepReconcileOldLayoutDirSyncForTest,
	} {
		t.Run(failStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-absent-old-layout-failure"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))

			failedPublish := false
			failedRollback := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, nil, newBoard, pairPresentContentForTest(newLayout))
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == pairStepPublishBoardFileSyncForTest && !failedPublish {
					failedPublish = true
					return errors.New("initial board durability unavailable")
				}
				if strings.HasPrefix(step, "reconcile:new:") {
					return errors.New("roll-forward unavailable")
				}
				if step == failStep {
					failedRollback = true
					return errors.New("absent-layout rollback I/O unavailable")
				}
				return nil
			})
			if !failedRollback {
				t.Fatalf("absent-layout rollback failure %q was not reached; steps=%v", failStep, steps)
			}
			assertDefinitionPublicationUncertainForTest(t, err)
			if failStep == pairStepReconcileOldLayoutAbsentForTest || failStep == pairStepReconcileOldLayoutDirSyncForTest {
				// The old hashes are exact after unlink, but absence confirmation
				// and parent durability remain causal requirements.
				assertPairFilesForTest(t, store, slug, oldBoard, pairAbsentContentForTest())
			}
		})
	}
}

func TestDefinitionPairAbsentNewLayoutReconciliationIOFailureIsUncertain(t *testing.T) {
	for _, failStep := range []string{
		pairStepReconcileNewLayoutAbsentForTest,
		pairStepReconcileNewLayoutDirSyncForTest,
	} {
		t.Run(failStep, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-absent-new-layout-failure"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			failedPublish := false
			failedRollForward := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairAbsentContentForTest())
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == pairStepPublishBoardFileSyncForTest && !failedPublish {
					failedPublish = true
					return errors.New("initial board durability unavailable")
				}
				if strings.HasPrefix(step, "reconcile:old:") {
					return errors.New("rollback unavailable")
				}
				if step == failStep {
					failedRollForward = true
					return errors.New("absent-layout roll-forward I/O unavailable")
				}
				return nil
			})
			if !failedRollForward {
				t.Fatalf("absent-layout roll-forward failure %q was not reached; steps=%v", failStep, steps)
			}
			assertDefinitionPublicationUncertainForTest(t, err)
			// Both new byte identities are exact throughout reconciliation, but
			// absence proof and layout-parent sync are still mandatory.
			assertPairFilesForTest(t, store, slug, newBoard, pairAbsentContentForTest())
		})
	}
}

func TestDefinitionPairUnreconciledMixedStateReturnsStableUncertainWithoutAutomaticRetry(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-uncertain"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	initialFailure := errors.New("layout publication durability failed")
	failedInitial := false
	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		if step == pairStepPublishLayoutFileSyncForTest && !failedInitial {
			failedInitial = true
			return initialFailure
		}
		if strings.HasPrefix(step, "reconcile:") {
			return errors.New("all reconciliation I/O unavailable")
		}
		return nil
	})
	assertDefinitionPublicationUncertainForTest(t, err)
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(newLayout))
	if got := pairStepCountForTest(steps, pairStepPublishLayoutRenameForTest); got != 1 {
		t.Fatalf("uncertain publication attempted layout publication %d times, want no automatic retry; steps=%v", got, steps)
	}
	firstPublish := firstPairStepWithPrefixForTest(steps, "publish:")
	for index := firstPublish + 1; index < len(steps); index++ {
		if strings.HasPrefix(steps[index], "stage:") {
			t.Fatalf("uncertain publication automatically restaged at %q; steps=%v", steps[index], steps)
		}
	}
}

func TestDefinitionPairFirstExactMutationAfterUncertaintyIsNotBlocked(t *testing.T) {
	fixture := newUncertainDefinitionPairFixtureForTest(t, "pair-after-uncertain-exact")
	explicitRequest := definitionPairRequestForTest(
		fixture.oldBoard,
		fixture.mixedLayout,
		fixture.finalBoard,
		pairPresentContentForTest(fixture.finalLayout),
	)
	var steps []string
	if err := fixture.store.publishDefinitionPair(fixture.slug, explicitRequest, func(step string) error {
		steps = append(steps, step)
		return nil
	}); err != nil {
		t.Fatalf("first exact mutation after uncertainty remained blocked: %v", err)
	}
	for _, required := range []string{
		pairStepPreflightBoardFileSyncForTest,
		pairStepPreflightLayoutFileSyncForTest,
		pairStepPreflightBoardDirSyncForTest,
		pairStepPreflightLayoutDirSyncForTest,
	} {
		assertPairStepObservedForTest(t, steps, required)
	}
	assertPairFilesForTest(t, fixture.store, fixture.slug, fixture.finalBoard, pairPresentContentForTest(fixture.finalLayout))
}

func TestDefinitionPairFirstStaleMutationAfterIndependentUncertaintyPreflightsThenConflicts(t *testing.T) {
	fixture := newUncertainDefinitionPairFixtureForTest(t, "pair-after-uncertain-stale")
	staleRequest := definitionPairRequestForTest(
		fixture.oldBoard,
		fixture.mixedLayout,
		fixture.finalBoard,
		pairPresentContentForTest(fixture.finalLayout),
	)
	staleRequest.expected.board.sha256 = etag([]byte("stale after uncertainty"))
	var staleSteps []string
	if err := fixture.store.publishDefinitionPair(fixture.slug, staleRequest, func(step string) error {
		staleSteps = append(staleSteps, step)
		return nil
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("post-uncertainty stale CAS error = %v, want ErrConflict", err)
	}
	for _, required := range []string{
		pairStepPreflightBoardFileSyncForTest,
		pairStepPreflightLayoutFileSyncForTest,
		pairStepPreflightBoardDirSyncForTest,
		pairStepPreflightLayoutDirSyncForTest,
	} {
		assertPairStepObservedForTest(t, staleSteps, required)
	}
	for _, step := range staleSteps {
		if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
			t.Fatalf("post-uncertainty stale CAS reached %q; steps=%v", step, staleSteps)
		}
	}
	assertPairFilesForTest(t, fixture.store, fixture.slug, fixture.oldBoard, pairPresentContentForTest(fixture.mixedLayout))
}

type uncertainDefinitionPairFixtureForTest struct {
	store       *Store
	slug        string
	oldBoard    []byte
	mixedLayout []byte
	finalBoard  []byte
	finalLayout []byte
}

func newUncertainDefinitionPairFixtureForTest(t *testing.T, slug string) uncertainDefinitionPairFixtureForTest {
	t.Helper()
	store := NewStore(t.TempDir())
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	mixedBoard := pairBoardFixture(slug, 2, "Mixed candidate")
	mixedLayout := pairLayoutFixture(2, "mixed candidate")
	finalBoard := pairBoardFixture(slug, 3, "Explicit retry")
	finalLayout := pairLayoutFixture(3, "explicit retry")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	failedInitial := false
	request := definitionPairRequestForTest(oldBoard, oldLayout, mixedBoard, pairPresentContentForTest(mixedLayout))
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		if step == pairStepPublishLayoutFileSyncForTest && !failedInitial {
			failedInitial = true
			return errors.New("leave contracted mixed crash state")
		}
		if strings.HasPrefix(step, "reconcile:") {
			return errors.New("reconciliation unavailable")
		}
		return nil
	})
	assertDefinitionPublicationUncertainForTest(t, err)
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(mixedLayout))
	return uncertainDefinitionPairFixtureForTest{
		store:       store,
		slug:        slug,
		oldBoard:    oldBoard,
		mixedLayout: mixedLayout,
		finalBoard:  finalBoard,
		finalLayout: finalLayout,
	}
}

type pairCrashForTest struct {
	step string
}

func assertPairReconciliationDurabilityForTest(t *testing.T, steps []string, state string) {
	t.Helper()
	if state == "new" {
		assertPairPartialOrderForTest(t, steps,
			pairStepReconcileNewLayoutFileSyncForTest,
			pairStepReconcileNewLayoutDirSyncForTest,
			pairStepReconcileNewBoardFileSyncForTest,
			pairStepReconcileNewBoardDirSyncForTest,
			pairStepReconcileNewBoardDirSyncedForTest,
		)
		return
	}
	for _, required := range []string{
		pairStepReconcileOldBoardFileSyncForTest,
		pairStepReconcileOldBoardDirSyncForTest,
		pairStepReconcileOldLayoutFileSyncForTest,
		pairStepReconcileOldLayoutDirSyncForTest,
		pairStepReconcileOldLayoutDirSyncedForTest,
	} {
		assertPairStepObservedForTest(t, steps, required)
	}
}

func assertDefinitionPublicationUncertainForTest(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, errDefinitionPublicationUncertain) {
		t.Fatalf("publication error = %v, want stable definition_publication_uncertain", err)
	}
	if err == nil || !strings.Contains(err.Error(), "definition_publication_uncertain") {
		t.Fatalf("publication uncertainty text = %q, want stable definition_publication_uncertain code", err)
	}
}

func pairStepCountForTest(steps []string, want string) int {
	count := 0
	for _, step := range steps {
		if step == want {
			count++
		}
	}
	return count
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
