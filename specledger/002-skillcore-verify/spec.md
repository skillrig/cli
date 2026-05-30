# Feature Specification: Vendor & Verify Skills (`add` + `verify`)

**Feature Branch**: `002-skillcore-verify`
**Created**: 2026-05-29
**Status**: Draft
**Input**: User description: "Implement `skillcore` + `verify` — git tree-SHA + `skill.toml` manifest parse; offline label-honesty + orphan check; exit codes 0/2/3 from docs/ARCHITECTURE-v0.md"

> **Technical companion**: [spec-tech-spike.md](./spec-tech-spike.md) holds the implementation-level decisions (the shared-core primitives, tree-SHA mechanics, lock schema, exit-code mapping, the clarification session that reshaped scope). This spec stays user-facing; the spike is the input to `/specledger.plan`.

## Overview

This feature delivers the product's core promise — **"the skill your agent runs is exactly the version that was reviewed and approved"** — as something a user can do end-to-end, offline:

1. A user **vendors a skill** from their org's library into their repo, recording its exact identity (`add`).
2. Anyone — CI, an agent, or a human — can later **prove the vendored skills are exactly what was recorded**, and that nothing untracked has slipped in (`verify`).

Both verbs sit on a single shared trust primitive (the content fingerprint), so the value written when a skill is vendored and the value checked at verification time are computed the same way and cannot drift apart. That shared primitive is invisible to users; its user-meaning is simply that **the gate cannot lie**.

This is the second slice of the CLI. The first (`001-init-origin-resolution`) made a repo self-describing about *where* its skills come from; this slice makes the skills it carries *honest*.

## Clarifications

### Session 2026-05-29

- Q: The input lists exit codes 0/2/3, but names only label-honesty + orphan checks (both the integrity/exit-2 class). Exit 3 is the *prerequisite* class. Does this slice check backing-CLI prerequisites? → A: **No.** Prerequisite / eligibility checking (does the agent have the backing CLIs a skill needs to *run*?) belongs to a later `doctor` capability, not to `verify`. `verify` is integrity-only and uses exit statuses `0/1/2`; the prerequisite class (exit 3) is deferred. (`docs/ARCHITECTURE-v0.md` was corrected to reflect this — see the spike §8.)
- Q: Why not have `verify` warn-or-fail on prerequisites depending on caller? → A: That framing was rejected. The CI gate validates *content* and needs no backing binaries installed; the caller that actually needs prerequisites present is the runtime agent. So prerequisite/eligibility lives where the agent asks for it (`doctor`), and the content gate stays free of it. Practical consequence: a CI run of `verify` never fails because a backing tool is missing.
- Q: `verify` needs something to verify against, but the lock's writers weren't built yet — fixtures only, or a real producer? → A: A real producer. **`skillrig add` (vendoring from a local copy of the origin) is in scope**, so the `add → verify` round-trip is the acceptance contract and the recorded fingerprint is genuine, not hand-authored. `verify` itself remains read-only.
- Q: Is vendoring-from-a-local-path a throwaway test affordance or a real capability? → A: A **durable capability** — consuming from a local checkout of the org library is a legitimate, kept use case. Fetching directly from a remote origin is a *later, additive* mode, not a replacement for it.

### Session 2026-05-30

- Q: How does `add` obtain its source — an explicit `--from`/path argument, or the configured origin? → A: **The configured origin.** `add` resolves the active origin through the shared resolver (env > project config > global), exactly like every command — there is **no** separate source argument that bypasses it. For this feature the resolved origin may be a **local** source (a local checkout); tests run `skillrig init --origin <local-origin>` first, then `skillrig add <skill>`. This keeps a single-origin contract (the earlier `--from` idea is dropped). Remote GitHub-hosted origins + auth are a later, additive mode (production lean: GitHub-only).
- Q: When `add` re-vendors a skill whose on-disk content diverges from the record (local edits), does it three-way-merge? → A: **No — it detects and refuses without `--force`.** A true three-way merge needs an *upstream-advanced* axis (base/theirs/ours); re-vendoring the **same** version has no such axis, so there is nothing to merge — that belongs to a later `bump`. `add` here only refuses to clobber divergent content (override with `--force`); `verify` independently flags the divergence as a label-honesty mismatch.
- Q: Should `verify` detect unresolved git conflict markers as a distinct failure now? → A: **Deferred.** A skill file containing conflict markers already fails label-honesty (its fingerprint won't match the record), so detection only upgrades the *error message*, not correctness; and the producer of such markers (`bump`'s merge) does not exist yet. The distinct check lands with `bump`.
- Q: Does this feature materialize multi-client symlink views (e.g. `.claude/skills → ../.agents/skills`) and agent-shell selection? → A: **No — canonical only.** `add` writes only the canonical `.agents/skills/<skill>`. Multi-client symlink views and the `init`-time agent-shell selection (stored in `.skillrig/config.toml`) are a separate, later feature. `verify`'s orphan check therefore scans only the canonical location.
- Q: Should the record (lock) mirror each skill's `[[requires]]` backing-CLI declarations? → A: **No.** The full skill subtree — including its `skill.toml` manifest — is vendored on disk and fingerprint-attested, so the vendored manifest is the single source of truth for prerequisites; a later health command reads it directly. Mirroring into the record would only duplicate data that can drift. (This diverges from architecture §4.2's "mirror requires for offline prereq check" rationale, which assumed the manifest might not be on disk — see spike.)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Vendor an approved skill into my repo (Priority: P1)

A developer (or an agent acting for them) has pointed this repo at their org's skill library with `init` (the origin may be a local checkout). They run a single command naming a skill, and the tool vendors it from the configured origin. The skill's files are placed into the repo's canonical skills location, and its exact identity — which version, where it came from, and a tamper-evident fingerprint of its content — is recorded in a committed record file. The repo now carries both the skill and proof of what it is.

**Why this priority**: Nothing can be verified until something has been vendored and recorded. This is the producer half of the promise and the smallest standalone slice that delivers value: a repo gains an approved skill plus a durable record of its identity. Consuming from a local copy of the library is itself a real, supported workflow (offline, air-gapped, or pre-cloned origins).

**Independent Test**: In a git repo whose origin is a local sample library, run the vendor command for a named skill; confirm the skill's files land in the canonical skills location (`.agents/skills/<skill>`) and the record file gains an entry naming the skill's version, source, and content fingerprint. No network is involved.

**Acceptance Scenarios**:

1. **Given** a git repo pointed at a skill library (its origin — which may be a local checkout) and no vendored skills, **When** the developer vendors a named skill, **Then** the skill's files appear under the canonical skills location (`.agents/skills/<skill>`) and the record file contains one entry for it (version, source/provenance, and a content fingerprint), and the command reports success.
2. **Given** a skill already vendored from a library, **When** the developer vendors the identical content again, **Then** the outcome is unchanged (idempotent) and the command reports success — no duplicate or corrupted record.
3. **Given** the developer requests machine-readable output, **When** the vendor command succeeds, **Then** the tool emits structured output naming the skill, its recorded version, and where it was placed, and that output is complete and parseable.
4. **Given** a skill is already vendored and the developer has locally changed its files, **When** they vendor the same version again, **Then** the tool detects the divergence from the recorded fingerprint, refuses to silently overwrite it, and requires an explicit override (`--force`) — local edits are never lost without intent. (Merging local edits with an *upstream advance* is a later `bump` concern; re-vendoring the same version has no upstream change to merge.)
5. **Given** the developer wants to preview only, **When** they run the vendor command in dry-run mode, **Then** the tool reports what it *would* place and record, and writes nothing.

---

### User Story 2 - Prove a vendored skill is exactly what was approved (Priority: P1)

A reviewer, a CI job, or an agent needs assurance that the skills checked into a repo are exactly the approved versions and have not been altered to claim a version they aren't. They run a single verification command. It recomputes each vendored skill's content fingerprint and compares it to the recorded value. If every skill matches, the command passes. If any skills' content diverges from what their record claims, the command fails and produces a full report naming **every** offending skill and its discrepancy — it never stops at the first failure.

**Why this priority**: This is the core product promise made checkable. It is the reason the feature exists: a long skill file can hide a change no human reviewer would catch by eye, and this turns "is this really the approved version?" into a deterministic pass/fail. It builds on US1 (something must be vendored and recorded first), so the two together form the minimum viable round-trip.

**Independent Test**: With at least one vendored-and-recorded skill, run verify and confirm it passes; then alter a single byte of a vendored skill file and confirm verify fails with a non-zero status that names the skill and reports a content mismatch.

**Acceptance Scenarios**:

1. **Given** a repo whose vendored skills all match their recorded fingerprints, **When** verify runs, **Then** it reports success with a success exit status and a summary of how many skills were verified.
2. **Given** one or more vendored skills whose content has been modified so they no longer match their recorded fingerprint, **When** verify runs, **Then** it exits with the verification-failure status and names **every** such skill, each with its recorded-vs-actual discrepancy, in a single aggregated report (it does not exit on the first failure).
3. **Given** the verification runs entirely on the committed files with no network or external service, **When** it is run repeatedly with unchanged inputs, **Then** it returns the same result every time (deterministic, offline).
4. **Given** a repo with no vendored skills and no record file, **When** verify runs, **Then** it reports success (nothing to verify) rather than an error.

---

### User Story 3 - Catch a skill that's untracked or missing (Priority: P2)

A security-conscious reviewer worries about a skill that was added to the repo without going through the record — an untracked skill that could quietly instruct an agent to do something unreviewed — or, conversely, a recorded skill whose files have gone missing. Running verify covers the *whole* set of skills on disk against the recorded set, not only the ones listed, so neither an extra unrecorded skill nor a missing recorded one can pass unnoticed.

**Why this priority**: It closes the highest-severity supply-chain gap the design calls out (an untracked skill is the primary attack vector), and it makes "everything present is accounted for" part of the gate. It depends on the matching machinery from US2, so it follows it.

**Independent Test**: Starting from a passing repo, (a) add a skill directory that has no record entry and confirm verify fails identifying it as untracked; (b) separately, remove a recorded skill's files and confirm verify fails identifying it as missing.

**Acceptance Scenarios**:

1. **Given** a skill directory present in the repo's skills location with no corresponding record entry, **When** verify runs, **Then** it exits with the verification-failure status and identifies the untracked (orphan) skill.
2. **Given** a record entry for a skill whose files are absent from the repo, **When** verify runs, **Then** it exits with the verification-failure status and identifies the missing skill.
3. **Given** multi-client compatibility views are **not** created by this feature (deferred — see Out of Scope and FR-011), **When** verify runs, **Then** the orphan/completeness check scans only the canonical skills location (`.agents/skills`); robust handling of any manually-created view directories is deferred together with multi-client materialization.
4. **Given** both a content mismatch (US2) and an untracked skill are present, **When** verify runs, **Then** it fails and reports both classes of problem rather than stopping at the first.

---

### User Story 4 - Branch on the outcome deterministically (Priority: P3)

An automated caller — a CI merge gate or an agent deciding its next step — needs to act on the verification result without parsing prose. It relies on stable exit statuses to branch (proceed vs. block) and on complete machine-readable output to report *which* skills failed and why. When something is wrong, the message states what failed, the real reason, and a concrete fix.

**Why this priority**: It hardens the gate for its primary non-human callers and makes the result composable in pipelines, but the happy and failure paths (US1–US3) must exist first.

**Independent Test**: Trigger each outcome — all-pass, content mismatch, untracked/missing — and confirm each yields its documented exit status and a structurally complete machine-readable verdict; confirm every failure message contains what failed, why, and a suggested fix, and that diagnostics go to the error stream while data goes to standard output.

**Acceptance Scenarios**:

1. **Given** any verification outcome, **When** verify runs, **Then** it returns a stable exit status distinguishing success, verification failure, and usage/config error, consistent across repeated runs.
2. **Given** machine-readable output is requested, **When** verify runs, **Then** the output is complete (every checked skill with its per-skill verdict) and parseable, for both passing and failing runs.
3. **Given** a verification failure, **When** the result is reported, **Then** the message names what failed, the underlying reason (never swallowed), and at least one concrete next step; an escape-hatch verbose mode exposes the raw underlying cause.
4. **Given** the record file is itself unreadable or malformed, **When** verify runs, **Then** it exits with the usage/config error status (distinct from a verification failure) and explains the problem rather than dumping a raw parser error.

---

### Edge Cases

- **No record file at all**: treated as "no skills recorded" — verify passes if there are also no skills on disk, and reports every on-disk skill as untracked if there are. Vendoring creates the record file on first use.
- **Empty repo / nothing vendored**: verify is a success (nothing to check), not an error.
- **Local edits then re-vendor**: re-vendoring content that diverges from the recorded fingerprint requires an explicit override; it never silently discards local edits.
- **Re-vendor identical content**: produces no spurious change and reports success (idempotent).
- **Per-client view directories**: this feature does not create per-client symlink views (deferred — see Out of Scope), so the orphan/completeness check scans only the canonical skills location (`.agents/skills`). Robust handling of any manually-created view directories lands with multi-client materialization.
- **Malformed record file**: surfaced as a usage/config error with a clear message, distinct from a content-verification failure.
- **Not inside a git repo**: both `add` and `verify` require a git repository — the canonical skills location and the content fingerprint both derive from the repo's git content model — so running outside one is a usage/config error that says so.
- **Byte-exact fingerprint (no formatting tolerance on skill content)**: the content fingerprint is byte-exact; **any** change to a vendored skill file — including whitespace or line-ending changes — produces a different fingerprint and is a mismatch (there is no git-style "ignore whitespace" leniency). Only the *record file's own* incidental formatting is tolerated when reading it back; that has no bearing on the skill-content fingerprint.

## Requirements *(mandatory)*

### Functional Requirements

**Vendoring a skill (`add`)**

- **FR-001**: The system MUST provide a command that vendors a named skill from the repo's **configured origin** (resolved via the shared origin resolver — there is no separate source argument that bypasses it; the origin may be a local checkout) into the canonical skills location (`.agents/skills/<skill>`) and records its identity. For this **project-scope** feature, both this command and verification MUST run inside a git repository (the canonical `.agents/skills` location lives at the repo root). A future **global** scope (`--global`, materializing into the user's home `~/.agents/skills`, which is not a repo) is a separate, deferred carve-out — see Out of Scope — and will **not** be bound by this project-scope requirement.
- **FR-002**: For each vendored skill, the system MUST record its version, its provenance (where it came from), and a content fingerprint that uniquely reflects the skill's content.
- **FR-003**: The vendor command MUST be idempotent: re-vendoring identical content leaves an equivalent result and reports success without error.
- **FR-004**: The vendor command MUST NOT silently overwrite vendored content that diverges from the recorded fingerprint; it MUST detect the divergence and require an explicit override (`--force`) so local modifications are never lost without intent. It MUST NOT attempt a three-way merge — re-vendoring the same version has no upstream-advance axis to merge; that is a later `bump` concern.
- **FR-005**: The vendor command MUST support a preview (dry-run) mode that reports the intended placement and record changes without writing anything.
- **FR-006**: The vendor command MUST create any missing skills location and record file on first use.
- **FR-007**: The vendor command MUST operate offline when the resolved origin is a local source; it MUST NOT require network access in this feature. (Fetching from a remote GitHub-hosted origin, and the credential/auth handling that needs, is a later, additive mode — see Out of Scope.)

**Verifying vendored skills (`verify`)**

- **FR-008**: The system MUST provide a verification command that checks the repo's vendored skills against their recorded identities, entirely offline and deterministically (same inputs always yield the same result; no network or external/live signal).
- **FR-009**: Verification MUST recompute each recorded skill's content fingerprint from its **committed** on-disk content and compare it to the recorded value, failing when they differ (label honesty). A vendored skill with **uncommitted** local modifications MUST be surfaced as a **distinct** finding (a "dirty" verdict — "commit it / it has local modifications"), never silently passed nor conflated with a content mismatch.
- **FR-010**: Verification MUST compare the set of skills present on disk against the set of recorded skills, failing when a skill is present but unrecorded (untracked/orphan) or recorded but absent (missing) — covering the whole set, not only recorded entries.
- **FR-011**: Verification's orphan/completeness check MUST scan the canonical skills location (`.agents/skills`). This feature does not create per-client symlink views; robust handling of such views is deferred together with multi-client materialization (see Out of Scope).
- **FR-012**: Verification MUST report *all* detected problems in a run (e.g. both a content mismatch and an untracked skill), not stop at the first.
- **FR-013**: Verification MUST treat an empty repo / absent record as success (nothing to verify), and MUST treat an unreadable or malformed record as a usage/config error distinct from a verification failure.
- **FR-014**: Verification MUST NOT perform any backing-tool prerequisite or eligibility check; that is explicitly out of scope for this feature (reserved for a later health command). A missing backing tool MUST NOT cause a verification failure.
- **FR-015**: Verification MUST be read-only — it checks, and never writes, the record or the skill files.

**Shared trust primitive (`skillcore`)**

- **FR-016**: The content fingerprint and the skill-record parsing MUST have exactly one shared implementation used by both vendoring and verification, so the value written at vendor time and the value checked at verify time cannot diverge. (No parallel/duplicate implementation.)
- **FR-017**: That shared implementation MUST be reusable by future commands (e.g. upgrade-proposing and health commands) without copying the logic.

**Command experience (baseline conformance, consistent with the first slice)**

- **FR-018**: Every command MUST provide self-documenting help including at least two usage examples sufficient to construct a correct invocation without external docs.
- **FR-019**: Every error MUST state (a) what failed, (b) the real underlying reason (never swallowed), and (c) at least one concrete suggested fix; a verbose mode MUST expose the raw underlying cause.
- **FR-020**: Diagnostic output MUST go to the error stream and primary data to the standard output stream, so output can be cleanly piped.
- **FR-021**: The system MUST offer a machine-readable output mode whose output is complete (every checked skill with a per-skill verdict; no truncation) and parseable, for both passing and failing runs.
- **FR-022**: The system MUST use distinct, stable exit statuses: success; usage/config error (bad arguments, malformed record, not in a git repo); and verification failure (content mismatch, untracked/missing skill, **or an uncommitted/locally-modified vendored skill**). The prerequisite-failure status is reserved and MUST NOT be emitted by this feature's commands.

### Key Entities *(include if feature involves data)*

- **Skill**: a unit of agent capability — a directory of files (including a machine-readable manifest declaring its name, version, and any backing-tool prerequisites) vendored into the repo. The thing that is vendored, recorded, and verified.
- **Skill manifest**: the per-skill machine-readable description (name, version, namespace, description, discovery tags, declared backing-tool prerequisites) — vendored on disk as part of the skill subtree. Read at vendor time for identity. Its prerequisite declarations are **neither copied into the record nor evaluated** in this feature; the vendored manifest itself is the single source of truth for them (a later health command reads it directly).
- **Skill record (lock)**: the committed file (`.skillrig/skills-lock.json`) mapping each vendored skill to its recorded version, provenance (origin + commit), content fingerprint, and location. It does **not** duplicate the skill's backing-tool prerequisites (those live in the vendored manifest). Written by vendoring, read by verification; the source of truth for "what was approved."
- **Content fingerprint**: a value that uniquely reflects a skill's content as published for a given version. Used for *label honesty* — confirming on-disk content matches the version it claims to be. Computed identically at vendor time and verify time.
- **Verification verdict**: the outcome of verification — overall pass/fail plus a per-skill result: **matched** (`ok`), **content-mismatch** (`mismatch`), **untracked** (`orphan`), **missing**, or **uncommitted/modified** (`dirty`) — surfaced both compactly for humans and completely for machines. (Parenthised names are the machine field values used in `--json`.)

## Out of Scope

The following are explicitly **not** part of this feature and MUST NOT be pulled in:

- **Backing-tool prerequisite / eligibility checking** (is a skill's required CLI present, the right version, authenticable) and its dedicated exit status — reserved for a later health command. `verify` here is integrity-only.
- **Fetching from a remote origin** (network/git fetch) and the credential/auth it needs, plus immutable version pins — this feature consumes a **local** origin (a local checkout); remote GitHub-hosted origins + auth are a later, additive mode (production lean: GitHub-only).
- **Upgrade proposal & three-way merge** (`bump`): detecting upstream advances and merging them with local edits (base/theirs/ours), plus the conflict-marker handling that merge produces. Re-vendoring the *same* version has no upstream-advance axis, so this feature only **detects and refuses** divergence (`--force` to override); the merge and its conflict markers have no producer here.
- **Discovery** (`index.json`, search) and any browse UI.
- **Multi-client symlink materialization & agent-shell selection** — creating per-client view directories (e.g. `.claude/skills → ../.agents/skills`) and the `init`-time agent-shell selection stored in `.skillrig/config.toml`. `add` writes only the canonical `.agents/skills`; verification scans only that location. A separate, later feature.
- **External-source allowlists, audit classification, and risk/vulnerability surfacing** — later governance work.
- **Global-scope skills** (`--global` / a future `global add` / `global verify` materializing into the user's home `~/.agents/skills`) — a separate, later tier; this feature is **project-scope only**. Global targets are **not** git repos, so the git-repo requirement here is project-scope-specific, and global `verify` will need a non-repo fingerprint mechanism (recorded as a future concern in the spike).
- **Any authentication or credential handling.**

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can vendor a skill from a local library, commit it, and verify it — the vendor→commit→verify round-trip — with zero network access and no hand-authored records. (Verification checks committed content, so the commit is part of the loop.)
- **SC-002**: When a vendored skill's content matches its record, verification passes (success status); when any skill's content diverges from its record, verification fails with the verification-failure status — correct for 100% of label-honesty cases.
- **SC-003**: A single altered byte in any vendored skill file is detected as a content mismatch — 0 false negatives; and when multiple skills are altered, all are reported in one run (the check never exits on the first failure).
- **SC-004**: Any on-disk skill with no record entry (untracked) and any recorded skill absent on disk (missing) are both detected and fail the gate.
- **SC-005**: The content fingerprint computed at verify time is identical to the value recorded at vendor time for unmodified content — verified by vendoring real content and re-checking it, never by a hand-written value.
- **SC-006**: A missing backing tool never causes a verification failure (verification is integrity-only).
- **SC-007**: Verification is fully offline and deterministic: identical inputs yield the identical exit status and verdict on every run.
- **SC-008**: Every failure a user can hit (content mismatch, untracked/missing skill, malformed record) produces a message naming the problem and at least one concrete next step — 0 raw-only errors — and machine-readable output is complete and parseable for both passing and failing runs.
- **SC-009**: Each command's help output alone is sufficient for a first-time user or agent to construct a correct invocation (purpose plus ≥2 examples).

## Constitution Alignment *(skillrig-specific)*

- **II — Quickstart-as-Contract**: `quickstart.md` scenarios will be authored as executable steps (concrete invocations, observable record/skill contents, exit statuses) mapping 1:1 to integration tests. The vendor→verify round-trip, the tamper→fail case, and the untracked/missing cases are each a scenario. Output-shape assertions are required: machine-readable output parseable and structurally complete; error output checked for its three parts (what/why/fix) plus the correct exit status — not a single substring match.
- **III — Ground-Truth Anchoring**: the content fingerprint must be anchored to real captured content — vendored from a real local library fixture and recomputed — never an invented or hand-written fingerprint. (See spike §7.)
- **VIII — Single-Implementation Discipline (AP-04)**: the fingerprint and record-parsing primitives have exactly one implementation, shared by vendoring and verification and reusable by later commands; a parallel copy is a defect.
- **IX — Skill–CLI Co-Evolution**: this capability ships with a corresponding agent skill update teaching agents how to phrase "vendor this skill / check our skills are unmodified," how to read the pass/fail outcome, and that a missing backing tool is *not* a verification failure (it is a later health concern).

## Dependencies & Assumptions

**Assumptions**:

- The repo is pointed at its origin via `init` (feature 001); for this feature the resolved origin is a **local** source (a local checkout). `add` consumes the *resolved* origin — there is no separate source argument that bypasses it. Remote fetch is additive future work.
- Both `add` and `verify` (this **project-scope** feature) require being run inside a **git repository**: the canonical skills location and the content fingerprint derive from the repo's git content model, so running outside one is a usage/config error. The deferred **global** scope (`--global` → user home, not a repo) is explicitly *not* bound by this (see Out of Scope).
- Skills are vendored into the repo under version control, so the repo's own content model carries file integrity; the recorded fingerprint adds *label honesty* (content matches its claimed version) on top of that.
- `add` requires the origin to be configured (it resolves it to know what to vendor); `verify` does **not** (it reads the committed record and on-disk content). The skill record file and the per-skill manifest are separate concerns from the origin config of the first slice.

**Dependencies**:

- Builds on the first slice (`001-init-origin-resolution`) for the baseline command experience (help, errors-as-navigation, two-level output, exit-code discipline) and the project's config/skills directory conventions.
- Conventions are governed by `docs/design/cli.md` (Verification Gate and Vendor Mutation command patterns, Exit Codes) and `docs/ARCHITECTURE-v0.md` (§2, §4, §8, §9b), which were updated this branch to attribute prerequisite checking to the later health command rather than to verification. The detailed technical decisions live in [spec-tech-spike.md](./spec-tech-spike.md).

### Previous work

### Epic: 001-init-origin-resolution - CLI Initialization & Origin Resolution (closed)

- **Origin resolution + `skillrig init`**: established the single origin resolver and the baseline command experience (help, errors-as-navigation, two-level output, exit codes 0/1) that this feature extends with the verification-failure class (exit 2). This is the first feature to exercise integrity verification; no prior `add`/`verify`/`skillcore` work exists (`sl issue list --all` shows only closed `001` items).
