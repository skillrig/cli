package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/skillrig/cli/internal/config"
)

// originPromptLabel is the single interactive prompt (research D5: one stdlib
// bufio read on stderr, no retry loop).
const originPromptLabel = "Origin (OWNER/REPO): "

// missingOriginFix is the shared fix line for the no-origin error paths.
const missingOriginFix = "fix: pass --origin OWNER/REPO (e.g. --origin my-org/my-skills) or set SKILLRIG_ORIGIN"

// initCmd holds the init command's flags and its injectable seams. Production
// uses the os-backed defaults; tests inject deterministic stubs (interactivity,
// cwd, env) so the prompt and write-target logic are testable without a TTY.
type initCmd struct {
	opts           *globalOpts
	origin         string
	global         bool
	nonInteractive bool

	// interactive reports whether stdin is an interactive terminal. Defaults to
	// stdinIsTTY; overridden in tests to exercise the prompt path.
	interactive func() bool
	// getwd returns the working directory. Defaults to os.Getwd.
	getwd func() (string, error)
	// env is the environment accessor used for the global config path.
	env config.Env
}

// newInitCmd builds the `skillrig init` command (Environment pattern): bind a
// repo (or the per-user global default) to an existing origin.
func newInitCmd(opts *globalOpts) *cobra.Command {
	ic := &initCmd{
		opts:        opts,
		interactive: stdinIsTTY,
		getwd:       os.Getwd,
		env:         config.OSEnv,
	}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bind this repo (or your global default) to an origin",
		Long: "Bind a repository — or your per-user global default — to an existing origin,\n" +
			"the OWNER/REPO that hosts your team's agent skills, by recording it in config.\n" +
			"init is idempotent and consume-only: it does not create or scaffold an origin.\n\n" +
			"Without --origin, init prompts on an interactive terminal; with --non-interactive\n" +
			"(or no TTY) it fails fast instead of prompting, so scripts and agents never block.",
		Example: "  # Bind the current repo to an existing origin\n" +
			"  skillrig init --origin my-org/my-skills\n\n" +
			"  # Set your personal default origin (used when a repo has none)\n" +
			"  skillrig init --origin my-org/my-skills --global",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return ic.run(cmd)
		},
	}

	cmd.Flags().StringVar(&ic.origin, "origin", "", "origin to bind, OWNER/REPO (prompted if omitted on a TTY)")
	cmd.Flags().BoolVar(&ic.global, "global", false, "write the per-user global default instead of the repo config")
	cmd.Flags().BoolVar(&ic.nonInteractive, "non-interactive", false, "never prompt; fail fast if --origin is missing")

	return cmd
}

// run executes the bind: determine the write target and origin, validate, then
// write atomically (skipping the write when the origin is unchanged) and emit
// the result.
func (ic *initCmd) run(cmd *cobra.Command) error {
	target, scope, err := ic.writeTarget(cmd)
	if err != nil {
		return err
	}

	target = absOrSelf(target)

	raw, err := ic.resolveOriginInput(cmd)
	if err != nil {
		return err
	}

	origin, err := config.ParseOrigin(raw)
	if err != nil {
		return &UsageError{Msg: err.Error(), Cause: err}
	}

	written, err := writeIfChanged(target, origin)
	if err != nil {
		return &UsageError{
			Msg:   fmt.Sprintf("cannot write %s\nwhy: %v\nfix: check directory permissions and path", target, err),
			Cause: err,
		}
	}

	return renderBindResult(cmd.OutOrStdout(), bindResult{
		OK:         true,
		Origin:     origin.String(),
		Scope:      scope,
		ConfigPath: target,
		Written:    written,
	}, ic.opts.json)
}

// writeTarget resolves where to write and the scope label. --global → the
// per-user global path; otherwise the git repo root (offline rev-parse) with a
// cwd fallback (FR-005/FR-010).
func (ic *initCmd) writeTarget(cmd *cobra.Command) (path, scope string, err error) {
	if ic.global {
		path, err = config.GlobalConfigPath(ic.env)
		if err != nil {
			return "", "", &UsageError{Msg: "cannot locate global config path\nwhy: " + err.Error() + "\nfix: set HOME or XDG_CONFIG_HOME", Cause: err}
		}

		return path, "global", nil
	}

	cwd, err := ic.getwd()
	if err != nil {
		return "", "", &UsageError{Msg: "cannot determine working directory\nwhy: " + err.Error(), Cause: err}
	}

	return config.ProjectWriteTarget(cmd.Context(), cwd), "project", nil
}

// resolveOriginInput returns the raw origin string from --origin, an
// interactive prompt, or fails fast (FR-006a/FR-006c). It never blocks a
// non-interactive session.
func (ic *initCmd) resolveOriginInput(cmd *cobra.Command) (string, error) {
	if strings.TrimSpace(ic.origin) != "" {
		return ic.origin, nil
	}

	if ic.nonInteractive {
		return "", usageNoOrigin("non-interactive mode requested (--non-interactive)")
	}

	if !ic.interactive() {
		return "", usageNoOrigin("non-interactive session (no TTY)")
	}

	return ic.prompt(cmd)
}

// prompt writes the single origin prompt to stderr and reads one line from
// stdin (stdlib bufio, research D5). Empty input is a usage error — no retry.
func (ic *initCmd) prompt(cmd *cobra.Command) (string, error) {
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), originPromptLabel); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		return "", usageNoOrigin("no input received on the prompt")
	}

	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return "", usageNoOrigin("empty origin entered")
	}

	return line, nil
}

// usageNoOrigin builds the three-part "no origin given" usage error (what / why
// / fix) used by the non-interactive and empty-prompt paths (US3, FR-006a/c).
func usageNoOrigin(why string) *UsageError {
	return usageErrorf("no origin given\nwhy: %s\n%s", why, missingOriginFix)
}

// writeIfChanged compares the requested origin against the existing config and
// writes atomically only when it differs (idempotent re-bind, FR-008/FR-009).
// It reports whether a write happened.
func writeIfChanged(target string, origin config.Origin) (bool, error) {
	if existing, err := config.Load(target); err == nil {
		if cur, perr := config.ParseOrigin(existing.Origin); perr == nil && cur.String() == origin.String() {
			return false, nil
		}
	}

	if err := config.Save(target, origin); err != nil {
		return false, err
	}

	return true, nil
}

// stdinIsTTY reports whether stdin is an interactive character device (research
// D4). Note: this is true for /dev/null too, so non-interactive callers should
// pipe stdin (a pipe is not a char device).
func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

// absOrSelf returns the absolute form of p, or p unchanged if that fails.
func absOrSelf(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}

	return p
}
