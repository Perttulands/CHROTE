import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Dispatch, FormEvent, SetStateAction } from 'react'
import { useDndMonitor, useDroppable } from '@dnd-kit/core'
import { CalendarClock, Plus, SquareTerminal } from 'lucide-react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getTerminalUserInitial } from '../types'
import type { WorkspaceId } from '../types'
import SessionPanel from './SessionPanel'
import ScheduleEditor from './ScheduleEditor'
import { CONFIRM_WINDOW_MS, useConfirmInPlace } from './confirmInPlace'
import type { SessionsDockState } from './workspaceFilesState'
import TableColumn from './TableColumn'
import {
  describeSchedule,
  emptyScheduleForm,
  scheduleFormError,
  scheduleFromForm,
  scheduleToForm,
  type ScheduleForm,
  type ScheduledSchedule,
} from './scheduledSchedule'

interface ScheduledTarget {
  sessionName: string
  unixUser?: string
}

interface ScheduledTargetRun {
  sessionName: string
  unixUser?: string
  status: string
  pane?: string
  message?: string
}

interface ScheduledRun {
  id: string
  trigger?: string
  startedAt?: string
  finishedAt?: string
  status: string
  message?: string
  targets?: ScheduledTargetRun[]
}

interface ScheduledTask {
  id: string
  name: string
  prompt: string
  targets?: ScheduledTarget[]
  target?: ScheduledTarget
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

interface TaskForm {
  id?: string
  name: string
  prompt: string
  targets: ScheduledTarget[]
  schedule: ScheduleForm
  paused: boolean
}

const DROPZONE_ID = 'scheduled-task-targets'
const ACTOR = 'user:dashboard'

interface ScheduledTasksViewProps {
  activeWorkspaceId: WorkspaceId
  sessionsDockState: SessionsDockState
  onSessionsDockStateChange: Dispatch<SetStateAction<SessionsDockState>>
  sessionsForcedPinned: boolean
}

function newTaskForm(targets: ScheduledTarget[] = []): TaskForm {
  return { name: '', prompt: '', targets, schedule: emptyScheduleForm(), paused: false }
}

export function taskTargets(task: ScheduledTask): ScheduledTarget[] {
  if (task.targets && task.targets.length > 0) return task.targets
  return task.target ? [task.target] : []
}

function targetKey(target: ScheduledTarget): string {
  return `${target.unixUser || ''}:${target.sessionName}`
}

function targetLabel(target: ScheduledTarget): string {
  return target.sessionName
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

function formatDateTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// formatCountdown keeps liveness as text rather than a blinking indicator.
export function formatCountdown(value: string | undefined, now: number): string {
  if (!value) return ''
  const target = new Date(value).getTime()
  if (Number.isNaN(target)) return ''
  const seconds = Math.round((target - now) / 1000)
  if (seconds <= 0) return 'due now'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `in ${days}d ${hours}h`
  if (hours > 0) return `in ${hours}h ${minutes}m`
  if (minutes > 0) return `in ${minutes}m`
  return 'in <1m'
}

function statusLabel(task: ScheduledTask): string {
  if (task.paused) return 'Paused'
  if (!task.enabled) return 'Disabled'
  return 'Active'
}

function ScheduledTasksView({
  activeWorkspaceId,
  sessionsDockState,
  onSessionsDockStateChange,
  sessionsForcedPinned,
}: ScheduledTasksViewProps) {
  const { sessions } = useSession()
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [form, setForm] = useState<TaskForm | null>(null)
  const [formDirty, setFormDirty] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const formRef = useRef<TaskForm | null>(null)
  formRef.current = form

  const selectedTask = useMemo(
    () => tasks.find(task => task.id === selectedTaskId) || null,
    [selectedTaskId, tasks],
  )

  const load = useCallback(async (): Promise<ScheduledTask[]> => {
    setError(null)
    try {
      const data = await fetchJSON<{ tasks: ScheduledTask[] }>('/api/scheduled-tasks')
      const loaded = data.tasks || []
      setTasks(loaded)
      setSelectedTaskId(previous => (previous && loaded.some(task => task.id === previous) ? previous : loaded[0]?.id || null))
      return loaded
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Failed to load scheduled tasks.')
      return []
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  // Countdowns tick once a minute; the list itself only reloads after actions.
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])

  const updateForm = useCallback((update: (previous: TaskForm) => TaskForm) => {
    setFormDirty(true)
    setForm(previous => (previous ? update(previous) : previous))
  }, [])

  const addTarget = useCallback((target: ScheduledTarget) => {
    setNotice(null)
    setFormDirty(true)
    setForm(previous => {
      const base = previous ?? newTaskForm()
      if (base.targets.some(existing => targetKey(existing) === targetKey(target))) return base
      return { ...base, targets: [...base.targets, target] }
    })
  }, [])

  // Sessions dragged out of the sidecar land on the editor's target zone. The
  // ancestor DndContext lives in App, so this view only listens.
  useDndMonitor({
    onDragEnd(event) {
      if (event.over?.id !== DROPZONE_ID) return
      const data = event.active.data.current as { sessionName?: string; unixUser?: string } | undefined
      if (!data?.sessionName) return
      addTarget({ sessionName: data.sessionName, ...(data.unixUser ? { unixUser: data.unixUser } : {}) })
    },
  })

  const { isOver, setNodeRef: setDropRef } = useDroppable({ id: DROPZONE_ID })

  // Leaving a dirty form confirms in place: the same gesture, repeated within
  // three seconds, discards. Nothing is torn out of the way to ask.
  const discardArmedAt = useRef(0)
  const canDiscardForm = useCallback(() => {
    if (!formRef.current || !formDirty) return true
    if (Date.now() - discardArmedAt.current < CONFIRM_WINDOW_MS) {
      discardArmedAt.current = 0
      return true
    }
    discardArmedAt.current = Date.now()
    setNotice('Unsaved scheduled task changes. Repeat within three seconds to discard them.')
    return false
  }, [formDirty])

  const startCreate = useCallback((targets: ScheduledTarget[] = []) => {
    if (!canDiscardForm()) return
    setForm(newTaskForm(targets))
    setFormDirty(false)
    setNotice(null)
  }, [canDiscardForm])

  const startEdit = useCallback((task: ScheduledTask) => {
    if (!canDiscardForm()) return
    setSelectedTaskId(task.id)
    setForm({
      id: task.id,
      name: task.name,
      prompt: task.prompt,
      targets: taskTargets(task),
      schedule: scheduleToForm(task.schedule),
      paused: task.paused,
    })
    setFormDirty(false)
    setNotice(null)
  }, [canDiscardForm])

  const startDuplicate = useCallback((task: ScheduledTask) => {
    if (!canDiscardForm()) return
    setForm({
      name: `${task.name} copy`,
      prompt: task.prompt,
      targets: taskTargets(task),
      schedule: scheduleToForm(task.schedule),
      paused: task.paused,
    })
    setFormDirty(false)
    setNotice(null)
  }, [canDiscardForm])

  const closeForm = useCallback(() => {
    if (!canDiscardForm()) return
    setForm(null)
    setFormDirty(false)
  }, [canDiscardForm])

  const saveForm = useCallback(async (event: FormEvent) => {
    event.preventDefault()
    if (!form) return
    if (!form.prompt.trim()) {
      setError('Prompt is required.')
      return
    }
    if (form.targets.length === 0) {
      setError('Add at least one target session.')
      return
    }
    const scheduleError = scheduleFormError(form.schedule)
    if (scheduleError) {
      setError(scheduleError)
      return
    }

    const name = form.name.trim() || form.prompt.trim().split('\n')[0].slice(0, 60)
    const request = {
      name,
      prompt: form.prompt,
      targets: form.targets,
      schedule: scheduleFromForm(form.schedule),
      enabled: true,
      paused: form.paused,
      createdBy: ACTOR,
      updatedBy: ACTOR,
    }

    setError(null)
    setBusy(true)
    try {
      const data = form.id
        ? await fetchJSON<{ task: ScheduledTask }>(`/api/scheduled-tasks/${encodeURIComponent(form.id)}`, {
            method: 'PATCH',
            body: JSON.stringify(request),
          })
        : await fetchJSON<{ task: ScheduledTask }>('/api/scheduled-tasks', { method: 'POST', body: JSON.stringify(request) })
      setForm(null)
      setFormDirty(false)
      setNotice(form.id ? 'Task updated.' : 'Task created.')
      await load()
      setSelectedTaskId(data.task.id)
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : 'Failed to save scheduled task.')
    } finally {
      setBusy(false)
    }
  }, [form, load])

  const actionTask = useCallback(async (task: ScheduledTask, action: 'run-now' | 'pause' | 'resume') => {
    setError(null)
    setBusy(true)
    try {
      const data = await fetchJSON<{ task: ScheduledTask; run?: ScheduledRun }>(
        `/api/scheduled-tasks/${encodeURIComponent(task.id)}/${action}`,
        { method: 'POST', body: JSON.stringify({ updatedBy: ACTOR }) },
      )
      if (action === 'run-now') {
        const run = data.run
        const delivered = run?.targets?.filter(result => result.status === 'success').length ?? 0
        const total = run?.targets?.length ?? taskTargets(task).length
        setNotice(run?.status === 'success'
          ? `Sent to ${total} ${total === 1 ? 'session' : 'sessions'}.`
          : `Sent to ${delivered} of ${total}: ${run?.message || 'delivery failed'}`)
      } else {
        setNotice(action === 'pause' ? 'Task paused.' : 'Task resumed.')
      }
      await load()
    } catch (actionError) {
      setError(actionError instanceof Error ? actionError.message : `Failed to ${action} task.`)
    } finally {
      setBusy(false)
    }
  }, [load])

  const deleteTask = useCallback(async (task: ScheduledTask) => {
    setBusy(true)
    try {
      await fetchJSON(`/api/scheduled-tasks/${encodeURIComponent(task.id)}`, { method: 'DELETE' })
      setNotice('Task deleted.')
      if (formRef.current?.id === task.id) {
        setForm(null)
        setFormDirty(false)
      }
      await load()
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : 'Failed to delete task.')
    } finally {
      setBusy(false)
    }
  }, [load])

  const sessionOptions = useMemo(
    () => [...sessions].sort((a, b) => a.name.localeCompare(b.name)),
    [sessions],
  )

  const updateSessionsDockState = useCallback((patch: Partial<SessionsDockState>) => {
    onSessionsDockStateChange(previous => ({ ...previous, ...patch }))
  }, [onSessionsDockStateChange])
  const sessionsPinned = sessionsDockState.pinned || sessionsForcedPinned

  return (
    <div className="scheduled-dock" data-sidecar={sessionsDockState.open ? 'sessions' : 'closed'}>
      {sessionsDockState.open && (
        <SessionPanel
          activeWorkspaceId={activeWorkspaceId}
          collapsed={false}
          width={sessionsDockState.width}
          pinned={sessionsPinned}
          panelId="scheduled-sessions-sidecar"
          onClose={() => updateSessionsDockState({ open: false })}
          onWidthChange={width => updateSessionsDockState({ width })}
          searchTerm={sessionsDockState.searchTerm}
          collapsedGroups={sessionsDockState.collapsedGroups}
          onSearchTermChange={searchTerm => updateSessionsDockState({ searchTerm })}
          onCollapsedGroupsChange={collapsedGroups => updateSessionsDockState({ collapsedGroups })}
        />
      )}

      <div className="scheduled-main">
        <header className="scheduled-toolbar">
          <button
            type="button"
            className={`terminal-sidecar-button ${sessionsDockState.open ? 'active' : ''}`}
            aria-label="Sessions sidecar"
            aria-controls="scheduled-sessions-sidecar"
            aria-expanded={sessionsDockState.open}
            aria-pressed={sessionsDockState.open}
            title="Sessions"
            onClick={() => updateSessionsDockState({ open: !sessionsDockState.open })}
          >
            <SquareTerminal size={16} aria-hidden="true" />
            <span className="terminal-sidecar-label">Sessions</span>
            <span className="terminal-sidecar-count">{sessions.length}</span>
          </button>
          <h2>Scheduled Tasks</h2>
          <span className="scheduled-toolbar-hint">Prompts CHROTE sends into your sessions on a timer</span>
          <button type="button" className="scheduled-primary" onClick={() => startCreate()}>
            <Plus size={14} aria-hidden="true" /> New Task
          </button>
        </header>

        {error && <div className="scheduled-alert" role="alert">{error}</div>}
        {notice && <div className="scheduled-notice" role="status">{notice}</div>}

        <div className="scheduled-body">
          <section className="scheduled-list" aria-label="Scheduled task list">
            {loading ? (
              <p className="scheduled-empty">Loading scheduled tasks…</p>
            ) : tasks.length === 0 ? (
              <p className="scheduled-empty">No scheduled tasks yet. Create one to keep sessions moving while you are away.</p>
            ) : (
              tasks.map(task => {
                const targets = taskTargets(task)
                return (
                  <button
                    key={task.id}
                    type="button"
                    className={`scheduled-task-card ${task.id === selectedTaskId ? 'active' : ''} ${task.paused || !task.enabled ? 'muted' : ''}`}
                    onClick={() => setSelectedTaskId(task.id)}
                    onDoubleClick={() => startEdit(task)}
                  >
                    <span className="scheduled-task-name">{task.name}</span>
                    <span className="scheduled-task-when">
                      {describeSchedule(task.schedule)}
                      <span className="scheduled-task-zone"> · {task.schedule?.timezone || 'Local'}</span>
                    </span>
                    <span className="scheduled-task-targets">
                      {targets.map(target => (
                        <span key={targetKey(target)} className="scheduled-chip">{targetLabel(target)}</span>
                      ))}
                    </span>
                    <span className="scheduled-task-status">
                      <span className={task.paused || !task.enabled ? 'scheduled-muted' : ''}>{statusLabel(task)}</span>
                      {!task.paused && task.enabled && task.nextRun && (
                        <span className="scheduled-muted"> · next {formatCountdown(task.nextRun, now)}</span>
                      )}
                      {task.lastStatus && (
                        <span className={task.lastStatus === 'success' ? 'scheduled-muted' : 'scheduled-bad'}> · last {task.lastStatus}</span>
                      )}
                    </span>
                  </button>
                )
              })
            )}
          </section>

          <section className="scheduled-detail" aria-label="Scheduled task detail">
            {form ? (
              <form className="scheduled-form" onSubmit={saveForm}>
                <div className="scheduled-form-header">
                  <h3>{form.id ? 'Edit task' : 'New task'}</h3>
                  <div className="scheduled-form-header-actions">
                    <button type="button" className="scheduled-secondary" onClick={closeForm}>Cancel</button>
                    <button type="submit" className="scheduled-primary" disabled={busy}>
                      {form.id ? 'Save task' : 'Create task'}
                    </button>
                  </div>
                </div>

                <label className="scheduled-field">
                  <span>Prompt</span>
                  <textarea
                    className="scheduled-prompt-input"
                    value={form.prompt}
                    rows={4}
                    placeholder="Continue if work is clear…"
                    onChange={event => updateForm(previous => ({ ...previous, prompt: event.target.value }))}
                  />
                </label>

                <div
                  ref={setDropRef}
                  className={`scheduled-targets ${isOver ? 'drop-active' : ''}`}
                  data-testid="scheduled-target-dropzone"
                >
                  <div className="scheduled-targets-head">
                    <span className="scheduled-label">Send to</span>
                    <span className="scheduled-muted">Drag sessions here, or pick them below</span>
                  </div>
                  <div className="scheduled-target-chips">
                    {form.targets.length === 0 ? (
                      <span className="scheduled-muted">No sessions selected</span>
                    ) : (
                      form.targets.map(target => (
                        <span key={targetKey(target)} className="scheduled-chip scheduled-chip-removable">
                          {target.unixUser && (
                            <span className="scheduled-chip-user" title={`Unix user: ${target.unixUser}`}>
                              {getTerminalUserInitial(target.unixUser)}
                            </span>
                          )}
                          {targetLabel(target)}
                          <button
                            type="button"
                            aria-label={`Remove ${target.sessionName}`}
                            onClick={() => updateForm(previous => ({
                              ...previous,
                              targets: previous.targets.filter(existing => targetKey(existing) !== targetKey(target)),
                            }))}
                          >
                            ×
                          </button>
                        </span>
                      ))
                    )}
                  </div>
                  <div className="scheduled-session-picker" role="group" aria-label="Available sessions">
                    {sessionOptions.map(session => {
                      const target: ScheduledTarget = {
                        sessionName: session.name,
                        ...(session.unixUser ? { unixUser: session.unixUser } : {}),
                      }
                      const selected = form.targets.some(existing => targetKey(existing) === targetKey(target))
                      return (
                        <button
                          key={getSessionKey(session.name, session.unixUser)}
                          type="button"
                          className={`scheduled-session-option ${selected ? 'selected' : ''}`}
                          aria-pressed={selected}
                          onClick={() => (selected
                            ? updateForm(previous => ({
                                ...previous,
                                targets: previous.targets.filter(existing => targetKey(existing) !== targetKey(target)),
                              }))
                            : addTarget(target))}
                        >
                          {session.unixUser && (
                            <span className="scheduled-chip-user">{getTerminalUserInitial(session.unixUser)}</span>
                          )}
                          {session.name}
                        </button>
                      )
                    })}
                  </div>
                </div>

                <ScheduleEditor
                  schedule={form.schedule}
                  onChange={schedule => updateForm(previous => ({ ...previous, schedule }))}
                />

                <label className="scheduled-field">
                  <span>Name <span className="scheduled-muted">(optional)</span></span>
                  <input
                    value={form.name}
                    placeholder="Defaults to the first line of the prompt"
                    onChange={event => updateForm(previous => ({ ...previous, name: event.target.value }))}
                  />
                </label>
              </form>
            ) : selectedTask ? (
              <TaskDetail
                task={selectedTask}
                now={now}
                busy={busy}
                onEdit={() => startEdit(selectedTask)}
                onDuplicate={() => startDuplicate(selectedTask)}
                onDelete={() => void deleteTask(selectedTask)}
                onRunNow={() => void actionTask(selectedTask, 'run-now')}
                onTogglePause={() => void actionTask(selectedTask, selectedTask.paused ? 'resume' : 'pause')}
              />
            ) : (
              <div className="scheduled-empty scheduled-detail-empty">
                <CalendarClock size={20} aria-hidden="true" />
                <p>Select a task, or create one to send a prompt into your sessions on a schedule.</p>
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  )
}

/** Delete, asking where it stands: a second press within three seconds runs it. */
function ConfirmDeleteButton({ onDelete }: { onDelete: () => void }) {
  const confirm = useConfirmInPlace(onDelete)
  return (
    <button type="button" className={confirm.armed ? 'danger armed' : 'danger'} onClick={confirm.press}>
      {confirm.armed ? 'Confirm delete' : 'Delete'}
    </button>
  )
}

function TaskDetail({
  task,
  now,
  busy,
  onEdit,
  onDuplicate,
  onDelete,
  onRunNow,
  onTogglePause,
}: {
  task: ScheduledTask
  now: number
  busy: boolean
  onEdit: () => void
  onDuplicate: () => void
  onDelete: () => void
  onRunNow: () => void
  onTogglePause: () => void
}) {
  const targets = taskTargets(task)
  const runs = task.recentRuns || []

  return (
    <div className="scheduled-card">
      <div className="scheduled-card-header">
        <div>
          <h3>{task.name}</h3>
          <p className="scheduled-muted">
            {describeSchedule(task.schedule)} · {task.schedule?.timezone || 'Local'} · {statusLabel(task)}
          </p>
        </div>
        <div className="scheduled-actions">
          <button type="button" onClick={onEdit}>Edit</button>
          <button type="button" onClick={onRunNow} disabled={busy}>Run now</button>
          <button type="button" onClick={onTogglePause} disabled={busy}>{task.paused ? 'Resume' : 'Pause'}</button>
          <button type="button" onClick={onDuplicate}>Duplicate</button>
          <ConfirmDeleteButton onDelete={onDelete} />
        </div>
      </div>

      <dl className="scheduled-meta">
        <div>
          <dt>Next run</dt>
          <dd>{task.paused || !task.enabled ? 'Paused' : `${formatDateTime(task.nextRun)} (${formatCountdown(task.nextRun, now)})`}</dd>
        </div>
        <div>
          <dt>Last run</dt>
          <dd>{task.lastRun ? `${formatDateTime(task.lastRun)} · ${task.lastStatus || 'unknown'}` : 'Never run'}</dd>
        </div>
        <div>
          <dt>Sends to</dt>
          <dd className="scheduled-target-chips">
            {targets.map(target => (
              <span key={targetKey(target)} className="scheduled-chip">
                {target.unixUser && <span className="scheduled-chip-user">{getTerminalUserInitial(target.unixUser)}</span>}
                {targetLabel(target)}
              </span>
            ))}
          </dd>
        </div>
        <div>
          <dt>Task ID</dt>
          <dd className="scheduled-muted">{task.id}</dd>
        </div>
      </dl>

      <div className="scheduled-prompt-readback">
        <span className="scheduled-label">Prompt</span>
        <pre>{task.prompt}</pre>
      </div>

      <div className="scheduled-runs">
        <span className="scheduled-label">Recent runs</span>
        {runs.length === 0 ? (
          <ul><li className="scheduled-muted">No runs recorded yet.</li></ul>
        ) : (
          <ul>
            {runs.map(run => (
              <li key={run.id}>
                <span className={run.status === 'success' ? '' : 'scheduled-bad'}>{run.status}</span>
                <span className="scheduled-muted"> · {formatDateTime(run.startedAt)} · {run.trigger || 'scheduled'}</span>
                {run.targets && run.targets.length > 0 && (
                  <span className="scheduled-muted">
                    {' · '}
                    {run.targets.map(result => `${result.sessionName}: ${result.status}`).join(', ')}
                  </span>
                )}
                {run.message && <span className="scheduled-bad"> · {run.message}</span>}
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* The Sessions panel here has the same row menu as anywhere, so what it
          puts on the table is shown here too. */}
      <TableColumn />
    </div>
  )
}

export default ScheduledTasksView
import './ScheduledTasksView.css'
