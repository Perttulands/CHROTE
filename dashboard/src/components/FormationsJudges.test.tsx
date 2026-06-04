import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

const judgeFormation = {
  id: 'fmn_j1',
  type: 'solo',
  title: 'Judge 1',
  inputs: [{ id: 'port_j1_in', label: 'Input' }],
  outputs: [{ id: 'port_j1_out', label: 'Output' }],
  slots: [{ id: 'slot_j1', label: 'Agent', controller: false }],
}

function successResponse(data: unknown, etag?: string) {
  return Promise.resolve({
    ok: true,
    headers: { get: (name: string) => name.toLowerCase() === 'etag' ? etag ?? null : null },
    json: () => Promise.resolve({ success: true, data }),
    text: () => Promise.resolve(''),
  })
}

function mockFetch() {
  let board: any = {
    schema: 1,
    id: 'brd_01J9_sesssearch',
    slug: 'session-search',
    title: 'Improve session search',
    rev: 7,
    etag: 'board-etag',
    formations: [judgeFormation],
    gates: [{ id: 'gate_review', title: 'Review', kinds: ['code'], criterion: 'Check it.' }],
    connections: [],
  }
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/agents') return successResponse({ agents: [] })
    if (url === '/api/formations/boards') {
      return successResponse({ boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }] })
    }
    if (url === '/api/formations/boards/session-search/layout') {
      return successResponse({ layout: { schema: 1, boardId: board.id, boardRev: board.rev, etag: 'layout-etag', nodes: [] } }, 'layout-etag')
    }
    if (url === '/api/formations/boards/session-search') {
      if (init?.method === 'PATCH') {
        const body = JSON.parse(String(init.body || '{}'))
        if (body.setGateJudge) {
          board = {
            ...board,
            rev: 8,
            etag: 'board-etag-2',
            gates: [{ ...board.gates[0], kinds: ['code', 'formation'] }],
            connections: [
              { id: 'edge_send', from: 'gate_review:judge', to: 'fmn_j1:port_j1_in' },
              { id: 'edge_return', from: 'fmn_j1:port_j1_out', to: 'gate_review:judge' },
            ],
          }
        }
        return successResponse({ board }, board.etag)
      }
      return successResponse({ board }, board.etag)
    }
    return Promise.resolve({
      ok: false,
      headers: { get: () => null },
      json: () => Promise.resolve({ success: false, error: { code: 'NOT_FOUND', message: url } }),
      text: () => Promise.resolve(''),
    })
  })
  vi.stubGlobal('fetch', fetchMock as any)
  return { calls }
}

describe('FormationsView S3 judges', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('attaches a formation judge through normal gate judge connections', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    const gate = await screen.findByTestId('gate-node-gate_review')

    fireEvent.change(within(gate).getByLabelText('Judge chain for Review'), { target: { value: 'fmn_j1' } })
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('setGateJudge'))).toBe(true)
    })
    const judgeCall = calls.find(call => String(call.init?.body || '').includes('setGateJudge'))
    expect(JSON.parse(String(judgeCall?.init?.body))).toMatchObject({
      expectedRev: 7,
      setGateJudge: {
        gateId: 'gate_review',
        chain: ['fmn_j1'],
      },
    })
  })
})
