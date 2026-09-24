/** Share only work already in progress. A later read always reaches its owner. */
const pending = new Map<string, Promise<unknown>>()

export function shareInFlight<T>(key: string, read: () => Promise<T>): Promise<T> {
  const existing = pending.get(key)
  if (existing) return existing as Promise<T>
  const request = read()
  pending.set(key, request)
  const settled = () => { if (pending.get(key) === request) pending.delete(key) }
  void request.then(settled, settled)
  return request
}
