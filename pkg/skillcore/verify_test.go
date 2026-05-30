package skillcore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// seedVerifyRepo builds a consumer git repo with one committed skill under
// .agents/skills/<name>/ and an on-disk lock recording its real (raw-git)
// tree-SHA. The tree-SHA is computed via raw git (the independent oracle, D11),
// never through skillcore. Returns the repo dir and the recorded tree-SHA.
func seedVerifyRepo(t *testing.T, name string) (repo, treeSha string) {
	t.Helper()

	repo = t.TempDir()
	runGit(t, repo, "init", "-q")

	rel := skillsRoot + "/" + name
	writeFile(t, repo, filepath.Join(rel, "SKILL.md"), 0o644, sampleSkillMd)
	writeFile(t, repo, filepath.Join(rel, "skill.toml"), 0o644, sampleManifest)
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "vendor "+name)

	treeSha = runGit(t, repo, "rev-parse", "HEAD:"+rel)
	commit := runGit(t, repo, "rev-parse", "HEAD")

	writeVerifyLock(t, repo, LockFile{
		LockfileVersion: 1,
		Origin:          "my-org/my-skills",
		Skills: map[string]LockEntry{
			name: {Version: "1.4.0", Commit: commit, TreeSha: treeSha, Path: rel},
		},
	})

	return repo, treeSha
}

func writeVerifyLock(t *testing.T, repo string, lf LockFile) {
	t.Helper()

	if err := WriteLock(repo, lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
}

// findVerdict returns the verdict for name, failing if absent.
func findVerdict(t *testing.T, rep Report, name string) Verdict {
	t.Helper()

	for _, v := range rep.Verdicts {
		if v.Name == name {
			return v
		}
	}

	t.Fatalf("no verdict for %q in %+v", name, rep.Verdicts)

	return Verdict{}
}

// TestVerify_CleanPass: a committed skill matching its lock → ok, no error,
// expected == actual == the raw-git tree-SHA (label-honesty ground truth).
func TestVerify_CleanPass(t *testing.T) {
	t.Parallel()

	repo, treeSha := seedVerifyRepo(t, "terraform-plan-review")

	rep, err := Verify(repo)
	if err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}

	if !rep.OK || rep.Counts.Verified != 1 {
		t.Fatalf("report = %+v, want ok with 1 verified", rep)
	}

	v := findVerdict(t, rep, "terraform-plan-review")
	if v.Status != StatusOK || v.ExpectedTreeSha != treeSha || v.ActualTreeSha != treeSha {
		t.Errorf("verdict = %+v, want ok with expected==actual==%s", v, treeSha)
	}
}

// TestVerify_Mismatch: committed content tampered (and re-committed) so HEAD's
// tree-SHA no longer matches the lock → mismatch, *VerifyFailure (exit-2 class).
func TestVerify_Mismatch(t *testing.T) {
	t.Parallel()

	repo, treeSha := seedVerifyRepo(t, "terraform-plan-review")

	writeFile(t, repo, filepath.Join(skillsRoot, "terraform-plan-review", "SKILL.md"), 0o644, "tampered\n")
	runGit(t, repo, "commit", "-aqm", "tamper")

	rep, err := Verify(repo)

	var vf *VerifyFailure
	if !errors.As(err, &vf) {
		t.Fatalf("Verify error = %T (%v), want *VerifyFailure", err, err)
	}

	if rep.OK || rep.Counts.Mismatch != 1 {
		t.Fatalf("report = %+v, want not-ok with 1 mismatch", rep)
	}

	v := findVerdict(t, rep, "terraform-plan-review")
	if v.Status != StatusMismatch || v.ExpectedTreeSha != treeSha || v.ActualTreeSha == treeSha {
		t.Errorf("verdict = %+v, want mismatch with expected==%s != actual", v, treeSha)
	}
}

// TestVerify_Orphan: an on-disk skill dir with no lock entry → orphan.
func TestVerify_Orphan(t *testing.T) {
	t.Parallel()

	repo, _ := seedVerifyRepo(t, "terraform-plan-review")

	writeFile(t, repo, filepath.Join(skillsRoot, "rogue", "skill.toml"), 0o644, "name = \"rogue\"\nversion = \"0.0.1\"\n")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-qm", "add rogue")

	rep, err := Verify(repo)

	var vf *VerifyFailure
	if !errors.As(err, &vf) {
		t.Fatalf("Verify error = %T, want *VerifyFailure", err)
	}

	if rep.Counts.Orphan != 1 {
		t.Errorf("counts.orphan = %d, want 1 (%+v)", rep.Counts.Orphan, rep)
	}

	if v := findVerdict(t, rep, "rogue"); v.Status != StatusOrphan {
		t.Errorf("rogue verdict = %q, want orphan", v.Status)
	}
}

// TestVerify_Missing: a lock entry whose files were removed (and the removal
// committed) → missing.
func TestVerify_Missing(t *testing.T) {
	t.Parallel()

	repo, _ := seedVerifyRepo(t, "terraform-plan-review")

	if err := os.RemoveAll(filepath.Join(repo, skillsRoot, "terraform-plan-review")); err != nil {
		t.Fatalf("remove skill: %v", err)
	}

	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-qm", "remove skill")

	rep, err := Verify(repo)

	var vf *VerifyFailure
	if !errors.As(err, &vf) {
		t.Fatalf("Verify error = %T, want *VerifyFailure", err)
	}

	if rep.Counts.Missing != 1 {
		t.Errorf("counts.missing = %d, want 1 (%+v)", rep.Counts.Missing, rep)
	}

	if v := findVerdict(t, rep, "terraform-plan-review"); v.Status != StatusMissing {
		t.Errorf("verdict = %q, want missing", v.Status)
	}
}

// TestVerify_Dirty: a committed skill with an uncommitted local edit → dirty
// (a working-state finding, distinct from mismatch).
func TestVerify_Dirty(t *testing.T) {
	t.Parallel()

	repo, _ := seedVerifyRepo(t, "terraform-plan-review")

	// Uncommitted edit to the vendored tree.
	writeFile(t, repo, filepath.Join(skillsRoot, "terraform-plan-review", "SKILL.md"), 0o644, "locally edited\n")

	rep, err := Verify(repo)

	var vf *VerifyFailure
	if !errors.As(err, &vf) {
		t.Fatalf("Verify error = %T, want *VerifyFailure", err)
	}

	if rep.Counts.Dirty != 1 {
		t.Errorf("counts.dirty = %d, want 1 (%+v)", rep.Counts.Dirty, rep)
	}

	if v := findVerdict(t, rep, "terraform-plan-review"); v.Status != StatusDirty {
		t.Errorf("verdict = %q, want dirty", v.Status)
	}
}

// TestVerify_DirtyTakesPrecedenceOverMismatch: when content is both committed-
// tampered (would be mismatch) AND has a further uncommitted edit, dirty wins —
// the by-design precedence (commit before verifying).
func TestVerify_DirtyTakesPrecedenceOverMismatch(t *testing.T) {
	t.Parallel()

	repo, _ := seedVerifyRepo(t, "terraform-plan-review")

	// Committed tamper (would be a mismatch on its own)…
	writeFile(t, repo, filepath.Join(skillsRoot, "terraform-plan-review", "SKILL.md"), 0o644, "committed tamper\n")
	runGit(t, repo, "commit", "-aqm", "committed tamper")
	// …plus a further uncommitted edit.
	writeFile(t, repo, filepath.Join(skillsRoot, "terraform-plan-review", "SKILL.md"), 0o644, "uncommitted on top\n")

	rep, _ := Verify(repo)

	v := findVerdict(t, rep, "terraform-plan-review")
	if v.Status != StatusDirty {
		t.Errorf("verdict = %q, want dirty (dirty masks mismatch until committed)", v.Status)
	}
}

// TestVerify_EmptyRepoPasses: a fresh repo with no skills and no lock → ok.
func TestVerify_EmptyRepoPasses(t *testing.T) {
	t.Parallel()

	repo := newConsumer(t)

	rep, err := Verify(repo)
	if err != nil {
		t.Fatalf("Verify(empty): unexpected error: %v", err)
	}

	if !rep.OK || len(rep.Verdicts) != 0 || rep.Counts != (Counts{}) {
		t.Errorf("report = %+v, want ok/empty", rep)
	}
}

// TestVerify_AggregatesAllFindings: a mismatch AND an orphan in one run are both
// reported (never first-fail).
func TestVerify_AggregatesAllFindings(t *testing.T) {
	t.Parallel()

	repo, _ := seedVerifyRepo(t, "terraform-plan-review")

	writeFile(t, repo, filepath.Join(skillsRoot, "terraform-plan-review", "SKILL.md"), 0o644, "tampered\n")
	writeFile(t, repo, filepath.Join(skillsRoot, "rogue", "skill.toml"), 0o644, "name = \"rogue\"\n")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-aqm", "tamper + orphan")

	rep, _ := Verify(repo)

	if rep.Counts.Mismatch < 1 || rep.Counts.Orphan < 1 {
		t.Errorf("counts = %+v, want >=1 mismatch AND >=1 orphan in one run", rep.Counts)
	}

	if len(rep.Verdicts) < 2 {
		t.Errorf("verdicts = %d, want >= 2 (did not aggregate)", len(rep.Verdicts))
	}
}

// TestVerify_MalformedLockVersion: an unsupported lockfileVersion is a *LockError
// (config/usage, exit 1), NOT a *VerifyFailure (exit 2).
func TestVerify_MalformedLockVersion(t *testing.T) {
	t.Parallel()

	repo, treeSha := seedVerifyRepo(t, "terraform-plan-review")

	writeVerifyLock(t, repo, LockFile{
		LockfileVersion: 99,
		Skills:          map[string]LockEntry{"terraform-plan-review": {TreeSha: treeSha, Path: skillsRoot + "/terraform-plan-review"}},
	})

	_, err := Verify(repo)

	var lockErr *LockError
	if !errors.As(err, &lockErr) {
		t.Fatalf("Verify error = %T (%v), want *LockError", err, err)
	}

	var vf *VerifyFailure
	if errors.As(err, &vf) {
		t.Error("unsupported lockfileVersion must NOT be a *VerifyFailure (it is exit 1, not 2)")
	}
}
