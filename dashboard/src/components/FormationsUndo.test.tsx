import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

const initialFormation = {
  id: 'fmn_01J9_research',
  type: 'peer',
  title: 'Research huddle',
  inputs: [{ id: 'port_in', label: 'Input' }],
  outputs: [{ id: 'port_out', label: 'Output' }],
  slots: [
    { id: 'slot_a', label: 'Peer', controller: false },
    { id: 'slot_b', label: 'Peer', controller: false },
  ],
}

const createdFormation = {
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

const initialBoard = {
  schema: 1,
  id: 'brd_01J9_sesssearch',
  slug: 'session-search',
  title: 'Improve session search',
  rev: 7,
  etag: 'board-etag',
  formations: [initialFormation],
  connections: [],
}

const initialLayout = {
  schema: 1,
  boardId: 'brd_01J9_sesssearch',
  boardRev: 7,
  etag: 'layout-etag',
  nodes: [{ id: 'fmn_01J9_research', x: 120, y: 80 }],
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
  let board = initialBoard
  let layout = initialLayout
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
        const body = JSON.parse(String(init.body || '{}'))
        if (body.createFormation) {
          board = { ...board, rev: 8, etag: 'board-etag-2', formations: [...board.formations, createdFormation] }
          layout = { ...layout, etag: 'layout-etag-2', nodes: [...layout.nodes, { id: createdFormation.id, x: 120, y: 120 }] }
          return successResponse({ board, formation: createdFormation, layout }, 'board-etag-2')
        }
        if (body.deleteFormation) {
          board = { ...board, rev: 9, etag: 'board-etag-3', formations: board.formations.filter(item => item.id !== body.deleteFormation.id) }
          layout = { ...layout, etag: 'layout-etag-3', nodes: layout.nodes.filter(item => item.id !== body.deleteFormation.id) }
          return successResponse({ board, layout }, 'board-etag-3')
        }
      }
      return successResponse({ board }, board.etag)
    }
    if (url === '/api/formations/boards/session-search/layout') {
      if (init?.method === 'PATCH') {
        const body = JSON.parse(String(init.body || '{}'))
        layout = { ...layout, etag: layout.etag === 'layout-etag' ? 'layout-etag-2' : 'layout-etag-3', nodes: body.nodes || layout.nodes }
        return successResponse({ layout }, layout.etag)
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
  return { calls }
}

function mockS3UndoMatrixFetch() {
  let nextPortID = 1
  let board: any = {
    schema: 1,
    id: 'brd_01J9_sesssearch',
    slug: 'session-search',
    title: 'Improve session search',
    rev: 7,
    etag: 'board-etag-7',
    missions: [],
    gates: [],
    formations: [
      {
        id: 'fmn_frame',
        type: 'orchestrated',
        title: 'Frame',
        inputs: [{ id: 'port_frame_in', label: 'Input' }],
        outputs: [{ id: 'port_frame_out', label: 'Output' }],
        slots: [
          { id: 'slot_lead', label: 'Lead', controller: true },
          { id: 'slot_worker', label: 'Worker', controller: false },
        ],
        brief: {
          goal: 'Frame it',
          beadId: 'home-7kc4.1',
          files: ['README.md'],
          links: ['https://example.invalid'],
        },
        verification: {
          id: 'ver_frame',
          kinds: ['code'],
          criterion: 'Tests pass',
          onFail: 'block',
        },
      },
      {
        id: 'fmn_ship',
        type: 'solo',
        title: 'Ship',
        inputs: [{ id: 'port_ship_in', label: 'Input' }],
        outputs: [{ id: 'port_ship_out', label: 'Output' }],
        slots: [{ id: 'slot_ship', label: 'Agent', controller: false }],
      },
    ],
    connections: [],
  }
  const layout = {
    schema: 1,
    boardId: board.id,
    boardRev: 7,
    etag: 'layout-etag',
    nodes: [
      { id: 'fmn_frame', x: 120, y: 80 },
      { id: 'fmn_ship', x: 440, y: 80 },
    ],
  }
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const bumpBoard = () => {
    const nextRev = board.rev + 1
    board = { ...board, rev: nextRev, etag: `board-etag-${nextRev}` }
  }
  const updateFormation = (formationId: string, update: (formation: any) => any) => {
    board = {
      ...board,
      formations: board.formations.map((formation: any) => formation.id === formationId ? update(formation) : formation),
    }
  }
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
          const patch = body.assignSlot
          updateFormation(patch.formationId, (formation: any) => ({
            ...formation,
            slots: formation.slots.map((slot: any) => slot.id === patch.slotId
              ? { ...slot, agentId: patch.agentId || undefined, harness: patch.harness || undefined }
              : slot),
          }))
        }
        if (body.makeController) {
          const patch = body.makeController
          updateFormation(patch.formationId, (formation: any) => ({
            ...formation,
            slots: formation.slots.map((slot: any) => ({ ...slot, controller: slot.id === patch.slotId })),
          }))
        }
        if (body.setBrief) {
          const patch = body.setBrief
          updateFormation(patch.formationId, (formation: any) => ({
            ...formation,
            brief: {
              goal: patch.goal,
              beadId: patch.beadId,
              files: patch.files || [],
              links: patch.links || [],
            },
          }))
        }
        if (body.clearBrief) {
          updateFormation(body.clearBrief.formationId, (formation: any) => ({ ...formation, brief: undefined }))
        }
        if (body.setVerification) {
          const patch = body.setVerification
          updateFormation(patch.formationId, (formation: any) => ({
            ...formation,
            verification: {
              id: formation.verification?.id || 'ver_frame',
              kinds: patch.kinds || ['code'],
              criterion: patch.criterion,
              onFail: patch.onFail,
            },
          }))
        }
        if (body.removeVerification) {
          updateFormation(body.removeVerification.formationId, (formation: any) => ({ ...formation, verification: undefined }))
        }
        if (body.addPort) {
          const patch = body.addPort
          const port = { id: `port_added_${nextPortID++}`, label: patch.label }
          updateFormation(patch.formationId, (formation: any) => ({
            ...formation,
            [patch.direction === 'input' ? 'inputs' : 'outputs']: [...formation[patch.direction === 'input' ? 'inputs' : 'outputs'], port],
          }))
        }
        if (body.removePort) {
          const patch = body.removePort
          updateFormation(patch.formationId, (formation: any) => ({
            ...formation,
            inputs: formation.inputs.filter((port: any) => port.id !== patch.portId),
            outputs: formation.outputs.filter((port: any) => port.id !== patch.portId),
          }))
        }
        if (body.wireConnection) {
          board = { ...board, connections: [{ id: 'edge_frame_ship', from: body.wireConnection.from, to: body.wireConnection.to }] }
        }
        if (body.unwireConnection) {
          board = { ...board, connections: board.connections.filter((connection: any) => connection.from !== body.unwireConnection.from || connection.to !== body.unwireConnection.to) }
        }
        if (body.createGate) {
          board = { ...board, gates: [{ id: 'gate_review', title: body.createGate.title, kinds: body.createGate.kinds, criterion: body.createGate.criterion }] }
        }
        if (body.deleteGate) {
          board = { ...board, gates: board.gates.filter((gate: any) => gate.id !== body.deleteGate.id) }
        }
        if (body.createMission) {
          board = { ...board, missions: [{ id: 'mis_showcase', ...body.createMission }] }
        }
        if (body.deleteMission) {
          board = { ...board, missions: board.missions.filter((mission: any) => mission.id !== body.deleteMission.id) }
        }
        bumpBoard()
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

function boardPatches(calls: Array<{ url: string; init?: RequestInit }>) {
  return calls
    .filter(call => call.url === '/api/formations/boards/session-search' && call.init?.method === 'PATCH')
    .map(call => ({
      body: JSON.parse(String(call.init?.body || '{}')),
      headers: call.init?.headers as Record<string, string>,
    }))
}

function lastBoardPatch(calls: Array<{ url: string; init?: RequestInit }>) {
  const patches = boardPatches(calls)
  return patches[patches.length - 1]
}

async function waitForPatch(calls: Array<{ url: string; init?: RequestInit }>, key: string) {
  await waitFor(() => {
    expect(boardPatches(calls).some(call => key in call.body)).toBe(true)
  })
}

async function waitForPatchCount(calls: Array<{ url: string; init?: RequestInit }>, key: string, count: number) {
  await waitFor(() => {
    expect(boardPatches(calls).filter(call => key in call.body)).toHaveLength(count)
  })
}

describe('FormationsView S3 undo', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('undoes a created formation through an inverse board PATCH unless focus is inside a text field', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    await screen.findByText('Research huddle')

    const title = screen.getByLabelText('Formation title')
    fireEvent.change(title, { target: { value: 'Created huddle' } })
    fireEvent.click(screen.getByRole('button', { name: 'Peer' }))
    await screen.findByText('Created huddle')

    title.focus()
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    expect(calls.some(call => call.url === '/api/formations/boards/session-search' && call.init?.method === 'PATCH' && String(call.init.body || '').includes('deleteFormation'))).toBe(false)

    title.blur()
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })

    await waitFor(() => {
      expect(calls.some(call => call.url === '/api/formations/boards/session-search' && call.init?.method === 'PATCH' && String(call.init.body || '').includes('deleteFormation'))).toBe(true)
    })
    const undoCall = calls.find(call => call.url === '/api/formations/boards/session-search' && call.init?.method === 'PATCH' && String(call.init.body || '').includes('deleteFormation'))
    expect((undoCall?.init?.headers as Record<string, string>)['If-Match']).toBe('board-etag-2')
    expect(JSON.parse(String(undoCall?.init?.body))).toMatchObject({
      expectedRev: 8,
      deleteFormation: { id: 'fmn_01J9_created' },
    })
  })

  it('undoes a layout move through an inverse layout PATCH with the latest layout ETag', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    const node = await screen.findByTestId('formation-node-fmn_01J9_research')

    fireEvent.pointerDown(node, { button: 0, clientX: 120, clientY: 80 })
    fireEvent.pointerMove(window, { clientX: 180, clientY: 110 })
    fireEvent.pointerUp(window)

    await waitFor(() => {
      expect(calls.filter(call => call.url === '/api/formations/boards/session-search/layout' && call.init?.method === 'PATCH')).toHaveLength(1)
    })
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })

    await waitFor(() => {
      expect(calls.filter(call => call.url === '/api/formations/boards/session-search/layout' && call.init?.method === 'PATCH')).toHaveLength(2)
    })
    const undoCall = calls.filter(call => call.url === '/api/formations/boards/session-search/layout' && call.init?.method === 'PATCH')[1]
    expect((undoCall.init?.headers as Record<string, string>)['If-Match']).toBe('layout-etag-2')
    expect(JSON.parse(String(undoCall.init?.body))).toMatchObject({
      nodes: [{ id: 'fmn_01J9_research', x: 120, y: 80 }],
    })
  })

  it('undoes every implemented S3 board mutation through inverse PATCHes with current ETags', async () => {
    const { calls } = mockS3UndoMatrixFetch()
    render(<FormationsView />)
    const frameNode = await screen.findByTestId('formation-node-fmn_frame')

    fireEvent.change(within(frameNode).getByLabelText('Agent for Worker'), { target: { value: 'mason|codex' } })
    await waitForPatch(calls, 'assignSlot')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatchCount(calls, 'assignSlot', 2)
    expect(lastBoardPatch(calls)).toMatchObject({
      headers: { 'If-Match': 'board-etag-8' },
      body: {
        expectedRev: 8,
        assignSlot: {
          formationId: 'fmn_frame',
          slotId: 'slot_worker',
          agentId: '',
          harness: '',
        },
      },
    })

    fireEvent.click(within(frameNode).getByRole('button', { name: 'Make Worker controller' }))
    await waitForPatch(calls, 'makeController')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatchCount(calls, 'makeController', 2)
    expect(lastBoardPatch(calls)?.body).toMatchObject({
      expectedRev: 10,
      makeController: {
        formationId: 'fmn_frame',
        slotId: 'slot_lead',
      },
    })

    fireEvent.change(within(frameNode).getByLabelText('Goal for Frame'), { target: { value: 'Frame it again' } })
    fireEvent.click(within(frameNode).getByRole('button', { name: 'Save brief for Frame' }))
    await waitForPatch(calls, 'setBrief')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatchCount(calls, 'setBrief', 2)
    expect(lastBoardPatch(calls)?.body).toMatchObject({
      expectedRev: 12,
      setBrief: {
        formationId: 'fmn_frame',
        goal: 'Frame it',
        beadId: 'home-7kc4.1',
        files: ['README.md'],
        links: ['https://example.invalid'],
      },
    })

    fireEvent.change(within(frameNode).getByLabelText('Verification criterion for Frame'), { target: { value: 'Review is green' } })
    fireEvent.click(within(frameNode).getByRole('button', { name: 'Save verification for Frame' }))
    await waitForPatch(calls, 'setVerification')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatchCount(calls, 'setVerification', 2)
    expect(lastBoardPatch(calls)?.body).toMatchObject({
      expectedRev: 14,
      setVerification: {
        formationId: 'fmn_frame',
        kinds: ['code'],
        criterion: 'Tests pass',
        onFail: 'block',
      },
    })

    fireEvent.click(within(frameNode).getByRole('button', { name: 'Add input to Frame' }))
    await waitForPatch(calls, 'addPort')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatch(calls, 'removePort')
    expect(lastBoardPatch(calls)?.body).toMatchObject({
      expectedRev: 16,
      removePort: {
        formationId: 'fmn_frame',
        portId: 'port_added_1',
      },
    })

    fireEvent.change(within(frameNode).getByLabelText('Wire Output from Frame'), { target: { value: 'fmn_ship:port_ship_in' } })
    await waitForPatch(calls, 'wireConnection')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatch(calls, 'unwireConnection')
    expect(lastBoardPatch(calls)?.body).toMatchObject({
      expectedRev: 18,
      unwireConnection: {
        from: 'fmn_frame:port_frame_out',
        to: 'fmn_ship:port_ship_in',
      },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Gate' }))
    await waitForPatch(calls, 'createGate')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatch(calls, 'deleteGate')
    expect(lastBoardPatch(calls)?.body).toMatchObject({
      expectedRev: 20,
      deleteGate: { id: 'gate_review' },
    })

    fireEvent.change(screen.getByLabelText('Mission title'), { target: { value: 'Showcase' } })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: 'Build the page' } })
    fireEvent.change(screen.getByLabelText('Mission bead'), { target: { value: 'home-7kc4.5' } })
    fireEvent.click(screen.getByRole('button', { name: 'Mission' }))
    await waitForPatch(calls, 'createMission')
    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })
    await waitForPatch(calls, 'deleteMission')
    expect(lastBoardPatch(calls)?.body).toMatchObject({
      expectedRev: 22,
      deleteMission: { id: 'mis_showcase' },
    })
  })
})
