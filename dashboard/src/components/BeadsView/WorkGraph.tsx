/**
 * The work graph pane: the selected Bead's family, its blocking edges, and the
 * Beads in other stores that block it or wait on it. Each card's border is its
 * project's colour, named in the legend beneath; finished work is dimmed and
 * blocked work outlined in dashes. A click selects, as a row click does, and a
 * double-click opens the card. The wheel zooms and a drag pans.
 */

import { memo, useEffect, useMemo, useRef } from 'react'
import { useMeasuredSize } from '../../hooks/useMeasuredSize'
import { useMapTransform, type MapScaleLimits } from '../../hooks/useMapTransform'
import { beadGlyph } from '../../beads/beadStatus'
import { SLICE_STATUSES, STATUS_LABELS, type SliceIndex } from '../../beads/beadsSlice'
import {
  NODE_HEIGHT,
  NODE_WIDTH,
  READABLE_SCALE,
  buildWorkGraph,
  edgePath,
  projectColours,
} from '../../beads/workGraph'

const LIMITS: MapScaleLimits = { min: 0.25, max: 2 }
const TITLE_CHARS = Math.floor((NODE_WIDTH - 16) / 6.6)

/** However many shelf colours the theme wrote; the theme decides, not this file. */
function shelfCount(): number {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return 10
  const style = getComputedStyle(document.documentElement)
  let count = 0
  while (count < 64 && style.getPropertyValue(`--shelf-${count}`).trim()) count += 1
  return count || 10
}

function clip(text: string, chars: number): string {
  return text.length > chars ? `${text.slice(0, chars - 1)}…` : text
}

interface WorkGraphProps {
  index: SliceIndex
  selected: number
  onSelect: (row: number) => void
  onOpen: (row: number) => void
}

function WorkGraph({ index, selected, onSelect, onOpen }: WorkGraphProps) {
  const { ref: box, width, height } = useMeasuredSize<HTMLDivElement>({ width: 640, height: 520 })
  const graph = useMemo(
    () => selected >= 0 ? buildWorkGraph(index, selected, height) : null,
    [height, index, selected],
  )
  // The canvas and its drawing are always in the document, so the box is
  // measured and the wheel bound before the first Bead is selected.
  const view = useMapTransform<SVGSVGElement>({ width, height, limits: LIMITS, enabled: graph !== null })
  const { restore, centreOn, toScreen } = view
  const placed = useRef<string>('')

  // A new family is placed once: whole if it fits at a readable scale,
  // otherwise from its first column, which the layout made as tall as the
  // pane. Moving the selection inside the family leaves the drawing where the
  // operator put it, unless the Bead is out of view.
  const node = graph?.nodes.find(candidate => candidate.row === selected)
  const layoutKey = graph ? `${index.rows[graph.head].id}:${graph.width}x${graph.height}:${width}x${height}` : ''
  useEffect(() => {
    if (!graph) return
    if (placed.current !== layoutKey) {
      placed.current = layoutKey
      const fit = Math.min(1, width / graph.width, height / graph.height)
      const scale = Math.max(READABLE_SCALE, fit)
      const x = graph.width * scale <= width ? (width - graph.width * scale) / 2 : 0
      const y = graph.height * scale <= height ? (height - graph.height * scale) / 2 : 0
      restore({ x, y, scale })
      if (node && (node.x + NODE_WIDTH) * scale + x > width) {
        centreOn({ x: node.x + NODE_WIDTH / 2, y: node.y + NODE_HEIGHT / 2 }, scale)
      }
      return
    }
    if (!node) return
    const topLeft = toScreen({ x: node.x, y: node.y })
    const bottomRight = toScreen({ x: node.x + NODE_WIDTH, y: node.y + NODE_HEIGHT })
    if (topLeft.x < 0 || topLeft.y < 0 || bottomRight.x > width || bottomRight.y > height) {
      centreOn({ x: node.x + NODE_WIDTH / 2, y: node.y + NODE_HEIGHT / 2 })
    }
    // The drawing is placed for a family or a box, and followed for a selection.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layoutKey, selected])

  const colours = useMemo(() => {
    if (!graph) return new Map<number, number>()
    const projects = [...new Set(graph.nodes.map(item => index.project[item.row]))]
    const byKey = projectColours(projects.map(p => index.projects[p].key), shelfCount())
    return new Map(projects.map(p => [p, byKey.get(index.projects[p].key) ?? 0]))
  }, [graph, index])

  const head = graph ? index.rows[graph.head] : null
  const headStatus = graph ? SLICE_STATUSES[index.status[graph.head]] : 'open'
  const parent = graph && graph.headParent >= 0 ? index.rows[graph.headParent] : null

  return (
    <div className="beads-graph" data-ui="beads.graph">
      {graph && head && (
        <header className="beads-graph-head">
          {parent && (
            <button type="button" className="beads-graph-up" onClick={() => onSelect(graph.headParent)} title={parent.title}>
              ↑ {parent.id}
            </button>
          )}
          <button
            type="button"
            className={`beads-graph-title${graph.head === selected ? ' selected' : ''}`}
            aria-label={`${head.id}: ${head.title}, ${STATUS_LABELS[headStatus]}`}
            onClick={() => onSelect(graph.head)}
            onDoubleClick={() => onOpen(graph.head)}
          >
            <span className="beads-graph-id">{head.id}</span>
            <span className="beads-graph-name">{head.title}</span>
            <span className="beads-graph-state">{beadGlyph(headStatus === 'closed' ? 'closed' : head.status, headStatus === 'blocked')} {STATUS_LABELS[headStatus]}</span>
          </button>
        </header>
      )}
      <div className="beads-graph-canvas" ref={box}>
        <svg
          ref={view.ref}
          className={graph ? 'drawn' : undefined}
          width={width}
          height={height}
          role="group"
          aria-label={head ? `Work graph of ${head.id}` : 'Work graph'}
          {...(graph ? view.handlers : {})}
        >
          <defs>
            <marker id="beads-graph-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto">
              <path d="M0,0 L8,4 L0,8 z" className="beads-graph-arrowhead" />
            </marker>
          </defs>
          {graph && (
            <g transform={view.groupTransform}>
              {graph.edges.map(edge => (
                <path
                  key={`${edge.from}-${edge.to}`}
                  className="beads-graph-edge"
                  d={edgePath(graph.nodes[edge.from], graph.nodes[edge.to])}
                  markerEnd="url(#beads-graph-arrow)"
                />
              ))}
              {graph.nodes.map(item => {
                const bead = index.rows[item.row]
                const status = SLICE_STATUSES[index.status[item.row]]
                const project = index.projects[index.project[item.row]]
                return (
                  <g
                    key={item.row}
                    className={[
                      'beads-node',
                      item.row === selected ? 'selected' : '',
                      status === 'closed' ? 'closed' : '',
                      status === 'blocked' ? 'blocked' : '',
                      item.outside ? 'outside' : '',
                    ].filter(Boolean).join(' ')}
                    transform={`translate(${item.x},${item.y})`}
                    role="button"
                    aria-label={`${bead.id}: ${bead.title}`}
                    aria-pressed={item.row === selected}
                    onClick={() => { if (!view.panned()) onSelect(item.row) }}
                    onDoubleClick={() => onOpen(item.row)}
                  >
                    <title>{`${bead.id}: ${bead.title}\n${project.label} · ${STATUS_LABELS[status]}`}</title>
                    <rect width={NODE_WIDTH} height={NODE_HEIGHT} style={{ stroke: `var(--shelf-${colours.get(index.project[item.row]) ?? 0})` }} />
                    <text x={8} y={15} className="beads-node-id">
                      {`${beadGlyph(status === 'closed' ? 'closed' : bead.status, status === 'blocked')} ${bead.id}`}
                    </text>
                    <text x={8} y={31} className="beads-node-title">{clip(bead.title, TITLE_CHARS)}</text>
                  </g>
                )
              })}
            </g>
          )}
        </svg>
        {!graph && <p className="beads-graph-empty">Select a Bead to see its work graph.</p>}
      </div>
      {graph && (
        <footer className="beads-graph-legend">
          {[...colours.entries()].map(([p, colour]) => (
            <span key={p} className="beads-graph-key" title={index.projects[p].path}>
              <span className="beads-graph-swatch" style={{ borderColor: `var(--shelf-${colour})` }} />
              {index.projects[p].label}
            </span>
          ))}
          {graph.omitted > 0 && <span className="beads-graph-omitted">{`${graph.omitted} more linked Beads not drawn`}</span>}
        </footer>
      )}
    </div>
  )
}

export default memo(WorkGraph)
