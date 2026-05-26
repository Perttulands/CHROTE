const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  
  // Capture all console messages
  const consoleMessages = [];
  page.on('console', msg => {
    const text = msg.text();
    if (text.includes('[ADD]') || text.includes('[CYCLE]') || text.includes('[KB]')) {
      consoleMessages.push(text);
      console.log('CONSOLE:', text);
    }
  });

  // Mock session data matching the actual test setup
  const mockSessions = {
    sessions: [
      { name: 'hq-mayor', windows: 1, attached: false, group: 'hq' },
      { name: 'hq-deacon', windows: 1, attached: true, group: 'hq' },
      { name: 'main', windows: 2, attached: false, group: 'main' },
      { name: 'gt-gastown-jack', windows: 1, attached: false, group: 'gt-gastown' },
      { name: 'gt-gastown-joe', windows: 1, attached: false, group: 'gt-gastown' },
      { name: 'gt-gastown-max', windows: 1, attached: false, group: 'gt-gastown' },
      { name: 'gt-beads-lizzy', windows: 1, attached: false, group: 'gt-beads' },
      { name: 'gt-beads-darcy', windows: 1, attached: false, group: 'gt-beads' },
    ],
    grouped: {
      'hq': [
        { name: 'hq-mayor', windows: 1, attached: false, group: 'hq' },
        { name: 'hq-deacon', windows: 1, attached: true, group: 'hq' },
      ],
      'main': [
        { name: 'main', windows: 2, attached: false, group: 'main' },
      ],
      'gt-gastown': [
        { name: 'gt-gastown-jack', windows: 1, attached: false, group: 'gt-gastown' },
        { name: 'gt-gastown-joe', windows: 1, attached: false, group: 'gt-gastown' },
        { name: 'gt-gastown-max', windows: 1, attached: false, group: 'gt-gastown' },
      ],
      'gt-beads': [
        { name: 'gt-beads-lizzy', windows: 1, attached: false, group: 'gt-beads' },
        { name: 'gt-beads-darcy', windows: 1, attached: false, group: 'gt-beads' },
      ],
    },
    timestamp: new Date().toISOString(),
  };

  // Mock API routes
  await page.route('**/api/tmux/sessions', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockSessions),
    });
  });

  await page.goto('http://localhost:5173/');
  await page.waitForSelector('.dashboard');
  await page.waitForSelector('.session-item');

  const targetWindow = page.locator('.terminal-window').first();

  // Helper for drag and drop
  async function dragAndDrop(sourceSelector, targetSelector) {
    const source = page.locator(sourceSelector).first();
    const target = page.locator(targetSelector).first();
    const sourceBox = await source.boundingBox();
    const targetBox = await target.boundingBox();
    
    if (!sourceBox || !targetBox) {
      throw new Error('Could not find source or target element for: ' + sourceSelector + ' -> ' + targetSelector);
    }
    
    const startX = sourceBox.x + sourceBox.width / 2;
    const startY = sourceBox.y + sourceBox.height / 2;
    const endX = targetBox.x + targetBox.width / 2;
    const endY = targetBox.y + targetBox.height / 2;
    
    await page.mouse.move(startX, startY);
    await page.mouse.down();
    await page.mouse.move(startX + 10, startY + 10, { steps: 5 });
    await page.mouse.move(endX, endY, { steps: 10 });
    await page.waitForTimeout(100);
    await page.mouse.up();
    await page.waitForTimeout(100);
  }

  // Add multiple sessions to first window (using actual session names from mock)
  console.log('Adding gt-gastown-jack...');
  await dragAndDrop('.session-item:has-text("jack")', '.terminal-window');
  console.log('Adding gt-gastown-joe...');
  await dragAndDrop('.session-item:has-text("joe")', '.terminal-window');
  console.log('Adding gt-gastown-max...');
  await dragAndDrop('.session-item:has-text("max")', '.terminal-window');

  // Verify 3 sessions in window
  const sessionCount = await targetWindow.locator('.session-tag').count();
  console.log('Session count:', sessionCount);

  // Check first session is active
  const firstTagClass = await targetWindow.locator('.session-tag').first().getAttribute('class');
  console.log('First session class:', firstTagClass);

  // Click on the window header to focus it
  console.log('Clicking window header...');
  await targetWindow.locator('.terminal-window-header').click();
  await page.waitForTimeout(200);

  // Check window focus
  const windowClass = await targetWindow.getAttribute('class');
  console.log('Window class after click:', windowClass);

  // Press Ctrl+Right to cycle to next session
  console.log('Pressing Ctrl+ArrowRight...');
  await page.keyboard.press('Control+ArrowRight');
  await page.waitForTimeout(500);

  // Check second session
  const secondTagClass = await targetWindow.locator('.session-tag').nth(1).getAttribute('class');
  console.log('Second session class after Ctrl+Right:', secondTagClass);

  // Press again
  console.log('Pressing Ctrl+ArrowRight again...');
  await page.keyboard.press('Control+ArrowRight');
  await page.waitForTimeout(500);

  const thirdTagClass = await targetWindow.locator('.session-tag').nth(2).getAttribute('class');
  console.log('Third session class after 2nd Ctrl+Right:', thirdTagClass);

  console.log('');
  console.log('=== All captured console messages ===');
  consoleMessages.forEach(msg => console.log(msg));

  // Check test result
  const passed = secondTagClass && secondTagClass.includes('active');
  console.log('');
  console.log('Test', passed ? 'PASSED' : 'FAILED');
  if (!passed) {
    console.log('Expected second tag to have active class, got:', secondTagClass);
  }

  await browser.close();
})();
