import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

const initialBoard = {
  schema: 1,
  id: 'brd_01J9_sesssearch',
  slug: 'session-search',
  title: 'Improve session search',
  rev: 7,
  etag: 'board-etag',
  formations: [
    {
      id: 'fmn_01J9_research',
      type: 'peer',
      title: 'Research huddle',
      inputs: [{ id: 'port_in', label: 'Input' }],
      outputs: [{ id: 'port_out', label: 'Output' }],
      slots: [
        { id: 'slot_a', label: 'Peer', controller: false },
        { id: 'slot_b', label: 'Peer', controller: false },
      ],
    },
  ],
  connections: [],
}

const initialLayout = {
  schema: 1,
  boardId: 'brd_01J9_sesssearch',
  boardRev: 7,
  etag: 'layout-etag',
  nodes: [{ id: 'fmn_01J9_research', x: 120, y: 80 }],
}

const shipFormation = {
  id: 'fmn_01J9_ship',
  type: 'solo',
  title: 'Ship handoff',
  inputs: [{ id: 'port_ship_in', label: 'Input' }],
  outputs: [{ id: 'port_ship_out', label: 'Output' }],
  slots: [{ id: 'slot_ship', label: 'Agent', controller: false }],
}

const blockerFormation = {
  id: 'fmn_01J9_blocker',
  type: 'solo',
  title: 'Middle review',
  inputs: [{ id: 'port_blocker_in', label: 'Input' }],
  outputs: [{ id: 'port_blocker_out', label: 'Output' }],
  slots: [{ id: 'slot_blocker', label: 'Agent', controller: false }],
}

function successResponse(data: unknown, etag?: string) {
  return Promise.resolve({
    ok: true,
    headers: { get: (name: string) => name.toLowerCase() === 'etag' ? etag ?? null : null },
    json: () => Promise.resolve({ success: true, data }),
    text: () => Promise.resolve(''),
  })
}

function mockFetch(options: { board?: any; layout?: any; layoutMissing?: boolean } = {}) {
  const board = options.board || initialBoard
  const layout = options.layout || initialLayout
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/formations/boards') {
      return successResponse({
        boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }],
      })
    }
    if (url === '/api/formations/boards/session-search') {
      if (init?.method === 'PATCH') {
        const created = {
          id: 'fmn_01J9_created',
          type: 'peer',
          title: 'Created huddle',
          inputs: [{ id: 'port_created_in', label: 'Input' }],
          outputs: [{ id: 'port_created_out', label: 'Output' }],
          slots: [
            { id: 'slot_created_a', label: 'Peer', controller: false },
            { id: 'slot_created_b', label: 'Peer', controller: false },
          ],
        }
        return successResponse({
          board: { ...board, rev: 8, etag: 'board-etag-2', formations: [...board.formations, created] },
          formation: created,
          layout: { ...layout, etag: 'layout-etag-2', nodes: [...layout.nodes, { id: created.id, x: 120, y: 120 }] },
        }, 'board-etag-2')
      }
      return successResponse({ board }, board.etag)
    }
    if (url === '/api/formations/boards/session-search/layout') {
      if (init?.method === 'PATCH') {
        const body = JSON.parse(String(init.body || '{}'))
        return successResponse({ layout: { ...layout, etag: 'layout-etag-2', nodes: body.nodes || layout.nodes } }, 'layout-etag-2')
      }
      if (options.layoutMissing) {
        return Promise.resolve({
          ok: false,
          status: 404,
          headers: { get: () => null },
          json: () => Promise.resolve({ success: false, error: { code: 'NOT_FOUND', message: 'layout deleted' } }),
          text: () => Promise.resolve(''),
        })
      }
      return successResponse({ layout }, layout.etag)
    }
    return Promise.resolve({
      ok: false,
      headers: { get: () => null },
      json: () => Promise.resolve({ success: false, error: { code: 'NOT_FOUND', message: url } }),
      text: () => Promise.resolve(''),
    })
  })
  vi.stubGlobal('fetch', fetchMock as any)
  return { fetchMock, calls }
}

describe('FormationsView S2 canvas', () => {
  beforeEach(() => {
    mockFetch()
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('loads the first board and renders typed formation cards with stable slots', async () => {
    render(<FormationsView />)

    expect(await screen.findByText('Improve session search')).toBeInTheDocument()
    expect(await screen.findByText('Research huddle')).toBeInTheDocument()
    const node = screen.getByTestId('formation-node-fmn_01J9_research')
    expect(node).toHaveAttribute('data-formation-type', 'peer')
    expect(within(node).getAllByText('Peer')).toHaveLength(3)
    expect(screen.getByTestId('formation-zoom-level')).toHaveTextContent('100%')
  })

  it('creates a peer formation with board ETag and revision preconditions', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    await screen.findByText('Research huddle')

    fireEvent.change(screen.getByLabelText('Formation title'), { target: { value: 'Created huddle' } })
    fireEvent.click(screen.getByRole('button', { name: 'Peer' }))

    await screen.findByText('Created huddle')
    const createCall = calls.find(call => call.url === '/api/formations/boards/session-search' && call.init?.method === 'PATCH')
    expect(createCall).toBeTruthy()
    expect((createCall?.init?.headers as Record<string, string>)['If-Match']).toBe('board-etag')
    expect(JSON.parse(String(createCall?.init?.body))).toMatchObject({
      expectedRev: 7,
      createFormation: {
        type: 'peer',
        title: 'Created huddle',
      },
    })
  })

  it('pans and zooms the canvas around the cursor', async () => {
    render(<FormationsView />)
    await screen.findByText('Research huddle')
    const viewport = screen.getByTestId('formations-canvas')
    viewport.getBoundingClientRect = () => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 800,
      bottom: 500,
      width: 800,
      height: 500,
      toJSON: () => ({}),
    })

    fireEvent.pointerDown(screen.getByTestId('formations-world'), { button: 0, clientX: 100, clientY: 100 })
    fireEvent.pointerMove(window, { clientX: 145, clientY: 130 })
    fireEvent.pointerUp(window)

    expect(viewport).toHaveAttribute('data-pan-x', '45')
    expect(viewport).toHaveAttribute('data-pan-y', '30')

    fireEvent.wheel(viewport, { deltaY: -100, clientX: 200, clientY: 200 })
    expect(screen.getByTestId('formation-zoom-level')).toHaveTextContent('112%')
    expect(viewport).toHaveAttribute('data-pan-x', '26')
    expect(viewport).toHaveAttribute('data-pan-y', '10')
  })

  it('FIT frames all loaded nodes with padding', async () => {
    mockFetch({
      board: { ...initialBoard, formations: [...initialBoard.formations, shipFormation] },
      layout: {
        ...initialLayout,
        nodes: [
          { id: 'fmn_01J9_research', x: 100, y: 80 },
          { id: 'fmn_01J9_ship', x: 1260, y: 640 },
        ],
      },
    })
    render(<FormationsView />)
    await screen.findByText('Ship handoff')
    const viewport = screen.getByTestId('formations-canvas')
    viewport.getBoundingClientRect = () => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 800,
      bottom: 500,
      width: 800,
      height: 500,
      toJSON: () => ({}),
    })

    fireEvent.click(screen.getByRole('button', { name: 'FIT' }))

    expect(screen.getByTestId('formation-zoom-level')).toHaveTextContent('48%')
    expect(viewport).toHaveAttribute('data-pan-x', '16')
    expect(viewport).toHaveAttribute('data-pan-y', '41')
  })

  it('FIT frames S3 missions and gates as canvas nodes', async () => {
    mockFetch({
      board: {
        ...initialBoard,
        missions: [{
          id: 'mis_showcase',
          title: 'Showcase',
          goal: 'Build the page',
          beadId: 'home-7kc4.5',
        }],
        gates: [{
          id: 'gate_review',
          title: 'Review',
          kinds: ['code'],
          criterion: 'Review it',
        }],
      },
      layout: {
        ...initialLayout,
        nodes: [
          { id: 'mis_showcase', x: 80, y: -80 },
          { id: 'fmn_01J9_research', x: 100, y: 80 },
          { id: 'gate_review', x: 1260, y: 560 },
        ],
      },
    })
    render(<FormationsView />)
    await screen.findByText('Showcase')
    await screen.findByText('Review')
    const viewport = screen.getByTestId('formations-canvas')
    viewport.getBoundingClientRect = () => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 800,
      bottom: 500,
      width: 800,
      height: 500,
      toJSON: () => ({}),
    })

    fireEvent.click(screen.getByRole('button', { name: 'FIT' }))

    expect(screen.getByTestId('formation-zoom-level')).toHaveTextContent('47%')
    expect(viewport).toHaveAttribute('data-pan-x', '29')
    expect(viewport).toHaveAttribute('data-pan-y', '102')
  })

  it('renders the graph with default positions when the layout sidecar was deleted', async () => {
    const { calls } = mockFetch({ layoutMissing: true })
    render(<FormationsView />)

    expect(await screen.findByText('Research huddle')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    const node = screen.getByTestId('formation-node-fmn_01J9_research')

    fireEvent.pointerDown(node, { button: 0, clientX: 120, clientY: 120 })
    fireEvent.pointerMove(window, { clientX: 140, clientY: 150 })
    fireEvent.pointerUp(window)

    await waitFor(() => {
      expect(calls.some(call => call.url === '/api/formations/boards/session-search/layout' && call.init?.method === 'PATCH')).toBe(true)
    })
    const layoutPatch = calls.find(call => call.url === '/api/formations/boards/session-search/layout' && call.init?.method === 'PATCH')
    expect((layoutPatch?.init?.headers as Record<string, string>)['If-Match']).toBe('*')
  })

  it('renders existing connections as routed wires around blocking cards while preserving direct drag feedback', async () => {
    mockFetch({
      board: {
        ...initialBoard,
        formations: [
          {
            ...initialBoard.formations[0],
            id: 'fmn_01J9_source',
            title: 'Source',
            outputs: [{ id: 'port_source_out', label: 'Output' }],
          },
          blockerFormation,
          { ...shipFormation, id: 'fmn_01J9_target', title: 'Target', inputs: [{ id: 'port_target_in', label: 'Input' }] },
        ],
        connections: [{ id: 'edge_source_target', from: 'fmn_01J9_source:port_source_out', to: 'fmn_01J9_target:port_target_in' }],
      },
      layout: {
        ...initialLayout,
        nodes: [
          { id: 'fmn_01J9_source', x: 80, y: 120 },
          { id: 'fmn_01J9_blocker', x: 360, y: 110 },
          { id: 'fmn_01J9_target', x: 640, y: 120 },
        ],
      },
    })
    render(<FormationsView />)

    const wire = await screen.findByTestId('formation-wire-edge_source_target')
    expect(wire).toHaveAttribute('data-from', 'fmn_01J9_source')
    expect(wire).toHaveAttribute('data-to', 'fmn_01J9_target')
    expect(wire.getAttribute('d')).toContain('L344,292')
    expect(wire.getAttribute('d')).toContain('L616,292')

    fireEvent.pointerDown(screen.getByTestId('formation-node-fmn_01J9_source'), { button: 0, clientX: 80, clientY: 120 })
    fireEvent.pointerMove(window, { clientX: 100, clientY: 120 })
    await waitFor(() => {
      expect(screen.getByTestId('formation-wire-edge_source_target').getAttribute('d')).not.toContain('292')
    })
    fireEvent.pointerUp(window)
  })

  it('moves a node through the layout sidecar and never patches the board definition', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    const node = await screen.findByTestId('formation-node-fmn_01J9_research')

    fireEvent.pointerDown(node, { button: 0, clientX: 120, clientY: 80 })
    fireEvent.pointerMove(window, { clientX: 180, clientY: 110 })
    fireEvent.pointerUp(window)

    await waitFor(() => {
      expect(calls.some(call => call.url === '/api/formations/boards/session-search/layout' && call.init?.method === 'PATCH')).toBe(true)
    })
    const layoutPatch = calls.find(call => call.url === '/api/formations/boards/session-search/layout' && call.init?.method === 'PATCH')
    expect((layoutPatch?.init?.headers as Record<string, string>)['If-Match']).toBe('layout-etag')
    expect(JSON.parse(String(layoutPatch?.init?.body))).toMatchObject({
      nodes: [{ id: 'fmn_01J9_research', x: 180, y: 110 }],
    })
    expect(calls.some(call => call.url === '/api/formations/boards/session-search' && call.init?.method === 'PATCH')).toBe(false)
  })
})
