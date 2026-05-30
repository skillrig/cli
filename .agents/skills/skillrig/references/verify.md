# `skillrig verify` — prove vendored skills are unmodified (Verification Gate)

> Offline, deterministic, read-only integrity gate. **Needs no origin.** The headline CI use.

Checks **this repository's** vendored skills (project scope: `.agents/skills` vs the committed
`.skillrig/skills-lock.json`). It aggregates **all** findings in one run (never stops at the
first failure) and takes no arguments. Two checks:

- **Label-honesty**: recompute each locked skill's git tree-SHA from its **committed** content
  and compare to the lock.
- **Orphan / completeness**: the on-disk skill set under `.agents/skills` must equal the locked
  set — an unrecorded skill (`orphan`) or a recorded-but-absent one (`missing`) fails.

## Per-skill verdicts (the `status` field)

| Status | Meaning | Fix |
|--------|---------|-----|
| `ok` | committed content matches the recorded fingerprint | — |
| `mismatch` | committed content differs from the record (label-honesty failure) | re-`add` from origin, or restore the approved content |
| `orphan` | on disk but no lock entry (untracked — the primary supply-chain risk) | `skillrig add` it, or remove it |
| `missing` | lock entry whose files are absent | restore the files, or remove the lock entry |
| `dirty` | locked + present but **uncommitted / locally modified** | commit it (verify checks committed content) — *distinct* from `mismatch` |

## CRITICAL: verify is integrity-only — a missing backing tool is NOT a failure

`verify` does **no** prerequisite/eligibility check. A skill may declare backing tools in its
`SKILL.md` frontmatter (`metadata.x-skillrig.requires`); if those tools are absent in the
environment, `verify` **still passes** (it checks content, not runnability). Prerequisite checking is a future `doctor`
concern (the reserved exit `3`), never emitted here. Don't tell a user that verify failed
because a tool isn't installed — that's never the cause.

## Exit codes (load-bearing — branch on these in CI/agents)

| Code | When |
|------|------|
| `0` | All verdicts `ok` (**including** the empty case: no skills / no lock → clean pass) |
| `1` | Usage/config: malformed or unreadable lock, bad args, **not inside a git repo** |
| `2` | Verification failure: any `mismatch`, `orphan`, `missing`, or `dirty` |
| `3` | **Never emitted** — reserved for `doctor`'s prerequisite class |

A malformed lock is a **`1`**, not a `2` — keep that distinction when scripting (`2` = content
drifted; `1` = couldn't even run the check).

`verify --json` keys: `ok, counts{verified,mismatch,orphan,missing,dirty}, verdicts[]` with each
verdict carrying `name, path, status, expectedTreeSha, actualTreeSha, reason`. Diagnostics go to
stderr, so `skillrig verify --json 2>/dev/null | jq .` stays clean JSON.

## Workflow patterns

1. **CI merge gate** (headline): run `skillrig verify` (or `--json` for an agent); exit `0`
   proceeds, `2` blocks with a per-skill report, `1` is a setup/config problem.
2. **Triage a failure**: `skillrig verify --json | jq '.verdicts[] | select(.status != "ok")'`.
3. **`dirty`?** commit the vendored files (verify checks committed content), then re-verify.
4. **`mismatch`?** the committed content no longer matches its recorded version — re-`add
   <skill> --force` to restore, or investigate the change. (See [add.md](add.md).)

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `cannot read .skillrig/skills-lock.json` | malformed/unreadable lock (exit `1`, **not** `2`) | check/repair the file, or re-vendor with `skillrig add` |
| `not a git repository` | run outside a repo | run inside the repo |
| bad/extra args | `verify` takes none | the error states what/why/fix; run `skillrig verify` (add `--json`) |

All failures state what/why/fix; `--verbose` shows the raw cause. Human output is compact (a
summary line per finding + a footer hint); use `--json` only when a program/agent parses it.
