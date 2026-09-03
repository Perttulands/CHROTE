/**
 * A sheet docks to an edge of the workspace and takes a share of it.
 *
 * It is not a modal: there is no backdrop, and the surfaces it does not cover
 * stay readable and usable. Dismissal belongs to the owner in keys/dismiss: a
 * work sheet, which is what a sheet is unless told otherwise, closes on Escape
 * and on its header's own control, and a press outside it is an ordinary
 * press; a glance closes on a press outside too, and that press is consumed.
 *
 * The sheet draws one hairline on the edge it faces and nothing on the three
 * edges it shares with the viewport, so it reads as a panel of the workspace
 * rather than as a window floating over it.
 */

import { useRef } from 'react'
import type { ReactNode } from 'react'
import { useSurface, type SurfaceKind } from '../keys/dismiss'
import './Sheet.css'

export type SheetEdge = 'left' | 'right' | 'bottom'

export interface SheetProps {
  open: boolean
  edge: SheetEdge
  /** How much of the workspace the sheet takes, as a CSS length. */
  extent: string
  label: string
  onClose: () => void
  /** Whether a press outside closes the sheet. A sheet is work unless said otherwise. */
  kind?: SurfaceKind
  /** The one-line header: what this is, and what can be done with it. */
  header?: ReactNode
  children: ReactNode
}

export default function Sheet({ open, edge, extent, label, onClose, kind = 'work', header, children }: SheetProps) {
  const sheetRef = useRef<HTMLElement>(null)
  useSurface({ open, kind, onClose, ref: sheetRef })

  if (!open) return null

  const style = edge === 'bottom' ? { height: extent } : { width: extent }

  return (
    <aside ref={sheetRef} className={`sheet sheet-${edge}`} style={style} role="dialog" aria-label={label}>
      {header !== undefined && <div className="sheet-header">{header}</div>}
      <div className="sheet-body">{children}</div>
    </aside>
  )
}
