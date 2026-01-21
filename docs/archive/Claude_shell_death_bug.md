# Claude Code Shell Death Bug (Tmux-Specific)

## Problem Summary

Claude Code's bash tool stops working after **2–6 hours** of continuous operation **when running inside a tmux session**. All bash commands return exit code 1 with no output, including `true` (which should always return 0).

This is not a general "bash is broken" failure: other tools (e.g. search) continue to work, the tmux server remains healthy, and a reset of the Claude conversation restores bash-tool functionality.

## Key Evidence

| Environment | Problem? |
|-------------|----------|
| VSCode terminal (direct) | No |
| Direct terminal | No |
| VSCode extension | No |
| **Inside tmux session (Chrote)** | **Yes** |

**Critical observation:** The problem only occurs when Claude Code runs inside tmux.

## Error Pattern

```
● Bash(true)
  ⎿  Error: Exit code 1      <-- Impossible for healthy bash
● Bash(ls -la)
  ⎿  Error: Exit code 1
● Search(pattern: "*.go")
  ⎿  Found 33 files          <-- Non-bash tools still work
```

## Recovery

- **Clearing the Claude conversation fixes it** without touching tmux
- The tmux session remains healthy (new sessions work immediately)
- This indicates Claude Code's internal shell subprocess is corrupted, not tmux itself

## Architecture Context (Chrote)

In Chrote, the "terminal" is typically:

```
browser → ttyd (websocket terminal) → launch script → tmux attach/new-session → Claude Code process
```

Key implications:

- Claude is running under tmux-provided PTY semantics (`TERM=screen*`, `TMUX` env var, frequent SIGWINCH, tmux escape-sequence handling).
- Chrote forces a stable tmux socket directory by setting `TMUX_TMPDIR` (often `/run/tmux/chrote` in WSL/systemd setups).
- The tmux server remaining healthy strongly suggests the failure is inside Claude Code’s own long-lived "bash tool" runtime or its communication with that runtime.

## Root Cause Hypothesis

Claude Code + tmux interaction causes the issue. Possible mechanisms:

1. **PTY layer interference** - Tmux creates its own PTY. Claude Code's subprocess stack becomes: `Claude Code → bash → tmux PTY → inner bash`. This extra layer may cause issues with Claude Code's shell management.

2. **Escape sequence conflicts** - Tmux intercepts certain escape sequences. If Claude Code uses escape sequences for prompt detection or shell synchronization, tmux might interfere.

3. **Signal handling conflicts** - Tmux has its own SIGWINCH, SIGCHLD handling that could interfere with Claude Code's subprocess management.

4. **Output buffering differences** - Tmux buffers terminal output differently than direct PTY. This could affect how Claude Code parses command output.

Additional high-probability mechanisms (based on how many agent CLIs implement shell tools):

5. **Prompt/marker parsing under `TERM=screen*`** - If the bash tool relies on detecting prompts or sentinels in output, tmux terminal semantics (OSC sequences, bracketed paste, title sequences) can introduce output that confuses the parser. A rare parser/state-machine bug can cause every subsequent tool invocation to be treated as a failure.

6. **FD/PTY/pipe leak over time** - Long-lived "shell tool" subprocesses can leak file descriptors, PTYs, or pipes. After hours, the Claude process may reach `ulimit -n` and fail to spawn/communicate with its shell, surfacing as generic exit code 1 with no output.

7. **TTY mode drift** - `stty` flags can drift (flow control, echo, newline translation). This usually breaks interactive behavior first, but it can also break output capture or termination detection.

## Workarounds

1. **Periodic conversation reset** - Clear every few hours before it breaks
2. **Try `/compact`** - May reset internal state without losing full context
3. **Run Claude Code outside tmux** - If feasible, run directly in ttyd without tmux layer

## Diagnostics (Run *when it is broken*)

These checks help distinguish "can't spawn" vs "can't parse/handshake":

### 1) Check whether the Claude process is hitting FD limits

In the pane running Claude:

```bash
PID=$(tmux display -p '#{pane_pid}')
ulimit -n
ls /proc/$PID/fd | wc -l
cat /proc/$PID/limits | sed -n '1,80p'
```

If `fd` count is near the limit, this strongly suggests a leak.

### 2) Confirm the environment differences tmux introduces

```bash
echo "TERM=$TERM"
echo "TMUX=$TMUX"
```

### 3) Check process tree to see if a shell child is being created

```bash
PID=$(tmux display -p '#{pane_pid}')
pstree -ap $PID | head -200
```

- If no shell/tool children appear during a failing tool invocation, suspect resource exhaustion or spawn failure.
- If children appear but output is consistently empty, suspect parser/PTY handshake issues.

### 4) Sanity-check the terminal TTY mode

```bash
tty
stty -a
```

If you see obvious weirdness (e.g. flow control enabled and output seems frozen), try `stty sane` as a quick rule-out.

## Discriminating Experiments

### A) Change TERM for Claude (tests prompt/escape parsing sensitivity)

Start Claude in a new tmux session with a different `TERM`:

```bash
TERM=xterm-256color claude
```

If the problem disappears, it points strongly to a tmux-terminal-semantics interaction rather than a general leak.

### B) Run Claude in a fresh tmux session while the old one is broken

If a fresh Claude instance in a new session works immediately while the old instance stays broken, it supports the hypothesis that this is per-process state inside Claude.

## Recommended Action

Report to Anthropic at https://github.com/anthropics/claude-code/issues with:
- Error pattern showing `true` returning exit code 1
- Note that it **only occurs inside tmux sessions**
- Works fine in direct terminal, VSCode terminal, VSCode extension

Include the diagnostic outputs above if possible (especially FD counts and `TERM`/`TMUX`), since those quickly separate "leak" vs "terminal semantics/parser" classes of bugs.
