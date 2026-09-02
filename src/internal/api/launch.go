package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chrote/server/internal/core"
)

// shellHarnessID names the harness that starts nothing: the session is the
// login shell tmux already gives it. It always exists, so a request that names
// no harness always resolves.
const shellHarnessID = "shell"

// homeDirToken stands for the target Unix user's home directory in a launch
// folder or a create-session cwd. The server resolves it against that user's
// passwd entry, never against its own $HOME: CHROTE runs as its own account.
const homeDirToken = "~"

var launchHarnessIDPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// LaunchHarness is one thing the operator can start in a new session. The
// command is server-side configuration and never reaches the browser; the
// browser asks for a harness by id.
type LaunchHarness struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Command string `json:"command"`
}

// LaunchConfig is what the launcher may offer: which harnesses can be started
// and which folders are worth listing. It is read once at startup from
// CHROTE_LAUNCH_CONFIG.
type LaunchConfig struct {
	Harnesses []LaunchHarness `json:"harnesses"`
	Folders   []string        `json:"folders"`
}

// LaunchHarnessOption is the browser's view of a harness: what to show and
// what to send back.
type LaunchHarnessOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// LaunchOptionsResponse is the body of GET /api/launch.
type LaunchOptionsResponse struct {
	Harnesses []LaunchHarnessOption `json:"harnesses"`
	Folders   []string              `json:"folders"`
}

// DefaultLaunchConfig is what CHROTE offers when no launch file is configured:
// a shell in the target user's home.
func DefaultLaunchConfig() LaunchConfig {
	return LaunchConfig{
		Harnesses: []LaunchHarness{{ID: shellHarnessID, Label: "Shell"}},
		Folders:   []string{homeDirToken},
	}
}

// LoadLaunchConfig reads the launch configuration named by path, which is the
// value of CHROTE_LAUNCH_CONFIG. An empty path means "not configured" and
// yields the default; every other failure is an operator mistake and is
// returned so startup can refuse to run on a launcher nobody can use.
func LoadLaunchConfig(path string) (LaunchConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return DefaultLaunchConfig(), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return LaunchConfig{}, fmt.Errorf("read launch config %q: %w", path, err)
	}
	var config LaunchConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return LaunchConfig{}, fmt.Errorf("parse launch config %q: %w", path, err)
	}
	if err := validateLaunchConfig(config); err != nil {
		return LaunchConfig{}, fmt.Errorf("launch config %q: %w", path, err)
	}
	return withLaunchDefaults(config), nil
}

func validateLaunchConfig(config LaunchConfig) error {
	seen := map[string]bool{}
	for index, harness := range config.Harnesses {
		id := strings.TrimSpace(harness.ID)
		if !launchHarnessIDPattern.MatchString(id) {
			return fmt.Errorf("harness %d has id %q, which must match %s", index, harness.ID, launchHarnessIDPattern)
		}
		if seen[id] {
			return fmt.Errorf("harness id %q appears more than once", id)
		}
		seen[id] = true
		if strings.TrimSpace(harness.Label) == "" {
			return fmt.Errorf("harness %q has no label", id)
		}
		if id == shellHarnessID && strings.TrimSpace(harness.Command) != "" {
			return fmt.Errorf("harness %q must have an empty command; it is the bare login shell", id)
		}
	}
	for index, folder := range config.Folders {
		if strings.TrimSpace(folder) == "" {
			return fmt.Errorf("folder %d is empty", index)
		}
	}
	return nil
}

// withLaunchDefaults fills in what every launcher needs regardless of what the
// operator wrote: a shell harness and somewhere to start it.
func withLaunchDefaults(config LaunchConfig) LaunchConfig {
	harnesses := make([]LaunchHarness, 0, len(config.Harnesses)+1)
	hasShell := false
	for _, harness := range config.Harnesses {
		harness.ID = strings.TrimSpace(harness.ID)
		harness.Label = strings.TrimSpace(harness.Label)
		harness.Command = strings.TrimSpace(harness.Command)
		if harness.ID == shellHarnessID {
			hasShell = true
		}
		harnesses = append(harnesses, harness)
	}
	if !hasShell {
		harnesses = append(harnesses, LaunchHarness{ID: shellHarnessID, Label: "Shell"})
	}
	folders := make([]string, 0, len(config.Folders))
	for _, folder := range config.Folders {
		folders = append(folders, strings.TrimSpace(folder))
	}
	if len(folders) == 0 {
		folders = []string{homeDirToken}
	}
	return LaunchConfig{Harnesses: harnesses, Folders: folders}
}

// options is what the browser is allowed to know: ids, labels and folders.
func (c LaunchConfig) options() LaunchOptionsResponse {
	harnesses := make([]LaunchHarnessOption, 0, len(c.Harnesses))
	for _, harness := range c.Harnesses {
		harnesses = append(harnesses, LaunchHarnessOption{ID: harness.ID, Label: harness.Label})
	}
	folders := append([]string(nil), c.Folders...)
	if folders == nil {
		folders = []string{}
	}
	return LaunchOptionsResponse{Harnesses: harnesses, Folders: folders}
}

// resolveHarness turns a requested harness id into the id to report and the
// command to run. An absent id and the shell run nothing.
func (c LaunchConfig) resolveHarness(requested string) (string, string, error) {
	id := strings.TrimSpace(requested)
	if id == "" {
		id = shellHarnessID
	}
	for _, harness := range c.Harnesses {
		if harness.ID == id {
			return harness.ID, harness.Command, nil
		}
	}
	return "", "", fmt.Errorf("unknown harness %q", requested)
}

// resolveLaunchCwd decides the directory a new session starts in. An empty
// request keeps today's per-user working directory; the home token resolves
// against the target Unix user's passwd entry; an absolute path is taken as
// asked, because broad access inside the configured roots is intentional.
func resolveLaunchCwd(requested, unixUser, defaultWorkDir string) (string, error) {
	cwd := strings.TrimSpace(requested)
	switch {
	case cwd == "":
		return defaultWorkDir, nil
	case cwd == homeDirToken || strings.HasPrefix(cwd, homeDirToken+"/"):
		home, err := unixUserHomeDir(unixUser)
		if err != nil {
			return "", err
		}
		return filepath.Clean(filepath.Join(home, strings.TrimPrefix(cwd, homeDirToken))), nil
	case filepath.IsAbs(cwd):
		return filepath.Clean(cwd), nil
	default:
		return "", fmt.Errorf("working directory %q must be absolute or start with %s", requested, homeDirToken)
	}
}

func unixUserHomeDir(unixUser string) (string, error) {
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		return "", fmt.Errorf("cannot resolve %s without a Unix user", homeDirToken)
	}
	account, err := user.Lookup(unixUser)
	if err != nil {
		return "", fmt.Errorf("resolve home directory of Unix user %q: %w", unixUser, err)
	}
	if strings.TrimSpace(account.HomeDir) == "" {
		return "", fmt.Errorf("Unix user %q has no home directory", unixUser)
	}
	return account.HomeDir, nil
}

// LaunchHandler serves the launcher's choices.
type LaunchHandler struct {
	config LaunchConfig
}

// NewLaunchHandler creates a launch handler over an explicit configuration.
func NewLaunchHandler(config LaunchConfig) *LaunchHandler {
	return &LaunchHandler{config: withLaunchDefaults(config)}
}

// RegisterRoutes registers the launch routes on the given mux.
func (h *LaunchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/launch", h.Options)
}

// Options handles GET /api/launch. Commands stay on the server.
func (h *LaunchHandler) Options(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, h.config.options())
}
