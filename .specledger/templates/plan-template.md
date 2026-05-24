# Implementation Plan: [FEATURE]

**Branch**: `[###-feature-name]` | **Date**: [DATE] | **Spec**: [link]
**Input**: Feature specification from `/specledger/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/specledger.plan` command. See `.specledger/templates/commands/plan.md` for the execution workflow.

## Summary

[Extract from feature spec: primary requirement + technical approach from research]

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: [e.g., Python 3.11, Swift 5.9, Rust 1.75 or NEEDS CLARIFICATION]  
**Primary Dependencies**: [e.g., FastAPI, UIKit, LLVM or NEEDS CLARIFICATION]  
**Storage**: [if applicable, e.g., PostgreSQL, CoreData, files or N/A]  
**Testing**: Go `go test` (standard) — quickstart.md scenarios become `TestQuickstart_<scenario>` integration tests with output-shape assertions (Constitution II); unit tests via `httptest`/go-vcr cassettes; ≥1 real recorded fixture per integration boundary, secret-scrubbed (Constitution III)  
**Target Platform**: [e.g., Linux server, iOS 15+, WASM or NEEDS CLARIFICATION]
**Project Type**: [single/web/mobile - determines source structure]  
**Performance Goals**: [domain-specific, e.g., 1000 req/s, 10k lines/sec, 60 fps or NEEDS CLARIFICATION]  
**Constraints**: [domain-specific, e.g., <200ms p95, <100MB memory, offline-capable or NEEDS CLARIFICATION]  
**Scale/Scope**: [domain-specific, e.g., 10k users, 1M LOC, 50 screens or NEEDS CLARIFICATION]

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Verify compliance with principles from `.specledger/memory/constitution.md` (v2.1.0, nine principles):

- [ ] **I. Specification-First**: Spec.md complete with prioritized user stories before planning
- [ ] **II. Quickstart-as-Contract**: quickstart.md scenarios authored as executable steps, each mapping 1:1 to a Go integration test (`TestQuickstart_<scenario>`); acceptance lives at the integration layer. **Output-shape assertions planned** — line-bound for compact human output, parse + structurally-complete for `--json`, 3-part (what/why/fix) + exit-code for errors (never content-only `strings.Contains`)
- [ ] **III. Ground-Truth Anchoring**: data-model.md cites ≥1 *real recorded* sample per integration boundary (git `ls-tree`/tree-SHA, `mise` resolve, GitHub `bump --pr` response, generated `index.json`), secret-scrubbed; unit tests via `httptest`/go-vcr cassettes, E2E via the real binary
- [ ] **IV. Agent-First CLI Design**: New/changed commands conform to [docs/design/cli.md](../../docs/design/cli.md) (progressive `--help`, errors-as-navigation, two-level output, standard flags, consume-only, single `skillcore` for integrity primitives) and are classified against a cli.md command pattern via the [pattern-gate checklist](../../docs/design/checklist-template.md)
- [ ] **V. Code Quality (Go)**: `go test` is the test framework; `gofmt`/`go vet`/lint pass; execution logic independent of output format
- [ ] **VI–VIII. Simplicity (YAGNI / Shortest Path to MVP / Simplicity Over Cleverness)**: no premature abstraction; minimum viable implementation; boring obvious Go over clever code; dependencies/indirection justified by a concrete requirement
- [ ] **IX. Skill–CLI Co-Evolution**: new/changed commands have a planned skill update (keywords matching real user phrasing + failure modes) and a trigger-eval task using `.claude/skills/skill-creator/` (`scripts/run_eval.py`)

**Complexity Violations** (if any, justify against Principles VI–VIII in the Complexity Tracking table below):
- None identified / [List violations and justifications]

## Project Structure

### Documentation (this feature)

```text
specledger/[###-feature]/
├── plan.md              # This file (/specledger.plan command output)
├── research.md          # Phase 0 output (/specledger.plan command)
├── data-model.md        # Phase 1 output (/specledger.plan command)
├── quickstart.md        # Phase 1 output — executable integration-test scenarios (Constitution II); each maps 1:1 to a Go TestQuickstart_<scenario>
├── contracts/           # Phase 1 output (/specledger.plan command)
└── tasks.md             # Phase 2 output (/specledger.tasks command - NOT created by /specledger.plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Expand the Go layout below with the concrete packages this
  feature touches. skillrig is a single Go module; the generic CLI's own
  internals live here (the org ORIGIN's skills + backing CLIs live elsewhere —
  see architecture.md). Add/rename packages as the feature requires; do not
  introduce a non-Go layout (Constitution V).
-->

```text
cmd/
└── skillrig/                 # main package — Cobra root + subcommand wiring

internal/
├── skillcore/                # THE single source of tree-SHA + manifest logic
│                             #   (verify, bump, doctor all dispatch here — cli.md AP-04)
├── config/                   # origin resolution: env > project > global (one resolver — AP-06)
├── lock/                     # skills-lock.json read/write
├── index/                    # index.json walk + compare (bump)
├── client/                   # client registry: client → path(s) → link-or-copy (multi-client)
└── <feature-pkg>/            # this feature's package(s)

testdata/                     # real recorded fixtures per boundary, secret-scrubbed (Constitution III)
│                             #   e.g. git ls-tree output, mise resolve, GitHub bump-PR response
└── cassettes/                # go-vcr cassettes for the GitHub path (unit tier)

# Tests live beside the code as Go convention dictates (pkg/foo_test.go).
# Quickstart-as-Contract integration/E2E tests: TestQuickstart_<scenario> invoking the real binary.
```

**Structure Decision**: [Document the selected packages and reference the real
directories captured above]

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
