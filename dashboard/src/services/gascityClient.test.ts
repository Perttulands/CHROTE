import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createGasCitySession } from './gascityClient'

describe('createGasCitySession', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('posts the native Gas City identity creation request through the CHROTE API', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({
        success: true,
        data: {
          source: 'gascity',
          id: 'ga-77',
          name: 'codxia',
          sessionName: 'codxia',
          alias: 'codxia',
          title: 'Codxia',
          template: 'codex-smoke',
          transport: 'tmux',
          workDir: '/tmp/codxia',
          deferredStart: false,
          attached: false,
          attachTarget: 'gc:ga-77',
        },
      }),
    } as Response)

    const created = await createGasCitySession({
      name: 'codxia',
      template: 'codex-smoke',
      title: 'Codxia',
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/gascity/sessions', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ name: 'codxia', template: 'codex-smoke', title: 'Codxia' }),
    }))
    expect(created.attachTarget).toBe('gc:ga-77')
  })

  it('surfaces CHROTE envelope errors with status and code', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.resolve({
        success: false,
        error: {
          code: 'GASCITY_SESSION_CREATE_FAILED',
          message: 'beads creation timed out',
        },
      }),
    } as Response)

    await expect(createGasCitySession({ name: 'codxia', template: 'planner' }))
      .rejects.toMatchObject({
        code: 'GASCITY_SESSION_CREATE_FAILED',
        status: 502,
        message: 'beads creation timed out',
      })
  })
})
