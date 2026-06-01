package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/skillrig/cli/internal/config"
	"github.com/skillrig/cli/pkg/skillcore"
)

// showCmd holds the show command's state and its injectable seams. Like search
// it is read-only and resolves the origin through the shared resolver; tests
// inject deterministic stubs (cwd, env).
type showCmd struct {
	opts  *globalOpts
	skill string

	// getwd returns the working directory. Defaults to os.Getwd.
	getwd func() (string, error)
	// env is the environment accessor used by the origin resolver.
	env config.Env
}

// newShowCmd builds the `skillrig show <skill>` command (Query pattern): print
// the full, human-readable record of ONE skill the resolved origin publishes —
// notably the COMPLETE, untruncated description that search abbreviates to a
// one-liner (issue #17). `info` is an alias (the second name the issue proposed,
// and the desire path an agent reaches for). Read-only, exit 0 on a found skill,
// exit 1 on a config/convention/reachability problem or an unknown skill name.
func newShowCmd(opts *globalOpts) *cobra.Command {
	sc := &showCmd{
		opts:  opts,
		getwd: osGetwd,
		env:   config.OSEnv,
	}

	cmd := &cobra.Command{
		Use:     "show <skill>",
		Aliases: []string{"info"},
		Short:   "Show one skill's full details from your configured origin",
		Long: "show prints the complete, human-readable record of a single skill published by\n" +
			"your configured origin: its full (untruncated) description, version, namespace,\n" +
			"topics, path, and backing-tool requirements. It is the human counterpart to\n" +
			"search — where search lists many skills with a one-line, truncated description,\n" +
			"show drills into one and prints the whole thing (an agent gets the same data from\n" +
			"search --json piped to jq).\n\n" +
			"show resolves the active origin (SKILLRIG_ORIGIN > project > global) and reads the\n" +
			"origin's catalog (index.json) exactly like search; the skill name is matched\n" +
			"exactly (the same canonical name add vendors by). It is read-only and needs no git\n" +
			"working tree — only a resolvable origin. Add --json for the complete record.",
		Example: "  # Show a skill's full details (alias: skillrig info <skill>)\n" +
			"  skillrig show terraform-plan-review\n\n" +
			"  # The complete record as JSON, for an agent or jq\n" +
			"  skillrig show terraform-plan-review --json",
		// A custom validator (not cobra.ExactArgs) so a misinvocation is
		// errors-as-navigation — what/why/fix + an example — instead of cobra's
		// terse "accepts 1 arg(s)" dead end (cli.md Principle 1/2).
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageShowArgs(len(args))
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			sc.skill = args[0]

			return sc.run(cmd)
		},
	}

	return cmd
}

// run resolves the origin, loads + gates the catalog (the same acquisition path
// search uses, AP-04), finds the named skill, and renders its full record. A
// missing origin/catalog or an incompatible convention map to the shared
// navigational errors (exit 1); an unknown skill name is its own what/why/fix
// usage error (exit 1) — distinct from search, where an empty *query* is exit 0.
func (sc *showCmd) run(cmd *cobra.Command) error {
	cwd, err := sc.getwd()
	if err != nil {
		return &UsageError{Msg: "cannot determine working directory\nwhy: " + err.Error(), Cause: err}
	}

	res, err := config.ResolveOrigin(cwd, sc.env)
	if err != nil {
		return &UsageError{Msg: "cannot resolve the active origin\nwhy: " + err.Error() + "\n" + missingOriginFix, Cause: err}
	}

	if res.Source == config.SourceNone {
		return usageNoOriginConfigured()
	}

	// A git repo is OPTIONAL (as for search): it only enables the local-checkout
	// fast-path for a remote origin's committed checkout. Outside a repo, repoRoot
	// is empty and loadCatalog fetches the catalog directly (FIX-7).
	repoRoot, err := gitToplevel(cmd.Context(), cwd)
	if err != nil {
		if !errors.Is(err, errNotGitRepo) {
			return mapSearchError(res.Origin.String(), err)
		}

		repoRoot = ""
	}

	catalog, err := loadCatalog(cmd.Context(), repoRoot, res.Origin)
	if err != nil {
		return mapSearchError(res.Origin.String(), err)
	}

	if err := skillcore.CheckConvention(catalog.SkillrigConvention); err != nil {
		return mapSearchError(res.Origin.String(), err)
	}

	entry, ok := skillcore.FindSkill(catalog, sc.skill)
	if !ok {
		return usageShowSkillNotFound(sc.skill, res.Origin.String())
	}

	return renderShowResult(cmd.OutOrStdout(), res.Origin.String(), entry, sc.opts.json)
}

// usageShowArgs builds the navigational usage error for a wrong show argument
// count (errors-as-navigation: what / why / fix + a concrete example), replacing
// cobra's bare "accepts 1 arg(s)" message.
func usageShowArgs(got int) *UsageError {
	return usageErrorf("show requires exactly one argument: the skill name\n"+
		"why: got %d argument(s)\n"+
		"fix: skillrig show <skill> (e.g. skillrig show terraform-plan-review); run skillrig search to list the origin's skills", got)
}

// usageShowSkillNotFound builds the 3-part error for a named skill the origin's
// catalog does not publish. Unlike search (an empty *query* is a clean exit 0),
// show is a point lookup of a NAMED skill, so an unknown name is a usage error
// the agent must correct — the fix points at search to list the real names.
func usageShowSkillNotFound(skill, origin string) *UsageError {
	return usageErrorf("skill %q not found in origin %s\n"+
		"why: the origin's catalog publishes no skill by that exact name\n"+
		"fix: run skillrig search to list the skills the origin publishes, then show one of those names", skill, origin)
}
