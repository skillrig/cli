// Package config holds skillrig's origin/config business logic: the Origin
// value type, reading and writing config.toml, and the single origin resolver.
// It is presentation-free — callers in internal/cli format the results.
package config

import (
	"fmt"
	"regexp"
	"strings"
)

// originPattern is the offline shape check for an origin: two non-empty,
// slash-separated segments over the GitHub owner/repo charset (research D6).
// Existence/reachability is intentionally not checked here.
var originPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// Origin is an org's skill source in OWNER/REPO form. It is the single value
// this feature reads, validates, records, and resolves.
type Origin struct {
	Owner string
	Repo  string
}

// String renders the origin as "Owner/Repo". The zero Origin (the SourceNone
// sentinel) renders as "" so a "no origin" result never stringifies to a
// misleading "/" that looks configured.
func (o Origin) String() string {
	if o.Owner == "" && o.Repo == "" {
		return ""
	}

	return o.Owner + "/" + o.Repo
}

// ParseOrigin trims surrounding whitespace and validates s against the
// OWNER/REPO shape. On failure it returns a usage error that names the expected
// format and echoes the offending value (FR-012). A blank string is rejected;
// callers that treat blank as "unset" (e.g. SKILLRIG_ORIGIN) must check before
// calling.
func ParseOrigin(s string) (Origin, error) {
	trimmed := strings.TrimSpace(s)
	if !originPattern.MatchString(trimmed) {
		return Origin{}, fmt.Errorf("invalid origin %q: expected OWNER/REPO (e.g. my-org/my-skills)", s)
	}

	owner, repo, _ := strings.Cut(trimmed, "/")

	return Origin{Owner: owner, Repo: repo}, nil
}
