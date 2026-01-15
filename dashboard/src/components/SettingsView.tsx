import { useSession } from '../context/SessionContext'
import type { UserSettings, TmuxAppearance } from '../types'
import { TMUX_PRESETS } from '../types'

// Color input component with picker and text field
interface ColorInputProps {
  label: string
  value: string
  onChange: (value: string) => void
}

function ColorInput({ label, value, onChange }: ColorInputProps) {
  return (
    <div className="color-input-group">
      <span className="color-input-label">{label}</span>
      <div className="color-input-controls">
        <input
          type="color"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="color-picker"
        />
        <input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="color-text-input"
          placeholder="#000000"
        />
      </div>
    </div>
  )
}

function SettingsView() {
  const { settings, updateSettings } = useSession()

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
