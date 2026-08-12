import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import { assertPlaywrightExecuted } from './assert-playwright-executed.mjs'
import './release-admission.test.mjs'
import './release-candidate-sequence.test.mjs'
import './release-contract.test.mjs'

const repoRoot = fileURLToPath(new URL('../', import.meta.url))

test('Formations discovery rejects a zero-test Playwright report', () => {
  assert.throws(
    () => assertPlaywrightExecuted({
      stats: { expected: 0, skipped: 0, unexpected: 0, flaky: 0 },
    }),
    /discovered zero tests/,
  )
})

test('Formations discovery rejects an all-skipped Playwright report', () => {
  assert.throws(
    () => assertPlaywrightExecuted({
      stats: { expected: 0, skipped: 3, unexpected: 0, flaky: 0 },
    }),
    /executed zero tests/,
  )
})

test('Formations discovery accepts a report with an executed test', () => {
  assert.deepEqual(
    assertPlaywrightExecuted({
      stats: { expected: 2, skipped: 1, unexpected: 0, flaky: 1 },
    }),
    { discovered: 4, executed: 3, skipped: 1 },
  )
})

test('Formations CI records a JSON report and rejects an empty execution', () => {
  const workflow = fs.readFileSync(`${repoRoot}/.github/workflows/ci.yml`, 'utf8')

  assert.match(workflow, /run: node --test scripts\/ci-lane-contract\.test\.mjs/)
  assert.match(workflow, /PLAYWRIGHT_JSON_OUTPUT_FILE: \$\{\{ runner\.temp \}\}\/formations-playwright\.json/)
  assert.match(workflow, /npm run test:formations -- --reporter=line,json/)
  assert.match(workflow, /node \.\.\/scripts\/assert-playwright-executed\.mjs "\$PLAYWRIGHT_JSON_OUTPUT_FILE"/)
  assert.match(workflow, /\$\{\{ runner\.temp \}\}\/formations-playwright\.json/)
})

test('active environment template cannot advertise retired Gate process controls', () => {
  const envExample = fs.readFileSync(`${repoRoot}/.env.example`, 'utf8')
  const retiredControls = [
    'CHROTE_FORMATIONS_SCRIPT_GATES',
    'CHROTE_FORMATIONS_GATE_TIMEOUT_SECONDS',
    'CHROTE_FORMATIONS_GATE_OUTPUT_CAP_BYTES',
    'CHROTE_FORMATIONS_GATE_ENV_ALLOWLIST',
  ]

  for (const control of retiredControls) {
    assert.doesNotMatch(envExample, new RegExp(`^${control}=`, 'm'))
  }
  assert.doesNotMatch(envExample, /Formations script gates|Gate TOML should use commandArgv|commandShell is the explicit shell escape hatch|Gate commands receive a sanitized env/)
})

test('built-server wrapper owns its environment, roots, socket, and port', () => {
  const wrapper = fs.readFileSync(`${repoRoot}/scripts/test-built-server-contract.sh`, 'utf8')

  assert.match(wrapper, /mkdir -p[\s\S]*"\$artifact_root\/formations-data"/)
  assert.match(wrapper, /mkdir -p[\s\S]*"\$artifact_root\/formations-tmux"/)
  assert.match(wrapper, /mkdir -p[\s\S]*"\$artifact_root\/runtime"/)
  assert.match(wrapper, /mkdir -p[\s\S]*"\$artifact_root\/tmp"/)
  assert.match(wrapper, /\nenv -i \\\n/)
  assert.match(wrapper, /CHROTE_FORMATIONS_DATA_ROOT="\$artifact_root\/formations-data"/)
  assert.match(wrapper, /CHROTE_FORMATIONS_LAB_HARNESSES= \\/)
  assert.match(wrapper, /CHROTE_FORMATIONS_LAB_CWD="\$workspace"/)
  assert.match(wrapper, /CHROTE_FORMATIONS_LAB_ROOTS="\$workspace"/)
  assert.match(wrapper, /CHROTE_FORMATIONS_TMUX_HARNESSES= \\/)
  assert.match(wrapper, /CHROTE_FORMATIONS_TMUX_SOCKET="\$artifact_root\/formations-tmux\/default"/)
  assert.match(wrapper, /CHROTE_FORMATIONS_TMUX_CWD="\$workspace"/)
  assert.match(wrapper, /CHROTE_FORMATIONS_TMUX_ROOTS="\$workspace"/)
  assert.match(wrapper, /CHROTE_FORMATIONS_TMUX_DEDICATED= \\/)
  assert.match(wrapper, /CHROTE_FORMATIONS_TMUX_PROD_SMOKE= \\/)
  assert.doesNotMatch(wrapper, /CHROTE_FORMATIONS_SCRIPT_GATES/)
  assert.match(wrapper, /CHROTE_DEFAULT_TMUX_SOCKET="\$artifact_root\/tmux\/default"/)
  assert.match(wrapper, /port="\$\(python3 -c '[^']*s\.bind\(\("127\.0\.0\.1", 0\)\)[^']*'\)"/)
  assert.match(wrapper, /-port "\$port"/)
})
