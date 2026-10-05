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
import ClosedView from './ClosedView'
import FlowView from './FlowView'
import { FlowNavigationProvider } from './FlowNavigation'
import MapView from './MapView'
import ReadyView from './ReadyView'
import StoreState from './StoreState'
import StaleView from './StaleView'
import TemplateExplorer from './TemplateExplorer'
import ResidentColumn from '../ResidentColumn'
import { TableSlot } from '../TableHost'
import Rail, { RailScroll, RailSection } from '../Rail'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { tableReference, useTableObject } from '../../context/TableContext'
import { refreshBeadProjects } from '../../beads/beadIds'
import {
  fetchBeadProjectList,
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
import { beadProjectSkeletons, beadRows, beadStoreRead, rememberedBeadProjects, rememberBeadProjects, readBeadDetail, refreshBeads, useBeadsRead } from '../../beads/beadsRead'
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
import { BEAD_SORTS, sortBeadRows, sortBeadTree, type BeadSort } from '../../beads/beadsSort'
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
    parent: detail.parents[0]?.id,
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

/** Join only changed project projections, so freshness checks do not relayout Flow. */
function useScopeRows(projects: readonly BeadProject[], kind: 'work' | 'closed', currentOnly = false): WorkRow[] {
  const groups = projects.map(project => {
    const store = beadStoreRead(project.path)
    return currentOnly && store?.[kind].generation !== store?.state?.availableGeneration
      ? null : beadRows(project.path, project.prefix || project.name, kind)
  }).filter((rows): rows is WorkRow[] => rows !== null)
  const retained = useRef<{ groups: WorkRow[][]; rows: WorkRow[] }>({ groups: [], rows: [] })
  if (groups.length !== retained.current.groups.length || groups.some((rows, index) => rows !== retained.current.groups[index])) {
    retained.current = { groups, rows: groups.flat() }
  }
  return retained.current.rows
}

export default function BeadsView({ active = true, reveal }: BeadsViewProps = {}) {
  const { settings, updateSettings } = useSession()
  const { announce } = useStatus()
  const [listedProjects, setProjects] = useState<BeadProject[]>(() => beadProjectSkeletons(rememberedBeadProjects(), [
    ...(settings.beadsProjectPaths || []), ...(settings.beadsSelectedProject && settings.beadsSelectedProject !== ALL_PROJECTS ? [settings.beadsSelectedProject] : []),
  ]))
  const [projectsRefresh, setProjectsRefresh] = useState(0)
  const [projectsError, setProjectsError] = useState<string | null>(null)
  const [selected, setSelected] = useState<string>(settings.beadsSelectedProject || ALL_PROJECTS)
  const [view, setView] = useState<BeadsTabView>(
    VIEWS.some(item => item.id === settings.beadsView) ? settings.beadsView : 'map',
  )
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<BeadSort>('default')
  const [staleDays, setStaleDays] = useState(DEFAULT_STALE_DAYS)
  const [quietShown, setQuietShown] = useState(false)
  const [flowReveal, setFlowReveal] = useState<FlowRevealRequest | null>(null)
  const [flowSupplements, setFlowSupplements] = useState<WorkRow[]>([])
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
    if (!active) return
    let current = true
    const apply = (found: BeadProject[]) => {
      if (!current) return
      setProjects(previous => {
        const skeletons = beadProjectSkeletons(found, manualPaths).map(project => {
          const prior = previous.find(old => old.path === project.path)
          return { ...prior, ...project, prefix: project.prefix || prior?.prefix }
        })
        rememberBeadProjects(skeletons)
        return skeletons
      })
      setProjectsError(null)
    }
    // Publish the cheap host skeleton first. Identity enrichment (including
    // manually configured stores) is independent of source snapshot reads.
    void fetchBeadProjectList().then(found => {
      apply(found.map(project => ({ ...listedProjects.find(old => old.path === project.path), ...project })))
    }).catch((cause: unknown) => {
      if (current) setProjectsError(errorMessage(cause, 'Could not list Beads projects'))
    })
    void refreshBeadProjects(manualPaths).then(found => {
      if (current) setProjects(previous => {
        const merged = beadProjectSkeletons(found.map(project => ({ ...previous.find(old => old.path === project.path), ...project })), manualPaths)
        rememberBeadProjects(merged)
        return merged
      })
    }).catch((cause: unknown) => {
      if (current) announce(`Bead links unavailable · ${errorMessage(cause, 'Could not discover Bead links')}`, 'error')
    })
    return () => { current = false }
    // The remembered list is a seed; source discovery follows visibility and settings.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, announce, manualPaths, projectsRefresh])

  const scopePaths = useMemo(() => selected === ALL_PROJECTS
    ? listedProjects.map(project => project.path) : [selected], [listedProjects, selected])
  const readRevision = useBeadsRead({
    paths: scopePaths, foreground: selected === ALL_PROJECTS ? [] : [selected],
    work: true, closed: view === 'closed',
  }, active)
  const flowReadRevision = useBeadsRead({
    paths: flowReveal ? [flowReveal.projectPath] : [],
    ...(flowReveal ? { card: { path: flowReveal.projectPath, id: flowReveal.id } } : {}),
  }, active && view === 'flow' && !!flowReveal)
  const projects = useMemo(() => listedProjects.map(project => {
    const state = beadStoreRead(project.path)?.state
    return state ? {
      ...project, state, counts: state.counts, openBeads: state.openBeads,
      newestUpdate: state.newestUpdate, error: state.error, summaryPending: state.pending,
    } : project
  }), [listedProjects, readRevision])
  const unreadableProjects = projects.filter(project => !!project.error && !project.state?.availableGeneration)
  const readableProjects = projects.filter(project => !unreadableProjects.includes(project))
  const openProjects = readableProjects.filter(project => project.openBeads !== 0)
  const quietProjects = readableProjects.filter(project => project.openBeads === 0)
  const selectedStore = selected === ALL_PROJECTS ? null : projects.find(project => project.path === selected) ?? null
  const selectedQuiet = quietProjects.find(project => project.path === selected)
  const scoped = selected === ALL_PROJECTS ? projects : projects.filter(project => project.path === selected)
  const rows = useScopeRows(scoped, 'work')
  const loading = scoped.some(project => {
    const store = beadStoreRead(project.path)
    return !store?.work.data && !store?.work.error && !store?.state?.error
  }) || (listedProjects.length === 0 && !projectsError)
  const failures = scoped.flatMap(project => {
    const store = beadStoreRead(project.path)
    const message = store?.work.error || store?.state?.error || project.error
    return message ? [{ projectName: project.prefix || project.name, message }] : []
  })
  const error = failures.map(failure => `${failure.projectName}: ${failure.message}`).join(' · ') || projectsError
  const sourcePending = scoped.some(project => {
    const store = beadStoreRead(project.path)
    return project.state?.pending || store?.work.loading || !!store?.work.data && store.work.generation !== store.state?.availableGeneration
  })
  const incomplete = loading || scoped.some(project => !beadStoreRead(project.path)?.work.data)
  const closedRows = useScopeRows(scoped, 'closed')
  const closed = {
    rows: closedRows,
    loading: scoped.some(project => {
      const store = beadStoreRead(project.path)
      return !store?.closed.data && !store?.closed.error && !store?.state?.error
    }),
    failures: scoped.flatMap(project => {
      const store = beadStoreRead(project.path)
      const message = store?.closed.error || store?.state?.error || project.error
      return message ? [{ projectName: project.prefix || project.name, message }] : []
    }),
  }
  const reportRef = useRef('')
  useEffect(() => {
    if (!error || reportRef.current === error) return
    reportRef.current = error
    announce(`Beads unavailable · ${error}`, 'error')
  }, [announce, error])

  // Formula and molecule lists belong to one store. They load when the store
  // is selected, remain in the rail, and fail independently of open work.
  useEffect(() => {
    if (!active || !selectedStore || selectedStore.error || !beadStoreRead(selectedStore.path)?.work.data || templateCatalogRequested.current.has(selectedStore.path)) return
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
  }, [active, readRevision, selectedStore])

  useEffect(() => {
    if (!reveal) return
    setSelected(reveal.projectPath)
    setQuery(reveal.id)
    setView('map')
    updateSettings({ beadsSelectedProject: reveal.projectPath, beadsView: 'map' })
  }, [reveal, updateSettings])

  const map = useMemo(() => sortBeadTree(filterBeadTree(buildBeadMap(rows), query), sort), [rows, query, sort])
  const matching = useMemo(() => filterBeadRows(rows, query), [rows, query])
  const closedMatching = useMemo(() => sortBeadRows(filterBeadRows(closed?.rows ?? [], query), sort), [closed?.rows, query, sort])
  const loadedClosedRows = useScopeRows(listedProjects, 'closed', true)
  const flowRows = useMemo(
    () => mergeFlowRows(flowSupplements, loadedClosedRows, rows),
    [flowSupplements, loadedClosedRows, rows],
  )
  const flowRowsRef = useRef(flowRows)
  flowRowsRef.current = flowRows
  const flowDetailApplied = useRef<BeadDetail | null>(null)
  useEffect(() => {
    if (!flowReveal) return
    const store = beadStoreRead(flowReveal.projectPath)
    const card = store?.cards.get(flowReveal.id)
    if (!card?.data || card.data === flowDetailApplied.current || card.generation !== store?.state?.availableGeneration) return
    flowDetailApplied.current = card.data
    const target = rows.find(row => row.projectPath === flowReveal.projectPath && row.id === flowReveal.id)
      ?? flowSupplements.find(row => row.projectPath === flowReveal.projectPath && row.id === flowReveal.id)
    if (target) setFlowSupplements(flowRowsFromDetail(rows, target, card.data))
    // Data changes (including removed relations) replace this one-hop projection.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [flowReadRevision, flowReveal])
  const catalog = selectedStore ? templateCatalogs[selectedStore.path] : undefined
  const formulas = (catalog?.formulas ?? []).filter(formula => formulaName(formula) !== '')
  const protos = (catalog?.molecules ?? []).filter(molecule => moleculeID(molecule) !== '' && isTemplateProto(molecule))
  const molecules = (catalog?.molecules ?? []).filter(molecule => moleculeID(molecule) !== '' && !isTemplateProto(molecule))
  const selectProject = useCallback((path: string) => {
    setFlowReveal(null)
    setFlowSupplements([])
    setTemplateSelection(null)
    setTemplateDetail(null)
    setSelected(path)
    updateSettings({ beadsSelectedProject: path })
  }, [updateSettings])
  const selectView = useCallback((next: BeadsTabView) => {
    setFlowReveal(null)
    setFlowSupplements([])
    setTemplateSelection(null)
    setTemplateDetail(null)
    setView(next)
    updateSettings({ beadsView: next })
  }, [updateSettings])
  const openInFlow = useCallback((row: WorkRow) => {
    flowRequestNonce.current += 1
    const request = flowRequestNonce.current
    void readBeadDetail(row.projectPath, row.id).then(detail => {
      if (flowRequestNonce.current !== request) return
      const supplements = flowRowsFromDetail(flowRowsRef.current, row, detail)
      const nextRows = mergeFlowRows(flowRowsRef.current, supplements)
      const target = nextRows.find(candidate => candidate.projectPath === row.projectPath && candidate.id === row.id) ?? row
      const component = flowComponent(nextRows, target)
      setFlowSupplements(supplements)
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
        <button type="button" className="beads-refresh" onClick={() => { refreshBeads(scopePaths); setProjectsRefresh(value => value + 1) }}>
          Refresh projects
        </button>
        {projectsError && <p className="beads-rail-error">{projectsError}</p>}
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
        <div className={`beads-controls${view === 'flow' ? ' beads-controls-flow' : ''}`}>
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
          {!templateSelection && view !== 'flow' && (
            <label className="beads-sort">
              Sort
              <select aria-label="Sort Beads" value={sort} onChange={event => setSort(event.target.value as BeadSort)}>
                {BEAD_SORTS.map(option => <option key={option.id} value={option.id}>{option.label}</option>)}
              </select>
            </label>
          )}
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
            {!templateSelection && view === 'flow' && flowReveal && beadStoreRead(flowReveal.projectPath)?.cards.get(flowReveal.id)?.error && <p className="beads-error">Selected linked Bead unavailable · {beadStoreRead(flowReveal.projectPath)?.cards.get(flowReveal.id)?.error} · showing the last successful graph</p>}
            {!templateSelection && view !== 'closed' && loading && <p className="beads-empty">Reading Beads… · {scoped.filter(project => !!beadStoreRead(project.path)?.work.data).length}/{scoped.length} stores loaded</p>}
            {!templateSelection && !loading && sourcePending && <p className="beads-store-note">Refreshing Beads · showing the last successful read</p>}
            {!templateSelection && view === 'map' && <MapView roots={map} expandAll={query.trim() !== ''} incomplete={incomplete} />}
            {!templateSelection && view === 'ready' && (
              <ReadyView incomplete={incomplete} ready={sortBeadRows(readyRows(matching), sort)} inProgress={sortBeadRows(inProgressRows(matching), sort)} />
            )}
            {/* The flow is a graph: search narrows the lists, not the drawing,
                because a filtered graph loses the edges that explain it. */}
            {!templateSelection && view === 'flow' && <FlowView rows={flowRows} reveal={flowReveal} scopeKey={selected} incomplete={incomplete} />}
            {!templateSelection && view === 'stale' && <StaleView incomplete={incomplete} rows={sortBeadRows(staleRows(matching, staleDays), sort)} />}
            {!templateSelection && view === 'closed' && projects.length === 0 && error && <p className="beads-error">{error}</p>}
            {!templateSelection && view === 'closed' && !(projects.length === 0 && error) && (!closed || closed.loading) && (
              <p className="beads-empty">Reading closed Beads…</p>
            )}
            {!templateSelection && view === 'closed' && !(projects.length === 0 && error) && closed && (
              <ClosedView rows={closedMatching} failures={closed.failures} query={query} incomplete={closed.loading || scoped.some(project => !beadStoreRead(project.path)?.closed.data)} />
            )}
          </div>
        </FlowNavigationProvider>
      </div>

      <TableSlot active={active} />
      <ResidentColumn active={active} tab="beads" reference={table ? tableReference(table) : null} />
    </div>
  )
}
