/**
 * The flow of an epic: how the work travels, and where it forks.
 *
 * One epic at a time, drawn as waves left to right. The columns are what the
 * blocking edges say, so a column is a set of Beads that can be handed out at
 * once, and the number of columns is how long the epic is however many hands
 * work it. The lines are the blocking edges themselves; a band behind a group
 * is a parent inside the epic. Clicking a Bead puts it on the table without
 * moving the drawing; the arrow keys travel wave to wave and up and down a
 * column; the wheel and a drag move through the drawing.
 *
 * The arithmetic is in flowLayout.ts, the moving in useMapTransform.ts. This
 * file only chooses the epic, measures its box, and draws.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
  flowComponentKey,
  flowEpics,
  flowNeighbour,
  layoutFlow,
  layoutFlowComponent,
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

export interface FlowRevealRequest {
  projectPath: string
  id: string
  /** The linked component chosen when the menu action was invoked. */
  graphKey: string
  nonce: number
}

export default function FlowView({ rows, reveal }: { rows: WorkRow[]; reveal?: FlowRevealRequest | null }) {
  const table = useTableObject()
  const epics = useMemo(() => flowEpics(rows), [rows])
  // The picker's choice outlives a click on a node; until it is made, the epic
  // on the table is the one being read, and failing that the first in the store.
  const [picked, setPicked] = useState<string | null>(null)
  const [dismissedReveal, setDismissedReveal] = useState<number | null>(null)
  const revealTarget = reveal
    ? rows.find(row => row.projectPath === reveal.projectPath && row.id === reveal.id)
    : undefined
  const revealing = revealTarget !== undefined && reveal?.nonce !== dismissedReveal
  const onTable = table?.kind === 'bead'
    ? epics.find(epic => epic.id === table.id && (!table.projectPath || table.projectPath === epic.projectPath))
    : undefined

  // Entering Flow from an epic already on the table chooses that graph. Latch
  // the choice before a child click replaces the table object, or the fallback
  // would silently jump to the first epic in the store.
  useEffect(() => {
    if (picked === null && onTable) setPicked(beadRowKey(onTable))
  }, [onTable, picked])

  const epic = epics.find(candidate => beadRowKey(candidate) === picked) ?? onTable ?? epics[0]

  const graph = useMemo(
    () => (revealing && revealTarget
      ? layoutFlowComponent(rows, revealTarget)
      : epic ? layoutFlow(rows, epic) : EMPTY_FLOW),
    [epic, revealTarget, revealing, rows],
  )
  const { ref: box, width, height } = useMeasuredSize<HTMLDivElement>()
  const { ref, transform, moved, reset, centreOn, handlers, panned } =
    useMapTransform<HTMLDivElement>({ width, height })
  // The buttons, so the arrow keys can hand focus to the Bead next door. A
  // node that leaves the drawing takes its entry with it, on the ref itself.
  const nodeRefs = useRef(new Map<string, HTMLButtonElement>())
  const centredRequest = useRef<number | null>(null)

  // A menu navigation is explicit movement. Consume its nonce once, after the
  // target and a real viewport both exist; later table resizes and ordinary
  // node selections leave this transform alone.
  useEffect(() => {
    if (!reveal || !revealing || !revealTarget || width <= 0 || height <= 0) return
    if (centredRequest.current === reveal.nonce) return
    const measured = box.current?.getBoundingClientRect()
    if (!measured || Math.round(measured.width) !== width || Math.round(measured.height) !== height) return
    const componentKey = flowComponentKey(graph.nodes.map(node => node.row))
    if (componentKey !== reveal.graphKey) return
    const target = graph.nodes.find(node => node.row.id === reveal.id)
    if (!target) return
    centreOn(flowCentre(target))
    centredRequest.current = reveal.nonce
  }, [centreOn, graph, height, reveal, revealTarget, revealing, width])

  const pickEpic = useCallback((key: string) => {
    if (reveal) setDismissedReveal(reveal.nonce)
    setPicked(key)
  }, [reveal])

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

  if (!revealing && epics.length === 0) return <p className="beads-empty">No epic here to flow.</p>
  if (graph.nodes.length === 0) {
    return (
      <div className="bead-flow-view">
        {epics.length > 1 && epic && <EpicPicker epics={epics} chosen={epic} onPick={pickEpic} />}
        <p className="beads-empty">{epic?.id ?? revealTarget?.id} has nothing hanging under it yet.</p>
      </div>
    )
  }

  return (
    <div className="bead-flow-view">
      <div className="bead-flow-head">
        {revealing && revealTarget
          ? <span className="bead-flow-epic-name">Linked flow · {revealTarget.id} · {revealTarget.title}</span>
          : epics.length > 1 && epic
            ? <EpicPicker epics={epics} chosen={epic} onPick={pickEpic} />
            : epic && <span className="bead-flow-epic-name">{epic.id} · {epic.title}</span>}
        <span className="bead-flow-tally">
          {graph.waves} {graph.waves === 1 ? 'wave' : 'waves'} · {graph.nodes.length} Beads
        </span>
        {moved && (
          <button type="button" className="bead-flow-reset" onClick={reset}>Fit</button>
        )}
      </div>

      <div
        className="bead-flow"
        ref={box}
        data-ui="beads.flow"
        data-flow-graph={revealing ? reveal?.graphKey : epic && beadRowKey(epic)}
      >
        <div
          className="bead-flow-surface"
          ref={ref}
          {...handlers}
          role="group"
          aria-label={`Flow of ${revealing ? revealTarget?.id : epic?.id}`}
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
