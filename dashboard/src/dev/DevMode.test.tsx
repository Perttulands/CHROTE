import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DevMode from './DevMode'
import { identifyElement, labelFor, placeOf, referenceFor } from './identify'
import { resetChordsForTest } from '../keys/chords'

const mockState = vi.hoisted(() => ({
  announce: vi.fn(),
  openSendToSession: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({ openSendToSession: mockState.openSendToSession }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

/** A component with a name of its own, nested inside a named surface. */
function KillButton({ onPress }: { onPress: () => void }) {
  return <button className="kill" onClick={onPress}>Kill session</button>
}

function TileHeader({ onPress }: { onPress: () => void }) {
  return (
    <div data-ui="tile" data-window="2">
      <div data-ui="tile.header">
        <span className="tag">main</span>
        <KillButton onPress={onPress} />
      </div>
    </div>
  )
}

/** Leader, then the chord's key: how the registry runs a chord with no Alt form. */
function pressChord(key: string) {
  fireEvent.keyDown(document, { key: ' ', code: 'Space', ctrlKey: true, shiftKey: true })
  fireEvent.keyDown(document, { key })
}

describe('dev mode identity', () => {
  beforeEach(() => {
    resetChordsForTest()
    mockState.announce.mockClear()
    mockState.openSendToSession.mockClear()
  })

  it('names the nearest component and the file it is written in', () => {
    render(<TileHeader onPress={() => {}} />)

    const identity = identifyElement(screen.getByRole('button', { name: 'Kill session' }))

    expect(identity?.component).toBe('KillButton')
    // This test file is the one place a component named KillButton exists, so
    // the map built over the source cannot name a file for it; the component is
    // still named, which is the half that comes from the fiber walk.
    expect(identity?.file).toBeNull()

    const outer = identifyElement(screen.getByText('main'))
    expect(outer?.component).toBe('TileHeader')
  })

  it('reports the data-ui identity over the component that holds it', () => {
    render(<TileHeader onPress={() => {}} />)

    const identity = identifyElement(screen.getByRole('button', { name: 'Kill session' }))

    // The button is what KillButton rendered, and it is the precise thing under
    // the pointer, so it is what gets outlined. The identity reported with it is
    // still tile.header: that name outlives a rename of the component.
    expect(identity?.uiId).toBe('tile.header')
    expect(identity?.element.className).toBe('kill')
    expect(labelFor(identity!)).toBe('KillButton · file unknown · tile.header')

    // The span is nobody's root, so the outline snaps out to the nearest thing
    // that has a name — the header — while the words stay the span's own.
    const surface = identifyElement(screen.getByText('main'))!
    expect(surface.element).toBe(document.querySelector('[data-ui="tile.header"]'))
    expect(surface.role).toBe('span')
    expect(surface.text).toBe('main')
  })

  it('writes a reference an agent can act on', () => {
    render(<TileHeader onPress={() => {}} />)
    const identity = identifyElement(screen.getByText('main'))!

    expect(referenceFor(identity, placeOf(identity.element, 'terminal1')))
      .toBe("component TileHeader tile.header in terminal1 window 2: span 'main'")

    // With a file to name and no window to be in, both the parenthesis and the
    // window drop out rather than leaving an empty place in the line.
    const named = { ...identity, component: 'TerminalWindow', file: 'dashboard/src/components/TerminalWindow.tsx' }
    expect(referenceFor(named, { tab: 'beads', window: null }))
      .toBe("component TerminalWindow (dashboard/src/components/TerminalWindow.tsx) tile.header in beads: span 'main'")
  })

  it('annotates a click instead of letting it press the button', () => {
    const press = vi.fn()
    render(
      <>
        <DevMode activeTab="terminal1" />
        <TileHeader onPress={press} />
      </>,
    )

    pressChord('d')
    expect(mockState.announce).toHaveBeenCalledWith('Dev mode on', 'success')

    fireEvent.click(screen.getByRole('button', { name: 'Kill session' }))

    expect(press).not.toHaveBeenCalled()
    expect(mockState.openSendToSession).toHaveBeenCalledTimes(1)
    const request = mockState.openSendToSession.mock.calls[0][0]
    expect(request.reference).toBe(
      "component KillButton tile.header in terminal1 window 2: button 'Kill session'",
    )
    expect(request.launch).toEqual({ label: 'New agent in CHROTE', harness: 'claude-code' })

    // The annotation ends dev mode: the drawer it just opened has to be usable.
    expect(mockState.announce).toHaveBeenLastCalledWith('Dev mode off', 'success')
    fireEvent.click(screen.getByRole('button', { name: 'Kill session' }))
    expect(press).toHaveBeenCalledTimes(1)
  })

  it('follows the pointer while on and ends on Escape', () => {
    render(
      <>
        <DevMode activeTab="terminal1" />
        <TileHeader onPress={() => {}} />
      </>,
    )

    pressChord('d')
    fireEvent.pointerMove(screen.getByText('main'))
    expect(screen.getByText(/TileHeader/)).toBeInTheDocument()

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(mockState.announce).toHaveBeenLastCalledWith('Dev mode off', 'success')
    expect(screen.queryByText(/TileHeader/)).not.toBeInTheDocument()
  })
})
