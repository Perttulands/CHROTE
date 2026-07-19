package formations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

const (
	testInitialRegistrationAuthorityID = "wsa_01KXNP6VY3227H78329V52CKF8"

	testInitialRegistrationOwnerLockAcquired        = "owner_lock_acquired"
	testInitialRegistrationPolicyPublished          = "admission_policy_published"
	testInitialRegistrationWorkspacePublished       = "workspace_authority_published"
	testInitialRegistrationAuthorityDirectorySynced = "authority_directory_synced"
	testInitialRegistrationRegistryPublished        = "registry_published_and_parent_synced"
)

var testInitialRegistrationSteps = []string{
	testInitialRegistrationOwnerLockAcquired,
	testInitialRegistrationPolicyPublished,
	testInitialRegistrationWorkspacePublished,
	testInitialRegistrationAuthorityDirectorySynced,
	testInitialRegistrationRegistryPublished,
}

func TestWorkspaceAuthorityInitialRegistrationPublishesExactLockedTransaction(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	initialRegistryStat := workspaceAuthorityInitialRegistrationStat(t, fixture.registry)
	workspaceDescriptorsBefore := countWorkspaceAuthorityDescriptors(t, fixture.workspace)
	steps := make([]string, 0, len(testInitialRegistrationSteps))
	generated := 0

	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(err, syscall.EWOULDBLOCK) {
					t.Fatalf("authority-id generation registry lock probe = %v, want would-block", err)
				}
				if got, want := countWorkspaceAuthorityDescriptors(t, fixture.workspace), workspaceDescriptorsBefore+1; got != want {
					t.Fatalf("authority-id generation retained workspace descriptors = %d, want %d after pinned identity", got, want)
				}
				return testInitialRegistrationAuthorityID, nil
			},
			observeInitialRegistration: func(step string) error {
				steps = append(steps, step)
				assertWorkspaceAuthorityInitialRegistrationStage(t, fixture, step)
				return nil
			},
		},
	)

	callbackCalls := 0
	err := registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testInitialRegistrationAuthorityID)
		assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
		if raw := readWorkspaceAuthorityInitialRegistrationFile(t, fixture.registry); !bytes.Equal(raw, fixture.finalRegistryRaw) {
			t.Fatalf("registration callback registry bytes\n got: %s\nwant: %s", raw, fixture.finalRegistryRaw)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("initial workspace-authority registration: %v", err)
	}
	if generated != 1 {
		t.Fatalf("workspace-authority id generator calls = %d, want exactly one", generated)
	}
	if callbackCalls != 1 {
		t.Fatalf("registration callback calls = %d, want exactly one", callbackCalls)
	}
	if !reflect.DeepEqual(steps, testInitialRegistrationSteps) {
		t.Fatalf("initial registration steps = %#v, want exact transaction order %#v", steps, testInitialRegistrationSteps)
	}
	assertWorkspaceAuthorityInitialRegistrationPrivateState(t, fixture)
	finalRegistryStat := workspaceAuthorityInitialRegistrationStat(t, fixture.registry)
	if finalRegistryStat.inode == initialRegistryStat.inode {
		t.Fatalf("mutable registry publication kept predecessor inode %d; want atomic replacement", initialRegistryStat.inode)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("successful registration leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("successful registration leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func TestWorkspaceAuthorityInitialRegistrationDefaultOpsGenerateCanonicalUniqueIDsAndUseRealDirectorySync(t *testing.T) {
	registrar := newWorkspaceAuthorityRegistrar("/unused/test-authority-root", uint32(os.Geteuid()), newWorkspaceAuthorityCapabilityGate())
	if registrar.ops.generateWorkspaceAuthorityID == nil {
		t.Fatal("normal workspace-authority registrar has no production id generator")
	}
	generated := map[string]bool{}
	for index := 0; index < 8; index++ {
		authorityID, err := registrar.ops.generateWorkspaceAuthorityID()
		if err != nil {
			t.Fatalf("generate production workspace-authority id %d: %v", index, err)
		}
		if !runtimeWorkspaceAuthorityIDPattern.MatchString(authorityID) {
			t.Fatalf("production workspace-authority id %d = %q, want canonical uppercase wsa_ ULID grammar", index, authorityID)
		}
		if generated[authorityID] {
			t.Fatalf("production workspace-authority generator repeated %q within bounded sample", authorityID)
		}
		generated[authorityID] = true
	}

	if registrar.ops.syncInitialAuthorityDirectory == nil {
		t.Fatal("normal workspace-authority registrar has no production directory-sync operation")
	}
	closed, err := os.CreateTemp(t.TempDir(), "closed-authority-directory-sync-")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := registrar.ops.syncInitialAuthorityDirectory(closed); err == nil {
		t.Fatal("production authority-directory sync accepted a closed descriptor; want real file.Sync-backed failure")
	}
	if registrar.ops.syncWorkspaceRegistrationDirectory == nil {
		t.Fatal("normal workspace-authority registrar has no workspace-registration directory-sync operation")
	}
	if err := registrar.ops.syncWorkspaceRegistrationDirectory(closed); err == nil {
		t.Fatal("production workspace-registration directory sync accepted a closed descriptor; want real file.Sync-backed failure")
	}
}

func TestWorkspaceAuthorityRegistrationExistingMappingIsReadOnlyIdempotent(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityInitialRegistrationPrivateState(t, fixture)
	writePrivateAuthorityTestFile(t, fixture.registry, fixture.mappedRegistryRaw)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	generated := 0
	observed := 0

	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return testAuthorityRecordWorkspaceID2, nil
			},
			observeInitialRegistration: func(string) error {
				observed++
				return nil
			},
		},
	)

	callbackCalls := 0
	err := registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testInitialRegistrationAuthorityID)
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(err, syscall.EWOULDBLOCK) {
			t.Fatalf("idempotent callback registry lock probe = %v, want would-block", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("idempotent registration of existing exact mapping: %v", err)
	}
	if generated != 0 || observed != 0 {
		t.Fatalf("existing mapping generator/initialization calls = %d/%d, want 0/0", generated, observed)
	}
	if callbackCalls != 1 {
		t.Fatalf("existing mapping callback calls = %d, want exactly one", callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("idempotent existing registration changed private state\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("idempotent registration leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("idempotent registration leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func TestWorkspaceAuthorityRegistrationExactMappingCompletesAmbiguousParentDurabilityBeforeCallback(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	installWorkspaceAuthorityInitialRegistrationPrivateState(t, fixture)
	predecessorRegistry := workspaceAuthorityInitialRegistrationStat(t, fixture.registry)
	postRenameRegistry := fixture.registry + ".post-rename"
	writePrivateAuthorityTestFile(t, postRenameRegistry, fixture.mappedRegistryRaw)
	if err := os.Rename(postRenameRegistry, fixture.registry); err != nil {
		t.Fatal(err)
	}
	// Deliberately do not sync workspacesRoot: this is the visible
	// post-rename/pre-parent-fsync state a retry must close before success.
	visibleRegistry := workspaceAuthorityInitialRegistrationStat(t, fixture.registry)
	if visibleRegistry.inode == predecessorRegistry.inode {
		t.Fatalf("ambiguous registry setup kept predecessor inode %d; want visible atomic replacement", predecessorRegistry.inode)
	}
	assertWorkspaceAuthorityInitialRegistrationPrivateStateExceptRegistry(t, fixture)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.registry, fixture.mappedRegistryRaw, fixture.ownerUID)

	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	injected := errors.New("injected workspaces-parent sync failure")
	generated := 0
	initialPublicationSteps := 0
	initialDirectorySyncs := 0
	workspaceRegistrationSyncs := 0
	callbackCalls := 0
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return testAuthorityRecordWorkspaceID2, nil
			},
			observeInitialRegistration: func(string) error {
				initialPublicationSteps++
				return nil
			},
			syncInitialAuthorityDirectory: func(*os.File) error {
				initialDirectorySyncs++
				return nil
			},
			syncWorkspaceRegistrationDirectory: func(directory *os.File) error {
				workspaceRegistrationSyncs++
				openedInfo, err := directory.Stat()
				if err != nil {
					t.Fatal(err)
				}
				namedInfo, err := os.Stat(fixture.workspacesRoot)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(openedInfo, namedInfo) {
					t.Fatalf("mapping durability sync used descriptor for %v, want operative %v", openedInfo, namedInfo)
				}
				assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
				if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
					t.Fatalf("mapping durability sync began after private mutation\nbefore: %#v\nafter:  %#v", before, after)
				}
				if workspaceRegistrationSyncs == 1 {
					return injected
				}
				return directory.Sync()
			},
		},
	)

	err := registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("ambiguous mapping parent-sync error = %v, want injected sentinel", err)
	}
	if callbackCalls != 0 {
		t.Fatalf("ambiguous mapping callback calls after failed durability barrier = %d, want zero", callbackCalls)
	}
	if generated != 0 || initialPublicationSteps != 0 || initialDirectorySyncs != 0 {
		t.Fatalf("ambiguous mapping generator/initial-step/initial-sync calls = %d/%d/%d, want 0/0/0", generated, initialPublicationSteps, initialDirectorySyncs)
	}
	if workspaceRegistrationSyncs != 1 {
		t.Fatalf("ambiguous mapping parent-sync calls after failure = %d, want exactly one", workspaceRegistrationSyncs)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("failed mapping durability barrier changed registry bytes, inode, or private topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("failed mapping durability barrier leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("failed mapping durability barrier leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)

	err = registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		if workspaceRegistrationSyncs != 2 {
			t.Fatalf("mapping callback ran before successful parent durability barrier: sync calls = %d, want 2", workspaceRegistrationSyncs)
		}
		assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testInitialRegistrationAuthorityID)
		assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
		return nil
	})
	if err != nil {
		t.Fatalf("retry completing ambiguous mapping parent durability: %v", err)
	}
	if callbackCalls != 1 || workspaceRegistrationSyncs != 2 {
		t.Fatalf("successful durability retry callback/parent-sync calls = %d/%d, want 1/2", callbackCalls, workspaceRegistrationSyncs)
	}
	if generated != 0 || initialPublicationSteps != 0 || initialDirectorySyncs != 0 {
		t.Fatalf("successful durability retry generator/initial-step/initial-sync calls = %d/%d/%d, want 0/0/0", generated, initialPublicationSteps, initialDirectorySyncs)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("successful mapping durability retry changed registry bytes, inode, or private topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("successful mapping durability retry leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("successful mapping durability retry leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func TestWorkspaceAuthorityInitialRegistrationRejectsOversizeNextRegistryBeforePrivateMutation(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	// A valid predecessor may fit while its canonical append does not. The
	// append must be size-checked before creating orphanable private state.
	paddingEntry := workspaceRegistryEntryJCSV1{
		ConfiguredPath:              "/padding",
		Device:                      "0",
		Inode:                       "0",
		WorkspaceAuthorityID:        testAuthorityRecordWorkspaceID2,
		WorkspaceRootIdentitySHA256: strings.Repeat("d", 64),
	}
	nearLimit := workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{paddingEntry},
		RecordRev:      1,
		RegistrySchema: 1,
	}
	nearLimitRaw, err := encodeWorkspaceRegistryJCSV1(nearLimit)
	if err != nil {
		t.Fatal(err)
	}
	paddingBytes := int(runtimeAuthorityMaxRecordBytes) - 1 - len(nearLimitRaw)
	if paddingBytes <= 0 {
		t.Fatalf("near-limit registry fixture has no padding budget: base=%d limit=%d", len(nearLimitRaw), runtimeAuthorityMaxRecordBytes)
	}
	nearLimit.Entries[0].ConfiguredPath += strings.Repeat("p", paddingBytes)
	nearLimitRaw, err = encodeWorkspaceRegistryJCSV1(nearLimit)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := int64(len(nearLimitRaw)), runtimeAuthorityMaxRecordBytes-1; got != want {
		t.Fatalf("near-limit registry fixture bytes = %d, want %d", got, want)
	}

	nextEntries := append([]workspaceRegistryEntryJCSV1(nil), nearLimit.Entries...)
	nextEntries = append(nextEntries, workspaceRegistryEntryJCSV1{
		ConfiguredPath:              fixture.identity.configuredPath,
		Device:                      strconv.FormatUint(fixture.identity.device, 10),
		Inode:                       strconv.FormatUint(fixture.identity.inode, 10),
		WorkspaceAuthorityID:        testInitialRegistrationAuthorityID,
		WorkspaceRootIdentitySHA256: fixture.identity.rootHash,
	})
	nextRaw, err := encodeWorkspaceRegistryJCSV1(workspaceRegistryJCSV1{
		Entries: nextEntries,
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
		t.Fatalf("appended registry fixture bytes = %d, want above %d", len(nextRaw), runtimeAuthorityMaxRecordBytes)
	}

	writePrivateAuthorityTestFile(t, fixture.registry, nearLimitRaw)
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	generated := 0
	initialPublicationSteps := 0
	initialDirectorySyncs := 0
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
				initialPublicationSteps++
				return nil
			},
			syncInitialAuthorityDirectory: func(*os.File) error {
				initialDirectorySyncs++
				return nil
			},
		},
	)

	err = registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if !errors.Is(err, errRuntimeOutOfRange) {
		t.Fatalf("oversize appended registry error = %v, want %v", err, errRuntimeOutOfRange)
	}
	if generated > 1 {
		t.Fatalf("oversize appended registry id generator calls = %d, want at most one size-computation input", generated)
	}
	if initialPublicationSteps != 0 || initialDirectorySyncs != 0 || callbackCalls != 0 {
		t.Fatalf("oversize appended registry publication-step/directory-sync/callback calls = %d/%d/%d, want 0/0/0 before private mutation", initialPublicationSteps, initialDirectorySyncs, callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("oversize appended registry changed predecessor bytes, inode, or private topology\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("oversize appended registry path leaked registry lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func TestWorkspaceAuthorityInitialRegistrationRejectsInvalidIDCollisionAndPreconditionsWithoutMutation(t *testing.T) {
	t.Run("invalid generated id", func(t *testing.T) {
		for _, invalidID := range []string{
			"wsa_81KXNP6VY3227H78329V52CKF8",
			"wsa_01kxnp6vy3227h78329v52ckf8",
			"../wsa_01KXNP6VY3227H78329V52CKF8",
		} {
			t.Run(invalidID, func(t *testing.T) {
				fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
				assertWorkspaceAuthorityInitialRegistrationRejectsUnchanged(t, fixture, newWorkspaceAuthorityCapabilityGate(), 1, func() (string, error) {
					return invalidID, nil
				}, errRuntimeNoncanonical)
			})
		}
	})

	t.Run("id generator error", func(t *testing.T) {
		fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
		injected := errors.New("injected authority id generator failure")
		assertWorkspaceAuthorityInitialRegistrationRejectsUnchanged(t, fixture, newWorkspaceAuthorityCapabilityGate(), 1, func() (string, error) {
			return "", injected
		}, injected)
	})

	t.Run("authority id occupied by another registry entry", func(t *testing.T) {
		fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
		before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
		paths := workspaceAuthorityInitialRegistrationPaths(fixture)
		descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
		generated := 0
		registrar := newWorkspaceAuthorityRegistrarForTest(
			fixture.hostRoot,
			fixture.ownerUID,
			newWorkspaceAuthorityCapabilityGate(),
			workspaceAuthorityRegistrationTestOps{
				generateWorkspaceAuthorityID: func() (string, error) {
					generated++
					if generated == 1 {
						return testAuthorityRecordWorkspaceID2, nil
					}
					return "", errors.New("stop after occupied registry id")
				},
			},
		)
		callbackCalls := 0
		err := registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
			callbackCalls++
			assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testInitialRegistrationAuthorityID)
			return nil
		})
		if err == nil || generated < 1 || generated > 2 || callbackCalls != 0 {
			t.Fatalf("occupied-id result err=%v generator/callback calls=%d/%d; want no registration and at most one bounded retry attempt", err, generated, callbackCalls)
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
			t.Fatalf("occupied-id rejection mutated registry entry or private topology\nbefore: %#v\nafter:  %#v", before, after)
		}
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
			t.Fatalf("occupied-id path leaked registry lock: %v", err)
		}
		if _, statErr := os.Lstat(fixture.ownerLock); statErr == nil {
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
				t.Fatalf("occupied-id path leaked owner lock: %v", err)
			}
		}
		assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
	})

	t.Run("capability gate before id selection", func(t *testing.T) {
		fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
		canonical := newWorkspaceAuthorityCapabilityGate().capabilities
		invalidGate := workspaceAuthorityCapabilityGate{capabilities: cloneWorkspaceAuthorityCapabilities(canonical[:1])}
		assertWorkspaceAuthorityInitialRegistrationRejectsUnchanged(t, fixture, invalidGate, 0, func() (string, error) {
			return testInitialRegistrationAuthorityID, nil
		}, nil)
	})

	t.Run("exhausted registry revision before id selection", func(t *testing.T) {
		fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
		record, err := decodeWorkspaceRegistryJCSV1(fixture.initialRegistryRaw)
		if err != nil {
			t.Fatal(err)
		}
		record.RecordRev = runtimeAuthorityMaxJSONInteger
		record.PriorGeneration = &authorityGeneration{
			recordRev: runtimeAuthorityMaxJSONInteger - 1,
			sha256:    strings.Repeat("d", 64),
		}
		exhaustedRaw, err := encodeWorkspaceRegistryJCSV1(record)
		if err != nil {
			t.Fatalf("encode exhausted registry fixture: %v", err)
		}
		writePrivateAuthorityTestFile(t, fixture.registry, exhaustedRaw)
		assertWorkspaceAuthorityInitialRegistrationRejectsUnchanged(t, fixture, newWorkspaceAuthorityCapabilityGate(), 0, func() (string, error) {
			return testInitialRegistrationAuthorityID, nil
		}, errRuntimeOutOfRange)
	})

	t.Run("nil callback before id selection", func(t *testing.T) {
		fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
		before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
		paths := workspaceAuthorityInitialRegistrationPaths(fixture)
		descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
		generated := 0
		observed := 0
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
		err := registrar.register(fixture.workspace, nil)
		if !errors.Is(err, errRuntimeNoncanonical) {
			t.Fatalf("nil registration callback error = %v, want noncanonical", err)
		}
		if generated != 0 || observed != 0 {
			t.Fatalf("nil callback generator/observer calls = %d/%d, want 0/0", generated, observed)
		}
		if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
			t.Fatalf("nil callback changed authority state\nbefore: %#v\nafter:  %#v", before, after)
		}
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
			t.Fatalf("nil callback leaked registry lock: %v", err)
		}
		assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
	})
}

func TestWorkspaceAuthorityInitialRegistrationKeepsRegistryUnchangedUntilPrivateDirectoryIsCompleteAndSynced(t *testing.T) {
	fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	injected := errors.New("injected post-authority-directory-fsync failure")
	steps := []string{}
	callbackCalls := 0
	productionOps := newWorkspaceAuthorityRegistrar(fixture.hostRoot, fixture.ownerUID, newWorkspaceAuthorityCapabilityGate()).ops
	if productionOps.syncInitialAuthorityDirectory == nil {
		t.Fatal("normal registrar has no authority-directory sync operation")
	}
	syncCalls := 0
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		newWorkspaceAuthorityCapabilityGate(),
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				return testInitialRegistrationAuthorityID, nil
			},
			observeInitialRegistration: func(step string) error {
				steps = append(steps, step)
				assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "pre-sync-fault step "+step)
				if step == testInitialRegistrationAuthorityDirectorySynced || step == testInitialRegistrationRegistryPublished {
					t.Fatalf("registration reported %q after authority-directory sync returned an error", step)
				}
				return nil
			},
			syncInitialAuthorityDirectory: func(directory *os.File) error {
				syncCalls++
				openedInfo, err := directory.Stat()
				if err != nil {
					t.Fatal(err)
				}
				namedInfo, err := os.Stat(fixture.authorityDir)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(openedInfo, namedInfo) {
					t.Fatalf("authority-directory sync used descriptor for %v, want operative %v", openedInfo, namedInfo)
				}
				if err := productionOps.syncInitialAuthorityDirectory(directory); err != nil {
					t.Fatalf("real authority-directory sync before injected failure: %v", err)
				}
				assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
				assertWorkspaceAuthorityInitialRegistrationPrivateStateExceptRegistry(t, fixture)
				assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "post-real-fsync injected fault")
				return injected
			},
		},
	)

	err := registrar.register(fixture.workspace, func(workspaceAuthorityRegistrationScope) error {
		callbackCalls++
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("post-authority-directory-fsync error = %v, want injected sentinel", err)
	}
	if callbackCalls != 0 {
		t.Fatalf("post-fsync failure callback calls = %d, want zero before registry publication", callbackCalls)
	}
	wantSteps := testInitialRegistrationSteps[:3]
	if !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("post-fsync failure observation steps = %#v, want pre-sync prefix %#v", steps, wantSteps)
	}
	if syncCalls != 1 {
		t.Fatalf("authority-directory sync calls = %d, want exactly one real sync before injected error", syncCalls)
	}
	assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "post-fsync registration failure")
	assertWorkspaceAuthorityInitialRegistrationPrivateStateExceptRegistry(t, fixture)
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("post-fsync failure leaked registry lock: %v", err)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
		t.Fatalf("post-fsync failure leaked owner lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func TestWorkspaceAuthorityInitialRegistrationCallbackFailureKeepsDurableMappingAndReleasesResources(t *testing.T) {
	tests := []struct {
		name  string
		panic bool
	}{
		{name: "error"},
		{name: "panic", panic: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceAuthorityInitialRegistrationFixture(t)
			paths := workspaceAuthorityInitialRegistrationPaths(fixture)
			descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
			generated := 0
			observed := 0
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
			injected := errors.New("injected registration callback failure")
			callbackCalls := 0
			returned := false
			var returnedErr error
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				returnedErr = registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
					callbackCalls++
					assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testInitialRegistrationAuthorityID)
					assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t, fixture)
					if test.panic {
						panic(injected)
					}
					return injected
				})
				returned = true
			}()
			if test.panic {
				if returned || recovered != injected {
					t.Fatalf("registration callback panic returned=%t err=%v recovered=%#v, want exact panic %#v", returned, returnedErr, recovered, injected)
				}
			} else {
				if !returned || !errors.Is(returnedErr, injected) || recovered != nil {
					t.Fatalf("registration callback error returned=%t err=%v recovered=%#v, want propagated sentinel", returned, returnedErr, recovered)
				}
			}
			if callbackCalls != 1 || generated != 1 || observed != len(testInitialRegistrationSteps) {
				t.Fatalf("callback/generator/step calls = %d/%d/%d, want 1/1/%d", callbackCalls, generated, observed, len(testInitialRegistrationSteps))
			}
			assertWorkspaceAuthorityInitialRegistrationPrivateState(t, fixture)
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
				t.Fatalf("callback failure leaked registry lock: %v", err)
			}
			if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); err != nil {
				t.Fatalf("callback failure leaked owner lock: %v", err)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)

			beforeRetry := snapshotWorkspaceAuthorityTopology(t, fixture.base)
			retryCalls := 0
			if err := registrar.register(fixture.workspace, func(scope workspaceAuthorityRegistrationScope) error {
				retryCalls++
				assertWorkspaceAuthorityInitialRegistrationScope(t, scope, fixture.identity, testInitialRegistrationAuthorityID)
				return nil
			}); err != nil {
				t.Fatalf("idempotent retry after callback %s: %v", test.name, err)
			}
			if generated != 1 || observed != len(testInitialRegistrationSteps) || retryCalls != 1 {
				t.Fatalf("retry generator/step/callback calls = %d/%d/%d, want 1/%d/1", generated, observed, retryCalls, len(testInitialRegistrationSteps))
			}
			if afterRetry := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(afterRetry, beforeRetry) {
				t.Fatalf("idempotent retry after callback %s changed durable mapping\nbefore: %#v\nafter:  %#v", test.name, beforeRetry, afterRetry)
			}
			assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
		})
	}
}

type workspaceAuthorityInitialRegistrationFixture struct {
	workspaceAuthorityRegistrationFixture
	identity            runtimeWorkspaceIdentity
	authorityDir        string
	ownerLock           string
	bootstrap           string
	policyDir           string
	policy              string
	workspaceAuthority  string
	initialRegistryRaw  []byte
	initialRegistryNode workspaceAuthorityTopologyEntry
	finalRegistryRaw    []byte
	mappedRegistryRaw   []byte
	bootstrapRaw        []byte
	policyRaw           []byte
	workspaceRaw        []byte
}

func newWorkspaceAuthorityInitialRegistrationFixture(t *testing.T) workspaceAuthorityInitialRegistrationFixture {
	t.Helper()
	registration := newWorkspaceAuthorityRegistrationFixture(t, workspaceRegistryJCSV1{
		Entries:        []workspaceRegistryEntryJCSV1{},
		RecordRev:      1,
		RegistrySchema: 1,
	})
	identity := testWorkspaceAuthorityIdentityAtPath(t, registration.workspace).identity
	prior := &authorityGeneration{recordRev: 6, sha256: strings.Repeat("a", 64)}
	initial := workspaceRegistryJCSV1{
		Entries: []workspaceRegistryEntryJCSV1{
			{
				ConfiguredPath:              "/foreign/before",
				Device:                      strconv.FormatUint(identity.device, 10),
				Inode:                       "0",
				WorkspaceAuthorityID:        testAuthorityRecordWorkspaceID2,
				WorkspaceRootIdentitySHA256: strings.Repeat("b", 64),
			},
			{
				ConfiguredPath:              "/foreign/after",
				Device:                      strconv.FormatUint(identity.device, 10),
				Inode:                       strconv.FormatUint(^uint64(0), 10),
				WorkspaceAuthorityID:        testAuthorityRecordWorkspaceID3,
				WorkspaceRootIdentitySHA256: strings.Repeat("c", 64),
			},
		},
		PriorGeneration: prior,
		RecordRev:       7,
		RegistrySchema:  1,
	}
	initialRaw, err := encodeWorkspaceRegistryJCSV1(initial)
	if err != nil {
		t.Fatalf("encode initial registration registry fixture: %v", err)
	}
	writePrivateAuthorityTestFile(t, registration.registry, initialRaw)
	initialRegistryNode := snapshotWorkspaceAuthorityTopology(t, registration.registry)[registration.registry]

	newEntry := workspaceRegistryEntryJCSV1{
		ConfiguredPath:              identity.configuredPath,
		Device:                      strconv.FormatUint(identity.device, 10),
		Inode:                       strconv.FormatUint(identity.inode, 10),
		WorkspaceAuthorityID:        testInitialRegistrationAuthorityID,
		WorkspaceRootIdentitySHA256: identity.rootHash,
	}
	finalEntries := append(append([]workspaceRegistryEntryJCSV1(nil), initial.Entries...), newEntry)
	sort.Slice(finalEntries, func(i, j int) bool {
		leftDevice, _ := strconv.ParseUint(finalEntries[i].Device, 10, 64)
		rightDevice, _ := strconv.ParseUint(finalEntries[j].Device, 10, 64)
		if leftDevice != rightDevice {
			return leftDevice < rightDevice
		}
		leftInode, _ := strconv.ParseUint(finalEntries[i].Inode, 10, 64)
		rightInode, _ := strconv.ParseUint(finalEntries[j].Inode, 10, 64)
		if leftInode != rightInode {
			return leftInode < rightInode
		}
		return finalEntries[i].ConfiguredPath < finalEntries[j].ConfiguredPath
	})
	finalRaw := workspaceAuthorityInitialRegistrationRegistryRaw(
		t,
		finalEntries,
		8,
		&authorityGeneration{recordRev: 7, sha256: testWorkspaceAuthoritySHA256(initialRaw)},
	)
	mappedRaw := workspaceAuthorityInitialRegistrationRegistryRaw(t, []workspaceRegistryEntryJCSV1{newEntry}, 1, nil)
	policyRaw := []byte(`{"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}`)
	policyHash := testWorkspaceAuthoritySHA256(policyRaw)
	bootstrapRaw := []byte(fmt.Sprintf(
		`{"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		testInitialRegistrationAuthorityID,
		identity.rootHash,
	))
	workspaceRaw := []byte(fmt.Sprintf(
		`{"admissionPolicyRef":{"policyRev":1,"policySha256":"%s"},"authoritySchema":2,"nextAdmissionSeq":1,"nextWriterFence":1,"priorGeneration":null,"recordRev":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		policyHash,
		testInitialRegistrationAuthorityID,
		identity.rootHash,
	))
	authorityDir := filepath.Join(registration.workspacesRoot, testInitialRegistrationAuthorityID)
	policyDir := filepath.Join(authorityDir, "admission-policies")
	return workspaceAuthorityInitialRegistrationFixture{
		workspaceAuthorityRegistrationFixture: registration,
		identity:                              identity,
		authorityDir:                          authorityDir,
		ownerLock:                             filepath.Join(authorityDir, "owner.lock"),
		bootstrap:                             filepath.Join(authorityDir, "workspace.bootstrap.json"),
		policyDir:                             policyDir,
		policy:                                filepath.Join(policyDir, "1.json"),
		workspaceAuthority:                    filepath.Join(authorityDir, "workspace.private.json"),
		initialRegistryRaw:                    initialRaw,
		initialRegistryNode:                   initialRegistryNode,
		finalRegistryRaw:                      finalRaw,
		mappedRegistryRaw:                     mappedRaw,
		bootstrapRaw:                          bootstrapRaw,
		policyRaw:                             policyRaw,
		workspaceRaw:                          workspaceRaw,
	}
}

func workspaceAuthorityInitialRegistrationRegistryRaw(t *testing.T, entries []workspaceRegistryEntryJCSV1, recordRev uint64, prior *authorityGeneration) []byte {
	t.Helper()
	entryRaw := make([][]byte, len(entries))
	for index, entry := range entries {
		configuredPath := workspaceAuthorityInitialRegistrationJSONString(t, entry.ConfiguredPath)
		device := workspaceAuthorityInitialRegistrationJSONString(t, entry.Device)
		inode := workspaceAuthorityInitialRegistrationJSONString(t, entry.Inode)
		authorityID := workspaceAuthorityInitialRegistrationJSONString(t, entry.WorkspaceAuthorityID)
		rootHash := workspaceAuthorityInitialRegistrationJSONString(t, entry.WorkspaceRootIdentitySHA256)
		entryRaw[index] = []byte(fmt.Sprintf(
			`{"configuredPath":%s,"device":%s,"inode":%s,"workspaceAuthorityId":%s,"workspaceRootIdentitySha256":%s}`,
			configuredPath,
			device,
			inode,
			authorityID,
			rootHash,
		))
	}
	priorRaw := "null"
	if prior != nil {
		priorRaw = fmt.Sprintf(`{"recordRev":%d,"sha256":"%s"}`, prior.recordRev, prior.sha256)
	}
	return []byte(fmt.Sprintf(
		`{"entries":[%s],"priorGeneration":%s,"recordRev":%d,"registrySchema":1}`,
		bytes.Join(entryRaw, []byte(",")),
		priorRaw,
		recordRev,
	))
}

func workspaceAuthorityInitialRegistrationJSONString(t *testing.T, value string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func installWorkspaceAuthorityInitialRegistrationPrivateState(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
	t.Helper()
	for _, directory := range []string{fixture.authorityDir, fixture.policyDir} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writePrivateAuthorityTestFile(t, fixture.bootstrap, fixture.bootstrapRaw)
	writePrivateAuthorityTestFile(t, fixture.ownerLock, nil)
	writePrivateAuthorityTestFile(t, fixture.policy, fixture.policyRaw)
	writePrivateAuthorityTestFile(t, fixture.workspaceAuthority, fixture.workspaceRaw)
}

func assertWorkspaceAuthorityInitialRegistrationStage(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, step string) {
	t.Helper()
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("registration step %q registry lock probe = %v, want would-block", step, err)
	}
	if step != testInitialRegistrationRegistryPublished {
		assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t, fixture, "registration step "+step)
	}
	switch step {
	case testInitialRegistrationOwnerLockAcquired:
		assertWorkspaceAuthorityInitialRegistrationDirectory(t, fixture.authorityDir, fixture.ownerUID)
		assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.ownerLock, nil, fixture.ownerUID)
		if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); !errors.Is(err, syscall.EWOULDBLOCK) {
			t.Fatalf("owner-lock-acquired probe = %v, want would-block", err)
		}
		if _, err := os.Lstat(fixture.policy); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owner-lock acquisition already published admission policy: %v", err)
		}
		if _, err := os.Lstat(fixture.workspaceAuthority); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owner-lock acquisition already published workspace authority: %v", err)
		}
	case testInitialRegistrationPolicyPublished:
		assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
		assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.policy, fixture.policyRaw, fixture.ownerUID)
		if _, err := os.Lstat(fixture.workspaceAuthority); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("admission policy stage already published workspace authority: %v", err)
		}
	case testInitialRegistrationWorkspacePublished:
		assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
		assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.policy, fixture.policyRaw, fixture.ownerUID)
		assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.workspaceAuthority, fixture.workspaceRaw, fixture.ownerUID)
	case testInitialRegistrationAuthorityDirectorySynced:
		assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t, fixture)
		assertWorkspaceAuthorityInitialRegistrationPrivateStateExceptRegistry(t, fixture)
	case testInitialRegistrationRegistryPublished:
		assertWorkspaceAuthorityInitialRegistrationPrivateState(t, fixture)
	default:
		t.Fatalf("unexpected initial registration step %q", step)
	}
}

func assertWorkspaceAuthorityInitialRegistrationRejectsUnchanged(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, gate workspaceAuthorityCapabilityGate, wantGenerated int, generate func() (string, error), wantError error) {
	t.Helper()
	before := snapshotWorkspaceAuthorityTopology(t, fixture.base)
	paths := workspaceAuthorityInitialRegistrationPaths(fixture)
	descriptorsBefore := snapshotWorkspaceAuthorityOpenDescriptors(t, paths...)
	generated := 0
	observed := 0
	callbackCalls := 0
	registrar := newWorkspaceAuthorityRegistrarForTest(
		fixture.hostRoot,
		fixture.ownerUID,
		gate,
		workspaceAuthorityRegistrationTestOps{
			generateWorkspaceAuthorityID: func() (string, error) {
				generated++
				return generate()
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
	if err == nil {
		t.Fatal("invalid initial registration precondition was accepted")
	}
	if wantError != nil && !errors.Is(err, wantError) {
		t.Fatalf("initial registration rejection = %v, want %v", err, wantError)
	}
	if generated != wantGenerated {
		t.Fatalf("workspace-authority id generator calls = %d, want %d", generated, wantGenerated)
	}
	if observed != 0 || callbackCalls != 0 {
		t.Fatalf("rejected initial registration observer/callback calls = %d/%d, want 0/0", observed, callbackCalls)
	}
	if after := snapshotWorkspaceAuthorityTopology(t, fixture.base); !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected initial registration changed authority state\nbefore: %#v\nafter:  %#v", before, after)
	}
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); err != nil {
		t.Fatalf("rejected initial registration leaked registry lock: %v", err)
	}
	assertWorkspaceAuthorityOpenDescriptorsUnchanged(t, descriptorsBefore, paths...)
}

func assertWorkspaceAuthorityInitialRegistrationScope(t *testing.T, scope workspaceAuthorityRegistrationScope, wantIdentity runtimeWorkspaceIdentity, wantID string) {
	t.Helper()
	if scope == nil {
		t.Fatal("registration callback received nil scope")
	}
	gotID, matched := scope.matchedWorkspaceAuthorityID()
	if !matched || gotID != wantID {
		t.Fatalf("registered workspace authority mapping = %q, matched=%t; want %q, true", gotID, matched, wantID)
	}
	if gotIdentity := scope.workspaceIdentity(); gotIdentity != wantIdentity {
		t.Fatalf("registered workspace identity = %+v, want %+v", gotIdentity, wantIdentity)
	}
}

func assertWorkspaceAuthorityInitialRegistrationRegistryLockHeld(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
	t.Helper()
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.registryLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("registration callback registry lock probe = %v, want would-block", err)
	}
}

func assertWorkspaceAuthorityInitialRegistrationOwnerLockHeld(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
	t.Helper()
	if err := tryWorkspaceAuthorityExclusiveLock(fixture.ownerLock); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("registration owner lock probe = %v, want would-block", err)
	}
}

func assertWorkspaceAuthorityInitialRegistrationPrivateState(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
	t.Helper()
	assertWorkspaceAuthorityInitialRegistrationPrivateStateExceptRegistry(t, fixture)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.registry, fixture.finalRegistryRaw, fixture.ownerUID)
}

func assertWorkspaceAuthorityInitialRegistrationRegistryUnchanged(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture, context string) {
	t.Helper()
	if raw := readWorkspaceAuthorityInitialRegistrationFile(t, fixture.registry); !bytes.Equal(raw, fixture.initialRegistryRaw) {
		t.Fatalf("%s changed registry bytes\n got: %s\nwant: %s", context, raw, fixture.initialRegistryRaw)
	}
	got := snapshotWorkspaceAuthorityTopology(t, fixture.registry)[fixture.registry]
	if got != fixture.initialRegistryNode {
		t.Fatalf("%s changed registry inode/topology\n got: %+v\nwant: %+v", context, got, fixture.initialRegistryNode)
	}
}

func assertWorkspaceAuthorityInitialRegistrationPrivateStateExceptRegistry(t *testing.T, fixture workspaceAuthorityInitialRegistrationFixture) {
	t.Helper()
	assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t, fixture.workspacesRoot,
		"registry.lock",
		"registry.private.json",
		testInitialRegistrationAuthorityID,
	)
	assertWorkspaceAuthorityInitialRegistrationDirectory(t, fixture.authorityDir, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationDirectory(t, fixture.policyDir, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.bootstrap, fixture.bootstrapRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.ownerLock, nil, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.policy, fixture.policyRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationFile(t, fixture.workspaceAuthority, fixture.workspaceRaw, fixture.ownerUID)
	assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t, fixture.authorityDir,
		"admission-policies",
		"owner.lock",
		"workspace.bootstrap.json",
		"workspace.private.json",
	)
	assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t, fixture.policyDir, "1.json")
}

func assertWorkspaceAuthorityInitialRegistrationDirectory(t *testing.T, path string, ownerUID uint32) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("private registration directory %q stat type = %T, want *syscall.Stat_t", path, info.Sys())
	}
	if !info.IsDir() || !authorityPrivateModeIsExact(info.Mode(), authorityPrivateDirectoryMode) || stat.Uid != ownerUID {
		t.Fatalf("private registration directory %q mode=%v uid=%d, want directory 0700 owner %d", path, info.Mode(), stat.Uid, ownerUID)
	}
}

func assertWorkspaceAuthorityInitialRegistrationFile(t *testing.T, path string, want []byte, ownerUID uint32) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("private registration file %q stat type = %T, want *syscall.Stat_t", path, info.Sys())
	}
	if !info.Mode().IsRegular() || !authorityPrivateModeIsExact(info.Mode(), authorityPrivateFileMode) || stat.Uid != ownerUID || stat.Nlink != 1 {
		t.Fatalf("private registration file %q mode=%v uid=%d links=%d, want regular 0600 owner %d single-link", path, info.Mode(), stat.Uid, stat.Nlink, ownerUID)
	}
	if raw := readWorkspaceAuthorityInitialRegistrationFile(t, path); !bytes.Equal(raw, want) {
		t.Fatalf("private registration file %q bytes\n got: %s\nwant: %s", path, raw, want)
	}
}

func assertWorkspaceAuthorityInitialRegistrationDirectoryEntries(t *testing.T, path string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(entries))
	for index, entry := range entries {
		got[index] = entry.Name()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registration directory %q entries = %#v, want %#v", path, got, want)
	}
}

func readWorkspaceAuthorityInitialRegistrationFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type workspaceAuthorityInitialRegistrationFileStat struct {
	device uint64
	inode  uint64
}

func workspaceAuthorityInitialRegistrationStat(t *testing.T, path string) workspaceAuthorityInitialRegistrationFileStat {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("registration stat %q type = %T, want *syscall.Stat_t", path, info.Sys())
	}
	return workspaceAuthorityInitialRegistrationFileStat{device: uint64(stat.Dev), inode: stat.Ino}
}

func workspaceAuthorityInitialRegistrationPaths(fixture workspaceAuthorityInitialRegistrationFixture) []string {
	return []string{
		fixture.hostRoot,
		fixture.workspacesRoot,
		fixture.registryLock,
		fixture.registry,
		fixture.workspace,
		fixture.authorityDir,
		fixture.ownerLock,
		fixture.bootstrap,
		fixture.policyDir,
		fixture.policy,
		fixture.workspaceAuthority,
	}
}
