import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LibraryView from './LibraryView'
import { resetChordsForTest } from '../keys/chords'
import { resetSurfacesForTest } from '../keys/dismiss'
import { mountResident, resetResidentForTest } from '../residents/residentPresence'
import type { Resident } from '../residents/residentsApi'
import { DEFAULT_SETTINGS } from '../types'
import type {
  LibraryChange,
  LibraryGraph,
  LibraryPage,
  LibraryPageContent,
  LibrarySearchResult,
  LibraryShelves,
} from '../library/libraryApi'

const mockState = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
  announce: vi.fn(),
  copy: vi.fn(),
  paste: vi.fn(),
  sessions: [] as { name: string; unixUser?: string }[],
  residents: [] as Resident[],
  shelves: null as LibraryShelves | null,
  shelvesError: null as Error | null,
  changes: [] as LibraryChange[],
  gitError: '',
  pages: new Map<string, LibraryPage[]>(),
  page: new Map<string, LibraryPageContent>(),
  graph: null as LibraryGraph | null,
  results: [] as LibrarySearchResult[],
  saved: [] as { path: string; content: string; summary: string }[],
  saveError: null as Error | null,
  hiddenShelves: [] as string[],
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: { ...DEFAULT_SETTINGS, libraryHiddenShelves: mockState.hiddenShelves },
    sessions: mockState.sessions,
    openSendToSession: mockState.openSendToSession,
  }),
}))

vi.mock('../utils/clipboard', () => ({
  copyAndAnnounce: (text: string, what: string, announce: unknown) => mockState.copy(text, what, announce),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../residents/residentsApi', async () => {
  const actual = await vi.importActual<typeof import('../residents/residentsApi')>('../residents/residentsApi')
  return {
    ...actual,
    readCachedResidents: () => null,
    fetchResidents: () => Promise.resolve(mockState.residents),
  }
})

vi.mock('./ResidentColumn', () => ({
  default: ({ tab, reference }: { tab: string; reference: string | null }) => (
    <div data-testid="resident" data-tab={tab} data-reference={reference ?? ''} />
  ),
}))

vi.mock('../library/libraryApi', async () => {
  const actual = await vi.importActual<typeof import('../library/libraryApi')>('../library/libraryApi')
  return {
    ...actual,
    fetchShelves: () => (mockState.shelvesError ? Promise.reject(mockState.shelvesError) : Promise.resolve(mockState.shelves)),
    fetchChanges: () => Promise.resolve({ changes: mockState.changes, error: mockState.gitError }),
    fetchShelfPages: (shelf: string) => Promise.resolve({ pages: mockState.pages.get(shelf) ?? [], error: mockState.gitError }),
    fetchGraph: () => Promise.resolve(mockState.graph),
    fetchPage: (path: string) => {
      const found = mockState.page.get(path)
      return found ? Promise.resolve(found) : Promise.reject(new Error('No such page'))
    },
    searchLibrary: () => Promise.resolve(mockState.results),
    savePage: (path: string, content: string, summary: string) => {
      if (mockState.saveError) return Promise.reject(mockState.saveError)
      mockState.saved.push({ path, content, summary })
      return Promise.resolve({ hash: 'abcdef1234', time: NOW, author: 'The Operator', message: summary })
    },
  }
})

const NOW = new Date(Date.now() - 3_600_000).toISOString()

beforeEach(() => {
  mockState.hiddenShelves = []
  // The sky draws itself at the size it is given; jsdom gives none.
  vi.stubGlobal('ResizeObserver', class {
    constructor(private readonly report: ResizeObserverCallback) {}
    observe() { this.report([{ contentRect: { width: 1200, height: 800 } } as ResizeObserverEntry], this as unknown as ResizeObserver) }
    disconnect() {}
  })
  resetChordsForTest()
  resetResidentForTest()
  resetSurfacesForTest()
  mockState.paste.mockReset()
  mockState.paste.mockResolvedValue(false)
  mockState.openSendToSession.mockReset()
  mockState.announce.mockReset()
  mockState.copy.mockReset()
  mockState.sessions = []
  mockState.residents = []
  mockState.shelvesError = null
  mockState.saveError = null
  mockState.gitError = ''
  mockState.saved = []
  mockState.shelves = {
    root: '/corpus',
    shelves: [
      { name: 'knowledge', path: 'knowledge', pages: 13 },
      { name: 'preferences', path: 'preferences', pages: 7 },
    ],
  }
  mockState.changes = [
    { hash: 'aaaaaaa1', time: NOW, author: 'The Operator', message: 'Record a workflow preference', files: ['preferences/workflow.md'] },
    { hash: 'bbbbbbb2', time: NOW, author: 'The Operator', message: 'Curate the knowledge shelf', files: ['knowledge/testing.md'] },
  ]
  mockState.pages = new Map<string, LibraryPage[]>([
    ['preferences', [
      { path: 'preferences/tools.md', title: 'Tool Preferences', updated: NOW, author: 'The Operator' },
      { path: 'preferences/workflow.md', title: 'Workflow Preferences', updated: NOW, author: 'The Operator' },
    ]],
  ])
  mockState.page = new Map<string, LibraryPageContent>([
    ['preferences/workflow.md', {
      path: 'preferences/workflow.md',
      title: 'Workflow Preferences',
      author: 'The Operator',
      updated: NOW,
      content: '# Workflow Preferences\n\nPrefer small, verifiable changes.\n',
      history: [{ hash: 'aaaaaaa1', time: NOW, author: 'The Operator', message: 'Record a workflow preference' }],
    }],
    ['knowledge/testing.md', {
      path: 'knowledge/testing.md',
      title: 'Test isolation',
      author: 'The Operator',
      updated: NOW,
      content: '# Test isolation\n\nA serious lab gets a durable path.\n',
      history: [],
    }],
    ['preferences/tools.md', {
      path: 'preferences/tools.md',
      title: 'Tool Preferences',
      author: 'The Operator',
      updated: NOW,
      content: '# Tool Preferences\n\nTools the operator reaches for.\n',
      history: [],
    }],
  ])
  mockState.graph = {
    pages: [
      { path: 'knowledge/testing.md', shelf: 'knowledge', title: 'Test isolation', words: 60, updated: NOW, created: NOW, candidate: false },
      { path: 'preferences/tools.md', shelf: 'preferences', title: 'Tool Preferences', words: 30, updated: NOW, created: NOW, candidate: false },
      { path: 'preferences/workflow.md', shelf: 'preferences', title: 'Workflow Preferences', words: 200, updated: NOW, created: NOW, candidate: false },
    ],
    links: [['knowledge/testing.md', 'preferences/workflow.md'], ['preferences/workflow.md', 'preferences/tools.md']],
    tags: [['preferences/tools.md', 'preferences/workflow.md', 'tooling']],
  }
  mockState.results = [
    { path: 'knowledge/testing.md', title: 'Test isolation', kind: 'topic', collection: 'knowledge', summary: 'Isolate tests from live data.', snippet: 'A serious lab gets a »durable« path.' },
    { path: 'preferences/workflow.md', title: 'Workflow Preferences', kind: 'note', collection: 'self', summary: 'Small, verifiable changes.', snippet: '' },
  ]
})

/**
 * A page's name appears in more than one place on purpose — in the rail, on
 * the map, in a listing — so every query says which region it means.
 */
function region(name: 'library-left' | 'library-shelves' | 'library-arrivals' | 'library-room' | 'brain-map' | 'library-dive' | 'library-search-results') {
  const found = document.querySelector<HTMLElement>(`.${name}`)
  if (!found) throw new Error(`the ${name} region is not on screen`)
  return within(found)
}

/** The rows of the menu that is open, in the order they are offered. */
const menuItems = () => screen.getAllByRole('menuitem').map(item => item.querySelector('.menu-row-label')?.textContent)

/** The Related list of the reader, whose links also name pages elsewhere. */
function related() {
  const found = document.querySelector<HTMLElement>('.library-links')
  if (!found) throw new Error('the reader lists nothing related')
  return within(found)
}

/** A star on the sky, by the name it carries. */
function star(title: string) {
  return region('brain-map').getByRole('button', { name: title })
}

/** The Brain, landed on its sky. */
async function openLibrary() {
  render(<LibraryView />)
  await screen.findByRole('region', { name: 'Brain map' })
  await screen.findByText(/^\d+ pages? · \d+ subjects? · \d+ links?$/)
}

/** Draw a shelf open in the rail. */
function openShelf(name: string) {
  fireEvent.click(region('library-left').getByRole('button', { name: new RegExp(`^${name}`) }))
}

/** A page's row where the rail lists it, by the name it carries. */
function railRow(where: 'library-shelves' | 'library-arrivals', title: string) {
  return region(where).getByRole('button', { name: new RegExp(`^${title}`) })
}

/** Open the workflow page from its star. */
async function openWorkflowPage() {
  fireEvent.click(await region('brain-map').findByRole('button', { name: 'Workflow Preferences' }))
  await screen.findByText('Prefer small, verifiable changes.')
}

function pressEscape() {
  act(() => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
  })
}

describe('LibraryView', () => {
  it('says so when no corpus is configured', async () => {
    mockState.shelves = { root: '', shelves: [] }
    render(<LibraryView />)

    expect(await screen.findByText('No library is configured')).toBeInTheDocument()
  })

  it('says what refused rather than showing a blank tab', async () => {
    mockState.shelvesError = new Error('The corpus is not readable')
    render(<LibraryView />)

    expect(await screen.findByText('The corpus is not readable')).toBeInTheDocument()
  })

  it('lands on the sky with its counts and every page a star', async () => {
    await openLibrary()
    expect(screen.getByText('3 pages · 2 subjects · 2 links')).toBeInTheDocument()
    for (const title of ['Test isolation', 'Tool Preferences', 'Workflow Preferences']) {
      expect(star(title)).toBeInTheDocument()
    }
    expect(region('brain-map').getByRole('button', { name: 'Explore preferences, 2 pages' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Overview' })).toBeNull()
  })

  it('opens a page from its star and marks the star as the one in hand', async () => {
    await openLibrary()
    await openWorkflowPage()
    expect(star('Workflow Preferences')).toHaveAttribute('aria-current', 'true')
    expect(star('Test isolation')).not.toHaveAttribute('aria-current')
  })

  it('reads a page under a one-line head: where it lives, when it changed, and what it is related to', async () => {
    await openLibrary()
    await openWorkflowPage()
    expect(document.querySelector('.library-page-meta')).toHaveTextContent('preferences')
    expect(document.querySelector('.library-page-meta')).toHaveTextContent('changed 1 hour ago by The Operator')
    expect(document.querySelector('.library-trail')).toBeNull()
    expect(document.querySelector('.library-history-strip')).toBeNull()
    expect(related().getByRole('button', { name: 'Test isolation' })).toBeInTheDocument()
    expect(related().getByRole('button', { name: 'Tool Preferences' })).toBeInTheDocument()
  })

  it('travels to a related page, shows the trail, and goes back along it', async () => {
    await openLibrary()
    await openWorkflowPage()

    const trail = () => within(document.querySelector<HTMLElement>('.library-trail')!)
    const steps = () => Array.from(document.querySelectorAll('.library-trail-step')).map(step => step.textContent)
    // One page is not yet a dive: the trail appears once there is a way back.
    expect(steps()).toEqual([])

    fireEvent.click(related().getByRole('button', { name: 'Tool Preferences' }))
    expect(await screen.findByText('Tools the operator reaches for.')).toBeInTheDocument()
    expect(steps()).toEqual(['Workflow Preferences', 'Tool Preferences'])

    // A step back is a step back: the trail is cut to it, not extended.
    fireEvent.click(trail().getByRole('button', { name: 'Workflow Preferences' }))
    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    expect(steps()).toEqual([])
  })

  it('follows prose wikilinks through the dive trail and leaves unknown targets as text', async () => {
    const workflow = mockState.page.get('preferences/workflow.md')!
    mockState.page.set(workflow.path, {
      ...workflow,
      content: `${workflow.content}\n[[chiron#Health|Chiron]] stewards this system. [[nowhere]] stays unknown. [[workflow|This page]].`,
    })
    mockState.graph!.pages.push({
      path: 'observations/chiron.md', shelf: 'observations', title: 'Chiron',
      words: 20, updated: NOW, created: NOW, candidate: false,
    })
    mockState.page.set('observations/chiron.md', {
      path: 'observations/chiron.md', title: 'Chiron', content: '# Chiron\n\nCare for the shared system.',
      updated: NOW, author: 'The Operator', history: [],
    })
    await openLibrary()
    await openWorkflowPage()

    const link = region('library-dive').getByRole('link', { name: 'Chiron' })
    expect(link).toHaveAttribute('href', '/observations/chiron.md')
    expect(region('library-dive').getByText(/\[\[nowhere\]\] stays unknown/)).toBeInTheDocument()
    expect(region('library-dive').queryByRole('link', { name: /nowhere/ })).toBeNull()
    fireEvent.click(link)
    expect(await screen.findByText('Care for the shared system.')).toBeInTheDocument()

    const trail = () => within(screen.getByRole('navigation', { name: 'This dive' }))
    expect(trail().getAllByRole('button').map(button => button.textContent))
      .toEqual(['Workflow Preferences', 'Chiron'])
    fireEvent.click(trail().getByRole('button', { name: 'Workflow Preferences' }))
    const self = await region('library-dive').findByRole('link', { name: 'This page' })
    fireEvent.click(self)
    await waitFor(() => expect(screen.queryByRole('navigation', { name: 'This dive' })).toBeNull())
  })

  it('closes the dive with Escape and leaves the map standing', async () => {
    await openLibrary()
    await openWorkflowPage()

    pressEscape()

    await waitFor(() => expect(screen.queryByText('Prefer small, verifiable changes.')).not.toBeInTheDocument())
    expect(star('Workflow Preferences')).not.toHaveAttribute('aria-current')
  })



  it('opens a shelf in the rail, opens one of its pages, and dives from it', async () => {
    await openLibrary()

    openShelf('preferences')
    expect(await region('library-shelves').findByRole('button', { name: /^Tool Preferences/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Brain map' })).toBeInTheDocument()

    fireEvent.click(railRow('library-shelves', 'Workflow Preferences'))
    expect(region('library-shelves').getByText('preferences/workflow.md')).toBeInTheDocument()
    expect(region('library-shelves').getByText('changed 1 hour ago by The Operator · 200 words')).toBeInTheDocument()

    fireEvent.click(region('library-shelves').getByRole('button', { name: 'Open' }))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    expect(star('Workflow Preferences')).toHaveAttribute('aria-current', 'true')
  })

  it('folds the history away until it is asked for, then shows every commit', async () => {
    const workflow = mockState.page.get('preferences/workflow.md')!
    mockState.page.set('preferences/workflow.md', {
      ...workflow,
      history: ['1111111', '2222222', '3333333', '4444444', '5555555'].map(hash => ({
        hash, time: NOW, author: 'The Operator', message: `Commit ${hash}`,
      })),
    })
    await openLibrary()
    await openWorkflowPage()

    expect(screen.queryByText('1111111')).not.toBeInTheDocument()
    const toggle = screen.getByRole('button', { name: 'History' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('1111111')).toBeInTheDocument()
    expect(screen.getByText('5555555')).toBeInTheDocument()
  })

  it('counts what links to a page as related, and follows one back', async () => {
    await openLibrary()
    fireEvent.click(star('Tool Preferences'))
    await screen.findByText('Tools the operator reaches for.')

    // Workflow links to Tools; nothing in Tools links back, yet it is related.
    fireEvent.click(related().getByRole('button', { name: 'Workflow Preferences' }))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
  })

  it('carries the open page as the reference to the Librarian and the drawer', async () => {
    await openLibrary()
    await openWorkflowPage()

    expect(screen.getByTestId('resident')).toHaveAttribute('data-reference', 'library preferences/workflow.md')

    fireEvent.click(screen.getByRole('button', { name: /^Send/ }))
    expect(mockState.openSendToSession).toHaveBeenCalledWith({ reference: 'library preferences/workflow.md' })
  })

  // Arrivals are the pages that changed, not the commits that changed them:
  // a page touched twice is listed once, at the newest commit that touched it.
  it('lists the pages that arrived, each once, newest first, and dives into one', async () => {
    mockState.changes = [
      { hash: 'aaaaaaa1', time: NOW, author: 'The Operator', message: 'Record a workflow preference', files: ['preferences/workflow.md', 'preferences/tools.md'] },
      { hash: 'bbbbbbb2', time: NOW, author: 'The Operator', message: 'Curate the knowledge shelf', files: ['knowledge/testing.md', 'preferences/workflow.md'] },
    ]
    await openLibrary()

    const rows = () => Array.from(
      document.querySelectorAll<HTMLElement>('.library-arrivals .library-row-title'),
    ).map(row => row.textContent)
    await waitFor(() => expect(rows()).toEqual(['Workflow Preferences', 'Tool Preferences', 'Test isolation']))

    fireEvent.click(railRow('library-arrivals', 'Workflow Preferences'))
    fireEvent.click(region('library-arrivals').getByRole('button', { name: 'Open' }))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
  })

  it('lights the star of the page under the pointer in the rail', async () => {
    await openLibrary()
    openShelf('preferences')
    fireEvent.mouseEnter(await region('library-shelves').findByRole('button', { name: /^Workflow Preferences/ }))
    expect(star('Workflow Preferences')).toHaveClass('is-hot')
    fireEvent.mouseLeave(railRow('library-shelves', 'Workflow Preferences'))
    expect(star('Workflow Preferences')).not.toHaveClass('is-hot')
  })


  // The brain ranks; the tab lists its answer beside the sky as the operator
  // types, lights it there and keeps the list while a page is read.
  it('lists the brain search beside the sky and reads results beside it', async () => {
    await openLibrary()

    const search = screen.getByRole('searchbox', { name: 'Search the brain' })
    fireEvent.change(search, { target: { value: 'durable' } })

    expect(await screen.findByText('Isolate tests from live data.')).toBeInTheDocument()
    expect(screen.getByText('durable').tagName).toBe('MARK')
    expect(region('library-search-results').getByText('Test isolation')).toBeInTheDocument()
    expect(document.querySelector('.brain-map')).toBeInTheDocument()
    expect(screen.queryByRole('group', { name: 'Collections searched' })).toBeNull()

    // Pointing at a result lights its star.
    fireEvent.mouseEnter(region('library-search-results').getByRole('button', { name: /^Workflow Preferences/ }))
    expect(star('Workflow Preferences')).toHaveClass('is-hot')

    fireEvent.keyDown(search, { key: 'ArrowDown' })
    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    expect(region('library-search-results').getByText('Test isolation')).toBeInTheDocument()

    // Escape clears the search first, and then closes the page.
    fireEvent.keyDown(search, { key: 'Escape' })
    expect(search).toHaveValue('')
    expect(document.querySelector('.library-search-results')).toBeNull()
    expect(screen.getByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    fireEvent.keyDown(search, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByText('Prefer small, verifiable changes.')).not.toBeInTheDocument())
  })

  it('leaves a collection hidden from the sky out of the search', async () => {
    mockState.hiddenShelves = ['preferences']
    await openLibrary()
    mockState.results = mockState.results.map(result => ({ ...result, collection: result.path.split('/')[0] }))
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search the brain' }), { target: { value: 'durable' } })

    expect(await region('library-search-results').findByText('Test isolation')).toBeInTheDocument()
    expect(region('library-search-results').queryByText('Workflow Preferences')).toBeNull()
    expect(region('library-search-results').getByText('1 page')).toBeInTheDocument()
  })

  it('edits a page in place and commits it with a summary', async () => {
    await openLibrary()
    await openWorkflowPage()

    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    const editor = screen.getByRole('textbox', { name: 'Editing preferences/workflow.md' })
    expect(editor).toHaveValue('# Workflow Preferences\n\nPrefer small, verifiable changes.\n')
    expect(screen.getByRole('textbox', { name: 'What this edit changes' })).toHaveValue('Edit Workflow Preferences')

    fireEvent.change(editor, { target: { value: '# Workflow Preferences\n\nCorrected.\n' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'What this edit changes' }), { target: { value: 'Correct a wording' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(mockState.saved).toEqual([{
      path: 'preferences/workflow.md',
      content: '# Workflow Preferences\n\nCorrected.\n',
      summary: 'Correct a wording',
    }]))
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      'Shelved preferences/workflow.md · Correct a wording', 'success',
    ))
    expect(await screen.findByText('Corrected.')).toBeInTheDocument()
  })

  it('says what the library refused rather than pretending the save landed', async () => {
    mockState.saveError = new Error('preferences/workflow.md already has an uncommitted change in the corpus')
    await openLibrary()
    await openWorkflowPage()

    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      'preferences/workflow.md already has an uncommitted change in the corpus', 'error',
    ))
    expect(screen.getByRole('textbox', { name: 'Editing preferences/workflow.md' })).toBeInTheDocument()
  })

  it('asks before it throws a draft away', async () => {
    await openLibrary()
    await openWorkflowPage()

    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    fireEvent.click(screen.getByRole('button', { name: 'Discard' }))
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Editing preferences/workflow.md' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
  })

  it('offers a page row the five things a page can do, and copies its path', async () => {
    await openLibrary()
    openShelf('preferences')

    fireEvent.contextMenu(await region('library-shelves').findByText('Workflow Preferences'))
    expect(menuItems()).toEqual(['Open', 'Send to Librarian', 'Edit', 'Copy path', 'History'])

    fireEvent.click(screen.getByRole('menuitem', { name: /^Copy path/ }))
    expect(mockState.copy).toHaveBeenCalledWith(
      'preferences/workflow.md', 'preferences/workflow.md', mockState.announce,
    )
  })

  // Edit and History are a state the page arrives in, not a step to take once
  // it is open: the menu asked for the page that way.
  it('opens a page from its menu already in the editor, or with its history unfolded', async () => {
    await openLibrary()
    openShelf('preferences')

    fireEvent.contextMenu(await region('library-shelves').findByText('Workflow Preferences'))
    fireEvent.click(screen.getByRole('menuitem', { name: /^Edit/ }))
    expect(await screen.findByRole('textbox', { name: 'Editing preferences/workflow.md' })).toBeInTheDocument()

    fireEvent.contextMenu(region('library-shelves').getByText('Workflow Preferences'))
    fireEvent.click(screen.getByRole('menuitem', { name: /^History/ }))
    expect(await screen.findByText('Record a workflow preference')).toBeInTheDocument()
  })

  it('hands a page to the Librarian where he is running', async () => {
    mockState.sessions = [{ name: 'librarian', unixUser: 'alice' }]
    mockState.residents = [
      { tab: 'library', label: 'Librarian', session: 'librarian', folder: '/corpus', beads: '/corpus' },
    ]
    await openLibrary()
    openShelf('preferences')

    fireEvent.contextMenu(await region('library-shelves').findByText('Workflow Preferences'))
    fireEvent.click(screen.getByRole('menuitem', { name: /^Send to Librarian/ }))

    await waitFor(() => expect(mockState.openSendToSession).toHaveBeenCalledWith({
      targetSessionKey: 'alice:librarian',
      reference: 'library preferences/workflow.md',
    }))
  })

  // With no Librarian session the hand-off is not dead: the drawer opens on
  // the launcher, at the corpus, with the page already named.
  it('offers to launch the Librarian when his session is not there', async () => {
    await openLibrary()
    openShelf('preferences')

    fireEvent.contextMenu(await region('library-shelves').findByText('Workflow Preferences'))
    fireEvent.click(screen.getByRole('menuitem', { name: /^Send to Librarian/ }))

    await waitFor(() => expect(mockState.openSendToSession).toHaveBeenCalledWith({
      reference: 'library preferences/workflow.md',
      launch: { label: 'Launch the Librarian', folder: '/corpus' },
    }))
  })

  // With the Librarian living in the column, a row's hand-off is a paste into
  // his prompt rather than a drawer over him.
  it('pastes a page and a shelf into the Librarian where his column is there', async () => {
    mountResident({ tab: 'library', focus: vi.fn(), paste: mockState.paste })
    mockState.paste.mockResolvedValue(true)
    await openLibrary()
    openShelf('preferences')

    fireEvent.contextMenu(await region('library-shelves').findByText('Workflow Preferences'))
    fireEvent.click(screen.getByRole('menuitem', { name: /^Send to Librarian/ }))
    await waitFor(() => expect(mockState.paste).toHaveBeenCalledWith('library preferences/workflow.md'))

    fireEvent.contextMenu(region('library-left').getByRole('button', { name: /^preferences/ }))
    fireEvent.click(screen.getByRole('menuitem', { name: /^Send shelf to Librarian/ }))
    await waitFor(() => expect(mockState.paste).toHaveBeenLastCalledWith('library preferences'))

    expect(mockState.openSendToSession).not.toHaveBeenCalled()
  })

  it('offers a shelf its two actions, and collapses the one it is showing', async () => {
    await openLibrary()
    const shelf = () => region('library-left').getByRole('button', { name: /^preferences/ })

    fireEvent.contextMenu(shelf())
    expect(menuItems()).toEqual(['Send shelf to Librarian', 'Collapse'])
    expect(screen.getByRole('menuitem', { name: /^Collapse/ })).toBeDisabled()
    fireEvent.keyDown(document, { key: 'Escape' })

    fireEvent.click(shelf())
    expect(await region('library-shelves').findByRole('button', { name: /^Tool Preferences/ })).toBeInTheDocument()

    fireEvent.contextMenu(shelf())
    fireEvent.click(screen.getByRole('menuitem', { name: /^Collapse/ }))
    await waitFor(() => expect(
      region('library-shelves').queryByRole('button', { name: /^Tool Preferences/ }),
    ).toBeNull())
  })
})
