package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// ServicesConfig is host-owned connection information, not the service catalog.
type ServicesConfig struct {
	Socket   string `json:"socket"`
	Resident struct {
		Session string `json:"session"`
		Folder  string `json:"folder"`
		Beads   string `json:"beads"`
	} `json:"resident"`
}

// LoadServicesConfig reads an optional host configuration. Absence leaves the
// tab available with an explanation, without imposing a host-specific path.
func LoadServicesConfig() (ServicesConfig, error) {
	path := os.Getenv("CHROTE_SERVICES_CONFIG")
	if path == "" {
		if launch := os.Getenv("CHROTE_LAUNCH_CONFIG"); launch != "" {
			path = filepath.Join(filepath.Dir(launch), "services.json")
		} else {
			dir, err := os.UserConfigDir()
			if err != nil {
				return ServicesConfig{}, err
			}
			path = filepath.Join(dir, "chrote", "services.json")
		}
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ServicesConfig{}, nil
	}
	if err != nil {
		return ServicesConfig{}, err
	}
	var config ServicesConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return ServicesConfig{}, fmt.Errorf("invalid services configuration: %w", err)
	}
	if config.Socket != "" && !filepath.IsAbs(config.Socket) {
		return ServicesConfig{}, errors.New("services socket must be an absolute path")
	}
	return config, nil
}

var serviceID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type ServicesHandler struct {
	socket string
	client *http.Client
}

func NewServicesHandler(socket string) *ServicesHandler {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &ServicesHandler{socket: socket, client: &http.Client{Transport: transport, Timeout: 90 * time.Second}}
}
func (h *ServicesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/services", h.list)
	mux.HandleFunc("POST /api/services/{id}/{action}", h.action)
}
func (h *ServicesHandler) list(w http.ResponseWriter, r *http.Request) { h.forward(w, r, "/services") }
func (h *ServicesHandler) action(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Requested-With") != "chrote" {
		http.Error(w, "service actions require a CHROTE request", http.StatusForbidden)
		return
	}
	id, action := r.PathValue("id"), r.PathValue("action")
	if !serviceID.MatchString(id) || (action != "start" && action != "stop") {
		http.Error(w, "invalid service action", http.StatusBadRequest)
		return
	}
	// A group can contain several services, each of which may take time to
	// stop. The server's ordinary response deadline is shorter than that work.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(95 * time.Second))
	h.forward(w, r, "/services/"+id+"/"+action)
}
func (h *ServicesHandler) forward(w http.ResponseWriter, r *http.Request, path string) {
	if h.socket == "" {
		http.Error(w, "Service controller is not configured", http.StatusServiceUnavailable)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://services"+path, nil)
	if err != nil {
		http.Error(w, "Could not prepare service request", http.StatusInternalServerError)
		return
	}
	response, err := h.client.Do(req)
	if err != nil {
		http.Error(w, "Service controller is unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, 2<<20))
}
