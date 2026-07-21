package formations

import (
	"encoding/json"
	"errors"
)

const (
	RunViewSchema       = "formations.run-view.v1"
	RunEventPageSchema  = "formations.run-events.v1"
	RunListPageSchema   = "formations.run-list.v1"
	RunPageDefaultLimit = 200
	RunPageMaximumLimit = 200
	RunPageMaximumBytes = 1 << 20
	RunListPageLimit    = 50
	RunListMaximumBytes = 4 << 20
	MaxJSONSafeInteger  = uint64(9007199254740991)
)

var (
	ErrRunProjectionInvalid       = errors.New("formations run projection invalid")
	ErrRunEventUnknown            = errors.New("formations run event unknown")
	ErrRunProjectionResourceLimit = errors.New("formations run projection resource limit")
	ErrRunCommandNotTerminal      = errors.New("formations run command not terminal")
)

type CanonicalRunSource string

const (
	CanonicalRunSourceSchema1 CanonicalRunSource = "schema-1"
	CanonicalRunSourceSchema2 CanonicalRunSource = "schema-2"
)

type CanonicalInputRole string

const (
	CanonicalInputRoleSchema1Ledger           CanonicalInputRole = "schema-1-ledger"
	CanonicalInputRoleSchema1GraphSnapshot    CanonicalInputRole = "schema-1-graph-snapshot"
	CanonicalInputRoleSchema1BindingsSnapshot CanonicalInputRole = "schema-1-bindings-snapshot"

	CanonicalInputRoleSchema2WorkspaceRegistry  CanonicalInputRole = "schema-2-workspace-registry"
	CanonicalInputRoleSchema2WorkspaceBootstrap CanonicalInputRole = "schema-2-workspace-bootstrap"
	CanonicalInputRoleSchema2WorkspaceAuthority CanonicalInputRole = "schema-2-workspace-authority"
	CanonicalInputRoleSchema2AdmissionPolicy    CanonicalInputRole = "schema-2-admission-policy"
	CanonicalInputRoleSchema2RunBootstrap       CanonicalInputRole = "schema-2-run-bootstrap"
	CanonicalInputRoleSchema2GraphSnapshot      CanonicalInputRole = "schema-2-graph-snapshot"
	CanonicalInputRoleSchema2PrivateBindings    CanonicalInputRole = "schema-2-private-bindings"
	CanonicalInputRoleSchema2Ledger             CanonicalInputRole = "schema-2-ledger"
	CanonicalInputRoleSchema2CommandRecord      CanonicalInputRole = "schema-2-command-record"
	CanonicalInputRoleSchema2RunPrivateState    CanonicalInputRole = "schema-2-run-private-state"
)

type CanonicalInputDocument struct {
	Role   CanonicalInputRole
	Bytes  []byte
	SHA256 string
}

type CanonicalRunReadInput struct {
	RunID     string
	Source    CanonicalRunSource
	Documents []CanonicalInputDocument
}

type CanonicalCommandReadInput struct {
	Source    CanonicalRunSource
	Submitted SubmittedCommandIdentity
	Record    []byte
}

type SubmittedCommandIdentity struct {
	CommandID            string
	CommandKind          string
	CommandPayloadSHA256 string
}

type RunIdentityPageRequest struct {
	After string
	Limit int
}

type RunIdentityPage struct {
	RunIDs  []string
	Cursor  string
	HasMore bool
}

type CanonicalRunAuthorityReader interface {
	ReadRun(runID string) (CanonicalRunReadInput, error)
	ListRunIdentities(request RunIdentityPageRequest) (RunIdentityPage, error)
	ReadCommand(submitted SubmittedCommandIdentity) (CanonicalCommandReadInput, error)
}

type CanonicalRunSourceProjection struct {
	EventSchema     uint64  `json:"eventSchema"`
	AuthoritySchema *uint64 `json:"authoritySchema,omitempty"`
	Compatibility   bool    `json:"compatibility"`
}

type RunRoot struct {
	Kind   string `json:"kind"`
	NodeID string `json:"nodeId"`
}

type RunIdentity struct {
	BoardID   string  `json:"boardId"`
	BoardSlug string  `json:"boardSlug"`
	BoardRev  uint64  `json:"boardRev"`
	RunRoot   RunRoot `json:"runRoot"`
	MissionID string  `json:"missionId,omitempty"`
	BeadID    string  `json:"beadId,omitempty"`
	Epoch     uint64  `json:"epoch"`
	Redact    bool    `json:"redact"`
}

type RunAudit struct {
	EventSchema             uint64  `json:"eventSchema"`
	AuthoritySchema         *uint64 `json:"authoritySchema,omitempty"`
	AdmissionCommandID      string  `json:"admissionCommandId,omitempty"`
	CommandPayloadSHA256    string  `json:"commandPayloadSha256,omitempty"`
	WorkspaceAdmissionSeq   uint64  `json:"workspaceAdmissionSeq,omitempty"`
	AdmissionPolicyRev      uint64  `json:"admissionPolicyRev,omitempty"`
	AdmissionPolicySHA256   string  `json:"admissionPolicySha256,omitempty"`
	ActivationPolicyRev     uint64  `json:"activationPolicyRev,omitempty"`
	ActivationPolicySHA256  string  `json:"activationPolicySha256,omitempty"`
	LatestWriterFence       uint64  `json:"latestWriterFence,omitempty"`
	GraphSnapshotSHA256     string  `json:"graphSnapshotSha256,omitempty"`
	BindingProjectionSHA256 string  `json:"bindingProjectionSha256,omitempty"`
	StartSeq                uint64  `json:"startSeq"`
	ConsumedEventCount      uint64  `json:"consumedEventCount"`
}

type RunReadiness struct {
	NeededInputs uint64   `json:"neededInputs"`
	ReadyInputs  uint64   `json:"readyInputs"`
	TotalInputs  uint64   `json:"totalInputs"`
	WaitingFor   []string `json:"waitingFor"`
}

type RunAttemptRef struct {
	NodeID  string `json:"nodeId"`
	Attempt uint64 `json:"attempt"`
}

type RunOutputRef struct {
	NodeID  string `json:"nodeId"`
	Attempt uint64 `json:"attempt"`
	PortID  string `json:"portId"`
}

type RunGateRef struct {
	GateID  string `json:"gateId"`
	Attempt uint64 `json:"attempt"`
}

type RunSessionRef struct {
	BindingID string `json:"bindingId"`
	NodeID    string `json:"nodeId"`
	Attempt   uint64 `json:"attempt"`
	SlotID    string `json:"slotId"`
}

type PayloadValue struct {
	Kind      string                   `json:"kind"`
	MediaType string                   `json:"mediaType,omitempty"`
	Text      string                   `json:"text,omitempty"`
	Code      string                   `json:"code,omitempty"`
	Message   string                   `json:"message,omitempty"`
	Retryable *bool                    `json:"retryable,omitempty"`
	Feedback  *SafeGateFeedbackPayload `json:"feedback,omitempty"`
}

type PayloadProjection struct {
	Availability   string       `json:"availability"`
	Exact          bool         `json:"exact"`
	Classification string       `json:"classification,omitempty"`
	SourceKind     string       `json:"sourceKind,omitempty"`
	Encoding       string       `json:"encoding,omitempty"`
	MediaType      string       `json:"mediaType,omitempty"`
	SHA256         string       `json:"sha256,omitempty"`
	Payload        PayloadValue `json:"payload"`
}

type RootInputProjection struct {
	Classification string `json:"classification"`
	SourceKind     string `json:"sourceKind"`
	Encoding       string `json:"encoding"`
	MediaType      string `json:"mediaType"`
	SHA256         string `json:"sha256"`
	Text           string `json:"text"`
}

type SafeInputIdentity struct {
	EdgeID            string             `json:"edgeId,omitempty"`
	OriginEdgeID      string             `json:"originEdgeId,omitempty"`
	DeliveryEdgeID    string             `json:"deliveryEdgeId,omitempty"`
	FromNodeID        string             `json:"fromNodeId,omitempty"`
	FromPortID        string             `json:"fromPortId,omitempty"`
	SourceNodeID      string             `json:"sourceNodeId,omitempty"`
	SourcePortID      string             `json:"sourcePortId,omitempty"`
	SourceOutputSeq   uint64             `json:"sourceOutputSeq,omitempty"`
	SourceAttempt     uint64             `json:"sourceAttempt,omitempty"`
	ToPortID          string             `json:"toPortId,omitempty"`
	OutputSeq         uint64             `json:"outputSeq,omitempty"`
	InputID           string             `json:"inputId,omitempty"`
	SourceKind        string             `json:"sourceKind,omitempty"`
	RunID             string             `json:"runId,omitempty"`
	SeedID            string             `json:"seedId,omitempty"`
	SeedEncoding      string             `json:"seedEncoding,omitempty"`
	SeedMediaType     string             `json:"seedMediaType,omitempty"`
	SeedSHA256        string             `json:"seedSha256,omitempty"`
	ToNodeID          string             `json:"toNodeId,omitempty"`
	GateInputSeq      uint64             `json:"gateInputSeq,omitempty"`
	PayloadProjection *PayloadProjection `json:"payloadProjection,omitempty"`
}

type RunNodeView struct {
	NodeID           string          `json:"nodeId"`
	Kind             string          `json:"kind"`
	Status           string          `json:"status"`
	FinalDisposition string          `json:"finalDisposition,omitempty"`
	LatestAttempt    uint64          `json:"latestAttempt,omitempty"`
	Readiness        RunReadiness    `json:"readiness"`
	Attempts         []RunAttemptRef `json:"attempts"`
	Outputs          []RunOutputRef  `json:"outputs"`
	Gates            []RunGateRef    `json:"gates"`
	Sessions         []RunSessionRef `json:"sessions"`
}

type RunAttemptView struct {
	NodeID       string              `json:"nodeId"`
	Attempt      uint64              `json:"attempt"`
	Status       string              `json:"status"`
	StartedSeq   uint64              `json:"startedSeq,omitempty"`
	CompletedSeq uint64              `json:"completedSeq,omitempty"`
	InputRefs    []SafeInputIdentity `json:"inputRefs"`
	Slots        []RunSessionRef     `json:"slots"`
	Outputs      []RunOutputRef      `json:"outputs"`
	Gate         *RunGateRef         `json:"gate,omitempty"`
	Disposition  string              `json:"disposition,omitempty"`
}

type SafeGateEvidence struct {
	Kind       string `json:"kind"`
	Reason     string `json:"reason,omitempty"`
	ArtifactID string `json:"artifactId,omitempty"`
	Seq        uint64 `json:"seq,omitempty"`
	Text       string `json:"text,omitempty"`
}

type RunGateView struct {
	GateID        string             `json:"gateId"`
	Attempt       uint64             `json:"attempt"`
	Status        string             `json:"status"`
	EvaluatingSeq uint64             `json:"evaluatingSeq,omitempty"`
	RequestSeq    uint64             `json:"requestSeq,omitempty"`
	VerdictSeq    uint64             `json:"verdictSeq,omitempty"`
	Verdict       string             `json:"verdict,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	Evidence      []SafeGateEvidence `json:"evidence"`
}

type RunOutputView struct {
	NodeID            string            `json:"nodeId"`
	Attempt           uint64            `json:"attempt"`
	PortID            string            `json:"portId"`
	OutcomeSeq        uint64            `json:"outcomeSeq"`
	PayloadProjection PayloadProjection `json:"payloadProjection"`
}

type SafeArtifactRef struct {
	ArtifactID string `json:"artifactId"`
	RootID     string `json:"rootId"`
	Ref        string `json:"ref"`
	MediaType  string `json:"mediaType"`
	SizeBytes  uint64 `json:"sizeBytes"`
	SHA256     string `json:"sha256"`
}

type ArtifactProjection interface{ isArtifactProjection() }

type AvailableArtifactProjection struct {
	ArtifactID   string          `json:"artifactId"`
	Availability string          `json:"availability"`
	Name         string          `json:"name"`
	Artifact     SafeArtifactRef `json:"artifact"`
}

func (AvailableArtifactProjection) isArtifactProjection() {}

type UnavailableArtifactProjection struct {
	ArtifactID   string `json:"artifactId"`
	Availability string `json:"availability"`
	Name         string `json:"name"`
	ErrorCode    string `json:"errorCode"`
}

func (UnavailableArtifactProjection) isArtifactProjection() {}

type SafeOpenDispatch interface{ isSafeOpenDispatch() }

type SafeSchema1OpenDispatch struct {
	DispatchID  string  `json:"dispatchId"`
	NodeID      string  `json:"nodeId"`
	SlotID      string  `json:"slotId"`
	DispatchSeq *uint64 `json:"dispatchSeq,omitempty"`
}

func (dispatch SafeSchema1OpenDispatch) MarshalJSON() ([]byte, error) {
	object := map[string]any{
		"dispatchId": dispatch.DispatchID,
		"nodeId":     dispatch.NodeID,
		"slotId":     dispatch.SlotID,
	}
	if dispatch.DispatchSeq != nil {
		object["dispatchSeq"] = *dispatch.DispatchSeq
	}
	return json.Marshal(object)
}

func (SafeSchema1OpenDispatch) isSafeOpenDispatch() {}

type SafeSchema2OpenDispatch struct {
	DispatchID                 string  `json:"dispatchId"`
	TargetLeaseID              string  `json:"targetLeaseId"`
	NodeID                     string  `json:"nodeId"`
	Attempt                    uint64  `json:"attempt"`
	SlotID                     string  `json:"slotId"`
	AgentID                    string  `json:"agentId"`
	BindingID                  string  `json:"bindingId"`
	SessionTargetID            string  `json:"sessionTargetId"`
	TargetFingerprint          string  `json:"targetFingerprint"`
	DispatchSeq                uint64  `json:"dispatchSeq"`
	PeekCapabilityState        string  `json:"peekCapabilityState"`
	LatestCapabilityGeneration string  `json:"latestCapabilityGeneration"`
	LatestCapabilityIssuedSeq  uint64  `json:"latestCapabilityIssuedSeq"`
	LatestSteeringGeneration   string  `json:"latestSteeringGeneration"`
	OpenSteeringStartedSeq     *uint64 `json:"openSteeringStartedSeq,omitempty"`
	PeekCapabilityRevokedSeq   *uint64 `json:"peekCapabilityRevokedSeq,omitempty"`
	InterruptState             string  `json:"interruptState"`
	InterruptRequestedSeq      *uint64 `json:"interruptRequestedSeq,omitempty"`
	InterruptOutcomeSeq        *uint64 `json:"interruptOutcomeSeq,omitempty"`
}

func (SafeSchema2OpenDispatch) isSafeOpenDispatch() {}

type RunBlockView struct {
	Seq            uint64             `json:"seq"`
	Epoch          uint64             `json:"epoch"`
	Scope          string             `json:"scope"`
	NodeID         string             `json:"nodeId,omitempty"`
	GateID         string             `json:"gateId,omitempty"`
	Code           string             `json:"code,omitempty"`
	Reason         string             `json:"reason"`
	ResumeAllowed  bool               `json:"resumeAllowed"`
	ResumePolicy   string             `json:"resumePolicy"`
	NextEpoch      uint64             `json:"nextEpoch,omitempty"`
	OpenDispatches []SafeOpenDispatch `json:"openDispatches"`
}

type RunEscalationView struct {
	Seq      uint64 `json:"seq"`
	NodeID   string `json:"nodeId,omitempty"`
	GateID   string `json:"gateId,omitempty"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Source   string `json:"source"`
	Trigger  string `json:"trigger"`
	Blocks   bool   `json:"blocks"`
}

type RunSessionBaseline struct {
	Encoding string `json:"encoding"`
	SHA256   string `json:"sha256"`
	State    string `json:"state"`
}

type RunSessionAttachment struct {
	State string `json:"state"`
}
type RunSessionOccupancy struct {
	State string `json:"state"`
}

type RunSessionPeekCapability struct {
	State      string `json:"state"`
	IssuedSeq  uint64 `json:"issuedSeq"`
	Generation string `json:"generation"`
}

type RunSessionSteering struct {
	State      string  `json:"state"`
	Generation string  `json:"generation"`
	StartedSeq *uint64 `json:"startedSeq,omitempty"`
}

type RunSessionView struct {
	BindingID               string                   `json:"bindingId"`
	NodeID                  string                   `json:"nodeId"`
	Attempt                 uint64                   `json:"attempt"`
	SlotID                  string                   `json:"slotId"`
	DispatchID              string                   `json:"dispatchId,omitempty"`
	TargetLeaseID           string                   `json:"targetLeaseId,omitempty"`
	SessionTargetID         string                   `json:"sessionTargetId"`
	BindingHealth           string                   `json:"bindingHealth"`
	AvailabilityReason      string                   `json:"availabilityReason,omitempty"`
	SessionLineageSHA256    string                   `json:"sessionLineageSha256"`
	TargetFingerprintSHA256 string                   `json:"targetFingerprintSha256"`
	Baseline                RunSessionBaseline       `json:"baseline"`
	Attachment              RunSessionAttachment     `json:"attachment"`
	Occupancy               RunSessionOccupancy      `json:"occupancy"`
	PeekCapability          RunSessionPeekCapability `json:"peekCapability"`
	Steering                RunSessionSteering       `json:"steering"`
	OperatorInfluenced      bool                     `json:"operatorInfluenced"`
}

type CoordinatorReconcileCondition struct {
	Kind  string `json:"kind"`
	State string `json:"state"`
}

type RunAction interface{ isRunAction() }

type RunCancelAction struct {
	Kind            string `json:"kind"`
	ExpectedLastSeq uint64 `json:"expectedLastSeq"`
}

func (RunCancelAction) isRunAction() {}

type RunResumeAction struct {
	Kind       string `json:"kind"`
	BlockedSeq uint64 `json:"blockedSeq"`
	ResumeMode string `json:"resumeMode"`
}

func (RunResumeAction) isRunAction() {}

type RunVerdictAction struct {
	Kind            string   `json:"kind"`
	GateID          string   `json:"gateId"`
	RequestedSeq    uint64   `json:"requestedSeq"`
	AllowedVerdicts []string `json:"allowedVerdicts"`
}

func (RunVerdictAction) isRunAction() {}

type RunPeekAction struct {
	Kind            string `json:"kind"`
	BindingID       string `json:"bindingId"`
	NodeID          string `json:"nodeId"`
	Attempt         uint64 `json:"attempt"`
	SlotID          string `json:"slotId"`
	DispatchID      string `json:"dispatchId"`
	TargetLeaseID   string `json:"targetLeaseId"`
	SessionTargetID string `json:"sessionTargetId"`
}

func (RunPeekAction) isRunAction() {}

type RunView struct {
	Schema             string                         `json:"schema"`
	RunID              string                         `json:"runId"`
	Generation         string                         `json:"generation"`
	Source             CanonicalRunSourceProjection   `json:"source"`
	Cursor             uint64                         `json:"cursor"`
	Status             string                         `json:"status"`
	Final              bool                           `json:"final"`
	RecoveryState      string                         `json:"recoveryState"`
	ReconcileCondition *CoordinatorReconcileCondition `json:"reconcileCondition"`
	Identity           RunIdentity                    `json:"identity"`
	Audit              RunAudit                       `json:"audit"`
	Nodes              []RunNodeView                  `json:"nodes"`
	Attempts           []RunAttemptView               `json:"attempts"`
	Gates              []RunGateView                  `json:"gates"`
	Outputs            []RunOutputView                `json:"outputs"`
	Artifacts          []ArtifactProjection           `json:"artifacts"`
	Blocks             []RunBlockView                 `json:"blocks"`
	Escalations        []RunEscalationView            `json:"escalations"`
	Sessions           []RunSessionView               `json:"sessions"`
	Actions            []RunAction                    `json:"actions"`
}

type RunListPage struct {
	Schema  string    `json:"schema"`
	Runs    []RunView `json:"runs"`
	Cursor  string    `json:"cursor"`
	HasMore bool      `json:"hasMore"`
}

type RunEventPage struct {
	Schema     string                       `json:"schema"`
	RunID      string                       `json:"runId"`
	Generation string                       `json:"generation"`
	Source     CanonicalRunSourceProjection `json:"source"`
	Cursor     uint64                       `json:"cursor"`
	HasMore    bool                         `json:"hasMore"`
	Events     []SafeRunEvent               `json:"events"`
}

type RunCommandReceipt interface{ isRunCommandReceipt() }

type RunCommandAppliedReceipt struct {
	CommandID                  string              `json:"commandId"`
	CommandPayloadSHA256       string              `json:"commandPayloadSha256"`
	CommandKind                string              `json:"commandKind"`
	OutcomeWriterFence         string              `json:"outcomeWriterFence"`
	State                      string              `json:"state"`
	RunID                      string              `json:"runId"`
	EffectSeq                  uint64              `json:"effectSeq"`
	DecisionAdmissionPolicyRef *AdmissionPolicyRef `json:"decisionAdmissionPolicyRef"`
}

func (RunCommandAppliedReceipt) isRunCommandReceipt() {}

type RunCommandRejectedReceipt struct {
	CommandID                  string              `json:"commandId"`
	CommandPayloadSHA256       string              `json:"commandPayloadSha256"`
	CommandKind                string              `json:"commandKind"`
	OutcomeWriterFence         string              `json:"outcomeWriterFence"`
	State                      string              `json:"state"`
	RejectionCode              string              `json:"rejectionCode"`
	DecisionAdmissionPolicyRef *AdmissionPolicyRef `json:"decisionAdmissionPolicyRef"`
}

func (RunCommandRejectedReceipt) isRunCommandReceipt() {}

type AdmissionPolicyRef struct {
	PolicyRev    uint64 `json:"policyRev"`
	PolicySHA256 string `json:"policySha256"`
}

type safeEventEnvelope struct {
	Timestamp string `json:"ts"`
	RunID     string `json:"runId"`
	Seq       uint64 `json:"seq"`
	Actor     string `json:"actor"`
	BoardID   string `json:"boardId,omitempty"`
	BoardRev  uint64 `json:"boardRev,omitempty"`
	MissionID string `json:"missionId,omitempty"`
	BeadID    string `json:"beadId,omitempty"`
	NodeID    string `json:"nodeId,omitempty"`
	SlotID    string `json:"slotId,omitempty"`
	GateID    string `json:"gateId,omitempty"`
	EdgeID    string `json:"edgeId,omitempty"`
	Epoch     uint64 `json:"epoch,omitempty"`
	Attempt   uint64 `json:"attempt,omitempty"`
}

type SafeRunEvent interface{ isSafeRunEvent() }

type SafeSchema1RunStartedData interface{ isSafeSchema1RunStartedData() }

type SafeSchema1RunStartedMissionData struct {
	BoardSlug string    `json:"boardSlug"`
	BoardRev  uint64    `json:"boardRev"`
	MissionID string    `json:"missionId"`
	BeadID    string    `json:"beadId"`
	Limits    RunLimits `json:"limits"`
}

func (SafeSchema1RunStartedMissionData) isSafeSchema1RunStartedData() {}

type SafeSchema1RunStartedFormationData struct {
	BoardSlug   string    `json:"boardSlug"`
	BoardRev    uint64    `json:"boardRev"`
	MissionID   string    `json:"missionId"`
	BeadID      string    `json:"beadId"`
	Limits      RunLimits `json:"limits"`
	Mode        string    `json:"mode"`
	FormationID string    `json:"formationId"`
}

func (SafeSchema1RunStartedFormationData) isSafeSchema1RunStartedData() {}

type SafeSchema1RunResumedData struct {
	ResumedFromSeq uint64                    `json:"resumedFromSeq"`
	ResumedBy      string                    `json:"resumedBy"`
	ResumeMode     string                    `json:"resumeMode"`
	Reason         string                    `json:"reason"`
	OpenDispatches []SafeSchema1OpenDispatch `json:"openDispatches"`
}
type SafeSchema1NodeWaitingData struct {
	NeededInputs, ReadyInputs, TotalInputs uint64
	WaitingFor                             []string `json:"waitingFor"`
}

func (d SafeSchema1NodeWaitingData) MarshalJSON() ([]byte, error) {
	type wire struct {
		NeededInputs uint64   `json:"neededInputs"`
		ReadyInputs  uint64   `json:"readyInputs"`
		TotalInputs  uint64   `json:"totalInputs"`
		WaitingFor   []string `json:"waitingFor"`
	}
	return json.Marshal(wire{d.NeededInputs, d.ReadyInputs, d.TotalInputs, d.WaitingFor})
}

type SafeSchema1NodeStartedData struct {
	NodeKind  string              `json:"nodeKind"`
	InputRefs []SafeInputIdentity `json:"inputRefs"`
	Reason    string              `json:"reason"`
}
type SafeSchema1Participant struct {
	SlotID  string `json:"slotId"`
	Label   string `json:"label"`
	AgentID string `json:"agentId"`
	Harness string `json:"harness"`
}
type SafeSchema1OrchestrationTeamData struct {
	Mode           string                   `json:"mode"`
	ControllerSlot string                   `json:"controllerSlot"`
	Controller     SafeSchema1Participant   `json:"controller"`
	Workers        []SafeSchema1Participant `json:"workers"`
}
type SafeSchema1PeerPlaneData struct {
	Mode  string                   `json:"mode"`
	Peers []SafeSchema1Participant `json:"peers"`
}
type SafeSchema1SlotDispatchData struct {
	DispatchID         string `json:"dispatchId"`
	NodeID             string `json:"nodeId"`
	SlotID             string `json:"slotId"`
	AgentID            string `json:"agentId"`
	Harness            string `json:"harness"`
	Phase              string `json:"phase"`
	PromptSHA256       string `json:"promptSha256"`
	NativeAck          bool   `json:"nativeAck"`
	RecordedBeforeSend bool   `json:"recordedBeforeSend"`
}
type SafeSchema1AdapterSendData struct {
	Adapter      string `json:"adapter"`
	DispatchID   string `json:"dispatchId"`
	NodeID       string `json:"nodeId"`
	SlotID       string `json:"slotId"`
	Phase        string `json:"phase"`
	SocketSHA256 string `json:"socketSha256"`
	PromptSHA256 string `json:"promptSha256"`
	Sent         bool   `json:"sent"`
}
type SafeSchema1Sentinel struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
}
type SafeSchema1SlotResultData struct {
	DispatchID string              `json:"dispatchId"`
	NodeID     string              `json:"nodeId"`
	SlotID     string              `json:"slotId"`
	Status     string              `json:"status"`
	Sentinel   SafeSchema1Sentinel `json:"sentinel"`
}
type SafeSchema1OutputValue struct {
	Text string `json:"text,omitempty"`
}

type SafeSchema1Outputs map[string]SafeSchema1OutputValue
type SafeSchema1Verdicts map[string]string

type SafeSchema1NodeOutputData struct {
	Status  string             `json:"status"`
	Text    string             `json:"text"`
	Outputs SafeSchema1Outputs `json:"outputs"`
	Reason  string             `json:"reason"`
}
type SafeSchema1GateEvaluatingData struct {
	Kinds      []string          `json:"kinds"`
	Criterion  string            `json:"criterion"`
	InputRef   SafeInputIdentity `json:"inputRef"`
	JudgeChain []string          `json:"judgeChain"`
}
type SafeSchema1GateVerdictData struct {
	Verdict     string              `json:"verdict"`
	PerKind     SafeSchema1Verdicts `json:"perKind"`
	RoutePort   string              `json:"routePort"`
	RoutedEdges []string            `json:"routedEdges"`
	Reason      string              `json:"reason"`
	InputRef    SafeInputIdentity   `json:"inputRef"`
}
type SafeSchema1VerificationVerdictData struct {
	VerificationID string `json:"verificationId"`
	Verdict        string `json:"verdict"`
}
type SafeSchema1EscalationRaisedData struct {
	Trigger  string `json:"trigger"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Source   string `json:"source"`
	NodeID   string `json:"nodeId"`
	GateID   string `json:"gateId"`
	Blocks   bool   `json:"blocks"`
}
type SafeSchema1HumanInputRequestedData struct {
	GateID         string              `json:"gateId"`
	NodeID         string              `json:"nodeId"`
	Choices        []string            `json:"choices"`
	RequestedBy    string              `json:"requestedBy"`
	InputRef       SafeInputIdentity   `json:"inputRef"`
	CodeVerdict    string              `json:"codeVerdict"`
	CodeReason     string              `json:"codeReason"`
	CodePerKind    SafeSchema1Verdicts `json:"codePerKind"`
	TimeoutSeconds uint64              `json:"timeoutSeconds"`
}
type SafeSchema1HumanVerdictRecordedData struct {
	GateID       string `json:"gateId"`
	NodeID       string `json:"nodeId"`
	Verdict      string `json:"verdict"`
	Reason       string `json:"reason"`
	RequestedSeq uint64 `json:"requestedSeq"`
	DecidedBy    string `json:"decidedBy"`
}
type SafeSchema1ErrorData struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Reason      string `json:"reason"`
	Boundary    string `json:"boundary"`
	NodeID      string `json:"nodeId"`
	GateID      string `json:"gateId"`
	SlotID      string `json:"slotId"`
	DispatchID  string `json:"dispatchId"`
	Recoverable bool   `json:"recoverable"`
	RelatedSeq  uint64 `json:"relatedSeq"`
}
type SafeSchema1RunBlockedData struct {
	Reason         string                    `json:"reason"`
	Code           string                    `json:"code"`
	Boundary       string                    `json:"boundary"`
	BlockedNodeID  string                    `json:"blockedNodeId"`
	BlockedGateID  string                    `json:"blockedGateId"`
	WaitingNodes   []string                  `json:"waitingNodes"`
	Recoverable    bool                      `json:"recoverable"`
	ResumeAllowed  bool                      `json:"resumeAllowed"`
	ResumePolicy   string                    `json:"resumePolicy"`
	OpenDispatches []SafeSchema1OpenDispatch `json:"openDispatches"`
	NextEpoch      uint64                    `json:"nextEpoch"`
}
type SafeSchema1RunCanceledData struct {
	Reason               string   `json:"reason"`
	RequestedBy          string   `json:"requestedBy"`
	SoftInterruptedSlots []string `json:"softInterruptedSlots"`
	Final                bool     `json:"final"`
}
type SafeSchema1RunFailedData struct {
	Code        string `json:"code"`
	Reason      string `json:"reason"`
	Boundary    string `json:"boundary"`
	Recoverable bool   `json:"recoverable"`
	RelatedSeq  uint64 `json:"relatedSeq"`
	Final       bool   `json:"final"`
}
type SafeSchema1RunSucceededData struct {
	Final       bool   `json:"final"`
	Mode        string `json:"mode"`
	FormationID string `json:"formationId"`
	MissionID   string `json:"missionId"`
	Reason      string `json:"reason"`
}

type SafePriorTurnResult struct {
	SlotResultSeq    uint64 `json:"slotResultSeq"`
	TurnResultSHA256 string `json:"turnResultSha256"`
}

type SafeTurnInputs struct {
	NodeStartedSeq   uint64                `json:"nodeStartedSeq"`
	PriorTurnResults []SafePriorTurnResult `json:"priorTurnResults"`
}

type SafePayloadProjections map[string]PayloadProjection
type SafeProjectionHashes map[string]string
type SafeGateKindVerdicts map[string]string
type SafeGateKindResultSeqs map[string]*uint64
type SafeCompletedKindResultSeqs map[string]uint64

type SafeTurnResult struct {
	TurnKey          string                 `json:"turnKey"`
	Phase            string                 `json:"phase"`
	Status           string                 `json:"status"`
	TurnPayload      PayloadProjection      `json:"turnPayload"`
	Outputs          SafePayloadProjections `json:"outputs"`
	ReportArtifactID string                 `json:"reportArtifactId"`
	ArtifactIDs      []string               `json:"artifactIds"`
	DiffArtifactIDs  []string               `json:"diffArtifactIds"`
}

type SafeProducedBy struct {
	Kind       string `json:"kind"`
	OutcomeSeq uint64 `json:"outcomeSeq"`
}

type SafeEventTiming struct {
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	DurationMS uint64 `json:"durationMs"`
}

type SafeDeliveredEdge struct {
	OriginEdgeID    string `json:"originEdgeId"`
	DeliveryEdgeID  string `json:"deliveryEdgeId"`
	ToNodeID        string `json:"toNodeId"`
	ToPortID        string `json:"toPortId"`
	SourceNodeID    string `json:"sourceNodeId"`
	SourcePortID    string `json:"sourcePortId"`
	SourceOutputSeq uint64 `json:"sourceOutputSeq"`
	SourceAttempt   uint64 `json:"sourceAttempt"`
}

type SafeCriterionProjection struct {
	Classification string `json:"classification"`
	SourceKind     string `json:"sourceKind"`
	Encoding       string `json:"encoding"`
	MediaType      string `json:"mediaType"`
	SHA256         string `json:"sha256"`
	Text           string `json:"text"`
}

type SafeJudgeResult struct {
	Verdict  string             `json:"verdict"`
	Reason   string             `json:"reason"`
	Evidence []SafeGateEvidence `json:"evidence"`
}

type SafeGateFeedbackInput struct {
	InputID      string `json:"inputId"`
	GateInputSeq uint64 `json:"gateInputSeq"`
}

type SafeGateFeedbackPayload struct {
	FeedbackID      string                `json:"feedbackId"`
	GateID          string                `json:"gateId"`
	Verdict         string                `json:"verdict"`
	EvaluatedInput  SafeGateFeedbackInput `json:"evaluatedInput"`
	Reason          string                `json:"reason"`
	Evidence        []SafeGateEvidence    `json:"evidence"`
	GateSeq         uint64                `json:"gateSeq"`
	GateAttempt     uint64                `json:"gateAttempt"`
	RevisionCycleID string                `json:"revisionCycleId,omitempty"`
}

type SafeArtifactSource struct {
	Kind        string `json:"kind"`
	DispatchID  string `json:"dispatchId,omitempty"`
	NodeID      string `json:"nodeId,omitempty"`
	SlotID      string `json:"slotId,omitempty"`
	GateID      string `json:"gateId,omitempty"`
	GateAttempt uint64 `json:"gateAttempt,omitempty"`
	SourceID    string `json:"sourceId,omitempty"`
}

type SafeFixedSystemProjection struct {
	Classification string `json:"classification"`
	SourceKind     string `json:"sourceKind"`
	TemplateID     string `json:"templateId"`
}

type SafeHumanChoiceProjections struct {
	Pass SafeFixedSystemProjection `json:"pass"`
	Fail SafeFixedSystemProjection `json:"fail"`
}

type SafeRetryTarget struct {
	NodeID         string   `json:"nodeId"`
	Attempt        uint64   `json:"attempt"`
	OutputPortIDs  []string `json:"outputPortIds"`
	OutcomeSeqs    []uint64 `json:"outcomeSeqs"`
	DeliveredEdges []string `json:"deliveredEdges"`
}

type SafeNodeAttemptSnapshot struct {
	NodeID   string `json:"nodeId"`
	NodeKind string `json:"nodeKind"`
	Attempt  uint64 `json:"attempt"`
	StartSeq uint64 `json:"startSeq"`
	Phase    string `json:"phase"`
	PhaseSeq uint64 `json:"phaseSeq"`
}

type SafeToolLaunchSnapshot struct {
	LaunchID            string `json:"launchId"`
	Generation          string `json:"generation"`
	ProcessScopeID      string `json:"processScopeId"`
	DeadlineAuthorityID string `json:"deadlineAuthorityId"`
	LaunchSeq           uint64 `json:"launchSeq"`
}

type SafeToolLeaseSnapshot struct {
	ToolLeaseID  string                  `json:"toolLeaseId"`
	NodeID       string                  `json:"nodeId"`
	Attempt      uint64                  `json:"attempt"`
	DispatchSeq  uint64                  `json:"dispatchSeq"`
	LatestLaunch *SafeToolLaunchSnapshot `json:"latestLaunch,omitempty"`
}

type SafeFailureCause struct {
	Kind        string `json:"kind"`
	DispatchID  string `json:"dispatchId,omitempty"`
	ToolLeaseID string `json:"toolLeaseId,omitempty"`
	ErrorSeq    uint64 `json:"errorSeq,omitempty"`
}

type SafeNodeAttemptDisposition struct {
	SafeNodeAttemptSnapshot
	Disposition string `json:"disposition"`
}

type SafeSlotDispatchDisposition struct {
	SafeSchema2OpenDispatch
	Disposition                   string `json:"disposition"`
	SoftInterrupt                 string `json:"softInterrupt"`
	SoftInterruptRequestedSeq     uint64 `json:"softInterruptRequestedSeq"`
	SoftInterruptOutcomeSeq       uint64 `json:"softInterruptOutcomeSeq,omitempty"`
	TargetLeaseState              string `json:"targetLeaseState"`
	FinalPeekCapabilityState      string `json:"finalPeekCapabilityState"`
	FinalCapabilityGeneration     string `json:"finalCapabilityGeneration"`
	FinalCapabilityIssuedSeq      uint64 `json:"finalCapabilityIssuedSeq"`
	FinalSteeringGeneration       string `json:"finalSteeringGeneration"`
	FinalPeekCapabilityRevokedSeq uint64 `json:"finalPeekCapabilityRevokedSeq"`
}

type SafeToolLeaseDisposition struct {
	SafeToolLeaseSnapshot
	Disposition string `json:"disposition"`
}

type SafeDisplayEvidence struct {
	Kind               string             `json:"kind"`
	Text               string             `json:"text,omitempty"`
	ArtifactProjection ArtifactProjection `json:"artifactProjection,omitempty"`
}

type SafeSchema2RunStartedData struct {
	WorkspaceAuthorityID    string              `json:"workspaceAuthorityId"`
	WorkspaceAdmissionSeq   uint64              `json:"workspaceAdmissionSeq"`
	AdmissionPolicyRev      uint64              `json:"admissionPolicyRev"`
	AdmissionPolicySHA256   string              `json:"admissionPolicySha256"`
	AdmissionCommandID      string              `json:"admissionCommandId"`
	CommandPayloadSHA256    string              `json:"commandPayloadSha256"`
	BoardSlug               string              `json:"boardSlug"`
	SourceBoardSchema       uint64              `json:"sourceBoardSchema"`
	SnapshotSchema          uint64              `json:"snapshotSchema"`
	RunAuthorityID          string              `json:"runAuthorityId"`
	GraphSnapshotSHA256     string              `json:"graphSnapshotSha256"`
	PrivateBindingsSHA256   string              `json:"privateBindingsSha256"`
	BindingProjectionSHA256 string              `json:"bindingProjectionSha256"`
	RunRoot                 RunRoot             `json:"runRoot"`
	RootInputProjection     RootInputProjection `json:"rootInputProjection"`
	Limits                  RunLimits           `json:"limits"`
}
type SafeSchema2RunActivatedData struct {
	WorkspaceAdmissionSeq uint64 `json:"workspaceAdmissionSeq"`
	AdmissionPolicyRev    uint64 `json:"admissionPolicyRev"`
	AdmissionPolicySHA256 string `json:"admissionPolicySha256"`
	Reason                string `json:"reason"`
}
type SafeSchema2RunResumedData struct {
	CommandID            string                    `json:"commandId"`
	CommandPayloadSHA256 string                    `json:"commandPayloadSha256"`
	ResumedFromSeq       uint64                    `json:"resumedFromSeq"`
	ResumedBy            string                    `json:"resumedBy"`
	ResumeMode           string                    `json:"resumeMode"`
	Reason               string                    `json:"reason"`
	OpenDispatches       []SafeSchema2OpenDispatch `json:"openDispatches"`
	RetryTargets         []SafeRetryTarget         `json:"retryTargets"`
}
type SafeSchema2NodeWaitingData struct {
	NodeID       string   `json:"nodeId"`
	NeededInputs uint64   `json:"neededInputs"`
	ReadyInputs  uint64   `json:"readyInputs"`
	TotalInputs  uint64   `json:"totalInputs"`
	WaitingFor   []string `json:"waitingFor"`
}
type SafeSchema2NodeInputIgnoredData struct {
	NodeID         string            `json:"nodeId"`
	ToPortID       string            `json:"toPortId"`
	InputRef       SafeInputIdentity `json:"inputRef"`
	Reason         string            `json:"reason"`
	RelatedAttempt uint64            `json:"relatedAttempt"`
}
type SafeSchema2NodeStartedData struct {
	NodeID             string              `json:"nodeId"`
	NodeKind           string              `json:"nodeKind"`
	Attempt            uint64              `json:"attempt"`
	Reason             string              `json:"reason"`
	InputRefs          []SafeInputIdentity `json:"inputRefs"`
	ContextEncoding    string              `json:"contextEncoding,omitempty"`
	JudgeContextSHA256 string              `json:"judgeContextSha256,omitempty"`
	PriorResultSeqs    []uint64            `json:"priorResultSeqs,omitempty"`
	TriggerFeedbackID  string              `json:"triggerFeedbackId,omitempty"`
	PriorGateSeq       uint64              `json:"priorGateSeq,omitempty"`
}
type SafeSchema2SlotBindingObservedData struct {
	BindingID       string `json:"bindingId"`
	SlotID          string `json:"slotId"`
	SessionTargetID string `json:"sessionTargetId"`
	Health          string `json:"health"`
	Reason          string `json:"reason"`
	ObservedAt      string `json:"observedAt"`
	RelatedSeq      uint64 `json:"relatedSeq"`
}
type SafeSchema2SlotDispatchData struct {
	DispatchID                   string         `json:"dispatchId"`
	TargetLeaseID                string         `json:"targetLeaseId"`
	TurnKey                      string         `json:"turnKey"`
	TurnPhase                    string         `json:"turnPhase"`
	TurnInputs                   SafeTurnInputs `json:"turnInputs"`
	NodeID                       string         `json:"nodeId"`
	Attempt                      uint64         `json:"attempt"`
	SlotID                       string         `json:"slotId"`
	AgentID                      string         `json:"agentId"`
	Harness                      string         `json:"harness"`
	BindingID                    string         `json:"bindingId"`
	SessionTargetID              string         `json:"sessionTargetId"`
	TargetFingerprint            string         `json:"targetFingerprint"`
	DispatchInputBarrierEncoding string         `json:"dispatchInputBarrierEncoding"`
	DispatchInputBarrierSHA256   string         `json:"dispatchInputBarrierSha256"`
	TargetReadyProofEncoding     string         `json:"targetReadyProofEncoding"`
	TargetReadyProofSHA256       string         `json:"targetReadyProofSha256"`
	PaneHistoryBaselineEncoding  string         `json:"paneHistoryBaselineEncoding"`
	PaneHistoryBaselineSHA256    string         `json:"paneHistoryBaselineSha256"`
	SteeringGeneration           string         `json:"steeringGeneration"`
	PromptSHA256                 string         `json:"promptSha256"`
	NativeAck                    bool           `json:"nativeAck"`
	RecordedBeforeSend           bool           `json:"recordedBeforeSend"`
}
type SafeSchema2SlotPeekCapabilityIssuedData struct {
	DispatchID           string `json:"dispatchId"`
	TargetLeaseID        string `json:"targetLeaseId"`
	BindingID            string `json:"bindingId"`
	SessionTargetID      string `json:"sessionTargetId"`
	TargetFingerprint    string `json:"targetFingerprint"`
	CapabilityGeneration string `json:"capabilityGeneration"`
	PriorIssuedSeq       uint64 `json:"priorIssuedSeq"`
	IssuedAt             string `json:"issuedAt"`
}
type SafeSchema2SlotSteeringStartedData struct {
	DispatchID           string `json:"dispatchId"`
	TargetLeaseID        string `json:"targetLeaseId"`
	BindingID            string `json:"bindingId"`
	SessionTargetID      string `json:"sessionTargetId"`
	TargetFingerprint    string `json:"targetFingerprint"`
	CapabilityIssuedSeq  uint64 `json:"capabilityIssuedSeq"`
	CapabilityGeneration string `json:"capabilityGeneration"`
	SteeringGeneration   string `json:"steeringGeneration"`
	Actor                string `json:"actor"`
	StartedAt            string `json:"startedAt"`
	RecordedBeforeInput  bool   `json:"recordedBeforeInput"`
}
type SafeSchema2SlotSteeringEndedData struct {
	StartedSeq         uint64 `json:"startedSeq"`
	DispatchID         string `json:"dispatchId"`
	TargetLeaseID      string `json:"targetLeaseId"`
	TargetFingerprint  string `json:"targetFingerprint"`
	SteeringGeneration string `json:"steeringGeneration"`
	Reason             string `json:"reason"`
	EndedAt            string `json:"endedAt"`
}
type SafeSchema2SlotPeekCapabilityRevokedData struct {
	DispatchID           string `json:"dispatchId"`
	TargetLeaseID        string `json:"targetLeaseId"`
	BindingID            string `json:"bindingId"`
	SessionTargetID      string `json:"sessionTargetId"`
	TargetFingerprint    string `json:"targetFingerprint"`
	CapabilityGeneration string `json:"capabilityGeneration"`
	CapabilityIssuedSeq  uint64 `json:"capabilityIssuedSeq"`
	SteeringGeneration   string `json:"steeringGeneration"`
	Reason               string `json:"reason"`
	RevokedAt            string `json:"revokedAt"`
	InputClosed          bool   `json:"inputClosed"`
}
type SafeSchema2SlotReconciliationInterruptData struct {
	DispatchID         string `json:"dispatchId"`
	TargetLeaseID      string `json:"targetLeaseId"`
	BindingID          string `json:"bindingId"`
	SessionTargetID    string `json:"sessionTargetId"`
	TargetFingerprint  string `json:"targetFingerprint"`
	AuthorityKind      string `json:"authorityKind"`
	AuthoritySeq       uint64 `json:"authoritySeq"`
	InterruptEncoding  string `json:"interruptEncoding"`
	InterruptSHA256    string `json:"interruptSha256"`
	RecordedBeforeSend bool   `json:"recordedBeforeSend"`
}
type SafeSchema2SlotReconciliationInterruptOutcomeData struct {
	RequestedSeq      uint64 `json:"requestedSeq"`
	DispatchID        string `json:"dispatchId"`
	TargetLeaseID     string `json:"targetLeaseId"`
	TargetFingerprint string `json:"targetFingerprint"`
	Outcome           string `json:"outcome"`
	ObservedAt        string `json:"observedAt"`
}
type SafeSchema2SlotResultData struct {
	DispatchID                       string         `json:"dispatchId"`
	TargetLeaseID                    string         `json:"targetLeaseId"`
	TurnKey                          string         `json:"turnKey"`
	TurnPhase                        string         `json:"turnPhase"`
	NodeID                           string         `json:"nodeId"`
	Attempt                          uint64         `json:"attempt"`
	SlotID                           string         `json:"slotId"`
	AgentID                          string         `json:"agentId"`
	BindingID                        string         `json:"bindingId"`
	SessionTargetID                  string         `json:"sessionTargetId"`
	TargetFingerprint                string         `json:"targetFingerprint"`
	PaneHistoryBaselineEncoding      string         `json:"paneHistoryBaselineEncoding"`
	PaneHistoryBaselineDispatchSeq   uint64         `json:"paneHistoryBaselineDispatchSeq"`
	PaneHistoryBaselineSHA256        string         `json:"paneHistoryBaselineSha256"`
	PeekCapabilityRevokedSeq         uint64         `json:"peekCapabilityRevokedSeq"`
	SteeringGeneration               string         `json:"steeringGeneration"`
	OperatorInfluenced               bool           `json:"operatorInfluenced"`
	Status                           string         `json:"status"`
	TurnResult                       SafeTurnResult `json:"turnResult"`
	TurnResultEncoding               string         `json:"turnResultEncoding"`
	TurnResultSHA256                 string         `json:"turnResultSha256"`
	ClientAttachmentAuditProofSHA256 string         `json:"clientAttachmentAuditProofSha256"`
}
type SafeSchema2FormationResultData struct {
	NodeID                     string                 `json:"nodeId"`
	Attempt                    uint64                 `json:"attempt"`
	Status                     string                 `json:"status"`
	Outputs                    SafePayloadProjections `json:"outputs"`
	OutputHashes               SafeProjectionHashes   `json:"outputHashes"`
	ReportArtifactID           string                 `json:"reportArtifactId"`
	ArtifactIDs                []string               `json:"artifactIds"`
	DiffArtifactIDs            []string               `json:"diffArtifactIds"`
	ContributingSlotResultSeqs []uint64               `json:"contributingSlotResultSeqs"`
	ResultEncoding             string                 `json:"resultEncoding"`
	ResultSHA256               string                 `json:"resultSha256"`
}

type SafeSchema2ToolDispatchData struct {
	ToolLeaseID             string               `json:"toolLeaseId"`
	NodeID                  string               `json:"nodeId"`
	Attempt                 uint64               `json:"attempt"`
	ToolBindingID           string               `json:"toolBindingId"`
	InputManifestSHA256     string               `json:"inputManifestSha256"`
	InputHashes             SafeProjectionHashes `json:"inputHashes"`
	ProfileSHA256           string               `json:"profileSha256"`
	ParametersSHA256        string               `json:"parametersSha256"`
	PolicySHA256            string               `json:"policySha256"`
	DeterminismPolicySHA256 string               `json:"determinismPolicySha256"`
	ExecutionBundleSHA256   string               `json:"executionBundleSha256"`
	RecordedBeforeExecute   bool                 `json:"recordedBeforeExecute"`
}
type SafeSchema2ToolProcessLaunchData struct {
	ToolLeaseID         string `json:"toolLeaseId"`
	LaunchID            string `json:"launchId"`
	NodeID              string `json:"nodeId"`
	Attempt             uint64 `json:"attempt"`
	Generation          string `json:"generation"`
	RecordedBeforeSpawn bool   `json:"recordedBeforeSpawn"`
}
type SafeSchema2ToolResultData struct {
	ToolLeaseID           string                 `json:"toolLeaseId"`
	LaunchID              string                 `json:"launchId"`
	Generation            string                 `json:"generation"`
	NodeID                string                 `json:"nodeId"`
	Attempt               uint64                 `json:"attempt"`
	Status                string                 `json:"status"`
	Outputs               SafePayloadProjections `json:"outputs"`
	OutputHashes          SafeProjectionHashes   `json:"outputHashes"`
	ArtifactRegistrations []ArtifactProjection   `json:"artifactRegistrations"`
	Artifacts             []ArtifactProjection   `json:"artifacts"`
	DisplayEvidence       *[]SafeDisplayEvidence `json:"displayEvidence,omitempty"`
	Timing                SafeEventTiming        `json:"timing"`
}
type SafeSchema2NodeOutputData struct {
	NodeID           string                 `json:"nodeId"`
	Status           string                 `json:"status"`
	Outputs          SafePayloadProjections `json:"outputs"`
	ReportArtifactID string                 `json:"reportArtifactId,omitempty"`
	ArtifactIDs      []string               `json:"artifactIds"`
	DiffArtifactIDs  []string               `json:"diffArtifactIds"`
	ProducedBy       SafeProducedBy         `json:"producedBy"`
	Timing           SafeEventTiming        `json:"timing"`
	DeliveredEdges   []SafeDeliveredEdge    `json:"deliveredEdges"`
}
type SafeSchema2GateEvaluatingData struct {
	GateID              string                  `json:"gateId"`
	GateAttempt         uint64                  `json:"gateAttempt"`
	NodeID              string                  `json:"nodeId"`
	Kinds               []string                `json:"kinds"`
	CriterionProjection SafeCriterionProjection `json:"criterionProjection"`
	InputRef            SafeInputIdentity       `json:"inputRef"`
	JudgeChain          []string                `json:"judgeChain"`
	RevisionCycleID     string                  `json:"revisionCycleId,omitempty"`
	TriggerFeedbackID   string                  `json:"triggerFeedbackId,omitempty"`
	PriorGateSeq        uint64                  `json:"priorGateSeq,omitempty"`
}
type SafeSchema2GateKindResultData struct {
	GateID                  string             `json:"gateId"`
	GateAttempt             uint64             `json:"gateAttempt"`
	Kind                    string             `json:"kind"`
	Verdict                 string             `json:"verdict"`
	Reason                  string             `json:"reason"`
	Evidence                []SafeGateEvidence `json:"evidence"`
	EvaluatedInputRef       SafeInputIdentity  `json:"evaluatedInputRef"`
	ResultEncoding          string             `json:"resultEncoding"`
	ResultSHA256            string             `json:"resultSha256"`
	RelatedSeqs             []uint64           `json:"relatedSeqs"`
	GateBindingID           string             `json:"gateBindingId,omitempty"`
	InputSHA256             string             `json:"inputSha256,omitempty"`
	ProfileSHA256           string             `json:"profileSha256,omitempty"`
	EvaluatorBundleSHA256   string             `json:"evaluatorBundleSha256,omitempty"`
	ParametersSHA256        string             `json:"parametersSha256,omitempty"`
	PolicySHA256            string             `json:"policySha256,omitempty"`
	DeterminismPolicySHA256 string             `json:"determinismPolicySha256,omitempty"`
}
type SafeSchema2JudgeResultData struct {
	GateID          string          `json:"gateId"`
	GateAttempt     uint64          `json:"gateAttempt"`
	JudgeNodeID     string          `json:"judgeNodeId"`
	JudgeAttempt    uint64          `json:"judgeAttempt"`
	ChainIndex      uint64          `json:"chainIndex"`
	ContextEncoding string          `json:"contextEncoding"`
	ContextSHA256   string          `json:"contextSha256"`
	PriorResultSeqs []uint64        `json:"priorResultSeqs"`
	Result          SafeJudgeResult `json:"result"`
	ResultEncoding  string          `json:"resultEncoding"`
	ResultSHA256    string          `json:"resultSha256"`
}
type SafeSchema2JudgeAttemptFailedData struct {
	GateID          string   `json:"gateId"`
	GateAttempt     uint64   `json:"gateAttempt"`
	JudgeNodeID     string   `json:"judgeNodeId"`
	JudgeAttempt    uint64   `json:"judgeAttempt"`
	ChainIndex      uint64   `json:"chainIndex"`
	ContextSHA256   string   `json:"contextSha256"`
	PriorResultSeqs []uint64 `json:"priorResultSeqs"`
	Code            string   `json:"code"`
	Reason          string   `json:"reason"`
	RelatedSeq      uint64   `json:"relatedSeq"`
}
type SafeSchema2GateVerdictData struct {
	GateID            string                   `json:"gateId"`
	GateAttempt       uint64                   `json:"gateAttempt"`
	Verdict           string                   `json:"verdict"`
	PerKind           SafeGateKindVerdicts     `json:"perKind"`
	KindResultSeqs    SafeGateKindResultSeqs   `json:"kindResultSeqs"`
	EvaluatedInputRef SafeInputIdentity        `json:"evaluatedInputRef"`
	RoutePort         string                   `json:"routePort"`
	RoutedEdges       []string                 `json:"routedEdges"`
	Reason            string                   `json:"reason"`
	FeedbackPayload   *SafeGateFeedbackPayload `json:"feedbackPayload,omitempty"`
}
type SafeSchema2ArtifactAttachedData struct {
	ArtifactProjection ArtifactProjection `json:"artifactProjection"`
	Source             SafeArtifactSource `json:"source"`
}
type SafeSchema2ArtifactObservedData struct {
	ArtifactID   string           `json:"artifactId"`
	Availability string           `json:"availability"`
	Artifact     *SafeArtifactRef `json:"artifact,omitempty"`
	ErrorCode    string           `json:"errorCode,omitempty"`
	ObservedAt   string           `json:"observedAt"`
	RelatedSeq   uint64           `json:"relatedSeq"`
}
type SafeSchema2EscalationRaisedData struct {
	Trigger  string `json:"trigger"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Source   string `json:"source"`
	NodeID   string `json:"nodeId"`
	GateID   string `json:"gateId"`
	Blocks   bool   `json:"blocks"`
}
type SafeSchema2HumanInputRequestedData struct {
	GateID                  string                      `json:"gateId"`
	GateAttempt             uint64                      `json:"gateAttempt"`
	NodeID                  string                      `json:"nodeId"`
	PromptProjection        SafeFixedSystemProjection   `json:"promptProjection"`
	ChoiceProjections       SafeHumanChoiceProjections  `json:"choiceProjections"`
	RequestedBy             string                      `json:"requestedBy"`
	EvaluatedInputRef       SafeInputIdentity           `json:"evaluatedInputRef"`
	CompletedKindResultSeqs SafeCompletedKindResultSeqs `json:"completedKindResultSeqs"`
}
type SafeSchema2HumanVerdictRecordedData struct {
	CommandID            string `json:"commandId"`
	CommandPayloadSHA256 string `json:"commandPayloadSha256"`
	GateID               string `json:"gateId"`
	GateAttempt          uint64 `json:"gateAttempt"`
	NodeID               string `json:"nodeId"`
	Verdict              string `json:"verdict"`
	Reason               string `json:"reason"`
	RequestedSeq         uint64 `json:"requestedSeq"`
	DecidedBy            string `json:"decidedBy"`
}
type SafeSchema2ErrorData struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Boundary    string `json:"boundary"`
	ErrorScope  string `json:"errorScope"`
	NodeID      string `json:"nodeId,omitempty"`
	GateID      string `json:"gateId,omitempty"`
	SlotID      string `json:"slotId,omitempty"`
	ToolLeaseID string `json:"toolLeaseId,omitempty"`
	Recoverable bool   `json:"recoverable"`
	RelatedSeq  uint64 `json:"relatedSeq"`
}
type SafeSchema2RunBlockedData struct {
	Reason         string                    `json:"reason"`
	BlockScope     string                    `json:"blockScope"`
	BlockedNodeID  string                    `json:"blockedNodeId,omitempty"`
	BlockedGateID  string                    `json:"blockedGateId,omitempty"`
	ResumeAllowed  bool                      `json:"resumeAllowed"`
	ResumePolicy   string                    `json:"resumePolicy"`
	OpenDispatches []SafeSchema2OpenDispatch `json:"openDispatches"`
	RetryTargets   []SafeRetryTarget         `json:"retryTargets"`
	NextEpoch      uint64                    `json:"nextEpoch"`
}
type SafeSchema2RunCancelRequestedData struct {
	CommandID            string                    `json:"commandId"`
	CommandPayloadSHA256 string                    `json:"commandPayloadSha256"`
	Reason               string                    `json:"reason"`
	RequestedBy          string                    `json:"requestedBy"`
	OpenNodeAttempts     []SafeNodeAttemptSnapshot `json:"openNodeAttempts"`
	OpenSlotDispatches   []SafeSchema2OpenDispatch `json:"openSlotDispatches"`
	OpenToolLeases       []SafeToolLeaseSnapshot   `json:"openToolLeases"`
}
type SafeSchema2RunCanceledData struct {
	CancelRequestSeq         uint64                        `json:"cancelRequestSeq"`
	Reason                   string                        `json:"reason"`
	RequestedBy              string                        `json:"requestedBy"`
	NodeAttemptDispositions  []SafeNodeAttemptDisposition  `json:"nodeAttemptDispositions"`
	SlotDispatchDispositions []SafeSlotDispatchDisposition `json:"slotDispatchDispositions"`
	ReconciledToolLeases     []SafeToolLeaseDisposition    `json:"reconciledToolLeases"`
	Final                    bool                          `json:"final"`
}
type SafeSchema2RunFailureReconciliationStartedData struct {
	OriginCancelRequestSeq       uint64                    `json:"originCancelRequestSeq"`
	Code                         string                    `json:"code"`
	Reason                       string                    `json:"reason"`
	Unrecoverable                bool                      `json:"unrecoverable"`
	RelatedSeq                   uint64                    `json:"relatedSeq"`
	FailureCause                 SafeFailureCause          `json:"failureCause"`
	OpenNodeAttempts             []SafeNodeAttemptSnapshot `json:"openNodeAttempts"`
	OpenSlotDispatches           []SafeSchema2OpenDispatch `json:"openSlotDispatches"`
	OpenToolLeases               []SafeToolLeaseSnapshot   `json:"openToolLeases"`
	RecordedBeforeReconciliation bool                      `json:"recordedBeforeReconciliation"`
}
type SafeSchema2RunFailedData struct {
	FailureReconciliationSeq uint64                        `json:"failureReconciliationSeq"`
	Code                     string                        `json:"code"`
	Reason                   string                        `json:"reason"`
	Unrecoverable            bool                          `json:"unrecoverable"`
	RelatedSeq               uint64                        `json:"relatedSeq"`
	FailureCause             SafeFailureCause              `json:"failureCause"`
	NodeAttemptDispositions  []SafeNodeAttemptDisposition  `json:"nodeAttemptDispositions"`
	SlotDispatchDispositions []SafeSlotDispatchDisposition `json:"slotDispatchDispositions"`
	ToolLeaseDispositions    []SafeToolLeaseDisposition    `json:"toolLeaseDispositions"`
	Final                    bool                          `json:"final"`
}
type SafeSchema2RunSucceededData struct {
	SummaryArtifactID string   `json:"summaryArtifactId,omitempty"`
	OutputArtifactIDs []string `json:"outputArtifactIds"`
	Final             bool     `json:"final"`
}

type SafeSchema1RunStartedEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema1RunStartedData `json:"data"`
}

func (SafeSchema1RunStartedEvent) isSafeRunEvent() {}

type SafeSchema1RunResumedEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema1RunResumedData `json:"data"`
}

func (SafeSchema1RunResumedEvent) isSafeRunEvent() {}

type SafeSchema1NodeWaitingEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema1NodeWaitingData `json:"data"`
}

func (SafeSchema1NodeWaitingEvent) isSafeRunEvent() {}

type SafeSchema1NodeStartedEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema1NodeStartedData `json:"data"`
}

func (SafeSchema1NodeStartedEvent) isSafeRunEvent() {}

type SafeSchema1OrchestrationTeamEvent struct {
	safeEventEnvelope
	Type string                           `json:"type"`
	Data SafeSchema1OrchestrationTeamData `json:"data"`
}

func (SafeSchema1OrchestrationTeamEvent) isSafeRunEvent() {}

type SafeSchema1PeerPlaneEvent struct {
	safeEventEnvelope
	Type string                   `json:"type"`
	Data SafeSchema1PeerPlaneData `json:"data"`
}

func (SafeSchema1PeerPlaneEvent) isSafeRunEvent() {}

type SafeSchema1SlotDispatchEvent struct {
	safeEventEnvelope
	Type string                      `json:"type"`
	Data SafeSchema1SlotDispatchData `json:"data"`
}

func (SafeSchema1SlotDispatchEvent) isSafeRunEvent() {}

type SafeSchema1AdapterSendEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema1AdapterSendData `json:"data"`
}

func (SafeSchema1AdapterSendEvent) isSafeRunEvent() {}

type SafeSchema1SlotResultEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema1SlotResultData `json:"data"`
}

func (SafeSchema1SlotResultEvent) isSafeRunEvent() {}

type SafeSchema1NodeOutputEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema1NodeOutputData `json:"data"`
}

func (SafeSchema1NodeOutputEvent) isSafeRunEvent() {}

type SafeSchema1GateEvaluatingEvent struct {
	safeEventEnvelope
	Type string                        `json:"type"`
	Data SafeSchema1GateEvaluatingData `json:"data"`
}

func (SafeSchema1GateEvaluatingEvent) isSafeRunEvent() {}

type SafeSchema1GateVerdictEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema1GateVerdictData `json:"data"`
}

func (SafeSchema1GateVerdictEvent) isSafeRunEvent() {}

type SafeSchema1VerificationVerdictEvent struct {
	safeEventEnvelope
	Type string                             `json:"type"`
	Data SafeSchema1VerificationVerdictData `json:"data"`
}

func (SafeSchema1VerificationVerdictEvent) isSafeRunEvent() {}

type SafeSchema1EscalationRaisedEvent struct {
	safeEventEnvelope
	Type string                          `json:"type"`
	Data SafeSchema1EscalationRaisedData `json:"data"`
}

func (SafeSchema1EscalationRaisedEvent) isSafeRunEvent() {}

type SafeSchema1HumanInputRequestedEvent struct {
	safeEventEnvelope
	Type string                             `json:"type"`
	Data SafeSchema1HumanInputRequestedData `json:"data"`
}

func (SafeSchema1HumanInputRequestedEvent) isSafeRunEvent() {}

type SafeSchema1HumanVerdictRecordedEvent struct {
	safeEventEnvelope
	Type string                              `json:"type"`
	Data SafeSchema1HumanVerdictRecordedData `json:"data"`
}

func (SafeSchema1HumanVerdictRecordedEvent) isSafeRunEvent() {}

type SafeSchema1ErrorEvent struct {
	safeEventEnvelope
	Type string               `json:"type"`
	Data SafeSchema1ErrorData `json:"data"`
}

func (SafeSchema1ErrorEvent) isSafeRunEvent() {}

type SafeSchema1RunBlockedEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema1RunBlockedData `json:"data"`
}

func (SafeSchema1RunBlockedEvent) isSafeRunEvent() {}

type SafeSchema1RunCanceledEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema1RunCanceledData `json:"data"`
}

func (SafeSchema1RunCanceledEvent) isSafeRunEvent() {}

type SafeSchema1RunFailedEvent struct {
	safeEventEnvelope
	Type string                   `json:"type"`
	Data SafeSchema1RunFailedData `json:"data"`
}

func (SafeSchema1RunFailedEvent) isSafeRunEvent() {}

type SafeSchema1RunSucceededEvent struct {
	safeEventEnvelope
	Type string                      `json:"type"`
	Data SafeSchema1RunSucceededData `json:"data"`
}

func (SafeSchema1RunSucceededEvent) isSafeRunEvent() {}

type SafeSchema2RunStartedEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema2RunStartedData `json:"data"`
}

func (SafeSchema2RunStartedEvent) isSafeRunEvent() {}

type SafeSchema2RunActivatedEvent struct {
	safeEventEnvelope
	Type string                      `json:"type"`
	Data SafeSchema2RunActivatedData `json:"data"`
}

func (SafeSchema2RunActivatedEvent) isSafeRunEvent() {}

type SafeSchema2RunResumedEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema2RunResumedData `json:"data"`
}

func (SafeSchema2RunResumedEvent) isSafeRunEvent() {}

type SafeSchema2NodeWaitingEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema2NodeWaitingData `json:"data"`
}

func (SafeSchema2NodeWaitingEvent) isSafeRunEvent() {}

type SafeSchema2NodeInputIgnoredEvent struct {
	safeEventEnvelope
	Type string                          `json:"type"`
	Data SafeSchema2NodeInputIgnoredData `json:"data"`
}

func (SafeSchema2NodeInputIgnoredEvent) isSafeRunEvent() {}

type SafeSchema2NodeStartedEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema2NodeStartedData `json:"data"`
}

func (SafeSchema2NodeStartedEvent) isSafeRunEvent() {}

type SafeSchema2SlotBindingObservedEvent struct {
	safeEventEnvelope
	Type string                             `json:"type"`
	Data SafeSchema2SlotBindingObservedData `json:"data"`
}

func (SafeSchema2SlotBindingObservedEvent) isSafeRunEvent() {}

type SafeSchema2SlotDispatchEvent struct {
	safeEventEnvelope
	Type string                      `json:"type"`
	Data SafeSchema2SlotDispatchData `json:"data"`
}

func (SafeSchema2SlotDispatchEvent) isSafeRunEvent() {}

type SafeSchema2SlotPeekCapabilityIssuedEvent struct {
	safeEventEnvelope
	Type string                                  `json:"type"`
	Data SafeSchema2SlotPeekCapabilityIssuedData `json:"data"`
}

func (SafeSchema2SlotPeekCapabilityIssuedEvent) isSafeRunEvent() {}

type SafeSchema2SlotSteeringStartedEvent struct {
	safeEventEnvelope
	Type string                             `json:"type"`
	Data SafeSchema2SlotSteeringStartedData `json:"data"`
}

func (SafeSchema2SlotSteeringStartedEvent) isSafeRunEvent() {}

type SafeSchema2SlotSteeringEndedEvent struct {
	safeEventEnvelope
	Type string                           `json:"type"`
	Data SafeSchema2SlotSteeringEndedData `json:"data"`
}

func (SafeSchema2SlotSteeringEndedEvent) isSafeRunEvent() {}

type SafeSchema2SlotPeekCapabilityRevokedEvent struct {
	safeEventEnvelope
	Type string                                   `json:"type"`
	Data SafeSchema2SlotPeekCapabilityRevokedData `json:"data"`
}

func (SafeSchema2SlotPeekCapabilityRevokedEvent) isSafeRunEvent() {}

type SafeSchema2SlotReconciliationInterruptEvent struct {
	safeEventEnvelope
	Type string                                     `json:"type"`
	Data SafeSchema2SlotReconciliationInterruptData `json:"data"`
}

func (SafeSchema2SlotReconciliationInterruptEvent) isSafeRunEvent() {}

type SafeSchema2SlotReconciliationInterruptOutcomeEvent struct {
	safeEventEnvelope
	Type string                                            `json:"type"`
	Data SafeSchema2SlotReconciliationInterruptOutcomeData `json:"data"`
}

func (SafeSchema2SlotReconciliationInterruptOutcomeEvent) isSafeRunEvent() {}

type SafeSchema2SlotResultEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema2SlotResultData `json:"data"`
}

func (SafeSchema2SlotResultEvent) isSafeRunEvent() {}

type SafeSchema2FormationResultEvent struct {
	safeEventEnvelope
	Type string                         `json:"type"`
	Data SafeSchema2FormationResultData `json:"data"`
}

func (SafeSchema2FormationResultEvent) isSafeRunEvent() {}

type SafeSchema2ToolDispatchEvent struct {
	safeEventEnvelope
	Type string                      `json:"type"`
	Data SafeSchema2ToolDispatchData `json:"data"`
}

func (SafeSchema2ToolDispatchEvent) isSafeRunEvent() {}

type SafeSchema2ToolProcessLaunchEvent struct {
	safeEventEnvelope
	Type string                           `json:"type"`
	Data SafeSchema2ToolProcessLaunchData `json:"data"`
}

func (SafeSchema2ToolProcessLaunchEvent) isSafeRunEvent() {}

type SafeSchema2ToolResultEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema2ToolResultData `json:"data"`
}

func (SafeSchema2ToolResultEvent) isSafeRunEvent() {}

type SafeSchema2NodeOutputEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema2NodeOutputData `json:"data"`
}

func (SafeSchema2NodeOutputEvent) isSafeRunEvent() {}

type SafeSchema2GateEvaluatingEvent struct {
	safeEventEnvelope
	Type string                        `json:"type"`
	Data SafeSchema2GateEvaluatingData `json:"data"`
}

func (SafeSchema2GateEvaluatingEvent) isSafeRunEvent() {}

type SafeSchema2GateKindResultEvent struct {
	safeEventEnvelope
	Type string                        `json:"type"`
	Data SafeSchema2GateKindResultData `json:"data"`
}

func (SafeSchema2GateKindResultEvent) isSafeRunEvent() {}

type SafeSchema2JudgeResultEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema2JudgeResultData `json:"data"`
}

func (SafeSchema2JudgeResultEvent) isSafeRunEvent() {}

type SafeSchema2JudgeAttemptFailedEvent struct {
	safeEventEnvelope
	Type string                            `json:"type"`
	Data SafeSchema2JudgeAttemptFailedData `json:"data"`
}

func (SafeSchema2JudgeAttemptFailedEvent) isSafeRunEvent() {}

type SafeSchema2GateVerdictEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema2GateVerdictData `json:"data"`
}

func (SafeSchema2GateVerdictEvent) isSafeRunEvent() {}

type SafeSchema2ArtifactAttachedEvent struct {
	safeEventEnvelope
	Type string                          `json:"type"`
	Data SafeSchema2ArtifactAttachedData `json:"data"`
}

func (SafeSchema2ArtifactAttachedEvent) isSafeRunEvent() {}

type SafeSchema2ArtifactObservedEvent struct {
	safeEventEnvelope
	Type string                          `json:"type"`
	Data SafeSchema2ArtifactObservedData `json:"data"`
}

func (SafeSchema2ArtifactObservedEvent) isSafeRunEvent() {}

type SafeSchema2EscalationRaisedEvent struct {
	safeEventEnvelope
	Type string                          `json:"type"`
	Data SafeSchema2EscalationRaisedData `json:"data"`
}

func (SafeSchema2EscalationRaisedEvent) isSafeRunEvent() {}

type SafeSchema2HumanInputRequestedEvent struct {
	safeEventEnvelope
	Type string                             `json:"type"`
	Data SafeSchema2HumanInputRequestedData `json:"data"`
}

func (SafeSchema2HumanInputRequestedEvent) isSafeRunEvent() {}

type SafeSchema2HumanVerdictRecordedEvent struct {
	safeEventEnvelope
	Type string                              `json:"type"`
	Data SafeSchema2HumanVerdictRecordedData `json:"data"`
}

func (SafeSchema2HumanVerdictRecordedEvent) isSafeRunEvent() {}

type SafeSchema2ErrorEvent struct {
	safeEventEnvelope
	Type string               `json:"type"`
	Data SafeSchema2ErrorData `json:"data"`
}

func (SafeSchema2ErrorEvent) isSafeRunEvent() {}

type SafeSchema2RunBlockedEvent struct {
	safeEventEnvelope
	Type string                    `json:"type"`
	Data SafeSchema2RunBlockedData `json:"data"`
}

func (SafeSchema2RunBlockedEvent) isSafeRunEvent() {}

type SafeSchema2RunCancelRequestedEvent struct {
	safeEventEnvelope
	Type string                            `json:"type"`
	Data SafeSchema2RunCancelRequestedData `json:"data"`
}

func (SafeSchema2RunCancelRequestedEvent) isSafeRunEvent() {}

type SafeSchema2RunCanceledEvent struct {
	safeEventEnvelope
	Type string                     `json:"type"`
	Data SafeSchema2RunCanceledData `json:"data"`
}

func (SafeSchema2RunCanceledEvent) isSafeRunEvent() {}

type SafeSchema2RunFailureReconciliationStartedEvent struct {
	safeEventEnvelope
	Type string                                         `json:"type"`
	Data SafeSchema2RunFailureReconciliationStartedData `json:"data"`
}

func (SafeSchema2RunFailureReconciliationStartedEvent) isSafeRunEvent() {}

type SafeSchema2RunFailedEvent struct {
	safeEventEnvelope
	Type string                   `json:"type"`
	Data SafeSchema2RunFailedData `json:"data"`
}

func (SafeSchema2RunFailedEvent) isSafeRunEvent() {}

type SafeSchema2RunSucceededEvent struct {
	safeEventEnvelope
	Type string                      `json:"type"`
	Data SafeSchema2RunSucceededData `json:"data"`
}

func (SafeSchema2RunSucceededEvent) isSafeRunEvent() {}
