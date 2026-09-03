/**
 * One Bead, as every view of the tab draws it: its state as a glyph, its type,
 * its id and its title. Clicking it opens the card, which is the only thing a
 * row does, because reading a Bead is the whole point of the tab.
 */

import type { ReactNode } from 'react'
import { openBeadCard } from '../../beads/beadCard'
import { beadGlyph, beadStatusLabel } from '../../beads/beadStatus'
import type { WorkRow } from '../../beads/beadsTree'

export interface BeadRowProps {
  row: WorkRow
  /** How deep under an epic the row sits, in the map. */
  depth?: number
  /** An epic's rows fold; the glyph is the control that folds them. */
  fold?: { expanded: boolean; onToggle: () => void }
  /** What the view puts at the right of the row, such as an age and a Send. */
  trailing?: ReactNode
}

export default function BeadRow({ row, depth = 0, fold, trailing }: BeadRowProps) {
  const glyph = beadGlyph(row.status, row.blocked)
  const state = beadStatusLabel(row.status, row.blocked)

  return (
    <div className="bead-row" data-ui="beads.row" style={{ paddingLeft: `${12 + depth * 22}px` }}>
      {fold ? (
        <button
          type="button"
          className="bead-row-glyph"
          aria-expanded={fold.expanded}
          aria-label={`${fold.expanded ? 'Collapse' : 'Expand'} ${row.id}`}
          onClick={fold.onToggle}
        >
          {glyph}
        </button>
      ) : (
        <span className="bead-row-glyph" title={state}>{glyph}</span>
      )}
      <button type="button" className="bead-row-open" onClick={() => openBeadCard(row.id, row.projectPath)}>
        <span className="bead-row-type">{row.type || 'task'}</span>
        <span className="bead-row-id">{row.id}</span>
        <span className="bead-row-title">{row.title}</span>
      </button>
      {trailing}
    </div>
  )
}
