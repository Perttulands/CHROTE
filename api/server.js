// Arena Dashboard - Tmux Session API
// Simple Express server to list tmux sessions

const express = require('express');
const { execFileSync } = require('child_process');
const { getGroupPriority, categorizeSession } = require('./utils');
const app = express();

// Ensure consistent tmux socket location
const TMUX_ENV = { env: { ...process.env, TMUX_TMPDIR: '/tmp' } };

// Simple response cache to avoid spawning tmux process on every poll
let sessionsCache = { data: null, timestamp: 0 };
const CACHE_TTL = 1000; // 1 second cache

// Session name validation regex - alphanumeric, dash, underscore only
const SESSION_NAME_REGEX = /^[a-zA-Z0-9_-]+$/;

// Validate session name for route params (oldName wasn't validated before)
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

// Invalidate sessions cache (call after mutations)
function invalidateSessionsCache() {
  sessionsCache = { data: null, timestamp: 0 };
}

// CORS configuration - restrict to known origins in production
const ALLOWED_ORIGINS = process.env.CORS_ORIGINS
  ? process.env.CORS_ORIGINS.split(',').map(o => o.trim())
  : null; // null = allow all (dev mode)

app.use((req, res, next) => {
  const origin = req.get('Origin');

  if (ALLOWED_ORIGINS) {
    // Production mode: check against allowlist
    if (origin && ALLOWED_ORIGINS.includes(origin)) {
      res.header('Access-Control-Allow-Origin', origin);
    }
    // If no match, don't set the header (browser will block)
  } else {
    // Development mode: allow all
    res.header('Access-Control-Allow-Origin', '*');
  }

  res.header('Access-Control-Allow-Headers', 'Origin, X-Requested-With, Content-Type, Accept, Authorization');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PATCH, DELETE, OPTIONS');
  next();
});

// Optional authentication middleware
// Set API_AUTH_TOKEN env var to enable bearer token auth
const API_AUTH_TOKEN = process.env.API_AUTH_TOKEN;

function authMiddleware(req, res, next) {
  // Skip auth if no token configured (dev mode)
  if (!API_AUTH_TOKEN) {
    return next();
  }

  // Skip auth for health check
  if (req.path === '/api/health') {
    return next();
  }

  const authHeader = req.get('Authorization');
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return res.status(401).json({
      success: false,
      error: 'Authorization required',
      timestamp: new Date().toISOString()
    });
  }

  const token = authHeader.slice(7); // Remove 'Bearer ' prefix
  if (token !== API_AUTH_TOKEN) {
    return res.status(403).json({
      success: false,
      error: 'Invalid token',
      timestamp: new Date().toISOString()
    });
  }

  next();
}

// Apply auth middleware to all /api routes
app.use('/api', authMiddleware);

// Parse JSON bodies
app.use(express.json());

app.get('/api/tmux/sessions', (req, res) => {
  // Return cached response if still fresh
  const now = Date.now();
  if (sessionsCache.data && (now - sessionsCache.timestamp) < CACHE_TTL) {
    return res.json(sessionsCache.data);
  }

  try {
    // API runs as dev user, so tmux commands access dev's sessions directly
    // Use execFileSync to avoid shell injection
    const output = execFileSync(
      'tmux',
      ['list-sessions', '-F', '#{session_name}:#{session_windows}:#{session_attached}'],
      { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV }
    );

    const sessions = output.trim().split('\n')
      .filter(line => line.trim())
      .map(line => {
        const [name, windows, attached] = line.split(':');
        const group = categorizeSession(name);
        return {
          name,
          windows: parseInt(windows) || 1,
          attached: attached === '1',
          group
        };
      });

    // Sort: by group priority, then by group name, then by session name
    sessions.sort((a, b) => {
      const priorityA = getGroupPriority(a.group);
      const priorityB = getGroupPriority(b.group);
      if (priorityA !== priorityB) return priorityA - priorityB;
      if (a.group !== b.group) return a.group.localeCompare(b.group);
      return a.name.localeCompare(b.name);
    });

    // Group sessions by their group
    const grouped = {};
    sessions.forEach(session => {
      if (!grouped[session.group]) {
        grouped[session.group] = [];
      }
      grouped[session.group].push(session);
    });

    const response = {
      sessions,
      grouped,
      timestamp: new Date().toISOString()
    };

    // Cache the successful response
    sessionsCache = { data: response, timestamp: now };
    res.json(response);
  } catch (e) {
    // tmux server might not be running - various error messages possible
    const noServerErrors = ['no server running', 'No such file or directory', 'error connecting'];
    const isNoServer = noServerErrors.some(msg => e.message.includes(msg));

    const response = {
      sessions: [],
      grouped: {},
      // Don't expose error for expected "no sessions" case
      ...(isNoServer ? {} : { error: e.message }),
      timestamp: new Date().toISOString()
    };

    // Cache empty response too (prevents hammering tmux on errors)
    sessionsCache = { data: response, timestamp: now };
    res.json(response);
  }
});

// Create a new tmux session
app.post('/api/tmux/sessions', (req, res) => {
  try {
    let { name } = req.body;

    // Generate a name if not provided
    if (!name) {
      const timestamp = Date.now().toString(36);
      name = `shell-${timestamp}`;
    } else {
      // Validate user-provided session name
      const validation = validateSessionName(name);
      if (!validation.valid) {
        return res.status(400).json({
          success: false,
          error: validation.error,
          timestamp: new Date().toISOString()
        });
      }
    }

    // Create the session (detached) - use execFileSync to avoid shell injection
    execFileSync('tmux', ['new-session', '-d', '-s', name, '-c', '/code'], {
      encoding: 'utf-8',
      timeout: 5000,
      ...TMUX_ENV
    });

    // Invalidate cache so next poll sees the new session
    invalidateSessionsCache();

    res.json({
      success: true,
      session: name,
      timestamp: new Date().toISOString()
    });
  } catch (e) {
    res.status(400).json({
      success: false,
      error: e.message,
      timestamp: new Date().toISOString()
    });
  }
});

// Rename a tmux session
app.patch('/api/tmux/sessions/:name', (req, res) => {
  try {
    const oldName = decodeURIComponent(req.params.name);
    const { newName } = req.body;

    // Validate old session name (CRITICAL: was missing before - command injection vector)
    const oldValidation = validateSessionName(oldName, 'current session name');
    if (!oldValidation.valid) {
      return res.status(400).json({
        success: false,
        error: oldValidation.error,
        timestamp: new Date().toISOString()
      });
    }

    // Validate new session name
    const newValidation = validateSessionName(newName, 'new session name');
    if (!newValidation.valid) {
      return res.status(400).json({
        success: false,
        error: newValidation.error,
        timestamp: new Date().toISOString()
      });
    }

    // Rename the session - use execFileSync to avoid shell injection
    execFileSync('tmux', ['rename-session', '-t', oldName, newName], {
      encoding: 'utf-8',
      timeout: 5000,
      ...TMUX_ENV
    });

    // Invalidate cache so next poll sees the renamed session
    invalidateSessionsCache();

    res.json({
      success: true,
      oldName,
      newName,
      timestamp: new Date().toISOString()
    });
  } catch (e) {
    res.status(500).json({
      success: false,
      error: e.message,
      timestamp: new Date().toISOString()
    });
  }
});

// Delete a specific tmux session
app.delete('/api/tmux/sessions/:name', (req, res) => {
  try {
    const sessionName = decodeURIComponent(req.params.name);

    // Validate session name (CRITICAL: was missing before - command injection vector)
    const validation = validateSessionName(sessionName);
    if (!validation.valid) {
      return res.status(400).json({
        success: false,
        error: validation.error,
        timestamp: new Date().toISOString()
      });
    }

    // Kill the specific session - use execFileSync to avoid shell injection
    execFileSync('tmux', ['kill-session', '-t', sessionName], {
      encoding: 'utf-8',
      timeout: 5000,
      ...TMUX_ENV
    });

    // Invalidate cache so next poll reflects the deletion
    invalidateSessionsCache();

    res.json({
      success: true,
      killed: sessionName,
      timestamp: new Date().toISOString()
    });
  } catch (e) {
    res.status(500).json({
      success: false,
      error: e.message,
      timestamp: new Date().toISOString()
    });
  }
});

// Delete ALL tmux sessions (nuke) - PROTECTED
// Requires X-Nuke-Confirm header with value "DASHBOARD-NUKE-CONFIRMED"
// This prevents agents from programmatically nuking - only the dashboard UI knows this header
const NUKE_CONFIRM_HEADER = 'DASHBOARD-NUKE-CONFIRMED';

app.delete('/api/tmux/sessions/all', (req, res) => {
  // Verify the request came from the dashboard UI, not an agent
  const confirmHeader = req.get('X-Nuke-Confirm');
  if (confirmHeader !== NUKE_CONFIRM_HEADER) {
    return res.status(403).json({
      success: false,
      error: 'Nuke operation requires dashboard confirmation. Use the UI.',
      timestamp: new Date().toISOString()
    });
  }

  try {
    // Get list of all sessions first
    let sessionNames = [];
    try {
      const output = execFileSync(
        'tmux',
        ['list-sessions', '-F', '#{session_name}'],
        { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV }
      );
      sessionNames = output.trim().split('\n').filter(line => line.trim());
    } catch (e) {
      // No sessions running - that's fine
      return res.json({
        success: true,
        killed: 0,
        message: 'No sessions to kill',
        timestamp: new Date().toISOString()
      });
    }

    // Kill the tmux server (destroys all sessions)
    execFileSync('tmux', ['kill-server'], { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV });

    // Invalidate cache
    invalidateSessionsCache();

    res.json({
      success: true,
      killed: sessionNames.length,
      sessions: sessionNames,
      timestamp: new Date().toISOString()
    });
  } catch (e) {
    res.status(500).json({
      success: false,
      error: e.message,
      timestamp: new Date().toISOString()
    });
  }
});

// Apply tmux appearance settings (hot-reload)
app.post('/api/tmux/appearance', (req, res) => {
  try {
    const { statusBg, statusFg, paneBorderActive, paneBorderInactive, modeStyleBg, modeStyleFg } = req.body;

    // Validate color inputs (hex, named colors, or 'default' for transparency)
    const validateColor = (color) => {
      if (!color) return true; // Allow undefined/null (skip this setting)
      // Allow hex colors (#RGB, #RRGGBB), named colors, or 'default' for transparency
      return /^#[0-9A-Fa-f]{3,6}$/.test(color) || /^[a-zA-Z]+$/.test(color) || color === 'default';
    };

    const colors = { statusBg, statusFg, paneBorderActive, paneBorderInactive, modeStyleBg, modeStyleFg };
    for (const [key, val] of Object.entries(colors)) {
      if (val && !validateColor(val)) {
        return res.status(400).json({
          success: false,
          error: `Invalid color for ${key}: ${val}`,
          timestamp: new Date().toISOString()
        });
      }
    }

    // Build tmux set commands as argument arrays - use execFileSync to avoid shell injection
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

    // Execute all commands
    let applied = 0;
    for (const args of commands) {
      try {
        execFileSync('tmux', args, { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV });
        applied++;
      } catch (e) {
        // tmux server might not be running - that's okay, settings apply when it starts
        const noServerErrors = ['no server running', 'No such file'];
        if (!noServerErrors.some(msg => e.message.includes(msg))) {
          console.warn(`tmux command failed: tmux ${args.join(' ')}`, e.message);
        }
      }
    }

    res.json({
      success: true,
      applied,
      total: commands.length,
      timestamp: new Date().toISOString()
    });
  } catch (e) {
    res.status(500).json({
      success: false,
      error: e.message,
      timestamp: new Date().toISOString()
    });
  }
});

// Health check
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// FILE API - replaces filebrowser container
const fileRoutes = require('./file-routes');
app.use('/api/files', fileRoutes);

// BEADS API - beads_viewer integration
const beadsRoutes = require('./beads-routes');
app.use('/api/beads', beadsRoutes);

const PORT = process.env.API_PORT || 3001;
app.listen(PORT, '0.0.0.0', () => {
  console.log(`Arena API server listening on port ${PORT}`);
});
