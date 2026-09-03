import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView'
import { MAX_TEXT_PREVIEW_BYTES } from './FileViewer'
import { deleteItem, fetchDirectory, probeTextFile, readTextFile, renameItem, uploadFiles, writeTextFile } from './FilesView/fileService'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return {
    ...actual,
    fetchDirectory: vi.fn(),
    createFile: vi.fn(),
    createFolder: vi.fn(),
    deleteItem: vi.fn(),
    probeTextFile: vi.fn(),
    readTextFile: vi.fn(),
    renameItem: vi.fn(),
    uploadFiles: vi.fn(),
    writeTextFile: vi.fn(),
    getDownloadUrl: (path: string) => `/api/files/raw${path}`,
  }
})

const mockFetchDirectory = vi.mocked(fetchDirectory)
const mockProbeTextFile = vi.mocked(probeTextFile)
const mockReadTextFile = vi.mocked(readTextFile)
const mockWriteTextFile = vi.mocked(writeTextFile)

const textFile = (name: string, content = '') => ({
  name,
  path: `/${name}`,
  isDir: false,
  size: content.length,
  modified: '2026-01-01T00:00:00Z',
  type: 'text/plain',
})

const directory = (name: string) => ({
  name,
  path: `/${name}`,
  isDir: true,
  size: 0,
  modified: '2026-01-01T00:00:00Z',
  type: '',
})

function mockRootFiles(contentsByName: Record<string, string>) {
  mockFetchDirectory.mockImplementation(async (path: string) => {
    if (path === '/') {
      return Object.entries(contentsByName).map(([name, content]) => textFile(name, content))
    }
    return []
  })
  mockReadTextFile.mockImplementation(async (path: string) => {
    const name = path.replace(/^\//, '')
    return contentsByName[name] ?? ''
  })
}

async function openRootFile(name: string) {
  const namePattern = new RegExp(name.replace('.', '\\.'), 'i')
  const folderRow = screen.queryByRole('row', { name: namePattern })
  const treeRow = await screen.findByRole('treeitem', { name: namePattern })
  fireEvent.click(folderRow || treeRow)
  await within(editorTabs()).findByRole('button', { name: namePattern })
}

function editorTabs() {
  return document.querySelector('.fb-editor-tabs') as HTMLElement
}

function installClipboardMock(writeText = vi.fn().mockResolvedValue(undefined)) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
  return writeText
}

beforeEach(() => {
  vi.clearAllMocks()
  window.localStorage.clear()
  installClipboardMock()
  mockFetchDirectory.mockResolvedValue([])
  mockProbeTextFile.mockResolvedValue(null)
  mockReadTextFile.mockResolvedValue('')
  mockWriteTextFile.mockResolvedValue(undefined)
})

describe('FilesView directory values', () => {
  it('shows loading, directory contents, item count, explorer tree, and upload control', async () => {
    let finishLoad!: (items: Array<ReturnType<typeof textFile> | ReturnType<typeof directory>>) => void
    mockFetchDirectory.mockReturnValue(new Promise(resolve => { finishLoad = resolve }))

    const { container } = render(<FilesView />)
    expect(container.querySelector('.fb-loading')).toBeInTheDocument()

    finishLoad([directory('code'), textFile('readme.txt', 'hello')])
    expect(await screen.findByRole('row', { name: /readme\.txt/i })).toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: /File readme\.txt/i })).toBeInTheDocument()
    expect(container.querySelector('.fb-statusbar')).toHaveTextContent('2 items')
    expect(screen.getByRole('button', { name: /^Folder$/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTitle('Upload')).toBeEnabled()
  })

  it('reloads the failed folder from its own error, and keeps the exact navigated path in the breadcrumb', async () => {
    let offline = true
    mockFetchDirectory.mockImplementation(async path => {
      if (offline) throw new Error('file service offline')
      return path === '/' ? [directory('code')] : [textFile('package.json', '{}')]
    })

    const { container } = render(<FilesView />)
    expect(await screen.findByText('file service offline')).toBeInTheDocument()

    // Retry re-reads the folder in place: the error goes and its contents arrive.
    offline = false
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(await screen.findByRole('row', { name: /code/i })).toBeInTheDocument()
    expect(screen.queryByText('file service offline')).not.toBeInTheDocument()

    fireEvent.doubleClick(screen.getByRole('row', { name: /code/i }))
    expect(await screen.findByRole('row', { name: /package\.json/i })).toBeInTheDocument()
    expect(container.querySelector('.fb-breadcrumb-item')).toHaveTextContent('code')
    expect(container.querySelector('.fb-path-display')).toHaveTextContent('/code')
  })
})

describe('FilesView folder navigation', () => {
  beforeEach(() => {
    mockFetchDirectory.mockImplementation(async path => (
      path === '/'
        ? [directory('code'), directory('projects'), textFile('readme.txt', 'hello')]
        : [directory('src'), textFile('package.json', '{}')]
    ))
  })

  it('walks into a folder and back out by breadcrumb and by Up, and re-reads the folder on Refresh', async () => {
    const { container } = render(<FilesView />)
    await screen.findByRole('row', { name: /readme\.txt/i })

    // Nothing to go back to, forward to, or up from: this is where Files opens.
    expect(screen.getByRole('button', { name: 'Back' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Forward' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Up' })).toBeDisabled()

    fireEvent.doubleClick(screen.getByRole('row', { name: /code/i }))

    expect(await screen.findByRole('row', { name: /package\.json/i })).toBeInTheDocument()
    expect(screen.getByRole('row', { name: /src/i })).toBeInTheDocument()
    expect(container.querySelector('.fb-breadcrumb-item')).toHaveTextContent('code')

    // The leading / of the path is a way home, not a way into the path editor.
    fireEvent.click(container.querySelector('.fb-breadcrumb-root') as HTMLElement)

    expect(await screen.findByRole('row', { name: /readme\.txt/i })).toBeInTheDocument()
    expect(container.querySelector('.fb-breadcrumb-item')).toBeNull()

    fireEvent.doubleClick(screen.getByRole('row', { name: /code/i }))
    await screen.findByRole('row', { name: /package\.json/i })
    fireEvent.click(screen.getByRole('button', { name: 'Up' }))

    expect(await screen.findByRole('row', { name: /readme\.txt/i })).toBeInTheDocument()

    mockFetchDirectory.mockClear()
    fireEvent.click(screen.getByTitle('Refresh'))

    await waitFor(() => expect(mockFetchDirectory).toHaveBeenCalledWith('/'))
  })

  it('uploads into the folder on screen and re-reads it so the new file appears', async () => {
    const { container } = render(<FilesView />)
    fireEvent.doubleClick(await screen.findByRole('row', { name: /code/i }))
    await screen.findByRole('row', { name: /package\.json/i })
    mockFetchDirectory.mockClear()

    const upload = new File(['uploaded content'], 'upload.txt', { type: 'text/plain' })
    const input = container.querySelector('input[type="file"]') as HTMLInputElement
    Object.defineProperty(input, 'files', { configurable: true, value: [upload] })
    fireEvent.change(input)

    await waitFor(() => expect(uploadFiles).toHaveBeenCalledWith('/code', [upload]))
    await waitFor(() => expect(mockFetchDirectory).toHaveBeenCalledWith('/code'))
  })
})

describe('FilesView listing controls', () => {
  beforeEach(() => {
    mockFetchDirectory.mockResolvedValue([directory('code'), directory('projects'), textFile('readme.txt', 'hello')])
  })

  it('reads the same folder as columned rows or as named tiles', async () => {
    const { container } = render(<FilesView />)
    await screen.findByRole('row', { name: /readme\.txt/i })

    // The list is the columned reading, sortable by each column it shows.
    expect(screen.getByRole('button', { name: /^Modified/ })).toBeInTheDocument()

    fireEvent.click(screen.getByTitle('Grid view'))

    // Tiles drop the columns and keep every item named.
    const grid = container.querySelector('.fb-grid') as HTMLElement
    expect(screen.queryByRole('row')).not.toBeInTheDocument()
    for (const name of ['code', 'projects', 'readme.txt']) {
      expect(within(grid).getByText(name)).toBeInTheDocument()
    }

    fireEvent.click(screen.getByTitle('List view'))

    expect(screen.getByRole('row', { name: /readme\.txt/i })).toBeInTheDocument()
    expect(container.querySelector('.fb-grid')).toBeNull()
  })

  it('narrows the listing to the items matching the filter', async () => {
    const { container } = render(<FilesView />)
    await screen.findByRole('row', { name: /readme\.txt/i })

    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'code' } })

    expect(screen.getByRole('row', { name: /code/i })).toBeInTheDocument()
    expect(screen.queryByRole('row', { name: /readme\.txt/i })).not.toBeInTheDocument()
    expect(container.querySelector('.fb-statusbar')).toHaveTextContent('1 items')

    // A filter that matches nothing says so rather than looking like an empty folder.
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'nothing here' } })
    expect(screen.getByText('No matching files')).toBeInTheDocument()

    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '' } })
    expect(screen.getByRole('row', { name: /readme\.txt/i })).toBeInTheDocument()
  })
})

describe('FilesView fallback text preview', () => {
  it('previews an unknown small UTF-8 file without treating binary formats as editable', async () => {
    mockRootFiles({ 'events.records': 'ignored direct read' })
    mockProbeTextFile.mockResolvedValueOnce('detected text content')

    render(<FilesView />)
    await openRootFile('events.records')

    expect(await screen.findByText('detected text content')).toBeInTheDocument()
    expect(mockProbeTextFile).toHaveBeenCalledWith('/events.records', MAX_TEXT_PREVIEW_BYTES)
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  })
})

describe('FilesView editor tab bulk close', () => {
  it('closes every clean tab at once, and refuses while any of them is unsaved', async () => {
    mockRootFiles({
      'one.txt': 'one',
      'two.txt': 'two',
    })

    const { unmount } = render(<FilesView />)

    await openRootFile('one.txt')
    await openRootFile('two.txt')

    const tabs = editorTabs()
    expect(await within(tabs).findByRole('button', { name: /one\.txt/ })).toBeInTheDocument()
    expect(await within(tabs).findByRole('button', { name: /two\.txt/ })).toBeInTheDocument()

    fireEvent.click(await within(tabs).findByRole('button', { name: /close all/i }))

    await waitFor(() => {
      expect(within(tabs).queryByRole('button', { name: /one\.txt/ })).not.toBeInTheDocument()
      expect(within(tabs).queryByRole('button', { name: /two\.txt/ })).not.toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Folder' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'File' })).toBeDisabled()
    unmount()

    // One unsaved buffer refuses the whole gesture: nothing closes, and the
    // operator is told why rather than losing the edit.
    mockRootFiles({
      'clean.txt': 'clean',
      'dirty.txt': 'dirty',
    })

    render(<FilesView />)

    await openRootFile('clean.txt')
    await openRootFile('dirty.txt')
    fireEvent.change(await screen.findByDisplayValue('dirty'), { target: { value: 'changed but unsaved' } })

    fireEvent.click(await within(editorTabs()).findByRole('button', { name: /close all/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/save or close unsaved files/i)
    expect(within(editorTabs()).getByRole('button', { name: /clean\.txt/ })).toBeInTheDocument()
    expect(within(editorTabs()).getByRole('button', { name: /dirty\.txt/ })).toBeInTheDocument()
  })

  it('offers Close Others and Close All from the tab context menu for multiple open tabs', async () => {
    mockRootFiles({
      'one.txt': 'one',
      'two.txt': 'two',
    })

    render(<FilesView />)

    await openRootFile('one.txt')
    await openRootFile('two.txt')

    fireEvent.contextMenu(within(editorTabs()).getByRole('button', { name: /one\.txt/ }), { clientX: 320, clientY: 180 })
    const menu = document.querySelector('.menu-sheet') as HTMLElement

    expect(within(menu).getByRole('menuitem', { name: 'Close others' })).toBeInTheDocument()
    expect(within(menu).getByRole('menuitem', { name: 'Close all' })).toBeInTheDocument()

    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Close others' }))

    await waitFor(() => {
      expect(within(editorTabs()).getByRole('button', { name: /one\.txt/ })).toBeInTheDocument()
      expect(within(editorTabs()).queryByRole('button', { name: /two\.txt/ })).not.toBeInTheDocument()
    })
  })
})

describe('FilesView dirty buffer safety', () => {
  it('keeps unsaved edits when the same file is reopened from the tree', async () => {
    mockRootFiles({ 'notes.txt': 'from disk' })
    render(<FilesView />)

    await openRootFile('notes.txt')
    fireEvent.change(await screen.findByDisplayValue('from disk'), { target: { value: 'unsaved work' } })
    mockReadTextFile.mockClear()

    await openRootFile('notes.txt')

    expect(await screen.findByDisplayValue('unsaved work')).toBeInTheDocument()
    expect(mockReadTextFile).not.toHaveBeenCalled()
    expect(within(editorTabs()).getAllByRole('button', { name: /notes\.txt/ })).toHaveLength(1)
  })

  it('keeps a dirty tab until its close is repeated', async () => {
    mockRootFiles({ 'notes.txt': 'from disk' })
    render(<FilesView />)

    await openRootFile('notes.txt')
    fireEvent.change(await screen.findByDisplayValue('from disk'), { target: { value: 'unsaved work' } })

    const closeControl = () => within(within(editorTabs()).getByRole('button', { name: /notes\.txt/ }))
      .getByRole('button', { name: 'x' })
    fireEvent.click(closeControl())

    // The close asks where it stands and leaves the buffer exactly as it was.
    expect(await screen.findByRole('alert')).toHaveTextContent(/close it again to discard them/i)
    expect(within(editorTabs()).getByRole('button', { name: /notes\.txt/ })).toBeInTheDocument()
    expect(screen.getByDisplayValue('unsaved work')).toBeInTheDocument()

    // Repeating it discards the buffer; the last tab closing drops back to Folder mode.
    fireEvent.click(closeControl())
    await waitFor(() => expect(screen.getByRole('button', { name: 'Folder' })).toHaveAttribute('aria-pressed', 'true'))
    expect(screen.queryByDisplayValue('unsaved work')).not.toBeInTheDocument()
  })

  it('refuses to delete a file whose open buffer has unsaved edits', async () => {
    mockRootFiles({ 'notes.txt': 'from disk' })
    render(<FilesView />)

    await openRootFile('notes.txt')
    fireEvent.change(await screen.findByDisplayValue('from disk'), { target: { value: 'unsaved work' } })

    // The file listing only renders in Folder mode.
    fireEvent.click(screen.getByRole('button', { name: 'Folder' }))
    fireEvent.contextMenu(await screen.findByRole('row', { name: /notes\.txt/i }))
    fireEvent.click(await screen.findByRole('menuitem', { name: /^delete$/i }))
    fireEvent.click(await screen.findByRole('button', { name: /^delete$/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/save or close unsaved files before deleting/i)
    expect(deleteItem).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'File' }))
    expect(await screen.findByDisplayValue('unsaved work')).toBeInTheDocument()
  })

  it('follows a renamed file so the unsaved buffer saves to its new path', async () => {
    mockRootFiles({ 'notes.txt': 'from disk' })
    render(<FilesView />)

    await openRootFile('notes.txt')
    fireEvent.change(await screen.findByDisplayValue('from disk'), { target: { value: 'unsaved work' } })

    fireEvent.click(screen.getByRole('button', { name: 'Folder' }))
    fireEvent.contextMenu(await screen.findByRole('row', { name: /notes\.txt/i }))
    fireEvent.click(await screen.findByRole('menuitem', { name: /^rename$/i }))
    const renameInput = await screen.findByDisplayValue('notes.txt')
    fireEvent.change(renameInput, { target: { value: 'renamed.txt' } })
    fireEvent.keyDown(renameInput, { key: 'Enter' })

    await waitFor(() => expect(renameItem).toHaveBeenCalledWith('/notes.txt', '/renamed.txt'))
    fireEvent.click(screen.getByRole('button', { name: 'File' }))
    await within(editorTabs()).findByRole('button', { name: /renamed\.txt/ })
    expect(await screen.findByDisplayValue('unsaved work')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mockWriteTextFile).toHaveBeenCalledWith('/renamed.txt', 'unsaved work'))
  })

  it('surfaces a read failure without leaving the tab stuck loading', async () => {
    mockRootFiles({ 'broken.txt': 'x' })
    mockReadTextFile.mockRejectedValue(new Error('boom'))
    render(<FilesView />)

    await openRootFile('broken.txt')

    expect(await screen.findByText(/boom|failed to read/i)).toBeInTheDocument()
  })
})

describe('FilesView Markdown editor', () => {
  it('renders Markdown files with a markdown-specific editor and safe preview markup', async () => {
    mockRootFiles({
      'notes.md': [
        '# Title',
        '',
        '- Plain item',
        '- [x] Done task',
        '- [ ] Open task',
        '',
        '> Quoted line',
        '',
        '~~~ts',
        'const answer = 42',
        '~~~',
        '',
        '[Safe link](https://example.com/path)',
        '<script>alert("owned")</script>',
        '[Bad link](javascript:alert(1))',
      ].join('\n'),
    })

    render(<FilesView />)

    await openRootFile('notes.md')

    fireEvent.click(screen.getByRole('button', { name: 'Split Markdown view' }))
    expect(await screen.findByLabelText('Markdown source for notes.md')).toBeInTheDocument()
    expect(screen.getByLabelText('Markdown preview for notes.md')).toBeInTheDocument()
    expect(document.querySelector('.fb-editor-textarea')).not.toBeInTheDocument()

    expect(screen.getByRole('heading', { level: 1, name: 'Title' })).toBeInTheDocument()
    expect(screen.getByText('Plain item').closest('li')).toBeInTheDocument()
    expect(screen.getByText('Quoted line').closest('blockquote')).toBeInTheDocument()
    expect(screen.getByText('const answer = 42').closest('code')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: /Done task/ })).toBeChecked()
    expect(screen.getByRole('checkbox', { name: /Done task/ })).toBeDisabled()
    expect(screen.getByRole('checkbox', { name: /Open task/ })).not.toBeChecked()

    const safeLink = screen.getByRole('link', { name: 'Safe link' })
    expect(safeLink).toHaveAttribute('href', 'https://example.com/path')
    expect(safeLink).toHaveAttribute('rel', expect.stringContaining('noopener'))

    expect(document.querySelector('script')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Markdown preview for notes.md')).toHaveTextContent('<script>alert("owned")</script>')
    expect(screen.getByText('Bad link')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Bad link' })).not.toBeInTheDocument()
  })

  it('marks Markdown edits dirty and saves through the existing text file API', async () => {
    mockRootFiles({
      'notes.markdown': '# Original',
    })

    render(<FilesView />)

    await openRootFile('notes.markdown')
    fireEvent.click(screen.getByRole('button', { name: 'Show Markdown source' }))
    const source = await screen.findByLabelText('Markdown source for notes.markdown')
    const nextContent = '# Edited\n\n- saved item'

    fireEvent.change(source, { target: { value: nextContent } })

    expect(within(editorTabs()).getByRole('button', { name: /\* notes\.markdown/ })).toBeInTheDocument()
    const saveButton = screen.getByRole('button', { name: 'Save' })
    expect(saveButton).toBeEnabled()

    fireEvent.click(saveButton)

    await waitFor(() => expect(mockWriteTextFile).toHaveBeenCalledWith('/notes.markdown', nextContent))
    await waitFor(() => expect(within(editorTabs()).getByRole('button', { name: /notes\.markdown/ })).not.toHaveTextContent(/^\*/))
  })

  it('renders malformed fenced code starts as text instead of hanging the Markdown preview', async () => {
    mockRootFiles({
      'odd.md': ['# Odd', '```foo bar', 'still visible', 'after malformed fence'].join('\n'),
    })

    render(<FilesView />)

    await openRootFile('odd.md')

    expect(await screen.findByRole('heading', { level: 1, name: 'Odd' })).toBeInTheDocument()
    const preview = screen.getByLabelText('Markdown preview for odd.md')
    expect(preview).toHaveTextContent('```foo bar still visible after malformed fence')
  })
  it('restores the active file, Markdown mode, and file scroll state after remount', async () => {
    mockRootFiles({ 'persisted.md': '# Persisted\n\nBody' })

    const first = render(<FilesView />)
    await openRootFile('persisted.md')
    fireEvent.click(screen.getByRole('button', { name: 'Split Markdown view' }))
    const scroll = await screen.findByTestId('file-viewer-scroll')
    Object.defineProperty(scroll, 'scrollTop', { configurable: true, writable: true, value: 360 })
    fireEvent.scroll(scroll)
    first.unmount()

    render(<FilesView />)

    expect(await within(editorTabs()).findByRole('button', { name: /persisted\.md/ })).toBeInTheDocument()
    expect(await screen.findByLabelText('Markdown source for persisted.md')).toBeInTheDocument()
    expect(screen.getByLabelText('Markdown preview for persisted.md')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('file-viewer-scroll')).toHaveProperty('scrollTop', 360))
  })

  it('prefills Send to Session with the active absolute path', async () => {
    const onSendPath = vi.fn()
    mockRootFiles({ 'send-me.txt': 'path payload' })

    render(<FilesView onSendPath={onSendPath} sendTargetLabel="shell" />)
    await openRootFile('send-me.txt')
    fireEvent.click(screen.getByRole('button', { name: 'Send Path' }))

    expect(onSendPath).toHaveBeenCalledWith('/send-me.txt')
    expect(screen.getByRole('button', { name: 'Send Path' })).toHaveAttribute('title', 'Send path to shell')
  })
})

describe('FilesView saved path groups', () => {
  it('normalizes and deduplicates persisted pins before rendering them', async () => {
    window.localStorage.setItem('chrote.files.pinnedPaths', JSON.stringify([
      { path: '/srv/chrote', kind: 'directory' },
      { path: '/srv//chrote/', kind: 'directory' },
      { path: '/srv/chrote', kind: 'file' },
    ]))

    render(<FilesView />)

    expect(await screen.findByRole('button', { name: /Pinned.*1/ })).toBeInTheDocument()
    expect(JSON.parse(window.localStorage.getItem('chrote.files.pinnedPaths') || '[]')).toEqual([
      { path: '/srv/chrote', kind: 'directory' },
    ])
  })

  it('renders multiple pinned paths and collapses the pinned group independently', async () => {
    window.localStorage.setItem('chrote.files.pinnedPaths', JSON.stringify([
      { path: '/home/operator', kind: 'directory' },
      { path: '/srv', kind: 'directory' },
    ]))
    window.localStorage.setItem('chrote.files.recentPaths', JSON.stringify([
      { path: '/etc/hosts', kind: 'file' },
    ]))

    render(<FilesView />)

    const pinnedToggle = await screen.findByRole('button', { name: /Pinned.*2/ })
    expect(pinnedToggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: /operator/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /srv/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Recent.*1/ })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: /hosts/ })).toBeInTheDocument()

    fireEvent.click(pinnedToggle)

    expect(screen.getByRole('button', { name: /Pinned.*2/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('button', { name: /alice/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /srv/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /hosts/ })).toBeInTheDocument()
    expect(JSON.parse(window.localStorage.getItem('chrote.files.savedGroupsCollapsed') || '{}')).toEqual({
      pinned: true,
      recent: false,
    })
  })

  it('restores collapsed Recent state from localStorage without collapsing Pinned', async () => {
    window.localStorage.setItem('chrote.files.pinnedPaths', JSON.stringify([
      { path: '/home/operator', kind: 'directory' },
    ]))
    window.localStorage.setItem('chrote.files.recentPaths', JSON.stringify([
      { path: '/etc/hosts', kind: 'file' },
    ]))
    window.localStorage.setItem('chrote.files.savedGroupsCollapsed', JSON.stringify({
      pinned: false,
      recent: true,
    }))

    render(<FilesView />)

    await waitFor(() => expect(screen.getByRole('button', { name: /Pinned.*1/ })).toHaveAttribute('aria-expanded', 'true'))
    expect(screen.getByRole('button', { name: /operator/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Recent.*1/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('button', { name: /hosts/ })).not.toBeInTheDocument()
  })
})

describe('FilesView context menu copy/open actions', () => {
  beforeEach(() => {
    mockFetchDirectory.mockImplementation(async (path: string) => {
      if (path === '/') {
        return [
          { name: 'etc', path: '/etc', isDir: true, size: 0, modified: '2026-01-01T00:00:00Z', type: '' },
          { name: 'hosts', path: '/etc/hosts', isDir: false, size: 42, modified: '2026-01-01T00:00:00Z', type: 'text/plain' },
        ]
      }
      return []
    })
  })

  it('offers useful background actions at filesystem root without arbitrary root restrictions', async () => {
    render(<FilesView />)
    await screen.findByRole('row', { name: /hosts/ })

    fireEvent.contextMenu(document.querySelector('.fb-content') as HTMLElement)
    const menu = document.querySelector('.menu-sheet') as HTMLElement

    expect(within(menu).getByRole('menuitem', { name: 'New file' })).not.toBeDisabled()
    expect(within(menu).getByRole('menuitem', { name: 'New folder' })).not.toBeDisabled()
    expect(within(menu).getByRole('menuitem', { name: 'Upload' })).not.toBeDisabled()
    expect(within(menu).getByRole('menuitem', { name: 'Refresh' })).toBeInTheDocument()
    expect(within(menu).getByRole('menuitem', { name: 'Copy current folder path' })).toBeInTheDocument()
    expect(within(menu).getByRole('menuitem', { name: 'Pin current folder' })).toBeInTheDocument()

    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Copy current folder path' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('/')
  })

  it('copies selected paths, relative path, and opens a selected item parent from item menus', async () => {
    render(<FilesView />)
    const hostsRow = await screen.findByRole('row', { name: /hosts/ })

    fireEvent.contextMenu(hostsRow)
    const menu = document.querySelector('.menu-sheet') as HTMLElement

    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Copy selected paths' }))
    expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith('/etc/hosts')

    fireEvent.contextMenu(hostsRow)
    const secondMenu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(secondMenu).getByRole('menuitem', { name: 'Copy relative path' }))
    expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith('etc/hosts')

    fireEvent.contextMenu(hostsRow)
    const thirdMenu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(thirdMenu).getByRole('menuitem', { name: 'Open parent folder' }))
    await waitFor(() => expect(mockFetchDirectory).toHaveBeenCalledWith('/etc'))
  })
})
