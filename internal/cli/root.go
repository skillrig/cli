// Package cli wires the skillrig command tree (cobra) and the presentation
// layer. It must not contain origin/config business logic — that lives in
// internal/config. The CLI layer only parses flags, calls into config, and
// renders results or errors-as-navigation.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/skillrig/cli/pkg/skillcore"
)

// globalOpts holds the persistent, command-wide output flags. Shared by value
// of a pointer so subcommands read the parsed values at run time.
type globalOpts struct {
	json    bool
	verbose bool
}

// newRootCmd builds the root `skillrig` command and its subtree. It is exported
// indirectly via Execute; tests construct it directly to drive commands
// in-process with SetArgs/SetOut/SetErr.
func newRootCmd(opts *globalOpts) *cobra.Command {
	root := &cobra.Command{
		Use:   "skillrig",
		Short: "Manage your org's agent-skills library",
		Long: "skillrig points a repository (or your per-user default) at an origin —\n" +
			"the OWNER/REPO that hosts your team's agent skills — and resolves which\n" +
			"origin is active for any working directory.",
		Example: "  # Bind the current repo to an existing origin\n" +
			"  skillrig init --origin my-org/my-skills\n\n" +
			"  # Set your personal default origin\n" +
			"  skillrig init --origin my-org/my-skills --global",
		// We render errors and usage ourselves (errors-as-navigation,
		// docs/design/cli.md Principle 2 / Rule 5), so silence cobra's built-ins.
		SilenceUsage:  true,
		SilenceErrors: true,
		// Bare invocation prints help (cli.md Level-0 progressive discovery).
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().BoolVar(&opts.json, "json", false, "emit a complete JSON result on stdout instead of human text")
	root.PersistentFlags().BoolVar(&opts.verbose, "verbose", false, "print underlying paths / raw causes behind summaries and errors")

	// Subcommands are registered here as they are implemented (init in US1).
	registerSubcommands(root, opts)

	return root
}

// renderError prints an error as navigation: the actionable what/why/fix
// message, plus the raw cause when --verbose is set. Always to stderr; the
// write itself is best-effort.
//
// A *skillcore.VerifyFailure is rendered to NOTHING here: the verify command
// already wrote the per-skill report to stdout (the report IS the message), so
// printing a generic "error:" line on stderr would double-report. The non-nil
// error still drives the exit code (2) via exitCodeFor.
func renderError(w io.Writer, err error, verbose bool) {
	var verifyFail *skillcore.VerifyFailure
	if errors.As(err, &verifyFail) {
		return
	}

	_, _ = io.WriteString(w, errorMessage(err, verbose))
}

// errorMessage builds the what/why/fix text for an error. UsageError messages
// are shown verbatim (already authored as navigation); anything else gets a
// generic prefix. The raw cause is appended only under --verbose.
func errorMessage(err error, verbose bool) string {
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		msg := usageErr.Msg + "\n"

		if verbose && usageErr.Cause != nil {
			msg += fmt.Sprintf("  cause: %v\n", usageErr.Cause)
		}

		return msg
	}

	return fmt.Sprintf("error: %v\n", err)
}

// Execute builds and runs the root command, renders any error as navigation to
// stderr, and returns the process exit code. main is a thin shim over this.
func Execute() int {
	opts := &globalOpts{}
	root := newRootCmd(opts)

	err := root.Execute()
	if err != nil {
		renderError(os.Stderr, err, opts.verbose)
	}

	return exitCodeFor(err)
}

// registerSubcommands attaches the implemented subcommands to root. Kept
// separate so each user story wires its command here as it lands.
func registerSubcommands(root *cobra.Command, opts *globalOpts) {
	root.AddCommand(newInitCmd(opts))
	root.AddCommand(newSearchCmd(opts))
	root.AddCommand(newAddCmd(opts))
	root.AddCommand(newVerifyCmd(opts))
	root.AddCommand(newIndexCmd(opts))
}
