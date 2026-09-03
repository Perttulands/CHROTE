import { useEffect, useRef, useState } from 'react'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { isFeatureEnabled } from '../featureFlags'
import { useSession } from '../context/SessionContext'
import { getTerminalLabel, isTerminalWorkspaceId } from '../types'
import type { WorkspaceId } from '../types'
import DismissiblePanel from './DismissiblePanel'
import Menu, { type MenuGroup } from './Menu'
import { CLAIM_EXPLANATION } from './TerminalWindow'
import { useTerminalPool } from './TerminalPool'

export type Tab = WorkspaceId | 'files' | 'beads' | 'library' | 'agents' | 'scheduled' | 'server' | 'settings' | 'help'

/** The counts a workspace can show, as the grid classes and the clamp allow. */
const WINDOW_COUNTS = [1, 2, 3, 4] as const

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
  /** The Sessions panel's own setting, kept here because its header no longer has room for it. */
  sessionsPinned?: boolean
  onToggleSessionsPinned?: () => void
}

interface TabMenuState {
  x: number
  y: number
  workspaceId: WorkspaceId
}

function TabBar({ activeTab, onTabChange, onShowKeys, sessionsPinned = false, onToggleSessionsPinned }: TabBarProps) {
  const [keysMenu, setKeysMenu] = useState<{ x: number; y: number } | null>(null)
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [tabMenu, setTabMenu] = useState<TabMenuState | null>(null)
  // Naming a preset happens in the menu, in the row the operator chose.
  const [presetName, setPresetName] = useState<string | null>(null)
  // Renaming a tab happens in the tab, in the tab bar, like a session tag.
  const [renaming, setRenaming] = useState<{ workspaceId: WorkspaceId; value: string } | null>(null)
  const renameInputRef = useRef<HTMLInputElement>(null)
  const { settings, updateSettings, saveCurrentLayout, loadPreset, deletePreset, layoutPresets, clearWorkspaceAssignments, setWindowCount, workspaceIds, workspaces } = useSession()
  const pool = useTerminalPool()

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
    { id: 'library', label: 'Library' },
    { id: 'agents', label: 'Agents' },
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

  // The frames this tab is showing. Both maintenance actions used to sit on the
  // workspace strip; they act on the same windows from here.
  const visibleWindows = activeTerminalWorkspace
    ? (workspaces[activeTerminalWorkspace]?.windows.slice(0, workspaces[activeTerminalWorkspace].windowCount) ?? [])
    : []
  const boundInView = Array.from(new Set(visibleWindows.flatMap(window => window.boundSessions)))
  const activeInView = Array.from(new Set(
    visibleWindows.map(window => window.activeSession).filter((key): key is string => Boolean(key)),
  ))

  const reconnectFrames = () => {
    boundInView.forEach(sessionKey => pool.terminals.get(sessionKey)?.reconnect())
  }
  // Claiming resizes a tmux window for every client watching it, so it is
  // offered only for the frames in front of this device. A phone shows one
  // slide at a time, and the tab bar cannot tell which: there, the tile's own
  // tag menu claims the frame the operator is actually looking at.
  const claimSessionsInView = () => {
    activeInView.forEach(sessionKey => pool.terminals.get(sessionKey)?.claim())
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
          // Alt+= and Alt+- are the chords for this, and a phone has neither.
          // The row is here on every viewport because it costs a desktop
          // nothing and is the only way a phone changes the count.
          {
            id: 'windows',
            label: 'Windows',
            state: String(workspaces[tabMenu.workspaceId]?.windowCount ?? 1),
            submenu: WINDOW_COUNTS.map(count => ({
              id: `windows-${count}`,
              label: String(count),
              onSelect: () => setWindowCount(tabMenu.workspaceId, count),
            })),
          },
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
        {
          id: 'reconnect',
          label: 'Reconnect frames',
          disabled: boundInView.length === 0,
          onSelect: reconnectFrames,
        },
        {
          id: 'claim',
          label: 'Claim all',
          reason: isMobile
            ? 'One frame is on screen here; claim it from its own tag menu.'
            : CLAIM_EXPLANATION,
          disabled: isMobile || activeInView.length === 0,
          onSelect: claimSessionsInView,
        },
      ],
    },
    ...(onToggleSessionsPinned ? [{
      id: 'panels',
      rows: [
        {
          id: 'pin-sessions',
          label: 'Pin sessions panel',
          state: sessionsPinned ? 'on' : 'off',
          onSelect: onToggleSessionsPinned,
        },
      ],
    }] : []),
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
    // The active terminal tab is its own menu trigger: a caret appears on hover
    // and the secondary button opens the same menu anywhere on the tab.
    const carries = !tab.external && tab.id === activeTab && activeTerminalWorkspace !== null
    return (
      <button
        key={tab.id}
        className={`tab ${!tab.external && activeTab === tab.id ? 'active' : ''} ${tab.external ? 'external' : ''} ${carries ? 'tab-with-menu' : ''} ${carries && tabMenu ? 'dismissible-trigger-active' : ''}`}
        aria-haspopup={carries ? 'menu' : undefined}
        aria-expanded={carries ? tabMenu !== null : undefined}
        onClick={event => {
          if (carries && (event.target as HTMLElement).closest('.tab-menu-caret')) {
            openActiveTabMenu(event.currentTarget)
            return
          }
          handleClick(tab)
        }}
        onKeyDown={event => {
          if (!carries) return
          if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return
          event.preventDefault()
          openActiveTabMenu(event.currentTarget)
        }}
        onContextMenu={event => {
          if (!carries) return
          event.preventDefault()
          openActiveTabMenu(event.currentTarget)
        }}
        title={tab.external ? `Open ${tab.label.replace(' ↗', '')} in new tab` : undefined}
      >
        {tab.label}
        {carries && <span className="tab-menu-caret" aria-hidden="true">▾</span>}
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
          estimatedSize={{ width: 230, height: 210 }}
          onClose={closeTabMenu}
          groups={tabMenuGroups}
        />
      )}
    </div>
  )
}

export default TabBar
