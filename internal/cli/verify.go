package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/skillrig/cli/pkg/skillcore"
)

// verifyCmd holds the verify command's flags and its injectable cwd seam.
type verifyCmd struct {
	opts *globalOpts

	// getwd returns the working directory. Defaults to os.Getwd.
	getwd func() (string, error)
}

// newVerifyCmd builds the `skillrig verify` command (Verification Gate pattern):
// prove THIS repo's vendored skills match their recorded versions. Offline,
// deterministic, read-only, exit-code driven (0 ok / 1 usage / 2 failure).
func newVerifyCmd(opts *globalOpts) *cobra.Command {
	vc := &verifyCmd{
		opts:  opts,
		getwd: osGetwd,
	}

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check THIS repo's vendored skills match their recorded versions (project scope)",
		Long: "verify checks the PROJECT's vendored skills (.agents/skills) against the\n" +
			"committed lock (.skillrig/skills-lock.json) — label-honesty (git tree-SHA)\n" +
			"+ orphan/completeness — offline and deterministic. PROJECT-SCOPE: it verifies\n" +
			"THIS repository, not global/user-scope skills. It is read-only (recomputes git\n" +
			"tree-SHAs; writes nothing) and needs no origin and no network.\n\n" +
			"Exit 0 ok / 1 usage / 2 verification failure.",
		Example: "  # Verify this repo's vendored skills match their recorded versions (project-scope CI gate)\n" +
			"  skillrig verify\n\n" +
			"  # Machine-readable per-skill verdicts for an agent / jq\n" +
			"  skillrig verify --json",
		// Custom validator (not cobra.NoArgs) so an extra positional yields
		// what/why/fix instead of cobra's "unknown command" dead end (cli.md P1/P2).
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageVerifyArgs(args)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return vc.run(cmd)
		},
	}

	return cmd
}

// run resolves the repo root, runs skillcore.Verify, renders the report to
// stdout, and returns the exit-driving error. A verification failure renders the
// report (so the report IS the message) and then returns the *VerifyFailure so
// exitCodeFor maps it to exit 2; a malformed lock / not-a-repo is a *UsageError
// (exit 1).
func (vc *verifyCmd) run(cmd *cobra.Command) error {
	cwd, err := vc.getwd()
	if err != nil {
		return usageCannotGetwd(err)
	}

	repoRoot, err := gitToplevel(cmd.Context(), cwd)
	if err != nil {
		return usageNotGitRepo(verifyNotGitRepoWhy, err)
	}

	report, err := skillcore.Verify(repoRoot)
	if err != nil {
		return vc.handleVerifyError(cmd, err)
	}

	return renderVerifyReport(cmd.OutOrStdout(), report, vc.opts.json)
}

// verifyNotGitRepoWhy is the rationale for verify's not-a-repo error.
const verifyNotGitRepoWhy = "tree-SHA recompute needs git"

// usageVerifyArgs builds the navigational usage error when verify is given
// positional arguments it does not take (errors-as-navigation: what / why / fix).
func usageVerifyArgs(args []string) *UsageError {
	return usageErrorf("verify takes no arguments\n"+
		"why: it verifies the whole repo (got: %s)\n"+
		"fix: run skillrig verify (add --json for machine-readable per-skill verdicts)",
		strings.Join(args, " "))
}

// handleVerifyError classifies skillcore.Verify's error. A *VerifyFailure is a
// per-skill finding: render the report to stdout (human or --json) and return
// the failure so exitCodeFor yields exit 2. A *LockError is a config/usage
// problem (exit 1); any other error is wrapped as a usage error.
func (vc *verifyCmd) handleVerifyError(cmd *cobra.Command, err error) error {
	var verifyFail *skillcore.VerifyFailure
	if errors.As(err, &verifyFail) {
		if renderErr := renderVerifyReport(cmd.OutOrStdout(), verifyFail.Report, vc.opts.json); renderErr != nil {
			return renderErr
		}

		return verifyFail
	}

	var lockErr *skillcore.LockError
	if errors.As(err, &lockErr) {
		return &UsageError{
			Msg: "cannot read .skillrig/skills-lock.json\n" +
				"why: " + lockErr.Cause.Error() + "\n" +
				"fix: check the file, or re-vendor with skillrig add",
			Cause: err,
		}
	}

	var gitErr *skillcore.GitError
	if errors.As(err, &gitErr) {
		return usageNotGitRepo(verifyNotGitRepoWhy, err)
	}

	return &UsageError{Msg: "verify failed\nwhy: " + err.Error(), Cause: err}
}
