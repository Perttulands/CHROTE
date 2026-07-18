package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/chrote/server/internal/core"
)

const maxJSONRequestBytes int64 = 1 << 20

func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONRequestBytes))
	if err := decoder.Decode(v); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			core.WriteError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "JSON body is too large")
			return false
		}
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			core.WriteError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "JSON body is too large")
			return false
		}
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return false
	}
	return true
}
