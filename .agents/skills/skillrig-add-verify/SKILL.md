---
name: skillrig-add-verify
description: >-
  Vendor agent skills from your configured origin into a repo with `skillrig add`, and
  prove vendored skills are exactly what was recorded with `skillrig verify`. Use this
  whenever the user wants to "add/vendor/pull in a skill" from their org's skills library,
  "lock" or "pin" a skill into the repo, set up a CI gate that the committed skills are
  unmodified, "check/verify our skills haven't been tampered with", understand the
  vendor→commit→verify round-trip, read a verify pass/fail or exit code, or debug a
  `mismatch` / `orphan` / `missing` / `dirty` verdict. Trigger even when the user doesn't
  name the command — e.g. "make sure nobody changed our skills", "why did the skills check
  fail in CI", or "our agent skill got edited, how do I restore it". Also use to explain
  that a missing backing tool is NOT a verify failure (integrity-only).
license: MIT
metadata:
  author: skillrig
  cli: skillrig
  user-invocable: true
---

# skillrig-add-verify Skill

**When to Load**: The user wants to vendor a skill from their origin (`skillrig add`),
verify the repo's vendored skills against their recorded identities (`skillrig verify`),
gate CI on that verification, or interpret/debug a verify outcome (exit codes, per-skill
verdicts) — or whenever `skillrig add` / `skillrig verify` is referenced.

## The promise these two commands make

> *The skill your agent runs is exactly the version that was reviewed and approved.*

`add` records a tamper-evident fingerprint of a skill's content when it is vendored;
`verify` later recomputes that fingerprint and fails if it drifted. Both use the **same**
git tree-SHA, computed by shelling `git`, so the value written at vendor time and the value
checked at verify time **cannot diverge** — the gate cannot lie. This is the whole point;
keep it intact (never hand-edit the lock, never mutate vendored files to "fix" a mismatch).

## `skillrig add <skill>` — vendor a skill (Vendor Mutation)

Vendors `<skill>` from the repo's **configured origin** into the canonical
`.agents/skills/<skill>/`, byte-identical and mode-preserving (it injects nothing), and
records its identity — `version`, `commit`, `treeSha`, `path` — in `.skillrig/skills-lock.json`.
Offline and consume-only. **Requires a git repository** (project scope).

- **Origin, not a path**: `add` resolves the active origin via the shared resolver
  (`SKILLRIG_ORIGIN` > project `.skillrig/config.toml` > global) exactly like every command.
  There is **no** `--from`/path argument.
- **Local origin (this release)**: the configured `OWNER/REPO` is read from a local git
  checkout at `./OWNER/REPO`, relative to where you run `add` (your repo root) — no network.
  So `init --origin my-org/my-skills` expects that library checked out at `./my-org/my-skills`
  (keep it out of your index, e.g. `echo 'my-org/' >> .git/info/exclude`).
- **Idempotent**: re-adding identical content reports success and changes nothing
  (`action: "unchanged"`).
- **Never clobbers**: if the on-disk copy diverges from the recorded fingerprint, `add`
  **refuses** without `--force` (so local edits are never lost silently). It does **not**
  three-way-merge — re-vendoring the same version has no upstream change to merge; that is a
  future `bump`. Use `--force` to overwrite with the origin's content, or revert your edits.
- **`--dry-run`** previews placement + record changes and writes nothing.

After `add`, **commit** `.agents/skills/<skill>` + the lock, then run `verify` — verify
checks the *committed* tree.

| Flag | Purpose |
|------|---------|
| `--dry-run` | Report what would be vendored/recorded; write nothing |
| `--force` | Overwrite a vendored skill whose on-disk content diverges from the lock |
| `--json` | Emit the complete `AddResult` on stdout |
| `--verbose` | Show underlying paths / raw git cause behind summaries and errors |

`--json` keys (always present): `ok, name, version, path, commit, treeSha, action, dryRun`;
`action ∈ {vendored, unchanged, overwritten}`.

## `skillrig verify` — prove vendored skills are unmodified (Verification Gate)

Checks **this repository's** vendored skills (project scope: `.agents/skills` vs the
committed `.skillrig/skills-lock.json`) — offline, deterministic, **read-only**. It
aggregates **all** findings in one run (never stops at the first failure). Takes no args.

Two checks:
- **Label-honesty**: recompute each locked skill's tree-SHA from its **committed** content
  and compare to the lock.
- **Orphan / completeness**: the on-disk skill set under `.agents/skills` must equal the
  locked set — an unrecorded skill (`orphan`) or a recorded-but-absent one (`missing`) fails.

### Per-skill verdicts (the `status` field)

| Status | Meaning | Fix |
|--------|---------|-----|
| `ok` | committed content matches the recorded fingerprint | — |
| `mismatch` | committed content differs from the record (label-honesty failure) | re-`add` from origin, or restore the approved content |
| `orphan` | on disk but no lock entry (untracked — the primary supply-chain risk) | `skillrig add` it, or remove it |
| `missing` | lock entry whose files are absent | restore the files, or remove the lock entry |
| `dirty` | locked + present but **uncommitted / locally modified** | commit it (verify checks committed content) — *distinct* from `mismatch` |

### CRITICAL: verify is integrity-only — a missing backing tool is NOT a failure

`verify` does **no** prerequisite/eligibility check. A skill may declare `[[requires]]`
backing tools in its `skill.toml`; if those tools are absent in the environment, `verify`
**still passes** (it checks content, not runnability). Prerequisite checking is a future
`doctor` concern (the reserved exit `3`), never emitted here. Don't tell a user that verify
failed because a tool isn't installed — that's never the cause.

## Exit codes (load-bearing — branch on these in CI/agents)

| Code | When |
|------|------|
| `0` | All verdicts `ok` (**including** the empty case: no skills / no lock → clean pass) |
| `1` | Usage/config: malformed or unreadable lock, bad flags, **not inside a git repo** |
| `2` | Verification failure: any `mismatch`, `orphan`, `missing`, or `dirty` |
| `3` | **Never emitted** — reserved for `doctor`'s prerequisite class |

A malformed lock is a **`1`**, not a `2` — keep that distinction when scripting (a `2`
means "content drifted"; a `1` means "I couldn't even run the check").

`verify --json` keys: `ok, counts{verified,mismatch,orphan,missing,dirty}, verdicts[]` with
each verdict carrying `name, path, status, expectedTreeSha, actualTreeSha, reason`.
Diagnostics go to stderr, so `skillrig verify --json 2>/dev/null | jq .` stays clean JSON.

## Workflow patterns

1. **Vendor + lock a skill**:
   `skillrig add terraform-plan-review` → `git add -A && git commit` → `skillrig verify`.
2. **CI merge gate** (the headline use): run `skillrig verify` (or `--json` for an agent);
   exit `0` proceeds, `2` blocks with a per-skill report, `1` is a setup/config problem.
3. **Recover a tampered skill**: a `mismatch`/`dirty` verdict → re-vendor from origin with
   `skillrig add <skill> --force`, then commit and re-verify.
4. **Found an `orphan`**: either `skillrig add` it (record it) or delete the directory.
5. **Preview before writing**: `skillrig add <skill> --dry-run`.

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `no origin configured` | no `SKILLRIG_ORIGIN` / project / global origin | `skillrig init --origin OWNER/REPO`, or set `SKILLRIG_ORIGIN` |
| `skill "<name>" not found in origin` | no `skills/<name>/` at the configured origin | check the name against the origin's `skills/` |
| `refusing to overwrite <path>` | on-disk content diverges from the record | re-run with `--force`, or revert local edits |
| `not a git repository` | `add`/`verify` run outside a repo | run inside the repo (or `git init` first) |
| `cannot read .skillrig/skills-lock.json` | malformed/unreadable lock (exit `1`, **not** `2`) | check/repair the file, or re-vendor with `skillrig add` |

All failures state what/why/fix; add `--verbose` for the raw underlying cause. Errors go to
stderr, data to stdout.

## Token efficiency

Human output is compact (a summary line per finding + a footer hint). Use `--json` only when
a program/agent will parse the verdicts; otherwise the compact human form keeps context small.

## Related

- `skillrig-init` — bind the repo to an origin first (this skill assumes an origin is set;
  see that skill for origin references, `@REF` branch tracking, and resolution precedence).
