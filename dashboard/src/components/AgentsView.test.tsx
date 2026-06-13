import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AgentsView from './AgentsView'

describe('AgentsView', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders the persona roster and inspects source pointers without source contents', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            agents: [
              {
                id: 'susie',
                displayName: 'Susie',
                kind: 'specialist',
                tags: ['design', 'react', 'taste:visual'],
                liveness: 'offline',
                assignable: true,
              },
            ],
            count: 1,
          },
        }))
      }
      if (url === '/api/agents/susie') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            id: 'susie',
            displayName: 'Susie',
            kind: 'specialist',
            tags: ['design', 'react'],
            harnessDefault: 'claude-code',
            harnessVariants: [
              { id: 'claude-code', sessionStem: 'susie', source: '/tmp/CLAUDE.md' },
            ],
          },
        }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    expect(await screen.findByText('Susie')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Susie/ }))

    expect(await screen.findByText((_, element) => Boolean(
      element?.classList.contains('oracle-output-line') &&
      (element.textContent?.includes('/tmp/CLAUDE.md') ?? false),
    ))).toBeInTheDocument()
    expect(screen.queryByText('CLAUDE.md contents')).not.toBeInTheDocument()
  })

  it('creates an agent through the API and refreshes the roster', async () => {
    const postedBodies: string[] = []
    fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/agents' && init?.method === 'POST') {
        postedBodies.push(String(init.body))
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            id: 'writer',
            kind: 'specialist',
            tags: ['writing', 'voice'],
            harnessDefault: 'claude-code',
            harnessVariants: [{ id: 'claude-code', sessionStem: 'writer' }],
          },
        }, 201))
      }
      if (url === '/api/agents') {
        return Promise.resolve(jsonResponse({
          success: true,
          data: {
            agents: postedBodies.length
              ? [{ id: 'writer', kind: 'specialist', tags: ['writing', 'voice'], liveness: 'offline', assignable: true }]
              : [],
            count: postedBodies.length,
          },
        }))
      }
      return Promise.reject(new Error(`unexpected fetch ${url}`))
    })

    render(<AgentsView />)

    fireEvent.change(screen.getByLabelText('Agent id'), { target: { value: 'writer' } })
    fireEvent.change(screen.getByLabelText('Capabilities'), { target: { value: 'writing, voice' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(postedBodies).toHaveLength(1)
    })
    expect(JSON.parse(postedBodies[0])).toMatchObject({
      id: 'writer',
      kind: 'specialist',
      harness: 'claude-code',
      capabilities: ['writing', 'voice'],
    })
    await waitFor(() => {
      expect(screen.getAllByText('writer').length).toBeGreaterThan(0)
    })
  })
})

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}
