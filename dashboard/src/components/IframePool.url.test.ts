import { describe, expect, it } from 'vitest'
import { getTerminalUrl } from './IframePool'

describe('getTerminalUrl', () => {
  it('encodes Gas City attach targets without stripping the gc prefix', () => {
    expect(getTerminalUrl('gc:gc-1')).toContain('/terminal/?arg=gc%3Agc-1')
  })
})
