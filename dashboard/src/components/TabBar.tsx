import { useEffect, useRef, useState } from 'react'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { isFeatureEnabled } from '../featureFlags'
import { useSession } from '../context/SessionContext'
import { getTerminalLabel, isTerminalWorkspaceId } from '../types'
import type { WorkspaceId } from '../types'
import DismissiblePanel from './DismissiblePanel'
import Menu, { type MenuGroup } from './Menu'

export type Tab = WorkspaceId | 'files' | 'beads' | 'services' | 'scheduled' | 'server' | 'settings' | 'help'

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
  onShowKeys?: () => void
}

interface TabMenuState {
  x: number
  y: number
  workspaceId: WorkspaceId
}

function TabBar({ activeTab, onTabChange, onShowKeys }: TabBarProps) {
  const [keysMenu, setKeysMenu] = useState<{ x: number; y: number } | null>(null)
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [tabMenu, setTabMenu] = useState<TabMenuState | null>(null)
  // Naming a preset happens in the menu, in the row the operator chose.
  const [presetName, setPresetName] = useState<string | null>(null)
  // Renaming a tab happens in the tab, in the tab bar, like a session tag.
  const [renaming, setRenaming] = useState<{ workspaceId: WorkspaceId; value: string } | null>(null)
  const renameInputRef = useRef<HTMLInputElement>(null)
  const { settings, updateSettings, saveCurrentLayout, loadPreset, deletePreset, layoutPresets, clearWorkspaceAssignments, workspaceIds } = useSession()

  const isMobile = useMediaQuery('(max-width: 768px)')

  useEffect(() => {
    if (!renaming) return
    renameInputRef.current?.focus()
    renameInputRef.current?.select()
  }, [renaming?.workspaceId])

  const tabs: TabConfig[] = [
    ...workspaceIds.map((id): InternalTab => ({ id, label: settings.terminalLabels[id]?.trim() || getTerminalLabel(id) })),
    { id: 'files', label: 'Files' },
    { id: 'beads', label: 'Beads' },
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

  const closeTabMenu = () => {
    setTabMenu(null)
    setPresetName(null)
  }
  const toggleMobileMenu = () => {
    closeTabMenu()
    setKeysMenu(null)
    setMobileMenuOpen(open => !open)
  }
  // The toggle is device-local and states what it is, not what pressing it
  // does: "Keys on" means chords are live. The panel's own row turns them off,
  // and this button is how they come back.
  const keysEnabled = settings.keysEnabled
  const toggleKeys = () => {
    setKeysMenu(null)
    updateSettings({ keysEnabled: !keysEnabled })
  }

  const commitRename = () => {
    if (!renaming) return
    updateSettings({
      terminalLabels: { ...settings.terminalLabels, [renaming.workspaceId]: renaming.value.trim() },
    })
    setRenaming(null)
  }

  const commitPresetName = () => {
    const name = (presetName ?? '').trim()
    if (name) saveCurrentLayout(name)
    closeTabMenu()
  }

  const activeTerminalWorkspace = isTerminalWorkspaceId(activeTab, workspaceIds) ? activeTab : null
  const openActiveTabMenu = (anchor: HTMLElement) => {
    if (!activeTerminalWorkspace) return
    const rect = anchor.getBoundingClientRect()
    setKeysMenu(null)
    setMobileMenuOpen(false)
    setPresetName(null)
    setTabMenu({ x: rect.left, y: rect.bottom, workspaceId: activeTerminalWorkspace })
  }

  const presetRows = layoutPresets.length === 0
    ? [{ id: 'preset-none', label: 'No presets', disabled: true }]
    : layoutPresets.map(preset => ({
      id: `restore-${preset.id}`,
      label: preset.name,
      onSelect: () => loadPreset(preset.id),
    }))

  const tabMenuGroups: MenuGroup[] = tabMenu === null ? [] : [
    {
      id: 'tab',
      rows: [
        {
          id: 'rename',
          label: 'Rename tab',
          onSelect: () => setRenaming({
            workspaceId: tabMenu.workspaceId,
            value: settings.terminalLabels[tabMenu.workspaceId] || getTerminalLabel(tabMenu.workspaceId),
          }),
        },
      ],
    },
    {
      id: 'layout',
      rows: presetName === null
        ? [
          { id: 'save-preset', label: 'Save layout as preset', keepOpen: true, onSelect: () => setPresetName('') },
          { id: 'restore-preset', label: 'Restore preset', submenu: presetRows },
          ...(layoutPresets.length > 0
            ? [{
              id: 'delete-preset',
              label: 'Delete preset',
              danger: true,
              submenu: layoutPresets.map(preset => ({
                id: `delete-${preset.id}`,
                label: preset.name,
                danger: true,
                confirmLabel: `Confirm delete ${preset.name}`,
                onSelect: () => deletePreset(preset.id),
              })),
            }]
            : []),
        ]
        : [
          {
            id: 'preset-name',
            node: (
              <input
                className="menu-inline-input"
                autoFocus
                placeholder="Preset name"
                maxLength={30}
                value={presetName}
                onChange={event => setPresetName(event.target.value)}
                onKeyDown={event => {
                  event.stopPropagation()
                  if (event.key === 'Enter') commitPresetName()
                  if (event.key === 'Escape') setPresetName(null)
                }}
              />
            ),
          },
        ],
    },
    {
      id: 'assignments',
      rows: [
        {
          id: 'clear',
          label: 'Clear tab assignments',
          danger: true,
          confirmLabel: 'Confirm clear',
          onSelect: () => clearWorkspaceAssignments(tabMenu.workspaceId),
        },
      ],
    },
  ]

  const renderTab = (tab: TabConfig) => {
    if (!tab.external && renaming?.workspaceId === tab.id) {
      return (
        <input
          key={tab.id}
          ref={renameInputRef}
          className="tab-rename-input"
          aria-label={`Rename ${tab.label}`}
          value={renaming.value}
          onChange={event => setRenaming({ workspaceId: renaming.workspaceId, value: event.target.value })}
          onKeyDown={event => {
            event.stopPropagation()
            if (event.key === 'Enter') commitRename()
            if (event.key === 'Escape') setRenaming(null)
          }}
          onBlur={commitRename}
        />
      )
    }
    return (
      <button
        key={tab.id}
        className={`tab ${!tab.external && activeTab === tab.id ? 'active' : ''} ${tab.external ? 'external' : ''}`}
        onClick={() => handleClick(tab)}
        onContextMenu={event => {
          if (tab.external || tab.id !== activeTab || activeTerminalWorkspace === null) return
          event.preventDefault()
          openActiveTabMenu(event.currentTarget)
        }}
        title={tab.external ? `Open ${tab.label.replace(' ↗', '')} in new tab` : undefined}
      >
        {tab.label}
      </button>
    )
  }

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
                  toggleKeys()
                  setMobileMenuOpen(false)
                }}
              >
                {keysEnabled ? 'Keys on' : 'Keys off'}
              </button>
              <button
                className="mobile-nav-item"
                onClick={() => {
                  if (onShowKeys) onShowKeys()
                  setMobileMenuOpen(false)
                }}
              >
                Keybindings
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
            {tabs.map(renderTab)}
          </div>
          <div className="tab-bar-actions">
            {activeTerminalWorkspace && (
              <button className={`tab ${tabMenu ? 'dismissible-trigger-active' : ''}`} onClick={(event) => openActiveTabMenu(event.currentTarget)}>
                ⋯ Tab
              </button>
            )}
            <div className="keys-menu-container">
              <button
                className={`tab keys-toggle ${keysMenu ? 'active dismissible-trigger-active' : ''}`}
                onClick={toggleKeys}
                onContextMenu={(event) => {
                  event.preventDefault()
                  const rect = event.currentTarget.getBoundingClientRect()
                  closeTabMenu()
                  setMobileMenuOpen(false)
                  setKeysMenu({ x: rect.left, y: rect.bottom })
                }}
                aria-pressed={keysEnabled}
                title={keysEnabled ? 'Chords are live. Click to turn keys off; right-click for the keybindings.' : 'Chords are off. Click to turn keys on; right-click for the keybindings.'}
              >
                {keysEnabled ? 'Keys on' : 'Keys off'}
              </button>
            </div>
          </div>
        </>
      )}
      {keysMenu && (
        <Menu
          at={keysMenu}
          label="Keys"
          zIndex={2300}
          estimatedSize={{ width: 220, height: 80 }}
          onClose={() => setKeysMenu(null)}
          groups={[{
            id: 'keys',
            rows: [
              ...(onShowKeys ? [{ id: 'panel', label: 'Keybindings', chord: 'Alt+K', onSelect: onShowKeys }] : []),
              { id: 'help', label: 'Dashboard Help', onSelect: () => onTabChange('help') },
            ],
          }]}
        />
      )}
      {tabMenu && (
        <Menu
          at={{ x: tabMenu.x, y: tabMenu.y }}
          label="Terminal tab actions"
          estimatedSize={{ width: 230, height: 180 }}
          onClose={closeTabMenu}
          groups={tabMenuGroups}
        />
      )}
    </div>
  )
}

export default TabBar
