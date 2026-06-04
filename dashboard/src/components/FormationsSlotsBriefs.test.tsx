import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

const orchestratedFormation = {
  id: 'fmn_orch',
  type: 'orchestrated',
  title: 'Orchestrate',
  inputs: [{ id: 'port_in', label: 'Input' }],
  outputs: [{ id: 'port_out', label: 'Output' }],
  slots: [
    { id: 'slot_controller', label: 'Controller', controller: true },
    { id: 'slot_worker', label: 'Worker', controller: false },
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

function mockFetch() {
  let board: any = {
    schema: 1,
    id: 'brd_01J9_sesssearch',
    slug: 'session-search',
    title: 'Improve session search',
    rev: 7,
    etag: 'board-etag',
    formations: [orchestratedFormation],
    connections: [],
  }
  const layout = {
    schema: 1,
    boardId: board.id,
    boardRev: 7,
    etag: 'layout-etag',
    nodes: [{ id: 'fmn_orch', x: 120, y: 80 }],
  }
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/agents') {
      return successResponse({
        agents: [
          { id: 'codex', displayName: 'Codex', harnessDefault: 'openai-codex', assignable: true, liveness: 'live' },
          { id: 'conductor', displayName: 'Conductor', harnessDefault: 'claude-code', assignable: true, liveness: 'offline' },
        ],
      })
    }
    if (url === '/api/formations/boards') {
      return successResponse({
        boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }],
      })
    }
    if (url === '/api/formations/boards/session-search/layout') {
      return successResponse({ layout }, layout.etag)
    }
    if (url === '/api/formations/boards/session-search') {
      if (init?.method === 'PATCH') {
        const body = JSON.parse(String(init.body || '{}'))
        const formation = board.formations[0]
        if (body.assignSlot) {
          board = {
            ...board,
            rev: board.rev + 1,
            etag: `board-etag-${board.rev + 1}`,
            formations: [{
              ...formation,
              slots: formation.slots.map((slot: any) => slot.id === body.assignSlot.slotId
                ? { ...slot, agentId: body.assignSlot.agentId, harness: body.assignSlot.harness }
                : slot),
            }],
          }
        }
        if (body.makeController) {
          board = {
            ...board,
            rev: board.rev + 1,
            etag: `board-etag-${board.rev + 1}`,
            formations: [{
              ...formation,
              slots: formation.slots.map((slot: any) => ({ ...slot, controller: slot.id === body.makeController.slotId })),
            }],
          }
        }
        if (body.setBrief) {
          board = {
            ...board,
            rev: board.rev + 1,
            etag: `board-etag-${board.rev + 1}`,
            formations: [{ ...formation, brief: body.setBrief }],
          }
        }
        if (body.setVerification) {
          board = {
            ...board,
            rev: board.rev + 1,
            etag: `board-etag-${board.rev + 1}`,
            formations: [{ ...formation, verification: { id: 'ver_review', ...body.setVerification } }],
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

describe('FormationsView S3 slots and briefs', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('assigns persona ids to slots, changes the controller, and saves brief plus verification', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    const node = await screen.findByTestId('formation-node-fmn_orch')

    fireEvent.change(within(node).getByLabelText('Agent for Worker'), { target: { value: 'codex|openai-codex' } })
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('assignSlot'))).toBe(true)
    })
    const assignCall = calls.find(call => String(call.init?.body || '').includes('assignSlot'))
    expect(JSON.parse(String(assignCall?.init?.body))).toMatchObject({
      expectedRev: 7,
      assignSlot: {
        formationId: 'fmn_orch',
        slotId: 'slot_worker',
        agentId: 'codex',
        harness: 'openai-codex',
      },
    })
    expect(String(assignCall?.init?.body || '')).not.toContain('session')

    fireEvent.click(within(node).getByRole('button', { name: 'Make Worker controller' }))
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('makeController'))).toBe(true)
    })
    const controllerCall = calls.find(call => String(call.init?.body || '').includes('makeController'))
    expect(JSON.parse(String(controllerCall?.init?.body))).toMatchObject({
      makeController: { formationId: 'fmn_orch', slotId: 'slot_worker' },
    })

    fireEvent.change(within(node).getByLabelText('Goal for Orchestrate'), { target: { value: 'Frame the goal' } })
    fireEvent.change(within(node).getByLabelText('Bead for Orchestrate'), { target: { value: 'home-7kc4.5' } })
    fireEvent.change(within(node).getByLabelText('Files for Orchestrate'), { target: { value: 'src/SessionPanel.tsx' } })
    fireEvent.change(within(node).getByLabelText('Links for Orchestrate'), { target: { value: 'https://example.com/spec' } })
    fireEvent.click(within(node).getByRole('button', { name: 'Save brief for Orchestrate' }))
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('setBrief'))).toBe(true)
    })
    const briefCall = calls.find(call => String(call.init?.body || '').includes('setBrief'))
    expect(JSON.parse(String(briefCall?.init?.body))).toMatchObject({
      setBrief: {
        formationId: 'fmn_orch',
        goal: 'Frame the goal',
        beadId: 'home-7kc4.5',
        files: ['src/SessionPanel.tsx'],
        links: ['https://example.com/spec'],
      },
    })

    fireEvent.change(within(node).getByLabelText('Verification criterion for Orchestrate'), { target: { value: 'Tests pass.' } })
    fireEvent.change(within(node).getByLabelText('Verification failure for Orchestrate'), { target: { value: 'pushback' } })
    fireEvent.click(within(node).getByRole('button', { name: 'Save verification for Orchestrate' }))
    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('setVerification'))).toBe(true)
    })
    const verificationCall = calls.find(call => String(call.init?.body || '').includes('setVerification'))
    expect(JSON.parse(String(verificationCall?.init?.body))).toMatchObject({
      setVerification: {
        formationId: 'fmn_orch',
        kinds: ['code'],
        criterion: 'Tests pass.',
        onFail: 'pushback',
      },
    })
  })
})
