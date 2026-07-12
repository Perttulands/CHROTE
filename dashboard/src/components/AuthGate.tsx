import { FormEvent, ReactNode, useEffect, useState } from 'react'

type AuthGateProps = {
  children: ReactNode
}

type AuthState = 'checking' | 'locked' | 'authorized'

type AuthStatus = {
  required?: boolean
  authenticated?: boolean
}

export default function AuthGate({ children }: AuthGateProps) {
  const [state, setState] = useState<AuthState>('checking')
  const [token, setToken] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const response = await fetch('/auth/session', { credentials: 'same-origin' })
        if (cancelled) return
        if (response.status === 401) {
          setState('locked')
          return
        }
        const contentType = response.headers.get('Content-Type') ?? ''
        if (response.ok && contentType.includes('application/json')) {
          const status = await response.json() as AuthStatus
          setState(status.required && !status.authenticated ? 'locked' : 'authorized')
          return
        }
        // The Vite-only development server serves index.html for this route.
        // Production servers always return JSON from the registered endpoint.
        setState('authorized')
      } catch {
        if (!cancelled) {
          setError('Unable to verify the secure session')
          setState('locked')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const unlock = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      const response = await fetch('/auth/session', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token }),
      })
      if (!response.ok) {
        setError(response.status === 403 ? 'Invalid access token' : 'Unable to establish a secure session')
        return
      }
      setToken('')
      setState('authorized')
    } catch {
      setError('Unable to reach CHROTE')
    } finally {
      setSubmitting(false)
    }
  }

  if (state === 'authorized') return children

  return (
    <main className="auth-gate">
      <section className="auth-card" aria-busy={state === 'checking'}>
        <div className="auth-mark">CHROTE</div>
        {state === 'checking' ? (
          <p>Checking secure session…</p>
        ) : (
          <form onSubmit={unlock}>
            <h1>Unlock CHROTE</h1>
            <p>Enter the service access token. It is exchanged for a secure HttpOnly session and is not stored by the dashboard.</p>
            <label htmlFor="chrote-access-token">CHROTE access token</label>
            <input
              id="chrote-access-token"
              type="password"
              value={token}
              onChange={event => setToken(event.target.value)}
              autoComplete="current-password"
              autoFocus
              required
            />
            {error && <p className="auth-error" role="alert">{error}</p>}
            <button type="submit" disabled={submitting || token.length === 0}>
              {submitting ? 'Unlocking…' : 'Unlock CHROTE'}
            </button>
          </form>
        )}
      </section>
    </main>
  )
}
