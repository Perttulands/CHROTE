const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1920, height: 1080 });

  console.log('Fetching perttuland.dev...');
  try {
    await page.goto('https://perttuland.dev', { timeout: 30000, waitUntil: 'networkidle' });

    // Wait for any animations to settle
    await page.waitForTimeout(2000);

    await page.screenshot({ path: 'perttuland-homepage.png', fullPage: true });
    console.log('Screenshot saved: perttuland-homepage.png');

    // Extract CSS custom properties / colors if available
    const styles = await page.evaluate(() => {
      const computed = getComputedStyle(document.documentElement);
      const body = getComputedStyle(document.body);
      return {
        bodyBg: body.backgroundColor,
        bodyColor: body.color,
        fontFamily: body.fontFamily,
      };
    });
    console.log('Styles:', styles);

  } catch (err) {
    console.log(`Error: ${err.message}`);
  }

  await browser.close();
})();
