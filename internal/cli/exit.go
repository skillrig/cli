package cli

import (
	"errors"
	"fmt"

	"github.com/skillrig/cli/pkg/skillcore"
)

// Load-bearing process exit codes (docs/design/cli.md). These are part of the
// CLI contract: scripts and agents branch on them, so their meanings are fixed.
const (
	// ExitOK signals success, including idempotent no-ops.
	ExitOK = 0
	// ExitUsage signals a usage or configuration error (bad flags, invalid
	// origin, no origin configured, unwritable config).
	ExitUsage = 1
	// ExitVerification signals a verification failure (`verify` found at least
	// one mismatch / orphan / missing / dirty skill). Mapped from a
	// *skillcore.VerifyFailure.
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

// exitCodeFor maps a returned error to a process exit code (a typed switch, so
// the gate's exit code can never diverge from the error class that produced it):
//   - nil                       → ExitOK (0)
//   - *skillcore.VerifyFailure  → ExitVerification (2)
//   - everything else           → ExitUsage (1)
//
// Code 3 (prerequisite) is reserved for a future `doctor` and never returned.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}

	var verifyFail *skillcore.VerifyFailure
	if errors.As(err, &verifyFail) {
		return ExitVerification
	}

	return ExitUsage
}
