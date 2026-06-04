import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

const board = {
  schema: 1,
  id: 'brd_01J9_sesssearch',
  slug: 'session-search',
  title: 'Improve session search',
  rev: 7,
  etag: 'board-etag',
  missions: [{
    id: 'mis_showcase',
    title: 'Showcase',
    goal: 'Build the page',
    beadId: 'home-7kc4.7',
  }],
  formations: [{
    id: 'fmn_work',
    type: 'solo',
    title: 'Work',
    inputs: [{ id: 'port_work_in', label: 'Input' }],
    outputs: [{ id: 'port_work_out', label: 'Output' }],
    slots: [{ id: 'slot_work', label: 'Worker', controller: true, agentId: 'scout', harness: 'openai-codex' }],
  }],
  gates: [],
  connections: [{ id: 'edge_mission_work', from: 'mis_showcase:out', to: 'fmn_work:port_work_in' }],
}

const layout = {
  schema: 1,
  boardId: board.id,
  boardRev: board.rev,
  etag: 'layout-etag',
  nodes: [
    { id: 'mis_showcase', x: 80, y: -120 },
    { id: 'fmn_work', x: 120, y: 80 },
  ],
}

function successResponse(data: unknown, etag?: string) {
  return Promise.resolve({
    ok: true,
    headers: { get: (name: string) => name.toLowerCase() === 'etag' ? etag ?? null : null },
    json: () => Promise.resolve({ success: true, data }),
    text: () => Promise.resolve(''),
  })
}

class MockEventSource {
  static instances: MockEventSource[] = []

  url: string
  listeners = new Map<string, Array<(event: MessageEvent) => void>>()
  closed = false

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) || []), listener])
  }

  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(type, (this.listeners.get(type) || []).filter(candidate => candidate !== listener))
  }

  close() {
    this.closed = true
  }

  emit(type: string, data: unknown) {
    const event = { data: JSON.stringify(data) } as MessageEvent
    for (const listener of this.listeners.get(type) || []) listener(event)
  }
}

function mockFetch() {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/agents') return successResponse({ agents: [] })
    if (url === '/api/formations/boards') {
      return successResponse({ boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }] })
    }
    if (url === '/api/formations/boards/session-search') return successResponse({ board }, board.etag)
    if (url === '/api/formations/boards/session-search/layout') return successResponse({ layout }, layout.etag)
    if (url === '/api/formations/runs' && init?.method === 'POST') {
      return successResponse({
        runId: 'run_01J9_cli',
        status: {
          runId: 'run_01J9_cli',
          status: 'running',
          final: false,
          boardSlug: 'session-search',
          missionId: 'mis_showcase',
          eventCount: 1,
        },
      })
    }
    if (url === '/api/formations/runs/run_01J9_cli/abort' && init?.method === 'POST') {
      return successResponse({
        runId: 'run_01J9_cli',
        status: 'canceled',
        final: true,
        boardSlug: 'session-search',
        missionId: 'mis_showcase',
        eventCount: 4,
      })
    }
    if (url === '/api/formations/runs/run_01J9_cli/resume' && init?.method === 'POST') {
      return successResponse({
        status: {
          runId: 'run_01J9_cli',
          status: 'running',
          final: false,
          boardSlug: 'session-search',
          missionId: 'mis_showcase',
          eventCount: 3,
          resumeAllowed: false,
        },
      })
    }
    if (url === '/api/formations/runs/run_01J9_cli/gates/gate_review/verdict' && init?.method === 'POST') {
      return successResponse({
        status: {
          runId: 'run_01J9_cli',
          status: 'succeeded',
          final: true,
          boardSlug: 'session-search',
          missionId: 'mis_showcase',
          eventCount: 6,
          resumeAllowed: false,
        },
      })
    }
    return Promise.resolve({
      ok: false,
      headers: { get: () => null },
      json: () => Promise.resolve({ success: false, error: { code: 'NOT_FOUND', message: url } }),
      text: () => Promise.resolve(''),
    })
  })
  vi.stubGlobal('fetch', fetchMock as any)
  vi.stubGlobal('EventSource', MockEventSource as any)
  return { calls }
}

describe('FormationsView S4 run status', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    MockEventSource.instances = []
  })

  it('starts a mission, streams ledger events, and aborts through the run API', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    await screen.findByText('Showcase')

    fireEvent.click(screen.getByRole('button', { name: 'Start Showcase' }))

    await waitFor(() => {
      expect(calls.some(call => call.url === '/api/formations/runs' && call.init?.method === 'POST')).toBe(true)
    })
    const startCall = calls.find(call => call.url === '/api/formations/runs' && call.init?.method === 'POST')
    expect(JSON.parse(String(startCall?.init?.body))).toMatchObject({
      board: 'session-search',
      missionId: 'mis_showcase',
      actor: 'agent:ui',
    })
    expect(screen.getByTestId('formation-run-status')).toHaveTextContent('running')
    expect(MockEventSource.instances[0]?.url).toBe('/api/formations/runs/run_01J9_cli/stream?since=0')

    MockEventSource.instances[0].emit('node_started', {
      seq: 2,
      type: 'node_started',
      runId: 'run_01J9_cli',
      nodeId: 'fmn_work',
    })
    expect(await screen.findByText('node_started')).toBeInTheDocument()
    expect(screen.getByText('fmn_work')).toBeInTheDocument()
    MockEventSource.instances[0].emit('node_output', {
      seq: 3,
      type: 'node_output',
      runId: 'run_01J9_cli',
      nodeId: 'fmn_work',
      data: {
        text: 'Report body from ledger',
        reportRef: 'reports/fmn_work.md',
      },
    })
    expect(await screen.findByText('Report body from ledger')).toBeInTheDocument()
    expect(screen.getByText('reports/fmn_work.md')).toBeInTheDocument()

    MockEventSource.instances[0].emit('escalation_raised', {
      seq: 4,
      type: 'escalation_raised',
      runId: 'run_01J9_cli',
      nodeId: 'fmn_work',
      data: {
        reason: 'found a better direction',
        severity: 'needs-attention',
      },
    })
    expect(await screen.findByText('found a better direction')).toBeInTheDocument()
    expect(screen.getByTestId('formation-node-fmn_work')).toHaveClass('formation-card-escalating')

    MockEventSource.instances[0].emit('human_input_requested', {
      seq: 5,
      type: 'human_input_requested',
      runId: 'run_01J9_cli',
      gateId: 'gate_review',
      data: {
        prompt: 'Good enough to ship',
      },
    })
    expect(await screen.findByText('Good enough to ship')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Abort run' }))

    await waitFor(() => {
      expect(calls.some(call => call.url === '/api/formations/runs/run_01J9_cli/abort' && call.init?.method === 'POST')).toBe(true)
    })
    expect(screen.getByTestId('formation-run-status')).toHaveTextContent('canceled')
  })

  it('resumes blocked runs and submits human gate verdicts through the run API', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    await screen.findByText('Showcase')

    fireEvent.click(screen.getByRole('button', { name: 'Start Showcase' }))
    await waitFor(() => {
      expect(MockEventSource.instances[0]?.url).toBe('/api/formations/runs/run_01J9_cli/stream?since=0')
    })

    MockEventSource.instances[0].emit('run_blocked', {
      seq: 2,
      type: 'run_blocked',
      runId: 'run_01J9_cli',
      data: {
        resumeAllowed: true,
      },
    })
    expect(await screen.findByRole('button', { name: 'Resume run' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Resume run' }))
    await waitFor(() => {
      expect(calls.some(call => call.url === '/api/formations/runs/run_01J9_cli/resume' && call.init?.method === 'POST')).toBe(true)
    })
    const resumeCall = calls.find(call => call.url === '/api/formations/runs/run_01J9_cli/resume' && call.init?.method === 'POST')
    expect(JSON.parse(String(resumeCall?.init?.body))).toMatchObject({
      actor: 'agent:ui',
      mode: 'reattach',
      reason: 'operator resume',
    })
    expect(screen.getByTestId('formation-run-status')).toHaveTextContent('running')

    MockEventSource.instances[0].emit('human_input_requested', {
      seq: 4,
      type: 'human_input_requested',
      runId: 'run_01J9_cli',
      gateId: 'gate_review',
      data: {
        prompt: 'Good enough to ship',
      },
    })
    expect(await screen.findByText('Good enough to ship')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Approve gate gate_review' }))
    await waitFor(() => {
      expect(calls.some(call => call.url === '/api/formations/runs/run_01J9_cli/gates/gate_review/verdict' && call.init?.method === 'POST')).toBe(true)
    })
    const verdictCall = calls.find(call => call.url === '/api/formations/runs/run_01J9_cli/gates/gate_review/verdict' && call.init?.method === 'POST')
    expect(JSON.parse(String(verdictCall?.init?.body))).toMatchObject({
      actor: 'agent:ui',
      verdict: 'pass',
      reason: 'operator approved',
    })
    expect(screen.getByTestId('formation-run-status')).toHaveTextContent('succeeded')
  })
})
