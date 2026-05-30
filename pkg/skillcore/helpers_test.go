package skillcore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// pinnedGitEnv is a fully reproducible author/committer identity+date (research
// D8): with it the commit SHA is deterministic; tests that only need a
// well-formed commit can ignore it, but pinning keeps the fixtures stable.
func pinnedGitEnv() []string {
	const stamp = "2026-01-01T00:00:00Z"

	return append(os.Environ(),
		"GIT_AUTHOR_NAME=skillrig",
		"GIT_AUTHOR_EMAIL=ci@skillrig.dev",
		"GIT_AUTHOR_DATE="+stamp,
		"GIT_COMMITTER_NAME=skillrig",
		"GIT_COMMITTER_EMAIL=ci@skillrig.dev",
		"GIT_COMMITTER_DATE="+stamp,
		// Neutralize any ambient user config so the fixture is hermetic.
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
}

// runGit execs the real git binary in dir with the pinned identity and fails
// the test on any error. It is the independent oracle (research D11): the tests
// drive setup and compute expected values through raw git, never through
// skillcore, so a TreeSHA bug cannot hide behind matching-but-wrong output.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = pinnedGitEnv()

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return strings.TrimSpace(string(out))
}

// writeFile writes content under dir/rel (creating parents) with the given
// mode, failing the test on error.
func writeFile(t *testing.T, dir, rel string, mode os.FileMode, content string) {
	t.Helper()

	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// sampleManifest is a representative skill.toml carrying [[requires]] and an
// UNKNOWN key, mirroring the data-model sample. Content is illustrative (D12):
// the tests recompute every expected SHA independently, so it may change freely.
const sampleManifest = `name = "terraform-plan-review"
version = "1.4.0"
namespace = "dev.skillrig.samples"
description = "Review a terraform plan"
tags = ["terraform", "review"]

# Unknown top-level key — must be ignored (forward-compat).
experimental = true

[[requires]]
tool = "terraform"
version = ">=1.5"
source = "https://releases.hashicorp.com"
manager = "asdf"

[[requires]]
tool = "tflint"
version = "0.50.0"
# Unknown per-require key — must also be ignored.
optional = true
`

const sampleSkillMd = "# terraform-plan-review\n\nReview a terraform plan.\n"

// bootstrapOrigin creates a real git repo in a fresh tmpDir containing a single
// committed skill at skills/<name>/, returning the repo dir and skill name. The
// commit is reproducible (pinned identity), and the subtree tree-SHA is
// content-only and therefore deterministic.
func bootstrapOrigin(t *testing.T) (dir, skill string) {
	t.Helper()

	dir = t.TempDir()
	skill = "terraform-plan-review"

	runGit(t, dir, "init", "-q")

	writeFile(t, dir, filepath.Join("skills", skill, "SKILL.md"), 0o644, sampleSkillMd)
	writeFile(t, dir, filepath.Join("skills", skill, "skill.toml"), 0o644, sampleManifest)
	// A non-skill origin file, to prove add ignores everything outside skills/<name>.
	writeFile(t, dir, ".skillrig-origin.toml", 0o644, "convention_version = 1\nskills_dir = \"skills\"\n")

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "seed sample skill")

	return dir, skill
}

// The helper-process pattern (Go stdlib's os/exec test idiom): TestHelperProcess
// is a real test that, when invoked with GO_WANT_HELPER_PROCESS=1, impersonates
// the git binary. stubCommandContext returns a commandContext that re-execs the
// test binary into this function, letting unit tests simulate an exact git exit
// code + stderr without a real git invocation.
func stubCommandContext(exitCode int, stderr string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		// Re-exec this test binary, routing into TestHelperProcess.
		csArgs := append([]string{"-test.run=TestHelperProcess", "--"}, args...)

		cmd := exec.CommandContext(ctx, os.Args[0], csArgs...)
		cmd.Env = []string{
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_EXIT_CODE=" + strconv.Itoa(exitCode),
			"HELPER_STDERR=" + stderr,
		}

		return cmd
	}
}

// TestHelperProcess is not a real test: it is the stub "git" binary re-exec'd by
// stubCommandContext. It writes HELPER_STDERR to stderr and exits with
// HELPER_EXIT_CODE, so the gitClient under test sees a genuine *exec.ExitError.
// It must be named TestHelperProcess (matched by -test.run) and must not call
// t.Parallel — it impersonates git and calls os.Exit.
//
//nolint:paralleltest // not a real test; the os/exec helper-process re-exec target.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	if msg := os.Getenv("HELPER_STDERR"); msg != "" {
		fmt.Fprint(os.Stderr, msg)
	}

	code, _ := strconv.Atoi(os.Getenv("HELPER_EXIT_CODE"))
	os.Exit(code)
}
