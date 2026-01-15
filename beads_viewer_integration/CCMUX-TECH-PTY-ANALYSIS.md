# CCMUX Technical Analysis: PTY Management & Cross-Platform Compatibility

**Document ID:** CCMUX-TECH-PTY-ANALYSIS
**Date:** 2026-01-15
**Focus:** Pseudo-Terminal Implementation & Platform Differences

---

## Issue Summary

| Severity | Count | Categories |
|----------|-------|------------|
| CRITICAL | 4 | Signal handling, blocking I/O, FD leaks, FFI safety |
| HIGH | 6 | Resize races, buffer management, cross-platform PTY |
| MEDIUM | 4 | Socket paths, file locking, color support |
| LOW | 2 | Unicode width, terminal detection |
| **TOTAL** | **16** | |

---

## 1. PTY Implementation Challenges

### 1.1 portable-pty 0.9 Platform Abstraction

**Severity: CRITICAL**

The `portable-pty` crate handles PTY allocation but has significant platform-specific issues:

#### Linux (ptmx) Flow
```rust
let master_fd = open("/dev/ptmx", O_RDWR | O_NOCTTY | O_CLOEXEC);
grantpt(master_fd);    // May spawn setuid helper - can fail silently
unlockpt(master_fd);
let slave_name = ptsname(master_fd);  // NOT thread-safe!
```

#### macOS (openpty) Flow
```rust
openpty(&master_fd, &slave_fd, slave_name, &termios, &winsize);
// Doesn't support O_CLOEXEC - requires manual fcntl
```

**Key Issues:**
- `ptsname()` returns static storage (race condition in multi-threaded code)
- `grantpt()` can fail in containers without proper /dev/pts setup
- macOS requires extra `fcntl(F_SETFD, FD_CLOEXEC)` call

---

### 1.2 Signal Handling Complexity

**Severity: CRITICAL**

#### SIGWINCH (Window Resize)

**Problem:** Must be async-signal-safe, but most operations aren't.

```rust
// WRONG - This crashes
extern "C" fn sigwinch_handler(_sig: c_int) {
    let now = Instant::now();  // ALLOCATES! Not signal-safe
    resize_all_ptys();         // Complex logic in signal context
}

// CORRECT - Atomic flag only
static SIGWINCH_PENDING: AtomicBool = AtomicBool::new(false);

extern "C" fn sigwinch_handler(_sig: c_int) {
    SIGWINCH_PENDING.store(true, Ordering::SeqCst);
}
```

**Real-World Failure:**
1. User resizes terminal rapidly
2. SIGWINCH fires while another SIGWINCH is being processed
3. Signal handler allocates memory
4. malloc() was mid-operation → heap corruption → crash

#### SIGCHLD (Child Termination)

**Problem:** Must reap ALL children in a loop, not just one.

```rust
// WRONG - Leaves zombies
extern "C" fn sigchld_handler(_sig: c_int) {
    waitpid(-1, &mut status, WNOHANG);  // Only reaps ONE
}

// CORRECT - Reap all
extern "C" fn sigchld_handler(_sig: c_int) {
    loop {
        match waitpid(-1, &mut status, WNOHANG) {
            pid if pid > 0 => continue,  // Reaped one, try again
            _ => break,  // No more children
        }
    }
}
```

---

### 1.3 File Descriptor Management

**Severity: HIGH**

#### FD Leak Pattern

```rust
fn create_pty_session() -> Result<Session> {
    let pty = PtyPair::new()?;              // FDs allocated
    let child = spawn_shell(&pty)?;          // Can fail here!
    let reader = pty.master.try_clone_reader()?;  // Another FD
    // If this fails, all previous FDs are leaked
    let writer = pty.master.take_writer()?;
    Ok(Session { pty, child, reader, writer })
}
```

**Solution:** Use RAII guards or careful cleanup on error paths.

#### FD Inheritance Attack

Child processes inherit all non-CLOEXEC FDs. If ccmux holds an IPC socket on FD 5:
1. Fork child shell
2. Child inherits FD 5
3. Child can read/write ccmux's internal socket

**Defense:** Set `O_CLOEXEC` on all FDs immediately after creation.

---

### 1.4 Blocking PTY I/O in Async Runtime

**Severity: CRITICAL**

Tokio's async runtime threads **must not block**. But PTY read() is blocking.

```rust
// WRONG - Blocks tokio worker thread
async fn read_pty_bad(master: &MasterPty) -> io::Result<Vec<u8>> {
    let n = master.try_clone_reader()?.read(&mut buf)?;  // BLOCKS!
}

// CORRECT - Use spawn_blocking
async fn read_pty_good(master: Arc<MasterPty>) -> io::Result<Vec<u8>> {
    tokio::task::spawn_blocking(move || {
        // Runs on dedicated blocking thread pool
        master.try_clone_reader()?.read(&mut buf)
    }).await?
}
```

**Consequence of Blocking:** Under high output load (e.g., `cat /dev/urandom`), all tokio workers block on PTY reads. Input handling, resize events, and other async tasks starve. UI freezes.

---

## 2. Cross-Platform Issues

### 2.1 Unix Socket Path Length

**Severity: HIGH**

| Platform | Max Path Length |
|----------|-----------------|
| Linux | 108 bytes |
| macOS | 104 bytes |

**Problem:** Long usernames or nested project paths can exceed limits:
```
/home/very-long-username/projects/my-project/.ccmux/session-abc123/socket
// 78+ characters - may exceed on macOS
```

**Solutions:**
1. **Linux:** Use abstract sockets (start with `\0`, no filesystem path)
2. **macOS:** Use short paths like `/tmp/ccmux-{uid}/{short_id}.sock`

---

### 2.2 Windows ConPTY

**Severity: HIGH**

Windows ConPTY is fundamentally different from Unix PTY:

| Aspect | Unix PTY | Windows ConPTY |
|--------|----------|----------------|
| API | `openpty()` / `posix_openpt()` | `CreatePseudoConsole()` |
| Resize | `TIOCSWINSZ` ioctl | `ResizePseudoConsole()` |
| Signals | `SIGWINCH` | Event-based |
| I/O | File descriptors | HANDLE pipes |

**Key Differences:**
- No SIGWINCH - must poll or use callback
- Different escape sequences (Windows Terminal has extensions)
- `EXTENDED_STARTUPINFO_PRESENT` flag required for process spawn

---

### 2.3 Terminal Resize Race Conditions

**Severity: HIGH**

**Timeline of a resize bug:**
```
T0: Terminal emulator resizes
T1: ccmux receives SIGWINCH
T2: ccmux calls TIOCSWINSZ on PTY
T3: Child receives SIGWINCH
T4: Child queries TIOCGWINSZ
T5: Child redraws

BUG: If T5 starts before T2 completes, child uses old dimensions
```

**Solution:** Debounce resizes and ensure TIOCSWINSZ completes before delivering SIGWINCH to child.

---

## 3. Terminal Emulation

### 3.1 vt100 Crate Limitations

**Severity: HIGH**

The `vt100` crate (0.15) used for terminal parsing has gaps:

| Feature | Supported | Impact |
|---------|-----------|--------|
| Basic ANSI | ✅ Yes | - |
| 256 colors | ✅ Yes | - |
| True color (24-bit) | ⚠️ Partial | Colors may be wrong |
| Sixel graphics | ❌ No | No inline images |
| OSC 52 (clipboard) | ❌ No | Clipboard broken |
| kitty keyboard | ❌ No | Modifier keys ambiguous |

**Alternative:** `alacritty_terminal` crate has better coverage but is much larger.

---

### 3.2 Escape Sequence Splitting

**Severity: MEDIUM**

Escape sequences can span multiple read() calls:

```
Read 1: "Hello \x1b[38"      // Partial CSI sequence
Read 2: ";5;196mWorld"       // Rest of sequence + content
```

**Naive parsers** will interpret `\x1b[38` as a complete (invalid) sequence, corrupting output.

**Solution:** Buffer input until sequence is complete:
- CSI sequences end with byte in range 0x40-0x7E
- OSC sequences end with BEL (0x07) or ST (ESC \)

---

## 4. Concurrency Patterns

### 4.1 Event Loop Starvation

**Severity: HIGH**

**Scenario:** User runs `cat /dev/urandom` in a pane.

1. PTY reader task monopolizes CPU processing gigabytes of output
2. Other tasks (input handling, resize, UI) starve
3. User cannot Ctrl+C to stop the process
4. UI freezes

**Solutions:**
1. **Cooperative yielding:** Call `tokio::task::yield_now()` every N KB
2. **Rate limiting:** Cap output processing to M MB/s
3. **Priority scheduling:** Use `tokio::select!` with biased selection

---

### 4.2 ArcSwap vs Mutex Trade-offs

**Severity: MEDIUM**

| Pattern | Read Latency | Write Latency | Contention Behavior |
|---------|--------------|---------------|---------------------|
| `Mutex<HashMap>` | ~100ns | ~100ns | Blocks readers during write |
| `ArcSwap<HashMap>` | ~50ns | ~1ms (clone!) | Writers clone entire map |
| `DashMap` | ~80ns | ~80ns | Per-shard locking |

**For ccmux:**
- Session list: `DashMap` (frequent reads, rare writes)
- Scrollback: `ArcSwap` per-pane (frequent writes, frequent reads)
- Config: `ArcSwap` (rare writes, frequent reads)

---

## 5. Memory Safety Risks

### 5.1 libc FFI Hazards

**Severity: CRITICAL**

| Hazard | Example | Mitigation |
|--------|---------|------------|
| Null pointer | `ptsname()` returns NULL on error | Always check return value |
| Uninitialized memory | `tcgetattr()` writes to uninitialized struct | Use `mem::zeroed()` |
| Buffer overflow | `ptsname()` static buffer overrun | Use `ptsname_r()` |
| Use after free | `ptsname()` result reused after second call | Copy result immediately |

---

## 6. Recommended Implementation

### Minimal Safe PTY Layer

```rust
pub struct SafePty {
    master_fd: OwnedFd,
    slave_path: PathBuf,
}

impl SafePty {
    pub fn new() -> Result<Self, Error> {
        // 1. Open PTY with CLOEXEC
        let master = posix_openpt_cloexec()?;

        // 2. Grant and unlock
        grantpt(&master)?;
        unlockpt(&master)?;

        // 3. Get slave name (thread-safe version)
        let slave_path = ptsname_r(&master)?;

        Ok(Self {
            master_fd: master,
            slave_path,
        })
    }

    pub fn spawn_child<F>(self, setup: F) -> Result<Child, Error>
    where
        F: FnOnce() -> Result<(), Error>,
    {
        match unsafe { fork() }? {
            ForkResult::Parent { child } => {
                // Parent keeps master_fd
                Ok(Child { pid: child, master: self.master_fd })
            }
            ForkResult::Child => {
                // Child sets up PTY as controlling terminal
                setsid()?;
                let slave = open(&self.slave_path, O_RDWR)?;
                ioctl_tiocsctty(slave)?;
                dup2(slave, 0)?;
                dup2(slave, 1)?;
                dup2(slave, 2)?;
                setup()?;
                unreachable!("exec should not return")
            }
        }
    }
}
```

---

## References

- [POSIX PTY specification](https://pubs.opengroup.org/onlinepubs/9699919799/)
- [Linux PTY documentation](https://man7.org/linux/man-pages/man7/pty.7.html)
- [Windows ConPTY API](https://docs.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session)
- [portable-pty crate](https://crates.io/crates/portable-pty)
- [Signal safety (man 7 signal-safety)](https://man7.org/linux/man-pages/man7/signal-safety.7.html)
