package formations

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type createOutcome struct {
	nodeID string
	err    error
}

// createNodeCase drives one board-node creation path (formation, gate, or
// mission) both through the public API and through the unexported fault seam so
// the atomicity guarantees can be exercised uniformly.
type createNodeCase struct {
	name            string
	create          func(store *Store, slug string, opts WriteOptions) createOutcome
	createWithFault func(store *Store, slug string, opts WriteOptions, fault func(string) error) createOutcome
	nodes           func(board *BoardDocument) []string
}

func createNodeCases() []createNodeCase {
	formationNodes := func(board *BoardDocument) []string {
		ids := make([]string, 0, len(board.Formations))
		for _, formation := range board.Formations {
			ids = append(ids, formation.ID)
		}
		return ids
	}
	gateNodes := func(board *BoardDocument) []string {
		ids := make([]string, 0, len(board.Gates))
		for _, gate := range board.Gates {
			ids = append(ids, gate.ID)
		}
		return ids
	}
	missionNodes := func(board *BoardDocument) []string {
		ids := make([]string, 0, len(board.Missions))
		for _, mission := range board.Missions {
			ids = append(ids, mission.ID)
		}
		return ids
	}
	return []createNodeCase{
		{
			name: "formation",
			create: func(store *Store, slug string, opts WriteOptions) createOutcome {
				result, err := store.CreateFormation(slug, FormationCreateRequest{
					Type: FormationTypePeer, Title: "Research huddle", X: 840, Y: 135, UpdatedBy: "agent:test",
				}, opts)
				if err != nil {
					return createOutcome{err: err}
				}
				return createOutcome{nodeID: result.Formation.ID}
			},
			createWithFault: func(store *Store, slug string, opts WriteOptions, fault func(string) error) createOutcome {
				result, err := store.createFormation(slug, FormationCreateRequest{
					Type: FormationTypePeer, Title: "Research huddle", X: 840, Y: 135, UpdatedBy: "agent:test",
				}, opts, fault)
				if err != nil {
					return createOutcome{err: err}
				}
				return createOutcome{nodeID: result.Formation.ID}
			},
			nodes: formationNodes,
		},
		{
			name: "gate",
			create: func(store *Store, slug string, opts WriteOptions) createOutcome {
				result, err := store.CreateGate(slug, GateCreateRequest{
					Title: "Review gate", Kinds: []string{"code"}, Criterion: "Check it.", X: 448, Y: 280, UpdatedBy: "agent:test",
				}, opts)
				if err != nil {
					return createOutcome{err: err}
				}
				return createOutcome{nodeID: result.Gate.ID}
			},
			createWithFault: func(store *Store, slug string, opts WriteOptions, fault func(string) error) createOutcome {
				result, err := store.createGate(slug, GateCreateRequest{
					Title: "Review gate", Kinds: []string{"code"}, Criterion: "Check it.", X: 448, Y: 280, UpdatedBy: "agent:test",
				}, opts, fault)
				if err != nil {
					return createOutcome{err: err}
				}
				return createOutcome{nodeID: result.Gate.ID}
			},
			nodes: gateNodes,
		},
		{
			name: "mission",
			create: func(store *Store, slug string, opts WriteOptions) createOutcome {
				result, err := store.CreateMission(slug, MissionCreateRequest{
					Title: "Mission", Goal: "Ship it", BeadID: "home-7kc4.5", X: 500, Y: 100, UpdatedBy: "agent:test",
				}, opts)
				if err != nil {
					return createOutcome{err: err}
				}
				return createOutcome{nodeID: result.Mission.ID}
			},
			createWithFault: func(store *Store, slug string, opts WriteOptions, fault func(string) error) createOutcome {
				result, err := store.createMission(slug, MissionCreateRequest{
					Title: "Mission", Goal: "Ship it", BeadID: "home-7kc4.5", X: 500, Y: 100, UpdatedBy: "agent:test",
				}, opts, fault)
				if err != nil {
					return createOutcome{err: err}
				}
				return createOutcome{nodeID: result.Mission.ID}
			},
			nodes: missionNodes,
		},
	}
}

func seedCreateBoard(t *testing.T) (*Store, string, WriteOptions, *BoardDocument) {
	t.Helper()
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	slug := "session-search"
	writeFixture(t, store.BoardPath(slug), minimalBoard(slug, 7))
	before, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	return store, slug, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}, before
}

// TestCreateNodeLayoutFailureLeavesNoPartialDefinition asserts that when the
// layout sidecar write fails during node creation, the create has ONE
// unambiguous durable outcome: no new node in the authoritative board
// definition. A failed create must never leave a board node the caller cannot
// see (an opaque cross-file partial commit), and a blind retry carrying the
// original preconditions must succeed and create exactly one node rather than a
// duplicate or an unrecoverable conflict against durable-but-invisible state.
func TestCreateNodeLayoutFailureLeavesNoPartialDefinition(t *testing.T) {
	for _, tc := range createNodeCases() {
		t.Run(tc.name, func(t *testing.T) {
			store, slug, opts, before := seedCreateBoard(t)

			// Force the layout sidecar write to fail by making the layout
			// definition directory unopenable while the board directory stays
			// writable. This injects a sidecar failure at the exact seam where
			// the board definition would otherwise already be durable.
			layoutDir := filepath.Dir(store.LayoutPath(slug))
			if err := os.MkdirAll(layoutDir, 0o2770); err != nil {
				t.Fatalf("prepare layout directory: %v", err)
			}
			if err := os.Chmod(layoutDir, 0); err != nil {
				t.Fatalf("make layout directory unopenable: %v", err)
			}
			restored := false
			defer func() {
				if !restored {
					_ = os.Chmod(layoutDir, 0o0770)
				}
			}()

			failed := tc.create(store, slug, opts)

			if err := os.Chmod(layoutDir, 0o0770); err != nil {
				t.Fatalf("restore layout directory: %v", err)
			}
			restored = true

			if failed.err == nil {
				t.Fatalf("create unexpectedly succeeded despite a failed layout sidecar write (node %q)", failed.nodeID)
			}

			// The authoritative board definition must carry no new node: a
			// failed create has exactly one durable outcome, and it is "no new
			// node". Anything else is an opaque cross-file partial commit.
			afterFailure, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("reread board after failed create: %v", err)
			}
			if ids := tc.nodes(afterFailure); len(ids) != 0 {
				t.Fatalf("failed create left %d durable node(s) %v in the board definition; want none", len(ids), ids)
			}
			if afterFailure.Rev != before.Rev {
				t.Fatalf("failed create advanced board rev to %d; want %d unchanged", afterFailure.Rev, before.Rev)
			}
			if afterFailure.ETag != before.ETag {
				t.Fatalf("failed create changed the board etag; want the definition untouched")
			}

			// A blind retry carrying the ORIGINAL preconditions must succeed and
			// create exactly one node — never a duplicate and never an
			// unrecoverable conflict against durable-but-invisible state.
			retried := tc.create(store, slug, opts)
			if retried.err != nil {
				t.Fatalf("blind retry with original preconditions failed: %v", retried.err)
			}
			afterRetry, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("reread board after retry: %v", err)
			}
			ids := tc.nodes(afterRetry)
			if len(ids) != 1 || ids[0] != retried.nodeID {
				t.Fatalf("after retry board nodes = %v, want exactly the retried node %q", ids, retried.nodeID)
			}
			if afterRetry.Rev != before.Rev+1 {
				t.Fatalf("after retry board rev = %d, want %d", afterRetry.Rev, before.Rev+1)
			}
		})
	}
}

// TestCreateNodeUnrecoverableBoardPublishFaultIsRecoverable drives an
// unrecoverable fault at every board publication/reconciliation step (so the
// board content can never be committed even after reconciliation) and asserts
// the create surfaces the explicit ErrDefinitionPublicationUncertain result --
// the recoverable partial-commit outcome carrying a "reload both board and
// layout" reconciliation path -- while leaving no phantom node in the board
// definition. A subsequent fault-free retry with the original preconditions
// must still create exactly one board node.
func TestCreateNodeUnrecoverableBoardPublishFaultIsRecoverable(t *testing.T) {
	for _, tc := range createNodeCases() {
		t.Run(tc.name, func(t *testing.T) {
			store, slug, opts, before := seedCreateBoard(t)

			// Fault every board publish/reconcile step (never preflight or
			// temp-file staging), so the layout half of the pair commits but the
			// board half can neither publish nor be reconciled to a matching
			// state -- forcing the uncertain reconciliation outcome.
			boardPublishFault := func(step string) error {
				if (strings.HasPrefix(step, "publish:") || strings.HasPrefix(step, "reconcile:")) && strings.Contains(step, "board") {
					return errors.New("injected board publication fault")
				}
				return nil
			}

			failed := tc.createWithFault(store, slug, opts, boardPublishFault)
			if !errors.Is(failed.err, ErrDefinitionPublicationUncertain) {
				t.Fatalf("create error = %v, want ErrDefinitionPublicationUncertain", failed.err)
			}

			// No phantom: the authoritative board definition never gained the
			// node and its revision did not advance.
			afterFailure, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("reread board after uncertain create: %v", err)
			}
			if ids := tc.nodes(afterFailure); len(ids) != 0 {
				t.Fatalf("uncertain create left %d durable node(s) %v in the board definition; want none", len(ids), ids)
			}
			if afterFailure.Rev != before.Rev {
				t.Fatalf("uncertain create advanced board rev to %d; want %d unchanged", afterFailure.Rev, before.Rev)
			}

			// The reconciliation path (reload both, then retry) must succeed and
			// create exactly one board node.
			retried := tc.create(store, slug, opts)
			if retried.err != nil {
				t.Fatalf("retry after reload failed: %v", retried.err)
			}
			afterRetry, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("reread board after retry: %v", err)
			}
			ids := tc.nodes(afterRetry)
			if len(ids) != 1 || ids[0] != retried.nodeID {
				t.Fatalf("after retry board nodes = %v, want exactly the retried node %q", ids, retried.nodeID)
			}
		})
	}
}
