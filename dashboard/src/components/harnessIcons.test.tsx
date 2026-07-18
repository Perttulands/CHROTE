import { describe, expect, it } from 'vitest'
import { HARNESS_ICONS, harnessIcon, harnessIdFor } from './harnessIcons'

describe('harness icon library', () => {
  it('maps harness strings to their product marks', () => {
    expect(harnessIdFor('claude-code')).toBe('claude-code')
    expect(harnessIdFor('anthropic-api')).toBe('claude-code')
    expect(harnessIdFor('codex')).toBe('codex')
    expect(harnessIdFor('openai-codex')).toBe('codex')
    expect(harnessIdFor('opencode')).toBe('opencode')
    expect(harnessIdFor('pi')).toBe('pi')
    expect(harnessIdFor('pi-agent')).toBe('pi')
    expect(harnessIdFor('hermes-athena')).toBe('hermes')
  })

  it('falls back to the terminal mark for unknown harnesses, null for none', () => {
    expect(harnessIdFor('some-new-cli')).toBe('terminal')
    expect(harnessIdFor('')).toBeNull()
    expect(harnessIdFor(undefined)).toBeNull()
    expect(harnessIcon(null)).toBeNull()
  })

  it('does not read "pi" out of unrelated substrings', () => {
    expect(harnessIdFor('api-harness')).toBe('terminal')
    expect(harnessIdFor('pipeline')).toBe('terminal')
  })

  it('has an icon for every declared harness id', () => {
    for (const [id, icon] of Object.entries(HARNESS_ICONS)) {
      expect(icon, id).toBeTruthy()
    }
  })
})
