package formations

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func (s *PersonaStore) openPersonaDirectory(create bool) (*os.File, error) {
	if create {
		if err := os.MkdirAll(s.AgentsDir, sharedDirMode); err != nil {
			return nil, err
		}
	}
	fd, err := syscall.Open(s.AgentsDir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(fd), s.AgentsDir)
	if directory == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("open persona directory returned nil file")
	}
	info, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	if !info.IsDir() {
		_ = directory.Close()
		return nil, fmt.Errorf("persona root %q is not a directory", s.AgentsDir)
	}
	if create && !hasSharedDirMode(info.Mode()) {
		if err := directory.Chmod(sharedDirMode); err != nil {
			_ = directory.Close()
			return nil, err
		}
	}
	return directory, nil
}

func (s *PersonaStore) listPersonaEntries() ([]os.DirEntry, error) {
	directory, err := s.openPersonaDirectory(false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []os.DirEntry{}, nil
		}
		return nil, err
	}
	defer directory.Close()
	return directory.ReadDir(-1)
}

func (s *PersonaStore) withPersonaLock(id string, fn func() error) error {
	directory, err := s.openPersonaDirectory(true)
	if err != nil {
		return err
	}
	defer directory.Close()

	lockName := id + ".toml.lock"
	mutex := mutexFor(filepath.Join(s.AgentsDir, lockName))
	mutex.Lock()
	defer mutex.Unlock()

	fd, err := syscall.Openat(int(directory.Fd()), lockName, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, uint32(sharedFileMode.Perm()))
	if err != nil {
		return fmt.Errorf("open persona lock %q: %w", lockName, err)
	}
	lockFile := os.NewFile(uintptr(fd), lockName)
	if lockFile == nil {
		_ = syscall.Close(fd)
		return errors.New("open persona lock returned nil file")
	}
	defer lockFile.Close()
	if err := requireRegularPersonaFile(lockFile, lockName); err != nil {
		return err
	}
	if err := lockFile.Chmod(sharedFileMode); err != nil {
		return err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(fd, syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func (s *PersonaStore) readPersonaRaw(id string) ([]byte, error) {
	directory, err := s.openPersonaDirectory(false)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	name := id + ".toml"
	fd, err := syscall.Openat(int(directory.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("open persona card returned nil file")
	}
	defer file.Close()
	if err := requireRegularPersonaFile(file, name); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}

func (s *PersonaStore) personaExists(id string) (bool, error) {
	directory, err := s.openPersonaDirectory(true)
	if err != nil {
		return false, err
	}
	defer directory.Close()
	name := id + ".toml"
	fd, err := syscall.Openat(int(directory.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return false, errors.New("open persona card returned nil file")
	}
	defer file.Close()
	if err := requireRegularPersonaFile(file, name); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PersonaStore) writePersonaAtomic(id string, raw []byte) error {
	directory, err := s.openPersonaDirectory(true)
	if err != nil {
		return err
	}
	defer directory.Close()
	name := id + ".toml"
	temporaryName := "." + name + ".tmp-" + newPrefixedID("persona")
	fd, err := syscall.Openat(int(directory.Fd()), temporaryName, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, uint32(sharedFileMode.Perm()))
	if err != nil {
		return err
	}
	temporary := os.NewFile(uintptr(fd), temporaryName)
	if temporary == nil {
		_ = syscall.Close(fd)
		return errors.New("create persona temporary returned nil file")
	}
	temporaryExists := true
	defer func() {
		_ = temporary.Close()
		if temporaryExists {
			_ = syscall.Unlinkat(int(directory.Fd()), temporaryName)
		}
	}()
	if err := temporary.Chmod(sharedFileMode); err != nil {
		return err
	}
	written, err := temporary.Write(raw)
	if err != nil {
		return err
	}
	if written != len(raw) {
		return io.ErrShortWrite
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := syscall.Renameat(int(directory.Fd()), temporaryName, int(directory.Fd()), name); err != nil {
		return err
	}
	temporaryExists = false
	return directory.Sync()
}

func requireRegularPersonaFile(file *os.File, name string) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return fmt.Errorf("persona file %q is not regular", name)
	}
	return nil
}
