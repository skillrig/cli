# Loading rules for a coding task (`load`)

This is the **consume** path — pull the org's rules relevant to what you're about to build,
so you write code that already complies instead of getting flagged later. (It replaces the
deprecated `qodo-get-rules` skill, whose auth lookup was broken — see SKILL.md.)

```sh
S=.claude/skills/qodo-manage-rules/scripts/qodo_rules.py
python3 $S load --scope /skillrig/cli/ \
  --query $'Name: <topic query>\nCategory: <Category>\nContent: <what to check>' \
  --query $'Name: <cross-cutting query>\nCategory: <Category>\nContent: <what to check>'
```

The script merges the queries (first = priority), dedupes, enriches each hit with its
severity/state (the search endpoint is sparse), keeps only **active** rules, and prints
them grouped by severity. Empty result is valid — proceed without constraints.

## When to run it

- Right before you start writing/refactoring code, or when planning an implementation.
- **Skip if rules are already loaded** this session (look for "📋 Qodo Rules Loaded" in
  recent context) — re-running wastes calls and clutters context.
- Needs a token (`~/.qodo/auth.key` or `$QODO_API_KEY`). No token / no network → say so and
  proceed gracefully; don't crash.

## Why structured queries (not keywords)

Each rule is embedded as a three-line vector — `Name` / `Category` / `Content`. Mirror that
exact shape so the query aligns on all three dimensions. Keyword lists and flat sentences
retrieve poorly.

```
Name: {5–10 word title of the rule this task would trigger}
Category: {one of the categories below}
Content: {1–2 sentences (≥15 words) on what to check; name the tech stack if known}
```

**Categories:** `Security, Correctness, Quality, Reliability, Performance, Testability,
Compliance, Accessibility, Observability, Architecture`.

Picking the category:
- Prefer `Security` if security is plausibly in scope (highest cost if missed).
- Don't default to `Correctness` — it's over-used. Structural work → `Architecture`;
  style/naming → `Quality`; availability/error-handling → `Reliability`; logging/metrics →
  `Observability`; speed/memory → `Performance`.
- If a topic query returns <3 rules, broaden `Content` with adjacent concepts (e.g. auth →
  "token validation, credential handling, session management").

## Two-query strategy

Always pass **two** `--query` blocks:
1. **Topic query** — the task's primary concern (most specific Category + Content).
2. **Cross-cutting query** — recurring standards that apply to most changes (module
   structure, error handling, logging, testing conventions). Use `Architecture`,
   `Observability`, or `Security` as the Category.

A single topic query misses the cross-cutting rules; two give the broadest useful coverage.

## Scope

`--scope` narrows to rules tagged for a repo path, e.g. `/skillrig/cli/`. These are Qodo's
own scope strings (visible in any rule's `scopes` field), **not** necessarily your git
`org/repo`. When unsure, omit `--scope` for an org-wide search — narrowing is an
optimization, not a requirement.

## Applying what you get

| Severity | Enforcement |
|---|---|
| **ERROR** | Must comply. If you genuinely can't, flag it to the user rather than silently violating. |
| **WARNING** | Comply by default; if you deviate, say why in your response. |
| **RECOMMENDATION** | Consider; apply when it fits. |

After coding, briefly note which rules you applied and any WARNING you consciously skipped.
If a rule itself looks wrong for the task (over-broad, stale, conflicts with a documented
convention), that's the cue to switch to the **manage** path — see
`references/managing-derived-rules.md`.
