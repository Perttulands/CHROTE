import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import ServicesView from './index'
vi.mock('../ResidentColumn', () => ({ default: () => <div>Agent drawer</div> }))
const entry = { id: 'example', name: 'Example', description: 'An example service', link: 'https://example.test/', state: 'stopped', controllable: true }
afterEach(() => vi.unstubAllGlobals())
it('shows the configured service and starts it on request', async () => {
  const fetcher = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ services: [entry] })))
    .mockResolvedValueOnce(new Response(JSON.stringify({ service: { ...entry, state: 'running' }, errors: [] })))
    .mockResolvedValueOnce(new Response(JSON.stringify({ services: [{ ...entry, state: 'running' }] })))
  vi.stubGlobal('fetch', fetcher)
  render(<ServicesView active />)
  expect(await screen.findByText('An example service')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Open' })).toHaveAttribute('href', 'https://example.test/')
  fireEvent.click(screen.getByRole('checkbox', { name: 'Example running' }))
  await waitFor(() => expect(fetcher).toHaveBeenCalledWith('/api/services/example/start', expect.objectContaining({ method: 'POST' })))
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Example running' })).toHaveAttribute('aria-checked', 'true'))
})
it('keeps an action error visible after status refresh', async () => {
  const fetcher = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ services: [entry] })))
    .mockResolvedValueOnce(new Response(JSON.stringify({ errors: ['Could not start'] }), { status: 502 }))
    .mockResolvedValueOnce(new Response(JSON.stringify({ services: [entry] })))
  vi.stubGlobal('fetch', fetcher)
  render(<ServicesView active />)
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Example running' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not start')
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(3))
  expect(screen.getByRole('alert')).toHaveTextContent('Could not start')
})
