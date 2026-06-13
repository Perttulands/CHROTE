#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dashboard_dir="$repo_root/dashboard"
embed_dir="$repo_root/src/internal/dashboard/dist"

cd "$dashboard_dir"
if [ ! -d node_modules ]; then
  npm ci
fi
npm run build

rm -rf "$embed_dir"
mkdir -p "$(dirname "$embed_dir")"
cp -R "$dashboard_dir/dist" "$embed_dir"
find "$embed_dir" -type d -empty -delete

echo "Embedded dashboard written to $embed_dir"
