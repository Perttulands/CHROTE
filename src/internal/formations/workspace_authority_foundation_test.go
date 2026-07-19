package formations

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkspaceAuthorityCapabilityRegistryIsExactCodeOwnedOrderedPair(t *testing.T) {
	descriptorType := reflect.TypeOf(workspaceAuthorityCapability{})
	for index := 0; index < descriptorType.NumField(); index++ {
		field := descriptorType.Field(index)
		if field.PkgPath == "" || field.Tag.Get("json") != "" {
			t.Fatalf("capability descriptor field %s is public or persisted: %+v", field.Name, field)
		}
	}
	want := []workspaceAuthorityCapability{
		{
			id: RuntimeAuthorityGuardCapabilityV1,
		},
		{
			id:                 "formations.workspace-authority.v1",
			registration:       true,
			privatePublication: true,
			ownerLease:         true,
			fencing:            true,
			commandJournal:     true,
		},
	}
	gate := newWorkspaceAuthorityCapabilityGate()
	if !reflect.DeepEqual(gate.capabilities, want) {
		t.Fatalf("code-owned workspace authority capabilities = %#v, want exact ordered pair %#v", gate.capabilities, want)
	}
	if err := gate.beforeMutation(func() error { return nil }); err != nil {
		t.Fatalf("exact code-owned capability pair rejected before foundation callback: %v", err)
	}
	gate.capabilities[0].id = "mutated-test-copy"
	if fresh := newWorkspaceAuthorityCapabilityGate().capabilities; !reflect.DeepEqual(fresh, want) {
		t.Fatalf("mutating one capability snapshot changed code-owned registry: %#v", fresh)
	}
}

func TestWorkspaceAuthorityCapabilityGateRejectsBeforeMutation(t *testing.T) {
	canonical := cloneWorkspaceAuthorityCapabilities(newWorkspaceAuthorityCapabilityGate().capabilities)
	tests := []struct {
		name         string
		capabilities []workspaceAuthorityCapability
	}{
		{name: "missing read guard", capabilities: cloneWorkspaceAuthorityCapabilities(canonical[1:])},
		{name: "missing workspace authority", capabilities: cloneWorkspaceAuthorityCapabilities(canonical[:1])},
		{name: "duplicate capability", capabilities: append(cloneWorkspaceAuthorityCapabilities(canonical), canonical[1])},
		{name: "unknown capability", capabilities: append(cloneWorkspaceAuthorityCapabilities(canonical), workspaceAuthorityCapability{id: "formations.unknown.v1"})},
		{name: "wrong order", capabilities: []workspaceAuthorityCapability{canonical[1], canonical[0]}},
		{name: "read guard becomes authorizing", capabilities: mutateWorkspaceAuthorityCapability(canonical, 0, func(capability *workspaceAuthorityCapability) { capability.fencing = true })},
		{name: "workspace authority omits command journal", capabilities: mutateWorkspaceAuthorityCapability(canonical, 1, func(capability *workspaceAuthorityCapability) { capability.commandJournal = false })},
		{name: "workspace authority gains semantic projection", capabilities: mutateWorkspaceAuthorityCapability(canonical, 1, func(capability *workspaceAuthorityCapability) { capability.semanticProjection = true })},
		{name: "workspace authority gains reconciliation", capabilities: mutateWorkspaceAuthorityCapability(canonical, 1, func(capability *workspaceAuthorityCapability) { capability.reconciliation = true })},
		{name: "workspace authority gains cleanup", capabilities: mutateWorkspaceAuthorityCapability(canonical, 1, func(capability *workspaceAuthorityCapability) { capability.cleanup = true })},
		{name: "workspace authority gains quarantine", capabilities: mutateWorkspaceAuthorityCapability(canonical, 1, func(capability *workspaceAuthorityCapability) { capability.quarantine = true })},
		{name: "workspace authority gains execution", capabilities: mutateWorkspaceAuthorityCapability(canonical, 1, func(capability *workspaceAuthorityCapability) { capability.execution = true })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			before := snapshotRuntimeAuthorityFixture(t, root)
			mutationCalls := 0
			gate := workspaceAuthorityCapabilityGate{capabilities: test.capabilities}
			err := gate.beforeMutation(func() error {
				mutationCalls++
				return os.WriteFile(filepath.Join(root, "owner.lock"), []byte("selected"), 0o600)
			})
			if err == nil {
				t.Fatal("noncanonical capability registry reached authority mutation")
			}
			if mutationCalls != 0 {
				t.Fatalf("authority mutation calls = %d, want zero", mutationCalls)
			}
			if got := snapshotRuntimeAuthorityFixture(t, root); !reflect.DeepEqual(got, before) {
				t.Fatalf("capability rejection changed authority root\nbefore: %#v\nafter:  %#v", before, got)
			}
		})
	}
}

func TestProductionAuthorityWriterUIDUsesLiveEffectiveUID(t *testing.T) {
	if got, want := productionAuthorityWriterUID(), uint32(os.Geteuid()); got != want {
		t.Fatalf("production authority writer uid = %d, want live effective uid %d", got, want)
	}
}

func TestAuthorityWriterUIDMatchesOpenedRootDirectoriesFilesAndLocks(t *testing.T) {
	expectedUID := productionAuthorityWriterUID()
	hostRoot := t.TempDir()
	workspacesRoot := filepath.Join(hostRoot, "workspaces")
	privateRoot := filepath.Join(workspacesRoot, testWorkspaceAuthorityID)
	for _, path := range []string{hostRoot, workspacesRoot, privateRoot} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(workspacesRoot, "registry.lock"),
		filepath.Join(workspacesRoot, "registry.private.json"),
		filepath.Join(privateRoot, "owner.lock"),
		filepath.Join(privateRoot, "workspace.private.json"),
	} {
		if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotRuntimeAuthorityFixture(t, hostRoot)

	wrongUID := expectedUID ^ 1
	for _, path := range []string{hostRoot, workspacesRoot, privateRoot} {
		directory, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := newAuthorityPublisher(directory, expectedUID, nil); err != nil {
			directory.Close()
			t.Fatalf("validate opened private directory %q: %v", path, err)
		}
		if _, err := newAuthorityPublisher(directory, wrongUID, nil); err == nil {
			directory.Close()
			t.Fatalf("opened private directory %q accepted non-owner uid %d", path, wrongUID)
		}
		if err := directory.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(workspacesRoot, "registry.lock"),
		filepath.Join(workspacesRoot, "registry.private.json"),
		filepath.Join(privateRoot, "owner.lock"),
		filepath.Join(privateRoot, "workspace.private.json"),
	} {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateAuthorityPrivateFile(file, expectedUID); err != nil {
			file.Close()
			t.Fatalf("validate opened private file %q: %v", path, err)
		}
		if err := validateAuthorityPrivateFile(file, wrongUID); err == nil {
			file.Close()
			t.Fatalf("opened private file %q accepted non-owner uid %d", path, wrongUID)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if got := snapshotRuntimeAuthorityFixture(t, hostRoot); !reflect.DeepEqual(got, before) {
		t.Fatalf("writer identity checks changed private authority state\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func cloneWorkspaceAuthorityCapabilities(source []workspaceAuthorityCapability) []workspaceAuthorityCapability {
	return append([]workspaceAuthorityCapability(nil), source...)
}

func mutateWorkspaceAuthorityCapability(source []workspaceAuthorityCapability, index int, mutate func(*workspaceAuthorityCapability)) []workspaceAuthorityCapability {
	result := cloneWorkspaceAuthorityCapabilities(source)
	mutate(&result[index])
	return result
}
