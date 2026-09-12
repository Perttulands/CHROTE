import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ImageGlance, { IMAGE_GLANCE_HEADER_PX, imageGlanceSize } from './ImageGlance'
import { openImageGlance, resetImageGlanceForTest } from './imageGlance'
import { resetOpenInFilesForTest, useOpenInFilesRequest } from '../terminal/openInFiles'
import { resetSurfacesForTest } from '../keys/dismiss'
import { renderHook } from '@testing-library/react'

const statusMocks = vi.hoisted(() => ({ announce: vi.fn() }))
const fileServiceMocks = vi.hoisted(() => ({ describeReadFailure: vi.fn().mockResolvedValue(null) }))

vi.mock('./FilesView/fileService', async importOriginal => ({
  ...(await importOriginal<typeof import('./FilesView/fileService')>()),
  describeReadFailure: fileServiceMocks.describeReadFailure,
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ status: null, announce: statusMocks.announce }),
}))

describe('the size rule', () => {
  const workspace = { width: 1280, height: 800 }

  it('shows a picture that fits at 1:1, with the header and hairline around it', () => {
    expect(imageGlanceSize({ width: 640, height: 480 }, workspace)).toEqual({
      width: 642,
      height: 480 + 2 + IMAGE_GLANCE_HEADER_PX,
      image: { width: 640, height: 480 },
    })
  })

  it('scales a picture down at its own ratio to fit inside 90% of the workspace', () => {
    const size = imageGlanceSize({ width: 4000, height: 1000 }, workspace)
    // 90% of 1280 is 1152, less the hairline: the width is what binds.
    expect(size.image.width).toBe(1150)
    expect(size.image.height).toBe(Math.floor(1000 * (1150 / 4000)))
    expect(size.width).toBeLessThanOrEqual(1152)

    const tall = imageGlanceSize({ width: 100, height: 3000 }, workspace)
    expect(tall.height).toBeLessThanOrEqual(720)
    expect(tall.image.width).toBe(Math.floor(100 * (tall.image.height / 3000)))
  })

  it('never upscales a small picture', () => {
    expect(imageGlanceSize({ width: 3, height: 2 }, workspace).image).toEqual({ width: 3, height: 2 })
  })

  it('draws the picture at the level, and caps the window when the level is bigger than the workspace', () => {
    // 1:1 on a picture wider than the workspace: the picture keeps its pixels
    // and the window stops at 90%, so the picture scrolls inside it.
    const actual = imageGlanceSize({ width: 4000, height: 1000 }, workspace, { kind: 'percent', percent: 100 })
    expect(actual.image).toEqual({ width: 4000, height: 1000 })
    expect(actual.width).toBe(1152)
    expect(actual.height).toBe(Math.min(720, 1000 + 2 + IMAGE_GLANCE_HEADER_PX))

    // A percent is whole pixels, and a picture is never drawn away to nothing.
    expect(imageGlanceSize({ width: 801, height: 601 }, workspace, { kind: 'percent', percent: 33 }).image)
      .toEqual({ width: 264, height: 198 })
    expect(imageGlanceSize({ width: 3, height: 2 }, workspace, { kind: 'percent', percent: 10 }).image)
      .toEqual({ width: 1, height: 1 })

    // A small picture at 1:1 is the window's size, as fit already made it.
    expect(imageGlanceSize({ width: 3, height: 2 }, workspace, { kind: 'percent', percent: 100 }))
      .toEqual(imageGlanceSize({ width: 3, height: 2 }, workspace))
  })
})

describe('ImageGlance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  afterEach(() => {
    resetImageGlanceForTest()
    resetOpenInFilesForTest()
    resetSurfacesForTest()
  })

  it('draws nothing until asked, then the path, the pixels once loaded, and its words', () => {
    const files = renderHook(() => useOpenInFilesRequest())
    const { container } = render(<ImageGlance />)
    expect(container.querySelector('.image-glance')).toBeNull()

    act(() => openImageGlance('/srv/evidence/frame.png'))
    const glance = screen.getByRole('dialog', { name: 'Image frame.png' })
    expect(glance).toHaveTextContent('Loading…')

    const image = screen.getByRole('img', { name: 'frame.png' })
    expect(image).toHaveAttribute('src', expect.stringContaining('/api/files/raw/srv/evidence/frame.png'))
    Object.defineProperty(image, 'naturalWidth', { value: 320, configurable: true })
    Object.defineProperty(image, 'naturalHeight', { value: 200, configurable: true })
    fireEvent.load(image)
    expect(glance).toHaveTextContent('320 × 200')

    fireEvent.click(screen.getByRole('button', { name: 'Copy path' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('/srv/evidence/frame.png')
    expect(statusMocks.announce).toHaveBeenCalledWith('Copied /srv/evidence/frame.png', 'success')

    // A glance: a press outside closes it.
    fireEvent.pointerDown(document.body)
    expect(container.querySelector('.image-glance')).toBeNull()

    act(() => openImageGlance('/srv/evidence/frame.png'))
    fireEvent.click(screen.getByRole('button', { name: 'Open in Files' }))
    expect(files.result.current?.path).toBe('/srv/evidence/frame.png')
    expect(container.querySelector('.image-glance')).toBeNull()
  })

  it('says so when the picture cannot be loaded, and why once the route has said', async () => {
    fileServiceMocks.describeReadFailure.mockResolvedValueOnce('Permission denied')
    render(<ImageGlance />)
    act(() => openImageGlance('/srv/evidence/gone.png'))

    fireEvent.error(screen.getByRole('img', { name: 'gone.png' }))
    expect(screen.getByText('Could not load /srv/evidence/gone.png.')).toBeInTheDocument()
    expect(await screen.findByText('Could not load /srv/evidence/gone.png: Permission denied.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
