# Report Template — Specification Analysis Report (cross-artifact verification, no-tasks)

> Used by `/specledger.verify-workflow`. The **merge** agent reads this file and emits a single filled copy as its output. Keep the headings and table columns **verbatim**; replace `<…>` placeholders. STRICTLY READ-ONLY — recommend fixes, edit no artifacts.

## Specification Analysis Report — <FEATURE-ID> (no-tasks cross-verify)

**Scope:** spec.md ↔ spec-tech/plan/research/data-model/contracts/quickstart. Tasks dimension intentionally skipped. <N> independent reviewers merged.

<!-- (a) FINDINGS TABLE. KEEP complementary findings from different passes; DEDUP true overlaps (same location+claim); reconcile each severity to the highest justified. Source = which passes raised it (e.g. r1,r2). Category ∈ Coverage|Traceability|Consistency|DecisionIntegrity|Constitution|Ambiguity. -->

| ID | Source(passes) | Category | Severity | Location(s) | Summary | Recommendation |
|----|----------------|----------|----------|-------------|---------|----------------|
| C1 | r1,r2 | Consistency | HIGH | spec.md:FR-0XX ↔ contracts/<file>:LN | <one-line> | <fix> |

### Coverage summary

<!-- (b) One row per FR-* and SC-* in spec.md. Status = Covered / Gap. Evidence cells name the plan step + contract + TestQuickstart_* that cover it (or "—" + Gap). -->

| Requirement | Plan | Contract | Quickstart test | Status |
|-------------|------|----------|-----------------|--------|
| FR-001 | <step> | <contract> | <TestQuickstart_*> | Covered / Gap |

### Decision integrity

<!-- (c) Each recorded clarification/spike decision (the spec's Clarifications session bullets + any spike resolutions Dn) → "applied consistently ✓" or "stale wording at <file:line>". -->

- <Decision> → ✓ / stale at <loc>

### Metrics

- Requirements: <N> FR + <N> SC · Reviewers: <N> · Critical: <N> · High: <N> · Medium/Low/Info: <N> · Coverage gaps: <N>

### Next actions

- Resolve CRITICAL/HIGH before implementation; LOW/MEDIUM may proceed with noted improvements.
