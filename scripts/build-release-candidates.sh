#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
release_version="${RELEASE_VERSION:?RELEASE_VERSION is required}"
release_commit="${RELEASE_COMMIT:?RELEASE_COMMIT is required}"
runner_temp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
candidate_dir="$(mktemp -d "$runner_temp/chrote-release-candidates.XXXXXX")"

cd "$repo_root/src"
go test ./...

GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=true \
  -ldflags "-X main.Version=$release_version -X main.BuildCommit=$release_commit" \
  -o "$candidate_dir/chrote-server-linux-amd64" ./cmd/server
GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=true \
  -ldflags "-X main.Version=$release_version -X main.BuildCommit=$release_commit" \
  -o "$candidate_dir/chrote-server-linux-arm64" ./cmd/server

for binary in "$candidate_dir"/chrote-server-linux-*; do
  metadata="$(go version -m "$binary")"
  printf '%s\n' "$metadata"
  grep -Fq "vcs.revision=$release_commit" <<<"$metadata"
  grep -Fq 'vcs.modified=false' <<<"$metadata"
done

if [ -n "$(git -C "$repo_root" status --porcelain)" ]; then
  git -C "$repo_root" status --short >&2
  exit 1
fi

mkdir -p "$repo_root/dist"
mv "$candidate_dir"/chrote-server-linux-amd64 \
  "$candidate_dir"/chrote-server-linux-arm64 \
  "$repo_root/dist/"

printf 'Release candidates moved from %s to %s/dist\n' "$candidate_dir" "$repo_root"
