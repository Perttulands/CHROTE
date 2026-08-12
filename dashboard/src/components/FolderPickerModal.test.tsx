import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchDirectory } from './FilesView/fileService'
import FolderPickerModal from './FolderPickerModal'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return { ...actual, fetchDirectory: vi.fn() }
})

const mockedFetchDirectory = vi.mocked(fetchDirectory)

const directory = (path: string, isDir: boolean) => ({
  path,
  name: path.split('/').pop() || '/',
  isDir,
  size: isDir ? 0 : 42,
  modified: '2026-01-01T00:00:00Z',
  type: isDir ? '' : 'text/plain',
})

describe('FolderPickerModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockedFetchDirectory.mockImplementation(async path => {
      if (path === '/') return [directory('/repo', true), directory('/README.md', false)]
      return []
    })
    vi.stubGlobal('fetch', vi.fn(async input => ({
      ok: true,
      json: async () => ({ isDir: String(input).endsWith('/.beads') }),
    })))
  })

  it('loads Beads folders, navigates them, and selects the current project path', async () => {
    const onSelect = vi.fn()
    const onClose = vi.fn()

    render(<FolderPickerModal onSelect={onSelect} onClose={onClose} />)

    expect(await screen.findByText('repo')).toBeInTheDocument()
    expect(screen.getAllByText('.beads')).toHaveLength(2)
    expect(screen.getByRole('button', { name: 'Select This Folder' })).toBeEnabled()

    fireEvent.click(screen.getByText('repo'))
    await waitFor(() => expect(screen.getByText('No subfolders')).toBeInTheDocument())
    expect(screen.getByTitle('Go up')).toBeEnabled()

    fireEvent.click(screen.getByTitle('Go up'))
    await waitFor(() => expect(mockedFetchDirectory).toHaveBeenLastCalledWith('/'))
    fireEvent.click(screen.getByRole('button', { name: 'Select This Folder' }))

    expect(onSelect).toHaveBeenCalledWith('/')
    expect(onClose).not.toHaveBeenCalled()
  })
})
