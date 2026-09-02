import { describe, expect, it } from 'vitest'
import {
  isDetached,
  isSessionEnded,
  liveSessionKeys,
  retainSessionEvidence,
  sessionEvidenceFrom,
  tileStateFor,
  NO_SESSION_EVIDENCE,
  type SessionEvidence,
  type TileStateInput,
} from './tileState'
import type { TmuxSession } from '../types'

const sessions: TmuxSession[] = [
  { name: 'main', windows: 1, attached: false, group: 'main' },
  { name: 'build', windows: 1, attached: false, group: 'build', unixUser: 'forge' },
]

const live = liveSessionKeys(sessions)
const heard: SessionEvidence = { live, answering: null }

const tileState = (input: TileStateInput) => tileStateFor(input)

describe('liveSessionKeys', () => {
  it('matches a binding written either qualified or bare', () => {
    expect(live.has('forge:build')).toBe(true)
    expect(live.has('build')).toBe(true)
    expect(live.has('main')).toBe(true)
    expect(live.has('gone')).toBe(false)
  })
})

describe('sessionEvidenceFrom', () => {
  it('claims nothing until the first response lands', () => {
    expect(sessionEvidenceFrom({ sessions, loading: true, error: null, partialAnsweringUsers: null }))
      .toEqual(NO_SESSION_EVIDENCE)
  })

  it('claims nothing when the poll failed outright', () => {
    expect(sessionEvidenceFrom({ sessions, loading: false, error: 'Failed to fetch sessions', partialAnsweringUsers: null }))
      .toEqual(NO_SESSION_EVIDENCE)
  })

  it('joins every binding against a whole response', () => {
    const evidence = sessionEvidenceFrom({ sessions, loading: false, error: null, partialAnsweringUsers: null })
    expect(evidence.live?.has('forge:build')).toBe(true)
    expect(evidence.answering).toBeNull()
  })

  it('keeps the answering users of a partial response, error and all', () => {
    const evidence = sessionEvidenceFrom({
      sessions,
      loading: false,
      error: "tmux failed for user 'alice'",
      partialAnsweringUsers: ['forge'],
    })
    expect(evidence.live?.has('forge:build')).toBe(true)
    expect([...(evidence.answering ?? [])]).toEqual(['forge'])
  })
})

describe('isSessionEnded during a partial outage', () => {
  // 'forge' answered and no longer lists 'forge:gone'; 'alice' never answered,
  // so its list is whatever the previous poll left behind.
  const partial: SessionEvidence = { live, answering: new Set(['forge']) }

  it('declares a dead binding under an answering user ended', () => {
    expect(isSessionEnded('forge:gone', partial)).toBe(true)
  })

  it('leaves a live binding under an answering user alone', () => {
    expect(isSessionEnded('forge:build', partial)).toBe(false)
  })

  it('holds a binding under the user whose socket failed', () => {
    expect(isSessionEnded('alice:gone', partial)).toBe(false)
  })

  it('holds a bare binding, which names no user to scope by', () => {
    expect(isSessionEnded('gone', partial)).toBe(false)
  })

  it('joins every binding once the response is whole again', () => {
    expect(isSessionEnded('alice:gone', heard)).toBe(true)
    expect(isSessionEnded('gone', heard)).toBe(true)
  })

  it('declares nothing at all without evidence', () => {
    expect(isSessionEnded('gone', NO_SESSION_EVIDENCE)).toBe(false)
  })
})

describe('retainSessionEvidence', () => {
  const failed = sessionEvidenceFrom({ sessions, loading: false, error: 'Failed to fetch sessions', partialAnsweringUsers: null })

  it('holds the last answer through a poll that reports nothing', () => {
    expect(retainSessionEvidence(heard, failed)).toBe(heard)
  })

  it('lets a poll that answered replace what is held', () => {
    const fresh = sessionEvidenceFrom({ sessions, loading: false, error: null, partialAnsweringUsers: null })
    expect(retainSessionEvidence(heard, fresh)).toBe(fresh)
  })

  it('still claims nothing when nothing has ever been heard', () => {
    expect(retainSessionEvidence(NO_SESSION_EVIDENCE, failed)).toEqual(NO_SESSION_EVIDENCE)
  })

  it('keeps an Ended verdict Ended instead of offering Reclaim on a session that is gone', () => {
    const beforeFailure = tileState({ sessionKey: 'gone', onScreen: true, evidence: heard, connection: 'closed' })
    expect(beforeFailure).toBe('ended')
    // The poll then fails. Without the hold this reads 'takenOver', and the
    // tile offers Reclaim on a session tmux does not have.
    expect(tileState({
      sessionKey: 'gone',
      onScreen: true,
      evidence: retainSessionEvidence(heard, failed),
      connection: 'closed',
    })).toBe('ended')
  })

  it('holds an Ended verdict through a response that cannot speak for that user', () => {
    const partial = sessionEvidenceFrom({
      sessions,
      loading: false,
      error: "tmux failed for user 'alice'",
      partialAnsweringUsers: ['forge'],
    })
    const retained = retainSessionEvidence(heard, partial)
    // 'alice' did not answer this poll, so it overturns nothing the last whole
    // one said: the session is still gone and the tile still offers Restart.
    expect(tileState({ sessionKey: 'alice:gone', onScreen: true, evidence: retained, connection: 'closed' }))
      .toBe('ended')
    // The user that did answer is still joined against the fresh response.
    expect(tileState({ sessionKey: 'forge:gone', onScreen: true, evidence: retained, connection: 'closed' }))
      .toBe('ended')
    expect(tileState({ sessionKey: 'forge:build', onScreen: true, evidence: retained, connection: 'closed' }))
      .toBe('takenOver')
  })

  it('holds nothing about a user no whole response has ever spoken for', () => {
    const partial = sessionEvidenceFrom({
      sessions,
      loading: false,
      error: "tmux failed for user 'alice'",
      partialAnsweringUsers: ['forge'],
    })
    expect(tileState({
      sessionKey: 'alice:gone',
      onScreen: true,
      evidence: retainSessionEvidence(NO_SESSION_EVIDENCE, partial),
      connection: 'closed',
    })).toBe('takenOver')
  })

  it('never holds a verdict against the tile own open connection', () => {
    expect(tileState({
      sessionKey: 'gone',
      onScreen: true,
      evidence: retainSessionEvidence(heard, failed),
      connection: 'open',
    })).toBe('live')
  })
})

describe('tileStateFor', () => {
  it('is Live while the shown binding holds an open connection', () => {
    expect(tileState({ sessionKey: 'main', onScreen: true, evidence: heard, connection: 'open' }))
      .toBe('live')
  })

  it('is Live, not lost, while a shown binding is still dialling', () => {
    expect(tileState({ sessionKey: 'main', onScreen: true, evidence: heard, connection: 'connecting' }))
      .toBe('live')
    expect(tileState({ sessionKey: 'main', onScreen: true, evidence: heard, connection: 'idle' }))
      .toBe('live')
  })

  it('is Idle for a binding that is not the one on screen', () => {
    expect(tileState({ sessionKey: 'main', onScreen: false, evidence: heard, connection: 'idle' }))
      .toBe('idle')
    expect(tileState({ sessionKey: 'main', onScreen: false, evidence: heard, connection: 'open' }))
      .toBe('idle')
  })

  it('is Taken over when the host ended this terminal but tmux still lists the session', () => {
    expect(tileState({ sessionKey: 'forge:build', onScreen: true, evidence: heard, connection: 'closed' }))
      .toBe('takenOver')
  })

  // The two causes a tile used to read as one. Both leave it with no
  // connection and a live session; only the close tells them apart.
  it('is Lost, not Taken over, when the connection was lost and no other client holds the session', () => {
    expect(tileState({ sessionKey: 'forge:build', onScreen: true, evidence: heard, connection: 'dropped' }))
      .toBe('lost')
  })

  // Another client being attached used to make this Taken over, because
  // dialling again attached with -d and would have evicted them. Nothing
  // attaches with -d now, so a dial costs them nothing and the tile takes it.
  it('is Lost even when another client is attached, because dialling no longer evicts anyone', () => {
    expect(tileState({
      sessionKey: 'forge:build',
      onScreen: true,
      evidence: heard,
      connection: 'dropped',
    })).toBe('lost')
  })

  it('is Ended, not Lost, when the connection was lost and the session is gone with it', () => {
    expect(tileState({ sessionKey: 'gone', onScreen: true, evidence: heard, connection: 'dropped' }))
      .toBe('ended')
  })

  it('is Idle, not Lost, for a lost connection on a binding that is not on screen', () => {
    expect(tileState({ sessionKey: 'forge:build', onScreen: false, evidence: heard, connection: 'dropped' }))
      .toBe('idle')
  })

  it('is Ended when tmux no longer lists the session, on screen or not', () => {
    expect(tileState({ sessionKey: 'gone', onScreen: true, evidence: heard, connection: 'closed' }))
      .toBe('ended')
    expect(tileState({ sessionKey: 'gone', onScreen: false, evidence: heard, connection: 'idle' }))
      .toBe('ended')
  })

  it('never calls a binding ended before a trustworthy session list has arrived', () => {
    expect(tileState({ sessionKey: 'gone', onScreen: true, evidence: NO_SESSION_EVIDENCE, connection: 'closed' }))
      .toBe('takenOver')
    expect(tileState({ sessionKey: 'gone', onScreen: false, evidence: NO_SESSION_EVIDENCE, connection: 'idle' }))
      .toBe('idle')
  })

  it('holds only the bindings a partial outage cannot speak for', () => {
    const partial: SessionEvidence = { live, answering: new Set(['forge']) }
    expect(tileState({ sessionKey: 'forge:gone', onScreen: true, evidence: partial, connection: 'closed' }))
      .toBe('ended')
    expect(tileState({ sessionKey: 'alice:gone', onScreen: true, evidence: partial, connection: 'closed' }))
      .toBe('takenOver')
  })

  it('trusts its own open connection over a poll that has not caught up with a restart', () => {
    expect(tileState({ sessionKey: 'gone', onScreen: true, evidence: heard, connection: 'open' }))
      .toBe('live')
  })

  it('groups exactly the states that show a last frame with an action', () => {
    expect(isDetached('takenOver')).toBe(true)
    expect(isDetached('lost')).toBe(true)
    expect(isDetached('ended')).toBe(true)
    expect(isDetached('live')).toBe(false)
    expect(isDetached('idle')).toBe(false)
  })
})
