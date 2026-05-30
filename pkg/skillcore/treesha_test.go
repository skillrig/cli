package skillcore

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTreeSHA_DashRefNotTreatedAsOption guards Qodo #7: a ref beginning with '-'
// (which the shape-only origin validation permits) must be treated as a revision
// via `--end-of-options`, never parsed as a git option — so the failure is a
// bad-revision *GitError, not an "unknown option" error.
func TestTreeSHA_DashRefNotTreatedAsOption(t *testing.T) {
	t.Parallel()

	originDir, skill := bootstrapOrigin(t)

	_, err := TreeSHA(originDir, "-h", "skills/"+skill)

	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("TreeSHA(ref=-h) error = %T (%v), want *GitError", err, err)
	}

	if strings.Contains(strings.ToLower(gitErr.Stderr), "unknown option") {
		t.Errorf("ref '-h' was parsed as a git option (stderr: %q); --end-of-options should prevent this", gitErr.Stderr)
	}
}

// hex40 matches a git SHA-1 (40 lowercase hex chars).
var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// TestTreeSHA_GroundTruth is the Constitution-III ground-truth anchor: it
// asserts skillcore.TreeSHA equals the value raw `git rev-parse HEAD:<path>`
// produces against a bootstrapped fixture. git is the independent oracle (D11) —
// the expected value is never routed through skillcore, so a TreeSHA bug cannot
// hide behind agreeing-but-wrong output.
func TestTreeSHA_GroundTruth(t *testing.T) {
	t.Parallel()

	dir, skill := bootstrapOrigin(t)
	relPath := "skills/" + skill

	// Independent oracle: raw git, not skillcore.
	want := runGit(t, dir, "rev-parse", "HEAD:"+relPath)

	got, err := TreeSHA(dir, "HEAD", relPath)
	if err != nil {
		t.Fatalf("TreeSHA: unexpected error: %v", err)
	}

	if !hex40.MatchString(got) {
		t.Errorf("TreeSHA = %q, want a 40-hex tree SHA", got)
	}

	if got != want {
		t.Errorf("TreeSHA = %q, want (raw git) %q", got, want)
	}
}

// TestTreeSHA_RelocationInvariance proves the fingerprint depends only on the
// subtree contents: the same skill committed at skills/<name> in one repo and at
// .agents/skills/<name> in another yields the identical tree-SHA. This is the
// entire label-honesty mechanism (add records origin's SHA; verify recomputes
// the consumer's and they match).
func TestTreeSHA_RelocationInvariance(t *testing.T) {
	t.Parallel()

	originDir, skill := bootstrapOrigin(t)

	originSHA, err := TreeSHA(originDir, "HEAD", "skills/"+skill)
	if err != nil {
		t.Fatalf("origin TreeSHA: %v", err)
	}

	// A second repo with the SAME files at a DIFFERENT path.
	consumerDir := t.TempDir()
	runGit(t, consumerDir, "init", "-q")
	writeFile(t, consumerDir, filepath.Join(".agents/skills", skill, "SKILL.md"), 0o644, sampleSkillMd)
	writeFile(t, consumerDir, filepath.Join(".agents/skills", skill, "skill.toml"), 0o644, sampleManifest)
	runGit(t, consumerDir, "add", "-A")
	runGit(t, consumerDir, "commit", "-q", "-m", "vendor")

	consumerSHA, err := TreeSHA(consumerDir, "HEAD", ".agents/skills/"+skill)
	if err != nil {
		t.Fatalf("consumer TreeSHA: %v", err)
	}

	if consumerSHA != originSHA {
		t.Errorf("relocation changed the tree SHA: origin %q, consumer %q", originSHA, consumerSHA)
	}
}

// TestTreeSHA_RealGitError exercises the public TreeSHA error path against real
// git: asking for a path absent from HEAD makes rev-parse exit non-zero, and
// TreeSHA must surface a *GitError carrying that positive exit code and stderr.
func TestTreeSHA_RealGitError(t *testing.T) {
	t.Parallel()

	dir, _ := bootstrapOrigin(t)

	_, err := TreeSHA(dir, "HEAD", "skills/does-not-exist")
	if err == nil {
		t.Fatal("TreeSHA: want error for a missing subtree, got nil")
	}

	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("TreeSHA error = %T (%v), want *GitError", err, err)
	}

	if gitErr.ExitCode <= 0 {
		t.Errorf("GitError.ExitCode = %d, want a positive git exit code", gitErr.ExitCode)
	}

	if gitErr.Stderr == "" {
		t.Error("GitError.Stderr is empty, want git's diagnostic text")
	}
}

// TestGitClient_StubbedExit is the explicit "stub the command constructor"
// error-path test (D8): swapping commandContext for a fake binary that exits
// non-zero with known stderr must yield a *GitError carrying exactly that exit
// code and (trimmed) stderr — no real git involved. This is the seam skillcore's
// git layer exposes for deterministic error-path unit tests.
func TestGitClient_StubbedExit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		stderr   string
	}{
		{name: "generic failure", exitCode: 1, stderr: "fatal: not a git repository"},
		{name: "rev-parse bad object", exitCode: 128, stderr: "fatal: bad revision 'HEAD:skills/x'"},
		{name: "stderr is trimmed", exitCode: 2, stderr: "  fatal: boom  \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &gitClient{commandContext: stubCommandContext(tt.exitCode, tt.stderr)}

			_, err := c.run(context.Background(), "rev-parse", "HEAD:skills/x")
			if err == nil {
				t.Fatal("run: want error from non-zero git exit, got nil")
			}

			var gitErr *GitError
			if !errors.As(err, &gitErr) {
				t.Fatalf("run error = %T (%v), want *GitError", err, err)
			}

			if gitErr.ExitCode != tt.exitCode {
				t.Errorf("GitError.ExitCode = %d, want %d", gitErr.ExitCode, tt.exitCode)
			}

			// Stderr is trimmed by the client (errors-as-navigation: the raw
			// cause, tidied) — compare against the trimmed expectation.
			wantStderr := regexp.MustCompile(`^\s+|\s+$`).ReplaceAllString(tt.stderr, "")
			if gitErr.Stderr != wantStderr {
				t.Errorf("GitError.Stderr = %q, want %q", gitErr.Stderr, wantStderr)
			}
		})
	}
}

// TestGitClient_StubbedSuccess confirms the same stub seam round-trips a
// successful invocation: exit 0 yields no error (the stub writes nothing to
// stdout, so the client returns the empty trimmed string).
func TestGitClient_StubbedSuccess(t *testing.T) {
	t.Parallel()

	c := &gitClient{commandContext: stubCommandContext(0, "")}

	out, err := c.run(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("run: unexpected error on exit 0: %v", err)
	}

	if out != "" {
		t.Errorf("run output = %q, want empty (stub wrote no stdout)", out)
	}
}
