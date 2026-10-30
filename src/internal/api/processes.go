package api

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

// The process map: every process on the host, grouped by what it belongs to,
// so the Server tab can show what draws the machine's resources.
//
// A process belongs to the tmux session whose pane it runs under, else to the
// systemd service its cgroup names, else to "system". CPU is the share of one
// core since the previous read; the first read takes two samples a moment
// apart so it is never blank.

const (
	// USER_HZ: /proc reports CPU time in these ticks on every Linux the
	// kernel ABI supports.
	processClockTicks        = 100
	processFirstSampleGap    = 300 * time.Millisecond
	processSampleStaleAfter  = 30 * time.Second
	processCommandMaxLength  = 160
	processesPerGroupDefault = 40
)

type ProcessMapResponse struct {
	Timestamp        string         `json:"timestamp"`
	Hostname         string         `json:"hostname"`
	Cores            int            `json:"cores"`
	MemoryTotalBytes uint64         `json:"memoryTotalBytes"`
	Groups           []ProcessGroup `json:"groups"`
}

type ProcessGroup struct {
	Key        string        `json:"key"`
	Kind       string        `json:"kind"` // tmux, service, system
	Name       string        `json:"name"`
	CPUPercent float64       `json:"cpuPercent"`
	RSSBytes   uint64        `json:"rssBytes"`
	Count      int           `json:"count"`
	Processes  []ProcessNode `json:"processes"`
}

type ProcessNode struct {
	PID        int     `json:"pid"`
	PPID       int     `json:"ppid"`
	Name       string  `json:"name"`
	Command    string  `json:"command"`
	CPUPercent float64 `json:"cpuPercent"`
	RSSBytes   uint64  `json:"rssBytes"`
	Threads    int     `json:"threads"`
}

// ProcessHandler serves GET /api/system/processes.
type ProcessHandler struct {
	procRoot string
	panes    func(context.Context) map[int]string
	perGroup int

	mu     sync.Mutex
	prev   map[int]uint64
	prevAt time.Time
}

// NewProcessHandler reads /proc and names tmux sessions through tmux.
func NewProcessHandler(tmux *TmuxHandler) *ProcessHandler {
	return &ProcessHandler{procRoot: "/proc", panes: tmux.panePIDs, perGroup: processesPerGroupDefault}
}

func (h *ProcessHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/system/processes", h.Processes)
}

func (h *ProcessHandler) Processes(w http.ResponseWriter, r *http.Request) {
	result, err := h.Snapshot(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "PROCESS_MAP_ERROR", err.Error())
		return
	}
	core.WriteSuccess(w, result)
}

type rawProcess struct {
	pid, ppid int
	name      string
	ticks     uint64
	rss       uint64
	threads   int
}

func (h *ProcessHandler) Snapshot(ctx context.Context) (ProcessMapResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	procs, err := readProcesses(h.procRoot)
	if err != nil {
		return ProcessMapResponse{}, err
	}
	now := time.Now()
	if h.prev == nil || now.Sub(h.prevAt) > processSampleStaleAfter {
		h.remember(procs, now)
		select {
		case <-ctx.Done():
			return ProcessMapResponse{}, ctx.Err()
		case <-time.After(processFirstSampleGap):
		}
		if procs, err = readProcesses(h.procRoot); err != nil {
			return ProcessMapResponse{}, err
		}
		now = time.Now()
	}
	elapsed := now.Sub(h.prevAt).Seconds()
	cpu := make(map[int]float64, len(procs))
	for _, p := range procs {
		if before, ok := h.prev[p.pid]; ok && elapsed > 0 && p.ticks >= before {
			cpu[p.pid] = float64(p.ticks-before) / processClockTicks / elapsed * 100
		}
	}
	h.remember(procs, now)

	var panes map[int]string
	if h.panes != nil {
		panes = h.panes(ctx)
	}
	groups := groupProcesses(procs, cpu, panes, func(pid int) string {
		return readCgroup(h.procRoot, pid)
	}, func(pid int) string {
		return readCommand(h.procRoot, pid)
	}, h.perGroup)

	hostname, _ := os.Hostname()
	return ProcessMapResponse{
		Timestamp:        now.UTC().Format(time.RFC3339Nano),
		Hostname:         hostname,
		Cores:            runtime.NumCPU(),
		MemoryTotalBytes: readMemTotal(h.procRoot),
		Groups:           groups,
	}, nil
}

func (h *ProcessHandler) remember(procs []rawProcess, at time.Time) {
	h.prev = make(map[int]uint64, len(procs))
	for _, p := range procs {
		h.prev[p.pid] = p.ticks
	}
	h.prevAt = at
}

func readProcesses(procRoot string) ([]rawProcess, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", procRoot, err)
	}
	pageSize := uint64(os.Getpagesize())
	procs := make([]rawProcess, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(procRoot, entry.Name(), "stat"))
		if err != nil {
			continue // it exited, or it is not ours to read
		}
		p, ok := parseProcessStat(pid, string(raw), pageSize)
		// Kernel threads are not anything a person started.
		if !ok || pid == 2 || p.ppid == 2 {
			continue
		}
		procs = append(procs, p)
	}
	return procs, nil
}

// parseProcessStat reads /proc/<pid>/stat. The command name sits in
// parentheses and may itself hold spaces or parentheses, so the fields are
// counted from the last ')'.
func parseProcessStat(pid int, stat string, pageSize uint64) (rawProcess, bool) {
	open := strings.IndexByte(stat, '(')
	end := strings.LastIndexByte(stat, ')')
	if open < 0 || end < open {
		return rawProcess{}, false
	}
	fields := strings.Fields(stat[end+1:])
	// fields[0] is state (field 3); utime is field 14, rss field 24.
	if len(fields) < 22 {
		return rawProcess{}, false
	}
	ppid, _ := strconv.Atoi(fields[1])
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	threads, _ := strconv.Atoi(fields[17])
	rssPages, _ := strconv.ParseUint(fields[21], 10, 64)
	return rawProcess{
		pid:     pid,
		ppid:    ppid,
		name:    stat[open+1 : end],
		ticks:   utime + stime,
		rss:     rssPages * pageSize,
		threads: threads,
	}, true
}

func readCommand(procRoot string, pid int) string {
	raw, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	command := strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", " "))
	if len(command) > processCommandMaxLength {
		command = command[:processCommandMaxLength] + "…"
	}
	return command
}

func readCgroup(procRoot string, pid int) string {
	raw, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return ""
	}
	return cgroupPath(string(raw))
}

// cgroupPath picks the unified hierarchy's path, or the systemd one on a
// host still on cgroup v1.
func cgroupPath(raw string) string {
	fallback := ""
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		if parts[0] == "0" && parts[1] == "" {
			return parts[2]
		}
		if parts[1] == "name=systemd" {
			fallback = parts[2]
		}
	}
	return fallback
}

// serviceFromCgroup names the systemd service a cgroup path belongs to: the
// deepest "x.service" in it, skipping a user's own service manager.
func serviceFromCgroup(path string) string {
	segments := strings.Split(path, "/")
	for i := len(segments) - 1; i >= 0; i-- {
		name, ok := strings.CutSuffix(segments[i], ".service")
		if !ok || strings.HasPrefix(name, "user@") {
			continue
		}
		return strings.ReplaceAll(name, "\\x2d", "-")
	}
	return ""
}

func readMemTotal(procRoot string) uint64 {
	file, err := os.Open(filepath.Join(procRoot, "meminfo"))
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			return kb * 1024
		}
	}
	return 0
}

func groupProcesses(
	procs []rawProcess,
	cpu map[int]float64,
	panes map[int]string,
	cgroup func(int) string,
	command func(int) string,
	perGroup int,
) []ProcessGroup {
	parent := make(map[int]int, len(procs))
	for _, p := range procs {
		parent[p.pid] = p.ppid
	}
	// The session a process runs under: the nearest ancestor that is a pane.
	sessionOf := func(pid int) (string, bool) {
		for hops := 0; pid > 1 && hops < 64; hops++ {
			if name, ok := panes[pid]; ok {
				return name, true
			}
			pid = parent[pid]
		}
		return "", false
	}

	byKey := map[string]*ProcessGroup{}
	members := map[string][]rawProcess{}
	for _, p := range procs {
		kind, name := "system", "system"
		if session, ok := sessionOf(p.pid); ok {
			kind, name = "tmux", session
		} else if strings.HasPrefix(p.name, "tmux") {
			kind, name = "service", "tmux"
		} else if service := serviceFromCgroup(cgroup(p.pid)); service != "" {
			kind, name = "service", service
		}
		key := kind + ":" + name
		group := byKey[key]
		if group == nil {
			group = &ProcessGroup{Key: key, Kind: kind, Name: name}
			byKey[key] = group
		}
		group.CPUPercent += cpu[p.pid]
		group.RSSBytes += p.rss
		group.Count++
		members[key] = append(members[key], p)
	}

	groups := make([]ProcessGroup, 0, len(byKey))
	for key, group := range byKey {
		list := members[key]
		// Keep the heaviest; a process's weight is its CPU, plus memory at
		// one percent per 64 MiB, so an idle giant still shows.
		weight := func(p rawProcess) float64 {
			return cpu[p.pid] + float64(p.rss)/(64<<20)
		}
		sort.Slice(list, func(i, j int) bool {
			wi, wj := weight(list[i]), weight(list[j])
			if wi != wj {
				return wi > wj
			}
			return list[i].pid < list[j].pid
		})
		if perGroup > 0 && len(list) > perGroup {
			list = list[:perGroup]
		}
		sort.Slice(list, func(i, j int) bool { return list[i].pid < list[j].pid })
		group.Processes = make([]ProcessNode, 0, len(list))
		for _, p := range list {
			group.Processes = append(group.Processes, ProcessNode{
				PID:        p.pid,
				PPID:       p.ppid,
				Name:       p.name,
				Command:    command(p.pid),
				CPUPercent: cpu[p.pid],
				RSSBytes:   p.rss,
				Threads:    p.threads,
			})
		}
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Key < groups[j].Key })
	return groups
}

// panePIDs maps each pane's root process to its session, on every configured
// socket. A socket that cannot be read contributes nothing. With several
// Unix users, a session is named user/session.
func (h *TmuxHandler) panePIDs(ctx context.Context) map[int]string {
	users := configuredTerminalUsers()
	panes := map[int]string{}
	for _, unixUser := range users {
		target, err := h.targetForUnixUserContext(ctx, unixUser)
		if err != nil {
			continue
		}
		output, err := h.runTmuxOnSocketContext(ctx, target.socket, "list-panes", "-a", "-F", "#{pane_pid} #{session_name}")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(output, "\n") {
			pidText, session, ok := strings.Cut(strings.TrimSpace(line), " ")
			pid, err := strconv.Atoi(pidText)
			if !ok || err != nil {
				continue
			}
			if len(users) > 1 {
				session = unixUser + "/" + session
			}
			panes[pid] = session
		}
	}
	return panes
}
