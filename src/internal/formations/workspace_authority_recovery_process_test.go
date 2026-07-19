package formations

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	workspaceAuthorityRecoveryOwnerLockHelperEnv      = "CHROTE_TEST_WORKSPACE_AUTHORITY_RECOVERY_OWNER_LOCK_HELPER"
	workspaceAuthorityRecoveryOwnerLockPathEnv        = "CHROTE_TEST_WORKSPACE_AUTHORITY_RECOVERY_OWNER_LOCK_PATH"
	workspaceAuthorityRecoveryOwnerLockReadyFD        = uintptr(3)
	workspaceAuthorityRecoveryOwnerLockReleaseFD      = uintptr(4)
	workspaceAuthorityRecoveryOwnerLockProcessTimeout = 10 * time.Second
	workspaceAuthorityRecoveryOwnerLockReturnWindow   = time.Second
)

func TestWorkspaceAuthorityCrashRecoveryOwnerLockContentionFailsWithoutRepair(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)

	holder := startWorkspaceAuthorityRecoveryOwnerLockHelper(t, fixture.ownerLock)
	holder.waitReady(t)
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("process-held recovery owner lock probe = %v, want would-block", err)
	}

	generated := 0
	observed := 0
	callbackCalls := 0
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return testInitialRegistrationAuthorityID, nil
			},
			observeInitialRegistration: func(string) error {
				observed++
				return nil
			},
		},
	)
	result := make(chan error, 1)
	go func() {
		result <- registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			return nil
		})
	}()

	var registrationErr error
	returnedBeforeRelease := false
	select {
	case registrationErr = <-result:
		returnedBeforeRelease = true
	case <-time.After(workspaceAuthorityRecoveryOwnerLockReturnWindow):
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(err, syscall.EWOULDBLOCK) {
			t.Errorf("owner-lock-blocked recovery registry lock probe = %v, want would-block under registry->owner order", err)
		}
	}
	holder.releaseAndWait(t)
	if !returnedBeforeRelease {
		select {
		case registrationErr = <-result:
		case <-time.After(workspaceAuthorityRecoveryOwnerLockProcessTimeout):
			t.Fatal("owner-lock-contended recovery did not return after holder release")
		}
		t.Fatal("owner-lock-contended recovery blocked instead of failing loud before holder release")
	}
	if registrationErr == nil {
		t.Fatal("owner-lock-contended recovery reported success")
	}
	if generated != 0 || observed != 0 || callbackCalls != 0 {
		t.Fatalf("owner-lock-contention generator/initial-step/callback calls = %d/%d/%d, want 0/0/0", generated, observed, callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("owner-lock contention changed registry predecessor or selected candidate\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("owner-lock contention leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("owner-lock contention leaked owner lock after holder exit: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func TestWorkspaceAuthorityRecoveryOwnerLockProcessHelper(t *testing.T) {
	if os.Getenv(workspaceAuthorityRecoveryOwnerLockHelperEnv) == "" {
		return
	}
	lockPath := os.Getenv(workspaceAuthorityRecoveryOwnerLockPathEnv)
	if lockPath == "" || !filepath.IsAbs(lockPath) {
		t.Fatal("workspace-authority recovery owner-lock helper requires an explicit absolute lock path")
	}
	ready := os.NewFile(workspaceAuthorityRecoveryOwnerLockReadyFD, "workspace-authority-recovery-owner-lock-ready")
	if err := syscall.SetNonblock(int(workspaceAuthorityRecoveryOwnerLockReleaseFD), true); err != nil {
		t.Fatalf("make recovery owner-lock release pipe pollable: %v", err)
	}
	release := os.NewFile(workspaceAuthorityRecoveryOwnerLockReleaseFD, "workspace-authority-recovery-owner-lock-release")
	if ready == nil || release == nil {
		t.Fatal("workspace-authority recovery owner-lock helper missing synchronization pipe")
	}
	defer ready.Close()
	defer release.Close()

	lock, err := os.OpenFile(lockPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck // process exit also releases the helper lock
	if _, err := ready.Write([]byte{1}); err != nil {
		t.Fatalf("signal recovery owner lock acquired: %v", err)
	}
	if err := release.SetReadDeadline(time.Now().Add(workspaceAuthorityRecoveryOwnerLockProcessTimeout)); err != nil {
		t.Fatal(err)
	}
	var signal [1]byte
	if _, err := io.ReadFull(release, signal[:]); err != nil {
		t.Fatalf("wait for recovery owner-lock release: %v", err)
	}
}

type workspaceAuthorityRecoveryOwnerLockHelper struct {
	command *exec.Cmd
	cancel  context.CancelFunc
	ready   *os.File
	release *os.File
	output  *bytes.Buffer
	waited  bool
}

func startWorkspaceAuthorityRecoveryOwnerLockHelper(t *testing.T, lockPath string) *workspaceAuthorityRecoveryOwnerLockHelper {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	releaseReader, releaseWriter, err := os.Pipe()
	if err != nil {
		readyReader.Close()
		readyWriter.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspaceAuthorityRecoveryOwnerLockProcessTimeout)
	output := &bytes.Buffer{}
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWorkspaceAuthorityRecoveryOwnerLockProcessHelper$", "-test.count=1")
	command.Env = append(workspaceAuthorityRecoveryOwnerLockHelperBaseEnvironment(),
		workspaceAuthorityRecoveryOwnerLockHelperEnv+"=1",
		workspaceAuthorityRecoveryOwnerLockPathEnv+"="+lockPath,
	)
	command.ExtraFiles = []*os.File{readyWriter, releaseReader}
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		cancel()
		readyReader.Close()
		readyWriter.Close()
		releaseReader.Close()
		releaseWriter.Close()
		t.Fatal(err)
	}
	readyWriter.Close()
	releaseReader.Close()
	helper := &workspaceAuthorityRecoveryOwnerLockHelper{
		command: command,
		cancel:  cancel,
		ready:   readyReader,
		release: releaseWriter,
		output:  output,
	}
	t.Cleanup(func() { helper.cleanup() })
	return helper
}

func workspaceAuthorityRecoveryOwnerLockHelperBaseEnvironment() []string {
	blocked := map[string]bool{
		workspaceAuthorityRecoveryOwnerLockHelperEnv: true,
		workspaceAuthorityRecoveryOwnerLockPathEnv:   true,
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			environment = append(environment, entry)
		}
	}
	return environment
}

func (helper *workspaceAuthorityRecoveryOwnerLockHelper) waitReady(t *testing.T) {
	t.Helper()
	if err := helper.ready.SetReadDeadline(time.Now().Add(workspaceAuthorityRecoveryOwnerLockProcessTimeout)); err != nil {
		t.Fatal(err)
	}
	var signal [1]byte
	if _, err := io.ReadFull(helper.ready, signal[:]); err != nil {
		helper.fail(t, "wait for recovery owner lock", err)
	}
}

func (helper *workspaceAuthorityRecoveryOwnerLockHelper) releaseAndWait(t *testing.T) {
	t.Helper()
	if helper.waited {
		return
	}
	if _, err := helper.release.Write([]byte{1}); err != nil {
		helper.fail(t, "release recovery owner lock", err)
	}
	if err := helper.release.Close(); err != nil {
		helper.fail(t, "close recovery owner-lock release pipe", err)
	}
	helper.release = nil
	if err := helper.command.Wait(); err != nil {
		helper.fail(t, "wait for recovery owner-lock helper", err)
	}
	helper.waited = true
	helper.cancel()
	if err := helper.ready.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		helper.fail(t, "close recovery owner-lock ready pipe", err)
	}
	helper.ready = nil
}

func (helper *workspaceAuthorityRecoveryOwnerLockHelper) fail(t *testing.T, operation string, err error) {
	t.Helper()
	t.Fatalf("%s: %v; helper output=%s", operation, err, helper.output.String())
}

func (helper *workspaceAuthorityRecoveryOwnerLockHelper) cleanup() {
	if helper == nil {
		return
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
