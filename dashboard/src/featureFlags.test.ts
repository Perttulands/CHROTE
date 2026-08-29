import { beforeEach, describe, expect, it } from 'vitest'
import { featureFlagKey, isFeatureEnabled, setFeatureEnabled } from './featureFlags'

/* Pin the generic flag mechanism (default, persisted opt-out, persisted opt-in)
   against a default-on flag. */
describe('feature flag mechanism', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('returns the configured default when the flag is unset', () => {
    expect(featureFlagKey('serverStatusTab')).toBe('chrote-server-status-tab')
    expect(isFeatureEnabled('serverStatusTab')).toBe(true)
  })

  it('honors a persisted opt-out and a persisted opt-in', () => {
    setFeatureEnabled('serverStatusTab', false)
    expect(isFeatureEnabled('serverStatusTab')).toBe(false)

    setFeatureEnabled('serverStatusTab', true)
    expect(isFeatureEnabled('serverStatusTab')).toBe(true)
  })
})
