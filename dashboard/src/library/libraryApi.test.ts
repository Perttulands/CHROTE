import { describe, expect, it } from 'vitest'
import { libraryProse, libraryWhen, shelfOf } from './libraryApi'

describe('libraryWhen', () => {
  const now = Date.parse('2026-09-03T12:00:00Z')
  const ago = (ms: number) => new Date(now - ms).toISOString()

  const cases: { name: string; iso: string; want: string }[] = [
    { name: 'a moment is just now', iso: ago(10_000), want: 'just now' },
    { name: 'one minute is singular', iso: ago(60_000), want: '1 minute ago' },
    { name: 'minutes below an hour', iso: ago(45 * 60_000), want: '45 minutes ago' },
    { name: 'one hour is singular', iso: ago(3_600_000), want: '1 hour ago' },
    { name: 'hours below a day', iso: ago(19 * 3_600_000), want: '19 hours ago' },
    { name: 'days below a month', iso: ago(2 * 86_400_000), want: '2 days ago' },
    { name: 'months below a year', iso: ago(120 * 86_400_000), want: '4 months ago' },
    { name: 'years beyond that', iso: ago(800 * 86_400_000), want: '2 years ago' },
    { name: 'a page git never mentioned', iso: '', want: 'never' },
    { name: 'something that is not a date', iso: 'soon', want: 'never' },
  ]

  cases.forEach(({ name, iso, want }) => {
    it(name, () => {
      expect(libraryWhen(iso, now)).toBe(want)
    })
  })
})

describe('shelfOf', () => {
  it('is the first segment of a page path', () => {
    expect(shelfOf('preferences/workflow.md')).toBe('preferences')
  })

  it('reaches past a nested directory', () => {
    expect(shelfOf('knowledge/notes/deep.md')).toBe('knowledge')
  })

  it('is nothing for a page at the root', () => {
    expect(shelfOf('README.md')).toBe('')
  })
})

describe('libraryProse', () => {
  it('drops the opening heading the running head already carries', () => {
    expect(libraryProse('# Workflow Preferences\n\nPrefer small changes.\n', 'Workflow Preferences'))
      .toBe('Prefer small changes.\n')
  })

  it('keeps a heading that is not the page\'s own title', () => {
    const content = '# Something else\n\nBody.\n'
    expect(libraryProse(content, 'Workflow Preferences')).toBe(content)
  })

  it('keeps a deeper heading, which is part of the prose', () => {
    const content = '## A section\n\nBody.\n'
    expect(libraryProse(content, 'A section')).toBe(content)
  })

  it('leaves a page with no heading alone', () => {
    const content = 'Just prose.\n'
    expect(libraryProse(content, 'tools')).toBe(content)
  })
})
