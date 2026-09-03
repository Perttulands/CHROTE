import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchDirectory, fetchFileDiff, findFiles, probeTextFile, readTextFile } from './fileService'

describe('probeTextFile', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns valid UTF-8 text even when the extension and response media type are generic', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      '{"event":"started"}\n{"event":"finished"}\n',
      { status: 200, headers: { 'Content-Type': 'application/octet-stream' } },
    ))
    vi.stubGlobal('fetch', fetchMock)

    await expect(probeTextFile('/events.records')).resolves.toBe('{"event":"started"}\n{"event":"finished"}\n')
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/files/raw/events.records?inline=false',
      expect.objectContaining({ headers: { Accept: 'text/plain, text/*, application/json, */*' } }),
    )
  })

  it.each([
    new Uint8Array([0x00, 0x01, 0x02, 0x03]),
    new Uint8Array([0x01, 0x02]),
    new Uint8Array([0xff, 0xfe, 0xfd]),
  ])('returns null instead of rendering binary bytes as text', async bytes => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(bytes, {
      status: 200,
      headers: { 'Content-Type': 'application/octet-stream' },
    })))

    await expect(probeTextFile('/artifact.unknown')).resolves.toBeNull()
  })

  it('cancels and rejects an unknown-file probe that crosses the byte limit', async () => {
    const cancel = vi.fn()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('more than five bytes'))
      },
      cancel,
    })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { status: 200 })))

    await expect(probeTextFile('/growing.unknown', 5)).resolves.toBeNull()
    expect(cancel).toHaveBeenCalledOnce()
  })

  it('rejects a known text read that crosses the byte limit despite stale metadata', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('content grew', { status: 200 })))

    await expect(readTextFile('/growing.txt', 5)).rejects.toMatchObject({
      code: 'STORAGE',
      status: 413,
    })
  })
})

describe('fetchDirectory paths', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('requests the exact root and nested resource paths supplied by Files', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(
      JSON.stringify({ isDir: true, items: [] }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )))
    vi.stubGlobal('fetch', fetchMock)

    await fetchDirectory('/')
    await fetchDirectory('/code')

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/files/resources/',
      '/api/files/resources/code',
    ])
  })
})

describe('findFiles', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('carries the query to the find route and reads the matches back', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ matches: [{ path: '/srv/chrote/docs/journeys.md', name: 'journeys.md' }], truncated: true }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    ))
    vi.stubGlobal('fetch', fetchMock)

    await expect(findFiles('journeys md')).resolves.toEqual({
      matches: [{ path: '/srv/chrote/docs/journeys.md', name: 'journeys.md' }],
      truncated: true,
    })
    expect(fetchMock).toHaveBeenCalledWith('/api/files/find?q=journeys%20md', expect.anything())
  })

  it('drops entries the server could not name a path for rather than rendering blanks', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ matches: [{ name: 'orphan' }, { path: '/srv/a.ts' }, 'nonsense'] }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )))

    await expect(findFiles('a')).resolves.toEqual({
      matches: [{ path: '/srv/a.ts', name: 'a.ts' }],
      truncated: false,
    })
  })

  it('surfaces a refused find instead of showing an empty result', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 403 })))

    await expect(findFiles('secret')).rejects.toThrow('Permission denied')
  })
})

describe('fetchFileDiff', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('reads a diff, its repository and its truncation flag', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ path: '/srv/chrote/a.ts', repository: '/srv/chrote', diff: '@@ -1 +1 @@\n-a\n+b\n', truncated: false }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    ))
    vi.stubGlobal('fetch', fetchMock)

    await expect(fetchFileDiff('/srv/chrote/a.ts')).resolves.toEqual({
      path: '/srv/chrote/a.ts',
      repository: '/srv/chrote',
      diff: '@@ -1 +1 @@\n-a\n+b\n',
      truncated: false,
    })
    expect(fetchMock).toHaveBeenCalledWith('/api/files/diff?path=%2Fsrv%2Fchrote%2Fa.ts', expect.anything())
  })

  it('reads a file in no repository as an empty diff rather than a failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ repository: '', diff: '' }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )))

    await expect(fetchFileDiff('/tmp/loose.txt')).resolves.toEqual({
      path: '/tmp/loose.txt',
      repository: '',
      diff: '',
      truncated: false,
    })
  })
})
