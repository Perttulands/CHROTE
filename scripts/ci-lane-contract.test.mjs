import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

import './release-admission.test.mjs'
import './release-candidate-sequence.test.mjs'
import './release-contract.test.mjs'

const repoRoot = fileURLToPath(new URL('../', import.meta.url))

test('built-server wrapper owns its environment, roots, socket, and port', () => {
  const wrapper = fs.readFileSync(`${repoRoot}/scripts/test-built-server-contract.sh`, 'utf8')

  assert.match(wrapper, /mkdir -p[\s\S]*"\$artifact_root\/runtime"/)
  assert.match(wrapper, /mkdir -p[\s\S]*"\$artifact_root\/tmp"/)
  assert.match(wrapper, /\nenv -i \\\n/)
  assert.match(wrapper, /CHROTE_DEFAULT_TMUX_SOCKET="\$artifact_root\/tmux\/default"/)
  assert.match(wrapper, /port="\$\(python3 -c '[^']*s\.bind\(\("127\.0\.0\.1", 0\)\)[^']*'\)"/)
  assert.match(wrapper, /-port "\$port"/)
})
