import { useState } from 'react'

interface NukeConfirmModalProps {
  onConfirm: () => void
  onCancel: () => void
  sessionCount: number
}

function NukeConfirmModal({ onConfirm, onCancel, sessionCount }: NukeConfirmModalProps) {
  const [inputValue, setInputValue] = useState('')
  const isValid = inputValue === 'NUKE'

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
            This will permanently destroy <strong>{sessionCount}</strong> tmux session{sessionCount !== 1 ? 's' : ''}.
          </p>
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
