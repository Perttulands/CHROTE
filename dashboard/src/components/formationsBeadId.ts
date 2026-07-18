const safeBeadsIssueIDPattern = /^[a-z][a-z0-9]*-[a-z0-9]+(\.[0-9]+)*$/

export function isSafeBeadsIssueID(value: string): boolean {
  return safeBeadsIssueIDPattern.test(value)
}

export function isOptionalSafeBeadsIssueID(value: string): boolean {
  const trimmed = value.trim()
  return trimmed === '' || isSafeBeadsIssueID(trimmed)
}
