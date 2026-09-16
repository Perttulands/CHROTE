import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchAgentContext, fetchAgentFile } from './agentContextApi'

// The agent routes answer success and refusal in the same {success, data,
// timestamp} envelope the rest of the API uses, so the readers unwrap one shape
// and take a refusal's message from the same body.
function respondWith(body: unknown, init: { ok?: boolean; status?: number } = {}) {
  return vi.fn().mockResolvedValue({
    ok: init.ok ?? true,
    status: init.status ?? 200,
    text: async () => JSON.stringify(body),
  })
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the agent routes are read through the shared envelope', () => {
  it('reads the stack out of the envelope', async () => {
    const context = {
      folder: '/srv/chrote',
      harness: 'claude-code',
      user: 'operator',
      instructions: [{ path: '/srv/chrote/CLAUDE.md', scope: 'project', kind: 'CLAUDE.md', readable: true, size: 12 }],
      skills: [],
      memories: [],
    }
    vi.stubGlobal('fetch', respondWith({ success: true, data: context, timestamp: '2026-09-16T00:00:00Z' }))

    await expect(fetchAgentContext('/srv/chrote', 'claude-code', 'operator')).resolves.toEqual(context)
  })

  it('reads a file out of the envelope', async () => {
    vi.stubGlobal('fetch', respondWith({
      success: true,
      data: { path: '/srv/chrote/CLAUDE.md', content: '# CHROTE\n' },
      timestamp: '2026-09-16T00:00:00Z',
    }))

    await expect(fetchAgentFile('/srv/chrote/CLAUDE.md', '/srv/chrote', 'claude-code', 'operator'))
      .resolves.toBe('# CHROTE\n')
  })

  it('raises the refusal the envelope carries', async () => {
    vi.stubGlobal('fetch', respondWith(
      { success: false, error: { code: 'FORBIDDEN', message: 'Not a file this folder\'s stack lists: /etc/passwd' } },
      { ok: false, status: 403 },
    ))

    await expect(fetchAgentFile('/etc/passwd', '/srv/chrote', 'claude-code', 'operator'))
      .rejects.toThrow("Not a file this folder's stack lists: /etc/passwd")
  })
})
