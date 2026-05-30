# Contract: `skillrig index` — origin-side catalog generator (D2)

Generates the origin's `index.json` from the skills' `SKILL.md` frontmatter. Runs **inside the origin repo** (locally or in its `index.yml` CI on merge to `main`). Origin-maintainer facing (spec US5). **Not** a consumer command and **not** one of the five cli.md consumer patterns — classify as an *origin-side generator* (propose a short cli.md note; FR-024).

## Synopsis
```
skillrig index [--out <path>] [--json] [--verbose]
```

## Flags
| Flag | Meaning |
|---|---|
| `--out <path>` | write the catalog to `<path>` (default: `index.json` at repo root) |
| `--json` | machine summary of what was generated (counts, path) |
| `--verbose` | raw cause on error |

## Behavior
1. Locate the origin repo root + `skills_dir` (from `.skillrig-origin.toml`; default `skills`).
2. Walk `<skills_dir>/*/SKILL.md`; `ParseManifest` each (the **same** parser `add`/`verify`/`search` use — AP-04).
3. Project into catalog entries (`path` = dir relative to repo root); **sort by `name`** (determinism).
4. Marshal `index.json` with stable key order + trailing newline; carry `skillrigConvention: 1` + `origin`.
5. Write to `--out`. **Single-tip, full-regenerate** — overwrite wholesale; no aggregation, no GC (D2).

## Output
- **Human:** `indexed N skills → <path>` + footer. Bounded.
- **`--json`:** `{ "out": "...", "skills": N, "convention": 1 }`.

## Exit codes
| Code | When |
|---|---|
| 0 | catalog written (incl. no-change rewrite) |
| 1 | not in an origin repo / unreadable `skills_dir` / a malformed `SKILL.md` frontmatter |

## Determinism & ground-truth (SC-009)
- Regenerating over an unchanged skill set is **byte-identical**.
- **Contract test (the oracle):** `skillrig index` over the PoC origin fixture MUST equal the committed `index.json` (producer == artifact). `TestQuickstart_IndexMatchesCommitted`.

## Errors
Malformed frontmatter → what/why/fix naming the offending `SKILL.md`. Not-in-origin → fix (`run inside the origin repo`). `--verbose` → raw cause.

## Tests
`TestQuickstart_IndexGenerates` (fields incl. tags present), `_IndexDeterministic` (twice → identical), `_IndexMatchesCommitted` (oracle), `_IndexMalformedFrontmatter`.
