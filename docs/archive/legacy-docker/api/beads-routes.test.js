// Beads Routes API Tests
// Tests path validation, error handling, and API contracts
// NO MOCKS - tests actual routes with real error handling

const request = require('supertest');
const express = require('express');
const path = require('path');

// Import the actual routes
const beadsRoutes = require('./beads-routes');

// Create test app with actual routes
function createTestApp() {
  const app = express();
  app.use(express.json());
  app.use('/api/beads', beadsRoutes);
  return app;
}

// Cross-platform path constants
const RESOLVED_CODE = path.resolve('/code');
const RESOLVED_WORKSPACE = path.resolve('/workspace');

// ============================================================================
// PATH VALIDATION TESTS (Security)
// ============================================================================

describe('Beads API: Path validation (security)', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('rejects missing path parameter with clear error', async () => {
    const res = await request(app)
      .get('/api/beads/issues')
      .expect(400);

    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBe('BAD_REQUEST');
    expect(res.body.error.message).toContain('Missing required query parameter: path');
  });

  test('rejects paths outside allowed roots with clear error', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/etc/passwd')
      .expect(403);

    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBe('FORBIDDEN');
    expect(res.body.error.message).toContain('not in allowed roots');
    expect(res.body.error.message).toContain('Allowed:');
  });

  test('rejects path traversal attempts', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/code/../etc/passwd')
      .expect(403);

    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBe('FORBIDDEN');
  });

  test('rejects /code-evil (path prefix attack)', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/code-evil')
      .expect(403);

    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBe('FORBIDDEN');
  });

  test('rejects absolute paths outside roots', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/root/.ssh/id_rsa')
      .expect(403);

    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBe('FORBIDDEN');
  });
});

// ============================================================================
// MISSING RESOURCES TESTS
// ============================================================================

describe('Beads API: Missing resources', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('returns 404 when project path does not exist', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/code/nonexistent-project-xyz-12345')
      .expect(404);

    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBe('NOT_FOUND');
    expect(res.body.error.message).toContain('does not exist');
  });
});

// ============================================================================
// HEALTH ENDPOINT TESTS
// ============================================================================

describe('Beads API: Health endpoint', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('returns bv installation status (success or clear error)', async () => {
    const res = await request(app)
      .get('/api/beads/health');

    // Either bv is installed (200) or not (503)
    expect([200, 503]).toContain(res.status);

    if (res.status === 200) {
      expect(res.body.success).toBe(true);
      expect(res.body.data.bvVersion).toBeDefined();
      expect(res.body.data.allowedRoots).toBeDefined();
      expect(Array.isArray(res.body.data.allowedRoots)).toBe(true);
    } else {
      expect(res.body.success).toBe(false);
      expect(res.body.error.code).toBe('BV_NOT_INSTALLED');
      expect(res.body.error.message).toContain('go install');
      expect(res.body.error.message).toContain('beads_viewer');
    }
  });
});

// ============================================================================
// PROJECTS ENDPOINT TESTS
// ============================================================================

describe('Beads API: Projects endpoint', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('returns projects list or warnings', async () => {
    const res = await request(app)
      .get('/api/beads/projects');

    // Projects endpoint can return 200 (with projects or empty) or 404 (all roots missing)
    expect([200, 404]).toContain(res.status);

    if (res.status === 200) {
      expect(res.body.success).toBe(true);
      expect(Array.isArray(res.body.data.projects)).toBe(true);
      // May have warnings if some roots don't exist
      if (res.body.data.warnings) {
        expect(Array.isArray(res.body.data.warnings)).toBe(true);
      }
    } else {
      expect(res.body.success).toBe(false);
      expect(res.body.error.code).toBe('NOT_FOUND');
    }
  });
});

// ============================================================================
// ERROR RESPONSE FORMAT TESTS
// ============================================================================

describe('Beads API: Error response format', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('error responses have consistent structure', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/etc/passwd')
      .expect(403);

    // Check structure
    expect(res.body).toHaveProperty('success', false);
    expect(res.body).toHaveProperty('timestamp');
    expect(res.body).toHaveProperty('error');
    expect(res.body.error).toHaveProperty('code');
    expect(res.body.error).toHaveProperty('message');

    // Timestamp should be valid ISO string
    expect(() => new Date(res.body.timestamp)).not.toThrow();
    expect(new Date(res.body.timestamp).toISOString()).toBe(res.body.timestamp);

    // Error code should be a string
    expect(typeof res.body.error.code).toBe('string');
    expect(res.body.error.code.length).toBeGreaterThan(0);

    // Error message should be descriptive
    expect(typeof res.body.error.message).toBe('string');
    expect(res.body.error.message.length).toBeGreaterThan(10);
  });

  test('success responses have consistent structure', async () => {
    const res = await request(app)
      .get('/api/beads/projects');

    // Check structure
    expect(res.body).toHaveProperty('success');
    expect(res.body).toHaveProperty('timestamp');

    if (res.body.success) {
      expect(res.body).toHaveProperty('data');
      expect(res.body).not.toHaveProperty('error');
    } else {
      expect(res.body).toHaveProperty('error');
    }
  });
});

// ============================================================================
// BV-DEPENDENT ENDPOINTS TESTS
// ============================================================================

describe('Beads API: bv-dependent endpoints require bv (no fallbacks)', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('/triage returns clear error without fallback', async () => {
    const res = await request(app)
      .get('/api/beads/triage?path=/code/test-project');

    // Should be one of: 400 (bad request), 404 (not found), 503 (bv not installed), 502 (bv error)
    expect([400, 404, 502, 503]).toContain(res.status);
    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBeDefined();
    expect(res.body.error.message).toBeDefined();
    expect(res.body.error.message.length).toBeGreaterThan(10);

    // Should NOT have fallback data
    expect(res.body.data).toBeUndefined();
  });

  test('/insights returns clear error without fallback', async () => {
    const res = await request(app)
      .get('/api/beads/insights?path=/code/test-project');

    expect([400, 404, 502, 503]).toContain(res.status);
    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBeDefined();
    expect(res.body.error.message).toBeDefined();

    // Should NOT have fallback data
    expect(res.body.data).toBeUndefined();
  });

  test('/graph returns clear error without fallback', async () => {
    const res = await request(app)
      .get('/api/beads/graph?path=/code/test-project');

    expect([400, 404, 502, 503]).toContain(res.status);
    expect(res.body.success).toBe(false);
    expect(res.body.error.code).toBeDefined();
    expect(res.body.error.message).toBeDefined();

    // Should NOT have fallback data
    expect(res.body.data).toBeUndefined();
  });
});

// ============================================================================
// ERROR CODE MAPPING TESTS
// ============================================================================

describe('Beads API: Error codes map to correct HTTP status', () => {
  let app;

  beforeEach(() => {
    app = createTestApp();
  });

  test('BAD_REQUEST returns 400', async () => {
    const res = await request(app)
      .get('/api/beads/issues'); // Missing path

    expect(res.status).toBe(400);
    expect(res.body.error.code).toBe('BAD_REQUEST');
  });

  test('FORBIDDEN returns 403', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/etc/passwd');

    expect(res.status).toBe(403);
    expect(res.body.error.code).toBe('FORBIDDEN');
  });

  test('NOT_FOUND returns 404', async () => {
    const res = await request(app)
      .get('/api/beads/issues?path=/code/definitely-not-a-real-project-123456');

    expect(res.status).toBe(404);
    expect(res.body.error.code).toBe('NOT_FOUND');
  });
});
