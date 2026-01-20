// Mail data fetching hooks

import { useState, useEffect, useCallback } from 'react'
import type { MailMessage, MailRecipient, TownStatus, ApiResponse } from './types'

const API_BASE = '/api/mail'

async function fetchApi<T>(endpoint: string, options?: RequestInit): Promise<ApiResponse<T>> {
  const url = new URL(endpoint, window.location.origin)
  const response = await fetch(url.toString(), options)
  return response.json()
}

// Hook to fetch inbox messages
export function useInbox() {
  const [messages, setMessages] = useState<MailMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await fetchApi<{ messages: MailMessage[] }>(`${API_BASE}/inbox`)
      if (result.success && result.data) {
        setMessages(result.data.messages)
      } else {
        setError(result.error?.message || 'Failed to fetch inbox')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { messages, loading, error, refresh }
}

// Hook to fetch available recipients
export function useRecipients() {
  const [recipients, setRecipients] = useState<MailRecipient[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await fetchApi<{ recipients: MailRecipient[] }>(`${API_BASE}/recipients`)
      if (result.success && result.data) {
        setRecipients(result.data.recipients)
      } else {
        setError(result.error?.message || 'Failed to fetch recipients')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { recipients, loading, error, refresh }
}

// Hook to fetch town status
export function useTownStatus() {
  const [status, setStatus] = useState<TownStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await fetchApi<TownStatus>(`${API_BASE}/status`)
      if (result.success && result.data) {
        setStatus(result.data)
      } else {
        setError(result.error?.message || 'Failed to fetch status')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Network error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { status, loading, error, refresh }
}

// Send mail function
export async function sendMail(to: string, subject: string, body: string): Promise<{ success: boolean; error?: string }> {
  try {
    const result = await fetchApi<{ messageId: string }>(`${API_BASE}/send`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ to, subject, body }),
    })

    if (result.success) {
      return { success: true }
    } else {
      return { success: false, error: result.error?.message || 'Failed to send message' }
    }
  } catch (e) {
    return { success: false, error: e instanceof Error ? e.message : 'Network error' }
  }
}

// Mark message as read
export async function markAsRead(messageId: string): Promise<{ success: boolean; error?: string }> {
  try {
    const result = await fetchApi<{}>(`${API_BASE}/read/${messageId}`, {
      method: 'POST',
    })

    if (result.success) {
      return { success: true }
    } else {
      return { success: false, error: result.error?.message || 'Failed to mark as read' }
    }
  } catch (e) {
    return { success: false, error: e instanceof Error ? e.message : 'Network error' }
  }
}
