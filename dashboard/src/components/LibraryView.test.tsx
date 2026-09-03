import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LibraryView from './LibraryView'
import { resetBeadCardForTest, useBeadCardRequest } from '../beads/beadCard'
import { resetChordsForTest } from '../keys/chords'
import type {
  LibraryChange,
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
  results: [] as LibrarySearchResult[],
  saved: [] as { path: string; content: string; summary: string }[],
  saveError: null as Error | null,
  beadWork: { beads: [] as unknown[], prefix: 'ctx', projectPath: '' },
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({ openSendToSession: mockState.openSendToSession }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../beads/beadsApi', () => ({
  fetchBeadWork: () => Promise.resolve(mockState.beadWork),
}))

vi.mock('./Desk', () => ({
  default: ({ label, sessionName, reference }: { label: string; sessionName?: string; reference: string }) => (
    <div data-testid="desk" data-session={sessionName ?? ''} data-reference={reference}>{label}</div>
  ),
}))

vi.mock('../library/libraryApi', async () => {
  const actual = await vi.importActual<typeof import('../library/libraryApi')>('../library/libraryApi')
  return {
    ...actual,
    fetchShelves: () => (mockState.shelvesError ? Promise.reject(mockState.shelvesError) : Promise.resolve(mockState.shelves)),
    fetchChanges: () => Promise.resolve({ changes: mockState.changes, error: mockState.gitError }),
    fetchShelfPages: (shelf: string) => Promise.resolve({ pages: mockState.pages.get(shelf) ?? [], error: mockState.gitError }),
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
  ])
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
 * The shelves appear twice on purpose — as the rail at the left and as the
 * cards in the room — so every query says which column it means.
 */
function column(name: 'library-left' | 'library-room' | 'library-right') {
  const found = document.querySelector<HTMLElement>(`.${name}`)
  if (!found) throw new Error(`the ${name} column is not on screen`)
  return within(found)
}

async function openLibrary() {
  render(<LibraryView />)
  await screen.findByText('Reading room')
}

/** Step into a shelf from the rail, then open one of its pages. */
async function openWorkflowPage() {
  fireEvent.click(column('library-left').getByRole('button', { name: /^preferences/ }))
  fireEvent.click(await column('library-room').findByText('Workflow Preferences'))
  await screen.findByText('Prefer small, verifiable changes.')
}

describe('LibraryView', () => {
  it('says so when no corpus is configured', async () => {
    mockState.shelves = { root: '', shelves: [], librarianSession: '', beadsProject: '' }
    render(<LibraryView />)

    expect(await screen.findByText('No library is configured')).toBeInTheDocument()
    expect(screen.queryByTestId('desk')).not.toBeInTheDocument()
  })

  it('says what refused rather than showing a blank tab', async () => {
    mockState.shelvesError = new Error('The corpus is not readable')
    render(<LibraryView />)

    expect(await screen.findByText('The corpus is not readable')).toBeInTheDocument()
  })

  it('opens on the shelves, their counts and what last touched each', async () => {
    await openLibrary()

    expect(await screen.findByText('20 pages on 2 shelves · last change 1 hour ago')).toBeInTheDocument()
    expect(column('library-left').getByRole('button', { name: /^knowledge\s*13$/ })).toBeInTheDocument()
    expect(column('library-room').getByText('Curate the knowledge shelf')).toBeInTheDocument()
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith('Library loaded · 20 pages on 2 shelves', 'info'))
  })

  it('opens on the corpus\'s own README when it has one', async () => {
    mockState.page.set('README.md', {
      path: 'README.md',
      title: 'The corpus',
      updated: NOW,
      author: 'The Operator',
      content: '# The corpus\n\nWhat this library holds.\n',
      history: [],
    })
    render(<LibraryView />)

    expect(await screen.findByText('What this library holds.')).toBeInTheDocument()
    expect(screen.queryByText('Reading room')).not.toBeInTheDocument()
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

  it('steps from a shelf to a page and shows its history and its shelf', async () => {
    await openLibrary()

    fireEvent.click(column('library-left').getByRole('button', { name: /^preferences/ }))
    expect(await column('library-room').findByText('Tool Preferences')).toBeInTheDocument()

    fireEvent.click(column('library-room').getByText('Workflow Preferences'))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
    expect(screen.getByText('preferences/workflow.md · changed 1 hour ago by The Operator')).toBeInTheDocument()
    expect(screen.getByText('aaaaaaa')).toBeInTheDocument()
    expect(screen.getByText('On preferences')).toBeInTheDocument()
  })

  it('carries the open page as the reference to the desk and the drawer', async () => {
    await openLibrary()
    await openWorkflowPage()

    expect(screen.getByTestId('desk')).toHaveAttribute('data-reference', 'library preferences/workflow.md')

    fireEvent.click(screen.getByRole('button', { name: /^Send/ }))
    expect(mockState.openSendToSession).toHaveBeenCalledWith({ reference: 'library preferences/workflow.md' })
  })

  it('opens a page straight from an arrival', async () => {
    await openLibrary()

    fireEvent.click(column('library-left').getByText('Record a workflow preference'))

    expect(await screen.findByText('Prefer small, verifiable changes.')).toBeInTheDocument()
  })

  it('searches the whole corpus and opens a result', async () => {
    await openLibrary()

    const search = screen.getByRole('searchbox', { name: 'Search the library' })
    fireEvent.change(search, { target: { value: 'isolation' } })
    fireEvent.keyDown(search, { key: 'Enter' })

    expect(await screen.findByText('Test isolation')).toBeInTheDocument()
    expect(screen.getByText('A serious lab gets a durable path.')).toBeInTheDocument()
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith('1 page mentions "isolation"', 'info'))
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
