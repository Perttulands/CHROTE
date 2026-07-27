package core

import "os"

// FsyncDir makes a completed rename durable by syncing the directory entry.
// A rename is only crash-safe once the parent directory's metadata reaches
// disk; callers that report success before this can lose the whole file on
// power loss. Mirrors the sequence proven in internal/formations/write.go.
func FsyncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
