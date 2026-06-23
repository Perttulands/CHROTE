import { useState, useRef, useEffect } from 'react'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { isFeatureEnabled } from '../featureFlags'
import { useSession } from '../context/SessionContext'
import { TERMINAL_LABELS, TERMINAL_WORKSPACE_IDS } from '../types'
import type { WorkspaceId } from '../types'

export type Tab = 'terminal1' | 'terminal2' | 'terminal3' | 'files' | 'agents' | 'beads' | 'formations' | 'services' | 'server' | 'settings' | 'help'

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
  const [tabMenu, setTabMenu] = useState<{ show: boolean; x: number; y: number; workspaceId: WorkspaceId | null; submenu: string | null }>({ show: false, x: 0, y: 0, workspaceId: null, submenu: null })
  const helpMenuRef = useRef<HTMLDivElement>(null)
  const { settings, updateSettings, saveCurrentLayout, loadPreset, layoutPresets, clearWorkspaceAssignments } = useSession()

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
      if (tabMenu.show && !target.closest('.session-context-menu')) {
        setTabMenu({ show: false, x: 0, y: 0, workspaceId: null, submenu: null })
      }
    }

    if (helpMenuOpen || mobileMenuOpen || tabMenu.show) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [helpMenuOpen, mobileMenuOpen, tabMenu.show])

  const tabs: TabConfig[] = [
    { id: 'terminal1', label: settings.terminalLabels.terminal1?.trim() || TERMINAL_LABELS.terminal1 },
    { id: 'terminal2', label: settings.terminalLabels.terminal2?.trim() || TERMINAL_LABELS.terminal2 },
    { id: 'terminal3', label: settings.terminalLabels.terminal3?.trim() || TERMINAL_LABELS.terminal3 },
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

  const closeTabMenu = () => setTabMenu({ show: false, x: 0, y: 0, workspaceId: null, submenu: null })

  const renameTab = () => {
    if (!tabMenu.workspaceId) return
    const label = window.prompt('Terminal tab label', settings.terminalLabels[tabMenu.workspaceId] || TERMINAL_LABELS[tabMenu.workspaceId])?.trim() ?? ''
    updateSettings({
      terminalLabels: {
        ...settings.terminalLabels,
        [tabMenu.workspaceId]: label,
      },
    })
    closeTabMenu()
  }

  const saveLayoutFromTab = () => {
    const name = window.prompt('Layout preset name')?.trim()
    if (name) saveCurrentLayout(name)
    closeTabMenu()
  }

  const terminalTabMenu = tabMenu.workspaceId

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
                onContextMenu={(event) => {
                  if (tab.external || !TERMINAL_WORKSPACE_IDS.includes(tab.id as WorkspaceId)) return
                  event.preventDefault()
                  setTabMenu({ show: true, x: event.clientX, y: event.clientY, workspaceId: tab.id as WorkspaceId, submenu: null })
                }}
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
          {terminalTabMenu && tabMenu.show && (
            <div className="session-context-menu" style={{ left: tabMenu.x, top: tabMenu.y }}>
              <button className="session-context-item" onClick={renameTab}>
                <span className="session-context-icon">✎</span>
                Rename tab label
              </button>
              <button className="session-context-item" onClick={saveLayoutFromTab}>
                <span className="session-context-icon">▣</span>
                Save layout as preset
              </button>
              <div
                className="session-context-item session-context-submenu-trigger"
                onMouseEnter={() => setTabMenu(prev => ({ ...prev, submenu: 'preset' }))}
              >
                <span className="session-context-icon">⊞</span>
                Restore layout preset
                <span className="session-context-arrow">▶</span>
                {tabMenu.submenu === 'preset' && (
                  <div className="session-context-submenu">
                    {layoutPresets.length === 0 ? (
                      <button className="session-context-item" disabled>No presets</button>
                    ) : layoutPresets.map(preset => (
                      <button key={preset.id} className="session-context-item" onClick={() => { loadPreset(preset.id); closeTabMenu() }}>
                        {preset.name}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <button
                className="session-context-item"
                onClick={() => {
                  clearWorkspaceAssignments(terminalTabMenu)
                  closeTabMenu()
                }}
              >
                <span className="session-context-icon">⌫</span>
                Clear tab assignments
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}

export default TabBar
