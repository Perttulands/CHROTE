import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FormationsView from './FormationsView'

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
    formations: [],
    missions: [],
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
        if (body.createMission) {
          board = { ...board, rev: 8, etag: 'board-etag-2', missions: [{ id: 'mis_showcase', ...body.createMission }] }
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

describe('FormationsView S3 missions', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('creates a mission backed by a project Beads issue id', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    await screen.findByText('Improve session search')

    fireEvent.change(screen.getByLabelText('Mission title'), { target: { value: 'Showcase' } })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: 'Build it' } })
    fireEvent.change(screen.getByLabelText('Mission bead'), { target: { value: 'chlab-123' } })
    fireEvent.click(screen.getByRole('button', { name: 'Mission' }))

    await waitFor(() => {
      expect(calls.some(call => String(call.init?.body || '').includes('createMission'))).toBe(true)
    })
    const missionCall = calls.find(call => String(call.init?.body || '').includes('createMission'))
    expect(JSON.parse(String(missionCall?.init?.body))).toMatchObject({
      expectedRev: 7,
      createMission: {
        title: 'Showcase',
        goal: 'Build it',
        beadId: 'chlab-123',
      },
    })
    expect(String(missionCall?.init?.body || '')).not.toContain('chain')
  })

  it('keeps mission create disabled for unsafe Beads issue ids', async () => {
    const { calls } = mockFetch()
    render(<FormationsView />)
    await screen.findByText('Improve session search')

    fireEvent.change(screen.getByLabelText('Mission title'), { target: { value: 'Showcase' } })
    fireEvent.change(screen.getByLabelText('Mission goal'), { target: { value: 'Build it' } })
    fireEvent.change(screen.getByLabelText('Mission bead'), { target: { value: 'chlab/123' } })

    const missionButton = screen.getByRole('button', { name: 'Mission' })
    expect(missionButton).toBeDisabled()
    fireEvent.click(missionButton)

    expect(calls.some(call => String(call.init?.body || '').includes('createMission'))).toBe(false)
  })
})
