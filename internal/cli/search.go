package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/skillrig/cli/internal/config"
	"github.com/skillrig/cli/pkg/skillcore"
)

// catalogName is the origin's committed, machine-readable catalog file
// (index.json) that search reads, gates, and matches against (contract search.md).
const catalogName = "index.json"

// searchCmd holds the search command's flags and its injectable seams. Production
// uses the os-backed defaults; tests inject deterministic stubs (cwd, env).
type searchCmd struct {
	opts   *globalOpts
	query  []string
	topics []string

	// getwd returns the working directory. Defaults to os.Getwd.
	getwd func() (string, error)
	// env is the environment accessor used by the origin resolver.
	env config.Env
}

// newSearchCmd builds the `skillrig search [QUERY...]` command (Query pattern):
// discover skills published by the resolved origin by reading its catalog,
// gating the convention version, and matching deterministically. Read-only,
// exit 0 on any well-formed query (including an empty result), exit 1 on a
// config/convention/reachability problem.
func newSearchCmd(opts *globalOpts) *cobra.Command {
	sc := &searchCmd{
		opts:  opts,
		getwd: osGetwd,
		env:   config.OSEnv,
	}

	cmd := &cobra.Command{
		Use:   "search [QUERY...]",
		Short: "Discover skills published by your configured origin",
		Long: "search discovers the skills your configured origin publishes. It resolves the\n" +
			"active origin (SKILLRIG_ORIGIN > project > global) exactly like every command,\n" +
			"reads the origin's catalog (index.json), and matches deterministically: a free-text\n" +
			"QUERY is a case-insensitive token-AND substring over name+description+topics (a\n" +
			"skill matches only if EVERY term is present), and --topic adds an exact-string,\n" +
			"case-insensitive AND filter. Results are ordered by a fixed relevance bucket\n" +
			"(exact-name > name > topic > description) then by name — no fuzzy or learned\n" +
			"ranking. An empty result is success (exit 0); add --json for the complete record.\n\n" +
			"search is read-only and needs no git working tree — only a resolvable origin.",
		Example: "  # List every skill the origin publishes\n" +
			"  skillrig search\n\n" +
			"  # Free-text query (token-AND over name + description + topics)\n" +
			"  skillrig search terraform plan\n\n" +
			"  # Filter by topic (repeatable; AND across topics)\n" +
			"  skillrig search --topic aws --topic terraform",
		RunE: func(cmd *cobra.Command, args []string) error {
			sc.query = args

			return sc.run(cmd)
		},
	}

	cmd.Flags().StringArrayVar(&sc.topics, "topic", nil, "filter to skills carrying this topic (repeatable; AND across topics)")

	return cmd
}

// run resolves the origin, loads + gates the catalog, matches the query, and
// renders the two-level result. skillcore's typed errors are mapped to
// navigational *UsageError values (exit 1), preserving the raw cause for
// --verbose; a well-formed query — even one that matches nothing — is exit 0.
func (sc *searchCmd) run(cmd *cobra.Command) error {
	cwd, err := sc.getwd()
	if err != nil {
		return &UsageError{Msg: "cannot determine working directory\nwhy: " + err.Error(), Cause: err}
	}

	res, err := config.ResolveOrigin(cwd, sc.env)
	if err != nil {
		return &UsageError{Msg: "cannot resolve the active origin\nwhy: " + err.Error() + "\n" + missingOriginFix, Cause: err}
	}

	if res.Source == config.SourceNone {
		return usageNoOriginConfigured()
	}

	repoRoot, err := gitToplevel(cmd.Context(), cwd)
	if err != nil {
		return usageNotGitRepo(searchNotGitRepoWhy, err)
	}

	catalog, err := loadCatalog(repoRoot, res.Origin)
	if err != nil {
		return mapSearchError(res.Origin.String(), err)
	}

	if err := skillcore.CheckConvention(catalog.SkillrigConvention); err != nil {
		return mapSearchError(res.Origin.String(), err)
	}

	matches := skillcore.Search(catalog, sc.query, sc.topics)

	return renderSearchResult(cmd.OutOrStdout(), res.Origin.String(), matches, sc.opts.json)
}

// loadCatalog reads the origin's index.json (anchored at repoRoot) and parses
// it. This release resolves the origin to a local checkout at
// <repo-root>/OWNER/REPO (the same local form add uses), reading the catalog from
// disk; fetching it from a remote origin is an additive mode. Parse stays
// separate from the convention gate so a malformed catalog and an incompatible
// convention are distinct failures (contract search.md).
func loadCatalog(repoRoot string, origin config.Origin) (skillcore.Catalog, error) {
	originDir, _ := originDirRef(origin)
	catalogPath := filepath.Join(repoRoot, originDir, catalogName)

	//nolint:gosec // G304: path is repoRoot + the resolved origin owner/repo + a fixed file name, not attacker-controlled.
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return skillcore.Catalog{}, &catalogReadError{path: catalogPath, cause: err}
	}

	catalog, err := skillcore.ParseCatalog(data)
	if err != nil {
		return skillcore.Catalog{}, &catalogParseError{path: catalogPath, cause: err}
	}

	return catalog, nil
}

// searchNotGitRepoWhy is the rationale for search's not-a-repo error: the origin
// catalog is read from a checkout anchored at the repo root.
const searchNotGitRepoWhy = "the origin catalog is read from a checkout anchored at the repo root"

// catalogReadError marks the origin's catalog as unreadable (absent or no
// permission). It is presentation-free here only in that mapSearchError renders
// the what/why/fix; it carries the path and raw cause for --verbose.
type catalogReadError struct {
	path  string
	cause error
}

func (e *catalogReadError) Error() string {
	return fmt.Sprintf("reading catalog %q: %v", e.path, e.cause)
}
func (e *catalogReadError) Unwrap() error { return e.cause }

// catalogParseError marks the origin's catalog as malformed JSON.
type catalogParseError struct {
	path  string
	cause error
}

func (e *catalogParseError) Error() string {
	return fmt.Sprintf("parsing catalog %q: %v", e.path, e.cause)
}
func (e *catalogParseError) Unwrap() error { return e.cause }

// mapSearchError maps the failure classes search can surface to navigational
// *UsageError values (exit 1), authoring the what/why/fix prose while preserving
// the raw cause for --verbose. The convention mismatch, an unreachable/auth
// origin, and a malformed catalog are distinct messages so the agent debugs the
// real problem (errors-as-navigation; do not conflate look-alike classes).
func mapSearchError(origin string, err error) error {
	var convErr *skillcore.IncompatibleConventionError
	if errors.As(err, &convErr) {
		return mapConventionError(origin, convErr, err)
	}

	var authErr *skillcore.AuthError
	if errors.As(err, &authErr) {
		return mapAuthError(origin, err)
	}

	var unreachErr *skillcore.UnreachableError
	if errors.As(err, &unreachErr) {
		return mapUnreachableError(origin, err)
	}

	var notFound *skillcore.NotFoundError
	if errors.As(err, &notFound) {
		return mapNotFoundError("", notFound, err)
	}

	var readErr *catalogReadError
	if errors.As(err, &readErr) {
		return &UsageError{
			Msg: fmt.Sprintf("cannot read the origin catalog at %q\n", readErr.path) +
				"why: the origin has no index.json there (origin not checked out, or its catalog has not been generated)\n" +
				"fix: check out the origin at that path, or run skillrig index in the origin and commit index.json",
			Cause: err,
		}
	}

	var parseErr *catalogParseError
	if errors.As(err, &parseErr) {
		return &UsageError{
			Msg: fmt.Sprintf("the origin catalog at %q is malformed\n", parseErr.path) +
				"why: index.json is not valid JSON\n" +
				"fix: regenerate it with skillrig index in the origin, then commit the result",
			Cause: err,
		}
	}

	// A not-a-repo failure is already a *UsageError from loadCatalog; pass typed
	// usage errors through untouched so their authored prose survives.
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return usageErr
	}

	return &UsageError{Msg: "search failed\nwhy: " + err.Error(), Cause: err}
}
