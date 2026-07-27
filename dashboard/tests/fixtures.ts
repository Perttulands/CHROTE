import { test as base, expect } from '@playwright/test'

type ConsoleMatcher = string | RegExp

function matchesAllowedMessage(text: string, allowedMessages: ConsoleMatcher[]) {
  return allowedMessages.some((allowed) => (
    typeof allowed === 'string' ? text.includes(allowed) : allowed.test(text)
  ))
}

function systemStatusMockBody() {
  return JSON.stringify({
    success: true,
    timestamp: new Date().toISOString(),
    data: {
      timestamp: new Date().toISOString(),
      host: {
        hostname: 'test-host',
        uptimeSeconds: 3600,
        load1: 1,
        load5: 0.8,
        load15: 0.7,
      },
      cpu: {
        cores: 4,
        totalTicks: 1000,
        idleTicks: 750,
      },
      memory: {
        totalBytes: 16 * 1024 * 1024 * 1024,
        freeBytes: 2 * 1024 * 1024 * 1024,
        availableBytes: 8 * 1024 * 1024 * 1024,
        usedBytes: 8 * 1024 * 1024 * 1024,
        usedPercent: 50,
        swapTotalBytes: 2 * 1024 * 1024 * 1024,
        swapUsedBytes: 0,
        swapUsedPercent: 0,
      },
      disks: [
        {
          mount: '/',
          totalBytes: 100 * 1024 * 1024 * 1024,
          availableBytes: 60 * 1024 * 1024 * 1024,
          usedBytes: 40 * 1024 * 1024 * 1024,
          usedPercent: 40,
        },
      ],
      network: [
        { name: 'eth0', rxBytes: 1000000, txBytes: 500000 },
      ],
      gpus: [
        { available: false, message: 'nvidia-smi unavailable' },
      ],
      warnings: [],
    },
  })
}

function systemHistoryMockBody() {
  const now = Date.now()
  const samples = [0, 1].map((index) => {
    const sample = JSON.parse(systemStatusMockBody()).data
    sample.timestamp = new Date(now - (1 - index) * 60_000).toISOString()
    sample.cpu.totalTicks += index * 100
    sample.cpu.idleTicks += index * 55
    sample.memory.usedPercent += index
    return sample
  })

  return JSON.stringify({
    success: true,
    timestamp: new Date().toISOString(),
    data: {
      limit: 288,
      samples,
    },
  })
}

export const test = base.extend<{ allowedConsoleMessages: ConsoleMatcher[] }>({
  allowedConsoleMessages: [[], { option: true }],
  page: async ({ page, allowedConsoleMessages }, use, testInfo) => {
    const consoleMessages: string[] = []
    const unexpectedBackendRequests: string[] = []

    await page.route('**/api/**', async (route) => {
      const request = route.request()
      const pathname = new URL(request.url()).pathname
      if (pathname === '/api/system/status') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: systemStatusMockBody(),
        })
        return
      }
      if (pathname === '/api/system/history') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: systemHistoryMockBody(),
        })
        return
      }
      if (pathname === '/api/tmux/mouse') {
        const body = request.postDataJSON() as { enabled?: boolean } | null
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            success: true,
            mouse: body?.enabled ? 'on' : 'off',
            applied: 1,
            total: 1,
          }),
        })
        return
      }
      unexpectedBackendRequests.push(`${request.method()} ${request.url()}`)
      await route.fulfill({
        status: 599,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Unexpected unmocked Playwright API request' }),
      })
    })

    // Match only the terminal proxy path (/terminal, /terminal/, /terminal?arg=…),
    // never source modules like /src/utils/terminalIframe.ts — serving HTML for a
    // module URL breaks the whole app with a MIME error.
    await page.route(/\/terminal(\/|\?|$)/, async (route) => {
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
