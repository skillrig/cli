# skillrig

`skillrig` is a CLI for pointing a repository (or your per-user default) at an
**origin** — the `OWNER/REPO` that hosts your team's agent skills — and resolving
which origin is active for any working directory. An origin reference is
`OWNER/REPO[@REF]`; the optional `@REF` tracks a specific branch.

> NOTE: a skillrig **origin** is a GitHub repository with a determined structure, use the template repository provided to create your own origin.

## Install

Requires Go 1.24+ and `git` on your `PATH` (used for an offline repo-root lookup).

```sh
go build -o skillrig .
```

## Usage

### Bind a repo to an origin

```sh
# Run from anywhere in the repo; the config lands at the git repository root.
skillrig init --origin my-org/my-skills
```

This writes `.skillrig/config.toml` at the git repository root (or the current
directory if you are not inside a git repo). `init` is **idempotent** and
**consume-only**: it records an existing origin, never creates or scaffolds one,
and binding the same origin twice is a no-op.

```sh
# Track a specific branch of the origin instead of its default branch.
skillrig init --origin my-org/my-skills@staging
```

The optional `@REF` is validated for shape only (offline) — it is not checked
against the remote.

### Set a personal default

```sh
# Used when a repo has no origin of its own.
skillrig init --origin my-org/my-skills --global
```

Writes `$XDG_CONFIG_HOME/skillrig/config.toml` (or `~/.config/skillrig/config.toml`).

### Scripts and agents

```sh
# Never prompt; fail fast if --origin is missing (safe for CI/agents).
skillrig init --origin my-org/my-skills --non-interactive

# Machine-readable result.
skillrig init --origin my-org/my-skills --json
```

## Origin resolution precedence

Every command resolves the active origin with a single rule — highest wins:

```
SKILLRIG_ORIGIN  >  project .skillrig/config.toml (nearest ancestor)  >  global config
```

- `SKILLRIG_ORIGIN` overrides everything without editing any file:
  ```sh
  SKILLRIG_ORIGIN=ci-org/ci-skills skillrig <command>
  ```
  A blank/whitespace value is treated as **unset**.
- The project config is found by walking **up** from the current directory, so any
  subdirectory of a bound repo resolves the same origin.
- A malformed or origin-less config file is skipped; resolution continues down the
  order. When no source supplies an origin, commands report **"no origin configured"** —
  fix it with `skillrig init --origin OWNER/REPO`, by setting `SKILLRIG_ORIGIN`, or with
  a `--global` default.

## Exit codes

| Code | Meaning |
|------|---------|
| `0`  | Success (including an idempotent no-op) |
| `1`  | Usage or configuration error (bad flags, invalid origin, no origin configured) |

Codes `2` (verification) and `3` (prerequisite) are reserved for later commands.

## Configuration file

The v0 `config.toml` holds a single key:

```toml
origin = 'my-org/my-skills'
```

To track a branch, the optional `@REF` is stored combined in the same key:

```toml
origin = 'my-org/my-skills@staging'
```

Unknown keys are ignored on read, so config added by later versions will not break this
one. The full, extended `config.toml` structure is documented on the project docs
website; see also [docs/design/cli.md](docs/design/cli.md) for the CLI design contract.
