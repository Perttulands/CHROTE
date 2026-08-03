#!/usr/bin/env bash
# Dirty-tree snapshot safety net (bead chrote-biu, guardrail chrote-7dh/A).
#
# If the shared checkout has uncommitted work — tracked or untracked,
# .gitignore respected — preserve it as a commit on refs/snapshots/<UTC-ts>.
# Uses a throwaway index file, so it NEVER touches the working tree, the real
# index, HEAD, or any agent's in-flight state. Skips when the tree is clean,
# when nothing changed since the latest snapshot, and when the dirty flags
# produce a tree identical to HEAD's.
#
# Recovery:  git for-each-ref refs/snapshots
#            git show <ref> --stat
#            git checkout <ref> -- <path>
# Pruning (manual, snapshots are cheap):
#            git update-ref -d refs/snapshots/<ts>
# Snapshots are local refs and are never pushed.
set -euo pipefail

REPO=${SNAPSHOT_REPO:-/srv/chrote}
cd "$REPO"

if [ -z "$(git status --porcelain)" ]; then
  exit 0
fi

TMP_INDEX=$(mktemp)
trap 'rm -f "$TMP_INDEX"' EXIT

export GIT_INDEX_FILE="$TMP_INDEX"
git read-tree HEAD
git add -A
TREE=$(git write-tree)
unset GIT_INDEX_FILE

if [ "$(git rev-parse 'HEAD^{tree}')" = "$TREE" ]; then
  exit 0
fi

LAST=$(git for-each-ref --sort=-refname --format='%(refname)' refs/snapshots | head -1)
if [ -n "$LAST" ] && [ "$(git rev-parse "$LAST^{tree}")" = "$TREE" ]; then
  exit 0
fi

TS=$(date -u +%Y%m%dT%H%M%SZ)
COMMIT=$(git commit-tree "$TREE" -p HEAD -m "snapshot: uncommitted work in shared tree at $TS")
git update-ref "refs/snapshots/$TS" "$COMMIT"
echo "snapshot: refs/snapshots/$TS ($COMMIT)"
