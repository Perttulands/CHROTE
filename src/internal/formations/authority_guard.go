package formations

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const RuntimeAuthorityGuardCapabilityV1 = "formations.runtime-authority-read-guard.v1"

const (
	runtimeAuthoritySchema             = uint64(2)
	runtimeAuthorityMaxJSONInteger     = uint64(9007199254740991)
	runtimeAuthorityMaxRunLimit        = uint64(2147483647)
	runtimeAuthorityMaxRecordBytes     = int64(1 << 20)
	runtimeAuthorityMaxEventBytes      = 1 << 20
	runtimeAuthorityMaxJSONDepth       = 64
	runtimeAuthorityDirectoryBatchSize = 128
)

var (
	runtimeWorkspaceAuthorityIDPattern = regexp.MustCompile(`^wsa_[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	runtimeSHA256Pattern               = regexp.MustCompile(`^[0-9a-f]{64}$`)
	runtimeUnsignedIntegerPattern      = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
)

type RuntimeAuthorityInputClass string

const (
	RuntimeAuthoritySchema1Inspection RuntimeAuthorityInputClass = "schema_1_inspection"
	RuntimeAuthoritySchema2Guarded    RuntimeAuthorityInputClass = "schema_2_guarded_non_authorizing"
)

type RuntimeAuthorityCapability struct {
	ID                 string `json:"id"`
	AuthoritySchema    uint64 `json:"authoritySchema"`
	Authorizing        bool   `json:"authorizing"`
	SemanticProjection bool   `json:"semanticProjection"`
	Recovery           bool   `json:"recovery"`
	Cleanup            bool   `json:"cleanup"`
	Quarantine         bool   `json:"quarantine"`
	Fencing            bool   `json:"fencing"`
	Execution          bool   `json:"execution"`
}

type RuntimeAuthorityLedgerSummary struct {
	Schema1Inspection uint64 `json:"schema1Inspection"`
	Schema2Guarded    uint64 `json:"schema2Guarded"`
}

type RuntimeAuthorityGuardResult struct {
	Capability RuntimeAuthorityCapability    `json:"capability"`
	Ledgers    RuntimeAuthorityLedgerSummary `json:"ledgers"`
}

type RuntimeAuthorityGuardStage string

const (
	RuntimeAuthorityGuardStageRoot               RuntimeAuthorityGuardStage = "root"
	RuntimeAuthorityGuardStageRegistry           RuntimeAuthorityGuardStage = "registry"
	RuntimeAuthorityGuardStageBootstrap          RuntimeAuthorityGuardStage = "bootstrap"
	RuntimeAuthorityGuardStageWorkspaceAuthority RuntimeAuthorityGuardStage = "workspace_authority"
	RuntimeAuthorityGuardStageAdmissionPolicy    RuntimeAuthorityGuardStage = "admission_policy"
	RuntimeAuthorityGuardStageEventEnvelope      RuntimeAuthorityGuardStage = "event_envelope"
)

type RuntimeAuthorityGuardCode string

const (
	RuntimeAuthorityGuardMissing           RuntimeAuthorityGuardCode = "missing"
	RuntimeAuthorityGuardMalformed         RuntimeAuthorityGuardCode = "malformed"
	RuntimeAuthorityGuardDuplicateKey      RuntimeAuthorityGuardCode = "duplicate_key"
	RuntimeAuthorityGuardUnknownKey        RuntimeAuthorityGuardCode = "unknown_key"
	RuntimeAuthorityGuardUnsupportedSchema RuntimeAuthorityGuardCode = "unsupported_schema"
	RuntimeAuthorityGuardNoncanonical      RuntimeAuthorityGuardCode = "noncanonical"
	RuntimeAuthorityGuardConflict          RuntimeAuthorityGuardCode = "conflict"
	RuntimeAuthorityGuardIntegrityMismatch RuntimeAuthorityGuardCode = "integrity_mismatch"
	RuntimeAuthorityGuardMixedSchema       RuntimeAuthorityGuardCode = "mixed_schema"
	RuntimeAuthorityGuardOutOfRange        RuntimeAuthorityGuardCode = "out_of_range"
)

type RuntimeAuthorityGuardError struct {
	Stage        RuntimeAuthorityGuardStage
	Code         RuntimeAuthorityGuardCode
	RelativePath string
	Err          error
}

func (e *RuntimeAuthorityGuardError) Error() string {
	if e == nil {
		return "formations runtime authority guard rejected input"
	}
	message := fmt.Sprintf("formations runtime authority guard rejected %s", e.Stage)
	if e.RelativePath != "" {
		message += " at " + e.RelativePath
	}
	if e.Code != "" {
		message += ": " + string(e.Code)
	}
	return message
}

func (e *RuntimeAuthorityGuardError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type runtimeWorkspaceRegistry struct {
	RegistrySchema json.Number                      `json:"registrySchema"`
	RecordRev      json.Number                      `json:"recordRev"`
	Entries        *[]runtimeWorkspaceRegistryEntry `json:"entries"`
}

type runtimeWorkspaceRegistryEntry struct {
	WorkspaceAuthorityID      string `json:"workspaceAuthorityId"`
	ConfiguredPath            string `json:"configuredPath"`
	Device                    string `json:"device"`
	Inode                     string `json:"inode"`
	WorkspaceRootIdentityHash string `json:"workspaceRootIdentitySha256"`
	decodedDevice             uint64
	decodedInode              uint64
}

type runtimeWorkspaceBootstrap struct {
	BootstrapSchema           json.Number `json:"bootstrapSchema"`
	WorkspaceAuthorityID      string      `json:"workspaceAuthorityId"`
	RootIdentityEncoding      string      `json:"rootIdentityEncoding"`
	WorkspaceRootIdentityHash string      `json:"workspaceRootIdentitySha256"`
}

type runtimeWorkspaceAuthority struct {
	RecordRev                 json.Number                        `json:"recordRev"`
	AuthoritySchema           json.Number                        `json:"authoritySchema"`
	WorkspaceAuthorityID      string                             `json:"workspaceAuthorityId"`
	RootIdentityEncoding      string                             `json:"rootIdentityEncoding"`
	WorkspaceRootIdentityHash string                             `json:"workspaceRootIdentitySha256"`
	NextWriterFence           json.Number                        `json:"nextWriterFence"`
	NextAdmissionSeq          json.Number                        `json:"nextAdmissionSeq"`
	AdmissionPolicyRef        runtimeWorkspaceAdmissionPolicyRef `json:"admissionPolicyRef"`
}

type runtimeWorkspaceAdmissionPolicyRef struct {
	PolicyRev    json.Number `json:"policyRev"`
	PolicySHA256 string      `json:"policySha256"`
}

type runtimeWorkspaceAdmissionPolicy struct {
	PolicySchema      json.Number  `json:"policySchema"`
	PolicyRev         json.Number  `json:"policyRev"`
	PriorPolicySHA256 string       `json:"priorPolicySha256"`
	State             string       `json:"state"`
	MaxActiveRuns     *json.Number `json:"maxActiveRuns,omitempty"`
	MaxQueuedRuns     *json.Number `json:"maxQueuedRuns,omitempty"`
}

type runtimeEventEnvelope struct {
	Schema          *json.Number    `json:"schema,omitempty"`
	AuthoritySchema *json.Number    `json:"authoritySchema,omitempty"`
	WriterFence     *json.Number    `json:"writerFence,omitempty"`
	Timestamp       string          `json:"ts,omitempty"`
	RunID           string          `json:"runId,omitempty"`
	Seq             *json.Number    `json:"seq,omitempty"`
	Type            string          `json:"type"`
	Actor           string          `json:"actor,omitempty"`
	BoardID         string          `json:"boardId,omitempty"`
	BoardRev        *json.Number    `json:"boardRev,omitempty"`
	MissionID       string          `json:"missionId,omitempty"`
	BeadID          string          `json:"beadId,omitempty"`
	NodeID          string          `json:"nodeId,omitempty"`
	SlotID          string          `json:"slotId,omitempty"`
	GateID          string          `json:"gateId,omitempty"`
	EdgeID          string          `json:"edgeId,omitempty"`
	Epoch           *json.Number    `json:"epoch,omitempty"`
	Attempt         *json.Number    `json:"attempt,omitempty"`
	Data            json.RawMessage `json:"data,omitempty"`
}

var runtimeAuthorityClosedJSONKeys = map[reflect.Type]map[string]struct{}{
	reflect.TypeOf(runtimeWorkspaceRegistry{}): runtimeAuthorityJSONKeySet(
		"registrySchema", "recordRev", "entries",
	),
	reflect.TypeOf(runtimeWorkspaceRegistryEntry{}): runtimeAuthorityJSONKeySet(
		"workspaceAuthorityId", "configuredPath", "device", "inode", "workspaceRootIdentitySha256",
	),
	reflect.TypeOf(runtimeWorkspaceBootstrap{}): runtimeAuthorityJSONKeySet(
		"bootstrapSchema", "workspaceAuthorityId", "rootIdentityEncoding", "workspaceRootIdentitySha256",
	),
	reflect.TypeOf(runtimeWorkspaceAuthority{}): runtimeAuthorityJSONKeySet(
		"recordRev", "authoritySchema", "workspaceAuthorityId", "rootIdentityEncoding", "workspaceRootIdentitySha256",
		"nextWriterFence", "nextAdmissionSeq", "admissionPolicyRef",
	),
	reflect.TypeOf(runtimeWorkspaceAdmissionPolicyRef{}): runtimeAuthorityJSONKeySet(
		"policyRev", "policySha256",
	),
	reflect.TypeOf(runtimeWorkspaceAdmissionPolicy{}): runtimeAuthorityJSONKeySet(
		"policySchema", "policyRev", "priorPolicySha256", "state", "maxActiveRuns", "maxQueuedRuns",
	),
	reflect.TypeOf(runtimeEventEnvelope{}): runtimeAuthorityJSONKeySet(
		"schema", "authoritySchema", "writerFence", "ts", "runId", "seq", "type", "actor", "boardId", "boardRev",
		"missionId", "beadId", "nodeId", "slotId", "gateId", "edgeId", "epoch", "attempt", "data",
	),
}

func runtimeAuthorityJSONKeySet(names ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		result[name] = struct{}{}
	}
	return result
}

func GuardRuntimeAuthorityV1(workspacesRoot string) (RuntimeAuthorityGuardResult, error) {
	result := RuntimeAuthorityGuardResult{Capability: disabledRuntimeAuthorityCapability()}
	ledgers, err := guardRuntimeAuthorityV1(workspacesRoot)
	if err != nil {
		return result, err
	}
	result.Ledgers = ledgers
	return result, nil
}

// GuardRuntimeWorkspaceAuthorityV1 validates the host-private authority domain
// bound to one configured workspace. A successful read remains deliberately
// non-authorizing until the coordinator owns execution and fencing.
func GuardRuntimeWorkspaceAuthorityV1(formationsDataRoot, configuredWorkspace string) (RuntimeAuthorityGuardResult, error) {
	result := RuntimeAuthorityGuardResult{Capability: disabledRuntimeAuthorityCapability()}
	if err := validateRuntimeAuthorityRootPath(formationsDataRoot); err != nil {
		return result, runtimeGuardError(RuntimeAuthorityGuardStageRoot, RuntimeAuthorityGuardNoncanonical, "", err)
	}
	identity, err := openRuntimeWorkspaceIdentity(configuredWorkspace)
	if err != nil {
		code := RuntimeAuthorityGuardMalformed
		switch {
		case errors.Is(err, errRuntimeNoncanonical):
			code = RuntimeAuthorityGuardNoncanonical
		case errors.Is(err, os.ErrNotExist):
			code = RuntimeAuthorityGuardMissing
		}
		return result, runtimeGuardError(RuntimeAuthorityGuardStageRegistry, code, "", err)
	}
	authorityRoot, err := openRuntimeAuthorityRoot(formationsDataRoot)
	if err != nil {
		code := RuntimeAuthorityGuardNoncanonical
		if errors.Is(err, os.ErrNotExist) {
			code = RuntimeAuthorityGuardMissing
		}
		return result, runtimeGuardError(RuntimeAuthorityGuardStageRoot, code, "", err)
	}
	defer authorityRoot.Close()
	if err := validateRuntimeAuthorityWorkspaceIsolation(formationsDataRoot, authorityRoot, identity); err != nil {
		code := runtimeGuardValidationCode(err)
		if errors.Is(err, os.ErrNotExist) {
			code = RuntimeAuthorityGuardMissing
		}
		return result, runtimeGuardError(RuntimeAuthorityGuardStageRoot, code, "", err)
	}

	workspacesRoot := filepath.Join(formationsDataRoot, "workspaces")
	rootDir, err := openRuntimeAuthorityDirectoryAt(authorityRoot, "workspaces")
	if err != nil {
		code := RuntimeAuthorityGuardNoncanonical
		if errors.Is(err, os.ErrNotExist) {
			code = RuntimeAuthorityGuardMissing
		}
		return result, runtimeGuardError(RuntimeAuthorityGuardStageRoot, code, "", err)
	}
	registry, err := readRuntimeWorkspaceRegistry(rootDir, workspacesRoot)
	if err != nil {
		rootDir.Close()
		return result, err
	}
	defer rootDir.Close()
	entry, err := matchRuntimeWorkspaceRegistryEntry(registry, identity)
	if err != nil {
		code := runtimeGuardValidationCode(err)
		if errors.Is(err, os.ErrNotExist) {
			code = RuntimeAuthorityGuardMissing
		}
		return result, runtimeGuardError(RuntimeAuthorityGuardStageRegistry, code, "registry.private.json", err)
	}
	if err := validateRuntimeAuthorityEntry(rootDir, workspacesRoot, entry, &result.Ledgers); err != nil {
		return RuntimeAuthorityGuardResult{Capability: disabledRuntimeAuthorityCapability()}, err
	}
	return result, nil
}

func disabledRuntimeAuthorityCapability() RuntimeAuthorityCapability {
	return RuntimeAuthorityCapability{
		ID:              RuntimeAuthorityGuardCapabilityV1,
		AuthoritySchema: runtimeAuthoritySchema,
	}
}

func guardRuntimeAuthorityV1(workspacesRoot string) (RuntimeAuthorityLedgerSummary, error) {
	rootDir, registry, err := openRuntimeWorkspaceRegistry(workspacesRoot)
	if err != nil {
		return RuntimeAuthorityLedgerSummary{}, err
	}
	defer rootDir.Close()

	var ledgers RuntimeAuthorityLedgerSummary
	for _, entry := range *registry.Entries {
		if err := validateRuntimeAuthorityEntry(rootDir, workspacesRoot, entry, &ledgers); err != nil {
			return RuntimeAuthorityLedgerSummary{}, err
		}
	}
	return ledgers, nil
}

func openRuntimeWorkspaceRegistry(workspacesRoot string) (*os.File, runtimeWorkspaceRegistry, error) {
	rootDir, err := openRuntimeAuthorityRoot(workspacesRoot)
	if err != nil {
		code := RuntimeAuthorityGuardNoncanonical
		if errors.Is(err, os.ErrNotExist) {
			code = RuntimeAuthorityGuardMissing
		}
		return nil, runtimeWorkspaceRegistry{}, runtimeGuardError(RuntimeAuthorityGuardStageRoot, code, "", err)
	}
	registry, err := readRuntimeWorkspaceRegistry(rootDir, workspacesRoot)
	if err != nil {
		rootDir.Close()
		return nil, runtimeWorkspaceRegistry{}, err
	}
	return rootDir, registry, nil
}

func readRuntimeWorkspaceRegistry(rootDir *os.File, workspacesRoot string) (runtimeWorkspaceRegistry, error) {
	registryPath := filepath.Join(workspacesRoot, "registry.private.json")
	registryRaw, err := readRuntimeAuthorityFileAt(rootDir, "registry.private.json", runtimeAuthorityMaxRecordBytes)
	if err != nil {
		return runtimeWorkspaceRegistry{}, runtimeGuardFileError(RuntimeAuthorityGuardStageRegistry, workspacesRoot, registryPath, err)
	}
	var registry runtimeWorkspaceRegistry
	if err := decodeRuntimeAuthorityJSON(registryRaw, &registry); err != nil {
		return runtimeWorkspaceRegistry{}, runtimeGuardDecodeError(RuntimeAuthorityGuardStageRegistry, workspacesRoot, registryPath, err)
	}
	if err := validateRuntimeWorkspaceRegistry(&registry); err != nil {
		return runtimeWorkspaceRegistry{}, runtimeGuardError(RuntimeAuthorityGuardStageRegistry, runtimeGuardValidationCode(err), "registry.private.json", err)
	}
	return registry, nil
}

type runtimeWorkspaceIdentity struct {
	configuredPath string
	resolvedPath   string
	device         uint64
	inode          uint64
	rootHash       string
}

func openRuntimeWorkspaceIdentity(configuredWorkspace string) (runtimeWorkspaceIdentity, error) {
	configuredPath := filepath.ToSlash(filepath.Clean(configuredWorkspace))
	if err := validateRuntimeConfiguredPath(configuredPath); err != nil {
		return runtimeWorkspaceIdentity{}, err
	}
	fd, err := syscall.Open(configuredPath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return runtimeWorkspaceIdentity{}, err
	}
	workspace := os.NewFile(uintptr(fd), configuredPath)
	if workspace == nil {
		_ = syscall.Close(fd)
		return runtimeWorkspaceIdentity{}, errRuntimeNoncanonical
	}
	defer workspace.Close()
	info, err := workspace.Stat()
	if err != nil {
		return runtimeWorkspaceIdentity{}, err
	}
	if !info.IsDir() {
		return runtimeWorkspaceIdentity{}, errRuntimeNoncanonical
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return runtimeWorkspaceIdentity{}, errRuntimeNoncanonical
	}
	resolvedPath, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", workspace.Fd()))
	if err != nil {
		return runtimeWorkspaceIdentity{}, err
	}
	resolvedPath = filepath.ToSlash(resolvedPath)
	if err := validateRuntimeConfiguredPath(resolvedPath); err != nil {
		return runtimeWorkspaceIdentity{}, err
	}
	identity := runtimeWorkspaceIdentity{
		configuredPath: configuredPath,
		resolvedPath:   resolvedPath,
		device:         uint64(stat.Dev),
		inode:          stat.Ino,
	}
	identity.rootHash = runtimeWorkspaceIdentityHash(identity)
	return identity, nil
}

func runtimeWorkspaceIdentityHash(identity runtimeWorkspaceIdentity) string {
	var canonical bytes.Buffer
	canonical.WriteString(`{"configuredPath":`)
	writeRuntimeCanonicalJSONString(&canonical, identity.configuredPath)
	canonical.WriteString(`,"device":`)
	writeRuntimeCanonicalJSONString(&canonical, strconv.FormatUint(identity.device, 10))
	canonical.WriteString(`,"inode":`)
	writeRuntimeCanonicalJSONString(&canonical, strconv.FormatUint(identity.inode, 10))
	canonical.WriteString(`,"resolvedPath":`)
	writeRuntimeCanonicalJSONString(&canonical, identity.resolvedPath)
	canonical.WriteByte('}')
	return runtimeSHA256Hex(canonical.Bytes())
}

func writeRuntimeCanonicalJSONString(output *bytes.Buffer, value string) {
	const hexDigits = "0123456789abcdef"
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"', '\\':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '\b':
			output.WriteString(`\b`)
		case '\t':
			output.WriteString(`\t`)
		case '\n':
			output.WriteString(`\n`)
		case '\f':
			output.WriteString(`\f`)
		case '\r':
			output.WriteString(`\r`)
		default:
			if character < 0x20 {
				output.WriteString(`\u00`)
				output.WriteByte(hexDigits[byte(character)>>4])
				output.WriteByte(hexDigits[byte(character)&0x0f])
			} else {
				output.WriteRune(character)
			}
		}
	}
	output.WriteByte('"')
}

func matchRuntimeWorkspaceRegistryEntry(registry runtimeWorkspaceRegistry, identity runtimeWorkspaceIdentity) (runtimeWorkspaceRegistryEntry, error) {
	for _, entry := range *registry.Entries {
		if entry.ConfiguredPath == identity.configuredPath {
			if entry.decodedDevice != identity.device || entry.decodedInode != identity.inode || entry.WorkspaceRootIdentityHash != identity.rootHash {
				return runtimeWorkspaceRegistryEntry{}, errRuntimeIntegrityMismatch
			}
			return entry, nil
		}
	}
	for _, entry := range *registry.Entries {
		if (entry.decodedDevice == identity.device && entry.decodedInode == identity.inode) || entry.WorkspaceRootIdentityHash == identity.rootHash {
			return runtimeWorkspaceRegistryEntry{}, errRuntimeConflict
		}
	}
	return runtimeWorkspaceRegistryEntry{}, os.ErrNotExist
}

func validateRuntimeAuthorityRootPath(root string) error {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || (root != string(filepath.Separator) && strings.HasSuffix(root, string(filepath.Separator))) {
		return errors.New("authority root must be an absolute clean path")
	}
	return nil
}

func validateRuntimeAuthorityWorkspaceIsolation(formationsDataRoot string, root *os.File, identity runtimeWorkspaceIdentity) error {
	for _, workspacePath := range []string{identity.configuredPath, identity.resolvedPath} {
		if runtimePathsOverlap(formationsDataRoot, filepath.FromSlash(workspacePath)) {
			return errRuntimeConflict
		}
	}
	if root == nil {
		return errRuntimeNoncanonical
	}
	info, err := root.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errRuntimeNoncanonical
	}
	if uint64(stat.Dev) == identity.device && stat.Ino == identity.inode {
		return errRuntimeConflict
	}
	resolvedRoot, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", root.Fd()))
	if err != nil {
		return err
	}
	if !filepath.IsAbs(resolvedRoot) || filepath.Clean(resolvedRoot) != resolvedRoot {
		return errRuntimeNoncanonical
	}
	if filepath.ToSlash(resolvedRoot) != filepath.ToSlash(formationsDataRoot) {
		return errRuntimeConflict
	}
	for _, workspacePath := range []string{identity.configuredPath, identity.resolvedPath} {
		if runtimePathsOverlap(resolvedRoot, filepath.FromSlash(workspacePath)) {
			return errRuntimeConflict
		}
	}
	return nil
}

func runtimePathsOverlap(left, right string) bool {
	return runtimePathContains(left, right) || runtimePathContains(right, left)
}

func runtimePathContains(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func validateRuntimeWorkspaceRegistry(registry *runtimeWorkspaceRegistry) error {
	if !jsonNumberEquals(registry.RegistrySchema, 1) {
		return errRuntimeUnsupportedSchema
	}
	if _, err := runtimePositiveJSONInteger(registry.RecordRev); err != nil {
		return err
	}
	if registry.Entries == nil {
		return errRuntimeConflict
	}
	entries := *registry.Entries
	configuredPaths := map[string]bool{}
	openedRoots := map[string]bool{}
	authorityIDs := map[string]bool{}
	rootIdentityHashes := map[string]bool{}
	for i := range entries {
		entry := &entries[i]
		if !runtimeWorkspaceAuthorityIDPattern.MatchString(entry.WorkspaceAuthorityID) {
			return errRuntimeNoncanonical
		}
		if err := validateRuntimeConfiguredPath(entry.ConfiguredPath); err != nil {
			return err
		}
		device, err := runtimeCanonicalUint64String(entry.Device)
		if err != nil {
			return err
		}
		inode, err := runtimeCanonicalUint64String(entry.Inode)
		if err != nil {
			return err
		}
		if !runtimeSHA256Pattern.MatchString(entry.WorkspaceRootIdentityHash) {
			return errRuntimeNoncanonical
		}
		entry.decodedDevice = device
		entry.decodedInode = inode
		openedKey := entry.Device + ":" + entry.Inode
		if configuredPaths[entry.ConfiguredPath] || openedRoots[openedKey] || authorityIDs[entry.WorkspaceAuthorityID] || rootIdentityHashes[entry.WorkspaceRootIdentityHash] {
			return errRuntimeConflict
		}
		configuredPaths[entry.ConfiguredPath] = true
		openedRoots[openedKey] = true
		authorityIDs[entry.WorkspaceAuthorityID] = true
		rootIdentityHashes[entry.WorkspaceRootIdentityHash] = true
		if i > 0 && !runtimeRegistryEntryLess(entries[i-1], *entry) {
			return errRuntimeNoncanonical
		}
	}
	return nil
}

func runtimeRegistryEntryLess(left, right runtimeWorkspaceRegistryEntry) bool {
	if left.decodedDevice != right.decodedDevice {
		return left.decodedDevice < right.decodedDevice
	}
	if left.decodedInode != right.decodedInode {
		return left.decodedInode < right.decodedInode
	}
	return left.ConfiguredPath < right.ConfiguredPath
}

func validateRuntimeConfiguredPath(value string) error {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || strings.Contains(value, `\`) || !pathpkg.IsAbs(value) || pathpkg.Clean(value) != value || (value != "/" && strings.HasSuffix(value, "/")) {
		return errRuntimeNoncanonical
	}
	return nil
}

func validateRuntimeAuthorityEntry(rootDir *os.File, workspacesRoot string, entry runtimeWorkspaceRegistryEntry, ledgers *RuntimeAuthorityLedgerSummary) error {
	authorityPath := filepath.Join(workspacesRoot, entry.WorkspaceAuthorityID)
	authorityDir, err := openRuntimeAuthorityDirectoryAt(rootDir, entry.WorkspaceAuthorityID)
	if err != nil {
		return runtimeGuardFileError(RuntimeAuthorityGuardStageBootstrap, workspacesRoot, authorityPath, err)
	}
	defer authorityDir.Close()
	if err := validateRuntimeAuthorityDomain(workspacesRoot, authorityPath, authorityDir, entry); err != nil {
		return err
	}
	return validateRuntimeAuthorityLedgers(workspacesRoot, authorityPath, authorityDir, ledgers)
}

func validateRuntimeAuthorityDomain(workspacesRoot, authorityPath string, authorityDir *os.File, entry runtimeWorkspaceRegistryEntry) error {
	bootstrapPath := filepath.Join(authorityPath, "workspace.bootstrap.json")
	bootstrapRaw, err := readRuntimeAuthorityFileAt(authorityDir, "workspace.bootstrap.json", runtimeAuthorityMaxRecordBytes)
	if err != nil {
		return runtimeGuardFileError(RuntimeAuthorityGuardStageBootstrap, workspacesRoot, bootstrapPath, err)
	}
	var bootstrap runtimeWorkspaceBootstrap
	if err := decodeRuntimeAuthorityJSON(bootstrapRaw, &bootstrap); err != nil {
		return runtimeGuardDecodeError(RuntimeAuthorityGuardStageBootstrap, workspacesRoot, bootstrapPath, err)
	}
	if err := validateRuntimeBootstrap(bootstrapRaw, bootstrap, entry); err != nil {
		return runtimeGuardError(RuntimeAuthorityGuardStageBootstrap, runtimeGuardValidationCode(err), runtimeGuardRelativePath(workspacesRoot, bootstrapPath), err)
	}

	workspacePath := filepath.Join(authorityPath, "workspace.private.json")
	workspaceRaw, err := readRuntimeAuthorityFileAt(authorityDir, "workspace.private.json", runtimeAuthorityMaxRecordBytes)
	if err != nil {
		return runtimeGuardFileError(RuntimeAuthorityGuardStageWorkspaceAuthority, workspacesRoot, workspacePath, err)
	}
	var workspaceAuthority runtimeWorkspaceAuthority
	if err := decodeRuntimeAuthorityJSON(workspaceRaw, &workspaceAuthority); err != nil {
		return runtimeGuardDecodeError(RuntimeAuthorityGuardStageWorkspaceAuthority, workspacesRoot, workspacePath, err)
	}
	if err := validateRuntimeWorkspaceAuthority(workspaceAuthority, entry); err != nil {
		return runtimeGuardError(RuntimeAuthorityGuardStageWorkspaceAuthority, runtimeGuardValidationCode(err), runtimeGuardRelativePath(workspacesRoot, workspacePath), err)
	}
	if err := validateRuntimeAdmissionPolicyChain(workspacesRoot, authorityPath, authorityDir, workspaceAuthority.AdmissionPolicyRef); err != nil {
		return err
	}
	return nil
}

func validateRuntimeBootstrap(raw []byte, bootstrap runtimeWorkspaceBootstrap, entry runtimeWorkspaceRegistryEntry) error {
	if !jsonNumberEquals(bootstrap.BootstrapSchema, 1) {
		return errRuntimeUnsupportedSchema
	}
	if bootstrap.WorkspaceAuthorityID != entry.WorkspaceAuthorityID || bootstrap.RootIdentityEncoding != "workspace-root-identity-v1" || bootstrap.WorkspaceRootIdentityHash != entry.WorkspaceRootIdentityHash {
		return errRuntimeIntegrityMismatch
	}
	if !bytes.Equal(raw, canonicalRuntimeBootstrap(bootstrap)) {
		return errRuntimeNoncanonical
	}
	return nil
}

func canonicalRuntimeBootstrap(bootstrap runtimeWorkspaceBootstrap) []byte {
	return []byte(fmt.Sprintf(
		`{"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		bootstrap.WorkspaceAuthorityID,
		bootstrap.WorkspaceRootIdentityHash,
	))
}

func validateRuntimeWorkspaceAuthority(authority runtimeWorkspaceAuthority, entry runtimeWorkspaceRegistryEntry) error {
	if _, err := runtimePositiveJSONInteger(authority.RecordRev); err != nil {
		return err
	}
	authoritySchema, err := runtimePositiveJSONInteger(authority.AuthoritySchema)
	if err != nil {
		return err
	}
	if authoritySchema != runtimeAuthoritySchema {
		return errRuntimeUnsupportedSchema
	}
	if authority.WorkspaceAuthorityID != entry.WorkspaceAuthorityID || authority.RootIdentityEncoding != "workspace-root-identity-v1" || authority.WorkspaceRootIdentityHash != entry.WorkspaceRootIdentityHash {
		return errRuntimeIntegrityMismatch
	}
	if _, err := runtimePositiveJSONInteger(authority.NextWriterFence); err != nil {
		return err
	}
	if _, err := runtimePositiveJSONInteger(authority.NextAdmissionSeq); err != nil {
		return err
	}
	if _, err := runtimePositiveJSONInteger(authority.AdmissionPolicyRef.PolicyRev); err != nil {
		return err
	}
	if !runtimeSHA256Pattern.MatchString(authority.AdmissionPolicyRef.PolicySHA256) {
		return errRuntimeNoncanonical
	}
	return nil
}

func validateRuntimeAdmissionPolicyChain(workspacesRoot, authorityPath string, authorityDir *os.File, ref runtimeWorkspaceAdmissionPolicyRef) error {
	policyDirPath := filepath.Join(authorityPath, "admission-policies")
	policyRev, err := runtimePositiveJSONInteger(ref.PolicyRev)
	if err != nil {
		return runtimeGuardError(RuntimeAuthorityGuardStageAdmissionPolicy, runtimeGuardValidationCode(err), runtimeGuardRelativePath(workspacesRoot, policyDirPath), err)
	}
	policyDir, err := openRuntimeAuthorityDirectoryAt(authorityDir, "admission-policies")
	if err != nil {
		return runtimeGuardFileError(RuntimeAuthorityGuardStageAdmissionPolicy, workspacesRoot, policyDirPath, err)
	}
	defer policyDir.Close()
	expectedHash := ref.PolicySHA256
	for revision := policyRev; revision >= 1; revision-- {
		policyName := strconv.FormatUint(revision, 10) + ".json"
		policyPath := filepath.Join(policyDirPath, policyName)
		raw, err := readRuntimeAuthorityFileAt(policyDir, policyName, runtimeAuthorityMaxRecordBytes)
		if err != nil {
			return runtimeGuardFileError(RuntimeAuthorityGuardStageAdmissionPolicy, workspacesRoot, policyPath, err)
		}
		if runtimeSHA256Hex(raw) != expectedHash {
			return runtimeGuardError(RuntimeAuthorityGuardStageAdmissionPolicy, RuntimeAuthorityGuardIntegrityMismatch, runtimeGuardRelativePath(workspacesRoot, policyPath), errRuntimeIntegrityMismatch)
		}
		var policy runtimeWorkspaceAdmissionPolicy
		if err := decodeRuntimeAuthorityJSON(raw, &policy); err != nil {
			return runtimeGuardDecodeError(RuntimeAuthorityGuardStageAdmissionPolicy, workspacesRoot, policyPath, err)
		}
		if err := validateRuntimeAdmissionPolicy(raw, policy, revision); err != nil {
			return runtimeGuardError(RuntimeAuthorityGuardStageAdmissionPolicy, runtimeGuardValidationCode(err), runtimeGuardRelativePath(workspacesRoot, policyPath), err)
		}
		if revision == 1 {
			break
		}
		expectedHash = policy.PriorPolicySHA256
	}
	return nil
}

func validateRuntimeAdmissionPolicy(raw []byte, policy runtimeWorkspaceAdmissionPolicy, expectedRevision uint64) error {
	if !jsonNumberEquals(policy.PolicySchema, 1) {
		return errRuntimeUnsupportedSchema
	}
	policyRev, err := runtimePositiveJSONInteger(policy.PolicyRev)
	if err != nil {
		return err
	}
	if policyRev != expectedRevision {
		return errRuntimeConflict
	}
	if expectedRevision == 1 {
		if policy.PriorPolicySHA256 != "" {
			return errRuntimeIntegrityMismatch
		}
	} else if !runtimeSHA256Pattern.MatchString(policy.PriorPolicySHA256) {
		return errRuntimeNoncanonical
	}
	switch policy.State {
	case "disabled":
		if policy.MaxActiveRuns != nil || policy.MaxQueuedRuns != nil {
			return errRuntimeConflict
		}
	case "configured":
		if policy.MaxActiveRuns == nil || policy.MaxQueuedRuns == nil {
			return errRuntimeConflict
		}
		active, err := runtimePositiveJSONInteger(*policy.MaxActiveRuns)
		if err != nil {
			return err
		}
		if active > runtimeAuthorityMaxRunLimit {
			return errRuntimeOutOfRange
		}
		queued, err := runtimeNonNegativeJSONInteger(*policy.MaxQueuedRuns, runtimeAuthorityMaxRunLimit)
		if err != nil {
			return err
		}
		_ = queued
	default:
		return errRuntimeNoncanonical
	}
	if !bytes.Equal(raw, canonicalRuntimeAdmissionPolicy(policy, expectedRevision)) {
		return errRuntimeNoncanonical
	}
	return nil
}

func canonicalRuntimeAdmissionPolicy(policy runtimeWorkspaceAdmissionPolicy, revision uint64) []byte {
	if policy.State == "disabled" {
		return []byte(fmt.Sprintf(
			`{"policyRev":%d,"policySchema":1,"priorPolicySha256":"%s","state":"disabled"}`,
			revision,
			policy.PriorPolicySHA256,
		))
	}
	return []byte(fmt.Sprintf(
		`{"maxActiveRuns":%s,"maxQueuedRuns":%s,"policyRev":%d,"policySchema":1,"priorPolicySha256":"%s","state":"configured"}`,
		policy.MaxActiveRuns.String(),
		policy.MaxQueuedRuns.String(),
		revision,
		policy.PriorPolicySHA256,
	))
}

func validateRuntimeAuthorityLedgers(workspacesRoot, authorityPath string, authorityDir *os.File, classifications *RuntimeAuthorityLedgerSummary) error {
	runsPath := filepath.Join(authorityPath, "runs")
	runsDir, err := openRuntimeAuthorityDirectoryAt(authorityDir, "runs")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return runtimeGuardFileError(RuntimeAuthorityGuardStageEventEnvelope, workspacesRoot, runsPath, err)
	}
	defer runsDir.Close()
	for {
		runNames, done, err := readRuntimeAuthorityDirectoryNameBatch(runsDir, runtimeAuthorityDirectoryBatchSize)
		if err != nil {
			return runtimeGuardFileError(RuntimeAuthorityGuardStageEventEnvelope, workspacesRoot, runsPath, err)
		}
		for _, runName := range runNames {
			runPath := filepath.Join(runsPath, runName)
			runDir, err := openRuntimeAuthorityDirectoryAt(runsDir, runName)
			if err != nil {
				return runtimeGuardFileError(RuntimeAuthorityGuardStageEventEnvelope, workspacesRoot, runPath, err)
			}
			ledgerPath := filepath.Join(runPath, "events.ndjson")
			ledger, err := openRuntimeAuthorityFileAt(runDir, "events.ndjson")
			if err != nil {
				runDir.Close()
				return runtimeGuardFileError(RuntimeAuthorityGuardStageEventEnvelope, workspacesRoot, ledgerPath, err)
			}
			class, err := classifyRuntimeAuthorityLedger(ledger, runtimeAuthoritySchema, runName)
			ledger.Close()
			runDir.Close()
			if err != nil {
				return runtimeGuardDecodeError(RuntimeAuthorityGuardStageEventEnvelope, workspacesRoot, ledgerPath, err)
			}
			if err := recordRuntimeAuthorityLedgerClass(classifications, class); err != nil {
				return runtimeGuardError(RuntimeAuthorityGuardStageEventEnvelope, RuntimeAuthorityGuardOutOfRange, runtimeGuardRelativePath(workspacesRoot, ledgerPath), err)
			}
		}
		if done {
			break
		}
	}
	return nil
}

func recordRuntimeAuthorityLedgerClass(summary *RuntimeAuthorityLedgerSummary, class RuntimeAuthorityInputClass) error {
	if summary == nil {
		return errRuntimeConflict
	}
	const maximumCount = ^uint64(0)
	switch class {
	case RuntimeAuthoritySchema1Inspection:
		if summary.Schema1Inspection == maximumCount {
			return errRuntimeOutOfRange
		}
		summary.Schema1Inspection++
	case RuntimeAuthoritySchema2Guarded:
		if summary.Schema2Guarded == maximumCount {
			return errRuntimeOutOfRange
		}
		summary.Schema2Guarded++
	default:
		return errRuntimeConflict
	}
	return nil
}

func classifyRuntimeAuthorityLedger(input io.Reader, expectedAuthoritySchema uint64, expectedRunID string) (RuntimeAuthorityInputClass, error) {
	return classifyRuntimeAuthorityLedgerWithVisitor(input, expectedAuthoritySchema, expectedRunID, nil)
}

type runtimeAuthorityLedgerVisitor func(line []byte) error

func classifyRuntimeAuthorityLedgerWithVisitor(input io.Reader, expectedAuthoritySchema uint64, expectedRunID string, visit runtimeAuthorityLedgerVisitor) (RuntimeAuthorityInputClass, error) {
	reader := bufio.NewReaderSize(input, runtimeAuthorityMaxEventBytes+1)
	var class RuntimeAuthorityInputClass
	var runID string
	var previousFence uint64
	eventCount := 0
	for {
		line, err := readRuntimeAuthorityLedgerLine(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if !utf8.Valid(line) {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("ledger event is not valid UTF-8")}
		}
		if len(bytes.TrimSpace(line)) == 0 {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("ledger contains a blank event")}
		}
		index := eventCount
		eventCount++
		var event runtimeEventEnvelope
		if err := decodeRuntimeAuthorityJSON(line, &event); err != nil {
			return "", err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: err}
		}
		_, schemaPresent := fields["schema"]
		_, authoritySchemaPresent := fields["authoritySchema"]
		_, writerFencePresent := fields["writerFence"]
		if (schemaPresent && event.Schema == nil) || (authoritySchemaPresent && event.AuthoritySchema == nil) || (writerFencePresent && event.WriterFence == nil) {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("event schema fields cannot be null")}
		}
		for _, field := range []string{"ts", "runId", "type", "actor", "boardId", "missionId", "beadId", "nodeId", "slotId", "gateId", "edgeId"} {
			if runtimeJSONFieldIsNull(fields, field) {
				return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: fmt.Errorf("event field %q cannot be null", field)}
			}
		}
		for _, optionalInteger := range []struct {
			name    string
			value   *json.Number
			minimum uint64
		}{
			{name: "boardRev", value: event.BoardRev, minimum: 1},
			{name: "epoch", value: event.Epoch, minimum: 0},
			{name: "attempt", value: event.Attempt, minimum: 1},
		} {
			if _, present := fields[optionalInteger.name]; !present {
				continue
			}
			if optionalInteger.value == nil {
				return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: fmt.Errorf("event field %q cannot be null", optionalInteger.name)}
			}
			if _, err := runtimeCanonicalJSONInteger(*optionalInteger.value, optionalInteger.minimum, runtimeAuthorityMaxJSONInteger); err != nil {
				return "", runtimeValidationDecodeError(err)
			}
		}
		if event.Seq == nil || event.Timestamp == "" || event.RunID == "" || event.Type == "" || event.Actor == "" {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("event is missing a required envelope field")}
		}
		sequence, err := runtimePositiveJSONInteger(*event.Seq)
		if err != nil {
			return "", runtimeValidationDecodeError(err)
		}
		if sequence != uint64(index+1) {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardConflict, err: errRuntimeConflict}
		}
		if _, err := time.Parse(time.RFC3339Nano, event.Timestamp); err != nil {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardNoncanonical, err: err}
		}
		if index == 0 {
			runID = event.RunID
			if expectedRunID != "" && runID != expectedRunID {
				return "", runtimeDecodeError{code: RuntimeAuthorityGuardConflict, err: errRuntimeConflict}
			}
		} else if event.RunID != runID {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardConflict, err: errRuntimeConflict}
		}
		lineClass := RuntimeAuthoritySchema1Inspection
		if schemaPresent {
			schema, err := runtimePositiveJSONInteger(*event.Schema)
			if err != nil {
				return "", runtimeValidationDecodeError(err)
			}
			if schema != 2 {
				return "", runtimeDecodeError{code: RuntimeAuthorityGuardUnsupportedSchema, err: errRuntimeUnsupportedSchema}
			}
			lineClass = RuntimeAuthoritySchema2Guarded
		}
		if index == 0 {
			class = lineClass
		} else if class != lineClass {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardMixedSchema, err: errRuntimeMixedSchema}
		}
		if lineClass == RuntimeAuthoritySchema1Inspection {
			if authoritySchemaPresent || writerFencePresent {
				return "", runtimeDecodeError{code: RuntimeAuthorityGuardMixedSchema, err: errRuntimeMixedSchema}
			}
			if visit != nil {
				if err := visit(line); err != nil {
					return "", err
				}
			}
			continue
		}
		if !authoritySchemaPresent || !writerFencePresent || event.AuthoritySchema == nil || event.WriterFence == nil {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("schema-2 event is missing a required envelope field")}
		}
		authoritySchema, err := runtimePositiveJSONInteger(*event.AuthoritySchema)
		if err != nil {
			return "", runtimeValidationDecodeError(err)
		}
		if authoritySchema != expectedAuthoritySchema {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardUnsupportedSchema, err: errRuntimeUnsupportedSchema}
		}
		fence, err := runtimePositiveJSONInteger(*event.WriterFence)
		if err != nil {
			return "", runtimeValidationDecodeError(err)
		}
		if index > 0 && fence < previousFence {
			return "", runtimeDecodeError{code: RuntimeAuthorityGuardConflict, err: errRuntimeConflict}
		}
		previousFence = fence
	}
	if eventCount == 0 {
		return "", runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("ledger is empty")}
	}
	return class, nil
}

func readRuntimeAuthorityLedgerLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, runtimeDecodeError{code: RuntimeAuthorityGuardOutOfRange, err: errors.New("ledger event exceeds authority byte limit")}
	}
	if errors.Is(err, io.EOF) {
		if len(line) == 0 {
			return nil, io.EOF
		}
		if len(line) > runtimeAuthorityMaxEventBytes {
			return nil, runtimeDecodeError{code: RuntimeAuthorityGuardOutOfRange, err: errors.New("ledger event exceeds authority byte limit")}
		}
		return line, nil
	}
	if err != nil {
		return nil, runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: err}
	}
	line = bytes.TrimSuffix(line, []byte{'\n'})
	if len(line) > runtimeAuthorityMaxEventBytes {
		return nil, runtimeDecodeError{code: RuntimeAuthorityGuardOutOfRange, err: errors.New("ledger event exceeds authority byte limit")}
	}
	return line, nil
}

func runtimeJSONFieldIsNull(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	return ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func runtimeValidationDecodeError(err error) runtimeDecodeError {
	return runtimeDecodeError{code: runtimeGuardValidationCode(err), err: err}
}

type runtimeDecodeError struct {
	code RuntimeAuthorityGuardCode
	err  error
}

func (e runtimeDecodeError) Error() string { return e.err.Error() }
func (e runtimeDecodeError) Unwrap() error { return e.err }

func decodeRuntimeAuthorityJSON(raw []byte, destination any) error {
	if !utf8.Valid(raw) {
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("JSON is not valid UTF-8")}
	}
	if !json.Valid(raw) {
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("JSON syntax is invalid")}
	}
	if err := validateRuntimeAuthorityJSONShape(raw, destination); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		code := RuntimeAuthorityGuardMalformed
		if strings.Contains(err.Error(), "unknown field") {
			code = RuntimeAuthorityGuardUnknownKey
		}
		return runtimeDecodeError{code: code, err: err}
	}
	if err := ensureRuntimeJSONEOF(decoder); err != nil {
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: err}
	}
	return nil
}

func validateRuntimeAuthorityJSONShape(raw []byte, destination any) error {
	if err := rejectRuntimeInvalidJSONSurrogates(raw); err != nil {
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: err}
	}
	expectedType := reflect.TypeOf(destination)
	if expectedType == nil || expectedType.Kind() != reflect.Pointer {
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("JSON destination must be a pointer")}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := scanRuntimeJSONValue(decoder, expectedType.Elem(), 0); err != nil {
		return err
	}
	if err := ensureRuntimeJSONEOF(decoder); err != nil {
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: err}
	}
	return nil
}

func scanRuntimeJSONValue(decoder *json.Decoder, expectedType reflect.Type, depth int) error {
	if depth > runtimeAuthorityMaxJSONDepth {
		return runtimeDecodeError{code: RuntimeAuthorityGuardOutOfRange, err: errors.New("JSON nesting depth exceeds authority limit")}
	}
	token, err := decoder.Token()
	if err != nil {
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: err}
	}
	expectedType = dereferenceRuntimeJSONType(expectedType)
	delimiter, ok := token.(json.Delim)
	if !ok {
		if token == nil && expectedType != nil && expectedType.Kind() == reflect.Struct {
			return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("closed JSON record cannot be null")}
		}
		return nil
	}
	if depth >= runtimeAuthorityMaxJSONDepth {
		return runtimeDecodeError{code: RuntimeAuthorityGuardOutOfRange, err: errors.New("JSON nesting depth exceeds authority limit")}
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		fieldTypes, closed := runtimeJSONStructFieldTypes(expectedType)
		if expectedType != nil && expectedType.Kind() == reflect.Struct && !closed {
			return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: fmt.Errorf("unregistered closed JSON type %s", expectedType)}
		}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: err}
			}
			key, ok := keyToken.(string)
			if !ok {
				return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("JSON object key is not a string")}
			}
			if seen[key] {
				return runtimeDecodeError{code: RuntimeAuthorityGuardDuplicateKey, err: fmt.Errorf("duplicate JSON key %q", key)}
			}
			seen[key] = true
			fieldType, known := fieldTypes[key]
			if closed && !known {
				return runtimeDecodeError{code: RuntimeAuthorityGuardUnknownKey, err: fmt.Errorf("unknown JSON key %q", key)}
			}
			if err := scanRuntimeJSONValue(decoder, fieldType, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("unterminated JSON object")}
		}
	case '[':
		var elementType reflect.Type
		if expectedType != nil && (expectedType.Kind() == reflect.Array || expectedType.Kind() == reflect.Slice) {
			elementType = expectedType.Elem()
		}
		for decoder.More() {
			if err := scanRuntimeJSONValue(decoder, elementType, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("unterminated JSON array")}
		}
	default:
		return runtimeDecodeError{code: RuntimeAuthorityGuardMalformed, err: errors.New("unexpected JSON delimiter")}
	}
	return nil
}

func dereferenceRuntimeJSONType(value reflect.Type) reflect.Type {
	for value != nil && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

func runtimeJSONStructFieldTypes(value reflect.Type) (map[string]reflect.Type, bool) {
	value = dereferenceRuntimeJSONType(value)
	if value == nil || value.Kind() != reflect.Struct {
		return nil, false
	}
	allowedKeys, closed := runtimeAuthorityClosedJSONKeys[value]
	if !closed {
		return nil, false
	}
	fields := make(map[string]reflect.Type, value.NumField())
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		if _, allowed := allowedKeys[name]; allowed {
			fields[name] = field.Type
		}
	}
	return fields, true
}

func rejectRuntimeInvalidJSONSurrogates(raw []byte) error {
	for index := 0; index < len(raw); index++ {
		if raw[index] != '"' {
			continue
		}
		for index++; index < len(raw); index++ {
			switch raw[index] {
			case '"':
				goto nextString
			case '\\':
				index++
				if index >= len(raw) || raw[index] != 'u' {
					continue
				}
				first, ok := runtimeJSONHexCodeUnit(raw, index+1)
				if !ok {
					continue
				}
				index += 4
				switch {
				case first >= 0xd800 && first <= 0xdbff:
					if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
						return errors.New("JSON contains an unpaired high surrogate")
					}
					second, ok := runtimeJSONHexCodeUnit(raw, index+3)
					if !ok || second < 0xdc00 || second > 0xdfff {
						return errors.New("JSON contains an unpaired high surrogate")
					}
					index += 6
				case first >= 0xdc00 && first <= 0xdfff:
					return errors.New("JSON contains an unpaired low surrogate")
				}
			}
		}
	nextString:
	}
	return nil
}

func runtimeJSONHexCodeUnit(raw []byte, start int) (uint16, bool) {
	if start+4 > len(raw) {
		return 0, false
	}
	value, err := strconv.ParseUint(string(raw[start:start+4]), 16, 16)
	return uint16(value), err == nil
}

func ensureRuntimeJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("JSON contains multiple values")
		}
		return err
	}
	return nil
}

func openRuntimeAuthorityRoot(root string) (*os.File, error) {
	if err := validateRuntimeAuthorityRootPath(root); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(string(filepath.Separator), syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: string(filepath.Separator), Err: err}
	}
	current := os.NewFile(uintptr(fd), string(filepath.Separator))
	if current == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open authority root")
	}
	trimmed := strings.TrimPrefix(root, string(filepath.Separator))
	if trimmed == "" {
		return current, nil
	}
	openedPath := string(filepath.Separator)
	for _, component := range strings.Split(trimmed, string(filepath.Separator)) {
		next, err := openRuntimeAuthorityDirectoryAt(current, component)
		if err != nil {
			current.Close()
			return nil, &os.PathError{Op: "open", Path: filepath.Join(openedPath, component), Err: err}
		}
		current.Close()
		current = next
		openedPath = filepath.Join(openedPath, component)
	}
	return current, nil
}

func openRuntimeAuthorityDirectoryAt(parent *os.File, name string) (*os.File, error) {
	return openRuntimeAuthorityComponentAt(parent, name, true)
}

func openRuntimeAuthorityFileAt(parent *os.File, name string) (*os.File, error) {
	return openRuntimeAuthorityComponentAt(parent, name, false)
}

func openRuntimeAuthorityComponentAt(parent *os.File, name string, directory bool) (*os.File, error) {
	if parent == nil || !runtimeAuthorityPathComponent(name) {
		return nil, &os.PathError{Op: "openat", Path: name, Err: syscall.EINVAL}
	}
	flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if directory {
		flags |= syscall.O_DIRECTORY
	}
	fd, err := syscall.Openat(int(parent.Fd()), name, flags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open authority component")
	}
	if !directory {
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, err
		}
		if !info.Mode().IsRegular() {
			file.Close()
			return nil, errors.New("authority record is not a regular file")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Nlink != 1 {
			file.Close()
			return nil, errors.New("authority record must have exactly one link")
		}
	}
	return file, nil
}

func runtimeAuthorityPathComponent(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsRune(name, 0) && !strings.ContainsRune(name, filepath.Separator)
}

func readRuntimeAuthorityFileAt(parent *os.File, name string, maximumBytes int64) ([]byte, error) {
	file, err := openRuntimeAuthorityFileAt(parent, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if maximumBytes < 0 || info.Size() < 0 || info.Size() > maximumBytes {
		return nil, fmt.Errorf("authority record exceeds byte limit: %w", errRuntimeOutOfRange)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximumBytes {
		return nil, fmt.Errorf("authority record exceeds byte limit: %w", errRuntimeOutOfRange)
	}
	return raw, nil
}

func readRuntimeAuthorityDirectoryNameBatch(directory *os.File, batchSize int) ([]string, bool, error) {
	if directory == nil || batchSize <= 0 {
		return nil, false, errRuntimeOutOfRange
	}
	names, err := directory.Readdirnames(batchSize)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	sort.Strings(names)
	return names, errors.Is(err, io.EOF), nil
}

func runtimePositiveJSONInteger(value json.Number) (uint64, error) {
	return runtimeCanonicalJSONInteger(value, 1, runtimeAuthorityMaxJSONInteger)
}

func runtimeNonNegativeJSONInteger(value json.Number, maximum uint64) (uint64, error) {
	return runtimeCanonicalJSONInteger(value, 0, maximum)
}

func runtimeCanonicalJSONInteger(value json.Number, minimum, maximum uint64) (uint64, error) {
	raw := value.String()
	if !runtimeUnsignedIntegerPattern.MatchString(raw) {
		return 0, errRuntimeNoncanonical
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errRuntimeOutOfRange
	}
	return parsed, nil
}

func runtimeCanonicalUint64String(value string) (uint64, error) {
	if !runtimeUnsignedIntegerPattern.MatchString(value) {
		return 0, errRuntimeNoncanonical
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, errRuntimeOutOfRange
	}
	return parsed, nil
}

func jsonNumberEquals(value json.Number, expected uint64) bool {
	parsed, err := runtimeCanonicalJSONInteger(value, 0, runtimeAuthorityMaxJSONInteger)
	return err == nil && parsed == expected
}

func runtimeSHA256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var (
	errRuntimeUnsupportedSchema = errors.New("unsupported authority schema")
	errRuntimeNoncanonical      = errors.New("noncanonical authority value")
	errRuntimeConflict          = errors.New("conflicting authority value")
	errRuntimeIntegrityMismatch = errors.New("authority integrity mismatch")
	errRuntimeMixedSchema       = errors.New("mixed ledger schema")
	errRuntimeOutOfRange        = errors.New("authority value out of range")
)

func runtimeGuardValidationCode(err error) RuntimeAuthorityGuardCode {
	switch {
	case errors.Is(err, errRuntimeUnsupportedSchema):
		return RuntimeAuthorityGuardUnsupportedSchema
	case errors.Is(err, errRuntimeNoncanonical):
		return RuntimeAuthorityGuardNoncanonical
	case errors.Is(err, errRuntimeConflict):
		return RuntimeAuthorityGuardConflict
	case errors.Is(err, errRuntimeIntegrityMismatch):
		return RuntimeAuthorityGuardIntegrityMismatch
	case errors.Is(err, errRuntimeMixedSchema):
		return RuntimeAuthorityGuardMixedSchema
	case errors.Is(err, errRuntimeOutOfRange):
		return RuntimeAuthorityGuardOutOfRange
	default:
		return RuntimeAuthorityGuardMalformed
	}
}

func runtimeGuardDecodeError(stage RuntimeAuthorityGuardStage, root, path string, err error) error {
	code := RuntimeAuthorityGuardMalformed
	var decodeErr runtimeDecodeError
	if errors.As(err, &decodeErr) {
		code = decodeErr.code
	}
	return runtimeGuardError(stage, code, runtimeGuardRelativePath(root, path), err)
}

func runtimeGuardFileError(stage RuntimeAuthorityGuardStage, root, path string, err error) error {
	code := RuntimeAuthorityGuardMalformed
	if errors.Is(err, errRuntimeOutOfRange) {
		code = RuntimeAuthorityGuardOutOfRange
	} else if errors.Is(err, os.ErrNotExist) {
		code = RuntimeAuthorityGuardMissing
	}
	return runtimeGuardError(stage, code, runtimeGuardRelativePath(root, path), err)
}

func runtimeGuardError(stage RuntimeAuthorityGuardStage, code RuntimeAuthorityGuardCode, relativePath string, err error) error {
	return &RuntimeAuthorityGuardError{Stage: stage, Code: code, RelativePath: relativePath, Err: err}
}

func runtimeGuardRelativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
