import { describe, expect, it } from 'vitest'
import { isOptionalSafeBeadsIssueID, isSafeBeadsIssueID } from './formationsBeadId'

describe('Formations Beads issue id validation', () => {
  it('accepts safe project-local Beads issue ids', () => {
    for (const beadID of ['home-pfyv', 'home-vdki.8', 'home-vdki.34.1', 'chlab-123', 'srv-abc.2', 'bd-204']) {
      expect(isSafeBeadsIssueID(beadID)).toBe(true)
    }
  })

  it('rejects unsafe Beads issue ids', () => {
    for (const beadID of ['', 'nohyphen', 'Home-123', 'chlab', 'chlab-', 'chlab/123', '../home-pfyv', 'home-pfyv/evil', 'home-pfyv\n']) {
      expect(isSafeBeadsIssueID(beadID)).toBe(false)
    }
  })

  it('allows empty optional Beads anchors', () => {
    expect(isOptionalSafeBeadsIssueID('')).toBe(true)
    expect(isOptionalSafeBeadsIssueID('srv-abc.2')).toBe(true)
    expect(isOptionalSafeBeadsIssueID('chlab/123')).toBe(false)
  })
})
