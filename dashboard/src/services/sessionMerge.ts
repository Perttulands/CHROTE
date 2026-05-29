import type { SessionsResponse, TmuxSession } from '../types'
import type { GasCityObserver, GasCitySession } from './gascityClient'

const GASCITY_GROUP = 'gascity'

function clean(value: string | undefined): string | undefined {
  const trimmed = value?.trim()
  return trimmed || undefined
}

function gasCityDisplayName(session: GasCitySession): string {
  return clean(session.alias) ?? clean(session.name) ?? clean(session.title) ?? session.id
}

function gasCityAttachTarget(session: GasCitySession): string {
  const existing = clean(session.attachTarget)
  if (existing?.startsWith('gc:')) return existing
  return `gc:${session.id}`
}

function normalizeGasCitySession(session: GasCitySession): TmuxSession | null {
  const id = clean(session.id)
  if (!id) return null

  const attachTarget = gasCityAttachTarget({ ...session, id })
  const state = clean(session.status) ?? clean(session.state)

  return {
    name: attachTarget,
    windows: 1,
    attached: session.attached,
    group: GASCITY_GROUP,
    source: 'gascity',
    attachTarget,
    displayName: gasCityDisplayName({ ...session, id }),
    gasCityId: id,
    gasCityCity: session.city,
    title: clean(session.title),
    alias: clean(session.alias),
    template: clean(session.template),
    status: state,
    running: session.running,
  }
}

export function mergeTmuxAndGasCitySessions(
  tmuxResponse: SessionsResponse,
  gasCityObserver?: GasCityObserver | null,
): SessionsResponse {
  const gasCitySessions = (gasCityObserver?.sessions ?? [])
    .map(normalizeGasCitySession)
    .filter((session): session is TmuxSession => session !== null)

  if (gasCitySessions.length === 0) {
    return tmuxResponse
  }

  return {
    ...tmuxResponse,
    sessions: [...tmuxResponse.sessions, ...gasCitySessions],
    grouped: {
      ...tmuxResponse.grouped,
      [GASCITY_GROUP]: gasCitySessions,
    },
  }
}
