import type { ReactNode } from 'react'
import { act, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

vi.mock('./App', () => ({
  default: () => <div>CHROTE dashboard mounted</div>,
}))

vi.mock('./context/ToastContext', () => ({
  ToastProvider: ({ children }: { children: ReactNode }) => children,
}))

afterEach(() => {
  vi.restoreAllMocks()
  document.body.innerHTML = ''
})

it('mounts CHROTE directly without checking or requesting an access token', async () => {
  document.body.innerHTML = '<div id="root"></div>'
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network unavailable'))

  await act(async () => {
    await import('./main')
  })

  expect(await screen.findByText('CHROTE dashboard mounted')).toBeInTheDocument()
  expect(fetchMock).not.toHaveBeenCalled()
})
