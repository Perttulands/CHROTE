package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validThemeJSON is a schema-1 theme with one art file, used as the base every
// invalid-theme case deviates from.
const validThemeJSON = `{
  "schema": 1,
  "name": "test-theme",
  "ui": {
    "background": "#0f0f0f",
    "surface": "#1a1a1a",
    "surfaceRaised": "#252525",
    "divider": "#3a3a3a",
    "text": "#e5e5e5",
    "textSecondary": "#a3a3a3",
    "textDim": "#737373",
    "accent": "#6b9fff",
    "error": "#f87171"
  },
  "terminal": {
    "background": "#0a0a0a",
    "foreground": "#e5e5e5",
    "cursor": "#e5e5e5",
    "selectionBackground": "#6b9fff40",
    "ansi": ["#0f0f0f","#f87171","#8bd450","#e5c07b","#6b9fff","#c084fc","#45d6d6","#a3a3a3",
             "#737373","#ff8a8a","#a6e37a","#f0d48a","#8fb5ff","#d3a4ff","#7ae2e2","#ffffff"]
  },
  "identity": ["#4f6d8f"],
  "art": ["town.webp"]
}`

func themeWithout(t *testing.T, old, new string) string {
	t.Helper()

	if !strings.Contains(validThemeJSON, old) {
		t.Fatalf("base theme does not contain %q", old)
	}
	return strings.Replace(validThemeJSON, old, new, 1)
}

// newThemeHandlerForDir points a handler at dir the way startup does, through
// the environment.
func newThemeHandlerForDir(t *testing.T, dir string) *ThemeHandler {
	t.Helper()

	t.Setenv("CHROTE_THEME_DIR", dir)
	return NewThemeHandler()
}

func writeThemeFile(t *testing.T, dir, contents string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "theme.json"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write theme.json: %v", err)
	}
}

func writeArtFile(t *testing.T, dir, name string, contents []byte) {
	t.Helper()

	artDir := filepath.Join(dir, "art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatalf("create art directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artDir, name), contents, 0o644); err != nil {
		t.Fatalf("write art file %s: %v", name, err)
	}
}

func getTheme(t *testing.T, handler *ThemeHandler) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/theme", nil)
	rec := httptest.NewRecorder()
	handler.Theme(rec, req)
	return rec
}

func getArt(t *testing.T, handler *ThemeHandler, name string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/theme/art/"+name, nil)
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	handler.Art(rec, req)
	return rec
}

func TestValidateTheme(t *testing.T) {
	tests := []struct {
		name     string
		theme    string
		wantErr  bool
		wantText string
	}{
		{name: "valid", theme: validThemeJSON},
		{
			name:     "not JSON",
			theme:    "{",
			wantErr:  true,
			wantText: "not valid theme JSON",
		},
		{
			name:     "wrong schema",
			theme:    themeWithout(t, `"schema": 1`, `"schema": 2`),
			wantErr:  true,
			wantText: "schema must be 1",
		},
		{
			name:     "empty name",
			theme:    themeWithout(t, `"name": "test-theme"`, `"name": "  "`),
			wantErr:  true,
			wantText: "name must not be empty",
		},
		{
			name:     "missing ui colour",
			theme:    themeWithout(t, `"textDim": "#737373",`, ``),
			wantErr:  true,
			wantText: "ui.textDim is missing",
		},
		{
			name:     "ui colour is a tmux colour name",
			theme:    themeWithout(t, `"accent": "#6b9fff"`, `"accent": "blue"`),
			wantErr:  true,
			wantText: `ui.accent "blue" is not #rrggbb`,
		},
		{
			name:     "ui colour is three digits",
			theme:    themeWithout(t, `"accent": "#6b9fff"`, `"accent": "#fff"`),
			wantErr:  true,
			wantText: "ui.accent",
		},
		{
			name:     "missing terminal cursor",
			theme:    themeWithout(t, `"cursor": "#e5e5e5",`, ``),
			wantErr:  true,
			wantText: "terminal.cursor is missing",
		},
		{
			name:    "terminal alpha colour is allowed",
			theme:   themeWithout(t, `"selectionBackground": "#6b9fff40"`, `"selectionBackground": "#6B9FFF40"`),
			wantErr: false,
		},
		{
			name:     "fifteen ansi colours",
			theme:    themeWithout(t, `"#7ae2e2","#ffffff"`, `"#7ae2e2"`),
			wantErr:  true,
			wantText: "terminal.ansi must have exactly 16 colours, got 15",
		},
		{
			name:     "seventeen ansi colours",
			theme:    themeWithout(t, `"#7ae2e2","#ffffff"`, `"#7ae2e2","#ffffff","#000000"`),
			wantErr:  true,
			wantText: "got 17",
		},
		{
			name:     "invalid ansi colour",
			theme:    themeWithout(t, `"#8bd450"`, `"#gggggg"`),
			wantErr:  true,
			wantText: "terminal.ansi[2]",
		},
		{
			name:     "empty identity",
			theme:    themeWithout(t, `"identity": ["#4f6d8f"]`, `"identity": []`),
			wantErr:  true,
			wantText: "identity must have at least 1 colour",
		},
		{
			name:     "invalid identity colour",
			theme:    themeWithout(t, `"identity": ["#4f6d8f"]`, `"identity": ["4f6d8f"]`),
			wantErr:  true,
			wantText: "identity[0]",
		},
		{
			name:     "art name with a separator",
			theme:    themeWithout(t, `"art": ["town.webp"]`, `"art": ["nested/town.webp"]`),
			wantErr:  true,
			wantText: `art[0] "nested/town.webp" is not a valid art file name`,
		},
		{
			name:     "art name is the parent directory",
			theme:    themeWithout(t, `"art": ["town.webp"]`, `"art": [".."]`),
			wantErr:  true,
			wantText: "art[0]",
		},
		{
			name:    "no art",
			theme:   themeWithout(t, `"art": ["town.webp"]`, `"art": []`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTheme([]byte(tt.theme))
			if tt.wantErr && err == nil {
				t.Fatalf("validateTheme() = nil, want an error mentioning %q", tt.wantText)
			}
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("validateTheme() = %v, want nil", err)
				}
				return
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("validateTheme() = %v, want it to mention %q", err, tt.wantText)
			}
		})
	}
}

func TestThemeHandler_EmbeddedDefaultIsAValidSchemaOneTheme(t *testing.T) {
	if err := validateTheme(themeDefaultJSON); err != nil {
		t.Fatalf("embedded default theme is invalid: %v", err)
	}
	if !strings.Contains(string(themeDefaultJSON), `"art": []`) {
		t.Fatalf("embedded default names art it cannot serve: %s", themeDefaultJSON)
	}
}

func TestThemeHandler_ServesEmbeddedDefaultWithoutAThemeDirectory(t *testing.T) {
	handler := newThemeHandlerForDir(t, "")

	rec := getTheme(t, handler)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), themeDefaultJSON) {
		t.Fatalf("body = %s, want the embedded default", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
}

func TestThemeHandler_ServesEmbeddedDefaultWhenTheThemeFileIsAbsent(t *testing.T) {
	handler := newThemeHandlerForDir(t, t.TempDir())

	rec := getTheme(t, handler)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), themeDefaultJSON) {
		t.Fatalf("body = %s, want the embedded default", rec.Body.String())
	}
}

func TestThemeHandler_ServesTheHostThemeVerbatim(t *testing.T) {
	dir := t.TempDir()
	authored := themeWithout(t, `"name": "test-theme"`, `"name": "host-theme", "comment": "authored by hand"`)
	writeThemeFile(t, dir, authored)
	handler := newThemeHandlerForDir(t, dir)

	rec := getTheme(t, handler)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != authored {
		t.Fatalf("body = %s, want the authored file byte for byte", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
}

func TestThemeHandler_InvalidThemeFileFailsLoudlyWithTheReason(t *testing.T) {
	tests := []struct {
		name     string
		theme    string
		wantText string
	}{
		{
			name:     "wrong schema",
			theme:    themeWithout(t, `"schema": 1`, `"schema": 7`),
			wantText: "schema must be 1",
		},
		{
			name:     "truncated file",
			theme:    validThemeJSON[:40],
			wantText: "not valid theme JSON",
		},
		{
			name:     "short ansi palette",
			theme:    themeWithout(t, `"#7ae2e2","#ffffff"`, `"#7ae2e2"`),
			wantText: "terminal.ansi must have exactly 16 colours",
		},
		{
			name:     "art name that walks out of the directory",
			theme:    themeWithout(t, `"art": ["town.webp"]`, `"art": ["../secret.webp"]`),
			wantText: "art[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeThemeFile(t, dir, tt.theme)
			handler := newThemeHandlerForDir(t, dir)

			rec := getTheme(t, handler)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantText) {
				t.Fatalf("body = %s, want it to name %q", rec.Body.String(), tt.wantText)
			}
			if strings.Contains(rec.Body.String(), `"schema"`) {
				t.Fatalf("body = %s, want the reason rather than a fallback theme", rec.Body.String())
			}
		})
	}
}

func TestThemeHandler_ArtServesTheNamedFileWithItsContentType(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		contentType string
	}{
		{name: "png", file: "town.png", contentType: "image/png"},
		{name: "jpg", file: "crew.jpg", contentType: "image/jpeg"},
		{name: "jpeg", file: "convoy.jpeg", contentType: "image/jpeg"},
		{name: "webp", file: "badger.webp", contentType: "image/webp"},
		{name: "svg", file: "hawk.svg", contentType: "image/svg+xml"},
		{name: "uppercase extension", file: "wolf.WEBP", contentType: "image/webp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			body := []byte("art bytes for " + tt.file)
			writeArtFile(t, dir, tt.file, body)
			handler := newThemeHandlerForDir(t, dir)

			rec := getArt(t, handler, tt.file)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if !bytes.Equal(rec.Body.Bytes(), body) {
				t.Fatalf("body = %q, want %q", rec.Body.String(), body)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.contentType {
				t.Fatalf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if got := rec.Header().Get("Cache-Control"); got != "max-age=86400" {
				t.Fatalf("Cache-Control = %q, want max-age=86400", got)
			}
		})
	}
}

func TestThemeHandler_ArtRefusesEverythingOutsideTheArtDirectory(t *testing.T) {
	tests := []string{
		"absent.webp",
		"../theme.json",
		"..%2Ftheme.json",
		"../../secret.webp",
		"..",
		".",
		"nested/town.webp",
		"/etc/passwd",
		"town.exe",
		"town",
		"sub",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeThemeFile(t, dir, validThemeJSON)
			writeArtFile(t, dir, "town.webp", []byte("art bytes"))
			if err := os.MkdirAll(filepath.Join(dir, "art", "sub"), 0o755); err != nil {
				t.Fatalf("create art subdirectory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "secret.webp"), []byte("secret bytes"), 0o644); err != nil {
				t.Fatalf("write secret file: %v", err)
			}
			handler := newThemeHandlerForDir(t, dir)

			rec := getArt(t, handler, name)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
			for _, leak := range []string{"secret bytes", `"schema"`} {
				if strings.Contains(rec.Body.String(), leak) {
					t.Fatalf("body = %s, want no file outside the art directory", rec.Body.String())
				}
			}
		})
	}
}

func TestThemeHandler_ArtIsAbsentWithoutAThemeDirectory(t *testing.T) {
	handler := newThemeHandlerForDir(t, "")

	rec := getArt(t, handler, "town.webp")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestThemeHandler_RoutedArtTraversalIsRefused drives the registered routes so
// that an escaped separator is unescaped the way the mux unescapes it before
// the handler sees the name.
func TestThemeHandler_RoutedArtTraversalIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeThemeFile(t, dir, validThemeJSON)
	writeArtFile(t, dir, "town.webp", []byte("art bytes"))
	if err := os.WriteFile(filepath.Join(dir, "secret.webp"), []byte("secret bytes"), 0o644); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	handler := newThemeHandlerForDir(t, dir)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	tests := []struct {
		name string
		path string
		want int
	}{
		{name: "art", path: "/api/theme/art/town.webp", want: http.StatusOK},
		{name: "escaped parent", path: "/api/theme/art/..%2Fsecret.webp", want: http.StatusNotFound},
		{name: "escaped absolute", path: "/api/theme/art/%2Fetc%2Fpasswd", want: http.StatusNotFound},
		{name: "theme file", path: "/api/theme/art/theme.json", want: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "secret bytes") {
				t.Fatalf("body = %s, want no file outside the art directory", rec.Body.String())
			}
		})
	}
}
