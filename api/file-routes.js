// File API routes - replaces filebrowser container
// Direct filesystem access to mounted volumes

const express = require('express');
const fs = require('fs/promises');
const path = require('path');
const router = express.Router();

// Allowed root paths (already mounted in container)
const ALLOWED_ROOTS = ['/code', '/vault'];

// Validate and resolve path - CRITICAL for security
function resolveSafePath(requestPath) {
  // Normalize and decode
  const decoded = decodeURIComponent(requestPath || '/');
  const normalized = path.normalize(decoded);

  // Must start with allowed root
  const matchedRoot = ALLOWED_ROOTS.find(root =>
    normalized === root || normalized.startsWith(root + '/')
  );

  if (!matchedRoot) {
    // Root listing - return list of allowed roots
    if (normalized === '/' || normalized === '') {
      return { isRoot: true };
    }
    return { error: 'Path not allowed' };
  }

  // Resolve to absolute and verify it's still under allowed root
  const resolved = path.resolve(normalized);
  if (!resolved.startsWith(matchedRoot)) {
    return { error: 'Path traversal detected' };
  }

  return { path: resolved, root: matchedRoot };
}

// GET /resources/ - Root listing
router.get('/resources/', async (req, res) => {
  res.json({
    isDir: true,
    items: ALLOWED_ROOTS.map(root => ({
      name: root.slice(1), // Remove leading /
      size: 0,
      modified: new Date().toISOString(),
      isDir: true,
      type: ''
    }))
  });
});

// GET /resources/* - List directory or get file info
router.get('/resources/*', async (req, res) => {
  const requestPath = '/' + (req.params[0] || '');
  const result = resolveSafePath(requestPath);

  if (result.error) {
    return res.status(403).json({ error: result.error });
  }

  if (result.isRoot) {
    // Return virtual root listing
    return res.json({
      isDir: true,
      items: ALLOWED_ROOTS.map(root => ({
        name: root.slice(1),
        size: 0,
        modified: new Date().toISOString(),
        isDir: true,
        type: ''
      }))
    });
  }

  try {
    const stat = await fs.stat(result.path);

    if (stat.isDirectory()) {
      const entries = await fs.readdir(result.path, { withFileTypes: true });
      const items = await Promise.all(entries.map(async entry => {
        const fullPath = path.join(result.path, entry.name);
        try {
          const entryStat = await fs.stat(fullPath);
          return {
            name: entry.name,
            size: entryStat.size,
            modified: entryStat.mtime.toISOString(),
            isDir: entry.isDirectory(),
            type: entry.isDirectory() ? '' : path.extname(entry.name).slice(1)
          };
        } catch {
          return null; // Skip inaccessible files
        }
      }));

      res.json({
        isDir: true,
        items: items.filter(Boolean)
      });
    } else {
      res.json({
        isDir: false,
        name: path.basename(result.path),
        size: stat.size,
        modified: stat.mtime.toISOString(),
        type: path.extname(result.path).slice(1)
      });
    }
  } catch (err) {
    if (err.code === 'ENOENT') {
      return res.status(404).json({ error: 'Not found' });
    }
    res.status(500).json({ error: err.message });
  }
});

// POST /resources/* - Create folder or upload file
router.post('/resources/*', express.raw({ type: '*/*', limit: '100mb' }), async (req, res) => {
  const requestPath = '/' + (req.params[0] || '');
  const result = resolveSafePath(requestPath);

  if (result.error || result.isRoot) {
    return res.status(403).json({ error: result.error || 'Cannot create at root' });
  }

  try {
    // If path ends with /, create directory
    if (requestPath.endsWith('/')) {
      await fs.mkdir(result.path, { recursive: true });
      return res.json({ success: true });
    }

    // Otherwise, upload file
    await fs.mkdir(path.dirname(result.path), { recursive: true });
    await fs.writeFile(result.path, req.body);
    res.json({ success: true });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// PATCH /resources/* - Rename/move
router.patch('/resources/*', express.json(), async (req, res) => {
  const requestPath = '/' + (req.params[0] || '');
  const result = resolveSafePath(requestPath);

  if (result.error || result.isRoot) {
    return res.status(403).json({ error: result.error || 'Cannot rename root' });
  }

  const { destination, action } = req.body;
  if (action !== 'rename' || !destination) {
    return res.status(400).json({ error: 'Invalid request' });
  }

  const destResult = resolveSafePath(destination);
  if (destResult.error || destResult.isRoot) {
    return res.status(403).json({ error: 'Invalid destination' });
  }

  try {
    await fs.rename(result.path, destResult.path);
    res.json({ success: true });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// DELETE /resources/* - Delete file/folder
router.delete('/resources/*', async (req, res) => {
  const requestPath = '/' + (req.params[0] || '');
  const result = resolveSafePath(requestPath);

  if (result.error || result.isRoot) {
    return res.status(403).json({ error: result.error || 'Cannot delete root' });
  }

  try {
    const stat = await fs.stat(result.path);
    if (stat.isDirectory()) {
      await fs.rm(result.path, { recursive: true });
    } else {
      await fs.unlink(result.path);
    }
    res.json({ success: true });
  } catch (err) {
    if (err.code === 'ENOENT') {
      return res.status(404).json({ error: 'Not found' });
    }
    res.status(500).json({ error: err.message });
  }
});

// GET /raw/* - Download file
router.get('/raw/*', async (req, res) => {
  const requestPath = '/' + (req.params[0] || '');
  const result = resolveSafePath(requestPath);

  if (result.error || result.isRoot) {
    return res.status(403).json({ error: result.error || 'Cannot download root' });
  }

  try {
    const stat = await fs.stat(result.path);
    if (stat.isDirectory()) {
      return res.status(400).json({ error: 'Cannot download directory' });
    }

    res.download(result.path);
  } catch (err) {
    if (err.code === 'ENOENT') {
      return res.status(404).json({ error: 'Not found' });
    }
    res.status(500).json({ error: err.message });
  }
});

module.exports = router;
