import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent, MouseEvent as ReactMouseEvent } from 'react'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import { copyTextToClipboard } from '../utils/clipboard'
import DismissiblePanel from './DismissiblePanel'

interface ScheduledTarget {
  sessionName: string
  unixUser?: string
}

interface ScheduledSchedule {
  type: 'interval' | 'cron'
  expression?: string
  timezone: string
  everyMinutes?: number
}

interface ScheduledRun {
  id: string
  trigger?: string
  startedAt?: string
  finishedAt?: string
  status: string
  message?: string
}

interface ScheduledTask {
  id: string
  name: string
  prompt: string
  target: ScheduledTarget
  schedule: ScheduledSchedule
  enabled: boolean
  paused: boolean
  nextRun?: string
  lastRun?: string
  lastStatus?: string
  createdBy?: string
  updatedBy?: string
  createdAt?: string
  updatedAt?: string
  recentRuns?: ScheduledRun[]
}

interface TmuxSessionOption {
  name: string
  unixUser?: string
  windows?: number
  attached?: boolean
  group?: string
}

interface TaskFormState {
  id?: string
  name: string
  prompt: string
  unixUser: string
  sessionName: string
  scheduleType: 'interval' | 'cron'
  everyMinutes: string
  cronExpression: string
  timezone: string
  createdBy: string
  enabled: boolean
  paused: boolean
}

interface MenuState {
  x: number
  y: number
  taskId: string
}

const emptyForm: TaskFormState = {
  name: '',
  prompt: '',
  unixUser: '',
  sessionName: '',
  scheduleType: 'interval',
  everyMinutes: '15',
  cronExpression: '0 9 * * *',
  timezone: 'UTC',
  createdBy: 'agent:dashboard',
  enabled: true,
  paused: false,
}

function apiErrorMessage(response: unknown, fallback: string): string {
  if (response && typeof response === 'object' && 'error' in response) {
    const error = (response as { error?: { message?: string } }).error
    if (error?.message) return error.message
  }
  return fallback
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-Chrote-Intent': 'scheduled-task',
      ...(init?.headers || {}),
    },
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || payload?.success === false) {
    throw new Error(apiErrorMessage(payload, `Request failed: ${response.status}`))
  }
  return (payload?.data ?? payload) as T
}

function formatSchedule(schedule: ScheduledSchedule): string {
  if (schedule.type === 'interval') return `Every ${schedule.everyMinutes ?? '?'} minutes`
  return `Cron ${schedule.expression || '?'}`
}

function formatDateTime(value?: string): string {
  if (!value) return 'Not scheduled'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function taskToForm(task: ScheduledTask): TaskFormState {
  return {
    id: task.id,
    name: task.name,
    prompt: task.prompt,
    unixUser: task.target.unixUser || '',
    sessionName: task.target.sessionName,
    scheduleType: task.schedule.type,
    everyMinutes: String(task.schedule.everyMinutes || 15),
    cronExpression: task.schedule.expression || '0 9 * * *',
    timezone: task.schedule.timezone || 'UTC',
    createdBy: task.updatedBy || task.createdBy || 'agent:dashboard',
    enabled: task.enabled,
    paused: task.paused,
  }
}

function duplicateTaskForm(task: ScheduledTask): TaskFormState {
  return {
    ...taskToForm(task),
    id: undefined,
    name: `${task.name} copy`,
    createdBy: 'agent:dashboard',
  }
}

function buildRequest(form: TaskFormState) {
  const actor = form.createdBy.trim() || 'agent:dashboard'
  return {
    name: form.name.trim(),
    prompt: form.prompt,
    target: {
      unixUser: form.unixUser.trim(),
      sessionName: form.sessionName.trim(),
    },
    schedule: form.scheduleType === 'interval'
      ? { type: 'interval', everyMinutes: Number(form.everyMinutes), timezone: form.timezone.trim() || 'UTC' }
      : { type: 'cron', expression: form.cronExpression.trim(), timezone: form.timezone.trim() || 'UTC' },
    enabled: form.enabled,
    paused: form.paused,
    createdBy: actor,
    updatedBy: actor,
  }
}

function validateForm(form: TaskFormState): string | null {
  if (!form.name.trim()) return 'Name is required.'
  if (!form.prompt.trim()) return 'Prompt is required.'
  if (!form.sessionName.trim()) return 'Session is required.'
  if (form.scheduleType === 'interval') {
    const minutes = Number(form.everyMinutes)
    if (!Number.isFinite(minutes) || minutes <= 0) return 'Every minutes must be a positive number.'
  }
  if (form.scheduleType === 'cron' && form.cronExpression.trim().split(/\s+/).length !== 5) {
    return 'Cron expression must have five fields.'
  }
  return null
}

function unwrapSessions(data: unknown): TmuxSessionOption[] {
  if (data && typeof data === 'object') {
    const maybe = data as { sessions?: TmuxSessionOption[]; data?: { sessions?: TmuxSessionOption[] } }
    return maybe.sessions || maybe.data?.sessions || []
  }
  return []
}

function ScheduledTasksView() {
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [sessions, setSessions] = useState<TmuxSessionOption[]>([])
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [form, setForm] = useState<TaskFormState>(emptyForm)
  const [formDirty, setFormDirty] = useState(false)
  const [showForm, setShowForm] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [menu, setMenu] = useState<MenuState | null>(null)
  const menuPosition = useViewportMenuPosition<HTMLDivElement>(menu ? { x: menu.x, y: menu.y } : null, {
    estimatedSize: { width: 180, height: 180 },
  })

  const selectedTask = useMemo(
    () => tasks.find(task => task.id === selectedTaskId) || tasks[0] || null,
    [selectedTaskId, tasks],
  )
  const menuTask = useMemo(
    () => tasks.find(task => task.id === menu?.taskId) || null,
    [menu?.taskId, tasks],
  )

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [tasksData, sessionsData] = await Promise.all([
        fetchJSON<{ tasks: ScheduledTask[] }>('/api/scheduled-tasks'),
        fetchJSON<unknown>('/api/tmux/sessions'),
      ])
      setTasks(tasksData.tasks || [])
      setSessions(unwrapSessions(sessionsData))
      setSelectedTaskId(previous => previous && tasksData.tasks?.some(task => task.id === previous) ? previous : tasksData.tasks?.[0]?.id || null)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Failed to load scheduled tasks.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])


  const updateForm = <K extends keyof TaskFormState>(key: K, value: TaskFormState[K]) => {
    setFormDirty(true)
    setForm(previous => ({ ...previous, [key]: value }))
  }

  const canDiscardForm = () => !showForm || !formDirty || window.confirm('Discard unsaved scheduled task changes?')

  const closeForm = () => {
    if (!canDiscardForm()) return
    setShowForm(false)
    setFormDirty(false)
  }

  const startCreate = () => {
    if (!canDiscardForm()) return
    setForm(emptyForm)
    setFormDirty(false)
    setShowForm(true)
    setNotice(null)
  }

  const startEdit = (task: ScheduledTask) => {
    if (!canDiscardForm()) return
    setForm(taskToForm(task))
    setFormDirty(false)
    setSelectedTaskId(task.id)
    setShowForm(true)
    setMenu(null)
    setNotice(null)
  }

  const startDuplicate = (task: ScheduledTask) => {
    if (!canDiscardForm()) return
    setForm(duplicateTaskForm(task))
    setFormDirty(false)
    setShowForm(true)
    setMenu(null)
    setNotice(null)
  }

  const saveForm = async (event: FormEvent) => {
    event.preventDefault()
    const validationError = validateForm(form)
    if (validationError) {
      setError(validationError)
      return
    }
    setError(null)
    try {
      const request = buildRequest(form)
      const data = form.id
        ? await fetchJSON<{ task: ScheduledTask }>(`/api/scheduled-tasks/${encodeURIComponent(form.id)}`, { method: 'PATCH', body: JSON.stringify(request) })
        : await fetchJSON<{ task: ScheduledTask }>('/api/scheduled-tasks', { method: 'POST', body: JSON.stringify(request) })
      setNotice(form.id ? 'Task updated.' : 'Task created.')
      setShowForm(false)
      setFormDirty(false)
      await load()
      setSelectedTaskId(data.task.id)
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : 'Failed to save scheduled task.')
    }
  }

  const actionTask = async (task: ScheduledTask, action: 'run-now' | 'pause' | 'resume') => {
    setMenu(null)
    try {
      await fetchJSON(`/api/scheduled-tasks/${encodeURIComponent(task.id)}/${action}`, { method: 'POST', body: '{}' })
      setNotice(action === 'run-now' ? 'Task sent to tmux.' : action === 'pause' ? 'Task paused.' : 'Task resumed.')
      await load()
    } catch (actionError) {
      setError(actionError instanceof Error ? actionError.message : `Failed to ${action} task.`)
    }
  }

  const deleteTask = async (task: ScheduledTask) => {
    setMenu(null)
    if (!window.confirm(`Delete scheduled task "${task.name}"?`)) return
    try {
      await fetchJSON(`/api/scheduled-tasks/${encodeURIComponent(task.id)}`, { method: 'DELETE' })
      setNotice('Task deleted.')
      await load()
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : 'Failed to delete task.')
    }
  }

  const copyTaskID = async (task: ScheduledTask) => {
    setMenu(null)
    const copied = await copyTextToClipboard(task.id)
    setNotice(copied ? 'Task ID copied.' : task.id)
  }

  const openContextMenu = (event: ReactMouseEvent, task: ScheduledTask) => {
    event.preventDefault()
    setSelectedTaskId(task.id)
    setMenu({ x: event.clientX, y: event.clientY, taskId: task.id })
  }

  return (
    <div className="scheduled-view">
      <header className="scheduled-header">
        <div>
          <h2>Scheduled Tasks</h2>
          <p>Persisted CHROTE jobs that send explicit prompts into tmux sessions.</p>
        </div>
        <button className="scheduled-primary" type="button" onClick={startCreate}>New Task</button>
      </header>

      {error && <div className="scheduled-alert" role="alert">{error}</div>}
      {notice && <div className="scheduled-notice" role="status">{notice}</div>}

      <div className="scheduled-grid">
        <section className="scheduled-list" aria-label="Scheduled task list">
          {loading ? (
            <div className="scheduled-empty">Loading scheduled tasks...</div>
          ) : tasks.length === 0 ? (
            <div className="scheduled-empty">No scheduled tasks yet. Create one to send prompts into tmux later.</div>
          ) : (
            tasks.map(task => (
              <button
                key={task.id}
                className={`scheduled-task-row ${task.id === selectedTask?.id ? 'active' : ''}`}
                type="button"
                onClick={() => setSelectedTaskId(task.id)}
                onContextMenu={(event) => openContextMenu(event, task)}
              >
                <span className="scheduled-task-title">{task.name}</span>
                <span>{task.target.unixUser || 'default'} / {task.target.sessionName}</span>
                <span>{formatSchedule(task.schedule)}</span>
                <span className={task.paused || !task.enabled ? 'scheduled-muted' : 'scheduled-good'}>
                  {task.paused ? 'Paused' : task.enabled ? 'Enabled' : 'Disabled'}
                </span>
              </button>
            ))
          )}
        </section>

        <section className="scheduled-detail" aria-label="Scheduled task details">
          {showForm ? (
            <form className="scheduled-form" onSubmit={saveForm}>
              <div className="scheduled-form-header">
                <h3>{form.id ? 'Edit scheduled task' : 'Create scheduled task'}</h3>
                <button type="button" className="scheduled-secondary" onClick={closeForm}>Cancel</button>
              </div>
              <label>
                <span>Name</span>
                <input value={form.name} onChange={(event) => updateForm('name', event.target.value)} />
              </label>
              <label>
                <span>Prompt</span>
                <textarea value={form.prompt} onChange={(event) => updateForm('prompt', event.target.value)} />
              </label>
              <div className="scheduled-form-row">
                <label>
                  <span>Unix user</span>
                  <input list="scheduled-users" value={form.unixUser} onChange={(event) => updateForm('unixUser', event.target.value)} />
                </label>
                <label>
                  <span>Session</span>
                  <input list="scheduled-sessions" value={form.sessionName} onChange={(event) => updateForm('sessionName', event.target.value)} />
                </label>
              </div>
              <datalist id="scheduled-users">
                {Array.from(new Set(sessions.map(session => session.unixUser).filter(Boolean))).map(user => <option key={user} value={user} />)}
              </datalist>
              <datalist id="scheduled-sessions">
                {sessions.map(session => <option key={`${session.unixUser || ''}:${session.name}`} value={session.name}>{session.unixUser || 'default'}</option>)}
              </datalist>
              <div className="scheduled-form-row">
                <label>
                  <span>Schedule type</span>
                  <select value={form.scheduleType} onChange={(event) => updateForm('scheduleType', event.target.value as 'interval' | 'cron')}>
                    <option value="interval">Interval</option>
                    <option value="cron">Cron</option>
                  </select>
                </label>
                {form.scheduleType === 'interval' ? (
                  <label>
                    <span>Every minutes</span>
                    <input type="number" min="1" value={form.everyMinutes} onChange={(event) => updateForm('everyMinutes', event.target.value)} />
                  </label>
                ) : (
                  <label>
                    <span>Cron expression</span>
                    <input value={form.cronExpression} onChange={(event) => updateForm('cronExpression', event.target.value)} />
                  </label>
                )}
                <label>
                  <span>Timezone</span>
                  <input value={form.timezone} onChange={(event) => updateForm('timezone', event.target.value)} />
                </label>
              </div>
              <div className="scheduled-form-row compact">
                <label>
                  <span>Created by</span>
                  <input value={form.createdBy} onChange={(event) => updateForm('createdBy', event.target.value)} />
                </label>
                <label className="scheduled-checkbox">
                  <input type="checkbox" checked={form.enabled} onChange={(event) => updateForm('enabled', event.target.checked)} />
                  <span>Enabled</span>
                </label>
                <label className="scheduled-checkbox">
                  <input type="checkbox" checked={form.paused} onChange={(event) => updateForm('paused', event.target.checked)} />
                  <span>Paused</span>
                </label>
              </div>
              <button className="scheduled-primary" type="submit">{form.id ? 'Save Task' : 'Create Task'}</button>
            </form>
          ) : selectedTask ? (
            <div className="scheduled-card">
              <div className="scheduled-card-header">
                <div>
                  <h3>{selectedTask.name}</h3>
                  <p>{selectedTask.id}</p>
                </div>
                <div className="scheduled-actions">
                  <button type="button" onClick={() => startEdit(selectedTask)}>Edit</button>
                  <button type="button" onClick={() => void actionTask(selectedTask, 'run-now')}>Run Now</button>
                  <button type="button" onClick={() => void actionTask(selectedTask, selectedTask.paused ? 'resume' : 'pause')}>{selectedTask.paused ? 'Resume' : 'Pause'}</button>
                  <button type="button" onClick={() => startDuplicate(selectedTask)}>Duplicate</button>
                  <button type="button" className="danger" onClick={() => void deleteTask(selectedTask)}>Delete</button>
                </div>
              </div>
              <dl className="scheduled-meta">
                <div><dt>Target</dt><dd>{selectedTask.target.unixUser || 'default'} / {selectedTask.target.sessionName}</dd></div>
                <div><dt>Schedule</dt><dd>{formatSchedule(selectedTask.schedule)} ({selectedTask.schedule.timezone || 'Local'})</dd></div>
                <div><dt>Next run</dt><dd>{formatDateTime(selectedTask.nextRun)}</dd></div>
                <div><dt>Last status</dt><dd>{selectedTask.lastStatus || 'Never run'}</dd></div>
                <div><dt>Agent metadata</dt><dd>createdBy {selectedTask.createdBy || 'unknown'} · updatedBy {selectedTask.updatedBy || 'unknown'}</dd></div>
              </dl>
              <label className="scheduled-prompt-readback">
                <span>Prompt</span>
                <textarea readOnly value={selectedTask.prompt} />
              </label>
            </div>
          ) : (
            <div className="scheduled-empty">Select or create a scheduled task.</div>
          )}
        </section>
      </div>

      {menu && menuTask && (
        <DismissiblePanel onDismiss={() => setMenu(null)} panelPosition="fixed">
          <div ref={menuPosition.ref} className="session-context-menu scheduled-task-menu" style={menuPosition.style}>
            <button className="session-context-item" type="button" onClick={() => startEdit(menuTask)}>Edit</button>
            <button className="session-context-item" type="button" onClick={() => void actionTask(menuTask, 'run-now')}>Run Now</button>
            <button className="session-context-item" type="button" onClick={() => void actionTask(menuTask, menuTask.paused ? 'resume' : 'pause')}>{menuTask.paused ? 'Resume' : 'Pause'}</button>
            <button className="session-context-item" type="button" onClick={() => startDuplicate(menuTask)}>Duplicate</button>
            <button className="session-context-item" type="button" onClick={() => void copyTaskID(menuTask)}>Copy ID</button>
            <button className="session-context-item session-context-danger" type="button" onClick={() => void deleteTask(menuTask)}>Delete</button>
          </div>
        </DismissiblePanel>
      )}
    </div>
  )
}

export default ScheduledTasksView
