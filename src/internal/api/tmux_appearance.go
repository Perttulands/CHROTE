package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chrote/server/internal/core"
)

// MouseModeRequest is the request body for toggling tmux mouse mode.
type MouseModeRequest struct {
	Enabled *bool `json:"enabled"`
}

// AppearanceRequest is the request body for tmux appearance settings
type AppearanceRequest struct {
	StatusBg           string `json:"statusBg"`
	StatusFg           string `json:"statusFg"`
	PaneBorderActive   string `json:"paneBorderActive"`
	PaneBorderInactive string `json:"paneBorderInactive"`
	ModeStyleBg        string `json:"modeStyleBg"`
	ModeStyleFg        string `json:"modeStyleFg"`
}

func (h *TmuxHandler) appearanceTargets() []tmuxTarget {
	users := configuredTerminalUsers()
	if len(users) == 0 {
		return []tmuxTarget{{socket: h.socket, workDir: h.workDir}}
	}
	targets := make([]tmuxTarget, 0, len(users))
	seenSockets := map[string]bool{}
	for _, user := range users {
		target, err := h.targetForUnixUser(user)
		if err != nil {
			continue
		}
		key := target.socket
		if key == "" {
			key = "ambient:"
		}
		if seenSockets[key] {
			continue
		}
		seenSockets[key] = true
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return []tmuxTarget{{socket: h.socket, workDir: h.workDir}}
	}
	return targets
}

var tmuxRightClickMenuKeys = []string{
	"MouseDown3Pane",
	"MouseDown3Status",
	"MouseDown3StatusLeft",
	"M-MouseDown3Pane",
	"M-MouseDown3Status",
	"M-MouseDown3StatusLeft",
}

func (h *TmuxHandler) removeTmuxRightClickMenus(parent context.Context, socket string) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	for _, key := range tmuxRightClickMenuKeys {
		// -q makes an already-absent binding successful; any remaining error is real.
		if _, err := h.runTmuxOnSocketContext(ctx, socket, "unbind-key", "-q", "-n", key); err != nil {
			return fmt.Errorf("remove tmux right-click binding %q: %w", key, err)
		}
	}
	return nil
}

func (h *TmuxHandler) applyMouseMode(parent context.Context, socket, value string) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	if _, err := h.runTmuxOnSocketContext(ctx, socket, "set-option", "-g", "mouse", value); err != nil {
		return err
	}
	return h.removeTmuxRightClickMenus(ctx, socket)
}

// SetMouseMode handles POST /api/tmux/mouse. It toggles tmux's global mouse
// option across all configured CHROTE terminal sockets.
func (h *TmuxHandler) SetMouseMode(w http.ResponseWriter, r *http.Request) {
	var req MouseModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if req.Enabled == nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "enabled must be a boolean")
		return
	}

	value := "off"
	if *req.Enabled {
		value = "on"
	}

	applied := 0
	targets := h.appearanceTargets()
	for _, target := range targets {
		if err := h.applyMouseMode(r.Context(), target.socket, value); err == nil {
			applied++
		}
		// A tmux server/profile may not be running yet; applied/success report that truthfully.
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   applied == len(targets),
		"mouse":     value,
		"applied":   applied,
		"total":     len(targets),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ApplyAppearance handles POST /api/tmux/appearance
func (h *TmuxHandler) ApplyAppearance(w http.ResponseWriter, r *http.Request) {
	var req AppearanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	// Validate colors
	colors := map[string]string{
		"statusBg":           req.StatusBg,
		"statusFg":           req.StatusFg,
		"paneBorderActive":   req.PaneBorderActive,
		"paneBorderInactive": req.PaneBorderInactive,
		"modeStyleBg":        req.ModeStyleBg,
		"modeStyleFg":        req.ModeStyleFg,
	}

	for key, val := range colors {
		if val != "" && !h.colorRegex.MatchString(val) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("Invalid color for %s: %s", key, val))
			return
		}
	}

	// Build tmux set commands
	var commands [][]string
	if req.StatusBg != "" && req.StatusFg != "" {
		commands = append(commands, []string{"set", "-g", "status-style", fmt.Sprintf("bg=%s,fg=%s", req.StatusBg, req.StatusFg)})
	}
	if req.PaneBorderActive != "" {
		commands = append(commands, []string{"set", "-g", "pane-active-border-style", fmt.Sprintf("fg=%s", req.PaneBorderActive)})
	}
	if req.PaneBorderInactive != "" {
		commands = append(commands, []string{"set", "-g", "pane-border-style", fmt.Sprintf("fg=%s", req.PaneBorderInactive)})
	}
	if req.ModeStyleBg != "" && req.ModeStyleFg != "" {
		commands = append(commands, []string{"set", "-g", "mode-style", fmt.Sprintf("bg=%s,fg=%s", req.ModeStyleBg, req.ModeStyleFg)})
	}

	applied := 0
	targets := h.appearanceTargets()
	for _, target := range targets {
		for _, args := range commands {
			_, err := h.runTmuxOnSocket(target.socket, args...)
			if err == nil {
				applied++
			}
			// Ignore errors for appearance - a tmux server/profile might not be running.
		}
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"applied":   applied,
		"total":     len(commands) * len(targets),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
