# Contract: `skillrig search` (Query pattern)

Discover skills published by the resolved origin. Read-only. Reads the origin's `index.json` (D2/D7), gates `skillrigConvention` (D-convention), filters deterministically.

## Synopsis
```
skillrig search [QUERY] [--tag T ...] [--json] [--verbose]
```

## Flags
| Flag | Meaning |
|---|---|
| `[QUERY]` | optional substring matched (case-insensitive) against `name` + `description` |
| `--tag T` | repeatable; a skill must carry **all** requested tags (AND); exact-string, case-sensitive (A3) |
| `--json` | complete, untruncated machine output (every catalog field) |
| `--verbose` | raw underlying cause on error |

## Behavior
1. Resolve origin via `config.ResolveOrigin` (env > project > global). No origin → usage error (FR exit 1).
2. Fetch `index.json` from the origin **per call** (no cache, D-catalog-fetch); for a remote origin this is a sparse `git` fetch at the resolved `@ref` (D7) with token auth (D4); for a local-path origin, read it from disk.
3. Gate `skillrigConvention` — unsupported → `IncompatibleConventionError` (FR-016).
4. Filter: substring over name/description AND all `--tag` present. Deterministic, no ranking (FR-002, N6).
5. Render two-level output.

## Output (Two-Level — cli.md P3)
- **Human (compact):** one line per matching skill (`name  version  — description` truncated) + a summary/footer hint (`N skills · run: skillrig add <name>`). Line count ≤ matches + K (K≤5) — Constitution §II shape assertion.
- **`--json`:** `{ "origin": "...", "skills": [ {name,version,namespace,description,tags,path,requires} ] }` — complete, parseable, every field `add` needs (FR-003).
- **Empty result:** human → `no skills matched`; `--json` → `{"skills":[]}`. **Exit 0** (FR-004), data to stdout.

## Exit codes
| Code | When |
|---|---|
| 0 | any well-formed query, including empty result |
| 1 | no origin configured; malformed origin; convention mismatch; auth/unreachable reaching the origin |

## Errors (errors-as-navigation, all exit 1)
- no origin → what/why/fix (`skillrig init --origin …`).
- `IncompatibleConventionError`, `AuthError`, `UnreachableError` — distinct messages (data-model §5). `--verbose` shows raw cause.

## Help (FR-018/SC-008)
Purpose line + ≥2 examples (`skillrig search`, `skillrig search terraform --tag aws`). Asserted by `TestQuickstart_SearchHelpExamples`.

## Tests
`TestQuickstart_SearchListsSkills`, `_SearchFilterByTag`, `_SearchEmptyResult` (exit 0), `_SearchJSONComplete`, `_SearchConventionMismatch`, `_SearchHelpExamples`.
