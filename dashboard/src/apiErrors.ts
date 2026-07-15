function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function apiErrorMessage(raw: string, fallback: string): string {
  const text = raw.trim()
  if (!text) return fallback
  try {
    const parsed = JSON.parse(text) as unknown
    if (isRecord(parsed)) {
      const error = parsed.error
      if (isRecord(error) && typeof error.message === 'string' && error.message.trim()) {
        return error.message.trim()
      }
      if (typeof parsed.message === 'string' && parsed.message.trim()) {
        return parsed.message.trim()
      }
    }
  } catch {
    // Non-JSON responses are still useful if they are small enough to show directly.
  }
  return text.length <= 240 ? text : fallback
}
