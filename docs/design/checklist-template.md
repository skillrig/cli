# Pattern-Gate Checklist

Copy this checklist into the PR that adds or changes a `skillrig` subcommand. It gates the command against the design principles in [cli.md](cli.md) before merge. Every box must be checked or explicitly justified in the PR.

> One command may legitimately span more than one pattern (e.g. `doctor` is Environment but also does Verification-Gate-style prereq checks). Fill in every section that applies.

---

## 1. Classification

- [ ] The command is classified against **exactly one primary pattern** (and any secondary ones noted): `Query` / `Vendor Mutation` / `Verification Gate` / `Environment` / `Global Management`.
- [ ] The classification is stated in the PR description and matches the [Pattern Classification](cli.md#pattern-classification) table.

## 2. Progressive Discovery (all commands)

- [ ] Has a Cobra `Long` description.
- [ ] Has **at least 2 `Examples`**.
- [ ] No args → prints subcommand list (parent) or usage (leaf), not a stack trace.
- [ ] An agent can construct a correct invocation from `--help` alone.

## 3. Errors as Navigation (all commands)

- [ ] Every error states **what failed**, the **actual raw error** (never swallowed), and a **suggested fix**.
- [ ] Guidance *supplements* the raw error — it never *replaces* it with a guess.
- [ ] Auth failures are distinguished from "missing tool" / "wrong origin" (R18).
- [ ] Errors go to **stderr**; data to **stdout**.

## 4. Output & Standard Flags (all commands)

- [ ] `--json` produces complete, untruncated, pipeable output.
- [ ] Human output is compact (truncated previews, counts) and ends with a **footer hint**.
- [ ] `--verbose` exists and prints the raw underlying git / mise / exec output.
- [ ] Exit codes follow the [table](cli.md#exit-codes): `0` pass · `1` usage/config · `2` verification failure · `3` prereq failure.

## 5. Pattern-specific constraints

### If `Query`
- [ ] Reads the committed `index.json` only; **offline**.
- [ ] Filtering is **deterministic** — no inference / LLM in the truth path (N6).

### If `Vendor Mutation`
- [ ] Writes the lock via `skillcore` only (no parallel tree-SHA / manifest-parse implementation — AP-04).
- [ ] Supports `--dry-run` (preview tree + lock changes, write nothing).
- [ ] Refuses to clobber content diverging from the locked `treeSha` without `--force` (§9b).
- [ ] **Never silently discards local edits** (R32); merge conflicts → non-zero exit + git-style conflict markers.
- [ ] `bump` *proposes* (opens a PR), never force-adopts (R13).

### If `Verification Gate`
- [ ] Runs **fully offline and deterministically** — no live/online signal in this path (R11/N1).
- [ ] Integrity primitives come from the single `skillcore` implementation shared with `bump`/`doctor`.
- [ ] Failure classes map to distinct exit codes (label-honesty / orphan / conflict markers = `2`; prereq = `3`).

### If `Environment`
- [ ] **Idempotent** and safe to retry.
- [ ] Works without a fully-configured project where sensible.
- [ ] `init` binds to an *existing* origin only — never bootstraps one (architecture §2d).

### If `Global Management`
- [ ] Genuinely fetches/restores missing skills (the restore mode project scope lacks, §3).
- [ ] Touches per-environment home dirs only — **never** the repo's project lock (R8).

## 6. Origin & substrate (all commands that resolve an origin)

- [ ] Origin is obtained from the single resolver (env > project config > global default, §2d) — **not** re-derived in-command (AP-06).
- [ ] No write credential is baked into the binary; the surface stays consume-only (architecture §2b).

---

> If a box can't be checked, the PR must say **why** in line with the [design principles](cli.md). A deliberate, justified exception is fine; a silent one is not.
