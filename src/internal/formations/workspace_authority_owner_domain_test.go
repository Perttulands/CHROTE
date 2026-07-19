package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestWorkspaceAuthorityOwnerDomainScopeIsClosedValueOnly(t *testing.T) {
	scopeType := reflect.TypeOf((*workspaceAuthorityOwnerDomainScope)(nil)).Elem()
	if scopeType.Kind() != reflect.Interface || scopeType.Name() != "workspaceAuthorityOwnerDomainScope" || scopeType.PkgPath() == "" {
		t.Fatalf("owner-domain scope type = %v, want private named interface", scopeType)
	}
	wantMethods := map[string][]reflect.Kind{
		"ownerLockIdentity":    {reflect.Uint64, reflect.Uint64},
		"workspaceAuthorityID": {reflect.String},
		"workspaceIdentity":    {reflect.Uint64, reflect.Uint64, reflect.String},
	}
	if scopeType.NumMethod() != len(wantMethods) {
		t.Fatalf("owner-domain scope methods = %d, want exact closed set %d", scopeType.NumMethod(), len(wantMethods))
	}
	for index := 0; index < scopeType.NumMethod(); index++ {
		method := scopeType.Method(index)
		want, ok := wantMethods[method.Name]
		if !ok {
			t.Fatalf("owner-domain scope exposes unexpected method %q", method.Name)
		}
		if method.Type.NumIn() != 0 || method.Type.NumOut() != len(want) {
			t.Fatalf("owner-domain scope method %s signature = %v, want no inputs and %d closed outputs", method.Name, method.Type, len(want))
		}
		for outputIndex, wantKind := range want {
			if gotKind := method.Type.Out(outputIndex).Kind(); gotKind != wantKind {
				t.Fatalf("owner-domain scope method %s output %d kind = %s, want %s", method.Name, outputIndex, gotKind, wantKind)
			}
		}
	}
}

func TestWorkspaceAuthorityOwnerDomainUsesExactMappedIdentityAndStaysReadOnly(t *testing.T) {
	fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
	decoyID := testAuthorityRecordWorkspaceID2
	decoyDirectory := filepath.Join(fixture.workspacesRoot, decoyID)
	if err := os.Mkdir(decoyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(decoyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	decoyOwnerLock := filepath.Join(decoyDirectory, "owner.lock")
	writePrivateAuthorityTestFile(t, decoyOwnerLock, nil)

	paths := workspaceAuthorityOwnerDomainPaths(fixture)
	paths = append(paths, decoyDirectory, decoyOwnerLock)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate())
	callbackCalls := 0
	err := registrar.withWorkspaceAuthorityOwnerDomain(fixture.workspace, func(scope workspaceAuthorityOwnerDomainScope) error {
		callbackCalls++
		if scope == nil {
			t.Fatal("owner-domain callback received nil scope")
		}
		if got := scope.workspaceAuthorityID(); got != testInitialRegistrationAuthorityID {
			t.Fatalf("owner-domain authority id = %q, want mapped id %q", got, testInitialRegistrationAuthorityID)
		}
		workspaceDevice, workspaceInode, rootHash := scope.workspaceIdentity()
		if workspaceDevice != fixture.identity.device || workspaceInode != fixture.identity.inode || rootHash != fixture.identity.rootHash {
			t.Fatalf("owner-domain workspace identity = (%d,%d,%q), want (%d,%d,%q)", workspaceDevice, workspaceInode, rootHash, fixture.identity.device, fixture.identity.inode, fixture.identity.rootHash)
		}
		ownerDevice, ownerInode := scope.ownerLockIdentity()
		wantOwner := workspaceAuthorityLockIdentityAtPath(t, fixture.ownerLock)
		if ownerDevice != wantOwner.device || ownerInode != wantOwner.inode {
			t.Fatalf("owner-domain lock identity = (%d,%d), want (%d,%d)", ownerDevice, ownerInode, wantOwner.device, wantOwner.inode)
		}
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
			t.Fatalf("registry lock during owner-domain callback = %v, want released", err)
		}
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); !errors.Is(err, syscall.EWOULDBLOCK) {
			t.Fatalf("mapped owner lock during owner-domain callback = %v, want would-block", err)
		}
		if err := tryWorkspaceAuthorityExclusiveLock(decoyOwnerLock); err != nil {
			t.Fatalf("unmapped decoy owner lock during callback = %v, want unlocked", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("validate mapped workspace owner domain: %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("owner-domain callback calls = %d, want exactly one", callbackCalls)
	}
	assertWorkspaceAuthorityOwnerDomainUnchanged(t, fixture, before)
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
	assertWorkspaceAuthorityOwnerDomainLocksReleased(t, fixture)
}

func TestWorkspaceAuthorityOwnerDomainCapabilityPairRejectsBeforeOwnerPathSelection(t *testing.T) {
	fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, workspaceAuthorityOwnerDomainPaths(fixture)...)
	gate := newWorkspaceAuthorityCapabilityGate()
	gate.capabilities = cloneWorkspaceAuthorityCapabilities(gate.capabilities[:1])
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, gate)
	productionValidate := registrar.ops.validatePrivateNode
	validationCalls := 0
	registrar.ops.validatePrivateNode = func(opened *os.File, expectedUID uint32) error {
		validationCalls++
		return productionValidate(opened, expectedUID)
	}
	callbackCalls := 0
	err := registrar.withWorkspaceAuthorityOwnerDomain(fixture.workspace, func(workspaceAuthorityOwnerDomainScope) error {
		callbackCalls++
		return nil
	})
	if !errors.Is(err, errRuntimeUnsupportedSchema) {
		t.Fatalf("incomplete capability pair owner-domain error = %v, want unsupported schema", err)
	}
	if validationCalls != 0 || callbackCalls != 0 {
		t.Fatalf("capability rejection private-node/callback calls = %d/%d, want 0/0 before owner path selection", validationCalls, callbackCalls)
	}
	assertWorkspaceAuthorityOwnerDomainUnchanged(t, fixture, before)
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, workspaceAuthorityOwnerDomainPaths(fixture)...)
	assertWorkspaceAuthorityOwnerDomainLocksReleased(t, fixture)
}

func TestWorkspaceAuthorityOwnerDomainUsesRegistryMappingWithoutDirectoryFallback(t *testing.T) {
	t.Run("missing mapping", func(t *testing.T) {
		fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
		emptyRaw, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryJCSV1{
			Entries:        []workspaceRegistryEntryJCSV1{},
			RecordRev:      1,
			RegistrySchema: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		writePrivateAuthorityTestFile(t, fixture.registry, emptyRaw)
		assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
	})

	t.Run("mapped id has no private state", func(t *testing.T) {
		fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
		mappedID := testAuthorityRecordWorkspaceID2
		mappedDirectory := filepath.Join(fixture.workspacesRoot, mappedID)
		if err := os.Mkdir(mappedDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(mappedDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		writePrivateAuthorityTestFile(t, filepath.Join(mappedDirectory, "owner.lock"), nil)
		mappedRegistry, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryWithIdentity(fixture.identity, mappedID))
		if err != nil {
			t.Fatal(err)
		}
		writePrivateAuthorityTestFile(t, fixture.registry, mappedRegistry)
		assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
	})
}

func TestWorkspaceAuthorityOwnerDomainRejectsMissingClosedStateWithoutRepair(t *testing.T) {
	tests := []struct {
		name   string
		remove func(workspaceAuthorityOwnerDomainFixture) error
	}{
		{name: "registry lock", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error { return os.Remove(fixture.registryLock) }},
		{name: "private registry", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error { return os.Remove(fixture.registry) }},
		{name: "authority directory", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error { return os.RemoveAll(fixture.authorityDir) }},
		{name: "owner lock", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error { return os.Remove(fixture.ownerLock) }},
		{name: "bootstrap", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error { return os.Remove(fixture.bootstrap) }},
		{name: "workspace authority", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error { return os.Remove(fixture.workspaceAuthority) }},
		{name: "policy directory", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error { return os.RemoveAll(fixture.policyDir) }},
		{name: "current policy", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error {
			return os.Remove(filepath.Join(fixture.policyDir, "3.json"))
		}},
		{name: "prior policy revision 2", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error {
			return os.Remove(filepath.Join(fixture.policyDir, "2.json"))
		}},
		{name: "prior policy revision 1", remove: func(fixture workspaceAuthorityOwnerDomainFixture) error {
			return os.Remove(filepath.Join(fixture.policyDir, "1.json"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
			if err := test.remove(fixture); err != nil {
				t.Fatal(err)
			}
			assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
		})
	}
}

func TestWorkspaceAuthorityOwnerDomainRejectsUnsafePrivateTopologyWithoutRepair(t *testing.T) {
	t.Run("symlinks", func(t *testing.T) {
		tests := []struct {
			name       string
			target     func(workspaceAuthorityOwnerDomainFixture) string
			linkTarget func(workspaceAuthorityOwnerDomainFixture) string
		}{
			{name: "authority directory", target: func(f workspaceAuthorityOwnerDomainFixture) string { return f.authorityDir }, linkTarget: func(f workspaceAuthorityOwnerDomainFixture) string { return f.workspace }},
			{name: "owner lock", target: func(f workspaceAuthorityOwnerDomainFixture) string { return f.ownerLock }, linkTarget: func(f workspaceAuthorityOwnerDomainFixture) string { return f.registryLock }},
			{name: "bootstrap", target: func(f workspaceAuthorityOwnerDomainFixture) string { return f.bootstrap }, linkTarget: func(f workspaceAuthorityOwnerDomainFixture) string { return f.registry }},
			{name: "workspace authority", target: func(f workspaceAuthorityOwnerDomainFixture) string { return f.workspaceAuthority }, linkTarget: func(f workspaceAuthorityOwnerDomainFixture) string { return f.registry }},
			{name: "policy directory", target: func(f workspaceAuthorityOwnerDomainFixture) string { return f.policyDir }, linkTarget: func(f workspaceAuthorityOwnerDomainFixture) string { return f.workspace }},
			{name: "current policy", target: func(f workspaceAuthorityOwnerDomainFixture) string { return filepath.Join(f.policyDir, "3.json") }, linkTarget: func(f workspaceAuthorityOwnerDomainFixture) string { return f.registry }},
			{name: "prior policy", target: func(f workspaceAuthorityOwnerDomainFixture) string { return filepath.Join(f.policyDir, "2.json") }, linkTarget: func(f workspaceAuthorityOwnerDomainFixture) string { return f.registry }},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
				path := test.target(fixture)
				if info, err := os.Lstat(path); err != nil {
					t.Fatal(err)
				} else if info.IsDir() {
					if err := os.RemoveAll(path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(test.linkTarget(fixture), path); err != nil {
					t.Fatal(err)
				}
				assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
			})
		}
	})

	t.Run("hard links", func(t *testing.T) {
		tests := []struct {
			name string
			path func(workspaceAuthorityOwnerDomainFixture) string
		}{
			{name: "owner lock", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.ownerLock }},
			{name: "bootstrap", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.bootstrap }},
			{name: "workspace authority", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.workspaceAuthority }},
			{name: "current policy", path: func(f workspaceAuthorityOwnerDomainFixture) string { return filepath.Join(f.policyDir, "3.json") }},
			{name: "prior policy", path: func(f workspaceAuthorityOwnerDomainFixture) string { return filepath.Join(f.policyDir, "2.json") }},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
				path := test.path(fixture)
				if err := os.Link(path, path+".second-link"); err != nil {
					t.Fatal(err)
				}
				assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
			})
		}
	})

	t.Run("wrong modes", func(t *testing.T) {
		tests := []struct {
			name string
			path func(workspaceAuthorityOwnerDomainFixture) string
			mode os.FileMode
		}{
			{name: "host root", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.hostRoot }, mode: 0o755},
			{name: "workspaces root", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.workspacesRoot }, mode: 0o755},
			{name: "registry lock", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.registryLock }, mode: 0o640},
			{name: "private registry", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.registry }, mode: 0o640},
			{name: "authority directory", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.authorityDir }, mode: 0o755},
			{name: "owner lock", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.ownerLock }, mode: 0o640},
			{name: "bootstrap", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.bootstrap }, mode: 0o640},
			{name: "workspace authority", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.workspaceAuthority }, mode: 0o640},
			{name: "policy directory", path: func(f workspaceAuthorityOwnerDomainFixture) string { return f.policyDir }, mode: 0o755},
			{name: "current policy", path: func(f workspaceAuthorityOwnerDomainFixture) string { return filepath.Join(f.policyDir, "3.json") }, mode: 0o640},
			{name: "prior policy", path: func(f workspaceAuthorityOwnerDomainFixture) string { return filepath.Join(f.policyDir, "2.json") }, mode: 0o640},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
				if err := os.Chmod(test.path(fixture), test.mode); err != nil {
					t.Fatal(err)
				}
				assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
			})
		}
	})

	t.Run("wrong writer uid", func(t *testing.T) {
		fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
		paths := workspaceAuthorityOwnerDomainPaths(fixture)
		before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
		descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
		registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID^1, newWorkspaceAuthorityCapabilityGate())
		callbackCalls := 0
		err := registrar.withWorkspaceAuthorityOwnerDomain(fixture.workspace, func(workspaceAuthorityOwnerDomainScope) error {
			callbackCalls++
			return nil
		})
		if err == nil || callbackCalls != 0 {
			t.Fatalf("wrong-uid owner-domain result = %v, callback calls %d; want fail before callback", err, callbackCalls)
		}
		assertWorkspaceAuthorityOwnerDomainUnchanged(t, fixture, before)
		assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
	})

	t.Run("nonempty owner lock", func(t *testing.T) {
		fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
		writePrivateAuthorityTestFile(t, fixture.ownerLock, []byte("not-lock-state"))
		assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
	})
}

func TestWorkspaceAuthorityOwnerDomainStrictValidatesBootstrapWorkspaceAndCompletePolicyChain(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, workspaceAuthorityOwnerDomainFixture)
	}{
		{name: "bootstrap unsupported schema", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.bootstrap, `"bootstrapSchema":1`, `"bootstrapSchema":2`)
		}},
		{name: "bootstrap wrong authority id", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.bootstrap, testInitialRegistrationAuthorityID, testAuthorityRecordWorkspaceID2)
		}},
		{name: "bootstrap wrong root hash", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.bootstrap, f.identity.rootHash, strings.Repeat("b", 64))
		}},
		{name: "bootstrap noncanonical bytes", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.bootstrap, `{"bootstrapSchema":1`, `{ "bootstrapSchema":1`)
		}},
		{name: "workspace unsupported schema", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.workspaceAuthority, `"authoritySchema":2`, `"authoritySchema":3`)
		}},
		{name: "workspace wrong authority id", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.workspaceAuthority, testInitialRegistrationAuthorityID, testAuthorityRecordWorkspaceID2)
		}},
		{name: "workspace wrong root hash", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.workspaceAuthority, f.identity.rootHash, strings.Repeat("b", 64))
		}},
		{name: "workspace missing prior generation", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			raw := readWorkspaceAuthorityInitialRegistrationFile(t, f.workspaceAuthority)
			start := strings.Index(string(raw), `,"priorGeneration":`)
			end := strings.Index(string(raw), `,"recordRev":`)
			if start < 0 || end <= start {
				t.Fatalf("workspace fixture missing prior-generation field: %s", raw)
			}
			writePrivateAuthorityTestFile(t, f.workspaceAuthority, append(append([]byte(nil), raw[:start]...), raw[end:]...))
		}},
		{name: "workspace current policy revision missing", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.workspaceAuthority, `"policyRev":3`, `"policyRev":4`)
		}},
		{name: "workspace current policy hash mismatch", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, f.workspaceAuthority, runtimeSHA256Hex(f.policyRaw[3]), strings.Repeat("b", 64))
		}},
		{name: "current policy unsupported schema", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "3.json"), `"policySchema":1`, `"policySchema":2`)
		}},
		{name: "current policy revision mismatch", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "3.json"), `"policyRev":3`, `"policyRev":2`)
		}},
		{name: "current policy hash changed", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "3.json"), `"state":"disabled"`, `"state":"configured"`)
		}},
		{name: "current to prior hash discontinuity", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "3.json"), runtimeSHA256Hex(f.policyRaw[2]), strings.Repeat("b", 64))
		}},
		{name: "prior revision 2 unsupported schema", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "2.json"), `"policySchema":1`, `"policySchema":2`)
		}},
		{name: "prior revision 2 discontinuity", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "2.json"), runtimeSHA256Hex(f.policyRaw[1]), strings.Repeat("b", 64))
		}},
		{name: "revision 1 has predecessor", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "1.json"), `"priorPolicySha256":""`, `"priorPolicySha256":"`+strings.Repeat("b", 64)+`"`)
		}},
		{name: "prior policy noncanonical bytes", mutate: func(t *testing.T, f workspaceAuthorityOwnerDomainFixture) {
			replaceWorkspaceAuthorityOwnerDomainRaw(t, filepath.Join(f.policyDir, "2.json"), `{"maxActiveRuns":2`, `{ "maxActiveRuns":2`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityOwnerDomainFixture(t)
			test.mutate(t, fixture)
			assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t, fixture, newWorkspaceAuthorityCapabilityGate())
		})
	}
}

type workspaceAuthorityOwnerDomainFixture struct {
	workspaceAuthorityInitialRegistrationFixture
	policyRaw map[uint64][]byte
}

func newWorkspaceAuthorityOwnerDomainFixture(t *testing.T) workspaceAuthorityOwnerDomainFixture {
	t.Helper()
	initial := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityInitialRegistrationPrivateState(t, initial)
	writePrivateAuthorityTestFile(t, initial.registry, initial.mappedRegistryRaw)

	policies := map[uint64][]byte{1: append([]byte(nil), initial.policyRaw...)}
	policies[2] = []byte(fmt.Sprintf(
		`{"maxActiveRuns":2,"maxQueuedRuns":3,"policyRev":2,"policySchema":1,"priorPolicySha256":"%s","state":"configured"}`,
		runtimeSHA256Hex(policies[1]),
	))
	policies[3] = []byte(fmt.Sprintf(
		`{"policyRev":3,"policySchema":1,"priorPolicySha256":"%s","state":"disabled"}`,
		runtimeSHA256Hex(policies[2]),
	))
	for revision := uint64(2); revision <= 3; revision++ {
		writePrivateAuthorityTestFile(t, filepath.Join(initial.policyDir, fmt.Sprintf("%d.json", revision)), policies[revision])
	}
	priorWorkspace := append([]byte(nil), initial.workspaceRaw...)
	currentWorkspace := workspaceAuthorityJCSV1{
		AdmissionPolicyRef: workspaceAdmissionPolicyRefJCSV1{
			PolicyRev:    3,
			PolicySHA256: runtimeSHA256Hex(policies[3]),
		},
		AuthoritySchema:             2,
		NextAdmissionSeq:            11,
		NextWriterFence:             7,
		PriorGeneration:             &authorityGeneration{recordRev: 1, sha256: runtimeSHA256Hex(priorWorkspace)},
		RecordRev:                   2,
		RootIdentityEncoding:        "workspace-root-identity-v1",
		WorkspaceAuthorityID:        testInitialRegistrationAuthorityID,
		WorkspaceRootIdentitySHA256: initial.identity.rootHash,
	}
	currentRaw, err := encodeWorkspaceAuthorityJCSV1(currentWorkspace)
	if err != nil {
		t.Fatalf("encode owner-domain workspace authority: %v", err)
	}
	writePrivateAuthorityTestFile(t, initial.workspaceAuthority, currentRaw)
	initial.workspaceRaw = currentRaw
	return workspaceAuthorityOwnerDomainFixture{
		workspaceAuthorityInitialRegistrationFixture: initial,
		policyRaw: policies,
	}
}

func workspaceAuthorityOwnerDomainPaths(fixture workspaceAuthorityOwnerDomainFixture) []string {
	paths := workspaceAuthorityInitialRegistrationPaths(fixture.workspaceAuthorityInitialRegistrationFixture)
	paths = append(paths,
		filepath.Join(fixture.policyDir, "2.json"),
		filepath.Join(fixture.policyDir, "3.json"),
	)
	return paths
}

func assertWorkspaceAuthorityOwnerDomainRejectedReadOnly(t *testing.T, fixture workspaceAuthorityOwnerDomainFixture, gate workspaceAuthorityCapabilityGate) {
	t.Helper()
	paths := workspaceAuthorityOwnerDomainPaths(fixture)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	callbackCalls := 0
	registrar := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, gate)
	err := registrar.withWorkspaceAuthorityOwnerDomain(fixture.workspace, func(workspaceAuthorityOwnerDomainScope) error {
		callbackCalls++
		return nil
	})
	if err == nil {
		t.Fatal("invalid workspace owner domain reached callback")
	}
	if callbackCalls != 0 {
		t.Fatalf("invalid workspace owner-domain callback calls = %d, want zero", callbackCalls)
	}
	assertWorkspaceAuthorityOwnerDomainUnchanged(t, fixture, before)
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
	assertWorkspaceAuthorityOwnerDomainLocksReleased(t, fixture)
}

func assertWorkspaceAuthorityOwnerDomainUnchanged(t *testing.T, fixture workspaceAuthorityOwnerDomainFixture, before map[string]workspaceAuthorityTopologyEntry) {
	t.Helper()
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("read-only owner-domain operation changed authority state\nbefore: %#v\nafter:  %#v", before, after)
	}
	if _, err := os.Lstat(filepath.Join(fixture.authorityDir, "owner.private.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only owner-domain operation created owner.private.json: %v", err)
	}
	err := filepath.WalkDir(fixture.base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasPrefix(entry.Name(), ".authority-stage-") {
			return fmt.Errorf("unexpected authority staging path %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read-only owner-domain staging scan: %v", err)
	}
}

func assertWorkspaceAuthorityOwnerDomainLocksReleased(t *testing.T, fixture workspaceAuthorityOwnerDomainFixture) {
	t.Helper()
	for _, lockPath := range []string{fixture.registryLock, fixture.ownerLock} {
		info, err := os.Lstat(lockPath)
		if errors.Is(err, os.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
			continue
		}
		if err != nil {
			t.Fatalf("inspect owner-domain lock %q after operation: %v", lockPath, err)
		}
		if err := tryWorkspaceAuthorityExclusiveLock(lockPath); err != nil {
			t.Fatalf("owner-domain operation leaked lock %q: %v", lockPath, err)
		}
	}
}

func replaceWorkspaceAuthorityOwnerDomainRaw(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw := readWorkspaceAuthorityInitialRegistrationFile(t, path)
	if count := strings.Count(string(raw), old); count != 1 {
		t.Fatalf("owner-domain fixture occurrence count for %q in %q = %d, want exactly one", old, raw, count)
	}
	writePrivateAuthorityTestFile(t, path, []byte(strings.Replace(string(raw), old, replacement, 1)))
}
