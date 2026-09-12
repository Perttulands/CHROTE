import { afterEach, describe, expect, it } from 'vitest'
import {
  IMAGE_ZOOM_FIT,
  IMAGE_ZOOM_MAX_PERCENT,
  IMAGE_ZOOM_ONE_TO_ONE,
  drawImage,
  drawnZoomPercent,
  resetImageZoomForTest,
  setImageZoom,
  stepImageZoom,
  zoomPercentForWidth,
  zoomedPixels,
} from './imageZoom'

afterEach(() => resetImageZoomForTest())

const room = { width: 400, height: 300 }

describe('the level', () => {
  it('fits a picture into the room at fit, and never upscales it there', () => {
    expect(drawImage({ width: 800, height: 600 }, room, IMAGE_ZOOM_FIT)).toEqual({ width: 400, height: 300 })
    expect(drawImage({ width: 3, height: 2 }, room, IMAGE_ZOOM_FIT)).toEqual({ width: 3, height: 2 })
  })

  it('obeys a percent literally, in whole pixels and never under one', () => {
    expect(drawImage({ width: 801, height: 601 }, room, { kind: 'percent', percent: 33 }))
      .toEqual({ width: 264, height: 198 })
    expect(drawImage({ width: 3, height: 2 }, room, { kind: 'percent', percent: 10 }))
      .toEqual({ width: 1, height: 1 })
  })

  it('says the percent the picture is drawn at, fitted or asked for', () => {
    expect(drawnZoomPercent({ width: 800, height: 600 }, room, IMAGE_ZOOM_FIT)).toBe(50)
    expect(drawnZoomPercent({ width: 3, height: 2 }, room, IMAGE_ZOOM_FIT)).toBe(100)
    expect(drawnZoomPercent({ width: 800, height: 600 }, room, { kind: 'percent', percent: 250 })).toBe(250)
  })

  it('steps from where the picture is now, and stops at the ends of the ladder', () => {
    const half = { width: 800, height: 600 }
    // Drawn at 50% by the fit, so one step in is the next stop above it.
    expect(stepImageZoom(IMAGE_ZOOM_FIT, half, room, 1)).toEqual({ kind: 'percent', percent: 67 })
    expect(stepImageZoom(IMAGE_ZOOM_FIT, half, room, -1)).toEqual({ kind: 'percent', percent: 33 })
    expect(stepImageZoom({ kind: 'percent', percent: 100 }, half, room, 1)).toEqual({ kind: 'percent', percent: 125 })
    expect(stepImageZoom({ kind: 'percent', percent: 800 }, half, room, 1)).toEqual({ kind: 'percent', percent: 800 })
    expect(stepImageZoom({ kind: 'percent', percent: 10 }, half, room, -1)).toEqual({ kind: 'percent', percent: 10 })
  })

  it('reads a percent off a dragged width, held inside the ends of the ladder', () => {
    expect(zoomPercentForWidth({ width: 800, height: 600 }, 1200)).toBe(150)
    expect(zoomPercentForWidth({ width: 3, height: 2 }, 1200)).toBe(IMAGE_ZOOM_MAX_PERCENT)
  })

  it('leaves the fit to the stylesheet outside the glance, and sizes a percent in pixels', () => {
    const natural = { width: 800, height: 600 }
    expect(zoomedPixels(natural, IMAGE_ZOOM_FIT)).toBeNull()
    expect(zoomedPixels(null, IMAGE_ZOOM_ONE_TO_ONE)).toBeNull()
    expect(zoomedPixels(natural, { kind: 'percent', percent: 50 })).toEqual({ width: 400, height: 300 })
  })
})

describe('the level as it is kept', () => {
  it('is remembered for the device, and read back by a later picture', () => {
    setImageZoom({ kind: 'percent', percent: 250 })
    expect(JSON.parse(localStorage.getItem('chrote.imageZoom.v1') || 'null')).toEqual({ version: 1, percent: 250 })

    setImageZoom(IMAGE_ZOOM_FIT)
    expect(JSON.parse(localStorage.getItem('chrote.imageZoom.v1') || 'null')).toEqual({ version: 1 })
  })
})
