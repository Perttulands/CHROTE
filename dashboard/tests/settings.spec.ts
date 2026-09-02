import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { DEFAULT_THEME } from '../src/theme/theme'

test.describe('Settings View', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    // Clear persisted dashboard state before the app boots. The session flag keeps
    // reload-based persistence checks from wiping the state they just wrote.
    await page.addInitScript(() => {
      const clearFlag = '__chrote_settings_storage_cleared'
      if (sessionStorage.getItem(clearFlag) === '1') return
      localStorage.clear()
      sessionStorage.setItem(clearFlag, '1')
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test.describe('Appearance', () => {

    test('should switch to Settings view', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')
      await expect(page.locator('.settings-view')).toBeVisible()
    })

    // The host owns the theme. Settings offers nothing that would change it,
    // and the palette on the document is the one /api/theme served.
    test('offers no theme, tmux appearance or badge colour controls', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')
      await expect(page.locator('.settings-view')).toBeVisible()

      await expect(page.locator('.theme-option')).toHaveCount(0)
      await expect(page.locator('.settings-view').getByText('tmux Appearance')).toHaveCount(0)
      await expect(page.locator('.settings-view').getByText('Session User Indicators')).toHaveCount(0)
      expect(await page.evaluate(() => document.documentElement.getAttribute('data-theme'))).toBeNull()
    })

    test('paints the document in the palette the host served', async ({ page }) => {
      const applied = await page.evaluate(() => {
        const style = getComputedStyle(document.documentElement)
        return {
          accent: style.getPropertyValue('--accent').trim(),
          background: style.getPropertyValue('--background').trim(),
          terminal: style.getPropertyValue('--terminal-background').trim(),
          ansi15: style.getPropertyValue('--ansi-15').trim(),
          identity0: style.getPropertyValue('--identity-0').trim(),
        }
      })

      expect(applied).toEqual({
        accent: DEFAULT_THEME.ui.accent,
        background: DEFAULT_THEME.ui.background,
        terminal: DEFAULT_THEME.terminal.background,
        ansi15: DEFAULT_THEME.terminal.ansi[15],
        identity0: DEFAULT_THEME.identity[0],
      })
    })

    test('serves both faces from this host, so no font request leaves it', async ({ page }) => {
      const external: string[] = []
      page.on('request', request => {
        const url = new URL(request.url())
        if (url.host !== new URL(page.url()).host) external.push(request.url())
      })

      await page.reload()
      await page.waitForSelector('.dashboard')

      expect(external).toEqual([])
      const bodyFont = await page.evaluate(() => getComputedStyle(document.body).fontFamily)
      expect(bodyFont).toContain('JetBrains Mono')
      expect(bodyFont).toContain('CHROTE Term Symbols')

      for (const file of ['JetBrainsMono-Regular.woff2', 'JetBrainsMono-Bold.woff2', 'chrote-term-symbols.woff2']) {
        const response = await page.request.get(`/fonts/${file}`)
        expect(response.status(), file).toBe(200)
      }
    })

  })

})
