import { afterEach, describe, expect, it } from 'vitest'
import { beadLinksOnLine } from './beadLinks'
import { resetBeadCardForTest, useBeadCardRequest } from '../beads/beadCard'
import { act, renderHook } from '@testing-library/react'

afterEach(() => {
  resetBeadCardForTest()
})

describe('Bead ids as terminal links', () => {
  it('covers exactly the id, in the columns xterm counts', () => {
    const links = beadLinksOnLine('working chrote-5grx.15 now', 7)
    expect(links).toHaveLength(1)
    expect(links[0].text).toBe('chrote-5grx.15')
    expect(links[0].range).toEqual({ start: { x: 9, y: 7 }, end: { x: 22, y: 7 } })
  })

  it('offers no link on a line that names no Bead', () => {
    expect(beadLinksOnLine('npm run test:unit', 1)).toEqual([])
  })

  it('opens the card rather than a tab', () => {
    const { result } = renderHook(() => useBeadCardRequest())
    const [link] = beadLinksOnLine('see ctx-t4ak', 3)
    act(() => link.activate(new MouseEvent('click'), link.text))
    expect(result.current?.id).toBe('ctx-t4ak')
  })
})
