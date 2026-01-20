// Individual mail item component

import type { MailMessage } from './types'

interface MailItemProps {
  message: MailMessage
  selected: boolean
  onSelect: (id: string) => void
}

export default function MailItem({ message, selected, onSelect }: MailItemProps) {
  const formatDate = (timestamp: string) => {
    const date = new Date(timestamp)
    const now = new Date()
    const isToday = date.toDateString() === now.toDateString()

    if (isToday) {
      return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    }
    return date.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }

  return (
    <div
      className={`mail-item ${selected ? 'selected' : ''} ${!message.read ? 'unread' : ''}`}
      onClick={() => onSelect(message.id)}
    >
      <div className="mail-item-header">
        <span className="mail-from">{message.from}</span>
        <span className="mail-date">{formatDate(message.timestamp)}</span>
      </div>
      <div className="mail-subject">{message.subject}</div>
      <div className="mail-preview">
        {message.body.slice(0, 80)}{message.body.length > 80 ? '...' : ''}
      </div>
      {!message.read && <span className="mail-unread-dot" />}
    </div>
  )
}
