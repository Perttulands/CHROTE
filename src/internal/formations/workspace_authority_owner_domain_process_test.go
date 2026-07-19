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
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	workspaceAuthorityOwnerDomainLockHelperEnv   = "CHROTE_TEST_WORKSPACE_AUTHORITY_OWNER_DOMAIN_LOCK_HELPER"
	workspaceAuthorityOwnerDomainLockPathEnv     = "CHROTE_TEST_WORKSPACE_AUTHORITY_OWNER_DOMAIN_LOCK_PATH"
	workspaceAuthorityOwnerDomainLockAttemptedFD = uintptr(3)
	workspaceAuthorityOwnerDomainLockReadyFD     = uintptr(4)
	workspaceAuthorityOwnerDomainLockReleaseFD   = uintptr(5)
)

func TestWorkspaceAuthorityOwnerDomainRetainsRegistryUntilMappedOwnerLockIsAcquired(t *testing.T) {
	fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
	paths := workspaceAuthorityOwnerDomainPaths(fixture)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)

	ownerHolder := startWorkspaceAuthorityOwnerDomainLockHelper(t, fixture.ownerLock)
	ownerHolder.waitAttempted(t)
	if got, want := ownerHolder.waitReady(t, workspaceAuthorityLockProcessTimeout), workspaceAuthorityLockIdentityAtPath(t, fixture.ownerLock); got != want {
		t.Fatalf("process holder acquired owner lock %+v, want %+v", got, want)
	}

	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
	productionValidate := registrar.ops.validatePrivateNode
	ownerSelected := make(chan struct{})
	var selectedOnce sync.Once
	registrar.ops.validatePrivateNode = func(opened *os.File, expectedUID uint32) error {
		if err := productionValidate(opened, expectedUID); err != nil {
			return err
		}
		if opened.Name() == "owner.lock" {
			selectedOnce.Do(func() { close(ownerSelected) })
		}
		return nil
	}
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCallback) }) }
	t.Cleanup(release)
	result := make(chan error, 1)
	go func() {
		result <- registrar.withWorkspaceAuthorityOwnerDomain(fixture.workspace, func(workspaceAuthorityOwnerDomainScope) error {
			close(callbackEntered)
			<-releaseCallback
			return nil
		})
	}()

	select {
	case <-ownerSelected:
	case <-time.After(workspaceAuthorityLockProcessTimeout):
		t.Fatal("owner-domain acquisition did not select the mapped owner lock")
	}
	select {
	case <-callbackEntered:
		t.Fatal("owner-domain callback entered while another process still held owner.lock")
	case <-time.After(workspaceAuthorityLockExclusionWindow):
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("registry lock while owner-domain acquisition blocked = %v, want would-block", err)
	}

	ownerHolder.releaseAndWait(t)
	select {
	case <-callbackEntered:
	case <-time.After(workspaceAuthorityLockProcessTimeout):
		t.Fatal("owner-domain callback did not enter after mapped owner lock became available")
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("registry lock after owner epoch began = %v, want released", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("owner lock after owner epoch began = %v, want would-block", err)
	}
	release()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("owner-domain acquisition after process contention: %v", err)
		}
	case <-time.After(workspaceAuthorityLockProcessTimeout):
		t.Fatal("owner-domain acquisition did not return after callback release")
	}

	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("blocked owner-domain acquisition changed authority topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
	assertWorkspaceAuthorityOwnerDomainLocksReleased(t, fixture)
}

func TestWorkspaceAuthorityOwnerDomainReleasesRegistryWhileProcessOwnerEpochRemainsExclusive(t *testing.T) {
	fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
	paths := workspaceAuthorityOwnerDomainPaths(fixture)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())

	var ownerContender *workspaceAuthorityLockHelper
	callbackCalls := 0
	err := registrar.withWorkspaceAuthorityOwnerDomain(fixture.workspace, func(scope workspaceAuthorityOwnerDomainScope) error {
		callbackCalls++
		ownerDevice, ownerInode := scope.ownerLockIdentity()
		wantOwner := workspaceAuthorityLockIdentityAtPath(t, fixture.ownerLock)
		if ownerDevice != wantOwner.device || ownerInode != wantOwner.inode {
			t.Fatalf("owner-domain process lock identity = (%d,%d), want (%d,%d)", ownerDevice, ownerInode, wantOwner.device, wantOwner.inode)
		}

		ownerContender = startWorkspaceAuthorityOwnerDomainLockHelper(t, fixture.ownerLock)
		ownerContender.waitAttempted(t)
		ownerContender.assertNotReadyBeforeRelease(t)

		registryContender := startWorkspaceAuthorityLockHelper(t, fixture.hostRoot, fixture.workspace)
		registryContender.waitAttempted(t)
		gotRegistry := registryContender.waitReady(t, workspaceAuthorityLockProcessTimeout)
		wantRegistry := workspaceAuthorityLockIdentityAtPath(t, fixture.registryLock)
		if gotRegistry != wantRegistry {
			t.Fatalf("registry contender entered on lock %+v, want %+v", gotRegistry, wantRegistry)
		}
		ownerContender.assertNotReadyBeforeRelease(t)
		registryContender.releaseAndWait(t)
		return nil
	})
	if err != nil {
		t.Fatalf("cross-process owner-domain epoch: %v", err)
	}
	if callbackCalls != 1 || ownerContender == nil {
		t.Fatalf("owner-domain callback calls/helper = %d/%v, want one/non-nil", callbackCalls, ownerContender)
	}
	gotOwner := ownerContender.waitReady(t, workspaceAuthorityLockProcessTimeout)
	wantOwner := workspaceAuthorityLockIdentityAtPath(t, fixture.ownerLock)
	if gotOwner != wantOwner {
		t.Fatalf("owner contender acquired lock %+v, want exact mapped owner %+v", gotOwner, wantOwner)
	}
	ownerContender.releaseAndWait(t)

	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("cross-process owner-domain lock proof changed authority topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
	assertWorkspaceAuthorityOwnerDomainLocksReleased(t, fixture)
}

func TestWorkspaceAuthorityOwnerDomainProcessOwnerLockHelper(t *testing.T) {
	if os.Getenv(workspaceAuthorityOwnerDomainLockHelperEnv) == "" {
		return
	}
	lockPath := os.Getenv(workspaceAuthorityOwnerDomainLockPathEnv)
	if lockPath == "" || !filepath.IsAbs(lockPath) {
		t.Fatal("workspace owner-domain lock helper requires an explicit absolute lock path")
	}
	attempted := os.NewFile(workspaceAuthorityOwnerDomainLockAttemptedFD, "workspace-owner-domain-lock-attempted")
	ready := os.NewFile(workspaceAuthorityOwnerDomainLockReadyFD, "workspace-owner-domain-lock-ready")
	if err := syscall.SetNonblock(int(workspaceAuthorityOwnerDomainLockReleaseFD), true); err != nil {
		t.Fatalf("make owner-domain release pipe pollable: %v", err)
	}
	release := os.NewFile(workspaceAuthorityOwnerDomainLockReleaseFD, "workspace-owner-domain-lock-release")
	if attempted == nil || ready == nil || release == nil {
		t.Fatal("workspace owner-domain lock helper missing synchronization pipe")
	}
	defer attempted.Close()
	defer ready.Close()
	defer release.Close()

	ownerLock, err := os.OpenFile(lockPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open owner-domain lock: %v", err)
	}
	defer ownerLock.Close()
	if err := validateAuthorityPrivateFile(ownerLock, uint32(os.Geteuid())); err != nil {
		t.Fatalf("validate owner-domain lock: %v", err)
	}
	if _, err := attempted.Write([]byte{1}); err != nil {
		t.Fatalf("signal owner-domain lock attempt: %v", err)
	}
	if err := syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("acquire owner-domain lock: %v", err)
	}
	defer syscall.Flock(int(ownerLock.Fd()), syscall.LOCK_UN) //nolint:errcheck // process exit also releases the test lock
	device, inode, err := workspaceAuthorityRegistrationFileIdentity(ownerLock)
	if err != nil {
		t.Fatalf("read owner-domain lock identity: %v", err)
	}
	var payload [16]byte
	binary.BigEndian.PutUint64(payload[:8], device)
	binary.BigEndian.PutUint64(payload[8:], inode)
	if _, err := ready.Write(payload[:]); err != nil {
		t.Fatalf("signal acquired owner-domain lock: %v", err)
	}
	if err := release.SetReadDeadline(time.Now().Add(workspaceAuthorityLockProcessTimeout)); err != nil {
		t.Fatal(err)
	}
	var signal [1]byte
	if _, err := io.ReadFull(release, signal[:]); err != nil {
		t.Fatalf("wait for owner-domain lock release signal: %v", err)
	}
}

func startWorkspaceAuthorityOwnerDomainLockHelper(t *testing.T, ownerLock string) *workspaceAuthorityLockHelper {
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
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWorkspaceAuthorityOwnerDomainProcessOwnerLockHelper$", "-test.count=1")
	command.Env = append(workspaceAuthorityOwnerDomainLockHelperBaseEnvironment(),
		workspaceAuthorityOwnerDomainLockHelperEnv+"=1",
		workspaceAuthorityOwnerDomainLockPathEnv+"="+ownerLock,
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

func workspaceAuthorityOwnerDomainLockHelperBaseEnvironment() []string {
	blocked := map[string]bool{
		workspaceAuthorityOwnerDomainLockHelperEnv: true,
		workspaceAuthorityOwnerDomainLockPathEnv:   true,
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
