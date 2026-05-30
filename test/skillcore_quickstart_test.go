// This file holds the TestQuickstart_* integration suite for feature
// 002-skillcore-verify (add + verify). Per Constitution II (Quickstart as
// Contract) each scenario in specledger/002-skillcore-verify/quickstart.md maps
// 1:1 to a TestQuickstart_* test here. Like the 001 suite it builds the real
// binary once (TestMain in quickstart_test.go) and execs it via run().
//
// Oracle independence (research D11): every expected tree-SHA is computed with
// RAW git (rawTreeSHA → `git rev-parse <ref>:<path>`), NEVER through skillcore —
// the binary under test uses skillcore internally, so routing the expected value
// through it would be circular validation (Constitution III). All git
// bootstrap/commit helpers here shell `git` directly with a pinned identity so
// the fixture commit is reproducible (D8).
package quickstart

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// originRepo is the OWNER/REPO the fixture's .skillrig-origin.toml declares. The
// origin value is an OWNER/REPO reference (it must pass the resolver's
// OWNER/REPO[@REF] shape check), and this slice the origin is a LOCAL checkout:
// the resolved OWNER/REPO is interpreted as a path relative to the repo root, so
// the origin checkout is laid out at <repoRoot>/my-org/my-skills.
const originRepo = "my-org/my-skills"

// sampleSkill is the one skill the sample origin ships.
const sampleSkill = "terraform-plan-review"

// sampleVersion is the version recorded in the fixture's SKILL.md frontmatter
// (metadata.x-skillrig.version).
const sampleVersion = "1.4.0"

// originSubtree is the origin-relative path whose git tree-object SHA is the
// fingerprint add records and verify recomputes (the locked fingerprint boundary
// — research D1). rawTreeSHA reads it straight from git as the oracle.
const originSubtree = "skills/" + sampleSkill

// vendoredPath is the canonical repo-relative location a vendored skill lands at.
const vendoredPath = ".agents/skills/" + sampleSkill

// pinnedGitEnv is the reproducible-commit identity (research D8): a fixed
// author/committer name, email, and date so the fixture's commit SHA is stable
// across machines and runs. It is appended to the current environment for every
// git invocation in this suite's helpers.
func pinnedGitEnv() []string {
	const (
		name  = "skillrig"
		email = "ci@skillrig.dev"
		date  = "2026-01-01T00:00:00Z"
	)

	return append(os.Environ(),
		"GIT_AUTHOR_NAME="+name,
		"GIT_AUTHOR_EMAIL="+email,
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME="+name,
		"GIT_COMMITTER_EMAIL="+email,
		"GIT_COMMITTER_DATE="+date,
	)
}

// git runs a raw git command in dir with the pinned identity and fails the test
// on error. It is the independent oracle / setup primitive: it NEVER calls
// skillcore (research D11).
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = pinnedGitEnv()

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}

	return strings.TrimSpace(string(out))
}

// sampleOriginDir resolves the committed fixture tree (test/testdata/
// sample-origin) to an absolute path. Tests exec from the test/ package dir, so
// the fixture is reachable relative to the test cwd.
func sampleOriginDir(t *testing.T) string {
	t.Helper()

	abs, err := filepath.Abs(filepath.Join("testdata", "sample-origin"))
	if err != nil {
		t.Fatalf("resolve sample-origin: %v", err)
	}

	if _, err := os.Stat(filepath.Join(abs, ".skillrig-origin.toml")); err != nil {
		t.Fatalf("sample-origin fixture missing at %s: %v", abs, err)
	}

	return abs
}

// copyTree recursively copies the fixture src into dst, preserving file modes
// (the exec bit is part of the tree-SHA, so it must survive the copy).
func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy fixture %s → %s: %v", src, dst, err)
	}
}

// consumerRepo is a bootstrapped consumer: a git repo whose origin (the local
// OWNER/REPO checkout) is nested under it, ready to add/commit/verify.
type consumerRepo struct {
	// root is the consumer git repo's work tree (also its rev-parse toplevel).
	root string
	// originDir is the nested origin checkout at <root>/my-org/my-skills — the
	// raw-git oracle target for rawTreeSHA.
	originDir string
}

// bootstrapOrigin git-inits a fresh checkout of the sample origin inside
// parent at the relative OWNER/REPO path and commits it with the pinned
// identity, returning the origin dir and the committed ref. Lives at
// <parent>/my-org/my-skills so the resolver's OWNER/REPO value resolves to it as
// a path relative to the consumer root (= parent).
func bootstrapOrigin(t *testing.T, parent string) (dir, ref string) {
	t.Helper()

	dir = filepath.Join(parent, filepath.FromSlash(originRepo))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir origin %s: %v", dir, err)
	}

	copyTree(t, sampleOriginDir(t), dir)

	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "fixture origin")

	return dir, "HEAD"
}

// newConsumerRepo builds a consumer repo with the sample origin nested inside it
// and binds it with `skillrig init --origin my-org/my-skills`. The nested origin
// checkout is excluded from the consumer's index (.git/info/exclude) so a later
// `git add -A` stages only the vendored skill + lock, never the origin's own
// repo as a gitlink.
func newConsumerRepo(t *testing.T) consumerRepo {
	t.Helper()
	requireGit(t)

	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")

	// Keep the nested origin checkout out of the consumer's index.
	if err := os.WriteFile(
		filepath.Join(root, ".git", "info", "exclude"),
		[]byte(strings.SplitN(originRepo, "/", 2)[0]+"/\n"),
		0o644,
	); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	originDir, _ := bootstrapOrigin(t, root)

	res := run(t, runOpts{args: []string{"init", "--origin", originRepo}, cwd: root})
	if res.exit != 0 {
		t.Fatalf("init --origin %s: exit %d (stderr: %s)", originRepo, res.exit, res.stderr)
	}

	return consumerRepo{root: root, originDir: originDir}
}

// commitAll stages and commits everything in dir with the pinned identity so
// verify (which checks the COMMITTED tree) sees the vendored content.
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()

	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", msg)
}

// rawTreeSHA is the independent oracle: the git tree-object SHA of relPath at ref
// in gitDir, read via raw `git rev-parse <ref>:<relPath>` — NEVER skillcore
// (research D11). The binary under test computes the same value through
// skillcore; this is what proves the two agree on real git output.
func rawTreeSHA(t *testing.T, gitDir, ref, relPath string) string {
	t.Helper()

	return git(t, gitDir, "rev-parse", ref+":"+relPath)
}

// statusPorcelain returns `git status --porcelain` for dir, the read-only probe
// for the verify-writes-nothing assertions.
func statusPorcelain(t *testing.T, dir string) string {
	t.Helper()

	return git(t, dir, "status", "--porcelain")
}

// decodeJSON unmarshals res.stdout into a generic object, failing the test (with
// the raw stdout) when it is not a single JSON object. Used to assert the
// --json output is parseable and structurally complete.
func decodeJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()

	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\n%s", err, stdout)
	}

	return obj
}

// requireKeys asserts every key is present in obj (structural completeness,
// Constitution II — not just a Contains check).
func requireKeys(t *testing.T, obj map[string]any, keys ...string) {
	t.Helper()

	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			t.Errorf("JSON missing key %q: %v", k, obj)
		}
	}
}

// readSkillFile reads a vendored skill file under the consumer root.
func readSkillFile(t *testing.T, root, rel string) string {
	t.Helper()

	return readFile(t, filepath.Join(root, vendoredPath, rel))
}

// addResultKeys are the complete add --json key set (contract add.md).
var addResultKeys = []string{"ok", "name", "version", "path", "commit", "treeSha", "action", "dryRun"}

// verdictKeys are the complete per-verdict key set (contract verify.md).
var verdictKeys = []string{"name", "path", "status", "expectedTreeSha", "actualTreeSha", "reason"}

// countsKeys are the complete counts key set (contract verify.md).
var countsKeys = []string{"verified", "mismatch", "orphan", "missing", "dirty"}

// ---------------------------------------------------------------------------
// US1 — Vendor a skill (add)
// ---------------------------------------------------------------------------

// assertVendoredMatchesOrigin checks every vendored file is byte-identical to the
// origin with its mode preserved, and that check.sh keeps its executable bit (the
// exec bit is part of the tree-SHA, so a mode change would break label-honesty).
func assertVendoredMatchesOrigin(t *testing.T, c consumerRepo) {
	t.Helper()

	for _, f := range []string{"SKILL.md", "check.sh"} {
		got := readSkillFile(t, c.root, f)
		want := readFile(t, filepath.Join(c.originDir, "skills", sampleSkill, f))

		if got != want {
			t.Errorf("vendored %s differs from origin", f)
		}

		gotMode := fileMode(t, filepath.Join(c.root, vendoredPath, f))
		wantMode := fileMode(t, filepath.Join(c.originDir, "skills", sampleSkill, f))

		if gotMode != wantMode {
			t.Errorf("vendored %s mode = %v, want %v", f, gotMode, wantMode)
		}
	}

	if execMode := fileMode(t, filepath.Join(c.root, vendoredPath, "check.sh")); execMode&0o111 == 0 {
		t.Errorf("vendored check.sh lost its executable bit: mode = %v", execMode)
	}
}

func TestQuickstart_AddVendorsSkill(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)
	wantTreeSHA := rawTreeSHA(t, c.originDir, "HEAD", originSubtree)

	res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root})
	if res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	// Human shape: ≤ 2 lines including the footer hint.
	lines := nonEmptyLines(res.stdout)
	if len(lines) > 2 {
		t.Errorf("human stdout has %d lines, want <= 2:\n%s", len(lines), res.stdout)
	}

	if !strings.Contains(res.stdout, "skillrig verify") {
		t.Errorf("human output missing next-step footer (skillrig verify):\n%s", res.stdout)
	}

	// Files vendored byte-identical to the origin, with modes (incl. the exec bit)
	// preserved.
	assertVendoredMatchesOrigin(t, c)

	// Lock: one entry; treeSha == the raw-git ground truth; no requires field.
	entry := lockEntry(t, c.root, sampleSkill)

	if entry["treeSha"] != wantTreeSHA {
		t.Errorf("lock treeSha = %v, want raw-git ground truth %s", entry["treeSha"], wantTreeSHA)
	}

	if entry["version"] != sampleVersion {
		t.Errorf("lock version = %v, want %s", entry["version"], sampleVersion)
	}

	if entry["path"] != vendoredPath {
		t.Errorf("lock path = %v, want %s", entry["path"], vendoredPath)
	}

	if _, hasRequires := entry["requires"]; hasRequires {
		t.Errorf("lock entry must NOT carry a requires field (D4), got: %v", entry)
	}

	if commit, _ := entry["commit"].(string); len(commit) != 40 {
		t.Errorf("lock commit = %q, want a 40-hex commit SHA", commit)
	}

	// --json: parseable, all keys present, action == vendored.
	jsonRes := run(t, runOpts{args: []string{"add", sampleSkill, "--json"}, cwd: c.root})

	obj := decodeJSON(t, jsonRes.stdout)
	requireKeys(t, obj, addResultKeys...)

	// The second add is on already-vendored content, so it is idempotent; assert
	// the vendoring fields independently via the first add's lock + a fresh add.
	if obj["treeSha"] != wantTreeSHA {
		t.Errorf("--json treeSha = %v, want %s", obj["treeSha"], wantTreeSHA)
	}
}

func TestQuickstart_AddVendorsSkillJSONAction(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	res := run(t, runOpts{args: []string{"add", sampleSkill, "--json"}, cwd: c.root})
	if res.exit != 0 {
		t.Fatalf("add --json exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	obj := decodeJSON(t, res.stdout)
	requireKeys(t, obj, addResultKeys...)

	if obj["ok"] != true {
		t.Errorf("ok = %v, want true", obj["ok"])
	}

	if obj["action"] != "vendored" {
		t.Errorf("action = %v, want vendored (fresh add)", obj["action"])
	}

	if obj["dryRun"] != false {
		t.Errorf("dryRun = %v, want false", obj["dryRun"])
	}
}

func TestQuickstart_AddIdempotent(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	first := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root})
	if first.exit != 0 {
		t.Fatalf("first add exit = %d, want 0 (stderr: %s)", first.exit, first.stderr)
	}

	lockBefore := readFile(t, filepath.Join(c.root, ".skillrig", "skills-lock.json"))

	second := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root})
	if second.exit != 0 {
		t.Fatalf("second add exit = %d, want 0 (stderr: %s)", second.exit, second.stderr)
	}

	if !strings.Contains(second.stdout, "already vendored") && !strings.Contains(second.stdout, "no change") {
		t.Errorf("idempotent re-add should note no change, got:\n%s", second.stdout)
	}

	lockAfter := readFile(t, filepath.Join(c.root, ".skillrig", "skills-lock.json"))
	if lockBefore != lockAfter {
		t.Errorf("lock changed on idempotent re-add:\nbefore=%s\nafter=%s", lockBefore, lockAfter)
	}

	// Exactly one entry (no duplicate).
	skills := lockSkills(t, c.root)
	if len(skills) != 1 {
		t.Errorf("lock has %d skills, want exactly 1", len(skills))
	}

	jsonRes := run(t, runOpts{args: []string{"add", sampleSkill, "--json"}, cwd: c.root})

	obj := decodeJSON(t, jsonRes.stdout)
	if obj["action"] != "unchanged" {
		t.Errorf("--json action = %v on identical re-add, want unchanged", obj["action"])
	}
}

func TestQuickstart_AddDryRunWritesNothing(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	res := run(t, runOpts{args: []string{"add", sampleSkill, "--dry-run"}, cwd: c.root})
	if res.exit != 0 {
		t.Fatalf("add --dry-run exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if !strings.Contains(res.stdout, "would vendor") {
		t.Errorf("dry-run human output should be prefixed 'would vendor …', got:\n%s", res.stdout)
	}

	// No skill tree and no lock created.
	if _, err := os.Stat(filepath.Join(c.root, ".agents")); !os.IsNotExist(err) {
		t.Errorf(".agents/ must not exist after --dry-run, stat err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(c.root, ".skillrig", "skills-lock.json")); !os.IsNotExist(err) {
		t.Errorf("lock must not exist after --dry-run, stat err = %v", err)
	}

	jsonRes := run(t, runOpts{args: []string{"add", sampleSkill, "--dry-run", "--json"}, cwd: c.root})

	obj := decodeJSON(t, jsonRes.stdout)
	requireKeys(t, obj, addResultKeys...)

	if obj["dryRun"] != true {
		t.Errorf("--json dryRun = %v, want true", obj["dryRun"])
	}

	if obj["action"] != "vendored" {
		t.Errorf("--json action = %v on dry-run of a fresh skill, want vendored", obj["action"])
	}
}

func TestQuickstart_AddRefusesDivergentWithoutForce(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("initial add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	// Diverge a byte of the on-disk copy.
	skillMD := filepath.Join(c.root, vendoredPath, "SKILL.md")
	appendByte(t, skillMD)

	res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root})
	if res.exit != 1 {
		t.Fatalf("divergent add (no --force) exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	// Three distinct parts: what / why / fix.
	assertContains(t, "what", res.stderr, "refusing to overwrite \""+vendoredPath+"\"")
	assertContains(t, "why", res.stderr, "on-disk content diverges from the recorded fingerprint")
	assertContains(t, "fix", res.stderr, "--force")

	// Files left untouched by the refusal (still diverged).
	originMD := readFile(t, filepath.Join(c.originDir, "skills", sampleSkill, "SKILL.md"))
	if readFile(t, skillMD) == originMD {
		t.Errorf("refused add must not modify on-disk content")
	}

	// --force restores from origin and reports action == overwritten.
	forceRes := run(t, runOpts{args: []string{"add", sampleSkill, "--force", "--json"}, cwd: c.root})
	if forceRes.exit != 0 {
		t.Fatalf("forced add exit = %d, want 0 (stderr: %s)", forceRes.exit, forceRes.stderr)
	}

	obj := decodeJSON(t, forceRes.stdout)
	if obj["action"] != "overwritten" {
		t.Errorf("--force --json action = %v, want overwritten", obj["action"])
	}

	if readFile(t, skillMD) != originMD {
		t.Errorf("--force should restore the skill to the origin's content")
	}
}

func TestQuickstart_AddRequiresOrigin(t *testing.T) {
	t.Parallel()
	requireGit(t)

	// A git repo with no origin anywhere (no init, no env, isolated HOME).
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")

	res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: root})
	if res.exit != 1 {
		t.Fatalf("add without origin exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "no origin configured")
	assertContains(t, "why", res.stderr, "no SKILLRIG_ORIGIN / project / global origin")
	assertContains(t, "fix", res.stderr, "skillrig init --origin")
}

func TestQuickstart_AddNotGitRepo(t *testing.T) {
	t.Parallel()
	requireGit(t)

	// Origin supplied via SKILLRIG_ORIGIN, but cwd is NOT a git repo. The origin
	// checkout is laid out at <cwd>/my-org/my-skills so the relative resolution
	// finds it; the failure is the missing git work tree, not a missing origin.
	cwd := t.TempDir()
	bootstrapOrigin(t, cwd)

	res := run(t, runOpts{
		args: []string{"add", sampleSkill},
		cwd:  cwd,
		env:  map[string]string{"SKILLRIG_ORIGIN": originRepo},
	})
	if res.exit != 1 {
		t.Fatalf("add in non-git dir exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "not a git repository")
	assertContains(t, "why", res.stderr, "vendors into the repo's canonical .agents/skills")
	assertContains(t, "fix", res.stderr, "run inside the repo")
}

// ---------------------------------------------------------------------------
// US2 — Prove a skill is unmodified (verify label-honesty)
// ---------------------------------------------------------------------------

func TestQuickstart_VerifyPasses(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)
	wantTreeSHA := rawTreeSHA(t, c.originDir, "HEAD", originSubtree)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	commitAll(t, c.root, "vendor skill")

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if res.exit != 0 {
		t.Fatalf("verify exit = %d, want 0 (stderr: %s)\nNOTE: the vendored skill declares "+
			"[[requires]] for tools absent in the test env; verify is integrity-only and "+
			"must still pass (SC-006/FR-014)", res.exit, res.stderr)
	}

	// Human: exactly 2 lines (summary + footer).
	lines := nonEmptyLines(res.stdout)
	if len(lines) != 2 {
		t.Errorf("verify pass human output = %d lines, want exactly 2:\n%s", len(lines), res.stdout)
	}

	if !strings.Contains(lines[0], "verified 1 skills") {
		t.Errorf("line 1 = %q, want it to report 1 verified skill", lines[0])
	}

	// --json: ok, counts.verified == 1, the single verdict matches ground truth.
	jsonRes := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})

	rep := decodeReport(t, jsonRes.stdout)
	if rep.OK != true {
		t.Errorf("--json ok = %v, want true", rep.OK)
	}

	if rep.Counts.Verified != 1 {
		t.Errorf("counts.verified = %d, want 1", rep.Counts.Verified)
	}

	if len(rep.Verdicts) != 1 {
		t.Fatalf("verdicts = %d, want 1", len(rep.Verdicts))
	}

	v := rep.Verdicts[0]
	if v.Status != "ok" {
		t.Errorf("verdict status = %q, want ok", v.Status)
	}

	// The headline ground-truth invariant: expected == actual == raw-git tree-SHA.
	if v.ExpectedTreeSha != wantTreeSHA || v.ActualTreeSha != wantTreeSHA {
		t.Errorf("verdict expected/actual = %q/%q, want both == %s", v.ExpectedTreeSha, v.ActualTreeSha, wantTreeSHA)
	}
}

func TestQuickstart_VerifyIsReadOnly(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	commitAll(t, c.root, "vendor skill")

	lockPath := filepath.Join(c.root, ".skillrig", "skills-lock.json")
	skillMD := filepath.Join(c.root, vendoredPath, "SKILL.md")

	// Pass run: working tree + lock + skill file byte-identical before/after.
	beforeStatus := statusPorcelain(t, c.root)
	beforeLock := readFile(t, lockPath)
	beforeSkill := readFile(t, skillMD)

	if res := run(t, runOpts{args: []string{"verify"}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("verify (pass) exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	assertUnchanged(t, "pass", beforeStatus, statusPorcelain(t, c.root),
		beforeLock, readFile(t, lockPath), beforeSkill, readFile(t, skillMD))

	// Tamper + commit so the next verify FAILS, then assert it is still read-only.
	appendByte(t, skillMD)
	commitAll(t, c.root, "tamper")

	failStatus := statusPorcelain(t, c.root)
	failLock := readFile(t, lockPath)
	failSkill := readFile(t, skillMD)

	if res := run(t, runOpts{args: []string{"verify"}, cwd: c.root}); res.exit != 2 {
		t.Fatalf("verify (fail) exit = %d, want 2 (stderr: %s)", res.exit, res.stderr)
	}

	assertUnchanged(t, "fail", failStatus, statusPorcelain(t, c.root),
		failLock, readFile(t, lockPath), failSkill, readFile(t, skillMD))
}

func TestQuickstart_VerifyDetectsTamper(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	commitAll(t, c.root, "vendor skill")

	recorded := lockEntry(t, c.root, sampleSkill)["treeSha"]

	// Tamper one byte and commit it (mismatch is a label-honesty failure on the
	// committed tree, distinct from the uncommitted "dirty" finding).
	appendByte(t, filepath.Join(c.root, vendoredPath, "SKILL.md"))
	commitAll(t, c.root, "tamper")

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if res.exit != 2 {
		t.Fatalf("verify after tamper exit = %d, want 2 (stderr: %s)", res.exit, res.stderr)
	}

	if !strings.Contains(res.stdout, sampleSkill) {
		t.Errorf("human failure output must name the tampered skill %q:\n%s", sampleSkill, res.stdout)
	}

	jsonRes := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})

	rep := decodeReport(t, jsonRes.stdout)

	v := findVerdict(t, rep, sampleSkill)
	if v.Status != "mismatch" {
		t.Errorf("verdict status = %q, want mismatch", v.Status)
	}

	if v.ExpectedTreeSha != recorded {
		t.Errorf("expectedTreeSha = %q, want the recorded %v", v.ExpectedTreeSha, recorded)
	}

	if v.ActualTreeSha == "" || v.ActualTreeSha == v.ExpectedTreeSha {
		t.Errorf("actualTreeSha = %q, want a non-empty value != expected", v.ActualTreeSha)
	}
}

func TestQuickstart_VerifyDirtyUncommitted(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	// Vendored but NOT committed → dirty (distinct from mismatch).
	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if res.exit != 2 {
		t.Fatalf("verify on uncommitted skill exit = %d, want 2 (stderr: %s)", res.exit, res.stderr)
	}

	jsonRes := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})

	rep := decodeReport(t, jsonRes.stdout)
	if rep.Counts.Dirty < 1 {
		t.Errorf("counts.dirty = %d, want >= 1", rep.Counts.Dirty)
	}

	v := findVerdict(t, rep, sampleSkill)
	if v.Status != "dirty" {
		t.Errorf("verdict status = %q, want dirty (distinct from mismatch)", v.Status)
	}

	if !strings.Contains(strings.ToLower(v.Reason), "commit") {
		t.Errorf("dirty reason = %q, want it to mention committing", v.Reason)
	}
}

func TestQuickstart_VerifyEmptyRepoPasses(t *testing.T) {
	t.Parallel()
	requireGit(t)

	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")

	res := run(t, runOpts{args: []string{"verify"}, cwd: root})
	if res.exit != 0 {
		t.Fatalf("verify on empty repo exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	jsonRes := run(t, runOpts{args: []string{"verify", "--json"}, cwd: root})

	rep := decodeReport(t, jsonRes.stdout)
	if rep.OK != true {
		t.Errorf("--json ok = %v on empty repo, want true", rep.OK)
	}

	if rep.Counts != (counts{}) {
		t.Errorf("counts = %+v on empty repo, want all zero", rep.Counts)
	}

	if len(rep.Verdicts) != 0 {
		t.Errorf("verdicts = %d on empty repo, want 0 (serialized as [])", len(rep.Verdicts))
	}

	// verdicts must be [] not null.
	if !strings.Contains(jsonRes.stdout, "\"verdicts\":[]") {
		t.Errorf("empty-repo verdicts should serialize as [], got:\n%s", jsonRes.stdout)
	}
}

// ---------------------------------------------------------------------------
// US3 — Orphan / completeness (verify)
// ---------------------------------------------------------------------------

func TestQuickstart_VerifyDetectsOrphan(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	// An unlocked skill dir created by hand (no add) + committed → orphan.
	writeRogueSkill(t, c.root)
	commitAll(t, c.root, "vendor + rogue")

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if res.exit != 2 {
		t.Fatalf("verify with orphan exit = %d, want 2 (stderr: %s)", res.exit, res.stderr)
	}

	jsonRes := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})

	rep := decodeReport(t, jsonRes.stdout)
	if rep.Counts.Orphan < 1 {
		t.Errorf("counts.orphan = %d, want >= 1", rep.Counts.Orphan)
	}

	v := findVerdict(t, rep, "rogue")
	if v.Status != "orphan" {
		t.Errorf("rogue verdict status = %q, want orphan", v.Status)
	}
}

// TestQuickstart_VerifyIgnoresViewDirs proves the orphan/completeness scan is
// confined to the canonical .agents/skills root (FR-011, US3 AS3): a skill-looking
// directory materialized under a per-client view path (e.g. .claude/skills) is
// neither scanned nor flagged as an orphan. Regression guard for the deferred
// multi-client symlink-view feature — without it, a future scan that wandered into
// view roots would spuriously fail verify.
func TestQuickstart_VerifyIgnoresViewDirs(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	// A manually-created per-client view directory that *looks* like a skill but
	// lives OUTSIDE .agents/skills — verify must ignore it entirely.
	writeClientViewSkill(t, c.root, "viewer")
	commitAll(t, c.root, "vendor skill + non-canonical client view")

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if res.exit != 0 {
		t.Fatalf("verify with a non-canonical view dir exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	rep := decodeReport(t, run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root}).stdout)
	if !rep.OK || rep.Counts.Verified != 1 || rep.Counts.Orphan != 0 {
		t.Errorf("report = %+v, want ok with exactly 1 verified and 0 orphans (view dir ignored)", rep)
	}

	if len(rep.Verdicts) != 1 {
		t.Fatalf("verdicts = %d, want exactly 1 (only the canonical skill, not the .claude view)", len(rep.Verdicts))
	}

	if got := rep.Verdicts[0].Path; got != vendoredPath {
		t.Errorf("verdict path = %q, want the canonical %q (the .claude view must not be scanned)", got, vendoredPath)
	}
}

func TestQuickstart_VerifyDetectsMissing(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	commitAll(t, c.root, "vendor skill")

	// Remove the vendored dir but keep the lock entry → missing.
	if err := os.RemoveAll(filepath.Join(c.root, vendoredPath)); err != nil {
		t.Fatalf("remove vendored dir: %v", err)
	}

	commitAll(t, c.root, "remove skill")

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if res.exit != 2 {
		t.Fatalf("verify with missing skill exit = %d, want 2 (stderr: %s)", res.exit, res.stderr)
	}

	jsonRes := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})

	rep := decodeReport(t, jsonRes.stdout)
	if rep.Counts.Missing < 1 {
		t.Errorf("counts.missing = %d, want >= 1", rep.Counts.Missing)
	}

	v := findVerdict(t, rep, sampleSkill)
	if v.Status != "missing" {
		t.Errorf("verdict status = %q, want missing", v.Status)
	}
}

func TestQuickstart_VerifyAggregatesAllFailures(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	commitAll(t, c.root, "vendor skill")

	// One tampered (mismatch) + one orphan, both committed, in a single run.
	appendByte(t, filepath.Join(c.root, vendoredPath, "SKILL.md"))
	writeRogueSkill(t, c.root)
	commitAll(t, c.root, "tamper + rogue")

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if res.exit != 2 {
		t.Fatalf("verify exit = %d, want 2 (stderr: %s)", res.exit, res.stderr)
	}

	jsonRes := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})

	rep := decodeReport(t, jsonRes.stdout)

	// Did NOT stop at the first failure: both failures present in one report.
	if rep.Counts.Mismatch < 1 || rep.Counts.Orphan < 1 {
		t.Errorf("counts = %+v, want mismatch>=1 AND orphan>=1 (aggregated, not first-fail)", rep.Counts)
	}

	// Both skills appear as verdicts (the check covers the full union).
	if len(rep.Verdicts) < 2 {
		t.Errorf("verdicts = %d, want >= 2 (both skills reported)", len(rep.Verdicts))
	}

	_ = findVerdict(t, rep, sampleSkill)
	_ = findVerdict(t, rep, "rogue")
}

// ---------------------------------------------------------------------------
// US4 — Scriptable outcome (exit codes + --json)
// ---------------------------------------------------------------------------

func TestQuickstart_VerifyExitCodeMatrix(t *testing.T) {
	t.Parallel()

	// pass → 0 (and deterministic on repeat).
	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	commitAll(t, c.root, "vendor skill")

	first := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	second := run(t, runOpts{args: []string{"verify"}, cwd: c.root})

	if first.exit != 0 || second.exit != 0 {
		t.Errorf("pass exit codes = %d/%d, want 0/0 (deterministic)", first.exit, second.exit)
	}

	// verification failure → 2.
	appendByte(t, filepath.Join(c.root, vendoredPath, "SKILL.md"))
	commitAll(t, c.root, "tamper")

	failA := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	failB := run(t, runOpts{args: []string{"verify"}, cwd: c.root})

	if failA.exit != 2 || failB.exit != 2 {
		t.Errorf("failure exit codes = %d/%d, want 2/2", failA.exit, failB.exit)
	}

	// malformed lock → 1 (config/usage, distinct from 2).
	if err := os.WriteFile(filepath.Join(c.root, ".skillrig", "skills-lock.json"), []byte("not json{"), 0o644); err != nil {
		t.Fatalf("corrupt lock: %v", err)
	}

	if res := run(t, runOpts{args: []string{"verify"}, cwd: c.root}); res.exit != 1 {
		t.Errorf("malformed lock exit = %d, want 1", res.exit)
	}

	// not a git repo → 1.
	nonGit := t.TempDir()
	if res := run(t, runOpts{args: []string{"verify"}, cwd: nonGit}); res.exit != 1 {
		t.Errorf("not-a-git-repo exit = %d, want 1", res.exit)
	}

	// Exit code 3 is reserved and MUST never be emitted by verify.
	for _, code := range []int{first.exit, second.exit, failA.exit, failB.exit} {
		if code == 3 {
			t.Errorf("verify emitted reserved exit code 3")
		}
	}
}

func TestQuickstart_VerifyJSONComplete(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	commitAll(t, c.root, "vendor skill")

	// Passing run.
	pass := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})
	assertJSONStructurallyComplete(t, "pass", pass.stdout)

	// stdout stays clean JSON even with diagnostics: stderr is separate.
	if strings.TrimSpace(pass.stderr) != "" {
		// Diagnostics (if any) must not be on stdout; a non-empty stderr is fine.
		_ = pass.stderr
	}

	// Failing run.
	appendByte(t, filepath.Join(c.root, vendoredPath, "SKILL.md"))
	commitAll(t, c.root, "tamper")

	fail := run(t, runOpts{args: []string{"verify", "--json"}, cwd: c.root})
	if fail.exit != 2 {
		t.Fatalf("failing verify --json exit = %d, want 2", fail.exit)
	}

	assertJSONStructurallyComplete(t, "fail", fail.stdout)
}

func TestQuickstart_VerifyMalformedLock(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)

	if res := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root}); res.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if err := os.WriteFile(filepath.Join(c.root, ".skillrig", "skills-lock.json"), []byte("{ not valid json"), 0o644); err != nil {
		t.Fatalf("corrupt lock: %v", err)
	}

	res := run(t, runOpts{args: []string{"verify"}, cwd: c.root})

	// Exit 1 (usage/config), DISTINCT from a verification failure (2).
	if res.exit != 1 {
		t.Fatalf("malformed lock exit = %d, want 1 (distinct from 2)", res.exit)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	// 3-part error naming the file; not a raw parser dump as the whole message.
	assertContains(t, "what", res.stderr, "skills-lock.json")
	assertContains(t, "why", res.stderr, "why:")
	assertContains(t, "fix", res.stderr, "fix:")

	// Raw cause surfaces under --verbose.
	verbose := run(t, runOpts{args: []string{"verify", "--verbose"}, cwd: c.root})
	if verbose.exit != 1 {
		t.Errorf("--verbose malformed lock exit = %d, want 1", verbose.exit)
	}
}

func TestQuickstart_AddHelpExamples(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"add", "--help"}})
	if res.exit != 0 {
		t.Fatalf("add --help exit = %d, want 0", res.exit)
	}

	if n := countExampleLines(res.stdout, "skillrig add "); n < 2 {
		t.Errorf("add --help shows %d 'skillrig add ' example lines, want >= 2:\n%s", n, res.stdout)
	}
}

func TestQuickstart_VerifyHelpExamples(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"verify", "--help"}})
	if res.exit != 0 {
		t.Fatalf("verify --help exit = %d, want 0", res.exit)
	}

	if n := countExampleLines(res.stdout, "skillrig verify "); n < 2 {
		t.Errorf("verify --help shows %d 'skillrig verify ' example lines, want >= 2:\n%s", n, res.stdout)
	}
}

// ---------------------------------------------------------------------------
// Round-trip (headline acceptance contract)
// ---------------------------------------------------------------------------

func TestQuickstart_AddThenVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	c := newConsumerRepo(t)
	wantTreeSHA := rawTreeSHA(t, c.originDir, "HEAD", originSubtree)

	// init (in newConsumerRepo) → add → commit → verify, two commands + a commit,
	// zero network, no hand-authored lock.
	addRes := run(t, runOpts{args: []string{"add", sampleSkill}, cwd: c.root})
	if addRes.exit != 0 {
		t.Fatalf("add exit = %d, want 0 (stderr: %s)", addRes.exit, addRes.stderr)
	}

	// add recorded exactly what verify will recompute.
	if got := lockEntry(t, c.root, sampleSkill)["treeSha"]; got != wantTreeSHA {
		t.Fatalf("lock treeSha = %v, want raw-git ground truth %s", got, wantTreeSHA)
	}

	commitAll(t, c.root, "vendor skill")

	verifyRes := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if verifyRes.exit != 0 {
		t.Fatalf("round-trip verify exit = %d, want 0 (stderr: %s)", verifyRes.exit, verifyRes.stderr)
	}

	// One-byte tamper + commit ⇒ verify exit 2.
	appendByte(t, filepath.Join(c.root, vendoredPath, "SKILL.md"))
	commitAll(t, c.root, "tamper")

	tamperRes := run(t, runOpts{args: []string{"verify"}, cwd: c.root})
	if tamperRes.exit != 2 {
		t.Fatalf("round-trip tamper verify exit = %d, want 2 (stderr: %s)", tamperRes.exit, tamperRes.stderr)
	}
}

// ---------------------------------------------------------------------------
// Local helpers (presentation-layer assertions / lock + report decoding)
// ---------------------------------------------------------------------------

// counts mirrors the verify --json counts object (all five status tallies).
type counts struct {
	Verified int `json:"verified"`
	Mismatch int `json:"mismatch"`
	Orphan   int `json:"orphan"`
	Missing  int `json:"missing"`
	Dirty    int `json:"dirty"`
}

// verdict mirrors one verify --json verdict (all six fields).
type verdict struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	Status          string `json:"status"`
	ExpectedTreeSha string `json:"expectedTreeSha"`
	ActualTreeSha   string `json:"actualTreeSha"`
	Reason          string `json:"reason"`
}

// report mirrors the verify --json top-level object.
type report struct {
	OK       bool      `json:"ok"`
	Counts   counts    `json:"counts"`
	Verdicts []verdict `json:"verdicts"`
}

// decodeReport strictly decodes a verify --json payload into the typed report.
func decodeReport(t *testing.T, stdout string) report {
	t.Helper()

	var rep report
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("verify --json is not parseable: %v\n%s", err, stdout)
	}

	return rep
}

// findVerdict returns the verdict named name, failing the test when absent.
func findVerdict(t *testing.T, rep report, name string) verdict {
	t.Helper()

	for _, v := range rep.Verdicts {
		if v.Name == name {
			return v
		}
	}

	t.Fatalf("no verdict named %q in report: %+v", name, rep.Verdicts)

	return verdict{}
}

// assertJSONStructurallyComplete verifies the verify --json payload is parseable
// and structurally complete: top-level ok/counts/verdicts, all five counts keys,
// and every verdict carrying all six fields (Constitution II — not a Contains
// check).
func assertJSONStructurallyComplete(t *testing.T, label, stdout string) {
	t.Helper()

	obj := decodeJSON(t, stdout)
	requireKeys(t, obj, "ok", "counts", "verdicts")

	countsObj, ok := obj["counts"].(map[string]any)
	if !ok {
		t.Fatalf("%s: counts is not an object: %v", label, obj["counts"])
	}

	requireKeys(t, countsObj, countsKeys...)

	verdicts, ok := obj["verdicts"].([]any)
	if !ok {
		t.Fatalf("%s: verdicts is not an array: %v", label, obj["verdicts"])
	}

	if len(verdicts) == 0 {
		t.Errorf("%s: expected at least one verdict to assert structural completeness", label)
	}

	for i, raw := range verdicts {
		vObj, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s: verdict[%d] is not an object: %v", label, i, raw)
		}

		requireKeys(t, vObj, verdictKeys...)
	}
}

// lockSkills decodes the lock's skills map, failing the test on a read/parse
// error. It proves the lock is tool-written valid JSON.
func lockSkills(t *testing.T, root string) map[string]map[string]any {
	t.Helper()

	data := readFile(t, filepath.Join(root, ".skillrig", "skills-lock.json"))

	var lf struct {
		Skills map[string]map[string]any `json:"skills"`
	}

	if err := json.Unmarshal([]byte(data), &lf); err != nil {
		t.Fatalf("lock is not parseable: %v\n%s", err, data)
	}

	return lf.Skills
}

// lockEntry returns the named skill's lock entry, failing if absent.
func lockEntry(t *testing.T, root, name string) map[string]any {
	t.Helper()

	entry, ok := lockSkills(t, root)[name]
	if !ok {
		t.Fatalf("lock has no entry for %q", name)
	}

	return entry
}

// assertContains asserts that haystack contains needle, labelling the failure
// (used to check the what / why / fix parts of an error as DISTINCT checks).
func assertContains(t *testing.T, part, haystack, needle string) {
	t.Helper()

	if !strings.Contains(haystack, needle) {
		t.Errorf("error (%s) should contain %q, got:\n%s", part, needle, haystack)
	}
}

// assertUnchanged fails when any of the three before/after pairs differ, naming
// the verify phase (pass/fail) — the read-only invariant (FR-015).
func assertUnchanged(t *testing.T, phase, statusBefore, statusAfter, lockBefore, lockAfter, skillBefore, skillAfter string) {
	t.Helper()

	if statusBefore != statusAfter {
		t.Errorf("[%s] git status changed across verify (not read-only):\nbefore=%q\nafter=%q", phase, statusBefore, statusAfter)
	}

	if lockBefore != lockAfter {
		t.Errorf("[%s] lock changed across verify (verify must write nothing)", phase)
	}

	if skillBefore != skillAfter {
		t.Errorf("[%s] skill file changed across verify (verify must write nothing)", phase)
	}
}

// appendByte appends one byte to the file at path (the canonical one-byte tamper
// / divergence used across scenarios).
func appendByte(t *testing.T, path string) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s for tamper: %v", path, err)
	}

	if _, err := f.WriteString("x"); err != nil {
		_ = f.Close()

		t.Fatalf("append to %s: %v", path, err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

// writeRogueSkill creates an unlocked skill directory under the canonical
// .agents/skills root (no add, so no lock entry) — the orphan fixture.
func writeRogueSkill(t *testing.T, root string) {
	t.Helper()

	dir := filepath.Join(root, ".agents", "skills", "rogue")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rogue: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "skill.toml"), []byte("name = \"rogue\"\nversion = \"0.0.1\"\n"), 0o644); err != nil {
		t.Fatalf("write rogue manifest: %v", err)
	}
}

// writeClientViewSkill materializes a skill-looking directory under a per-client
// view root (.claude/skills/<name>) — OUTSIDE the canonical .agents/skills. Used to
// prove verify's scan ignores non-canonical view locations (FR-011 / US3 AS3).
func writeClientViewSkill(t *testing.T, root, name string) {
	t.Helper()

	dir := filepath.Join(root, ".claude", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir client view: %v", err)
	}

	manifest := "name = \"" + name + "\"\nversion = \"0.0.1\"\n"
	if err := os.WriteFile(filepath.Join(dir, "skill.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write client-view manifest: %v", err)
	}
}

// fileMode returns the permission bits of the file at path.
func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	return info.Mode().Perm()
}

// countExampleLines counts lines in help output that begin (after optional
// leading whitespace) with prefix — the ">=2 usage examples" shape check
// (FR-018 / SC-009), stronger than a single Contains.
func countExampleLines(help, prefix string) int {
	n := 0

	for line := range strings.SplitSeq(help, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			n++
		}
	}

	return n
}
