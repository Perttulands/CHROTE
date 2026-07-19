package formations

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"syscall"
	"testing"
)

func TestAuthorityPublisherCreatesPrivateImmutableFile(t *testing.T) {
	directory, path := openAuthorityTestDirectory(t)
	publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
	if err != nil {
		t.Fatalf("create authority publisher: %v", err)
	}
	raw := []byte("complete immutable authority bytes\n")

	ref, err := publisher.publishImmutable("bootstrap.json", raw)
	if err != nil {
		t.Fatalf("publish immutable authority file: %v", err)
	}
	if ref.sha256 != runtimeSHA256Hex(raw) || ref.size != int64(len(raw)) {
		t.Fatalf("content ref = %+v, want hash %s and size %d", ref, runtimeSHA256Hex(raw), len(raw))
	}
	assertAuthorityTestFile(t, filepath.Join(path, "bootstrap.json"), raw)
	assertAuthorityTestDirectoryEntries(t, directory, "bootstrap.json")
}

func TestAuthorityPublisherImmutableRetryRequiresExactBytes(t *testing.T) {
	directory, path := openAuthorityTestDirectory(t)
	publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
	if err != nil {
		t.Fatal(err)
	}
	original := []byte("immutable authority bytes\n")
	ref, err := publisher.publishImmutable("policy.json", original)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityTestFileStat(t, filepath.Join(path, "policy.json"))

	retryRef, err := publisher.publishImmutable("policy.json", append([]byte(nil), original...))
	if err != nil {
		t.Fatalf("retry exact immutable publication: %v", err)
	}
	if retryRef != ref {
		t.Fatalf("retry ref = %+v, want %+v", retryRef, ref)
	}
	afterRetry := authorityTestFileStat(t, filepath.Join(path, "policy.json"))
	if afterRetry.Ino != before.Ino {
		t.Fatalf("exact retry replaced canonical inode: before %d, after %d", before.Ino, afterRetry.Ino)
	}

	if _, err := publisher.publishImmutable("policy.json", []byte("conflicting bytes\n")); !errors.Is(err, errRuntimeConflict) {
		t.Fatalf("conflicting immutable publication error = %v, want conflict", err)
	}
	afterConflict := authorityTestFileStat(t, filepath.Join(path, "policy.json"))
	if afterConflict.Ino != before.Ino {
		t.Fatalf("conflicting retry replaced canonical inode: before %d, after %d", before.Ino, afterConflict.Ino)
	}
	assertAuthorityTestFile(t, filepath.Join(path, "policy.json"), original)
	assertAuthorityTestDirectoryEntries(t, directory, "policy.json")
}

func TestAuthorityPublisherRejectsUnsafeCanonicalFile(t *testing.T) {
	tests := []struct {
		name   string
		create func(*testing.T, string, []byte) string
	}{
		{
			name: "hard link",
			create: func(t *testing.T, path string, raw []byte) string {
				writeAuthorityFixture(t, path, raw)
				escaped := filepath.Join(t.TempDir(), "escaped-authority")
				if err := os.Link(path, escaped); err != nil {
					t.Fatal(err)
				}
				return escaped
			},
		},
		{
			name: "wrong mode",
			create: func(t *testing.T, path string, raw []byte) string {
				writeAuthorityFixture(t, path, raw)
				if err := os.Chmod(path, 0o640); err != nil {
					t.Fatal(err)
				}
				return ""
			},
		},
		{
			name: "special mode bits",
			create: func(t *testing.T, path string, raw []byte) string {
				writeAuthorityFixture(t, path, raw)
				if err := os.Chmod(path, os.ModeSetgid|0o600); err != nil {
					t.Fatal(err)
				}
				return ""
			},
		},
		{
			name: "symlink",
			create: func(t *testing.T, path string, raw []byte) string {
				victim := filepath.Join(t.TempDir(), "victim")
				writeAuthorityFixture(t, victim, raw)
				if err := os.Symlink(victim, path); err != nil {
					t.Fatal(err)
				}
				return ""
			},
		},
		{
			name: "directory",
			create: func(t *testing.T, path string, _ []byte) string {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				return ""
			},
		},
		{
			name: "fifo",
			create: func(t *testing.T, path string, _ []byte) string {
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
				return ""
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
			raw := []byte("authority bytes that must not be exposed unsafely\n")
			canonical := filepath.Join(path, "workspace.private.json")
			escaped := test.create(t, canonical, raw)
			before := snapshotRuntimeAuthorityFixture(t, path)
			var linkBefore authorityHardLinkStat
			if escaped != "" {
				linkBefore = authorityHardLinkIdentity(t, canonical, escaped)
			}

			if _, err := publisher.publishImmutable("workspace.private.json", raw); !errors.Is(err, errRuntimeIntegrityMismatch) {
				t.Fatalf("unsafe canonical error = %v, want integrity mismatch", err)
			}
			if after := snapshotRuntimeAuthorityFixture(t, path); !reflect.DeepEqual(after, before) {
				t.Fatalf("unsafe canonical rejection mutated directory\nbefore: %#v\nafter:  %#v", before, after)
			}
			if escaped != "" {
				if linkAfter := authorityHardLinkIdentity(t, canonical, escaped); linkAfter != linkBefore {
					t.Fatalf("unsafe canonical rejection changed link identity: before %+v, after %+v", linkBefore, linkAfter)
				}
			}
		})
	}
}

func TestAuthorityPublisherRejectsNonPrivateDirectoryAndUnsafeName(t *testing.T) {
	t.Run("wrong directory mode", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		if err := os.Chmod(path, 0o750); err != nil {
			t.Fatal(err)
		}
		if _, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil); !errors.Is(err, errRuntimeIntegrityMismatch) {
			t.Fatalf("wrong directory mode error = %v, want integrity mismatch", err)
		}
	})

	t.Run("directory special mode bits", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		if err := os.Chmod(path, os.ModeSetgid|0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil); !errors.Is(err, errRuntimeIntegrityMismatch) {
			t.Fatalf("special directory mode error = %v, want integrity mismatch", err)
		}
	})

	t.Run("wrong expected owner", func(t *testing.T) {
		directory, _ := openAuthorityTestDirectory(t)
		wrongOwner := uint32(os.Geteuid()) + 1
		if _, err := newAuthorityPublisher(directory, wrongOwner, nil); !errors.Is(err, errRuntimeIntegrityMismatch) {
			t.Fatalf("wrong directory owner error = %v, want integrity mismatch", err)
		}
	})

	t.Run("regular file descriptor", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-a-directory")
		writeAuthorityFixture(t, path, []byte("sentinel"))
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		if _, err := newAuthorityPublisher(file, uint32(os.Geteuid()), nil); !errors.Is(err, errRuntimeIntegrityMismatch) {
			t.Fatalf("regular descriptor error = %v, want integrity mismatch", err)
		}
	})

	t.Run("unsafe path components", func(t *testing.T) {
		directory, _ := openAuthorityTestDirectory(t)
		publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"", ".", "..", "../escape", "nested/file"} {
			if _, err := publisher.publishImmutable(name, []byte("sentinel")); !errors.Is(err, errRuntimeNoncanonical) {
				t.Fatalf("unsafe name %q error = %v, want noncanonical", name, err)
			}
		}
		assertAuthorityTestDirectoryEntries(t, directory)
	})
}

func TestAuthorityPublisherImmutableInstallRace(t *testing.T) {
	tests := []struct {
		name      string
		winner    []byte
		wantError error
	}{
		{name: "exact competing winner", winner: []byte("requested immutable bytes\n")},
		{name: "different competing winner", winner: []byte("different winner bytes\n"), wantError: errRuntimeConflict},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory, path := openAuthorityTestDirectory(t)
			publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
			if err != nil {
				t.Fatal(err)
			}
			requested := []byte("requested immutable bytes\n")
			publisher.ops.installNoReplace = func(_ int, _, canonical string) error {
				writeAuthorityFixture(t, filepath.Join(path, canonical), test.winner)
				return syscall.EEXIST
			}

			_, err = publisher.publishImmutable("bootstrap.json", requested)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("install-race error = %v, want %v", err, test.wantError)
			}
			assertAuthorityTestFile(t, filepath.Join(path, "bootstrap.json"), test.winner)
			assertAuthorityTestDirectoryEntries(t, directory, "bootstrap.json")
		})
	}

	t.Run("unsupported no-replace fails closed", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
		if err != nil {
			t.Fatal(err)
		}
		publisher.ops.installNoReplace = func(int, string, string) error { return syscall.ENOSYS }

		if _, err := publisher.publishImmutable("bootstrap.json", []byte("sentinel")); !errors.Is(err, errAuthorityAtomicNoReplaceUnavailable) {
			t.Fatalf("unsupported no-replace error = %v, want capability failure", err)
		}
		if _, err := os.Lstat(filepath.Join(path, "bootstrap.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unsupported no-replace exposed canonical file: %v", err)
		}
		assertAuthorityTestDirectoryEntries(t, directory)
	})
}

func TestAuthorityPublisherImmutableFsyncOrderAndFailures(t *testing.T) {
	t.Run("ordered durable publication", func(t *testing.T) {
		directory, _ := openAuthorityTestDirectory(t)
		publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
		if err != nil {
			t.Fatal(err)
		}
		realOps := publisher.ops
		var order []string
		publisher.ops.syncFile = func(file *os.File) error {
			order = append(order, "file-sync")
			return realOps.syncFile(file)
		}
		publisher.ops.installNoReplace = func(directoryFD int, stage, canonical string) error {
			order = append(order, "install")
			return realOps.installNoReplace(directoryFD, stage, canonical)
		}
		publisher.ops.syncDirectory = func(file *os.File) error {
			order = append(order, "directory-sync")
			return realOps.syncDirectory(file)
		}

		if _, err := publisher.publishImmutable("bootstrap.json", []byte("sentinel")); err != nil {
			t.Fatal(err)
		}
		if want := []string{"file-sync", "install", "directory-sync"}; !slices.Equal(order, want) {
			t.Fatalf("publication order = %q, want %q", order, want)
		}
	})

	t.Run("stage sync failure exposes no canonical", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
		if err != nil {
			t.Fatal(err)
		}
		injected := errors.New("injected stage sync failure")
		publisher.ops.syncFile = func(*os.File) error { return injected }

		if _, err := publisher.publishImmutable("bootstrap.json", []byte("sentinel")); !errors.Is(err, injected) {
			t.Fatalf("stage sync failure = %v, want injected error", err)
		}
		if _, err := os.Lstat(filepath.Join(path, "bootstrap.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stage sync failure exposed canonical file: %v", err)
		}
		assertAuthorityTestDirectoryEntries(t, directory)
	})

	t.Run("directory sync failure leaves complete retryable canonical", func(t *testing.T) {
		directory, path := openAuthorityTestDirectory(t)
		publisher, err := newAuthorityPublisher(directory, uint32(os.Geteuid()), nil)
		if err != nil {
			t.Fatal(err)
		}
		realSync := publisher.ops.syncDirectory
		injected := errors.New("injected directory sync failure")
		publisher.ops.syncDirectory = func(*os.File) error { return injected }
		raw := []byte("complete immutable bytes\n")

		if _, err := publisher.publishImmutable("bootstrap.json", raw); !errors.Is(err, injected) {
			t.Fatalf("directory sync failure = %v, want injected error", err)
		}
		assertAuthorityTestFile(t, filepath.Join(path, "bootstrap.json"), raw)
		assertAuthorityTestDirectoryEntries(t, directory, "bootstrap.json")

		publisher.ops.syncDirectory = realSync
		if _, err := publisher.publishImmutable("bootstrap.json", raw); err != nil {
			t.Fatalf("retry complete canonical after directory sync failure: %v", err)
		}
	})
}

func openAuthorityTestDirectory(t *testing.T) (*os.File, string) {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	return directory, path
}

func assertAuthorityTestFile(t *testing.T, path string, want []byte) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, want) {
		t.Fatalf("authority bytes = %q, want %q", raw, want)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		t.Fatalf("authority file identity = mode %v stat %+v", info.Mode(), info.Sys())
	}
}

func authorityTestFileStat(t *testing.T, path string) *syscall.Stat_t {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("authority stat type = %T, want *syscall.Stat_t", info.Sys())
	}
	copy := *stat
	return &copy
}

func assertAuthorityTestDirectoryEntries(t *testing.T, directory *os.File, want ...string) {
	t.Helper()
	if _, err := directory.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	names, err := directory.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("directory entries = %q, want %q", names, want)
	}
}
