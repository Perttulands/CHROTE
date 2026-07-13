import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchDirectory, readTextFile } from './FilesView/fileService'
import TerminalFilesPanel from './TerminalFilesPanel'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return {
    ...actual,
    fetchDirectory: vi.fn(),
    readTextFile: vi.fn(),
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
    sessions: [{ name: 'shell', unixUser: 'alice', windows: 1, attached: true, group: 'shell' }],
    sessionBank: [{ name: 'shell', unixUser: 'alice', cwd: '/srv/chrote', live: true }],
    openSendToSession: sessionMocks.openSendToSession,
  }),
}))

const mockedFetchDirectory = vi.mocked(fetchDirectory)
const mockedReadTextFile = vi.mocked(readTextFile)

const readme = {
  path: '/srv/chrote/README.md',
  name: 'README.md',
  isDir: false,
  size: 40,
  modified: '2026-01-01T00:00:00Z',
  type: 'text/markdown',
}

describe('TerminalFilesPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    mockedFetchDirectory.mockImplementation(async path => {
      if (path === '/') return [{ ...readme, path: '/README.md' }]
      if (path === '/srv/chrote') return [readme]
      return []
    })
    mockedReadTextFile.mockResolvedValue('# CHROTE')
  })

  it('opens one non-modal Peek and hands a path to the focused workspace session', async () => {
    const openInFiles = vi.fn()
    render(
      <TerminalFilesPanel
        workspaceId="terminal1"
        collapsed={false}
        width={320}
        onToggle={vi.fn()}
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

  it('retains tree state when Peek closes and exposes its own labeled collapse control', async () => {
    const toggle = vi.fn()
    render(
      <TerminalFilesPanel
        workspaceId="terminal1"
        collapsed={false}
        width={320}
        onToggle={toggle}
        onWidthChange={vi.fn()}
        onOpenInFiles={vi.fn()}
      />,
    )

    fireEvent.click(await screen.findByRole('treeitem', { name: /README\.md/ }))
    await screen.findByRole('dialog', { name: /File Peek/ })
    fireEvent.click(screen.getByRole('button', { name: 'Close File Peek' }))

    expect(screen.queryByRole('dialog', { name: /File Peek/ })).not.toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: /README\.md/ })).toHaveAttribute('aria-selected', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Collapse Files panel' }))
    expect(toggle).toHaveBeenCalledOnce()
    await waitFor(() => {
      const stored = JSON.parse(window.localStorage.getItem('chrote.workspaceFiles.v1') || '{}')
      expect(stored.version).toBe(1)
      expect(stored.workspaces.terminal1.selectedPath).toBe('/README.md')
      expect(stored.workspaces.terminal1.peek).toBeNull()
    })
  })
})
