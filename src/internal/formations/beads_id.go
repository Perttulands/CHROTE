package formations

import (
	"fmt"
	"regexp"
)

var safeBeadsIssueIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*-[a-z0-9]+(\.[0-9]+)*$`)

func isSafeBeadsIssueID(value string) bool {
	return safeBeadsIssueIDPattern.MatchString(value)
}

// validateOptionalBeadID checks the format of a bead id only when one is
// provided. A bead is optional context for a mission or brief, so an empty
// value is allowed; a non-empty value must match the Beads issue id format.
// The returned error wraps ErrInvalidBeadID with the actual value and the
// expected format so callers can report exactly what is wrong.
func validateOptionalBeadID(value string) error {
	if value == "" {
		return nil
	}
	if !isSafeBeadsIssueID(value) {
		return fmt.Errorf("%w: beadId %q is not a valid Beads issue id; expected lowercase like \"chrt-abcd\" matching %s", ErrInvalidBeadID, value, safeBeadsIssueIDPattern.String())
	}
	return nil
}
