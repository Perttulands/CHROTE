/**
 * The strip across the top of the Beads tab: Beads created and closed each
 * day, with the Beads open at the end of each day as a faint area behind. A
 * drag across it brushes a date range that filters the table; a click takes
 * one day. The strip counts the current search, projects and statuses but not
 * the range itself, so the brush always has its context. A store that came
 * into being after the strip begins is marked on the axis, so the step it adds
 * reads as a new store and not as a backlog.
 *
 * The strip spans the last RECENT_DAYS days unless the operator asks for every
 * day, or a brushed range starts earlier; the counts behind it are the whole
 * host's either way, so the open area starts at its true height.
 */

import { memo, useMemo, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'
import { useMeasuredSize } from '../../hooks/useMeasuredSize'
import { RECENT_DAYS, dayLabel, stripStart, type SliceIndex, type SliceResult } from '../../beads/beadsSlice'
import type { StripSpan } from '../../beads/sliceUrl'

const HEIGHT = 128
const TOP = 14
const AXIS = 22
const LEFT = 34
const RIGHT = 40
const PLOT = HEIGHT - TOP - AXIS

interface TimeStripProps {
  index: SliceIndex
  result: SliceResult
  range: { from: number; to: number } | null
  onRange: (range: { from: number; to: number } | null) => void
  span: StripSpan
  onSpan: (span: StripSpan) => void
}

/** A round axis maximum and its step, so the gridlines fall on whole numbers. */
function niceScale(max: number): { top: number; step: number } {
  if (max <= 4) return { top: Math.max(1, max), step: 1 }
  const rough = max / 3
  const magnitude = 10 ** Math.floor(Math.log10(rough))
  const step = [1, 2, 2.5, 5, 10].map(f => f * magnitude).find(s => s >= rough) ?? rough
  return { top: Math.ceil(max / step) * step, step }
}

function TimeStrip({ index, result, range, onRange, span, onSpan }: TimeStripProps) {
  const { ref, width } = useMeasuredSize<HTMLDivElement>({ width: 960, height: HEIGHT })
  const total = result.created.length
  // Days are offsets from the index's first day; the strip draws `start` on.
  const start = stripStart(total, span, range, index.firstDay)
  const days = total - start
  const plotWidth = Math.max(40, width - LEFT - RIGHT)
  const dayWidth = plotWidth / days
  const at = (d: number) => LEFT + (d - start) * dayWidth
  const [drag, setDrag] = useState<{ start: number; end: number } | null>(null)
  const [hover, setHover] = useState<number | null>(null)
  const dragging = useRef(false)

  const scale = useMemo(() => {
    let most = 0
    let open = 0
    for (let d = start; d < total; d += 1) {
      most = Math.max(most, result.created[d], result.closed[d])
      open = Math.max(open, result.open[d])
    }
    return { bars: niceScale(most), open: niceScale(open) }
  }, [result, start, total])

  // The open count is a level, not a daily event: a line through each day's
  // middle, filled beneath, rather than a step per day that turns into blocks
  // when the days are wide.
  const open = useMemo(() => {
    if (scale.open.top === 0) return { area: '', line: '' }
    const y = (value: number) => TOP + PLOT - (value / scale.open.top) * PLOT
    let line = `M${LEFT},${y(result.open[start])}`
    for (let d = start; d < total; d += 1) line += ` L${LEFT + (d - start + 0.5) * dayWidth},${y(result.open[d])}`
    line += ` L${LEFT + days * dayWidth},${y(result.open[total - 1])}`
    return { area: `${line} L${LEFT + days * dayWidth},${TOP + PLOT} L${LEFT},${TOP + PLOT} Z`, line }
  }, [dayWidth, days, result, scale.open.top, start, total])

  const labels = useMemo(() => {
    const every = Math.max(1, Math.ceil(64 / dayWidth))
    const marks: number[] = []
    for (let d = total - 1; d >= start; d -= every) marks.unshift(d)
    return marks
  }, [dayWidth, start, total])

  const arrivals = useMemo(() => index.projects
    .filter(project => project.firstDay > index.firstDay + start)
    .map(project => ({ day: project.firstDay - index.firstDay, label: project.label })), [index, start])

  const dayAt = (event: ReactPointerEvent<SVGRectElement>) => {
    const box = event.currentTarget.getBoundingClientRect()
    const d = Math.floor((event.clientX - box.left) / Math.max(1, box.width / days))
    return start + Math.max(0, Math.min(days - 1, d))
  }

  const commit = (start: number, end: number) => {
    onRange({ from: index.firstDay + Math.min(start, end), to: index.firstDay + Math.max(start, end) })
  }

  const shown = drag
    ? { from: Math.min(drag.start, drag.end), to: Math.max(drag.start, drag.end) }
    : range ? { from: range.from - index.firstDay, to: range.to - index.firstDay } : null
  const barWidth = Math.max(1, Math.min(6, dayWidth / 2 - 0.5))
  const y = (value: number) => TOP + PLOT - (value / scale.bars.top) * PLOT
  const gridlines: number[] = []
  for (let v = scale.bars.step; v <= scale.bars.top; v += scale.bars.step) gridlines.push(v)

  return (
    <div className="beads-strip" ref={ref} data-ui="beads.strip">
      <svg width={width} height={HEIGHT} role="img" aria-label="Beads created and closed per day">
        <path className="beads-strip-open" d={open.area} />
        <path className="beads-strip-open-line" d={open.line} />
        {gridlines.map(v => (
          <g key={v}>
            <line className="beads-strip-grid" x1={LEFT} x2={LEFT + plotWidth} y1={y(v)} y2={y(v)} />
            <text className="beads-strip-tick" x={LEFT - 6} y={y(v) + 4} textAnchor="end">{v}</text>
          </g>
        ))}
        {scale.open.top > 0 && (
          <text className="beads-strip-tick beads-strip-open-tick" x={LEFT + plotWidth + 6} y={TOP + 4}>{scale.open.top}</text>
        )}
        <line className="beads-strip-axis" x1={LEFT} x2={LEFT + plotWidth} y1={TOP + PLOT} y2={TOP + PLOT} />
        {shown && (
          <rect
            className="beads-strip-brush"
            x={at(Math.max(start, shown.from))}
            y={TOP - 4}
            width={Math.max(2, (shown.to - Math.max(start, shown.from) + 1) * dayWidth)}
            height={PLOT + 4}
          />
        )}
        {Array.from({ length: days }, (_, offset) => {
          const d = start + offset
          const created = result.created[d]
          const closed = result.closed[d]
          if (created === 0 && closed === 0) return null
          const x = at(d) + dayWidth / 2
          return (
            <g key={d}>
              {created > 0 && <rect className="beads-strip-created" x={x - barWidth} y={y(created)} width={barWidth} height={TOP + PLOT - y(created)} />}
              {closed > 0 && <rect className="beads-strip-closed" x={x} y={y(closed)} width={barWidth} height={TOP + PLOT - y(closed)} />}
            </g>
          )
        })}
        {arrivals.map(arrival => (
          <line
            key={arrival.label}
            className="beads-strip-arrival"
            x1={at(arrival.day) + dayWidth / 2}
            x2={at(arrival.day) + dayWidth / 2}
            y1={TOP + PLOT + 1}
            y2={TOP + PLOT + 5}
          >
            <title>{`${arrival.label} first seen ${dayLabel(index.firstDay + arrival.day)}`}</title>
          </line>
        ))}
        {labels.map(d => (
          <text key={d} className="beads-strip-tick" x={at(d) + dayWidth / 2} y={HEIGHT - 6} textAnchor="middle">
            {dayLabel(index.firstDay + d)}
          </text>
        ))}
        <rect
          className="beads-strip-hit"
          x={LEFT}
          y={TOP - 4}
          width={plotWidth}
          height={PLOT + 4}
          onPointerDown={event => {
            if (event.button !== 0) return
            event.currentTarget.setPointerCapture?.(event.pointerId)
            dragging.current = true
            const d = dayAt(event)
            setDrag({ start: d, end: d })
            commit(d, d)
          }}
          onPointerMove={event => {
            const d = dayAt(event)
            if (d !== hover) setHover(d)
            if (!dragging.current || !drag) return
            if (d === drag.end) return
            setDrag({ start: drag.start, end: d })
            commit(drag.start, d)
          }}
          onPointerUp={() => { dragging.current = false; setDrag(null) }}
          onPointerCancel={() => { dragging.current = false; setDrag(null) }}
          onPointerLeave={() => setHover(null)}
        >
          <title>Drag to keep the Beads created, updated or closed in a range of days</title>
        </rect>
      </svg>
      <div className="beads-strip-legend">
        {total > RECENT_DAYS && (
          <span className="beads-strip-span" role="group" aria-label="Days the strip spans">
            <button type="button" aria-pressed={span === 'recent'} onClick={() => onSpan('recent')}>{`${RECENT_DAYS} days`}</button>
            <button type="button" aria-pressed={span === 'all'} onClick={() => onSpan('all')}>{`Since ${dayLabel(index.firstDay)}`}</button>
          </span>
        )}
        {hover !== null && hover < total && (
          <span className="beads-strip-reading" aria-hidden="true">
            {`${dayLabel(index.firstDay + hover)} · ${result.created[hover]} created · ${result.closed[hover]} closed · ${result.open[hover]} open`}
          </span>
        )}
        <span className="beads-strip-key beads-strip-key-created" aria-hidden="true">Created</span>
        <span className="beads-strip-key beads-strip-key-closed" aria-hidden="true">Closed</span>
        <span className="beads-strip-key beads-strip-key-open" aria-hidden="true">Open</span>
      </div>
    </div>
  )
}

export default memo(TimeStrip)
