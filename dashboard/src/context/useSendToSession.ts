import { useCallback, useState } from 'react'
import type { LaunchUser, SendSessionPane, SendToSessionOutcome, SendToSessionPayload, SendToSessionResult } from '../types'
import { apiErrorMessage } from '../apiErrors'
import { useToast } from './ToastContext'

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
  const { addToast } = useToast()
  const [sendToSessionTarget, setSendToSessionTarget] = useState<string | null>(null)
  const [sendToSessionPrefill, setSendToSessionPrefill] = useState('')
  const [sendToSessionRequestId, setSendToSessionRequestId] = useState(0)

  const openSendToSession = useCallback((sessionName: string, prefill = '') => {
    setSendToSessionPrefill(prefill)
    setSendToSessionRequestId(previous => previous + 1)
    setSendToSessionTarget(sessionName)
  }, [])

  const closeSendToSession = useCallback(() => {
    setSendToSessionTarget(null)
    setSendToSessionPrefill('')
  }, [])

  const listSessionPanes = useCallback(async (sessionName: string, unixUser?: LaunchUser): Promise<SendSessionPane[] | null> => {
    const expectedUnixUser = unixUser ?? ''
    try {
      const query = unixUser ? `?unixUser=${encodeURIComponent(unixUser)}` : ''
      const response = await fetch(`/api/tmux/sessions/${encodeURIComponent(sessionName)}/panes${query}`, {
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        addToast(apiErrorMessage(await response.text(), 'Failed to resolve session panes'), 'error')
        return null
      }
      const result = await response.json().catch(() => null) as { success?: unknown; session?: unknown; unixUser?: unknown; panes?: unknown } | null
      if (!result || result.success !== true || result.session !== sessionName ||
          result.unixUser !== expectedUnixUser || !Array.isArray(result.panes)) {
        addToast('Unexpected pane discovery response', 'error')
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
        addToast('Unexpected pane discovery response', 'error')
        return null
      }
      return panes
    } catch (e) {
      console.error('Failed to resolve session panes:', e)
      addToast('Failed to resolve session panes', 'error')
      return null
    }
  }, [addToast])

  const sendToSession = useCallback(async (
    sessionName: string,
    payload: SendToSessionPayload,
    unixUser?: LaunchUser,
  ): Promise<SendToSessionOutcome> => {
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
        if (definitiveMessage) {
          addToast(definitiveMessage, 'error')
          return 'failed'
        }
        addToast(`Delivery outcome is unknown for '${sessionName}'; inspect the exact pane before retrying`, 'error')
        return 'unknown'
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
        addToast(`Delivery outcome is unknown for '${sessionName}' (${result.pane}); ${result.warning.trim()}`, 'error')
        return 'unknown'
      }
      if (commonResultValid && result && result.success === true && result.transport === 'pasted' &&
          payload.submit && result.submitKeyDispatched === false) {
        const warning = result.warning.trim()
        addToast(`Pasted to '${sessionName}' (${result.pane}), but the submit key was not dispatched${warning ? `; ${warning}` : ''}`, 'error')
        return 'unknown'
      }
      if (!commonResultValid || !result || result.success !== true || result.transport !== 'pasted' ||
          result.submitKeyDispatched !== payload.submit || result.bufferCleaned !== true ||
          (result.targetVerified !== true && result.warning.trim() === '')) {
        addToast('Unexpected send response; inspect the target pane before retrying', 'error')
        return 'unknown'
      }
      const paneLabel = ` (${result.pane})`
      const submitLabel = result.submitKeyDispatched ? '; submit key dispatched (application acceptance unconfirmed)' : ''
      const warning = result.warning?.trim() ?? ''
      addToast(`Pasted to '${sessionName}'${paneLabel}${submitLabel}${warning ? `; ${warning}` : ''}`, warning ? 'info' : 'success')
      return 'sent'
    } catch (e) {
      console.error('Send-to-session delivery outcome is unknown:', e)
      addToast(`Delivery outcome is unknown for '${sessionName}'; inspect the exact pane before retrying`, 'error')
      return 'unknown'
    }
  }, [addToast])

  return {
    sendToSessionTarget,
    setSendToSessionTarget,
    sendToSessionPrefill,
    sendToSessionRequestId,
    openSendToSession,
    closeSendToSession,
    listSessionPanes,
    sendToSession,
  }
}
