package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

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

// maxLaunchFlagsBytes bounds the flags line a request may carry. It is a
// generous single command line; anything longer is a mistake, not a launch.
const maxLaunchFlagsBytes = 4096

// flagsNotOneLineMessage is what the operator is told when the flags line is
// not one line. A control character and an over-long line are the same
// mistake: what arrived is not something that can be typed at a prompt.
const flagsNotOneLineMessage = "flags must be one line"

var launchHarnessIDPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// LaunchFlag is one option a harness's binary accepts, as that binary's own
// --help printed it. The catalogue is a browsable reference for composing a
// flags line; it constrains nothing, because the line the operator sends is
// what runs.
type LaunchFlag struct {
	Name        string   `json:"name"`
	Short       string   `json:"short,omitempty"`
	Value       string   `json:"value,omitempty"`
	Description string   `json:"description"`
	Values      []string `json:"values,omitempty"`
}

// LaunchHarness is one thing the operator can start in a new session. The
// command is the binary alone; the flags a launch types are the operator's
// configured defaults or the line the request carried.
type LaunchHarness struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	Command      string       `json:"command"`
	DefaultFlags string       `json:"defaultFlags,omitempty"`
	Flags        []LaunchFlag `json:"flags,omitempty"`
}

// LaunchConfig is what the launcher may offer: which harnesses can be started
// and which folders are worth listing. It is read once at startup from
// CHROTE_LAUNCH_CONFIG.
type LaunchConfig struct {
	Harnesses []LaunchHarness `json:"harnesses"`
	Folders   []string        `json:"folders"`
}

// LaunchHarnessOption is the browser's view of a harness: what to show, what
// to send back, and what the launcher needs to offer a flags line. The binary
// is the first token of the configured command; nothing else about that
// command leaves the server.
type LaunchHarnessOption struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	Binary       string       `json:"binary"`
	DefaultFlags string       `json:"defaultFlags"`
	Flags        []LaunchFlag `json:"flags"`
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
		if id == shellHarnessID {
			if strings.TrimSpace(harness.Command) != "" {
				return fmt.Errorf("harness %q must have an empty command; it is the bare login shell", id)
			}
			if strings.TrimSpace(harness.DefaultFlags) != "" {
				return fmt.Errorf("harness %q must have no default flags; it is the bare login shell", id)
			}
			if len(harness.Flags) > 0 {
				return fmt.Errorf("harness %q must have no flag catalogue; it is the bare login shell", id)
			}
		}
		for flagIndex, flag := range harness.Flags {
			name := strings.TrimSpace(flag.Name)
			if !strings.HasPrefix(name, "-") {
				return fmt.Errorf("harness %q flag %d has name %q, which must start with a dash", id, flagIndex, flag.Name)
			}
			if strings.TrimSpace(flag.Description) == "" {
				return fmt.Errorf("harness %q flag %q has no description", id, name)
			}
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
		harness.DefaultFlags = strings.TrimSpace(harness.DefaultFlags)
		if harness.DefaultFlags == "" {
			harness.Command, harness.DefaultFlags = splitCommandLine(harness.Command)
		}
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

// splitCommandLine separates a configured command into the binary that runs
// and the flags that follow it. Configurations written before flags existed
// hold both in one string, so the first token is the binary and the rest is
// that harness's default flags.
func splitCommandLine(command string) (string, string) {
	command = strings.TrimSpace(command)
	index := strings.IndexFunc(command, unicode.IsSpace)
	if index < 0 {
		return command, ""
	}
	return command[:index], strings.TrimSpace(command[index:])
}

// options is what the browser is allowed to know: ids, labels, folders, and
// the flags line each harness starts from.
func (c LaunchConfig) options() LaunchOptionsResponse {
	harnesses := make([]LaunchHarnessOption, 0, len(c.Harnesses))
	for _, harness := range c.Harnesses {
		flags := harness.Flags
		if flags == nil {
			flags = []LaunchFlag{}
		}
		harnesses = append(harnesses, LaunchHarnessOption{
			ID:           harness.ID,
			Label:        harness.Label,
			Binary:       harness.Command,
			DefaultFlags: harness.DefaultFlags,
			Flags:        flags,
		})
	}
	folders := append([]string(nil), c.Folders...)
	if folders == nil {
		folders = []string{}
	}
	return LaunchOptionsResponse{Harnesses: harnesses, Folders: folders}
}

// resolvedLaunch is what a create-session request settled on before anything
// was created: which harness answered, the line to type, and the flags line
// that produced it. An empty command types nothing.
type resolvedLaunch struct {
	harnessID string
	command   string
	flags     string
}

// resolveHarness turns a requested harness id and flags line into what the new
// session starts. An absent id and the shell run nothing. Absent flags mean
// the harness's configured defaults; an empty flags line means the operator
// asked for none.
func (c LaunchConfig) resolveHarness(requested string, requestedFlags *string) (resolvedLaunch, error) {
	id := strings.TrimSpace(requested)
	if id == "" {
		id = shellHarnessID
	}
	for _, harness := range c.Harnesses {
		if harness.ID != id {
			continue
		}
		flags := harness.DefaultFlags
		if requestedFlags != nil {
			if err := validateLaunchFlags(*requestedFlags); err != nil {
				return resolvedLaunch{}, err
			}
			flags = strings.TrimSpace(*requestedFlags)
		}
		if harness.Command == "" {
			if flags != "" {
				return resolvedLaunch{}, fmt.Errorf("harness %q starts no command, so it takes no flags", harness.ID)
			}
			return resolvedLaunch{harnessID: harness.ID}, nil
		}
		command := harness.Command
		if flags != "" {
			command += " " + flags
		}
		return resolvedLaunch{harnessID: harness.ID, command: command, flags: flags}, nil
	}
	return resolvedLaunch{}, fmt.Errorf("unknown harness %q", requested)
}

// validateLaunchFlags refuses anything that is not one command line. A control
// character would type an extra key into the session rather than a flag, and a
// line past the bound is not a launch anybody meant.
func validateLaunchFlags(flags string) error {
	if len(flags) > maxLaunchFlagsBytes {
		return errors.New(flagsNotOneLineMessage)
	}
	for index := 0; index < len(flags); index++ {
		if flags[index] < 0x20 || flags[index] == 0x7f {
			return errors.New(flagsNotOneLineMessage)
		}
	}
	return nil
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
