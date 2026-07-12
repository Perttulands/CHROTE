import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AuthGate from './AuthGate'

describe('AuthGate', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    sessionStorage.clear()
  })

  it('renders immediately when server authentication is not required', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ required: false, authenticated: true }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))

    render(<AuthGate><div>Dashboard</div></AuthGate>)

    expect(await screen.findByText('Dashboard')).toBeInTheDocument()
  })

  it('exchanges the access token for an HttpOnly browser session without persisting it', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ required: true, authenticated: false }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))

    render(<AuthGate><div>Dashboard</div></AuthGate>)
    const tokenInput = await screen.findByLabelText('CHROTE access token')
    fireEvent.change(tokenInput, { target: { value: 'owner-token' } })
    fireEvent.click(screen.getByRole('button', { name: 'Unlock CHROTE' }))

    expect(await screen.findByText('Dashboard')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenLastCalledWith('/auth/session', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ token: 'owner-token' }),
    }))
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
  })

  it('keeps the dashboard locked after a rejected token', async () => {
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ required: true, authenticated: false }), { status: 401 }))
      .mockResolvedValueOnce(new Response('Forbidden', { status: 403 }))

    render(<AuthGate><div>Dashboard</div></AuthGate>)
    fireEvent.change(await screen.findByLabelText('CHROTE access token'), { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: 'Unlock CHROTE' }))

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Invalid access token'))
    expect(screen.queryByText('Dashboard')).not.toBeInTheDocument()
  })
})
