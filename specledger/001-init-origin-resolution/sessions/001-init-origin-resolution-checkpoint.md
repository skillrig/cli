# Session Log: 001-init-origin-resolution

## Divergence Review: 2026-05-26 10:23

Scope: full post-implementation checkpoint (implementation session knowledge).
Adversarial fresh-eyes agent intentionally deferred to a separate session.

### Divergences

| # | Severity | Type | Category | Issue/Artifact | Description |
|---|----------|------|----------|----------------|-------------|
| 1 | ~~MEDIUM~~ **RESOLVED** | oversight | Contract gap (latent) | contracts/resolve.md / spec FR-004 | ~~`ResolveOrigin` → `usableOrigin` collapses every bad-source failure to `(Origin{}, false)` and discards the parse error, so the contract's "malformed-file diagnostic via `--verbose`" cannot be honored.~~ **Fixed in this session** (post-checkpoint): added `config.MalformedError` (typed) + `ResolutionResult.Diagnostics []SourceDiagnostic`. `originFromFile` now classifies each source as usable / skippable-with-diagnostic (malformed or invalid origin) / quiet-fall-through (absent or origin-less) / fatal I/O error. Diagnostics are recorded regardless of final Source. Covered by `TestResolveOrigin_{MalformedProjectRecordsDiagnostic,InvalidShapeRecordsDiagnostic,OriginlessNoDiagnostic,UnreadableFileIsFatal}` + `TestLoadMalformedErrors` (errors.As *MalformedError). contracts/resolve.md + data-model.md updated. |
| 2 | ~~LOW~~ **RESOLVED** | conscious | Architecture/design drift | research.md D9 vs internal/config/config.go | ~~Config written with mode `0o600`/`0o750`; research D9 specified `0o644`/`0o755`, and D9 was not updated.~~ **Reconciled this session**: research.md D9 now documents the shipped `0o600`/`0o750` with rationale (gosec G301/G302; git normalizes the committed mode regardless; user-only is safer). |
| 3 | LOW | conscious | Test realization | quickstart.md Part A / TestQuickstart_PromptInteractive | The interactive-prompt scenario is realized **in-process** (`internal/cli/init_test.go` → `TestInit_PromptInteractive`, injected `interactive=true` + `SetIn`) rather than as a binary-exec E2E test, because the pty test dependency (`creack/pty`) was denied (project mandates minimal deps). The quickstart harness note explicitly sanctions "a pty **or the harness's interactive shim**", so this is within bounds. The one line not exercised E2E is the real `os.Stdin` char-device TTY detection; FR-006c's "override even on a TTY" is covered in-process by `TestInit_NonInteractiveFlagOverridesTTY`. |
| 4 | LOW | conscious | Ground-truth (G1) | data-model.md / quickstart.md / test/fixtures/config.toml | `Save` emits TOML literal-string form `origin = 'my-org/my-skills'` (single quotes — go-toml/v2 default), not the double-quoted form originally shown in the docs. Fixture regenerated from real output and the two illustrative doc blocks updated with a G1 note. `TestSaveMatchesFixture` anchors it. (Review finding G1 pre-authorized regenerating the fixture from real `init` output.) |
| 5 | LOW | conscious | Scope boundary | plan.md / spec FR-001 | `ResolveOrigin` has **no production caller** — only `internal/config/resolve.go` defines it; no CLI command invokes it. This is by design: no consuming command (search/add/verify) is in scope. The resolver is fully unit-tested (`TestResolveOrigin_Precedence` rows 1–7 + `FromSubdir`) but never exercised end-to-end through the binary. First consuming command (next feature) provides E2E coverage. |
| 6 | LOW | oversight | Test coverage | quickstart.md TestQuickstart_BindFromGitSubdir | The quickstart lists a "resolve-symmetry" sub-assertion inside BindFromGitSubdir (`ResolveOrigin(cwd=a/b/c)` → `Source==project`, `ConfigPath==<repo-root>/...`). The E2E test asserts file placement at the git root + no subdir `.skillrig`, but omits the resolve-symmetry check (an exec test cannot call the Go function). Walk-up resolution is covered independently by the in-process `TestResolveOrigin_FromSubdir`, so the behavior is verified — just not within the same test. |
| 7 | LOW | conscious | Coverage (accepted) | spec FR-011 | "MUST NOT scaffold/bootstrap an origin" has no positive/negative test — a "don't do X" constraint. Pre-accepted in the verify review (finding C1); `init` is consume-only with no network/scaffold code path. |

### Force-Closed Issues (DoD Bypassed)

None. All 26 closed issues have fully checked Definition of Done (47/47 DoD items checked). No `--force` closes were used.

### Issues Encountered & Resolutions
- pty dependency (`creack/pty`) denied by sandbox (minimal-deps policy) → interactive prompt covered by an in-process injection shim (quickstart-sanctioned). [#3]
- go-toml/v2 emits single-quoted TOML literals → anchored fixture + docs to real output instead of forcing double quotes. [#4]
- Strict golang-lint baseline (gosec/wsl_v5/noctx) → tightened config perms [#2], added a `_test.go` gosec exclusion rule, used `exec.CommandContext` + `t.Context()` in tests, single targeted `//nolint:gosec` on `config.Load` (G304, documented).

### Items Requiring Action Before Merge
1. ~~[MEDIUM] Carry divergence #1 forward~~ — **DONE this session**: resolver now surfaces skipped-source diagnostics (`ResolutionResult.Diagnostics`) and distinguishes fatal I/O from skippable-malformed (`config.MalformedError`). FR-004 contract gap closed; a future `--verbose` caller has a field to read. Followed up because deferring it risked a later agent (with less context) re-introducing a silent skip.
2. ~~[LOW] Reconcile divergence #2~~ — **DONE this session**: research.md D9 updated to document the shipped `0o600`/`0o750` with rationale.
3. [LOW] Optional: when a pty test dependency becomes acceptable, add an E2E `TestQuickstart_PromptInteractive` to cover the real TTY char-device detection line (#3), and add the resolve-symmetry assertion to BindFromGitSubdir (#6).

### Tests & Checks
- Status: PASS
- Commands run: `go test -count=1 ./...` · `gofmt -l .` · `go vet ./...` · `golangci-lint run ./...`
- Results: all packages pass (internal/cli, internal/config, test); gofmt clean; vet clean; golangci-lint **0 issues**.

### Progress Summary
- Closed: 26 issues (19 leaf tasks + 6 phase parents + 1 epic)
- In Progress: 0 issues
- Open/Remaining: 0 issues
- Force-Closed: 0 issues (DoD bypassed)

### Uncommitted Changes
- None related to this feature. Implementation committed as `1e9beda` (feat(001): implement skillrig init + single origin resolver).
- Pre-existing untracked files unrelated to this feature: `docs/design/{README,commands,hooks,testing,tf-cli-ref}.md`, `docs/guides/vcr-cassettes.md`.

---

## Adversarial Checkpoint Review (fresh-eyes): 2026-05-26 10:50

Scope: context-free adversarial pass (no implementation-session knowledge). Run against the working tree **including the uncommitted FR-004 diagnostics change** (config.go, resolve.go, contracts/resolve.md, data-model.md + tests). Goal: find problems, not confirm success.

### Gate status
All clean on the working tree: `go test -count=1 ./...` PASS (cli, config, test) · `gofmt -l` clean · `go vet ./...` exit 0 · `golangci-lint run` **0 issues**.

### DoD / force-close audit
26 closed issues, 0 open, 0 in_progress. **No force-closed issues** — every `definition_of_done` item is `checked: true` with a `verified_at`. Highest-signal category empty.

### Findings (fresh-eyes; cross-referenced to the self-review above)

| # | Severity | Type | Finding |
|---|----------|------|---------|
| A1 | MEDIUM | oversight | **Malformed `SKILLRIG_ORIGIN` hard-error branch is untested.** `resolve.go:47-51` makes an explicitly-set-but-malformed env override the resolver's *one hard error* (vs. the skip-and-continue of file sources). No test sets a malformed `SKILLRIG_ORIGIN`; the precedence matrix has only blank-env (row 6). The FR-004 diagnostics work added 4 new tests but **none cover this branch** — the error-wrapping (`"%s: %w"`, `envOriginKey` prefix) and the "deliberate override must be valid" hard-fail are unasserted. Add a matrix row / test. |
| A2 | MEDIUM | conscious (scope) | **FR-003 / US3-AS1 no-origin rendering is unreachable end-to-end**, and the new `Diagnostics` field inherits the same status. No registered command calls `ResolveOrigin` (`root.go:97-99` wires only `init`, which *writes*). `Source==none` → actionable two-fix error, and now `Diagnostics` → `--verbose` surfacing, both exist only at the data level with **no production caller** (consistent with self-review #5). Directly qualifies **SC-004**: the resolution no-origin path cannot be hit by a user in this slice. Acceptable as a primitive-for-later, but the spec presents it as reachable. |
| A3 | LOW | conscious | **FR-004 fix introduces a behavior change for the future caller to vet:** a genuine I/O error on a *project* config (e.g. unreadable, perms) is now **fatal** and aborts resolution — even when a valid global default exists. Previously all `Load` failures were skipped → fall-through. The new malformed-vs-I/O split (`MalformedError`) is a real improvement, but "unreadable project config is fatal despite a usable global" is a debatable semantic the wiring command should confirm. `UnreadableFileIsFatal` covers project only (global unreadable-fatal shares the path, untested). |
| A4 | LOW | oversight | `internal/config/origin.go:30` **`Origin.IsZero()` is dead code** — no caller in `internal/`, `test/`, or `main.go`. `golangci-lint`'s `unused` does not flag exported methods. Delete or wire it. |
| A5 | LOW | conscious (stale DoD) | Issue **SL-db8e96** DoD reads *"TestQuickstart_…PromptInteractive written"* → `checked: true`, but no test of that name exists; it was relocated in-process to `TestInit_PromptInteractive` (self-review #3). Relocation is justified and documented in code, but the DoD checkbox describes a deliverable that never existed under that name/package. |
| A6 | LOW | conscious | `stdinIsTTY` (`init.go:199-206`) classifies `/dev/null` as interactive (char device). `skillrig init </dev/null` (no `--origin`) emits the prompt label to stderr, then fails on EOF with `"no input received"` — a different message than the documented non-interactive path. No hang, exits 1. Documented in code comment. |

Findings A2/A5/A6 overlap the self-review's #5/#3 (acknowledged there); restated from a fresh-eyes angle. **A1 is new and is the one gap I'd close before merge.** A3 is a new semantic flag created by the in-flight FR-004 change. A4 is new (dead code).

### Local FR-004 change — sufficiency verdict
The uncommitted diagnostics change **correctly and completely closes the self-review #1 FR-004 gap** (typed `MalformedError`, `ResolutionResult.Diagnostics`, malformed/invalid/origin-less/I-O classification, contract + data-model updated, 4 new tests, gates green). It does **not** address adversarial findings A1 (malformed-env untested) or A2 (no production caller) — and it adds A3. See sufficiency note below the log.

### Disposition of adversarial findings: 2026-05-26 (same session)

| # | Disposition | Detail |
|---|-------------|--------|
| A1 | **FIXED** | Added `TestResolveOrigin_MalformedEnvIsFatal` — explicitly-set-but-malformed `SKILLRIG_ORIGIN` hard-errors (does not fall through to a valid project source) and names the variable. |
| A2 | **Accepted (scope)** | No consuming command in scope, so the resolution-path no-origin render (FR-003 / SC-004 sub-case) and `Diagnostics` `--verbose` surfacing remain primitives-for-later with no production caller. Consistent with self-review #5 and plan.md. The first consuming command (next feature) must wire both. No code change. |
| A3 | **Pinned (contract-aligned)** | The unreadable-project-is-fatal-despite-global semantic is what contract resolve.md specifies (I/O error → fatal Go error). Added `TestResolveOrigin_UnreadableProjectIsFatalDespiteGlobal` to lock it. The wiring command may revisit if a fall-through is later preferred. |
| A4 | **FIXED** | Removed dead `Origin.IsZero()` (no caller; `unused` doesn't flag exported methods). |
| A5 | **Addressed (issue note)** | Added a clarifying `--notes` entry to closed issue **SL-db8e96** recording that the DoD's `TestQuickstart_PromptInteractive` was realized in-process as `TestInit_PromptInteractive` (pty disallowed; harness-shim sanctioned). DoD wording predates the relocation; behavior fully covered. |
| A6 | **Accepted (documented)** | `/dev/null` char-device quirk: `init </dev/null` with no `--origin` prompts then fails "no input received" (exit 1, still a 3-part usage error). Low impact; documented in `stdinIsTTY` comment. Pipe-based non-interactive (the normal agent/CI case) is unaffected. |

Post-disposition gate: `go test -count=1 ./...` PASS · gofmt/vet clean · golangci-lint **0 issues**.

---

## Wrap-up: 2026-05-26 — branch ready to merge

Every divergence and adversarial finding is now resolved or has a tracked justification. Final disposition:

| Item | Status | How addressed / why deferred |
|------|--------|------------------------------|
| Self-review #1 (FR-004 diagnostic gap) | **RESOLVED** | `MalformedError` + `ResolutionResult.Diagnostics`; 4 tests; contract/data-model updated. |
| Self-review #2 (config perms vs D9) | **RESOLVED** | research.md D9 reconciled to `0o600`/`0o750` with rationale. |
| Self-review #3 / A-findings overlap | see below | — |
| A1 (malformed-env untested) | **RESOLVED** | `TestResolveOrigin_MalformedEnvIsFatal`. |
| A3 (I/O-fatal-despite-global semantic) | **RESOLVED (pinned)** | `TestResolveOrigin_UnreadableProjectIsFatalDespiteGlobal`; contract-aligned. |
| A4 (dead `Origin.IsZero`) | **RESOLVED** | Removed; no references remain. |
| A5 (stale SL-db8e96 DoD wording) | **RESOLVED** | Clarifying note added to the closed issue. |
| **A2 (no production caller for `ResolveOrigin` / `Diagnostics`)** | **DEFERRED — justified** | Out of scope by design: this feature ships the resolver + diagnostics as a tested primitive; no consuming command (search/add/verify) is in scope (spec "Out of Scope"; plan.md). FR-003/SC-004's *resolution-path* no-origin render and the `--verbose` diagnostic surfacing become reachable only when the first consuming command is wired (next feature). `init`'s own no-origin paths (US3 AS2/AS4) **are** covered. Not a blocker. |
| **A3-followup / #6 (resolve-symmetry, fall-through semantic)** | **DEFERRED — justified** | The next feature's consuming command should confirm "unreadable project is fatal even with a valid global" is intended (currently contract-aligned + test-pinned) and add the E2E resolve-symmetry assertion inside BindFromGitSubdir (walk-up already covered by `TestResolveOrigin_FromSubdir`). Low risk. |
| #3 / A6 (pty → in-process shim; `/dev/null` message) | **ACCEPTED** | Sanctioned by quickstart harness note; documented in code. Optional E2E pty variant if the dep is later allowed. |
| #4 (G1 single-quote TOML), #7 / C1 (FR-011 no test) | **ACCEPTED** | Pre-authorized by verify review; documented. |

**Tracker:** 26 closed, **0 open, 0 in_progress, 0 force-closed** (47/47 DoD items checked, + SL-db8e96 A5 note).
**Gate:** `go test -count=1 ./...` PASS · gofmt/vet clean · golangci-lint **0 issues**.
**Deferred items (A2, A3-followup) carry no open issue by design** — they are next-feature work with no actionable surface in this slice; justification recorded above and in the PR body so they are not lost.

**Verdict: branch is merge-ready.**

---
