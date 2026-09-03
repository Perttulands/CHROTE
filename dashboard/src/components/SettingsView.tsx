import { useState } from 'react'
import type { FormEvent } from 'react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import type { WorkspaceId, LaunchUser } from '../types'
import { MAX_TERMINAL_TAB_COUNT, MIN_TERMINAL_TAB_COUNT, defaultSessionPrefixForUser, getSessionPrefixForUser, getTerminalLabel, normalizeTerminalUsers, resolveLaunchUser } from '../types'
import FolderPickerModal from './FolderPickerModal'
import { useConfirmInPlace } from './confirmInPlace'
import { toDisplayPath } from './FilesView/types'

function normalizeProjectPath(path: string): string {
  const trimmed = path.trim()
  if (trimmed === '/') return trimmed
  return trimmed.replace(/\/+$/, '')
}

// Sessions the server refuses to destroy, whatever the dashboard asks.
const PROTECTED_SESSIONS = new Set(['chrote-chat'])

function SettingsView() {
  const { settings, updateSettings, terminalUsers, sessions, refreshSessions, workspaceIds } = useSession()
  const { announce } = useStatus()
  const [showFolderPicker, setShowFolderPicker] = useState(false)
  const [projectPathInput, setProjectPathInput] = useState('')
  const [nuking, setNuking] = useState(false)
  const configuredUsers = normalizeTerminalUsers(terminalUsers.length > 0
    ? terminalUsers
    : [
        ...Object.values(settings.terminalLaunchUsers),
        ...Object.keys(settings.terminalSessionPrefixes),
      ])

  const handleAddProjectPath = (path: string) => {
    const normalizedPath = normalizeProjectPath(path)
    if (!normalizedPath) {
      setShowFolderPicker(false)
      return
    }

    const currentPaths = settings.beadsProjectPaths || []
    const exists = currentPaths.some(existing => normalizeProjectPath(existing) === normalizedPath)
    if (!exists) {
      updateSettings({ beadsProjectPaths: [...currentPaths, normalizedPath] })
    }
    setProjectPathInput('')
    setShowFolderPicker(false)
  }

  const handleProjectPathSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    handleAddProjectPath(projectPathInput)
  }

  const handleRemoveProjectPath = (pathToRemove: string) => {
    const currentPaths = settings.beadsProjectPaths || []
    updateSettings({ beadsProjectPaths: currentPaths.filter(p => p !== pathToRemove) })
  }

  const handleFontSizeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(e.target.value, 10)
    if (value >= 12 && value <= 20) {
      updateSettings({ fontSize: value })
    }
  }

  const handleMouseScrollChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    updateSettings({ mouseScroll: e.target.checked })
  }

  const handleHideScrollbarChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    updateSettings({ hideScrollbar: e.target.checked })
  }

  const handleRefreshIntervalChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    updateSettings({ autoRefreshInterval: parseInt(e.target.value, 10) })
  }

  const handleLaunchUserChange = (workspaceId: WorkspaceId, launchUser: LaunchUser) => {
    updateSettings({
      terminalLaunchUsers: {
        ...settings.terminalLaunchUsers,
        [workspaceId]: launchUser,
      },
    })
  }

  const handleSessionPrefixChange = (launchUser: LaunchUser, prefix: string) => {
    updateSettings({
      terminalSessionPrefixes: {
        ...settings.terminalSessionPrefixes,
        [launchUser]: prefix,
      },
    })
  }

  const nukeAllSessions = async () => {
    setNuking(true)
    try {
      const response = await fetch('/api/tmux/sessions/all', {
        method: 'DELETE',
        headers: { 'X-Nuke-Confirm': 'DASHBOARD-NUKE-CONFIRMED' },
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) throw new Error(await response.text())
      announce('All sessions destroyed', 'warning')
      refreshSessions()
    } catch (error) {
      console.error('Failed to destroy sessions:', error)
      announce('Failed to destroy sessions', 'error')
    } finally {
      setNuking(false)
    }
  }

  // The button that destroys is the button that asks. A first press arms it and
  // names what is at stake; a second within three seconds runs it.
  const nuke = useConfirmInPlace(() => { void nukeAllSessions() })
  const protectedNames = sessions.map(session => session.name).filter(name => PROTECTED_SESSIONS.has(name))
  const killableCount = sessions.length - protectedNames.length

  return (
    <div className="settings-view">
      <h1 className="settings-title">Settings</h1>


      {/* Appearance Section */}
      <section className="settings-section">
        <h2 className="settings-section-title">Appearance</h2>

        <div className="settings-field">
          <label className="settings-label">
            Font Size: {settings.fontSize}px
          </label>
          <input
            type="range"
            min="12"
            max="20"
            value={settings.fontSize}
            onChange={handleFontSizeChange}
            className="settings-slider"
          />
          <div className="slider-labels">
            <span>12px</span>
            <span>20px</span>
          </div>
        </div>

        <div className="settings-field">
          <label
            className="settings-label"
            style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}
          >
            <input
              type="checkbox"
              checked={settings.mouseScroll}
              onChange={handleMouseScrollChange}
            />
            Mouse-wheel scrolling
          </label>
          <p className="settings-hint">
            Enables tmux mouse mode so the scroll wheel scrolls history. Applies instantly to
            configured CHROTE terminal sockets. This is global per tmux server and makes
            click-drag select inside tmux; hold Shift for browser text selection.
          </p>
        </div>

        <div className="settings-field">
          <label
            className="settings-label"
            style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}
          >
            <input
              type="checkbox"
              checked={settings.hideScrollbar}
              onChange={handleHideScrollbarChange}
            />
            Hide terminal scrollbar
          </label>
          <p className="settings-hint">
            Hides the xterm scrollbar gutter in terminal windows. Under tmux that scrollback is
            empty, so the bar is dead UI; scrolling stays on the mouse wheel via tmux history.
            Applies instantly to all open terminals.
          </p>
        </div>
      </section>

      {/* Session Defaults Section */}
      <section className="settings-section">
        <h2 className="settings-section-title">Session Defaults</h2>

        <div className="settings-field">
          <label className="settings-label">Auto-refresh Interval</label>
          <select
            value={settings.autoRefreshInterval}
            onChange={handleRefreshIntervalChange}
            className="settings-select"
          >
            <option value={1000}>1 second</option>
            <option value={2000}>2 seconds</option>
            <option value={5000}>5 seconds</option>
            <option value={10000}>10 seconds</option>
            <option value={30000}>30 seconds</option>
          </select>
          <p className="settings-hint">How often to check for new tmux sessions</p>
        </div>

        <div className="settings-field">
          <label className="settings-label">Session Prefixes</label>
          {configuredUsers.length === 0 ? (
            <p className="settings-hint">No terminal users configured by the server yet.</p>
          ) : (
            <div className="settings-color-row">
              {configuredUsers.map(launchUser => {
                const label = `${launchUser} session prefix`
                const id = `session-prefix-${launchUser}`
                return (
                  <div key={launchUser} className="color-input-group">
                    <label className="color-input-label" htmlFor={id}>{label}</label>
                    <input
                      id={id}
                      aria-label={label}
                      type="text"
                      value={getSessionPrefixForUser(settings, launchUser, configuredUsers)}
                      onChange={(e) => handleSessionPrefixChange(launchUser, e.target.value)}
                      className="settings-input"
                      placeholder={defaultSessionPrefixForUser(launchUser, configuredUsers)}
                      maxLength={20}
                    />
                  </div>
                )
              })}
            </div>
          )}
          <p className="settings-hint">Prefix used when creating new sessions for each Unix user (e.g., "user-abc123")</p>
        </div>

        <div className="settings-field">
          <label className="settings-label" htmlFor="terminal-tab-count">Terminal tabs</label>
          <select
            id="terminal-tab-count"
            aria-label="Terminal tabs"
            className="settings-select"
            value={settings.terminalTabCount}
            onChange={(e) => updateSettings({ terminalTabCount: Number(e.target.value) })}
          >
            {Array.from({ length: MAX_TERMINAL_TAB_COUNT - MIN_TERMINAL_TAB_COUNT + 1 }, (_, i) => MIN_TERMINAL_TAB_COUNT + i).map(count => (
              <option key={count} value={count}>{count}</option>
            ))}
          </select>
          <p className="settings-hint">Hiding tabs never deletes their layouts or sessions; raise the count to bring them back.</p>
        </div>

        <div className="settings-field">
          <label className="settings-label">Terminal launch users</label>
          <div className="settings-color-row">
            {workspaceIds.map(workspaceId => {
              const label = `${getTerminalLabel(workspaceId)} launch user`
              const id = `launch-user-${workspaceId}`
              const value = resolveLaunchUser(settings, workspaceId, configuredUsers)
              return (
                <div key={workspaceId} className="color-input-group">
                  <label className="color-input-label" htmlFor={id}>{label}</label>
                  <select
                    id={id}
                    aria-label={label}
                    className="settings-select"
                    value={value}
                    onChange={(e) => handleLaunchUserChange(workspaceId, e.target.value as LaunchUser)}
                    disabled={configuredUsers.length === 0}
                  >
                    {configuredUsers.map(user => (
                      <option key={user} value={user}>{user}</option>
                    ))}
                  </select>
                </div>
              )
            })}
          </div>
          <p className="settings-hint">Controls which Unix user's tmux socket new shells attach to in each terminal tab.</p>
        </div>
      </section>

      <section className="settings-section settings-danger-zone">
        <h2 className="settings-section-title">Session cleanup</h2>
        <p className="settings-description">Bulk destruction is an emergency action. Individual session controls are safer.</p>
        <button
          className={nuke.armed ? 'nuke-trigger-btn armed' : 'nuke-trigger-btn'}
          onClick={nuke.press}
          disabled={nuking || sessions.length === 0}
        >
          {nuking
            ? 'Nuking…'
            : nuke.armed
              ? `Confirm: destroy ${killableCount} session${killableCount === 1 ? '' : 's'}`
              : 'Nuke All sessions'}
        </button>
        {nuke.armed && protectedNames.length > 0 && (
          <p className="settings-hint">Preserved: {protectedNames.join(', ')}</p>
        )}
      </section>

      {/* Beads Projects Section */}
      <section className="settings-section">
        <h2 className="settings-section-title">Beads Projects</h2>
        <p className="settings-description">
          Manually add project paths for beads discovery. Use this for projects in nested directories.
        </p>

        {/* List of saved project paths */}
        <div className="beads-project-list">
          {settings.beadsProjectPaths && settings.beadsProjectPaths.length > 0 ? (
            settings.beadsProjectPaths.map(path => (
              <div key={path} className="beads-project-item">
                <span className="beads-project-path">{toDisplayPath(path)}</span>
                <button
                  type="button"
                  className="beads-project-remove"
                  onClick={() => handleRemoveProjectPath(path)}
                  title="Remove project"
                  aria-label={`Remove ${path}`}
                >
                  &times;
                </button>
              </div>
            ))
          ) : (
            <p className="settings-hint">No manual project paths configured.</p>
          )}
        </div>

        <form className="beads-project-add-form" onSubmit={handleProjectPathSubmit}>
          <label className="settings-label" htmlFor="beads-project-path-input">Project path</label>
          <div className="beads-project-add-row">
            <input
              id="beads-project-path-input"
              aria-label="Beads project path"
              type="text"
              className="settings-input"
              value={projectPathInput}
              onChange={(event) => setProjectPathInput(event.target.value)}
              placeholder="/workspace/project"
            />
            <button
              type="submit"
              className="settings-btn"
              disabled={!projectPathInput.trim()}
            >
              Add Path
            </button>
            <button
              type="button"
              className="settings-btn settings-btn-add"
              onClick={() => setShowFolderPicker(true)}
            >
              Browse...
            </button>
          </div>
          <p className="settings-hint">Enter an absolute path to a modern Beads project containing .beads, or browse for one.</p>
        </form>
      </section>

      {/* Info Section */}
      <section className="settings-section settings-info">
        <p>Settings are automatically saved to your browser's local storage.</p>
      </section>

      {/* Folder Picker Modal */}
      {showFolderPicker && (
        <FolderPickerModal
          onSelect={handleAddProjectPath}
          onClose={() => setShowFolderPicker(false)}
        />
      )}

    </div>
  )
}

export default SettingsView
import './SettingsView.css'
