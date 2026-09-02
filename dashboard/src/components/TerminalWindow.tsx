import { useState, useEffect, useMemo, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { Send } from 'lucide-react'
import { useSession } from '../context/SessionContext'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { useTerminalPool } from './TerminalPool'
import TerminalSurface from './TerminalSurface'
import { getSessionBadges, getSessionKey, getSessionNameFromKey, getSessionUserFromKey, getTerminalUserInitial } from '../types'
import { SessionCommandMark, SessionLabel } from './sessionLabel'
import { identityColorFor } from '../theme/theme'
import { useTheme } from '../theme/ThemeContext'
import type { TerminalWindow as TerminalWindowType, WorkspaceId } from '../types'
import { isDetached, tileStateFor, type TileState } from '../terminal/tileState'
import { useSessionEvidence } from '../context/useSessionEvidence'
import DismissiblePanel from './DismissiblePanel'
import EmptyWindow from './EmptyWindow'

interface SessionTagProps {
  sessionName: string
  isActive: boolean
  workspaceId: WorkspaceId
  windowId: string
  onRemove: () => void
  onClick: () => void
  onOpenFilesAtPath?: (path: string) => void
  workspaceActive: boolean
  tileState: TileState
}

function SessionTag({ sessionName, isActive, workspaceId, windowId, onRemove, onClick, onOpenFilesAtPath, workspaceActive, tileState }: SessionTagProps) {
  const { sessions, terminalUsers, deleteSession, renameSession, openSendToSession } = useSession()
  const theme = useTheme()
  const pool = useTerminalPool()
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null)
  const [isRenaming, setIsRenaming] = useState(false)
  const [renameValue, setRenameValue] = useState('')
  const contextMenuPosition = useViewportMenuPosition<HTMLDivElement>(contextMenu, {
    estimatedSize: { width: 240, height: 320 },
  })
  const tagRef = useRef<HTMLDivElement | null>(null)
  const firstActionRef = useRef<HTMLButtonElement | null>(null)
  const renameInputRef = useRef<HTMLInputElement | null>(null)
  const actualName = getSessionNameFromKey(sessionName)
  const unixUser = getSessionUserFromKey(sessionName)
  const matchingSessions = unixUser
    ? sessions.filter(s => getSessionKey(s.name, s.unixUser) === sessionName)
    : sessions.filter(s => s.name === actualName)
  const session = matchingSessions.length === 1 ? matchingSessions[0] : undefined
  const resolvedUser = session?.unixUser || unixUser
  const sessionKey = getSessionKey(actualName, resolvedUser)
  const workingDirectory = session?.cwd || null
  // tmux owns the size of a manual window, so refitting the frame cannot move
  // it. The control says so instead of doing nothing, in the badge's words.
  const pinnedSize = session
    ? getSessionBadges(session).find(badge => badge.id === 'pinned-size')
    : undefined
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `tag-${workspaceId}-${windowId}-${sessionKey}`,
    data: { type: 'tag', sessionName: actualName, sessionKey, unixUser: resolvedUser, sourceWindowId: windowId, sourceWorkspaceId: workspaceId },
  })

  const style = isDragging
    ? { opacity: 0, transition: 'none' }
    : undefined

  // Show full tmux session name, including prefixes (e.g. critique-codex).
  const displayName = actualName
  const dragLabel = `Drag ${displayName}${resolvedUser ? ` (Unix user ${resolvedUser})` : ''}`

  // Handle click on the tag - only fire if not dragging
  const handleClick = (e: React.MouseEvent) => {
    // Don't trigger click if we're dragging
    if (isDragging) return
    // Don't trigger if clicking the remove button
    if ((e.target as HTMLElement).closest('.tag-remove')) return
    onClick()
  }

  const handleTagKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if ((event.target as HTMLElement).closest('.tag-remove')) return
    if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) {
      event.preventDefault()
      event.stopPropagation()
      const rect = event.currentTarget.getBoundingClientRect()
      openContextMenu(rect.left, rect.bottom)
      return
    }
    listeners?.onKeyDown?.(event)
  }

  const setTagNodeRef = useCallback((node: HTMLDivElement | null) => {
    tagRef.current = node
    setNodeRef(node)
  }, [setNodeRef])
  const closeContextMenu = useCallback((restoreFocus = true) => {
    setContextMenu(null)
    if (restoreFocus) tagRef.current?.focus()
  }, [])
  const openContextMenu = useCallback((x: number, y: number) => {
    setContextMenu({ x, y })
  }, [])
  const runContextAction = (action: () => void, restoreFocus = true) => {
    closeContextMenu(restoreFocus)
    action()
  }
  const startRename = () => {
    setRenameValue(actualName)
    setIsRenaming(true)
  }
  const submitRename = async () => {
    if (renameValue && renameValue !== actualName) {
      await renameSession(actualName, renameValue, resolvedUser)
    }
    setIsRenaming(false)
  }

  useEffect(() => {
    if (!workspaceActive) closeContextMenu(false)
  }, [closeContextMenu, workspaceActive])

  useEffect(() => {
    if (contextMenu) firstActionRef.current?.focus()
  }, [contextMenu])

  useEffect(() => {
    if (!isRenaming) return
    renameInputRef.current?.focus()
    renameInputRef.current?.select()
  }, [isRenaming])

  return (
    <>
      <div
        ref={setTagNodeRef}
        className={`session-tag ${isActive ? 'active' : ''} ${isDragging ? 'dragging' : ''}`}
        data-tile-state={tileState}
        style={style}
        title={dragLabel}
        {...attributes}
        {...listeners}
        aria-label={`Session ${displayName}`}
        onClick={handleClick}
        onKeyDown={handleTagKeyDown}
        onContextMenu={(event) => {
          if ((event.target as HTMLElement).closest('.tag-remove')) return
          event.preventDefault()
          event.stopPropagation()
          openContextMenu(event.clientX, event.clientY)
        }}
      >
        {resolvedUser && (
          <span
            className="session-user-badge"
            style={{ backgroundColor: identityColorFor(resolvedUser, terminalUsers, theme) }}
            title={`Unix user: ${resolvedUser}`}
          >
            {getTerminalUserInitial(resolvedUser)}
          </span>
        )}
        {isRenaming ? (
          <input
            ref={renameInputRef}
            className="session-tag-rename-input"
            aria-label={`Rename session ${displayName}`}
            value={renameValue}
            onPointerDown={event => event.stopPropagation()}
            onClick={event => event.stopPropagation()}
            onChange={event => setRenameValue(event.target.value)}
            onKeyDown={event => {
              event.stopPropagation()
              if (event.key === 'Enter') {
                event.preventDefault()
                void submitRename()
              } else if (event.key === 'Escape') {
                event.preventDefault()
                setIsRenaming(false)
              }
            }}
            onBlur={() => void submitRename()}
          />
        ) : (
          <>
            <SessionCommandMark command={session?.currentCommand} />
            <SessionLabel name={displayName} className="tag-name" />
            {tileState === 'ended' && <span className="tag-state">ended</span>}
          </>
        )}
        <button
          className="tag-remove"
          onPointerDown={event => event.stopPropagation()}
          onContextMenu={event => event.stopPropagation()}
          onClick={(event) => { event.stopPropagation(); onRemove() }}
        >
          ×
        </button>
      </div>

      {contextMenu && (
        <DismissiblePanel onDismiss={() => closeContextMenu()} panelPosition="fixed">
          <div
            ref={contextMenuPosition.ref}
            className="session-context-menu"
            style={contextMenuPosition.style}
            role="menu"
            aria-label={`Session actions for ${displayName}`}
            onClick={event => event.stopPropagation()}
            onKeyDown={(event) => {
              if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
              event.preventDefault()
              const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'))
              if (items.length === 0) return
              const current = items.indexOf(document.activeElement as HTMLButtonElement)
              const next = event.key === 'Home'
                ? 0
                : event.key === 'End'
                  ? items.length - 1
                  : event.key === 'ArrowDown'
                    ? (current + 1) % items.length
                    : (current - 1 + items.length) % items.length
              items[next]?.focus()
            }}
          >
            <button ref={firstActionRef} role="menuitem" className="session-context-item" onClick={() => runContextAction(() => openSendToSession(sessionKey))}>
              <span className="session-context-icon" aria-hidden="true">↗</span>
              Send to session
            </button>
            <button role="menuitem" className="session-context-item" onClick={() => runContextAction(() => pool.terminals.get(sessionName)?.reconnect())}>
              <span className="session-context-icon" aria-hidden="true">↻</span>
              Reconnect frame
            </button>
            <button
              role="menuitem"
              className="session-context-item"
              disabled={!!pinnedSize}
              title={pinnedSize?.detail ?? CLAIM_EXPLANATION}
              onClick={() => runContextAction(() => pool.terminals.get(sessionName)?.claim())}
            >
              <span className="session-context-icon" aria-hidden="true">◎</span>
              <span className="session-context-stack">
                Claim session
                <span className="session-context-reason">{pinnedSize?.detail ?? CLAIM_EXPLANATION}</span>
              </span>
            </button>
            <button
              role="menuitem"
              className="session-context-item"
              disabled={!!pinnedSize}
              title={pinnedSize?.detail}
              onClick={() => runContextAction(() => pool.terminals.get(sessionName)?.fit())}
            >
              <span className="session-context-icon" aria-hidden="true">↔</span>
              <span className="session-context-stack">
                Refit frame
                {pinnedSize && <span className="session-context-reason">{pinnedSize.detail}</span>}
              </span>
            </button>
            <button
              className="session-context-item"
              role="menuitem"
              disabled={!workingDirectory || !onOpenFilesAtPath}
              onClick={() => {
                if (workingDirectory && onOpenFilesAtPath) runContextAction(() => onOpenFilesAtPath(workingDirectory))
              }}
            >
              <span className="session-context-icon" aria-hidden="true">▣</span>
              Open files in working directory
            </button>
            <button role="menuitem" className="session-context-item" onClick={() => runContextAction(startRename, false)}>
              <span className="session-context-icon" aria-hidden="true">✎</span>
              Rename session
            </button>
            <div className="session-context-divider" />
            <button
              className="session-context-item session-context-danger"
              role="menuitem"
              onClick={() => runContextAction(() => { void deleteSession(actualName, resolvedUser) })}
            >
              <span className="session-context-icon" aria-hidden="true">✕</span>
              Kill session
            </button>
          </div>
        </DismissiblePanel>
      )}
    </>
  )
}

/**
 * What claiming does, in the operator's terms, wherever it is offered. tmux
 * draws a window once for every client watching it, so this is also the honest
 * limit of shared viewing: claiming on a phone makes the desktop watching the
 * same session narrow, and no interface can make one grid two sizes.
 */
export const CLAIM_EXPLANATION =
  'Set the tmux window to this device\'s size. Other devices keep watching, at that size.'

interface DetachedTileProps {
  /** Taken over, Lost and Ended are the same shape: no connection, last frame, an action. */
  state: 'takenOver' | 'lost' | 'ended'
  sessionName: string
  restarting: boolean
  onRestore: () => void
  onRestart: () => void
  onRemove: () => void
}

// Each note says only what CHROTE actually knows. "Attached elsewhere" is a
// claim about another client, so it is reserved for a session that has one.
const DETACHED_NOTE: Record<DetachedTileProps['state'], (sessionName: string) => string> = {
  takenOver: name => `${name} was detached by another client. This frame shows its last output.`,
  lost: name => `${name} lost its connection. This frame shows its last output.`,
  ended: name => `${name} ended. This frame shows its last output.`,
}

function DetachedTile({ state, sessionName, restarting, onRestore, onRestart, onRemove }: DetachedTileProps) {
  return (
    <div className="terminal-tile-detached" data-tile-state={state} role="status">
      <span className="terminal-tile-detached-note">{DETACHED_NOTE[state](sessionName)}</span>
      {/* Plain outline buttons: the way back is an ordinary control, not an
          invitation drawn in the accent colour. */}
      <div className="terminal-tile-detached-actions">
        {state === 'ended' ? (
          <>
            <button className="terminal-tile-detached-action" type="button" disabled={restarting} onClick={onRestart}>
              {restarting ? 'Restarting…' : 'Restart'}
            </button>
            <button className="terminal-tile-detached-action" type="button" onClick={onRemove}>Remove</button>
          </>
        ) : (
          // Both dial the same session again, and neither displaces anybody.
          // Claim also takes the tmux sizing seat, so the session comes back at
          // this device's size; Reconnect leaves the size where it is, which is
          // why the tile does that one for the operator when it can.
          <button
            className="terminal-tile-detached-action"
            type="button"
            title={state === 'takenOver' ? CLAIM_EXPLANATION : undefined}
            onClick={onRestore}
          >
            {state === 'takenOver' ? 'Claim' : 'Reconnect'}
          </button>
        )}
      </div>
    </div>
  )
}

interface TerminalWindowProps {
  workspaceId: WorkspaceId
  window: TerminalWindowType
  refitNonce?: number
  style?: React.CSSProperties
  onOpenFilesAtPath?: (path: string) => void
  workspaceActive?: boolean
}

function TerminalWindow({ workspaceId, window: windowConfig, refitNonce = 0, style, onOpenFilesAtPath, workspaceActive = true }: TerminalWindowProps) {
  const windowRef = useRef<HTMLDivElement>(null)
  const { setNodeRef: setDropNodeRef, isOver, active } = useDroppable({
    id: `drop-${workspaceId}-${windowConfig.id}`,
    data: { type: 'window', workspaceId, windowId: windowConfig.id },
  })
  // Drop feedback is fully local: only the hovered window reacts, and the
  // window a tag is dragged from stays calm because dropping there is a no-op.
  const activeDragData = active?.data.current as { type?: string; sourceWorkspaceId?: string; sourceWindowId?: string } | undefined
  const isSessionDrag = activeDragData?.type === 'session' || activeDragData?.type === 'tag'
  const isDragSourceWindow = activeDragData?.type === 'tag'
    && activeDragData.sourceWorkspaceId === workspaceId
    && activeDragData.sourceWindowId === windowConfig.id
  const isDropTarget = isOver && isSessionDrag && !isDragSourceWindow
  const pool = useTerminalPool()

  const {
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    focusedWindowKey,
    setFocusedWindowKey,
    openSendToSession,
    restartSession,
  } = useSession()
  const [restarting, setRestarting] = useState(false)

  // Generate a unique key for this window
  const windowKey = `${workspaceId}-${windowConfig.id}`
  const isFocused = focusedWindowKey === windowKey

  const activeSession = windowConfig.activeSession

  const activeTerminal = activeSession ? pool.terminals.get(activeSession) ?? null : null

  // The one fact joined in from the host, scoped to the users the last poll
  // actually heard from. A list that has not arrived, or one whose poll failed
  // outright, proves nothing about any binding.
  const evidence = useSessionEvidence()
  // A window hidden by the mobile carousel or by an inactive workspace tab is
  // not on screen, whatever its bindings say.
  const windowOnScreen = workspaceActive && style?.display !== 'none'
  const tileStates = useMemo(() => new Map(windowConfig.boundSessions.map(sessionKey => [
    sessionKey,
    tileStateFor({
      sessionKey,
      onScreen: windowOnScreen && sessionKey === windowConfig.activeSession,
      evidence,
      connection: pool.connectionStates.get(sessionKey) ?? 'idle',
    }),
  ])), [windowConfig.boundSessions, windowConfig.activeSession, windowOnScreen, evidence, pool.connectionStates])
  const activeTileState = activeSession ? tileStates.get(activeSession) ?? 'idle' : 'idle'

  // A tile put on screen dials again if its connection was lost rather than
  // ended — the case a chrote-srv restart creates for every open tile at once.
  // One attempt per such moment, no retry and no recurring check; a dial that
  // does not land leaves the tile's own Reconnect control.
  useEffect(() => {
    if (!windowOnScreen || !activeSession) return
    activeTerminal?.redialIfDropped()
  }, [windowOnScreen, activeSession, activeTerminal])

  // The Refit control and the workspace-level refit both land here. Each
  // surface already refits itself on container resize; this is the explicit
  // operator request. Retired by chrote-xl5 once auto-fit is proven.
  useEffect(() => {
    if (!activeTerminal) return
    const frame = requestAnimationFrame(() => activeTerminal.fit())
    return () => cancelAnimationFrame(frame)
  }, [activeTerminal, refitNonce])

  useEffect(() => {
    if (isFocused) activeTerminal?.focus()
  }, [isFocused, activeTerminal])

  // Handle click on this window to focus it for keyboard navigation
  const handleWindowClick = useCallback(() => {
    setFocusedWindowKey(windowKey)
  }, [windowKey, setFocusedWindowKey])

  const handleRemoveSession = (sessionName: string) => {
    removeSessionFromWindow(workspaceId, windowConfig.id, sessionName)
  }

  const handleTagClick = (sessionName: string) => {
    setActiveSession(workspaceId, windowConfig.id, sessionName)
  }

  // Claim attaches again if it has to and takes the sizing seat, so the session
  // comes back at this device's size without detaching whoever else is
  // watching. Reconnect only dials.
  const handleClaim = (sessionKey: string) => pool.terminals.get(sessionKey)?.claim()
  const handleReconnect = (sessionKey: string) => pool.terminals.get(sessionKey)?.reconnect()

  // The pooled terminal is keyed by the binding, which Restart does not change,
  // so the recreated session needs an explicit dial on the existing terminal.
  const handleRestart = async (sessionKey: string) => {
    setRestarting(true)
    try {
      if (await restartSession(workspaceId, windowConfig.id, sessionKey)) {
        pool.terminals.get(sessionKey)?.reconnect()
      }
    } finally {
      setRestarting(false)
    }
  }


  const hasSessions = windowConfig.boundSessions.length > 0

  return (
    <div
      ref={windowRef}
      className={`terminal-window ${isFocused ? 'focused' : ''} ${isDropTarget ? 'drop-target' : ''}`}
      tabIndex={-1}
      style={style}
    >
      <div className="terminal-window-header">
        <div className="session-tags">
          {windowConfig.boundSessions.map(sessionName => (
            <SessionTag
              key={sessionName}
              sessionName={sessionName}
              isActive={sessionName === activeSession}
              workspaceId={workspaceId}
              windowId={windowConfig.id}
              onRemove={() => handleRemoveSession(sessionName)}
              onClick={() => handleTagClick(sessionName)}
              onOpenFilesAtPath={onOpenFilesAtPath}
              workspaceActive={workspaceActive}
              tileState={tileStates.get(sessionName) ?? 'idle'}
            />
          ))}
        </div>

        <div className="window-controls">
          {hasSessions && windowConfig.boundSessions.length > 1 && (
            <>
              <button
                className="cycle-btn"
                onClick={() => cycleSession(workspaceId, windowConfig.id, 'prev')}
                title="Previous session"
              >
                ←
              </button>
              <button
                className="cycle-btn"
                onClick={() => cycleSession(workspaceId, windowConfig.id, 'next')}
                title="Next session"
              >
                →
              </button>
            </>
          )}
          {activeSession && (
            <button
              className="window-send-btn"
              onClick={() => openSendToSession(activeSession)}
              title={`Send to ${getSessionNameFromKey(activeSession)}`}
              aria-label={`Send to session ${getSessionNameFromKey(activeSession)}`}
            >
              <Send size={12} aria-hidden="true" />
            </button>
          )}
          {/* The two detached states say so in the tile itself; only a tile on
              its way up has nothing else to show for it. */}
          {activeSession && activeTileState === 'live' && pool.connectionStates.get(activeSession) !== 'open' && (
            <span className="terminal-loading-state">Connecting…</span>
          )}
        </div>
      </div>

      <div
        ref={setDropNodeRef}
        className={isDetached(activeTileState) ? 'terminal-window-body detached' : 'terminal-window-body'}
        data-tile-state={activeTileState}
        onClick={handleWindowClick}
      >
        {!hasSessions && (
          <EmptyWindow
            workspaceId={workspaceId}
            windowId={windowConfig.id}
            colorIndex={windowConfig.colorIndex}
          />
        )}
        {windowConfig.boundSessions.map(sessionName => (
          <TerminalSurface
            key={sessionName}
            session={pool.terminals.get(sessionName) ?? null}
            hidden={sessionName !== activeSession}
            connect={tileStates.get(sessionName) !== 'ended'}
          />
        ))}
        {activeSession && isDetached(activeTileState) && (
          <DetachedTile
            state={activeTileState}
            sessionName={getSessionNameFromKey(activeSession)}
            restarting={restarting}
            onRestore={() => (activeTileState === 'takenOver' ? handleClaim(activeSession) : handleReconnect(activeSession))}
            onRestart={() => { void handleRestart(activeSession) }}
            onRemove={() => handleRemoveSession(activeSession)}
          />
        )}
        {isDropTarget && (
          <div className="terminal-drop-overlay" style={{ inset: 0, pointerEvents: 'none' }}>
            <span className="drop-hint">Release to add</span>
          </div>
        )}
      </div>
    </div>
  )
}

export default TerminalWindow
