import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Markdown, { isFileLink, resolveMarkdownPath } from './Markdown'

describe('resolveMarkdownPath', () => {
  it.each([
    ['/srv/chrote/docs/journeys.md', '../DESIGN-SYSTEM.md', '/srv/chrote/DESIGN-SYSTEM.md'],
    ['/srv/chrote/docs/journeys.md', 'nested/page.md', '/srv/chrote/docs/nested/page.md'],
    ['/srv/chrote/docs/journeys.md', './same.md', '/srv/chrote/docs/same.md'],
    ['/srv/chrote/docs/journeys.md', '/srv/other/absolute.md', '/srv/other/absolute.md'],
    ['/srv/chrote/docs/journeys.md', '../../escape.md', '/srv/escape.md'],
    ['/srv/chrote/docs/journeys.md', '../../../../../../etc/passwd', '/etc/passwd'],
    ['/srv/chrote/docs/journeys.md', 'anchored.md#section', '/srv/chrote/docs/anchored.md'],
  ])('resolves %s + %s', (base, href, expected) => {
    expect(resolveMarkdownPath(base, href)).toBe(expected)
  })
})

describe('isFileLink', () => {
  it.each([
    ['../PRD.md', true],
    ['/srv/chrote/PRD.md', true],
    ['https://example.com', false],
    ['mailto:someone@example.com', false],
    ['#section', false],
    ['', false],
  ])('reads %s', (href, expected) => {
    expect(isFileLink(href)).toBe(expected)
  })
})

describe('Markdown', () => {
  it('renders headings, code, tables and lists in the theme', () => {
    render(
      <Markdown
        content={[
          '# Title',
          '',
          'Some `inline` text and **strong** words.',
          '',
          '| Head | Other |',
          '| --- | --- |',
          '| one | two |',
          '',
          '- first',
          '- second',
          '',
          '```go',
          'func main() {}',
          '```',
        ].join('\n')}
      />,
    )

    expect(screen.getByRole('heading', { name: 'Title' })).toBeInTheDocument()
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Head' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'one' })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('func main() {}')).toBeInTheDocument()
    expect(screen.getByText('inline').tagName).toBe('CODE')
  })

  it('opens a link to another file in the panel rather than the browser', () => {
    const openPath = vi.fn()
    render(<Markdown content="See [the system](../DESIGN-SYSTEM.md)." basePath="/srv/chrote/docs/journeys.md" onOpenPath={openPath} />)

    const link = screen.getByRole('link', { name: 'the system' })
    expect(link).toHaveAttribute('href', '/srv/chrote/DESIGN-SYSTEM.md')
    fireEvent.click(link)
    expect(openPath).toHaveBeenCalledWith('/srv/chrote/DESIGN-SYSTEM.md')
  })

  it('leaves an external link external and draws an unfollowable one as text', () => {
    render(<Markdown content={'[out](https://example.com) and [bad](javascript:alert(1))'} />)

    const external = screen.getByRole('link', { name: 'out' })
    expect(external).toHaveAttribute('target', '_blank')
    expect(external).toHaveAttribute('rel', expect.stringContaining('noopener'))
    expect(screen.queryByRole('link', { name: 'bad' })).not.toBeInTheDocument()
    expect(screen.getByText('bad')).toBeInTheDocument()
  })

  it('reads an image beside the document off the host', () => {
    render(<Markdown content="![shot](shots/panel.png)" basePath="/srv/chrote/docs/journeys.md" />)

    expect(screen.getByRole('img', { name: 'shot' }))
      .toHaveAttribute('src', '/api/files/raw/srv/chrote/docs/shots/panel.png?inline=false')
  })
})

describe('Markdown tokens', () => {
  it('turns bare tokens into controls the host handles', () => {
    const onToken = vi.fn()
    render(<Markdown content="Blocked by chrote-5grx.13 today" tokenPattern={/chrote-[a-z0-9]{3,6}(?:\.\d+)*/g} onToken={onToken} />)
    fireEvent.click(screen.getByRole('button', { name: 'chrote-5grx.13' }))
    expect(onToken).toHaveBeenCalledWith('chrote-5grx.13')
  })

  it('leaves a token inside code as text', () => {
    const onToken = vi.fn()
    render(<Markdown content="run `bd show chrote-5grx.13`" tokenPattern={/chrote-[a-z0-9]{3,6}(?:\.\d+)*/g} onToken={onToken} />)
    expect(screen.queryByRole('button')).toBeNull()
  })
})
