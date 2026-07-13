import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readTextFile } from './FilesView/fileService'
import FileViewer from './FileViewer'
import { DEFAULT_FILE_VIEW_STATE } from './workspaceFilesState'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return {
    ...actual,
    readTextFile: vi.fn(),
    getDownloadUrl: (path: string) => `/api/files/raw${path}`,
  }
})

const mockedReadTextFile = vi.mocked(readTextFile)
const markdownFile = {
  path: '/README.md',
  name: 'README.md',
  isDir: false,
  size: 40,
  modified: '2026-01-01T00:00:00Z',
  type: 'text/markdown',
}

describe('FileViewer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockedReadTextFile.mockResolvedValue('# Hello\n\nUseful text')
  })

  it('loads a read-only Markdown file and offers Preview, Source, and Split modes', async () => {
    const onViewStateChange = vi.fn()
    render(
      <FileViewer
        item={markdownFile}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={onViewStateChange}
      />,
    )

    expect(await screen.findByRole('heading', { name: 'Hello' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Preview Markdown' })).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Show Markdown source' }))
    expect(onViewStateChange).toHaveBeenLastCalledWith(expect.objectContaining({ markdownMode: 'source' }))
  })

  it('supports keyboard adjustment of the persisted Markdown split', async () => {
    const onViewStateChange = vi.fn()
    render(
      <FileViewer
        item={markdownFile}
        viewState={{ ...DEFAULT_FILE_VIEW_STATE, markdownMode: 'split', markdownSplitPercent: 50 }}
        onViewStateChange={onViewStateChange}
      />,
    )

    const separator = await screen.findByRole('separator', { name: 'Resize Markdown split' })
    fireEvent.keyDown(separator, { key: 'ArrowRight' })
    expect(onViewStateChange).toHaveBeenLastCalledWith(expect.objectContaining({ markdownSplitPercent: 55 }))
  })

  it('restores content scroll and reports changes without reloading the file', async () => {
    const onViewStateChange = vi.fn()
    const { rerender } = render(
      <FileViewer
        item={markdownFile}
        viewState={{ ...DEFAULT_FILE_VIEW_STATE, scrollTop: 180 }}
        onViewStateChange={onViewStateChange}
      />,
    )

    const viewer = await screen.findByTestId('file-viewer-scroll')
    await waitFor(() => expect(viewer.scrollTop).toBe(180))

    viewer.scrollTop = 260
    fireEvent.scroll(viewer)
    expect(onViewStateChange).toHaveBeenLastCalledWith(expect.objectContaining({ scrollTop: 260 }))

    rerender(
      <FileViewer
        item={markdownFile}
        viewState={{ ...DEFAULT_FILE_VIEW_STATE, scrollTop: 260 }}
        onViewStateChange={onViewStateChange}
      />,
    )
    expect(mockedReadTextFile).toHaveBeenCalledTimes(1)
  })

  it('clears pending text state when the reused viewer switches to an image', async () => {
    mockedReadTextFile.mockImplementation(() => new Promise(() => {}))
    const onViewStateChange = vi.fn()
    const { rerender } = render(
      <FileViewer
        item={markdownFile}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={onViewStateChange}
      />,
    )

    expect(await screen.findByText('Loading file...')).toBeInTheDocument()
    rerender(
      <FileViewer
        item={{ ...markdownFile, path: '/photo.png', name: 'photo.png', type: 'image/png' }}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={onViewStateChange}
      />,
    )

    expect(await screen.findByRole('img', { name: 'photo.png' })).toBeInTheDocument()
    expect(screen.queryByText('Loading file...')).not.toBeInTheDocument()
  })

  it('clears a text read error when the reused viewer switches to an image', async () => {
    mockedReadTextFile.mockRejectedValueOnce(new Error('read denied'))
    const onViewStateChange = vi.fn()
    const { rerender } = render(
      <FileViewer
        item={markdownFile}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={onViewStateChange}
      />,
    )

    expect(await screen.findByText('read denied')).toBeInTheDocument()
    rerender(
      <FileViewer
        item={{ ...markdownFile, path: '/photo.png', name: 'photo.png', type: 'image/png' }}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={onViewStateChange}
      />,
    )

    expect(await screen.findByRole('img', { name: 'photo.png' })).toBeInTheDocument()
    expect(screen.queryByText('read denied')).not.toBeInTheDocument()
  })
})
