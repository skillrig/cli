package cli

import (
	"errors"
	"fmt"

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
			"git checkout at ./OWNER/REPO — relative to the directory you run add from (your\n" +
			"repo root) — not over the network. So `init --origin my-org/my-skills` expects that\n" +
			"library checked out at ./my-org/my-skills; keep it out of your index (e.g. echo\n" +
			"'my-org/' >> .git/info/exclude). Fetching a remote origin is a later, additive mode.\n\n" +
			"add is idempotent on identical content and refuses to overwrite a vendored skill\n" +
			"whose on-disk content diverges from the lock unless you pass --force. Requires a\n" +
			"git repository; commit the result, then run skillrig verify.",
		Example: "  # Vendor a skill from your configured origin (a local checkout at ./OWNER/REPO)\n" +
			"  skillrig add terraform-plan-review\n\n" +
			"  # Preview what would be vendored, writing nothing\n" +
			"  skillrig add terraform-plan-review --dry-run\n\n" +
			"  # Overwrite a locally-diverged copy with the origin's content\n" +
			"  skillrig add terraform-plan-review --force",
		Args: cobra.ExactArgs(1),
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
	var notFound *skillcore.SkillNotFoundError
	if errors.As(err, &notFound) {
		return &UsageError{
			Msg: fmt.Sprintf("skill %q not found in origin\n", skill) +
				"why: no skills/" + skill + "/ at the configured origin\n" +
				"fix: check the skill name against the origin",
			Cause: err,
		}
	}

	var overwrite *skillcore.OverwriteError
	if errors.As(err, &overwrite) {
		return &UsageError{
			Msg: fmt.Sprintf("refusing to overwrite %s\n", overwrite.Path) +
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
