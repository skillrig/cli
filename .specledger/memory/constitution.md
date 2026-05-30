<!--
  Sync Impact Report
  Version change: 2.0.0 → 2.1.0 (amendment)
  Added principles:
    III. Ground-Truth Anchoring (new) — real-output fixtures per integration boundary + testing tiers
    IX. Skill–CLI Co-Evolution (new) — every CLI change ships a skill update + trigger-accuracy evals
  Amended principles:
    II. Quickstart-as-Contract — added Output-Shape Assertions (compliance with cli.md Principles 1–3, not content-only)
    V (old: Simplicity/YAGNI) split into three: VI. YAGNI / VII. Shortest Path to MVP / VIII. Simplicity Over Cleverness
  Renumbered (additive, nothing removed):
    III. Agent-First CLI Design → IV
    IV. Code Quality (Go)        → V
  Added sections:
    - Development Workflow (Specify→Clarify→Plan→Tasks→Review→Implement; design-doc + skill sync)
  Templates requiring updates (follow-up TODOs, not yet verified):
    - .specledger/templates/plan-template.md — add Output-Shape-Assertion + Ground-Truth gates
    - .specledger/templates/tasks-template.md — add pattern-compliance + skill-coverage DoD tasks
    - .specledger/templates/spec-template.md — acceptance-scenario shape-constraint guidance
  Source: learnings adopted from tfc-cli-constitution.md v1.2.0 (Principles V, VI, VII). The
    stale SpecLedger-isms in that source (4-layer L0–L3 model, 5 Data-CRUD/Launcher/Hook command
    patterns) were deliberately NOT adopted; skillrig's patterns live in docs/design/cli.md.
-->

# skillrig-cli Constitution

## Core Principles

### I. Specification-First
Every feature starts with a spec before code. The spec defines *what* and *why*;
the plan defines *how*. No production code is written ahead of an approved spec.

### II. Quickstart-as-Contract (Executable Acceptance)
`quickstart.md` is the acceptance contract, not documentation. Each quickstart
scenario maps 1:1 to a Go integration test (`TestQuickstart_<scenario>`), and a
feature is DONE only when every such test passes. Acceptance lives at the
integration layer; unit tests cover internal logic where genuinely useful.

This imposes obligations upstream and downstream of the test:
- **Authoring**: `quickstart.md` MUST be written so it can be executed as
  integration tests — concrete invocations, observable inputs/outputs, exit
  codes — not prose.
- **Verification** (`/specledger.verify`): MUST check that `quickstart.md` is
  consistent with the spec's user stories and with the plan/tasks derived from
  research. A quickstart that drifts from the user stories is a failure.
- **Definition of Done** (`/specledger.tasks`): task DoD MUST include verifying
  that quickstart scenarios match the user stories AND that the corresponding
  Go integration tests pass.

**Output-shape assertions.** Tests for commands that produce output MUST validate
output *shape* (compliance with [cli.md](../../docs/design/cli.md) Principles 1–3),
not only *content*. A content-only check (`strings.Contains(out, "skill-name")`)
cannot detect a Two-Level Output violation — output that is correct but far too
verbose. Required by output type:
- **Human output (compact):** line count MUST be bounded relative to input size —
  assert `len(lines) ≤ itemCount + K` for a small constant K (≤ 5 covers summary
  line + footer hint). A `search`/`verify` run over N skills produces at most N + K
  lines, never N × fieldCount.
- **JSON output (`--json`):** MUST be parseable (`json.Unmarshal` succeeds) AND
  structurally complete (key counts match the schema — e.g. a lock entry carries
  `version`/`commit`/`treeSha`). Assert field presence, not truncation
  absence.
- **Error output:** MUST contain all three Principle-2 parts (what failed, why,
  suggested fix) as *distinct* checks, plus the correct exit code (0/1/2/3 per
  cli.md) — never a single `strings.Contains(stderr, "...")`.

### III. Ground-Truth Anchoring
Data models and fixtures MUST be anchored to real output from each integration
boundary, not hand-crafted from the spec. At least one fixture per boundary MUST
be derived from actual recorded output (then scrubbed of tokens/secrets). This
breaks the circular validation where a wrong spec yields matching-but-wrong types,
fixtures, AND tests that all agree with each other and with nothing real.

skillrig's integration boundaries and their ground-truth sources:
- **git origin** — a real skill subtree's git tree SHA and `git ls-tree` output for
  a known commit (the label-honesty primitive, cli.md §4.2).
- **mise** — actual `mise` resolution/version output for a backing CLI in `[[requires]]`.
- **GitHub** — a real API/PR response on the `bump --pr` path, scrubbed of tokens.
- **index.json** — generated from a real `skills/*/skill.toml` walk, not authored by hand.

**Testing tiers:**
- **Unit:** mocked/recorded boundaries — `httptest` + go-vcr cassettes for the
  GitHub path — for fast, deterministic tests.
- **E2E:** full `skillrig` binary invocation against a fixture origin, validating
  real output and exit codes. E2E tests carry the §II output-shape assertions.

Checkpoint validation MUST verify spec-matches-ground-truth, not only
code-matches-spec.

### IV. Agent-First CLI Design
The `skillrig` binary is the entry point for humans, agents, and CI alike, and
must be navigable from `--help` alone (one-shot success without external docs).
The binding design contract lives in [docs/design/cli.md](../../docs/design/cli.md)
(git-tracked) — this principle references it rather than restating it. Its
non-negotiable concepts:
- **Progressive discovery** — every command/subcommand has complete help with ≥2 examples.
- **Errors as navigation** — every error states what failed, the *real* reason
  (never swallowed), and a suggested fix; errors to stderr, data to stdout.
- **Two-level output** — compact human output with footer hints; complete,
  untruncated `--json` for pipes. Standard flags (`--json`, `--verbose`, and
  `--dry-run`/`--force` on mutating commands) mean the same thing everywhere.
- **Load-bearing exit codes** — `verify`/`lint` are deterministic offline CI gates.
- **One generic binary, consume-only** — origin resolved at runtime, never baked
  in; no write credential in the binary.
- **Single implementation of integrity primitives** — `skillcore` is the one
  source of tree-SHA/manifest logic; `verify`, `bump`, `doctor` all dispatch to it.

### V. Code Quality (Go)
Built in Go with the standard `go test` framework. Code MUST pass `gofmt`,
`go vet`, and the project linter before merge. Follow idiomatic Go; execution
logic must not depend on output format (presentation vs. execution layers stay
separable within each command).

### VI. YAGNI (You Aren't Gonna Need It)
Don't build abstractions, features, or configuration for hypothetical future
needs. Build what's needed now. If a requirement isn't on the current task list,
it doesn't exist yet. Three similar lines beat a speculative abstraction.

### VII. Shortest Path to MVP
For every feature, find the minimum viable implementation. Ship, get feedback,
iterate. No gold-plating, no "while we're here" scope creep. The right amount of
complexity is the minimum needed for the current task.

### VIII. Simplicity Over Cleverness
Prefer boring, obvious Go code over clever solutions. If a reviewer needs to pause
to understand it, simplify it. Optimize for readability and maintainability, not
elegance.

### IX. Skill–CLI Co-Evolution
Every new CLI feature or command change MUST ship with a corresponding skill
update. The skill's description keywords MUST reflect how users actually phrase
requests — including failure modes and adjacent contexts (CI, `gh`/mise auth).
Use the `skill-creator` skill at `.claude/skills/skill-creator/` to test trigger
accuracy and run evals (`scripts/run_eval.py`) after changes. A feature is not
complete until its skill coverage is verified.

This is doubly load-bearing here: **undertriggering** — an agent failing to invoke
a skill when it should — is a documented failure mode that skillrig itself exists
to fight (it is the rationale behind the origin's `lint` conformance gate, cli.md /
architecture §2b). A skill that ships with a vague description is the same defect
skillrig polices in others.

## Architecture & CLI Design

The detailed CLI architecture and command-pattern rules live in the git-tracked
design docs under [`docs/design/`](../../docs/design/), which are authoritative for:

- **CLI design principles** (Progressive Discovery, Errors as Navigation,
  Two-Level Output, Standard Flags, Exit Codes) — [`cli.md`](../../docs/design/cli.md)
- **Command pattern classification** (Query, Vendor Mutation, Verification Gate,
  Environment, Global Management) and the per-command
  [pattern-gate checklist](../../docs/design/checklist-template.md)
- **Agent owns outcomes** — the CLI provides tools; the agent makes decisions. The
  CLI does NOT auto-apply, auto-interpret, or assume intent (`bump` proposes a PR,
  never force-adopts).

**This separation is intentional:** high-level development principles live here in
the constitution; architecture and CLI-specific rules live in the design docs.

**Spec/plan validation:** All specifications and implementation plans MUST be
validated against the design docs in `docs/design/`. If a feature changes CLI
patterns, command classification, standard-flag behavior, or integrity primitives,
the relevant design doc MUST be updated in the same feature branch.

## Testing Strategy

Principles II (Quickstart-as-Contract + output-shape assertions) and III
(Ground-Truth Anchoring) govern the strategy. Implementation details evolve and
live with the test code.

**This separation is intentional:** principles are stable and live here;
implementation details evolve and live with the test code.

## Development Workflow

- Feature branches are short-lived and focused on a single spec.
- Every feature goes through: **Specify → Clarify → Plan → Tasks → Review → Implement**.
- Quickstart scenarios are written during planning, before implementation begins.
- Integration/E2E tests covering quickstart scenarios are part of the task list,
  not an afterthought, and carry the §II output-shape assertions.
- Data models produced during planning MUST include at least one real boundary
  sample as evidence (Principle III, e.g. a recorded `git ls-tree`, mise output,
  or scrubbed GitHub response). Checkpoint validation MUST verify
  spec-matches-ground-truth, not only code-matches-spec.
- New CLI commands MUST be classified into one of the
  [cli.md](../../docs/design/cli.md) patterns (Query / Vendor Mutation /
  Verification Gate / Environment / Global Management), and task lists MUST include
  a pattern-compliance verification task using the
  [pattern-gate checklist](../../docs/design/checklist-template.md).
- Design docs in `docs/design/` MUST be kept in sync with implementation changes;
  CLI behavior changes update `cli.md` in the same branch.
- Every CLI change ships a skill update with verified trigger coverage (Principle IX).

## Agent Preferences

- **Preferred Agent**: Claude Code

## Selected Agents

- Claude Code

## Governance

Constitution supersedes all other practices. Amendments require:
1. Documentation of the change and rationale (recorded in the Sync Impact Report header).
2. Review and team approval.
3. Update to this file and any affected artifacts (including design docs and templates).

All PRs and reviews MUST verify compliance with these principles. Complexity MUST
be justified against Principles VI, VII, and VIII. The binding CLI design contract
is [docs/design/cli.md](../../docs/design/cli.md); changes to CLI behavior must
remain consistent with it.

**Version**: 2.1.0 | **Ratified**: 2026-05-24 | **Last Amended**: 2026-05-24
