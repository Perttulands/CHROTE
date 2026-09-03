package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// Every git CHROTE runs goes through this file.
//
// The server runs as its own account while the repositories it reads - the
// Library's corpus, the operator's working trees - belong to the operator. Git
// refuses to read a repository owned by another user unless that exact
// directory is named as safe, and it refuses by failing, which a route that
// reads failure as absence reports as "no history" rather than "git would not
// look". So every invocation names its own repository, and a failure keeps
// git's own words for the route to show and the log to hold.

// gitArguments is one git invocation inside repository. safe.directory leads,
// and names the resolved root itself: never the wildcard, which would trust
// every repository on the host instead of the one the caller already resolved.
func gitArguments(repository string, args ...string) []string {
	return append([]string{
		"-c", "safe.directory=" + repository,
		"--no-pager",
		"-C", repository,
	}, args...)
}

// runGitCommand runs one git command inside repository and returns at most
// limit bytes of its standard output, whether that output was cut short, and
// what went wrong. A failure carries git's own standard error and is logged
// against the repository it was about, so a host fault that empties a route is
// visible both in the response and on the server.
func runGitCommand(ctx context.Context, repository string, limit int, args ...string) (string, bool, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", false, errors.New("git is not installed on the server")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &boundedOutput{limit: limit, onOverflow: cancel}
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, gitPath, gitArguments(repository, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	cmd.Stdout = output
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	// Output stopped at the bound is the caller's answer, not a failure: the
	// process was killed on purpose once it had said enough.
	if output.truncated {
		return output.buffer.String(), true, nil
	}
	if runErr == nil {
		return output.buffer.String(), false, nil
	}

	said := strings.TrimSpace(stderr.String())
	if said == "" {
		said = runErr.Error()
	}
	failure := fmt.Errorf("git %s: %s", gitSubcommand(args), said)
	log.Printf("Repository %s: %v", repository, failure)
	return "", false, failure
}

// gitSubcommand is what git was asked to do, for the message a failure carries.
// The -c options a caller passes are configuration rather than the verb.
func gitSubcommand(args []string) string {
	for index := 0; index < len(args); index++ {
		if args[index] == "-c" {
			index++
			continue
		}
		if !strings.HasPrefix(args[index], "-") {
			return args[index]
		}
	}
	return "command"
}
