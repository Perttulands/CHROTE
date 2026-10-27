/**
 * The host's service controller, as CHROTE's proxy serves it: the catalog with
 * each service's state, and start and stop. The last list read is kept, so
 * the tab draws at once from it, and a read the pointer asked for on its way
 * to the tab is the one the tab then waits on.
 */

export type ServiceState = 'running' | 'stopped' | 'starting' | 'stopping' | 'partial' | 'failed' | 'unknown'
export type ServiceAction = 'start' | 'stop'

export interface Service {
  id: string
  name: string
  description: string
  link: string
  state: ServiceState
  controllable: boolean
}

interface Answer { error?: string; errors?: string[]; services?: Service[]; service?: Service }

async function readAnswer(response: Response): Promise<Answer> {
  const text = await response.text()
  try { return JSON.parse(text) as Answer }
  catch {
    if (!response.ok) throw new Error(text.trim() || 'The service controller is unavailable')
    throw new Error('The service controller answered with something other than JSON')
  }
}

let lastList: Service[] | null = null
let reading: Promise<Service[]> | null = null

export function readCachedServices(): Service[] | null {
  return lastList
}

async function fetchList(): Promise<Service[]> {
  const response = await fetch('/api/services', { signal: AbortSignal.timeout(30000) })
  const answer = await readAnswer(response)
  if (!response.ok) throw new Error(answer.error || 'Could not read the services')
  lastList = answer.services ?? []
  return lastList
}

/**
 * The list as the controller reports it. A read already on its way is shared;
 * `fresh` waits it out and asks again, for a caller that must see a change
 * that landed after that read left.
 */
export function readServices(fresh = false): Promise<Service[]> {
  if (reading && !fresh) return reading
  const request = (fresh && reading ? reading.then(() => undefined, () => undefined) : Promise.resolve()).then(fetchList)
  reading = request
  const settled = () => { if (reading === request) reading = null }
  void request.then(settled, settled)
  return request
}

/** Start or stop one service; the controller's own words on failure. */
export async function actOnService(id: string, action: ServiceAction): Promise<Service | null> {
  const response = await fetch(`/api/services/${encodeURIComponent(id)}/${action}`, {
    method: 'POST',
    headers: { 'X-Requested-With': 'chrote' },
    signal: AbortSignal.timeout(90000),
  })
  const answer = await readAnswer(response)
  const failure = answer.errors?.filter(Boolean).join('; ') || answer.error
  if (!response.ok || failure) throw new Error(failure || `Could not ${action} the service`)
  return answer.service ?? null
}

export function resetServicesForTest(): void {
  lastList = null
  reading = null
}
