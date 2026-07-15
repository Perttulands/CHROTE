package api

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	defaultManagedRecoveryStatusFile = "/srv/data/chrote/tmux-recovery/managed-status.json"
	managedRecoveryStatusMaxBytes    = 256 << 10
	managedRecoveryStatusMaxEntries  = 128
)

var (
	managedRecoveryUnitRegex       = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+\.service$`)
	managedRecoveryUnixUserRegex   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	managedRecoveryHealthStateSet  = map[string]bool{"active": true, "inactive": true, "failed": true, "activating": true, "deactivating": true, "reloading": true, "maintenance": true, "unknown": true}
	managedRecoveryStatusSourceSet = map[string]bool{"restore": true, "snapshot": true}
)

// ManagedRecoveryStatusEntry is read-only status written by the external
// owner-side restore flow. It is intentionally not a recovery descriptor.
type ManagedRecoveryStatusEntry struct {
	Name        string                      `json:"name"`
	SessionName string                      `json:"sessionName"`
	UnixUser    string                      `json:"unixUser,omitempty"`
	Owner       WorkloadRecoveryOwner       `json:"owner"`
	ManagerKind string                      `json:"managerKind"`
	ManagerRef  string                      `json:"managerRef"`
	Status      ManagedRecoveryHealthStatus `json:"status"`
	StorageKind string                      `json:"storageKind"`
	SourceKind  string                      `json:"sourceKind"`
}

type ManagedRecoveryHealthStatus struct {
	OK          bool   `json:"ok"`
	ActiveState string `json:"activeState"`
	CheckedAt   string `json:"checkedAt"`
}

type managedRecoveryStatusStore struct {
	path string
	mu   sync.Mutex
}

func defaultManagedRecoveryStatusPath() string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH")); override != "" {
		return override
	}
	return defaultManagedRecoveryStatusFile
}

func newManagedRecoveryStatusStore(path string) *managedRecoveryStatusStore {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultManagedRecoveryStatusFile
	}
	return &managedRecoveryStatusStore{path: path}
}

func (s *managedRecoveryStatusStore) Read() ([]ManagedRecoveryStatusEntry, error) {
	if s == nil {
		return []ManagedRecoveryStatusEntry{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

func (s *managedRecoveryStatusStore) readLocked() ([]ManagedRecoveryStatusEntry, error) {
	lstatInfo, err := os.Lstat(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ManagedRecoveryStatusEntry{}, nil
		}
		return nil, err
	}
	if lstatInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("managed status registry must not be a symlink")
	}
	if !lstatInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("managed status registry must be a regular file")
	}
	if lstatInfo.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("managed status registry permissions must be 0600 or stricter")
	}
	handle, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ManagedRecoveryStatusEntry{}, nil
		}
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(lstatInfo, info) {
		return nil, fmt.Errorf("managed status registry changed while opening")
	}
	if info.Size() > managedRecoveryStatusMaxBytes {
		return nil, fmt.Errorf("managed status registry exceeds %d bytes", managedRecoveryStatusMaxBytes)
	}
	limited := io.LimitReader(handle, managedRecoveryStatusMaxBytes+1)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	var entries []ManagedRecoveryStatusEntry
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("managed status registry is malformed: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, fmt.Errorf("managed status registry contains trailing JSON")
	}
	if len(entries) > managedRecoveryStatusMaxEntries {
		return nil, fmt.Errorf("managed status registry has %d entries, max %d", len(entries), managedRecoveryStatusMaxEntries)
	}
	seen := map[string]bool{}
	for i := range entries {
		entry, err := normalizeManagedRecoveryStatusEntry(entries[i], i)
		if err != nil {
			return nil, err
		}
		key := sessionBankKey(entry.Name, entry.UnixUser)
		if seen[key] {
			return nil, fmt.Errorf("duplicate managed status entry for %s", managedRecoveryDisplayKey(entry.Name, entry.UnixUser))
		}
		seen[key] = true
		entries[i] = entry
	}
	return entries, nil
}

func normalizeManagedRecoveryStatusEntry(entry ManagedRecoveryStatusEntry, index int) (ManagedRecoveryStatusEntry, error) {
	name := strings.TrimSpace(entry.Name)
	sessionName := strings.TrimSpace(entry.SessionName)
	if name == "" {
		name = sessionName
	}
	if sessionName == "" {
		sessionName = name
	}
	if name == "" {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d name is required", index)
	}
	if sessionName != name {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d name/sessionName mismatch", index)
	}
	if ok, msg := core.ValidateSessionName(name, "managed status session name"); !ok {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d: %s", index, msg)
	}
	entry.Name = name
	entry.SessionName = name
	entry.UnixUser = strings.TrimSpace(entry.UnixUser)
	if !managedRecoveryUnixUserRegex.MatchString(entry.UnixUser) {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d unixUser is invalid", index)
	}
	if entry.Owner.Kind != RecoveryOwnerExternalManager || entry.Owner.MayRestart {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d owner must be read-only external_manager", index)
	}
	entry.Owner.Ref = strings.TrimSpace(entry.Owner.Ref)
	if !recoverySafeReferenceRegex.MatchString(entry.Owner.Ref) {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d owner ref is invalid", index)
	}
	entry.ManagerKind = strings.TrimSpace(entry.ManagerKind)
	if entry.ManagerKind != "systemd-user" {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d managerKind must be systemd-user", index)
	}
	entry.ManagerRef = strings.TrimSpace(entry.ManagerRef)
	if !managedRecoveryUnitRegex.MatchString(entry.ManagerRef) {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d managerRef is invalid", index)
	}
	if entry.Owner.Ref != "systemd:user/"+entry.ManagerRef {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d owner ref does not match managerRef", index)
	}
	entry.Status.ActiveState = strings.TrimSpace(entry.Status.ActiveState)
	if !managedRecoveryHealthStateSet[entry.Status.ActiveState] {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d activeState is invalid", index)
	}
	if entry.Status.OK != (entry.Status.ActiveState == "active") {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d status ok contradicts activeState", index)
	}
	entry.Status.CheckedAt = strings.TrimSpace(entry.Status.CheckedAt)
	if _, err := time.Parse(time.RFC3339, entry.Status.CheckedAt); err != nil {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d checkedAt is invalid", index)
	}
	entry.StorageKind = strings.TrimSpace(entry.StorageKind)
	if entry.StorageKind != "managed-status" {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d storageKind must be managed-status", index)
	}
	entry.SourceKind = strings.TrimSpace(entry.SourceKind)
	if !managedRecoveryStatusSourceSet[entry.SourceKind] {
		return ManagedRecoveryStatusEntry{}, fmt.Errorf("managed status entry %d sourceKind is invalid", index)
	}
	return entry, nil
}

func managedRecoveryDisplayKey(name, unixUser string) string {
	if strings.TrimSpace(unixUser) == "" {
		return strings.TrimSpace(name)
	}
	return strings.TrimSpace(unixUser) + "/" + strings.TrimSpace(name)
}

func (s *managedRecoveryStatusStore) Contains(name, unixUser string) (bool, error) {
	entries, err := s.Read()
	if err != nil {
		return false, err
	}
	key := sessionBankKey(name, unixUser)
	for _, entry := range entries {
		if sessionBankKey(entry.Name, entry.UnixUser) == key {
			return true, nil
		}
	}
	return false, nil
}

func (h *TmuxHandler) ensureManagedRecoveryOwnershipAvailable(name, unixUser string) error {
	if h == nil || h.managed == nil {
		return nil
	}
	found, err := h.managed.Contains(name, unixUser)
	if err != nil {
		return fmt.Errorf("managed status registry: %w", err)
	}
	if found {
		return fmt.Errorf("external manager owns recovery for %s", managedRecoveryDisplayKey(name, unixUser))
	}
	return nil
}

func (s *managedRecoveryStatusStore) FilterBanked(entries []SessionBankEntry) ([]SessionBankEntry, error) {
	managed, err := s.Read()
	if err != nil {
		return entries, err
	}
	return filterBankedForManagedStatus(entries, managed), nil
}

func filterBankedForManagedStatus(entries []SessionBankEntry, managed []ManagedRecoveryStatusEntry) []SessionBankEntry {
	blocked := map[string]bool{}
	for _, entry := range managed {
		blocked[sessionBankKey(entry.Name, entry.UnixUser)] = true
	}
	filtered := make([]SessionBankEntry, 0, len(entries))
	for _, entry := range entries {
		if blocked[sessionBankKey(entry.Name, entry.UnixUser)] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func filterLiveSessionsForManagedStatus(sessions []core.Session, managed []ManagedRecoveryStatusEntry) []core.Session {
	if len(managed) == 0 {
		return sessions
	}
	blocked := map[string]bool{}
	for _, entry := range managed {
		blocked[sessionBankKey(entry.Name, entry.UnixUser)] = true
	}
	filtered := make([]core.Session, 0, len(sessions))
	for _, session := range sessions {
		if blocked[sessionBankKey(session.Name, session.UnixUser)] {
			continue
		}
		filtered = append(filtered, session)
	}
	return filtered
}
