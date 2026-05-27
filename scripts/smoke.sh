#!/usr/bin/env bash
# CHROTE read-only smoke test. Verifies the live cockpit is healthy and that the
# transcript-recovery API is live, and (optionally) that no chrote tmux session
# was lost vs a recorded "before" list.
#
# This script is READ-ONLY: it never restarts the service, never touches the
# tmux socket, never runs tmux kill-*, and never modifies any tree.
#
# Model: docs/runtime-deploy-model.md
#
# Usage:
#   scripts/smoke.sh                 # checks (a) service, (b) health, (d) transcript-live
#   scripts/smoke.sh --capture FILE  # write current chrote tmux session names to FILE, exit
#   scripts/smoke.sh --before FILE   # also verify (c): every session in FILE still present

set -Eeuo pipefail

SERVICE="chrote.service"
HEALTH_URL="http://127.0.0.1:8094/api/health"
BASE="http://127.0.0.1:8094"
CHROTE_TMUX_TMPDIR="/run/user/1000/chrote-tmux"
# A session id the Gas City supervisor will not resolve, so the transcript route
# exercises the not-found / archive (stale-recovery) branch. Either way the NEW
# binary returns the JSON API envelope; the OLD binary returned SPA HTML.
PROBE_ID="gc-smoke-nonexistent-0000"
# Real Gas City sessions for the live-recovery acceptance gate: the transcript
# route must return 200 with real content (success=true), not a 502. Override
# with CHROTE_SMOKE_REAL_SESSIONS="gc-a gc-b".
REAL_SESSIONS="${CHROTE_SMOKE_REAL_SESSIONS:-gc-4168 gc-51923}"

BEFORE_FILE=""
CAPTURE_FILE=""
for ((i=1;i<=$#;i++)); do
  case "${!i}" in
    --before) j=$((i+1)); BEFORE_FILE="${!j}" ;;
    --capture) j=$((i+1)); CAPTURE_FILE="${!j}" ;;
    -h|--help) sed -n '1,25p' "$0"; exit 0 ;;
  esac
done

info() { printf '    %s\n' "$*"; }

list_sessions() { TMUX_TMPDIR="$CHROTE_TMUX_TMPDIR" tmux list-sessions -F '#{session_name}' 2>/dev/null | sort; }

if [ -n "$CAPTURE_FILE" ]; then
  list_sessions > "$CAPTURE_FILE"
  printf 'captured %s chrote tmux session names -> %s\n' "$(grep -c . "$CAPTURE_FILE" || echo 0)" "$CAPTURE_FILE"
  exit 0
fi

ok=1
printf '=== CHROTE smoke (read-only) ===\n'

# (a) service active
if systemctl --user is-active --quiet "$SERVICE"; then
  info "(a) service active: OK"
else
  info "(a) service active: FAIL"; ok=0
fi

# (b) web health
body="$(curl -sS -m 5 "$HEALTH_URL" 2>/dev/null || true)"
if printf '%s' "$body" | grep -q '"status":"ok"'; then
  info "(b) health: OK ($body)"
else
  info "(b) health: FAIL (got: ${body:-<none>})"; ok=0
fi

# (c) tmux session preservation (optional)
if [ -n "$BEFORE_FILE" ]; then
  if [ -f "$BEFORE_FILE" ]; then
    after="$(list_sessions)"
    missing="$(comm -23 <(sort "$BEFORE_FILE") <(printf '%s\n' "$after") | grep -c . || true)"
    bcount="$(grep -c . "$BEFORE_FILE" || echo 0)"
    if [ "${missing:-0}" -eq 0 ]; then
      info "(c) tmux sessions preserved: OK ($bcount before; all present after)"
    else
      info "(c) tmux sessions preserved: FAIL ($missing missing):"
      comm -23 <(sort "$BEFORE_FILE") <(printf '%s\n' "$after") | sed 's/^/        - /'
      ok=0
    fi
  else
    info "(c) tmux preservation: SKIP (before file '$BEFORE_FILE' not found)"
  fi
else
  info "(c) tmux preservation: SKIP (no --before file)"
fi

# (d) transcript-recovery live: route must return the JSON API envelope, not SPA HTML.
ctype="$(curl -sS -m 8 -o /dev/null -w '%{content_type}' "$BASE/api/gascity/sessions/$PROBE_ID/transcript?lines=1" 2>/dev/null || true)"
tbody="$(curl -sS -m 8 "$BASE/api/gascity/sessions/$PROBE_ID/transcript?lines=1" 2>/dev/null || true)"
if printf '%s' "$ctype" | grep -qi 'application/json' && printf '%s' "$tbody" | grep -q '"success"'; then
  # Show whether it returned an archive snapshot (success) or a structured error.
  code="$(printf '%s' "$tbody" | sed -n 's/.*"code":"\([A-Z_]*\)".*/\1/p' | head -1)"
  info "(d) transcript route live: OK (content-type=$ctype${code:+, code=$code})"
else
  info "(d) transcript route live: FAIL (content-type='${ctype:-?}', body head='${tbody:0:120}')"
  info "    note: HTML/text content-type means the OLD binary (SPA fallback) — new API route not live."
  ok=0
fi

# (e) REAL-SESSION recovery gate: each real session must return HTTP 200 with
# success=true and non-empty transcript content (the deployed-502 acceptance gate).
for sid in $REAL_SESSIONS; do
  http_code="$(curl -sS -m 20 -o /tmp/chrote-smoke-real.$$ -w '%{http_code}' "$BASE/api/gascity/sessions/$sid/transcript?lines=20" 2>/dev/null || echo 000)"
  rbody="$(cat /tmp/chrote-smoke-real.$$ 2>/dev/null || true)"; rm -f /tmp/chrote-smoke-real.$$
  tlen="$(printf '%s' "$rbody" | sed -n 's/.*"transcript":"\(.*\)".*/\1/p' | head -1 | wc -c)"
  if [ "$http_code" = "200" ] && printf '%s' "$rbody" | grep -q '"success":true' && [ "${tlen:-0}" -gt 1 ]; then
    src="$(printf '%s' "$rbody" | sed -n 's/.*"source":"\([a-z-]*\)".*/\1/p' | head -1)"
    info "(e) real-session $sid: OK (HTTP 200, success, source=$src, ~$tlen transcript bytes)"
  else
    info "(e) real-session $sid: FAIL (HTTP $http_code, body head='${rbody:0:160}')"
    ok=0
  fi
done

# (f) provenance: /api/version must report a real source commit (not 'unknown').
vbody="$(curl -sS -m 5 "$BASE/api/version" 2>/dev/null || true)"
vcommit="$(printf '%s' "$vbody" | sed -n 's/.*"commit":"\([^"]*\)".*/\1/p' | head -1)"
if [ -n "$vcommit" ] && [ "$vcommit" != "unknown" ]; then
  info "(f) binary provenance: OK (/api/version commit=$vcommit)"
else
  info "(f) binary provenance: FAIL (commit='${vcommit:-<none>}', body='${vbody:0:120}')"
  ok=0
fi

printf '=== %s ===\n' "$([ "$ok" -eq 1 ] && echo PASS || echo FAIL)"
[ "$ok" -eq 1 ]
