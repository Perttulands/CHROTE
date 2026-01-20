// Inbox view component

import { useState } from 'react'
import type { MailMessage } from './types'
import { markAsRead } from './hooks'
import MailItem from './MailItem'

interface InboxViewProps {
  messages: MailMessage[]
  loading: boolean
  error: string | null
  onRefresh: () => void
}

export default function InboxView({ messages, loading, error, onRefresh }: InboxViewProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const selectedMessage = messages.find(m => m.id === selectedId)

  const handleSelect = async (id: string) => {
    setSelectedId(id)
    const message = messages.find(m => m.id === id)
    if (message && !message.read) {
      await markAsRead(id)
      onRefresh()
    }
  }

  const handleBack = () => {
    setSelectedId(null)
  }

  if (loading && messages.length === 0) {
    return (
      <div className="mail-loading">
        <div className="loading-spinner" />
        Loading inbox...
      </div>
    )
  }

  if (error) {
    return (
      <div className="mail-error">
        <div className="error-icon">!</div>
        <p>{error}</p>
        <button onClick={onRefresh}>Retry</button>
      </div>
    )
  }

  // Message detail view
  if (selectedMessage) {
    return (
      <div className="mail-detail">
        <div className="mail-detail-header">
          <button className="mail-back-btn" onClick={handleBack}>
            Back
          </button>
        </div>
        <div className="mail-detail-content">
          <div className="mail-detail-meta">
            <div className="mail-detail-subject">{selectedMessage.subject}</div>
            <div className="mail-detail-from">
              <span className="label">From:</span> {selectedMessage.from}
            </div>
            <div className="mail-detail-to">
              <span className="label">To:</span> {selectedMessage.to}
            </div>
            <div className="mail-detail-date">
              {new Date(selectedMessage.timestamp).toLocaleString()}
            </div>
          </div>
          <div className="mail-detail-body">
            {selectedMessage.body}
          </div>
        </div>
      </div>
    )
  }

  // Inbox list view
  if (messages.length === 0) {
    return (
      <div className="mail-empty">
        <div className="empty-icon">&#x1F4EC;</div>
        <h2>No Messages</h2>
        <p>Your inbox is empty.</p>
      </div>
    )
  }

  return (
    <div className="mail-list">
      {messages.map(message => (
        <MailItem
          key={message.id}
          message={message}
          selected={selectedId === message.id}
          onSelect={handleSelect}
        />
      ))}
    </div>
  )
}
