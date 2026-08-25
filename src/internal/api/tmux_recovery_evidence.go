package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	osuser "os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/chrote/server/internal/core"
)

const (
	tmuxSourceComplete    = "complete"
	tmuxSourceFailed      = "failed"
	recoveryEvidenceLive  = "live"
	recoveryEvidenceStale = "stale"
	recoveryEvidenceGone  = "offline"
)

// TmuxSourceEvidence qualifies one configured tmux inventory. A generation is
// present only when the source was read authoritatively.
type TmuxSourceEvidence struct {
	SourceID       string `json:"sourceId"`
	UnixUser       string `json:"unixUser,omitempty"`
	Status         string `json:"status"`
	ObservedAt     string `json:"observedAt"`
	Generation     string `json:"generation,omitempty"`
	ServerIdentity string `json:"serverIdentity,omitempty"`
	ErrorCode      string `json:"errorCode,omitempty"`
	Error          string `json:"error,omitempty"`
}

// NativeSessionEvidence is an already-recorded native harness identity. It is
// evidence for an agent to rank, never a CHROTE resume decision.
type NativeSessionEvidence struct {
	Provider        string `json:"provider"`
	NativeSessionID string `json:"nativeSessionId"`
	EvidenceSource  string `json:"evidenceSource"`
	ObservedAt      string `json:"observedAt,omitempty"`
}

// RecoverySessionEvidence projects bounded current/offline state without
// exposing transcripts, environments, or inferred commands.
type RecoverySessionEvidence struct {
	SourceID      string                  `json:"sourceId"`
	UnixUser      string                  `json:"unixUser,omitempty"`
	Name          string                  `json:"name"`
	State         string                  `json:"state"`
	TmuxSessionID string                  `json:"tmuxSessionId,omitempty"`
	CWD           string                  `json:"cwd,omitempty"`
	FirstSeen     string                  `json:"firstSeen,omitempty"`
	LastSeen      string                  `json:"lastSeen,omitempty"`
	Native        []NativeSessionEvidence `json:"native,omitempty"`
}

type tmuxGenerationSession struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	UnixUser string `json:"unixUser"`
	Windows  int    `json:"windows"`
	Attached bool   `json:"attached"`
	CWD      string `json:"cwd"`
}

func tmuxSourceID(unixUser string) string {
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		return "tmux:default"
	}
	return "tmux:" + unixUser
}

func tmuxSourceGeneration(unixUser string, sessions []core.Session, sourceIdentity ...string) string {
	rows := make([]tmuxGenerationSession, 0, len(sessions))
	for _, session := range sessions {
		rows = append(rows, tmuxGenerationSession{
			ID:       strings.TrimSpace(session.ID),
			Name:     strings.TrimSpace(session.Name),
			UnixUser: strings.TrimSpace(session.UnixUser),
			Windows:  session.Windows,
			Attached: session.Attached,
			CWD:      strings.TrimSpace(session.CWD),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UnixUser != rows[j].UnixUser {
			return rows[i].UnixUser < rows[j].UnixUser
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].ID < rows[j].ID
	})
	raw, _ := json.Marshal(struct {
		UnixUser       string                  `json:"unixUser"`
		SourceIdentity string                  `json:"sourceIdentity"`
		Sessions       []tmuxGenerationSession `json:"sessions"`
	}{UnixUser: strings.TrimSpace(unixUser), SourceIdentity: strings.Join(sourceIdentity, ""), Sessions: rows})
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

const (
	tmuxInventoryMaxBytes = 1 << 20
	tmuxInventoryMaxRows  = 4096
	tmuxInventoryMaxCWD   = 4096
)

var readTmuxServerProcessIdentity = readLinuxTmuxServerProcessIdentity

func readLinuxTmuxServerProcessIdentity(pidText, unixUser string) (string, error) {
	if !tmuxPIDPattern.MatchString(pidText) {
		return "", fmt.Errorf("tmux server pid is invalid")
	}
	account, err := osuser.Lookup(strings.TrimSpace(unixUser))
	if err != nil {
		return "", fmt.Errorf("tmux server Unix user is unavailable")
	}
	wantUID, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return "", fmt.Errorf("tmux server Unix user id is invalid")
	}
	procDir := filepath.Join("/proc", pidText)
	info, err := os.Stat(procDir)
	if err != nil {
		return "", fmt.Errorf("tmux server process is unavailable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || uint64(stat.Uid) != wantUID {
		return "", fmt.Errorf("tmux server process owner does not match its configured Unix user")
	}
	statRaw, err := os.ReadFile(filepath.Join(procDir, "stat"))
	if err != nil || len(statRaw) > 4096 {
		return "", fmt.Errorf("tmux server process identity is unavailable")
	}
	closing := strings.LastIndex(string(statRaw), ") ")
	if closing < 0 {
		return "", fmt.Errorf("tmux server process identity is malformed")
	}
	fields := strings.Fields(string(statRaw)[closing+2:])
	if len(fields) <= 19 || !tmuxPIDPattern.MatchString(fields[19]) {
		return "", fmt.Errorf("tmux server process start identity is malformed")
	}
	bootRaw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	bootID := strings.TrimSpace(string(bootRaw))
	if err != nil || len(bootRaw) > 128 || bootID == "" || strings.ContainsAny(bootID, "\x00\r\n	 ;@") {
		return "", fmt.Errorf("kernel boot identity is unavailable")
	}
	return fmt.Sprintf("boot=%s;start=%s;uid=%d", bootID, fields[19], wantUID), nil
}

func parseAuthoritativeSessionsOutput(output, unixUser, expectedSocket string) ([]core.Session, string, error) {
	if len(output) > tmuxInventoryMaxBytes {
		return nil, "", fmt.Errorf("tmux inventory protocol exceeds the bounded output limit")
	}
	trimmed := strings.TrimRight(output, "\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return nil, "", fmt.Errorf("tmux inventory protocol returned empty output")
	}
	sessions := []core.Session{}
	seenIDs := map[string]bool{}
	seenNames := map[string]bool{}
	serverIdentity := ""
	lines := strings.Split(trimmed, "\n")
	if len(lines) > tmuxInventoryMaxRows {
		return nil, "", fmt.Errorf("tmux inventory protocol exceeds the row limit")
	}
	for lineIndex, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		parts := strings.Split(line, "	")
		if len(parts) != 8 {
			return nil, "", fmt.Errorf("tmux inventory protocol row %d has %d fields, want 8", lineIndex+1, len(parts))
		}
		serverPID := strings.TrimSpace(parts[0])
		serverStarted := strings.TrimSpace(parts[1])
		socketPath := filepath.Clean(strings.TrimSpace(parts[2]))
		sessionID := strings.TrimSpace(parts[3])
		name := strings.TrimSpace(parts[4])
		windowsText := strings.TrimSpace(parts[5])
		attachedText := strings.TrimSpace(parts[6])
		cwd := strings.TrimSpace(parts[7])
		if !tmuxPIDPattern.MatchString(serverPID) || !tmuxPIDPattern.MatchString(serverStarted) || !filepath.IsAbs(socketPath) || socketPath != effectiveTmuxSocket(expectedSocket) {
			return nil, "", fmt.Errorf("tmux inventory protocol row %d has invalid server identity", lineIndex+1)
		}
		rowServerBase := serverPID + "@" + serverStarted + "@" + socketPath
		if serverIdentity == "" {
			processIdentity, err := readTmuxServerProcessIdentity(serverPID, unixUser)
			if err != nil {
				return nil, "", fmt.Errorf("tmux inventory protocol server identity is not authoritative: %w", err)
			}
			serverIdentity = rowServerBase + "@" + processIdentity
		} else if !strings.HasPrefix(serverIdentity, rowServerBase+"@") {
			return nil, "", fmt.Errorf("tmux inventory protocol changed server identity between rows")
		}
		if !tmuxSessionIDPattern.MatchString(sessionID) || name == "" || len(name) > 256 || strings.ContainsAny(name, "\x00\r\n	") {
			return nil, "", fmt.Errorf("tmux inventory protocol row %d has invalid session identity", lineIndex+1)
		}
		windows, err := strconv.Atoi(windowsText)
		if err != nil || windows <= 0 {
			return nil, "", fmt.Errorf("tmux inventory protocol row %d has invalid window count", lineIndex+1)
		}
		if attachedText != "0" && attachedText != "1" {
			return nil, "", fmt.Errorf("tmux inventory protocol row %d has invalid attached state", lineIndex+1)
		}
		if cwd != "" && (!filepath.IsAbs(cwd) || len(cwd) > tmuxInventoryMaxCWD || strings.ContainsRune(cwd, '\x00')) {
			return nil, "", fmt.Errorf("tmux inventory protocol row %d has invalid cwd", lineIndex+1)
		}
		if seenIDs[sessionID] || seenNames[name] {
			return nil, "", fmt.Errorf("tmux inventory protocol row %d duplicates a session identity", lineIndex+1)
		}
		seenIDs[sessionID] = true
		seenNames[name] = true
		if isReservedInternalSessionName(name) {
			continue
		}
		sessions = append(sessions, core.Session{
			ID:       sessionID,
			Name:     name,
			Windows:  windows,
			Attached: attachedText == "1",
			Group:    core.CategorizeSession(name),
			UnixUser: unixUser,
			CWD:      cwd,
		})
	}
	return sessions, serverIdentity, nil
}

func completeTmuxSource(unixUser, observedAt, serverIdentity string, sessions []core.Session) TmuxSourceEvidence {
	return TmuxSourceEvidence{
		SourceID:       tmuxSourceID(unixUser),
		UnixUser:       strings.TrimSpace(unixUser),
		Status:         tmuxSourceComplete,
		ObservedAt:     observedAt,
		Generation:     tmuxSourceGeneration(unixUser, sessions, serverIdentity),
		ServerIdentity: serverIdentity,
	}
}

func boundedTmuxEvidenceError(message string) string {
	message = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.TrimSpace(message))
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 256 {
		message = message[:256]
	}
	return message
}

func failedTmuxSource(unixUser, observedAt, message string) TmuxSourceEvidence {
	return TmuxSourceEvidence{
		SourceID:   tmuxSourceID(unixUser),
		UnixUser:   strings.TrimSpace(unixUser),
		Status:     tmuxSourceFailed,
		ObservedAt: observedAt,
		ErrorCode:  "TMUX_SOURCE_UNAVAILABLE",
		Error:      boundedTmuxEvidenceError(message),
	}
}

func nativeEvidenceFromBank(entry SessionBankEntry) []NativeSessionEvidence {
	result := []NativeSessionEvidence{}
	seen := map[string]bool{}
	appendEvidence := func(provider, id, source string) {
		provider = strings.ToLower(strings.TrimSpace(provider))
		id = strings.TrimSpace(id)
		source = strings.TrimSpace(source)
		if provider == "" || id == "" {
			return
		}
		key := provider + "\x00" + id + "\x00" + source
		if seen[key] {
			return
		}
		seen[key] = true
		result = append(result, NativeSessionEvidence{Provider: provider, NativeSessionID: id, EvidenceSource: source, ObservedAt: entry.NativeEvidenceObservedAt})
	}
	if entry.AgentSessionID != "" {
		appendEvidence(entry.AgentKind, entry.AgentSessionID, "session-bank")
	}
	for _, descriptor := range entry.RecoveryPlan {
		if descriptor.Agent == nil {
			continue
		}
		appendEvidence(descriptor.Agent.Kind, descriptor.Agent.NativeSessionID, descriptor.EvidenceSource)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		if result[i].NativeSessionID != result[j].NativeSessionID {
			return result[i].NativeSessionID < result[j].NativeSessionID
		}
		return result[i].EvidenceSource < result[j].EvidenceSource
	})
	return result
}

func projectRecoveryEvidence(sources []TmuxSourceEvidence, live []core.Session, banked []SessionBankEntry) []RecoverySessionEvidence {
	sourceStatus := map[string]string{}
	for _, source := range sources {
		sourceStatus[strings.TrimSpace(source.UnixUser)] = source.Status
	}
	liveByKey := map[string]core.Session{}
	for _, session := range live {
		liveByKey[sessionBankKey(session.Name, session.UnixUser)] = session
	}
	result := make([]RecoverySessionEvidence, 0, len(banked))
	for _, entry := range banked {
		state := recoveryEvidenceStale
		if current, ok := liveByKey[sessionBankKey(entry.Name, entry.UnixUser)]; ok {
			state = recoveryEvidenceLive
			entry.ID = current.ID
			if current.CWD != "" {
				entry.CWD = current.CWD
			}
		} else if sourceStatus[strings.TrimSpace(entry.UnixUser)] == tmuxSourceComplete {
			state = recoveryEvidenceGone
		}
		result = append(result, RecoverySessionEvidence{
			SourceID:      tmuxSourceID(entry.UnixUser),
			UnixUser:      entry.UnixUser,
			Name:          entry.Name,
			State:         state,
			TmuxSessionID: entry.ID,
			CWD:           entry.CWD,
			FirstSeen:     entry.FirstSeen,
			LastSeen:      entry.LastSeen,
			Native:        nativeEvidenceFromBank(entry),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].State != result[j].State {
			return result[i].State < result[j].State
		}
		if result[i].UnixUser != result[j].UnixUser {
			return result[i].UnixUser < result[j].UnixUser
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func (s *sessionBankStore) SnapshotForUsers(liveSessions []core.Session, authoritativeUsers map[string]bool) ([]SessionBankEntry, error) {
	if s == nil {
		return []SessionBankEntry{}, nil
	}
	return s.snapshotForUsers(liveSessions, authoritativeUsers)
}
