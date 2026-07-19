package formations

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	openWorkspace                 func(string) (*os.File, error)
	validatePrivateNode           func(*os.File, uint32) error
	generateWorkspaceAuthorityID  func() (string, error)
	observeInitialRegistration    func(string) error
	syncInitialAuthorityDirectory func(*os.File) error
}

type workspaceAuthorityRegistrar struct {
	hostRoot    string
	expectedUID uint32
	gate        workspaceAuthorityCapabilityGate
	ops         workspaceAuthorityRegistrationOps
	local       sync.Mutex
}

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
