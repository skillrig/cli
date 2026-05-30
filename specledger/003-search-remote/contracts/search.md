# Contract: `skillrig search` (Query pattern)

Discover skills published by the resolved origin. Read-only. Reads the origin's `index.json` (D2/D7), gates `skillrigConvention` (D-convention), matches/filters **deterministically** (D8, N6).

## Synopsis
```
skillrig search [QUERY...] [--topic T ...] [--json] [--verbose]
```

## Flags
| Flag | Meaning |
|---|---|
| `[QUERY...]` | free-text query; case-insensitive **token-AND substring** over `name + description + topics` — a skill matches iff **every** whitespace-separated term is a substring of that concatenated text (D8) |
| `--topic T` | repeatable; structured filter — a skill must carry **all** requested topics (AND); exact-string, case-insensitive (A3). (Named "topic", not "tag" — git tags are version pins.) |
| `--json` | complete, untruncated machine output (every catalog field) |
| `--verbose` | raw underlying cause on error |

## Behavior
1. Resolve origin via `config.ResolveOrigin` (env > project > global). No origin → usage error (FR exit 1).
2. Fetch `index.json` from the origin **per call** (no cache, D-catalog-fetch); for a remote origin this is a sparse `git` fetch at the resolved `@ref` (D7) with token auth (D4); for a local-path origin, read it from disk.
3. Gate `skillrigConvention` — unsupported → `IncompatibleConventionError` (FR-016).
4. **Match** (in-memory, `pkg/skillcore`, AP-04): keep entries where every QUERY term is a substring of `name+description+topics` AND every `--topic` is present. Empty QUERY + no `--topic` ⇒ list all.
5. **Order** deterministically (D8, N6): fixed relevance bucket — exact-name `3` > name-hit `2` > topic-hit `1` > description-only `0` — then **lexicographic by `name`** (unique, total order). No fuzzy/semantic/learned ranking.
6. Render two-level output.

## Output (Two-Level — cli.md P3)
- **Human (compact):** one line per matching skill (`name  version  — description` truncated) + a summary/footer hint (`N skills · run: skillrig add <name>`). Line count ≤ matches + K (K≤5) — Constitution §II shape assertion.
- **`--json`:** `{ "origin": "...", "skills": [ {name,version,namespace,description,topics,path,requires} ] }` — complete, parseable, every field `add` needs (FR-003).
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
Purpose line + ≥2 examples (`skillrig search terraform`, `skillrig search --topic aws`). Asserted by `TestQuickstart_SearchHelpExamples`.

## Tests
`TestQuickstart_SearchQueryMatchesNameDesc`, `_SearchListsSkills`, `_SearchFilterByTopic`, `_SearchOrderingDeterministic`, `_SearchEmptyResult` (exit 0), `_SearchJSONComplete`, `_SearchConventionMismatch`, `_SearchHelpExamples`.
