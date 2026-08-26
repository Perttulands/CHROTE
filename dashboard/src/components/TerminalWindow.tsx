import { useState, useEffect, useLayoutEffect, useRef, useCallback } from 'react'
import { useDroppable, useDraggable } from '@dnd-kit/core'
import { Send } from 'lucide-react'
import { useSession } from '../context/SessionContext'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { useIframePool } from './IframePool'
import { WINDOW_COLORS, getSessionKey, getSessionNameFromKey, getSessionUserFromKey, getTerminalUserColor, getTerminalUserInitial } from '../types'
import type { TerminalWindow as TerminalWindowType, WorkspaceId } from '../types'
import DismissiblePanel from './DismissiblePanel'

interface CreateSessionButtonProps {
  workspaceId: WorkspaceId
  windowId: string
  accentColor: string
}

function CreateSessionButton({ workspaceId, windowId, accentColor }: CreateSessionButtonProps) {
  const [creating, setCreating] = useState(false)
  const { createSession } = useSession()

  const handleCreate = async () => {
    setCreating(true)
    try {
      await createSession({
        workspaceId,
        attachTo: { workspaceId, windowId },
      })
    } finally {
      setCreating(false)
    }
  }

  return (
    <button
      className="create-session-btn"
      onClick={handleCreate}
      disabled={creating}
      style={{ '--btn-accent': accentColor } as React.CSSProperties}
      title="Create new session"
    >
      <span className="create-session-icon">{creating ? '...' : '+'}</span>
      <span className="create-session-label">New Session</span>
    </button>
  )
}

interface SessionTagProps {
  sessionName: string
  isActive: boolean
  workspaceId: WorkspaceId
  windowId: string
  onRemove: () => void
  onClick: () => void
  onOpenFilesAtPath?: (path: string) => void
  workspaceActive: boolean
  contextActionsEnabled: boolean
}

function SessionTag({ sessionName, isActive, workspaceId, windowId, onRemove, onClick, onOpenFilesAtPath, workspaceActive, contextActionsEnabled }: SessionTagProps) {
  const { sessions, settings, deleteSession, renameSession, openSendToSession } = useSession()
  const pool = useIframePool()
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null)
  const [isRenaming, setIsRenaming] = useState(false)
  const [renameValue, setRenameValue] = useState('')
  const contextMenuPosition = useViewportMenuPosition<HTMLDivElement>(contextMenu, {
    estimatedSize: { width: 240, height: 270 },
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
    if (contextActionsEnabled && (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10'))) {
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
    if (!workspaceActive || !contextActionsEnabled) closeContextMenu(false)
  }, [closeContextMenu, contextActionsEnabled, workspaceActive])

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
        style={style}
        title={dragLabel}
        {...attributes}
        {...listeners}
        aria-label={`Session ${displayName}`}
        onClick={handleClick}
        onKeyDown={handleTagKeyDown}
        onContextMenu={(event) => {
          if (!contextActionsEnabled || (event.target as HTMLElement).closest('.tag-remove')) return
          event.preventDefault()
          event.stopPropagation()
          openContextMenu(event.clientX, event.clientY)
        }}
      >
        {resolvedUser && (
          <span
            className="session-user-badge"
            style={{ backgroundColor: getTerminalUserColor(settings, resolvedUser) }}
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
        ) : <span className="tag-name">{displayName}</span>}
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
            <button role="menuitem" className="session-context-item" onClick={() => runContextAction(() => pool.reconnectIframe(sessionName))}>
              <span className="session-context-icon" aria-hidden="true">↻</span>
              Reconnect frame
            </button>
            <button role="menuitem" className="session-context-item" onClick={() => runContextAction(() => pool.triggerFit(sessionName))}>
              <span className="session-context-icon" aria-hidden="true">↔</span>
              Refit frame
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

interface TerminalWindowProps {
  workspaceId: WorkspaceId
  window: TerminalWindowType
  refitNonce?: number
  style?: React.CSSProperties
  onOpenFilesAtPath?: (path: string) => void
  workspaceActive?: boolean
}

function TerminalWindow({ workspaceId, window: windowConfig, refitNonce = 0, style, onOpenFilesAtPath, workspaceActive = true }: TerminalWindowProps) {
  const bodyRef = useRef<HTMLDivElement | null>(null)
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
  const setBodyRef = useCallback((node: HTMLDivElement | null) => {
    bodyRef.current = node
    setDropNodeRef(node)
  }, [setDropNodeRef])

  const pool = useIframePool()

  const {
    removeSessionFromWindow,
    setActiveSession,
    cycleSession,
    focusedWindowKey,
    setFocusedWindowKey,
    openSendToSession,
  } = useSession()

  // Generate a unique key for this window
  const windowKey = `${workspaceId}-${windowConfig.id}`
  const isFocused = focusedWindowKey === windowKey

  const activeSession = windowConfig.activeSession

  // Check if active session is loaded via pool
  const activeSessionLoaded = activeSession ? pool.loadedSessions.has(activeSession) : false

  // Claim/release iframes from the pool as boundSessions change.
  // Layout effect, not passive: on unmount (e.g. window-count shrink) a
  // passive cleanup runs AFTER React removes this window's DOM subtree, so
  // the iframe is already disconnected and can only be re-inserted into the
  // pool — which reloads its document and kills the ttyd WebSocket. Layout
  // cleanups run during the mutation phase, before detach, letting the pool
  // move the still-connected iframe with state-preserving moveBefore.
  useLayoutEffect(() => {
    const body = bodyRef.current
    if (!body) return

    const cleanups: (() => void)[] = []
    windowConfig.boundSessions.forEach(sessionName => {
      if (sessionName && sessionName !== 'INIT-PENDING') {
        const cleanup = pool.claimIframe(sessionName, body)
        cleanups.push(cleanup)
      }
    })

    return () => {
      cleanups.forEach(fn => fn())
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.claimIframe is a stable ref
  }, [windowConfig.boundSessions])

  // Manage visibility of claimed iframes based on active session.
  // CSS (.terminal-window-body iframe) handles position/size via position:absolute + inset.
  // This effect only toggles display to show/hide the correct iframe.
  useEffect(() => {
    windowConfig.boundSessions.forEach(sessionName => {
      const iframe = pool.getIframe(sessionName)
      if (!iframe) return
      const isActive = sessionName === activeSession
      iframe.style.display = isActive ? 'block' : 'none'
    })

  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool functions are stable refs
  }, [activeSession, windowConfig.boundSessions])

  useEffect(() => {
    if (!activeSession || !activeSessionLoaded) return
    const rafId = requestAnimationFrame(() => pool.triggerFit(activeSession))
    return () => cancelAnimationFrame(rafId)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.triggerFit is a stable ref
  }, [activeSession, activeSessionLoaded, refitNonce])

  // Focus iframe when this window is focused
  useEffect(() => {
    if (isFocused && activeSession) {
      pool.focusIframe(activeSession)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.focusIframe is a stable ref
  }, [isFocused, activeSession])

  // Handle click on this window to focus it for keyboard navigation
  const handleWindowClick = useCallback(() => {
    setFocusedWindowKey(windowKey)
  }, [windowKey, setFocusedWindowKey])


  // Store activeSession in ref for ResizeObserver callback
  const activeSessionRef = useRef(activeSession)
  useEffect(() => { activeSessionRef.current = activeSession }, [activeSession])

  // ResizeObserver to trigger fit() when container size changes
  useEffect(() => {
    const body = bodyRef.current
    if (!body) return

    let timeoutId: ReturnType<typeof setTimeout>
    const observer = new ResizeObserver(() => {
      clearTimeout(timeoutId)
      timeoutId = setTimeout(() => {
        if (activeSessionRef.current) pool.triggerFit(activeSessionRef.current)
      }, 100)
    })

    observer.observe(body)
    return () => {
      clearTimeout(timeoutId)
      observer.disconnect()
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- pool.triggerFit is a stable ref
  }, [])

  const colorTheme = WINDOW_COLORS[windowConfig.colorIndex % WINDOW_COLORS.length]

  const handleRemoveSession = (sessionName: string) => {
    removeSessionFromWindow(workspaceId, windowConfig.id, sessionName)
  }

  const handleTagClick = (sessionName: string) => {
    setActiveSession(workspaceId, windowConfig.id, sessionName)
  }


  const hasSessions = windowConfig.boundSessions.length > 0
  const sendableSession = activeSession && activeSession !== 'INIT-PENDING' ? activeSession : null

  return (
    <div
      ref={windowRef}
      className={`terminal-window ${isFocused ? 'focused' : ''} ${isDropTarget ? 'drop-target' : ''}`}
      tabIndex={-1}
      style={{
        '--window-accent': colorTheme.accent,
        '--window-bg': colorTheme.bg,
        '--window-border': colorTheme.border,
        ...style,
      } as React.CSSProperties}
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
              contextActionsEnabled={sessionName !== 'INIT-PENDING'}
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
          {sendableSession && (
            <button
              className="window-send-btn"
              onClick={() => openSendToSession(sendableSession)}
              title={`Send to ${getSessionNameFromKey(sendableSession)}`}
              aria-label={`Send to session ${getSessionNameFromKey(sendableSession)}`}
            >
              <Send size={12} aria-hidden="true" />
            </button>
          )}
          {activeSession && !activeSessionLoaded && (
            <span className="terminal-loading-state">Loading terminal…</span>
          )}
        </div>
      </div>

      <div ref={setBodyRef} className="terminal-window-body" onClick={handleWindowClick}>
        {activeSession === 'INIT-PENDING' ? (
          <div style={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100%',
            color: colorTheme.accent
          }}>
            <span>Initializing Session...</span>
          </div>
        ) : !hasSessions ? (
          <div className="empty-window-state">
            <CreateSessionButton workspaceId={workspaceId} windowId={windowConfig.id} accentColor={colorTheme.accent} />
            <span className="empty-window-hint">or drag a session here</span>
          </div>
        ) : null}
        {/* Iframes are injected here by the IframePool via DOM manipulation */}
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
