import { useState } from 'react'
import TerminalPane from './TerminalPane'

interface PaneConfig {
  id: string
  sessionName: string
}

function TerminalGrid() {
  const [panes, setPanes] = useState<PaneConfig[]>([
    { id: '1', sessionName: 'Terminal 1' },
    { id: '2', sessionName: 'Terminal 2' },
    { id: '3', sessionName: 'Terminal 3' },
    { id: '4', sessionName: 'Terminal 4' },
    { id: '5', sessionName: 'Terminal 5' },
  ])

  const addPane = () => {
    if (panes.length >= 6) return
    const newId = String(Date.now())
    setPanes([...panes, { id: newId, sessionName: `Terminal ${panes.length + 1}` }])
  }

  const removePane = (id: string) => {
    setPanes(panes.filter((p) => p.id !== id))
  }

  // Always show 6 slots (2x3 grid)
  const slots = [...panes]
  while (slots.length < 6) {
    slots.push({ id: `empty-${slots.length}`, sessionName: '' })
  }

  return (
    <div className="terminal-grid">
      {slots.map((slot) =>
        slot.sessionName ? (
          <TerminalPane
            key={slot.id}
            sessionName={slot.sessionName}
            onClose={panes.length > 1 ? () => removePane(slot.id) : undefined}
          />
        ) : (
          <div
            key={slot.id}
            className="terminal-pane empty"
            onClick={addPane}
            title="Add terminal"
          >
            <span className="add-pane-icon">+</span>
          </div>
        )
      )}
    </div>
  )
}

export default TerminalGrid
