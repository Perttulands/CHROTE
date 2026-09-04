/**
 * The browser's own notification for an agent event, for when this tab is
 * hidden. Permission is the browser's to give: the setting asks for it when
 * it is turned on and reads back what the browser answered, as a word.
 */

import { agentEventTitle, type AgentEventNotice } from './agentEvents'

export type NotificationPermissionWord = 'granted' | 'denied' | 'not asked' | 'unsupported'

export function notificationPermissionWord(): NotificationPermissionWord {
  if (typeof Notification === 'undefined') return 'unsupported'
  switch (Notification.permission) {
    case 'granted': return 'granted'
    case 'denied': return 'denied'
    default: return 'not asked'
  }
}

/** Ask the browser, then report what it holds now, whatever it answered. */
export async function askNotificationPermission(): Promise<NotificationPermissionWord> {
  if (typeof Notification === 'undefined') return 'unsupported'
  try {
    await Notification.requestPermission()
  } catch {
    // The word below reports what the browser holds.
  }
  return notificationPermissionWord()
}

export function showAgentNotification(notice: AgentEventNotice): Notification | null {
  if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return null
  try {
    const notification = new Notification(agentEventTitle(notice), {
      body: notice.summary,
      // One per session: a newer event replaces the last rather than stacking.
      tag: `chrote-agent-event-${notice.sessionKey}`,
    })
    notification.onclick = () => {
      window.focus()
      notification.close()
    }
    return notification
  } catch (error) {
    console.warn('Failed to show the agent event notification:', error)
    return null
  }
}
