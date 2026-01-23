// ChroteChat - Dual-channel messaging (Mail + Nudge)

import { useState, useRef, useEffect } from 'react'
import type { Conversation, ChatMessage } from './types'
import { useConversations, useChatHistory, sendChatMessage, sendNudge, initSession, restartSession, getSessionStatus } from './hooks'
import { useToast } from '../../context/ToastContext'
import './styles.css'

const STORAGE_KEY = 'chrote-chat-selected'

export default function ChroteChat() {
  // Restore selected target from localStorage
  const [selectedTarget, setSelectedTarget] = useState<string | null>(() => {
    try {
      return localStorage.getItem(STORAGE_KEY)
    } catch {
      return null
    }
  })
  const [mobileView, setMobileView] = useState<'list' | 'chat'>('list')
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [pendingMessage, setPendingMessage] = useState<string | null>(null) // Message shown while waiting for server
  const [nudging, setNudging] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [sessionStatus, setSessionStatus] = useState<{ exists: boolean; initializing: boolean }>({ exists: false, initializing: false })

  const { conversations, loading: convoLoading, refresh: refreshConvos } = useConversations()
  const selectedConvo = conversations.find(c => c.target === selectedTarget)
  const { messages, loading: historyLoading, refresh: refreshHistory } = useChatHistory(selectedTarget, selectedConvo?.workspace ?? null)
  const { addToast } = useToast()

  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const chatContainerRef = useRef<HTMLDivElement>(null)
  const shouldAutoScroll = useRef(true)
  const prevMessageCount = useRef(0)

  // Handle mobile keyboard - adjust height when virtual keyboard appears
  // Only applies on mobile devices (touch-enabled with narrow viewport)
  useEffect(() => {
    const isMobile = () => window.innerWidth <= 768 && 'ontouchstart' in window

    const handleResize = () => {
      if (!chatContainerRef.current || !isMobile()) return
      // Use visualViewport if available (handles keyboard properly)
      if (window.visualViewport) {
        const vh = window.visualViewport.height
        chatContainerRef.current.style.height = `${vh}px`
        // Scroll messages to bottom when keyboard opens
        setTimeout(() => {
          messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
        }, 100)
      }
    }

    // Reset height when switching to desktop
    const handleWindowResize = () => {
      if (!chatContainerRef.current) return
      if (!isMobile()) {
        chatContainerRef.current.style.height = ''
      }
    }

    // Listen to visualViewport resize (fires when keyboard shows/hides)
    if (window.visualViewport) {
      window.visualViewport.addEventListener('resize', handleResize)
      window.visualViewport.addEventListener('scroll', handleResize)
    }
    window.addEventListener('resize', handleWindowResize)

    return () => {
      if (window.visualViewport) {
        window.visualViewport.removeEventListener('resize', handleResize)
        window.visualViewport.removeEventListener('scroll', handleResize)
      }
      window.removeEventListener('resize', handleWindowResize)
    }
  }, [])

  // Track if user is near bottom of chat
  const handleScroll = () => {
    const container = messagesContainerRef.current
    if (!container) return
    const { scrollTop, scrollHeight, clientHeight } = container
    // Consider "near bottom" if within 100px of bottom
    shouldAutoScroll.current = scrollHeight - scrollTop - clientHeight < 100
  }

  // Auto-scroll only when appropriate (user at bottom or just sent a message)
  useEffect(() => {
    const currentCount = messages.length
    const hasNewMessages = currentCount > prevMessageCount.current
    prevMessageCount.current = currentCount

    // Only scroll if user was already at bottom
    if (hasNewMessages && shouldAutoScroll.current) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    }

    // Clear pending message if it appears in server history
    if (pendingMessage && messages.length > 0) {
      const lastUserMsg = [...messages].reverse().find(m => m.role === 'user')
      if (lastUserMsg && lastUserMsg.content === pendingMessage) {
        setPendingMessage(null)
      }
    }
  }, [messages, pendingMessage])

  // Auto-scroll when sending (to show the animation)
  useEffect(() => {
    if (sending) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [sending])

  // Focus input when conversation selected (Desktop only)
  useEffect(() => {
    if (selectedTarget) {
      // Don't auto-focus on mobile as it pops the keyboard and obscures messages
      const isMobile = window.matchMedia('(max-width: 768px)').matches
      if (!isMobile) {
        inputRef.current?.focus()
      }
    }
  }, [selectedTarget])

  // Poll for new messages (silent to avoid UI flicker)
  useEffect(() => {
    if (!selectedTarget) return

    const interval = setInterval(() => {
      refreshHistory(true) // silent refresh
    }, 5000) // Poll every 5 seconds

    return () => clearInterval(interval)
  }, [selectedTarget, refreshHistory])

  // Check session status on mount and when conversations load
  useEffect(() => {
    getSessionStatus().then(status => {
      setSessionStatus({ exists: status.exists, initializing: false })
    })
  }, [])

  // Initialize session when a conversation with workspace is selected
  useEffect(() => {
    const initializeSession = async () => {
      if (!selectedConvo?.workspace || sessionStatus.exists || sessionStatus.initializing) return

      setSessionStatus(prev => ({ ...prev, initializing: true }))
      const result = await initSession(selectedConvo.workspace)
      setSessionStatus({ exists: result.created || sessionStatus.exists, initializing: false })

      if (result.created) {
        addToast('Chat session initialized', 'success')
      }
    }

    initializeSession()
  }, [selectedConvo?.workspace, sessionStatus.exists, sessionStatus.initializing, addToast])

  // Handle restart session
  const handleRestartSession = async () => {
    // Find a workspace from any conversation
    const workspace = selectedConvo?.workspace || conversations.find(c => c.workspace)?.workspace
    if (!workspace) {
      addToast('No workspace available to restart session', 'error')
      return
    }

    setSessionStatus({ exists: false, initializing: true })
    const result = await restartSession(workspace)
    setSessionStatus({ exists: result.created, initializing: false })

    if (result.created) {
      addToast('Chat session restarted', 'success')
    } else {
      addToast(result.message || 'Failed to restart session', 'error')
    }
  }

  const handleSelectConversation = (target: string) => {
    setSelectedTarget(target)
    setMobileView('chat')
    // Persist selection
    try {
      localStorage.setItem(STORAGE_KEY, target)
    } catch { /* ignore */ }
  }

  const handleBackToList = () => {
    setMobileView('list')
    // Optional: Keep selection or clear it? Clearing it might stop polling history.
    // Let's keep selection but stop polling? No, polling is fine.
  }

  const handleSend = async () => {
    if (!selectedConvo || !selectedConvo.workspace || !input.trim() || sending) return

    const messageContent = input.trim()
    setInput('')
    setSending(true)

    // Send via dual-channel - workspace comes from the conversation
    const result = await sendChatMessage(selectedConvo.workspace, selectedConvo.target, messageContent)

    if (result.success) {
      const details = []
      if (result.mailSent) details.push('Mail sent')
      if (result.nudged) details.push('Agent nudged')
      addToast(details.join(', ') || 'Message sent', 'success')

      // Show message on top of spinner while waiting for server confirmation
      setPendingMessage(messageContent)
      setSending(false)

      // Refresh to get server-confirmed state, then clear pending after 10s
      setTimeout(() => {
        refreshHistory()
      }, 1000)

      setTimeout(() => {
        setPendingMessage(null)
      }, 10000)
    } else {
      addToast(result.error || 'Send failed', 'error')
      setSending(false)
    }
  }

  const handleNudge = async () => {
    if (!selectedConvo || !selectedConvo.workspace || nudging) return

    setNudging(true)

    const result = await sendNudge(selectedConvo.workspace, selectedConvo.target)

    if (result.success) {
      addToast('Nudged!', 'success')
    } else {
      addToast(result.error || 'Nudge failed', 'error')
    }

    setNudging(false)
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

  const allMessages = messages

  return (
    <div ref={chatContainerRef} className={`chrote-chat mobile-view-${mobileView}`}>
      {/* Conversation List */}
      <div className={`chat-sidebar ${sidebarCollapsed ? 'collapsed' : ''}`}>
        <div className="chat-sidebar-header">
          <button
            className="toggle-btn"
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            title={sidebarCollapsed ? 'Expand' : 'Collapse'}
          >
            {sidebarCollapsed ? '»' : '«'}
          </button>
          {!sidebarCollapsed && (
            <>
              <span className="panel-title">Chat</span>
              <button
                className="refresh-btn"
                onClick={() => refreshConvos()}
                disabled={convoLoading}
                title="Refresh"
              >
                ↻
              </button>
            </>
          )}
        </div>

        {!sidebarCollapsed && (
          <>
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
            <div className="chat-sidebar-footer">
              <button
                className="restart-session-btn"
                onClick={handleRestartSession}
                disabled={sessionStatus.initializing}
                title="Restart the chrote-chat tmux session"
              >
                {sessionStatus.initializing ? 'Restarting...' : 'Restart Chat Session'}
              </button>
              <div className="session-status">
                {sessionStatus.exists ? 'Session active' : 'No session'}
              </div>
            </div>
          </>
        )}
      </div>

      {/* Chat Area */}
      <div className="chat-main">
        {/* Header - always visible on mobile for navigation */}
        <div className="chat-header">
          <button
            className="chat-back-btn mobile-only"
            onClick={handleBackToList}
            title="Back to agents"
          >
            ←
          </button>
          <div className="chat-header-info">
            <span className="chat-header-name">
              {selectedConvo?.displayName || selectedTarget || 'Chat'}
            </span>
            {selectedTarget && (
              <span className={`chat-header-status ${selectedConvo?.online ? 'online' : 'offline'}`}>
                {selectedConvo?.online ? 'Online' : 'Offline'}
              </span>
            )}
          </div>
          {selectedTarget && (
            <button
              className="refresh-btn"
              onClick={() => refreshHistory()}
              disabled={historyLoading}
              title="Refresh history"
            >
              ↻
            </button>
          )}
        </div>

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
            {/* Messages */}
            <div className="chat-messages" ref={messagesContainerRef} onScroll={handleScroll}>
              {historyLoading && allMessages.length === 0 ? (
                <div className="chat-loading">Loading history...</div>
              ) : allMessages.length === 0 && !sending && !pendingMessage ? (
                <div className="chat-empty-history">
                  No messages yet. Send one to start the conversation.
                </div>
              ) : (
                allMessages.map(msg => (
                  <MessageBubble key={msg.id} message={msg} />
                ))
              )}
              {/* Pending message shown on top of spinner after send completes */}
              {pendingMessage && <PendingMessageBubble content={pendingMessage} />}
              {/* Spinner while actively sending */}
              {sending && <SendingSpinner />}
              <div ref={messagesEndRef} />
            </div>

            {/* Input Area */}
            <div className="chat-input-area">
              {!selectedConvo?.workspace && (
                <div className="chat-input-warning">
                  Cannot send: No Gastown workspace detected for this agent
                </div>
              )}
              <div className="chat-input-row">
                <textarea
                  ref={inputRef}
                  className="chat-input"
                  value={input}
                  onChange={handleInputChange}
                  onKeyDown={handleKeyDown}
                  placeholder={selectedConvo?.workspace ? "Type a message... (Enter to send)" : "Messaging unavailable"}
                  rows={1}
                  disabled={sending || !selectedConvo?.workspace}
                />
                <button
                  className="chat-nudge-btn"
                  onClick={handleNudge}
                  disabled={nudging || !selectedConvo?.workspace}
                  title="Send a quick nudge"
                >
                  {nudging ? '...' : 'Nudge!'}
                </button>
                <button
                  className="chat-send-btn"
                  onClick={handleSend}
                  disabled={!input.trim() || sending || !selectedConvo?.workspace}
                  title={selectedConvo?.workspace ? "Send (Enter)" : "No workspace detected"}
                >
                  {sending ? '...' : '\u2192'}
                </button>
              </div>
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

// Spinner shown while actively sending
function SendingSpinner() {
  return (
    <div className="chat-sending-spinner">
      <div className="spinner"></div>
    </div>
  )
}

// Pending message bubble (shown after send, before server confirms)
function PendingMessageBubble({ content }: { content: string }) {
  return (
    <div className="chat-message user pending">
      <div className="chat-message-content">
        {content.split('\n').map((line, i) => (
          <p key={i}>{line || '\u00A0'}</p>
        ))}
      </div>
      <div className="chat-message-meta">
        <span className="chat-message-pending">Delivering...</span>
      </div>
    </div>
  )
}

// Message bubble
interface MessageBubbleProps {
  message: ChatMessage
}

function MessageBubble({ message }: MessageBubbleProps) {
  const isUser = message.role === 'user'

  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    } catch {
      return ''
    }
  }

  return (
    <div className={`chat-message ${isUser ? 'user' : 'agent'}`}>
      <div className="chat-message-content">
        {message.content.split('\n').map((line, i) => (
          <p key={i}>{line || '\u00A0'}</p>
        ))}
      </div>
      <div className="chat-message-meta">
        <span className="chat-message-time">{formatTime(message.timestamp)}</span>
      </div>
    </div>
  )
}

