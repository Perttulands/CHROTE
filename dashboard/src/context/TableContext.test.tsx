import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import {
  TABLE_WIDTH_DEFAULT,
  TABLE_WIDTH_MIN,
  clampTableWidth,
  clearTable,
  dismissTable,
  resetTableForTest,
  tableLabel,
  useTableObject,
} from './TableContext'
import { followBeadFromCard, openBeadCard } from '../beads/beadCard'
import { openAgentContext } from '../agents/agentContextPanel'

const AGENT = { sessionKey: 'alice:jack', folder: '/srv/chrote', harness: 'claude-code' as const, user: 'alice', shell: false }

afterEach(() => {
  resetTableForTest()
})

describe('the table', () => {
  it('holds one object: putting a Bead down replaces what an agent sees', () => {
    const { result } = renderHook(() => useTableObject())
    expect(result.current).toBeNull()

    act(() => openAgentContext(AGENT))
    expect(result.current).toMatchObject({ kind: 'agent-context', sessionKey: 'alice:jack' })

    act(() => openBeadCard('chrote-5grx.47', '/srv/chrote'))
    expect(result.current).toMatchObject({ kind: 'bead', id: 'chrote-5grx.47', projectPath: '/srv/chrote', trail: [] })

    act(() => clearTable())
    expect(result.current).toBeNull()
  })

  // A Bead reached from another Bead's text is a step, not a new sitting:
  // Escape retraces the steps, and only the first Bead's Escape puts it away.
  it('retraces a trail of followed Beads before it puts the table away', () => {
    const { result } = renderHook(() => useTableObject())
    act(() => openBeadCard('chrote-5grx'))
    act(() => followBeadFromCard('chrote-5grx.47'))
    act(() => followBeadFromCard('chrote-5grx.25'))
    expect(result.current).toMatchObject({ id: 'chrote-5grx.25', trail: ['chrote-5grx', 'chrote-5grx.47'] })

    act(() => dismissTable())
    expect(result.current).toMatchObject({ id: 'chrote-5grx.47', trail: ['chrote-5grx'] })

    act(() => dismissTable())
    expect(result.current).toMatchObject({ id: 'chrote-5grx', trail: [] })

    act(() => dismissTable())
    expect(result.current).toBeNull()
  })

  it('names what it holds, for the reader who cannot see it', () => {
    expect(tableLabel({ kind: 'bead', id: 'chrote-5grx.47', trail: [], nonce: 1 })).toBe('Bead chrote-5grx.47')
    expect(tableLabel({ kind: 'agent-context', ...AGENT, nonce: 1 })).toBe('What jack sees')
    expect(tableLabel({ kind: 'file', path: '/srv/chrote/README.md', nonce: 1 })).toBe('/srv/chrote/README.md')
  })
})

describe('the column width', () => {
  it('honours nothing narrower than the minimum, and falls back from anything that is not a width', () => {
    expect(clampTableWidth(520.4)).toBe(520)
    expect(clampTableWidth(TABLE_WIDTH_MIN)).toBe(TABLE_WIDTH_MIN)
    expect(clampTableWidth(100)).toBe(TABLE_WIDTH_MIN)
    expect(clampTableWidth(undefined)).toBe(TABLE_WIDTH_DEFAULT)
    expect(clampTableWidth('wide')).toBe(TABLE_WIDTH_DEFAULT)
    expect(clampTableWidth(Number.NaN)).toBe(TABLE_WIDTH_DEFAULT)
  })
})
