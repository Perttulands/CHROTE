// MobileView - Mobile-friendly chat interface for Chrote

import { useState, useRef, useEffect, useCallback } from 'react'

interface ChatMessage {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  timestamp: Date
}

export default function MobileView() {
  const [messages, setMessages] = useState<ChatMessage[]>([
    {
      id: '1',
      role: 'system',
      content: 'Welcome to Chrote Mobile. Type a message to begin.',
      timestamp: new Date(),
    },
  ])
  const [input, setInput] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [])

  useEffect(() => {
    scrollToBottom()
  }, [messages, scrollToBottom])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmedInput = input.trim()
    if (!trimmedInput || isLoading) return

    const userMessage: ChatMessage = {
      id: Date.now().toString(),
      role: 'user',
      content: trimmedInput,
      timestamp: new Date(),
    }

    setMessages(prev => [...prev, userMessage])
    setInput('')
    setIsLoading(true)

    // Simulate response (in production, this would call an API)
    setTimeout(() => {
      const assistantMessage: ChatMessage = {
        id: (Date.now() + 1).toString(),
        role: 'assistant',
        content: `Received: "${trimmedInput}"\n\nMobile chat is connected. Use the full Chrote dashboard for terminal access.`,
        timestamp: new Date(),
      }
      setMessages(prev => [...prev, assistantMessage])
      setIsLoading(false)
    }, 500)
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit(e)
    }
  }

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value)
    // Auto-resize textarea
    const textarea = e.target
    textarea.style.height = 'auto'
    textarea.style.height = Math.min(textarea.scrollHeight, 120) + 'px'
  }

  const formatTime = (date: Date) => {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  return (
    <div className="mobile-view">
      <div className="mobile-header">
        <div className="mobile-header-title">
          <span className="mobile-logo">C</span>
          <span>Chrote Mobile</span>
        </div>
        <div className="mobile-header-status">
          <span className="mobile-status-dot"></span>
          <span>Connected</span>
        </div>
      </div>

      <div className="mobile-messages">
        {messages.map(message => (
          <div key={message.id} className={`mobile-message mobile-message-${message.role}`}>
            <div className="mobile-message-content">
              {message.content.split('\n').map((line, i) => (
                <p key={i}>{line || '\u00A0'}</p>
              ))}
            </div>
            <div className="mobile-message-time">{formatTime(message.timestamp)}</div>
          </div>
        ))}
        {isLoading && (
          <div className="mobile-message mobile-message-assistant">
            <div className="mobile-message-content">
              <div className="mobile-typing">
                <span></span>
                <span></span>
                <span></span>
              </div>
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      <form className="mobile-input-area" onSubmit={handleSubmit}>
        <textarea
          ref={inputRef}
          className="mobile-input"
          value={input}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          placeholder="Type a message..."
          rows={1}
          disabled={isLoading}
        />
        <button
          type="submit"
          className="mobile-send-btn"
          disabled={!input.trim() || isLoading}
          title="Send message"
        >
          <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
            <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z" />
          </svg>
        </button>
      </form>
    </div>
  )
}
