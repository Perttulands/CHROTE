import { describe, expect, it } from 'vitest'
import { NOW, SCRUB_STEPS, backAt, backFrom, currentAt, existedAt, momentOf, positionOf, snapTo, stopsOn } from './mapMoment'

const DAY = 86_400_000
const SPAN = 120 * DAY

describe('what a moment says about a page', () => {
  it('draws a page that had arrived by the moment and leaves out one that had not', () => {
    expect(existedAt(1_000, 2_000)).toBe(true)
    expect(existedAt(3_000, 2_000)).toBe(false)
  })

  it('calls a page current when its last change is at or before the moment', () => {
    expect(currentAt(2_000, 2_000)).toBe(true)
    expect(currentAt(2_001, 2_000)).toBe(false)
  })

  // A page the corpus cannot date cannot be placed in time either way, so no
  // moment hides it and no moment calls it stale.
  it('places a page git has never seen at every moment', () => {
    expect(existedAt(0, 1)).toBe(true)
    expect(currentAt(0, 1)).toBe(true)
    expect(momentOf('')).toBe(0)
    expect(momentOf('not a date')).toBe(0)
  })

  it('draws the whole corpus at now', () => {
    expect(existedAt(Date.now(), NOW)).toBe(true)
    expect(currentAt(Date.now(), NOW)).toBe(true)
  })
})

describe('the scrubber', () => {
  it('is now at its right end and the corpus’s first page at its left', () => {
    expect(backAt(SCRUB_STEPS, SPAN)).toBe(0)
    expect(backAt(0, SPAN)).toBe(SPAN)
  })

  // The recent past is what a reader asks about, so it takes most of the track:
  // a week ago must be somewhere he can land on, not in the last percent.
  it('gives the recent past most of the track', () => {
    const week = positionOf(7 * DAY, SPAN)
    expect(week).toBeGreaterThan(SCRUB_STEPS * 0.5)
    expect(week).toBeLessThan(SCRUB_STEPS * 0.75)
    expect(positionOf(30 * DAY, SPAN)).toBeLessThan(week)
    expect(positionOf(DAY, SPAN)).toBeGreaterThan(week)
  })

  // A thousand whole steps over months means the place nearest a week ago is
  // some hours off it, and a stop that reads "6 days ago" is a stop that does
  // not work. On a stop the scrubber is asking about that stop exactly.
  it('is exact on a stop and follows the curve between them', () => {
    expect(backFrom(positionOf(7 * DAY, SPAN), SPAN)).toBe(7 * DAY)
    const between = positionOf(7 * DAY, SPAN) + 40
    expect(backFrom(between, SPAN)).toBe(backAt(between, SPAN))
  })

  it('carries the four windows the map used to offer as buttons', () => {
    expect(stopsOn(SPAN).map(stop => stop.label))
      .toEqual(['A month ago', 'A week ago', 'A day ago', 'Now'])
  })

  // A corpus younger than a stop has nowhere to put it.
  it('leaves out a stop older than the corpus', () => {
    expect(stopsOn(5 * DAY).map(stop => stop.id)).toEqual(['day', 'now'])
  })

  it('takes a stop the scrubber came near, and leaves a place between stops alone', () => {
    const week = positionOf(7 * DAY, SPAN)
    expect(snapTo(week + 3, SPAN)).toBe(week)
    expect(snapTo(week + 60, SPAN)).toBe(week + 60)
  })
})
