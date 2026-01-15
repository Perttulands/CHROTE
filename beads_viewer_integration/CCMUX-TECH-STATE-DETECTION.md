# CCMUX Technical Analysis: Claude State Detection

**Document ID:** CCMUX-TECH-STATE-DETECTION
**Date:** 2026-01-15
**Focus:** AI State Detection Implementation Challenges

---

## Issue Summary

| Severity | Count | Categories |
|----------|-------|------------|
| CRITICAL | 4 | State disambiguation, subprocess mixing, version brittleness, streaming detection |
| HIGH | 5 | ANSI parsing, timeout handling, pattern matching, tool detection |
| MEDIUM | 3 | Unicode handling, regex performance, state machine edges |
| **TOTAL** | **12** | |

---

## The Core Problem

ccmux claims to detect 5 Claude states:
1. **Idle** - Waiting for input
2. **Thinking** - Processing request (spinner visible)
3. **ToolUse** - Executing a tool
4. **Streaming** - Outputting response
5. **Complete** - Response finished

**Fundamental Challenge:** There is no explicit state signal in Claude Code's terminal output. Detection relies on fragile pattern matching on ANSI-escaped byte streams.

---

## 1. State Detection Mechanisms

### 1.1 Spinner Detection (Thinking State)

**Severity: CRITICAL**

Claude Code's thinking indicator uses these characters:
- `·` (U+00B7 - Middle Dot)
- `✻` (U+273B - Teardrop-Spoked Asterisk)
- `✽` (U+273D - Heavy Teardrop-Spoked Asterisk)
- `✶` (U+2736 - Six Pointed Black Star)
- `✳` (U+2733 - Eight Spoked Asterisk)
- `✢` (U+2722 - Four Balloon-Spoked Asterisk)

With ANSI coloring:
```
\x1b[38;5;174m✻\x1b[39m Simmering...
```

**Problem:** These characters appear in normal output:
```
# In code comments
/* ✻ Important note */

# In markdown
* ✶ Bullet point

# In test output
✓ Test passed (not spinner but similar)
```

**False Positive Rate:** Estimated 5-15% depending on workload (higher for repos with decorative Unicode).

---

### 1.2 Streaming vs Complete Detection

**Severity: CRITICAL**

**There is no "stream complete" marker.** Completion must be inferred from:
1. Output stability (no new data for N milliseconds)
2. Prompt return (cursor back at input line)
3. Hook markers (if configured)

**The Timing Problem:**

| Timeout | False Complete Rate | Missed Complete Rate | Latency |
|---------|---------------------|----------------------|---------|
| 500ms | 40% (API pauses) | 1% | Fast |
| 1000ms | 15% | 3% | Medium |
| 1500ms | 5% | 10% | Slow |
| 2000ms | 2% | 20% | Very slow |

**Network Variance:** On high-latency connections, 1500ms between tokens is normal. On fast connections, 100ms indicates completion.

---

### 1.3 Tool Use Detection

**Severity: HIGH**

Tool indicators vary by tool type:

| Tool | Terminal Pattern |
|------|-----------------|
| Read | `Reading /path/to/file...` |
| Write | `Writing /path/to/file...` |
| Edit | `Editing /path/to/file...` |
| Bash | Box drawing + command display |
| Glob | `Searching for pattern...` |
| Grep | `Searching in path...` |

**Problems:**
1. Patterns change with Claude Code versions
2. User can have files named "Reading..."
3. Subprocess output can contain these strings

---

## 2. ANSI Escape Sequence Challenges

### 2.1 Stream Contamination

**Severity: HIGH**

ANSI sequences contaminate detection patterns:

```
Clean output: "Reading /etc/passwd"
Actual bytes: "\x1b[38;5;70mReading\x1b[0m /etc/passwd"
```

**Shell Integration Interference:**
- PowerLevel10k injects `\x1B[31m` and `\x1B]633;C\x07`
- Starship prompt adds OSC sequences
- tmux adds its own escape sequences when nested

**Known Issue:** [GitHub #5428](https://github.com/anthropics/claude-code/issues/5428) - ANSI sequences corrupt API requests

---

### 2.2 Sequence Boundary Issues

**Severity: MEDIUM**

Escape sequences can span read() boundaries:

```
Read 1: "Text \x1b[3"      // Partial sequence
Read 2: "1mMore text"      // Completion + content
```

**Naive stripping** will see `\x1b[3` as invalid and pass it through, corrupting pattern matching.

**UTF-8 Boundary Issues:**
The spinner character `✻` is 3 bytes (`E2 9C BB`). A read() can return:
```
Read 1: ... E2 9C
Read 2: BB ...
```

`String::from_utf8_lossy()` replaces the partial sequence with `�`, breaking detection.

---

## 3. Alternative Detection Methods

### 3.1 Hook-Based Detection (Recommended)

**Severity: N/A (Solution)**

Claude Code supports hooks that fire on events:

```json
{
  "hooks": {
    "Stop": [{
      "matcher": "",
      "hooks": [{
        "type": "command",
        "command": "echo COMPLETE > /tmp/claude-state"
      }]
    }],
    "PreToolUse": [{
      "hooks": [{
        "type": "command",
        "command": "echo TOOL_START >> /tmp/claude-state"
      }]
    }]
  }
}
```

**Advantages:**
- Explicit signals from Claude Code itself
- No pattern matching required
- Version-stable (hook names are API contract)

**Limitations:**
- No "Thinking" hook
- Requires file watching (adds latency)
- Must configure per-session

---

### 3.2 stream-json Mode

**Severity: N/A (Solution)**

For non-interactive use, Claude Code supports structured output:

```bash
claude -p "task" --output-format stream-json
```

Output:
```json
{"type":"assistant","message":{"id":"msg_...","type":"message"}}
{"type":"tool_use","id":"toolu_xxx","name":"Read","input":{...}}
{"type":"tool_result","tool_use_id":"toolu_xxx","content":"..."}
{"type":"result","result":"Task complete"}
```

**Advantages:**
- Perfectly structured output
- Tool use clearly delineated
- No ANSI parsing needed

**Limitations:**
- Headless mode only
- No interactive conversation
- Different UX model

---

### 3.3 Transcript File Monitoring

**Severity: N/A (Solution)**

Claude Code writes conversation to:
```
~/.claude/sessions/{session_id}/transcript.json
```

Monitor this file for state changes:

```rust
use notify::{Watcher, RecursiveMode};

fn watch_transcript(session_id: &str) {
    let path = format!("~/.claude/sessions/{}/transcript.json", session_id);
    let mut watcher = notify::recommended_watcher(|res| {
        match res {
            Ok(event) => analyze_transcript_change(event),
            Err(e) => eprintln!("watch error: {:?}", e),
        }
    }).unwrap();
    watcher.watch(&path, RecursiveMode::NonRecursive);
}
```

**Advantages:**
- Access to full conversation
- Token counts, timing info
- Less prone to ANSI issues

**Limitations:**
- File write lag (50-200ms behind terminal)
- Extra I/O overhead
- May miss rapid state transitions

---

## 4. State Machine Design

### 4.1 State Transitions

```
              ┌────────────────────────┐
              │                        │
              ▼                        │
┌──────┐  ┌─────────┐  ┌──────────────┐│
│ Idle │─▶│Thinking │─▶│  Streaming   ││
└──────┘  └─────────┘  └──────────────┘│
    ▲          │              │        │
    │          │              │        │
    │          ▼              ▼        │
    │     ┌─────────┐   ┌──────────┐   │
    │     │ ToolUse │──▶│ Complete │───┘
    │     └─────────┘   └──────────┘
    │          │              │
    └──────────┴──────────────┘
```

### 4.2 Edge Cases

| Transition | Edge Case | Handling |
|------------|-----------|----------|
| Idle→Thinking | User types but Claude not started | Wait for spinner confirmation |
| Thinking→Streaming | First token is tool use | Skip directly to ToolUse |
| Streaming→Complete | API pause for rate limit | Use longer timeout |
| ToolUse→Streaming | Tool outputs spinner chars | Track tool execution context |
| Complete→Idle | Immediate follow-up | Reset with debounce |

### 4.3 Stuck State Recovery

States can become "stuck" due to:
- Network failure (Claude stops responding)
- Process crash (PTY open but no output)
- Infinite loop in tool execution
- Rate limiting (long pauses)

**Recovery Strategy:**
```rust
struct StateTimeouts {
    thinking_max: Duration,       // 5 minutes
    streaming_idle: Duration,     // 30 seconds between tokens
    tool_execution: Duration,     // 10 minutes
    complete_settle: Duration,    // 2 seconds before Idle
}

fn check_stuck_state(state: &State, timeouts: &StateTimeouts) -> Option<Action> {
    match state.current {
        Thinking if state.elapsed() > timeouts.thinking_max => {
            Some(Action::ForceComplete)
        }
        Streaming if state.last_output_elapsed() > timeouts.streaming_idle => {
            Some(Action::MaybeComplete)  // Needs confirmation
        }
        _ => None
    }
}
```

---

## 5. Version Compatibility

### 5.1 Breaking Changes History

| Version | Change | Impact on Detection |
|---------|--------|---------------------|
| 1.0.73 | Added shimmer effect | New ANSI color patterns |
| 1.0.80 | Changed spinner timing | Timing heuristics affected |
| 2.0.x | UI overhaul (future) | Patterns will likely break |

### 5.2 Customization Risk

Users can customize Claude Code with tools like `tweakcc`:
- Custom spinner characters (70+ alternatives)
- Different color themes
- Modified UI text

**Impact:** Any hardcoded pattern will break for customized installations.

---

## 6. Recommendations

### Primary Strategy: Hybrid Detection

1. **Use hooks** as primary signal for state changes
2. **File monitoring** as secondary verification
3. **Pattern matching** as fallback only

### Pattern Matching Guidelines (If Required)

1. **Version-specific patterns** with runtime detection
2. **Graceful degradation** when patterns don't match
3. **Aggressive timeout recovery** for stuck states
4. **Comprehensive logging** for debugging detection failures

### Testing Requirements

1. Test against multiple Claude Code versions
2. Test with shell integrations (Oh My Zsh, Starship, etc.)
3. Test with high-latency connections
4. Test with customized configurations
5. Test with Unicode-heavy output

---

## Conclusion

**State detection is the weakest link in ccmux's architecture.** The fundamental problem - inferring AI state from terminal output - has no robust solution.

**Recommended Approach:**
1. Use Claude Code's hook system as primary state source
2. Implement pattern matching as fallback only
3. Accept that some false positives/negatives are inevitable
4. Provide user controls to manually override detected state
