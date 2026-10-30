import { useEffect, useMemo, useRef, useState } from 'react'
import type { ProcessMap as ProcessMapData, ProcessGroup, ProcessNode } from '../../services/systemClient'
import { blendLayouts, layoutProcessMap, type PlacedGroup, type ProcessLayout } from './processLayout'
import './ProcessMap.css'

const GLIDE_MS = 700
const PAD = 40
const MiB = 1024 * 1024

function formatBytes(bytes: number) {
  if (bytes >= 1024 * MiB) return `${(bytes / (1024 * MiB)).toFixed(1)}G`
  if (bytes >= MiB) return `${Math.round(bytes / MiB)}M`
  return `${Math.round(bytes / 1024)}K`
}

function formatCPU(percent: number) {
  return percent >= 10 ? `${Math.round(percent)}%` : `${percent.toFixed(1)}%`
}

const KIND_LABEL: Record<ProcessGroup['kind'], string> = {
  tmux: 'tmux session',
  service: 'service',
  system: 'system',
}

type Box = { x: number; y: number; w: number; h: number }

function boxFor(layout: ProcessLayout, focus: PlacedGroup | null): Box {
  if (focus) {
    const r = focus.r + PAD
    return { x: focus.x - r, y: focus.y - r - 30, w: r * 2, h: r * 2 + 30 }
  }
  const { minX, minY, maxX, maxY } = layout.bounds
  return { x: minX - PAD, y: minY - PAD, w: maxX - minX + PAD * 2, h: maxY - minY + PAD * 2 }
}

/** Glides from the drawn value to the latest one whenever the latest changes. */
function useGlide<T>(target: T | null, blend: (from: T, to: T, t: number) => T): T | null {
  const [drawn, setDrawn] = useState<T | null>(target)
  const drawnRef = useRef<T | null>(target)
  useEffect(() => {
    if (!target) return
    const from = drawnRef.current
    if (!from || window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches) {
      drawnRef.current = target
      setDrawn(target)
      return
    }
    const start = performance.now()
    let frame = 0
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / GLIDE_MS)
      const next = t >= 1 ? target : blend(from, target, t)
      drawnRef.current = next
      setDrawn(next)
      if (t < 1) frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [target, blend])
  return drawn
}

function blendBox(a: Box, b: Box, t: number): Box {
  const e = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2
  return { x: a.x + (b.x - a.x) * e, y: a.y + (b.y - a.y) * e, w: a.w + (b.w - a.w) * e, h: a.h + (b.h - a.h) * e }
}

type Picked = { group: ProcessGroup; node?: ProcessNode; hue: string }

export function ProcessMap({ data, error }: { data: ProcessMapData | null; error: string }) {
  const previous = useRef<ProcessLayout | null>(null)
  const layout = useMemo(() => {
    if (!data) return null
    const next = layoutProcessMap(data.groups || [], previous.current)
    previous.current = next
    return next
  }, [data])
  const drawn = useGlide(layout, blendLayouts)

  const [focusKey, setFocusKey] = useState<string | null>(null)
  const [picked, setPicked] = useState<Picked | null>(null)
  const focus = layout?.groups.find(group => group.key === focusKey) || null
  const targetBox = useMemo(() => (layout ? boxFor(layout, focus) : null), [layout, focus])
  const box = useGlide(targetBox, blendBox)

  // The drawn size of the map, so text and the hub keep one size on screen
  // whatever the zoom.
  const svgRef = useRef<SVGSVGElement | null>(null)
  const [screen, setScreen] = useState({ w: 1000, h: 700 })
  const ready = drawn !== null
  useEffect(() => {
    const svg = svgRef.current
    if (!svg || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect
      if (width > 0 && height > 0) setScreen({ w: width, h: height })
    })
    observer.observe(svg)
    return () => observer.disconnect()
  }, [ready])

  // Keep the picked process fresh as polls arrive, and drop it when it ends.
  const live = useMemo(() => {
    if (!picked || !layout) return null
    const group = layout.groups.find(g => g.key === picked.group.key)
    if (!group) return null
    if (!picked.node) return { group: group.group, hue: group.hue }
    const node = group.nodes.find(n => n.pid === picked.node?.pid)
    return node ? { group: group.group, node: node.node, hue: group.hue } : null
  }, [picked, layout])

  if (!drawn || !box || !data) {
    return (
      <section className="process-map" aria-label="Process map">
        <div className="process-map-empty">{error || 'Reading processes…'}</div>
      </section>
    )
  }

  const groups = data.groups || []
  const totalCPU = groups.reduce((sum, group) => sum + group.cpuPercent, 0) / Math.max(1, data.cores)
  const totalRSS = groups.reduce((sum, group) => sum + group.rssBytes, 0)
  const processCount = groups.reduce((sum, group) => sum + group.count, 0)
  // One screen pixel in map units.
  const unit = Math.max(box.w / screen.w, box.h / screen.h)

  return (
    <section className="process-map" aria-label="Process map">
      <div className="process-map-bar">
        <span className="process-map-title">Process map</span>
        <span>{processCount} processes</span>
        <span>{groups.length} groups</span>
        {focus && (
          <button type="button" className="process-map-back" onClick={() => setFocusKey(null)}>
            ← whole host
          </button>
        )}
        {error && <span className="process-map-stale" title={error}>stale</span>}
      </div>
      <svg
        ref={svgRef}
        className="process-map-svg"
        viewBox={`${box.x} ${box.y} ${box.w} ${box.h}`}
        preserveAspectRatio="xMidYMid meet"
        onClick={() => {
          setPicked(null)
          setFocusKey(null)
        }}
        role="img"
        aria-label={`${processCount} processes in ${groups.length} groups`}
        style={{ '--u': unit } as React.CSSProperties}
      >
        <g className="process-map-spokes">
          {drawn.groups.map(group => {
            const d = Math.hypot(group.x, group.y) || 1
            return (
              <line
                key={group.key}
                x1={group.x / d * 26 * unit}
                y1={group.y / d * 26 * unit}
                x2={group.x - group.x / d * group.r}
                y2={group.y - group.y / d * group.r}
              />
            )
          })}
        </g>

        {drawn.groups.map(group => (
          <g
            key={group.key}
            className={`process-group${focusKey === group.key ? ' is-focus' : ''}`}
            transform={`translate(${group.x} ${group.y})`}
            style={{ '--hue': group.hue } as React.CSSProperties}
          >
            <circle
              className="process-group-ring"
              r={group.r}
              onClick={event => {
                event.stopPropagation()
                setFocusKey(group.key)
                setPicked({ group: group.group, hue: group.hue })
              }}
            />
            <text className="process-group-name" y={-group.r - 22 * unit} fontSize={13 * unit}>{group.group.name}</text>
            <text className="process-group-meta" y={-group.r - 8 * unit} fontSize={10 * unit}>
              {group.group.count} {group.group.count === 1 ? 'process' : 'processes'} · {formatCPU(group.group.cpuPercent)} · {formatBytes(group.group.rssBytes)}
            </text>
            {group.nodes.map(node => {
              if (node.parent === null) return null
              const parent = group.nodes.find(other => other.pid === node.parent)
              return parent ? <line key={`l${node.pid}`} className="process-link" x1={parent.x} y1={parent.y} x2={node.x} y2={node.y} /> : null
            })}
            {group.nodes.map(node => {
              const busy = node.node.cpuPercent >= 2
              const beat = Math.max(0.6, 2.6 - node.node.cpuPercent / 30)
              const chosen = live?.node?.pid === node.pid
              return (
                <g
                  key={node.pid}
                  className={`process-node${node.centre ? ' is-centre' : ''}${chosen ? ' is-picked' : ''}`}
                  transform={`translate(${node.x} ${node.y})`}
                  onClick={event => {
                    event.stopPropagation()
                    setPicked({ group: group.group, node: node.node, hue: group.hue })
                  }}
                >
                  {busy && <circle className="process-halo" r={node.r * 2} style={{ animationDuration: `${beat}s` }} />}
                  {node.centre && <circle className="process-centre-ring" r={node.r + 5} />}
                  <circle className="process-dot" r={node.r} style={{ opacity: 0.55 + Math.min(1, node.node.cpuPercent / 25) * 0.45 }} />
                  {node.labelled && (
                    <g className="process-label" transform={`translate(${node.r + 5 * unit} ${-2 * unit})`}>
                      <text fontSize={(node.centre ? 12 : 10.5) * unit}>{node.node.name}</text>
                      <text className="process-label-meta" y={12 * unit} fontSize={9.5 * unit}>
                        {node.centre ? `cpu ${formatCPU(node.node.cpuPercent)}` : formatCPU(node.node.cpuPercent)}
                        {node.centre ? '' : '  '}
                        {node.centre ? '' : formatBytes(node.node.rssBytes)}
                      </text>
                      {node.centre && (
                        <text className="process-label-meta" y={24 * unit} fontSize={9.5 * unit}>mem {formatBytes(node.node.rssBytes)}</text>
                      )}
                    </g>
                  )}
                </g>
              )
            })}
          </g>
        ))}

        <g className="process-hub">
          <circle className="process-hub-halo" r={26 * unit} />
          <circle className="process-hub-dot" r={13 * unit} />
          <text x={34 * unit} y={-4 * unit} fontSize={13 * unit} className="process-hub-name">{data.hostname || 'host'}</text>
          <text x={34 * unit} y={12 * unit} fontSize={10 * unit} className="process-label-meta">cpu {formatCPU(totalCPU)} of {data.cores} cores</text>
          <text x={34 * unit} y={25 * unit} fontSize={10 * unit} className="process-label-meta">
            mem {formatBytes(totalRSS)}{data.memoryTotalBytes ? ` / ${formatBytes(data.memoryTotalBytes)}` : ''}
          </text>
        </g>
      </svg>

      {live && (
        <aside className="process-detail" style={{ '--hue': live.hue } as React.CSSProperties}>
          <div className="process-detail-group">
            {KIND_LABEL[live.group.kind]} · <strong>{live.group.name}</strong>
          </div>
          {live.node ? (
            <>
              <div className="process-detail-name">
                {live.node.name} <span>pid {live.node.pid}</span>
              </div>
              {live.node.command && <code className="process-detail-command">{live.node.command}</code>}
              <dl>
                <dt>CPU</dt><dd>{formatCPU(live.node.cpuPercent)}</dd>
                <dt>MEM</dt><dd>{formatBytes(live.node.rssBytes)}</dd>
                <dt>THREADS</dt><dd>{live.node.threads}</dd>
                <dt>PARENT</dt><dd>{live.node.ppid}</dd>
              </dl>
            </>
          ) : (
            <dl>
              <dt>PROCESSES</dt><dd>{live.group.count}</dd>
              <dt>CPU</dt><dd>{formatCPU(live.group.cpuPercent)}</dd>
              <dt>MEM</dt><dd>{formatBytes(live.group.rssBytes)}</dd>
            </dl>
          )}
        </aside>
      )}

      {layout && layout.hidden.length > 0 && (
        <div className="process-map-hidden">
          +{layout.hidden.length} quieter: {layout.hidden.slice(0, 8).map(group => group.name).join(', ')}
          {layout.hidden.length > 8 ? '…' : ''}
        </div>
      )}
    </section>
  )
}
