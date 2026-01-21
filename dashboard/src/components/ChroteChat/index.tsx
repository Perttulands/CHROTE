// ChroteChat - Dual-channel messaging (Mail + Nudge)

import { useState, useRef, useEffect, useCallback } from 'react'
import type { Conversation, ChatMessage } from './types'
import { useConversations, useChatHistory, sendChatMessage } from './hooks'
import { useToast } from '../../context/ToastContext'
import './styles.css'

export default function ChroteChat() {
  const [selectedTarget, setSelectedTarget] = useState<string | null>(null)
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [optimisticMessages, setOptimisticMessages] = useState<ChatMessage[]>([])

  const { conversations, loading: convoLoading, refresh: refreshConvos } = useConversations()
  const { messages, loading: historyLoading, refresh: refreshHistory } = useChatHistory(selectedTarget)
  const { addToast } = useToast()

  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // Auto-scroll to bottom when messages change
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, optimisticMessages])

  // Focus input when conversation selected
  useEffect(() => {
    if (selectedTarget) {
      inputRef.current?.focus()
    }
  }, [selectedTarget])

  // Poll for new messages
  useEffect(() => {
    if (!selectedTarget) return

    const interval = setInterval(() => {
      refreshHistory()
    }, 5000) // Poll every 5 seconds

    return () => clearInterval(interval)
  }, [selectedTarget, refreshHistory])

  const selectedConvo = conversations.find(c => c.target === selectedTarget)

  const handleSelectConversation = (target: string) => {
    setSelectedTarget(target)
    setOptimisticMessages([])
  }

  const handleSend = async () => {
    if (!selectedTarget || !input.trim() || sending) return

    const messageContent = input.trim()
    setInput('')
    setSending(true)

    // Optimistic UI: Show message immediately
    const optimisticMsg: ChatMessage = {
      id: `optimistic-${Date.now()}`,
      role: 'user',
      from: 'you',
      to: selectedTarget,
      content: messageContent,
      timestamp: new Date().toISOString(),
      read: true,
    }
    setOptimisticMessages(prev => [...prev, optimisticMsg])

    // Send via dual-channel
    const result = await sendChatMessage(selectedTarget, messageContent)

    if (result.success) {
      const details = []
      if (result.mailSent) details.push('Mail sent')
      if (result.nudged) details.push('Agent nudged')
      addToast(details.join(', ') || 'Message sent', 'success')

      // Refresh to get server-confirmed state
      setTimeout(() => {
        refreshHistory()
        setOptimisticMessages([])
      }, 1000)
    } else {
      addToast(result.error || 'Send failed', 'error')
      // Remove optimistic message on failure
      setOptimisticMessages(prev => prev.filter(m => m.id !== optimisticMsg.id))
    }

    setSending(false)
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value)
    // Auto-resize
    const textarea = e.target
    textarea.style.height = 'auto'
    textarea.style.height = Math.min(textarea.scrollHeight, 120) + 'px'
  }

  const allMessages = [...messages, ...optimisticMessages]

  return (
    <div className="chrote-chat">
      {/* Conversation List */}
      <div className="chat-sidebar">
        <div className="chat-sidebar-header">
          <h2>ChroteChat</h2>
          <button
            className="chat-refresh-btn"
            onClick={refreshConvos}
            disabled={convoLoading}
            title="Refresh"
          >
            {convoLoading ? '...' : '\u21BB'}
          </button>
        </div>

        <div className="chat-conversations">
          {convoLoading && conversations.length === 0 ? (
            <div className="chat-loading">Loading...</div>
          ) : conversations.length === 0 ? (
            <div className="chat-empty">No agents available</div>
          ) : (
            conversations.map(convo => (
              <ConversationItem
                key={convo.target}
                conversation={convo}
                selected={convo.target === selectedTarget}
                onClick={() => handleSelectConversation(convo.target)}
              />
            ))
          )}
        </div>
      </div>

      {/* Chat Area */}
      <div className="chat-main">
        {!selectedTarget ? (
          <div className="chat-placeholder">
            <div className="chat-placeholder-icon">💬</div>
            <div className="chat-placeholder-text">
              Select a conversation to start chatting
            </div>
            <div className="chat-placeholder-hint">
              Messages are sent via Mail + Nudge for reliable delivery
            </div>
          </div>
        ) : (
          <>
            {/* Chat Header */}
            <div className="chat-header">
              <div className="chat-header-info">
                <RoleIcon role={selectedConvo?.role || 'unknown'} />
                <span className="chat-header-name">
                  {selectedConvo?.displayName || selectedTarget}
                </span>
                <span className={`chat-header-status ${selectedConvo?.online ? 'online' : 'offline'}`}>
                  {selectedConvo?.online ? 'Online' : 'Offline'}
                </span>
              </div>
              <button
                className="chat-refresh-btn"
                onClick={refreshHistory}
                disabled={historyLoading}
                title="Refresh history"
              >
                {historyLoading ? '...' : '\u21BB'}
              </button>
            </div>

            {/* Messages */}
            <div className="chat-messages">
              {historyLoading && allMessages.length === 0 ? (
                <div className="chat-loading">Loading history...</div>
              ) : allMessages.length === 0 ? (
                <div className="chat-empty-history">
                  No messages yet. Send one to start the conversation.
                </div>
              ) : (
                allMessages.map(msg => (
                  <MessageBubble key={msg.id} message={msg} />
                ))
              )}
              <div ref={messagesEndRef} />
            </div>

            {/* Input Area */}
            <div className="chat-input-area">
              <textarea
                ref={inputRef}
                className="chat-input"
                value={input}
                onChange={handleInputChange}
                onKeyDown={handleKeyDown}
                placeholder="Type a message... (Enter to send)"
                rows={1}
                disabled={sending}
              />
              <button
                className="chat-send-btn"
                onClick={handleSend}
                disabled={!input.trim() || sending}
                title="Send (Enter)"
              >
                {sending ? '...' : '\u2192'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

// Conversation list item
interface ConversationItemProps {
  conversation: Conversation
  selected: boolean
  onClick: () => void
}

function ConversationItem({ conversation, selected, onClick }: ConversationItemProps) {
  return (
    <button
      className={`chat-convo-item ${selected ? 'selected' : ''} ${conversation.online ? 'online' : 'offline'}`}
      onClick={onClick}
    >
      <RoleIcon role={conversation.role} />
      <div className="chat-convo-info">
        <div className="chat-convo-name">{conversation.displayName}</div>
        <div className="chat-convo-status">
          {conversation.online ? 'Online' : 'Offline'}
        </div>
      </div>
      {conversation.unreadCount > 0 && (
        <span className="chat-convo-badge">{conversation.unreadCount}</span>
      )}
    </button>
  )
}

// Message bubble
interface MessageBubbleProps {
  message: ChatMessage
}

function MessageBubble({ message }: MessageBubbleProps) {
  const isUser = message.role === 'user'
  const isOptimistic = message.id.startsWith('optimistic-')

  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    } catch {
      return ''
    }
  }

  return (
    <div className={`chat-message ${isUser ? 'user' : 'agent'} ${isOptimistic ? 'optimistic' : ''}`}>
      <div className="chat-message-content">
        {message.content.split('\n').map((line, i) => (
          <p key={i}>{line || '\u00A0'}</p>
        ))}
      </div>
      <div className="chat-message-meta">
        <span className="chat-message-time">{formatTime(message.timestamp)}</span>
        {isOptimistic && <span className="chat-message-sending">Sending...</span>}
      </div>
    </div>
  )
}

// Role icon component
function RoleIcon({ role }: { role: string }) {
  const icons: Record<string, string> = {
    mayor: '\uD83C\uDFA9',      // 🎩
    deacon: '\uD83D\uDC3A',     // 🐺
    witness: '\uD83E\uDD89',    // 🦉
    refinery: '\uD83C\uDFED',   // 🏭
    polecat: '\uD83D\uDE3A',    // 😺
    crew: '\uD83D\uDC77',       // 👷
    human: '\uD83D\uDC64',      // 👤
  }

  return <span className="chat-role-icon">{icons[role] || '\uD83D\uDCAC'}</span>
}
