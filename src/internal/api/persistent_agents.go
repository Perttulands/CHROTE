package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

const defaultPersistentAgentsFile = "/srv/data/chrote/persistent-agents/agents.json"

const (
	PersistentAgentStateStarting         = "starting"
	PersistentAgentStateHealthy          = "healthy"
	PersistentAgentStateNeedsInteraction = "needs_interaction"
	PersistentAgentStateWrongIdentity    = "wrong_identity"
	PersistentAgentStateBackoff          = "backoff"
	PersistentAgentStateFailed           = "failed"

	persistentAgentPaneTailLines         = 80
	persistentAgentMaxLaunchFailures     = 3
	persistentAgentInitialBackoff        = time.Minute
	persistentAgentMaxBackoff            = 5 * time.Minute
	persistentAgentEnableMaxRequestBytes = 256 << 10
	persistentAgentHermesModule          = "hermes_cli.main"
	persistentAgentHermesExecutableTail  = "/.hermes/hermes-agent-current/venv/bin/python"
)

var persistentAgentNow = func() time.Time {
	return time.Now().UTC()
}

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
	Name                      string                      `json:"name"`
	UnixUser                  string                      `json:"unixUser,omitempty"`
	Identity                  string                      `json:"identity,omitempty"`
	AgentKind                 string                      `json:"agentKind"`
	AgentSessionID            string                      `json:"agentSessionId"`
	ResumeCommand             string                      `json:"resumeCommand"`
	CWD                       string                      `json:"cwd,omitempty"`
	TranscriptPath            string                      `json:"transcriptPath,omitempty"`
	RecoveryDescriptor        *WorkloadRecoveryDescriptor `json:"recoveryDescriptor,omitempty"`
	State                     string                      `json:"state,omitempty"`
	ConsecutiveLaunchFailures int                         `json:"consecutiveLaunchFailures,omitempty"`
	NextRetryAt               string                      `json:"nextRetryAt,omitempty"`
	CreatedAt                 string                      `json:"createdAt"`
	UpdatedAt                 string                      `json:"updatedAt"`
	LastCheckAt               string                      `json:"lastCheckAt,omitempty"`
	LastRestartAt             string                      `json:"lastRestartAt,omitempty"`
	LastError                 string                      `json:"lastError,omitempty"`
}

// EnablePersistentAgentRequest is the request body for making a live session supervised.
type EnablePersistentAgentRequest struct {
	UnixUser           string                      `json:"unixUser,omitempty"`
	NewName            string                      `json:"newName,omitempty"`
	Identity           string                      `json:"identity,omitempty"`
	AgentKind          string                      `json:"agentKind"`
	AgentSessionID     string                      `json:"agentSessionId"`
	CWD                string                      `json:"cwd,omitempty"`
	TranscriptPath     string                      `json:"transcriptPath,omitempty"`
	RecoveryDescriptor *WorkloadRecoveryDescriptor `json:"recoveryDescriptor,omitempty"`
}

// PersistentAgentReconcileResult describes one desired-state reconciliation decision.
// annotateStatusPersistError surfaces a failed reconcile bookkeeping write on
// the result instead of letting the report claim durable state that was never
// saved (ctx-6m5).
func annotateStatusPersistError(result *PersistentAgentReconcileResult, err error) {
	if err == nil {
		return
	}
	note := "status not persisted: " + err.Error()
	if result.Error == "" {
		result.Error = note
		return
	}
	result.Error += "; " + note
}

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

func persistentAgentOwnerRef(unixUser, sessionName string) string {
	return "persistent:" + sessionBankOwnerRef(unixUser, sessionName)
}

func sanitizePersistentIdentity(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func sanitizePersistentAgentState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PersistentAgentStateStarting:
		return PersistentAgentStateStarting
	case PersistentAgentStateHealthy:
		return PersistentAgentStateHealthy
	case PersistentAgentStateNeedsInteraction:
		return PersistentAgentStateNeedsInteraction
	case PersistentAgentStateWrongIdentity:
		return PersistentAgentStateWrongIdentity
	case PersistentAgentStateBackoff:
		return PersistentAgentStateBackoff
	case PersistentAgentStateFailed:
		return PersistentAgentStateFailed
	default:
		return ""
	}
}

func sanitizePersistentRetryTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return ""
	}
	return value
}

func storedHermesResumeCommandLooksCanonical(command, profile, sessionID string) bool {
	if strings.ContainsAny(command, "\x00\n\r") {
		return false
	}
	tokens := strings.Fields(command)
	if len(tokens) != 7 {
		return false
	}
	executable := filepath.Clean(tokens[0])
	if !filepath.IsAbs(executable) || !strings.HasSuffix(executable, persistentAgentHermesExecutableTail) {
		return false
	}
	want := []string{"-m", persistentAgentHermesModule, "--profile", profile, "--resume", sessionID}
	for i, token := range want {
		if tokens[i+1] != token {
			return false
		}
	}
	return true
}

func canonicalPersistentAgentDescriptor(name, unixUser, ownerHome string, raw WorkloadRecoveryDescriptor) (WorkloadRecoveryDescriptor, error) {
	desc, err := CanonicalizeWorkloadRecoveryDescriptor(raw, ownerHome)
	if err != nil {
		return WorkloadRecoveryDescriptor{}, err
	}
	if desc.Mode == RecoveryModeManaged || desc.Owner.Kind == RecoveryOwnerExternalManager {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("managed descriptors cannot be made persistent")
	}
	if desc.Mode != RecoveryModeAgent || desc.Agent == nil {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("persistent agents require an agent recovery descriptor")
	}
	if desc.Owner.Kind != RecoveryOwnerPersistentAgent {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("persistent agents require a persistent_agent recovery owner")
	}
	expectedOwnerRef := persistentAgentOwnerRef(unixUser, name)
	if desc.Owner.Ref != expectedOwnerRef {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("recovery owner ref %q does not match target %q", desc.Owner.Ref, expectedOwnerRef)
	}
	if !desc.Owner.MayRestart {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("persistent agent recovery owner must be restartable")
	}
	if desc.Topology.SessionName != "" && desc.Topology.SessionName != name {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("recovery descriptor targets session %q, want %q", desc.Topology.SessionName, name)
	}
	if desc.Topology.SessionName == "" {
		desc.Topology.SessionName = name
	}
	cwd, err := canonicalOwnerHomePath(desc.Topology.PaneCurrentPath, "", ownerHome)
	if err != nil {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("persistent agent topology cwd: %w", err)
	}
	desc.Topology.PaneCurrentPath = cwd
	return desc, nil
}

func persistentDescriptorFromLegacyFields(name, unixUser, kind, sessionID, cwd string) WorkloadRecoveryDescriptor {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return WorkloadRecoveryDescriptor{
		Mode: RecoveryModeAgent,
		Owner: WorkloadRecoveryOwner{
			Kind:       RecoveryOwnerPersistentAgent,
			Ref:        persistentAgentOwnerRef(unixUser, name),
			MayRestart: true,
		},
		Topology: WorkloadRecoveryTopology{
			SessionName:     name,
			WindowIndex:     0,
			PaneIndex:       0,
			PaneCurrentPath: strings.TrimSpace(cwd),
		},
		WorkloadKind: kind,
		Agent: &WorkloadRecoveryAgent{
			Kind:            kind,
			NativeSessionID: strings.TrimSpace(sessionID),
		},
		EvidenceSource: RecoveryEvidenceStateDB,
		Confidence:     RecoveryConfidenceHigh,
	}
}

func persistentDescriptorCommand(desc WorkloadRecoveryDescriptor, ownerHome string) (string, bool) {
	if command, ok := desc.CanonicalCommand(ownerHome); ok {
		return command, true
	}
	if desc.Agent == nil {
		return "", false
	}
	return canonicalAgentResumeCommand(desc.Agent.Kind, desc.Agent.NativeSessionID)
}

func sanitizePersistentAgentEntry(entry PersistentAgentEntry) (PersistentAgentEntry, bool) {
	entry.Name = strings.TrimSpace(entry.Name)
	entry.UnixUser = strings.TrimSpace(entry.UnixUser)
	if valid, _ := core.ValidateSessionName(entry.Name, "session name"); !valid {
		return PersistentAgentEntry{}, false
	}
	entry.Identity = sanitizePersistentIdentity(entry.Identity)
	entry.CWD = sanitizeRecoveryPath(entry.CWD, true)
	entry.TranscriptPath = sanitizeRecoveryPath(entry.TranscriptPath, false)
	entry.State = sanitizePersistentAgentState(entry.State)
	if entry.ConsecutiveLaunchFailures < 0 {
		entry.ConsecutiveLaunchFailures = 0
	}
	entry.NextRetryAt = sanitizePersistentRetryTimestamp(entry.NextRetryAt)
	if entry.State != PersistentAgentStateBackoff {
		entry.NextRetryAt = ""
	}
	if entry.State != PersistentAgentStateBackoff && entry.State != PersistentAgentStateFailed {
		entry.ConsecutiveLaunchFailures = 0
	}
	if entry.RecoveryDescriptor != nil {
		desc, err := canonicalPersistentAgentDescriptor(entry.Name, entry.UnixUser, "/", *entry.RecoveryDescriptor)
		if err != nil {
			return PersistentAgentEntry{}, false
		}
		entry.RecoveryDescriptor = &desc
		entry.AgentKind = desc.Agent.Kind
		entry.AgentSessionID = desc.Agent.NativeSessionID
		entry.CWD = desc.Topology.PaneCurrentPath
		switch desc.Agent.Kind {
		case RecoveryAgentCodex, RecoveryAgentClaude:
			command, _ := canonicalAgentResumeCommand(desc.Agent.Kind, desc.Agent.NativeSessionID)
			entry.ResumeCommand = command
		case RecoveryAgentHermes:
			entry.ResumeCommand = ""
		}
		return entry, true
	}
	command, ok := canonicalAgentResumeCommand(entry.AgentKind, entry.AgentSessionID)
	if !ok {
		return PersistentAgentEntry{}, false
	}
	entry.AgentKind = strings.ToLower(strings.TrimSpace(entry.AgentKind))
	entry.AgentSessionID = strings.TrimSpace(entry.AgentSessionID)
	entry.ResumeCommand = command
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

func validatePersistentAgentEntries(entries []PersistentAgentEntry) ([]PersistentAgentEntry, error) {
	result := make([]PersistentAgentEntry, 0, len(entries))
	seen := map[string]int{}
	for i, entry := range entries {
		rawName := strings.TrimSpace(entry.Name)
		rawUser := strings.TrimSpace(entry.UnixUser)
		sanitized, ok := sanitizePersistentAgentEntry(entry)
		if !ok {
			return nil, fmt.Errorf("invalid persistent agent record %d for %q/%q", i, rawUser, rawName)
		}
		key := persistentAgentKey(sanitized.Name, sanitized.UnixUser)
		if first, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate persistent agent record %d for %q/%q; first seen at record %d", i, sanitized.UnixUser, sanitized.Name, first)
		}
		seen[key] = i
		result = append(result, sanitized)
	}
	return result, nil
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
	return validatePersistentAgentEntries(entries)
}

func (s *persistentAgentStore) Upsert(name, unixUser string, req EnablePersistentAgentRequest, ownerHome string) (PersistentAgentEntry, error) {
	if s == nil {
		return PersistentAgentEntry{}, fmt.Errorf("persistent agent store is unavailable")
	}
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	if valid, errMsg := core.ValidateSessionName(name, "session name"); !valid {
		return PersistentAgentEntry{}, fmt.Errorf("%s", errMsg)
	}
	ownerHome = filepath.Clean(strings.TrimSpace(ownerHome))
	if !filepath.IsAbs(ownerHome) {
		return PersistentAgentEntry{}, fmt.Errorf("trusted owner home is required for persistent agent recovery")
	}
	var descriptor WorkloadRecoveryDescriptor
	var err error
	if req.RecoveryDescriptor != nil {
		descriptor, err = canonicalPersistentAgentDescriptor(name, unixUser, ownerHome, *req.RecoveryDescriptor)
		if err != nil {
			return PersistentAgentEntry{}, err
		}
	} else {
		descriptor, err = canonicalPersistentAgentDescriptor(name, unixUser, ownerHome, persistentDescriptorFromLegacyFields(name, unixUser, req.AgentKind, req.AgentSessionID, req.CWD))
		if err != nil {
			return PersistentAgentEntry{}, fmt.Errorf("unsafe or unsupported agent persistence metadata")
		}
	}
	command, ok := persistentDescriptorCommand(descriptor, ownerHome)
	if !ok {
		return PersistentAgentEntry{}, fmt.Errorf("persistent agent descriptor has no canonical command")
	}
	storedCommand := command
	if descriptor.Agent.Kind == RecoveryAgentHermes {
		storedCommand = ""
	}
	cwd := descriptor.Topology.PaneCurrentPath
	transcript := sanitizeRecoveryPath(req.TranscriptPath, false)
	if strings.TrimSpace(req.TranscriptPath) != "" && transcript == "" {
		return PersistentAgentEntry{}, fmt.Errorf("unsafe transcript path")
	}

	now := persistentAgentNow().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return PersistentAgentEntry{}, err
	}
	entries, err = validatePersistentAgentEntries(entries)
	if err != nil {
		return PersistentAgentEntry{}, err
	}
	key := persistentAgentKey(name, unixUser)
	entry := PersistentAgentEntry{
		Name:               name,
		UnixUser:           unixUser,
		Identity:           sanitizePersistentIdentity(req.Identity),
		AgentKind:          descriptor.Agent.Kind,
		AgentSessionID:     descriptor.Agent.NativeSessionID,
		ResumeCommand:      storedCommand,
		CWD:                cwd,
		TranscriptPath:     transcript,
		RecoveryDescriptor: &descriptor,
		State:              PersistentAgentStateHealthy,
		CreatedAt:          now,
		UpdatedAt:          now,
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

func (s *persistentAgentStore) EnsureTargetAvailable(name, unixUser string) error {
	if s == nil {
		return nil
	}
	exists, err := s.IsPersistent(name, unixUser)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("persistent target %q already exists for user %q", strings.TrimSpace(name), strings.TrimSpace(unixUser))
	}
	return nil
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
	entries, err = validatePersistentAgentEntries(entries)
	if err != nil {
		return false, err
	}
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
	entries, err = validatePersistentAgentEntries(entries)
	if err != nil {
		return err
	}
	oldKey := persistentAgentKey(oldName, unixUser)
	newKey := persistentAgentKey(newName, unixUser)
	for _, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) == newKey {
			return fmt.Errorf("persistent target %q already exists for user %q", newName, unixUser)
		}
	}
	updated := false
	for i, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) != oldKey {
			continue
		}
		entry.Name = newName
		if entry.RecoveryDescriptor != nil {
			entry.RecoveryDescriptor.Owner.Ref = persistentAgentOwnerRef(unixUser, newName)
			entry.RecoveryDescriptor.Topology.SessionName = newName
		}
		entry.UpdatedAt = persistentAgentNow().Format(time.RFC3339)
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
		sessions[i].PersistentState = entry.State
		sessions[i].PersistentConsecutiveLaunchFailures = entry.ConsecutiveLaunchFailures
		sessions[i].PersistentNextRetryAt = entry.NextRetryAt
		sessions[i].PersistentLastCheckAt = entry.LastCheckAt
		sessions[i].PersistentLastRestartAt = entry.LastRestartAt
		sessions[i].PersistentLastError = entry.LastError
		sessions[i].PersistentHermesProfile = persistentAgentHermesProfile(entry)
	}
	return sessions, nil
}

func persistentAgentHermesProfile(entry PersistentAgentEntry) string {
	if entry.RecoveryDescriptor == nil || entry.RecoveryDescriptor.Agent == nil {
		return ""
	}
	if entry.RecoveryDescriptor.Agent.Kind != RecoveryAgentHermes {
		return ""
	}
	return strings.TrimSpace(entry.RecoveryDescriptor.Agent.HermesProfile)
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
	now := persistentAgentNow().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	entries, err = validatePersistentAgentEntries(entries)
	if err != nil {
		return err
	}
	key := persistentAgentKey(name, unixUser)
	for i, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) != key {
			continue
		}
		entry.LastCheckAt = now
		entry.UpdatedAt = now
		entry.LastError = strings.TrimSpace(errText)
		switch action {
		case "ok", PersistentAgentStateHealthy:
			entry.State = PersistentAgentStateHealthy
			entry.ConsecutiveLaunchFailures = 0
			entry.NextRetryAt = ""
		case "recreated", "restarted", PersistentAgentStateStarting:
			entry.State = PersistentAgentStateStarting
			entry.ConsecutiveLaunchFailures = 0
			entry.NextRetryAt = ""
			entry.LastRestartAt = now
		case PersistentAgentStateNeedsInteraction:
			entry.State = PersistentAgentStateNeedsInteraction
			entry.NextRetryAt = ""
		case PersistentAgentStateWrongIdentity:
			entry.State = PersistentAgentStateWrongIdentity
			entry.NextRetryAt = ""
		case PersistentAgentStateFailed:
			entry.State = PersistentAgentStateFailed
			entry.NextRetryAt = ""
		}
		entries[i] = entry
		break
	}
	return s.saveLocked(entries)
}

func persistentAgentBackoffDelay(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := persistentAgentInitialBackoff
	for i := 1; i < failures; i++ {
		delay *= 2
		if delay >= persistentAgentMaxBackoff {
			return persistentAgentMaxBackoff
		}
	}
	return delay
}

func (s *persistentAgentStore) RecordLaunchFailure(name, unixUser, errText string) (PersistentAgentEntry, error) {
	if s == nil {
		return PersistentAgentEntry{}, nil
	}
	nowTime := persistentAgentNow()
	now := nowTime.Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return PersistentAgentEntry{}, err
	}
	entries, err = validatePersistentAgentEntries(entries)
	if err != nil {
		return PersistentAgentEntry{}, err
	}
	key := persistentAgentKey(name, unixUser)
	var updated PersistentAgentEntry
	for i, entry := range entries {
		if persistentAgentKey(entry.Name, entry.UnixUser) != key {
			continue
		}
		entry.LastCheckAt = now
		entry.UpdatedAt = now
		entry.LastError = strings.TrimSpace(errText)
		entry.ConsecutiveLaunchFailures++
		if entry.ConsecutiveLaunchFailures >= persistentAgentMaxLaunchFailures {
			entry.State = PersistentAgentStateFailed
			entry.NextRetryAt = ""
		} else {
			entry.State = PersistentAgentStateBackoff
			entry.NextRetryAt = nowTime.Add(persistentAgentBackoffDelay(entry.ConsecutiveLaunchFailures)).Format(time.RFC3339)
		}
		entries[i] = entry
		updated = entry
		break
	}
	if err := s.saveLocked(entries); err != nil {
		return PersistentAgentEntry{}, err
	}
	return updated, nil
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
	var err error
	entries, err = validatePersistentAgentEntries(entries)
	if err != nil {
		return err
	}
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
	if info, err := os.Stat(dir); err == nil && info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("persistent agent store directory is not writable: %s", dir)
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
	if err := tmp.Sync(); err != nil {
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
	return core.FsyncDir(dir)
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

func managedHermesPythonPath(ownerHome string) string {
	return filepath.Join(filepath.Clean(strings.TrimSpace(ownerHome)), ".hermes", "hermes-agent-current", "venv", "bin", "python")
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

func tokenAfterFlag(tokens []string, flag string) (string, bool, bool) {
	found := ""
	count := 0
	for i := 0; i < len(tokens); i++ {
		token := normalizeProcessArgToken(tokens[i])
		if strings.HasPrefix(token, flag+"=") {
			value := strings.TrimPrefix(token, flag+"=")
			if value != "" {
				found = value
				count++
			}
			continue
		}
		if token != flag || i+1 >= len(tokens) {
			continue
		}
		value := normalizeProcessArgToken(tokens[i+1])
		if value != "" {
			found = value
			count++
		}
		i++
	}
	return found, count > 0, count > 1
}

func inferHermesMetadataFromArgs(args, ownerHome string) (inferredPersistentAgentMetadata, bool, bool, error) {
	ownerHome = filepath.Clean(strings.TrimSpace(ownerHome))
	if !filepath.IsAbs(ownerHome) {
		return inferredPersistentAgentMetadata{}, false, false, nil
	}
	tokens := strings.Fields(args)
	if len(tokens) == 0 {
		return inferredPersistentAgentMetadata{}, false, false, nil
	}
	executable := filepath.Clean(strings.Trim(normalizeProcessArgToken(tokens[0]), `"`))
	if executable != managedHermesPythonPath(ownerHome) {
		return inferredPersistentAgentMetadata{}, false, false, nil
	}
	foundModule := false
	for i := 0; i+1 < len(tokens); i++ {
		if normalizeProcessArgToken(tokens[i]) == "-m" && normalizeProcessArgToken(tokens[i+1]) == persistentAgentHermesModule {
			foundModule = true
			break
		}
	}
	if !foundModule {
		return inferredPersistentAgentMetadata{}, false, false, nil
	}
	profile, hasProfile, multiProfile := tokenAfterFlag(tokens, "--profile")
	resumeID, hasResume, multiResume := tokenAfterFlag(tokens, "--resume")
	if multiProfile || multiResume {
		return inferredPersistentAgentMetadata{}, true, false, fmt.Errorf("hermes process has multiple profile or resume identities")
	}
	if !hasProfile || !recoveryHermesProfileRegex.MatchString(profile) {
		return inferredPersistentAgentMetadata{}, true, false, fmt.Errorf("hermes profile is missing or malformed")
	}
	if !hasResume {
		return inferredPersistentAgentMetadata{Kind: RecoveryAgentHermes, HermesProfile: profile, Source: "process", Confidence: RecoveryConfidenceLow}, true, false, nil
	}
	if !recoveryNativeIDRegex.MatchString(resumeID) {
		return inferredPersistentAgentMetadata{}, true, false, fmt.Errorf("hermes resume id is malformed")
	}
	return inferredPersistentAgentMetadata{
		Kind:          RecoveryAgentHermes,
		SessionID:     resumeID,
		HermesProfile: profile,
		Source:        "process",
		Confidence:    RecoveryConfidenceHigh,
	}, true, true, nil
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
	Kind          string
	SessionID     string
	HermesProfile string
	Source        string
	Confidence    string
}

func persistentAgentIdentityKey(metadata inferredPersistentAgentMetadata) string {
	return metadata.Kind + "\x00" + metadata.SessionID + "\x00" + metadata.HermesProfile
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
	metadata, foundAgent, foundSessionID, _ := inferPersistentAgentMetadataInTableForOwner(infos, panePID, requestedKind, "")
	return metadata, foundAgent, foundSessionID
}

func inferPersistentAgentMetadataInTableForOwner(infos []processInfo, panePID, requestedKind, ownerHome string) (inferredPersistentAgentMetadata, bool, bool, error) {
	requestedKind = strings.ToLower(strings.TrimSpace(requestedKind))
	foundAgent := false
	var partial inferredPersistentAgentMetadata
	candidates := map[string]inferredPersistentAgentMetadata{}
	for _, info := range processTreeForPane(infos, panePID) {
		for _, kind := range []string{"codex", "claude"} {
			if !processLooksLikeAgent(info.comm, info.args, kind) {
				continue
			}
			if requestedKind == "" || requestedKind == kind {
				foundAgent = true
			}
			if sessionID := inferAgentSessionIDFromArgs(kind, info.args); sessionID != "" {
				metadata := inferredPersistentAgentMetadata{Kind: kind, SessionID: sessionID, Source: "process", Confidence: RecoveryConfidenceHigh}
				candidates[persistentAgentIdentityKey(metadata)] = metadata
			}
		}
		metadata, foundHermes, foundSessionID, err := inferHermesMetadataFromArgs(info.args, ownerHome)
		if err != nil {
			if requestedKind == "" || requestedKind == RecoveryAgentHermes {
				return inferredPersistentAgentMetadata{}, true, false, err
			}
			return inferredPersistentAgentMetadata{}, foundAgent, false, err
		}
		if foundHermes {
			if requestedKind == "" || requestedKind == RecoveryAgentHermes {
				foundAgent = true
				partial = metadata
			}
			if foundSessionID {
				candidates[persistentAgentIdentityKey(metadata)] = metadata
			}
		}
	}
	if len(candidates) > 1 {
		return inferredPersistentAgentMetadata{}, foundAgent, false, fmt.Errorf("multiple identified agent identities found in pane process tree")
	}
	for _, metadata := range candidates {
		if requestedKind == "" || metadata.Kind == requestedKind {
			return metadata, true, true, nil
		}
	}
	return partial, foundAgent, false, nil
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

requested_kind = env_b64("CHROTE_PROBE_KIND").strip().lower()
pane_pid = env_b64("CHROTE_PROBE_PANE_PID").strip()

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

def fd_candidates():
    candidates = []
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
                candidates.append(candidate)
    return candidates

candidates = {}
for candidate in fd_candidates():
    key = (candidate.get("kind", ""), candidate.get("sessionId", ""))
    candidates[key] = candidate
if len(candidates) == 1:
    emit(next(iter(candidates.values())))
elif len(candidates) == 0:
    emit({"error": "owner probe found no matching open agent transcript"})
else:
    emit({"error": "owner probe found multiple open agent transcripts"})
PY
printf '\nCHROTE_PROBE_DONE\n'
sleep 2
`

var probePersistentAgentOwnerMetadata = runPersistentAgentOwnerProbe

func encodeProbeEnv(name, value string) string {
	return name + "=" + base64.StdEncoding.EncodeToString([]byte(value))
}

func parsePersistentAgentOwnerProbeOutput(raw string) (inferredPersistentAgentMetadata, error) {
	results := map[string]persistentAgentOwnerProbeResponse{}
	var probeErrors []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		payload, ok := strings.CutPrefix(line, persistentAgentOwnerProbeResultPrefix)
		if !ok {
			continue
		}
		var result persistentAgentOwnerProbeResponse
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe returned invalid metadata")
		}
		if strings.TrimSpace(result.Error) != "" {
			probeErrors = append(probeErrors, strings.TrimSpace(result.Error))
			continue
		}
		result.Kind = strings.ToLower(strings.TrimSpace(result.Kind))
		result.SessionID = strings.TrimSpace(result.SessionID)
		if _, ok := canonicalAgentResumeCommand(result.Kind, result.SessionID); !ok {
			return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe returned unsafe or unsupported metadata")
		}
		results[result.Kind+"\x00"+result.SessionID] = result
	}
	if len(results) == 0 && len(probeErrors) == 0 {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe did not return metadata")
	}
	if len(results) == 0 {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("%s", probeErrors[len(probeErrors)-1])
	}
	if len(probeErrors) > 0 {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe returned both metadata and errors")
	}
	if len(results) > 1 {
		return inferredPersistentAgentMetadata{}, fmt.Errorf("owner probe found multiple open agent transcripts")
	}
	var last persistentAgentOwnerProbeResponse
	for _, result := range results {
		last = result
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
	metadata, foundAgent, foundSessionID, inferErr := inferPersistentAgentMetadataInTableForOwner(infos, pane.PID, requestedKind, target.ownerHome)
	if inferErr != nil {
		return inferredPersistentAgentMetadata{}, inferErr
	}
	if foundSessionID {
		return metadata, nil
	}
	if foundAgent {
		if strings.EqualFold(strings.TrimSpace(requestedKind), RecoveryAgentHermes) || metadata.Kind == RecoveryAgentHermes {
			return inferredPersistentAgentMetadata{}, fmt.Errorf("hermes resume id is required and must be unique")
		}
		metadata, probeErr := probePersistentAgentOwnerMetadata(ctx, h, target, pane, requestedKind)
		if probeErr == nil {
			return metadata, nil
		}
		return inferredPersistentAgentMetadata{}, fmt.Errorf("could not infer Codex/Claude session id: live agent process has no resume session id in its arguments and owner probe failed: %w", probeErr)
	}
	return inferredPersistentAgentMetadata{}, fmt.Errorf("could not infer Codex/Claude/Hermes session id: session is not running a supported live agent")
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

func persistentDescriptorFromMetadata(name, unixUser string, pane paneInspection, metadata inferredPersistentAgentMetadata) WorkloadRecoveryDescriptor {
	desc := persistentDescriptorFromLegacyFields(name, unixUser, metadata.Kind, metadata.SessionID, pane.CWD)
	desc.EvidenceSource = RecoveryEvidenceArgv
	desc.Confidence = RecoveryConfidenceHigh
	if desc.Agent != nil {
		desc.Agent.HermesProfile = metadata.HermesProfile
	}
	return desc
}

func persistentDescriptorForEntry(entry PersistentAgentEntry, ownerHome string) (WorkloadRecoveryDescriptor, error) {
	if entry.RecoveryDescriptor != nil {
		return canonicalPersistentAgentDescriptor(entry.Name, entry.UnixUser, ownerHome, *entry.RecoveryDescriptor)
	}
	return canonicalPersistentAgentDescriptor(entry.Name, entry.UnixUser, ownerHome, persistentDescriptorFromLegacyFields(entry.Name, entry.UnixUser, entry.AgentKind, entry.AgentSessionID, entry.CWD))
}

func persistentSessionBankConflict(entry SessionBankEntry, ownerHome string) error {
	if len(entry.RecoveryPlan) == 0 {
		if _, ok := canonicalAgentResumeCommand(entry.AgentKind, entry.AgentSessionID); ok {
			return &recoveryOwnershipConflict{
				OwnerKind: RecoveryOwnerSessionBank,
				OwnerRef:  sessionBankOwnerRef(entry.UnixUser, entry.Name),
			}
		}
		return nil
	}
	expectedSessionBankRef := sessionBankOwnerRef(entry.UnixUser, entry.Name)
	owners := map[string]bool{}
	for i, raw := range entry.RecoveryPlan {
		desc, err := CanonicalizeWorkloadRecoveryDescriptor(raw, ownerHome)
		if err != nil {
			return fmt.Errorf("session bank recovery plan descriptor %d is unsafe: %w", i, err)
		}
		ownerKey := desc.Owner.Kind + "\x00" + desc.Owner.Ref
		owners[ownerKey] = true
		if len(owners) > 1 {
			return fmt.Errorf("session bank recovery plan has conflicting recovery owners")
		}
		if desc.Owner.Kind == RecoveryOwnerSessionBank && desc.Owner.Ref == expectedSessionBankRef {
			return &recoveryOwnershipConflict{OwnerKind: desc.Owner.Kind, OwnerRef: desc.Owner.Ref}
		}
		if desc.Owner.Kind == RecoveryOwnerExternalManager || desc.Mode == RecoveryModeManaged {
			return &recoveryOwnershipConflict{OwnerKind: desc.Owner.Kind, OwnerRef: desc.Owner.Ref}
		}
	}
	return nil
}

func (h *TmuxHandler) ensurePersistentAgentOwnershipAvailable(name, unixUser, ownerHome string) error {
	if h == nil {
		return nil
	}
	if err := h.ensureExternalRecoveryOwnershipAvailable(name, unixUser); err != nil {
		return err
	}
	if h.bank == nil {
		return nil
	}
	entry, found, err := h.bank.Find(name, unixUser)
	if err != nil {
		return fmt.Errorf("session bank: %w", err)
	}
	if !found {
		return nil
	}
	return persistentSessionBankConflict(entry, ownerHome)
}

func persistentMetadataMatchesDescriptor(metadata inferredPersistentAgentMetadata, desc WorkloadRecoveryDescriptor) bool {
	if desc.Agent == nil {
		return false
	}
	if metadata.Kind != desc.Agent.Kind || metadata.SessionID != desc.Agent.NativeSessionID {
		return false
	}
	if desc.Agent.Kind == RecoveryAgentHermes && metadata.HermesProfile != desc.Agent.HermesProfile {
		return false
	}
	return true
}

func persistentMetadataWrongIdentityMessage(metadata inferredPersistentAgentMetadata, desc WorkloadRecoveryDescriptor) string {
	if desc.Agent == nil {
		return "wrong identity: missing expected agent descriptor"
	}
	if metadata.Kind == "" {
		return fmt.Sprintf("wrong identity: %s process has unknown identity", desc.Agent.Kind)
	}
	if metadata.SessionID == "" {
		return fmt.Sprintf("wrong identity: %s process has unknown identity", metadata.Kind)
	}
	if desc.Agent.Kind == RecoveryAgentHermes && metadata.HermesProfile != desc.Agent.HermesProfile {
		return fmt.Sprintf("wrong identity: hermes profile %q, want %q", metadata.HermesProfile, desc.Agent.HermesProfile)
	}
	return fmt.Sprintf("wrong identity: %s session %q, want %q", metadata.Kind, metadata.SessionID, desc.Agent.NativeSessionID)
}

func persistentProcessKindLive(ctx context.Context, pane paneInspection, desc WorkloadRecoveryDescriptor) (bool, error) {
	if desc.Agent == nil {
		return false, nil
	}
	if desc.Agent.Kind == RecoveryAgentHermes {
		if strings.EqualFold(filepath.Base(pane.Command), "python") || strings.EqualFold(filepath.Base(pane.Command), "python3") {
			return true, nil
		}
		return false, nil
	}
	return agentProcessLive(ctx, pane, desc.Agent.Kind)
}

func (h *TmuxHandler) verifyPersistentDescriptorForLivePane(ctx context.Context, target tmuxTarget, pane paneInspection, desc WorkloadRecoveryDescriptor, inferred bool) error {
	if desc.Agent == nil {
		return fmt.Errorf("persistent agent descriptor is missing agent identity")
	}
	infos, err := readPersistentAgentProcessTable(ctx)
	if err != nil {
		return fmt.Errorf("could not inspect session process tree: %w", err)
	}
	metadata, foundAgent, foundSessionID, inferErr := inferPersistentAgentMetadataInTableForOwner(infos, pane.PID, desc.Agent.Kind, target.ownerHome)
	if inferErr != nil {
		return inferErr
	}
	if foundSessionID {
		if persistentMetadataMatchesDescriptor(metadata, desc) {
			return nil
		}
		return fmt.Errorf("%s", persistentMetadataWrongIdentityMessage(metadata, desc))
	}
	if foundAgent {
		if desc.Agent.Kind == RecoveryAgentHermes {
			return fmt.Errorf("hermes resume id is required and must be unique")
		}
		probed, probeErr := probePersistentAgentOwnerMetadata(ctx, h, target, pane, desc.Agent.Kind)
		if probeErr != nil {
			return fmt.Errorf("could not prove exact %s identity: %w", desc.Agent.Kind, probeErr)
		}
		if persistentMetadataMatchesDescriptor(probed, desc) {
			return nil
		}
		return fmt.Errorf("%s", persistentMetadataWrongIdentityMessage(probed, desc))
	}
	if desc.Agent.Kind == RecoveryAgentHermes {
		return fmt.Errorf("session is not running Hermes")
	}
	if inferred {
		return fmt.Errorf("could not prove exact %s identity", desc.Agent.Kind)
	}
	return fmt.Errorf("session is running %q, not %s", pane.Command, desc.Agent.Kind)
}

func detectPersistentInteraction(tail string) (string, bool) {
	lower := strings.ToLower(tail)
	checks := []struct {
		kind     string
		patterns []string
	}{
		{kind: "update", patterns: []string{"update available", "please update", "run codex update", "new version"}},
		{kind: "hook approval", patterns: []string{"hook approval", "allow this hook", "approve hook", "hook required"}},
		{kind: "trust", patterns: []string{"do you trust", "trust this workspace", "untrusted workspace", "trust the authors"}},
		{kind: "migration", patterns: []string{"migration required", "first-run migration", "first run migration", "first-run", "first run", "migrate"}},
	}
	for _, check := range checks {
		for _, pattern := range check.patterns {
			if strings.Contains(lower, pattern) {
				return check.kind, true
			}
		}
	}
	return "", false
}

func (h *TmuxHandler) capturePersistentPaneTail(ctx context.Context, target tmuxTarget, sessionName string) string {
	if h == nil {
		return ""
	}
	output, err := h.runTmuxOnSocketContext(ctx, target.socket, "capture-pane", "-p", "-J", "-S", fmt.Sprintf("-%d", persistentAgentPaneTailLines), "-t", sessionName)
	if err != nil {
		return ""
	}
	return output
}

type persistentLiveStatus struct {
	action  string
	message string
	noAgent bool
}

func (h *TmuxHandler) inspectPersistentLiveStatus(ctx context.Context, target tmuxTarget, entry PersistentAgentEntry, desc WorkloadRecoveryDescriptor, pane paneInspection) persistentLiveStatus {
	if kind, ok := detectPersistentInteraction(h.capturePersistentPaneTail(ctx, target, entry.Name)); ok {
		return persistentLiveStatus{action: PersistentAgentStateNeedsInteraction, message: "blocked-needs-interaction: " + kind}
	}
	infos, err := readPersistentAgentProcessTable(ctx)
	if err != nil {
		return persistentLiveStatus{action: "error", message: err.Error()}
	}
	metadata, foundAgent, foundSessionID, inferErr := inferPersistentAgentMetadataInTableForOwner(infos, pane.PID, desc.Agent.Kind, target.ownerHome)
	if inferErr != nil {
		return persistentLiveStatus{action: PersistentAgentStateWrongIdentity, message: inferErr.Error()}
	}
	if foundSessionID {
		if persistentMetadataMatchesDescriptor(metadata, desc) {
			return persistentLiveStatus{action: "ok"}
		}
		return persistentLiveStatus{action: PersistentAgentStateWrongIdentity, message: persistentMetadataWrongIdentityMessage(metadata, desc)}
	}
	if foundAgent {
		return persistentLiveStatus{action: PersistentAgentStateWrongIdentity, message: persistentMetadataWrongIdentityMessage(metadata, desc)}
	}
	return persistentLiveStatus{action: "missing", noAgent: true}
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
	if err := decodeOptionalJSONBodyLimited(w, r, &req, persistentAgentEnableMaxRequestBytes); err != nil {
		if errors.Is(err, errRecoveryRequestBodyTooLarge) {
			core.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", fmt.Sprintf("Persistent agent request body exceeds %d bytes", persistentAgentEnableMaxRequestBytes))
			return
		}
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
	ownerHome, ownerHomeErr := trustedSessionBankOwnerHome(target)
	if ownerHomeErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", ownerHomeErr.Error())
		return
	}
	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()
	var descriptor WorkloadRecoveryDescriptor
	descriptorReady := false
	if req.RecoveryDescriptor != nil {
		var err error
		descriptor, err = canonicalPersistentAgentDescriptor(newName, unixUser, ownerHome, *req.RecoveryDescriptor)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		descriptorReady = true
	}
	if err := h.ensurePersistentAgentOwnershipAvailable(sessionName, unixUser, ownerHome); err != nil {
		writeRecoveryOwnershipError(w, "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", "PERSISTENT_AGENT_ERROR", err)
		return
	}
	if newName != sessionName {
		if h.persistent != nil {
			sourcePersistent, err := h.persistent.IsPersistent(sessionName, unixUser)
			if err != nil {
				core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_ERROR", err.Error())
				return
			}
			if sourcePersistent {
				core.WriteError(w, http.StatusConflict, "PERSISTENT_AGENT_SOURCE_EXISTS", "Persistent source sessions must be renamed through the session rename endpoint")
				return
			}
		}
		if err := h.ensurePersistentAgentOwnershipAvailable(newName, unixUser, ownerHome); err != nil {
			writeRecoveryOwnershipError(w, "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", "PERSISTENT_AGENT_ERROR", err)
			return
		}
		if h.persistent != nil {
			if err := h.persistent.EnsureTargetAvailable(newName, unixUser); err != nil {
				core.WriteError(w, http.StatusConflict, "PERSISTENT_AGENT_TARGET_EXISTS", err.Error())
				return
			}
		}
	}
	pane, err := h.inspectSessionPane(r.Context(), target.socket, sessionName)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "TMUX_ERROR", err.Error())
		return
	}
	inferred := false
	if !descriptorReady && (strings.TrimSpace(req.AgentKind) == "" || strings.TrimSpace(req.AgentSessionID) == "") {
		metadata, err := h.inferPersistentAgentMetadata(r.Context(), target, pane, req.AgentKind)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", capitalizeFirst(err.Error()))
			return
		}
		descriptor = persistentDescriptorFromMetadata(newName, unixUser, pane, metadata)
		descriptorReady = true
		inferred = true
	}
	if !descriptorReady {
		descriptor = persistentDescriptorFromLegacyFields(newName, unixUser, req.AgentKind, req.AgentSessionID, pane.CWD)
		descriptorReady = true
	}
	descriptor, err = canonicalPersistentAgentDescriptor(newName, unixUser, ownerHome, descriptor)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(req.CWD) != "" {
		requestCWD, cwdErr := canonicalOwnerHomePath(req.CWD, "", ownerHome)
		if cwdErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Request CWD is unsafe: "+cwdErr.Error())
			return
		}
		if requestCWD != descriptor.Topology.PaneCurrentPath {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("Request CWD %q does not match recovery descriptor topology cwd %q", requestCWD, descriptor.Topology.PaneCurrentPath))
			return
		}
	}
	if !inferred {
		err = h.verifyPersistentDescriptorForLivePane(r.Context(), target, pane, descriptor, false)
	}
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", capitalizeFirst(err.Error()))
		return
	}
	req.CWD = descriptor.Topology.PaneCurrentPath
	req.AgentKind = descriptor.Agent.Kind
	req.AgentSessionID = descriptor.Agent.NativeSessionID
	req.RecoveryDescriptor = &descriptor
	entry, err := h.persistent.Upsert(newName, unixUser, req, ownerHome)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "PERSISTENT_AGENT_ERROR", err.Error())
		return
	}
	// Hand the promise to systemd. Identity was resolved once, above, because we
	// are adopting a pane we did not create; from here nothing in this process
	// watches it (ADR-0014).
	if h.agentUnits != nil {
		unitConfig := agentUnitConfig{
			Session:        entry.Name,
			UnixUser:       unixUser,
			AgentKind:      entry.AgentKind,
			AgentSessionID: entry.AgentSessionID,
			AgentBin:       agentBinaryForKind(entry.AgentKind, ownerHome),
			HermesProfile:  persistentAgentHermesProfile(entry),
			TmuxBin:        absoluteToolPath(core.TmuxBin()),
			TmuxSocket:     target.socket,
			Workdir:        entry.CWD,
			KeeperUnit:     strings.TrimSpace(os.Getenv("CHROTE_TMUX_KEEPER_UNIT")),
		}
		if err := h.agentUnits.Enable(r.Context(), unitConfig); err != nil {
			// A registry entry without a running unit is a lock that does not
			// lock. Roll it back rather than report a promise nothing keeps.
			if _, forgetErr := h.persistent.Forget(entry.Name, unixUser); forgetErr != nil {
				core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_ERROR",
					fmt.Sprintf("%s; rollback also failed, a stale entry remains for %q: %s", err.Error(), entry.Name, forgetErr))
				return
			}
			core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_UNIT_ERROR", err.Error())
			return
		}
	}
	if newName != sessionName {
		if _, err := h.runTmuxOnSocket(target.socket, "rename-session", "-t", sessionName, newName); err != nil {
			// The registry entry was written before the rename; a failed
			// rollback must be reported, or a stale entry survives silently
			// for a tmux name that was never committed.
			if _, forgetErr := h.persistent.Forget(newName, unixUser); forgetErr != nil {
				core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR",
					fmt.Sprintf("%s; rollback also failed, a stale persistent entry remains for %q: %s", err.Error(), newName, forgetErr))
				return
			}
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
	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()
	// Stop supervising first: if this fails, the session stays locked rather
	// than becoming a unit CHROTE has forgotten but systemd still restarts.
	// Unlocking withdraws the promise and deliberately leaves the agent running
	// (ADR-0014 decision 8).
	if h.agentUnits != nil {
		if err := h.agentUnits.Disable(r.Context(), sessionName, unixUser); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_UNIT_ERROR", err.Error())
			return
		}
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
