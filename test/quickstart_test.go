// Package quickstart holds the TestQuickstart_* integration suite. Per
// Constitution II (Quickstart-as-Contract) each scenario in
// specledger/001-init-origin-resolution/quickstart.md maps 1:1 to a test here.
// CLI scenarios build the real binary once (TestMain) and exec it in an
// isolated temp HOME/cwd with SKILLRIG_ORIGIN supplied per scenario.
package quickstart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binPath is the built skillrig binary, shared across all scenarios.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "skillrig-bin-*")
	if err != nil {
		panic(err)
	}

	binPath = filepath.Join(dir, "skillrig")

	build := exec.CommandContext(context.Background(), "go", "build", "-o", binPath, ".")
	build.Dir = ".." // module root, relative to this package dir (test/)

	if out, err := build.CombinedOutput(); err != nil {
		_, _ = os.Stderr.WriteString("build failed: " + err.Error() + "\n" + string(out))

		os.Exit(1)
	}

	code := m.Run()

	_ = os.RemoveAll(dir)

	os.Exit(code)
}

// runOpts configures one invocation of the binary.
type runOpts struct {
	args  []string
	cwd   string            // working directory (required for write scenarios)
	home  string            // HOME / XDG base (defaults to a fresh temp dir)
	env   map[string]string // extra env, e.g. SKILLRIG_ORIGIN
	stdin string            // piped to the child (always a pipe, never a TTY)
}

// runResult captures the observable contract: stdout, stderr, exit code.
type runResult struct {
	stdout string
	stderr string
	exit   int
}

// run execs the binary with isolated HOME/XDG so project and global config
// never touch the real user environment. stdin is always a pipe (never a char
// device), so the binary's TTY detection classifies every scenario here as
// non-interactive — the interactive prompt path is covered in-process
// (internal/cli, the quickstart's sanctioned "interactive shim").
func run(t *testing.T, opts runOpts) runResult {
	t.Helper()

	home := opts.home
	if home == "" {
		home = t.TempDir()
	}

	cwd := opts.cwd
	if cwd == "" {
		cwd = t.TempDir()
	}

	cmd := exec.CommandContext(t.Context(), binPath, opts.args...)
	cmd.Dir = cwd
	// Build a clean env: keep PATH (git lookup) but never inherit the tester's
	// SKILLRIG_ORIGIN. HOME + XDG_CONFIG_HOME are pinned to the temp home.
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
	}
	for k, v := range opts.env {
		env = append(env, k+"="+v)
	}

	cmd.Env = env
	cmd.Stdin = strings.NewReader(opts.stdin)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exit := 0

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
		} else {
			t.Fatalf("exec %v: %v", opts.args, err)
		}
	}

	return runResult{stdout: stdout.String(), stderr: stderr.String(), exit: exit}
}

// nonEmptyLines splits s on newlines and drops the trailing empty element so a
// final "\n" does not inflate the line count for shape assertions.
func nonEmptyLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// requireGit skips a scenario with a clear message when git is unavailable. git
// is a declared required dependency (plan.md), but the suite stays honest where
// it is missing rather than failing opaquely.
func requireGit(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping git-root write-target scenario")
	}
}

// gitInit initialises a throwaway repo in dir (offline, quiet).
func gitInit(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", "init", "-q")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s: %v\n%s", dir, err, out)
	}
}

// realPath resolves symlinks (macOS /var → /private/var) so path assertions
// match git rev-parse --show-toplevel output.
func realPath(t *testing.T, p string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", p, err)
	}

	return resolved
}

func TestQuickstart_BindProject(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	res := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills"}, cwd: cwd})

	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	lines := nonEmptyLines(res.stdout)
	if len(lines) > 2 {
		t.Errorf("stdout has %d lines, want <= 2:\n%s", len(lines), res.stdout)
	}

	if !strings.Contains(lines[0], "my-org/my-skills") || !strings.Contains(lines[0], "project") {
		t.Errorf("line 1 = %q, want it to mention bound origin + project", lines[0])
	}

	if !strings.Contains(res.stdout, "→ resolve order:") {
		t.Errorf("stdout missing resolve-order footer hint:\n%s", res.stdout)
	}

	gotFile := readFile(t, filepath.Join(cwd, ".skillrig", "config.toml"))
	wantFile := readFile(t, filepath.Join("fixtures", "config.toml"))

	if gotFile != wantFile {
		t.Errorf("config.toml = %q, want fixture %q", gotFile, wantFile)
	}
}

func TestQuickstart_BindProjectJSON(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	res := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills", "--json"}, cwd: cwd})

	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &obj); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\n%s", err, res.stdout)
	}

	for _, key := range []string{"ok", "origin", "scope", "configPath", "written"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("JSON missing key %q: %v", key, obj)
		}
	}

	if obj["ok"] != true {
		t.Errorf("ok = %v, want true", obj["ok"])
	}

	if obj["origin"] != "my-org/my-skills" {
		t.Errorf("origin = %v, want my-org/my-skills", obj["origin"])
	}

	if obj["scope"] != "project" {
		t.Errorf("scope = %v, want project", obj["scope"])
	}

	if obj["written"] != true {
		t.Errorf("written = %v, want true", obj["written"])
	}
}

// TestQuickstart_BindWithRef maps to quickstart BindWithRef (amendment
// 001-origin-ref-support): an origin may carry an optional @REF (a branch) and
// it is recorded combined in the single `origin` key.
func TestQuickstart_BindWithRef(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	res := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills@staging"}, cwd: cwd})

	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	lines := nonEmptyLines(res.stdout)
	if len(lines) > 2 {
		t.Errorf("stdout has %d lines, want <= 2:\n%s", len(lines), res.stdout)
	}

	if !strings.Contains(lines[0], "my-org/my-skills@staging") || !strings.Contains(lines[0], "project") {
		t.Errorf("line 1 = %q, want it to mention bound origin (incl. @staging) + project", lines[0])
	}

	if !strings.Contains(res.stdout, "→ resolve order:") {
		t.Errorf("stdout missing resolve-order footer hint:\n%s", res.stdout)
	}

	got := readFile(t, filepath.Join(cwd, ".skillrig", "config.toml"))
	if got != "origin = 'my-org/my-skills@staging'\n" {
		t.Errorf("config.toml = %q, want origin = 'my-org/my-skills@staging'", got)
	}

	jsonRes := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills@staging", "--json"}, cwd: cwd})

	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonRes.stdout), &obj); err != nil {
		t.Fatalf("json variant: %v\n%s", err, jsonRes.stdout)
	}

	// JSON must be structurally complete (Constitution II), not just carry the ref.
	for _, key := range []string{"ok", "origin", "scope", "configPath", "written"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("JSON missing key %q: %v", key, obj)
		}
	}

	if obj["origin"] != "my-org/my-skills@staging" {
		t.Errorf("origin = %v, want my-org/my-skills@staging", obj["origin"])
	}

	if obj["scope"] != "project" {
		t.Errorf("scope = %v, want project", obj["scope"])
	}
}

func TestQuickstart_IdempotentRebind(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	first := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills"}, cwd: cwd})

	if first.exit != 0 {
		t.Fatalf("first run exit = %d, want 0 (stderr: %s)", first.exit, first.stderr)
	}

	before := readFile(t, filepath.Join(cwd, ".skillrig", "config.toml"))

	second := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills"}, cwd: cwd})
	if second.exit != 0 {
		t.Fatalf("second run exit = %d, want 0 (stderr: %s)", second.exit, second.stderr)
	}

	if !strings.Contains(second.stdout, "already bound") && !strings.Contains(second.stdout, "no change") {
		t.Errorf("second run stdout should note no change, got:\n%s", second.stdout)
	}

	after := readFile(t, filepath.Join(cwd, ".skillrig", "config.toml"))
	if before != after {
		t.Errorf("file changed on idempotent rebind:\n before=%q\n after =%q", before, after)
	}

	jsonRes := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills", "--json"}, cwd: cwd})

	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonRes.stdout), &obj); err != nil {
		t.Fatalf("json variant: %v\n%s", err, jsonRes.stdout)
	}

	if obj["written"] != false {
		t.Errorf("written = %v on idempotent rebind, want false", obj["written"])
	}
}

func TestQuickstart_RebindDifferent(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills"}, cwd: cwd})

	res := run(t, runOpts{args: []string{"init", "--origin", "other-org/other-skills"}, cwd: cwd})
	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	got := readFile(t, filepath.Join(cwd, ".skillrig", "config.toml"))
	want := "origin = 'other-org/other-skills'\n"

	if got != want {
		t.Errorf("after rebind config = %q, want %q (cleanly replaced)", got, want)
	}
}

func TestQuickstart_Global(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := t.TempDir()
	res := run(t, runOpts{
		args: []string{"init", "--origin", "my-org/my-skills", "--global", "--json"},
		cwd:  cwd,
		home: home,
	})

	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	globalPath := filepath.Join(home, ".config", "skillrig", "config.toml")
	if readFile(t, globalPath) != readFile(t, filepath.Join("fixtures", "config.toml")) {
		t.Errorf("global config at %s does not equal fixture", globalPath)
	}

	if _, err := os.Stat(filepath.Join(cwd, ".skillrig", "config.toml")); !os.IsNotExist(err) {
		t.Errorf("project config should not exist for --global, stat err = %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &obj); err != nil {
		t.Fatalf("json: %v\n%s", err, res.stdout)
	}

	if obj["scope"] != "global" {
		t.Errorf("scope = %v, want global", obj["scope"])
	}
}

func TestQuickstart_Help(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"init", "--help"}})
	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0", res.exit)
	}

	examples := strings.Count(res.stdout, "skillrig init")
	if examples < 2 {
		t.Errorf("help shows %d 'skillrig init' lines, want >= 2 examples:\n%s", examples, res.stdout)
	}
}

func TestQuickstart_BindFromGitSubdir(t *testing.T) {
	t.Parallel()
	requireGit(t)

	repo := t.TempDir()
	gitInit(t, repo)

	sub := filepath.Join(repo, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	res := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills"}, cwd: sub})
	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	rootCfg := filepath.Join(realPath(t, repo), ".skillrig", "config.toml")
	if _, err := os.Stat(rootCfg); err != nil {
		t.Errorf("config not written at git root %s: %v", rootCfg, err)
	}

	if _, err := os.Stat(filepath.Join(sub, ".skillrig")); !os.IsNotExist(err) {
		t.Errorf("no .skillrig should be created under subdir, stat err = %v", err)
	}
}

func TestQuickstart_BindNonGitCwdFallback(t *testing.T) {
	t.Parallel()

	// A fresh temp dir is not inside a git repo, so the write target falls back
	// to cwd/.skillrig/config.toml.
	cwd := t.TempDir()
	res := run(t, runOpts{args: []string{"init", "--origin", "my-org/my-skills"}, cwd: cwd})

	if res.exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if _, err := os.Stat(filepath.Join(cwd, ".skillrig", "config.toml")); err != nil {
		t.Errorf("config not written at cwd fallback: %v", err)
	}
}

func TestQuickstart_MalformedOrigin(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	res := run(t, runOpts{args: []string{"init", "--origin", "not-a-valid-origin"}, cwd: cwd})

	if res.exit != 1 {
		t.Errorf("exit = %d, want 1", res.exit)
	}

	if res.stdout != "" {
		t.Errorf("stdout = %q, want empty", res.stdout)
	}

	// Three distinct parts (what / why / fix), each asserted separately.
	if !strings.Contains(res.stderr, "not-a-valid-origin") {
		t.Errorf("stderr (what) should echo the offending value, got: %q", res.stderr)
	}

	if !strings.Contains(res.stderr, "OWNER/REPO") {
		t.Errorf("stderr (why) should state expected OWNER/REPO, got: %q", res.stderr)
	}

	if !strings.Contains(res.stderr, "my-org/my-skills") {
		t.Errorf("stderr (fix) should show a concrete example, got: %q", res.stderr)
	}

	if _, err := os.Stat(filepath.Join(cwd, ".skillrig", "config.toml")); !os.IsNotExist(err) {
		t.Errorf("no config should be written on malformed origin, stat err = %v", err)
	}
}

func TestQuickstart_NoOriginNonInteractive(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	// stdin is a pipe (not a char device) → non-interactive session, no TTY.
	res := run(t, runOpts{args: []string{"init"}, cwd: cwd, stdin: ""})

	if res.exit != 1 {
		t.Errorf("exit = %d, want 1", res.exit)
	}

	if res.stdout != "" {
		t.Errorf("stdout = %q, want empty", res.stdout)
	}

	if !strings.Contains(res.stderr, "no origin") {
		t.Errorf("stderr (what) should say no origin given, got: %q", res.stderr)
	}

	if !strings.Contains(res.stderr, "no TTY") {
		t.Errorf("stderr (why) should cite non-interactive session/no TTY, got: %q", res.stderr)
	}

	if !strings.Contains(res.stderr, "--origin") || !strings.Contains(res.stderr, "SKILLRIG_ORIGIN") {
		t.Errorf("stderr (fix) should suggest --origin or SKILLRIG_ORIGIN, got: %q", res.stderr)
	}

	if _, err := os.Stat(filepath.Join(cwd, ".skillrig", "config.toml")); !os.IsNotExist(err) {
		t.Errorf("no config should be written, stat err = %v", err)
	}
}

func TestQuickstart_NonInteractiveFlag(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	res := run(t, runOpts{args: []string{"init", "--non-interactive"}, cwd: cwd})

	if res.exit != 1 {
		t.Errorf("exit = %d, want 1", res.exit)
	}

	if !strings.Contains(res.stderr, "no origin") {
		t.Errorf("stderr (what) should say no origin given, got: %q", res.stderr)
	}

	if !strings.Contains(res.stderr, "--non-interactive") {
		t.Errorf("stderr (why) should cite --non-interactive, got: %q", res.stderr)
	}

	if !strings.Contains(res.stderr, "--origin") || !strings.Contains(res.stderr, "SKILLRIG_ORIGIN") {
		t.Errorf("stderr (fix) should suggest --origin or SKILLRIG_ORIGIN, got: %q", res.stderr)
	}

	// FR-006c: forced fail-fast must not emit the prompt.
	if strings.Contains(res.stderr, "Origin (OWNER/REPO):") {
		t.Errorf("--non-interactive must not prompt, but stderr contains the prompt: %q", res.stderr)
	}
}

// readFile reads a file and fails the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}
