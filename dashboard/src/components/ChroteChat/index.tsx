// ChroteChat - Dual-channel messaging (Mail + Nudge)

import { useState, useRef, useEffect, useMemo } from 'react'
import type { Conversation, ChatMessage } from './types'
import { useConversations, useChatHistory, useChannels, sendChatMessage, sendNudge, initSession, restartSession, getSessionStatus, createChannel, inviteToChannel, deleteChannel, getChannelSubscribers } from './hooks'
import { useToast } from '../../context/ToastContext'
import RoleBadge from '../RoleBadge'
import './styles.css'

const STORAGE_KEY = 'chrote-chat-selected'
const COLLAPSED_GROUPS_KEY = 'chrote-chat-collapsed-groups'

// Group priority - matches session.go logic + channels
function getChatGroupPriority(group: string): number {
  if (group === 'channels') return -1
  if (group === 'hq') return 0
  if (group === 'main') return 1
  if (group.startsWith('gt-')) return 3
  return 4 // other
}

// Get display name for a group - matches types.ts getGroupDisplayName
function getChatGroupDisplayName(group: string): string {
  if (group === 'channels') return 'Channels'
  if (group === 'hq') return 'HQ'
  if (group === 'main') return 'Main'
  if (group === 'other') return 'Other'
  if (group.startsWith('gt-')) {
    // gt-gastown → Gastown
    const rigName = group.slice(3)
    return rigName.charAt(0).toUpperCase() + rigName.slice(1)
  }
  return group
}

// Categorize a conversation into a group based on displayName (session name)
// This mirrors CategorizeSession from session.go
function categorizeConversation(convo: Conversation): string {
  if (convo.role === 'channel') return 'channels'
  const name = convo.displayName.toLowerCase()

  // HQ sessions: hq-mayor, hq-deacon, etc.
  if (name.startsWith('hq-')) {
    return 'hq'
  }

  // Main sessions
  if (name === 'main' || name === 'shell') {
    return 'main'
  }

  // Gastown rig sessions: gt-{rigname}-{role}-{name}
  // e.g., gt-greenplace-crew-max → gt-greenplace
  if (name.startsWith('gt-')) {
    const parts = name.split('-')
    if (parts.length >= 2) {
      return parts[0] + '-' + parts[1] // "gt-rigname"
    }
    return 'gt-unknown'
  }

  return 'other'
}

// Group conversations by their category (same as session grouping)
function groupConversations(conversations: Conversation[]): Record<string, Conversation[]> {
  const groups: Record<string, Conversation[]> = {}

  for (const convo of conversations) {
    const group = categorizeConversation(convo)
    if (!groups[group]) {
      groups[group] = []
    }
    groups[group].push(convo)
  }

  return groups
}

export default function ChroteChat() {
  // Restore selected target from localStorage
  const [selectedTarget, setSelectedTarget] = useState<string | null>(() => {
    try {
      return localStorage.getItem(STORAGE_KEY)
    } catch {
      return null
    }
  })
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [pendingMessage, setPendingMessage] = useState<string | null>(null) // Message shown while waiting for server
  const [nudging, setNudging] = useState(false)
  // Auto-collapse sidebar on mobile/narrow screens
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => window.innerWidth <= 768)
  const [sessionStatus, setSessionStatus] = useState<{ exists: boolean; initializing: boolean }>({ exists: false, initializing: false })
  const [showChannelMgr, setShowChannelMgr] = useState(false)
  const [channelName, setChannelName] = useState('')
  const [inviteChannelName, setInviteChannelName] = useState('')
  const [selectedInvitees, setSelectedInvitees] = useState<string[]>([])
  const [channelSubscribers, setChannelSubscribers] = useState<string[]>([])
  const [loadingSubscribers, setLoadingSubscribers] = useState(false)

  // Collapsed groups state - "other" collapsed by default
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => {
    try {
      const saved = localStorage.getItem(COLLAPSED_GROUPS_KEY)
      if (saved) {
        return new Set(JSON.parse(saved))
      }
    } catch { /* ignore */ }
    return new Set(['other']) // Default: collapse "other"
  })

  const { conversations, loading: convoLoading, refresh: refreshConvos } = useConversations()
  const workspace = conversations.find(c => c.workspace)?.workspace || null
  const { channels, refresh: refreshChannels } = useChannels(workspace)
  
  // Create unified options list including channels
  const allTargets: Conversation[] = useMemo(() => {
    const channelConvos: Conversation[] = channels.map(c => ({
      target: `channel:${c.name}`,
      displayName: `#${c.name}`,
      role: 'channel',
      online: true,
      unreadCount: 0,
      workspace: workspace || undefined
    }))
    return [...conversations, ...channelConvos]
  }, [conversations, channels, workspace])

  // Group and sort conversations
  const sortedGroups = useMemo(() => {
    const grouped = groupConversations(allTargets)
    return Object.entries(grouped)
      .sort(([a], [b]) => getChatGroupPriority(a) - getChatGroupPriority(b))
  }, [allTargets])

  const selectedConvo = allTargets.find(c => c.target === selectedTarget)
  
  const isChannel = selectedTarget?.startsWith('channel:') ?? false
  const realTarget = isChannel ? selectedTarget!.replace('channel:', '') : selectedTarget
  
  const { messages, loading: historyLoading, refresh: refreshHistory } = useChatHistory(
    realTarget, 
    selectedConvo?.workspace ?? null,
    isChannel
  )
  const { addToast } = useToast()

  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const chatContainerRef = useRef<HTMLDivElement>(null)
  const shouldAutoScroll = useRef(true)
  const prevMessageCount = useRef(0)

  // Handle sidebar responsive behavior
  useEffect(() => {
    const handleResize = () => {
      // Auto-collapse when window becomes narrow
      if (window.innerWidth <= 768) {
        setSidebarCollapsed(true)
      }
    }

    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  // Ensure we scroll to bottom when viewport resizes (e.g. mobile keyboard)
  useEffect(() => {
    const scrollToBottom = () => {
      if (shouldAutoScroll.current && messagesEndRef.current) {
        messagesEndRef.current.scrollIntoView({ behavior: 'smooth' })
      }
    }

    if (window.visualViewport) {
      window.visualViewport.addEventListener('resize', scrollToBottom)
    }
    window.addEventListener('resize', scrollToBottom)

    return () => {
      if (window.visualViewport) {
        window.visualViewport.removeEventListener('resize', scrollToBottom)
      }
      window.removeEventListener('resize', scrollToBottom)
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

  // Fetch channel subscribers when a channel is selected
  useEffect(() => {
    if (!isChannel || !realTarget || !workspace) {
      setChannelSubscribers([])
      return
    }

    const fetchSubscribers = async () => {
      setLoadingSubscribers(true)
      const result = await getChannelSubscribers(workspace, realTarget)
      setChannelSubscribers(result.subscribers)
      setLoadingSubscribers(false)
    }

    fetchSubscribers()
  }, [isChannel, realTarget, workspace])

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
    // Auto-collapse sidebar on mobile when selecting a conversation
    if (window.innerWidth <= 768) {
      setSidebarCollapsed(true)
    }
    // Persist selection
    try {
      localStorage.setItem(STORAGE_KEY, target)
    } catch { /* ignore */ }
  }

  const handleSend = async () => {
    if (!selectedConvo || !selectedConvo.workspace || !input.trim() || sending) return

    const messageContent = input.trim()
    setInput('')
    setSending(true)

    // Send via dual-channel - workspace comes from the conversation
    // Channels already have "channel:" prefix in selectedConvo.target and use it directly
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

  // Channel Management Handlers
  const handleCreateChannel = async () => {
    const ws = selectedConvo?.workspace || conversations.find(c => c.workspace)?.workspace
    if (!channelName || !ws) {
        addToast('Select an agent first to define workspace', 'error')
        return
    }
    const res = await createChannel(ws, channelName)
    if (res.success) {
      addToast(`Channel '${channelName}' created`, 'success')
      setChannelName('')
      setInviteChannelName(channelName) // Auto-fill next step
      refreshChannels()
    } else {
      addToast(res.message || 'Failed', 'error')
    }
  }

  const handleInvite = async () => {
    const ws = selectedConvo?.workspace || conversations.find(c => c.workspace)?.workspace
    if (!inviteChannelName || selectedInvitees.length === 0 || !ws) {
        addToast('Missing info or workspace', 'error')
        return
    }
    
    addToast('Sending invites...', 'info')
    const res = await inviteToChannel(ws, inviteChannelName, selectedInvitees)
    if (res.success) {
      addToast(`Invited ${res.sent} agents`, 'success')
      setSelectedInvitees([])
      setShowChannelMgr(false)
    } else {
      addToast(res.message || 'Failed', 'error')
    }
  }

  const toggleInvitee = (target: string) => {
    if (selectedInvitees.includes(target)) {
        setSelectedInvitees(selectedInvitees.filter(t => t !== target))
    } else {
        setSelectedInvitees([...selectedInvitees, target])
    }
  }

  const handleDeleteChannel = async () => {
    if (!isChannel || !realTarget || !workspace) return

    if (!confirm(`Delete channel "#${realTarget}"? This cannot be undone.`)) return

    const res = await deleteChannel(workspace, realTarget)
    if (res.success) {
      addToast(`Channel "${realTarget}" deleted`, 'success')
      setSelectedTarget(null)
      refreshChannels()
    } else {
      addToast(res.message || 'Failed to delete channel', 'error')
    }
  }

  const toggleGroupCollapse = (role: string) => {
    setCollapsedGroups(prev => {
      const newSet = new Set(prev)
      if (newSet.has(role)) {
        newSet.delete(role)
      } else {
        newSet.add(role)
      }
      // Persist to localStorage
      try {
        localStorage.setItem(COLLAPSED_GROUPS_KEY, JSON.stringify([...newSet]))
      } catch { /* ignore */ }
      return newSet
    })
  }

  const allMessages = messages

  return (
    <div ref={chatContainerRef} className="chrote-chat">
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
                onClick={() => { refreshConvos(); refreshChannels(); }}
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
                sortedGroups.map(([role, groupConvos]) => (
                  <ChatGroup
                    key={role}
                    role={role}
                    conversations={groupConvos}
                    collapsed={collapsedGroups.has(role)}
                    onToggle={() => toggleGroupCollapse(role)}
                    selectedTarget={selectedTarget}
                    onSelect={handleSelectConversation}
                  />
                ))
              )}
            </div>
            <div className="chat-sidebar-footer">
              <button
                className="restart-session-btn"
                onClick={() => setShowChannelMgr(true)}
                title="Create or invite to channels"
              >
                📢 Manage Channels
              </button>
              {sessionStatus.exists ? (
                <div className="session-status">Session active</div>
              ) : (
                <button
                  className="restart-session-btn"
                  onClick={handleRestartSession}
                  disabled={sessionStatus.initializing}
                  title="Start the chrote-chat tmux session"
                >
                  {sessionStatus.initializing ? 'Starting...' : 'Start Chat Session'}
                </button>
              )}
            </div>
          </>
        )}
      </div>

      {/* Chat Area */}
      <div className="chat-main">
        {/* Header */}
        <div className="chat-header">
          <div className="chat-header-info">
            <span className="chat-header-name">
              {selectedConvo?.displayName || selectedTarget || 'Chat'}
            </span>
            {selectedTarget && !isChannel && (
              <span className={`chat-header-status ${selectedConvo?.online ? 'online' : 'offline'}`}>
                {selectedConvo?.online ? 'Online' : 'Offline'}
              </span>
            )}
            {isChannel && (
              <span className="chat-header-subscribers">
                {loadingSubscribers ? '...' :
                  channelSubscribers.length === 0 ? 'No subscribers' :
                  `${channelSubscribers.length} subscriber${channelSubscribers.length !== 1 ? 's' : ''}: ${channelSubscribers.join(', ')}`
                }
              </span>
            )}
          </div>
          <div className="chat-header-actions">
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
            {isChannel && (
              <button
                className="delete-channel-btn"
                onClick={handleDeleteChannel}
                title="Delete channel"
              >
                🗑
              </button>
            )}
          </div>
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

      {showChannelMgr && (
        <div className="channel-modal-overlay" onClick={() => setShowChannelMgr(false)}>
          <div className="channel-modal" onClick={e => e.stopPropagation()}>
            <div className="channel-modal-header">
              <h3>📢 Channels</h3>
              <button className="close-btn" onClick={() => setShowChannelMgr(false)}>✕</button>
            </div>

            <div className="channel-modal-section">
              <h4>Create Channel</h4>
              <div className="channel-row">
                <input
                  value={channelName}
                  onChange={e => setChannelName(e.target.value)}
                  placeholder="Channel Name"
                />
                <button onClick={handleCreateChannel}>Create</button>
              </div>
            </div>

            <div className="channel-modal-section">
              <h4>Invite Members</h4>
              <p className="hint">Selected agents receive a DM with subscription instructions.</p>
              <input
                value={inviteChannelName}
                onChange={e => setInviteChannelName(e.target.value)}
                placeholder="Channel Name to Invite To"
                className="full-width"
              />
              <div className="agent-select-list">
                {conversations.map(c => (
                  <label key={c.target} className="agent-select-item">
                    <input
                      type="checkbox"
                      checked={selectedInvitees.includes(c.target)}
                      onChange={() => toggleInvitee(c.target)}
                    />
                    <span>{c.displayName}</span>
                  </label>
                ))}
              </div>
              <button
                className="invite-btn full-width"
                disabled={!inviteChannelName || selectedInvitees.length === 0}
                onClick={handleInvite}
              >
                Invite {selectedInvitees.length} Agents
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// Conversation list item
interface ConversationItemProps {
  conversation: Conversation
  selected: boolean
  onClick: () => void
}

// Chat group component (similar to SessionGroup)
interface ChatGroupProps {
  role: string
  conversations: Conversation[]
  collapsed: boolean
  onToggle: () => void
  selectedTarget: string | null
  onSelect: (target: string) => void
}

function ChatGroup({ role, conversations, collapsed, onToggle, selectedTarget, onSelect }: ChatGroupProps) {
  const displayName = getChatGroupDisplayName(role)

  return (
    <div className="chat-group">
      <div className="chat-group-header" onClick={onToggle}>
        <span className="expand-icon">{collapsed ? '▶' : '▼'}</span>
        <span className="group-name">{displayName}</span>
        <span className="chat-group-count">{conversations.length}</span>
      </div>

      {!collapsed && (
        <div className="chat-group-items">
          {conversations.map(convo => (
            <ConversationItem
              key={convo.target}
              conversation={convo}
              selected={convo.target === selectedTarget}
              onClick={() => onSelect(convo.target)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function ConversationItem({ conversation, selected, onClick }: ConversationItemProps) {
  return (
    <button
      className={`chat-convo-item ${selected ? 'selected' : ''} ${conversation.online ? 'online' : 'offline'}`}
      onClick={onClick}
    >
      <RoleBadge sessionName={conversation.target} />
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

