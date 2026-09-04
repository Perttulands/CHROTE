/**
 * The Beads tab: the open work of every configured store, read three ways.
 *
 * A rail of projects at the left, "All" first; the map, the ready lists, the
 * flow of an epic and the stale list as a segmented control; one search across
 * the lists. Nothing here
 * writes: creating, editing and closing Beads stays with `bd` and the agents,
 * and the hand-off out of this tab is the Send drawer.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ClosedView, { type ClosedFailure } from './ClosedView'
import FlowView from './FlowView'
import { FlowNavigationProvider } from './FlowNavigation'
import MapView from './MapView'
import ReadyView from './ReadyView'
import StoreState from './StoreState'
import StaleView from './StaleView'
import TemplateExplorer from './TemplateExplorer'
import ResidentColumn from '../ResidentColumn'
import TableColumn from '../TableColumn'
import Rail, { RailScroll, RailSection } from '../Rail'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { tableReference, useTableObject } from '../../context/TableContext'
import { setBeadProjects } from '../../beads/beadIds'
import {
  fetchBeadProjectList,
  fetchBeadProjects,
  fetchBeadWork,
  fetchBead,
  fetchClosedBeadWork,
  fetchFormula,
  fetchFormulas,
  fetchMolecule,
  fetchMolecules,
  type BeadProject,
  type BeadDetail,
  type BeadLink,
  type BeadsStructure,
  type FormulaSummary,
  type MoleculeSummary,
} from '../../beads/beadsApi'
import { rememberBeadRows } from '../../beads/knownBeads'
import { flowComponent, flowComponentKey } from '../../beads/flowLayout'
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
import type { FlowRevealRequest } from './FlowView'

type BeadsTabView = BeadsViewSetting

const VIEWS: { id: BeadsTabView; label: string }[] = [
  { id: 'map', label: 'Map' },
  { id: 'ready', label: 'Open' },
  { id: 'flow', label: 'Flow' },
  { id: 'stale', label: 'Stale' },
  { id: 'closed', label: 'Closed' },
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
  active?: boolean
  /** A Bead the card asked to be shown here, in its own project. */
  reveal?: BeadsRevealRequest | null
}

interface ClosedSnapshot {
  loading: boolean
  rows: WorkRow[]
  failures: ClosedFailure[]
}

interface TemplateCatalog {
  loading: boolean
  formulas: FormulaSummary[]
  molecules: MoleculeSummary[]
  formulaError: string | null
  moleculeError: string | null
}

interface TemplateSelection {
  kind: 'formula' | 'molecule'
  key: string
  label: string
  projectPath: string
}

interface TemplateDetail {
  loading: boolean
  detail: BeadsStructure | null
  error: string | null
}

function errorMessage(cause: unknown, fallback: string): string {
  return cause instanceof Error ? cause.message : fallback
}

function formulaName(formula: FormulaSummary): string {
  return typeof formula.name === 'string' ? formula.name : typeof formula.formula === 'string' ? formula.formula : ''
}

function moleculeID(molecule: MoleculeSummary): string {
  return typeof molecule.id === 'string' ? molecule.id : ''
}

function moleculeTitle(molecule: MoleculeSummary): string {
  return typeof molecule.title === 'string' && molecule.title.trim() !== '' ? molecule.title : moleculeID(molecule)
}

function isTemplateProto(molecule: MoleculeSummary): boolean {
  return molecule.is_template === true
}

function mergeFlowRows(...groups: readonly WorkRow[][]): WorkRow[] {
  const merged = new Map<string, WorkRow>()
  groups.flat().forEach(row => {
    const key = `${row.projectPath}\u0000${row.id}`
    merged.set(key, { ...merged.get(key), ...row })
  })
  return [...merged.values()]
}

function linkedRow(link: BeadLink, target: WorkRow, existing?: WorkRow): WorkRow {
  return {
    ...existing,
    id: link.id,
    title: link.title,
    status: link.status,
    type: link.type,
    priority: link.priority,
    blocked: existing?.blocked ?? false,
    linked: true,
    projectPath: target.projectPath,
    projectName: target.projectName,
  }
}

/** Turn the issue route's one-hop relationships back into the flat edge fields
 * Flow consumes. This fills neighbours omitted by the unfinished/closed split. */
function flowRowsFromDetail(snapshot: readonly WorkRow[], target: WorkRow, detail: BeadDetail): WorkRow[] {
  const inStore = new Map(snapshot
    .filter(row => row.projectPath === target.projectPath)
    .map(row => [row.id, row]))
  const rows = new Map<string, WorkRow>()
  const put = (row: WorkRow) => rows.set(row.id, { ...rows.get(row.id), ...row })
  const blockers = detail.blockedBy.map(link => link.id)
  put({
    ...target,
    id: target.id,
    title: detail.title || target.title,
    status: detail.status || target.status,
    type: detail.type || target.type,
    priority: detail.priority,
    parent: target.parent || detail.parents[0]?.id,
    blockedBy: blockers,
    blocked: blockers.length > 0,
    linked: true,
  })
  detail.parents.forEach(link => put(linkedRow(link, target, inStore.get(link.id))))
  detail.children.forEach(link => put({
    ...linkedRow(link, target, inStore.get(link.id)),
    parent: target.id,
  }))
  detail.blockedBy.forEach(link => put(linkedRow(link, target, inStore.get(link.id))))
  detail.blocks.forEach(link => {
    const existing = inStore.get(link.id)
    put({
      ...linkedRow(link, target, existing),
      blockedBy: [...new Set([...(existing?.blockedBy ?? []), target.id])],
      blocked: true,
    })
  })
  return [...rows.values()]
}

/** What one project's load says on the status line. */
function projectTally(name: string, rows: { status: string }[]): string {
  const open = rows.filter(row => !isBeadClosed(row.status) && row.status !== 'in_progress').length
  const active = rows.filter(row => row.status === 'in_progress').length
  return active > 0 ? `${name} ${open} open, ${active} in progress` : `${name} ${open} open`
}

export default function BeadsView({ active = true, reveal }: BeadsViewProps = {}) {
  const { settings, updateSettings } = useSession()
  const { announce } = useStatus()
  const [projects, setProjects] = useState<BeadProject[]>([])
  const [projectsReady, setProjectsReady] = useState(false)
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
  const [flowReveal, setFlowReveal] = useState<FlowRevealRequest | null>(null)
  const [flowSupplements, setFlowSupplements] = useState<WorkRow[]>([])
  const [closedCache, setClosedCache] = useState<Record<string, ClosedSnapshot>>({})
  const closedRequested = useRef(new Set<string>())
  const [templateCatalogs, setTemplateCatalogs] = useState<Record<string, TemplateCatalog>>({})
  const templateCatalogRequested = useRef(new Set<string>())
  const [templateSelection, setTemplateSelection] = useState<TemplateSelection | null>(null)
  const [templateDetail, setTemplateDetail] = useState<TemplateDetail | null>(null)
  const templateRequestNonce = useRef(0)
  const flowRevealNonce = useRef(0)
  const flowRequestNonce = useRef(0)
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
            setProjectsReady(true)
            setBeadProjects(detailed)
            setSelected(previous => {
              if (previous === ALL_PROJECTS || detailed.some(project => project.path === previous)) return previous
              updateSettings({ beadsSelectedProject: ALL_PROJECTS })
              return ALL_PROJECTS
            })
          })
          .catch((cause: unknown) => {
            if (!current) return
            setProjectsReady(true)
            const message = cause instanceof Error ? cause.message : 'Could not read Beads counts'
            announce(`Beads counts unavailable · ${message}`, 'error')
          })
      })
      .catch((cause: unknown) => {
        if (!current) return
        setProjects([])
        setProjectsReady(true)
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
  const closedMode = view === 'closed'
  const selectedStore = useMemo(
    () => (selected === ALL_PROJECTS ? null : projects.find(project => project.path === selected) ?? null),
    [projects, selected],
  )
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
  const selectedQuiet = quietProjects.find(project => project.path === selected)

  useEffect(() => {
    if (closedMode) return
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
  }, [announce, chosenKey, closedMode, selected, summariesPending])

  // Closed is a separate read. The cache key is the operator's selected scope,
  // so returning to it in this mounted tab does not run bd again.
  useEffect(() => {
    if (view !== 'closed' || !projectsReady || closedRequested.current.has(selected)) return
    closedRequested.current.add(selected)
    const loadProjects = selected === ALL_PROJECTS
      ? readableProjects
      : readableProjects.filter(project => project.path === selected)
    const knownFailures = (selected === ALL_PROJECTS ? unreadableProjects : unreadableProjects.filter(project => project.path === selected))
      .map(project => ({ projectName: project.prefix || project.name, message: project.error || 'Unreadable store' }))
    setClosedCache(previous => ({
      ...previous,
      [selected]: { loading: true, rows: [], failures: knownFailures },
    }))
    void Promise.allSettled(loadProjects.map(async project => ({
      project,
      work: await fetchClosedBeadWork(project.path),
    }))).then(settled => {
      const loaded = settled.flatMap(result => result.status === 'fulfilled' ? [result.value] : [])
      const failures = settled.flatMap((result, index) => result.status === 'rejected'
        ? [{
            projectName: loadProjects[index].prefix || loadProjects[index].name,
            message: errorMessage(result.reason, 'Could not read closed work'),
          }]
        : [])
      loaded.forEach(({ project, work }) => rememberBeadRows(project.path, work.beads))
      const closedRows = loaded.flatMap(({ project, work }) => work.beads.map(bead => ({
        ...bead,
        projectPath: project.path,
        projectName: project.prefix || project.name,
      })))
      setClosedCache(previous => ({
        ...previous,
        [selected]: { loading: false, rows: closedRows, failures: [...knownFailures, ...failures] },
      }))
      if (failures.length > 0 || knownFailures.length > 0) {
        const report = [...knownFailures, ...failures].map(failure => `${failure.projectName}: ${failure.message}`).join(' · ')
        announce(`Closed Beads unavailable · ${report}`, 'error')
      }
    })
  }, [announce, projectsReady, readableProjects, selected, unreadableProjects, view])

  // Formula and molecule lists belong to one store. They load when the store
  // is selected, remain in the rail, and fail independently of open work.
  useEffect(() => {
    if (!selectedStore || selectedStore.error || templateCatalogRequested.current.has(selectedStore.path)) return
    const path = selectedStore.path
    templateCatalogRequested.current.add(path)
    setTemplateCatalogs(previous => ({
      ...previous,
      [path]: { loading: true, formulas: [], molecules: [], formulaError: null, moleculeError: null },
    }))
    void Promise.allSettled([fetchFormulas(path), fetchMolecules(path)]).then(([formulaResult, moleculeResult]) => {
      setTemplateCatalogs(previous => ({
        ...previous,
        [path]: {
          loading: false,
          formulas: formulaResult.status === 'fulfilled' ? formulaResult.value.formulas : [],
          molecules: moleculeResult.status === 'fulfilled' ? moleculeResult.value.molecules : [],
          formulaError: formulaResult.status === 'rejected'
            ? errorMessage(formulaResult.reason, 'Could not read formulas')
            : null,
          moleculeError: moleculeResult.status === 'rejected'
            ? errorMessage(moleculeResult.reason, 'Could not read molecules')
            : null,
        },
      }))
    })
  }, [selectedStore])

  useEffect(() => {
    if (!reveal) return
    setSelected(reveal.projectPath)
    setQuery(reveal.id)
    setView('map')
    updateSettings({ beadsSelectedProject: reveal.projectPath, beadsView: 'map' })
  }, [reveal, updateSettings])

  const map = useMemo(() => filterBeadTree(buildBeadMap(rows), query), [rows, query])
  const matching = useMemo(() => filterBeadRows(rows, query), [rows, query])
  const closed = closedCache[selected]
  const closedMatching = useMemo(() => filterBeadRows(closed?.rows ?? [], query), [closed?.rows, query])
  const loadedClosedRows = useMemo(
    () => Object.values(closedCache).flatMap(snapshot => snapshot.loading ? [] : snapshot.rows),
    [closedCache],
  )
  const flowRows = useMemo(
    () => mergeFlowRows(rows, loadedClosedRows, flowSupplements),
    [flowSupplements, loadedClosedRows, rows],
  )
  const flowRowsRef = useRef(flowRows)
  flowRowsRef.current = flowRows
  const catalog = selectedStore ? templateCatalogs[selectedStore.path] : undefined
  const formulas = (catalog?.formulas ?? []).filter(formula => formulaName(formula) !== '')
  const protos = (catalog?.molecules ?? []).filter(molecule => moleculeID(molecule) !== '' && isTemplateProto(molecule))
  const molecules = (catalog?.molecules ?? []).filter(molecule => moleculeID(molecule) !== '' && !isTemplateProto(molecule))
  const selectProject = useCallback((path: string) => {
    setFlowReveal(null)
    setTemplateSelection(null)
    setTemplateDetail(null)
    setSelected(path)
    updateSettings({ beadsSelectedProject: path })
  }, [updateSettings])
  const selectView = useCallback((next: BeadsTabView) => {
    setFlowReveal(null)
    setTemplateSelection(null)
    setTemplateDetail(null)
    setView(next)
    updateSettings({ beadsView: next })
  }, [updateSettings])
  const openInFlow = useCallback((row: WorkRow) => {
    flowRequestNonce.current += 1
    const request = flowRequestNonce.current
    void fetchBead(row.projectPath, row.id).then(detail => {
      if (flowRequestNonce.current !== request) return
      const supplements = flowRowsFromDetail(flowRowsRef.current, row, detail)
      const nextRows = mergeFlowRows(flowRowsRef.current, supplements)
      const target = nextRows.find(candidate => candidate.projectPath === row.projectPath && candidate.id === row.id) ?? row
      const component = flowComponent(nextRows, target)
      setFlowSupplements(previous => mergeFlowRows(previous, supplements))
      flowRevealNonce.current += 1
      setFlowReveal({
        projectPath: row.projectPath,
        id: row.id,
        graphKey: flowComponentKey(component),
        nonce: flowRevealNonce.current,
      })
      setTemplateSelection(null)
      setTemplateDetail(null)
      setSelected(row.projectPath)
      setQuery('')
      setView('flow')
      updateSettings({ beadsSelectedProject: row.projectPath, beadsView: 'flow' })
    }).catch((cause: unknown) => {
      if (flowRequestNonce.current !== request) return
      announce(`Flow unavailable · ${row.id}: ${errorMessage(cause, 'Could not read linked work')}`, 'error')
    })
  }, [announce, updateSettings])
  const openTemplate = useCallback((selection: TemplateSelection) => {
    templateRequestNonce.current += 1
    const nonce = templateRequestNonce.current
    setTemplateSelection(selection)
    setTemplateDetail({ loading: true, detail: null, error: null })
    const request = selection.kind === 'formula'
      ? fetchFormula(selection.projectPath, selection.key)
      : fetchMolecule(selection.projectPath, selection.key)
    void request.then(detail => {
      if (templateRequestNonce.current === nonce) setTemplateDetail({ loading: false, detail, error: null })
    }).catch((cause: unknown) => {
      if (templateRequestNonce.current === nonce) {
        setTemplateDetail({ loading: false, detail: null, error: errorMessage(cause, `Could not read ${selection.kind}`) })
      }
    })
  }, [])
  const commitRailWidth = useCallback((beads: number) => {
    updateSettings({ railWidth: { ...settings.railWidth, beads } })
  }, [settings.railWidth, updateSettings])

  return (
    <div className="beads-view">
      <Rail
        className="beads-rail"
        role="navigation"
        label="Beads projects"
        width={settings.railWidth.beads}
        onWidthCommit={commitRailWidth}
      >
        <RailSection fill>
        <RailScroll>
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
              aria-expanded={quietShown}
              onClick={() => setQuietShown(open => !open)}
            >
              {quietShown ? 'Fewer' : `More (${quietProjects.length} quiet)`}
            </button>
          )}
          {quietShown && quietProjects.map(project => (
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
          {!quietShown && selectedQuiet && (
            <button
              type="button"
              className="beads-rail-item beads-rail-quiet active"
              onClick={() => selectProject(selectedQuiet.path)}
              title={selectedQuiet.path}
            >
              {selectedQuiet.prefix || selectedQuiet.name}
            </button>
          )}
        </RailScroll>
        </RailSection>

        {/* The selected store's own state. */}
        <RailSection fill title="Store" className="beads-rail-state">
          <RailScroll>
            <StoreState store={selectedStore} />
          </RailScroll>
        </RailSection>

        <RailSection fill title="Templates" className="beads-rail-templates">
          <RailScroll>
            {!selectedStore && <p className="beads-rail-note">Choose a store to browse formulas and molecules.</p>}
            {selectedStore?.error && <p className="beads-rail-error">{selectedStore.error}</p>}
            {selectedStore && !selectedStore.error && (!catalog || catalog.loading) && (
              <p className="beads-rail-note">Reading templates…</p>
            )}
            {selectedStore && catalog && !catalog.loading && (
              <>
                {catalog.formulaError && <p className="beads-rail-error">Formulas: {catalog.formulaError}</p>}
                {catalog.moleculeError && <p className="beads-rail-error">Molecules: {catalog.moleculeError}</p>}
                {formulas.length > 0 && <p className="beads-template-group-label">Formulas</p>}
                {formulas.map(formula => {
                  const name = formulaName(formula)
                  return (
                    <button
                      key={`formula:${name}`}
                      type="button"
                      className={`beads-template-item ${templateSelection?.kind === 'formula' && templateSelection.key === name ? 'active' : ''}`}
                      title={formula.description || formula.source || name}
                      onClick={() => openTemplate({ kind: 'formula', key: name, label: name, projectPath: selectedStore.path })}
                    >
                      {name}
                    </button>
                  )
                })}
                {protos.length > 0 && <p className="beads-template-group-label">Template protos</p>}
                {protos.map(molecule => {
                  const id = moleculeID(molecule)
                  return (
                    <button
                      key={`proto:${id}`}
                      type="button"
                      className={`beads-template-item ${templateSelection?.kind === 'molecule' && templateSelection.key === id ? 'active' : ''}`}
                      title={id}
                      onClick={() => openTemplate({ kind: 'molecule', key: id, label: moleculeTitle(molecule), projectPath: selectedStore.path })}
                    >
                      <span>{moleculeTitle(molecule)}</span>
                      <small>{id}</small>
                    </button>
                  )
                })}
                {molecules.length > 0 && <p className="beads-template-group-label">Molecules</p>}
                {molecules.map(molecule => {
                  const id = moleculeID(molecule)
                  return (
                    <button
                      key={`molecule:${id}`}
                      type="button"
                      className={`beads-template-item ${templateSelection?.kind === 'molecule' && templateSelection.key === id ? 'active' : ''}`}
                      title={id}
                      onClick={() => openTemplate({ kind: 'molecule', key: id, label: moleculeTitle(molecule), projectPath: selectedStore.path })}
                    >
                      <span>{moleculeTitle(molecule)}</span>
                      <small>{id}</small>
                    </button>
                  )
                })}
                {formulas.length === 0 && protos.length === 0 && molecules.length === 0 &&
                  !catalog.formulaError && !catalog.moleculeError && (
                    <p className="beads-rail-note">No formulas or molecules in this store.</p>
                  )}
              </>
            )}
          </RailScroll>
        </RailSection>
      </Rail>

      <div className="beads-main">
        <div className="beads-controls">
          <div className="beads-views" role="tablist" aria-label="Beads views">
            {VIEWS.map(item => (
              <button
                key={item.id}
                type="button"
                role="tab"
                aria-selected={!templateSelection && view === item.id}
                className={`beads-view-tab ${!templateSelection && view === item.id ? 'active' : ''}`}
                onClick={() => selectView(item.id)}
              >
                {item.label}
              </button>
            ))}
          </div>
          {!templateSelection && (
            <input
              className="beads-search"
              type="search"
              value={query}
              placeholder={view === 'closed' ? 'Search closed Beads…' : 'Search Beads…'}
              aria-label={view === 'closed' ? 'Search closed Beads' : 'Search Beads'}
              onChange={event => setQuery(event.target.value)}
            />
          )}
          {templateSelection && <span className="beads-template-mode">Read-only template</span>}
          {!templateSelection && view === 'stale' && (
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

        <FlowNavigationProvider rows={flowRows} reveal={openInFlow}>
          <div className="beads-content">
            {templateSelection && selectedStore && (
              <TemplateExplorer
                kind={templateSelection.kind}
                fallbackName={templateSelection.label}
                projectName={selectedStore.prefix || selectedStore.name}
                projectPath={selectedStore.path}
                loading={templateDetail?.loading ?? false}
                error={templateDetail?.error ?? null}
                detail={templateDetail?.detail ?? null}
              />
            )}
            {!templateSelection && view !== 'closed' && error && <p className="beads-error">{error}</p>}
            {!templateSelection && view !== 'closed' && !error && loading && <p className="beads-empty">Reading Beads…</p>}
            {!templateSelection && !error && !loading && view === 'map' && <MapView roots={map} expandAll={query.trim() !== ''} />}
            {!templateSelection && !error && !loading && view === 'ready' && (
              <ReadyView ready={readyRows(matching)} inProgress={inProgressRows(matching)} />
            )}
            {/* The flow is a graph: search narrows the lists, not the drawing,
                because a filtered graph loses the edges that explain it. */}
            {!templateSelection && !error && !loading && view === 'flow' && <FlowView rows={flowRows} reveal={flowReveal} />}
            {!templateSelection && !error && !loading && view === 'stale' && <StaleView rows={staleRows(matching, staleDays)} />}
            {!templateSelection && view === 'closed' && projects.length === 0 && error && <p className="beads-error">{error}</p>}
            {!templateSelection && view === 'closed' && !(projects.length === 0 && error) && (!closed || closed.loading) && (
              <p className="beads-empty">Reading closed Beads…</p>
            )}
            {!templateSelection && view === 'closed' && !(projects.length === 0 && error) && closed && !closed.loading && (
              <ClosedView rows={closedMatching} failures={closed.failures} query={query} />
            )}
          </div>
        </FlowNavigationProvider>
      </div>

      <TableColumn />
      <ResidentColumn active={active} tab="beads" reference={table ? tableReference(table) : null} />
    </div>
  )
}
