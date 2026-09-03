import { describe, expect, it } from 'vitest'
import { beadGlyph, beadStatusLabel, daysSince, formatBeadTime } from './beadStatus'

describe('how a Bead reads', () => {
  it('gives each state its glyph, with blocked winning over open', () => {
    expect(beadGlyph('open')).toBe('○')
    expect(beadGlyph('in_progress')).toBe('◐')
    expect(beadGlyph('closed')).toBe('✓')
    expect(beadGlyph('open', true)).toBe('●')
    expect(beadGlyph('closed', true)).toBe('✓')
  })

  it('says the state in words the operator uses', () => {
    expect(beadStatusLabel('in_progress')).toBe('in progress')
    expect(beadStatusLabel('open', true)).toBe('blocked')
  })

  it('counts whole days since an update, and admits when there is none', () => {
    const now = Date.parse('2026-09-10T00:00:00Z')
    expect(daysSince('2026-09-01T00:00:00Z', now)).toBe(9)
    expect(daysSince(undefined, now)).toBe(-1)
    expect(daysSince('not a time', now)).toBe(-1)
  })

  it('writes a timestamp as local date and time', () => {
    const at = new Date('2026-09-03T09:47:00Z')
    const pad = (value: number) => String(value).padStart(2, '0')
    const expected = `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())} ${pad(at.getHours())}:${pad(at.getMinutes())}`
    expect(formatBeadTime('2026-09-03T09:47:00Z')).toBe(expected)
    expect(formatBeadTime(undefined)).toBe('')
  })
})
