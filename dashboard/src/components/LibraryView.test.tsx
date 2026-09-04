import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LibraryView from './LibraryView'
import { resetChordsForTest } from '../keys/chords'
import { resetSurfacesForTest } from '../keys/dismiss'
import { mountResident, resetResidentForTest } from '../residents/residentPresence'
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
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: DEFAULT_SETTINGS,
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
  resetChordsForTest()
  resetResidentForTest()
  resetSurfacesForTest()
  mockState.paste.mockReset()
  mockState.paste.mockResolvedValue(false)
  mockState.openSendToSession.mockReset()
  mockState.announce.mockReset()
  mockState.copy.mockReset()
  mockState.sessions = []
  mockState.shelvesError = null
  mockState.saveError = null
  mockState.gitError = ''
  mockState.saved = []
  mockState.shelves = {
    root: '/corpus',
    librarianSession: 'librarian',
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
      { path: 'knowledge/testing.md', shelf: 'knowledge', title: 'Test isolation', words: 60, updated: NOW, candidate: false },
      { path: 'preferences/tools.md', shelf: 'preferences', title: 'Tool Preferences', words: 30, updated: NOW, candidate: false },
      { path: 'preferences/workflow.md', shelf: 'preferences', title: 'Workflow Preferences', words: 200, updated: NOW, candidate: false },
    ],
    links: [['knowledge/testing.md', 'preferences/workflow.md'], ['preferences/workflow.md', 'preferences/tools.md']],
    tags: [['preferences/tools.md', 'preferences/workflow.md', 'tooling']],
  }
  mockState.results = [
    { path: 'knowledge/testing.md', title: 'Test isolation', line: 3, snippet: 'A serious lab gets a durable path.' },
  ]
})

/**
 * A page's name appears in more than one place on purpose — in the rail, on
 * the map, in a listing — so every query says which region it means.
 */
function region(name: 'library-left' | 'library-shelves' | 'library-arrivals' | 'library-room' | 'library-map' | 'library-dive') {
  const found = document.querySelector<HTMLElement>(`.${name}`)
  if (!found) throw new Error(`the ${name} region is not on screen`)
  return within(found)
}

/** The rows of the menu that is open, in the order they are offered. */
const menuItems = () => screen.getAllByRole('menuitem').map(item => item.querySelector('.menu-row-label')?.textContent)

/** The Neighbours list of the dive, whose links also name pages elsewhere. */
function neighbours() {
  const found = document.querySelector<HTMLElement>('.library-links')
  if (!found) throw new Error('the dive lists no neighbours')
  return within(found)
}

/** A page on the map, by the name it carries. */
function mapNode(title: string) {
  return region('library-map').getByRole('button', { name: title })
}

/** The library, landed on its map. */
async function openLibrary() {
  render(<LibraryView />)
  await screen.findByText('The map')
  await waitFor(() => region('library-map'))
}

/** Draw a shelf open in the rail. */
function openShelf(name: string) {
  fireEvent.click(region('library-left').getByRole('button', { name: new RegExp(`^${name}`) }))
}

/** A page's row where the rail lists it, by the name it carries. */
function railRow(where: 'library-shelves' | 'library-arrivals', title: string) {
  return region(where).getByRole('button', { name: new RegExp(`^${title}`) })
}

/** Open a shelf, open one of its pages, and dive into it. */
async function openWorkflowPage() {
  openShelf('preferences')
  fireEvent.click(await region('library-shelves').findByRole('button', { name: /^Workflow Preferences/ }))
  fireEvent.click(region('library-shelves').getByRole('button', { name: 'Dive' }))
  await screen.findByText('Prefer small, verifiable changes.')
}

function pressEscape() {
  act(() => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
  })
}

describe('LibraryView', () => {
  it('says so when no corpus is configured', async () => {
    mockState.shelves = { root: '', shelves: [], librarianSession: '' }
    render(<LibraryView />)

    expect(await screen.findByText('No library is configured')).toBeInTheDocument()
  })

  it('says what refused rather than showing a blank tab', async () => {
    mockState.shelvesError = new Error('The corpus is not readable')
    render(<LibraryView />)

    expect(await screen.findByText('The corpus is not readable')).toBeInTheDocument()
  })

  it('lands on the map with the shelves labelled and its links counted', async () => {
    await openLibrary()

    expect(screen.getByText('3 pages · 2 shelves · 2 links · 1 shared tag')).toBeInTheDocument()
    expect(region('library-map').getByText('knowledge · 1')).toBeInTheDocument()
    expect(region('library-map').getByText('preferences · 2')).toBeInTheDocument()
    expect(mapNode('Workflow Preferences')).toBeInTheDocument()
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith('Library loaded · 20 pages on 2 shelves', 'info'))
  })

  it('lights a page and its neighbours under the pointer', async () => {
    await openLibrary()

    fireEvent.mouseEnter(mapNode('Test isolation'))

    // The lit dot is drawn by class; there is no other way to read it off the
    // SVG, and the class is what the stylesheet colours.
    expect(mapNode('Test isolation')).toHaveClass('hot')
    expect(mapNode('Workflow Preferences')).toHaveClass('hot')
    expect(mapNode('Tool Preferences')).not.toHaveClass('hot')
  })

  it('dives into a page from the map, keeping the map beside it', async () => {
    await openLibrary()

    fireEvent.click(mapNode('Workflow Preferences'))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    // The map is still there, with the dived-into page and its neighbours lit.
    expect(mapNode('Workflow Preferences')).toHaveClass('hot')
    expect(neighbours().getByRole('button', { name: 'Test isolation' })).toBeInTheDocument()
    expect(neighbours().getByRole('button', { name: 'Tool Preferences' })).toBeInTheDocument()
  })

  it('travels to a neighbour, grows the trail, and goes back along it', async () => {
    await openLibrary()
    fireEvent.click(mapNode('Workflow Preferences'))
    await screen.findByText('Prefer small, verifiable changes.')

    const trail = () => within(document.querySelector<HTMLElement>('.library-trail')!)
    const steps = () => Array.from(document.querySelectorAll('.library-trail-step')).map(step => step.textContent)
    expect(steps()).toEqual(['Workflow Preferences'])

    fireEvent.click(neighbours().getByRole('button', { name: 'Tool Preferences' }))
    expect(await screen.findByText('Tools the operator reaches for.')).toBeInTheDocument()
    expect(steps()).toEqual(['Workflow Preferences', 'Tool Preferences'])

    // A step back is a step back: the trail is cut to it, not extended.
    fireEvent.click(trail().getByRole('button', { name: 'Workflow Preferences' }))
    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    expect(steps()).toEqual(['Workflow Preferences'])
  })

  it('closes the dive with Escape and leaves the map standing', async () => {
    await openLibrary()
    fireEvent.click(mapNode('Workflow Preferences'))
    await screen.findByText('Prefer small, verifiable changes.')

    pressEscape()

    await waitFor(() => expect(screen.queryByText('Prefer small, verifiable changes.')).not.toBeInTheDocument())
    expect(document.querySelector('.library-trail')).toBeNull()
    expect(mapNode('Workflow Preferences')).toBeInTheDocument()
  })

  it('lights the pages whose names hold what is being typed', async () => {
    await openLibrary()

    fireEvent.change(screen.getByRole('searchbox', { name: 'Search the library' }), { target: { value: 'isolation' } })

    expect(mapNode('Test isolation')).toHaveClass('hot')
    expect(mapNode('Tool Preferences')).not.toHaveClass('hot')
  })

  // A shelf is worked inside the rail: it draws open where it stands, the map
  // is still the room behind it, and the dive is a step the row offers.
  it('opens a shelf in the rail, opens one of its pages, and dives from it', async () => {
    await openLibrary()

    openShelf('preferences')
    expect(await region('library-shelves').findByRole('button', { name: /^Tool Preferences/ })).toBeInTheDocument()
    expect(region('library-map')).toBeTruthy()

    fireEvent.click(railRow('library-shelves', 'Workflow Preferences'))
    expect(region('library-shelves').getByText('preferences/workflow.md')).toBeInTheDocument()
    expect(region('library-shelves').getByText('changed 1 hour ago by The Operator · 200 words')).toBeInTheDocument()

    fireEvent.click(region('library-shelves').getByRole('button', { name: 'Dive' }))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    expect(screen.getByText('preferences/workflow.md · changed 1 hour ago by The Operator')).toBeInTheDocument()
    const history = document.querySelector<HTMLElement>('.library-history-strip')
    expect(history).toHaveTextContent('aaaaaaa')
    expect(history).toHaveTextContent('Record a workflow preference')
  })

  it('shows three commits and expands to the rest', async () => {
    const workflow = mockState.page.get('preferences/workflow.md')!
    mockState.page.set('preferences/workflow.md', {
      ...workflow,
      history: ['1111111', '2222222', '3333333', '4444444', '5555555'].map(hash => ({
        hash, time: NOW, author: 'The Operator', message: `Commit ${hash}`,
      })),
    })
    await openLibrary()
    await openWorkflowPage()

    expect(screen.getByText('3333333')).toBeInTheDocument()
    expect(screen.queryByText('4444444')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '… 2 more' }))

    expect(screen.getByText('5555555')).toBeInTheDocument()
  })

  it('lists what this page touches and what touches it, and follows one back', async () => {
    await openLibrary()
    await openWorkflowPage()

    const lists = Array.from(document.querySelectorAll<HTMLElement>('.library-links'))
    expect(lists.map(list => list.querySelector('h2')?.textContent)).toEqual(['Neighbours', 'Linked from'])
    fireEvent.click(within(lists[1]).getByRole('button', { name: 'Test isolation' }))

    expect(await screen.findByText('A serious lab gets a durable path.')).toBeInTheDocument()
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
    fireEvent.click(region('library-arrivals').getByRole('button', { name: 'Dive' }))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
  })

  // The one channel from the rail to the map: the row under the pointer is the
  // page the map lights and brings to the middle.
  it('lights and centres the map on the page under the pointer in the rail', async () => {
    await openLibrary()
    const drawing = () => document.querySelector('.library-map svg g')?.getAttribute('transform')
    const before = drawing()

    openShelf('preferences')
    fireEvent.mouseEnter(await region('library-shelves').findByRole('button', { name: /^Workflow Preferences/ }))

    expect(mapNode('Workflow Preferences')).toHaveClass('hot')
    expect(mapNode('Test isolation')).toHaveClass('hot')
    expect(drawing()).not.toEqual(before)

    fireEvent.mouseLeave(railRow('library-shelves', 'Workflow Preferences'))
    expect(mapNode('Workflow Preferences')).not.toHaveClass('hot')
  })

  it('searches the whole corpus and opens a result', async () => {
    await openLibrary()

    const search = screen.getByRole('searchbox', { name: 'Search the library' })
    fireEvent.change(search, { target: { value: 'durable' } })
    fireEvent.keyDown(search, { key: 'Enter' })

    expect(await screen.findByText('A serious lab gets a durable path.')).toBeInTheDocument()
    expect(region('library-room').getByText('Test isolation')).toBeInTheDocument()
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith('1 page mentions "durable"', 'info'))
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
