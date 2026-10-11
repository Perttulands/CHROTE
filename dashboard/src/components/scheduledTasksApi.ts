function apiErrorMessage(response: unknown, fallback: string): string {
  if (response && typeof response === 'object' && 'error' in response) {
    const error = (response as { error?: { message?: string } }).error
    if (error?.message) return error.message
  }
  return fallback
}

export async function scheduledRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-Chrote-Intent': 'scheduled-task',
      ...(init?.headers || {}),
    },
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload?.success === false) {
    throw new Error(apiErrorMessage(payload, `Request failed: ${response.status}`))
  }
  return (payload?.data ?? payload) as T
}

export const SCHEDULED_TASKS_CHANGED = 'chrote:scheduled-tasks-changed'
