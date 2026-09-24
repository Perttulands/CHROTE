import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createTerminalSession, type FixedGrid, type FixedGridBox, type TerminalConnectionState, type TerminalSession } from '../terminal/terminalSession'
import { useStatus } from '../context/StatusContext'
import { useTheme } from '../theme/ThemeContext'
import { TERMINAL_FONT_FAMILY } from '../theme/theme'

interface TerminalSurfaceProps {
  /** The terminal to show here. Pooled for tiles, owned for peek. */
  session: TerminalSession | null
  /** Kept mounted and connected, but not on screen. */
  hidden?: boolean
  /** False for an ended tile: show the last frame without dialling again. */
  connect?: boolean
}

const FIT_DEBOUNCE_MS = 100

/**
 * The one place a terminal is put on screen. Tiles and peek both render this;
 * only the ownership of the session differs.
 */
function TerminalSurface({ session, hidden = false, connect = true }: TerminalSurfaceProps) {
  const hostRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const host = hostRef.current
    if (!host || !session) return
    session.attach(host, { connect })
    return () => session.detach()
  }, [session, connect])

  useEffect(() => {
    const host = hostRef.current
    if (!host || !session || hidden) return
    session.fit()
    let timer: ReturnType<typeof setTimeout>
    const observer = new ResizeObserver(() => {
      clearTimeout(timer)
      timer = setTimeout(() => session.fit(), FIT_DEBOUNCE_MS)
    })
    observer.observe(host)
    return () => {
      clearTimeout(timer)
      observer.disconnect()
    }
  }, [session, hidden])

  return (
    <div
      ref={hostRef}
      className="terminal-surface-host"
      data-testid="terminal-surface"
      style={hidden ? { display: 'none' } : undefined}
    />
  )
}

/**
 * A terminal owned by the calling component for as long as `url` holds, then
 * disposed. Peek uses this; tiles take theirs from the pool so a released tile
 * keeps its connection. A fixed grid is held from the terminal's first
 * handshake on, and follows the caller's grid and room after that;
 * `onFixedGridFit` is told the box of every fit of it.
 */
export function useTerminalSession(
  url: string | null,
  fontSize: number,
  hideScrollbar: boolean,
  fixedGrid: FixedGrid | null = null,
  onFixedGridFit?: (box: FixedGridBox) => void,
) {
  const [session, setSession] = useState<TerminalSession | null>(null)
  const [connectionState, setConnectionState] = useState<TerminalConnectionState>('idle')
  const theme = useTheme()
  const { announce } = useStatus()
  const initialAppearance = useRef({ fontSize, hideScrollbar, theme, fixedGrid })
  initialAppearance.current = { fontSize, hideScrollbar, theme, fixedGrid }
  // Read through refs so the terminal's life is keyed on the url alone.
  const announceRef = useRef(announce)
  announceRef.current = announce
  const onFixedGridFitRef = useRef(onFixedGridFit)
  onFixedGridFitRef.current = onFixedGridFit

  useEffect(() => {
    if (!url) {
      setSession(null)
      setConnectionState('idle')
      return
    }
    const created = createTerminalSession({
      url,
      fontSize: initialAppearance.current.fontSize,
      hideScrollbar: initialAppearance.current.hideScrollbar,
      terminalTheme: initialAppearance.current.theme.terminal,
      fontFamily: TERMINAL_FONT_FAMILY,
      fixedGrid: initialAppearance.current.fixedGrid,
      onFixedGridFit: box => onFixedGridFitRef.current?.(box),
      onStateChange: setConnectionState,
      announce: (message, severity) => announceRef.current(message, severity),
    })
    setSession(created)
    return () => {
      setSession(null)
      created.dispose()
    }
  }, [url])

  useEffect(() => { session?.applyAppearance(theme.terminal, TERMINAL_FONT_FAMILY) }, [session, theme])
  useEffect(() => { session?.setFontSize(fontSize) }, [session, fontSize])
  useEffect(() => { session?.setScrollbarHidden(hideScrollbar) }, [session, hideScrollbar])
  // Keyed on the numbers, so a caller handing a fresh object with the same
  // grid on every render changes nothing; and before paint, so a new room is
  // never drawn with the last room's font.
  const gridCols = fixedGrid?.cols
  const gridRows = fixedGrid?.rows
  const roomWidth = fixedGrid?.room.width
  const roomHeight = fixedGrid?.room.height
  useLayoutEffect(() => {
    session?.setFixedGrid(gridCols !== undefined && gridRows !== undefined && roomWidth !== undefined && roomHeight !== undefined
      ? { cols: gridCols, rows: gridRows, room: { width: roomWidth, height: roomHeight } }
      : null)
  }, [session, gridCols, gridRows, roomWidth, roomHeight])

  return { session, connectionState }
}

export default TerminalSurface
