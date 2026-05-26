---
name: skillrig-init
description: >-
  Bind a repository (or your per-user default) to a skill origin with `skillrig init`,
  and understand how skillrig resolves the active origin. Use when the user wants to
  "point this repo at our skills library", "set the origin", configure where skills come
  from, set up `skillrig` in a repo, choose between a project vs global default origin,
  use the `SKILLRIG_ORIGIN` environment variable, or fix a "no origin configured" /
  "no origin given" error. Triggers on `skillrig init`, origin binding (OWNER/REPO),
  and origin-resolution precedence questions.
license: MIT
metadata:
  author: skillrig
  cli: skillrig
  user-invocable: true
---

# skillrig-init Skill

**When to Load**: The user wants to point a repository at an existing skills origin
(`OWNER/REPO`), set a personal default, configure `SKILLRIG_ORIGIN`, or resolve a
"no origin configured" failure — or whenever `skillrig init` is referenced.

## Overview

`skillrig init` is an **Environment-pattern** command: it records an *existing* origin
(the `OWNER/REPO` that hosts your team's agent skills) into config so every later
`skillrig` command knows where skills come from. It is **idempotent** and
**consume-only** — it never creates or scaffolds an origin, never reaches the network,
and binding the same origin twice is a no-op.

It writes one of two config files:

- **Project** (default): `.skillrig/config.toml` at the **git repository root** (located
  via an offline `git rev-parse --show-toplevel`), so a repo has one canonical config
  regardless of which subdirectory you run from. Outside a git repo it falls back to
  `./.skillrig/config.toml` in the current directory.
- **Global** (`--global`): the per-user default at `$XDG_CONFIG_HOME/skillrig/config.toml`
  or `~/.config/skillrig/config.toml`, used when a repo has no origin of its own.

`git` must be on `PATH` (used only for the offline repo-root lookup).

## Command Surface

| Flag | Purpose | When to use |
|------|---------|-------------|
| `--origin OWNER/REPO` | The origin to bind | Always prefer passing it explicitly (scripts/agents) |
| `--global` | Write the per-user default instead of the repo config | Setting a fallback used across all your repos |
| `--non-interactive` | Never prompt; fail fast if `--origin` is missing | CI/agents that must not block on input |
| `--json` | Emit a complete result object on stdout | Machine consumption |
| `--verbose` | Show underlying paths / raw cause behind summaries and errors | Debugging a failure |

## Decision Criteria

- **Project vs global**: bind the repo (no `--global`) so the repo is self-describing
  and teammates resolve the same origin. Use `--global` only for a personal fallback.
- **`--origin` vs prompt**: always pass `--origin` in scripts/agents. The interactive
  prompt appears only on a real terminal when `--origin` is omitted.
- **`--non-interactive`**: set it in any automated context. It forces fail-fast even on
  a TTY, so the command never hangs waiting for input.
- **`SKILLRIG_ORIGIN`**: prefer this env var for one-off overrides (e.g. CI) — it beats
  both config files without editing anything on disk.

## Resolution Precedence

Every command resolves the active origin with one rule (highest wins):

```
SKILLRIG_ORIGIN  >  project .skillrig/config.toml (nearest ancestor)  >  global config
```

- A blank/whitespace `SKILLRIG_ORIGIN` is treated as **unset** (falls through).
- A malformed or origin-less config file is **skipped**, and resolution continues down
  the order — it is not a hard failure.
- When no source supplies an origin, that is the "no origin configured" state the user
  must fix (see Error Handling).

The project lookup walks **up** from the working directory, so any subdirectory of a
bound repo resolves the same origin.

## JSON Output

`skillrig init --origin my-org/my-skills --json` emits a single object with all keys
present; branch on `written` to tell a fresh bind from an idempotent no-op:

```json
{ "ok": true, "origin": "my-org/my-skills", "scope": "project", "configPath": "/abs/.skillrig/config.toml", "written": true }
```

`scope` is `project` or `global`; `written` is `false` when the origin was already bound.

## Workflow Patterns

1. **Bind a repo**: `skillrig init --origin my-org/my-skills` → run from anywhere in the
   repo; config lands at the repo root.
2. **Personal default**: `skillrig init --origin my-org/my-skills --global`.
3. **CI / agent**: pass `--origin` (or set `SKILLRIG_ORIGIN`) **and** `--non-interactive`
   so the command never prompts.
4. **One-off override**: `SKILLRIG_ORIGIN=ci-org/ci-skills skillrig <cmd>` — no file edit.

## Error Handling

| Symptom (stderr) | Cause | Fix |
|------------------|-------|-----|
| `invalid origin "<value>": expected OWNER/REPO` | Origin not in `OWNER/REPO` shape | Pass a valid `--origin my-org/my-skills` |
| `no origin given … non-interactive session (no TTY)` | `init` run without `--origin` and stdin is not a terminal | Pass `--origin OWNER/REPO` or set `SKILLRIG_ORIGIN` |
| `no origin given … non-interactive mode requested (--non-interactive)` | `--non-interactive` set but no `--origin` | Pass `--origin OWNER/REPO` or set `SKILLRIG_ORIGIN` |
| "no origin configured" from a later command | No source supplied an origin | Run `skillrig init --origin OWNER/REPO`, or set `SKILLRIG_ORIGIN`, or add a `--global` default |

All failures exit non-zero (usage/config errors exit `1`); add `--verbose` to see the
raw cause behind the message.

## Token Efficiency

Default human output is ≤2 lines (a confirmation plus a one-line resolve-order hint).
Use `--json` only when a program will parse the result; otherwise the compact human form
keeps context small.
