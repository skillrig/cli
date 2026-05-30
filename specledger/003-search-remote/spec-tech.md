# Technical Companion — `003-search-remote` (Discover & Acquire)

**Status:** Draft — input to `/specledger.plan`. Companion to [spec.md](./spec.md) (the user-facing contract).
**Purpose:** capture every implementation-level decision surfaced during specify, and enumerate the **seven open decisions deliberately deferred to `/specledger.clarify`**. spec.md stays non-technical; this file is where transport, authentication, fingerprint, and test-tier mechanics live. Mirrors how 002 used `spec-tech-spike.md`.

> Binding docs (do not contradict): `docs/ARCHITECTURE-v0.md` §2 (command surface), §2d (origin discovery + convention-version contract), §4.2 (treeSha = label-honesty, commit = provenance), §8b.2 (auth resolution), §9 (`index.json` + search), §9b (`OWNER/REPO[/path]@ref` identity grammar, immutable pins); `docs/design/cli.md` (search = **Query** pattern, remote add = **Vendor Mutation** pattern; errors-as-navigation; two-level output; standard flags; exit codes). Single-resolver rule (`config.ResolveOrigin`) and single-`skillcore` rule (AP-04 / AP-06) hold: remote fetch + catalog parsing get **exactly one** implementation in `pkg/skillcore`, shared by `search` and `add`.

---

## 1. What ships in this slice

Two consumer-side commands over the already-resolved origin:

| Command | cli.md pattern | Exit codes | New surface |
|---|---|---|---|
| `skillrig search [QUERY] [--tag T ...]` | Query | 0 (incl. empty result), 1 (usage/config) | brand new |
| `skillrig add <skill> [--pin <ref>] [--dry-run] [--force] [--json] [--verbose]` | Vendor Mutation | 0 (vendor or idempotent no-op), 1 (usage/config) | extends 002's local-copy `add` with a **remote** acquisition path + `--pin` |

Both resolve the origin through `config.ResolveOrigin` (precedence: `SKILLRIG_ORIGIN` env > project `.skillrig/config.toml` > global). Both must read and honor the origin's **convention version** before acting (see §4).

Out of scope (reserved): verification-failure exit 2 and prerequisite exit 3; `bump`; multi-client symlink materialization; `doctor`; the policy/allowlist enforcement in `policy.toml` (v1 governance); building the origin-side `skillrig index` generator (see Open Decision #6).

## 2. Ground truth — the real PoC origin

`github.com/skillrig/origin-template`, checked out at `/Users/vincentdesmet/specledger/skillrig/skillrig-origin`. Relevant artifacts as they exist today:

- **`.skillrig-origin.toml`** — `convention_version = 1`, `origin = "my-org/my-skills"`, `skills_dir = "skills"`, `cmd_dir = "cmd"`, `tag_scheme = "name-vSEMVER"` (a skill version is tagged e.g. `terraform-plan-review-v1.4.0`).
- **`index.json`** (the catalog) — carries `skillrigConvention: 1`, `origin`, and `skills[]` with `name / version / namespace / description / tags / path / requires`. **No per-skill `treeSha` or `commit`** — the catalog is discovery-only.
- **`skills/terraform-plan-review/`** — `skill.toml` (full manifest incl. `[[requires]]`) + `SKILL.md`. The single sample skill.
- **`scripts/build-index.sh`** — a documentation-grade catalog emitter that **emits a reduced schema** (`name/version/description/path` only — drops `tags`, `namespace`, `requires`). This **drifts** from the committed `index.json` and from what `search --tag` needs. Reconciling this is FR-023 (Open Decision #6).
- **`policy.toml`** — external-source allowlist (v1 governance; not consumed here).

**Schema the CLI will consume for `search`** (must be the reconciled, full shape): per-skill `name`, `version`, `description`, `tags[]`, `path`; catalog-level `skillrigConvention`, `origin`. `namespace` and `requires` may be carried but are not required by `search` this slice.

## 3. The seam being replaced (from 002)

002's `add` treats the origin as a **local checkout** at `<repoRoot>/OWNER/REPO`:

- `internal/cli/add.go:139` `originDirRef(origin)` maps a resolved `config.Origin{Owner,Repo,Ref}` → a local directory `OWNER/REPO` (anchored at repo root after the AR-1 fix) + ref.
- `pkg/skillcore` then reads the skill subtree from that local dir, computes the git tree-SHA, and vendors byte-identically.

Remote `add` introduces a **fetch step in front of that subtree read**: instead of requiring the directory to already exist, fetch the skill's content (and the catalog, for `search`) from `github.com/OWNER/REPO@ref`. The byte-identical vendoring + lock-write that follow are unchanged and stay in `pkg/skillcore` (AP-04).

`config.Origin` already has `Owner / Repo / Ref` and parses `OWNER/REPO[@REF]` — no config-schema change needed. `Ref` is the origin-level **branch** pointer; a per-skill `--pin` is a separate immutable tag/SHA (§5, US3).

## 4. Convention-version gate (cross-cutting, both commands)

Architecture §2d.3: the generic binary speaks a **convention contract**; it must check the origin's `convention_version` (mirrored as `skillrigConvention` in the catalog) and **fail clearly** on an incompatible origin rather than misbehaving (FR-016). This binary supports convention `1`. Decide the compatibility policy (exact-match? `N` and `N-1`?) — see architecture open Q14. The check happens once, through the shared core, for both `search` and `add`.

## 5. Identity, fingerprint, and pins

- **Provenance = `commit`**, **label-honesty = `treeSha`** (architecture §4.2). At remote-add time: fetch the skill subtree at the resolved commit, record that **commit** (provenance) and the **git tree-SHA computed from the fetched subtree** (label-honesty) into the lock — using the *same* `skillcore` tree-SHA code `verify` recomputes (R9/R14/N2). `verify` then checks on-disk content against the recorded tree-SHA, offline, exactly as today.
- **The origin publishes no per-skill tree-SHA** (the catalog has none). So label-honesty here means *"the on-disk content still matches what was vendored,"* anchored by provenance (you fetched it from the origin at a specific commit), **not** *"matches an origin-attested hash."* This is Open Decision #5 — confirm the framing is acceptable, or decide the catalog/release should publish tree-SHAs (larger origin-side change).
- **Pin (`--pin <ref>`)** — an immutable tag or SHA per skill, distinct from the origin-level `@ref` branch. `tag_scheme = "name-vSEMVER"` ⇒ a version pin maps to tag `<skill>-v<semver>` (e.g. `--pin v1.4.0` → `terraform-plan-review-v1.4.0`, or accept the full tag). Recorded so re-acquisition reproduces byte-identical content (FR-013/014, SC-004). A non-existent pin → distinct "no such version" (FR-015).

## 6. Failure taxonomy (errors-as-navigation, FR-016–019)

Three confusable classes must be **distinct typed errors** in `pkg/skillcore`, rendered with what/why/fix by `internal/cli`:

1. **Incompatible convention** — origin's convention version unsupported (FR-016) → "update skillrig" class.
2. **Authentication** — private origin, no/invalid credentials (FR-017, R18) → distinct from not-found; point at how to authenticate. The top onboarding/CI footgun; surface loudly. Must be distinguishable from "origin not found" (a missing/typo'd repo) and "skill not found" (valid origin, no such skill).
3. **Unreachable** — network failure / wrong location (FR-018) → distinct from auth and compatibility.

Plus the existing 002 classes carried forward: skill-not-found (vs origin-not-found, the AR-2/R2-M4 distinction), invalid-skill-name (path-traversal guard), overwrite-on-divergence. `--verbose` surfaces the raw underlying cause on every command (never swallowed).

## 7. Test tier — the new network boundary

002 had **no network boundary** (no `httptest`/go-vcr). This slice introduces one, so the test substrate is a first-class decision (Open Decision #7). Leaning: a **local bare git repo** acting as the "remote" origin (fixtures bootstrap it in a tmpDir, the same way 002 bootstrapped a local origin), exercised over `file://`/local-remote git transport — keeps the suite offline and deterministic while running the *real* fetch path. A ground-truth test must assert the **fetched tree-SHA == raw `git` tree-SHA** of the origin subtree (the §III ground-truth discipline, extended across the fetch). `TestQuickstart_*` scenarios exercise: remote add with no local copy, idempotent re-add, pinned reproducibility, each failure class, and search filtering/empty-result/json-completeness. Avoid coupling to live GitHub in the gate.

## 8. Open Decisions (deferred to `/specledger.clarify`)

These shape *how* the spec's goals are met. Each has a documented leaning; `/clarify` ratifies or revises. spec.md records the leanings as Assumptions so it stays internally consistent.

1. **Local-vs-remote classification + precedence.** Local add is **kept** (FR-011) — the question is how an origin is classified as "use the local copy" vs "fetch from GitHub," and which wins when both are available. Candidates: (a) prefer an existing local checkout at `<repoRoot>/OWNER/REPO`, else fetch remote; (b) explicit scheme/flag (`file://…` or a path-shaped origin = local; bare `OWNER/REPO` = remote); (c) a `--local`/`--remote` selector. Must be deterministic and the chosen source must be reported to the user. *Leaning:* (a) as the zero-config default for dev ergonomics, with the source named in output. Note interaction with `tag_scheme` for pins on a local checkout.
2. **Fetch transport.** Shell `git` partial-clone + sparse-checkout (consistent with the project's "shell `git`, no in-process hashing dependency" stance; makes the tree-SHA ground-truth trivial since git already computes it) **vs** raw HTTPS file fetch (e.g. `raw.githubusercontent`/contents API) for the catalog. One mechanism or two? *Leaning (strong):* shell `git` for the subtree; decide whether the catalog (`index.json`) is fetched the same way (a sparse single-file checkout) or via a lighter raw GET. One transport is simpler and keeps auth uniform.
3. **Catalog handling for `search`.** Fetch-every-call vs cache under `.skillrig/` (e.g. `.skillrig/cache/index.json`); and the **offline behavior** of `search` (hard-fail unreachable vs serve a stale cache with a warning). *Leaning:* fetch-per-call for correctness in the MVP, with caching deferred unless latency hurts; offline `search` fails with the unreachable error (FR-018) unless a cache decision says otherwise.
4. **Auth source + precedence.** Which credential sources, in what order (e.g. `GITHUB_TOKEN` / `GH_TOKEN` env → `gh auth` token → git credential helper), matching architecture §8b.2 and R18. How auth-failure is detected and worded (FR-017). *Leaning:* reuse the same git/`gh` token path mise relies on; no bespoke credential store. Confirm exact precedence and the GitHub Enterprise host case.
5. **Tree-SHA trust anchor.** Confirm `commit` = provenance, `treeSha` = computed-at-add (origin publishes none), so label-honesty = "matches what was vendored," not "matches an origin-attested hash" (§5). *Leaning:* accept this framing for v0; record publishing per-version tree-SHAs as a possible origin-side hardening, not in this slice.
6. **Build `skillrig index` now, or consume only?** This slice is consumer-side; the origin-side catalog generator (`skillrig index`) is a separate roadmap concern. *Leaning (recommended):* **consume only** — do *not* build `skillrig index` here. Instead reconcile the origin template's catalog **schema** (FR-023: make `build-index.sh`/`index.json` carry `tags` etc.), and ship a **contract test** that the committed PoC `index.json` parses into the structures `search` consumes (guards co-evolution). Defer the authoritative generator to its own slice.
7. **Network test substrate.** Local bare git repo fixture vs `httptest`/go-vcr (§7). *Leaning (strong):* local bare git repo + `file://` transport, with the fetched-vs-raw tree-SHA ground-truth test. Keeps the gate offline/deterministic and exercises the real code path.

## 9. Co-evolution work items (this branch touches two repos + docs)

- **FR-023 — origin template (`skillrig/origin-template`):** reconcile `scripts/build-index.sh` and the committed `index.json` so the catalog carries every field `search` consumes (notably `tags`); record the schema as the convention-1 catalog contract; note the `skillrig index` follow-up. Track whether these origin-repo edits are part of this branch's PR or a sibling work item.
- **FR-024 — `docs/ROADMAP.md` + `docs/ARCHITECTURE-v0.md`:** record the divergences — (a) roadmap 003 + 004 ship as **one** combined slice; (b) the 002 local-checkout seam is superseded by / now coexists with real remote acquisition; (c) the catalog schema is pinned to what `search` consumes. Per CLAUDE.md, a CLI behavior change updates `docs/design/cli.md` in the same branch — add the `search` command (Query pattern) and the remote `add` surface (incl. `--pin`).
- **Skill co-evolution (constitution IX):** extend the single consolidated `skillrig` skill — add `references/search.md`, update `references/add.md` for the remote path + `--pin` + the new failure classes, and update the root routing/description keywords. Run trigger evals (`model: "sonnet"` per global instructions).

## 10. Decision integrity (carried from 001/002, must stay consistent)

single origin resolver (`config.ResolveOrigin`) · single `skillcore` (no parallel fetch/hash impl) · shell-`git` tree-SHA · `pkg/skillcore` as the public SDK boundary · byte-identical vendoring · idempotent no-op = exit 0 · refuse-overwrite-on-divergence (prompt/`--force`) · errors-as-navigation with `--verbose` raw cause · two-level output (human compact + complete `--json`) · exit 2/3 reserved (not emitted here) · path-traversal + symlink guards from the 002 Qodo round still apply to remotely-fetched content.
