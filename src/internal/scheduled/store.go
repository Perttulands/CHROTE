package scheduled

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultTasksDir = "/srv/data/chrote/scheduled-tasks"

const staleTaskLockAfter = time.Hour

var safeTaskID = regexp.MustCompile(`^tsk_[A-Za-z0-9_-]+$`)

// DefaultTasksDir returns the configured scheduled-task persistence directory.
func DefaultTasksDir() string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_SCHEDULED_TASKS_DIR")); override != "" {
		return override
	}
	return defaultTasksDir
}

// Store persists one JSON document per task.
type Store struct {
	Dir string
	mu  sync.Mutex
}

// NewStore creates a Store. Empty dir uses CHROTE_SCHEDULED_TASKS_DIR or the
// CHROTE service-owned default under /srv/data/chrote.
func NewStore(dir string) *Store {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = DefaultTasksDir()
	}
	return &Store{Dir: dir}
}

// List returns all persisted tasks sorted by creation time then ID.
func (s *Store) List() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

// Get loads a task by ID.
func (s *Store) Get(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}

// Save writes a task atomically.
func (s *Store) Save(task *Task) error {
	if task == nil {
		return fmt.Errorf("%w: task is required", ErrInvalid)
	}
	if !safeTaskID.MatchString(task.ID) {
		return fmt.Errorf("%w: invalid task id", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDirLocked(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeFileAtomic(s.taskPathLocked(task.ID), raw)
}

// Delete removes a task by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.checkedTaskPathLocked(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// TryLock creates a best-effort cross-process claim file for one task. The
// returned release function must be called by the claimant.
func (s *Store) TryLock(id string) (func(), bool, error) {
	if !safeTaskID.MatchString(id) {
		return nil, false, fmt.Errorf("%w: invalid task id", ErrInvalid)
	}

	s.mu.Lock()
	if err := s.ensureDirLocked(); err != nil {
		s.mu.Unlock()
		return nil, false, err
	}
	lockPath := s.lockPathLocked(id)
	s.mu.Unlock()

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o660)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			if s.reclaimStaleLock(lockPath, time.Now()) {
				file, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o660)
				if err == nil {
					_, _ = fmt.Fprintf(file, "%d %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
					_ = file.Chmod(0o660)
					release := func() {
						_ = file.Close()
						_ = os.Remove(lockPath)
					}
					return release, true, nil
				}
				if !errors.Is(err, os.ErrExist) {
					return nil, false, err
				}
			}
			return func() {}, false, nil
		}
		return nil, false, err
	}
	_, _ = fmt.Fprintf(file, "%d %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
	_ = file.Chmod(0o660)
	release := func() {
		_ = file.Close()
		_ = os.Remove(lockPath)
	}
	return release, true, nil
}

func (s *Store) reclaimStaleLock(lockPath string, now time.Time) bool {
	info, err := os.Stat(lockPath)
	if err != nil {
		return os.IsNotExist(err)
	}
	if now.Sub(info.ModTime()) < staleTaskLockAfter {
		return false
	}
	return os.Remove(lockPath) == nil
}

func (s *Store) listLocked() ([]Task, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, err
	}
	tasks := make([]Task, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !safeTaskID.MatchString(id) {
			continue
		}
		task, err := s.readTaskPathLocked(filepath.Join(s.Dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}
		return tasks[i].ID < tasks[j].ID
	})
	return tasks, nil
}

func (s *Store) getLocked(id string) (*Task, error) {
	path, err := s.checkedTaskPathLocked(id)
	if err != nil {
		return nil, err
	}
	task, err := s.readTaskPathLocked(path)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Store) readTaskPathLocked(path string) (*Task, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var task Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, fmt.Errorf("read scheduled task %s: %w", path, err)
	}
	return cloneTask(&task), nil
}

func (s *Store) ensureDirLocked() error {
	if err := os.MkdirAll(s.Dir, 0o2770); err != nil {
		return err
	}
	// Best effort: /srv lane operators commonly share this directory by group.
	_ = os.Chmod(s.Dir, 0o2770)
	return nil
}

func (s *Store) checkedTaskPathLocked(id string) (string, error) {
	if !safeTaskID.MatchString(id) {
		return "", fmt.Errorf("%w: invalid task id", ErrInvalid)
	}
	return s.taskPathLocked(id), nil
}

func (s *Store) taskPathLocked(id string) string {
	return filepath.Join(s.Dir, id+".json")
}

func (s *Store) lockPathLocked(id string) string {
	return filepath.Join(s.Dir, "."+id+".lock")
}

func writeFileAtomic(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o2770); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o2770)
	tmp, err := os.CreateTemp(dir, ".tmp-scheduled-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o660); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o660)
	return nil
}

func newTaskID(now time.Time) string {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "tsk_" + strconv.FormatInt(now.UnixNano(), 36)
	}
	return "tsk_" + strconv.FormatInt(now.UnixMilli(), 36) + "_" + hex.EncodeToString(random[:])
}

func newRunID(now time.Time) string {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "run_" + strconv.FormatInt(now.UnixNano(), 36)
	}
	return "run_" + strconv.FormatInt(now.UnixMilli(), 36) + "_" + hex.EncodeToString(random[:])
}
