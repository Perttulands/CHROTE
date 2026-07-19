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

var (
	errAuthorityAtomicNoReplaceUnavailable = errors.New("atomic authority no-replace publication unavailable")
	errAuthorityDurabilityUncertain        = errors.New("authority publication durability uncertain")
)

type authorityContentRef struct {
	sha256 string
	size   int64
}

type authorityGeneration struct {
	recordRev uint64
	sha256    string
}

// authorityRevisionFunc strictly validates the complete closed record before
// returning its persisted recordRev.
type authorityRevisionFunc func([]byte) (uint64, error)

type authorityFileGeneration struct {
	generation authorityGeneration
	device     uint64
	inode      uint64
	raw        []byte
}

type authorityPublicationStep string

const (
	authorityPublicationStageSynced        authorityPublicationStep = "stage_synced"
	authorityPublicationInstalled          authorityPublicationStep = "installed"
	authorityPublicationMutableStageSynced authorityPublicationStep = "mutable_stage_synced"
	authorityPublicationMutableReplaced    authorityPublicationStep = "mutable_replaced"
	authorityPublicationDirectorySynced    authorityPublicationStep = "directory_synced"
)

type authorityPublicationHook func(authorityPublicationStep) error

type authorityPublicationOps struct {
	syncFile         func(*os.File) error
	syncDirectory    func(*os.File) error
	installNoReplace func(int, string, string) error
	replace          func(int, string, string) error
}

// authorityPublisher enforces publication integrity; it does not replace the
// registry or owner lock that the caller must hold for the complete operation.
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
			replace: func(directoryFD int, temporary, canonical string) error {
				return syscall.Renameat(directoryFD, temporary, directoryFD, canonical)
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
	if int64(len(raw)) > runtimeAuthorityMaxRecordBytes {
		return authorityContentRef{}, errRuntimeOutOfRange
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
			return authorityContentRef{}, authorityDurabilityUncertain(err)
		}
		return ref, nil
	}

	stage, err := p.stage(raw, authorityPublicationStageSynced)
	if err != nil {
		return authorityContentRef{}, err
	}
	defer stage.cleanup()
	if err := p.validateDirectory(); err != nil {
		return authorityContentRef{}, err
	}
	if err := stage.validate(raw); err != nil {
		return authorityContentRef{}, err
	}

	err = p.ops.installNoReplace(int(p.directory.Fd()), stage.name, name)
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
				return authorityContentRef{}, authorityDurabilityUncertain(syncErr)
			}
			return ref, nil
		}
		if authorityNoReplaceUnsupported(err) {
			return authorityContentRef{}, fmt.Errorf("%w: %v", errAuthorityAtomicNoReplaceUnavailable, err)
		}
		return authorityContentRef{}, fmt.Errorf("install immutable authority file: %w", err)
	}
	stage.installed = true
	if err := stage.validateInstalled(name, raw); err != nil {
		return authorityContentRef{}, authorityDurabilityUncertain(err)
	}
	if err := p.runHook(authorityPublicationInstalled); err != nil {
		return authorityContentRef{}, authorityDurabilityUncertain(err)
	}
	if err := p.syncDirectory(); err != nil {
		return authorityContentRef{}, authorityDurabilityUncertain(err)
	}
	return ref, nil
}

func (p *authorityPublisher) publishMutable(name string, expected *authorityGeneration, raw []byte, revisionOf authorityRevisionFunc) (authorityGeneration, error) {
	if p == nil || p.directory == nil || !runtimeAuthorityPathComponent(name) || revisionOf == nil {
		return authorityGeneration{}, errRuntimeNoncanonical
	}
	if int64(len(raw)) > runtimeAuthorityMaxRecordBytes {
		return authorityGeneration{}, errRuntimeOutOfRange
	}
	if err := p.validateDirectory(); err != nil {
		return authorityGeneration{}, err
	}
	if expected != nil {
		if expected.recordRev == 0 || expected.recordRev > runtimeAuthorityMaxJSONInteger {
			return authorityGeneration{}, errRuntimeOutOfRange
		}
		if !runtimeSHA256Pattern.MatchString(expected.sha256) {
			return authorityGeneration{}, errRuntimeNoncanonical
		}
		if expected.recordRev == runtimeAuthorityMaxJSONInteger {
			return authorityGeneration{}, errRuntimeOutOfRange
		}
	}
	nextRevision, err := revisionOf(raw)
	if err != nil {
		return authorityGeneration{}, err
	}
	if nextRevision == 0 || nextRevision > runtimeAuthorityMaxJSONInteger {
		return authorityGeneration{}, errRuntimeOutOfRange
	}
	next := authorityGeneration{recordRev: nextRevision, sha256: runtimeSHA256Hex(raw)}
	if expected == nil {
		if nextRevision != 1 {
			return authorityGeneration{}, errRuntimeConflict
		}
		if _, err := p.publishImmutable(name, raw); err != nil {
			return authorityGeneration{}, err
		}
		return next, nil
	}
	if nextRevision != expected.recordRev+1 {
		return authorityGeneration{}, errRuntimeConflict
	}

	current, exists, err := p.readMutable(name, revisionOf)
	if err != nil {
		return authorityGeneration{}, err
	}
	if !exists {
		return authorityGeneration{}, errRuntimeConflict
	}
	if current.generation != *expected {
		return authorityGeneration{}, errRuntimeConflict
	}

	stage, err := p.stage(raw, authorityPublicationMutableStageSynced)
	if err != nil {
		return authorityGeneration{}, err
	}
	defer stage.cleanup()

	rechecked, exists, err := p.readMutable(name, revisionOf)
	if err != nil {
		return authorityGeneration{}, err
	}
	if !exists || !sameAuthorityFileGeneration(current, rechecked) {
		return authorityGeneration{}, errRuntimeConflict
	}
	if err := p.validateDirectory(); err != nil {
		return authorityGeneration{}, err
	}
	if err := stage.validate(raw); err != nil {
		return authorityGeneration{}, err
	}
	if err := p.ops.replace(int(p.directory.Fd()), stage.name, name); err != nil {
		return authorityGeneration{}, fmt.Errorf("replace mutable authority file: %w", err)
	}
	stage.installed = true
	if err := stage.validateInstalled(name, raw); err != nil {
		return authorityGeneration{}, authorityDurabilityUncertain(err)
	}
	if err := p.runHook(authorityPublicationMutableReplaced); err != nil {
		return authorityGeneration{}, authorityDurabilityUncertain(err)
	}
	if err := p.syncDirectory(); err != nil {
		return authorityGeneration{}, authorityDurabilityUncertain(err)
	}
	return next, nil
}

func (p *authorityPublisher) readMutable(name string, revisionOf authorityRevisionFunc) (authorityFileGeneration, bool, error) {
	file, err := openAuthorityPrivateFileAt(p.directory, name, syscall.O_RDONLY, false, p.ownerUID)
	if errors.Is(err, os.ErrNotExist) {
		return authorityFileGeneration{}, false, nil
	}
	if err != nil {
		return authorityFileGeneration{}, true, fmt.Errorf("%w: unsafe mutable authority target: %v", errRuntimeIntegrityMismatch, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return authorityFileGeneration{}, true, err
	}
	if info.Size() < 0 || info.Size() > runtimeAuthorityMaxRecordBytes {
		return authorityFileGeneration{}, true, errRuntimeOutOfRange
	}
	raw, err := io.ReadAll(io.LimitReader(file, runtimeAuthorityMaxRecordBytes+1))
	if err != nil {
		return authorityFileGeneration{}, true, err
	}
	if int64(len(raw)) > runtimeAuthorityMaxRecordBytes {
		return authorityFileGeneration{}, true, errRuntimeOutOfRange
	}
	revision, err := revisionOf(raw)
	if err != nil {
		return authorityFileGeneration{}, true, err
	}
	if revision == 0 || revision > runtimeAuthorityMaxJSONInteger {
		return authorityFileGeneration{}, true, errRuntimeOutOfRange
	}
	if err := validateAuthorityPrivateFile(file, p.ownerUID); err != nil {
		return authorityFileGeneration{}, true, err
	}
	info, err = file.Stat()
	if err != nil {
		return authorityFileGeneration{}, true, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return authorityFileGeneration{}, true, errRuntimeIntegrityMismatch
	}
	return authorityFileGeneration{
		generation: authorityGeneration{recordRev: revision, sha256: runtimeSHA256Hex(raw)},
		device:     uint64(stat.Dev),
		inode:      stat.Ino,
		raw:        raw,
	}, true, nil
}

func sameAuthorityFileGeneration(left, right authorityFileGeneration) bool {
	return left.generation == right.generation &&
		left.device == right.device &&
		left.inode == right.inode &&
		bytes.Equal(left.raw, right.raw)
}

type authorityStagedFile struct {
	publisher *authorityPublisher
	file      *os.File
	name      string
	installed bool
}

func (p *authorityPublisher) stage(raw []byte, syncedStep authorityPublicationStep) (*authorityStagedFile, error) {
	name, err := newAuthorityPublicationStageName()
	if err != nil {
		return nil, err
	}
	file, err := openAuthorityPrivateFileAt(p.directory, name, syscall.O_WRONLY, true, p.ownerUID)
	if err != nil {
		return nil, err
	}
	stage := &authorityStagedFile{publisher: p, file: file, name: name}
	complete := false
	defer func() {
		if !complete {
			stage.cleanup()
		}
	}()
	if _, err := file.Write(raw); err != nil {
		return nil, err
	}
	if err := p.ops.syncFile(file); err != nil {
		return nil, err
	}
	if err := validateAuthorityPrivateFile(file, p.ownerUID); err != nil {
		return nil, err
	}
	if err := p.runHook(syncedStep); err != nil {
		return nil, err
	}
	complete = true
	return stage, nil
}

func (s *authorityStagedFile) cleanup() {
	if s == nil {
		return
	}
	if !s.installed && s.namedFileMatches(s.name) {
		_ = syscall.Unlinkat(int(s.publisher.directory.Fd()), s.name)
	}
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}

func (s *authorityStagedFile) validate(raw []byte) error {
	return s.validateNamedFile(s.name, raw)
}

func (s *authorityStagedFile) validateInstalled(canonical string, raw []byte) error {
	return s.validateNamedFile(canonical, raw)
}

func (s *authorityStagedFile) validateNamedFile(name string, raw []byte) error {
	if s == nil || s.publisher == nil || s.publisher.directory == nil || s.file == nil {
		return errRuntimeNoncanonical
	}
	if err := validateAuthorityPrivateFile(s.file, s.publisher.ownerUID); err != nil {
		return err
	}
	stagedInfo, err := s.file.Stat()
	if err != nil {
		return err
	}
	named, err := openAuthorityPrivateFileAt(s.publisher.directory, name, syscall.O_RDONLY, false, s.publisher.ownerUID)
	if err != nil {
		return fmt.Errorf("%w: reopen staged authority file: %v", errRuntimeIntegrityMismatch, err)
	}
	defer named.Close()
	namedInfo, err := named.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(stagedInfo, namedInfo) || namedInfo.Size() != int64(len(raw)) {
		return errRuntimeIntegrityMismatch
	}
	existing, err := io.ReadAll(io.LimitReader(named, int64(len(raw))+1))
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, raw) {
		return errRuntimeIntegrityMismatch
	}
	if err := validateAuthorityPrivateFile(s.file, s.publisher.ownerUID); err != nil {
		return err
	}
	if err := validateAuthorityPrivateFile(named, s.publisher.ownerUID); err != nil {
		return err
	}
	stagedInfo, err = s.file.Stat()
	if err != nil {
		return err
	}
	namedInfo, err = named.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(stagedInfo, namedInfo) {
		return errRuntimeIntegrityMismatch
	}
	return nil
}

func (s *authorityStagedFile) namedFileMatches(name string) bool {
	if s == nil || s.publisher == nil || s.publisher.directory == nil || s.file == nil {
		return false
	}
	stagedInfo, err := s.file.Stat()
	if err != nil {
		return false
	}
	fd, err := syscall.Openat(
		int(s.publisher.directory.Fd()),
		name,
		syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK,
		0,
	)
	if err != nil {
		return false
	}
	named := os.NewFile(uintptr(fd), name)
	if named == nil {
		_ = syscall.Close(fd)
		return false
	}
	defer named.Close()
	namedInfo, err := named.Stat()
	return err == nil && os.SameFile(stagedInfo, namedInfo)
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
	if err := p.validateDirectory(); err != nil {
		return err
	}
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

func authorityDurabilityUncertain(err error) error {
	return fmt.Errorf("%w: %w", errAuthorityDurabilityUncertain, err)
}
