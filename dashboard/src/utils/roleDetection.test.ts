import { describe, it, expect } from 'vitest'
import { detectAgentRole } from './roleDetection'

describe('detectAgentRole', () => {
  it('detects mayor role', () => {
    expect(detectAgentRole('mayor')).toBe('mayor')
    expect(detectAgentRole('hq-mayor-1')).toBe('mayor')
    expect(detectAgentRole('chrote-mayor-service')).toBe('mayor')
  })

  it('detects deacon role', () => {
    expect(detectAgentRole('deacon')).toBe('deacon')
    expect(detectAgentRole('hq-deacon')).toBe('deacon')
  })

  it('detects polecat role', () => {
    expect(detectAgentRole('polecat')).toBe('polecat')
    expect(detectAgentRole('gt-my-job-pc-1')).toBe('polecat')
    expect(detectAgentRole('job-pc')).toBe('polecat')
  })

  it('detects refinery role', () => {
    expect(detectAgentRole('refinery')).toBe('refinery')
    expect(detectAgentRole('job-refinery')).toBe('refinery')
  })

  it('detects crew role', () => {
    expect(detectAgentRole('crew')).toBe('crew')
    expect(detectAgentRole('job-crew-1')).toBe('crew')
  })

  it('returns null for unknown roles', () => {
    expect(detectAgentRole('random-session')).toBe(null)
    expect(detectAgentRole('unknown')).toBe(null)
  })

  it('is case insensitive', () => {
    expect(detectAgentRole('MAYOR')).toBe('mayor')
    expect(detectAgentRole('Job-Crew-1')).toBe('crew')
  })
})
