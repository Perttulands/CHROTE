import { afterEach, describe, expect, it } from 'vitest'
import {
  FLOATING_WINDOW_MINIMUM,
  clampFrameSize,
  clearFloatingWindowSize,
  readFloatingWindowSize,
  resolveFrameSize,
  writeFloatingWindowSize,
} from './floatingWindowSize'

const minimum = FLOATING_WINDOW_MINIMUM.image
const workspace = { width: 1280, height: 800 }

afterEach(() => {
  localStorage.clear()
})

describe('the floating window size rule', () => {
  it('holds a dragged size inside the workspace and above the minimum', () => {
    expect(clampFrameSize({ width: 4000, height: 4000 }, minimum, workspace)).toEqual(workspace)
    expect(clampFrameSize({ width: 10, height: 10 }, minimum, workspace)).toEqual(minimum)
    expect(clampFrameSize({ width: 600, height: 400 }, minimum, workspace)).toEqual({ width: 600, height: 400 })
  })

  it('keeps the minimum when the workspace itself is smaller than it', () => {
    expect(clampFrameSize({ width: 500, height: 500 }, minimum, { width: 100, height: 60 })).toEqual(minimum)
  })

  it('leaves a content-derived default alone: a small picture is a small window', () => {
    // The minimum belongs to the gesture. A three-pixel picture is three
    // pixels wide until the operator says otherwise.
    expect(resolveFrameSize({ remembered: null, content: { width: 5, height: 34 }, minimum, bounds: workspace }))
      .toEqual({ width: 5, height: 34 })
  })

  it('caps a content-derived default at the workspace', () => {
    expect(resolveFrameSize({ remembered: null, content: { width: 4000, height: 60 }, minimum, bounds: workspace }))
      .toEqual({ width: 1280, height: 60 })
  })

  it('has no size until the content or the memory says', () => {
    expect(resolveFrameSize({ remembered: null, content: null, minimum, bounds: workspace })).toBeNull()
  })

  it('lets a remembered size override the content', () => {
    expect(resolveFrameSize({
      remembered: { width: 900, height: 500 },
      content: { width: 5, height: 34 },
      minimum,
      bounds: workspace,
    })).toEqual({ width: 900, height: 500 })
  })

  it('holds a remembered size inside a workspace that has since shrunk', () => {
    expect(resolveFrameSize({
      remembered: { width: 900, height: 500 },
      content: { width: 5, height: 34 },
      minimum,
      bounds: { width: 640, height: 480 },
    })).toEqual({ width: 640, height: 480 })
  })
})

describe('the remembered size', () => {
  it('is nothing until a window is resized, is kept per kind, and is cleared by the reset', () => {
    expect(readFloatingWindowSize('image')).toBeNull()

    writeFloatingWindowSize('image', { width: 900.4, height: 500.6 })
    expect(readFloatingWindowSize('image')).toEqual({ width: 900, height: 501 })
    expect(readFloatingWindowSize('peek')).toBeNull()

    writeFloatingWindowSize('peek', { width: 700, height: 400 })
    clearFloatingWindowSize('image')
    expect(readFloatingWindowSize('image')).toBeNull()
    expect(readFloatingWindowSize('peek')).toEqual({ width: 700, height: 400 })
  })

  it('ignores a stored value that is not a size, rather than opening a broken window', () => {
    localStorage.setItem('chrote.floatingWindowSize.v1', JSON.stringify({
      version: 1,
      sizes: { image: { width: 'wide', height: 200 }, peek: { width: 0, height: 0 } },
    }))
    expect(readFloatingWindowSize('image')).toBeNull()
    expect(readFloatingWindowSize('peek')).toBeNull()
  })
})
