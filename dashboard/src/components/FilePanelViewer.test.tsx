import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FilePanelViewer, { parseUnifiedDiff, prettyJson } from './FilePanelViewer'
import { fetchFileDiff } from './FilesView/fileService'
import { resetImageZoomForTest, setImageZoom } from './imageZoom'

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return {
    ...actual,
    fetchFileDiff: vi.fn(),
    getDownloadUrl: (path: string) => `/api/files/raw${path}`,
  }
})

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: vi.fn() }),
}))

describe('parseUnifiedDiff', () => {
  it('drops the file headers and signs every changed line in the gutter', () => {
    const rows = parseUnifiedDiff([
      'diff --git a/docs/journeys.md b/docs/journeys.md',
      'index 1111111..2222222 100644',
      '--- a/docs/journeys.md',
      '+++ b/docs/journeys.md',
      '@@ -1,3 +1,3 @@',
      ' kept',
      '-gone',
      '+added',
      '',
    ].join('\n'))

    expect(rows).toEqual([
      { kind: 'hunk', gutter: '', text: '@@ -1,3 +1,3 @@' },
      { kind: 'context', gutter: '', text: 'kept' },
      { kind: 'del', gutter: '-', text: 'gone' },
      { kind: 'add', gutter: '+', text: 'added' },
    ])
  })

  it('keeps a second hunk and the no-newline marker', () => {
    const rows = parseUnifiedDiff('@@ -1 +1 @@\n-a\n@@ -9 +9 @@\n+b\n\\ No newline at end of file\n')

    expect(rows.map(row => row.kind)).toEqual(['hunk', 'del', 'hunk', 'add', 'context'])
    expect(rows[4].text).toBe('\\ No newline at end of file')
  })

  it.each(['', 'diff --git a/x b/x\nindex 1..2 100644\n'])('reads a diff with no hunk as no rows', diff => {
    expect(parseUnifiedDiff(diff)).toEqual([])
  })
})

describe('prettyJson', () => {
  it('pretty-prints what parses', () => {
    expect(prettyJson('{"a":1,"b":[2,3]}')).toBe('{\n  "a": 1,\n  "b": [\n    2,\n    3\n  ]\n}')
  })

  it('shows the bytes on disk when the file is not valid JSON', () => {
    expect(prettyJson('{not json')).toBe('{not json')
  })
})

/**
 * Pictures and videos in the panel, the table column and the pop-out
 * (bead: chrote-jtvh). jsdom has no layout, so the room the viewer measures
 * is given here, and so is the ResizeObserver that tells it the room changed.
 */
describe('FilePanelViewer media', () => {
  let room = { width: 400, height: 300 }
  let resized: (() => void) | null = null

  beforeEach(() => {
    room = { width: 400, height: 300 }
    vi.mocked(fetchFileDiff).mockResolvedValue({ path: '', repository: '', diff: '', truncated: false })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
      () => ({ x: 0, y: 0, top: 0, left: 0, right: room.width, bottom: room.height, ...room, toJSON: () => ({}) }),
    )
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { resized = callback }
      observe() {}
      disconnect() {}
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetImageZoomForTest()
    resized = null
  })

  function open(path: string) {
    return render(
      <FilePanelViewer path={path} onClose={() => {}} onOpenPath={() => {}} onSend={null} pictureOpensGlance={false} />,
    )
  }

  function loadPicture(picture: HTMLElement, width: number, height: number) {
    Object.defineProperty(picture, 'naturalWidth', { value: width, configurable: true })
    Object.defineProperty(picture, 'naturalHeight', { value: height, configurable: true })
    fireEvent.load(picture)
  }

  function drawn(element: HTMLElement) {
    return { width: element.style.width, height: element.style.height }
  }

  it('fits a tall picture and a wide one whole inside the room, and keeps a small one at 1:1', () => {
    const { rerender } = open('/tmp/tall.png')
    const tall = screen.getByRole('img', { name: 'tall.png' })
    loadPicture(tall, 300, 1200)
    expect(drawn(tall)).toEqual({ width: '75px', height: '300px' })

    rerender(<FilePanelViewer path="/tmp/wide.png" onClose={() => {}} onOpenPath={() => {}} onSend={null} pictureOpensGlance={false} />)
    const wide = screen.getByRole('img', { name: 'wide.png' })
    loadPicture(wide, 1600, 400)
    expect(drawn(wide)).toEqual({ width: '400px', height: '100px' })

    rerender(<FilePanelViewer path="/tmp/small.png" onClose={() => {}} onOpenPath={() => {}} onSend={null} pictureOpensGlance={false} />)
    const small = screen.getByRole('img', { name: 'small.png' })
    loadPicture(small, 40, 30)
    expect(drawn(small)).toEqual({ width: '40px', height: '30px' })
    expect(screen.getByText('40 × 30')).toBeInTheDocument()
  })

  it('refits when the room changes size', () => {
    open('/tmp/tall.png')
    const picture = screen.getByRole('img', { name: 'tall.png' })
    loadPicture(picture, 300, 1200)

    room = { width: 400, height: 600 }
    act(() => resized?.())

    expect(drawn(picture)).toEqual({ width: '150px', height: '600px' })
  })

  it('draws a picture at a percentage zoom literally, bigger than the room', () => {
    act(() => setImageZoom({ kind: 'percent', percent: 200 }))
    open('/tmp/tall.png')
    const picture = screen.getByRole('img', { name: 'tall.png' })
    loadPicture(picture, 300, 1200)

    expect(drawn(picture)).toEqual({ width: '600px', height: '2400px' })
  })

  it('plays a video with native controls, fitted to the room at any zoom level, with its size beneath', () => {
    act(() => setImageZoom({ kind: 'percent', percent: 200 }))
    const { container } = open('/tmp/clip.mp4')
    const video = container.querySelector('video')!
    expect(video).toHaveAttribute('src', '/api/files/raw/tmp/clip.mp4')
    expect(video.controls).toBe(true)
    expect(video.autoplay).toBe(false)

    Object.defineProperty(video, 'videoWidth', { value: 1920, configurable: true })
    Object.defineProperty(video, 'videoHeight', { value: 1080, configurable: true })
    fireEvent.loadedMetadata(video)

    expect(drawn(video)).toEqual({ width: '400px', height: '225px' })
    expect(screen.getByText('1920 × 1080')).toBeInTheDocument()
  })

  it('plays a sound with native audio controls', () => {
    const { container } = open('/tmp/voice.mp3')
    const audio = container.querySelector('audio')!
    expect(audio).toHaveAttribute('src', '/api/files/raw/tmp/voice.mp3')
    expect(audio.controls).toBe(true)
  })

  it('offers a video the browser cannot decode as a download instead of a broken player', () => {
    const { container } = open('/tmp/old.avi')
    fireEvent.error(container.querySelector('video')!)

    expect(screen.getByText(/No inline view for this file/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute('href', '/api/files/raw/tmp/old.avi')
  })
})
