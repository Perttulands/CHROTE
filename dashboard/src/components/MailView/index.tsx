// Main MailView component with sub-tab navigation

import { useState, useCallback } from 'react'
import { DndContext, DragEndEvent, DragStartEvent, DragOverlay, useSensor, useSensors, PointerSensor, useDroppable } from '@dnd-kit/core'
import type { MailSubTab, MailRecipient } from './types'
import { useInbox, useRecipients, sendMail } from './hooks'
import InboxView from './InboxView'
import RecipientChip from './RecipientChip'
import { useToast } from '../../context/ToastContext'

const SUB_TABS: { id: MailSubTab; label: string }[] = [
  { id: 'inbox', label: 'Inbox' },
  { id: 'compose', label: 'Compose' },
]

export default function MailView() {
  const [activeSubTab, setActiveSubTab] = useState<MailSubTab>('inbox')
  const [draggedRecipient, setDraggedRecipient] = useState<MailRecipient | null>(null)
  const [selectedRecipients, setSelectedRecipients] = useState<MailRecipient[]>([])

  const { messages, loading: inboxLoading, error: inboxError, refresh: refreshInbox } = useInbox()
  const { recipients, loading: recipientsLoading } = useRecipients()
  const { addToast } = useToast()

  const unreadCount = messages.filter(m => !m.read).length

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 4 },
    })
  )

  const handleDragStart = (event: DragStartEvent) => {
    const data = event.active.data.current as { type?: string; recipient?: MailRecipient } | undefined
    if (data?.type === 'recipient' && data.recipient) {
      setDraggedRecipient(data.recipient)
    }
  }

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    setDraggedRecipient(null)

    if (!over) return

    const activeData = active.data.current
    const overData = over.data.current

    if (activeData?.type === 'recipient' && overData?.type === 'recipient-zone') {
      const recipient = activeData.recipient as MailRecipient
      setSelectedRecipients(prev => {
        if (prev.find(r => r.id === recipient.id)) return prev
        return [...prev, recipient]
      })
    }
  }

  const handleRemoveRecipient = useCallback((id: string) => {
    setSelectedRecipients(prev => prev.filter(r => r.id !== id))
  }, [])

  const handleSent = useCallback(() => {
    addToast('Message sent successfully', 'success')
    setActiveSubTab('inbox')
    setSelectedRecipients([])
    refreshInbox()
  }, [addToast, refreshInbox])

  const handleRefresh = useCallback(() => {
    refreshInbox()
  }, [refreshInbox])

  return (
    <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="mail-view">
        <div className="mail-header">
          <div className="mail-tabs">
            {SUB_TABS.map(tab => (
              <button
                key={tab.id}
                className={`mail-tab ${activeSubTab === tab.id ? 'active' : ''}`}
                onClick={() => setActiveSubTab(tab.id)}
              >
                {tab.label}
                {tab.id === 'inbox' && unreadCount > 0 && (
                  <span className="mail-badge">{unreadCount}</span>
                )}
              </button>
            ))}
          </div>
          {activeSubTab === 'inbox' && (
            <button
              className="mail-refresh-btn"
              onClick={handleRefresh}
              disabled={inboxLoading}
              title="Refresh inbox"
            >
              {inboxLoading ? 'Loading...' : 'Refresh'}
            </button>
          )}
        </div>

        <div className="mail-content">
          {activeSubTab === 'inbox' && (
            <InboxView
              messages={messages}
              loading={inboxLoading}
              error={inboxError}
              onRefresh={refreshInbox}
            />
          )}
          {activeSubTab === 'compose' && (
            <ComposeView
              recipients={recipients}
              loading={recipientsLoading}
              selectedRecipients={selectedRecipients}
              onRemoveRecipient={handleRemoveRecipient}
              onSent={handleSent}
            />
          )}
        </div>
      </div>

      <DragOverlay>
        {draggedRecipient && <RecipientChip recipient={draggedRecipient} />}
      </DragOverlay>
    </DndContext>
  )
}

// Compose view component
interface ComposeViewProps {
  recipients: MailRecipient[]
  loading: boolean
  selectedRecipients: MailRecipient[]
  onRemoveRecipient: (id: string) => void
  onSent: () => void
}

function ComposeView({ recipients, loading, selectedRecipients, onRemoveRecipient, onSent }: ComposeViewProps) {
  const [subject, setSubject] = useState('')
  const [body, setBody] = useState('')
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const { addToast } = useToast()

  const { setNodeRef, isOver } = useDroppable({
    id: 'recipient-drop-zone',
    data: { type: 'recipient-zone' },
  })

  const handleSend = async () => {
    if (selectedRecipients.length === 0) {
      setError('Please select at least one recipient')
      return
    }
    if (!subject.trim()) {
      setError('Please enter a subject')
      return
    }
    if (!body.trim()) {
      setError('Please enter a message')
      return
    }

    setSending(true)
    setError(null)

    try {
      const results = await Promise.all(
        selectedRecipients.map(r => sendMail(r.path, subject, body))
      )

      const failed = results.filter(r => !r.success)
      if (failed.length > 0) {
        setError(`Failed to send: ${failed.map(f => f.error).join(', ')}`)
        addToast('Some messages failed to send', 'error')
      } else {
        setSubject('')
        setBody('')
        onSent()
      }
    } catch {
      setError('Network error')
      addToast('Network error', 'error')
    }

    setSending(false)
  }

  const handleClear = () => {
    setSubject('')
    setBody('')
    setError(null)
  }

  const availableRecipients = recipients.filter(
    r => !selectedRecipients.find(sr => sr.id === r.id)
  )

  const groupedRecipients = availableRecipients.reduce((acc, r) => {
    const group = r.role
    if (!acc[group]) acc[group] = []
    acc[group].push(r)
    return acc
  }, {} as Record<string, MailRecipient[]>)

  const roleOrder = ['mayor', 'deacon', 'witness', 'refinery', 'crew', 'polecat', 'human']

  return (
    <div className="mail-compose">
      <div className="compose-recipients-panel">
        <h3>Recipients</h3>
        <p className="compose-hint">Drag recipients to the To field</p>
        <div className="recipients-list">
          {loading ? (
            <div className="recipients-loading">Loading...</div>
          ) : (
            roleOrder.map(role => {
              const group = groupedRecipients[role]
              if (!group || group.length === 0) return null
              return (
                <div key={role} className="recipient-group">
                  <div className="recipient-group-label">{role}</div>
                  <div className="recipient-group-items">
                    {group.map(r => (
                      <RecipientChip key={r.id} recipient={r} />
                    ))}
                  </div>
                </div>
              )
            })
          )}
        </div>
      </div>

      <div className="compose-form">
        <div
          ref={setNodeRef}
          className={`compose-to ${isOver ? 'drop-active' : ''} ${selectedRecipients.length === 0 ? 'empty' : ''}`}
        >
          <span className="compose-label">To:</span>
          <div className="compose-to-chips">
            {selectedRecipients.length === 0 ? (
              <span className="compose-to-placeholder">Drop recipients here</span>
            ) : (
              selectedRecipients.map(r => (
                <RecipientChip
                  key={r.id}
                  recipient={r}
                  inCompose
                  onRemove={() => onRemoveRecipient(r.id)}
                />
              ))
            )}
          </div>
        </div>

        <div className="compose-subject">
          <span className="compose-label">Subject:</span>
          <input
            type="text"
            value={subject}
            onChange={e => setSubject(e.target.value)}
            placeholder="Enter subject..."
            disabled={sending}
          />
        </div>

        <div className="compose-body">
          <textarea
            value={body}
            onChange={e => setBody(e.target.value)}
            placeholder="Write your message..."
            disabled={sending}
          />
        </div>

        {error && <div className="compose-error">{error}</div>}

        <div className="compose-actions">
          <button
            className="compose-btn send"
            onClick={handleSend}
            disabled={sending || selectedRecipients.length === 0}
          >
            {sending ? 'Sending...' : 'Send'}
          </button>
          <button
            className="compose-btn clear"
            onClick={handleClear}
            disabled={sending}
          >
            Clear
          </button>
        </div>
      </div>
    </div>
  )
}
