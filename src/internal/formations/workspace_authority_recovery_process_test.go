package formations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
)

func TestWorkspaceAuthorityCrashRecoveryOwnerLockContentionBlocksUnderRegistryThenCompletes(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	present := installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
	stable := snapshotWorkspaceAuthorityRecoveryFiles(t, present...)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	productionOps := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate()).ops

	holder := startWorkspaceAuthorityRecoveryOwnerLockHelper(t, fixture.ownerLock)
	holder.waitReady(t)
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("process-held recovery owner lock probe = %v, want would-block", err)
	}

	generated := 0
	steps := []string{}
	callbackCalls := 0
	syncCalls := 0
	lockHeld := func(path, label string) error {
		if err := tryWorkspaceAuthorityExclusiveLock(path); !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("%s lock probe = %v, want would-block", label, err)
		}
		return nil
	}
	registryRawEquals := func(want []byte, context string) error {
		raw, err := os.ReadFile(fixture.registry)
		if err != nil {
			return fmt.Errorf("%s read registry: %w", context, err)
		}
		if !bytes.Equal(raw, want) {
			return fmt.Errorf("%s registry bytes = %s, want %s", context, raw, want)
		}
		return nil
	}
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return "", errors.New("owner-lock recovery selected the id generator")
			},
			observeInitialRegistration: func(step string) error {
				steps = append(steps, step)
				if err := lockHeld(fixture.registryLock, "recovery registry"); err != nil {
					return err
				}
				if err := lockHeld(fixture.ownerLock, "recovery owner"); err != nil {
					return err
				}
				if step == testInitialRegistrationRegistryPublished {
					return registryRawEquals(fixture.finalRegistryRaw, "registry publication observer")
				}
				return registryRawEquals(fixture.initialRegistryRaw, "pre-publication recovery observer")
			},
			syncInitialAuthorityDirectory: func(directory *os.File) error {
				syncCalls++
				if err := lockHeld(fixture.registryLock, "recovery registry at authority sync"); err != nil {
					return err
				}
				if err := lockHeld(fixture.ownerLock, "recovery owner at authority sync"); err != nil {
					return err
				}
				openedInfo, err := directory.Stat()
				if err != nil {
					return err
				}
				namedInfo, err := os.Stat(fixture.authorityDir)
				if err != nil {
					return err
				}
				if !os.SameFile(openedInfo, namedInfo) {
					return errors.New("recovery synced a directory other than the pinned selected candidate")
				}
				if err := registryRawEquals(fixture.initialRegistryRaw, "authority-directory sync"); err != nil {
					return err
				}
				return productionOps.syncInitialAuthorityDirectory(directory)
			},
		},
	)
	result := make(chan error, 1)
	go func() {
		result <- registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			if scope == nil {
				return errors.New("owner-lock recovery callback received nil scope")
			}
			gotID, matched := scope.matchedWorkspaceAuthorityID()
			if !matched || gotID != testInitialRegistrationAuthorityID {
				return fmt.Errorf("owner-lock recovery callback mapping = %q, matched=%t", gotID, matched)
			}
			if gotIdentity := scope.workspaceIdentity(); gotIdentity != fixture.identity {
				return fmt.Errorf("owner-lock recovery callback identity = %+v, want %+v", gotIdentity, fixture.identity)
			}
			if err := lockHeld(fixture.registryLock, "recovery callback registry"); err != nil {
				return err
			}
			return registryRawEquals(fixture.finalRegistryRaw, "recovery callback")
		})
	}()

	orderDeadline := time.NewTimer(workspaceAuthorityRecoveryOwnerLockProcessTimeout)
	defer orderDeadline.Stop()
	orderProbe := time.NewTicker(10 * time.Millisecond)
	defer orderProbe.Stop()
	registryHeldWhileOwnerBlocked := false
	for !registryHeldWhileOwnerBlocked {
		select {
		case registrationErr := <-result:
			holder.releaseAndWait(t)
			t.Fatalf("owner-lock recovery returned before holder release: %v", registrationErr)
		case <-orderProbe.C:
			err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock)
			switch {
			case errors.Is(err, syscall.EWOULDBLOCK):
				registryHeldWhileOwnerBlocked = true
			case err != nil:
				holder.releaseAndWait(t)
				t.Fatalf("probe owner-blocked recovery registry lock: %v", err)
			}
		case <-orderDeadline.C:
			holder.releaseAndWait(t)
			t.Fatal("owner-lock recovery did not retain registry lock while blocked on the process-held owner lock")
		}
	}
	select {
	case registrationErr := <-result:
		holder.releaseAndWait(t)
		t.Fatalf("owner-lock recovery returned while the owner lock remained process-held: %v", registrationErr)
	default:
	}

	holder.releaseAndWait(t)
	var registrationErr error
	select {
	case registrationErr = <-result:
	case <-time.After(workspaceAuthorityRecoveryOwnerLockProcessTimeout):
		t.Fatal("owner-lock recovery did not complete after holder release")
	}
	if registrationErr != nil {
		t.Fatalf("owner-lock recovery after holder release: %v", registrationErr)
	}
	if generated != 0 || callbackCalls != 1 || syncCalls != 1 {
		t.Fatalf("owner-lock recovery generator/callback/authority-sync calls = %d/%d/%d, want 0/1/1", generated, callbackCalls, syncCalls)
	}
	assertWorkspaceAuthorityRecoveryStepOrder(t, workspaceAuthorityRecoveryOwnerLock, steps)
	assertWorkspaceAuthorityRecoveryFilesStable(t, stable)
	assertWorkspaceAuthorityRecoveryFinalState(t, fixture)
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("owner-lock recovery leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("owner-lock recovery leaked owner lock after holder exit: %v", err)
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
