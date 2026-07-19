package formations

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	authorityPrivateDirectoryMode = os.FileMode(0o700)
	authorityPrivateFileMode      = os.FileMode(0o600)
	authorityPrivateSpecialMode   = os.ModeSetuid | os.ModeSetgid | os.ModeSticky
)

var errAuthorityAtomicNoReplaceUnavailable = errors.New("atomic authority no-replace publication unavailable")

type authorityContentRef struct {
	sha256 string
	size   int64
}

type authorityPublicationStep string

const (
	authorityPublicationStageSynced     authorityPublicationStep = "stage_synced"
	authorityPublicationInstalled       authorityPublicationStep = "installed"
	authorityPublicationDirectorySynced authorityPublicationStep = "directory_synced"
)

type authorityPublicationHook func(authorityPublicationStep) error

type authorityPublicationOps struct {
	syncFile         func(*os.File) error
	syncDirectory    func(*os.File) error
	installNoReplace func(int, string, string) error
}

type authorityPublisher struct {
	directory *os.File
	ownerUID  uint32
	hook      authorityPublicationHook
	ops       authorityPublicationOps
}

func newAuthorityPublisher(directory *os.File, ownerUID uint32, hook authorityPublicationHook) (*authorityPublisher, error) {
	publisher := &authorityPublisher{
		directory: directory,
		ownerUID:  ownerUID,
		hook:      hook,
		ops: authorityPublicationOps{
			syncFile:      func(file *os.File) error { return file.Sync() },
			syncDirectory: func(file *os.File) error { return file.Sync() },
			installNoReplace: func(directoryFD int, stage, canonical string) error {
				return unix.Renameat2(directoryFD, stage, directoryFD, canonical, unix.RENAME_NOREPLACE)
			},
		},
	}
	if err := publisher.validateDirectory(); err != nil {
		return nil, err
	}
	return publisher, nil
}

func (p *authorityPublisher) publishImmutable(name string, raw []byte) (authorityContentRef, error) {
	ref := authorityContentRef{sha256: runtimeSHA256Hex(raw), size: int64(len(raw))}
	if p == nil || p.directory == nil || !runtimeAuthorityPathComponent(name) {
		return authorityContentRef{}, errRuntimeNoncanonical
	}
	if err := p.validateDirectory(); err != nil {
		return authorityContentRef{}, err
	}
	if exact, exists, err := p.immutableMatches(name, raw); err != nil {
		return authorityContentRef{}, err
	} else if exists {
		if !exact {
			return authorityContentRef{}, errRuntimeConflict
		}
		if err := p.syncDirectory(); err != nil {
			return authorityContentRef{}, err
		}
		return ref, nil
	}

	stageName, err := newAuthorityPublicationStageName()
	if err != nil {
		return authorityContentRef{}, err
	}
	stage, err := openAuthorityPrivateFileAt(p.directory, stageName, syscall.O_WRONLY, true, p.ownerUID)
	if err != nil {
		return authorityContentRef{}, err
	}
	stageExists := true
	defer func() {
		_ = stage.Close()
		if stageExists {
			_ = syscall.Unlinkat(int(p.directory.Fd()), stageName)
		}
	}()

	if _, err := stage.Write(raw); err != nil {
		return authorityContentRef{}, err
	}
	if err := p.ops.syncFile(stage); err != nil {
		return authorityContentRef{}, err
	}
	if err := validateAuthorityPrivateFile(stage, p.ownerUID); err != nil {
		return authorityContentRef{}, err
	}
	if err := p.runHook(authorityPublicationStageSynced); err != nil {
		return authorityContentRef{}, err
	}

	err = p.ops.installNoReplace(int(p.directory.Fd()), stageName, name)
	if err != nil {
		exact, exists, matchErr := p.immutableMatches(name, raw)
		if matchErr != nil {
			return authorityContentRef{}, matchErr
		}
		if exists {
			if !exact {
				return authorityContentRef{}, errRuntimeConflict
			}
			if syncErr := p.syncDirectory(); syncErr != nil {
				return authorityContentRef{}, syncErr
			}
			return ref, nil
		}
		if authorityNoReplaceUnsupported(err) {
			return authorityContentRef{}, fmt.Errorf("%w: %v", errAuthorityAtomicNoReplaceUnavailable, err)
		}
		return authorityContentRef{}, fmt.Errorf("install immutable authority file: %w", err)
	}
	stageExists = false
	if err := p.runHook(authorityPublicationInstalled); err != nil {
		return authorityContentRef{}, err
	}
	if err := p.syncDirectory(); err != nil {
		return authorityContentRef{}, err
	}
	return ref, nil
}

func (p *authorityPublisher) immutableMatches(name string, raw []byte) (bool, bool, error) {
	file, err := openAuthorityPrivateFileAt(p.directory, name, syscall.O_RDONLY, false, p.ownerUID)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, true, fmt.Errorf("%w: unsafe immutable authority target: %v", errRuntimeIntegrityMismatch, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, true, err
	}
	if info.Size() != int64(len(raw)) {
		return false, true, nil
	}
	existing, err := io.ReadAll(io.LimitReader(file, int64(len(raw))+1))
	if err != nil {
		return false, true, err
	}
	return bytes.Equal(existing, raw), true, nil
}

func (p *authorityPublisher) validateDirectory() error {
	if p == nil || p.directory == nil {
		return errRuntimeNoncanonical
	}
	info, err := p.directory.Stat()
	if err != nil {
		return fmt.Errorf("%w: stat authority directory: %v", errRuntimeIntegrityMismatch, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !authorityPrivateModeIsExact(info.Mode(), authorityPrivateDirectoryMode) || !ok || stat.Uid != p.ownerUID {
		return fmt.Errorf("%w: authority directory must be owner %d mode 0700; got mode %v stat %T", errRuntimeIntegrityMismatch, p.ownerUID, info.Mode(), info.Sys())
	}
	return nil
}

func (p *authorityPublisher) syncDirectory() error {
	if err := p.ops.syncDirectory(p.directory); err != nil {
		return err
	}
	return p.runHook(authorityPublicationDirectorySynced)
}

func (p *authorityPublisher) runHook(step authorityPublicationStep) error {
	if p.hook == nil {
		return nil
	}
	return p.hook(step)
}

func openAuthorityPrivateFileAt(directory *os.File, name string, flags int, create bool, ownerUID uint32) (*os.File, error) {
	if directory == nil || !runtimeAuthorityPathComponent(name) {
		return nil, errRuntimeNoncanonical
	}
	flags |= syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		flags |= syscall.O_CREAT | syscall.O_EXCL
	}
	fd, err := syscall.Openat(int(directory.Fd()), name, flags, uint32(authorityPrivateFileMode.Perm()))
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		if create {
			_ = syscall.Unlinkat(int(directory.Fd()), name)
		}
		return nil, errors.New("could not open private authority file")
	}
	valid := false
	defer func() {
		if valid {
			return
		}
		_ = file.Close()
		if create {
			_ = syscall.Unlinkat(int(directory.Fd()), name)
		}
	}()
	if create {
		if err := syscall.Fchmod(fd, uint32(authorityPrivateFileMode.Perm())); err != nil {
			return nil, &os.PathError{Op: "fchmod", Path: name, Err: err}
		}
	}
	if err := validateAuthorityPrivateFile(file, ownerUID); err != nil {
		return nil, err
	}
	valid = true
	return file, nil
}

func validateAuthorityPrivateFile(file *os.File, ownerUID uint32) error {
	if file == nil {
		return errRuntimeNoncanonical
	}
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("%w: stat private authority file: %v", errRuntimeIntegrityMismatch, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !authorityPrivateModeIsExact(info.Mode(), authorityPrivateFileMode) || !ok || stat.Uid != ownerUID || stat.Nlink != 1 {
		return errRuntimeIntegrityMismatch
	}
	return nil
}

func authorityPrivateModeIsExact(mode, want os.FileMode) bool {
	return mode.Perm() == want && mode&authorityPrivateSpecialMode == 0
}

func newAuthorityPublicationStageName() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate authority stage identity: %w", err)
	}
	return ".authority-stage-" + hex.EncodeToString(random), nil
}

func authorityNoReplaceUnsupported(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EOPNOTSUPP)
}
