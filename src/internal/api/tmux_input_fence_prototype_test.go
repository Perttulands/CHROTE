package api

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestSamePoolInputFencePrototypePreservesResolverLineageAndDurableBarriers(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"shared-agent"},
		"bob":   {"observer"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)
	terminal := newPrototypeTerminalConsumer(fixture, resolver)
	formations := newPrototypeFormationsConsumer(resolver)

	var terminalTarget, formationsTarget prototypeResolvedTarget
	for unixUser, sessionName := range map[string]string{"alice": "shared-agent", "bob": "observer"} {
		var err error
		terminalTarget, err = terminal.Resolve(context.Background(), unixUser, sessionName)
		if err != nil {
			t.Fatalf("resolve Terminal target %s/%s: %v", unixUser, sessionName, err)
		}
		formationsTarget, err = formations.Resolve(context.Background(), sessionName)
		if err != nil {
			t.Fatalf("resolve Formations target %s/%s: %v", unixUser, sessionName, err)
		}
		if terminalTarget.Observation != formationsTarget.Observation {
			t.Fatalf("consumer observations differ for %s/%s: Terminal=%+v Formations=%+v", unixUser, sessionName, terminalTarget.Observation, formationsTarget.Observation)
		}
		if terminalTarget.Observation.UnixUser != unixUser {
			t.Fatalf("Terminal resolver source = %q, want %q", terminalTarget.Observation.UnixUser, unixUser)
		}
	}
	terminalTarget, err := terminal.Resolve(context.Background(), "alice", "shared-agent")
	if err != nil {
		t.Fatalf("resolve dispatch Terminal target: %v", err)
	}
	formationsTarget, err = formations.Resolve(context.Background(), "shared-agent")
	if err != nil {
		t.Fatalf("resolve dispatch Formations target: %v", err)
	}

	fence, err := openSamePoolInputFencePrototype(fixture.journalPath, formationsTarget)
	if err != nil {
		t.Fatalf("open prototype fence: %v", err)
	}
	if err := fence.AcquireOccupancy(fence.Registration(), "occupancy_1"); err != nil {
		t.Fatalf("acquire prototype occupancy: %v", err)
	}
	permit := prototypeDispatchPermit{
		ID:          "permit_dispatch_1",
		Observation: formationsTarget.Observation,
	}
	const dispatchMarker = "FORMATIONS_PROTOTYPE_DISPATCH"
	if err := fence.Dispatch(context.Background(), permit, dispatchMarker+"\n"); err != nil {
		t.Fatalf("dispatch through prototype fence: %v", err)
	}
	fixture.waitForPaneInput(t, formationsTarget, dispatchMarker)

	peek := prototypePeekPermit{
		RecordID:    "peek_generation_1",
		Generation:  1,
		Credential:  "peek_secret_1_must_not_be_journaled",
		Observation: terminalTarget.Observation,
	}
	if err := fence.OpenPeekGeneration(peek, "occupancy_1"); err != nil {
		t.Fatalf("open prototype Peek generation: %v", err)
	}
	const acceptedPeekMarker = "TERMINAL_INPUT_BEFORE_CLOSURE"
	if err := fence.SendPeek(context.Background(), peek, acceptedPeekMarker+"\n"); err != nil {
		t.Fatalf("send pre-closure Terminal input: %v", err)
	}
	fixture.waitForPaneInput(t, terminalTarget, acceptedPeekMarker)
	if err := fence.ClosePeekGeneration(peek, "peek_generation_1_close"); err != nil {
		t.Fatalf("close prototype Peek generation 1: %v", err)
	}
	latestPeek := prototypePeekPermit{
		RecordID:    "peek_generation_2",
		Generation:  2,
		Credential:  "peek_secret_2_must_not_be_journaled",
		Observation: terminalTarget.Observation,
	}
	if err := fence.OpenPeekGeneration(latestPeek, "peek_generation_1_close"); err != nil {
		t.Fatalf("open prototype Peek generation 2: %v", err)
	}
	const supersededMarker = "SUPERSEDED_PEEK_GENERATION_MUST_NOT_SEND"
	if err := fence.SendPeek(context.Background(), peek, supersededMarker+"\n"); err == nil {
		t.Fatal("superseded Peek generation 1 input unexpectedly succeeded")
	}
	fixture.assertPaneInputAbsent(t, terminalTarget, supersededMarker)
	const latestPeekMarker = "LATEST_PEEK_GENERATION_INPUT"
	if err := fence.SendPeek(context.Background(), latestPeek, latestPeekMarker+"\n"); err != nil {
		t.Fatalf("send latest Peek generation input: %v", err)
	}
	fixture.waitForPaneInput(t, terminalTarget, latestPeekMarker)
	if err := fence.ClosePeekGeneration(latestPeek, "peek_generation_2_close"); err != nil {
		t.Fatalf("close prototype Peek generation 2: %v", err)
	}
	const replayedMarker = "REPLAYED_PEEK_GENERATION_MUST_NOT_SEND"
	if err := fence.OpenPeekGeneration(peek, "peek_generation_2_close"); err == nil {
		t.Fatal("replayed Peek generation 1 unexpectedly reopened after generation 2")
	}
	if err := fence.SendPeek(context.Background(), peek, replayedMarker+"\n"); err == nil {
		t.Fatal("replayed Peek generation 1 input unexpectedly succeeded")
	}
	fixture.assertPaneInputAbsent(t, terminalTarget, replayedMarker)

	if err := fence.Close("closure_1"); err != nil {
		t.Fatalf("close prototype fence: %v", err)
	}
	checkpoint := fence.Checkpoint()
	recovered, err := recoverSamePoolInputFencePrototype(fixture.journalPath, terminalTarget, checkpoint)
	if err != nil {
		t.Fatalf("recover closed prototype fence: %v", err)
	}
	if !recovered.Closed() {
		t.Fatal("recovered prototype fence is open after durable closure barrier")
	}

	const rejectedMarker = "TERMINAL_INPUT_AFTER_CLOSURE_MUST_NOT_SEND"
	if err := recovered.SendPeek(context.Background(), peek, rejectedMarker+"\n"); !errors.Is(err, errPrototypeFenceClosed) {
		t.Fatalf("post-closure Terminal input error = %v, want %v", err, errPrototypeFenceClosed)
	}
	fixture.assertPaneInputAbsent(t, terminalTarget, rejectedMarker)

	raw, err := os.ReadFile(fixture.journalPath)
	if err != nil {
		t.Fatalf("read prototype journal: %v", err)
	}
	journal := string(raw)
	for _, want := range []string{
		`"kind":"genesis"`,
		`"kind":"dispatch_intent"`,
		`"kind":"dispatch_effect"`,
		`"kind":"dispatch_barrier"`,
		`"kind":"peek_generation_open_barrier"`,
		`"kind":"peek_generation_close_intent"`,
		`"kind":"peek_generation_close_barrier"`,
		`"kind":"peek_input_intent"`,
		`"kind":"peek_input_effect"`,
		`"kind":"peek_input_barrier"`,
		`"kind":"closure_intent"`,
		`"kind":"closure_barrier"`,
	} {
		if !strings.Contains(journal, want) {
			t.Fatalf("prototype journal missing %s:\n%s", want, journal)
		}
	}
	if strings.Contains(journal, dispatchMarker) || strings.Contains(journal, acceptedPeekMarker) || strings.Contains(journal, supersededMarker) || strings.Contains(journal, latestPeekMarker) || strings.Contains(journal, replayedMarker) || strings.Contains(journal, rejectedMarker) || strings.Contains(journal, peek.Credential) || strings.Contains(journal, latestPeek.Credential) {
		t.Fatalf("prototype journal persisted input content:\n%s", journal)
	}
}

func TestSamePoolInputFencePrototypeFailsClosedOnAmbiguousMultiSourceLineage(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"duplicate-agent"},
		"bob":   {"duplicate-agent"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)

	_, err := resolver.Resolve(context.Background(), prototypeFormationsConsumer, "duplicate-agent")
	if !errors.Is(err, errPrototypeTargetAmbiguous) {
		t.Fatalf("ambiguous target error = %v, want %v", err, errPrototypeTargetAmbiguous)
	}
	if _, statErr := os.Stat(fixture.journalPath); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous resolution created journal state: %v", statErr)
	}
}

func TestSamePoolInputFencePrototypeRecoveryBlocksIncompleteDispatch(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"shared-agent"},
		"bob":   {"observer"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)
	target, err := resolver.Resolve(context.Background(), prototypeFormationsConsumer, "shared-agent")
	if err != nil {
		t.Fatalf("resolve Formations target: %v", err)
	}
	fence, err := openSamePoolInputFencePrototype(fixture.journalPath, target)
	if err != nil {
		t.Fatalf("open prototype fence: %v", err)
	}
	if err := fence.AcquireOccupancy(fence.Registration(), "occupancy_uncertain"); err != nil {
		t.Fatalf("acquire prototype occupancy: %v", err)
	}
	if err := fence.appendEvent(prototypeJournalEvent{Kind: "dispatch_intent", PermitID: "permit_uncertain"}); err != nil {
		t.Fatalf("append incomplete dispatch intent: %v", err)
	}

	recovered, err := recoverSamePoolInputFencePrototype(fixture.journalPath, target, fence.Checkpoint())
	if err != nil {
		t.Fatalf("recover incomplete prototype fence: %v", err)
	}
	if !recovered.Blocked() {
		t.Fatal("recovered prototype fence authorized after an intent without effect and barrier")
	}
	permit := prototypeDispatchPermit{ID: "permit_after_uncertainty", Observation: target.Observation}
	if err := recovered.Dispatch(context.Background(), permit, "MUST_NOT_SEND\n"); !errors.Is(err, errPrototypeFenceBlocked) {
		t.Fatalf("blocked recovery dispatch error = %v, want %v", err, errPrototypeFenceBlocked)
	}
	fixture.assertPaneInputAbsent(t, target, "MUST_NOT_SEND")
}

func TestSamePoolInputFencePrototypeBlocksSameProcessAfterPostIntentFailure(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"shared-agent"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)
	target, err := resolver.Resolve(context.Background(), prototypeFormationsConsumer, "shared-agent")
	if err != nil {
		t.Fatalf("resolve Formations target: %v", err)
	}
	fence, err := openSamePoolInputFencePrototype(fixture.journalPath, target)
	if err != nil {
		t.Fatalf("open prototype fence: %v", err)
	}
	if err := fence.AcquireOccupancy(fence.Registration(), "occupancy_live_failure"); err != nil {
		t.Fatalf("acquire prototype occupancy: %v", err)
	}

	brokenTarget := target
	brokenTarget.tmux.socket = fixture.journalPath + ".missing-socket"
	fence.target = brokenTarget
	permit := prototypeDispatchPermit{ID: "permit_uncertain_live", Observation: target.Observation}
	if err := fence.Dispatch(context.Background(), permit, "MUST_NOT_SEND\n"); err == nil {
		t.Fatal("dispatch through missing socket unexpectedly succeeded")
	}
	if !fence.Blocked() {
		t.Fatal("same-process fence remained authorizing after durable intent and effect-path failure")
	}
	fixture.assertPaneInputAbsent(t, target, "MUST_NOT_SEND")
}

func TestSamePoolInputFencePrototypeSerializesConcurrentDuplicateDispatch(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"shared-agent"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)
	target, err := resolver.Resolve(context.Background(), prototypeFormationsConsumer, "shared-agent")
	if err != nil {
		t.Fatalf("resolve Formations target: %v", err)
	}
	fence, err := openSamePoolInputFencePrototype(fixture.journalPath, target)
	if err != nil {
		t.Fatalf("open prototype fence: %v", err)
	}
	if err := fence.AcquireOccupancy(fence.Registration(), "occupancy_concurrent"); err != nil {
		t.Fatalf("acquire prototype occupancy: %v", err)
	}

	permit := prototypeDispatchPermit{ID: "permit_concurrent_once", Observation: target.Observation}
	const marker = "CONCURRENT_DISPATCH_MUST_SEND_ONCE"
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- fence.Dispatch(context.Background(), permit, marker+"\n")
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent duplicate dispatch successes = %d, want 1", successes)
	}
	fixture.waitForPaneInput(t, target, marker)
	raw, err := os.ReadFile(fixture.inputPaths[prototypeInputKey("alice", "shared-agent")])
	if err != nil {
		t.Fatalf("read concurrent pane input: %v", err)
	}
	if count := strings.Count(string(raw), marker); count != 1 {
		t.Fatalf("concurrent marker count = %d, want 1: %q", count, raw)
	}
}

func TestSamePoolInputFencePrototypeRejectsJournalWithoutAnchoredGenesis(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"shared-agent"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)
	target, err := resolver.Resolve(context.Background(), prototypeFormationsConsumer, "shared-agent")
	if err != nil {
		t.Fatalf("resolve Formations target: %v", err)
	}
	fence, err := openSamePoolInputFencePrototype(fixture.journalPath, target)
	if err != nil {
		t.Fatalf("open prototype fence: %v", err)
	}
	checkpoint := fence.Checkpoint()
	raw, err := os.ReadFile(fixture.journalPath)
	if err != nil {
		t.Fatalf("read prototype journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 1 {
		t.Fatal("prototype journal has no genesis to remove")
	}
	if err := os.WriteFile(fixture.journalPath, []byte(strings.Join(lines[1:], "\n")), 0o600); err != nil {
		t.Fatalf("remove prototype genesis: %v", err)
	}
	if _, err := recoverSamePoolInputFencePrototype(fixture.journalPath, target, checkpoint); err == nil {
		t.Fatal("recovery accepted a journal without its externally anchored genesis")
	}
}

func TestSamePoolInputFencePrototypeRejectsJointJournalAndLocalAnchorRewrite(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"shared-agent"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)
	target, err := resolver.Resolve(context.Background(), prototypeFormationsConsumer, "shared-agent")
	if err != nil {
		t.Fatalf("resolve Formations target: %v", err)
	}
	original, err := openSamePoolInputFencePrototype(fixture.journalPath, target)
	if err != nil {
		t.Fatalf("open original prototype fence: %v", err)
	}
	externalAnchor := original.Checkpoint()
	if err := os.Remove(fixture.journalPath); err != nil {
		t.Fatalf("remove original prototype journal: %v", err)
	}
	replacement, err := openSamePoolInputFencePrototype(fixture.journalPath, target)
	if err != nil {
		t.Fatalf("write replacement prototype journal: %v", err)
	}
	if replacement.Checkpoint() == externalAnchor {
		t.Fatal("replacement journal reused the original coordinator anchor identity")
	}
	if _, err := recoverSamePoolInputFencePrototype(fixture.journalPath, target, externalAnchor); err == nil {
		t.Fatal("recovery accepted a jointly rewritten journal against the coordinator-supplied anchor")
	}
	if _, err := os.Stat(fixture.journalPath + ".registration.json"); !os.IsNotExist(err) {
		t.Fatalf("prototype wrote a local sibling anchor: %v", err)
	}
}

func TestSamePoolInputFencePrototypeRejectsClosedJournalPrefixRollback(t *testing.T) {
	requireSamePoolInputFencePrototype(t)

	fixture := newSamePoolInputFenceFixture(t, map[string][]string{
		"alice": {"shared-agent"},
	})
	resolver := newSamePoolPrototypeResolver(fixture.handler)
	target, err := resolver.Resolve(context.Background(), prototypeFormationsConsumer, "shared-agent")
	if err != nil {
		t.Fatalf("resolve Formations target: %v", err)
	}
	fence, err := openSamePoolInputFencePrototype(fixture.journalPath, target)
	if err != nil {
		t.Fatalf("open prototype fence: %v", err)
	}
	registration := fence.Registration()
	if err := fence.AcquireOccupancy(registration, "occupancy_prefix_rollback"); err != nil {
		t.Fatalf("acquire prototype occupancy: %v", err)
	}
	if err := fence.Close("closure_prefix_rollback"); err != nil {
		t.Fatalf("close prototype occupancy: %v", err)
	}
	externalCheckpoint := fence.Checkpoint()
	raw, err := os.ReadFile(fixture.journalPath)
	if err != nil {
		t.Fatalf("read closed prototype journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 3 {
		t.Fatalf("closed prototype journal has %d records, want at least 3", len(lines))
	}
	if err := os.WriteFile(fixture.journalPath, []byte(lines[0]+"\n"), 0o600); err != nil {
		t.Fatalf("truncate prototype journal to genesis: %v", err)
	}
	if _, err := recoverSamePoolInputFencePrototype(fixture.journalPath, target, externalCheckpoint); err == nil {
		t.Fatal("recovery accepted a closed journal truncated to its valid genesis")
	}
}

func requireSamePoolInputFencePrototype(t *testing.T) {
	t.Helper()
	if os.Getenv("CHROTE_INPUT_FENCE_PROTOTYPE") != "1" {
		t.Skip("set CHROTE_INPUT_FENCE_PROTOTYPE=1 only with explicit approval for disposable tmux servers")
	}
}
