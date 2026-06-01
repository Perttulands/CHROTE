import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Copy, Pause, Play, RefreshCw, Trash2 } from 'lucide-react'
import {
  SystemApiError,
  getSystemStatus,
  type SystemDiskStatus,
  type SystemGPUStatus,
  type SystemStatus,
  type SystemWarning,
} from '../../services/systemClient'

const POLL_MS = 2000
const MAX_HISTORY_POINTS = 300

interface StatusSample {
  at: number
  cpuPercent: number | null
  memoryPercent: number
  loadPercent: number
  rxBytesPerSecond: number | null
  txBytesPerSecond: number | null
}

interface PreviousSnapshot {
  status: SystemStatus
  at: number
}

function clampPercent(value: number) {
  return Math.max(0, Math.min(100, value))
}

function formatPercent(value: number | null | undefined) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  return `${value.toFixed(value >= 10 ? 0 : 1)}%`
}

function formatBytes(bytes: number | undefined) {
  if (!bytes || bytes <= 0) return '--'
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
  if (typeof bytesPerSecond !== 'number' || Number.isNaN(bytesPerSecond)) return '--'
  return `${formatBytes(bytesPerSecond)}/s`
}

function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
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

function cpuUsage(previous: SystemStatus | null, next: SystemStatus) {
  if (!previous) return null
  const totalDelta = next.cpu.totalTicks - previous.cpu.totalTicks
  const idleDelta = next.cpu.idleTicks - previous.cpu.idleTicks
  if (totalDelta <= 0 || idleDelta < 0) return null
  return clampPercent((1 - idleDelta / totalDelta) * 100)
}

function totalNetworkBytes(status: SystemStatus) {
  return status.network.reduce(
    (total, item) => ({
      rx: total.rx + item.rxBytes,
      tx: total.tx + item.txBytes,
    }),
    { rx: 0, tx: 0 }
  )
}

function networkRate(previous: PreviousSnapshot | null, next: SystemStatus, at: number) {
  if (!previous) return { rxBytesPerSecond: null, txBytesPerSecond: null }
  const seconds = (at - previous.at) / 1000
  if (seconds <= 0) return { rxBytesPerSecond: null, txBytesPerSecond: null }

  const prevBytes = totalNetworkBytes(previous.status)
  const nextBytes = totalNetworkBytes(next)
  return {
    rxBytesPerSecond: Math.max(0, (nextBytes.rx - prevBytes.rx) / seconds),
    txBytesPerSecond: Math.max(0, (nextBytes.tx - prevBytes.tx) / seconds),
  }
}

function diskLabel(disk?: SystemDiskStatus) {
  if (!disk) return '--'
  return `${formatBytes(disk.usedBytes)} / ${formatBytes(disk.totalBytes)}`
}

function gpuLabel(gpu?: SystemGPUStatus) {
  if (!gpu) return 'No GPU data'
  if (!gpu.available) return gpu.message || 'Unavailable'
  if (typeof gpu.utilizationPercent === 'number') {
    return `${gpu.name || 'GPU'} at ${formatPercent(gpu.utilizationPercent)}`
  }
  return gpu.name || 'GPU available'
}

function gpuMemoryLabel(gpu?: SystemGPUStatus) {
  if (!gpu?.available || !gpu.memoryTotalBytes) return ''
  return `${formatBytes(gpu.memoryUsedBytes)} / ${formatBytes(gpu.memoryTotalBytes)}`
}

function buildWarnings(status: SystemStatus | null, rootDisk?: SystemDiskStatus): SystemWarning[] {
  if (!status) return []
  const warnings = [...(status.warnings || [])]
  if (status.memory.usedPercent >= 90) {
    warnings.push({ code: 'MEMORY_HIGH', message: `Memory use is ${formatPercent(status.memory.usedPercent)}` })
  }
  if (rootDisk && rootDisk.usedPercent >= 85) {
    warnings.push({ code: 'DISK_HIGH', message: `${rootDisk.mount} disk use is ${formatPercent(rootDisk.usedPercent)}` })
  }
  const loadPercent = status.cpu.cores ? (status.host.load1 / status.cpu.cores) * 100 : 0
  if (loadPercent >= 100) {
    warnings.push({ code: 'LOAD_HIGH', message: `1m load is ${status.host.load1.toFixed(2)} across ${status.cpu.cores} cores` })
  }
  return warnings
}

function MetricCard({
  label,
  value,
  detail,
  tone = 'normal',
}: {
  label: string
  value: string
  detail?: string
  tone?: 'normal' | 'warn' | 'muted'
}) {
  return (
    <section className={`system-metric system-metric-${tone}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      {detail && <small>{detail}</small>}
    </section>
  )
}

function StatusGraph({ history }: { history: StatusSample[] }) {
  const width = 680
  const height = 210
  const padX = 28
  const padY = 20
  const chartWidth = width - padX * 2
  const chartHeight = height - padY * 2

  const series = [
    { key: 'cpuPercent' as const, label: 'CPU', color: '#58a6ff' },
    { key: 'memoryPercent' as const, label: 'MEM', color: '#3fb950' },
    { key: 'loadPercent' as const, label: 'LOAD', color: '#f2cc60' },
  ]

  const xFor = (index: number) => {
    if (history.length <= 1) return padX
    return padX + (index / (history.length - 1)) * chartWidth
  }

  const yFor = (value: number) => padY + (1 - clampPercent(value) / 100) * chartHeight

  return (
    <section className="system-graph-panel" aria-label="Server metrics history">
      <div className="system-panel-header">
        <div>
          <h2>History</h2>
          <p>{history.length ? `${history.length} samples` : 'warming'}</p>
        </div>
        <div className="system-graph-legend">
          {series.map(item => (
            <span key={item.key}>
              <i style={{ background: item.color }} />
              {item.label}
            </span>
          ))}
        </div>
      </div>
      <svg className="system-graph" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="CPU memory and load history">
        {[0, 25, 50, 75, 100].map(value => (
          <g key={value}>
            <line
              x1={padX}
              x2={width - padX}
              y1={yFor(value)}
              y2={yFor(value)}
              className="system-graph-grid"
            />
            <text x={6} y={yFor(value) + 4} className="system-graph-label">{value}</text>
          </g>
        ))}
        {series.map(item => {
          const points = history
            .map((sample, index) => {
              const value = sample[item.key]
              if (value === null || Number.isNaN(value)) return ''
              return `${xFor(index).toFixed(1)},${yFor(value).toFixed(1)}`
            })
            .filter(Boolean)
            .join(' ')
          if (!points) return null
          return (
            <polyline
              key={item.key}
              className="system-graph-line"
              points={points}
              fill="none"
              stroke={item.color}
            />
          )
        })}
      </svg>
    </section>
  )
}

function SystemStatusView() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [history, setHistory] = useState<StatusSample[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [paused, setPaused] = useState(false)
  const [copyStatus, setCopyStatus] = useState('')
  const previousRef = useRef<PreviousSnapshot | null>(null)

  const refresh = useCallback(async () => {
    setError('')
    try {
      const next = await getSystemStatus()
      const at = Date.now()
      const previous = previousRef.current
      const cpuPercent = cpuUsage(previous?.status || null, next)
      const rates = networkRate(previous, next, at)
      const loadPercent = next.cpu.cores ? clampPercent((next.host.load1 / next.cpu.cores) * 100) : 0

      setStatus(next)
      setHistory(current => [
        ...current,
        {
          at,
          cpuPercent,
          memoryPercent: clampPercent(next.memory.usedPercent),
          loadPercent,
          rxBytesPerSecond: rates.rxBytesPerSecond,
          txBytesPerSecond: rates.txBytesPerSecond,
        },
      ].slice(-MAX_HISTORY_POINTS))
      previousRef.current = { status: next, at }
    } catch (err) {
      if (err instanceof SystemApiError) {
        setError(`${err.code}: ${err.message}`)
      } else if (err instanceof Error) {
        setError(err.message)
      } else {
        setError('System status request failed')
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    if (paused) return
    const interval = window.setInterval(refresh, POLL_MS)
    return () => window.clearInterval(interval)
  }, [paused, refresh])

  const latest = history[history.length - 1]
  const rootDisk = status?.disks.find(disk => disk.mount === '/') || status?.disks[0]
  const primaryGPU = status?.gpus.find(gpu => gpu.available) || status?.gpus[0]
  const warnings = useMemo(() => buildWarnings(status, rootDisk), [status, rootDisk])

  const handleClearHistory = () => {
    setHistory([])
  }

  const handleCopySnapshot = async () => {
    if (!status) return
    setCopyStatus('')
    try {
      await navigator.clipboard.writeText(JSON.stringify(status, null, 2))
      setCopyStatus('copied')
      window.setTimeout(() => setCopyStatus(''), 1400)
    } catch {
      setCopyStatus('copy failed')
    }
  }

  return (
    <div className="system-status-view">
      <div className="system-status-strip">
        <span className="system-status-pill">
          <strong>{status?.host.hostname || 'host'}</strong>
          <span>{status ? formatUptime(status.host.uptimeSeconds) : '--'}</span>
        </span>
        <span className="system-status-pill">
          <strong>load</strong>
          <span>{status ? `${status.host.load1.toFixed(2)} ${status.host.load5.toFixed(2)} ${status.host.load15.toFixed(2)}` : '--'}</span>
        </span>
        <span className="system-status-pill">
          <strong>updated</strong>
          <span>{formatTime(status?.timestamp)}</span>
        </span>
        <div className="system-status-actions">
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
          <button type="button" className="system-icon-button" onClick={handleClearHistory} title="Clear graph history" aria-label="Clear graph history">
            <Trash2 size={16} />
          </button>
          <button type="button" className="system-icon-button" onClick={handleCopySnapshot} disabled={!status} title="Copy status JSON" aria-label="Copy status JSON">
            <Copy size={16} />
          </button>
          {copyStatus && <span className="system-copy-status">{copyStatus}</span>}
        </div>
      </div>

      {error && <div className="system-error" role="alert">{error}</div>}

      <div className="system-status-body">
        <div className="system-metrics-grid" aria-live="polite">
          <MetricCard
            label="CPU"
            value={formatPercent(latest?.cpuPercent)}
            detail={status ? `${status.cpu.cores} cores` : loading ? 'loading' : undefined}
            tone={latest?.cpuPercent && latest.cpuPercent >= 85 ? 'warn' : 'normal'}
          />
          <MetricCard
            label="Memory"
            value={formatPercent(status?.memory.usedPercent)}
            detail={status ? `${formatBytes(status.memory.usedBytes)} / ${formatBytes(status.memory.totalBytes)}` : undefined}
            tone={status?.memory.usedPercent && status.memory.usedPercent >= 90 ? 'warn' : 'normal'}
          />
          <MetricCard
            label="Disk"
            value={formatPercent(rootDisk?.usedPercent)}
            detail={diskLabel(rootDisk)}
            tone={rootDisk?.usedPercent && rootDisk.usedPercent >= 85 ? 'warn' : 'normal'}
          />
          <MetricCard
            label="GPU"
            value={primaryGPU?.available ? formatPercent(primaryGPU.utilizationPercent) : 'offline'}
            detail={gpuMemoryLabel(primaryGPU) || gpuLabel(primaryGPU)}
            tone={primaryGPU?.available ? 'normal' : 'muted'}
          />
          <MetricCard
            label="Network"
            value={`${formatRate(latest?.rxBytesPerSecond)} down`}
            detail={`${formatRate(latest?.txBytesPerSecond)} up`}
          />
        </div>

        <StatusGraph history={history} />

        <section className="system-detail-grid">
          <div className="system-detail-panel">
            <div className="system-panel-header">
              <div>
                <h2>Disks</h2>
                <p>{status?.disks.length || 0} mounts</p>
              </div>
            </div>
            <div className="system-table">
              {(status?.disks || []).map(disk => (
                <div key={disk.mount} className="system-table-row">
                  <strong>{disk.mount}</strong>
                  <span>{formatPercent(disk.usedPercent)}</span>
                  <small>{diskLabel(disk)}</small>
                </div>
              ))}
              {status && status.disks.length === 0 && <div className="system-empty">No disk data</div>}
            </div>
          </div>

          <div className="system-detail-panel">
            <div className="system-panel-header">
              <div>
                <h2>GPU</h2>
                <p>{primaryGPU?.available ? primaryGPU.name : 'unavailable'}</p>
              </div>
            </div>
            <div className="system-gpu-detail">
              <strong>{gpuLabel(primaryGPU)}</strong>
              {primaryGPU?.available && (
                <div className="system-detail-chips">
                  {typeof primaryGPU.temperatureCelsius === 'number' && <span>{primaryGPU.temperatureCelsius.toFixed(0)} C</span>}
                  {typeof primaryGPU.powerWatts === 'number' && <span>{primaryGPU.powerWatts.toFixed(0)} W</span>}
                  {gpuMemoryLabel(primaryGPU) && <span>{gpuMemoryLabel(primaryGPU)}</span>}
                </div>
              )}
            </div>
          </div>

          <div className="system-detail-panel">
            <div className="system-panel-header">
              <div>
                <h2>Warnings</h2>
                <p>{warnings.length ? `${warnings.length} active` : 'clear'}</p>
              </div>
            </div>
            <div className="system-warning-list">
              {warnings.length === 0 ? (
                <div className="system-empty">No active warnings</div>
              ) : warnings.map((warning, index) => (
                <div key={`${warning.code}-${index}`} className="system-warning-row">
                  <strong>{warning.code}</strong>
                  <span>{warning.message}</span>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}

export default SystemStatusView
