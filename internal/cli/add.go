package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/skillrig/cli/internal/config"
	"github.com/skillrig/cli/pkg/skillcore"
)

// addCmd holds the add command's flags and its injectable seams. Production uses
// the os-backed defaults; tests inject deterministic stubs (cwd, env).
type addCmd struct {
	opts   *globalOpts
	skill  string
	pin    string
	dryRun bool
	force  bool

	// getwd returns the working directory. Defaults to os.Getwd.
	getwd func() (string, error)
	// env is the environment accessor used by the origin resolver.
	env config.Env
}

// newAddCmd builds the `skillrig add <skill>` command (Vendor Mutation pattern):
// vendor a named skill from the repo's resolved origin into the canonical
// .agents/skills/<skill>/ and record its identity in the lock.
func newAddCmd(opts *globalOpts) *cobra.Command {
	ac := &addCmd{
		opts:  opts,
		getwd: osGetwd,
		env:   config.OSEnv,
	}

	cmd := &cobra.Command{
		Use:   "add <skill>",
		Short: "Vendor a skill from your configured origin into .agents/skills/",
		Long: "Vendor a named skill from this repo's configured origin into the canonical\n" +
			".agents/skills/<skill>/, recording its identity (version, commit, tree-SHA, path)\n" +
			"in .skillrig/skills-lock.json. add is consume-only: it resolves the active origin\n" +
			"(SKILLRIG_ORIGIN > project > global) exactly like every command and copies the\n" +
			"skill byte-identically, injecting nothing.\n\n" +
			"Two acquisition forms, chosen automatically and reported in the result:\n" +
			"  • Local — the configured origin OWNER/REPO is checked out at <repo-root>/OWNER/REPO;\n" +
			"    add reads that local checkout (resolved against the repo root, so it works from\n" +
			"    any subdirectory) — no network. Keep the checkout out of your index\n" +
			"    (e.g. echo 'my-org/' >> .git/info/exclude).\n" +
			"  • Remote — no local checkout exists; add fetches the skill subtree over git from\n" +
			"    the origin OWNER/REPO at the origin's @ref (or --pin), using a GitHub token from\n" +
			"    GH_TOKEN / GITHUB_TOKEN / `gh auth token` when one is available (public origins\n" +
			"    need none).\n\n" +
			"--pin acquires an immutable version instead of the origin's branch tip: a bare\n" +
			"semver (v1.4.0 / 1.4.0) expands via the origin's tag scheme to <skill>-v<semver>;\n" +
			"anything else is a literal git ref (a full tag or commit SHA). Both forms of the\n" +
			"same release resolve to the same content.\n\n" +
			"add is idempotent on identical content and refuses to overwrite a vendored skill\n" +
			"whose on-disk content diverges from the lock unless you pass --force. Requires a\n" +
			"git repository; commit the result, then run skillrig verify.",
		Example: "  # Vendor a skill from your configured origin\n" +
			"  skillrig add terraform-plan-review\n\n" +
			"  # Pin an immutable version (bare semver — expands via the origin's tag scheme)\n" +
			"  skillrig add terraform-plan-review --pin v1.4.0\n\n" +
			"  # Preview what would be vendored, writing nothing\n" +
			"  skillrig add terraform-plan-review --dry-run\n\n" +
			"  # Overwrite a locally-diverged copy with the origin's content\n" +
			"  skillrig add terraform-plan-review --force",
		// A custom validator (not cobra.ExactArgs) so a misinvocation is
		// errors-as-navigation — what/why/fix + an example — instead of cobra's
		// terse "accepts 1 arg(s), received 0" dead end (cli.md Principle 1/2).
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageAddArgs(len(args))
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ac.skill = args[0]

			return ac.run(cmd)
		},
	}

	cmd.Flags().StringVar(&ac.pin, "pin", "", "acquire an immutable version: a bare semver (v1.4.0) expands via the origin tag scheme, else a literal tag/SHA")
	cmd.Flags().BoolVar(&ac.dryRun, "dry-run", false, "report what would be vendored and recorded; write nothing")
	cmd.Flags().BoolVar(&ac.force, "force", false, "overwrite a vendored skill whose on-disk content diverges from the lock")

	return cmd
}

// run resolves the origin and the repo root, vendors the skill via skillcore,
// and renders the result. skillcore's typed errors are mapped to navigational
// *UsageError values (exit 1), preserving the raw cause for --verbose.
func (ac *addCmd) run(cmd *cobra.Command) error {
	cwd, err := ac.getwd()
	if err != nil {
		return &UsageError{Msg: "cannot determine working directory\nwhy: " + err.Error(), Cause: err}
	}

	res, err := config.ResolveOrigin(cwd, ac.env)
	if err != nil {
		return &UsageError{Msg: "cannot resolve the active origin\nwhy: " + err.Error() + "\n" + missingOriginFix, Cause: err}
	}

	if res.Source == config.SourceNone {
		return usageNoOriginConfigured()
	}

	repoRoot, err := gitToplevel(cmd.Context(), cwd)
	if err != nil {
		return usageNotGitRepo(addNotGitRepoWhy, err)
	}

	opts := addOptionsFor(res.Origin, repoRoot, ac)

	result, err := skillcore.Add(opts)
	if err != nil {
		return mapAddError(ac.skill, res.Origin.String(), err)
	}

	return renderAddResult(cmd.OutOrStdout(), result, ac.opts.json)
}

// addOptionsFor classifies the resolved origin's acquisition form (D3) and builds
// the skillcore.AddOptions for it. The form is chosen automatically:
//
//   - a file:// / filesystem-path LOCAL origin → remote-fetch form over a real
//     git transport against that path (RepoURL = origin.CloneURL(), Local), so a
//     local origin and the file:// test substrate are fetched without a checkout;
//   - a remote OWNER/REPO checked out at <repo-root>/OWNER/REPO → the 002
//     local-copy form reading that checkout;
//   - a remote OWNER/REPO with no checkout → remote-fetch form over GitHub.
//
// --pin and the destination/lock fields are common to all. skillcore gates the
// origin's convention before vendoring in the remote-fetch forms (FIX-4).
//
// AR-1: the local checkout is anchored to the repo root, not the process CWD — the
// destination (.agents/skills + the lock) is already repo-root-anchored via
// repoRoot, so anchoring the source there too makes add work from any subdirectory.
func addOptionsFor(origin config.Origin, repoRoot string, ac *addCmd) skillcore.AddOptions {
	_, ref := originDirRef(origin)

	opts := skillcore.AddOptions{
		Ref:      ref,
		Skill:    ac.skill,
		RepoRoot: repoRoot,
		Origin:   origin.String(),
		Pin:      ac.pin,
		Force:    ac.force,
		DryRun:   ac.dryRun,
	}

	// A file:// / path LOCAL origin has no OWNER/REPO checkout; fetch it over a
	// real git transport from its CloneURL (FR-011 + file:// substrate).
	if origin.IsLocal() {
		opts.RepoURL = origin.CloneURL()
		opts.Local = true

		return opts
	}

	originDir, _ := originDirRef(origin)
	localCheckout := filepath.Join(repoRoot, originDir)

	if isLocalCheckout(localCheckout) {
		opts.OriginDir = localCheckout

		return opts
	}

	// Remote form: setting Owner+Repo selects skillcore's remote-fetch path
	// (the catalog/conventional skills/<skill> subtree is resolved by skillcore).
	opts.Owner = origin.Owner
	opts.Repo = origin.Repo

	return opts
}

// isLocalCheckout reports whether dir is a directory on disk — the signal that the
// origin is checked out locally, selecting the local-copy form over a remote fetch.
func isLocalCheckout(dir string) bool {
	info, err := os.Stat(dir)

	return err == nil && info.IsDir()
}

// addNotGitRepoWhy is the project-scope rationale for add's not-a-repo error.
const addNotGitRepoWhy = "project-scope add vendors into the repo's canonical .agents/skills " +
	"and writes a lock that verify checks against git"

// originDirRef maps a resolved origin to the (local directory, ref) skillcore
// needs. This slice the origin value IS a local checkout path, so the OWNER/REPO
// portion is the directory and Ref (empty → HEAD) selects the revision.
func originDirRef(origin config.Origin) (dir, ref string) {
	dir = config.Origin{Owner: origin.Owner, Repo: origin.Repo}.String()

	ref = origin.Ref
	if ref == "" {
		ref = "HEAD"
	}

	return dir, ref
}

// usageAddArgs builds the navigational usage error for a wrong add argument
// count (errors-as-navigation: what / why / fix + a concrete example), replacing
// cobra's bare "accepts 1 arg(s)" message.
func usageAddArgs(got int) *UsageError {
	return usageErrorf("add requires exactly one argument: the skill name\n"+
		"why: got %d argument(s)\n"+
		"fix: skillrig add <skill> (e.g. skillrig add terraform-plan-review); run skillrig add --help for flags and examples", got)
}

// usageNoOriginConfigured builds the 3-part "no origin configured" usage error
// (contract add.md): what / why / fix.
func usageNoOriginConfigured() *UsageError {
	return usageErrorf("no origin configured\n" +
		"why: no SKILLRIG_ORIGIN / project / global origin\n" +
		"fix: skillrig init --origin OWNER/REPO or set SKILLRIG_ORIGIN")
}

// mapAddError maps skillcore's typed Add errors to navigational *UsageError
// values (exit 1), authoring the what/why/fix prose while preserving the raw
// cause for --verbose. origin is the OWNER/REPO[@REF] reference, anchored in the
// network/version error prose. An unexpected error is wrapped generically. All
// classes here map to exit 1 — the reserved exit 2 (verification) and 3
// (prerequisite) are never emitted from add.
func mapAddError(skill, origin string, err error) error {
	var invalidName *skillcore.InvalidSkillNameError
	if errors.As(err, &invalidName) {
		return &UsageError{
			Msg: fmt.Sprintf("invalid skill name %q\n", invalidName.Skill) +
				"why: a skill name must be a single path segment (no '/' or '..') so it stays inside .agents/skills/\n" +
				"fix: pass just the skill's directory name, e.g. skillrig add terraform-plan-review",
			Cause: err,
		}
	}

	var symlink *skillcore.SymlinkUnsupportedError
	if errors.As(err, &symlink) {
		return &UsageError{
			Msg: fmt.Sprintf("cannot vendor %q: it contains a symlink (%q)\n", skill, symlink.Path) +
				"why: symlinks are not supported in vendored skills this release — following them would break byte-identical, git-canonical vendoring\n" +
				"fix: remove the symlink in the origin skill, or vendor a skill without symlinks",
			Cause: err,
		}
	}

	var originMissing *skillcore.OriginNotFoundError
	if errors.As(err, &originMissing) {
		return &UsageError{
			Msg: fmt.Sprintf("origin checkout not found at %q\n", originMissing.OriginDir) +
				"why: this release reads the configured origin from a local checkout at that path, and it is absent\n" +
				fmt.Sprintf("fix: check out the origin there (git clone <origin-url> %q), or re-bind with skillrig init --origin OWNER/REPO", originMissing.OriginDir),
			Cause: err,
		}
	}

	var notFound *skillcore.SkillNotFoundError
	if errors.As(err, &notFound) {
		return &UsageError{
			Msg: fmt.Sprintf("skill %q not found in origin\n", skill) +
				fmt.Sprintf("why: no skills/%s/ at the configured origin\n", skill) +
				"fix: check the skill name against the origin",
			Cause: err,
		}
	}

	var overwrite *skillcore.OverwriteError
	if errors.As(err, &overwrite) {
		return &UsageError{
			Msg: fmt.Sprintf("refusing to overwrite %q\n", overwrite.Path) +
				"why: on-disk content diverges from the recorded fingerprint\n" +
				"fix: re-run with --force, or revert local edits",
			Cause: err,
		}
	}

	var remoteNotFound *skillcore.NotFoundError
	if errors.As(err, &remoteNotFound) {
		return mapNotFoundError(skill, remoteNotFound, err)
	}

	var noVersion *skillcore.NoSuchVersionError
	if errors.As(err, &noVersion) {
		return &UsageError{
			Msg: fmt.Sprintf("%q has no version %q\n", noVersion.Skill, noVersion.Ref) +
				"why: the pin does not resolve to a released tag or a commit in the origin\n" +
				"fix: run skillrig search for the current version, or --pin an existing tag",
			Cause: err,
		}
	}

	var authErr *skillcore.AuthError
	if errors.As(err, &authErr) {
		return mapAuthError(origin, err)
	}

	var unreachErr *skillcore.UnreachableError
	if errors.As(err, &unreachErr) {
		return mapUnreachableError(origin, err)
	}

	var convErr *skillcore.IncompatibleConventionError
	if errors.As(err, &convErr) {
		return mapConventionError(origin, convErr, err)
	}

	var gitErr *skillcore.GitError
	if errors.As(err, &gitErr) {
		return &UsageError{
			Msg: "add failed: git error\n" +
				"why: " + gitErr.Error() + "\n" +
				"fix: ensure git works in this directory and the origin is a valid checkout",
			Cause: err,
		}
	}

	return &UsageError{Msg: "add failed\nwhy: " + err.Error(), Cause: err}
}

// mapNotFoundError renders a remote *NotFoundError (the origin or the skill
// subtree is absent) as navigation. The D4 subtlety: GitHub returns "not found"
// (not 403) for a PRIVATE repo reached with no resolved token, so when the fetch
// was unauthenticated the fix adds the "if private, authenticate" hint — the
// agent must not be sent to re-check a skill name when the real problem is a
// missing credential. The raw *GitError is preserved for --verbose. Shared by
// add and search.
func mapNotFoundError(skill string, nf *skillcore.NotFoundError, cause error) error {
	what := fmt.Sprintf("skill %q not found in the origin\n", skill)
	if skill == "" {
		what = "the origin was not found\n"
	}

	fix := "fix: run skillrig search to list the skills the origin publishes"
	if !nf.Authenticated {
		fix += "; if the origin is private, authenticate first (gh auth login, or set GH_TOKEN / GITHUB_TOKEN)"
	}

	return &UsageError{
		Msg: what +
			"why: no such skill is published, or the origin is private and the fetch was unauthenticated\n" +
			fix,
		Cause: cause,
	}
}

// mapAuthError renders an *AuthError (a credential WAS presented and the origin
// rejected it — distinct from the not-found-because-private class) as navigation.
// Shared by add and search.
func mapAuthError(origin string, cause error) error {
	return &UsageError{
		Msg: fmt.Sprintf("authentication failed reaching %s\n", origin) +
			"why: the GitHub token presented was rejected (expired, revoked, or lacking access to a private origin)\n" +
			"fix: refresh credentials with gh auth login, or set a valid GH_TOKEN / GITHUB_TOKEN",
		Cause: cause,
	}
}

// mapUnreachableError renders an *UnreachableError (the origin host could not be
// resolved or connected to) as navigation. Shared by add and search.
func mapUnreachableError(origin string, cause error) error {
	return &UsageError{
		Msg: fmt.Sprintf("could not reach %s\n", origin) +
			"why: the origin host could not be resolved or connected to (offline, or a misspelled origin)\n" +
			"fix: check your network connection and the origin spelling (OWNER/REPO)",
		Cause: cause,
	}
}

// mapConventionError renders an *IncompatibleConventionError as navigation. The
// gate is exact-match (C1): the origin's catalog declares a convention this
// binary does not implement (a higher, lower, or absent/zero value all fail), so
// the fix is to align the tool and the origin. Shared by add and search.
func mapConventionError(origin string, ce *skillcore.IncompatibleConventionError, cause error) error {
	return &UsageError{
		Msg: fmt.Sprintf("%s uses skill convention v%d (this tool supports exactly v%d)\n", origin, ce.Found, ce.Supported) +
			"why: the origin's catalog declares a convention version this skillrig does not implement\n" +
			"fix: update skillrig, or check the origin's .skillrig-origin.toml convention_version",
		Cause: cause,
	}
}
