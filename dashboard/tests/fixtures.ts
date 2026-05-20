import { test as base, expect } from '@playwright/test'

type ConsoleMatcher = string | RegExp

function matchesAllowedMessage(text: string, allowedMessages: ConsoleMatcher[]) {
  return allowedMessages.some((allowed) => (
    typeof allowed === 'string' ? text.includes(allowed) : allowed.test(text)
  ))
}

export const test = base.extend<{ allowedConsoleMessages: ConsoleMatcher[] }>({
  allowedConsoleMessages: [[], { option: true }],
  page: async ({ page, allowedConsoleMessages }, use, testInfo) => {
    const consoleMessages: string[] = []
    const unexpectedBackendRequests: string[] = []

    await page.route('**/api/**', async (route) => {
      const request = route.request()
      unexpectedBackendRequests.push(`${request.method()} ${request.url()}`)
      await route.fulfill({
        status: 599,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Unexpected unmocked Playwright API request' }),
      })
    })

    await page.route(/.*\/terminal\/?.*/, async (route) => {
      const request = route.request()
      unexpectedBackendRequests.push(`${request.method()} ${request.url()}`)
      await route.fulfill({
        status: 599,
        contentType: 'text/html',
        body: '<html><body>Unexpected unmocked Playwright terminal request</body></html>',
      })
    })

    page.on('console', (message) => {
      if (message.type() !== 'error' && message.type() !== 'warning') return

      const text = message.text()
      consoleMessages.push(`${message.type()}: ${text}`)
    })

    await use(page)
    await page.waitForTimeout(100).catch(() => undefined)

    const annotatedAllowedMessages = testInfo.annotations
      .filter((annotation) => annotation.type === 'allowed-browser-console')
      .map((annotation) => annotation.description)
      .filter((description): description is string => Boolean(description))
    const allAllowedMessages = [...allowedConsoleMessages, ...annotatedAllowedMessages]
    const unexpectedMessages = consoleMessages.filter((message) => !matchesAllowedMessage(message, allAllowedMessages))

    if (unexpectedBackendRequests.length > 0) {
      throw new Error(`Unexpected unmocked backend requests:\n${unexpectedBackendRequests.join('\n')}`)
    }

    if (unexpectedMessages.length > 0) {
      throw new Error(`Unexpected browser console output:\n${unexpectedMessages.join('\n')}`)
    }
  },
})

export function allowBrowserConsoleMessage(message: string) {
  test.info().annotations.push({ type: 'allowed-browser-console', description: message })
}

export { expect }
export type { Frame, Locator, Page, Route } from '@playwright/test'
