// Arena Dashboard - Tmux Session API
// Simple Express server to list tmux sessions

const express = require('express');
const { execSync } = require('child_process');
const app = express();

// Ensure consistent tmux socket location
const TMUX_ENV = { env: { ...process.env, TMUX_TMPDIR: '/tmp' } };

// CORS for development
app.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Headers', 'Origin, X-Requested-With, Content-Type, Accept');
  res.header('Access-Control-Allow-Methods', 'GET, POST, DELETE, OPTIONS');
  next();
});

// Parse JSON bodies
app.use(express.json());

// Session sorting priority
const GROUP_PRIORITY = {
  'hq': 0,
  'main': 1,
};

function getGroupPriority(group) {
  if (GROUP_PRIORITY[group] !== undefined) return GROUP_PRIORITY[group];
  if (group.startsWith('gt-')) return 2; // Rigs after main
  return 3; // Other
}

function categorizeSession(name) {
  if (name.startsWith('hq-')) return 'hq';
  if (name === 'main' || name === 'shell') return 'main';
  if (name.startsWith('gt-')) {
    // Extract rig name: gt-gastown-jack → gt-gastown
    const parts = name.split('-');
    if (parts.length >= 2) {
      return parts.slice(0, 2).join('-');
    }
    return 'gt-unknown';
  }
  return 'other';
}

function extractAgentName(sessionName) {
  // gt-gastown-jack → jack
  // hq-mayor → mayor
  const parts = sessionName.split('-');
  if (parts.length >= 3 && sessionName.startsWith('gt-')) {
    return parts.slice(2).join('-');
  }
  if (parts.length >= 2) {
    return parts.slice(1).join('-');
  }
  return sessionName;
}

app.get('/api/tmux/sessions', (req, res) => {
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

    res.json({
      sessions,
      grouped,
      timestamp: new Date().toISOString()
    });
  } catch (e) {
    // tmux server might not be running - various error messages possible
    const noServerErrors = ['no server running', 'No such file or directory', 'error connecting'];
    const isNoServer = noServerErrors.some(msg => e.message.includes(msg));

    res.json({
      sessions: [],
      grouped: {},
      // Don't expose error for expected "no sessions" case
      ...(isNoServer ? {} : { error: e.message }),
      timestamp: new Date().toISOString()
    });
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
    }

    // Sanitize session name (alphanumeric, dash, underscore only)
    name = name.replace(/[^a-zA-Z0-9_-]/g, '-').substring(0, 50);

    // Create the session (detached) - API runs as dev, same user as ttyd
    execSync(`tmux new-session -d -s "${name}"`, { encoding: 'utf-8', timeout: 5000, ...TMUX_ENV });

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

// Health check
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

const PORT = process.env.API_PORT || 3001;
app.listen(PORT, '0.0.0.0', () => {
  console.log(`Arena API server listening on port ${PORT}`);
});
