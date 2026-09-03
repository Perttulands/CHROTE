import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView'
import {
  createFile,
  createFolder,
  deleteItem,
  fetchDirectory,
  readTextFile,
  renameItem,
  uploadFiles,
} from './FilesView/fileService'
import TerminalFilesPanel from './TerminalFilesPanel'
import {
  DEFAULT_FILE_VIEW_STATE,
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
    readTextFile: vi.fn(),
    renameItem: vi.fn(),
    uploadFiles: vi.fn(),
    getDownloadUrl: (path: string) => `/api/files/raw${path}`,
  }
})

const sessionMocks = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
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

const mockedFetchDirectory = vi.mocked(fetchDirectory)
const mockedReadTextFile = vi.mocked(readTextFile)
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

async function navigatePanel(path: string) {
  fireEvent.change(screen.getByLabelText('Files panel path'), { target: { value: path } })
  fireEvent.submit(screen.getByLabelText('Files panel path form'))
  const normalized = path.replace(/\/{2,}/g, '/').replace(/\/$/, '') || '/'
  await waitFor(() => expect(screen.getByLabelText('Files panel path')).toHaveValue(normalized))
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

  it('opens one non-modal Peek and hands a path to the focused workspace session', async () => {
    const openInFiles = vi.fn()
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
        onOpenInFiles={openInFiles}
      />,
    )

    fireEvent.change(screen.getByLabelText('Files panel path'), { target: { value: '/srv/chrote' } })
    fireEvent.submit(screen.getByLabelText('Files panel path form'))
    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))

    const peek = await screen.findByRole('dialog', { name: 'File Peek: README.md' })
    expect(peek).toHaveAttribute('aria-modal', 'false')
    expect(await screen.findByRole('heading', { name: 'CHROTE' })).toBeInTheDocument()
    expect(screen.getByText(/Send target:.*shell/)).toBeInTheDocument()

    const initialWidth = Number.parseFloat(peek.style.width)
    fireEvent.keyDown(screen.getByRole('separator', { name: 'Resize File Peek' }), { key: 'ArrowRight' })
    await waitFor(() => expect(Number.parseFloat(peek.style.width)).toBeGreaterThan(initialWidth))

    fireEvent.click(screen.getByRole('button', { name: 'Send README.md to session' }))
    expect(sessionMocks.openSendToSession).toHaveBeenCalledWith(
      'alice:shell',
      expect.stringContaining('/srv/chrote/README.md'),
    )

    fireEvent.click(screen.getByRole('button', { name: 'Open README.md in Files tab' }))
    expect(openInFiles).toHaveBeenCalledWith('/srv/chrote/README.md')
  })

  it('retains tree state when Peek closes and exposes sidecar pin and close controls', async () => {
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
    await screen.findByRole('dialog', { name: /File Peek/ })
    fireEvent.click(screen.getByRole('button', { name: 'Close File Peek' }))

    expect(screen.queryByRole('dialog', { name: /File Peek/ })).not.toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: /README\.md/ })).toHaveAttribute('aria-selected', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Pin Files sidecar' }))
    expect(togglePin).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: 'Close Files sidecar' }))
    expect(close).toHaveBeenCalledOnce()
    await waitFor(() => {
      const stored = JSON.parse(window.localStorage.getItem('chrote.workspaceFiles.v1') || '{}')
      expect(stored.version).toBe(1)
      expect(stored.workspaces.terminal1.selectedPath).toBe('/README.md')
      expect(stored.workspaces.terminal1.peek).toBeNull()
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

    await waitFor(() => expect(screen.getByLabelText('Files panel path')).toHaveValue('/srv/chrote'))
  })

  it('invalidates the FileTree cache when Refresh is clicked', async () => {
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
        onOpenInFiles={vi.fn()}
      />,
    )

    await screen.findByRole('treeitem', { name: /README\.md/ })
    expect(mockedFetchDirectory).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: 'Refresh Files' }))

    await waitFor(() => expect(mockedFetchDirectory).toHaveBeenCalledTimes(2))
  })

  it('navigates one normalized parent from an accessible header control and no-ops at root', async () => {
    renderPanel()

    const up = screen.getByRole('button', { name: 'Go to parent folder' })
    expect(up).toBeDisabled()
    fireEvent.click(up)
    expect(screen.getByLabelText('Files panel path')).toHaveValue('/')

    await navigatePanel('/srv//chrote/')
    expect(up).toBeEnabled()
    fireEvent.click(up)

    await waitFor(() => expect(screen.getByLabelText('Files panel path')).toHaveValue('/srv'))
    expect(mockedFetchDirectory).toHaveBeenCalledWith('/srv')
  })

  it('offers the exact single-item actions and preserves sidecar/global routing', async () => {
    const openInFiles = renderPanel()
    await navigatePanel('/srv/chrote')

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
    expect(await screen.findByRole('dialog', { name: 'File Peek: README.md' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close File Peek' }))

    readmeRow = screen.getByRole('treeitem', { name: /README\.md/ })
    fireEvent.contextMenu(readmeRow)
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Open parent folder' }))
    expect(openInFiles).toHaveBeenCalledWith('/srv/chrote')
    expect(screen.getByLabelText('Files panel path')).toHaveValue('/srv/chrote')

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
    await waitFor(() => expect(screen.getByLabelText('Files panel path')).toHaveValue('/srv/chrote/docs'))
    expect(openInFiles).toHaveBeenCalledTimes(1)
  })

  it('remaps persisted Peek view state with a terminal-sidecar rename', async () => {
    const viewState = { ...DEFAULT_FILE_VIEW_STATE, scrollTop: 144, fontSize: 18 }
    writeWorkspaceFilesState('terminal1', {
      ...DEFAULT_WORKSPACE_FILES_STATE,
      currentPath: '/srv/chrote',
      selectedPath: readme.path,
      expandedPaths: ['/srv/chrote'],
      peek: { path: readme.path, name: readme.name, size: readme.size, type: readme.type, x: 40, y: 40, width: 480, height: 360 },
      fileViewStates: { [readme.path]: viewState },
    })
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('treeitem', { name: /README\.md/ }))
    const menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Rename' }))
    fireEvent.change(screen.getByLabelText('New name'), { target: { value: 'GUIDE.md' } })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => {
      const persisted = readWorkspaceFilesState('terminal1')
      expect(persisted.peek?.path).toBe('/srv/chrote/GUIDE.md')
      expect(persisted.fileViewStates['/srv/chrote/GUIDE.md']).toEqual(viewState)
      expect(persisted.fileViewStates[readme.path]).toBeUndefined()
    })
  })

  it('prunes persisted Peek view state with a terminal-sidecar delete', async () => {
    const viewState = { ...DEFAULT_FILE_VIEW_STATE, scrollTop: 144, fontSize: 18 }
    writeWorkspaceFilesState('terminal1', {
      ...DEFAULT_WORKSPACE_FILES_STATE,
      currentPath: '/srv/chrote',
      selectedPath: readme.path,
      expandedPaths: ['/srv/chrote'],
      peek: { path: readme.path, name: readme.name, size: readme.size, type: readme.type, x: 40, y: 40, width: 480, height: 360 },
      fileViewStates: { [readme.path]: viewState },
    })
    renderPanel()

    fireEvent.contextMenu(await screen.findByRole('treeitem', { name: /README\.md/ }))
    const menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Delete' }))
    const dialog = screen.getByRole('dialog', { name: 'Delete README.md' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      const persisted = readWorkspaceFilesState('terminal1')
      expect(persisted.peek).toBeNull()
      expect(persisted.fileViewStates[readme.path]).toBeUndefined()
    })
  })

  it('offers blank-tree actions and executes sidecar mutation primitives', async () => {
    renderPanel()
    await navigatePanel('/srv/chrote')

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
    await navigatePanel('/srv/chrote')
    const sidecarTree = screen.getAllByRole('tree', { name: 'File tree' })[0]
    fireEvent.contextMenu(sidecarTree)
    let menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Unpin current folder' }))
    await waitFor(() => expect(screen.queryByRole('button', { name: /Pinned/ })).not.toBeInTheDocument())

    fireEvent.contextMenu(sidecarTree)
    menu = document.querySelector('.menu-sheet') as HTMLElement
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'Pin current folder' }))
    expect(await screen.findByRole('button', { name: /Pinned.*1/ })).toBeInTheDocument()
    expect(JSON.parse(window.localStorage.getItem('chrote.files.pinnedPaths') || '[]')).toEqual([
      { path: '/srv/chrote', kind: 'directory' },
    ])
  })
})
