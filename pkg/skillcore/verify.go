package skillcore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Verdict statuses for Verdict.Status.
const (
	// StatusOK: locked, present, committed, and the recomputed tree-SHA matches.
	StatusOK = "ok"
	// StatusMismatch: locked and committed, but the tree-SHA differs (label-honesty failure).
	StatusMismatch = "mismatch"
	// StatusOrphan: on disk under .agents/skills/ but with no lock entry.
	StatusOrphan = "orphan"
	// StatusMissing: a lock entry whose path is absent on disk / from HEAD.
	StatusMissing = "missing"
	// StatusDirty: locked and present but uncommitted/modified versus HEAD.
	StatusDirty = "dirty"
)

// skillsRoot is the canonical vendored-skills directory, repo-relative.
const skillsRoot = ".agents/skills"

// expectedLockfileVersion is the only lockfileVersion this slice understands.
const expectedLockfileVersion = 1

// Report is the aggregate result of Verify.
type Report struct {
	OK       bool
	Counts   Counts
	Verdicts []Verdict
}

// Counts tallies verdicts by status for the compact summary.
type Counts struct {
	Verified int
	Mismatch int
	Orphan   int
	Missing  int
	Dirty    int
}

// Verdict is the per-skill outcome over the union of locked and on-disk skills.
type Verdict struct {
	Name            string
	Path            string
	Status          string
	ExpectedTreeSha string
	ActualTreeSha   string
	Reason          string
}

// Verify checks every vendored skill in repoRoot against the lock:
// label-honesty (recompute TreeSHA on HEAD), orphan/completeness (on-disk set
// versus locked set), and dirty (uncommitted). It is read-only, offline, and
// deterministic, and aggregates all findings. It returns a *VerifyFailure (with
// the same Report attached) when the report is not ok, so callers can branch.
//
// It distinguishes two error classes: a configuration/usage problem — a
// malformed lock (*LockError) or a git failure such as "not a git repository"
// (*GitError) — is returned as a non-*VerifyFailure error (the CLI maps these to
// exit 1). Per-skill findings are returned as a *VerifyFailure (exit 2). It is
// presentation-free and never writes git objects (only rev-parse / status).
func Verify(repoRoot string) (Report, error) {
	lf, err := readVerifyLock(repoRoot)
	if err != nil {
		return Report{}, err
	}

	onDisk, err := enumerateOnDiskSkills(repoRoot)
	if err != nil {
		return Report{}, err
	}

	// Index on-disk skills by repo-relative path so locked entries can claim
	// them and the remainder fall through as orphans.
	onDiskByPath := map[string]bool{}
	for _, p := range onDisk {
		onDiskByPath[p] = true
	}

	verdicts := []Verdict{}
	lockedPaths := map[string]bool{}

	for name, entry := range lf.Skills {
		lockedPaths[entry.Path] = true

		verdict, err := verifyLockedSkill(repoRoot, name, entry, onDiskByPath[entry.Path])
		if err != nil {
			return Report{}, err
		}

		verdicts = append(verdicts, verdict)
	}

	for _, path := range onDisk {
		if lockedPaths[path] {
			continue
		}

		verdict, err := verifyOrphanSkill(repoRoot, path)
		if err != nil {
			return Report{}, err
		}

		verdicts = append(verdicts, verdict)
	}

	// Deterministic ordering: callers render this directly, so sort by path.
	sort.Slice(verdicts, func(i, j int) bool {
		return verdicts[i].Path < verdicts[j].Path
	})

	rep := buildReport(verdicts)
	if !rep.OK {
		return rep, &VerifyFailure{Report: rep}
	}

	return rep, nil
}

// readVerifyLock reads the lock and enforces the supported lockfileVersion. An
// absent lock is not an error (zero skills). A malformed/unreadable lock, or one
// with an unsupported version, is a *LockError (a config/usage problem).
func readVerifyLock(repoRoot string) (LockFile, error) {
	path := lockPath(repoRoot)

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LockFile{}, nil
		}

		return LockFile{}, &LockError{Path: path, Cause: err}
	}

	lf, err := ReadLock(repoRoot)
	if err != nil {
		return LockFile{}, &LockError{Path: path, Cause: err}
	}

	if lf.LockfileVersion != expectedLockfileVersion {
		return LockFile{}, &LockError{
			Path:  path,
			Cause: errors.New("unsupported lockfileVersion: " + strconv.Itoa(lf.LockfileVersion)),
		}
	}

	if lf.Skills == nil {
		lf.Skills = map[string]LockEntry{}
	}

	return lf, nil
}

// enumerateOnDiskSkills returns the repo-relative paths of every directory under
// .agents/skills/* that contains a skill.toml or SKILL.md (the spike §6 rule for
// "this dir is a skill"). An absent skills root yields an empty set.
func enumerateOnDiskSkills(repoRoot string) ([]string, error) {
	rootAbs := filepath.Join(repoRoot, skillsRoot)

	entries, err := os.ReadDir(rootAbs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}

		return nil, &LockError{Path: rootAbs, Cause: err}
	}

	paths := []string{}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		if !isSkillDir(filepath.Join(rootAbs, e.Name())) {
			continue
		}

		paths = append(paths, filepath.ToSlash(filepath.Join(skillsRoot, e.Name())))
	}

	return paths, nil
}

// isSkillDir reports whether dir contains a skill.toml or SKILL.md.
func isSkillDir(dir string) bool {
	for _, marker := range []string{"skill.toml", "SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}

	return false
}

// verifyLockedSkill produces the verdict for one locked skill. presentOnDisk is
// whether the skill's directory exists on disk (with a marker file).
func verifyLockedSkill(
	repoRoot, name string,
	entry LockEntry,
	presentOnDisk bool,
) (Verdict, error) {
	verdict := Verdict{
		Name:            name,
		Path:            entry.Path,
		ExpectedTreeSha: entry.TreeSha,
	}

	inHead, headErr := pathInHead(repoRoot, entry.Path)
	if headErr != nil {
		return Verdict{}, headErr
	}

	// Absent entirely (not committed, not on disk) → missing.
	if !inHead && !presentOnDisk {
		verdict.Status = StatusMissing
		verdict.Reason = "lock entry whose path is absent on disk and from HEAD"

		return verdict, nil
	}

	dirty, err := pathDirty(repoRoot, entry.Path)
	if err != nil {
		return Verdict{}, err
	}

	// Uncommitted (dirty working tree) or present on disk but not yet in HEAD →
	// dirty (a working-state finding, distinct from a label-honesty mismatch).
	if dirty || (!inHead && presentOnDisk) {
		verdict.Status = StatusDirty
		verdict.Reason = "vendored but uncommitted or locally modified — commit before verifying"

		return verdict, nil
	}

	actual, err := TreeSHA(repoRoot, "HEAD", entry.Path)
	if err != nil {
		return Verdict{}, err
	}

	verdict.ActualTreeSha = actual
	if actual != entry.TreeSha {
		verdict.Status = StatusMismatch
		verdict.Reason = "content does not match recorded version"

		return verdict, nil
	}

	verdict.Status = StatusOK

	return verdict, nil
}

// verifyOrphanSkill produces the orphan verdict for an on-disk skill with no
// lock entry. The actual tree-SHA is recomputed when the path is committed and
// left empty otherwise (uncommitted orphan).
func verifyOrphanSkill(repoRoot, path string) (Verdict, error) {
	verdict := Verdict{
		Name:   filepath.Base(path),
		Path:   path,
		Status: StatusOrphan,
		Reason: "present on disk but not in the lock",
	}

	inHead, err := pathInHead(repoRoot, path)
	if err != nil {
		return Verdict{}, err
	}

	if !inHead {
		return verdict, nil
	}

	actual, err := TreeSHA(repoRoot, "HEAD", path)
	if err != nil {
		return Verdict{}, err
	}

	verdict.ActualTreeSha = actual

	return verdict, nil
}

// pathInHead reports whether path resolves to a tree in HEAD. A path that git
// specifically reports as absent from the committed tree is (false, nil); any
// other git failure (not a git repository, git not on PATH, unborn HEAD, …)
// propagates as a *GitError so the SDK contract holds — Verify must NOT downgrade
// a fatal git failure into a "missing"/"orphan" verdict (it returns the error).
func pathInHead(repoRoot, path string) (bool, error) {
	_, err := TreeSHA(repoRoot, "HEAD", path)
	if err == nil {
		return true, nil
	}

	var gitErr *GitError
	if errors.As(err, &gitErr) && gitErr.ExitCode > 0 && !isFatalGitError(gitErr.Stderr) {
		// git ran inside a repo but could not resolve HEAD:<path> — the path is
		// not in the committed tree. This covers an absent path ("does not exist
		// in 'HEAD'") AND an unborn HEAD (no commits yet → "ambiguous argument
		// 'HEAD'"/"bad revision"), both of which are "not committed", not failures.
		return false, nil
	}

	// git could not run (ExitCode <= 0) or reported a fatal repo error (e.g. "not
	// a git repository"): surface it as a *GitError so Verify honors its contract
	// (a config/usage problem, not a missing/orphan verdict).
	return false, err
}

// isFatalGitError reports whether stderr indicates git could not operate on a
// repository at all (as opposed to merely failing to resolve a revision/path).
// Only these are propagated; every other rev-parse failure means "not in the
// committed tree".
func isFatalGitError(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "not a git repository")
}

// pathDirty reports whether the working tree for path has uncommitted changes,
// via git status --porcelain. Non-empty output means dirty.
func pathDirty(repoRoot, path string) (bool, error) {
	out, err := statusPorcelain(repoRoot, path)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(out) != "", nil
}

// buildReport tallies verdicts into Counts and sets OK iff all verdicts are ok.
func buildReport(verdicts []Verdict) Report {
	counts := Counts{}
	ok := true

	for _, v := range verdicts {
		switch v.Status {
		case StatusOK:
			counts.Verified++
		case StatusMismatch:
			counts.Mismatch++
			ok = false
		case StatusOrphan:
			counts.Orphan++
			ok = false
		case StatusMissing:
			counts.Missing++
			ok = false
		case StatusDirty:
			counts.Dirty++
			ok = false
		}
	}

	return Report{OK: ok, Counts: counts, Verdicts: verdicts}
}
