package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// osGetwd is the production working-directory accessor (a package-level seam so
// commands can inject a deterministic cwd in tests).
var osGetwd = os.Getwd

// errNotGitRepo is the sentinel returned by gitToplevel when cwd is not inside a
// git work tree (git ran and reported so, or git is not installed). Commands map
// it to their own project-scope "not a git repository" usage error.
var errNotGitRepo = errors.New("not a git repository")

// gitToplevel returns the absolute work-tree root for cwd via an offline
// `git rev-parse --show-toplevel` (reads local .git, no network). It is the one
// repo-root helper both add and verify dispatch to. A clean non-zero git exit
// (cwd is not a repository) or a missing git binary is reported as errNotGitRepo
// — a project-scope precondition the caller renders as navigation; any other
// failure (e.g. context cancellation) is returned verbatim so callers never
// silently treat an unexpected error as "not a repo".
func gitToplevel(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) || errors.Is(err, exec.ErrNotFound) {
			return "", errNotGitRepo
		}

		return "", err
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errNotGitRepo
	}

	return root, nil
}

// usageCannotGetwd builds the shared "cannot determine working directory" usage
// error (exit 1) every command returns when os.Getwd fails. It is errors-as-
// navigation like the rest: the what, the real cause (why), and an actionable
// fix — a single implementation so the fix line cannot drift per command. The
// raw cause is preserved for --verbose.
func usageCannotGetwd(cause error) *UsageError {
	return &UsageError{
		Msg: "cannot determine working directory\n" +
			"why: " + cause.Error() + "\n" +
			"fix: re-run from an existing, readable directory (the shell's current directory may have been removed or be unreadable)",
		Cause: cause,
	}
}

// usageNotGitRepo builds the project-scope "not a git repository" usage error
// (exit 1) shared by add and verify. why states the command-specific rationale;
// the raw cause is preserved for --verbose.
func usageNotGitRepo(why string, cause error) *UsageError {
	return &UsageError{
		Msg: "not a git repository\n" +
			"why: " + why + "\n" +
			"fix: run inside the repo (or git init first)",
		Cause: cause,
	}
}
