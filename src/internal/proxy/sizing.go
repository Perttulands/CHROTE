package proxy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// A tmux window is drawn once, at one size, however many clients are watching
// it. tmux keeps two things separate that CHROTE used to conflate: how many
// clients may attach, and how many may set the size. Only the second has to be
// exclusive, and the `ignore-size` client flag is what makes it so
// (ADR-0017 decision 1).
//
// The flag is per client and behaves as measured on tmux 3.6a:
//
//   - Of several attached clients, the ones without the flag decide the window
//     size; a flagged client watches at whatever size they set.
//   - A client that is the *only* candidate sizes the window whether it carries
//     the flag or not, and keeps tracking its own resizes. So the flag cannot be
//     used to make a window nobody sizes.
//   - When every attached client carries the flag, tmux falls back to sizing by
//     the most recent one. That is the `window-size latest` flapping this model
//     exists to stop, which is why exactly one client must be left unflagged.
//   - `refresh-client -f` toggles the named flags and leaves the rest alone, so
//     setting and clearing `ignore-size` touches nothing else about a client.
const ignoreSizeFlag = "ignore-size"

// clientListFormat pairs each attached client with its flags. A client with no
// tty, such as the transient control-mode client that sizes a new session,
// cannot be addressed by `refresh-client -t` and is skipped.
const clientListFormat = "#{client_tty}\t#{client_flags}"

// sizingClientTTYs reports the clients that currently set the size of a
// session's window: everything attached to it without the `ignore-size` flag,
// CHROTE's own clients and foreign ones alike.
func sizingClientTTYs(socket, session string) ([]string, error) {
	output, err := runTmux(socket, "list-clients", "-t", session, "-F", clientListFormat)
	if err != nil {
		return nil, err
	}
	return sizingClientTTYsFrom(output), nil
}

func sizingClientTTYsFrom(output string) []string {
	var sizing []string
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		tty, flags, found := strings.Cut(strings.TrimSuffix(line, "\r"), "\t")
		if !found || tty == "" {
			continue
		}
		if hasClientFlag(flags, ignoreSizeFlag) {
			continue
		}
		sizing = append(sizing, tty)
	}
	return sizing
}

// hasClientFlag reports whether a tmux client_flags list carries a flag. The
// list is comma separated and mixes settable flags with status ones such as
// `attached`, so it is compared element by element rather than by substring.
func hasClientFlag(flags, wanted string) bool {
	for _, flag := range strings.Split(flags, ",") {
		if strings.TrimSpace(flag) == wanted {
			return true
		}
	}
	return false
}

// claimSizing makes ourTTY the session's one sizing client, without detaching
// anybody: every other sizing client is flagged first, and only then is our own
// flag cleared. Doing it the other way round would leave two unflagged clients
// for as long as the first command takes, which is the state that flaps.
//
// Clients come and go while this runs, so a client that has left by the time it
// is addressed is not a failure — the window it was sizing is gone with it.
func claimSizing(socket, session, ourTTY string) error {
	if strings.TrimSpace(ourTTY) == "" {
		return errors.New("this terminal has no tty to claim the session with")
	}
	sizing, err := sizingClientTTYs(socket, session)
	if err != nil {
		return err
	}
	alreadyOurs := false
	for _, tty := range sizing {
		if tty == ourTTY {
			alreadyOurs = true
			continue
		}
		if _, err := runTmux(socket, "refresh-client", "-t", tty, "-f", ignoreSizeFlag); err != nil {
			if isMissingClient(err) {
				continue
			}
			return fmt.Errorf("hand the size of session %q over from client %s: %w", session, tty, err)
		}
	}
	if alreadyOurs && len(sizing) == 1 {
		return nil
	}
	if _, err := runTmux(socket, "refresh-client", "-t", ourTTY, "-f", "!"+ignoreSizeFlag); err != nil {
		return fmt.Errorf("take the size of session %q: %w", session, err)
	}
	return nil
}

// isMissingClient reports the one tmux failure claiming may ignore: the client
// detached between being listed and being addressed.
func isMissingClient(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "can't find client")
}

// runTmux runs one bounded tmux command on a configured socket and returns its
// output. It reads and flags clients; it never creates or destroys a session.
func runTmux(socket string, args ...string) (string, error) {
	if strings.TrimSpace(socket) == "" {
		return "", errors.New("no tmux socket is configured for this terminal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), tmuxProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, core.TmuxBin(), append([]string{"-S", socket}, args...)...)
	cmd.Env = attachEnv()
	output, err := cmd.Output()
	if err != nil {
		detail := ""
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), detail)
	}
	return string(output), nil
}

// tmuxProbeTimeout bounds every tmux command on the terminal path. A tmux
// server that does not answer is a failure to report, not something to wait on
// forever.
const tmuxProbeTimeout = 5 * time.Second
