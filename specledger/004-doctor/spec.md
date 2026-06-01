# Feature Specification: `doctor` — Environment Health & Backing-CLI Readiness

**Feature Branch**: `004-doctor`
**Created**: 2026-05-31
**Status**: Draft
**Input**: User description: "doctor — environment health & backing-CLI readiness (roadmap 005 + 006 combined into one slice). A `skillrig doctor` command that answers 'is this repo's vendored-skill setup sound AND is my environment actually ready to run those skills?' — a developer/agent-facing readiness check, complementary to the CI-focused `verify`. doctor is a rules-based engine, designed so more rules can be added later."

## Overview

The first three slices made a repo *self-describing about where its skills come from* (001 `init`), made the skills it carries *honest* (002 `add` + `verify`), and let a user *find and acquire* skills from a remote library (003 `search` + remote `add` + `index`). The everyday path is now `init` → `search` → `add` → `verify`.

But a skill is rarely self-contained: it often expects a backing CLI to be installed (e.g. a plan-review skill expects `terraform` and an org-internal tool on PATH). `verify` deliberately does **not** check for those — it validates that vendored *content* is byte-honest, nothing about the surrounding environment. So a developer or agent can have integrity-clean skills and still fail at run time because the tool a skill needs isn't on PATH, is the wrong version, or lives in a private repo the user can't authenticate to. Today nothing surfaces that gap, and the failure shows up late — at agent-run time — with no actionable diagnosis.

This slice closes that gap with `skillrig doctor`: a **developer/agent-facing readiness check** that answers two questions at once — *is the vendored-skill setup sound* (integrity) and *is my environment actually ready to run these skills* (readiness). `verify` and `doctor` have **different audiences**: `verify` is the **CI integrity gate** (is the committed content exactly what was approved?); `doctor` is the **developer/agent "can I actually use these skills right now, or am I about to install them cleanly?"** check. doctor is a superset in coverage (it includes an integrity rollup) but its purpose and primary caller are the human/agent at the keyboard, not the CI pipeline.

`doctor` is built as a **rules-based engine**: each check is an independent rule evaluated over the repo and its vendored skills. The foundational rule set in this slice is:

- **`path-presence`** (always runs) — is each required tool resolvable on PATH?
- **`mise-version-check`** (runs when a `mise.toml` is present at repo root) — does the tool's declared version constraint hold against the `mise.toml` pin?
- **`source-auth-reachability`** (runs for requirements whose source is a concrete GitHub repo; online by default) — can the source repo and its release assets be reached with current authentication?
- **`integrity`** (always runs) — the existing label-honesty + orphan check, rolled up from `verify`.

`path-presence` and the mise rules are **separate rules that both run** — there is **no PATH→mise fallback** (a mise shim may not have reached the agent's environment even when `mise.toml` lists the tool, so PATH must be checked on its own). The engine is designed so later slices add rules (allowlist, audit, risk signals, default-branch) by adding a rule, without reworking the core — and this slice exposes that extensibility as a first-class, tested developer outcome.

doctor runs **online by default** (so it can answer reachability/auth questions a developer actually cares about); local rules are deterministic and a `--offline` flag (or an unreachable network) degrades the online rules to an honest advisory rather than failing. This slice combines roadmap items **005** (backing-CLI prerequisites — declare + verify) and **006** (`doctor`). They ship together because the prerequisite-readiness logic has no command home *except* `doctor` (and `add`); `verify` stays integrity-only by design. `doctor` is also where the long-reserved **exit code 3 (prerequisite failure)** finally lands.

## Clarifications

### Session 2026-05-31

Initial targeting (pre-specification):

- **Q: How does doctor learn a tool's declared version for a constraint check?** → **Read the consumer repo's `mise.toml` `[tools]` pins as data** (no subprocess, no probing the tool itself). The version check is the `mise-version-check` rule, gated on a `mise.toml` being present.
- **Q: How deep is version-constraint matching?** → Evaluated **only against the `mise.toml` pin**; doctor never executes a tool to discover its version (constitution N6 — no inferential truth). A non-concrete pin (e.g. `latest`, `ref:…`) passes the rule **with a warning** (no pin means the actual installed version can't be guaranteed to match the constraint).
- **Q: Slice boundary for 005 + 006?** → **One combined slice** delivering the `doctor` command and the readiness rules.

Comment-driven clarifications (reviewer comments on this spec):

- **Q: Must doctor be offline/deterministic? (comments a1600392, 9d60641f)** → **No — online by default, `--offline` skips network rules.** Local rules (`path-presence`, `mise-version-check`, `integrity`) are deterministic and always available; online rules (`source-auth-reachability`) run by default and degrade to an advisory "unverified (offline / unreachable)" note under `--offline` or when the network is unreachable. The blanket "deterministic offline" framing is dropped (it contradicted the auth-fetch story).
- **Q: Is standalone `doctor` the only entry point? (comment 6566d72b)** → **No.** `skillrig add` also runs the readiness rules on the just-vendored skill and prints a concise notice when required binaries are missing; when a `mise.toml` is present it adds a "try installing via mise" footnote (a hint only — actual install is left to a mise-specific skill / the org's mise workflow).
- **Q: When does the auth/reachability rule apply? (comment 5c587452)** → **Only when a requirement is concretely identifiable as served from a GitHub repo** (a parseable `owner/repo` source). Then doctor probes repo + release-asset readability under current auth. For opaque sources (or `manager: mise` with no GitHub-repo source — the mise backend is mise-specific and not skillrig's to interpret) the rule reports **N/A**, never a false failure.
- **Q: PATH vs mise — fallback or separate rules? (comment 083ad9ff)** → **Separate rules; both always run; no fallback.** When `mise.toml` lists a tool but the PATH lookup fails, `path-presence` reports a **prerequisite failure (exit 3)** — a mise shim that hasn't reached the environment means the tool is not actually runnable, and that must surface, not be masked by the mise declaration.
- **Q: doctor vs verify positioning? (comment 1f106cf5)** → **Different audiences.** `verify` = CI-focused integrity gate; `doctor` = developer/agent-focused readiness ("everything's fine to use the available skills, or to install a new one cleanly"). doctor rolls integrity in but is not "verify-for-CI."
- **Q: Is rule-engine extensibility a user story? (comment a1600392)** → **Yes** — add an explicit developer-extensibility user story (a contributor adds a new rule by implementing one rule interface + registering it, with a test pattern), so easy rule-addition is a tracked, demonstrable outcome rather than only an internal design note.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — "Can I actually use these skills?" backing-CLI presence (Priority: P1)

A developer or agent working in a repo that carries vendored skills wants to know, before relying on them, whether the environment can actually run them. They run `skillrig doctor`. The command inspects every vendored skill's declared requirements and reports, per skill and per required tool, whether each tool is **present on PATH** — partitioning skills into **eligible** (every requirement satisfied) and **ineligible** (something unmet), each ineligible item carrying a specific, actionable reason. A tool missing from PATH is a prerequisite failure with a non-zero exit, even if it is declared in `mise.toml` (a mise shim may not have reached this environment — see US3).

**Why this priority**: This is the core value and smallest standalone MVP. It turns "the agent mysteriously failed" into "tool X required by skill Y is not on PATH — here is how to fix it." Without it, integrity-clean skills can still be unrunnable with no diagnosis.

**Independent Test**: In a repo with one vendored skill that requires a tool, run `doctor` with the tool absent from PATH (ineligible, actionable reason, exit 3) and again with it present (eligible, exit 0).

**Acceptance Scenarios**:

1. **Given** a vendored skill that requires a tool, **and** the tool is on PATH, **When** the user runs `skillrig doctor`, **Then** the skill is eligible and the command exits 0.
2. **Given** the same repo **but** the tool is absent from PATH, **When** the user runs `skillrig doctor`, **Then** the skill is ineligible with a reason naming the missing tool and the skill that needs it, and the command exits 3.
3. **Given** multiple vendored skills with varied requirements, **When** the user runs `skillrig doctor`, **Then** the output partitions skills into eligible/ineligible sets, each ineligible entry stating which requirement failed and why.
4. **Given** vendored skills that declare **no** requirements, **When** the user runs `skillrig doctor`, **Then** every skill is eligible and the command exits 0.

---

### User Story 2 — Readiness notice when vendoring a new skill (`add`) (Priority: P2)

A developer or agent runs `skillrig add <skill>` to vendor a new skill from the origin. Immediately after the skill lands, `skillrig add` runs the readiness rules over the **just-vendored skill** and, if any required binary is missing, prints a concise notice ("`<skill>` requires `<tool>`, which is not on PATH"). When a `mise.toml` is present at repo root, the notice adds a hint that the tool may be installable via mise — a pointer only; performing the install is left to a mise-specific skill or the org's mise workflow. The notice is informational and does **not** fail the `add` (the skill is vendored successfully regardless).

**Why this priority**: Vendoring is the moment a new requirement enters the repo, so it's the most natural point to surface a missing prerequisite — the developer learns immediately, not later at run time. It reuses doctor's engine, so it's additive once the rules exist.

**Independent Test**: `add` a skill that requires an absent tool; assert a readiness notice naming the tool appears (and a mise hint when `mise.toml` exists), while `add` still succeeds. Repeat with the tool present; assert no notice.

**Acceptance Scenarios**:

1. **Given** an origin skill that requires a tool not on PATH, **When** the user runs `skillrig add <skill>`, **Then** the skill is vendored successfully **and** a readiness notice names the missing tool and the skill that needs it.
2. **Given** the same, **and** a `mise.toml` exists at repo root, **When** the user runs `skillrig add <skill>`, **Then** the notice additionally hints that the tool may be installable via mise.
3. **Given** an origin skill whose required tools are all present, **When** the user runs `skillrig add <skill>`, **Then** no readiness notice is printed and `add` behaves exactly as before.

---

### User Story 3 — Version constraints via mise, separate from PATH (Priority: P2)

A skill declares it needs a tool at a minimum version. Independently of the PATH-presence check (US1), when a `mise.toml` is present at repo root the `mise-version-check` rule evaluates the skill's version constraint against the tool's `mise.toml` pin. A pin that violates the constraint is a prerequisite failure. A non-concrete pin (`latest`, `ref:…`, etc.) cannot guarantee the installed version matches, so the rule **passes with a warning**. The PATH-presence and mise rules are separate and both run: a tool listed in `mise.toml` but absent from PATH still fails US1's presence check (the shim may not have reached this environment).

**Why this priority**: Version mismatches are a real readiness failure, and `mise.toml` is the only trustworthy declarative version source (constitution N6). It builds on US1's presence rule.

**Independent Test**: In a repo with `mise.toml`, run `doctor` for a skill requiring `tool >= X`: (a) pin satisfies → eligible, exit 0; (b) pin violates → ineligible, exit 3, reason cites declared pin + required constraint; (c) pin is `latest` → pass with a warning, exit 0; (d) tool in `mise.toml` but not on PATH → exit 3 on the presence rule.

**Acceptance Scenarios**:

1. **Given** a skill requiring a minimum version **and** a satisfying `mise.toml` pin, **When** `skillrig doctor` runs, **Then** the version rule passes and (if PATH-present) the skill is eligible, exit 0.
2. **Given** a skill requiring a minimum version **and** a violating `mise.toml` pin, **When** `skillrig doctor` runs, **Then** the version rule fails with a reason citing the declared pin, the required constraint, and the skill, and the command exits 3.
3. **Given** a skill requiring a minimum version **and** a non-concrete `mise.toml` pin (e.g. `latest`), **When** `skillrig doctor` runs, **Then** the version rule passes with a warning that the version is unverified, exit 0.
4. **Given** no `mise.toml` at repo root, **When** `skillrig doctor` runs, **Then** the `mise-version-check` rule does not run for any tool (only `path-presence` applies to version-bearing requirements), and version is reported as unverified-advisory.
5. **Given** a tool declared in `mise.toml` but absent from PATH, **When** `skillrig doctor` runs, **Then** the command exits 3 on the `path-presence` rule (no fallback to the mise declaration).

---

### User Story 4 — Authentication to a backing CLI's GitHub source as a distinct failure (Priority: P2)

A repo's skill requires an org-internal backing CLI whose source is a **concrete GitHub repo** (`owner/repo`). When `skillrig doctor` runs online (the default), the `source-auth-reachability` rule probes whether that repo and its release assets are reachable under the current authentication, and reports an **authentication failure** as a class **distinct** from "tool missing" and from "repo unreachable/not-found." The rule applies **only** to requirements with a parseable GitHub-repo source; for opaque sources (including `manager: mise` with no GitHub-repo source — the mise backend is mise-specific) it reports **N/A**. Under `--offline` or when the network is unreachable, the rule degrades to an advisory "reachability unverified (offline)" note rather than a failure.

**Why this priority**: Authentication-versus-missing is the top onboarding footgun for private backing CLIs (architecture R18); conflating them sends people down the wrong fix path. It is scoped narrowly (probeable GitHub sources only) to avoid false verdicts on sources skillrig can't interpret.

**Independent Test**: For a requirement whose source is a GitHub repo, run `doctor` online with auth unavailable (distinct auth failure, exit 3) and with auth available (rule passes). Run with `--offline` (advisory, not a failure). For an opaque-source requirement, assert the rule is N/A. The probe is exercised through the existing exec-stub seam.

**Acceptance Scenarios**:

1. **Given** a requirement whose source is a concrete GitHub repo **and** authentication is unavailable, **When** `skillrig doctor` runs online, **Then** the rule reports an authentication failure distinct from "tool missing" and "unreachable," names the tool and source, tells the user how to authenticate, and the command exits 3.
2. **Given** the same **but** authentication is available, **When** `skillrig doctor` runs online, **Then** the rule passes (no auth-related failure for that requirement).
3. **Given** a requirement whose source is opaque or not a GitHub repo, **When** `skillrig doctor` runs, **Then** the rule reports N/A for that requirement (no auth check).
4. **Given** `--offline` (or an unreachable network), **When** `skillrig doctor` runs, **Then** `source-auth-reachability` is reported as an advisory "unverified (offline)" note and does not by itself cause a non-zero exit.

---

### User Story 5 — One readiness command that also rolls up integrity (Priority: P2)

A developer wants a single "is everything OK here for me to work?" command. `skillrig doctor` rolls the existing integrity check (the same label-honesty + orphan detection `verify` performs) into its report alongside the readiness findings. This makes doctor the developer/agent superset; it does not change `verify`, which remains the CI-focused integrity gate.

**Why this priority**: Gives the developer/agent one trustworthy command. It reuses the already-shipped integrity primitive, so it is low-cost, but depends on the readiness rules existing.

**Independent Test**: In a repo with a tampered/orphaned vendored skill, run `doctor`; the integrity problem appears and drives the integrity-failure exit. In a clean, ready repo, doctor reports integrity-OK + readiness-OK and exits 0.

**Acceptance Scenarios**:

1. **Given** a repo whose vendored content fails the integrity check, **When** `skillrig doctor` runs, **Then** the integrity problem is reported and the command exits with the integrity-failure code (2).
2. **Given** a repo that is integrity-clean and fully ready, **When** `skillrig doctor` runs, **Then** the report shows integrity-OK and readiness-OK and the command exits 0.
3. **Given** a repo that is integrity-clean but has a missing required tool, **When** `skillrig doctor` runs, **Then** both facts are reported and the exit code follows the defined precedence (prerequisite failure wins).

---

### User Story 6 — Machine-readable readiness for agents and CI (Priority: P3)

An agent or CI pipeline runs `skillrig doctor --json` and consumes a complete, untruncated report: every skill, every requirement, each rule's verdict (pass / fail / warning / N/A / advisory) and reason, the integrity verdict, and the network-rule status. Human output stays compact with a footer hint pointing at `--json`/`--verbose`.

**Why this priority**: Agents and CI are first-class callers and the two-level output contract is a project-wide requirement; it's additive once the rule results exist.

**Independent Test**: Run `doctor --json` against a mixed repo and assert the output is parseable and structurally complete (all skills, requirements, rule verdicts, integrity verdict, network-rule status). Run the human form and assert bounded, compact output with a footer hint.

**Acceptance Scenarios**:

1. **Given** any repo state, **When** the user runs `skillrig doctor --json`, **Then** the output is valid, parseable, and contains the complete per-skill/per-requirement rule results, the integrity verdict, and the network-rule status, with no truncation.
2. **Given** any repo state, **When** the user runs `skillrig doctor` (human form), **Then** the output is compact (bounded), groups skills into eligible/ineligible, and ends with a footer hint pointing at `--json`/`--verbose`.
3. **Given** a failing run, **When** the user adds `--verbose`, **Then** the raw underlying causes behind each failure are printed; errors go to stderr while report data goes to stdout.

---

### User Story 7 — A contributor adds a new doctor rule (Priority: P3)

A skillrig contributor wants to add a new health rule (e.g. a future allowlist or default-branch check) without touching the engine core or the output rendering. They implement a single rule contract (one function/type producing verdicts with reasons) and register it; the new rule is then evaluated and rendered uniformly with the existing rules, and the contributor can unit-test it in isolation against fact fixtures.

**Why this priority**: The rules engine's whole point is cheap extension; making that a tested, demonstrable outcome (not just an internal note) guards the design against later rules forcing a rewrite. P3 because it's a developer-experience outcome, not an end-user flow.

**Independent Test**: Add a trivial example rule via the rule contract + registration, and assert (a) it appears in `doctor` output and `--json` with verdict + reason, (b) it is unit-testable against a fact fixture without invoking the CLI, and (c) no engine-core or renderer change was required.

**Acceptance Scenarios**:

1. **Given** the rule contract, **When** a contributor implements and registers a new rule, **Then** `doctor` evaluates it and renders its verdict/reason in both human and `--json` output with no change to the engine core or renderer.
2. **Given** a new rule, **When** the contributor writes a unit test, **Then** the rule can be exercised in isolation over a fact fixture (no CLI invocation, no network).

---

### Edge Cases

- **No origin configured / not in a project.** `doctor` still runs and reports what it can (integrity over on-disk content, presence/version over vendored manifests); a missing origin is reported, not fatal.
- **No vendored skills at all.** `doctor` reports a healthy/empty state and exits 0 (idempotent, nothing to check).
- **A vendored skill declares no requirements.** Trivially eligible on readiness; only `integrity` applies.
- **A requirement names a tool but no version constraint.** `path-presence` applies; `mise-version-check` is a no-op for that tool.
- **A `mise.toml` is absent.** The `mise-version-check` rule does not run; version-bearing requirements report version as unverified-advisory, and presence is judged on PATH alone.
- **A `mise.toml` is malformed/unreadable.** Treated as "no usable mise version source," recorded as a diagnostic under `--verbose`; it does not crash the run.
- **A `mise.toml` pin is non-concrete (`latest`, `ref:…`).** `mise-version-check` passes with a warning (version unverifiable against the constraint).
- **A tool is in `mise.toml` but not on PATH.** `path-presence` fails (exit 3) — no fallback to the mise declaration.
- **A requirement's version constraint is malformed** in the manifest. Reported as a requirement-level problem naming the offending skill; must not silently pass.
- **Network unreachable / `--offline`.** Online rules (`source-auth-reachability`) degrade to advisory "unverified (offline)" notes; local rules and exit codes are unaffected.
- **A requirement's `source` is not a parseable GitHub repo.** `source-auth-reachability` reports N/A (no false failure).
- **Multiple failure classes in one run** (integrity mismatch *and* a missing tool). Exit code follows a single documented precedence; the report shows all findings.

## Requirements *(mandatory)*

### Functional Requirements

**Command surface, audience & engine**

- **FR-001**: The system MUST provide a `skillrig doctor` command that runs the readiness + integrity rules over the current consumer repo and exits with a status code reflecting the outcome.
- **FR-002**: `doctor` MUST be idempotent and perform no mutations: repeated runs against an unchanged repo and environment produce the same findings and exit code (subject to genuine environment/network changes for online rules).
- **FR-003**: `doctor` MUST function without a fully-configured project — it reports what it can and records what it cannot check, rather than hard-failing on absent configuration such as a missing origin.
- **FR-004**: `doctor` MUST be implemented as a rules-based engine in which each check is an independent rule producing a verdict (pass / fail / warning / advisory / N/A) with an actionable reason, structured so additional rules can be added by adding a rule, without reworking the engine core or the output rendering.
- **FR-005**: `doctor` is **developer/agent-facing readiness**; `verify` remains the **CI-focused integrity gate**. `doctor` MUST NOT change `verify`'s behavior or scope, and the two MUST be described by audience, not as duplicates.

**Network posture**

- **FR-006**: `doctor` MUST run **online by default**, executing online rules (reachability/auth) as well as local rules.
- **FR-007**: Local rules (`path-presence`, `mise-version-check`, `integrity`) MUST be deterministic and fully available without network access.
- **FR-008**: `doctor` MUST support `--offline`, which skips online rules; under `--offline` (or when the network is genuinely unreachable) online rules MUST degrade to an advisory "unverified (offline)" note and MUST NOT, by themselves, cause a non-zero exit.

**Rule: `path-presence` (always runs)**

- **FR-009**: For every vendored skill, `doctor` MUST read that skill's declared tool requirements from its vendored manifest (the single on-disk source of truth).
- **FR-010**: For each required tool, `doctor` MUST check whether it is resolvable on PATH and MUST treat a tool absent from PATH as a prerequisite failure (exit 3), naming the tool and the skill that needs it — **independently of any `mise.toml` declaration** (no PATH→mise fallback).
- **FR-011**: `doctor` MUST partition vendored skills into **eligible** (all rules satisfied) and **ineligible** (one or more rules failed), and for each ineligible skill MUST state, per failing rule, what failed and why. A skill with no requirements is eligible on readiness.

**Rule: `mise-version-check` (runs when `mise.toml` present at repo root)**

- **FR-012**: When a `mise.toml` exists at repo root, `doctor` MUST read its `[tools]` pins as structured data (no subprocess) and, for each requirement that declares a version constraint, evaluate the constraint against the corresponding pin.
- **FR-013**: A pin that violates the constraint MUST be a prerequisite failure (exit 3) with a reason citing the declared pin, the required constraint, and the skill.
- **FR-014**: A non-concrete pin (e.g. `latest`, `ref:…`) MUST cause the rule to pass **with a warning** that the version cannot be verified against the constraint.
- **FR-015**: `doctor` MUST NOT determine a tool's version by executing the tool or inferring it from runtime output; version evaluation relies solely on the `mise.toml` pin (constitution N6).
- **FR-016**: When no `mise.toml` is present (or it lacks a pin for the tool), `doctor` MUST report the version as an unverified advisory and MUST NOT fail on version grounds; the `path-presence` rule still applies.
- **FR-017**: A `mise.toml` that is absent, malformed, or unreadable MUST NOT be fatal; a parse problem is recorded as a diagnostic surfaced under `--verbose`.

**Rule: `source-auth-reachability` (runs online for GitHub-repo sources)**

- **FR-018**: The `source-auth-reachability` rule MUST apply **only** to requirements whose declared source is a parseable GitHub repo (`owner/repo`); for opaque or non-GitHub sources (including `manager: mise` with no GitHub-repo source) it MUST report **N/A** and never a failure.
- **FR-019**: When applicable and running online, the rule MUST probe whether the source repo and its release assets are reachable under current authentication, and MUST report an **authentication failure** as a class distinct from "tool missing" and from "repo unreachable/not-found."
- **FR-020**: An authentication failure MUST be actionable — naming the tool and source and telling the user how to authenticate — and MUST be surfaced prominently (top onboarding footgun, architecture R18).
- **FR-021**: Authentication credentials MUST be resolved via the same mechanism the rest of the CLI uses (token env vars then `gh`), reusing the existing token-resolution path; no new credential surface is introduced.

**Rule: `integrity` (always runs)**

- **FR-022**: `doctor` MUST run the existing vendored-content integrity check (label-honesty + orphan detection) via the **same shared implementation** `verify` uses (one implementation, never a parallel copy — AP-04), and fold its verdict into the report.

**`add`-time readiness notice**

- **FR-023**: After `skillrig add` vendors a skill, it MUST run the readiness rules over the just-vendored skill and print a concise notice when any required binary is missing, naming the tool and the skill.
- **FR-024**: When a `mise.toml` is present at repo root, the `add` readiness notice MUST add a hint that the missing tool may be installable via mise (a pointer only; skillrig does not perform the install).
- **FR-025**: The `add` readiness notice MUST be informational only — it MUST NOT change `add`'s exit code or prevent the skill from being vendored.

**Exit codes & precedence**

- **FR-026**: `doctor` MUST use the project exit-code contract: `0` healthy (including warnings and advisory notes), `1` usage/configuration error, `2` integrity failure, `3` prerequisite failure (tool missing from PATH, declared-and-violated version constraint, or authentication failure on a probeable GitHub source).
- **FR-027**: When more than one failure class is present, `doctor` MUST select the exit code by a single documented deterministic precedence (prerequisite `3` over integrity `2` over `0`), while the report still shows findings from all classes.
- **FR-028**: `doctor` MUST be the only command that emits exit code 3; this slice introduces the first real use of the prerequisite-failure code.

**Output contract (two-level, errors-as-navigation)**

- **FR-029**: `doctor` MUST provide a compact human report grouping skills into eligible/ineligible, summarizing each finding, and ending with a footer hint pointing to the fuller views.
- **FR-030**: `doctor` MUST provide a `--json` form that is complete and untruncated: every skill, every requirement, each rule's verdict and reason, the integrity verdict, and the network-rule status.
- **FR-031**: `doctor` MUST support `--verbose` as the escape hatch printing raw underlying causes; errors MUST go to stderr and report data to stdout.
- **FR-032**: Every `doctor` failure message MUST follow errors-as-navigation: what failed, the real (never-swallowed) cause, and a suggested fix.

**Extensibility (developer outcome)**

- **FR-033**: The rule engine MUST expose a single rule contract such that a contributor can add a new rule by implementing that contract and registering it, with the new rule evaluated and rendered uniformly (human + `--json`) without changes to the engine core or renderer.
- **FR-034**: Each rule MUST be unit-testable in isolation against a fact fixture, without invoking the CLI or the network.

**Co-evolution & docs**

- **FR-035**: The consolidated `skillrig` agent skill MUST be extended to cover `doctor` (and the `add` readiness notice) — a reference plus updated routing/triggering keywords, no new top-level skill — with triggering accuracy validated.
- **FR-036**: The CLI design contract and the roadmap MUST be updated in the same change to reflect `doctor` (its audience-vs-`verify` framing, the rule set, network posture, `add` notice, exit-code-3 introduction, and that roadmap 005 + 006 are delivered by slice 004), and the architecture document kept in sync.

### Key Entities *(include if feature involves data)*

- **Requirement**: One declared dependency of a vendored skill — the tool name (looked up on PATH and against `mise.toml`), an optional version constraint, the source (which, when a parseable GitHub repo, enables the auth-reachability rule), and the manager (provisioning hint). Parsed from the vendored skill manifest; never written to the lockfile.
- **Rule**: An independent check (`path-presence`, `mise-version-check`, `source-auth-reachability`, `integrity`, …) producing a verdict (pass / fail / warning / advisory / N/A) and an actionable reason. The set is extensible via a single rule contract.
- **Health Report**: The complete result of a run — per-skill eligibility, per-requirement per-rule verdicts with reasons/warnings/advisories, the integrity verdict, and the network-rule status — rendered compactly for humans and completely for `--json`.
- **mise Version Pin**: A tool's version declaration in the consumer repo's `mise.toml` `[tools]` — the only trusted basis for version-constraint evaluation; may be concrete (checkable) or non-concrete (warning).
- **Eligibility Partition**: The grouping of vendored skills into those whose every rule is satisfied and those with at least one failing rule, each ineligible entry carrying its specific failing reason(s).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer/agent running `skillrig doctor` on a repo with a missing required backing CLI learns, from one command, exactly which tool is missing and which skill needs it — without consulting any other tool.
- **SC-002**: A developer vendoring a skill whose backing CLI is absent is told so at `add` time (with a mise hint when applicable), so a missing prerequisite is discovered at vendor time rather than at agent-run time.
- **SC-003**: `doctor`'s local rules produce the same findings and exit code on every run against an unchanged repo (deterministic) and run identically with no network when `--offline` is used; online rules add reachability/auth signal when the network is available and degrade to an explicit advisory when it is not — never a false failure.
- **SC-004**: When a required private backing CLI hosted on a GitHub repo cannot be authenticated, `doctor` reports an authentication failure unambiguously distinct from "tool missing," so the user takes the correct corrective action (fix credentials, not reinstall) on the first attempt.
- **SC-005**: `doctor` never reports a version verdict it cannot justify: every version pass/fail traces to a concrete `mise.toml` pin, and every unverifiable version (no pin or non-concrete pin) is reported as an explicit warning/advisory rather than a guess.
- **SC-006**: A tool present in `mise.toml` but absent from PATH is reported as a prerequisite failure (exit 3), so a mis-shimmed mise environment is caught rather than masked.
- **SC-007**: An agent or CI consumer can extract any individual finding (a skill's eligibility, a requirement's per-rule verdict, the integrity result, the network-rule status) from `doctor --json` without truncation.
- **SC-008**: The human form of `doctor` stays compact (bounded output) regardless of skill/requirement count, while `--json` remains complete — verified by output-shape assertions, not substring matching.
- **SC-009**: A contributor can add a new health rule by implementing and registering the rule contract, and the rule is evaluated, rendered (human + `--json`), and unit-tested in isolation — with no change to the engine core or renderer.

### Previous work

These merged slices established the primitives `doctor` builds on:

- **001 — `init` + origin resolution**: the single origin resolver and Environment-pattern command conventions. (Epic `SL-227789`.)
- **002 — `skillcore` + local `add` + `verify`**: the shared integrity primitive (label-honesty + orphan) `doctor` rolls up, and the manifest data model carrying tool requirements (parsed, not yet consumed).
- **003 — `search` + remote `add` + `index`**: the migration to `SKILL.md` frontmatter where requirements are declared (`metadata.x-skillrig.requires`), and the auth/token-resolution pattern (`os.exec` of `gh`/token env) reused by `source-auth-reachability`. `add` is also extended here for the readiness notice.

Open tracked items related to but **out of scope** for this slice:

- **Vendor more skills into the fixture origin** (cli#9): broadens multi-skill end-to-end coverage; useful test substrate but not a `doctor` dependency.
- **Flag if origin is not pointing at default branch** (cli#6): a future `doctor` rule candidate (a natural fit for the extensible engine); not included here.
