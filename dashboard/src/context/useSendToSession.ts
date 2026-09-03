import { useCallback, useState } from 'react'
import type { LaunchUser, SendSessionPane, SendToSessionOutcome, SendToSessionPayload, SendToSessionReport, SendToSessionRequest, SendToSessionResult } from '../types'
import { apiErrorMessage } from '../apiErrors'
import { useStatus, type StatusSeverity } from './StatusContext'

const definitiveSendErrorCodes = new Map<number, ReadonlySet<string>>([
  [400, new Set(['BAD_REQUEST'])],
  [404, new Set(['SESSION_NOT_FOUND'])],
  [408, new Set(['REQUEST_CANCELLED'])],
  [409, new Set(['PANE_REQUIRED', 'PANE_NOT_IN_SESSION', 'TARGET_CHANGED'])],
])

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function definitiveSendErrorMessage(status: number, raw: string): string | null {
  const allowedCodes = definitiveSendErrorCodes.get(status)
  if (!allowedCodes) return null
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!isRecord(parsed) || parsed.success !== false || !isRecord(parsed.error) ||
        typeof parsed.timestamp !== 'string' || Number.isNaN(Date.parse(parsed.timestamp))) return null
    const { code, message } = parsed.error
    return typeof code === 'string' && allowedCodes.has(code) && typeof message === 'string' && message.trim()
      ? message.trim()
      : null
  } catch {
    return null
  }
}

export function useSendToSession() {
  const { announce } = useStatus()
  const [sendToSessionRequest, setSendToSessionRequest] = useState<SendToSessionRequest | null>(null)
  const [sendToSessionRequestId, setSendToSessionRequestId] = useState(0)

  // Every surface opens the one drawer, and each says what it was looking at.
  // A surface with nothing to name opens it bare: the drawer then targets the
  // focused tile, which is what "Send" means with no object in hand.
  const openSendToSession = useCallback((request: SendToSessionRequest = {}) => {
    setSendToSessionRequestId(previous => previous + 1)
    setSendToSessionRequest(request)
  }, [])

  const closeSendToSession = useCallback(() => {
    setSendToSessionRequest(null)
  }, [])

  const listSessionPanes = useCallback(async (sessionName: string, unixUser?: LaunchUser): Promise<SendSessionPane[] | null> => {
    const expectedUnixUser = unixUser ?? ''
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/panes${query}`, {
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        announce(apiErrorMessage(await response.text(), 'Failed to resolve session panes'), 'error')
        return null
      }
      const result = await response.json().catch(() => null) as { success?: unknown; session?: unknown; unixUser?: unknown; panes?: unknown } | null
      if (!result || result.success !== true || result.session !== sessionName ||
          result.unixUser !== expectedUnixUser || !Array.isArray(result.panes)) {
        announce('Unexpected pane discovery response', 'error')
        return null
      }
      const panes = result.panes.filter((pane): pane is SendSessionPane => {
        if (!pane || typeof pane !== 'object') return false
        const candidate = pane as Partial<SendSessionPane>
        return typeof candidate.sessionId === 'string' && /^\$\d+$/.test(candidate.sessionId) &&
          typeof candidate.pane === 'string' && /^%\d+$/.test(candidate.pane) &&
          typeof candidate.panePid === 'string' && /^[1-9]\d*$/.test(candidate.panePid) &&
          typeof candidate.serverPid === 'string' && /^[1-9]\d*$/.test(candidate.serverPid) &&
          typeof candidate.active === 'boolean' &&
          (candidate.windowId === undefined || (typeof candidate.windowId === 'string' && /^@\d+$/.test(candidate.windowId))) &&
          (candidate.windowName === undefined || typeof candidate.windowName === 'string') &&
          (candidate.currentPath === undefined || typeof candidate.currentPath === 'string') &&
          (candidate.currentCommand === undefined || typeof candidate.currentCommand === 'string')
      })
      if (panes.length !== result.panes.length || panes.length === 0 ||
          new Set(panes.map(pane => pane.pane)).size !== panes.length ||
          new Set(panes.map(pane => pane.sessionId)).size !== 1 ||
          new Set(panes.map(pane => pane.serverPid)).size !== 1) {
        announce('Unexpected pane discovery response', 'error')
        return null
      }
      return panes
    } catch (e) {
      console.error('Failed to resolve session panes:', e)
      announce('Failed to resolve session panes', 'error')
      return null
    }
  }, [announce])

  // One place decides what happened, says it on the status line, and hands the
  // same words back to the caller. The drawer that stayed open prints them
  // beside the note that failed; nothing has to reconstruct the reason.
  const sendToSession = useCallback(async (
    sessionName: string,
    payload: SendToSessionPayload,
    unixUser?: LaunchUser,
  ): Promise<SendToSessionReport> => {
    const report = (
      message: string,
      severity: StatusSeverity,
      outcome: SendToSessionOutcome,
    ): SendToSessionReport => {
      announce(message, severity)
      return { outcome, message }
    }
    const expectedUnixUser = unixUser ?? ''
    try {
      const form = new FormData()
      form.set('text', payload.text)
      form.set('submit', payload.submit ? 'true' : 'false')
      if (payload.pane) {
        form.set('pane', payload.pane)
        if (payload.sessionId) form.set('sessionId', payload.sessionId)
        if (payload.panePid) form.set('panePid', payload.panePid)
        if (payload.serverPid) form.set('serverPid', payload.serverPid)
      }
      payload.files.forEach(file => form.append('files', file, file.name))
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/send${query}`, {
        method: 'POST',
        body: form,
        signal: AbortSignal.timeout(30000),
      })
      if (!response.ok) {
        const errorText = await response.text()
        const definitiveMessage = definitiveSendErrorMessage(response.status, errorText)
        console.error('Failed to send to session:', errorText)
        if (definitiveMessage) return report(definitiveMessage, 'error', 'failed')
        return report(`Delivery outcome is unknown for '${sessionName}'; inspect the exact pane before retrying`, 'error', 'unknown')
      }
      const result = await response.json().catch(() => null) as SendToSessionResult | null
      const commonResultValid = !!result &&
        result.session === sessionName &&
        typeof result.sessionId === 'string' && /^\$\d+$/.test(result.sessionId) &&
        typeof result.pane === 'string' && /^%\d+$/.test(result.pane) &&
        typeof result.panePid === 'string' && /^[1-9]\d*$/.test(result.panePid) &&
        typeof result.serverPid === 'string' && /^[1-9]\d*$/.test(result.serverPid) &&
        result.unixUser === expectedUnixUser &&
        (!payload.pane || result.pane === payload.pane) &&
        (!payload.sessionId || result.sessionId === payload.sessionId) &&
        (!payload.panePid || result.panePid === payload.panePid) &&
        (!payload.serverPid || result.serverPid === payload.serverPid) &&
        typeof result.submissionRequested === 'boolean' && result.submissionRequested === payload.submit &&
        typeof result.submitKeyDispatched === 'boolean' &&
        typeof result.bufferCleaned === 'boolean' &&
        typeof result.targetVerified === 'boolean' &&
        typeof result.warning === 'string'
      if (commonResultValid && result && result.success === false && result.transport === 'unknown' &&
          result.retryable === false && result.deliveryConfirmed === false &&
          result.submitKeyDispatched === false && result.targetVerified === false && result.warning.trim() !== '') {
        return report(`Delivery outcome is unknown for '${sessionName}' (${result.pane}); ${result.warning.trim()}`, 'error', 'unknown')
      }
      if (commonResultValid && result && result.success === true && result.transport === 'pasted' &&
          payload.submit && result.submitKeyDispatched === false) {
        const warning = result.warning.trim()
        return report(`Pasted to '${sessionName}' (${result.pane}), but the submit key was not dispatched${warning ? `; ${warning}` : ''}`, 'error', 'unknown')
      }
      if (!commonResultValid || !result || result.success !== true || result.transport !== 'pasted' ||
          result.submitKeyDispatched !== payload.submit || result.bufferCleaned !== true ||
          (result.targetVerified !== true && result.warning.trim() === '')) {
        return report('Unexpected send response; inspect the target pane before retrying', 'error', 'unknown')
      }
      const paneLabel = ` (${result.pane})`
      const submitLabel = result.submitKeyDispatched ? '; submit key dispatched (application acceptance unconfirmed)' : ''
      const warning = result.warning?.trim() ?? ''
      return report(`Pasted to '${sessionName}'${paneLabel}${submitLabel}${warning ? `; ${warning}` : ''}`, warning ? 'info' : 'success', 'sent')
    } catch (e) {
      console.error('Send-to-session delivery outcome is unknown:', e)
      return report(`Delivery outcome is unknown for '${sessionName}'; inspect the exact pane before retrying`, 'error', 'unknown')
    }
  }, [announce])

  return {
    sendToSessionRequest,
    sendToSessionRequestId,
    openSendToSession,
    closeSendToSession,
    listSessionPanes,
    sendToSession,
  }
}
