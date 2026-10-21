/**
 * The Beads tab: every Bead on the host in one table, sliced by search,
 * project, status and days, with the selected Bead's work graph beside it.
 *
 * The strip across the top counts what was created and closed each day and
 * brushes a range of days; the rail keeps projects and statuses; the columns
 * sort; Split, Table and Graph choose what the work area shows. The slice is
 * the URL, so a copied link opens the same view. Every interaction here is the
 * browser's own work over an index built once per read: the server is asked
 * for nothing but changed rows and, for a search, the ids whose text matches.
 * Nothing here writes; creating and closing Beads stays with bd and the agents.
 */

import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import BeadTable from './BeadTable'
import FacetRail from './FacetRail'
import TimeStrip from './TimeStrip'
import WorkGraph from './WorkGraph'
import ResidentColumn from '../ResidentColumn'
import { TableSlot } from '../TableHost'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { tableReference, useTableObject } from '../../context/TableContext'
import { openBeadCard } from '../../beads/beadCard'
import { searchHostBeads, useHostBeads } from '../../beads/hostBeads'
import { rememberBeadRows } from '../../beads/knownBeads'
import {
  SLICE_STATUSES,
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

const MODE_LABELS: Record<BeadsMode, string> = { split: 'Split', table: 'Table', graph: 'Graph' }
/** A link is rewritten once the slice settles, not on every step of a brush. */
const URL_DELAY_MS = 200
/** Content search waits for a pause in typing; ids and titles answer at once. */
const SEARCH_DELAY_MS = 120

const NUMBER = new Intl.NumberFormat('en-US')

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
  const showTable = view.mode !== 'graph'
  const showGraph = view.mode !== 'table'
  const empty = host.loading
    ? 'Reading every Beads store on this host…'
    : index.size === 0
      ? pendingStores.length > 0
        ? `Reading ${pendingStores.length} ${pendingStores.length === 1 ? 'store' : 'stores'}…`
        : 'No Beads in any store this host can read.'
      : result.count === 0 ? 'No Beads in this slice.' : null

  return (
    <div className="beads-view">
      <div className="beads-slicer" data-ui="beads.slicer">
        <div className="beads-top">
          <label className="beads-search">
            <input
              ref={searchBox}
              type="search"
              value={view.q}
              placeholder="Search Beads (title, id, content)"
              aria-label="Search Beads"
              onChange={event => patch({ q: event.target.value })}
              onKeyDown={event => {
                if (event.key !== 'ArrowDown' && event.key !== 'Enter') return
                event.preventDefault()
                if (selectedRow < 0 && ordered.length > 0) selectInTable(ordered[0])
                if (showTable) document.querySelector<HTMLElement>('.beads-table-body')?.focus()
              }}
            />
            <span className="beads-chord" aria-hidden="true">/</span>
          </label>
          <span className={`beads-range${view.range ? ' set' : ''}`} data-ui="beads.range">
            {view.range ? (
              <>
                <span>{view.range.from === view.range.to ? dayLabel(view.range.from) : `${dayLabel(view.range.from)} to ${dayLabel(view.range.to)}`}</span>
                <button type="button" aria-label="Clear the date range" onClick={() => setRange(null)}>×</button>
              </>
            ) : <span>All days</span>}
          </span>
          <button type="button" className="beads-copy-link" onClick={copyLink}>Copy view link</button>
        </div>

        <TimeStrip index={index} result={result} range={view.range} onRange={setRange} />

        <div className="beads-body">
          <FacetRail
            index={index}
            result={result}
            projects={projectSet}
            statuses={statusSet}
            onProjects={keepProjects}
            onStatuses={keepStatuses}
            width={settings.railWidth.beads}
            onWidthCommit={commitRailWidth}
          />
          <div className="beads-work">
            <div className="beads-work-head">
              <span className="beads-count" role="status">
                {`${NUMBER.format(result.count)} of ${NUMBER.format(index.size)} Beads`}
              </span>
              <div className="beads-modes" role="tablist" aria-label="Beads layout" onKeyDown={moveMode}>
                {BEADS_MODES.map(mode => (
                  <button
                    key={mode}
                    id={`beads-mode-${mode}`}
                    type="button"
                    role="tab"
                    aria-selected={view.mode === mode}
                    aria-controls="beads-panes"
                    tabIndex={view.mode === mode ? 0 : -1}
                    className={`beads-mode${view.mode === mode ? ' active' : ''}`}
                    onClick={() => patch({ mode })}
                  >
                    {MODE_LABELS[mode]}
                  </button>
                ))}
              </div>
            </div>
            <div id="beads-panes" role="tabpanel" aria-labelledby={`beads-mode-${view.mode}`} className={`beads-panes beads-panes-${view.mode}`}>
              {showTable && (
                <div className="beads-pane beads-pane-table">
                  <BeadTable
                    index={index}
                    ordered={ordered}
                    selected={selectedRow}
                    sort={view.sort}
                    dir={view.dir}
                    reveal={revealNonce}
                    onSort={sortBy}
                    onSelect={selectInTable}
                    onOpen={open}
                    onSend={send}
                    onCopy={copy}
                  />
                  {empty && (
                    <div className="beads-empty" role="status">
                      <p>{empty}</p>
                      {result.count === 0 && index.size > 0 && !host.loading && (
                        <button type="button" className="beads-clear" onClick={clearSlice}>Show every Bead</button>
                      )}
                    </div>
                  )}
                </div>
              )}
              {showGraph && (
                <div className="beads-pane beads-pane-graph">
                  <WorkGraph index={index} selected={selectedRow} onSelect={selectElsewhere} onOpen={open} />
                </div>
              )}
            </div>
          </div>
        </div>

        <footer className="beads-footer" data-ui="beads.footer">
          <span>{host.loading ? 'Reading stores' : `${readStores.length} ${readStores.length === 1 ? 'store' : 'stores'} read`}</span>
          {pendingStores.length > 0 && <span>{`${pendingStores.length} reading`}</span>}
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
