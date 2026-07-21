package api

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type prototypeConsumer string

const (
	prototypeTerminalConsumer   prototypeConsumer = "terminal"
	prototypeFormationsConsumer prototypeConsumer = "formations"
)

var (
	errPrototypeFenceBlocked    = errors.New("prototype input fence is blocked")
	errPrototypeFenceClosed     = errors.New("prototype input fence is closed")
	errPrototypeTargetAmbiguous = errors.New("prototype target is ambiguous")
)

type prototypeTargetObservation struct {
	UnixUser  string `json:"unixUser"`
	ServerPID string `json:"serverPid"`
	SessionID string `json:"sessionId"`
	WindowID  string `json:"windowId"`
	PaneID    string `json:"paneId"`
	PanePID   string `json:"panePid"`
}

type prototypeResolvedTarget struct {
	Consumer    prototypeConsumer
	SessionName string
	Observation prototypeTargetObservation
	tmux        tmuxTarget
	handler     *TmuxHandler
}

type samePoolPrototypeResolver struct {
	handler *TmuxHandler
}

type prototypeTerminalConsumerAdapter struct {
	fixture  *samePoolInputFenceFixture
	resolver *samePoolPrototypeResolver
}

type prototypeFormationsConsumerAdapter struct {
	resolver *samePoolPrototypeResolver
}

type prototypeTerminalLaunchTarget struct {
	Socket  string
	Session string
}

func newSamePoolPrototypeResolver(handler *TmuxHandler) *samePoolPrototypeResolver {
	return &samePoolPrototypeResolver{handler: handler}
}

func newPrototypeTerminalConsumer(fixture *samePoolInputFenceFixture, resolver *samePoolPrototypeResolver) *prototypeTerminalConsumerAdapter {
	return &prototypeTerminalConsumerAdapter{fixture: fixture, resolver: resolver}
}

func newPrototypeFormationsConsumer(resolver *samePoolPrototypeResolver) *prototypeFormationsConsumerAdapter {
	return &prototypeFormationsConsumerAdapter{resolver: resolver}
}

func (c *prototypeTerminalConsumerAdapter) Resolve(ctx context.Context, unixUser, sessionName string) (prototypeResolvedTarget, error) {
	if c == nil || c.fixture == nil || c.resolver == nil {
		return prototypeResolvedTarget{}, fmt.Errorf("prototype Terminal consumer is unavailable")
	}
	launchTarget, err := c.fixture.resolveThroughProductionTerminalLaunch(unixUser, sessionName)
	if err != nil {
		return prototypeResolvedTarget{}, err
	}
	apiTarget, err := c.fixture.handler.targetForUnixUser(unixUser)
	if err != nil {
		return prototypeResolvedTarget{}, fmt.Errorf("resolve production API Terminal source %q: %w", unixUser, err)
	}
	if launchTarget.Socket != apiTarget.socket || launchTarget.Session != sessionName {
		return prototypeResolvedTarget{}, fmt.Errorf("Terminal launch and API sources differ for %q: launch=%+v apiSocket=%q", unixUser, launchTarget, apiTarget.socket)
	}
	resolved, err := c.resolver.Resolve(ctx, prototypeTerminalConsumer, sessionName)
	if err != nil {
		return prototypeResolvedTarget{}, err
	}
	if resolved.Observation.UnixUser != unixUser || resolved.tmux.socket != launchTarget.Socket {
		return prototypeResolvedTarget{}, fmt.Errorf("broker target differs from production Terminal source for %q", unixUser)
	}
	return resolved, nil
}

func (c *prototypeFormationsConsumerAdapter) Resolve(ctx context.Context, sessionName string) (prototypeResolvedTarget, error) {
	if c == nil || c.resolver == nil {
		return prototypeResolvedTarget{}, fmt.Errorf("prototype Formations consumer is unavailable")
	}
	return c.resolver.Resolve(ctx, prototypeFormationsConsumer, sessionName)
}

func (r *samePoolPrototypeResolver) Resolve(ctx context.Context, consumer prototypeConsumer, sessionName string) (prototypeResolvedTarget, error) {
	if r == nil || r.handler == nil {
		return prototypeResolvedTarget{}, fmt.Errorf("prototype resolver is unavailable")
	}
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return prototypeResolvedTarget{}, fmt.Errorf("prototype target session is empty")
	}

	matches := []prototypeResolvedTarget{}
	for _, unixUser := range configuredTerminalUsers() {
		target, err := r.handler.targetForUnixUser(unixUser)
		if err != nil {
			return prototypeResolvedTarget{}, fmt.Errorf("resolve production Terminal source %q: %w", unixUser, err)
		}
		pane, err := r.handler.resolveSendPane(ctx, target, sessionName, "")
		if err != nil {
			var targetErr *sendTargetError
			if errors.As(err, &targetErr) && targetErr.Code == "SESSION_NOT_FOUND" {
				continue
			}
			return prototypeResolvedTarget{}, fmt.Errorf("resolve exact pane on production API Terminal source %q: %w", unixUser, err)
		}
		matches = append(matches, prototypeResolvedTarget{
			Consumer:    consumer,
			SessionName: pane.Session,
			Observation: prototypeTargetObservation{
				UnixUser:  unixUser,
				ServerPID: pane.ServerPID,
				SessionID: pane.SessionID,
				WindowID:  pane.WindowID,
				PaneID:    pane.PaneID,
				PanePID:   pane.PanePID,
			},
			tmux:    target,
			handler: r.handler,
		})
	}
	if len(matches) == 0 {
		return prototypeResolvedTarget{}, fmt.Errorf("prototype target %q was not found", sessionName)
	}
	if len(matches) != 1 {
		return prototypeResolvedTarget{}, fmt.Errorf("%w: session %q matched %d production Terminal sources", errPrototypeTargetAmbiguous, sessionName, len(matches))
	}
	return matches[0], nil
}

type samePoolInputFenceFixture struct {
	handler     *TmuxHandler
	journalPath string
	root        string
	sessions    map[string][]string
	targets     map[string]tmuxTarget
	inputPaths  map[string]string
}

func newSamePoolInputFenceFixture(t *testing.T, inventory map[string][]string) *samePoolInputFenceFixture {
	t.Helper()
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatalf("find tmux for approved disposable prototype: %v", err)
	}
	version, err := exec.Command(tmuxPath, "-V").CombinedOutput()
	if err != nil {
		t.Fatalf("read tmux version for approved disposable prototype: %v: %s", err, version)
	}
	if got := strings.TrimSpace(string(version)); got != "tmux 3.6a" {
		t.Fatalf("approved prototype requires audited tmux 3.6a, got %q", got)
	}
	root, err := os.MkdirTemp("/tmp", "chrote-tmux.")
	if err != nil {
		t.Fatalf("create disposable TMUX_TMPDIR: %v", err)
	}
	fixture := &samePoolInputFenceFixture{
		journalPath: filepath.Join(root, "interaction.journal.ndjson"),
		root:        root,
		sessions:    map[string][]string{},
		targets:     map[string]tmuxTarget{},
		inputPaths:  map[string]string{},
	}
	t.Cleanup(func() { fixture.cleanup(t) })

	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("TMUX_TMPDIR", root)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "")

	users := make([]string, 0, len(inventory))
	for unixUser := range inventory {
		users = append(users, unixUser)
	}
	sort.Strings(users)
	sockets := make([]string, 0, len(users))
	workDirs := make([]string, 0, len(users))
	for _, unixUser := range users {
		socket := filepath.Join(root, unixUser+".sock")
		sockets = append(sockets, unixUser+"="+socket)
		workDirs = append(workDirs, unixUser+"="+root)
	}
	t.Setenv("CHROTE_TERMINAL_USERS", strings.Join(users, ","))
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", strings.Join(sockets, ","))
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", strings.Join(workDirs, ","))

	fixture.handler = NewTmuxHandler()
	for _, unixUser := range users {
		target, resolveErr := fixture.handler.targetForUnixUser(unixUser)
		if resolveErr != nil {
			t.Fatalf("resolve disposable Terminal source %q: %v", unixUser, resolveErr)
		}
		fixture.targets[unixUser] = target
		sessions := append([]string(nil), inventory[unixUser]...)
		sort.Strings(sessions)
		fixture.sessions[unixUser] = sessions
		for _, sessionName := range sessions {
			inputPath := filepath.Join(root, "pane-input-"+unixUser+"-"+sessionName+".txt")
			fixture.inputPaths[prototypeInputKey(unixUser, sessionName)] = inputPath
			output, startErr := fixture.handler.runTmuxOnSocketContext(context.Background(), target.socket,
				"new-session", "-d", "-s", sessionName, "-x", "80", "-y", "24",
				"sh", "-c", `cat >> "$1"`, "sh", inputPath,
			)
			if startErr != nil {
				t.Fatalf("start approved disposable tmux source %q session %q: %v: %s", unixUser, sessionName, startErr, output)
			}
		}
	}
	return fixture
}

func prototypeInputKey(unixUser, sessionName string) string {
	return unixUser + "\x00" + sessionName
}

func (f *samePoolInputFenceFixture) resolveThroughProductionTerminalLaunch(unixUser, sessionName string) (prototypeTerminalLaunchTarget, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("locate prototype source file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	launchScript := filepath.Join(repoRoot, "terminal-launch.sh")
	binDir := filepath.Join(f.root, "terminal-launch-bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("create fake tmux bin: %w", err)
	}
	fakeTmux := filepath.Join(binDir, "tmux")
	fakeScript := `#!/bin/sh
printf 'call' >> "$CHROTE_PROTOTYPE_TMUX_ARGS"
for arg do
  printf '\t%s' "$arg" >> "$CHROTE_PROTOTYPE_TMUX_ARGS"
done
printf '\n' >> "$CHROTE_PROTOTYPE_TMUX_ARGS"
exit 0
`
	if err := os.WriteFile(fakeTmux, []byte(fakeScript), 0o700); err != nil {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("write fake tmux: %w", err)
	}
	argsFile, err := os.CreateTemp(f.root, "terminal-launch-"+unixUser+"-*.args")
	if err != nil {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("create production Terminal launch capture: %w", err)
	}
	argsPath := argsFile.Name()
	if err := argsFile.Close(); err != nil {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("close production Terminal launch capture: %w", err)
	}
	cmd := exec.Command("bash", launchScript, sessionName, unixUser)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HOME="+f.root,
		"CHROTE_WORKDIR="+f.root,
		"CHROTE_PROTOTYPE_TMUX_ARGS="+argsPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("run production Terminal launch resolver: %w: %s", err, strings.TrimSpace(string(output)))
	}
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("read production Terminal launch calls: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		return prototypeTerminalLaunchTarget{}, fmt.Errorf("production Terminal launch made %d tmux calls, want 2: %q", len(lines), raw)
	}
	wantSocket := f.targets[unixUser].socket
	wantCalls := [][]string{
		{"call", "-S", wantSocket, "has-session", "-t", sessionName},
		{"call", "-S", wantSocket, "attach-session", "-t", sessionName},
	}
	for index, line := range lines {
		got := strings.Split(line, "\t")
		if strings.Join(got, "\x00") != strings.Join(wantCalls[index], "\x00") {
			return prototypeTerminalLaunchTarget{}, fmt.Errorf("production Terminal launch call %d = %q, want %q", index+1, got, wantCalls[index])
		}
	}
	return prototypeTerminalLaunchTarget{Socket: wantSocket, Session: sessionName}, nil
}

func (f *samePoolInputFenceFixture) waitForPaneInput(t *testing.T, target prototypeResolvedTarget, marker string) {
	t.Helper()
	path := f.inputPaths[prototypeInputKey(target.Observation.UnixUser, target.SessionName)]
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(raw), marker) {
			return
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read pane input %s: %v", path, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	raw, _ := os.ReadFile(path)
	t.Fatalf("pane input %s never contained %q: %q", path, marker, raw)
}

func (f *samePoolInputFenceFixture) assertPaneInputAbsent(t *testing.T, target prototypeResolvedTarget, marker string) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	path := f.inputPaths[prototypeInputKey(target.Observation.UnixUser, target.SessionName)]
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read pane input %s: %v", path, err)
	}
	if strings.Contains(string(raw), marker) {
		t.Fatalf("pane input %s contains rejected marker %q: %q", path, marker, raw)
	}
}

func (f *samePoolInputFenceFixture) cleanup(t *testing.T) {
	t.Helper()
	if f.handler != nil {
		for unixUser, sessions := range f.sessions {
			target, ok := f.targets[unixUser]
			if !ok {
				t.Errorf("disposable cleanup source %q was not captured", unixUser)
				continue
			}
			for _, sessionName := range sessions {
				if _, err := f.handler.runTmuxOnSocketContext(context.Background(), target.socket, "kill-session", "-t", sessionName); err != nil && !isTmuxNoServerError(err.Error()) {
					t.Errorf("stop disposable session %q on %q: %v", sessionName, unixUser, err)
				}
			}
		}
	}
	base := filepath.Base(f.root)
	info, err := os.Lstat(f.root)
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("inspect disposable root %s: %v", f.root, err)
		return
	}
	if err == nil {
		if filepath.Dir(f.root) != "/tmp" || !strings.HasPrefix(base, "chrote-tmux.") || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("refuse unsafe disposable cleanup target %s mode=%v", f.root, info.Mode())
			return
		}
		if err := os.RemoveAll(f.root); err != nil {
			t.Errorf("remove disposable root %s: %v", f.root, err)
		}
	}
}

type prototypeDispatchPermit struct {
	ID          string
	Observation prototypeTargetObservation
}

type prototypePeekPermit struct {
	RecordID    string
	Generation  uint64
	Credential  string
	Observation prototypeTargetObservation
}

type prototypeJournalEvent struct {
	Seq           uint64                     `json:"seq"`
	Kind          string                     `json:"kind"`
	JournalEpoch  string                     `json:"journalEpoch"`
	PermitID      string                     `json:"permitId,omitempty"`
	RecordID      string                     `json:"recordId,omitempty"`
	PriorRecordID string                     `json:"priorRecordId,omitempty"`
	Generation    uint64                     `json:"generation,omitempty"`
	Target        prototypeTargetObservation `json:"target"`
	PriorSHA256   string                     `json:"priorSha256,omitempty"`
	SHA256        string                     `json:"sha256,omitempty"`
}

type prototypeAuditRegistration struct {
	Target        prototypeTargetObservation `json:"target"`
	JournalEpoch  string                     `json:"journalEpoch"`
	GenesisSHA256 string                     `json:"genesisSha256"`
	JournalCursor uint64                     `json:"journalCursor"`
}

type prototypeAuditCheckpoint struct {
	Registration    prototypeAuditRegistration
	JournalCursor   uint64
	TailSHA256      string
	Closed          bool
	ClosureRecordID string
}

type samePoolInputFencePrototype struct {
	mu                      sync.Mutex
	journalPath             string
	target                  prototypeResolvedTarget
	registration            prototypeAuditRegistration
	journalEpoch            string
	seq                     uint64
	lastHash                string
	occupied                bool
	peekRecordID            string
	peekGeneration          uint64
	lastPeekGeneration      uint64
	lastPeekClosureRecordID string
	peekCredential          string
	closed                  bool
	closureRecordID         string
	blocked                 bool
	usedPermits             map[string]bool
}

func openSamePoolInputFencePrototype(journalPath string, target prototypeResolvedTarget) (*samePoolInputFencePrototype, error) {
	file, err := os.OpenFile(journalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	journalEpoch, err := newPrototypeJournalEpoch()
	if err != nil {
		return nil, err
	}
	fence := &samePoolInputFencePrototype{
		journalPath:  journalPath,
		journalEpoch: journalEpoch,
		target:       target,
		usedPermits:  map[string]bool{},
	}
	if err := fence.appendEvent(prototypeJournalEvent{Kind: "genesis"}); err != nil {
		return nil, err
	}
	fence.registration = prototypeAuditRegistration{
		Target:        target.Observation,
		JournalEpoch:  fence.journalEpoch,
		GenesisSHA256: fence.lastHash,
		JournalCursor: fence.seq,
	}
	dir, err := os.Open(filepath.Dir(journalPath))
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return nil, err
	}
	return fence, nil
}

func newPrototypeJournalEpoch() (string, error) {
	raw := make([]byte, 16)
	if _, err := cryptorand.Read(raw); err != nil {
		return "", fmt.Errorf("generate prototype journal epoch: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func recoverSamePoolInputFencePrototype(journalPath string, target prototypeResolvedTarget, expectedCheckpoint prototypeAuditCheckpoint) (*samePoolInputFencePrototype, error) {
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("prototype journal is empty")
	}
	fence := &samePoolInputFencePrototype{
		journalPath: journalPath,
		target:      target,
		usedPermits: map[string]bool{},
	}
	pendingDispatch := false
	dispatchEffect := false
	pendingPeekInput := false
	peekInputEffect := false
	pendingPeekClose := false
	pendingClosure := false
	for index, line := range lines {
		var event prototypeJournalEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("decode prototype journal event %d: %w", index+1, err)
		}
		if index == 0 {
			if event.Kind != "genesis" || event.JournalEpoch == "" {
				return nil, fmt.Errorf("prototype journal does not start with an identified genesis")
			}
			fence.journalEpoch = event.JournalEpoch
		} else if event.JournalEpoch != fence.journalEpoch {
			return nil, fmt.Errorf("prototype journal epoch changed at event %d", index+1)
		}
		if event.Seq != uint64(index+1) || event.PriorSHA256 != fence.lastHash || event.Target != target.Observation {
			return nil, fmt.Errorf("prototype journal continuity failure at event %d", index+1)
		}
		if prototypeEventHash(event) != event.SHA256 {
			return nil, fmt.Errorf("prototype journal hash failure at event %d", index+1)
		}
		fence.seq = event.Seq
		fence.lastHash = event.SHA256
		switch event.Kind {
		case "genesis":
			if index != 0 {
				return nil, fmt.Errorf("prototype journal has non-initial genesis")
			}
			fence.registration = prototypeAuditRegistration{Target: event.Target, JournalEpoch: event.JournalEpoch, GenesisSHA256: event.SHA256, JournalCursor: event.Seq}
		case "occupancy_barrier":
			if index == 0 || fence.occupied || pendingDispatch || pendingPeekInput || pendingClosure || fence.closed {
				return nil, fmt.Errorf("prototype journal has invalid occupancy barrier")
			}
			fence.occupied = true
			fence.lastPeekClosureRecordID = event.PermitID
		case "dispatch_intent":
			if !fence.occupied || pendingDispatch || pendingPeekInput || pendingClosure || fence.closed {
				return nil, fmt.Errorf("prototype journal has overlapping dispatch intent")
			}
			pendingDispatch = true
			dispatchEffect = false
			fence.usedPermits[event.PermitID] = true
		case "dispatch_effect":
			if !pendingDispatch || dispatchEffect {
				return nil, fmt.Errorf("prototype journal has unbound dispatch effect")
			}
			dispatchEffect = true
		case "dispatch_barrier":
			if !pendingDispatch || !dispatchEffect {
				return nil, fmt.Errorf("prototype journal has unbound dispatch barrier")
			}
			pendingDispatch = false
			dispatchEffect = false
		case "peek_generation_open_barrier":
			if !fence.occupied || fence.peekRecordID != "" || pendingDispatch || pendingPeekInput || pendingPeekClose || pendingClosure || fence.closed || event.PermitID == "" || event.Generation != fence.lastPeekGeneration+1 || event.PriorRecordID != fence.lastPeekClosureRecordID {
				return nil, fmt.Errorf("prototype journal has invalid Peek generation open barrier")
			}
			fence.peekRecordID = event.PermitID
			fence.peekGeneration = event.Generation
			fence.lastPeekGeneration = event.Generation
		case "peek_input_intent":
			if !fence.occupied || fence.peekRecordID == "" || event.PermitID != fence.peekRecordID || event.Generation != fence.peekGeneration || pendingDispatch || pendingPeekInput || pendingPeekClose || pendingClosure || fence.closed {
				return nil, fmt.Errorf("prototype journal has invalid Peek input intent")
			}
			pendingPeekInput = true
			peekInputEffect = false
		case "peek_input_effect":
			if !pendingPeekInput || peekInputEffect || event.PermitID != fence.peekRecordID || event.Generation != fence.peekGeneration {
				return nil, fmt.Errorf("prototype journal has unbound Peek input effect")
			}
			peekInputEffect = true
		case "peek_input_barrier":
			if !pendingPeekInput || !peekInputEffect || event.PermitID != fence.peekRecordID || event.Generation != fence.peekGeneration {
				return nil, fmt.Errorf("prototype journal has unbound Peek input barrier")
			}
			pendingPeekInput = false
			peekInputEffect = false
		case "peek_generation_close_intent":
			if !fence.occupied || fence.peekRecordID == "" || event.PermitID != fence.peekRecordID || event.Generation != fence.peekGeneration || pendingDispatch || pendingPeekInput || pendingPeekClose || pendingClosure || fence.closed {
				return nil, fmt.Errorf("prototype journal has invalid Peek generation close intent")
			}
			pendingPeekClose = true
		case "peek_generation_close_barrier":
			if !pendingPeekClose || event.PermitID != fence.peekRecordID || event.Generation != fence.peekGeneration || event.RecordID == "" {
				return nil, fmt.Errorf("prototype journal has unbound Peek generation close barrier")
			}
			pendingPeekClose = false
			fence.peekRecordID = ""
			fence.peekGeneration = 0
			fence.lastPeekClosureRecordID = event.RecordID
		case "closure_intent":
			if !fence.occupied || fence.peekRecordID != "" || pendingDispatch || pendingPeekInput || pendingPeekClose || pendingClosure || fence.closed {
				return nil, fmt.Errorf("prototype journal has invalid closure intent")
			}
			pendingClosure = true
		case "closure_barrier":
			if !pendingClosure || fence.closed {
				return nil, fmt.Errorf("prototype journal has unbound closure barrier")
			}
			pendingClosure = false
			fence.occupied = false
			fence.closed = true
			fence.closureRecordID = event.PermitID
		default:
			return nil, fmt.Errorf("prototype journal has unknown kind %q", event.Kind)
		}
	}
	if expectedCheckpoint.Registration != fence.registration || expectedCheckpoint.JournalCursor != fence.seq || expectedCheckpoint.TailSHA256 != fence.lastHash || expectedCheckpoint.Closed != fence.closed || expectedCheckpoint.ClosureRecordID != fence.closureRecordID {
		return nil, fmt.Errorf("coordinator-supplied checkpoint does not match the acknowledged journal tail")
	}
	if pendingDispatch || pendingPeekInput || pendingPeekClose || pendingClosure || (fence.occupied && !fence.closed) {
		fence.blocked = true
	}
	return fence, nil
}

func (f *samePoolInputFencePrototype) appendEvent(event prototypeJournalEvent) error {
	event.Seq = f.seq + 1
	event.JournalEpoch = f.journalEpoch
	event.Target = f.target.Observation
	event.PriorSHA256 = f.lastHash
	event.SHA256 = prototypeEventHash(event)
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(f.journalPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	f.seq = event.Seq
	f.lastHash = event.SHA256
	return nil
}

func prototypeEventHash(event prototypeJournalEvent) string {
	event.SHA256 = ""
	raw, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (f *samePoolInputFencePrototype) Registration() prototypeAuditRegistration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registration
}

func (f *samePoolInputFencePrototype) Checkpoint() prototypeAuditCheckpoint {
	f.mu.Lock()
	defer f.mu.Unlock()
	return prototypeAuditCheckpoint{
		Registration:    f.registration,
		JournalCursor:   f.seq,
		TailSHA256:      f.lastHash,
		Closed:          f.closed,
		ClosureRecordID: f.closureRecordID,
	}
}

func (f *samePoolInputFencePrototype) AcquireOccupancy(registration prototypeAuditRegistration, permitID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked {
		return errPrototypeFenceBlocked
	}
	if f.closed {
		return errPrototypeFenceClosed
	}
	if f.occupied || registration != f.registration || strings.TrimSpace(permitID) == "" {
		return fmt.Errorf("prototype occupancy registration or permit is invalid")
	}
	f.blocked = true
	if err := f.appendEvent(prototypeJournalEvent{Kind: "occupancy_barrier", PermitID: permitID}); err != nil {
		return err
	}
	f.occupied = true
	f.lastPeekClosureRecordID = permitID
	f.blocked = false
	return nil
}

func (f *samePoolInputFencePrototype) Dispatch(ctx context.Context, permit prototypeDispatchPermit, payload string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked {
		return errPrototypeFenceBlocked
	}
	if f.closed {
		return errPrototypeFenceClosed
	}
	if !f.occupied {
		return fmt.Errorf("prototype occupancy is not acquired")
	}
	if permit.ID == "" || permit.Observation != f.target.Observation || f.usedPermits[permit.ID] {
		return fmt.Errorf("prototype dispatch permit is invalid or already used")
	}
	f.usedPermits[permit.ID] = true
	f.blocked = true
	if err := f.appendEvent(prototypeJournalEvent{Kind: "dispatch_intent", PermitID: permit.ID}); err != nil {
		return err
	}
	bufferName := "chrote-prototype-" + strings.ReplaceAll(permit.ID, "_", "-")
	if _, err := f.target.handler.runTmuxOnSocketContext(ctx, f.target.tmux.socket, "set-buffer", "-b", bufferName, payload); err != nil {
		return err
	}
	defer func() {
		_, _ = f.target.handler.runTmuxOnSocketContext(context.Background(), f.target.tmux.socket, "delete-buffer", "-b", bufferName)
	}()
	if _, err := f.target.handler.runTmuxOnSocketContext(ctx, f.target.tmux.socket, "paste-buffer", "-d", "-b", bufferName, "-t", f.target.Observation.PaneID); err != nil {
		return err
	}
	if err := f.appendEvent(prototypeJournalEvent{Kind: "dispatch_effect", PermitID: permit.ID}); err != nil {
		return err
	}
	if err := f.appendEvent(prototypeJournalEvent{Kind: "dispatch_barrier", PermitID: permit.ID}); err != nil {
		return err
	}
	f.blocked = false
	return nil
}

func (f *samePoolInputFencePrototype) OpenPeekGeneration(permit prototypePeekPermit, priorClosureRecordID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked {
		return errPrototypeFenceBlocked
	}
	if f.closed {
		return errPrototypeFenceClosed
	}
	if !f.occupied || f.peekRecordID != "" || permit.RecordID == "" || permit.Generation != f.lastPeekGeneration+1 || permit.Credential == "" || permit.Observation != f.target.Observation || priorClosureRecordID == "" || priorClosureRecordID != f.lastPeekClosureRecordID {
		return fmt.Errorf("prototype Peek generation permit is invalid")
	}
	f.blocked = true
	if err := f.appendEvent(prototypeJournalEvent{Kind: "peek_generation_open_barrier", PermitID: permit.RecordID, PriorRecordID: priorClosureRecordID, Generation: permit.Generation}); err != nil {
		return err
	}
	f.peekRecordID = permit.RecordID
	f.peekGeneration = permit.Generation
	f.lastPeekGeneration = permit.Generation
	f.peekCredential = permit.Credential
	f.blocked = false
	return nil
}

func (f *samePoolInputFencePrototype) ClosePeekGeneration(permit prototypePeekPermit, closureRecordID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked {
		return errPrototypeFenceBlocked
	}
	if f.closed {
		return errPrototypeFenceClosed
	}
	if !f.occupied || f.peekRecordID == "" || permit.RecordID != f.peekRecordID || permit.Generation != f.peekGeneration || permit.Credential != f.peekCredential || permit.Observation != f.target.Observation || strings.TrimSpace(closureRecordID) == "" {
		return fmt.Errorf("prototype Peek generation closure is invalid")
	}
	f.blocked = true
	if err := f.appendEvent(prototypeJournalEvent{Kind: "peek_generation_close_intent", PermitID: permit.RecordID, Generation: permit.Generation}); err != nil {
		return err
	}
	if err := f.appendEvent(prototypeJournalEvent{Kind: "peek_generation_close_barrier", PermitID: permit.RecordID, RecordID: closureRecordID, Generation: permit.Generation}); err != nil {
		return err
	}
	f.peekRecordID = ""
	f.peekGeneration = 0
	f.lastPeekClosureRecordID = closureRecordID
	f.peekCredential = ""
	f.blocked = false
	return nil
}

func (f *samePoolInputFencePrototype) SendPeek(ctx context.Context, permit prototypePeekPermit, payload string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked {
		return errPrototypeFenceBlocked
	}
	if f.closed {
		return errPrototypeFenceClosed
	}
	if !f.occupied || f.peekRecordID == "" || permit.RecordID != f.peekRecordID || permit.Generation != f.peekGeneration || permit.Credential != f.peekCredential || permit.Observation != f.target.Observation {
		return fmt.Errorf("prototype Peek generation is not the latest open generation")
	}
	f.blocked = true
	if err := f.appendEvent(prototypeJournalEvent{Kind: "peek_input_intent", PermitID: permit.RecordID, Generation: permit.Generation}); err != nil {
		return err
	}
	bufferName := "chrote-prototype-peek-" + strings.ReplaceAll(permit.RecordID, "_", "-")
	if _, err := f.target.handler.runTmuxOnSocketContext(ctx, f.target.tmux.socket, "set-buffer", "-b", bufferName, payload); err != nil {
		return err
	}
	defer func() {
		_, _ = f.target.handler.runTmuxOnSocketContext(context.Background(), f.target.tmux.socket, "delete-buffer", "-b", bufferName)
	}()
	if _, err := f.target.handler.runTmuxOnSocketContext(ctx, f.target.tmux.socket, "paste-buffer", "-d", "-b", bufferName, "-t", f.target.Observation.PaneID); err != nil {
		return err
	}
	if err := f.appendEvent(prototypeJournalEvent{Kind: "peek_input_effect", PermitID: permit.RecordID, Generation: permit.Generation}); err != nil {
		return err
	}
	if err := f.appendEvent(prototypeJournalEvent{Kind: "peek_input_barrier", PermitID: permit.RecordID, Generation: permit.Generation}); err != nil {
		return err
	}
	f.blocked = false
	return nil
}

func (f *samePoolInputFencePrototype) Close(permitID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked {
		return errPrototypeFenceBlocked
	}
	if f.closed {
		return errPrototypeFenceClosed
	}
	if !f.occupied || f.peekRecordID != "" {
		return fmt.Errorf("prototype occupancy is not acquired")
	}
	if strings.TrimSpace(permitID) == "" {
		return fmt.Errorf("prototype closure permit is empty")
	}
	f.blocked = true
	if err := f.appendEvent(prototypeJournalEvent{Kind: "closure_intent", PermitID: permitID}); err != nil {
		return err
	}
	if err := f.appendEvent(prototypeJournalEvent{Kind: "closure_barrier", PermitID: permitID}); err != nil {
		return err
	}
	f.closed = true
	f.closureRecordID = permitID
	f.occupied = false
	f.peekRecordID = ""
	f.peekCredential = ""
	f.blocked = false
	return nil
}

func (f *samePoolInputFencePrototype) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *samePoolInputFencePrototype) Blocked() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocked
}
