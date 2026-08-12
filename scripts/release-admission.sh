#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'Release admission failed: %s\n' "$1" >&2
  exit 1
}

: "${GITHUB_REF:?GITHUB_REF is required}"
: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"

case "$GITHUB_REF" in
  refs/tags/v*) ;;
  *) die "release ref is not a version tag: $GITHUB_REF" ;;
esac

command -v gh >/dev/null 2>&1 || die "gh is required"
command -v jq >/dev/null 2>&1 || die "jq is required"

release_commit="$(git rev-parse "${GITHUB_REF}^{commit}" 2>/dev/null)" \
  || die "tag ref does not resolve to a commit: $GITHUB_REF"
if [ "$release_commit" != "$GITHUB_SHA" ]; then
  die "tag target $release_commit does not equal event SHA $GITHUB_SHA"
fi

git fetch --no-tags origin main:refs/remotes/origin/main
main_commit="$(git rev-parse refs/remotes/origin/main)" \
  || die "origin/main does not resolve to a commit"
if ! git merge-base --is-ancestor "$release_commit" "$main_commit"; then
  die "tag target $release_commit is not an ancestor of origin/main $main_commit"
fi

check_runs="$(gh api --paginate --slurp \
  "repos/$GITHUB_REPOSITORY/commits/$release_commit/check-runs?per_page=100")" \
  || die "could not read check runs for $release_commit"

required_jobs=(quality formations-browser built-server-contract)
for job in "${required_jobs[@]}"; do
  if ! jq -e --arg job "$job" --arg sha "$release_commit" '
    [.[] | .check_runs[]? | select(
      .name == $job and
      .head_sha == $sha and
      .conclusion == "success"
    )] | length > 0
  ' >/dev/null <<<"$check_runs"; then
    die "CI job $job has no successful check run for $release_commit"
  fi
done

release_version="${GITHUB_REF_NAME#v}"
[ -n "$release_version" ] || die "version tag has no release version"

if [ -n "${GITHUB_ENV:-}" ]; then
  printf 'RELEASE_COMMIT=%s\nRELEASE_VERSION=%s\n' \
    "$release_commit" "$release_version" >>"$GITHUB_ENV"
fi
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  printf 'release_commit=%s\nrelease_version=%s\n' \
    "$release_commit" "$release_version" >>"$GITHUB_OUTPUT"
fi

printf 'Release admission passed: tag=%s commit=%s origin/main=%s\n' \
  "$GITHUB_REF_NAME" "$release_commit" "$main_commit"
printf 'Successful exact-SHA CI jobs: %s\n' "${required_jobs[*]}"
