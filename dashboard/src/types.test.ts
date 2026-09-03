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
  it('names every group as the operator wrote it, and orders main first and ungrouped last', () => {
    expect(getGroupDisplayName('project')).toBe('project')
    expect(getGroupDisplayName('team-tools')).toBe('team-tools')
    expect(getGroupDisplayName('main')).toBe('Main')

    expect(getGroupPriority('project')).toBe(getGroupPriority('team-tools'))
    expect(getGroupPriority('main')).toBeLessThan(getGroupPriority('project'))
    expect(getGroupPriority('project')).toBeLessThan(getGroupPriority('other'))
  })
})

describe('normalizeTerminalTabCount', () => {
  // Whatever the setting or old storage hands over, the answer is a count the
  // tab bar can actually draw.
  it('answers every shape with a count the tab bar can draw', () => {
    const answers: [unknown, number][] = [
      [1, 1],
      [3, 3],
      [6, 6],
      [0, MIN_TERMINAL_TAB_COUNT],
      [-4, MIN_TERMINAL_TAB_COUNT],
      [99, MAX_TERMINAL_TAB_COUNT],
      [Number.POSITIVE_INFINITY, MAX_TERMINAL_TAB_COUNT],
      [Number.NEGATIVE_INFINITY, MIN_TERMINAL_TAB_COUNT],
      [2.9, 2],
      [undefined, DEFAULT_TERMINAL_TAB_COUNT],
      [null, DEFAULT_TERMINAL_TAB_COUNT],
      ['5', DEFAULT_TERMINAL_TAB_COUNT],
      [Number.NaN, DEFAULT_TERMINAL_TAB_COUNT],
      [{}, DEFAULT_TERMINAL_TAB_COUNT],
    ]

    for (const [given, expected] of answers) {
      expect(normalizeTerminalTabCount(given)).toBe(expected)
    }
  })
})

describe('terminalWorkspaceIds', () => {
  it('derives terminal1..terminalN, and the exported list is that derivation at the default count', () => {
    expect(terminalWorkspaceIds(1)).toEqual(['terminal1'])
    expect(terminalWorkspaceIds(4)).toEqual(['terminal1', 'terminal2', 'terminal3', 'terminal4'])

    expect(DEFAULT_TERMINAL_TAB_COUNT).toBe(3)
    expect(TERMINAL_WORKSPACE_IDS).toEqual(['terminal1', 'terminal2', 'terminal3'])
    expect(TERMINAL_WORKSPACE_IDS).toEqual(terminalWorkspaceIds())
  })
})

describe('getTerminalLabel', () => {
  // The first tab is the plain one; every other carries its number, including
  // past the default count.
  it('labels the first workspace Terminal and every other by its number', () => {
    expect(getTerminalLabel('terminal1')).toBe('Terminal')
    expect(getTerminalLabel('terminal2')).toBe('Terminal 2')
    expect(getTerminalLabel('terminal3')).toBe('Terminal 3')
    expect(getTerminalLabel('terminal12')).toBe('Terminal 12')
  })
})

describe('isTerminalWorkspaceId', () => {
  it('answers membership of the canonical list, or of an explicit one', () => {
    expect(isTerminalWorkspaceId('terminal1')).toBe(true)
    expect(isTerminalWorkspaceId('terminal3')).toBe(true)

    expect(isTerminalWorkspaceId('terminal4')).toBe(false)
    expect(isTerminalWorkspaceId('terminal')).toBe(false)
    expect(isTerminalWorkspaceId('files')).toBe(false)
    expect(isTerminalWorkspaceId(3)).toBe(false)
    expect(isTerminalWorkspaceId(null)).toBe(false)

    // A grown tab bar passes its own list, so terminal4 is a member there and
    // terminal3 is not a member of a shrunken one.
    expect(isTerminalWorkspaceId('terminal4', terminalWorkspaceIds(4))).toBe(true)
    expect(isTerminalWorkspaceId('terminal3', terminalWorkspaceIds(2))).toBe(false)
  })
})

describe('getDefaultLaunchUser terminal3 rule', () => {
  it('gives terminal3 the second configured user, and every other tab the first', () => {
    expect(getDefaultLaunchUser('terminal3', ['alice', 'build'])).toBe('build')
    expect(getDefaultLaunchUser('terminal1', ['alice', 'build'])).toBe('alice')
    expect(getDefaultLaunchUser('terminal2', ['alice', 'build'])).toBe('alice')
    expect(getDefaultLaunchUser('terminal4', ['alice', 'build'])).toBe('alice')

    // With no second user configured there is nothing to prefer, so terminal3
    // launches as everyone else does.
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

    // One viewer is the ordinary case and says nothing.
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
