package api

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A tmux client is identified only by the terminal it runs on. tmux records no
// origin for a client, so CHROTE recognises its own clients by the ptys it
// spawned: every terminal CHROTE opens runs in a process descended from the
// server, so the controlling terminals of the server's process descendants are
// exactly the ptys CHROTE created. Anything else attached to a session came
// from somewhere else, such as an SSH login.
//
// This reads /proc only. It runs no tmux command and mutates no tmux state.

const (
	// ptsDeviceMajorLow and ptsDeviceMajorHigh bound the Unix 98 pty slave
	// device majors. CHROTE only ever opens a pty, so other terminal kinds
	// cannot be one of its clients and are not translated.
	ptsDeviceMajorLow  = 136
	ptsDeviceMajorHigh = 143

	// ownedPTYWalkLimit bounds the descendant walk. The real tree is two
	// levels deep and a few dozen wide; the limit only stops a pathological
	// process tree from making an inventory expensive.
	ownedPTYWalkLimit = 4096
)

// procSource locates the process filesystem to read. Tests point it at a
// fixture tree.
type procSource struct {
	root string
	pid  int
}

func systemProcSource(pid int) procSource {
	return procSource{root: "/proc", pid: pid}
}

// ownedPTYs returns the pty device paths of every process descended from the
// source pid, including the source itself. An unreadable or absent /proc
// yields an empty set, which reports every client as foreign rather than
// silently reporting none.
func (s procSource) ownedPTYs() map[string]bool {
	owned := map[string]bool{}
	if s.root == "" || s.pid <= 0 {
		return owned
	}
	seen := map[int]bool{s.pid: true}
	queue := []int{s.pid}
	for len(queue) > 0 && len(seen) <= ownedPTYWalkLimit {
		pid := queue[0]
		queue = queue[1:]
		if device := s.controllingPTY(pid); device != "" {
			owned[device] = true
		}
		for _, child := range s.children(pid) {
			if child <= 0 || seen[child] {
				continue
			}
			seen[child] = true
			queue = append(queue, child)
		}
	}
	return owned
}

// controllingPTY reports the pty device a process is attached to, or "" when
// it has no controlling terminal or that terminal is not a pty.
func (s procSource) controllingPTY(pid int) string {
	raw, err := os.ReadFile(filepath.Join(s.root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return ""
	}
	// Field 2 is the executable name in parentheses and may itself contain
	// spaces and parentheses, so the numbered fields are counted from the
	// last closing parenthesis. tty_nr is the fifth field after it.
	commEnd := strings.LastIndex(string(raw), ")")
	if commEnd < 0 {
		return ""
	}
	fields := strings.Fields(string(raw)[commEnd+1:])
	if len(fields) < 5 {
		return ""
	}
	ttyNr, err := strconv.Atoi(fields[4])
	if err != nil || ttyNr == 0 {
		return ""
	}
	return ptyDevicePath(ttyNr)
}

// ptyDevicePath translates a Linux tty_nr device number into a pts device
// path, or "" when the device is not a pty.
func ptyDevicePath(ttyNr int) string {
	major := (ttyNr >> 8) & 0xfff
	if major < ptsDeviceMajorLow || major > ptsDeviceMajorHigh {
		return ""
	}
	minor := (ttyNr & 0xff) | ((ttyNr >> 12) & 0xfff00)
	return "/dev/pts/" + strconv.Itoa(minor)
}

// children reports the direct child processes of a pid. The children list is
// per thread, and a Go program forks from whichever thread happened to run the
// exec, so every thread is read rather than only the main one.
func (s procSource) children(pid int) []int {
	taskDir := filepath.Join(s.root, strconv.Itoa(pid), "task")
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil
	}
	var children []int
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(taskDir, entry.Name(), "children"))
		if err != nil {
			continue
		}
		for _, field := range strings.Fields(string(raw)) {
			child, err := strconv.Atoi(field)
			if err != nil {
				continue
			}
			children = append(children, child)
		}
	}
	return children
}

// countAttachedClients counts the clients in a tmux session_attached_list,
// whoever created them. tmux draws one grid per window however many are
// watching, so this is what tells the operator his pane is showing somebody
// else's dimensions.
func countAttachedClients(attachedList string) int {
	count := 0
	for _, tty := range strings.Split(attachedList, ",") {
		if strings.TrimSpace(tty) != "" {
			count++
		}
	}
	return count
}

// foreignClientTTYs splits a tmux session_attached_list into the client ttys
// CHROTE did not create. Clients with no tty, such as a transient control-mode
// client, cannot be attributed and are not reported.
func foreignClientTTYs(attachedList string, ownedPTYs map[string]bool) []string {
	var foreign []string
	for _, tty := range strings.Split(attachedList, ",") {
		tty = strings.TrimSpace(tty)
		if tty == "" || ownedPTYs[tty] {
			continue
		}
		foreign = append(foreign, tty)
	}
	return foreign
}
