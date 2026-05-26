import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  askContext,
  approveContextIngestionCandidate,
  createContextGrant,
  enqueueTTS,
  getContextAudit,
  getContextDocs,
  getContextGrants,
  getTTSMessages,
  previewContextGrant,
  rejectContextIngestionCandidate,
  revokeContextGrant,
  rotateContextGrant,
  listServices,
  readContextDoc,
  saveContextDoc,
  getContextIngestionQueue,
} from './servicesClient'

const fetchMock = vi.fn()

function success(data: unknown) {
  return Promise.resolve(new Response(JSON.stringify({ success: true, data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  }))
}

describe('servicesClient', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  it('calls only CHROTE-owned service routes', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      expect(url).toMatch(/^\/api\/services/)
      expect(url).not.toContain('3100')
      expect(url).not.toContain('3200')

      if (url === '/api/services') return success({ services: [] })
      if (url === '/api/services/tts/messages') return success({ messages: [] })
      if (url === '/api/services/context/docs') return success({ docs: [] })
      return success({})
    })

    await listServices()
    await getTTSMessages()
    await getContextDocs()
    await getContextGrants()
    await getContextIngestionQueue()
    await getContextAudit()

    expect(fetchMock).toHaveBeenCalledTimes(6)
  })

  it('forwards TTS enqueue controls through CHROTE', async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      success: true,
      data: { id: 'msg-1', status: 'queued' },
    }), { status: 202, headers: { 'Content-Type': 'application/json' } }))

    await enqueueTTS({
      text: 'Operator status',
      source: 'Codex',
      backend: 'kokoro',
      voice: 'am_onyx',
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/services/tts/enqueue', expect.objectContaining({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        text: 'Operator status',
        source: 'Codex',
        backend: 'kokoro',
        voice: 'am_onyx',
      }),
    }))
  })

  it('supports Context read, save, and ask without browser tokens', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(JSON.stringify({
        success: true,
        data: { path: 'identity/communication.md', content: '# Communication\n' },
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        success: true,
        data: { ok: true, path: 'identity/communication.md' },
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        success: true,
        data: {
          answer: 'Use short status updates.',
          sources: [{ path: 'identity/communication.md', snippet: 'Keep it brief.' }],
        },
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }))

    await readContextDoc('identity/communication.md')
    await saveContextDoc('identity/communication.md', 'updated')
    await askContext('How should agents speak?')

    for (const call of fetchMock.mock.calls) {
      const [, init] = call
      expect(init?.headers).not.toHaveProperty('Authorization')
    }
    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      '/api/services/context/docs/identity%2Fcommunication.md',
      '/api/services/context/docs/identity%2Fcommunication.md',
      '/api/services/context/ask',
    ])
  })

  it('supports Context integration routes without browser bearer tokens', async () => {
    fetchMock.mockImplementation(() => success({
      grant: { id: 'grant_1' },
      token: 'ctx_live_fixturehandle_fixturesecretvalue',
    }))

    await createContextGrant({ name: 'ChatGPT' })
    await revokeContextGrant('grant_1')
    await rotateContextGrant('grant_1')
    await previewContextGrant({ grant_id: 'grant_1', question: 'What is safe to show?' })
    await approveContextIngestionCandidate('inbox/candidates/idea.md')
    await rejectContextIngestionCandidate('inbox/candidates/risky.md', 'Not reliable enough.')

    for (const [, init] of fetchMock.mock.calls) {
      expect(init?.headers).not.toHaveProperty('Authorization')
    }
    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual([
      '/api/services/context/grants',
      '/api/services/context/grants/grant_1/revoke',
      '/api/services/context/grants/grant_1/rotate',
      '/api/services/context/grants/preview',
      '/api/services/context/ingestion/candidates/inbox%2Fcandidates%2Fidea.md/approve',
      '/api/services/context/ingestion/candidates/inbox%2Fcandidates%2Frisky.md/reject',
    ])
  })
})
