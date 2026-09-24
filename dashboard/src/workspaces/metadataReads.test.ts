import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchWorkspaces } from './workspacesApi'
import { fetchResidents, resetResidentsForTest } from '../residents/residentsApi'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

afterEach(() => {
  vi.unstubAllGlobals()
  resetResidentsForTest()
})

describe('metadata reads', () => {
  it('shares simultaneous workspace reads but reads again after completion', async () => {
    const pending = deferred<Response>()
    const fetch = vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValue(new Response('[]'))
    vi.stubGlobal('fetch', fetch)
    const first = fetchWorkspaces()
    const second = fetchWorkspaces()
    expect(fetch).toHaveBeenCalledTimes(1)
    pending.resolve(new Response('[]'))
    await Promise.all([first, second])
    await fetchWorkspaces()
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('keeps workspace projections and caller-owned cancellation independent', async () => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(new Response('[]')))
    vi.stubGlobal('fetch', fetch)
    const controller = new AbortController()
    await Promise.all([
      fetchWorkspaces(),
      fetchWorkspaces({ beads: true }),
      fetchWorkspaces({ beads: true, waitForBeads: true }),
      fetchWorkspaces({ signal: controller.signal }),
    ])
    expect(fetch.mock.calls.map(([url]) => url)).toEqual([
      '/api/workspaces', '/api/workspaces?beads=1', '/api/workspaces?beads=wait', '/api/workspaces',
    ])
    expect(fetch.mock.calls[3][1].signal).toBe(controller.signal)
  })

  it('shares resident reads, clears a failed read, and retries with current metadata', async () => {
    const pending = deferred<Response>()
    const residents = [{ tab: 'agents', label: 'Tender', session: 'tender', folder: '/work', beads: '/work' }]
    const fetch = vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValue(new Response(JSON.stringify(residents)))
    vi.stubGlobal('fetch', fetch)
    const first = fetchResidents()
    const second = fetchResidents()
    const refused = Promise.allSettled([first, second])
    expect(fetch).toHaveBeenCalledTimes(1)
    pending.resolve(new Response('unavailable', { status: 503 }))
    expect((await refused).map(result => result.status)).toEqual(['rejected', 'rejected'])
    expect(await fetchResidents()).toEqual(residents)
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
