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

---

# Post-Implementation Adversarial Review — 2026-05-30

**Scope:** an independent cold-context agent (Opus 4.8, xhigh) reviewed the **implemented** branch against the per-user-story DoD: it read every artifact, read the code (`pkg/skillcore`, `internal/cli`, tests), ran `make check`, and exercised the binary on the add→commit→verify→tamper round-trip plus edge probes. The agent was stopped just before it emitted its final compiled report; the findings below are **distilled from its complete in-flight analysis** (cross-checked against the code). *(Process note: this agent ran a git round-trip in the repo root by mistake — see AR-P. That motivated the clean-tree-before-review rule now added to `specledger.checkpoint-workflow`.)*

## Findings

| ID | New? | Category | Severity | Summary | Status |
|----|------|----------|----------|---------|--------|
| **AR-1** | confirms checkpoint #1 | Origin resolution | HIGH | **Local-origin lookup is split-brain.** `RepoRoot` is absolute (`git rev-parse --show-toplevel`) but `OriginDir` is a bare relative path (`my-org/my-skills`, from `originDirRef`), resolved against the **process CWD**. Running `add` from a **subdirectory** fails ("skill not found in origin") even though the origin checkout is correctly at the repo root — while the vendored files + lock would still target the repo root. Empirically confirmed. | Documented (checkpoint div #1); **CWD-relative resolution itself still unfixed** — hardening candidate noted in `contracts/add.md`. |
| **AR-2** | **NEW** | Errors-as-navigation (FR-019) | **HIGH** | **A missing origin checkout is indistinguishable from a typo'd skill name.** `add.go` (`os.Stat(srcDir)`, ~L85-90) returns `SkillNotFoundError` on *any* stat error, so when the entire `./OWNER/REPO` checkout is absent (user ran `init` but never cloned it), the user gets *"skill … not found in origin → check the skill name"* — actively misleading. The two failure classes (origin-dir-absent vs skill-subdir-absent) must be distinguished (cli.md Principle 2). | **Open.** |
| **AR-3** | **NEW** | Test tier (Constitution III) | MEDIUM | **No `pkg/skillcore/verify_test.go`.** The headline gate's logic — status taxonomy (`ok/mismatch/orphan/missing/dirty`), counts, all-findings aggregation, dirty-before-mismatch precedence — is covered **only** by black-box integration tests, with no presentation-free unit test. The two-tier discipline is met for `add`/`treesha`/`lock`/`manifest` but not for `verify`. | **Open.** |
| **AR-4** | **NEW** (tied to AR-1/AR-2) | Skill accuracy | LOW | The new `skillrig-add-verify` skill understates the CWD-relative fragility ("relative to where you run add (your repo root)") and repeats the misleading *"not found → check the name"* error mapping without the "or the origin checkout is missing" case. | **Open** (fix with AR-1/AR-2). |
| **AR-5** | — | Doc drift | LOW | `data-model.md` sample tree-SHA `c967789…` is stale vs the actual fixture (`40e4cad…`). Documented as "representative, not canonical"; tests recompute via raw git, so **not** a correctness bug. | Optional cleanup. |

## Positives confirmed (independently verified)

- **AP-04 upheld:** the only `git` shelling outside `pkg/skillcore` is `gitToplevel` in `internal/cli/repo.go` (repo-root discovery, *not* tree-SHA). All tree-SHA / lock / manifest logic lives solely in `pkg/skillcore`. No parallel implementation.
- **Tree-SHA covers additions, not just byte edits:** an untracked stowaway file inside a locked skill → `dirty`; a committed stowaway → `mismatch`. Correct.
- **Verdict taxonomy is sound,** including `dirty`-before-`mismatch` precedence and working-tree-deletion → `dirty` (exit 2) — both judged defensible/by-design.
- **Round-trip verified live:** clean → exit 0; uncommitted tamper → `dirty` (2); committed tamper → `mismatch` (2); empty `.agents/skills` + lock entry → `missing` (2).
- **`make check` green**, `cli.md` correctly synced this branch (verify integrity-only, `pkg/skillcore` as separate public package, exit 3 reserved).

## Cleared (false alarms the agent self-corrected)

- **Wrong-`lockfileVersion` exit code:** initially looked like exit 0, but that was **pipe-masking** (`head` exit, not `skillrig`); true exit is **1**. No bug.
- **`lock_test.go` hard-coded SHAs:** used only as write→read serialization round-trip fixtures, not asserted against real git output — harmless (no circular oracle).

## Observation (not a finding)

Running `skillrig verify` inside the `skillrig-cli` repo itself reports all ~17 of its own vendored agent skills as `orphan` (it has no committed `.skillrig/skills-lock.json`) — expected behavior, but a reminder that this repo is not yet self-managed by `skillrig`.

## AR-P — Process incident (review harness)

The first review agent's manual round-trip used a `cd "$WORK"` that silently no-op'd (empty var), so `git add -A && git commit -m vendor` ran in the **repo root**, creating a stray commit; the agent then `git reset --soft` back to `e0d8ccd`. No file contents or real commits were lost, but the **staging area was disturbed**. **Mitigation adopted:** require a clean/committed working tree *before* launching a review agent (the agent may freely run git to test) — now documented in `.agents/commands/specledger.checkpoint-workflow.md`.

## Recommended actions before merge

1. **[HIGH] AR-2** — distinguish "origin checkout missing" from "skill not found" in `add` (a dedicated error + fix hint). Cheap, high-value.
2. **[HIGH] AR-1** — decide the CWD-vs-repo-root resolution: make `OriginDir` repo-root-relative (robust; tests still pass since root==CWD there), or document "run `add` from the repo root" prominently. Currently only a follow-up note.
3. **[MEDIUM] AR-3** — add `pkg/skillcore/verify_test.go` (stub the git client; table-drive the status taxonomy + counts + aggregation).
4. **[LOW] AR-4 / AR-5** — fold the AR-1/AR-2 nuance into the `skillrig-add-verify` skill; refresh the stale data-model sample SHA.
