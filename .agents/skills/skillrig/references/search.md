# `skillrig search [QUERY...]` — discover skills in your origin (Query)

> Find approved skills in your **configured origin** before vendoring one with [add](add.md).
> Needs an origin (run [init](init.md) first). Reads the origin's catalog (`index.json`).

`search` reads the origin's generated catalog and lists the skills that match. It is the
**discovery** step of the `init → search → add → verify` loop — use it to learn a skill's
exact name and version before `skillrig add <skill>`. Scope: it finds an approved skill in
*your origin* (distinct from the generic `find-skills` skill, which discovers skills from
anywhere).

## How matching works (deterministic — no fuzzy/semantic ranking)

- **Free-text `[QUERY...]`** — case-insensitive **token-AND substring** over each skill's
  `name` + `description` + `topics`. Every query token must match somewhere; multiple tokens
  are ANDed. No query = list everything.
- **`--topic <T>` (repeatable)** — a separate **exact-string** filter applied after the text
  match: keep only skills carrying topic `<T>`. (`--topic`, not `--tag`/`--filter`.)
- **Order is deterministic** — a fixed relevance bucket (exact-name > name-match >
  topic-match > description-match) then lexicographic by `name`. Same inputs → same output,
  always (no TF-IDF, no inference).

```
skillrig search                       # list all skills in the origin
skillrig search terraform             # text match on name/description/topics
skillrig search plan --topic aws      # text match AND the 'aws' topic
skillrig search --topic platform-team # topic filter only
```

## Freshness & origin

`search` fetches the origin's `index.json` **per call** (no local cache this slice), so
results always reflect the origin's current tip. It resolves the active origin through the
shared resolver (`SKILLRIG_ORIGIN` > project `.skillrig/config.toml` > global) and checks the
origin's convention version before reading. A **remote** origin is fetched over `git` (a
private one uses the auto-resolved read-only token); a **local-path** origin is read with no
network.

## Output

- **Human (default)** — one compact line per match (name, version, namespace, truncated
  description, `requires` summary) + a footer hint pointing at `add`. An **empty match set is
  a clean success (exit 0)** with a "no matches" hint, *not* an error.
- **`--json`** — the complete catalog entries, untruncated, pipeable:
  `skillrig search terraform --json | jq '.[].name'`.

| Flag | Purpose |
|------|---------|
| `--topic <T>` | Exact-string topic filter (repeatable), applied after the text match |
| `--json` | Emit the complete catalog entries on stdout |
| `--verbose` | Show the raw underlying cause behind a summary or error |

## Exit codes

| Code | When |
|------|------|
| `0` | Success — **including zero matches** (empty result is not a failure) |
| `1` | Usage/config: no origin configured, unreachable/auth/incompatible-convention fetching the catalog, bad args |

`search` never emits `2`/`3` (those are reserved for verification/prerequisite gates).

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `no origin configured` | no resolvable origin | `skillrig init --origin OWNER/REPO`, or set `SKILLRIG_ORIGIN` |
| `... is unreachable` (**UnreachableError**) | network failure / wrong host | check connectivity/proxy/host; retry |
| `authentication ... failed` (**AuthError**) | private origin, no/invalid token | `gh auth login`, or export `GH_TOKEN`/`GITHUB_TOKEN` |
| `origin "<OWNER/REPO>" not found` (**NotFoundError**) | origin missing, or private with no token | check spelling; **if private, authenticate** |
| `origin ... uses convention version N` | origin layout unsupported by this binary | update `skillrig` or use a compatible origin |

All failures state what/why/fix and exit `1`; `--verbose` shows the raw cause. Errors to
stderr, data to stdout (so `skillrig search --json 2>/dev/null | jq .` stays clean).
