# Contract: `config.ResolveOrigin` (the single origin resolver)

**Scope**: internal Go API (`internal/config`), not a CLI command. The **one** implementation every current and future command calls (architecture AP-06 — never re-derive precedence per command).

## Signature

```go
// ResolveOrigin determines the active origin for the given working directory
// and environment, applying precedence env > project > global.
func ResolveOrigin(cwd string, env Env) (ResolutionResult, error)
```

- `env` is an injected accessor (e.g. `func(key string) string`) so tests set `SKILLRIG_ORIGIN` deterministically without mutating process env (golang-testing: table-driven, parallel-safe).
- Returns `ResolutionResult{Origin, Source, ConfigPath}` (see data-model.md).

## Precedence (FR-002)

1. **`SKILLRIG_ORIGIN`** — if set and non-blank → parse; `Source=env`.
2. **Project** — walk up from `cwd` to the nearest ancestor with `.skillrig/config.toml`; if found and it yields a valid origin → `Source=project`, `ConfigPath` set.
3. **Global** — `$XDG_CONFIG_HOME/skillrig/config.toml` else `~/.config/skillrig/config.toml`; if it yields a valid origin → `Source=global`.
4. Otherwise → `Source=none`, zero Origin.

A lower source is consulted **only** when every higher source is absent/blank/unusable. Behavior is the recorded matrix in `data-model.md` (rows 1–7).

## Error / robustness semantics

- Blank `SKILLRIG_ORIGIN` → treated as unset; fall through (matrix row 6).
- Unparseable or origin-less config file at a source → that source yields "none"; resolution **continues** down precedence (FR-004, matrix row 7). The malformed-file diagnostic is available via `--verbose`; it is never a raw parser dump to the user.
- `Source=none` is a normal return (not a Go error). The **caller** (e.g. a future command, or `init`'s sibling commands) converts `none` into the actionable "no origin configured" CLI error (exit 1, US3/FR-003).
- A genuine I/O error (e.g. unreadable file due to permissions) is returned as a Go `error` for the caller to surface with guidance.

## Determinism

Pure function of (`cwd`, `env`, filesystem state). No network, no time, no global mutable state → fully deterministic and table-testable. This is the property later offline gates (`verify`) depend on.

## Test mapping

- Unit: table-driven test over matrix rows 1–7 (`TestResolveOrigin_Precedence`), each asserting `Origin`, `Source`, and `ConfigPath`.
- Integration: exercised end-to-end through `skillrig init` + a follow-on resolution check in `quickstart.md` (e.g. env override beating project config).
