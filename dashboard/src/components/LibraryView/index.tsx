/**
 * The Library: the map of the operator's own context corpus, dived into.
 *
 * A rail, the map, and the Librarian. The shelves and what arrived lately down
 * the left; the map of the whole corpus in the middle, always; the Librarian
 * live in a column at the far edge. The rail works the map rather than
 * replacing it: a shelf opens inside the rail, pointing at one of its pages
 * takes the map to that page and lights it, and clicking the page opens the
 * row on what it is with a dive to take. Diving puts the page in a column
 * beside the map with its neighbours as links to travel by and a trail back to
 * where the dive began. Escape ends the dive and the map is exactly where it
 * was left.
 *
 * The corpus is a Markdown tree under git, so every fact here — a title, a
 * date, an author, a history, a link — is read out of that tree rather than
 * kept anywhere in CHROTE. The one thing written back is the operator's own
 * correction, and it is a commit signed as him.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import BrainSky from './BrainSky'
import SearchResults from './SearchResults'
import PageReader, { usePageReader } from './PageReader'
import { useBrainSearch } from './useBrainSearch'
import { neighboursOf, subjectName, subjectOf, withHomes } from './brainSubjects'
import { NOW, SCRUB_STEPS, backFrom, existedAt, momentOf, positionOf, snapTo, stopsOn } from './mapMoment'
import { shelfHues } from './mapShelves'
import { useTheme } from '../../theme/ThemeContext'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { useSurface } from '../../keys/dismiss'
import { createLibraryResolver } from '../../library/libraryLinks'
import { getSessionKey } from '../../types'
import { pasteToResident } from '../../residents/residentPresence'
import { fetchResidents, readCachedResidents } from '../../residents/residentsApi'
import { copyAndAnnounce } from '../../utils/clipboard'
import type { MenuGroup } from '../Menu'
import MenuTarget from '../MenuTarget'
import ResidentColumn from '../ResidentColumn'
import { TableSlot } from '../TableHost'
import Rail, { RailScroll, RailSection } from '../Rail'
import {
  arrivalPages,
  fetchChanges,
  fetchGraph,
  fetchShelfPages,
  fetchShelves,
  libraryWhen,
  type LibraryGraph,
  type LibraryArrival,
  type LibraryPage,
  type LibraryShelves,
} from '../../library/libraryApi'
import './LibraryView.css'

/** How many commits the New arrivals column reads. */
const ARRIVALS = 30

/** The one line the Librarian and the drawer are handed: the dived-into page. */
export function libraryReference(path: string | null): string {
  return path ? `library ${path}` : 'library'
}

/** How far back the scrubber reaches when the corpus is younger than that. */
const LEAST_SPAN = 30 * 86_400_000

/** How long playing the brain's whole history takes. */
const PLAY_MS = 9000

/** A day as the time line names it: 22 Oct. */
function dayOf(at: number): string {
  const day = new Date(at)
  return `${day.getDate()} ${MONTHS[day.getMonth()]}`
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

function count(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`
}

export default function LibraryView({ active = true }: { active?: boolean } = {}) {
  const { openSendToSession, sessions, settings, updateSettings } = useSession()
  const { announce } = useStatus()
  const theme = useTheme()

  const [shelves, setShelves] = useState<LibraryShelves | null>(null)
  const [arrivals, setArrivals] = useState<LibraryArrival[]>([])
  const [rawGraph, setGraph] = useState<LibraryGraph | null>(null)
  // Records drawn and counted with the subject they were filed for.
  const graph = useMemo(() => rawGraph && withHomes(rawGraph), [rawGraph])
  const [graphError, setGraphError] = useState('')
  const [error, setError] = useState<string | null>(null)
  // What git said when it would not read the corpus. Without it a refused
  // repository looks exactly like a library nobody has written in.
  const [gitError, setGitError] = useState('')

  // The shelf drawn open in the rail, and its pages; null while they are
  // still being read, so an empty shelf and an unread one do not look alike.
  const [shelf, setShelf] = useState<string | null>(null)
  const [shelfPages, setShelfPages] = useState<LibraryPage[] | null>(null)
  // The page the pointer is on in the rail, which the map centres and lights,
  // and the one row opened on what it is. Both are the rail's, not a list's,
  // so a shelf and an arrival behave the same way.
  const [hoverPath, setHoverPath] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)
  // Where the scrubber stands, as a place along it rather than as a time, so
  // the control and the reading cannot drift apart. The right end is now.
  const [scrub, setScrub] = useState(SCRUB_STEPS)
  const diveRef = useRef<HTMLElement>(null)

  useEffect(() => {
    let current = true
    fetchShelves()
      .then(found => {
        if (!current) return
        setShelves(found)
        if (!found.root) return
        const pages = found.shelves.reduce((total, entry) => total + entry.pages, 0)
        announce(`Library loaded · ${pages} pages on ${found.shelves.length} shelves`, 'info')
      })
      .catch((cause: unknown) => {
        if (!current) return
        setError(cause instanceof Error ? cause.message : 'The library did not answer')
      })
    return () => { current = false }
  }, [announce])

  const root = shelves?.root ?? ''

  useEffect(() => {
    if (!root) return
    let current = true
    fetchChanges(ARRIVALS)
      .then(found => {
        if (!current) return
        setArrivals(arrivalPages(found.changes))
        setGitError(found.error ?? '')
      })
      .catch(() => { if (current) setArrivals([]) })
    return () => { current = false }
  }, [root])

  // Re-read on tab entry and explicit refresh; agents also change the corpus.
  const loadGraph = useCallback(() => {
    let current = true
    fetchGraph()
      .then(found => {
        if (!current) return
        setGraph(previous => JSON.stringify(previous) === JSON.stringify(found) ? previous : found)
        setGraphError(found.error ?? '')
      })
      .catch((cause: unknown) => {
        if (current) setGraphError(cause instanceof Error ? cause.message : 'The map did not answer')
      })
    return () => { current = false }
  }, [])

  useEffect(() => {
    if (!root || !active) return
    return loadGraph()
  }, [active, loadGraph, root])

  // Diving into a page puts the map back under it: a dive is read against the
  // map, not instead of it.
  const reader = usePageReader(announce, loadGraph, setGitError)
  const { page, open: openPage, close: closeDive } = reader
  const resolveTarget = useMemo(() => createLibraryResolver(graph?.pages ?? []), [graph])
  const resolveWikiLink = useCallback((target: string) => {
    const path = resolveTarget(target, page?.path ?? '')
    return path === undefined ? undefined : `/${path}`
  }, [resolveTarget, page?.path])

  useSurface({ open: page !== null, kind: 'work', onClose: closeDive, ref: diveRef })

  // A shelf is drawn open inside the rail, one at a time. The map stays: the
  // shelf is a way of working it, not a room that replaces it.
  const openShelf = useCallback((name: string) => {
    setShelf(current => (current === name ? null : name))
  }, [])

  // The shelf listing follows the reader, whether he chose the shelf or
  // arrived on it through a page.
  useEffect(() => {
    if (!shelf) {
      setShelfPages(null)
      return
    }
    let current = true
    setShelfPages(null)
    fetchShelfPages(shelf)
      .then(found => {
        if (!current) return
        setShelfPages(found.pages)
        setGitError(found.error ?? '')
      })
      .catch(() => { if (current) setShelfPages([]) })
    return () => { current = false }
  }, [shelf])

  // The page's Send is the drawer, for any other session; Alt+S is the
  // Librarian's, and pastes the same reference into the column at the right.
  const send = useCallback(() => {
    openSendToSession({ reference: libraryReference(page?.path ?? null) })
  }, [openSendToSession, page])

  // Send to Librarian is the column at the right where he is living in it: the
  // line lands in his prompt, unsubmitted, exactly as Alt+S puts the open page
  // there. With no column, or with his session not running, the drawer takes
  // over — targeting him where he is somewhere else, and offering to launch
  // him from the corpus root when he is nowhere. Every row's hand-off goes
  // through here, so all of them behave the same.
  const sendToLibrarian = useCallback(async (path: string) => {
    const reference = libraryReference(path)
    if (await pasteToResident(reference)) return
    // Who the Librarian is comes from /api/residents; the shelves route does
    // not name him. The column in this same tab has read it by the time a row
    // menu is open, and asking again costs nothing when it has not.
    const residents = readCachedResidents() ?? await fetchResidents().catch(() => [])
    const name = residents.find(entry => entry.tab === 'library')?.session ?? ''
    const live = name ? sessions.find(candidate => candidate.name === name) : undefined
    openSendToSession(live
      ? { targetSessionKey: getSessionKey(live.name, live.unixUser), reference }
      : { reference, launch: { label: 'Launch the Librarian', folder: root } })
  }, [openSendToSession, root, sessions])

  // The shelves the operator has left off the map, remembered per device: a
  // library he is not working in today stays out of the picture until he asks
  // for it back.
  const hidden = useMemo(() => new Set(settings.libraryHiddenShelves), [settings.libraryHiddenShelves])
  const search = useBrainSearch(announce, openPage, hidden)
  const toggleShelfOnMap = useCallback((name: string) => {
    const next = new Set(settings.libraryHiddenShelves)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    updateSettings({ libraryHiddenShelves: Array.from(next).sort() })
  }, [settings.libraryHiddenShelves, updateSettings])

  const shelfOpen = (name: string) => shelf === name
  const collapseShelf = useCallback(() => setShelf(null), [])

  const pageMenu = (path: string) => (): MenuGroup[] => [
    {
      id: 'read',
      rows: [
        { id: 'open', label: 'Open', onSelect: () => openPage(path) },
        { id: 'send', label: 'Send to Librarian', chord: 'Alt+S', onSelect: () => { void sendToLibrarian(path) } },
      ],
    },
    {
      id: 'keep',
      rows: [
        { id: 'edit', label: 'Edit', onSelect: () => openPage(path, 'edit') },
        { id: 'copy-path', label: 'Copy path', onSelect: () => { void copyAndAnnounce(path, path, announce) } },
        { id: 'history', label: 'History', onSelect: () => openPage(path, 'history') },
      ],
    },
  ]

  const shelfMenu = (name: string) => (): MenuGroup[] => [
    {
      id: 'shelf',
      rows: [
        { id: 'send', label: 'Send shelf to Librarian', chord: 'Alt+S', onSelect: () => { void sendToLibrarian(name) } },
        { id: 'collapse', label: 'Collapse', disabled: !shelfOpen(name), onSelect: collapseShelf },
      ],
    },
  ]

  // The map is always the room. A search lists its answer down the sky's left
  // edge and lights it on the sky.
  const searching = search.query.trim() !== ''
  const { results } = search
  const matches = useMemo(() => (results ? new Set(results.map(result => result.path)) : null), [results])

  // The corpus the map draws: the whole of it, less the shelves the operator
  // has hidden. They are taken out before the layout rather than left out of
  // the drawing, so what is left fills the frame instead of sitting around a
  // hole where a shelf used to be.
  const shown = useMemo(() => {
    if (!graph || hidden.size === 0) return graph
    const pages = graph.pages.filter(entry => !hidden.has(entry.shelf))
    const kept = new Set(pages.map(entry => entry.path))
    return {
      ...graph,
      pages,
      links: graph.links.filter(([from, to]) => kept.has(from) && kept.has(to)),
      tags: graph.tags.filter(([from, to]) => kept.has(from) && kept.has(to)),
    }
  }, [graph, hidden])

  /** What the map calls a page, for the trail and the two link lists. */
  const titles = useMemo(
    () => new Map(graph?.pages.map(entry => [entry.path, entry.title]) ?? []),
    [graph],
  )
  const titleOf = useCallback((path: string) => titles.get(path) ?? path, [titles])
  const summaries = useMemo(
    () => new Map(graph?.pages.map(entry => [entry.path, entry.summary ?? '']) ?? []),
    [graph],
  )
  const summaryOf = useCallback((path: string) => summaries.get(path) ?? '', [summaries])
  const subjects = useMemo(
    () => new Map(graph?.pages.map(entry => [entry.path, subjectName(subjectOf(entry))]) ?? []),
    [graph],
  )
  const whereOf = useCallback((path: string) => subjects.get(path) ?? '', [subjects])

  /** How long a page is, as the map measures it, for a row opened on it. */
  const words = useMemo(
    () => new Map(graph?.pages.map(entry => [entry.path, entry.words]) ?? []),
    [graph],
  )

  // Every page one hop away — a written link either way, or a shared tag —
  // read off the same graph the map draws, so travelling costs no request.
  const neighbours = useMemo(() => {
    if (!graph || !page) return []
    return neighboursOf(graph, page.path)
      .map(path => ({ path, title: titleOf(path) }))
      .sort((a, b) => a.title.localeCompare(b.title))
  }, [graph, page, titleOf])

  // The shelves' colours, worked out once from every shelf the library has and
  // handed to both the rail and the map, so a shelf is the same colour in the
  // list and on the drawing. The rail is then the legend, and there is no
  // legend.
  const hues = useMemo(
    () => shelfHues(shelves?.shelves.map(entry => entry.name) ?? [], theme.shelves),
    [shelves, theme.shelves],
  )

  // The moment the map is read at, and how far back the scrubber reaches: from
  // the corpus's own first page, or a month, whichever is longer. One reading
  // of the clock per graph, so nothing moves because time passed.
  const now = useMemo(() => Date.now(), [graph])
  // Every moment a page arrived at, oldest first, each once.
  const births = useMemo(
    () => [...new Set((graph?.pages ?? []).map(entry => momentOf(entry.created)).filter(at => at > 0))].sort((a, b) => a - b),
    [graph],
  )
  const span = Math.max(LEAST_SPAN, births.length > 0 ? now - births[0] : 0)
  const moment = scrub >= SCRUB_STEPS ? NOW : now - backFrom(scrub, span)
  const stops = useMemo(() => stopsOn(span), [span])
  // The time line is read in dates: where the corpus began, and the day each
  // stop falls on.
  const dates = useMemo(() => [
    { position: 0, label: dayOf(now - span) },
    ...stops.filter(stop => stop.back > 0).map(stop => ({ position: stop.position, label: dayOf(now - stop.back) })),
  ], [now, span, stops])
  const momentLegend = moment === NOW
    ? 'The corpus as it stands'
    : `The corpus ${libraryWhen(new Date(moment).toISOString(), now)}`

  // Played, the scrubber steps through the moments pages arrived at, each
  // given the same time, so a quiet month and a busy day both show their
  // arrivals instead of the busy day being a blink at the end.
  const [playing, setPlaying] = useState(false)
  useEffect(() => {
    if (!playing) return
    const start = performance.now()
    let frame = requestAnimationFrame(function step(at) {
      const t = Math.min(1, (at - start) / PLAY_MS)
      const born = births[Math.floor(t * (births.length - 1))] ?? now
      setScrub(t >= 1 ? SCRUB_STEPS : positionOf(now - born, span))
      if (t < 1) frame = requestAnimationFrame(step)
      else setPlaying(false)
    })
    return () => cancelAnimationFrame(frame)
  }, [births, now, playing, span])

  const commitRailWidth = useCallback((library: number) => {
    updateSettings({ railWidth: { ...settings.railWidth, library } })
  }, [settings.railWidth, updateSettings])

  if (error) return <div className="library-view"><p className="library-error">{error}</p></div>
  if (!shelves) return <div className="library-view"><p className="library-empty">Opening the library…</p></div>
  if (!shelves.root) return <div className="library-view"><p className="library-empty">No library is configured</p></div>

  // What the bar counts is what the map draws: the shelves that are on, at the
  // moment the scrubber stands at. The layout is not narrowed by the moment —
  // that is what lets the corpus grow in place under a drag — so the count is
  // worked out here rather than read off the layout.
  const drawnPages = (shown?.pages ?? []).filter(entry => existedAt(momentOf(entry.created), moment))
  const drawnPaths = new Set(drawnPages.map(entry => entry.path))
  const mapPages = drawnPages.filter(entry => entry.shelf !== '').length
  const mapShelves = new Set(drawnPages.filter(entry => entry.shelf !== '').map(subjectOf)).size
  const mapLinks = (shown?.links ?? []).filter(([from, to]) => drawnPaths.has(from) && drawnPaths.has(to)).length

  const mapOrWhy = () => {
    if (shown) {
      return (
        <BrainSky
          graph={shown}
          fullGraph={graph ?? shown}
          timeControl={<>
            <button type="button" className="library-map-play" aria-pressed={playing}
              aria-label={playing ? 'Stop playing the brain\'s growth' : 'Play the brain\'s growth'}
              title={playing ? 'Stop' : 'Watch the brain grow'}
              onClick={() => setPlaying(current => !current)}>{playing ? '■' : '▶'}</button>
            <div className="brain-time" title={momentLegend}>
              <input type="range" className="library-map-scrubber" data-ui="library.map.scrubber"
                min={0} max={SCRUB_STEPS} step={1} value={scrub} list="library-map-stops"
                aria-label="The moment the map is read at" aria-valuetext={momentLegend}
                onChange={event => { setPlaying(false); setScrub(snapTo(Number(event.target.value), span)) }} />
              <datalist id="library-map-stops">{stops.map(stop => <option key={stop.id} value={stop.position} label={stop.label} />)}</datalist>
              <div className="brain-time-dates" aria-hidden="true">
                {dates.map(date => <span key={date.position} style={{ left: `${date.position / SCRUB_STEPS * 100}%` }}
                  className={scrub === date.position ? 'is-here' : undefined}>{date.label}</span>)}
              </div>
            </div>
            <span className="brain-time-at">{scrub === SCRUB_STEPS ? 'Now' : libraryWhen(new Date(moment).toISOString(), now)}</span>
          </>}
          openPath={page?.path ?? null}
          matches={matches}
          hoverPath={hoverPath}
          moment={moment}
          onOpen={openPage}
        />
      )
    }
    return <p className="library-empty">{graphError || 'Drawing the map…'}</p>
  }

  /**
   * A page as the rail lists it, on a shelf or among the arrivals.
   *
   * Pointing at the row takes the map to that page and lights it, so the rail
   * is read against the map rather than instead of it. Clicking opens the row
   * on what the page is — where it sits, when it last moved, how long it is —
   * and offers the dive, which is the step that leaves the rail.
   */
  const pageRow = (path: string, title: string, updated: string, author: string) => {
    const open = expanded === path
    const length = words.get(path)
    return (
      <MenuTarget key={path} label={`Actions for ${path}`} groups={pageMenu(path)}>
        <div className={`library-row${open ? ' open' : ''}`}>
          <button
            type="button"
            className="library-row-head"
            aria-expanded={open}
            onClick={() => setExpanded(current => (current === path ? null : path))}
            onMouseEnter={() => setHoverPath(path)}
            onMouseLeave={() => setHoverPath(current => (current === path ? null : current))}
            onFocus={() => setHoverPath(path)}
            onBlur={() => setHoverPath(current => (current === path ? null : current))}
          >
            <span className="library-row-title">{title}</span>
            <span className="library-row-when">{libraryWhen(updated)}</span>
          </button>
          {open && (
            <div className="library-row-open">
              <span className="library-row-path">{path}</span>
              <span className="library-row-meta">
                {updated ? `changed ${libraryWhen(updated)} by ${author}` : 'never committed'}
                {' · '}
                {length === undefined ? 'not on the map' : count(length, 'word')}
              </span>
              <button type="button" className="library-action" onClick={() => openPage(path)}>Open</button>
            </div>
          )}
        </div>
      </MenuTarget>
    )
  }

  return (
    <div className="library-view">
      <div className="library-columns">
        <Rail
          className="library-left"
          data-ui="library.shelves"
          label="Brain"
          width={settings.railWidth.library}
          onWidthCommit={commitRailWidth}
        >

          <RailSection className="library-section library-shelves" title="Collections">
            <RailScroll className="library-scroll">
              {shelves.shelves.map(entry => (
                <div key={entry.path} className="library-shelf-group">
                  <MenuTarget label={`Actions for shelf ${entry.name}`} groups={shelfMenu(entry.name)}>
                    {/* The row is the shelf and what to do with it on the map. */}
                    <div className={`library-shelf-row${hidden.has(entry.name) ? ' off' : ''}`}>
                      <button
                        type="button"
                        className={`library-shelf ${shelf === entry.name ? 'active' : ''}`}
                        aria-expanded={shelf === entry.name}
                        onClick={() => openShelf(entry.name)}
                      >
                        <span className="library-shelf-name">{entry.name}</span>
                        <span className="library-shelf-count" style={{ color: hues.get(entry.name) }}>{entry.pages}</span>
                      </button>
                      <button
                        type="button"
                        className="library-shelf-toggle"
                        aria-pressed={hidden.has(entry.name)}
                        aria-label={`${hidden.has(entry.name) ? 'Show' : 'Hide'} ${entry.name} on the map`}
                        onClick={() => toggleShelfOnMap(entry.name)}
                      >
                        {hidden.has(entry.name) ? 'Show' : 'Hide'}
                      </button>
                    </div>
                  </MenuTarget>
                  {shelf === entry.name && (
                    <div className="library-shelf-pages">
                      {shelfPages === null && <p className="library-empty">Reading the shelf…</p>}
                      {shelfPages?.length === 0 && <p className="library-empty">This shelf is empty.</p>}
                      {shelfPages?.map(entryPage => pageRow(entryPage.path, entryPage.title, entryPage.updated, entryPage.author))}
                    </div>
                  )}
                </div>
              ))}
            </RailScroll>
          </RailSection>

          <RailSection className="library-section library-arrivals" title="New arrivals" fill>
            <RailScroll className="library-scroll">
              {gitError && <p className="library-git-error">{gitError}</p>}
              {arrivals.length === 0 && <p className="library-empty">Nothing has arrived yet.</p>}
              {arrivals.map(arrival => pageRow(arrival.path, titleOf(arrival.path), arrival.time, arrival.author))}
            </RailScroll>
          </RailSection>

        </Rail>

        <div className="library-room" data-ui="library.room">
          {/* Search along the sky's top edge; what it finds is listed beside
              the sky and lit on it. */}
          <div className="library-map-bar">
            <input
              className="library-search"
              type="search"
              value={search.query}
              placeholder="Search the brain…"
              aria-label="Search the brain"
              onChange={event => search.ask(event.target.value)}
              onKeyDown={event => { if (search.key(event.key)) event.preventDefault() }}
            />
            {shown && (
              <span className="library-map-count">
                {count(mapPages, 'page')} · {count(mapShelves, 'subject')} · {count(mapLinks, 'link')}
              </span>
            )}
          </div>
          <div className="library-room-body">
            {searching && (
              <SearchResults
                results={results}
                selected={search.selected}
                openPath={page?.path ?? null}
                listRef={search.listRef}
                menu={pageMenu}
                onOpen={search.choose}
                onHover={setHoverPath}
              />
            )}
            <div className="library-map-frame">
              {graphError && shown && <p className="library-git-error" role="alert">{graphError}</p>}
              {mapOrWhy()}
            </div>
          </div>
        </div>

        {page && (
          <PageReader
            reader={{ ...reader, page }}
            diveRef={diveRef}
            titleOf={titleOf}
            summaryOf={summaryOf}
            whereOf={whereOf}
            related={neighbours}
            resolveWikiLink={resolveWikiLink}
            onSend={send}
          />
        )}

        <TableSlot active={active} />
        <ResidentColumn active={active} tab="library" reference={libraryReference(page?.path ?? null)} />
      </div>
    </div>
  )
}
