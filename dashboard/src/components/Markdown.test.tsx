import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Markdown from './Markdown'

describe('Markdown', () => {
  it('renders headings, lists and GitHub tables', () => {
    render(<Markdown text={'# Title\n\n- one\n- two\n\n| a | b |\n| - | - |\n| 1 | 2 |\n'} />)
    expect(screen.getByRole('heading', { name: 'Title' })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByRole('table')).toBeInTheDocument()
  })

  it('turns bare tokens into controls the host handles', () => {
    const onToken = vi.fn()
    render(<Markdown text="Blocked by chrote-5grx.13 today" tokenPattern={/chrote-[a-z0-9]{3,6}(?:\.\d+)*/g} onToken={onToken} />)
    fireEvent.click(screen.getByRole('button', { name: 'chrote-5grx.13' }))
    expect(onToken).toHaveBeenCalledWith('chrote-5grx.13')
  })

  it('leaves a token inside code as text', () => {
    const onToken = vi.fn()
    render(<Markdown text="run `bd show chrote-5grx.13`" tokenPattern={/chrote-[a-z0-9]{3,6}(?:\.\d+)*/g} onToken={onToken} />)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('keeps an ordinary link a link, and sends it away from the app', () => {
    render(<Markdown text="[docs](https://example.test/docs)" />)
    const link = screen.getByRole('link', { name: 'docs' })
    expect(link).toHaveAttribute('href', 'https://example.test/docs')
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('refuses a link whose scheme could run something', () => {
    render(<Markdown text="[bad](javascript:alert(1))" />)
    expect(screen.getByText('bad').closest('a')).not.toHaveAttribute('href', 'javascript:alert(1)')
  })
})
