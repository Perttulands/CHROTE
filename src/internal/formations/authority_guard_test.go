package formations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
)

const (
	testWorkspaceAuthorityID = "wsa_01KXNP6VY3227H78329V52CKF8"
	testAuthorityRunID       = "run_01KXNP6VY3227H78329V52CKF8"
	testOtherAuthorityRunID  = "run_01KXNP6VY3227H78329V52CKF9"
)

func TestGuardRuntimeAuthorityV1ValidDisabledFixtureIsNonAuthorizingAndReadOnly(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)
	t.Setenv("CHROTE_DATA_DIR", filepath.Join(t.TempDir(), "bait"))
	t.Setenv("CHROTE_FORMATIONS_DATA_ROOT", filepath.Join(t.TempDir(), "bait-formations"))

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard valid disabled fixture: %v", err)
	}
	if result.Capability.ID != RuntimeAuthorityGuardCapabilityV1 {
		t.Fatalf("capability id = %q, want %q", result.Capability.ID, RuntimeAuthorityGuardCapabilityV1)
	}
	if result.Capability.AuthoritySchema != 2 {
		t.Fatalf("authority schema = %d, want 2", result.Capability.AuthoritySchema)
	}
	if result.Capability.Authorizing || result.Capability.SemanticProjection || result.Capability.Recovery || result.Capability.Cleanup || result.Capability.Quarantine || result.Capability.Fencing || result.Capability.Execution {
		t.Fatalf("guard capability authorized runtime behavior: %+v", result.Capability)
	}
	if result.Ledgers.Schema1Inspection != 0 || result.Ledgers.Schema2Guarded != 1 {
		t.Fatalf("ledger classifications = %+v, want one guarded schema-2 ledger", result.Ledgers)
	}
	if got, want := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace), before; !reflect.DeepEqual(got, want) {
		t.Fatalf("guard mutated authority fixture\nbefore: %#v\nafter:  %#v", want, got)
	}
	for _, lockName := range []string{"registry.lock", filepath.Join(testWorkspaceAuthorityID, "owner.lock")} {
		if _, statErr := os.Lstat(filepath.Join(fixture.root, lockName)); !os.IsNotExist(statErr) {
			t.Fatalf("guard created or touched absent lock %q: %v", lockName, statErr)
		}
	}
}

func TestGuardRuntimeWorkspaceAuthorityV1MatchesOpenedWorkspaceIdentity(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, fixture.workspace)
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)
	t.Setenv("CHROTE_WORKDIR", filepath.Join(t.TempDir(), "workspace-bait"))
	t.Setenv("CHROTE_ROOTS", filepath.Join(t.TempDir(), "roots-bait"))

	result, err := GuardRuntimeWorkspaceAuthorityV1(filepath.Dir(fixture.root), fixture.workspace)
	if err != nil {
		t.Fatalf("guard matching workspace authority: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	if result.Ledgers.Schema1Inspection != 0 || result.Ledgers.Schema2Guarded != 1 {
		t.Fatalf("selected ledger summary = %+v, want one guarded schema-2 ledger", result.Ledgers)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("workspace guard mutated state\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeWorkspaceAuthorityV1RejectsAuthorityRootWorkspaceOverlap(t *testing.T) {
	tests := []struct {
		name  string
		paths func(*testing.T) (string, string)
	}{
		{
			name: "authority root inside workspace",
			paths: func(t *testing.T) (string, string) {
				workspace := t.TempDir()
				root := filepath.Join(workspace, ".host-private-formations")
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				return root, workspace
			},
		},
		{
			name: "workspace inside authority root",
			paths: func(t *testing.T) (string, string) {
				root := t.TempDir()
				workspace := filepath.Join(root, "workspace")
				if err := os.Mkdir(workspace, 0o700); err != nil {
					t.Fatal(err)
				}
				return root, workspace
			},
		},
		{
			name: "authority root inside resolved workspace",
			paths: func(t *testing.T) (string, string) {
				base := t.TempDir()
				resolvedWorkspace := filepath.Join(base, "resolved-workspace")
				if err := os.Mkdir(resolvedWorkspace, 0o700); err != nil {
					t.Fatal(err)
				}
				configuredWorkspace := filepath.Join(base, "configured-workspace")
				if err := os.Symlink(resolvedWorkspace, configuredWorkspace); err != nil {
					t.Fatal(err)
				}
				root := filepath.Join(resolvedWorkspace, ".host-private-formations")
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				return root, configuredWorkspace
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, workspace := test.paths(t)
			before := snapshotRuntimeAuthorityFixture(t, root, workspace)
			result, err := GuardRuntimeWorkspaceAuthorityV1(root, workspace)
			assertRuntimeGuardDisabled(t, result.Capability)
			var guardErr *RuntimeAuthorityGuardError
			if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRoot || guardErr.Code != RuntimeAuthorityGuardConflict {
				t.Fatalf("overlapping authority root error = %#v, want typed root conflict", err)
			}
			if got := snapshotRuntimeAuthorityFixture(t, root, workspace); !reflect.DeepEqual(got, before) {
				t.Fatalf("overlap rejection mutated state\nbefore: %#v\nafter:  %#v", before, got)
			}
		})
	}
}

func TestRuntimeAuthorityWorkspaceIsolationRejectsRenamedOpenedRoot(t *testing.T) {
	base := t.TempDir()
	workspacePath := filepath.Join(base, "workspace")
	rootPath := filepath.Join(base, "formations-data")
	for _, path := range []string{workspacePath, rootPath} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := openRuntimeWorkspaceIdentity(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openRuntimeAuthorityRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(rootPath, rootPath+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := validateRuntimeAuthorityWorkspaceIsolation(rootPath, root, workspace); !errors.Is(err, errRuntimeConflict) {
		t.Fatalf("renamed authority-root descriptor error = %v, want conflict", err)
	}
}

func TestGuardRuntimeWorkspaceAuthorityV1RejectsAliasAndChangedTarget(t *testing.T) {
	t.Run("alias cannot select registered workspace", func(t *testing.T) {
		fixture := newRuntimeAuthorityFixture(t)
		bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, fixture.workspace)
		alias := filepath.Join(filepath.Dir(fixture.workspace), "workspace-alias")
		if err := os.Symlink(fixture.workspace, alias); err != nil {
			t.Fatal(err)
		}
		before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, alias)

		result, err := GuardRuntimeWorkspaceAuthorityV1(filepath.Dir(fixture.root), alias)
		if err == nil {
			t.Fatal("workspace alias selected registered authority")
		}
		assertRuntimeGuardDisabled(t, result.Capability)
		var guardErr *RuntimeAuthorityGuardError
		if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRegistry || guardErr.Code != RuntimeAuthorityGuardConflict {
			t.Fatalf("alias guard error = %#v, want typed registry conflict", err)
		}
		if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, alias); !reflect.DeepEqual(got, before) {
			t.Fatalf("alias rejection mutated state\nbefore: %#v\nafter:  %#v", before, got)
		}
	})

	t.Run("changed symlink target cannot retain authority", func(t *testing.T) {
		fixture := newRuntimeAuthorityFixture(t)
		configured := filepath.Join(filepath.Dir(fixture.workspace), "configured-workspace")
		if err := os.Symlink(fixture.workspace, configured); err != nil {
			t.Fatal(err)
		}
		bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, configured)
		replacement := filepath.Join(filepath.Dir(fixture.workspace), "replacement-workspace")
		if err := os.Mkdir(replacement, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(configured); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(replacement, configured); err != nil {
			t.Fatal(err)
		}
		before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, configured, replacement)

		result, err := GuardRuntimeWorkspaceAuthorityV1(filepath.Dir(fixture.root), configured)
		if err == nil {
			t.Fatal("changed workspace symlink target retained authority")
		}
		assertRuntimeGuardDisabled(t, result.Capability)
		var guardErr *RuntimeAuthorityGuardError
		if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRegistry || guardErr.Code != RuntimeAuthorityGuardIntegrityMismatch {
			t.Fatalf("retarget guard error = %#v, want typed registry integrity mismatch", err)
		}
		if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, configured, replacement); !reflect.DeepEqual(got, before) {
			t.Fatalf("retarget rejection mutated state\nbefore: %#v\nafter:  %#v", before, got)
		}
	})
}

func TestGuardRuntimeWorkspaceAuthorityV1RejectsNonDirectoryTargetsWithoutOpeningStreams(t *testing.T) {
	base := t.TempDir()
	dataRoot := filepath.Join(base, "formations-data")
	regular := filepath.Join(base, "regular")
	if err := os.WriteFile(regular, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(base, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	fifoAlias := filepath.Join(base, "fifo-alias")
	if err := os.Symlink(fifo, fifoAlias); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{regular, fifo, fifoAlias, "/dev/null"} {
		t.Run(filepath.Base(target), func(t *testing.T) {
			result, err := GuardRuntimeWorkspaceAuthorityV1(dataRoot, target)
			if err == nil {
				t.Fatalf("non-directory workspace target %q was accepted", target)
			}
			assertRuntimeGuardDisabled(t, result.Capability)
			var guardErr *RuntimeAuthorityGuardError
			if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRegistry {
				t.Fatalf("non-directory target error = %#v, want typed registry rejection", err)
			}
		})
	}
	if raw, err := os.ReadFile(regular); err != nil || string(raw) != "sentinel" {
		t.Fatalf("regular target changed: raw=%q err=%v", raw, err)
	}
}

func TestGuardRuntimeWorkspaceAuthorityV1ReportsMissingExactMapping(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	bindRuntimeAuthorityFixtureToOpenedWorkspace(t, &fixture, fixture.workspace)
	unregistered := filepath.Join(filepath.Dir(fixture.workspace), "unregistered-workspace")
	if err := os.Mkdir(unregistered, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := GuardRuntimeWorkspaceAuthorityV1(filepath.Dir(fixture.root), unregistered)
	assertRuntimeGuardDisabled(t, result.Capability)
	var guardErr *RuntimeAuthorityGuardError
	if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRegistry || guardErr.Code != RuntimeAuthorityGuardMissing {
		t.Fatalf("unregistered workspace error = %#v, want typed registry missing", err)
	}
}

func TestRuntimeWorkspaceIdentityHashUsesCanonicalUTF8JSONStringBytes(t *testing.T) {
	separator := "\u2028"
	identity := runtimeWorkspaceIdentity{
		configuredPath: "/tmp/quo\"te&snow-雪" + separator + "\x01",
		resolvedPath:   "/real/quo\"te&snow-雪" + separator + "\x01",
		device:         7,
		inode:          9,
	}
	canonical := `{"configuredPath":"/tmp/quo\"te&snow-雪` + separator + `\u0001","device":"7","inode":"9","resolvedPath":"/real/quo\"te&snow-雪` + separator + `\u0001"}`
	if got, want := runtimeWorkspaceIdentityHash(identity), runtimeSHA256Hex([]byte(canonical)); got != want {
		t.Fatalf("workspace identity hash = %s, want hash of exact canonical UTF-8 bytes %q (%s)", got, canonical, want)
	}
}

func TestGuardRuntimeAuthorityV1RejectsRunDirectoryEventIdentityMismatch(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	originalRunDir := filepath.Dir(fixture.ledger)
	mismatchedRunDir := filepath.Join(filepath.Dir(originalRunDir), testOtherAuthorityRunID)
	if err := os.Rename(originalRunDir, mismatchedRunDir); err != nil {
		t.Fatal(err)
	}
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err == nil {
		t.Fatal("guard accepted an event whose runId did not match its authority directory")
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	var guardErr *RuntimeAuthorityGuardError
	if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageEventEnvelope || guardErr.Code != RuntimeAuthorityGuardConflict {
		t.Fatalf("guard error = %#v, want typed event-envelope conflict", err)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected guard call mutated authority fixture\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeAuthorityV1RejectsNullRegistryEntries(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	writeAuthorityFixture(t, fixture.registry, []byte(`{"registrySchema":1,"recordRev":1,"entries":null}`))
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err == nil {
		t.Fatal("guard accepted null registry entries as an empty closed registry")
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	var guardErr *RuntimeAuthorityGuardError
	if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRegistry {
		t.Fatalf("guard error = %#v, want typed registry rejection", err)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected guard call mutated authority fixture\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeAuthorityV1DoesNotTreatExplicitNullSchemaAsLegacy(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	writeAuthorityFixture(t, fixture.ledger, []byte(fmt.Sprintf(
		`{"schema":null,"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started","actor":"agent:test"}`+"\n",
		testAuthorityRunID,
	)))
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err == nil {
		t.Fatal("guard classified an explicit null schema as schema-absent legacy input")
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	var guardErr *RuntimeAuthorityGuardError
	if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageEventEnvelope || guardErr.Code != RuntimeAuthorityGuardMalformed {
		t.Fatalf("guard error = %#v, want typed malformed event-envelope rejection", err)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected guard call mutated authority fixture\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeAuthorityV1RejectsCaseVariantClosedKeys(t *testing.T) {
	tests := []struct {
		name   string
		stage  RuntimeAuthorityGuardStage
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name:  "registry key alias",
			stage: RuntimeAuthorityGuardStageRegistry,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"registrySchema":1`, `"RegistrySchema":1`)
			},
		},
		{
			name:  "registry canonical plus case variant",
			stage: RuntimeAuthorityGuardStageRegistry,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"registrySchema":1`, `"registrySchema":1,"RegistrySchema":1`)
			},
		},
		{
			name:  "bootstrap key alias",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.bootstrap, `"bootstrapSchema":1`, `"BootstrapSchema":1`)
			},
		},
		{
			name:  "workspace authority key alias",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"authoritySchema":2`, `"AuthoritySchema":2`)
			},
		},
		{
			name:  "nested workspace authority key alias",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"policyRev":1`, `"PolicyRev":1`)
			},
		},
		{
			name:  "policy key alias",
			stage: RuntimeAuthorityGuardStageAdmissionPolicy,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"PolicyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name:  "event key aliases are not legacy",
			stage: RuntimeAuthorityGuardStageEventEnvelope,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"schema":2,"authoritySchema":2,"writerFence":1`, `"Schema":2,"AuthoritySchema":2,"WriterFence":1`)
			},
		},
		{
			name:  "event canonical plus case variant",
			stage: RuntimeAuthorityGuardStageEventEnvelope,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"writerFence":1`, `"writerFence":1,"WriterFence":1`)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, test.stage)
			if guardErr.Code != RuntimeAuthorityGuardUnknownKey {
				t.Fatalf("guard code = %q, want unknown_key for non-exact closed key", guardErr.Code)
			}
		})
	}
}

func TestRuntimeAuthorityClosedJSONSchemasRequireExplicitRegistration(t *testing.T) {
	type convenienceStruct struct {
		MayDispatch bool `json:"mayDispatch"`
	}

	var destination convenienceStruct
	if err := decodeRuntimeAuthorityJSON([]byte(`{}`), &destination); err == nil {
		t.Fatal("unregistered Go struct silently became an authority decode schema")
	}
}

func TestGuardRuntimeAuthorityV1ClassifiesMalformedJSONSeparatelyFromDuplicateKeys(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name: "truncated JSON",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `}]}`, `}]`)
			},
		},
		{
			name: "duplicate before later truncation",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				writeAuthorityFixture(t, fixture.registry, []byte(`{"registrySchema":1,"registrySchema":1`))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageRegistry)
			if guardErr.Code != RuntimeAuthorityGuardMalformed {
				t.Fatalf("guard code = %q, want malformed for invalid JSON", guardErr.Code)
			}
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsInvalidRegistry(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name: "unknown top-level key",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"entries":`, `"futureAuthority":true,"entries":`)
			},
		},
		{
			name: "duplicate top-level key",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"registrySchema":1`, `"registrySchema":1,"registrySchema":1`)
			},
		},
		{
			name: "duplicate nested key",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"device":"123"`, `"device":"123","device":"123"`)
			},
		},
		{
			name: "future registry schema",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"registrySchema":1`, `"registrySchema":2`)
			},
		},
		{
			name: "non-integer record revision",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"recordRev":1`, `"recordRev":1.0`)
			},
		},
		{
			name: "record revision above JSON-safe maximum",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"recordRev":1`, `"recordRev":9007199254740992`)
			},
		},
		{
			name: "noncanonical authority id",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, testWorkspaceAuthorityID, "wsa_81KXNP6VY3227H78329V52CKF8")
			},
		},
		{
			name: "noncanonical hash",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, fixture.rootHash, strings.ToUpper(fixture.rootHash))
			},
		},
		{
			name: "relative configured path",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, fixture.configuredPath, "relative/workspace")
			},
		},
		{
			name: "configured path with dot segment",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, fixture.configuredPath, fixture.configuredPath+"/../workspace")
			},
		},
		{
			name: "configured path with backslash separator",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, fmt.Sprintf("%q", fixture.configuredPath), `"/work\\alias"`)
			},
		},
		{
			name: "configured path with lone escaped surrogate",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, fmt.Sprintf("%q", fixture.configuredPath), `"/work/\ud800"`)
			},
		},
		{
			name: "configured path with lone escaped low surrogate",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, fmt.Sprintf("%q", fixture.configuredPath), `"/work/\udc00"`)
			},
		},
		{
			name: "configured path with two escaped high surrogates",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, fmt.Sprintf("%q", fixture.configuredPath), `"/work/\ud800\ud800"`)
			},
		},
		{
			name: "device leading zero",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"device":"123"`, `"device":"0123"`)
			},
		},
		{
			name: "device as JSON number",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"device":"123"`, `"device":123`)
			},
		},
		{
			name: "device overflows uint64",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.registry, `"device":"123"`, `"device":"18446744073709551616"`)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageRegistry)
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsRegistryIdentityConflictsAndUnsortedEntries(t *testing.T) {
	otherID := "wsa_01KXNP6VY3227H78329V52CKF9"
	otherHash := strings.Repeat("b", 64)
	tests := []struct {
		name   string
		entry1 string
		entry2 string
	}{
		{
			name:   "numeric order",
			entry1: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/a","device":"10","inode":"1","workspaceRootIdentitySha256":"%s"}`, testWorkspaceAuthorityID, strings.Repeat("a", 64)),
			entry2: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/b","device":"2","inode":"1","workspaceRootIdentitySha256":"%s"}`, otherID, otherHash),
		},
		{
			name:   "duplicate configured path",
			entry1: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/a","device":"1","inode":"1","workspaceRootIdentitySha256":"%s"}`, testWorkspaceAuthorityID, strings.Repeat("a", 64)),
			entry2: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/a","device":"2","inode":"1","workspaceRootIdentitySha256":"%s"}`, otherID, otherHash),
		},
		{
			name:   "duplicate opened identity",
			entry1: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/a","device":"1","inode":"1","workspaceRootIdentitySha256":"%s"}`, testWorkspaceAuthorityID, strings.Repeat("a", 64)),
			entry2: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/b","device":"1","inode":"1","workspaceRootIdentitySha256":"%s"}`, otherID, otherHash),
		},
		{
			name:   "authority id mapped twice",
			entry1: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/a","device":"1","inode":"1","workspaceRootIdentitySha256":"%s"}`, testWorkspaceAuthorityID, strings.Repeat("a", 64)),
			entry2: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/b","device":"2","inode":"1","workspaceRootIdentitySha256":"%s"}`, testWorkspaceAuthorityID, otherHash),
		},
		{
			name:   "root identity hash mapped twice",
			entry1: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/a","device":"1","inode":"1","workspaceRootIdentitySha256":"%s"}`, testWorkspaceAuthorityID, otherHash),
			entry2: fmt.Sprintf(`{"workspaceAuthorityId":"%s","configuredPath":"/work/b","device":"2","inode":"1","workspaceRootIdentitySha256":"%s"}`, otherID, otherHash),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			writeAuthorityFixture(t, fixture.registry, []byte(fmt.Sprintf(
				`{"registrySchema":1,"recordRev":1,"entries":[%s,%s]}`,
				test.entry1,
				test.entry2,
			)))
			assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageRegistry)
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsInvalidBootstrapAndWorkspaceAuthority(t *testing.T) {
	tests := []struct {
		name   string
		stage  RuntimeAuthorityGuardStage
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name:  "bootstrap unknown key",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.bootstrap, `"bootstrapSchema":1`, `"bootstrapSchema":1,"maySelectOwner":true`)
			},
		},
		{
			name:  "bootstrap duplicate key",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.bootstrap, `"bootstrapSchema":1`, `"bootstrapSchema":1,"bootstrapSchema":1`)
			},
		},
		{
			name:  "bootstrap carries mutable authority schema",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.bootstrap, `"bootstrapSchema":1`, `"bootstrapSchema":1,"authoritySchema":2`)
			},
		},
		{
			name:  "future bootstrap schema",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.bootstrap, `"bootstrapSchema":1`, `"bootstrapSchema":2`)
			},
		},
		{
			name:  "bootstrap trailing newline is not canonical JCS",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.bootstrap)
				if err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, fixture.bootstrap, append(raw, '\n'))
			},
		},
		{
			name:  "bootstrap key order is not canonical JCS",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw := fmt.Sprintf(
					`{"workspaceAuthorityId":"%s","bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceRootIdentitySha256":"%s"}`,
					testWorkspaceAuthorityID,
					fixture.rootHash,
				)
				writeAuthorityFixture(t, fixture.bootstrap, []byte(raw))
			},
		},
		{
			name:  "bootstrap authority id mismatch",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.bootstrap, testWorkspaceAuthorityID, "wsa_01KXNP6VY3227H78329V52CKF9")
			},
		},
		{
			name:  "bootstrap root hash mismatch",
			stage: RuntimeAuthorityGuardStageBootstrap,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.bootstrap, fixture.rootHash, strings.Repeat("b", 64))
			},
		},
		{
			name:  "workspace authority missing",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				if err := os.Remove(fixture.workspaceDB); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:  "workspace unknown key",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"authoritySchema":2`, `"authoritySchema":2,"mayFence":true`)
			},
		},
		{
			name:  "workspace duplicate authority schema",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"authoritySchema":2`, `"authoritySchema":2,"authoritySchema":2`)
			},
		},
		{
			name:  "future authority high-water",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"authoritySchema":2`, `"authoritySchema":3`)
			},
		},
		{
			name:  "lower authority high-water",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"authoritySchema":2`, `"authoritySchema":1`)
			},
		},
		{
			name:  "workspace record revision zero",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"recordRev":1`, `"recordRev":0`)
			},
		},
		{
			name:  "workspace writer counter fractional",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"nextWriterFence":2`, `"nextWriterFence":2.5`)
			},
		},
		{
			name:  "workspace admission counter above JSON-safe maximum",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, `"nextAdmissionSeq":1`, `"nextAdmissionSeq":9007199254740992`)
			},
		},
		{
			name:  "workspace authority id mismatch",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, testWorkspaceAuthorityID, "wsa_01KXNP6VY3227H78329V52CKF9")
			},
		},
		{
			name:  "workspace root hash mismatch",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, fixture.rootHash, strings.Repeat("b", 64))
			},
		},
		{
			name:  "workspace policy hash malformed",
			stage: RuntimeAuthorityGuardStageWorkspaceAuthority,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, fixture.policyHash, "sha256:"+fixture.policyHash)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			assertRuntimeGuardRejectsUnchanged(t, fixture, test.stage)
		})
	}
}

func TestGuardRuntimeAuthorityV1ValidConfiguredPolicyChainRemainsNonAuthorizing(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	policy := []byte(fmt.Sprintf(
		`{"maxActiveRuns":2,"maxQueuedRuns":0,"policyRev":2,"policySchema":1,"priorPolicySha256":"%s","state":"configured"}`,
		fixture.policyHash,
	))
	setRuntimeAuthorityCurrentPolicy(t, fixture, 2, policy)
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard valid configured policy chain: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	if result.Ledgers.Schema1Inspection != 0 || result.Ledgers.Schema2Guarded != 1 {
		t.Fatalf("ledger classifications = %+v, want one guarded schema-2 ledger", result.Ledgers)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("guard mutated configured authority fixture\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeAuthorityV1AllowsUnreferencedInstalledNextPolicy(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	policy := []byte(fmt.Sprintf(
		`{"maxActiveRuns":2,"maxQueuedRuns":1,"policyRev":2,"policySchema":1,"priorPolicySha256":"%s","state":"configured"}`,
		fixture.policyHash,
	))
	writeAuthorityFixture(t, filepath.Join(fixture.policyDir, "2.json"), policy)
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard rejected unreferenced exact next policy generation: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("guard mutated unreferenced-policy fixture\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeAuthorityV1RejectsInvalidAdmissionPolicyChain(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name: "current generation missing",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				if err := os.Remove(filepath.Join(fixture.policyDir, "1.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "current hash mismatch",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.workspaceDB, fixture.policyHash, strings.Repeat("b", 64))
			},
		},
		{
			name: "policy trailing newline is not canonical JCS",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}` + "\n")
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "policy key order is not canonical JCS",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"policySchema":1,"policyRev":1,"priorPolicySha256":"","state":"disabled"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "unknown policy key",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled","mayAdmit":true}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "duplicate policy key",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"policyRev":1,"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "future policy schema",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"policyRev":1,"policySchema":2,"priorPolicySha256":"","state":"disabled"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "file revision mismatch",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(fmt.Sprintf(`{"maxActiveRuns":1,"maxQueuedRuns":0,"policyRev":2,"policySchema":1,"priorPolicySha256":"%s","state":"configured"}`, fixture.policyHash))
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "revision one carries prior hash",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(fmt.Sprintf(`{"policyRev":1,"policySchema":1,"priorPolicySha256":"%s","state":"disabled"}`, strings.Repeat("b", 64)))
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "unknown state",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"open"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "disabled carries limits",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"maxActiveRuns":1,"maxQueuedRuns":0,"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "configured missing queue limit",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"maxActiveRuns":1,"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"configured"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "configured active limit zero",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"maxActiveRuns":0,"maxQueuedRuns":0,"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"configured"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "configured queue limit negative",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"maxActiveRuns":1,"maxQueuedRuns":-1,"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"configured"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "configured limit above maximum",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(`{"maxActiveRuns":2147483648,"maxQueuedRuns":0,"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"configured"}`)
				setRuntimeAuthorityCurrentPolicy(t, fixture, 1, body)
			},
		},
		{
			name: "prior hash mismatch",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				body := []byte(fmt.Sprintf(`{"maxActiveRuns":1,"maxQueuedRuns":0,"policyRev":2,"policySchema":1,"priorPolicySha256":"%s","state":"configured"}`, strings.Repeat("b", 64)))
				setRuntimeAuthorityCurrentPolicy(t, fixture, 2, body)
			},
		},
		{
			name: "missing prior generation",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				p2Hash := strings.Repeat("b", 64)
				body := []byte(fmt.Sprintf(`{"maxActiveRuns":1,"maxQueuedRuns":0,"policyRev":3,"policySchema":1,"priorPolicySha256":"%s","state":"configured"}`, p2Hash))
				setRuntimeAuthorityCurrentPolicy(t, fixture, 3, body)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageAdmissionPolicy)
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsUnsafeEventEnvelopes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name: "missing authority schema",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"authoritySchema":2,`, "")
			},
		},
		{
			name: "duplicate schema",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"schema":2`, `"schema":2,"schema":2`)
			},
		},
		{
			name: "unknown authority-changing top-level field",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"mayDispatch":true,"data":{}`)
			},
		},
		{
			name: "future event schema",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"schema":2`, `"schema":3`)
			},
		},
		{
			name: "future event authority schema",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"authoritySchema":2`, `"authoritySchema":3`)
			},
		},
		{
			name: "null event authority schema",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"authoritySchema":2`, `"authoritySchema":null`)
			},
		},
		{
			name: "writer fence zero",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"writerFence":1`, `"writerFence":0`)
			},
		},
		{
			name: "writer fence fractional",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"writerFence":1`, `"writerFence":1.5`)
			},
		},
		{
			name: "writer fence above JSON-safe maximum",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"writerFence":1`, `"writerFence":9007199254740992`)
			},
		},
		{
			name: "first sequence is not one",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"seq":1`, `"seq":2`)
			},
		},
		{
			name: "missing actor",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `,"actor":"agent:test"`, "")
			},
		},
		{
			name: "invalid timestamp",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, "2026-07-18T00:00:00Z", "not-a-time")
			},
		},
		{
			name: "explicit schema one is unsupported",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"schema":2`, `"schema":1`)
			},
		},
		{
			name: "mixed schema one and two lines",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.ledger)
				if err != nil {
					t.Fatal(err)
				}
				legacy := fmt.Sprintf(`{"ts":"2026-07-18T00:00:01Z","runId":"%s","seq":2,"type":"run_succeeded","actor":"agent:test"}`+"\n", testAuthorityRunID)
				writeAuthorityFixture(t, fixture.ledger, append(raw, legacy...))
			},
		},
		{
			name: "mixed authority schemas",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.ledger)
				if err != nil {
					t.Fatal(err)
				}
				second := schema2AuthorityEvent(testAuthorityRunID, 2, 1, 3)
				writeAuthorityFixture(t, fixture.ledger, append(raw, second...))
			},
		},
		{
			name: "writer fence regression",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"writerFence":1`, `"writerFence":2`)
				raw, err := os.ReadFile(fixture.ledger)
				if err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, fixture.ledger, append(raw, schema2AuthorityEvent(testAuthorityRunID, 2, 1, 2)...))
			},
		},
		{
			name: "duplicate sequence",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.ledger)
				if err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, fixture.ledger, append(raw, schema2AuthorityEvent(testAuthorityRunID, 1, 1, 2)...))
			},
		},
		{
			name: "run id changes within ledger",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.ledger)
				if err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, fixture.ledger, append(raw, schema2AuthorityEvent(testOtherAuthorityRunID, 2, 1, 2)...))
			},
		},
		{
			name: "blank interior line",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.ledger)
				if err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, fixture.ledger, append(raw, '\n'))
			},
		},
		{
			name: "two JSON values on one line",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.ledger)
				if err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, fixture.ledger, []byte(strings.TrimSpace(string(raw))+` {}`+"\n"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsInvalidOptionalEventHeaders(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
		wantCode    RuntimeAuthorityGuardCode
	}{
		{name: "null optional string", replacement: `"boardId":null,`, wantCode: RuntimeAuthorityGuardMalformed},
		{name: "null optional number", replacement: `"boardRev":null,`, wantCode: RuntimeAuthorityGuardMalformed},
		{name: "fractional board revision", replacement: `"boardRev":1.5,`, wantCode: RuntimeAuthorityGuardNoncanonical},
		{name: "negative epoch", replacement: `"epoch":-1,`, wantCode: RuntimeAuthorityGuardNoncanonical},
		{name: "attempt above JSON-safe maximum", replacement: `"attempt":9007199254740992,`, wantCode: RuntimeAuthorityGuardOutOfRange},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, test.replacement+`"data":{}`)
			guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
			if guardErr.Code != test.wantCode {
				t.Fatalf("guard code = %q, want %q", guardErr.Code, test.wantCode)
			}
		})
	}
}

func TestGuardRuntimeAuthorityV1AllowsInitialEpochZero(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"epoch":0,"data":{}`)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard rejected canonical initial epoch zero: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
}

func TestGuardRuntimeAuthorityV1RejectsMalformedSchemaAbsentLedgers(t *testing.T) {
	validFirst := fmt.Sprintf(`{"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started","actor":"agent:test"}`, testAuthorityRunID)
	tests := []struct {
		name string
		body string
	}{
		{name: "null event", body: "null\n"},
		{name: "empty object", body: "{}\n"},
		{name: "null sequence", body: fmt.Sprintf(`{"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":null,"type":"run_started","actor":"agent:test"}`+"\n", testAuthorityRunID)},
		{name: "missing actor", body: fmt.Sprintf(`{"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started"}`+"\n", testAuthorityRunID)},
		{name: "sequence gap", body: validFirst + "\n" + fmt.Sprintf(`{"ts":"2026-07-18T00:00:01Z","runId":"%s","seq":3,"type":"run_succeeded","actor":"agent:test"}`+"\n", testAuthorityRunID)},
		{name: "run changes", body: validFirst + "\n" + fmt.Sprintf(`{"ts":"2026-07-18T00:00:01Z","runId":"%s","seq":2,"type":"run_succeeded","actor":"agent:test"}`+"\n", testOtherAuthorityRunID)},
		{name: "directory run mismatch", body: fmt.Sprintf(`{"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started","actor":"agent:test"}`+"\n", testOtherAuthorityRunID)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			writeAuthorityFixture(t, fixture.ledger, []byte(test.body))
			assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
		})
	}
}

func TestGuardRuntimeAuthorityV1ClassifiesSchemaAbsentLegacyInspectionOnly(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	legacy := strings.Join([]string{
		fmt.Sprintf(`{"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started","actor":"agent:test","data":{"legacy":true}}`, testAuthorityRunID),
		fmt.Sprintf(`{"ts":"2026-07-18T00:00:01Z","runId":"%s","seq":2,"type":"run_succeeded","actor":"agent:test","data":{"final":true}}`, testAuthorityRunID),
	}, "\n") + "\n"
	writeAuthorityFixture(t, fixture.ledger, []byte(legacy))
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard legacy inspection fixture: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	if result.Ledgers.Schema1Inspection != 1 || result.Ledgers.Schema2Guarded != 0 {
		t.Fatalf("ledger classifications = %+v, want one schema-absent legacy inspection ledger", result.Ledgers)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("legacy guard mutated authority fixture\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeAuthorityV1SummarizesMixedLedgerClassesWithoutRetainingRunPaths(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	legacyPath := filepath.Join(fixture.authority, "runs", testOtherAuthorityRunID, "events.ndjson")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeAuthorityFixture(t, legacyPath, []byte(fmt.Sprintf(
		`{"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started","actor":"agent:test"}`+"\n",
		testOtherAuthorityRunID,
	)))

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard mixed ledger classes: %v", err)
	}
	if result.Ledgers.Schema1Inspection != 1 || result.Ledgers.Schema2Guarded != 1 {
		t.Fatalf("ledger summary = %+v, want one legacy and one schema-2 ledger", result.Ledgers)
	}
}

func TestGuardRuntimeAuthorityV1KeepsEventDataOpaqueInFoundationSlice(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "future nested key", data: `{"projectionOnlyCandidate":{"future":true}}`},
		{name: "case variant is nested data not envelope", data: `{"Schema":2,"WriterFence":9}`},
		{name: "valid surrogate pair", data: `{"emoji":"\ud83d\ude00"}`},
		{name: "escaped surrogate text", data: `{"literal":"\\ud800"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"data":`+test.data)

			result, err := GuardRuntimeAuthorityV1(fixture.root)
			if err != nil {
				t.Fatalf("guard rejected opaque event-specific data before semantic projector exists: %v", err)
			}
			assertRuntimeGuardDisabled(t, result.Capability)
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsAmbiguousJSONInsideOpaqueEventData(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantCode RuntimeAuthorityGuardCode
	}{
		{name: "duplicate nested key", data: `{"same":1,"same":2}`, wantCode: RuntimeAuthorityGuardDuplicateKey},
		{name: "lone surrogate", data: `{"invalid":"\ud800"}`, wantCode: RuntimeAuthorityGuardMalformed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"data":`+test.data)

			guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
			if guardErr.Code != test.wantCode {
				t.Fatalf("guard code = %q, want %q inside otherwise opaque data", guardErr.Code, test.wantCode)
			}
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsDecodedDuplicateEventKeys(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	replaceAuthorityFixture(t, fixture.ledger, `"schema":2`, `"schema":2,"\u0073chema":2`)

	guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
	if guardErr.Code != RuntimeAuthorityGuardDuplicateKey {
		t.Fatalf("guard code = %q, want duplicate_key for two spellings that decode to the same key", guardErr.Code)
	}
}

func TestGuardRuntimeAuthorityV1EnforcesExactJSONContainerDepth(t *testing.T) {
	t.Run("64 levels accepted", func(t *testing.T) {
		fixture := newRuntimeAuthorityFixture(t)
		nested := strings.Repeat("[", 63) + strings.Repeat("]", 63)
		replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"data":`+nested)

		if _, err := GuardRuntimeAuthorityV1(fixture.root); err != nil {
			t.Fatalf("guard rejected the documented 64-container boundary: %v", err)
		}
	})

	t.Run("65 levels rejected", func(t *testing.T) {
		fixture := newRuntimeAuthorityFixture(t)
		nested := strings.Repeat("[", 64) + strings.Repeat("]", 64)
		replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"data":`+nested)

		guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
		if guardErr.Code != RuntimeAuthorityGuardOutOfRange {
			t.Fatalf("guard code = %q, want out_of_range above 64 containers", guardErr.Code)
		}
	})
}

func TestGuardRuntimeAuthorityV1BoundsAuthorityInputs(t *testing.T) {
	tests := []struct {
		name   string
		stage  RuntimeAuthorityGuardStage
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name:  "oversized mutable record",
			stage: RuntimeAuthorityGuardStageRegistry,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				raw, err := os.ReadFile(fixture.registry)
				if err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, fixture.registry, append(raw, []byte(strings.Repeat(" ", 2<<20))...))
			},
		},
		{
			name:  "oversized event line",
			stage: RuntimeAuthorityGuardStageEventEnvelope,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"data":{"opaque":"`+strings.Repeat("x", 2<<20)+`"}`)
			},
		},
		{
			name:  "excessive JSON depth",
			stage: RuntimeAuthorityGuardStageEventEnvelope,
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				deep := strings.Repeat("[", 80) + "true" + strings.Repeat("]", 80)
				replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"data":{"opaque":`+deep+`}`)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, test.stage)
			if guardErr.Code != RuntimeAuthorityGuardOutOfRange {
				t.Fatalf("guard code = %q, want out_of_range for bounded authority input", guardErr.Code)
			}
		})
	}
}

func TestGuardRuntimeAuthorityV1StreamsLedgerBeyondFormerWholeFileCap(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	const formerWholeLedgerCap = 16 << 20
	padding := strings.Repeat("x", 512<<10)
	var ledger bytes.Buffer
	for sequence := 1; ledger.Len() <= formerWholeLedgerCap; sequence++ {
		fmt.Fprintf(
			&ledger,
			`{"schema":2,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":%d,"type":"observation","actor":"agent:test","data":{"padding":"%s"}}`+"\n",
			testAuthorityRunID,
			sequence,
			padding,
		)
	}
	writeAuthorityFixture(t, fixture.ledger, ledger.Bytes())

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard imposed a whole-ledger history cap instead of streaming events: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
}

func TestGuardRuntimeAuthorityV1AllowsPolicyRevisionAboveFormerTraversalCap(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	const currentRevision = 4097
	priorHash := ""
	var currentPolicy []byte
	for revision := 1; revision <= currentRevision; revision++ {
		currentPolicy = []byte(fmt.Sprintf(
			`{"policyRev":%d,"policySchema":1,"priorPolicySha256":"%s","state":"disabled"}`,
			revision,
			priorHash,
		))
		writeAuthorityFixture(t, filepath.Join(fixture.policyDir, fmt.Sprintf("%d.json", revision)), currentPolicy)
		priorHash = sha256Hex(currentPolicy)
	}
	setRuntimeAuthorityCurrentPolicy(t, fixture, currentRevision, currentPolicy)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard imposed a policy-revision cap below the JSON-safe authority domain: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
}

func TestRuntimeAuthorityDirectoryEnumerationIsBatched(t *testing.T) {
	directoryPath := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		writeAuthorityFixture(t, filepath.Join(directoryPath, name), nil)
	}
	directory, err := os.Open(directoryPath)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()

	var names []string
	for {
		batch, done, err := readRuntimeAuthorityDirectoryNameBatch(directory, 2)
		if err != nil {
			t.Fatalf("batched directory read: %v", err)
		}
		names = append(names, batch...)
		if done {
			break
		}
	}
	sort.Strings(names)
	if want := []string{"one", "three", "two"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("batched directory names = %v, want %v", names, want)
	}
}

func TestGuardRuntimeAuthorityV1RejectsFieldPermissiveRunEventWouldIgnore(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	replaceAuthorityFixture(t, fixture.ledger, `"data":{}`, `"mayDispatch":true,"data":{}`)
	raw, err := os.ReadFile(fixture.ledger)
	if err != nil {
		t.Fatal(err)
	}
	var permissive RunEvent
	if err := json.Unmarshal(bytes.TrimSpace(raw), &permissive); err != nil {
		t.Fatalf("reproduced permissive reader did not accept unsafe fixture: %v", err)
	}
	guardErr := assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
	if guardErr.Code != RuntimeAuthorityGuardUnknownKey {
		t.Fatalf("guard code = %q, want unknown_key for ignored authority field", guardErr.Code)
	}
}

func TestGuardRuntimeAuthorityV1AcceptsNoncanonicalWhitespaceOnlyForMutableRecords(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	registry := fmt.Sprintf(
		"{\n  \"entries\": [{\"workspaceAuthorityId\":\"%s\",\"configuredPath\":%q,\"device\":\"123\",\"inode\":\"456\",\"workspaceRootIdentitySha256\":\"%s\"}],\n  \"recordRev\": 1,\n  \"registrySchema\": 1\n}\n",
		testWorkspaceAuthorityID,
		fixture.configuredPath,
		fixture.rootHash,
	)
	workspace := fmt.Sprintf(
		"{\n  \"admissionPolicyRef\": {\"policySha256\":\"%s\",\"policyRev\":1},\n  \"nextAdmissionSeq\":1,\n  \"nextWriterFence\":2,\n  \"workspaceRootIdentitySha256\":\"%s\",\n  \"rootIdentityEncoding\":\"workspace-root-identity-v1\",\n  \"workspaceAuthorityId\":\"%s\",\n  \"authoritySchema\":2,\n  \"recordRev\":1\n}\n",
		fixture.policyHash,
		fixture.rootHash,
		testWorkspaceAuthorityID,
	)
	writeAuthorityFixture(t, fixture.registry, []byte(registry))
	writeAuthorityFixture(t, fixture.workspaceDB, []byte(workspace))

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err != nil {
		t.Fatalf("guard invented canonical-byte requirement for mutable records: %v", err)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
}

func TestGuardRuntimeAuthorityV1UsesOnlyExplicitRoot(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	missingParent := t.TempDir()
	missingRoot := filepath.Join(missingParent, "missing-workspaces")
	t.Setenv("CHROTE_DATA_DIR", fixture.root)
	t.Setenv("CHROTE_FORMATIONS_DATA_ROOT", fixture.root)
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, missingParent)

	result, err := GuardRuntimeAuthorityV1(missingRoot)
	if err == nil {
		t.Fatal("guard discovered authority state from the environment instead of the explicit root")
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	var guardErr *RuntimeAuthorityGuardError
	if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRoot || guardErr.Code != RuntimeAuthorityGuardMissing {
		t.Fatalf("guard error = %#v, want typed missing explicit-root rejection", err)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, missingParent); !reflect.DeepEqual(got, before) {
		t.Fatalf("explicit-root rejection mutated state\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestGuardRuntimeAuthorityV1RejectsSymlinkedAuthorityRecords(t *testing.T) {
	tests := []struct {
		name   string
		stage  RuntimeAuthorityGuardStage
		target func(runtimeAuthorityFixture) string
	}{
		{name: "registry", stage: RuntimeAuthorityGuardStageRegistry, target: func(f runtimeAuthorityFixture) string { return f.registry }},
		{name: "bootstrap", stage: RuntimeAuthorityGuardStageBootstrap, target: func(f runtimeAuthorityFixture) string { return f.bootstrap }},
		{name: "workspace authority", stage: RuntimeAuthorityGuardStageWorkspaceAuthority, target: func(f runtimeAuthorityFixture) string { return f.workspaceDB }},
		{name: "policy", stage: RuntimeAuthorityGuardStageAdmissionPolicy, target: func(f runtimeAuthorityFixture) string { return filepath.Join(f.policyDir, "1.json") }},
		{name: "event ledger", stage: RuntimeAuthorityGuardStageEventEnvelope, target: func(f runtimeAuthorityFixture) string { return f.ledger }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			path := test.target(fixture)
			realPath := path + ".real"
			if err := os.Rename(path, realPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(realPath, path); err != nil {
				t.Fatal(err)
			}
			assertRuntimeGuardRejectsUnchanged(t, fixture, test.stage)
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsSymlinkedAuthorityDirectories(t *testing.T) {
	tests := []struct {
		name   string
		stage  RuntimeAuthorityGuardStage
		target func(runtimeAuthorityFixture) string
	}{
		{name: "admission policies", stage: RuntimeAuthorityGuardStageAdmissionPolicy, target: func(f runtimeAuthorityFixture) string { return f.policyDir }},
		{name: "runs", stage: RuntimeAuthorityGuardStageEventEnvelope, target: func(f runtimeAuthorityFixture) string { return filepath.Join(f.authority, "runs") }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			target := test.target(fixture)
			external := target + ".outside"
			if err := os.Rename(target, external); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, target); err != nil {
				t.Fatal(err)
			}
			before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, external)

			result, err := GuardRuntimeAuthorityV1(fixture.root)
			if err == nil {
				t.Fatalf("guard followed symlinked %s directory outside the authority tree", test.name)
			}
			assertRuntimeGuardDisabled(t, result.Capability)
			var guardErr *RuntimeAuthorityGuardError
			if !errors.As(err, &guardErr) || guardErr.Stage != test.stage {
				t.Fatalf("guard error = %#v, want typed %s rejection", err, test.stage)
			}
			if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, external); !reflect.DeepEqual(got, before) {
				t.Fatalf("directory-symlink rejection mutated state\nbefore: %#v\nafter:  %#v", before, got)
			}
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsMalformedRunDirectoryEntries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, runtimeAuthorityFixture)
	}{
		{
			name: "dangling runs symlink",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				runs := filepath.Join(fixture.authority, "runs")
				if err := os.Rename(runs, runs+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(runs+".missing", runs); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked run entry",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				runDir := filepath.Dir(fixture.ledger)
				external := filepath.Join(fixture.authority, "outside-run-entry")
				if err := os.Rename(runDir, external); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, runDir); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "non-directory run entry",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				writeAuthorityFixture(t, filepath.Join(fixture.authority, "runs", "unexpected.private.json"), []byte(`{}`))
			},
		},
		{
			name: "run directory without ledger",
			mutate: func(t *testing.T, fixture runtimeAuthorityFixture) {
				if err := os.Mkdir(filepath.Join(fixture.authority, "runs", testOtherAuthorityRunID), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeAuthorityFixture(t)
			test.mutate(t, fixture)
			assertRuntimeGuardRejectsUnchanged(t, fixture, RuntimeAuthorityGuardStageEventEnvelope)
		})
	}
}

func TestGuardRuntimeAuthorityV1RejectsAuthorityRootWithSymlinkedAncestor(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	dataDir := filepath.Dir(filepath.Dir(fixture.root))
	realDataDir := dataDir + ".real"
	if err := os.Rename(dataDir, realDataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDataDir, dataDir); err != nil {
		t.Fatal(err)
	}
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, realDataDir)

	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err == nil {
		t.Fatal("guard accepted an authority root reached through a symlinked ancestor")
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	var guardErr *RuntimeAuthorityGuardError
	if !errors.As(err, &guardErr) || guardErr.Stage != RuntimeAuthorityGuardStageRoot {
		t.Fatalf("guard error = %#v, want typed root rejection", err)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace, realDataDir); !reflect.DeepEqual(got, before) {
		t.Fatalf("ancestor-symlink rejection mutated state\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func TestRuntimeAuthorityRootReaderPinsOpenedTreeAcrossPathReplacement(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	want, err := os.ReadFile(fixture.registry)
	if err != nil {
		t.Fatal(err)
	}
	rootDir, err := openRuntimeAuthorityRoot(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootDir.Close()

	movedRoot := fixture.root + ".opened"
	if err := os.Rename(fixture.root, movedRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeAuthorityFixture(t, filepath.Join(fixture.root, "registry.private.json"), []byte(`{"outside":true}`))

	got, err := readRuntimeAuthorityFileAt(rootDir, "registry.private.json", runtimeAuthorityMaxRecordBytes)
	if err != nil {
		t.Fatalf("read pinned authority root: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("pinned reader followed path replacement\ngot:  %s\nwant: %s", got, want)
	}
}

func TestGuardRuntimeAuthorityV1ConcurrentReadersRemainReadOnly(t *testing.T) {
	fixture := newRuntimeAuthorityFixture(t)
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)
	const readers = 16
	var wait sync.WaitGroup
	errorsByReader := make(chan error, readers)
	for range readers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := GuardRuntimeAuthorityV1(fixture.root)
			if err == nil && (result.Capability.Authorizing || result.Capability.Execution || result.Ledgers.Schema1Inspection != 0 || result.Ledgers.Schema2Guarded != 1) {
				err = fmt.Errorf("unexpected concurrent guard result: %+v", result)
			}
			errorsByReader <- err
		}()
	}
	wait.Wait()
	close(errorsByReader)
	for err := range errorsByReader {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("concurrent guard readers mutated state\nbefore: %#v\nafter:  %#v", before, got)
	}
}

func assertRuntimeGuardDisabled(t *testing.T, capability RuntimeAuthorityCapability) {
	t.Helper()
	if capability.ID != RuntimeAuthorityGuardCapabilityV1 || capability.AuthoritySchema != 2 || capability.Authorizing || capability.SemanticProjection || capability.Recovery || capability.Cleanup || capability.Quarantine || capability.Fencing || capability.Execution {
		t.Fatalf("runtime authority capability is not the frozen disabled contract: %+v", capability)
	}
}

func assertRuntimeGuardRejectsUnchanged(t *testing.T, fixture runtimeAuthorityFixture, stage RuntimeAuthorityGuardStage) *RuntimeAuthorityGuardError {
	t.Helper()
	before := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace)
	result, err := GuardRuntimeAuthorityV1(fixture.root)
	if err == nil {
		t.Fatalf("guard accepted invalid %s fixture", stage)
	}
	assertRuntimeGuardDisabled(t, result.Capability)
	var guardErr *RuntimeAuthorityGuardError
	if !errors.As(err, &guardErr) || guardErr.Stage != stage {
		t.Fatalf("guard error = %#v, want typed %s rejection", err, stage)
	}
	if got := snapshotRuntimeAuthorityFixture(t, fixture.root, fixture.workspace); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected guard call mutated authority fixture\nbefore: %#v\nafter:  %#v", before, got)
	}
	return guardErr
}

type runtimeAuthorityFixture struct {
	root           string
	workspace      string
	configuredPath string
	rootHash       string
	policyHash     string
	authority      string
	registry       string
	bootstrap      string
	workspaceDB    string
	policyDir      string
	ledger         string
}

func bindRuntimeAuthorityFixtureToOpenedWorkspace(t *testing.T, fixture *runtimeAuthorityFixture, configuredPath string) {
	t.Helper()
	workspace, err := os.Open(configuredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	info, err := workspace.Stat()
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("workspace stat type = %T, want *syscall.Stat_t", info.Sys())
	}
	resolvedPath, err := filepath.EvalSymlinks(configuredPath)
	if err != nil {
		t.Fatal(err)
	}
	configuredPath = filepath.ToSlash(filepath.Clean(configuredPath))
	resolvedPath = filepath.ToSlash(filepath.Clean(resolvedPath))
	device := strconv.FormatUint(uint64(stat.Dev), 10)
	inode := strconv.FormatUint(stat.Ino, 10)
	rootIdentity := fmt.Sprintf(
		`{"configuredPath":%q,"device":%q,"inode":%q,"resolvedPath":%q}`,
		configuredPath,
		device,
		inode,
		resolvedPath,
	)
	rootHash := sha256Hex([]byte(rootIdentity))
	writeAuthorityFixture(t, fixture.registry, []byte(fmt.Sprintf(
		`{"registrySchema":1,"recordRev":1,"entries":[{"workspaceAuthorityId":"%s","configuredPath":%q,"device":%q,"inode":%q,"workspaceRootIdentitySha256":"%s"}]}`,
		testWorkspaceAuthorityID,
		configuredPath,
		device,
		inode,
		rootHash,
	)))
	writeAuthorityFixture(t, fixture.bootstrap, []byte(fmt.Sprintf(
		`{"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		testWorkspaceAuthorityID,
		rootHash,
	)))
	writeAuthorityFixture(t, fixture.workspaceDB, []byte(fmt.Sprintf(
		`{"recordRev":1,"authoritySchema":2,"workspaceAuthorityId":"%s","rootIdentityEncoding":"workspace-root-identity-v1","workspaceRootIdentitySha256":"%s","nextWriterFence":2,"nextAdmissionSeq":1,"admissionPolicyRef":{"policyRev":1,"policySha256":"%s"}}`,
		testWorkspaceAuthorityID,
		rootHash,
		fixture.policyHash,
	)))
	fixture.configuredPath = configuredPath
	fixture.rootHash = rootHash
}

func newRuntimeAuthorityFixture(t *testing.T) runtimeAuthorityFixture {
	t.Helper()
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	root := filepath.Join(base, "data", "formations", "workspaces")
	authority := filepath.Join(root, testWorkspaceAuthorityID)
	policyDir := filepath.Join(authority, "admission-policies")
	ledger := filepath.Join(authority, "runs", testAuthorityRunID, "events.ndjson")
	for _, dir := range []string{workspace, policyDir, filepath.Dir(ledger)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	configuredPath := filepath.ToSlash(workspace)
	rootIdentity := fmt.Sprintf(`{"configuredPath":%q,"device":"123","inode":"456","resolvedPath":%q}`, configuredPath, configuredPath)
	rootHash := sha256Hex([]byte(rootIdentity))
	policy := []byte(`{"policyRev":1,"policySchema":1,"priorPolicySha256":"","state":"disabled"}`)
	policyHash := sha256Hex(policy)
	registryPath := filepath.Join(root, "registry.private.json")
	bootstrapPath := filepath.Join(authority, "workspace.bootstrap.json")
	workspacePath := filepath.Join(authority, "workspace.private.json")

	writeAuthorityFixture(t, registryPath, []byte(fmt.Sprintf(
		`{"registrySchema":1,"recordRev":1,"entries":[{"workspaceAuthorityId":"%s","configuredPath":%q,"device":"123","inode":"456","workspaceRootIdentitySha256":"%s"}]}`,
		testWorkspaceAuthorityID, configuredPath, rootHash,
	)))
	writeAuthorityFixture(t, bootstrapPath, []byte(fmt.Sprintf(
		`{"bootstrapSchema":1,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		testWorkspaceAuthorityID, rootHash,
	)))
	writeAuthorityFixture(t, filepath.Join(policyDir, "1.json"), policy)
	writeAuthorityFixture(t, workspacePath, []byte(fmt.Sprintf(
		`{"recordRev":1,"authoritySchema":2,"workspaceAuthorityId":"%s","rootIdentityEncoding":"workspace-root-identity-v1","workspaceRootIdentitySha256":"%s","nextWriterFence":2,"nextAdmissionSeq":1,"admissionPolicyRef":{"policyRev":1,"policySha256":"%s"}}`,
		testWorkspaceAuthorityID, rootHash, policyHash,
	)))
	writeAuthorityFixture(t, ledger, []byte(fmt.Sprintf(
		`{"schema":2,"authoritySchema":2,"writerFence":1,"ts":"2026-07-18T00:00:00Z","runId":"%s","seq":1,"type":"run_started","actor":"agent:test","data":{}}`+"\n",
		testAuthorityRunID,
	)))

	return runtimeAuthorityFixture{
		root:           root,
		workspace:      workspace,
		configuredPath: configuredPath,
		rootHash:       rootHash,
		policyHash:     policyHash,
		authority:      authority,
		registry:       registryPath,
		bootstrap:      bootstrapPath,
		workspaceDB:    workspacePath,
		policyDir:      policyDir,
		ledger:         ledger,
	}
}

func writeAuthorityFixture(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func setRuntimeAuthorityCurrentPolicy(t *testing.T, fixture runtimeAuthorityFixture, revision int, policy []byte) {
	t.Helper()
	policyHash := sha256Hex(policy)
	writeAuthorityFixture(t, filepath.Join(fixture.policyDir, fmt.Sprintf("%d.json", revision)), policy)
	writeAuthorityFixture(t, fixture.workspaceDB, []byte(fmt.Sprintf(
		`{"recordRev":%d,"authoritySchema":2,"workspaceAuthorityId":"%s","rootIdentityEncoding":"workspace-root-identity-v1","workspaceRootIdentitySha256":"%s","nextWriterFence":2,"nextAdmissionSeq":1,"admissionPolicyRef":{"policyRev":%d,"policySha256":"%s"}}`,
		revision,
		testWorkspaceAuthorityID,
		fixture.rootHash,
		revision,
		policyHash,
	)))
}

func schema2AuthorityEvent(runID string, sequence, writerFence, authoritySchema int) []byte {
	return []byte(fmt.Sprintf(
		`{"schema":2,"authoritySchema":%d,"writerFence":%d,"ts":"2026-07-18T00:00:01Z","runId":"%s","seq":%d,"type":"run_succeeded","actor":"agent:test","data":{"final":true}}`+"\n",
		authoritySchema,
		writerFence,
		runID,
		sequence,
	))
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func snapshotRuntimeAuthorityFixture(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := filepath.ToSlash(root + ":" + rel)
			info, err := entry.Info()
			if err != nil {
				return err
			}
			value := info.Mode().String()
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				value += ":" + target
			case info.Mode().IsRegular():
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				value += ":" + sha256Hex(raw)
			}
			snapshot[key] = value
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(snapshot))
	for _, key := range keys {
		ordered[key] = snapshot[key]
	}
	return ordered
}

func replaceAuthorityFixture(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), old, replacement, 1)
	if updated == string(raw) {
		t.Fatalf("fixture replacement did not match %q", old)
	}
	writeAuthorityFixture(t, path, []byte(updated))
}
