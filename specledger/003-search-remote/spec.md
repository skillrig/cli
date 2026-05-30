# Feature Specification: Discover & Acquire Skills (`search` + remote `add`)

**Feature Branch**: `003-search-remote`
**Created**: 2026-05-30
**Status**: Draft
**Input**: User description: "Combine roadmap 003 (search) and 004 (remote add) into one MVP slice: a user who has bound their repo to an origin can FIND a skill in the org's library and VENDOR it straight from the remote library — with no pre-existing local copy of the library."

> **Technical companion**: [spec-tech.md](./spec-tech.md) holds every implementation-level decision (origin classification, fetch mechanism, catalog handling, authentication sources, fingerprint semantics, the new network test tier) and the **seven open decisions deferred to `/specledger.clarify`**. This spec stays user-facing; the companion is the input to `/specledger.plan`. Where this spec says "the library catalog" or "records the skill's exact identity," the companion names the concrete artifacts.

## Overview

The first two slices made a repo *self-describing about where its skills come from* (001) and made the skills it already carries *honest* (002 — `add` from a **local copy** of the library, plus `verify`). But to vendor a skill today, a user must already have the entire library checked out next to their repo. That is a developer-only workaround, not something an organization can adopt.

This slice closes that gap with the smallest coherent "discover & acquire" loop:

1. A user **finds** the skill they want by browsing or filtering their org's library (`search`).
2. A user **vendors** that skill directly **from the remote library** — no manual checkout, no copy step — and the tool records its exact identity so it can be verified later (`add`, extended to fetch remotely).

After this slice, the everyday path is: `skillrig init` → `skillrig search` → `skillrig add <skill>` → `skillrig verify`. That is the first end-to-end experience a real consumer can adopt.

Remote acquisition is **additive**: vendoring from a local copy of the library (shipped in 002) keeps working unchanged, as a development, offline, and air-gapped path.

## Clarifications

This specification intentionally leaves seven decisions open for `/specledger.clarify`; they are enumerated in [spec-tech.md](./spec-tech.md) ("Open Decisions"). Each is recorded here as a documented **assumption** (see [Assumptions](#assumptions)) so the spec is internally consistent, with the note that `/clarify` may revise it. None of the seven changes the user-visible *goals* below; they shape *how* the goals are met.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover a skill in the org library (Priority: P1)

A developer (or their agent) has bound the repo to an origin but does not know the exact name of the skill they want. They ask the tool to list what the library offers, optionally narrowing by topic, and get back a short, scannable answer they can act on — including the exact name to feed to `add`.

**Why this priority**: You cannot vendor what you cannot find. Discovery is the entry point of the loop and the lowest-risk half of the slice; on its own it already delivers value (an agent can enumerate available skills before deciding). It is independently demonstrable without any change to how skills are vendored.

**Independent Test**: Bind a repo to a library that publishes a catalog of skills; run the search command with and without a topic filter; confirm the matching skills are listed (human-readable and machine-readable), that filtering is exact and repeatable, and that an empty result is reported as a clean "nothing matched" rather than an error.

**Acceptance Scenarios**:

1. **Given** a repo bound to a library that publishes two or more skills, **When** the user runs `skillrig search`, **Then** the tool lists every published skill with enough detail (name, version, one-line description) to choose one, plus a footer hint pointing to the next step.
2. **Given** the same repo, **When** the user runs `skillrig search` filtered to a topic that only some skills carry, **Then** only the skills carrying that topic are listed, and the result is identical on repeated runs.
3. **Given** the same repo, **When** the user filters to a topic no skill carries, **Then** the tool reports "no skills matched" and succeeds (it is not an error to find nothing).
4. **Given** any search, **When** the user requests machine-readable output, **Then** the output is complete (no truncation) and contains every field a downstream agent needs to call `add`.

---

### User Story 2 - Vendor a skill directly from the remote library (Priority: P1)

A developer (or their agent) has found the skill they want and vendors it with a single command. They do **not** first clone or copy the library anywhere — the tool fetches the skill's content from the remote library on their behalf, places it in the repo, and records its exact identity so the same content can be proven later.

**Why this priority**: This is the keystone that turns skillrig from a local-path tool into one an organization can adopt. It completes the discover→acquire→verify loop and unblocks every later capability (upgrades, multi-client placement).

**Independent Test**: From a repo bound to a remote library, with **no** local copy of that library present, run the add command for a published skill; confirm the skill's files appear in the repo identical to the library's, that an identity record is written, and that the existing `verify` command then passes against what was vendored.

**Acceptance Scenarios**:

1. **Given** a repo bound to a remote library and **no** local copy of it, **When** the user runs `skillrig add <skill>` for a published skill, **Then** the skill's content is placed in the repo exactly as the library holds it and an identity record (which version, where it came from, and a fingerprint of the content) is written.
2. **Given** a freshly vendored skill, **When** the user runs `skillrig verify`, **Then** verification passes — the recorded identity and the on-disk content agree.
3. **Given** a skill already vendored at the same version and content, **When** the user runs `skillrig add <skill>` again, **Then** the tool reports "already up to date" and changes nothing (a safe, repeatable no-op).
4. **Given** a skill already vendored but locally modified, **When** the user runs `skillrig add <skill>` again, **Then** the tool refuses to silently overwrite and tells the user how to force it — matching the behavior vendoring from a local copy already has.
5. **Given** a request for a skill the library does not publish, **When** the user runs `skillrig add <skill>`, **Then** the tool reports the skill was not found in the library and suggests how to discover the correct name.

---

### User Story 3 - Acquire a pinned, reproducible version (Priority: P2)

A developer wants the acquisition to be reproducible: not "whatever the library's current tip happens to be," but an exact, immutable version. They pin the skill to a specific released version when vendoring, and that exact identity is recorded so a later acquisition (on another machine, in CI, months later) reproduces the same content byte-for-byte.

**Why this priority**: Reproducibility is core to the product promise ("exactly the version that was reviewed and approved"), but the default path (US2) already records a verifiable fingerprint, so explicit pinning is a strengthening rather than a prerequisite. It can ship immediately after US2 or split out if it risks the MVP.

**Independent Test**: Vendor a skill pinned to a specific released version; record the result; on a clean repo, vendor the same skill pinned to the same version; confirm the two results are byte-identical and carry the same recorded identity.

**Acceptance Scenarios**:

1. **Given** a library that has published more than one released version of a skill, **When** the user vendors it pinned to a specific version, **Then** that exact version's content is placed and its immutable identity is recorded.
2. **Given** a skill vendored at a pinned version, **When** another user vendors the same skill at the same pin on a clean repo, **Then** both repos hold byte-identical content with the same recorded identity.

---

### User Story 4 - Trustworthy, navigable failures (Priority: P2)

When acquisition or discovery cannot proceed, the developer (or their agent) gets an error that says what failed, the real reason, and what to do next — never a misleading or generic message. In particular, three confusable situations are kept distinct: (a) the tool is too old (or too new) for the library's format; (b) the user lacks permission to reach a private library; (c) the library cannot be reached at all.

**Why this priority**: Errors-as-navigation is a binding principle of this CLI, and the auth-vs-not-found confusion is the single most common onboarding and CI footgun. Getting these distinct is what makes the remote path safe to hand to an agent. It is P2 only because the happy path (US1/US2) must exist first to fail against.

**Independent Test**: Drive each failure independently — point the tool at a library whose format it does not support; attempt to reach a private library without credentials; attempt to reach an unreachable library — and confirm each produces a *distinct*, actionable message, and that a verbose mode reveals the underlying cause.

**Acceptance Scenarios**:

1. **Given** a library whose published format is newer (or otherwise incompatible) than this tool understands, **When** the user runs `search` or `add`, **Then** the tool fails clearly stating a compatibility mismatch and what to do (e.g. update the tool), rather than misbehaving or producing partial results.
2. **Given** a private library the user is not authenticated to, **When** the user runs `search` or `add`, **Then** the tool reports an **authentication** problem distinctly from "skill not found" or "library not found," and points at how to authenticate.
3. **Given** a library that cannot be reached (offline, wrong location), **When** the user runs `search` or `add`, **Then** the tool reports the library could not be reached and suggests the likely fixes, distinct from an authentication or compatibility failure.
4. **Given** any of the above, **When** the user re-runs with verbose output, **Then** the underlying raw cause is shown without being swallowed.

---

### Edge Cases

- **Library with an empty catalog**: `search` reports "no skills published" and succeeds; `add` of any name reports "not found."
- **Local copy present AND remote reachable**: the tool's choice of which to use is deterministic and documented (the precedence rule is one of the deferred decisions; see Assumptions). The user can always tell which was used.
- **Skill listed in the catalog but its content is missing/incomplete in the library**: treated as a library-side problem and reported as such (distinct from "not found" and from "auth"), not as a silent partial vendor.
- **Topic filter matches the catalog but the chosen skill is later not fetchable** (US1 found it, US2 cannot get it): the discovery success and the acquisition failure are reported independently and honestly.
- **Catalog and the actual published skills disagree** (a skill is listed but the library has moved on, or vice-versa): the tool does not invent results; it reports what it can verify and surfaces the discrepancy.
- **Pin names a version that does not exist**: reported as an actionable "no such version," distinct from "skill not found."

## Requirements *(mandatory)*

### Functional Requirements

**Discovery (`search`)**

- **FR-001**: Users MUST be able to list the skills their bound library publishes, via a `search` command, without first obtaining a local copy of the library.
- **FR-002**: Users MUST be able to narrow the listing by one or more topics, and the filtering MUST be deterministic and repeatable (the same inputs always produce the same listing), with no inference or ranking by relevance.
- **FR-003**: `search` MUST present results in two levels: a compact, scannable human listing with a footer hint toward the next step, and a complete machine-readable form that includes every field needed to subsequently vendor a listed skill.
- **FR-004**: Finding no matches MUST be a successful outcome that clearly says nothing matched — never an error.
- **FR-005**: Each listed skill MUST include at least its exact name (as accepted by `add`), its version, and a one-line description.

**Remote acquisition (`add`)**

- **FR-006**: Users MUST be able to vendor a published skill directly from the **remote** library with a single `add` command, with **no** pre-existing local copy of the library required.
- **FR-007**: Vendoring MUST place the skill's content in the repo identical to what the library holds for the acquired version.
- **FR-008**: Vendoring MUST record the skill's exact identity — which version, where it came from, and a content fingerprint — such that the existing `verify` command can later confirm the on-disk content matches what was recorded.
- **FR-009**: Re-vendoring a skill that is already present at the same version and content MUST be a safe no-op that reports "already up to date" and changes nothing.
- **FR-010**: Re-vendoring a skill whose local content has diverged MUST refuse to silently overwrite, and MUST tell the user how to force the overwrite — consistent with the behavior when vendoring from a local copy.
- **FR-011**: Vendoring from a **local copy** of the library MUST continue to work unchanged (remote acquisition is additive, not a replacement); the tool MUST behave predictably and tell the user which source was used when both a local copy and a remote are available.
- **FR-012**: Requesting a skill the library does not publish MUST report "not found in the library" with guidance to discover the correct name (e.g. run `search`), and MUST be distinct from reaching/auth failures.

**Reproducible pinning (`add`)**

- **FR-013**: Users MUST be able to vendor a skill pinned to a specific, immutable released version, and that exact identity MUST be recorded.
- **FR-014**: Vendoring the same skill at the same pin on a clean repo MUST reproduce byte-identical content with the same recorded identity.
- **FR-015**: Pinning to a version that does not exist MUST be reported as an actionable "no such version," distinct from "skill not found."

**Trust & failure modes (both commands)**

- **FR-016**: When the library's published format is incompatible with this tool, both `search` and `add` MUST fail clearly with a compatibility-mismatch message and a suggested remedy, rather than misbehaving or producing partial results.
- **FR-017**: When the library is private and the user is not authenticated, both commands MUST report an **authentication** failure that is distinct from "not found" and from "unreachable," and MUST point at how to authenticate.
- **FR-018**: When the library cannot be reached, both commands MUST report an unreachable-library failure distinct from authentication and compatibility failures.
- **FR-019**: Every error MUST state what failed, the real (never-swallowed) cause, and a suggested fix, with a verbose mode that reveals the underlying raw cause (errors-as-navigation).
- **FR-020**: Both commands MUST expose the project's standard output and diagnostic options (machine-readable output; verbose); `add` MUST additionally support a dry-run preview and a force override, consistent with the existing vendoring command.

**Exit behavior**

- **FR-021**: `search` MUST exit success on any well-formed query (including an empty result) and signal a usage/configuration problem with the standard usage/config exit status; it does not produce verification or prerequisite failures.
- **FR-022**: `add` MUST exit success on a completed vendor *and* on an idempotent no-op, and signal a usage/configuration problem (including not-found, auth, unreachable, and incompatibility, which are configuration/usage-class for this slice) with the standard usage/config exit status. Verification-failure and prerequisite-failure exit statuses remain out of scope for this slice.

**Co-evolution deliverables (process requirements for this branch)**

- **FR-023**: The PoC origin template repository (the real library this feature is designed against) MUST be updated so the catalog it publishes carries exactly the fields `search` consumes, and so any catalog-generation helper it ships agrees with that shape (it currently omits topics). The reconciliation MUST be recorded.
- **FR-024**: The project roadmap and architecture documents MUST be updated to record the divergences this branch introduces: that discovery and remote acquisition ship as **one** combined slice, and that the earlier local-copy seam is superseded by (now coexists with) real remote acquisition.

### Key Entities *(include if feature involves data)*

- **Library (origin)**: the org's source-of-truth repository of skills, identified by `OWNER/REPO` with an optional branch/ref, already resolvable by the tool. May be reached remotely or via a local copy.
- **Library catalog**: the library's published, machine-readable list of available skills — each entry carrying at least name, version, description, and topics — plus the format/compatibility marker the tool checks. The basis for `search`. Discovery-only: it does not itself carry per-skill fingerprints.
- **Skill**: a named, versioned unit of agent instruction content vendored into the consumer repo.
- **Topic (tag)**: a deterministic label attached to a skill in the catalog, used to filter discovery. Data only; no inferential matching.
- **Pin**: an explicit, immutable version reference used at acquisition time to make the result reproducible (distinct from the library's moving branch pointer).
- **Identity record (lock entry)**: the per-skill record written at acquisition time — version, source/provenance, and content fingerprint — that `verify` later checks. Unchanged in shape from 002; this slice writes it from a remote source.

## Assumptions

These are reasonable defaults adopted so the spec is internally consistent. Each corresponds to an open decision deferred to `/specledger.clarify` (full detail and alternatives in [spec-tech.md](./spec-tech.md) §8); `/clarify` may revise any of them. None changes the user-visible goals.

- **A1 — Local vs remote source (FR-006, FR-011)**: when both a local copy of the library and a remote are available, the tool prefers the local copy and names which source it used; a bare `OWNER/REPO` with no local copy is fetched remotely. The selection is deterministic and visible to the user.
- **A2 — Discovery freshness**: `search` reflects the library's current published catalog at run time; if the library cannot be reached, `search` reports an unreachable failure rather than serving a stale result (no offline cache assumed in this slice).
- **A3 — Topic filtering**: multiple topics narrow the result (a skill must carry all requested topics to match); matching is exact-string and case-sensitive on the catalog's labels, with no relevance ranking.
- **A4 — Authentication**: the tool reuses the user's existing standard credentials for reaching the library (the same mechanism already required to clone private org repos); it introduces no credential of its own and stores nothing.
- **A5 — Reproducibility anchor**: the recorded content fingerprint proves the on-disk content still matches what was vendored from the library at a specific provenance point; it is not an independently library-attested hash (the library does not publish per-skill fingerprints in its catalog).
- **A6 — Identity/lock shape**: the per-skill identity record keeps the same shape established in 002; this slice only changes that the content can originate remotely.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A new user can go from a freshly bound repo to a vendored, verifying skill using only `search` then `add` — with **no** manual checkout or copy of the library — in a single sitting, demonstrated end-to-end against the real PoC library.
- **SC-002**: 100% of `search` results are deterministic: identical inputs against an unchanged library produce identical output across repeated runs.
- **SC-003**: A skill vendored remotely and then checked with `verify` passes 100% of the time when untouched (the recorded identity and on-disk content agree).
- **SC-004**: The same skill vendored at the same pin on two clean repos yields byte-identical content and identical recorded identity 100% of the time.
- **SC-005**: Each of the three confusable failure classes — incompatible format, authentication, unreachable — produces a distinct, actionable message; in usability checks a reader can correctly identify which class occurred from the message alone.
- **SC-006**: Re-running `add` on an unchanged, already-vendored skill changes nothing on disk and reports an idempotent no-op 100% of the time.
- **SC-007**: Vendoring from a local copy of the library (the 002 path) continues to pass its existing acceptance scenarios unchanged (no regression).
- **SC-008**: Both commands' help text alone lets an agent succeed on the first attempt: it states purpose and shows at least two runnable examples.

### Previous work

### Epic: SL-227789 — CLI Initialization & Origin Resolution (001, closed)

- **Origin resolution & `init`**: established the single origin resolver (`env > project config > global default`), the `OWNER/REPO[@REF]` origin grammar, and the baseline CLI experience (self-documenting help, errors-as-navigation, two-level output, exit codes) that this slice inherits.

### Feature: 002 — Vendor & Verify Skills (`add` + `verify`, merged)

- **`skillcore` + local `add` + `verify`**: shipped the shared trust primitive (content fingerprint + manifest parse), local-copy vendoring with idempotent no-op / force-on-divergence UX, and the offline integrity gate this slice's remote acquisition writes records for and reuses. This slice extends `add` from a local copy to a remote library and adds `search`; both reuse the same shared core (no parallel implementation).

> External references: this feature is designed against the real PoC origin template repository (`github.com/skillrig/origin-template`, checked out alongside this repo). If its published contract should be tracked as a formal dependency for reading/reference, add it with `sl deps add`.
