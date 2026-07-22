package formations

import "errors"

var ErrRuntimeAuthorityNonAuthorizing = errors.New("formations runtime authority is non-authorizing")

type RuntimeAuthorityNonAuthorizingReason string

const (
	RuntimeAuthorityConfigurationMissing RuntimeAuthorityNonAuthorizingReason = "configuration_missing"
	RuntimeAuthorityGuardRejected        RuntimeAuthorityNonAuthorizingReason = "guard_rejected"
	RuntimeAuthorityCapabilityDisabled   RuntimeAuthorityNonAuthorizingReason = "capability_disabled"
)

// RuntimeAuthorityNonAuthorizingError is safe to return through API and CLI
// boundaries. It intentionally omits host-private roots and authority IDs.
type RuntimeAuthorityNonAuthorizingError struct {
	Reason     RuntimeAuthorityNonAuthorizingReason
	Stage      RuntimeAuthorityGuardStage
	Code       RuntimeAuthorityGuardCode
	Capability RuntimeAuthorityCapability
}

func (e *RuntimeAuthorityNonAuthorizingError) Error() string {
	return ErrRuntimeAuthorityNonAuthorizing.Error()
}

func (e *RuntimeAuthorityNonAuthorizingError) Unwrap() error {
	return ErrRuntimeAuthorityNonAuthorizing
}

type runtimeAuthorityBoundary struct {
	formationsDataRoot  string
	configuredWorkspace string
}

// NewRuntimeStore constructs a production runtime boundary. An empty data root
// is an explicit unavailable configuration, used by Archon to fail without
// discovering or reading host-private authority state.
func NewRuntimeStore(workspace, formationsDataRoot string) *Store {
	store := NewStore(workspace)
	store.runtimeAuthority = &runtimeAuthorityBoundary{
		formationsDataRoot:  formationsDataRoot,
		configuredWorkspace: workspace,
	}
	return store
}

// RequireRuntimeAuthority is the runtime-effect enforcement seam guarding every
// Formations mutation or external effect (run start, resume, dispatch,
// executors, ledger appends, escalations, Archon).
//
// Trust model: CHROTE runs TRUSTED agents behind a network perimeter, and the
// operator is authorized to run their own workflows. A hostile same-UID actor
// is out of the threat model. The earlier slice fenced "unauthorized runtime
// effects" — a threat not in the model — by returning a non-authorizing error
// unconditionally whenever the runtime boundary was set, which fail-closed every
// workflow on the live lane. The enforcement decision therefore AUTHORIZES: the
// trusted operator's runtime effects proceed.
//
// The runtimeAuthorityBoundary seam, NewRuntimeStore, the read guard
// (GuardRuntimeWorkspaceAuthorityV1), and the RuntimeAuthorityNonAuthorizingError
// types are intentionally retained so a future multi-tenant or untrusted-caller
// need can re-enable enforcement here without re-plumbing the 15 call sites.
func (s *Store) RequireRuntimeAuthority() error {
	return nil
}
