// ChroteChat types

export interface ChatMessage {
  id: string
  role: 'user' | 'agent'
  from: string
  to: string
  content: string
  timestamp: string
  read: boolean
}

export interface Conversation {
  target: string
  displayName: string
  role: string
  online: boolean
  unreadCount: number
  lastMessage?: ChatMessage
  workspace?: string // Gastown workspace for this agent
}

export interface SendResponse {
  success: boolean
  messageId?: string
  mailSent: boolean
  nudged: boolean
  error?: string
}

export interface GastownWorkspace {
  name: string
  path: string
}

export interface Channel {
  name: string
  subscribers: string[]
  status: string
  retention_count: number
  retention_hours: number
  created_by: string
  created_at: string
}
