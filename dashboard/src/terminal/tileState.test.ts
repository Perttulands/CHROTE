import { describe, expect, it } from 'vitest'
import { isDetached, liveSessionKeys, tileStateFor } from './tileState'

const live = liveSessionKeys([
  { name: 'main', windows: 1, attached: false, group: 'main' },
  { name: 'build', windows: 1, attached: false, group: 'build', unixUser: 'forge' },
])

describe('liveSessionKeys', () => {
  it('matches a binding written either qualified or bare', () => {
    expect(live.has('forge:build')).toBe(true)
    expect(live.has('build')).toBe(true)
    expect(live.has('main')).toBe(true)
    expect(live.has('gone')).toBe(false)
  })
})

describe('tileStateFor', () => {
  it('is Live while the shown binding holds an open connection', () => {
    expect(tileStateFor({ sessionKey: 'main', onScreen: true, liveSessions: live, connection: 'open' }))
      .toBe('live')
  })

  it('is Live, not lost, while a shown binding is still dialling', () => {
    expect(tileStateFor({ sessionKey: 'main', onScreen: true, liveSessions: live, connection: 'connecting' }))
      .toBe('live')
    expect(tileStateFor({ sessionKey: 'main', onScreen: true, liveSessions: live, connection: 'idle' }))
      .toBe('live')
  })

  it('is Idle for a binding that is not the one on screen', () => {
    expect(tileStateFor({ sessionKey: 'main', onScreen: false, liveSessions: live, connection: 'idle' }))
      .toBe('idle')
    expect(tileStateFor({ sessionKey: 'main', onScreen: false, liveSessions: live, connection: 'open' }))
      .toBe('idle')
  })

  it('is Taken over when the connection dropped but tmux still lists the session', () => {
    expect(tileStateFor({ sessionKey: 'forge:build', onScreen: true, liveSessions: live, connection: 'closed' }))
      .toBe('takenOver')
  })

  it('is Ended when tmux no longer lists the session, on screen or not', () => {
    expect(tileStateFor({ sessionKey: 'gone', onScreen: true, liveSessions: live, connection: 'closed' }))
      .toBe('ended')
    expect(tileStateFor({ sessionKey: 'gone', onScreen: false, liveSessions: live, connection: 'idle' }))
      .toBe('ended')
  })

  it('never calls a binding ended before a trustworthy session list has arrived', () => {
    expect(tileStateFor({ sessionKey: 'gone', onScreen: true, liveSessions: null, connection: 'closed' }))
      .toBe('takenOver')
    expect(tileStateFor({ sessionKey: 'gone', onScreen: false, liveSessions: null, connection: 'idle' }))
      .toBe('idle')
  })

  it('trusts its own open connection over a poll that has not caught up with a restart', () => {
    expect(tileStateFor({ sessionKey: 'gone', onScreen: true, liveSessions: live, connection: 'open' }))
      .toBe('live')
  })

  it('groups exactly the two states that show a last frame with an action', () => {
    expect(isDetached('takenOver')).toBe(true)
    expect(isDetached('ended')).toBe(true)
    expect(isDetached('live')).toBe(false)
    expect(isDetached('idle')).toBe(false)
  })
})
