// Package core provides business logic and utility functions
package core

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Default allowed roots (your personal setup)
var defaultAllowedRoots = []string{"/code", "/vault"}

var (
	allowedRootsOnce sync.Once
	allowedRoots     []string
)

// GetAllowedRoots returns the configured allowed roots
// Reads from CHROTE_ROOTS env var, defaults to /code,/vault
func GetAllowedRoots() []string {
	allowedRootsOnce.Do(func() {
		if roots := os.Getenv("CHROTE_ROOTS"); roots != "" {
			allowedRoots = strings.Split(roots, ",")
			for i := range allowedRoots {
				allowedRoots[i] = strings.TrimSpace(allowedRoots[i])
			}
		} else {
			allowedRoots = defaultAllowedRoots
		}
	})
	return allowedRoots
}

// AllowedRoots is deprecated, use GetAllowedRoots() instead
// Kept for backwards compatibility
var AllowedRoots = defaultAllowedRoots

// ResetConfigForTesting resets the cached config (for testing only)
func ResetConfigForTesting() {
	allowedRootsOnce = sync.Once{}
	allowedRoots = nil
}

// ValidateProjectPath ensures a path is within allowed roots
func ValidateProjectPath(inputPath string) (string, string, string) {
	if inputPath == "" {
		return "", "BAD_REQUEST", "Missing required parameter: path"
	}

	resolved, err := filepath.Abs(inputPath)
	if err != nil {
		return "", "BAD_REQUEST", "Invalid path: " + err.Error()
	}

	isAllowed := false
	for _, root := range GetAllowedRoots() {
		absRoot, _ := filepath.Abs(root)
		if resolved == absRoot || strings.HasPrefix(resolved, absRoot+string(os.PathSeparator)) {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return "", "FORBIDDEN", "Project path not in allowed roots: " + resolved + ". Allowed: " + strings.Join(GetAllowedRoots(), ", ")
	}

	if _, err := os.Stat(resolved); os.IsNotExist(err) {
		return "", "NOT_FOUND", "Project path does not exist: " + resolved
	}

	return resolved, "", ""
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetWorkDir returns the default working directory for new sessions
// Reads from CHROTE_WORKDIR env var, defaults to first allowed root
func GetWorkDir() string {
	if workdir := os.Getenv("CHROTE_WORKDIR"); workdir != "" {
		return workdir
	}
	roots := GetAllowedRoots()
	if len(roots) > 0 {
		return roots[0]
	}
	return "/code"
}

// GetLaunchScript returns the terminal launch script path
// Reads from CHROTE_LAUNCH_SCRIPT env var, defaults to /usr/local/bin/terminal-launch.sh
func GetLaunchScript() string {
	if script := os.Getenv("CHROTE_LAUNCH_SCRIPT"); script != "" {
		return script
	}
	return "/usr/local/bin/terminal-launch.sh"
}

// GetBvLaunchScript returns the beads viewer launch script path
// Reads from CHROTE_BV_LAUNCH_SCRIPT env var, defaults to /usr/local/bin/bv-launch.sh
func GetBvLaunchScript() string {
	if script := os.Getenv("CHROTE_BV_LAUNCH_SCRIPT"); script != "" {
		return script
	}
	return "/usr/local/bin/bv-launch.sh"
}
