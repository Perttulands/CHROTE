package formations

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const definitionDirectoryMode = uint32(0o2770)

type definitionKind struct {
	directory string
	suffix    string
}

var (
	boardDefinitionKind  = definitionKind{directory: "boards", suffix: ".formation.toml"}
	layoutDefinitionKind = definitionKind{directory: "layout", suffix: ".layout.toml"}
	notesDefinitionKind  = definitionKind{directory: "notes", suffix: ".notes.toml"}
)

type definitionFile struct {
	directory *os.File
	name      string
	path      string
}

func (f *definitionFile) close() {
	if f != nil && f.directory != nil {
		_ = f.directory.Close()
	}
}

func (s *Store) openBoardDefinition(slug string, createDirectory bool) (*definitionFile, error) {
	return s.openDefinition(boardDefinitionKind, slug, createDirectory)
}

func (s *Store) openLayoutDefinition(slug string, createDirectory bool) (*definitionFile, error) {
	return s.openDefinition(layoutDefinitionKind, slug, createDirectory)
}

func (s *Store) openNotesDefinition(slug string, createDirectory bool) (*definitionFile, error) {
	return s.openDefinition(notesDefinitionKind, slug, createDirectory)
}

func (s *Store) withBoardDefinitionLock(slug string, fn func(*definitionFile) error) error {
	return s.withDefinitionLock(boardDefinitionKind, slug, fn)
}

func (s *Store) withLayoutDefinitionLock(slug string, fn func(*definitionFile) error) error {
	return s.withDefinitionLock(layoutDefinitionKind, slug, fn)
}

func (s *Store) withNotesDefinitionLock(slug string, fn func(*definitionFile) error) error {
	return s.withDefinitionLock(notesDefinitionKind, slug, fn)
}

func (s *Store) withDefinitionLock(kind definitionKind, slug string, fn func(*definitionFile) error) error {
	definition, err := s.openDefinition(kind, slug, true)
	if err != nil {
		return err
	}
	defer definition.close()
	return definition.withLock(fn)
}

func (s *Store) openDefinition(kind definitionKind, slug string, createDirectory bool) (*definitionFile, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	directory, err := s.openDefinitionDirectory(kind, createDirectory)
	if err != nil {
		return nil, definitionPathError(err)
	}
	name := slug + kind.suffix
	return &definitionFile{
		directory: directory,
		name:      name,
		path:      filepath.Join(s.workspaceRoot(), ".formations", kind.directory, name),
	}, nil
}

func (s *Store) openDefinitionDirectory(kind definitionKind, create bool) (*os.File, error) {
	return s.openDefinitionDirectoryWithLeafParentSync(kind, create, nil)
}

func (s *Store) openDefinitionDirectoryWithLeafParentSync(
	kind definitionKind,
	create bool,
	beforeParentSync func() error,
) (*os.File, error) {
	workspace, err := filepath.Abs(s.workspaceRoot())
	if err != nil {
		return nil, err
	}
	workspace = filepath.Clean(workspace)
	flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NONBLOCK | syscall.O_DIRECTORY
	fd, err := syscall.Open(workspace, flags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: workspace, Err: err}
	}
	current := os.NewFile(uintptr(fd), workspace)
	if current == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open formations workspace")
	}
	components := []string{".formations", kind.directory}
	for index, component := range components {
		next, openErr := openDefinitionDirectoryAt(current, component, create)
		if openErr == nil && index == len(components)-1 && beforeParentSync != nil {
			openErr = beforeParentSync()
			if openErr == nil {
				openErr = current.Sync()
			}
		}
		_ = current.Close()
		if openErr != nil {
			if next != nil {
				_ = next.Close()
			}
			return nil, openErr
		}
		current = next
	}
	return current, nil
}

func openDefinitionDirectoryAt(parent *os.File, name string, create bool) (*os.File, error) {
	directory, err := openRuntimeAuthorityDirectoryAt(parent, name)
	if err != nil && (!create || !errors.Is(err, os.ErrNotExist)) {
		return nil, err
	}
	if errors.Is(err, os.ErrNotExist) {
		if err := syscall.Mkdirat(int(parent.Fd()), name, definitionDirectoryMode); err != nil && !errors.Is(err, syscall.EEXIST) {
			return nil, &os.PathError{Op: "mkdirat", Path: name, Err: err}
		}
		directory, err = openRuntimeAuthorityDirectoryAt(parent, name)
		if err != nil {
			return nil, err
		}
	}
	if create {
		if err := ensureDefinitionDirectoryMode(directory, name); err != nil {
			_ = directory.Close()
			return nil, err
		}
	}
	return directory, nil
}

func ensureDefinitionDirectoryMode(directory *os.File, name string) error {
	info, err := directory.Stat()
	if err != nil {
		return err
	}
	if hasSharedDirMode(info.Mode()) {
		return nil
	}
	if err := syscall.Fchmod(int(directory.Fd()), definitionDirectoryMode); err != nil {
		return &os.PathError{Op: "fchmod", Path: name, Err: err}
	}
	return nil
}

func (s *Store) listDefinitionNames(kind definitionKind) ([]string, error) {
	directory, err := s.openDefinitionDirectory(kind, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, definitionPathError(err)
	}
	defer directory.Close()
	names, err := directory.Readdirnames(-1)
	if err != nil {
		return nil, definitionPathError(err)
	}
	definitions := make([]string, 0, len(names))
	for _, name := range names {
		if !strings.HasSuffix(name, kind.suffix) {
			continue
		}
		definitions = append(definitions, name)
	}
	return definitions, nil
}

func (f *definitionFile) read() ([]byte, os.FileInfo, error) {
	file, err := openDefinitionRegularFileAt(f.directory, f.name, syscall.O_RDONLY, false)
	if err != nil {
		return nil, nil, definitionPathError(err)
	}
	defer file.Close()
	if err := ensureDefinitionDirectoryMode(f.directory, filepath.Dir(f.path)); err != nil {
		return nil, nil, definitionPathError(err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, definitionPathError(err)
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, definitionPathError(err)
	}
	return raw, info, nil
}

func (f *definitionFile) readBytes() ([]byte, error) {
	raw, _, err := f.read()
	return raw, err
}

func (f *definitionFile) exists() (bool, error) {
	file, err := openDefinitionRegularFileAt(f.directory, f.name, syscall.O_RDONLY, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, definitionPathError(err)
	}
	return true, file.Close()
}

func (f *definitionFile) withLock(fn func(*definitionFile) error) error {
	lockName := f.name + ".lock"
	mutex := mutexFor(f.path + ".lock")
	mutex.Lock()
	defer mutex.Unlock()
	if err := ensureDefinitionDirectoryMode(f.directory, filepath.Dir(f.path)); err != nil {
		return definitionPathError(err)
	}

	lockFile, err := openDefinitionLockAt(f.directory, lockName)
	if err != nil {
		return definitionPathError(err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return definitionPathError(err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) //nolint:errcheck // best-effort unlock on return
	return fn(f)
}

func openDefinitionLockAt(directory *os.File, name string) (*os.File, error) {
	lockFile, err := openDefinitionRegularFileAt(directory, name, syscall.O_RDWR|syscall.O_EXCL, true)
	if err == nil {
		return lockFile, nil
	}
	if !errors.Is(err, syscall.EEXIST) {
		return nil, err
	}
	lockFile, err = openDefinitionRegularFileAt(directory, name, syscall.O_RDWR, false)
	if err != nil {
		return nil, err
	}
	info, err := lockFile.Stat()
	if err != nil {
		_ = lockFile.Close()
		return nil, err
	}
	if info.Mode().Perm() != sharedFileMode {
		if err := syscall.Fchmod(int(lockFile.Fd()), uint32(sharedFileMode.Perm())); err != nil {
			_ = lockFile.Close()
			return nil, &os.PathError{Op: "fchmod", Path: name, Err: err}
		}
	}
	return lockFile, nil
}

func openDefinitionRegularFileAt(directory *os.File, name string, flags int, createExclusive bool) (*os.File, error) {
	if directory == nil || !runtimeAuthorityPathComponent(name) {
		return nil, &os.PathError{Op: "openat", Path: name, Err: syscall.EINVAL}
	}
	flags |= syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if createExclusive {
		flags |= syscall.O_CREAT | syscall.O_EXCL
	}
	fd, err := syscall.Openat(int(directory.Fd()), name, flags, uint32(sharedFileMode.Perm()))
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open formations definition")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Nlink != 1 {
		_ = file.Close()
		return nil, errors.New("formations definition must be a regular single-link file")
	}
	if createExclusive {
		if err := syscall.Fchmod(fd, uint32(sharedFileMode.Perm())); err != nil {
			_ = file.Close()
			return nil, &os.PathError{Op: "fchmod", Path: name, Err: err}
		}
	}
	return file, nil
}

func (f *definitionFile) writeAtomic(raw []byte) error {
	validateTarget := func() error {
		file, err := openDefinitionRegularFileAt(f.directory, f.name, syscall.O_RDONLY, false)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return file.Close()
	}
	if err := validateTarget(); err != nil {
		return definitionPathError(err)
	}

	temporaryName := "." + f.name + "." + newPrefixedID("tmp")
	temporary, err := openDefinitionRegularFileAt(f.directory, temporaryName, syscall.O_WRONLY, true)
	if err != nil {
		return definitionPathError(err)
	}
	temporaryExists := true
	defer func() {
		_ = temporary.Close()
		if temporaryExists {
			_ = syscall.Unlinkat(int(f.directory.Fd()), temporaryName)
		}
	}()
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := validateTarget(); err != nil {
		return definitionPathError(err)
	}
	if err := syscall.Renameat(int(f.directory.Fd()), temporaryName, int(f.directory.Fd()), f.name); err != nil {
		return definitionPathError(&os.PathError{Op: "renameat", Path: f.name, Err: err})
	}
	temporaryExists = false
	if err := f.directory.Sync(); err != nil {
		return definitionPathError(err)
	}
	return nil
}

func (f *definitionFile) archive(marker string) (string, error) {
	return f.archiveWithSync(marker, nil)
}

func (f *definitionFile) archiveWithSync(marker string, syncDirectory func() error) (string, error) {
	file, err := openDefinitionRegularFileAt(f.directory, f.name, syscall.O_RDONLY, false)
	if err != nil {
		return "", definitionPathError(err)
	}
	if err := file.Close(); err != nil {
		return "", definitionPathError(err)
	}
	archiveName := f.name + ".deleted-" + marker
	archive, err := openDefinitionRegularFileAt(f.directory, archiveName, syscall.O_RDONLY, false)
	if err == nil {
		_ = archive.Close()
		return "", ErrAlreadyExists
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", definitionPathError(err)
	}
	if err := syscall.Renameat(int(f.directory.Fd()), f.name, int(f.directory.Fd()), archiveName); err != nil {
		return "", definitionPathError(&os.PathError{Op: "renameat", Path: f.name, Err: err})
	}
	if syncDirectory == nil {
		syncDirectory = f.directory.Sync
	}
	if err := syncDirectory(); err != nil {
		return archiveName, fmt.Errorf("%w: archive %q directory sync failed: %v", ErrDefinitionPublicationUncertain, f.name, definitionPathError(err))
	}
	return archiveName, nil
}

func (f *definitionFile) restoreArchived(archiveName string) error {
	archive, err := openDefinitionRegularFileAt(f.directory, archiveName, syscall.O_RDONLY, false)
	if err != nil {
		return definitionPathError(err)
	}
	if err := archive.Close(); err != nil {
		return definitionPathError(err)
	}
	if err := syscall.Renameat(int(f.directory.Fd()), archiveName, int(f.directory.Fd()), f.name); err != nil {
		return definitionPathError(&os.PathError{Op: "renameat", Path: f.name, Err: err})
	}
	if err := f.directory.Sync(); err != nil {
		return definitionPathError(err)
	}
	return nil
}

func definitionPathError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return fmt.Errorf("formations definition path rejected: %w", err)
}
