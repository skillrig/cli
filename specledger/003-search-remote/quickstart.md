# Quickstart — Acceptance Contract: `003-search-remote`

Each scenario is an executable `TestQuickstart_*` (Constitution §II): concrete invocations, observable output, exit codes, and **output-shape** assertions (bounded human lines; parseable+complete `--json`; 3-part errors). Every user story (US1–US5) maps here. Tests build and exec the real binary against the S4 substrate.

## Test substrate (S4 / D6)
- **Origin fixture** bootstrapped in `t.TempDir()`: a working tree with `index.json` + `skills/terraform-plan-review/SKILL.md` (frontmatter), committed; pushed to a local **bare** repo. The CLI's origin is `file://<bareDir>` for the remote-fetch path.
- **Failure injection:** the existing `pkg/skillcore/git.go` `commandContext` exec-stub seam (extended to `Clone`/`FetchSparse`) returns crafted `(exit=128, stderr=…)` for auth/unreachable/transient — `pkg/skillcore` unit tests, not integration.
- **Ground-truth oracles:** `fetched treeSha == rawTreeSHA(fixture,"HEAD","skills/<name>")`; `skillrig index`(fixture) == committed `index.json`.

---

## US1 — Discover (search) · P1

**`TestQuickstart_SearchListsSkills`** — Given an origin publishing ≥2 skills, `skillrig search` (no query) lists each (`name`, `version`, one-line desc) + footer hint; assert `len(lines) ≤ matches + 5`.
**`TestQuickstart_SearchQueryMatchesNameDesc`** — `skillrig search terraform plan` returns only skills whose name+description+topics contain **both** terms (token-AND substring); a skill matching one term but not the other is excluded (FR-002).
**`TestQuickstart_SearchOrderingDeterministic`** — for a query hitting several skills, results are ordered by the fixed relevance bucket then name, and are **byte-identical across two runs** (D8/N6, SC-002).
**`TestQuickstart_SearchFilterByTopic`** — `skillrig search --topic aws` lists only aws-topic skills; identical across two runs.
**`TestQuickstart_SearchEmptyResult`** — `skillrig search --topic nonesuch` → `no skills matched`, **exit 0**.
**`TestQuickstart_SearchJSONComplete`** — `--json` parses (`json.Unmarshal` ok) and every entry has name/version/namespace/description/topics/path (field-presence, not truncation).
**`TestQuickstart_SearchConventionMismatch`** — origin catalog `skillrigConvention: 2` → exit 1, message names a compatibility mismatch + "update skillrig" (3 parts).
**`TestQuickstart_SearchConventionBoundary`** (C1) — exact-match gate: `skillrigConvention: 0` **and** an absent field each → exit 1 `IncompatibleConventionError` (a lower/missing convention does **not** silently pass), while `1` passes — pinning the non-`>` boundary so FR-016/SC-005 is unambiguous.
**`TestQuickstart_SearchHelpExamples`** — `search --help` shows purpose + ≥2 examples.

## US2 — Acquire remotely (add) · P1

**`TestQuickstart_AddRemoteNoLocalCopy`** — Given a `file://` origin and **no** local checkout, `skillrig add terraform-plan-review` vendors the subtree into `.agents/skills/…` byte-identical to the fixture, writes a lock entry (`version`/`commit`/`treeSha`/`path`); then **`skillrig verify` exits 0**. Ground-truth: lock `treeSha` == raw `git ls-tree`.
**`TestQuickstart_AddRemoteIdempotent`** — re-running `add` on the unchanged vendored skill → `unchanged`, **exit 0**, lock byte-unchanged, no FS change (SC-006).
**`TestQuickstart_AddRemoteForceOnDivergence`** — locally modify the vendored skill, re-`add` → refused with a `--force` hint (002 parity); `--force` overwrites.
**`TestQuickstart_AddDryRun`** (C6) — `add … --dry-run` prints a bounded preview, **exit 0**, and leaves the working tree + lock byte-unchanged (`git status --porcelain` empty, lock unchanged) — FR-020 dry-run for the remote path.
**`TestQuickstart_AddHelpExamples`** (C5) — `add --help` shows the purpose line + **≥2 runnable examples**, one of which is the `--pin` form (SC-008 for the second consumer command; bounded shape).

## US3 — Reproducible pin · P2

**`TestQuickstart_AddPinnedReproducible`** — `add … --pin v1.4.0` on two clean repos → byte-identical content + identical lock (`version=1.4.0`, same `commit`/`treeSha`) (SC-004).
**`TestQuickstart_AddPinTagFormEquivalent`** (C3) — `add … --pin v1.4.0` and `add … --pin terraform-plan-review-v1.4.0` resolve to the **same** `commit`/`treeSha` (bare-semver expansion == full-tag literal), confirming the deterministic `--pin` resolution rule.
**`TestQuickstart_AddPinNotFound`** — `--pin v9.9.9` → exit 1; assert the error is a **`NoSuchVersionError`** (typed/structured discriminator, not a substring) — distinct from skill-not-found (FR-015, C2).

## US4 — Trustworthy failures · P2 (unit-level via the stub seam + integration)

**`TestSkillcore_ClassifyAuthError`** (unit) — stderr `Authentication failed` → `AuthError`.
**`TestSkillcore_ClassifyUnreachable`** (unit) — stderr `Could not resolve host` → `UnreachableError`.
**`TestSkillcore_ClassifyNotFound`** (unit) — stderr `repository '…' not found` → `NotFoundError`.
**`TestQuickstart_AddAuthFailureDistinct`** — injected auth failure → exit 1, message is an **authentication** failure distinct from not-found/unreachable, points at `gh auth login`/`GITHUB_TOKEN`.
**`TestQuickstart_AddPrivateNotFoundHintsAuth`** — not-found + no resolved token → message adds the "if private, authenticate" hint (D4 subtlety).
**`TestQuickstart_AddUnreachableDistinct`** — injected unreachable → exit 1, distinct message.
**`TestQuickstart_VerboseShowsRawCause`** — any of the above with `--verbose` prints the raw git/gh stderr (never swallowed).

## US5 — Catalog generation (index) · P2

**`TestQuickstart_IndexGenerates`** — `skillrig index` over the origin fixture writes `index.json` whose entries match the skills' frontmatter, **including topics** (the field `build-index.sh` dropped).
**`TestQuickstart_IndexDeterministic`** — run twice on unchanged skills → byte-identical output (SC-009).
**`TestQuickstart_IndexMatchesCommitted`** — `skillrig index` output **equals** the committed PoC `index.json` (producer == artifact oracle).
**`TestQuickstart_IndexMalformedFrontmatter`** — a skill with broken frontmatter → exit 1 naming the offending `SKILL.md`.
**`TestQuickstart_IndexNotInOrigin`** (C8) — running `skillrig index` outside an origin repo (no `.skillrig-origin.toml` / unreadable `skills_dir`) → exit 1 with the what/why/fix "run inside the origin repo" navigation message.
**`TestQuickstart_IndexMissingVersion`** (C9) — a skill whose frontmatter omits the required `x-skillrig.version` → exit 1 naming the offending `SKILL.md` (the catalog-entry validation rule from data-model §1; guards the seed-enrichment precondition of `IndexMatchesCommitted`).

## Regression (no 002 break · SC-007)
**`TestQuickstart_AddLocalPathStillWorks`** — the 002 local-path `add` suite passes unchanged against an explicit local-path origin.
**Manifest migration:** existing `verify`/`add` ground-truth + lock tests pass after `ParseManifest` is rewritten on `SKILL.md` frontmatter (the migrated fixtures replace `skill.toml`).

---

### Traceability
| US | Scenarios | FRs | SCs |
|---|---|---|---|
| US1 search | SearchQueryMatchesNameDesc/ListsSkills/OrderingDeterministic/FilterByTopic/EmptyResult/JSONComplete/ConventionMismatch/ConventionBoundary/HelpExamples | 001–002a, 003, 004, 005, 016, 020, 021 | 002, 005, 008 |
| US2 add remote | AddRemoteNoLocalCopy/Idempotent/ForceOnDivergence/DryRun/HelpExamples | 006–010, 012, 020 | 001, 003, 006, 008 |
| US3 pin | AddPinnedReproducible/AddPinTagFormEquivalent/AddPinNotFound | 013–015 | 004 |
| US4 failures | Classify*/AddAuth/PrivateNotFound/Unreachable/Verbose | 016–019, 022 | 005 |
| US5 index | IndexGenerates/Deterministic/MatchesCommitted/Malformed/NotInOrigin/MissingVersion | 023, 025–028 | 009 |
| regression | AddLocalPathStillWorks + migration | 011 | 007 |
