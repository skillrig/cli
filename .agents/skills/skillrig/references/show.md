# `skillrig show <skill>` — read one skill's full details (Query)

> The **human** counterpart to [search](search.md): drill into ONE skill and print its whole
> record — most importantly the **complete, untruncated description** that the `search` table
> clips to ~80 chars. `info` is an alias. Needs an origin (run [init](init.md) first).

`show` resolves the active origin and reads its catalog (`index.json`) through the **same**
path `search` uses, then prints a single named skill's full record: name, version, namespace,
the full description, topics, path, and backing-tool requirements. Reach for it when `search`
showed a truncated one-liner and a human wants to read the whole thing (an agent can instead
pipe `search <name> --json` to `jq`).

```
skillrig show terraform-plan-review        # full human-readable record
skillrig info terraform-plan-review        # identical (alias)
skillrig show terraform-plan-review --json # complete record for an agent / jq
```

## Lookup is exact (a point lookup, not a filter)

The skill name is matched **exactly** — the same canonical name `add` vendors by — not the
fuzzy, case-insensitive substring match `search` uses. So `show` is for a name you already
know (typically one you saw in `search`). A name the origin does not publish is an **error**
(exit 1), pointing you back at `search` — deliberately unlike `search`, where an empty result
is a clean exit 0.

## Freshness & origin

Like `search`, `show` fetches the origin's `index.json` **per call** (no local cache), resolves
the active origin through the shared resolver (`SKILLRIG_ORIGIN` > project `.skillrig/config.toml`
> global), and checks the origin's convention version before reading. A **remote** origin is
fetched over `git` (a private one uses the auto-resolved read-only token); a **local-path**
origin is read with no network.

## Output

- **Human (default)** — a labelled block: a `name  version  (namespace)` header, the `path`,
  `topics`, and `requires` lines, then the **full description** as the body, and a footer hint
  pointing at `add`.
- **`--json`** — `{ "origin": ..., "skill": { ...full catalog entry... } }`, untruncated and
  pipeable: `skillrig show terraform-plan-review --json | jq '.skill.requires'`.

| Flag | Purpose |
|------|---------|
| `--json` | Emit the complete record (origin + the whole catalog entry) on stdout |
| `--verbose` | Show the raw underlying cause behind a summary or error |

## Exit codes

| Code | When |
|------|------|
| `0` | Success — the named skill was found and printed |
| `1` | Usage/config: no origin configured, the named skill is **not in the origin**, unreachable/auth/incompatible-convention fetching the catalog, bad args |

`show` never emits `2`/`3` (those are reserved for verification/prerequisite gates).

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `skill "<name>" not found in origin ...` | no skill by that exact name in the catalog | run `skillrig search` to list the real names, then `show` one |
| `no origin configured` | no resolvable origin | `skillrig init --origin OWNER/REPO`, or set `SKILLRIG_ORIGIN` |
| `... is unreachable` / `authentication ... failed` / `... not found` / `convention version N` | catalog fetch/gate failures (shared with `search`) | see [search.md](search.md) — same typed errors and fixes |

All failures state what/why/fix and exit `1`; `--verbose` shows the raw cause. Errors to
stderr, data to stdout (so `skillrig show <name> --json 2>/dev/null | jq .` stays clean).
