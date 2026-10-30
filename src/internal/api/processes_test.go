package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseProcessStatCountsFieldsFromTheLastParenthesis(t *testing.T) {
	stat := "4242 (my (odd) name) S 7 4242 4242 0 -1 4194560 100 0 0 0 150 50 0 0 20 0 3 0 12345 1000000 25 18446744073709551615"
	p, ok := parseProcessStat(4242, stat, 4096)
	if !ok {
		t.Fatal("not parsed")
	}
	if p.name != "my (odd) name" || p.ppid != 7 || p.ticks != 200 || p.threads != 3 || p.rss != 25*4096 {
		t.Fatalf("got %+v", p)
	}
}

func TestServiceFromCgroupNamesTheDeepestService(t *testing.T) {
	cases := map[string]string{
		"/system.slice/nginx.service":                                          "nginx",
		"/user.slice/user-1000.slice/user@1000.service/app.slice/chrote.service": "chrote",
		"/user.slice/user-1000.slice/user@1000.service/init.scope":               "",
		"/system.slice/system-getty.slice/getty@tty1.service":                   "getty@tty1",
		"/system.slice/foo\\x2dbar.service":                                    "foo-bar",
		"/init.scope":                                                          "",
	}
	for path, want := range cases {
		if got := serviceFromCgroup(path); got != want {
			t.Errorf("serviceFromCgroup(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestCgroupPathPrefersTheUnifiedHierarchy(t *testing.T) {
	if got := cgroupPath("12:cpu:/x\n1:name=systemd:/system.slice/a.service\n0::/system.slice/b.service\n"); got != "/system.slice/b.service" {
		t.Fatalf("got %q", got)
	}
	if got := cgroupPath("1:name=systemd:/system.slice/a.service\n"); got != "/system.slice/a.service" {
		t.Fatalf("got %q", got)
	}
}

func TestGroupProcessesPutsPaneDescendantsInTheirSession(t *testing.T) {
	procs := []rawProcess{
		{pid: 1, ppid: 0, name: "systemd", rss: 10 << 20},
		{pid: 100, ppid: 1, name: "tmux: server", rss: 4 << 20},
		{pid: 200, ppid: 100, name: "bash"},
		{pid: 210, ppid: 200, name: "claude", rss: 400 << 20},
		{pid: 220, ppid: 210, name: "rg"},
		{pid: 300, ppid: 1, name: "nginx", rss: 50 << 20},
	}
	cgroups := map[int]string{300: "/system.slice/nginx.service"}
	groups := groupProcesses(procs, map[int]float64{210: 18, 300: 2}, map[int]string{200: "chrote3-claude"},
		func(pid int) string { return cgroups[pid] }, func(int) string { return "" }, 40)

	byKey := map[string]ProcessGroup{}
	for _, g := range groups {
		byKey[g.Key] = g
	}
	session := byKey["tmux:chrote3-claude"]
	if session.Count != 3 || session.CPUPercent != 18 || session.RSSBytes != 400<<20 {
		t.Fatalf("session group %+v", session)
	}
	if byKey["service:nginx"].Count != 1 || byKey["service:tmux"].Count != 1 || byKey["system:system"].Count != 1 {
		t.Fatalf("groups %+v", byKey)
	}
}

func TestGroupProcessesKeepsTheHeaviestPerGroup(t *testing.T) {
	procs := []rawProcess{
		{pid: 10, ppid: 1, name: "small"},
		{pid: 11, ppid: 1, name: "big", rss: 1 << 30},
		{pid: 12, ppid: 1, name: "busy"},
	}
	groups := groupProcesses(procs, map[int]float64{12: 50}, nil,
		func(int) string { return "" }, func(int) string { return "" }, 2)
	if len(groups) != 1 || groups[0].Count != 3 || len(groups[0].Processes) != 2 {
		t.Fatalf("groups %+v", groups)
	}
	for _, p := range groups[0].Processes {
		if p.Name == "small" {
			t.Fatal("kept the lightest process")
		}
	}
}

func TestSnapshotReadsAFakeProc(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("meminfo", "MemTotal:       2048 kB\n")
	write("7/stat", "7 (worker) R 1 7 7 0 -1 0 0 0 0 0 10 0 0 0 20 0 1 0 1 1 3 0")
	write("7/cmdline", "worker\x00--fast\x00")
	write("7/cgroup", "0::/system.slice/jobs.service\n")
	write("9/stat", "9 (kworker/0:1) I 2 0 0 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1 0 0 0")

	h := &ProcessHandler{procRoot: root, perGroup: 40}
	got, err := h.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.MemoryTotalBytes != 2048*1024 || len(got.Groups) != 1 {
		t.Fatalf("got %+v", got)
	}
	g := got.Groups[0]
	if g.Key != "service:jobs" || len(g.Processes) != 1 || g.Processes[0].Command != "worker --fast" {
		t.Fatalf("group %+v", g)
	}
}
