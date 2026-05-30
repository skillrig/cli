package cli

import (
	"errors"
	"fmt"
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
			"in .skillrig/skills-lock.json. add is offline and consume-only: it resolves the\n" +
			"active origin (SKILLRIG_ORIGIN > project > global) exactly like every command and\n" +
			"copies the skill byte-identically, injecting nothing.\n\n" +
			"Local origin (this release): the configured origin OWNER/REPO is read from a local\n" +
			"git checkout at <repo-root>/OWNER/REPO — resolved against the repo root, so add\n" +
			"works from any subdirectory — not over the network. So `init --origin my-org/my-skills`\n" +
			"expects that library checked out at <repo-root>/my-org/my-skills; keep it out of your\n" +
			"index (e.g. echo 'my-org/' >> .git/info/exclude). Fetching a remote origin is a later,\n" +
			"additive mode.\n\n" +
			"add is idempotent on identical content and refuses to overwrite a vendored skill\n" +
			"whose on-disk content diverges from the lock unless you pass --force. Requires a\n" +
			"git repository; commit the result, then run skillrig verify.",
		Example: "  # Vendor a skill from your configured origin (a local checkout at ./OWNER/REPO)\n" +
			"  skillrig add terraform-plan-review\n\n" +
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

	originDir, ref := originDirRef(res.Origin)
	// AR-1: anchor the local origin checkout to the repo root, not the process
	// CWD. The destination (.agents/skills + the lock) is already repo-root-anchored
	// via repoRoot; leaving the origin source relative made `add` resolve it against
	// the CWD, so it failed from any subdirectory while the output still went to the
	// repo root. Joining with repoRoot makes both sides consistent — `add` now works
	// from anywhere in the repo.
	originDir = filepath.Join(repoRoot, originDir)

	result, err := skillcore.Add(skillcore.AddOptions{
		OriginDir: originDir,
		Ref:       ref,
		Skill:     ac.skill,
		RepoRoot:  repoRoot,
		Origin:    res.Origin.String(),
		Force:     ac.force,
		DryRun:    ac.dryRun,
	})
	if err != nil {
		return mapAddError(ac.skill, err)
	}

	return renderAddResult(cmd.OutOrStdout(), result, ac.opts.json)
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
// cause for --verbose. An unexpected error is wrapped generically.
func mapAddError(skill string, err error) error {
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
