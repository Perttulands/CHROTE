import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SystemStatusView from './index'

const fetchMock = vi.fn()

function envelope(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ success: true, data }), { status: 200 }))
}

function baseMemory(overrides: Record<string, unknown> = {}) {
  return {
    totalBytes: 64 * 1024 ** 3,
    freeBytes: 1 * 1024 ** 3,
    availableBytes: 48 * 1024 ** 3,
    usedBytes: 16 * 1024 ** 3,
    usedPercent: 25,
    swapTotalBytes: 8 * 1024 ** 3,
    swapUsedBytes: 1 * 1024 ** 3,
    swapUsedPercent: 12.5,
    ...overrides,
  }
}

function baseGPU(overrides: Record<string, unknown> = {}) {
  return {
    available: true,
    name: 'NVIDIA GeForce RTX 4070 Ti SUPER',
    utilizationPercent: 17,
    memoryTotalBytes: 16 * 1024 ** 3,
    memoryUsedBytes: 2.2 * 1024 ** 3,
    temperatureCelsius: 39,
    powerWatts: 58,
    source: '/usr/lib/wsl/lib/nvidia-smi',
    ...overrides,
  }
}

function statusResponse(overrides: Record<string, unknown> = {}) {
  return {
    timestamp: '2026-06-28T07:20:00Z',
    host: { hostname: 'landmass', uptimeSeconds: 1080000, load1: 0.42, load5: 0.73, load15: 0.62 },
    cpu: { cores: 16, totalTicks: 2000, idleTicks: 1200 },
    memory: baseMemory(),
    disks: [
      { mount: '/', totalBytes: 1000 * 1024 ** 3, availableBytes: 720 * 1024 ** 3, usedBytes: 280 * 1024 ** 3, usedPercent: 28 },
      { mount: '/home', totalBytes: 1000 * 1024 ** 3, availableBytes: 720 * 1024 ** 3, usedBytes: 280 * 1024 ** 3, usedPercent: 28 },
      { mount: '/srv', totalBytes: 1000 * 1024 ** 3, availableBytes: 720 * 1024 ** 3, usedBytes: 280 * 1024 ** 3, usedPercent: 28 },
    ],
    network: [{ name: 'eth0', rxBytes: 10_000_000, txBytes: 4_000_000 }],
    gpus: [baseGPU()],
    warnings: [],
    ...overrides,
  }
}

function historyResponse(samples = [
  statusResponse({
    timestamp: '2026-06-28T07:00:00Z',
    host: { hostname: 'landmass', uptimeSeconds: 1078800, load1: 0.25, load5: 0.45, load15: 0.53 },
    cpu: { cores: 16, totalTicks: 1200, idleTicks: 960 },
    memory: baseMemory({ usedBytes: 14 * 1024 ** 3, usedPercent: 21.9, swapUsedBytes: 0.5 * 1024 ** 3, swapUsedPercent: 6.25 }),
    gpus: [baseGPU({ utilizationPercent: 8, memoryUsedBytes: 1.2 * 1024 ** 3 })],
  }),
  statusResponse({
    timestamp: '2026-06-28T07:10:00Z',
    host: { hostname: 'landmass', uptimeSeconds: 1079400, load1: 0.5, load5: 0.58, load15: 0.6 },
    cpu: { cores: 16, totalTicks: 1600, idleTicks: 1240 },
    memory: baseMemory({ usedBytes: 15 * 1024 ** 3, usedPercent: 23.4, swapUsedBytes: 0.75 * 1024 ** 3, swapUsedPercent: 9.4 }),
    gpus: [baseGPU({ utilizationPercent: 12, memoryUsedBytes: 1.8 * 1024 ** 3 })],
  }),
]) {
  return { limit: 288, samples }
}

function requestPath(input: RequestInfo | URL) {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.pathname
  return input.url
}

function mockSystemResponses(status = statusResponse(), history = historyResponse()) {
  fetchMock.mockImplementation((input: RequestInfo | URL) => {
    const path = requestPath(input)
    if (path.endsWith('/api/system/status')) return envelope(status)
    if (path.endsWith('/api/system/history')) return envelope(history)
    return Promise.resolve(new Response(JSON.stringify({ success: false, error: { code: 'NOT_FOUND', message: path } }), { status: 404 }))
  })
}

function mockHistoryFailure(status = statusResponse()) {
  fetchMock.mockImplementation((input: RequestInfo | URL) => {
    const path = requestPath(input)
    if (path.endsWith('/api/system/status')) return envelope(status)
    if (path.endsWith('/api/system/history')) {
      return Promise.resolve(new Response(JSON.stringify({
        success: false,
        error: { code: 'HISTORY_DOWN', message: 'history unavailable' },
      }), { status: 503 }))
    }
    return Promise.resolve(new Response(JSON.stringify({ success: false, error: { code: 'NOT_FOUND', message: path } }), { status: 404 }))
  })
}

function barXCoordinates(nodes: NodeListOf<SVGRectElement>) {
  return Array.from(nodes, node => Number(node.getAttribute('x')))
}

function barWidths(nodes: NodeListOf<SVGRectElement>) {
  return Array.from(nodes, node => Number(node.getAttribute('width')))
}

describe('SystemStatusView', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    mockSystemResponses()
    vi.stubGlobal('fetch', fetchMock)
    Object.assign(navigator, { clipboard: { writeText: vi.fn() } })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders backend-backed TUI telemetry strips with storage state', async () => {
    const { container } = render(<SystemStatusView active />)

    expect(await screen.findByRole('heading', { name: 'Server cockpit' })).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/status'))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/history'))

    expect(await screen.findByRole('heading', { name: 'History' })).toBeInTheDocument()
    expect(screen.getByText('3 samples')).toBeInTheDocument()
    expect(screen.queryByRole('img', { name: 'Server telemetry timeline' })).not.toBeInTheDocument()
    expect(screen.queryByText(/TUI-style separated graphs/)).not.toBeInTheDocument()
    expect(screen.queryByText(/backend history/)).not.toBeInTheDocument()
    expect(screen.queryByText(/scroll sideways/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('list', { name: 'Telemetry legend' })).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'GPU history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'VRAM history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'RAM history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Swap history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'CPU history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Load history' })).toBeInTheDocument()
    expect(container.querySelectorAll('.system-tui-row')).toHaveLength(6)
    expect(container.querySelectorAll('.system-tui-strip')).toHaveLength(6)
    expect(container.querySelectorAll('.system-timeline-meta')).toHaveLength(0)
    expect(container.querySelectorAll('.system-timeline-hint')).toHaveLength(0)
    expect(container.querySelectorAll('.system-tui-guide')).toHaveLength(0)
    const timeLabels = container.querySelectorAll('.system-tui-time')
    expect(timeLabels[0]).toHaveAttribute('text-anchor', 'start')
    expect(timeLabels[timeLabels.length - 1]).toHaveAttribute('text-anchor', 'end')
    const scroll = screen.getByLabelText(/scrollable server telemetry history/i)
    expect(scroll).toHaveClass('system-timeline-scroll')
    expect(container.querySelectorAll('.system-donut')).toHaveLength(6)
    expect(container.querySelectorAll('.system-summary-label svg')).toHaveLength(0)
    expect(container.querySelectorAll('.system-summary-card small')).toHaveLength(0)
    expect(screen.getByRole('img', { name: /GPU 17%/i })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /VRAM 14%/i })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /RAM 25%/i })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /SWAP 13%/i })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /Storage 28%/i })).toBeInTheDocument()
    expect(container.querySelectorAll('.system-tui-bar-gpu').length).toBeGreaterThan(0)
    expect(container.querySelectorAll('.system-tui-bar-vram').length).toBeGreaterThan(0)
    expect(container.querySelectorAll('.system-tui-bar-memory').length).toBeGreaterThan(0)
    expect(container.querySelectorAll('.system-tui-bar-swap').length).toBeGreaterThan(0)
    expect(container.querySelectorAll('.system-tui-bar-cpu').length).toBeGreaterThan(0)
    expect(container.querySelectorAll('.system-tui-bar-load').length).toBeGreaterThan(0)

    expect(screen.getByText('landmass')).toBeInTheDocument()
    expect(screen.queryByText('nominal')).not.toBeInTheDocument()
    expect(screen.getAllByText(/NVIDIA GeForce RTX 4070 Ti SUPER/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/WSL nvidia-smi/).length).toBeGreaterThan(0)
    expect(screen.getByText('16 cores')).toBeInTheDocument()
    expect(screen.getByText('2.2 GB / 16 GB')).toBeInTheDocument()
    expect(screen.getByText('16 GB / 64 GB')).toBeInTheDocument()
    expect(screen.getByText('free 1.0 GB · available 48 GB')).toBeInTheDocument()
    expect(screen.getByText('1.0 GB / 8.0 GB')).toBeInTheDocument()
    expect(screen.getByText('280 GB / 1000 GB')).toBeInTheDocument()
    expect(screen.queryByText(/occupied/)).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Storage' })).not.toBeInTheDocument()
    expect(container.querySelectorAll('.system-storage-row')).toHaveLength(0)
    expect(container.querySelectorAll('.system-tui-row-label strong')).toHaveLength(0)
    expect(container.querySelectorAll('.system-tui-row-label small')).toHaveLength(0)
    expect(screen.queryByText('0-100')).not.toBeInTheDocument()

    expect(screen.queryByRole('img', { name: /CPU utilization trend/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('img', { name: /GPU utilization trend/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Signals' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/server signal history/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/network detail/i)).not.toBeInTheDocument()
    expect(container.querySelectorAll('.system-sparkline')).toHaveLength(0)
    expect(container.querySelectorAll('.system-signal-row')).toHaveLength(0)
    expect(container.querySelectorAll('.system-graph-panel')).toHaveLength(0)
    expect(container.querySelectorAll('.system-graph-line-memory')).toHaveLength(0)
    expect(container.querySelectorAll('.system-timeline-lane')).toHaveLength(0)
    expect(container.querySelectorAll('.system-timeline-direct-label')).toHaveLength(0)
    expect(container.querySelectorAll('.system-timeline-path-cpu-load')).toHaveLength(0)
    expect(container.querySelectorAll('.system-timeline-path-gpu')).toHaveLength(0)
    expect(container.querySelectorAll('.system-health')).toHaveLength(0)
    expect(screen.queryByText(/attention/i)).not.toBeInTheDocument()
    expect(container.querySelectorAll('.system-tui-path')).toHaveLength(0)
    expect(container.querySelectorAll('.system-tui-dot')).toHaveLength(0)
  })

  it('packs wide telemetry histories into tight fixed-width bars', async () => {
    const start = Date.parse('2026-06-28T02:20:00Z')
    const samples = Array.from({ length: 299 }, (_, index) => statusResponse({
      timestamp: new Date(start + index * 60_000).toISOString(),
      host: { hostname: 'landmass', uptimeSeconds: 1060000 + index * 60, load1: 0.25 + index / 1000, load5: 0.45, load15: 0.53 },
      cpu: { cores: 16, totalTicks: 1200 + index * 100, idleTicks: 960 + index * 60 },
      memory: baseMemory({ usedPercent: 20 + index / 20 }),
      gpus: [baseGPU({ utilizationPercent: 8 + index / 20 })],
    }))
    const current = statusResponse({
      timestamp: new Date(start + 299 * 60_000).toISOString(),
      cpu: { cores: 16, totalTicks: 1200 + 299 * 100, idleTicks: 960 + 299 * 60 },
    })
    mockSystemResponses(current, historyResponse(samples))

    const { container } = render(<SystemStatusView active />)

    await screen.findByText(/300 samples/)
    const timeline = await screen.findByRole('img', { name: 'GPU history' })
    const width = Number(timeline.getAttribute('width'))
    expect(width).toBeGreaterThanOrEqual(1400)
    expect(width).toBeLessThanOrEqual(1700)

    const bars = container.querySelectorAll<SVGRectElement>('.system-tui-bar-gpu')
    const [firstWidth] = barWidths(bars)
    const xs = barXCoordinates(bars)
    expect(firstWidth).toBeLessThanOrEqual(7.2)
    expect(xs[2] - xs[1]).toBeLessThanOrEqual(firstWidth + 1)
  })

  it('uses one synchronized crosshair readout instead of per-bar tooltip clutter', async () => {
    mockSystemResponses(statusResponse({ timestamp: '2026-06-28T07:20:00Z' }), historyResponse([
      statusResponse({
        timestamp: '2026-06-28T07:00:00Z',
        host: { hostname: 'landmass', uptimeSeconds: 1078800, load1: 0.25, load5: 0.45, load15: 0.53 },
        cpu: { cores: 16, totalTicks: 1200, idleTicks: 960 },
        gpus: [baseGPU({ utilizationPercent: 8 })],
      }),
      statusResponse({
        timestamp: '2026-06-28T07:10:00Z',
        host: { hostname: 'landmass', uptimeSeconds: 1079400, load1: 0.5, load5: 0.58, load15: 0.6 },
        cpu: { cores: 16, totalTicks: 1600, idleTicks: 1240 },
        gpus: [baseGPU({ utilizationPercent: 12 })],
      }),
    ]))

    const { container } = render(<SystemStatusView active />)

    await screen.findByText(/3 samples/)
    expect(container.querySelectorAll('.system-tui-bar-point title')).toHaveLength(0)
    expect(screen.queryByLabelText('History sample')).not.toBeInTheDocument()

    const strip = await screen.findByRole('img', { name: 'GPU history' })
    vi.spyOn(strip, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 900,
      bottom: 64,
      width: 900,
      height: 64,
      toJSON: () => ({}),
    } as DOMRect)

    fireEvent.pointerMove(strip, { clientX: 451, clientY: 20 })

    const readout = await screen.findByLabelText('History sample')
    expect(readout).toHaveTextContent(new Date('2026-06-28T07:10:00Z').toLocaleTimeString())
    expect(readout).toHaveTextContent('GPU 12%')
    expect(readout).toHaveTextContent('VRAM 14%')
    expect(readout).toHaveTextContent('RAM 25%')
    expect(readout).toHaveTextContent('SWAP 13%')
    expect(readout).toHaveTextContent('CPU 30%')
    expect(readout).toHaveTextContent('LOAD 3.1%')
    expect(container.querySelectorAll('.system-tui-crosshair')).toHaveLength(6)
    expect(container.querySelectorAll('.system-tui-bar-hover')).toHaveLength(6)

    fireEvent.pointerLeave(strip)
    expect(screen.queryByLabelText('History sample')).not.toBeInTheDocument()
    expect(container.querySelectorAll('.system-tui-crosshair')).toHaveLength(0)
  })

  it('positions telemetry by timestamp gaps instead of sample index', async () => {
    const current = statusResponse({ timestamp: '2026-06-28T07:20:00Z' })
    mockSystemResponses(current, historyResponse([
      statusResponse({ timestamp: '2026-06-28T07:00:00Z', gpus: [baseGPU({ utilizationPercent: 8 })] }),
      statusResponse({ timestamp: '2026-06-28T07:19:00Z', gpus: [baseGPU({ utilizationPercent: 12 })] }),
    ]))

    const { container } = render(<SystemStatusView active />)

    await screen.findByText(/3 samples/)
    await screen.findByRole('img', { name: 'GPU history' })
    const xs = barXCoordinates(container.querySelectorAll('.system-tui-bar-gpu'))
    expect(xs).toHaveLength(3)
    expect(xs[1] - xs[0]).toBeGreaterThan((xs[2] - xs[1]) * 10)
  })

  it('renders current status with a non-fatal history note when history fails', async () => {
    mockHistoryFailure()
    const { container } = render(<SystemStatusView active />)

    expect(await screen.findByText('landmass')).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/status'))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/history'))
    expect(screen.getByText('1 sample · current status fallback')).toBeInTheDocument()
    expect(await screen.findByRole('status')).toHaveTextContent('History unavailable: HISTORY_DOWN: history unavailable')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: /Storage 28%/i })).toBeInTheDocument()
    expect(container.querySelectorAll('.system-tui-bar-gpu')).toHaveLength(1)
  })

  it('keeps the Server tab alive when a status payload has no disks', async () => {
    const status = statusResponse({ disks: undefined })
    mockSystemResponses(status, historyResponse([status]))

    render(<SystemStatusView active />)

    expect(await screen.findByText('landmass')).toBeInTheDocument()
    expect(screen.getByText('storage unavailable')).toBeInTheDocument()
  })

  it('keeps the Server tab alive when a status payload has no GPU array', async () => {
    const status = statusResponse({ gpus: undefined })
    mockSystemResponses(status, historyResponse([status]))

    render(<SystemStatusView active />)

    expect(await screen.findByText('landmass')).toBeInTheDocument()
    expect(screen.getByText('No GPU data')).toBeInTheDocument()
  })

  it('keeps the Server tab alive when a status payload has no memory block', async () => {
    const status = statusResponse({ memory: undefined })
    mockSystemResponses(status, historyResponse([status]))

    render(<SystemStatusView active />)

    expect(await screen.findByText('landmass')).toBeInTheDocument()
    expect(screen.getByText('memory unavailable')).toBeInTheDocument()
    expect(screen.getByText('swap unavailable')).toBeInTheDocument()
  })

  it('keeps the Server tab alive when a status payload has no CPU block', async () => {
    const status = statusResponse({ cpu: undefined })
    mockSystemResponses(status, historyResponse([status]))

    render(<SystemStatusView active />)

    expect(await screen.findByText('landmass')).toBeInTheDocument()
    expect(screen.getByText('CPU unavailable')).toBeInTheDocument()
  })

  it('shows offline GPU state without burying the reason', async () => {
    const status = statusResponse({
      gpus: [{ available: false, message: 'nvidia-smi not found' }],
    })
    mockSystemResponses(status, historyResponse([status]))

    render(<SystemStatusView active />)

    await waitFor(() => expect(screen.getAllByText('GPU unavailable').length).toBeGreaterThan(0))
    expect(screen.getAllByText('nvidia-smi not found').length).toBeGreaterThan(0)
  })

  it('treats high stale swap as occupied memory instead of active pressure', async () => {
    const status = statusResponse({
      memory: baseMemory({
        swapUsedBytes: 7.9 * 1024 ** 3,
        swapUsedPercent: 98.8,
        swapInPages: 1_000,
        swapOutPages: 2_000,
        pageSizeBytes: 4096,
      }),
    })
    mockSystemResponses(status, historyResponse([
      statusResponse({
        timestamp: '2026-06-28T07:10:00Z',
        memory: baseMemory({
          swapUsedBytes: 7.8 * 1024 ** 3,
          swapUsedPercent: 97.5,
          swapInPages: 1_000,
          swapOutPages: 2_000,
          pageSizeBytes: 4096,
        }),
      }),
      status,
    ]))

    render(<SystemStatusView active />)

    expect(await screen.findByRole('img', { name: /SWAP 99%/i })).toBeInTheDocument()
    expect(screen.getByText('7.9 GB / 8.0 GB')).toBeInTheDocument()
    expect(screen.getByText('0 B/s active swap I/O')).toBeInTheDocument()
    expect(screen.queryByText('SWAP_HIGH')).not.toBeInTheDocument()
    expect(screen.queryByText('SWAP_ACTIVE')).not.toBeInTheDocument()
  })

  it('surfaces active swap churn as the warning, not swap occupancy alone', async () => {
    const previous = statusResponse({
      timestamp: '2026-06-28T07:10:00Z',
      memory: baseMemory({ swapUsedPercent: 65, swapInPages: 1_000, swapOutPages: 2_000, pageSizeBytes: 4096 }),
    })
    const current = statusResponse({
      timestamp: '2026-06-28T07:11:00Z',
      memory: baseMemory({ swapUsedPercent: 66, swapInPages: 3_000, swapOutPages: 20_000, pageSizeBytes: 4096 }),
    })
    mockSystemResponses(current, historyResponse([previous, current]))

    render(<SystemStatusView active />)

    expect(await screen.findByText('SWAP_ACTIVE')).toBeInTheDocument()
    expect(screen.getAllByText(/active swap I\/O/).length).toBeGreaterThan(0)
  })
})
