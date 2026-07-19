package formations

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"syscall"
)

type workspaceAuthorityRecoveryCandidate struct {
	authorityID string
	directory   *os.File
	bootstrap   *os.File
}

type workspaceAuthorityRecoveryTransaction struct {
	registrar         *workspaceAuthorityRegistrar
	root              *os.File
	workspaces        *os.File
	registryLock      *os.File
	registry          *os.File
	workspace         *os.File
	identity          runtimeWorkspaceIdentity
	candidate         *workspaceAuthorityRecoveryCandidate
	currentGeneration authorityGeneration
	nextRegistryRaw   []byte
	policyRaw         []byte
	workspaceRaw      []byte
}

type workspaceAuthorityRecoveryInitialState struct {
	directory        *os.File
	policyDirectory  *os.File
	policy           *os.File
	workspace        *os.File
	policyPresent    bool
	workspacePresent bool
}

func (candidate *workspaceAuthorityRecoveryCandidate) close() {
	if candidate == nil {
		return
	}
	if candidate.bootstrap != nil {
		_ = candidate.bootstrap.Close()
		candidate.bootstrap = nil
	}
	if candidate.directory != nil {
		_ = candidate.directory.Close()
		candidate.directory = nil
	}
}

func (state *workspaceAuthorityRecoveryInitialState) close() {
	if state == nil {
		return
	}
	for _, opened := range []*os.File{state.workspace, state.policy, state.policyDirectory, state.directory} {
		if opened != nil {
			_ = opened.Close()
		}
	}
	state.workspace = nil
	state.policy = nil
	state.policyDirectory = nil
	state.directory = nil
}

func (registrar *workspaceAuthorityRegistrar) selectWorkspaceAuthorityRecoveryCandidate(
	workspaces *os.File,
	registry workspaceRegistryJCSV1,
	identity runtimeWorkspaceIdentity,
) (selected *workspaceAuthorityRecoveryCandidate, err error) {
	if registrar == nil || workspaces == nil {
		return nil, errRuntimeNoncanonical
	}
	registered := make(map[string]struct{}, len(registry.Entries))
	for _, entry := range registry.Entries {
		registered[entry.WorkspaceAuthorityID] = struct{}{}
	}
	names, err := workspaces.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	complete := false
	defer func() {
		if !complete && selected != nil {
			selected.close()
			selected = nil
		}
	}()
	for _, name := range names {
		if !runtimeWorkspaceAuthorityIDPattern.MatchString(name) {
			continue
		}
		if _, ok := registered[name]; ok {
			continue
		}
		candidate, matches, classifyErr := registrar.classifyWorkspaceAuthorityRecoveryCandidate(workspaces, name, identity.rootHash)
		if classifyErr != nil {
			return selected, classifyErr
		}
		if !matches {
			continue
		}
		if selected != nil {
			candidate.close()
			return selected, fmt.Errorf("%w: multiple orphan workspace authorities claim the same workspace root", errRuntimeConflict)
		}
		selected = candidate
	}
	complete = true
	return selected, nil
}

func (registrar *workspaceAuthorityRegistrar) classifyWorkspaceAuthorityRecoveryCandidate(
	workspaces *os.File,
	authorityID string,
	rootHash string,
) (_ *workspaceAuthorityRecoveryCandidate, matches bool, err error) {
	directory, err := openRuntimeAuthorityDirectoryAt(workspaces, authorityID)
	if err != nil {
		return nil, false, fmt.Errorf("%w: open orphan workspace authority %q: %v", errRuntimeIntegrityMismatch, authorityID, err)
	}
	candidate := &workspaceAuthorityRecoveryCandidate{authorityID: authorityID, directory: directory}
	keep := false
	defer func() {
		if !keep {
			candidate.close()
		}
	}()
	if err := registrar.ops.validatePrivateNode(directory, registrar.expectedUID); err != nil {
		return nil, false, err
	}
	bootstrap, err := openAuthorityPrivateFileAt(directory, "workspace.bootstrap.json", syscall.O_RDONLY, false, registrar.expectedUID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("%w: open orphan bootstrap for %q: %v", errRuntimeIntegrityMismatch, authorityID, err)
	}
	candidate.bootstrap = bootstrap
	if err := registrar.ops.validatePrivateNode(bootstrap, registrar.expectedUID); err != nil {
		return nil, false, err
	}
	raw, err := readWorkspaceAuthorityRecoveryPrivateFile(bootstrap)
	if err != nil {
		return nil, false, err
	}
	var record runtimeWorkspaceBootstrap
	if err := decodeRuntimeAuthorityJSON(raw, &record); err != nil {
		return nil, false, err
	}
	if err := validateWorkspaceAuthorityRecoveryBootstrap(raw, record, authorityID); err != nil {
		return nil, false, err
	}
	if record.WorkspaceRootIdentityHash != rootHash {
		return nil, false, nil
	}
	keep = true
	return candidate, true, nil
}

func validateWorkspaceAuthorityRecoveryBootstrap(raw []byte, record runtimeWorkspaceBootstrap, directoryName string) error {
	if !jsonNumberEquals(record.BootstrapSchema, 1) {
		return errRuntimeUnsupportedSchema
	}
	if !runtimeWorkspaceAuthorityIDPattern.MatchString(record.WorkspaceAuthorityID) ||
		!runtimeSHA256Pattern.MatchString(record.WorkspaceRootIdentityHash) {
		return errRuntimeNoncanonical
	}
	if record.RootIdentityEncoding != "workspace-root-identity-v1" || record.WorkspaceAuthorityID != directoryName {
		return errRuntimeIntegrityMismatch
	}
	if !bytes.Equal(raw, canonicalRuntimeBootstrap(record)) {
		return errRuntimeNoncanonical
	}
	return nil
}

func (transaction *workspaceAuthorityRecoveryTransaction) complete() error {
	if transaction == nil || transaction.registrar == nil || transaction.candidate == nil {
		return errRuntimeNoncanonical
	}
	if err := transaction.validatePins(nil); err != nil {
		return err
	}
	ownerLock, ownerExists, err := transaction.openOwnerLock()
	if err != nil {
		return err
	}
	if !ownerExists {
		preflight, err := transaction.preflightInitialState(nil, false)
		if err != nil {
			return err
		}
		preflight.close()
		if err := transaction.validatePins(nil); err != nil {
			return err
		}
		ownerLock, err = openAuthorityPrivateFileAt(
			transaction.candidate.directory,
			"owner.lock",
			syscall.O_RDWR,
			true,
			transaction.registrar.expectedUID,
		)
		if err != nil {
			return err
		}
	}
	defer ownerLock.Close()
	if err := transaction.registrar.ops.validatePrivateNode(ownerLock, transaction.registrar.expectedUID); err != nil {
		return err
	}
	if err := syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_UN) //nolint:errcheck // close also releases the scoped lock
	if err := transaction.validatePins(ownerLock); err != nil {
		return err
	}
	if err := transaction.registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationOwnerLockAcquired); err != nil {
		return err
	}
	if err := transaction.validatePins(ownerLock); err != nil {
		return err
	}

	initial, err := transaction.preflightInitialState(ownerLock, true)
	if err != nil {
		return err
	}
	defer initial.close()
	if err := transaction.publishMissingInitialState(initial); err != nil {
		return err
	}

	finalState, err := transaction.preflightInitialState(ownerLock, true)
	if err != nil {
		return err
	}
	defer finalState.close()
	if !finalState.policyPresent || !finalState.workspacePresent {
		return errRuntimeIntegrityMismatch
	}
	if err := transaction.registrar.ops.syncInitialAuthorityDirectory(transaction.candidate.directory); err != nil {
		return err
	}
	if err := transaction.registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationAuthorityDirectorySynced); err != nil {
		return err
	}
	if err := transaction.validatePins(ownerLock); err != nil {
		return err
	}
	if err := finalState.validateNamed(transaction); err != nil {
		return err
	}

	registryPublisher, err := newAuthorityPublisher(transaction.workspaces, transaction.registrar.expectedUID, nil)
	if err != nil {
		return err
	}
	if _, err := registryPublisher.publishMutable(
		"registry.private.json",
		&transaction.currentGeneration,
		transaction.nextRegistryRaw,
		func(raw []byte) (uint64, error) {
			record, err := decodeWorkspaceRegistryJCSV1(raw)
			return record.RecordRev, err
		},
	); err != nil {
		return err
	}
	return transaction.registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationRegistryPublished)
}

func (transaction *workspaceAuthorityRecoveryTransaction) openOwnerLock() (*os.File, bool, error) {
	ownerLock, err := openAuthorityPrivateFileAt(
		transaction.candidate.directory,
		"owner.lock",
		syscall.O_RDWR,
		false,
		transaction.registrar.expectedUID,
	)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("%w: open orphan owner lock: %v", errRuntimeIntegrityMismatch, err)
	}
	return ownerLock, true, nil
}

func (transaction *workspaceAuthorityRecoveryTransaction) preflightInitialState(
	ownerLock *os.File,
	expectOwner bool,
) (state *workspaceAuthorityRecoveryInitialState, err error) {
	directory, err := openRuntimeAuthorityDirectoryAt(transaction.workspaces, transaction.candidate.authorityID)
	if err != nil {
		return nil, fmt.Errorf("%w: reopen selected orphan workspace authority: %v", errRuntimeIntegrityMismatch, err)
	}
	state = &workspaceAuthorityRecoveryInitialState{directory: directory}
	complete := false
	defer func() {
		if !complete {
			state.close()
			state = nil
		}
	}()
	if err := transaction.registrar.ops.validatePrivateNode(directory, transaction.registrar.expectedUID); err != nil {
		return state, err
	}
	if err := workspaceAuthorityRecoveryOpenedMatchesNamed(transaction.candidate.directory, transaction.workspaces, transaction.candidate.authorityID); err != nil {
		return state, err
	}
	names, err := workspaceAuthorityRecoveryDirectoryNames(directory)
	if err != nil {
		return state, err
	}
	present := make(map[string]bool, len(names))
	for _, name := range names {
		switch name {
		case "workspace.bootstrap.json", "owner.lock", "admission-policies", "workspace.private.json":
			present[name] = true
		default:
			return state, fmt.Errorf("%w: unexpected initial workspace authority entry %q", errRuntimeConflict, name)
		}
	}
	if !present["workspace.bootstrap.json"] || present["owner.lock"] != expectOwner {
		return state, errRuntimeIntegrityMismatch
	}
	if expectOwner {
		if ownerLock == nil {
			return state, errRuntimeNoncanonical
		}
		ownerRaw, err := readWorkspaceAuthorityRecoveryPrivateFile(ownerLock)
		if err != nil {
			return state, err
		}
		if len(ownerRaw) != 0 {
			return state, errRuntimeConflict
		}
	}
	if present["admission-policies"] {
		policyDirectory, err := openRuntimeAuthorityDirectoryAt(directory, "admission-policies")
		if err != nil {
			return state, fmt.Errorf("%w: open orphan admission policies: %v", errRuntimeIntegrityMismatch, err)
		}
		state.policyDirectory = policyDirectory
		if err := transaction.registrar.ops.validatePrivateNode(policyDirectory, transaction.registrar.expectedUID); err != nil {
			return state, err
		}
		policyNames, err := workspaceAuthorityRecoveryDirectoryNames(policyDirectory)
		if err != nil {
			return state, err
		}
		for _, name := range policyNames {
			if name != "1.json" {
				return state, fmt.Errorf("%w: unexpected initial admission policy entry %q", errRuntimeConflict, name)
			}
			state.policyPresent = true
		}
		if state.policyPresent {
			policy, err := openAuthorityPrivateFileAt(policyDirectory, "1.json", syscall.O_RDONLY, false, transaction.registrar.expectedUID)
			if err != nil {
				return state, fmt.Errorf("%w: open initial admission policy: %v", errRuntimeIntegrityMismatch, err)
			}
			state.policy = policy
			if err := transaction.registrar.ops.validatePrivateNode(policy, transaction.registrar.expectedUID); err != nil {
				return state, err
			}
			policyRaw, err := readWorkspaceAuthorityRecoveryPrivateFile(policy)
			if err != nil {
				return state, err
			}
			if !bytes.Equal(policyRaw, transaction.policyRaw) {
				return state, errRuntimeConflict
			}
		}
	}
	if present["workspace.private.json"] {
		workspace, err := openAuthorityPrivateFileAt(directory, "workspace.private.json", syscall.O_RDONLY, false, transaction.registrar.expectedUID)
		if err != nil {
			return state, fmt.Errorf("%w: open initial workspace authority: %v", errRuntimeIntegrityMismatch, err)
		}
		state.workspace = workspace
		if err := transaction.registrar.ops.validatePrivateNode(workspace, transaction.registrar.expectedUID); err != nil {
			return state, err
		}
		workspaceRaw, err := readWorkspaceAuthorityRecoveryPrivateFile(workspace)
		if err != nil {
			return state, err
		}
		if !bytes.Equal(workspaceRaw, transaction.workspaceRaw) {
			return state, errRuntimeConflict
		}
		state.workspacePresent = true
	}
	if err := state.validateNamed(transaction); err != nil {
		return state, err
	}
	complete = true
	return state, nil
}

func (transaction *workspaceAuthorityRecoveryTransaction) publishMissingInitialState(state *workspaceAuthorityRecoveryInitialState) error {
	if state == nil {
		return errRuntimeNoncanonical
	}
	if err := transaction.validatePins(nil); err != nil {
		return err
	}
	if err := state.validateNamed(transaction); err != nil {
		return err
	}
	policyDirectory := state.policyDirectory
	createdPolicyDirectory := false
	if !state.policyPresent {
		if policyDirectory == nil {
			var err error
			policyDirectory, err = createWorkspaceAuthorityRegistrationDirectory(
				transaction.candidate.directory,
				"admission-policies",
				transaction.registrar.expectedUID,
				transaction.registrar.ops.validatePrivateNode,
			)
			if err != nil {
				return err
			}
			createdPolicyDirectory = true
			defer policyDirectory.Close()
		}
		policyPublisher, err := newAuthorityPublisher(policyDirectory, transaction.registrar.expectedUID, nil)
		if err != nil {
			return err
		}
		if _, err := policyPublisher.publishImmutable("1.json", transaction.policyRaw); err != nil {
			return err
		}
		if err := transaction.registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationPolicyPublished); err != nil {
			return err
		}
	}
	if !state.workspacePresent {
		if err := transaction.validatePins(nil); err != nil {
			return err
		}
		if err := state.validateNamed(transaction); err != nil {
			return err
		}
		if createdPolicyDirectory {
			if err := workspaceAuthorityRecoveryOpenedMatchesNamed(policyDirectory, transaction.candidate.directory, "admission-policies"); err != nil {
				return err
			}
		}
		authorityPublisher, err := newAuthorityPublisher(transaction.candidate.directory, transaction.registrar.expectedUID, nil)
		if err != nil {
			return err
		}
		if _, err := authorityPublisher.publishMutable(
			"workspace.private.json",
			nil,
			transaction.workspaceRaw,
			func(raw []byte) (uint64, error) {
				record, err := decodeWorkspaceAuthorityJCSV1(raw)
				return record.RecordRev, err
			},
		); err != nil {
			return err
		}
		if err := transaction.registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationWorkspacePublished); err != nil {
			return err
		}
	}
	return nil
}

func (transaction *workspaceAuthorityRecoveryTransaction) validatePins(ownerLock *os.File) error {
	if err := validateWorkspaceAuthorityRegistrationPins(
		transaction.registrar.hostRoot,
		transaction.root,
		transaction.workspaces,
		transaction.registryLock,
		transaction.registry,
		transaction.identity,
		transaction.workspace,
	); err != nil {
		return err
	}
	if err := transaction.candidate.validateNamed(transaction.workspaces, transaction.registrar); err != nil {
		return err
	}
	if ownerLock != nil {
		if err := transaction.registrar.ops.validatePrivateNode(ownerLock, transaction.registrar.expectedUID); err != nil {
			return err
		}
		if err := workspaceAuthorityRecoveryOpenedMatchesNamed(ownerLock, transaction.candidate.directory, "owner.lock"); err != nil {
			return err
		}
	}
	return nil
}

func (candidate *workspaceAuthorityRecoveryCandidate) validateNamed(workspaces *os.File, registrar *workspaceAuthorityRegistrar) error {
	if candidate == nil || workspaces == nil || registrar == nil || candidate.directory == nil || candidate.bootstrap == nil {
		return errRuntimeNoncanonical
	}
	if err := registrar.ops.validatePrivateNode(candidate.directory, registrar.expectedUID); err != nil {
		return err
	}
	if err := registrar.ops.validatePrivateNode(candidate.bootstrap, registrar.expectedUID); err != nil {
		return err
	}
	if err := workspaceAuthorityRecoveryOpenedMatchesNamed(candidate.directory, workspaces, candidate.authorityID); err != nil {
		return err
	}
	return workspaceAuthorityRecoveryOpenedMatchesNamed(candidate.bootstrap, candidate.directory, "workspace.bootstrap.json")
}

func (state *workspaceAuthorityRecoveryInitialState) validateNamed(transaction *workspaceAuthorityRecoveryTransaction) error {
	if state == nil || state.directory == nil || transaction == nil {
		return errRuntimeNoncanonical
	}
	if err := transaction.registrar.ops.validatePrivateNode(state.directory, transaction.registrar.expectedUID); err != nil {
		return err
	}
	if err := workspaceAuthorityRecoveryOpenedMatchesNamed(state.directory, transaction.workspaces, transaction.candidate.authorityID); err != nil {
		return err
	}
	checks := []struct {
		opened *os.File
		parent *os.File
		name   string
	}{
		{opened: state.policyDirectory, parent: state.directory, name: "admission-policies"},
		{opened: state.policy, parent: state.policyDirectory, name: "1.json"},
		{opened: state.workspace, parent: state.directory, name: "workspace.private.json"},
	}
	for _, check := range checks {
		if check.opened == nil {
			continue
		}
		if err := transaction.registrar.ops.validatePrivateNode(check.opened, transaction.registrar.expectedUID); err != nil {
			return err
		}
		if err := workspaceAuthorityRecoveryOpenedMatchesNamed(check.opened, check.parent, check.name); err != nil {
			return err
		}
	}
	return nil
}

func workspaceAuthorityRecoveryOpenedMatchesNamed(opened, parent *os.File, name string) error {
	openedDevice, openedInode, err := workspaceAuthorityRegistrationFileIdentity(opened)
	if err != nil {
		return err
	}
	namedDevice, namedInode, err := workspaceAuthorityRegistrationNamedIdentity(parent, name, false)
	if err != nil {
		return fmt.Errorf("%w: inspect named recovery node %q: %v", errRuntimeIntegrityMismatch, name, err)
	}
	if openedDevice != namedDevice || openedInode != namedInode {
		return fmt.Errorf("%w: replaced recovery node %q", errRuntimeIntegrityMismatch, name)
	}
	return nil
}

func workspaceAuthorityRecoveryDirectoryNames(directory *os.File) ([]string, error) {
	if directory == nil {
		return nil, errRuntimeNoncanonical
	}
	names, err := directory.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func readWorkspaceAuthorityRecoveryPrivateFile(file *os.File) ([]byte, error) {
	if file == nil {
		return nil, errRuntimeNoncanonical
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > runtimeAuthorityMaxRecordBytes {
		return nil, errRuntimeOutOfRange
	}
	raw, err := io.ReadAll(io.NewSectionReader(file, 0, runtimeAuthorityMaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > runtimeAuthorityMaxRecordBytes {
		return nil, errRuntimeOutOfRange
	}
	return raw, nil
}
