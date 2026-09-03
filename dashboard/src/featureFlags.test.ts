import { beforeEach, describe, expect, it } from 'vitest'
import { featureFlagKey, isFeatureEnabled, setFeatureEnabled } from './featureFlags'

/* Pin the generic flag mechanism (default, persisted opt-out, persisted opt-in)
   against a default-on flag. */
describe('feature flag mechanism', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('answers the configured default until the operator persists a choice either way', () => {
    expect(featureFlagKey('serverStatusTab')).toBe('chrote-server-status-tab')
    expect(isFeatureEnabled('serverStatusTab')).toBe(true)

    setFeatureEnabled('serverStatusTab', false)
    expect(isFeatureEnabled('serverStatusTab')).toBe(false)

    setFeatureEnabled('serverStatusTab', true)
    expect(isFeatureEnabled('serverStatusTab')).toBe(true)
  })
})
