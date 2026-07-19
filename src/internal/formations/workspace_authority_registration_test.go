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
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWorkspaceAuthorityRegistrationIdentityUsesOneOpenedDirectory(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	configuredWorkspace := filepath.Join(base, "configured-workspace")
	if err := os.Symlink(workspace, configuredWorkspace); err != nil {
		t.Fatal(err)
	}
	wantIdentity := testWorkspaceAuthorityIdentityAtPath(t, configuredWorkspace)
	openDescriptorsBefore := countWorkspaceAuthorityDescriptors(t, workspace)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, workspace)
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
	callbackCalls := 0
	err := withOpenedWorkspaceAuthorityIdentity(configuredWorkspace, openWorkspace, func(opened *os.File, identity runtimeWorkspaceIdentity) error {
		callbackCalls++
		if opened.Fd() != openedDescriptor {
			t.Fatalf("retained workspace descriptor = %d, want injected descriptor %d", opened.Fd(), openedDescriptor)
		}
		workspaceInfo, statErr := opened.Stat()
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
		if got, want := countWorkspaceAuthorityDescriptors(t, workspace), openDescriptorsBefore+1; got != want {
			t.Fatalf("retained workspace descriptors = %d, want %d during scoped identity callback", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("derive scoped workspace identity: %v", err)
	}
	if openCalls != 1 {
		t.Fatalf("workspace directory opener calls = %d, want exactly one", openCalls)
	}
	if callbackCalls != 1 {
		t.Fatalf("scoped workspace identity callback calls = %d, want exactly one", callbackCalls)
	}
	if got := countWorkspaceAuthorityDescriptors(t, workspace); got != openDescriptorsBefore {
		t.Fatalf("workspace descriptors after inspection close = %d, want original %d", got, openDescriptorsBefore)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, workspace)
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
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, fixture.workspace)
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
	callbackCalls := 0
	err := withOpenedWorkspaceAuthorityIdentity(configuredWorkspace, openWorkspace, func(workspace *os.File, identity runtimeWorkspaceIdentity) error {
		callbackCalls++
		if workspace.Fd() != openedDescriptor {
			t.Fatalf("retarget identity scope descriptor %d, want injected descriptor %d", workspace.Fd(), openedDescriptor)
		}
		if identity.configuredPath != wantIdentity.identity.configuredPath ||
			identity.resolvedPath != wantIdentity.identity.resolvedPath ||
			identity.device != wantIdentity.identity.device ||
			identity.inode != wantIdentity.identity.inode ||
			identity.rootHash != wantIdentity.identity.rootHash {
			t.Fatalf("retarget identity scope = %+v, want sole pre-retarget opened identity %+v", identity, wantIdentity.identity)
		}
		if got, want := countWorkspaceAuthorityDescriptors(t, fixture.workspace), openDescriptorsBefore+1; got != want {
			t.Fatalf("retarget identity scope descriptors for original workspace = %d, want %d", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("derive identity across controlled configured-path retarget: %v", err)
	}
	if openCalls != 1 {
		t.Fatalf("workspace directory opener calls across retarget = %d, want exactly one", openCalls)
	}
	if callbackCalls != 1 {
		t.Fatalf("retarget identity callback calls = %d, want exactly one", callbackCalls)
	}
	replacementInfo, err := os.Stat(configuredWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(replacementInfo, wantIdentity.info) {
		t.Fatal("controlled opener did not retarget configured workspace before returning")
	}
	if got := countWorkspaceAuthorityDescriptors(t, fixture.workspace); got != openDescriptorsBefore {
		t.Fatalf("original workspace descriptors after retarget identity scope = %d, want %d", got, openDescriptorsBefore)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, fixture.workspace)
}

func TestWorkspaceAuthorityRegistrationIdentityFollowsRetargetBeforeInjectedOpen(t *testing.T) {
	base := t.TempDir()
	originalWorkspace := filepath.Join(base, "original-workspace")
	replacementWorkspace := filepath.Join(base, "replacement-workspace")
	for _, path := range []string{originalWorkspace, replacementWorkspace} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	configuredWorkspace := filepath.Join(base, "configured-workspace")
	if err := os.Symlink(originalWorkspace, configuredWorkspace); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(configuredWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	openDescriptorsBefore := countWorkspaceAuthorityDescriptors(t, replacementWorkspace)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, originalWorkspace, replacementWorkspace)
	openCalls := 0
	var openedDescriptor uintptr
	var wantIdentity runtimeWorkspaceIdentity
	openWorkspace := func(configuredPath string) (*os.File, error) {
		openCalls++
		if err := os.Remove(configuredPath); err != nil {
			return nil, err
		}
		if err := os.Symlink(replacementWorkspace, configuredPath); err != nil {
			return nil, err
		}
		workspace, err := openWorkspaceAuthorityTestDirectory(configuredPath)
		if err != nil {
			return nil, err
		}
		openedDescriptor = workspace.Fd()
		wantIdentity = testWorkspaceAuthorityIdentityFromOpened(t, configuredPath, workspace)
		return workspace, nil
	}
	callbackCalls := 0
	err = withOpenedWorkspaceAuthorityIdentity(configuredWorkspace, openWorkspace, func(workspace *os.File, identity runtimeWorkspaceIdentity) error {
		callbackCalls++
		if workspace.Fd() != openedDescriptor {
			t.Fatalf("pre-open retarget scope descriptor %d, want injected descriptor %d", workspace.Fd(), openedDescriptor)
		}
		openedInfo, statErr := workspace.Stat()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if os.SameFile(openedInfo, originalInfo) {
			t.Fatal("pre-open retarget identity remained bound to original configured target")
		}
		if identity != wantIdentity {
			t.Fatalf("pre-open retarget identity = %+v, want independently captured replacement FD identity %+v", identity, wantIdentity)
		}
		if got, want := countWorkspaceAuthorityDescriptors(t, replacementWorkspace), openDescriptorsBefore+1; got != want {
			t.Fatalf("pre-open retarget replacement descriptors = %d, want %d during callback", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("derive identity after controlled pre-open retarget: %v", err)
	}
	if openCalls != 1 || callbackCalls != 1 {
		t.Fatalf("pre-open retarget opener/callback calls = %d/%d, want 1/1", openCalls, callbackCalls)
	}
	if got := countWorkspaceAuthorityDescriptors(t, replacementWorkspace); got != openDescriptorsBefore {
		t.Fatalf("replacement workspace descriptors after pre-open retarget identity scope = %d, want %d", got, openDescriptorsBefore)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, originalWorkspace, replacementWorkspace)
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

func TestWorkspaceAuthorityRegistrationScopeIsClosedValueOnlyObservationInterface(t *testing.T) {
	scopeType := reflect.TypeOf((*workspaceAuthorityRegistrationScope)(nil)).Elem()
	if scopeType.Kind() != reflect.Interface {
		t.Fatalf("registration scope kind = %s, want interface", scopeType.Kind())
	}
	if scopeType.Name() != "workspaceAuthorityRegistrationScope" || scopeType.PkgPath() == "" {
		t.Fatalf("registration scope identity = %s from %q, want named unexported package interface", scopeType.Name(), scopeType.PkgPath())
	}
	wantMethods := []struct {
		name      string
		signature reflect.Type
	}{
		{name: "matchedWorkspaceAuthorityID", signature: reflect.TypeOf((func() (string, bool))(nil))},
		{name: "registryLockIdentity", signature: reflect.TypeOf((func() (uint64, uint64))(nil))},
		{name: "workspaceIdentity", signature: reflect.TypeOf((func() runtimeWorkspaceIdentity)(nil))},
	}
	if scopeType.NumMethod() != len(wantMethods) {
		t.Fatalf("registration scope method count = %d, want exact closed set of %d", scopeType.NumMethod(), len(wantMethods))
	}
	for index, want := range wantMethods {
		method := scopeType.Method(index)
		if method.Name != want.name || method.Type != want.signature {
			t.Fatalf("registration scope method %d = %s %s, want %s %s", index, method.Name, method.Type, want.name, want.signature)
		}
		if method.PkgPath == "" || method.Func.IsValid() {
			t.Fatalf("registration scope method %s must remain unexported interface-only: package=%q func-valid=%t", method.Name, method.PkgPath, method.Func.IsValid())
		}
		for output := 0; output < method.Type.NumOut(); output++ {
			assertWorkspaceAuthorityValueOnlyTypeGraph(t, method.Type.Out(output), fmt.Sprintf("registration scope method %s result %d", method.Name, output))
		}
	}
}

func assertWorkspaceAuthorityValueOnlyTypeGraph(t *testing.T, root reflect.Type, rootPath string) {
	t.Helper()
	visited := map[reflect.Type]bool{}
	active := map[reflect.Type]bool{}
	var visit func(reflect.Type, string)
	visit = func(current reflect.Type, path string) {
		if current == nil {
			t.Fatalf("%s exposes an invalid return type", path)
		}
		if visited[current] || active[current] {
			return
		}
		switch current.Kind() {
		case reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Float32, reflect.Float64,
			reflect.Complex64, reflect.Complex128,
			reflect.String:
			visited[current] = true
		case reflect.Array:
			active[current] = true
			visit(current.Elem(), fmt.Sprintf("%s array element", path))
			delete(active, current)
			visited[current] = true
		case reflect.Struct:
			active[current] = true
			for fieldIndex := 0; fieldIndex < current.NumField(); fieldIndex++ {
				field := current.Field(fieldIndex)
				visit(field.Type, path+"."+field.Name)
			}
			delete(active, current)
			visited[current] = true
		default:
			t.Fatalf("%s exposes resource/reference-bearing kind %s through type %s", path, current.Kind(), current)
		}
	}
	visit(root, rootPath)
}

func TestWorkspaceAuthorityRegistrationCleansConfiguredSpellingAndRejectsInvalidPathGrammar(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())

	noise := filepath.Join(filepath.Dir(fixture.workspace), "noise")
	if err := os.Mkdir(noise, 0o700); err != nil {
		t.Fatal(err)
	}
	unclean := filepath.Join(noise, "..", filepath.Base(fixture.workspace))
	descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
	cleanDescriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	var configuredPath string
	callbackCalls := 0
	err := registrar.inspect(unclean, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		configuredPath = scope.workspaceIdentity().configuredPath
		return nil
	})
	if err != nil {
		t.Fatalf("inspect cleaned configured spelling: %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("clean configured path callback calls = %d, want exactly one", callbackCalls)
	}
	if got, want := configuredPath, filepath.ToSlash(filepath.Clean(unclean)); got != want {
		t.Fatalf("cleaned configured spelling = %q, want %q", got, want)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, cleanDescriptorsBefore, descriptorPaths...)

	for _, configured := range []string{
		"relative/workspace",
		fixture.workspace + "\x00suffix",
		fixture.workspace + `\alias`,
		fixture.workspace + string([]byte{0xff}),
	} {
		t.Run(strconv.Quote(configured), func(t *testing.T) {
			before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
			callbackCalls := 0
			err := registrar.inspect(configured, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if err == nil || !errors.Is(err, errRuntimeNoncanonical) {
				t.Fatalf("invalid configured path error = %v, want noncanonical", err)
			}
			if callbackCalls != 0 {
				t.Fatalf("invalid configured path callback calls = %d, want zero", callbackCalls)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid configured path changed authority topology\nbefore: %#v\nafter:  %#v", before, after)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
	openDescriptorsBefore := countWorkspaceAuthorityDescriptors(t, fixture.workspace)
	descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	openCalls := 0
	var beforeRejection map[string]workspaceAuthorityTopologyEntry
	openWorkspace := func(configuredPath string) (*os.File, error) {
		openCalls++
		workspace, err := openWorkspaceAuthorityTestDirectory(configuredPath)
		if err != nil {
			return nil, err
		}
		if err := os.Remove(configuredPath); err != nil {
			workspace.Close()
			return nil, err
		}
		if err := os.Symlink(replacement, configuredPath); err != nil {
			workspace.Close()
			return nil, err
		}
		beforeRejection = snapshotWorkspaceAuthorityTopology(t, fixture.base)
		return workspace, nil
	}
	registrar := newWorkspaceAuthorityRegistrarForTest(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), workspaceAuthorityRegistrationTestOps{openWorkspace: openWorkspace})
	callbackCalls := 0
	err := registrar.inspect(configured, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if !errors.Is(err, errRuntimeIntegrityMismatch) {
		t.Fatalf("retargeted configured workspace error = %v, want integrity mismatch", err)
	}
	if callbackCalls != 0 {
		t.Fatalf("retargeted configured workspace callback calls = %d, want zero before critical-section callback", callbackCalls)
	}
	if openCalls != 1 {
		t.Fatalf("retargeted configured workspace opener calls = %d, want exactly one", openCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, beforeRejection) {
		t.Fatalf("retarget rejection changed authority topology\nbefore: %#v\nafter:  %#v", beforeRejection, after)
	}
	if got := countWorkspaceAuthorityDescriptors(t, fixture.workspace); got != openDescriptorsBefore {
		t.Fatalf("original workspace descriptors after registrar retarget rejection = %d, want %d", got, openDescriptorsBefore)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
			descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)

			registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, expectedUID, newWorkspaceAuthorityCapabilityGate())
			callbackCalls := 0
			err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if err == nil || !errors.Is(err, errRuntimeIntegrityMismatch) {
				t.Fatalf("unsafe private root/lock error = %v, want integrity mismatch", err)
			}
			if callbackCalls != 0 {
				t.Fatalf("unsafe private root/lock callback calls = %d, want zero", callbackCalls)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, roots...); !reflect.DeepEqual(after, before) {
				t.Fatalf("private root/lock rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
		})
	}
}

func TestWorkspaceAuthorityRegistrationRequiresEveryPreprovisionedNodeWithoutCreatingIt(t *testing.T) {
	tests := []struct {
		name string
		path func(workspaceAuthorityRegistrationFixture) string
	}{
		{name: "host root missing", path: func(fixture workspaceAuthorityRegistrationFixture) string { return fixture.hostRoot }},
		{name: "workspaces root missing", path: func(fixture workspaceAuthorityRegistrationFixture) string { return fixture.workspacesRoot }},
		{name: "registry lock missing", path: func(fixture workspaceAuthorityRegistrationFixture) string { return fixture.registryLock }},
		{name: "registry private missing", path: func(fixture workspaceAuthorityRegistrationFixture) string { return fixture.registry }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
				Entries:        []workspaceRegistryEntryJCSV1{},
				RecordRev:      1,
				RegistrySchema: 1,
			})
			missingPath := test.path(fixture)
			movedPath := missingPath + ".preprovisioned"
			if err := os.Rename(missingPath, movedPath); err != nil {
				t.Fatal(err)
			}
			before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
			descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
			registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
			callbackCalls := 0
			err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s error = %v, want missing preprovisioned node", test.name, err)
			}
			if callbackCalls != 0 {
				t.Fatalf("%s callback calls = %d, want zero", test.name, callbackCalls)
			}
			if _, statErr := os.Lstat(missingPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("%s was recreated at %q: %v", test.name, missingPath, statErr)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
				t.Fatalf("%s rejection changed topology\nbefore: %#v\nafter:  %#v", test.name, before, after)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
			descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
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
			registrar := newWorkspaceAuthorityRegistrarForTest(
				fixture.hostRoot,
				fixture.ownerUID,
				newWorkspaceAuthorityCapabilityGate(),
				workspaceAuthorityRegistrationTestOps{validatePrivateNode: validatePrivateNode},
			)
			callbackCalls := 0
			err = registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if !errors.Is(err, errRuntimeIntegrityMismatch) {
				t.Fatalf("injected %s uid mismatch error = %v, want integrity mismatch", component.name, err)
			}
			if callbackCalls != 0 {
				t.Fatalf("injected %s uid mismatch callback calls = %d, want zero", component.name, callbackCalls)
			}
			if validatedTarget != 1 {
				t.Fatalf("%s expected-writer-uid validation calls = %d, want exactly one", component.name, validatedTarget)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
				t.Fatalf("%s uid rejection changed topology\nbefore: %#v\nafter:  %#v", component.name, before, after)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
		descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
		descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
		callbackCalls := 0
		err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			return nil
		})
		if err == nil {
			t.Fatal("registration accepted a non-directory host root")
		}
		if callbackCalls != 0 {
			t.Fatalf("non-directory host-root callback calls = %d, want zero", callbackCalls)
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
			t.Fatalf("host-root type rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
		}
		assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
		descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
		descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
		callbackCalls := 0
		err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			return nil
		})
		if err == nil {
			t.Fatal("registration followed a symlinked host root")
		}
		if callbackCalls != 0 {
			t.Fatalf("symlinked host-root callback calls = %d, want zero", callbackCalls)
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
			t.Fatalf("host-root symlink rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
		}
		assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
		descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
		descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
		callbackCalls := 0
		err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			return nil
		})
		if err == nil {
			t.Fatal("registration followed a symlinked host-root ancestor")
		}
		if callbackCalls != 0 {
			t.Fatalf("symlinked host-root ancestor callback calls = %d, want zero", callbackCalls)
		}
		if after := snapshotWorkspaceAuthorityTopology(t, ancestor, realAncestor); !reflect.DeepEqual(after, before) {
			t.Fatalf("ancestor-symlink rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
		}
		assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
	})

	t.Run("opened root renamed and replaced", func(t *testing.T) {
		fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
			Entries:        []workspaceRegistryEntryJCSV1{},
			RecordRev:      1,
			RegistrySchema: 1,
		})
		movedRoot := fixture.hostRoot + ".opened"
		descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
		descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
		var beforeRejection map[string]workspaceAuthorityTopologyEntry
		openWorkspace := func(configuredPath string) (*os.File, error) {
			workspace, err := openWorkspaceAuthorityTestDirectory(configuredPath)
			if err != nil {
				return nil, err
			}
			if err := os.Rename(fixture.hostRoot, movedRoot); err != nil {
				workspace.Close()
				return nil, err
			}
			if err := os.Mkdir(fixture.hostRoot, 0o700); err != nil {
				workspace.Close()
				return nil, err
			}
			beforeRejection = snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedRoot)
			return workspace, nil
		}
		registrar := newWorkspaceAuthorityRegistrarForTest(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), workspaceAuthorityRegistrationTestOps{openWorkspace: openWorkspace})
		callbackCalls := 0
		err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			return nil
		})
		if !errors.Is(err, errRuntimeIntegrityMismatch) {
			t.Fatalf("renamed host root error = %v, want integrity mismatch", err)
		}
		if callbackCalls != 0 {
			t.Fatalf("renamed host-root callback calls = %d, want zero", callbackCalls)
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedRoot); !reflect.DeepEqual(after, beforeRejection) {
			t.Fatalf("renamed-root rejection changed topology\nbefore: %#v\nafter:  %#v", beforeRejection, after)
		}
		assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
	})
}

func TestWorkspaceAuthorityRegistrationRejectsReplacedWorkspacesRoot(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	movedWorkspaces := fixture.workspacesRoot + ".opened"
	descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	var beforeRejection map[string]workspaceAuthorityTopologyEntry
	openWorkspace := func(configuredPath string) (*os.File, error) {
		workspace, err := openWorkspaceAuthorityTestDirectory(configuredPath)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(fixture.workspacesRoot, movedWorkspaces); err != nil {
			workspace.Close()
			return nil, err
		}
		if err := os.Mkdir(fixture.workspacesRoot, 0o700); err != nil {
			workspace.Close()
			return nil, err
		}
		writePrivateAuthorityTestFile(t, fixture.registryLock, nil)
		replacementRegistry, encodeErr := encodeWorkspaceRegistryJCSV1(workspaceRegistryJCSV1{
			Entries:        []workspaceRegistryEntryJCSV1{},
			RecordRev:      1,
			RegistrySchema: 1,
		})
		if encodeErr != nil {
			workspace.Close()
			return nil, encodeErr
		}
		writePrivateAuthorityTestFile(t, fixture.registry, replacementRegistry)
		beforeRejection = snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedWorkspaces)
		return workspace, nil
	}
	registrar := newWorkspaceAuthorityRegistrarForTest(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), workspaceAuthorityRegistrationTestOps{openWorkspace: openWorkspace})
	callbackCalls := 0
	err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if !errors.Is(err, errRuntimeIntegrityMismatch) {
		t.Fatalf("replaced workspaces root error = %v, want integrity mismatch", err)
	}
	if callbackCalls != 0 {
		t.Fatalf("replaced workspaces-root callback calls = %d, want zero", callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.hostRoot, movedWorkspaces); !reflect.DeepEqual(after, beforeRejection) {
		t.Fatalf("workspaces-root replacement rejection changed topology\nbefore: %#v\nafter:  %#v", beforeRejection, after)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
}

func TestWorkspaceAuthorityRegistrationRejectsReplacedRegistryNodesBeforeCallback(t *testing.T) {
	tests := []struct {
		name           string
		target         func(workspaceAuthorityRegistrationFixture) string
		replace        func(*testing.T, string, string)
		proveSplitLock bool
	}{
		{
			name:   "registry lock",
			target: func(fixture workspaceAuthorityRegistrationFixture) string { return fixture.registryLock },
			replace: func(t *testing.T, _, namedPath string) {
				writePrivateAuthorityTestFile(t, namedPath, nil)
			},
			proveSplitLock: true,
		},
		{
			name:   "registry private",
			target: func(fixture workspaceAuthorityRegistrationFixture) string { return fixture.registry },
			replace: func(t *testing.T, movedPath, namedPath string) {
				raw, err := os.ReadFile(movedPath)
				if err != nil {
					t.Fatal(err)
				}
				writePrivateAuthorityTestFile(t, namedPath, raw)
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
			targetPath := test.target(fixture)
			movedPath := targetPath + ".opened"
			descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
			openCalls := 0
			var beforeRejection map[string]workspaceAuthorityTopologyEntry
			splitLockProved := false
			openWorkspace := func(configuredPath string) (*os.File, error) {
				openCalls++
				workspace, err := openWorkspaceAuthorityTestDirectory(configuredPath)
				if err != nil {
					return nil, err
				}
				if err := os.Rename(targetPath, movedPath); err != nil {
					workspace.Close()
					return nil, err
				}
				test.replace(t, movedPath, targetPath)
				if test.proveSplitLock {
					if err := tryWorkspaceAuthorityExclusiveLock(targetPath); err != nil {
						workspace.Close()
						return nil, fmt.Errorf("replacement registry lock should expose a distinct lock domain: %w", err)
					}
					splitLockProved = true
				}
				beforeRejection = snapshotWorkspaceAuthorityTopology(t, fixture.base)
				return workspace, nil
			}
			registrar := newWorkspaceAuthorityRegistrarForTest(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate(), workspaceAuthorityRegistrationTestOps{openWorkspace: openWorkspace})
			callbackCalls := 0
			err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if !errors.Is(err, errRuntimeIntegrityMismatch) {
				t.Fatalf("replaced %s error = %v, want integrity mismatch", test.name, err)
			}
			if callbackCalls != 0 {
				t.Fatalf("replaced %s callback calls = %d, want zero", test.name, callbackCalls)
			}
			if openCalls != 1 {
				t.Fatalf("replaced %s workspace opener calls = %d, want exactly one", test.name, openCalls)
			}
			if test.proveSplitLock && !splitLockProved {
				t.Fatal("registry lock replacement did not prove the named path selected a distinct lock inode")
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, beforeRejection) {
				t.Fatalf("replaced %s rejection changed topology\nbefore: %#v\nafter:  %#v", test.name, beforeRejection, after)
			}
			for _, lockPath := range []string{fixture.registryLock, movedPath} {
				if test.proveSplitLock {
					if err := tryWorkspaceAuthorityExclusiveLock(lockPath); err != nil {
						t.Fatalf("registry scope leaked lock on %q after rejection: %v", lockPath, err)
					}
				}
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
		})
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
			descriptorPaths := []string{hostRoot, filepath.Join(hostRoot, "workspaces"), filepath.Join(hostRoot, "workspaces", "registry.lock"), filepath.Join(hostRoot, "workspaces", "registry.private.json"), workspace}
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
			registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate())
			callbackCalls := 0
			err := registrar.inspect(workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if !errors.Is(err, errRuntimeConflict) {
				t.Fatalf("overlapping authority/workspace error = %v, want conflict", err)
			}
			if callbackCalls != 0 {
				t.Fatalf("overlapping authority/workspace callback calls = %d, want zero", callbackCalls)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, hostRoot, workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("overlap rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
		})
	}
}

func TestWorkspaceAuthorityRegistrationAcceptsConfiguredAndResolvedSiblingPrefix(t *testing.T) {
	base := t.TempDir()
	hostRoot := filepath.Join(base, "workspace-authority")
	prepareWorkspaceAuthorityRegistrationRoot(t, hostRoot, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	workspace := filepath.Join(base, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	configuredWorkspace := filepath.Join(base, "configured-workspace")
	if err := os.Symlink(workspace, configuredWorkspace); err != nil {
		t.Fatal(err)
	}
	before := snapshotWorkspaceAuthorityTopology(t, base)
	descriptorPaths := []string{hostRoot, filepath.Join(hostRoot, "workspaces"), filepath.Join(hostRoot, "workspaces", "registry.lock"), filepath.Join(hostRoot, "workspaces", "registry.private.json"), workspace}
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate())
	callbackCalls := 0
	err := registrar.inspect(configuredWorkspace, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		if authorityID, matched := scope.matchedWorkspaceAuthorityID(); matched {
			t.Fatalf("unregistered sibling-prefix workspace matched authority %q", authorityID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sibling-prefix workspace inspection: %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("sibling-prefix workspace callback calls = %d, want exactly one", callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, base); !reflect.DeepEqual(after, before) {
		t.Fatalf("sibling-prefix workspace inspection changed topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
	descriptorPaths := []string{hostRoot, workspacesRoot, filepath.Join(workspacesRoot, "registry.private.json"), workspace}
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)

	invalidGate := workspaceAuthorityCapabilityGate{capabilities: []workspaceAuthorityCapability{{id: RuntimeAuthorityGuardCapabilityV1}}}
	registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), invalidGate)
	callbackCalls := 0
	err = registrar.inspect(workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if !errors.Is(err, errRuntimeUnsupportedSchema) {
		t.Fatalf("invalid capability error = %v, want unsupported schema before missing lock", err)
	}
	if callbackCalls != 0 {
		t.Fatalf("invalid capability callback calls = %d, want zero", callbackCalls)
	}
	if _, statErr := os.Lstat(filepath.Join(workspacesRoot, "registry.lock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid capability selected or created registry lock: %v", statErr)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, base); !reflect.DeepEqual(after, before) {
		t.Fatalf("capability rejection changed topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
}

func TestWorkspaceAuthorityRegistryLockSpansPrivateValidationIdentityAndScopedCallback(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	wantLock := workspaceAuthorityLockIdentityAtPath(t, fixture.registryLock)
	registryInfo, err := os.Lstat(fixture.registry)
	if err != nil {
		t.Fatal(err)
	}
	registryValidationLocked := false
	validatePrivateNode := func(opened *os.File, expectedUID uint32) error {
		info, statErr := opened.Stat()
		if statErr != nil {
			return statErr
		}
		if os.SameFile(info, registryInfo) {
			if lockErr := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(lockErr, syscall.EWOULDBLOCK) {
				return fmt.Errorf("registry lock during registry.private validation = %v, want would-block", lockErr)
			}
			registryValidationLocked = true
		}
		return validateWorkspaceAuthorityTestPrivateNode(opened, expectedUID)
	}
	identityOpenLocked := false
	openWorkspace := func(configuredPath string) (*os.File, error) {
		if lockErr := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(lockErr, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("registry lock during workspace identity open = %v, want would-block", lockErr)
		}
		identityOpenLocked = true
		return openWorkspaceAuthorityTestDirectory(configuredPath)
	}
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{openWorkspace: openWorkspace, validatePrivateNode: validatePrivateNode},
	)
	callbackCalls := 0
	err = registrar.inspect(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		device, inode := scope.registryLockIdentity()
		if got := (workspaceAuthorityLockIdentity{device: device, inode: inode}); got != wantLock {
			return fmt.Errorf("scoped registry lock identity = %+v, want named inode %+v", got, wantLock)
		}
		if lockErr := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(lockErr, syscall.EWOULDBLOCK) {
			return fmt.Errorf("registry lock during authorized callback = %v, want would-block", lockErr)
		}
		if authorityID, matched := scope.matchedWorkspaceAuthorityID(); matched {
			return fmt.Errorf("empty registry matched authority %q", authorityID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("registry lock scope proof: %v", err)
	}
	if !registryValidationLocked || !identityOpenLocked || callbackCalls != 1 {
		t.Fatalf("registry lock scope checkpoints validation/open/callback = %t/%t/%d, want true/true/1", registryValidationLocked, identityOpenLocked, callbackCalls)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("registry lock remained held after scoped callback: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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
			name: "configured spelling changed device only",
			registry: func(identity runtimeWorkspaceIdentity) workspaceRegistryJCSV1 {
				record := workspaceRegistryWithIdentity(identity, testWorkspaceAuthorityID)
				record.Entries[0].Device = strconv.FormatUint(identity.device+1, 10)
				return record
			},
			wantError: errRuntimeIntegrityMismatch,
		},
		{
			name: "configured spelling changed root identity hash only",
			registry: func(identity runtimeWorkspaceIdentity) workspaceRegistryJCSV1 {
				record := workspaceRegistryWithIdentity(identity, testWorkspaceAuthorityID)
				record.Entries[0].WorkspaceRootIdentitySHA256 = strings.Repeat("f", 64)
				if record.Entries[0].WorkspaceRootIdentitySHA256 == identity.rootHash {
					record.Entries[0].WorkspaceRootIdentitySHA256 = strings.Repeat("e", 64)
				}
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
			workspacesRoot, registryLock := prepareWorkspaceAuthorityRegistrationRoot(t, hostRoot, workspaceRegistryJCSV1{
				Entries:        []workspaceRegistryEntryJCSV1{},
				RecordRev:      1,
				RegistrySchema: 1,
			})
			writePrivateAuthorityTestFile(t, filepath.Join(workspacesRoot, "registry.private.json"), raw)
			before := snapshotWorkspaceAuthorityTopology(t, base)
			descriptorPaths := []string{hostRoot, workspacesRoot, registryLock, filepath.Join(workspacesRoot, "registry.private.json"), workspace}
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)

			registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate())
			callbackCalls := 0
			var authorityID string
			var matched bool
			err = registrar.inspect(workspace, func(scope workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				authorityID, matched = scope.matchedWorkspaceAuthorityID()
				return nil
			})
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("registry lookup error = %v, want %v", err, test.wantError)
				}
				if callbackCalls != 0 {
					t.Fatalf("invalid registry callback calls = %d, want zero", callbackCalls)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if callbackCalls != 1 {
					t.Fatalf("valid registry callback calls = %d, want exactly one", callbackCalls)
				}
				if matched != test.wantEntry {
					t.Fatalf("registry match authority id = %q, present=%t, want present=%t", authorityID, matched, test.wantEntry)
				}
				if test.wantEntry && authorityID != testWorkspaceAuthorityID {
					t.Fatalf("matched workspace authority id = %q, want %q", authorityID, testWorkspaceAuthorityID)
				}
			}
			if after := snapshotWorkspaceAuthorityTopology(t, base); !reflect.DeepEqual(after, before) {
				t.Fatalf("registry lookup changed topology\nbefore: %#v\nafter:  %#v", before, after)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
		})
	}
}

func TestWorkspaceAuthorityRegistrationUsesCertifiedStrictRegistryDecoderWithoutMutation(t *testing.T) {
	tests := []struct {
		name     string
		wantCode RuntimeAuthorityGuardCode
		mutate   func(*testing.T, []byte) []byte
	}{
		{
			name:     "unknown top-level key",
			wantCode: RuntimeAuthorityGuardUnknownKey,
			mutate: func(t *testing.T, raw []byte) []byte {
				return replaceWorkspaceAuthorityRegistryRawExactlyOnce(t, raw, `,"priorGeneration":`, `,"futureTopLevel":true,"priorGeneration":`)
			},
		},
		{
			name:     "unknown nested entry key",
			wantCode: RuntimeAuthorityGuardUnknownKey,
			mutate: func(t *testing.T, raw []byte) []byte {
				return replaceWorkspaceAuthorityRegistryRawExactlyOnce(t, raw, `,"device":`, `,"futureNested":true,"device":`)
			},
		},
		{
			name:     "duplicate top-level key",
			wantCode: RuntimeAuthorityGuardDuplicateKey,
			mutate: func(t *testing.T, raw []byte) []byte {
				return replaceWorkspaceAuthorityRegistryRawExactlyOnce(t, raw, `,"recordRev":`, `,"recordRev":1,"recordRev":`)
			},
		},
		{
			name:     "duplicate nested entry key",
			wantCode: RuntimeAuthorityGuardDuplicateKey,
			mutate: func(t *testing.T, raw []byte) []byte {
				return replaceWorkspaceAuthorityRegistryRawExactlyOnce(t, raw, `,"device":`, `,"device":"0","device":`)
			},
		},
		{
			name:     "unsupported registry schema",
			wantCode: RuntimeAuthorityGuardUnsupportedSchema,
			mutate: func(t *testing.T, raw []byte) []byte {
				return replaceWorkspaceAuthorityRegistryRawExactlyOnce(t, raw, `"registrySchema":1}`, `"registrySchema":2}`)
			},
		},
		{
			name:     "null entries",
			wantCode: RuntimeAuthorityGuardMalformed,
			mutate: func(t *testing.T, raw []byte) []byte {
				return replaceWorkspaceAuthorityRegistryEntries(t, raw, "null")
			},
		},
		{
			name:     "wrong entries type",
			wantCode: RuntimeAuthorityGuardMalformed,
			mutate: func(t *testing.T, raw []byte) []byte {
				return replaceWorkspaceAuthorityRegistryEntries(t, raw, `{}`)
			},
		},
		{
			name:     "malformed JSON",
			wantCode: RuntimeAuthorityGuardMalformed,
			mutate: func(*testing.T, []byte) []byte {
				return []byte(`{"entries":]}`)
			},
		},
		{
			name:     "truncated JSON",
			wantCode: RuntimeAuthorityGuardMalformed,
			mutate: func(t *testing.T, raw []byte) []byte {
				if len(raw) < 2 {
					t.Fatal("canonical registry fixture unexpectedly empty")
				}
				return append([]byte(nil), raw[:len(raw)-1]...)
			},
		},
		{
			name:     "noncanonical top-level key order",
			wantCode: RuntimeAuthorityGuardNoncanonical,
			mutate: func(t *testing.T, raw []byte) []byte {
				const prefix = `{`
				const suffix = `,"registrySchema":1}`
				text := string(raw)
				if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, suffix) {
					t.Fatalf("canonical registry fixture = %q, want expected object prefix/suffix", raw)
				}
				body := text[len(prefix) : len(text)-len(suffix)]
				return []byte(`{"registrySchema":1,` + body + `}`)
			},
		},
		{
			name:     "noncanonical whitespace",
			wantCode: RuntimeAuthorityGuardNoncanonical,
			mutate: func(_ *testing.T, raw []byte) []byte {
				return append([]byte(" \t"), raw...)
			},
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
			canonical, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryWithIdentity(identity, testWorkspaceAuthorityID))
			if err != nil {
				t.Fatal(err)
			}
			raw := test.mutate(t, canonical)
			if _, err := decodeWorkspaceRegistryJCSV1(raw); err == nil {
				t.Fatal("certified registry decoder accepted strict rejection fixture")
			} else if code := workspaceAuthorityRegistrationErrorCode(err); code != test.wantCode {
				t.Fatalf("certified registry decoder code = %q, want %q", code, test.wantCode)
			}

			hostRoot := filepath.Join(base, "authority")
			workspacesRoot, registryLock := prepareWorkspaceAuthorityRegistrationRoot(t, hostRoot, workspaceRegistryJCSV1{
				Entries:        []workspaceRegistryEntryJCSV1{},
				RecordRev:      1,
				RegistrySchema: 1,
			})
			registry := filepath.Join(workspacesRoot, "registry.private.json")
			writePrivateAuthorityTestFile(t, registry, raw)
			beforeRaw, err := os.ReadFile(registry)
			if err != nil {
				t.Fatal(err)
			}
			before := snapshotWorkspaceAuthorityTopology(t, base)
			descriptorPaths := []string{hostRoot, workspacesRoot, registryLock, registry, workspace}
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)

			registrar := newWorkspaceAuthorityRegistrar(hostRoot, uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate())
			callbackCalls := 0
			err = registrar.inspect(workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if err == nil {
				t.Fatal("registrar accepted registry bytes rejected by certified decoder")
			}
			if code := workspaceAuthorityRegistrationErrorCode(err); code != test.wantCode {
				t.Fatalf("registrar registry error code = %q, want certified decoder code %q: %v", code, test.wantCode, err)
			}
			if callbackCalls != 0 {
				t.Fatalf("strict registry rejection callback calls = %d, want zero", callbackCalls)
			}
			afterRaw, readErr := os.ReadFile(registry)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !reflect.DeepEqual(afterRaw, beforeRaw) {
				t.Fatalf("strict registry rejection changed bytes\nbefore: %q\nafter:  %q", beforeRaw, afterRaw)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, base); !reflect.DeepEqual(after, before) {
				t.Fatalf("strict registry rejection changed byte/mtime/ctime topology\nbefore: %#v\nafter:  %#v", before, after)
			}
			if err := tryWorkspaceAuthorityExclusiveLock(registryLock); err != nil {
				t.Fatalf("strict registry rejection leaked registry lock: %v", err)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
		})
	}
}

func TestWorkspaceAuthorityRegistryCriticalSectionSerializesLocally(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
	descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	firstEntered := make(chan struct{})
	firstLockIdentity := make(chan workspaceAuthorityLockIdentity, 1)
	firstRelease := make(chan struct{})
	firstReleased := false
	t.Cleanup(func() {
		if !firstReleased {
			close(firstRelease)
		}
	})
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- registrar.inspect(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
			device, inode := scope.registryLockIdentity()
			firstLockIdentity <- workspaceAuthorityLockIdentity{device: device, inode: inode}
			close(firstEntered)
			<-firstRelease
			return nil
		})
	}()
	select {
	case <-firstEntered:
	case err := <-firstResult:
		t.Fatalf("first local registry scope returned before entering its callback: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("first local registry callback did not enter within bounded wait")
	}
	if got, want := <-firstLockIdentity, workspaceAuthorityLockIdentityAtPath(t, fixture.registryLock); got != want {
		t.Fatalf("local holder registry lock identity = %+v, want exact named inode %+v", got, want)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("independent nonblocking flock while local holder callback active = %v, want would-block", err)
	}

	secondAttempted := make(chan struct{})
	secondEntered := make(chan struct{})
	secondResult := make(chan error, 1)
	go func() {
		close(secondAttempted)
		secondResult <- registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			close(secondEntered)
			return nil
		})
	}()
	<-secondAttempted
	select {
	case <-secondEntered:
		t.Fatal("second local registry callback entered before first callback returned")
	case err := <-secondResult:
		t.Fatalf("second local registry scope returned before first callback released: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(firstRelease)
	firstReleased = true
	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("first local registry scope after callback release: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first local registry scope did not return after callback release")
	}
	secondResultConsumed := false
	select {
	case <-secondEntered:
	case err := <-secondResult:
		secondResultConsumed = true
		if err != nil {
			t.Fatalf("second local registry scope after release: %v", err)
		}
		select {
		case <-secondEntered:
		default:
			t.Fatal("second local registry scope returned without entering its callback")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second local registry callback did not enter after first callback returned")
	}
	if !secondResultConsumed {
		select {
		case err := <-secondResult:
			if err != nil {
				t.Fatalf("second local registry scope after release: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("second local registry scope did not return after callback")
		}
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("local registry scope leaked lock after callbacks: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
}

func TestWorkspaceAuthorityRegistrationScopeReleasesDescriptorsAndLockAfterCallbackError(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
	wantError := errors.New("test registration callback failure")
	callbackCalls := 0
	err := registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return wantError
	})
	if !errors.Is(err, wantError) {
		t.Fatalf("registration callback error = %v, want propagated sentinel", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("failing registration callback calls = %d, want exactly one", callbackCalls)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("registration callback error leaked registry lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)

	retryCalls := 0
	retryResult := make(chan error, 1)
	go func() {
		retryResult <- registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			retryCalls++
			return nil
		})
	}()
	select {
	case err := <-retryResult:
		if err != nil {
			t.Fatalf("registration retry after callback error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("registration retry remained blocked after callback error")
	}
	if retryCalls != 1 {
		t.Fatalf("registration retry callback calls = %d, want exactly one", retryCalls)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
}

func TestWorkspaceAuthorityRegistrationScopeReleasesDescriptorsAndLockAfterCallbackPanic(t *testing.T) {
	fixture := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	descriptorPaths := workspaceAuthorityRegistrationFixturePaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, descriptorPaths...)
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
	wantPanic := errors.New("test registration callback panic")
	callbackCalls := 0
	returned := false
	var returnedErr error
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		returnedErr = registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			panic(wantPanic)
		})
		returned = true
	}()
	if returned {
		t.Fatalf("registration callback panic was swallowed as return %v", returnedErr)
	}
	if recovered != wantPanic {
		t.Fatalf("registration callback recovered panic = %#v, want exact sentinel %#v", recovered, wantPanic)
	}
	if callbackCalls != 1 {
		t.Fatalf("panicking registration callback calls = %d, want exactly one", callbackCalls)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("registration callback panic leaked registry lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)

	retryCalls := 0
	retryResult := make(chan error, 1)
	go func() {
		retryResult <- registrar.inspect(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			retryCalls++
			return nil
		})
	}()
	select {
	case err := <-retryResult:
		if err != nil {
			t.Fatalf("registration retry after callback panic: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("registration retry remained blocked after callback panic")
	}
	if retryCalls != 1 {
		t.Fatalf("registration retry after panic callback calls = %d, want exactly one", retryCalls)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("registration retry after panic leaked registry lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, descriptorPaths...)
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

type workspaceAuthorityRegistrationTestOps struct {
	openWorkspace       func(string) (*os.File, error)
	validatePrivateNode func(*os.File, uint32) error
}

func newWorkspaceAuthorityRegistrarForTest(hostRoot string, expectedUID uint32, gate workspaceAuthorityCapabilityGate, overrides workspaceAuthorityRegistrationTestOps) *workspaceAuthorityRegistrar {
	registrar := newWorkspaceAuthorityRegistrar(hostRoot, expectedUID, gate)
	ops := registrar.ops
	if overrides.openWorkspace != nil {
		ops.openWorkspace = overrides.openWorkspace
	}
	if overrides.validatePrivateNode != nil {
		ops.validatePrivateNode = overrides.validatePrivateNode
	}
	registrar.ops = ops
	return registrar
}

func workspaceAuthorityRegistrationErrorCode(err error) RuntimeAuthorityGuardCode {
	var decodeErr runtimeDecodeError
	if errors.As(err, &decodeErr) {
		return decodeErr.code
	}
	return runtimeGuardValidationCode(err)
}

func replaceWorkspaceAuthorityRegistryRawExactlyOnce(t *testing.T, raw []byte, old, replacement string) []byte {
	t.Helper()
	text := string(raw)
	if count := strings.Count(text, old); count != 1 {
		t.Fatalf("registry fixture occurrence count for %q = %d, want exactly one in %q", old, count, raw)
	}
	return []byte(strings.Replace(text, old, replacement, 1))
}

func replaceWorkspaceAuthorityRegistryEntries(t *testing.T, raw []byte, replacement string) []byte {
	t.Helper()
	const prefix = `{"entries":`
	const nextField = `,"priorGeneration":`
	text := string(raw)
	if !strings.HasPrefix(text, prefix) {
		t.Fatalf("registry fixture = %q, want entries first", raw)
	}
	next := strings.Index(text, nextField)
	if next < len(prefix) {
		t.Fatalf("registry fixture = %q, want priorGeneration after entries", raw)
	}
	return []byte(prefix + replacement + text[next:])
}

type workspaceAuthorityOpenDescriptorSnapshot struct {
	total  int
	byPath map[string]int
}

func workspaceAuthorityRegistrationFixturePaths(fixture workspaceAuthorityRegistrationFixture) []string {
	return []string{fixture.hostRoot, fixture.workspacesRoot, fixture.registryLock, fixture.registry, fixture.workspace}
}

func snapshotWorkspaceAuthorityOpenDescriptors(t *testing.T, paths ...string) workspaceAuthorityOpenDescriptorSnapshot {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	openInfos := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, statErr := os.Stat(filepath.Join("/proc/self/fd", entry.Name()))
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			t.Fatal(statErr)
		}
		openInfos = append(openInfos, info)
	}
	snapshot := workspaceAuthorityOpenDescriptorSnapshot{total: len(openInfos), byPath: map[string]int{}}
	for _, path := range paths {
		want, statErr := os.Stat(path)
		if errors.Is(statErr, os.ErrNotExist) || errors.Is(statErr, syscall.ENOTDIR) {
			continue
		}
		if statErr != nil {
			t.Fatal(statErr)
		}
		for _, info := range openInfos {
			if os.SameFile(info, want) {
				snapshot.byPath[path]++
			}
		}
	}
	return snapshot
}

func assertWorkspaceAuthorityOpenDescriptorsUnchanged(t *testing.T, before workspaceAuthorityOpenDescriptorSnapshot, paths ...string) {
	t.Helper()
	after := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("workspace-authority descriptor leak\nbefore: %#v\nafter:  %#v", before, after)
	}
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

func testWorkspaceAuthorityIdentityFromOpened(t *testing.T, configuredWorkspace string, workspace *os.File) runtimeWorkspaceIdentity {
	t.Helper()
	info, err := workspace.Stat()
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("opened workspace stat type = %T, want *syscall.Stat_t", info.Sys())
	}
	resolvedPath, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", workspace.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	identity := runtimeWorkspaceIdentity{
		configuredPath: filepath.ToSlash(filepath.Clean(configuredWorkspace)),
		resolvedPath:   filepath.ToSlash(resolvedPath),
		device:         uint64(stat.Dev),
		inode:          stat.Ino,
	}
	identity.rootHash = testWorkspaceAuthoritySHA256(testWorkspaceAuthorityIdentityJCS(t, identity))
	return identity
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

func validateWorkspaceAuthorityTestPrivateNode(opened *os.File, expectedUID uint32) error {
	if opened == nil {
		return errRuntimeNoncanonical
	}
	info, err := opened.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != expectedUID {
		return errRuntimeIntegrityMismatch
	}
	if info.IsDir() {
		if !authorityPrivateModeIsExact(info.Mode(), authorityPrivateDirectoryMode) {
			return errRuntimeIntegrityMismatch
		}
		return nil
	}
	return validateAuthorityPrivateFile(opened, expectedUID)
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

type workspaceAuthorityTopologyEntry struct {
	mode   os.FileMode
	uid    uint32
	device uint64
	inode  uint64
	links  uint64
	size   int64
	mtime  int64
	ctime  int64
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
				mtime:  info.ModTime().UnixNano(),
				ctime:  stat.Ctim.Sec*int64(time.Second) + stat.Ctim.Nsec,
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
