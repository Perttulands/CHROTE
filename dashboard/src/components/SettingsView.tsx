import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useSession } from '../context/SessionContext'
import { useToast } from '../context/ToastContext'
import type { UserSettings, TmuxAppearance, WorkspaceId, LaunchUser, FormationsTextSize } from '../types'
import { resolveFormationsTextSize } from '../types'
import { MAX_TERMINAL_TAB_COUNT, MIN_TERMINAL_TAB_COUNT, TMUX_PRESETS, defaultSessionPrefixForUser, defaultTerminalUserColor, getSessionPrefixForUser, getTerminalLabel, getTerminalUserColor, normalizeTerminalUsers, resolveLaunchUser } from '../types'
import FolderPickerModal from './FolderPickerModal'
import SessionBankSection from './SessionBankSection'
import NukeConfirmModal from './NukeConfirmModal'
import { toDisplayPath } from './FilesView/types'

// Color input component with picker and text field
interface ColorInputProps {
  label: string
  value: string
  onChange: (value: string) => void
}

function ColorInput({ label, value, onChange }: ColorInputProps) {
  const pickerValue = /^#[0-9a-fA-F]{6}$/.test(value) ? value : '#000000'

  return (
    <div className="color-input-group">
      <span className="color-input-label">{label}</span>
      <div className="color-input-controls">
        <input
          type="color"
          value={pickerValue}
          onChange={(e) => onChange(e.target.value)}
          className="color-picker"
          aria-label={`${label} picker`}
        />
        <input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="color-text-input"
          placeholder="#000000"
          aria-label={`${label} value`}
        />
      </div>
    </div>
  )
}

function normalizeProjectPath(path: string): string {
  const trimmed = path.trim()
  if (trimmed === '/') return trimmed
  return trimmed.replace(/\/+$/, '')
}

type SettingsViewProps = {
  sessionBankFocusNonce?: number
}

function SettingsView({ sessionBankFocusNonce = 0 }: SettingsViewProps = {}) {
  const { settings, updateSettings, terminalUsers, sessions, refreshSessions, workspaceIds } = useSession()
  const { addToast } = useToast()
  const [showFolderPicker, setShowFolderPicker] = useState(false)
  const [projectPathInput, setProjectPathInput] = useState('')
  const [sessionBankCollapsed, setSessionBankCollapsed] = useState(true)
  const [showNukeModal, setShowNukeModal] = useState(false)
  const [nuking, setNuking] = useState(false)
  const sessionBankRef = useRef<HTMLDivElement>(null)
  const configuredUsers = normalizeTerminalUsers(terminalUsers.length > 0
    ? terminalUsers
    : [
        ...Object.values(settings.terminalLaunchUsers),
        ...Object.keys(settings.terminalSessionPrefixes),
        ...Object.keys(settings.terminalUserColors),
      ])

  useEffect(() => {
    if (!sessionBankFocusNonce) return
    setSessionBankCollapsed(false)
    window.requestAnimationFrame(() => {
      sessionBankRef.current?.scrollIntoView({ block: 'start' })
    })
  }, [sessionBankFocusNonce])

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

  const handleThemeChange = (theme: UserSettings['theme']) => {
    updateSettings({ theme })
  }

  const handleFormationsTextSizeChange = (formationsTextSize: FormationsTextSize) => {
    updateSettings({ formationsTextSize })
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

  const handleUserColorChange = (launchUser: LaunchUser, color: string) => {
    updateSettings({
      terminalUserColors: {
        ...settings.terminalUserColors,
        [launchUser]: color,
      },
    })
  }

  const handleTmuxColorChange = (key: keyof TmuxAppearance, value: string) => {
    updateSettings({
      tmuxAppearance: {
        ...settings.tmuxAppearance,
        [key]: value,
      }
    })
  }

  const applyTmuxPreset = (presetName: string) => {
    const preset = TMUX_PRESETS[presetName]
    if (preset) {
      updateSettings({ tmuxAppearance: preset })
    }
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
      addToast('All sessions destroyed', 'warning')
      refreshSessions()
    } catch (error) {
      console.error('Failed to destroy sessions:', error)
      addToast('Failed to destroy sessions', 'error')
    } finally {
      setNuking(false)
      setShowNukeModal(false)
    }
  }

  return (
    <div className="settings-view">
      <h1 className="settings-title">Settings</h1>


      {/* Appearance Section */}
      <section className="settings-section">
        <h2 className="settings-section-title">Appearance</h2>

        <div className="settings-field">
          <label className="settings-label">Theme</label>
          <div className="settings-theme-options">
            {(['matrix', 'dark', 'gastown'] as const).map((theme) => (
              <button
                key={theme}
                className={`theme-option ${settings.theme === theme ? 'selected' : ''} theme-${theme}`}
                onClick={() => handleThemeChange(theme)}
              >
                {theme.charAt(0).toUpperCase() + theme.slice(1)}
              </button>
            ))}
          </div>
        </div>

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
          <label className="settings-label">Formations text size</label>
          <div className="settings-theme-options">
            {(['default', 'large', 'xlarge'] as const).map(size => (
              <button
                key={size}
                className={`theme-option ${resolveFormationsTextSize(settings.formationsTextSize) === size ? 'selected' : ''}`}
                onClick={() => handleFormationsTextSizeChange(size)}
                data-testid={`formations-textsize-${size}`}
              >
                {size === 'default' ? 'Default' : size === 'large' ? 'Large' : 'X-Large'}
              </button>
            ))}
          </div>
          <p className="settings-hint">Scales card titles, roster rows, and labels on the Formations board.</p>
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

      {/* tmux Appearance Section */}
      <section className="settings-section">
        <h2 className="settings-section-title">tmux Appearance</h2>
        <p className="settings-description">
          Customize tmux colors. Changes apply instantly to all running sessions.
        </p>

        {/* Preset Buttons */}
        <div className="settings-field">
          <label className="settings-label">Presets</label>
          <div className="settings-theme-options">
            {(['matrix', 'dark', 'gastown'] as const).map((preset) => (
              <button
                key={preset}
                className={`theme-option theme-${preset}`}
                onClick={() => applyTmuxPreset(preset)}
              >
                {preset.charAt(0).toUpperCase() + preset.slice(1)}
              </button>
            ))}
          </div>
        </div>

        {/* Status Bar Colors */}
        <div className="settings-field">
          <label className="settings-label">Status Bar</label>
          <div className="settings-color-row">
            <ColorInput
              label="Background"
              value={settings.tmuxAppearance?.statusBg ?? '#000000'}
              onChange={(v) => handleTmuxColorChange('statusBg', v)}
            />
            <ColorInput
              label="Foreground"
              value={settings.tmuxAppearance?.statusFg ?? '#00ff41'}
              onChange={(v) => handleTmuxColorChange('statusFg', v)}
            />
          </div>
        </div>

        {/* Pane Border Colors */}
        <div className="settings-field">
          <label className="settings-label">Pane Borders</label>
          <div className="settings-color-row">
            <ColorInput
              label="Active"
              value={settings.tmuxAppearance?.paneBorderActive ?? '#00ff41'}
              onChange={(v) => handleTmuxColorChange('paneBorderActive', v)}
            />
            <ColorInput
              label="Inactive"
              value={settings.tmuxAppearance?.paneBorderInactive ?? '#333333'}
              onChange={(v) => handleTmuxColorChange('paneBorderInactive', v)}
            />
          </div>
        </div>

        {/* Selection / Copy Mode Colors */}
        <div className="settings-field">
          <label className="settings-label">Selection / Copy Mode</label>
          <div className="settings-color-row">
            <ColorInput
              label="Background"
              value={settings.tmuxAppearance?.modeStyleBg ?? '#00ff41'}
              onChange={(v) => handleTmuxColorChange('modeStyleBg', v)}
            />
            <ColorInput
              label="Foreground"
              value={settings.tmuxAppearance?.modeStyleFg ?? '#000000'}
              onChange={(v) => handleTmuxColorChange('modeStyleFg', v)}
            />
          </div>
        </div>

        <p className="settings-hint">
          Use hex colors (#00ff41) or named colors (green, black, etc.)
        </p>
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
          <label className="settings-label">Session User Indicators</label>
          {configuredUsers.length === 0 ? (
            <p className="settings-hint">No terminal users configured by the server yet.</p>
          ) : (
            <div className="settings-color-row">
              {configuredUsers.map(launchUser => (
                <ColorInput
                  key={launchUser}
                  label={`${launchUser} badge color`}
                  value={getTerminalUserColor(settings, launchUser) || defaultTerminalUserColor(launchUser)}
                  onChange={(value) => handleUserColorChange(launchUser, value)}
                />
              ))}
            </div>
          )}
          <p className="settings-hint">Small badges in the Sessions panel use the configured Unix username initial and color.</p>
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

      {/* Session Bank Section */}
      <div ref={sessionBankRef}>
        <SessionBankSection
          collapsed={sessionBankCollapsed}
          onCollapsedChange={setSessionBankCollapsed}
          className="settings-session-bank"
        />
      </div>

      <section className="settings-section settings-danger-zone">
        <h2 className="settings-section-title">Advanced session recovery</h2>
        <p className="settings-description">Bulk destruction is an emergency action. Individual session controls are safer.</p>
        <button
          className="nuke-trigger-btn"
          onClick={() => setShowNukeModal(true)}
          disabled={nuking || sessions.length === 0}
        >
          ☢ {nuking ? 'Nuking…' : 'Nuke All sessions'}
        </button>
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

      {showNukeModal && (
        <NukeConfirmModal
          sessionCount={sessions.length}
          sessionNames={sessions.map(session => session.name)}
          protectedSessionNames={sessions.filter(session => session.persistent).map(session => session.name)}
          onConfirm={nukeAllSessions}
          onCancel={() => setShowNukeModal(false)}
        />
      )}
    </div>
  )
}

export default SettingsView
