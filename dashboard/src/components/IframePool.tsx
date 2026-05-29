import { createContext, useContext, useState, useEffect, useRef, useCallback, useMemo, ReactNode } from 'react'
import { useSession } from '../context/SessionContext'
import type { WorkspaceId } from '../types'

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
}

const IframePoolContext = createContext<IframePoolContextType | null>(null)

export function useIframePool(): IframePoolContextType {
  const ctx = useContext(IframePoolContext)
  if (!ctx) throw new Error('useIframePool must be used within IframePoolProvider')
  return ctx
}

export function getTerminalUrl(sessionName: string): string {
  return `/terminal/?arg=${encodeURIComponent(sessionName)}&theme=${encodeURIComponent('{"background":"transparent"}')}`
}

export function IframePoolProvider({ children }: { children: ReactNode }) {
  const { workspaces, settings } = useSession()

  // Compute all unique session names that need iframes
  const allSessions = useMemo(() => {
    const sessions = new Set<string>()

    // Current workspaces
    const wsIds: WorkspaceId[] = ['terminal1', 'terminal2']
    wsIds.forEach(wsId => {
      workspaces[wsId].windows.forEach(w => {
        w.boundSessions.forEach(s => {
          if (s && s !== 'INIT-PENDING') sessions.add(s)
        })
      })
    })

    return sessions
  }, [workspaces])

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
      iframe.title = `Terminal - ${sessionName}`
      // Start hidden in pool; will be cleared when claimed into a container
      iframe.style.cssText = 'width:400px;height:300px;border:none;background:transparent;position:absolute;visibility:hidden;'

      iframe.addEventListener('load', () => {
        setLoadedSessions(prev => new Set(prev).add(sessionName))
        applyFontSizeToIframe(iframe, settings.fontSize)
      })

      iframeRefs.current.set(sessionName, iframe)

      // If already claimed, put it in the container with visible styles; otherwise hide in pool
      const claimContainer = claimsRef.current.get(sessionName)
      if (claimContainer) {
        iframe.style.cssText = 'width:100%;height:100%;border:none;background:transparent;'
        claimContainer.appendChild(iframe)
        // Set src since it's being claimed immediately
        if (!connectedRef.current.has(sessionName)) {
          connectedRef.current.add(sessionName)
          iframe.src = getTerminalUrl(sessionName)
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
      iframe.style.cssText = 'border:none;background:transparent;'
      container.appendChild(iframe)

      // Deferred connection: set src only on first claim into a visible container
      if (!connectedRef.current.has(sessionName)) {
        connectedRef.current.add(sessionName)
        iframe.src = getTerminalUrl(sessionName)
      }
    }

    // Return cleanup: move iframe back to pool
    return () => {
      claimsRef.current.delete(sessionName)
      const iframe = iframeRefs.current.get(sessionName)
      const pool = poolContainerRef.current
      if (iframe && pool) {
        // Override CSS positioning: park in hidden pool with explicit inline styles
        iframe.style.cssText = 'width:400px;height:300px;border:none;background:transparent;position:absolute;visibility:hidden;'
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

  const contextValue = useMemo<IframePoolContextType>(() => ({
    claimIframe,
    isLoaded,
    getIframe,
    applyFontSize,
    triggerFit,
    focusIframe,
  }), [claimIframe, isLoaded, getIframe, applyFontSize, triggerFit, focusIframe])

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
