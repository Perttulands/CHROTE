/**
 * The complaint box: one key from annoyance to a filed Bead.
 *
 * Alt+R opens a small box near the top of the viewport with one field in it.
 * The first line is the title and the rest is the description, so a complaint
 * is written the way a commit message is, and nothing has to be tabbed between
 * before the words are down. Enter files; Shift+Enter breaks the line; Escape
 * closes.
 *
 * Three choices sit under the field and each opens on its answer for the
 * common case, every time: CHROTE's own store, a bug, and the context line
 * ticked. The store picker filters as it is typed into and lists every store
 * the Beads tab knows, by prefix. The context line is shown exactly as it will
 * be appended — the tab in front and the session in the focused tile — so the
 * operator knows what the Bead will say about where he was.
 *
 * A filing that lands closes the box and says `Filed <id>` on the status line,
 * which is where every receipt goes. A filing that fails keeps the box open
 * with the text and the server's own words. A draft closed unfiled is kept for
 * the next Alt+R, because the box is the place a thought is put down in a hurry
 * and a stray Escape should not cost it.
 *
 * File & send (Ctrl+Enter, or its button) files and then hands the new Bead to
 * the Send drawer with a note the operator can edit, so the target defaults the
 * way Send always does: the focused tile's session. The box gives the focus
 * back to where it was taken from — the focused tile's terminal when that is
 * gone — before the drawer opens, so the drawer takes that as its opener and
 * closing it lands the operator back in his terminal rather than on a box that
 * no longer exists.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { useFocusedSession } from '../context/useFocusedSession'
import { useSurface } from '../keys/dismiss'
import { beadProjects, ensureBeadProjects } from '../beads/beadIds'
import { beadReference } from '../beads/beadReference'
import { createBead, type BeadProject, type FiledBeadType } from '../beads/beadsApi'
import { getSessionNameFromKey } from '../types'
import { useTerminalPool } from './TerminalPool'
import './ReportBox.css'

/** The store a complaint goes to unless the operator says otherwise. */
export const DEFAULT_REPORT_PREFIX = 'chrote'

/** What the agent is told with the Bead it is handed. */
export const HANDOFF_NOTE = 'I just filed this Bead. Use grilling if anything about it is unclear; if it is clear, implement it.'

/** The first line is the title; the rest, trimmed, is the description. */
export function splitReport(text: string): { title: string; description: string } {
  const [first = '', ...rest] = text.split('\n')
  return { title: first.trim(), description: rest.join('\n').trim() }
}

/** The line that says where the operator was, as the Bead will carry it. */
export function contextLine(activeTab: string, sessionKey: string | null): string {
  return sessionKey ? `Context: ${activeTab} · ${getSessionNameFromKey(sessionKey)}` : `Context: ${activeTab}`
}

/** The description as filed: the operator's words, then the context line. */
export function reportDescription(description: string, context: string | null): string {
  if (!context) return description
  return description ? `${description}\n\n${context}` : context
}

function storeLabel(project: BeadProject): string {
  return project.prefix || project.name
}

function defaultStore(projects: readonly BeadProject[]): string | null {
  return (projects.find(project => project.prefix === DEFAULT_REPORT_PREFIX) ?? projects[0])?.path ?? null
}

interface ReportBoxProps {
  open: boolean
  onClose: () => void
  /** The tab in front, for the context line. */
  activeTab: string
}

export default function ReportBox({ open, onClose, activeTab }: ReportBoxProps) {
  const { openSendToSession } = useSession()
  const { announce } = useStatus()
  const pool = useTerminalPool()
  const focusedSession = useFocusedSession()

  // The draft outlives a closing: the component stays mounted and only its
  // box goes away.
  const [text, setText] = useState('')
  const [projects, setProjects] = useState<readonly BeadProject[]>(() => beadProjects())
  const [store, setStore] = useState<string | null>(null)
  const [storeQuery, setStoreQuery] = useState<string | null>(null)
  const [storeCursor, setStoreCursor] = useState(0)
  const [type, setType] = useState<FiledBeadType>('bug')
  const [withContext, setWithContext] = useState(true)
  const [filing, setFiling] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)

  const boxRef = useRef<HTMLDivElement>(null)
  const fieldRef = useRef<HTMLTextAreaElement>(null)
  const typeRefs = useRef<Record<FiledBeadType, HTMLButtonElement | null>>({ bug: null, feature: null })
  const landingRef = useRef<() => void>(() => {})

  // Where the focus goes when the box closes: back where it was taken from,
  // or into the focused tile's terminal when that control is gone.
  landingRef.current = () => {
    if (focusedSession) pool.terminals.get(focusedSession)?.focus()
  }

  useSurface({ open, kind: 'glance', onClose, ref: boxRef })

  // Every opening starts on the common answers; only the words carry over.
  useEffect(() => {
    if (!open) return
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const known = beadProjects()
    setProjects(known)
    setStore(defaultStore(known))
    setStoreQuery(null)
    setType('bug')
    setWithContext(true)
    setFailure(null)
    setFiling(false)
    let current = true
    void ensureBeadProjects().then(found => {
      if (!current || found.length === 0) return
      setProjects(found)
      setStore(previous => (previous && found.some(project => project.path === previous) ? previous : defaultStore(found)))
    })
    const field = fieldRef.current
    if (field) {
      field.focus()
      field.setSelectionRange(field.value.length, field.value.length)
    }
    return () => {
      current = false
      if (opener?.isConnected && opener !== document.body) opener.focus()
      else landingRef.current()
    }
  }, [open])

  const context = contextLine(activeTab, focusedSession)
  const { title, description } = splitReport(text)
  const storeProject = projects.find(project => project.path === store) ?? null

  const storeMatches = useMemo(() => {
    if (storeQuery === null) return []
    const needle = storeQuery.trim().toLowerCase()
    return projects.filter(project =>
      !needle || storeLabel(project).toLowerCase().includes(needle) || project.path.toLowerCase().includes(needle))
  }, [projects, storeQuery])

  const file = useCallback(async (send: boolean) => {
    if (filing || !title || !storeProject) return
    setFiling(true)
    setFailure(null)
    try {
      const created = await createBead({
        path: storeProject.path,
        title,
        description: reportDescription(description, withContext ? context : null),
        type,
      })
      setText('')
      onClose()
      announce(`Filed ${created.id}`, 'success')
      if (send) openSendToSession({ reference: beadReference(created), note: HANDOFF_NOTE })
    } catch (cause) {
      setFailure(cause instanceof Error ? cause.message : 'Filing failed')
    } finally {
      setFiling(false)
    }
  }, [announce, context, description, filing, onClose, openSendToSession, storeProject, title, type, withContext])

  const pickStore = (project: BeadProject | undefined) => {
    if (project) setStore(project.path)
    setStoreQuery(null)
  }

  // Enter files from anywhere in the box but a button, which Enter presses,
  // and the store picker while its list is open, where Enter picks.
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key !== 'Enter' || event.shiftKey || event.altKey || event.metaKey) return
    if (event.target instanceof HTMLButtonElement && event.target.getAttribute('role') !== 'radio') return
    event.preventDefault()
    void file(event.ctrlKey)
  }

  const handleStoreKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (storeQuery === null) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      const step = event.key === 'ArrowDown' ? 1 : -1
      setStoreCursor(index => Math.max(0, Math.min(storeMatches.length - 1, index + step)))
    } else if (event.key === 'Enter' && !event.ctrlKey) {
      event.preventDefault()
      event.stopPropagation()
      pickStore(storeMatches[storeCursor])
    }
  }

  // The switch is one stop on Tab; the arrows flip it, whichever they are.
  const handleTypeKeyDown = (event: React.KeyboardEvent) => {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return
    event.preventDefault()
    const next: FiledBeadType = type === 'bug' ? 'feature' : 'bug'
    setType(next)
    typeRefs.current[next]?.focus()
  }

  if (!open) return null

  return (
    <div
      ref={boxRef}
      className="report-box"
      role="dialog"
      aria-label="Report an issue"
      data-ui="report.box"
      onKeyDown={handleKeyDown}
    >
      <textarea
        ref={fieldRef}
        className="report-box-field"
        aria-label="Issue"
        placeholder="What is wrong? The first line is the title."
        rows={4}
        value={text}
        onChange={event => setText(event.target.value)}
      />
      <div className="report-box-choices">
        <div className="report-box-store">
          <input
            className="report-box-store-input"
            role="combobox"
            aria-label="Store"
            aria-expanded={storeQuery !== null}
            aria-controls="report-box-stores"
            value={storeQuery ?? (storeProject ? storeLabel(storeProject) : '')}
            placeholder="no store"
            // Focused, the field is a filter over every store with the current
            // one under the cursor, so Enter alone keeps it.
            onFocus={() => {
              setStoreQuery('')
              setStoreCursor(Math.max(0, projects.findIndex(project => project.path === store)))
            }}
            onChange={event => {
              setStoreQuery(event.target.value)
              setStoreCursor(0)
            }}
            onBlur={() => setStoreQuery(null)}
            onKeyDown={handleStoreKeyDown}
          />
          {storeQuery !== null && (
            <div className="report-box-stores" id="report-box-stores" role="listbox" aria-label="Stores">
              {storeMatches.map((project, index) => (
                <div
                  key={project.path}
                  role="option"
                  aria-selected={index === storeCursor}
                  className={`report-box-store-option${index === storeCursor ? ' current' : ''}`}
                  // Picked on press, before the input's blur closes the list.
                  onMouseDown={event => {
                    event.preventDefault()
                    pickStore(project)
                  }}
                >
                  <span className="report-box-store-prefix">{storeLabel(project)}</span>
                  <span className="report-box-store-path">{project.path}</span>
                </div>
              ))}
              {storeMatches.length === 0 && <div className="report-box-store-empty">No store matches</div>}
            </div>
          )}
        </div>
        <div className="report-box-type" role="radiogroup" aria-label="Type" onKeyDown={handleTypeKeyDown}>
          {(['bug', 'feature'] as const).map(option => (
            <button
              key={option}
              ref={element => { typeRefs.current[option] = element }}
              type="button"
              role="radio"
              aria-checked={type === option}
              tabIndex={type === option ? 0 : -1}
              className={`report-box-type-option${type === option ? ' checked' : ''}`}
              onClick={() => setType(option)}
            >
              {option === 'bug' ? 'Bug' : 'Feature'}
            </button>
          ))}
        </div>
        <label className="report-box-context">
          <input
            type="checkbox"
            checked={withContext}
            onChange={event => setWithContext(event.target.checked)}
          />
          <span>{context}</span>
        </label>
      </div>
      {failure !== null && <p className="report-box-failure" role="alert">{failure}</p>}
      <div className="report-box-actions">
        <span className="report-box-hint">Enter files · Shift+Enter new line · Ctrl+Enter files and sends</span>
        <button
          type="button"
          className="report-box-send"
          disabled={filing || !title || !storeProject}
          onClick={() => { void file(true) }}
        >
          {filing ? 'Filing…' : 'File & send'}
        </button>
      </div>
    </div>
  )
}
