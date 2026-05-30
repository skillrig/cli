# Implementation Plan: Discover & Acquire Skills (`search` + remote `add` + `index`)

**Branch**: `003-search-remote` | **Date**: 2026-05-31 | **Spec**: [spec.md](./spec.md)
**Input**: [spec.md](./spec.md) + [spec-tech.md](./spec-tech.md) + research spikes [S1](./research/2026-05-31-skill-manifest-format.md) · [S2](./research/2026-05-31-catalog-generation-lifecycle.md) · [S3](./research/2026-05-31-auth-token-resolution.md) · [S4](./research/2026-05-31-remote-git-testing.md)

## Summary

Deliver the first end-to-end consumer loop — **`init` → `search` → `add` → `verify`** — plus the origin-side **`index`** generator that keeps discovery honest. A user binds a repo to a remote GitHub origin, **finds** a skill (`search` reads the origin's catalog — query-first over name+description, deterministic `--topic` filter), and **vendors** it directly from the remote (`add` fetches the subtree, no local checkout), recording `commit` + `treeSha` + resolved `version/tag` in the lock so `verify` (002) still passes. The catalog `search` reads is produced by `skillrig index` from each skill's **`SKILL.md` frontmatter** — which this slice migrates to (dropping `skill.toml`, the first build step), aligning with the agentskills.io standard.

All four design uncertainties were resolved in spikes **before** planning (S1 manifest format, S2 catalog lifecycle, S3 auth, S4 testing); this plan consumes their conclusions and does not re-open them.

## Technical Context

**Language/Version**: Go 1.24+ (toolchain 1.24.4) — single static binary (unchanged).
**Primary Dependencies**: `github.com/spf13/cobra` (commands); `github.com/pelletier/go-toml/v2` (config + retained for `.skillrig/config.toml`); **NEW: `gopkg.in/yaml.v3`** (SKILL.md frontmatter — accepted 2026-05-31, the parser `gh` uses; see Complexity Tracking). Lock uses stdlib `encoding/json`. Fetch + tree-SHA via **shelling `git`** (no in-process git/hashing lib). Token via `os.exec` of `git`/`gh` (no `gh`-as-library).
**Storage**: local files only — vendored subtree `.agents/skills/<skill>/`, committed lock `.skillrig/skills-lock.json`; origin-side `index.json` (committed in the origin). No DB. **No tool-managed cache** (catalog fetched per `search`).
**Testing**: `go test`, two tiers — (a) presentation-free **unit** in `internal/...` + `pkg/skillcore` (table-driven + ground-truth: fetched tree-SHA == raw `git`; `index` output == committed `index.json`); (b) **`TestQuickstart_*` integration** in `test/` building/exec'ing the real binary. **New network boundary** tested via S4's substrate: `file://` + local bare repo for happy/integrity; the existing `pkg/skillcore/git.go` `commandContext` exec-stub seam (extended to `Clone`/`FetchSparse`) for auth/unreachable/transient. **No `httptest`/go-vcr** (skillrig shells `git`, never calls the GitHub HTTP API — see Constitution Check).
**Target Platform**: developer/CI machines with `git` (and optionally `gh`) on PATH (macOS/Linux; Windows later).
**Project Type**: single project (existing two-layer Go CLI).
**Performance Goals**: interactive CLI; `search`/`add` dominated by one `git` fetch — no throughput target. Determinism is the hard requirement (SC-002/004/009), not latency.
**Constraints**: offline-deterministic test gate; errors-as-navigation; two-level output; exit codes `search`/`index` 0/1, `add` 0(incl. no-op)/1; exit 2/3 reserved (not emitted). Single origin resolver (`config.ResolveOrigin`); single `skillcore` (one fetch + one `ParseManifest`, shared by `index`/`add`/`verify`/`search` — AP-04/06).
**Scale/Scope**: one origin, tens–hundreds of skills per catalog; vendored subtrees small. 3 new/changed commands (`search`, `index`, remote `add`) + manifest migration + co-evolution docs.

## Constitution Check

*GATE: re-checked after Phase 1 (see end).*

- [x] **I. Specification-First**: spec.md complete, 5 prioritized user stories, clarified + spiked.
- [x] **II. Quickstart-as-Contract**: every US (US1–US5) → a `TestQuickstart_*` scenario with **output-shape** assertions (bounded human lines; parseable+complete `--json`; 3-part errors + exit code). See quickstart.md.
- [x] **III. Ground-Truth Anchoring**: fixtures derived from real output — fetched tree-SHA == raw `git ls-tree`/`rev-parse`; `skillrig index` output == the committed origin `index.json`; manifest fixtures = real `SKILL.md` frontmatter. **Divergence (justified):** §III's "httptest + go-vcr for the GitHub path" does **not** apply — skillrig fetches via shell `git`, so the integrity boundary is the `git` exec, not an HTTP call; S4's exec-stub seam is the faithful mock. Also §III lists `skill.toml` as the index source; S1 changes it to `SKILL.md` frontmatter. Both are **constitution-doc touch-ups** flagged for the FR-024 sweep (amendment needs team approval; not changed unilaterally here).
- [x] **IV. Agent-First CLI Design**: `search` = Query, remote `add` = Vendor Mutation, `index` = origin-side generator; all navigable from `--help` (≥2 examples); errors-as-navigation; two-level output; consume-only (no write credential — token is read-only fetch auth). cli.md updated in-branch (FR-024).
- [x] **V. Code Quality (Go)**: `gofmt`/`go vet`/golangci-lint gate; presentation/execution split preserved (typed errors in `skillcore`, prose in `cli`).
- [x] **VI–VIII. YAGNI / Shortest-path / Simplicity**: catalog is single-tip full-regenerate (no aggregation/GC — S2); `index` is a thin walk+marshal reusing `ParseManifest`; no `httptest`; GHE deferred; `--pin` minimal.
- [x] **IX. Skill–CLI Co-Evolution**: extend the single consolidated `skillrig` skill — `references/search.md` + `references/index.md` (new), update `references/add.md` (remote + `--pin` + auth/unreachable errors); root routing + keywords; sonnet trigger evals.
- [ ] **Issue Tracking**: epic + per-US features to be created (`sl issue create --type epic`) at `/specledger.tasks` time (002 skipped the ledger; 003 restores it).

**Complexity Violations**: one — a new dependency (`gopkg.in/yaml.v3`) against the "no new dependencies" note. Justified in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specledger/003-search-remote/
├── plan.md              # this file
├── spec.md  spec-tech.md
├── research/2026-05-31-*.md   # S1–S4 spikes (done)
├── research.md          # Phase 0 — consolidates the spikes + prior work
├── data-model.md        # Phase 1 — manifest, catalog, lock, typed errors
├── quickstart.md        # Phase 1 — US1–US5 → TestQuickstart_*
├── contracts/           # Phase 1 — search.md, add.md, index.md, schemas
└── tasks.md             # /specledger.tasks (not this command)
```

### Source Code (repository root)

```text
main.go                          # unchanged shim
internal/cli/                    # PRESENTATION + cobra wiring only
├── root.go                      # + register search, index (add already registered)
├── add.go                       # extend: remote path, --pin, map Auth/Unreachable/NotFound
├── search.go        (new)       # Query: render two-level list + --json
├── index.go         (new)       # origin-side generate; render summary
├── output.go  exit.go  repo.go  # reuse/extend renderers + exit mapping
internal/config/                 # resolver (unchanged); origin form classification helper
pkg/skillcore/                   # business logic, presentation-FREE — the single core
├── manifest.go                  # REWRITE: SKILL.md frontmatter (yaml.v3) + metadata.x-skillrig.*
├── fetch.go         (new)       # git clone --sparse over origin; token via os.exec; typed errors
├── catalog.go       (new)       # parse index.json (search) + generate it from frontmatter (index)
├── add.go                       # branch: local-path origin (002) vs remote fetch; lock w/ version/tag
├── errors.go                    # + AuthError, UnreachableError, NotFoundError (from GitError.Stderr)
├── git.go                       # + Clone/FetchSparse on the existing commandContext stub seam
├── lock.go treesha.go verify.go # lock entry gains resolved version/tag; verify unchanged logic
test/                            # TestQuickstart_* (build+exec real binary) — S4 substrate
```

**Structure Decision**: keep the established two-layer split. **All** remote-fetch, token-resolution, catalog parse/generate, and manifest parsing land in `pkg/skillcore` (one implementation, AP-04); `internal/cli` only wires cobra + renders. New commands `search`/`index` mirror `add`/`verify` wiring.

## Build sequence (independently testable slices)

1. **Manifest migration (commit 1)** — rewrite `ParseManifest` to read `SKILL.md` frontmatter via `yaml.v3` (standard fields + `metadata.x-skillrig.*`); drop `skill.toml`; migrate the origin-template skill + fixtures; remove the name/description duplication. Existing `verify`/`add` tests stay green. *(S1)*
2. **`skillrig index` + contract test** — `catalog.go` generate: walk `skills/*/SKILL.md`, `ParseManifest`, marshal `index.json`; `index.go` CLI. Ground-truth: `index` over the origin fixture == committed `index.json`. *(S2)*
3. **Remote fetch layer** — `fetch.go`: `git clone --sparse`/sparse-checkout at ref; `ResolveGitHubToken(hostname)` via `os.exec` (GH_TOKEN→GITHUB_TOKEN→`gh auth token`); classify `GitError.Stderr`→`AuthError`/`NotFoundError`/`UnreachableError`; inject token via `git -c http.extraHeader`. Unit-tested on the exec-stub seam. *(S3, S4)*
4. **Remote `add`** — branch origin classification (local path vs remote `OWNER/REPO`); remote: fetch→byte-identical vendor (reuse 002 copy/treeSha)→lock with `commit`+`treeSha`+resolved `version/tag`; `--pin`; idempotent no-op; force-on-divergence; map new errors in cli. *(S3/S4 + 002 reuse)*
5. **`search`** — `catalog.go` parse; convention-version gate; deterministic AND-tag + substring filter; two-level output. *(S2)*
6. **Co-evolution (FR-023/024)** — origin-template (frontmatter + `index.yml` calls `skillrig index`); `docs/ROADMAP.md`, `docs/ARCHITECTURE-v0.md` (003+004 merge, local-vs-remote reframe, frontmatter+`yaml.v3`, "on merge" not "on release", mise precedence fix), `docs/design/cli.md` (search/index/remote-add surface), constitution §III touch-ups; extend the `skillrig` skill + sonnet evals.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| New dep `gopkg.in/yaml.v3` (vs "no new dependencies") | SKILL.md frontmatter *is* YAML; adopting the agentskills.io standard (S1) requires a YAML parser. It is the same parser `gh` uses, and it replaces `skill.toml`'s bespoke sibling-file format. | Hand-rolling a YAML subset parser is more code + more risk for a worse result; staying on `skill.toml` keeps skillrig diverged from the 26+ agentskills.io-compliant clients (portability loss) and keeps the name/description duplication drift bug. User accepted the dep 2026-05-31. |
| `index` command (origin-side generator) in a "consume-only" CLI | `search` is meaningless against a hand-maintained catalog that drifts (the shipped `build-index.sh` already drops `tags`); skillrig is the single tool for origin maintenance too (spec US5). The generator is thin (walk + shared `ParseManifest` + marshal) and reuses the consumer parser (AP-04). | Deferring to a sibling feature ships a consumer (`search`) against a known-broken producer — the false economy S2 flagged. It is not a write-credential path (no auth, local FS only), so it doesn't breach "consume-only" in the credential sense. |

## Notes for `/specledger.tasks`
- Create the epic + 5 user-story features + the migration/co-evolution features in `sl issue` (restore the ledger 002 skipped).
- Each US → `TestQuickstart_*` with §II output-shape asserts; include the two ground-truth tests.
- Add a cli.md pattern-gate checklist task per new command (Query for `search`; Vendor Mutation for remote `add`; classify `index` as origin-side generator — propose a cli.md note since it's not one of the five consumer patterns).
