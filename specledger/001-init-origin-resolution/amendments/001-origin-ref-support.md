# Amendment 001: Optional branch/ref in the origin reference (`OWNER/REPO[@REF]`)

**Amends**: [spec.md](../spec.md) — *CLI Initialization & Origin Resolution* (feature `001-init-origin-resolution`)
**Created**: 2026-05-29
**Status**: Accepted
**Tracking**: `SL-2f13c6` (feature) + child tasks, spec `001-init-origin-resolution`

## Motivation

The original feature fixed the origin reference at the bare `OWNER/REPO` shape and listed "branch, ref, tag, version, or commit pinning" under *Out of Scope*. A consumer cannot currently point a repo at a **non-default branch** of the skills library (e.g. a `staging` line under active review before it merges to the default branch). This amendment adds an **optional** branch/ref to the origin so a repo can track a specific branch, while preserving the offline, consume-only character of `init`.

The chosen shape is the ecosystem-standard identity grammar already adopted as the v0 foundation in [architecture §9b R26](../../../docs/ARCHITECTURE-v0.md) — `OWNER/REPO[/path]@ref` — narrowed here to `OWNER/REPO[@REF]` (the `[/path]` portion remains future work).

> **Supersedes**: the *Out of Scope* bullet "Collecting any onboarding metadata beyond the origin … no branch/ref/tag/version/commit pinning". Branch tracking on the **origin** is now in scope. Immutable pinning of an individual **skill** (tag/SHA via `add --pin`) remains out of scope for this feature.

## Clarifications

### Session 2026-05-29

- Q: How is the branch/ref passed — a separate flag, a single string with a separator, or something else? → A: A single string with an `@` separator (`--origin OWNER/REPO@REF`), matching the R26 grammar and the `gh skill` / npm / Go-module conventions. No new flag; stored combined in the single `origin` config key. (A separate `--branch` flag and a `#` separator were considered and rejected — see *Decision* D-A1.)
- Q: What does the ref accept — strictly a branch name, or any git ref? → A: General/shape-only. The ref is validated only for shape (a permissive charset, offline); the CLI does not try to detect branch-vs-tag-vs-commit (heuristics misfire on unusual names) and does not perform any network lookup. Semantically, for an *origin* the ref is intended as a **branch** (a moving pointer the consumer tracks).
- Q: Does `init` verify the origin/branch exists or that the user has access? → A: No — and this amendment does not change that. Origin handling is shape-only and offline (the only git call is a local `git rev-parse --show-toplevel` for the write target). Existence/reachability/auth remain deferred to future commands (`doctor`/`verify`/`add`).

## Requirements (additions)

Continuing the FR sequence from spec.md (last was FR-017):

- **FR-018**: The origin reference MUST accept an optional `@REF` suffix — overall shape `OWNER/REPO[@REF]`. When `@REF` is omitted the origin tracks the library's default branch; when supplied it tracks that ref (intended use: a branch). This applies uniformly to `--origin`, `SKILLRIG_ORIGIN`, and the `origin` key in config.
- **FR-019**: The `@REF` MUST be validated **shape-only and offline**, consistent with FR-012's `OWNER/REPO` validation. The `@`-split is unambiguous (the owner/repo charset excludes `@`); a malformed ref — empty after `@`, or containing characters outside the permitted charset — is rejected as a usage error (exit 1) that echoes the offending value and names the expected `OWNER/REPO[@REF]` shape, **without writing config**. Existence/reachability of the ref is **not** checked.
- **FR-020**: The ref MUST be stored **combined within the single `origin` key** (`origin = 'OWNER/REPO@REF'`) — no new config key or struct field is introduced. The resolver returns the ref as part of the resolved origin unchanged; the value round-trips through `Origin.String()` so write and read stay symmetric and the precedence resolver needs no structural change.

These extend (do not replace) FR-012: a bare `OWNER/REPO` remains valid and is the common case.

## Data-model delta

Amends [data-model.md](../data-model.md) → *Entities* → **Origin**:

| Field | Type | Rules |
|-------|------|-------|
| `Owner` | string | non-empty, charset `[A-Za-z0-9._-]` |
| `Repo`  | string | non-empty, charset `[A-Za-z0-9._-]` |
| `Ref`   | string | **optional**; when present, charset `[A-Za-z0-9._/-]` (owner/repo charset plus `/` for branch names like `feature/x`). Empty = default branch. |

- `ParseOrigin(s)` now: trims whitespace; splits on the first `@` into `OWNER/REPO` and `REF`; matches `OWNER/REPO` against `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$` and, when an `@` was present, `REF` against `^[A-Za-z0-9._/-]+$`; on failure returns a typed `*InvalidOriginError` carrying the offending value. Per the presentation-free `internal/config` rule (CLAUDE.md; Qodo rule 783432), `internal/cli` renders the user-facing `OWNER/REPO[@REF]` what/why/fix message (e.g. `--origin my-org/my-skills` or `--origin my-org/my-skills@main`) (FR-012/FR-018).
- `String()` renders `Owner/Repo`, appending `@Ref` only when `Ref != ""`. The zero Origin still renders `""` (SourceNone sentinel — unchanged).
- **Storage is unchanged structurally**: `Config{ Origin string }` and the single `origin` TOML key. A ref'd origin serializes as `origin = 'my-org/my-skills@staging'`.

### Canonical fixtures

- The base fixture `test/fixtures/config.toml` (`origin = 'my-org/my-skills'`) is **unchanged** — the ref-less form is the canonical default and keeps `TestSaveMatchesFixture` + the resolver matrix stable.
- The ref'd form (`origin = 'my-org/my-skills@staging'`) is anchored in-test by `TestSaveLoadRoundTripWithRef` and `TestQuickstart_BindWithRef` rather than a second committed fixture.

### Precedence matrix delta

The recorded matrix (rows 1–7) is unchanged in behavior; two rows are added to prove the ref survives resolution end-to-end:

| # | `SKILLRIG_ORIGIN` | project config | global | → Resolved | → Source |
|---|---|---|---|---|---|
| 8 | – | ✓ `my-org/my-skills@staging` | – | `my-org/my-skills@staging` | `project` |
| 9 | ✓ `ci-org/ci-skills@main` | ✓ `my-org/my-skills` | – | `ci-org/ci-skills@main` | `env` |

## Quickstart addition

Amends [quickstart.md](../quickstart.md) → *Part A*:

### TestQuickstart_BindWithRef  (FR-018, FR-019, FR-020)
```
$ skillrig init --origin my-org/my-skills@staging
```
- **exit**: 0
- **stdout** (human, compact): line 1 contains `bound origin my-org/my-skills@staging` and `project`; line 2 is the `→ resolve order:` footer hint.
- **file** `./.skillrig/config.toml`:
  ```toml
  origin = 'my-org/my-skills@staging'
  ```
- **`--json`** (variant): `origin == "my-org/my-skills@staging"`.
- **shape assert**: `len(stdoutLines) <= 2`.

Coverage map adds: `BindWithRef → FR-018, FR-019, FR-020`. Resolver rows 8–9 fold into `TestResolveOrigin_Precedence`.

## Contract deltas

- [contracts/init.md](../contracts/init.md): `--origin` synopsis/flag becomes `OWNER/REPO[@REF]`; the malformed-origin error row names the `OWNER/REPO[@REF]` shape. Behavior step 3 (`ParseOrigin`) now also validates the optional ref.
- [contracts/resolve.md](../contracts/resolve.md): a resolved origin MAY carry a `Ref`; the resolver passes it through unchanged (no precedence/structural change — the ref lives inside the origin string).

## Decision

- **D-A1 — `@ref` single string over `--branch` flag / `#` separator.** Chosen for: alignment with the already-adopted R26 grammar (`OWNER/REPO[/path]@ref`) and the `gh skill` / `npx skills` / npm / Go conventions an agent already knows; the "one key, three consumers" property (config, future `index.json` rows, future allowlist/lock entries share one canonical key); and avoiding a "what wins if both `@ref` and `--branch` are set?" ambiguity. A `#` separator (git-URL / npm git-dep style) was rejected for weaker alignment with the grammar this project already committed to.
- **D-A2 — general, shape-only ref (no network, no branch-vs-tag-vs-commit detection).** Detection would require heuristics that misfire on unusual but legal ref names, or a network lookup that breaks the offline guarantee. Shape-only keeps `init` deterministic and offline; whether the ref exists is a later command's job.

## Constitution alignment

- **II — Quickstart-as-Contract**: `TestQuickstart_BindWithRef` is the executable acceptance test for this amendment; resolver rows 8–9 cover the resolution path.
- **III — Ground-Truth Anchoring**: the ref'd `config.toml` bytes are anchored by `TestSaveLoadRoundTripWithRef` (`origin = 'my-org/my-skills@staging'`), regenerated from `Save`.
- **IX — Skill–CLI Co-Evolution**: the `skillrig-init` agent skill is updated to mention the optional `@REF`/branch, with the eval re-run to confirm trigger accuracy holds.
