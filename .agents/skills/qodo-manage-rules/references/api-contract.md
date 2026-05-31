# Qodo Rules API — contract

Reverse-engineered live (the management calls are not in any published doc) and verified
with a real `auth.key` bearer token on 2026-05-31.

## Base URL resolution

Priority, highest first:
1. `$QODO_API_URL` (or `QODO_API_URL` in `~/.qodo/config.json`) → `{value}/rules/v1`
2. `$QODO_ENVIRONMENT_NAME` → `https://qodo-platform.{env}.qodo.ai/rules/v1`
3. default → `https://qodo-platform.qodo.ai/rules/v1`

## Auth

- `Authorization: Bearer <token>` where the token is a raw `sk-...` string from
  **`~/.qodo/auth.key`** (or `$QODO_API_KEY`). **Not** `config.json:API_KEY` — that file
  holds only UI prefs (theme, diffDisplay, …).
- The browser portal authenticates the same calls via a session cookie (Chrome's HAR
  export strips both the `Authorization` *and* `Cookie` header values, so a captured HAR
  shows neither — don't conclude the call is unauthenticated). The bearer token is
  accepted for **both reads and writes** — confirmed by a live `PUT` returning `200`.
- Attribution headers used by this skill: `request-id` (a fresh UUID per call) and
  `qodo-client-type: skill-qodo-manage-rules` (the portal sends `portal`).

## Endpoints

| Op | Method + path | Notes |
|----|---------------|-------|
| List | `GET /rules?page=N` | Paginated; returns `{page, totalCount, rules[]}`. 50 per page. Full rule schema. |
| Get one | `GET /rule/{ruleId}` | Full rule schema. Note **singular** `/rule/` (the list is plural `/rules`). |
| Search | `POST /rules/search` | Semantic. Body `{query, top_k, scopes?}`. **Sparse** result shape — see gotcha. |
| Update | `PUT /rule/{ruleId}` | **Full-document replace.** Used for content/severity/scope edits AND (de)activation via `state`. |

No hard-delete endpoint is used by this skill. A `DELETE /rule/{ruleId}` very likely
exists (same `/rule/{id}` shape) but is intentionally not wired up — deactivation via
`PUT … "state":"inactive"` is the reversible equivalent.

## ⚠️ Schema gotcha: list/get vs search

These two return **different shapes** for the same rule:

- **List / Get** → rich object. Id key is **`ruleId`** (int). Keys include:
  `ruleId, name, category, severity, state, content, goodExamples, badExamples,
  source, sourceType, sourceUri, sourceUris, scopes, suggestionType, insights,
  similaritiesCount, url, createdAt, updatedAt`.
- **Search** → sparse object: only `{id, name, content}`. Id key is **`id`** (int), and
  there is **no** severity/state/source. To act on a search hit you must `get` it for the
  full record. `qodo_rules.py` normalizes the id key via `_rid()` and `find` auto-enriches
  with a follow-up `GET`.

## PUT body — full-document replace

A `PUT` must carry the whole mutable document; omitted fields are wiped. Send only the
**server-accepted mutable fields** (everything else is server-managed and rejected/ignored):

```
name, category, severity, content, goodExamples, badExamples, sourceUri, scopes, state
```

Drop these from the body (server-managed): `ruleId` (it's in the URL), `createdAt`,
`updatedAt`, `source`, `sourceType`, `sourceUris`, `suggestionType`, `insights`,
`similaritiesCount`, `url`.

Correct pattern (what the script does): `GET /rule/{id}` → keep mutable fields → mutate →
`PUT /rule/{id}`. Response echoes the updated full rule with a fresh `updatedAt`.

### Field value vocabularies (observed)
- `severity`: `error` | `warning` | `recommendation`
- `state`: `active` | `inactive`
- `category`: e.g. `Security, Correctness, Quality, Reliability, Performance, Testability,
  Compliance, Accessibility, Observability, Architecture`
- `scopes`: list of repo-path globs like `/skillrig/cli/` (narrows where the rule applies)

## Worked example — the `PUT` that deactivated rule 782313

```
PUT https://qodo-platform.qodo.ai/rules/v1/rule/782313
Content-Type: application/json
Authorization: Bearer sk-...

{ "name": "...", "category": "Architecture", "severity": "warning",
  "content": "...", "goodExamples": "...", "badExamples": "...",
  "sourceUri": "skillrig/cli/CLAUDE.md", "scopes": ["/skillrig/cli/"],
  "state": "inactive" }            ← the only changed field
→ 200, body = full updated rule with new updatedAt
```
