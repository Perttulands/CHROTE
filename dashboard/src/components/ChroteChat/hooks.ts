// ChroteChat API hooks

import { useState, useEffect, useCallback } from 'react'
import type { Conversation, ChatMessage, SendResponse } from './types'

const API_BASE = '/api/chat'

// Fetch conversations (available chat targets)
export function useConversations() {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      setLoading(true)
      const res = await fetch(`${API_BASE}/conversations`)
      const data = await res.json()

      if (data.success) {
        setConversations(data.data?.conversations || [])
        setError(null)
      } else {
        setError(data.error?.message || 'Failed to load conversations')
      }
    } catch (e) {
      setError('Network error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { conversations, loading, error, refresh }
}

// Fetch chat history for a specific target
export function useChatHistory(target: string | null) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!target) {
      setMessages([])
      return
    }

    try {
      setLoading(true)
      const encodedTarget = encodeURIComponent(target)
      const res = await fetch(`${API_BASE}/${encodedTarget}/history`)
      const data = await res.json()

      if (data.success) {
        setMessages(data.data?.messages || [])
        setError(null)
      } else {
        setError(data.error?.message || 'Failed to load history')
      }
    } catch (e) {
      setError('Network error')
    } finally {
      setLoading(false)
    }
  }, [target])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { messages, loading, error, refresh }
}

// Send a chat message (dual-channel: mail + nudge)
export async function sendChatMessage(
  target: string,
  message: string
): Promise<SendResponse> {
  try {
    const encodedTarget = encodeURIComponent(target)
    const res = await fetch(`${API_BASE}/${encodedTarget}/send`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message }),
    })
    const data = await res.json()

    if (data.success) {
      return {
        success: true,
        messageId: data.data?.messageId,
        mailSent: data.data?.mailSent ?? false,
        nudged: data.data?.nudged ?? false,
      }
    } else {
      return {
        success: false,
        mailSent: false,
        nudged: false,
        error: data.error?.message || 'Send failed',
      }
    }
  } catch (e) {
    return {
      success: false,
      mailSent: false,
      nudged: false,
      error: 'Network error',
    }
  }
}
