//go:build darwin

package formations

import "golang.org/x/sys/unix"

func publishDirectoryNoReplace(source, destination string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_EXCL)
}
