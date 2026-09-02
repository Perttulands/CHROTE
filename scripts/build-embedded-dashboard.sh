#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dashboard_dir="$repo_root/dashboard"
embed_dir="$repo_root/src/internal/dashboard/dist"

# Vite copies everything under dashboard/public into dist verbatim, so whatever
# sits there ends up inside the server binary. The build stamp fingerprints
# TRACKED dashboard files only (scripts/check-embedded-dashboard.py), so an
# untracked file here changes the served artifact without changing the recorded
# source fingerprint: the bundle and the stamp both look right while the binary
# ships something no one declared. That is how ~40MB of host media once rode
# along in a 74MB binary. Refuse to build instead of copying an input nothing
# records; git's index is the declaration, so staging a new asset is enough.
undeclared="$(git -C "$repo_root" ls-files --others -- dashboard/public)"
if [ -n "$undeclared" ]; then
  echo 'Refusing to build: dashboard/public holds files git does not track,' >&2
  echo 'and Vite would copy them into the embedded bundle unrecorded:' >&2
  printf '%s\n' "$undeclared" | sed 's/^/  /' >&2
  echo 'Stage them to make them part of the product, or move them out of the repository.' >&2
  exit 1
fi

cd "$dashboard_dir"
if [ ! -d node_modules ]; then
  npm ci
fi
npm run build

rm -rf "$embed_dir"
mkdir -p "$(dirname "$embed_dir")"
cp -R "$dashboard_dir/dist" "$embed_dir"
find "$embed_dir" -type d -empty -delete

# Record WHICH dashboard source this bundle came from. go:embed fails loudly on a
# missing bundle but silently accepts a stale one, and both dist directories are
# gitignored, so nothing else can tell. Without this stamp an 8-day-old UI built
# from an abandoned branch shipped unnoticed on 2026-07-24 and a committed change
# looked lost. scripts/check-embedded-dashboard.py verifies it.
#
# Kept beside dist/, not inside it: anything inside is embedded and served.
stamp="$repo_root/src/internal/dashboard/dist.stamp"
{
  printf '# written by scripts/build-embedded-dashboard.sh -- do not edit or commit\n'
  printf 'source_sha256=%s\n' \
    "$(cd "$repo_root" && python3 scripts/check-embedded-dashboard.py --print-fingerprint)"
  printf 'commit=%s\n' "$(cd "$repo_root" && git rev-parse HEAD 2>/dev/null || printf unknown)"
  printf 'built_at=%s\n' "$(date --iso-8601=seconds)"
} >"$stamp"

echo "Embedded dashboard written to $embed_dir"
echo "Build stamp written to $stamp"
