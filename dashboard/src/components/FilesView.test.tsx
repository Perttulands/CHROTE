import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView'
import { fetchDirectory } from './FilesView/fileService'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return {
    ...actual,
    fetchDirectory: vi.fn(),
    createFile: vi.fn(),
    createFolder: vi.fn(),
    deleteItem: vi.fn(),
    readTextFile: vi.fn(),
    renameItem: vi.fn(),
    uploadFiles: vi.fn(),
    writeTextFile: vi.fn(),
    getDownloadUrl: (path: string) => `/api/files/raw${path}`,
  }
})

const mockFetchDirectory = vi.mocked(fetchDirectory)

beforeEach(() => {
  vi.clearAllMocks()
  window.localStorage.clear()
  Object.assign(navigator, {
    clipboard: {
      writeText: vi.fn().mockResolvedValue(undefined),
    },
  })
  mockFetchDirectory.mockResolvedValue([])
})

describe('FilesView saved path groups', () => {
  it('renders multiple pinned paths and collapses the pinned group independently', async () => {
    window.localStorage.setItem('chrote.files.pinnedPaths', JSON.stringify([
      { path: '/home/perttu', kind: 'directory' },
      { path: '/srv', kind: 'directory' },
    ]))
    window.localStorage.setItem('chrote.files.recentPaths', JSON.stringify([
      { path: '/etc/hosts', kind: 'file' },
    ]))

    render(<FilesView />)

    const pinnedToggle = await screen.findByRole('button', { name: /Pinned.*2/ })
    expect(pinnedToggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: /perttu/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /srv/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Recent.*1/ })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: /hosts/ })).toBeInTheDocument()

    fireEvent.click(pinnedToggle)

    expect(screen.getByRole('button', { name: /Pinned.*2/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('button', { name: /perttu/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /srv/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /hosts/ })).toBeInTheDocument()
    expect(JSON.parse(window.localStorage.getItem('chrote.files.savedGroupsCollapsed') || '{}')).toEqual({
      pinned: true,
      recent: false,
    })
  })

  it('restores collapsed Recent state from localStorage without collapsing Pinned', async () => {
    window.localStorage.setItem('chrote.files.pinnedPaths', JSON.stringify([
      { path: '/home/perttu', kind: 'directory' },
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
    expect(screen.getByRole('button', { name: /perttu/ })).toBeInTheDocument()
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
    const menu = document.querySelector('.fb-context-menu') as HTMLElement

    expect(within(menu).getByRole('button', { name: 'New File' })).not.toBeDisabled()
    expect(within(menu).getByRole('button', { name: 'New Folder' })).not.toBeDisabled()
    expect(within(menu).getByRole('button', { name: 'Upload' })).not.toBeDisabled()
    expect(within(menu).getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
    expect(within(menu).getByRole('button', { name: 'Copy Current Folder Path' })).toBeInTheDocument()
    expect(within(menu).getByRole('button', { name: 'Pin Current Folder' })).toBeInTheDocument()

    fireEvent.click(within(menu).getByRole('button', { name: 'Copy Current Folder Path' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('/')
  })

  it('copies selected paths, relative path, and opens a selected item parent from item menus', async () => {
    render(<FilesView />)
    const hostsRow = await screen.findByRole('row', { name: /hosts/ })

    fireEvent.contextMenu(hostsRow)
    const menu = document.querySelector('.fb-context-menu') as HTMLElement

    fireEvent.click(within(menu).getByRole('button', { name: 'Copy Selected Path(s)' }))
    expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith('/etc/hosts')

    fireEvent.contextMenu(hostsRow)
    const secondMenu = document.querySelector('.fb-context-menu') as HTMLElement
    fireEvent.click(within(secondMenu).getByRole('button', { name: 'Copy Relative Path' }))
    expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith('etc/hosts')

    fireEvent.contextMenu(hostsRow)
    const thirdMenu = document.querySelector('.fb-context-menu') as HTMLElement
    fireEvent.click(within(thirdMenu).getByRole('button', { name: 'Open Parent Folder' }))
    await waitFor(() => expect(mockFetchDirectory).toHaveBeenCalledWith('/etc'))
  })
})
