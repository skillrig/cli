# `skillrig index` — generate the origin's catalog (origin-side generator)

> **This runs INSIDE the skills-library (origin) repo, not a consumer repo.** It is the
> producer of the `index.json` that [search](search.md) consumes. Most users never run it by
> hand — the origin's `index.yml` CI runs it on merge to `main`.

`index` walks the origin's `skills/*/SKILL.md`, parses each skill's **YAML frontmatter**
(the same `ParseManifest` the consumer commands use — there is exactly one implementation),
and emits the catalog `index.json` at the origin root. The catalog is **discovery-only**:
per-skill `name`, `version`, `namespace`, `description`, `topics[]`, `path` (+ a `requires`
summary) and catalog-level `skillrigConvention` + `origin`. No per-skill tree-SHA/commit.

## When and how it runs

- **In origin CI**, on `push` to `main` touching `skills/**` (not "on release") — the
  workflow regenerates `index.json` and commits it if it changed.
- **Locally**, an origin maintainer can run `skillrig index --out index.json` to regenerate
  the catalog after adding/enriching a skill, then commit the result.

```
skillrig index --out index.json   # regenerate the catalog from skills/*/SKILL.md frontmatter
skillrig index                    # print to stdout (no --out)
```

## Single-tip, full-regenerate

The catalog reflects **only the branch tip** — one version per skill (the HEAD/latest
version). It is fully regenerated each run and accumulates nothing (no append, no GC). It is
**not** a version-history index: prior versions live in git tags, reached by
`skillrig add <skill> --pin <tag>`. A skill removed at HEAD correctly drops from `search`.

## Required frontmatter (enrichment is a checked precondition)

Each skill's `SKILL.md` must carry the skillrig block under `metadata.x-skillrig.*` —
`version`, `convention-version`, `topics`, `namespace`, and `requires` if it has backing
CLIs. `index` **fails clearly (exit 1)** on a skill missing the required `x-skillrig.version`
rather than silently emitting an under-populated catalog. (Standard agentskills.io
vendored skills carry only `name`/`description`, so this block must be added per skill.)

| Flag | Purpose |
|------|---------|
| `--out <path>` | Write the catalog to `<path>` (default: stdout) |
| `--json` | The catalog is JSON; this flag keeps parity across commands |
| `--verbose` | Show the raw underlying cause behind an error |

## Exit codes

| Code | When |
|------|------|
| `0` | Catalog generated successfully |
| `1` | Usage/config: a skill missing required `x-skillrig.version`, unreadable/invalid frontmatter, bad args, not run in an origin |

`index` is local-filesystem + `git`-tree only — no network, no auth. It never emits `2`/`3`.

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `skill "<name>" missing x-skillrig.version` | frontmatter not enriched | add the `metadata.x-skillrig.*` block to that `SKILL.md` |
| `cannot parse frontmatter in <path>` | malformed YAML frontmatter | fix the `SKILL.md` frontmatter |
| bad args | wrong invocation | the error states what/why/fix; or `skillrig index --help` |

All failures state what/why/fix and exit `1`; `--verbose` shows the raw cause.
