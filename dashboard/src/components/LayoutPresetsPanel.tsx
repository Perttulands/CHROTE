import { useState } from 'react'
import { useSession } from '../context/SessionContext'
import type { LayoutPreset } from '../types'

interface LayoutPresetsPanelProps {
  isOpen: boolean
  onClose: () => void
}

function LayoutPresetsPanel({ isOpen, onClose }: LayoutPresetsPanelProps) {
  const {
    layoutPresets,
    saveCurrentLayout,
    loadPreset,
    deletePreset,
    renamePreset,
  } = useSession()

  const [newPresetName, setNewPresetName] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingName, setEditingName] = useState('')

  const handleSave = () => {
    if (newPresetName.trim()) {
      const success = saveCurrentLayout(newPresetName.trim())
      if (success) {
        setNewPresetName('')
      }
    }
  }

  const handleLoad = (presetId: string) => {
    loadPreset(presetId)
    onClose()
  }

  const handleDelete = (presetId: string) => {
    deletePreset(presetId)
  }

  const startEditing = (preset: LayoutPreset) => {
    setEditingId(preset.id)
    setEditingName(preset.name)
  }

  const handleRename = () => {
    if (editingId && editingName.trim()) {
      renamePreset(editingId, editingName.trim())
      setEditingId(null)
      setEditingName('')
    }
  }

  const cancelEditing = () => {
    setEditingId(null)
    setEditingName('')
  }

  // Get preset summary (windows and sessions count)
  const getPresetSummary = (preset: LayoutPreset): string => {
    let totalWindows = 0
    let totalSessions = 0
    Object.values(preset.workspaces).forEach(ws => {
      totalWindows += ws.windowCount
      ws.windows.forEach(w => {
        totalSessions += w.boundSessions.length
      })
    })
    return `${totalWindows} windows, ${totalSessions} sessions`
  }

  if (!isOpen) return null

  return (
    <div className="presets-panel-overlay" onClick={onClose}>
      <div className="presets-panel" onClick={e => e.stopPropagation()}>
        <div className="presets-panel-header">
          <h2 className="presets-panel-title">Layout Presets</h2>
          <button className="presets-panel-close" onClick={onClose} aria-label="Close">
            ×
          </button>
        </div>

        <div className="presets-panel-body">
          {/* Save new preset */}
          <div className="preset-save-section">
            <h3 className="preset-section-title">Save Current Layout</h3>
            <div className="preset-save-form">
              <input
                type="text"
                className="preset-name-input"
                placeholder="Preset name..."
                value={newPresetName}
                onChange={e => setNewPresetName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSave()}
                maxLength={30}
              />
              <button
                className="preset-save-btn"
                onClick={handleSave}
                disabled={!newPresetName.trim() || layoutPresets.length >= 10}
              >
                Save
              </button>
            </div>
            {layoutPresets.length >= 10 && (
              <p className="preset-limit-warning">Maximum 10 presets reached</p>
            )}
          </div>

          {/* List presets */}
          <div className="preset-list-section">
            <h3 className="preset-section-title">
              Saved Presets {layoutPresets.length > 0 && `(${layoutPresets.length})`}
            </h3>
            {layoutPresets.length === 0 ? (
              <p className="preset-empty-message">
                No presets saved yet. Save your current layout to quickly restore it later.
              </p>
            ) : (
              <ul className="preset-list">
                {layoutPresets.map((preset, index) => (
                  <li key={preset.id} className="preset-item">
                    {editingId === preset.id ? (
                      <div className="preset-edit-form">
                        <input
                          type="text"
                          className="preset-edit-input"
                          value={editingName}
                          onChange={e => setEditingName(e.target.value)}
                          onKeyDown={e => {
                            if (e.key === 'Enter') handleRename()
                            if (e.key === 'Escape') cancelEditing()
                          }}
                          autoFocus
                          maxLength={30}
                        />
                        <button className="preset-edit-save" onClick={handleRename}>✓</button>
                        <button className="preset-edit-cancel" onClick={cancelEditing}>✕</button>
                      </div>
                    ) : (
                      <>
                        <div className="preset-info">
                          <span className="preset-shortcut">Ctrl+{index + 1}</span>
                          <span className="preset-name">{preset.name}</span>
                          <span className="preset-summary">{getPresetSummary(preset)}</span>
                        </div>
                        <div className="preset-actions">
                          <button
                            className="preset-load-btn"
                            onClick={() => handleLoad(preset.id)}
                            title="Load preset"
                          >
                            Load
                          </button>
                          <button
                            className="preset-rename-btn"
                            onClick={() => startEditing(preset)}
                            title="Rename preset"
                          >
                            ✎
                          </button>
                          <button
                            className="preset-delete-btn"
                            onClick={() => handleDelete(preset.id)}
                            title="Delete preset"
                          >
                            ✕
                          </button>
                        </div>
                      </>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>

        <div className="presets-panel-footer">
          <span className="presets-hint">
            Use <span className="keyboard-key">Ctrl</span>+<span className="keyboard-key">1-9</span> to quickly load presets
          </span>
        </div>
      </div>
    </div>
  )
}

export default LayoutPresetsPanel
