import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GasCityView from './index'

const fetchMock = vi.fn()

function envelope(data: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify({ success: true, data }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  }))
}

const observerFixture = {
  status: 'ok',
  checkedAt: '2026-05-26T10:00:00Z',
  health: {
    status: 'ok',
    version: 'dev',
    uptimeSeconds: 3600,
    citiesTotal: 1,
    citiesRunning: 1,
    startupReady: true,
    startupPhase: 'running',
  },
  cities: [{ name: 'gascity', running: true }],
  sessions: [
    {
      city: 'gascity',
      id: 'gc-1',
      title: 'planner',
      alias: 'planner',
      template: 'planner',
      state: 'active',
      provider: './bin/mock-agent',
      running: true,
      lastActive: '2026-05-26T10:01:00Z',
    },
  ],
  mail: { total: 8, unread: 5 },
  work: { open: 22, ready: 4, inProgress: 1, routed: 2, molecules: 3, wisps: 1, convoys: 2 },
  formulas: [{ city: 'gascity', name: 'mol-review-quorum', version: '2', runCount: 1 }],
  molecules: [{ city: 'gascity', id: 'gc-31', title: 'Review molecule', status: 'open', issueType: 'molecule' }],
  wisps: [{ city: 'gascity', id: 'gc-32', title: 'Temporary workflow', status: 'open', issueType: 'wisp' }],
  convoys: [{ city: 'gascity', id: 'gc-30', title: 'sling-gc-29', status: 'open', issueType: 'convoy' }],
  recentEvents: [{ city: 'gascity', seq: 101, type: 'session.woke', actor: 'controller', subject: 'planner', time: '2026-05-26T10:02:00Z' }],
}

const mailFixture = {
  recipient: 'human',
  limit: 20,
  messages: [
    {
      id: 'gc-52383',
      from: 'chrote-poem-pi',
      recipient: 'human',
      subject: 'C3 remedial pi poem C3R-20260527-004915',
      body: 'The mail of Gas City carries nonce C3R-20260527-004915.',
      status: 'open',
      issueType: 'message',
      read: true,
      fromSessionId: 'gc-51923',
      createdAt: '2026-05-27T00:50:34Z',
    },
  ],
}

function mockGasCityFetch() {
  fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/gascity/observer') return envelope(observerFixture)
    if (path === '/api/gascity/mail?recipient=human&limit=20') return envelope(mailFixture)
    if (path === '/api/gascity/requests/pi-poem' && init?.method === 'POST') {
      return envelope({
        nonce: 'C4A-TEST-NONCE',
        subject: 'CHROTE Pi poem C4A-TEST-NONCE',
        target: 'gc-51923',
        targetAlias: 'chrote-poem-pi',
        targetTemplate: 'pi-smoke',
        targetSessionId: 'gc-51923',
        recipient: 'human',
        output: 'Nudged chrote-poem-pi',
      })
    }
    if (path === '/api/gascity/sessions/gc-1/transcript?lines=120') {
      return envelope({
        source: 'gc-session-peek',
        sessionId: 'gc-1',
        alias: 'planner',
        template: 'planner',
        state: 'active',
        city: 'gascity',
        lines: 120,
        lineCount: 2,
        transcript: 'planner ready\nrecovered after browser disconnect',
        truncated: false,
      })
    }
    return Promise.resolve(new Response(JSON.stringify({
      success: false,
      error: { code: 'UNEXPECTED_FETCH', message: path },
    }), { status: 500, headers: { 'Content-Type': 'application/json' } }))
  })
}

describe('GasCityView', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders Gas City health, sessions, work, mail, formulas, and events', async () => {
    mockGasCityFetch()

    render(<GasCityView />)

    expect(await screen.findByText('Gas City')).toBeInTheDocument()
    expect(screen.getByText('status: ok')).toBeInTheDocument()
    expect(screen.getByText('1 running / 1 total')).toBeInTheDocument()
    expect(screen.getByText('5 unread')).toBeInTheDocument()
    expect(screen.getByText('22 open')).toBeInTheDocument()
    expect(screen.getByText('planner')).toBeInTheDocument()
    expect(screen.getByText('mol-review-quorum')).toBeInTheDocument()
    expect(screen.getByText('Review molecule')).toBeInTheDocument()
    expect(screen.getByText('Temporary workflow')).toBeInTheDocument()
    expect(screen.getByText('session.woke')).toBeInTheDocument()
    expect(screen.getByText('C3 remedial pi poem C3R-20260527-004915')).toBeInTheDocument()
    expect(screen.getByText('The mail of Gas City carries nonce C3R-20260527-004915.')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/observer')
    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/mail?recipient=human&limit=20')
  })

  it('shows unavailable state without mutating Gas City', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const path = String(input)
      if (path === '/api/gascity/observer') {
        return envelope({
          status: 'unavailable',
          checkedAt: '2026-05-26T10:00:00Z',
          error: 'Gas City supervisor unavailable',
          health: { status: '', citiesTotal: 0, citiesRunning: 0, startupReady: false },
          cities: [],
          sessions: [],
          mail: { total: 0, unread: 0 },
          work: { open: 0, ready: 0, inProgress: 0, routed: 0, molecules: 0, wisps: 0, convoys: 0 },
          formulas: [],
          molecules: [],
          wisps: [],
          convoys: [],
          recentEvents: [],
          upstreamErrors: [{ route: '/health', message: 'upstream service unavailable' }],
        })
      }
      if (path === '/api/gascity/mail?recipient=human&limit=20') {
        return envelope({ recipient: 'human', limit: 20, messages: [] })
      }
      return envelope({})
    })

    render(<GasCityView />)

    expect(await screen.findByText('status: unavailable')).toBeInTheDocument()
    expect(screen.getByText('Gas City supervisor unavailable')).toBeInTheDocument()
    expect(screen.getByText('/health: upstream service unavailable')).toBeInTheDocument()
    expect(screen.getByText('No active Gas City sessions.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh Gas City observer' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(4))

    const methods = fetchMock.mock.calls.map(([, init]) => init?.method || 'GET')
    expect(methods).toEqual(['GET', 'GET', 'GET', 'GET'])
  })

  it('sends one bounded Pi poem smoke request', async () => {
    mockGasCityFetch()

    render(<GasCityView />)

    expect(await screen.findByText('C3 remedial pi poem C3R-20260527-004915')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Pi poem topic'), { target: { value: 'mail routes' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send Pi poem smoke' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/gascity/requests/pi-poem', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ topic: 'mail routes' }),
      }))
    })
    expect(await screen.findByText('C4A-TEST-NONCE')).toBeInTheDocument()
    expect(screen.getByText('CHROTE Pi poem C4A-TEST-NONCE')).toBeInTheDocument()
    expect(screen.getByText('Nudged chrote-poem-pi')).toBeInTheDocument()
  })

  it('recovers a Gas City session transcript through its immutable session id', async () => {
    mockGasCityFetch()

    render(<GasCityView />)

    expect(await screen.findByText('planner')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Recover transcript for planner' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/gascity/sessions/gc-1/transcript?lines=120')
    })
    expect(await screen.findByText('Transcript: planner')).toBeInTheDocument()
    expect(screen.getByText('gc-session-peek / gc-1 / active / 2 lines')).toBeInTheDocument()
    expect(screen.getByText(/recovered after browser disconnect/)).toBeInTheDocument()
  })
})
