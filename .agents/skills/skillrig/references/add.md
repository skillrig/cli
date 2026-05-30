# `skillrig add <skill>` — vendor a skill (Vendor Mutation)

> Bring a skill from your **configured origin** into the repo and record its identity.
> Needs an origin (run [init](init.md) first). After `add`, commit, then [verify](verify.md).

Vendors `<skill>` into the canonical `.agents/skills/<skill>/`, byte-identical and
mode-preserving (it injects nothing), and records its identity — `version`, `commit`,
`treeSha`, `path` — in `.skillrig/skills-lock.json`. Offline and consume-only.
**Requires a git repository** (project scope). The recorded `treeSha` is the git tree-SHA
`verify` later recomputes, so the two cannot drift (the gate cannot lie) — never hand-edit
the lock.

- **Origin, not a path**: `add` resolves the active origin via the shared resolver
  (`SKILLRIG_ORIGIN` > project `.skillrig/config.toml` > global). There is **no**
  `--from`/path argument.
- **Local origin (this release)**: the configured `OWNER/REPO` is read from a local git
  checkout at `<repo-root>/OWNER/REPO` (resolved against the repo root, so `add` works from
  **any subdirectory**) — no network. So `init --origin my-org/my-skills` expects that
  library checked out at `<repo-root>/my-org/my-skills` (keep it out of your index, e.g.
  `echo 'my-org/' >> .git/info/exclude`). If that checkout is absent, `add` says **"origin
  checkout not found"** (distinct from "skill not found").
- **Idempotent**: re-adding identical content reports success and changes nothing
  (`action: "unchanged"`).
- **Never clobbers**: if the on-disk copy diverges from the recorded fingerprint, `add`
  **refuses** without `--force` (so local edits are never lost silently). It does **not**
  three-way-merge — re-vendoring the same version has no upstream change to merge (that is a
  future `bump`). Use `--force` to overwrite with the origin's content, or revert your edits.
- **`--dry-run`** previews placement + record changes and writes nothing.

After `add`, **commit** `.agents/skills/<skill>` + the lock, then run `verify` — it checks
the *committed* tree.

| Flag | Purpose |
|------|---------|
| `--dry-run` | Report what would be vendored/recorded; write nothing |
| `--force` | Overwrite a vendored skill whose on-disk content diverges from the lock |
| `--json` | Emit the complete `AddResult` on stdout |
| `--verbose` | Show underlying paths / raw git cause behind summaries and errors |

`--json` keys (always present): `ok, name, version, path, commit, treeSha, action, dryRun`;
`action ∈ {vendored, unchanged, overwritten}`.

## Workflow patterns

1. **Vendor + lock**: `skillrig add terraform-plan-review` → `git add -A && git commit` →
   `skillrig verify`.
2. **Recover a tampered skill** (a `mismatch`/`dirty` verdict from verify): re-vendor with
   `skillrig add <skill> --force`, then commit and re-verify.
3. **Adopt an `orphan`**: `skillrig add <skill>` records an on-disk-but-unlocked skill.
4. **Preview**: `skillrig add <skill> --dry-run`.

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `no origin configured` | no `SKILLRIG_ORIGIN` / project / global origin | `skillrig init --origin OWNER/REPO`, or set `SKILLRIG_ORIGIN` |
| `origin checkout not found at <path>` | the configured `OWNER/REPO` is not checked out locally at `<repo-root>/OWNER/REPO` | clone the origin there (`git clone <url> <path>`), or re-bind with `skillrig init` |
| `skill "<name>" not found in origin` | the origin IS present but has no `skills/<name>/` | check the name against the origin's `skills/` |
| `refusing to overwrite <path>` | on-disk content diverges from the record | re-run with `--force`, or revert local edits |
| `not a git repository` | run outside a repo | run inside the repo (or `git init` first) |
| bad/missing args | wrong invocation | the error states what/why/fix + an example; or `skillrig add --help` |

All failures state what/why/fix and exit `1`; add `--verbose` for the raw cause. Errors go to
stderr, data to stdout.
