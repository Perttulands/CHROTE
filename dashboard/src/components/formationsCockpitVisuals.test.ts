import { describe, expect, it } from 'vitest'
import { TYPE_TAG, agentColor, agentRole, agentState, initials } from './formationsCockpitVisuals'
import type { AgentProjection } from './formationsTypes'

const agent = (over: Partial<AgentProjection> & { assignable: boolean }): AgentProjection => ({ id: 'a', ...over })

describe('agentColor + initials', () => {
  it('produce a deterministic gradient per id', () => {
    expect(agentColor('mason')).toBe(agentColor('mason'))
    expect(agentColor('mason')).toMatch(/^radial-gradient\(hsl\(/)
    expect(agentColor('mason')).not.toBe(agentColor('hazel'))
  })
  it('take the first two alphanumerics, uppercased', () => {
    expect(initials('lab-poet')).toBe('LA')
    expect(initials('Z')).toBe('Z')
    expect(initials('___')).toBe('?')
  })
})

describe('agentRole', () => {
  it('prefers the default harness, then unbound, then a generic agent label', () => {
    expect(agentRole(agent({ assignable: true, harnessDefault: 'openai-codex' }))).toBe('openai-codex')
    expect(agentRole(agent({ assignable: false, unbound: true }))).toBe('unbound')
    expect(agentRole(agent({ assignable: true }))).toBe('agent')
  })
})

describe('agentState', () => {
  it('is on when live or assignable, otherwise idle', () => {
    expect(agentState(agent({ assignable: false, liveness: 'live' }))).toBe('on')
    expect(agentState(agent({ assignable: true, liveness: 'dead' }))).toBe('on')
    expect(agentState(agent({ assignable: false, liveness: 'dead' }))).toBe('idle')
  })
})

describe('TYPE_TAG', () => {
  it('has a tagline for every formation type', () => {
    expect(Object.keys(TYPE_TAG).sort()).toEqual(['flow', 'orchestrated', 'peer', 'solo'])
  })
})
