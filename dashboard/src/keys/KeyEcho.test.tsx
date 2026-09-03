import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import KeyEcho from './KeyEcho'
import { registerChords, resetChordsForTest, type Chord } from './chords'

const chords: Chord[] = [
  { id: 'tab1', key: '1', direct: { alt: true, shift: false, key: '1' }, label: 'Terminal tab 1', scope: 'global', run: () => {} },
  { id: 'prevWindow', key: 'W', direct: { alt: true, shift: true, key: 'w' }, label: 'Previous window', scope: 'global', run: () => {} },
  { id: 'keysOff', key: 'k', label: 'Keys off', scope: 'global', run: () => {} },
]

function press(init: KeyboardEventInit) {
  act(() => {
    document.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init }))
  })
}

describe('KeyEcho', () => {
  let retire: () => void

  beforeEach(() => {
    vi.useFakeTimers()
    resetChordsForTest()
    retire = registerChords(chords)
  })

  afterEach(() => {
    retire()
    resetChordsForTest()
    vi.useRealTimers()
  })

  it('echoes a registered chord as key caps and takes them away again', () => {
    const { container } = render(<KeyEcho />)
    expect(container.querySelector('.key-echo')).toBeNull()

    press({ key: '1', altKey: true })

    const caps = Array.from(container.querySelectorAll('.key-echo-cap'))
    expect(caps.map(cap => cap.textContent)).toEqual(['ALT', '1'])
    // The modifier is the filled cap; the key is the outlined one.
    expect(caps[0]).toHaveClass('key-echo-modifier')
    expect(caps[1]).not.toHaveClass('key-echo-modifier')

    act(() => { vi.advanceTimersByTime(800) })
    expect(container.querySelector('.key-echo')).toBeNull()
  })

  it('shows every modifier the chord holds', () => {
    const { container } = render(<KeyEcho />)

    press({ key: 'w', altKey: true, shiftKey: true })

    expect(Array.from(container.querySelectorAll('.key-echo-cap')).map(cap => cap.textContent))
      .toEqual(['ALT', 'SHIFT', 'W'])
  })

  it('says nothing for a key CHROTE did not take', () => {
    const { container } = render(<KeyEcho />)

    // Alt+X is nobody's chord, so it belongs to the program in the terminal.
    press({ key: 'x', altKey: true })
    // And ordinary typing is never CHROTE's at all.
    press({ key: 'k' })

    expect(container.querySelector('.key-echo')).toBeNull()
  })
})
