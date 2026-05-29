import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getGasCityReviewQuorumCapability, launchGasCityReviewQuorum } from '../../services/gascityClient'
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

const reviewQuorumCapabilityFixture = {
  available: true,
  formula: 'mol-review-quorum',
  targets: ['codex-review', 'claude-review', 'codex-synth'],
}

function mockGasCityFetch() {
  fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/gascity/observer') return envelope(observerFixture)
    if (path === '/api/gascity/mail?recipient=human&limit=20') return envelope(mailFixture)
    if (path === '/api/gascity/workflows/review-quorum/capability') return envelope(reviewQuorumCapabilityFixture)
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
    if (path === '/api/gascity/workflows/review-quorum' && init?.method === 'POST') {
      return envelope({
        formula: 'mol-review-quorum',
        workflowId: 'wf-review-42',
        beadId: 'home-a2vw',
        target: 'codex-synth',
        title: 'Quorum review',
        subject: 'Review launcher frontend slice',
        baseRef: 'origin/main',
        mode: 'read-only review quorum',
        scope: { kind: 'city', ref: 'gascity' },
        output: 'Queued review quorum',
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

describe('gascityClient', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('posts expected review quorum JSON to Gas City', async () => {
    const request = {
      subject: 'Review launcher frontend slice',
      title: 'Quorum review',
      baseRef: 'origin/main',
      scopeKind: 'city' as const,
      scopeRef: 'gascity',
      laneOne: {
        id: 'codex-review-mock-1',
        provider: 'codex',
        model: 'codex-cli-default',
        target: 'codex-review',
      },
      laneTwo: {
        id: 'claude-review-mock-2',
        provider: 'claude',
        model: 'claude-cli-default',
        target: 'claude-review',
      },
      synthesisTarget: 'codex-synth',
    }

    fetchMock.mockImplementation(() => envelope({
      formula: 'mol-review-quorum',
      workflowId: 'wf-review-42',
      target: 'codex-synth',
      title: 'Quorum review',
      subject: 'Review launcher frontend slice',
      baseRef: 'origin/main',
      mode: 'read-only review quorum',
      scope: { kind: 'city', ref: 'gascity' },
    }))

    await expect(launchGasCityReviewQuorum(request)).resolves.toMatchObject({
      workflowId: 'wf-review-42',
      title: 'Quorum review',
    })
    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/workflows/review-quorum', expect.objectContaining({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    }))
  })

  it('fetches review quorum capability from the dedicated endpoint', async () => {
    fetchMock.mockImplementation(() => envelope(reviewQuorumCapabilityFixture))

    await expect(getGasCityReviewQuorumCapability()).resolves.toEqual(reviewQuorumCapabilityFixture)
    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/workflows/review-quorum/capability')
  })
})

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
    expect(screen.getByRole('button', { name: 'Launch review quorum' })).toBeInTheDocument()
    expect(screen.getByLabelText('Review quorum targets')).toHaveTextContent('codex-review')
    expect(screen.getByLabelText('Review quorum targets')).toHaveTextContent('claude-review')
    expect(screen.getByLabelText('Review quorum targets')).toHaveTextContent('codex-synth')
    expect(screen.getByLabelText('Review base ref')).toHaveValue('origin/main')
    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/observer')
    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/mail?recipient=human&limit=20')
    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/workflows/review-quorum/capability')
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
      if (path === '/api/gascity/workflows/review-quorum/capability') {
        return envelope({ available: false, formula: 'mol-review-quorum', reason: 'Gas City unavailable' })
      }
      return envelope({})
    })

    render(<GasCityView />)

    expect(await screen.findByText('status: unavailable')).toBeInTheDocument()
    expect(screen.getByText('Gas City supervisor unavailable')).toBeInTheDocument()
    expect(screen.getByText('/health: upstream service unavailable')).toBeInTheDocument()
    expect(screen.getByText('No active Gas City sessions.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh Gas City observer' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(6))

    const methods = fetchMock.mock.calls.map(([, init]) => init?.method || 'GET')
    expect(methods).toEqual(['GET', 'GET', 'GET', 'GET', 'GET', 'GET'])
  })

  it('hides review quorum launcher when capability is unavailable but keeps workflow lists', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const path = String(input)
      if (path === '/api/gascity/observer') return envelope(observerFixture)
      if (path === '/api/gascity/mail?recipient=human&limit=20') return envelope(mailFixture)
      if (path === '/api/gascity/workflows/review-quorum/capability') {
        return envelope({ available: false, formula: 'mol-review-quorum', reason: 'Gas City review quorum formula is unavailable' })
      }
      return Promise.resolve(new Response(JSON.stringify({
        success: false,
        error: { code: 'UNEXPECTED_FETCH', message: path },
      }), { status: 500, headers: { 'Content-Type': 'application/json' } }))
    })

    render(<GasCityView />)

    expect(await screen.findByText('mol-review-quorum')).toBeInTheDocument()
    expect(screen.getByText('Review molecule')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Launch review quorum' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Review subject')).not.toBeInTheDocument()
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

  it('launches review quorum and renders the returned workflow id', async () => {
    mockGasCityFetch()

    render(<GasCityView />)

    expect(await screen.findByText('mol-review-quorum')).toBeInTheDocument()
    expect(screen.getByLabelText('Review base ref')).toHaveValue('origin/main')
    expect(screen.getByLabelText('Review quorum targets')).toHaveTextContent('codex-review')
    expect(screen.getByLabelText('Review quorum targets')).toHaveTextContent('claude-review')
    expect(screen.getByLabelText('Review quorum targets')).toHaveTextContent('codex-synth')
    fireEvent.change(screen.getByLabelText('Review subject'), { target: { value: 'Review launcher frontend slice' } })
    fireEvent.change(screen.getByLabelText('Review title'), { target: { value: 'Quorum review' } })
    fireEvent.click(screen.getByRole('button', { name: 'Launch review quorum' }))

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([input, init]) => (
        String(input) === '/api/gascity/workflows/review-quorum' && init?.method === 'POST'
      ))).toBe(true)
    })
    const launchCall = fetchMock.mock.calls.find(([input, init]) => (
      String(input) === '/api/gascity/workflows/review-quorum' && init?.method === 'POST'
    ))
    const launchBody = JSON.parse(String(launchCall?.[1]?.body))
    expect(launchBody).toMatchObject({
      subject: 'Review launcher frontend slice',
      title: 'Quorum review',
      baseRef: 'origin/main',
      scopeKind: 'city',
      scopeRef: 'gascity',
      laneOne: {
        provider: 'codex',
        model: 'codex-cli-default',
        target: 'codex-review',
      },
      laneTwo: {
        provider: 'claude',
        model: 'claude-cli-default',
        target: 'claude-review',
      },
      synthesisTarget: 'codex-synth',
    })
    expect(launchBody.safetyMode).toBeUndefined()
    expect(launchBody.laneOne.id).toMatch(/^codex-review-[a-z0-9]+-\d+$/)
    expect(launchBody.laneTwo.id).toMatch(/^claude-review-[a-z0-9]+-\d+$/)
    expect(launchBody.laneOne.id).not.toEqual(launchBody.laneTwo.id)
    expect(await screen.findByText('wf-review-42')).toBeInTheDocument()
    expect(screen.getByText('Quorum review / read-only review quorum / base origin/main / target codex-synth')).toBeInTheDocument()
    expect(screen.getByText('Queued review quorum')).toBeInTheDocument()
  })

  it('shows review quorum API errors', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/gascity/observer') return envelope(observerFixture)
      if (path === '/api/gascity/mail?recipient=human&limit=20') return envelope(mailFixture)
      if (path === '/api/gascity/workflows/review-quorum/capability') return envelope(reviewQuorumCapabilityFixture)
      if (path === '/api/gascity/workflows/review-quorum' && init?.method === 'POST') {
        return Promise.resolve(new Response(JSON.stringify({
          success: false,
          error: { code: 'GASCITY_REVIEW_FAILED', message: 'review quorum refused' },
        }), { status: 400, headers: { 'Content-Type': 'application/json' } }))
      }
      return Promise.resolve(new Response(JSON.stringify({
        success: false,
        error: { code: 'UNEXPECTED_FETCH', message: path },
      }), { status: 500, headers: { 'Content-Type': 'application/json' } }))
    })

    render(<GasCityView />)

    expect(await screen.findByText('mol-review-quorum')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Review subject'), { target: { value: 'Review launcher frontend slice' } })
    fireEvent.click(screen.getByRole('button', { name: 'Launch review quorum' }))

    expect(await screen.findByText('GASCITY_REVIEW_FAILED: review quorum refused')).toBeInTheDocument()
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
    expect(screen.queryByText(/Recovered from CHROTE archive/)).not.toBeInTheDocument()
  })

  it('flags an archive-recovered transcript as a stale snapshot after restart', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const path = String(input)
      if (path === '/api/gascity/observer') return envelope(observerFixture)
      if (path === '/api/gascity/mail?recipient=human&limit=20') return envelope(mailFixture)
      if (path === '/api/gascity/sessions/gc-1/transcript?lines=120') {
        return envelope({
          source: 'chrote-archive',
          stale: true,
          sessionId: 'gc-1',
          alias: 'planner',
          template: 'planner',
          state: 'active',
          city: 'gascity',
          lines: 120,
          lineCount: 1,
          capturedAt: '2026-05-27T12:00:00Z',
          transcript: 'last captured planner output',
          truncated: false,
        })
      }
      return Promise.resolve(new Response(JSON.stringify({
        success: false,
        error: { code: 'UNEXPECTED_FETCH', message: path },
      }), { status: 500, headers: { 'Content-Type': 'application/json' } }))
    })

    render(<GasCityView />)

    expect(await screen.findByText('planner')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Recover transcript for planner' }))

    expect(await screen.findByText('Transcript: planner')).toBeInTheDocument()
    expect(screen.getByText(/Recovered from CHROTE archive/)).toBeInTheDocument()
    expect(screen.getByText(/captured 2026-05-27T12:00:00Z/)).toBeInTheDocument()
    expect(screen.getByText(/last captured planner output/)).toBeInTheDocument()
  })
})
