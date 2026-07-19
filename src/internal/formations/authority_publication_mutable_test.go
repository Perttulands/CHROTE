package formations

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestAuthorityPublisherMutableRequiresExactGeneration(t *testing.T) {
	t.Run("initial generation starts at one", func(t *testing.T) {
		directory, _ := openAuthorityTestDirectory(t)
		publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := publisher.publishMutable("workspace.private.json", nil, testAuthorityMutableRaw(2, "skipped initial"), testAuthorityMutableRevision); !errors.Is(err, errRuntimeConflict) {
			t.Fatalf("skipped initial revision error = %v, want conflict", err)
		}
		assertAuthorityTestDirectoryEntries(t, directory)
	})

	tests := []struct {
		name         string
		expected     func([]byte) authorityGeneration
		next         []byte
		wantError    error
		prepare      func(*testing.T, *authorityPublisher, string) []byte
		missingStart bool
	}{
		{
			name: "wrong current hash",
			expected: func(first []byte) authorityGeneration {
				return authorityGeneration{recordRev: 1, sha256: strings.Repeat("0", 64)}
			},
			next:      testAuthorityMutableRaw(2, "next"),
			wantError: errRuntimeConflict,
		},
		{
			name: "noncanonical expected hash",
			expected: func(first []byte) authorityGeneration {
				return authorityGeneration{recordRev: 1, sha256: "not-a-sha256"}
			},
			next:      testAuthorityMutableRaw(2, "next"),
			wantError: errRuntimeNoncanonical,
		},
		{
			name: "wrong current revision",
			expected: func(first []byte) authorityGeneration {
				return testAuthorityGeneration(2, first)
			},
			next:      testAuthorityMutableRaw(3, "next"),
			wantError: errRuntimeConflict,
		},
		{
			name:      "same revision",
			expected:  func(first []byte) authorityGeneration { return testAuthorityGeneration(1, first) },
			next:      testAuthorityMutableRaw(1, "same revision"),
			wantError: errRuntimeConflict,
		},
		{
			name:      "skipped revision",
			expected:  func(first []byte) authorityGeneration { return testAuthorityGeneration(1, first) },
			next:      testAuthorityMutableRaw(3, "skipped revision"),
			wantError: errRuntimeConflict,
		},
		{
			name:      "zero revision",
			expected:  func(first []byte) authorityGeneration { return testAuthorityGeneration(1, first) },
			next:      testAuthorityMutableRaw(0, "zero revision"),
			wantError: errRuntimeOutOfRange,
		},
		{
			name:      "malformed next record",
			expected:  func(first []byte) authorityGeneration { return testAuthorityGeneration(1, first) },
			next:      []byte("not a revisioned record"),
			wantError: errRuntimeNoncanonical,
		},
		{
			name:      "oversize next record",
			expected:  func(first []byte) authorityGeneration { return testAuthorityGeneration(1, first) },
			next:      bytes.Repeat([]byte("x"), int(runtimeAuthorityMaxRecordBytes)+1),
			wantError: errRuntimeOutOfRange,
		},
		{
			name:         "missing expected generation",
			expected:     func(first []byte) authorityGeneration { return testAuthorityGeneration(1, first) },
			next:         testAuthorityMutableRaw(2, "next"),
			wantError:    errRuntimeConflict,
			missingStart: true,
		},
		{
			name: "revision exhaustion",
			expected: func(current []byte) authorityGeneration {
				return testAuthorityGeneration(runtimeAuthorityMaxJSONInteger, current)
			},
			next:      testAuthorityMutableRaw(runtimeAuthorityMaxJSONInteger, "maximum"),
			wantError: errRuntimeOutOfRange,
			prepare: func(t *testing.T, _ *authorityPublisher, path string) []byte {
				current := testAuthorityMutableRaw(runtimeAuthorityMaxJSONInteger, "maximum")
				writeAuthorityFixture(t, filepath.Join(path, "workspace.private.json"), current)
				return current
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory, path := openAuthorityTestDirectory(t)
			publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
			if err != nil {
				t.Fatal(err)
			}
			first := testAuthorityMutableRaw(1, "first")
			switch {
			case test.prepare != nil:
				first = test.prepare(t, publisher, path)
			case !test.missingStart:
				if _, err := publisher.publishMutable("workspace.private.json", nil, first, testAuthorityMutableRevision); err != nil {
					t.Fatalf("publish initial generation: %v", err)
				}
			}
			before := snapshotRuntimeAuthorityFixture(t, path)
			expected := test.expected(first)

			if _, err := publisher.publishMutable("workspace.private.json", &expected, test.next, testAuthorityMutableRevision); !errors.Is(err, test.wantError) {
				t.Fatalf("mutable validation error = %v, want %v", err, test.wantError)
			}
			if after := snapshotRuntimeAuthorityFixture(t, path); !reflectDeepEqualAuthoritySnapshot(after, before) {
				t.Fatalf("rejected mutable publication changed state\nbefore: %#v\nafter:  %#v", before, after)
			}
		})
	}
}

func TestAuthorityPublisherMutableReplacesOneGenerationAndRejectsConsumedPredecessor(t *testing.T) {
	directory, path := openAuthorityTestDirectory(t)
	publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstRaw := testAuthorityMutableRaw(1, "first")
	first, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision)
	if err != nil {
		t.Fatalf("publish initial mutable generation: %v", err)
	}
	if first != testAuthorityGeneration(1, firstRaw) {
		t.Fatalf("initial generation = %+v", first)
	}
	firstStat := authorityTestFileStat(t, filepath.Join(path, "workspace.private.json"))
	if retry, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision); err != nil || retry != first {
		t.Fatalf("retry initial generation = %+v, %v; want %+v", retry, err, first)
	}

	secondRaw := testAuthorityMutableRaw(2, "second")
	second, err := publisher.publishMutable("workspace.private.json", &first, secondRaw, testAuthorityMutableRevision)
	if err != nil {
		t.Fatalf("replace mutable generation: %v", err)
	}
	if second != testAuthorityGeneration(2, secondRaw) {
		t.Fatalf("second generation = %+v", second)
	}
	secondStat := authorityTestFileStat(t, filepath.Join(path, "workspace.private.json"))
	if secondStat.Ino == firstStat.Ino {
		t.Fatalf("mutable replacement reused canonical inode %d", firstStat.Ino)
	}
	assertAuthorityTestFile(t, filepath.Join(path, "workspace.private.json"), secondRaw)

	retryStat := authorityTestFileStat(t, filepath.Join(path, "workspace.private.json"))
	if _, err := publisher.publishMutable("workspace.private.json", &first, secondRaw, testAuthorityMutableRevision); !errors.Is(err, errRuntimeConflict) {
		t.Fatalf("consumed predecessor retry error = %v, want conflict", err)
	}
	falsePredecessor := authorityGeneration{recordRev: first.recordRev, sha256: strings.Repeat("0", 64)}
	if _, err := publisher.publishMutable("workspace.private.json", &falsePredecessor, secondRaw, testAuthorityMutableRevision); !errors.Is(err, errRuntimeConflict) {
		t.Fatalf("false predecessor exact-next error = %v, want conflict", err)
	}
	if after := authorityTestFileStat(t, filepath.Join(path, "workspace.private.json")); after.Ino != retryStat.Ino {
		t.Fatalf("consumed predecessor retry replaced inode: before %d, after %d", retryStat.Ino, after.Ino)
	}

	conflictingSecond := testAuthorityMutableRaw(2, "conflicting second")
	if _, err := publisher.publishMutable("workspace.private.json", &first, conflictingSecond, testAuthorityMutableRevision); !errors.Is(err, errRuntimeConflict) {
		t.Fatalf("same revision different bytes error = %v, want conflict", err)
	}
	assertAuthorityTestFile(t, filepath.Join(path, "workspace.private.json"), secondRaw)
	assertAuthorityTestDirectoryEntries(t, directory, "workspace.private.json")
}

func TestAuthorityPublisherMutableRechecksGenerationBeforeRename(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, []byte)
	}{
		{
			name: "replaced current inode",
			mutate: func(t *testing.T, canonical string, current []byte) {
				replacement := canonical + ".replacement"
				writeAuthorityFixture(t, replacement, current)
				if err := os.Rename(replacement, canonical); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "changed current mode",
			mutate: func(t *testing.T, canonical string, _ []byte) {
				if err := os.Chmod(canonical, 0o640); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "linked current inode",
			mutate: func(t *testing.T, canonical string, _ []byte) {
				if err := os.Link(canonical, filepath.Join(t.TempDir(), "escaped-current")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory, path := openAuthorityTestDirectory(t)
			publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
			if err != nil {
				t.Fatal(err)
			}
			canonical := filepath.Join(path, "workspace.private.json")
			firstRaw := testAuthorityMutableRaw(1, "first")
			first, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision)
			if err != nil {
				t.Fatal(err)
			}
			replaced := false
			realReplace := publisher.ops.replace
			publisher.ops.replace = func(directoryFD int, temporary, name string) error {
				replaced = true
				return realReplace(directoryFD, temporary, name)
			}
			publisher.hook = func(step authorityPublicationStep) error {
				if step == authorityPublicationMutableStageSynced {
					test.mutate(t, canonical, firstRaw)
				}
				return nil
			}

			if _, err := publisher.publishMutable("workspace.private.json", &first, testAuthorityMutableRaw(2, "second"), testAuthorityMutableRevision); err == nil {
				t.Fatal("mutable publication ignored changed current generation")
			}
			if replaced {
				t.Fatal("mutable publication renamed after the current generation changed")
			}
			assertAuthorityTestDirectoryEntries(t, directory, "workspace.private.json")
		})
	}
}

func TestAuthorityPublisherRejectsChangedStageBeforeInstall(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "hard link",
			mutate: func(t *testing.T, stage string) {
				if err := os.Link(stage, filepath.Join(t.TempDir(), "escaped-stage")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "changed mode",
			mutate: func(t *testing.T, stage string) {
				if err := os.Chmod(stage, 0o640); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "changed bytes",
			mutate: func(t *testing.T, stage string) {
				if err := os.WriteFile(stage, []byte("attacker-controlled staged bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "rebound path",
			mutate: func(t *testing.T, stage string) {
				if err := os.Rename(stage, stage+".detached"); err != nil {
					t.Fatal(err)
				}
				writeAuthorityFixture(t, stage, []byte("replacement stage bytes"))
			},
		},
	}
	operations := []struct {
		name string
		step authorityPublicationStep
		run  func(*testing.T, *authorityPublisher, string) error
	}{
		{
			name: "immutable",
			step: authorityPublicationStageSynced,
			run: func(t *testing.T, publisher *authorityPublisher, path string) error {
				_, err := publisher.publishImmutable("bootstrap.json", []byte("requested immutable bytes"))
				if _, statErr := os.Lstat(filepath.Join(path, "bootstrap.json")); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("changed stage exposed immutable canonical: %v", statErr)
				}
				return err
			},
		},
		{
			name: "mutable",
			step: authorityPublicationMutableStageSynced,
			run: func(t *testing.T, publisher *authorityPublisher, path string) error {
				firstRaw := testAuthorityMutableRaw(1, "first")
				first, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision)
				if err != nil {
					t.Fatal(err)
				}
				_, err = publisher.publishMutable("workspace.private.json", &first, testAuthorityMutableRaw(2, "second"), testAuthorityMutableRevision)
				assertAuthorityTestFile(t, filepath.Join(path, "workspace.private.json"), firstRaw)
				return err
			},
		},
	}

	for _, operation := range operations {
		for _, mutation := range mutations {
			t.Run(operation.name+"/"+mutation.name, func(t *testing.T) {
				directory, path := openAuthorityTestDirectory(t)
				publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
				if err != nil {
					t.Fatal(err)
				}
				publisher.hook = func(step authorityPublicationStep) error {
					if step == operation.step {
						mutation.mutate(t, findAuthorityStagePath(t, path))
					}
					return nil
				}
				if err := operation.run(t, publisher, path); !errors.Is(err, errRuntimeIntegrityMismatch) {
					t.Fatalf("changed stage error = %v, want integrity mismatch", err)
				}
			})
		}
	}
}

func TestAuthorityPublisherRejectsCanonicalMutationAtPostInstallHook(t *testing.T) {
	operations := []struct {
		name      string
		step      authorityPublicationStep
		canonical string
		run       func(*testing.T, *authorityPublisher) error
	}{
		{
			name:      "immutable",
			step:      authorityPublicationInstalled,
			canonical: "bootstrap.json",
			run: func(_ *testing.T, publisher *authorityPublisher) error {
				_, err := publisher.publishImmutable("bootstrap.json", []byte("requested immutable bytes"))
				return err
			},
		},
		{
			name:      "mutable",
			step:      authorityPublicationMutableReplaced,
			canonical: "workspace.private.json",
			run: func(t *testing.T, publisher *authorityPublisher) error {
				firstRaw := testAuthorityMutableRaw(1, "first")
				first, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision)
				if err != nil {
					t.Fatal(err)
				}
				_, err = publisher.publishMutable("workspace.private.json", &first, testAuthorityMutableRaw(2, "second"), testAuthorityMutableRevision)
				return err
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			directory, path := openAuthorityTestDirectory(t)
			canonical := filepath.Join(path, operation.canonical)
			publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), func(step authorityPublicationStep) error {
				if step == operation.step {
					return os.WriteFile(canonical, []byte("post-install attacker bytes"), authorityPrivateFileMode)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			if err := operation.run(t, publisher); !errors.Is(err, errRuntimeIntegrityMismatch) || !errors.Is(err, errAuthorityDurabilityUncertain) {
				t.Fatalf("post-install canonical mutation error = %v, want integrity-mismatch durability-uncertain error", err)
			}
		})
	}
}

func findAuthorityStagePath(t *testing.T, directory string) string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".authority-stage-") && !strings.HasSuffix(entry.Name(), ".detached") {
			stages = append(stages, filepath.Join(directory, entry.Name()))
		}
	}
	if len(stages) != 1 {
		t.Fatalf("authority stages = %q, want one", stages)
	}
	return stages[0]
}

func TestAuthorityPublisherMutableFsyncOrderAndFailures(t *testing.T) {
	t.Run("ordered generation replacement", func(t *testing.T) {
		directory, _ := openAuthorityTestDirectory(t)
		publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
		if err != nil {
			t.Fatal(err)
		}
		firstRaw := testAuthorityMutableRaw(1, "first")
		first, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision)
		if err != nil {
			t.Fatal(err)
		}
		realOps := publisher.ops
		var order []string
		publisher.ops.syncFile = func(file *os.File) error {
			order = append(order, "file-sync")
			return realOps.syncFile(file)
		}
		publisher.ops.replace = func(directoryFD int, temporary, canonical string) error {
			order = append(order, "replace")
			return realOps.replace(directoryFD, temporary, canonical)
		}
		publisher.ops.syncDirectory = func(file *os.File) error {
			order = append(order, "directory-sync")
			return realOps.syncDirectory(file)
		}

		if _, err := publisher.publishMutable("workspace.private.json", &first, testAuthorityMutableRaw(2, "second"), testAuthorityMutableRevision); err != nil {
			t.Fatal(err)
		}
		if want := []string{"file-sync", "replace", "directory-sync"}; !slices.Equal(order, want) {
			t.Fatalf("mutable publication order = %q, want %q", order, want)
		}
	})

	t.Run("stage sync failure preserves current", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		publisher, first, firstRaw := newInitializedMutablePublisher(t, directory)
		injected := errors.New("injected stage sync failure")
		publisher.ops.syncFile = func(*os.File) error { return injected }

		if _, err := publisher.publishMutable("workspace.private.json", &first, testAuthorityMutableRaw(2, "second"), testAuthorityMutableRevision); !errors.Is(err, injected) {
			t.Fatalf("mutable stage sync failure = %v, want injected error", err)
		}
		assertAuthorityTestFile(t, filepath.Join(path, "workspace.private.json"), firstRaw)
		assertAuthorityTestDirectoryEntries(t, directory, "workspace.private.json")
	})

	t.Run("replace failure preserves current", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		publisher, first, firstRaw := newInitializedMutablePublisher(t, directory)
		injected := errors.New("injected replace failure")
		publisher.ops.replace = func(int, string, string) error { return injected }

		if _, err := publisher.publishMutable("workspace.private.json", &first, testAuthorityMutableRaw(2, "second"), testAuthorityMutableRevision); !errors.Is(err, injected) {
			t.Fatalf("mutable replace failure = %v, want injected error", err)
		}
		assertAuthorityTestFile(t, filepath.Join(path, "workspace.private.json"), firstRaw)
		assertAuthorityTestDirectoryEntries(t, directory, "workspace.private.json")
	})

	t.Run("directory sync failure requires authoritative reread", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		publisher, first, _ := newInitializedMutablePublisher(t, directory)
		realSync := publisher.ops.syncDirectory
		injected := errors.New("injected directory sync failure")
		publisher.ops.syncDirectory = func(*os.File) error { return injected }
		secondRaw := testAuthorityMutableRaw(2, "second")

		if _, err := publisher.publishMutable("workspace.private.json", &first, secondRaw, testAuthorityMutableRevision); !errors.Is(err, injected) || !errors.Is(err, errAuthorityDurabilityUncertain) {
			t.Fatalf("mutable directory sync failure = %v, want injected durability-uncertain error", err)
		}
		assertAuthorityTestFile(t, filepath.Join(path, "workspace.private.json"), secondRaw)
		assertAuthorityTestDirectoryEntries(t, directory, "workspace.private.json")

		publisher.ops.syncDirectory = realSync
		if _, err := publisher.publishMutable("workspace.private.json", &first, secondRaw, testAuthorityMutableRevision); !errors.Is(err, errRuntimeConflict) {
			t.Fatalf("retry with consumed predecessor error = %v, want conflict", err)
		}
		current, exists, err := publisher.readMutable("workspace.private.json", testAuthorityMutableRevision)
		if err != nil || !exists || current.generation != testAuthorityGeneration(2, secondRaw) {
			t.Fatalf("authoritative reread = %+v, %t, %v", current.generation, exists, err)
		}
	})
}

func newInitializedMutablePublisher(t *testing.T, directory *os.File) (*authorityPublisher, authorityGeneration, []byte) {
	t.Helper()
	publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstRaw := testAuthorityMutableRaw(1, "first")
	first, err := publisher.publishMutable("workspace.private.json", nil, firstRaw, testAuthorityMutableRevision)
	if err != nil {
		t.Fatal(err)
	}
	return publisher, first, firstRaw
}

func testAuthorityGeneration(recordRev uint64, raw []byte) authorityGeneration {
	return authorityGeneration{recordRev: recordRev, sha256: runtimeSHA256Hex(raw)}
}

func testAuthorityMutableRaw(recordRev uint64, body string) []byte {
	return []byte(fmt.Sprintf("rev:%d\n%s", recordRev, body))
}

func testAuthorityMutableRevision(raw []byte) (uint64, error) {
	line, _, ok := bytes.Cut(raw, []byte("\n"))
	if !ok || !bytes.HasPrefix(line, []byte("rev:")) {
		return 0, errRuntimeNoncanonical
	}
	rawRevision := strings.TrimPrefix(string(line), "rev:")
	if rawRevision == "" || (len(rawRevision) > 1 && rawRevision[0] == '0') {
		return 0, errRuntimeNoncanonical
	}
	revision, err := strconv.ParseUint(rawRevision, 10, 64)
	if err != nil {
		return 0, errRuntimeOutOfRange
	}
	return revision, nil
}

func reflectDeepEqualAuthoritySnapshot(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
