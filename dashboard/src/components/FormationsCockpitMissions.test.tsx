import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FormationsCockpit, { FormationsCockpitHandle } from './FormationsCockpit'

/* The Missions open-board contract on the cockpit:
   - card↔pane linkage: a staffed slot whose persona has a LIVE mission-<stem>
     session gets a "view agent" action; one WITHOUT shows an honest "no live
     session" hint (never fabricated).
   - the controlled selectedSlug prop hides the in-canvas board picker.
   - the imperative openMissionEditor opens the board's single mission. */

const formation = {
  id: 'fmn_frame',
  type: 'solo',
  title: 'Frame',
  inputs: [{ id: 'port_in', label: 'Input' }],
  outputs: [{ id: 'port_out', label: 'Output' }],
  slots: [{ id: 'slot_lead', label: 'Lead', controller: true, agentId: 'scout', harness: 'codex' }],
}

const mission = { id: 'mis_showcase', title: 'Showcase', goal: 'Build the page', beadId: 'home-7kc4.5' }

function makeBoard() {
  return {
    schema: 1,
    id: 'brd_test',
    slug: 'test-board',
    title: 'Test board',
    rev: 7,
    etag: 'board-etag',
    missions: [mission],
    formations: [formation],
    gates: [],
    connections: [{ id: 'edge', from: 'mis_showcase:out', to: 'fmn_frame:port_in' }],
  }
}

const layout = {
  schema: 1,
  boardId: 'brd_test',
  boardRev: 7,
  etag: 'layout-etag',
  nodes: [{ id: 'mis_showcase', x: 100, y: 100 }, { id: 'fmn_frame', x: 420, y: 100 }],
  edges: [],
}

const agents = [{ id: 'scout', displayName: 'Scout', harnessDefault: 'codex', liveness: 'live', assignable: true, unbound: false }]

function installFetchMock() {
  let board = makeBoard()
  ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const respond = (data: unknown, etag = '') => Promise.resolve({
      ok: true,
      headers: { get: (name: string) => (name.toLowerCase() === 'etag' ? etag || null : null) },
      json: () => Promise.resolve({ success: true, data }),
      text: () => Promise.resolve(''),
    })
    if (init?.method === 'PATCH') {
      board = { ...board, rev: board.rev + 1 }
      if (url.endsWith('/layout')) return respond({ layout }, 'layout-etag-2')
      return respond({ board }, 'board-etag-2')
    }
    if (url === '/api/formations/boards') return respond({ boards: [{ slug: board.slug, title: board.title }] })
    if (url.includes('/changes')) return respond({ signal: { changed: false } })
    if (url.endsWith('/layout')) return respond({ layout }, 'layout-etag')
    if (url.includes('/api/formations/boards/')) return respond({ board }, 'board-etag')
    if (url === '/api/agents') return respond({ agents })
    return respond({})
  }) as unknown as typeof fetch
}

function mockResizeObserver() {
  class ResizeObserverMock {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }
  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
}

describe('FormationsCockpit Missions open-board props', () => {
  beforeEach(() => {
    localStorage.clear()
    mockResizeObserver()
    installFetchMock()
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('hides the in-canvas board picker when selectedSlug is controlled', async () => {
    render(<FormationsCockpit selectedSlug="test-board" />)
    await screen.findByTestId('formation-node-fmn_frame')
    expect(screen.queryByTestId('board-picker')).toBeNull()
    expect(screen.getByTestId('board-name')).toBeInTheDocument()
  })

  it('shows "view agent" only when the persona has a live mission-<stem> session', async () => {
    const onViewAgentSession = vi.fn()
    render(
      <FormationsCockpit
        selectedSlug="test-board"
        liveSessionNames={new Set(['mission-scout'])}
        onViewAgentSession={onViewAgentSession}
      />
    )
    await screen.findByTestId('formation-node-fmn_frame')

    const viewBtn = screen.getByTestId('view-agent-scout')
    fireEvent.click(viewBtn)
    // Focuses the matching formations pane by session name.
    expect(onViewAgentSession).toHaveBeenCalledWith('mission-scout')
    expect(screen.queryByTestId('no-session-scout')).toBeNull()
  })

  it('shows an honest "no live session" hint when the persona has no live session', async () => {
    render(
      <FormationsCockpit
        selectedSlug="test-board"
        liveSessionNames={new Set()}
        onViewAgentSession={vi.fn()}
      />
    )
    await screen.findByTestId('formation-node-fmn_frame')
    // No fabricated session: the honest hint is shown, no view-agent action.
    expect(screen.getByTestId('no-session-scout')).toBeInTheDocument()
    expect(screen.queryByTestId('view-agent-scout')).toBeNull()
  })

  it('opens the board mission editor through the imperative handle', async () => {
    const ref = createRef<FormationsCockpitHandle>()
    render(<FormationsCockpit ref={ref} selectedSlug="test-board" />)
    await screen.findByTestId('formation-node-fmn_frame')

    ref.current?.openMissionEditor()

    // The shared mission editor popover opens for the board's single mission.
    expect(await screen.findByRole('dialog', { name: /Mission · Showcase/ })).toBeInTheDocument()
    await waitFor(() => expect((screen.getByLabelText('Goal for Showcase') as HTMLTextAreaElement).value).toBe('Build the page'))
  })
})
