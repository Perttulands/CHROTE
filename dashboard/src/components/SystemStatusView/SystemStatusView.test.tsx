import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
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

/** Pulls the x coordinates out of a trace path so geometry can be asserted without a layout engine. */
function pathXCoordinates(container: HTMLElement, selector: string) {
  const node = container.querySelector(selector)
  const numbers = (node?.getAttribute('d') || '').match(/-?\d+(?:\.\d+)?/g) || []
  return numbers.map(Number).filter((_value, index) => index % 2 === 0)
}

/** Pulls the y coordinates out of a trace path to assert how a series was scaled. */
function pathYCoordinates(container: HTMLElement, selector: string) {
  const node = container.querySelector(selector)
  const numbers = (node?.getAttribute('d') || '').match(/-?\d+(?:\.\d+)?/g) || []
  return numbers.map(Number).filter((_value, index) => index % 2 === 1)
}

function stretchTrace(strip: HTMLElement, width = 900) {
  vi.spyOn(strip, 'getBoundingClientRect').mockReturnValue({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    right: width,
    bottom: 96,
    width,
    height: 96,
    toJSON: () => ({}),
  } as DOMRect)
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
    vi.useRealTimers()
  })

  // Behind another tab this view is the only thing keeping the history warm, so
  // it samples on mount and goes on sampling, just slower than the tab in front.
  it('samples on mount and keeps sampling while the Server tab is not the one on screen', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })

    render(<SystemStatusView active={false} />)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/status'))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/history'))
    const onMount = fetchMock.mock.calls.length

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000)
    })

    expect(fetchMock.mock.calls.length).toBeGreaterThan(onMount)
  })

  it('renders one instrument row per metric with its reading, detail and window stats', async () => {
    const { container } = render(<SystemStatusView active />)

    expect(await screen.findByRole('heading', { name: 'Server cockpit' })).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/status'))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/system/history'))

    expect(await screen.findByText('3 samples')).toBeInTheDocument()
    expect(container.querySelectorAll('.system-instrument')).toHaveLength(6)
    expect(container.querySelectorAll('.system-trace')).toHaveLength(6)

    expect(screen.getByRole('img', { name: 'CPU history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Load history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'RAM history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Swap history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'GPU history' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'VRAM history' })).toBeInTheDocument()

    expect(screen.getByRole('region', { name: /^CPU 30%/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /^Load 2\.6%/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /^RAM 25%/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /^Swap 13%/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /^GPU 17%/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /^VRAM 14%/ })).toBeInTheDocument()

    expect(screen.getByText('landmass')).toBeInTheDocument()
    expect(screen.getByText('up 12d 12h')).toBeInTheDocument()
    expect(screen.getByText('16 cores')).toBeInTheDocument()
    expect(screen.getByText('2.2 GB / 16 GB')).toBeInTheDocument()
    expect(screen.getByText('16 GB / 64 GB')).toBeInTheDocument()
    expect(screen.getByText('free 1.0 GB · available 48 GB')).toBeInTheDocument()
    expect(screen.getByText('1.0 GB / 8.0 GB')).toBeInTheDocument()
    expect(screen.getByText('0.42 / 0.73 / 0.62')).toBeInTheDocument()

    // Peak and average over the sampled window, not just the live tip.
    expect(screen.getByText('peak 17% · avg 12%')).toBeInTheDocument()

  })

  it('scales each row to its own peak so a quiet host is still readable', async () => {
    const { container } = render(<SystemStatusView active />)

    await screen.findByText('3 samples')

    // GPU peaks at 17%, so the axis tops out at 25% rather than a flat 0-100 baseline.
    const gpuScale = container.querySelector('.system-instrument-gpu .system-instrument-scale')
    expect(gpuScale).toHaveTextContent('25%')
    const gpuYs = pathYCoordinates(container, '.system-instrument-gpu .system-trace-line')
    expect(gpuYs[gpuYs.length - 1]).toBeCloseTo(100 - 17 / 25 * 100, 1)

    // A different peak gets a different ceiling, and the label always states it.
    expect(container.querySelector('.system-instrument-cpu .system-instrument-scale')).toHaveTextContent('40%')
    expect(container.querySelector('.system-instrument-swap .system-instrument-scale')).toHaveTextContent('20%')
  })

  it('floors the axis instead of magnifying an all-zero series', async () => {
    const idle = (timestamp: string) => statusResponse({
      timestamp,
      gpus: [baseGPU({ utilizationPercent: 0 })],
    })
    mockSystemResponses(idle('2026-06-28T07:20:00Z'), historyResponse([
      idle('2026-06-28T07:00:00Z'),
      idle('2026-06-28T07:10:00Z'),
    ]))

    const { container } = render(<SystemStatusView active />)

    await screen.findByText('3 samples')
    expect(container.querySelector('.system-instrument-gpu .system-instrument-scale')).toHaveTextContent('5%')
    const ys = pathYCoordinates(container, '.system-instrument-gpu .system-trace-line')
    expect(ys.every(y => y === 100)).toBe(true)
  })

  it('caps the axis at 100% once a series actually saturates', async () => {
    const busy = (timestamp: string, usedPercent: number) => statusResponse({
      timestamp,
      memory: baseMemory({ usedPercent }),
    })
    mockSystemResponses(busy('2026-06-28T07:20:00Z', 94), historyResponse([
      busy('2026-06-28T07:00:00Z', 88),
      busy('2026-06-28T07:10:00Z', 91),
    ]))

    const { container } = render(<SystemStatusView active />)

    await screen.findByText('3 samples')
    expect(container.querySelector('.system-instrument-memory .system-instrument-scale')).toHaveTextContent('100%')
    expect(container.querySelectorAll('.system-instrument-memory.system-tone-warn')).toHaveLength(1)
  })

  it('shows GPU thermals and power draw from the payload instead of discarding them', async () => {
    render(<SystemStatusView active />)

    expect(await screen.findByText('39 °C · 58 W · WSL nvidia-smi')).toBeInTheDocument()
    expect(screen.getByText('NVIDIA GeForce RTX 4070 Ti SUPER')).toBeInTheDocument()
  })

  it('keeps root storage as a header meter rather than a metric row', async () => {
    render(<SystemStatusView active />)

    const meter = await screen.findByRole('img', { name: 'Storage 28%' })
    expect(meter).toHaveTextContent('280 GB / 1000 GB')
    expect(meter).toHaveTextContent('Storage /')
    expect(screen.queryByRole('img', { name: 'Storage history' })).not.toBeInTheDocument()
  })

  it('stretches every trace across the full chart width regardless of sample count', async () => {
    const start = Date.parse('2026-06-28T02:20:00Z')
    const samples = Array.from({ length: 299 }, (_unused, index) => statusResponse({
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
    const strip = await screen.findByRole('img', { name: 'GPU history' })
    expect(strip).toHaveAttribute('preserveAspectRatio', 'none')
    expect(strip).toHaveAttribute('viewBox', '0 0 1000 100')
    expect(strip).not.toHaveAttribute('width')

    const xs = pathXCoordinates(container, '.system-instrument-gpu .system-trace-line')
    expect(xs).toHaveLength(300)
    expect(xs[0]).toBe(0)
    expect(xs[xs.length - 1]).toBe(1000)
  })

  it('renders a sparse history at the same full width as a dense one', async () => {
    const { container } = render(<SystemStatusView active />)

    await screen.findByText('3 samples')
    const xs = pathXCoordinates(container, '.system-instrument-memory .system-trace-line')
    expect(xs).toHaveLength(3)
    expect(xs[0]).toBe(0)
    expect(xs[xs.length - 1]).toBe(1000)
  })

  it('positions telemetry by timestamp gaps instead of sample index', async () => {
    const current = statusResponse({ timestamp: '2026-06-28T07:20:00Z' })
    mockSystemResponses(current, historyResponse([
      statusResponse({ timestamp: '2026-06-28T07:00:00Z', gpus: [baseGPU({ utilizationPercent: 8 })] }),
      statusResponse({ timestamp: '2026-06-28T07:19:00Z', gpus: [baseGPU({ utilizationPercent: 12 })] }),
    ]))

    const { container } = render(<SystemStatusView active />)

    await screen.findByText(/3 samples/)
    const xs = pathXCoordinates(container, '.system-instrument-gpu .system-trace-line')
    expect(xs).toHaveLength(3)
    expect(xs[1] - xs[0]).toBeGreaterThan((xs[2] - xs[1]) * 10)
  })

  it('scrubs every row to the hovered sample with one synchronized crosshair', async () => {
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
    expect(screen.queryByLabelText('History sample')).not.toBeInTheDocument()

    const strip = await screen.findByRole('img', { name: 'GPU history' })
    stretchTrace(strip)
    fireEvent.pointerMove(strip, { clientX: 451, clientY: 20 })

    const readout = await screen.findByLabelText('History sample')
    expect(readout).toHaveTextContent(new Date('2026-06-28T07:10:00Z').toLocaleTimeString())
    expect(container.querySelectorAll('.system-trace-crosshair')).toHaveLength(6)
    expect(container.querySelectorAll('.system-instrument-reading.is-scrubbed')).toHaveLength(6)

    // Every row's number reports the hovered moment, not the live tip.
    expect(screen.getByRole('region', { name: /^GPU 12%/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /^CPU 30%/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /^Load 3\.1%/ })).toBeInTheDocument()

    fireEvent.pointerLeave(strip)
    expect(screen.queryByLabelText('History sample')).not.toBeInTheDocument()
    expect(container.querySelectorAll('.system-trace-crosshair')).toHaveLength(0)
    expect(container.querySelectorAll('.system-instrument-reading.is-scrubbed')).toHaveLength(0)
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
    expect(screen.getByRole('img', { name: 'Storage 28%' })).toBeInTheDocument()
    // A lone reading still gets a visible stem rather than vanishing into a zero-length path.
    expect(container.querySelectorAll('.system-instrument-gpu .system-trace-stem')).toHaveLength(1)
    expect(container.querySelectorAll('.system-instrument-gpu .system-trace-line')).toHaveLength(0)
  })

  it('leaves gaps in a series instead of bridging missing readings', async () => {
    mockSystemResponses(statusResponse({ timestamp: '2026-06-28T07:20:00Z' }), historyResponse([
      statusResponse({ timestamp: '2026-06-28T07:00:00Z', gpus: [baseGPU({ utilizationPercent: 8 })] }),
      statusResponse({ timestamp: '2026-06-28T07:05:00Z', gpus: [{ available: false, message: 'nvidia-smi busy' }] }),
      statusResponse({ timestamp: '2026-06-28T07:10:00Z', gpus: [baseGPU({ utilizationPercent: 12 })] }),
    ]))

    const { container } = render(<SystemStatusView active />)

    await screen.findByText(/4 samples/)
    expect(container.querySelectorAll('.system-instrument-gpu .system-trace-stem')).toHaveLength(1)
    expect(container.querySelectorAll('.system-instrument-gpu .system-trace-line')).toHaveLength(1)
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

    expect(await screen.findByRole('region', { name: /^Swap 99%/ })).toBeInTheDocument()
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

    const { container } = render(<SystemStatusView active />)

    expect(await screen.findByText('SWAP_ACTIVE')).toBeInTheDocument()
    expect(screen.getAllByText(/active swap I\/O/).length).toBeGreaterThan(0)
    expect(container.querySelectorAll('.system-tone-warn')).toHaveLength(1)
  })
})
