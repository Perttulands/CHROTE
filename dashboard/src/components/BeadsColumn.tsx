/**
 * The Beads column: what is moving in every readable store, docked at the
 * right of any tab.
 *
 * The workspace projection is the cheap first answer. Cached counts let this
 * column skip stores with no in-progress or ready work; a cache miss falls
 * through to the work route instead of holding the first paint. Each store is
 * independent, so one unreadable store never hides another store's work.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { openBeadCard } from '../beads/beadCard'
import {
  fetchBeadProjectList,
  fetchBeadProjects,
  fetchBeadWork,
  type BeadProject,
  type BeadWork,
} from '../beads/beadsApi'
import { beadGlyph } from '../beads/beadStatus'
import { inProgressRows, readyRows, type WorkRow } from '../beads/beadsTree'
import { rememberBeadRows } from '../beads/knownBeads'
import { useSession } from '../context/SessionContext'
import { useResizableWidth } from '../hooks/useResizableWidth'
import { useSurface } from '../keys/dismiss'
import './BeadsColumn.css'

const COLUMN_WIDTH_DEFAULT = 360
const COLUMN_WIDTH_MIN = 280
const CONTENT_WIDTH_MIN = 480

interface LoadedStore {
  project: BeadProject
  work: BeadWork
}

export interface BeadsColumnGroup {
  label: string
  projectPath: string
  inProgress: WorkRow[]
  ready: WorkRow[]
}

export interface BeadsColumnFailure {
  label: string
  message: string
}

/** Group each store's claimed work before its ready work, newest first. */
export function arrangeBeadsColumnGroups(stores: readonly LoadedStore[]): BeadsColumnGroup[] {
  return stores.flatMap(({ project, work }) => {
    const rows = work.beads.map(bead => ({
      ...bead,
      projectPath: project.path,
      projectName: project.name,
    }))
    const inProgress = inProgressRows(rows)
    const ready = readyRows(rows)
    if (inProgress.length === 0 && ready.length === 0) return []
    return [{
      label: project.prefix || work.prefix || project.name,
      projectPath: project.path,
      inProgress,
      ready,
    }]
  }).sort((a, b) => a.label.localeCompare(b.label))
}

function shouldReadWork(project: BeadProject): boolean {
  if (project.error) return false
  if (project.counts) {
    return project.counts.status.inProgress > 0 || project.counts.status.open > 0
  }
  return project.openBeads !== 0
}

function clampRememberedWidth(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(COLUMN_WIDTH_MIN, Math.round(value))
    : COLUMN_WIDTH_DEFAULT
}

interface BeadsColumnProps {
  open: boolean
  onClose: () => void
}

export default function BeadsColumn({ open, onClose }: BeadsColumnProps) {
  const { settings, updateSettings } = useSession()
  const [groups, setGroups] = useState<BeadsColumnGroup[]>([])
  const [failures, setFailures] = useState<BeadsColumnFailure[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const columnRef = useRef<HTMLElement>(null)
  const manualPaths = useMemo(() => settings.beadsProjectPaths || [], [settings.beadsProjectPaths])

  useSurface({ open, kind: 'work', onClose, ref: columnRef })

  useEffect(() => {
    if (!open) return
    let current = true

    const readProjects = async (projects: BeadProject[]) => {
      const knownFailures = projects.flatMap(project => project.error
        ? [{ label: project.prefix || project.name, message: project.error }]
        : [])
      const candidates = projects.filter(shouldReadWork)
      const settled = await Promise.allSettled(candidates.map(async project => ({
        project,
        work: await fetchBeadWork(project.path),
      })))
      if (!current) return
      const loaded = settled.flatMap(result => result.status === 'fulfilled' ? [result.value] : [])
      const requestFailures = settled.flatMap((result, index) => {
        if (result.status === 'fulfilled') return []
        const project = candidates[index]
        const message = result.reason instanceof Error ? result.reason.message : 'Could not read open work'
        return [{ label: project.prefix || project.name, message }]
      })
      loaded.forEach(({ project, work }) => rememberBeadRows(project.path, work.beads))
      setGroups(arrangeBeadsColumnGroups(loaded))
      setFailures([...knownFailures, ...requestFailures].sort((a, b) => a.label.localeCompare(b.label)))
      setLoading(false)
      setError(null)
    }

    setLoading(true)
    setError(null)
    fetchBeadProjectList()
      .then(async projects => {
        await readProjects(projects)
        if (!current || manualPaths.length === 0) return
        try {
          await readProjects(await fetchBeadProjects(manualPaths))
        } catch (cause) {
          if (current) setError(cause instanceof Error ? cause.message : 'Could not list manual Beads stores')
        }
      })
      .catch((cause: unknown) => {
        if (!current) return
        setGroups([])
        setFailures([])
        setLoading(false)
        setError(cause instanceof Error ? cause.message : 'Could not list Beads stores')
      })
    return () => { current = false }
  }, [manualPaths, open])

  const widest = useCallback(() => {
    const room = columnRef.current?.parentElement?.clientWidth || Number.POSITIVE_INFINITY
    return Math.max(COLUMN_WIDTH_MIN, room - CONTENT_WIDTH_MIN)
  }, [])
  const commitWidth = useCallback((beadsColumnWidth: number) => {
    updateSettings({ beadsColumnWidth })
  }, [updateSettings])
  const resize = useResizableWidth({
    elementRef: columnRef,
    width: clampRememberedWidth(settings.beadsColumnWidth),
    minWidth: COLUMN_WIDTH_MIN,
    maxWidth: widest,
    edge: 'left',
    onCommit: commitWidth,
  })

  if (!open) return null

  return (
    <aside
      ref={columnRef}
      className="beads-global-column"
      role="complementary"
      aria-label="Beads column"
      data-ui="beads.column"
      style={{ width: resize.width } as CSSProperties}
    >
      <div
        {...resize.handleProps}
        className={`beads-global-column-handle${resize.resizing ? ' dragging' : ''}`}
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize Beads column"
        aria-valuenow={Math.round(resize.width)}
        aria-valuemin={COLUMN_WIDTH_MIN}
        tabIndex={0}
      />
      <header className="beads-global-column-header">
        <h2>Beads</h2>
        <span className="beads-global-column-chord">Alt+B</span>
        <button type="button" aria-keyshortcuts="Alt+B" onClick={onClose}>Close</button>
      </header>
      <div className="beads-global-column-body">
        {loading && <p className="beads-global-column-note">Reading Beads…</p>}
        {!loading && error && <p className="beads-global-column-error">{error}</p>}
        {!loading && !error && groups.length === 0 && failures.length === 0 && (
          <p className="beads-global-column-note">Nothing is moving.</p>
        )}
        {!loading && groups.map(group => (
          <section className="beads-global-store" key={group.projectPath}>
            <h3>{group.label}</h3>
            {group.inProgress.length > 0 && (
              <BeadsColumnRows label="In progress" rows={group.inProgress} />
            )}
            {group.ready.length > 0 && <BeadsColumnRows label="Ready" rows={group.ready} />}
          </section>
        ))}
        {!loading && failures.length > 0 && (
          <section className="beads-global-failures">
            <h3>Unreadable</h3>
            {failures.map(failure => (
              <p key={`${failure.label}\u0000${failure.message}`}>
                <span>{failure.label}</span>
                {failure.message}
              </p>
            ))}
          </section>
        )}
      </div>
    </aside>
  )
}

function BeadsColumnRows({ label, rows }: { label: string; rows: readonly WorkRow[] }) {
  return (
    <div className="beads-global-stage">
      <h4>{label}</h4>
      {rows.map(row => (
        <button
          type="button"
          className="beads-global-row"
          key={`${row.projectPath}\u0000${row.id}`}
          title={row.updated ? `${row.id} · updated ${row.updated}` : row.id}
          onClick={() => openBeadCard(row.id, row.projectPath, row.title)}
        >
          <span className="beads-global-row-head">
            <span title={row.status}>{beadGlyph(row.status, row.blocked)}</span>
            <span>{row.id}</span>
          </span>
          <span className="beads-global-row-title">{row.title}</span>
        </button>
      ))}
    </div>
  )
}
