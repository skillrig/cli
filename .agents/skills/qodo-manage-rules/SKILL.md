---
name: qodo-manage-rules
description: >-
  The single skill for an org's Qodo coding rules — both CONSUMING them and ADMINISTERING
  them. (1) LOAD the rules relevant to a coding task and apply them while writing code:
  use whenever you're about to write, edit, refactor, or review code, or start planning an
  implementation, and want to comply with the org's standards up front ("what rules apply
  here", "load our coding rules", "check our conventions before I build this"). Skip if
  rules are already loaded this session. (2) MANAGE the rule catalog — list, search,
  inspect, modify (severity, content, scope), and deactivate/reactivate rules via the Qodo
  rules API: use whenever a Qodo automated review flags something that is actually correct
  or a deliberate decision and you want to fix the RULE not the code ("this Qodo rule is
  wrong / over-broad / stale", "Qodo keeps flagging X", "the rule contradicts our
  convention", "loosen / narrow / scope / carve-out that rule", "disable / deactivate /
  turn off this rule", "change the rule from error to warning", "which rule produced this
  review comment"), or to triage a declined PR review rule-by-rule. Trigger even when
  "Qodo" isn't named, if the user is loading coding standards or reacting to an automated
  review by changing the governing rule. This skill REPLACES the deprecated qodo-get-rules
  skill (which had a broken auth lookup). NOT for posting PR comments.
---

# Qodo Rules

Qodo coding rules are **org-wide**: every teammate's PR is graded against them. This skill
is the one place to work with them, in two modes:

- **Consume** (`load`) — pull the rules relevant to what you're about to build and apply
  them while coding, so you comply up front. *(This is the job the old `qodo-get-rules`
  skill did; it's folded in here and fixed — see Auth.)*
- **Administer** (`list` / `find` / `get` / `update` / `set-state`) — change the rules
  themselves when one is wrong, over-broad, or stale. Higher stakes: an edit changes what
  everyone's review flags, so writes are dry-run by default.

## The one tool

All API plumbing is in `scripts/qodo_rules.py` (stdlib only, no deps). Use it rather than
hand-rolling curl — the write path is a **full-document PUT**, so the script does the
read-modify-write for you (a hand-built partial body would blank out other fields).

```sh
S=.claude/skills/qodo-manage-rules/scripts/qodo_rules.py

# CONSUME — load rules for the current task (apply while coding)
python3 $S load --scope /skillrig/cli/ \
  --query $'Name: <topic>\nCategory: <Cat>\nContent: <what to check>' \
  --query $'Name: <cross-cutting>\nCategory: <Cat>\nContent: <what to check>'

# ADMINISTER — read
python3 $S list --all
python3 $S get 782313
python3 $S search "verification offline" --scope /skillrig/cli/
python3 $S find "go:build integration"        # resolve ruleId(s) from review text → enriched

# ADMINISTER — write (default DRY-RUN; add --apply to send the PUT)
python3 $S set-state 782685 inactive          # deactivate (reversible — preferred over delete)
python3 $S set-state 782685 inactive --apply
python3 $S update 782313 --severity warning
python3 $S update 782313 --append-content "- Exception: the fetch layer is the feature."
python3 $S update 782313 --content-file /tmp/new_content.md --apply
```

Add `--json` for complete machine-readable output; `--limit N` bounds human rows.

## Auth (differs from the old qodo-get-rules)

The token is a raw `sk-...` bearer from **`~/.qodo/auth.key`** (or `$QODO_API_KEY`). It is
**not** in `~/.qodo/config.json` — that file is only UI prefs. The old `qodo-get-rules`
skill looked for `config.json:API_KEY`, which doesn't exist, so it was effectively broken in
this environment; that's the main reason this skill replaces it. The same bearer token does
both reads and writes (verified). The script never prints the token.

## Mode 1 — load rules for a coding task

Run `load` with two structured queries (a topic query + a cross-cutting query) right before
you write or plan code. It prints the relevant active rules grouped by severity; apply
ERROR (must), WARNING (should), RECOMMENDATION (consider). **Skip if rules are already
loaded this session** ("📋 Qodo Rules Loaded" in recent context). Empty result is valid —
proceed without constraints; never crash on no token / no network.

Full query-writing guidance (the Name/Category/Content format, category selection, the
two-query strategy, scope, and the severity-application table) is in
**`references/loading-rules.md`** — read it before composing queries.

## Mode 2 — manage a rule you disagree with

When a Qodo finding is actually a correct/deliberate decision, fix the rule, not the code:

1. **Find it.** `python3 $S find "<phrase from the review comment>"` → ruleId, severity,
   state, and the `source` file it was derived from.
2. **Read it.** `python3 $S get <ruleId>` — confirm the content matches what was enforced.
3. **Decide** (see `references/managing-derived-rules.md` for the decision tree):
   - **Modify** when the intent is right but too broad/strict → narrow `content` (carve-out)
     or downgrade `severity`.
   - **Deactivate** (`set-state inactive`) when the rule shouldn't apply here at all (e.g.
     derived from a generic vendored skill and conflicting with the project's own
     convention). Reversible — preferred over deletion; this skill exposes no hard delete.
4. **Preview, then apply.** Writes default to a dry-run printing before/after. Confirm, then
   re-run with `--apply`.
5. **Fix the source.** Qodo re-derives rules from files (`CLAUDE.md`, vendored `SKILL.md`s,
   design docs). Update the `source` file too, or the rule comes back wrong next scan.

## Safety

- **Default to dry-run.** Pass `--apply` only after the diff is shown and clearly correct —
  these edits are org-wide.
- **Prefer deactivate over delete.** No undo for a hard delete, so this skill doesn't offer
  one; deactivation is a clean round-trip.
- **Confirm gate-weakening writes with the user** — severity downgrades on `error` rules and
  deactivations weaken the org's gates; call that out.
- **Never paste the token**, commit it, or write it to a tracked file.

## References

- `references/loading-rules.md` — Mode 1: structured query format, two-query strategy,
  scope, and severity application.
- `references/managing-derived-rules.md` — Mode 2: modify-vs-deactivate decision tree, the
  "rules are derived from source files" model, and the worked PR-triage playbook.
- `references/api-contract.md` — endpoints, auth resolution, request/response shapes, and
  the list-vs-search schema gotcha (`ruleId` vs `id`).
