/**
 * What the browser knows about the Library, and how it asks.
 *
 * One corpus, read through the server's own git: the shelves, the pages, a
 * page and its history, a search, what changed lately, and the one write the
 * operator can make from here. There is no store on this side and nothing is
 * cached beyond the request that asked for it.
 */

export interface LibraryShelf {
  name: string
  path: string
  pages: number
}

/** The shelves and the configuration the tab needs to know it has any. */
export interface LibraryShelves {
  root: string
  shelves: LibraryShelf[]
  librarianSession: string
  beadsProject: string
}

export interface LibraryPage {
  path: string
  title: string
  updated: string
  author: string
}

export interface LibraryCommit {
  hash: string
  time: string
  author: string
  message: string
}

export interface LibraryChange extends LibraryCommit {
  files: string[]
}

export interface LibraryPageContent extends LibraryPage {
  content: string
  history: LibraryCommit[]
  /** What git said when it would not read the corpus, if it would not. */
  error?: string
}

/** A shelf's pages, and why they carry no dates when git refused the corpus. */
export interface LibraryPages {
  pages: LibraryPage[]
  error?: string
}

/** What arrived lately, and why nothing did when git refused the corpus. */
export interface LibraryChanges {
  changes: LibraryChange[]
  error?: string
}

export interface LibrarySearchResult {
  path: string
  title: string
  line: number
  snippet: string
}

const API_BASE = '/api/library'

async function request<T>(path: string, params: Record<string, string> = {}, init?: RequestInit): Promise<T> {
  const url = new URL(`${API_BASE}${path}`, window.location.origin)
  Object.entries(params).forEach(([key, value]) => url.searchParams.set(key, value))
  const response = await fetch(url.toString(), {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json', ...init?.headers } : init?.headers,
    signal: AbortSignal.timeout(30000),
  })
  const body = await response.json().catch(() => null) as unknown
  if (!response.ok) {
    const error = (body as { error?: { message?: string } } | null)?.error
    throw new Error(error?.message || `The library refused the request (${response.status})`)
  }
  if (body === null) throw new Error('The library answered with something that is not JSON')
  return body as T
}

export function fetchShelves(): Promise<LibraryShelves> {
  return request<LibraryShelves>('/shelves')
}

export function fetchShelfPages(shelf: string): Promise<LibraryPages> {
  return request<LibraryPages>('/pages', { shelf })
}

export function fetchPage(path: string): Promise<LibraryPageContent> {
  return request<LibraryPageContent>('/page', { path })
}

export function searchLibrary(query: string): Promise<LibrarySearchResult[]> {
  return request<LibrarySearchResult[]>('/search', { q: query })
}

export function fetchChanges(limit = 30): Promise<LibraryChanges> {
  return request<LibraryChanges>('/changes', { limit: String(limit) })
}

/** The one write: the operator's own correction, committed as him. */
export function savePage(path: string, content: string, summary: string): Promise<LibraryCommit> {
  return request<LibraryCommit>('/page', {}, {
    method: 'PUT',
    body: JSON.stringify({ path, content, summary }),
  })
}

/** One page as the map draws it. */
export interface LibraryGraphPage {
  path: string
  /** The top-level directory, or '' for a page at the root. */
  shelf: string
  title: string
  words: number
  /** When git last touched it; '' for a page git has never seen. */
  updated: string
  /** Still a proposal rather than an accepted page. */
  candidate: boolean
}

/** The corpus as a graph: pages, the wiki links between them, the tags two pages share. */
export interface LibraryGraph {
  pages: LibraryGraphPage[]
  /** [from, to] page paths. */
  links: [string, string][]
  /** [from, to, tag]. */
  tags: [string, string, string][]
  error?: string
}

export function fetchGraph(): Promise<LibraryGraph> {
  return request<LibraryGraph>('/graph')
}

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
const MONTH = 30 * DAY
const YEAR = 365 * DAY

/**
 * When something happened, in the words a reader uses. A library reads by age,
 * not by timestamp: what matters is whether a page moved this morning or last
 * spring.
 */
export function libraryWhen(iso: string, now = Date.now()): string {
  if (!iso) return 'never'
  const at = Date.parse(iso)
  if (Number.isNaN(at)) return 'never'
  const elapsed = Math.max(0, now - at)
  if (elapsed < MINUTE) return 'just now'
  const say = (value: number, unit: string) => `${value} ${unit}${value === 1 ? '' : 's'} ago`
  if (elapsed < HOUR) return say(Math.floor(elapsed / MINUTE), 'minute')
  if (elapsed < DAY) return say(Math.floor(elapsed / HOUR), 'hour')
  if (elapsed < MONTH) return say(Math.floor(elapsed / DAY), 'day')
  if (elapsed < YEAR) return say(Math.floor(elapsed / MONTH), 'month')
  return say(Math.floor(elapsed / YEAR), 'year')
}

/** The shelf a page sits on: the first segment of its corpus-relative path. */
export function shelfOf(path: string): string {
  const [first] = path.split('/')
  return path.includes('/') ? first : ''
}

/**
 * A page's prose, without the heading the dive already shows above it.
 *
 * Every page in the corpus opens with its own title, and the dive's running
 * head carries that title over the path and the date. Printing it twice, once
 * in each size, makes the page look like it starts over.
 */
export function libraryProse(content: string, title: string): string {
  const lines = content.split('\n')
  const first = lines.findIndex(line => line.trim() !== '')
  if (first < 0) return content
  const heading = /^#\s+(.*)$/.exec(lines[first].trim())
  if (!heading || heading[1].trim() !== title.trim()) return content
  return lines.slice(first + 1).join('\n').replace(/^\n+/, '')
}
