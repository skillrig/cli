// Package config holds skillrig's origin/config business logic: the Origin
// value type, reading and writing config.toml, and the single origin resolver.
// It is presentation-free — callers in internal/cli format the results.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// originPattern is the offline shape check for the OWNER/REPO part of an
// origin: two non-empty, slash-separated segments over the GitHub owner/repo
// charset (research D6). Existence/reachability is intentionally not checked
// here.
var originPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// refPattern is the offline shape check for the optional @REF: a non-empty
// string over the owner/repo charset plus '/', so branch names with slashes
// ("feature/x", "release/1.2") are accepted alongside tags ("v1.4.0") and
// commit SHAs. Like originPattern this is shape-only — whether the ref actually
// exists upstream is deliberately not checked (amendment 001-origin-ref-support,
// FR-019). It is general by design: detecting branch vs tag vs commit would need
// either heuristics that misfire on unusual names or a network lookup, both out
// of scope.
var refPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// InvalidOriginError marks a value that fails the OWNER/REPO[@REF] shape check.
// It carries only the offending value so internal/config stays presentation-free:
// the user-facing "expected …, e.g. …" guidance is rendered by internal/cli
// (FR-012 / amendment FR-018). Callers match it with errors.As.
type InvalidOriginError struct {
	Value string
}

func (e *InvalidOriginError) Error() string {
	return fmt.Sprintf("invalid origin %q", e.Value)
}

// Origin is an org's skill source in one of two forms (D3):
//
//   - REMOTE: OWNER/REPO[@REF] — Owner and Repo are set, Path is empty. Ref is
//     optional: when set it pins the origin to a branch (a moving pointer the
//     consumer tracks — distinct from the immutable tag/SHA a *skill* is pinned
//     to); an empty Ref means the origin's default branch.
//   - LOCAL: an explicit filesystem path or a file:// URL — Path is set, Owner
//     and Repo are empty (FR-011). This points at a local checkout/bare repo so
//     fetches read from disk instead of github.com; it is also the file://
//     substrate the remote-fetch path is tested against.
//
// The two forms are mutually exclusive. IsLocal reports which one a value is;
// CloneURL renders the git transport target for either.
type Origin struct {
	Owner string
	Repo  string
	Ref   string
	// Path is the local origin's filesystem path or file:// URL (LOCAL form).
	// Empty for the remote OWNER/REPO form.
	Path string
}

// IsLocal reports whether the origin is the LOCAL form (a filesystem path or
// file:// URL) rather than the remote OWNER/REPO form. The two are mutually
// exclusive, so a non-empty Path is the discriminant.
func (o Origin) IsLocal() bool {
	return o.Path != ""
}

// CloneURL renders the git transport target the fetch layer clones from. For a
// LOCAL origin it is the Path as a file:// URL (a bare/working-tree path becomes
// file://<path>, an already-file:// path passes through), so git runs a real
// transport handshake offline; for a REMOTE origin it is the GitHub HTTPS clone
// URL. The token (remote only) is never embedded here — git.go injects it via
// http.extraHeader — so the result is safe to surface in diagnostics.
func (o Origin) CloneURL() string {
	if o.IsLocal() {
		if strings.HasPrefix(o.Path, "file://") {
			return o.Path
		}

		return "file://" + o.Path
	}

	return "https://github.com/" + o.Owner + "/" + o.Repo + ".git"
}

// String renders the origin to its canonical configured form: a LOCAL origin is
// its Path verbatim (round-trips through ParseOrigin); a REMOTE origin is
// "Owner/Repo" with "@Ref" appended when a ref is set. The zero Origin (the
// SourceNone sentinel) renders as "" so a "no origin" result never stringifies
// to a misleading "/" that looks configured.
func (o Origin) String() string {
	if o.IsLocal() {
		return o.Path
	}

	if o.Owner == "" && o.Repo == "" {
		return ""
	}

	s := o.Owner + "/" + o.Repo
	if o.Ref != "" {
		s += "@" + o.Ref
	}

	return s
}

// ParseOrigin trims surrounding whitespace and classifies s into one of the two
// origin forms (D3, FR-011):
//
//   - LOCAL: a file:// URL, or a filesystem path (absolute "/…", explicit
//     "./"/"../", or "~"-rooted). These yield an Origin with Path set; a "~"
//     prefix is expanded against $HOME. No @REF split is applied — a local path
//     may legitimately contain '@', and the local form has no origin-level ref.
//   - REMOTE: bare OWNER/REPO[@REF]. The optional ref is split on the first '@'
//     (the owner/repo charset excludes '@', so the split is unambiguous) and
//     validated against refPattern; a trailing '@' with no ref is rejected.
//
// On failure it returns a typed *InvalidOriginError carrying the offending
// value; the user-facing expected-format guidance is rendered by internal/cli
// (FR-012). A blank string is rejected; callers that treat blank as "unset"
// (e.g. SKILLRIG_ORIGIN) must check before calling.
func ParseOrigin(s string) (Origin, error) {
	trimmed := strings.TrimSpace(s)

	if isLocalForm(trimmed) {
		path, err := normalizeLocalPath(trimmed)
		if err != nil {
			return Origin{}, &InvalidOriginError{Value: s}
		}

		return Origin{Path: path}, nil
	}

	ownerRepo, ref, hasRef := strings.Cut(trimmed, "@")
	if !originPattern.MatchString(ownerRepo) {
		return Origin{}, &InvalidOriginError{Value: s}
	}

	if hasRef && !refPattern.MatchString(ref) {
		return Origin{}, &InvalidOriginError{Value: s}
	}

	owner, repo, _ := strings.Cut(ownerRepo, "/")

	return Origin{Owner: owner, Repo: repo, Ref: ref}, nil
}

// isLocalForm reports whether s is the LOCAL origin form: a file:// URL or a
// path-shaped value (absolute, explicit-relative, or "~"-rooted). A bare
// OWNER/REPO stays the remote form — only these unambiguous path markers select
// local, so the two forms never collide.
func isLocalForm(s string) bool {
	return strings.HasPrefix(s, "file://") ||
		strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "~/") ||
		s == "~"
}

// normalizeLocalPath canonicalizes a LOCAL origin value to an absolute path or a
// file:// URL. A file:// URL passes through unchanged; a "~"/"~/" prefix is
// expanded against $HOME; a relative path is made absolute against the working
// directory so the recorded origin is stable regardless of where a later command
// runs. The git transport (CloneURL) turns a bare path into file://<path>.
func normalizeLocalPath(s string) (string, error) {
	if strings.HasPrefix(s, "file://") {
		return s, nil
	}

	if s == "~" || strings.HasPrefix(s, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		s = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(s, "~"), "/"))
	}

	return filepath.Abs(s)
}
