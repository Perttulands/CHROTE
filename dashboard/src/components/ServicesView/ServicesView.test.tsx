import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ServicesView from './index'

const fetchMock = vi.fn()

function envelope(data: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify({ success: true, data }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  }))
}

function errorEnvelope(code: string, message: string, status = 502) {
  return Promise.resolve(new Response(JSON.stringify({
    success: false,
    error: { code, message },
  }), { status, headers: { 'Content-Type': 'application/json' } }))
}

function installHappyPathFetch() {
  fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method || 'GET'

    if (url === '/api/services') {
      return envelope({
        services: [
          { id: 'tts', name: 'TTS Gateway', status: 'configured', configured: true, capabilities: [] },
          { id: 'context', name: 'Context Citadel', status: 'configured', configured: true, tokenConfigured: true, capabilities: [] },
        ],
      })
    }
    if (url === '/api/services/tts/health') return envelope({ status: 'ok', messages: 1, clients: 0 })
    if (url === '/api/services/tts/messages') {
      return envelope({
        messages: [
          {
            id: 'ready1',
            text: 'Ready message',
            source: 'Codex',
            backend: 'edge',
            voice: 'en-US-ChristopherNeural',
            status: 'ready',
            createdAt: '2026-05-19T08:00:00Z',
          },
          {
            id: 'failed1',
            text: 'Failed message',
            source: 'Codex',
            status: 'error',
            error: 'voice backend unavailable',
          },
        ],
      })
    }
    if (url === '/api/services/tts/enqueue' && method === 'POST') {
      return envelope({ id: 'queued1', status: 'queued' }, 202)
    }
    if (url === '/api/services/context/docs') {
      return envelope({
        docs: [
          { path: 'identity/communication.md', size: 120, modified: '2026-05-19T08:00:00Z', title: 'Communication' },
        ],
      })
    }
    if (url === '/api/services/context/docs/identity%2Fcommunication.md' && method === 'GET') {
      return envelope({
        path: 'identity/communication.md',
        content: '# Communication\nKeep it brief.',
        modified: '2026-05-19T08:00:00Z',
      })
    }
    if (url === '/api/services/context/docs/identity%2Fcommunication.md' && method === 'PUT') {
      return envelope({ ok: true, path: 'identity/communication.md' })
    }
    if (url === '/api/services/context/history/identity%2Fcommunication.md') {
      return envelope({
        path: 'identity/communication.md',
        history: [{ hash: 'abc123', date: '2026-05-19T08:00:00Z', subject: 'PUT identity/communication.md' }],
      })
    }
    if (url === '/api/services/context/ask' && method === 'POST') {
      return envelope({
        answer: 'Use short status updates.',
        sources: [{ path: 'identity/communication.md', snippet: 'Keep it brief.' }],
      })
    }
    if (url === '/api/services/context/grants' && method === 'GET') {
      return envelope({
        grants: [
          {
            id: 'grant_1',
            name: 'CHROTE preview grant',
            status: 'active',
            scopes: ['retrieve', 'docs:excerpt'],
            constraints: { domains: ['world'], max_sensitivity: 'internal' },
            egress: { allowed_providers: ['openai'], allowed_models: ['gpt-5.2'] },
            token_fingerprint: 'sha256:fingerprint',
          },
        ],
      })
    }
    if (url === '/api/services/context/grants' && method === 'POST') {
      return envelope({
        token: 'ctx_live_fixturehandle_fixturesecretvalue',
        grant: { id: 'grant_2', name: 'ChatGPT action', status: 'active' },
      })
    }
    if (url === '/api/services/context/grants/grant_1/revoke' && method === 'POST') {
      return envelope({ grant: { id: 'grant_1', name: 'CHROTE preview grant', status: 'revoked' } })
    }
    if (url === '/api/services/context/grants/grant_1/rotate' && method === 'POST') {
      return envelope({
        token: 'ctx_live_rotatedhandle_rotatedsecretvalue',
        grant: { id: 'grant_1', name: 'CHROTE preview grant', status: 'active' },
      })
    }
    if (url === '/api/services/context/grants/preview' && method === 'POST') {
      return envelope({
        preview: {
          egress_plan: {
            allowed: true,
            provider: 'openai',
            model: 'gpt-5.2',
            reason: 'allowed',
            total_prompt_chars: 240,
          },
          chunks: [
            { canonical_path: 'world/egress-allowed.md', snippet: 'Allowed bounded context.' },
          ],
          denied: [{ canonical_path: 'health/private.md', reason: 'domain_denied' }],
        },
      })
    }
    if (url === '/api/services/context/ingestion/queue') {
      return envelope({
        items: [
          {
            path: 'inbox/candidates/candidate-safe.md',
            lifecycle: 'candidate',
            review_status: 'pending',
            prompt_injection_risk: 'medium',
          },
          {
            path: 'inbox/candidates/candidate-risky.md',
            lifecycle: 'candidate',
            review_status: 'pending',
            prompt_injection_risk: 'high',
          },
        ],
      })
    }
    if (url === '/api/services/context/ingestion/candidates/inbox%2Fcandidates%2Fcandidate-safe.md/approve' && method === 'POST') {
      return envelope({
        item: {
          path: 'inbox/candidates/candidate-safe.md',
          lifecycle: 'candidate',
          review_status: 'approved',
        },
      })
    }
    if (url === '/api/services/context/ingestion/candidates/inbox%2Fcandidates%2Fcandidate-risky.md/reject' && method === 'POST') {
      return envelope({
        item: {
          path: 'inbox/candidates/candidate-risky.md',
          lifecycle: 'candidate',
          review_status: 'rejected',
        },
      })
    }
    if (url === '/api/services/context/audit?limit=25') {
      return envelope({
        events: [
          {
            id: 'audit_1',
            type: 'grant.created',
            actor: 'owner',
            operation: 'grants:admin',
            grant_id: 'grant_1',
            timestamp: '2026-05-19T14:00:00Z',
          },
        ],
      })
    }

    throw new Error(`Unexpected fetch ${method} ${url}`)
  })
}

describe('ServicesView', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('EventSource', undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders TTS health, queue, playback, and failed generation state', async () => {
    installHappyPathFetch()

    render(<ServicesView />)

    expect(await screen.findByText('TTS Gateway')).toBeInTheDocument()
    expect(screen.getByText('status: ok')).toBeInTheDocument()
    expect(screen.getByText('Ready message')).toBeInTheDocument()
    expect(screen.getByText('Failed message')).toBeInTheDocument()
    expect(screen.getByText('voice backend unavailable')).toBeInTheDocument()
    expect(screen.getByLabelText('Play Ready message')).toHaveAttribute('src', '/api/services/tts/audio/ready1')
  })

  it('submits TTS enqueue controls through the CHROTE proxy', async () => {
    installHappyPathFetch()

    render(<ServicesView />)

    fireEvent.change(await screen.findByLabelText('TTS text'), { target: { value: 'Short spoken status.' } })
    fireEvent.change(screen.getByLabelText('TTS source'), { target: { value: 'Codex' } })
    fireEvent.change(screen.getByLabelText('TTS backend'), { target: { value: 'edge' } })
    fireEvent.change(screen.getByLabelText('TTS voice'), { target: { value: 'en-US-ChristopherNeural' } })
    fireEvent.click(screen.getByRole('button', { name: 'Enqueue' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/services/tts/enqueue', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          text: 'Short spoken status.',
          source: 'Codex',
          backend: 'edge',
          voice: 'en-US-ChristopherNeural',
        }),
      }))
    })
    expect(await screen.findByText('queued1 queued')).toBeInTheDocument()
  })

  it('starts polling refreshes after the TTS SSE feed errors', async () => {
    const eventSources: MockEventSource[] = []
    class MockEventSource {
      url: string
      onerror: ((event: Event) => void) | null = null
      close = vi.fn()
      addEventListener = vi.fn()

      constructor(url: string) {
        this.url = url
        eventSources.push(this)
      }
    }

    vi.stubGlobal('EventSource', MockEventSource)
    vi.spyOn(window, 'setInterval').mockImplementation((handler: Parameters<typeof window.setInterval>[0]) => {
      if (typeof handler === 'function') handler()
      return {} as ReturnType<typeof window.setInterval>
    })
    vi.spyOn(window, 'clearInterval').mockImplementation(() => undefined)
    installHappyPathFetch()

    render(<ServicesView />)

    await waitFor(() => expect(eventSources).toHaveLength(1))
    expect(eventSources[0].url).toBe('/api/services/tts/feed')
    expect(await screen.findByText('LIVE')).toBeInTheDocument()

    const messageRequestsBeforeError = fetchMock.mock.calls.filter(([input]) => (
      String(input) === '/api/services/tts/messages'
    )).length

    act(() => {
      eventSources[0].onerror?.(new Event('error'))
    })

    expect(await screen.findByText('POLL')).toBeInTheDocument()
    await waitFor(() => {
      const messageRequestsAfterError = fetchMock.mock.calls.filter(([input]) => (
        String(input) === '/api/services/tts/messages'
      )).length
      expect(messageRequestsAfterError).toBeGreaterThan(messageRequestsBeforeError)
    })
  })

  it('supports Context list, read, save, history, and ask flows', async () => {
    installHappyPathFetch()

    render(<ServicesView />)

    fireEvent.click(await screen.findByRole('button', { name: 'identity/communication.md' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Context document content')).toHaveValue('# Communication\nKeep it brief.')
    })
    expect(screen.getByText('abc123')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Context document content'), {
      target: { value: '# Communication\nUpdated guidance.' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save Document' }))
    expect(await screen.findByText('Saved identity/communication.md')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Ask Context'), {
      target: { value: 'How should agents speak?' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Ask' }))

    expect(await screen.findByText('Use short status updates.')).toBeInTheDocument()
    expect(screen.getAllByText('identity/communication.md').length).toBeGreaterThan(0)
    expect(screen.getByText('Keep it brief.')).toBeInTheDocument()
  })

  it('supports Context integration grant preview, rotation, ingestion, and audit controls', async () => {
    installHappyPathFetch()

    render(<ServicesView />)

    expect((await screen.findAllByText('CHROTE preview grant')).length).toBeGreaterThan(0)
    expect(screen.getByText('inbox/candidates/candidate-safe.md')).toBeInTheDocument()
    expect(screen.getByText('inbox/candidates/candidate-risky.md')).toBeInTheDocument()
    expect(screen.getByText('grant.created')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Grant name'), { target: { value: 'ChatGPT action' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create Grant' }))

    expect(await screen.findByLabelText('One-time grant token')).toHaveTextContent('ctx_live_fixturehandle_fixturesecretvalue')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/services/context/grants', expect.objectContaining({
        method: 'POST',
      }))
    })

    fireEvent.click(screen.getByRole('button', { name: 'Revoke grant CHROTE preview grant' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/services/context/grants/grant_1/revoke', expect.objectContaining({
        method: 'POST',
      }))
    })

    fireEvent.click(screen.getByRole('button', { name: 'Rotate grant CHROTE preview grant' }))
    expect(await screen.findByLabelText('One-time grant token')).toHaveTextContent('ctx_live_rotatedhandle_rotatedsecretvalue')

    fireEvent.change(screen.getByLabelText('Preview question'), { target: { value: 'What can this grant see?' } })
    fireEvent.click(screen.getByRole('button', { name: 'Preview Grant' }))

    expect(await screen.findByText('egress: allowed')).toBeInTheDocument()
    expect(screen.getByText('world/egress-allowed.md')).toBeInTheDocument()
    expect(screen.getByText('health/private.md')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Approve inbox/candidates/candidate-safe.md' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/services/context/ingestion/candidates/inbox%2Fcandidates%2Fcandidate-safe.md/approve', expect.objectContaining({
        method: 'POST',
      }))
    })

    fireEvent.click(screen.getByRole('button', { name: 'Reject inbox/candidates/candidate-risky.md' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/services/context/ingestion/candidates/inbox%2Fcandidates%2Fcandidate-risky.md/reject', expect.objectContaining({
        method: 'POST',
      }))
    })

    const callsWithHeaders = fetchMock.mock.calls.map(([, init]) => init?.headers || {})
    expect(callsWithHeaders).not.toContainEqual(expect.objectContaining({ Authorization: expect.any(String) }))
    expect(screen.queryByText('token_hash')).not.toBeInTheDocument()
  })

  it('disables saving stale context content after opening a different document fails', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method || 'GET'

      if (url === '/api/services') {
        return envelope({
          services: [
            { id: 'tts', name: 'TTS Gateway', status: 'configured', configured: true, capabilities: [] },
            { id: 'context', name: 'Context Citadel', status: 'configured', configured: true, tokenConfigured: true, capabilities: [] },
          ],
        })
      }
      if (url === '/api/services/tts/health') return envelope({ status: 'ok', messages: 0, clients: 0 })
      if (url === '/api/services/tts/messages') return envelope({ messages: [] })
      if (url === '/api/services/context/docs') {
        return envelope({
          docs: [
            { path: 'identity/communication.md', title: 'Communication' },
            { path: 'identity/broken.md', title: 'Broken' },
          ],
        })
      }
      if (url === '/api/services/context/docs/identity%2Fcommunication.md' && method === 'GET') {
        return envelope({
          path: 'identity/communication.md',
          content: '# Communication\nKeep it brief.',
          modified: '2026-05-19T08:00:00Z',
        })
      }
      if (url === '/api/services/context/history/identity%2Fcommunication.md') {
        return envelope({
          path: 'identity/communication.md',
          history: [{ hash: 'abc123', date: '2026-05-19T08:00:00Z', subject: 'PUT identity/communication.md' }],
        })
      }
      if (url === '/api/services/context/docs/identity%2Fbroken.md' && method === 'GET') {
        return errorEnvelope('CONTEXT_DOC_READ_FAILED', 'read failed', 500)
      }
      if (url === '/api/services/context/history/identity%2Fbroken.md') {
        return envelope({ path: 'identity/broken.md', history: [] })
      }
      if (url === '/api/services/context/docs/identity%2Fbroken.md' && method === 'PUT') {
        return envelope({ ok: true, path: 'identity/broken.md' })
      }
      if (url === '/api/services/context/grants') return envelope({ grants: [] })
      if (url === '/api/services/context/ingestion/queue') return envelope({ items: [] })
      if (url === '/api/services/context/audit?limit=25') return envelope({ events: [] })

      throw new Error(`Unexpected fetch ${method} ${url}`)
    })

    render(<ServicesView />)

    fireEvent.click(await screen.findByRole('button', { name: 'identity/communication.md' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Context document content')).toHaveValue('# Communication\nKeep it brief.')
    })

    fireEvent.click(screen.getByRole('button', { name: 'identity/broken.md' }))
    expect(await screen.findByText('read failed')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save Document' })).toBeDisabled()
  })

  it('renders upstream and missing-token states without destructive actions', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/services') {
        return envelope({
          services: [
            { id: 'tts', name: 'TTS Gateway', status: 'configured', configured: true, capabilities: [] },
            {
              id: 'context',
              name: 'Context Citadel',
              status: 'degraded',
              configured: true,
              tokenConfigured: false,
              message: 'CHROTE_CONTEXT_API_TOKEN is not configured',
              capabilities: [],
            },
          ],
        })
      }
      if (url === '/api/services/tts/health') return errorEnvelope('SERVICE_UPSTREAM_ERROR', 'connect refused')
      if (url === '/api/services/tts/messages') return envelope({ messages: [] })
      if (url === '/api/services/context/docs') return errorEnvelope('MISSING_CONTEXT_TOKEN', 'CHROTE_CONTEXT_API_TOKEN is not configured', 503)
      return envelope({})
    })

    render(<ServicesView />)

    expect(await screen.findByText('connect refused')).toBeInTheDocument()
    expect(await screen.findByText('CHROTE_CONTEXT_API_TOKEN is not configured')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save Document' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Ask' })).toBeDisabled()
  })
})
