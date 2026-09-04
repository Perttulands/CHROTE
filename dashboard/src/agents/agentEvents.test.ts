import { describe, expect, it } from 'vitest'
import { bindingsCarryMark, summaryLine, takeInAgentEvents } from './agentEvents'
import type { AgentEvent, TmuxSession } from '../types'

const session = (name: string, lastEvent?: AgentEvent, unixUser = 'chrote'): TmuxSession => ({
  name, windows: 1, attached: false, group: 'claude', unixUser, ...(lastEvent ? { lastEvent } : {}),
})

const finished = (time: string, seen = false): AgentEvent => ({ event: 'finished', time, seen, summary: 'Wrote the tests' })

describe('takeInAgentEvents', () => {
  // A reload must not replay the afternoon: whatever the first list carries,
  // seen or not, is history.
  it('seeds from the first list without telling anything', () => {
    const taken = takeInAgentEvents(null, [session('builder', finished('2026-09-03T10:00:00.000Z'))])
    expect(taken.notices).toEqual([])
    expect(taken.ledger.get('chrote:builder')).toBe('2026-09-03T10:00:00.000Z')
  })

  it('tells an event with a time the ledger did not hold, once, and only while it is unseen', () => {
    const seeded = takeInAgentEvents(null, [session('builder', finished('2026-09-03T10:00:00.000Z'))]).ledger

    const news = takeInAgentEvents(seeded, [session('builder', finished('2026-09-03T10:05:00.000Z'))])
    expect(news.notices).toEqual([{
      sessionKey: 'chrote:builder', session: 'builder', unixUser: 'chrote',
      event: 'finished', time: '2026-09-03T10:05:00.000Z', summary: 'Wrote the tests',
    }])

    // The same event on the next poll is not news again, and neither is the
    // poll that reports it seen from another device.
    expect(takeInAgentEvents(news.ledger, [session('builder', finished('2026-09-03T10:05:00.000Z'))]).notices).toEqual([])
    expect(takeInAgentEvents(news.ledger, [session('builder', finished('2026-09-03T10:10:00.000Z', true))]).notices).toEqual([])
  })

  it('tells the event of a session that appeared after the first list', () => {
    const seeded = takeInAgentEvents(null, [session('architect')]).ledger
    const taken = takeInAgentEvents(seeded, [
      session('architect'),
      session('builder', { event: 'needs-input', time: '2026-09-03T10:05:00.000Z', seen: false }),
    ])
    expect(taken.notices.map(notice => [notice.session, notice.event])).toEqual([['builder', 'needs-input']])
  })

  it('forgets a session the list no longer shows', () => {
    const seeded = takeInAgentEvents(null, [session('builder', finished('2026-09-03T10:00:00.000Z'))]).ledger
    expect(takeInAgentEvents(seeded, []).ledger.size).toBe(0)
  })
})

describe('summaryLine', () => {
  it('keeps a short summary whole and cuts a long one at sixty characters with a mark', () => {
    expect(summaryLine('  Wrote the tests  ')).toBe('Wrote the tests')
    const long = 'a'.repeat(61)
    expect(summaryLine(long)).toBe(`${'a'.repeat(60)}…`)
  })
})

describe('bindingsCarryMark', () => {
  const sessions = [session('builder', finished('2026-09-03T10:05:00.000Z'))]
  const marked = new Set(['chrote:builder'])

  it('matches a binding by its key, or by the bare name a binding carried before keys named a user', () => {
    expect(bindingsCarryMark(marked, sessions, ['chrote:architect', 'chrote:builder'])).toBe(true)
    expect(bindingsCarryMark(marked, sessions, ['builder'])).toBe(true)
    expect(bindingsCarryMark(marked, sessions, ['chrote:architect'])).toBe(false)
    expect(bindingsCarryMark(new Set(), sessions, ['chrote:builder'])).toBe(false)
  })
})
