import { beforeEach, describe, expect, it } from 'vitest'
import { featureFlagKey, isFeatureEnabled, setFeatureEnabled } from './featureFlags'

/* The Formations flags were retired when Formations became always-on, so this
   pins the generic flag mechanism (default, persisted opt-out, persisted opt-in)
   against a surviving default-on flag instead. */
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
