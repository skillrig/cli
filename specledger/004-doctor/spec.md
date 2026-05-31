# Feature Specification: `doctor` — Environment Health & Backing-CLI Readiness

**Feature Branch**: `004-doctor`
**Created**: 2026-05-31
**Status**: Draft
**Input**: User description: "doctor — environment health & backing-CLI readiness (roadmap 005 + 006 combined into one slice). A `skillrig doctor` command: the superset, deterministic, offline health check for a consuming repo that answers 'is this repo's vendored-skill setup sound AND is my environment actually ready to run those skills?' — the operational-lifecycle complement to `verify`. doctor is a rules-based engine, designed so more rules can be added later."

## Overview

The first three slices made a repo *self-describing about where its skills come from* (001 `init`), made the skills it carries *honest* (002 `add` + `verify`), and let a user *find and acquire* skills from a remote library (003 `search` + remote `add` + `index`). The everyday path is now `init` → `search` → `add` → `verify`.

But a skill is rarely self-contained: it often expects a backing CLI to be installed (e.g. a plan-review skill expects `terraform` and an org-internal tool on PATH). `verify` deliberately does **not** check for those — it validates that vendored *content* is byte-honest, nothing about the surrounding environment. So a repo can pass `verify` and still have an agent fail at runtime because the tool a skill needs isn't installed, is the wrong version, or can't be fetched because the user isn't authenticated to the private repo that hosts it. Today nothing surfaces that gap, and the failure shows up late — at agent-run time or in CI — with no actionable diagnosis.

This slice closes that gap with `skillrig doctor`: one **deterministic, offline** health command that answers two questions at once — *is the vendored-skill setup sound* (integrity) and *is my environment actually ready to run those skills* (readiness). It is the operational-lifecycle complement to `verify`: where `verify` is the integrity gate, `doctor` is the "am I ready to work here?" check a developer runs on a fresh checkout and CI runs before letting an agent loose.

`doctor` is built as a **rules-based engine**: each check is an independent, deterministic rule evaluated over the repo and its vendored skills. This first slice ships a foundational rule set (backing-CLI presence, version constraints, authentication, integrity rollup), and the design must let later slices add rules (allowlist, audit, risk signals) without reworking the core.

This slice combines roadmap items **005** (backing-CLI prerequisites — declare + verify) and **006** (`doctor` superset health check). They ship together because the prerequisite-readiness logic has no command home *except* `doctor`: `verify` stays integrity-only by design (it must remain a pure content gate). `doctor` is also where the long-reserved **exit code 3 (prerequisite failure)** finally lands.

## Clarifications

The targeting decisions below were resolved interactively before specification and are treated as firm:

### Session 2026-05-31

- **Q: When a tool is present but its version cannot be deterministically verified (no declarative version source)?** → **PASS with an advisory note.** The tool counts as eligible (does not fail the run); doctor emits an honest "version unverified (no declarative source)" advisory but never guesses by parsing tool output.
- **Q: How does doctor learn a tool's declared version for a constraint check?** → **Read the consumer repo's `mise.toml` `[tools]` pins as data, offline** (no subprocess, no probing the tool itself). "Resolvable via mise" means "listed there."
- **Q: Slice boundary for 005 + 006?** → **One combined `doctor` slice.** The prerequisite-checking logic and the `doctor` command surface ship together.
- **Q: What does doctor check in this first slice?** → All four foundational rules: backing-CLI prerequisites, version constraints, authentication-as-distinct-failure, and an integrity rollup; with mise resolvability folded into presence/version checking.
- **Q: How deep is version-constraint matching?** → Evaluated **only against a declared version source** (the `mise.toml` pin); doctor never executes a tool to discover its version (constitution N6 — no inferential truth).

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Diagnose backing-CLI readiness on a fresh checkout (Priority: P1)

A developer (or an agent, or CI) clones a repo that has vendored skills and wants to know, before doing any work, whether the environment can actually run those skills. They run `skillrig doctor`. The command inspects every vendored skill's declared requirements and reports, per skill and per required tool, whether each tool is present and ready — partitioning skills into **eligible** (every requirement satisfied) and **ineligible** (something missing), each ineligible item carrying a specific, actionable reason. The command exits non-zero when something is genuinely missing, so CI can gate on it.

**Why this priority**: This is the core value of the slice and the smallest standalone MVP. Without it, a repo can pass `verify` and still be unrunnable, and the failure surfaces late and undiagnosed. A single command that turns "the agent mysteriously failed" into "tool X required by skill Y is not installed — here is how to fix it" is the whole point.

**Independent Test**: In a repo with one vendored skill that requires a tool, run `doctor` with the tool absent from PATH (ineligible, actionable reason, non-zero exit) and again with it present (eligible, exit 0). Fully testable on its own; delivers immediate value.

**Acceptance Scenarios**:

1. **Given** a repo with a vendored skill that declares a required tool, **and** that tool is on PATH, **When** the user runs `skillrig doctor`, **Then** the skill is reported eligible and the command exits 0.
2. **Given** the same repo **but** the required tool is absent from PATH and not declared in any version source, **When** the user runs `skillrig doctor`, **Then** the skill is reported ineligible with a reason naming the missing tool and the skill that needs it, and the command exits with the prerequisite-failure code.
3. **Given** a repo with multiple vendored skills with varied requirements, **When** the user runs `skillrig doctor`, **Then** the output partitions skills into eligible and ineligible sets, each ineligible entry stating which specific requirement failed and why.
4. **Given** a repo with vendored skills that declare **no** requirements, **When** the user runs `skillrig doctor`, **Then** every skill is reported eligible and the command exits 0.

---

### User Story 2 — Distinguish "tool missing" from "can't authenticate to fetch it" (Priority: P1)

A developer is onboarding to a repo whose skills require an **org-internal** backing CLI — one hosted in a private repository. The tool isn't installed yet, and fetching it requires authentication. When they run `skillrig doctor`, the command must not lump "you need to install this" together with "you can't even reach the place it lives." If the requirement points at a private source and the user's GitHub authentication is not reachable, `doctor` reports that as its **own distinct, actionable failure** — separate from a plain "tool missing" and separate from "unreachable/not-found."

**Why this priority**: Authentication-versus-missing is the single most common onboarding and CI footgun for private backing CLIs (architecture R18). Conflating the two sends people down the wrong fix path (reinstalling a tool they can't fetch instead of fixing their token). Surfacing it loudly and separately is high-value and inseparable from the prerequisite check, so it ships in the same MVP.

**Independent Test**: In a repo with a skill whose requirement names a private source, run `doctor` with authentication unavailable (distinct "authentication unreachable" failure, its own actionable message) and again with authentication available (the requirement is no longer flagged on auth grounds). The authentication probe is exercised through a stubbed seam — no network.

**Acceptance Scenarios**:

1. **Given** a vendored skill whose requirement names a private (org-internal) source, **and** GitHub authentication is not reachable, **When** the user runs `skillrig doctor`, **Then** the requirement is reported as an authentication failure that is explicitly distinct from "tool missing," names the tool and source, and tells the user how to authenticate, **and** the command exits with the prerequisite-failure code.
2. **Given** the same skill **but** authentication is reachable, **When** the user runs `skillrig doctor`, **Then** the requirement is not flagged on authentication grounds (only presence/version rules apply).
3. **Given** a vendored skill whose requirement names a **public** (external) source, **When** the user runs `skillrig doctor`, **Then** no authentication check is applied to that requirement.

---

### User Story 3 — Verify version constraints only where they can be trusted (Priority: P2)

A skill declares that it needs a tool at a minimum version. The developer wants to know whether their environment satisfies that constraint — but only if `doctor` can determine the answer *deterministically*. When the repo declares the tool's version in a recognized version source (a `mise.toml` pin), `doctor` evaluates the constraint against that declared version and fails the run if it's violated. When there is no declarative version source, `doctor` does **not** guess by inspecting the tool: it reports the tool as present with an advisory that the version is unverified, and the run still passes.

**Why this priority**: Version mismatches are a real readiness failure, but trustworthy version checking depends on a declarative source. Honest "unverified" beats a fragile guess (constitution N6). It builds on US1's presence check and is valuable but secondary to "is the tool even there."

**Independent Test**: In a repo whose skill requires `tool >= X`, run `doctor` three ways: (a) a `mise.toml` pin satisfies the constraint → eligible, exit 0; (b) a `mise.toml` pin violates the constraint → ineligible with a constraint-violation reason, prerequisite-failure exit; (c) no version source, tool on PATH → eligible with a "version unverified" advisory, exit 0.

**Acceptance Scenarios**:

1. **Given** a skill requiring a tool at a minimum version **and** a `mise.toml` pin that satisfies it, **When** the user runs `skillrig doctor`, **Then** the requirement passes and the command exits 0.
2. **Given** a skill requiring a tool at a minimum version **and** a `mise.toml` pin that violates it, **When** the user runs `skillrig doctor`, **Then** the requirement fails with a reason stating the declared version, the required constraint, and the skill that needs it, **and** the command exits with the prerequisite-failure code.
3. **Given** a skill requiring a tool at a minimum version **and** no declarative version source, **but** the tool is present, **When** the user runs `skillrig doctor`, **Then** the requirement passes with an advisory that the version is unverified, **and** the command exits 0.
4. **Given** a tool that is declared in `mise.toml` but not on PATH, **When** the user runs `skillrig doctor`, **Then** the tool counts as present (resolvable via the declared source) for the presence rule.

---

### User Story 4 — One health command that also covers integrity (Priority: P2)

A developer wants a single "is everything OK here?" command rather than running `verify` and a separate readiness check. `skillrig doctor` rolls the existing integrity check (the same one `verify` performs) into its report: it reports whether vendored skills are byte-honest and free of orphans, alongside the readiness findings. The integrity result and the readiness result are both reflected in the report and in the exit code.

**Why this priority**: Makes `doctor` the genuine "superset health check" the roadmap describes and gives users one command to trust. It depends on the readiness rules existing first, so it's P2 — but it reuses the already-shipped integrity primitive, so it's low-cost.

**Independent Test**: In a repo with a tampered (or orphaned) vendored skill, run `doctor`; the integrity problem appears in the report and drives the integrity-failure exit code, while readiness findings are reported alongside. In a clean, ready repo, `doctor` reports both integrity-OK and readiness-OK and exits 0.

**Acceptance Scenarios**:

1. **Given** a repo whose vendored content fails the integrity check (a mismatch or orphan), **When** the user runs `skillrig doctor`, **Then** the integrity problem is reported and the command exits with the integrity-failure code.
2. **Given** a repo that is both integrity-clean and fully ready, **When** the user runs `skillrig doctor`, **Then** the report shows integrity-OK and readiness-OK and the command exits 0.
3. **Given** a repo that is integrity-clean but has a missing required tool, **When** the user runs `skillrig doctor`, **Then** both facts are reported and the exit code reflects the prerequisite failure per the defined precedence.

---

### User Story 5 — Machine-readable health for agents and CI (Priority: P3)

An agent or CI pipeline runs `skillrig doctor --json` and consumes a complete, untruncated health report: every skill, every requirement, each rule's verdict and reason, the integrity verdict, and the authentication status. The consumer decides what to extract; nothing is truncated or summarized away. Human output stays compact with a footer hint pointing to `--json`/`--verbose` for the full picture.

**Why this priority**: Agents and CI are first-class callers (this is a CLI for humans, agents, and CI alike), and the two-level output contract is a project-wide requirement. It's P3 only because the human path already delivers the core value; the JSON shape is additive and mechanical once the rule results exist.

**Independent Test**: Run `doctor --json` against a mixed repo and assert the output is parseable and structurally complete (all skills, all requirements, all rule verdicts, integrity verdict, auth status present); run the human form and assert bounded, compact output with a footer hint.

**Acceptance Scenarios**:

1. **Given** any repo state, **When** the user runs `skillrig doctor --json`, **Then** the output is valid, parseable, and contains the complete per-skill/per-requirement rule results, the integrity verdict, and the authentication status, with no truncation.
2. **Given** any repo state, **When** the user runs `skillrig doctor` (human form), **Then** the output is compact (bounded), groups skills into eligible/ineligible, and ends with a footer hint pointing at the fuller `--json`/`--verbose` views.
3. **Given** a failing run, **When** the user adds `--verbose`, **Then** the raw underlying causes behind each failure are printed (the escape hatch), and errors go to stderr while report data goes to stdout.

---

### Edge Cases

- **No origin configured / not in a project.** `doctor` is an Environment-pattern command and must still run and report what it can (e.g. integrity over what's on disk, readiness over vendored manifests) rather than hard-failing on a missing origin. A missing origin is reported, not fatal, where the checks don't strictly need it.
- **No vendored skills at all.** `doctor` reports a healthy/empty state and exits 0 (idempotent, nothing to check).
- **A vendored skill declares no requirements.** It is trivially eligible; only the integrity rule applies to it.
- **A requirement names a tool but no version constraint.** Presence rule applies; version rule is a no-op (not a failure).
- **A `mise.toml` exists but does not list the required tool.** That tool falls back to the PATH presence check; absence there is a "tool missing," not a version failure.
- **A `mise.toml` is malformed or unreadable.** Treated as "no declarative version source" for version purposes and recorded as a diagnostic surfaced under `--verbose`; it does not crash the run (consistent with how malformed config is handled elsewhere — skipped, recorded, never fatal).
- **A requirement's version constraint is itself malformed/unparseable** in the skill manifest. Reported as a requirement-level problem with an actionable message naming the offending skill; it must not silently pass.
- **Multiple failure classes in one run** (e.g. an integrity mismatch *and* a missing tool). The exit code follows a single deterministic precedence; the report shows all findings regardless of which class wins the exit code.
- **A requirement names a private source but the source identity is ambiguous/unparseable.** Reported as an actionable requirement-level problem rather than silently skipping the auth check.
- **Running in an environment with no GitHub auth tooling at all** (no token env vars, no `gh`). For public requirements this is irrelevant; for private requirements it surfaces as the distinct authentication failure (US2), never as a crash.

## Requirements *(mandatory)*

### Functional Requirements

**Command surface & classification**

- **FR-001**: The system MUST provide a `skillrig doctor` command that runs a deterministic, offline health check over the current consumer repo and exits with a status code reflecting the outcome.
- **FR-002**: `doctor` MUST be idempotent and safe to retry (Environment pattern): repeated runs against an unchanged repo produce the same report and exit code, and the command performs no mutations.
- **FR-003**: `doctor` MUST function without a fully-configured project — it reports what it can (and records what it cannot check) rather than hard-failing on absent configuration such as a missing origin.
- **FR-004**: `doctor` MUST be implemented as a rules-based engine in which each check is an independent, deterministic rule evaluated over the repo and its vendored skills, structured so that additional rules can be added later without reworking the core or the output contract.

**Readiness rule: backing-CLI presence (roadmap 005)**

- **FR-005**: For every vendored skill, `doctor` MUST read that skill's declared tool requirements from its vendored manifest (the single on-disk source of truth) so the check runs offline.
- **FR-006**: For each declared requirement, `doctor` MUST determine whether the required tool is **present**, where "present" means available on the executable search path OR declared in a recognized version source in the consumer repo.
- **FR-007**: `doctor` MUST partition vendored skills into **eligible** (every requirement satisfied) and **ineligible** (one or more requirements unmet), and for each ineligible skill MUST state, per failing requirement, exactly what failed and why, naming the tool and the skill that needs it.
- **FR-008**: A vendored skill that declares no requirements MUST be treated as eligible by the readiness rules (only integrity applies to it).

**Readiness rule: version constraints (roadmap 005)**

- **FR-009**: When a requirement declares a version constraint AND a recognized declarative version source provides a version for that tool, `doctor` MUST evaluate the constraint against the declared version and treat a violation as a prerequisite failure with a reason stating the declared version, the required constraint, and the skill that needs it.
- **FR-010**: `doctor` MUST NOT determine a tool's version by executing the tool or otherwise inferring it from runtime output; version evaluation MUST rely solely on a declarative version source (constitution N6 — no inferential truth).
- **FR-011**: When a requirement declares a version constraint but no declarative version source provides a version, AND the tool is present, `doctor` MUST report the requirement as satisfied with an advisory that the version is unverified, and MUST NOT fail the run on that basis.
- **FR-012**: `doctor` MUST read declared tool versions from the consumer repo's `mise.toml` as structured data (offline, no subprocess); a tool listed there counts as both present (FR-006) and version-determinable (FR-009).
- **FR-013**: A `mise.toml` that is absent, malformed, or unreadable MUST be treated as "no declarative version source" (not a fatal error), with any parse problem recorded as a diagnostic surfaced under `--verbose`.

**Readiness rule: authentication as a distinct failure (R18)**

- **FR-014**: For a requirement whose declared source identifies a **private** (org-internal) origin, `doctor` MUST check whether read authentication to that source is reachable, and MUST report an authentication problem as a failure class **distinct** from "tool missing" and from "unreachable/not-found."
- **FR-015**: An authentication failure MUST be actionable — naming the tool and source and telling the user how to authenticate — and MUST be surfaced prominently (it is the top onboarding/CI footgun).
- **FR-016**: For a requirement whose declared source is **public/external**, `doctor` MUST NOT apply an authentication check.
- **FR-017**: The authentication reachability check MUST be deterministic and offline-friendly (it probes local credential availability, not the live remote), so `doctor` remains a no-network command.

**Integrity rollup (roadmap 006 superset)**

- **FR-018**: `doctor` MUST run the existing vendored-content integrity check (label-honesty + orphan detection) and fold its verdict into the health report, using the same shared integrity implementation that `verify` uses (one implementation, never a parallel copy — AP-04).
- **FR-019**: `verify` MUST remain integrity-only; `doctor` MUST NOT change `verify`'s behavior or scope.

**Exit codes & precedence**

- **FR-020**: `doctor` MUST use the project exit-code contract: `0` healthy (including advisory-only notes), `1` usage/configuration error, `2` integrity failure, `3` prerequisite failure (a missing required tool, a declared-and-violated version constraint, or an unauthenticated private source).
- **FR-021**: When more than one failure class is present in a single run, `doctor` MUST select the exit code by a single, documented, deterministic precedence, while the report still shows findings from all classes.
- **FR-022**: `doctor` MUST be the only command that emits exit code 3; this slice introduces the first real use of the previously-reserved prerequisite-failure code.

**Output contract (two-level, errors-as-navigation)**

- **FR-023**: `doctor` MUST provide a compact human report that groups skills into eligible/ineligible, summarizes each finding without dumping full detail, and ends with a footer hint pointing to the fuller views.
- **FR-024**: `doctor` MUST provide a `--json` form that is complete and untruncated: every skill, every requirement, each rule's verdict and reason, the integrity verdict, and the authentication status, suitable for an agent or CI to parse and branch on.
- **FR-025**: `doctor` MUST support `--verbose` as the escape hatch that prints the raw underlying causes behind failures; errors MUST go to stderr and report data to stdout.
- **FR-026**: Every `doctor` failure message MUST follow errors-as-navigation: state what failed, the real (never-swallowed) underlying cause, and a suggested fix.

**Co-evolution & docs**

- **FR-027**: The consolidated `skillrig` agent skill MUST be extended to cover `doctor` (a `doctor` activity reference plus updated routing and triggering keywords), with no new top-level skill created, and its triggering accuracy validated.
- **FR-028**: The CLI design contract and the roadmap MUST be updated in the same change to reflect `doctor` (its classification, flags, exit-code-3 introduction, and that roadmap 005 + 006 are delivered by this slice), and the architecture document kept in sync as the design is refined.

### Key Entities *(include if feature involves data)*

- **Requirement**: One declared dependency of a vendored skill — the tool name to look for, an optional version constraint, the source it comes from (which determines public-vs-private and thus whether authentication applies), and the manager that provisions it (which signals whether a declarative version source can be trusted). Parsed from the vendored skill manifest; never written to the lockfile (the manifest is the single source of truth).
- **Health Report**: The complete result of a `doctor` run — per-skill eligibility (eligible/ineligible), per-requirement rule verdicts with reasons and advisories, the integrity verdict, and the authentication status — rendered compactly for humans and completely for `--json`.
- **Rule**: An independent, deterministic check evaluated by the engine (presence, version, authentication, integrity), each producing a verdict (pass / fail / advisory) and an actionable reason; the rule set is extensible.
- **Declarative Version Source**: A recognized, data-readable declaration of a tool's version in the consumer repo (the `mise.toml` `[tools]` pins) — the only trusted basis for version-constraint evaluation.
- **Eligibility Partition**: The grouping of vendored skills into those whose every requirement is satisfied and those with at least one unmet requirement, each ineligible entry carrying its specific failing reason(s).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user running `skillrig doctor` on a repo with a missing required backing CLI learns, from a single command, exactly which tool is missing and which skill needs it — without consulting any other tool or documentation.
- **SC-002**: For any given repo state, `doctor` produces the same verdict and exit code on every run (fully deterministic) and touches no files (no mutations), verified by repeated runs.
- **SC-003**: `doctor` completes its checks using only local data — no network access is required or attempted — so it runs identically offline, in CI, and air-gapped.
- **SC-004**: When a required private backing CLI cannot be authenticated, `doctor` reports an authentication failure that is unambiguously distinct from "tool missing," so a user takes the correct corrective action (fix credentials, not reinstall) on the first attempt.
- **SC-005**: `doctor` never reports a version verdict it cannot determine deterministically: every version pass/fail traces to a declarative source, and every undeterminable version is reported as an explicit "unverified" advisory rather than a guess.
- **SC-006**: CI can gate on `doctor` purely by its exit code — `0` for a healthy/ready repo, `2` for an integrity problem, `3` for a prerequisite problem — and the mapping is stable across runs.
- **SC-007**: An agent or CI consumer can extract any individual finding (a specific skill's eligibility, a specific requirement's verdict, the integrity result, the auth status) from `doctor --json` without the data being truncated or summarized away.
- **SC-008**: The human form of `doctor` stays compact (bounded output) regardless of how many skills/requirements are present, while the `--json` form remains complete — verified by output-shape assertions, not substring matching.

### Previous work

These merged slices established the primitives `doctor` builds on:

- **001 — `init` + origin resolution**: the single origin resolver and the Environment-pattern command conventions `doctor` follows. (Epic `SL-227789`.)
- **002 — `skillcore` + local `add` + `verify`**: the shared integrity primitive (label-honesty + orphan detection) `doctor` rolls up, and the manifest data model carrying tool requirements (parsed, not yet consumed).
- **003 — `search` + remote `add` + `index`**: the migration to `SKILL.md` frontmatter where requirements are declared (`metadata.x-skillrig.requires`), and the auth/token resolution pattern (`os.exec` of `gh`/token env) reused for the authentication-reachability rule.

Open tracked items related to but **out of scope** for this slice:

- **Vendor more skills into the fixture origin** (cli#9): broadens multi-skill end-to-end coverage; useful test substrate but not a `doctor` dependency.
- **Flag if origin is not pointing at default branch** (cli#6): a future `doctor`-adjacent rule candidate; not included here.
