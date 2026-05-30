package skillcore

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Action is the outcome of an Add: how the vendored tree changed.
type Action string

const (
	// ActionVendored means the skill was newly written into the repo.
	ActionVendored Action = "vendored"
	// ActionUnchanged means an identical copy was already present (idempotent re-add).
	ActionUnchanged Action = "unchanged"
	// ActionOverwritten means a divergent copy was replaced under Force.
	ActionOverwritten Action = "overwritten"
)

// vendorRoot is the canonical, repo-relative root every skill is vendored under.
const vendorRoot = ".agents/skills"

// AddOptions configures Add. The caller supplies an already-resolved local
// origin checkout (OriginDir + Ref); skillcore neither resolves origins, reads
// config, nor fetches.
type AddOptions struct {
	OriginDir string
	Ref       string
	Skill     string
	RepoRoot  string
	// Origin is the resolved origin reference (OWNER/REPO[@REF]) the CLI
	// resolved this add from. skillcore records it verbatim in the lock's
	// top-level origin field; it does not parse or resolve it (presentation- and
	// resolution-free). Empty leaves any existing lock origin untouched.
	Origin string
	Force  bool
	DryRun bool
}

// AddResult reports what Add did, for the CLI to render.
type AddResult struct {
	Name    string
	Version string
	Path    string
	Commit  string
	TreeSha string
	Action  Action
	DryRun  bool
}

// SkillNotFoundError is returned when the requested skill has no skills/<skill>/
// directory in the origin checkout. It is presentation-free (terse Error); the
// CLI maps it to a usage error (exit 1) and renders the what/why/fix prose.
type SkillNotFoundError struct {
	Skill string
}

func (e *SkillNotFoundError) Error() string {
	return fmt.Sprintf("skill %q not found in origin", e.Skill)
}

// OverwriteError is returned when the vendored skill already exists on disk with
// content that diverges from the recorded fingerprint and Force is not set. It
// is presentation-free (terse Error); the CLI maps it to a usage error (exit 1)
// and renders the "use --force" guidance.
type OverwriteError struct {
	Skill string
	Path  string
}

func (e *OverwriteError) Error() string {
	return fmt.Sprintf("refusing to overwrite %q", e.Path)
}

// Add vendors one skill from the local origin at opts.OriginDir into
// opts.RepoRoot's canonical .agents/skills/<name>/, byte-identical and
// mode-preserving, then writes/updates the lock. It refuses a divergent
// overwrite unless opts.Force, writes nothing when opts.DryRun, and is
// idempotent on identical content.
func Add(opts AddOptions) (AddResult, error) {
	srcDir, err := prepareSource(opts)
	if err != nil {
		return AddResult{}, err
	}

	manifest, err := ParseManifest(filepath.Join(srcDir, "skill.toml"))
	if err != nil {
		return AddResult{}, err
	}

	originRelPath := "skills/" + opts.Skill

	treeSha, err := TreeSHA(opts.OriginDir, opts.Ref, originRelPath)
	if err != nil {
		return AddResult{}, err
	}

	commit, err := revParse(opts.OriginDir, opts.Ref)
	if err != nil {
		return AddResult{}, err
	}

	destPath := vendorRoot + "/" + opts.Skill
	destDir := filepath.Join(opts.RepoRoot, ".agents", "skills", opts.Skill)

	action, err := resolvePlacement(opts, manifest.Name, srcDir, destDir, treeSha)
	if err != nil {
		return AddResult{}, err
	}

	result := AddResult{
		Name:    manifest.Name,
		Version: manifest.Version,
		Path:    destPath,
		Commit:  commit,
		TreeSha: treeSha,
		Action:  action,
		DryRun:  opts.DryRun,
	}

	if opts.DryRun || action == ActionUnchanged {
		return result, nil
	}

	if action == ActionOverwritten {
		if err := os.RemoveAll(destDir); err != nil {
			return AddResult{}, fmt.Errorf("remove %s: %w", destDir, err)
		}
	}

	if err := copyTreePreservingModes(srcDir, destDir); err != nil {
		return AddResult{}, err
	}

	if err := writeLockEntry(opts.RepoRoot, manifest.Name, opts.Origin, result); err != nil {
		return AddResult{}, err
	}

	return result, nil
}

// prepareSource validates the skill name, locates the origin skill subtree, and
// rejects any symlink within it — all the safety pre-flight before Add touches
// the filesystem. opts.Skill is used as a path segment for the source, the
// destination, and os.RemoveAll on overwrite, so a traversal name (e.g. "../x")
// must be refused here, before any copy or delete can escape the canonical
// subtree; a symlink would let copy/compare follow it outside the subtree and
// break byte-identical/git-canonical vendoring.
func prepareSource(opts AddOptions) (string, error) {
	if err := validateSkillName(opts.Skill); err != nil {
		return "", err
	}

	srcDir, err := locateSkillSource(opts)
	if err != nil {
		return "", err
	}

	if err := ensureNoSymlinks(srcDir); err != nil {
		return "", err
	}

	return srcDir, nil
}

// validateSkillName rejects any skill name that is not a single safe path
// segment, so it can never escape the canonical .agents/skills/<name> subtree
// (path-traversal hardening): non-empty, not "." or "..", no path separator
// (`/` or `\`), and equal to its own filepath.Base.
func validateSkillName(name string) error {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || filepath.Base(name) != name {
		return &InvalidSkillNameError{Skill: name}
	}

	return nil
}

// ensureNoSymlinks walks srcDir and returns a *SymlinkUnsupportedError on the
// first symlink found. WalkDir yields a symlink as a non-directory entry and does
// not descend into a symlinked directory, so d.Type()&fs.ModeSymlink detects both
// file and directory symlinks before any copy/compare can follow them.
func ensureNoSymlinks(srcDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Type()&fs.ModeSymlink != 0 {
			rel, relErr := filepath.Rel(srcDir, path)
			if relErr != nil {
				rel = path
			}

			return &SymlinkUnsupportedError{Path: rel}
		}

		return nil
	})
}

// locateSkillSource resolves and validates the origin's skill subtree, returning
// its directory. It distinguishes a missing origin checkout
// (*OriginNotFoundError — the library isn't checked out at OriginDir) from a
// missing skill (*SkillNotFoundError — the origin is present but has no such
// skill), so the CLI can give the right fix (errors-as-navigation).
func locateSkillSource(opts AddOptions) (string, error) {
	if info, err := os.Stat(opts.OriginDir); err != nil || !info.IsDir() {
		return "", &OriginNotFoundError{OriginDir: opts.OriginDir, Ref: opts.Ref}
	}

	srcDir := filepath.Join(opts.OriginDir, "skills", opts.Skill)
	if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
		return "", &SkillNotFoundError{Skill: opts.Skill}
	}

	return srcDir, nil
}

// resolvePlacement inspects the destination and decides the Action without
// writing anything. A fresh placement is ActionVendored. An existing tree is
// ActionUnchanged (idempotent) only when its on-disk content is byte-identical
// to the origin source AND the lock records the matching fingerprint; any
// divergence is ActionOverwritten under Force, otherwise an *OverwriteError.
//
// name is the manifest name the lock is keyed by (writeLockEntry uses it); the
// lookup MUST use it, not opts.Skill (the directory arg), or a skill whose
// manifest name differs from its directory would never match its own lock entry
// and an identical re-add would be wrongly refused (FR-003 idempotency).
func resolvePlacement(opts AddOptions, name, srcDir, destDir, treeSha string) (Action, error) {
	if _, err := os.Stat(destDir); err != nil {
		if os.IsNotExist(err) {
			return ActionVendored, nil
		}

		return "", fmt.Errorf("stat %s: %w", destDir, err)
	}

	lock, err := ReadLock(opts.RepoRoot)
	if err != nil {
		return "", err
	}

	identical, err := treesIdentical(srcDir, destDir)
	if err != nil {
		return "", err
	}

	recorded := lock.Skills[name].TreeSha
	if identical && recorded == treeSha {
		return ActionUnchanged, nil
	}

	if !opts.Force {
		return "", &OverwriteError{
			Skill: opts.Skill,
			Path:  vendorRoot + "/" + opts.Skill,
		}
	}

	return ActionOverwritten, nil
}

// treesIdentical reports whether the directory trees at a and b have the same
// set of relative paths with byte-identical regular-file contents. It is the
// read-only divergence probe for the placement guard (no git objects written,
// per research): the on-disk vendored tree is "unchanged" only when it still
// matches the origin source exactly.
func treesIdentical(a, b string) (bool, error) {
	files := map[string]struct{}{}

	walkA, err := relFiles(a)
	if err != nil {
		return false, err
	}

	walkB, err := relFiles(b)
	if err != nil {
		return false, err
	}

	if len(walkA) != len(walkB) {
		return false, nil
	}

	for rel := range walkA {
		files[rel] = struct{}{}
	}

	for rel := range walkB {
		if _, ok := files[rel]; !ok {
			return false, nil
		}
	}

	for rel := range files {
		same, err := filesEqual(filepath.Join(a, rel), filepath.Join(b, rel))
		if err != nil {
			return false, err
		}

		if !same {
			return false, nil
		}
	}

	return true, nil
}

// relFiles returns the set of regular-file paths under root, each relative to
// root. Directories are not listed (an empty directory is not part of git's
// content identity anyway).
func relFiles(root string) (map[string]struct{}, error) {
	out := map[string]struct{}{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		out[rel] = struct{}{}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// filesEqual reports whether two files have identical mode bits and identical
// bytes. The exec bit is part of the tree-SHA, so a mode change alone is a
// divergence.
func filesEqual(a, b string) (bool, error) {
	infoA, err := os.Stat(a)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", a, err)
	}

	infoB, err := os.Stat(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("stat %s: %w", b, err)
	}

	if infoA.Mode().Perm() != infoB.Mode().Perm() || infoA.Size() != infoB.Size() {
		return false, nil
	}

	//nolint:gosec // G304: a and b are paths within the resolved origin/repo trees.
	dataA, err := os.ReadFile(a)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", a, err)
	}

	//nolint:gosec // G304: see above; both paths are tool-controlled subtree members.
	dataB, err := os.ReadFile(b)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", b, err)
	}

	return bytes.Equal(dataA, dataB), nil
}

// writeLockEntry merges one skill's entry into the lock at repoRoot, preserving
// every other skill and pinning LockfileVersion to 1. It records origin (the
// resolved OWNER/REPO[@REF] the CLI vendored from) when non-empty, leaving any
// existing value in place otherwise.
func writeLockEntry(repoRoot, name, origin string, result AddResult) error {
	lock, err := ReadLock(repoRoot)
	if err != nil {
		return err
	}

	lock.LockfileVersion = 1
	if origin != "" {
		lock.Origin = origin
	}

	if lock.Skills == nil {
		lock.Skills = map[string]LockEntry{}
	}

	lock.Skills[name] = LockEntry{
		Version: result.Version,
		Commit:  result.Commit,
		TreeSha: result.TreeSha,
		Path:    result.Path,
	}

	return WriteLock(repoRoot, lock)
}

// copyTreePreservingModes recursively copies src to dst byte-identically,
// preserving each file's mode (the exec bit is part of the tree SHA) and
// creating directories with 0o755. It injects nothing.
func copyTreePreservingModes(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}

			return nil
		}

		return copyFilePreservingMode(path, target)
	})
}

// copyFilePreservingMode copies a single regular file byte-identically and then
// chmods the destination to match the source's mode bits.
func copyFilePreservingMode(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	//nolint:gosec // G304: src is a file within the resolved origin subtree.
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}

	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}

	//nolint:gosec // G304: dst is within the repo's canonical .agents/skills root.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return fmt.Errorf("copy %s: %w", dst, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dst, err)
	}

	if err := os.Chmod(dst, info.Mode()); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}

	return nil
}
