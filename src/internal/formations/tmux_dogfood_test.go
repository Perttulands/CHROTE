package formations

// Real-agent dedicated execution dogfood.
//
// These tests drive the REAL TmuxFormationExecutor and the REAL tmux capture
// path against a REAL tmux server on a dedicated non-cockpit socket. The
// "agent" in each pane is a real but deterministic bash program that waits for
// the dispatched prompt to land in its stdin, then prints a scripted response
// plus the completion sentinel. Nothing about the executor or the tmux client
// is faked here: the only fixture is the in-pane agent, which is a legitimate
// integration stand-in for a coding-agent TUI, not a mock of the code under
// test.
//
// GATING. These tests spawn tmux, so they are inert unless the operator opts in
// with CHROTE_FORMATIONS_DEDICATED_RUN=1. A plain `go test ./...` skips them and
// never force-spawns tmux. Run them explicitly with:
//
//	CHROTE_FORMATIONS_DEDICATED_RUN=1 go test -run TestDogfood ./internal/formations/ -v
//
// GOLDEN RULE. The harness owns its socket and sessions and never touches the
// cockpit/default tmux socket. validateConfiguredBoundary requires Dedicated and
// independently refuses the cockpit socket before any keystroke reaches a real
// session.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const dogfoodOptInEnv = dedicatedRunOptInEnv

// requireDogfoodTmux enforces the honest opt-in/environment gate. It returns the
// absolute path to a real tmux binary; if the opt-in is unset it skips quietly,
// and if the opt-in is set but tmux is missing it skips with a precise message
// naming exactly what is required.
func requireDogfoodTmux(t *testing.T) string {
	t.Helper()
	if strings.TrimSpace(os.Getenv(dogfoodOptInEnv)) == "" {
		t.Skipf("dedicated dogfood disabled; set %s=1 to run it", dogfoodOptInEnv)
	}
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("%s=1 is set but the 'tmux' binary is not on PATH; install tmux (>=3.0) to run the dogfood: %v", dogfoodOptInEnv, err)
	}
	return tmuxPath
}

// dogfoodAgent describes one scripted turn the in-pane bash agent should emit.
// The agent matches on the node id rendered into the prompt and prints the
// configured body (already including its fenced/bare chrote-outputs block and
// any non-sentinel text). emitSentinel controls whether the agent also prints a
// matching completion sentinel for that turn — turning it off lets the dogfood
// prove the completion_sentinel_timeout fail-loud path against a real pane.
type dogfoodAgent struct {
	body         string
	emitSentinel bool
	artifact     string
}

// dogfoodHarness owns a real tmux server, its dedicated socket, workspace/roots,
// and the deterministic in-pane agent script.
type dogfoodHarness struct {
	t        *testing.T
	tmuxPath string
	socket   string
	workdir  string
	store    *Store
	personas *PersonaStore
	scripted map[string]dogfoodAgent // keyed by node id
}

// newDogfoodHarness builds a dedicated non-cockpit boundary and starts a real
// tmux server on that socket. It does NOT start any sessions yet — callers stage
// scripted responses, then call startSession for each session name the executor
// will resolve from its persona/prefix.
func newDogfoodHarness(t *testing.T, tmuxPath string) *dogfoodHarness {
	t.Helper()
	root := nonTempBoundaryDir(t)
	socket := filepath.Join(root, "dogfood-tmux.sock")
	workdir := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("make dogfood workspace: %v", err)
	}

	h := &dogfoodHarness{
		t:        t,
		tmuxPath: tmuxPath,
		socket:   socket,
		workdir:  workdir,
		store:    NewStore(workdir),
		personas: NewPersonaStore(filepath.Join(root, "agents")),
		scripted: map[string]dogfoodAgent{},
	}
	h.store.Now = fixedClock()
	h.personas.Now = fixedClock()

	// Tear the server down no matter how the test exits. kill-server on this
	// dedicated socket can only reach the sessions this harness created.
	t.Cleanup(func() {
		_ = h.tmux(context.Background(), "kill-server")
		_ = os.Remove(socket)
	})

	// Start the server with a placeholder holder session so list-sessions works
	// before any agent session exists. It is killed by kill-server in cleanup.
	if err := h.tmux(context.Background(), "new-session", "-d", "-s", "dogfood-holder", "-x", "240", "-y", "60", "bash --noprofile --norc"); err != nil {
		t.Fatalf("start dedicated tmux server: %v", err)
	}
	return h
}

func (h *dogfoodHarness) tmux(ctx context.Context, args ...string) error {
	h.t.Helper()
	all := append([]string{"-S", h.socket}, args...)
	cmd := exec.CommandContext(ctx, h.tmuxPath, all...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %v: %v: %s", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// agentScript writes the deterministic in-pane agent. It reads pasted prompt
// lines from stdin, learns the run id and node id of the current turn, and when
// the always-present sentinel-instruction marker arrives it prints the scripted
// body for that node followed (optionally) by a matching sentinel. This mirrors
// a real agent that waits for the dispatched prompt before responding.
func (h *dogfoodHarness) agentScript() string {
	h.t.Helper()
	scriptPath := filepath.Join(h.workdir, "dogfood-agent.sh")
	respDir := filepath.Join(h.workdir, "responses")
	if err := os.MkdirAll(respDir, 0o755); err != nil {
		h.t.Fatalf("make responses dir: %v", err)
	}
	// One response file per node id. Each holds the scripted body, a sentinel
	// flag, and an artifact ref. The agent reads <node>.json on each turn so a
	// single long-lived session can answer many sequential dispatches (work,
	// judge-1, judge-2) with the right per-node payload.
	script := `#!/usr/bin/env bash
# Deterministic dogfood agent. NOT a mock of the executor: this is a real
# process producing real terminal output that the real executor captures.
set +e
RESP_DIR=` + shellQuote(respDir) + `
run_id=""
node_id=""
while IFS= read -r line; do
  case "$line" in
    "run: "*) run_id="${line#run: }" ;;
    "node: "*) node_id="${line#node: }" ;;
  esac
  case "$line" in
    "When complete, emit exactly one sentinel line"*)
      resp_file="$RESP_DIR/$node_id.json"
      if [ ! -f "$resp_file" ]; then
        # No scripted turn for this node: stay silent so the executor's
        # completion-sentinel timeout fires loudly instead of getting a fake ok.
        run_id=""; node_id=""
        continue
      fi
      # Parse the per-node response JSON with a dependency-free reader.
      body="$(BODY_FILE="$resp_file" python3 - <<'PY'
import json, os
with open(os.environ["BODY_FILE"]) as fh:
    d = json.load(fh)
print(d.get("body", ""), end="")
PY
)"
      emit_sentinel="$(SENT_FILE="$resp_file" python3 - <<'PY'
import json, os
with open(os.environ["SENT_FILE"]) as fh:
    d = json.load(fh)
print("1" if d.get("emitSentinel") else "0", end="")
PY
)"
      artifact="$(ART_FILE="$resp_file" python3 - <<'PY'
import json, os
with open(os.environ["ART_FILE"]) as fh:
    d = json.load(fh)
print(d.get("artifact", "dogfood.md"), end="")
PY
)"
      printf '%s\n' "$body"
      if [ "$emit_sentinel" = "1" ]; then
        printf '<<<CHROTE-DONE run-id=%s status=ok artifact=%s>>>\n' "$run_id" "$artifact"
      fi
      run_id=""; node_id=""
      ;;
  esac
done
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		h.t.Fatalf("write dogfood agent script: %v", err)
	}
	return scriptPath
}

// stageResponse writes the scripted turn for a node id so the in-pane agent
// answers that node's dispatch with the configured body/sentinel.
func (h *dogfoodHarness) stageResponse(nodeID string, agent dogfoodAgent) {
	h.t.Helper()
	respDir := filepath.Join(h.workdir, "responses")
	if err := os.MkdirAll(respDir, 0o755); err != nil {
		h.t.Fatalf("make responses dir: %v", err)
	}
	if agent.artifact == "" {
		agent.artifact = "dogfood.md"
	}
	raw, err := json.Marshal(map[string]any{
		"body":         agent.body,
		"emitSentinel": agent.emitSentinel,
		"artifact":     agent.artifact,
	})
	if err != nil {
		h.t.Fatalf("marshal staged response: %v", err)
	}
	if err := os.WriteFile(filepath.Join(respDir, nodeID+".json"), raw, 0o644); err != nil {
		h.t.Fatalf("write staged response for %s: %v", nodeID, err)
	}
}

// startSession launches the deterministic agent in a real session whose name is
// exactly what the executor resolves (prefix + persona session stem). Python3
// must exist for the agent's dependency-free JSON reader; gate honestly if not.
func (h *dogfoodHarness) startSession(sessionName string) {
	h.t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		h.t.Skipf("%s=1 is set but 'python3' (needed by the deterministic in-pane agent) is not on PATH: %v", dogfoodOptInEnv, err)
	}
	script := h.agentScript()
	launch := fmt.Sprintf("cd %s && exec bash --noprofile --norc %s", shellQuote(h.workdir), shellQuote(script))
	if err := h.tmux(context.Background(), "new-session", "-d", "-s", sessionName, "-x", "240", "-y", "60", launch); err != nil {
		h.t.Fatalf("start dogfood session %q: %v", sessionName, err)
	}
}

// config builds the executor config pointed at the dedicated non-cockpit socket.
func (h *dogfoodHarness) config(timeoutSeconds int) TmuxExecutorConfig {
	return TmuxExecutorConfig{
		Harnesses:      []string{"openai-codex"},
		Socket:         h.socket,
		Cwd:            h.workdir,
		Roots:          []string{h.workdir},
		SessionPrefix:  "tmux-",
		OutputCapBytes: 1 << 20,
		TimeoutSeconds: timeoutSeconds,
		Dedicated:      true,
	}
}

// fenced wraps a single-line declared-port JSON object in a real chrote-outputs
// markdown fence, as a well-behaved agent emits it.
func fenced(jsonLine string) string {
	return "```chrote-outputs\n" + jsonLine + "\n```"
}

// --- Behavior 1, 2: prompt reaches the pane; fenced chrote-outputs survives
// real capture, parses, and routes to the declared port over a real pane. ---

func TestDogfoodDedicatedTmuxRoutesFencedOutputsOverRealPane(t *testing.T) {
	tmuxPath := requireDogfoodTmux(t)
	h := newDogfoodHarness(t, tmuxPath)
	createS4Persona(t, h.personas, "scout")
	writeFixture(t, h.store.BoardPath("session-search"), s4NamedOutputBoardFixture())

	// The splitter emits two declared ports through a real fence; the two
	// consumers each echo the routed payload they receive, all over one real
	// session (tmux-scout) answering three sequential dispatches.
	h.stageResponse("fmn_split", dogfoodAgent{
		body: "splitter routed two ports\n" + fenced(
			`{"port_split_left":{"text":"LEFT-OVER-REAL-PANE"},"port_split_right":{"text":"RIGHT-OVER-REAL-PANE"}}`),
		emitSentinel: true,
		artifact:     "split.md",
	})
	h.stageResponse("fmn_left", dogfoodAgent{
		body:         "left consumer saw routed payload\n" + fenced(`{"port_left_out":{"text":"LEFT-CONSUMED"}}`),
		emitSentinel: true,
	})
	h.stageResponse("fmn_right", dogfoodAgent{
		body:         "right consumer saw routed payload\n" + fenced(`{"port_right_out":{"text":"RIGHT-CONSUMED"}}`),
		emitSentinel: true,
	})
	h.startSession("tmux-scout")

	executor := NewTmuxFormationExecutor(h.store, h.personas, h.config(20))
	engine := NewRunEngine(h.store, h.personas, executor)
	board, err := h.store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:dogfood",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("run mission over real tmux: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final over real pane", status)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, h.store, "session-search"))

	// Behavior 1: the dispatched prompt actually reached the real pane. The
	// adapter_send event records it, and the captured node_output proves the
	// agent reacted to the prompt content (it only responds after the prompt
	// lands).
	if !eventsContainType(events, RunEventAdapterSend) {
		t.Fatalf("events = %v, want adapter_send proving the prompt reached the pane", eventTypes(events))
	}

	// Behavior 2: the fenced chrote-outputs block survived real capture, parsed,
	// and routed the right payload to each declared port over a real pane.
	splitOutput := findNodeOutputEvent(t, events, "fmn_split")
	outputs, ok := splitOutput.Data["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("split outputs = %#v, want routing map from real capture", splitOutput.Data["outputs"])
	}
	assertOutputPayloadText(t, outputs, "port_split_left", "LEFT-OVER-REAL-PANE")
	assertOutputPayloadText(t, outputs, "port_split_right", "RIGHT-OVER-REAL-PANE")
	if got, want := firstStartedInputText(t, events, "fmn_left"), "LEFT-OVER-REAL-PANE"; got != want {
		t.Fatalf("left routed input over real pane = %q, want %q", got, want)
	}
	if got, want := firstStartedInputText(t, events, "fmn_right"), "RIGHT-OVER-REAL-PANE"; got != want {
		t.Fatalf("right routed input over real pane = %q, want %q", got, want)
	}
}

// --- Behavior 3: fence-stripped capture. A real TUI can render the markdown
// fence away and emit the chrote-outputs payload as BARE JSON keyed by the
// declared port ids. The recovery path (extractDeclaredPortOutputs) must recover
// it exactly, and an undeclared bare port must still block loudly — never a
// silent wrong answer. ---

func TestDogfoodDedicatedTmuxRecoversFenceStrippedBareOutputsOrBlocksLoudly(t *testing.T) {
	tmuxPath := requireDogfoodTmux(t)

	t.Run("recovers declared bare port over real pane", func(t *testing.T) {
		h := newDogfoodHarness(t, tmuxPath)
		createS4Persona(t, h.personas, "scout")
		writeFixture(t, h.store.BoardPath("session-search"), s4RunBoardFixture())
		// No fence: bare single-line JSON keyed by the declared output port id,
		// exactly what a fence-dropping TUI leaves in the scrollback.
		h.stageResponse("fmn_research", dogfoodAgent{
			body:         `done; recovered answer below` + "\n" + `{"port_research_out":{"text":"BARE-RECOVERED-OVER-REAL-PANE"}}`,
			emitSentinel: true,
		})
		h.startSession("tmux-scout")

		executor := NewTmuxFormationExecutor(h.store, h.personas, h.config(20))
		engine := NewRunEngine(h.store, h.personas, executor)
		status, err := engine.RunFormation("session-search", "fmn_research", FormationRunRequest{
			Actor:  "agent:dogfood",
			Limits: RunLimits{MaxDispatch: 3, MaxAttempts: 1},
		})
		if err != nil {
			t.Fatalf("run fence-stripped formation: %v", err)
		}
		if status.Status != RunStatusSucceeded || !status.Final {
			t.Fatalf("status = %+v, want recovery to succeed (not silent wrong answer, not block)", status)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, h.store, "session-search"))
		out := findNodeOutputEvent(t, events, "fmn_research")
		outputs, ok := out.Data["outputs"].(map[string]any)
		if !ok {
			t.Fatalf("research outputs = %#v, want recovered routing map", out.Data["outputs"])
		}
		assertOutputPayloadText(t, outputs, "port_research_out", "BARE-RECOVERED-OVER-REAL-PANE")
	})

	t.Run("undeclared bare port blocks loudly", func(t *testing.T) {
		h := newDogfoodHarness(t, tmuxPath)
		createS4Persona(t, h.personas, "scout")
		writeFixture(t, h.store.BoardPath("session-search"), s4RunBoardFixture())
		// Bare JSON keyed by a port that is NOT declared: the recovery path must
		// reject it (all keys must be declared), so the declared port stays
		// missing and the run blocks loudly with missing_output_payload — never a
		// silent wrong answer.
		h.stageResponse("fmn_research", dogfoodAgent{
			body:         `done; wrong key` + "\n" + `{"port_not_declared":{"text":"SHOULD-NOT-ROUTE"}}`,
			emitSentinel: true,
		})
		h.startSession("tmux-scout")

		executor := NewTmuxFormationExecutor(h.store, h.personas, h.config(20))
		engine := NewRunEngine(h.store, h.personas, executor)
		status, err := engine.RunFormation("session-search", "fmn_research", FormationRunRequest{
			Actor:  "agent:dogfood",
			Limits: RunLimits{MaxDispatch: 3, MaxAttempts: 1},
		})
		if err != nil {
			t.Fatalf("run undeclared-port formation: %v", err)
		}
		if status.Status != RunStatusBlocked || status.Final {
			t.Fatalf("status = %+v, want loud block on undeclared bare port (no silent wrong answer)", status)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, h.store, "session-search"))
		errEvent := eventOfType(t, events, RunEventError)
		if errEvent.Data["code"] != "missing_output_payload" {
			t.Fatalf("error code = %v, want missing_output_payload (block=%#v)", errEvent.Data["code"], errEvent.Data)
		}
	})
}

// --- Behavior 4: when the agent omits the completion sentinel, the run BLOCKS
// with completion_sentinel_timeout over a real pane — it must NOT hang or report
// success. ---

func TestDogfoodDedicatedTmuxMissingSentinelBlocksWithTimeout(t *testing.T) {
	tmuxPath := requireDogfoodTmux(t)
	h := newDogfoodHarness(t, tmuxPath)
	createS4Persona(t, h.personas, "scout")
	writeFixture(t, h.store.BoardPath("session-search"), s4RunBoardFixture())
	// The agent emits a well-formed output block but DELIBERATELY no sentinel.
	h.stageResponse("fmn_research", dogfoodAgent{
		body:         "agent finished work but forgot the sentinel\n" + fenced(`{"port_research_out":{"text":"NO-SENTINEL"}}`),
		emitSentinel: false,
	})
	h.startSession("tmux-scout")

	// Short timeout so the loud block arrives fast; this is the honest deadline,
	// not a hidden fallback.
	executor := NewTmuxFormationExecutor(h.store, h.personas, h.config(2))
	engine := NewRunEngine(h.store, h.personas, executor)

	done := make(chan struct{})
	var status *RunStatusProjection
	var runErr error
	go func() {
		status, runErr = engine.RunFormation("session-search", "fmn_research", FormationRunRequest{
			Actor:  "agent:dogfood",
			Limits: RunLimits{MaxDispatch: 3, MaxAttempts: 1},
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("run hung waiting for an absent sentinel; want a loud completion_sentinel_timeout block, never a hang")
	}
	if runErr != nil {
		t.Fatalf("run missing-sentinel formation: %v", runErr)
	}
	if status.Status != RunStatusBlocked || status.Final {
		t.Fatalf("status = %+v, want blocked (never succeeded) when sentinel is absent", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, h.store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "completion_sentinel_timeout" {
		t.Fatalf("error code = %v, want completion_sentinel_timeout (event=%#v)", errEvent.Data["code"], errEvent.Data)
	}
}

// --- Behavior 5: a judge gate driven by a real pane returns strict pass/fail
// and routes; an ambiguous pane verdict blocks loudly (ambiguous_gate_verdict).
// Respects the A1 strict-verdict contract. ---

func TestDogfoodDedicatedTmuxJudgeGateStrictVerdictRoutesAndBlocksAmbiguous(t *testing.T) {
	tmuxPath := requireDogfoodTmux(t)

	// The gate strictly parses the judge formation's final DISPLAY text (the
	// judge chain returns result.Text as the verdict, then parseStrictVerdict
	// requires exactly "pass"/"fail"). So a real judge agent must make its
	// free-form output BE the verdict word; the declared output port still has to
	// carry a payload because the formation declares port_j2_out. judgeBody
	// builds both: the verdict word as the display text plus a matching fenced
	// port payload.
	judgeBody := func(verdict string) string {
		return verdict + "\n" + fenced(`{"port_j2_out":{"text":`+jsonString(verdict)+`}}`)
	}

	t.Run("exact pass routes ship over real panes", func(t *testing.T) {
		h := newDogfoodHarness(t, tmuxPath)
		createS4Persona(t, h.personas, "scout")
		writeFixture(t, h.store.BoardPath("session-search"), s4JudgeChainRunBoardFixture())
		h.stageResponse("fmn_work", dogfoodAgent{body: "work done\n" + fenced(`{"port_work_out":{"text":"WORK-ARTIFACT"}}`), emitSentinel: true})
		h.stageResponse("fmn_j1", dogfoodAgent{body: "judge 1 notes\n" + fenced(`{"port_j1_out":{"text":"reviewed"}}`), emitSentinel: true})
		h.stageResponse("fmn_j2", dogfoodAgent{body: judgeBody("pass"), emitSentinel: true})
		h.stageResponse("fmn_ship", dogfoodAgent{body: "shipped\n" + fenced(`{"port_ship_out":{"text":"SHIPPED-OVER-REAL-PANE"}}`), emitSentinel: true})
		h.startSession("tmux-scout")

		status, events := h.runJudgeMission(t, 30)
		if status.Status != RunStatusSucceeded || !status.Final {
			t.Fatalf("status = %+v, want succeeded final on exact pass verdict", status)
		}
		verdict := lastEventOfType(t, events, RunEventGateVerdict)
		if verdict.Data["verdict"] != "pass" {
			t.Fatalf("gate verdict = %v, want pass routed (event=%#v)", verdict.Data["verdict"], verdict.Data)
		}
		if !eventNodeRan(events, "fmn_ship") {
			t.Fatalf("pass verdict did not route to ship; nodes=%v", nodeOutputIDs(events))
		}
	})

	t.Run("exact fail routes revise over real panes", func(t *testing.T) {
		h := newDogfoodHarness(t, tmuxPath)
		createS4Persona(t, h.personas, "scout")
		writeFixture(t, h.store.BoardPath("session-search"), s4JudgeChainRunBoardFixture(true))
		h.stageResponse("fmn_work", dogfoodAgent{body: "work done\n" + fenced(`{"port_work_out":{"text":"WORK-ARTIFACT"}}`), emitSentinel: true})
		h.stageResponse("fmn_j1", dogfoodAgent{body: "judge 1 notes\n" + fenced(`{"port_j1_out":{"text":"reviewed"}}`), emitSentinel: true})
		h.stageResponse("fmn_j2", dogfoodAgent{body: judgeBody("fail"), emitSentinel: true})
		h.stageResponse("fmn_revise", dogfoodAgent{body: "revising\n" + fenced(`{"port_revise_out":{"text":"REVISED-OVER-REAL-PANE"}}`), emitSentinel: true})
		h.startSession("tmux-scout")

		status, events := h.runJudgeMission(t, 30)
		if status.Status != RunStatusSucceeded || !status.Final {
			t.Fatalf("status = %+v, want succeeded final on exact fail verdict routing revise", status)
		}
		verdict := lastEventOfType(t, events, RunEventGateVerdict)
		if verdict.Data["verdict"] != "fail" {
			t.Fatalf("gate verdict = %v, want fail routed (event=%#v)", verdict.Data["verdict"], verdict.Data)
		}
		if !eventNodeRan(events, "fmn_revise") {
			t.Fatalf("fail verdict did not route to revise; nodes=%v", nodeOutputIDs(events))
		}
		if eventNodeRan(events, "fmn_ship") {
			t.Fatalf("fail verdict wrongly routed to ship; nodes=%v", nodeOutputIDs(events))
		}
	})

	t.Run("ambiguous verdict blocks loudly", func(t *testing.T) {
		h := newDogfoodHarness(t, tmuxPath)
		createS4Persona(t, h.personas, "scout")
		writeFixture(t, h.store.BoardPath("session-search"), s4JudgeChainRunBoardFixture())
		h.stageResponse("fmn_work", dogfoodAgent{body: "work done\n" + fenced(`{"port_work_out":{"text":"WORK-ARTIFACT"}}`), emitSentinel: true})
		h.stageResponse("fmn_j1", dogfoodAgent{body: "judge 1 notes\n" + fenced(`{"port_j1_out":{"text":"reviewed"}}`), emitSentinel: true})
		// Ambiguous prose, not exactly "pass"/"fail" — A1 must block loudly.
		h.stageResponse("fmn_j2", dogfoodAgent{body: judgeBody("looks good to me"), emitSentinel: true})
		h.stageResponse("fmn_ship", dogfoodAgent{body: "shipped\n" + fenced(`{"port_ship_out":{"text":"SHOULD-NOT-SHIP"}}`), emitSentinel: true})
		h.startSession("tmux-scout")

		status, events := h.runJudgeMission(t, 30)
		if status.Status != RunStatusBlocked || status.Final {
			t.Fatalf("status = %+v, want blocked non-final on ambiguous verdict", status)
		}
		if eventNodeRan(events, "fmn_ship") {
			t.Fatalf("ambiguous verdict silently routed pass to ship; nodes=%v", nodeOutputIDs(events))
		}
		for _, ev := range events {
			if ev.Type == RunEventGateVerdict {
				t.Fatalf("gate verdict recorded for ambiguous output: %+v", ev)
			}
		}
		block := lastEventOfType(t, events, RunEventBlocked)
		if block.Data["code"] != "ambiguous_gate_verdict" {
			t.Fatalf("block code = %v, want ambiguous_gate_verdict (block=%#v)", block.Data["code"], block.Data)
		}
		reason, _ := block.Data["reason"].(string)
		if !strings.Contains(reason, "gate_review") || !strings.Contains(reason, `expected exactly "pass" or "fail"`) {
			t.Fatalf("block reason = %q, want precise expected-token message naming the gate", reason)
		}
	})
}

func (h *dogfoodHarness) runJudgeMission(t *testing.T, timeoutSeconds int) (*RunStatusProjection, []RunEvent) {
	t.Helper()
	executor := NewTmuxFormationExecutor(h.store, h.personas, h.config(timeoutSeconds))
	engine := NewRunEngine(h.store, h.personas, executor)
	board, err := h.store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read judge board: %v", err)
	}
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:dogfood",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 12, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("run judge mission over real tmux: %v", err)
	}
	return status, readRunEvents(t, findOnlyRunLedger(t, h.store, "session-search"))
}

// --- Behavior 6: resume after a simulated disconnect rebuilds run state purely
// from the durable ledger. We drive a real run to a loud block (absent
// sentinel), then prove a FRESH Store/engine built from the same workspace
// rebuilds the blocked projection and the resume contract from persisted events
// alone — no in-memory carryover survives a disconnect. ---

func TestDogfoodDedicatedTmuxResumeRebuildsRunStateFromLedger(t *testing.T) {
	tmuxPath := requireDogfoodTmux(t)
	h := newDogfoodHarness(t, tmuxPath)
	createS4Persona(t, h.personas, "scout")
	writeFixture(t, h.store.BoardPath("session-search"), s4RunBoardFixture())
	// Real run that blocks loudly on a missing sentinel, leaving a resumable
	// blocked run persisted in the ledger.
	h.stageResponse("fmn_research", dogfoodAgent{
		body:         "work emitted, sentinel withheld to force a resumable block\n" + fenced(`{"port_research_out":{"text":"PRE-DISCONNECT"}}`),
		emitSentinel: false,
	})
	h.startSession("tmux-scout")

	executor := NewTmuxFormationExecutor(h.store, h.personas, h.config(2))
	engine := NewRunEngine(h.store, h.personas, executor)
	status, err := engine.RunFormation("session-search", "fmn_research", FormationRunRequest{
		Actor:  "agent:dogfood",
		Limits: RunLimits{MaxDispatch: 3, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("run pre-disconnect formation: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("pre-disconnect status = %+v, want blocked before resume", status)
	}
	runID := status.RunID
	if runID == "" {
		t.Fatalf("blocked projection missing run id: %+v", status)
	}

	// Simulate a disconnect: throw away the in-memory store/engine and rebuild
	// everything from the persisted workspace on disk.
	rebuiltStore := NewStore(h.workdir)
	rebuiltStore.Now = fixedClock()

	rebuilt, err := rebuiltStore.ProjectRun(runID)
	if err != nil {
		t.Fatalf("project run from rebuilt store: %v", err)
	}
	if rebuilt.Status != RunStatusBlocked || rebuilt.Final {
		t.Fatalf("rebuilt projection = %+v, want blocked non-final reconstructed from ledger", rebuilt)
	}
	if !rebuilt.ResumeAllowed {
		t.Fatalf("rebuilt projection = %+v, want resumeAllowed reconstructed from ledger", rebuilt)
	}
	if rebuilt.RunID != runID {
		t.Fatalf("rebuilt run id = %q, want %q", rebuilt.RunID, runID)
	}

	// Resuming through the rebuilt store must advance the epoch and persist a
	// run_resumed event, proving recovery does not depend on the original
	// process staying alive.
	beforeEpoch := rebuilt.Epoch
	resumed, err := rebuiltStore.ResumeRun(runID, RunResumeRequest{
		Actor:  "agent:dogfood",
		Mode:   "reattach",
		Reason: "operator confirmed recovery after disconnect",
	})
	if err != nil {
		// A blocked run that carries open dispatches is resume-allowed but
		// reattach-fails loudly; either way it must not silently succeed.
		if errors.Is(err, ErrRunResumeNotAllowed) {
			t.Fatalf("resume rejected a resumable blocked run rebuilt from ledger: %v", err)
		}
		t.Fatalf("resume from rebuilt store: %v", err)
	}
	if resumed.Epoch <= beforeEpoch {
		t.Fatalf("resumed epoch = %d, want greater than blocked epoch %d", resumed.Epoch, beforeEpoch)
	}
	events, err := rebuiltStore.ReadRunEvents(runID)
	if err != nil {
		t.Fatalf("read rebuilt run events: %v", err)
	}
	if !eventsContainType(events, RunEventResumed) {
		t.Fatalf("rebuilt ledger missing run_resumed after resume: %v", eventTypes(events))
	}
}

// --- small dependency-free helpers local to the dogfood ---

func jsonString(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(raw)
}

func eventNodeRan(events []RunEvent, nodeID string) bool {
	for _, ev := range events {
		if ev.Type == RunEventNodeOutput && ev.NodeID == nodeID {
			return true
		}
	}
	return false
}

func nodeOutputIDs(events []RunEvent) []string {
	var ids []string
	for _, ev := range events {
		if ev.Type == RunEventNodeOutput {
			ids = append(ids, ev.NodeID)
		}
	}
	return ids
}
