package formations

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

var lockRegistry sync.Map

const (
	sharedDirMode  = os.ModeSetgid | 0o770
	sharedFileMode = os.FileMode(0o660)
)

func withFileLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	if err := ensureSharedDir(filepath.Dir(lockPath)); err != nil {
		return err
	}

	mutex := mutexFor(lockPath)
	mutex.Lock()
	defer mutex.Unlock()

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, sharedFileMode)
	if err != nil {
		return err
	}
	if err := ensureSharedFile(lockPath); err != nil {
		_ = lockFile.Close()
		return err
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) //nolint:errcheck // best-effort unlock on return

	return fn()
}

func mutexFor(lockPath string) *sync.Mutex {
	value, _ := lockRegistry.LoadOrStore(lockPath, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func ensureSharedDir(dir string) error {
	if err := os.MkdirAll(dir, sharedDirMode); err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if hasSharedDirMode(info.Mode()) {
		return nil
	}
	if err := os.Chmod(dir, sharedDirMode); err != nil {
		return err
	}
	return nil
}

func hasSharedDirMode(mode os.FileMode) bool {
	return mode.Perm() == 0o770 && mode&os.ModeSetgid != 0
}

func ensureSharedFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() == sharedFileMode {
		return nil
	}
	return os.Chmod(path, sharedFileMode)
}

func writeAtomic(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := ensureSharedDir(dir); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, sharedFileMode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return fsyncDir(dir)
}

func fsyncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
