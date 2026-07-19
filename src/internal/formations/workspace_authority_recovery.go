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
	defer func() {
		if err != nil && selected != nil {
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
	raw, err := io.ReadAll(io.LimitReader(file, runtimeAuthorityMaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > runtimeAuthorityMaxRecordBytes {
		return nil, errRuntimeOutOfRange
	}
	return raw, nil
}
