# Implementation Plan: CLI Initialization & Origin Resolution

**Branch**: `001-init-origin-resolution` | **Date**: 2026-05-24 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specledger/001-init-origin-resolution/spec.md`

## Summary

Deliver the first slice of the generic `skillrig` CLI: a `skillrig init` command (Environment pattern) that records a chosen origin into project or global config, plus the **single** origin-resolution primitive (`config.ResolveOrigin`) every later command depends on. Resolution precedence is env `SKILLRIG_ORIGIN` > project `.skillrig/config.toml` > global `~/.config/skillrig/config.toml`. Scope is offline config bootstrap only — no network, no auth, no lockfile, no other verbs. This branch also establishes the Go/Cobra project skeleton and the baseline command experience (progressive help, errors-as-navigation, two-level output, load-bearing exit codes) that all subsequent commands inherit, per [docs/design/cli.md](../../docs/design/cli.md) and architecture §2/§2b/§2d.

## Technical Context

**Language/Version**: Go 1.24+ (toolchain in this environment is 1.24.4; 1.25 also fine) — single static binary; cross-OS/arch via goreleaser later, out of scope here
**Primary Dependencies**: `github.com/spf13/cobra` (command tree); `github.com/pelletier/go-toml/v2` (config read/write — see research.md). Dependencies kept minimal (consume-only, static binary).
**Storage**: Local files only — project `.skillrig/config.toml`, global `~/.config/skillrig/config.toml` (XDG-aware). No database, no network.
**Testing**: Go standard `go test`. Two tiers — (a) in-process Cobra unit tests via `SetArgs`/`SetOut`/`SetErr` + table-driven resolver tests; (b) `TestQuickstart_*` integration tests that build and exec the real binary (Constitution II/III).
**Target Platform**: macOS/Linux/Windows terminals, CI, and agent runners. Symlink/Windows concerns are not in this feature's scope.
**Project Type**: single project (CLI binary).
**Performance Goals**: Sub-100ms for `init` and resolution (fully offline; cli.md records no per-command duration metadata, so this is a soft target, not an SC).
**Constraints**: Offline-only; no auth/credentials; consume-only (no bootstrap of an origin); idempotent; deterministic. Exit codes for this feature: `0` success, `1` usage/config error. Codes `2` (verification) and `3` (prerequisite) are reserved for later commands.
**Scale/Scope**: Small — one command (`init`), one resolver primitive, the root command skeleton, and config read/write. ~Hundreds of LOC.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

Verify compliance with principles from `.specledger/memory/constitution.md` (v2.1.0):

- [x] **I. Specification-First**: spec.md complete, clarified (reviewer comments resolved), prioritized user stories P1–P3.
- [x] **II. Quickstart-as-Contract**: quickstart.md authored as executable scenarios mapping 1:1 to `TestQuickstart_<scenario>` integration tests; includes output-shape assertions (compact human line-count bound; `--json` parseable + structurally complete; error output asserts what/why/fix as distinct checks + exit code).
- [x] **III. Ground-Truth Anchoring**: data-model fixtures derived from a real captured `config.toml` and a recorded resolution-precedence matrix (golang-testing fixtures/table-driven/integration patterns). No network boundary here, so no httptest/go-vcr.
- [x] **IV. Agent-First CLI Design**: `init` classified as the **Environment** pattern (idempotent, consume-only); progressive `--help` with ≥2 examples; errors-as-navigation (what/why/fix, raw error preserved, stderr); two-level output (human compact default + footer hint, `--json` complete); standard flags `--json`/`--verbose`; load-bearing exit codes. The origin resolver is **one** implementation (AP-06).
- [x] **V. Code Quality (Go)**: `gofmt` + `go vet` + `golangci-lint` gate; idiomatic Go; execution logic (config/resolve) independent of presentation (cli output).
- [x] **VI. YAGNI**: origin-only; no `config` command; no extra metadata (repo tags/suggestions); codes 2/3 deferred.
- [x] **VII. Shortest Path to MVP**: minimum skeleton + `init` + resolver; nothing speculative.
- [x] **VIII. Simplicity Over Cleverness**: boring, obvious Go; plain struct + TOML marshal; no reflection tricks.
- [x] **IX. Skill–CLI Co-Evolution**: a corresponding agent skill (`skillrig-init` usage skill) is planned as a task; description keywords cover "point repo at our skills library / set origin / SKILLRIG_ORIGIN" + the no-origin failure mode.

**Complexity Violations**: None identified.

## Project Structure

### Documentation (this feature)

```text
specledger/001-init-origin-resolution/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output — executable integration-test scenarios (Constitution II)
├── contracts/           # Phase 1 output — CLI command + resolver contracts
│   ├── init.md          #   `skillrig init` command surface
│   └── resolve.md       #   ResolveOrigin precedence contract
└── tasks.md             # Phase 2 output (/specledger.tasks — not created here)
```

### Source Code (repository root)

Module path: `github.com/skillrig/cli`.

```text
.
├── main.go                      # package main; thin → internal/cli.Execute(); os.Exit(code)
├── go.mod / go.sum
├── .golangci.yml                # lint config (Constitution V)
├── internal/
│   ├── cli/
│   │   ├── root.go              # cobra root cmd; persistent --json/--verbose; help template
│   │   ├── init.go              # `skillrig init` (Environment pattern)
│   │   ├── exit.go              # exit-code constants: ExitOK=0, ExitUsage=1 (2/3 reserved, documented)
│   │   └── output.go            # presentation: human compact + footer hint vs --json (no business logic)
│   └── config/
│       ├── origin.go            # Origin type + ParseOrigin (OWNER/REPO validation)
│       ├── config.go            # ProjectConfig/GlobalConfig structs; paths; TOML load/save (atomic write)
│       └── resolve.go           # ResolveOrigin(cwd, env) — THE single resolver (AP-06)
└── test/
    └── quickstart_test.go       # TestQuickstart_* — build + exec the real binary (Constitution II)
```

**Structure Decision**: Single Go module rooted at the repo. Business logic lives in `internal/config` (origin parsing, config I/O, resolution); the CLI/presentation layer lives in `internal/cli` and must not leak output-format concerns into `internal/config` (Constitution V; cli.md Execution-vs-Presentation). `ResolveOrigin` is the sole resolver (AP-06) so every future command (`search`, `add`, `verify`, …) calls it rather than re-reading config. `main.go` is a thin shim that maps the returned error/exit code to `os.Exit`. The architecture's `skillcore` is **not** introduced in this feature (no tree-SHA/integrity work yet) — only `config`.

## Complexity Tracking

> No constitutional violations to justify. Table intentionally empty.
