import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'
import type { LaunchUser } from '../types'
import { createTerminalSession, type TerminalConnectionState, type TerminalSession } from '../terminal/terminalSession'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import { useTheme } from '../theme/ThemeContext'
import { TERMINAL_FONT_FAMILY } from '../theme/theme'

interface TerminalPoolContextType {
  /** One terminal per bound session, outliving the tile that shows it. */
  terminals: ReadonlyMap<string, TerminalSession>
  connectionStates: ReadonlyMap<string, TerminalConnectionState>
}

const TerminalPoolContext = createContext<TerminalPoolContextType | null>(null)

export function useTerminalPool(): TerminalPoolContextType {
  const ctx = useContext(TerminalPoolContext)
  if (!ctx) throw new Error('useTerminalPool must be used within TerminalPoolProvider')
  return ctx
}

export function TerminalPoolProvider({ children }: { children: ReactNode }) {
  const { workspaces, settings, sessions } = useSession()
  const theme = useTheme()

  const sessionUsers = useMemo(() => {
    const users = new Map<string, LaunchUser>()
    const sessionsByKey = new Map(sessions.map(session => [getSessionKey(session.name, session.unixUser), session]))
    const sessionsByName = new Map<string, typeof sessions>()
    sessions.forEach(session => {
      sessionsByName.set(session.name, [...(sessionsByName.get(session.name) ?? []), session])
    })
    // Every workspace in state contributes bindings, whether or not its tab is
    // currently reachable — a visibility-derived list here would turn the
    // reconcile effect below into a prune of hidden workspaces.
    Object.values(workspaces).forEach(workspace => {
      workspace.windows.forEach(w => {
        w.boundSessions.forEach(sessionKey => {
          if (!sessionKey || users.has(sessionKey)) return
          const exactSession = sessionsByKey.get(sessionKey)
          const legacyMatches = sessionsByName.get(sessionKey) ?? []
          const unixUser = exactSession?.unixUser
            ?? (legacyMatches.length === 1 ? legacyMatches[0].unixUser : undefined)
            ?? getSessionUserFromKey(sessionKey)
          users.set(sessionKey, unixUser ?? '')
        })
      })
    })
    return users
  }, [workspaces, sessions])

  const settingsRef = useRef(settings)
  settingsRef.current = settings
  // Read at creation only. The reconcile effect below is keyed on the bindings
  // alone, so a theme that lands later reaches live terminals through
  // applyAppearance rather than by rebuilding the pool.
  const themeRef = useRef(theme)
  themeRef.current = theme
  const terminalsRef = useRef<Map<string, TerminalSession>>(new Map())
  const [terminals, setTerminals] = useState<ReadonlyMap<string, TerminalSession>>(terminalsRef.current)
  const [connectionStates, setConnectionStates] = useState<ReadonlyMap<string, TerminalConnectionState>>(new Map())

  // Reconcile the pool with the bindings. A terminal is created unconnected;
  // it dials only when a tile first attaches it.
  useEffect(() => {
    const pool = terminalsRef.current
    let changed = false
    Array.from(pool.keys()).forEach(sessionKey => {
      if (sessionUsers.has(sessionKey)) return
      pool.get(sessionKey)?.dispose()
      pool.delete(sessionKey)
      changed = true
    })
    sessionUsers.forEach((unixUser, sessionKey) => {
      if (pool.has(sessionKey)) return
      pool.set(sessionKey, createTerminalSession({
        url: terminalSocketUrl(getSessionNameFromKey(sessionKey), unixUser, 'tile'),
        fontSize: settingsRef.current.fontSize,
        hideScrollbar: settingsRef.current.hideScrollbar,
        terminalTheme: themeRef.current.terminal,
        fontFamily: TERMINAL_FONT_FAMILY,
        onStateChange: state => setConnectionStates(prev => new Map(prev).set(sessionKey, state)),
      }))
      changed = true
    })
    if (!changed) return
    setTerminals(new Map(pool))
    setConnectionStates(prev => {
      const next = new Map(prev)
      Array.from(next.keys()).forEach(key => { if (!pool.has(key)) next.delete(key) })
      return next
    })
  }, [sessionUsers])

  useEffect(() => () => {
    terminalsRef.current.forEach(terminal => terminal.dispose())
    terminalsRef.current.clear()
  }, [])

  useEffect(() => {
    terminals.forEach(terminal => terminal.setFontSize(settings.fontSize))
  }, [settings.fontSize, terminals])

  useEffect(() => {
    terminals.forEach(terminal => terminal.setScrollbarHidden(settings.hideScrollbar))
  }, [settings.hideScrollbar, terminals])

  useEffect(() => {
    terminals.forEach(terminal => terminal.applyAppearance(theme.terminal, TERMINAL_FONT_FAMILY))
  }, [theme, terminals])

  // A tab hidden while its grid changed comes back with stale cell metrics, and
  // one hidden across a chrote-srv restart comes back with no connections at
  // all. Both are answered when the operator returns, because that is when he
  // is looking. fit() and redialIfDropped() ignore terminals that are detached
  // or not on screen, and neither retries.
  useEffect(() => {
    const wake = () => {
      if (document.visibilityState !== 'visible') return
      terminals.forEach(terminal => {
        terminal.redialIfDropped()
        terminal.fit()
      })
    }
    document.addEventListener('visibilitychange', wake)
    return () => document.removeEventListener('visibilitychange', wake)
  }, [terminals])

  const contextValue = useMemo<TerminalPoolContextType>(
    () => ({ terminals, connectionStates }),
    [terminals, connectionStates],
  )

  return (
    <TerminalPoolContext.Provider value={contextValue}>
      {children}
    </TerminalPoolContext.Provider>
  )
}
