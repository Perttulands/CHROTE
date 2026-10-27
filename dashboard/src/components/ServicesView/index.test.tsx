import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import ServicesView from './index'
import { readServices, resetServicesForTest } from './servicesApi'

const mockState = vi.hoisted(() => ({ announce: vi.fn(), openSendToSession: vi.fn() }))
vi.mock('../../context/SessionContext', () => ({ useSession: () => ({ openSendToSession: mockState.openSendToSession }) }))
vi.mock('../../context/StatusContext', () => ({ useStatus: () => ({ announce: mockState.announce }) }))
vi.mock('../TableHost', () => ({ TableSlot: () => null }))
vi.mock('../ResidentColumn', () => ({ default: ({ reference }: { reference: string | null }) => <div data-testid="resident">{reference ?? 'no reference'}</div> }))

const entry = { id: 'example', name: 'Example', description: 'An example service', link: 'https://example.test/', state: 'stopped', controllable: true }
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })

beforeEach(() => resetServicesForTest())
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks() })

it('starts a service with its Start button and says so until a read agrees', async () => {
  let finishStart: (response: Response) => void = () => {}
  const fetcher = vi.fn()
    .mockResolvedValueOnce(json({ services: [entry] }))
    .mockReturnValueOnce(new Promise<Response>(resolve => { finishStart = resolve }))
    .mockResolvedValueOnce(json({ services: [{ ...entry, state: 'running' }] }))
  vi.stubGlobal('fetch', fetcher)
  render(<ServicesView active />)
  const row = await screen.findByRole('listitem', { name: /Example/ })
  expect(within(row).getByRole('link', { name: 'Open' })).toHaveAttribute('href', 'https://example.test/')
  fireEvent.click(within(row).getByRole('button', { name: /Start/ }))
  expect(fetcher).toHaveBeenCalledWith('/api/services/example/start', expect.objectContaining({ method: 'POST' }))
  expect(within(row).getByText('Starting…')).toBeInTheDocument()
  expect(within(row).queryByRole('button')).toBeNull()
  finishStart(json({ service: { ...entry, state: 'running' }, errors: [] }))
  expect(await within(row).findByRole('button', { name: /Stop/ })).toBeInTheDocument()
  expect(within(row).getByText('Running')).toBeInTheDocument()
  expect(mockState.announce).toHaveBeenCalledWith('Example started', 'success')
})

it('keeps a failed action on its row in the controller\'s words after the next read', async () => {
  const fetcher = vi.fn()
    .mockResolvedValueOnce(json({ services: [entry] }))
    .mockResolvedValueOnce(json({ errors: ['Could not start'] }, 502))
    .mockResolvedValue(json({ services: [entry] }))
  vi.stubGlobal('fetch', fetcher)
  render(<ServicesView active />)
  const row = await screen.findByRole('listitem', { name: /Example/ })
  fireEvent.click(within(row).getByRole('button', { name: /Start/ }))
  expect(await within(row).findByRole('alert')).toHaveTextContent('Could not start')
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(3))
  expect(within(row).getByRole('alert')).toHaveTextContent('Could not start')
  expect(within(row).getByRole('button', { name: /Start/ })).toBeInTheDocument()
})

it('draws at once from a read the pointer asked for before the tab opened', async () => {
  const fetcher = vi.fn().mockResolvedValue(json({ services: [entry, { ...entry, id: 'watched', name: 'Watched', state: 'running', controllable: false }] }))
  vi.stubGlobal('fetch', fetcher)
  await readServices()
  render(<ServicesView active />)
  expect(screen.getByRole('listitem', { name: /Example/ })).toBeInTheDocument()
  expect(screen.queryByText('Asking the host…')).toBeNull()
  const watched = screen.getByRole('listitem', { name: /Watched/ })
  expect(within(watched).getByText('Read-only')).toBeInTheDocument()
  expect(within(watched).queryByRole('button')).toBeNull()
})

it('hands the chosen service to the resident and acts on it with Enter', async () => {
  const fetcher = vi.fn()
    .mockResolvedValueOnce(json({ services: [entry] }))
    .mockResolvedValueOnce(json({ service: { ...entry, state: 'running' }, errors: [] }))
    .mockResolvedValue(json({ services: [{ ...entry, state: 'running' }] }))
  vi.stubGlobal('fetch', fetcher)
  render(<ServicesView active />)
  const row = await screen.findByRole('listitem', { name: /Example/ })
  expect(screen.getByTestId('resident')).toHaveTextContent('no reference')
  fireEvent.click(row)
  expect(screen.getByTestId('resident')).toHaveTextContent('service example (Example, stopped)')
  fireEvent.keyDown(row, { key: 'Enter' })
  await waitFor(() => expect(fetcher).toHaveBeenCalledWith('/api/services/example/start', expect.anything()))
  await waitFor(() => expect(screen.getByTestId('resident')).toHaveTextContent('service example (Example, running)'))
})

it('says what failed when the controller cannot be reached', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('Service controller is not configured', { status: 503 })))
  render(<ServicesView active />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Service controller is not configured')
})
