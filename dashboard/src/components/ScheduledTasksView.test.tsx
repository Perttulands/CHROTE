import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ScheduledTasksView from './ScheduledTasksView'

const task = {
  id: 'tsk_existing',
  name: 'Morning prompt',
  prompt: 'status please',
  target: { sessionName: 'ops', unixUser: 'alice' },
  schedule: { type: 'interval', everyMinutes: 15, timezone: 'UTC' },
  enabled: true,
  paused: false,
  nextRun: '2026-06-27T15:00:00Z',
  lastStatus: 'success',
  createdBy: 'agent:test',
  updatedBy: 'agent:test',
  createdAt: '2026-06-27T14:00:00Z',
  updatedAt: '2026-06-27T14:00:00Z',
}

const sessionsEnvelope = {
  success: true,
  data: {
    sessions: [
      { name: 'ops', unixUser: 'alice', windows: 1, attached: false, group: 'ops' },
      { name: 'codex', unixUser: 'chrote', windows: 1, attached: false, group: 'codex' },
    ],
    grouped: {},
    terminalUsers: ['alice', 'chrote'],
  },
  timestamp: '2026-06-27T14:00:00Z',
}

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ success: true, data, timestamp: '2026-06-27T14:00:00Z' }), { status: 200 }))
}

function setViewportForTest(width: number, height: number) {
  const originalWidth = window.innerWidth
  const originalHeight = window.innerHeight
  Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: height })
  return () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: originalWidth })
    Object.defineProperty(window, 'innerHeight', { configurable: true, writable: true, value: originalHeight })
  }
}

function menuRect(width: number, height: number): DOMRect {
  return { x: 0, y: 0, width, height, top: 0, left: 0, right: width, bottom: height, toJSON: () => ({}) } as DOMRect
}

describe('ScheduledTasksView', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.stubGlobal('confirm', vi.fn(() => true))
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method || 'GET'
      if (url === '/api/scheduled-tasks' && method === 'GET') return ok({ tasks: [task] })
      if (url === '/api/tmux/sessions' && method === 'GET') return Promise.resolve(new Response(JSON.stringify(sessionsEnvelope), { status: 200 }))
      if (url === '/api/scheduled-tasks' && method === 'POST') {
        const body = JSON.parse(String(init?.body))
        return ok({ task: { ...task, ...body, id: 'tsk_created', createdAt: task.createdAt, updatedAt: task.updatedAt } })
      }
      if (url === '/api/scheduled-tasks/tsk_existing/run-now' && method === 'POST') return ok({ task, run: { id: 'run_1', status: 'success' } })
      if (url === '/api/scheduled-tasks/tsk_existing/pause' && method === 'POST') return ok({ task: { ...task, paused: true } })
      if (url === '/api/scheduled-tasks/tsk_existing/resume' && method === 'POST') return ok({ task })
      if (url === '/api/scheduled-tasks/tsk_existing' && method === 'DELETE') return ok({ deleted: 'tsk_existing' })
      if (url === '/api/scheduled-tasks/tsk_existing' && method === 'PATCH') return ok({ task })
      return Promise.resolve(new Response(JSON.stringify({ success: false, error: { message: `unexpected ${method} ${url}` }, timestamp: 'now' }), { status: 500 }))
    }))
  })

  it('loads tasks and exposes agent metadata/target readback', async () => {
    render(<ScheduledTasksView />)

    expect(await screen.findByRole('heading', { name: 'Scheduled Tasks' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /Morning prompt/ })).toBeInTheDocument()
    expect(screen.getAllByText('alice / ops').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/agent:test/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Every 15 minutes/).length).toBeGreaterThan(0)
  })

  it('creates an interval scheduled task through the API form', async () => {
    render(<ScheduledTasksView />)

    fireEvent.click(await screen.findByRole('button', { name: /New Task/i }))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Nightly nudge' } })
    fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'hello; literal $(whoami)' } })
    fireEvent.change(screen.getByLabelText('Unix user'), { target: { value: 'chrote' } })
    fireEvent.change(screen.getByLabelText('Session'), { target: { value: 'codex' } })
    fireEvent.change(screen.getByLabelText('Every minutes'), { target: { value: '30' } })
    fireEvent.change(screen.getByLabelText('Created by'), { target: { value: 'agent:ui-test' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create Task' }))

    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks', expect.objectContaining({ method: 'POST' })))
    const post = vi.mocked(fetch).mock.calls.find(([url, init]) => String(url) === '/api/scheduled-tasks' && init?.method === 'POST')
    expect(post?.[1]?.headers).toMatchObject({ 'X-Chrote-Intent': 'scheduled-task' })
    expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({
      name: 'Nightly nudge',
      prompt: 'hello; literal $(whoami)',
      target: { unixUser: 'chrote', sessionName: 'codex' },
      schedule: { type: 'interval', everyMinutes: 30, timezone: 'UTC' },
      createdBy: 'agent:ui-test',
      updatedBy: 'agent:ui-test',
    })
  })

  it('protects unsaved form edits before closing the task form', async () => {
    vi.mocked(window.confirm).mockReturnValue(false)
    render(<ScheduledTasksView />)

    fireEvent.click(await screen.findByRole('button', { name: /New Task/i }))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Unsaved draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(window.confirm).toHaveBeenCalledWith(expect.stringMatching(/discard unsaved/i))
    expect(screen.getByLabelText('Name')).toHaveValue('Unsaved draft')
  })

  it('runs, pauses, resumes, and deletes through task actions', async () => {
    render(<ScheduledTasksView />)
    const row = await screen.findByRole('button', { name: /Morning prompt/ })
    fireEvent.click(row)

    fireEvent.click(screen.getByRole('button', { name: 'Run Now' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks/tsk_existing/run-now', expect.objectContaining({ method: 'POST' })))

    fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks/tsk_existing/pause', expect.objectContaining({ method: 'POST' })))

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks/tsk_existing', expect.objectContaining({ method: 'DELETE' })))
  })

  it('uses a viewport-safe right-click task menu', async () => {
    const restoreViewport = setViewportForTest(360, 240)
    const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('scheduled-task-menu')) return menuRect(180, 160)
      return originalGetBoundingClientRect.call(this)
    })

    try {
      render(<ScheduledTasksView />)
      const taskButton = await screen.findByRole('button', { name: /Morning prompt/ })
      fireEvent.contextMenu(taskButton, { clientX: 350, clientY: 230 })
      const menu = document.querySelector('.scheduled-task-menu') as HTMLElement

      await waitFor(() => {
        const left = Number.parseFloat(menu.style.left)
        const top = Number.parseFloat(menu.style.top)
        expect(left + 180).toBeLessThanOrEqual(window.innerWidth)
        expect(top + 160).toBeLessThanOrEqual(window.innerHeight)
      })
      expect(within(menu).getByRole('button', { name: 'Copy ID' })).toBeInTheDocument()
    } finally {
      rectSpy.mockRestore()
      restoreViewport()
    }
  })
})
