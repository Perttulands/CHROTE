import { describe, expect, it } from 'vitest'
import {
  DEFAULT_TERMINAL_TAB_COUNT,
  MAX_TERMINAL_TAB_COUNT,
  MIN_TERMINAL_TAB_COUNT,
  TERMINAL_WORKSPACE_IDS,
  getDefaultLaunchUser,
  getGroupDisplayName,
  getGroupPriority,
  getTerminalLabel,
  isTerminalWorkspaceId,
  normalizeTerminalTabCount,
  terminalWorkspaceIds,
} from './types'

describe('session group presentation', () => {
  it('presents arbitrary name-prefix groups without harness-specific rewriting', () => {
    expect(getGroupDisplayName('project')).toBe('project')
    expect(getGroupDisplayName('team-tools')).toBe('team-tools')
    expect(getGroupPriority('project')).toBe(getGroupPriority('team-tools'))
  })

  it('keeps only the product-wide main and ungrouped ordering', () => {
    expect(getGroupDisplayName('main')).toBe('Main')
    expect(getGroupPriority('main')).toBeLessThan(getGroupPriority('project'))
    expect(getGroupPriority('project')).toBeLessThan(getGroupPriority('other'))
  })
})

describe('normalizeTerminalTabCount', () => {
  it('passes through integers in range', () => {
    expect(normalizeTerminalTabCount(1)).toBe(1)
    expect(normalizeTerminalTabCount(3)).toBe(3)
    expect(normalizeTerminalTabCount(6)).toBe(6)
  })

  it('clamps out-of-range numbers', () => {
    expect(normalizeTerminalTabCount(0)).toBe(MIN_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount(-4)).toBe(MIN_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount(99)).toBe(MAX_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount(Number.POSITIVE_INFINITY)).toBe(MAX_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount(Number.NEGATIVE_INFINITY)).toBe(MIN_TERMINAL_TAB_COUNT)
  })

  it('floors fractional counts', () => {
    expect(normalizeTerminalTabCount(2.9)).toBe(2)
  })

  it('defaults every non-numeric shape', () => {
    expect(normalizeTerminalTabCount(undefined)).toBe(DEFAULT_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount(null)).toBe(DEFAULT_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount('5')).toBe(DEFAULT_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount(Number.NaN)).toBe(DEFAULT_TERMINAL_TAB_COUNT)
    expect(normalizeTerminalTabCount({})).toBe(DEFAULT_TERMINAL_TAB_COUNT)
  })
})

describe('terminalWorkspaceIds', () => {
  it('derives terminal1..terminalN for the requested count', () => {
    expect(terminalWorkspaceIds(1)).toEqual(['terminal1'])
    expect(terminalWorkspaceIds(4)).toEqual(['terminal1', 'terminal2', 'terminal3', 'terminal4'])
  })

  it('defaults to the canonical count and TERMINAL_WORKSPACE_IDS is that derivation', () => {
    expect(DEFAULT_TERMINAL_TAB_COUNT).toBe(3)
    expect(TERMINAL_WORKSPACE_IDS).toEqual(['terminal1', 'terminal2', 'terminal3'])
    expect(TERMINAL_WORKSPACE_IDS).toEqual(terminalWorkspaceIds())
  })
})

describe('getTerminalLabel', () => {
  it('labels the first workspace as plain Terminal', () => {
    expect(getTerminalLabel('terminal1')).toBe('Terminal')
  })

  it('labels later workspaces by their number, beyond the default count too', () => {
    expect(getTerminalLabel('terminal2')).toBe('Terminal 2')
    expect(getTerminalLabel('terminal3')).toBe('Terminal 3')
    expect(getTerminalLabel('terminal12')).toBe('Terminal 12')
  })
})

describe('isTerminalWorkspaceId', () => {
  it('accepts members of the default id list', () => {
    expect(isTerminalWorkspaceId('terminal1')).toBe(true)
    expect(isTerminalWorkspaceId('terminal3')).toBe(true)
  })

  it('rejects ids outside the list and non-strings', () => {
    expect(isTerminalWorkspaceId('terminal4')).toBe(false)
    expect(isTerminalWorkspaceId('terminal')).toBe(false)
    expect(isTerminalWorkspaceId('files')).toBe(false)
    expect(isTerminalWorkspaceId(3)).toBe(false)
    expect(isTerminalWorkspaceId(null)).toBe(false)
  })

  it('honors an explicit id list', () => {
    expect(isTerminalWorkspaceId('terminal4', terminalWorkspaceIds(4))).toBe(true)
    expect(isTerminalWorkspaceId('terminal3', terminalWorkspaceIds(2))).toBe(false)
  })
})

describe('getDefaultLaunchUser terminal3 rule', () => {
  it('keeps terminal3 defaulting to the second configured Unix user', () => {
    expect(getDefaultLaunchUser('terminal3', ['alice', 'build'])).toBe('build')
  })

  it('defaults every other workspace to the first configured user', () => {
    expect(getDefaultLaunchUser('terminal1', ['alice', 'build'])).toBe('alice')
    expect(getDefaultLaunchUser('terminal2', ['alice', 'build'])).toBe('alice')
    expect(getDefaultLaunchUser('terminal4', ['alice', 'build'])).toBe('alice')
  })

  it('falls back to the first user when no second user exists', () => {
    expect(getDefaultLaunchUser('terminal3', ['alice'])).toBe('alice')
    expect(getDefaultLaunchUser('terminal3', [])).toBe('')
  })
})
