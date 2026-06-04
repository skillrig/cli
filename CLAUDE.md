# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> PRE-RELEASE MARKER: As long as this marker is present we NEVER PLAN BACKWARD COMPATBILITY. We are in rapid iteration and may make breaking changes to the CLI and/or skill contract at any time. ANY PLANNING and DESIGN EFFORTS MUST IGNORE BACKWARD COMPATIBILITY.

## What this is

`skillrig` is a single, generic, **consume-only** Go CLI for pointing a repo (or a per-user default) at an **origin** — the `OWNER/REPO[@REF]` that hosts an org's agent skills — and managing vendored skills from it. The same binary serves humans, agents, and CI. There is no `publish`/`login` and no write credential in the binary: GitHub is the authority plane ("publishing" = a PR to the origin).

> **DEPRECATED — the sibling `skill.toml` manifest.** As of **003 (spike S1)**, a skill's machine metadata lives in its **`SKILL.md` YAML frontmatter** following the [agentskills.io](https://agentskills.io) standard — standard keys (`name`, `description`, `license`, …) at top level, and skillrig-specific data (`version`, `namespace`, `convention-version`, `topics`, `requires`) under the standard's free-form `metadata` map, namespaced as **`metadata.x-skillrig.*`** (parsed with `gopkg.in/yaml.v3`). The old `skill.toml` sibling file is **removed**; do not reintroduce it. Likewise the historical `[[requires]]` TOML notation in `docs/ARCHITECTURE-v0.md` now means **`metadata.x-skillrig.requires`** (a YAML list) in the frontmatter. `go-toml/v2` is retained ONLY for `.skillrig/config.toml`/`.skillrig-origin.toml`, never for skill manifests.

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
make test-e2e           # TRUE-AUTH e2e (-tags e2e) -> ./test/e2e (real git http-backend + token gate)
make lint               # golangci-lint (v2 config in .golangci.yml)
make check              # pre-merge gate: fmt + vet + lint + test
```

The test tiers map to the package layout (constitution §III): presentation-free unit tests in `internal/...`, and the `TestQuickstart_*` acceptance suite in `test/` that builds the real binary and execs it. Run a single test directly, e.g. `go test ./internal/config -run TestParseOrigin`. `mise.toml` only provisions the `crit` review tool, not build tooling.

> **True-auth e2e is the loop-closing check (`make test-e2e`).** The `test/e2e` suite (build tag `e2e`, **excluded from `make check` and CI** by design) stands up a real `git http-backend` behind a real token gate and drives the real binary at it over the HTTPS `http.extraHeader` path. Unlike the stub tiers (fake git / `file://`), it actually validates the credential — a missing/wrong token gets a real 401. **Any change touching the fetch, token-resolution, or git-transport path is not "done" until `make test-e2e` is green.** It needs `git` (with `git-http-backend`) on PATH; no network, no Docker. See `test/e2e/doc.go`. The supported, tested origin transport is **HTTPS + a read-only token** (`gh auth` done, or `GH_TOKEN`/`GITHUB_TOKEN`); SSH origins are a roadmap item, not implemented or tested.

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
- **Skill–CLI co-evolution (constitution IX).** Every CLI change ships a matching skill update with verified trigger accuracy. There is **one consolidated skill** for the whole CLI at `.agents/skills/skillrig/` — a short root `SKILL.md` that routes to per-activity detail in `references/` (`init.md`/`add.md`/`verify.md`). A new command **extends** this skill (add a `references/<cmd>.md` + update the root's routing table + description keywords); do **not** create a new top-level `skillrig-<cmd>` skill. Eval tooling is `.agents/skills/skill-creator/scripts/run_eval.py` (note: the constitution's `scripts/run_eval.py` path is stale). Per global instructions, run skill evals with `model: "sonnet"`.

## Workflow & tracking

Features follow SpecLedger: **Specify → Clarify → Plan → Tasks → Review → Implement**, with artifacts under `specledger/<NNN-feature>/` (spec, plan, tasks, quickstart, contracts, data-model). Quickstart scenarios are the acceptance contract (each maps to a `TestQuickstart_<scenario>` integration test) and are written during planning.

**Commit & PR conventions.** Conventional prefixes (`feat:`, `fix:`, `chore:`, `docs:`), imperative subjects ≤72 chars, scoped to the feature (e.g. `docs(002): …`). Reference related issues in the body; call out migrations / new binaries explicitly. PRs carry a concise summary + testing evidence (`make test-unit`, `make test-integration`) and a CLI transcript when behavior changes.

**PR titles are load-bearing.** The repo is **squash-merge only**, and the squash commit subject is the **PR title** (GitHub setting `squash_merge_commit_title=PR_TITLE`). `release-please` derives the version bump + changelog from that subject, so **every PR title must be a Conventional Commit** — enforced by the `pr-title` workflow (`.github/workflows/pr-title.yml`). Only `fix:` (→ patch), `feat:` (→ minor), and `!`/`BREAKING CHANGE` (→ major while ≥1.0.0) cut a release; `docs:`/`chore:`/`ci:`/`refactor:`/`test:`/`build:` land without a release. Don't dress a non-functional change as `fix:`/`feat:` to force a release.

**Work-item tracking.** The durable, team-visible record lives in the SpecLedger issue tracker — `sl issue`, stored per-spec in `specledger/<spec>/issues.jsonl` (committed to git). The agent's in-session task list (the `Task*` tools) is an ephemeral execution aid, not a substitute for that committed record.

<!-- >>> specledger-generated -->
<!-- Auto-managed by specledger - do not edit this section -->
## Active Technologies

- Go 1.24+ (toolchain 1.24.4) — single static binary (unchanged).
- `github.com/spf13/cobra` (commands); `github.com/pelletier/go-toml/v2` (config + retained for `.skillrig/config.toml`); **NEW: `gopkg.in/yaml.v3`** (SKILL.md frontmatter — accepted 2026-05-31
- `go test`, two tiers — (a) presentation-free **unit** in `internal/...` + `pkg/skillcore` (table-driven + ground-truth: fetched tree-SHA == raw `git`; `index` output == committed `index.json`); (b) **`TestQuickstart_*` integration** in `test/` building/exec'ing the real binary. **New network boundary** tested via S4's substrate: `file://` + local bare repo for happy/integrity; the existing `pkg/skillcore/git.go` `commandContext` exec-stub seam (extended to `Clone`/`FetchSparse`) for auth/unreachable/transient. **No `httptest`/go-vcr** (skillrig shells `git`, never calls the GitHub HTTP API — see Constitution Check).
- local files only — vendored subtree `.agents/skills/<skill>/`, committed lock `.skillrig/skills-lock.json`; origin-side `index.json` (committed in the origin). No DB. **No tool-managed cache** (catalog fetched per `search`).
- the parser `gh` uses; see Complexity Tracking). Lock uses stdlib `encoding/json`. Fetch + tree-SHA via **shelling `git`** (no in-process git/hashing lib). Token via `os.exec` of `git`/`gh` (no `gh`-as-library).
<!-- <<< specledger-generated -->
