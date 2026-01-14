import { useSession } from '../context/SessionContext'
import type { UserSettings } from '../types'

function SettingsView() {
  const { settings, updateSettings } = useSession()

  const handleTerminalModeChange = (mode: UserSettings['terminalMode']) => {
    updateSettings({ terminalMode: mode })
  }

  const handleThemeChange = (theme: UserSettings['theme']) => {
    updateSettings({ theme })
  }

  const handleFontSizeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(e.target.value, 10)
    if (value >= 12 && value <= 20) {
      updateSettings({ fontSize: value })
    }
  }

  const handleRefreshIntervalChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    updateSettings({ autoRefreshInterval: parseInt(e.target.value, 10) })
  }

  const handlePrefixChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    updateSettings({ defaultSessionPrefix: e.target.value })
  }

  return (
    <div className="settings-view">
      <h1 className="settings-title">Settings</h1>

      {/* Terminal Mode Section */}
      <section className="settings-section">
        <h2 className="settings-section-title">Terminal Mode</h2>
        <p className="settings-description">
          Choose the default mode for new terminal sessions.
        </p>
        <div className="settings-radio-group">
          <label className={`settings-radio ${settings.terminalMode === 'tmux' ? 'selected' : ''}`}>
            <input
              type="radio"
              name="terminalMode"
              value="tmux"
              checked={settings.terminalMode === 'tmux'}
              onChange={() => handleTerminalModeChange('tmux')}
            />
            <span className="radio-label">
              <strong>tmux</strong>
              <span className="radio-description">
                Sessions are managed by tmux. Supports detaching, multiple windows, and persistence.
              </span>
            </span>
          </label>
          <label className={`settings-radio ${settings.terminalMode === 'shell' ? 'selected' : ''}`}>
            <input
              type="radio"
              name="terminalMode"
              value="shell"
              checked={settings.terminalMode === 'shell'}
              onChange={() => handleTerminalModeChange('shell')}
            />
            <span className="radio-label">
              <strong>shell</strong>
              <span className="radio-description">
                Plain bash shell. Simpler but sessions are lost when closed.
              </span>
            </span>
          </label>
        </div>
      </section>

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
          <label className="settings-label">Default Session Prefix</label>
          <input
            type="text"
            value={settings.defaultSessionPrefix}
            onChange={handlePrefixChange}
            className="settings-input"
            placeholder="shell"
            maxLength={20}
          />
          <p className="settings-hint">Prefix used when creating new sessions (e.g., "shell-abc123")</p>
        </div>
      </section>

      {/* Info Section */}
      <section className="settings-section settings-info">
        <p>Settings are automatically saved to your browser's local storage.</p>
      </section>
    </div>
  )
}

export default SettingsView
