import { createContext, useContext, useState, useEffect, useRef, useCallback, useMemo, ReactNode } from 'react'
import { useSession } from '../context/SessionContext'
import { TERMINAL_WORKSPACE_IDS, getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'
import type { LaunchUser } from '../types'

interface IframePoolContextType {
  /** Claim an iframe into a container element. Returns cleanup function. */
  claimIframe: (sessionName: string, container: HTMLElement) => (() => void)
  /** Check if a session's iframe has finished loading */
  isLoaded: (sessionName: string) => boolean
  /** Get the iframe element for a session (for font size / fit operations) */
  getIframe: (sessionName: string) => HTMLIFrameElement | null
  /** Apply font size to a specific session's iframe */
  applyFontSize: (sessionName: string, fontSize: number) => void
  /** Trigger xterm fit() on a session's iframe */
  triggerFit: (sessionName: string) => void
  /** Focus a session's iframe */
  focusIframe: (sessionName: string) => void
  /** Reconnect a session's iframe without changing the window/session binding */
  reconnectIframe: (sessionName: string) => void
}

const IframePoolContext = createContext<IframePoolContextType | null>(null)

export function useIframePool(): IframePoolContextType {
  const ctx = useContext(IframePoolContext)
  if (!ctx) throw new Error('useIframePool must be used within IframePoolProvider')
  return ctx
}

function getTerminalUrl(sessionName: string, unixUser: LaunchUser): string {
  const userArg = unixUser.trim() ? `&arg=${encodeURIComponent(unixUser)}` : ''
  return `/terminal/?arg=${encodeURIComponent(sessionName)}${userArg}`
}

const TERMINAL_IFRAME_BACKGROUND = '#0a0a0a'

function applyClaimedIframeStyle(iframe: HTMLIFrameElement) {
  iframe.style.cssText = ''
  iframe.style.width = '100%'
  iframe.style.height = '100%'
  iframe.style.border = 'none'
  iframe.style.backgroundColor = TERMINAL_IFRAME_BACKGROUND
  iframe.style.overflow = 'hidden'
}

function applyParkedIframeStyle(iframe: HTMLIFrameElement) {
  applyClaimedIframeStyle(iframe)
  iframe.style.width = '400px'
  iframe.style.height = '300px'
  iframe.style.position = 'absolute'
  iframe.style.visibility = 'hidden'
}

export function IframePoolProvider({ children }: { children: ReactNode }) {
  const { workspaces, settings, sessions } = useSession()

  const sessionUsers = useMemo(() => {
    const users = new Map<string, LaunchUser>()
    const nameCounts = new Map<string, number>()
    sessions.forEach(session => {
      nameCounts.set(session.name, (nameCounts.get(session.name) ?? 0) + 1)
    })
    sessions.forEach(session => {
      const sessionKey = getSessionKey(session.name, session.unixUser)
      users.set(sessionKey, session.unixUser ?? '')
      if (nameCounts.get(session.name) === 1) {
        users.set(session.name, session.unixUser ?? '') // backward compatibility only when unambiguous
      }
    })
    TERMINAL_WORKSPACE_IDS.forEach(wsId => {
      workspaces[wsId].windows.forEach(w => {
        w.boundSessions.forEach(sessionKey => {
          if (!sessionKey || sessionKey === 'INIT-PENDING' || users.has(sessionKey)) return
          const keyUser = getSessionUserFromKey(sessionKey)
          if (keyUser) users.set(sessionKey, keyUser)
        })
      })
    })
    return users
  }, [workspaces, sessions])
  const sessionUsersRef = useRef(sessionUsers)
  sessionUsersRef.current = sessionUsers

  // Compute all unique session names that need iframes
  const allSessions = useMemo(() => new Set(sessionUsers.keys()), [sessionUsers])

  // Refs for iframe elements and state
  const iframeRefs = useRef<Map<string, HTMLIFrameElement>>(new Map())
  const [loadedSessions, setLoadedSessions] = useState<Set<string>>(new Set())
  const poolContainerRef = useRef<HTMLDivElement>(null)

  // Track which sessions are claimed and where
  const claimsRef = useRef<Map<string, HTMLElement>>(new Map())

  // Track which sessions have had their src set (deferred connection)
  const connectedRef = useRef<Set<string>>(new Set())

  // Clean up iframes for sessions no longer needed
  useEffect(() => {
    const toRemove: string[] = []
    iframeRefs.current.forEach((_, sessionName) => {
      if (!allSessions.has(sessionName)) {
        toRemove.push(sessionName)
      }
    })
    toRemove.forEach(sessionName => {
      const iframe = iframeRefs.current.get(sessionName)
      if (iframe && iframe.parentNode) {
        iframe.parentNode.removeChild(iframe)
      }
      iframeRefs.current.delete(sessionName)
      claimsRef.current.delete(sessionName)
      connectedRef.current.delete(sessionName)
    })
    if (toRemove.length > 0) {
      setLoadedSessions(prev => {
        const next = new Set(prev)
        toRemove.forEach(s => next.delete(s))
        return next
      })
    }
  }, [allSessions])

  // Create iframe elements for new sessions (src deferred until first claim)
  useEffect(() => {
    const pool = poolContainerRef.current
    if (!pool) return

    allSessions.forEach(sessionName => {
      if (iframeRefs.current.has(sessionName)) return

      const iframe = document.createElement('iframe')
      // Deferred connection: do NOT set src here. It will be set on first claim.
      iframe.allow = 'clipboard-read; clipboard-write'
      iframe.title = `Terminal - ${getSessionNameFromKey(sessionName)}`
      iframe.scrolling = 'no'
      iframe.setAttribute('scrolling', 'no')
      // Start hidden in pool; will be cleared when claimed into a container
      applyParkedIframeStyle(iframe)

      iframe.addEventListener('load', () => {
        setLoadedSessions(prev => new Set(prev).add(sessionName))
        applyFontSizeToIframe(iframe, settings.fontSize)
      })

      iframeRefs.current.set(sessionName, iframe)

      // If already claimed, put it in the container with visible styles; otherwise hide in pool
      const claimContainer = claimsRef.current.get(sessionName)
      if (claimContainer) {
        applyClaimedIframeStyle(iframe)
        claimContainer.appendChild(iframe)
        // Set src since it's being claimed immediately
        if (!connectedRef.current.has(sessionName)) {
          connectedRef.current.add(sessionName)
          iframe.src = getTerminalUrl(getSessionNameFromKey(sessionName), sessionUsersRef.current.get(sessionName) ?? getSessionUserFromKey(sessionName))
        }
      } else {
        pool.appendChild(iframe)
      }
    })
  }, [allSessions, settings.fontSize])

  // Apply font size to all loaded iframes when setting changes
  useEffect(() => {
    loadedSessions.forEach(sessionName => {
      const iframe = iframeRefs.current.get(sessionName)
      if (iframe) applyFontSizeToIframe(iframe, settings.fontSize)
    })
  }, [settings.fontSize, loadedSessions])

  const applyFontSizeToIframe = useCallback((iframe: HTMLIFrameElement, fontSize: number) => {
    if (!iframe?.contentWindow) return
    let attempts = 0
    const tryApply = () => {
      try {
        const iframeWindow = iframe.contentWindow as Window & { term?: { options: { fontSize: number } } }
        if (iframeWindow?.term) {
          iframeWindow.term.options.fontSize = fontSize
          return
        }
      } catch { /* cross-origin or not ready */ }
      attempts++
      if (attempts < 20) setTimeout(tryApply, 50)
    }
    tryApply()
  }, [])

  const claimIframe = useCallback((sessionName: string, container: HTMLElement): (() => void) => {
    claimsRef.current.set(sessionName, container)

    const iframe = iframeRefs.current.get(sessionName)
    if (iframe) {
      // Move iframe from pool into the claiming container.
      // Clear pool-specific inline styles; CSS (.terminal-window-body iframe)
      // handles positioning via position:absolute + inset.
      applyClaimedIframeStyle(iframe)
      container.appendChild(iframe)

      // Deferred connection: set src only on first claim into a visible container
      if (!connectedRef.current.has(sessionName)) {
        connectedRef.current.add(sessionName)
        iframe.src = getTerminalUrl(getSessionNameFromKey(sessionName), sessionUsersRef.current.get(sessionName) ?? getSessionUserFromKey(sessionName))
      }
    }

    // Return cleanup: move iframe back to pool
    return () => {
      claimsRef.current.delete(sessionName)
      const iframe = iframeRefs.current.get(sessionName)
      const pool = poolContainerRef.current
      if (iframe && pool) {
        // Override CSS positioning: park in hidden pool with explicit inline styles
        applyParkedIframeStyle(iframe)
        pool.appendChild(iframe)
      }
    }
  }, [])

  // Use ref for isLoaded to keep a stable function identity.
  // This prevents the context value from changing every time an iframe loads,
  // which would cause all consumers to re-render unnecessarily.
  const loadedSessionsRef = useRef(loadedSessions)
  useEffect(() => { loadedSessionsRef.current = loadedSessions }, [loadedSessions])
  const isLoaded = useCallback((sessionName: string) => loadedSessionsRef.current.has(sessionName), [])

  const getIframe = useCallback((sessionName: string) => iframeRefs.current.get(sessionName) ?? null, [])

  const applyFontSize = useCallback((sessionName: string, fontSize: number) => {
    const iframe = iframeRefs.current.get(sessionName)
    if (iframe) applyFontSizeToIframe(iframe, fontSize)
  }, [applyFontSizeToIframe])

  const triggerFit = useCallback((sessionName: string) => {
    try {
      const iframe = iframeRefs.current.get(sessionName)
      if (!iframe?.contentWindow) return
      // Only fit iframes that are claimed into a visible container, not parked in pool
      if (!claimsRef.current.has(sessionName)) return
      // Don't trigger fit if iframe has no meaningful dimensions
      if (iframe.offsetWidth < 10 || iframe.offsetHeight < 10) return
      iframe.contentWindow.dispatchEvent(new Event('resize'))
    } catch { /* cross-origin */ }
  }, [])

  const focusIframe = useCallback((sessionName: string) => {
    try {
      const iframe = iframeRefs.current.get(sessionName)
      if (iframe?.contentWindow) {
        iframe.focus()
        iframe.contentWindow.focus()
      }
    } catch { /* cross-origin */ }
  }, [])

  const reconnectIframe = useCallback((sessionName: string) => {
    const iframe = iframeRefs.current.get(sessionName)
    if (!iframe) return
    connectedRef.current.delete(sessionName)
    setLoadedSessions(prev => {
      const next = new Set(prev)
      next.delete(sessionName)
      return next
    })
    connectedRef.current.add(sessionName)
    const url = getTerminalUrl(getSessionNameFromKey(sessionName), sessionUsersRef.current.get(sessionName) ?? getSessionUserFromKey(sessionName))
    iframe.src = `${url}&reconnect=${Date.now()}`
  }, [])

  const contextValue = useMemo<IframePoolContextType>(() => ({
    claimIframe,
    isLoaded,
    getIframe,
    applyFontSize,
    triggerFit,
    focusIframe,
    reconnectIframe,
  }), [claimIframe, isLoaded, getIframe, applyFontSize, triggerFit, focusIframe, reconnectIframe])

  return (
    <IframePoolContext.Provider value={contextValue}>
      {/* Pool container for released iframes that still have active ttyd connections.
          Deferred connection handles initial creation (no src until first claim).
          The 400x300 size prevents xterm from collapsing to 2x1 while parked. */}
      <div
        ref={poolContainerRef}
        style={{
          position: 'fixed',
          left: '-9999px',
          top: '-9999px',
          width: '400px',
          height: '300px',
          overflow: 'hidden',
          pointerEvents: 'none',
          visibility: 'hidden',
        }}
      />
      {children}
    </IframePoolContext.Provider>
  )
}
