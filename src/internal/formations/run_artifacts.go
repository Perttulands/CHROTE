package formations

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const runArtifactDirectoryMode = uint32(0o2770)

type runArtifactDirectory struct {
	file *os.File
	path string
	slug string
}

func (d *runArtifactDirectory) close() {
	if d != nil && d.file != nil {
		_ = d.file.Close()
	}
}

type runLedgerHandle struct {
	directory *runArtifactDirectory
	file      *os.File
	runID     string
	path      string
}

func (h *runLedgerHandle) close() {
	if h == nil {
		return
	}
	if h.file != nil {
		_ = h.file.Close()
	}
	if h.directory != nil {
		h.directory.close()
	}
}

func validRunID(runID string) bool {
	return strings.HasPrefix(runID, "run_") && runtimeAuthorityPathComponent(runID) && !strings.Contains(runID, "..")
}

func (s *Store) workspaceAbsolutePath() (string, error) {
	workspace, err := filepath.Abs(s.workspaceRoot())
	if err != nil {
		return "", err
	}
	workspace = filepath.Clean(workspace)
	if !filepath.IsAbs(workspace) {
		return "", ErrRunLedgerInvalid
	}
	return workspace, nil
}

func openOrCreateRunArtifactDirectoryAt(parent *os.File, name string) (*os.File, error) {
	directory, err := openRuntimeAuthorityDirectoryAt(parent, name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if errors.Is(err, os.ErrNotExist) {
		if err := syscall.Mkdirat(int(parent.Fd()), name, runArtifactDirectoryMode); err != nil && !errors.Is(err, syscall.EEXIST) {
			return nil, &os.PathError{Op: "mkdirat", Path: name, Err: err}
		}
		directory, err = openRuntimeAuthorityDirectoryAt(parent, name)
		if err != nil {
			return nil, err
		}
	}
	if err := syscall.Fchmod(int(directory.Fd()), runArtifactDirectoryMode); err != nil {
		_ = directory.Close()
		return nil, &os.PathError{Op: "fchmod", Path: name, Err: err}
	}
	return directory, nil
}

func (s *Store) openRunsDirectory(create bool) (*os.File, string, error) {
	workspace, err := s.workspaceAbsolutePath()
	if err != nil {
		return nil, "", err
	}
	current, err := openRuntimeAuthorityRoot(workspace)
	if err != nil {
		return nil, "", err
	}
	for _, component := range []string{".formations", "runs"} {
		var next *os.File
		if create {
			next, err = openOrCreateRunArtifactDirectoryAt(current, component)
		} else {
			next, err = openRuntimeAuthorityDirectoryAt(current, component)
		}
		_ = current.Close()
		if err != nil {
			return nil, "", err
		}
		current = next
	}
	return current, workspace, nil
}

func (s *Store) openRunArtifactDirectory(slug string, create bool) (*runArtifactDirectory, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	runs, workspace, err := s.openRunsDirectory(create)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: open run root: %v", ErrRunLedgerInvalid, err)
	}
	defer runs.Close()
	var directory *os.File
	if create {
		directory, err = openOrCreateRunArtifactDirectoryAt(runs, slug)
	} else {
		directory, err = openRuntimeAuthorityDirectoryAt(runs, slug)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: open run directory: %v", ErrRunLedgerInvalid, err)
	}
	return &runArtifactDirectory{
		file: directory,
		path: filepath.Join(workspace, ".formations", "runs", slug),
		slug: slug,
	}, nil
}

func openRunArtifactFileAt(directory *os.File, name string, flags int, create bool) (*os.File, error) {
	if directory == nil || !runtimeAuthorityPathComponent(name) {
		return nil, &os.PathError{Op: "openat", Path: name, Err: syscall.EINVAL}
	}
	flags |= syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		flags |= syscall.O_CREAT
	}
	fd, err := syscall.Openat(int(directory.Fd()), name, flags, uint32(sharedFileMode.Perm()))
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open run artifact")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("run artifact is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		_ = file.Close()
		return nil, errors.New("run artifact must have exactly one link")
	}
	if create {
		if err := syscall.Fchmod(fd, uint32(sharedFileMode.Perm())); err != nil {
			_ = file.Close()
			return nil, &os.PathError{Op: "fchmod", Path: name, Err: err}
		}
	}
	return file, nil
}

func writeRunArtifactExclusiveAt(directory *runArtifactDirectory, name string, raw []byte) error {
	if directory == nil || directory.file == nil {
		return ErrRunLedgerInvalid
	}
	file, err := openRunArtifactFileAt(directory.file, name, syscall.O_WRONLY|syscall.O_EXCL, true)
	if err != nil {
		if errors.Is(err, syscall.EEXIST) {
			return ErrConflict
		}
		return fmt.Errorf("%w: create run artifact: %v", ErrRunLedgerInvalid, err)
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = syscall.Unlinkat(int(directory.file.Fd()), name)
		}
	}()
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := directory.file.Sync(); err != nil {
		return err
	}
	complete = true
	return nil
}

func (s *Store) openRunLedger(runID string, writable bool) (*runLedgerHandle, error) {
	if !validRunID(runID) {
		return nil, ErrInvalidSlug
	}
	runs, workspace, err := s.openRunsDirectory(false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: open run root: %v", ErrRunLedgerInvalid, err)
	}
	defer runs.Close()

	var match *runLedgerHandle
	for {
		names, done, readErr := readRuntimeAuthorityDirectoryNameBatch(runs, runtimeAuthorityDirectoryBatchSize)
		if readErr != nil {
			if match != nil {
				match.close()
			}
			return nil, fmt.Errorf("%w: enumerate run directories: %v", ErrRunLedgerInvalid, readErr)
		}
		for _, slug := range names {
			if validateSlug(slug) != nil {
				continue
			}
			directoryFile, openErr := openRuntimeAuthorityDirectoryAt(runs, slug)
			if openErr != nil {
				if errors.Is(openErr, syscall.ELOOP) || errors.Is(openErr, syscall.ENOTDIR) || errors.Is(openErr, os.ErrNotExist) {
					continue
				}
				if match != nil {
					match.close()
				}
				return nil, fmt.Errorf("%w: open run directory: %v", ErrRunLedgerInvalid, openErr)
			}
			flags := syscall.O_RDONLY
			if writable {
				flags = syscall.O_RDWR | syscall.O_APPEND
			}
			ledgerFile, ledgerErr := openRunArtifactFileAt(directoryFile, runID+".ndjson", flags, false)
			if ledgerErr != nil {
				_ = directoryFile.Close()
				if errors.Is(ledgerErr, os.ErrNotExist) {
					continue
				}
				if match != nil {
					match.close()
				}
				return nil, fmt.Errorf("%w: open run ledger: %v", ErrRunLedgerInvalid, ledgerErr)
			}
			directory := &runArtifactDirectory{
				file: directoryFile,
				path: filepath.Join(workspace, ".formations", "runs", slug),
				slug: slug,
			}
			candidate := &runLedgerHandle{
				directory: directory,
				file:      ledgerFile,
				runID:     runID,
				path:      filepath.Join(directory.path, runID+".ndjson"),
			}
			if match != nil {
				match.close()
				candidate.close()
				return nil, fmt.Errorf("%w: run id %q appears in multiple ledgers", ErrRunLedgerInvalid, runID)
			}
			match = candidate
		}
		if done {
			break
		}
	}
	if match == nil {
		return nil, ErrNotFound
	}
	return match, nil
}

func (s *Store) listRunIDs() ([]string, error) {
	runs, _, err := s.openRunsDirectory(false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("%w: open run root: %v", ErrRunLedgerInvalid, err)
	}
	defer runs.Close()
	seen := map[string]string{}
	for {
		slugs, done, readErr := readRuntimeAuthorityDirectoryNameBatch(runs, runtimeAuthorityDirectoryBatchSize)
		if readErr != nil {
			return nil, fmt.Errorf("%w: enumerate run directories: %v", ErrRunLedgerInvalid, readErr)
		}
		for _, slug := range slugs {
			if validateSlug(slug) != nil {
				continue
			}
			directory, openErr := openRuntimeAuthorityDirectoryAt(runs, slug)
			if openErr != nil {
				if errors.Is(openErr, syscall.ELOOP) || errors.Is(openErr, syscall.ENOTDIR) || errors.Is(openErr, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("%w: open run directory: %v", ErrRunLedgerInvalid, openErr)
			}
			for {
				names, namesDone, namesErr := readRuntimeAuthorityDirectoryNameBatch(directory, runtimeAuthorityDirectoryBatchSize)
				if namesErr != nil {
					_ = directory.Close()
					return nil, fmt.Errorf("%w: enumerate run artifacts: %v", ErrRunLedgerInvalid, namesErr)
				}
				for _, name := range names {
					if !strings.HasSuffix(name, ".ndjson") {
						continue
					}
					runID := strings.TrimSuffix(name, ".ndjson")
					if !validRunID(runID) {
						continue
					}
					file, fileErr := openRunArtifactFileAt(directory, name, syscall.O_RDONLY, false)
					if fileErr != nil {
						_ = directory.Close()
						return nil, fmt.Errorf("%w: open run ledger: %v", ErrRunLedgerInvalid, fileErr)
					}
					_ = file.Close()
					if previous, ok := seen[runID]; ok {
						_ = directory.Close()
						return nil, fmt.Errorf("%w: run id %q appears in multiple ledgers: %s and %s", ErrRunLedgerInvalid, runID, previous, slug)
					}
					seen[runID] = slug
				}
				if namesDone {
					break
				}
			}
			_ = directory.Close()
		}
		if done {
			break
		}
	}
	runIDs := make([]string, 0, len(seen))
	for runID := range seen {
		runIDs = append(runIDs, runID)
	}
	sort.Strings(runIDs)
	return runIDs, nil
}

func withRunArtifactLock(directory *runArtifactDirectory, ledgerName, lockKey string, fn func() error) error {
	if directory == nil || directory.file == nil || !runtimeAuthorityPathComponent(ledgerName) {
		return ErrRunLedgerInvalid
	}
	lockName := ledgerName + ".lock"
	mutex := mutexFor(lockKey)
	mutex.Lock()
	defer mutex.Unlock()

	lockFile, err := openRunArtifactFileAt(directory.file, lockName, syscall.O_RDWR, true)
	if err != nil {
		return fmt.Errorf("%w: open run ledger lock: %v", ErrRunLedgerInvalid, err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("%w: lock run ledger: %v", ErrRunLedgerInvalid, err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) //nolint:errcheck // best-effort unlock on return
	return fn()
}

func (h *runLedgerHandle) withLock(fn func() error) error {
	if h == nil || h.file == nil || h.directory == nil {
		return ErrRunLedgerInvalid
	}
	return withRunArtifactLock(h.directory, h.runID+".ndjson", h.path+".lock", fn)
}

func classifyAndReadRunEvents(file *os.File, expectedRunID string) ([]RunEvent, error) {
	if file == nil {
		return nil, ErrRunLedgerInvalid
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("%w: seek run ledger: %v", ErrRunLedgerInvalid, err)
	}
	class, err := classifyRuntimeAuthorityLedger(file, runtimeAuthoritySchema, expectedRunID)
	if err != nil {
		if runtimeAuthorityClassifierRequiresAuthority(err) {
			return nil, fmt.Errorf("%w: %w: classify run ledger: %v", ErrRunLedgerInvalid, ErrRuntimeAuthorityNonAuthorizing, err)
		}
		return nil, fmt.Errorf("%w: classify run ledger: %v", ErrRunLedgerInvalid, err)
	}
	if class != RuntimeAuthoritySchema1Inspection {
		return nil, fmt.Errorf("%w: %w: schema-2 ledger", ErrRunLedgerInvalid, ErrRuntimeAuthorityNonAuthorizing)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("%w: seek run ledger: %v", ErrRunLedgerInvalid, err)
	}
	reader := bufio.NewReaderSize(file, runtimeAuthorityMaxEventBytes+1)
	events := make([]RunEvent, 0)
	for {
		line, lineErr := readRuntimeAuthorityLedgerLine(reader)
		if errors.Is(lineErr, io.EOF) {
			break
		}
		if lineErr != nil {
			return nil, fmt.Errorf("%w: read run ledger: %v", ErrRunLedgerInvalid, lineErr)
		}
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("%w: blank run event", ErrRunLedgerInvalid)
		}
		var event RunEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRunLedgerInvalid, err)
		}
		events = append(events, event)
	}
	return events, nil
}

func runtimeAuthorityClassifierRequiresAuthority(err error) bool {
	var decodeErr runtimeDecodeError
	if !errors.As(err, &decodeErr) {
		return false
	}
	switch decodeErr.code {
	case RuntimeAuthorityGuardUnknownKey, RuntimeAuthorityGuardUnsupportedSchema, RuntimeAuthorityGuardMixedSchema:
		return true
	default:
		return false
	}
}

func appendRunEventToFile(file *os.File, directory *os.File, event RunEvent) error {
	if event.Type == "" {
		return fmt.Errorf("%w: event type required", ErrInvalidSlug)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if len(raw) > runtimeAuthorityMaxEventBytes {
		return fmt.Errorf("%w: run event exceeds byte limit", ErrRunLedgerInvalid)
	}
	raw = append(raw, '\n')
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if directory != nil {
		return directory.Sync()
	}
	return nil
}

func writeInitialRunEventAt(directory *runArtifactDirectory, runID string, event RunEvent) error {
	if directory == nil || !validRunID(runID) {
		return ErrRunLedgerInvalid
	}
	ledgerName := runID + ".ndjson"
	ledgerPath := filepath.Join(directory.path, ledgerName)
	return withRunArtifactLock(directory, ledgerName, ledgerPath+".lock", func() error {
		file, err := openRunArtifactFileAt(directory.file, ledgerName, syscall.O_RDWR|syscall.O_APPEND|syscall.O_EXCL, true)
		if err != nil {
			if errors.Is(err, syscall.EEXIST) {
				return ErrConflict
			}
			return fmt.Errorf("%w: create run ledger: %v", ErrRunLedgerInvalid, err)
		}
		complete := false
		defer func() {
			_ = file.Close()
			if !complete {
				_ = syscall.Unlinkat(int(directory.file.Fd()), ledgerName)
			}
		}()
		if err := appendRunEventToFile(file, directory.file, event); err != nil {
			return err
		}
		complete = true
		return nil
	})
}

func openOrCreateAbsoluteDirectory(path string) (*runArtifactDirectory, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || clean != path {
		return nil, ErrRunLedgerInvalid
	}
	current, err := openRuntimeAuthorityRoot(string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimPrefix(clean, string(filepath.Separator))
	if trimmed == "" {
		return &runArtifactDirectory{file: current, path: clean}, nil
	}
	components := strings.Split(trimmed, string(filepath.Separator))
	for index, component := range components {
		next, openErr := openRuntimeAuthorityDirectoryAt(current, component)
		if openErr != nil && !errors.Is(openErr, os.ErrNotExist) {
			_ = current.Close()
			return nil, openErr
		}
		created := errors.Is(openErr, os.ErrNotExist)
		if created {
			if err := syscall.Mkdirat(int(current.Fd()), component, runArtifactDirectoryMode); err != nil && !errors.Is(err, syscall.EEXIST) {
				_ = current.Close()
				return nil, err
			}
			next, openErr = openRuntimeAuthorityDirectoryAt(current, component)
			if openErr != nil {
				_ = current.Close()
				return nil, openErr
			}
		}
		_ = current.Close()
		current = next
		if created || index == len(components)-1 {
			if err := syscall.Fchmod(int(current.Fd()), runArtifactDirectoryMode); err != nil {
				_ = current.Close()
				return nil, err
			}
		}
	}
	return &runArtifactDirectory{file: current, path: clean}, nil
}
