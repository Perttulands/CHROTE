// Draggable recipient chip component

import { useDraggable } from '@dnd-kit/core'
import type { MailRecipient } from './types'

interface RecipientChipProps {
  recipient: MailRecipient
  inCompose?: boolean
  onRemove?: () => void
}

const ROLE_ICONS: Record<string, string> = {
  mayor: '\uD83C\uDFA9',   // Top hat
  deacon: '\uD83D\uDC3A',  // Wolf
  witness: '\uD83E\uDD89', // Owl
  refinery: '\uD83C\uDFED', // Factory
  crew: '\uD83D\uDC77',    // Construction worker
  polecat: '\uD83D\uDE3A', // Cat face
  human: '\uD83D\uDC64',   // Person
}

export default function RecipientChip({ recipient, inCompose, onRemove }: RecipientChipProps) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `recipient-${recipient.id}`,
    data: {
      type: 'recipient',
      recipient,
    },
  })

  const icon = ROLE_ICONS[recipient.role] || '\u2709'

  return (
    <div
      ref={setNodeRef}
      className={`recipient-chip ${isDragging ? 'dragging' : ''} ${recipient.online ? 'online' : 'offline'} ${inCompose ? 'in-compose' : ''}`}
      {...(!inCompose ? { ...listeners, ...attributes } : {})}
    >
      <span className="recipient-icon">{icon}</span>
      <span className="recipient-name">{recipient.name}</span>
      <span className={`recipient-status ${recipient.online ? 'online' : 'offline'}`} />
      {inCompose && onRemove && (
        <button className="recipient-remove" onClick={onRemove} title="Remove">
          x
        </button>
      )}
    </div>
  )
}
