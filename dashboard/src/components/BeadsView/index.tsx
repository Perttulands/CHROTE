/**
 * The Beads tab: the open work of every configured store, read three ways.
 *
 * A rail of projects at the left, "All" first; the map, the ready lists and the
 * stale list as a segmented control; one search across all three. Nothing here
 * writes: creating, editing and closing Beads stays with `bd` and the agents,
 * and the hand-off out of this tab is the Send drawer.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import MapView from './MapView'
import ReadyView from './ReadyView'
import StaleView from './StaleView'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { setBeadProjects } from '../../beads/beadIds'
import { fetchBeadProjects, fetchBeadWork, type BeadProject } from '../../beads/beadsApi'
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
import './BeadsView.css'

type BeadsTabView = 'map' | 'ready' | 'stale'

const VIEWS: { id: BeadsTabView; label: string }[] = [
  { id: 'map', label: 'Map' },
  { id: 'ready', label: 'Ready and in progress' },
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
  const { settings } = useSession()
  const { announce } = useStatus()
  const [projects, setProjects] = useState<BeadProject[]>([])
  const [selected, setSelected] = useState<string>(ALL_PROJECTS)
  const [view, setView] = useState<BeadsTabView>('map')
  const [query, setQuery] = useState('')
  const [staleDays, setStaleDays] = useState(DEFAULT_STALE_DAYS)
  const [rows, setRows] = useState<WorkRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const manualPaths = useMemo(() => settings.beadsProjectPaths || [], [settings.beadsProjectPaths])

  useEffect(() => {
    let current = true
    fetchBeadProjects(manualPaths)
      .then(found => {
        if (!current) return
        setProjects(found)
        // The terminal's link provider matches the prefixes of the projects
        // that actually exist; this is where it learns them.
        setBeadProjects(found)
        if (found.length === 0) setLoading(false)
      })
      .catch((cause: unknown) => {
        if (!current) return
        setProjects([])
        setError(cause instanceof Error ? cause.message : 'Could not list Beads projects')
        setLoading(false)
      })
    return () => { current = false }
  }, [manualPaths])

  const chosen = useMemo(
    () => (selected === ALL_PROJECTS ? projects : projects.filter(project => project.path === selected)),
    [projects, selected],
  )

  useEffect(() => {
    if (projects.length === 0) return
    let current = true
    setLoading(true)
    setError(null)
    Promise.all(chosen.map(async project => ({
      project,
      work: await fetchBeadWork(project.path),
    })))
      .then(loaded => {
        if (!current) return
        // The card opens from these rows before the server has answered.
        loaded.forEach(({ project, work }) => rememberBeadRows(project.path, work.beads))
        const all = loaded.flatMap(({ project, work }) => work.beads.map(bead => ({
          ...bead,
          projectPath: project.path,
          projectName: project.name,
        })))
        setRows(all)
        setLoading(false)
        const tally = loaded.map(({ project, work }) => projectTally(project.prefix || project.name, work.beads))
        announce(`Beads loaded · ${tally.join(' · ')}`, 'info')
      })
      .catch((cause: unknown) => {
        if (!current) return
        setRows([])
        setLoading(false)
        setError(cause instanceof Error ? cause.message : 'Could not read open work')
      })
    return () => { current = false }
  }, [chosen, projects.length, announce])

  useEffect(() => {
    if (!reveal) return
    setSelected(reveal.projectPath)
    setQuery(reveal.id)
    setView('map')
  }, [reveal])

  const map = useMemo(() => filterBeadTree(buildBeadMap(rows), query), [rows, query])
  const matching = useMemo(() => filterBeadRows(rows, query), [rows, query])
  const selectProject = useCallback((path: string) => {
    setSelected(path)
  }, [])

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
        {projects.map(project => (
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
                onClick={() => setView(item.id)}
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
    </div>
  )
}
