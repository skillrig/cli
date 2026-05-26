---
date: 2026-05-25
total_requirements: 20
total_tasks: 19
coverage_pct: 100%
critical_issues: 0
---

# Specification Analysis Report — 001-init-origin-resolution

**Feature:** CLI Initialization & Origin Resolution
**Command:** `/specledger.verify` (cross-artifact consistency & quality analysis)
**Artifacts reviewed:** spec.md, plan.md, tasks.md (issue store), quickstart.md, data-model.md, contracts/{init,resolve}.md, constitution v2.1.0

> This review reflects the artifacts **after** the agreed remediation was applied
> (FR-006c forced non-interactive mode, git-root write target made explicit, `git`
> declared a required dependency, requirement labels backfilled, "bind"→`init`
> naming reconciled). The findings below are recorded for traceability; the
> "Resolution" column states how each was closed.

## Findings

| ID | Category | Severity | Location(s) | Summary | Resolution |
|----|----------|----------|-------------|---------|------------|
| I1 | Inconsistency | MEDIUM | init task vs contracts/init.md, quickstart.md | `--non-interactive` flag existed in the init task but not in the contract/quickstart, and the intent (force fail-fast even on a TTY, not TTY auto-detect) was unstated. | **Resolved** — added FR-006c to spec, US3 acceptance scenario 4, `--non-interactive` to contract Synopsis/Flags/Behavior/Errors, and `TestQuickstart_NonInteractiveFlag` to quickstart. |
| I2 | Inconsistency / Underspec | MEDIUM | init task vs contracts/init.md, spec, plan | Init resolved the write target via `git rev-parse --show-toplevel` (git-root), but the contract said write to cwd and `git` was never declared a dependency. | **Resolved** — git-root write target documented in init.md/data-model.md; `git` declared a required (offline) dependency in plan.md; added `TestQuickstart_BindFromGitSubdir` + `BindNonGitCwdFallback` and a dedicated git-fixture task (SL-3b4985). |
| T1 | Coverage (traceability) | MEDIUM | all task issues | Only 6 of ~19 functional requirements carried a `requirement:` label; the rest relied on quickstart + task design. | **Resolved** — backfilled `requirement:` labels (FR-003/004 on resolver; FR-006/006a/006b/006c/007/008/009/013/016 on init; FR-013 on root; FR-015/016 on output helper; FR-017 on exit codes; FR-006a/006c on US3 tests). |
| D1 | Naming drift | LOW | spec.md "bind command" vs init | Spec uses the abstract verb "bind"; concrete command is `skillrig init`. | **Resolved** — added a clarification mapping "the bind command" → `skillrig init` and confirming no separate `config` command (config is hand-edited input). |
| G1 | Ground-Truth (Const. III) | LOW | data-model.md | `config.toml` fixture described as "real captured output" though no binary exists yet (format is trivially one `origin=` line). | **Accepted** — SL-60a982 regenerates the fixture from real `init` output once built; round-trip test enforces it. |
| C1 | Coverage gap | LOW | spec FR-011 | "MUST NOT scaffold/bootstrap an origin" has no positive/negative test. | **Accepted** — a "don't do X" constraint; init.md notes consume-only, no network/scaffold. |

## Coverage Summary (functional requirements → coverage)

100% of functional requirements (FR-001–017 + FR-006a/006b/006c = 20) have ≥1 task
or quickstart scenario. After remediation, label-based traceability covers the
core write/resolve/error requirements. Quickstart-as-Contract (Constitution II):
all user stories map to scenarios; every scenario traces to a user story; each is
executable; each has a backing test task (SL-db8e96, SL-ca8e55, SL-03ebb3,
SL-3b4985); story task SL-2e4214 carries quickstart-match + test-passing DoD.

## Constitution Alignment

No MUST violations. II ✅ (executable quickstart, output-shape asserts, no
story↔scenario drift), III ⚠️→accepted (G1), IV ✅ (Environment pattern,
progressive help, two-level output, exit codes), V/VI/VII/VIII ✅, IX ✅
(SL-0990e2 skill task with trigger evals).

## Metrics

- Total functional requirements: **20** (FR-001–017 + FR-006a/006b/006c)
- Coverage (≥1 task or scenario): **20/20 = 100%**
- Total leaf tasks: **19** across 6 phases (was 18; +SL-3b4985 git fixtures)
- Quickstart scenarios: **20** (added BindFromGitSubdir, BindNonGitCwdFallback, NonInteractiveFlag)
- Ambiguity count: **0**
- Duplication count: **0**
- **Critical issues: 0**

## Next Actions

- No CRITICAL/HIGH blockers — proceed to `/specledger.implement`.
- Build order unchanged: Setup → Foundational → US1 (incl. git fixtures SL-3b4985) → US2 → US3 → Polish.
- Ensure CI/dev environments have `git` on PATH (now a declared required dependency).
