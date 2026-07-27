package api

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Browser terminals attach through ttyd before the page reports its real
// viewport, so they arrive at the terminal default of 80x24. With
// `window-size latest` the newest client wins, which means a hidden keep-alive
// iframe — or every iframe at once after a service restart — clamps live agent
// panes to 80 columns and truncates their TUI output.
//
// The guard marks such a client `ignore-size` instead of detaching it: the
// browser stays attached so reconnects remain instant, but it no longer decides
// how wide an agent's window is. The flag is cleared as soon as the client
// reports a real viewport, so an operator who actually opens the tab gets
// their size back.

const (
	defaultSizeGuardInterval = time.Minute
	defaultSizeGuardIdle     = 5 * time.Minute
	// An un-negotiated ttyd client arrives at exactly this size. A client that
	// reports anything else has told us about a real viewport.
	unnegotiatedClientWidth  = 80
	unnegotiatedClientHeight = 24
	// Size given to a session whose only remaining clients ignore sizing, so an
	// unobserved agent renders wide instead of staying clamped at 80 columns.
	defaultUnobservedWidth  = 200
	defaultUnobservedHeight = 50
)

type tmuxClient struct {
	TTY        string
	Session    string
	Width      int
	Height     int
	ActivityAt time.Time
	IgnoreSize bool
}

// sizeGuardDecision is what the sweep wants to change about one client.
type sizeGuardDecision struct {
	TTY        string
	Session    string
	IgnoreSize bool
}

// parseTmuxClients reads the guard's list-clients format.
func parseTmuxClients(output string) []tmuxClient {
	clients := []tmuxClient{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 6 {
			continue
		}
		width, widthErr := strconv.Atoi(strings.TrimSpace(fields[2]))
		height, heightErr := strconv.Atoi(strings.TrimSpace(fields[3]))
		activity, activityErr := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		if widthErr != nil || heightErr != nil || activityErr != nil {
			continue
		}
		tty := strings.TrimSpace(fields[0])
		if tty == "" {
			continue
		}
		clients = append(clients, tmuxClient{
			TTY:        tty,
			Session:    strings.TrimSpace(fields[1]),
			Width:      width,
			Height:     height,
			ActivityAt: time.Unix(activity, 0),
			IgnoreSize: strings.Contains(fields[5], "ignore-size"),
		})
	}
	return clients
}

// planSizeGuard decides which clients should stop dictating window size.
//
// A client is disqualified only when it is still at the un-negotiated 80x24 and
// has been idle past the threshold: that pair distinguishes an abandoned or
// hidden tab from an operator who is reading output without typing at a
// deliberately small terminal, because the latter has reported its own size.
func planSizeGuard(clients []tmuxClient, now time.Time, idleAfter time.Duration) []sizeGuardDecision {
	decisions := []sizeGuardDecision{}
	for _, client := range clients {
		unnegotiated := client.Width == unnegotiatedClientWidth && client.Height == unnegotiatedClientHeight
		idle := now.Sub(client.ActivityAt) >= idleAfter
		shouldIgnore := unnegotiated && idle
		if shouldIgnore == client.IgnoreSize {
			continue
		}
		decisions = append(decisions, sizeGuardDecision{TTY: client.TTY, Session: client.Session, IgnoreSize: shouldIgnore})
	}
	return decisions
}

// sessionsNeedingResize lists sessions whose every client will be ignoring size
// after the plan is applied. tmux leaves such a window at whatever the last
// sizing client imposed, so the guard has to widen it back explicitly.
func sessionsNeedingResize(clients []tmuxClient, decisions []sizeGuardDecision) []string {
	ignoring := map[string]bool{}
	for _, client := range clients {
		ignoring[client.TTY] = client.IgnoreSize
	}
	for _, decision := range decisions {
		ignoring[decision.TTY] = decision.IgnoreSize
	}

	authoritative := map[string]bool{}
	sessions := []string{}
	seen := map[string]bool{}
	for _, client := range clients {
		if client.Session == "" {
			continue
		}
		if !seen[client.Session] {
			seen[client.Session] = true
			sessions = append(sessions, client.Session)
		}
		if !ignoring[client.TTY] {
			authoritative[client.Session] = true
		}
	}

	needing := []string{}
	for _, session := range sessions {
		if !authoritative[session] {
			needing = append(needing, session)
		}
	}
	return needing
}

func unobservedWindowSize() (int, int) {
	width := defaultUnobservedWidth
	height := defaultUnobservedHeight
	if raw := strings.TrimSpace(os.Getenv("CHROTE_TERMINAL_UNOBSERVED_COLS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= unnegotiatedClientWidth {
			width = parsed
		}
	}
	if raw := strings.TrimSpace(os.Getenv("CHROTE_TERMINAL_UNOBSERVED_ROWS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= unnegotiatedClientHeight {
			height = parsed
		}
	}
	return width, height
}

func sizeGuardIdleAfter() time.Duration {
	raw := strings.TrimSpace(os.Getenv("CHROTE_TERMINAL_SIZE_GUARD_IDLE"))
	if raw == "" {
		return defaultSizeGuardIdle
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return defaultSizeGuardIdle
	}
	return parsed
}

func sizeGuardInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("CHROTE_TERMINAL_SIZE_GUARD_INTERVAL"))
	if raw == "" {
		return defaultSizeGuardInterval
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return defaultSizeGuardInterval
	}
	return parsed
}

// sizeGuardEnabled lets an operator turn the sweep off without a rebuild.
func sizeGuardEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("CHROTE_TERMINAL_SIZE_GUARD")))
	return raw != "0" && raw != "false" && raw != "off"
}

// guardedSockets lists every tmux socket CHROTE manages, deduped.
func (h *TmuxHandler) guardedSockets() []string {
	sockets := []string{}
	seen := map[string]bool{}
	add := func(socket string) {
		socket = strings.TrimSpace(socket)
		if socket == "" || seen[socket] {
			return
		}
		seen[socket] = true
		sockets = append(sockets, socket)
	}
	add(h.socket)
	for user := range allowedTerminalUsers() {
		if target, err := h.targetForUnixUser(user); err == nil {
			add(target.socket)
		}
	}
	return sockets
}

// applySizeGuard runs one sweep over one socket and returns the applied decisions.
func (h *TmuxHandler) applySizeGuard(ctx context.Context, socket string, now time.Time, idleAfter time.Duration) ([]sizeGuardDecision, error) {
	output, err := h.runTmuxOnSocketContext(ctx, socket,
		"list-clients", "-F", "#{client_tty}\t#{client_session}\t#{client_width}\t#{client_height}\t#{client_activity}\t#{client_flags}")
	if err != nil {
		return nil, err
	}
	clients := parseTmuxClients(output)
	decisions := planSizeGuard(clients, now, idleAfter)
	applied := make([]sizeGuardDecision, 0, len(decisions))
	for _, decision := range decisions {
		flag := "ignore-size"
		if !decision.IgnoreSize {
			flag = "!ignore-size"
		}
		if _, err := h.runTmuxOnSocketContext(ctx, socket, "refresh-client", "-f", flag, "-t", decision.TTY); err != nil {
			// One unreachable client must not stop the rest of the sweep.
			continue
		}
		applied = append(applied, decision)
	}

	if len(applied) == 0 {
		return applied, nil
	}

	// An explicit resize-window pins the window to manual sizing, so a session
	// whose client just got its say back has to be handed to automatic sizing
	// again or it would ignore the viewport the client reports.
	for _, decision := range applied {
		if decision.IgnoreSize || decision.Session == "" {
			continue
		}
		if _, err := h.runTmuxOnSocketContext(ctx, socket, "resize-window", "-A", "-t", decision.Session); err != nil {
			continue
		}
	}

	width, height := unobservedWindowSize()
	for _, session := range sessionsNeedingResize(clients, applied) {
		if _, err := h.runTmuxOnSocketContext(ctx, socket,
			"resize-window", "-t", session, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height)); err != nil {
			continue
		}
	}
	return applied, nil
}

// StartTerminalSizeGuard sweeps managed sockets until ctx is cancelled. The
// returned channel closes once the sweep has stopped.
func (h *TmuxHandler) StartTerminalSizeGuard(ctx context.Context, report func(error)) <-chan struct{} {
	done := make(chan struct{})
	if !sizeGuardEnabled() {
		close(done)
		return done
	}
	interval := sizeGuardInterval()
	idleAfter := sizeGuardIdleAfter()

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			for _, socket := range h.guardedSockets() {
				if ctx.Err() != nil {
					return
				}
				if _, err := h.applySizeGuard(ctx, socket, time.Now(), idleAfter); err != nil && report != nil {
					// A socket that is absent or not yet shared is normal here.
					report(fmt.Errorf("terminal size guard sweep on %s: %w", socket, err))
				}
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return done
}
