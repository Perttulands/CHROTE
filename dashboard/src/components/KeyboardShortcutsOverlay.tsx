import { useEffect, useRef } from 'react'

interface ShortcutItem {
  keys: string[]
  description: string
}

interface ShortcutCategory {
  title: string
  shortcuts: ShortcutItem[]
}

const shortcutCategories: ShortcutCategory[] = [
  {
    title: 'Navigation',
    shortcuts: [
      { keys: ['1', '2', '3', '4'], description: 'Switch to window 1-4' },
      { keys: ['Tab'], description: 'Toggle Terminal 1 / Terminal 2' },
      { keys: ['Ctrl', 'Down'], description: 'Cycle through windows' },
      { keys: ['/'], description: 'Focus search box' },
    ],
  },
  {
    title: 'Sessions',
    shortcuts: [
      { keys: ['Ctrl', 'N'], description: 'Create new session' },
      { keys: ['Ctrl', 'Right'], description: 'Next session in window' },
      { keys: ['Ctrl', 'Left'], description: 'Previous session in window' },
    ],
  },
  {
    title: 'Windows',
    shortcuts: [
      { keys: ['Ctrl', 'S'], description: 'Toggle sidebar' },
      { keys: ['Ctrl', '1-9'], description: 'Load layout preset 1-9' },
    ],
  },
  {
    title: 'General',
    shortcuts: [
      { keys: ['?'], description: 'Show this help overlay' },
      { keys: ['Escape'], description: 'Close overlay / modal' },
    ],
  },
]

function KeyboardKey({ keyName }: { keyName: string }) {
  // Special key styling
  const isModifier = ['Ctrl', 'Alt', 'Shift', 'Cmd', 'Tab', 'Escape'].includes(keyName)
  const isWide = ['Tab', 'Escape', 'Ctrl', 'Shift'].includes(keyName)

  return (
    <span className={`keyboard-key ${isModifier ? 'modifier' : ''} ${isWide ? 'wide' : ''}`}>
      {keyName}
    </span>
  )
}

function ShortcutRow({ keys, description }: ShortcutItem) {
  return (
    <div className="shortcut-row">
      <div className="shortcut-keys">
        {keys.map((key, index) => (
          <span key={index}>
            {index > 0 && <span className="key-separator">+</span>}
            <KeyboardKey keyName={key} />
          </span>
        ))}
      </div>
      <span className="shortcut-description">{description}</span>
    </div>
  )
}

function ShortcutSection({ title, shortcuts }: ShortcutCategory) {
  return (
    <div className="shortcut-section">
      <h3 className="shortcut-section-title">{title}</h3>
      <div className="shortcut-list">
        {shortcuts.map((shortcut, index) => (
          <ShortcutRow key={index} {...shortcut} />
        ))}
      </div>
    </div>
  )
}

interface KeyboardShortcutsOverlayProps {
  isOpen: boolean
  onClose: () => void
}

function KeyboardShortcutsOverlay({ isOpen, onClose }: KeyboardShortcutsOverlayProps) {
  const modalRef = useRef<HTMLDivElement>(null)

  // Handle escape key and click outside
  useEffect(() => {
    if (!isOpen) return

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, onClose])

  const handleOverlayClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose()
    }
  }

  if (!isOpen) return null

  return (
    <div className="keyboard-overlay" onClick={handleOverlayClick}>
      <div className="keyboard-modal" ref={modalRef}>
        <div className="keyboard-modal-header">
          <h2 className="keyboard-modal-title">Keyboard Shortcuts</h2>
          <button className="keyboard-modal-close" onClick={onClose} aria-label="Close">
            <span aria-hidden="true">×</span>
          </button>
        </div>
        <div className="keyboard-modal-body">
          <div className="shortcut-grid">
            {shortcutCategories.map((category, index) => (
              <ShortcutSection key={index} {...category} />
            ))}
          </div>
        </div>
        <div className="keyboard-modal-footer">
          <span className="keyboard-hint">
            Press <KeyboardKey keyName="?" /> anywhere to show this overlay
          </span>
        </div>
      </div>
    </div>
  )
}

export default KeyboardShortcutsOverlay
