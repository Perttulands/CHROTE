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

/** The columns a row draws in, so what explains a row lines up under it. */
const ROW_INDENT = 22
const TYPE_COLUMN = 34
const TITLE_COLUMN = 290

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

function MapNode({ node, depth, expandAll }: { node: BeadTreeNode; depth: number; expandAll: boolean }) {
  const [collapsed, setCollapsed] = useState(false)
  const expanded = expandAll || !collapsed
  const foldable = node.children.length > 0
  const { row } = node

  return (
    <>
      <BeadRow
        row={row}
        depth={depth}
        fold={foldable ? { count: node.children.length, expanded, setExpanded: open => setCollapsed(!open) } : undefined}
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
        <MapNode key={beadRowKey(child.row)} node={child} depth={depth + 1} expandAll={expandAll} />
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
