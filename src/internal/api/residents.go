package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/chrote/server/internal/core"
)

// Resident is an agent that lives in one tab of the dashboard: the Librarian
// in the Library, the tender in Agents, the Clerk in Beads. Each is a tmux
// session the host names, a folder its launcher starts in, and a Beads project
// it works from. The dashboard shows the session live in a column at the far
// right of the tab, and offers to launch it from the folder when it is absent.
type Resident struct {
	// Tab is the dashboard tab the resident lives in: library, agents or beads.
	Tab string `json:"tab"`
	// Label is what the column's header calls the resident.
	Label string `json:"label"`
	// Session is the tmux session name; empty when the host configured none.
	Session string `json:"session"`
	// Folder is where the launcher starts the session when it is absent.
	Folder string `json:"folder"`
	// Beads is the project path whose open Beads are the resident's proposals.
	Beads string `json:"beads"`
}

// LoadResidents reads the three residents from the environment. The Librarian
// shares the Library's own configuration, so its folder is the corpus root the
// Library validated; the tender and the Clerk are named by their own three
// variables each. Nothing here is validated beyond trimming: an unset session
// is a host without that resident, which the column says in so many words.
func LoadResidents(library LibraryConfig) []Resident {
	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	return []Resident{
		{
			Tab:     "library",
			Label:   "Librarian",
			Session: library.LibrarianSession,
			Folder:  library.Root,
			Beads:   library.BeadsProject,
		},
		{
			Tab:     "agents",
			Label:   "Tender",
			Session: env("CHROTE_TENDER_SESSION"),
			Folder:  env("CHROTE_TENDER_FOLDER"),
			Beads:   env("CHROTE_TENDER_BEADS"),
		},
		{
			Tab:     "beads",
			Label:   "Clerk",
			Session: env("CHROTE_CLERK_SESSION"),
			Folder:  env("CHROTE_CLERK_FOLDER"),
			Beads:   env("CHROTE_CLERK_BEADS"),
		},
	}
}

// ResidentsHandler serves GET /api/residents: the one place the dashboard
// learns which session and folder each tab's resident has.
type ResidentsHandler struct {
	residents []Resident
}

// NewResidentsHandler creates the handler over residents read at startup.
func NewResidentsHandler(residents []Resident) *ResidentsHandler {
	return &ResidentsHandler{residents: residents}
}

// RegisterRoutes registers the residents route.
func (h *ResidentsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/residents", h.Residents)
}

// Residents handles GET /api/residents with a flat list, one entry per tab in
// tab order, whether or not the host configured anything for it.
func (h *ResidentsHandler) Residents(w http.ResponseWriter, _ *http.Request) {
	core.WriteJSON(w, http.StatusOK, h.residents)
}
