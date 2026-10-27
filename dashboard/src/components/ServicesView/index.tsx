import { useCallback, useEffect, useRef, useState } from 'react'
import ResidentColumn from '../ResidentColumn'
import Toggle from '../kit/Toggle'
import './services.css'

type State = 'running' | 'stopped' | 'starting' | 'stopping' | 'partial' | 'failed' | 'unknown'
interface Service { id: string; name: string; description: string; link: string; state: State; controllable: boolean }
interface Response { services: Service[] }

async function readResponse(response: globalThis.Response) {
  const text = await response.text()
  try { return JSON.parse(text) as { error?: string; errors?: string[]; services?: Service[] } }
  catch { if (!response.ok) throw new Error(text || 'Service controller is unavailable'); throw new Error('Invalid service response') }
}

function safeLink(value: string) {
  try { const url = new URL(value); return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : '' }
  catch { return '' }
}

export default function ServicesView({ active }: { active: boolean }) {
  const [services, setServices] = useState<Service[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const reading = useRef(false)
  const acting = useRef(false)
  const generation = useRef(0)
  const actionError = useRef(false)
  const read = useCallback(async () => {
    if (reading.current || acting.current) return
    reading.current = true
    const started = generation.current
    try {
      const response = await fetch('/api/services', { signal: AbortSignal.timeout(30000) })
      const body = await readResponse(response)
      if (!response.ok) throw new Error(body.error || 'Could not read services')
      if (started === generation.current) {
        setServices((body as Response).services)
        if (!actionError.current) setError('')
      }
    } catch (cause) { if (started === generation.current && !actionError.current) setError(cause instanceof Error ? cause.message : 'Could not read services') }
    finally {
      reading.current = false
      if (started !== generation.current && !acting.current) void read()
    }
  }, [])
  useEffect(() => {
    if (!active) return
    void read()
    const timer = window.setInterval(() => void read(), 10000)
    return () => window.clearInterval(timer)
  }, [active, read])

  async function act(service: Service, action: 'start' | 'stop') {
    if (acting.current) return
    acting.current = true
    generation.current += 1
    setBusy(service.id)
    actionError.current = false
    setError('')
    try {
      const response = await fetch(`/api/services/${encodeURIComponent(service.id)}/${action}`, { method: 'POST', headers: { 'X-Requested-With': 'chrote' }, signal: AbortSignal.timeout(90000) })
      const body = await readResponse(response)
      if (!response.ok) throw new Error(body.errors?.join('; ') || body.error || 'Could not change service')
      setError('')
    } catch (cause) { actionError.current = true; setError(cause instanceof Error ? cause.message : 'Could not change service') }
    finally { acting.current = false; setBusy(null); void read() }
  }

  return (
    <div className="services-view" data-ui="services.view">
      <main className="services-content">
        <header className="services-heading"><div><h1>Services</h1><p>What is running on this host</p></div><button className="kit-button" onClick={() => void read()}>Refresh</button></header>
        {error && <p className="services-error" role="alert">{error}</p>}
        {services === null ? (!error && <p>Reading services…</p>) : services.length === 0 ? <p>No services configured.</p> : (
          <div className="services-table-wrap"><table className="services-table"><thead><tr><th scope="col">Service</th><th scope="col">Status</th><th scope="col">Running</th><th scope="col">Open</th></tr></thead><tbody>
            {services.map(service => {
              const link = safeLink(service.link)
              const transitioning = service.state === 'starting' || service.state === 'stopping'
              return <tr key={service.id}><td><strong>{service.name}</strong><p>{service.description}</p></td><td><span className={`services-state state-${service.state}`}>{busy === service.id ? 'working…' : service.state}</span>{(service.state === 'partial' || service.state === 'failed' || service.state === 'unknown') && service.controllable && <button className="kit-button" disabled={busy !== null} onClick={() => void act(service, 'stop')}>Stop all</button>}</td><td>{service.controllable ? <Toggle label={`${service.name} running`} checked={service.state === 'running'} disabled={busy !== null || transitioning} onChange={checked => void act(service, checked ? 'start' : 'stop')} /> : <span className="services-readonly">Status only</span>}</td><td>{link && <a href={link} target="_blank" rel="noreferrer">Open</a>}</td></tr>
            })}
          </tbody></table></div>
        )}
      </main>
      <ResidentColumn active={active} tab="services" reference={null} />
    </div>
  )
}
