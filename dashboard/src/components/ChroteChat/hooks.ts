// ChroteChat API hooks

import { useState, useEffect, useCallback } from 'react'
import type { Conversation, ChatMessage, SendResponse } from './types'

const API_BASE = '/api/chat'

// Fetch conversations (available chat targets) with workspace info from backend
export function useConversations() {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      setLoading(true)
      const res = await fetch(`${API_BASE}/conversations`)
      const data = await res.json()

      if (data.data?.conversations !== undefined) {
        // Backend already includes workspace info and sorts
        setConversations(data.data.conversations || [])
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

// Fetch available channels
export function useChannels(workspace: string | null) {
  const [channels, setChannels] = useState<any[]>([]) // Using any for now to avoid extensive type updates, but effectively Channel[]
  const [loading, setLoading] = useState(false)

  const refresh = useCallback(async () => {
    if (!workspace) {
      setChannels([])
      return
    }
    try {
      setLoading(true)
      const res = await fetch(`${API_BASE}/channel/list?workspace=${encodeURIComponent(workspace)}`)
      const data = await res.json()
      if (data.data?.channels) {
        setChannels(data.data.channels)
      }
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [workspace])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { channels, loading, refresh }
}

// Fetch chat history for a specific target
export function useChatHistory(target: string | null, workspace: string | null, isChannel = false) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async (silent = false) => {
    if (!target || !workspace) {
      setMessages([])
      return
    }

    try {
      if (!silent) setLoading(true)
      const encodedTarget = encodeURIComponent(target)
      const encodedWorkspace = encodeURIComponent(workspace)
      
      let url = `${API_BASE}/history?target=${encodedTarget}&workspace=${encodedWorkspace}`
      if (isChannel) {
        url = `${API_BASE}/channel/messages?channel=${encodedTarget}&workspace=${encodedWorkspace}`
      }

      const res = await fetch(url)
      const data = await res.json()

      if (data.data?.messages !== undefined) {
        setMessages(data.data.messages || [])
        setError(null)
      } else {
        setError(data.error?.message || 'Failed to load history')
      }
    } catch (e) {
      setError('Network error')
    } finally {
      if (!silent) setLoading(false)
    }
  }, [target, workspace, isChannel])


  useEffect(() => {
    refresh()
  }, [refresh])

  return { messages, loading, error, refresh }
}

// Send a chat message (dual-channel: mail + nudge)
export async function sendChatMessage(
  workspace: string,
  target: string,
  message: string
): Promise<SendResponse> {
  const payload = { workspace, target, message }

  console.group('[ChroteChat] Sending message')
  console.log('Payload:', JSON.stringify(payload, null, 2))
  console.log('Endpoint:', `${API_BASE}/send`)

  try {
    const res = await fetch(`${API_BASE}/send`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })

    console.log('Response status:', res.status, res.statusText)

    const data = await res.json()
    console.log('Response body:', JSON.stringify(data, null, 2))
    console.groupEnd()

    if (data.data?.success) {
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
    console.error('Network error:', e)
    console.groupEnd()
    return {
      success: false,
      mailSent: false,
      nudged: false,
      error: 'Network error',
    }
  }
}

// Nudge-only response
export interface NudgeResponse {
  success: boolean
  nudged: boolean
  error?: string
}

// Send a nudge only (no mail)
export async function sendNudge(
  workspace: string,
  target: string,
  message?: string
): Promise<NudgeResponse> {
  const payload = { workspace, target, message }

  console.group('[ChroteChat] Sending nudge')
  console.log('Payload:', JSON.stringify(payload, null, 2))

  try {
    const res = await fetch(`${API_BASE}/nudge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })

    const data = await res.json()
    console.log('Response:', JSON.stringify(data, null, 2))
    console.groupEnd()

    if (data.data?.success) {
      return {
        success: true,
        nudged: data.data?.nudged ?? false,
      }
    } else {
      return {
        success: false,
        nudged: false,
        error: data.error?.message || 'Nudge failed',
      }
    }
  } catch (e) {
    console.error('Network error:', e)
    console.groupEnd()
    return {
      success: false,
      nudged: false,
      error: 'Network error',
    }
  }
}

// Session management types
export interface SessionStatus {
  exists: boolean
  workspace?: string
}

export interface SessionInitResult {
  created: boolean
  workspace?: string
  message?: string
}

// Get chrote-chat session status
export async function getSessionStatus(): Promise<SessionStatus> {
  try {
    const res = await fetch(`${API_BASE}/session/status`)
    const data = await res.json()
    return {
      exists: data.data?.exists ?? false,
      workspace: data.data?.workspace,
    }
  } catch {
    return { exists: false }
  }
}

// Initialize chrote-chat session in workspace
export async function initSession(workspace: string): Promise<SessionInitResult> {
  try {
    const res = await fetch(`${API_BASE}/session/init`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace }),
    })
    const data = await res.json()
    if (data.data) {
      return {
        created: data.data.created ?? false,
        workspace: data.data.workspace,
        message: data.data.message,
      }
    }
    return { created: false, message: data.error?.message || 'Failed to init session' }
  } catch {
    return { created: false, message: 'Network error' }
  }
}

// Restart chrote-chat session
export async function restartSession(workspace: string): Promise<SessionInitResult> {
  try {
    const res = await fetch(`${API_BASE}/session/restart`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace }),
    })
    const data = await res.json()
    if (data.data) {
      return {
        created: data.data.restarted ?? false,
        workspace: data.data.workspace,
      }
    }
    return { created: false, message: data.error?.message || 'Failed to restart session' }
  } catch {
    return { created: false, message: 'Network error' }
  }
}

// Create a broadcast channel
export async function createChannel(workspace: string, name: string): Promise<{ success: boolean; message?: string }> {
  try {
    const res = await fetch(`${API_BASE}/channel/create`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace, name }),
    })
    const data = await res.json()
    if (data.data?.success) {
      return { success: true }
    }
    return { success: false, message: data.error?.message || 'Failed to create channel' }
  } catch {
    return { success: false, message: 'Network error' }
  }
}

// Invite members to a channel (sends instructions via DM)
export async function inviteToChannel(workspace: string, channel: string, targets: string[]): Promise<{ success: boolean; sent?: number; total?: number; message?: string }> {
  try {
    const res = await fetch(`${API_BASE}/channel/invite`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace, channel, targets }),
    })
    const data = await res.json()
    if (data.data?.success) {
      return { success: true, sent: data.data.sent, total: data.data.total }
    }
    return { success: false, message: data.error?.message || 'Failed to invite members' }
  } catch {
    return { success: false, message: 'Network error' }
  }
}

// Delete a channel
export async function deleteChannel(workspace: string, name: string): Promise<{ success: boolean; message?: string }> {
  try {
    const res = await fetch(`${API_BASE}/channel/delete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace, name }),
    })
    const data = await res.json()
    if (data.data?.success) {
      return { success: true }
    }
    return { success: false, message: data.error?.message || 'Failed to delete channel' }
  } catch {
    return { success: false, message: 'Network error' }
  }
}

// Get channel subscribers
export async function getChannelSubscribers(workspace: string, channel: string): Promise<{ subscribers: string[]; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/channel/subscribers?workspace=${encodeURIComponent(workspace)}&channel=${encodeURIComponent(channel)}`)
    const data = await res.json()
    if (data.data?.subscribers !== undefined) {
      return { subscribers: data.data.subscribers || [] }
    }
    return { subscribers: [], error: data.error?.message || 'Failed to get subscribers' }
  } catch {
    return { subscribers: [], error: 'Network error' }
  }
}
