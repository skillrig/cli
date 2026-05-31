# Managing derived rules — decision tree & PR-triage playbook

## Rules are derived from source files

Qodo doesn't invent rules from nothing — it **derives** them by scanning repo files
(`CLAUDE.md`, vendored `SKILL.md`s, design docs) and distilling each into a rule with a
`source` / `sourceUri` pointing back at the file. Two consequences:

1. **A rule can go stale relative to its source.** If the source file evolves (e.g. the
   codebase legitimately grows a network/fetch layer), an old rule derived from an earlier
   version can keep enforcing the outdated intent.
2. **Deactivating a rule is not permanent if you leave the source wrong.** The next scan
   can re-derive it. So when you fix a rule, also fix the file it came from — otherwise it
   comes back, and humans reading the source still see the stale rule.

## Decision tree: a Qodo finding is wrong — now what?

```
Is the rule's intent still valid, just mis-applied here?
├── YES → MODIFY the rule
│        ├── Too broad / catches a legitimate case → narrow `content` (add a carve-out
│        │     exception) or add/adjust `scopes`.
│        └── Right check, wrong weight → change `severity` (e.g. error → warning).
│        └── Then: update the SOURCE file so the narrowed intent is documented there too.
│
└── NO → the rule shouldn't govern this repo at all
         ├── It conflicts with this project's own convention, and a *different* rule
         │     already encodes the right convention → DEACTIVATE the wrong rule
         │     (`set-state inactive`). Prefer this over delete (reversible).
         └── Then: if the wrong rule was derived from a file in THIS repo (e.g. a vendored
               generic skill), note that the source will keep re-deriving it; consider
               de-scoping the source or documenting the project override in CLAUDE.md.
```

Always **preview the dry-run, get user confirmation for org-wide writes, then `--apply`**.
Downgrading an `error` and deactivating both weaken a shared gate — say so explicitly.

## Worked playbook: the three PR #8 declines (skillrig 003)

This is the canonical example the skill was built around. A Qodo review declined PR #8 for
three reasons, all of which were actually correct/deliberate decisions:

| # | Finding | Rule (`find` it) | Right action |
|---|---------|------------------|--------------|
| 1 | Unapproved `yaml.v3` dependency | the "no new Go deps" rule (already **deleted** by the maintainer) | Resolved by deletion. The dep was user-approved + documented (CLAUDE.md, spec A7). Optionally re-create later as an *allowlist* that includes `yaml.v3`, but YAGNI under the pre-release "no backward-compat" marker. |
| 2 | `FetchSkill` does network (HTTPS clone) | **782313** *Disallow runtime network access in application code* (warning, `/skillrig/cli/`) — stale: CLAUDE.md now documents a deliberate fetch/network boundary | **MODIFY**: append a carve-out — the fetch layer used by `add`/`search` is the feature; only `verify`/`lint` must stay offline. Leave the narrow sibling **783461** *Verification commands must be fully offline* untouched — it's correct. Update CLAUDE.md if the carve-out isn't already explicit there. |
| 3 | Integration test lacks `//go:build integration` | **782685** *Integration tests must use //go:build integration build tag* (error, derived from the **vendored** `golang-testing/SKILL.md`) | **DEACTIVATE**: it conflicts with the project's deliberate dir-based convention, already encoded by **782315** + **783442** (integration = `test/` dir, `TestQuickstart_*`, no tag). Qodo declined this same rule on the 002 PR — it keeps re-flagging. The generic vendored skill shouldn't outrank the project's own CLAUDE.md. |

Commands for #2 and #3 (dry-run first, then `--apply`):

```sh
S=.claude/skills/qodo-manage-rules/scripts/qodo_rules.py
# #2 — narrow the over-broad network rule
python3 $S update 782313 --append-content \
  "- Exception: the fetch layer (clone/sparse-fetch in add/search to pull from a remote origin) is the feature and legitimately performs network I/O. This rule applies only to verify/lint code paths, which must stay offline."
# #3 — deactivate the build-tag rule that conflicts with the dir-based convention
python3 $S set-state 782685 inactive
```

The general lesson: **don't relitigate a correct decision in the PR thread — fix the rule
(and its source) so the gate reflects reality for everyone.**
