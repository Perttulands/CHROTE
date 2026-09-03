import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { DndContext } from '@dnd-kit/core'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ScheduledTasksView from './ScheduledTasksView'
import { DEFAULT_SETTINGS } from '../types'
import { DEFAULT_SESSIONS_DOCK_STATE } from './workspaceFilesState'

// The view listens to the ancestor DndContext through useDndMonitor. Capture the
// listener so a drop can be replayed exactly as DndContext would dispatch it.
const dndMonitor = vi.hoisted(() => ({ listener: null as null | { onDragEnd?: (event: unknown) => void } }))

vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  return {
    ...actual,
    useDndMonitor: (listener: { onDragEnd?: (event: unknown) => void }) => {
      dndMonitor.listener = listener
    },
  }
})

function dropSession(target: { sessionName: string; unixUser?: string }, overId = 'scheduled-task-targets') {
  act(() => {
    dndMonitor.listener?.onDragEnd?.({
      active: { id: `${target.unixUser || ''}:${target.sessionName}`, data: { current: { type: 'session', ...target } } },
      over: { id: overId },
    })
  })
}

const sessions = [
  { name: 'claude-chrote-worker', unixUser: 'build', windows: 1, attached: true, group: 'claude' },
  { name: 'claude-chrote-worker-2', unixUser: 'build', windows: 1, attached: true, group: 'claude' },
  { name: 'ops', unixUser: 'alice', windows: 1, attached: false, group: 'ops' },
]

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions,
    groupedSessions: {},
    loading: false,
    error: null,
    sidebarCollapsed: false,
    refreshSessions: vi.fn(),
    createSession: vi.fn(),
    settings: DEFAULT_SETTINGS,
    terminalUsers: ['build', 'alice'],
  }),
}))

const task = {
  id: 'tsk_existing',
  name: 'Continue work',
  prompt: 'Continue if work is clear',
  targets: [
    { sessionName: 'claude-chrote-worker', unixUser: 'build' },
    { sessionName: 'claude-chrote-worker-2', unixUser: 'build' },
  ],
  schedule: { type: 'cron', expression: '0 16 * * *', timezone: 'Europe/Helsinki' },
  enabled: true,
  paused: false,
  nextRun: '2026-06-27T15:00:00Z',
  lastStatus: 'success',
  createdBy: 'user:dashboard',
  updatedBy: 'user:dashboard',
  recentRuns: [
    {
      id: 'run_1',
      trigger: 'scheduled',
      startedAt: '2026-06-26T13:00:00Z',
      status: 'partial',
      message: 'claude-chrote-worker-2: session is gone',
      targets: [
        { sessionName: 'claude-chrote-worker', status: 'success', pane: '%1' },
        { sessionName: 'claude-chrote-worker-2', status: 'error', message: 'session is gone' },
      ],
    },
  ],
}

function ok(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ success: true, data, timestamp: '2026-06-27T14:00:00Z' }), { status: 200 }))
}

function renderView() {
  return render(
    <DndContext>
      <ScheduledTasksView
        activeWorkspaceId="terminal2"
        sessionsDockState={{ ...DEFAULT_SESSIONS_DOCK_STATE, open: true }}
        onSessionsDockStateChange={vi.fn()}
        sessionsForcedPinned={false}
      />
    </DndContext>,
  )
}

describe('ScheduledTasksView', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.stubGlobal('confirm', vi.fn(() => true))
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method || 'GET'
      if (url === '/api/scheduled-tasks' && method === 'GET') return ok({ tasks: [task] })
      if (url === '/api/scheduled-tasks' && method === 'POST') {
        return ok({ task: { ...task, ...JSON.parse(String(init?.body)), id: 'tsk_created' } })
      }
      if (url === '/api/scheduled-tasks/tsk_existing' && method === 'PATCH') return ok({ task })
      if (url === '/api/scheduled-tasks/tsk_existing' && method === 'DELETE') return ok({ deleted: 'tsk_existing' })
      if (url.startsWith('/api/scheduled-tasks/tsk_existing/') && method === 'POST') {
        return ok({ task, run: { id: 'run_2', status: 'success', targets: task.targets.map(t => ({ ...t, status: 'success' })) } })
      }
      return Promise.resolve(new Response(JSON.stringify({ success: false, error: { message: `unexpected ${method} ${url}` } }), { status: 500 }))
    }))
  })

  it('lists tasks with plain-language schedules and every target session', async () => {
    renderView()

    expect(await screen.findByRole('heading', { name: 'Scheduled Tasks' })).toBeInTheDocument()
    const card = await screen.findByRole('button', { name: /Continue work/ })
    expect(within(card).getByText(/Daily at 16:00/)).toBeInTheDocument()
    expect(within(card).getByText('Europe/Helsinki', { exact: false })).toBeInTheDocument()
    expect(within(card).getByText('claude-chrote-worker')).toBeInTheDocument()
    expect(within(card).getByText('claude-chrote-worker-2')).toBeInTheDocument()
  })

  it('shows per-target run history so a partial delivery is visible', async () => {
    renderView()

    fireEvent.click(await screen.findByRole('button', { name: /Continue work/ }))
    expect(await screen.findByText(/claude-chrote-worker: success, claude-chrote-worker-2: error/)).toBeInTheDocument()
    expect(screen.getByText(/session is gone/)).toBeInTheDocument()
  })

  it('creates a daily multi-session task without asking for a unix user or cron string', async () => {
    renderView()

    fireEvent.click(await screen.findByRole('button', { name: /New Task/i }))
    fireEvent.change(screen.getByLabelText('Prompt'), {
      target: { value: 'Continue if work is clear, keep things moving' },
    })
    fireEvent.click(screen.getByRole('button', { name: /claude-chrote-worker$/ }))
    fireEvent.click(screen.getByRole('button', { name: /claude-chrote-worker-2$/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Daily' }))
    fireEvent.change(screen.getByLabelText('Time of day'), { target: { value: '16:00' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create task' }))

    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks', expect.objectContaining({ method: 'POST' })))
    const post = vi.mocked(fetch).mock.calls.find(([url, init]) => String(url) === '/api/scheduled-tasks' && init?.method === 'POST')
    expect(post?.[1]?.headers).toMatchObject({ 'X-Chrote-Intent': 'scheduled-task' })
    const body = JSON.parse(String(post?.[1]?.body))
    expect(body).toMatchObject({
      prompt: 'Continue if work is clear, keep things moving',
      targets: [
        { sessionName: 'claude-chrote-worker', unixUser: 'build' },
        { sessionName: 'claude-chrote-worker-2', unixUser: 'build' },
      ],
      schedule: { type: 'cron', expression: '0 16 * * *' },
    })
    // The name is derived from the prompt when the operator leaves it blank.
    expect(body.name).toBe('Continue if work is clear, keep things moving')
    expect(screen.queryByLabelText(/unix user/i)).not.toBeInTheDocument()
  })

  it('adds a target when a session is dropped on the editor target zone', async () => {
    renderView()

    fireEvent.click(await screen.findByRole('button', { name: /New Task/i }))
    const dropzone = screen.getByTestId('scheduled-target-dropzone')
    expect(within(dropzone).getByText('No sessions selected')).toBeInTheDocument()

    // jsdom cannot perform a real pointer drag; drive the registered dnd-kit
    // monitor listener with the event DndContext would dispatch.
    dropSession({ sessionName: 'ops', unixUser: 'alice' })

    await waitFor(() => expect(within(dropzone).getByLabelText('Remove ops')).toBeInTheDocument())
  })

  it('ignores drops that land outside the target zone', async () => {
    renderView()
    fireEvent.click(await screen.findByRole('button', { name: /New Task/i }))

    dropSession({ sessionName: 'ops', unixUser: 'alice' }, 'some-terminal-window')

    expect(within(screen.getByTestId('scheduled-target-dropzone')).getByText('No sessions selected')).toBeInTheDocument()
  })

  it('refuses to save a task with no target session', async () => {
    renderView()

    fireEvent.click(await screen.findByRole('button', { name: /New Task/i }))
    fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'no targets yet' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create task' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/at least one target session/i)
    expect(vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(0)
  })

  it('runs, pauses, and deletes the selected task', async () => {
    renderView()
    fireEvent.click(await screen.findByRole('button', { name: /Continue work/ }))

    fireEvent.click(screen.getByRole('button', { name: 'Run now' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks/tsk_existing/run-now', expect.objectContaining({ method: 'POST' })))
    expect(await screen.findByText(/Sent to 2 sessions/)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks/tsk_existing/pause', expect.objectContaining({ method: 'POST' })))

    // Delete confirms in place: the first press arms the button, the second runs it.
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith('/api/scheduled-tasks/tsk_existing', expect.objectContaining({ method: 'DELETE' })))
  })

  it('loads an existing task back into the simple editor', async () => {
    renderView()
    fireEvent.click(await screen.findByRole('button', { name: /Continue work/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))

    expect(screen.getByLabelText('Prompt')).toHaveValue('Continue if work is clear')
    expect(screen.getByRole('button', { name: 'Daily' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByLabelText('Time of day')).toHaveValue('16:00')
    expect(screen.getByLabelText('Timezone')).toHaveValue('Europe/Helsinki')
  })

  it('protects unsaved edits until the discard is repeated', async () => {
    renderView()

    fireEvent.click(await screen.findByRole('button', { name: /New Task/i }))
    fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'unsaved draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    // The first Cancel says what is at stake and keeps the draft where it is.
    expect(screen.getByText(/Repeat within three seconds to discard/i)).toBeInTheDocument()
    expect(screen.getByLabelText('Prompt')).toHaveValue('unsaved draft')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByLabelText('Prompt')).not.toBeInTheDocument()
  })
})
