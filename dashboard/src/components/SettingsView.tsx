import { useState } from 'react'
import type { FormEvent } from 'react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import type { WorkspaceId, LaunchUser } from '../types'
import { MAX_TERMINAL_TAB_COUNT, MIN_TERMINAL_TAB_COUNT, defaultSessionPrefixForUser, getSessionPrefixForUser, getTerminalLabel, normalizeTerminalUsers, resolveLaunchUser } from '../types'
import FolderField from './FolderField'
import { useConfirmInPlace } from './confirmInPlace'
import { askNotificationPermission, notificationPermissionWord } from '../agents/browserNotifications'
import { audioContext, playTone } from '../agents/tones'
import type { AgentEventKind } from '../types'
import Choice from './kit/Choice'
import Toggle from './kit/Toggle'

const REFRESH_INTERVALS = [
  { value: '1000', label: '1 second' },
  { value: '2000', label: '2 seconds' },
  { value: '5000', label: '5 seconds' },
  { value: '10000', label: '10 seconds' },
  { value: '30000', label: '30 seconds' },
]

function normalizeProjectPath(path: string): string {
  const trimmed = path.trim()
  if (trimmed === '/') return trimmed
  return trimmed.replace(/\/+$/, '')
}

function SettingsView() {
  const { settings, updateSettings, terminalUsers, sessions, refreshSessions, workspaceIds } = useSession()
  const { announce } = useStatus()
  const [projectPathInput, setProjectPathInput] = useState('')
  const [nuking, setNuking] = useState(false)
  const [notificationPermission, setNotificationPermission] = useState(notificationPermissionWord)
  const configuredUsers = normalizeTerminalUsers(terminalUsers.length > 0
    ? terminalUsers
    : [
        ...Object.values(settings.terminalLaunchUsers),
        ...Object.keys(settings.terminalSessionPrefixes),
      ])

  const handleAddProjectPath = (path: string) => {
    const normalizedPath = normalizeProjectPath(path)
    if (!normalizedPath) return

    const currentPaths = settings.beadsProjectPaths || []
    const exists = currentPaths.some(existing => normalizeProjectPath(existing) === normalizedPath)
    if (!exists) {
      updateSettings({ beadsProjectPaths: [...currentPaths, normalizedPath] })
    }
    setProjectPathInput('')
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

  const handleMouseScrollChange = (checked: boolean) => {
    updateSettings({ mouseScroll: checked })
  }

  const handleHideScrollbarChange = (checked: boolean) => {
    updateSettings({ hideScrollbar: checked })
  }

  const handleRefreshIntervalChange = (value: string) => {
    updateSettings({ autoRefreshInterval: parseInt(value, 10) })
  }

  const handleMarksChange = (checked: boolean) => {
    updateSettings({ agentEventMarks: checked })
  }

  const handleToastChange = (checked: boolean) => {
    updateSettings({ agentEventToast: checked })
  }

  const handleTonesChange = (checked: boolean) => {
    updateSettings({ agentEventTones: checked })
  }

  // The click is the gesture the browser wants before it will make a sound.
  const playAgentTone = (event: AgentEventKind) => {
    const context = audioContext()
    if (!context) {
      announce('This browser offers no Web Audio', 'error')
      return
    }
    playTone(event, context)
  }

  // Turning notifications on asks the browser, and the word beside the setting
  // reads back what it answered. Anything but granted leaves the setting off:
  // a setting that is on and does nothing is a lie.
  const handleNotificationsChange = async (checked: boolean) => {
    if (!checked) {
      updateSettings({ agentEventNotifications: false })
      return
    }
    const word = await askNotificationPermission()
    setNotificationPermission(word)
    if (word === 'granted') updateSettings({ agentEventNotifications: true })
    else announce(`Browser notifications: ${word}`, 'error')
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
            aria-label="Font size"
            className="settings-slider kit-range"
          />
          <div className="slider-labels">
            <span>12px</span>
            <span>20px</span>
          </div>
        </div>

        <div className="settings-field">
          <Toggle className="settings-toggle" checked={settings.mouseScroll} onChange={handleMouseScrollChange}>
            Mouse-wheel scrolling
          </Toggle>
          <p className="settings-hint">
            Enables tmux mouse mode so the scroll wheel scrolls history. Applies instantly to
            configured CHROTE terminal sockets. This is global per tmux server and makes
            click-drag select inside tmux; hold Shift for browser text selection.
          </p>
        </div>

        <div className="settings-field">
          <Toggle className="settings-toggle" checked={settings.hideScrollbar} onChange={handleHideScrollbarChange}>
            Hide terminal scrollbar
          </Toggle>
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
          <Choice
            label="Auto-refresh interval"
            value={String(settings.autoRefreshInterval)}
            onChange={handleRefreshIntervalChange}
            options={REFRESH_INTERVALS}
          />
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
          <Choice
            id="terminal-tab-count"
            label="Terminal tabs"
            value={String(settings.terminalTabCount)}
            onChange={value => updateSettings({ terminalTabCount: Number(value) })}
            options={Array.from({ length: MAX_TERMINAL_TAB_COUNT - MIN_TERMINAL_TAB_COUNT + 1 }, (_, i) => {
              const count = String(MIN_TERMINAL_TAB_COUNT + i)
              return { value: count, label: count }
            })}
          />
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
                  <Choice
                    id={id}
                    label={label}
                    value={value}
                    onChange={next => handleLaunchUserChange(workspaceId, next as LaunchUser)}
                    disabled={configuredUsers.length === 0}
                    options={configuredUsers.map(user => ({ value: user, label: user }))}
                  />
                </div>
              )
            })}
          </div>
          <p className="settings-hint">Controls which Unix user's tmux socket new shells attach to in each terminal tab.</p>
        </div>
      </section>

      {/* Agent events: the four ways this device can tell the operator. */}
      <section className="settings-section" data-ui="settings.agent-events">
        <h2 className="settings-section-title">Agent events</h2>
        <p className="settings-description">
          When an agent finishes or needs input, its own hook reports it. How this device passes
          that on is its own choice; every other device keeps its own. Focusing the session marks
          the report seen everywhere, whatever is turned on here.
        </p>

        <div className="settings-field">
          <Toggle className="settings-toggle" checked={settings.agentEventMarks} onChange={handleMarksChange}>
            Mark the session&apos;s row and the tab holding it
          </Toggle>
          <p className="settings-hint">
            A dot and the report&apos;s first line, until the session is focused.
          </p>
        </div>

        <div className="settings-field">
          <Toggle className="settings-toggle" checked={settings.agentEventToast} onChange={handleToastChange}>
            Name the session in the toast
          </Toggle>
          <p className="settings-hint">
            The status line keeps the record either way.
          </p>
        </div>

        <div className="settings-field">
          <Toggle className="settings-toggle" checked={settings.agentEventTones} onChange={handleTonesChange}>
            Tones
          </Toggle>
          <div className="settings-tone-list">
            <div className="settings-tone-row">
              <span>Finished: two notes, falling</span>
              <button
                type="button"
                className="settings-btn"
                aria-label="Play the finished tone"
                onClick={() => playAgentTone('finished')}
              >
                Play
              </button>
            </div>
            <div className="settings-tone-row">
              <span>Needs input: one note, higher</span>
              <button
                type="button"
                className="settings-btn"
                aria-label="Play the needs-input tone"
                onClick={() => playAgentTone('needs-input')}
              >
                Play
              </button>
            </div>
          </div>
          <p className="settings-hint">
            Made in the browser, 120 ms a note. The browser plays them once you have clicked or typed on the page.
          </p>
        </div>

        <div className="settings-field">
          <Toggle className="settings-toggle" checked={settings.agentEventNotifications} onChange={handleNotificationsChange}>
            Browser notifications while this tab is hidden
          </Toggle>
          <p className="settings-hint">
            Permission: <span data-ui="settings.notification-permission">{notificationPermission}</span>.
            Turning this on asks the browser, which grants it only on a secure origin.
          </p>
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
              ? `Confirm: destroy ${sessions.length} session${sessions.length === 1 ? '' : 's'}`
              : 'Nuke All sessions'}
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
                <span className="beads-project-path">{path}</span>
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
            <FolderField
              id="beads-project-path-input"
              ariaLabel="Beads project path"
              inputClassName="settings-input"
              value={projectPathInput}
              onChange={setProjectPathInput}
              onSubmit={handleAddProjectPath}
              placeholder="/workspace/project"
            />
            <button
              type="submit"
              className="settings-btn"
              disabled={!projectPathInput.trim()}
            >
              Add Path
            </button>
          </div>
          <p className="settings-hint">
            An absolute path to a modern Beads project containing .beads. A fragment finds a workspace; Tab completes a folder.
          </p>
        </form>
      </section>

      {/* Info Section */}
      <section className="settings-section settings-info">
        <p>Settings are automatically saved to your browser's local storage.</p>
      </section>

    </div>
  )
}

export default SettingsView
import './SettingsView.css'
