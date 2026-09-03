package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFindHandler(roots ...string) *FilesHandler {
	return &FilesHandler{
		allowedRoots:   roots,
		maxUploadBytes: defaultMaxUploadBytes,
	}
}

func findRequest(t *testing.T, handler *FilesHandler, query string) (*httptest.ResponseRecorder, FindResponse) {
	t.Helper()
	return findRequestWithContext(t, handler, context.Background(), query)
}

func findRequestWithContext(t *testing.T, handler *FilesHandler, ctx context.Context, query string) (*httptest.ResponseRecorder, FindResponse) {
	t.Helper()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	target := "/api/files/find?" + url.Values{"q": {query}}.Encode()
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var response FindResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("find response is not JSON: %v: %s", err, rec.Body.String())
		}
	}
	return rec, response
}

func findPaths(response FindResponse) []string {
	paths := make([]string, 0, len(response.Matches))
	for _, match := range response.Matches {
		paths = append(paths, match.Path)
	}
	return paths
}

func TestFilesHandlerFindEmptyQueryReturnsNoMatches(t *testing.T) {
	root := t.TempDir()
	writeFileFixture(t, filepath.Join(root, "main.ts"), "")

	for _, query := range []string{"", "   "} {
		rec, response := findRequest(t, newFindHandler(root), query)
		if rec.Code != http.StatusOK {
			t.Fatalf("query %q status = %d, want 200: %s", query, rec.Code, rec.Body.String())
		}
		if strings.TrimSpace(rec.Body.String()) != `{"matches":[],"truncated":false}` {
			t.Fatalf("query %q body = %s, want an empty match list", query, rec.Body.String())
		}
		if len(response.Matches) != 0 || response.Truncated {
			t.Fatalf("query %q decoded = %+v, want no matches", query, response)
		}
	}
}

func TestFilesHandlerFindOrdersNameMatchesBeforePathMatches(t *testing.T) {
	root := t.TempDir()
	nameShort := filepath.Join(root, "main.go")
	nameB := filepath.Join(root, "src", "b", "main.ts")
	nameA := filepath.Join(root, "src", "a", "main.ts")
	pathOnly := filepath.Join(root, "src", "main", "util.go")
	for _, path := range []string{pathOnly, nameB, nameA, nameShort} {
		writeFileFixture(t, path, "")
	}
	writeFileFixture(t, filepath.Join(root, "src", "other.go"), "")

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "name matches first, then shorter path, then lexicographic",
			query: "main",
			want:  []string{nameShort, nameA, nameB, pathOnly},
		},
		{
			name:  "matching is case-insensitive",
			query: "MAIN.TS",
			want:  []string{nameA, nameB},
		},
		{
			name:  "path segments match the root-relative path",
			query: "src/main",
			want:  []string{pathOnly},
		},
		{
			name:  "the root's own name is not part of the path match",
			query: filepath.Base(root),
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, response := findRequest(t, newFindHandler(root), tt.query)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			if response.Truncated {
				t.Fatalf("truncated = true for %d matches", len(response.Matches))
			}
			got := findPaths(response)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("matches for %q =\n%s\nwant\n%s", tt.query, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			for _, match := range response.Matches {
				if match.Name != filepath.Base(match.Path) {
					t.Fatalf("match name %q does not match its path %q", match.Name, match.Path)
				}
			}
		})
	}
}

func TestFilesHandlerFindSkipsIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	visible := filepath.Join(root, "src", "main.ts")
	writeFileFixture(t, visible, "")
	ignored := []string{".hidden", ".git", "node_modules", "dist", "build", "target", "vendor"}
	for _, directory := range ignored {
		writeFileFixture(t, filepath.Join(root, directory, "main.ts"), "")
		writeFileFixture(t, filepath.Join(root, "src", directory, "deep", "main.ts"), "")
	}

	rec, response := findRequest(t, newFindHandler(root), "main")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := findPaths(response); len(got) != 1 || got[0] != visible {
		t.Fatalf("matches = %v, want only %s", got, visible)
	}
}

func TestFilesHandlerFindWalksRootWhoseNameIsIgnored(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vendor")
	inside := filepath.Join(root, "lib", "main.ts")
	writeFileFixture(t, inside, "")

	rec, response := findRequest(t, newFindHandler(root), "main")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := findPaths(response); len(got) != 1 || got[0] != inside {
		t.Fatalf("matches = %v, want %s from a root named vendor", got, inside)
	}
}

func TestFilesHandlerFindReturnsFilesOnly(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "main")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "main.ts")
	writeFileFixture(t, file, "")
	if err := os.Symlink(file, filepath.Join(root, "main-link.ts")); err != nil {
		t.Fatal(err)
	}

	rec, response := findRequest(t, newFindHandler(root), "main")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := findPaths(response); len(got) != 1 || got[0] != file {
		t.Fatalf("matches = %v, want only the regular file %s", got, file)
	}
}

func TestFilesHandlerFindCapsResults(t *testing.T) {
	tests := []struct {
		name          string
		files         int
		wantMatches   int
		wantTruncated bool
	}{
		{name: "exactly the cap", files: findResultLimit, wantMatches: findResultLimit, wantTruncated: false},
		{name: "one over the cap", files: findResultLimit + 1, wantMatches: findResultLimit, wantTruncated: true},
		{name: "well over the cap", files: 120, wantMatches: findResultLimit, wantTruncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for i := 0; i < tt.files; i++ {
				writeFileFixture(t, filepath.Join(root, fmt.Sprintf("hit-%03d.txt", i)), "")
			}

			rec, response := findRequest(t, newFindHandler(root), "hit")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			if len(response.Matches) != tt.wantMatches || response.Truncated != tt.wantTruncated {
				t.Fatalf("matches = %d truncated = %v, want %d and %v", len(response.Matches), response.Truncated, tt.wantMatches, tt.wantTruncated)
			}
		})
	}
}

func TestFilesHandlerFindSearchesEveryConfiguredRoot(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	inFirst := filepath.Join(first, "main.ts")
	inSecond := filepath.Join(second, "deep", "main.ts")
	writeFileFixture(t, inFirst, "")
	writeFileFixture(t, inSecond, "")

	rec, response := findRequest(t, newFindHandler(first, second), "main")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	got := findPaths(response)
	if len(got) != 2 || got[0] != inFirst || got[1] != inSecond {
		t.Fatalf("matches = %v, want %s then %s", got, inFirst, inSecond)
	}
}

func TestFilesHandlerFindSkipsUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	sealed := filepath.Join(root, "sealed")
	writeFileFixture(t, filepath.Join(sealed, "main.ts"), "")
	visible := filepath.Join(root, "zzz", "main.ts")
	writeFileFixture(t, visible, "")
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0755) })

	rec, response := findRequest(t, newFindHandler(root), "main")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite an unreadable directory: %s", rec.Code, rec.Body.String())
	}
	if got := findPaths(response); len(got) != 1 || got[0] != visible {
		t.Fatalf("matches = %v, want only %s", got, visible)
	}
}

func TestFilesHandlerFindStopsWhenRequestContextEnds(t *testing.T) {
	root := t.TempDir()
	writeFileFixture(t, filepath.Join(root, "main.ts"), "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec, response := findRequestWithContext(t, newFindHandler(root), ctx, "main")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(response.Matches) != 0 || !response.Truncated {
		t.Fatalf("response = %+v, want no matches and truncated after cancellation", response)
	}
}

func TestFindLeavesMountStopsAtAMountPointBelowTheRoot(t *testing.T) {
	mountPoints := map[string]bool{"/a/b": true, "/c": true}

	tests := []struct {
		name string
		path string
		root string
		want bool
	}{
		{name: "a mount point below the root is left", path: "/a/b", root: "/a", want: true},
		{name: "an ordinary directory below the root is walked", path: "/a/c", root: "/a", want: false},
		{name: "a root that is itself a mount point is walked", path: "/c", root: "/c", want: false},
		{name: "a directory below a mounted root is walked", path: "/c/d", root: "/c", want: false},
		{name: "a mount point outside the walking root is left", path: "/c", root: "/a", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findLeavesMount(tt.path, tt.root, mountPoints); got != tt.want {
				t.Fatalf("findLeavesMount(%q, %q) = %v, want %v", tt.path, tt.root, got, tt.want)
			}
		})
	}
}

func TestFindLeavesMountWalksEverythingWithoutAMountTable(t *testing.T) {
	missing := readMountPoints(filepath.Join(t.TempDir(), "absent"))
	if missing != nil {
		t.Fatalf("mount points from a missing table = %v, want none", missing)
	}
	for _, path := range []string{"/a/b", "/c", "/proc"} {
		if findLeavesMount(path, "/a", missing) {
			t.Fatalf("%q was skipped without a mount table, want the walk to descend", path)
		}
	}
}

func TestReadMountPointsParsesTheMountTable(t *testing.T) {
	table := strings.Join([]string{
		"21 28 0:20 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw",
		"25 28 0:23 / /run rw,nosuid,nodev - tmpfs tmpfs rw,mode=755",
		`31 28 8:1 /srv /mnt/copy\040of ro,relatime - ext4 /dev/sda1 ro`,
		"32 28 8:1 / /opt/trailing/ rw,relatime - ext4 /dev/sda1 rw",
		"truncated line",
		"",
	}, "\n")
	path := filepath.Join(t.TempDir(), "mountinfo")
	writeFileFixture(t, path, table)

	got := readMountPoints(path)
	want := map[string]bool{"/proc": true, "/run": true, "/mnt/copy of": true, "/opt/trailing": true}
	if len(got) != len(want) {
		t.Fatalf("mount points = %v, want %v", got, want)
	}
	for point := range want {
		if !got[point] {
			t.Fatalf("mount points = %v, want %q among them", got, point)
		}
	}
}
