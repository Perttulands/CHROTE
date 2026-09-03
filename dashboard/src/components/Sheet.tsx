/**
 * A sheet docks to an edge of the workspace and takes a share of it.
 *
 * It is not a modal: there is no backdrop, the surfaces it does not cover stay
 * readable and usable, and a click outside is a click outside — it does what it
 * would have done anyway. Escape closes, and so does the header's own control.
 *
 * The sheet draws one hairline on the edge it faces and nothing on the three
 * edges it shares with the viewport, so it reads as a panel of the workspace
 * rather than as a window floating over it.
 */

import { useEffect } from 'react'
import type { ReactNode } from 'react'
import './Sheet.css'

export type SheetEdge = 'left' | 'right' | 'bottom'

export interface SheetProps {
  open: boolean
  edge: SheetEdge
  /** How much of the workspace the sheet takes, as a CSS length. */
  extent: string
  label: string
  onClose: () => void
  /** The one-line header: what this is, and what can be done with it. */
  header?: ReactNode
  children: ReactNode
}

export default function Sheet({ open, edge, extent, label, onClose, header, children }: SheetProps) {
  useEffect(() => {
    if (!open) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      onClose()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [open, onClose])

  if (!open) return null

  const style = edge === 'bottom' ? { height: extent } : { width: extent }

  return (
    <aside className={`sheet sheet-${edge}`} style={style} role="dialog" aria-label={label}>
      {header !== undefined && <div className="sheet-header">{header}</div>}
      <div className="sheet-body">{children}</div>
    </aside>
  )
}
