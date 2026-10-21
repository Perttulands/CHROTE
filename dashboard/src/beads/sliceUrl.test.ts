import { describe, expect, it } from 'vitest'
import { parseDayISO } from './beadsSlice'
import { DEFAULT_VIEW, clearViewState, isBeadsLink, readViewState, writeViewState, type SliceViewState } from './sliceUrl'

const BASE = new URL('https://chrote.example/?variant=keep')

describe('the view as a link', () => {
  it('carries every part of a slice and reads it back the same', () => {
    const view: SliceViewState = {
      q: 'library-zvi brain',
      projects: ['library', 'home:alice'],
      statuses: ['open', 'blocked'],
      range: { from: parseDayISO('2026-10-20')!, to: parseDayISO('2026-10-21')! },
      sort: 'size',
      dir: 'asc',
      selected: { id: 'home-1', project: 'home:alice' },
      mode: 'graph',
    }
    const url = writeViewState(view, BASE)
    expect(url.searchParams.get('variant')).toBe('keep')
    expect(url.searchParams.get('from')).toBe('2026-10-20')
    expect(url.searchParams.get('sel')).toBe('home-1@home:alice')
    expect(readViewState(url.search)).toEqual(view)
  })

  it('leaves defaults out, so a plain view is a plain link', () => {
    const url = writeViewState(DEFAULT_VIEW, BASE)
    expect([...url.searchParams.keys()].sort()).toEqual(['tab', 'variant'])
    expect(isBeadsLink(url.search)).toBe(true)
    expect(readViewState(url.search)).toEqual(DEFAULT_VIEW)
  })

  it('writes a sort in its natural direction without the direction', () => {
    const url = writeViewState({ ...DEFAULT_VIEW, sort: 'idle', dir: 'desc' }, BASE)
    expect(url.searchParams.get('sort')).toBe('idle')
    expect(url.searchParams.has('dir')).toBe(false)
    expect(readViewState(url.search)).toMatchObject({ sort: 'idle', dir: 'desc' })
  })

  it('takes one day for a range given only its start, and ignores what it cannot read', () => {
    const view = readViewState('?tab=beads&from=2026-10-20&s=open,nonsense&sort=colour&mode=wide')
    expect(view.range).toEqual({ from: parseDayISO('2026-10-20'), to: parseDayISO('2026-10-20') })
    expect(view.statuses).toEqual(['open'])
    expect(view.sort).toBe(DEFAULT_VIEW.sort)
    expect(view.mode).toBe('split')
  })

  it('reads nothing from a link to another tab, and clears only its own keys', () => {
    expect(readViewState('?q=x&sort=size')).toEqual(DEFAULT_VIEW)
    const cleared = clearViewState(writeViewState({ ...DEFAULT_VIEW, q: 'x' }, BASE))
    expect(cleared.search).toBe('?variant=keep')
  })
})
