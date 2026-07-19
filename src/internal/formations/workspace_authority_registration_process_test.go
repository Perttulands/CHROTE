package formations

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

const (
	workspaceAuthorityLockHelperEnv       = "CHROTE_TEST_WORKSPACE_AUTHORITY_LOCK_HELPER"
	workspaceAuthorityLockRootEnv         = "CHROTE_TEST_WORKSPACE_AUTHORITY_LOCK_ROOT"
	workspaceAuthorityLockWorkspaceEnv    = "CHROTE_TEST_WORKSPACE_AUTHORITY_LOCK_WORKSPACE"
	workspaceAuthorityLockAttemptedFD     = uintptr(3)
	workspaceAuthorityLockReadyFD         = uintptr(4)
	workspaceAuthorityLockReleaseFD       = uintptr(5)
	workspaceAuthorityLockProcessTimeout  = 10 * time.Second
	workspaceAuthorityLockExclusionWindow = 250 * time.Millisecond
)

func TestWorkspaceAuthorityRegistryCriticalSectionSerializesAcrossProcesses(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)

	holder := startWorkspaceAuthorityLockHelper(t, fixture.hostRoot, fixture.workspace)
	holder.waitAttempted(t)
	holderLock := holder.waitReady(t, workspaceAuthorityLockProcessTimeout)

	contender := startWorkspaceAuthorityLockHelper(t, fixture.hostRoot, fixture.workspace)
	contender.waitAttempted(t)
	contender.assertNotReadyBeforeRelease(t)

	holder.releaseAndWait(t)
	contenderLock := contender.waitReady(t, workspaceAuthorityLockProcessTimeout)
	if contenderLock != holderLock {
		t.Fatalf("processes serialized on different registry lock inodes: holder=%+v contender=%+v", holderLock, contenderLock)
	}
	pathLock := workspaceAuthorityLockIdentityAtPath(t, fixture.registryLock)
	if holderLock != pathLock {
		t.Fatalf("process registry lock identity = %+v, want named lock identity %+v", holderLock, pathLock)
	}
	contender.releaseAndWait(t)

	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("cross-process registry locking changed authority topology\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func TestWorkspaceAuthorityRegistryLockProcessHelper(t *testing.T) {
	if os.Getenv(workspaceAuthorityLockHelperEnv) == "" {
		return
	}
	hostRoot := os.Getenv(workspaceAuthorityLockRootEnv)
	workspace := os.Getenv(workspaceAuthorityLockWorkspaceEnv)
	if hostRoot == "" || workspace == "" || !filepath.IsAbs(hostRoot) || !filepath.IsAbs(workspace) {
		t.Fatal("workspace authority lock helper requires explicit absolute root and workspace")
	}
	attempted := os.NewFile(workspaceAuthorityLockAttemptedFD, "workspace-authority-lock-attempted")
	ready := os.NewFile(workspaceAuthorityLockReadyFD, "workspace-authority-lock-ready")
	release := os.NewFile(workspaceAuthorityLockReleaseFD, "workspace-authority-lock-release")
	if attempted == nil || ready == nil || release == nil {
		t.Fatal("workspace authority lock helper missing synchronization pipe")
	}
	defer attempted.Close()
	defer ready.Close()
	defer release.Close()

	if _, err := attempted.Write([]byte{1}); err != nil {
		t.Fatalf("signal registry lock attempt: %v", err)
	}
	registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate(), nil, nil)
	inspection, err := registrar.inspect(workspace)
	if err != nil {
		t.Fatalf("acquire process-shared registry critical section: %v", err)
	}
	defer func() {
		if err := inspection.close(); err != nil {
			t.Errorf("close process registry inspection: %v", err)
		}
	}()

	var identity workspaceAuthorityLockIdentity
	inspection.observeRegistryLock(func(registryLock *os.File) {
		lockInfo, statErr := registryLock.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		lockStat, ok := lockInfo.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatalf("registry lock stat type = %T, want *syscall.Stat_t", lockInfo.Sys())
		}
		identity = workspaceAuthorityLockIdentity{device: uint64(lockStat.Dev), inode: lockStat.Ino}
	})
	var payload [16]byte
	binary.BigEndian.PutUint64(payload[:8], identity.device)
	binary.BigEndian.PutUint64(payload[8:], identity.inode)
	if _, err := ready.Write(payload[:]); err != nil {
		t.Fatalf("signal acquired registry lock: %v", err)
	}
	if err := release.SetReadDeadline(time.Now().Add(workspaceAuthorityLockProcessTimeout)); err != nil {
		t.Fatal(err)
	}
	var signal [1]byte
	if _, err := io.ReadFull(release, signal[:]); err != nil {
		t.Fatalf("wait for registry lock release signal: %v", err)
	}
}

type workspaceAuthorityLockIdentity struct {
	device uint64
	inode  uint64
}

type workspaceAuthorityLockHelper struct {
	command   *exec.Cmd
	cancel    context.CancelFunc
	attempted *os.File
	ready     *os.File
	release   *os.File
	output    *bytes.Buffer
	waited    bool
}

func startWorkspaceAuthorityLockHelper(t *testing.T, hostRoot, workspace string) *workspaceAuthorityLockHelper {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	attemptedReader, attemptedWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		attemptedReader.Close()
		attemptedWriter.Close()
		t.Fatal(err)
	}
	releaseReader, releaseWriter, err := os.Pipe()
	if err != nil {
		attemptedReader.Close()
		attemptedWriter.Close()
		readyReader.Close()
		readyWriter.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspaceAuthorityLockProcessTimeout)
	output := &bytes.Buffer{}
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWorkspaceAuthorityRegistryLockProcessHelper$", "-test.count=1")
	command.Env = append(os.Environ(),
		workspaceAuthorityLockHelperEnv+"=1",
		workspaceAuthorityLockRootEnv+"="+hostRoot,
		workspaceAuthorityLockWorkspaceEnv+"="+workspace,
	)
	command.ExtraFiles = []*os.File{attemptedWriter, readyWriter, releaseReader}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		cancel()
		attemptedReader.Close()
		attemptedWriter.Close()
		readyReader.Close()
		readyWriter.Close()
		releaseReader.Close()
		releaseWriter.Close()
		t.Fatal(err)
	}
	attemptedWriter.Close()
	readyWriter.Close()
	releaseReader.Close()
	helper := &workspaceAuthorityLockHelper{
		command:   command,
		cancel:    cancel,
		attempted: attemptedReader,
		ready:     readyReader,
		release:   releaseWriter,
		output:    output,
	}
	t.Cleanup(func() { helper.cleanup() })
	return helper
}

func (helper *workspaceAuthorityLockHelper) waitAttempted(t *testing.T) {
	t.Helper()
	if err := helper.attempted.SetReadDeadline(time.Now().Add(workspaceAuthorityLockProcessTimeout)); err != nil {
		t.Fatal(err)
	}
	var signal [1]byte
	if _, err := io.ReadFull(helper.attempted, signal[:]); err != nil {
		helper.fail(t, "wait for registry-lock attempt", err)
	}
}

func (helper *workspaceAuthorityLockHelper) waitReady(t *testing.T, timeout time.Duration) workspaceAuthorityLockIdentity {
	t.Helper()
	if err := helper.ready.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatal(err)
	}
	identity, err := readWorkspaceAuthorityLockIdentity(helper.ready)
	if err != nil {
		helper.fail(t, "wait for acquired registry lock", err)
	}
	return identity
}

func (helper *workspaceAuthorityLockHelper) assertNotReadyBeforeRelease(t *testing.T) {
	t.Helper()
	if err := helper.ready.SetReadDeadline(time.Now().Add(workspaceAuthorityLockExclusionWindow)); err != nil {
		t.Fatal(err)
	}
	identity, err := readWorkspaceAuthorityLockIdentity(helper.ready)
	if err == nil {
		t.Fatalf("contending process entered registry critical section before release on lock %+v", identity)
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		helper.fail(t, "prove contending registry lock remains blocked", err)
	}
	if err := helper.ready.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func (helper *workspaceAuthorityLockHelper) releaseAndWait(t *testing.T) {
	t.Helper()
	if helper.waited {
		return
	}
	if _, err := helper.release.Write([]byte{1}); err != nil {
		helper.fail(t, "release registry lock helper", err)
	}
	if err := helper.release.Close(); err != nil {
		helper.fail(t, "close registry lock release pipe", err)
	}
	if err := helper.command.Wait(); err != nil {
		helper.fail(t, "wait for registry lock helper", err)
	}
	helper.waited = true
	helper.cancel()
}

func (helper *workspaceAuthorityLockHelper) fail(t *testing.T, operation string, err error) {
	t.Helper()
	t.Fatalf("%s: %v; helper output=%s", operation, err, helper.output.String())
}

func (helper *workspaceAuthorityLockHelper) cleanup() {
	if helper == nil {
		return
	}
	if helper.attempted != nil {
		_ = helper.attempted.Close()
	}
	if helper.ready != nil {
		_ = helper.ready.Close()
	}
	if helper.release != nil {
		_ = helper.release.Close()
	}
	if !helper.waited && helper.command != nil && helper.command.Process != nil {
		helper.cancel()
		_ = helper.command.Wait()
		helper.waited = true
	}
}

func readWorkspaceAuthorityLockIdentity(reader io.Reader) (workspaceAuthorityLockIdentity, error) {
	var payload [16]byte
	if _, err := io.ReadFull(reader, payload[:]); err != nil {
		return workspaceAuthorityLockIdentity{}, err
	}
	return workspaceAuthorityLockIdentity{
		device: binary.BigEndian.Uint64(payload[:8]),
		inode:  binary.BigEndian.Uint64(payload[8:]),
	}, nil
}

func workspaceAuthorityLockIdentityAtPath(t *testing.T, path string) workspaceAuthorityLockIdentity {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("registry lock stat type = %T, want *syscall.Stat_t", info.Sys())
	}
	return workspaceAuthorityLockIdentity{device: uint64(stat.Dev), inode: stat.Ino}
}
