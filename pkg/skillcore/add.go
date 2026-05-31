package skillcore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

// AddOptions configures Add. The CLI supplies an already-resolved origin and
// has classified its form (local-path vs remote OWNER/REPO) by populating the
// coordinates below; skillcore neither resolves origins nor reads config.
//
// Form selection: a non-empty Owner AND Repo selects the REMOTE form (Add
// fetches the skill subtree over git from https://github.com/Owner/Repo);
// otherwise Add uses the LOCAL-PATH form against the OriginDir checkout (002
// behavior, unchanged). The two forms are mutually exclusive — the remote
// coordinates are simply absent for a local origin.
type AddOptions struct {
	// OriginDir and Ref drive the LOCAL-PATH form: OriginDir is the local origin
	// checkout, Ref the revision to read (empty → HEAD upstream). Ignored when
	// remote coordinates are set.
	OriginDir string
	Ref       string
	Skill     string
	RepoRoot  string
	// Origin is the resolved origin reference (OWNER/REPO[@REF]) the CLI
	// resolved this add from. skillcore records it verbatim in the lock's
	// top-level origin field; it does not parse or resolve it (presentation- and
	// resolution-free). Empty leaves any existing lock origin untouched.
	Origin string
	// Owner and Repo are the remote origin's OWNER/REPO halves. They name the
	// origin in error reporting and, when RepoURL is empty, derive the GitHub
	// HTTPS clone URL. They are empty for a file:// local origin (which carries
	// only RepoURL).
	Owner string
	Repo  string
	// RepoURL is the git transport target for the remote-fetch form when set —
	// the origin's file://<path> (FR-011, the file:// test substrate) or any
	// caller-supplied URL. Empty means "derive https://github.com/Owner/Repo.git".
	// The CLI fills it from config.Origin.CloneURL().
	RepoURL string
	// Local marks RepoURL as a file:// (local) target: no GitHub token is
	// resolved for the fetch, and its failures are never the remote
	// auth/private-not-found classes. The CLI sets it from config.Origin.IsLocal().
	Local bool
	// SkillPath is the repo-relative subtree to fetch in the remote form (the
	// catalog's path, e.g. "skills/<skill>"). Empty defaults to the conventional
	// skills/<skill>. Unused by the local-path form.
	SkillPath string
	// Pin is the optional --pin reference for the remote form: a bare semver
	// (^v?SEMVER$) is expanded via the name-vSEMVER tag scheme to
	// <skill>-v<semver>; anything else is a literal git ref or commit SHA passed
	// through unchanged (C3). Empty means "use Ref" (the origin @ref branch).
	Pin    string
	Force  bool
	DryRun bool
}

// isRemote reports whether opts selects the remote-fetch form: an explicit
// RepoURL (the file:// local origin / test substrate) OR both OWNER and REPO
// halves of a remote origin reference. The two forms are mutually exclusive, so
// neither marker present means the LOCAL-PATH checkout form (002).
func (opts AddOptions) isRemote() bool {
	return opts.RepoURL != "" || (opts.Owner != "" && opts.Repo != "")
}

// cloneURL derives the git transport target for the remote-fetch form: an
// explicit RepoURL (the file:// origin) when set, else the GitHub HTTPS URL for
// OWNER/REPO. The token is never embedded here — git.go injects it via
// http.extraHeader — so the URL is safe to surface in diagnostics.
func (opts AddOptions) cloneURL() string {
	if opts.RepoURL != "" {
		return opts.RepoURL
	}

	return "https://" + defaultGitHubHost + "/" + opts.Owner + "/" + opts.Repo + ".git"
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

// acquired is the form-independent result of locating a skill's source: the
// on-disk directory to vendor from, its git-canonical tree-SHA, the resolved
// upstream commit, and the human-readable version/tag to record in the lock.
// Both the local-path and remote-fetch forms produce one, so the vendor + lock
// path downstream is identical (AP-04: one byte-identical vendor, one tree-SHA,
// one lock writer).
type acquired struct {
	srcDir  string
	treeSha string
	commit  string
	version string
	// cleanup removes any temp checkout the acquisition created (remote form). Add
	// always defers it, so it is never nil — the local-path form returns a no-op
	// (it vendors from the in-repo origin checkout, with nothing to remove).
	cleanup func()
}

// Add vendors one skill into opts.RepoRoot's canonical .agents/skills/<name>/,
// byte-identical and mode-preserving, then writes/updates the lock. The source
// is the local origin checkout at opts.OriginDir (local-path form) or a remote
// OWNER/REPO fetched over git (remote form, when opts.Owner and opts.Repo are
// set); both forms converge on the same vendor + lock path. It refuses a
// divergent overwrite unless opts.Force, writes nothing when opts.DryRun, and is
// idempotent on identical content.
func Add(opts AddOptions) (AddResult, error) {
	if err := validateSkillName(opts.Skill); err != nil {
		return AddResult{}, err
	}

	acq, err := acquireSource(opts)
	if err != nil {
		return AddResult{}, err
	}

	defer acq.cleanup()

	if err := ensureNoSymlinks(acq.srcDir); err != nil {
		return AddResult{}, err
	}

	manifest, err := ParseManifest(filepath.Join(acq.srcDir, "SKILL.md"))
	if err != nil {
		return AddResult{}, err
	}

	destPath := vendorRoot + "/" + opts.Skill
	destDir := filepath.Join(opts.RepoRoot, ".agents", "skills", opts.Skill)

	action, err := resolvePlacement(opts, manifest.Name, acq.srcDir, destDir, acq.treeSha)
	if err != nil {
		return AddResult{}, err
	}

	version := manifest.Version
	if acq.version != "" {
		version = acq.version
	}

	result := AddResult{
		Name:    manifest.Name,
		Version: version,
		Path:    destPath,
		Commit:  acq.commit,
		TreeSha: acq.treeSha,
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

	if err := copyTreePreservingModes(acq.srcDir, destDir); err != nil {
		return AddResult{}, err
	}

	if err := writeLockEntry(opts.RepoRoot, manifest.Name, opts.Origin, result); err != nil {
		return AddResult{}, err
	}

	return result, nil
}

// acquireSource locates the skill's source for the selected form, computing the
// git-canonical tree-SHA, the resolved upstream commit, and (for a remote pin)
// the resolved version/tag. The skill name is validated by the caller (Add)
// before this runs, so opts.Skill is already a safe single path segment.
func acquireSource(opts AddOptions) (acquired, error) {
	if opts.isRemote() {
		return acquireRemote(opts)
	}

	return acquireLocal(opts)
}

// acquireLocal is the 002 local-path form: vendor from the in-repo origin
// checkout at opts.OriginDir. The tree-SHA and commit come from the local git
// repo; the version is the manifest's (acquired.version left empty so Add reads
// the manifest). The cleanup is a no-op — nothing temporary was created.
func acquireLocal(opts AddOptions) (acquired, error) {
	srcDir, err := locateSkillSource(opts)
	if err != nil {
		return acquired{}, err
	}

	originRelPath := "skills/" + opts.Skill

	treeSha, err := TreeSHA(opts.OriginDir, opts.Ref, originRelPath)
	if err != nil {
		return acquired{}, err
	}

	commit, err := revParse(opts.OriginDir, opts.Ref)
	if err != nil {
		return acquired{}, err
	}

	return acquired{
		srcDir:  srcDir,
		treeSha: treeSha,
		commit:  commit,
		cleanup: func() {},
	}, nil
}

// acquireRemote is the remote-fetch form. Before vendoring it GATES the origin's
// convention (FIX-4 / H1): it fetches the origin's index.json at the origin's
// @ref and CheckConventions its skillrigConvention EXACT-match (== 1, C1),
// returning *IncompatibleConventionError when the origin speaks a convention this
// binary does not implement — so a mismatching origin is refused before any
// subtree is fetched or written. It then resolves the pin to a concrete ref,
// fetches the skill subtree from the origin via FetchSkill (the ONE fetch impl,
// AP-04), and computes the tree-SHA over the fetched checkout with the same
// TreeSHA verify uses. The fetched temp dir is the caller's to remove, so the
// returned cleanup removes it.
//
// version records the human-readable label for the lock: the resolved tag when a
// pin was given (so `--pin v1.4.0` is honestly recorded as the tag it resolved
// to), otherwise empty so Add falls back to the manifest version at the fetched
// ref (data-model §3).
func acquireRemote(opts AddOptions) (acquired, error) {
	if err := gateRemoteConvention(opts); err != nil {
		return acquired{}, err
	}

	skillPath := opts.SkillPath
	if skillPath == "" {
		skillPath = "skills/" + opts.Skill
	}

	ref, version, pinned := resolvePin(opts.Skill, opts.Pin, opts.Ref)

	fetch, err := FetchSkill(context.Background(), FetchRequest{
		Owner:     opts.Owner,
		Repo:      opts.Repo,
		RepoURL:   opts.cloneURL(),
		Local:     opts.Local,
		Skill:     opts.Skill,
		SkillPath: skillPath,
		Ref:       ref,
		Pinned:    pinned,
	})
	if err != nil {
		return acquired{}, err
	}

	cleanup := func() { _ = os.RemoveAll(fetch.Dir) }

	treeSha, err := TreeSHA(fetch.Dir, fetch.Commit, skillPath)
	if err != nil {
		cleanup()

		return acquired{}, err
	}

	return acquired{
		srcDir:  filepath.Join(fetch.Dir, filepath.FromSlash(skillPath)),
		treeSha: treeSha,
		commit:  fetch.Commit,
		version: version,
		cleanup: cleanup,
	}, nil
}

// gateRemoteConvention fetches the origin's index.json at the origin's @ref (NOT
// the --pin, which addresses a skill tag, not the origin) through the one catalog
// acquisition path (FetchCatalog, AP-04) and enforces the exact-match convention
// gate (CheckConvention, C1) before any subtree is fetched or vendored (FIX-4).
// A convention mismatch surfaces as *IncompatibleConventionError, which the CLI
// maps via mapConventionError; a fetch failure surfaces as the same
// Auth/Unreachable/NotFound classes FetchCatalog already classifies, anchored on
// the origin identity. The catalog is fetched PER add — never cached.
func gateRemoteConvention(opts AddOptions) error {
	catalog, err := FetchCatalog(context.Background(), CatalogRequest{
		RepoURL: opts.cloneURL(),
		Origin:  opts.Origin,
		Ref:     opts.Ref,
		Local:   opts.Local,
	})
	if err != nil {
		return err
	}

	return CheckConvention(catalog.SkillrigConvention)
}

// bareSemver matches a bare semantic version, optionally v-prefixed (e.g.
// "v1.4.0" or "1.4.0"). A pin matching this is expanded via the name-vSEMVER tag
// scheme; anything else (a full tag, a commit SHA) is taken literally (C3).
var bareSemver = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// resolvePin maps a --pin value to the concrete git ref to fetch, the
// human-readable version/tag to record in the lock, and whether a pin was given
// (so a failed fetch of a pinned ref classifies as NoSuchVersionError, not a
// missing skill — C2). The single deterministic rule (C3):
//
//   - empty pin → fetch the origin's branch ref (fallbackRef); record no
//     explicit version (Add reads the manifest); not pinned.
//   - bare semver (^v?SEMVER^$) → expand via the name-vSEMVER tag scheme to
//     <skill>-v<semver>; that tag is both the ref and the recorded version.
//   - any other value → a literal git ref or commit SHA, passed through as both
//     the ref and the recorded version.
//
// So `--pin v1.4.0` and `--pin <skill>-v1.4.0` resolve to the same tag and thus
// the same commit/treeSha (SC-004).
func resolvePin(skill, pin, fallbackRef string) (ref, version string, pinned bool) {
	if pin == "" {
		return fallbackRef, "", false
	}

	if bareSemver.MatchString(pin) {
		tag := skill + "-v" + strings.TrimPrefix(pin, "v")

		return tag, tag, true
	}

	return pin, pin, true
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
