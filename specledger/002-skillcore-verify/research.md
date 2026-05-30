# Phase 0 Research: `skillcore` + `add` + `verify`

**Feature**: `002-skillcore-verify` | **Date**: 2026-05-30
**Inputs**: [spec.md](./spec.md), [spec-tech-spike.md](./spec-tech-spike.md) (§1–§12), [plan.md](./plan.md), constitution v2.1.0, `docs/ARCHITECTURE-v0.md`, `docs/design/cli.md`. Prior-art studied: `pkg/cli/skills` (skills.sh), `gh-cli/pkg/cmd/skills` (`gh skill`), `gh-cli/git` (gh's git wrapper).

## Prior Work

`sl issue list --all` → only **closed** `001-init-origin-resolution` items (epic SL-227789 + features/tasks). No prior `add`/`verify`/`skillcore` work. This feature reuses 001's `internal/config` resolver and baseline command experience and adds the verification-failure exit class (2).

Three external prior-art implementations were studied and triangulated (spike §11/§12 + the gh-cli `git` study):

| Tool | Acquisition | Integrity | Has `verify`? | Lesson for us |
|---|---|---|---|---|
| **skills.sh** (`npx skills`) | HTTP registry (Vercel) + GitHub Trees/raw API | bespoke **SHA-256** over files, `ref` only (no commit) | no | the "custom hash" §4.2 rejected; network-strict, no offline mode |
| **`gh skill`** (GitHub first-party) | GitHub REST API (Trees/Blobs) | git tree-SHA **from the API**, used for *online update-detection*; injects provenance into frontmatter | **no** | confirms our `verify` fills a real gap; frontmatter injection is *incompatible* with tree-SHA label-honesty (so we keep provenance lockfile-only) |
| **`gh-cli/git`** (gh's git wrapper) | n/a (general git ops) | **shells `git` for everything; zero in-process object hashing** | n/a | the decisive input for D1 below — don't reimplement git |

## Decisions

### D1 — Tree-SHA is git's own output (shell `git`), not in-process hashing

**Decision**: `skillcore.TreeSHA(gitDir, ref, relPath)` shells **`git -C gitDir rev-parse <ref>:<relPath>`**, returning git's canonical tree-object SHA. `add` calls it on the **origin** (`ref` = the resolved ref); `verify` calls it on the **consumer** repo (`ref` = `HEAD`). One primitive, both sides git-canonical.

**Rationale**:
- **gh-cli grounds it.** `gh/git` (`client.go:52–99`) wraps the `git` binary via a pluggable `commandContext` and shells out for *every* git operation; it contains **no** in-process git object hashing anywhere in the codebase. A mature, widely-used reference deliberately does not reimplement git internals.
- **Canonical by construction.** Because both the recorded value (`add`, on the origin) and the recomputed value (`verify`, on the consumer's committed tree) are *git's own* `rev-parse` output, they match by construction when content is identical — there is literally no second implementation to drift (AP-04 hardened). An in-process re-hash would have to exactly reproduce git's blob/tree object format, executable-bit/symlink mode mapping, the tree-entry sort quirk (subtrees sorted as if names end in `/`), and any clean filters — a real, ongoing correctness risk for marginal benefit.
- **Relocation-invariant** (spike §3): a git tree object hashes only immediate `{mode, name, childSHA}` entries, so the origin's `skills/foo` tree SHA equals the consumer's `.agents/skills/foo` tree SHA iff contents match — exactly what makes offline label-honesty survive the origin→consumer relocation.
- `git` is already a required dependency (001's `init` uses `git rev-parse --show-toplevel`); adds nothing new.

**Alternatives rejected**:
- *In-process SHA-1 tree hashing* (the spike's earlier lean): pure-Go, no subprocess, but must reproduce git's object model exactly — gh-cli's "never reimplement" signal + the correctness surface (autocrlf, mode bits, sort) outweigh the "no subprocess" benefit. Rejected.
- *Bespoke SHA-256 canonicalization* (skills.sh's approach): re-derives a guarantee git already gives, and would *not* equal the origin's git tree SHA (breaks future `bump`/origin comparison). Rejected (architecture §4.2 already rejected it).

### D2 — `verify` hashes the **committed** tree + flags dirty separately

**Decision**: `verify` recomputes each locked skill's tree-SHA from the **committed** vendored tree (`git rev-parse HEAD:<path>`) and compares to the lock. An **uncommitted / dirty** vendored path (detected via `git status --porcelain -- <path>`, or a path absent from `HEAD`) is reported as a **distinct** finding ("vendored but not committed / locally modified — commit before verifying"), not folded into the label-honesty fingerprint.

**Rationale**: the load-bearing caller is the **CI gate**, which runs on committed content (working tree == `HEAD`), so committed-tree hashing is exactly right there. It keeps `verify` truly **read-only** — `rev-parse`/`status` write no git objects (a temp-index `write-tree` over the working tree *would* write loose objects, violating the read-only spirit of FR-015). Separating "uncommitted local edits" (a working-state warning) from "committed content mismatches its recorded version" (a label-honesty failure, exit 2) is a *better* taxonomy than collapsing both into one fingerprint. This **refines** spike §3's "hash the working tree" intent.

**Alternatives rejected**:
- *Hash the working tree via temp-index `git add` + `write-tree --prefix`*: catches uncommitted edits in the fingerprint, but writes loose objects into `.git` (not read-only) and adds `.gitignore`/filter subtleties + temp-index/object-dir plumbing — too clever for the MVP (Constitution VIII). The dirty-flag covers the same ground more clearly.

**Consequence**: the `add → verify` round-trip commits the vendored skill before `verify` (realistic — vendored-in-git means you commit what you vendored). Quickstart scenarios include the `git add && git commit` step.

### D3 — `skillcore` is a public package at `pkg/skillcore` (SDK-1)

**Decision**: import path `github.com/skillrig/cli/pkg/skillcore`. **Rationale**: SDK-1 requires third-party Go tools import it (so not `internal/`); the `pkg/` convention explicitly signals "public, importable" and segregates it from `internal/`; the CLI imports the *same* package (AP-04). **Alternatives rejected**: `internal/skillcore` (un-importable — violates SDK-1); module-root `skillcore/` (also fine, but `pkg/` was chosen for the explicit public signal); a *separate module* (`github.com/skillrig/skillcore`) — independent SemVer, but multi-module overhead is YAGNI pre-release. (No constitutional rule mandated `internal/`; see spike §10.)

### D4 — Lock schema omits `[[requires]]`

**Decision**: lock entry = `{ version, commit, treeSha, path }`; top-level `{ lockfileVersion, origin, skills{} }`. **No** `requires` array. **Rationale**: the full skill subtree — including `skill.toml` — is vendored on disk and fingerprint-attested, so the vendored manifest is the single source of truth for prerequisites; a future `doctor` walks it directly. Mirroring into the lock duplicates data that can drift (YAGNI). **Diverges from architecture §4.2** (whose "mirror requires for offline prereq check" assumed the manifest might not be on disk) — flagged for architecture reconciliation. **Alternative rejected**: mirror `requires` now for forward-compat — rejected because nothing reads it this slice and the manifest is always present.

### D5 — Origin resolution stays in the CLI layer (SDK boundary)

**Decision**: `internal/cli/add.go` resolves the active origin via the existing `config.ResolveOrigin` and passes the **resolved local path** down to `skillcore.Add`. `skillcore` never resolves origins, reads config, or fetches. **Rationale**: keeps `skillcore` a pure filesystem/git core (the SDK boundary, spike §10) — an SDK consumer supplies the source path themselves; acquisition + origin policy are CLI concerns. `verify` needs no origin at all. **Alternative rejected**: a `--from`/path argument on `add` that bypasses the configured origin — rejected as a single-origin-contract violation (clarified 2026-05-30); tests do `init --origin <local>` then `add`.

### D6 — Vendor copy preserves file modes; injects nothing

**Decision**: `add` copies the skill subtree byte-for-byte **preserving file modes** (the executable bit is part of the git tree SHA) and adds/modifies nothing (no frontmatter injection). **Rationale**: any mutation or mode change alters the tree SHA and breaks label-honesty (the `gh skill` frontmatter-injection incompatibility, spike §12). A mode-preserving recursive copy is the boring, obvious implementation (Constitution VIII); `git archive`-based extraction is an equivalent alternative if mode handling proves fiddly. **Alternative rejected**: injecting provenance into `SKILL.md` (gh skill's model) — fundamentally incompatible with recompute-the-tree-SHA verification.

### D7 — git interaction via a small testable client (gh pattern)

**Decision**: a `git.go` inside `pkg/skillcore` with a `Client`-like struct carrying a pluggable `commandContext` (function field) and a `GitError{ExitCode, Stderr}` type, mirroring `gh/git` (`client.go`, `errors.go`). Exposes `revParse`, `status`, etc. used by `TreeSHA`/`Add`/`Verify`. **Rationale**: directly testable (swap `commandContext` for a stub in unit tests; run real `git` in a tmpdir for integration), errors classified (exit code + stderr) so the CLI can render errors-as-navigation. **Testing** (D8) uses gh's dual strategy. **Note**: `internal/config` already shells `git` for `rev-parse --show-toplevel`; consolidating both onto one git client is a future cleanup (out of scope — YAGNI), noted so it isn't lost.

### D8 — Fixtures: bootstrap real git in a tmpDir (gh `initRepo` pattern)

**Decision**: a test helper does `git init` + `git add` + `git commit` in a `t.TempDir()` from files committed under `test/testdata/sample-origin/`, producing a real origin to `add` from; the consumer repo is likewise a tmpDir git repo. **Rationale**: avoids committing a nested/bare git repo inside skillrig-cli (the rejected alternative); mirrors gh's `initRepo` helper (`git/client_test.go:1948`). **Determinism**: the **tree-SHA is content-only → deterministic**, so tests assert the exact tree-SHA / fingerprint; the **commit SHA varies** with author/date, so tests assert it is present + well-formed (40-hex) — *or* pin `GIT_AUTHOR_*`/`GIT_COMMITTER_*` env for a fully reproducible commit when an exact assertion is wanted. **Alternative rejected**: commit a bare fixture repo (gh also does this via `fixtures/simple.git`) — rejected to avoid nested-repo maintenance; bootstrap is cleaner here.

### D9 — Exit-code mapping extended for verification failure

**Decision**: `internal/cli/exit.go`'s `exitCodeFor` is extended from "any error → `ExitUsage(1)`" to a typed switch: a `skillcore` verification failure (a typed error, e.g. `*skillcore.VerifyFailure` surfaced through the CLI) → `ExitVerification(2)`; `*UsageError` and everything else → `ExitUsage(1)`; `nil` → `ExitOK(0)`. **Rationale**: load-bearing exit codes (Constitution IV; cli.md) — CI/agents branch on *why* `verify` failed. `ExitVerification=2` and `ExitPrereq=3` constants already exist (reserved); this activates 2. **Alternative rejected**: a sentinel error value — a typed error carrying the per-skill report is richer for rendering.

### D10 — No `go-getter`, no network, no new deps (this slice)

**Decision**: acquisition is a local origin (a filesystem path that is a git checkout); no `go-getter`, no HTTP, no auth. **Rationale**: OQ-3 says go-getter's value scales with multi-origin support, which is deferred; a thin git interaction suffices for a local origin and honors "minimal deps" (architecture §1). go-getter / `gh`-auth-as-library / GitHub-only remote fetch are recorded for the *remote-`add`* follow-up (spike §11 OQ-2/OQ-3). **Alternative rejected**: adopt go-getter now — premature (YAGNI); it also may not surface the commit SHA we need for provenance (spike OQ-3 caveat).

### D11 — Test-oracle independence: integration tests use raw `git`, not `skillcore`

**Decision**: the `TestQuickstart_*` integration tests (build + exec the real binary) use **raw `git`** (via `os/exec` helpers) for both fixture bootstrap *and* computing the **expected** tree-SHA they assert against — they do **not** route the expected value through `skillcore`. The small testable git client (pluggable `commandContext` stub) is for `skillcore`'s **own unit tests** (error paths — simulated `git` failures), and one `skillcore` unit test pins `skillcore.TreeSHA(...) == ` raw `git rev-parse <ref>:<path>` against the fixture (the SDK invariant vs ground truth).

**Rationale**: integration tests are **black-box** — the binary under test uses `skillcore` internally, so `skillcore` is part of the system under test. Using it to also produce the *expected* value is **circular validation**, which Constitution III explicitly forbids ("a wrong spec yields matching-but-wrong types, fixtures, AND tests that all agree with each other and with nothing real"). Raw `git` is the independent oracle. Mechanical setup (`git init`/`commit`) *could* reuse the client, but raw `git` is simplest and keeps the oracle boundary crisp.

**Alternative rejected**: route setup + expected values through `skillcore`'s git client for DRY — rejected: it couples the oracle to the implementation and cannot catch a `TreeSHA` bug.

### D12 — Fixture mirrors a *canonical* (design-aligned) origin layout; the existing template is a pre-design sample to reconcile

**Context** (clarified by the user, 2026-05-30): the existing `skillrig-origin` repo (`/Users/vincentdesmet/specledger/skillrig/skillrig-origin`) is a **pre-design sample** — *not* canonical to copy verbatim. The fixture and that template should both conform to the canonical origin structure the locked design implies; the specific sample content (and thus its tree-SHA) is illustrative, and tests compute the SHA independently (D11) so content can change freely.

**Decision — desired fixture** (`test/testdata/sample-origin/`), minimal + design-aligned:
```
test/testdata/sample-origin/
├── .skillrig-origin.toml          # convention_version, origin, skills_dir = "skills"
└── skills/
    └── <sample-skill>/
        ├── SKILL.md
        └── skill.toml             # manifest carries [[requires]] (manifest = single source of truth; the lock OMITS it, D4)
```
`add`/`verify` read **only** `skills/<name>/{skill.toml,SKILL.md}` this slice; `.skillrig-origin.toml` is carried for fidelity (and lets a test assert `add` ignores non-skill origin files). `index.json`, `cmd/`, `policy.toml`, workflows are **not** needed for `add`/`verify` and are the origin-template's concern, not the fixture's (YAGNI).

**Recommended changes to the `skillrig-origin` template** (cross-repo — tracked here as recommendations; the template is a separate repo, not edited on this branch):
- `docs/CONVENTION.md` MUST pin the **fingerprint boundary** precisely: `treeSha = git tree-object SHA of skills/<name>`, i.e. `git rev-parse <ref>:skills/<name>` — so origin and consumer compute identically (the locked shell-`git` decision, D1). Today it references the boundary loosely.
- Note (manifest vs lock): the skill.toml `[[requires]]` is the **single source of truth** and is **not** mirrored into the consumer lock (D4) — state this in `CONVENTION.md`/`AGENTS.md` so lock-mirroring isn't re-introduced (it contradicts architecture §4.2's old wording).
- **Layout constraint (perf, D13)**: each skill dir MUST be **self-contained** under `skills/<name>/` (no cross-skill shared files), so one skill can be fetched via partial-clone + sparse-checkout without the rest of the monorepo. The template already satisfies this — document it as a convention constraint.
- `convention_version` stays `1` (this slice introduces no structural change to the origin).

### D13 — Large-monorepo performance: no full clone needed; the tree-SHA primitive is acquisition-agnostic

**Decision (this slice)**: the origin is a **local checkout already on disk**, so `add` = read one subtree + 2 `git rev-parse` calls — **trivial** cost even for a huge monorepo (no clone). The git-wrapper (shell `git rev-parse <ref>:<path>` against a local dir) is the whole cost.

**Decision (remote-`add` follow-up — recorded for OQ-1/OQ-3)**: fetch one skill via `git clone --filter=blob:none --sparse <origin>` + `git sparse-checkout set skills/<name>` into a temp dir, then the **same** `TreeSHA(tempDir, ref, path)` primitive. **Key finding** (Explore agent, 2026-05-30): the git tree-SHA is **identical** whether the objects arrived by full clone, partial clone, or are read from GitHub's Trees API (`entry.SHA`) — it *is* the canonical git tree object SHA — so the remote case needs **zero change** to the integrity primitive. **The local-origin slice does not paint us into a corner.**

**Prior-art locking contrast** (the TreeSha difference):
- **`gh skill`** reads the tree SHA from GitHub's **Trees API** (`discovery.go:544-592`) — no clone, but GitHub-coupled, and used for *online update-detection*, not offline verify; injects it into frontmatter (`frontmatter.go:70-98`).
- **skills.sh** computes a **bespoke SHA-256** over fetched files (`hash.go:18-68`), records `ref` only (no commit), and falls back to a `--depth 1` **shallow clone** (whole tree) on API failure (`discover.go:102-182`).
- **skillrig** records the git **tree-SHA + commit**, computed by `git`, **offline-verifiable** — the only one of the three with an offline integrity gate.

**Recommendation for remote-`add` MVP**: **partial-clone + sparse-checkout** (pure `git`, any host, downloads only the skill's blobs, preserves the tree-SHA primitive) over the GitHub Trees+Blobs API (faster + no `git` binary, but GitHub-API-coupled — conflicts with the generic-binary stance). Shallow clone (`--depth 1`) is the simple fallback when partial clone is unavailable. The choice is deferred to the remote-`add` spec; **this slice needs none of it**.

## Open items carried to `/specledger.tasks` / implement

- **Design-doc sync (this branch):** update `docs/design/cli.md` — `verify` is integrity-only (prereq → `doctor`); reverse the "not separate packages" line for `skillcore` (SDK-1). (Architecture already updated, spike §8.)
- **Skill co-evolution (Constitution IX):** ship/extend an agent skill for `add` + `verify` (exit `0`/`2` meaning; missing backing tool is *not* a verify failure) with a trigger-accuracy eval.
- **Architecture reconciliation:** note D4 (lock omits `requires`) against architecture §4.2; rename `internal/skillcore` references → `pkg/skillcore` (spike §10 reconciliation list).
