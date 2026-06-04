# AGENTS.md

Agent guidance for the `skillrig` CLI. The **canonical, detailed instructions live
in [`CLAUDE.md`](CLAUDE.md)** (architecture, layer rules, exit-code contract,
conventions, SpecLedger workflow) — read it. This file surfaces the two things an
agent most often gets wrong here.

## 1. Close the loop: `make test-e2e` is required for fetch/auth changes

`make check` (fmt + vet + lint + unit + integration) is the normal pre-merge gate,
but it **deliberately excludes** the true-authentication end-to-end suite. The unit
and integration tiers stub the network (a fake `git`, or a `file://` origin), so
they can pass even if the credential skillrig sends is wrong or absent — a stub
never validates the header.

`make test-e2e` closes that gap: it stands up a **real `git http-backend`** behind a
**real token gate** and drives the **real binary** over the HTTPS `http.extraHeader`
path (no network, no Docker — just `git` on PATH). A missing or wrong token gets a
real `401`.

> **If you change anything in the fetch, token-resolution, or git-transport path
> (`pkg/skillcore/git.go`, `fetch.go`, `catalog.go`, the auth/`ClassifyGitError`
> logic, or `internal/config` origin/CloneURL), the work is NOT done until
> `make test-e2e` is green.** Treat it as the acceptance gate that the stub tiers
> cannot provide.

See [`test/e2e/doc.go`](test/e2e/doc.go) for how the harness works.

## 2. The supported, tested auth path is HTTPS + a token (via `gh auth`)

skillrig fetches every origin over **HTTPS** with a **read-only** token resolved
`GH_TOKEN` > `GITHUB_TOKEN` > `gh auth token`. `CloneURL` is hardcoded to
`https://github.com/OWNER/REPO.git` — there is **no SSH transport**. The path that
is implemented, documented, and exercised by `make test-e2e` assumes a completed
`gh auth login` (or a `GH_TOKEN`/`GITHUB_TOKEN` in CI). Public origins need no
credential; a private origin with no token fails fast as an `AuthError` — never an
interactive prompt or hang, including from a GUI askpass helper like VS Code's
(skillrig forces git non-interactive via `GIT_TERMINAL_PROMPT=0` + an empty
`GIT_ASKPASS` + `GCM_INTERACTIVE=Never`).

**SSH-key origins are a deferred roadmap item** ([`docs/ROADMAP.md`](docs/ROADMAP.md)),
not implemented and not tested. Do not assume `ssh://`/`git@` works; do not steer
users to it as the current path.
