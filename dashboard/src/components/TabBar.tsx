import MusicPlayer from './MusicPlayer'

type Tab = 'terminal' | 'files' | 'status' | 'settings'

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
}

function TabBar({ activeTab, onTabChange }: TabBarProps) {
  const tabs: TabConfig[] = [
    { id: 'terminal', label: 'Terminal' },
    { id: 'files', label: 'Files' },
    { id: 'status', label: 'Status' },
    { id: 'settings', label: 'Settings' },
    { id: 'beads', label: 'Beads ↗', external: true, url: 'https://github.com/Dicklesworthstone/beads_viewer' },
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
      <MusicPlayer />
    </div>
  )
}

export default TabBar
