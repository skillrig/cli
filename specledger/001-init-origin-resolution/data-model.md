# Data Model: CLI Initialization & Origin Resolution

> **Ground-Truth Anchoring (Constitution III)**: the `config.toml` shape below is the canonical fixture — a real file written by `skillrig init` and then read back — not invented from prose. The resolution-precedence matrix is the recorded ground-truth table the resolver tests assert against. Both live as test fixtures (`test/fixtures/config.toml`, `test/fixtures/precedence.json` or a table-driven literal).

## Entities

### Origin
The org's skill source, in `OWNER/REPO` form. The single value this feature reads, validates, records, and resolves.

| Field | Type | Rules |
|-------|------|-------|
| `Owner` | string | non-empty, charset `[A-Za-z0-9._-]` |
| `Repo`  | string | non-empty, charset `[A-Za-z0-9._-]` |

- Constructed via `ParseOrigin(s string) (Origin, error)`: trims whitespace, matches `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`. On failure returns a usage error naming the expected format and echoing the offending value (FR-012).
- `String()` renders `Owner/Repo`.

### ProjectConfig  → `.skillrig/config.toml` (committed, hand-editable INPUT)
| Field | TOML key | Type | Notes |
|-------|----------|------|-------|
| `Origin` | `origin` | string | `OWNER/REPO`; the only field this feature reads/writes |

> Forward-compatibility: unknown keys in `config.toml` are **ignored** on read (not an error), so fields added later (client targets, adoption policy — architecture §2d) don't break this version. Detailed/extended config structure is documented on the project docs website, not restated here (spec clarification).

### GlobalConfig → `$XDG_CONFIG_HOME/skillrig/config.toml` or `~/.config/skillrig/config.toml`
Same shape as ProjectConfig; the per-user default origin. Written only with `--global`.

### EnvOverride
`SKILLRIG_ORIGIN` environment variable. Highest precedence. A blank/whitespace-only value is treated as **unset** (not invalid).

### ResolutionResult (in-memory, returned by `ResolveOrigin`)
| Field | Type | Notes |
|-------|------|-------|
| `Origin` | Origin | the resolved origin (zero value if none) |
| `Source` | enum `env` \| `project` \| `global` \| `none` | which source supplied it |
| `ConfigPath` | string | path of the file used (empty for `env`/`none`) |

- `Source == none` is a distinct, first-class outcome (FR-003) callers convert into the actionable "no origin configured" error (US3).

## Canonical fixture — `config.toml` (ground truth)

```toml
# .skillrig/config.toml — written by `skillrig init`
origin = "my-org/my-skills"
```

That is the entire v0 file: one `origin` key. The byte-for-byte output of `init` is asserted against this fixture (round-trip: write → read → equal).

## Resolution precedence matrix (recorded ground truth)

`ResolveOrigin(cwd, env)` precedence — a lower source supplies the origin only when all higher sources are absent/empty (FR-002). `✓` = present, `–` = absent/blank.

| # | `SKILLRIG_ORIGIN` | project `.skillrig/config.toml` | global config | → Resolved origin | → Source |
|---|-------------------|----------------------------------|---------------|-------------------|----------|
| 1 | – | – | – | (none) | `none` |
| 2 | – | ✓ `my-org/my-skills` | – | `my-org/my-skills` | `project` |
| 3 | ✓ `ci-org/ci-skills` | ✓ `my-org/my-skills` | – | `ci-org/ci-skills` | `env` |
| 4 | – | – | ✓ `personal/skills` | `personal/skills` | `global` |
| 5 | – | ✓ `client-a/skills` | ✓ `personal/skills` | `client-a/skills` | `project` |
| 6 | ✓ (blank) | ✓ `my-org/my-skills` | – | `my-org/my-skills` | `project` (blank env = unset) |
| 7 | – | (malformed/unparseable file) | ✓ `personal/skills` | `personal/skills` | `global` (bad project source skipped, FR-004) |

Rows map directly to table-driven resolver unit tests and to quickstart precedence scenarios.

## State / lifecycle

`init` is the only state transition and is idempotent:

- **no config → write** : `Source=none` → file created, `written=true`.
- **same origin → no-op** : existing == requested → `written=false`, success (FR-008).
- **different origin → replace** : existing != requested → file rewritten with new origin, `written=true` (FR-009).

No deletion, no other transitions in scope.

## Validation rules (consolidated)

| Rule | Where | Failure → |
|------|-------|-----------|
| Origin matches `OWNER/REPO` | `ParseOrigin` (init write + resolved value) | usage error, exit 1, no write (FR-012) |
| Blank `SKILLRIG_ORIGIN` = unset | resolver | fall through precedence |
| Unparseable/origin-less config = "none from this source" | resolver | skip source, continue; clear diagnostic, not raw dump (FR-004) |
| No origin in any source | resolver → caller | actionable "no origin configured" error, exit 1 (US3, FR-003) |
| Non-interactive + no `--origin` | `init` | usage error, exit 1 (FR-006a) |
