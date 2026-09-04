import { useCallback, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { X } from 'lucide-react'
import { useSession } from '../context/SessionContext'
import { getGroupPriority } from '../types'
import type { WorkspaceId } from '../types'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { useResizableWidth } from '../hooks/useResizableWidth'
import Launcher from './Launcher'
import SessionGroup from './SessionGroup'
import DismissiblePanel from './DismissiblePanel'

type SessionPanelProps = {
  activeWorkspaceId: WorkspaceId
  collapsed?: boolean
  width?: number
  pinned?: boolean
  panelId?: string
  onClose?: () => void
  onWidthChange?: (width: number) => void
  searchTerm?: string
  collapsedGroups?: string[]
  onSearchTermChange?: (searchTerm: string) => void
  onCollapsedGroupsChange?: (collapsedGroups: string[]) => void
}

function SessionPanel({
  activeWorkspaceId,
  collapsed,
  width = 260,
  pinned = false,
  panelId,
  onClose,
  onWidthChange,
  searchTerm: controlledSearchTerm,
  collapsedGroups: controlledCollapsedGroups,
  onSearchTermChange,
  onCollapsedGroupsChange,
}: SessionPanelProps) {
  // The list refreshes itself on a poll; there is no button that says so.
  const { groupedSessions, loading, error, sidebarCollapsed } = useSession()
  const isCollapsed = collapsed ?? sidebarCollapsed
  const [localSearchTerm, setLocalSearchTerm] = useState('')
  const [localCollapsedGroups, setLocalCollapsedGroups] = useState<string[]>([])
  const searchTerm = controlledSearchTerm ?? localSearchTerm
  const collapsedGroups = controlledCollapsedGroups ?? localCollapsedGroups
  const updateSearchTerm = (nextSearchTerm: string) => {
    if (controlledSearchTerm !== undefined) onSearchTermChange?.(nextSearchTerm)
    else setLocalSearchTerm(nextSearchTerm)
  }
  const updateGroupExpanded = (groupKey: string, expanded: boolean) => {
    const nextCollapsedGroups = expanded
      ? collapsedGroups.filter(key => key !== groupKey)
      : Array.from(new Set([...collapsedGroups, groupKey]))
    if (controlledCollapsedGroups !== undefined) onCollapsedGroupsChange?.(nextCollapsedGroups)
    else setLocalCollapsedGroups(nextCollapsedGroups)
  }
  const [launcher, setLauncher] = useState<{ show: boolean; x: number; y: number }>({ show: false, x: 0, y: 0 })
  const panelRef = useRef<HTMLDivElement>(null)

  const launcherPosition = useViewportMenuPosition<HTMLDivElement>(
    launcher.show ? { x: launcher.x, y: launcher.y } : null,
    { estimatedSize: { width: 420, height: 400 } },
  )

  // Sort groups by priority and filter by search
  const sortedGroups = useMemo(() => {
    // Determine which groups/sessions to show
    const entries = Object.entries(groupedSessions)

    // Filter
    const filtered = searchTerm
      ? entries.map(([key, sessions]) => ([
          key,
          sessions.filter(s =>
            s.name.toLowerCase().includes(searchTerm.toLowerCase())
          )
        ] as [string, typeof sessions])).filter(([_, sessions]) => sessions.length > 0)
      : entries

    return filtered.sort(([a], [b]) => {
      const priorityA = getGroupPriority(a)
      const priorityB = getGroupPriority(b)
      if (priorityA !== priorityB) return priorityA - priorityB
      return a.localeCompare(b)
    })
  }, [groupedSessions, searchTerm])

  // The launcher is a glance until something has been typed into it: then a
  // press outside is an ordinary press, and only Escape or a launch closes it.
  const [launcherTyped, setLauncherTyped] = useState(false)
  const closeLauncher = () => {
    setLauncher({ show: false, x: 0, y: 0 })
    setLauncherTyped(false)
  }

  const widest = useCallback(() => 480, [])
  const commitWidth = useCallback((next: number) => onWidthChange?.(next), [onWidthChange])
  const resize = useResizableWidth({
    elementRef: panelRef,
    width,
    minWidth: 220,
    maxWidth: widest,
    edge: 'right',
    onCommit: commitWidth,
  })

  const panelStyle = isCollapsed ? undefined : ({ '--session-panel-width': `${resize.width}px` } as CSSProperties)

  return (
    <div
      ref={panelRef}
      id={panelId}
      className={`session-panel ${isCollapsed ? 'collapsed' : ''} ${pinned ? 'sidecar-pinned' : 'sidecar-overlay'}`}
      style={panelStyle}
      aria-label="Sessions sidecar"
      data-active-workspace={activeWorkspaceId}
    >
      <div className="session-panel-header" data-ui="sessions.header">
        <strong className="terminal-sidecar-title">Sessions</strong>
        {!isCollapsed && (
          <>
            <button
              className="add-btn"
              aria-expanded={launcher.show}
              onClick={(event) => {
                if (launcher.show) {
                  closeLauncher()
                  return
                }
                const rect = event.currentTarget.getBoundingClientRect()
                setLauncher({ show: true, x: rect.left, y: rect.bottom + 4 })
              }}
              title="New tmux session"
            >
              +
            </button>
            {onClose && (
              <button
                type="button"
                className="toggle-btn sidecar-close-btn"
                aria-label="Close Sessions sidecar"
                title="Close sidecar"
                onClick={onClose}
              >
                <X size={16} aria-hidden="true" />
              </button>
            )}
          </>
        )}
      </div>

      {!isCollapsed && launcher.show && (
        <DismissiblePanel onDismiss={closeLauncher} panelPosition="fixed" kind={launcherTyped ? 'work' : 'glance'}>
          <div
            ref={launcherPosition.ref}
            role="dialog"
            aria-label="Launch a tmux session"
            className="session-launcher-popup"
            style={launcherPosition.style}
          >
            <Launcher workspaceId={activeWorkspaceId} onLaunched={closeLauncher} onTypedChange={setLauncherTyped} />
          </div>
        </DismissiblePanel>
      )}

      {!isCollapsed && (
        <div className="session-search-container">
          <input
            type="text"
            className="session-search-input"
            placeholder="Filter sessions..."
            value={searchTerm}
            onChange={(e) => updateSearchTerm(e.target.value)}
          />
        </div>
      )}

      {!isCollapsed && (
        <div className="session-panel-content">
          {loading && (
            <div className="panel-status">Loading...</div>
          )}

          {error && (
            <div className="panel-error">{error}</div>
          )}

          {!loading && !error && sortedGroups.length === 0 && (
            <div className="getting-started">
              <div className="getting-started-title">Getting Started</div>
              <div className="getting-started-text">
                Click <strong>+</strong> above to create your first tmux session,
                or use the terminal to run <code>tmux new -s mysession</code>
              </div>
              <div className="getting-started-hint">
                Sessions appear here and can be dragged to terminal windows
              </div>
            </div>
          )}

          {sortedGroups.map(([groupKey, groupSessions]) => (
            <SessionGroup
              key={groupKey}
              groupKey={groupKey}
              sessions={groupSessions}
              expanded={!collapsedGroups.includes(groupKey)}
              onExpandedChange={expanded => updateGroupExpanded(groupKey, expanded)}
            />
          ))}
        </div>
      )}

      {!isCollapsed && onWidthChange && (
        <div
          {...resize.handleProps}
          className={`dock-resizer${resize.resizing ? ' dragging' : ''}`}
          role="separator"
          aria-label="Resize Sessions panel"
          aria-orientation="vertical"
          aria-valuenow={Math.round(resize.width)}
          aria-valuemin={220}
          aria-valuemax={480}
          tabIndex={0}
        />
      )}
    </div>
  )
}

export default SessionPanel
import './SessionPanel.css'
