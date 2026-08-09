package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/chrote/server/internal/core"
)

const (
	RecoveryModeTopology   = "topology"
	RecoveryModeAgent      = "agent"
	RecoveryModeCommand    = "command"
	RecoveryModeManaged    = "managed"
	RecoveryModeUnresolved = "unresolved"

	RecoveryOwnerSessionBank     = "session_bank"
	RecoveryOwnerPersistentAgent = "persistent_agent"
	RecoveryOwnerExternalManager = "external_manager"

	RecoveryAgentCodex  = "codex"
	RecoveryAgentClaude = "claude"
	RecoveryAgentHermes = "hermes"

	RecoveryWorkloadPythonHTTPServer = "python-http-server"
	RecoveryWorkloadShell            = "shell"
	RecoveryWorkloadManaged          = "managed"
	RecoveryWorkloadUnknown          = "unknown"

	RecoveryCommandPythonHTTPServer = "python-http-server"

	RecoveryEvidenceArgv       = "argv"
	RecoveryEvidenceTranscript = "transcript"
	RecoveryEvidenceStateDB    = "state_db"
	RecoveryEvidenceTopology   = "topology"
	RecoveryEvidenceManager    = "manager"
	RecoveryEvidenceProcess    = "process"

	RecoveryConfidenceHigh   = "high"
	RecoveryConfidenceMedium = "medium"
	RecoveryConfidenceLow    = "low"

	RecoveryUnresolvedUnknownProcess      = "unknown_process"
	RecoveryUnresolvedAmbiguousCandidates = "ambiguous_candidates"
	RecoveryUnresolvedUnsafeEvidence      = "unsafe_evidence"
	RecoveryUnresolvedUnsupportedWorkload = "unsupported_workload"
	RecoveryUnresolvedMissingEvidence     = "missing_evidence"
	RecoveryUnresolvedConflictingOwners   = "conflicting_owners"
	RecoveryUnresolvedConflictingEvidence = "conflicting_evidence"

	recoveryMaxWindowLayoutLength = 4096
)

// WorkloadRecoveryDescriptor captures typed restart evidence for one tmux pane.
// It intentionally stores typed workload identity, not a raw argv or shell
// command that could later override canonical recovery behavior.
type WorkloadRecoveryDescriptor struct {
	Mode             string                   `json:"mode"`
	Owner            WorkloadRecoveryOwner    `json:"owner"`
	Topology         WorkloadRecoveryTopology `json:"topology"`
	WorkloadKind     string                   `json:"workloadKind"`
	Agent            *WorkloadRecoveryAgent   `json:"agent,omitempty"`
	Command          *WorkloadRecoveryCommand `json:"command,omitempty"`
	EvidenceSource   string                   `json:"evidenceSource"`
	Confidence       string                   `json:"confidence"`
	UnresolvedReason string                   `json:"unresolvedReason,omitempty"`
}

type WorkloadRecoveryOwner struct {
	Kind       string `json:"kind"`
	Ref        string `json:"ref"`
	MayRestart bool   `json:"mayRestart"`
}

type WorkloadRecoveryTopology struct {
	SessionName     string `json:"sessionName,omitempty"`
	SessionID       string `json:"sessionId,omitempty"`
	WindowIndex     int    `json:"windowIndex"`
	WindowName      string `json:"windowName,omitempty"`
	WindowLayout    string `json:"windowLayout,omitempty"`
	PaneIndex       int    `json:"paneIndex"`
	PaneID          string `json:"paneId,omitempty"`
	PaneCurrentPath string `json:"paneCurrentPath,omitempty"`
}

type WorkloadRecoveryAgent struct {
	Kind            string `json:"kind"`
	NativeSessionID string `json:"nativeSessionId"`
	HermesProfile   string `json:"hermesProfile,omitempty"`
}

type WorkloadRecoveryCommand struct {
	Kind             string                           `json:"kind"`
	PythonHTTPServer *PythonHTTPServerRecoveryCommand `json:"pythonHTTPServer,omitempty"`
}

type PythonHTTPServerRecoveryCommand struct {
	Bind      string `json:"bind"`
	Port      int    `json:"port"`
	Directory string `json:"directory"`
}

var (
	recoveryUUIDRegex           = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	recoveryNativeIDRegex       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	recoveryHermesProfileRegex  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	recoverySafeReferenceRegex  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,239}$`)
	recoverySafeTmuxIDRegex     = regexp.MustCompile(`^[A-Za-z0-9%$._:-]{0,80}$`)
	recoveryShellSafeTokenRegex = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)
)

func CanonicalizeWorkloadRecoveryDescriptor(desc WorkloadRecoveryDescriptor, ownerHome string) (WorkloadRecoveryDescriptor, error) {
	ownerHome = filepath.Clean(strings.TrimSpace(ownerHome))
	if !filepath.IsAbs(ownerHome) {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("owner home must be absolute")
	}

	desc.Mode = strings.ToLower(strings.TrimSpace(desc.Mode))
	if !allowedRecoveryMode(desc.Mode) {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("unsupported recovery mode %q", desc.Mode)
	}
	owner, err := canonicalRecoveryOwner(desc.Owner)
	if err != nil {
		return WorkloadRecoveryDescriptor{}, err
	}
	desc.Owner = owner
	if err := validateRecoveryModeOwner(desc.Mode, desc.Owner); err != nil {
		return WorkloadRecoveryDescriptor{}, err
	}

	topology, err := canonicalRecoveryTopology(desc.Topology)
	if err != nil {
		return WorkloadRecoveryDescriptor{}, err
	}
	desc.Topology = topology

	desc.EvidenceSource = strings.ToLower(strings.TrimSpace(desc.EvidenceSource))
	if !allowedRecoveryEvidenceSource(desc.EvidenceSource) {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("unsupported recovery evidence source %q", desc.EvidenceSource)
	}
	desc.Confidence = strings.ToLower(strings.TrimSpace(desc.Confidence))
	if !allowedRecoveryConfidence(desc.Confidence) {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("unsupported recovery confidence %q", desc.Confidence)
	}

	switch desc.Mode {
	case RecoveryModeAgent:
		agent, err := canonicalRecoveryAgent(desc.Agent)
		if err != nil {
			return WorkloadRecoveryDescriptor{}, err
		}
		desc.Agent = &agent
		desc.Command = nil
		desc.UnresolvedReason = ""
		desc.WorkloadKind = agent.Kind
	case RecoveryModeCommand:
		command, err := canonicalRecoveryCommand(desc.Command, desc.Topology, ownerHome)
		if err != nil {
			return WorkloadRecoveryDescriptor{}, err
		}
		desc.Agent = nil
		desc.Command = &command
		desc.UnresolvedReason = ""
		desc.WorkloadKind = command.Kind
	case RecoveryModeTopology:
		desc.Agent = nil
		desc.Command = nil
		desc.UnresolvedReason = ""
		desc.WorkloadKind = strings.ToLower(strings.TrimSpace(desc.WorkloadKind))
		if desc.WorkloadKind == "" {
			desc.WorkloadKind = RecoveryWorkloadShell
		}
		if desc.WorkloadKind != RecoveryWorkloadShell {
			return WorkloadRecoveryDescriptor{}, fmt.Errorf("topology descriptors must use shell workload kind")
		}
	case RecoveryModeManaged:
		if desc.Owner.Kind != RecoveryOwnerExternalManager || desc.Owner.MayRestart {
			return WorkloadRecoveryDescriptor{}, fmt.Errorf("managed descriptors require a non-restarting external owner")
		}
		desc.Agent = nil
		desc.Command = nil
		desc.UnresolvedReason = ""
		desc.WorkloadKind = RecoveryWorkloadManaged
	case RecoveryModeUnresolved:
		desc.Agent = nil
		desc.Command = nil
		desc.WorkloadKind = RecoveryWorkloadUnknown
		desc.UnresolvedReason = strings.ToLower(strings.TrimSpace(desc.UnresolvedReason))
		if !allowedRecoveryUnresolvedReason(desc.UnresolvedReason) {
			return WorkloadRecoveryDescriptor{}, fmt.Errorf("unsupported unresolved reason %q", desc.UnresolvedReason)
		}
	}
	return desc, nil
}

func SelectWorkloadRecoveryDescriptor(candidates []WorkloadRecoveryDescriptor, ownerHome string) (WorkloadRecoveryDescriptor, error) {
	if len(candidates) == 0 {
		return WorkloadRecoveryDescriptor{}, fmt.Errorf("no recovery candidates")
	}
	canonical := make([]WorkloadRecoveryDescriptor, 0, len(candidates))
	for _, candidate := range candidates {
		desc, err := CanonicalizeWorkloadRecoveryDescriptor(candidate, ownerHome)
		if err != nil {
			return WorkloadRecoveryDescriptor{}, err
		}
		canonical = append(canonical, desc)
	}
	if len(canonical) == 1 {
		return canonical[0], nil
	}
	firstOwner := canonical[0].Owner
	for _, candidate := range canonical[1:] {
		if candidate.Owner.Kind != firstOwner.Kind || candidate.Owner.Ref != firstOwner.Ref || candidate.Owner.MayRestart != firstOwner.MayRestart {
			return WorkloadRecoveryDescriptor{}, fmt.Errorf("conflicting recovery owners")
		}
	}
	return WorkloadRecoveryDescriptor{}, fmt.Errorf("ambiguous recovery candidates")
}

func validateRecoveryModeOwner(mode string, owner WorkloadRecoveryOwner) error {
	switch mode {
	case RecoveryModeAgent:
		if owner.Kind != RecoveryOwnerSessionBank && owner.Kind != RecoveryOwnerPersistentAgent {
			return fmt.Errorf("agent descriptors require a session bank or legacy persistent agent owner")
		}
		if !owner.MayRestart {
			return fmt.Errorf("agent descriptors require restart permission")
		}
	case RecoveryModeCommand:
		if owner.Kind != RecoveryOwnerSessionBank {
			return fmt.Errorf("command descriptors require a session bank owner")
		}
		if !owner.MayRestart {
			return fmt.Errorf("command descriptors require restart permission")
		}
	case RecoveryModeTopology:
		if owner.Kind != RecoveryOwnerSessionBank {
			return fmt.Errorf("topology descriptors require a session bank owner")
		}
		if !owner.MayRestart {
			return fmt.Errorf("topology descriptors require topology recreation permission")
		}
	case RecoveryModeManaged:
		if owner.Kind != RecoveryOwnerExternalManager || owner.MayRestart {
			return fmt.Errorf("managed descriptors require a non-restarting external owner")
		}
	case RecoveryModeUnresolved:
		if owner.MayRestart {
			return fmt.Errorf("unresolved descriptors cannot permit restart")
		}
	}
	return nil
}

func (d WorkloadRecoveryDescriptor) CanonicalCommand(ownerHome string) (string, bool) {
	argv, ok := d.CanonicalArgv(ownerHome)
	if !ok {
		return "", false
	}
	return shellJoinArgv(argv), true
}

func (d WorkloadRecoveryDescriptor) CanonicalArgv(ownerHome string) ([]string, bool) {
	desc, err := CanonicalizeWorkloadRecoveryDescriptor(d, ownerHome)
	if err != nil {
		return nil, false
	}
	switch desc.Mode {
	case RecoveryModeAgent:
		return canonicalRecoveryAgentArgv(*desc.Agent, ownerHome)
	case RecoveryModeCommand:
		return canonicalRecoveryTypedArgv(*desc.Command)
	default:
		return nil, false
	}
}

func shellJoinArgv(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, token := range argv {
		parts = append(parts, shellQuoteArg(token))
	}
	return strings.Join(parts, " ")
}

func shellQuoteArg(token string) string {
	if token != "" && recoveryShellSafeTokenRegex.MatchString(token) {
		return token
	}
	return "'" + strings.ReplaceAll(token, "'", "'\\''") + "'"
}

func canonicalRecoveryAgent(agent *WorkloadRecoveryAgent) (WorkloadRecoveryAgent, error) {
	if agent == nil {
		return WorkloadRecoveryAgent{}, fmt.Errorf("agent descriptor is required")
	}
	result := WorkloadRecoveryAgent{
		Kind:            strings.ToLower(strings.TrimSpace(agent.Kind)),
		NativeSessionID: strings.TrimSpace(agent.NativeSessionID),
		HermesProfile:   strings.TrimSpace(agent.HermesProfile),
	}
	switch result.Kind {
	case RecoveryAgentCodex, RecoveryAgentClaude:
		if !recoveryUUIDRegex.MatchString(result.NativeSessionID) {
			return WorkloadRecoveryAgent{}, fmt.Errorf("%s native session id must be a uuid", result.Kind)
		}
		result.HermesProfile = ""
	case RecoveryAgentHermes:
		if !recoveryNativeIDRegex.MatchString(result.NativeSessionID) {
			return WorkloadRecoveryAgent{}, fmt.Errorf("hermes native session id is malformed")
		}
		if !recoveryHermesProfileRegex.MatchString(result.HermesProfile) {
			return WorkloadRecoveryAgent{}, fmt.Errorf("hermes profile is malformed")
		}
	default:
		return WorkloadRecoveryAgent{}, fmt.Errorf("unsupported agent kind %q", result.Kind)
	}
	return result, nil
}

func canonicalRecoveryAgentArgv(agent WorkloadRecoveryAgent, ownerHome string) ([]string, bool) {
	switch agent.Kind {
	case RecoveryAgentCodex:
		return []string{"codex", "resume", agent.NativeSessionID}, true
	case RecoveryAgentClaude:
		return []string{"claude", "--resume", agent.NativeSessionID}, true
	case RecoveryAgentHermes:
		ownerHome = filepath.Clean(strings.TrimSpace(ownerHome))
		executable := filepath.Join(ownerHome, ".hermes", "hermes-agent-current", "venv", "bin", "python")
		if !pathUnderOwnerHome(executable, ownerHome) {
			return nil, false
		}
		return []string{executable, "-m", "hermes_cli.main", "--profile", agent.HermesProfile, "--resume", agent.NativeSessionID}, true
	default:
		return nil, false
	}
}

func canonicalRecoveryCommand(command *WorkloadRecoveryCommand, topology WorkloadRecoveryTopology, ownerHome string) (WorkloadRecoveryCommand, error) {
	if command == nil {
		return WorkloadRecoveryCommand{}, fmt.Errorf("command descriptor is required")
	}
	kind := strings.ToLower(strings.TrimSpace(command.Kind))
	if kind != RecoveryCommandPythonHTTPServer {
		return WorkloadRecoveryCommand{}, fmt.Errorf("unsupported command kind %q", kind)
	}
	if command.PythonHTTPServer == nil {
		return WorkloadRecoveryCommand{}, fmt.Errorf("python-http-server fields are required")
	}
	python := *command.PythonHTTPServer
	python.Bind = strings.TrimSpace(python.Bind)
	if !isLoopbackBind(python.Bind) {
		return WorkloadRecoveryCommand{}, fmt.Errorf("python-http-server bind must be loopback")
	}
	if python.Port < 1 || python.Port > 65535 {
		return WorkloadRecoveryCommand{}, fmt.Errorf("python-http-server port must be 1-65535")
	}
	directory, err := canonicalOwnerHomePath(python.Directory, topology.PaneCurrentPath, ownerHome)
	if err != nil {
		return WorkloadRecoveryCommand{}, err
	}
	python.Directory = directory
	return WorkloadRecoveryCommand{
		Kind:             kind,
		PythonHTTPServer: &python,
	}, nil
}

func canonicalRecoveryTypedArgv(command WorkloadRecoveryCommand) ([]string, bool) {
	if command.Kind != RecoveryCommandPythonHTTPServer || command.PythonHTTPServer == nil {
		return nil, false
	}
	python := command.PythonHTTPServer
	return []string{"python3", "-m", "http.server", strconv.Itoa(python.Port), "--bind", python.Bind, "--directory", python.Directory}, true
}

func canonicalRecoveryOwner(owner WorkloadRecoveryOwner) (WorkloadRecoveryOwner, error) {
	result := WorkloadRecoveryOwner{
		Kind:       strings.ToLower(strings.TrimSpace(owner.Kind)),
		Ref:        strings.TrimSpace(owner.Ref),
		MayRestart: owner.MayRestart,
	}
	switch result.Kind {
	case RecoveryOwnerSessionBank, RecoveryOwnerPersistentAgent, RecoveryOwnerExternalManager:
	default:
		return WorkloadRecoveryOwner{}, fmt.Errorf("unsupported recovery owner kind %q", result.Kind)
	}
	if !recoverySafeReferenceRegex.MatchString(result.Ref) {
		return WorkloadRecoveryOwner{}, fmt.Errorf("recovery owner ref is malformed")
	}
	return result, nil
}

func canonicalRecoveryTopology(topology WorkloadRecoveryTopology) (WorkloadRecoveryTopology, error) {
	result := WorkloadRecoveryTopology{
		SessionName:     strings.TrimSpace(topology.SessionName),
		SessionID:       strings.TrimSpace(topology.SessionID),
		WindowIndex:     topology.WindowIndex,
		WindowName:      strings.TrimSpace(topology.WindowName),
		WindowLayout:    strings.TrimSpace(topology.WindowLayout),
		PaneIndex:       topology.PaneIndex,
		PaneID:          strings.TrimSpace(topology.PaneID),
		PaneCurrentPath: strings.TrimSpace(topology.PaneCurrentPath),
	}
	if result.SessionName != "" {
		if valid, _ := core.ValidateSessionName(result.SessionName, "session name"); !valid {
			return WorkloadRecoveryTopology{}, fmt.Errorf("topology session name is malformed")
		}
	}
	if result.WindowIndex < 0 || result.PaneIndex < 0 {
		return WorkloadRecoveryTopology{}, fmt.Errorf("topology indexes must be non-negative")
	}
	for _, value := range []string{result.SessionID, result.WindowName, result.PaneID, result.WindowLayout} {
		if strings.ContainsAny(value, "\x00\n\r") {
			return WorkloadRecoveryTopology{}, fmt.Errorf("topology contains unsafe control characters")
		}
	}
	if len(result.WindowLayout) > recoveryMaxWindowLayoutLength {
		return WorkloadRecoveryTopology{}, fmt.Errorf("topology window layout is too long")
	}
	if result.PaneID != "" && !recoverySafeTmuxIDRegex.MatchString(result.PaneID) {
		return WorkloadRecoveryTopology{}, fmt.Errorf("topology pane id is malformed")
	}
	if result.PaneCurrentPath != "" {
		path, err := canonicalPath(result.PaneCurrentPath)
		if err != nil {
			return WorkloadRecoveryTopology{}, err
		}
		result.PaneCurrentPath = path
	}
	return result, nil
}

func canonicalOwnerHomePath(value, basePath, ownerHome string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("owner-home-bounded path is required")
	}
	if strings.ContainsAny(value, "\x00\n\r#") {
		return "", fmt.Errorf("owner-home-bounded path contains unsafe characters")
	}
	if !filepath.IsAbs(value) {
		basePath = strings.TrimSpace(basePath)
		if basePath == "" || !filepath.IsAbs(basePath) {
			return "", fmt.Errorf("relative path requires absolute pane cwd")
		}
		value = filepath.Join(basePath, value)
	}
	path, err := canonicalPath(value)
	if err != nil {
		return "", err
	}
	if !pathUnderOwnerHome(path, ownerHome) {
		return "", fmt.Errorf("path must stay under owner home")
	}
	return path, nil
}

func canonicalPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\n\r#") {
		return "", fmt.Errorf("path contains unsafe characters")
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("path must be absolute")
	}
	return filepath.Clean(value), nil
}

func pathUnderOwnerHome(path, ownerHome string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	ownerHome = filepath.Clean(strings.TrimSpace(ownerHome))
	if path == "" || ownerHome == "" || !filepath.IsAbs(path) || !filepath.IsAbs(ownerHome) {
		return false
	}
	if !lexicalPathUnderOwnerHome(path, ownerHome) {
		return false
	}
	if _, err := os.Lstat(ownerHome); err != nil {
		return os.IsNotExist(err)
	}
	resolvedOwnerHome, err := filepath.EvalSymlinks(ownerHome)
	if err != nil {
		return false
	}
	existingPrefix, err := deepestExistingPathPrefix(path)
	if err != nil {
		return false
	}
	resolvedPrefix, err := filepath.EvalSymlinks(existingPrefix)
	if err != nil {
		return false
	}
	return lexicalPathUnderOwnerHome(resolvedPrefix, resolvedOwnerHome)
}

func lexicalPathUnderOwnerHome(path, ownerHome string) bool {
	rel, err := filepath.Rel(ownerHome, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func deepestExistingPathPrefix(path string) (string, error) {
	current := filepath.Clean(path)
	for {
		if _, err := os.Lstat(current); err == nil {
			return current, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}

func isLoopbackBind(bind string) bool {
	switch bind {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func allowedRecoveryMode(mode string) bool {
	switch mode {
	case RecoveryModeTopology, RecoveryModeAgent, RecoveryModeCommand, RecoveryModeManaged, RecoveryModeUnresolved:
		return true
	default:
		return false
	}
}

func allowedRecoveryEvidenceSource(source string) bool {
	switch source {
	case RecoveryEvidenceArgv, RecoveryEvidenceTranscript, RecoveryEvidenceStateDB, RecoveryEvidenceTopology, RecoveryEvidenceManager, RecoveryEvidenceProcess:
		return true
	default:
		return false
	}
}

func allowedRecoveryConfidence(confidence string) bool {
	switch confidence {
	case RecoveryConfidenceHigh, RecoveryConfidenceMedium, RecoveryConfidenceLow:
		return true
	default:
		return false
	}
}

func allowedRecoveryUnresolvedReason(reason string) bool {
	switch reason {
	case RecoveryUnresolvedUnknownProcess, RecoveryUnresolvedAmbiguousCandidates, RecoveryUnresolvedUnsafeEvidence, RecoveryUnresolvedUnsupportedWorkload, RecoveryUnresolvedMissingEvidence, RecoveryUnresolvedConflictingOwners, RecoveryUnresolvedConflictingEvidence:
		return true
	default:
		return false
	}
}
