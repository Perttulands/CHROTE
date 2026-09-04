/**
 * The Library: the map of the operator's own context corpus, dived into.
 *
 * A rail, the map, and the Librarian. The shelves, what arrived lately and the
 * proposals in flight down the left; the map of the whole corpus in the middle,
 * always; the Librarian live in a column at the far edge. Choosing a page dives
 * into it rather than leaving the map: the map takes that page to the middle
 * and closer in, and the page opens in a column beside it with its neighbours
 * as links to travel by and a trail back to where the dive began. Escape ends
 * the dive and the map is exactly where it was left.
 *
 * The corpus is a Markdown tree under git, so every fact here — a title, a
 * date, an author, a history, a link — is read out of that tree rather than
 * kept anywhere in CHROTE. The one thing written back is the operator's own
 * correction, and it is a commit signed as him.
 */

import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Editor from '../Editor'
import Markdown from '../Markdown'
import LibraryMap from './Map'
import { RECENCY_WINDOWS, neighboursOf, type RecencyWindow } from './mapLayout'
import Shelf from './Shelf'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { openBeadCard } from '../../beads/beadCard'
import { fetchBeadWork, type BeadRow } from '../../beads/beadsApi'
import { isBeadClosed } from '../../beads/beadStatus'
import { useSurface } from '../../keys/dismiss'
import { getSessionKey } from '../../types'
import { pasteToResident } from '../../residents/residentPresence'
import { copyAndAnnounce } from '../../utils/clipboard'
import type { MenuGroup } from '../Menu'
import MenuTarget from '../MenuTarget'
import ResidentColumn from '../ResidentColumn'
import TableColumn from '../TableColumn'
import {
  fetchChanges,
  fetchGraph,
  fetchPage,
  fetchShelfPages,
  fetchShelves,
  libraryProse,
  libraryWhen,
  savePage,
  searchLibrary,
  type LibraryChange,
  type LibraryGraph,
  type LibraryPage,
  type LibraryPageContent,
  type LibrarySearchResult,
  type LibraryShelves,
} from '../../library/libraryApi'
import './LibraryView.css'

/** How many commits the New arrivals column reads. */
const ARRIVALS = 30

/** How many commits the history strip shows before "… n more". */
const HISTORY_STRIP = 3

/** The one line the Librarian and the drawer are handed: the dived-into page. */
export function libraryReference(path: string | null): string {
  return path ? `library ${path}` : 'library'
}

/** The one line under the window control that says what the dimming means. */
function windowLegend(window: RecencyWindow): string {
  if (window === 'all') return 'Every page, faded as it ages'
  const entry = RECENCY_WINDOWS.find(candidate => candidate.id === window)
  return `Dimmed: not changed in the last ${entry?.days === 1 ? 'day' : `${entry?.days} days`}`
}

function count(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`
}

/** What a page is opened for: reading, editing, or its history unfolded. */
type OpenIntent = 'read' | 'edit' | 'history'

export default function LibraryView({ active = true }: { active?: boolean } = {}) {
  const { openSendToSession, sessions } = useSession()
  const { announce } = useStatus()

  const [shelves, setShelves] = useState<LibraryShelves | null>(null)
  const [changes, setChanges] = useState<LibraryChange[]>([])
  const [proposals, setProposals] = useState<BeadRow[]>([])
  const [graph, setGraph] = useState<LibraryGraph | null>(null)
  const [graphError, setGraphError] = useState('')
  const [error, setError] = useState<string | null>(null)
  // What git said when it would not read the corpus. Without it a refused
  // repository looks exactly like a library nobody has written in.
  const [gitError, setGitError] = useState('')

  const [query, setQuery] = useState('')
  const [results, setResults] = useState<LibrarySearchResult[] | null>(null)
  const [shelf, setShelf] = useState<string | null>(null)
  const [shelfPages, setShelfPages] = useState<LibraryPage[]>([])
  const [page, setPage] = useState<LibraryPageContent | null>(null)
  // Every page this dive has passed through, oldest first; the last of them is
  // the page being read. Empty means no dive is open.
  const [trail, setTrail] = useState<string[]>([])
  const [recency, setRecency] = useState<RecencyWindow>('all')
  const [historyOpen, setHistoryOpen] = useState(false)
  const diveRef = useRef<HTMLElement>(null)

  const [draft, setDraft] = useState<string | null>(null)
  const [summary, setSummary] = useState('')
  const [discarding, setDiscarding] = useState(false)

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
        setChanges(found.changes)
        setGitError(found.error ?? '')
      })
      .catch(() => { if (current) setChanges([]) })
    return () => { current = false }
  }, [root])

  // The map is read once, and again after the operator's own save, which is
  // the only way the corpus changes from here.
  const loadGraph = useCallback(() => {
    let current = true
    fetchGraph()
      .then(found => {
        if (!current) return
        setGraph(found)
        setGraphError('')
      })
      .catch((cause: unknown) => {
        if (current) setGraphError(cause instanceof Error ? cause.message : 'The map did not answer')
      })
    return () => { current = false }
  }, [])

  useEffect(() => {
    if (!root) return
    return loadGraph()
  }, [loadGraph, root])

  // Proposals are open Beads of the configured store. Without one the shelf
  // does not exist, rather than standing empty.
  const beadsProject = shelves?.beadsProject ?? ''
  useEffect(() => {
    if (!beadsProject) return
    let current = true
    fetchBeadWork(beadsProject)
      .then(work => { if (current) setProposals(work.beads.filter(bead => !isBeadClosed(bead.status))) })
      .catch(() => { if (current) setProposals([]) })
    return () => { current = false }
  }, [beadsProject])

  // Diving into a page puts the map back under it — a dive is read against the
  // map, not instead of it — and adds the page to the trail. A page the dive
  // has already passed through is not visited twice: the trail is cut back to
  // it, which is what a step back means.
  //
  // A row's menu can ask for the page already in the editor or with its whole
  // history unfolded; the page arrives in that state rather than a step short.
  const openPage = useCallback((path: string, intent: OpenIntent = 'read') => {
    setResults(null)
    setShelf(null)
    setDraft(null)
    fetchPage(path)
      .then(found => {
        setPage(found)
        setTrail(current => {
          const at = current.indexOf(found.path)
          return at === -1 ? [...current, found.path] : current.slice(0, at + 1)
        })
        setHistoryOpen(intent === 'history')
        setGitError(found.error ?? '')
        if (intent === 'edit') {
          setDraft(found.content)
          setSummary(`Edit ${found.title}`)
          setDiscarding(false)
        }
      })
      .catch((cause: unknown) => {
        announce(cause instanceof Error ? cause.message : `Could not open ${path}`, 'error')
      })
  }, [announce])

  // A dive ends where it was opened: the page and its trail go, the map stays
  // exactly where the operator left it.
  const closeDive = useCallback(() => {
    setPage(null)
    setTrail([])
    setDraft(null)
    setHistoryOpen(false)
    setDiscarding(false)
  }, [])

  useSurface({ open: page !== null, kind: 'work', onClose: closeDive, ref: diveRef })

  const openShelf = useCallback((name: string) => {
    setResults(null)
    setDraft(null)
    setShelf(name)
    closeDive()
  }, [closeDive])

  // The shelf listing follows the reader, whether he chose the shelf or
  // arrived on it through a page.
  useEffect(() => {
    if (!shelf) {
      setShelfPages([])
      return
    }
    let current = true
    fetchShelfPages(shelf)
      .then(found => {
        if (!current) return
        setShelfPages(found.pages)
        setGitError(found.error ?? '')
      })
      .catch(() => { if (current) setShelfPages([]) })
    return () => { current = false }
  }, [shelf])

  const runSearch = useCallback(() => {
    const asked = query.trim()
    if (!asked) {
      setResults(null)
      return
    }
    searchLibrary(asked)
      .then(found => {
        setResults(found)
        setShelf(null)
        closeDive()
        announce(`${found.length} ${found.length === 1 ? 'page mentions' : 'pages mention'} "${asked}"`, 'info')
      })
      .catch((cause: unknown) => {
        announce(cause instanceof Error ? cause.message : 'The search failed', 'error')
      })
  }, [announce, closeDive, query])

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
    const name = shelves?.librarianSession ?? ''
    const live = name ? sessions.find(candidate => candidate.name === name) : undefined
    openSendToSession(live
      ? { targetSessionKey: getSessionKey(live.name, live.unixUser), reference }
      : { reference, launch: { label: 'Launch the Librarian', folder: root } })
  }, [openSendToSession, root, sessions, shelves?.librarianSession])

  // A shelf is open while its listing is the room: not while a search stands
  // in the room's place.
  const shelfOpen = (name: string) => shelf === name && results === null
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

  // The map is the room, unless a search or a shelf listing is standing in it.
  // A dive never displaces it: the page is read in the column beside it.
  const showMap = results === null && shelf === null

  const save = useCallback(async () => {
    if (!page || draft === null) return
    const message = summary.trim() || `Edit ${page.title}`
    try {
      const commit = await savePage(page.path, draft, message)
      setDraft(null)
      setPage({ ...page, content: draft, updated: commit.time, author: commit.author, history: [commit, ...page.history] })
      announce(`Shelved ${page.path} · ${commit.message}`, 'success')
      loadGraph()
    } catch (cause: unknown) {
      announce(cause instanceof Error ? cause.message : 'The library refused the save', 'error')
    }
  }, [announce, draft, loadGraph, page, summary])

  const startEdit = useCallback(() => {
    if (!page) return
    setDraft(page.content)
    setSummary(`Edit ${page.title}`)
    setDiscarding(false)
  }, [page])

  // Discarding an edit asks in the control that does it: a second press within
  // the operator's own attention span throws the draft away.
  const discard = useCallback(() => {
    if (!discarding) {
      setDiscarding(true)
      window.setTimeout(() => setDiscarding(false), 3000)
      return
    }
    setDraft(null)
    setDiscarding(false)
  }, [discarding])

  // What the map lights: the pages a search found, or, while the operator is
  // still typing, the pages whose name holds what he has typed so far.
  const matches = useMemo(() => {
    if (results !== null) return new Set(results.map(result => result.path))
    const asked = query.trim().toLowerCase()
    if (!asked || !graph) return null
    return new Set(graph.pages
      .filter(entry => entry.title.toLowerCase().includes(asked) || entry.path.toLowerCase().includes(asked))
      .map(entry => entry.path))
  }, [graph, query, results])

  /** What the map calls a page, for the trail and the two link lists. */
  const titles = useMemo(
    () => new Map(graph?.pages.map(entry => [entry.path, entry.title]) ?? []),
    [graph],
  )
  const titleOf = useCallback((path: string) => titles.get(path) ?? path, [titles])

  // Every page one hop away — a written link either way, or a shared tag —
  // read off the same graph the map draws, so travelling costs no request.
  const neighbours = useMemo(() => {
    if (!graph || !page) return []
    return neighboursOf(graph, page.path)
      .map(path => ({ path, title: titleOf(path) }))
      .sort((a, b) => a.title.localeCompare(b.title))
  }, [graph, page, titleOf])

  // The pages that link to the open one, by title, from the same graph the
  // map draws; a backlink is a written link, not a shared tag.
  const backlinks = useMemo(() => {
    if (!graph || !page) return []
    return graph.links
      .filter(([, to]) => to === page.path)
      .map(([from]) => ({ path: from, title: titleOf(from) }))
      .sort((a, b) => a.title.localeCompare(b.title))
  }, [graph, page, titleOf])

  if (error) return <div className="library-view"><p className="library-error">{error}</p></div>
  if (!shelves) return <div className="library-view"><p className="library-empty">Opening the library…</p></div>
  if (!shelves.root) return <div className="library-view"><p className="library-empty">No library is configured</p></div>

  const mapPages = graph ? graph.pages.filter(entry => entry.shelf !== '').length : 0
  const mapShelves = graph ? new Set(graph.pages.filter(entry => entry.shelf !== '').map(entry => entry.shelf)).size : 0

  const mapOrWhy = () => {
    if (graph) {
      return (
        <LibraryMap
          graph={graph}
          openPath={page?.path ?? null}
          matches={matches}
          window={recency}
          onOpen={openPage}
        />
      )
    }
    return <p className="library-empty">{graphError || 'Drawing the map…'}</p>
  }

  /** A list of pages as links that travel, the way both link lists read. */
  const linkList = (heading: string, links: { path: string; title: string }[]) => (
    <div className="library-links">
      <h2>{heading}</h2>
      <p>
        {links.map((link, index) => (
          <Fragment key={link.path}>
            {index > 0 && <span className="library-linked-sep"> · </span>}
            <button type="button" className="library-link" onClick={() => openPage(link.path)}>{link.title}</button>
          </Fragment>
        ))}
      </p>
    </div>
  )

  return (
    <div className="library-view">
      <div className="library-columns">
        <div className="library-left" data-ui="library.shelves">
          <input
            className="library-search"
            type="search"
            value={query}
            placeholder="Search the library…"
            aria-label="Search the library"
            onChange={event => {
              setQuery(event.target.value)
              // An emptied field puts the map back: with the results standing
              // in the room, there is otherwise no way out of them.
              if (!event.target.value.trim()) setResults(null)
            }}
            onKeyDown={event => { if (event.key === 'Enter') runSearch() }}
          />

          <section className="library-section library-shelves">
            <h3>Shelves</h3>
            {shelves.shelves.map(entry => (
              <MenuTarget key={entry.path} label={`Actions for shelf ${entry.name}`} groups={shelfMenu(entry.name)}>
                <button
                  type="button"
                  className={`library-shelf ${shelf === entry.name ? 'active' : ''}`}
                  onClick={() => openShelf(entry.name)}
                >
                  <span className="library-shelf-name">{entry.name}</span>
                  <span className="library-shelf-count">{entry.pages}</span>
                </button>
              </MenuTarget>
            ))}
          </section>

          <section className="library-section library-arrivals">
            <h3>New arrivals</h3>
            <div className="library-scroll">
              {gitError && <p className="library-git-error">{gitError}</p>}
              {changes.length === 0 && <p className="library-empty">Nothing has arrived yet.</p>}
              {changes.map(change => {
                const path = change.files[0]
                const arrival = (
                  <button
                    type="button"
                    className="library-arrival"
                    disabled={!path}
                    onClick={() => { if (path) openPage(path) }}
                  >
                    <span className="library-arrival-when">{libraryWhen(change.time)} · {change.author}</span>
                    <span className="library-arrival-message">{change.message}</span>
                  </button>
                )
                // An arrival that touched no page is a fact with nothing to act on.
                return path
                  ? <MenuTarget key={change.hash} label={`Actions for ${path}`} groups={pageMenu(path)}>{arrival}</MenuTarget>
                  : <Fragment key={change.hash}>{arrival}</Fragment>
              })}
            </div>
          </section>

          {shelves.beadsProject && (
            <section className="library-section library-proposals">
              <h3>Proposals</h3>
              <div className="library-scroll">
                {proposals.length === 0 && <p className="library-empty">Nothing is in flight.</p>}
                {proposals.map(bead => (
                  <button
                    key={bead.id}
                    type="button"
                    className="library-proposal"
                    onClick={() => openBeadCard(bead.id, shelves.beadsProject)}
                  >
                    <span className="library-proposal-id">{bead.id}</span>
                    <span className="library-proposal-title">{bead.title}</span>
                  </button>
                ))}
              </div>
            </section>
          )}
        </div>

        <div className="library-room" data-ui="library.room">
          {showMap ? (
            <div className="library-map-frame">
              <div className="library-map-bar">
                <span className="library-map-title">The map</span>
                {graph && (
                  <span className="library-map-count">
                    {count(mapPages, 'page')} · {count(mapShelves, 'shelf', 'shelves')} · {count(graph.links.length, 'link')} · {count(graph.tags.length, 'shared tag')}
                  </span>
                )}
                <span className="library-map-windows" role="group" aria-label="The recency window">
                  {RECENCY_WINDOWS.map(entry => (
                    <button
                      key={entry.id}
                      type="button"
                      className={`library-map-window${entry.id === recency ? ' on' : ''}`}
                      aria-pressed={entry.id === recency}
                      onClick={() => setRecency(entry.id)}
                    >
                      {entry.label}
                    </button>
                  ))}
                </span>
                <span className="library-map-legend">{windowLegend(recency)}</span>
              </div>
              {mapOrWhy()}
            </div>
          ) : results !== null ? (
            <div className="library-page">
              <div className="library-page-head">
                <div className="library-page-title-row">
                  <h1>Search</h1>
                </div>
                <p className="library-page-meta">
                  {results.length} {results.length === 1 ? 'page mentions' : 'pages mention'} “{query.trim()}”
                </p>
                <div className="library-rule" />
              </div>
              <div className="library-results">
                {results.map(result => (
                  <MenuTarget key={result.path} label={`Actions for ${result.path}`} groups={pageMenu(result.path)}>
                    <button type="button" className="library-result" onClick={() => openPage(result.path)}>
                      <span className="library-result-title">{result.title}</span>
                      <span className="library-result-path">{result.path}{result.line > 0 ? `:${result.line}` : ''}</span>
                      {result.snippet && <span className="library-result-snippet">{result.snippet}</span>}
                    </button>
                  </MenuTarget>
                ))}
              </div>
            </div>
          ) : (
            <Shelf shelf={shelf ?? ''} pages={shelfPages} onOpenPage={openPage} pageMenu={pageMenu} />
          )}
        </div>

        {page && (
          <aside className="library-dive" data-ui="library.dive" ref={diveRef} aria-label={`Reading ${page.title}`}>
            {/* Where this dive has been. The last step is the page in hand;
                every earlier one is the way back to it. */}
            <nav className="library-trail" aria-label="This dive">
              {trail.map((path, index) => (
                <Fragment key={path}>
                  {index > 0 && <span className="library-trail-sep">›</span>}
                  <button
                    type="button"
                    className="library-trail-step"
                    aria-current={path === page.path ? 'page' : undefined}
                    onClick={() => openPage(path)}
                  >
                    {titleOf(path)}
                  </button>
                </Fragment>
              ))}
              <button type="button" className="library-action library-dive-close" onClick={closeDive}>
                Close<span className="library-chord">Esc</span>
              </button>
            </nav>
            <div className="library-page">
              <div className="library-page-head">
                <div className="library-page-title-row">
                  <h1>{page.title}</h1>
                  <div className="library-page-actions">
                    {draft === null ? (
                      <button type="button" className="library-action" onClick={startEdit}>Edit</button>
                    ) : (
                      <>
                        <button type="button" className="library-action library-action-primary" onClick={() => void save()}>Save</button>
                        <button type="button" className="library-action" onClick={discard}>
                          {discarding ? 'Confirm' : 'Discard'}
                        </button>
                      </>
                    )}
                    <button type="button" className="library-action" onClick={send}>
                      Send
                    </button>
                  </div>
                </div>
                <p className="library-page-meta">
                  {page.path}
                  {page.updated ? ` · changed ${libraryWhen(page.updated)} by ${page.author}` : ' · never committed'}
                </p>
                <div className="library-history-strip">
                  {page.history.length === 0 && <span className="library-history-none">This page is not committed yet.</span>}
                  {(historyOpen ? page.history : page.history.slice(0, HISTORY_STRIP)).map(entry => (
                    <span key={entry.hash} className="library-history">
                      <span className="library-history-hash">{entry.hash.slice(0, 7)}</span>
                      <span className="library-history-when">{libraryWhen(entry.time)}</span>
                      <span className="library-history-message">{entry.message}</span>
                    </span>
                  ))}
                  {!historyOpen && page.history.length > HISTORY_STRIP && (
                    <button type="button" className="library-history-more" onClick={() => setHistoryOpen(true)}>
                      … {page.history.length - HISTORY_STRIP} more
                    </button>
                  )}
                </div>
                <div className="library-rule" />
              </div>
              {draft === null ? (
                <div className="library-body">
                  <Markdown
                    content={libraryProse(page.content, page.title)}
                    basePath={`/${page.path}`}
                    onOpenPath={path => openPage(path.replace(/^\//, ''))}
                  />
                  {neighbours.length > 0 && linkList('Neighbours', neighbours)}
                  {backlinks.length > 0 && linkList('Linked from', backlinks)}
                </div>
              ) : (
                <div className="library-editor">
                  <input
                    className="library-summary"
                    type="text"
                    value={summary}
                    aria-label="What this edit changes"
                    onChange={event => setSummary(event.target.value)}
                  />
                  <Editor
                    value={draft}
                    onChange={setDraft}
                    onSave={() => void save()}
                    onCancel={discard}
                    label={`Editing ${page.path}`}
                    autoFocus
                  />
                </div>
              )}
            </div>
          </aside>
        )}

        <TableColumn />
        <ResidentColumn active={active} tab="library" reference={libraryReference(page?.path ?? null)} />
      </div>
    </div>
  )
}
