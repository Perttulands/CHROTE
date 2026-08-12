import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const repoRoot = fileURLToPath(new URL('../', import.meta.url))

test('release publication requires an admission gate before upload', () => {
  const workflow = fs.readFileSync(`${repoRoot}/.github/workflows/release.yml`, 'utf8')
  const admission = workflow.indexOf('scripts/release-admission.sh')
  const upload = workflow.indexOf('uses: softprops/action-gh-release@v2')

  assert.notEqual(admission, -1, 'release admission script is missing')
  assert.notEqual(upload, -1, 'release upload is missing')
  assert.ok(admission < upload, 'release admission must precede upload')
})

test('release admission pins the tag to exact green main CI', () => {
  const workflow = fs.readFileSync(`${repoRoot}/.github/workflows/release.yml`, 'utf8')
  const admission = fs.readFileSync(`${repoRoot}/scripts/release-admission.sh`, 'utf8')

  assert.match(workflow, /fetch-depth: 0/)
  assert.match(workflow, /checks: read/)
  assert.match(admission, /git fetch --no-tags origin main:refs\/remotes\/origin\/main/)
  assert.match(admission, /git merge-base --is-ancestor \"\$release_commit\" \"\$main_commit\"/)
  assert.match(admission, /check-runs\?per_page=100/)
  for (const job of ['quality', 'formations-browser', 'built-server-contract']) {
    assert.match(admission, new RegExp(job))
  }
  assert.match(admission, /\.head_sha == \$sha/)
  assert.match(admission, /\.conclusion == "success"/)
})

test('release checks and smokes the exact files it publishes', () => {
  const workflow = fs.readFileSync(`${repoRoot}/.github/workflows/release.yml`, 'utf8')
  const smoke = fs.readFileSync(`${repoRoot}/scripts/test-public-install.sh`, 'utf8')
  const candidates = fs.readFileSync(`${repoRoot}/scripts/build-release-candidates.sh`, 'utf8')
  const build = workflow.indexOf('name: Build publication candidates')
  const provenance = workflow.indexOf('name: Verify publication provenance')
  const scan = workflow.indexOf('name: Scan release binaries')
  const smokeStep = workflow.indexOf('name: Smoke release installer contract')
  const checksums = workflow.indexOf('name: Generate checksums')
  const upload = workflow.indexOf('uses: softprops/action-gh-release@v2')

  assert.ok(provenance < scan)
  assert.ok(scan < smokeStep)
  assert.ok(smokeStep < checksums)
  assert.ok(checksums < upload)
  assert.match(workflow, /run: bash scripts\/build-release-candidates\.sh/)
  assert.match(candidates, /GOOS=linux GOARCH=amd64 go build/)
  assert.match(candidates, /GOOS=linux GOARCH=arm64 go build/)
  assert.match(candidates, /runner_temp=.*RUNNER_TEMP/)
  assert.match(candidates, /go version -m \"\$binary\"/)
  assert.match(candidates, /vcs\.revision=\$release_commit/)
  assert.match(candidates, /vcs\.modified=false/)
  assert.match(workflow, /go version -m \"\$binary\"/)
  assert.match(workflow, /vcs\.revision=\$RELEASE_COMMIT/)
  assert.match(workflow, /vcs\.modified=false/)
  assert.ok(build < provenance)
  assert.match(workflow, /CHROTE_EXPECTED_BUILD_COMMIT=\"\$RELEASE_COMMIT\" .*chrote-server-linux-amd64/)
  assert.match(smoke, /payload\['commit'\]==sys\.argv\[3\]/)
  assert.match(smoke, /CHROTE_TEST_TMUX_BIN/)
  assert.match(smoke, /runtime_dir="\$\(mktemp -d \/tmp\/chrote-tmux\.XXXXXX\)"/)
  assert.match(smoke, /tmux_socket=.*runtime_dir/)
  assert.match(smoke, /export PATH="\$prefix\/bin:\$tmux_bin_dir:/)
  assert.match(smoke, /export CHROTE_TMUX_BIN="\$tmux_bin"/)
  assert.match(smoke, /export CHROTE_DEFAULT_TMUX_SOCKET="\$tmux_socket"/)
  assert.match(smoke, /tmux_cmd new-session/)
  assert.match(smoke, /kill-session -t =public-smoke/)
  assert.match(smoke, /tmux_cmd has-session -t =public-smoke/)
  assert.match(smoke, /tmux_cmd list-sessions -F '#\{session_name\}'/)
  assert.match(smoke, /no server running|No such file or directory|server exited unexpectedly/)
  assert.match(smoke, /can't find session: public-smoke/)
  assert.doesNotMatch(smoke, /has-session[^\n]*\n\s+.*\|\| true/)
  assert.doesNotMatch(smoke, /kill-session[^\n]*\|\| true/)
  assert.doesNotMatch(smoke, /list-sessions[^\n]*\|\| true/)
  assert.doesNotMatch(smoke, /kill-server/)
  assert.match(workflow, /sha256sum chrote-server-linux-amd64 chrote-server-linux-arm64 > SHA256SUMS/)
  assert.match(workflow, /dist\/chrote-server-linux-amd64\n\s+dist\/chrote-server-linux-arm64\n\s+dist\/SHA256SUMS/)
})

test('installer smoke reserves distinct ports until the server handoff', () => {
  const smoke = fs.readFileSync(`${repoRoot}/scripts/test-public-install.sh`, 'utf8')
  const reserve = smoke.indexOf('python3 - "$port_receipt" <<\'PY\' &')
  const distinct = smoke.indexOf('len(set(ports)) != 2')
  const receipt = smoke.indexOf('read -r port ttyd_port < "$port_receipt"')
  const term = smoke.indexOf('kill -TERM "$pid"')
  const wait = smoke.indexOf('wait "$pid"')
  const clear = smoke.lastIndexOf('port_reserver_pid=""')
  const release = smoke.indexOf('\nrelease_port_reserver\n"$installed_binary" >')
  const serverStart = smoke.indexOf('"$installed_binary" >')

  assert.ok(reserve >= 0, 'smoke must record the exact port reserver PID')
  assert.ok(distinct > reserve, 'smoke must reject equal reserved ports')
  assert.ok(receipt > distinct, 'smoke must wait for the task-owned port receipt')
  assert.ok(term >= 0, 'smoke must release only the exact port reserver PID')
  assert.ok(wait > term, 'smoke must wait for the exact port reserver PID')
  assert.ok(clear > wait, 'smoke must clear the exact port reserver PID after wait')
  assert.ok(release > distinct, 'smoke must release reservations explicitly')
  assert.ok(serverStart > release, 'smoke must release ports immediately before server start')
  assert.match(smoke, /signal\.signal\(signal\.SIGTERM, stop\)/)
  assert.match(smoke, /raise SystemExit\(0\)/)
  assert.doesNotMatch(smoke, /coproc\s+port_reserver/)
  assert.doesNotMatch(smoke, /port_reserver_(?:read|write)_fd/)
})

test('installer smoke bounds private tmux cleanup and rejects timeout outcomes', () => {
  const smoke = fs.readFileSync(`${repoRoot}/scripts/test-public-install.sh`, 'utf8')
  const tmuxCommand = smoke.slice(smoke.indexOf('tmux_cmd()'), smoke.indexOf('release_port_reserver()'))
  const timeoutConfig = smoke.slice(smoke.indexOf('tmux_timeout='), smoke.indexOf('tmux_cmd()'))
  const noServer = smoke.slice(smoke.indexOf('tmux_no_server_output()'), smoke.indexOf('tmux_session_absent_output()'))
  const cleanup = smoke.slice(smoke.indexOf('cleanup_tmux()'), smoke.indexOf('cleanup()'))

  assert.match(tmuxCommand, /mktemp "\$tmp\/tmux-command\.XXXXXX"/)
  assert.match(tmuxCommand, /timeout --kill-after=1s "\$tmux_timeout"/)
  assert.match(tmuxCommand, />"\$tmux_capture" 2>&1/)
  assert.match(tmuxCommand, /status=\$\?/)
  assert.ok(tmuxCommand.indexOf('cat "$tmux_capture"') < tmuxCommand.indexOf('rm -f "$tmux_capture"'))
  assert.ok(tmuxCommand.indexOf('rm -f "$tmux_capture"') < tmuxCommand.indexOf('return "$status"'))
  assert.doesNotMatch(tmuxCommand, /--foreground/)
  assert.match(timeoutConfig, /CHROTE_TEST_TMUX_TIMEOUT/)
  assert.match(timeoutConfig, /timeout --version/)
  assert.doesNotMatch(noServer, /timeout|124|137/i)
  assert.match(cleanup, /exact-session probe failed \(status %s\)/)
  assert.match(cleanup, /session listing failed \(status %s\)/)
})

test('installer smoke cleanup trap escalates cleanup failure and preserves body failure', () => {
  const smoke = fs.readFileSync(`${repoRoot}/scripts/test-public-install.sh`, 'utf8')
  const cleanupStart = smoke.indexOf('cleanup() {')
  const trapStart = smoke.indexOf('trap cleanup EXIT', cleanupStart)
  const cleanup = smoke.slice(cleanupStart, trapStart)
  const shellQuote = (value) => `'${value.replaceAll("'", "'\\''")}'`
  const runCleanup = ({ bodyStatus, releaseStatus, tmuxStatus }) => {
    const root = fs.mkdtempSync(join(tmpdir(), 'chrote-cleanup-contract-'))
    const harness = [
      'set -u',
      `tmp=${shellQuote(join(root, 'tmp'))}`,
      `runtime_dir=${shellQuote(join(root, 'runtime'))}`,
      'mkdir -p "$tmp" "$runtime_dir"',
      'server_pid=""',
      `release_port_reserver() { return ${releaseStatus}; }`,
      `cleanup_tmux() { return ${tmuxStatus}; }`,
      cleanup,
      'trap cleanup EXIT',
      `exit ${bodyStatus}`,
    ].join('\n')
    const result = spawnSync('bash', ['-c', harness], { encoding: 'utf8' })
    fs.rmSync(root, { recursive: true, force: true })
    return result
  }

  const cleanupFailure = runCleanup({ bodyStatus: 0, releaseStatus: 0, tmuxStatus: 1 })
  assert.equal(cleanupFailure.status, 1, cleanupFailure.stderr)

  const bodyFailure = runCleanup({ bodyStatus: 7, releaseStatus: 0, tmuxStatus: 0 })
  assert.equal(bodyFailure.status, 7, bodyFailure.stderr)
})

test('candidate build sequence verifies both outside-checkout binaries before moving them', () => {
  const candidates = fs.readFileSync(`${repoRoot}/scripts/build-release-candidates.sh`, 'utf8')
  const amd64 = candidates.indexOf('GOOS=linux GOARCH=amd64 go build')
  const arm64 = candidates.indexOf('GOOS=linux GOARCH=arm64 go build')
  const cleanCheck = candidates.indexOf('status --porcelain')
  const provenance = candidates.indexOf('for binary in')
  const move = candidates.indexOf('mv "$candidate_dir"')

  assert.ok(amd64 >= 0 && amd64 < arm64)
  assert.ok(arm64 < provenance)
  assert.ok(provenance < cleanCheck)
  assert.ok(cleanCheck < move)
  assert.match(candidates, /-o \"\$candidate_dir\/chrote-server-linux-amd64\"/)
  assert.match(candidates, /-o \"\$candidate_dir\/chrote-server-linux-arm64\"/)
  assert.doesNotMatch(candidates, /-o \"\.\.\/dist\//)
})
