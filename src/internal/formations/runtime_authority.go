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

// RequireRuntimeAuthority fences every runtime mutation or external effect.
// The guard capability is intentionally all-false in this slice, so even an
// exact valid authority match returns a typed non-authorizing error.
func (s *Store) RequireRuntimeAuthority() error {
	if s == nil || s.runtimeAuthority == nil {
		return nil
	}
	boundary := s.runtimeAuthority
	if boundary.formationsDataRoot == "" {
		return &RuntimeAuthorityNonAuthorizingError{
			Reason:     RuntimeAuthorityConfigurationMissing,
			Stage:      RuntimeAuthorityGuardStageRoot,
			Code:       RuntimeAuthorityGuardMissing,
			Capability: disabledRuntimeAuthorityCapability(),
		}
	}
	if s.Workspace != boundary.configuredWorkspace {
		return &RuntimeAuthorityNonAuthorizingError{
			Reason:     RuntimeAuthorityGuardRejected,
			Stage:      RuntimeAuthorityGuardStageRegistry,
			Code:       RuntimeAuthorityGuardConflict,
			Capability: disabledRuntimeAuthorityCapability(),
		}
	}
	result, err := GuardRuntimeWorkspaceAuthorityV1(boundary.formationsDataRoot, boundary.configuredWorkspace)
	if err != nil {
		rejection := &RuntimeAuthorityNonAuthorizingError{
			Reason:     RuntimeAuthorityGuardRejected,
			Capability: result.Capability,
		}
		var guardErr *RuntimeAuthorityGuardError
		if errors.As(err, &guardErr) {
			rejection.Stage = guardErr.Stage
			rejection.Code = guardErr.Code
		}
		return rejection
	}
	return &RuntimeAuthorityNonAuthorizingError{
		Reason:     RuntimeAuthorityCapabilityDisabled,
		Capability: result.Capability,
	}
}
