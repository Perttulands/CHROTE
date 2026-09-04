import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LibraryView from './LibraryView'
import { resetBeadCardForTest, useBeadCardRequest } from '../beads/beadCard'
import { resetChordsForTest } from '../keys/chords'
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
  beadWork: { beads: [] as unknown[], prefix: 'ctx', projectPath: '' },
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({ settings: DEFAULT_SETTINGS, openSendToSession: mockState.openSendToSession }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../beads/beadsApi', () => ({
  fetchBeadWork: () => Promise.resolve(mockState.beadWork),
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

function CardProbe() {
  const request = useBeadCardRequest()
  return <span data-testid="card-request">{request?.id ?? 'none'}</span>
}

beforeEach(() => {
  resetBeadCardForTest()
  resetChordsForTest()
  mockState.openSendToSession.mockReset()
  mockState.announce.mockReset()
  mockState.shelvesError = null
  mockState.saveError = null
  mockState.gitError = ''
  mockState.saved = []
  mockState.shelves = {
    root: '/corpus',
    librarianSession: 'librarian',
    beadsProject: '/corpus/store',
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
  mockState.beadWork = {
    prefix: 'ctx',
    projectPath: '/corpus/store',
    beads: [
      { id: 'ctx-c2f', title: 'Distil the harness notes', status: 'open', priority: 1, blocked: false },
      { id: 'ctx-d71', title: 'Already done', status: 'closed', priority: 2, blocked: false },
    ],
  }
})

/**
 * A page's name appears in more than one place on purpose — in the rail, on
 * the map, in a listing — so every query says which region it means.
 */
function region(name: 'library-left' | 'library-room' | 'library-map' | 'library-strip') {
  const found = document.querySelector<HTMLElement>(`.${name}`)
  if (!found) throw new Error(`the ${name} region is not on screen`)
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

/** Step into a shelf from the rail, then open one of its pages. */
async function openWorkflowPage() {
  fireEvent.click(region('library-left').getByRole('button', { name: /^preferences/ }))
  fireEvent.click(await region('library-room').findByText('Workflow Preferences'))
  await screen.findByText('Prefer small, verifiable changes.')
}

function pressAltR() {
  act(() => {
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'r', altKey: true, bubbles: true, cancelable: true }))
  })
}

describe('LibraryView', () => {
  it('says so when no corpus is configured', async () => {
    mockState.shelves = { root: '', shelves: [], librarianSession: '', beadsProject: '' }
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

  it('opens a page from the map and shows its neighbours in the strip above it', async () => {
    await openLibrary()

    fireEvent.click(mapNode('Workflow Preferences'))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    expect(screen.getByText('Near this page')).toBeInTheDocument()
    expect(region('library-strip').getByRole('button', { name: 'Test isolation' })).toBeInTheDocument()
    expect(region('library-strip').getByRole('button', { name: 'Tool Preferences' })).toBeInTheDocument()
  })

  it('turns the map over with Alt+R and back', async () => {
    await openLibrary()
    fireEvent.click(mapNode('Workflow Preferences'))
    await screen.findByText('Prefer small, verifiable changes.')

    pressAltR()
    expect(screen.getByText('The map')).toBeInTheDocument()
    expect(screen.queryByText('Prefer small, verifiable changes.')).not.toBeInTheDocument()
    // The open page stays on the table: it and its neighbours are lit.
    expect(mapNode('Workflow Preferences')).toHaveClass('hot')

    pressAltR()
    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
  })

  it('lights the pages whose names hold what is being typed', async () => {
    await openLibrary()

    fireEvent.change(screen.getByRole('searchbox', { name: 'Search the library' }), { target: { value: 'isolation' } })

    expect(mapNode('Test isolation')).toHaveClass('hot')
    expect(mapNode('Tool Preferences')).not.toHaveClass('hot')
  })

  it('lists only the proposals still in flight', async () => {
    await openLibrary()

    expect(await screen.findByText('ctx-c2f')).toBeInTheDocument()
    expect(screen.queryByText('ctx-d71')).not.toBeInTheDocument()
  })

  it('opens the Bead card from a proposal', async () => {
    render(<><LibraryView /><CardProbe /></>)

    fireEvent.click(await screen.findByText('ctx-c2f'))

    expect(screen.getByTestId('card-request')).toHaveTextContent('ctx-c2f')
  })

  it('steps from a shelf to a page and shows its history beneath the head', async () => {
    await openLibrary()

    fireEvent.click(region('library-left').getByRole('button', { name: /^preferences/ }))
    expect(await region('library-room').findByText('Tool Preferences')).toBeInTheDocument()

    fireEvent.click(region('library-room').getByText('Workflow Preferences'))

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

  it('lists the pages that link here beneath the body, and follows one', async () => {
    await openLibrary()
    await openWorkflowPage()

    const linkedFrom = document.querySelector<HTMLElement>('.library-linked-from')
    expect(linkedFrom).toHaveTextContent('Linked from')
    fireEvent.click(within(linkedFrom!).getByRole('button', { name: 'Test isolation' }))

    expect(await screen.findByText('A serious lab gets a durable path.')).toBeInTheDocument()
  })

  it('carries the open page as the reference to the Librarian and the drawer', async () => {
    await openLibrary()
    await openWorkflowPage()

    expect(screen.getByTestId('resident')).toHaveAttribute('data-reference', 'library preferences/workflow.md')

    fireEvent.click(screen.getByRole('button', { name: /^Send/ }))
    expect(mockState.openSendToSession).toHaveBeenCalledWith({ reference: 'library preferences/workflow.md' })
  })

  it('opens a page straight from an arrival', async () => {
    await openLibrary()

    fireEvent.click(region('library-left').getByText('Record a workflow preference'))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
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
})
