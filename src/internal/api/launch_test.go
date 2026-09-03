package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testLaunchConfigJSON holds both shapes an operator can have written:
// claude-code is a legacy entry with its flags inside the command, codex is a
// split entry with a refreshed catalogue.
const testLaunchConfigJSON = `{
  "harnesses": [
    {"id": "claude-code", "label": "Claude Code", "command": "claude --harness-flag"},
    {"id": "codex", "label": "Codex", "command": "codex", "defaultFlags": "--harness-flag",
     "flags": [
       {"name": "--model", "short": "-m", "value": "<model>", "description": "Model for the current session", "values": ["fast", "slow"]},
       {"name": "--search", "description": "Enable web search"}
     ]},
    {"id": "shell", "label": "Shell", "command": ""}
  ],
  "folders": ["/srv/work", "/srv", "~"]
}`

func writeLaunchConfigFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "launch.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write launch config: %v", err)
	}
	return path
}

func TestLoadLaunchConfig(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		// unconfigured asks for the unset-variable case instead of a file.
		unconfigured bool
		// unreadable points the loader at a path that does not exist.
		unreadable bool
		wantErr    bool
		want       LaunchConfig
	}{
		{
			name:         "unset offers only the shell in the user's home",
			unconfigured: true,
			want: LaunchConfig{
				Harnesses: []LaunchHarness{{ID: "shell", Label: "Shell"}},
				Folders:   []string{"~"},
			},
		},
		{
			name:     "a command carrying flags is split into a binary and defaults",
			contents: testLaunchConfigJSON,
			want: LaunchConfig{
				Harnesses: []LaunchHarness{
					{ID: "claude-code", Label: "Claude Code", Command: "claude", DefaultFlags: "--harness-flag"},
					{ID: "codex", Label: "Codex", Command: "codex", DefaultFlags: "--harness-flag", Flags: []LaunchFlag{
						{Name: "--model", Short: "-m", Value: "<model>", Description: "Model for the current session", Values: []string{"fast", "slow"}},
						{Name: "--search", Description: "Enable web search"},
					}},
					{ID: "shell", Label: "Shell"},
				},
				Folders: []string{"/srv/work", "/srv", "~"},
			},
		},
		{
			name:     "a missing shell harness is added",
			contents: `{"harnesses":[{"id":"codex","label":"Codex","command":"codex"}],"folders":["/srv/work"]}`,
			want: LaunchConfig{
				Harnesses: []LaunchHarness{
					{ID: "codex", Label: "Codex", Command: "codex"},
					{ID: "shell", Label: "Shell"},
				},
				Folders: []string{"/srv/work"},
			},
		},
		{
			name:     "an empty folder list falls back to the user's home",
			contents: `{"harnesses":[],"folders":[]}`,
			want: LaunchConfig{
				Harnesses: []LaunchHarness{{ID: "shell", Label: "Shell"}},
				Folders:   []string{"~"},
			},
		},
		{
			name:     "an id outside the allowed alphabet fails",
			contents: `{"harnesses":[{"id":"Claude Code","label":"Claude Code","command":"claude"}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "a duplicate id fails",
			contents: `{"harnesses":[{"id":"codex","label":"Codex","command":"codex"},{"id":"codex","label":"Codex 2","command":"codex2"}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "a harness without a label fails",
			contents: `{"harnesses":[{"id":"codex","label":"  ","command":"codex"}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "a shell harness with a command fails",
			contents: `{"harnesses":[{"id":"shell","label":"Shell","command":"claude"}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "a shell harness with default flags fails",
			contents: `{"harnesses":[{"id":"shell","label":"Shell","command":"","defaultFlags":"--model fast"}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "a shell harness with a flag catalogue fails",
			contents: `{"harnesses":[{"id":"shell","label":"Shell","command":"","flags":[{"name":"--model","description":"Model"}]}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "a catalogue flag that is not a flag fails",
			contents: `{"harnesses":[{"id":"codex","label":"Codex","command":"codex","flags":[{"name":"model","description":"Model"}]}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "a catalogue flag without a description fails",
			contents: `{"harnesses":[{"id":"codex","label":"Codex","command":"codex","flags":[{"name":"--model","description":"  "}]}],"folders":["/srv/work"]}`,
			wantErr:  true,
		},
		{
			name:     "an empty folder fails",
			contents: `{"harnesses":[{"id":"codex","label":"Codex","command":"codex"}],"folders":["/srv/work",""]}`,
			wantErr:  true,
		},
		{
			name:     "unparsable JSON fails",
			contents: `{"harnesses":`,
			wantErr:  true,
		},
		{
			name:       "an unreadable file fails",
			unreadable: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := ""
			switch {
			case tt.unconfigured:
				path = ""
			case tt.unreadable:
				path = filepath.Join(t.TempDir(), "absent", "launch.json")
			default:
				path = writeLaunchConfigFile(t, tt.contents)
			}

			got, err := LoadLaunchConfig(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadLaunchConfig(%q) = %#v, want an error", path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadLaunchConfig(%q): %v", path, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("LoadLaunchConfig(%q) = %#v, want %#v", path, got, tt.want)
			}
		})
	}
}

func TestLaunchHandlerServesTheBinaryDefaultFlagsAndCatalogue(t *testing.T) {
	config, err := LoadLaunchConfig(writeLaunchConfigFile(t, testLaunchConfigJSON))
	if err != nil {
		t.Fatalf("load launch config: %v", err)
	}
	handler := NewLaunchHandler(config)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/launch", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, `"command"`) {
		t.Fatalf("GET /api/launch leaked a harness command: %s", body)
	}

	var response struct {
		Harnesses []map[string]any `json:"harnesses"`
		Folders   []string         `json:"folders"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode /api/launch: %v; body=%s", err, body)
	}
	// Comparing whole objects keeps the browser's view exact: an added field
	// is a leak and a missing one is a launcher that cannot offer flags.
	var wantHarnesses []map[string]any
	if err := json.Unmarshal([]byte(`[
	  {"id": "claude-code", "label": "Claude Code", "binary": "claude", "defaultFlags": "--harness-flag", "flags": []},
	  {"id": "codex", "label": "Codex", "binary": "codex", "defaultFlags": "--harness-flag", "flags": [
	    {"name": "--model", "short": "-m", "value": "<model>", "description": "Model for the current session", "values": ["fast", "slow"]},
	    {"name": "--search", "description": "Enable web search"}
	  ]},
	  {"id": "shell", "label": "Shell", "binary": "", "defaultFlags": "", "flags": []}
	]`), &wantHarnesses); err != nil {
		t.Fatalf("decode the expected harnesses: %v", err)
	}
	if !reflect.DeepEqual(response.Harnesses, wantHarnesses) {
		t.Fatalf("harnesses = %#v, want %#v", response.Harnesses, wantHarnesses)
	}
	if want := []string{"/srv/work", "/srv", "~"}; !reflect.DeepEqual(response.Folders, want) {
		t.Fatalf("folders = %#v, want %#v", response.Folders, want)
	}

	var topLevel map[string]any
	if err := json.Unmarshal([]byte(body), &topLevel); err != nil {
		t.Fatalf("decode /api/launch envelope: %v", err)
	}
	assertTopLevelKeys(t, topLevel, []string{"folders", "harnesses"})
}

func TestResolveLaunchCwd(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatalf("look up the running account: %v", err)
	}

	tests := []struct {
		name      string
		requested string
		unixUser  string
		want      string
		wantErr   bool
	}{
		{
			name:      "empty keeps the configured workdir",
			requested: "",
			unixUser:  current.Username,
			want:      "/srv/configured",
		},
		{
			name:      "the home token is the target user's home",
			requested: "~",
			unixUser:  current.Username,
			want:      current.HomeDir,
		},
		{
			name:      "a path under the home token joins that home",
			requested: "~/projects/one",
			unixUser:  current.Username,
			want:      filepath.Join(current.HomeDir, "projects", "one"),
		},
		{
			name:      "an absolute path is taken as asked",
			requested: "/srv/work/thing",
			unixUser:  current.Username,
			want:      "/srv/work/thing",
		},
		{
			name:      "a relative path is refused",
			requested: "work/thing",
			unixUser:  current.Username,
			wantErr:   true,
		},
		{
			name:      "a relative path is refused",
			requested: "work/one",
			unixUser:  current.Username,
			wantErr:   true,
		},
		{
			name:      "the home token needs a resolvable user",
			requested: "~",
			unixUser:  "chrote-account-that-cannot-exist",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLaunchCwd(tt.requested, tt.unixUser, "/srv/configured")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveLaunchCwd(%q) = %q, want an error", tt.requested, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveLaunchCwd(%q): %v", tt.requested, err)
			}
			if got != tt.want {
				t.Fatalf("resolveLaunchCwd(%q) = %q, want %q", tt.requested, got, tt.want)
			}
		})
	}
}

func TestResolveHarness(t *testing.T) {
	config, err := LoadLaunchConfig(writeLaunchConfigFile(t, testLaunchConfigJSON))
	if err != nil {
		t.Fatalf("load launch config: %v", err)
	}

	flagsLine := func(line string) *string { return &line }

	tests := []struct {
		name      string
		requested string
		// flags is nil when the request carried no flags field at all.
		flags       *string
		want        resolvedLaunch
		wantErr     bool
		wantErrText string
	}{
		{name: "absent means the bare shell", requested: "", want: resolvedLaunch{harnessID: "shell"}},
		{name: "the shell starts nothing", requested: "shell", want: resolvedLaunch{harnessID: "shell"}},
		{
			name:      "absent flags mean the harness defaults",
			requested: "claude-code",
			want:      resolvedLaunch{harnessID: "claude-code", command: "claude --harness-flag", flags: "--harness-flag"},
		},
		{
			name:      "an empty line means the binary alone",
			requested: "claude-code",
			flags:     flagsLine(""),
			want:      resolvedLaunch{harnessID: "claude-code", command: "claude"},
		},
		{
			name:      "a requested line replaces the defaults",
			requested: "claude-code",
			flags:     flagsLine("--model fast --verbose"),
			want:      resolvedLaunch{harnessID: "claude-code", command: "claude --model fast --verbose", flags: "--model fast --verbose"},
		},
		{
			name:      "surrounding whitespace is not a flag",
			requested: "codex",
			flags:     flagsLine("  -m fast  "),
			want:      resolvedLaunch{harnessID: "codex", command: "codex -m fast", flags: "-m fast"},
		},
		{
			name:        "a control character is refused",
			requested:   "claude-code",
			flags:       flagsLine("--model fast\nrm -rf /"),
			wantErr:     true,
			wantErrText: flagsNotOneLineMessage,
		},
		{
			name:        "a line past the bound is refused",
			requested:   "claude-code",
			flags:       flagsLine(strings.Repeat("-", maxLaunchFlagsBytes+1)),
			wantErr:     true,
			wantErrText: flagsNotOneLineMessage,
		},
		{name: "the shell takes no flags", requested: "shell", flags: flagsLine("--model fast"), wantErr: true},
		{name: "an unknown id is refused", requested: "emacs", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.resolveHarness(tt.requested, tt.flags)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveHarness(%q) = %#v, want an error", tt.requested, got)
				}
				if tt.wantErrText != "" && err.Error() != tt.wantErrText {
					t.Fatalf("resolveHarness(%q) failed with %q, want %q", tt.requested, err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveHarness(%q): %v", tt.requested, err)
			}
			if got != tt.want {
				t.Fatalf("resolveHarness(%q) = %#v, want %#v", tt.requested, got, tt.want)
			}
		})
	}
}
