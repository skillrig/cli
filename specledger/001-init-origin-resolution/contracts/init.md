# Contract: `skillrig init`

**Pattern**: Environment (idempotent, consume-only) — [cli.md](../../../docs/design/cli.md) Pattern Classification.
**Purpose**: Bind this repo (or the per-user global default) to an **existing** origin by recording it in config. Does NOT bootstrap/scaffold an origin (architecture §2d).

## Synopsis

```
skillrig init [--origin OWNER/REPO] [--global] [--json] [--verbose]
```

## Flags

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--origin` | string | "" | Origin to bind, `OWNER/REPO`. If omitted: prompt in an interactive TTY; error in non-interactive. |
| `--global` | bool | false | Write the per-user global default (`$XDG_CONFIG_HOME/skillrig/config.toml` or `~/.config/skillrig/config.toml`) instead of `./.skillrig/config.toml`. |
| `--json` | bool | false | Emit the complete result object on stdout instead of compact human text. |
| `--verbose` | bool | false | Print the underlying file path(s) / raw cause behind any summary or error. |

`Args`: none (`cobra.NoArgs`); origin is a flag.

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

1. Resolve write target: `--global` → global config path; else `./.skillrig/config.toml` (create `.skillrig/` if missing — FR-010).
2. Determine origin: `--origin` value, else if interactive TTY prompt once on stderr, else usage error (FR-006a).
3. `ParseOrigin` → on invalid shape, usage error, no write (FR-012).
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
| `--origin` omitted, non-interactive | 1 | what: no origin given; why: non-interactive session; fix: pass `--origin OWNER/REPO` or set `SKILLRIG_ORIGIN`. |
| Malformed origin | 1 | what: invalid origin `<value>`; why: expected `OWNER/REPO`; fix: e.g. `skillrig init --origin my-org/my-skills`. |
| Config dir/file not writable | 1 | what: cannot write `<path>`; why: `<raw os error>`; fix: check permissions / path. |

Exit `0` on success (including idempotent no-op). Codes `2`/`3` not used by this command.

## Test mapping (Constitution II)

Each row of the Output/Errors/Behavior tables has a `TestQuickstart_*` scenario in `quickstart.md`. Output-shape assertions: human line-count bound; `--json` `json.Unmarshal` + all-keys-present; error asserts the three parts as distinct checks + exit code.
