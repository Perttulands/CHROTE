// Arena Dashboard - Tmux Session API
// Simple Express server to list tmux sessions

const express = require('express');
const { execSync } = require('child_process');
const { getGroupPriority, categorizeSession, extractAgentName } = require('./utils');
const app = express();

// Ensure consistent tmux socket location
const TMUX_ENV = { env: { ...process.env, TMUX_TMPDIR: '/tmp' } };

// Simple response cache to avoid spawning tmux process on every poll
let sessionsCache = { data: null, timestamp: 0 };
const CACHE_TTL = 1000; // 1 second cache

// CORS for development
app.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Headers', 'Origin, X-Requested-With, Content-Type, Accept');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PATCH, DELETE, OPTIONS');
  next();
});

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
    const output = execSync(
      'tmux list-sessions -F "#{session_name}:#{session_windows}:#{session_attached}"',
      { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV }
    );

    const sessions = output.trim().split('\n')
      .filter(line => line.trim())
      .map(line => {
        const [name, windows, attached] = line.split(':');
        const group = categorizeSession(name);
        const agentName = extractAgentName(name);
        return {
          name,
          agentName,
          windows: parseInt(windows) || 1,
          attached: attached === '1',
          group
        };
      });

    // Sort: by group priority, then by group name, then by agent name
    sessions.sort((a, b) => {
      const priorityA = getGroupPriority(a.group);
      const priorityB = getGroupPriority(b.group);
      if (priorityA !== priorityB) return priorityA - priorityB;
      if (a.group !== b.group) return a.group.localeCompare(b.group);
      return a.agentName.localeCompare(b.agentName);
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
      // Validate user-provided session name (alphanumeric, dash, underscore only)
      if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
        return res.status(400).json({
          success: false,
          error: 'Invalid session name. Use only letters, numbers, dashes, and underscores.',
          timestamp: new Date().toISOString()
        });
      }
      if (name.length > 50) {
        return res.status(400).json({
          success: false,
          error: 'Session name too long (max 50 characters).',
          timestamp: new Date().toISOString()
        });
      }
    }

    // Create the session (detached) - API runs as dev, same user as ttyd
    execSync(`tmux new-session -d -s "${name}" -c /code`, { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV });

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

    // Validate new session name
    if (!newName) {
      return res.status(400).json({
        success: false,
        error: 'New name is required.',
        timestamp: new Date().toISOString()
      });
    }
    if (!/^[a-zA-Z0-9_-]+$/.test(newName)) {
      return res.status(400).json({
        success: false,
        error: 'Invalid session name. Use only letters, numbers, dashes, and underscores.',
        timestamp: new Date().toISOString()
      });
    }
    if (newName.length > 50) {
      return res.status(400).json({
        success: false,
        error: 'Session name too long (max 50 characters).',
        timestamp: new Date().toISOString()
      });
    }

    // Rename the session
    execSync(`tmux rename-session -t "${oldName}" "${newName}"`, {
      encoding: 'utf-8',
      timeout: 5000,
      ...TMUX_ENV
    });

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

    // Kill the specific session
    execSync(`tmux kill-session -t "${sessionName}"`, {
      encoding: 'utf-8',
      timeout: 5000,
      ...TMUX_ENV
    });

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

// Delete ALL tmux sessions (nuke)
app.delete('/api/tmux/sessions/all', (req, res) => {
  try {
    // Get list of all sessions first
    let sessionNames = [];
    try {
      const output = execSync(
        'tmux list-sessions -F "#{session_name}"',
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
    execSync('tmux kill-server', { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV });

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

    // Build tmux set commands - these apply globally without disrupting sessions
    const commands = [];

    if (statusBg && statusFg) {
      commands.push(`tmux set -g status-style "bg=${statusBg},fg=${statusFg}"`);
    }
    if (paneBorderActive) {
      commands.push(`tmux set -g pane-active-border-style "fg=${paneBorderActive}"`);
    }
    if (paneBorderInactive) {
      commands.push(`tmux set -g pane-border-style "fg=${paneBorderInactive}"`);
    }
    if (modeStyleBg && modeStyleFg) {
      commands.push(`tmux set -g mode-style "bg=${modeStyleBg},fg=${modeStyleFg}"`);
    }

    // Execute all commands
    let applied = 0;
    for (const cmd of commands) {
      try {
        execSync(cmd, { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV });
        applied++;
      } catch (e) {
        // tmux server might not be running - that's okay, settings apply when it starts
        const noServerErrors = ['no server running', 'No such file'];
        if (!noServerErrors.some(msg => e.message.includes(msg))) {
          console.warn(`tmux command failed: ${cmd}`, e.message);
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

// BEADS MODULE INTEGRATION - START
const beadsRoutes = require('./beads-routes');
app.use('/api/beads', beadsRoutes);
// BEADS MODULE INTEGRATION - END

const PORT = process.env.API_PORT || 3001;
app.listen(PORT, '0.0.0.0', () => {
  console.log(`Arena API server listening on port ${PORT}`);
});
