package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The residents route is the one place the dashboard learns which session each
// tab's agent is: the shape is the contract, and so is that every tab answers
// whether or not the host configured anything for it.
func TestResidentsRouteListsEveryTabFromTheEnvironment(t *testing.T) {
	t.Setenv("CHROTE_TENDER_SESSION", " tender ")
	t.Setenv("CHROTE_TENDER_FOLDER", "/tender")
	t.Setenv("CHROTE_TENDER_BEADS", "/tender/store")
	t.Setenv("CHROTE_CLERK_SESSION", "clerk")
	t.Setenv("CHROTE_CLERK_FOLDER", "/clerk")
	t.Setenv("CHROTE_CLERK_BEADS", "")

	library := LibraryConfig{Root: "/corpus", LibrarianSession: "librarian", BeadsProject: "/corpus"}
	mux := http.NewServeMux()
	NewResidentsHandler(LoadResidents(library)).RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/residents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	residents := decodeLibrary[[]Resident](t, rec)
	want := []Resident{
		{Tab: "library", Label: "Librarian", Session: "librarian", Folder: "/corpus", Beads: "/corpus"},
		{Tab: "agents", Label: "Tender", Session: "tender", Folder: "/tender", Beads: "/tender/store"},
		{Tab: "beads", Label: "Clerk", Session: "clerk", Folder: "/clerk", Beads: ""},
	}
	if len(residents) != len(want) {
		t.Fatalf("residents = %+v, want %+v", residents, want)
	}
	for i := range want {
		if residents[i] != want[i] {
			t.Fatalf("resident %d = %+v, want %+v", i, residents[i], want[i])
		}
	}
}
