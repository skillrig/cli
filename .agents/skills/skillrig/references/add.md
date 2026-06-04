# `skillrig add <skill>` — vendor a skill (Vendor Mutation)

> Bring a skill from your **configured origin** into the repo and record its identity.
> Needs an origin (run [init](init.md) first). After `add`, commit, then [verify](verify.md).

Vendors `<skill>` into the canonical `.agents/skills/<skill>/`, byte-identical and
mode-preserving (it injects nothing), and records its identity — `version`, `commit`,
`treeSha`, `path` — in `.skillrig/skills-lock.json`. Consume-only (the fetch token is
read-only; there is no write/publish path). **Requires a git repository** (project scope).
The recorded `treeSha` is the git tree-SHA `verify` later recomputes, so the two cannot
drift (the gate cannot lie) — never hand-edit the lock. Path-traversal + symlink guards
apply to remotely-fetched content too.

- **Origin, not a path**: `add` resolves the active origin via the shared resolver
  (`SKILLRIG_ORIGIN` > project `.skillrig/config.toml` > global). There is **no**
  `--from`/path argument.
- **Two origin forms, classified automatically** (you do not choose a mode):
  - **Remote `OWNER/REPO`** (the common case) — `add` **fetches** the skill subtree directly
    from `github.com/OWNER/REPO@ref` over `git` (sparse checkout). Nothing needs to be checked
    out locally. A private origin needs a **read-only** token, resolved automatically via
    `GH_TOKEN` > `GITHUB_TOKEN` > `gh auth token` (so `gh auth login` once is enough). The fetch
    runs `git` **non-interactively**: with no usable token it fails fast as an `AuthError` —
    it never prompts for a username or hangs a no-TTY CI job.
  - **Local filesystem path** — if the configured origin is a real path, `add` reads that
    local checkout (no network). This is the generalized 002 behavior.
- **`--pin <ref>` — vendor a specific immutable version** (remote path). A bare semver
  (`v1.4.0` / `1.4.0`) expands via the origin's `tag_scheme` to the per-skill tag
  (`terraform-plan-review-v1.4.0`); anything else is a literal git ref/SHA. The lock records
  the resolved `commit` + `treeSha` + the human-readable `version`/tag, so re-adding the same
  pin reproduces byte-identical content. Omit `--pin` to vendor the origin branch's tip.
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
| `--pin <ref>` | Vendor an immutable version: bare semver → origin `tag_scheme` tag; else literal git ref/SHA |
| `--dry-run` | Report what would be vendored/recorded; write nothing |
| `--force` | Overwrite a vendored skill whose on-disk content diverges from the lock |
| `--json` | Emit the complete `AddResult` on stdout |
| `--verbose` | Show underlying paths / raw git cause behind summaries and errors |

`--json` keys (always present): `ok, name, version, path, commit, treeSha, action, dryRun`;
`action ∈ {vendored, unchanged, overwritten}`.

## Workflow patterns

1. **Vendor + lock**: `skillrig add terraform-plan-review` → `git add -A && git commit` →
   `skillrig verify`.
2. **Pin a version**: `skillrig add terraform-plan-review --pin v1.4.0` vendors that exact
   reviewed version (reproducible via the recorded commit + treeSha + tag).
3. **Recover a tampered skill** (a `mismatch`/`dirty` verdict from verify): re-vendor with
   `skillrig add <skill> --force`, then commit and re-verify.
4. **Adopt an `orphan`**: `skillrig add <skill>` records an on-disk-but-unlocked skill.
5. **Preview**: `skillrig add <skill> --dry-run`.

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `no origin configured` | no `SKILLRIG_ORIGIN` / project / global origin | `skillrig init --origin OWNER/REPO`, or set `SKILLRIG_ORIGIN` |
| `authentication ... failed` (**AuthError**) | private origin, no/invalid token (incl. CI with no credential — fails fast, never prompts) | `gh auth login`, or export a `GH_TOKEN`/`GITHUB_TOKEN` with read access. **Not** a typo'd repo — the name resolved fine |
| `... is unreachable` (**UnreachableError**) | network failure / wrong host | check connectivity/proxy and the host in the origin reference; retry |
| `origin "<OWNER/REPO>" not found` (**NotFoundError**) | origin repo missing **or** a private repo with no token (GitHub reports both as "not found") | check the spelling; **if private, authenticate** (`gh auth login` / `GITHUB_TOKEN`) — that's the common cause |
| `no version "<ref>" of "<skill>"` (**NoSuchVersionError**) | a `--pin` that resolves to no tag/ref | list the published versions, or drop `--pin` to take the tip. Distinct from "skill not found" |
| `origin ... uses convention version N` (**IncompatibleConvention**) | origin speaks a layout this binary doesn't support | update `skillrig`, or point at a compatible origin |
| `skill "<name>" not found in origin` | the origin IS reachable but has no `skills/<name>/` | check the name against the origin's catalog (`skillrig search`) |
| `refusing to overwrite <path>` | on-disk content diverges from the record | re-run with `--force`, or revert local edits |
| `not a git repository` | run outside a repo | run inside the repo (or `git init` first) |
| bad/missing args | wrong invocation | the error states what/why/fix + an example; or `skillrig add --help` |

These failure classes are **distinct on purpose** — auth vs unreachable vs not-found vs
no-such-version each point at a different fix; don't treat them as one. One nuance: a
**private origin with no credential** may surface as **either** `AuthError` (git aborted at
the credential step — the fetch is non-interactive, so it never prompts/hangs) **or**
`NotFoundError` with the authenticate hint (GitHub answers a private repo as "not found");
both share the same fix — authenticate. All failures state what/why/fix and exit `1`; add
`--verbose` for the raw cause. Errors go to stderr, data to stdout.
