# Contract: `skillrig verify`

**Pattern**: Verification Gate — [cli.md](../../../docs/design/cli.md) Pattern Classification. MUST be offline + deterministic, exit-code driven, **no online/inferential signal** (AP-02). Read-only.
**Purpose**: Prove **this repository's** vendored skills (**project scope** — `.agents/skills` checked against the committed `.skillrig/skills-lock.json`) are exactly what was recorded — **label-honesty** (tree-SHA) + **orphan/completeness** (on-disk set = locked set). Integrity only; **no** prerequisite check (that is `doctor`; exit `3` not emitted). **Project-scope**: it verifies the current repo, *not* global/user-scope skills. Requires a git repository; needs no origin and no network.

## Synopsis

```
skillrig verify [--json] [--verbose]
```

## Flags & Args

`Args`: none (`cobra.NoArgs`) — verifies the whole repo. `--json` (complete `VerifyReport` on stdout), `--verbose` (raw git/path causes). No `--dry-run`/`--force` (read-only).

## Help (Progressive Discovery)

The cobra `Short`/`Long`/`Example` MUST state project scope (other code/test readers never see this spec — the help text is where "project-level" must live):

```
Short: Check THIS repo's vendored skills match their recorded versions (project scope)
Long:  verify checks the PROJECT's vendored skills (.agents/skills) against the
       committed lock (.skillrig/skills-lock.json) — label-honesty (git tree-SHA)
       + orphan/completeness — offline and deterministic. PROJECT-SCOPE: it verifies
       THIS repository, not global/user-scope skills. Exit 0 ok / 1 usage / 2 failure.

Examples:
  # Verify this repo's vendored skills match their recorded versions (project-scope CI gate)
  skillrig verify

  # Machine-readable per-skill verdicts for an agent / jq
  skillrig verify --json
```

## Behavior (research D1/D2; aggregates ALL findings — never first-fail)

1. **Read the lock** `.skillrig/skills-lock.json` (`skillcore.ReadLock`). Absent → treat as empty (zero locked skills). Unparseable / wrong `lockfileVersion` → usage error (exit 1), not a verification failure.
2. **Label-honesty**, per locked skill: `actual = git rev-parse HEAD:<path>`; compare to the lock's `treeSha`. Differ → `mismatch`. Path absent from `HEAD` but present on disk (uncommitted) → `dirty`; path absent entirely → `missing`.
3. **Dirty check**: `git status --porcelain -- <path>` for each locked path; uncommitted modifications → `dirty` (distinct from `mismatch` — "commit / has local modifications").
4. **Orphan/completeness**: enumerate `.agents/skills/*` dirs that contain `skill.toml`/`SKILL.md`; any with no lock entry → `orphan`. (Multi-client symlink views are not created this slice, so only the canonical location is scanned — spike §6.)
5. **Aggregate** every verdict into a `VerifyReport`; **do not stop at the first failure** (FR-012). `ok` iff all verdicts are `ok`.

## Output

**Human (default, stdout, compact — line count bounded ≤ findings + K, Constitution II):**

Pass (2 lines):
```
verified 3 skills ✓
→ all match their recorded version
```
Fail (one line per failing skill + summary + footer; bounded by # findings):
```
verify FAILED: 2 of 3 skills
  ✗ terraform-plan-review  content mismatch (recorded c967789, on-disk a1b2c3d)
  ✗ secret-scanner         untracked (no lock entry)
→ inspect with: skillrig verify --json
```

**`--json` (stdout, complete + structurally complete — every checked skill):**
```json
{ "ok": false,
  "counts": { "verified": 1, "mismatch": 1, "orphan": 1, "missing": 0, "dirty": 0 },
  "verdicts": [
    { "name": "terraform-plan-review", "path": ".agents/skills/terraform-plan-review",
      "status": "mismatch",
      "expectedTreeSha": "c967789527370d2e0fba03a92e70dffef6f3bf31",
      "actualTreeSha": "a1b2c3d…", "reason": "content does not match recorded version" },
    { "name": "secret-scanner", "path": ".agents/skills/secret-scanner",
      "status": "orphan", "expectedTreeSha": "", "actualTreeSha": "…",
      "reason": "present on disk but not in the lock" },
    { "name": "pr-summary", "path": ".agents/skills/pr-summary", "status": "ok",
      "expectedTreeSha": "…", "actualTreeSha": "…", "reason": "" }
  ] }
```
Keys always present: `ok, counts{verified,mismatch,orphan,missing,dirty}, verdicts[]`; each verdict carries `name, path, status, expectedTreeSha, actualTreeSha, reason`. `status ∈ {ok, mismatch, orphan, missing, dirty}`.

## Exit codes (load-bearing)

| Code | When |
|---|---|
| 0 | All verdicts `ok` (incl. the empty case: no skills, no orphans → clean pass). |
| 1 | Usage/config: malformed/unreadable lock, bad flags, **not inside a git repo**. |
| 2 | Verification failure: any `mismatch`, `orphan`, `missing`, or `dirty`. |
| 3 | **Never emitted** — reserved for `doctor`'s prerequisite class. A missing backing tool MUST NOT fail `verify` (FR-014). |

## Errors (stderr; what/why/fix; raw cause under `--verbose`)

| Condition | Exit | Message shape |
|---|---|---|
| Malformed / unreadable lock | 1 | what: cannot read `.skillrig/skills-lock.json`; why: `<raw parse/io error>`; fix: check the file / re-vendor with `skillrig add`. |
| Not inside a git repo | 1 | what: not a git repository; why: tree-SHA recompute needs git; fix: run inside the repo. |
| (verification failures) | 2 | rendered as the per-skill report above (the report *is* the message). |

## Test mapping (Constitution II)

`TestQuickstart_Verify*`: clean pass; tamper one file → `mismatch` exit 2 (names it); add an unlocked dir → `orphan` exit 2; delete a locked dir → `missing` exit 2; **multiple failures in one run** → all reported (FR-012); empty repo → exit 0; malformed lock → exit 1; not-a-git-repo → exit 1. Output-shape: human line-count bound (≤ findings + K); `--json` `json.Unmarshal` + `counts`/`verdicts` structurally complete; the pass case's `treeSha` equals the fixture's **real** value (ground truth).
