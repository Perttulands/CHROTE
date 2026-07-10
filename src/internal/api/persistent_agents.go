package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

const defaultPersistentAgentsFile = "/srv/data/chrote/persistent-agents/agents.json"

func defaultPersistentAgentsPath() string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_PERSISTENT_AGENTS_PATH")); override != "" {
		return override
	}
	return defaultPersistentAgentsFile
}

// PersistentAgentEntry is CHROTE's desired-state record for a long-running
// Codex/Claude session. tmux is disposable transport; this record says the
// agent transcript should stay running under the named tmux session.
type PersistentAgentEntry struct {
	Name           string `json:"name"`
	UnixUser       string `json:"unixUser,omitempty"`
	Identity       string `json:"identity,omitempty"`
	AgentKind      string `json:"agentKind"`
	AgentSessionID string `json:"agentSessionId"`
	ResumeCommand  string `json:"resumeCommand"`
	CWD            string `json:"cwd,omitempty"`
	TranscriptPath string `json:"transcriptPath,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	LastCheckAt    string `json:"lastCheckAt,omitempty"`
	LastRestartAt  string `json:"lastRestartAt,omitempty"`
	LastError      string `json:"lastError,omitempty"`
}

// EnablePersistentAgentRequest is the request body for making a live session supervised.
type EnablePersistentAgentRequest struct {
	UnixUser       string `json:"unixUser,omitempty"`
	NewName        string `json:"newName,omitempty"`
	Identity       string `json:"identity,omitempty"`
	AgentKind      string `json:"agentKind"`
	AgentSessionID string `json:"agentSessionId"`
	CWD            string `json:"cwd,omitempty"`
	TranscriptPath string `json:"transcriptPath,omitempty"`
}

// PersistentAgentReconcileResult describes one desired-state reconciliation decision.
type PersistentAgentReconcileResult struct {
	Session        string `json:"session"`
	UnixUser       string `json:"unixUser,omitempty"`
	AgentKind      string `json:"agentKind"`
	AgentSessionID string `json:"agentSessionId"`
	Action         string `json:"action"`
	Error          string `json:"error,omitempty"`
}

type persistentAgentStore struct {
	path string
	mu   sync.Mutex
}

func newPersistentAgentStore(path string) *persistentAgentStore {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultPersistentAgentsFile
	}
	return &persistentAgentStore{path: path}
}

func persistentAgentKey(name, unixUser string) string {
	return strings.TrimSpace(unixUser) + "\x00" + strings.TrimSpace(name)
}

func sanitizePersistentIdentity(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func sanitizePersistentAgentEntry(entry PersistentAgentEntry) (PersistentAgentEntry, bool) {
	entry.Name = strings.TrimSpace(entry.Name)
	entry.UnixUser = strings.TrimSpace(entry.UnixUser)
	if valid, _ := core.ValidateSessionName(entry.Name, "session name"); !valid {
		return PersistentAgentEntry{}, false
	}
	command, ok := canonicalAgentResumeCommand(entry.AgentKind, entry.AgentSessionID)
	if !ok {
		return PersistentAgentEntry{}, false
	}
	entry.Identity = sanitizePersistentIdentity(entry.Identity)
	entry.AgentKind = strings.ToLower(strings.TrimSpace(entry.AgentKind))
	entry.AgentSessionID = strings.TrimSpace(entry.AgentSessionID)
	entry.ResumeCommand = command
	entry.CWD = sanitizeRecoveryPath(entry.CWD, true)
	entry.TranscriptPath = sanitizeRecoveryPath(entry.TranscriptPath, false)
	return entry, true
}

func sanitizePersistentAgentEntries(entries []PersistentAgentEntry) []PersistentAgentEntry {
	result := make([]PersistentAgentEntry, 0, len(entries))
	for _, entry := range entries {
		entry, ok := sanitizePersistentAgentEntry(entry)
		if !ok {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func (s *persistentAgentStore) Read() ([]PersistentAgentEntry, error) {
	if s == nil {
		return []PersistentAgentEntry{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	return sanitizePersistentAgentEntries(entries), nil
}

func (s *persistentAgentStore) Upsert(name, unixUser string, req EnablePersistentAgentRequest) (PersistentAgentEntry, error) {
	if s == nil {
		return PersistentAgentEntry{}, fmt.Errorf("persistent agent store is unavailable")
	}
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	if valid, errMsg := core.ValidateSessionName(name, "session name"); !valid {
		return PersistentAgentEntry{}, fmt.Errorf("%s", errMsg)
	}
	command, ok := canonicalAgentResumeCommand(req.AgentKind, req.AgentSessionID)
	if !ok {
		return PersistentAgentEntry{}, fmt.Errorf("unsafe or unsupported agent persistence metadata")
	}
	cwd := sanitizeRecoveryPath(req.CWD, true)
	if strings.TrimSpace(req.CWD) != "" && cwd == "" {
		return PersistentAgentEntry{}, fmt.Errorf("unsafe persistence working directory")
	}
	transcript := sanitizeRecoveryPath(req.TranscriptPath, false)
	if strings.TrimSpace(req.TranscriptPath) != "" && transcript == "" {
		return PersistentAgentEntry{}, fmt.Errorf("unsafe transcript path")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return PersistentAgentEntry{}, err
	}
	entries = sanitizePersistentAgentEntries(entries)
	key := persistentAgentKey(name, unixUser)
	entry := PersistentAgentEntry{
		Name:           name,
		UnixUser:       unixUser,
		Identity:       sanitizePersistentIdentity(req.Identity),
		AgentKind:      strings.ToLower(strings.TrimSpace(req.AgentKind)),
		AgentSessionID: strings.TrimSpace(req.AgentSessionID),
		ResumeCommand:  command,
		CWD:            cwd,
		TranscriptPath: transcript,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	updated := false
	for i, existing := range entries {
		if persistentAgentKey(existing.Name, existing.UnixUser) != key {
			continue
		}
		entry.CreatedAt = existing.CreatedAt
		if entry.CreatedAt == "" {
			entry.CreatedAt = now
		}
		entry.LastCheckAt = existing.LastCheckAt
		entry.LastRestartAt = existing.LastRestartAt
		entry.LastError = existing.LastError
		entries[i] = entry
		updated = true
		break
	}
	if !updated {
		entries = append(entries, entry)
	}
	if err := s.saveLocked(entries); err != nil {
		return PersistentAgentEntry{}, err
	}
	return entry, nil
}

func (s *persistentAgentStore) Forget(name, unixUser string) (bool, error) {
	if s == nil {
		return false, nil
	}
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	if valid, _ := core.ValidateSessionName(name, "session name"); !valid {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return false, err
	}
	entries = sanitizePersistentAgentEntries(entries)
	key := persistentAgentKey(name, unixUser)
	filtered := make([]PersistentAgentEntry, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) == key {
			removed = true
			continue
		}
		filtered = append(filtered, entry)
	}
	if !removed {
		return false, nil
	}
	if err := s.saveLocked(filtered); err != nil {
		return false, err
	}
	return true, nil
}

func (s *persistentAgentStore) Rename(oldName, newName, unixUser string) error {
	if s == nil {
		return nil
	}
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	unixUser = strings.TrimSpace(unixUser)
	if oldName == newName {
		return nil
	}
	if valid, _ := core.ValidateSessionName(oldName, "current session name"); !valid {
		return nil
	}
	if valid, errMsg := core.ValidateSessionName(newName, "new session name"); !valid {
		return fmt.Errorf("%s", errMsg)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	entries = sanitizePersistentAgentEntries(entries)
	oldKey := persistentAgentKey(oldName, unixUser)
	updated := false
	for i, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) != oldKey {
			continue
		}
		entry.Name = newName
		entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		entries[i] = entry
		updated = true
		break
	}
	if !updated {
		return nil
	}
	return s.saveLocked(entries)
}

func (s *persistentAgentStore) IsPersistent(name, unixUser string) (bool, error) {
	entries, err := s.Read()
	if err != nil {
		return false, err
	}
	key := persistentAgentKey(name, unixUser)
	for _, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) == key {
			return true, nil
		}
	}
	return false, nil
}

func (s *persistentAgentStore) NamesForUser(unixUser string) (map[string]bool, error) {
	entries, err := s.Read()
	if err != nil {
		return nil, err
	}
	unixUser = strings.TrimSpace(unixUser)
	result := map[string]bool{}
	for _, entry := range entries {
		if strings.TrimSpace(entry.UnixUser) == unixUser {
			result[entry.Name] = true
		}
	}
	return result, nil
}

func (s *persistentAgentStore) AnnotateSessions(sessions []core.Session) ([]core.Session, error) {
	entries, err := s.Read()
	if err != nil {
		return sessions, err
	}
	byKey := map[string]PersistentAgentEntry{}
	for _, entry := range entries {
		byKey[persistentAgentKey(entry.Name, entry.UnixUser)] = entry
	}
	for i := range sessions {
		entry, ok := byKey[persistentAgentKey(sessions[i].Name, sessions[i].UnixUser)]
		if !ok {
			continue
		}
		sessions[i].Persistent = true
		sessions[i].PersistentIdentity = entry.Identity
		sessions[i].PersistentAgentKind = entry.AgentKind
		sessions[i].PersistentAgentSessionID = entry.AgentSessionID
		sessions[i].PersistentResumeCommand = entry.ResumeCommand
		sessions[i].PersistentLastError = entry.LastError
	}
	return sessions, nil
}

func (s *persistentAgentStore) FilterBanked(entries []SessionBankEntry) ([]SessionBankEntry, error) {
	persistentEntries, err := s.Read()
	if err != nil {
		return entries, err
	}
	blocked := map[string]bool{}
	for _, entry := range persistentEntries {
		blocked[persistentAgentKey(entry.Name, entry.UnixUser)] = true
	}
	filtered := make([]SessionBankEntry, 0, len(entries))
	for _, entry := range entries {
		if blocked[persistentAgentKey(entry.Name, entry.UnixUser)] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered, nil
}

func (s *persistentAgentStore) FilterLiveSessionsForBank(sessions []core.Session) ([]core.Session, error) {
	persistentEntries, err := s.Read()
	if err != nil {
		return sessions, err
	}
	blocked := map[string]bool{}
	for _, entry := range persistentEntries {
		blocked[persistentAgentKey(entry.Name, entry.UnixUser)] = true
	}
	filtered := make([]core.Session, 0, len(sessions))
	for _, session := range sessions {
		if blocked[persistentAgentKey(session.Name, session.UnixUser)] {
			continue
		}
		filtered = append(filtered, session)
	}
	return filtered, nil
}

func (s *persistentAgentStore) UpdateStatus(name, unixUser, action, errText string) error {
	if s == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	entries = sanitizePersistentAgentEntries(entries)
	key := persistentAgentKey(name, unixUser)
	for i, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) != key {
			continue
		}
		entry.LastCheckAt = now
		entry.UpdatedAt = now
		entry.LastError = strings.TrimSpace(errText)
		if action == "recreated" || action == "restarted" {
			entry.LastRestartAt = now
		}
		entries[i] = entry
		break
	}
	return s.saveLocked(entries)
}

func (s *persistentAgentStore) loadLocked() ([]PersistentAgentEntry, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []PersistentAgentEntry{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []PersistentAgentEntry{}, nil
	}
	var entries []PersistentAgentEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("read persistent agents %s: %w", s.path, err)
	}
	return entries, nil
}

func (s *persistentAgentStore) saveLocked(entries []PersistentAgentEntry) error {
	entries = sanitizePersistentAgentEntries(entries)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].UnixUser != entries[j].UnixUser {
			return entries[i].UnixUser < entries[j].UnixUser
		}
		return entries[i].Name < entries[j].Name
	})
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o2770); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o2770)
	tmp, err := os.CreateTemp(dir, ".tmp-persistent-agents-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o660); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}
	_ = os.Chmod(s.path, 0o660)
	return nil
}

type paneInspection struct {
	PID     string
	Command string
	CWD     string
}

func parsePaneInspection(output string) paneInspection {
	output = strings.TrimSpace(output)
	if output == "" {
		return paneInspection{}
	}
	parts := strings.SplitN(output, ":", 3)
	if len(parts) == 3 {
		return paneInspection{PID: strings.TrimSpace(parts[0]), Command: strings.TrimSpace(parts[1]), CWD: sanitizeRecoveryPath(parts[2], true)}
	}
	cmd, cwd, ok := strings.Cut(output, ":")
	if !ok {
		return paneInspection{Command: strings.TrimSpace(output)}
	}
	return paneInspection{Command: strings.TrimSpace(cmd), CWD: sanitizeRecoveryPath(cwd, true)}
}

func matchesAgentCommand(command, kind string) bool {
	command = strings.ToLower(strings.TrimSpace(filepath.Base(command)))
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "codex" {
		return command == "codex"
	}
	if kind == "claude" {
		return command == "claude"
	}
	return false
}

func argvContainsAgentExecutable(args, kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	for _, token := range strings.Fields(args) {
		cleaned := strings.Trim(token, `"'`)
		base := strings.ToLower(filepath.Base(cleaned))
		if base == kind || strings.TrimSuffix(base, ".js") == kind {
			return true
		}
	}
	return false
}

func processLooksLikeAgent(command, args, kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	command = strings.ToLower(strings.TrimSpace(filepath.Base(command)))
	if matchesAgentCommand(command, kind) {
		return true
	}
	if command != "node" && command != "nodejs" {
		return false
	}
	return argvContainsAgentExecutable(args, kind)
}

type processInfo struct {
	pid  string
	ppid string
	comm string
	args string
}

var readPersistentAgentProcessTable = readProcessTable

func readProcessTable(ctx context.Context) ([]processInfo, error) {
	cmd := exec.CommandContext(ctx, "ps", "-eo", "pid=,ppid=,comm=,args=")
	raw, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %s", err.Error(), string(exitErr.Stderr))
		}
		return nil, err
	}
	infos := []processInfo{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		args := ""
		if len(parts) > 3 {
			args = strings.Join(parts[3:], " ")
		}
		infos = append(infos, processInfo{pid: parts[0], ppid: parts[1], comm: parts[2], args: args})
	}
	return infos, nil
}

func processTreeContainsAgentInTable(infos []processInfo, panePID, kind string) bool {
	for _, info := range processTreeForPane(infos, panePID) {
		if processLooksLikeAgent(info.comm, info.args, kind) {
			return true
		}
	}
	return false
}

func processTreeForPane(infos []processInfo, panePID string) []processInfo {
	panePID = strings.TrimSpace(panePID)
	if panePID == "" {
		return nil
	}
	childrenByParent := map[string][]processInfo{}
	byPID := map[string]processInfo{}
	for _, info := range infos {
		childrenByParent[info.ppid] = append(childrenByParent[info.ppid], info)
		byPID[info.pid] = info
	}
	queue := []processInfo{}
	if root, ok := byPID[panePID]; ok {
		queue = append(queue, root)
	} else {
		queue = append(queue, childrenByParent[panePID]...)
	}
	seen := map[string]bool{}
	result := []processInfo{}
	for len(queue) > 0 {
		info := queue[0]
		queue = queue[1:]
		if seen[info.pid] {
			continue
		}
		seen[info.pid] = true
		result = append(result, info)
		queue = append(queue, childrenByParent[info.pid]...)
	}
	return result
}

type inferredPersistentAgentMetadata struct {
	Kind       string
	SessionID  string
	Source     string
	Confidence string
}

type persistentAgentOwnerProbeResponse struct {
	Kind       string `json:"kind"`
	SessionID  string `json:"sessionId"`
	Confidence string `json:"confidence"`
	Error      string `json:"error"`
}

func normalizeProcessArgToken(token string) string {
	return strings.Trim(token, `"' ,;()[]<>`)
}

func looksLikeInferredAgentSessionID(token string) bool {
	token = normalizeProcessArgToken(token)
	if strings.Count(token, "-") < 4 {
		return false
	}
	return agentSessionIDRegex.MatchString(token)
}

func inferCodexSessionIDFromArgs(args string) string {
	foundResume := false
	for _, token := range strings.Fields(args) {
		cleaned := normalizeProcessArgToken(token)
		if cleaned == "resume" {
			foundResume = true
			continue
		}
		if !foundResume || strings.HasPrefix(cleaned, "-") {
			continue
		}
		if looksLikeInferredAgentSessionID(cleaned) {
			return cleaned
		}
	}
	return ""
}

func inferClaudeSessionIDFromArgs(args string) string {
	tokens := strings.Fields(args)
	for i, token := range tokens {
		cleaned := normalizeProcessArgToken(token)
		if strings.HasPrefix(cleaned, "--resume=") {
			candidate := strings.TrimPrefix(cleaned, "--resume=")
			if looksLikeInferredAgentSessionID(candidate) {
				return normalizeProcessArgToken(candidate)
			}
			continue
		}
		if cleaned != "--resume" {
			continue
		}
		for _, candidate := range tokens[i+1:] {
			candidate = normalizeProcessArgToken(candidate)
			if strings.HasPrefix(candidate, "-") {
				continue
			}
			if looksLikeInferredAgentSessionID(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func inferAgentSessionIDFromArgs(kind, args string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "codex":
		return inferCodexSessionIDFromArgs(args)
	case "claude":
		return inferClaudeSessionIDFromArgs(args)
	default:
		return ""
	}
}

func inferPersistentAgentMetadataInTable(infos []processInfo, panePID, requestedKind string) (inferredPersistentAgentMetadata, bool, bool) {
	requestedKind = strings.ToLower(strings.TrimSpace(requestedKind))
	foundAgent := false
	for _, info := range processTreeForPane(infos, panePID) {
		for _, kind := range []string{"codex", "claude"} {
			if requestedKind != "" && requestedKind != kind {
				continue
			}
			if !processLooksLikeAgent(info.comm, info.args, kind) {
				continue
			}
			foundAgent = true
			if sessionID := inferAgentSessionIDFromArgs(kind, info.args); sessionID != "" {
				return inferredPersistentAgentMetadata{Kind: kind, SessionID: sessionID, Source: "process"}, true, true
			}
		}
	}
	return inferredPersistentAgentMetadata{}, foundAgent, false
}

const persistentAgentOwnerProbeResultPrefix = "CHROTE_PROBE_RESULT "

const persistentAgentOwnerProbeShellCommand = `python3 - <<'PY'
import base64
import json
import os
import pathlib
import re

PREFIX = "CHROTE_PROBE_RESULT "
UUID_RE = re.compile(r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}")

def env_b64(name):
    raw = os.environ.get(name, "")
    if not raw:
        return ""
    try:
        return base64.b64decode(raw).decode("utf-8", "replace")
    except Exception:
        return ""

def emit(obj):
    print(PREFIX + json.dumps(obj, separators=(",", ":")), flush=True)

probe_cwd = env_b64("CHROTE_PROBE_CWD")
requested_kind = env_b64("CHROTE_PROBE_KIND").strip().lower()
pane_pid = env_b64("CHROTE_PROBE_PANE_PID").strip()
home = pathlib.Path.home()

def kind_allowed(kind):
    return not requested_kind or requested_kind == kind

def classify_transcript_path(value):
    value = value.removesuffix(" (deleted)")
    name = pathlib.Path(value).name
    match = UUID_RE.search(name)
    if not match:
        return None
    if "/.codex/sessions/" in value and value.endswith(".jsonl") and kind_allowed("codex"):
        return {"kind": "codex", "sessionId": match.group(0), "confidence": "high"}
    if "/.claude/projects/" in value and value.endswith(".jsonl") and kind_allowed("claude"):
        return {"kind": "claude", "sessionId": match.group(0), "confidence": "high"}
    return None

def process_descendants(root_pid):
    if not root_pid.isdigit():
        return []
    children = {}
    for stat_path in pathlib.Path("/proc").glob("[0-9]*/stat"):
        pid = stat_path.parent.name
        try:
            text = stat_path.read_text(errors="replace")
            after = text.rsplit(") ", 1)[1].split()
            ppid = after[1]
        except Exception:
            continue
        children.setdefault(ppid, []).append(pid)
    queue = [root_pid]
    seen = set()
    result = []
    while queue:
        pid = queue.pop(0)
        if pid in seen:
            continue
        seen.add(pid)
        result.append(pid)
        queue.extend(children.get(pid, []))
    return result

def fd_candidate():
    for pid in process_descendants(pane_pid):
        fd_dir = pathlib.Path("/proc") / pid / "fd"
        try:
            fds = list(fd_dir.iterdir())
        except Exception:
            continue
        for fd in fds:
            try:
                target = os.readlink(fd)
            except Exception:
                continue
            candidate = classify_transcript_path(target)
            if candidate:
                return candidate
    return None

def read_json_lines(path, limit):
    try:
        with path.open("r", encoding="utf-8", errors="replace") as handle:
            for index, line in enumerate(handle):
                if index >= limit:
                    break
                try:
                    yield json.loads(line)
                except Exception:
                    continue
    except Exception:
        return

def codex_file_candidates():
    root = home / ".codex" / "sessions"
    if not root.exists() or not kind_allowed("codex"):
        return []
    paths = [p for p in root.glob("**/*.jsonl") if p.is_file()]
    paths.sort(key=lambda p: p.stat().st_mtime if p.exists() else 0, reverse=True)
    result = []
    for path in paths[:200]:
        match = UUID_RE.search(path.name)
        session_id = match.group(0) if match else ""
        cwd = ""
        for item in read_json_lines(path, 30):
            payload = item.get("payload") if isinstance(item, dict) else None
            if isinstance(payload, dict):
                session_id = str(payload.get("id") or session_id)
                cwd = str(payload.get("cwd") or cwd)
            if session_id and cwd:
                break
        if session_id and (not probe_cwd or cwd == probe_cwd):
            result.append((path.stat().st_mtime, {"kind": "codex", "sessionId": session_id, "confidence": "low"}))
    return result

def claude_file_candidates():
    root = home / ".claude" / "projects"
    if not root.exists() or not kind_allowed("claude"):
        return []
    paths = [p for p in root.glob("**/*.jsonl") if p.is_file()]
    paths.sort(key=lambda p: p.stat().st_mtime if p.exists() else 0, reverse=True)
    result = []
    for path in paths[:200]:
        match = UUID_RE.search(path.name)
        session_id = match.group(0) if match else ""
        cwd = ""
        for item in read_json_lines(path, 120):
            if isinstance(item, dict):
                session_id = str(item.get("sessionId") or session_id)
                cwd = str(item.get("cwd") or cwd)
            if session_id and cwd:
                break
        if session_id and (not probe_cwd or cwd == probe_cwd):
            result.append((path.stat().st_mtime, {"kind": "claude", "sessionId": session_id, "confidence": "low"}))
    return result

candidate = fd_candidate()
if candidate:
    emit(candidate)
else:
    candidates = codex_file_candidates() + claude_file_candidates()
    candidates.sort(key=lambda item: item[0], reverse=True)
    unique = []
    seen = set()
    for _, item in candidates:
        key = (item["kind"], item["sessionId"])
        if key not in seen:
            seen.add(key)
            unique.append(item)
    if len(unique) == 1:
        emit(unique[0])
    elif len(unique) > 1:
        emit({"error": "owner probe found multiple matching agent transcripts"})
    else:
        emit({"error": "owner probe found no matching agent transcript"})
PY
printf '\nCHROTE_PROBE_DONE\n'
sleep 2
`

var probePersistentAgentOwnerMetadata = runPersistentAgentOwnerProbe

func encodeProbeEnv(name, value string) string {
	return name + "=" + base64.StdEncoding.EncodeToString([]byte(value))
}

func parsePersistentAgentOwnerProbeOutput(raw string) (inferredPersistentAgentMetadata, error) {
	var last persistentAgentOwnerProbeResponse
	found := false
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		payload, ok := strings.CutPrefix(line, persistentAgentOwnerProbeResultPrefix)
		if !ok {
			continue
		}
		found = true
		if err := json.Unmarshal([]byte(payload), &last); err != nil {
			return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe returned invalid metadata")
		}
	}
	if !found {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe did not return metadata")
	}
	if strings.TrimSpace(last.Error) != "" {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("%s", strings.TrimSpace(last.Error))
	}
	last.Kind = strings.ToLower(strings.TrimSpace(last.Kind))
	last.SessionID = strings.TrimSpace(last.SessionID)
	if _, ok := canonicalAgentResumeCommand(last.Kind, last.SessionID); !ok {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe returned unsafe or unsupported metadata")
	}
	confidence := strings.ToLower(strings.TrimSpace(last.Confidence))
	if confidence == "" {
		confidence = "low"
	}
	return inferredPersistentAgentMetadata{Kind: last.Kind, SessionID: last.SessionID, Source: "owner-probe", Confidence: confidence}, nil
}

func runPersistentAgentOwnerProbe(ctx context.Context, h *TmuxHandler, target tmuxTarget, pane paneInspection, requestedKind string) (inferredPersistentAgentMetadata, error) {
	if h == nil {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe unavailable")
	}
	if strings.TrimSpace(target.socket) == "" {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe requires a tmux socket")
	}
	probeName := fmt.Sprintf("chrote-probe-%d-%d", os.Getpid(), time.Now().UnixNano())
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_, err := h.runTmuxOnSocketContext(probeCtx, target.socket,
		"new-session", "-d", "-s", probeName,
		"-e", encodeProbeEnv("CHROTE_PROBE_CWD", pane.CWD),
		"-e", encodeProbeEnv("CHROTE_PROBE_KIND", requestedKind),
		"-e", encodeProbeEnv("CHROTE_PROBE_PANE_PID", pane.PID),
		persistentAgentOwnerProbeShellCommand,
	)
	if err != nil {
		return inferredPersistentAgentMetadata{}, err
	}
	defer func() {
		_, _ = h.runTmuxOnSocketContext(context.Background(), target.socket, "kill-session", "-t", probeName)
	}()

	deadline := time.Now().Add(6 * time.Second)
	lastCapture := ""
	for time.Now().Before(deadline) {
		capture, captureErr := h.runTmuxOnSocketContext(probeCtx, target.socket, "capture-pane", "-p", "-J", "-t", probeName)
		if captureErr == nil {
			lastCapture = capture
			if strings.Contains(capture, persistentAgentOwnerProbeResultPrefix) || strings.Contains(capture, "CHROTE_PROBE_DONE") {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return parsePersistentAgentOwnerProbeOutput(lastCapture)
}

func capitalizeFirst(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (h *TmuxHandler) inferPersistentAgentMetadata(ctx context.Context, target tmuxTarget, pane paneInspection, requestedKind string) (inferredPersistentAgentMetadata, error) {
	infos, err := readPersistentAgentProcessTable(ctx)
	if err != nil {
		return inferredPersistentAgentMetadata{}, err
	}
	metadata, foundAgent, foundSessionID := inferPersistentAgentMetadataInTable(infos, pane.PID, requestedKind)
	if foundSessionID {
		return metadata, nil
	}
	if foundAgent {
		metadata, probeErr := probePersistentAgentOwnerMetadata(ctx, h, target, pane, requestedKind)
		if probeErr == nil {
			return metadata, nil
		}
		return inferredPersistentAgentMetadata{}, fmt.Errorf("could not infer Codex/Claude session id: live agent process has no resume session id in its arguments and owner probe failed: %w", probeErr)
	}
	return inferredPersistentAgentMetadata{}, fmt.Errorf("could not infer Codex/Claude session id: session is not running a supported live agent")
}

func processTreeContainsAgent(ctx context.Context, panePID, kind string) (bool, error) {
	panePID = strings.TrimSpace(panePID)
	if panePID == "" {
		return false, nil
	}
	infos, err := readPersistentAgentProcessTable(ctx)
	if err != nil {
		return false, err
	}
	return processTreeContainsAgentInTable(infos, panePID, kind), nil
}

func agentProcessLive(ctx context.Context, pane paneInspection, kind string) (bool, error) {
	if matchesAgentCommand(pane.Command, kind) {
		return true, nil
	}
	return processTreeContainsAgent(ctx, pane.PID, kind)
}

func (h *TmuxHandler) inspectSessionPane(ctx context.Context, socket, sessionName string) (paneInspection, error) {
	output, err := h.runTmuxOnSocketContext(ctx, socket, "display-message", "-p", "-t", sessionName, "#{pane_pid}:#{pane_current_command}:#{pane_current_path}")
	if err != nil {
		return paneInspection{}, err
	}
	return parsePaneInspection(output), nil
}

func persistenceWorkDir(entry PersistentAgentEntry, target tmuxTarget) string {
	if entry.CWD != "" {
		return entry.CWD
	}
	if target.workDir != "" {
		return target.workDir
	}
	return core.GetWorkDir()
}

// EnablePersistentAgent handles POST /api/tmux/sessions/{name}/persistence.
func (h *TmuxHandler) EnablePersistentAgent(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	var req EnablePersistentAgentRequest
	if err := decodeOptionalJSONBody(r, &req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	unixUser := strings.TrimSpace(req.UnixUser)
	if r != nil {
		if queryUser := strings.TrimSpace(r.URL.Query().Get("unixUser")); queryUser != "" {
			unixUser = queryUser
		}
	}
	newName := strings.TrimSpace(req.NewName)
	if newName == "" {
		newName = sessionName
	}
	if valid, errMsg := core.ValidateSessionName(newName, "new session name"); !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	target, targetErr := h.targetForUnixUser(unixUser)
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}
	pane, err := h.inspectSessionPane(r.Context(), target.socket, sessionName)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "TMUX_ERROR", err.Error())
		return
	}
	if strings.TrimSpace(req.AgentKind) == "" || strings.TrimSpace(req.AgentSessionID) == "" {
		metadata, err := h.inferPersistentAgentMetadata(r.Context(), target, pane, req.AgentKind)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", capitalizeFirst(err.Error()))
			return
		}
		if strings.TrimSpace(req.AgentKind) == "" {
			req.AgentKind = metadata.Kind
		}
		if strings.TrimSpace(req.AgentSessionID) == "" {
			req.AgentSessionID = metadata.SessionID
		}
	}
	if _, ok := canonicalAgentResumeCommand(req.AgentKind, req.AgentSessionID); !ok {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Unsafe or unsupported agent persistence metadata")
		return
	}
	agentLive, err := agentProcessLive(r.Context(), pane, req.AgentKind)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Could not inspect session process tree: "+err.Error())
		return
	}
	if !agentLive {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("Session is running %q, not %s", pane.Command, strings.ToLower(strings.TrimSpace(req.AgentKind))))
		return
	}
	if strings.TrimSpace(req.CWD) == "" {
		req.CWD = pane.CWD
	}
	if strings.TrimSpace(req.CWD) == "" {
		req.CWD = target.workDir
	}
	if strings.TrimSpace(req.CWD) == "" {
		req.CWD = core.GetWorkDir()
	}
	entry, err := h.persistent.Upsert(newName, unixUser, req)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "PERSISTENT_AGENT_ERROR", err.Error())
		return
	}
	if newName != sessionName {
		if _, err := h.runTmuxOnSocket(target.socket, "rename-session", "-t", sessionName, newName); err != nil {
			_, _ = h.persistent.Forget(newName, unixUser)
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
			return
		}
	}
	h.invalidateCache()
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"session":        entry.Name,
		"unixUser":       entry.UnixUser,
		"persistent":     true,
		"identity":       entry.Identity,
		"agentKind":      entry.AgentKind,
		"agentSessionId": entry.AgentSessionID,
		"resumeCommand":  entry.ResumeCommand,
		"cwd":            entry.CWD,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
}

// DisablePersistentAgent handles DELETE /api/tmux/sessions/{name}/persistence.
func (h *TmuxHandler) DisablePersistentAgent(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	unixUser := ""
	if r != nil {
		unixUser = strings.TrimSpace(r.URL.Query().Get("unixUser"))
	}
	removed, err := h.persistent.Forget(sessionName, unixUser)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_ERROR", err.Error())
		return
	}
	h.invalidateCache()
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"session":    sessionName,
		"unixUser":   unixUser,
		"persistent": false,
		"removed":    removed,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *TmuxHandler) revivePersistentAgent(ctx context.Context, entry PersistentAgentEntry, target tmuxTarget) error {
	workDir := persistenceWorkDir(entry, target)
	if workDir == "" {
		return fmt.Errorf("no persistence working directory available")
	}
	resumeCommand, ok := canonicalAgentResumeCommand(entry.AgentKind, entry.AgentSessionID)
	if !ok {
		return fmt.Errorf("unsafe or unsupported persistent agent metadata")
	}
	if _, err := h.runTmuxOnSocketContext(ctx, target.socket, "new-session", "-d", "-s", entry.Name, "-c", workDir); err != nil {
		return err
	}
	if _, err := h.runTmuxOnSocketContext(ctx, target.socket, "send-keys", "-t", entry.Name, "-l", "--", resumeCommand); err != nil {
		h.recoverFailureCleanup(target.socket, entry.Name)
		return err
	}
	if _, err := h.runTmuxOnSocketContext(ctx, target.socket, "send-keys", "-t", entry.Name, "Enter"); err != nil {
		h.recoverFailureCleanup(target.socket, entry.Name)
		return err
	}
	return nil
}

// ReconcilePersistentAgents applies desired state once. Tests call this directly;
// the server also runs it periodically in the background.
func (h *TmuxHandler) ReconcilePersistentAgents(ctx context.Context) ([]PersistentAgentReconcileResult, error) {
	if h == nil || h.persistent == nil {
		return []PersistentAgentReconcileResult{}, nil
	}
	entries, err := h.persistent.Read()
	if err != nil {
		return nil, err
	}
	results := make([]PersistentAgentReconcileResult, 0, len(entries))
	for _, entry := range entries {
		result := PersistentAgentReconcileResult{
			Session:        entry.Name,
			UnixUser:       entry.UnixUser,
			AgentKind:      entry.AgentKind,
			AgentSessionID: entry.AgentSessionID,
			Action:         "ok",
		}
		target, targetErr := h.targetForUnixUser(entry.UnixUser)
		if targetErr != nil {
			result.Action = "error"
			result.Error = targetErr.Error()
			_ = h.persistent.UpdateStatus(entry.Name, entry.UnixUser, result.Action, result.Error)
			results = append(results, result)
			continue
		}
		if _, ok := canonicalAgentResumeCommand(entry.AgentKind, entry.AgentSessionID); !ok {
			result.Action = "error"
			result.Error = "unsafe or unsupported persistent agent metadata"
			_ = h.persistent.UpdateStatus(entry.Name, entry.UnixUser, result.Action, result.Error)
			results = append(results, result)
			continue
		}
		if _, err := h.runTmuxOnSocketContext(ctx, target.socket, "has-session", "-t", entry.Name); err != nil {
			if reviveErr := h.revivePersistentAgent(ctx, entry, target); reviveErr != nil {
				result.Action = "error"
				result.Error = reviveErr.Error()
			} else {
				result.Action = "recreated"
			}
			_ = h.persistent.UpdateStatus(entry.Name, entry.UnixUser, result.Action, result.Error)
			results = append(results, result)
			continue
		}
		pane, err := h.inspectSessionPane(ctx, target.socket, entry.Name)
		if err != nil {
			result.Action = "error"
			result.Error = err.Error()
			_ = h.persistent.UpdateStatus(entry.Name, entry.UnixUser, result.Action, result.Error)
			results = append(results, result)
			continue
		}
		live, err := agentProcessLive(ctx, pane, entry.AgentKind)
		if err != nil {
			result.Action = "error"
			result.Error = err.Error()
			_ = h.persistent.UpdateStatus(entry.Name, entry.UnixUser, result.Action, result.Error)
			results = append(results, result)
			continue
		}
		if !live {
			_, _ = h.runTmuxOnSocketContext(ctx, target.socket, "kill-session", "-t", entry.Name)
			if reviveErr := h.revivePersistentAgent(ctx, entry, target); reviveErr != nil {
				result.Action = "error"
				result.Error = reviveErr.Error()
			} else {
				result.Action = "restarted"
			}
			_ = h.persistent.UpdateStatus(entry.Name, entry.UnixUser, result.Action, result.Error)
			results = append(results, result)
			continue
		}
		_ = h.persistent.UpdateStatus(entry.Name, entry.UnixUser, result.Action, "")
		results = append(results, result)
	}
	return results, nil
}

// StartPersistentAgentSupervisor keeps persistent agents reconciled until ctx ends.
func (h *TmuxHandler) StartPersistentAgentSupervisor(ctx context.Context) {
	if h == nil || h.persistent == nil {
		return
	}
	if disabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("CHROTE_PERSISTENT_AGENTS_DISABLE"))); disabled {
		return
	}
	interval := 15 * time.Second
	if raw := strings.TrimSpace(os.Getenv("CHROTE_PERSISTENT_AGENTS_INTERVAL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	go func() {
		_, _ = h.ReconcilePersistentAgents(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = h.ReconcilePersistentAgents(ctx)
			}
		}
	}()
}
