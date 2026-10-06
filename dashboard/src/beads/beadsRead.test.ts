import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { beadStoreRead, demandBeads, refreshBeads, resetBeadsReadForTest } from './beadsRead'
import { knownBead, resetKnownBeadsForTest } from './knownBeads'
import type { BeadDetail, BeadRow, BeadStoreState, BeadWork } from './beadsApi'

const api = vi.hoisted(() => ({ state: vi.fn(), work: vi.fn(), closed: vi.fn(), card: vi.fn() }))
vi.mock('./beadsApi', () => ({
  fetchBeadStates: (...args: unknown[]) => api.state(...args),
  fetchBeadWork: (...args: unknown[]) => api.work(...args),
  fetchClosedBeadWork: (...args: unknown[]) => api.closed(...args),
  fetchBeadSnapshot: (...args: unknown[]) => api.card(...args),
}))
let generation = 'one'
const row: BeadRow = { id: 'test-one', title: 'Initial', status: 'open', priority: 1, blocked: false, linked: false }
const detail: BeadDetail = { ...row, description: 'Initial text', parents: [], children: [], blocks: [], blockedBy: [] }
const state = (path: string): BeadStoreState => ({ path, availableGeneration: generation, observedGeneration: generation, pending: false })
async function settle() { for (let i = 0; i < 12; i += 1) await Promise.resolve() }
async function tick() { await vi.advanceTimersByTimeAsync(2000); await settle() }

beforeEach(() => {
  vi.useFakeTimers()
  resetBeadsReadForTest(); resetKnownBeadsForTest()
  Object.values(api).forEach(mock => mock.mockReset())
  generation = 'one'
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
  api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(state)))
  api.work.mockImplementation((path: string) => Promise.resolve({ beads: [row], prefix: 'test', projectPath: path, state: state(path) }))
  api.closed.mockImplementation((path: string) => Promise.resolve({ beads: [], prefix: 'test', projectPath: path, state: state(path) }))
  api.card.mockImplementation((path: string) => Promise.resolve({ bead: detail, projectPath: path, state: state(path) }))
})
afterEach(() => { resetBeadsReadForTest(); vi.useRealTimers() })

describe('visible Beads read owner', () => {
  it.each([undefined, 'earlier source failure'])('checks pending foreground recovery promptly with retained error %s', async error => {
    demandBeads({ paths: ['/one'], work: true, card: { path: '/one', id: row.id } })
    await settle()
    const retainedWork = beadStoreRead('/one')?.work.data
    const retainedCard = beadStoreRead('/one')?.cards.get(row.id)?.data
    api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(path => ({
      ...state(path), observedGeneration: 'two', pending: true, error,
    }))))
    await tick()
    const checks = api.state.mock.calls.length
    generation = 'two'
    api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(state)))
    api.work.mockImplementation((path: string) => Promise.resolve({ beads: [{ ...row, title: 'New' }], prefix: 'test', projectPath: path, state: state(path) }))
    api.card.mockImplementation((path: string) => Promise.resolve({ bead: { ...detail, title: 'New' }, projectPath: path, state: state(path) }))
    await vi.advanceTimersByTimeAsync(249); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks)
    expect(beadStoreRead('/one')?.work.data).toBe(retainedWork)
    expect(beadStoreRead('/one')?.cards.get(row.id)?.data).toBe(retainedCard)
    await vi.advanceTimersByTimeAsync(1); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
    expect(beadStoreRead('/one')?.work.data?.beads[0].title).toBe('New')
    expect(beadStoreRead('/one')?.cards.get(row.id)?.data?.title).toBe('New')
    expect(api.work).toHaveBeenCalledTimes(2)
    expect(api.card).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(250); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
    await vi.advanceTimersByTimeAsync(1750); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 2)
    expect(api.work).toHaveBeenCalledTimes(2)
    expect(api.card).toHaveBeenCalledTimes(2)
  })

  it('keeps background-only pending reads at the normal cadence', async () => {
    demandBeads({ paths: ['/one'], work: true })
    await settle()
    api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(path => ({ ...state(path), pending: true }))))
    await tick()
    const checks = api.state.mock.calls.length
    await vi.advanceTimersByTimeAsync(250); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks)
    await vi.advanceTimersByTimeAsync(1750); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
    expect(api.work).toHaveBeenCalledTimes(1)
  })

  it('returns to the normal cadence when a foreground read fails without an active job', async () => {
    demandBeads({ paths: ['/one'], foreground: ['/one'], work: true })
    await settle()
    const retained = beadStoreRead('/one')?.work.data
    api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(path => ({ ...state(path), pending: true }))))
    await tick()
    const checks = api.state.mock.calls.length
    api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(path => ({ ...state(path), error: 'source unavailable' }))))
    await vi.advanceTimersByTimeAsync(250); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
    expect(beadStoreRead('/one')?.state?.error).toBe('source unavailable')
    expect(beadStoreRead('/one')?.work.data).toBe(retained)
    await vi.advanceTimersByTimeAsync(250); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
    await vi.advanceTimersByTimeAsync(1750); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 2)
  })

  it('cancels pending foreground checks when hidden or no longer demanded', async () => {
    const stop = demandBeads({ paths: ['/one'], foreground: ['/one'], work: true })
    await settle()
    api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(path => ({ ...state(path), pending: true }))))
    await tick()
    const checks = api.state.mock.calls.length
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(1000); await settle()
    expect(api.state).toHaveBeenCalledTimes(checks)
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
    await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
    stop()
    await tick()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
  })

  it('shares overlapping demand and polls unchanged state without projection reads', async () => {
    const stopHost = demandBeads({ paths: ['/one'], work: true })
    const stopCard = demandBeads({ paths: ['/one'], card: { path: '/one', id: row.id } })
    await settle()
    expect(api.work).toHaveBeenCalledTimes(1)
    expect(api.card).toHaveBeenCalledTimes(1)
    await tick(); await tick()
    expect(api.work).toHaveBeenCalledTimes(1)
    expect(api.card).toHaveBeenCalledTimes(1)
    expect(api.state).toHaveBeenLastCalledWith(['/one'], ['/one'], false)
    stopHost(); stopCard()
    const checks = api.state.mock.calls.length
    await tick()
    expect(api.state).toHaveBeenCalledTimes(checks)
  })

  it('publishes successful peers while another projection is still pending', async () => {
    let finish: (work: BeadWork) => void = () => {}
    api.work.mockImplementation((path: string) => path === '/slow'
      ? new Promise(resolve => { finish = resolve })
      : Promise.resolve({ beads: [row], prefix: 'test', projectPath: path, state: state(path) }))
    demandBeads({ paths: ['/slow', '/fast'], work: true })
    await settle()
    expect(beadStoreRead('/fast')?.work.data?.beads).toEqual([row])
    expect(beadStoreRead('/slow')?.work.loading).toBe(true)
    finish({ beads: [], prefix: 'test', projectPath: '/slow', state: state('/slow') })
    await settle()
    expect(beadStoreRead('/slow')?.work.loading).toBe(false)
  })

  it('retries a failed projection at the same source generation and only applies success', async () => {
    api.work.mockRejectedValueOnce(new Error('transport unavailable'))
    demandBeads({ paths: ['/one'], work: true })
    await settle()
    expect(beadStoreRead('/one')?.work.generation).toBeUndefined()
    expect(beadStoreRead('/one')?.work.error).toBe('transport unavailable')
    await tick()
    expect(beadStoreRead('/one')?.work.generation).toBe('one')
    expect(api.work).toHaveBeenCalledTimes(2)
    await tick()
    expect(api.work).toHaveBeenCalledTimes(2)
  })

  it('retains successful work and card through failure, then updates both on a new generation', async () => {
    demandBeads({ paths: ['/one'], work: true, card: { path: '/one', id: row.id } })
    await settle()
    generation = 'two'
    api.work.mockRejectedValueOnce(new Error('temporary failure'))
    api.card.mockRejectedValueOnce(new Error('temporary failure'))
    await tick()
    expect(beadStoreRead('/one')?.work.data?.beads[0].title).toBe('Initial')
    expect(knownBead('/one', row.id)?.complete).toBe(false)
    expect(knownBead('/one', row.id)?.bead.description).toBe('Initial text')
    api.work.mockImplementation((path: string) => Promise.resolve({ beads: [{ ...row, title: 'New' }], prefix: 'test', projectPath: path, state: state(path) }))
    api.card.mockImplementation((path: string) => Promise.resolve({ bead: { ...detail, title: 'New', description: 'New text' }, projectPath: path, state: state(path) }))
    await tick()
    expect(knownBead('/one', row.id)?.bead.title).toBe('New')
    expect(knownBead('/one', row.id)?.bead.description).toBe('New text')
    expect(knownBead('/one', row.id)?.complete).toBe(true)
  })

  it('stops hidden traffic and checks promptly on return', async () => {
    demandBeads({ paths: ['/one'], work: true })
    await settle()
    const checks = api.state.mock.calls.length
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
    await tick(); await tick()
    expect(api.state).toHaveBeenCalledTimes(checks)
    generation = 'two'
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
    await settle()
    expect(api.state).toHaveBeenCalledTimes(checks + 1)
    expect(beadStoreRead('/one')?.work.generation).toBe('two')
  })

  it('forces verification and refetches satisfied projections at the same generation', async () => {
    demandBeads({ paths: ['/one'], work: true })
    await settle()
    refreshBeads(['/one'])
    await settle()
    expect(api.state).toHaveBeenLastCalledWith(['/one'], [], true)
    expect(api.work).toHaveBeenCalledTimes(2)
    expect(beadStoreRead('/one')?.work.data?.beads).toEqual([row])
  })
})

it('does not let an older in-flight success satisfy a later same-generation force request', async () => {
  let finish: (work: BeadWork) => void = () => {}
  api.work.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  demandBeads({ paths: ['/one'], work: true })
  await settle()
  refreshBeads(['/one'])
  await settle()
  expect(api.work).toHaveBeenCalledTimes(1)
  finish({ beads: [row], prefix: 'test', projectPath: '/one', state: state('/one') })
  await settle()
  expect(api.work).toHaveBeenCalledTimes(2)
  expect(beadStoreRead('/one')?.work.dirty).toBe(false)
})

it('never rolls the desired generation back when an older projection finishes late', async () => {
  let finish: (work: BeadWork) => void = () => {}
  api.work.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  demandBeads({ paths: ['/one'], work: true })
  await settle()
  generation = 'two'
  await tick()
  finish({ beads: [row], prefix: 'test', projectPath: '/one', state: { path: '/one', availableGeneration: 'one', pending: false } })
  await settle()
  expect(beadStoreRead('/one')?.state?.availableGeneration).toBe('two')
  expect(beadStoreRead('/one')?.work.generation).toBe('one')
  await tick()
  expect(beadStoreRead('/one')?.work.generation).toBe('two')
})

it('keeps projected row identity through unchanged state checks but updates dated deferral expiry', async () => {
  const { beadRows } = await import('./beadsRead')
  const until = new Date(Date.now() + 3000).toISOString()
  api.work.mockImplementation((path: string) => Promise.resolve({ beads: [{ ...row, status: 'deferred', deferUntil: until }],
    prefix: 'test', projectPath: path, state: state(path) }))
  demandBeads({ paths: ['/one'], work: true })
  await settle()
  const before = beadRows('/one', 'test', 'work')
  expect(before[0].status).toBe('deferred')
  await tick()
  expect(beadRows('/one', 'test', 'work')).toBe(before)
  await tick()
  const after = beadRows('/one', 'test', 'work')
  expect(after).not.toBe(before)
  expect(after[0].status).toBe('open')
  expect(api.work).toHaveBeenCalledTimes(1)
})

it('waits for a queued forced verification before starting projections after an older state check', async () => {
  let finishState: (states: BeadStoreState[]) => void = () => {}
  api.state.mockImplementationOnce(() => new Promise(resolve => { finishState = resolve }))
    .mockImplementationOnce((paths: string[]) => Promise.resolve(paths.map(path => ({ ...state(path), pending: true }))))
  demandBeads({ paths: ['/one'], work: true })
  await settle()
  refreshBeads(['/one'])
  finishState([state('/one')])
  await settle()
  expect(api.state).toHaveBeenLastCalledWith(['/one'], [], true)
  expect(api.work).not.toHaveBeenCalled()
  expect(beadStoreRead('/one')?.waitForRefresh).toBe(true)
  await tick()
  expect(api.work).toHaveBeenCalledTimes(1)
  expect(beadStoreRead('/one')?.work.dirty).toBe(false)
})

it('keeps a newer authoritative empty projection when an older in-flight work response finishes', async () => {
  let finish: (work: BeadWork) => void = () => {}
  api.work.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  demandBeads({ paths: ['/one'], work: true })
  await settle()
  generation = 'two'
  api.state.mockImplementation((paths: string[]) => Promise.resolve(paths.map(path => ({ ...state(path), openBeads: 0 }))))
  await tick()
  expect(beadStoreRead('/one')?.work.data?.beads).toEqual([])
  expect(beadStoreRead('/one')?.work.generation).toBe('two')
  finish({ beads: [row], prefix: 'test', projectPath: '/one', state: { path: '/one', availableGeneration: 'one', pending: false } })
  await settle()
  expect(beadStoreRead('/one')?.work.data?.beads).toEqual([])
  expect(beadStoreRead('/one')?.work.generation).toBe('two')
  expect(knownBead('/one', row.id)).toBeNull()
})
