import { describe, expect, it } from 'vitest'
import { addFlag, flagNames, flagValue, hasFlag, removeFlag, tokenize } from './launchFlags'
import type { LaunchFlag } from './launchFlags'

const CONTINUE: LaunchFlag = { name: '--continue', short: '-c', description: 'Continue the most recent conversation' }
const VERBOSE: LaunchFlag = { name: '--verbose', description: 'Override verbose mode' }
const MODEL: LaunchFlag = { name: '--model', short: '-m', value: '<model>', description: 'Model', values: ['a', 'b'] }
const ADD_DIR: LaunchFlag = { name: '--add-dir', value: '<directories...>', description: 'More directories' }

describe('tokenize', () => {
  it('splits on whitespace and takes the quotes off', () => {
    expect(tokenize('--model sonnet --verbose')).toEqual(['--model', 'sonnet', '--verbose'])
    expect(tokenize('  --model   sonnet  ')).toEqual(['--model', 'sonnet'])
    expect(tokenize('')).toEqual([])
    expect(tokenize('   ')).toEqual([])
  })

  it('lets a quote group whitespace, either kind', () => {
    expect(tokenize('--append-system-prompt "be brief" -c')).toEqual(['--append-system-prompt', 'be brief', '-c'])
    expect(tokenize("--add-dir '/two words' --verbose")).toEqual(['--add-dir', '/two words', '--verbose'])
    expect(tokenize('--name ""')).toEqual(['--name', ''])
    expect(tokenize('--path a" "b')).toEqual(['--path', 'a b'])
  })
})

describe('hasFlag', () => {
  it('answers to the long form, the short form and --name=value', () => {
    expect(hasFlag('--verbose', VERBOSE)).toBe(true)
    expect(hasFlag('--continue --verbose', CONTINUE)).toBe(true)
    expect(hasFlag('-c --verbose', CONTINUE)).toBe(true)
    expect(hasFlag('--model=sonnet', MODEL)).toBe(true)
    expect(hasFlag('--model sonnet', MODEL)).toBe(true)
  })

  it('does not answer to a longer name that starts the same', () => {
    expect(hasFlag('--verbose-output', VERBOSE)).toBe(false)
    expect(hasFlag('--continued', CONTINUE)).toBe(false)
    expect(hasFlag('', VERBOSE)).toBe(false)
    expect(hasFlag('sonnet', MODEL)).toBe(false)
  })
})

describe('addFlag', () => {
  it('appends a boolean flag, keeping the line and single spacing', () => {
    expect(addFlag('', VERBOSE)).toBe('--verbose')
    expect(addFlag('--dangerously-skip-permissions', VERBOSE)).toBe('--dangerously-skip-permissions --verbose')
    expect(addFlag('  -c   --verbose  ', CONTINUE)).toBe('-c --verbose --continue')
  })

  it('appends a value flag with its value, quoted only when it has whitespace', () => {
    expect(addFlag('', MODEL, 'sonnet')).toBe('--model sonnet')
    expect(addFlag('--verbose', ADD_DIR, '/srv/two words')).toBe('--verbose --add-dir "/srv/two words"')
    expect(addFlag('--verbose', MODEL)).toBe('--verbose --model')
    expect(addFlag('--verbose', MODEL, '')).toBe('--verbose --model')
  })

  it('leaves an already quoted token quoted', () => {
    expect(addFlag('--add-dir "/two words"', VERBOSE)).toBe('--add-dir "/two words" --verbose')
  })
})

describe('removeFlag', () => {
  it('takes a boolean flag off and leaves the rest', () => {
    expect(removeFlag('--verbose', VERBOSE)).toBe('')
    expect(removeFlag('-c --verbose --continue', VERBOSE)).toBe('-c --continue')
    expect(removeFlag('--verbose -c', CONTINUE)).toBe('--verbose')
  })

  it('takes a value flag off with its value, in either spelling', () => {
    expect(removeFlag('--model sonnet --verbose', MODEL)).toBe('--verbose')
    expect(removeFlag('--verbose -m sonnet', MODEL)).toBe('--verbose')
    expect(removeFlag('--verbose --model=sonnet -c', MODEL)).toBe('--verbose -c')
    expect(removeFlag('--verbose --add-dir "/two words"', ADD_DIR)).toBe('--verbose')
  })

  it('leaves a line that never had the flag alone, bar the spacing', () => {
    expect(removeFlag('--verbose  -c', MODEL)).toBe('--verbose -c')
    expect(removeFlag('', MODEL)).toBe('')
  })

  it('survives a value flag written last with nothing after it', () => {
    expect(removeFlag('--verbose --model', MODEL)).toBe('--verbose')
  })
})

describe('reading a flag back off the line', () => {
  it('says the names a row shows', () => {
    expect(flagNames(MODEL)).toBe('-m, --model')
    expect(flagNames(VERBOSE)).toBe('--verbose')
  })

  it('says the value the line carries, in either spelling', () => {
    expect(flagValue('--model sonnet', MODEL)).toBe('sonnet')
    expect(flagValue('--model=opus', MODEL)).toBe('opus')
    expect(flagValue('--add-dir "/two words"', ADD_DIR)).toBe('/two words')
    expect(flagValue('--verbose', MODEL)).toBe('')
    expect(flagValue('--model', MODEL)).toBe('')
  })
})
