import { useState } from 'react'

// Sessions that are protected from nuke (must match backend)
const PROTECTED_SESSIONS = ['chrote-chat']

interface NukeConfirmModalProps {
  onConfirm: () => void
  onCancel: () => void
  sessionCount: number
  sessionNames?: string[]
  protectedSessionNames?: string[]
}

function NukeConfirmModal({ onConfirm, onCancel, sessionCount, sessionNames = [], protectedSessionNames = [] }: NukeConfirmModalProps) {
  const [inputValue, setInputValue] = useState('')
  const isValid = inputValue === 'NUKE'

  // Calculate how many sessions will actually be killed (excluding protected)
  const protectedSet = new Set([...PROTECTED_SESSIONS, ...protectedSessionNames])
  const protectedNames = sessionNames.filter(name => protectedSet.has(name))
  const protectedCount = protectedNames.length
  const killableCount = sessionCount - protectedCount

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (isValid) {
      onConfirm()
    }
  }

  return (
    <div className="nuke-modal-overlay" onClick={onCancel}>
      <div className="nuke-modal" onClick={e => e.stopPropagation()}>
        <div className="nuke-modal-header">
          <span className="nuke-icon">☢</span>
          <span className="nuke-title">DESTROY ALL SESSIONS</span>
        </div>

        <div className="nuke-modal-body">
          <p className="nuke-warning">
            This will permanently destroy <strong>{killableCount}</strong> tmux session{killableCount !== 1 ? 's' : ''}.
          </p>
          {protectedCount > 0 && (
            <p className="nuke-protected">
              <strong>{protectedCount}</strong> protected session{protectedCount !== 1 ? 's' : ''} will be preserved: {protectedNames.join(', ')}
            </p>
          )}
          <p className="nuke-warning">
            All running processes will be terminated. This cannot be undone.
          </p>

          <form onSubmit={handleSubmit}>
            <label className="nuke-label">
              Type <strong>NUKE</strong> to confirm:
            </label>
            <input
              type="text"
              className="nuke-input"
              value={inputValue}
              onChange={e => setInputValue(e.target.value.toUpperCase())}
              placeholder="Type NUKE"
              autoFocus
            />

            <div className="nuke-buttons">
              <button type="button" className="nuke-btn nuke-btn-cancel" onClick={onCancel}>
                Cancel
              </button>
              <button
                type="submit"
                className="nuke-btn nuke-btn-confirm"
                disabled={!isValid}
              >
                Destroy All
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}

export default NukeConfirmModal
