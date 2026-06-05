package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		// QUERY terms are free-form (a token-AND over name+description+topics), so
		// any number of positional args is valid — declare it explicitly rather
		// than leave the args contract unstated.
		Args: cobra.ArbitraryArgs,
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

	// A git repo is OPTIONAL for search: it only enables the local-checkout
	// fast-path (reading a remote origin's committed OWNER/REPO checkout off
	// disk). Outside a repo — or against a remote/file:// origin with no checkout
	// — repoRoot is left empty and loadCatalog fetches the catalog directly, so
	// `skillrig search` works from any directory (FIX-7).
	repoRoot, err := gitToplevel(cmd.Context(), cwd)
	if err != nil {
		if !errors.Is(err, errNotGitRepo) {
			// An unexpected failure (e.g. context cancellation) is not a "not a
			// repo" precondition — surface it rather than silently proceed.
			return mapSearchError(res.Origin.String(), err)
		}

		repoRoot = ""
	}

	catalog, err := loadCatalog(cmd.Context(), repoRoot, res.Origin)
	if err != nil {
		return mapSearchError(res.Origin.String(), err)
	}

	if err := skillcore.CheckConvention(catalog.SkillrigConvention); err != nil {
		return mapSearchError(res.Origin.String(), err)
	}

	matches := skillcore.Search(catalog, sc.query, sc.topics)

	return renderSearchResult(cmd.OutOrStdout(), res.Origin.String(), matches, sc.opts.json)
}

// loadCatalog acquires and parses the origin's index.json, choosing the
// transport from the origin form exactly as add does (FIX-2, contract search.md
// step 2). The catalog is fetched PER CALL — never cached — so every search sees
// the origin as it is now:
//
//   - bare-path LOCAL origin → read <path>/index.json from disk (no transport);
//   - remote OWNER/REPO with a local checkout at <repo-root>/OWNER/REPO → read
//     that checkout's index.json (the 002 local-copy form, kept green);
//   - otherwise (remote with no checkout, or a file:// origin) → a sparse git
//     fetch of index.json at the resolved @ref via skillcore.FetchCatalog (the
//     ONE catalog acquisition path, AP-04).
//
// Parse stays separate from the convention gate (run by the caller) so a
// malformed catalog and an incompatible convention are distinct failures.
func loadCatalog(ctx context.Context, repoRoot string, origin config.Origin) (skillcore.Catalog, error) {
	if path, ok := localCatalogPath(repoRoot, origin); ok {
		return readCatalogFile(path)
	}

	catalog, err := skillcore.FetchCatalog(ctx, skillcore.CatalogRequest{
		RepoURL: origin.CloneURL(),
		Origin:  origin.String(),
		Ref:     origin.Ref,
		Local:   origin.IsLocal(),
	})
	if err != nil {
		return skillcore.Catalog{}, err
	}

	return catalog, nil
}

// localCatalogPath returns the on-disk index.json path to read when the origin's
// catalog is available locally (no transport), and false when it must be
// fetched. A bare-path LOCAL origin (a filesystem path, not a file:// URL) reads
// <path>/index.json directly (independent of repoRoot); a remote OWNER/REPO reads
// its 002 local checkout at <repo-root>/OWNER/REPO only when there IS a repo root
// and that directory exists. A file:// origin, a checkout-less remote, and the
// no-repo case (empty repoRoot, e.g. search run outside a git repo) all return
// false so the caller fetches (FIX-7).
func localCatalogPath(repoRoot string, origin config.Origin) (string, bool) {
	if origin.IsLocal() {
		if strings.HasPrefix(origin.Path, "file://") {
			return "", false
		}

		return filepath.Join(origin.Path, catalogName), true
	}

	// The remote local-checkout fast-path is anchored at the repo root; with no
	// repo (empty root) there is no checkout to read, so fetch.
	if repoRoot == "" {
		return "", false
	}

	originDir, _ := originDirRef(origin)
	checkout := filepath.Join(repoRoot, originDir)

	if isLocalCheckout(checkout) {
		return filepath.Join(checkout, catalogName), true
	}

	return "", false
}

// readCatalogFile reads and parses a local index.json, tagging the read and parse
// failures distinctly so mapSearchError can author the right what/why/fix.
func readCatalogFile(catalogPath string) (skillcore.Catalog, error) {
	//nolint:gosec // G304: path is the resolved origin path/checkout + a fixed file name, not attacker-controlled.
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

	var refNotFound *skillcore.RefNotFoundError
	if errors.As(err, &refNotFound) {
		return mapRefNotFoundError(refNotFound.Origin, refNotFound.Ref, err)
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
