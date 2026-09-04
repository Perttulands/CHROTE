import { describe, expect, it } from 'vitest'
import { TONE_NOTE_SECONDS, playTone, type ToneContext } from './tones'

/** An AudioContext that records what was scheduled on it instead of playing. */
class StubParam {
  value = 0
  readonly ramps: Array<[string, number, number]> = []
  setValueAtTime(value: number, at: number) { this.ramps.push(['set', value, at]); return this }
  linearRampToValueAtTime(value: number, at: number) { this.ramps.push(['ramp', value, at]); return this }
}

class StubOscillator {
  type = ''
  readonly frequency = new StubParam()
  started = Number.NaN
  stopped = Number.NaN
  connect() { return this }
  start(at: number) { this.started = at }
  stop(at: number) { this.stopped = at }
}

class StubGain {
  readonly gain = new StubParam()
  connect() { return this }
}

function stubContext(currentTime = 3) {
  const oscillators: StubOscillator[] = []
  const gains: StubGain[] = []
  let resumed = 0
  const context = {
    currentTime,
    destination: {},
    state: 'suspended',
    createOscillator: () => { const oscillator = new StubOscillator(); oscillators.push(oscillator); return oscillator },
    createGain: () => { const gain = new StubGain(); gains.push(gain); return gain },
    resume: async () => { resumed += 1 },
  }
  return { context: context as unknown as ToneContext, oscillators, gains, resumed: () => resumed }
}

describe('playTone', () => {
  it('plays finished as two notes falling, 120 ms each, the second starting as the first stops', () => {
    const stub = stubContext(3)

    playTone('finished', stub.context)

    expect(stub.oscillators).toHaveLength(2)
    const [first, second] = stub.oscillators
    expect(first.started).toBe(3)
    expect(first.stopped).toBeCloseTo(3 + TONE_NOTE_SECONDS)
    expect(second.started).toBeCloseTo(first.stopped)
    expect(second.stopped).toBeCloseTo(3 + 2 * TONE_NOTE_SECONDS)
    expect(first.frequency.value).toBeGreaterThan(second.frequency.value)
    expect(TONE_NOTE_SECONDS).toBe(0.12)
    // The context was asked to wake, since it had never made a sound.
    expect(stub.resumed()).toBe(1)
  })

  it('plays needs-input as one note, higher than either note of finished', () => {
    const finished = stubContext()
    playTone('finished', finished.context)
    const stub = stubContext(1)

    playTone('needs-input', stub.context)

    expect(stub.oscillators).toHaveLength(1)
    const [note] = stub.oscillators
    expect(note.started).toBe(1)
    expect(note.stopped).toBeCloseTo(1 + TONE_NOTE_SECONDS)
    expect(note.frequency.value).toBeGreaterThan(Math.max(...finished.oscillators.map(o => o.frequency.value)))
    // Each note has an envelope, so it neither clicks on nor clicks off.
    const ramps = stub.gains[0].gain.ramps
    expect(ramps[0]).toEqual(['set', 0, 1])
    expect(ramps[ramps.length - 1]).toEqual(['ramp', 0, 1 + TONE_NOTE_SECONDS])
  })
})
