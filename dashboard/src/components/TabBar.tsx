import { useState } from 'react'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { isFeatureEnabled } from '../featureFlags'
import { useSession } from '../context/SessionContext'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { TERMINAL_LABELS, TERMINAL_WORKSPACE_IDS } from '../types'
import type { WorkspaceId } from '../types'
import DismissiblePanel from './DismissiblePanel'

export type Tab = 'terminal1' | 'terminal2' | 'terminal3' | 'files' | 'agents' | 'beads' | 'formations' | 'services' | 'scheduled' | 'server' | 'settings' | 'help'

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
  const tabMenuPosition = useViewportMenuPosition<HTMLDivElement>(
    tabMenu.show ? { x: tabMenu.x, y: tabMenu.y } : null,
    { estimatedSize: { width: 230, height: 210 } },
  )
  const { settings, updateSettings, saveCurrentLayout, loadPreset, layoutPresets, clearWorkspaceAssignments } = useSession()

  const isMobile = useMediaQuery('(max-width: 768px)')


  const tabs: TabConfig[] = [
    { id: 'terminal1', label: settings.terminalLabels.terminal1?.trim() || TERMINAL_LABELS.terminal1 },
    { id: 'terminal2', label: settings.terminalLabels.terminal2?.trim() || TERMINAL_LABELS.terminal2 },
    { id: 'terminal3', label: settings.terminalLabels.terminal3?.trim() || TERMINAL_LABELS.terminal3 },
    { id: 'files', label: 'Files' },
    { id: 'agents', label: 'Agents' },
    { id: 'beads', label: 'Beads' },
    { id: 'formations', label: 'Formations' },
    { id: 'services', label: 'Services' },
    { id: 'scheduled', label: 'Scheduled' },
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
  const toggleMobileMenu = () => {
    closeTabMenu()
    setHelpMenuOpen(false)
    setMobileMenuOpen(open => !open)
  }
  const toggleHelpMenu = () => {
    closeTabMenu()
    setMobileMenuOpen(false)
    setHelpMenuOpen(open => !open)
  }

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
  const activeTerminalWorkspace = TERMINAL_WORKSPACE_IDS.includes(activeTab as WorkspaceId)
    ? activeTab as WorkspaceId
    : null
  const openActiveTabMenu = (button: HTMLButtonElement) => {
    if (!activeTerminalWorkspace) return
    const rect = button.getBoundingClientRect()
    setHelpMenuOpen(false)
    setMobileMenuOpen(false)
    setTabMenu({ show: true, x: rect.left, y: rect.bottom + 4, workspaceId: activeTerminalWorkspace, submenu: null })
  }
  const tabOptionsMenu = terminalTabMenu && tabMenu.show ? (
    <DismissiblePanel onDismiss={closeTabMenu} panelPosition="fixed">
      <div ref={tabMenuPosition.ref} className="session-context-menu" style={tabMenuPosition.style}>
      <button className="session-context-item" onClick={renameTab}>
        <span className="session-context-icon">✎</span>
        Rename tab label
      </button>
      <button className="session-context-item" onClick={saveLayoutFromTab}>
        <span className="session-context-icon">▣</span>
        Save layout as preset
      </button>
      <div className="session-context-submenu-trigger">
        <button
          className="session-context-item"
          aria-expanded={tabMenu.submenu === 'preset'}
          onClick={() => setTabMenu(prev => ({ ...prev, submenu: prev.submenu === 'preset' ? null : 'preset' }))}
        >
          <span className="session-context-icon">⊞</span>
          Restore layout preset
          <span className="session-context-arrow">▶</span>
        </button>
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
    </DismissiblePanel>
  ) : null

  return (
    <div className={`tab-bar ${isMobile ? 'mobile-mode' : ''}`}>
      {isMobile ? (
        <>
          <div className="tab-bar-mobile-start">
            <button
              className={`tab hamburger-btn ${mobileMenuOpen ? 'active dismissible-trigger-active' : ''}`}
              onClick={toggleMobileMenu}
            >
              ☰
            </button>
            <span className="mobile-active-tab">{activeTabLabel}</span>
          </div>

          {/* Mobile Menu Dropdown */}
          {mobileMenuOpen && (
            <DismissiblePanel onDismiss={() => setMobileMenuOpen(false)} panelPosition="absolute">
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
              {activeTerminalWorkspace && (
                <button
                  className="mobile-nav-item"
                  onClick={(event) => {
                    openActiveTabMenu(event.currentTarget)
                    setMobileMenuOpen(false)
                  }}
                >
                  Terminal tab options
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
            </DismissiblePanel>
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
            {activeTerminalWorkspace && (
              <button className={`tab ${tabMenu.show ? 'dismissible-trigger-active' : ''}`} onClick={(event) => openActiveTabMenu(event.currentTarget)}>
                ⋯ Tab
              </button>
            )}
            {onShowPresets && (
              <button
                className="tab"
                onClick={onShowPresets}
                title="Layout Presets"
              >
                ⊞ Layouts
              </button>
            )}
            <div className="help-menu-container">
              <button
                className={`tab ${helpMenuOpen ? 'active dismissible-trigger-active' : ''}`}
                onClick={toggleHelpMenu}
                title="Help & Documentation"
              >
                ?
              </button>
              {helpMenuOpen && (
                <DismissiblePanel onDismiss={() => setHelpMenuOpen(false)} panelZIndex={1000} panelPosition="absolute">
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
                </DismissiblePanel>
              )}
            </div>
          </div>
        </>
      )}
      {tabOptionsMenu}
    </div>
  )
}

export default TabBar
