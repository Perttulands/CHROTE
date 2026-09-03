#!/usr/bin/env node
/**
 * Which file each React component lives in.
 *
 * Dev mode names the component under the pointer by reading the function's own
 * name off the React fiber, and the operator then wants the file so an agent
 * can open it. The name is all the running program has: nothing in a bundle
 * remembers where a function was written. So the mapping is made here, over the
 * source, and shipped as data.
 *
 * The map is written into the dashboard source tree rather than into the build
 * output because dev mode reads it the same way in development, in the unit
 * tests and in the production bundle: one lookup, one shape, no branch. It is
 * committed for the same reason — `npm run dev` and `vitest` must not depend on
 * a build step having run first — and the dashboard build rewrites it, so a
 * component that moved shows up as a diff instead of as a wrong file path.
 *
 * A name declared in two files is dropped rather than guessed: dev mode can say
 * a component's name with no file beside it, but it must not name the wrong
 * file.
 */

import { readdirSync, readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const sourceRoot = join(repoRoot, 'dashboard', 'src')
const outputPath = join(sourceRoot, 'generated', 'componentMap.json')

/** A component is a capitalised declaration; everything else is a helper. */
const DECLARATIONS = [
  /^\s*export\s+default\s+function\s+([A-Z][A-Za-z0-9_]*)\s*[(<]/,
  /^\s*export\s+function\s+([A-Z][A-Za-z0-9_]*)\s*[(<]/,
  /^\s*function\s+([A-Z][A-Za-z0-9_]*)\s*[(<]/,
  /^\s*(?:export\s+)?const\s+([A-Z][A-Za-z0-9_]*)\s*(?::\s*[^=]+)?=\s*(?:\(|memo\(|forwardRef\(|function\b)/,
]

function sourceFiles(directory) {
  const found = []
  for (const entry of readdirSync(directory, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === 'generated' || entry.name === 'test') continue
      found.push(...sourceFiles(path))
      continue
    }
    if (!entry.name.endsWith('.tsx')) continue
    if (entry.name.includes('.test.')) continue
    found.push(path)
  }
  return found
}

function componentsIn(path) {
  const names = new Set()
  for (const line of readFileSync(path, 'utf8').split('\n')) {
    for (const pattern of DECLARATIONS) {
      const match = pattern.exec(line)
      if (match) names.add(match[1])
    }
  }
  return names
}

const owners = new Map()
for (const path of sourceFiles(sourceRoot)) {
  const file = relative(repoRoot, path).split('\\').join('/')
  for (const name of componentsIn(path)) {
    const already = owners.get(name)
    owners.set(name, already === undefined || already === file ? file : null)
  }
}

const map = {}
for (const name of [...owners.keys()].sort()) {
  const file = owners.get(name)
  if (file !== null) map[name] = file
}

mkdirSync(dirname(outputPath), { recursive: true })
writeFileSync(outputPath, `${JSON.stringify(map, null, 2)}\n`)
process.stdout.write(`${Object.keys(map).length} components mapped into ${relative(repoRoot, outputPath)}\n`)
