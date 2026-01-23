import { test, expect } from '@playwright/test';

test.describe('Real Backend Integration', () => {
  const BACKEND_URL = 'http://localhost:8080';

  test('GET /api/tmux/sessions connection and contract', async ({ request }) => {
    // 1. Fetch from the real backend
    let response;
    try {
      response = await request.get(`${BACKEND_URL}/api/tmux/sessions`, {
        timeout: 5000 // 5s timeout for connection
      });
    } catch (e) {
      throw new Error(`Failed to connect to backend at ${BACKEND_URL}. Is the Go server running? Error: ${e.message}`);
    }

    // 2. verify status
    expect(response.ok(), `API returned error: ${response.status()}`).toBeTruthy();

    // 3. verify structure
    const data = await response.json();
    console.log('Real backend response:', JSON.stringify(data, null, 2));

    // Verify root properties matches SessionsResponse interface
    // interface SessionsResponse { sessions: TmuxSession[], grouped: Record<string, TmuxSession[]>, timestamp: string }
    expect(data).toHaveProperty('sessions');
    expect(data).toHaveProperty('grouped');
    expect(data).toHaveProperty('timestamp');
    expect(Array.isArray(data.sessions)).toBeTruthy();
    expect(typeof data.grouped).toBe('object');
    expect(typeof data.timestamp).toBe('string');

    // 4. Verify TmuxSession structure if sessions exist
    if (data.sessions.length > 0) {
      const session = data.sessions[0];
      // TmuxSession interface: name, windows, attached, group
      expect(session).toHaveProperty('name');
      expect(typeof session.name).toBe('string');
      
      expect(session).toHaveProperty('windows');
      expect(typeof session.windows).toBe('number');
      
      expect(session).toHaveProperty('attached');
      expect(typeof session.attached).toBe('boolean');
      
      expect(session).toHaveProperty('group');
      expect(typeof session.group).toBe('string');
    }
  });
});
