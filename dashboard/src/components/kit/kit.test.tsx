import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import Choice from './Choice'
import Toggle from './Toggle'
import { choice, choiceValue, choose } from './testing'

describe('Toggle', () => {
  it('is a checkbox whose whole row, words and count included, toggles it', () => {
    const onChange = vi.fn()
    render(<Toggle checked={false} onChange={onChange} meta="12">Show finished</Toggle>)
    const toggle = screen.getByRole('checkbox', { name: /Show finished/ })
    expect(toggle).not.toBeChecked()
    fireEvent.click(screen.getByText('12'))
    expect(onChange).toHaveBeenCalledWith(true)
  })

  it('names a bare mark by its label and does nothing while disabled', () => {
    const onChange = vi.fn()
    render(<Toggle checked label="Keep chrote" onChange={onChange} disabled />)
    const toggle = screen.getByRole('checkbox', { name: 'Keep chrote' })
    expect(toggle).toBeChecked()
    fireEvent.click(toggle)
    expect(onChange).not.toHaveBeenCalled()
  })
})

describe('Choice', () => {
  function Unit() {
    const [unit, setUnit] = useState<'minutes' | 'hours'>('minutes')
    return <Choice label="Interval unit" value={unit} onChange={setUnit}
      options={[{ value: 'minutes', label: 'minutes' }, { value: 'hours', label: 'hours' }]} />
  }

  it('reads its choice in words and changes it through the menu', () => {
    render(<Unit />)
    expect(choiceValue('Interval unit')).toBe('minutes')
    fireEvent.click(choice('Interval unit'))
    expect(screen.getByRole('menu', { name: 'Interval unit' })).toBeInTheDocument()
    expect(screen.getAllByRole('menuitem').map(row => row.textContent)).toEqual([expect.stringMatching(/^minutes/), expect.stringMatching(/^hours/)])
    fireEvent.click(screen.getByRole('menuitem', { name: /^hours/ }))
    expect(choiceValue('Interval unit')).toBe('hours')
    expect(screen.queryByRole('menu')).toBeNull()
  })

  it('shows its placeholder while the value is none of the options', () => {
    render(<Choice label="Pane" placeholder="Pick the exact pane…" value="" onChange={vi.fn()}
      options={[{ value: '%1', label: 'claude' }]} />)
    expect(choiceValue('Pane')).toBe('Pick the exact pane…')
  })

  it('is driven by the shared test helper', () => {
    render(<Unit />)
    choose('Interval unit', 'hours')
    expect(choiceValue('Interval unit')).toBe('hours')
  })
})
