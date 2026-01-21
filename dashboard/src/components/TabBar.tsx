import MusicPlayer from './MusicPlayer'

export type Tab = 'terminal1' | 'terminal2' | 'files' | 'beads' | 'mail' | 'settings' | 'help' | 'mobile'

interface InternalTab {
  id: Tab
  label: string
  external?: false
}

interface ExternalTab {
  id: string
  label: string
  external: true
  url: string
}

type TabConfig = InternalTab | ExternalTab

interface TabBarProps {
  activeTab: Tab
  onTabChange: (tab: Tab) => void
  onShowHelp?: () => void
  onShowPresets?: () => void
}

function TabBar({ activeTab, onTabChange, onShowHelp, onShowPresets }: TabBarProps) {
  const tabs: TabConfig[] = [
    { id: 'terminal1', label: 'Terminal' },
    { id: 'terminal2', label: 'Terminal 2' },
    { id: 'files', label: 'Files' },
    { id: 'beads', label: 'Beads' },
    { id: 'mail', label: 'Mail' },
    { id: 'settings', label: 'Settings' },
    { id: 'help', label: 'Help' },
  ]

  const handleClick = (tab: TabConfig) => {
    if (tab.external) {
      window.open(tab.url, '_blank', 'noopener,noreferrer')
    } else {
      onTabChange(tab.id)
    }
  }

  return (
    <div className="tab-bar">
      <button
        className={`tab-bar-mobile-btn ${activeTab === 'mobile' ? 'active' : ''}`}
        onClick={() => onTabChange('mobile')}
        title="Mobile Chat"
      >
        <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
          <path d="M17 1.01L7 1c-1.1 0-2 .9-2 2v18c0 1.1.9 2 2 2h10c1.1 0 2-.9 2-2V3c0-1.1-.9-1.99-2-1.99zM17 19H7V5h10v14z"/>
        </svg>
        Mobile
      </button>
      <div className="tab-bar-tabs">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={`tab ${!tab.external && activeTab === tab.id ? 'active' : ''} ${tab.external ? 'external' : ''}`}
            onClick={() => handleClick(tab)}
            title={tab.external ? `Open ${tab.label.replace(' ↗', '')} in new tab` : undefined}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div className="tab-bar-actions">
        {onShowPresets && (
          <button
            className="tab-bar-btn presets-btn"
            onClick={onShowPresets}
            title="Layout Presets"
          >
            ⊞
          </button>
        )}
        {onShowHelp && (
          <button
            className="tab-bar-btn help-btn"
            onClick={onShowHelp}
            title="Keyboard Shortcuts (?)"
          >
            ?
          </button>
        )}
        <MusicPlayer />
      </div>
    </div>
  )
}

export default TabBar
