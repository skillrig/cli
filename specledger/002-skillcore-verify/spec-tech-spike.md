# Tech Spike: `skillcore` + `add` (local) + `verify`

**Feature**: `002-skillcore-verify`
**Created**: 2026-05-29
**Status**: Draft — input to `/specledger.plan`
**Purpose**: Capture the technical decisions and their rationale *before* writing user stories, so `spec.md` can stay user-facing (WHAT/WHY) while the HOW lives here and feeds planning. Decisions here are anchored to `docs/ARCHITECTURE-v0.md` (§2, §4, §8, §9b) and `docs/design/cli.md` (Exit Codes, Verification Gate, AP-02/AP-04).

> This document is a **spike**, not a contract. Where it commits to a behavior the user feels, that behavior is restated as a user story / FR in `spec.md`. Where it commits to internals (package boundaries, hashing mechanics), that stays here and is finalized in `plan.md`.

---

## 1. Scope of this increment

Three deliverables, smallest coherent slice that makes the core promise *demonstrable end-to-end and offline*:

1. **`skillcore`** — the single shared primitive package. One implementation of: git **tree-SHA** computation, **`skill.toml`** manifest parse, and **`skills-lock.json`** read/write. Presentation-free (same layering discipline as `internal/config`). **Package path is an open question — likely public (`skillcore/` or `pkg/skillcore`), not `internal/`** — driven by the SDK requirement; see §10. Consumed by `add` and `verify` now; reusable by `bump`/`doctor`/`index` later **without a parallel copy** (AP-04).
2. **`skillrig add <skill>`** — vendor a named skill from the repo's **configured origin** (resolved via the shared resolver; the origin may be a **local checkout** for this increment — offline, no network/git fetch yet) into the consumer repo's canonical skill location, and write/update the lock entry. There is **no** `--from`/path argument that bypasses the origin (clarified 2026-05-30 — that would break the single-origin contract from `init`); tests run `skillrig init --origin <local-origin>` then `skillrig add <skill>`. This is the **producer** that gives `verify` something real to check; it is also the first real cut of the `add` verb (architecture §2, Vendor Mutation pattern).
3. **`skillrig verify`** — the offline, deterministic, read-only **integrity gate**: label-honesty (tree-SHA) + orphan (on-disk = locked). Exit `0/1/2`.

**Why `add` is in scope** (course-correction 2026-05-29): without a producer, `verify` could only be tested against hand-authored locks, which can't anchor a *real* git tree-SHA (constitution III, ground-truth). `add` from a local fixture origin produces a genuine `commit`+`treeSha`, so the `add → verify` round-trip is the acceptance contract and the tree-SHA is never invented.

---

## 2. Clarification decisions (2026-05-29 session)

| # | Question | Decision | Consequence |
|---|---|---|---|
| C1 | Does `verify` check `[[requires]]` prerequisites (emitting exit 3)? | **No.** Prerequisite / eligibility checking belongs to **`doctor`** (and `bump` when re-vendoring), never `verify`. | `verify` is integrity-only → exit `0/1/2`. Exit `3` (prerequisite) is **deferred to a future `doctor` spec**. `docs/ARCHITECTURE-v0.md` updated (§2, §8, open-Q5 resolved). |
| C2 | Why not "fail in CI / warn for humans" on prereqs? | **Rejected framing.** It conflated "non-interactive" with "CI". CI validates *content* and needs **no backing binaries**; the runtime **agent** is the caller that needs prerequisites present. So eligibility belongs where the agent asks for it (`doctor`), and the content gate (`verify`) stays binary-free. | CI `verify` **never** fails on a missing/`mise`-absent binary — that worry is entirely a `doctor` concern now. |
| C3 | What counts as a prerequisite "satisfied" (for the future `doctor`)? | On **PATH** (`--version` parsed, constraint checked) **OR** mise-resolvable. A `mise.toml` is a *suggestion*, not required — without it, skillrig just reports "missing from PATH". | Recorded as **future `doctor` design intent**; not implemented in this slice. Open question carried: should `doctor` flag "tool declared in `mise.toml` but not installed" distinctly from "absent everywhere"? |
| C4 | Where does the lock under test come from, given `add`/`bump` are unimplemented? | **`add` is in scope** (local-path mode). `verify` itself stays **read-only**. | The `add → verify` round-trip is the test vehicle; no hand-authored locks needed for the happy path. |

---

## 3. `skillcore` primitives

Single implementation, presentation-free, the AP-04 hard boundary.

- **`TreeSHA(skillDir) -> sha`** — the **git tree SHA** of the skill subtree (architecture §4.2). Computed from the on-disk content using git's own object model (no bespoke canonicalization — line endings / mode bits / symlinks handled by git). The value `add` records and the value `verify` recomputes come from **this one function**, so the gate can never diverge from what was written (R9, R14, N2).
  - **Resolved (plan.md/research, 2026-05-30):** **shell `git`** — `TreeSHA = git rev-parse <ref>:<path>` (git's canonical tree SHA), *not* in-process hashing. Rationale: gh-cli (mature reference) shells git for everything and reimplements nothing; both `add` (on the origin) and `verify` (on the consumer's `HEAD`) use git's own output, so they match by construction with zero autocrlf/mode-bit/tree-sort reimplementation risk. The git client follows gh's pattern (pluggable `commandContext` + `GitError`). See research.md.
  - **Resolved:** `verify` hashes the **committed** vendored tree (`HEAD:<path>`) and flags an uncommitted/dirty vendored tree as a *distinct* finding via `git status --porcelain` — a cleaner taxonomy than folding uncommitted edits into the fingerprint, and it keeps `verify` truly read-only (`rev-parse`/`status` write no objects, unlike `write-tree`). This **refines** the earlier "hash the working tree on disk" intent.
  - **Must equal git's canonical tree-object SHA** — the same value a git origin (and GitHub's Trees API, per §12 `gh skill`) publishes for that subtree. A git tree object hashes only *immediate entry names + modes + child SHAs*, so it is **relocation-invariant**: the subtree's SHA at the origin's `skills/<n>/` equals the vendored copy's at `.agents/skills/<n>/` **iff their contents match**. That invariance is precisely what makes offline label-honesty survive the origin→consumer relocation.
- **`ParseManifest(skill.toml) -> Manifest`** — parse `name, version, namespace, description, tags, [[requires]]` (architecture §4.1). In this slice, `verify` uses it to *recognize* a directory as a skill and `add` uses it to read `name`/`version`. The `[[requires]]` data is **NOT mirrored into the lock** (clarified 2026-05-30): the full subtree — including `skill.toml` — is vendored on disk and fingerprint-attested, so the **vendored manifest is the single source of truth** for prerequisites; a later `doctor` walks it directly. Mirroring would only duplicate data that can drift. → **diverges from architecture §4.2**, whose "mirror requires for offline prereq check (R16)" rationale assumed the manifest might *not* be on disk; in our vendored-in-git model it always is. (Flag for architecture reconciliation.)
- **`ReadLock` / `WriteLock` (`skills-lock.json`)** — typed lock I/O (architecture §4.2, **minus `requires`** per above): `lockfileVersion, origin, skills{ name -> { version, commit, treeSha, path } }`. Atomic write (temp + rename — open Q10). `WriteLock` used by `add`; `ReadLock` by `verify`.

> All primitives are **path-in / data-out and never fetch** — they operate on a local filesystem working tree only. This is what makes the local-vs-network choice irrelevant to the core, and it is the SDK boundary (see §10).

---

## 4. `add` (local-path mode) — the producer

- **Input**: a local path to a skill subtree inside a sample-origin fixture (the fixture is a real git repo, so a `commit` SHA is readable offline).
- **Behavior**: copy the skill subtree into the consumer repo's canonical skill location; compute `treeSha = skillcore.TreeSHA(...)`, read `commit` from the fixture's git, `version`/`requires` from `skill.toml`; write/update the lock entry. Idempotent on re-add of identical content.
- **Pattern** (cli.md): **Vendor Mutation** — supports `--dry-run`; refuses to clobber content diverging from a locked `treeSha` without `--force` (content-comparison-on-write, architecture §9b). Writes the lock via `skillcore` only.
- **No content mutation (critical, contrast with `gh skill` §12):** `add` MUST vendor the skill subtree **byte-identical** to the source — it MUST NOT inject provenance into `SKILL.md` frontmatter the way `gh skill` does. Any injection would change the subtree's tree SHA and **immediately break label-honesty** at `verify` time (`verify` recomputes the *whole* subtree, so an added/modified file is a mismatch). All provenance lives **only** in the sidecar lockfile. This is *required* by the tree-SHA model, not a style choice.
- **Acquisition layer (builds toward full `add`)**: the local source is a **git checkout** (it must be — see §7), i.e. the *offline analog of a git origin cloned locally*. Local-path `add` against it exercises the **exact same** `skillcore` + lock path the future remote git-origin `add` will, with only the *acquisition* step swapped (path → `git clone/fetch`). So this slice is a deliberate stepping-stone, not a throwaway. **Investigate HashiCorp `go-getter` (v2)** as the unifying acquisition layer for that step — see §11 OQ-3.
- **Deferred**: network/git fetch from a remote origin; origin-reference resolution coupling (this mode takes a path directly); `@ref`/`--pin` immutable pins; multi-client symlink materialization (vendor the canonical copy only — symlink views are a later §6 concern).
- **Origin-driven, not path-driven** (clarified 2026-05-30): `add` **requires a configured origin** and resolves it via the shared resolver — no `--from`/path argument (that would bypass the `init --origin` contract). The origin *value* may be a local checkout for this increment. Borrow the **source grammar** for the skill identity (`<skill>`, optionally `OWNER/REPO//path@ref` later) from `gh skill` / go-getter (§12, OQ-3). *Open (planning):* exact grammar for naming the skill within the origin, and whether the origin reference itself must grow a local-path form or reuse `OWNER/REPO`.

---

## 5. `verify` — integrity gate

**Pattern** (cli.md): **Verification Gate** — offline, deterministic, exit-code driven, **no online/inferential signal** (AP-02). Reads only committed/​on-disk files. Needs **no origin and no network** (works before/without `init`).

Two checks, both exit-2 class on failure:

1. **Label-honesty** — for each locked skill, recompute `skillcore.TreeSHA(path)` and compare to the lock's `treeSha`. Mismatch ⇒ fail, naming the skill + expected vs actual.
2. **Orphan / completeness** — the set of skill directories present under the canonical project skill location must **equal** the set of locked skills. An on-disk skill with no lock entry (**orphan** — the supply-chain vector, architecture §9b) ⇒ fail; a locked skill absent on disk (**missing**) ⇒ fail.
   - Canonical location: `.agents/skills/` (architecture §6). A "skill on disk" = a directory containing a `skill.toml`/`SKILL.md`. Since multi-client symlink views are **deferred** (clarified 2026-05-30 — `add` creates only `.agents/skills`), the orphan check **scans only the canonical location** and need not reason about views in this slice. (When views land, *realpath-containment* applies — resolve each candidate dir's real path and only count entries whose resolved path stays inside the canonical root, so a symlink view like `.claude/skills/foo → ../.agents/skills/foo` is not double-counted. Deferred with multi-client materialization.)

**Exit codes (this slice):**

| Code | Meaning here |
|---|---|
| 0 | All locked skills match (label-honesty) and on-disk set = locked set. Incl. the empty case (no skills, no orphans). |
| 1 | Usage/config: malformed/unreadable lock, bad flags, not inside a git repo. |
| 2 | Verification failure: any label-honesty mismatch **or** any orphan/missing skill. |
| 3 | **Not emitted by `verify`** — reserved for `doctor`'s prerequisite class. |

**Conflict markers** (cli.md lists as exit-2): **deferred** (revalidated & confirmed 2026-05-30). Two reasons: (1) **correctness is already covered** — a vendored file containing `<<<<<<<`/`=======`/`>>>>>>>` differs from the locked clean content, so its recomputed tree-SHA won't match the lock and `verify` **already fails it as a label-honesty mismatch**; an explicit marker check would only *upgrade the error message*, not catch a new case. (2) **No producer exists** — the only thing that writes markers is `bump`'s 3-way merge, which is out of scope. The exit-2 slot is reserved; `verify`'s taxonomy grows the distinct "unresolved conflict markers" reason when `bump` lands.

**Output** (cli.md two-level): human = compact summary (`N skills verified` / per-failure lines) + footer hint; `--json` = complete, structurally complete per-skill verdicts (`name, path, expectedTreeSha, actualTreeSha, status, orphan/missing`) + overall result. `--verbose` surfaces raw causes. Tests assert *shape* + exit code, not just `Contains` (constitution II).

---

## 6. Edge cases (technical)

- **No lock file present** → treated as an **empty lock** (zero locked skills). With no on-disk skills ⇒ exit 0 (clean, idempotent no-op); with on-disk skills ⇒ all are orphans ⇒ exit 2.
- **Malformed/unparseable lock** → exit 1 (usage/config), errors-as-navigation (what/why/fix), raw cause under `--verbose` — never a silent skip (a corrupt *gate* input must be loud, unlike a malformed *origin* config which is skipped in resolution).
- **Skill dir with no manifest** → not counted as a skill (or flagged — planning decision; lean: ignore non-skill dirs, only `skill.toml`-bearing dirs count).
- **Symlink view dirs** → not applicable this slice: multi-client views are deferred (`add` creates only canonical `.agents/skills`), so the orphan check scans the canonical location only. (Realpath-containment handling arrives with multi-client materialization — see §5.)
- **Not inside a git repo** → exit 1 (tree-SHA needs git's object model); message tells the user to run inside the vendored repo.

---

## 7. Ground-truth anchoring (constitution III)

- A **sample-origin fixture**: a real git repo containing ≥1 skill subtree with a `skill.toml`. Its real git tree-SHA is the ground truth. **Setup-helper requirement:** the integration tests need helpers to (a) bootstrap the origin (git init + commit) and (b) lay down the origin-template filesystem from committed `testdata/`. *Open (planning) — two strategies, decide in plan.md:*
  - **(i) Embed a real (bare?) git repo** committed inside the skillrig-cli repo → fixed, reproducible tree-SHA *and* commit, but a nested git repo is awkward to store/maintain.
  - **(ii) Bootstrap in a tmpDir per test** from `testdata/` files + `git init`/commit in a helper → cleaner to maintain. **Determinism note:** the **tree-SHA is deterministic** either way (it depends only on content), but the **commit SHA is not** (it depends on author/timestamp) — so tests should assert the tree-SHA / fingerprint, and treat the recorded `commit` as present-and-well-formed rather than a fixed value, unless commit identity is pinned (e.g. fixed `GIT_AUTHOR_DATE`/committer env).
- The `TestQuickstart_*` integration tests build the binary, run `add <fixture-path>`, assert the lock records the **fixture's actual** `treeSha`/`commit`, then run `verify` and assert exit 0; then tamper one byte and assert exit 2 with the named skill; then introduce an orphan dir and a missing dir and assert exit 2 for each.
- No tree-SHA is hand-written into a fixture — it is always computed by `skillcore` from real content, so SC "same primitive both sides" is genuinely exercised.

---

## 8. Cross-doc updates required

- **`docs/ARCHITECTURE-v0.md`** — ✅ updated this branch (§2 table + overlap paragraph, §8 prereq bullet + reference-design para, open-Q5 resolved, v0 roadmap bullet). Captures the doctor-design clarification (C1/C2/C3).
- **`docs/design/cli.md`** — needs a matching update during planning/implementation (CLAUDE.md: a CLI behavior change updates `cli.md` in the same branch): the command index line for `verify` ("integrity + prereq check") and the Verification-Gate row should state `verify` is **integrity-only**, with prerequisite/eligibility attributed to `doctor`. The exit-code *table* (2 = label-honesty/orphan/conflict-markers; 3 = prerequisite) stays correct as the *contract*; only the verb-to-class attribution changes.
- **Skill co-evolution (constitution IX)** — a skill update teaching agents to run `skillrig verify`, interpret exit `0`/`2`, and understand that prerequisites are a `doctor` concern (not a verify failure). New skill vs. extend `skillrig-init` — planning decision.

---

## 9. Deferred / out of this increment

- Prerequisite/eligibility check + exit 3 → **`doctor`** (future spec; C3 intent recorded above).
- `bump` (upstream advance, 3-way merge) + conflict-marker detection.
- Network/git **fetch** in `add`; origin-resolution-driven `add`; `@ref`/`--pin` immutable pins; **auth for remote `add`** (PAT/SSH/registry token — see §11 OQ-2).
- `index.json` / `search`; multi-client symlink materialization (§6); allowlist/audit (§9b, v1); auth (R18).

---

## 10. `skillcore` as a public SDK (requirement for planning)

**SDK-1 (requirement):** `skillcore` MUST be consumable as a **Go SDK** by third-party Go projects that want to implement skill `add`/`verify` (and future operations) on top of the same primitives — i.e. **not** locked inside Go's `internal/` (which is import-unreachable outside the module). Goal: a third-party tool can `import` it and do `skillcore.Verify(repoPath)` / `skillcore.Add(opts)`, rendering its own output.

**No rule blocks this** (verified 2026-05-29):
- Constitution §IV "Single implementation of integrity primitives" mandates *one* implementation everyone dispatches to; it says nothing about visibility. A public SDK **strengthens** AP-04 — external tools reuse the one source instead of re-deriving tree-SHA.
- Constitution §V layering requires execution logic to be presentation-free; an SDK **must** be presentation-free anyway → aligned, not in tension.
- The `internal/skillcore` naming in `docs/ARCHITECTURE-v0.md` (§1, §2, §5, §9), `CLAUDE.md`, and `cli.md` is a **convention inherited from `internal/config`, not a constitutional rule.** The architecture's vNext note already anticipates *other* surfaces (MCP) dispatching to `skillcore`; an SDK is exactly another such surface — it runs with the design's grain.
- The PRE-RELEASE marker (`CLAUDE.md`) means the SDK can be exposed **now with no backward-compatibility obligation** — its API may churn freely while we iterate.

**Network independence — precisely scoped** (re-evaluated 2026-05-29 against prior art, §11): the *primitives* never fetch (§3) — `TreeSHA` / `ParseManifest` / lock-I/O operate on a local working tree and are genuinely network-free, model-agnostic, and **confirmed by prior art** (the skills.sh implementation computes its content hash locally, *post-fetch*). `verify` is likewise network-free and origin-independent (it recomputes and compares to the committed lock). **But `add`-the-capability is NOT network-free in general** — it is offline *only* because this slice deliberately scopes it to a *local source*. A future remote `add` must fetch (git clone/fetch, or an HTTP registry) and authenticate. So the honest SDK boundary is: **`skillcore` = pure filesystem-operating core (no fetch, ever); acquisition + auth = a layer above it that the CLI or SDK consumer supplies.** Do **not** over-read this as "add needs no network" — it means the *core* needs no network. (My earlier "a git remote can be a `file://` path so fetch is basically local" framing was a weak escape hatch and is **retracted** — see §11: the real origin may be an HTTP registry with no git remote at all.)

**Proposed SDK surface (plan.md finalizes):**
- Primitives: `TreeSHA`, `ParseManifest`, lock `Read/Write` + typed structs.
- Operation-level, presentation-free entry points so a consumer can do the whole job and render its own output: e.g. `Add(opts) (AddResult, error)`, `Verify(repoPath) (VerifyReport, error)` returning structured verdicts + typed errors (no stdout/stderr writes).
- This pushes execution logic **out of `runXxx()` into the package** — a deliberate revision of cli.md's "these are not separate packages, just a concern within `runXxx()`" line: for SDK consumability the execution layer SHOULD be a separate importable package, with `runXxx()` reduced to flag-parse → call SDK → render.

**Packaging options (plan.md decision — record trade-off):**
- (a) Exported package at module root (`skillcore/`) or `pkg/skillcore/` — simplest; versioned with the CLI.
- (b) Separate Go module (`github.com/skillrig/skillcore`) — independent SemVer for SDK consumers, at the cost of multi-module release overhead.
- Either honors AP-04: the CLI imports the same package the SDK exposes.

**Reconciliation list (planning, same branch per the CLI-doc rule — doc-convention changes, not contract changes):** rename `internal/skillcore` → the chosen public path in `docs/ARCHITECTURE-v0.md` (§1, §2, §5, §9), `CLAUDE.md` (Architecture section + skillcore note), and `cli.md` (the Execution-vs-Presentation "not separate packages" statement, which the SDK goal reverses).

---

## 11. Prior-art contrast & the git-origin coupling (re-evaluation 2026-05-29)

Explored `/Users/vincentdesmet/specledger/specledger/pkg/cli/skills` — an existing `npx skills` / **skills.sh** implementation targeting an **HTTP registry hosted on Vercel**, *not* a git repo. Findings that bear on our design:

- **It is network-strict — no offline path.** `add` discovers via the **GitHub Trees API** (`api.github.com/.../git/trees`), fetches each `SKILL.md` via **`raw.githubusercontent.com`**, queries an audit API + telemetry on **`add-skill.vercel.sh`**, with a `git clone --depth 1` fallback only for non-GitHub/API-failure cases. No `file://`, no cache; the source parser accepts only `owner/repo` or git URLs. (`client.go:23–273`, `discover.go:26–182`, `source.go:40–99`.) **Takeaway:** our local-path `add` is a *divergence/addition* vs this prior art (which has no offline mode) — we are designing the offline UX **new**, not borrowing it.
- **It uses a bespoke SHA-256 over the installed files, NOT a git tree-SHA** (`hash.go:21–68`), recording `{source, ref, sourceType, computedHash}` — note **ref only, no commit SHA** (`lock.go:11–24`). This is exactly the "custom content hash" our architecture **§4.2 explicitly rejected** in favor of the git tree-SHA, on the stated grounds that "the origin already computes git tree SHAs for free."

**The sharp finding — §4.2's "tree-SHA is free" justification is contingent on a git origin.** It holds only when the origin is a **git repo** (our stated model — §2c: "the origin … is literally the git remote skills are fetched from"). For an **HTTP-registry origin like skills.sh/Vercel there is no git tree object** to get for free, and our label-honesty primitive ("recompute the subtree's git tree SHA and compare to the origin's recorded tree SHA") has nothing origin-side to compare against. The prior art's SHA-256 is the *consequence* of that: a registry model is forced into a bespoke digest.

**Assumptions + open questions (for plan.md / architecture):**
- **A-1 (make explicit):** skillrig's integrity model **presumes a git-repo origin**. Internally consistent with §2c/§4.2 and our local-path slice (the fixture is a git checkout, so `TreeSHA` *and* the `commit` provenance read offline). State it; don't leave it implicit.
- **OQ-1:** if skillrig ever consumes an **HTTP-registry origin** (skills.sh-style), the integrity primitive must change from "compare to the origin's git tree SHA" to "recompute a content digest from the fetched bytes" (on-disk self-consistency), **losing the origin-attested "modified-in-transit but mislabeled" check** unless the registry publishes a trusted digest. Decide: git-origin-only (current design), or also support registry origins?
- **OQ-2 (auth, future network `add`):** remote `add` from a **private** origin needs credentials — a GH **PAT** (`GITHUB_TOKEN`/`GH_TOKEN`, as the prior art reads) or an **SSH key** for git, or a **registry token** for an HTTP registry. Re-enters scope the moment network `add` lands; deferred today (this slice is local-origin, no auth). Pairs with the doctor-side prerequisite **auth** check (R18) already moved out of `verify`. **MVP lean (confirmed 2026-05-30):** production `add` is **GitHub-only** (no arbitrary git remotes) for quick delivery, and reusing **`gh` CLI's auth as a library** is a strong candidate for the token path — both are plan.md decisions. Local-origin consumption (this slice) needs none of it.
- **OQ-3 (acquisition library):** evaluate **HashiCorp `go-getter`** (the fetch engine behind Terraform/Packer/Nomad) as the acquisition layer *above* `skillcore`. It unifies `file://` (our local-path slice), `git::ssh/https` with `?ref=` + `//subdir` (the git-origin future), and `http`/`s3`/`gcs` under one source grammar + detectors — a clean fit for "acquisition = the layer above the core." **Two things to verify before adopting:**
  1. **Dependency footprint** vs the architecture's "minimal deps, static consume-only binary" stance — prefer `go-getter/v2` and trim unused getters (the s3/gcs detectors drag in cloud SDKs). A thin `git` wrapper (we already require `git` on PATH) may honor minimal-deps *better* **if** skillrig stays git-origin-only.
  2. **Provenance capture** — go-getter is built to *get content*, not to preserve git identity; confirm we can still obtain the **resolved commit SHA** and compute the **git tree SHA** for the lock (e.g. capture the commit *before* go-getter copies the subdir / drops `.git`). If it can't, it doesn't serve our provenance need and a `git` wrapper wins.

  **Its value scales with OQ-1:** git-origin-only → a thin `git` wrapper is likely sufficient; multi-origin (incl. HTTP registry) → go-getter's unified acquisition + checksum support becomes attractive. Decide OQ-1 first, then OQ-3.
- **Unaffected:** `verify` and the `skillcore` primitives stay network-free and origin-model-agnostic regardless of the above — they only ever recompute against the *committed lock* and *on-disk content*. **The coupling bites at `add`-time provenance, not at verify-time.**

---

## 12. Prior-art: `gh skill` (GitHub first-party, MIT) — re-evaluation 2026-05-29

Explored `/Users/vincentdesmet/specledger/skillrig/gh-cli/pkg/cmd/skills`. A **third** acquisition model alongside skills.sh (HTTP registry, §11) and skillrig (git): the **GitHub REST API** (Trees / Blobs / Contents / Refs / Releases), reusing `gh`'s auth token; network-strict except a `--from-local` mode. Subcommands: `install`(`add`) / `update` / `preview` / `search` / `publish`. **No `verify`.** (`skills.go:15-57`.)

**What it confirms about skillrig's design:**
- **It GETS the tree SHA from the Trees API (`entry.SHA`) and never recomputes it offline** (`discovery.go:544-589`). It stores that SHA in *both* `SKILL.md` frontmatter (`github-tree-sha`/`-ref`/`-path`/`-pinned`/`-repo`; `frontmatter.go:70-98`) **and** a **user-scope** lockfile `~/.agents/.skill-lock.json` (`skillFolderHash`, `source`, `sourceUrl`, `pinnedRef`, `installedAt`; `lockfile.go:94-137`). It uses the SHA only for **online update-detection** (recorded vs. remote; `update.go:302`). → **Exactly the architecture §11/§4.2 critique, now verified:** right primitive (git tree SHA), wrong *question* ("did upstream move?" not offline label-honesty), and **no offline `verify`**. skillrig's `verify` fills a gap `gh skill` genuinely does not.
- **No `[[requires]]` / backing-CLI concept** — pure file placement. Confirms skillrig's doctor-eligibility check is a real differentiator.
- **Copies, not symlinks; rewrites frontmatter** (`installer.go:251-305`). Confirms architecture §6.
- **Lockfile is user-scope only, not committed per-repo** — a cloning teammate/CI gets no provenance. skillrig's **committed project lock** is what makes the repo self-describing + CI-verifiable (architecture §3) — a divergence in skillrig's favor.

**The sharp incompatibility finding (resolves architecture open Q7):**
`gh skill` **injects provenance into each `SKILL.md`** *after* computing the upstream tree SHA — so its installed content **deliberately diverges from the tree SHA it recorded**, and it only gets away with this because it **never recomputes** the installed tree's SHA. **skillrig cannot do this:** since `verify` recomputes the on-disk subtree's tree SHA, **any in-file injection fails label-honesty.** Therefore:
- skillrig's **lockfile-only, no-frontmatter-mutation** provenance is **required by the integrity model**, not a preference (recorded in §4 "no content mutation").
- **You cannot wrap `gh skill`'s placement** and keep tree-SHA label-honesty — its frontmatter rewriting destroys the very SHA you'd verify. Concrete evidence for architecture **§11b Option B (reimplement core)**, not wrap.

**What to borrow:**
- **Source grammar** for skillrig's `add` (the §4 open question): `OWNER/REPO`, `OWNER/REPO@ref`, **`OWNER/REPO//path`** (subdir), `./local` + `--from-local`. The `//path` subdir syntax **converges with go-getter's `//subdir`** (§11 OQ-3) — a point in go-getter's favor for a unified grammar.
- **`--from-local` is direct prior art for our local-path `add`** — but `gh skill`'s local mode injects only a `local-path` field and records **no git tree SHA / commit** (`frontmatter.go:102-127`). skillrig's local-path `add` deliberately **requires a git checkout** so it records a real tree SHA + commit (§4, §7) — a deliberate improvement, not a copy.
- **Soft-pin** semantics (`--pin v1.0.0` stored as a human-readable ref, re-resolved each fetch, used only as update-skip) vs skillrig's intended **immutable** skill pin (architecture §2d) — decide pin hardness at planning.
- **Upstream-provenance redirect** (`install.go:225-298`): detects re-published skills via `github-repo` metadata and offers the upstream — a governance idea adjacent to skillrig's allowlist/orphan work (v1).
