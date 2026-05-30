package skillcore

import (
	"fmt"
	"strconv"
)

// VerifyFailure is returned by Verify when at least one verdict is not ok. It
// carries the full Report so callers can render it their own way and choose
// their own exit policy. It is presentation-free: Error is terse.
//
//nolint:errname // name fixed by the skillcore SDK contract (contracts/skillcore-sdk.md); a report-carrying failure, not an "XxxError".
type VerifyFailure struct {
	Report Report
}

func (e *VerifyFailure) Error() string {
	return "skillcore: verification failed (" +
		strconv.Itoa(len(e.Report.Verdicts)) + " verdicts)"
}

// LockError is returned when the lock file cannot be read or is malformed
// (unreadable, unparseable, or an unsupported lockfileVersion). It is a
// configuration/usage problem — the CLI maps it to exit 1 — distinct from a
// *VerifyFailure, which reports per-skill findings (exit 2). It is
// presentation-free: it carries the lock path and the raw underlying cause.
type LockError struct {
	Path  string
	Cause error
}

func (e *LockError) Error() string {
	return fmt.Sprintf("skillcore: invalid lock %q: %v", e.Path, e.Cause)
}

func (e *LockError) Unwrap() error {
	return e.Cause
}

// OriginNotFoundError is returned when the resolved local origin checkout does
// not exist at OriginDir — e.g. the user ran `skillrig init` (which records only
// an OWNER/REPO reference) but never checked the library out to its expected
// local path. It is deliberately distinct from *SkillNotFoundError (the origin
// IS present but the named skill is absent) so the CLI can tell the user to
// check out the origin rather than re-check the skill name (errors-as-navigation:
// do not conflate look-alike failure classes). Presentation-free: terse Error.
type OriginNotFoundError struct {
	OriginDir string
	Ref       string
}

func (e *OriginNotFoundError) Error() string {
	return fmt.Sprintf("origin checkout not found at %q", e.OriginDir)
}

// GitError is returned when a git invocation fails. It carries the process exit
// code and captured stderr, mirroring the gh/git client pattern, so the caller
// can render an environment error. It is presentation-free.
type GitError struct {
	ExitCode int
	Stderr   string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git failed (exit %d): %s", e.ExitCode, e.Stderr)
}
