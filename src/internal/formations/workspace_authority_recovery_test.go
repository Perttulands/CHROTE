package formations

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const (
	testWorkspaceAuthorityRecoveryForeignID      = "wsa_01KXNP6VY3227H78329V52CKFB"
	testWorkspaceAuthorityRecoverySecondMatchID  = "wsa_01KXNP6VY3227H78329V52CKFC"
	testWorkspaceAuthorityRecoveryPreBootstrapID = "wsa_01KXNP6VY3227H78329V52CKFD"
	testWorkspaceAuthorityRecoveryFreshID        = "wsa_01KXNP6VY3227H78329V52CKFE"
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

func TestWorkspaceAuthorityCrashRecoveryExcludesRegisteredMalformedSiblingBeforeOrphanClassification(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	registeredEntry := workspaceRegistryEntryJCSV1{
		ConfiguredPath:              "/registered/foreign",
		Device:                      strconv.FormatUint(^uint64(0), 10),
		Inode:                       "1",
		WorkspaceAuthorityID:        testWorkspaceAuthorityRecoveryForeignID,
		WorkspaceRootIdentitySHA256: strings.Repeat("f", 64),
	}
	initialRegistry, err := decodeWorkspaceRegistryJCSV1(fixture.initialRegistryRaw)
	if err != nil {
		t.Fatal(err)
	}
	initialRegistry.Entries = append(initialRegistry.Entries, registeredEntry)
	initialRaw, err := encodeWorkspaceRegistryJCSV1(initialRegistry)
	if err != nil {
		t.Fatal(err)
	}
	writePrivateAuthorityTestFile(t, fixture.registry, initialRaw)
	fixture.initialRegistryRaw = initialRaw
	fixture.initialRegistryNode = snapshotWorkspaceAuthorityTopology(t, fixture.registry)[fixture.registry]

	finalRegistry, err := decodeWorkspaceRegistryJCSV1(fixture.finalRegistryRaw)
	if err != nil {
		t.Fatal(err)
	}
	finalRegistry.Entries = append(finalRegistry.Entries, registeredEntry)
	fixture.finalRegistryRaw = workspaceAuthorityInitialRegistrationRegistryRaw(
		t,
		finalRegistry.Entries,
		finalRegistry.RecordRev,
		&authorityGeneration{recordRev: initialRegistry.RecordRev, sha256: testWorkspaceAuthoritySHA256(initialRaw)},
	)

	registeredSibling := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryForeignID)
	createWorkspaceAuthorityRecoveryDirectory(t, registeredSibling)
	writePrivateAuthorityTestFile(t, filepath.Join(registeredSibling, "workspace.bootstrap.json"), []byte("{"))
	registeredBefore := snapshotWorkspaceAuthorityTopology(t, registeredSibling)
	present := installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
	stable := snapshotWorkspaceAuthorityRecoveryFiles(t, present...)

	assertWorkspaceAuthorityRecoverySucceeds(t, fixture, workspaceAuthorityRecoveryBootstrapOnly, stable)
	if after := snapshotWorkspaceAuthorityTopology(t, registeredSibling); !reflect.DeepEqual(after, registeredBefore) {
		t.Fatalf("orphan recovery changed registered malformed sibling\nbefore: %#v\nafter:  %#v", registeredBefore, after)
	}
}

func TestWorkspaceAuthorityCrashRecoverySafeDecoysPermitFreshRegistration(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	preBootstrap := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryPreBootstrapID)
	createWorkspaceAuthorityRecoveryDirectory(t, preBootstrap)
	writePrivateAuthorityTestFile(t, filepath.Join(preBootstrap, "owner.lock"), nil)
	foreign := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryForeignID)
	createWorkspaceAuthorityRecoveryDirectory(t, foreign)
	writePrivateAuthorityTestFile(t, filepath.Join(foreign, "workspace.bootstrap.json"), workspaceAuthorityRecoveryBootstrapRaw(
		testWorkspaceAuthorityRecoveryForeignID,
		strings.Repeat("f", 64),
	))
	decoysBefore := snapshotWorkspaceAuthorityTopology(t, preBootstrap, foreign)
	freshDirectory := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryFreshID)
	freshOwnerLock := filepath.Join(freshDirectory, "owner.lock")
	paths := append(workspaceAuthorityInitialRegistrationPaths(fixture), preBootstrap, foreign, freshDirectory, freshOwnerLock)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	generated := 0
	callbackCalls := 0
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return testWorkspaceAuthorityRecoveryFreshID, nil
			},
		},
	)
	err := registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testWorkspaceAuthorityRecoveryFreshID)
		assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
		return nil
	})
	if err != nil {
		t.Fatalf("fresh registration beside safe recovery decoys: %v", err)
	}
	if generated == 0 || callbackCalls != 1 {
		t.Fatalf("fresh registration generator/callback calls = %d/%d, want at least one/one", generated, callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, preBootstrap, foreign); !reflect.DeepEqual(after, decoysBefore) {
		t.Fatalf("fresh registration changed safe recovery decoys\nbefore: %#v\nafter:  %#v", decoysBefore, after)
	}
	registryRaw, err := os.ReadFile(fixture.registry)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := decodeWorkspaceRegistryJCSV1(registryRaw)
	if err != nil {
		t.Fatal(err)
	}
	projectedRegistry := projectRuntimeWorkspaceRegistry(registry)
	if err := validateRuntimeWorkspaceRegistry(&projectedRegistry); err != nil {
		t.Fatal(err)
	}
	entry, err := matchRuntimeWorkspaceRegistryEntry(projectedRegistry, fixture.identity)
	if err != nil || entry.WorkspaceAuthorityID != testWorkspaceAuthorityRecoveryFreshID {
		t.Fatalf("fresh registration mapping = %+v err=%v, want authority id %q", entry, err, testWorkspaceAuthorityRecoveryFreshID)
	}
	assertWorkspaceAuthorityInitialRegistrationFile(t, filepath.Join(freshDirectory, "workspace.bootstrap.json"), workspaceAuthorityRecoveryBootstrapRaw(testWorkspaceAuthorityRecoveryFreshID, fixture.identity.rootHash), fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, freshOwnerLock, nil, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, filepath.Join(freshDirectory, "admission-policies", "1.json"), fixture.policyRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, filepath.Join(freshDirectory, "workspace.private.json"), bytes.Replace(fixture.workspaceRaw, []byte(testInitialRegistrationAuthorityID), []byte(testWorkspaceAuthorityRecoveryFreshID), 1), fixture.ownerUID)
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("fresh registration beside safe decoys leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(freshOwnerLock); err != nil {
		t.Fatalf("fresh registration beside safe decoys leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func TestWorkspaceAuthorityCrashRecoveryGeneratedIDCollisionDoesNotAdoptPreBootstrapDirectory(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	preBootstrap := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryPreBootstrapID)
	createWorkspaceAuthorityRecoveryDirectory(t, preBootstrap)
	writePrivateAuthorityTestFile(t, filepath.Join(preBootstrap, "owner.lock"), []byte("non-authorizing bytes"))
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	paths := append(workspaceAuthorityInitialRegistrationPaths(fixture), preBootstrap)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
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
				if generated > 4 {
					return "", errors.New("bounded collision fixture exhausted")
				}
				return testWorkspaceAuthorityRecoveryPreBootstrapID, nil
			},
			observeInitialRegistration: func(string) error {
				observed++
				return nil
			},
		},
	)
	err := registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if err == nil || generated == 0 {
		t.Fatalf("generated pre-bootstrap collision err=%v generator calls=%d, want rejection after at least one collision", err, generated)
	}
	if observed != 0 || callbackCalls != 0 {
		t.Fatalf("generated pre-bootstrap collision initial-step/callback calls = %d/%d, want 0/0", observed, callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("generated id collision adopted or changed pre-bootstrap directory\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("generated pre-bootstrap collision leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(filepath.Join(preBootstrap, "owner.lock")); err != nil {
		t.Fatalf("generated pre-bootstrap collision leaked sibling owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
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

func TestWorkspaceAuthorityCrashRecoveryScansPastExactMatchForFatalSibling(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, workspaceAuthorityInitialRegistrationFixture, string)
	}{
		{
			name: "malformed bootstrap sibling",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, sibling string) {
				writePrivateAuthorityTestFile(t, filepath.Join(sibling, "workspace.bootstrap.json"), []byte("{"))
			},
		},
		{
			name: "unsafe canonical foreign-root sibling",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, sibling string) {
				bootstrap := filepath.Join(sibling, "workspace.bootstrap.json")
				writePrivateAuthorityTestFile(t, bootstrap, workspaceAuthorityRecoveryBootstrapRaw(
					testWorkspaceAuthorityRecoveryForeignID,
					strings.Repeat("f", 64),
				))
				if err := os.Chmod(bootstrap, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
			sibling := filepath.Join(fixture.workspacesRoot, testWorkspaceAuthorityRecoveryForeignID)
			createWorkspaceAuthorityRecoveryDirectory(t, sibling)
			test.setup(t, fixture, sibling)
			assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
		})
	}
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

func TestWorkspaceAuthorityCrashRecoveryPreflightsWorkspaceConflictBeforeCreatingMissingOwnerLock(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
	conflicting := bytes.Replace(fixture.workspaceRaw, []byte(`"nextAdmissionSeq":1`), []byte(`"nextAdmissionSeq":2`), 1)
	writePrivateAuthorityTestFile(t, fixture.workspaceAuthority, conflicting)

	assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
	if _, err := os.Lstat(fixture.ownerLock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap-only recovery workspace conflict owner lock = %v, want absent", err)
	}
}

func TestWorkspaceAuthorityCrashRecoveryPreflightsUnsafeWorkspaceSymlinkBeforeCreatingMissingOwnerLock(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
	external := filepath.Join(fixture.base, "external-workspace-authority-symlink-target")
	writePrivateAuthorityTestFile(t, external, fixture.workspaceRaw)
	if err := os.Symlink(external, fixture.workspaceAuthority); err != nil {
		t.Fatal(err)
	}

	assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
	if _, err := os.Lstat(fixture.ownerLock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap-only recovery unsafe workspace owner lock = %v, want absent", err)
	}
}

func TestWorkspaceAuthorityCrashRecoveryRejectsRegistryBoundsBeforeOwnerAcquisition(t *testing.T) {
	t.Run("exhausted registry revision", func(t *testing.T) {
		fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
		installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
		record, err := decodeWorkspaceRegistryJCSV1(fixture.initialRegistryRaw)
		if err != nil {
			t.Fatal(err)
		}
		record.RecordRev = runtimeAuthorityMaxJSONInteger
		record.PriorGeneration = &authorityGeneration{
			recordRev: runtimeAuthorityMaxJSONInteger - 1,
			sha256:    strings.Repeat("d", 64),
		}
		raw, err := encodeWorkspaceRegistryJCSV1(record)
		if err != nil {
			t.Fatal(err)
		}
		writePrivateAuthorityTestFile(t, fixture.registry, raw)
		assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
	})

	t.Run("next registry exceeds byte limit", func(t *testing.T) {
		fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
		installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
		nearLimit := workspaceRegistryJCSV1{
			Entries: []workspaceRegistryEntryJCSV1{{
				ConfiguredPath:              "/padding",
				Device:                      "0",
				Inode:                       "0",
				WorkspaceAuthorityID:        testAuthorityRecordWorkspaceID2,
				WorkspaceRootIdentitySHA256: strings.Repeat("d", 64),
			}},
			RecordRev:      1,
			RegistrySchema: 1,
		}
		nearLimitRaw, err := encodeWorkspaceRegistryJCSV1(nearLimit)
		if err != nil {
			t.Fatal(err)
		}
		paddingBytes := int(runtimeAuthorityMaxRecordBytes) - 1 - len(nearLimitRaw)
		if paddingBytes <= 0 {
			t.Fatalf("recovery near-limit registry has no padding budget: base=%d limit=%d", len(nearLimitRaw), runtimeAuthorityMaxRecordBytes)
		}
		nearLimit.Entries[0].ConfiguredPath += strings.Repeat("p", paddingBytes)
		nearLimitRaw, err = encodeWorkspaceRegistryJCSV1(nearLimit)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := int64(len(nearLimitRaw)), runtimeAuthorityMaxRecordBytes-1; got != want {
			t.Fatalf("recovery near-limit registry bytes = %d, want %d", got, want)
		}
		nextRaw, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryJCSV1{
			Entries: append(append([]workspaceRegistryEntryJCSV1(nil), nearLimit.Entries...), workspaceRegistryEntryJCSV1{
				ConfiguredPath:              fixture.identity.configuredPath,
				Device:                      strconv.FormatUint(fixture.identity.device, 10),
				Inode:                       strconv.FormatUint(fixture.identity.inode, 10),
				WorkspaceAuthorityID:        testInitialRegistrationAuthorityID,
				WorkspaceRootIdentitySHA256: fixture.identity.rootHash,
			}),
			PriorGeneration: &authorityGeneration{
				recordRev: 1,
				sha256:    testWorkspaceAuthoritySHA256(nearLimitRaw),
			},
			RecordRev:      2,
			RegistrySchema: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(nextRaw)) <= runtimeAuthorityMaxRecordBytes {
			t.Fatalf("recovery next registry bytes = %d, want above %d", len(nextRaw), runtimeAuthorityMaxRecordBytes)
		}
		writePrivateAuthorityTestFile(t, fixture.registry, nearLimitRaw)
		assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
	})
}

func TestWorkspaceAuthorityCrashRecoveryRejectsAdditionalPolicyGenerationUnchanged(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryPolicy)
	writePrivateAuthorityTestFile(t, filepath.Join(fixture.policyDir, "2.json"), []byte("unexpected generation"))
	assertWorkspaceAuthorityRecoveryRejectsSelectedUnchanged(t, fixture, fixture.base)
}

func TestWorkspaceAuthorityCrashRecoveryRejectsUnsafeCandidateAndInitialStateTopology(t *testing.T) {
	tests := []struct {
		name     string
		selected bool
		setup    func(*testing.T, workspaceAuthorityInitialRegistrationFixture)
	}{
		{
			name: "candidate is regular file",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				writePrivateAuthorityTestFile(t, fixture.authorityDir, []byte("not a directory"))
			},
		},
		{
			name: "candidate is symlink",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				external := filepath.Join(fixture.base, "external-authority-candidate")
				createWorkspaceAuthorityRecoveryDirectory(t, external)
				writePrivateAuthorityTestFile(t, filepath.Join(external, "workspace.bootstrap.json"), fixture.bootstrapRaw)
				if err := os.Symlink(external, fixture.authorityDir); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "candidate mode is not private",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
				if err := os.Chmod(fixture.authorityDir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "bootstrap is symlink",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				createWorkspaceAuthorityRecoveryDirectory(t, fixture.authorityDir)
				external := filepath.Join(fixture.base, "external-bootstrap-symlink-target")
				writePrivateAuthorityTestFile(t, external, fixture.bootstrapRaw)
				if err := os.Symlink(external, fixture.bootstrap); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "bootstrap has two links",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				createWorkspaceAuthorityRecoveryDirectory(t, fixture.authorityDir)
				external := filepath.Join(fixture.base, "external-bootstrap-hardlink-target")
				writePrivateAuthorityTestFile(t, external, fixture.bootstrapRaw)
				if err := os.Link(external, fixture.bootstrap); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "bootstrap mode is not private",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
				if err := os.Chmod(fixture.bootstrap, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "owner lock is symlink",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
				external := filepath.Join(fixture.base, "external-owner-lock-symlink-target")
				writePrivateAuthorityTestFile(t, external, nil)
				if err := os.Symlink(external, fixture.ownerLock); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "owner lock has two links",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryBootstrapOnly)
				external := filepath.Join(fixture.base, "external-owner-lock-hardlink-target")
				writePrivateAuthorityTestFile(t, external, nil)
				if err := os.Link(external, fixture.ownerLock); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "owner lock mode is not private",
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
				if err := os.Chmod(fixture.ownerLock, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "admission policy directory is symlink",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
				external := filepath.Join(fixture.base, "external-policy-directory")
				createWorkspaceAuthorityRecoveryDirectory(t, external)
				writePrivateAuthorityTestFile(t, filepath.Join(external, "1.json"), fixture.policyRaw)
				if err := os.Symlink(external, fixture.policyDir); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "admission policy directory mode is not private",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryPolicy)
				if err := os.Chmod(fixture.policyDir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "admission policy is symlink",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
				createWorkspaceAuthorityRecoveryDirectory(t, fixture.policyDir)
				external := filepath.Join(fixture.base, "external-policy-symlink-target")
				writePrivateAuthorityTestFile(t, external, fixture.policyRaw)
				if err := os.Symlink(external, fixture.policy); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "admission policy has two links",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
				createWorkspaceAuthorityRecoveryDirectory(t, fixture.policyDir)
				external := filepath.Join(fixture.base, "external-policy-hardlink-target")
				writePrivateAuthorityTestFile(t, external, fixture.policyRaw)
				if err := os.Link(external, fixture.policy); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "admission policy mode is not private",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryPolicy)
				if err := os.Chmod(fixture.policy, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "workspace authority is symlink",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryPolicy)
				external := filepath.Join(fixture.base, "external-workspace-symlink-target")
				writePrivateAuthorityTestFile(t, external, fixture.workspaceRaw)
				if err := os.Symlink(external, fixture.workspaceAuthority); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "workspace authority has two links",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryPolicy)
				external := filepath.Join(fixture.base, "external-workspace-hardlink-target")
				writePrivateAuthorityTestFile(t, external, fixture.workspaceRaw)
				if err := os.Link(external, fixture.workspaceAuthority); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:     "workspace authority mode is not private",
			selected: true,
			setup: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryWorkspace)
				if err := os.Chmod(fixture.workspaceAuthority, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			test.setup(t, fixture)
			if test.selected {
				assertWorkspaceAuthorityRecoveryRejectsSelectedUnchanged(t, fixture, fixture.base)
			} else {
				assertWorkspaceAuthorityRecoveryRejectsBeforeOwnerSelection(t, fixture, fixture.base)
			}
		})
	}
}

func TestWorkspaceAuthorityCrashRecoveryFencesCandidateAndBootstrapRetargets(t *testing.T) {
	tests := []struct {
		name           string
		retarget       string
		beforeMutation bool
	}{
		{name: "candidate retargeted while scan descriptor is open", retarget: "candidate"},
		{name: "bootstrap retargeted while scan descriptor is open", retarget: "bootstrap"},
		{name: "candidate retargeted after owner lock before mutation", retarget: "candidate", beforeMutation: true},
		{name: "bootstrap retargeted after owner lock before mutation", retarget: "bootstrap", beforeMutation: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			prefix := workspaceAuthorityRecoveryBootstrapOnly
			if test.beforeMutation {
				prefix = workspaceAuthorityRecoveryOwnerLock
			}
			installWorkspaceAuthorityRecoveryPrefix(t, fixture, prefix)
			paths := workspaceAuthorityInitialRegistrationPaths(fixture)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
			originalCandidate, err := os.Stat(fixture.authorityDir)
			if err != nil {
				t.Fatal(err)
			}
			originalBootstrap, err := os.Stat(fixture.bootstrap)
			if err != nil {
				t.Fatal(err)
			}
			productionOps := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate()).ops
			attacked := false
			var attackTopology map[string]workspaceAuthorityTopologyEntry
			performAttack := func() {
				if attacked {
					return
				}
				attacked = true
				switch test.retarget {
				case "candidate":
					moved := fixture.authorityDir + ".retargeted"
					if err := os.Rename(fixture.authorityDir, moved); err != nil {
						t.Fatal(err)
					}
					createWorkspaceAuthorityRecoveryDirectory(t, fixture.authorityDir)
					writePrivateAuthorityTestFile(t, fixture.bootstrap, fixture.bootstrapRaw)
					if test.beforeMutation {
						writePrivateAuthorityTestFile(t, fixture.ownerLock, nil)
					}
				case "bootstrap":
					moved := fixture.bootstrap + ".retargeted"
					if err := os.Rename(fixture.bootstrap, moved); err != nil {
						t.Fatal(err)
					}
					writePrivateAuthorityTestFile(t, fixture.bootstrap, fixture.bootstrapRaw)
				default:
					t.Fatalf("unknown recovery retarget kind %q", test.retarget)
				}
				attackTopology = snapshotWorkspaceAuthorityTopology(t, fixture.base)
			}

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
					validatePrivateNode: func(opened *os.File, expectedUID uint32) error {
						if !test.beforeMutation && !attacked {
							info, statErr := opened.Stat()
							if statErr != nil {
								return statErr
							}
							if (test.retarget == "candidate" && os.SameFile(info, originalCandidate)) ||
								(test.retarget == "bootstrap" && os.SameFile(info, originalBootstrap)) {
								performAttack()
							}
						}
						return productionOps.validatePrivateNode(opened, expectedUID)
					},
					observeInitialRegistration: func(step string) error {
						steps = append(steps, step)
						assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
						if step == testInitialRegistrationOwnerLockAcquired {
							assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
							if test.beforeMutation {
								performAttack()
							}
						}
						return nil
					},
				},
			)

			err = registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if err == nil {
				t.Fatal("retargeted recovery candidate was accepted")
			}
			if !attacked {
				t.Fatal("recovery never reached the pinned node needed for deterministic retarget injection")
			}
			wantSteps := []string{}
			if test.beforeMutation {
				wantSteps = []string{testInitialRegistrationOwnerLockAcquired}
			}
			if generated != 0 || callbackCalls != 0 || !reflect.DeepEqual(steps, wantSteps) {
				t.Fatalf("retargeted recovery generator/steps/callback = %d/%#v/%d, want 0/%#v/0", generated, steps, callbackCalls, wantSteps)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, attackTopology) {
				t.Fatalf("recovery mutated after injected %s retarget\nattack: %#v\nafter:  %#v", test.retarget, attackTopology, after)
			}
			assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "retargeted recovery")
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
				t.Fatalf("retargeted recovery leaked registry lock: %v", err)
			}
			for _, lockPath := range []string{fixture.ownerLock, filepath.Join(fixture.authorityDir+".retargeted", "owner.lock")} {
				if info, statErr := os.Lstat(lockPath); statErr == nil && info.Mode().IsRegular() {
					if err := tryWorkspaceAuthorityExclusiveLock(lockPath); err != nil {
						t.Fatalf("retargeted recovery leaked owner lock %q: %v", lockPath, err)
					}
				}
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
		})
	}
}

func TestWorkspaceAuthorityCrashRecoveryFencesValidatedInitialRecordRetargetsBeforeDownstreamMutation(t *testing.T) {
	tests := []struct {
		name                       string
		prefix                     workspaceAuthorityRecoveryPrefix
		targetPath                 func(workspaceAuthorityInitialRegistrationFixture) string
		targetRaw                  func(workspaceAuthorityInitialRegistrationFixture) []byte
		assertNoDownstreamMutation func(*testing.T, workspaceAuthorityInitialRegistrationFixture)
	}{
		{
			name:       "policy replacement before missing workspace creation",
			prefix:     workspaceAuthorityRecoveryPolicy,
			targetPath: func(fixture workspaceAuthorityInitialRegistrationFixture) string { return fixture.policy },
			targetRaw:  func(fixture workspaceAuthorityInitialRegistrationFixture) []byte { return fixture.policyRaw },
			assertNoDownstreamMutation: func(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
				if _, err := os.Lstat(fixture.workspaceAuthority); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("policy-retarget recovery workspace authority = %v, want absent", err)
				}
			},
		},
		{
			name:       "workspace replacement before registry mapping",
			prefix:     workspaceAuthorityRecoveryWorkspace,
			targetPath: func(fixture workspaceAuthorityInitialRegistrationFixture) string { return fixture.workspaceAuthority },
			targetRaw:  func(fixture workspaceAuthorityInitialRegistrationFixture) []byte { return fixture.workspaceRaw },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			installWorkspaceAuthorityRecoveryPrefix(t, fixture, test.prefix)
			target := test.targetPath(fixture)
			originalTarget, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			retargeted := filepath.Join(fixture.base, "retargeted-"+filepath.Base(target))
			paths := append(workspaceAuthorityInitialRegistrationPaths(fixture), retargeted)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
			productionOps := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate()).ops
			attacked := false
			var attackTopology map[string]workspaceAuthorityTopologyEntry

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
					validatePrivateNode: func(opened *os.File, expectedUID uint32) error {
						if err := productionOps.validatePrivateNode(opened, expectedUID); err != nil {
							return err
						}
						if attacked {
							return nil
						}
						info, err := opened.Stat()
						if err != nil {
							return err
						}
						if !os.SameFile(info, originalTarget) {
							return nil
						}
						attacked = true
						if err := os.Rename(target, retargeted); err != nil {
							t.Fatal(err)
						}
						writePrivateAuthorityTestFile(t, target, test.targetRaw(fixture))
						attackTopology = snapshotWorkspaceAuthorityTopology(t, fixture.base)
						return nil
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

			err = registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if err == nil {
				t.Fatal("recovery accepted a replacement of an already validated initial record")
			}
			if !attacked {
				t.Fatal("recovery never validated the initial record needed for deterministic retarget injection")
			}
			wantSteps := []string{testInitialRegistrationOwnerLockAcquired}
			if generated != 0 || callbackCalls != 0 || !reflect.DeepEqual(steps, wantSteps) {
				t.Fatalf("validated-record retarget generator/steps/callback = %d/%#v/%d, want 0/%#v/0", generated, steps, callbackCalls, wantSteps)
			}
			if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, attackTopology) {
				t.Fatalf("recovery mutated after validated-record retarget\nattack: %#v\nafter:  %#v", attackTopology, after)
			}
			assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "validated-record retarget recovery")
			if test.assertNoDownstreamMutation != nil {
				test.assertNoDownstreamMutation(t, fixture)
			}
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
				t.Fatalf("validated-record retarget recovery leaked registry lock: %v", err)
			}
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
				t.Fatalf("validated-record retarget recovery leaked owner lock: %v", err)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
		})
	}
}

func TestWorkspaceAuthorityCrashRecoveryFaultPrefixesReleaseResourcesAndRetry(t *testing.T) {
	tests := []struct {
		name       string
		start      workspaceAuthorityRecoveryPrefix
		failStep   string
		failSync   bool
		wantPrefix workspaceAuthorityRecoveryPrefix
	}{
		{name: "owner-lock observer error", start: workspaceAuthorityRecoveryOwnerLock, failStep: testInitialRegistrationOwnerLockAcquired, wantPrefix: workspaceAuthorityRecoveryOwnerLock},
		{name: "policy publication observer error", start: workspaceAuthorityRecoveryOwnerLock, failStep: testInitialRegistrationPolicyPublished, wantPrefix: workspaceAuthorityRecoveryPolicy},
		{name: "workspace publication observer error", start: workspaceAuthorityRecoveryPolicy, failStep: testInitialRegistrationWorkspacePublished, wantPrefix: workspaceAuthorityRecoveryWorkspace},
		{name: "authority directory sync error", start: workspaceAuthorityRecoveryWorkspace, failSync: true, wantPrefix: workspaceAuthorityRecoveryWorkspace},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			installWorkspaceAuthorityRecoveryPrefix(t, fixture, test.start)
			paths := workspaceAuthorityInitialRegistrationPaths(fixture)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
			productionOps := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate()).ops
			injected := errors.New("injected workspace-authority recovery prefix failure")
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
						return "", errors.New("recovery fault path selected the id generator")
					},
					observeInitialRegistration: func(step string) error {
						steps = append(steps, step)
						assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
						assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
						if step == test.failStep {
							return injected
						}
						return nil
					},
					syncInitialAuthorityDirectory: func(directory *os.File) error {
						assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
						assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
						if err := productionOps.syncInitialAuthorityDirectory(directory); err != nil {
							return err
						}
						if test.failSync {
							return injected
						}
						return nil
					},
				},
			)
			err := registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
				callbackCalls++
				return nil
			})
			if !errors.Is(err, injected) {
				t.Fatalf("recovery prefix fault error = %v, want injected sentinel", err)
			}
			if generated != 0 || callbackCalls != 0 {
				t.Fatalf("recovery prefix fault generator/callback calls = %d/%d, want 0/0", generated, callbackCalls)
			}
			if test.failStep != "" && (len(steps) == 0 || steps[len(steps)-1] != test.failStep) {
				t.Fatalf("recovery prefix fault steps = %#v, want failure boundary %q last", steps, test.failStep)
			}
			assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "recovery prefix fault")
			assertWorkspaceAuthorityRecoveryPrefix(t, fixture, test.wantPrefix)
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
				t.Fatalf("recovery prefix fault leaked registry lock: %v", err)
			}
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
				t.Fatalf("recovery prefix fault leaked owner lock: %v", err)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)

			stablePaths := workspaceAuthorityRecoveryPrefixFiles(fixture, test.wantPrefix)
			stable := snapshotWorkspaceAuthorityRecoveryFiles(t, stablePaths...)
			assertWorkspaceAuthorityRecoverySucceeds(t, fixture, test.wantPrefix, stable)
		})
	}
}

func TestWorkspaceAuthorityCrashRecoveryObserverPanicReleasesLocksAndDescriptors(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityRecoveryPrefix(t, fixture, workspaceAuthorityRecoveryOwnerLock)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	wantPanic := errors.New("injected recovery observer panic")
	generated := 0
	callbackCalls := 0
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return "", errors.New("recovery panic path selected the id generator")
			},
			observeInitialRegistration: func(step string) error {
				if step != testInitialRegistrationOwnerLockAcquired {
					t.Fatalf("recovery panic observer step = %q, want owner lock acquired", step)
				}
				assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
				assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
				panic(wantPanic)
			},
		},
	)
	returned := false
	var returnedErr error
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		returnedErr = registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			return nil
		})
		returned = true
	}()
	if returned || recovered != wantPanic {
		t.Fatalf("recovery observer panic returned=%t err=%v recovered=%#v, want exact panic %#v", returned, returnedErr, recovered, wantPanic)
	}
	if generated != 0 || callbackCalls != 0 {
		t.Fatalf("recovery panic generator/callback calls = %d/%d, want 0/0", generated, callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("recovery observer panic changed registry predecessor or selected prefix\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("recovery observer panic leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("recovery observer panic leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
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

func workspaceAuthorityRecoveryPrefixFiles(fixture workspaceAuthorityInitialRegistrationFixture, prefix workspaceAuthorityRecoveryPrefix) []string {
	result := []string{fixture.bootstrap}
	if prefix >= workspaceAuthorityRecoveryOwnerLock {
		result = append(result, fixture.ownerLock)
	}
	if prefix >= workspaceAuthorityRecoveryPolicy {
		result = append(result, fixture.policy)
	}
	if prefix >= workspaceAuthorityRecoveryWorkspace {
		result = append(result, fixture.workspaceAuthority)
	}
	return result
}

func assertWorkspaceAuthorityRecoveryPrefix(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, prefix workspaceAuthorityRecoveryPrefix) {
	t.Helper()
	assertWorkspaceAuthorityInitialRegistrationDirectory(t, fixture.authorityDir, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.bootstrap, fixture.bootstrapRaw, fixture.ownerUID)
	authorityEntries := []string{"workspace.bootstrap.json"}
	if prefix >= workspaceAuthorityRecoveryOwnerLock {
		assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.ownerLock, nil, fixture.ownerUID)
		authorityEntries = []string{"owner.lock", "workspace.bootstrap.json"}
	}
	if prefix >= workspaceAuthorityRecoveryPolicy {
		assertWorkspaceAuthorityInitialRegistrationDirectory(t, fixture.policyDir, fixture.ownerUID)
		assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.policy, fixture.policyRaw, fixture.ownerUID)
		assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t, fixture.policyDir, "1.json")
		authorityEntries = []string{"admission-policies", "owner.lock", "workspace.bootstrap.json"}
	}
	if prefix >= workspaceAuthorityRecoveryWorkspace {
		assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.workspaceAuthority, fixture.workspaceRaw, fixture.ownerUID)
		authorityEntries = []string{"admission-policies", "owner.lock", "workspace.bootstrap.json", "workspace.private.json"}
	}
	assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t, fixture.authorityDir, authorityEntries...)
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
