//go:build !linux && !darwin

package formations

import "fmt"

func publishDirectoryNoReplace(_, _ string) error {
	return fmt.Errorf("%w: atomic no-replace workflow pack publication is unsupported on this platform", ErrConflict)
}
