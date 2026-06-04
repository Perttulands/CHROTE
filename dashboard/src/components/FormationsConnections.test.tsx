import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

const frame = {
  id: 'fmn_frame',
  type: 'solo',
  title: 'Frame',
  inputs: [{ id: 'port_frame_in', label: 'Input' }],
  outputs: [{ id: 'port_frame_out', label: 'Output' }],
  slots: [{ id: 'slot_frame', label: 'Agent', controller: false }],
}

const ship = {
  id: 'fmn_ship',
  type: 'solo',
  title: 'Ship',
  inputs: [{ id: 'port_ship_in', label: 'Input' }],
  outputs: [{ id: 'port_ship_out', label: 'Output' }],
  slots: [{ id: 'slot_ship', label: 'Agent', controller: false }],
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
    formations: [frame, ship],
    connections: [],
  }
  const layout = {
    schema: 1,
    boardId: board.id,
    boardRev: 7,
    etag: 'layout-etag',
    nodes: [
      { id: 'fmn_frame', x: 80, y: 80 },
      { id: 'fmn_ship', x: 440, y: 80 },
    ],
  }
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/agents') return successResponse({ agents: [] })
    if (url === '/api/formations/boards') {
      return successResponse({ boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }] })
    }
    if (url === '/api/formations/boards/session-search/layout') return successResponse({ layout }, layout.etag)
    if (url === '/api/formations/boards/session-search') {
      if (init?.method === 'PATCH') {
        const body = JSON.parse(String(init.body || '{}'))
        if (body.wireConnection) {
          board = { ...board, rev: 8, etag: 'board-etag-2', connections: [{ id: 'edge_frame_ship', from: body.wireConnection.from, to: body.wireConnection.to }] }
        }
        if (body.addPort) {
          board = {
            ...board,
            rev: 9,
            etag: 'board-etag-3',
            formations: board.formations.map((formation: any) => formation.id === body.addPort.formationId
              ? { ...formation, inputs: [...formation.inputs, { id: 'port_ship_second', label: body.addPort.label }] }
              : formation),
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

describe('FormationsView S3 connections', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('adds a stable input port and wires output to input through board PATCH', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    const frameNode = await screen.findByTestId('formation-node-fmn_frame')
    const shipNode = await screen.findByTestId('formation-node-fmn_ship')

    fireEvent.change(within(frameNode).getByLabelText('Wire Output from Frame'), { target: { value: 'fmn_ship:port_ship_in' } })
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('wireConnection'))).toBe(true)
    })
    const wireCall = calls.find(call => String(call.init?.body || '').includes('wireConnection'))
    expect(JSON.parse(String(wireCall?.init?.body))).toMatchObject({
      expectedRev: 7,
      wireConnection: {
        from: 'fmn_frame:port_frame_out',
        to: 'fmn_ship:port_ship_in',
      },
    })

    fireEvent.click(within(shipNode).getByRole('button', { name: 'Add input to Ship' }))
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('addPort'))).toBe(true)
    })
    const addPortCall = calls.find(call => String(call.init?.body || '').includes('addPort'))
    expect(JSON.parse(String(addPortCall?.init?.body))).toMatchObject({
      addPort: {
        formationId: 'fmn_ship',
        direction: 'input',
        label: 'Input',
      },
    })
  })
})
