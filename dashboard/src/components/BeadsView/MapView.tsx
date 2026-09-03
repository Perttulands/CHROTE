/**
 * The map: all open work, hung under the epics it belongs to.
 *
 * An expanded epic shows its acceptance criteria before its children, because
 * reviewing an epic is reviewing its definition of done. A blocked row says
 * what it is waiting for, beneath itself, so the map explains its own shape.
 */

import { useState } from 'react'
import BeadRow from './BeadRow'
import { openBeadCard } from '../../beads/beadCard'
import { beadRowKey, type BeadTreeNode } from '../../beads/beadsTree'

interface MapViewProps {
  roots: BeadTreeNode[]
  /** Search keeps a branch for its match, so a filtered map opens itself. */
  expandAll: boolean
}

/**
 * The columns a row draws in, so what explains a row lines up under it: the
 * row's 12px inset, the 36px fold slot, the 14px glyph, then the type and the
 * id at their fixed widths, with the row's 8px gaps between.
 */
const ROW_INDENT = 22
const TYPE_COLUMN = 78
const TITLE_COLUMN = 334

/**
 * Expand all or Collapse all, given on a row and followed by every row beneath
 * it. Orders are told apart by when they were given, so a row follows the
 * newest one above it until the operator folds the row himself, and a later
 * order overrides that fold again.
 */
interface FoldOrder {
  expanded: boolean
  nonce: number
}

let lastOrder = 0

function BlockedBy({ ids, depth, projectPath }: { ids: string[]; depth: number; projectPath: string }) {
  return (
    <div className="bead-row-blocked" style={{ paddingLeft: `${TITLE_COLUMN + depth * ROW_INDENT}px` }}>
      blocked by{' '}
      {ids.map((id, index) => (
        <span key={id}>
          {index > 0 && ', '}
          <button type="button" className="bead-row-blocked-link" onClick={() => openBeadCard(id, projectPath)}>{id}</button>
        </span>
      ))}
    </div>
  )
}

interface MapNodeProps {
  node: BeadTreeNode
  depth: number
  expandAll: boolean
  /** The newest order from a row above, if one was given. */
  order?: FoldOrder
}

function MapNode({ node, depth, expandAll, order }: MapNodeProps) {
  // The row's own fold, and the last order it was folded after: an order given
  // since then wins until the row is touched again.
  const [own, setOwn] = useState({ collapsed: false, after: 0 })
  const [given, setGiven] = useState<FoldOrder | null>(null)
  const orderNonce = order?.nonce ?? 0
  const collapsed = order !== undefined && own.after < orderNonce ? !order.expanded : own.collapsed
  const expanded = expandAll || !collapsed
  const foldable = node.children.length > 0
  const { row } = node

  const setExpanded = (open: boolean) => setOwn({ collapsed: !open, after: orderNonce })
  const setSubtreeExpanded = (open: boolean) => {
    lastOrder += 1
    setOwn({ collapsed: !open, after: lastOrder })
    setGiven({ expanded: open, nonce: lastOrder })
  }
  // What the children follow: the newest order, whether given here or above.
  const passed = given !== null && given.nonce > orderNonce ? given : order

  return (
    <>
      <BeadRow
        row={row}
        depth={depth}
        fold={foldable ? { count: node.children.length, expanded, setExpanded, setSubtreeExpanded } : undefined}
      />
      {row.blocked && row.blockedBy && row.blockedBy.length > 0 && (
        <BlockedBy ids={row.blockedBy} depth={depth} projectPath={row.projectPath} />
      )}
      {expanded && row.acceptance && (
        <div className="bead-map-acceptance" style={{ paddingLeft: `${TYPE_COLUMN + depth * ROW_INDENT}px` }}>
          <h3>Acceptance criteria</h3>
          <p>{row.acceptance}</p>
        </div>
      )}
      {expanded && node.children.map(child => (
        <MapNode key={beadRowKey(child.row)} node={child} depth={depth + 1} expandAll={expandAll} order={passed} />
      ))}
    </>
  )
}

export default function MapView({ roots, expandAll }: MapViewProps) {
  if (roots.length === 0) return <p className="beads-empty">No open work here.</p>
  return (
    <div className="bead-map">
      {roots.map(node => <MapNode key={beadRowKey(node.row)} node={node} depth={0} expandAll={expandAll} />)}
    </div>
  )
}
