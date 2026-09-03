package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetAllowedRootsNormalizesEnvRoots(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("CHROTE_ROOTS", " , workspace/. , "+root+" , ")

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Clean(absRoot)}
	if got := GetAllowedRoots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAllowedRoots() = %#v, want %#v", got, want)
	}
}

func TestGetAllowedRootsRootDominatesOtherRoots(t *testing.T) {
	t.Setenv("CHROTE_ROOTS", "/, /srv, /home/operator")
	want := []string{string(os.PathSeparator)}
	if got := GetAllowedRoots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAllowedRoots() = %#v, want %#v", got, want)
	}
}

func TestIsPathUnderRootRequiresAbsolutePath(t *testing.T) {
	if IsPathUnderRoot("relative/path", string(os.PathSeparator)) {
		t.Fatal("relative path reported under filesystem root; want false")
	}
}
