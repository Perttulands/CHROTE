import { type CSSProperties, type PointerEvent as ReactPointerEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, Copy, Gauge, Pause, Play, RefreshCw } from 'lucide-react'
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

interface TimelineSeries {
  key: 'gpu' | 'vram' | 'memory' | 'swap' | 'cpu' | 'load'
  label: string
  values: Array<number | null>
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

function formatTime(raw?: string) {
  if (!raw) return '--'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  return date.toLocaleTimeString()
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
  if (typeof gpu.utilizationPercent === 'number') {
    return `${gpu.name || 'GPU'} · ${formatPercent(gpu.utilizationPercent)}`
  }
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

function SummaryCard({
  label,
  value,
  meta,
  tone = 'normal',
  gaugeValue,
}: {
  label: string
  value?: string
  meta?: string
  tone?: 'normal' | 'warn' | 'muted' | 'gpu' | 'vram' | 'memory' | 'swap' | 'cpu' | 'storage'
  gaugeValue: number | null | undefined
}) {
  const safe = readablePercent(gaugeValue)
  const gaugeDegrees = (safe || 0) * 3.6

  return (
    <section className={`system-summary-card system-summary-${tone}`}>
      <span
        className={`system-donut system-donut-${safe === null ? 'muted' : tone}`}
        role="img"
        aria-label={`${label} ${formatPercent(safe)}`}
        style={{ '--system-gauge-deg': `${gaugeDegrees}deg` } as CSSProperties}
      >
        <span>{formatPercent(safe)}</span>
      </span>
      <div className="system-summary-copy">
        <span className="system-summary-label">{label}</span>
        {value && <strong>{value}</strong>}
        {meta && <em>{meta}</em>}
      </div>
    </section>
  )
}

function buildTimelineSeries(samples: TimelineSample[]): TimelineSeries[] {
  const gpuValues = samples.map(sample => sample.gpuPercent)
  const vramValues = samples.map(sample => sample.vramPercent)
  const ramValues = samples.map(sample => sample.ramPercent)
  const swapValues = samples.map(sample => sample.swapPercent)
  const cpuValues = samples.map(sample => sample.cpuPercent)
  const loadValues = samples.map(sample => sample.loadPercent)

  return [
    {
      key: 'gpu',
      label: 'GPU',
      values: gpuValues,
    },
    {
      key: 'vram',
      label: 'VRAM',
      values: vramValues,
    },
    {
      key: 'memory',
      label: 'RAM',
      values: ramValues,
    },
    {
      key: 'swap',
      label: 'Swap',
      values: swapValues,
    },
    {
      key: 'cpu',
      label: 'CPU',
      values: cpuValues,
    },
    {
      key: 'load',
      label: 'Load',
      values: loadValues,
    },
  ]
}

function TimelineGraph({ samples, historyError, active }: { samples: TimelineSample[]; historyError?: string; active: boolean }) {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)
  const series = buildTimelineSeries(samples)
  const labelWidth = 154
  const rowHeight = 64
  const rowGap = 0
  const axisHeight = 24
  const sampleSpacing = 5
  const barWidth = 4
  const innerWidth = Math.max(900, Math.min(2960, Math.max(samples.length - 1, 1) * sampleSpacing))
  const width = labelWidth + innerWidth + 16
  const finiteTimes = samples.map(sample => sample.at).filter(Number.isFinite)
  const startTime = finiteTimes.length ? Math.min(...finiteTimes) : 0
  const endTime = finiteTimes.length ? Math.max(...finiteTimes) : startTime
  const timeRange = endTime - startTime
  const xFor = (index: number) => {
    const at = samples[index]?.at
    if (!Number.isFinite(at) || timeRange <= 0) return innerWidth
    return ((at - startTime) / timeRange) * innerWidth
  }
  const chartTop = 8
  const chartBottom = rowHeight - 7
  const yFor = (value: number) => chartTop + (100 - clampPercent(value)) / 100 * (chartBottom - chartTop)
  const barXFor = (index: number) => Math.max(0, Math.min(innerWidth - barWidth, xFor(index)))
  const tickIndexes = Array.from(new Set(samples.length > 0 ? [0, samples.length - 1] : []))
  const latestSampleAt = samples[samples.length - 1]?.at
  const sampleCountLabel = `${samples.length} ${samples.length === 1 ? 'sample' : 'samples'}`
  const historyLabel = samples.length ? `${sampleCountLabel}${historyError ? ' · current status fallback' : ''}` : 'waiting for history'
  const hoveredSample = hoveredIndex === null ? null : samples[hoveredIndex]
  const hoveredX = hoveredIndex === null ? null : xFor(hoveredIndex)

  const nearestSampleIndex = (localX: number) => {
    if (samples.length === 0) return null
    return samples.reduce((nearestIndex, _sample, index) => {
      const nearestDistance = Math.abs(xFor(nearestIndex) - localX)
      const distance = Math.abs(xFor(index) - localX)
      return distance < nearestDistance ? index : nearestIndex
    }, 0)
  }

  const handleStripPointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    if (bounds.width <= 0) return
    const localX = Math.max(0, Math.min(innerWidth, (event.clientX - bounds.left) / bounds.width * innerWidth))
    setHoveredIndex(nearestSampleIndex(localX))
  }

  const clearHover = () => setHoveredIndex(null)

  useEffect(() => {
    const node = scrollRef.current
    if (!node) return
    const scrollToLatest = () => {
      node.scrollLeft = Math.max(0, node.scrollWidth - node.clientWidth)
    }
    scrollToLatest()
    const frame = window.requestAnimationFrame(scrollToLatest)
    const timeout = window.setTimeout(scrollToLatest, 60)
    return () => {
      window.cancelAnimationFrame(frame)
      window.clearTimeout(timeout)
    }
  }, [samples.length, latestSampleAt, width, active])

  return (
    <section className="system-panel system-timeline-panel" aria-labelledby="system-timeline-title">
      <div className="system-panel-header">
        <div>
          <h2 id="system-timeline-title">History</h2>
          <p>{historyLabel}</p>
        </div>
        {hoveredSample && (
          <div className="system-history-readout" role="status" aria-label="History sample">
            <strong>{formatTime(hoveredSample.timestamp)}</strong>
            {series.map(item => (
              <span key={`readout-${item.key}`}>
                <b>{item.label.toUpperCase()}</b> {formatPercent(item.values[hoveredIndex ?? 0])}
              </span>
            ))}
          </div>
        )}
      </div>
      {historyError && <div className="system-timeline-note" role="status">History unavailable: {historyError}</div>}
      <div ref={scrollRef} className="system-timeline-scroll" aria-label="Scrollable server telemetry history" tabIndex={0}>
        <div className="system-tui-chart" style={{ width }}>
          <div className="system-tui-time-axis" style={{ gridTemplateColumns: `${labelWidth}px ${innerWidth}px` }}>
            <span />
            <svg viewBox={`0 0 ${innerWidth} ${axisHeight}`} width={innerWidth} height={axisHeight} aria-hidden="true">
              {tickIndexes.map(index => {
                const tickX = xFor(index)
                const isFirstTick = index === 0
                const isLastTick = index === samples.length - 1
                return (
                  <g key={`axis-${index}`}>
                    <line className="system-tui-tick" x1={tickX} x2={tickX} y1="4" y2={axisHeight - 12} />
                    <text
                      className="system-tui-time"
                      x={tickX}
                      dx={isFirstTick ? 4 : isLastTick ? -4 : 0}
                      y={axisHeight - 2}
                      textAnchor={isFirstTick ? 'start' : isLastTick ? 'end' : 'middle'}
                    >
                      {formatTime(samples[index]?.timestamp)}
                    </text>
                  </g>
                )
              })}
            </svg>
          </div>
          <div className="system-tui-rows" style={{ gap: rowGap }}>
            {samples.length === 0 && <div className="system-timeline-empty">Waiting for history</div>}
            {series.map(item => {
              return (
                <div key={item.key} className={`system-tui-row system-tui-row-${item.key}`} style={{ gridTemplateColumns: `${labelWidth}px ${innerWidth}px` }}>
                  <div className="system-tui-row-label">
                    <span>{item.label}</span>
                  </div>
                  <svg
                    className="system-tui-strip"
                    role="img"
                    aria-label={`${item.label} history`}
                    viewBox={`0 0 ${innerWidth} ${rowHeight}`}
                    width={innerWidth}
                    height={rowHeight}
                    onPointerMove={handleStripPointerMove}
                    onPointerLeave={clearHover}
                  >
                    <title>{item.label} history</title>
                    <rect className="system-tui-bg" x="0" y="0" width={innerWidth} height={rowHeight} />
                    {tickIndexes.map(index => (
                      <line key={`${item.key}-tick-${index}`} className="system-tui-tick" x1={xFor(index)} x2={xFor(index)} y1="0" y2={rowHeight} />
                    ))}
                    {item.values.map((value, index) => {
                      if (typeof value !== 'number' || Number.isNaN(value)) return null
                      const y = yFor(value)
                      const hoverLabel = `${item.label} ${formatPercent(value)} · ${formatTime(samples[index]?.timestamp)}`
                      return (
                        <g key={`${item.key}-bar-${index}`} className="system-tui-bar-point" aria-label={hoverLabel}>
                          <rect
                            className={`system-tui-bar system-tui-bar-${item.key}${hoveredIndex === index ? ' system-tui-bar-hover' : ''}`}
                            x={barXFor(index)}
                            y={y}
                            width={barWidth}
                            height={Math.max(2, chartBottom - y)}
                          />
                        </g>
                      )
                    })}
                    {hoveredX !== null && (
                      <line className="system-tui-crosshair" x1={hoveredX} x2={hoveredX} y1="0" y2={rowHeight} />
                    )}
                  </svg>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </section>
  )
}

function WarningPanel({ warnings }: { warnings: SystemWarning[] }) {
  if (warnings.length === 0) return null
  return (
    <section className="system-panel system-warning-panel" aria-label="Warnings">
      <div className="system-panel-header">
        <div>
          <h2>Warnings</h2>
          <p>{warnings.length} active</p>
        </div>
      </div>
      <div className="system-warning-list">
        {warnings.map((warning, index) => (
          <div key={`${warning.code}-${index}`} className="system-warning-row">
            <AlertTriangle size={15} />
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
  const gpuSource = gpuSourceLabel(gpu)
  const gpuMemory = gpuMemoryLabel(gpu)
  const memory = status?.memory
  const hasSwap = (memory?.swapTotalBytes || 0) > 0
  const cpuValue = cpu ? `${cpu.cores} cores` : status ? 'CPU unavailable' : loading ? 'loading' : undefined
  const cpuMeta = host ? `load ${host.load1.toFixed(2)} / ${host.load5.toFixed(2)} / ${host.load15.toFixed(2)}` : undefined
  const gpuValue = gpu?.available ? undefined : gpuLabel(gpu)
  const gpuMeta = gpu?.available ? undefined : gpu?.message || 'telemetry unavailable'
  const vramValue = gpu?.available && gpuMemory ? gpuMemory : status ? 'VRAM unavailable' : loading ? 'loading' : undefined
  const vramMeta = gpu?.available ? [gpu.name, gpuSource].filter(Boolean).join(' · ') : undefined
  const ramValue = memory ? `${formatBytes(memory.usedBytes)} / ${formatBytes(memory.totalBytes)}` : status ? 'memory unavailable' : loading ? 'loading' : undefined
  const ramMeta = memory ? `free ${formatBytes(memory.freeBytes)} · available ${formatBytes(memory.availableBytes)}` : undefined
  const swapValue = hasSwap ? `${formatBytes(memory?.swapUsedBytes)} / ${formatBytes(memory?.swapTotalBytes)}` : status ? memory ? 'swap not configured' : 'swap unavailable' : loading ? 'loading' : undefined
  const storageValue = rootDisk ? diskLabel(rootDisk) : status ? 'storage unavailable' : loading ? 'loading' : undefined
  const timelineSamples = useMemo(() => buildTimelineSamples(historySamples), [historySamples])
  const latestTimeline = timelineSamples[timelineSamples.length - 1]
  const warnings = useMemo(() => buildWarnings(status, rootDisk, latestTimeline), [status, rootDisk, latestTimeline])

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
            <span>{host ? `${formatUptime(host.uptimeSeconds)} uptime` : loading ? 'loading' : 'offline'}</span>
            <span>{status ? `updated ${formatTime(status.timestamp)}` : '--'}</span>
          </p>
        </div>
        <div className="system-hero-actions">
          <span className="system-load-pill">
            <Gauge size={14} />
            load {host ? `${host.load1.toFixed(2)} / ${host.load5.toFixed(2)} / ${host.load15.toFixed(2)}` : '--'}
          </span>
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

      <div className="system-status-body">
        <div className="system-summary-grid">
          <SummaryCard
            label="CPU"
            value={cpuValue}
            meta={cpuMeta}
            gaugeValue={latestTimeline?.cpuPercent}
            tone={(latestTimeline?.cpuPercent || 0) >= 85 ? 'warn' : 'cpu'}
          />
          <SummaryCard
            label="GPU"
            value={gpuValue}
            meta={gpuMeta}
            gaugeValue={gpu?.available ? gpu.utilizationPercent : null}
            tone={gpu?.available ? 'gpu' : 'muted'}
          />
          <SummaryCard
            label="VRAM"
            value={vramValue}
            meta={vramMeta}
            gaugeValue={gpuMemoryPercent(gpu)}
            tone={gpu?.available ? 'vram' : 'muted'}
          />
          <SummaryCard
            label="RAM"
            value={ramValue}
            meta={ramMeta}
            gaugeValue={memory?.usedPercent}
            tone={(memory?.usedPercent || 0) >= 90 ? 'warn' : 'memory'}
          />
          <SummaryCard
            label="SWAP"
            value={swapValue}
            meta={hasSwap ? [swapActivityMeta(latestTimeline?.swapActivityBytesPerSecond), (memory?.swapCachedBytes || 0) > 0 ? `${formatBytes(memory?.swapCachedBytes)} cached` : ''].filter(Boolean).join(' · ') : undefined}
            gaugeValue={hasSwap ? memory?.swapUsedPercent : null}
            tone={hasSwap && hasActiveSwapPressure(latestTimeline?.swapActivityBytesPerSecond) ? 'warn' : 'swap'}
          />
          <SummaryCard
            label="Storage"
            value={storageValue}
            gaugeValue={rootDisk?.usedPercent}
            tone={(rootDisk?.usedPercent || 0) >= 85 ? 'warn' : 'storage'}
          />
        </div>

        <div className="system-main-grid">
          <TimelineGraph samples={timelineSamples} historyError={historyError} active={active} />
          <WarningPanel warnings={warnings} />
        </div>
      </div>
    </div>
  )
}

export default SystemStatusView
