import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import FlagPanel from './FlagPanel'
import type { LaunchFlag } from './launchFlags'

const flags: LaunchFlag[] = [
  { name: '--continue', short: '-c', description: 'Continue the most recent conversation' },
  { name: '--verbose', description: 'Override verbose mode setting from config' },
  { name: '--model', short: '-m', value: '<model>', description: 'Model for the session', values: ['sonnet', 'opus'] },
  { name: '--add-dir', value: '<directories...>', description: 'Additional directories to allow tool access to' },
]

function panel(line: string, onChange = vi.fn(), onClose = vi.fn()) {
  render(
    <FlagPanel harnessLabel="Claude Code" flags={flags} line={line} onChange={onChange} onClose={onClose} />,
  )
  return { onChange, onClose }
}

describe('FlagPanel', () => {
  it('names the harness and lists its whole catalogue', () => {
    panel('')

    expect(screen.getByText('Flags for Claude Code')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /^-|^--/ })).toHaveLength(4)
    expect(screen.getByLabelText('Search flags')).toHaveFocus()
  })

  it('filters by name, short form and description, ignoring case', () => {
    panel('')
    const search = screen.getByLabelText('Search flags')

    fireEvent.change(search, { target: { value: 'MODEL' } })
    expect(screen.getAllByRole('button', { name: /--/ })).toHaveLength(1)
    expect(screen.getByRole('button', { name: /--model/ })).toBeInTheDocument()

    fireEvent.change(search, { target: { value: 'directories' } })
    expect(screen.getByRole('button', { name: /--add-dir/ })).toBeInTheDocument()

    fireEvent.change(search, { target: { value: '-c' } })
    expect(screen.getByRole('button', { name: /--continue/ })).toBeInTheDocument()

    fireEvent.change(search, { target: { value: 'nothing here' } })
    expect(screen.getByText('No flag matches')).toBeInTheDocument()
  })

  it('says which flags the line already carries, and takes one off when clicked', () => {
    const { onChange } = panel('--verbose --model opus')

    expect(screen.getByRole('button', { name: /--verbose/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: /--continue/ })).toHaveAttribute('aria-pressed', 'false')
    // A selected value flag shows the value on the line, not the placeholder.
    expect(screen.getByRole('button', { name: /--model opus/ })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /--verbose/ }))
    expect(onChange).toHaveBeenCalledWith('--model opus')
  })

  it('puts a boolean flag on the line with one click', () => {
    const { onChange } = panel('--dangerously-skip-permissions')

    fireEvent.click(screen.getByRole('button', { name: /--continue/ }))
    expect(onChange).toHaveBeenCalledWith('--dangerously-skip-permissions --continue')
  })

  it('asks a value flag for its value before adding it, and takes Enter for it', () => {
    const { onChange } = panel('')

    fireEvent.click(screen.getByRole('button', { name: /--add-dir/ }))
    const field = screen.getByLabelText('Value for --add-dir')
    expect(field).toHaveAttribute('placeholder', '<directories...>')
    expect(field).toHaveFocus()
    expect(onChange).not.toHaveBeenCalled()

    fireEvent.change(field, { target: { value: '/srv/two words' } })
    fireEvent.keyDown(field, { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('--add-dir "/srv/two words"')
  })

  it('offers the possible values as a choice, added by the Add control', () => {
    const { onChange } = panel('--verbose')

    fireEvent.click(screen.getByRole('button', { name: /--model/ }))
    const choice = screen.getByLabelText('Value for --model')
    expect(choice).toHaveValue('sonnet')

    fireEvent.change(choice, { target: { value: 'opus' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    expect(onChange).toHaveBeenCalledWith('--verbose --model opus')
  })

  it('takes a value flag off with its value when its row is clicked again', () => {
    const { onChange } = panel('--verbose -m opus --continue')

    fireEvent.click(screen.getByRole('button', { name: /--model/ }))
    expect(onChange).toHaveBeenCalledWith('--verbose --continue')
    expect(screen.queryByLabelText('Value for --model')).not.toBeInTheDocument()
  })

  it('closes on the close control and on Escape', () => {
    const { onClose } = panel('')

    fireEvent.click(screen.getByLabelText('Close flags'))
    expect(onClose).toHaveBeenCalledTimes(1)

    fireEvent.keyDown(screen.getByLabelText('Search flags'), { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('says plainly when the host has read no flags for this harness', () => {
    render(<FlagPanel harnessLabel="Codex" flags={[]} line="" onChange={vi.fn()} onClose={vi.fn()} />)

    expect(screen.getByText('No flags read from this harness yet')).toBeInTheDocument()
  })
})
