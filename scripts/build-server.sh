#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[ "$#" -eq 1 ] && [ -n "$1" ] || { echo "Usage: $0 <output-path>" >&2; exit 1; }
output="$1"
case "$output" in
  /*) ;;
  *) output="$PWD/$output" ;;
esac

command -v git >/dev/null || { echo 'git is required' >&2; exit 1; }
version="$(tr -d '\r\n' < "$repo_root/VERSION")"
[ -n "$version" ] || { echo 'VERSION is empty' >&2; exit 1; }
commit="$(git -C "$repo_root" rev-parse HEAD)"
[ -n "$commit" ] || { echo 'git HEAD is empty' >&2; exit 1; }

cd "$repo_root/src"
GOTOOLCHAIN=auto go build -trimpath -buildvcs=true \
  -ldflags "-X main.Version=$version -X main.BuildCommit=$commit" \
  -o "$output" ./cmd/server
printf 'Built CHROTE %s at commit %s\n' "$version" "$commit"
