# Phase 0 — Research: `003-search-remote`

All design uncertainties were resolved by four time-boxed spikes during `/specledger.clarify` (2026-05-31). This file consolidates their conclusions in Decision/Rationale/Alternatives form. **No NEEDS CLARIFICATION remain.** Full writeups: `research/2026-05-31-{skill-manifest-format,catalog-generation-lifecycle,auth-token-resolution,remote-git-testing}.md`.

## Prior work

- **001 (epic SL-227789, closed):** `config.ResolveOrigin` (env > project > global), `OWNER/REPO[@REF]` grammar (`config.Origin{Owner,Repo,Ref}`), baseline CLI (help, errors-as-navigation, two-level output, exit codes). This slice reuses the resolver verbatim and the `@REF` branch pointer.
- **002 (merged):** `pkg/skillcore` (`ParseManifest`, `TreeSHA` via shell `git`, `Add`, `Verify`, lock, typed errors, the `git.go` `commandContext` exec-stub seam), local-copy `add` (byte-identical vendor, idempotent no-op, force-on-divergence, path-traversal + symlink guards), offline `verify`. 002 tested against a **local git working tree** (`git init`+fixtures+commit in tmpDir; no `file://`, no remote). This slice **extends** `add` with a remote fetch path and **reuses** the vendor/treeSha/lock machinery unchanged.
- 002's `add` overloaded `OWNER/REPO` as a directory `<consumerRepoRoot>/OWNER/REPO` (the seam this slice splits into explicit-local-path vs remote-fetch).

## D1 — Skill manifest format (Spike S1)

**Decision:** Migrate each skill's machine metadata into **`SKILL.md` agentskills.io frontmatter**; drop the `skill.toml` sibling file. Standard fields (`name`, `description`) used verbatim; skillrig-specific data (`version`, `namespace`, `tags`, `convention-version`, `requires`) under the standard's free-form `metadata` map as **`metadata.x-skillrig.*`**. Parser: `gopkg.in/yaml.v3`.

**Rationale:** The agentskills.io `metadata` map is the spec-sanctioned extension point (its own example puts `version` there). The Go `gh` CLI does exactly this in production (`internal/skills/frontmatter/frontmatter.go`, `yaml.v3`, flat dotted keys like `metadata.github-tree-sha`, prefixed to avoid collisions). One atomic file per skill, portability across 26+ compliant clients, no parallel format to lint, and it removes 002's latent `name`/`description` duplication-drift bug. Migration is small/in-slice (commit 1): only `pkg/skillcore/manifest.go` (~47 lines, single caller `add.go:91`); `verify.go`'s `isSkillDir` already accepts `SKILL.md`.

**Correction to the original hypothesis:** `requires` does **NOT** go in `allowed-tools` — the standard defines `allowed-tools` as a space-separated string of agent-permission invocations (`Bash(git:*) Read`) and `gh` actively rejects an array form; `compatibility` is free-text prose. So `requires` (tool + version constraint + private `source`) lives under `metadata.x-skillrig.requires`.

**Alternatives considered:** (a) keep `skill.toml` — rejected: diverges from the ecosystem standard, keeps the duplication bug, two formats to maintain. (b) put `requires` in `allowed-tools` — rejected: wrong semantics, `gh` rejects arrays. (c) a separate "manifest reframe" feature first — rejected: the parser is the only real change and it's ~47 lines; landing it as commit 1 of 003 means the fetch/catalog/verify code is written against the new format once.

**Risk carried to implementation:** `metadata.x-skillrig.requires` is a nested list, bending the spec's string→string `metadata` letter. `gh`'s `map[string]interface{}` parses it fine and it's namespaced; **validate against `skills-ref validate` during build**, fall back to a JSON-encoded string only if a strict validator rejects it.

## D2 — Catalog generation & lifecycle (Spike S2)

**Decision:** Ship **`skillrig index`** (origin-side generator) **in 003**. The catalog is **single-tip** (reflects only the skills at the origin's selected branch/ref — one entry per skill = the HEAD version), **full-regenerated** from HEAD frontmatter on each run; **no cross-ref/version-history aggregation, no GC** (YAGNI). Version history lives in git tags, reached by `add --pin <tag>` (D5), never via the catalog.

**Rationale:** `search` is only as honest as the catalog; the shipped `build-index.sh` provably drifts (emits `name/version/description/path`, drops `tags`/`requires` — that is FR-023). skillrig is the single tool for origin maintenance, and the generator is thin: walk `skills/*/SKILL.md` + the **same** `ParseManifest` consumers use + marshal (AP-04 by construction). The origin's `index.yml` workflow is already authored to call it (`command -v skillrig … skillrig index --out`) and is **`push: main` (paths `skills/**`) triggered**, full-regenerating and committing if changed. Single-tip keeps the root `skillrigConvention` coherent and bounds catalog size; removed-at-HEAD skills correctly disappear from `search` while already-vendored consumers stay fine (their lock is offline-verifiable).

**Alternatives considered:** (a) consume-only + roadmap a generator — rejected: ships `search` against a known-broken producer. (b) cross-ref aggregated catalog (all versions across tag history) — rejected: needs tag-walking, grows unbounded, breaks the single convention root; pins already cover history. (c) append-only + GC — rejected: nothing accumulates under full-regenerate, so GC is moot.

**Contract test:** `skillrig index` over the origin fixture MUST equal the committed `index.json` (producer == artifact), mirroring the tree-SHA oracle.

## D3 — Origin classification: local vs remote (firm decision, §8a)

**Decision:** The origin is **either** a remote `OWNER/REPO` (fetched over the network) **or** an explicitly-configured **local filesystem path**. The tool **never** creates or caches a local copy of a remote — there is no "both present" precedence. It reports which form it used.

**Rationale:** Confirmed against 002's code (it conflated the two by treating `OWNER/REPO` as a path). Matches the user's intent and keeps `search` correct under fetch-per-call (no stale cache to reconcile). FR-011 (local add) is preserved as the explicit-path form.

**Alternatives considered:** tool-managed local cache (original assumption A1) — rejected by review: introduces staleness/precedence the tool can't honestly resolve.

## D4 — Authentication / token resolution (Spike S3)

**Decision:** Resolve a GitHub token via `os.exec`, order: **`GH_TOKEN` env → `GITHUB_TOKEN` env → `gh auth token --hostname github.com`** (exit 0 + non-empty stdout = token; non-zero = no session → skip, not fatal; `gh` absent → skip silently). Inject via **`git -c http.extraHeader="Authorization: Basic <base64(x-access-token:TOKEN)>"`** — never embedded in the clone URL. Seam signature `ResolveGitHubToken(hostname string)`; **GitHub Enterprise deferred** (one-line extension later). `git credential fill` deferred.

**Rationale:** Mirrors `gh`'s own precedence; `gh auth token` cleanly surfaces keyring-stored tokens that reading `hosts.yml` directly would miss. No `gh`-as-a-library (heavy); no bespoke credential store. `http.extraHeader` avoids token leakage via process listing / shell history. (S3 also **corrected** architecture §8b.2: mise's real precedence puts env vars before `credential_command` — doesn't affect skillrig, but the doc claim is wrong → FR-024.)

**Failure classification** (all three exit `128`; split by `git`/`gh` **stderr**): `Authentication failed`/`Invalid username or token` → **AuthError** (FR-017); `repository '…' not found` → **NotFoundError** (FR-012); `Could not resolve host`/`Failed to connect` → **UnreachableError** (FR-018). **Private-repo subtlety:** GitHub returns *not found* (not 403) for a private repo with no/bad token, so NotFound + no resolved token MUST add the hint *"if this is a private origin, authenticate via `gh auth login` or set GITHUB_TOKEN."*

**Alternatives considered:** vendor `gh`'s auth packages (too heavy); plain `GITHUB_TOKEN`-only (misses `gh`/keyring users).

## D5 — Identity, fingerprint, pins (firm decisions, §8a)

**Decision:** At add time, record in the lock entry: **`commit`** (provenance, exact upstream commit), **`treeSha`** (label-honesty, git tree-SHA computed from the fetched subtree by the *same* `skillcore` code `verify` recomputes), **and the resolved human-readable `version`/`tag`**. `--pin <ref>` is a per-skill immutable tag/SHA (distinct from the origin-level `@ref` branch); `tag_scheme = "name-vSEMVER"` ⇒ `--pin v1.4.0` resolves to tag `terraform-plan-review-v1.4.0`. Non-existent pin → distinct "no such version" (NotFoundError variant). The origin publishes no per-skill tree-SHA, so label-honesty = "matches what was vendored," anchored by provenance — not an origin-attested hash (per-version tree-SHA publishing deferred).

**Rationale:** A commit is opaque to humans; the tag conveys the version ordering they reason about (kept even if upstream rewrites it). `verify` then checks on-disk content against `treeSha` offline, unchanged from 002.

## D6 — Remote-git test substrate (Spike S4)

**Decision:** Three tiers — (1) **happy/integrity**: `file://` + a local **bare** repo in `t.TempDir()` (push the fixture working tree to a bare, point the CLI at `file://<bare>`), running the real `git clone --sparse` offline; ground-truth assertion `fetched treeSha == rawTreeSHA(fixture,"HEAD","skills/<name>")`. (2) **FR-017/018 + transient**: extend the **existing** `pkg/skillcore/git.go` `commandContext` exec-stub seam to the new `Clone`/`FetchSparse` and inject `(exit=128, stderr=…)` — `pkg/skillcore` unit tests; classification lives in `skillcore`, not `cli`. (3) **Reject** real git-over-HTTP `httptest` (smart-HTTP/CGI handshake is fragile, OS-specific, unnecessary).

**Rationale:** skillrig owns error-classification + rendering + exit codes; the exec-boundary seam exercises all of it deterministically and offline. (This is the justified divergence from Constitution §III's "httptest + go-vcr for the GitHub path" — skillrig shells `git`, it never calls the GitHub HTTP API, so there is no HTTP boundary to record.)

**Not coverable offline (future E2E/manual, not gate-blockers):** GitHub's real auth handshake, mid-stream TCP abort, HTTP 429.

**Alternatives considered:** `httptest`/go-vcr (no HTTP boundary exists to mock); `file://`-only (can't simulate auth/unreachable/transient).

## D7 — Fetch transport (leaning → confirmed)

**Decision:** Shell **`git` partial-clone + sparse-checkout** for both the skill subtree and the catalog file (a sparse single-file checkout of `index.json`), one transport. **Rationale:** keeps the "shell `git`, no in-process hashing dep" stance, makes the tree-SHA ground-truth trivial (git computes it), and keeps auth uniform (one `http.extraHeader` path). **Alternative:** raw HTTPS GET (`raw.githubusercontent`/contents API) for the catalog — rejected for this slice (a second transport + a second auth path for marginal latency benefit; revisit only if `git`-fetching a single file proves too slow).

## Open items intentionally deferred (not blockers)
- GitHub Enterprise host auth (S3) · per-version tree-SHA publishing (S5/§8a) · catalog caching (D-catalog-fetch) · cross-ref aggregation (D2) · `httptest` real-HTTP coverage (D6). All recorded; none block this slice.
