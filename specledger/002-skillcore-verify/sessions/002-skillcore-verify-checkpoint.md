# Session Log: 002-skillcore-verify

## Divergence Review: 2026-05-30 12:09

Scope: **staged** changes only (the user staged the implement-workflow output; unstaged files are excluded). Implementation produced by `/specledger.implement-workflow` (multi-agent Workflow). Reviewer mindset: adversarial.

### Divergences

| # | Severity | Type | Category | Artifact | Description |
|---|----------|------|----------|----------|-------------|
| 1 | HIGH | oversight | Local-origin resolution undocumented & awkward | spec.md US1 / FR-001 / FR-007 + clarification 2026-05-30 Q1; `internal/cli/add.go:originDirRef` | `skillrig init --origin <local-path>` is **rejected** — 001's init validates `OWNER/REPO[@REF]`. `add` resolves the configured `OWNER/REPO` to a **same-named relative directory** `./<owner>/<repo>` nested in the consumer repo. The spec's "consume from a local checkout" *durable capability* is only reachable via this nested-dir convention, which **no contract, help text, or data-model documents**. Integration tests pass only because `newConsumerRepo` constructs that exact layout. |
| 2 | HIGH | conscious | Constitution IX skill co-evolution not delivered | spec.md Constitution Alignment §IX; CLAUDE.md ("Every CLI change ships a matching skill update with verified trigger accuracy") | No agent-skill update teaching `add`/`verify` usage (exit 0/2 meaning; missing backing tool ≠ verify failure) and no trigger-accuracy eval. The experimental workflow deliberately skipped it (documented in the command's "What this deliberately skips"). |
| 3 | LOW | oversight | Test gap — US3 AS3 | spec.md US3 AS3 / FR-011 | "orphan/completeness scans only the canonical `.agents/skills`" has no dedicated `TestQuickstart_` asserting a manually-created view dir is ignored. Behavior is implicit in `enumerateOnDiskSkills` (only `.agents/skills/*` with a marker file). |
| 4 | LOW | conscious | Commit hygiene — unrelated files staged | git index | `AGENTS.md` (deletion), `CLAUDE.md` (mod), and `.agents/commands/specledger.implement-workflow.md` are staged alongside the 002 feature; they are not part of the feature and would pollute a `feat(002)` commit. |

**Zero divergences found in the core integrity logic** — a positive signal worth stating: `pkg/skillcore/verify.go` faithfully implements FR-008/009/010/012/013/015 (lockfileVersion guard → `*LockError`/exit 1; dirty via `git status --porcelain` distinct from mismatch; orphan/missing; aggregates all findings; read-only rev-parse/status only; deterministic sort). Data-model entities (Manifest, LockFile/LockEntry without `requires`, AddResult, Report/Counts/Verdict, 5 statuses) match. `docs/design/cli.md` was synced this branch (same-branch doc-sync rule satisfied).

### DoD Bypassed

| User Story | Title | Acceptance Criteria | Risk |
|------------|-------|---------------------|------|
| US1 | Vendor a skill | All 5 AS tested (vendor/idempotent/json/refuse-divergent/dry-run). Underlying *local-origin capability* awkward & undocumented (div #1) | HIGH — capability shipped but undiscoverable per spec intent |
| US2 | Prove unmodified | All 4 AS tested + dirty + read-only | none |
| US3 | Orphan/missing | AS1/AS2/AS4 tested; **AS3 (view dirs ignored) not explicitly tested** (div #3) | LOW |
| US4 | Scriptable outcome | All 4 AS tested (exit matrix, json-complete, what/why/fix, malformed-lock) | none |
| Constitution §IX | Agent-skill co-evolution | **Not delivered** (div #2) | HIGH — hard project rule (CLAUDE.md) |

### Issues Encountered & Resolutions
- Live hand-transcript of `init --origin <abs-local-path>` failed ("expected OWNER/REPO[@REF]") → root-caused to `originDirRef` mapping `OWNER/REPO` → `./owner/repo`; re-ran the round-trip with the nested layout and it passed (exit 0 vendor→commit→verify; exit 2 on tamper). Surfaced as divergence #1.
- Transient LSP "undefined" diagnostics during the run → confirmed stale by a clean `go build ./...`; not real.

### Items Requiring Action Before Merge
1. [HIGH] **Decide & document local-origin resolution** (div #1): either (a) extend `init`/origin to accept a local filesystem path, or (b) document the `./<owner>/<repo>` nested-checkout convention in `contracts/add.md` + `add --help`, so the spec's "durable local-origin capability" is actually usable/discoverable. Currently neither.
2. [HIGH] **Ship the `add`/`verify` agent-skill update + trigger-accuracy eval** (Constitution §IX / CLAUDE.md hard rule) — div #2.
3. [LOW] Add a `TestQuickstart_VerifyIgnoresViewDirs` (or note the deferral) for US3 AS3 — div #3.
4. [LOW] Unstage `AGENTS.md`, `CLAUDE.md`, `.agents/commands/specledger.implement-workflow.md` before the `feat(002)` commit — div #4.

### Tests & Checks
- Status: **PASS**
- Commands run: `make check` (gofmt, go vet, golangci-lint → **0 issues**, `go test ./...` → all ok); plus an out-of-band live `init→add→commit→verify→tamper→verify` round-trip transcript.
- Failures: none.
- Coverage: all **20** quickstart scenarios present as `TestQuickstart_*` and passing; `skillcore` unit suite (ground-truth `TreeSHA == git rev-parse`, lock round-trip, manifest parse, stubbed-git error paths).

### Uncommitted Changes (excluded from this checkpoint, not staged/reverted)
- `.agents/commands/specledger.checkpoint.md` (M), `.agents/commands/specledger.implement-workflow.md` (working-tree edit over staged add), `.specledger/templates/tasks-template.md` (M), `docs/guides/vcr-cassettes.md` (untracked)

---

## Resolutions: 2026-05-30 12:25 (follow-up, same session)

Acted on the divergences above per user direction:

- **Div #1 (HIGH) — RESOLVED (documented).** Local-origin resolution (`OWNER/REPO` → `./OWNER/REPO` relative to the invocation dir) is now documented in three places: `skillrig add --help` (`internal/cli/add.go` Long + Example), `contracts/add.md` (new "Local-origin resolution (this slice)" note, incl. the CWD-relative caveat + repo-root-relative hardening follow-up), and the `skillrig-init` skill (new "Local origin (this release)" section with a worked setup). *Chosen path: document the current behavior, not re-architect.*
- **Div #2 (HIGH) — RESOLVED (skill authored; eval not run).** New agent skill `.agents/skills/skillrig-add-verify/` (Constitution §IX) teaching the vendor→commit→verify round-trip, exit-code branching, and that a missing backing tool is NOT a verify failure. Eval sets **defined but not run** per user: `evals/evals.json` (5 behavioral cases) + `evals/trigger-eval-set.json` (20 trigger queries). Running `run_eval.py` / trigger-optimization is a deliberate later step.
- **Div #3 (LOW) — RESOLVED.** Added `TestQuickstart_VerifyIgnoresViewDirs` (+ `writeClientViewSkill` helper) asserting the orphan scan ignores a non-canonical `.claude/skills/<name>` view dir (FR-011 / US3 AS3). Passing.
- **Div #4 (LOW) — DEFERRED by user** ("ignore the git index diff"). Commit hygiene left to the user.

**Re-check:** `make check` green (gofmt, go vet, golangci-lint 0 issues, `go test ./...` all ok incl. the new test). Next: relaunch the independent cold adversarial review agent.

---
