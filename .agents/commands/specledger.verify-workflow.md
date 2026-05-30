---
description: EXPERIMENTAL — cross-artifact verification WITHOUT tasks.md. Fans out N INDEPENDENT reviewers (spec → plan/research/data-model/contracts/quickstart) via a deterministic Workflow, then MERGES their findings into one report (independent passes catch complementary issues). Read-only. Run from a FRESH session at DEFAULT effort.
handoffs:
  - label: Implement (workflow)
    agent: specledger.implement-workflow
    prompt: Implement the verified feature via the workflow pipeline
---

## User Input

```text
$ARGUMENTS
```

Optional `$ARGUMENTS`: number of independent reviewers (default 2), or extra focus (e.g. "emphasize security", a feature id).

## Purpose

Read-only cross-artifact consistency verification for a feature whose **`tasks.md` was intentionally not generated** (e.g. when using `/specledger.implement-workflow`). It validates **`spec.md` against `plan.md`, `research.md`, `data-model.md`, `contracts/*.md`, and `quickstart.md`** — the planning artifacts — and produces a Specification Analysis Report.

**Why a workflow, not a single pass:** independent reviewers reliably catch *different* things. (Real example: one pass caught a committed-tree-vs-working-tree spec↔plan ambiguity; another caught a stale acceptance scenario the first missed.) So this runs **N independent reviewers in parallel** and **merges** them — keeping complementary findings, deduping overlap.

> **STRICTLY READ-ONLY.** Reviewers and the merge step report findings and recommend fixes; they do **not** edit artifacts. The only write is the optional, explicit save of the report (last step).
> **Do NOT flag the absence of `tasks.md`** — it is intentional for this flow. Skip the task↔requirement and `TestQuickstart_*`-task-mapping checks.

> **Pause for model and effort.** Subagents inherit the launcher's *model* (this script leaves `model` unset). Before launching, **AskUserQuestion which model** the reviewers should use and offer to pause to change effort because **effort is inherited** from the launching session — so a cheap, lower-effort session keeps the fan-out costs under control.

## What each reviewer checks (the four focus areas)

1. **Coverage** — every Functional Requirement (FR-*) and Success Criterion (SC-*) in `spec.md` is covered by the plan + a contract + a `TestQuickstart_*` scenario. Flag any requirement with no downstream coverage.
2. **Reverse traceability** — every contract behavior and every quickstart scenario traces back to a spec requirement (no invented behavior ungrounded in the spec).
3. **Consistency** — no contradictions across artifacts (exit codes, lock schema, fingerprint semantics, origin resolution, etc.); in particular flag any **AMBIGUITY that would let an implementer/model decide a behavior two different ways**.
4. **Decision integrity** — the spec's recorded clarification decisions are applied **consistently everywhere they appear**, with **no leftover stale wording**.

Constitution (`.specledger/memory/constitution.md`) is in scope: a MUST-principle conflict is automatically CRITICAL.

## Execution steps

1. **Locate artifacts**: run `sl spec info --json --paths-only`; read `FEATURE_DIR`. (The reviewers Read the artifacts themselves.)
2. **Discover relevant skills**: enumerate the skills available in the session (the available-skills list surfaced by the harness; or invoke `/find-skills` for a gap). **focus on design skills** — e.g. cobra, agentic CLI design, Supabase Architecture, REST and data modeling. Workflow subagents **do** have the `Skill` tool (verified empirically), so every review agent prompt **can and MUST** instruct the agent to load its relevant design and architecture skills via the `Skill` tool *before* reviewing artifacts. Record the review skills you'll bake into the prompts.
3. **AskUserQuestion**: Batch which `model` for the reviewers (note effort is inherited from this session) together with the relevant skills (multiple selections allowed). Pass the `model` (or leave `model` unset to inherit) and `skills` choices into the script .
4. **Author + launch the Workflow** below.
5. When it returns, **present the merged report** (format below). Then **offer to save** it (final step).

## Skill loading is mandatory (not optional)

> **Every review `agent()` prompt MUST begin with a `SKILLS:` line** naming the skills to invoke via the `Skill` tool and apply *before* reviewing. Design artifacts say *what* to build; the skills carry *how this repo designs it* — relying on the artifacts alone leaves that on the table. Workflow subagents have the `Skill` tool, so this review directly; do **not** distill skill content into the prompt by hand and do **not** assume an agent will load a skill unprompted.

## Workflow pipeline (author this script)

```
export const meta = {
  name: 'verify-artifacts',
  description: 'Cross-artifact verification (no tasks): N independent reviewers + merge',
  phases: [{ title: 'Review' }, { title: 'Merge' }],
}
const FD = args.featureDir
const N = args.reviewers || 2
const MODEL = args.model            // undefined → inherit launcher; or set from the AskUserQuestion answer

const FINDINGS = { type: 'object', required: ['findings'], properties: {
  findings: { type: 'array', items: { type: 'object',
    required: ['category','severity','location','summary','recommendation'],
    properties: {
      category: { type: 'string' },                    // Coverage|Traceability|Consistency|DecisionIntegrity|Constitution|Ambiguity
      severity: { enum: ['CRITICAL','HIGH','MEDIUM','LOW','INFO'] },
      location: { type: 'string' },                    // file:line(s) or section refs
      summary: { type: 'string' },
      recommendation: { type: 'string' } } } },
  coverageGaps: { type: 'array', items: { type: 'string' } },   // FR-*/SC-* with no downstream coverage
  staleWording: { type: 'array', items: { type: 'string' } } } }

// Phase 1 — N INDEPENDENT reviewers (parallel, fresh context each), schema'd findings.
phase('Review')
const ART = `${FD}/spec.md, ${FD}/plan.md, ${FD}/research.md, ${FD}/data-model.md, ${FD}/contracts/*.md, ${FD}/quickstart.md, and .specledger/memory/constitution.md`
const passes = (await parallel(Array.from({ length: N }, (_, i) => () =>
  agent(`SKILLS: invoke "agentic-go-cli-design" via the Skill tool and apply it (to judge the CLI contracts/help/errors/exit codes). You are an INDEPENDENT reviewer (pass ${i + 1} of ${N}) — do not assume other reviewers exist. READ-ONLY. Read ${ART}. tasks.md is intentionally ABSENT — do NOT flag it. Cross-verify spec → {plan,research,data-model,contracts,quickstart} on: (1) COVERAGE — every FR-* + SC-* covered by plan + a contract + a TestQuickstart_* scenario; (2) REVERSE TRACEABILITY — every contract/quickstart behavior traces to a spec requirement; (3) CONSISTENCY — contradictions + any AMBIGUITY that lets an implementer/model decide two ways; (4) DECISION INTEGRITY — clarification decisions applied consistently, no stale wording. Constitution MUST-conflict = CRITICAL. Return findings per the schema; cite file:line. Do not edit anything.`,
    { schema: FINDINGS, model: MODEL, phase: 'Review', label: `reviewer#${i + 1}` }))).filter(Boolean)

// Phase 2 — Merge: keep complementary findings, dedup overlap, reconcile severity, build the report.
phase('Merge')
const report = await agent(`Merge ${passes.length} INDEPENDENT review passes into ONE Specification Analysis Report. KEEP complementary findings (independent reviewers catch different things), DEDUP true overlaps (same location+claim), reconcile severities to the highest justified. Then emit the report EXACTLY in the format under "## Report format" of the calling command: (a) findings table with columns ID | Source(passes) | Category | Severity | Location | Summary | Recommendation; (b) Coverage summary (each FR-*/SC-* → covered? evidence / gap); (c) Decision-integrity checklist (each recorded decision → consistent? ✓ / stale-wording refs); (d) Metrics (total FR+SC, critical/high/medium counts, coverage gaps); (e) Next actions. READ-ONLY — recommend fixes, do not edit. Passes (schema'd): ${JSON.stringify(passes)}`,
  { model: MODEL, phase: 'Merge', label: 'merge' })
return { report, reviewers: passes.length }
```

Pass `args: { featureDir: "<FEATURE_DIR>", reviewers: <N>, model: "<choice-or-undefined>" }`.

## Report format (the merge agent emits this; you present it)

```
## Specification Analysis Report — <feature> (no-tasks cross-verify)

**Scope:** spec.md ↔ plan/research/data-model/contracts/quickstart. Tasks dimension intentionally skipped. <N> independent reviewers merged.

| ID | Source | Category | Severity | Location(s) | Summary | Recommendation |
|----|--------|----------|----------|-------------|---------|----------------|
| C1 | r1,r3  | Consistency | HIGH | spec FR-09 ↔ contracts/verify.md | … | … |

### Coverage summary
| Requirement | Plan | Contract | Quickstart test | Status |
(every FR-*/SC-* → Covered / Gap + evidence)

### Decision integrity
(each recorded clarification decision → applied consistently ✓ / stale-wording at <loc>)

### Metrics
- Requirements: N FR + N SC · Reviewers: N · Critical: N · High: N · Medium/Low/Info: N · Coverage gaps: N

### Next actions
- Resolve CRITICAL/HIGH before implementation; LOW/MEDIUM may proceed with noted improvements.
```

## Final step — offer to save (explicit, opt-in write)

After presenting the report, **AskUserQuestion**: save to `FEATURE_DIR/reviews/<spec-number>-review.md`? If yes, write the report with YAML frontmatter (`date`, `total_requirements`, `total_tasks: 0`, `coverage_pct`, `critical_issues`), creating `reviews/` if needed, and confirm the path. If a review already exists, offer to **merge into it** (mark resolved/open) rather than overwrite blindly.
