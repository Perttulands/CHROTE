import assert from 'node:assert/strict'
import { execFile as execFileCallback } from 'node:child_process'
import { chmod, mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { promisify } from 'node:util'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

const execFile = promisify(execFileCallback)
const scriptPath = new URL('./build-release-candidates.sh', import.meta.url)
const releaseCommit = '0123456789abcdef0123456789abcdef01234567'

test('candidate caller builds both files in a clean disposable checkout before moving them', async () => {
  const checkout = await mkdtemp(join(tmpdir(), 'chrote-release-candidates-checkout-'))
  const bin = await mkdtemp(join(tmpdir(), 'chrote-release-candidates-tools-'))
  const runnerTemp = await mkdtemp(join(tmpdir(), 'chrote-release-candidates-runner-'))
  await mkdir(join(checkout, 'scripts'))
  await mkdir(join(checkout, 'src'))
  await writeFile(join(checkout, 'src/go.mod'), 'module example.invalid/chrote\n\ngo 1.26.5\n')
  await writeFile(join(checkout, 'scripts/build-release-candidates.sh'), await readFile(scriptPath))
  await chmod(join(checkout, 'scripts/build-release-candidates.sh'), 0o755)

  await execFile('/usr/bin/git', ['init', '--quiet', checkout])
  await execFile('/usr/bin/git', ['-C', checkout, 'config', 'user.email', 'release-test@example.invalid'])
  await execFile('/usr/bin/git', ['-C', checkout, 'config', 'user.name', 'Release Test'])
  await execFile('/usr/bin/git', ['-C', checkout, 'add', '.'])
  await execFile('/usr/bin/git', ['-C', checkout, 'commit', '--quiet', '-m', 'fixture'])

  await writeFile(join(bin, 'go'), `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  test) exit 0 ;;
  build)
    output=""
    while [ "$#" -gt 0 ]; do
      if [ "$1" = '-o' ]; then output="$2"; shift 2; else shift; fi
    done
    printf 'candidate-%s\\n' "$GOARCH" >"$output"
    chmod +x "$output"
    ;;
  version)
    printf 'fixture: build\\n'
    printf '\\tbuild\\tvcs.revision=%s\\n' "$RELEASE_COMMIT"
    printf '\\tbuild\\tvcs.modified=false\\n'
    ;;
  *) echo "unexpected fake go command: $*" >&2; exit 1 ;;
esac
`)
  await writeFile(join(bin, 'git'), '#!/usr/bin/env bash\nexec /usr/bin/git "$@"\n')
  await chmod(join(bin, 'go'), 0o755)
  await chmod(join(bin, 'git'), 0o755)

  const { stdout } = await execFile('bash', [join(checkout, 'scripts/build-release-candidates.sh')], {
    cwd: checkout,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      RELEASE_VERSION: '2.0.0',
      RELEASE_COMMIT: releaseCommit,
      RUNNER_TEMP: runnerTemp,
    },
  })

  assert.match(stdout, /Release candidates moved from .* to .*\/dist/)
  assert.equal(await readFile(join(checkout, 'dist/chrote-server-linux-amd64'), 'utf8'), 'candidate-amd64\n')
  assert.equal(await readFile(join(checkout, 'dist/chrote-server-linux-arm64'), 'utf8'), 'candidate-arm64\n')
})
