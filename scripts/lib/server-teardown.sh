# Shared teardown for the smoke scripts that start a chrote-server in the
# background. Source it, keep the pid in $server_pid, call stop_server on every
# exit path, and call assert_server_released before exiting.
#
# It exists because a leaked server is silent: the exit status, the output and
# the PASS line all say the run cleaned up, so anything reading them as evidence
# reads a false receipt. One such server was found still listening ten hours
# after its script printed PASS and exited 0.
#
# Contract with the caller:
#   server_pid   the pid of the server this run started; set it right after the
#                background start, and let stop_server clear it.
#   stop_server  ends that server, bounded; safe to call twice.
#   assert_server_released <process_pattern> <port>
#                fails when the pid survived, when anything matching
#                <process_pattern> (a pgrep -f ERE) survived, or when <port> is
#                still held. Either argument may be empty.

# The pid the run last started, kept after server_pid is cleared so the closing
# assertion can name what it found.
server_pid_last=""
# Tenths of a second to wait after TERM, then after KILL.
server_stop_grace=100
server_kill_grace=50

# require_server_teardown_tools fails early rather than letting the closing
# assertion pass vacuously because a tool it needs is missing.
require_server_teardown_tools() {
  if ! command -v pgrep >/dev/null 2>&1; then
    echo 'pgrep is required to assert the run left no server behind' >&2
    return 1
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    echo 'python3 is required to assert the run released its port' >&2
    return 1
  fi
}

# server_has_exited reports whether the pid is gone or is a zombie waiting to be
# reaped. kill -0 alone cannot tell the two apart, and a zombie has exited.
server_has_exited() {
  local pid="$1" state
  kill -0 "$pid" 2>/dev/null || return 0
  state="$(sed -e 's/.*) //' -e 's/ .*//' "/proc/$pid/stat" 2>/dev/null || true)"
  [ "$state" = "Z" ]
}

# stop_server ends the server this run started and reaps it. The wait is bounded
# on purpose: a server that ignores TERM must fail the run rather than hang it,
# because a hung run is what gets killed from outside, and an outside kill is
# what leaves the server behind.
stop_server() {
  local pid="$server_pid" _attempt
  [ -n "$pid" ] || return 0
  server_pid=""
  server_pid_last="$pid"
  kill -TERM "$pid" 2>/dev/null || true
  for _attempt in $(seq 1 "$server_stop_grace"); do
    if server_has_exited "$pid"; then
      break
    fi
    sleep 0.1
  done
  if ! server_has_exited "$pid"; then
    printf 'chrote-server PID %s ignored TERM; killing it\n' "$pid" >&2
    kill -KILL "$pid" 2>/dev/null || true
    for _attempt in $(seq 1 "$server_kill_grace"); do
      if server_has_exited "$pid"; then
        break
      fi
      sleep 0.1
    done
  fi
  wait "$pid" 2>/dev/null || true
}

# port_is_free rebinds the port the run used. SO_REUSEADDR is set so a socket
# left in TIME_WAIT by the run's own client connections does not read as a
# leaked listener.
port_is_free() {
  python3 - "$1" <<'PORT_PROBE'
import socket
import sys

probe = socket.socket()
probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    probe.bind(('127.0.0.1', int(sys.argv[1])))
    probe.listen(1)
except OSError:
    raise SystemExit(1)
finally:
    probe.close()
PORT_PROBE
}

# assert_server_released is what keeps a PASS honest. Fail the run rather than
# report success over a server that is still running.
assert_server_released() {
  local pattern="${1:-}" port="${2:-}" status=0 survivors=""
  if [ -n "$server_pid_last" ] && kill -0 "$server_pid_last" 2>/dev/null; then
    printf 'chrote-server PID %s started by this run is still alive\n' "$server_pid_last" >&2
    status=1
  fi
  if [ -n "$pattern" ]; then
    survivors="$(pgrep -f "$pattern" 2>/dev/null | tr '\n' ' ' || true)"
    survivors="${survivors% }"
    if [ -n "$survivors" ]; then
      printf 'processes matching %s survived the run: %s\n' "$pattern" "$survivors" >&2
      status=1
    fi
  fi
  if [ -n "$port" ] && ! port_is_free "$port"; then
    printf 'port %s is still held after the run\n' "$port" >&2
    status=1
  fi
  return "$status"
}
