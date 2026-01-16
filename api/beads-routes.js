// Beads Module API Routes
// ============================================================================
// This is a self-contained API module for beads_viewer integration.
// It can be safely removed without affecting the rest of the application.
//
// To integrate: Add to server.js:
//   const beadsRoutes = require('./beads-routes');
//   app.use('/api/beads', beadsRoutes);
//
// To remove:
//   1. Delete this file
//   2. Remove the require and app.use lines from server.js
//
// See ../beads_viewer_integration/REMOVAL_GUIDE.md for detailed instructions.
// ============================================================================

const express = require('express');
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const router = express.Router();

// ============================================================================
// CONFIGURATION
// ============================================================================

const DEFAULT_PROJECT_PATH = process.env.BEADS_PROJECT_PATH || '/workspace';
const BV_COMMAND = process.env.BV_COMMAND || 'bv';
const EXEC_TIMEOUT = 60000; // 60 seconds for robot commands
const MAX_BUFFER = 10 * 1024 * 1024; // 10MB

// Allowed root paths for project selection (security: restrict path traversal)
const ALLOWED_ROOTS = (process.env.BEADS_ALLOWED_ROOTS || '/code,/workspace')
  .split(',')
  .map(r => path.resolve(r.trim()));

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Validate and sanitize project path
 * SECURITY: Ensures path is within allowed roots to prevent path traversal
 */
function validateProjectPath(inputPath) {
  const projectPath = inputPath || DEFAULT_PROJECT_PATH;

  // Resolve to absolute path (handles .., ., symlinks)
  const resolved = path.resolve(projectPath);

  // SECURITY: Check if resolved path is within any allowed root
  const isAllowed = ALLOWED_ROOTS.some(root => {
    // Ensure the resolved path starts with the root and is either exact match
    // or followed by a path separator (prevents /code-evil matching /code)
    return resolved === root || resolved.startsWith(root + path.sep);
  });

  if (!isAllowed) {
    throw new Error(`Project path not in allowed roots: ${resolved}. Allowed: ${ALLOWED_ROOTS.join(', ')}`);
  }

  // Check if path exists
  if (!fs.existsSync(resolved)) {
    throw new Error(`Project path does not exist: ${resolved}`);
  }

  return resolved;
}

/**
 * Check if .beads directory exists
 */
function checkBeadsDirectory(projectPath) {
  const beadsPath = path.join(projectPath, '.beads');
  if (!fs.existsSync(beadsPath)) {
    throw new Error(`No .beads directory found in ${projectPath}`);
  }
  return beadsPath;
}

/**
 * Execute bv command and return JSON result
 * SECURITY: Uses execFileSync to avoid shell injection
 * @param {string} flag - Single flag like '--robot-triage' (validated)
 * @param {string} projectPath - Already validated project path
 */
function execBvCommand(flag, projectPath, timeout = EXEC_TIMEOUT) {
  // Validate flag format (must start with -- and contain only alphanumeric/dash)
  if (!/^--[a-zA-Z0-9-]+$/.test(flag)) {
    throw new Error(`Invalid bv flag: ${flag}`);
  }

  try {
    // Use execFileSync with args array to avoid shell injection
    const result = execFileSync(BV_COMMAND, [flag, projectPath], {
      encoding: 'utf-8',
      timeout,
      maxBuffer: MAX_BUFFER,
      cwd: projectPath,
    });

    return JSON.parse(result);
  } catch (error) {
    // Check if bv is not installed
    if (error.message.includes('not found') || error.message.includes('not recognized') || error.code === 'ENOENT') {
      throw new Error('bv command not found. Please install beads_viewer.');
    }

    // Check if it's a JSON parse error (command succeeded but output isn't JSON)
    if (error instanceof SyntaxError) {
      throw new Error('Failed to parse bv output as JSON');
    }

    // Re-throw with more context
    throw new Error(`bv command failed: ${error.message}`);
  }
}

/**
 * Create standardized API response
 */
function createResponse(success, data = null, error = null) {
  const response = {
    success,
    timestamp: new Date().toISOString(),
  };

  if (success && data) {
    response.data = data;
  }

  if (!success && error) {
    response.error = {
      code: error.code || 'UNKNOWN_ERROR',
      message: error.message || 'An unknown error occurred',
      details: error.details || undefined,
    };
  }

  return response;
}

/**
 * Error handler wrapper for routes
 */
function asyncHandler(fn) {
  return (req, res, next) => {
    Promise.resolve(fn(req, res, next)).catch((error) => {
      console.error('Beads API Error:', error);

      let code = 'INTERNAL_ERROR';
      let statusCode = 500;

      if (error.message.includes('not found')) {
        code = 'NOT_FOUND';
        statusCode = 404;
      } else if (error.message.includes('not installed') || error.message.includes('not recognized')) {
        code = 'BV_NOT_INSTALLED';
        statusCode = 503;
      } else if (error.message.includes('timeout')) {
        code = 'TIMEOUT';
        statusCode = 504;
      }

      res.status(statusCode).json(createResponse(false, null, {
        code,
        message: error.message,
      }));
    });
  };
}

// ============================================================================
// ROUTES
// ============================================================================

/**
 * GET /api/beads/health
 * Health check endpoint
 */
router.get('/health', (req, res) => {
  res.json(createResponse(true, { status: 'ok' }));
});

/**
 * GET /api/beads/issues
 * Fetch all issues from a project
 */
router.get('/issues', asyncHandler(async (req, res) => {
  const projectPath = validateProjectPath(req.query.path);
  const beadsPath = checkBeadsDirectory(projectPath);
  const issuesFile = path.join(beadsPath, 'issues.jsonl');

  if (!fs.existsSync(issuesFile)) {
    throw new Error(`No issues.jsonl file found in ${beadsPath}`);
  }

  const content = fs.readFileSync(issuesFile, 'utf-8');
  const lines = content.trim().split('\n').filter(line => line.trim());

  const issues = [];
  for (const line of lines) {
    try {
      issues.push(JSON.parse(line));
    } catch (e) {
      console.warn('Failed to parse issue line:', line.substring(0, 100));
    }
  }

  res.json(createResponse(true, {
    issues,
    totalCount: issues.length,
    projectPath,
  }));
}));

/**
 * GET /api/beads/triage
 * Fetch AI triage recommendations
 */
router.get('/triage', asyncHandler(async (req, res) => {
  const projectPath = validateProjectPath(req.query.path);
  checkBeadsDirectory(projectPath);

  // Try to use bv robot command
  try {
    const result = execBvCommand('--robot-triage', projectPath);
    res.json(createResponse(true, result));
  } catch (error) {
    // If bv command fails, generate mock triage from issues
    console.warn('bv --robot-triage failed, generating fallback:', error.message);

    const beadsPath = path.join(projectPath, '.beads');
    const issuesFile = path.join(beadsPath, 'issues.jsonl');

    if (!fs.existsSync(issuesFile)) {
      throw error; // Re-throw original error if we can't fall back
    }

    const content = fs.readFileSync(issuesFile, 'utf-8');
    const issues = content.trim().split('\n')
      .filter(line => line.trim())
      .map(line => {
        try { return JSON.parse(line); } catch { return null; }
      })
      .filter(Boolean);

    // Generate basic triage
    const openIssues = issues.filter(i =>
      i.status === 'open' || i.status === 'in_progress'
    );

    const blockedIssues = issues.filter(i => i.status === 'blocked');

    const recommendations = openIssues
      .sort((a, b) => (a.priority || 5) - (b.priority || 5))
      .slice(0, 10)
      .map((issue, idx) => ({
        issueId: issue.id,
        rank: idx + 1,
        reasoning: `Priority ${issue.priority || 'unset'} ${issue.type || 'task'}`,
        unblockChain: issue.dependencies || [],
        estimatedImpact: issue.priority <= 2 ? 'high' : issue.priority <= 3 ? 'medium' : 'low',
      }));

    const quickWins = openIssues
      .filter(i => (!i.dependencies || i.dependencies.length === 0) && (i.priority || 5) >= 3)
      .slice(0, 5)
      .map(i => i.id);

    res.json(createResponse(true, {
      recommendations,
      quickWins,
      blockers: blockedIssues.map(i => i.id),
      timestamp: new Date().toISOString(),
    }));
  }
}));

/**
 * GET /api/beads/insights
 * Fetch graph metrics and insights
 */
router.get('/insights', asyncHandler(async (req, res) => {
  const projectPath = validateProjectPath(req.query.path);
  checkBeadsDirectory(projectPath);

  // Try to use bv robot command
  try {
    const result = execBvCommand('--robot-insights', projectPath);
    res.json(createResponse(true, result));
  } catch (error) {
    // Generate fallback insights from issues
    console.warn('bv --robot-insights failed, generating fallback:', error.message);

    const beadsPath = path.join(projectPath, '.beads');
    const issuesFile = path.join(beadsPath, 'issues.jsonl');

    if (!fs.existsSync(issuesFile)) {
      throw error;
    }

    const content = fs.readFileSync(issuesFile, 'utf-8');
    const issues = content.trim().split('\n')
      .filter(line => line.trim())
      .map(line => {
        try { return JSON.parse(line); } catch { return null; }
      })
      .filter(Boolean);

    // Calculate basic metrics
    const issueCount = issues.length;
    const openCount = issues.filter(i =>
      i.status === 'open' || i.status === 'in_progress'
    ).length;
    const blockedCount = issues.filter(i => i.status === 'blocked').length;

    // Calculate degree for each issue
    const degree = {};
    issues.forEach(issue => {
      const deps = (issue.dependencies || []).length;
      const blocks = (issue.blocking || []).length;
      degree[issue.id] = deps + blocks;
    });

    // Basic density calculation
    const totalDeps = issues.reduce((sum, i) =>
      sum + (i.dependencies || []).length + (i.blocking || []).length, 0
    );
    const maxPossibleEdges = issueCount * (issueCount - 1);
    const density = maxPossibleEdges > 0 ? totalDeps / maxPossibleEdges : 0;

    // Find cycles (simplified - just detect self-references and immediate cycles)
    const cycles = [];
    issues.forEach(issue => {
      if (issue.dependencies?.includes(issue.id)) {
        cycles.push([issue.id]);
      }
      issue.dependencies?.forEach(depId => {
        const depIssue = issues.find(i => i.id === depId);
        if (depIssue?.dependencies?.includes(issue.id)) {
          const cycle = [issue.id, depId].sort();
          if (!cycles.some(c => c.join() === cycle.join())) {
            cycles.push(cycle);
          }
        }
      });
    });

    // Calculate health score
    let healthScore = 100;
    const risks = [];
    const warnings = [];

    if (blockedCount > issueCount * 0.3) {
      healthScore -= 20;
      risks.push(`High blocked ratio: ${blockedCount}/${issueCount} issues blocked`);
    }

    if (cycles.length > 0) {
      healthScore -= cycles.length * 10;
      risks.push(`${cycles.length} circular dependencies detected`);
    }

    if (density > 0.5) {
      healthScore -= 10;
      warnings.push('High dependency density may indicate over-coupling');
    }

    healthScore = Math.max(0, Math.min(100, healthScore));

    res.json(createResponse(true, {
      metrics: {
        pageRank: {}, // Would need graph library for proper calculation
        betweenness: {},
        eigenvector: {},
        degree,
        criticalPath: [],
        cycles,
        density,
      },
      health: {
        score: healthScore,
        risks,
        warnings,
      },
      issueCount,
      openCount,
      blockedCount,
      timestamp: new Date().toISOString(),
    }));
  }
}));

/**
 * GET /api/beads/plan
 * Fetch execution plan
 */
router.get('/plan', asyncHandler(async (req, res) => {
  const projectPath = validateProjectPath(req.query.path);
  checkBeadsDirectory(projectPath);

  try {
    const result = execBvCommand('--robot-plan', projectPath);
    res.json(createResponse(true, result));
  } catch (error) {
    // Return empty plan as fallback
    console.warn('bv --robot-plan failed:', error.message);
    res.json(createResponse(true, {
      tracks: [],
      executionOrder: [],
      parallelizable: [],
      timestamp: new Date().toISOString(),
    }));
  }
}));

/**
 * GET /api/beads/projects
 * Discover available beads projects
 *
 * Query params:
 *   root - Root path to search from (default: searches common mount points)
 *   depth - Maximum directory depth to search (default: 3)
 *
 * By default, searches these mount points:
 *   - /code (E:/Code)
 *   - /workspace
 */
router.get('/projects', asyncHandler(async (req, res) => {
  const maxDepth = parseInt(req.query.depth) || 3;

  // Default search roots - common mount points in Arena
  const defaultRoots = ['/code', '/workspace'];
  const searchRoots = req.query.root ? [req.query.root] : defaultRoots;

  const projects = [];
  const searched = new Set();

  function findBeadsProjects(dir, depth) {
    if (depth > maxDepth) return;
    if (searched.has(dir)) return;
    searched.add(dir);

    try {
      const beadsPath = path.join(dir, '.beads');
      if (fs.existsSync(beadsPath) && fs.statSync(beadsPath).isDirectory()) {
        // Check for issues.jsonl to confirm it's a valid beads project
        const issuesPath = path.join(beadsPath, 'issues.jsonl');
        if (fs.existsSync(issuesPath)) {
          projects.push(dir);
        }
      }

      const entries = fs.readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        if (entry.isDirectory() && !entry.name.startsWith('.') && entry.name !== 'node_modules') {
          findBeadsProjects(path.join(dir, entry.name), depth + 1);
        }
      }
    } catch (e) {
      // Skip directories we can't read
    }
  }

  // Search all root paths
  for (const root of searchRoots) {
    if (fs.existsSync(root)) {
      findBeadsProjects(root, 0);
    }
  }

  res.json(createResponse(true, {
    projects,
    searchRoots: searchRoots.filter(r => fs.existsSync(r)),
  }));
}));

module.exports = router;
