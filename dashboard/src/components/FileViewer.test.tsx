import { useLayoutEffect, useRef } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { probeTextFile, readTextFile } from './FilesView/fileService'
import FileViewer, { MAX_TEXT_PREVIEW_BYTES, getPreviewKind } from './FileViewer'
import { DEFAULT_FILE_VIEW_STATE } from './workspaceFilesState'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return {
    ...actual,
    probeTextFile: vi.fn(),
    readTextFile: vi.fn(),
    getDownloadUrl: (path: string) => `/api/files/raw${path}`,
  }
})

const mockedReadTextFile = vi.mocked(readTextFile)
const mockedProbeTextFile = vi.mocked(probeTextFile)
const markdownFile = {
  path: '/README.md',
  name: 'README.md',
  isDir: false,
  size: 40,
  modified: '2026-01-01T00:00:00Z',
  type: 'text/markdown',
}

function LayoutSnapshotViewer({
  item,
  onLayout,
}: {
  item: typeof markdownFile
  onLayout: (text: string) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    onLayout(containerRef.current?.textContent || '')
  }, [item.path, onLayout])
  return (
    <div ref={containerRef}>
      <FileViewer
        item={item}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={vi.fn()}
      />
    </div>
  )
}

describe('FileViewer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockedProbeTextFile.mockResolvedValue(null)
    mockedReadTextFile.mockResolvedValue('# Hello\n\nUseful text')
  })

  it.each([
    'events.jsonl',
    'records.ndjson',
    'settings.jsonc',
    'schema.graphql',
    'messages.proto',
    'changes.patch',
    'notebook.ipynb',
    'network.har',
    'map.geojson',
    'state.tfstate',
    '.gitignore',
    'service.timer',
  ])('recognizes %s as previewable text', name => {
    expect(getPreviewKind({ ...markdownFile, path: `/${name}`, name, type: name.split('.').pop() || '' })).toBe('text')
  })

  it.each([
    ['photo.avif', 'image'],
    ['favicon.ico', 'image'],
    ['voice.opus', 'audio'],
    ['clip.m4v', 'video'],
    ['movie.ogv', 'video'],
  ] as const)('recognizes %s as a browser-native %s preview', (name, expected) => {
    expect(getPreviewKind({ ...markdownFile, path: `/${name}`, name, type: name.split('.').pop() || '' })).toBe(expected)
  })

  it.each([
    ['go.mod', 'text'],
    ['go.sum', 'text'],
    ['go.work', 'text'],
    ['music.mod', 'download'],
    ['publisher.pub', 'download'],
    ['tiles.map', 'download'],
    ['transport.mts', 'download'],
    ['game.vb', 'download'],
    ['flash-cookie.sol', 'download'],
    ['notability.note', 'download'],
  ] as const)('classifies ambiguous format %s as %s before content probing', (name, expected) => {
    expect(getPreviewKind({ ...markdownFile, path: `/${name}`, name, type: name.split('.').pop() || '' })).toBe(expected)
  })

  it('probes and previews a small UTF-8 file with an unknown extension', async () => {
    mockedProbeTextFile.mockResolvedValueOnce('custom format content')
    render(
      <FileViewer
        item={{ ...markdownFile, path: '/events.records', name: 'events.records', type: 'records' }}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={vi.fn()}
      />,
    )

    expect(await screen.findByText('custom format content')).toBeInTheDocument()
    expect(mockedProbeTextFile).toHaveBeenCalledWith('/events.records', MAX_TEXT_PREVIEW_BYTES)
  })

  it('never renders probed content under a different file path', async () => {
    mockedProbeTextFile
      .mockResolvedValueOnce('first file content')
      .mockReturnValueOnce(new Promise(() => {}))
    const onLayout = vi.fn()
    const { rerender } = render(
      <LayoutSnapshotViewer
        item={{ ...markdownFile, path: '/first.records', name: 'first.records', type: 'records' }}
        onLayout={onLayout}
      />,
    )
    expect(await screen.findByText('first file content')).toBeInTheDocument()

    onLayout.mockClear()
    rerender(
      <LayoutSnapshotViewer
        item={{ ...markdownFile, path: '/second.records', name: 'second.records', type: 'records' }}
        onLayout={onLayout}
      />,
    )

    const pathSwitchSnapshot = onLayout.mock.calls[onLayout.mock.calls.length - 1]?.[0]
    expect(pathSwitchSnapshot).not.toContain('first file content')
  })

  it('keeps a probed binary file download-only', async () => {
    render(
      <FileViewer
        item={{ ...markdownFile, path: '/artifact.unknown', name: 'artifact.unknown', type: 'unknown' }}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={vi.fn()}
      />,
    )

    expect(await screen.findByText('No inline preview is available for this file type.')).toBeInTheDocument()
    expect(mockedProbeTextFile).toHaveBeenCalledWith('/artifact.unknown', MAX_TEXT_PREVIEW_BYTES)
  })

  it('does not probe an unknown file beyond the text preview limit', async () => {
    render(
      <FileViewer
        item={{
          ...markdownFile,
          path: '/archive.payload',
          name: 'archive.payload',
          type: 'payload',
          size: MAX_TEXT_PREVIEW_BYTES + 1,
        }}
        viewState={DEFAULT_FILE_VIEW_STATE}
        onViewStateChange={vi.fn()}
      />,
    )

    expect(await screen.findByText('No inline preview is available for this file type.')).toBeInTheDocument()
    expect(mockedProbeTextFile).not.toHaveBeenCalled()
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

    const split = await screen.findByLabelText(`Markdown viewer for ${markdownFile.name}`)
    vi.spyOn(split, 'getBoundingClientRect').mockReturnValue({ width: 320 } as DOMRect)
    const separator = screen.getByRole('separator', { name: 'Resize Markdown split' })
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
