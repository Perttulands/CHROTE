package api

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chrote/server/internal/core"
)

// themeDefaultJSON is the theme CHROTE serves when the host authored none: the
// schema-1 document with no art, so an install without a theme directory still
// renders a complete palette.
//
//go:embed theme_default.json
var themeDefaultJSON []byte

const (
	themeFileName = "theme.json"
	themeArtDir   = "art"
	// themeShelfHuesWanted is how many shelf hues a theme is expected to name:
	// as many as a corpus a reader keeps has shelves, with room to grow. Fewer
	// is allowed and said in the log.
	themeShelfHuesWanted = 10
)

var (
	// themeColorPattern is the one colour shape the schema allows: #rrggbb or
	// #rrggbbaa.
	themeColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)
	// themeArtNamePattern is the one art file-name shape the schema allows. It
	// admits no separator, so no name can name a file outside the art
	// directory.
	themeArtNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// themeUIKeys are the ui colours the dashboard binds to CSS custom
	// properties. Every one must be present; a missing key would leave a
	// variable undefined in the browser.
	themeUIKeys = []string{
		"background", "surface", "surfaceRaised", "divider",
		"text", "textSecondary", "textDim", "accent", "error",
	}
)

// themeDocument is the schema-1 theme, decoded only far enough to validate it.
// The bytes the host wrote are what the browser receives; this type never
// re-encodes them.
type themeDocument struct {
	Schema   int               `json:"schema"`
	Name     string            `json:"name"`
	UI       map[string]string `json:"ui"`
	Terminal themeTerminal     `json:"terminal"`
	// Shelves are the hues the Library's map draws its shelves in, taken in
	// shelf order. A theme authored before the map had colour carries none,
	// and the dashboard falls back to the built-in palette rather than
	// drawing a colourless map, so the field is optional.
	Shelves  []string `json:"shelves"`
	Identity []string `json:"identity"`
	Art      []string `json:"art"`
}

type themeTerminal struct {
	Background          string   `json:"background"`
	Foreground          string   `json:"foreground"`
	Cursor              string   `json:"cursor"`
	SelectionBackground string   `json:"selectionBackground"`
	ANSI                []string `json:"ansi"`
}

// ThemeHandler serves the host's theme document and the art it names.
type ThemeHandler struct {
	// dir is $CHROTE_THEME_DIR as read at startup. Empty means the host
	// configured no theme directory, so the embedded default is the whole
	// answer and no art exists.
	dir string
}

// NewThemeHandler creates a theme handler bound to the theme directory named by
// CHROTE_THEME_DIR at startup.
func NewThemeHandler() *ThemeHandler {
	return &ThemeHandler{dir: strings.TrimSpace(os.Getenv("CHROTE_THEME_DIR"))}
}

// RegisterRoutes registers the theme routes on the given mux
func (h *ThemeHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/theme", h.Theme)
	mux.HandleFunc("GET /api/theme/art/{name}", h.Art)
}

// Theme handles GET /api/theme. It serves the host's theme.json verbatim once
// it validates, the embedded default when the host authored none, and a loud
// 500 when a theme exists but cannot be trusted: a silent fallback would hide
// the operator's broken edit behind a palette that looks deliberate.
func (h *ThemeHandler) Theme(w http.ResponseWriter, r *http.Request) {
	body := themeDefaultJSON

	if h.dir != "" {
		path := filepath.Join(h.dir, themeFileName)
		raw, err := os.ReadFile(path)
		switch {
		case err == nil:
			if invalid := validateTheme(raw); invalid != nil {
				log.Printf("Theme %s is invalid: %v", path, invalid)
				core.WriteError(w, http.StatusInternalServerError, "INVALID_THEME",
					fmt.Sprintf("%s is invalid: %v", themeFileName, invalid))
				return
			}
			body = raw
		case errors.Is(err, fs.ErrNotExist):
			// No theme authored for this host; the embedded default answers.
		default:
			log.Printf("Theme %s could not be read: %v", path, err)
			core.WriteError(w, http.StatusInternalServerError, "THEME_UNREADABLE",
				fmt.Sprintf("%s could not be read", themeFileName))
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// Art handles GET /api/theme/art/{name}. It serves one file from the theme's
// art directory. The name is validated before any path is built, so no request
// can address a file outside that directory.
func (h *ThemeHandler) Art(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isThemeArtName(name) {
		themeArtNotFound(w)
		return
	}
	contentType, ok := themeArtContentType(name)
	if !ok {
		themeArtNotFound(w)
		return
	}
	if h.dir == "" {
		themeArtNotFound(w)
		return
	}

	artDir := filepath.Join(h.dir, themeArtDir)
	path := filepath.Join(artDir, name)
	if filepath.Dir(path) != artDir {
		themeArtNotFound(w)
		return
	}

	file, err := os.Open(path)
	if err != nil {
		themeArtNotFound(w)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		themeArtNotFound(w)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func themeArtNotFound(w http.ResponseWriter) {
	core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Theme art not found")
}

// isThemeArtName reports whether name is a schema-legal art file name. The
// pattern admits no path separator, and the two relative directory names are
// rejected outright so that no name can walk out of the art directory.
func isThemeArtName(name string) bool {
	if name == "." || name == ".." {
		return false
	}
	return themeArtNamePattern.MatchString(name)
}

// themeArtContentType maps the art extensions the schema serves to their
// content types. An unknown extension is not art.
func themeArtContentType(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".webp":
		return "image/webp", true
	case ".svg":
		return "image/svg+xml", true
	default:
		return "", false
	}
}

// validateTheme reports why raw is not a schema-1 theme, or nil when it is. The
// message is the operator's whole diagnosis, so it names the offending field.
func validateTheme(raw []byte) error {
	var theme themeDocument
	if err := json.Unmarshal(raw, &theme); err != nil {
		return fmt.Errorf("not valid theme JSON: %w", err)
	}

	if theme.Schema != 1 {
		return fmt.Errorf("schema must be 1, got %d", theme.Schema)
	}
	if strings.TrimSpace(theme.Name) == "" {
		return errors.New("name must not be empty")
	}

	for _, key := range themeUIKeys {
		if err := validateThemeColor(fmt.Sprintf("ui.%s", key), theme.UI[key]); err != nil {
			return err
		}
	}

	terminalColors := []struct {
		field string
		value string
	}{
		{"terminal.background", theme.Terminal.Background},
		{"terminal.foreground", theme.Terminal.Foreground},
		{"terminal.cursor", theme.Terminal.Cursor},
		{"terminal.selectionBackground", theme.Terminal.SelectionBackground},
	}
	for _, color := range terminalColors {
		if err := validateThemeColor(color.field, color.value); err != nil {
			return err
		}
	}

	if len(theme.Terminal.ANSI) != 16 {
		return fmt.Errorf("terminal.ansi must have exactly 16 colours, got %d", len(theme.Terminal.ANSI))
	}
	for index, color := range theme.Terminal.ANSI {
		if err := validateThemeColor(fmt.Sprintf("terminal.ansi[%d]", index), color); err != nil {
			return err
		}
	}

	for index, color := range theme.Shelves {
		if err := validateThemeColor(fmt.Sprintf("shelves[%d]", index), color); err != nil {
			return err
		}
	}
	// A palette shorter than the corpus has shelves makes two shelves the same
	// colour, which is a picture that says something untrue. It is the
	// operator's theme to author, so this is said in the log rather than
	// refused: a map with repeated hues is still a map.
	if len(theme.Shelves) > 0 && len(theme.Shelves) < themeShelfHuesWanted {
		log.Printf("Theme names %d shelf hues; a corpus with more shelves than that will repeat one", len(theme.Shelves))
	}

	if len(theme.Identity) == 0 {
		return errors.New("identity must have at least 1 colour")
	}
	for index, color := range theme.Identity {
		if err := validateThemeColor(fmt.Sprintf("identity[%d]", index), color); err != nil {
			return err
		}
	}

	for index, name := range theme.Art {
		if !isThemeArtName(name) {
			return fmt.Errorf("art[%d] %q is not a valid art file name", index, name)
		}
	}

	return nil
}

func validateThemeColor(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is missing", field)
	}
	if !themeColorPattern.MatchString(value) {
		return fmt.Errorf("%s %q is not #rrggbb or #rrggbbaa", field, value)
	}
	return nil
}
