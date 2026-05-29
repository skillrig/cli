// Package config holds skillrig's origin/config business logic: the Origin
// value type, reading and writing config.toml, and the single origin resolver.
// It is presentation-free — callers in internal/cli format the results.
package config

import (
	"fmt"
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

// originErrFmt is the shared usage-error format for a malformed origin. It names
// the expected shape (including the optional ref) and echoes the offending value
// (FR-012 / amendment FR-018).
const originErrFmt = "invalid origin %q: expected OWNER/REPO[@REF] (e.g. my-org/my-skills or my-org/my-skills@main)"

// Origin is an org's skill source in OWNER/REPO[@REF] form. It is the single
// value this feature reads, validates, records, and resolves. Ref is optional:
// when set it pins the origin to a branch (for an origin, a moving pointer the
// consumer tracks — distinct from the immutable tag/SHA a *skill* is pinned to);
// an empty Ref means the origin's default branch.
type Origin struct {
	Owner string
	Repo  string
	Ref   string
}

// String renders the origin as "Owner/Repo", appending "@Ref" when a ref is
// set. The zero Origin (the SourceNone sentinel) renders as "" so a "no origin"
// result never stringifies to a misleading "/" that looks configured.
func (o Origin) String() string {
	if o.Owner == "" && o.Repo == "" {
		return ""
	}

	s := o.Owner + "/" + o.Repo
	if o.Ref != "" {
		s += "@" + o.Ref
	}

	return s
}

// ParseOrigin trims surrounding whitespace and validates s against the
// OWNER/REPO[@REF] shape. The optional ref is split on the first '@' (the
// owner/repo charset excludes '@', so the split is unambiguous) and validated
// against refPattern; a trailing '@' with no ref is rejected. On failure it
// returns a usage error that names the expected format and echoes the offending
// value (FR-012). A blank string is rejected; callers that treat blank as
// "unset" (e.g. SKILLRIG_ORIGIN) must check before calling.
func ParseOrigin(s string) (Origin, error) {
	trimmed := strings.TrimSpace(s)

	ownerRepo, ref, hasRef := strings.Cut(trimmed, "@")
	if !originPattern.MatchString(ownerRepo) {
		return Origin{}, fmt.Errorf(originErrFmt, s)
	}

	if hasRef && !refPattern.MatchString(ref) {
		return Origin{}, fmt.Errorf(originErrFmt, s)
	}

	owner, repo, _ := strings.Cut(ownerRepo, "/")

	return Origin{Owner: owner, Repo: repo, Ref: ref}, nil
}
