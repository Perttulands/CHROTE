import assert from 'node:assert/strict'
import { chmod, mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawn } from 'node:child_process'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const script = fileURLToPath(new URL('./release-admission.sh', import.meta.url))
const releaseCommit = '0123456789abcdef0123456789abcdef01234567'
const mainCommit = 'fedcba9876543210fedcba9876543210fedcba98'

async function runAdmission({ ancestor, checks, main = mainCommit }) {
  const root = await mkdtemp(join(tmpdir(), 'chrote-release-admission-'))
  const bin = join(root, 'bin')
  await mkdir(bin)
  const fakeGit = join(bin, 'git')
  const fakeGh = join(bin, 'gh')
  const checksPath = join(root, 'checks.json')
  const envPath = join(root, 'github.env')

  await writeFile(checksPath, JSON.stringify([{ check_runs: checks }]))
  await writeFile(fakeGit, `#!/usr/bin/env bash
case "$1:$2" in
  rev-parse:refs/tags/v2.0.0^{commit}) printf '%s\\n' '${releaseCommit}' ;;
  rev-parse:refs/remotes/origin/main) printf '%s\\n' '${main}' ;;
  fetch:*) exit 0 ;;
  merge-base:*) exit ${ancestor ? 0 : 1} ;;
  *) exit 0 ;;
esac
`)
  await writeFile(fakeGh, '#!/usr/bin/env bash\ncat "$CHROTE_CHECKS_FIXTURE"\n')
  await chmod(fakeGit, 0o755)
  await chmod(fakeGh, 0o755)

  return new Promise((resolve, reject) => {
    const child = spawn('bash', [script], {
      env: {
        ...process.env,
        PATH: `${bin}:${process.env.PATH}`,
        GITHUB_REF: 'refs/tags/v2.0.0',
        GITHUB_REF_NAME: 'v2.0.0',
        GITHUB_REPOSITORY: 'example/chrote',
        GITHUB_SHA: releaseCommit,
        GITHUB_ENV: envPath,
        CHROTE_CHECKS_FIXTURE: checksPath,
      },
    })
    let stdout = ''
    let stderr = ''
    child.stdout.on('data', (chunk) => { stdout += chunk })
    child.stderr.on('data', (chunk) => { stderr += chunk })
    child.on('error', reject)
    child.on('close', async (status) => {
      resolve({ status, stdout, stderr, env: await readFile(envPath, 'utf8').catch(() => '') })
    })
  })
}

const successfulChecks = [
  'quality',
  'built-server-contract',
].map((name) => ({ name, head_sha: releaseCommit, conclusion: 'success' }))

test('release admission rejects a tag outside origin/main', async () => {
  const result = await runAdmission({ ancestor: false, checks: successfulChecks, main: releaseCommit })

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /not an ancestor of origin\/main/)
})

test('release admission rejects an older ancestor tag even when every exact-SHA job succeeded', async () => {
  const result = await runAdmission({ ancestor: true, checks: successfulChecks })

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /must equal origin\/main/)
})

test('release admission rejects an exact-main tag without every successful exact-SHA job', async () => {
  const result = await runAdmission({
    ancestor: true,
    checks: successfulChecks.filter(({ name }) => name !== 'built-server-contract'),
    main: releaseCommit,
  })

  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /CI job built-server-contract has no successful check run/)
})

test('release admission accepts an exact-main tag with all successful exact-SHA jobs', async () => {
  const result = await runAdmission({ ancestor: true, checks: successfulChecks, main: releaseCommit })

  assert.equal(result.status, 0, result.stderr)
  assert.match(result.stdout, /Release admission passed: tag=v2\.0\.0/)
  assert.match(result.env, new RegExp(`RELEASE_COMMIT=${releaseCommit}`))
  assert.match(result.env, /RELEASE_VERSION=2\.0\.0/)
})
