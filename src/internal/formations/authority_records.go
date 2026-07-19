package formations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	authorityCommandIDPattern = regexp.MustCompile(`^cmd_[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	authorityRunIDPattern     = regexp.MustCompile(`^run_[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	authorityStableIDPattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	authorityCodePattern      = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

type workspaceRegistryJCSV1 struct {
	Entries         []workspaceRegistryEntryJCSV1
	PriorGeneration *authorityGeneration
	RecordRev       uint64
	RegistrySchema  uint64
}

type workspaceRegistryEntryJCSV1 struct {
	ConfiguredPath              string
	Device                      string
	Inode                       string
	WorkspaceAuthorityID        string
	WorkspaceRootIdentitySHA256 string
}

type workspaceAdmissionPolicyRefJCSV1 struct {
	PolicyRev    uint64
	PolicySHA256 string
}

type workspaceAuthorityJCSV1 struct {
	AdmissionPolicyRef          workspaceAdmissionPolicyRefJCSV1
	AuthoritySchema             uint64
	NextAdmissionSeq            uint64
	NextWriterFence             uint64
	PriorGeneration             *authorityGeneration
	RecordRev                   uint64
	RootIdentityEncoding        string
	WorkspaceAuthorityID        string
	WorkspaceRootIdentitySHA256 string
}

type workspaceOwnerLeaseJCSV1 struct {
	AcquiredAt           string
	LeaseSchema          uint64
	LeaseUntil           string
	OwnerInstanceID      string
	PriorGeneration      *authorityGeneration
	RecordRev            uint64
	RenewedAt            string
	WorkspaceAuthorityID string
	WriterFence          uint64
}

type runCommandRecordJCSV1 struct {
	AdmittedWriterFence        uint64
	CommandEncoding            string
	CommandID                  string
	CommandKind                string
	CommandPayload             runCommandPayloadJCSV1
	CommandPayloadSHA256       string
	CommandSchema              uint64
	DecisionAdmissionPolicyRef *workspaceAdmissionPolicyRefJCSV1
	EffectSeq                  uint64
	OutcomeWriterFence         uint64
	PriorGeneration            *authorityGeneration
	RecordRev                  uint64
	RejectionCode              string
	RunID                      string
	State                      string
	StateWriterFence           uint64
}

type runCommandPayloadJCSV1 interface {
	authorityCommandKind() string
}

type runCommandStartJCSV1 struct {
	Actor                string
	AuthoritySchema      uint64
	BoardID              string
	ExpectedBoardETag    string
	ExpectedBoardRev     uint64
	Kind                 string
	Limits               runCommandLimitsJCSV1
	RunRoot              runCommandRootJCSV1
	WorkspaceAuthorityID string
}

func (runCommandStartJCSV1) authorityCommandKind() string { return "start" }

type runCommandResumeJCSV1 struct {
	Actor                string
	AuthoritySchema      uint64
	BlockedSeq           uint64
	Kind                 string
	Reason               string
	ResumeMode           string
	RunID                string
	WorkspaceAuthorityID string
}

func (runCommandResumeJCSV1) authorityCommandKind() string { return "resume" }

type runCommandCancelJCSV1 struct {
	Actor                string
	AuthoritySchema      uint64
	ExpectedLastSeq      uint64
	Kind                 string
	Reason               string
	RunID                string
	WorkspaceAuthorityID string
}

func (runCommandCancelJCSV1) authorityCommandKind() string { return "cancel" }

type runCommandVerdictJCSV1 struct {
	Actor                string
	AuthoritySchema      uint64
	GateID               string
	Kind                 string
	Reason               string
	RequestedSeq         uint64
	RunID                string
	Verdict              string
	WorkspaceAuthorityID string
}

func (runCommandVerdictJCSV1) authorityCommandKind() string { return "verdict" }

type runCommandLimitsJCSV1 struct {
	MaxAttempts      uint64
	MaxDispatch      uint64
	Redact           bool
	WallClockSeconds uint64
}

type runCommandRootJCSV1 struct {
	Kind   string
	NodeID string
}

type authorityGenerationWire struct {
	RecordRev uint64 `json:"recordRev"`
	SHA256    string `json:"sha256"`
}

type workspaceRegistryWire struct {
	Entries         *[]workspaceRegistryEntryWire `json:"entries"`
	PriorGeneration json.RawMessage               `json:"priorGeneration"`
	RecordRev       uint64                        `json:"recordRev"`
	RegistrySchema  uint64                        `json:"registrySchema"`
}

type workspaceRegistryEntryWire struct {
	ConfiguredPath              string `json:"configuredPath"`
	Device                      string `json:"device"`
	Inode                       string `json:"inode"`
	WorkspaceAuthorityID        string `json:"workspaceAuthorityId"`
	WorkspaceRootIdentitySHA256 string `json:"workspaceRootIdentitySha256"`
}

type workspaceAdmissionPolicyRefWire struct {
	PolicyRev    uint64 `json:"policyRev"`
	PolicySHA256 string `json:"policySha256"`
}

type workspaceAuthorityWire struct {
	AdmissionPolicyRef          *workspaceAdmissionPolicyRefWire `json:"admissionPolicyRef"`
	AuthoritySchema             uint64                           `json:"authoritySchema"`
	NextAdmissionSeq            uint64                           `json:"nextAdmissionSeq"`
	NextWriterFence             uint64                           `json:"nextWriterFence"`
	PriorGeneration             json.RawMessage                  `json:"priorGeneration"`
	RecordRev                   uint64                           `json:"recordRev"`
	RootIdentityEncoding        string                           `json:"rootIdentityEncoding"`
	WorkspaceAuthorityID        string                           `json:"workspaceAuthorityId"`
	WorkspaceRootIdentitySHA256 string                           `json:"workspaceRootIdentitySha256"`
}

type workspaceOwnerLeaseWire struct {
	AcquiredAt           string          `json:"acquiredAt"`
	LeaseSchema          uint64          `json:"leaseSchema"`
	LeaseUntil           string          `json:"leaseUntil"`
	OwnerInstanceID      string          `json:"ownerInstanceId"`
	PriorGeneration      json.RawMessage `json:"priorGeneration"`
	RecordRev            uint64          `json:"recordRev"`
	RenewedAt            string          `json:"renewedAt"`
	WorkspaceAuthorityID string          `json:"workspaceAuthorityId"`
	WriterFence          uint64          `json:"writerFence"`
}

type runCommandRecordWire struct {
	AdmittedWriterFence        uint64          `json:"admittedWriterFence"`
	CommandEncoding            string          `json:"commandEncoding"`
	CommandID                  string          `json:"commandId"`
	CommandKind                string          `json:"commandKind"`
	CommandPayload             json.RawMessage `json:"commandPayload"`
	CommandPayloadSHA256       string          `json:"commandPayloadSha256"`
	CommandSchema              uint64          `json:"commandSchema"`
	DecisionAdmissionPolicyRef json.RawMessage `json:"decisionAdmissionPolicyRef"`
	EffectSeq                  *uint64         `json:"effectSeq"`
	OutcomeWriterFence         *uint64         `json:"outcomeWriterFence"`
	PriorGeneration            json.RawMessage `json:"priorGeneration"`
	RecordRev                  uint64          `json:"recordRev"`
	RejectionCode              *string         `json:"rejectionCode"`
	RunID                      *string         `json:"runId"`
	State                      string          `json:"state"`
	StateWriterFence           uint64          `json:"stateWriterFence"`
}

type runCommandStartWire struct {
	Actor                string                `json:"actor"`
	AuthoritySchema      uint64                `json:"authoritySchema"`
	BoardID              string                `json:"boardId"`
	ExpectedBoardETag    string                `json:"expectedBoardETag"`
	ExpectedBoardRev     uint64                `json:"expectedBoardRev"`
	Kind                 string                `json:"kind"`
	Limits               *runCommandLimitsWire `json:"limits"`
	RunRoot              *runCommandRootWire   `json:"runRoot"`
	WorkspaceAuthorityID string                `json:"workspaceAuthorityId"`
}

type runCommandResumeWire struct {
	Actor                string `json:"actor"`
	AuthoritySchema      uint64 `json:"authoritySchema"`
	BlockedSeq           uint64 `json:"blockedSeq"`
	Kind                 string `json:"kind"`
	Reason               string `json:"reason"`
	ResumeMode           string `json:"resumeMode"`
	RunID                string `json:"runId"`
	WorkspaceAuthorityID string `json:"workspaceAuthorityId"`
}

type runCommandCancelWire struct {
	Actor                string `json:"actor"`
	AuthoritySchema      uint64 `json:"authoritySchema"`
	ExpectedLastSeq      uint64 `json:"expectedLastSeq"`
	Kind                 string `json:"kind"`
	Reason               string `json:"reason"`
	RunID                string `json:"runId"`
	WorkspaceAuthorityID string `json:"workspaceAuthorityId"`
}

type runCommandVerdictWire struct {
	Actor                string `json:"actor"`
	AuthoritySchema      uint64 `json:"authoritySchema"`
	GateID               string `json:"gateId"`
	Kind                 string `json:"kind"`
	Reason               string `json:"reason"`
	RequestedSeq         uint64 `json:"requestedSeq"`
	RunID                string `json:"runId"`
	Verdict              string `json:"verdict"`
	WorkspaceAuthorityID string `json:"workspaceAuthorityId"`
}

type runCommandLimitsWire struct {
	MaxAttempts      uint64 `json:"maxAttempts"`
	MaxDispatch      uint64 `json:"maxDispatch"`
	Redact           *bool  `json:"redact"`
	WallClockSeconds uint64 `json:"wallClockSeconds"`
}

type runCommandRootWire struct {
	Kind   string `json:"kind"`
	NodeID string `json:"nodeId"`
}

func decodeWorkspaceRegistryJCSV1(raw []byte) (workspaceRegistryJCSV1, error) {
	var wire workspaceRegistryWire
	if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
		return workspaceRegistryJCSV1{}, err
	}
	if wire.Entries == nil {
		return workspaceRegistryJCSV1{}, authorityRecordInvalid("registry entries must be an array")
	}
	prior, err := decodeAuthorityPriorGeneration(wire.PriorGeneration, wire.RecordRev)
	if err != nil {
		return workspaceRegistryJCSV1{}, err
	}
	record := workspaceRegistryJCSV1{
		Entries:         make([]workspaceRegistryEntryJCSV1, len(*wire.Entries)),
		PriorGeneration: prior,
		RecordRev:       wire.RecordRev,
		RegistrySchema:  wire.RegistrySchema,
	}
	for i, entry := range *wire.Entries {
		record.Entries[i] = workspaceRegistryEntryJCSV1{
			ConfiguredPath:              entry.ConfiguredPath,
			Device:                      entry.Device,
			Inode:                       entry.Inode,
			WorkspaceAuthorityID:        entry.WorkspaceAuthorityID,
			WorkspaceRootIdentitySHA256: entry.WorkspaceRootIdentitySHA256,
		}
	}
	encoded, err := encodeWorkspaceRegistryJCSV1(record)
	if err != nil {
		return workspaceRegistryJCSV1{}, err
	}
	if !bytes.Equal(raw, encoded) {
		return workspaceRegistryJCSV1{}, authorityRecordNoncanonical("workspace registry bytes")
	}
	return record, nil
}

func encodeWorkspaceRegistryJCSV1(record workspaceRegistryJCSV1) ([]byte, error) {
	if err := validateWorkspaceRegistryJCSV1(record); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString(`{"entries":[`)
	for i, entry := range record.Entries {
		if i != 0 {
			output.WriteByte(',')
		}
		output.WriteString(`{"configuredPath":`)
		writeRuntimeCanonicalJSONString(&output, entry.ConfiguredPath)
		output.WriteString(`,"device":`)
		writeRuntimeCanonicalJSONString(&output, entry.Device)
		output.WriteString(`,"inode":`)
		writeRuntimeCanonicalJSONString(&output, entry.Inode)
		output.WriteString(`,"workspaceAuthorityId":`)
		writeRuntimeCanonicalJSONString(&output, entry.WorkspaceAuthorityID)
		output.WriteString(`,"workspaceRootIdentitySha256":`)
		writeRuntimeCanonicalJSONString(&output, entry.WorkspaceRootIdentitySHA256)
		output.WriteByte('}')
	}
	output.WriteString(`],"priorGeneration":`)
	writeAuthorityPriorGeneration(&output, record.PriorGeneration)
	output.WriteString(`,"recordRev":`)
	writeAuthorityUint(&output, record.RecordRev)
	output.WriteString(`,"registrySchema":1}`)
	return output.Bytes(), nil
}

func decodeWorkspaceAuthorityJCSV1(raw []byte) (workspaceAuthorityJCSV1, error) {
	var wire workspaceAuthorityWire
	if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
		return workspaceAuthorityJCSV1{}, err
	}
	if wire.AdmissionPolicyRef == nil {
		return workspaceAuthorityJCSV1{}, authorityRecordInvalid("admission policy ref is required")
	}
	prior, err := decodeAuthorityPriorGeneration(wire.PriorGeneration, wire.RecordRev)
	if err != nil {
		return workspaceAuthorityJCSV1{}, err
	}
	record := workspaceAuthorityJCSV1{
		AdmissionPolicyRef: workspaceAdmissionPolicyRefJCSV1{
			PolicyRev:    wire.AdmissionPolicyRef.PolicyRev,
			PolicySHA256: wire.AdmissionPolicyRef.PolicySHA256,
		},
		AuthoritySchema:             wire.AuthoritySchema,
		NextAdmissionSeq:            wire.NextAdmissionSeq,
		NextWriterFence:             wire.NextWriterFence,
		PriorGeneration:             prior,
		RecordRev:                   wire.RecordRev,
		RootIdentityEncoding:        wire.RootIdentityEncoding,
		WorkspaceAuthorityID:        wire.WorkspaceAuthorityID,
		WorkspaceRootIdentitySHA256: wire.WorkspaceRootIdentitySHA256,
	}
	encoded, err := encodeWorkspaceAuthorityJCSV1(record)
	if err != nil {
		return workspaceAuthorityJCSV1{}, err
	}
	if !bytes.Equal(raw, encoded) {
		return workspaceAuthorityJCSV1{}, authorityRecordNoncanonical("workspace authority bytes")
	}
	return record, nil
}

func encodeWorkspaceAuthorityJCSV1(record workspaceAuthorityJCSV1) ([]byte, error) {
	if err := validateWorkspaceAuthorityJCSV1(record); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString(`{"admissionPolicyRef":`)
	writeWorkspaceAdmissionPolicyRef(&output, record.AdmissionPolicyRef)
	output.WriteString(`,"authoritySchema":2,"nextAdmissionSeq":`)
	writeAuthorityUint(&output, record.NextAdmissionSeq)
	output.WriteString(`,"nextWriterFence":`)
	writeAuthorityUint(&output, record.NextWriterFence)
	output.WriteString(`,"priorGeneration":`)
	writeAuthorityPriorGeneration(&output, record.PriorGeneration)
	output.WriteString(`,"recordRev":`)
	writeAuthorityUint(&output, record.RecordRev)
	output.WriteString(`,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":`)
	writeRuntimeCanonicalJSONString(&output, record.WorkspaceAuthorityID)
	output.WriteString(`,"workspaceRootIdentitySha256":`)
	writeRuntimeCanonicalJSONString(&output, record.WorkspaceRootIdentitySHA256)
	output.WriteByte('}')
	return output.Bytes(), nil
}

func decodeWorkspaceOwnerLeaseJCSV1(raw []byte) (workspaceOwnerLeaseJCSV1, error) {
	var wire workspaceOwnerLeaseWire
	if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
		return workspaceOwnerLeaseJCSV1{}, err
	}
	prior, err := decodeAuthorityPriorGeneration(wire.PriorGeneration, wire.RecordRev)
	if err != nil {
		return workspaceOwnerLeaseJCSV1{}, err
	}
	record := workspaceOwnerLeaseJCSV1{
		AcquiredAt:           wire.AcquiredAt,
		LeaseSchema:          wire.LeaseSchema,
		LeaseUntil:           wire.LeaseUntil,
		OwnerInstanceID:      wire.OwnerInstanceID,
		PriorGeneration:      prior,
		RecordRev:            wire.RecordRev,
		RenewedAt:            wire.RenewedAt,
		WorkspaceAuthorityID: wire.WorkspaceAuthorityID,
		WriterFence:          wire.WriterFence,
	}
	encoded, err := encodeWorkspaceOwnerLeaseJCSV1(record)
	if err != nil {
		return workspaceOwnerLeaseJCSV1{}, err
	}
	if !bytes.Equal(raw, encoded) {
		return workspaceOwnerLeaseJCSV1{}, authorityRecordNoncanonical("workspace owner lease bytes")
	}
	return record, nil
}

func encodeWorkspaceOwnerLeaseJCSV1(record workspaceOwnerLeaseJCSV1) ([]byte, error) {
	if err := validateWorkspaceOwnerLeaseJCSV1(record); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString(`{"acquiredAt":`)
	writeRuntimeCanonicalJSONString(&output, record.AcquiredAt)
	output.WriteString(`,"leaseSchema":1,"leaseUntil":`)
	writeRuntimeCanonicalJSONString(&output, record.LeaseUntil)
	output.WriteString(`,"ownerInstanceId":`)
	writeRuntimeCanonicalJSONString(&output, record.OwnerInstanceID)
	output.WriteString(`,"priorGeneration":`)
	writeAuthorityPriorGeneration(&output, record.PriorGeneration)
	output.WriteString(`,"recordRev":`)
	writeAuthorityUint(&output, record.RecordRev)
	output.WriteString(`,"renewedAt":`)
	writeRuntimeCanonicalJSONString(&output, record.RenewedAt)
	output.WriteString(`,"workspaceAuthorityId":`)
	writeRuntimeCanonicalJSONString(&output, record.WorkspaceAuthorityID)
	output.WriteString(`,"writerFence":`)
	writeAuthorityUint(&output, record.WriterFence)
	output.WriteByte('}')
	return output.Bytes(), nil
}

func decodeRunCommandRecordJCSV1(raw []byte) (runCommandRecordJCSV1, error) {
	var wire runCommandRecordWire
	if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
		return runCommandRecordJCSV1{}, err
	}
	prior, err := decodeAuthorityPriorGeneration(wire.PriorGeneration, wire.RecordRev)
	if err != nil {
		return runCommandRecordJCSV1{}, err
	}
	payload, payloadRaw, err := decodeRunCommandPayloadJCSV1(wire.CommandKind, wire.CommandPayload)
	if err != nil {
		return runCommandRecordJCSV1{}, err
	}
	record := runCommandRecordJCSV1{
		AdmittedWriterFence:  wire.AdmittedWriterFence,
		CommandEncoding:      wire.CommandEncoding,
		CommandID:            wire.CommandID,
		CommandKind:          wire.CommandKind,
		CommandPayload:       payload,
		CommandPayloadSHA256: wire.CommandPayloadSHA256,
		CommandSchema:        wire.CommandSchema,
		PriorGeneration:      prior,
		RecordRev:            wire.RecordRev,
		State:                wire.State,
		StateWriterFence:     wire.StateWriterFence,
	}
	if wire.EffectSeq != nil {
		record.EffectSeq = *wire.EffectSeq
	}
	if wire.OutcomeWriterFence != nil {
		record.OutcomeWriterFence = *wire.OutcomeWriterFence
	}
	if wire.RejectionCode != nil {
		record.RejectionCode = *wire.RejectionCode
	}
	if wire.RunID != nil {
		record.RunID = *wire.RunID
	}
	decisionPresent := len(wire.DecisionAdmissionPolicyRef) != 0
	if decisionPresent && !bytes.Equal(wire.DecisionAdmissionPolicyRef, []byte("null")) {
		var decision workspaceAdmissionPolicyRefWire
		if err := decodeAuthorityRecordJSON(wire.DecisionAdmissionPolicyRef, &decision); err != nil {
			return runCommandRecordJCSV1{}, err
		}
		record.DecisionAdmissionPolicyRef = &workspaceAdmissionPolicyRefJCSV1{
			PolicyRev: decision.PolicyRev, PolicySHA256: decision.PolicySHA256,
		}
	}
	if err := validateRunCommandRecordPresence(record, wire, decisionPresent); err != nil {
		return runCommandRecordJCSV1{}, err
	}
	if runtimeSHA256Hex(payloadRaw) != record.CommandPayloadSHA256 {
		return runCommandRecordJCSV1{}, fmt.Errorf("%w: command payload hash", errRuntimeIntegrityMismatch)
	}
	encoded, err := encodeRunCommandRecordJCSV1(record)
	if err != nil {
		return runCommandRecordJCSV1{}, err
	}
	if !bytes.Equal(raw, encoded) {
		return runCommandRecordJCSV1{}, authorityRecordNoncanonical("run command record bytes")
	}
	return record, nil
}

func encodeRunCommandRecordJCSV1(record runCommandRecordJCSV1) ([]byte, error) {
	payload, err := encodeRunCommandPayloadJCSV1(record.CommandPayload)
	if err != nil {
		return nil, err
	}
	if err := validateRunCommandRecordJCSV1(record, payload); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString(`{"admittedWriterFence":`)
	writeAuthorityUint(&output, record.AdmittedWriterFence)
	output.WriteString(`,"commandEncoding":"run-command-jcs-v1","commandId":`)
	writeRuntimeCanonicalJSONString(&output, record.CommandID)
	output.WriteString(`,"commandKind":`)
	writeRuntimeCanonicalJSONString(&output, record.CommandKind)
	output.WriteString(`,"commandPayload":`)
	output.Write(payload)
	output.WriteString(`,"commandPayloadSha256":`)
	writeRuntimeCanonicalJSONString(&output, record.CommandPayloadSHA256)
	output.WriteString(`,"commandSchema":1`)
	switch record.State {
	case "applied":
		output.WriteString(`,"decisionAdmissionPolicyRef":`)
		writeNullableWorkspaceAdmissionPolicyRef(&output, record.DecisionAdmissionPolicyRef)
		output.WriteString(`,"effectSeq":`)
		writeAuthorityUint(&output, record.EffectSeq)
		output.WriteString(`,"outcomeWriterFence":`)
		writeAuthorityUint(&output, record.OutcomeWriterFence)
	case "rejected":
		output.WriteString(`,"decisionAdmissionPolicyRef":`)
		writeNullableWorkspaceAdmissionPolicyRef(&output, record.DecisionAdmissionPolicyRef)
		output.WriteString(`,"outcomeWriterFence":`)
		writeAuthorityUint(&output, record.OutcomeWriterFence)
	}
	output.WriteString(`,"priorGeneration":`)
	writeAuthorityPriorGeneration(&output, record.PriorGeneration)
	output.WriteString(`,"recordRev":`)
	writeAuthorityUint(&output, record.RecordRev)
	if record.State == "rejected" {
		output.WriteString(`,"rejectionCode":`)
		writeRuntimeCanonicalJSONString(&output, record.RejectionCode)
	}
	if record.State == "applied" {
		output.WriteString(`,"runId":`)
		writeRuntimeCanonicalJSONString(&output, record.RunID)
	}
	output.WriteString(`,"state":`)
	writeRuntimeCanonicalJSONString(&output, record.State)
	output.WriteString(`,"stateWriterFence":`)
	writeAuthorityUint(&output, record.StateWriterFence)
	output.WriteByte('}')
	return output.Bytes(), nil
}

func validateAuthorityRecordTransition(recordRev uint64, priorGeneration, expectedGeneration *authorityGeneration) error {
	if err := validateAuthorityRecordLink(recordRev, priorGeneration); err != nil {
		return err
	}
	if recordRev == 1 {
		if expectedGeneration != nil {
			return fmt.Errorf("%w: initial record cannot name an expected predecessor", errRuntimeConflict)
		}
		return nil
	}
	if expectedGeneration == nil {
		return fmt.Errorf("%w: expected predecessor is required", errRuntimeConflict)
	}
	if expectedGeneration.recordRev == 0 || expectedGeneration.recordRev > runtimeAuthorityMaxJSONInteger || !runtimeSHA256Pattern.MatchString(expectedGeneration.sha256) {
		return authorityRecordNoncanonical("expected predecessor generation")
	}
	if *priorGeneration != *expectedGeneration {
		return fmt.Errorf("%w: predecessor generation mismatch", errRuntimeIntegrityMismatch)
	}
	return nil
}

func validateWorkspaceRegistryJCSV1(record workspaceRegistryJCSV1) error {
	if record.RegistrySchema != 1 {
		return errRuntimeUnsupportedSchema
	}
	if record.Entries == nil {
		return authorityRecordInvalid("registry entries must not be nil")
	}
	if err := validateAuthorityRecordLink(record.RecordRev, record.PriorGeneration); err != nil {
		return err
	}
	configuredPaths := map[string]bool{}
	openedRoots := map[string]bool{}
	authorityIDs := map[string]bool{}
	rootHashes := map[string]bool{}
	var previousDevice, previousInode uint64
	var previousPath string
	for index, entry := range record.Entries {
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
		if !runtimeWorkspaceAuthorityIDPattern.MatchString(entry.WorkspaceAuthorityID) || !runtimeSHA256Pattern.MatchString(entry.WorkspaceRootIdentitySHA256) {
			return authorityRecordNoncanonical("registry identity")
		}
		opened := entry.Device + ":" + entry.Inode
		if configuredPaths[entry.ConfiguredPath] || openedRoots[opened] || authorityIDs[entry.WorkspaceAuthorityID] || rootHashes[entry.WorkspaceRootIdentitySHA256] {
			return fmt.Errorf("%w: conflicting workspace registry entry", errRuntimeConflict)
		}
		configuredPaths[entry.ConfiguredPath] = true
		openedRoots[opened] = true
		authorityIDs[entry.WorkspaceAuthorityID] = true
		rootHashes[entry.WorkspaceRootIdentitySHA256] = true
		if index > 0 && !authorityRegistryEntryOrder(previousDevice, previousInode, previousPath, device, inode, entry.ConfiguredPath) {
			return authorityRecordNoncanonical("registry entry order")
		}
		previousDevice, previousInode, previousPath = device, inode, entry.ConfiguredPath
	}
	return nil
}

func authorityRegistryEntryOrder(leftDevice, leftInode uint64, leftPath string, rightDevice, rightInode uint64, rightPath string) bool {
	if leftDevice != rightDevice {
		return leftDevice < rightDevice
	}
	if leftInode != rightInode {
		return leftInode < rightInode
	}
	return leftPath < rightPath
}

func validateWorkspaceAuthorityJCSV1(record workspaceAuthorityJCSV1) error {
	if record.AuthoritySchema != runtimeAuthoritySchema {
		return errRuntimeUnsupportedSchema
	}
	if err := validateAuthorityRecordLink(record.RecordRev, record.PriorGeneration); err != nil {
		return err
	}
	if record.RootIdentityEncoding != "workspace-root-identity-v1" || !runtimeWorkspaceAuthorityIDPattern.MatchString(record.WorkspaceAuthorityID) || !runtimeSHA256Pattern.MatchString(record.WorkspaceRootIdentitySHA256) {
		return authorityRecordNoncanonical("workspace authority identity")
	}
	if err := validateAuthorityPositiveSafeInteger(record.NextAdmissionSeq, "next admission sequence"); err != nil {
		return err
	}
	if err := validateAuthorityPositiveSafeInteger(record.NextWriterFence, "next writer fence"); err != nil {
		return err
	}
	return validateWorkspaceAdmissionPolicyRef(record.AdmissionPolicyRef)
}

func validateWorkspaceOwnerLeaseJCSV1(record workspaceOwnerLeaseJCSV1) error {
	if record.LeaseSchema != 1 {
		return errRuntimeUnsupportedSchema
	}
	if err := validateAuthorityRecordLink(record.RecordRev, record.PriorGeneration); err != nil {
		return err
	}
	if !runtimeWorkspaceAuthorityIDPattern.MatchString(record.WorkspaceAuthorityID) || !authorityOpaqueToken(record.OwnerInstanceID) {
		return authorityRecordNoncanonical("owner lease identity")
	}
	if err := validateAuthorityPositiveSafeInteger(record.WriterFence, "writer fence"); err != nil {
		return err
	}
	acquired, err := parseAuthorityCanonicalTimestamp(record.AcquiredAt)
	if err != nil {
		return err
	}
	renewed, err := parseAuthorityCanonicalTimestamp(record.RenewedAt)
	if err != nil {
		return err
	}
	leaseUntil, err := parseAuthorityCanonicalTimestamp(record.LeaseUntil)
	if err != nil {
		return err
	}
	if acquired.After(renewed) || !renewed.Before(leaseUntil) {
		return fmt.Errorf("%w: lease time order", errRuntimeConflict)
	}
	return nil
}

func validateRunCommandRecordPresence(record runCommandRecordJCSV1, wire runCommandRecordWire, decisionPresent bool) error {
	switch record.State {
	case "pending":
		if decisionPresent || wire.EffectSeq != nil || wire.OutcomeWriterFence != nil || wire.RejectionCode != nil || wire.RunID != nil {
			return fmt.Errorf("%w: pending command has outcome fields", errRuntimeConflict)
		}
	case "applied":
		if !decisionPresent || wire.EffectSeq == nil || wire.OutcomeWriterFence == nil || wire.RunID == nil || wire.RejectionCode != nil {
			return fmt.Errorf("%w: applied command outcome shape", errRuntimeConflict)
		}
	case "rejected":
		if !decisionPresent || wire.OutcomeWriterFence == nil || wire.RejectionCode == nil || wire.EffectSeq != nil || wire.RunID != nil {
			return fmt.Errorf("%w: rejected command outcome shape", errRuntimeConflict)
		}
	default:
		return authorityRecordNoncanonical("command state")
	}
	return nil
}

func validateRunCommandRecordJCSV1(record runCommandRecordJCSV1, canonicalPayload []byte) error {
	if record.CommandSchema != 1 || record.CommandEncoding != "run-command-jcs-v1" {
		return errRuntimeUnsupportedSchema
	}
	if err := validateAuthorityRecordLink(record.RecordRev, record.PriorGeneration); err != nil {
		return err
	}
	if !authorityCommandIDPattern.MatchString(record.CommandID) || record.CommandPayload == nil || record.CommandKind != record.CommandPayload.authorityCommandKind() {
		return authorityRecordNoncanonical("command identity or kind")
	}
	if !runtimeSHA256Pattern.MatchString(record.CommandPayloadSHA256) || runtimeSHA256Hex(canonicalPayload) != record.CommandPayloadSHA256 {
		return fmt.Errorf("%w: command payload hash", errRuntimeIntegrityMismatch)
	}
	if err := validateAuthorityPositiveSafeInteger(record.AdmittedWriterFence, "admitted writer fence"); err != nil {
		return err
	}
	if err := validateAuthorityPositiveSafeInteger(record.StateWriterFence, "state writer fence"); err != nil {
		return err
	}
	if record.StateWriterFence < record.AdmittedWriterFence {
		return fmt.Errorf("%w: state fence precedes admission fence", errRuntimeConflict)
	}
	switch record.State {
	case "pending":
		if (record.RecordRev == 1 && record.StateWriterFence != record.AdmittedWriterFence) || record.DecisionAdmissionPolicyRef != nil || record.EffectSeq != 0 || record.OutcomeWriterFence != 0 || record.RejectionCode != "" || record.RunID != "" {
			return fmt.Errorf("%w: pending command state", errRuntimeConflict)
		}
	case "applied":
		if record.RecordRev == 1 || record.EffectSeq == 0 || record.OutcomeWriterFence == 0 || !authorityRunIDPattern.MatchString(record.RunID) || record.RejectionCode != "" {
			return fmt.Errorf("%w: applied command state", errRuntimeConflict)
		}
		if err := validateAuthorityPositiveSafeInteger(record.EffectSeq, "effect sequence"); err != nil {
			return err
		}
		if err := validateAuthorityOutcome(record); err != nil {
			return err
		}
	case "rejected":
		if record.RecordRev == 1 || record.EffectSeq != 0 || record.OutcomeWriterFence == 0 || record.RunID != "" || !authorityCodePattern.MatchString(record.RejectionCode) {
			return fmt.Errorf("%w: rejected command state", errRuntimeConflict)
		}
		if err := validateAuthorityOutcome(record); err != nil {
			return err
		}
	default:
		return authorityRecordNoncanonical("command state")
	}
	return nil
}

func validateAuthorityOutcome(record runCommandRecordJCSV1) error {
	if err := validateAuthorityPositiveSafeInteger(record.OutcomeWriterFence, "outcome writer fence"); err != nil {
		return err
	}
	if record.OutcomeWriterFence < record.AdmittedWriterFence || record.OutcomeWriterFence > record.StateWriterFence {
		return fmt.Errorf("%w: outcome fence is outside the admitted state interval", errRuntimeConflict)
	}
	if record.CommandKind == "start" {
		if record.DecisionAdmissionPolicyRef == nil {
			return fmt.Errorf("%w: start outcome lacks policy generation", errRuntimeConflict)
		}
		return validateWorkspaceAdmissionPolicyRef(*record.DecisionAdmissionPolicyRef)
	}
	if record.DecisionAdmissionPolicyRef != nil {
		return fmt.Errorf("%w: non-start outcome names policy generation", errRuntimeConflict)
	}
	if payloadRunID(record.CommandPayload) != record.RunID && record.State == "applied" {
		return fmt.Errorf("%w: applied run identity differs from command", errRuntimeConflict)
	}
	return nil
}

func decodeRunCommandPayloadJCSV1(kind string, raw []byte) (runCommandPayloadJCSV1, []byte, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil, authorityRecordInvalid("command payload is required")
	}
	var payload runCommandPayloadJCSV1
	switch kind {
	case "start":
		var wire runCommandStartWire
		if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
			return nil, nil, err
		}
		if wire.Limits == nil || wire.Limits.Redact == nil || wire.RunRoot == nil {
			return nil, nil, authorityRecordInvalid("start command nested shape")
		}
		payload = runCommandStartJCSV1{
			Actor: wire.Actor, AuthoritySchema: wire.AuthoritySchema, BoardID: wire.BoardID,
			ExpectedBoardETag: wire.ExpectedBoardETag, ExpectedBoardRev: wire.ExpectedBoardRev, Kind: wire.Kind,
			Limits:  runCommandLimitsJCSV1{MaxAttempts: wire.Limits.MaxAttempts, MaxDispatch: wire.Limits.MaxDispatch, Redact: *wire.Limits.Redact, WallClockSeconds: wire.Limits.WallClockSeconds},
			RunRoot: runCommandRootJCSV1{Kind: wire.RunRoot.Kind, NodeID: wire.RunRoot.NodeID}, WorkspaceAuthorityID: wire.WorkspaceAuthorityID,
		}
	case "resume":
		var wire runCommandResumeWire
		if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
			return nil, nil, err
		}
		payload = runCommandResumeJCSV1{Actor: wire.Actor, AuthoritySchema: wire.AuthoritySchema, BlockedSeq: wire.BlockedSeq, Kind: wire.Kind, Reason: wire.Reason, ResumeMode: wire.ResumeMode, RunID: wire.RunID, WorkspaceAuthorityID: wire.WorkspaceAuthorityID}
	case "cancel":
		var wire runCommandCancelWire
		if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
			return nil, nil, err
		}
		payload = runCommandCancelJCSV1{Actor: wire.Actor, AuthoritySchema: wire.AuthoritySchema, ExpectedLastSeq: wire.ExpectedLastSeq, Kind: wire.Kind, Reason: wire.Reason, RunID: wire.RunID, WorkspaceAuthorityID: wire.WorkspaceAuthorityID}
	case "verdict":
		var wire runCommandVerdictWire
		if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
			return nil, nil, err
		}
		payload = runCommandVerdictJCSV1{Actor: wire.Actor, AuthoritySchema: wire.AuthoritySchema, GateID: wire.GateID, Kind: wire.Kind, Reason: wire.Reason, RequestedSeq: wire.RequestedSeq, RunID: wire.RunID, Verdict: wire.Verdict, WorkspaceAuthorityID: wire.WorkspaceAuthorityID}
	default:
		return nil, nil, authorityRecordNoncanonical("command kind")
	}
	canonical, err := encodeRunCommandPayloadJCSV1(payload)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(raw, canonical) {
		return nil, nil, authorityRecordNoncanonical("command payload bytes")
	}
	return payload, canonical, nil
}

func encodeRunCommandPayloadJCSV1(payload runCommandPayloadJCSV1) ([]byte, error) {
	if payload == nil {
		return nil, authorityRecordInvalid("command payload is nil")
	}
	if err := validateRunCommandPayloadJCSV1(payload); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	switch value := payload.(type) {
	case runCommandStartJCSV1:
		output.WriteString(`{"actor":`)
		writeRuntimeCanonicalJSONString(&output, value.Actor)
		output.WriteString(`,"authoritySchema":2,"boardId":`)
		writeRuntimeCanonicalJSONString(&output, value.BoardID)
		output.WriteString(`,"expectedBoardETag":`)
		writeRuntimeCanonicalJSONString(&output, value.ExpectedBoardETag)
		output.WriteString(`,"expectedBoardRev":`)
		writeAuthorityUint(&output, value.ExpectedBoardRev)
		output.WriteString(`,"kind":"start","limits":{"maxAttempts":`)
		writeAuthorityUint(&output, value.Limits.MaxAttempts)
		output.WriteString(`,"maxDispatch":`)
		writeAuthorityUint(&output, value.Limits.MaxDispatch)
		output.WriteString(`,"redact":`)
		output.WriteString(strconv.FormatBool(value.Limits.Redact))
		output.WriteString(`,"wallClockSeconds":`)
		writeAuthorityUint(&output, value.Limits.WallClockSeconds)
		output.WriteString(`},"runRoot":{"kind":`)
		writeRuntimeCanonicalJSONString(&output, value.RunRoot.Kind)
		output.WriteString(`,"nodeId":`)
		writeRuntimeCanonicalJSONString(&output, value.RunRoot.NodeID)
		output.WriteString(`},"workspaceAuthorityId":`)
		writeRuntimeCanonicalJSONString(&output, value.WorkspaceAuthorityID)
		output.WriteByte('}')
	case runCommandResumeJCSV1:
		output.WriteString(`{"actor":`)
		writeRuntimeCanonicalJSONString(&output, value.Actor)
		output.WriteString(`,"authoritySchema":2,"blockedSeq":`)
		writeAuthorityUint(&output, value.BlockedSeq)
		output.WriteString(`,"kind":"resume","reason":`)
		writeRuntimeCanonicalJSONString(&output, value.Reason)
		output.WriteString(`,"resumeMode":`)
		writeRuntimeCanonicalJSONString(&output, value.ResumeMode)
		output.WriteString(`,"runId":`)
		writeRuntimeCanonicalJSONString(&output, value.RunID)
		output.WriteString(`,"workspaceAuthorityId":`)
		writeRuntimeCanonicalJSONString(&output, value.WorkspaceAuthorityID)
		output.WriteByte('}')
	case runCommandCancelJCSV1:
		output.WriteString(`{"actor":`)
		writeRuntimeCanonicalJSONString(&output, value.Actor)
		output.WriteString(`,"authoritySchema":2,"expectedLastSeq":`)
		writeAuthorityUint(&output, value.ExpectedLastSeq)
		output.WriteString(`,"kind":"cancel","reason":`)
		writeRuntimeCanonicalJSONString(&output, value.Reason)
		output.WriteString(`,"runId":`)
		writeRuntimeCanonicalJSONString(&output, value.RunID)
		output.WriteString(`,"workspaceAuthorityId":`)
		writeRuntimeCanonicalJSONString(&output, value.WorkspaceAuthorityID)
		output.WriteByte('}')
	case runCommandVerdictJCSV1:
		output.WriteString(`{"actor":`)
		writeRuntimeCanonicalJSONString(&output, value.Actor)
		output.WriteString(`,"authoritySchema":2,"gateId":`)
		writeRuntimeCanonicalJSONString(&output, value.GateID)
		output.WriteString(`,"kind":"verdict","reason":`)
		writeRuntimeCanonicalJSONString(&output, value.Reason)
		output.WriteString(`,"requestedSeq":`)
		writeAuthorityUint(&output, value.RequestedSeq)
		output.WriteString(`,"runId":`)
		writeRuntimeCanonicalJSONString(&output, value.RunID)
		output.WriteString(`,"verdict":`)
		writeRuntimeCanonicalJSONString(&output, value.Verdict)
		output.WriteString(`,"workspaceAuthorityId":`)
		writeRuntimeCanonicalJSONString(&output, value.WorkspaceAuthorityID)
		output.WriteByte('}')
	default:
		return nil, authorityRecordNoncanonical("command payload variant")
	}
	return output.Bytes(), nil
}

func validateRunCommandPayloadJCSV1(payload runCommandPayloadJCSV1) error {
	var actor, workspaceAuthorityID, kind string
	switch value := payload.(type) {
	case runCommandStartJCSV1:
		actor, workspaceAuthorityID, kind = value.Actor, value.WorkspaceAuthorityID, value.Kind
		if value.AuthoritySchema != runtimeAuthoritySchema || !authorityStableID(value.BoardID, "brd_") || value.ExpectedBoardRev == 0 || value.ExpectedBoardRev > runtimeAuthorityMaxJSONInteger {
			return authorityRecordNoncanonical("start command identity or revision")
		}
		if value.ExpectedBoardETag != "" && !runtimeSHA256Pattern.MatchString(value.ExpectedBoardETag) {
			return authorityRecordNoncanonical("expected board etag")
		}
		if err := validateAuthorityRunLimit(value.Limits.MaxAttempts, "max attempts"); err != nil {
			return err
		}
		if err := validateAuthorityRunLimit(value.Limits.MaxDispatch, "max dispatch"); err != nil {
			return err
		}
		if err := validateAuthorityRunLimit(value.Limits.WallClockSeconds, "wall clock seconds"); err != nil {
			return err
		}
		prefix := "mis_"
		if value.RunRoot.Kind == "formation" {
			prefix = "fmn_"
		} else if value.RunRoot.Kind != "mission" {
			return authorityRecordNoncanonical("run root kind")
		}
		if !authorityStableID(value.RunRoot.NodeID, prefix) {
			return authorityRecordNoncanonical("run root node id")
		}
	case runCommandResumeJCSV1:
		actor, workspaceAuthorityID, kind = value.Actor, value.WorkspaceAuthorityID, value.Kind
		if value.AuthoritySchema != runtimeAuthoritySchema || !authorityRunIDPattern.MatchString(value.RunID) || (value.ResumeMode != "reattach" && value.ResumeMode != "retry-failed-producer") {
			return authorityRecordNoncanonical("resume command domain")
		}
		if err := validateAuthorityPositiveSafeInteger(value.BlockedSeq, "blocked sequence"); err != nil {
			return err
		}
		if !authorityCanonicalReason(value.Reason) {
			return authorityRecordNoncanonical("resume reason")
		}
	case runCommandCancelJCSV1:
		actor, workspaceAuthorityID, kind = value.Actor, value.WorkspaceAuthorityID, value.Kind
		if value.AuthoritySchema != runtimeAuthoritySchema || !authorityRunIDPattern.MatchString(value.RunID) {
			return authorityRecordNoncanonical("cancel command domain")
		}
		if err := validateAuthorityPositiveSafeInteger(value.ExpectedLastSeq, "expected last sequence"); err != nil {
			return err
		}
		if !authorityCanonicalReason(value.Reason) {
			return authorityRecordNoncanonical("cancel reason")
		}
	case runCommandVerdictJCSV1:
		actor, workspaceAuthorityID, kind = value.Actor, value.WorkspaceAuthorityID, value.Kind
		if value.AuthoritySchema != runtimeAuthoritySchema || !authorityRunIDPattern.MatchString(value.RunID) || !authorityStableID(value.GateID, "gate_") || (value.Verdict != "pass" && value.Verdict != "fail") {
			return authorityRecordNoncanonical("verdict command domain")
		}
		if err := validateAuthorityPositiveSafeInteger(value.RequestedSeq, "requested sequence"); err != nil {
			return err
		}
		if !authorityCanonicalReason(value.Reason) {
			return authorityRecordNoncanonical("verdict reason")
		}
	default:
		return authorityRecordNoncanonical("command payload variant")
	}
	if kind != payload.authorityCommandKind() || !authorityCanonicalActor(actor) || !runtimeWorkspaceAuthorityIDPattern.MatchString(workspaceAuthorityID) {
		return authorityRecordNoncanonical("command payload authority")
	}
	return nil
}

func payloadRunID(payload runCommandPayloadJCSV1) string {
	switch value := payload.(type) {
	case runCommandResumeJCSV1:
		return value.RunID
	case runCommandCancelJCSV1:
		return value.RunID
	case runCommandVerdictJCSV1:
		return value.RunID
	default:
		return ""
	}
}

func decodeAuthorityPriorGeneration(raw []byte, recordRev uint64) (*authorityGeneration, error) {
	if len(raw) == 0 {
		return nil, authorityRecordInvalid("prior generation is required")
	}
	if bytes.Equal(raw, []byte("null")) {
		if err := validateAuthorityRecordLink(recordRev, nil); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var wire authorityGenerationWire
	if err := decodeAuthorityRecordJSON(raw, &wire); err != nil {
		return nil, err
	}
	prior := &authorityGeneration{recordRev: wire.RecordRev, sha256: wire.SHA256}
	if err := validateAuthorityRecordLink(recordRev, prior); err != nil {
		return nil, err
	}
	return prior, nil
}

func validateAuthorityRecordLink(recordRev uint64, prior *authorityGeneration) error {
	if err := validateAuthorityPositiveSafeInteger(recordRev, "record revision"); err != nil {
		return err
	}
	if recordRev == 1 {
		if prior != nil {
			return fmt.Errorf("%w: revision one has a predecessor", errRuntimeConflict)
		}
		return nil
	}
	if prior == nil || prior.recordRev != recordRev-1 {
		return fmt.Errorf("%w: record predecessor revision", errRuntimeConflict)
	}
	if prior.recordRev == 0 || prior.recordRev > runtimeAuthorityMaxJSONInteger || !runtimeSHA256Pattern.MatchString(prior.sha256) {
		return authorityRecordNoncanonical("record predecessor generation")
	}
	return nil
}

func decodeAuthorityRecordJSON(raw []byte, destination any) error {
	if int64(len(raw)) > runtimeAuthorityMaxRecordBytes {
		return fmt.Errorf("%w: authority record exceeds byte limit", errRuntimeOutOfRange)
	}
	if len(raw) == 0 || !utf8.Valid(raw) || !json.Valid(raw) {
		return authorityRecordInvalid("invalid authority JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return authorityRecordInvalid(err.Error())
	}
	if err := ensureAuthorityRecordJSONEOF(decoder); err != nil {
		return authorityRecordInvalid(err.Error())
	}
	return nil
}

func ensureAuthorityRecordJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("authority JSON contains multiple values")
		}
		return err
	}
	return nil
}

func validateWorkspaceAdmissionPolicyRef(ref workspaceAdmissionPolicyRefJCSV1) error {
	if err := validateAuthorityPositiveSafeInteger(ref.PolicyRev, "policy revision"); err != nil {
		return err
	}
	if !runtimeSHA256Pattern.MatchString(ref.PolicySHA256) {
		return authorityRecordNoncanonical("policy hash")
	}
	return nil
}

func validateAuthorityPositiveSafeInteger(value uint64, name string) error {
	if value == 0 || value > runtimeAuthorityMaxJSONInteger {
		return fmt.Errorf("%w: %s", errRuntimeOutOfRange, name)
	}
	return nil
}

func validateAuthorityRunLimit(value uint64, name string) error {
	if value == 0 || value > runtimeAuthorityMaxRunLimit {
		return fmt.Errorf("%w: %s", errRuntimeOutOfRange, name)
	}
	return nil
}

func parseAuthorityCanonicalTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, authorityRecordNoncanonical("lease timestamp")
	}
	return parsed, nil
}

func authorityCanonicalActor(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && !strings.ContainsRune(value, '\ufeff') && strings.Trim(value, " \t") == value
}

func authorityCanonicalReason(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && !strings.ContainsRune(value, '\ufeff') && !strings.ContainsRune(value, '\r') && strings.Trim(value, " \t\n") == value
}

func authorityOpaqueToken(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || strings.ContainsRune(value, '\ufeff') {
		return false
	}
	for _, character := range value {
		if character <= 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func authorityStableID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) && authorityStableIDPattern.MatchString(value)
}

func writeAuthorityPriorGeneration(output *bytes.Buffer, prior *authorityGeneration) {
	if prior == nil {
		output.WriteString("null")
		return
	}
	output.WriteString(`{"recordRev":`)
	writeAuthorityUint(output, prior.recordRev)
	output.WriteString(`,"sha256":`)
	writeRuntimeCanonicalJSONString(output, prior.sha256)
	output.WriteByte('}')
}

func writeWorkspaceAdmissionPolicyRef(output *bytes.Buffer, ref workspaceAdmissionPolicyRefJCSV1) {
	output.WriteString(`{"policyRev":`)
	writeAuthorityUint(output, ref.PolicyRev)
	output.WriteString(`,"policySha256":`)
	writeRuntimeCanonicalJSONString(output, ref.PolicySHA256)
	output.WriteByte('}')
}

func writeNullableWorkspaceAdmissionPolicyRef(output *bytes.Buffer, ref *workspaceAdmissionPolicyRefJCSV1) {
	if ref == nil {
		output.WriteString("null")
		return
	}
	writeWorkspaceAdmissionPolicyRef(output, *ref)
}

func writeAuthorityUint(output *bytes.Buffer, value uint64) {
	output.WriteString(strconv.FormatUint(value, 10))
}

func authorityRecordInvalid(message string) error {
	return fmt.Errorf("malformed authority record: %s", message)
}

func authorityRecordNoncanonical(message string) error {
	return fmt.Errorf("%w: %s", errRuntimeNoncanonical, message)
}
