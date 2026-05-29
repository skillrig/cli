# Contract: `skillrig init`

**Pattern**: Environment (idempotent, consume-only) — [cli.md](../../../docs/design/cli.md) Pattern Classification.
**Purpose**: Bind this repo (or the per-user global default) to an **existing** origin by recording it in config. Does NOT bootstrap/scaffold an origin (architecture §2d).

## Synopsis

```
skillrig init [--origin OWNER/REPO[@REF]] [--global] [--non-interactive] [--json] [--verbose]
```

## Flags

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--origin` | string | "" | Origin to bind, `OWNER/REPO[@REF]`. The optional `@REF` tracks a branch (amendment [001-origin-ref-support](../amendments/001-origin-ref-support.md)). If omitted: prompt in an interactive TTY; error in non-interactive. |
| `--global` | bool | false | Write the per-user global default (`$XDG_CONFIG_HOME/skillrig/config.toml` or `~/.config/skillrig/config.toml`) instead of the repo project config. |
| `--non-interactive` | bool | false | Force non-interactive mode: never prompt. If required flags such `--origin` are omitted, fail (exit 1) instead of prompting, **even on an interactive TTY** (FR-006c). For scripts/agents that must not block on input. |
| `--json` | bool | false | Emit the complete result object on stdout instead of compact human text. |
| `--verbose` | bool | false | Print the underlying file path(s) / raw cause behind any summary or error. |

`Args`: none (`cobra.NoArgs`); origin is a flag.

> **Project write target (FR-005/FR-010):** without `--global`, the project config is written at the **git repository root** — located via `git rev-parse --show-toplevel`, a fully **offline** call — as `<repo-root>/.skillrig/config.toml`, so a repo has a single canonical config regardless of the cwd subdirectory. When the cwd is **not** inside a git repository, it falls back to `./.skillrig/config.toml` in the cwd. `git` is a required dependency of the framework (see plan.md → Technical Context). The resolver (`contracts/resolve.md`) finds this file from any subdirectory via walk-up, keeping write and read symmetric.

## Help (Progressive Discovery — cli.md Principle 1, Rule 1)

`Long` description + **≥2 examples**:

```
Examples:
  # Bind the current repo to an existing origin
  skillrig init --origin my-org/my-skills

  # Set your personal default origin (used when a repo has none)
  skillrig init --origin my-org/my-skills --global
```

## Behavior

1. Resolve write target: `--global` → global config path; else the **git repo root** via `git rev-parse --show-toplevel` (offline) → `<repo-root>/.skillrig/config.toml`; if not inside a git repo, fall back to `./.skillrig/config.toml` in cwd (create `.skillrig/` if missing — FR-010).
2. Determine origin: `--origin` value; else if `--non-interactive` is set → usage error without prompting (FR-006c); else if interactive TTY → prompt once on stderr; else (no TTY) usage error (FR-006a).
3. `ParseOrigin` → splits the optional `@REF` and validates both parts shape-only/offline; on invalid shape it returns a typed `*config.InvalidOriginError` (presentation-free) which the CLI renders as the user-facing what/why/fix, no write (FR-012 / FR-018, FR-019). A valid `@REF` is carried on the resolved origin and stored combined in the `origin` key (FR-020).
4. Load existing config at target (if any). Compare:
   - none present → write, `written=true`.
   - equal origin → no-op, `written=false` (idempotent, FR-008).
   - different origin → rewrite with new origin, `written=true` (FR-009).
5. Write atomically (temp + rename, research D9).
6. Emit result (see Output).

Only the origin is collected; no other metadata (FR-006b).

## Output

**Human (default, stdout, compact — ≤2 lines incl. footer hint):**
```
bound origin my-org/my-skills (project: ./.skillrig/config.toml)
→ resolve order: SKILLRIG_ORIGIN > ./.skillrig/config.toml > ~/.config/skillrig/config.toml
```
(idempotent no-op prints `already bound to my-org/my-skills (no change)`.)

**`--json` (stdout, complete + parseable):**
```json
{ "ok": true, "origin": "my-org/my-skills", "scope": "project", "configPath": "/abs/.skillrig/config.toml", "written": true }
```
Keys always present: `ok, origin, scope, configPath, written`. `scope ∈ {project, global}`.

## Errors (stderr; prose what/why/fix; raw cause preserved — cli.md Principle 2)

| Condition | Exit | Message shape |
|-----------|------|---------------|
| `--origin` omitted, non-interactive **session** (no TTY) | 1 | what: no origin given; why: non-interactive session (no TTY); fix: pass `--origin OWNER/REPO[@REF]` or set `SKILLRIG_ORIGIN`. |
| `--origin` omitted, `--non-interactive` **forced** (even on a TTY) | 1 | what: no origin given; why: non-interactive mode requested (`--non-interactive`); fix: pass `--origin OWNER/REPO[@REF]` or set `SKILLRIG_ORIGIN`. |
| Malformed origin (incl. malformed `@REF`) | 1 | what: invalid origin `<value>`; why: expected `OWNER/REPO[@REF]`; fix: e.g. `skillrig init --origin my-org/my-skills` or `--origin my-org/my-skills@main`. |
| Config dir/file not writable | 1 | what: cannot write `<path>`; why: `<raw os error>`; fix: check permissions / path. |

Exit `0` on success (including idempotent no-op). Codes `2`/`3` not used by this command.

## Test mapping (Constitution II)

Each row of the Output/Errors/Behavior tables has a `TestQuickstart_*` scenario in `quickstart.md`. Output-shape assertions: human line-count bound; `--json` `json.Unmarshal` + all-keys-present; error asserts the three parts as distinct checks + exit code.
