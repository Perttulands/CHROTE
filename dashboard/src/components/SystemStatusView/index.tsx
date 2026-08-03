import { type PointerEvent as ReactPointerEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { AlertTriangle, Copy, Pause, Play, RefreshCw } from 'lucide-react'
import {
  SystemApiError,
  getSystemHistory,
  getSystemStatus,
  type SystemDiskStatus,
  type SystemGPUStatus,
  type SystemStatus,
  type SystemWarning,
} from '../../services/systemClient'
import { copyTextToClipboard } from '../../utils/clipboard'

const ACTIVE_POLL_MS = 2000
const BACKGROUND_POLL_MS = 10000
const MAX_TIMELINE_SAMPLES = 300
const SWAP_ACTIVITY_WARN_BPS = 1024 * 1024

// The chart SVGs stretch with preserveAspectRatio="none", so geometry is authored in a
// fixed viewBox and every stroke opts out of scaling. Text lives in HTML overlays instead
// of the SVG so it never inherits the non-uniform stretch.
const CHART_VIEW_WIDTH = 1000
const CHART_VIEW_HEIGHT = 100
const AXIS_TICK_COUNT = 5
const MIN_SCALE_STEP = 5

type InstrumentKey = 'cpu' | 'load' | 'memory' | 'swap' | 'gpu' | 'vram'
type InstrumentTone = 'normal' | 'warn' | 'muted'

interface TimelineSample {
  at: number
  timestamp: string
  gpuPercent: number | null
  vramPercent: number | null
  ramPercent: number | null
  swapPercent: number | null
  swapActivityBytesPerSecond: number | null
  cpuPercent: number | null
  loadPercent: number | null
}

interface Instrument {
  key: InstrumentKey
  label: string
  value?: string
  meta?: string
  tone: InstrumentTone
  values: Array<number | null>
}

interface SeriesStats {
  peak: number
  average: number
  scaleMax: number
}

interface AxisTick {
  at: number
  percent: number
}

function clampPercent(value: number) {
  return Math.max(0, Math.min(100, value))
}

function readablePercent(value: number | null | undefined) {
  if (typeof value !== 'number' || Number.isNaN(value)) return null
  return clampPercent(value)
}

function formatPercent(value: number | null | undefined) {
  const safe = readablePercent(value)
  if (safe === null) return '--'
  return `${safe.toFixed(safe >= 10 ? 0 : 1)}%`
}

function formatBytes(bytes: number | null | undefined) {
  if (typeof bytes !== 'number' || Number.isNaN(bytes)) return '--'
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`
}

function formatRate(bytesPerSecond: number | null | undefined) {
  if (typeof bytesPerSecond !== 'number' || Number.isNaN(bytesPerSecond)) return '--/s'
  return `${formatBytes(bytesPerSecond)}/s`
}

function formatUptime(seconds: number) {
  const wholeSeconds = Math.max(0, Math.floor(seconds))
  const days = Math.floor(wholeSeconds / 86400)
  const hours = Math.floor((wholeSeconds % 86400) / 3600)
  const minutes = Math.floor((wholeSeconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}

function formatClock(ms: number) {
  if (!Number.isFinite(ms)) return '--'
  return new Date(ms).toLocaleTimeString()
}

function formatTime(raw?: string) {
  if (!raw) return '--'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  return date.toLocaleTimeString()
}

function formatTemperature(celsius: number | null | undefined) {
  if (typeof celsius !== 'number' || Number.isNaN(celsius)) return ''
  return `${Math.round(celsius)} °C`
}

function formatWatts(watts: number | null | undefined) {
  if (typeof watts !== 'number' || Number.isNaN(watts)) return ''
  return `${Math.round(watts)} W`
}

function timestampMs(status: SystemStatus, fallbackIndex = 0) {
  const parsed = Date.parse(status.timestamp)
  return Number.isNaN(parsed) ? fallbackIndex : parsed
}

function cpuUsage(previous: SystemStatus | null, next: SystemStatus) {
  if (!previous?.cpu || !next.cpu) return null
  const totalDelta = next.cpu.totalTicks - previous.cpu.totalTicks
  const idleDelta = next.cpu.idleTicks - previous.cpu.idleTicks
  if (totalDelta <= 0 || idleDelta < 0) return null
  return clampPercent((1 - idleDelta / totalDelta) * 100)
}

function secondsBetween(previous: SystemStatus | null, next: SystemStatus) {
  if (!previous) return null
  const seconds = (timestampMs(next) - timestampMs(previous)) / 1000
  return seconds > 0 ? seconds : null
}

function swapActivityBytesPerSecond(previous: SystemStatus | null, next: SystemStatus) {
  const previousMemory = previous?.memory
  const nextMemory = next.memory
  if (!previousMemory || !nextMemory) return null
  const previousIn = previousMemory.swapInPages
  const previousOut = previousMemory.swapOutPages
  const nextIn = nextMemory.swapInPages
  const nextOut = nextMemory.swapOutPages
  if ([previousIn, previousOut, nextIn, nextOut].some(value => typeof value !== 'number')) return null

  const seconds = secondsBetween(previous, next)
  if (!seconds) return null
  const inDelta = Math.max(0, (nextIn || 0) - (previousIn || 0))
  const outDelta = Math.max(0, (nextOut || 0) - (previousOut || 0))
  const pageSize = nextMemory.pageSizeBytes || previousMemory.pageSizeBytes || 4096
  return (inDelta + outDelta) * pageSize / seconds
}

function swapActivityMeta(bytesPerSecond: number | null | undefined) {
  if (typeof bytesPerSecond !== 'number' || Number.isNaN(bytesPerSecond)) return 'active swap I/O pending'
  return `${formatRate(bytesPerSecond)} active swap I/O`
}

function hasActiveSwapPressure(bytesPerSecond: number | null | undefined) {
  return typeof bytesPerSecond === 'number' && bytesPerSecond >= SWAP_ACTIVITY_WARN_BPS
}

function diskLabel(disk?: SystemDiskStatus) {
  if (!disk) return '--'
  return `${formatBytes(disk.usedBytes)} / ${formatBytes(disk.totalBytes)}`
}

function gpuSourceLabel(gpu?: SystemGPUStatus) {
  if (!gpu?.source) return ''
  if (gpu.source.includes('/wsl/') || gpu.source.includes('lxss')) return 'WSL nvidia-smi'
  return gpu.source.endsWith('nvidia-smi') ? 'nvidia-smi' : gpu.source
}

function primaryGPU(status?: SystemStatus | null) {
  return status?.gpus?.find(gpu => gpu.available) || status?.gpus?.[0]
}

function gpuLabel(gpu?: SystemGPUStatus) {
  if (!gpu) return 'No GPU data'
  if (!gpu.available) return 'GPU unavailable'
  return gpu.name || 'GPU available'
}

function gpuMemoryPercent(gpu?: SystemGPUStatus) {
  if (!gpu?.available || !gpu.memoryTotalBytes) return null
  return clampPercent((gpu.memoryUsedBytes || 0) / gpu.memoryTotalBytes * 100)
}

function gpuMemoryLabel(gpu?: SystemGPUStatus) {
  if (!gpu?.available || !gpu.memoryTotalBytes) return ''
  return `${formatBytes(gpu.memoryUsedBytes)} / ${formatBytes(gpu.memoryTotalBytes)}`
}

// Thermals and power draw ship in every nvidia-smi payload; surface them next to the GPU trace.
function gpuThermalMeta(gpu?: SystemGPUStatus) {
  if (!gpu?.available) return ''
  return [formatTemperature(gpu.temperatureCelsius), formatWatts(gpu.powerWatts), gpuSourceLabel(gpu)]
    .filter(Boolean)
    .join(' · ')
}

function formatSystemError(err: unknown, fallback: string) {
  if (err instanceof SystemApiError) return `${err.code}: ${err.message}`
  if (err instanceof Error) return err.message
  return fallback
}

function loadPercent(status?: SystemStatus | null) {
  if (!status?.cpu?.cores || typeof status.host?.load1 !== 'number') return null
  return clampPercent((status.host.load1 / status.cpu.cores) * 100)
}

function buildWarnings(status: SystemStatus | null, rootDisk?: SystemDiskStatus, latestTimeline?: TimelineSample): SystemWarning[] {
  if (!status) return []
  const warnings = [...(status.warnings || [])]
  const memory = status.memory
  if (memory?.usedPercent >= 90) {
    warnings.push({ code: 'MEMORY_HIGH', message: `Memory use is ${formatPercent(memory.usedPercent)}` })
  }
  if ((memory?.swapTotalBytes || 0) > 0 && hasActiveSwapPressure(latestTimeline?.swapActivityBytesPerSecond)) {
    warnings.push({ code: 'SWAP_ACTIVE', message: `Active swap I/O is ${formatRate(latestTimeline?.swapActivityBytesPerSecond)}` })
  }
  if (rootDisk && rootDisk.usedPercent >= 85) {
    warnings.push({ code: 'DISK_HIGH', message: `${rootDisk.mount} disk use is ${formatPercent(rootDisk.usedPercent)}` })
  }
  const load = loadPercent(status) || 0
  if (load >= 100) {
    warnings.push({ code: 'LOAD_HIGH', message: `1m load is ${status.host.load1.toFixed(2)} across ${status.cpu?.cores || '--'} cores` })
  }
  return warnings
}

function combineStatusHistory(samples: SystemStatus[], current: SystemStatus | null) {
  const byTimestamp = new Map<string, SystemStatus>()
  samples.forEach((sample, index) => {
    byTimestamp.set(sample.timestamp || `history-${index}`, sample)
  })
  if (current) {
    byTimestamp.set(current.timestamp || 'current', current)
  }
  return Array.from(byTimestamp.values())
    .sort((left, right) => timestampMs(left) - timestampMs(right))
    .slice(-MAX_TIMELINE_SAMPLES)
}

function buildTimelineSamples(statuses: SystemStatus[]): TimelineSample[] {
  return statuses.map((sample, index) => {
    const previous = index > 0 ? statuses[index - 1] : null
    const gpu = primaryGPU(sample)
    const memory = sample.memory
    return {
      at: timestampMs(sample, index),
      timestamp: sample.timestamp,
      gpuPercent: gpu?.available ? readablePercent(gpu.utilizationPercent) : null,
      vramPercent: gpuMemoryPercent(gpu),
      ramPercent: readablePercent(memory?.usedPercent),
      swapPercent: (memory?.swapTotalBytes || 0) > 0 ? readablePercent(memory?.swapUsedPercent) : null,
      swapActivityBytesPerSecond: swapActivityBytesPerSecond(previous, sample),
      cpuPercent: cpuUsage(previous, sample),
      loadPercent: loadPercent(sample),
    }
  })
}

// A fixed 0-100 axis makes an idle host unreadable: every trace collapses onto the baseline.
// Each row instead gets headroom above its own peak, rounded to a multiple of five, and the
// axis is labelled with that ceiling so a scaled row can never be mistaken for a busy one.
function scaleCeiling(peak: number) {
  const withHeadroom = Math.ceil(peak * 1.25 / MIN_SCALE_STEP) * MIN_SCALE_STEP
  return Math.min(100, Math.max(MIN_SCALE_STEP, withHeadroom))
}

function seriesStats(values: Array<number | null>): SeriesStats | null {
  let peak = Number.NEGATIVE_INFINITY
  let total = 0
  let count = 0
  values.forEach(value => {
    if (typeof value !== 'number' || Number.isNaN(value)) return
    peak = Math.max(peak, value)
    total += value
    count += 1
  })
  if (count === 0) return null
  return { peak, average: total / count, scaleMax: scaleCeiling(peak) }
}

function lastReading(values: Array<number | null>) {
  for (let index = values.length - 1; index >= 0; index -= 1) {
    const value = values[index]
    if (typeof value === 'number' && !Number.isNaN(value)) return value
  }
  return null
}

function buildAxisTicks(startTime: number, endTime: number, hasSamples: boolean): AxisTick[] {
  if (!hasSamples || !Number.isFinite(startTime) || !Number.isFinite(endTime)) return []
  const range = endTime - startTime
  if (range <= 0) return [{ at: endTime, percent: 100 }]
  return Array.from({ length: AXIS_TICK_COUNT }, (_unused, index) => {
    const fraction = index / (AXIS_TICK_COUNT - 1)
    return { at: startTime + range * fraction, percent: fraction * 100 }
  })
}

// Splits a series into runs of consecutive readings so gaps stay gaps instead of being
// bridged by a straight line that never happened.
function buildTraceSegments(values: Array<number | null>, xFor: (index: number) => number, scaleMax: number) {
  const segments: Array<Array<{ x: number; y: number }>> = []
  let run: Array<{ x: number; y: number }> = []
  values.forEach((value, index) => {
    if (typeof value !== 'number' || Number.isNaN(value)) {
      if (run.length) segments.push(run)
      run = []
      return
    }
    run.push({ x: xFor(index), y: yForValue(value, scaleMax) })
  })
  if (run.length) segments.push(run)
  return segments
}

function yForValue(value: number, scaleMax: number) {
  const ceiling = scaleMax > 0 ? scaleMax : 100
  const fraction = Math.max(0, Math.min(1, clampPercent(value) / ceiling))
  return CHART_VIEW_HEIGHT - fraction * CHART_VIEW_HEIGHT
}

function traceLinePath(points: Array<{ x: number; y: number }>) {
  return points.map((point, index) => `${index === 0 ? 'M' : 'L'}${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(' ')
}

function traceAreaPath(points: Array<{ x: number; y: number }>) {
  const first = points[0]
  const last = points[points.length - 1]
  const body = points.map(point => `L${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(' ')
  return `M${first.x.toFixed(2)},${CHART_VIEW_HEIGHT} ${body} L${last.x.toFixed(2)},${CHART_VIEW_HEIGHT} Z`
}

function InstrumentTrace({
  instrument,
  xFor,
  axisTicks,
  scaleMax,
  hoveredIndex,
  onHoverIndex,
  onLeave,
}: {
  instrument: Instrument
  xFor: (index: number) => number
  axisTicks: AxisTick[]
  scaleMax: number
  hoveredIndex: number | null
  onHoverIndex: (localX: number) => void
  onLeave: () => void
}) {
  const segments = buildTraceSegments(instrument.values, xFor, scaleMax)
  const hoveredValue = hoveredIndex === null ? null : instrument.values[hoveredIndex]
  const hoveredX = hoveredIndex === null ? null : xFor(hoveredIndex)

  const handlePointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    if (bounds.width <= 0) return
    onHoverIndex((event.clientX - bounds.left) / bounds.width * CHART_VIEW_WIDTH)
  }

  return (
    <svg
      className="system-trace"
      role="img"
      aria-label={`${instrument.label} history`}
      viewBox={`0 0 ${CHART_VIEW_WIDTH} ${CHART_VIEW_HEIGHT}`}
      preserveAspectRatio="none"
      onPointerMove={handlePointerMove}
      onPointerLeave={onLeave}
    >
      <title>{instrument.label} history</title>
      <rect className="system-trace-bg" x="0" y="0" width={CHART_VIEW_WIDTH} height={CHART_VIEW_HEIGHT} />
      <line className="system-trace-grid" x1="0" x2={CHART_VIEW_WIDTH} y1={CHART_VIEW_HEIGHT / 2} y2={CHART_VIEW_HEIGHT / 2} />
      {axisTicks.map(tick => (
        <line
          key={`guide-${tick.percent}`}
          className="system-trace-guide"
          x1={tick.percent / 100 * CHART_VIEW_WIDTH}
          x2={tick.percent / 100 * CHART_VIEW_WIDTH}
          y1="0"
          y2={CHART_VIEW_HEIGHT}
        />
      ))}
      <line className="system-trace-baseline" x1="0" x2={CHART_VIEW_WIDTH} y1={CHART_VIEW_HEIGHT} y2={CHART_VIEW_HEIGHT} />
      {segments.map((points, index) => (
        points.length === 1 ? (
          <line
            key={`stem-${index}`}
            className="system-trace-stem"
            x1={points[0].x}
            x2={points[0].x}
            y1={points[0].y}
            y2={CHART_VIEW_HEIGHT}
          />
        ) : (
          <g key={`segment-${index}`}>
            <path className="system-trace-area" d={traceAreaPath(points)} />
            <path className="system-trace-line" d={traceLinePath(points)} />
          </g>
        )
      ))}
      {hoveredX !== null && (
        <line className="system-trace-crosshair" x1={hoveredX} x2={hoveredX} y1="0" y2={CHART_VIEW_HEIGHT} />
      )}
      {hoveredX !== null && typeof hoveredValue === 'number' && (
        <line
          className="system-trace-marker"
          x1={hoveredX}
          x2={hoveredX}
          y1={yForValue(hoveredValue, scaleMax)}
          y2={CHART_VIEW_HEIGHT}
        />
      )}
    </svg>
  )
}

function InstrumentBoard({
  instruments,
  samples,
  historyError,
  historyLabel,
}: {
  instruments: Instrument[]
  samples: TimelineSample[]
  historyError?: string
  historyLabel: string
}) {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)

  const finiteTimes = samples.map(sample => sample.at).filter(Number.isFinite)
  const startTime = finiteTimes.length ? Math.min(...finiteTimes) : 0
  const endTime = finiteTimes.length ? Math.max(...finiteTimes) : startTime
  const timeRange = endTime - startTime

  const xFor = useCallback((index: number) => {
    const at = samples[index]?.at
    if (!Number.isFinite(at) || timeRange <= 0) return CHART_VIEW_WIDTH
    return (at - startTime) / timeRange * CHART_VIEW_WIDTH
  }, [samples, startTime, timeRange])

  const axisTicks = buildAxisTicks(startTime, endTime, samples.length > 0)

  const handleHoverIndex = useCallback((localX: number) => {
    if (samples.length === 0) return
    const clamped = Math.max(0, Math.min(CHART_VIEW_WIDTH, localX))
    let nearest = 0
    let nearestDistance = Number.POSITIVE_INFINITY
    samples.forEach((_sample, index) => {
      const distance = Math.abs(xFor(index) - clamped)
      if (distance < nearestDistance) {
        nearest = index
        nearestDistance = distance
      }
    })
    setHoveredIndex(nearest)
  }, [samples, xFor])

  const clearHover = useCallback(() => setHoveredIndex(null), [])
  const hoveredSample = hoveredIndex === null ? null : samples[hoveredIndex]

  return (
    <div className={`system-board ${hoveredSample ? 'is-scrubbing' : ''}`}>
      {historyError && <div className="system-timeline-note" role="status">History unavailable: {historyError}</div>}
      <div className="system-axis">
        <div className="system-axis-gutter">
          {hoveredSample
            ? <span className="system-axis-scrub" aria-label="History sample">at {formatTime(hoveredSample.timestamp)}</span>
            : <span>{historyLabel}</span>}
        </div>
        <div className="system-axis-track">
          {axisTicks.map((tick, index) => (
            <span
              key={`tick-${tick.percent}`}
              className="system-axis-tick"
              data-align={index === 0 ? 'start' : index === axisTicks.length - 1 ? 'end' : 'middle'}
              style={{ left: `${tick.percent}%` }}
            >
              {formatClock(tick.at)}
            </span>
          ))}
        </div>
      </div>
      <div className="system-instruments">
        {samples.length === 0 && <div className="system-timeline-empty">Waiting for history</div>}
        {samples.length > 0 && instruments.map(instrument => {
          const stats = seriesStats(instrument.values)
          const scaleMax = stats?.scaleMax ?? 100
          const live = lastReading(instrument.values)
          const hovered = hoveredIndex === null ? null : instrument.values[hoveredIndex]
          const reading = hoveredIndex === null ? live : hovered
          const rowLabel = [instrument.label, formatPercent(reading), instrument.value].filter(Boolean).join(' ')

          return (
            <section
              key={instrument.key}
              className={`system-instrument system-instrument-${instrument.key} system-tone-${instrument.tone}`}
              aria-label={rowLabel}
            >
              <div className="system-instrument-gutter">
                <div className="system-instrument-head">
                  <span className="system-instrument-label">{instrument.label}</span>
                  <strong className={`system-instrument-reading ${hoveredIndex === null ? '' : 'is-scrubbed'}`}>
                    {formatPercent(reading)}
                  </strong>
                </div>
                {instrument.value && <span className="system-instrument-value" title={instrument.value}>{instrument.value}</span>}
                {instrument.meta && <span className="system-instrument-meta" title={instrument.meta}>{instrument.meta}</span>}
                {stats && (
                  <span className="system-instrument-stats">
                    peak {formatPercent(stats.peak)} · avg {formatPercent(stats.average)}
                  </span>
                )}
              </div>
              <div className="system-instrument-scale" aria-hidden="true">
                <span>{scaleMax}%</span>
                <span>0</span>
              </div>
              <div className="system-instrument-chart">
                <InstrumentTrace
                  instrument={instrument}
                  xFor={xFor}
                  axisTicks={axisTicks}
                  scaleMax={scaleMax}
                  hoveredIndex={hoveredIndex}
                  onHoverIndex={handleHoverIndex}
                  onLeave={clearHover}
                />
              </div>
            </section>
          )
        })}
      </div>
    </div>
  )
}

function WarningPanel({ warnings }: { warnings: SystemWarning[] }) {
  if (warnings.length === 0) return null
  return (
    <section className="system-warning-panel" aria-label="Warnings">
      <span className="system-warning-count">{warnings.length} active</span>
      <div className="system-warning-list">
        {warnings.map((warning, index) => (
          <div key={`${warning.code}-${index}`} className="system-warning-row">
            <AlertTriangle size={14} />
            <strong>{warning.code}</strong>
            <span>{warning.message}</span>
          </div>
        ))}
      </div>
    </section>
  )
}

function SystemStatusView({ active = true }: { active?: boolean }) {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [historySamples, setHistorySamples] = useState<SystemStatus[]>([])

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [historyError, setHistoryError] = useState('')
  const [paused, setPaused] = useState(false)
  const [copyStatus, setCopyStatus] = useState('')

  const refresh = useCallback(async () => {
    setError('')
    setHistoryError('')

    const [statusResult, historyResult] = await Promise.allSettled([
      getSystemStatus(),
      getSystemHistory(),
    ])

    let next: SystemStatus | null = null

    if (statusResult.status === 'fulfilled') {
      next = statusResult.value
      setStatus(next)
    } else {
      setError(formatSystemError(statusResult.reason, 'System status request failed'))
    }

    if (historyResult.status === 'fulfilled') {
      setHistorySamples(combineStatusHistory(historyResult.value.samples || [], next))
    } else {
      setHistorySamples(combineStatusHistory([], next))
      setHistoryError(formatSystemError(historyResult.reason, 'System history request failed'))
    }

    setLoading(false)
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    if (paused) return
    const interval = window.setInterval(refresh, active ? ACTIVE_POLL_MS : BACKGROUND_POLL_MS)
    return () => window.clearInterval(interval)
  }, [active, paused, refresh])

  const host = status?.host
  const cpu = status?.cpu
  const disks = status?.disks || []
  const rootDisk = disks.find(disk => disk.mount === '/') || disks[0]
  const gpu = primaryGPU(status)
  const gpuMemory = gpuMemoryLabel(gpu)
  const memory = status?.memory
  const hasSwap = (memory?.swapTotalBytes || 0) > 0
  const timelineSamples = useMemo(() => buildTimelineSamples(historySamples), [historySamples])
  const latestTimeline = timelineSamples[timelineSamples.length - 1]
  const warnings = useMemo(() => buildWarnings(status, rootDisk, latestTimeline), [status, rootDisk, latestTimeline])

  const cpuValue = cpu ? `${cpu.cores} cores` : status ? 'CPU unavailable' : loading ? 'loading' : undefined
  const loadValue = host ? `${host.load1.toFixed(2)} / ${host.load5.toFixed(2)} / ${host.load15.toFixed(2)}` : status ? 'load unavailable' : loading ? 'loading' : undefined
  const ramValue = memory ? `${formatBytes(memory.usedBytes)} / ${formatBytes(memory.totalBytes)}` : status ? 'memory unavailable' : loading ? 'loading' : undefined
  const swapValue = hasSwap ? `${formatBytes(memory?.swapUsedBytes)} / ${formatBytes(memory?.swapTotalBytes)}` : status ? memory ? 'swap not configured' : 'swap unavailable' : loading ? 'loading' : undefined
  const gpuValue = gpu?.available ? gpuLabel(gpu) : gpu ? gpuLabel(gpu) : status ? 'No GPU data' : loading ? 'loading' : undefined
  const vramValue = gpu?.available && gpuMemory ? gpuMemory : status ? 'VRAM unavailable' : loading ? 'loading' : undefined
  const storageValue = rootDisk ? diskLabel(rootDisk) : status ? 'storage unavailable' : loading ? 'loading' : undefined

  const instruments = useMemo<Instrument[]>(() => {
    const cpuSeries = timelineSamples.map(sample => sample.cpuPercent)
    const loadSeries = timelineSamples.map(sample => sample.loadPercent)
    const ramSeries = timelineSamples.map(sample => sample.ramPercent)
    const swapSeries = timelineSamples.map(sample => sample.swapPercent)
    const gpuSeries = timelineSamples.map(sample => sample.gpuPercent)
    const vramSeries = timelineSamples.map(sample => sample.vramPercent)

    return [
      {
        key: 'cpu',
        label: 'CPU',
        value: cpuValue,
        // The load triple belongs to the Load row; repeating it here would duplicate the same figures.
        meta: undefined,
        tone: (latestTimeline?.cpuPercent || 0) >= 85 ? 'warn' : 'normal',
        values: cpuSeries,
      },
      {
        key: 'load',
        label: 'Load',
        value: loadValue,
        meta: cpu ? `1m / 5m / 15m across ${cpu.cores} cores` : undefined,
        tone: (latestTimeline?.loadPercent || 0) >= 100 ? 'warn' : 'normal',
        values: loadSeries,
      },
      {
        key: 'memory',
        label: 'RAM',
        value: ramValue,
        meta: memory ? `free ${formatBytes(memory.freeBytes)} · available ${formatBytes(memory.availableBytes)}` : undefined,
        tone: (memory?.usedPercent || 0) >= 90 ? 'warn' : 'normal',
        values: ramSeries,
      },
      {
        key: 'swap',
        label: 'Swap',
        value: swapValue,
        meta: hasSwap
          ? [swapActivityMeta(latestTimeline?.swapActivityBytesPerSecond), (memory?.swapCachedBytes || 0) > 0 ? `${formatBytes(memory?.swapCachedBytes)} cached` : ''].filter(Boolean).join(' · ')
          : undefined,
        tone: hasSwap
          ? hasActiveSwapPressure(latestTimeline?.swapActivityBytesPerSecond) ? 'warn' : 'normal'
          : 'muted',
        values: swapSeries,
      },
      {
        key: 'gpu',
        label: 'GPU',
        value: gpuValue,
        meta: gpu?.available ? gpuThermalMeta(gpu) : gpu?.message || (status ? 'telemetry unavailable' : undefined),
        tone: gpu?.available ? 'normal' : 'muted',
        values: gpuSeries,
      },
      {
        key: 'vram',
        label: 'VRAM',
        value: vramValue,
        meta: gpu?.available ? undefined : gpu?.message || (status ? 'telemetry unavailable' : undefined),
        tone: gpu?.available ? 'normal' : 'muted',
        values: vramSeries,
      },
    ]
  }, [cpu, cpuValue, gpu, gpuValue, hasSwap, host, latestTimeline, loadValue, memory, ramValue, status, swapValue, timelineSamples, vramValue])

  const sampleCountLabel = `${timelineSamples.length} ${timelineSamples.length === 1 ? 'sample' : 'samples'}`
  const historyLabel = timelineSamples.length
    ? `${sampleCountLabel}${historyError ? ' · current status fallback' : ''}`
    : 'waiting for history'

  const handleCopySnapshot = async () => {
    if (!status) return
    setCopyStatus('')
    const copied = await copyTextToClipboard(JSON.stringify(status, null, 2))
    if (copied) {
      setCopyStatus('copied')
      window.setTimeout(() => setCopyStatus(''), 1400)
    } else {
      setCopyStatus('copy failed')
    }
  }

  return (
    <div className="system-status-view">
      <header className="system-hero">
        <div className="system-hero-title">
          <h1>Server cockpit</h1>
          <p>
            <strong>{host?.hostname || 'host'}</strong>
            <span>{host ? `up ${formatUptime(host.uptimeSeconds)}` : loading ? 'loading' : 'offline'}</span>
            <span>{status ? `updated ${formatTime(status.timestamp)}` : '--'}</span>
          </p>
        </div>
        <div className="system-hero-actions">
          <div
            className={`system-storage-meter ${(rootDisk?.usedPercent || 0) >= 85 ? 'is-warn' : ''}`}
            role="img"
            aria-label={`Storage ${formatPercent(rootDisk?.usedPercent)}`}
          >
            <span className="system-storage-label">Storage {rootDisk?.mount || ''}</span>
            <span className="system-storage-figures">
              <strong>{storageValue}</strong>
              <b>{formatPercent(rootDisk?.usedPercent)}</b>
            </span>
            <span className="system-storage-bar">
              <span style={{ width: `${readablePercent(rootDisk?.usedPercent) || 0}%` }} />
            </span>
          </div>
          <button type="button" className="system-icon-button" onClick={refresh} title="Refresh status" aria-label="Refresh status">
            <RefreshCw size={16} />
          </button>
          <button
            type="button"
            className={`system-icon-button ${paused ? 'active' : ''}`}
            onClick={() => setPaused(value => !value)}
            title={paused ? 'Resume polling' : 'Pause polling'}
            aria-label={paused ? 'Resume polling' : 'Pause polling'}
          >
            {paused ? <Play size={16} /> : <Pause size={16} />}
          </button>
          <button type="button" className="system-icon-button" onClick={handleCopySnapshot} disabled={!status} title="Copy status JSON" aria-label="Copy status JSON">
            <Copy size={16} />
          </button>
          {copyStatus && <span className="system-copy-status">{copyStatus}</span>}
        </div>
      </header>

      {error && <div className="system-error" role="alert">{error}</div>}

      <InstrumentBoard
        instruments={instruments}
        samples={timelineSamples}
        historyError={historyError}
        historyLabel={historyLabel}
      />

      <WarningPanel warnings={warnings} />
    </div>
  )
}

export default SystemStatusView
