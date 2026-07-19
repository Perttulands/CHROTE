package formations

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
)

// workspaceAuthorityOwnerDomainScope exposes only closed values observed while
// the exact mapped owner lock is held. It intentionally carries no path,
// descriptor, registrar, or mutable authority record.
type workspaceAuthorityOwnerDomainScope interface {
	ownerLockIdentity() (uint64, uint64)
	workspaceAuthorityID() string
	workspaceIdentity() (uint64, uint64, string)
}

type workspaceAuthorityOwnerDomainObservation struct {
	authorityID   string
	ownerDevice   uint64
	ownerInode    uint64
	workspaceHash string
	workspaceDev  uint64
	workspaceIno  uint64
}

func (scope workspaceAuthorityOwnerDomainObservation) ownerLockIdentity() (uint64, uint64) {
	return scope.ownerDevice, scope.ownerInode
}

func (scope workspaceAuthorityOwnerDomainObservation) workspaceAuthorityID() string {
	return scope.authorityID
}

func (scope workspaceAuthorityOwnerDomainObservation) workspaceIdentity() (uint64, uint64, string) {
	return scope.workspaceDev, scope.workspaceIno, scope.workspaceHash
}

type workspaceAuthorityOwnerDomainPolicy struct {
	file     *os.File
	revision uint64
}

type workspaceAuthorityOwnerDomainState struct {
	authorityDirectory *os.File
	bootstrap          *os.File
	workspaceAuthority *os.File
	policyDirectory    *os.File
	policies           []workspaceAuthorityOwnerDomainPolicy
}

func (state *workspaceAuthorityOwnerDomainState) close() {
	if state == nil {
		return
	}
	for index := len(state.policies) - 1; index >= 0; index-- {
		_ = state.policies[index].file.Close()
	}
	if state.policyDirectory != nil {
		_ = state.policyDirectory.Close()
	}
	if state.workspaceAuthority != nil {
		_ = state.workspaceAuthority.Close()
	}
	if state.bootstrap != nil {
		_ = state.bootstrap.Close()
	}
}

func (registrar *workspaceAuthorityRegistrar) withWorkspaceAuthorityOwnerDomain(
	configuredWorkspace string,
	callback func(workspaceAuthorityOwnerDomainScope) error,
) error {
	if registrar == nil || callback == nil || registrar.ops.openWorkspace == nil || registrar.ops.validatePrivateNode == nil {
		return errRuntimeNoncanonical
	}
	return registrar.gate.beforeMutation(func() error {
		registrar.local.Lock()
		defer registrar.local.Unlock()
		return registrar.withWorkspaceAuthorityOwnerDomainCapability(configuredWorkspace, callback)
	})
}

func (registrar *workspaceAuthorityRegistrar) withWorkspaceAuthorityOwnerDomainCapability(
	configuredWorkspace string,
	callback func(workspaceAuthorityOwnerDomainScope) error,
) error {
	root, err := openRuntimeAuthorityRoot(registrar.hostRoot)
	if err != nil {
		return workspaceAuthorityRegistrationOpenError("host root", err)
	}
	defer root.Close()
	if err := registrar.ops.validatePrivateNode(root, registrar.expectedUID); err != nil {
		return err
	}

	workspaces, err := openRuntimeAuthorityDirectoryAt(root, "workspaces")
	if err != nil {
		return workspaceAuthorityRegistrationOpenError("workspaces root", err)
	}
	defer workspaces.Close()
	if err := registrar.ops.validatePrivateNode(workspaces, registrar.expectedUID); err != nil {
		return err
	}

	registryLock, err := openAuthorityPrivateFileAt(workspaces, "registry.lock", syscall.O_RDWR, false, registrar.expectedUID)
	if err != nil {
		return workspaceAuthorityRegistrationOpenError("registry lock", err)
	}
	defer registryLock.Close()
	if err := registrar.ops.validatePrivateNode(registryLock, registrar.expectedUID); err != nil {
		return err
	}
	if err := syscall.Flock(int(registryLock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	registryLocked := true
	defer func() {
		if registryLocked {
			_ = syscall.Flock(int(registryLock.Fd()), syscall.LOCK_UN)
		}
	}()

	registry, err := openRuntimeAuthorityFileAt(workspaces, "registry.private.json")
	if err != nil {
		return workspaceAuthorityRegistrationOpenError("private registry", err)
	}
	defer registry.Close()
	if err := registrar.ops.validatePrivateNode(registry, registrar.expectedUID); err != nil {
		return err
	}
	strictRegistry, err := readWorkspaceAuthorityRegistrationRegistry(registry)
	if err != nil {
		return err
	}
	projectedRegistry := projectRuntimeWorkspaceRegistry(strictRegistry)
	if err := validateRuntimeWorkspaceRegistry(&projectedRegistry); err != nil {
		return err
	}

	return withOpenedWorkspaceAuthorityIdentity(configuredWorkspace, registrar.ops.openWorkspace, func(workspace *os.File, identity runtimeWorkspaceIdentity) error {
		if err := registrar.validateWorkspaceAuthorityOwnerDomainGlobalPins(root, workspaces, registryLock, registry, workspace, identity); err != nil {
			return err
		}
		entry, err := matchRuntimeWorkspaceRegistryEntry(projectedRegistry, identity)
		if err != nil {
			return err
		}

		authorityDirectory, err := openRuntimeAuthorityDirectoryAt(workspaces, entry.WorkspaceAuthorityID)
		if err != nil {
			return workspaceAuthorityRegistrationOpenError("mapped authority directory", err)
		}
		defer authorityDirectory.Close()
		if err := registrar.validateWorkspaceAuthorityOwnerDomainNamed(authorityDirectory, workspaces, entry.WorkspaceAuthorityID); err != nil {
			return err
		}

		ownerLock, err := openAuthorityPrivateFileAt(authorityDirectory, "owner.lock", syscall.O_RDWR, false, registrar.expectedUID)
		if err != nil {
			return workspaceAuthorityRegistrationOpenError("mapped owner lock", err)
		}
		defer ownerLock.Close()
		if err := registrar.validateWorkspaceAuthorityOwnerDomainNamed(ownerLock, authorityDirectory, "owner.lock"); err != nil {
			return err
		}
		if err := syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_EX); err != nil {
			return err
		}
		defer syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_UN) //nolint:errcheck // close also releases the scoped lock

		if err := registrar.validateWorkspaceAuthorityOwnerDomainAcquiredPins(
			root,
			workspaces,
			registryLock,
			registry,
			workspace,
			identity,
			authorityDirectory,
			entry.WorkspaceAuthorityID,
			ownerLock,
		); err != nil {
			return err
		}
		if err := syscall.Flock(int(registryLock.Fd()), syscall.LOCK_UN); err != nil {
			return err
		}
		registryLocked = false

		ownerRaw, err := readWorkspaceAuthorityRecoveryPrivateFile(ownerLock)
		if err != nil {
			return err
		}
		if len(ownerRaw) != 0 {
			return fmt.Errorf("%w: owner lock contains authority state", errRuntimeConflict)
		}

		state, err := registrar.openWorkspaceAuthorityOwnerDomainState(authorityDirectory, entry)
		if err != nil {
			return err
		}
		defer state.close()

		ownerDevice, ownerInode, err := workspaceAuthorityRegistrationFileIdentity(ownerLock)
		if err != nil {
			return err
		}
		scope := workspaceAuthorityOwnerDomainObservation{
			authorityID:   entry.WorkspaceAuthorityID,
			ownerDevice:   ownerDevice,
			ownerInode:    ownerInode,
			workspaceHash: identity.rootHash,
			workspaceDev:  identity.device,
			workspaceIno:  identity.inode,
		}
		if err := registrar.validateWorkspaceAuthorityOwnerDomainFinalPins(
			root,
			workspaces,
			registryLock,
			registry,
			workspace,
			identity,
			state,
			entry.WorkspaceAuthorityID,
			ownerLock,
		); err != nil {
			return err
		}
		return callback(scope)
	})
}

func (registrar *workspaceAuthorityRegistrar) openWorkspaceAuthorityOwnerDomainState(
	authorityDirectory *os.File,
	entry runtimeWorkspaceRegistryEntry,
) (_ *workspaceAuthorityOwnerDomainState, err error) {
	state := &workspaceAuthorityOwnerDomainState{authorityDirectory: authorityDirectory}
	complete := false
	defer func() {
		if !complete {
			state.close()
		}
	}()

	state.bootstrap, err = openAuthorityPrivateFileAt(authorityDirectory, "workspace.bootstrap.json", syscall.O_RDONLY, false, registrar.expectedUID)
	if err != nil {
		return nil, workspaceAuthorityRegistrationOpenError("workspace bootstrap", err)
	}
	if err := registrar.ops.validatePrivateNode(state.bootstrap, registrar.expectedUID); err != nil {
		return nil, err
	}
	bootstrapRaw, err := readWorkspaceAuthorityRecoveryPrivateFile(state.bootstrap)
	if err != nil {
		return nil, err
	}
	var bootstrap runtimeWorkspaceBootstrap
	if err := decodeRuntimeAuthorityJSON(bootstrapRaw, &bootstrap); err != nil {
		return nil, err
	}
	if err := validateRuntimeBootstrap(bootstrapRaw, bootstrap, entry); err != nil {
		return nil, err
	}

	state.workspaceAuthority, err = openAuthorityPrivateFileAt(authorityDirectory, "workspace.private.json", syscall.O_RDONLY, false, registrar.expectedUID)
	if err != nil {
		return nil, workspaceAuthorityRegistrationOpenError("workspace authority", err)
	}
	if err := registrar.ops.validatePrivateNode(state.workspaceAuthority, registrar.expectedUID); err != nil {
		return nil, err
	}
	workspaceRaw, err := readWorkspaceAuthorityRecoveryPrivateFile(state.workspaceAuthority)
	if err != nil {
		return nil, err
	}
	strictWorkspace, err := decodeWorkspaceAuthorityJCSV1(workspaceRaw)
	if err != nil {
		return nil, err
	}
	workspaceAuthority := projectRuntimeWorkspaceAuthority(strictWorkspace)
	if err := validateRuntimeWorkspaceAuthority(workspaceAuthority, entry); err != nil {
		return nil, err
	}

	state.policyDirectory, err = openRuntimeAuthorityDirectoryAt(authorityDirectory, "admission-policies")
	if err != nil {
		return nil, workspaceAuthorityRegistrationOpenError("admission policies", err)
	}
	if err := registrar.ops.validatePrivateNode(state.policyDirectory, registrar.expectedUID); err != nil {
		return nil, err
	}

	expectedHash := strictWorkspace.AdmissionPolicyRef.PolicySHA256
	for revision := strictWorkspace.AdmissionPolicyRef.PolicyRev; revision >= 1; revision-- {
		name := strconv.FormatUint(revision, 10) + ".json"
		policyFile, err := openAuthorityPrivateFileAt(state.policyDirectory, name, syscall.O_RDONLY, false, registrar.expectedUID)
		if err != nil {
			return nil, workspaceAuthorityRegistrationOpenError("admission policy "+name, err)
		}
		state.policies = append(state.policies, workspaceAuthorityOwnerDomainPolicy{file: policyFile, revision: revision})
		if err := registrar.ops.validatePrivateNode(policyFile, registrar.expectedUID); err != nil {
			return nil, err
		}
		policyRaw, err := readWorkspaceAuthorityRecoveryPrivateFile(policyFile)
		if err != nil {
			return nil, err
		}
		if runtimeSHA256Hex(policyRaw) != expectedHash {
			return nil, errRuntimeIntegrityMismatch
		}
		var policy runtimeWorkspaceAdmissionPolicy
		if err := decodeRuntimeAuthorityJSON(policyRaw, &policy); err != nil {
			return nil, err
		}
		if err := validateRuntimeAdmissionPolicy(policyRaw, policy, revision); err != nil {
			return nil, err
		}
		if revision == 1 {
			break
		}
		expectedHash = policy.PriorPolicySHA256
	}

	complete = true
	return state, nil
}

func (registrar *workspaceAuthorityRegistrar) validateWorkspaceAuthorityOwnerDomainGlobalPins(
	root *os.File,
	workspaces *os.File,
	registryLock *os.File,
	registry *os.File,
	workspace *os.File,
	identity runtimeWorkspaceIdentity,
) error {
	for _, opened := range []*os.File{root, workspaces, registryLock, registry} {
		if err := registrar.ops.validatePrivateNode(opened, registrar.expectedUID); err != nil {
			return err
		}
	}
	if err := validateWorkspaceAuthorityRegistrationPins(
		registrar.hostRoot,
		root,
		workspaces,
		registryLock,
		registry,
		identity,
		workspace,
	); err != nil {
		return err
	}
	return validateRuntimeAuthorityWorkspaceIsolation(registrar.hostRoot, root, identity)
}

func (registrar *workspaceAuthorityRegistrar) validateWorkspaceAuthorityOwnerDomainAcquiredPins(
	root *os.File,
	workspaces *os.File,
	registryLock *os.File,
	registry *os.File,
	workspace *os.File,
	identity runtimeWorkspaceIdentity,
	authorityDirectory *os.File,
	authorityID string,
	ownerLock *os.File,
) error {
	if err := registrar.validateWorkspaceAuthorityOwnerDomainGlobalPins(root, workspaces, registryLock, registry, workspace, identity); err != nil {
		return err
	}
	if err := registrar.validateWorkspaceAuthorityOwnerDomainNamed(authorityDirectory, workspaces, authorityID); err != nil {
		return err
	}
	return registrar.validateWorkspaceAuthorityOwnerDomainNamed(ownerLock, authorityDirectory, "owner.lock")
}

func (registrar *workspaceAuthorityRegistrar) validateWorkspaceAuthorityOwnerDomainFinalPins(
	root *os.File,
	workspaces *os.File,
	registryLock *os.File,
	registry *os.File,
	workspace *os.File,
	identity runtimeWorkspaceIdentity,
	state *workspaceAuthorityOwnerDomainState,
	authorityID string,
	ownerLock *os.File,
) error {
	if state == nil || state.authorityDirectory == nil {
		return errRuntimeNoncanonical
	}
	if err := registrar.validateWorkspaceAuthorityOwnerDomainGlobalPins(root, workspaces, registryLock, registry, workspace, identity); err != nil {
		return err
	}
	if err := registrar.validateWorkspaceAuthorityOwnerDomainNamed(state.authorityDirectory, workspaces, authorityID); err != nil {
		return err
	}
	checks := []struct {
		opened *os.File
		parent *os.File
		name   string
	}{
		{opened: state.bootstrap, parent: state.authorityDirectory, name: "workspace.bootstrap.json"},
		{opened: state.workspaceAuthority, parent: state.authorityDirectory, name: "workspace.private.json"},
		{opened: state.policyDirectory, parent: state.authorityDirectory, name: "admission-policies"},
	}
	for _, policy := range state.policies {
		checks = append(checks, struct {
			opened *os.File
			parent *os.File
			name   string
		}{
			opened: policy.file,
			parent: state.policyDirectory,
			name:   strconv.FormatUint(policy.revision, 10) + ".json",
		})
	}
	for _, check := range checks {
		if err := registrar.validateWorkspaceAuthorityOwnerDomainNamed(check.opened, check.parent, check.name); err != nil {
			return err
		}
	}
	return registrar.validateWorkspaceAuthorityOwnerDomainNamed(ownerLock, state.authorityDirectory, "owner.lock")
}

func (registrar *workspaceAuthorityRegistrar) validateWorkspaceAuthorityOwnerDomainNamed(opened, parent *os.File, name string) error {
	if err := registrar.ops.validatePrivateNode(opened, registrar.expectedUID); err != nil {
		return err
	}
	return workspaceAuthorityRecoveryOpenedMatchesNamed(opened, parent, name)
}
