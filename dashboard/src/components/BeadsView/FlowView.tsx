/**
 * The flow of an epic: how the work travels, and where it forks.
 *
 * One epic at a time, drawn as waves left to right. The columns are what the
 * blocking edges say, so a column is a set of Beads that can be handed out at
 * once, and the number of columns is how long the epic is however many hands
 * work it. The lines are the blocking edges themselves; a band behind a group
 * is a parent inside the epic. Clicking a Bead puts it on the table and brings
 * it to the middle; the arrow keys travel wave to wave and up and down a
 * column; the wheel and a drag move through the drawing.
 *
 * The arithmetic is in flowLayout.ts, the moving in useMapTransform.ts. This
 * file only chooses the epic, measures its box, and draws.
 */

import { useCallback, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import BeadTypeLabel from '../BeadTypeLabel'
import { openBeadCard } from '../../beads/beadCard'
import { beadGlyph, beadStatusLabel, isBeadClosed } from '../../beads/beadStatus'
import { beadRowKey, type WorkRow } from '../../beads/beadsTree'
import {
  EMPTY_FLOW,
  NODE_HEIGHT,
  NODE_WIDTH,
  flowCentre,
  flowEpics,
  flowNeighbour,
  layoutFlow,
  type FlowStep,
} from '../../beads/flowLayout'
import { useMapTransform } from '../../hooks/useMapTransform'
import { useMeasuredSize } from '../../hooks/useMeasuredSize'
import { useTableObject } from '../../context/TableContext'

const ARROWS: Record<string, FlowStep> = {
  ArrowUp: 'up',
  ArrowDown: 'down',
  ArrowLeft: 'left',
  ArrowRight: 'right',
}

export default function FlowView({ rows }: { rows: WorkRow[] }) {
  const table = useTableObject()
  const epics = useMemo(() => flowEpics(rows), [rows])
  // The picker's choice outlives a click on a node; until it is made, the epic
  // on the table is the one being read, and failing that the first in the store.
  const [picked, setPicked] = useState<string | null>(null)
  const onTable = table?.kind === 'bead'
    ? epics.find(epic => epic.id === table.id && (!table.projectPath || table.projectPath === epic.projectPath))
    : undefined
  const epic = epics.find(candidate => beadRowKey(candidate) === picked) ?? onTable ?? epics[0]

  const graph = useMemo(() => (epic ? layoutFlow(rows, epic) : EMPTY_FLOW), [epic, rows])
  const { ref: box, width, height } = useMeasuredSize<HTMLDivElement>()
  const { ref, transform, moved, reset, centreOn, handlers, panned } =
    useMapTransform<HTMLDivElement>({ width, height })
  // The buttons, so the arrow keys can hand focus to the Bead next door. A
  // node that leaves the drawing takes its entry with it, on the ref itself.
  const nodeRefs = useRef(new Map<string, HTMLButtonElement>())

  const travel = useCallback((key: string, step: FlowStep) => {
    const next = flowNeighbour(graph, key, step)
    if (next) nodeRefs.current.get(next.key)?.focus()
  }, [graph])

  const onKeyDown = (key: string) => (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    const step = ARROWS[event.key]
    if (!step) return
    event.preventDefault()
    travel(key, step)
  }

  if (epics.length === 0) return <p className="beads-empty">No epic here to flow.</p>
  if (graph.nodes.length === 0) {
    return (
      <div className="bead-flow-view">
        {epics.length > 1 && <EpicPicker epics={epics} chosen={epic} onPick={setPicked} />}
        <p className="beads-empty">{epic.id} has nothing hanging under it yet.</p>
      </div>
    )
  }

  return (
    <div className="bead-flow-view">
      <div className="bead-flow-head">
        {epics.length > 1
          ? <EpicPicker epics={epics} chosen={epic} onPick={setPicked} />
          : <span className="bead-flow-epic-name">{epic.id} · {epic.title}</span>}
        <span className="bead-flow-tally">
          {graph.waves} {graph.waves === 1 ? 'wave' : 'waves'} · {graph.nodes.length} Beads
        </span>
        {moved && (
          <button type="button" className="bead-flow-reset" onClick={reset}>Fit</button>
        )}
      </div>

      <div className="bead-flow" ref={box} data-ui="beads.flow">
        <div
          className="bead-flow-surface"
          ref={ref}
          {...handlers}
          role="group"
          aria-label={`Flow of ${epic.id}`}
        >
          <div
            className="bead-flow-canvas"
            style={{
              width: graph.width,
              height: graph.height,
              // The hook's transform, said in CSS: the same scale and offset
              // the Library map puts on its SVG group.
              transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})`,
            }}
          >
            <svg className="bead-flow-lines" width={graph.width} height={graph.height} aria-hidden="true">
              {graph.bands.map(band => (
                <g key={band.key}>
                  <rect
                    className="bead-flow-band"
                    x={band.x}
                    y={band.y}
                    width={band.width}
                    height={band.height}
                    rx={6}
                  />
                  <text className="bead-flow-band-name" x={band.x + 6} y={band.y - 4}>{band.key}</text>
                </g>
              ))}
              {graph.edges.map(edge => (
                <path
                  key={edge.key}
                  className={`bead-flow-edge${edge.back ? ' broken' : ''}`}
                  d={`M ${edge.x1} ${edge.y1} C ${edge.x1 + 32} ${edge.y1}, ${edge.x2 - 32} ${edge.y2}, ${edge.x2} ${edge.y2}`}
                />
              ))}
            </svg>
            {graph.nodes.map(node => {
              const { row } = node
              const state = beadStatusLabel(row.status, row.blocked)
              const shape = [
                'bead-flow-node',
                // The doctrine's own marker for a Bead that is done arguing,
                // so the type word greys with the rest wherever it is drawn.
                isBeadClosed(row.status) ? 'bead-row-closed' : '',
                row.status === 'in_progress' ? 'in-progress' : '',
              ].filter(Boolean).join(' ')
              return (
                <button
                  key={node.key}
                  type="button"
                  className={shape}
                  style={{ left: node.x, top: node.y, width: NODE_WIDTH, height: NODE_HEIGHT }}
                  title={`${row.id} · ${state}`}
                  ref={element => {
                    if (element) nodeRefs.current.set(node.key, element)
                    else nodeRefs.current.delete(node.key)
                  }}
                  onKeyDown={onKeyDown(node.key)}
                  onClick={() => {
                    // A drag that ended over a node moved the drawing; it did
                    // not open the Bead.
                    if (panned()) return
                    openBeadCard(row.id, row.projectPath, row.title)
                    centreOn(flowCentre(node))
                  }}
                >
                  <span className="bead-flow-node-line">
                    <span className="bead-flow-node-glyph">{beadGlyph(row.status, row.blocked)}</span>
                    <BeadTypeLabel type={row.type} className="bead-flow-node-type" />
                    <span className="bead-flow-node-id">{row.id}</span>
                  </span>
                  <span className="bead-flow-node-title">{row.title}</span>
                </button>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}

function EpicPicker({ epics, chosen, onPick }: {
  epics: WorkRow[]
  chosen: WorkRow
  onPick: (key: string) => void
}) {
  return (
    <label className="bead-flow-picker">
      Epic
      <select
        aria-label="Epic to flow"
        value={beadRowKey(chosen)}
        onChange={event => onPick(event.target.value)}
      >
        {epics.map(epic => (
          <option key={beadRowKey(epic)} value={beadRowKey(epic)}>{epic.id} · {epic.title}</option>
        ))}
      </select>
    </label>
  )
}
