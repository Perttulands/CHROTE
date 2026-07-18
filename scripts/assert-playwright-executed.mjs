#!/usr/bin/env node

import fs from 'node:fs'
import { pathToFileURL } from 'node:url'

const outcomeNames = ['expected', 'skipped', 'unexpected', 'flaky']

export function assertPlaywrightExecuted(report) {
  const stats = report?.stats
  if (!stats || typeof stats !== 'object') {
    throw new Error('Playwright report is missing stats')
  }

  const counts = Object.fromEntries(outcomeNames.map((name) => {
    const value = stats[name]
    if (!Number.isInteger(value) || value < 0) {
      throw new Error(`Playwright report has invalid stats.${name}`)
    }
    return [name, value]
  }))
  const discovered = outcomeNames.reduce((total, name) => total + counts[name], 0)
  if (discovered === 0) {
    throw new Error('Playwright discovered zero tests')
  }

  const executed = counts.expected + counts.unexpected + counts.flaky
  if (executed === 0) {
    throw new Error(`Playwright executed zero tests; all ${counts.skipped} discovered tests were skipped`)
  }

  return { discovered, executed, skipped: counts.skipped }
}

function main(argv) {
  if (argv.length !== 1) {
    throw new Error('usage: assert-playwright-executed.mjs <report.json>')
  }
  const report = JSON.parse(fs.readFileSync(argv[0], 'utf8'))
  const counts = assertPlaywrightExecuted(report)
  console.log(`Playwright execution verified: ${counts.executed}/${counts.discovered} tests executed (${counts.skipped} skipped)`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    main(process.argv.slice(2))
  } catch (error) {
    console.error(error instanceof Error ? error.message : error)
    process.exitCode = 1
  }
}
