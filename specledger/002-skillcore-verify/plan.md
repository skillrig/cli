# Implementation Plan: Vendor & Verify Skills (`add` + `verify`)

**Branch**: `002-skillcore-verify` | **Date**: 2026-05-30 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specledger/002-skillcore-verify/spec.md` · Technical companion: [spec-tech-spike.md](./spec-tech-spike.md)

## Summary

Deliver the second slice of `skillrig`: the shared integrity primitive **`skillcore`** (a public Go SDK package at `pkg/skillcore`) plus two consumer commands that make the product promise — *"the skill your agent runs is exactly the version that was reviewed and approved"* — demonstrable end-to-end and offline:

- **`skillrig add <skill>`** (Vendor Mutation): vendors a named skill from the repo's **resolved origin** (which may be a *local checkout* this slice) into the canonical `.agents/skills/<skill>/`, and writes its identity (version, commit, tree-SHA, path) to a committed `.skillrig/skills-lock.json`.
- **`skillrig verify`** (Verification Gate): offline, deterministic, read-only — recomputes each locked skill's git tree-SHA and compares to the lock (**label-honesty**), and checks the on-disk skill set equals the locked set (**orphan/completeness**). Exit `0`/`1`/`2`.

`skillcore` is the **one** implementation (AP-04) of git tree-SHA computation, `skill.toml` parse, and `skills-lock.json` I/O, consumed by both `add` and `verify` (and reusable by future `bump`/`doctor`). It is **presentation-free** and **never fetches** — pure filesystem core; acquisition + auth live above it (the SDK boundary, SDK-1). The `add → verify` round-trip is the acceptance contract; the tree-SHA is anchored to real `git` output, never invented (Constitution III).

**Deferred (clarified 2026-05-29/30):** prerequisite/eligibility check + exit `3` → `doctor`; network/git **fetch** + auth in `add`; three-way merge + conflict-marker detection → `bump`; multi-client symlink views + agent-shell selection; `index.json`/search; allowlist/audit. See spike §9.

## Technical Context

**Language/Version**: Go 1.24+ (toolchain 1.24.4) — single static binary.
**Primary Dependencies**: existing only — `github.com/spf13/cobra` (command tree), `github.com/pelletier/go-toml/v2` (config + `skill.toml` parse); lock uses stdlib `encoding/json`. **No new dependencies, and no in-process hashing dependency** — the tree-SHA is obtained by *shelling `git`* (see Runtime dependency + research). `go-getter` is explicitly *not* adopted this slice (acquisition is a local origin; OQ-3 deferred). Deps kept minimal (consume-only static binary).
**Runtime dependency (required)**: **`git`** on `PATH` — `skillcore` shells `git` for **all** integrity plumbing (gh-cli's proven pattern — it reimplements nothing): the tree-SHA is `git rev-parse <ref>:<path>` (git's own **canonical tree-object SHA**), commit provenance is `git rev-parse <ref>`, and an uncommitted/dirty vendored tree is detected with `git status --porcelain`. Because the value `add` records and the value `verify` recomputes are *both git's own output*, they match by construction — there is no second implementation to drift (AP-04 hardened), and no autocrlf/mode-bit/tree-sort reimplementation to get subtly wrong. `git` is already a project prerequisite (`init`), and tests use it to bootstrap fixtures (gh-cli's `initRepo` pattern).
**Storage**: local files only — vendored subtree under `.agents/skills/<skill>/` (canonical, committed), `.skillrig/skills-lock.json` (committed, tool-written, atomic). `add` reads the resolved origin (a local path this slice). No database, no network.
**Testing**: Go standard `go test`, two tiers (Constitution II/III): (a) **unit** — table-driven `skillcore` tests + a **ground-truth** test asserting `skillcore.TreeSHA` equals real `git` tree output; (b) **integration** — `TestQuickstart_*` build + exec the real binary over a fixture origin bootstrapped in a tmpDir. **No network boundary this slice → no `httptest`/go-vcr** (that tier arrives with remote `add`).
**Target Platform**: macOS/Linux/Windows terminals, CI, agent runners. (Symlink/Windows concerns deferred with multi-client materialization.)
**Project Type**: single Go module (`github.com/skillrig/cli`).
**Performance Goals**: sub-100ms for `add`/`verify` on typical small skill trees (offline; soft target — cli.md records no per-command duration).
**Constraints**: offline; deterministic; `verify` is **read-only** (it only runs `git rev-parse`/`git status` — no object writes); `verify` checks the **committed** vendored tree (`git rev-parse HEAD:<path>`) and flags an uncommitted/dirty vendored tree as a *distinct* finding; `add` vendors **byte-identical** preserving file modes (the exec bit is part of the tree SHA) and injects nothing; both `add` and `verify` require a **git repository**. Exit codes this slice: `0` ok, `1` usage/config, `2` verification failure. `3` (prerequisite) reserved for `doctor`.
**Scale/Scope**: small — one new package (`pkg/skillcore`), two commands, one exit-code-mapping extension, fixtures. ~Several hundred LOC.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design (below).*

Verify compliance with `.specledger/memory/constitution.md` (v2.1.0):

- [x] **I. Specification-First**: spec.md complete, clarified (2 sessions; 21 reviewer comments resolved), prioritized user stories P1–P3.
- [x] **II. Quickstart-as-Contract**: quickstart.md authored as executable scenarios mapping 1:1 to `TestQuickstart_*`; output-shape assertions — `verify` human output line-count bounded (`≤ skillCount + K`), `--json` parseable + structurally complete (per-skill verdict carries `name`/`path`/`expectedTreeSha`/`actualTreeSha`/`status`), error output asserts what/why/fix as distinct checks + exit code.
- [x] **III. Ground-Truth Anchoring**: the git-origin boundary's ground truth is a **real git tree-SHA** — and because `skillcore` *shells `git`* to obtain it, the recorded value **is** git's canonical tree SHA by construction (no reimplementation to validate against ground truth). Fixtures are bootstrapped via `git init` + commit from `testdata/` (gh-cli's `initRepo` pattern), never hand-authored; data-model.md captures a real recorded tree-SHA sample. The `add → verify` round-trip proves `add` records exactly what `verify` recomputes. No network boundary this slice, so no httptest/go-vcr (deferred with remote `add`).
- [x] **IV. Agent-First CLI Design**: `add` classified **Vendor Mutation** (`--dry-run`, `--force`, idempotent, writes lock via `skillcore` only); `verify` classified **Verification Gate** (offline, deterministic, exit-code-driven, no online/inferential signal — AP-02). Progressive `--help` with ≥2 examples each; errors-as-navigation (what/why/fix, raw cause under `--verbose`, stderr); two-level output (compact human + footer hint, complete `--json`). `skillcore` is the **one** integrity implementation (AP-04).
- [x] **V. Code Quality (Go)**: `gofmt` + `go vet` + `golangci-lint` gate; `pkg/skillcore` is **presentation-free** (returns typed structs + typed errors, no `fmt.Println` of user text); CLI layer renders. Execution/presentation separation preserved.
- [x] **VI. YAGNI**: no `requires` in the lock (manifest on disk is the source of truth); no symlink views; no conflict-marker detection; no network/auth; no `bump`/3-way-merge; no `go-getter`.
- [x] **VII. Shortest Path to MVP**: `skillcore` + `add` (local origin) + `verify` only — the minimum that makes the promise demonstrable.
- [x] **VIII. Simplicity Over Cleverness**: the tree-SHA is git's **own** output (shelled via a small testable client, gh-cli pattern) — no clever in-process re-hashing to get subtly wrong; plain structs + stdlib json/toml; no reflection.
- [x] **IX. Skill–CLI Co-Evolution**: an agent skill update (teaching `add` / `verify` usage, exit `0`/`2` meaning, and that a missing backing tool is **not** a verify failure) is a planned task with a trigger-accuracy eval.

**Design-doc sync (Constitution / Architecture & CLI Design):** `docs/design/cli.md` MUST be updated **in this branch** (CLI behavior change): the `verify` command index line + Verification-Gate row to state `verify` is **integrity-only** (prerequisite/eligibility attributed to `doctor`), and the Execution-vs-Presentation "these are not separate packages" line is **reversed** for `skillcore` (it IS a separate importable package per SDK-1). `docs/ARCHITECTURE-v0.md` was already updated (spike §8). These are tracked as tasks, not plan-blocking.

**Complexity Violations**: None. (`skillcore` living in public `pkg/` rather than `internal/` is **required by SDK-1**, not a complexity violation — recorded in research.md; it strengthens AP-04 rather than weakening any principle.)

## Project Structure

### Documentation (this feature)

```text
specledger/002-skillcore-verify/
├── spec.md                 # user-facing spec (clarified)
├── spec-tech-spike.md      # technical companion / decision log (§1–§12)
├── plan.md                 # This file
├── research.md             # Phase 0 output — decisions + rationale + prior work
├── data-model.md           # Phase 1 output — entities + a real git-tree-SHA ground-truth sample
├── quickstart.md           # Phase 1 output — executable TestQuickstart_* scenarios
├── contracts/              # Phase 1 output
│   ├── add.md              #   `skillrig add` command surface (Vendor Mutation)
│   ├── verify.md           #   `skillrig verify` command surface (Verification Gate)
│   └── skillcore-sdk.md    #   public pkg/skillcore API surface (SDK-1)
├── checklists/requirements.md
└── tasks.md                # Phase 2 output (/specledger.tasks — NOT created here)
```

### Source Code (repository root)

Module path: `github.com/skillrig/cli`.

```text
.
├── main.go                          # unchanged: os.Exit(cli.Execute())
├── go.mod / go.sum                  # no new deps
├── .golangci.yml
├── pkg/
│   └── skillcore/                   # PUBLIC SDK package (SDK-1) — presentation-free, never fetches
│       ├── git.go                   #   small testable git client (gh pattern): pluggable commandContext + GitError; revParse / status
│       ├── treesha.go               #   TreeSHA(gitDir, ref, relPath) — shells `git rev-parse <ref>:<path>` (canonical)
│       ├── manifest.go              #   Manifest + ParseManifest(skill.toml)  (go-toml/v2)
│       ├── lock.go                  #   LockFile/LockEntry types + ReadLock/WriteLock (atomic, NO requires)
│       ├── add.go                   #   Add(opts) (AddResult, error)    — copy subtree (mode-preserving) + write lock
│       ├── verify.go                #   Verify(repoRoot) (Report, error) — label-honesty + orphan + dirty-flag, read-only
│       └── errors.go                #   typed errors (VerifyFailure, etc.) — no user-facing formatting
├── internal/
│   ├── cli/
│   │   ├── root.go                  # registerSubcommands: + newAddCmd, + newVerifyCmd
│   │   ├── add.go                   # wiring: ResolveOrigin → skillcore.Add → render AddResult
│   │   ├── verify.go                # wiring: skillcore.Verify → render Report → exit code
│   │   ├── exit.go                  # exitCodeFor EXTENDED: skillcore.VerifyFailure → ExitVerification (2)
│   │   └── output.go                # render AddResult / VerifyReport (human compact + footer; --json)
│   └── config/                      # UNCHANGED — ResolveOrigin reused by add (CLI layer resolves, passes local path down)
└── test/
    ├── quickstart_test.go           # TestQuickstart_Add* / _Verify* — build + exec the real binary
    └── testdata/
        └── sample-origin/           # canonical sample origin: .skillrig-origin.toml + skills/<skill>/ (research D12);
                                     #   a helper git-inits + commits it into a tmpDir (raw git oracle, D11)
```

**Structure Decision**: `skillcore` lives in **public `pkg/skillcore`** (import `github.com/skillrig/cli/pkg/skillcore`) per SDK-1 — third-party Go tools can build their own `add`/`verify` on the same primitives, and the CLI imports exactly that package (so there is no parallel implementation, AP-04). It is **presentation-free and never fetches** (the SDK boundary): `skillcore.Add` takes an already-resolved local source path + destination repo root; **origin resolution stays in the CLI layer** (`internal/cli/add.go` calls the existing `config.ResolveOrigin`, then passes the resolved local path down), keeping `skillcore` free of origin/config/network concerns. `internal/config` is unchanged and reused.

`skillcore` shells `git` through a **small testable client** (`git.go`) modeled on `gh`'s `git.Client` — a struct with a pluggable `commandContext` (function field, swappable in tests) and a `GitError{ExitCode, Stderr}` type. The **one** primitive `TreeSHA(gitDir, ref, relPath)` runs `git -C gitDir rev-parse <ref>:<relPath>`; `add` calls it on the *origin* (`ref` = resolved ref), `verify` calls it on the *consumer* (`ref` = `HEAD`) — same function, both sides git-canonical (AP-04). `verify` needs no origin: `skillcore.Verify(repoRoot)` reads only the committed lock + the committed vendored tree + `git status` (read-only). The CLI's `exitCodeFor` is extended so a `skillcore` verification failure maps to exit `2` while everything else stays `1` (a typed-error switch). `main.go` is untouched.

## Phase Breakdown (this command produces Phase 0 + Phase 1 artifacts)

- **Phase 0 — research.md**: resolve the spike's plan-level open questions (tree-SHA mechanism, package path, fixture strategy, lock schema, origin-resolution reuse, go-getter deferral) with decisions + rationale + alternatives; summarize prior work.
- **Phase 1 — design**: `data-model.md` (entities + a real git-tree-SHA ground-truth sample), `contracts/{add,verify,skillcore-sdk}.md`, `quickstart.md` (executable scenarios), agent-context update.
- **Phase 2 — tasks.md**: produced by `/specledger.tasks` (NOT here).

## Complexity Tracking

> No constitutional violations to justify. (`pkg/skillcore` public placement is mandated by SDK-1 and recorded in research.md, not a violation.) Table intentionally empty.
