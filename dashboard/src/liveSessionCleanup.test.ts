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
      { name: 'still-live', unixUser: 'tavern' },
    ];
    const deleteSession = vi.fn(async (session: LiveSessionIdentity) => ({
      ok: session.name === 'deleted',
      status: session.name === 'deleted' ? 200 : 500,
      body: session.name === 'deleted' ? '' : 'denied',
    }));

    const failures = await cleanupTrackedSessions(sessions, deleteSession, 1);

    expect(sessions).toEqual([{ name: 'still-live', unixUser: 'tavern' }]);
    expect(failures).toEqual(['tavern/still-live: attempt 1: HTTP 500 denied']);
  });
});
