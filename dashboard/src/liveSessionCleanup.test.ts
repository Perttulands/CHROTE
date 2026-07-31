import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it, vi } from 'vitest';
import { cleanupTrackedSessions, LiveSessionIdentity } from '../tests/helpers/liveSessionCleanup';

describe('live terminal session cleanup ledger', () => {
  it('retries failed deletion and retains the exact identity for final reconciliation', async () => {
    const sessions: LiveSessionIdentity[] = [{ name: 'smoke-generated', unixUser: 'alice' }];
    const deleteSession = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 503, body: 'busy' })
      .mockRejectedValueOnce(new Error('socket unavailable'));
    const wait = vi.fn().mockResolvedValue(undefined);

    const failures = await cleanupTrackedSessions(sessions, deleteSession, 2, wait);

    expect(deleteSession).toHaveBeenCalledTimes(2);
    expect(wait).toHaveBeenCalledTimes(1);
    expect(sessions).toEqual([{ name: 'smoke-generated', unixUser: 'alice' }]);
    expect(failures).toEqual([
      'alice/smoke-generated: attempt 2: Error: socket unavailable',
    ]);
  });

  it('removes only identities whose deletion was confirmed', async () => {
    const sessions: LiveSessionIdentity[] = [
      { name: 'deleted', unixUser: 'alice' },
      { name: 'still-live', unixUser: 'alice' },
    ];
    const deleteSession = vi.fn(async (session: LiveSessionIdentity) => ({
      ok: session.name === 'deleted',
      status: session.name === 'deleted' ? 200 : 500,
      body: session.name === 'deleted' ? '' : 'denied',
    }));

    const failures = await cleanupTrackedSessions(sessions, deleteSession, 1);

    expect(sessions).toEqual([{ name: 'still-live', unixUser: 'alice' }]);
    expect(failures).toEqual(['alice/still-live: attempt 1: HTTP 500 denied']);
  });

  it('pins retry masking off and final reconciliation on for the live smoke', () => {
    const spec = readFileSync(resolve(process.cwd(), 'tests/integration/terminal-sizing.spec.ts'), 'utf8');
    expect(spec).toContain('test.describe.configure({ retries: 0 });');

    const afterEach = spec.match(/test\.afterEach\([\s\S]*?\n {2}\}\);/)?.[0];
    expect(afterEach).toContain('cleanupTrackedSessions(request, 2)');
    expect(afterEach).toContain("expect(failures, 'every live sizing smoke session must be deleted");

    const afterAll = spec.match(/test\.afterAll\([\s\S]*?\n {2}\}\);/)?.[0];
    expect(afterAll).toContain('cleanupTrackedSessions(request, 3)');
    expect(afterAll).toContain("expect(failures, 'final live sizing reconciliation");
  });

  it('pins response-derived exact cleanup for the iframe-pool live smoke', () => {
    const spec = readFileSync(resolve(process.cwd(), 'tests/integration/iframe-pool.spec.ts'), 'utf8');
    expect(spec).toContain('test.describe.configure({ retries: 0 });');
    expect(spec).toContain("response.request().postDataJSON()");
    expect(spec).toContain("createdSessions.push({ name: payload.session!, unixUser: requestPayload?.unixUser });");
    expect(spec).toContain("const query = session.unixUser ? `?unixUser=${encodeURIComponent(session.unixUser)}` : '';");
    expect(spec).toContain("request.delete(`/api/tmux/sessions/${encodeURIComponent(session.name)}${query}`)");
    expect(spec).toContain('ok: response.ok()');
    expect(spec).toContain('status: response.status()');
    expect(spec).not.toContain("page.on('request'");

    const afterEach = spec.match(/test\.afterEach\([\s\S]*?\n {2}\}\);/)?.[0];
    expect(afterEach).toContain('cleanupTrackedSessions(request, 2)');
    expect(afterEach).toContain("expect(failures, 'every iframe-pool smoke session must be deleted");

    const afterAll = spec.match(/test\.afterAll\([\s\S]*?\n {2}\}\);/)?.[0];
    expect(afterAll).toContain('cleanupTrackedSessions(request, 3)');
    expect(afterAll).toContain("expect(failures, 'final iframe-pool reconciliation");
  });
});
