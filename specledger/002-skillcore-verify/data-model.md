# Data Model: `skillcore` + `add` + `verify`

**Feature**: `002-skillcore-verify` | **Date**: 2026-05-30
**Anchored to ground truth** (Constitution III): the SHAs below are **real `git` output** captured from a bootstrapped fixture (a `git init` + commit of the sample skill), not invented. The same procedure the tests use (D8).

## Ground-truth sample (real, captured 2026-05-30)

> **Representative, not canonical.** The sample skill's *content* (and therefore the exact SHA below) is illustrative — both this fixture skill and the existing `skillrig-origin` template are **pre-canonical samples** (research D12). What is anchored is the **mechanism**: these are genuine `git` tree/commit SHAs, and the tests recompute the expected value independently via raw `git` (research D11/D8), so the fixture content can change without touching the tests.

A skill `terraform-plan-review` (`SKILL.md` + `skill.toml`) committed at `skills/terraform-plan-review/` with pinned author/committer (`GIT_*_DATE=2026-01-01T00:00:00Z`, name `skillrig`, email `ci@skillrig.dev`):

```
$ git rev-parse HEAD
9f1a052e596d5d28f13838061a1ab93207ef6fc3                     # commit (provenance)

$ git rev-parse HEAD:skills/terraform-plan-review
c967789527370d2e0fba03a92e70dffef6f3bf31                     # subtree tree-SHA (the fingerprint)

$ git ls-tree HEAD:skills/terraform-plan-review
100644 blob 22de421b19fe58eeccfae1660dff0d139914e312    SKILL.md
100644 blob ec4b72549e3a28d59f7ec4e3ea29087b2ba5699f    skill.toml
```

**Relocation-invariance — confirmed empirically**: copying that subtree into a *different* repo at `.agents/skills/terraform-plan-review` and recomputing yields the **identical** tree-SHA:

```
$ git rev-parse HEAD:.agents/skills/terraform-plan-review
c967789527370d2e0fba03a92e70dffef6f3bf31                     # same as the origin's skills/… tree
```

This is the entire label-honesty mechanism: `add` records the origin's `c96778…`; after the consumer commits the vendored copy, `verify` recomputes `c96778…` and they match — both via `git rev-parse`, so equal by construction (research D1). The `commit` SHA is reproducible only because author/committer identity+date were pinned; with default identity it varies (tests assert it is present + 40-hex, or pin env — D8).

## Entities

### Manifest — `skill.toml` (parsed, read-only)

The per-skill manifest, vendored on disk as part of the subtree. `skillcore.ParseManifest` reads it (go-toml/v2).

| Field | Type | Notes |
|---|---|---|
| `name` | string | skill identity; SHOULD equal the directory name |
| `version` | string | recorded into the lock (e.g. `1.4.0`); not deep-validated this slice |
| `namespace` | string | reverse-DNS-ish; informational this slice |
| `description` | string | informational |
| `tags` | []string | discovery data (architecture §9); informational this slice |
| `requires` | []Require | `{ tool, version, source, manager }` — **parsed but NOT written to the lock** (D4); the on-disk manifest is the single source of truth, read later by `doctor` |

`add` uses `name` + `version`; `verify` uses the presence of `skill.toml` (or `SKILL.md`) to *recognize* a directory as a skill (for orphan detection). Unknown keys ignored on read (forward-compat).

### LockFile — `.skillrig/skills-lock.json` (committed; tool-written)

| Field | Type | Notes |
|---|---|---|
| `lockfileVersion` | int | `1` for this slice |
| `origin` | string | the configured origin reference for provenance (`OWNER/REPO[@REF]`, or the local origin used this slice) |
| `skills` | map[string]LockEntry | keyed by skill name |

**LockEntry** (note: **no `requires`** — D4):

| Field | Type | Notes |
|---|---|---|
| `version` | string | from the manifest at vendor time |
| `commit` | string | 40-hex; the origin commit the skill was vendored from (provenance) |
| `treeSha` | string | 40-hex; git tree-object SHA of the skill subtree (label honesty) |
| `path` | string | repo-relative vendored location, e.g. `.agents/skills/terraform-plan-review` |

**Serialization**: deterministic JSON — keys sorted (Go `encoding/json` sorts map keys), 2-space indent, trailing newline; **atomic write** (temp file in the same dir + rename, mirroring `internal/config.Save`) so the CI-bump-vs-human-edit race and partial writes are avoided (spike open Q10). Hand-editing is not intended (tool-written output).

Real example (from the ground-truth sample above):

```json
{
  "lockfileVersion": 1,
  "origin": "my-org/my-skills",
  "skills": {
    "terraform-plan-review": {
      "version": "1.4.0",
      "commit": "9f1a052e596d5d28f13838061a1ab93207ef6fc3",
      "treeSha": "c967789527370d2e0fba03a92e70dffef6f3bf31",
      "path": ".agents/skills/terraform-plan-review"
    }
  }
}
```

### TreeSHA (value)

The git tree-object SHA (40-hex SHA-1) of a skill subtree, produced by `git rev-parse <ref>:<relPath>` (research D1). Relocation-invariant (depends only on subtree contents). The single fingerprint primitive both `add` and `verify` use.

### AddResult (returned by `skillcore.Add`, rendered by the CLI)

| Field | Type | Notes |
|---|---|---|
| `name` | string | vendored skill |
| `version` | string | recorded version |
| `path` | string | where it was placed (`.agents/skills/<name>`) |
| `commit` | string | recorded provenance commit |
| `treeSha` | string | recorded fingerprint |
| `dryRun` | bool | true when `--dry-run` (nothing written) |
| `action` | enum | `vendored` \| `unchanged` (idempotent re-add) \| `overwritten` (`--force` over divergent) |

### VerifyReport + SkillVerdict (returned by `skillcore.Verify`, rendered by the CLI)

**VerifyReport**:

| Field | Type | Notes |
|---|---|---|
| `ok` | bool | true iff every verdict is `ok` |
| `verdicts` | []SkillVerdict | one per skill in the **union** of locked ∪ on-disk (so orphans + missing both appear) |
| `counts` | struct | `{ verified, mismatch, orphan, missing, dirty }` for the compact summary |

**SkillVerdict**:

| Field | Type | Notes |
|---|---|---|
| `name` | string | skill name |
| `path` | string | repo-relative path |
| `status` | enum | `ok` \| `mismatch` \| `orphan` \| `missing` \| `dirty` |
| `expectedTreeSha` | string | from the lock (empty for `orphan`) |
| `actualTreeSha` | string | recomputed (empty for `missing`) |
| `reason` | string | human-facing one-liner for non-`ok` (what/why/fix seed) |

**Status semantics** (research D1/D2):
- `ok` — locked, present, committed, `actualTreeSha == expectedTreeSha`.
- `mismatch` — locked + committed, but `actualTreeSha != expectedTreeSha` (label-honesty failure → exit 2).
- `orphan` — on disk under `.agents/skills/` (has `skill.toml`/`SKILL.md`) but no lock entry (→ exit 2).
- `missing` — lock entry whose `path` is absent on disk / absent from `HEAD` (→ exit 2).
- `dirty` — locked + present but uncommitted/modified vs `HEAD` (working tree dirty for that path) → reported distinctly; **counts as a verification failure (exit 2)** but with a "commit it / it has local modifications" message rather than a tree-SHA mismatch.

### Typed errors (presentation-free; `pkg/skillcore/errors.go`)

| Type | Carries | CLI mapping |
|---|---|---|
| `VerifyFailure` | the `VerifyReport` (≥1 non-`ok` verdict) | → `ExitVerification` (2); CLI renders per-skill verdicts |
| `GitError` | `{ ExitCode, Stderr }` (gh pattern) | → wrapped as a `*cli.UsageError` (1) for env problems (e.g. not a git repo), with raw cause under `--verbose` |
| (malformed lock / not-a-git-repo) | path + cause | → `*cli.UsageError` (1) |

`skillcore` returns these; **it never formats user-facing text** (Constitution V) — the CLI layer renders human/`--json` output and maps errors to exit codes (research D9).

## Validation rules

- `treeSha`, `commit`: 40-char lowercase hex (git SHA-1).
- `path`: repo-relative, under the canonical `.agents/skills/` root; the leaf SHOULD equal the skill `name`.
- `lockfileVersion`: exactly `1`; any other value → malformed-lock usage error (forward-compat guard).
- `origin`: the configured origin string (provenance only this slice; not re-validated by `verify`).
- A directory under `.agents/skills/` is a "skill on disk" iff it contains `skill.toml` or `SKILL.md` (non-skill dirs ignored — spike §6).

## State transitions (add)

```
absent ──add──▶ vendored(locked, on disk)            # first add: write files + lock entry
vendored ──add(identical)──▶ unchanged               # idempotent (action=unchanged)
vendored ──[local edit]──▶ divergent                 # tree-SHA != lock
divergent ──add──▶ refused (exit 1) unless --force   # never silently clobber (FR-004)
divergent ──add --force──▶ overwritten(re-vendored)  # explicit override
```

`verify` performs **no** state transitions (read-only).
