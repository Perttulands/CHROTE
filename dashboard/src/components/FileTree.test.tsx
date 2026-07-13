import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchDirectory } from './FilesView/fileService'
import FileTree from './FileTree'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return { ...actual, fetchDirectory: vi.fn() }
})

const mockedFetchDirectory = vi.mocked(fetchDirectory)
const item = (path: string, isDir: boolean) => ({
  path,
  name: path.split('/').pop() || '/',
  isDir,
  size: isDir ? 0 : 42,
  modified: '2026-01-01T00:00:00Z',
  type: isDir ? '' : 'text/plain',
})

describe('FileTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockedFetchDirectory.mockImplementation(async path => {
      if (path === '/') return [item('/srv', true), item('/README.md', false)]
      if (path === '/srv') return [item('/srv/chrote', true), item('/srv/notes.txt', false)]
      return []
    })
  })

  it('renders folders and files, lazily expands folders, and opens files', async () => {
    const openFile = vi.fn()
    const openDirectory = vi.fn()
    const expandedChange = vi.fn()

    render(
      <FileTree
        currentPath="/"
        selectedPath={null}
        expandedPaths={['/']}
        scrollTop={0}
        onOpenDirectory={openDirectory}
        onOpenFile={openFile}
        onExpandedPathsChange={expandedChange}
        onScrollTopChange={vi.fn()}
      />,
    )

    const readme = await screen.findByRole('treeitem', { name: /README\.md/ })
    expect(screen.getByRole('treeitem', { name: /srv/ })).toBeInTheDocument()

    fireEvent.click(readme)
    expect(openFile).toHaveBeenCalledWith(expect.objectContaining({ path: '/README.md', isDir: false }))

    fireEvent.click(screen.getByRole('button', { name: 'Expand /srv' }))
    expect(expandedChange).toHaveBeenCalledWith(['/', '/srv'])
    expect(await screen.findByRole('treeitem', { name: /notes\.txt/ })).toBeInTheDocument()
    expect(mockedFetchDirectory).toHaveBeenCalledWith('/srv')
  })

  it('restores and reports tree scroll without resetting it on selection changes', async () => {
    const onScrollTopChange = vi.fn()
    const { rerender } = render(
      <FileTree
        currentPath="/"
        selectedPath="/README.md"
        expandedPaths={['/']}
        scrollTop={180}
        onOpenDirectory={vi.fn()}
        onOpenFile={vi.fn()}
        onExpandedPathsChange={vi.fn()}
        onScrollTopChange={onScrollTopChange}
      />,
    )

    const tree = await screen.findByRole('tree', { name: 'File tree' })
    expect(tree).toHaveProperty('scrollTop', 180)

    rerender(
      <FileTree
        currentPath="/srv"
        selectedPath="/srv/notes.txt"
        expandedPaths={['/', '/srv']}
        scrollTop={180}
        onOpenDirectory={vi.fn()}
        onOpenFile={vi.fn()}
        onExpandedPathsChange={vi.fn()}
        onScrollTopChange={onScrollTopChange}
      />,
    )

    await waitFor(() => expect(tree.scrollTop).toBe(180))
    tree.scrollTop = 260
    fireEvent.scroll(tree)
    expect(onScrollTopChange).toHaveBeenLastCalledWith(260)
  })

  it('refreshes every expanded directory instead of only the tree root', async () => {
    const props = {
      currentPath: '/srv',
      selectedPath: null,
      expandedPaths: ['/', '/srv'],
      scrollTop: 0,
      onOpenDirectory: vi.fn(),
      onOpenFile: vi.fn(),
      onExpandedPathsChange: vi.fn(),
      onScrollTopChange: vi.fn(),
    }
    const { rerender } = render(<FileTree {...props} refreshToken={0} />)

    expect(await screen.findByRole('treeitem', { name: /notes\.txt/ })).toBeInTheDocument()
    mockedFetchDirectory.mockImplementation(async path => {
      if (path === '/') return [item('/srv', true), item('/README.md', false)]
      if (path === '/srv') return [item('/srv/fresh.txt', false)]
      return []
    })

    rerender(<FileTree {...props} refreshToken={1} />)

    expect(await screen.findByRole('treeitem', { name: /fresh\.txt/ })).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByRole('treeitem', { name: /notes\.txt/ })).not.toBeInTheDocument())
    expect(mockedFetchDirectory.mock.calls.filter(([path]) => path === '/srv')).toHaveLength(2)
  })
})
