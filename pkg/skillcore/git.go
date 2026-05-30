package skillcore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
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

// revParse runs `git -C <gitDir> rev-parse <rev>` and returns the trimmed output
// (e.g. a resolved commit or tree SHA).
//
// A rev never legitimately begins with '-', so one that does is refused up front
// rather than passed to git — where a leading-dash rev (e.g. from an origin ref
// like "-h", which the shape-only origin validation permits) would be parsed as
// an option. git rev-parse cannot be made safe here with `--`/`--end-of-options`
// (it echoes those tokens to stdout instead of treating them as terminators), so
// the guard is the correct fix for this option-injection vector (Qodo #7).
func (c *gitClient) revParse(gitDir, rev string) (string, error) {
	if strings.HasPrefix(rev, "-") {
		return "", &GitError{
			ExitCode: -1,
			Stderr:   "refusing to use a revision that begins with '-': " + rev,
		}
	}

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

// authConfigArgs returns the global `-c http.extraHeader=...` git arguments that
// inject token as an HTTP Basic credential. The token never appears in the clone
// URL or process URL — only in a config value passed via `-c` — mirroring gh's
// transport (research D4). An empty token yields no args (unauthenticated fetch).
//
// These are GLOBAL flags, so callers MUST place them before the git subcommand.
func authConfigArgs(token string) []string {
	if token == "" {
		return []string{}
	}

	// GitHub accepts any non-empty username with a token as the password; the
	// conventional "x-access-token" username matches gh's own header.
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))

	return []string{"-c", "http.extraHeader=Authorization: Basic " + basic}
}

// Clone runs a partial, sparse, no-checkout clone of repoURL into destDir,
// authenticating with token when non-empty (research D7: one git transport for
// both skill subtrees and the catalog). It fetches no blobs and lays down no
// working tree yet — the caller selects paths via FetchSparse-style checkout.
// A non-zero git exit surfaces as *GitError (the stub seam classifies it).
func (c *gitClient) Clone(ctx context.Context, repoURL, destDir, token string) error {
	if strings.HasPrefix(repoURL, "-") {
		return &GitError{
			ExitCode: -1,
			Stderr:   "refusing to clone a URL that begins with '-': " + repoURL,
		}
	}

	args := authConfigArgs(token)
	args = append(
		args,
		"clone",
		"--filter=blob:none",
		"--sparse",
		"--no-checkout",
		"--",
		repoURL,
		destDir,
	)

	_, err := c.run(ctx, args...)

	return err
}

// FetchSparse sparse-checks-out a single skillPath from repoURL at ref into a
// fresh temp dir and returns that dir. It clones (partial + sparse + no-checkout)
// into the temp dir, narrows the sparse cone to skillPath, then checks out ref —
// so only that subtree materializes on disk. token is injected per git
// invocation via -c http.extraHeader when non-empty (research D4/D7).
//
// The returned dir is the caller's to remove. On any git failure the temp dir is
// cleaned up and a *GitError is returned (the stub seam classifies exit/stderr).
func (c *gitClient) FetchSparse(
	ctx context.Context,
	repoURL, skillPath, ref, token string,
) (string, error) {
	if strings.HasPrefix(skillPath, "-") {
		return "", &GitError{
			ExitCode: -1,
			Stderr:   "refusing to use a path that begins with '-': " + skillPath,
		}
	}

	if strings.HasPrefix(ref, "-") {
		return "", &GitError{
			ExitCode: -1,
			Stderr:   "refusing to use a ref that begins with '-': " + ref,
		}
	}

	tmpDir, err := os.MkdirTemp("", "skillrig-fetch-*")
	if err != nil {
		return "", err
	}

	if err := c.fetchSparseInto(ctx, tmpDir, repoURL, skillPath, ref, token); err != nil {
		// Best-effort cleanup; the git failure is the error worth surfacing.
		_ = os.RemoveAll(tmpDir)

		return "", err
	}

	return tmpDir, nil
}

// fetchSparseInto performs the three git steps of FetchSparse against an existing
// dir, keeping FetchSparse's temp-dir lifecycle (create/cleanup) separate from the
// git sequence so the error path has a single cleanup site.
func (c *gitClient) fetchSparseInto(
	ctx context.Context,
	dir, repoURL, skillPath, ref, token string,
) error {
	if err := c.Clone(ctx, repoURL, dir, token); err != nil {
		return err
	}

	auth := authConfigArgs(token)

	sparseArgs := append([]string{}, auth...)
	sparseArgs = append(sparseArgs, "-C", dir, "sparse-checkout", "set", "--", skillPath)

	if _, err := c.run(ctx, sparseArgs...); err != nil {
		return err
	}

	checkoutArgs := append([]string{}, auth...)
	checkoutArgs = append(checkoutArgs, "-C", dir, "checkout", ref)

	_, err := c.run(ctx, checkoutArgs...)

	return err
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

// ResolveGitHubToken resolves a GitHub token for hostname, mirroring gh's own
// precedence (research D4): GH_TOKEN env → GITHUB_TOKEN env → `gh auth token
// --hostname <hostname>`. It returns (token, true) on the first non-empty source
// and ("", false) when none yields a token. Absence is never fatal: gh missing
// from PATH or exiting non-zero (no session) is a clean skip, not an error — an
// unauthenticated fetch is still valid for a public origin.
//
// hostname is the seam for GitHub Enterprise; today callers pass "github.com".
func ResolveGitHubToken(hostname string) (string, bool) {
	if token := strings.TrimSpace(os.Getenv("GH_TOKEN")); token != "" {
		return token, true
	}

	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		return token, true
	}

	return ghAuthToken(hostname)
}

// ghAuthToken shells `gh auth token --hostname <hostname>` to surface a
// keyring-stored token that reading hosts.yml directly would miss. gh absent from
// PATH, or any non-zero exit (no authenticated session), is a skip — ("", false)
// — never a fatal error.
func ghAuthToken(hostname string) (string, bool) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return "", false
	}

	var stdout bytes.Buffer

	//nolint:gosec // G204: fixed `gh auth token` argv; hostname is the caller-controlled host seam (today "github.com"), never untrusted input.
	cmd := exec.CommandContext(context.Background(), ghPath, "auth", "token", "--hostname", hostname)
	cmd.Stdout = &stdout
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return "", false
	}

	token := strings.TrimSpace(stdout.String())
	if token == "" {
		return "", false
	}

	return token, true
}

// FetchSparse sparse-checks-out skillPath from repoURL at ref into a fresh temp
// dir using the default client (the real git binary), authenticating with token
// when non-empty. It is the package-level entry point add's remote path
// dispatches to; the client method underneath stays pluggable for unit tests.
func FetchSparse(
	ctx context.Context,
	repoURL, skillPath, ref, token string,
) (string, error) {
	return newGitClient().FetchSparse(ctx, repoURL, skillPath, ref, token)
}
