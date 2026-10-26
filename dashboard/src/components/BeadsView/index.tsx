/**
 * The Beads tab: every Bead on the host in one table, sliced by search,
 * project, status and days, with the selected Bead's work graph and its text
 * beside it.
 *
 * The strip across the top counts what was created and closed each day and
 * brushes a range of days; the rail keeps projects and statuses; the columns
 * sort; List and Map choose what the work area shows. List is the table, and
 * once a Bead is selected its work graph and text beside it; Map is the slice
 * as the Brain's night sky, each epic a star system whose finished work is its
 * glow, with the selected Bead's text beside it. The slice is
 * the URL, so a copied link opens the same view. Every interaction here is the
 * browser's own work over an index built once per read: the server is asked
 * for nothing but changed rows, for a search the ids whose text matches, and
 * for a selected Bead its text. A store the server holds only an earlier read
 * of is drawn from that read at once, named in the footer and the rail with
 * its age, and replaced when the new read lands. Nothing here writes; creating
 * and closing Beads stays with bd and the agents.
 */

import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import BeadTable, { idColumnChars, tableMinWidth } from './BeadTable'
import BeadText from './BeadText'
import FacetRail from './FacetRail'
import TimeStrip from './TimeStrip'
import WorkGraph from './WorkGraph'
import BrainSky from '../LibraryView/BrainSky'
import type { SkyWords } from '../LibraryView/SkyMap'
import ResidentColumn from '../ResidentColumn'
import SearchField from '../kit/SearchField'
import { TableSlot } from '../TableHost'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { tableReference, useTableObject } from '../../context/TableContext'
import { openBeadCard } from '../../beads/beadCard'
import { beadIdPattern } from '../../beads/beadIds'
import { searchHostBeads, useHostBeads } from '../../beads/hostBeads'
import { beadsSky } from '../../beads/beadsSky'
import { rememberBeadRows } from '../../beads/knownBeads'
import {
  SLICE_STATUSES,
  ageLabel,
  buildSliceIndex,
  dayLabel,
  filterSlice,
  orderedRows,
  rowReference,
  searchTerms,
  storeRowsForCard,
  textMatchMask,
  type SliceSortKey,
  type SortDirection,
} from '../../beads/beadsSlice'
import {
  BEADS_MODES,
  clearViewState,
  readViewState,
  writeViewState,
  type BeadsMode,
  type SliceViewState,
  type StripSpan,
} from '../../beads/sliceUrl'
import { registerChords } from '../../keys/chords'
import { copyAndAnnounce } from '../../utils/clipboard'
import './BeadsView.css'

export interface BeadsRevealRequest {
  projectPath: string
  id: string
  nonce: number
}

interface BeadsViewProps {
  active?: boolean
  /** A Bead the card asked to be shown here. */
  reveal?: BeadsRevealRequest | null
}

const MODE_LABELS: Record<BeadsMode, string> = { list: 'List', map: 'Map' }
/** What the map calls an epic's work. */
const MAP_WORDS: SkyWords = {
  count: (open, done) => `${open} open${done > 0 ? ` · ${done} done` : ''}`,
  coreless: 'filed under no epic',
  stray: 'linked to nothing',
  empty: 'No Beads in this slice.',
}
/** A link is rewritten once the slice settles, not on every step of a brush. */
const URL_DELAY_MS = 200
/** Content search waits for a pause in typing; ids and titles answer at once. */
const SEARCH_DELAY_MS = 120

const NUMBER = new Intl.NumberFormat('en-US')
const NO_SKY = { graph: { pages: [], links: [], tags: [] }, pathOf: () => '', rowOf: () => -1, nameOf: () => undefined }
const EMPTY_SKY = { full: NO_SKY, visible: NO_SKY }

function clockTime(iso: string | undefined): string {
  if (!iso) return ''
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(at.getHours())}:${pad(at.getMinutes())}:${pad(at.getSeconds())}`
}

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || !!target.closest('input, textarea, select, .terminal-window-body, .peek-body, .xterm')
}

function initialView(): SliceViewState {
  return readViewState(typeof window === 'undefined' ? '' : window.location.search)
}

export default function BeadsView({ active = true, reveal }: BeadsViewProps = {}) {
  const { settings, updateSettings, openSendToSession } = useSession()
  const { announce } = useStatus()
  const table = useTableObject()
  const [view, setView] = useState<SliceViewState>(initialView)
  const patch = useCallback((next: Partial<SliceViewState>) => setView(current => ({ ...current, ...next })), [])
  // Rises when the selection moved by anything but a click in the table, so
  // the table brings the row into view without fighting the operator's scroll.
  const [revealNonce, setRevealNonce] = useState(0)
  const searchBox = useRef<HTMLInputElement>(null)

  const manualPaths = useMemo(() => settings.beadsProjectPaths || [], [settings.beadsProjectPaths])
  const host = useHostBeads(active, manualPaths)
  const index = useMemo(() => buildSliceIndex(host.rowStores), [host.rowStores])
  const idChars = idColumnChars(index)
  // What the text pane needs from the read: each store's generation, so a
  // text is asked for once per read, and the prefixes an id in it can carry.
  const generations = useMemo(() => new Map(host.rowStores.map(store => [store.path, store.generation])), [host.rowStores])
  const idPattern = useMemo(
    () => beadIdPattern([...new Set(host.rowStores.map(store => store.prefix ?? '').filter(Boolean))]),
    [host.rowStores],
  )
  // A store drawn from an earlier read, by its project, with the read's age.
  // The age is recomputed on each poll; the map changes only when it reads differently.
  const ageKey = host.stores
    .filter(store => store.stale)
    .map(store => `${store.path}\u0000${ageLabel(store.readAt)}`)
    .join('\u0001')
  const ages = useMemo(() => {
    const byPath = new Map(index.projects.map((project, p) => [project.path, p]))
    const found = new Map<number, string>()
    ageKey.split('\u0001').filter(Boolean).forEach(entry => {
      const [path, age] = entry.split('\u0000')
      const p = byPath.get(path)
      if (p !== undefined && age) found.set(p, age)
    })
    return found
  }, [ageKey, index.projects])

  // The link is the slice: written while the tab is in front, taken away when
  // the operator leaves it, so another tab's address carries no Beads state.
  const shown = useRef(false)
  useEffect(() => {
    if (!active) {
      if (shown.current) {
        shown.current = false
        window.history.replaceState(window.history.state, '', clearViewState(new URL(window.location.href)).toString())
      }
      return
    }
    shown.current = true
    const timer = setTimeout(() => {
      const next = writeViewState(view, new URL(window.location.href)).toString()
      if (next !== window.location.href) window.history.replaceState(window.history.state, '', next)
    }, URL_DELAY_MS)
    return () => clearTimeout(timer)
  }, [active, view])

  const projectSet = useMemo(() => {
    if (!view.projects) return null
    const keys = new Set(view.projects)
    const kept = new Set<number>()
    index.projects.forEach((project, p) => { if (keys.has(project.key)) kept.add(p) })
    return kept
  }, [index, view.projects])
  const statusSet = useMemo(
    () => view.statuses ? new Set(view.statuses.map(status => SLICE_STATUSES.indexOf(status))) : null,
    [view.statuses],
  )

  // Ids and titles are searched here as the operator types; descriptions,
  // designs, acceptance and notes are searched by the server, which keeps
  // their text, and the two answers are joined.
  const terms = useMemo(() => searchTerms(view.q), [view.q])
  const termKey = terms.join(' ')
  const [found, setFound] = useState<{ key: string; matches: Record<string, string[]> } | null>(null)
  const [searchError, setSearchError] = useState<string | null>(null)
  useEffect(() => {
    if (!active || !termKey) return
    const controller = new AbortController()
    const timer = setTimeout(() => {
      searchHostBeads(termKey, controller.signal)
        .then(matches => { setFound({ key: termKey, matches }); setSearchError(null) })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted) setSearchError(cause instanceof Error ? cause.message : 'Content search failed')
        })
    }, SEARCH_DELAY_MS)
    return () => { clearTimeout(timer); controller.abort() }
  }, [active, termKey, host.rowStores])
  // While the answer for a longer or shorter query is on its way, the last
  // answer stands: its rows narrow or widen once, rather than vanishing on
  // every keystroke and coming back.
  const related = !!found && !!termKey && (termKey.startsWith(found.key) || found.key.startsWith(termKey))
  const textMatches = useMemo(
    () => found && related ? textMatchMask(index, found.matches) : null,
    [found, index, related],
  )

  const result = useMemo(() => filterSlice(index, {
    terms, textMatches, projects: projectSet, statuses: statusSet, range: view.range,
  }), [index, projectSet, statusSet, terms, textMatches, view.range])
  const ordered = useMemo(
    () => orderedRows(index, result, view.sort, view.dir),
    [index, result, view.dir, view.sort],
  )
  // The map places every Bead of the projects in view, so a status or a search
  // fades stars rather than moving them, and lights the slice.
  const sky = useMemo(() => {
    if (view.mode !== 'map') return EMPTY_SKY
    const all: number[] = []
    for (let row = 0; row < index.size; row += 1) if (!projectSet || projectSet.has(index.project[row])) all.push(row)
    return { full: beadsSky(index, all), visible: beadsSky(index, ordered) }
  }, [index, ordered, projectSet, view.mode])

  const selectedRow = useMemo(() => {
    if (!view.selected) return -1
    const key = view.selected.project
    const p = key ? index.projects.findIndex(project => project.key === key) : -1
    return index.find(view.selected.id, p >= 0 ? p : undefined)
  }, [index, view.selected])

  const selectionOf = useCallback((row: number) => {
    const bead = index.rows[row]
    return index.find(bead.id) === row
      ? { id: bead.id }
      : { id: bead.id, project: index.projects[index.project[row]].key }
  }, [index])
  const selectInTable = useCallback((row: number) => patch({ selected: selectionOf(row) }), [patch, selectionOf])
  const deselect = useCallback(() => patch({ selected: null }), [patch])
  const selectElsewhere = useCallback((row: number) => {
    patch({ selected: selectionOf(row) })
    setRevealNonce(nonce => nonce + 1)
  }, [patch, selectionOf])
  const open = useCallback((row: number) => {
    const bead = index.rows[row]
    const path = index.projects[index.project[row]].path
    // The card draws what the table already knows at once, and its text when bd answers.
    rememberBeadRows(path, storeRowsForCard(index, index.project[row]))
    openBeadCard(bead.id, path, bead.title)
  }, [index])
  const send = useCallback((row: number) => openSendToSession({ reference: rowReference(index, row) }), [index, openSendToSession])
  const copy = useCallback((text: string, what: string) => { void copyAndAnnounce(text, what, announce) }, [announce])
  const sortBy = useCallback((sort: SliceSortKey, dir: SortDirection) => patch({ sort, dir }), [patch])
  const setRange = useCallback((range: SliceViewState['range']) => patch({ range }), [patch])
  const setSpan = useCallback((span: StripSpan) => patch({ span }), [patch])
  // An id in a Bead's text selects that Bead here; one the host does not
  // hold, or holds twice, goes on the table, where the card resolves it.
  const followToken = useCallback((id: string) => {
    const row = index.find(id)
    if (row >= 0) selectElsewhere(row)
    else openBeadCard(id)
  }, [index, selectElsewhere])
  const keepProjects = useCallback((kept: ReadonlySet<number> | null) => {
    patch({ projects: kept === null ? null : index.projects.filter((_, p) => kept.has(p)).map(project => project.key) })
  }, [index, patch])
  const keepStatuses = useCallback((kept: ReadonlySet<number> | null) => {
    patch({ statuses: kept === null ? null : SLICE_STATUSES.filter((_, s) => kept.has(s)) })
  }, [patch])
  const commitRailWidth = useCallback((beads: number) => {
    updateSettings({ railWidth: { ...settings.railWidth, beads } })
  }, [settings.railWidth, updateSettings])

  // A selection arriving with a link is brought into view once its store is read.
  const linkSelection = useRef(view.selected !== null)
  useEffect(() => {
    if (!linkSelection.current || selectedRow < 0) return
    linkSelection.current = false
    setRevealNonce(nonce => nonce + 1)
  }, [selectedRow])

  // Open in Beads, from the card: the Bead is selected, and the slice is
  // cleared only when it hides the Bead. A Bead whose store is still being
  // read waits for it.
  const [pendingReveal, setPendingReveal] = useState<BeadsRevealRequest | null>(null)
  useEffect(() => { if (reveal) setPendingReveal(reveal) }, [reveal])
  useEffect(() => {
    if (!pendingReveal) return
    let row = index.rowOf(pendingReveal.projectPath, pendingReveal.id)
    if (row < 0) row = index.find(pendingReveal.id)
    if (row < 0) return
    setPendingReveal(null)
    const hidden = result.visible[row] !== 1
    patch({
      ...(hidden ? { q: '', projects: null, statuses: null, range: null } : {}),
      selected: selectionOf(row),
    })
    setRevealNonce(nonce => nonce + 1)
  }, [index, patch, pendingReveal, result, selectionOf])

  const focusSearch = useCallback(() => {
    searchBox.current?.focus()
    searchBox.current?.select()
  }, [])
  // Plain `/` is search outside a terminal, as it is on every tab; the leader's
  // `/` reaches it from anywhere while the tab is in front.
  useEffect(() => {
    if (!active) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey || event.defaultPrevented) return
      if (isTyping(event.target)) return
      event.preventDefault()
      focusSearch()
    }
    window.addEventListener('keydown', onKeyDown)
    const retire = registerChords([{ id: 'beads.search', key: '/', label: 'Search Beads', scope: 'global', run: focusSearch }])
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      retire()
    }
  }, [active, focusSearch])

  const copyLink = () => copy(writeViewState(view, new URL(window.location.href)).toString(), 'the view link')

  const clearSlice = () => patch({ q: '', projects: null, statuses: null, range: null })

  // The layout switch is one tab stop; the arrows move between its choices.
  const moveMode = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    const step = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0
    if (!step) return
    event.preventDefault()
    const next = BEADS_MODES[(BEADS_MODES.indexOf(view.mode) + step + BEADS_MODES.length) % BEADS_MODES.length]
    patch({ mode: next })
    document.getElementById(`beads-mode-${next}`)?.focus()
  }

  const reference = selectedRow >= 0 ? rowReference(index, selectedRow) : table ? tableReference(table) : null
  const readStores = host.stores.filter(store => store.generation)
  const pendingStores = host.stores.filter(store => store.pending)
  const failedStores = host.stores.filter(store => store.error)
  const staleStores = host.stores.filter(store => store.stale)
  const oldestStale = staleStores.reduce<string | undefined>(
    (oldest, store) => !oldest || (store.readAt && store.readAt < oldest) ? store.readAt : oldest, undefined)
  const showTable = view.mode === 'list'
  const showMap = view.mode === 'map'
  const showDetail = selectedRow >= 0
  const empty = host.loading
    ? 'Reading every Beads store on this host…'
    : index.size === 0
      ? pendingStores.length > 0
        ? `Reading ${pendingStores.length} ${pendingStores.length === 1 ? 'store' : 'stores'}…`
        : 'No Beads in any store this host can read.'
      : result.count === 0 ? 'No Beads in this slice.' : null

  return (
    <div className="beads-view" style={{ '--beads-table-min': tableMinWidth(idChars) } as CSSProperties}>
      <div className="beads-slicer" data-ui="beads.slicer">
        <div className="beads-top">
          <SearchField
            className="beads-search"
            inputRef={searchBox}
            value={view.q}
            label="Search Beads"
            placeholder="Search Beads by title, id or text…"
            hint={<span aria-hidden="true">/</span>}
            onChange={q => patch({ q })}
            onKeyDown={event => {
              if (event.key !== 'ArrowDown' && event.key !== 'Enter') return
              event.preventDefault()
              if (selectedRow < 0 && ordered.length > 0) selectInTable(ordered[0])
              if (showTable) document.querySelector<HTMLElement>('.beads-table-body')?.focus()
            }}
          />
          <span className={`beads-range${view.range ? ' set' : ''}`} data-ui="beads.range">
            {view.range ? (
              <>
                <span>{view.range.from === view.range.to ? dayLabel(view.range.from) : `${dayLabel(view.range.from)} to ${dayLabel(view.range.to)}`}</span>
                <button type="button" aria-label="Clear the date range" onClick={() => setRange(null)}>×</button>
              </>
            ) : <span>All days</span>}
          </span>
          <button type="button" className="kit-button" onClick={copyLink}>Copy view link</button>
        </div>

        <TimeStrip index={index} result={result} range={view.range} onRange={setRange} span={view.span} onSpan={setSpan} />

        <div className="beads-body">
          <FacetRail
            index={index}
            result={result}
            projects={projectSet}
            statuses={statusSet}
            onProjects={keepProjects}
            onStatuses={keepStatuses}
            ages={ages}
            width={settings.railWidth.beads}
            onWidthCommit={commitRailWidth}
          />
          <div className="beads-work">
            <div className="beads-work-head">
              <span className="beads-count" role="status">
                {`${NUMBER.format(result.count)} of ${NUMBER.format(index.size)} Beads`}
              </span>
              <div className="beads-modes kit-tabs" role="tablist" aria-label="Beads layout" onKeyDown={moveMode}>
                {BEADS_MODES.map(mode => (
                  <button
                    key={mode}
                    id={`beads-mode-${mode}`}
                    type="button"
                    role="tab"
                    aria-selected={view.mode === mode}
                    aria-controls="beads-panes"
                    tabIndex={view.mode === mode ? 0 : -1}
                    className="kit-button beads-mode"
                    onClick={() => patch({ mode })}
                  >
                    {MODE_LABELS[mode]}
                  </button>
                ))}
              </div>
            </div>
            <div
              id="beads-panes"
              role="tabpanel"
              aria-labelledby={`beads-mode-${view.mode}`}
              className={`beads-panes beads-panes-${view.mode}${showDetail ? ' with-detail' : ''}`}
            >
              {showTable && (
                <div className="beads-pane beads-pane-table">
                  <BeadTable
                    index={index}
                    idChars={idChars}
                    ordered={ordered}
                    selected={selectedRow}
                    sort={view.sort}
                    dir={view.dir}
                    reveal={revealNonce}
                    onSort={sortBy}
                    onSelect={selectInTable}
                    onDeselect={deselect}
                    onOpen={open}
                    onSend={send}
                    onCopy={copy}
                  />
                  {empty && (
                    <div className="beads-empty" role="status">
                      <p>{empty}</p>
                      {result.count === 0 && index.size > 0 && !host.loading && (
                        <button type="button" className="kit-button primary beads-clear" onClick={clearSlice}>Show every Bead</button>
                      )}
                    </div>
                  )}
                </div>
              )}
              {showMap && (
                <div className="beads-pane beads-pane-map">
                  <BrainSky
                    label="Beads map"
                    placementKey="chrote.beads.sky.placement"
                    graph={sky.visible.graph}
                    fullGraph={sky.full.graph}
                    nameOf={sky.full.nameOf}
                    words={MAP_WORDS}
                    matches={null}
                    openPath={selectedRow >= 0 ? sky.full.pathOf(selectedRow) : null}
                    onOpen={path => { const row = sky.full.rowOf(path); if (row >= 0) selectInTable(row) }}
                    timeControl={null}
                  />
                </div>
              )}
              {showDetail && (
                <div className="beads-pane beads-pane-detail">
                  <div className="beads-detail">
                    {showTable && (
                      <div className="beads-detail-graph">
                        <WorkGraph index={index} selected={selectedRow} onSelect={selectElsewhere} onOpen={open} />
                      </div>
                    )}
                    <BeadText index={index} row={selectedRow} generations={generations} idPattern={idPattern} onToken={followToken} />
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>

        <footer className="beads-footer" data-ui="beads.footer">
          <span>{host.loading ? 'Reading stores' : `${readStores.length} ${readStores.length === 1 ? 'store' : 'stores'} read`}</span>
          {staleStores.length > 0 && (
            <span className="beads-footer-stale" title={staleStores.map(store => `${store.path}: as read ${ageLabel(store.readAt)} ago`).join('\n')}>
              {`${staleStores.length} as read up to ${ageLabel(oldestStale)} ago${staleStores.some(store => store.pending) ? ', refreshing' : ''}`}
            </span>
          )}
          {pendingStores.length > staleStores.filter(store => store.pending).length && (
            <span>{`${pendingStores.length - staleStores.filter(store => store.pending).length} reading`}</span>
          )}
          {failedStores.map(store => (
            <span key={store.path} className="beads-footer-error" title={`${store.path}: ${store.error}`}>
              {`${store.path}: ${store.error}`}
            </span>
          ))}
          {host.error && <span className="beads-footer-error">{host.error}</span>}
          {searchError && termKey && <span className="beads-footer-error">{`Content search: ${searchError}`}</span>}
          {host.checkedAt && <span>{`refreshed ${clockTime(host.checkedAt)}`}</span>}
        </footer>
      </div>

      <TableSlot active={active} />
      <ResidentColumn active={active} tab="beads" reference={reference} />
    </div>
  )
}
