import { useState, useRef, useEffect } from 'react'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { isFeatureEnabled } from '../featureFlags'

export type Tab = 'terminal1' | 'terminal2' | 'files' | 'agents' | 'beads' | 'formations' | 'services' | 'server' | 'settings' | 'help'

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
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const helpMenuRef = useRef<HTMLDivElement>(null)

  const isMobile = useMediaQuery('(max-width: 768px)')

  // Close menu when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      // Close help menu
      if (helpMenuRef.current && !helpMenuRef.current.contains(e.target as Node)) {
        setHelpMenuOpen(false)
      }

      // Close mobile menu if clicking outside tab bar
      const target = e.target as HTMLElement
      if (mobileMenuOpen && !target.closest('.tab-bar')) {
        setMobileMenuOpen(false)
      }
    }

    if (helpMenuOpen || mobileMenuOpen) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [helpMenuOpen, mobileMenuOpen])

  const tabs: TabConfig[] = [
    { id: 'terminal1', label: 'Terminal' },
    { id: 'terminal2', label: 'Terminal 2' },
    { id: 'files', label: 'Files' },
    { id: 'agents', label: 'Agents' },
    { id: 'beads', label: 'Beads' },
    { id: 'formations', label: 'Formations' },
    { id: 'services', label: 'Services' },
    ...(isFeatureEnabled('serverStatusTab') ? [{ id: 'server' as const, label: 'Server' }] : []),
    { id: 'settings', label: 'Settings' },
  ]

  const handleClick = (tab: TabConfig) => {
    if (tab.external) {
      window.open(tab.url, '_blank', 'noopener,noreferrer')
    } else {
      onTabChange(tab.id)
      setMobileMenuOpen(false)
    }
  }

  const activeTabLabel = tabs.find(t => t.id === activeTab)?.label || 'Menu'

  return (
    <div className={`tab-bar ${isMobile ? 'mobile-mode' : ''}`}>
      {isMobile ? (
        <>
          <div className="tab-bar-mobile-start">
            <button
              className={`tab hamburger-btn ${mobileMenuOpen ? 'active' : ''}`}
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            >
              ☰
            </button>
            <span className="mobile-active-tab">{activeTabLabel}</span>
          </div>

          {/* Mobile Menu Dropdown */}
          {mobileMenuOpen && (
            <div className="mobile-nav-dropdown">
              {tabs.map((tab) => (
                <button
                  key={tab.id}
                  className={`mobile-nav-item ${!tab.external && activeTab === tab.id ? 'active' : ''}`}
                  onClick={() => handleClick(tab)}
                >
                  {tab.label}
                </button>
              ))}

              <div className="mobile-nav-divider"></div>

              {onShowPresets && (
                <button
                  className="mobile-nav-item"
                  onClick={() => {
                    onShowPresets()
                    setMobileMenuOpen(false)
                  }}
                >
                 ⊞ Layouts
                </button>
              )}
              <button
                className="mobile-nav-item"
                onClick={() => {
                  if (onShowHelp) onShowHelp()
                  setMobileMenuOpen(false)
                }}
              >
                Keyboard Shortcuts
              </button>
              <button
                className="mobile-nav-item"
                onClick={() => {
                   onTabChange('help')
                   setMobileMenuOpen(false)
                }}
              >
                Dashboard Help
              </button>
            </div>
          )}
        </>
      ) : (
        <>
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
                </div>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  )
}

export default TabBar
