#!/usr/bin/env bash
# CHROTE local deploy: build artifacts from the canonical source worktree and
# swap the runtime chrote-server binary, preserving the tmux socket/sessions.
#
# Model and rationale: docs/runtime-deploy-model.md
# Beads: home-gia (epic), home-a3l (this script), home-j7x (exclusions),
#        home-8qm (controlled deploy).
#
# Safety guarantees (fail loud; never force):
#   - Builds in the canonical worktree + a staging path; the runtime binary is
#     only replaced after a fully successful build, atomically, with a backup.
#   - NEVER runs any git operation on the runtime tree.
#   - NEVER runs tmux kill-* and NEVER removes the tmux socket dir.
#   - NEVER copies private config (services.env) into the repo.
#   - Restarts ONLY chrote.service (systemctl --user).
#   - Records rollback state (runtime git HEAD, service status, tmux sessions,
#     backed-up binary path) before restart.
#
# Usage:
#   scripts/deploy-local.sh [--dry-run] [--allow-dirty] [--skip-tests]
#
#   --dry-run     Build + checks only. Does not touch the runtime binary or
#                 restart the service. Prints planned actions.
#   --allow-dirty Permit deploying from a dirty canonical worktree (NOT default).
#   --skip-tests  Skip `go test ./...` (NOT recommended).

set -Eeuo pipefail

# ---- Fixed, verified runtime facts (see docs/runtime-deploy-model.md) --------
CANONICAL_DIR="/home/perttu/chrote-3.0-gascity"
RUNTIME_DIR="/home/perttu/chrote"
RUNTIME_BINARY="${RUNTIME_DIR}/chrote-server"
SERVICE="chrote.service"
HEALTH_URL="http://127.0.0.1:8094/api/health"
CHROTE_TMUX_TMPDIR="/run/user/1000/chrote-tmux"
GO_BIN="/usr/local/go/bin/go"
BACKUP_DIR="${RUNTIME_DIR}/.deploy-backups"

DRY_RUN=0
ALLOW_DIRTY=0
SKIP_TESTS=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --allow-dirty) ALLOW_DIRTY=1 ;;
    --skip-tests) SKIP_TESTS=1 ;;
    -h|--help) sed -n '1,40p' "$0"; exit 0 ;;
    *) echo "ERROR: unknown argument: $arg" >&2; exit 2 ;;
  esac
done

log()  { printf '\n=== %s ===\n' "$*"; }
info() { printf '    %s\n' "$*"; }
die()  { printf '\nDEPLOY ABORTED: %s\n' "$*" >&2; exit 1; }

command -v "$GO_BIN" >/dev/null 2>&1 || GO_BIN="$(command -v go || true)"
[ -n "$GO_BIN" ] || die "Go toolchain not found"
command -v npm >/dev/null 2>&1 || die "npm not found"

# ---- 1. Verify canonical source -------------------------------------------
log "1. Verify canonical source"
[ -d "$CANONICAL_DIR/src" ] || die "canonical src not found at $CANONICAL_DIR/src"
[ -f "$CANONICAL_DIR/src/internal/dashboard/embed.go" ] || die "embed.go missing (wrong tree?)"
cd "$CANONICAL_DIR"
SRC_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
SRC_HEAD="$(git rev-parse HEAD)"
info "canonical: $CANONICAL_DIR @ $SRC_BRANCH ($SRC_HEAD)"

if [ -n "$(git status --porcelain)" ]; then
  if [ "$ALLOW_DIRTY" -eq 1 ]; then
    info "WARNING: canonical worktree is dirty; proceeding due to --allow-dirty"
  else
    git status --short
    die "canonical worktree is dirty. Commit/stash, or pass --allow-dirty."
  fi
fi

# ---- 2. Go tests ------------------------------------------------------------
log "2. Go tests"
if [ "$SKIP_TESTS" -eq 1 ]; then
  info "SKIPPED (--skip-tests)"
else
  ( cd "$CANONICAL_DIR/src" && "$GO_BIN" test ./... ) || die "go test failed"
fi

# ---- 3. Dashboard build + refresh embedded dist -----------------------------
log "3. Build dashboard and refresh embedded dist"
( cd "$CANONICAL_DIR/dashboard" && npm run build ) || die "dashboard build failed"
[ -d "$CANONICAL_DIR/dashboard/dist" ] || die "dashboard/dist missing after build"
rm -rf "$CANONICAL_DIR/src/internal/dashboard/dist"
cp -r "$CANONICAL_DIR/dashboard/dist" "$CANONICAL_DIR/src/internal/dashboard/dist"
info "embedded dist refreshed from dashboard/dist"

# ---- 4. Build binary into staging ------------------------------------------
# Provenance (home-altx): the canonical tree is a git worktree whose .git is a
# file, so Go's automatic VCS stamp walks up to the OUTER /home/perttu repo and
# records the wrong commit with modified=true. We disable that misleading stamp
# (-buildvcs=false) and stamp the real CHROTE source commit explicitly via
# -ldflags, surfaced at /api/version and in the startup log.
log "4. Build chrote-server (staging)"
SRC_VERSION="0.2.0"
SRC_COMMIT_FULL="$SRC_HEAD"
SRC_COMMIT_SHORT="$(git rev-parse --short HEAD)"
LDFLAGS="-X main.Version=${SRC_VERSION} -X main.Commit=${SRC_COMMIT_SHORT}"
STAGE_BIN="$(mktemp "${TMPDIR:-/tmp}/chrote-server.stage.XXXXXX")"
( cd "$CANONICAL_DIR/src" && "$GO_BIN" build -buildvcs=false -ldflags "$LDFLAGS" -o "$STAGE_BIN" ./cmd/server ) \
  || { rm -f "$STAGE_BIN"; die "go build failed"; }
chmod +x "$STAGE_BIN"
STAGE_SIZE="$(stat -c %s "$STAGE_BIN")"
info "staged binary: $STAGE_BIN ($STAGE_SIZE bytes)"
info "stamped provenance: version=$SRC_VERSION commit=$SRC_COMMIT_SHORT (source $SRC_COMMIT_FULL)"
"$STAGE_BIN" --help >/dev/null 2>&1 || info "note: --help nonzero (binary still runnable)"

# ---- 5. Capture rollback state (always, even in dry-run) --------------------
log "5. Capture rollback state"
mkdir -p "$BACKUP_DIR"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
STATE_FILE="${BACKUP_DIR}/deploy-${STAMP}.state.txt"
{
  echo "deploy_timestamp_utc: $STAMP"
  echo "canonical: $CANONICAL_DIR @ $SRC_BRANCH ($SRC_HEAD)"
  echo "source_commit: $SRC_COMMIT_FULL"
  echo "binary_provenance: version=$SRC_VERSION commit=$SRC_COMMIT_SHORT (ldflags-stamped; buildvcs disabled)"
  echo "runtime_dir: $RUNTIME_DIR"
  echo "runtime_git_head: $(git -C "$RUNTIME_DIR" rev-parse HEAD 2>&1)"
  echo "runtime_git_branch: $(git -C "$RUNTIME_DIR" rev-parse --abbrev-ref HEAD 2>&1)"
  echo "staged_binary_size: $STAGE_SIZE"
  echo "--- systemctl --user status $SERVICE (head) ---"
  systemctl --user status "$SERVICE" --no-pager 2>&1 | head -6 || true
  echo "--- chrote tmux sessions (TMUX_TMPDIR=$CHROTE_TMUX_TMPDIR) ---"
  TMUX_TMPDIR="$CHROTE_TMUX_TMPDIR" tmux list-sessions -F '#{session_name}' 2>&1 | sort || true
} > "$STATE_FILE"
info "rollback state recorded: $STATE_FILE"

# Snapshot of session names BEFORE, for preservation check.
BEFORE_SESSIONS="$(TMUX_TMPDIR="$CHROTE_TMUX_TMPDIR" tmux list-sessions -F '#{session_name}' 2>/dev/null | sort || true)"
BEFORE_COUNT="$(printf '%s\n' "$BEFORE_SESSIONS" | grep -c . || true)"
info "chrote tmux sessions before: $BEFORE_COUNT"

# Back up the current runtime binary.
BACKUP_BIN=""
if [ -f "$RUNTIME_BINARY" ]; then
  BACKUP_BIN="${BACKUP_DIR}/chrote-server.${STAMP}.bak"
  cp -p "$RUNTIME_BINARY" "$BACKUP_BIN"
  info "runtime binary backed up: $BACKUP_BIN ($(stat -c %s "$BACKUP_BIN") bytes)"
else
  info "WARNING: no existing runtime binary at $RUNTIME_BINARY to back up"
fi

if [ "$DRY_RUN" -eq 1 ]; then
  log "DRY RUN — stopping before runtime change"
  info "Would copy: $STAGE_BIN  ->  $RUNTIME_BINARY"
  info "Would run:  systemctl --user restart $SERVICE"
  info "Would smoke: $HEALTH_URL and tmux session preservation"
  rm -f "$STAGE_BIN"
  exit 0
fi

# ---- 6. Swap binary (atomic) ------------------------------------------------
log "6. Deploy binary into runtime (atomic swap)"
INSTALL_TMP="${RUNTIME_BINARY}.new.${STAMP}"
cp "$STAGE_BIN" "$INSTALL_TMP"
chmod +x "$INSTALL_TMP"
mv -f "$INSTALL_TMP" "$RUNTIME_BINARY"   # rename within same fs == atomic
rm -f "$STAGE_BIN"
info "installed: $RUNTIME_BINARY ($(stat -c %s "$RUNTIME_BINARY") bytes)"

# ---- 7. Restart only chrote.service ----------------------------------------
log "7. Restart $SERVICE"
systemctl --user restart "$SERVICE" || die "service restart failed (see: systemctl --user status $SERVICE)"

# ---- 8. Smoke: health + tmux preservation ----------------------------------
log "8. Smoke: health + tmux preservation"
ok=1
# (a) service active
if systemctl --user is-active --quiet "$SERVICE"; then
  info "service active: OK"
else
  info "service active: FAIL"; ok=0
fi
# (b) health (retry briefly for startup)
health_ok=0
for _ in 1 2 3 4 5 6 7 8 9 10; do
  body="$(curl -sS -m 5 "$HEALTH_URL" 2>/dev/null || true)"
  if printf '%s' "$body" | grep -q '"status":"ok"'; then health_ok=1; break; fi
  sleep 1
done
if [ "$health_ok" -eq 1 ]; then info "health: OK ($body)"; else info "health: FAIL (last: ${body:-<none>})"; ok=0; fi
# (c) tmux session preservation
AFTER_SESSIONS="$(TMUX_TMPDIR="$CHROTE_TMUX_TMPDIR" tmux list-sessions -F '#{session_name}' 2>/dev/null | sort || true)"
MISSING="$(comm -23 <(printf '%s\n' "$BEFORE_SESSIONS") <(printf '%s\n' "$AFTER_SESSIONS") | grep -c . || true)"
if [ "${MISSING:-0}" -eq 0 ]; then
  info "tmux sessions preserved: OK ($BEFORE_COUNT before; all present after)"
else
  info "tmux sessions preserved: FAIL ($MISSING missing)"; ok=0
  comm -23 <(printf '%s\n' "$BEFORE_SESSIONS") <(printf '%s\n' "$AFTER_SESSIONS")
fi

if [ "$ok" -ne 1 ]; then
  log "SMOKE FAILED — rolling back"
  if [ -n "$BACKUP_BIN" ] && [ -f "$BACKUP_BIN" ]; then
    cp -p "$BACKUP_BIN" "$RUNTIME_BINARY"
    systemctl --user restart "$SERVICE" || true
    info "rolled back to $BACKUP_BIN and restarted $SERVICE"
    sleep 2
    rb="$(curl -sS -m 5 "$HEALTH_URL" 2>/dev/null || true)"
    info "post-rollback health: ${rb:-<none>}"
  else
    info "NO BACKUP BINARY AVAILABLE — manual recovery required"
  fi
  die "deploy verification failed; rollback attempted. See $STATE_FILE"
fi

log "DEPLOY OK"
info "canonical $SRC_BRANCH ($SRC_HEAD) -> $RUNTIME_BINARY"
info "binary provenance: version=$SRC_VERSION commit=$SRC_COMMIT_SHORT (verify: curl -s ${HEALTH_URL%/api/health}/api/version)"
info "rollback binary: ${BACKUP_BIN:-<none>}"
info "rollback state:  $STATE_FILE"
info "Run scripts/smoke.sh for the full read-only verification incl. transcript route."
