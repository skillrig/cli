package cli

import "fmt"

// Load-bearing process exit codes (docs/design/cli.md). These are part of the
// CLI contract: scripts and agents branch on them, so their meanings are fixed.
const (
	// ExitOK signals success, including idempotent no-ops.
	ExitOK = 0
	// ExitUsage signals a usage or configuration error (bad flags, invalid
	// origin, no origin configured, unwritable config).
	ExitUsage = 1
	// ExitVerification is reserved for a future verification failure (e.g. a
	// `verify` command). Declared here so the meaning is stable; unused in this
	// feature.
	ExitVerification = 2
	// ExitPrereq is reserved for a future missing-prerequisite failure.
	// Declared here for stability; unused in this feature.
	ExitPrereq = 3
)

// UsageError marks an error as a usage or configuration problem that maps to
// ExitUsage. It preserves the raw cause so --verbose can surface it while the
// human-facing message stays an actionable what/why/fix string.
type UsageError struct {
	// Msg is the rendered what/why/fix message shown to the user.
	Msg string
	// Cause is the underlying error, preserved for --verbose; may be nil.
	Cause error
}

func (e *UsageError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}

	return e.Msg
}

func (e *UsageError) Unwrap() error { return e.Cause }

// usageErrorf builds a UsageError with no underlying cause from a format string.
func usageErrorf(format string, args ...any) *UsageError {
	return &UsageError{Msg: fmt.Sprintf(format, args...)}
}

// exitCodeFor maps a returned error to a process exit code. nil → ExitOK; every
// error in this feature's surface (usage/config) → ExitUsage. Codes 2/3 are
// reserved for later commands and never returned here.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}

	return ExitUsage
}
