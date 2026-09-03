package api

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chrote/server/internal/core"
)

// FindMatch is one file returned by the find route.
type FindMatch struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// FindResponse is the body of GET /api/files/find.
type FindResponse struct {
	Matches   []FindMatch `json:"matches"`
	Truncated bool        `json:"truncated"`
}

// findResultLimit caps the matches a response carries.
const findResultLimit = 50

// findCandidateLimit caps the candidates collected before sorting, so the
// ordering rule is applied to a bounded superset instead of walk order.
const findCandidateLimit = 500

// findIgnoredDirectories lists directory names the walk never enters, on top
// of every directory whose name starts with a dot. Worktree copies and store
// internals therefore never appear in results.
var findIgnoredDirectories = map[string]bool{
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"vendor":       true,
}

type findCandidate struct {
	path      string
	name      string
	nameMatch bool
}

func findIgnoresDirectory(name string) bool {
	return strings.HasPrefix(name, ".") || findIgnoredDirectories[name]
}

// mountInfoPath is the kernel's table of the mounts this process can see. Its
// fifth field is the mount point.
const mountInfoPath = "/proc/self/mountinfo"

// mountFieldUnescaper decodes the octal escapes the kernel writes for the
// characters that would otherwise break the space-separated fields.
var mountFieldUnescaper = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

// readMountPoints returns the set of paths that are mount points. A system
// that does not publish the table yields no set at all, and the walk then
// descends exactly as it did before.
func readMountPoints(path string) map[string]bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseMountPoints(string(content))
}

func parseMountPoints(content string) map[string]bool {
	points := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		points[filepath.Clean(mountFieldUnescaper.Replace(fields[4]))] = true
	}
	return points
}

// findLeavesMount reports whether descending into path would take the walk out
// of the mount its root sits in. A mount point is a second filesystem, and a
// bind mount presents a tree that is already reachable elsewhere, so entering
// one returns the same file twice and can walk into kernel filesystems that
// hold no operator files. The root itself is always entered: a root that is
// its own mount point is exactly the tree the operator configured.
func findLeavesMount(path string, root string, mountPoints map[string]bool) bool {
	return path != root && mountPoints[path]
}

// FindFiles handles GET /api/files/find?q= - files under the configured roots
// whose name or root-relative path contains the query.
func (h *FilesHandler) FindFiles(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		core.WriteJSON(w, http.StatusOK, FindResponse{Matches: []FindMatch{}})
		return
	}

	candidates, truncated := h.collectFindCandidates(r.Context(), query)
	sortFindCandidates(candidates)
	if len(candidates) > findResultLimit {
		candidates = candidates[:findResultLimit]
		truncated = true
	}

	matches := make([]FindMatch, 0, len(candidates))
	for _, candidate := range candidates {
		matches = append(matches, FindMatch{Path: candidate.path, Name: candidate.name})
	}
	core.WriteJSON(w, http.StatusOK, FindResponse{Matches: matches, Truncated: truncated})
}

// collectFindCandidates walks every configured root and returns up to
// findCandidateLimit matching regular files. The second result reports that
// the walk stopped before it finished: the candidate cap was reached or the
// request context ended.
//
// The path match is taken against the path relative to its root. The root's
// own name is shared by everything beneath it, so matching it would turn a
// query like the root's directory name into a match for every file.
//
// The mount table is read once here rather than per root, so a request sees
// one consistent view of where the walk has to stop.
func (h *FilesHandler) collectFindCandidates(ctx context.Context, query string) ([]findCandidate, bool) {
	needle := strings.ToLower(query)
	candidates := make([]findCandidate, 0)
	stopped := false
	mountPoints := readMountPoints(mountInfoPath)

	for _, root := range h.allowedRoots {
		if stopped {
			break
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absoluteRoot = filepath.Clean(absoluteRoot)
		if canonicalRoot, err := canonicalPathAllowMissing(absoluteRoot); err == nil {
			absoluteRoot = canonicalRoot
		}

		_ = filepath.WalkDir(absoluteRoot, func(path string, entry fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				stopped = true
				return fs.SkipAll
			}
			if err != nil {
				// An unreadable or vanished entry is skipped; the walk goes on.
				return nil
			}
			if entry.IsDir() {
				if path != absoluteRoot && findIgnoresDirectory(entry.Name()) {
					return fs.SkipDir
				}
				if findLeavesMount(path, absoluteRoot, mountPoints) {
					return fs.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}

			name := entry.Name()
			nameMatch := strings.Contains(strings.ToLower(name), needle)
			if !nameMatch {
				relative, relErr := filepath.Rel(absoluteRoot, path)
				if relErr != nil || !strings.Contains(strings.ToLower(filepath.ToSlash(relative)), needle) {
					return nil
				}
			}
			candidates = append(candidates, findCandidate{path: path, name: name, nameMatch: nameMatch})
			if len(candidates) >= findCandidateLimit {
				stopped = true
				return fs.SkipAll
			}
			return nil
		})
	}
	return candidates, stopped
}

// sortFindCandidates orders name matches before path-only matches, then
// shorter full paths first, then lexicographically by path.
func sortFindCandidates(candidates []findCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.nameMatch != right.nameMatch {
			return left.nameMatch
		}
		if len(left.path) != len(right.path) {
			return len(left.path) < len(right.path)
		}
		return left.path < right.path
	})
}
