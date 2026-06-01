<!--
  Sync Impact Report
  Version change: 2.1.1 → 2.2.0 (amendment — 2026-06-01, resolves issue #10)
    Added principle:
      X. User-Facing Documentation Co-Evolution (new) — the root README MUST be updated
        in the same branch whenever the command surface or user-facing behavior changes;
        the README is the entry-point doc and a stale one is a defect (002/003 shipped
        new commands — add/verify/search/index — without updating it: the gap this closes).
    Amended section:
      - Development Workflow — added a bullet binding the root README sync to every CLI change.
    No other principle changes.
  ---
  Version change: 2.1.0 → 2.1.1 (terminology patch — 2026-05-31, resolves issue #7)
    - §III ground-truth: "skills/*/skill.toml walk" → "skills/*/SKILL.md frontmatter walk
      (skillrig index)"; mise `[[requires]]` → `metadata.x-skillrig.requires` (manifest moved
      to SKILL.md frontmatter in 003/S1).
    - §III testing tiers: the GitHub "httptest + go-vcr" Unit boundary → the pkg/skillcore git
      exec-stub seam (skillrig shells `git`; the fetch/integrity boundary is the git exec, not
      an HTTP API), matching 003's remote-fetch design.
    - §IX: corrected the eval tooling path to .agents/skills/skill-creator/scripts/run_eval.py.
    No principle changes.
  ---
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
- **mise** — actual `mise` resolution/version output for a backing CLI declared in its
  `metadata.x-skillrig.requires` (the `SKILL.md` frontmatter manifest, since 003/S1).
- **GitHub** — a real API/PR response on the `bump --pr` path, scrubbed of tokens.
- **index.json** — generated from a real `skills/*/SKILL.md` frontmatter walk (`skillrig index`), not authored by hand.

**Testing tiers:**
- **Unit:** mocked/recorded boundaries via the `pkg/skillcore` git **exec-stub seam**
  (`commandContext`) — skillrig shells `git`, so the fetch/integrity boundary is the
  `git` exec, not an HTTP API; **no `httptest`/go-vcr**. Remote fetch is exercised over a
  `file://` bare-repo substrate — for fast, deterministic, offline tests.
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
Use the `skill-creator` skill at `.agents/skills/skill-creator/` to test trigger
accuracy and run evals (`.agents/skills/skill-creator/scripts/run_eval.py`) after
changes. A feature is not complete until its skill coverage is verified.

**One consolidated skill, not one-per-command.** There is a SINGLE user-facing
agent skill for the whole CLI — `.agents/skills/skillrig/` — with a short root
`SKILL.md` (what it is, when to use it, the `init`→`add`→`verify` workflow, the
origin precondition) that routes to per-activity detail under `references/`
(one file per command/activity, e.g. `init.md`/`add.md`/`verify.md`). A new
command extends this skill (a new `references/<cmd>.md` + the root's routing table
and description keywords), it does NOT spawn a new top-level `skillrig-<cmd>`
skill. Progressive disclosure (skill-creator's domain-organization pattern) keeps
the root grokkable while the references carry depth; splitting per command
fragments triggering and duplicates the shared workflow/precondition guidance.

This is doubly load-bearing here: **undertriggering** — an agent failing to invoke
a skill when it should — is a documented failure mode that skillrig itself exists
to fight (it is the rationale behind the origin's `lint` conformance gate, cli.md /
architecture §2b). A skill that ships with a vague description is the same defect
skillrig polices in others.

### X. User-Facing Documentation Co-Evolution
The root **`README.md`** is the project's entry-point document — the first thing a
human or agent reads to learn what the CLI does and how to use it — so it MUST stay
in sync with the shipped behavior. Whenever a feature branch changes the **command
surface** (adds/removes/renames a command or flag) or any **user-facing behavior**
(exit-code meanings, origin-resolution rules, output contract, configuration file
shape), that branch MUST update `README.md` in the **same branch** as the code. A
README that documents only a subset of the implemented commands is a **defect**, not
a backlog item — the same standard this constitution applies to the design docs
(`cli.md`) and the consolidated skill (Principle IX). Reviews MUST treat an
out-of-date README as a blocking finding.

This principle is corrective: features 002 and 003 added `add`, `verify`, `search`,
and `index` while the README still described only `init` (issue #10). The remedy is
not a one-time cleanup but a standing obligation — every CLI change ships its README
update, just as it ships its skill update.

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
- The root `README.md` MUST be updated in the same branch whenever the command
  surface or user-facing behavior changes (Principle X) — a README that lags the
  implemented commands is a blocking review finding.
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

**Version**: 2.2.0 | **Ratified**: 2026-05-24 | **Last Amended**: 2026-06-01
