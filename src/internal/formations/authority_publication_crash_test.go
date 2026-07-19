package formations

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	authorityCrashHelperEnv       = "CHROTE_TEST_AUTHORITY_CRASH_HELPER"
	authorityCrashDirectoryEnv    = "CHROTE_TEST_AUTHORITY_CRASH_DIRECTORY"
	authorityCrashOperationEnv    = "CHROTE_TEST_AUTHORITY_CRASH_OPERATION"
	authorityCrashStepEnv         = "CHROTE_TEST_AUTHORITY_CRASH_STEP"
	authorityCrashInstalledEnv    = "CHROTE_TEST_AUTHORITY_CRASH_INSTALLED"
	authorityCrashReadyFD         = uintptr(3)
	authorityCrashImmutableRecord = "complete immutable bytes\n"
)

func TestAuthorityPublisherSurvivesSIGKILLAtPublicationBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		step      authorityPublicationStep
		installed bool
	}{
		{name: "immutable/stage synced", operation: "immutable", step: authorityPublicationStageSynced},
		{name: "immutable/installed", operation: "immutable", step: authorityPublicationInstalled, installed: true},
		{name: "immutable/directory synced", operation: "immutable", step: authorityPublicationDirectorySynced, installed: true},
		{name: "mutable/stage synced", operation: "mutable", step: authorityPublicationMutableStageSynced},
		{name: "mutable/replaced", operation: "mutable", step: authorityPublicationMutableReplaced, installed: true},
		{name: "mutable/directory synced", operation: "mutable", step: authorityPublicationDirectorySynced, installed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory, path := openAuthorityTestDirectory(t)
			firstRaw := testAuthorityMutableRaw(1, "first")
			secondRaw := testAuthorityMutableRaw(2, "second")
			if test.operation == "mutable" {
				publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision); err != nil {
					t.Fatalf("publish initial mutable generation: %v", err)
				}
			}
			if err := directory.Close(); err != nil {
				t.Fatal(err)
			}

			killAuthorityPublisherAtStep(t, path, test.operation, test.step)

			canonical := filepath.Join(path, "bootstrap.json")
			installedRaw := []byte(authorityCrashImmutableRecord)
			if test.operation == "mutable" {
				canonical = filepath.Join(path, "workspace.private.json")
				installedRaw = firstRaw
				if test.installed {
					installedRaw = secondRaw
				}
			}
			if test.installed || test.operation == "mutable" {
				assertAuthorityTestFile(t, canonical, installedRaw)
			} else if _, err := os.Lstat(canonical); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("pre-install SIGKILL exposed canonical file: %v", err)
			}

			stages := authorityCrashStageNames(t, path)
			if test.installed {
				if len(stages) != 0 {
					t.Fatalf("post-install SIGKILL left named stages: %q", stages)
				}
			} else {
				if len(stages) != 1 {
					t.Fatalf("pre-install SIGKILL stages = %q, want one complete nonauthorizing stage", stages)
				}
				stagedRaw := []byte(authorityCrashImmutableRecord)
				if test.operation == "mutable" {
					stagedRaw = secondRaw
				}
				assertAuthorityTestFile(t, filepath.Join(path, stages[0]), stagedRaw)
			}

			runAuthorityPublisherRecovery(t, path, test.operation, test.installed)
			if test.operation == "immutable" {
				assertAuthorityTestFile(t, canonical, []byte(authorityCrashImmutableRecord))
			} else {
				assertAuthorityTestFile(t, canonical, secondRaw)
			}

			if after := authorityCrashStageNames(t, path); !slices.Equal(after, stages) {
				t.Fatalf("recovery created additional stale stages: before %q, after %q", stages, after)
			}
		})
	}
}

func TestAuthorityPublisherCrashHelper(t *testing.T) {
	mode := os.Getenv(authorityCrashHelperEnv)
	if mode == "" {
		return
	}
	path := os.Getenv(authorityCrashDirectoryEnv)
	operation := os.Getenv(authorityCrashOperationEnv)
	if path == "" {
		t.Fatal("authority crash helper missing directory")
	}
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if mode == "recover" {
		recoverAuthorityPublication(t, directory, operation, os.Getenv(authorityCrashInstalledEnv) == "1")
		return
	}
	if mode != "crash" {
		t.Fatalf("unknown authority helper mode %q", mode)
	}
	crashStep := authorityPublicationStep(os.Getenv(authorityCrashStepEnv))
	ready := os.NewFile(authorityCrashReadyFD, "authority-crash-ready")
	if ready == nil {
		t.Fatal("authority crash helper missing ready pipe")
	}
	defer ready.Close()
	publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), func(step authorityPublicationStep) error {
		if step != crashStep {
			return nil
		}
		if _, err := ready.Write([]byte{1}); err != nil {
			return err
		}
		for {
			time.Sleep(time.Hour)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	switch operation {
	case "immutable":
		_, err = publisher.publishImmutable("bootstrap.json", []byte(authorityCrashImmutableRecord))
	case "mutable":
		firstRaw := testAuthorityMutableRaw(1, "first")
		first := testAuthorityGeneration(1, firstRaw)
		_, err = publisher.publishMutable("workspace.private.json", &first, testAuthorityMutableRaw(2, "second"), testAuthorityMutableRevision)
	default:
		t.Fatalf("unknown crash operation %q", operation)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("publication returned without reaching crash step %q", crashStep)
}

func recoverAuthorityPublication(t *testing.T, directory *os.File, operation string, installed bool) {
	t.Helper()
	publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstRaw := testAuthorityMutableRaw(1, "first")
	secondRaw := testAuthorityMutableRaw(2, "second")
	switch {
	case operation == "immutable":
		if _, err := publisher.publishImmutable("bootstrap.json", []byte(authorityCrashImmutableRecord)); err != nil {
			t.Fatalf("recover immutable publication: %v", err)
		}
	case operation == "mutable" && installed:
		first := testAuthorityGeneration(1, firstRaw)
		if _, err := publisher.publishMutable("workspace.private.json", &first, secondRaw, testAuthorityMutableRevision); !errors.Is(err, errRuntimeConflict) {
			t.Fatalf("retry after ambiguous mutable commit = %v, want authoritative-reread conflict", err)
		}
		current, exists, err := publisher.readMutable("workspace.private.json", testAuthorityMutableRevision)
		if err != nil || !exists || current.generation != testAuthorityGeneration(2, secondRaw) {
			t.Fatalf("mutable authoritative reread = %+v, %t, %v", current.generation, exists, err)
		}
	case operation == "mutable":
		first := testAuthorityGeneration(1, firstRaw)
		if _, err := publisher.publishMutable("workspace.private.json", &first, secondRaw, testAuthorityMutableRevision); err != nil {
			t.Fatalf("recover pre-replace mutable publication: %v", err)
		}
	default:
		t.Fatalf("unknown recovery operation %q", operation)
	}
}

func killAuthorityPublisherAtStep(t *testing.T, directory, operation string, step authorityPublicationStep) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readyReader.Close()
	cmd := exec.Command(executable, "-test.run=^TestAuthorityPublisherCrashHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		authorityCrashHelperEnv+"=crash",
		authorityCrashDirectoryEnv+"="+directory,
		authorityCrashOperationEnv+"="+operation,
		authorityCrashStepEnv+"="+string(step),
	)
	cmd.ExtraFiles = []*os.File{readyWriter}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		readyWriter.Close()
		t.Fatal(err)
	}
	readyWriter.Close()

	readyResult := make(chan error, 1)
	go func() {
		var signal [1]byte
		_, err := io.ReadFull(readyReader, signal[:])
		readyResult <- err
	}()
	select {
	case err := <-readyResult:
		if err != nil {
			waitErr := cmd.Wait()
			t.Fatalf("crash helper exited before %q: ready=%v wait=%v output=%s", step, err, waitErr, output.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		waitErr := cmd.Wait()
		t.Fatalf("crash helper did not reach %q: wait=%v output=%s", step, waitErr, output.String())
	}

	if err := cmd.Process.Kill(); err != nil {
		_ = cmd.Wait()
		t.Fatalf("SIGKILL crash helper at %q: %v", step, err)
	}
	waitErr := cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("crash helper wait = %v, want SIGKILL; output=%s", waitErr, output.String())
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("crash helper status = %v, want SIGKILL; output=%s", exitErr.Sys(), output.String())
	}
}

func runAuthorityPublisherRecovery(t *testing.T, directory, operation string, installed bool) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	installedValue := "0"
	if installed {
		installedValue = "1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestAuthorityPublisherCrashHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		authorityCrashHelperEnv+"=recover",
		authorityCrashDirectoryEnv+"="+directory,
		authorityCrashOperationEnv+"="+operation,
		authorityCrashInstalledEnv+"="+installedValue,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("authority recovery helper timed out: output=%s", output)
		}
		t.Fatalf("authority recovery helper failed: %v output=%s", err, output)
	}
}

func authorityCrashStageNames(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".authority-stage-") {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names
}
