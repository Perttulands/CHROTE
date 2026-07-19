package formations

import "os"

const workspaceAuthorityCapabilityV1 = "formations.workspace-authority.v1"

// workspaceAuthorityCapability is binary-owned process state. It is not a
// persisted or public capability descriptor.
type workspaceAuthorityCapability struct {
	id                 string
	registration       bool
	privatePublication bool
	ownerLease         bool
	fencing            bool
	commandJournal     bool
	semanticProjection bool
	reconciliation     bool
	cleanup            bool
	quarantine         bool
	execution          bool
}

type workspaceAuthorityCapabilityGate struct {
	capabilities []workspaceAuthorityCapability
}

func newWorkspaceAuthorityCapabilityGate() workspaceAuthorityCapabilityGate {
	return workspaceAuthorityCapabilityGate{capabilities: codeOwnedWorkspaceAuthorityCapabilities()}
}

func codeOwnedWorkspaceAuthorityCapabilities() []workspaceAuthorityCapability {
	return []workspaceAuthorityCapability{
		{id: RuntimeAuthorityGuardCapabilityV1},
		{
			id:                 workspaceAuthorityCapabilityV1,
			registration:       true,
			privatePublication: true,
			ownerLease:         true,
			fencing:            true,
			commandJournal:     true,
		},
	}
}

func (gate workspaceAuthorityCapabilityGate) beforeMutation(callback func() error) error {
	if callback == nil {
		return errRuntimeNoncanonical
	}
	expected := codeOwnedWorkspaceAuthorityCapabilities()
	if len(gate.capabilities) != len(expected) {
		return errRuntimeUnsupportedSchema
	}
	for index := range expected {
		if gate.capabilities[index] != expected[index] {
			return errRuntimeUnsupportedSchema
		}
	}
	return callback()
}

func productionAuthorityWriterUID() uint32 {
	return uint32(os.Geteuid())
}
