import { afterEach, describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { render, waitFor } from '@testing-library/react'
import { fetchBeadProjectIdentities } from './beadsApi'
import { beadProjectPath, beadProjects, refreshBeadProjects, resetBeadProjectsForTest, setBeadProjects } from './beadIds'
import BeadCatalog from './BeadCatalog'

const announce = vi.hoisted(() => vi.fn())
vi.mock('../context/SessionContext', () => ({ useSession: () => ({ settings: { beadsProjectPaths: ['/work/manual'] } }) }))
vi.mock('../context/StatusContext', () => ({ useStatus: () => ({ announce }) }))

const store = { path: '/work/discovered', sources: ['store'], sessions: [], instructions: 0 }
const projects = [
  { name: 'discovered', path: store.path, beadsPath: `${store.path}/.beads`, prefix: 'found' },
  { name: 'manual', path: '/work/manual', beadsPath: '/work/manual/.beads', prefix: 'manual' },
]

afterEach(() => {
  announce.mockReset()
  vi.unstubAllGlobals()
  resetBeadProjectsForTest()
})

describe('terminal Bead catalog', () => {
  it('reports a partial identity failure while healthy links and a retained prefix stay usable, then clears it on refresh', async () => {
    setBeadProjects([projects[0]])
    let failed = true
    const known = { ...projects[0], prefix: undefined, prefixError: 'identity lookup refused' }
    const unknown = { name: 'unknown', path: '/work/unknown', beadsPath: '/work/unknown/.beads', prefixError: 'identity unavailable' }
    const empty = { name: 'empty', path: '/work/empty', beadsPath: '/work/empty/.beads' }
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: string) => Promise.resolve(new Response(JSON.stringify(
      input === '/api/workspaces' ? [store] : { success: true, data: { projects: failed
        ? [known, projects[1], unknown, empty]
        : [projects[0], projects[1], { ...unknown, prefix: 'new', prefixError: undefined }, empty] } },
    )))))

    render(createElement(BeadCatalog))
    await waitFor(() => expect(announce).toHaveBeenCalledWith(
      'Bead link lookup failed · /work/discovered: identity lookup refused · /work/unknown: identity unavailable', 'error',
    ))
    expect(beadProjectPath('found-abc')).toBe(store.path)
    expect(beadProjectPath('manual-abc')).toBe('/work/manual')
    expect(beadProjectPath('new-abc')).toBeNull()
    expect(beadProjects().find(project => project.path === store.path)?.prefixError).toBe('identity lookup refused')
    expect(beadProjects().find(project => project.path === '/work/empty')?.prefixError).toBeUndefined()

    failed = false
    await refreshBeadProjects(['/work/manual'])
    expect(beadProjectPath('new-abc')).toBe('/work/unknown')
    expect(beadProjects().every(project => !project.prefixError)).toBe(true)
  })

  it('retains missing prefixes only for stores that still exist and accepts changed prefixes', async () => {
    setBeadProjects([
      ...projects,
      { name: 'removed', path: '/work/removed', beadsPath: '/work/removed/.beads', prefix: 'removed' },
    ])
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: string) => Promise.resolve(new Response(JSON.stringify(
      input === '/api/workspaces' ? [store] : { success: true, data: { projects: [
        { ...projects[0], prefix: 'changed' },
        { ...projects[1], prefix: undefined },
      ] } },
    )))))

    await refreshBeadProjects(['/work/manual'])

    expect(beadProjectPath('manual-abc')).toBe('/work/manual')
    expect(beadProjectPath('changed-abc')).toBe(store.path)
    expect(beadProjectPath('found-abc')).toBeNull()
    expect(beadProjectPath('removed-abc')).toBeNull()
  })

  it('shares identity reads for the same discovered and manual paths without requesting counts', async () => {
    const fetch = vi.fn().mockImplementation((input: string) => Promise.resolve(new Response(JSON.stringify(
      input === '/api/workspaces' ? [store] : { success: true, data: { projects } },
    ))))
    vi.stubGlobal('fetch', fetch)
    await Promise.all([
      fetchBeadProjectIdentities(['/work/manual', store.path]),
      fetchBeadProjectIdentities([store.path, '/work/manual']),
    ])
    expect(fetch).toHaveBeenCalledTimes(2)
    const url = new URL(fetch.mock.calls[1][0])
    expect(url.pathname).toBe('/api/beads/projects')
    expect(url.searchParams.getAll('path')).toEqual(['/work/discovered', '/work/manual'])
  })

  it('refreshes changed manual paths and can retry a failed discovery', async () => {
    let fail = true
    const fetch = vi.fn().mockImplementation((input: string) => {
      if (input === '/api/workspaces') {
        if (fail) return Promise.resolve(new Response('unavailable', { status: 503 }))
        return Promise.resolve(new Response(JSON.stringify([store])))
      }
      return Promise.resolve(new Response(JSON.stringify({ success: true, data: { projects } })))
    })
    vi.stubGlobal('fetch', fetch)
    await expect(refreshBeadProjects(['/work/old'])).rejects.toThrow()
    fail = false
    await refreshBeadProjects(['/work/manual'])
    expect(beadProjectPath('manual-abc')).toBe('/work/manual')
    expect(beadProjectPath('found-abc')).toBe(store.path)
    expect(new URL(fetch.mock.calls[2][0]).searchParams.getAll('path')).toEqual([store.path, '/work/manual'])
  })

  it('keeps a newer manual-path catalog when the older discovery finishes last', async () => {
    const answers = new Map<string, (value: Response) => void>()
    vi.stubGlobal('fetch', vi.fn().mockImplementation((input: string) => {
      if (input === '/api/workspaces') return Promise.resolve(new Response('[]'))
      const path = new URL(input).searchParams.get('path') as string
      return new Promise<Response>(resolve => { answers.set(path, resolve) })
    }))
    const old = refreshBeadProjects(['/work/old'])
    const current = refreshBeadProjects(['/work/new'])
    await vi.waitFor(() => expect(answers.size).toBe(2))
    const answer = (name: string) => new Response(JSON.stringify({
      success: true,
      data: { projects: [{ name, path: `/work/${name}`, beadsPath: `/work/${name}/.beads`, prefix: name }] },
    }))
    answers.get('/work/new')?.(answer('new'))
    await current
    answers.get('/work/old')?.(answer('old'))
    await old
    expect(beadProjectPath('new-abc')).toBe('/work/new')
    expect(beadProjectPath('old-abc')).toBeNull()
  })
})
