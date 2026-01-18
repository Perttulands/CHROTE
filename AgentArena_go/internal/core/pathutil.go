// Package core provides business logic and utility functions
package core

import (
	"os"
	"path/filepath"
	"strings"
)

// AllowedRoots are the directories that can be accessed
var AllowedRoots = []string{"/code", "/workspace"}

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
	for _, root := range AllowedRoots {
		absRoot, _ := filepath.Abs(root)
		if resolved == absRoot || strings.HasPrefix(resolved, absRoot+string(os.PathSeparator)) {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return "", "FORBIDDEN", "Project path not in allowed roots: " + resolved + ". Allowed: " + strings.Join(AllowedRoots, ", ")
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
