package formations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestWorkspaceAuthorityRegistrationIdentityUsesOneOpenedDirectory(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	configuredWorkspace := filepath.Join(fixture.base, "configured-workspace")
	if err := os.Symlink(fixture.workspace, configuredWorkspace); err != nil {
		t.Fatal(err)
	}
	wantIdentity := testWorkspaceAuthorityIdentityAtPath(t, configuredWorkspace)
	openDescriptorsBefore := countWorkspaceAuthorityDescriptors(t, fixture.workspace)
	openCalls := 0
	var openedDescriptor uintptr
	openWorkspace := func(configuredPath string) (*os.File, error) {
		openCalls++
		if got, want := configuredPath, filepath.ToSlash(filepath.Clean(configuredWorkspace)); got != want {
			return nil, fmt.Errorf("workspace opener path = %q, want cleaned configured path %q", got, want)
		}
		workspace, err := openWorkspaceAuthorityTestDirectory(configuredPath)
		if err == nil {
			openedDescriptor = workspace.Fd()
		}
		return workspace, err
	}
	registrar := newWorkspaceAuthorityRegistrar(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		openWorkspace,
		nil,
	)

	inspection, err := registrar.inspect(configuredWorkspace)
	if err != nil {
		t.Fatalf("inspect unregistered workspace identity: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = inspection.close()
		}
	})
	if openCalls != 1 {
		t.Fatalf("workspace directory opener calls = %d, want exactly one", openCalls)
	}
	if got, want := countWorkspaceAuthorityDescriptors(t, fixture.workspace), openDescriptorsBefore+1; got != want {
		t.Fatalf("retained workspace descriptors = %d, want %d after one injected open", got, want)
	}
	inspection.observePinnedWorkspace(func(workspace *os.File, identity runtimeWorkspaceIdentity) {
		if workspace.Fd() != openedDescriptor {
			t.Fatalf("retained workspace descriptor = %d, want injected descriptor %d", workspace.Fd(), openedDescriptor)
		}
		workspaceInfo, statErr := workspace.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if !os.SameFile(workspaceInfo, wantIdentity.info) {
			t.Fatalf("retained workspace descriptor identifies %v, want pre-open identity %v", workspaceInfo, wantIdentity.info)
		}
		if identity.configuredPath != wantIdentity.identity.configuredPath ||
			identity.resolvedPath != wantIdentity.identity.resolvedPath ||
			identity.device != wantIdentity.identity.device ||
			identity.inode != wantIdentity.identity.inode ||
			identity.rootHash != wantIdentity.identity.rootHash {
			t.Fatalf("opened workspace identity = %+v, want independent pre-open identity %+v", identity, wantIdentity.identity)
		}
	})
	if authorityID, matched := inspection.matchedWorkspaceAuthorityID(); matched {
		t.Fatalf("unregistered workspace matched registry authority %q", authorityID)
	}
	if err := inspection.validatePinnedPaths(); err != nil {
		t.Fatalf("freshly opened workspace/root binding rejected: %v", err)
	}
	if err := inspection.close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	if got := countWorkspaceAuthorityDescriptors(t, fixture.workspace); got != openDescriptorsBefore {
		t.Fatalf("workspace descriptors after inspection close = %d, want original %d", got, openDescriptorsBefore)
	}
}

func TestWorkspaceAuthorityRegistrationDerivesIdentityFromOpenedDirectoryBeforeRetargetFence(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	configuredWorkspace := filepath.Join(fixture.base, "configured-workspace")
	if err := os.Symlink(fixture.workspace, configuredWorkspace); err != nil {
		t.Fatal(err)
	}
	replacementWorkspace := filepath.Join(fixture.base, "replacement-workspace")
	if err := os.Mkdir(replacementWorkspace, 0o700); err != nil {
		t.Fatal(err)
	}
	wantIdentity := testWorkspaceAuthorityIdentityAtPath(t, configuredWorkspace)
	openDescriptorsBefore := countWorkspaceAuthorityDescriptors(t, fixture.workspace)
	openCalls := 0
	var openedDescriptor uintptr
	openWorkspace := func(configuredPath string) (*os.File, error) {
		openCalls++
		workspace, err := openWorkspaceAuthorityTestDirectory(configuredPath)
		if err != nil {
			return nil, err
		}
		openedDescriptor = workspace.Fd()
		if err := os.Remove(configuredPath); err != nil {
			workspace.Close()
			return nil, err
		}
		if err := os.Symlink(replacementWorkspace, configuredPath); err != nil {
			workspace.Close()
			return nil, err
		}
		return workspace, nil
	}
	registrar := newWorkspaceAuthorityRegistrar(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		openWorkspace,
		nil,
	)
	inspection, err := registrar.inspect(configuredWorkspace)
	if err != nil {
		t.Fatalf("inspect identity across controlled configured-path retarget: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = inspection.close()
		}
	})
	if openCalls != 1 {
		t.Fatalf("workspace directory opener calls across retarget = %d, want exactly one", openCalls)
	}
	inspection.observePinnedWorkspace(func(workspace *os.File, identity runtimeWorkspaceIdentity) {
		if workspace.Fd() != openedDescriptor {
			t.Fatalf("retarget inspection retained descriptor %d, want injected descriptor %d", workspace.Fd(), openedDescriptor)
		}
		if identity.configuredPath != wantIdentity.identity.configuredPath ||
			identity.resolvedPath != wantIdentity.identity.resolvedPath ||
			identity.device != wantIdentity.identity.device ||
			identity.inode != wantIdentity.identity.inode ||
			identity.rootHash != wantIdentity.identity.rootHash {
			t.Fatalf("retarget inspection identity = %+v, want sole pre-retarget opened identity %+v", identity, wantIdentity.identity)
		}
	})
	if got, want := countWorkspaceAuthorityDescriptors(t, fixture.workspace), openDescriptorsBefore+1; got != want {
		t.Fatalf("retarget inspection descriptors for original workspace = %d, want %d", got, want)
	}
	replacementInfo, err := os.Stat(configuredWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(replacementInfo, wantIdentity.info) {
		t.Fatal("controlled opener did not retarget configured workspace before returning")
	}
	beforeRejection := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	if err := inspection.validatePinnedPaths(); !errors.Is(err, errRuntimeIntegrityMismatch) {
		t.Fatalf("controlled retarget validation error = %v, want integrity mismatch", err)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, beforeRejection) {
		t.Fatalf("controlled retarget rejection changed topology\nbefore: %#v\nafter:  %#v", beforeRejection, after)
	}
	if err := inspection.close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	if got := countWorkspaceAuthorityDescriptors(t, fixture.workspace); got != openDescriptorsBefore {
		t.Fatalf("original workspace descriptors after retarget inspection close = %d, want %d", got, openDescriptorsBefore)
	}
}

func TestWorkspaceRootIdentityV1UsesCanonicalStringsBeyondJSONSafeInteger(t *testing.T) {
	separator := "\u2028"
	identity := runtimeWorkspaceIdentity{
		configuredPath: "/tmp/quo\"te&snow-雪" + separator + "\x01",
		resolvedPath:   "/real/quo\"te&snow-雪" + separator + "\x01",
		device:         math.MaxUint64,
		inode:          9007199254740992,
	}
	want := []byte(`{"configuredPath":"/tmp/quo\"te&snow-雪` + separator + `\u0001","device":"18446744073709551615","inode":"9007199254740992","resolvedPath":"/real/quo\"te&snow-雪` + separator + `\u0001"}`)

	wantHash := testWorkspaceAuthoritySHA256(want)
	if hash := runtimeWorkspaceIdentityHash(identity); hash != wantHash {
		t.Fatalf("workspace-root-identity-v1 hash = %q, want standard-library SHA-256 of exact JCS bytes %q", hash, wantHash)
	}
}

func TestWorkspaceAuthorityRegistrationCleansConfiguredSpellingAndRejectsInvalidPathGrammar(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)

	noise := filepath.Join(filepath.Dir(fixture.workspace), "noise")
	if err := os.Mkdir(noise, 0o700); err != nil {
		t.Fatal(err)
	}
	unclean := filepath.Join(noise, "..", filepath.Base(fixture.workspace))
	inspection, err := registrar.inspect(unclean)
	if err != nil {
		t.Fatalf("inspect cleaned configured spelling: %v", err)
	}
	var configuredPath string
	inspection.observePinnedWorkspace(func(_ *os.File, identity runtimeWorkspaceIdentity) {
		configuredPath = identity.configuredPath
	})
	if got, want := configuredPath, filepath.ToSlash(filepath.Clean(unclean)); got != want {
		closeWorkspaceAuthorityInspection(t, inspection)
		t.Fatalf("cleaned configured spelling = %q, want %q", got, want)
	}
	closeWorkspaceAuthorityInspection(t, inspection)

	for _, configured := range []string{
		"relative/workspace",
		fixture.workspace + "\x00suffix",
		fixture.workspace + `\alias`,
		fixture.workspace + string([]byte{0xff}),
	} {
		t.Run(strconv.Quote(configured), func(t *testing.T) {
			before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
			inspection, err := registrar.inspect(configured)
			if inspection != nil {
				closeWorkspaceAuthorityInspection(t, inspection)
			}
			if err == nil || !errors.Is(err, errRuntimeNoncanonical) {
				t.Fatalf("invalid configured path error = %v, want noncanonical", err)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid configured path changed authority topology\nbefore: %#v\nafter:  %#v", before, after)
			}
		})
	}
}

func TestWorkspaceAuthorityRegistrationRejectsRetargetedConfiguredWorkspaceBeforeMutation(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	configured := filepath.Join(fixture.base, "configured-workspace")
	if err := os.Symlink(fixture.workspace, configured); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(fixture.base, "replacement-workspace")
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
	inspection, err := registrar.inspect(configured)
	if err != nil {
		t.Fatal(err)
	}
	defer closeWorkspaceAuthorityInspection(t, inspection)
	var originalInfo os.FileInfo
	inspection.observePinnedWorkspace(func(workspace *os.File, _ runtimeWorkspaceIdentity) {
		originalInfo, err = workspace.Stat()
		if err != nil {
			t.Fatal(err)
		}
	})

	if err := os.Remove(configured); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, configured); err != nil {
		t.Fatal(err)
	}
	beforeRejection := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	if err := inspection.validatePinnedPaths(); !errors.Is(err, errRuntimeIntegrityMismatch) {
		t.Fatalf("retargeted configured workspace error = %v, want integrity mismatch", err)
	}
	var openedInfo os.FileInfo
	inspection.observePinnedWorkspace(func(workspace *os.File, _ runtimeWorkspaceIdentity) {
		openedInfo, err = workspace.Stat()
		if err != nil {
			t.Fatal(err)
		}
	})
	if !os.SameFile(originalInfo, openedInfo) {
		t.Fatal("pinned workspace descriptor changed after configured symlink retarget")
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, beforeRejection) {
		t.Fatalf("retarget rejection changed authority topology\nbefore: %#v\nafter:  %#v", beforeRejection, after)
	}
}

func TestWorkspaceAuthorityRegistrationRequiresPrivatePinnedRootAndRegistryLock(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*testing.T, *workspaceAuthorityRegistrationFixture) []string
		expectedUID func(uint32) uint32
	}{
		{
			name: "host root wrong mode",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.hostRoot, 0o750); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "host root special bits",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.hostRoot, os.ModeSetgid|0o700); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "workspaces root wrong mode",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.workspacesRoot, 0o750); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "workspaces root special bits",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.workspacesRoot, os.ModeSetgid|0o700); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "workspaces root symlink",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				victim := filepath.Join(fixture.base, "workspaces-root-victim")
				if err := os.Rename(fixture.workspacesRoot, victim); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(victim, fixture.workspacesRoot); err != nil {
					t.Fatal(err)
				}
				return []string{victim}
			},
		},
		{
			name: "workspaces root regular file",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				victim := filepath.Join(fixture.base, "workspaces-root-directory")
				if err := os.Rename(fixture.workspacesRoot, victim); err != nil {
					t.Fatal(err)
				}
				writePrivateAuthorityTestFile(t, fixture.workspacesRoot, []byte("not-a-directory"))
				return []string{victim}
			},
		},
		{
			name: "registry lock wrong mode",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.registryLock, 0o640); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "registry lock special bits",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.registryLock, os.ModeSetgid|0o600); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "registry lock hard link",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				escaped := filepath.Join(fixture.workspace, "escaped-registry-lock")
				if err := os.Link(fixture.registryLock, escaped); err != nil {
					t.Fatal(err)
				}
				return []string{fixture.workspace}
			},
		},
		{
			name: "registry lock symlink",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				victim := filepath.Join(fixture.base, "registry-lock-victim")
				if err := os.Rename(fixture.registryLock, victim); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(victim, fixture.registryLock); err != nil {
					t.Fatal(err)
				}
				return []string{victim}
			},
		},
		{
			name: "registry lock directory",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Remove(fixture.registryLock); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(fixture.registryLock, 0o700); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "registry lock fifo",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Remove(fixture.registryLock); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(fixture.registryLock, 0o600); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "registry private wrong mode",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.registry, 0o640); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "registry private special bits",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Chmod(fixture.registry, os.ModeSetgid|0o600); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "registry private hard link",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				escaped := filepath.Join(fixture.workspace, "escaped-registry-private")
				if err := os.Link(fixture.registry, escaped); err != nil {
					t.Fatal(err)
				}
				return []string{fixture.workspace}
			},
		},
		{
			name: "registry private symlink",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				victim := filepath.Join(fixture.base, "registry-private-victim")
				if err := os.Rename(fixture.registry, victim); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(victim, fixture.registry); err != nil {
					t.Fatal(err)
				}
				return []string{victim}
			},
		},
		{
			name: "registry private directory",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Remove(fixture.registry); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(fixture.registry, 0o700); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name: "registry private fifo",
			mutate: func(t *testing.T, fixture *workspaceAuthorityRegistrationFixture) []string {
				if err := os.Remove(fixture.registry); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(fixture.registry, 0o600); err != nil {
					t.Fatal(err)
				}
				return nil
			},
		},
		{
			name:   "all private nodes wrong expected uid",
			mutate: func(*testing.T, *workspaceAuthorityRegistrationFixture) []string { return nil },
			expectedUID: func(owner uint32) uint32 {
				return owner ^ 1
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
				Entries:        []workspaceRegistryEntryJCSV1{},
				RecordRev:      1,
				RegistrySchema: 1,
			})
			extraRoots := test.mutate(t, &fixture)
			expectedUID := fixture.ownerUID
			if test.expectedUID != nil {
				expectedUID = test.expectedUID(expectedUID)
			}
			roots := append([]string{fixture.base}, extraRoots...)
			before := snapshotWorkspaceAuthorityTopology(t, roots...)

			registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, expectedUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
			inspection, err := registrar.inspect(fixture.workspace)
			if inspection != nil {
				closeWorkspaceAuthorityInspection(t, inspection)
			}
			if err == nil || !errors.Is(err, errRuntimeIntegrityMismatch) {
				t.Fatalf("unsafe private root/lock error = %v, want integrity mismatch", err)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, roots...); !reflect.DeepEqual(after, before) {
				t.Fatalf("private root/lock rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
			}
		})
	}
}

func TestWorkspaceAuthorityRegistrationRoutesExpectedWriterUIDThroughEveryPrivateNode(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	components := []struct {
		name string
		path string
	}{
		{name: "host root", path: fixture.hostRoot},
		{name: "workspaces root", path: fixture.workspacesRoot},
		{name: "registry lock", path: fixture.registryLock},
		{name: "registry private", path: fixture.registry},
	}

	for _, component := range components {
		t.Run(component.name, func(t *testing.T) {
			wantInfo, err := os.Lstat(component.path)
			if err != nil {
				t.Fatal(err)
			}
			before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
			validatedTarget := 0
			validatePrivateNode := func(opened *os.File, expectedUID uint32) error {
				if expectedUID != fixture.ownerUID {
					return fmt.Errorf("private-node validator received uid %d, want dedicated writer uid %d", expectedUID, fixture.ownerUID)
				}
				info, statErr := opened.Stat()
				if statErr != nil {
					return statErr
				}
				if os.SameFile(info, wantInfo) {
					validatedTarget++
					return fmt.Errorf("%w: injected %s uid mismatch", errRuntimeIntegrityMismatch, component.name)
				}
				return nil
			}
			registrar := newWorkspaceAuthorityRegistrar(
				fixture.hostRoot,
				fixture.ownerUID,
				newWorkspaceAuthorityCapabilityGate(),
				nil,
				validatePrivateNode,
			)
			inspection, err := registrar.inspect(fixture.workspace)
			if inspection != nil {
				closeWorkspaceAuthorityInspection(t, inspection)
			}
			if !errors.Is(err, errRuntimeIntegrityMismatch) {
				t.Fatalf("injected %s uid mismatch error = %v, want integrity mismatch", component.name, err)
			}
			if validatedTarget != 1 {
				t.Fatalf("%s expected-writer-uid validation calls = %d, want exactly one", component.name, validatedTarget)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
				t.Fatalf("%s uid rejection changed topology\nbefore: %#v\nafter:  %#v", component.name, before, after)
			}
		})
	}
}

func TestWorkspaceAuthorityRegistrationRejectsSymlinkedOrRenamedHostRoot(t *testing.T) {
	t.Run("non-directory host root", func(t *testing.T) {
		fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
			Entries:        []workspaceRegistryEntryJCSV1{},
			RecordRev:      1,
			RegistrySchema: 1,
		})
		realRoot := fixture.hostRoot + ".directory"
		if err := os.Rename(fixture.hostRoot, realRoot); err != nil {
			t.Fatal(err)
		}
		writePrivateAuthorityTestFile(t, fixture.hostRoot, []byte("not-a-directory"))
		before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
		inspection, err := registrar.inspect(fixture.workspace)
		if inspection != nil {
			closeWorkspaceAuthorityInspection(t, inspection)
		}
		if err == nil {
			t.Fatal("registration accepted a non-directory host root")
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
			t.Fatalf("host-root type rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
		}
	})

	t.Run("symlinked host root", func(t *testing.T) {
		fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
			Entries:        []workspaceRegistryEntryJCSV1{},
			RecordRev:      1,
			RegistrySchema: 1,
		})
		realRoot := fixture.hostRoot + ".real"
		if err := os.Rename(fixture.hostRoot, realRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realRoot, fixture.hostRoot); err != nil {
			t.Fatal(err)
		}
		before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
		inspection, err := registrar.inspect(fixture.workspace)
		if inspection != nil {
			closeWorkspaceAuthorityInspection(t, inspection)
		}
		if err == nil {
			t.Fatal("registration followed a symlinked host root")
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
			t.Fatalf("host-root symlink rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
		}
	})

	t.Run("symlinked ancestor", func(t *testing.T) {
		fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
			Entries:        []workspaceRegistryEntryJCSV1{},
			RecordRev:      1,
			RegistrySchema: 1,
		})
		ancestor := filepath.Dir(fixture.hostRoot)
		realAncestor := ancestor + ".real"
		if err := os.Rename(ancestor, realAncestor); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realAncestor, ancestor); err != nil {
			t.Fatal(err)
		}
		before := snapshotWorkspaceAuthorityTopology(t, ancestor, realAncestor)
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
		inspection, err := registrar.inspect(fixture.workspace)
		if inspection != nil {
			closeWorkspaceAuthorityInspection(t, inspection)
		}
		if err == nil {
			t.Fatal("registration followed a symlinked host-root ancestor")
		}
		if after := snapshotWorkspaceAuthorityTopology(t, ancestor, realAncestor); !reflect.DeepEqual(after, before) {
			t.Fatalf("ancestor-symlink rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
		}
	})

	t.Run("opened root renamed and replaced", func(t *testing.T) {
		fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
			Entries:        []workspaceRegistryEntryJCSV1{},
			RecordRev:      1,
			RegistrySchema: 1,
		})
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
		inspection, err := registrar.inspect(fixture.workspace)
		if err != nil {
			t.Fatal(err)
		}
		defer closeWorkspaceAuthorityInspection(t, inspection)

		movedRoot := fixture.hostRoot + ".opened"
		if err := os.Rename(fixture.hostRoot, movedRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(fixture.hostRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		before := snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedRoot)
		if err := inspection.validatePinnedPaths(); !errors.Is(err, errRuntimeIntegrityMismatch) {
			t.Fatalf("renamed host root error = %v, want integrity mismatch", err)
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedRoot); !reflect.DeepEqual(after, before) {
			t.Fatalf("renamed-root rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
		}
	})
}

func TestWorkspaceAuthorityRegistrationRejectsReplacedWorkspacesRoot(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
	inspection, err := registrar.inspect(fixture.workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer closeWorkspaceAuthorityInspection(t, inspection)

	movedWorkspaces := fixture.workspacesRoot + ".opened"
	if err := os.Rename(fixture.workspacesRoot, movedWorkspaces); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.workspacesRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writePrivateAuthorityTestFile(t, fixture.registryLock, nil)
	replacementRegistry, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	writePrivateAuthorityTestFile(t, fixture.registry, replacementRegistry)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedWorkspaces)
	if err := inspection.validatePinnedPaths(); !errors.Is(err, errRuntimeIntegrityMismatch) {
		t.Fatalf("replaced workspaces root error = %v, want integrity mismatch", err)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedWorkspaces); !reflect.DeepEqual(after, before) {
		t.Fatalf("workspaces-root replacement rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func TestWorkspaceAuthorityRegistrationRejectsAuthorityWorkspaceOverlap(t *testing.T) {
	tests := []struct {
		name         string
		paths        func(*testing.T) (string, string)
		afterPrepare func(*testing.T, string, string)
	}{
		{
			name: "authority root inside workspace",
			paths: func(t *testing.T) (string, string) {
				workspace := t.TempDir()
				hostRoot := filepath.Join(workspace, ".host-private-formations")
				return hostRoot, workspace
			},
		},
		{
			name: "workspace inside authority root",
			paths: func(t *testing.T) (string, string) {
				base := t.TempDir()
				hostRoot := filepath.Join(base, "authority")
				return hostRoot, filepath.Join(hostRoot, "workspace")
			},
			afterPrepare: func(t *testing.T, _, workspace string) {
				if err := os.Mkdir(workspace, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "workspace configured alias resolves inside authority root",
			paths: func(t *testing.T) (string, string) {
				base := t.TempDir()
				return filepath.Join(base, "authority"), filepath.Join(base, "configured-workspace")
			},
			afterPrepare: func(t *testing.T, hostRoot, workspace string) {
				resolvedWorkspace := filepath.Join(hostRoot, "resolved-workspace")
				if err := os.Mkdir(resolvedWorkspace, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(resolvedWorkspace, workspace); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "authority root is inside resolved workspace",
			paths: func(t *testing.T) (string, string) {
				base := t.TempDir()
				resolvedWorkspace := filepath.Join(base, "resolved-workspace")
				if err := os.Mkdir(resolvedWorkspace, 0o700); err != nil {
					t.Fatal(err)
				}
				workspace := filepath.Join(base, "configured-workspace")
				if err := os.Symlink(resolvedWorkspace, workspace); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(resolvedWorkspace, "authority"), workspace
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hostRoot, workspace := test.paths(t)
			prepareWorkspaceAuthorityRegistrationRoot(t, hostRoot, workspaceRegistryJCSV1{
				Entries:        []workspaceRegistryEntryJCSV1{},
				RecordRev:      1,
				RegistrySchema: 1,
			})
			if test.afterPrepare != nil {
				test.afterPrepare(t, hostRoot, workspace)
			}
			before := snapshotWorkspaceAuthorityTopology(t, hostRoot, workspace)
			registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate(), nil, nil)
			inspection, err := registrar.inspect(workspace)
			if inspection != nil {
				closeWorkspaceAuthorityInspection(t, inspection)
			}
			if !errors.Is(err, errRuntimeConflict) {
				t.Fatalf("overlapping authority/workspace error = %v, want conflict", err)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, hostRoot, workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("overlap rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
			}
		})
	}
}

func TestWorkspaceAuthorityCapabilityRejectsBeforeRegistryLockSelection(t *testing.T) {
	base := t.TempDir()
	hostRoot := filepath.Join(base, "authority")
	workspacesRoot := filepath.Join(hostRoot, "workspaces")
	for _, path := range []string{hostRoot, workspacesRoot} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := filepath.Join(base, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	registryRaw, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	writePrivateAuthorityTestFile(t, filepath.Join(workspacesRoot, "registry.private.json"), registryRaw)
	before := snapshotWorkspaceAuthorityTopology(t, base)

	invalidGate := workspaceAuthorityCapabilityGate{capabilities: []workspaceAuthorityCapability{{id: RuntimeAuthorityGuardCapabilityV1}}}
	registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), invalidGate, nil, nil)
	inspection, err := registrar.inspect(workspace)
	if inspection != nil {
		closeWorkspaceAuthorityInspection(t, inspection)
	}
	if !errors.Is(err, errRuntimeUnsupportedSchema) {
		t.Fatalf("invalid capability error = %v, want unsupported schema before missing lock", err)
	}
	if _, statErr := os.Lstat(filepath.Join(workspacesRoot, "registry.lock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid capability selected or created registry lock: %v", statErr)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, base); !reflect.DeepEqual(after, before) {
		t.Fatalf("capability rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func TestWorkspaceAuthorityRegistryLookupClassifiesIdentityWithoutMutation(t *testing.T) {
	tests := []struct {
		name      string
		registry  func(runtimeWorkspaceIdentity) workspaceRegistryJCSV1
		mutateRaw func([]byte) []byte
		wantEntry bool
		wantError error
	}{
		{
			name: "exact configured and opened identity",
			registry: func(identity runtimeWorkspaceIdentity) workspaceRegistryJCSV1 {
				return workspaceRegistryWithIdentity(identity, testWorkspaceAuthorityID)
			},
			wantEntry: true,
		},
		{
			name: "unregistered configured and opened identity",
			registry: func(runtimeWorkspaceIdentity) workspaceRegistryJCSV1 {
				return workspaceRegistryJCSV1{Entries: []workspaceRegistryEntryJCSV1{}, RecordRev: 1, RegistrySchema: 1}
			},
		},
		{
			name: "configured spelling changed target",
			registry: func(identity runtimeWorkspaceIdentity) workspaceRegistryJCSV1 {
				record := workspaceRegistryWithIdentity(identity, testWorkspaceAuthorityID)
				record.Entries[0].Inode = strconv.FormatUint(identity.inode+1, 10)
				return record
			},
			wantError: errRuntimeIntegrityMismatch,
		},
		{
			name: "different alias names opened identity",
			registry: func(identity runtimeWorkspaceIdentity) workspaceRegistryJCSV1 {
				record := workspaceRegistryWithIdentity(identity, testWorkspaceAuthorityID)
				record.Entries[0].ConfiguredPath = identity.configuredPath + "-registered-alias"
				return record
			},
			wantError: errRuntimeConflict,
		},
		{
			name: "noncanonical registry bytes",
			registry: func(identity runtimeWorkspaceIdentity) workspaceRegistryJCSV1 {
				return workspaceRegistryWithIdentity(identity, testWorkspaceAuthorityID)
			},
			mutateRaw: func(raw []byte) []byte {
				return append(raw, '\n')
			},
			wantError: errRuntimeNoncanonical,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			workspace := filepath.Join(base, "workspace")
			if err := os.Mkdir(workspace, 0o700); err != nil {
				t.Fatal(err)
			}
			identity := testWorkspaceAuthorityIdentityAtPath(t, workspace).identity
			record := test.registry(identity)
			raw, err := encodeWorkspaceRegistryJCSV1(record)
			if err != nil {
				t.Fatal(err)
			}
			if test.mutateRaw != nil {
				raw = test.mutateRaw(raw)
			}
			hostRoot := filepath.Join(base, "authority")
			workspacesRoot, _ := prepareWorkspaceAuthorityRegistrationRoot(t, hostRoot, workspaceRegistryJCSV1{
				Entries:        []workspaceRegistryEntryJCSV1{},
				RecordRev:      1,
				RegistrySchema: 1,
			})
			writePrivateAuthorityTestFile(t, filepath.Join(workspacesRoot, "registry.private.json"), raw)
			before := snapshotWorkspaceAuthorityTopology(t, base)

			registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate(), nil, nil)
			inspection, err := registrar.inspect(workspace)
			if test.wantError != nil {
				if inspection != nil {
					closeWorkspaceAuthorityInspection(t, inspection)
				}
				if !errors.Is(err, test.wantError) {
					t.Fatalf("registry lookup error = %v, want %v", err, test.wantError)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				authorityID, matched := inspection.matchedWorkspaceAuthorityID()
				if matched != test.wantEntry {
					closeWorkspaceAuthorityInspection(t, inspection)
					t.Fatalf("registry match authority id = %q, present=%t, want present=%t", authorityID, matched, test.wantEntry)
				}
				if test.wantEntry && authorityID != testWorkspaceAuthorityID {
					closeWorkspaceAuthorityInspection(t, inspection)
					t.Fatalf("matched workspace authority id = %q, want %q", authorityID, testWorkspaceAuthorityID)
				}
				closeWorkspaceAuthorityInspection(t, inspection)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, base); !reflect.DeepEqual(after, before) {
				t.Fatalf("registry lookup changed topology\nbefore: %#v\nafter:  %#v", before, after)
			}
		})
	}
}

func TestWorkspaceAuthorityRegistryCriticalSectionSerializesLocally(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), nil, nil)
	first, err := registrar.inspect(fixture.workspace)
	if err != nil {
		t.Fatal(err)
	}
	firstClosed := false
	t.Cleanup(func() {
		if !firstClosed {
			_ = first.close()
		}
	})

	attempted := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(attempted)
		second, err := registrar.inspect(fixture.workspace)
		if second != nil {
			if closeErr := second.close(); err == nil {
				err = closeErr
			}
		}
		result <- err
	}()
	<-attempted
	select {
	case err := <-result:
		t.Fatalf("second local registry inspection entered before first released: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := first.close(); err != nil {
		t.Fatal(err)
	}
	firstClosed = true
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("second local registry inspection after release: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second local registry inspection did not enter after release")
	}
}

type workspaceAuthorityRegistrationFixture struct {
	base           string
	hostRoot       string
	workspacesRoot string
	registryLock   string
	registry       string
	workspace      string
	ownerUID       uint32
}

type testWorkspaceAuthorityIdentity struct {
	identity runtimeWorkspaceIdentity
	info     os.FileInfo
}

func testWorkspaceAuthorityIdentityAtPath(t *testing.T, configuredWorkspace string) testWorkspaceAuthorityIdentity {
	t.Helper()
	configuredPath := filepath.ToSlash(filepath.Clean(configuredWorkspace))
	resolvedPath, err := filepath.EvalSymlinks(configuredWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	resolvedPath = filepath.ToSlash(filepath.Clean(resolvedPath))
	info, err := os.Stat(configuredWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("workspace stat type = %T, want *syscall.Stat_t", info.Sys())
	}
	identity := runtimeWorkspaceIdentity{
		configuredPath: configuredPath,
		resolvedPath:   resolvedPath,
		device:         uint64(stat.Dev),
		inode:          stat.Ino,
	}
	identity.rootHash = testWorkspaceAuthoritySHA256(testWorkspaceAuthorityIdentityJCS(t, identity))
	return testWorkspaceAuthorityIdentity{identity: identity, info: info}
}

func testWorkspaceAuthorityIdentityJCS(t *testing.T, identity runtimeWorkspaceIdentity) []byte {
	t.Helper()
	configuredJSON, err := json.Marshal(identity.configuredPath)
	if err != nil {
		t.Fatal(err)
	}
	resolvedJSON, err := json.Marshal(identity.resolvedPath)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(fmt.Sprintf(
		`{"configuredPath":%s,"device":%q,"inode":%q,"resolvedPath":%s}`,
		configuredJSON,
		strconv.FormatUint(identity.device, 10),
		strconv.FormatUint(identity.inode, 10),
		resolvedJSON,
	))
}

func testWorkspaceAuthoritySHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func openWorkspaceAuthorityTestDirectory(configuredPath string) (*os.File, error) {
	fd, err := syscall.Open(configuredPath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	workspace := os.NewFile(uintptr(fd), configuredPath)
	if workspace == nil {
		_ = syscall.Close(fd)
		return nil, errRuntimeNoncanonical
	}
	return workspace, nil
}

func countWorkspaceAuthorityDescriptors(t *testing.T, workspacePath string) int {
	t.Helper()
	want, err := os.Stat(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		info, statErr := os.Stat(filepath.Join("/proc/self/fd", entry.Name()))
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			t.Fatal(statErr)
		}
		if os.SameFile(info, want) {
			count++
		}
	}
	return count
}

func newWorkspaceAuthorityRegistrationFixture(t *testing.T, registry workspaceRegistryJCSV1) workspaceAuthorityRegistrationFixture {
	t.Helper()
	base := t.TempDir()
	hostRoot := filepath.Join(base, "authority")
	workspacesRoot, registryLock := prepareWorkspaceAuthorityRegistrationRoot(t, hostRoot, registry)
	workspace := filepath.Join(base, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	return workspaceAuthorityRegistrationFixture{
		base:           base,
		hostRoot:       hostRoot,
		workspacesRoot: workspacesRoot,
		registryLock:   registryLock,
		registry:       filepath.Join(workspacesRoot, "registry.private.json"),
		workspace:      workspace,
		ownerUID:       uint32(os.Geteuid()),
	}
}

func prepareWorkspaceAuthorityRegistrationRoot(t *testing.T, hostRoot string, registry workspaceRegistryJCSV1) (string, string) {
	t.Helper()
	workspacesRoot := filepath.Join(hostRoot, "workspaces")
	for _, path := range []string{hostRoot, workspacesRoot} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	registryLock := filepath.Join(workspacesRoot, "registry.lock")
	writePrivateAuthorityTestFile(t, registryLock, nil)
	raw, err := encodeWorkspaceRegistryJCSV1(registry)
	if err != nil {
		t.Fatal(err)
	}
	writePrivateAuthorityTestFile(t, filepath.Join(workspacesRoot, "registry.private.json"), raw)
	return workspacesRoot, registryLock
}

func workspaceRegistryWithIdentity(identity runtimeWorkspaceIdentity, workspaceAuthorityID string) workspaceRegistryJCSV1 {
	return workspaceRegistryJCSV1{
		Entries: []workspaceRegistryEntryJCSV1{{
			ConfiguredPath:              identity.configuredPath,
			Device:                      strconv.FormatUint(identity.device, 10),
			Inode:                       strconv.FormatUint(identity.inode, 10),
			WorkspaceAuthorityID:        workspaceAuthorityID,
			WorkspaceRootIdentitySHA256: identity.rootHash,
		}},
		RecordRev:      1,
		RegistrySchema: 1,
	}
}

func writePrivateAuthorityTestFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

func closeWorkspaceAuthorityInspection(t *testing.T, inspection interface{ close() error }) {
	t.Helper()
	if inspection == nil {
		return
	}
	if err := inspection.close(); err != nil {
		t.Fatalf("close workspace authority registry inspection: %v", err)
	}
}

type workspaceAuthorityTopologyEntry struct {
	mode   os.FileMode
	uid    uint32
	device uint64
	inode  uint64
	links  uint64
	size   int64
	hash   string
	target string
}

func snapshotWorkspaceAuthorityTopology(t *testing.T, roots ...string) map[string]workspaceAuthorityTopologyEntry {
	t.Helper()
	snapshot := map[string]workspaceAuthorityTopologyEntry{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return fmt.Errorf("stat type %T for %s", info.Sys(), path)
			}
			entry := workspaceAuthorityTopologyEntry{
				mode:   info.Mode(),
				uid:    stat.Uid,
				device: uint64(stat.Dev),
				inode:  stat.Ino,
				links:  uint64(stat.Nlink),
				size:   info.Size(),
			}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				entry.target, err = os.Readlink(path)
			case info.Mode().IsRegular():
				var raw []byte
				raw, err = os.ReadFile(path)
				if err == nil {
					sum := sha256.Sum256(raw)
					entry.hash = hex.EncodeToString(sum[:])
				}
			}
			if err != nil {
				return err
			}
			snapshot[path] = entry
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return snapshot
}
