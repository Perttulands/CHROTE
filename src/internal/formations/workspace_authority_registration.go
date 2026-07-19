package formations

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// workspaceAuthorityRegistrationScope exposes only the values observed while
// the registry lock and all pinned descriptors are held.
type workspaceAuthorityRegistrationScope interface {
	matchedWorkspaceAuthorityID() (string, bool)
	registryLockIdentity() (uint64, uint64)
	workspaceIdentity() runtimeWorkspaceIdentity
}

type workspaceAuthorityRegistrationObservation struct {
	identity             runtimeWorkspaceIdentity
	lockDevice           uint64
	lockInode            uint64
	workspaceAuthorityID string
	matched              bool
}

func (scope workspaceAuthorityRegistrationObservation) matchedWorkspaceAuthorityID() (string, bool) {
	return scope.workspaceAuthorityID, scope.matched
}

func (scope workspaceAuthorityRegistrationObservation) registryLockIdentity() (uint64, uint64) {
	return scope.lockDevice, scope.lockInode
}

func (scope workspaceAuthorityRegistrationObservation) workspaceIdentity() runtimeWorkspaceIdentity {
	return scope.identity
}

type workspaceAuthorityRegistrationOps struct {
	openWorkspace                      func(string) (*os.File, error)
	validatePrivateNode                func(*os.File, uint32) error
	generateWorkspaceAuthorityID       func() (string, error)
	observeInitialRegistration         func(string) error
	syncInitialAuthorityDirectory      func(*os.File) error
	syncWorkspaceRegistrationDirectory func(*os.File) error
}

type workspaceAuthorityRegistrar struct {
	hostRoot    string
	expectedUID uint32
	gate        workspaceAuthorityCapabilityGate
	ops         workspaceAuthorityRegistrationOps
	local       sync.Mutex
}

const (
	workspaceAuthorityInitialRegistrationOwnerLockAcquired        = "owner_lock_acquired"
	workspaceAuthorityInitialRegistrationPolicyPublished          = "admission_policy_published"
	workspaceAuthorityInitialRegistrationWorkspacePublished       = "workspace_authority_published"
	workspaceAuthorityInitialRegistrationAuthorityDirectorySynced = "authority_directory_synced"
	workspaceAuthorityInitialRegistrationRegistryPublished        = "registry_published_and_parent_synced"
)

func newWorkspaceAuthorityRegistrar(hostRoot string, expectedUID uint32, gate workspaceAuthorityCapabilityGate) *workspaceAuthorityRegistrar {
	return &workspaceAuthorityRegistrar{
		hostRoot:    hostRoot,
		expectedUID: expectedUID,
		gate:        gate,
		ops: workspaceAuthorityRegistrationOps{
			openWorkspace:       openWorkspaceAuthorityRegistrationDirectory,
			validatePrivateNode: validateWorkspaceAuthorityRegistrationPrivateNode,
			generateWorkspaceAuthorityID: func() (string, error) {
				return newPrefixedID("wsa"), nil
			},
			observeInitialRegistration: func(string) error { return nil },
			syncInitialAuthorityDirectory: func(directory *os.File) error {
				if directory == nil {
					return errRuntimeNoncanonical
				}
				return directory.Sync()
			},
			syncWorkspaceRegistrationDirectory: func(directory *os.File) error {
				if directory == nil {
					return errRuntimeNoncanonical
				}
				return directory.Sync()
			},
		},
	}
}

func (registrar *workspaceAuthorityRegistrar) inspect(configuredWorkspace string, callback func(workspaceAuthorityRegistrationScope) error) error {
	if registrar == nil || callback == nil || registrar.ops.openWorkspace == nil || registrar.ops.validatePrivateNode == nil {
		return errRuntimeNoncanonical
	}
	return registrar.gate.beforeMutation(func() error {
		registrar.local.Lock()
		defer registrar.local.Unlock()
		return registrar.inspectWithCapability(configuredWorkspace, callback)
	})
}

func (registrar *workspaceAuthorityRegistrar) register(configuredWorkspace string, callback func(workspaceAuthorityRegistrationScope) error) error {
	if registrar == nil || callback == nil ||
		registrar.ops.openWorkspace == nil ||
		registrar.ops.validatePrivateNode == nil ||
		registrar.ops.generateWorkspaceAuthorityID == nil ||
		registrar.ops.observeInitialRegistration == nil ||
		registrar.ops.syncInitialAuthorityDirectory == nil ||
		registrar.ops.syncWorkspaceRegistrationDirectory == nil {
		return errRuntimeNoncanonical
	}

	return registrar.gate.beforeMutation(func() error {
		registrar.local.Lock()
		defer registrar.local.Unlock()

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
		defer syscall.Flock(int(registryLock.Fd()), syscall.LOCK_UN) //nolint:errcheck // close also releases the scoped lock

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
		currentRegistryRaw, err := encodeWorkspaceRegistryJCSV1(strictRegistry)
		if err != nil {
			return err
		}

		return withOpenedWorkspaceAuthorityIdentity(configuredWorkspace, registrar.ops.openWorkspace, func(workspace *os.File, identity runtimeWorkspaceIdentity) error {
			if err := validateWorkspaceAuthorityRegistrationPins(registrar.hostRoot, root, workspaces, registryLock, registry, identity, workspace); err != nil {
				return err
			}
			if err := validateRuntimeAuthorityWorkspaceIsolation(registrar.hostRoot, root, identity); err != nil {
				return err
			}

			lockDevice, lockInode, err := workspaceAuthorityRegistrationFileIdentity(registryLock)
			if err != nil {
				return err
			}
			entry, matchErr := matchRuntimeWorkspaceRegistryEntry(projectedRegistry, identity)
			if matchErr == nil {
				if err := registrar.ops.syncWorkspaceRegistrationDirectory(workspaces); err != nil {
					return err
				}
				return callback(workspaceAuthorityRegistrationObservation{
					identity:             identity,
					lockDevice:           lockDevice,
					lockInode:            lockInode,
					workspaceAuthorityID: entry.WorkspaceAuthorityID,
					matched:              true,
				})
			}
			if !errors.Is(matchErr, os.ErrNotExist) {
				return matchErr
			}
			if strictRegistry.RecordRev == runtimeAuthorityMaxJSONInteger {
				return errRuntimeOutOfRange
			}

			authorityID, err := registrar.ops.generateWorkspaceAuthorityID()
			if err != nil {
				return err
			}
			if !runtimeWorkspaceAuthorityIDPattern.MatchString(authorityID) {
				return errRuntimeNoncanonical
			}
			for _, existing := range strictRegistry.Entries {
				if existing.WorkspaceAuthorityID == authorityID {
					return fmt.Errorf("%w: workspace authority id is already registered", errRuntimeConflict)
				}
			}

			newEntry := workspaceRegistryEntryJCSV1{
				ConfiguredPath:              identity.configuredPath,
				Device:                      strconv.FormatUint(identity.device, 10),
				Inode:                       strconv.FormatUint(identity.inode, 10),
				WorkspaceAuthorityID:        authorityID,
				WorkspaceRootIdentitySHA256: identity.rootHash,
			}
			projectedEntries := *projectedRegistry.Entries
			projectedNewEntry := runtimeWorkspaceRegistryEntry{
				WorkspaceAuthorityID:      authorityID,
				ConfiguredPath:            identity.configuredPath,
				Device:                    newEntry.Device,
				Inode:                     newEntry.Inode,
				WorkspaceRootIdentityHash: identity.rootHash,
				decodedDevice:             identity.device,
				decodedInode:              identity.inode,
			}
			insertAt := sort.Search(len(projectedEntries), func(index int) bool {
				return !runtimeRegistryEntryLess(projectedEntries[index], projectedNewEntry)
			})
			nextEntries := make([]workspaceRegistryEntryJCSV1, len(strictRegistry.Entries)+1)
			copy(nextEntries, strictRegistry.Entries[:insertAt])
			nextEntries[insertAt] = newEntry
			copy(nextEntries[insertAt+1:], strictRegistry.Entries[insertAt:])
			currentGeneration := authorityGeneration{
				recordRev: strictRegistry.RecordRev,
				sha256:    runtimeSHA256Hex(currentRegistryRaw),
			}
			nextRegistryRaw, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryJCSV1{
				Entries:         nextEntries,
				PriorGeneration: &currentGeneration,
				RecordRev:       strictRegistry.RecordRev + 1,
				RegistrySchema:  1,
			})
			if err != nil {
				return err
			}
			if int64(len(nextRegistryRaw)) > runtimeAuthorityMaxRecordBytes {
				return errRuntimeOutOfRange
			}

			policyRaw := []byte(`{"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}`)
			workspaceRaw, err := encodeWorkspaceAuthorityJCSV1(workspaceAuthorityJCSV1{
				AdmissionPolicyRef: workspaceAdmissionPolicyRefJCSV1{
					PolicyRev:    1,
					PolicySHA256: runtimeSHA256Hex(policyRaw),
				},
				AuthoritySchema:             runtimeAuthoritySchema,
				NextAdmissionSeq:            1,
				NextWriterFence:             1,
				RecordRev:                   1,
				RootIdentityEncoding:        "workspace-root-identity-v1",
				WorkspaceAuthorityID:        authorityID,
				WorkspaceRootIdentitySHA256: identity.rootHash,
			})
			if err != nil {
				return err
			}
			var bootstrap bytes.Buffer
			bootstrap.WriteString(`{"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":`)
			writeRuntimeCanonicalJSONString(&bootstrap, authorityID)
			bootstrap.WriteString(`,"workspaceRootIdentitySha256":`)
			writeRuntimeCanonicalJSONString(&bootstrap, identity.rootHash)
			bootstrap.WriteByte('}')

			authorityDirectory, err := createWorkspaceAuthorityRegistrationDirectory(workspaces, authorityID, registrar.expectedUID, registrar.ops.validatePrivateNode)
			if err != nil {
				return err
			}
			defer authorityDirectory.Close()
			authorityPublisher, err := newAuthorityPublisher(authorityDirectory, registrar.expectedUID, nil)
			if err != nil {
				return err
			}
			if _, err := authorityPublisher.publishImmutable("workspace.bootstrap.json", bootstrap.Bytes()); err != nil {
				return err
			}

			if err := func() error {
				ownerLock, err := openAuthorityPrivateFileAt(authorityDirectory, "owner.lock", syscall.O_RDWR, true, registrar.expectedUID)
				if err != nil {
					return err
				}
				defer ownerLock.Close()
				if err := registrar.ops.validatePrivateNode(ownerLock, registrar.expectedUID); err != nil {
					return err
				}
				if err := syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_EX); err != nil {
					return err
				}
				defer syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_UN) //nolint:errcheck // close also releases the scoped lock
				if err := registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationOwnerLockAcquired); err != nil {
					return err
				}

				policyDirectory, err := createWorkspaceAuthorityRegistrationDirectory(authorityDirectory, "admission-policies", registrar.expectedUID, registrar.ops.validatePrivateNode)
				if err != nil {
					return err
				}
				defer policyDirectory.Close()
				policyPublisher, err := newAuthorityPublisher(policyDirectory, registrar.expectedUID, nil)
				if err != nil {
					return err
				}
				if _, err := policyPublisher.publishImmutable("1.json", policyRaw); err != nil {
					return err
				}
				if err := registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationPolicyPublished); err != nil {
					return err
				}

				if _, err := authorityPublisher.publishMutable("workspace.private.json", nil, workspaceRaw, func(raw []byte) (uint64, error) {
					record, err := decodeWorkspaceAuthorityJCSV1(raw)
					return record.RecordRev, err
				}); err != nil {
					return err
				}
				if err := registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationWorkspacePublished); err != nil {
					return err
				}
				if err := registrar.ops.syncInitialAuthorityDirectory(authorityDirectory); err != nil {
					return err
				}
				return registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationAuthorityDirectorySynced)
			}(); err != nil {
				return err
			}

			registryPublisher, err := newAuthorityPublisher(workspaces, registrar.expectedUID, nil)
			if err != nil {
				return err
			}
			if _, err := registryPublisher.publishMutable("registry.private.json", &currentGeneration, nextRegistryRaw, func(raw []byte) (uint64, error) {
				record, err := decodeWorkspaceRegistryJCSV1(raw)
				return record.RecordRev, err
			}); err != nil {
				return err
			}
			if err := registrar.ops.observeInitialRegistration(workspaceAuthorityInitialRegistrationRegistryPublished); err != nil {
				return err
			}

			return callback(workspaceAuthorityRegistrationObservation{
				identity:             identity,
				lockDevice:           lockDevice,
				lockInode:            lockInode,
				workspaceAuthorityID: authorityID,
				matched:              true,
			})
		})
	})
}

func (registrar *workspaceAuthorityRegistrar) inspectWithCapability(configuredWorkspace string, callback func(workspaceAuthorityRegistrationScope) error) error {
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
	defer syscall.Flock(int(registryLock.Fd()), syscall.LOCK_UN) //nolint:errcheck // close also releases the scoped lock

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
		if err := validateWorkspaceAuthorityRegistrationPins(registrar.hostRoot, root, workspaces, registryLock, registry, identity, workspace); err != nil {
			return err
		}
		if err := validateRuntimeAuthorityWorkspaceIsolation(registrar.hostRoot, root, identity); err != nil {
			return err
		}

		entry, err := matchRuntimeWorkspaceRegistryEntry(projectedRegistry, identity)
		matched := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		lockDevice, lockInode, err := workspaceAuthorityRegistrationFileIdentity(registryLock)
		if err != nil {
			return err
		}
		scope := workspaceAuthorityRegistrationObservation{
			identity:             identity,
			lockDevice:           lockDevice,
			lockInode:            lockInode,
			workspaceAuthorityID: entry.WorkspaceAuthorityID,
			matched:              matched,
		}
		return callback(scope)
	})
}

func withOpenedWorkspaceAuthorityIdentity(configuredWorkspace string, openWorkspace func(string) (*os.File, error), callback func(*os.File, runtimeWorkspaceIdentity) error) error {
	if openWorkspace == nil || callback == nil {
		return errRuntimeNoncanonical
	}
	configuredPath := filepath.ToSlash(filepath.Clean(configuredWorkspace))
	if err := validateRuntimeConfiguredPath(configuredPath); err != nil {
		return err
	}
	workspace, err := openWorkspace(configuredPath)
	if err != nil {
		return err
	}
	if workspace == nil {
		return errRuntimeNoncanonical
	}
	defer workspace.Close()

	identity, err := workspaceAuthorityRegistrationIdentityFromOpened(configuredPath, workspace)
	if err != nil {
		return err
	}
	return callback(workspace, identity)
}

func workspaceAuthorityRegistrationIdentityFromOpened(configuredPath string, workspace *os.File) (runtimeWorkspaceIdentity, error) {
	info, err := workspace.Stat()
	if err != nil {
		return runtimeWorkspaceIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok {
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

func openWorkspaceAuthorityRegistrationDirectory(configuredPath string) (*os.File, error) {
	fd, err := syscall.Open(configuredPath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	workspace := os.NewFile(uintptr(fd), configuredPath)
	if workspace == nil {
		_ = syscall.Close(fd)
		return nil, errRuntimeNoncanonical
	}
	return workspace, nil
}

func createWorkspaceAuthorityRegistrationDirectory(parent *os.File, name string, expectedUID uint32, validate func(*os.File, uint32) error) (*os.File, error) {
	if parent == nil || !runtimeAuthorityPathComponent(name) || validate == nil {
		return nil, errRuntimeNoncanonical
	}
	if err := syscall.Mkdirat(int(parent.Fd()), name, uint32(authorityPrivateDirectoryMode.Perm())); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			return nil, fmt.Errorf("%w: workspace authority directory already exists", errRuntimeConflict)
		}
		return nil, &os.PathError{Op: "mkdirat", Path: name, Err: err}
	}
	directory, err := openRuntimeAuthorityDirectoryAt(parent, name)
	if err != nil {
		return nil, err
	}
	valid := false
	defer func() {
		if !valid {
			_ = directory.Close()
		}
	}()
	if err := syscall.Fchmod(int(directory.Fd()), uint32(authorityPrivateDirectoryMode.Perm())); err != nil {
		return nil, &os.PathError{Op: "fchmod", Path: name, Err: err}
	}
	if err := validate(directory, expectedUID); err != nil {
		return nil, err
	}
	valid = true
	return directory, nil
}

func validateWorkspaceAuthorityRegistrationPrivateNode(opened *os.File, expectedUID uint32) error {
	if opened == nil {
		return errRuntimeNoncanonical
	}
	info, err := opened.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != expectedUID {
		return errRuntimeIntegrityMismatch
	}
	if info.IsDir() {
		if !authorityPrivateModeIsExact(info.Mode(), authorityPrivateDirectoryMode) {
			return errRuntimeIntegrityMismatch
		}
		return nil
	}
	return validateAuthorityPrivateFile(opened, expectedUID)
}

func readWorkspaceAuthorityRegistrationRegistry(registry *os.File) (workspaceRegistryJCSV1, error) {
	info, err := registry.Stat()
	if err != nil {
		return workspaceRegistryJCSV1{}, err
	}
	if info.Size() < 0 || info.Size() > runtimeAuthorityMaxRecordBytes {
		return workspaceRegistryJCSV1{}, errRuntimeOutOfRange
	}
	raw, err := io.ReadAll(io.LimitReader(registry, runtimeAuthorityMaxRecordBytes+1))
	if err != nil {
		return workspaceRegistryJCSV1{}, err
	}
	if int64(len(raw)) > runtimeAuthorityMaxRecordBytes {
		return workspaceRegistryJCSV1{}, errRuntimeOutOfRange
	}
	return decodeWorkspaceRegistryJCSV1(raw)
}

func validateWorkspaceAuthorityRegistrationPins(hostRoot string, root, workspaces, registryLock, registry *os.File, identity runtimeWorkspaceIdentity, workspace *os.File) error {
	checks := []struct {
		name   string
		opened *os.File
		parent *os.File
		path   string
		follow bool
	}{
		{name: "host root", opened: root, path: hostRoot},
		{name: "workspaces root", opened: workspaces, parent: root, path: "workspaces"},
		{name: "registry lock", opened: registryLock, parent: workspaces, path: "registry.lock"},
		{name: "private registry", opened: registry, parent: workspaces, path: "registry.private.json"},
		{name: "configured workspace", opened: workspace, path: identity.configuredPath, follow: true},
	}
	for _, check := range checks {
		openedDevice, openedInode, err := workspaceAuthorityRegistrationFileIdentity(check.opened)
		if err != nil {
			return err
		}
		namedDevice, namedInode, err := workspaceAuthorityRegistrationNamedIdentity(check.parent, check.path, check.follow)
		if err != nil {
			return fmt.Errorf("%w: inspect named %s: %v", errRuntimeIntegrityMismatch, check.name, err)
		}
		if openedDevice != namedDevice || openedInode != namedInode {
			return fmt.Errorf("%w: replaced %s", errRuntimeIntegrityMismatch, check.name)
		}
	}
	return nil
}

func workspaceAuthorityRegistrationNamedIdentity(parent *os.File, name string, follow bool) (uint64, uint64, error) {
	if parent == nil {
		info, err := os.Lstat(name)
		if follow {
			info, err = os.Stat(name)
		}
		if err != nil {
			return 0, 0, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return 0, 0, errRuntimeNoncanonical
		}
		return uint64(stat.Dev), stat.Ino, nil
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return 0, 0, err
	}
	return uint64(stat.Dev), stat.Ino, nil
}

func workspaceAuthorityRegistrationFileIdentity(file *os.File) (uint64, uint64, error) {
	if file == nil {
		return 0, 0, errRuntimeNoncanonical
	}
	info, err := file.Stat()
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, errRuntimeNoncanonical
	}
	return uint64(stat.Dev), stat.Ino, nil
}

func workspaceAuthorityRegistrationOpenError(component string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return err
	}
	return fmt.Errorf("%w: open %s: %v", errRuntimeIntegrityMismatch, component, err)
}
