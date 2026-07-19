package formations

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	testWorkspaceAuthorityRecoveryForeignID      = "wsa_01KXNP6VY3227H78329V52CKFB"
	testWorkspaceAuthorityRecoverySecondMatchID  = "wsa_01KXNP6VY3227H78329V52CKFC"
	testWorkspaceAuthorityRecoveryPreBootstrapID = "wsa_01KXNP6VY3227H78329V52CKFD"
)

type workspaceAuthorityRecoveryPrefix uint8

const (
	workspaceAuthorityRecoveryBootstrapOnly workspaceAuthorityRecoveryPrefix = iota
	workspaceAuthorityRecoveryOwnerLock
	workspaceAuthorityRecoveryPolicy
	workspaceAuthorityRecoveryWorkspace
)

func TestWorkspaceAuthorityCrashRecoveryCompletesSelectedExactPrefixes(t *testing.T) {
	tests := []struct {
		name          string
		prefix        workspaceAuthorityRecoveryPrefix
		durablePrefix bool
	}{
		{name: "bootstrap only", prefix: workspaceAuthorityRecoveryBootstrapOnly},
		{name: "owner lock already present", prefix: workspaceAuthorityRecoveryOwnerLock},
		{name: "disabled policy revision one already present", prefix: workspaceAuthorityRecoveryPolicy},
		{name: "workspace authority revision one already present", prefix: workspaceAuthorityRecoveryWorkspace},
		{name: "post authority sync before registry publication", prefix: workspaceAuthorityRecoveryWorkspace, durablePrefix: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			present := installWorkspaceAuthorityRecoveryPrefix(t, fixture, test.prefix)
			if test.durablePrefix {
				syncWorkspaceAuthorityRecoveryPrefix(t, fixture)
			}
			stable := snapshotWorkspaceAuthorityRecoveryFiles(t, present...)
			assertWorkspaceAuthorityRecoverySucceeds(t, fixture, test.prefix, stable)
		})
	}
}

func TestWorkspaceAuthorityCrashRecoveryIgnoresPreBootstrapAndCanonicalForeignRootSiblings(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	present := installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)

	preBootstrap := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryPreBootstrapID)
	createWorkspaceAuthorityRecoveryDirectory(t, preBootstrap)
	writePrivateAuthorityTestFile(t, filepath.Join(preBootstrap, "owner.lock"), nil)
	writePrivateAuthorityTestFile(t, filepath.Join(preBootstrap, ".workspace.private.json.stage-leftover"), []byte("non-authorizing pre-bootstrap bytes"))

	foreign := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryForeignID)
	createWorkspaceAuthorityRecoveryDirectory(t, foreign)
	writePrivateAuthorityTestFile(t, filepath.Join(foreign, "workspace.bootstrap.json"), workspaceAuthorityRecoveryBootstrapRaw(
		testWorkspaceAuthorityRecoveryForeignID,
		strings.Repeat("f", 64),
	))
	writePrivateAuthorityTestFile(t, filepath.Join(foreign, "owner.lock"), nil)

	siblingsBefore := snapshotWorkspaceAuthorityTopology(t, preBootstrap, foreign)
	stable := snapshotWorkspaceAuthorityRecoveryFiles(t, present...)
	assertWorkspaceAuthorityRecoverySucceeds(t, fixture, workspaceAuthorityRecoveryBootstrapOnly, stable)
	if after := snapshotWorkspaceAuthorityTopology(t, preBootstrap, foreign); !reflect.DeepEqual(after, siblingsBefore) {
		t.Fatalf("recovery changed nonauthorizing pre-bootstrap or canonical foreign-root sibling\nbefore: %#v\nafter:  %#v", siblingsBefore, after)
	}
}

func TestWorkspaceAuthorityCrashRecoveryRejectsTwoExactCurrentRootMatchesWithoutMutation(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
	second := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoverySecondMatchID)
	createWorkspaceAuthorityRecoveryDirectory(t, second)
	writePrivateAuthorityTestFile(t, filepath.Join(second, "workspace.bootstrap.json"), workspaceAuthorityRecoveryBootstrapRaw(
		testWorkspaceAuthorityRecoverySecondMatchID,
		fixture.identity.rootHash,
	))
	assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
}

func TestWorkspaceAuthorityCrashRecoveryRejectsUnclassifiableBootstrapWithoutMutation(t *testing.T) {
	canonical := func(authorityID, rootHash string) []byte {
		return workspaceAuthorityRecoveryBootstrapRaw(authorityID, rootHash)
	}
	tests := []struct {
		name string
		raw  func(workspaceAuthorityInitialRegistrationFixture) []byte
	}{
		{name: "malformed JSON", raw: func(workspaceAuthorityInitialRegistrationFixture) []byte { return []byte("{") }},
		{name: "noncanonical whitespace", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			return append([]byte(" "), canonical(testInitialRegistrationAuthorityID, f.identity.rootHash)...)
		}},
		{name: "trailing newline", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			return append(canonical(testInitialRegistrationAuthorityID, f.identity.rootHash), '\n')
		}},
		{name: "unknown key", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			return []byte(fmt.Sprintf(
				`{"bootstrapSchema":1,"mayRecover":true,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
				testInitialRegistrationAuthorityID,
				f.identity.rootHash,
			))
		}},
		{name: "duplicate key", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			return []byte(fmt.Sprintf(
				`{"bootstrapSchema":1,"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
				testInitialRegistrationAuthorityID,
				f.identity.rootHash,
			))
		}},
		{name: "unsupported schema", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			raw := canonical(testInitialRegistrationAuthorityID, f.identity.rootHash)
			return bytes.Replace(raw, []byte(`"bootstrapSchema":1`), []byte(`"bootstrapSchema":2`), 1)
		}},
		{name: "unknown root identity encoding", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			raw := canonical(testInitialRegistrationAuthorityID, f.identity.rootHash)
			return bytes.Replace(raw, []byte("workspace-root-identity-v1"), []byte("workspace-root-identity-v2"), 1)
		}},
		{name: "authority id conflicts with directory basename", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			return canonical(testWorkspaceAuthorityRecoveryForeignID, f.identity.rootHash)
		}},
		{name: "invalid root hash grammar", raw: func(workspaceAuthorityInitialRegistrationFixture) []byte {
			return canonical(testInitialRegistrationAuthorityID, strings.Repeat("g", 64))
		}},
		{name: "oversize bootstrap", raw: func(f workspaceAuthorityInitialRegistrationFixture) []byte {
			return append(canonical(testInitialRegistrationAuthorityID, f.identity.rootHash), bytes.Repeat([]byte("x"), int(runtimeAuthorityMaxRecordBytes))...)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			createWorkspaceAuthorityRecoveryDirectory(t, fixture.authorityDir)
			writePrivateAuthorityTestFile(t, fixture.bootstrap, test.raw(fixture))
			assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
		})
	}
}

func TestWorkspaceAuthorityCrashRecoveryRejectsConflictingPresentInitialBytesBeforeRepair(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, workspaceAuthorityInitialRegistrationFixture)
	}{
		{
			name: "nonempty owner lock",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
				writePrivateAuthorityTestFile(t, fixture.ownerLock, []byte("not an exact empty owner lock"))
			},
		},
		{
			name: "conflicting policy revision one",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryPolicy)
				writePrivateAuthorityTestFile(t, fixture.policy, append(append([]byte(nil), fixture.policyRaw...), '\n'))
			},
		},
		{
			name: "conflicting workspace revision one",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryWorkspace)
				conflicting := bytes.Replace(fixture.workspaceRaw, []byte(`"nextWriterFence":1`), []byte(`"nextWriterFence":2`), 1)
				writePrivateAuthorityTestFile(t, fixture.workspaceAuthority, conflicting)
			},
		},
		{
			name: "workspace conflict is preflighted before missing policy repair",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
				conflicting := bytes.Replace(fixture.workspaceRaw, []byte(`"nextAdmissionSeq":1`), []byte(`"nextAdmissionSeq":2`), 1)
				writePrivateAuthorityTestFile(t, fixture.workspaceAuthority, conflicting)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			test.setup(t, fixture)
			assertWorkspaceAuthorityRecoveryRejectsSelectedUnchanged(t, fixture, fixture.base)
		})
	}
}

func installWorkspaceAuthorityRecoveryPrefix(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, prefix workspaceAuthorityRecoveryPrefix) []string {
	t.Helper()
	createWorkspaceAuthorityRecoveryDirectory(t, fixture.authorityDir)
	writePrivateAuthorityTestFile(t, fixture.bootstrap, fixture.bootstrapRaw)
	present := []string{fixture.bootstrap}
	if prefix >= workspaceAuthorityRecoveryOwnerLock {
		writePrivateAuthorityTestFile(t, fixture.ownerLock, nil)
		present = append(present, fixture.ownerLock)
	}
	if prefix >= workspaceAuthorityRecoveryPolicy {
		createWorkspaceAuthorityRecoveryDirectory(t, fixture.policyDir)
		writePrivateAuthorityTestFile(t, fixture.policy, fixture.policyRaw)
		present = append(present, fixture.policy)
	}
	if prefix >= workspaceAuthorityRecoveryWorkspace {
		writePrivateAuthorityTestFile(t, fixture.workspaceAuthority, fixture.workspaceRaw)
		present = append(present, fixture.workspaceAuthority)
	}
	return present
}

func createWorkspaceAuthorityRecoveryDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, authorityPrivateDirectoryMode.Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, authorityPrivateDirectoryMode.Perm()); err != nil {
		t.Fatal(err)
	}
}

func workspaceAuthorityRecoveryBootstrapRaw(authorityID, rootHash string) []byte {
	return []byte(fmt.Sprintf(
		`{"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		authorityID,
		rootHash,
	))
}

func syncWorkspaceAuthorityRecoveryPrefix(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
	t.Helper()
	for _, path := range []string{fixture.bootstrap, fixture.ownerLock, fixture.policy, fixture.workspaceAuthority, fixture.policyDir, fixture.authorityDir} {
		opened, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := opened.Sync(); err != nil {
			opened.Close()
			t.Fatal(err)
		}
		if err := opened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func snapshotWorkspaceAuthorityRecoveryFiles(t *testing.T, paths ...string) map[string]workspaceAuthorityTopologyEntry {
	t.Helper()
	result := make(map[string]workspaceAuthorityTopologyEntry, len(paths))
	for _, path := range paths {
		result[path] = snapshotWorkspaceAuthorityTopology(t, path)[path]
	}
	return result
}

func assertWorkspaceAuthorityRecoveryFilesStable(t *testing.T, before map[string]workspaceAuthorityTopologyEntry) {
	t.Helper()
	for path, want := range before {
		got := snapshotWorkspaceAuthorityTopology(t, path)[path]
		if got != want {
			t.Fatalf("recovery replaced exact present file %q\n got: %+v\nwant: %+v", path, got, want)
		}
	}
}

func assertWorkspaceAuthorityRecoverySucceeds(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, prefix workspaceAuthorityRecoveryPrefix, stable map[string]workspaceAuthorityTopologyEntry) {
	t.Helper()
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	productionOps := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate()).ops
	generated := 0
	callbackCalls := 0
	syncCalls := 0
	steps := []string{}
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return "", errors.New("recovery selected the id generator instead of the exact bootstrap")
			},
			observeInitialRegistration: func(step string) error {
				steps = append(steps, step)
				assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
				if step != testInitialRegistrationRegistryPublished {
					assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "recovery step "+step)
				}
				switch step {
				case testInitialRegistrationOwnerLockAcquired,
					testInitialRegistrationPolicyPublished,
					testInitialRegistrationWorkspacePublished,
					testInitialRegistrationAuthorityDirectorySynced,
					testInitialRegistrationRegistryPublished:
					assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
				default:
					t.Fatalf("unexpected recovery step %q", step)
				}
				if step == testInitialRegistrationRegistryPublished {
					assertWorkspaceAuthorityRecoveryFinalState(t, fixture)
				}
				return nil
			},
			syncInitialAuthorityDirectory: func(directory *os.File) error {
				syncCalls++
				assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
				assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
				openedInfo, err := directory.Stat()
				if err != nil {
					t.Fatal(err)
				}
				namedInfo, err := os.Stat(fixture.authorityDir)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(openedInfo, namedInfo) {
					t.Fatalf("recovery synced %v, want pinned selected candidate %v", openedInfo, namedInfo)
				}
				assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "recovery authority-directory sync")
				return productionOps.syncInitialAuthorityDirectory(directory)
			},
		},
	)

	err := registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testInitialRegistrationAuthorityID)
		assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
		assertWorkspaceAuthorityRecoveryFinalState(t, fixture)
		return nil
	})
	if err != nil {
		t.Fatalf("recover exact workspace-authority prefix: %v", err)
	}
	if generated != 0 || callbackCalls != 1 || syncCalls != 1 {
		t.Fatalf("recovery generator/callback/authority-sync calls = %d/%d/%d, want 0/1/1", generated, callbackCalls, syncCalls)
	}
	assertWorkspaceAuthorityRecoveryStepOrder(t, prefix, steps)
	assertWorkspaceAuthorityRecoveryFilesStable(t, stable)
	assertWorkspaceAuthorityRecoveryFinalState(t, fixture)
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("successful recovery leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("successful recovery leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func assertWorkspaceAuthorityRecoveryStepOrder(t *testing.T, prefix workspaceAuthorityRecoveryPrefix, steps []string) {
	t.Helper()
	order := map[string]int{
		testInitialRegistrationOwnerLockAcquired:        0,
		testInitialRegistrationPolicyPublished:          1,
		testInitialRegistrationWorkspacePublished:       2,
		testInitialRegistrationAuthorityDirectorySynced: 3,
		testInitialRegistrationRegistryPublished:        4,
	}
	seen := map[string]bool{}
	previous := -1
	for _, step := range steps {
		position, ok := order[step]
		if !ok || seen[step] || position <= previous {
			t.Fatalf("recovery steps = %#v, want unique canonical transaction order", steps)
		}
		seen[step] = true
		previous = position
	}
	for _, required := range []string{
		testInitialRegistrationOwnerLockAcquired,
		testInitialRegistrationAuthorityDirectorySynced,
		testInitialRegistrationRegistryPublished,
	} {
		if !seen[required] {
			t.Fatalf("recovery steps = %#v, missing required boundary %q", steps, required)
		}
	}
	if prefix < workspaceAuthorityRecoveryPolicy && !seen[testInitialRegistrationPolicyPublished] {
		t.Fatalf("recovery steps = %#v, missing publication of absent policy", steps)
	}
	if prefix < workspaceAuthorityRecoveryWorkspace && !seen[testInitialRegistrationWorkspacePublished] {
		t.Fatalf("recovery steps = %#v, missing publication of absent workspace authority", steps)
	}
}

func assertWorkspaceAuthorityRecoveryFinalState(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
	t.Helper()
	assertWorkspaceAuthorityInitialRegistrationDirectory(t, fixture.authorityDir, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationDirectory(t, fixture.policyDir, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.bootstrap, fixture.bootstrapRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.ownerLock, nil, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.policy, fixture.policyRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.workspaceAuthority, fixture.workspaceRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.registry, fixture.finalRegistryRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t, fixture.authorityDir,
		"admission-policies",
		"owner.lock",
		"workspace.bootstrap.json",
		"workspace.private.json",
	)
	assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t, fixture.policyDir, "1.json")
}

func assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, roots ...string) {
	t.Helper()
	assertWorkspaceAuthorityRecoveryRejectsUnchanged(t, fixture, []string{}, roots...)
}

func assertWorkspaceAuthorityRecoveryRejectsSelectedUnchanged(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, roots ...string) {
	t.Helper()
	assertWorkspaceAuthorityRecoveryRejectsUnchanged(t, fixture, []string{testInitialRegistrationOwnerLockAcquired}, roots...)
}

func assertWorkspaceAuthorityRecoveryRejectsUnchanged(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, wantSteps []string, roots ...string) {
	t.Helper()
	before := snapshotWorkspaceAuthorityTopology(t, roots...)
	paths := append(workspaceAuthorityInitialRegistrationPaths(fixture), roots...)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	generated := 0
	steps := []string{}
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
			observeInitialRegistration: func(step string) error {
				steps = append(steps, step)
				assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
				if step == testInitialRegistrationOwnerLockAcquired {
					assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
				}
				return nil
			},
		},
	)
	err := registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if err == nil {
		t.Fatal("unsafe or ambiguous recovery candidate was accepted")
	}
	if generated != 0 || callbackCalls != 0 || !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("rejected recovery generator/steps/callback = %d/%#v/%d, want 0/%#v/0", generated, steps, callbackCalls, wantSteps)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, roots...); !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected recovery changed registry predecessor or candidate topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("rejected recovery leaked registry lock: %v", err)
	}
	if info, err := os.Lstat(fixture.ownerLock); err == nil && info.Mode().IsRegular() {
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
			t.Fatalf("rejected recovery leaked owner lock: %v", err)
		}
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}
