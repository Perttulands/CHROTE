/**
 * The Beads tab: the open work of every configured store, read three ways.
 *
 * A rail of projects at the left, "All" first; the map, the ready lists and the
 * stale list as a segmented control; one search across all three. Nothing here
 * writes: creating, editing and closing Beads stays with `bd` and the agents,
 * and the hand-off out of this tab is the Send drawer.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import MapView from './MapView'
import ReadyView from './ReadyView'
import StaleView from './StaleView'
import ResidentColumn from '../ResidentColumn'
import TableColumn from '../TableColumn'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { tableReference, useTableObject } from '../../context/TableContext'
import { setBeadProjects } from '../../beads/beadIds'
import { fetchBeadProjectList, fetchBeadProjects, fetchBeadWork, type BeadProject } from '../../beads/beadsApi'
import { rememberBeadRows } from '../../beads/knownBeads'
import {
  buildBeadMap,
  filterBeadRows,
  filterBeadTree,
  inProgressRows,
  readyRows,
  staleRows,
  type WorkRow,
} from '../../beads/beadsTree'
import { isBeadClosed } from '../../beads/beadStatus'
import type { BeadsViewSetting } from '../../types'
import './BeadsView.css'

type BeadsTabView = BeadsViewSetting

const VIEWS: { id: BeadsTabView; label: string }[] = [
  { id: 'map', label: 'Map' },
  { id: 'ready', label: 'Open' },
  { id: 'stale', label: 'Stale' },
]

/** What counts as stale until the operator says otherwise. */
export const DEFAULT_STALE_DAYS = 14

const ALL_PROJECTS = 'all'

export interface BeadsRevealRequest {
  projectPath: string
  id: string
  nonce: number
}

interface BeadsViewProps {
  /** A Bead the card asked to be shown here, in its own project. */
  reveal?: BeadsRevealRequest | null
}

/** What one project's load says on the status line. */
function projectTally(name: string, rows: { status: string }[]): string {
  const open = rows.filter(row => !isBeadClosed(row.status) && row.status !== 'in_progress').length
  const active = rows.filter(row => row.status === 'in_progress').length
  return active > 0 ? `${name} ${open} open, ${active} in progress` : `${name} ${open} open`
}

export default function BeadsView({ reveal }: BeadsViewProps = {}) {
  const { settings, updateSettings } = useSession()
  const { announce } = useStatus()
  const [projects, setProjects] = useState<BeadProject[]>([])
  const [selected, setSelected] = useState<string>(settings.beadsSelectedProject || ALL_PROJECTS)
  const [view, setView] = useState<BeadsTabView>(
    VIEWS.some(item => item.id === settings.beadsView) ? settings.beadsView : 'map',
  )
  const [query, setQuery] = useState('')
  const [staleDays, setStaleDays] = useState(DEFAULT_STALE_DAYS)
  const [rows, setRows] = useState<WorkRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [quietShown, setQuietShown] = useState(false)
  // What the Clerk is handed on Alt+S: whatever is on the table.
  const table = useTableObject()

  const manualPaths = useMemo(() => settings.beadsProjectPaths || [], [settings.beadsProjectPaths])

  useEffect(() => {
    let current = true
    fetchBeadProjectList()
      .then(found => {
        if (!current) return
        setProjects(found)
        // The terminal's link provider matches the prefixes of the projects
        // that actually exist; this is where it learns them.
        setBeadProjects(found)
        if (found.length === 0) setLoading(false)

        void fetchBeadProjects(manualPaths)
          .then(detailed => {
            if (!current) return
            setProjects(detailed)
            setBeadProjects(detailed)
            setSelected(previous => {
              if (previous === ALL_PROJECTS || detailed.some(project => project.path === previous)) return previous
              updateSettings({ beadsSelectedProject: ALL_PROJECTS })
              return ALL_PROJECTS
            })
          })
          .catch((cause: unknown) => {
            if (!current) return
            const message = cause instanceof Error ? cause.message : 'Could not read Beads counts'
            announce(`Beads counts unavailable · ${message}`, 'error')
          })
      })
      .catch((cause: unknown) => {
        if (!current) return
        setProjects([])
        setError(cause instanceof Error ? cause.message : 'Could not list Beads projects')
        setLoading(false)
      })
    return () => { current = false }
  }, [announce, manualPaths, updateSettings])

  // A quiet store has nothing open: it is folded in the rail, and "All" does
  // not ask it, because the answer is known to be empty.
  const unreadableProjects = useMemo(() => projects.filter(project => !!project.error), [projects])
  const readableProjects = useMemo(() => projects.filter(project => !project.error), [projects])
  const openProjects = useMemo(() => readableProjects.filter(project => project.openBeads !== 0), [readableProjects])
  const quietProjects = useMemo(() => readableProjects.filter(project => project.openBeads === 0), [readableProjects])
  const summariesPending = selected === ALL_PROJECTS && projects.some(project => project.summaryPending)
  const chosen = useMemo(() => {
    if (summariesPending) return []
    return selected === ALL_PROJECTS ? openProjects : readableProjects.filter(project => project.path === selected)
  }, [openProjects, readableProjects, selected, summariesPending])
  const chosenRef = useRef(chosen)
  const projectsRef = useRef(projects)
  chosenRef.current = chosen
  projectsRef.current = projects
  const chosenKey = chosen.map(project => project.path).join('\u0000')
  const quietOpen = quietShown || quietProjects.some(project => project.path === selected)

  useEffect(() => {
    if (projectsRef.current.length === 0) return
    if (summariesPending) {
      setLoading(true)
      setError(null)
      return
    }
    const loadProjects = chosenRef.current
    if (loadProjects.length === 0) {
      setRows([])
      setLoading(false)
      const selectedProject = projectsRef.current.find(project => project.path === selected)
      setError(selectedProject?.error ?? null)
      if (selectedProject?.error) {
        announce(`Beads unavailable · ${selectedProject.prefix || selectedProject.name}: ${selectedProject.error}`, 'error')
      }
      return
    }
    let current = true
    setLoading(true)
    setError(null)
    Promise.allSettled(loadProjects.map(async project => ({
      project,
      work: await fetchBeadWork(project.path),
    })))
      .then(settled => {
        if (!current) return
        const loaded = settled.flatMap(result => result.status === 'fulfilled' ? [result.value] : [])
        const failures = settled.flatMap((result, index) => {
          if (result.status === 'fulfilled') return []
          const project = loadProjects[index]
          const message = result.reason instanceof Error ? result.reason.message : 'Could not read open work'
          return [{ project, message }]
        })
        // The card opens from these rows before the server has answered.
        loaded.forEach(({ project, work }) => rememberBeadRows(project.path, work.beads))
        const all = loaded.flatMap(({ project, work }) => work.beads.map(bead => ({
          ...bead,
          projectPath: project.path,
          projectName: project.name,
        })))
        setRows(all)
        setLoading(false)
        if (loaded.length > 0) {
          const tally = loaded.map(({ project, work }) => projectTally(project.prefix || project.name, work.beads))
          announce(`Beads loaded · ${tally.join(' · ')}`, 'info')
        }
        if (failures.length > 0) {
          const failure = failures.map(({ project, message }) => `${project.prefix || project.name}: ${message}`).join(' · ')
          announce(`Beads unavailable · ${failure}`, 'error')
          if (loaded.length === 0) setError(failure)
        }
        const knownFailures = projectsRef.current.filter(project => !!project.error)
        if (knownFailures.length > 0) {
          const failure = knownFailures.map(project => `${project.prefix || project.name}: ${project.error}`).join(' · ')
          announce(`Beads unavailable · ${failure}`, 'error')
        }
      })
    return () => { current = false }
  }, [announce, chosenKey, selected, summariesPending])

  useEffect(() => {
    if (!reveal) return
    setSelected(reveal.projectPath)
    setQuery(reveal.id)
    setView('map')
    updateSettings({ beadsSelectedProject: reveal.projectPath, beadsView: 'map' })
  }, [reveal, updateSettings])

  const map = useMemo(() => filterBeadTree(buildBeadMap(rows), query), [rows, query])
  const matching = useMemo(() => filterBeadRows(rows, query), [rows, query])
  const selectProject = useCallback((path: string) => {
    setSelected(path)
    updateSettings({ beadsSelectedProject: path })
  }, [updateSettings])
  const selectView = useCallback((next: BeadsTabView) => {
    setView(next)
    updateSettings({ beadsView: next })
  }, [updateSettings])

  return (
    <div className="beads-view">
      <nav className="beads-rail" aria-label="Beads projects">
        <button
          type="button"
          className={`beads-rail-item ${selected === ALL_PROJECTS ? 'active' : ''}`}
          onClick={() => selectProject(ALL_PROJECTS)}
        >
          All
        </button>
        {openProjects.map(project => (
          <button
            key={project.path}
            type="button"
            className={`beads-rail-item ${selected === project.path ? 'active' : ''}`}
            onClick={() => selectProject(project.path)}
            title={project.path}
          >
            {project.prefix || project.name}
          </button>
        ))}
        {unreadableProjects.map(project => (
          <button
            key={project.path}
            type="button"
            className={`beads-rail-item beads-rail-unreadable ${selected === project.path ? 'active' : ''}`}
            onClick={() => selectProject(project.path)}
            title={`${project.path}: ${project.error}`}
          >
            {project.prefix || project.name} · unreadable
          </button>
        ))}
        {quietProjects.length > 0 && (
          <button
            type="button"
            className="beads-rail-item beads-rail-more"
            aria-expanded={quietOpen}
            onClick={() => setQuietShown(open => !open)}
          >
            {quietOpen ? 'Fewer' : `More (${quietProjects.length} quiet)`}
          </button>
        )}
        {quietOpen && quietProjects.map(project => (
          <button
            key={project.path}
            type="button"
            className={`beads-rail-item beads-rail-quiet ${selected === project.path ? 'active' : ''}`}
            onClick={() => selectProject(project.path)}
            title={project.path}
          >
            {project.prefix || project.name}
          </button>
        ))}
      </nav>

      <div className="beads-main">
        <div className="beads-controls">
          <div className="beads-views" role="tablist" aria-label="Beads views">
            {VIEWS.map(item => (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={view === item.id}
                className={`beads-view-tab ${view === item.id ? 'active' : ''}`}
                onClick={() => selectView(item.id)}
              >
                {item.label}
              </button>
            ))}
          </div>
          <input
            className="beads-search"
            type="search"
            value={query}
            placeholder="Search Beads…"
            aria-label="Search Beads"
            onChange={event => setQuery(event.target.value)}
          />
          {view === 'stale' && (
            <label className="beads-stale-days">
              No update in
              <input
                type="number"
                min={1}
                max={365}
                value={staleDays}
                aria-label="Days without an update"
                onChange={event => setStaleDays(Math.max(1, Number(event.target.value) || DEFAULT_STALE_DAYS))}
              />
              days
            </label>
          )}
        </div>

        <div className="beads-content">
          {error && <p className="beads-error">{error}</p>}
          {!error && loading && <p className="beads-empty">Reading Beads…</p>}
          {!error && !loading && view === 'map' && <MapView roots={map} expandAll={query.trim() !== ''} />}
          {!error && !loading && view === 'ready' && (
            <ReadyView ready={readyRows(matching)} inProgress={inProgressRows(matching)} />
          )}
          {!error && !loading && view === 'stale' && <StaleView rows={staleRows(matching, staleDays)} />}
        </div>
      </div>

      <TableColumn />
      <ResidentColumn tab="beads" reference={table ? tableReference(table) : null} />
    </div>
  )
}
