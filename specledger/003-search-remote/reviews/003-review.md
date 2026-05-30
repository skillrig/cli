---
date: 2026-05-31
total_requirements: 37
total_tasks: 0
coverage_pct: "86% (32/37 fully covered; 5 gaps flagged C1/C5/C6/C8/C9 — all remediated)"
critical_issues: 0
---

# Specification Analysis Report — 003-search-remote (no-tasks cross-verify)

**Scope:** spec.md ↔ spec-tech/plan/research/data-model/contracts/quickstart. Tasks dimension intentionally skipped. 2 independent reviewers (Opus, loading agentic-go-cli-design + golang-spf13-cobra + golang-testing) merged. Produced by `/specledger.verify-workflow` (disk brief + template).

> All findings below were **remediated 2026-05-31** (C1 per the exact-match `== 1` decision). See the Remediation section at the end.

| ID | Source | Category | Severity | Location(s) | Summary | Recommendation |
|----|--------|----------|----------|-------------|---------|----------------|
| C1 | r1,r2 | Consistency/Ambiguity (convention policy) | HIGH | spec-tech.md:51; data-model.md:97,141; contracts/search.md:21; quickstart.md:20 | Convention-version policy undecided AND contradictory: data-model:97 "supports 1" (exact-match) vs :141 trigger `> supported` (forward-only — convention 0 or a missing field silently PASSES). Only `convention:2` tested, so FR-016/SC-005 unverifiable. | Pick exact-match `== 1` (YAGNI); make :97/:141 agree (`> supported`→`!= 1`); align data-model §2/§5 + search.md step 3; add a non-`>` boundary quickstart case. |
| C2 | r2 | Consistency/Ambiguity (FR-015) | MEDIUM | contracts/add-remote.md:39; research.md:53 (D5); data-model.md:132-141 | "no such version" vs "skill not found" modeled as a `NotFoundError` *variant* with no structured discriminator — message-only differentiation, which errors-as-navigation cautions against (agents/CI can't branch on prose). Test asserts only a substring. | Add a distinct typed error (`NoSuchVersionError`) or a `kind` discriminator; assert it in the test, not just a substring. |
| C3 | r1 | Decision integrity/Ambiguity (`--pin`) | MEDIUM | spec-tech.md:57; contracts/add-remote.md:14; data-model.md:116 | `--pin` accepts bare semver / full `name-vSEMVER` tag / raw SHA with no disambiguation rule; FR-013/014/015 + SC-004 depend on a single deterministic resolution. | Specify resolution order (`^v?SEMVER$`→expand via tag_scheme; else literal ref/SHA) + precedence; quickstart bare-semver vs full-tag equivalence. |
| C4 | r1,r2 | Decision integrity (D8 tag→topic) | MEDIUM | spec-tech.md:33,102; contracts/index.md:42; research.md:13 | Residual `tag`/`tags`/`--tag` from the D8 rename in binding artifacts. | Replace with `topic`/`topics` (historical S1 quote + data-model "renamed from tags" may stay). |
| C5 | r1 | Coverage (add --help) | MEDIUM | contracts/add-remote.md:42; quickstart.md US2; spec.md SC-008 | `TestQuickstart_AddHelpExamples` referenced in the contract but absent from quickstart.md; SC-008 mapped only to US1. | Add the scenario to US2 (purpose + ≥2 examples incl. `--pin`); map SC-008 to US2. |
| C6 | r1 | Coverage (add --dry-run) | MEDIUM | spec.md:169 (FR-020); contracts/add-remote.md:15,27; quickstart.md US2 | No scenario exercises `add --dry-run` for the remote path; FR-020 dry-run clause unverified. | Add `TestQuickstart_AddDryRun` (bounded preview, exit 0, no FS/lock mutation). |
| C7 | r1 | Consistency (producer hardcodes convention) | LOW | contracts/index.md:21,26; spec-tech.md:30; data-model.md:97 | `index` "carry `skillrigConvention: 1`" as a literal vs `.skillrig-origin.toml` `convention_version` being the source of truth — a future divergence self-rejects. | State `index` reads `skillrigConvention` from `.skillrig-origin.toml`, not a hardcoded 1. |
| C8 | r1 | Coverage (index not-in-origin) | LOW | contracts/index.md:32,39; quickstart.md US5 | `index` exit-1 "not in an origin repo / unreadable skills_dir" path has no scenario (only malformed-frontmatter). | Add `TestQuickstart_IndexNotInOrigin`. |
| C9 | r1 | Consistency (seed vs validation) | LOW | data-model.md:54; plan.md:90 | "version required for catalog entries" hard rule vs plan step-6 seeded skills needing manual `x-skillrig.version` enrichment; no case covers a skill missing it. | Add a case asserting `index` fails clearly on missing `x-skillrig.version`; make enrichment a checked precondition of the oracle. |
| C10 | r2 | Consistency (matcher name) | LOW | plan.md:85; data-model.md:147 | Matcher named `SearchCatalog` (plan) vs `Search` (data-model §5b). | Unify on `Search` (the authored signature); fix plan.md:85. |
| C11 | r2 | Consistency (under-cited decisions) | LOW | data-model.md:3 | Header says "D1–D7" but the doc rests on D8 (§5b, topics). | Change to "D1–D8". |
| C12 | r2 | Coverage/Traceability (under-cited FRs) | LOW | quickstart.md:58-66 | US1 row omits FR-003/FR-004 (covered) and cites FR-020 only under US4. Citation gap, not a real coverage gap. | Add FR-003/FR-004 to US1; reference FR-020 from US1. |
| C13 | r2 | Consistency (help header miscite) | INFO | contracts/search.md:41 | "Help (FR-018/SC-008)" — FR-018 is the unreachable requirement, unrelated to help. | Change to "Help (SC-008/FR-020)". |
| C14 | r1,r2 | Constitution (§III divergence adequacy) | INFO | plan.md:30; constitution.md:78,82-83 | The two recorded §III divergences are ADEQUATELY handled (httptest-not-applicable; skill.toml→SKILL.md), correctly routed to the FR-024 team-approval sweep; no MUST violation. Also: §IX `scripts/run_eval.py` path is stale. | No plan change; ensure the FR-024 sweep enumerates §III:78, §III:82-83, and the stale §IX `run_eval.py` path. |

### Coverage summary

| Requirement | Plan | Contract | Quickstart test | Status |
|-------------|------|----------|-----------------|--------|
| FR-001 | step 5 | search.md | SearchListsSkills | Covered |
| FR-002 | step 5 | search.md | SearchQueryMatchesNameDesc / SearchOrderingDeterministic | Covered |
| FR-002a | step 5 | search.md | SearchFilterByTopic | Covered |
| FR-003 | step 5 | search.md | SearchJSONComplete | Covered (C12 citation) |
| FR-004 | step 5 | search.md | SearchEmptyResult | Covered (C12 citation) |
| FR-005 | step 5 | search.md | SearchListsSkills | Covered |
| FR-006..012 | step 6 | add-remote.md | AddRemoteNoLocalCopy/Idempotent/ForceOnDivergence/LocalPathStillWorks/ClassifyNotFound | Covered |
| FR-013/014 | step 6 | add-remote.md | AddPinnedReproducible | Covered (C3 pin grammar) |
| FR-015 | step 6 | add-remote.md | AddPinNotFound | Covered, was message-only (C2) |
| FR-016 | step 5/6 | search.md/add-remote.md | SearchConventionMismatch | Gap → fixed (C1) |
| FR-017/018/019 | step 6 | add-remote.md/search.md | ClassifyAuth/Unreachable / VerboseShowsRawCause | Covered |
| FR-020 | step 5/6 | search.md/add-remote.md | SearchJSONComplete; add --dry-run | Gap → fixed (C6) |
| FR-021/022 | step 5/6 | search.md/add-remote.md | SearchEmptyResult / AddRemote* | Covered |
| FR-023/024 | step 7 | (process) | — | Covered as process deliverables (C4 wording) |
| FR-025..028 | step 4 | index.md | IndexGenerates/Deterministic/MatchesCommitted | Covered |
| SC-001..004,006,007 | step 5/6 | search/add-remote | AddRemote*/Pinned/Idempotent/LocalPath | Covered |
| SC-005 | step 5/6 | search/add-remote | Auth/Unreachable/ConventionMismatch | Gap → fixed (C1) |
| SC-008 | step 5/6 | search/add-remote | SearchHelpExamples; add help | Gap → fixed (C5) |
| SC-009 | step 4 | index.md | IndexGenerates/Deterministic/MatchesCommitted | Covered (C9 precondition) |

### Decision integrity

D1 frontmatter ✓ (stale `tags` at research.md:13 — C4) · D2 catalog ✓ · D3 local-vs-remote ✓ · D4 auth ✓ · D5 lock/pin ✓ (C2/C3 caveats) · D6 test substrate ✓ (§III divergence adequate — C14) · D7 transport ✓ · D8 query/topic ✓ (residual `tag` — C4). Session 2026-05-31 bullets (S1–S5, A1, yaml.v3) all applied.

### Metrics

- Requirements: 28 FR + 9 SC · Reviewers: 2 · Critical: 0 · High: 1 · Medium: 5 · Low: 6 · Info: 2 · Coverage gaps: 5.

### Next actions

- C1 (HIGH) resolved via exact-match `== 1`; C2–C13 remediated; C14 folded into the FR-024 sweep list. See Remediation below.

---

# Remediation — 2026-05-31

All 14 findings resolved (C1 per the user's exact-match `== 1` decision; "fix all now"). Artifacts re-validated (`go build ./...` clean; no operative `--tag`/`D1–D7`/`> supported`/`SearchCatalog` stragglers — only historical spike narrative retains old terms, which review C4/C10 permit).

| ID | Sev | Status | Fix |
|----|-----|--------|-----|
| C1 | HIGH | ✅ Fixed | Exact-match policy: data-model §2 gate reworded + §5 trigger `> supported`→`!= 1` (incl. lower/absent); contracts/search.md step 3; spec-tech.md:51 (Q14 reframed as future change). New `TestQuickstart_SearchConventionBoundary` (0/absent → fail, 1 → pass). |
| C2 | MED | ✅ Fixed | New typed **`NoSuchVersionError`** in data-model §5 (now "Four typed errors"; raised from failed pin ref-resolution, not stderr); add-remote.md errors reworded; `TestQuickstart_AddPinNotFound` asserts the typed discriminator, not a substring. |
| C3 | MED | ✅ Fixed | Deterministic `--pin` rule (bare `^v?SEMVER$`→tag_scheme expand; else literal ref/SHA) in add-remote.md flag + data-model §3; new `TestQuickstart_AddPinTagFormEquivalent` (bare-semver == full-tag → same commit/treeSha). |
| C4 | MED | ✅ Fixed | `tag`→`topic` at spec-tech.md:33,102; contracts/index.md:42; research.md:13 (D1). Historical S1 quote left. |
| C5 | MED | ✅ Fixed | `TestQuickstart_AddHelpExamples` added to quickstart US2 (purpose + ≥2 examples incl. `--pin`); SC-008 mapped to US2 in traceability. |
| C6 | MED | ✅ Fixed | `TestQuickstart_AddDryRun` added (bounded preview, exit 0, no FS/lock mutation). |
| C7 | LOW | ✅ Fixed | contracts/index.md: `skillrigConvention` read from `.skillrig-origin.toml` `convention_version`, not a hardcoded 1 (shared source with the gate). |
| C8 | LOW | ✅ Fixed | `TestQuickstart_IndexNotInOrigin` added (not-in-origin/unreadable skills_dir → exit 1 navigation message). |
| C9 | LOW | ✅ Fixed | `TestQuickstart_IndexMissingVersion` added; plan step 6 marks `x-skillrig.version` enrichment a **checked precondition** of the `IndexMatchesCommitted` oracle. |
| C10 | LOW | ✅ Fixed | Matcher name unified on `Search(...)` (data-model §5b); plan.md:85 updated from `SearchCatalog`. |
| C11 | LOW | ✅ Fixed | data-model.md:3 "D1–D7"→"D1–D8". |
| C12 | LOW | ✅ Fixed | US1 traceability row gains FR-003/004/020; SC-005 added; US2 gains FR-020 + SC-008. |
| C13 | INFO | ✅ Fixed | contracts/search.md help header "FR-018/SC-008"→"SC-008/FR-020". |
| C14 | INFO | ✅ Fixed | plan step 7 enumerates the team-approval constitution touch-ups: §III:78 (skill.toml→SKILL.md), §III:82-83 (httptest→exec-stub seam), §IX stale `run_eval.py` path. |

**Gate after remediation:** `go build ./...` clean; 5 prior coverage gaps (FR-016/SC-005, FR-020 dry-run, SC-008 add-help, index not-in-origin, index missing-version) now each carry a `TestQuickstart_*` scenario. Artifacts internally consistent — **clear to proceed to `/specledger.implement-workflow`**.
