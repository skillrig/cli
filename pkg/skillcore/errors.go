package skillcore

import (
	"fmt"
	"strconv"
	"strings"
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

// InvalidSkillNameError is returned when a skill name is not a single safe path
// segment — it is empty, "."/"..", or contains a path separator (so it could
// escape the canonical .agents/skills/<name> subtree). Add validates the name
// before any filesystem operation, so a traversal name (e.g. "../x") is refused
// before any copy or os.RemoveAll. Presentation-free: terse Error.
type InvalidSkillNameError struct {
	Skill string
}

func (e *InvalidSkillNameError) Error() string {
	return fmt.Sprintf("invalid skill name %q", e.Skill)
}

// SymlinkUnsupportedError is returned when the origin skill subtree contains a
// symlink. Following symlinks could read content outside the subtree and would
// break the byte-identical / git-canonical vendoring guarantee (git records a
// symlink as a link, not its target's content), so this slice refuses them
// outright. Path is the offending symlink, relative to the skill dir.
// Presentation-free: terse Error.
type SymlinkUnsupportedError struct {
	Path string
}

func (e *SymlinkUnsupportedError) Error() string {
	return fmt.Sprintf("symlink not supported in vendored skill: %q", e.Path)
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

// AuthError is returned when a remote fetch cannot authenticate against the
// origin — either a credential was presented and REJECTED (git reported
// "Authentication failed" / "Invalid username or token") or the origin REQUIRED
// a credential that was unavailable and, with prompts disabled (issue #25), git
// could not obtain one ("could not read Username/Password", "terminal prompts
// disabled", or macOS's "Device not configured"). Both are the documented
// "private origin, no/invalid token" class. It is distinct from *NotFoundError
// (which GitHub returns for a private repo when it answers a clean 404). Origin
// is the OWNER/REPO[@REF] reference being reached. Presentation-free: the CLI
// renders the what/why/fix prose. Cause carries the raw *GitError, surfaced
// under --verbose.
type AuthError struct {
	Origin string
	Cause  error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication failed reaching %q", e.Origin)
}

func (e *AuthError) Unwrap() error {
	return e.Cause
}

// UnreachableError is returned when a remote fetch cannot reach the origin host
// — git reported "Could not resolve host" or "Failed to connect". It signals a
// network/connectivity problem (or a misspelled origin), as opposed to an
// auth/permission or missing-repo failure. Origin is the OWNER/REPO[@REF]
// reference. Presentation-free; Cause carries the raw *GitError for --verbose.
type UnreachableError struct {
	Origin string
	Cause  error
}

func (e *UnreachableError) Error() string {
	return fmt.Sprintf("could not reach %q", e.Origin)
}

func (e *UnreachableError) Unwrap() error {
	return e.Cause
}

// NotFoundError is returned when the origin repository (or a requested skill
// subtree within it) does not exist — git reported "repository not found". It is
// deliberately distinct from *SkillNotFoundError (the local-path origin IS
// checked out but lacks the skill) and from *NoSuchVersionError (the skill
// exists but the pinned version does not). The GitHub subtlety: a private repo
// reached without a resolved token also reports "not found", so Authenticated
// records whether a token was presented — the CLI adds an "if private,
// authenticate" hint when it was not. Presentation-free; Cause carries the raw
// *GitError for --verbose.
type NotFoundError struct {
	Origin        string
	Skill         string
	Authenticated bool
	Cause         error
}

func (e *NotFoundError) Error() string {
	if e.Skill != "" {
		return fmt.Sprintf("%q not found in %q", e.Skill, e.Origin)
	}

	return fmt.Sprintf("%q not found", e.Origin)
}

func (e *NotFoundError) Unwrap() error {
	return e.Cause
}

// NoSuchVersionError is returned when a --pin reference resolves to no existing
// tag or commit in the origin — the skill exists, but the requested version does
// not. It is a distinct type (not *NotFoundError) so callers/CI can branch on a
// bad pin versus a missing skill rather than on prose (C2). Ref is the
// unresolved pin as the user supplied it. Presentation-free; Cause carries the
// raw *GitError for --verbose.
type NoSuchVersionError struct {
	Skill string
	Ref   string
	Cause error
}

func (e *NoSuchVersionError) Error() string {
	return fmt.Sprintf("%q has no version %q", e.Skill, e.Ref)
}

func (e *NoSuchVersionError) Unwrap() error {
	return e.Cause
}

// IncompatibleConventionError is returned when the origin's catalog declares a
// skillrigConvention this binary does not support. The gate is exact-match (C1):
// only convention 1 is accepted, so any other value — a higher version, a lower
// version, or an absent/zero field — is incompatible. Found is the value read
// from the origin; Supported is the single version this tool implements.
// Presentation-free: the CLI renders the upgrade/mismatch guidance.
type IncompatibleConventionError struct {
	Found     int
	Supported int
}

func (e *IncompatibleConventionError) Error() string {
	return fmt.Sprintf(
		"origin uses convention v%d (this tool supports exactly v%d)",
		e.Found,
		e.Supported,
	)
}

// ClassifyGitError maps a raw *GitError from a remote fetch onto a typed,
// renderable failure by matching its captured stderr. The three network classes
// are the only ones classified here; an unrecognized stderr returns the original
// *GitError unchanged (the CLI surfaces it raw under --verbose). Classification
// lives in skillcore (the fetch layer), never in internal/cli — the prose
// what/why/fix is the CLI's job, the failure CLASS is skillcore's. The returned
// typed errors wrap err so --verbose can still reach the raw git output.
func ClassifyGitError(err *GitError) error {
	if err == nil {
		return nil
	}

	stderr := err.Stderr

	switch {
	case containsAny(
		stderr,
		"Authentication failed",
		"Invalid username or token",
		// The origin required a credential we could not supply, and with prompts
		// disabled (noninteractiveEnv, issue #25) git aborted instead of hanging.
		// git emits "could not read Username/Password for '<url>': <reason>", where
		// the <reason> tail varies by platform — "terminal prompts disabled" with
		// GIT_TERMINAL_PROMPT=0, macOS's "Device not configured" (ENXIO reading
		// /dev/tty), a bare errno, etc. We anchor ONLY on the stable "could not read
		// Username/Password" prefix, never the variable tail: that classifies every
		// credential-read failure regardless of platform AND keeps a generic tail
		// string (e.g. "Device not configured" / "terminal prompts disabled") seen
		// OUTSIDE a credential read from being force-classified as auth. The prefix
		// carries no host/connect or "not found" anchor (so without this it would
		// fall through to a raw, misleading *GitError) and is case-distinct from the
		// "Could not read from remote repository" local-unreachable anchor below.
		"could not read Username",
		"could not read Password",
	):
		return &AuthError{Cause: err}
	case containsAny(
		stderr,
		"Could not resolve host",
		"Failed to connect",
		// Local (file://-or-path) origins that are missing or not a repo fail
		// with these anchors instead of a host/connect error; neither contains
		// "not found", so isRepoNotFound is unaffected and auth is matched above.
		"Could not read from remote repository",
		"does not appear to be a git repository",
	):
		return &UnreachableError{Cause: err}
	case isRepoNotFound(stderr):
		return &NotFoundError{Cause: err}
	default:
		return err
	}
}

// isRepoNotFound reports whether git stderr signals a missing repository. git
// emits the URL between "repository" and "not found"
// (e.g. `repository 'https://…/' not found`) and a separate, capitalized
// "Remote: Repository not found." line — so a literal "repository not found"
// substring matches neither. Match the lowercased text on both anchors instead.
func isRepoNotFound(stderr string) bool {
	lower := strings.ToLower(stderr)

	return strings.Contains(lower, "repository") &&
		strings.Contains(lower, "not found")
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}

	return false
}
