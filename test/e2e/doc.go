// Package e2e holds skillrig's build-tagged TRUE-AUTHENTICATION end-to-end suite.
//
// Every other test tier stubs the network: the unit tests swap a fake git via the
// commandContext seam, and the quickstart suite fetches a file:// origin or injects
// a fake `git` on PATH. None of them ever validates the credential skillrig sends —
// a stub can return success no matter what header (or no header) arrives.
//
// This tier closes that loop. It stands up a REAL git server — an in-process
// `git http-backend` (the same CGI binary that ships with git) behind an
// Authorization-header gate — and points the REAL skillrig binary at it over the
// REAL HTTPS token-auth path (https://github.com/OWNER/REPO.git, redirected to the
// local server with git's url.insteadOf, so no skillrig code changes). A request
// with the wrong header (or none) gets a real 401; only the exact
// `Authorization: Basic base64("x-access-token:<token>")` skillrig injects via
// http.extraHeader is accepted. So a valid token genuinely authenticates and a
// missing one genuinely fails fast as an AuthError — proving the auth path the
// stub tiers cannot.
//
// It is tagged `e2e` and therefore EXCLUDED from `make test` / `make check` / CI
// (this doc.go is the only file the default build sees, so `go test ./...` reports
// "no test files" here). Run it explicitly with `make test-e2e`. It is the
// loop-closing check an implementation touching the fetch/auth path MUST pass
// before it is considered done — see AGENTS.md / CLAUDE.md.
//
// Scope today is HTTPS-token only, matching skillrig's only transport (CloneURL is
// always https://github.com/…). SSH-key origins are a deferred roadmap item
// (docs/ROADMAP.md); they are not the tested path.
package e2e
