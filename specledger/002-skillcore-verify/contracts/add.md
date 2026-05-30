# Contract: `skillrig add`

**Pattern**: Vendor Mutation — [cli.md](../../../docs/design/cli.md) Pattern Classification. Writes the skill tree + lock entry via `skillcore` only; supports `--dry-run`; refuses to clobber divergent content without `--force`.
**Purpose**: Vendor a named skill from the repo's **configured origin** (a local checkout this slice) into the canonical `.agents/skills/<skill>/`, recording its identity in `.skillrig/skills-lock.json`. Offline. Requires a git repository.

## Synopsis

```
skillrig add <skill> [--dry-run] [--force] [--json] [--verbose]
```

## Flags & Args

| | Type | Default | Meaning |
|---|---|---|---|
| `<skill>` | arg (`cobra.ExactArgs(1)`) | — | Skill name to vendor (its directory within the origin's `skills/`). |
| `--dry-run` | bool | false | Report what *would* be placed/recorded; write nothing. |
| `--force` | bool | false | Overwrite a vendored skill whose on-disk content diverges from the recorded fingerprint (otherwise refused). |
| `--json` | bool | false | Emit the complete `AddResult` on stdout instead of compact human text. |
| `--verbose` | bool | false | Print underlying paths / raw git cause behind summaries and errors. |

> **Origin, not a path** (clarified 2026-05-30): there is **no** `--from`/path argument. `add` resolves the active origin through the shared resolver (`SKILLRIG_ORIGIN` > project `.skillrig/config.toml` > global) exactly like every command; the origin *value* may be a local checkout this slice. Tests do `skillrig init --origin <local-origin>` then `skillrig add <skill>`.

## Help (Progressive Discovery)

```
Examples:
  # Vendor a skill from your configured origin into .agents/skills/
  skillrig add terraform-plan-review

  # Preview what would be vendored, writing nothing
  skillrig add terraform-plan-review --dry-run
```

## Behavior

1. **Resolve origin** (CLI layer, via `config.ResolveOrigin`). No origin in any source → usage error (exit 1, same shape as the resolver's "no origin configured"). The resolved origin (a local path this slice) + ref is handed to `skillcore.Add`.
2. **Locate the skill** in the origin at `skills/<skill>/`; absent → usage error (exit 1). Read `skill.toml` for `name`/`version` (`skillcore.ParseManifest`).
3. **Fingerprint + provenance** from the origin (git-canonical, research D1): `treeSha = git -C <origin> rev-parse <ref>:skills/<skill>`; `commit = git -C <origin> rev-parse <ref>`.
4. **Placement guard**: if `.agents/skills/<skill>` already exists and its content diverges from the lock's `treeSha`, **refuse** without `--force` (exit 1, "use --force"); never silently clobber (FR-004). No three-way merge (that is `bump`).
5. **Vendor** (unless `--dry-run`): copy the subtree into `.agents/skills/<skill>/` **byte-identical, preserving file modes** (the exec bit is part of the tree SHA); inject nothing. Idempotent if identical (`action=unchanged`).
6. **Write the lock** entry `{ version, commit, treeSha, path }` under `skills.<name>` (atomic temp+rename; `requires` is **not** recorded — research D4). `--dry-run` writes nothing.
7. Emit result (see Output). The user then commits `.agents/skills/<skill>` + the lock (vendored-in-git); `verify` checks the committed tree.

## Output

**Human (default, stdout, compact — ≤2 lines incl. footer):**
```
vendored terraform-plan-review@1.4.0 → .agents/skills/terraform-plan-review (treeSha c967789)
→ commit it, then run: skillrig verify
```
(idempotent re-add prints `terraform-plan-review@1.4.0 already vendored (no change)`; `--dry-run` prefixes `would vendor …`.)

**`--json` (stdout, complete + parseable):**
```json
{ "ok": true, "name": "terraform-plan-review", "version": "1.4.0",
  "path": ".agents/skills/terraform-plan-review",
  "commit": "9f1a052e596d5d28f13838061a1ab93207ef6fc3",
  "treeSha": "c967789527370d2e0fba03a92e70dffef6f3bf31",
  "action": "vendored", "dryRun": false }
```
Keys always present: `ok, name, version, path, commit, treeSha, action, dryRun`. `action ∈ {vendored, unchanged, overwritten}`.

## Errors (stderr; prose what/why/fix; raw cause preserved)

| Condition | Exit | Message shape |
|---|---|---|
| No origin configured | 1 | what: no origin configured; why: no `SKILLRIG_ORIGIN` / project / global origin; fix: `skillrig init --origin OWNER/REPO` or set `SKILLRIG_ORIGIN`. |
| Skill not found in origin | 1 | what: skill `<name>` not found in origin; why: no `skills/<name>/` at `<origin>@<ref>`; fix: check the name / `skillrig search` (future). |
| Divergent content, no `--force` | 1 | what: refusing to overwrite `<path>`; why: on-disk content diverges from the recorded fingerprint; fix: re-run with `--force`, or revert local edits. |
| Not inside a git repo (project scope) | 1 | what: not a git repository; why: project-scope `add` places `.agents/skills` at the repo root and writes a lock that `verify` checks against git; fix: run inside the repo (or `git init`). _(A future `--global` path is exempt — see spec Out of Scope.)_ |

Exit `0` on success (incl. idempotent no-op and `--dry-run`). Code `2` is `verify`'s; `3` is reserved (`doctor`).

## Test mapping (Constitution II)

Each Output/Errors/Behavior row maps to a `TestQuickstart_Add*` scenario. Output-shape: human line-count bound; `--json` `json.Unmarshal` + all-keys-present + the `treeSha`/`commit` are the **fixture's real** values (ground truth, data-model.md); error asserts what/why/fix as distinct checks + exit code.
