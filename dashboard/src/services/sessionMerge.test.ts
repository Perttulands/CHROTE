import { describe, expect, it } from 'vitest'
import type { SessionsResponse } from '../types'
import type { GasCityObserver } from './gascityClient'
import { mergeTmuxAndGasCitySessions } from './sessionMerge'

const tmuxResponse: SessionsResponse = {
  sessions: [
    { name: 'hq-mayor', windows: 1, attached: false, group: 'hq' },
    { name: 'main', windows: 2, attached: true, group: 'main' },
  ],
  grouped: {
    hq: [{ name: 'hq-mayor', windows: 1, attached: false, group: 'hq' }],
    main: [{ name: 'main', windows: 2, attached: true, group: 'main' }],
  },
  timestamp: '2026-05-30T00:00:00Z',
}

describe('mergeTmuxAndGasCitySessions', () => {
  it('adds Gas City identities as read-only session choices with gc attach targets', () => {
    const observer: GasCityObserver = {
      status: 'ok',
      checkedAt: '2026-05-30T00:00:01Z',
      sessions: [
        {
          source: 'gascity',
          city: 'gascity',
          id: 'gc-1',
          alias: 'planner',
          title: 'Planner',
          template: 'codex',
          status: 'active',
          attachTarget: 'gc:gc-1',
          running: true,
          attached: true,
        },
      ],
    }

    const merged = mergeTmuxAndGasCitySessions(tmuxResponse, observer)

    expect(merged.sessions.map(session => session.name)).toEqual(['hq-mayor', 'main', 'gc:gc-1'])
    expect(merged.grouped.gascity).toHaveLength(1)
    expect(merged.grouped.gascity[0]).toMatchObject({
      name: 'gc:gc-1',
      source: 'gascity',
      attachTarget: 'gc:gc-1',
      displayName: 'planner',
      gasCityId: 'gc-1',
      gasCityCity: 'gascity',
      attached: true,
      group: 'gascity',
    })
  })

  it('keeps plain tmux sessions when Gas City is unavailable', () => {
    const observer: GasCityObserver = {
      status: 'unavailable',
      checkedAt: '2026-05-30T00:00:01Z',
      error: 'Gas City supervisor unavailable',
      sessions: [],
    }

    const merged = mergeTmuxAndGasCitySessions(tmuxResponse, observer)

    expect(merged.sessions).toEqual(tmuxResponse.sessions)
    expect(merged.grouped).toEqual(tmuxResponse.grouped)
  })
})
