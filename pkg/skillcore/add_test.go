package skillcore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// newConsumer returns a fresh tmpDir git repo to vendor into.
func newConsumer(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init", "-q")

	return dir
}

// addOpts builds AddOptions for the bootstrapped origin + consumer.
func addOpts(originDir, skill, repoRoot string, force bool) AddOptions {
	return AddOptions{
		OriginDir: originDir,
		Ref:       "HEAD",
		Skill:     skill,
		RepoRoot:  repoRoot,
		Origin:    "my-org/my-skills",
		Force:     force,
	}
}

// TestAdd_VendorsAndRecordsLock is the happy path: a first add writes the
// subtree byte-identically under .agents/skills/<name>/ and records a lock entry
// whose treeSha equals the origin's raw-git tree-SHA (independent oracle, D11).
func TestAdd_VendorsAndRecordsLock(t *testing.T) {
	t.Parallel()

	originDir, skill := bootstrapOrigin(t)
	consumer := newConsumer(t)

	res, err := Add(addOpts(originDir, skill, consumer, false))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if res.Action != ActionVendored {
		t.Errorf("Action = %q, want %q", res.Action, ActionVendored)
	}

	wantTree := runGit(t, originDir, "rev-parse", "HEAD:skills/"+skill)
	if res.TreeSha != wantTree {
		t.Errorf("AddResult.TreeSha = %q, want (raw git) %q", res.TreeSha, wantTree)
	}

	// The manifest was copied, not injected/altered.
	vendored := filepath.Join(consumer, ".agents", "skills", skill, "skill.toml")

	got, err := os.ReadFile(vendored)
	if err != nil {
		t.Fatalf("read vendored manifest: %v", err)
	}

	if string(got) != sampleManifest {
		t.Error("vendored skill.toml is not byte-identical to the origin")
	}

	// The lock records the same fingerprint and the configured origin.
	lock, err := ReadLock(consumer)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}

	entry, ok := lock.Skills[skill]
	if !ok {
		t.Fatalf("lock has no entry for %q (entries: %v)", skill, lock.Skills)
	}

	if entry.TreeSha != wantTree {
		t.Errorf("lock treeSha = %q, want %q", entry.TreeSha, wantTree)
	}

	if lock.Origin != "my-org/my-skills" {
		t.Errorf("lock origin = %q, want my-org/my-skills", lock.Origin)
	}
}

// TestAdd_Idempotent asserts re-adding identical content is a no-op:
// action=unchanged, same fingerprint, no error (FR idempotency).
func TestAdd_Idempotent(t *testing.T) {
	t.Parallel()

	originDir, skill := bootstrapOrigin(t)
	consumer := newConsumer(t)

	first, err := Add(addOpts(originDir, skill, consumer, false))
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}

	second, err := Add(addOpts(originDir, skill, consumer, false))
	if err != nil {
		t.Fatalf("second Add: %v", err)
	}

	if second.Action != ActionUnchanged {
		t.Errorf("second Action = %q, want %q", second.Action, ActionUnchanged)
	}

	if second.TreeSha != first.TreeSha {
		t.Errorf("treeSha drifted across idempotent adds: %q vs %q", second.TreeSha, first.TreeSha)
	}
}

// TestAdd_IdempotentWhenManifestNameDiffersFromDir guards R2-H1: the lock is
// keyed by the manifest name, so the placement guard must look up the recorded
// fingerprint by that name too — not by the directory arg. data-model only says
// the leaf SHOULD equal the name, so a dir "tf-review" with manifest name
// "terraform-plan-review" is legal; before the fix an identical re-add was
// wrongly refused with an *OverwriteError (FR-003 violation).
func TestAdd_IdempotentWhenManifestNameDiffersFromDir(t *testing.T) {
	t.Parallel()

	originDir := t.TempDir()
	runGit(t, originDir, "init", "-q")

	const dirName = "tf-review" // != manifest name "terraform-plan-review"
	writeFile(t, originDir, filepath.Join("skills", dirName, "SKILL.md"), 0o644, sampleSkillMd)
	writeFile(t, originDir, filepath.Join("skills", dirName, "skill.toml"), 0o644, sampleManifest)
	runGit(t, originDir, "add", "-A")
	runGit(t, originDir, "commit", "-q", "-m", "seed dir!=name skill")

	consumer := newConsumer(t)

	if _, err := Add(addOpts(originDir, dirName, consumer, false)); err != nil {
		t.Fatalf("first Add: %v", err)
	}

	second, err := Add(addOpts(originDir, dirName, consumer, false))
	if err != nil {
		t.Fatalf("identical re-add must be idempotent, not refused: %v", err)
	}

	if second.Action != ActionUnchanged {
		t.Errorf("Action = %q, want %q (name!=dir must not false-refuse)", second.Action, ActionUnchanged)
	}
}

// TestAdd_OriginCheckoutMissing guards R2-M4/AR-2: when the entire origin
// checkout directory is absent, Add returns a distinct *OriginNotFoundError
// (check out the origin) rather than *SkillNotFoundError (check the skill name).
func TestAdd_OriginCheckoutMissing(t *testing.T) {
	t.Parallel()

	consumer := newConsumer(t)
	missingOrigin := filepath.Join(consumer, "my-org", "my-skills") // never created

	_, err := Add(addOpts(missingOrigin, "terraform-plan-review", consumer, false))
	if err == nil {
		t.Fatal("Add(missing origin checkout): want error, got nil")
	}

	var originMissing *OriginNotFoundError
	if !errors.As(err, &originMissing) {
		t.Fatalf("Add error = %T (%v), want *OriginNotFoundError", err, err)
	}

	var skillMissing *SkillNotFoundError
	if errors.As(err, &skillMissing) {
		t.Error("missing origin checkout must NOT be reported as SkillNotFoundError")
	}
}

// TestAdd_DivergentRefused asserts the never-silently-clobber guard (FR-004):
// once a vendored skill is locally modified it diverges from the lock, and a
// plain re-add must refuse with an *OverwriteError (the CLI maps it to exit 1).
func TestAdd_DivergentRefused(t *testing.T) {
	t.Parallel()

	originDir, skill := bootstrapOrigin(t)
	consumer := newConsumer(t)

	if _, err := Add(addOpts(originDir, skill, consumer, false)); err != nil {
		t.Fatalf("seed Add: %v", err)
	}

	// Diverge the vendored copy.
	writeFile(t, consumer, filepath.Join(".agents/skills", skill, "SKILL.md"), 0o644, "tampered\n")

	_, err := Add(addOpts(originDir, skill, consumer, false))
	if err == nil {
		t.Fatal("Add over divergent copy: want error, got nil")
	}

	var oe *OverwriteError
	if !errors.As(err, &oe) {
		t.Fatalf("Add error = %T (%v), want *OverwriteError", err, err)
	}
}

// TestAdd_ForceOverwritesDivergent asserts --force re-vendors a divergent copy
// (action=overwritten) and restores byte-identical origin content.
func TestAdd_ForceOverwritesDivergent(t *testing.T) {
	t.Parallel()

	originDir, skill := bootstrapOrigin(t)
	consumer := newConsumer(t)

	if _, err := Add(addOpts(originDir, skill, consumer, false)); err != nil {
		t.Fatalf("seed Add: %v", err)
	}

	writeFile(t, consumer, filepath.Join(".agents/skills", skill, "SKILL.md"), 0o644, "tampered\n")

	res, err := Add(addOpts(originDir, skill, consumer, true))
	if err != nil {
		t.Fatalf("forced Add: %v", err)
	}

	if res.Action != ActionOverwritten {
		t.Errorf("Action = %q, want %q", res.Action, ActionOverwritten)
	}

	restored, err := os.ReadFile(
		filepath.Join(consumer, ".agents", "skills", skill, "SKILL.md"))
	if err != nil {
		t.Fatalf("read restored SKILL.md: %v", err)
	}

	if string(restored) != sampleSkillMd {
		t.Errorf("forced overwrite did not restore origin content: got %q", restored)
	}
}

// TestAdd_SkillNotFound asserts a request for an absent origin skill returns a
// *SkillNotFoundError (CLI → exit 1), not a panic or generic error.
func TestAdd_SkillNotFound(t *testing.T) {
	t.Parallel()

	originDir, _ := bootstrapOrigin(t)
	consumer := newConsumer(t)

	_, err := Add(addOpts(originDir, "no-such-skill", consumer, false))
	if err == nil {
		t.Fatal("Add(missing skill): want error, got nil")
	}

	var nf *SkillNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("Add error = %T (%v), want *SkillNotFoundError", err, err)
	}
}

// TestAdd_DryRunWritesNothing asserts --dry-run reports the action but leaves no
// vendored files and no lock on disk.
func TestAdd_DryRunWritesNothing(t *testing.T) {
	t.Parallel()

	originDir, skill := bootstrapOrigin(t)
	consumer := newConsumer(t)

	opts := addOpts(originDir, skill, consumer, false)
	opts.DryRun = true

	res, err := Add(opts)
	if err != nil {
		t.Fatalf("dry-run Add: %v", err)
	}

	if !res.DryRun {
		t.Error("AddResult.DryRun = false, want true")
	}

	if _, err := os.Stat(filepath.Join(consumer, ".agents", "skills", skill)); !os.IsNotExist(err) {
		t.Error("dry-run vendored files on disk; want nothing written")
	}

	if _, err := os.Stat(filepath.Join(consumer, ".skillrig", "skills-lock.json")); !os.IsNotExist(err) {
		t.Error("dry-run wrote a lock file; want nothing written")
	}
}
