import { DndContext } from '@dnd-kit/core'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import TerminalWindow from './TerminalWindow'
import { IframePoolProvider } from './IframePool'
import { SessionProvider, useSession } from '../context/SessionContext'
import { ToastProvider } from '../context/ToastContext'
import type { TerminalWindow as TerminalWindowType } from '../types'

const emptyWindow: TerminalWindowType = {
  id: 'terminal1-window-0',
  boundSessions: [],
  activeSession: null,
  colorIndex: 0,
}

const fetchMock = vi.fn()

vi.stubGlobal('fetch', fetchMock)

vi.stubGlobal('ResizeObserver', class {
  observe = vi.fn()
  disconnect = vi.fn()
})

function BoundSessionsProbe() {
  const { workspaces } = useSession()
  return (
    <div data-testid="bound-sessions">
      {workspaces.terminal1.windows[0].boundSessions.join(',')}
    </div>
  )
}

function renderWindow() {
  return render(
    createElement(ToastProvider, null,
      createElement(SessionProvider, null,
        createElement(DndContext, null,
          createElement(IframePoolProvider, null,
            createElement(TerminalWindow, {
              workspaceId: 'terminal1',
              window: emptyWindow,
            }),
            createElement(BoundSessionsProbe)
          )
        )
      )
    )
  )
}

function mockFetch() {
  fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'

    if (url.includes('/api/gascity/observer')) {
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({
          success: true,
          data: {
            status: 'ok',
            checkedAt: '2026-05-30T00:00:00Z',
            sessions: [],
          },
        }),
        text: () => Promise.resolve(''),
      })
    }

    if (url.includes('/api/gascity/sessions') && method === 'POST') {
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({
          success: true,
          data: {
            source: 'gascity',
            id: 'ga-7777',
            name: 'codxia',
            sessionName: 'codxia',
            alias: 'codxia',
            title: 'Codxia',
            template: 'codex-smoke',
            transport: 'tmux',
            workDir: '/tmp/codxia',
            deferredStart: false,
            attached: false,
            attachTarget: 'gc:ga-7777',
          },
        }),
        text: () => Promise.resolve(''),
      })
    }

    if (url.includes('/api/tmux/sessions') && method === 'POST') {
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ success: true }),
        text: () => Promise.resolve(''),
      })
    }

    if (url.includes('/api/tmux/sessions')) {
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({
          sessions: [],
          grouped: {},
          timestamp: '2026-05-30T00:00:00Z',
        }),
        text: () => Promise.resolve(''),
      })
    }

    return Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ success: true }),
      text: () => Promise.resolve(''),
    })
  })
}

describe('TerminalWindow session creation', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    mockFetch()
    window.localStorage.clear()
    vi.spyOn(Date, 'now').mockReturnValue(1770000000000)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps the plain tmux create path and binds the new session to the empty window', async () => {
    renderWindow()

    fireEvent.click(screen.getByRole('button', { name: /new session/i }))

    await waitFor(() => {
      expect(screen.getByTestId('bound-sessions').textContent).toMatch(/^shell-/)
    })
    const tmuxPost = fetchMock.mock.calls.find(([input, init]) =>
      String(input).includes('/api/tmux/sessions') && init?.method === 'POST'
    )
    expect(tmuxPost).toBeTruthy()
    const body = JSON.parse(tmuxPost![1]!.body as string)
    expect(body.name).toBe(screen.getByTestId('bound-sessions').textContent)
  })

  it('creates a native Gas City identity and binds the returned gc attach target', async () => {
    renderWindow()

    fireEvent.click(screen.getByRole('button', { name: 'Gas City' }))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'codxia' } })
    fireEvent.change(screen.getByLabelText('Template'), { target: { value: 'codex-smoke' } })
    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Codxia' } })
    fireEvent.click(screen.getByRole('button', { name: /new identity/i }))

    await waitFor(() => {
      expect(screen.getByTestId('bound-sessions')).toHaveTextContent('gc:ga-7777')
    })

    const gasCityPost = fetchMock.mock.calls.find(([input, init]) =>
      String(input).includes('/api/gascity/sessions') && init?.method === 'POST'
    )
    expect(gasCityPost).toBeTruthy()
    expect(JSON.parse(gasCityPost![1]!.body as string)).toEqual({
      name: 'codxia',
      template: 'codex-smoke',
      title: 'Codxia',
    })

    const createForm = screen.getByRole('button', { name: /new identity/i }).closest('.create-session-controls')
    expect(createForm).not.toBeNull()
    expect(within(createForm as HTMLElement).getByDisplayValue('codex-smoke')).toBeInTheDocument()
  })
})
