package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newTestInitCmd builds an init command with injected seams: a stubbed
// interactivity flag, a fixed cwd, and an empty env. This is the quickstart's
// sanctioned "interactive shim" — it signals interactive mode in-process so the
// prompt path is exercised deterministically without a pty.
func newTestInitCmd(t *testing.T, interactive bool, cwd string) (*initCmd, *cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	ic := &initCmd{
		opts:        &globalOpts{},
		interactive: func() bool { return interactive },
		getwd:       func() (string, error) { return cwd, nil },
		env:         func(string) string { return "" },
	}

	var out, errBuf bytes.Buffer

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	return ic, cmd, &out, &errBuf
}

// TestInit_PromptInteractive maps to quickstart TestQuickstart_PromptInteractive
// (US1 / FR-006a). A pty is unavailable (project keeps deps minimal), so the
// interactive prompt is covered in-process via the shim.
func TestInit_PromptInteractive(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	ic, cmd, out, errBuf := newTestInitCmd(t, true, cwd)
	cmd.SetIn(strings.NewReader("my-org/my-skills\n"))

	if err := ic.run(cmd); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(errBuf.String(), originPromptLabel) {
		t.Errorf("stderr missing prompt %q, got: %q", originPromptLabel, errBuf.String())
	}

	if !strings.Contains(out.String(), "my-org/my-skills") {
		t.Errorf("stdout missing bound origin, got: %q", out.String())
	}

	got, err := os.ReadFile(filepath.Join(cwd, ".skillrig", "config.toml"))
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}

	if string(got) != "origin = 'my-org/my-skills'\n" {
		t.Errorf("config = %q, want origin = 'my-org/my-skills'", got)
	}
}

// TestInit_PromptEmptyInputErrors covers the interactive-but-empty path: a
// blank line is a usage error, no retry loop (research D5).
func TestInit_PromptEmptyInputErrors(t *testing.T) {
	t.Parallel()

	ic, cmd, _, _ := newTestInitCmd(t, true, t.TempDir())
	cmd.SetIn(strings.NewReader("\n"))

	err := ic.run(cmd)
	if err == nil {
		t.Fatal("expected usage error for empty prompt input")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("error %T is not a *UsageError", err)
	}
}

// TestInit_NonInteractiveFlagOverridesTTY is the precise FR-006c assertion the
// quickstart's pty scenario would make: with interactive() == true (a TTY is
// present) but --non-interactive set, init must fail fast WITHOUT prompting.
func TestInit_NonInteractiveFlagOverridesTTY(t *testing.T) {
	t.Parallel()

	ic, cmd, _, errBuf := newTestInitCmd(t, true /* interactive TTY present */, t.TempDir())
	ic.nonInteractive = true

	cmd.SetIn(strings.NewReader("my-org/my-skills\n")) // input that must be ignored

	err := ic.run(cmd)
	if err == nil {
		t.Fatal("expected usage error with --non-interactive and no --origin, even on a TTY")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("error %T is not a *UsageError", err)
	}

	if !strings.Contains(usageErr.Msg, "--non-interactive") {
		t.Errorf("error why should cite --non-interactive, got: %q", usageErr.Msg)
	}

	if strings.Contains(errBuf.String(), originPromptLabel) {
		t.Errorf("--non-interactive must not prompt even on a TTY, stderr: %q", errBuf.String())
	}
}

// TestInit_MalformedOriginMessage pins that the user-facing expected-format
// guidance for a malformed origin is rendered in the CLI layer — config returns
// a presentation-free *config.InvalidOriginError (Qodo rule 783432). Asserts the
// three-part what/why/fix at the unit level (quickstart covers it end-to-end).
func TestInit_MalformedOriginMessage(t *testing.T) {
	t.Parallel()

	ic, cmd, _, _ := newTestInitCmd(t, false, t.TempDir())
	ic.origin = "not-a-valid-origin"

	err := ic.run(cmd)
	if err == nil {
		t.Fatal("expected usage error for malformed origin")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("error %T is not a *UsageError", err)
	}

	for _, want := range []string{"not-a-valid-origin", "OWNER/REPO", "my-org/my-skills"} {
		if !strings.Contains(usageErr.Msg, want) {
			t.Errorf("message %q missing %q (what/why/fix must be rendered in cli)", usageErr.Msg, want)
		}
	}
}
