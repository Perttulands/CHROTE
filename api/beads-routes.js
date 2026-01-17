// Beads Routes - API endpoints for beads_viewer integration
// Requires bv CLI to be installed - NO FALLBACKS

const express = require('express');
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const router = express.Router();

// Configuration
const BV_COMMAND = 'bv';
const EXEC_TIMEOUT = 60000; // 60 seconds for bv commands
const ALLOWED_ROOTS = ['/code', '/workspace'].map(r => path.resolve(r));

// Helper: Validate project path is within allowed roots
function validateProjectPath(inputPath) {
  if (!inputPath) {
    const error = new Error('Missing required query parameter: path');
    error.code = 'BAD_REQUEST';
    throw error;
  }

  const resolved = path.resolve(inputPath);

  const isAllowed = ALLOWED_ROOTS.some(root => {
    return resolved === root || resolved.startsWith(root + path.sep);
  });

  if (!isAllowed) {
    const error = new Error(`Project path not in allowed roots: ${resolved}. Allowed: ${ALLOWED_ROOTS.join(', ')}`);
    error.code = 'FORBIDDEN';
    throw error;
  }

  if (!fs.existsSync(resolved)) {
    const error = new Error(`Project path does not exist: ${resolved}`);
    error.code = 'NOT_FOUND';
    throw error;
  }

  return resolved;
}

// Helper: Check if .beads directory exists
function checkBeadsDirectory(projectPath) {
  const beadsPath = path.join(projectPath, '.beads');
  if (!fs.existsSync(beadsPath)) {
    const error = new Error(`No .beads directory found in ${projectPath}. Run 'bv init' to create one.`);
    error.code = 'NOT_FOUND';
    throw error;
  }
  return beadsPath;
}

// Helper: Check if bv command is available
function checkBvInstalled() {
  try {
    execFileSync(BV_COMMAND, ['--version'], {
      encoding: 'utf-8',
      timeout: 5000,
    });
  } catch (error) {
    const err = new Error(`bv command not found. Install beads_viewer: go install github.com/Dicklesworthstone/beads_viewer@latest`);
    err.code = 'BV_NOT_INSTALLED';
    throw err;
  }
}

// Helper: Execute bv command safely
function execBvCommand(flag, projectPath, timeout = EXEC_TIMEOUT) {
  // Validate flag format (must be --something)
  if (!/^--[a-zA-Z0-9-]+$/.test(flag)) {
    const error = new Error(`Invalid bv flag format: ${flag}`);
    error.code = 'BAD_REQUEST';
    throw error;
  }

  try {
    const result = execFileSync(BV_COMMAND, [flag, projectPath], {
      encoding: 'utf-8',
      timeout,
      maxBuffer: 10 * 1024 * 1024, // 10MB buffer
      cwd: projectPath,
    });

    try {
      return JSON.parse(result);
    } catch (parseError) {
      const error = new Error(`bv ${flag} returned invalid JSON: ${parseError.message}. Output: ${result.substring(0, 200)}`);
      error.code = 'BV_INVALID_OUTPUT';
      throw error;
    }
  } catch (error) {
    if (error.code === 'BV_INVALID_OUTPUT') {
      throw error;
    }
    if (error.message.includes('not found') || error.message.includes('ENOENT') || error.code === 'ENOENT') {
      const err = new Error(`bv command not found. Install beads_viewer: go install github.com/Dicklesworthstone/beads_viewer@latest`);
      err.code = 'BV_NOT_INSTALLED';
      throw err;
    }
    if (error.killed || error.signal === 'SIGTERM') {
      const err = new Error(`bv ${flag} timed out after ${timeout}ms`);
      err.code = 'BV_TIMEOUT';
      throw err;
    }
    const err = new Error(`bv ${flag} failed: ${error.message}`);
    err.code = 'BV_ERROR';
    throw err;
  }
}

// Helper: Create standard response format
function createResponse(success, data = null, error = null) {
  const response = { success, timestamp: new Date().toISOString() };
  if (success && data) response.data = data;
  if (!success && error) {
    response.error = {
      code: error.code || 'INTERNAL_ERROR',
      message: error.message || 'An unknown error occurred',
    };
  }
  return response;
}

// Helper: Parse JSONL file with strict error handling
function parseJsonlFile(filePath) {
  if (!fs.existsSync(filePath)) {
    const error = new Error(`File not found: ${filePath}`);
    error.code = 'NOT_FOUND';
    throw error;
  }

  const content = fs.readFileSync(filePath, 'utf-8');
  const lines = content.split('\n');
  const items = [];
  const errors = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line) continue; // Skip empty lines

    try {
      items.push(JSON.parse(line));
    } catch (e) {
      errors.push(`Line ${i + 1}: ${e.message}`);
    }
  }

  if (errors.length > 0) {
    const error = new Error(`JSONL parse errors in ${filePath}:\n${errors.join('\n')}`);
    error.code = 'INVALID_JSONL';
    throw error;
  }

  return items;
}

// Helper: Get error status code
function getErrorStatusCode(error) {
  switch (error.code) {
    case 'BAD_REQUEST': return 400;
    case 'FORBIDDEN': return 403;
    case 'NOT_FOUND': return 404;
    case 'INVALID_JSONL': return 422;
    case 'BV_NOT_INSTALLED': return 503;
    case 'BV_TIMEOUT': return 504;
    case 'BV_ERROR': return 502;
    case 'BV_INVALID_OUTPUT': return 502;
    default: return 500;
  }
}

// ============================================================================
// ROUTES
// ============================================================================

// GET /api/beads/health - Health check
router.get('/health', (req, res) => {
  try {
    // Check if bv is installed
    let bvVersion = null;
    try {
      bvVersion = execFileSync(BV_COMMAND, ['--version'], {
        encoding: 'utf-8',
        timeout: 5000,
      }).trim();
    } catch (e) {
      return res.status(503).json(createResponse(false, null, {
        code: 'BV_NOT_INSTALLED',
        message: `bv command not found. Install beads_viewer: go install github.com/Dicklesworthstone/beads_viewer@latest`,
      }));
    }

    res.json(createResponse(true, {
      status: 'ok',
      bvVersion,
      allowedRoots: ALLOWED_ROOTS,
    }));
  } catch (error) {
    res.status(500).json(createResponse(false, null, error));
  }
});

// GET /api/beads/projects - Discover projects with .beads directories
router.get('/projects', (req, res) => {
  try {
    const projects = [];
    const errors = [];

    for (const root of ALLOWED_ROOTS) {
      if (!fs.existsSync(root)) {
        errors.push(`Allowed root does not exist: ${root}`);
        continue;
      }

      try {
        const entries = fs.readdirSync(root, { withFileTypes: true });
        for (const entry of entries) {
          if (entry.isDirectory()) {
            const projectPath = path.join(root, entry.name);
            const beadsPath = path.join(projectPath, '.beads');
            if (fs.existsSync(beadsPath)) {
              projects.push({
                name: entry.name,
                path: projectPath,
                beadsPath,
              });
            }
          }
        }
      } catch (e) {
        errors.push(`Cannot read directory ${root}: ${e.message}`);
      }
    }

    // Also check if root directories themselves have .beads
    for (const root of ALLOWED_ROOTS) {
      if (!fs.existsSync(root)) continue;
      const beadsPath = path.join(root, '.beads');
      if (fs.existsSync(beadsPath)) {
        projects.push({
          name: path.basename(root),
          path: root,
          beadsPath,
        });
      }
    }

    if (projects.length === 0 && errors.length > 0) {
      const error = new Error(`No projects found. Errors: ${errors.join('; ')}`);
      error.code = 'NOT_FOUND';
      return res.status(404).json(createResponse(false, null, error));
    }

    res.json(createResponse(true, { projects, warnings: errors.length > 0 ? errors : undefined }));
  } catch (error) {
    res.status(500).json(createResponse(false, null, error));
  }
});

// GET /api/beads/issues - List issues from .beads/issues.jsonl
router.get('/issues', (req, res) => {
  try {
    const projectPath = validateProjectPath(req.query.path);
    const beadsPath = checkBeadsDirectory(projectPath);
    const issuesFile = path.join(beadsPath, 'issues.jsonl');

    if (!fs.existsSync(issuesFile)) {
      const error = new Error(`No issues.jsonl file found in ${beadsPath}. Create issues with 'bv add'.`);
      error.code = 'NOT_FOUND';
      throw error;
    }

    const issues = parseJsonlFile(issuesFile);

    res.json(createResponse(true, {
      issues,
      totalCount: issues.length,
      projectPath,
    }));
  } catch (error) {
    res.status(getErrorStatusCode(error)).json(createResponse(false, null, error));
  }
});

// GET /api/beads/triage - Get triage recommendations (requires bv)
router.get('/triage', (req, res) => {
  try {
    checkBvInstalled();
    const projectPath = validateProjectPath(req.query.path);
    checkBeadsDirectory(projectPath);

    const result = execBvCommand('--robot-triage', projectPath);
    res.json(createResponse(true, result));
  } catch (error) {
    res.status(getErrorStatusCode(error)).json(createResponse(false, null, error));
  }
});

// GET /api/beads/insights - Get graph metrics and health (requires bv)
router.get('/insights', (req, res) => {
  try {
    checkBvInstalled();
    const projectPath = validateProjectPath(req.query.path);
    checkBeadsDirectory(projectPath);

    const result = execBvCommand('--robot-insights', projectPath);
    res.json(createResponse(true, result));
  } catch (error) {
    res.status(getErrorStatusCode(error)).json(createResponse(false, null, error));
  }
});

// GET /api/beads/graph - Get dependency graph data (requires bv)
router.get('/graph', (req, res) => {
  try {
    checkBvInstalled();
    const projectPath = validateProjectPath(req.query.path);
    checkBeadsDirectory(projectPath);

    const result = execBvCommand('--robot-graph', projectPath);
    res.json(createResponse(true, result));
  } catch (error) {
    res.status(getErrorStatusCode(error)).json(createResponse(false, null, error));
  }
});

module.exports = router;
