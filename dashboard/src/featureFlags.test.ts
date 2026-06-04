import { beforeEach, describe, expect, it } from 'vitest'
import { featureFlagKey, isFeatureEnabled } from './featureFlags'

describe('Formations feature flag', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('pins the persisted key and keeps Formations disabled by default', () => {
    expect(featureFlagKey('formations')).toBe('chrote-formations')
    expect(isFeatureEnabled('formations')).toBe(false)
  })

  it('allows a persisted opt-in for Formations', () => {
    localStorage.setItem('chrote-formations', '1')

    expect(isFeatureEnabled('formations')).toBe(true)
  })
})
