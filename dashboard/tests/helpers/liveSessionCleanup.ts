export type LiveSessionIdentity = { name: string; unixUser?: string };

export type DeleteSessionResult = {
  ok: boolean;
  status: number;
  body: string;
};

export async function cleanupTrackedSessions(
  sessions: LiveSessionIdentity[],
  deleteSession: (session: LiveSessionIdentity) => Promise<DeleteSessionResult>,
  attempts: number,
  wait: () => Promise<void> = () => new Promise(resolve => setTimeout(resolve, 100)),
): Promise<string[]> {
  const survivors: LiveSessionIdentity[] = [];
  const failures: string[] = [];
  for (const session of sessions) {
    const identity = `${session.unixUser || '<default>'}/${session.name}`;
    let cleaned = false;
    let detail = 'cleanup was not attempted';
    for (let attempt = 1; attempt <= attempts; attempt += 1) {
      try {
        const result = await deleteSession(session);
        if (result.ok) {
          cleaned = true;
          break;
        }
        detail = `attempt ${attempt}: HTTP ${result.status} ${result.body}`;
      } catch (error) {
        detail = `attempt ${attempt}: ${String(error)}`;
      }
      if (attempt < attempts) await wait();
    }
    if (!cleaned) {
      survivors.push(session);
      failures.push(`${identity}: ${detail}`);
    }
  }
  sessions.splice(0, sessions.length, ...survivors);
  return failures;
}
