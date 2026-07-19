package formations

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	pairStepPreflightBoardFileSyncForTest    = "preflight:board:file-sync"
	pairStepPreflightLayoutFileSyncForTest   = "preflight:layout:file-sync"
	pairStepPreflightBoardDirSyncForTest     = "preflight:board:dir-sync"
	pairStepPreflightLayoutDirSyncForTest    = "preflight:layout:dir-sync"
	pairStepPreflightLayoutParentSyncForTest = "preflight:layout-directory-parent:dir-sync"
	pairStepStageOldBoardCreateForTest       = "stage:old-board:create"
	pairStepStageOldBoardWriteForTest        = "stage:old-board:write"
	pairStepStageOldBoardFileSyncForTest     = "stage:old-board:file-sync"
	pairStepStageOldBoardCloseForTest        = "stage:old-board:close"
	pairStepStageOldLayoutCreateForTest      = "stage:old-layout:create"
	pairStepStageOldLayoutWriteForTest       = "stage:old-layout:write"
	pairStepStageOldLayoutFileSyncForTest    = "stage:old-layout:file-sync"
	pairStepStageOldLayoutCloseForTest       = "stage:old-layout:close"
	pairStepStageNewBoardCreateForTest       = "stage:new-board:create"
	pairStepStageNewBoardWriteForTest        = "stage:new-board:write"
	pairStepStageNewBoardFileSyncForTest     = "stage:new-board:file-sync"
	pairStepStageNewBoardCloseForTest        = "stage:new-board:close"
	pairStepStageNewLayoutCreateForTest      = "stage:new-layout:create"
	pairStepStageNewLayoutWriteForTest       = "stage:new-layout:write"
	pairStepStageNewLayoutFileSyncForTest    = "stage:new-layout:file-sync"
	pairStepStageNewLayoutCloseForTest       = "stage:new-layout:close"
	pairStepPublishLayoutRenameForTest       = "publish:layout:rename"
	pairStepPublishLayoutUnlinkForTest       = "publish:layout:unlink"
	pairStepPublishLayoutAbsentForTest       = "publish:layout:absence-check"
	pairStepPublishBoardRenameForTest        = "publish:board:rename"
)

func TestDefinitionPairIdentityKeepsAbsentLayoutDistinctFromPresentEmptyBytes(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-identity"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	newBoard := pairBoardFixture(slug, 2, "New")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), "")

	request := definitionPairRequestForTest(oldBoard, nil, newBoard, pairPresentContentForTest(nil))
	request.validate = func(current, candidate definitionPairState) error {
		// The protocol identity test deliberately admits empty bytes. Semantic
		// board/layout validation belongs to the caller and must not collapse
		// a present zero-byte file into the explicit absent state.
		return nil
	}
	if err := store.publishDefinitionPair(slug, request, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("absent-layout expectation against present empty file error = %v, want ErrConflict", err)
	}
	if got := readFile(t, store.BoardPath(slug)); got != string(oldBoard) {
		t.Fatalf("identity conflict mutated board:\n%s", got)
	}
	if info, err := os.Stat(store.LayoutPath(slug)); err != nil || info.Size() != 0 {
		t.Fatalf("identity conflict did not preserve present empty layout: info=%v err=%v", info, err)
	}

	request = definitionPairRequestForTest(oldBoard, []byte{}, newBoard, pairPresentContentForTest(nil))
	request.validate = func(current, candidate definitionPairState) error { return nil }
	if err := store.publishDefinitionPair(slug, request, nil); err != nil {
		t.Fatalf("present-empty exact identity publication: %v", err)
	}
	if got := readFile(t, store.BoardPath(slug)); got != string(newBoard) {
		t.Fatalf("present-empty publication board = %q, want exact candidate", got)
	}
	if info, err := os.Stat(store.LayoutPath(slug)); err != nil || info.Size() != 0 {
		t.Fatalf("present-empty publication changed layout presence: info=%v err=%v", info, err)
	}
}

func TestDefinitionPairRequiresPresentExactBoardIdentity(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-board-identity"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	newBoard := pairBoardFixture(slug, 2, "New")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))

	tests := []struct {
		name     string
		expected definitionPairIdentity
	}{
		{name: "absent", expected: definitionPairIdentity{}},
		{name: "wrong SHA", expected: definitionPairIdentity{present: true, sha256: etag([]byte("other board"))}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := definitionPairRequestForTest(oldBoard, nil, newBoard, pairAbsentContentForTest())
			request.expected.board = test.expected
			if err := store.publishDefinitionPair(slug, request, nil); !errors.Is(err, ErrConflict) {
				t.Fatalf("publication error = %v, want ErrConflict", err)
			}
			if got := readFile(t, store.BoardPath(slug)); got != string(oldBoard) {
				t.Fatalf("board identity conflict mutated board:\n%s", got)
			}
			if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("board identity conflict materialized layout: %v", err)
			}
		})
	}
}

func TestDefinitionPairRejectsWrongPresentLayoutSHAAfterDurablePreflight(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-layout-identity"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	request.expected.layout.sha256 = etag([]byte("wrong layout identity"))
	var steps []string
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		return nil
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong layout SHA error = %v, want ErrConflict", err)
	}
	for _, required := range []string{
		pairStepPreflightBoardFileSyncForTest,
		pairStepPreflightLayoutFileSyncForTest,
		pairStepPreflightBoardDirSyncForTest,
		pairStepPreflightLayoutDirSyncForTest,
	} {
		assertPairStepObservedForTest(t, steps, required)
	}
	for _, step := range steps {
		if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
			t.Fatalf("wrong layout SHA reached %q; steps=%v", step, steps)
		}
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
}

func TestDefinitionPairValidatesPinnedCurrentAndCandidateBytesWhileBothLocksAreHeld(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-validation"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	validated := false
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	request.validate = func(current, candidate definitionPairState) error {
		validated = true
		if string(current.board) != string(oldBoard) || !current.layout.present || string(current.layout.raw) != string(oldLayout) {
			return fmt.Errorf("current pair = %#v, want exact canonical bytes", current)
		}
		if string(candidate.board) != string(newBoard) || !candidate.layout.present || string(candidate.layout.raw) != string(newLayout) {
			return fmt.Errorf("candidate pair = %#v, want exact supplied bytes", candidate)
		}
		assertPairMutexHeldForTest(t, store.BoardPath(slug)+".lock", "board")
		assertPairMutexHeldForTest(t, store.LayoutPath(slug)+".lock", "layout")
		return nil
	}
	request.cas = func(current definitionPairState) error {
		assertPairMutexHeldForTest(t, store.BoardPath(slug)+".lock", "board at CAS")
		assertPairMutexHeldForTest(t, store.LayoutPath(slug)+".lock", "layout at CAS")
		return nil
	}

	if err := store.publishDefinitionPair(slug, request, func(step string) error {
		assertPairMutexHeldForTest(t, store.BoardPath(slug)+".lock", "board at "+step)
		assertPairMutexHeldForTest(t, store.LayoutPath(slug)+".lock", "layout at "+step)
		return nil
	}); err != nil {
		t.Fatalf("publish validated pair: %v", err)
	}
	if !validated {
		t.Fatal("pair publication skipped validation")
	}
}

func TestDefinitionPairAcquiresBoardThenLayoutAndHoldsBothThroughPublication(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-lock-order"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	layoutHeld := make(chan struct{})
	releaseLayout := make(chan struct{})
	layoutDone := make(chan error, 1)
	go func() {
		layoutDone <- store.withLayoutDefinitionLock(slug, func(*definitionFile) error {
			close(layoutHeld)
			<-releaseLayout
			return nil
		})
	}()
	<-layoutHeld

	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	locksHeldAtPublication := make(chan [2]bool, 1)
	callbackReached := make(chan string, 1)
	publishDone := make(chan error, 1)
	go func() {
		publishDone <- store.publishDefinitionPair(slug, request, func(step string) error {
			select {
			case callbackReached <- step:
			default:
			}
			if step == pairStepPublishLayoutRenameForTest {
				locksHeldAtPublication <- [2]bool{
					pairMutexHeldForTest(store.BoardPath(slug) + ".lock"),
					pairMutexHeldForTest(store.LayoutPath(slug) + ".lock"),
				}
			}
			return nil
		})
	}()

	boardMutex := mutexFor(store.BoardPath(slug) + ".lock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if !boardMutex.TryLock() {
			break
		}
		boardMutex.Unlock()
		if time.Now().After(deadline) {
			close(releaseLayout)
			t.Fatalf("pair publication did not acquire board lock before waiting for layout: %v", <-publishDone)
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-publishDone:
		close(releaseLayout)
		t.Fatalf("pair publication bypassed held layout lock: %v", err)
	case step := <-callbackReached:
		close(releaseLayout)
		<-layoutDone
		<-publishDone
		t.Fatalf("pair publication reached %q while another goroutine owned layout lock", step)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseLayout)
	if err := <-layoutDone; err != nil {
		t.Fatalf("held layout lock: %v", err)
	}
	if err := <-publishDone; err != nil {
		t.Fatalf("publish after releasing layout lock: %v", err)
	}
	select {
	case held := <-locksHeldAtPublication:
		if !held[0] || !held[1] {
			t.Fatalf("locks held at first rename = board:%t layout:%t, want both", held[0], held[1])
		}
	default:
		t.Fatal("pair publication never reached layout rename checkpoint")
	}
}

func TestDefinitionPairDurablyPreflightsCanonicalPairBeforeIdentityCAS(t *testing.T) {
	tests := []struct {
		name           string
		oldLayout      []byte
		wantFileSync   bool
		wantParentSync bool
	}{
		{name: "present layout", oldLayout: pairLayoutFixture(1, "old"), wantFileSync: true},
		{name: "absent layout", oldLayout: nil, wantFileSync: false, wantParentSync: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-preflight"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			newBoard := pairBoardFixture(slug, 2, "New")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			if test.oldLayout != nil {
				writeFixture(t, store.LayoutPath(slug), string(test.oldLayout))
			}

			request := definitionPairRequestForTest(oldBoard, test.oldLayout, newBoard, pairAbsentContentForTest())
			request.expected.board.sha256 = etag([]byte("stale board identity"))
			validated := false
			syncBeforeValidation := false
			request.validate = func(current, candidate definitionPairState) error {
				validated = true
				return nil
			}
			var steps []string
			if err := store.publishDefinitionPair(slug, request, func(step string) error {
				if step != pairStepPreflightLayoutParentSyncForTest && strings.HasPrefix(step, "preflight:") && !validated {
					syncBeforeValidation = true
				}
				steps = append(steps, step)
				return nil
			}); !errors.Is(err, ErrConflict) {
				t.Fatalf("stale identity error = %v, want ErrConflict", err)
			}
			if !validated {
				t.Fatal("stale expected identity bypassed current/candidate validation")
			}
			if syncBeforeValidation {
				t.Fatalf("canonical preflight sync ran before current/candidate validation; steps=%v", steps)
			}

			assertPairStepObservedForTest(t, steps, pairStepPreflightBoardFileSyncForTest)
			assertPairStepObservedForTest(t, steps, pairStepPreflightBoardDirSyncForTest)
			assertPairStepObservedForTest(t, steps, pairStepPreflightLayoutDirSyncForTest)
			if got := pairStepObservedForTest(steps, pairStepPreflightLayoutFileSyncForTest); got != test.wantFileSync {
				t.Fatalf("layout file sync observed = %t, want %t; steps=%v", got, test.wantFileSync, steps)
			}
			if got := pairStepObservedForTest(steps, pairStepPreflightLayoutParentSyncForTest); got != test.wantParentSync {
				t.Fatalf("new layout-directory parent sync observed = %t, want %t; steps=%v", got, test.wantParentSync, steps)
			}
			for _, step := range steps {
				if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
					t.Fatalf("stale CAS reached %q before rejection; steps=%v", step, steps)
				}
			}
			if got := readFile(t, store.BoardPath(slug)); got != string(oldBoard) {
				t.Fatalf("stale CAS mutated board:\n%s", got)
			}
			if test.oldLayout == nil {
				if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("stale CAS materialized absent layout: %v", err)
				}
			} else if got := readFile(t, store.LayoutPath(slug)); got != string(test.oldLayout) {
				t.Fatalf("stale CAS mutated layout:\n%s", got)
			}
		})
	}
}

func TestDefinitionPairRevisionCASRunsAfterValidationAndCanonicalDurability(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-revision-cas"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	validated := false
	casCalled := false
	casBeforeDurability := false
	var steps []string
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	request.validate = func(current, candidate definitionPairState) error {
		validated = true
		return nil
	}
	request.cas = func(current definitionPairState) error {
		casCalled = true
		for _, required := range []string{
			pairStepPreflightBoardFileSyncForTest,
			pairStepPreflightLayoutFileSyncForTest,
			pairStepPreflightBoardDirSyncForTest,
			pairStepPreflightLayoutDirSyncForTest,
		} {
			if !pairStepObservedForTest(steps, required) {
				casBeforeDurability = true
			}
		}
		return ErrConflict
	}
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		if !validated {
			t.Fatalf("canonical I/O %q ran before current/candidate validation", step)
		}
		steps = append(steps, step)
		return nil
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("revision CAS error = %v, want ErrConflict", err)
	}
	if !casCalled {
		t.Fatal("pair publication omitted revision CAS after exact identity preflight")
	}
	if casBeforeDurability {
		t.Fatalf("revision CAS ran before every canonical file and parent sync; steps=%v", steps)
	}
	for _, step := range steps {
		if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
			t.Fatalf("failed revision CAS reached %q; steps=%v", step, steps)
		}
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
}

func TestDefinitionPairDurablyPublishesNewLayoutDirectoryBeforeUsingIt(t *testing.T) {
	tests := []struct {
		name     string
		failSync bool
	}{
		{name: "success"},
		{name: "parent sync failure", failSync: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-new-layout-directory"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			layoutDirectory := filepath.Dir(store.LayoutPath(slug))
			if _, err := os.Lstat(layoutDirectory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("layout directory precondition = %v, want absent", err)
			}

			injected := errors.New("new layout-directory parent sync failed")
			layoutDirectoryExistedAtSync := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, nil, newBoard, pairPresentContentForTest(newLayout))
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == pairStepPreflightLayoutParentSyncForTest {
					info, statErr := os.Stat(layoutDirectory)
					layoutDirectoryExistedAtSync = statErr == nil && info.IsDir()
					if test.failSync {
						return injected
					}
				}
				return nil
			})
			if len(steps) == 0 || steps[0] != pairStepPreflightLayoutParentSyncForTest {
				t.Fatalf("first paired I/O after lazy layout-directory creation = %v, want parent sync", steps)
			}
			if !layoutDirectoryExistedAtSync {
				t.Fatalf("parent sync hook ran before layout directory existed; steps=%v", steps)
			}
			if test.failSync {
				if !errors.Is(err, injected) {
					t.Fatalf("layout-directory parent sync error = %v, want injected error", err)
				}
				for _, step := range steps {
					if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
						t.Fatalf("layout-directory parent sync failure reached %q; steps=%v", step, steps)
					}
				}
				assertPairFilesForTest(t, store, slug, oldBoard, pairAbsentContentForTest())
				return
			}
			if err != nil {
				t.Fatalf("publish after durable layout-directory creation: %v", err)
			}
			firstStage := firstPairStepWithPrefixForTest(steps, "stage:")
			if firstStage < 0 || pairStepIndexForTest(steps, pairStepPreflightLayoutParentSyncForTest) >= firstStage {
				t.Fatalf("new layout-directory parent was not durable before staging; steps=%v", steps)
			}
			assertPairFilesForTest(t, store, slug, newBoard, pairPresentContentForTest(newLayout))
		})
	}
}

func TestDefinitionPairPreflightSyncFailureLeavesExactOldPairWithoutStaging(t *testing.T) {
	tests := []struct {
		name      string
		oldLayout []byte
		failSteps []string
	}{
		{
			name:      "present layout",
			oldLayout: pairLayoutFixture(1, "old"),
			failSteps: []string{
				pairStepPreflightBoardFileSyncForTest,
				pairStepPreflightLayoutFileSyncForTest,
				pairStepPreflightBoardDirSyncForTest,
				pairStepPreflightLayoutDirSyncForTest,
			},
		},
		{
			name:      "absent layout",
			oldLayout: nil,
			failSteps: []string{
				pairStepPreflightLayoutParentSyncForTest,
				pairStepPreflightBoardFileSyncForTest,
				pairStepPreflightBoardDirSyncForTest,
				pairStepPreflightLayoutDirSyncForTest,
			},
		},
	}
	for _, test := range tests {
		for _, failStep := range test.failSteps {
			t.Run(test.name+"/"+failStep, func(t *testing.T) {
				store := NewStore(t.TempDir())
				slug := "pair-preflight-failure"
				oldBoard := pairBoardFixture(slug, 1, "Old")
				newBoard := pairBoardFixture(slug, 2, "New")
				newLayout := pairAbsentContentForTest()
				writeFixture(t, store.BoardPath(slug), string(oldBoard))
				if test.oldLayout != nil {
					writeFixture(t, store.LayoutPath(slug), string(test.oldLayout))
					newLayout = pairPresentContentForTest(pairLayoutFixture(2, "new"))
				}

				injected := errors.New("injected canonical preflight sync failure")
				failed := false
				validated := false
				var steps []string
				request := definitionPairRequestForTest(oldBoard, test.oldLayout, newBoard, newLayout)
				request.validate = func(current, candidate definitionPairState) error {
					validated = true
					return nil
				}
				err := store.publishDefinitionPair(slug, request, func(step string) error {
					steps = append(steps, step)
					if step == failStep && !failed {
						failed = true
						return injected
					}
					return nil
				})
				if !errors.Is(err, injected) {
					t.Fatalf("preflight failure error = %v, want injected error", err)
				}
				if !failed {
					t.Fatalf("preflight failure point %q was not reached; steps=%v", failStep, steps)
				}
				if failStep != pairStepPreflightLayoutParentSyncForTest && !validated {
					t.Fatal("preflight I/O started before validating the reopened canonical pair")
				}
				for _, step := range steps {
					if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
						t.Fatalf("preflight failure reached %q; steps=%v", step, steps)
					}
				}
				layout := pairAbsentContentForTest()
				if test.oldLayout != nil {
					layout = pairPresentContentForTest(test.oldLayout)
				}
				assertPairFilesForTest(t, store, slug, oldBoard, layout)
			})
		}
	}
}

func TestDefinitionPairStagesAndSyncsEveryPresentOldAndNewRepresentationBeforeFirstRename(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-stage-order"
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
		t.Fatalf("publish fully present pair: %v", err)
	}

	firstRename := firstPairStepWithPrefixForTest(steps, "publish:")
	if firstRename < 0 {
		t.Fatalf("publication did not rename a canonical file; steps=%v", steps)
	}
	for _, representation := range [][]string{
		{
			pairStepStageOldBoardCreateForTest,
			pairStepStageOldBoardWriteForTest,
			pairStepStageOldBoardFileSyncForTest,
			pairStepStageOldBoardCloseForTest,
		},
		{
			pairStepStageOldLayoutCreateForTest,
			pairStepStageOldLayoutWriteForTest,
			pairStepStageOldLayoutFileSyncForTest,
			pairStepStageOldLayoutCloseForTest,
		},
		{
			pairStepStageNewBoardCreateForTest,
			pairStepStageNewBoardWriteForTest,
			pairStepStageNewBoardFileSyncForTest,
			pairStepStageNewBoardCloseForTest,
		},
		{
			pairStepStageNewLayoutCreateForTest,
			pairStepStageNewLayoutWriteForTest,
			pairStepStageNewLayoutFileSyncForTest,
			pairStepStageNewLayoutCloseForTest,
		},
	} {
		previous := -1
		for _, required := range representation {
			index := pairStepIndexAfterForTest(steps, required, previous+1)
			if index < 0 || index >= firstRename {
				t.Fatalf("required stage operation %q index=%d, first rename index=%d; steps=%v", required, index, firstRename, steps)
			}
			previous = index
		}
	}
	if steps[firstRename] != pairStepPublishLayoutRenameForTest {
		t.Fatalf("first canonical mutation = %q, want durable layout-first rename; steps=%v", steps[firstRename], steps)
	}
}

func TestDefinitionPairSkipsAbsentRepresentationsWithoutInventingEmptyStages(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-absent-stage"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	newBoard := pairBoardFixture(slug, 2, "New")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))

	var steps []string
	request := definitionPairRequestForTest(oldBoard, nil, newBoard, pairAbsentContentForTest())
	if err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		return nil
	}); err != nil {
		t.Fatalf("publish pair with absent old and new layout: %v", err)
	}
	assertPairStepObservedForTest(t, steps, pairStepStageOldBoardFileSyncForTest)
	assertPairStepObservedForTest(t, steps, pairStepStageNewBoardFileSyncForTest)
	for _, forbidden := range []string{
		pairStepStageOldLayoutCreateForTest,
		pairStepStageOldLayoutWriteForTest,
		pairStepStageOldLayoutFileSyncForTest,
		pairStepStageOldLayoutCloseForTest,
		pairStepStageNewLayoutCreateForTest,
		pairStepStageNewLayoutWriteForTest,
		pairStepStageNewLayoutFileSyncForTest,
		pairStepStageNewLayoutCloseForTest,
	} {
		if pairStepObservedForTest(steps, forbidden) {
			t.Fatalf("absent layout produced stage sync %q; steps=%v", forbidden, steps)
		}
	}
	assertPairStepObservedForTest(t, steps, pairStepPublishLayoutAbsentForTest)
	if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent candidate layout became a file: %v", err)
	}
}

func TestDefinitionPairStageCreateWriteSyncAndCloseFailuresLeaveCanonicalPairByteIdentical(t *testing.T) {
	type stageFailure struct {
		name string
		step string
		err  error
	}
	var failures []stageFailure
	for _, representation := range []struct {
		name                       string
		create, write, sync, close string
	}{
		{name: "old board", create: pairStepStageOldBoardCreateForTest, write: pairStepStageOldBoardWriteForTest, sync: pairStepStageOldBoardFileSyncForTest, close: pairStepStageOldBoardCloseForTest},
		{name: "old layout", create: pairStepStageOldLayoutCreateForTest, write: pairStepStageOldLayoutWriteForTest, sync: pairStepStageOldLayoutFileSyncForTest, close: pairStepStageOldLayoutCloseForTest},
		{name: "new board", create: pairStepStageNewBoardCreateForTest, write: pairStepStageNewBoardWriteForTest, sync: pairStepStageNewBoardFileSyncForTest, close: pairStepStageNewBoardCloseForTest},
		{name: "new layout", create: pairStepStageNewLayoutCreateForTest, write: pairStepStageNewLayoutWriteForTest, sync: pairStepStageNewLayoutFileSyncForTest, close: pairStepStageNewLayoutCloseForTest},
	} {
		failures = append(failures,
			stageFailure{name: representation.name + "/create", step: representation.create, err: errors.New("injected stage create failure")},
			stageFailure{name: representation.name + "/write", step: representation.write, err: errors.New("injected stage write failure")},
			stageFailure{name: representation.name + "/short write", step: representation.write, err: io.ErrShortWrite},
			stageFailure{name: representation.name + "/file sync", step: representation.sync, err: errors.New("injected stage sync failure")},
			stageFailure{name: representation.name + "/close", step: representation.close, err: errors.New("injected stage close failure")},
		)
	}
	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			slug := "pair-stage-failure"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			failed := false
			var steps []string
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				if step == failure.step && !failed {
					failed = true
					return failure.err
				}
				return nil
			})
			if !errors.Is(err, failure.err) {
				t.Fatalf("stage failure error = %v, want %v", err, failure.err)
			}
			if !failed {
				t.Fatalf("stage failure point %q was not reached; steps=%v", failure.step, steps)
			}
			for _, step := range steps {
				if strings.HasPrefix(step, "publish:") {
					t.Fatalf("pre-rename failure reached canonical mutation %q; steps=%v", step, steps)
				}
			}
			assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
		})
	}
}

func TestDefinitionPairValidationFailureCannotReachStagingOrCanonicalMutation(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-validation-failure"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	injected := errors.New("candidate pair rejected")
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	request.validate = func(current, candidate definitionPairState) error { return injected }
	var steps []string
	if err := store.publishDefinitionPair(slug, request, func(step string) error {
		steps = append(steps, step)
		return nil
	}); !errors.Is(err, injected) {
		t.Fatalf("validation failure error = %v, want injected error", err)
	}
	for _, step := range steps {
		if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
			t.Fatalf("validation failure reached %q; steps=%v", step, steps)
		}
	}
	assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
}

func TestDefinitionPairCanonicalPreflightReusesNoFollowSingleLinkSecurity(t *testing.T) {
	tests := []struct {
		name   string
		member string
		attack string
	}{
		{name: "board symlink", member: "board", attack: "symlink"},
		{name: "board hardlink", member: "board", attack: "hardlink"},
		{name: "layout symlink", member: "layout", attack: "symlink"},
		{name: "layout hardlink", member: "layout", attack: "hardlink"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(filepath.Join(root, "workspace"))
			slug := "pair-security"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			canonical := store.BoardPath(slug)
			privateRaw := oldBoard
			if test.member == "layout" {
				canonical = store.LayoutPath(slug)
				privateRaw = oldLayout
			}
			if err := os.Remove(canonical); err != nil {
				t.Fatalf("remove canonical %s: %v", test.member, err)
			}
			victim := filepath.Join(root, "host-private-"+test.member)
			if err := os.WriteFile(victim, privateRaw, 0o600); err != nil {
				t.Fatalf("write private %s: %v", test.member, err)
			}
			switch test.attack {
			case "symlink":
				if err := os.Symlink(victim, canonical); err != nil {
					t.Fatalf("symlink private %s: %v", test.member, err)
				}
			case "hardlink":
				if err := os.Link(victim, canonical); err != nil {
					t.Fatalf("hardlink private %s: %v", test.member, err)
				}
			}

			var steps []string
			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			if err := store.publishDefinitionPair(slug, request, func(step string) error {
				steps = append(steps, step)
				return nil
			}); err == nil {
				t.Fatalf("pair publication accepted %s %s", test.member, test.attack)
			}
			for _, step := range steps {
				if strings.HasPrefix(step, "stage:") || strings.HasPrefix(step, "publish:") {
					t.Fatalf("rejected %s reached %q; steps=%v", test.attack, step, steps)
				}
			}
			if got := readFile(t, victim); got != string(privateRaw) {
				t.Fatalf("rejected %s mutated private %s bytes: %q", test.attack, test.member, got)
			}
			info, err := os.Lstat(canonical)
			if err != nil {
				t.Fatalf("lstat rejected canonical: %v", err)
			}
			if test.attack == "symlink" && info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("rejected symlink was replaced with mode %v", info.Mode())
			}
			if test.attack == "hardlink" {
				victimInfo, err := os.Stat(victim)
				if err != nil {
					t.Fatalf("stat private target: %v", err)
				}
				canonicalInfo, err := os.Stat(canonical)
				if err != nil {
					t.Fatalf("stat canonical hardlink: %v", err)
				}
				if !os.SameFile(victimInfo, canonicalInfo) {
					t.Fatal("rejected hardlink binding was replaced")
				}
			}
		})
	}
}

func TestDefinitionPairLocksReuseNoFollowSingleLinkSecurity(t *testing.T) {
	tests := []struct {
		name   string
		member string
		attack string
	}{
		{name: "board lock symlink", member: "board", attack: "symlink"},
		{name: "board lock hardlink", member: "board", attack: "hardlink"},
		{name: "layout lock symlink", member: "layout", attack: "symlink"},
		{name: "layout lock hardlink", member: "layout", attack: "hardlink"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(filepath.Join(root, "workspace"))
			slug := "pair-lock-security"
			oldBoard := pairBoardFixture(slug, 1, "Old")
			oldLayout := pairLayoutFixture(1, "old")
			newBoard := pairBoardFixture(slug, 2, "New")
			newLayout := pairLayoutFixture(2, "new")
			writeFixture(t, store.BoardPath(slug), string(oldBoard))
			writeFixture(t, store.LayoutPath(slug), string(oldLayout))

			lockPath := store.BoardPath(slug) + ".lock"
			if test.member == "layout" {
				lockPath = store.LayoutPath(slug) + ".lock"
			}
			victim := filepath.Join(root, "host-private-"+test.member+"-lock")
			victimRaw := "private lock authority\n"
			if err := os.WriteFile(victim, []byte(victimRaw), 0o600); err != nil {
				t.Fatalf("write private lock: %v", err)
			}
			switch test.attack {
			case "symlink":
				if err := os.Symlink(victim, lockPath); err != nil {
					t.Fatalf("symlink private lock: %v", err)
				}
			case "hardlink":
				if err := os.Link(victim, lockPath); err != nil {
					t.Fatalf("hardlink private lock: %v", err)
				}
			}

			request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
			if err := store.publishDefinitionPair(slug, request, nil); err == nil {
				t.Fatalf("pair publication accepted %s", test.name)
			}
			assertPairFilesForTest(t, store, slug, oldBoard, pairPresentContentForTest(oldLayout))
			if got := readFile(t, victim); got != victimRaw {
				t.Fatalf("rejected lock substitution mutated private bytes: %q", got)
			}
			victimInfo, err := os.Stat(victim)
			if err != nil {
				t.Fatalf("stat private lock: %v", err)
			}
			if got := victimInfo.Mode().Perm(); got != 0o600 {
				t.Fatalf("rejected lock substitution changed private mode to %04o", got)
			}
			linkInfo, err := os.Lstat(lockPath)
			if err != nil {
				t.Fatalf("lstat rejected lock binding: %v", err)
			}
			if test.attack == "symlink" && linkInfo.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("rejected lock symlink was replaced with mode %v", linkInfo.Mode())
			}
			if test.attack == "hardlink" {
				lockInfo, err := os.Stat(lockPath)
				if err != nil {
					t.Fatalf("stat rejected lock hardlink: %v", err)
				}
				if !os.SameFile(victimInfo, lockInfo) {
					t.Fatal("rejected lock hardlink was replaced")
				}
			}
		})
	}
}

func TestDefinitionPairHoldsBothAdvisoryFlocksAgainstPeerProcessThroughFinalBoardDurability(t *testing.T) {
	store := NewStore(t.TempDir())
	slug := "pair-flock-continuity"
	oldBoard := pairBoardFixture(slug, 1, "Old")
	oldLayout := pairLayoutFixture(1, "old")
	newBoard := pairBoardFixture(slug, 2, "New")
	newLayout := pairLayoutFixture(2, "new")
	writeFixture(t, store.BoardPath(slug), string(oldBoard))
	writeFixture(t, store.LayoutPath(slug), string(oldLayout))

	probedFinalDurability := false
	request := definitionPairRequestForTest(oldBoard, oldLayout, newBoard, pairPresentContentForTest(newLayout))
	err := store.publishDefinitionPair(slug, request, func(step string) error {
		if step == pairStepPublishBoardDirSyncForTest {
			probedFinalDurability = true
			assertPeerProcessDefinitionFlocksBlockedForTest(
				t,
				store.BoardPath(slug)+".lock",
				store.LayoutPath(slug)+".lock",
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("publish while probing advisory locks: %v", err)
	}
	if !probedFinalDurability {
		t.Fatal("publication omitted final board-directory durability checkpoint")
	}
}

func TestDefinitionPairPeerProcessFlockProbe(t *testing.T) {
	if os.Getenv("CHROTE_TEST_DEFINITION_PAIR_FLOCK_PROBE") != "1" {
		return
	}
	for _, member := range []struct {
		name string
		path string
	}{
		{name: "board", path: os.Getenv("CHROTE_TEST_DEFINITION_PAIR_BOARD_LOCK")},
		{name: "layout", path: os.Getenv("CHROTE_TEST_DEFINITION_PAIR_LAYOUT_LOCK")},
	} {
		fd, err := syscall.Open(member.path, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			t.Fatalf("open peer-process %s lock: %v", member.name, err)
		}
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			_ = syscall.Flock(fd, syscall.LOCK_UN)
			_ = syscall.Close(fd)
			t.Fatalf("peer process acquired %s flock before paired publication returned", member.name)
		}
		_ = syscall.Close(fd)
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			t.Fatalf("peer-process %s flock error = %v, want would-block", member.name, err)
		}
	}
}

func definitionPairRequestForTest(oldBoard, oldLayout, newBoard []byte, newLayout definitionPairContent) definitionPairPublicationRequest {
	request := definitionPairPublicationRequest{
		expected: definitionPairStateIdentity{
			board: definitionPairIdentity{present: true, sha256: etag(oldBoard)},
			layout: definitionPairIdentity{
				present: oldLayout != nil,
			},
		},
		candidate: definitionPairState{
			board:  append([]byte(nil), newBoard...),
			layout: newLayout,
		},
		validate: func(current, candidate definitionPairState) error { return nil },
		cas:      func(current definitionPairState) error { return nil },
	}
	if oldLayout != nil {
		request.expected.layout.sha256 = etag(oldLayout)
	}
	return request
}

func pairPresentContentForTest(raw []byte) definitionPairContent {
	return definitionPairContent{present: true, raw: append([]byte(nil), raw...)}
}

func pairAbsentContentForTest() definitionPairContent {
	return definitionPairContent{}
}

func pairBoardFixture(slug string, rev int, title string) []byte {
	return []byte(fmt.Sprintf("schema = 2\nid = \"brd_pair\"\nslug = %q\ntitle = %q\nrev = %d\n", slug, title, rev))
}

func pairLayoutFixture(boardRev int, marker string) []byte {
	return []byte(fmt.Sprintf("schema = 1\nboardId = \"brd_pair\"\nboardRev = %d\nupdatedAt = %q\n", boardRev, marker))
}

func assertPairFilesForTest(t *testing.T, store *Store, slug string, board []byte, layout definitionPairContent) {
	t.Helper()
	if got := readFile(t, store.BoardPath(slug)); got != string(board) {
		t.Fatalf("canonical board = %q, want %q", got, board)
	}
	if layout.present {
		if got := readFile(t, store.LayoutPath(slug)); got != string(layout.raw) {
			t.Fatalf("canonical layout = %q, want %q", got, layout.raw)
		}
		return
	}
	if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical layout presence error = %v, want absent", err)
	}
}

func assertPairStepObservedForTest(t *testing.T, steps []string, want string) {
	t.Helper()
	if !pairStepObservedForTest(steps, want) {
		t.Fatalf("publication steps omitted %q: %v", want, steps)
	}
}

func pairStepObservedForTest(steps []string, want string) bool {
	return pairStepIndexForTest(steps, want) >= 0
}

func pairStepIndexForTest(steps []string, want string) int {
	for index, step := range steps {
		if step == want {
			return index
		}
	}
	return -1
}

func firstPairStepWithPrefixForTest(steps []string, prefix string) int {
	for index, step := range steps {
		if strings.HasPrefix(step, prefix) {
			return index
		}
	}
	return -1
}

func assertPairMutexHeldForTest(t *testing.T, lockPath, member string) {
	t.Helper()
	if !pairMutexHeldForTest(lockPath) {
		t.Fatalf("%s mutex was not held during pair validation", member)
	}
}

func pairMutexHeldForTest(lockPath string) bool {
	mutex := mutexFor(lockPath)
	if mutex.TryLock() {
		mutex.Unlock()
		return false
	}
	return true
}

func assertPeerProcessDefinitionFlocksBlockedForTest(t *testing.T, boardLock, layoutLock string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestDefinitionPairPeerProcessFlockProbe$")
	command.Env = append(os.Environ(),
		"CHROTE_TEST_DEFINITION_PAIR_FLOCK_PROBE=1",
		"CHROTE_TEST_DEFINITION_PAIR_BOARD_LOCK="+boardLock,
		"CHROTE_TEST_DEFINITION_PAIR_LAYOUT_LOCK="+layoutLock,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("peer-process definition flock probe: %v\n%s", err, output)
	}
}
