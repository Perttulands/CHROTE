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

const testLaunchConfigJSON = `{
  "harnesses": [
    {"id": "claude-code", "label": "Claude Code", "command": "claude --harness-flag"},
    {"id": "codex", "label": "Codex", "command": "codex --harness-flag"},
    {"id": "shell", "label": "Shell", "command": ""}
  ],
  "folders": ["/srv/work", "/srv", "~~"]
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
				Folders:   []string{"~~"},
			},
		},
		{
			name:     "valid file is taken as written",
			contents: testLaunchConfigJSON,
			want: LaunchConfig{
				Harnesses: []LaunchHarness{
					{ID: "claude-code", Label: "Claude Code", Command: "claude --harness-flag"},
					{ID: "codex", Label: "Codex", Command: "codex --harness-flag"},
					{ID: "shell", Label: "Shell"},
				},
				Folders: []string{"/srv/work", "/srv", "~~"},
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
				Folders:   []string{"~~"},
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

func TestLaunchHandlerServesIdsAndLabelsWithoutCommands(t *testing.T) {
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
	if strings.Contains(body, "--harness-flag") {
		t.Fatalf("GET /api/launch leaked a harness command: %s", body)
	}

	var response struct {
		Harnesses []map[string]any `json:"harnesses"`
		Folders   []string         `json:"folders"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode /api/launch: %v; body=%s", err, body)
	}
	wantHarnesses := []map[string]any{
		{"id": "claude-code", "label": "Claude Code"},
		{"id": "codex", "label": "Codex"},
		{"id": "shell", "label": "Shell"},
	}
	if !reflect.DeepEqual(response.Harnesses, wantHarnesses) {
		t.Fatalf("harnesses = %#v, want %#v", response.Harnesses, wantHarnesses)
	}
	if want := []string{"/srv/work", "/srv", "~~"}; !reflect.DeepEqual(response.Folders, want) {
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
			requested: "~~",
			unixUser:  current.Username,
			want:      current.HomeDir,
		},
		{
			name:      "a path under the home token joins that home",
			requested: "~~/projects/one",
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
			name:      "a single tilde is refused",
			requested: "~/work",
			unixUser:  current.Username,
			wantErr:   true,
		},
		{
			name:      "the home token needs a resolvable user",
			requested: "~~",
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

	tests := []struct {
		name        string
		requested   string
		wantID      string
		wantCommand string
		wantErr     bool
	}{
		{name: "absent means the bare shell", requested: "", wantID: "shell"},
		{name: "the shell starts nothing", requested: "shell", wantID: "shell"},
		{name: "a configured harness carries its command", requested: "claude-code", wantID: "claude-code", wantCommand: "claude --harness-flag"},
		{name: "an unknown id is refused", requested: "emacs", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotCommand, err := config.resolveHarness(tt.requested)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveHarness(%q) = %q/%q, want an error", tt.requested, gotID, gotCommand)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveHarness(%q): %v", tt.requested, err)
			}
			if gotID != tt.wantID || gotCommand != tt.wantCommand {
				t.Fatalf("resolveHarness(%q) = %q/%q, want %q/%q", tt.requested, gotID, gotCommand, tt.wantID, tt.wantCommand)
			}
		})
	}
}
