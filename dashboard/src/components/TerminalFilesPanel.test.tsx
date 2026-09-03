import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView'
import {
  createFile,
  createFolder,
  deleteItem,
  fetchDirectory,
  fetchFileDiff,
  findFiles,
  readTextFile,
  renameItem,
  uploadFiles,
  writeTextFile,
} from './FilesView/fileService'
import TerminalFilesPanel from './TerminalFilesPanel'
import {
  DEFAULT_WORKSPACE_FILES_STATE,
  readWorkspaceFilesState,
  writeWorkspaceFilesState,
} from './workspaceFilesState'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return {
    ...actual,
    createFile: vi.fn(),
    createFolder: vi.fn(),
    deleteItem: vi.fn(),
    fetchDirectory: vi.fn(),
    fetchFileDiff: vi.fn(),
    findFiles: vi.fn(),
    readTextFile: vi.fn(),
    renameItem: vi.fn(),
    uploadFiles: vi.fn(),
    writeTextFile: vi.fn(),
    getDownloadUrl: (path: string) => `/api/files/raw${path}`,
  }
})

const sessionMocks = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
}))

const statusMocks = vi.hoisted(() => ({
  announce: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [{ id: 'terminal1-window-0', boundSessions: ['alice:shell'], activeSession: 'alice:shell', colorIndex: 0 }],
      },
      terminal2: { windowCount: 1, windows: [] },
      terminal3: { windowCount: 1, windows: [] },
    },
    focusedWindowKey: 'terminal1-terminal1-window-0',
    sessions: [{ name: 'shell', unixUser: 'alice', windows: 1, attached: true, group: 'shell', cwd: '/srv/chrote' }],
    openSendToSession: sessionMocks.openSendToSession,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ status: null, announce: statusMocks.announce }),
}))

const mockedFetchDirectory = vi.mocked(fetchDirectory)
const mockedFetchFileDiff = vi.mocked(fetchFileDiff)
const mockedFindFiles = vi.mocked(findFiles)
const mockedReadTextFile = vi.mocked(readTextFile)
const mockedWriteTextFile = vi.mocked(writeTextFile)
const mockedCreateFile = vi.mocked(createFile)
const mockedCreateFolder = vi.mocked(createFolder)
const mockedDeleteItem = vi.mocked(deleteItem)
const mockedRenameItem = vi.mocked(renameItem)
const mockedUploadFiles = vi.mocked(uploadFiles)

const readme = {
  path: '/srv/chrote/README.md',
  name: 'README.md',
  isDir: false,
  size: 40,
  modified: '2026-01-01T00:00:00Z',
  type: 'text/markdown',
}

const docs = {
  path: '/srv/chrote/docs',
  name: 'docs',
  isDir: true,
  size: 0,
  modified: '2026-01-01T00:00:00Z',
  type: '',
}

function renderPanel(onOpenInFiles = vi.fn()) {
  render(
    <TerminalFilesPanel
      workspaceId="terminal1"
      collapsed={false}
      width={320}
      pinned={false}
      canPin
      panelId="terminal1-files-sidecar"
      onTogglePin={vi.fn()}
      onClose={vi.fn()}
      onWidthChange={vi.fn()}
      onOpenInFiles={onOpenInFiles}
    />,
  )
  return onOpenInFiles
}

/** Start the panel where a test needs it, the way the operator left it. */
function seedPanelAt(currentPath: string) {
  writeWorkspaceFilesState('terminal1', {
    ...DEFAULT_WORKSPACE_FILES_STATE,
    currentPath,
    expandedPaths: [currentPath],
  })
}

describe('TerminalFilesPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    mockedFetchDirectory.mockImplementation(async path => {
      if (path === '/') return [{ ...readme, path: '/README.md' }]
      if (path === '/srv') return [{ ...docs, path: '/srv/chrote', name: 'chrote' }]
      if (path === '/srv/chrote') return [docs, readme]
      return []
    })
    mockedReadTextFile.mockResolvedValue('# CHROTE')
    mockedWriteTextFile.mockResolvedValue(undefined)
    mockedFindFiles.mockResolvedValue({ matches: [], truncated: false })
    mockedFetchFileDiff.mockResolvedValue({ path: readme.path, repository: '', diff: '', truncated: false })
    mockedCreateFile.mockImplementation(async (path, name) => `${path}/${name}`.replace('//', '/'))
    mockedCreateFolder.mockResolvedValue(undefined)
    mockedDeleteItem.mockResolvedValue(undefined)
    mockedRenameItem.mockResolvedValue(undefined)
    mockedUploadFiles.mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
  })

  it('finds a file by name across the roots and opens the first on Enter', async () => {
    mockedFindFiles.mockResolvedValue({
      matches: [
        { path: '/srv/chrote/docs/journeys.md', name: 'journeys.md' },
        { path: '/srv/chrote/docs/journal.md', name: 'journal.md' },
      ],
      truncated: false,
    })
    renderPanel()

    const field = screen.getByLabelText('Find files')
    fireEvent.change(field, { target: { value: 'journ' } })
    await waitFor(() => expect(mockedFindFiles).toHaveBeenCalledWith('journ', expect.any(AbortSignal)))
    expect(await screen.findByText('2 paths · Enter opens the first')).toBeInTheDocument()

    fireEvent.keyDown(field, { key: 'Enter' })

    expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
    expect(screen.queryByRole('tree', { name: 'File tree' })).not.toBeInTheDocument()
    expect(readWorkspaceFilesState('terminal1').openPath).toBe('/srv/chrote/docs/journeys.md')
  })

  it('says when a find matched more than it will show, and clears on Escape', async () => {
    mockedFindFiles.mockResolvedValue({
      matches: [{ path: '/srv/chrote/docs/journeys.md', name: 'journeys.md' }],
      truncated: true,
    })
    renderPanel()

    fireEvent.change(screen.getByLabelText('Find files'), { target: { value: 'j' } })
    expect(await screen.findByText('1 path · more matched · Enter opens the first')).toBeInTheDocument()

    fireEvent.keyDown(screen.getByLabelText('Find files'), { key: 'Escape' })
    await waitFor(() => expect(screen.getByLabelText('Find files')).toHaveValue(''))
    expect(await screen.findByRole('tree', { name: 'File tree' })).toBeInTheDocument()
  })

  it('replaces the tree with the viewer, renders Markdown, sends the path and comes back', async () => {
    seedPanelAt('/srv/chrote')
    renderPanel()

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))

    expect(await screen.findByRole('heading', { name: 'CHROTE' })).toBeInTheDocument()
    expect(screen.queryByRole('tree', { name: 'File tree' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(sessionMocks.openSendToSession).toHaveBeenCalledWith({
      targetSessionKey: 'alice:shell',
      reference: 'path /srv/chrote/README.md',
    })

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(await screen.findByRole('tree', { name: 'File tree' })).toBeInTheDocument()
    expect(readWorkspaceFilesState('terminal1').openPath).toBeNull()
  })

  it('offers Diff only inside a repository and draws the change with a signed gutter', async () => {
    seedPanelAt('/srv/chrote')
    renderPanel()

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))
    await screen.findByRole('button', { name: 'Edit' })
    expect(screen.queryByRole('button', { name: 'Diff' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    mockedFetchFileDiff.mockResolvedValue({
      path: readme.path,
      repository: '/srv/chrote',
      diff: 'diff --git a/README.md b/README.md\n@@ -1,2 +1,2 @@\n-old line\n+new line\n kept\n',
      truncated: false,
    })
    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))

    fireEvent.click(await screen.findByRole('button', { name: 'Diff' }))
    const diff = await screen.findByLabelText('Diff against HEAD')
    expect(within(diff).getByText('new line').parentElement).toHaveClass('is-add')
    expect(within(diff).getByText('old line').parentElement).toHaveClass('is-del')
    expect(within(diff).getByText('@@ -1,2 +1,2 @@').parentElement).toHaveClass('is-hunk')
    expect(diff.textContent).toContain('+')
    expect(diff.textContent).toContain('-')
    expect(diff.textContent).not.toContain('diff --git')
  })

  it('says so when a file is in no repository', async () => {
    seedPanelAt('/srv/chrote')
    mockedFetchFileDiff.mockResolvedValue({ path: readme.path, repository: '', diff: '', truncated: false })
    renderPanel()

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))
    await screen.findByRole('button', { name: 'Edit' })
    expect(screen.queryByRole('button', { name: 'Diff' })).not.toBeInTheDocument()
  })

  it('edits in place: Tab indents, Ctrl+S saves and announces, Escape asks before discarding', async () => {
    seedPanelAt('/srv/chrote')
    renderPanel()

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))

    const field = await screen.findByLabelText('Edit README.md') as HTMLTextAreaElement
    expect(field).toHaveValue('# CHROTE')

    field.setSelectionRange(0, 0)
    fireEvent.keyDown(field, { key: 'Tab' })
    await waitFor(() => expect(screen.getByLabelText('Edit README.md')).toHaveValue('  # CHROTE'))

    fireEvent.keyDown(screen.getByLabelText('Edit README.md'), { key: 's', ctrlKey: true })
    await waitFor(() => expect(mockedWriteTextFile).toHaveBeenCalledWith('/srv/chrote/README.md', '  # CHROTE'))
    expect(statusMocks.announce).toHaveBeenCalledWith('Saved README.md', 'success')
    expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
  })

  it('arms Discard before it throws an edit away', async () => {
    seedPanelAt('/srv/chrote')
    renderPanel()

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    const field = await screen.findByLabelText('Edit README.md')
    fireEvent.change(field, { target: { value: '# CHANGED' } })

    fireEvent.keyDown(screen.getByLabelText('Edit README.md'), { key: 'Escape' })
    expect(await screen.findByRole('button', { name: 'Confirm' })).toBeInTheDocument()
    expect(screen.getByLabelText('Edit README.md')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
    expect(mockedWriteTextFile).not.toHaveBeenCalled()
  })

  it('exposes sidecar pin and close controls and keeps the tree selection', async () => {
    const togglePin = vi.fn()
    const close = vi.fn()
    render(
      <TerminalFilesPanel
        workspaceId="terminal1"
        collapsed={false}
        width={320}
        pinned={false}
        canPin
        panelId="terminal1-files-sidecar"
        onTogglePin={togglePin}
        onClose={close}
        onWidthChange={vi.fn()}
        onOpenInFiles={vi.fn()}
      />,
    )

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Back' }))
    expect(await screen.findByRole('treeitem', { name: /README\.md/ })).toHaveAttribute('aria-selected', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Pin Files sidecar' }))
    expect(togglePin).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: 'Close Files sidecar' }))
    expect(close).toHaveBeenCalledOnce()
    await waitFor(() => {
      const stored = JSON.parse(window.localStorage.getItem('chrote.workspaceFiles.v1') || '{}')
      expect(stored.version).toBe(1)
      expect(stored.workspaces.terminal1.selectedPath).toBe('/README.md')
      expect(stored.workspaces.terminal1.openPath).toBeNull()
    })
  })

  it('navigates an already-mounted sidecar when a workspace request changes', async () => {
    const props = {
      workspaceId: 'terminal1' as const,
      collapsed: false,
      width: 320,
      pinned: true,
      canPin: true,
      panelId: 'terminal1-files-sidecar',
      onTogglePin: vi.fn(),
      onClose: vi.fn(),
      onWidthChange: vi.fn(),
      onOpenInFiles: vi.fn(),
    }
    const { rerender } = render(<TerminalFilesPanel {...props} />)

    rerender(
      <TerminalFilesPanel
        {...props}
        navigateRequest={{ path: '/srv/chrote', requestId: 1 }}
      />,
    )

    await waitFor(() => expect(readWorkspaceFilesState('terminal1').currentPath).toBe('/srv/chrote'))
  })

  // A path from a terminal link is a file more often than a folder: the
  // panel opens it in the viewer with the tree at its parent, and a path the
  // parent does not list goes to the viewer too, which is where the failure to
  // read it is reported in plain words.
  it('opens a requested file in the viewer, and reports a requested path that is not there', async () => {
    const props = {
      workspaceId: 'terminal1' as const,
      collapsed: false,
      width: 320,
      pinned: true,
      canPin: true,
      panelId: 'terminal1-files-sidecar',
      onTogglePin: vi.fn(),
      onClose: vi.fn(),
      onWidthChange: vi.fn(),
      onOpenInFiles: vi.fn(),
      // The dock clears the request as soon as it is acknowledged, before
      // the listing that resolves it has answered.
      onNavigateRequestHandled: vi.fn(),
    }
    const { rerender } = render(<TerminalFilesPanel {...props} />)

    rerender(<TerminalFilesPanel {...props} navigateRequest={{ path: '/srv/chrote/README.md', requestId: 1 }} />)
    await waitFor(() => expect(props.onNavigateRequestHandled).toHaveBeenCalledWith(1))
    rerender(<TerminalFilesPanel {...props} navigateRequest={null} />)

    expect(await screen.findByTitle('/srv/chrote/README.md')).toBeInTheDocument()
    expect(readWorkspaceFilesState('terminal1').currentPath).toBe('/srv/chrote')
    expect(readWorkspaceFilesState('terminal1').openPath).toBe('/srv/chrote/README.md')

    mockedReadTextFile.mockRejectedValueOnce(new Error('Not found'))
    rerender(<TerminalFilesPanel {...props} navigateRequest={{ path: '/srv/chrote/missing.txt', requestId: 2 }} />)

    expect(await screen.findByTitle('/srv/chrote/missing.txt')).toBeInTheDocument()
    expect(await screen.findByText(/Not found/)).toBeInTheDocument()
  })

  it('invalidates the FileTree cache when Refresh is clicked', async () => {
    renderPanel()

    await screen.findByRole('treeitem', { name: /README\.md/ })
    expect(mockedFetchDirectory).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: 'Refresh Files' }))

    await waitFor(() => expect(mockedFetchDirectory).toHaveBeenCalledTimes(2))
  })

  it('navigates one normalized parent from an accessible control and no-ops at root', async () => {
    renderPanel()

    const up = screen.getByRole('button', { name: 'Go to parent folder' })
    expect(up).toBeDisabled()
    fireEvent.click(up)
    expect(readWorkspaceFilesState('terminal1').currentPath).toBe('/')

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Back' }))
    fireEvent.contextMenu(await screen.findByRole('treeitem', { name: /README\.md/ }))
    const menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Open parent folder' }))

    await waitFor(() => expect(readWorkspaceFilesState('terminal1').currentPath).toBe('/'))
  })

  it('offers the exact single-item actions and preserves sidecar/global routing', async () => {
    seedPanelAt('/srv/chrote')
    const openInFiles = renderPanel()

    let readmeRow = await screen.findByRole('treeitem', { name: /README\.md/ })
    fireEvent.contextMenu(readmeRow, { clientX: 80, clientY: 120 })
    let menu = document.querySelector('.menu-sheet') as HTMLElement
    expect(within(menu).getAllByRole('menuitem').map(button => button.textContent)).toEqual([
      'Open',
      'Download',
      'Rename',
      'Pin',
      'Copy path',
      'Copy relative path',
      'Open parent folder',
      'Delete',
    ])

    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Open' }))
    expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))

    readmeRow = await screen.findByRole('treeitem', { name: /README\.md/ })
    fireEvent.contextMenu(readmeRow)
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Open parent folder' }))
    expect(openInFiles).toHaveBeenCalledWith('/srv/chrote')

    const docsRow = screen.getByRole('treeitem', { name: /Folder docs/ })
    fireEvent.contextMenu(docsRow)
    menu = document.querySelector('.menu-sheet') as HTMLElement
    expect(within(menu).getAllByRole('menuitem').map(button => button.textContent)).toEqual([
      'Open folder',
      'Rename',
      'Pin',
      'Copy path',
      'Copy relative path',
      'Open parent folder',
      'Delete',
    ])
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Open folder' }))
    await waitFor(() => expect(readWorkspaceFilesState('terminal1').currentPath).toBe('/srv/chrote/docs'))
    expect(openInFiles).toHaveBeenCalledTimes(1)
  })

  it('follows a renamed open file and forgets a deleted one', async () => {
    writeWorkspaceFilesState('terminal1', {
      ...DEFAULT_WORKSPACE_FILES_STATE,
      currentPath: '/srv/chrote',
      selectedPath: readme.path,
      expandedPaths: ['/srv/chrote'],
      openPath: readme.path,
    })
    renderPanel()

    fireEvent.click(await screen.findByRole('button', { name: 'Back' }))
    fireEvent.contextMenu(await screen.findByRole('treeitem', { name: /README\.md/ }))
    let menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Rename' }))
    fireEvent.change(screen.getByLabelText('New name'), { target: { value: 'GUIDE.md' } })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))
    await waitFor(() => expect(mockedRenameItem).toHaveBeenCalledWith(readme.path, '/srv/chrote/GUIDE.md'))

    fireEvent.contextMenu(await screen.findByRole('treeitem', { name: /README\.md/ }))
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Delete' }))
    const dialog = screen.getByRole('dialog', { name: 'Delete README.md' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(readWorkspaceFilesState('terminal1').openPath).toBeNull())
  })

  it('offers blank-tree actions and executes sidecar mutation primitives', async () => {
    seedPanelAt('/srv/chrote')
    renderPanel()
    await screen.findByRole('treeitem', { name: /README\.md/ })

    fireEvent.contextMenu(screen.getByRole('tree', { name: 'File tree' }), { clientX: 40, clientY: 90 })
    let menu = document.querySelector('.menu-sheet') as HTMLElement
    expect(within(menu).getAllByRole('menuitem').map(button => button.textContent)).toEqual([
      'New file',
      'New folder',
      'Upload',
      'Refresh',
      'Copy current folder path',
      'Pin current folder',
    ])

    fireEvent.click(within(menu).getByRole('menuitem', { name: 'New file' }))
    fireEvent.change(screen.getByLabelText('File name'), { target: { value: 'notes.txt' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => expect(mockedCreateFile).toHaveBeenCalledWith('/srv/chrote', 'notes.txt'))

    fireEvent.contextMenu(screen.getByRole('treeitem', { name: /README\.md/ }))
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Rename' }))
    fireEvent.change(screen.getByLabelText('New name'), { target: { value: 'GUIDE.md' } })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))
    await waitFor(() => expect(mockedRenameItem).toHaveBeenCalledWith('/srv/chrote/README.md', '/srv/chrote/GUIDE.md'))

    fireEvent.contextMenu(screen.getByRole('treeitem', { name: /README\.md/ }))
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Copy relative path' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('README.md')

    fireEvent.contextMenu(screen.getByRole('treeitem', { name: /README\.md/ }))
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Delete' }))
    const deleteDialog = screen.getByRole('dialog', { name: 'Delete README.md' })
    fireEvent.click(within(deleteDialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(mockedDeleteItem).toHaveBeenCalledWith('/srv/chrote/README.md'))

    fireEvent.contextMenu(screen.getByRole('tree', { name: 'File tree' }))
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Pin current folder' }))
    expect(JSON.parse(window.localStorage.getItem('chrote.files.pinnedPaths') || '[]')).toEqual([
      { path: '/srv/chrote', kind: 'directory' },
    ])

    fireEvent.contextMenu(screen.getByRole('tree', { name: 'File tree' }))
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Upload' }))
    const uploadInput = document.querySelector('.terminal-files-upload') as HTMLInputElement
    const upload = new File(['hello'], 'upload.txt', { type: 'text/plain' })
    fireEvent.change(uploadInput, { target: { files: [upload] } })
    await waitFor(() => expect(mockedUploadFiles).toHaveBeenCalledWith('/srv/chrote', [upload]))
  })

  it('updates a mounted Files tab from sidecar pin changes without duplicate storage entries', async () => {
    seedPanelAt('/srv/chrote')
    window.localStorage.setItem('chrote.files.pinnedPaths', JSON.stringify([
      { path: '/srv/chrote', kind: 'directory' },
      { path: '/srv//chrote/', kind: 'directory' },
    ]))
    render(
      <>
        <TerminalFilesPanel
          workspaceId="terminal1"
          collapsed={false}
          width={320}
          pinned={false}
          canPin
          panelId="terminal1-files-sidecar"
          onTogglePin={vi.fn()}
          onClose={vi.fn()}
          onWidthChange={vi.fn()}
          onOpenInFiles={vi.fn()}
        />
        <FilesView />
      </>,
    )

    expect(await screen.findByRole('button', { name: /Pinned.*1/ })).toBeInTheDocument()
    const sidecarTree = screen.getAllByRole('tree', { name: 'File tree' })[0]
    fireEvent.contextMenu(sidecarTree)
    let menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Unpin current folder' }))
    await waitFor(() => expect(screen.queryByRole('button', { name: /Pinned/ })).not.toBeInTheDocument())

    fireEvent.contextMenu(sidecarTree)
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Pin current folder' }))
    expect(await screen.findByRole('button', { name: /Pinned.*1/ })).toBeInTheDocument()
  })
})
