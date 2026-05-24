<!--
  Sync Impact Report
  Version change: 1.1.0 → 1.2.0 (amendment)
  Amended principles:
    VI. Quickstart-Driven Validation — added design-principle shape assertion requirement
  Amended sections:
    - Development Workflow — no change
  Templates requiring updates:
    - .specledger/templates/plan-template.md ✅ aligned (Design-Principle Shape Assertions gate added)
    - .specledger/templates/spec-template.md ✅ aligned (acceptance scenario shape constraint guidance added)
    - .specledger/templates/tasks-template.md — no change required
  Follow-up TODOs:
    - Retrofit TestPlanShowDiff with line-count bound assertion (issue #9)
  Trigger: plan show --diff Two-Level Output violation — content-only E2E assertions passed while
    real-world plans with 50+ resources produced 200+ lines, violating docs/cli.md Principle 3.
    See .specledger/sessions/004-process-failure-output-changes.md (parallel to P1 TFOutputChange bug).
-->

# tfc-cli Constitution

## Core Principles

### I. YAGNI (You Aren't Gonna Need It)

Don't build abstractions, features, or configuration for hypothetical future
needs. Build what's needed now. If a requirement isn't on the current task
list, it doesn't exist yet. Three similar API calls are better than a
premature wrapper.

### II. Shortest Path to MVP

For every feature, find the minimum viable implementation. Ship, get
feedback, iterate. No gold-plating, no "while we're here" scope creep.
The right amount of complexity is the minimum needed for the current task.

### III. Simplicity Over Cleverness

Prefer boring, obvious Go code over clever solutions. If a reviewer needs
to pause to understand it, simplify it. Optimize for readability and
maintainability, not elegance.

### IV. Fail Fast, Fix Forward

Surface errors early with clear, actionable messages. Don't silently
swallow failures. Fix issues with new commits, not by rewriting history.
Errors MUST tell the user what went wrong and suggest what to do next.

### V. Contract-First Testing

TFC API response contracts are the source of truth for integration
boundaries. Contracts are snapshotted on disk and validated in tests.
Any API change MUST update the contract snapshot.

**Ground-truth anchoring:** Data models defined during planning
(data-model.md) MUST be validated against real upstream responses —
not solely API documentation or prior assumptions. At least one test
fixture per integration boundary MUST be derived from actual upstream
output (recorded, then scrubbed of secrets), not hand-crafted from
the spec. This breaks circular validation where a wrong spec produces
matching-but-wrong types, fixtures, and tests.

**Testing tiers:**
- **Unit:** Mock HTTP (`httptest`) + go-vcr cassettes for fast,
  deterministic tests
- **E2E:** Full CLI binary invocation validating real command output
  and exit codes

### VI. Quickstart-Driven Validation

The `quickstart.md` generated during the planning phase defines user
scenarios that map 1:1 to E2E test cases. Plans and task lists MUST
include a phase that translates:

  **Spec user stories/FRs -> quickstart.md scenarios -> Go E2E test functions**

If a quickstart scenario isn't covered by a test, it's a gap. If a test
doesn't trace back to a quickstart scenario, question whether it's needed.

**Design-principle shape assertions:** E2E tests for commands that produce output
MUST validate output *shape* (compliance with docs/cli.md Principles 1–3), not
only output *content* (presence of a resource address or field value). Content-only
checks (`strings.Contains(out, "addr")`) cannot detect verbose violations of
Two-Level Output.

Required shape assertions by output type:

- **Human output (compact):** Line count MUST be bounded relative to input size.
  Assert `len(lines) ≤ resourceCount + K` for a small constant K (≤ 5 covers
  summary line + footer hint). A fixture with N resources produces at most N + K
  lines — never N × attributeCount.
- **JSON output (`--json`):** Output MUST be parseable (`json.Unmarshal` succeeds)
  and structurally complete (key counts match expected schema fields). Assert on
  field presence, not truncation absence.
- **Error output:** MUST contain all three structural parts (Principle 2 — Error
  Messages as Navigation): what failed, why it failed, and a suggested fix. Assert
  each part as a distinct check, not as a single `strings.Contains(stderr, "something")`.

This is the output-shape parallel to ground-truth anchoring (Principle V): just as
content-only fixtures passed with wrong types, content-only output assertions pass
with wrong shape. Both circular validations must be broken.

### VII. Skill–CLI Co-Evolution

Every new CLI feature or command change MUST include a corresponding
skill update. The skill description keywords must reflect how users
actually phrase requests (including failure modes and adjacent tool
contexts like GitHub CI). Ensure plan and tasks exist to use the `skill-creator` at
`.agents/skills/skill-creator/` to test trigger accuracy and run
evals after changes. A feature isn't complete until its skill coverage
is verified.

## Architecture & CLI Design

The detailed CLI architecture, command patterns, and layer design rules
live in [`docs/design/README.md`](../../docs/design/README.md). The
design docs are authoritative for:

- **4-layer tooling model** (L0 Hooks, L1 CLI, L2 Commands, L3 Skills)
- **CLI design principles** (Progressive Discovery, Error Messages as
  Navigation, Two-Level Output)
- **Command pattern classification** (Data CRUD, Launcher, Hook Trigger,
  Environment, Template Management)
- **Cross-layer interaction rules**
- **Agent Owns Outcomes** — CLI provides tools; the agent makes decisions.
  CLI does NOT auto-apply, auto-interpret plans, or make assumptions
  about intent.

**This separation is intentional:** high-level development principles
live here in the constitution; architecture and CLI-specific rules live
in the design docs.

**Spec/plan validation:** All specifications and implementation plans
MUST be validated against the design docs in `docs/design/`. If a
feature changes CLI patterns, command classification, or cross-layer
interactions, the relevant design doc MUST be updated as part of the
same feature branch.

## Testing Strategy

The principles above (V, VI) govern the strategy. Implementation details
evolve and live with the test code.

**This separation is intentional:** principles are stable and live here;
implementation details evolve and live with the test code.

## Development Workflow

- Feature branches are short-lived and focused on a single spec
- Every feature goes through: Specify -> Clarify -> Plan -> Tasks ->
  Review -> Implement
- Quickstart scenarios are written during planning, before
  implementation begins
- E2E tests covering quickstart scenarios are part of the task list,
  not an afterthought
- Data models produced during planning MUST include at least one real
  upstream sample (e.g., a recorded API response or CLI output excerpt)
  as evidence. Checkpoint validation MUST verify spec-matches-contract,
  not only code-matches-spec
- New CLI commands MUST be classified into one of the 5 command patterns
  (see [`docs/design/cli.md`](../../docs/design/cli.md)) and task lists
  MUST include a verification task for pattern compliance
- Design docs in `docs/design/` MUST be kept in sync with implementation
  changes

## Agent Preferences

- **Preferred Agent**: Claude Code

## Governance

This constitution supersedes all other practices. Amendments require:
1. Documentation of the change and rationale
2. Review and approval
3. Update to this file and any affected artifacts (including design docs)

All PRs and reviews MUST verify compliance with these principles.
Complexity MUST be justified against Principles I, II, and III.

**Version**: 1.2.0 | **Ratified**: 2026-03-24 | **Last Amended**: 2026-03-27