import { afterEach, describe, expect, it, vi } from 'vitest'
import { dismissKeyEvent, ownsKey, registerSurface, resetSurfacesForTest, topSurface, type Surface } from './dismiss'
import { terminalKeyEvent } from './chords'

afterEach(() => {
  resetSurfacesForTest()
})

function surface(kind: Surface['kind'], element?: HTMLElement): Surface & { close: ReturnType<typeof vi.fn> } {
  return {
    kind,
    close: vi.fn(),
    contains: target => element?.contains(target) ?? false,
  }
}

function press(target: EventTarget, type = 'pointerdown'): Event {
  const event = new Event(type, { bubbles: true, cancelable: true })
  target.dispatchEvent(event)
  return event
}

describe('the dismissal owner', () => {
  it('reaches the topmost surface first, and then the one beneath it', () => {
    const below = surface('work')
    const above = surface('glance')
    const retireBelow = registerSurface(below)
    const retireAbove = registerSurface(above)

    expect(dismissKeyEvent(new KeyboardEvent('keydown', { key: 'Escape' }))).toBe(true)
    expect(above.close).toHaveBeenCalledTimes(1)
    expect(below.close).not.toHaveBeenCalled()

    // The surface closes itself in answer, which is what retires it.
    retireAbove()
    expect(dismissKeyEvent(new KeyboardEvent('keydown', { key: 'Escape' }))).toBe(true)
    expect(below.close).toHaveBeenCalledTimes(1)

    retireBelow()
    expect(topSurface()).toBeNull()
    expect(dismissKeyEvent(new KeyboardEvent('keydown', { key: 'Escape' }))).toBe(false)
  })

  it('keeps Escape from the pty while anything is open, and hands it back when nothing is', () => {
    const escape = new KeyboardEvent('keydown', { key: 'Escape' })
    expect(terminalKeyEvent(escape)).toBe(true)

    const retire = registerSurface(surface('work'))
    expect(ownsKey(escape)).toBe(true)
    expect(terminalKeyEvent(escape)).toBe(false)
    // Only Escape: the surface does not take the keys the shell needs.
    expect(terminalKeyEvent(new KeyboardEvent('keydown', { key: 'a' }))).toBe(true)

    retire()
    expect(terminalKeyEvent(escape)).toBe(true)
  })

  it('leaves an Escape a control inside the surface already claimed', () => {
    const open = surface('work')
    registerSurface(open)
    const claimed = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true })
    claimed.preventDefault()

    expect(dismissKeyEvent(claimed)).toBe(false)
    expect(open.close).not.toHaveBeenCalled()
  })

  it('closes a glance on a press outside it and consumes the press, but not on one inside', () => {
    const element = document.createElement('div')
    const inside = document.createElement('button')
    element.appendChild(inside)
    document.body.appendChild(element)
    const glance = surface('glance', element)
    registerSurface(glance)

    const insidePress = press(inside)
    expect(glance.close).not.toHaveBeenCalled()
    expect(insidePress.defaultPrevented).toBe(false)

    const outside = press(document.body)
    expect(glance.close).toHaveBeenCalledTimes(1)
    expect(outside.defaultPrevented).toBe(true)

    // The click that ends the press is the same press, and goes nowhere.
    const click = new MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 })
    document.body.dispatchEvent(click)
    expect(click.defaultPrevented).toBe(true)

    // The next click is a new one.
    const next = new MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 })
    document.body.dispatchEvent(next)
    expect(next.defaultPrevented).toBe(false)
    element.remove()
  })

  it('leaves a press outside a work surface alone, even beneath an open glance that was closed', () => {
    const work = surface('work')
    registerSurface(work)

    const outside = press(document.body)
    expect(work.close).not.toHaveBeenCalled()
    expect(outside.defaultPrevented).toBe(false)
  })

  it('never consumes a click that no press produced', () => {
    const glance = surface('glance')
    registerSurface(glance)
    press(document.body)
    expect(glance.close).toHaveBeenCalledTimes(1)

    // Enter on a button, or a scripted click: detail 0.
    const keyboardClick = new MouseEvent('click', { bubbles: true, cancelable: true, detail: 0 })
    document.body.dispatchEvent(keyboardClick)
    expect(keyboardClick.defaultPrevented).toBe(false)
  })
})
