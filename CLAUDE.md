# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> PRE-RELEASE MARKER: As long as this marker is present we NEVER PLAN BACKWARD COMPATBILITY. We are in rapid iteration and may make breaking changes to the CLI and/or skill contract at any time. ANY PLAN IGNORES BACKWARD COMPATIBILITY.

## What this is

`skillrig` is a single, generic, **consume-only** Go CLI for pointing a repo (or a per-user default) at an **origin** — the `OWNER/REPO[@REF]` that hosts an org's agent skills — and managing vendored skills from it. The same binary serves humans, agents, and CI. There is no `publish`/`login` and no write credential in the binary: GitHub is the authority plane ("publishing" = a PR to the origin).

Two design documents are binding and override general instincts:
- `.specledger/memory/constitution.md` — development principles (spec-first, quickstart-as-contract, YAGNI, skill–CLI co-evolution).
- `docs/design/cli.md` — the CLI design contract (progressive discovery, errors-as-navigation, two-level output, standard flags, exit codes, command-pattern classification). A CLI behavior change must update this file in the same branch.

`docs/ARCHITECTURE-v0.md` is the full architectural rationale and roadmap. Most of the command surface it describes (`search`, `add`, `verify`, `bump`, `doctor`, `lint`, `global`) is **designed but not yet implemented** — only `init` (+ the origin resolver) exists today. Don't assume a command exists because the architecture mentions it; check `internal/cli/root.go`'s `registerSubcommands`.

> NOTE: If new user stories diverge from docs/ARCHITECTURE-v0.md clarify with the User and propose to update the user stories or the architecture as needed. The architecture is a living document and should evolve with new insights.

## Build / test / lint

Common tasks go through the `Makefile` (requires Go 1.24+ and `git` on PATH):

```sh
make build              # go build -o skillrig .
make test               # full suite (unit + integration; no network)
make test-unit          # unit tests only        -> ./internal/...
make test-integration   # quickstart acceptance   -> ./test/... (builds & execs the binary)
make lint               # golangci-lint (v2 config in .golangci.yml)
make check              # pre-merge gate: fmt + vet + lint + test
```

The test tiers map to the package layout (constitution §III): presentation-free unit tests in `internal/...`, and the `TestQuickstart_*` acceptance suite in `test/` that builds the real binary and execs it. Run a single test directly, e.g. `go test ./internal/config -run TestParseOrigin`. `mise.toml` only provisions the `crit` review tool, not build tooling.

## Architecture: two layers, one hard rule

```
main.go            → thin shim: os.Exit(cli.Execute())
internal/cli/      → PRESENTATION + cobra wiring ONLY
internal/config/   → business logic, presentation-FREE
```

The separation is load-bearing, not stylistic:
- `internal/cli` parses flags, calls into `config`, and renders results/errors. It must **not** contain origin/config business logic.
- `internal/config` is the value types + I/O + the resolver. It must **not** format output for humans (no fmt.Println of user-facing text; it returns structured results and typed errors).

**Single origin resolver (`config.ResolveOrigin`).** Every command resolves the active origin through this one function — never re-read config or env per command (this is anti-pattern AP-06 in `cli.md`). Precedence, highest wins: `SKILLRIG_ORIGIN` env > nearest-ancestor project `.skillrig/config.toml` > global `~/.config/skillrig/config.toml`. A blank/whitespace env value is "unset"; a malformed/origin-less file is **skipped** (recorded as a `SourceDiagnostic` for `--verbose`), not fatal — resolution continues down the order. The one hard error is an explicitly-set-but-invalid `SKILLRIG_ORIGIN`.

**Future shared primitive (`skillcore`).** When integrity commands land, tree-SHA and manifest parsing must have exactly ONE implementation that `verify`/`bump`/`doctor` all dispatch to — never a parallel copy (AP-04). Same single-implementation discipline as the resolver.

## Exit codes are part of the contract

Scripts and agents branch on them, so meanings are fixed (`internal/cli/exit.go`):
`0` ok (incl. idempotent no-ops) · `1` usage/config · `2` verification failure (reserved) · `3` prerequisite failure (reserved). Return a `*cli.UsageError` (it carries an actionable what/why/fix `Msg` + a preserved raw `Cause` surfaced under `--verbose`) for the `1` class; `exitCodeFor` maps it. Errors go to **stderr**, data to **stdout**.

## Conventions specific to this repo

- **Errors as navigation.** Every error states what failed, the *real* (never-swallowed) cause, and a suggested fix. `--verbose` is the escape hatch that prints the raw underlying cause — it must exist on every command. See `cli.md` Principle 2 and anti-patterns AP-03.
- **Two-level output.** Human output is compact with a footer hint; `--json` is complete and untruncated. `--json`/`--verbose` are persistent root flags (`globalOpts`); mutating commands also take `--dry-run`, and `add`/`update` take `--force`. Tests must assert output *shape* (bounded line count for human, parseable + structurally complete for JSON), not just `Contains` (constitution §II).
- **Classify every new command** into a `cli.md` pattern (Query / Vendor Mutation / Verification Gate / Environment / Global Management) and run the `docs/design/checklist-template.md` gate before merge.
- **Skill–CLI co-evolution (constitution IX).** Every CLI change ships a matching skill update with verified trigger accuracy. The relevant skill lives in `.agents/skills/skillrig-init/`; eval tooling is `.agents/skills/skill-creator/scripts/run_eval.py` (note: the constitution's `scripts/run_eval.py` path is stale). Per global instructions, run skill evals with `model: "sonnet"`.

## Workflow & tracking

Features follow SpecLedger: **Specify → Clarify → Plan → Tasks → Review → Implement**, with artifacts under `specledger/<NNN-feature>/` (spec, plan, tasks, quickstart, contracts, data-model). Quickstart scenarios are the acceptance contract (each maps to a `TestQuickstart_<scenario>` integration test) and are written during planning.

**Read `AGENTS.md` before tracking work or committing.** It defines the two repo-specific operating rules this project enforces: (1) all work-item tracking goes through the built-in `sl issue` CLI (issues stored per-spec in `specledger/<spec>/issues.jsonl`) — **never** ad-hoc markdown TODO lists; and (2) the commit/PR conventions (conventional prefixes, imperative ≤72-char subjects, testing evidence in PRs). It exists so task tracking and history stay in one git-friendly system rather than fragmenting across tools — consult it for the exact commands and the precise scope of each rule.
