import assert from 'node:assert/strict'
import fs from 'node:fs'
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
  assert.doesNotMatch(smoke, /kill-server/)
  assert.match(workflow, /sha256sum chrote-server-linux-amd64 chrote-server-linux-arm64 > SHA256SUMS/)
  assert.match(workflow, /dist\/chrote-server-linux-amd64\n\s+dist\/chrote-server-linux-arm64\n\s+dist\/SHA256SUMS/)
})

test('installer smoke reserves distinct ports until the server handoff', () => {
  const smoke = fs.readFileSync(`${repoRoot}/scripts/test-public-install.sh`, 'utf8')
  const reserve = smoke.indexOf('coproc port_reserver')
  const distinct = smoke.indexOf('len(set(ports)) != 2')
  const release = smoke.indexOf('\nrelease_port_reserver\n"$installed_binary" >')
  const serverStart = smoke.indexOf('"$installed_binary" >')

  assert.ok(reserve >= 0, 'smoke must own the port reservation helper')
  assert.ok(distinct > reserve, 'smoke must reject equal reserved ports')
  assert.ok(release > distinct, 'smoke must release reservations explicitly')
  assert.ok(serverStart > release, 'smoke must release ports immediately before server start')
  assert.doesNotMatch(smoke, /ports\.append\(sock\.getsockname\(\)\[1\]\)\s+sock\.close\(\)/)
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
