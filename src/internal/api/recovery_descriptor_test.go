package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	recoveryTestOwnerHome    = "/home/alice"
	recoveryTestCodexID      = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	recoveryTestClaudeID     = "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4"
	recoveryTestHermesID     = "hermes-session-20260715T100000Z"
	recoveryTestWindowLayout = "b25f,80x24,0,0[80x12,0,0,1,80x11,0,13,2]"
)

func TestWorkloadRecoveryDescriptorCanonicalAgentCommands(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		id      string
		profile string
		want    string
	}{
		{
			name: "codex uuid resume",
			kind: RecoveryAgentCodex,
			id:   recoveryTestCodexID,
			want: "codex resume " + recoveryTestCodexID,
		},
		{
			name: "claude uuid resume",
			kind: RecoveryAgentClaude,
			id:   recoveryTestClaudeID,
			want: "claude --resume " + recoveryTestClaudeID,
		},
		{
			name:    "hermes profile resume uses managed venv python",
			kind:    RecoveryAgentHermes,
			id:      recoveryTestHermesID,
			profile: "scout",
			want:    "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + recoveryTestHermesID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := recoveryTestAgentDescriptor(tt.kind, tt.id)
			desc.Agent.HermesProfile = tt.profile
			got, err := CanonicalizeWorkloadRecoveryDescriptor(desc, recoveryTestOwnerHome)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if got.Mode != RecoveryModeAgent || got.WorkloadKind != tt.kind {
				t.Fatalf("descriptor mode/kind = %q/%q, want agent/%q", got.Mode, got.WorkloadKind, tt.kind)
			}
			command, ok := got.CanonicalCommand(recoveryTestOwnerHome)
			if !ok || command != tt.want {
				t.Fatalf("canonical command = %q, %v; want %q, true", command, ok, tt.want)
			}
			if tt.kind == RecoveryAgentHermes {
				argv, ok := got.CanonicalArgv(recoveryTestOwnerHome)
				wantArgv := []string{
					"/home/alice/.hermes/hermes-agent-current/venv/bin/python",
					"-m",
					"hermes_cli.main",
					"--profile",
					"scout",
					"--resume",
					recoveryTestHermesID,
				}
				if !ok || !reflect.DeepEqual(argv, wantArgv) {
					t.Fatalf("canonical argv = %#v, %v; want %#v, true", argv, ok, wantArgv)
				}
			}
		})
	}
}

func TestWorkloadRecoveryDescriptorCanonicalPythonHTTPServerCommand(t *testing.T) {
	desc := recoveryTestBaseDescriptor()
	desc.Mode = RecoveryModeCommand
	desc.WorkloadKind = RecoveryWorkloadPythonHTTPServer
	desc.Command = &WorkloadRecoveryCommand{
		Kind: RecoveryCommandPythonHTTPServer,
		PythonHTTPServer: &PythonHTTPServerRecoveryCommand{
			Bind:      "127.0.0.1",
			Port:      8080,
			Directory: "public",
		},
	}
	got, err := CanonicalizeWorkloadRecoveryDescriptor(desc, recoveryTestOwnerHome)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if got.Command.PythonHTTPServer.Directory != "/home/alice/project/public" {
		t.Fatalf("canonical directory = %q, want pane-cwd-resolved directory", got.Command.PythonHTTPServer.Directory)
	}
	command, ok := got.CanonicalCommand(recoveryTestOwnerHome)
	want := "python3 -m http.server 8080 --bind 127.0.0.1 --directory /home/alice/project/public"
	if !ok || command != want {
		t.Fatalf("canonical command = %q, %v; want %q, true", command, ok, want)
	}
	argv, ok := got.CanonicalArgv(recoveryTestOwnerHome)
	wantArgv := []string{"python3", "-m", "http.server", "8080", "--bind", "127.0.0.1", "--directory", "/home/alice/project/public"}
	if !ok || !reflect.DeepEqual(argv, wantArgv) {
		t.Fatalf("canonical argv = %#v, %v; want %#v, true", argv, ok, wantArgv)
	}
}

func TestWorkloadRecoveryDescriptorCanonicalCommandQuotesUnsafePathTokens(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		wantToken string
	}{
		{name: "whitespace", directory: "/home/alice/site with spaces", wantToken: "'/home/alice/site with spaces'"},
		{name: "semicolon", directory: "/home/alice/site;rm", wantToken: "'/home/alice/site;rm'"},
		{name: "backtick", directory: "/home/alice/site`id`", wantToken: "'/home/alice/site`id`'"},
		{name: "subshell", directory: "/home/alice/$(touch pwn)", wantToken: "'/home/alice/$(touch pwn)'"},
		{name: "single quote", directory: "/home/alice/O'Brien", wantToken: "'/home/alice/O'\\''Brien'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := recoveryTestPythonDescriptor("127.0.0.1", 8080, tt.directory)
			got, err := CanonicalizeWorkloadRecoveryDescriptor(desc, recoveryTestOwnerHome)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			argv, ok := got.CanonicalArgv(recoveryTestOwnerHome)
			if !ok || argv[len(argv)-1] != tt.directory {
				t.Fatalf("canonical argv = %#v, %v; want final path token %q", argv, ok, tt.directory)
			}
			command, ok := got.CanonicalCommand(recoveryTestOwnerHome)
			if !ok || !strings.HasSuffix(command, "--directory "+tt.wantToken) {
				t.Fatalf("canonical command = %q, %v; want quoted directory token %q", command, ok, tt.wantToken)
			}
		})
	}
}

func TestWorkloadRecoveryDescriptorRejectsSymlinkEscapingCommandDirectory(t *testing.T) {
	root := t.TempDir()
	ownerHome := filepath.Join(root, "home", "alice")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(ownerHome, 0o755); err != nil {
		t.Fatalf("mkdir owner home: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(ownerHome, "link")); err != nil {
		t.Fatalf("symlink outside into owner home: %v", err)
	}

	escape := recoveryTestPythonDescriptor("127.0.0.1", 8080, filepath.Join(ownerHome, "link", "future-site"))
	if got, err := CanonicalizeWorkloadRecoveryDescriptor(escape, ownerHome); err == nil {
		t.Fatalf("canonicalize accepted symlink escape: %+v", got)
	}

	inHomePrefix := filepath.Join(ownerHome, "project")
	if err := os.MkdirAll(inHomePrefix, 0o755); err != nil {
		t.Fatalf("mkdir in-home prefix: %v", err)
	}
	allowed := recoveryTestPythonDescriptor("127.0.0.1", 8080, filepath.Join(inHomePrefix, "future-site"))
	if _, err := CanonicalizeWorkloadRecoveryDescriptor(allowed, ownerHome); err != nil {
		t.Fatalf("canonicalize rejected in-home future suffix: %v", err)
	}
}

func TestWorkloadRecoveryDescriptorRejectsSymlinkEscapingHermesExecutable(t *testing.T) {
	root := t.TempDir()
	ownerHome := filepath.Join(root, "home", "alice")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(ownerHome, ".hermes"), 0o755); err != nil {
		t.Fatalf("mkdir owner hermes dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "hermes-agent-current"), 0o755); err != nil {
		t.Fatalf("mkdir outside hermes dir: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "hermes-agent-current"), filepath.Join(ownerHome, ".hermes", "hermes-agent-current")); err != nil {
		t.Fatalf("symlink hermes current outside owner home: %v", err)
	}

	escape := recoveryTestAgentDescriptor(RecoveryAgentHermes, recoveryTestHermesID)
	escape.Agent.HermesProfile = "scout"
	if got, err := CanonicalizeWorkloadRecoveryDescriptor(escape, ownerHome); err != nil {
		t.Fatalf("canonicalize hermes descriptor: %v", err)
	} else if argv, ok := got.CanonicalArgv(ownerHome); ok {
		t.Fatalf("canonical argv accepted symlink escape: %#v", argv)
	}

	if err := os.Remove(filepath.Join(ownerHome, ".hermes", "hermes-agent-current")); err != nil {
		t.Fatalf("remove escaping symlink: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(ownerHome, ".hermes", "hermes-agent-current", "venv", "bin"), 0o755); err != nil {
		t.Fatalf("mkdir in-home hermes prefix: %v", err)
	}
	allowed := recoveryTestAgentDescriptor(RecoveryAgentHermes, recoveryTestHermesID)
	allowed.Agent.HermesProfile = "scout"
	if got, err := CanonicalizeWorkloadRecoveryDescriptor(allowed, ownerHome); err != nil {
		t.Fatalf("canonicalize in-home hermes descriptor: %v", err)
	} else if _, ok := got.CanonicalArgv(ownerHome); !ok {
		t.Fatalf("canonical argv rejected in-home hermes future executable")
	}
}

func TestWorkloadRecoveryDescriptorCanonicalNonCommandModes(t *testing.T) {
	tests := []struct {
		name string
		desc WorkloadRecoveryDescriptor
		want string
	}{
		{
			name: "topology shell",
			desc: recoveryTestTopologyDescriptor(),
			want: RecoveryModeTopology,
		},
		{
			name: "managed external owner",
			desc: recoveryTestManagedDescriptor(),
			want: RecoveryModeManaged,
		},
		{
			name: "unresolved reason",
			desc: recoveryTestUnresolvedDescriptor(RecoveryUnresolvedUnknownProcess),
			want: RecoveryModeUnresolved,
		},
		{
			name: "unresolved conflicting evidence reason",
			desc: recoveryTestUnresolvedDescriptor(RecoveryUnresolvedConflictingEvidence),
			want: RecoveryModeUnresolved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeWorkloadRecoveryDescriptor(tt.desc, recoveryTestOwnerHome)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if got.Mode != tt.want {
				t.Fatalf("mode = %q, want %q", got.Mode, tt.want)
			}
			if got.Topology.WindowLayout != recoveryTestWindowLayout {
				t.Fatalf("window layout = %q, want %q", got.Topology.WindowLayout, recoveryTestWindowLayout)
			}
			if command, ok := got.CanonicalCommand(recoveryTestOwnerHome); ok || command != "" {
				t.Fatalf("non-command descriptor produced command %q, %v", command, ok)
			}
		})
	}
}

func TestWorkloadRecoveryDescriptorFixtureSchemaParity(t *testing.T) {
	fixturePaths, err := filepath.Glob("../../../scripts/tmux-recovery/fixtures/*.json")
	if err != nil {
		t.Fatalf("glob owner-probe fixtures: %v", err)
	}
	if len(fixturePaths) == 0 {
		t.Fatalf("no owner-probe fixtures found")
	}

	for _, path := range fixturePaths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var fixture recoveryDescriptorFixture
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("unmarshal fixture: %v", err)
			}
			desc := fixture.wantDescriptor()
			if _, err := CanonicalizeWorkloadRecoveryDescriptor(desc, fixture.Input.OwnerHome); err != nil {
				t.Fatalf("fixture want descriptor does not canonicalize: %v\nwant: %+v", err, desc)
			}
		})
	}
}

func TestWorkloadRecoveryDescriptorRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name string
		desc WorkloadRecoveryDescriptor
	}{
		{
			name: "codex id must be uuid",
			desc: recoveryTestAgentDescriptor(RecoveryAgentCodex, "codex-session;touch-pwn"),
		},
		{
			name: "claude id must be uuid",
			desc: recoveryTestAgentDescriptor(RecoveryAgentClaude, "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4 extra"),
		},
		{
			name: "hermes profile is a slug",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestAgentDescriptor(RecoveryAgentHermes, recoveryTestHermesID)
				desc.Agent.HermesProfile = "../scout"
				return desc
			}(),
		},
		{
			name: "python bind must be loopback",
			desc: recoveryTestPythonDescriptor("0.0.0.0", 8080, "/home/alice/project"),
		},
		{
			name: "python port must be positive",
			desc: recoveryTestPythonDescriptor("127.0.0.1", 0, "/home/alice/project"),
		},
		{
			name: "python port must fit tcp range",
			desc: recoveryTestPythonDescriptor("127.0.0.1", 70000, "/home/alice/project"),
		},
		{
			name: "python directory must stay under owner home",
			desc: recoveryTestPythonDescriptor("127.0.0.1", 8080, "/srv/chrote"),
		},
		{
			name: "raw unresolved argv is rejected by enum",
			desc: recoveryTestUnresolvedDescriptor("node server.js --token secret"),
		},
		{
			name: "window layout rejects control characters",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestTopologyDescriptor()
				desc.Topology.WindowLayout = "80x24,0,0\npwn"
				return desc
			}(),
		},
		{
			name: "window layout is bounded",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestTopologyDescriptor()
				desc.Topology.WindowLayout = strings.Repeat("x", 4097)
				return desc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := CanonicalizeWorkloadRecoveryDescriptor(tt.desc, recoveryTestOwnerHome); err == nil {
				t.Fatalf("canonicalize succeeded with %+v", got)
			}
		})
	}
}

func TestSelectWorkloadRecoveryDescriptorRejectsAmbiguousCandidates(t *testing.T) {
	one := recoveryTestAgentDescriptor(RecoveryAgentCodex, recoveryTestCodexID)
	two := recoveryTestAgentDescriptor(RecoveryAgentClaude, recoveryTestClaudeID)
	two.Owner.Ref = "persistent:alice/claude-alpha"
	two.Owner.Kind = RecoveryOwnerPersistentAgent

	tests := []struct {
		name       string
		candidates []WorkloadRecoveryDescriptor
	}{
		{name: "none", candidates: nil},
		{name: "duplicate candidate", candidates: []WorkloadRecoveryDescriptor{one, one}},
		{name: "conflicting owners", candidates: []WorkloadRecoveryDescriptor{one, two}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := SelectWorkloadRecoveryDescriptor(tt.candidates, recoveryTestOwnerHome); err == nil {
				t.Fatalf("selection succeeded with %+v", got)
			}
		})
	}
}

func TestWorkloadRecoveryDescriptorModeOwnerCombinations(t *testing.T) {
	validLegacyPersistentAgent := recoveryTestAgentDescriptor(RecoveryAgentClaude, recoveryTestClaudeID)
	validLegacyPersistentAgent.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerPersistentAgent, Ref: "persistent:alice/claude-alpha", MayRestart: true}
	if _, err := CanonicalizeWorkloadRecoveryDescriptor(validLegacyPersistentAgent, recoveryTestOwnerHome); err != nil {
		t.Fatalf("legacy persistent agent owner should remain readable for agent mode: %v", err)
	}

	validUnresolved := recoveryTestUnresolvedDescriptor(RecoveryUnresolvedUnknownProcess)
	validUnresolved.Owner.MayRestart = false
	if _, err := CanonicalizeWorkloadRecoveryDescriptor(validUnresolved, recoveryTestOwnerHome); err != nil {
		t.Fatalf("non-restarting unresolved owner should be valid for diagnosis: %v", err)
	}

	tests := []struct {
		name string
		desc WorkloadRecoveryDescriptor
	}{
		{
			name: "agent rejects external owner",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestAgentDescriptor(RecoveryAgentCodex, recoveryTestCodexID)
				desc.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerExternalManager, Ref: "systemd:user/example.service", MayRestart: false}
				return desc
			}(),
		},
		{
			name: "agent requires restart permission",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestAgentDescriptor(RecoveryAgentCodex, recoveryTestCodexID)
				desc.Owner.MayRestart = false
				return desc
			}(),
		},
		{
			name: "command rejects persistent owner",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestPythonDescriptor("127.0.0.1", 8080, "/home/alice/project")
				desc.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerPersistentAgent, Ref: "persistent:alice/static-site", MayRestart: true}
				return desc
			}(),
		},
		{
			name: "command requires restart permission",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestPythonDescriptor("127.0.0.1", 8080, "/home/alice/project")
				desc.Owner.MayRestart = false
				return desc
			}(),
		},
		{
			name: "topology rejects persistent owner",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestTopologyDescriptor()
				desc.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerPersistentAgent, Ref: "persistent:alice/shell", MayRestart: true}
				return desc
			}(),
		},
		{
			name: "managed requires external owner",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestManagedDescriptor()
				desc.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerSessionBank, Ref: "alice/systemd", MayRestart: false}
				return desc
			}(),
		},
		{
			name: "unresolved cannot restart",
			desc: func() WorkloadRecoveryDescriptor {
				desc := recoveryTestUnresolvedDescriptor(RecoveryUnresolvedUnknownProcess)
				desc.Owner.MayRestart = true
				return desc
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := CanonicalizeWorkloadRecoveryDescriptor(tt.desc, recoveryTestOwnerHome); err == nil {
				t.Fatalf("canonicalize succeeded with invalid owner/mode combination: %+v", got)
			}
		})
	}
}

func TestWorkloadRecoveryDescriptorIgnoresStoredResumeCommandOverride(t *testing.T) {
	raw := `{
		"mode":"agent",
		"owner":{"kind":"session_bank","ref":"alice/codex-alpha","mayRestart":true},
		"topology":{"sessionName":"codex-alpha","windowIndex":0,"paneIndex":0,"paneId":"%1","paneCurrentPath":"/home/alice/project"},
		"workloadKind":"codex",
		"agent":{"kind":"codex","nativeSessionId":"` + recoveryTestCodexID + `"},
		"evidenceSource":"argv",
		"confidence":"high",
		"resumeCommand":"rm -rf /"
	}`
	var desc WorkloadRecoveryDescriptor
	if err := json.Unmarshal([]byte(raw), &desc); err != nil {
		t.Fatalf("unmarshal descriptor: %v", err)
	}
	got, err := CanonicalizeWorkloadRecoveryDescriptor(desc, recoveryTestOwnerHome)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	command, ok := got.CanonicalCommand(recoveryTestOwnerHome)
	if !ok || command != "codex resume "+recoveryTestCodexID {
		t.Fatalf("canonical command = %q, %v; want canonical codex resume", command, ok)
	}
	if strings.Contains(command, "rm -rf") {
		t.Fatalf("canonical command used raw stored resume command: %q", command)
	}
}

func TestSessionBankEntryLegacyJSONRoundTripKeepsCurrentFields(t *testing.T) {
	raw := `[{
		"id":"$7",
		"name":"codex-alpha",
		"unixUser":"alice",
		"group":"codex",
		"windows":1,
		"attached":false,
		"live":false,
		"firstSeen":"2026-07-09T00:00:00Z",
		"lastSeen":"2026-07-09T00:00:00Z",
		"recoveryKind":"agent",
		"agentKind":"codex",
		"agentSessionId":"` + recoveryTestCodexID + `",
		"resumeCommand":"codex resume ` + recoveryTestCodexID + `",
		"cwd":"/home/alice/project",
		"transcriptPath":"/home/alice/.codex/sessions/rollout.jsonl"
	}]`
	var entries []SessionBankEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("unmarshal legacy bank json: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "codex-alpha" || entries[0].ResumeCommand == "" || entries[0].TranscriptPath == "" {
		t.Fatalf("legacy entry lost fields: %+v", entries)
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal legacy bank json: %v", err)
	}
	for _, field := range []string{"resumeCommand", "agentSessionId", "transcriptPath"} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("round trip output missing %s: %s", field, encoded)
		}
	}
}

func TestSessionBankEntryRecoveryPlanJSONRoundTripKeepsPaneTopologyAndEvidence(t *testing.T) {
	raw := `[{
		"id":"$7",
		"name":"velis",
		"unixUser":"alice",
		"group":"velis",
		"windows":2,
		"attached":false,
		"live":false,
		"firstSeen":"2026-07-09T00:00:00Z",
		"lastSeen":"2026-07-09T00:00:00Z",
		"recoveryPlan":[{
			"mode":"agent",
			"owner":{"kind":"session_bank","ref":"alice/velis","mayRestart":true},
			"topology":{"sessionName":"velis","windowIndex":1,"windowName":"server","windowLayout":"b25f,120x40,0,0","paneIndex":0,"paneId":"%11","paneCurrentPath":"/home/alice/velis/server"},
			"workloadKind":"codex",
			"agent":{"kind":"codex","nativeSessionId":"` + recoveryTestCodexID + `"},
			"evidenceSource":"argv",
			"confidence":"high"
		}]
	}]`
	var entries []SessionBankEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("unmarshal recovery plan bank json: %v", err)
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal recovery plan bank json: %v", err)
	}
	for _, field := range []string{"recoveryPlan", "windowName", "windowLayout", "paneCurrentPath", "owner", "evidenceSource"} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("round trip output missing %s: %s", field, encoded)
		}
	}
}

func recoveryTestBaseDescriptor() WorkloadRecoveryDescriptor {
	return WorkloadRecoveryDescriptor{
		Owner: WorkloadRecoveryOwner{
			Kind:       RecoveryOwnerSessionBank,
			Ref:        "alice/codex-alpha",
			MayRestart: true,
		},
		Topology: WorkloadRecoveryTopology{
			SessionName:     "codex-alpha",
			WindowIndex:     0,
			WindowName:      "zsh",
			WindowLayout:    recoveryTestWindowLayout,
			PaneIndex:       0,
			PaneID:          "%1",
			PaneCurrentPath: "/home/alice/project",
		},
		EvidenceSource: RecoveryEvidenceArgv,
		Confidence:     RecoveryConfidenceHigh,
	}
}

func recoveryTestAgentDescriptor(kind, id string) WorkloadRecoveryDescriptor {
	desc := recoveryTestBaseDescriptor()
	desc.Mode = RecoveryModeAgent
	desc.WorkloadKind = kind
	desc.Agent = &WorkloadRecoveryAgent{
		Kind:            kind,
		NativeSessionID: id,
	}
	return desc
}

func recoveryTestPythonDescriptor(bind string, port int, directory string) WorkloadRecoveryDescriptor {
	desc := recoveryTestBaseDescriptor()
	desc.Mode = RecoveryModeCommand
	desc.WorkloadKind = RecoveryWorkloadPythonHTTPServer
	desc.Command = &WorkloadRecoveryCommand{
		Kind: RecoveryCommandPythonHTTPServer,
		PythonHTTPServer: &PythonHTTPServerRecoveryCommand{
			Bind:      bind,
			Port:      port,
			Directory: directory,
		},
	}
	return desc
}

func recoveryTestTopologyDescriptor() WorkloadRecoveryDescriptor {
	desc := recoveryTestBaseDescriptor()
	desc.Mode = RecoveryModeTopology
	desc.WorkloadKind = RecoveryWorkloadShell
	desc.EvidenceSource = RecoveryEvidenceTopology
	desc.Confidence = RecoveryConfidenceMedium
	return desc
}

func recoveryTestManagedDescriptor() WorkloadRecoveryDescriptor {
	desc := recoveryTestBaseDescriptor()
	desc.Mode = RecoveryModeManaged
	desc.Owner = WorkloadRecoveryOwner{
		Kind:       RecoveryOwnerExternalManager,
		Ref:        "systemd:user/chrote-srv.service",
		MayRestart: false,
	}
	desc.WorkloadKind = RecoveryWorkloadManaged
	desc.EvidenceSource = RecoveryEvidenceManager
	desc.Confidence = RecoveryConfidenceHigh
	return desc
}

func recoveryTestUnresolvedDescriptor(reason string) WorkloadRecoveryDescriptor {
	desc := recoveryTestBaseDescriptor()
	desc.Mode = RecoveryModeUnresolved
	desc.Owner.MayRestart = false
	desc.WorkloadKind = RecoveryWorkloadUnknown
	desc.UnresolvedReason = reason
	desc.EvidenceSource = RecoveryEvidenceProcess
	desc.Confidence = RecoveryConfidenceLow
	return desc
}

type recoveryDescriptorFixture struct {
	Input recoveryDescriptorFixtureInput `json:"input"`
	Want  WorkloadRecoveryDescriptor     `json:"want"`
}

type recoveryDescriptorFixtureInput struct {
	Owner       WorkloadRecoveryOwner       `json:"owner"`
	OwnerHome   string                      `json:"ownerHome"`
	SessionName string                      `json:"sessionName"`
	Pane        recoveryDescriptorPaneInput `json:"pane"`
}

type recoveryDescriptorPaneInput struct {
	WindowIndex  int    `json:"windowIndex"`
	WindowName   string `json:"windowName"`
	WindowLayout string `json:"windowLayout"`
	PaneIndex    int    `json:"paneIndex"`
	PaneID       string `json:"paneId"`
	CWD          string `json:"cwd"`
}

func (f recoveryDescriptorFixture) wantDescriptor() WorkloadRecoveryDescriptor {
	desc := f.Want
	if desc.Owner.Kind == "" {
		desc.Owner = f.Input.Owner
	}
	desc.Topology = recoveryMergeFixtureTopology(desc.Topology, f.inputTopology())
	return desc
}

func (f recoveryDescriptorFixture) inputTopology() WorkloadRecoveryTopology {
	return WorkloadRecoveryTopology{
		SessionName:     f.Input.SessionName,
		WindowIndex:     f.Input.Pane.WindowIndex,
		WindowName:      f.Input.Pane.WindowName,
		WindowLayout:    f.Input.Pane.WindowLayout,
		PaneIndex:       f.Input.Pane.PaneIndex,
		PaneID:          f.Input.Pane.PaneID,
		PaneCurrentPath: f.Input.Pane.CWD,
	}
}

func recoveryMergeFixtureTopology(desc, input WorkloadRecoveryTopology) WorkloadRecoveryTopology {
	if desc.SessionName == "" {
		desc.SessionName = input.SessionName
	}
	if desc.SessionID == "" {
		desc.SessionID = input.SessionID
	}
	if desc.WindowIndex == 0 {
		desc.WindowIndex = input.WindowIndex
	}
	if desc.WindowName == "" {
		desc.WindowName = input.WindowName
	}
	if desc.WindowLayout == "" {
		desc.WindowLayout = input.WindowLayout
	}
	if desc.PaneIndex == 0 {
		desc.PaneIndex = input.PaneIndex
	}
	if desc.PaneID == "" {
		desc.PaneID = input.PaneID
	}
	if desc.PaneCurrentPath == "" {
		desc.PaneCurrentPath = input.PaneCurrentPath
	}
	return desc
}
