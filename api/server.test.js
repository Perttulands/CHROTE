// API Route Tests for server.js
// Tests all tmux session management endpoints with mocked execFileSync

const { execFileSync } = require('child_process');

// Mock child_process before requiring the module
jest.mock('child_process', () => ({
  execFileSync: jest.fn()
}));

// Mock utils to avoid issues
jest.mock('./utils', () => ({
  getGroupPriority: jest.fn((group) => {
    if (group === 'hq') return 0;
    if (group === 'main') return 1;
    if (group.startsWith('gt-')) return 2;
    return 3;
  }),
  categorizeSession: jest.fn((name) => {
    if (name.startsWith('hq-')) return 'hq';
    if (name === 'main' || name === 'shell') return 'main';
    if (name.startsWith('gt-')) {
      const parts = name.split('-');
      return parts.length >= 2 ? `gt-${parts[1]}` : 'other';
    }
    return 'other';
  })
}));

// We need to test the Express app, so use supertest
const request = require('supertest');

// Create a minimal test app that mirrors server.js behavior
const express = require('express');
const { getGroupPriority, categorizeSession } = require('./utils');

function createTestApp(options = {}) {
  const app = express();

  // Session name validation
  const SESSION_NAME_REGEX = /^[a-zA-Z0-9_-]+$/;

  function validateSessionName(name, paramName = 'session name') {
    if (!name || typeof name !== 'string') {
      return { valid: false, error: `${paramName} is required.` };
    }
    if (!SESSION_NAME_REGEX.test(name)) {
      return { valid: false, error: `Invalid ${paramName}. Use only letters, numbers, dashes, and underscores.` };
    }
    if (name.length > 50) {
      return { valid: false, error: `${paramName} too long (max 50 characters).` };
    }
    return { valid: true };
  }

  // CORS middleware
  const ALLOWED_ORIGINS = options.corsOrigins || null;

  app.use((req, res, next) => {
    const origin = req.get('Origin');
    if (ALLOWED_ORIGINS) {
      if (origin && ALLOWED_ORIGINS.includes(origin)) {
        res.header('Access-Control-Allow-Origin', origin);
      }
    } else {
      res.header('Access-Control-Allow-Origin', '*');
    }
    res.header('Access-Control-Allow-Headers', 'Origin, X-Requested-With, Content-Type, Accept, Authorization');
    res.header('Access-Control-Allow-Methods', 'GET, POST, PATCH, DELETE, OPTIONS');
    next();
  });

  // Auth middleware
  const API_AUTH_TOKEN = options.authToken || null;

  function authMiddleware(req, res, next) {
    if (!API_AUTH_TOKEN) return next();
    // When mounted at /api, req.path is relative (e.g., '/health' not '/api/health')
    if (req.path === '/health') return next();

    const authHeader = req.get('Authorization');
    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return res.status(401).json({ success: false, error: 'Authorization required' });
    }

    const token = authHeader.slice(7);
    if (token !== API_AUTH_TOKEN) {
      return res.status(403).json({ success: false, error: 'Invalid token' });
    }
    next();
  }

  app.use('/api', authMiddleware);
  app.use(express.json());

  // GET /api/tmux/sessions
  app.get('/api/tmux/sessions', (req, res) => {
    try {
      const output = execFileSync(
        'tmux',
        ['list-sessions', '-F', '#{session_name}:#{session_windows}:#{session_attached}'],
        { encoding: 'utf-8', timeout: 5000 }
      );

      const sessions = output.trim().split('\n')
        .filter(line => line.trim())
        .map(line => {
          const [name, windows, attached] = line.split(':');
          const group = categorizeSession(name);
          return { name, windows: parseInt(windows) || 1, attached: attached === '1', group };
        });

      sessions.sort((a, b) => {
        const priorityA = getGroupPriority(a.group);
        const priorityB = getGroupPriority(b.group);
        if (priorityA !== priorityB) return priorityA - priorityB;
        if (a.group !== b.group) return a.group.localeCompare(b.group);
        return a.name.localeCompare(b.name);
      });

      const grouped = {};
      sessions.forEach(session => {
        if (!grouped[session.group]) grouped[session.group] = [];
        grouped[session.group].push(session);
      });

      res.json({ sessions, grouped, timestamp: new Date().toISOString() });
    } catch (e) {
      const noServerErrors = ['no server running', 'No such file or directory', 'error connecting'];
      const isNoServer = noServerErrors.some(msg => e.message.includes(msg));
      res.json({
        sessions: [],
        grouped: {},
        ...(isNoServer ? {} : { error: e.message }),
        timestamp: new Date().toISOString()
      });
    }
  });

  // POST /api/tmux/sessions
  app.post('/api/tmux/sessions', (req, res) => {
    try {
      let { name } = req.body;

      if (!name) {
        const timestamp = Date.now().toString(36);
        name = `shell-${timestamp}`;
      } else {
        const validation = validateSessionName(name);
        if (!validation.valid) {
          return res.status(400).json({ success: false, error: validation.error });
        }
      }

      execFileSync('tmux', ['new-session', '-d', '-s', name, '-c', '/code'], {
        encoding: 'utf-8', timeout: 5000
      });

      res.json({ success: true, session: name, timestamp: new Date().toISOString() });
    } catch (e) {
      res.status(400).json({ success: false, error: e.message });
    }
  });

  // PATCH /api/tmux/sessions/:name
  app.patch('/api/tmux/sessions/:name', (req, res) => {
    try {
      const oldName = decodeURIComponent(req.params.name);
      const { newName } = req.body;

      const oldValidation = validateSessionName(oldName, 'current session name');
      if (!oldValidation.valid) {
        return res.status(400).json({ success: false, error: oldValidation.error });
      }

      const newValidation = validateSessionName(newName, 'new session name');
      if (!newValidation.valid) {
        return res.status(400).json({ success: false, error: newValidation.error });
      }

      execFileSync('tmux', ['rename-session', '-t', oldName, newName], {
        encoding: 'utf-8', timeout: 5000
      });

      res.json({ success: true, oldName, newName, timestamp: new Date().toISOString() });
    } catch (e) {
      res.status(500).json({ success: false, error: e.message });
    }
  });

  // DELETE /api/tmux/sessions/all (nuke) - MUST be before :name route!
  const NUKE_CONFIRM_HEADER = 'DASHBOARD-NUKE-CONFIRMED';

  app.delete('/api/tmux/sessions/all', (req, res) => {
    const confirmHeader = req.get('X-Nuke-Confirm');
    if (confirmHeader !== NUKE_CONFIRM_HEADER) {
      return res.status(403).json({
        success: false,
        error: 'Nuke operation requires dashboard confirmation. Use the UI.'
      });
    }

    try {
      let sessionNames = [];
      try {
        const output = execFileSync('tmux', ['list-sessions', '-F', '#{session_name}'], {
          encoding: 'utf-8', timeout: 5000
        });
        sessionNames = output.trim().split('\n').filter(line => line.trim());
      } catch (e) {
        return res.json({ success: true, killed: 0, message: 'No sessions to kill' });
      }

      execFileSync('tmux', ['kill-server'], { encoding: 'utf-8', timeout: 5000 });

      res.json({ success: true, killed: sessionNames.length, sessions: sessionNames });
    } catch (e) {
      res.status(500).json({ success: false, error: e.message });
    }
  });

  // DELETE /api/tmux/sessions/:name
  app.delete('/api/tmux/sessions/:name', (req, res) => {
    try {
      const sessionName = decodeURIComponent(req.params.name);

      const validation = validateSessionName(sessionName);
      if (!validation.valid) {
        return res.status(400).json({ success: false, error: validation.error });
      }

      execFileSync('tmux', ['kill-session', '-t', sessionName], {
        encoding: 'utf-8', timeout: 5000
      });

      res.json({ success: true, killed: sessionName, timestamp: new Date().toISOString() });
    } catch (e) {
      res.status(500).json({ success: false, error: e.message });
    }
  });

  // POST /api/tmux/appearance
  app.post('/api/tmux/appearance', (req, res) => {
    try {
      const { statusBg, statusFg, paneBorderActive, paneBorderInactive, modeStyleBg, modeStyleFg } = req.body;

      const validateColor = (color) => {
        if (!color) return true;
        return /^#[0-9A-Fa-f]{3,6}$/.test(color) || /^[a-zA-Z]+$/.test(color) || color === 'default';
      };

      const colors = { statusBg, statusFg, paneBorderActive, paneBorderInactive, modeStyleBg, modeStyleFg };
      for (const [key, val] of Object.entries(colors)) {
        if (val && !validateColor(val)) {
          return res.status(400).json({ success: false, error: `Invalid color for ${key}: ${val}` });
        }
      }

      const commands = [];
      if (statusBg && statusFg) {
        commands.push(['set', '-g', 'status-style', `bg=${statusBg},fg=${statusFg}`]);
      }
      if (paneBorderActive) {
        commands.push(['set', '-g', 'pane-active-border-style', `fg=${paneBorderActive}`]);
      }
      if (paneBorderInactive) {
        commands.push(['set', '-g', 'pane-border-style', `fg=${paneBorderInactive}`]);
      }
      if (modeStyleBg && modeStyleFg) {
        commands.push(['set', '-g', 'mode-style', `bg=${modeStyleBg},fg=${modeStyleFg}`]);
      }

      let applied = 0;
      for (const args of commands) {
        try {
          execFileSync('tmux', args, { encoding: 'utf-8', timeout: 5000 });
          applied++;
        } catch (e) {
          const noServerErrors = ['no server running', 'No such file'];
          if (!noServerErrors.some(msg => e.message.includes(msg))) {
            console.warn(`tmux command failed: tmux ${args.join(' ')}`, e.message);
          }
        }
      }

      res.json({ success: true, applied, total: commands.length });
    } catch (e) {
      res.status(500).json({ success: false, error: e.message });
    }
  });

  // Health check
  app.get('/api/health', (req, res) => {
    res.json({ status: 'ok', timestamp: new Date().toISOString() });
  });

  return app;
}

// ============================================================================
// TESTS
// ============================================================================

describe('GET /api/tmux/sessions', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
    jest.clearAllMocks();
  });

  test('returns sessions when tmux has sessions', async () => {
    execFileSync.mockReturnValue('hq-mayor:1:0\ngt-gastown-jack:2:1\nmain:1:0\n');

    const res = await request(app).get('/api/tmux/sessions');

    expect(res.status).toBe(200);
    expect(res.body.sessions).toHaveLength(3);
    expect(res.body.sessions[0].name).toBe('hq-mayor'); // hq first (priority 0)
    expect(res.body.sessions[1].name).toBe('main'); // main second (priority 1)
    expect(res.body.sessions[2].name).toBe('gt-gastown-jack'); // gt third (priority 2)
    expect(res.body.grouped).toHaveProperty('hq');
    expect(res.body.grouped).toHaveProperty('main');
  });

  test('returns empty array when no server running', async () => {
    execFileSync.mockImplementation(() => {
      const error = new Error('no server running on /tmp/tmux-1000/default');
      throw error;
    });

    const res = await request(app).get('/api/tmux/sessions');

    expect(res.status).toBe(200);
    expect(res.body.sessions).toEqual([]);
    expect(res.body.grouped).toEqual({});
    expect(res.body.error).toBeUndefined(); // No error exposed for expected case
  });

  test('includes error for unexpected failures', async () => {
    execFileSync.mockImplementation(() => {
      throw new Error('tmux crashed unexpectedly');
    });

    const res = await request(app).get('/api/tmux/sessions');

    expect(res.status).toBe(200);
    expect(res.body.sessions).toEqual([]);
    expect(res.body.error).toBe('tmux crashed unexpectedly');
  });

  test('parses attached status correctly', async () => {
    execFileSync.mockReturnValue('session1:1:1\nsession2:1:0\n');

    const res = await request(app).get('/api/tmux/sessions');

    expect(res.body.sessions[0].attached).toBe(true);
    expect(res.body.sessions[1].attached).toBe(false);
  });

  test('parses window count correctly', async () => {
    execFileSync.mockReturnValue('session1:5:0\n');

    const res = await request(app).get('/api/tmux/sessions');

    expect(res.body.sessions[0].windows).toBe(5);
  });
});

describe('POST /api/tmux/sessions', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
    jest.clearAllMocks();
  });

  test('creates session with auto-generated name when none provided', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .post('/api/tmux/sessions')
      .send({});

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.session).toMatch(/^shell-[a-z0-9]+$/);
    expect(execFileSync).toHaveBeenCalledWith(
      'tmux',
      ['new-session', '-d', '-s', expect.stringMatching(/^shell-/), '-c', '/code'],
      expect.any(Object)
    );
  });

  test('creates session with user-provided valid name', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .post('/api/tmux/sessions')
      .send({ name: 'my-session' });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.session).toBe('my-session');
  });

  test('rejects invalid session names', async () => {
    const res = await request(app)
      .post('/api/tmux/sessions')
      .send({ name: 'bad name with spaces' });

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
    expect(res.body.error).toContain('Invalid');
  });

  test('rejects names with shell metacharacters', async () => {
    const badNames = ['$(whoami)', 'test;rm -rf', 'test`id`', 'test|cat'];

    for (const name of badNames) {
      const res = await request(app)
        .post('/api/tmux/sessions')
        .send({ name });

      expect(res.status).toBe(400);
      expect(res.body.success).toBe(false);
    }
  });

  test('rejects names that are too long', async () => {
    const longName = 'a'.repeat(51);

    const res = await request(app)
      .post('/api/tmux/sessions')
      .send({ name: longName });

    expect(res.status).toBe(400);
    expect(res.body.error).toContain('too long');
  });

  test('returns error when tmux command fails', async () => {
    execFileSync.mockImplementation(() => {
      throw new Error('duplicate session');
    });

    const res = await request(app)
      .post('/api/tmux/sessions')
      .send({ name: 'existing-session' });

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
  });
});

describe('PATCH /api/tmux/sessions/:name', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
    jest.clearAllMocks();
  });

  test('renames session successfully', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .patch('/api/tmux/sessions/old-name')
      .send({ newName: 'new-name' });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.oldName).toBe('old-name');
    expect(res.body.newName).toBe('new-name');
    expect(execFileSync).toHaveBeenCalledWith(
      'tmux',
      ['rename-session', '-t', 'old-name', 'new-name'],
      expect.any(Object)
    );
  });

  test('validates old name (blocks injection)', async () => {
    const res = await request(app)
      .patch('/api/tmux/sessions/$(whoami)')
      .send({ newName: 'safe-name' });

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
    expect(execFileSync).not.toHaveBeenCalled();
  });

  test('validates new name', async () => {
    const res = await request(app)
      .patch('/api/tmux/sessions/valid-old')
      .send({ newName: 'bad;rm -rf /' });

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
  });

  test('returns 500 when rename fails', async () => {
    execFileSync.mockImplementation(() => {
      throw new Error("can't find session: old-name");
    });

    const res = await request(app)
      .patch('/api/tmux/sessions/old-name')
      .send({ newName: 'new-name' });

    expect(res.status).toBe(500);
    expect(res.body.success).toBe(false);
  });
});

describe('DELETE /api/tmux/sessions/:name', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
    jest.clearAllMocks();
  });

  test('deletes session successfully', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .delete('/api/tmux/sessions/my-session');

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.killed).toBe('my-session');
    expect(execFileSync).toHaveBeenCalledWith(
      'tmux',
      ['kill-session', '-t', 'my-session'],
      expect.any(Object)
    );
  });

  test('validates session name (blocks injection)', async () => {
    // Test with invalid characters that would fail validation
    // Using semicolon which is invalid per SESSION_NAME_REGEX
    const res = await request(app)
      .delete('/api/tmux/sessions/session;whoami');

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
    expect(execFileSync).not.toHaveBeenCalled();
  });

  test('validates session name (blocks shell metacharacters)', async () => {
    // Test with backtick which is a shell metacharacter
    const res = await request(app)
      .delete('/api/tmux/sessions/session`id`');

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
    expect(execFileSync).not.toHaveBeenCalled();
  });

  test('handles URL-encoded session names', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .delete('/api/tmux/sessions/my-session-123');

    expect(res.status).toBe(200);
    expect(res.body.killed).toBe('my-session-123');
  });

  test('returns 500 when delete fails', async () => {
    execFileSync.mockImplementation(() => {
      throw new Error("can't find session");
    });

    const res = await request(app)
      .delete('/api/tmux/sessions/nonexistent');

    expect(res.status).toBe(500);
    expect(res.body.success).toBe(false);
  });
});

describe('DELETE /api/tmux/sessions/all (nuke)', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
    jest.clearAllMocks();
  });

  test('requires X-Nuke-Confirm header', async () => {
    const res = await request(app)
      .delete('/api/tmux/sessions/all');

    expect(res.status).toBe(403);
    expect(res.body.success).toBe(false);
    expect(res.body.error).toContain('dashboard confirmation');
  });

  test('rejects wrong header value', async () => {
    const res = await request(app)
      .delete('/api/tmux/sessions/all')
      .set('X-Nuke-Confirm', 'wrong-value');

    expect(res.status).toBe(403);
    expect(res.body.success).toBe(false);
  });

  test('kills all sessions with correct header', async () => {
    execFileSync
      .mockReturnValueOnce('session1\nsession2\nsession3\n') // list-sessions
      .mockReturnValueOnce(''); // kill-server

    const res = await request(app)
      .delete('/api/tmux/sessions/all')
      .set('X-Nuke-Confirm', 'DASHBOARD-NUKE-CONFIRMED');

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.killed).toBe(3);
    expect(res.body.sessions).toEqual(['session1', 'session2', 'session3']);
    expect(execFileSync).toHaveBeenCalledWith('tmux', ['kill-server'], expect.any(Object));
  });

  test('handles no sessions gracefully', async () => {
    execFileSync.mockImplementation(() => {
      throw new Error('no server running');
    });

    const res = await request(app)
      .delete('/api/tmux/sessions/all')
      .set('X-Nuke-Confirm', 'DASHBOARD-NUKE-CONFIRMED');

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.killed).toBe(0);
    expect(res.body.message).toBe('No sessions to kill');
  });
});

describe('POST /api/tmux/appearance', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
    jest.clearAllMocks();
  });

  test('applies valid hex colors', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .post('/api/tmux/appearance')
      .send({
        statusBg: '#1a1a1a',
        statusFg: '#00ff00',
        paneBorderActive: '#ff0000'
      });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.applied).toBe(2); // status-style + pane-active-border-style
  });

  test('applies valid named colors', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .post('/api/tmux/appearance')
      .send({ paneBorderActive: 'green', paneBorderInactive: 'gray' });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
  });

  test('allows default for transparency', async () => {
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .post('/api/tmux/appearance')
      .send({ statusBg: 'default', statusFg: 'green' });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
  });

  test('rejects invalid color values', async () => {
    const res = await request(app)
      .post('/api/tmux/appearance')
      .send({ statusBg: 'not-a-color-123' });

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
    expect(res.body.error).toContain('Invalid color');
  });

  test('rejects potential injection attempts', async () => {
    const res = await request(app)
      .post('/api/tmux/appearance')
      .send({ statusBg: '$(whoami)' });

    expect(res.status).toBe(400);
    expect(res.body.success).toBe(false);
  });

  test('handles tmux not running gracefully', async () => {
    execFileSync.mockImplementation(() => {
      throw new Error('no server running');
    });

    const res = await request(app)
      .post('/api/tmux/appearance')
      .send({ statusBg: '#000', statusFg: '#fff' });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.applied).toBe(0); // Commands failed but that's okay
  });
});

describe('GET /api/health', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('returns ok status', async () => {
    const res = await request(app).get('/api/health');

    expect(res.status).toBe(200);
    expect(res.body.status).toBe('ok');
    expect(res.body.timestamp).toBeDefined();
  });
});

// ============================================================================
// SECURITY TESTS
// ============================================================================

describe('Security: Session name validation', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
    jest.clearAllMocks();
  });

  const injectionPayloads = [
    '$(whoami)',
    '`id`',
    'test;rm -rf /',
    'test|cat /etc/passwd',
    'test && ls',
    'test\necho pwned',
    '../../../etc/passwd',
    'test$(id)',
    'test`id`',
    '${IFS}cat${IFS}/etc/passwd',
    'test;id;',
  ];

  test.each(injectionPayloads)('rejects injection payload: %s', async (payload) => {
    const res = await request(app)
      .post('/api/tmux/sessions')
      .send({ name: payload });

    expect(res.status).toBe(400);
    expect(execFileSync).not.toHaveBeenCalled();
  });

  test('allows valid session names', async () => {
    execFileSync.mockReturnValue('');

    const validNames = [
      'session1',
      'my-session',
      'my_session',
      'Session123',
      'a',
      'A-B-C_123',
      '0123456789',
    ];

    for (const name of validNames) {
      jest.clearAllMocks();
      const res = await request(app)
        .post('/api/tmux/sessions')
        .send({ name });

      expect(res.status).toBe(200);
      expect(res.body.session).toBe(name);
    }
  });
});

describe('Security: API authentication', () => {
  test('allows all requests when no token configured', async () => {
    const app = createTestApp({ authToken: null });
    execFileSync.mockReturnValue('session1:1:0\n');

    const res = await request(app).get('/api/tmux/sessions');

    expect(res.status).toBe(200);
  });

  test('requires Bearer token when configured', async () => {
    const app = createTestApp({ authToken: 'secret-token' });

    const res = await request(app).get('/api/tmux/sessions');

    expect(res.status).toBe(401);
    expect(res.body.error).toBe('Authorization required');
  });

  test('rejects invalid token', async () => {
    const app = createTestApp({ authToken: 'secret-token' });

    const res = await request(app)
      .get('/api/tmux/sessions')
      .set('Authorization', 'Bearer wrong-token');

    expect(res.status).toBe(403);
    expect(res.body.error).toBe('Invalid token');
  });

  test('accepts valid token', async () => {
    const app = createTestApp({ authToken: 'secret-token' });
    execFileSync.mockReturnValue('session1:1:0\n');

    const res = await request(app)
      .get('/api/tmux/sessions')
      .set('Authorization', 'Bearer secret-token');

    expect(res.status).toBe(200);
  });

  test('bypasses auth for health check', async () => {
    const app = createTestApp({ authToken: 'secret-token' });

    const res = await request(app).get('/api/health');

    expect(res.status).toBe(200);
    expect(res.body.status).toBe('ok');
  });
});

describe('Security: CORS', () => {
  test('allows all origins in dev mode', async () => {
    const app = createTestApp({ corsOrigins: null });
    execFileSync.mockReturnValue('');

    const res = await request(app)
      .get('/api/tmux/sessions')
      .set('Origin', 'http://evil.com');

    expect(res.headers['access-control-allow-origin']).toBe('*');
  });

  test('restricts to allowed origins in prod mode', async () => {
    const app = createTestApp({ corsOrigins: ['http://chrote:8080', 'http://localhost:5173'] });
    execFileSync.mockReturnValue('');

    // Allowed origin
    let res = await request(app)
      .get('/api/tmux/sessions')
      .set('Origin', 'http://chrote:8080');

    expect(res.headers['access-control-allow-origin']).toBe('http://chrote:8080');

    // Disallowed origin - header not set
    res = await request(app)
      .get('/api/tmux/sessions')
      .set('Origin', 'http://evil.com');

    expect(res.headers['access-control-allow-origin']).toBeUndefined();
  });
});
