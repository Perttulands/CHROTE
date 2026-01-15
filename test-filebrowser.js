const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1920, height: 1080 });

  try {
    // Test home page
    console.log('Testing home page...');
    await page.goto('http://arena:8080', { timeout: 15000, waitUntil: 'networkidle' });
    await page.waitForTimeout(1500);
    await page.screenshot({ path: 'filebrowser-home.png' });
    console.log('Screenshot: filebrowser-home.png');

    // Test Incoming folder
    console.log('Testing Incoming folder...');
    await page.goto('http://arena:8080/files/code/Incoming/', { timeout: 15000, waitUntil: 'networkidle' });
    await page.waitForTimeout(1500);
    await page.screenshot({ path: 'filebrowser-incoming.png' });
    console.log('Screenshot: filebrowser-incoming.png');

    // Test hover effect on home
    console.log('Testing hover effects...');
    await page.goto('http://arena:8080', { timeout: 15000, waitUntil: 'networkidle' });
    await page.waitForTimeout(1000);

    // Hover over the code folder
    const codeFolder = page.locator('text=code').first();
    await codeFolder.hover();
    await page.waitForTimeout(500);
    await page.screenshot({ path: 'filebrowser-hover.png' });
    console.log('Screenshot: filebrowser-hover.png');

  } catch (err) {
    console.log(`Error: ${err.message}`);
  }

  await browser.close();
})();
