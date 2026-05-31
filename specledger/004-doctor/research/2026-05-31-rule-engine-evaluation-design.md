# Research: Rule-Engine / Evaluation Design for `doctor` in Go

**Date**: 2026-05-31
**Context**: 004-doctor specifies `doctor` as a "rules-based engine" — each check (backing-CLI presence, version constraint, auth-as-distinct-failure, integrity rollup) is an independent deterministic rule, and the design must let more rules be added later (allowlist, audit, risk) without reworking the core. Before `/specledger.plan`, decide whether to adopt an existing Go rule-engine / expression library or hand-roll the engine, and pin down the architecture.
**Time-box**: ~30 min
**Confidence**: High on the library landscape + popularity (GitHub API, fetched today); High on the build-vs-buy conclusion (driven by hard project constraints — AP-04, N6, lean deps); Medium on the exact semver-library pick (a small, reversible call).

## Question

Is there an off-the-shelf Go rule engine / expression-evaluation library that fits `doctor`'s rule engine, and how popular/maintained is each? Or should the engine be hand-rolled? What architecture should `doctor`'s evaluation core take, given the constitution (N6 no-inferential-truth), AP-04 (single shared `skillcore` implementation), and the repo's deliberately minimal dependency footprint?

## Findings

### Finding 1: The Go "rule engine" landscape splits into two categories — and `doctor` needs neither's headline feature

GitHub stats fetched via API on 2026-05-31:

| Library | Stars | Last push | Archived | License | Category |
|---|---|---|---|---|---|
| [expr-lang/expr](https://github.com/expr-lang/expr) | 7,879 | 2026-05-26 | no | MIT | expression evaluator (bytecode VM) |
| [Knetic/govaluate](https://github.com/Knetic/govaluate) | 3,939 | 2025-03-25 | **YES (read-only)** | MIT | expression evaluator (legacy) |
| [google/cel-go](https://github.com/google/cel-go) | 2,981 | 2026-05-27 | no | Apache-2.0 | expression evaluator (typed, policy) |
| [hyperjumptech/grule-rule-engine](https://github.com/hyperjumptech/grule-rule-engine) | 2,503 | 2026-02-10 | no | NOASSERTION | **production rule system** (Drools-style DSL) |
| [PaesslerAG/gval](https://github.com/PaesslerAG/gval) | 812 | 2025-08-04 | no | BSD-3 | expression evaluator |
| [nikunjy/rules](https://github.com/nikunjy/rules) | 238 | 2024-02-24 | no | MIT | expression evaluator (antlr) |

Two distinct categories:

- **Expression evaluators** (expr, cel-go, gval, govaluate): parse and evaluate a *textual expression string* against a runtime context map — `user.Age >= 18 && country == "US"`. Their entire reason for existing is to let **rules live as data/text authored at runtime** (config files, DB rows, user input).
- **Production rule systems** (grule): a forward-chaining engine with a Drools-like DSL (`when … then …`), salience, conflict resolution, a working-memory/fact model. Heavyweight; aimed at hundreds of interacting business rules that mutate facts.

**`doctor`'s rules are neither.** Its rules are a small, fixed, *compiled-in* set (presence / version / auth / integrity) authored by us in Go, not by end users in text. There is **no DSL requirement, no runtime-authored-rule requirement, no forward-chaining/working-memory requirement.** The headline feature every one of these libraries sells — "evaluate rules supplied as text" — is a feature `doctor` does not want. Adopting one would be a category mismatch: paying complexity for a capability that goes unused.

Benchmark note (from [antonmedv/golang-expression-evaluation-comparison](https://github.com/antonmedv/golang-expression-evaluation-comparison)): expr ≈70 ns/op, cel-go ≈91, govaluate ≈153, gval ≈413. Performance is irrelevant here — `doctor` evaluates a handful of rules over a handful of skills, far from any hot path.

### Finding 2: The dependency-cost asymmetry is decisive

Current project footprint (deliberately minimal — see CLAUDE.md "Active Technologies"):

- **3 direct deps**: `go-toml/v2`, `cobra`, `yaml.v3`.
- **`go.sum` is 15 lines total** (incl. transitive). Only `cobra` pulls anything (`mousetrap`, `pflag`).

Cost of each candidate against that baseline:

- **cel-go** drags in `protobuf`, `antlr` runtime, and `golang.org/x/...` — dozens of transitive modules. A large, ongoing supply-chain surface for a tool whose whole posture (002/003) is "minimal, auditable, shells `git`, no bespoke runtime." Hard no.
- **grule** pulls antlr + bytecode + logging deps; also overkill.
- **expr / gval** are leaner (expr is famously zero-dep) but still add a parser + VM to evaluate strings we'd be *generating ourselves* just to feed the evaluator — pure ceremony.

For a CLI that prides itself on a 15-line `go.sum`, adding a rule/expression engine to run four hardcoded checks is unjustifiable. **YAGNI (constitution) points the same way.**

### Finding 3: `verify.go` already *is* the rule-engine pattern doctor should mirror (AP-04)

`pkg/skillcore/verify.go` already implements exactly the shape doctor needs, hand-rolled in idiomatic Go:

```go
// Verdict statuses (string consts): "ok" | "mismatch" | "orphan" | "missing" | "dirty"
type Verdict struct {
    Status string   // the rule outcome
    Reason string   // the actionable, human-readable why
    // … identity fields …
}
func Verify(repoRoot string) (Report, error)   // walks skills, emits a Report of Verdicts
```

This is a **rules-as-typed-functions-over-facts** engine: each check produces a `Verdict{Status, Reason}`; `Report` aggregates; the CLI layer renders. doctor's rules are the same shape — `present` / `missing` / `version-violated` / `version-unverified` / `auth-unreachable`, each with an actionable `Reason`. **AP-04 says there is exactly one `skillcore` implementation of the integrity primitives that `verify`/`doctor` share.** Doctor's integrity rule is *literally a call to the existing `Verify`* (FR-018), so the engine must live next to it in `skillcore` and reuse its verdict vocabulary — an external library would fork the model and violate AP-04.

### Finding 4: Hand-rolled engine — the extensibility seam is a `Rule` interface over a shared fact set

The "add more rules later without reworking the core" requirement (FR-004) is satisfied by a tiny interface, not a library:

```go
// pkg/skillcore (presentation-free). Sketch — finalize in /plan.
type Finding struct {
    Skill   string  // which vendored skill
    Tool    string  // requirement under test (empty for skill-level rules)
    Rule    string  // "presence" | "version" | "auth" | "integrity" | …
    Status  string  // "ok" | "missing" | "violated" | "unverified" | "unauthenticated"
    Reason  string  // actionable: what failed, real cause, suggested fix
    ExitClass int   // 0 advisory/ok · 2 integrity · 3 prereq  → drives precedence
}

type Rule interface {
    Eval(ctx HealthContext) []Finding   // pure over ctx; deterministic; no I/O of its own
}

// HealthContext = the gathered facts (collected ONCE, deterministically, offline):
//   vendored manifests + requires, mise.toml [tools] pins, PATH lookups,
//   local auth reachability, the verify.Report.
```

`doctor` = gather `HealthContext` once → run `[]Rule` → flatten `[]Finding` → pick exit code by max `ExitClass` (the documented precedence, FR-021) → render (human compact / `--json` complete). New rules (allowlist, audit, default-branch cli#6, risk) are new `Rule` values appended to the slice — zero core change. This is the Open/Closed seam the spec asks for, in ~30 lines, no dep.

**Key discipline:** rules must be **pure functions over a pre-gathered fact set**, not perform their own I/O. Gather all facts up front (PATH probes, `mise.toml` read, auth probe via the existing `os.exec` seam, `verify.Report`), then evaluate. This keeps rules trivially unit-testable and the whole run deterministic (N6) and offline.

### Finding 5: Version comparison IS a real sub-problem — and the one place a *focused* library is justified

The version rule (FR-009) compares a **declared version** (from `mise.toml` `[tools]`) against a **constraint** (the skill's `requires.version`, e.g. `>=0.4.0`, `~1.2`). Constraint *parsing + semver comparison* is genuinely fiddly (pre-release ordering, `~`/`^` range semantics) and is NOT a rule engine — it's a comparator. Candidates:

| Library | Stars | Last push | License | Notes |
|---|---|---|---|---|
| [hashicorp/go-version](https://github.com/hashicorp/go-version) | 1,765 | 2026-05-25 | MPL-2.0 | constraints + compare; MPL (weak copyleft) |
| [Masterminds/semver](https://github.com/Masterminds/semver) | 1,419 | 2026-05-27 | MIT | constraints + compare; used by Helm; **MIT** |
| [blang/semver](https://github.com/blang/semver) | 1,049 | 2023-01-15 | MIT | compare-focused; staler |

`Masterminds/semver/v3` is the natural pick: MIT (matches the repo's permissive posture; avoids MPL), actively maintained, battle-tested in Helm, supports exactly the `Constraint.Check(Version)` shape doctor needs, and is a single small module. This is a *bounded, justified* dependency for a hard sub-problem — categorically different from importing a whole rule/expression engine to dodge writing four `if`s.

Open nuances for `/plan` (medium confidence):
- **mise version-spec dialect ≠ strict semver.** `mise.toml` pins can be `1.2.3`, `1.2`, `latest`, `lts`, `ref:…`, `prefix:…`, fuzzy ranges. Only *concrete semver-ish* pins are deterministically checkable; non-concrete pins (`latest`, `ref:`) must fall through to the **"present, version unverified" advisory** (FR-011), never a guess. Decide the parse/normalize rules in `/plan`.
- If the constraint grammar we accept in `requires.version` is kept minimal (exact + `>=`), even Masterminds may be more than needed — but its correctness-for-free still beats hand-rolled semver parsing. Lean toward adopting it.

## Decisions

- **D1 — Hand-roll the rule engine; adopt NO rule-engine or expression-evaluation library** (grule/expr/cel-go/gval/govaluate all rejected). They solve "rules authored as text at runtime," which doctor explicitly does not need; they add disproportionate dependency/complexity against a 15-line `go.sum`; and they would fork the verdict model away from `verify`, violating AP-04.
- **D2 — Model the engine on the existing `verify.go` pattern**, in `pkg/skillcore` (presentation-free): a `Rule` interface returning `[]Finding{Status, Reason, ExitClass}` over a once-gathered, deterministic, offline `HealthContext`. Extensibility (FR-004) = append a `Rule`; no core rework.
- **D3 — doctor's integrity rule calls the existing `skillcore.Verify`** (not a reimplementation), honoring AP-04 and FR-018/FR-019 (verify stays integrity-only).
- **D4 — Rules are pure over pre-gathered facts**; all I/O (PATH, `mise.toml`, auth probe, verify) happens in a single gather phase, keeping the run deterministic (N6) and offline (no `tool --version` execution).
- **D5 — Adopt `Masterminds/semver/v3` (MIT) solely for version-constraint parsing/comparison** — a focused, justified dependency for a real sub-problem, not a rule engine. Non-concrete `mise` pins fall through to the "version unverified" advisory.

## Recommendations

1. **Plan the engine as a `skillcore` package extension**, reusing `Verdict`/`Reason` vocabulary and the `Report`-aggregation style already proven in `verify.go`. Put the `Rule` interface + `HealthContext` gatherer + exit-class precedence there; keep `internal/cli/doctor.go` to flag-parse + dispatch + render.
2. **Add `github.com/Masterminds/semver/v3` as the 4th direct dependency** (the first since 003's `yaml.v3`), scoped to the version rule. Record the rationale in the plan's Complexity Tracking so the dep is a conscious, reviewed choice.
3. **Decide the `mise.toml` version-spec normalization rules in `/plan`**: which pin forms are "concrete enough" to check vs. which fall through to the unverified advisory. This is the main remaining unknown.
4. **Write each rule as an independently table-testable pure function** (presentation-free unit tests, constitution §III), with the gather phase stubbed via fact fixtures and the auth probe via the existing `commandContext` exec-stub seam.
5. **Document the precedence (FR-021) as "max ExitClass wins"** (3 prereq > 2 integrity > 0), so multi-class runs are deterministic and the report still lists every finding.

## References

- [expr-lang/expr](https://github.com/expr-lang/expr) — 7.9k★, MIT, active (expression evaluator)
- [google/cel-go](https://github.com/google/cel-go) — 3.0k★, Apache-2.0 (typed expression/policy; heavy deps)
- [Knetic/govaluate](https://github.com/Knetic/govaluate) — 3.9k★, **archived 2025-03-25**
- [hyperjumptech/grule-rule-engine](https://github.com/hyperjumptech/grule-rule-engine) — 2.5k★ (Drools-style production rule system)
- [PaesslerAG/gval](https://github.com/PaesslerAG/gval) — 812★ (expression evaluator)
- [antonmedv/golang-expression-evaluation-comparison](https://github.com/antonmedv/golang-expression-evaluation-comparison) — benchmark harness
- [Masterminds/semver](https://github.com/Masterminds/semver) — 1.4k★, MIT (chosen for version constraints)
- [hashicorp/go-version](https://github.com/hashicorp/go-version) — 1.8k★, MPL-2.0 (semver alt)
- Internal: `pkg/skillcore/verify.go` (`Verdict`/`Report` pattern), `pkg/skillcore/manifest.go` (`Require` struct), `docs/ARCHITECTURE-v0.md` §8/§8b (mise realities, R18 auth), `docs/design/cli.md` (Environment pattern, exit codes), spec `004-doctor/spec.md` (FR-004/009/011/018/021).
