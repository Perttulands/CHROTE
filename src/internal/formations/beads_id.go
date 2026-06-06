package formations

import "regexp"

var safeBeadsIssueIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*-[a-z0-9]+(\.[0-9]+)*$`)

func isSafeBeadsIssueID(value string) bool {
	return safeBeadsIssueIDPattern.MatchString(value)
}
