package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/chrote/server/internal/comms"
	"github.com/chrote/server/internal/core"
	"github.com/chrote/server/internal/formations"
)

type CommsHandler struct {
	store *comms.Store
}

// NewCommsHandler constructs a schema-1 compatibility handler. Production
// server wiring must inject the shared runtime-authority Formations store.
func NewCommsHandler(workspace string) *CommsHandler {
	return NewCommsHandlerWithStore(comms.NewStore(workspace))
}

func NewCommsHandlerWithStore(store *comms.Store) *CommsHandler {
	return &CommsHandler{store: store}
}

func (h *CommsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/comms/rooms/{roomRef}/projection", h.GetProjection)
	mux.HandleFunc("GET /api/comms/rooms/{roomRef}/messages", h.GetMessages)
	mux.HandleFunc("GET /api/comms/rooms/{roomRef}/export", h.Export)
}

func (h *CommsHandler) GetProjection(w http.ResponseWriter, r *http.Request) {
	projection, err := h.store.ProjectRoom(r.PathValue("roomRef"), comms.ProjectionOptions{
		IncludePrivateFor: r.URL.Query().Get("includePrivateFor"),
	})
	if err != nil {
		writeCommsError(w, err)
		return
	}
	core.WriteSuccess(w, projection)
}

func (h *CommsHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	messages, err := h.store.Messages(r.PathValue("roomRef"), comms.MessageOptions{
		IncludePrivateFor: query.Get("includePrivateFor"),
		Since:             parseNonNegativeInt(query.Get("since")),
		Limit:             parseNonNegativeInt(query.Get("limit")),
	})
	if err != nil {
		writeCommsError(w, err)
		return
	}
	core.WriteSuccess(w, messages)
}

func (h *CommsHandler) Export(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	export, err := h.store.Export(r.PathValue("roomRef"), query.Get("format"), query.Get("includePrivateFor"))
	if err != nil {
		writeCommsError(w, err)
		return
	}
	core.WriteSuccess(w, export)
}

func parseNonNegativeInt(raw string) int {
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func writeCommsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, formations.ErrRuntimeAuthorityNonAuthorizing):
		core.WriteError(w, http.StatusServiceUnavailable, "RUNTIME_AUTHORITY_NON_AUTHORIZING", "Formations runtime authority is unavailable")
	case errors.Is(err, comms.ErrInvalidRoomRef):
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, comms.ErrRoomNotFound):
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	default:
		core.WriteError(w, http.StatusInternalServerError, "COMMS_PROJECTION_ERROR", err.Error())
	}
}
