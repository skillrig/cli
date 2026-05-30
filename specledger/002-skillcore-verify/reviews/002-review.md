---
date: 2026-05-30
total_requirements: 22
total_tasks: 0
coverage_pct: "100% project-scope coverage (tasks dimension intentionally skipped)"
critical_issues: 0
---

# Cross-Artifact Verification — `002-skillcore-verify` (merged + resolved)

**Scope:** read-only cross-verification of `spec.md` against `plan.md`, `research.md`, `data-model.md`, `contracts/{add,verify,skillcore-sdk}.md`, `quickstart.md`. The **tasks dimension was intentionally skipped** (no `tasks.md` — trialling `/specledger.implement-workflow` instead). Not a defect.

**Two independent reviews merged.** Review **A** (independent agent) and review **B** (cross-check, this session) were complementary — each caught a finding the other missed. All findings below are **resolved** (artifacts updated 2026-05-30).

## Findings (all resolved)

| ID | Source | Category | Severity | Summary | Resolution |
|----|--------|----------|----------|---------|------------|
| **C1** | B | Consistency / Decision | HIGH | **Committed-tree vs working-tree.** Spec FR-009 said "current on-disk content" (working-tree), verdict enum + FR-022 + SC-001 reflected a no-commit model — but research D2 + contracts + quickstart implement **committed-tree** hashing + a **`dirty`** verdict + a commit step. (A scored "0 ambiguities" and missed this.) | **Reconciled spec to committed-tree+dirty:** FR-009 (committed content; uncommitted → distinct `dirty`), FR-022 (dirty in exit-2), Key-Entities verdict (adds `dirty`, maps spec↔`--json` names), SC-001 (vendor→commit→verify loop). |
| **Q1** | A | Quickstart drift | CRITICAL | **US3 scenario 3** asserted per-client-view handling that the feature **defers** (FR-011 / Out-of-Scope) — stale acceptance scenario. (B missed this.) | US3.3 rewritten to the deferred / canonical-only behavior, aligned with FR-011. |
| **C3 / Q2** | A + B | Coverage gap | HIGH | FR-018 / SC-009 (help with ≥2 examples) had **no `--help` quickstart test**. | Added `TestQuickstart_AddHelpExamples` / `VerifyHelpExamples` (output-shape: purpose + ≥2 examples). |
| **C4** | A + B | Coverage gap | MEDIUM | FR-015 (verify read-only) had no test asserting files/lock unchanged. | Added `TestQuickstart_VerifyIsReadOnly` (before/after `git status --porcelain` + lock byte-unchanged). |
| **C2** | B | Coverage gap | MEDIUM | FR-014 / SC-006 (missing backing tool never fails verify) only implicit. | Made explicit on `VerifyPasses`: the vendored skill declares `[[requires]]` for tools absent in CI, yet verify exits 0. |
| **C6** | B | Terminology | LOW | Verdict-name drift (spec *matched/content-mismatch/untracked* vs contracts *ok/mismatch/orphan*). | Folded into the Key-Entities verdict reword — spec now shows both the readable name and the `--json` field value. |
| **C7** | B | Wording / Scope | LOW | `add` not-a-git-repo error rationale was *verify's* reason ("tree-SHA + provenance need git") — false for `add` (those come from the origin). Plus: "all commands require git" is too strong. | Corrected the `add` error rationale (project-scope: places `.agents/skills` at the repo root, writes a lock `verify` checks). Scoped the git requirement to **project scope** (FR-001 / Assumptions / Out-of-Scope), with `--global` as a deferred carve-out. Recorded two global-scope forward concerns in spike §9 (`add --global` non-git target; `verify --global` needs a working-tree fingerprint). |
| **C5** | B | Traceability | INFO | Public-SDK scope (SDK-1, third-party `pkg/skillcore`) lives in spike/research, not spec FR-016/017. | Left as-is — deliberate earlier choice to keep SDK-1 in the spike (plan input). Noted. |
| **T1** | A | Task coverage | INFO | `tasks.md` absent → task↔requirement + `TestQuickstart_*`-task mapping skipped. | Intentional (experiment). Re-run after `/specledger.tasks` if durable task coverage is wanted. |

## Coverage summary

All 22 FR + 9 SC trace to plan + contracts + a `TestQuickstart_*` scenario after the fixes (help / read-only / prereq gaps closed; US3.3 reconciled). Reverse traceability: the only previously-orphan behavior (`dirty` verdict) is now grounded in FR-009/FR-022.

## Decision integrity

exit 0/1/2-not-3 · add-detect+refuse-not-merge · conflict-markers-deferred · symlinks-deferred · requires-NOT-in-lock · shell-`git` tree-SHA · `pkg/skillcore` · origin-resolution-not-`--from` · byte-identical vendoring · oracle independence — **applied consistently across all artifacts**, no stale wording remaining. The committed-tree+`dirty` refinement (C1) is now propagated to the spec.

## Metrics

- Requirements: 22 FR + 9 SC · Tasks: 0 (skipped) · Critical: **0** (was 1 — Q1, resolved) · High: 0 (C1 + Q2 resolved) · Medium/Low/Info: resolved or noted.

## Next actions

- Artifacts are internally consistent — **clear to proceed to implementation** (`/specledger.implement-workflow` experiment, or `/specledger.tasks` for the durable ledger).
- Re-run `/specledger.verify` after `tasks.md` exists if you want task-coverage + `TestQuickstart_*`-task mapping validated.
