import { afterEach, describe, expect, it, vi } from 'vitest'
import { probeTextFile, readTextFile } from './fileService'

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
