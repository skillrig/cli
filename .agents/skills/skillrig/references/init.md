# `skillrig init` — bind the repo to an origin

> Choose **where** skills come from. Run this first; `add` needs a configured origin.

## Overview

`skillrig init` is an **Environment-pattern** command: it records an *existing* origin
(the `OWNER/REPO` that hosts your team's agent skills) into config so every later
`skillrig` command knows where skills come from. It is **idempotent** and
**consume-only** — it never creates or scaffolds an origin, never reaches the network,
and binding the same origin twice is a no-op.

The origin reference is `OWNER/REPO[@REF]`. The optional `@REF` tracks a specific
**branch** of the library (e.g. `my-org/my-skills@staging`); omit it to track the
default branch. The `@REF` is validated for shape only (offline) — it is **not** checked
against the remote — and is stored combined in the single `origin` key. (An origin's
`@ref` is a moving branch pointer; pinning an individual *skill* to an immutable tag/SHA
is a separate, later concern.)

It writes one of two config files:

- **Project** (default): `.skillrig/config.toml` at the **git repository root** (located
  via an offline `git rev-parse --show-toplevel`), so a repo has one canonical config
  regardless of which subdirectory you run from. Outside a git repo it falls back to
  `./.skillrig/config.toml` in the current directory.
- **Global** (`--global`): the per-user default at `$XDG_CONFIG_HOME/skillrig/config.toml`
  or `~/.config/skillrig/config.toml`, used when a repo has no origin of its own.

`git` must be on `PATH` (used only for the offline repo-root lookup).

## Command surface

| Flag | Purpose | When to use |
|------|---------|-------------|
| `--origin OWNER/REPO[@REF]` | The origin to bind; optional `@REF` tracks a branch | Always prefer passing it explicitly (scripts/agents); add `@branch` to track a non-default branch |
| `--global` | Write the per-user default instead of the repo config | Setting a fallback used across all your repos |
| `--non-interactive` | Never prompt; fail fast if `--origin` is missing | CI/agents that must not block on input |
| `--json` | Emit a complete result object on stdout | Machine consumption |
| `--verbose` | Show underlying paths / raw cause behind summaries and errors | Debugging a failure |

## Decision criteria

- **Project vs global**: bind the repo (no `--global`) so the repo is self-describing
  and teammates resolve the same origin. Use `--global` only for a personal fallback.
- **`--origin` vs prompt**: always pass `--origin` in scripts/agents. The interactive
  prompt appears only on a real terminal when `--origin` is omitted.
- **`--non-interactive`**: set it in any automated context. It forces fail-fast even on
  a TTY, so the command never hangs waiting for input.
- **`SKILLRIG_ORIGIN`**: prefer this env var for one-off overrides (e.g. CI) — it beats
  both config files without editing anything on disk.

## Resolution precedence

Every command resolves the active origin with one rule (highest wins):

```
SKILLRIG_ORIGIN  >  project .skillrig/config.toml (nearest ancestor)  >  global config
```

- A blank/whitespace `SKILLRIG_ORIGIN` is treated as **unset** (falls through).
- A malformed or origin-less config file is **skipped**, and resolution continues down
  the order — it is not a hard failure.
- When no source supplies an origin, that is the "no origin configured" state the user
  must fix (see Error handling).

The project lookup walks **up** from the working directory, so any subdirectory of a
bound repo resolves the same origin.

## Local origin (this release)

`init` records only an `OWNER/REPO[@REF]` **reference** — never a filesystem path (passing
a path fails with `invalid origin … expected OWNER/REPO[@REF]`). In this release there is no
network fetch, so when `skillrig add` needs the origin's files it reads them from a **local
git checkout at `<repo-root>/OWNER/REPO`** (resolved against the repo root, so it works from
any subdirectory). To vendor from a local copy of `my-org/my-skills`, from the repo root:

```
skillrig init --origin my-org/my-skills        # records the reference
git clone <library> my-org/my-skills           # the checkout add reads from
echo 'my-org/' >> .git/info/exclude            # keep it out of your repo's index
skillrig add <skill>                           # reads <repo-root>/my-org/my-skills/skills/<skill>/
```

`@REF` selects the revision (default `HEAD`). Fetching a remote origin over the network is a
later, additive mode. See [add.md](add.md) for the vendoring side.

## JSON output

`skillrig init --origin my-org/my-skills --json` emits a single object with all keys
present; branch on `written` to tell a fresh bind from an idempotent no-op:

```json
{ "ok": true, "origin": "my-org/my-skills", "scope": "project", "configPath": "/abs/.skillrig/config.toml", "written": true }
```

`scope` is `project` or `global`; `written` is `false` when the origin was already bound.

## Workflow patterns

1. **Bind a repo**: `skillrig init --origin my-org/my-skills` → run from anywhere in the
   repo; config lands at the repo root.
2. **Track a branch**: `skillrig init --origin my-org/my-skills@staging`.
3. **Personal default**: `skillrig init --origin my-org/my-skills --global`.
4. **CI / agent**: pass `--origin` (or set `SKILLRIG_ORIGIN`) **and** `--non-interactive`.
5. **One-off override**: `SKILLRIG_ORIGIN=ci-org/ci-skills skillrig <cmd>` — no file edit.

## Error handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `invalid origin "<value>": expected OWNER/REPO[@REF]` | Origin (or `@REF`) not in `OWNER/REPO[@REF]` shape | Pass `--origin my-org/my-skills` (or `…@main`) |
| `no origin given … non-interactive session (no TTY)` | `init` run without `--origin`, stdin not a terminal | Pass `--origin OWNER/REPO` or set `SKILLRIG_ORIGIN` |
| `no origin given … non-interactive mode requested (--non-interactive)` | `--non-interactive` set but no `--origin` | Pass `--origin OWNER/REPO` or set `SKILLRIG_ORIGIN` |
| `no origin configured` (from a later command) | No source supplied an origin | `skillrig init --origin OWNER/REPO`, set `SKILLRIG_ORIGIN`, or add a `--global` default |

All failures exit `1` (usage/config); add `--verbose` for the raw cause. Default human
output is ≤2 lines (a confirmation + a one-line resolve-order hint); use `--json` only when
a program will parse it.
