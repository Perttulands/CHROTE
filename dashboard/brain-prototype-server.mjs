/* global process, URL, fetch, AbortSignal */
// Throwaway, dev-only adapter for evaluating the Brain Library reader.
// Citadel owns parsing, ranking, links and preserved-source validation. The
// prototype keeps its derived index in RAM and never writes to the brain.
// From dashboard/: npm run prototype:brain
// Required env: BRAIN_PROJECT_ROOT (Citadel checkout), CHROTE_PROTOTYPE_BACKEND
// (selected CHROTE URL). Set BRAIN_ROOT, BRAIN_SOURCES and BRAIN_INDEX for the
// selected corpus; CITADEL_URL enables Ask through its trusted loopback service.
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const PREFIX = '/api/brain-prototype/';
const fail = (statusCode, message) => Object.assign(new Error(message), { statusCode });

export default function brainPrototypeServer() {
  let brain, cfg, db;
  const served = row => row && ['knowledge', 'self'].includes(row.collection)
    && (brain.KINDS.includes(row.kind) || row.kind === 'self')
    && !row.path.split(path.sep).some(part => part.startsWith('_') || part === 'AGENTS.md' || part === 'CLAUDE.md');
  const summary = row => ({ slug: row.slug, title: row.title, kind: row.kind,
    summary: row.summary, collection: row.collection,
    facets: row.facets ? row.facets.split(',').filter(Boolean) : [],
    ...(row.updated ? { updated: row.updated } : {}) });

  function initialize() {
    if (db) return;
    if (!process.env.BRAIN_PROJECT_ROOT) throw fail(503, 'BRAIN_PROJECT_ROOT is required to connect the prototype to Context Citadel.');
    const require = createRequire(import.meta.url);
    brain = require(path.join(path.resolve(process.env.BRAIN_PROJECT_ROOT), 'lib/brain.js'));
    cfg = brain.config(process.env);
    if (!fs.existsSync(path.join(cfg.root, 'knowledge'))) throw fail(503, 'The configured brain has no knowledge collection.');
    db = brain.index(cfg, { readOnly: true });
  }

  function rowFor(slug) {
    if (!slug) throw fail(400, 'A page slug is required.');
    const row = brain.resolve(db, slug).find(served);
    if (!row) throw fail(404, `Page not found: ${slug}`);
    return row;
  }

  function parsed(row) {
    return brain.parseFrontmatter(fs.readFileSync(path.join(cfg.root, row.path), 'utf8'));
  }

  function fullPage(slug) {
    const row = rowFor(slug);
    const { meta, body } = parsed(row);
    const outgoing = brain.outgoing(db, row.slug);
    const related = [
      ...outgoing.map(slug => ({ slug, relation: 'links_to' })),
      ...brain.incoming(db, row.slug).filter(slug => !outgoing.includes(slug))
        .map(slug => ({ slug, relation: 'linked_from' })),
    ].flatMap(link => {
      const target = brain.resolve(db, link.slug).find(served);
      return target ? [{ ...summary(target), relation: link.relation }] : [];
    });
    const source = row.kind === 'source' ? Object.fromEntries(
      ['stored', 'origin_path', 'url', 'captured', 'sha256', 'bytes'].filter(key => meta[key] !== undefined)
        .map(key => [key, meta[key]])) : undefined;
    return { ...summary(row), content: body, related, ...(source ? { source } : {}) };
  }

  function sourceWindow(slug, offsetText) {
    const row = rowFor(slug);
    if (row.kind !== 'source') throw fail(404, 'This page is not a source.');
    const offset = Number(offsetText || 0);
    if (!Number.isSafeInteger(offset) || offset < 0) throw fail(400, 'Source offset must be a non-negative integer.');
    const { meta } = parsed(row);
    const source = brain.readSource(cfg, meta);
    if (source.status === 'external') return { status: 'external', reference: meta.origin_path || meta.url || '' };
    const reasons = { binary: 'The source is not a supported text format.', missing: 'The preserved source is absent.',
      corrupt: 'The source failed integrity validation.', secret: 'The source contains credential-like text.',
      too_large: 'The source exceeds the 5 MiB read limit.' };
    const content = source.status === 'ok' ? source.text.slice(offset, offset + 16000) : '';
    const truncated = source.status === 'ok' && offset + content.length < source.text.length;
    return { slug: row.slug, title: row.title, status: source.status,
      sha256: source.sha256, bytes: source.bytes, media: path.extname(meta.blob || '').slice(1).toLowerCase(),
      offset, content, next_offset: truncated ? offset + content.length : null, truncated,
      ...(reasons[source.status] ? { reason: reasons[source.status] } : {}) };
  }

  async function ask(req) {
    let text = '';
    for await (const chunk of req) {
      text += chunk;
      if (text.length > 16000) throw fail(413, 'The question is too long.');
    }
    let input;
    try { input = JSON.parse(text); } catch { throw fail(400, 'Expected a JSON question.'); }
    if (typeof input.question !== 'string' || !input.question.trim()) throw fail(400, 'A question is required.');
    if (!process.env.CITADEL_URL) throw fail(503, 'Ask is unavailable: CITADEL_URL is not configured.');
    const base = new URL(process.env.CITADEL_URL);
    if (!['127.0.0.1', 'localhost', '[::1]'].includes(base.hostname)) throw fail(503, 'Ask requires the trusted local Context Citadel service.');
    const context = input.slug ? rowFor(input.slug) : null;
    const question = context ? `While reading [[${context.slug}]] (${context.title}):\n\n${input.question.trim()}` : input.question.trim();
    const response = await fetch(new URL('/v1/ask-context', base), {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question }), signal: AbortSignal.timeout(180000),
    });
    const raw = await response.text();
    let result;
    try { result = JSON.parse(raw); } catch { result = raw; }
    if (!response.ok) throw fail(response.status, result.error || 'The Ask worker failed.');
    const answer = typeof result === 'string' ? result : result.answer || result.result;
    if (typeof answer !== 'string' || !answer.trim()) throw fail(502, 'The Ask worker returned no answer.');
    const citations = [...new Set([...answer.matchAll(/\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]/g)].map(match => match[1]))]
      .flatMap(slug => {
        const row = brain.resolve(db, slug).find(served);
        return row ? [summary(row)] : [];
      });
    return { answer, citations };
  }

  return {
    name: 'brain-library-prototype',
    apply: 'serve',
    configureServer(server) {
      server.httpServer?.once('close', () => { db?.close(); db = undefined; });
      server.middlewares.use(async (req, res, next) => {
        if (!req.url?.startsWith(PREFIX)) return next();
        try {
          initialize();
          const url = new URL(req.url, 'http://prototype.local');
          const route = url.pathname.slice(PREFIX.length);
          let result;
          if (req.method === 'GET' && route === 'catalog') {
            if (url.searchParams.get('refresh') === '1') {
              const old = db;
              db = brain.index(cfg, { readOnly: true });
              old.close();
            }
            const rows = db.prepare('SELECT * FROM pages').all().filter(served);
            result = { topics: rows.filter(row => row.kind === 'topic' || row.collection === 'self')
              .map(summary).sort((a, b) => a.title.localeCompare(b.title)),
              facets: brain.facetVocabulary(cfg),
              recent: ['knowledge', 'self'].flatMap(collection => rows.filter(row => row.collection === collection && row.updated)
                .sort((a, b) => b.updated.localeCompare(a.updated) || a.title.localeCompare(b.title))
                .slice(0, 16)).map(summary),
              counts: { knowledge: rows.filter(row => row.collection === 'knowledge').length,
                self: rows.filter(row => row.collection === 'self').length } };
          } else if (req.method === 'GET' && route === 'search') {
            const collection = url.searchParams.get('collection') || 'all';
            if (!['all', 'knowledge', 'self'].includes(collection)) throw fail(400, 'Unknown collection.');
            result = { results: brain.search(db, url.searchParams.get('q') || '', {
              collection: collection === 'all' ? undefined : collection,
              facet: url.searchParams.get('facet') || undefined, limit: 50,
            }).filter(served).map(row => ({ ...summary(row), snippet: row.snip || '' })) };
          } else if (req.method === 'GET' && route === 'page') {
            result = fullPage(url.searchParams.get('slug'));
          } else if (req.method === 'GET' && route === 'source') {
            result = sourceWindow(url.searchParams.get('slug'), url.searchParams.get('offset'));
          } else if (req.method === 'POST' && route === 'ask') {
            result = await ask(req);
          } else throw fail(404, 'Unknown prototype operation.');
          res.statusCode = 200;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.setHeader('Cache-Control', 'no-store');
          res.end(JSON.stringify(result));
        } catch (error) {
          res.statusCode = error.statusCode || 503;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.end(JSON.stringify({ error: error.message || 'The brain could not be read.' }));
        }
      });
    },
  };
}
