package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	osuser "os/user"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chrote/server/internal/scheduled"
)

func TestScheduledTasksAPIEnvelopeAndLifecycle(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)
	runner := newFakeScheduledRunner()
	runner.allow(scheduled.Target{SessionName: "ops", UnixUser: "alice"})
	handler := newScheduledTestHandler(t, runner, fixedNow, true)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	created := scheduledAPIPost(t, mux, "/api/scheduled-tasks", `{
		"name":"Standup nudge",
		"prompt":"hello; rm -rf / $(whoami)",
		"target":{"sessionName":"ops","unixUser":"alice"},
		"schedule":{"type":"interval","everyMinutes":15,"timezone":"UTC"},
		"createdBy":"agent:test"
	}`)
	assertScheduledEnvelopeKeys(t, created, "task")
	createdTask := decodeScheduledTaskFromData(t, created, "task")
	if createdTask.ID == "" || createdTask.Name != "Standup nudge" || !createdTask.Enabled || createdTask.Paused {
		t.Fatalf("created task = %+v, want durable enabled task", createdTask)
	}
	wantNext := fixedNow.Add(15 * time.Minute)
	if createdTask.NextRun == nil || !createdTask.NextRun.Equal(wantNext) {
		t.Fatalf("created nextRun = %v, want %s", createdTask.NextRun, wantNext.Format(time.RFC3339))
	}
	if createdTask.CreatedBy != "agent:test" || createdTask.UpdatedBy != "agent:test" {
		t.Fatalf("created metadata = %q/%q, want createdBy and updatedBy", createdTask.CreatedBy, createdTask.UpdatedBy)
	}

	listed := scheduledAPIGet(t, mux, "/api/scheduled-tasks")
	assertScheduledEnvelopeKeys(t, listed, "tasks")
	listedTasks := decodeScheduledTasksFromData(t, listed, "tasks")
	if len(listedTasks) != 1 || listedTasks[0].ID != createdTask.ID {
		t.Fatalf("listed tasks = %+v, want created task", listedTasks)
	}

	read := scheduledAPIGet(t, mux, "/api/scheduled-tasks/"+createdTask.ID)
	readTask := decodeScheduledTaskFromData(t, read, "task")
	if readTask.ID != createdTask.ID {
		t.Fatalf("read id = %q, want %q", readTask.ID, createdTask.ID)
	}

	patched := scheduledAPIPatch(t, mux, "/api/scheduled-tasks/"+createdTask.ID, `{
		"name":"Renamed nudge",
		"prompt":"patched literal prompt",
		"schedule":{"type":"interval","everyMinutes":30,"timezone":"UTC"},
		"updatedBy":"agent:patch"
	}`)
	patchedTask := decodeScheduledTaskFromData(t, patched, "task")
	if patchedTask.Name != "Renamed nudge" || patchedTask.Prompt != "patched literal prompt" || patchedTask.Schedule.EveryMinutes != 30 {
		t.Fatalf("patched task = %+v, want changed name/prompt/schedule", patchedTask)
	}
	if patchedTask.UpdatedBy != "agent:patch" {
		t.Fatalf("updatedBy = %q, want agent:patch", patchedTask.UpdatedBy)
	}

	paused := scheduledAPIPost(t, mux, "/api/scheduled-tasks/"+createdTask.ID+"/pause", `{}`)
	pausedTask := decodeScheduledTaskFromData(t, paused, "task")
	if !pausedTask.Paused || pausedTask.NextRun != nil {
		t.Fatalf("paused task = %+v, want paused with no nextRun", pausedTask)
	}

	resumed := scheduledAPIPost(t, mux, "/api/scheduled-tasks/"+createdTask.ID+"/resume", `{}`)
	resumedTask := decodeScheduledTaskFromData(t, resumed, "task")
	if resumedTask.Paused || resumedTask.NextRun == nil {
		t.Fatalf("resumed task = %+v, want active nextRun", resumedTask)
	}

	runNow := scheduledAPIPost(t, mux, "/api/scheduled-tasks/"+createdTask.ID+"/run-now", `{}`)
	assertScheduledEnvelopeKeys(t, runNow, "run", "task")
	runTask := decodeScheduledTaskFromData(t, runNow, "task")
	if runTask.LastStatus != scheduled.RunStatusSuccess || runTask.LastRun == nil || len(runTask.RecentRuns) != 1 {
		t.Fatalf("run-now task = %+v, want successful persisted run", runTask)
	}
	if len(runner.sent) != 1 || runner.sent[0].prompt != "patched literal prompt" || runner.sent[0].target.SessionName != "ops" {
		t.Fatalf("runner sends = %+v, want patched prompt to selected target", runner.sent)
	}

	deleted := scheduledAPIDelete(t, mux, "/api/scheduled-tasks/"+createdTask.ID)
	assertScheduledEnvelopeKeys(t, deleted, "deleted")
	if deletedID, _ := deleted["data"].(map[string]any)["deleted"].(string); deletedID != createdTask.ID {
		t.Fatalf("deleted id = %q, want %q", deletedID, createdTask.ID)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scheduled-tasks/"+createdTask.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("read deleted status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduledTasksAPIFansOneTaskOutToManySessions(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)
	runner := newFakeScheduledRunner()
	for _, session := range []string{"worker-1", "worker-2", "worker-3"} {
		runner.allow(scheduled.Target{SessionName: session, UnixUser: "alice"})
	}
	handler := newScheduledTestHandler(t, runner, fixedNow, true)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	created := scheduledAPIPost(t, mux, "/api/scheduled-tasks", `{
		"name":"Continue work",
		"prompt":"Continue if work is clear",
		"targets":[
			{"sessionName":"worker-1","unixUser":"alice"},
			{"sessionName":"worker-2","unixUser":"alice"},
			{"sessionName":"worker-3","unixUser":"alice"}
		],
		"schedule":{"type":"cron","expression":"0 16 * * *","timezone":"Europe/Helsinki"},
		"createdBy":"agent:test"
	}`)
	createdTask := decodeScheduledTaskFromData(t, created, "task")
	if len(createdTask.Targets) != 3 {
		t.Fatalf("created targets = %+v, want all three sessions", createdTask.Targets)
	}

	runNow := scheduledAPIPost(t, mux, "/api/scheduled-tasks/"+createdTask.ID+"/run-now", `{}`)
	runTask := decodeScheduledTaskFromData(t, runNow, "task")
	if runTask.LastStatus != scheduled.RunStatusSuccess {
		t.Fatalf("lastStatus = %q, want success across every target", runTask.LastStatus)
	}
	if len(runner.sent) != 3 {
		t.Fatalf("runner sends = %+v, want one send per target", runner.sent)
	}
	if len(runTask.RecentRuns) != 1 || len(runTask.RecentRuns[0].Targets) != 3 {
		t.Fatalf("run entry = %+v, want per-target results recorded", runTask.RecentRuns)
	}

	// The legacy single-target field stays readable for older API clients.
	legacy := decodeScheduledLegacyTarget(t, runNow)
	if legacy["sessionName"] != "worker-1" {
		t.Fatalf("legacy target mirror = %v, want the first target", legacy)
	}

	patched := scheduledAPIPatch(t, mux, "/api/scheduled-tasks/"+createdTask.ID, `{
		"targets":[{"sessionName":"worker-2","unixUser":"alice"}],
		"updatedBy":"agent:patch"
	}`)
	patchedTask := decodeScheduledTaskFromData(t, patched, "task")
	if len(patchedTask.Targets) != 1 || patchedTask.Targets[0].SessionName != "worker-2" {
		t.Fatalf("patched targets = %+v, want the replacement list", patchedTask.Targets)
	}
}

func TestScheduledTasksAPIRejectsEmptyTargetList(t *testing.T) {
	runner := newFakeScheduledRunner()
	handler := newScheduledTestHandler(t, runner, time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC), true)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/scheduled-tasks", strings.NewReader(`{
		"name":"no targets","prompt":"hello","targets":[],
		"schedule":{"type":"interval","everyMinutes":15,"timezone":"UTC"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(scheduledMutationIntentHeader, scheduledMutationIntentValue)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "at least one target") {
		t.Fatalf("status/body = %d/%s, want 400 demanding a target", rec.Code, rec.Body.String())
	}
	if len(runner.sent) != 0 {
		t.Fatalf("runner sends = %+v, want none", runner.sent)
	}
}

func TestScheduledTasksAPIRequiresMutationIntentAndJSON(t *testing.T) {
	runner := newFakeScheduledRunner()
	runner.allow(scheduled.Target{SessionName: "ops", UnixUser: "alice"})
	handler := newScheduledTestHandler(t, runner, time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC), true)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"name":"csrf","prompt":"hello","target":{"sessionName":"ops","unixUser":"alice"},"schedule":{"type":"interval","everyMinutes":15,"timezone":"UTC"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/scheduled-tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "X-Chrote-Intent") {
		t.Fatalf("missing intent status/body = %d/%s, want 403 mentioning X-Chrote-Intent", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/scheduled-tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set(scheduledMutationIntentHeader, scheduledMutationIntentValue)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType || !strings.Contains(rec.Body.String(), "application/json") {
		t.Fatalf("simple content-type status/body = %d/%s, want 415 mentioning application/json", rec.Code, rec.Body.String())
	}

	if len(runner.sent) != 0 {
		t.Fatalf("runner sends = %+v, want no tmux side effects from rejected mutations", runner.sent)
	}
}

func TestScheduledTasksAPIValidationRejectsUnsafeOrInvalidRequests(t *testing.T) {
	fixedNow := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name       string
		body       string
		allow      []scheduled.Target
		wantStatus int
		wantBody   string
	}{
		{
			name: "empty prompt",
			body: `{"name":"bad","prompt":"   ","target":{"sessionName":"ops","unixUser":"alice"},"schedule":{"type":"interval","everyMinutes":15,"timezone":"UTC"}}`,
		},
		{
			name: "invalid interval",
			body: `{"name":"bad","prompt":"hello","target":{"sessionName":"ops","unixUser":"alice"},"schedule":{"type":"interval","everyMinutes":0,"timezone":"UTC"}}`,
		},
		{
			name: "invalid cron",
			body: `{"name":"bad","prompt":"hello","target":{"sessionName":"ops","unixUser":"alice"},"schedule":{"type":"cron","expression":"61 * * * *","timezone":"UTC"}}`,
		},
		{
			name:     "socket field is forbidden",
			body:     `{"name":"bad","prompt":"hello","target":{"sessionName":"ops","unixUser":"alice","socket":"/tmp/evil.sock"},"schedule":{"type":"interval","everyMinutes":15,"timezone":"UTC"}}`,
			wantBody: "socket",
		},
		{
			name:     "unknown target rejected when validation enabled",
			body:     `{"name":"bad","prompt":"hello","target":{"sessionName":"missing","unixUser":"alice"},"schedule":{"type":"interval","everyMinutes":15,"timezone":"UTC"}}`,
			wantBody: "target",
		},
		{
			name:     "unauthorized user rejected by validator",
			body:     `{"name":"bad","prompt":"hello","target":{"sessionName":"ops","unixUser":"intruder"},"schedule":{"type":"interval","everyMinutes":15,"timezone":"UTC"}}`,
			wantBody: "not allowed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := newFakeScheduledRunner()
			for _, target := range tt.allow {
				runner.allow(target)
			}
			handler := newScheduledTestHandler(t, runner, fixedNow, true)
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			req := httptest.NewRequest(http.MethodPost, "/api/scheduled-tasks", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(scheduledMutationIntentHeader, scheduledMutationIntentValue)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			wantStatus := tt.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusBadRequest
			}
			if rec.Code != wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
			}
			var response map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode validation response: %v", err)
			}
			if response["success"] != false || response["timestamp"] == "" || response["error"] == nil {
				t.Fatalf("validation response = %#v, want error envelope", response)
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want it to mention %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestScheduledTmuxRunnerUnixUserLookupHonorsCallerDeadline(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *ScheduledTmuxRunner, scheduled.Target) error
	}{
		{name: "validate", run: func(ctx context.Context, runner *ScheduledTmuxRunner, target scheduled.Target) error {
			return runner.ValidateTarget(ctx, target)
		}},
		{name: "send", run: func(ctx context.Context, runner *ScheduledTmuxRunner, target scheduled.Target) error {
			_, err := runner.SendPrompt(ctx, target, "deadline")
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			username := "deadline-" + operation.name
			t.Setenv("CHROTE_TERMINAL_USERS", username)
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "")
			oldLookup := tmuxLookupUser
			release := make(chan struct{})
			lookupFinished := make(chan struct{})
			var calls atomic.Int32
			tmuxLookupUser = func(name string) (*osuser.User, error) {
				calls.Add(1)
				<-release
				close(lookupFinished)
				return &osuser.User{Username: name, Uid: "1000", HomeDir: "/tmp/chrote-deadline-home"}, nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			started := time.Now()
			go func() {
				done <- operation.run(ctx, NewScheduledTmuxRunner(NewTmuxHandler()), scheduled.Target{SessionName: "ops", UnixUser: username})
			}()

			var err error
			completedInTime := true
			select {
			case err = <-done:
			case <-time.After(120 * time.Millisecond):
				completedInTime = false
			}
			close(release)
			<-lookupFinished
			if !completedInTime {
				err = <-done
			}
			tmuxLookupUser = oldLookup

			if !completedInTime {
				t.Fatalf("%s remained blocked in Unix-user lookup for %s after a 30ms deadline (eventual error: %v)", operation.name, time.Since(started), err)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("%s error = %v, want context deadline exceeded", operation.name, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("%s lookup calls = %d, want one", operation.name, calls.Load())
			}
		})
	}
}

func TestScheduledTmuxRunnerCurrentUserLookupHonorsCallerDeadline(t *testing.T) {
	const username = "deadline-current"
	t.Setenv("CHROTE_TERMINAL_USERS", username)
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "")
	oldLookup := tmuxLookupUser
	oldCurrent := tmuxCurrentUser
	tmuxLookupUser = func(name string) (*osuser.User, error) {
		return &osuser.User{Username: name, Uid: "1000", HomeDir: "/tmp/chrote-deadline-home"}, nil
	}
	release := make(chan struct{})
	tmuxCurrentUser = func() (*osuser.User, error) {
		<-release
		return &osuser.User{Username: "alice", Uid: "1001", HomeDir: "/tmp/chrote-current-home"}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- NewScheduledTmuxRunner(NewTmuxHandler()).ValidateTarget(ctx, scheduled.Target{SessionName: "ops", UnixUser: username})
	}()
	var err error
	completedInTime := true
	select {
	case err = <-done:
	case <-time.After(120 * time.Millisecond):
		completedInTime = false
	}
	close(release)
	if !completedInTime {
		err = <-done
	}
	tmuxLookupUser = oldLookup
	tmuxCurrentUser = oldCurrent
	if !completedInTime {
		t.Fatalf("validation remained blocked in current-user lookup after a 30ms deadline (eventual error: %v)", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ValidateTarget error = %v, want context deadline exceeded", err)
	}
}

func TestScheduledTmuxRunnerDeduplicatesCanceledUnixUserLookups(t *testing.T) {
	const username = "deadline-shared"
	t.Setenv("CHROTE_TERMINAL_USERS", username)
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "")
	oldLookup := tmuxLookupUser
	release := make(chan struct{})
	var calls atomic.Int32
	tmuxLookupUser = func(name string) (*osuser.User, error) {
		calls.Add(1)
		<-release
		return &osuser.User{Username: name, Uid: "1000", HomeDir: "/tmp/chrote-deadline-home"}, nil
	}

	runner := NewScheduledTmuxRunner(NewTmuxHandler())
	done := make(chan error, 2)
	for range 2 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			done <- runner.ValidateTarget(ctx, scheduled.Target{SessionName: "ops", UnixUser: username})
		}()
	}
	time.Sleep(60 * time.Millisecond)
	close(release)
	for range 2 {
		<-done
	}
	tmuxLookupUser = oldLookup
	if calls.Load() != 1 {
		t.Fatalf("concurrent canceled lookups spawned %d OS lookups, want one bounded in-flight lookup", calls.Load())
	}
}

func TestScheduledTmuxRunnerDeliversThroughGuardedPasteAndSubmits(t *testing.T) {
	tmpDir := t.TempDir()
	argsPath := tmpDir + "/tmux-argv.txt"
	payloadCopy := tmpDir + "/payload-copy.txt"
	payloadPathRecord := tmpDir + "/payload-path.txt"
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
{
  printf 'CALL\n'
  for arg in "$@"; do
    printf '%s\n' "$arg"
  done
} >> "$TMUX_ARGS_FILE"
last=""
for arg in "$@"; do
  last="$arg"
done
for arg in "$@"; do
  case "$arg" in
    list-panes)
      printf '$1\tops\t%%1\t1234\t5678\t@1\tmain\t/tmp\tclaude\t1\n'
      exit 0
      ;;
    load-buffer)
      cp "$last" "$TMUX_PAYLOAD_COPY"
      printf '%s\n' "$last" > "$TMUX_PAYLOAD_PATH"
      exit 0
      ;;
    if-shell)
      case " $* " in
        *" send-keys "*) printf 'CHROTE_SEND_SUBMIT_KEY_DISPATCHED\n' ;;
        *) printf 'CHROTE_SEND_PASTED\n' ;;
      esac
      exit 0
      ;;
  esac
done
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_PAYLOAD_COPY", payloadCopy)
	t.Setenv("TMUX_PAYLOAD_PATH", payloadPathRecord)
	// tmpDir stays first so the fake tmux wins; the stub also needs cp.
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/chrote-fake-tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/tmp/chrote-fake-home")

	runner := NewScheduledTmuxRunner(NewTmuxHandler())
	prompt := "-X; rm -rf / $(whoami)"
	delivery, err := runner.SendPrompt(context.Background(), scheduled.Target{SessionName: "ops", UnixUser: "alice"}, prompt)
	if err != nil {
		t.Fatalf("SendPrompt returned error: %v", err)
	}
	if delivery.Pane != "%1" || !delivery.SubmitKeyDispatched {
		t.Fatalf("delivery = %+v, want the resolved pane recorded with a submit-key receipt", delivery)
	}

	raw, err := osReadFileString(argsPath)
	if err != nil {
		t.Fatalf("read fake tmux args: %v", err)
	}
	calls := splitScheduledTmuxCalls(raw)
	if len(calls) != 5 {
		t.Fatalf("tmux calls = %#v, want list-panes, load-buffer, guarded paste, guarded submit key, then one fail-closed composer capture", calls)
	}
	if calls[0][2] != "list-panes" {
		t.Fatalf("first call = %#v, want pane resolution before any side effect", calls[0])
	}
	if calls[1][2] != "load-buffer" || calls[1][3] != "-b" || !strings.HasPrefix(calls[1][4], "chrote-scheduled-") {
		t.Fatalf("second call = %#v, want the prompt staged in a private buffer", calls[1])
	}
	guardedPaste := strings.Join(calls[2], "\x00")
	for _, want := range []string{
		"if-shell", "#{==:#{pane_id},%1}",
		"paste-buffer -p -d -b " + calls[1][4] + " -t %1 ; display-message -p CHROTE_SEND_PASTED",
	} {
		if !strings.Contains(guardedPaste, want) {
			t.Fatalf("guarded paste call = %#v, want it to contain %q", calls[2], want)
		}
	}
	guardedSubmit := strings.Join(calls[3], "\x00")
	for _, want := range []string{"if-shell", "#{==:#{pane_id},%1}", "send-keys -t %1 Enter", "CHROTE_SEND_SUBMIT_KEY_DISPATCHED"} {
		if !strings.Contains(guardedSubmit, want) {
			t.Fatalf("guarded submit call = %#v, want it to contain %q", calls[3], want)
		}
	}
	if got := strings.Join(calls[4], "\x00"); !strings.Contains(got, "capture-pane\x00-p\x00-J\x00-t\x00%1\x00-S\x00-200") {
		t.Fatalf("post-submit observation call = %#v, want a bounded exact-pane capture", calls[4])
	}
	for _, call := range calls {
		for _, arg := range call {
			if strings.Contains(arg, "rm -rf") {
				t.Fatalf("prompt text reached tmux argv in %#v; it must travel through the buffer only", call)
			}
		}
	}

	staged, err := osReadFileString(payloadCopy)
	if err != nil {
		t.Fatalf("read staged payload: %v; tmux calls were:\n%s", err, raw)
	}
	if staged != prompt {
		t.Fatalf("staged payload = %q, want exactly the prompt with nothing appended", staged)
	}
	stagedPath, err := osReadFileString(payloadPathRecord)
	if err != nil {
		t.Fatalf("read staged payload path: %v", err)
	}
	if _, err := os.Stat(strings.TrimSpace(stagedPath)); !os.IsNotExist(err) {
		t.Fatalf("staged prompt file still exists after delivery (stat err = %v)", err)
	}

	if _, err := runner.SendPrompt(context.Background(), scheduled.Target{SessionName: "ops", UnixUser: "alice"}, "second delivery with witness"); err != nil {
		t.Fatalf("second SendPrompt returned error: %v", err)
	}
	raw, err = osReadFileString(argsPath)
	if err != nil {
		t.Fatalf("read fake tmux args after second delivery: %v", err)
	}
	calls = splitScheduledTmuxCalls(raw)
	if len(calls) != 10 {
		t.Fatalf("tmux calls after two sends = %#v, want five calls per delivery", calls)
	}
	if calls[1][4] == calls[6][4] {
		t.Fatalf("scheduled deliveries reused buffer %q; timeout cleanup could delete a later run", calls[1][4])
	}
}

func TestScheduledTmuxRunnerReportsUnconfirmedDeliveryWithoutClaimingSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    list-panes)
      printf '$1\tops\t%%1\t1234\t5678\t@1\tmain\t/tmp\tclaude\t1\n'
      exit 0
      ;;
    if-shell)
      printf 'CHROTE_SEND_TARGET_CHANGED\n'
      exit 0
      ;;
  esac
done
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/chrote-fake-tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/tmp/chrote-fake-home")

	runner := NewScheduledTmuxRunner(NewTmuxHandler())
	_, err := runner.SendPrompt(context.Background(), scheduled.Target{SessionName: "ops", UnixUser: "alice"}, "continue")
	if err == nil {
		t.Fatal("SendPrompt returned nil when the pane generation changed")
	}
	if !errors.Is(err, scheduled.ErrTargetNotFound) {
		t.Fatalf("error = %v, want it classified as a target failure", err)
	}
}

func TestScheduledTmuxRunnerHonorsCanceledContextBeforeTmuxSideEffect(t *testing.T) {
	tmpDir := t.TempDir()
	argsPath := tmpDir + "/tmux-argv.txt"
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
printf 'tmux started\n' >> "$TMUX_ARGS_FILE"
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/chrote-fake-tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/tmp/chrote-fake-home")

	runner := NewScheduledTmuxRunner(NewTmuxHandler())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runner.SendPrompt(ctx, scheduled.Target{SessionName: "ops", UnixUser: "alice"}, "do not send"); err == nil {
		t.Fatal("SendPrompt returned nil for a canceled context")
	}
	if raw, err := os.ReadFile(argsPath); err == nil {
		t.Fatalf("tmux was started despite canceled context: %s", raw)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read fake tmux args: %v", err)
	}
}

func TestScheduledTmuxRunnerCancelsGuardedPasteWithCallerContext(t *testing.T) {
	tmpDir := t.TempDir()
	argsPath := tmpDir + "/tmux-argv.txt"
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS_FILE"
for arg in "$@"; do
  case "$arg" in
    list-panes)
      printf '$1\tops\t%%1\t1234\t5678\t@1\tmain\t/tmp\tclaude\t1\n'
      exit 0
      ;;
    load-buffer)
      exit 0
      ;;
    if-shell)
      exec sleep 2
      ;;
  esac
done
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/chrote-fake-tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/tmp/chrote-fake-home")

	runner := NewScheduledTmuxRunner(NewTmuxHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runner.SendPrompt(ctx, scheduled.Target{SessionName: "ops", UnixUser: "alice"}, "cancel paste")
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("SendPrompt returned nil after the caller deadline interrupted guarded paste")
	}
	if elapsed > 600*time.Millisecond {
		t.Fatalf("guarded paste escaped caller deadline: elapsed %s", elapsed)
	}
	cleanupDeadline := time.Now().Add(time.Second)
	for {
		raw, readErr := os.ReadFile(argsPath)
		if readErr == nil && strings.Contains(string(raw), "delete-buffer") {
			break
		}
		if time.Now().After(cleanupDeadline) {
			t.Fatalf("timed-out scheduled buffer did not start bounded background cleanup; log=%q err=%v", raw, readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestScheduledTmuxRunnerReportsCleanupFailure(t *testing.T) {
	tmpDir := t.TempDir()
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    list-panes)
      printf '$1\tops\t%%1\t1234\t5678\t@1\tmain\t/tmp\tclaude\t1\n'
      exit 0
      ;;
    load-buffer)
      exit 0
      ;;
    delete-buffer)
      printf 'delete denied\n' >&2
      exit 42
      ;;
    if-shell)
      exec sleep 2
      ;;
  esac
done
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/chrote-fake-tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/tmp/chrote-fake-home")

	runner := NewScheduledTmuxRunner(NewTmuxHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runner.SendPrompt(ctx, scheduled.Target{SessionName: "ops", UnixUser: "alice"}, "cleanup must be observable")
	if err == nil {
		t.Fatal("SendPrompt returned nil after guarded paste timeout and cleanup failure")
	}
	if !strings.Contains(err.Error(), "buffer cleanup failed") || !strings.Contains(err.Error(), "delete denied") {
		t.Fatalf("error = %v, want fail-loud delete-buffer failure", err)
	}
	if elapsed := time.Since(started); elapsed > 600*time.Millisecond {
		t.Fatalf("cleanup failure escaped the caller deadline: elapsed %s", elapsed)
	}
}

func TestScheduledTmuxRunnerCancelsSettleBeforeSubmitKey(t *testing.T) {
	tmpDir := t.TempDir()
	argsPath := tmpDir + "/tmux-argv.txt"
	fakeTmux := tmpDir + "/tmux"
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS_FILE"
for arg in "$@"; do
  case "$arg" in
    list-panes)
      printf '$1\tops\t%%1\t1234\t5678\t@1\tmain\t/tmp\tclaude\t1\n'
      exit 0
      ;;
    load-buffer)
      exit 0
      ;;
    if-shell)
      case " $* " in
        *" send-keys "*) printf 'CHROTE_SEND_SUBMIT_KEY_DISPATCHED\n' ;;
        *) printf 'CHROTE_SEND_PASTED\n' ;;
      esac
      exit 0
      ;;
  esac
done
exit 0
`
	if err := osWriteFileExecutable(fakeTmux, script); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/chrote-fake-tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/tmp/chrote-fake-home")

	runner := NewScheduledTmuxRunner(NewTmuxHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runner.SendPrompt(ctx, scheduled.Target{SessionName: "ops", UnixUser: "alice"}, "cancel submit")
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("SendPrompt returned nil after the caller deadline expired during paste settle")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("settle cancellation error = %v, want context deadline exceeded", err)
	}
	if elapsed > 600*time.Millisecond {
		t.Fatalf("paste settle escaped caller deadline: elapsed %s", elapsed)
	}
	raw, readErr := os.ReadFile(argsPath)
	if readErr != nil {
		t.Fatalf("read fake tmux args: %v", readErr)
	}
	if strings.Contains(string(raw), "send-keys") {
		t.Fatalf("submit key was dispatched after caller deadline:\n%s", raw)
	}
}

func TestProductionScheduledServiceUsesEightConcurrentWorkers(t *testing.T) {
	runner := &productionValidationRunner{
		entered: make(chan scheduled.Target, 16),
		release: make(chan struct{}),
	}
	service := newProductionScheduledService(scheduled.NewStore(t.TempDir()), runner)
	targets := make([]scheduled.Target, 16)
	for index := range targets {
		targets[index] = scheduled.Target{SessionName: fmt.Sprintf("worker-%02d", index)}
	}
	done := make(chan error, 1)
	go func() {
		_, err := service.Create(context.Background(), scheduled.CreateTaskRequest{
			Name:     "production concurrency",
			Prompt:   "validate",
			Targets:  targets,
			Schedule: scheduled.Schedule{Type: "interval", EveryMinutes: 60, Timezone: "UTC"},
		})
		done <- err
	}()
	for range 8 {
		select {
		case <-runner.entered:
		case <-time.After(time.Second):
			close(runner.release)
			<-done
			t.Fatal("production validation did not start eight workers")
		}
	}
	select {
	case target := <-runner.entered:
		close(runner.release)
		<-done
		t.Fatalf("production validation exceeded eight workers with %+v", target)
	case <-time.After(50 * time.Millisecond):
	}
	close(runner.release)
	if err := <-done; err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
}

type productionValidationRunner struct {
	entered chan scheduled.Target
	release chan struct{}
}

func (r *productionValidationRunner) ValidateTarget(_ context.Context, target scheduled.Target) error {
	r.entered <- target
	<-r.release
	return nil
}

func (r *productionValidationRunner) SendPrompt(context.Context, scheduled.Target, string) (scheduled.Delivery, error) {
	return scheduled.Delivery{}, errors.New("SendPrompt should not run during production validation test")
}

func TestWriteScheduledErrorReportsTaskMutationConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeScheduledError(recorder, fmt.Errorf("%w: tsk_busy", scheduled.ErrConflict))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"CONFLICT"`) {
		t.Fatalf("status/body = %d/%s, want public 409 CONFLICT", recorder.Code, recorder.Body.String())
	}
}

func newScheduledTestHandler(t *testing.T, runner scheduled.Runner, now time.Time, validateTargets bool) *ScheduledHandler {
	t.Helper()
	store := scheduled.NewStore(t.TempDir())
	service := scheduled.NewService(store, runner, scheduled.ServiceOptions{
		Now:             func() time.Time { return now },
		ValidateTargets: validateTargets,
	})
	return NewScheduledHandlerWithService(service)
}

type fakeScheduledRunner struct {
	allowed map[scheduled.Target]bool
	sent    []fakeScheduledSend
}

type fakeScheduledSend struct {
	target scheduled.Target
	prompt string
}

func newFakeScheduledRunner() *fakeScheduledRunner {
	return &fakeScheduledRunner{allowed: map[scheduled.Target]bool{}}
}

func (r *fakeScheduledRunner) allow(target scheduled.Target) {
	r.allowed[target] = true
}

func (r *fakeScheduledRunner) ValidateTarget(_ context.Context, target scheduled.Target) error {
	if target.UnixUser == "intruder" {
		return errors.New("Unix user \"intruder\" is not allowed for terminal launch")
	}
	if !r.allowed[target] {
		return scheduled.ErrTargetNotFound
	}
	return nil
}

func (r *fakeScheduledRunner) SendPrompt(_ context.Context, target scheduled.Target, prompt string) (scheduled.Delivery, error) {
	if err := r.ValidateTarget(context.Background(), target); err != nil {
		return scheduled.Delivery{}, err
	}
	r.sent = append(r.sent, fakeScheduledSend{target: target, prompt: prompt})
	return scheduled.Delivery{Pane: "%1", SubmitKeyDispatched: true, Detail: "submit key dispatched"}, nil
}

func scheduledAPIGet(t *testing.T, mux http.Handler, path string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return decodeScheduledOKResponse(t, rec)
}

func scheduledAPIPost(t *testing.T, mux http.Handler, path, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(scheduledMutationIntentHeader, scheduledMutationIntentValue)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return decodeScheduledOKResponse(t, rec)
}

func scheduledAPIPatch(t *testing.T, mux http.Handler, path, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(scheduledMutationIntentHeader, scheduledMutationIntentValue)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return decodeScheduledOKResponse(t, rec)
}

func scheduledAPIDelete(t *testing.T, mux http.Handler, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(scheduledMutationIntentHeader, scheduledMutationIntentValue)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return decodeScheduledOKResponse(t, rec)
}

func decodeScheduledOKResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	if response["success"] != true || response["timestamp"] == "" {
		t.Fatalf("response = %#v, want success envelope with timestamp", response)
	}
	return response
}

func assertScheduledEnvelopeKeys(t *testing.T, response map[string]any, dataKeys ...string) {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %T, want object in response %#v", response["data"], response)
	}
	for _, key := range dataKeys {
		if _, ok := data[key]; !ok {
			t.Fatalf("data keys missing %q in %#v", key, data)
		}
	}
}

func decodeScheduledTaskFromData(t *testing.T, response map[string]any, key string) scheduled.Task {
	t.Helper()
	data := response["data"].(map[string]any)
	raw, err := json.Marshal(data[key])
	if err != nil {
		t.Fatalf("marshal task data: %v", err)
	}
	var task scheduled.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatalf("decode task: %v; raw=%s", err, raw)
	}
	return task
}

func decodeScheduledLegacyTarget(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	data := response["data"].(map[string]any)
	task, ok := data["task"].(map[string]any)
	if !ok {
		t.Fatalf("response data has no task object: %#v", data)
	}
	legacy, ok := task["target"].(map[string]any)
	if !ok {
		t.Fatalf("task has no legacy target mirror: %#v", task)
	}
	return legacy
}

func decodeScheduledTasksFromData(t *testing.T, response map[string]any, key string) []scheduled.Task {
	t.Helper()
	data := response["data"].(map[string]any)
	raw, err := json.Marshal(data[key])
	if err != nil {
		t.Fatalf("marshal tasks data: %v", err)
	}
	var tasks []scheduled.Task
	if err := json.Unmarshal(raw, &tasks); err != nil {
		t.Fatalf("decode tasks: %v; raw=%s", err, raw)
	}
	return tasks
}

func splitScheduledTmuxCalls(raw string) [][]string {
	var calls [][]string
	for _, block := range strings.Split(strings.TrimSpace(raw), "CALL\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		calls = append(calls, strings.Split(block, "\n"))
	}
	return calls
}

func osWriteFileExecutable(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o755)
}

func osReadFileString(path string) (string, error) {
	raw, err := os.ReadFile(path)
	return string(raw), err
}
