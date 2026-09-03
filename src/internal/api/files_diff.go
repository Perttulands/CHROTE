package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chrote/server/internal/core"
)

// DiffResponse is the body of GET /api/files/diff.
type DiffResponse struct {
	Path       string `json:"path"`
	Repository string `json:"repository"`
	Diff       string `json:"diff"`
	Truncated  bool   `json:"truncated"`
}

// diffOutputLimit bounds the diff bytes a response carries.
const diffOutputLimit = 1 << 20

// DiffFile handles GET /api/files/diff?path= - the unified diff of a file
// against the HEAD of the repository that contains it. A file outside any
// repository, an unchanged file, and a repository git cannot diff all yield an
// empty diff rather than an error.
func (h *FilesHandler) DiffFile(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if requestPath == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing path")
		return
	}
	result := h.resolveSafePath(requestPath)
	if result.Error != "" || result.IsRoot {
		errMsg := result.Error
		if result.IsRoot {
			errMsg = "Cannot diff root"
		}
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", errMsg)
		return
	}

	repository := findRepositoryRoot(filepath.Dir(result.Path))
	if repository == "" {
		core.WriteJSON(w, http.StatusOK, DiffResponse{Path: result.Path})
		return
	}

	diff, truncated, err := gitDiffAgainstHead(r.Context(), repository, result.Path)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, DiffResponse{
		Path:       result.Path,
		Repository: repository,
		Diff:       diff,
		Truncated:  truncated,
	})
}

// findRepositoryRoot walks up from directory to the filesystem root and
// returns the first directory holding a .git entry. A worktree's .git is a
// file, so any entry counts. It returns "" when no repository contains the
// directory.
func findRepositoryRoot(directory string) string {
	current := filepath.Clean(directory)
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// boundedOutput keeps the first limit bytes written to it, drops the rest,
// and reports the overflow. It never refuses a write, so the producing process
// is not blocked on a full pipe; onOverflow lets the caller stop the producer
// instead.
type boundedOutput struct {
	limit      int
	buffer     bytes.Buffer
	truncated  bool
	onOverflow func()
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			b.buffer.Write(p)
			return len(p), nil
		}
		b.buffer.Write(p[:remaining])
	}
	if !b.truncated {
		b.truncated = true
		if b.onOverflow != nil {
			b.onOverflow()
		}
	}
	return len(p), nil
}

// gitDiffAgainstHead runs git diff HEAD for path inside repository and returns
// at most diffOutputLimit bytes of it. A missing git binary or a git failure
// (no commits yet, an unreadable gitfile) is reported as an empty diff. Only a
// failure to run the command at all is an error.
func gitDiffAgainstHead(ctx context.Context, repository, path string) (string, bool, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", false, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &boundedOutput{limit: diffOutputLimit, onOverflow: cancel}

	cmd := exec.CommandContext(ctx, gitPath, "--no-pager", "-C", repository, "diff", "HEAD", "--", path)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	cmd.Stdout = output
	runErr := cmd.Run()

	if output.truncated {
		return output.buffer.String(), true, nil
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) || ctx.Err() != nil {
			return "", false, nil
		}
		return "", false, runErr
	}
	return output.buffer.String(), false, nil
}
