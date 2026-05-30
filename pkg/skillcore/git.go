package skillcore

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// gitClient shells the git binary for skillcore's integrity primitives. It is
// modeled on gh-cli's git.Client (research D7): a small, testable wrapper whose
// command constructor is a pluggable field so unit tests can swap in a stub
// while integration tests run real git. It captures stdout/stderr into buffers
// and never writes to os.Stdout/os.Stderr — the CLI owns all presentation.
type gitClient struct {
	// commandContext builds the *exec.Cmd for a git invocation. It defaults to
	// exec.CommandContext; tests override it to return a stubbed command.
	commandContext func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// newGitClient returns a gitClient that shells the real git binary.
func newGitClient() *gitClient {
	return &gitClient{commandContext: exec.CommandContext}
}

// run invokes git with args, capturing stdout and stderr into buffers. On a
// non-zero exit it returns a *GitError carrying the exit code and trimmed
// stderr; on success it returns the trimmed stdout.
func (c *gitClient) run(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer

	cmd := c.commandContext(ctx, "git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// A non-zero git exit surfaces as *exec.ExitError carrying the code;
		// any other failure (e.g. git not on PATH) has no exit code, so we
		// record -1 to signal "git could not be run".
		exitCode := -1

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		return "", &GitError{
			ExitCode: exitCode,
			Stderr:   strings.TrimSpace(stderr.String()),
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// revParse runs `git -C <gitDir> rev-parse <rev>` and returns the trimmed
// output (e.g. a resolved commit or tree SHA).
func (c *gitClient) revParse(gitDir, rev string) (string, error) {
	return c.run(context.Background(), "-C", gitDir, "rev-parse", rev)
}

// statusPorcelain runs `git -C <gitDir> status --porcelain -- <relPath>` and
// returns the trimmed output. Empty output means relPath is clean versus HEAD.
func (c *gitClient) statusPorcelain(gitDir, relPath string) (string, error) {
	return c.run(
		context.Background(),
		"-C", gitDir,
		"status", "--porcelain",
		"--", relPath,
	)
}

// revParse runs `git -C <gitDir> rev-parse <rev>` using the default client (the
// real git binary). It is the package-level entry point TreeSHA dispatches to;
// the client method underneath stays pluggable for skillcore's own unit tests.
func revParse(gitDir, rev string) (string, error) {
	return newGitClient().revParse(gitDir, rev)
}

// statusPorcelain runs `git -C <gitDir> status --porcelain -- <relPath>` using
// the default client. Verify dispatches to it to detect uncommitted changes.
func statusPorcelain(gitDir, relPath string) (string, error) {
	return newGitClient().statusPorcelain(gitDir, relPath)
}
