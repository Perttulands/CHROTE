import { describe, expect, it } from 'vitest'
import type { TmuxSession } from './types'
import {
  DEFAULT_TERMINAL_TAB_COUNT,
  MAX_TERMINAL_TAB_COUNT,
  MIN_TERMINAL_TAB_COUNT,
  TERMINAL_WORKSPACE_IDS,
  getDefaultLaunchUser,
  getGroupDisplayName,
  getGroupPriority,
  getSessionBadges,
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

describe('getSessionBadges', () => {
  const plain: TmuxSession = {
    name: 'alice-shell',
    windows: 1,
    attached: true,
    group: 'main',
    panes: 1,
    width: 120,
    height: 40,
    mouseEnabled: true,
  }

  it('says nothing about a session that is what it looks like', () => {
    expect(getSessionBadges(plain)).toEqual([])
  })

  it('reports a pinned window with the size CHROTE cannot change', () => {
    const [badge] = getSessionBadges({ ...plain, sizePinned: true, width: 100, height: 30 })
    expect(badge.id).toBe('pinned-size')
    expect(badge.detail).toContain('100x30')
    expect(badge.detail).toContain('window-size is manual')
  })

  it('names the foreign clients now watching alongside CHROTE', () => {
    const [badge] = getSessionBadges({ ...plain, foreignClients: ['/dev/pts/12'] })
    expect(badge.id).toBe('foreign-client')
    expect(badge.detail).toContain('/dev/pts/12')
    expect(badge.detail).toContain('without disconnecting them')
  })

  // tmux draws one grid per window however many are watching, so a second
  // viewer means this pane is showing somebody else's dimensions. A glance
  // cannot tell the operator that, which is the whole membership test.
  it('says when a session is watched by more than one client, and at what size', () => {
    const badge = getSessionBadges({ ...plain, viewers: 2, width: 200, height: 50 })
      .find(candidate => candidate.id === 'shared-view')
    expect(badge?.label).toBe('Watched by 2')
    expect(badge?.detail).toContain('200x50')
    expect(badge?.detail).toContain('the size the claiming one set')
  })

  it('raises no shared-view claim for a session with one viewer or none', () => {
    for (const viewers of [undefined, 0, 1]) {
      expect(getSessionBadges({ ...plain, viewers }).map(badge => badge.id)).not.toContain('shared-view')
    }
  })

  it('reports structure the session list cannot show', () => {
    expect(getSessionBadges({ ...plain, windows: 3 })[0].detail).toContain('3 tmux windows')
    expect(getSessionBadges({ ...plain, panes: 2 })[0].detail).toContain('2 panes in the current window')
    expect(getSessionBadges({ ...plain, windows: 3, panes: 2 })[0].detail)
      .toBe('This session has 3 tmux windows and 2 panes in the current window. A terminal shows the current window only.')
  })

  it('reports mouse mode only when the server said it is off', () => {
    expect(getSessionBadges({ ...plain, mouseEnabled: false }).map(badge => badge.id)).toEqual(['mouse-off'])
    expect(getSessionBadges({ ...plain, mouseEnabled: undefined })).toEqual([])
  })

  it('raises no claim from a session the server described without the new facts', () => {
    expect(getSessionBadges({ name: 'legacy', windows: 1, attached: false, group: 'other' })).toEqual([])
  })

  it('carries every fact at once and keeps the markers distinct', () => {
    const badges = getSessionBadges({
      ...plain,
      windows: 2,
      panes: 2,
      sizePinned: true,
      mouseEnabled: false,
      foreignClients: ['/dev/pts/12', '/dev/pts/13'],
    })
    expect(badges.map(badge => badge.id)).toEqual(['pinned-size', 'foreign-client', 'structure', 'mouse-off'])
    expect(new Set(badges.map(badge => badge.marker)).size).toBe(4)
    expect(badges[1].detail).toContain('2 clients')
  })
})
