package proxy

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// pty is a pseudo-terminal pair. The server keeps the master and hands the
// slave to the attach client as its controlling terminal, which is what makes
// the tmux client behave as if it were on a real tty.
type pty struct {
	master *os.File
	slave  *os.File
}

// openPTY allocates a pseudo-terminal pair. CHROTE only ever runs on Linux, so
// this is the Linux /dev/ptmx sequence rather than a portability layer.
func openPTY() (*pty, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	var index uint32
	if err := ioctl(master, syscall.TIOCGPTN, unsafe.Pointer(&index)); err != nil {
		master.Close()
		return nil, fmt.Errorf("read pty index: %w", err)
	}
	var unlock int32
	if err := ioctl(master, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		master.Close()
		return nil, fmt.Errorf("unlock pty: %w", err)
	}

	name := fmt.Sprintf("/dev/pts/%d", index)
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	return &pty{master: master, slave: slave}, nil
}

// winsize is the kernel's struct winsize.
type winsize struct {
	rows, cols, xPixels, yPixels uint16
}

// resize sets the terminal size. The kernel raises SIGWINCH on the foreground
// process group, which is how the attached tmux client learns its new size.
// Zero or absurd values are refused rather than passed through, because a
// zero-sized terminal makes tmux report a client it cannot draw.
func (p *pty) resize(cols, rows int) error {
	if cols < 1 || rows < 1 || cols > 0xffff || rows > 0xffff {
		return fmt.Errorf("terminal size %dx%d is out of range", cols, rows)
	}
	size := winsize{rows: uint16(rows), cols: uint16(cols)}
	if err := ioctl(p.master, syscall.TIOCSWINSZ, unsafe.Pointer(&size)); err != nil {
		return fmt.Errorf("set terminal size: %w", err)
	}
	return nil
}

// ioctl issues one ioctl through SyscallConn rather than through Fd. Fd puts
// the file into blocking mode permanently, which would leave a goroutine parked
// in Read that Close can no longer interrupt — and interrupting that read is
// exactly how a browser disconnect ends the attach.
func ioctl(file *os.File, request uintptr, argument unsafe.Pointer) error {
	conn, err := file.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	if err := conn.Control(func(fd uintptr) {
		// SAFE: the argument always points at a live local of the type this
		// request expects, and fd is valid for the duration of the call.
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(argument))
	}); err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}
