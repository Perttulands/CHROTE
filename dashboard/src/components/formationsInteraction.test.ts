import { describe, expect, it, vi } from 'vitest'
import { createFormationsInteractionOwner } from './formationsInteraction'
import type { FormationsInteractionKind, InteractionPointer } from './formationsInteraction'

const kinds: FormationsInteractionKind[] = ['pan', 'node', 'wire', 'gate', 'lane', 'staff']
const pointer = (pointerId: number): InteractionPointer => ({ pointerId, clientX: 120, clientY: 80, target: null })

describe('Formations interaction owner', () => {
  it.each(kinds)('cancels %s projection without finalizing it', kind => {
    const project = vi.fn()
    const finalize = vi.fn()
    const cancel = vi.fn()
    const owner = createFormationsInteractionOwner()

    owner.begin({ kind, pointerId: 7, project, finalize, cancel })
    expect(owner.projection()).toEqual({ kind, pointerId: 7 })
    expect(owner.project(pointer(7))).toBe(true)

    expect(owner.cancel(7)).toBe(true)
    expect(owner.projection()).toBeNull()
    expect(project).toHaveBeenCalledOnce()
    expect(cancel).toHaveBeenCalledOnce()
    expect(finalize).not.toHaveBeenCalled()

    expect(owner.cancel(7)).toBe(false)
    expect(cancel).toHaveBeenCalledOnce()
  })

  it('cancels the previous projection when a new gesture takes ownership', () => {
    const firstCancel = vi.fn()
    const owner = createFormationsInteractionOwner()

    owner.begin({ kind: 'pan', pointerId: 1, project: vi.fn(), finalize: vi.fn(), cancel: firstCancel })
    owner.begin({ kind: 'node', pointerId: 2, project: vi.fn(), finalize: vi.fn(), cancel: vi.fn() })

    expect(firstCancel).toHaveBeenCalledOnce()
    expect(owner.projection()).toEqual({ kind: 'node', pointerId: 2 })
  })

  it('clears projection before the shared settlement path finalizes', () => {
    const owner = createFormationsInteractionOwner()
    const cancel = vi.fn(() => expect(owner.projection()).toBeNull())
    const finalize = vi.fn(() => expect(owner.projection()).toBeNull())

    owner.begin({ kind: 'wire', pointerId: 3, project: vi.fn(), finalize, cancel })

    expect(owner.finalize(pointer(3))).toBe(true)
    expect(cancel).toHaveBeenCalledOnce()
    expect(finalize).toHaveBeenCalledOnce()
    expect(owner.cancel()).toBe(false)
  })

  it('ignores events from a pointer that does not own the gesture', () => {
    const project = vi.fn()
    const finalize = vi.fn()
    const cancel = vi.fn()
    const owner = createFormationsInteractionOwner()

    owner.begin({ kind: 'staff', pointerId: 9, project, finalize, cancel })

    expect(owner.project(pointer(4))).toBe(false)
    expect(owner.finalize(pointer(4))).toBe(false)
    expect(owner.cancel(4)).toBe(false)
    expect(owner.projection()).toEqual({ kind: 'staff', pointerId: 9 })
    expect(project).not.toHaveBeenCalled()
    expect(finalize).not.toHaveBeenCalled()
    expect(cancel).not.toHaveBeenCalled()
  })
})
