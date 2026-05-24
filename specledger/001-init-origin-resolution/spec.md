# Feature Specification: CLI Initialization & Origin Resolution

**Feature Branch**: `001-init-origin-resolution`
**Created**: 2026-05-24
**Status**: Draft
**Input**: User description: "Build `skillrig init` and the single origin-resolution primitive for the generic skillrig CLI."

## Overview

skillrig is a single generic binary shared by every org; it carries **no baked-in origin**. The "origin" — the org's private git monorepo that is the source of truth for its skills (e.g. `my-org/my-skills`) — is supplied by the consumer at runtime. This feature delivers the two foundational capabilities every later command depends on:

1. A single, shared way to **resolve which origin a command should act against**, using a documented precedence order.
2. A **`skillrig init`** command that records the chosen origin into committed (or global) configuration, so a repo becomes self-describing for any later human, agent, or CI run.

This is the first slice of the CLI, so it also establishes the baseline command experience (self-documenting help, actionable errors, machine-readable output, meaningful exit codes) that all subsequent commands inherit.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Bind a repo to an existing origin (Priority: P1)

A developer (or an agent acting on their behalf) is in a repository that should consume skills from their org's library. They run a single command naming the origin, and the tool records it in a committed config file. From then on, anyone who clones the repo — a teammate, an agent, a CI job — knows the origin with no additional setup.

**Why this priority**: This is the entry point to the entire product. Without a recorded origin, no other command (search, add, verify) has a target. It is the smallest slice that delivers standalone value: a repo that declares where its skills come from.

**Independent Test**: In an empty repo, run the bind command with an origin argument; confirm a config file is written containing exactly that origin, the command reports success, and re-running is safe. No network or other command is required.

**Acceptance Scenarios**:

1. **Given** a repo with no skillrig config, **When** the developer binds it to `my-org/my-skills`, **Then** a project config file is created recording `my-org/my-skills` as the origin and the command reports success (success exit status).
2. **Given** a repo already bound to `my-org/my-skills`, **When** the developer re-runs the same bind command, **Then** the outcome is unchanged (idempotent) and the command still reports success — no error, no duplicate or corrupted config.
3. **Given** a repo already bound to `my-org/my-skills`, **When** the developer binds it to a different origin `other-org/other-skills`, **Then** the config is updated to the new origin and the change is reported.
4. **Given** the developer requests machine-readable output, **When** the bind succeeds, **Then** the tool emits structured output that names the origin written and the config location, and that output is complete and parseable.
5. **Given** the developer wants to set their personal default rather than bind a single repo, **When** they run the bind command in global mode, **Then** the origin is recorded in the per-user global default location instead of the repo, and the repo's own config is not modified.

---

### User Story 2 - Resolve the origin by precedence (Priority: P2)

A contractor works across multiple client repos. Repo A should always target client-A's origin and repo B client-B's, regardless of which directory they happen to be in, while they keep a personal default for ad-hoc work. CI needs to force a specific origin for an ephemeral job without touching committed files. Every command must agree on which origin is in effect, resolved the same way everywhere.

**Why this priority**: Correct, predictable resolution is what makes the recorded config trustworthy. It is required before any consuming command can act, but it builds on the config written in P1, so it is second.

**Independent Test**: Construct combinations of the environment override, a project config, and a global default; confirm the resolved origin always follows the documented precedence and that lower-priority sources fill in only when higher ones are absent.

**Acceptance Scenarios**:

1. **Given** a project config naming `my-org/my-skills` and no environment override, **When** the origin is resolved, **Then** the result is `my-org/my-skills`.
2. **Given** a project config naming `my-org/my-skills` **and** an environment override naming `ci-org/ci-skills`, **When** the origin is resolved, **Then** the result is `ci-org/ci-skills` (environment override wins).
3. **Given** no environment override and no project config, but a global default naming `personal/skills`, **When** the origin is resolved, **Then** the result is `personal/skills`.
4. **Given** repo A with a project config for `client-a/skills` and repo B with a project config for `client-b/skills`, **When** the origin is resolved from within each repo, **Then** each resolves to its own origin independently, even with a different personal global default set.

---

### User Story 3 - Actionable failure when no origin is configured (Priority: P3)

A developer or agent runs a command that needs an origin in a repo that was never bound and with no override or global default set. Instead of a cryptic failure, they get a message that states what failed, why, and exactly what to do next.

**Why this priority**: It hardens the experience and prevents wasted agent cycles, but the happy paths (P1, P2) must exist first.

**Independent Test**: With no environment override, no project config, and no global default, trigger origin resolution; confirm a non-zero exit status and an error that names the missing origin and offers concrete next steps.

**Acceptance Scenarios**:

1. **Given** no origin configured in any source, **When** a command requiring an origin runs, **Then** it exits with a usage/config error status and prints — to the error stream — (a) what failed, (b) that no origin is configured in any source, and (c) at least two concrete fixes (bind the repo, or set the environment override).
2. **Given** the bind command is invoked with no origin argument and the session is non-interactive, **When** it runs, **Then** it exits with a usage/config error status and explains how to supply the origin.
3. **Given** an origin value that is not in the expected `OWNER/REPO` shape, **When** it is supplied, **Then** the tool rejects it with an error naming the expected format and showing the offending value, without writing config.

---

### Edge Cases

- **Malformed config file**: a project or global config file exists but is unparseable or missing the origin field → resolution treats it as "no origin from this source" and the error/precedence behavior surfaces a clear message rather than a raw parse dump.
- **Whitespace / case**: surrounding whitespace in an origin value is trimmed; the `OWNER/REPO` shape check is applied to the trimmed value.
- **Both project and global exist**: project config takes precedence over global default (lower source fills only what higher omits).
- **Global mode in a repo**: requesting global mode writes only the per-user default and never the repo config, even when run inside a repo.
- **Re-bind to identical value**: writing the same origin again produces no spurious diff/error (idempotent).
- **Config directory does not yet exist**: the tool creates the necessary config directory/file on first bind.
- **Empty environment override**: an override set to an empty/blank value is treated as unset, not as an invalid origin.

## Requirements *(mandatory)*

### Functional Requirements

**Origin resolution**

- **FR-001**: The system MUST resolve the active origin from exactly one shared resolution path used by every command (no per-command re-derivation), so resolution is identical everywhere.
- **FR-002**: Resolution MUST follow this precedence, highest first: (1) environment override, (2) project config in the repo, (3) per-user global default. A lower-priority source supplies the origin only when all higher-priority sources are absent or empty.
- **FR-003**: When no source provides an origin, resolution MUST report a distinct "no origin configured" outcome (not a silent empty value) that callers can turn into an actionable error.
- **FR-004**: Resolution MUST treat an unparseable or origin-less config file as "no origin from that source" and continue down the precedence order, surfacing a clear diagnostic rather than a raw parser error.

**Initialization (`init`)**

- **FR-005**: The system MUST provide a command that records a chosen origin into the repo's committed project config.
- **FR-006**: The command MUST accept the origin as an explicit argument so it can run non-interactively (suitable for agents and scripts).
- **FR-007**: The command MUST support a global mode that records the origin as the per-user default instead of writing the repo config.
- **FR-008**: The command MUST be idempotent: re-running with the same origin leaves an equivalent config and reports success without error.
- **FR-009**: The command MUST update the recorded origin when invoked with a different value, replacing the prior value cleanly.
- **FR-010**: The command MUST create any missing config directory/file needed to store the origin.
- **FR-011**: The command MUST bind only to an origin the consumer supplies; it MUST NOT create, scaffold, or bootstrap an origin repository. (Standing up an origin is out of scope — see Out of Scope.)
- **FR-012**: The command MUST validate that a supplied origin matches the expected `OWNER/REPO` shape and reject malformed values without writing config.

**Command experience (baseline conformance, inherited by all later commands)**

- **FR-013**: Every command and subcommand MUST provide self-documenting help that includes at least two usage examples, sufficient to construct a correct invocation without external docs.
- **FR-014**: Every error MUST state (a) what failed, (b) the real underlying reason (never swallowed or replaced by a guess), and (c) at least one concrete suggested fix.
- **FR-015**: Error and diagnostic output MUST go to the error stream; primary data output MUST go to the standard output stream, so output can be cleanly piped.
- **FR-016**: The system MUST offer a machine-readable output mode whose output is complete (no truncation) and parseable.
- **FR-017**: The system MUST use distinct, stable exit statuses: success on success; a usage/config error status for bad arguments, malformed origin, or no-origin-configured. (Integrity/prerequisite failure classes are reserved for later commands and out of scope here.)

### Key Entities *(include if feature involves data)*

- **Origin reference**: an identifier for the org's skill source in `OWNER/REPO` form (e.g. `my-org/my-skills`). The single value this feature reads, validates, records, and resolves.
- **Project config**: per-repo, committed configuration that records the repo's origin so the repo is self-describing to any clone/agent/CI. Hand-editable input.
- **Global default config**: per-user configuration recording the developer's default origin, used when a repo has no project config.
- **Environment override**: a runtime-supplied origin (highest precedence) for ephemeral/CI use that does not touch any file.
- **Resolution result**: the outcome of resolving the active origin — either a resolved origin plus which source it came from, or a clear "none configured" state.

## Out of Scope

The following are explicitly **not** part of this feature and MUST NOT be pulled in:

- Any consuming command: search, add, verify, bump, doctor, global add/verify, lint.
- Discovery artifacts (`index.json`), lockfiles (`skills-lock.json`), tree-SHA / integrity primitives.
- Any network or git fetching, cloning, or authentication. This feature is offline config bootstrap only.
- Bootstrapping or scaffolding an origin repository (that is the GitHub template's job).
- Integrity (content-mismatch) and prerequisite (backing-CLI) failure classes and their exit statuses.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can bind a fresh repo to an origin with a single command invocation and zero prior configuration.
- **SC-002**: After binding, a freshly cloned copy of the repo (or an agent/CI run in it) resolves the same origin with no additional setup steps.
- **SC-003**: Origin resolution returns the correct result for 100% of the documented precedence combinations (override / project / global / none), verified by executable scenarios.
- **SC-004**: Every failure path a user can hit in this feature (no origin configured, missing origin argument, malformed origin) produces a message that names the problem and at least one concrete next step — 0 cryptic/raw-only errors.
- **SC-005**: Re-running the bind command with an unchanged origin produces no change and no error (idempotent) on every repeat.
- **SC-006**: Each command's help output alone is sufficient for a first-time user or agent to construct a correct invocation (contains purpose plus ≥2 examples).

## Constitution Alignment *(skillrig-specific)*

This feature is the first to exercise the project constitution; the following must hold and be carried into planning:

- **II — Quickstart-as-Contract**: `quickstart.md` scenarios will be authored as executable steps (concrete `skillrig init …` invocations, observable config contents, exit statuses) mapping 1:1 to integration tests. Output-shape assertions are required: machine-readable output must be parseable and structurally complete; error output must be checked for its three parts (what failed / why / fix) as distinct assertions plus the correct exit status — not a single substring match.
- **III — Ground-Truth Anchoring**: the recorded config format and the precedence resolution table must be anchored to a real captured sample (an actual written config file and a recorded resolution matrix), not invented from the spec.
- **IX — Skill–CLI Co-Evolution**: this CLI capability ships with a corresponding agent skill update whose description reflects how users actually phrase "point this repo at our skills library / set the origin," including the no-origin failure mode.

## Dependencies & Assumptions

**Assumptions**:

- The origin identifier shape is `OWNER/REPO` (two non-empty, slash-separated segments); deeper validation (does the repo exist / is it reachable) is deliberately deferred because this feature is offline.
- Config is stored as a small, hand-editable file at a project-scoped location and a per-user global location, consistent with the architecture's `config.toml` decision; the input config and any tool-written output (lockfiles) are separate concerns (lockfiles are out of scope here).
- A developer running global mode wants only the per-user default written, never the repo config.

**Dependencies**:

- Conventions are governed by the project's design docs (`docs/design/cli.md`) and the architecture's origin-discovery section. If those references should be tracked as external reading for this spec, add them with `sl deps add`.

### Previous work

No prior skillrig features or issues exist — this is the first feature spec in the project (`sl issue list --all` returned no issues).
