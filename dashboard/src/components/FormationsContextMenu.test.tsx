import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

const formation = {
  id: 'fmn_frame',
  type: 'orchestrated',
  title: 'Frame',
  inputs: [{ id: 'port_frame_in', label: 'Input' }],
  outputs: [{ id: 'port_frame_out', label: 'Output' }],
  slots: [
    { id: 'slot_lead', label: 'Lead', controller: true, agentId: 'mason', harness: 'codex' },
    { id: 'slot_worker', label: 'Worker', controller: false },
  ],
  verification: {
    id: 'ver_frame',
    kinds: ['code'],
    criterion: 'Tests pass',
    onFail: 'block',
  },
}

const gate = {
  id: 'gate_review',
  title: 'Review',
  kinds: ['code'],
  criterion: 'Review the frame',
}

const mission = {
  id: 'mis_showcase',
  title: 'Showcase',
  goal: 'Build the page',
  beadId: 'home-7kc4.5',
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
  let board = {
    schema: 1,
    id: 'brd_01J9_sesssearch',
    slug: 'session-search',
    title: 'Improve session search',
    rev: 7,
    etag: 'board-etag',
    missions: [mission],
    formations: [formation],
    gates: [gate],
    connections: [{ id: 'edge_mission_frame', from: 'mis_showcase:out', to: 'fmn_frame:port_frame_in' }],
  }
  const layout = {
    schema: 1,
    boardId: board.id,
    boardRev: board.rev,
    etag: 'layout-etag',
    nodes: [
      { id: 'mis_showcase', x: 80, y: -120 },
      { id: 'fmn_frame', x: 120, y: 80 },
      { id: 'gate_review', x: 440, y: 80 },
    ],
  }
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/agents') {
      return successResponse({ agents: [{ id: 'mason', displayName: 'Mason', harnessDefault: 'codex', assignable: true }] })
    }
    if (url === '/api/formations/boards') {
      return successResponse({ boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }] })
    }
    if (url === '/api/formations/boards/session-search/layout') return successResponse({ layout }, layout.etag)
    if (url === '/api/formations/boards/session-search') {
      if (init?.method === 'PATCH') {
        const body = JSON.parse(String(init.body || '{}'))
        if (body.assignSlot) {
          board = {
            ...board,
            rev: 8,
            etag: 'board-etag-2',
            formations: board.formations.map(item => item.id === body.assignSlot.formationId
              ? {
                ...item,
                slots: item.slots.map(slot => slot.id === body.assignSlot.slotId
                  ? { ...slot, agentId: body.assignSlot.agentId, harness: body.assignSlot.harness }
                  : slot),
              }
              : item),
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

function enabledMenuItems(menu: HTMLElement) {
  return within(menu).getAllByRole('menuitem').filter(item => item.getAttribute('aria-disabled') !== 'true')
}

describe('FormationsView S3 context menus', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('opens an authoring-only board menu on empty canvas without enabling run or terminal actions', async () => {
    mockFetch()
    render(<FormationsView />)
    await screen.findByText('Improve session search')

    fireEvent.contextMenu(screen.getByTestId('formations-canvas'), { clientX: 240, clientY: 160 })

    const menu = await screen.findByRole('menu', { name: 'Board actions' })
    expect(within(menu).getByRole('menuitem', { name: 'Mission' })).toBeEnabled()
    expect(within(menu).getByRole('menuitem', { name: 'Gate' })).toBeEnabled()
    expect(within(menu).getByRole('menuitem', { name: 'Peer formation' })).toBeEnabled()
    expect(enabledMenuItems(menu).map(item => item.textContent)).not.toEqual(expect.arrayContaining([
      expect.stringMatching(/run/i),
      expect.stringMatching(/terminal/i),
    ]))
  })

  it('uses element-specific menus for slots, gates, wires, and missions', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    await screen.findByText('Improve session search')

    const formationNode = screen.getByTestId('formation-node-fmn_frame')
    fireEvent.contextMenu(within(formationNode).getByTestId('formation-slot-slot_worker'), { clientX: 320, clientY: 220 })
    let menu = await screen.findByRole('menu', { name: 'Slot actions' })
    const assignMason = within(menu).getByRole('menuitem', { name: 'Assign Mason' })
    expect(assignMason).not.toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(assignMason)
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('assignSlot'))).toBe(true)
    })
    const assignCall = calls.find(call => String(call.init?.body || '').includes('assignSlot'))
    expect(JSON.parse(String(assignCall?.init?.body))).toMatchObject({
      expectedRev: 7,
      assignSlot: {
        formationId: 'fmn_frame',
        slotId: 'slot_worker',
        agentId: 'mason',
        harness: 'codex',
      },
    })

    fireEvent.contextMenu(within(formationNode).getByTestId('formation-slot-slot_worker'), { clientX: 320, clientY: 220 })
    menu = await screen.findByRole('menu', { name: 'Slot actions' })
    expect(within(menu).getByRole('menuitem', { name: 'Make controller' })).toBeEnabled()
    expect(within(menu).queryByRole('menuitem', { name: 'Add output' })).not.toBeInTheDocument()

    fireEvent.pointerDown(screen.getByTestId('formations-canvas'), { button: 0, clientX: 10, clientY: 10 })
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())

    fireEvent.contextMenu(screen.getByTestId('gate-node-gate_review'), { clientX: 420, clientY: 240 })
    menu = await screen.findByRole('menu', { name: 'Gate actions' })
    expect(within(menu).getByRole('menuitem', { name: 'Configure gate' })).toBeEnabled()
    expect(within(menu).getByRole('menuitem', { name: 'Delete gate' })).toHaveClass('destructive')

    fireEvent.contextMenu(screen.getByTestId('formation-wire-edge_mission_frame'), { clientX: 360, clientY: 180 })
    menu = await screen.findByRole('menu', { name: 'Connection actions' })
    expect(within(menu).getByRole('menuitem', { name: 'Remove connection' })).toHaveClass('destructive')

    fireEvent.contextMenu(screen.getByTestId('mission-node-mis_showcase'), { clientX: 200, clientY: 100 })
    menu = await screen.findByRole('menu', { name: 'Mission actions' })
    expect(within(menu).getByRole('menuitem', { name: 'Open panel' })).toBeEnabled()
    expect(within(menu).getByRole('menuitem', { name: 'Start mission' })).toHaveAttribute('aria-disabled', 'true')
    expect(enabledMenuItems(menu).map(item => item.textContent)).not.toEqual(expect.arrayContaining([
      expect.stringMatching(/terminal/i),
    ]))
  })
})
