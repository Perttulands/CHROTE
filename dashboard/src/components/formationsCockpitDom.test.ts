// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { connectionKind, isTextEditingTarget, laneYFrom, splitList } from './formationsCockpitDom'

describe('connectionKind', () => {
  it('classifies a connection by its judge/pass/fail source or target port', () => {
    expect(connectionKind({ id: 'e1', from: 'gate:judge', to: 'f:in' })).toBe('judge')
    expect(connectionKind({ id: 'e2', from: 'f:out', to: 'gate:judge' })).toBe('judge')
    expect(connectionKind({ id: 'e3', from: 'gate:pass', to: 'f:in' })).toBe('pass')
    expect(connectionKind({ id: 'e4', from: 'gate:fail', to: 'f:in' })).toBe('fail')
    expect(connectionKind({ id: 'e5', from: 'a:out', to: 'b:in' })).toBe('wire')
  })
})

describe('laneYFrom', () => {
  it('parses hand-routed y:<n> lanes and rejects legacy/absent lanes', () => {
    expect(laneYFrom('y:240')).toBe(240)
    expect(laneYFrom('y:-60')).toBe(-60)
    expect(laneYFrom('auto')).toBeNull()
    expect(laneYFrom('manual')).toBeNull()
    expect(laneYFrom(undefined)).toBeNull()
    expect(laneYFrom('y:nope')).toBeNull()
  })
})

describe('splitList', () => {
  it('splits on commas, trims, and drops empties', () => {
    expect(splitList('a, b ,, c ')).toEqual(['a', 'b', 'c'])
    expect(splitList('   ')).toEqual([])
  })
})

describe('isTextEditingTarget', () => {
  it('is true for inputs, textareas, and contenteditable; false otherwise', () => {
    expect(isTextEditingTarget(document.createElement('input'))).toBe(true)
    expect(isTextEditingTarget(document.createElement('textarea'))).toBe(true)
    const editable = document.createElement('div')
    Object.defineProperty(editable, 'isContentEditable', { value: true })
    expect(isTextEditingTarget(editable)).toBe(true)
    // jsdom reports isContentEditable as undefined for a plain span; the guard is
    // used in boolean conditions, so falsiness (not strict false) is the contract.
    expect(isTextEditingTarget(document.createElement('span'))).toBeFalsy()
    expect(isTextEditingTarget(null)).toBe(false)
  })
})
