import { useState, useRef, useEffect } from 'react'
import MusicPlayer from './MusicPlayer'

export type Tab = 'terminal1' | 'terminal2' | 'files' | 'beads' | 'chat' | 'manual' | 'settings' | 'help'

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
  const [helpMenuOpen, setHelpMenuOpen] = useState(false)
  const helpMenuRef = useRef<HTMLDivElement>(null)

  // Close menu when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (helpMenuRef.current && !helpMenuRef.current.contains(e.target as Node)) {
        setHelpMenuOpen(false)
      }
    }
    if (helpMenuOpen) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [helpMenuOpen])

  const tabs: TabConfig[] = [
    { id: 'chat', label: '✉ Chat' },
    { id: 'terminal1', label: 'Terminal' },
    { id: 'terminal2', label: 'Terminal 2' },
    { id: 'files', label: 'Files' },
    { id: 'beads', label: 'Beads' },
    { id: 'settings', label: 'Settings' },
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
      <div className="tab-bar-actions">
        {onShowPresets && (
          <button
            className="tab"
            onClick={onShowPresets}
            title="Layout Presets"
          >
            ⊞ Layouts
          </button>
        )}
        <div className="help-menu-container" ref={helpMenuRef}>
          <button
            className={`tab ${helpMenuOpen ? 'active' : ''}`}
            onClick={() => setHelpMenuOpen(!helpMenuOpen)}
            title="Help & Documentation"
          >
            ?
          </button>
          {helpMenuOpen && (
            <div className="help-dropdown">
              {onShowHelp && (
                <button
                  className="help-dropdown-item"
                  onClick={() => {
                    onShowHelp()
                    setHelpMenuOpen(false)
                  }}
                >
                  Keyboard Shortcuts
                </button>
              )}
              <button
                className="help-dropdown-item"
                onClick={() => {
                  onTabChange('help')
                  setHelpMenuOpen(false)
                }}
              >
                Dashboard Help
              </button>
              <button
                className="help-dropdown-item"
                onClick={() => {
                  onTabChange('manual')
                  setHelpMenuOpen(false)
                }}
              >
                Gastown Operators Manual
              </button>
            </div>
          )}
        </div>
        <MusicPlayer />
      </div>
    </div>
  )
}

export default TabBar
