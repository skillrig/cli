# skillrig

`skillrig` is a single, generic, **consume-only** CLI for pointing a repository
(or your per-user default) at an **origin** — the `OWNER/REPO` that hosts your
team's agent skills — and managing the skills you vendor from it. The same binary
serves humans, agents, and CI. There is no `publish` and no write credential in
the binary: GitHub is the authority plane ("publishing" a skill is a PR to the
origin).

An origin reference is `OWNER/REPO[@REF]`; the optional `@REF` tracks a specific
branch.

> NOTE: a skillrig **origin** is a GitHub repository with a determined structure;
> use the template repository provided to create your own origin.

## Install

Requires Go 1.24+ and `git` on your `PATH` (used for offline repo-root lookups,
tree-SHA recomputation, and fetching skills over git).

```sh
go build -o skillrig .
```

## Authentication

skillrig fetches origins over **HTTPS** with a **read-only** token (there is no
write credential in the binary). It resolves one automatically, first match wins:

```
GH_TOKEN  >  GITHUB_TOKEN  >  gh auth token
```

**The supported, tested path is a completed `gh auth login`.** Do that once and
private origins just work — no env var to manage:

```sh
gh auth login          # one-time; gh auth token then supplies the read-only credential
```

- **Public origins need no credential** at all.
- **CI / headless:** set `GH_TOKEN` (or `GITHUB_TOKEN`) to a read-only token — `gh`
  need not be installed.
- **No interactive prompts, ever.** If an origin is private and no token is
  available, skillrig fails fast with an `authentication failed` error (pointing at
  `gh auth login` / `GH_TOKEN`) instead of hanging on a username prompt — safe for
  no-TTY CI. The token is injected via git's `http.extraHeader` through the
  environment, so it never appears in the clone URL or a process listing.
- **SSH-key origins are not supported yet** — every origin is `OWNER/REPO` fetched
  over HTTPS. SSH is a roadmap item ([`docs/ROADMAP.md`](docs/ROADMAP.md)); until
  then, HTTPS-token (via `gh auth`) is the only tested transport.

## The workflow

Most consumers follow one path — bind an origin once, then discover, vendor, and
verify skills:

```sh
skillrig init --origin my-org/my-skills      # 1. bind this repo to an origin
skillrig search terraform                     # 2. discover what the origin offers
skillrig add terraform-plan-review            # 3. vendor a skill into .agents/skills/
skillrig verify                               # 4. prove vendored skills are unchanged
```

Origin **maintainers** also run `skillrig index` inside the origin repo to
regenerate the catalog that `search` reads (see [`index`](#index)).

Every command resolves the active origin the same way (see
[Origin resolution precedence](#origin-resolution-precedence)), accepts `--json`
for a complete machine-readable result, and accepts `--verbose` to surface the
raw cause behind any summary or error.

## Commands

### `init`

Bind a repository (or your global default) to an origin.

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

```sh
# Set a personal default, used when a repo has no origin of its own.
skillrig init --origin my-org/my-skills --global

# Never prompt; fail fast if --origin is missing (safe for CI/agents).
skillrig init --origin my-org/my-skills --non-interactive
```

`--global` writes `$XDG_CONFIG_HOME/skillrig/config.toml` (or
`~/.config/skillrig/config.toml`).

### `search`

Discover the skills your configured origin publishes by reading its catalog
(`index.json`). Read-only; needs a resolvable origin but no git working tree. A
free-text `QUERY` is a case-insensitive token-AND substring over
name + description + topics (a skill matches only if **every** term is present),
and `--topic` adds an exact-string AND filter. An empty result is success
(exit 0).

```sh
# List every skill the origin publishes.
skillrig search

# Free-text query (token-AND over name + description + topics).
skillrig search terraform plan

# Filter by topic (repeatable; AND across topics).
skillrig search --topic aws --topic terraform
```

### `show`

Print **one** skill's full record from the configured origin — its complete,
**untruncated** description (the part `search` clips to a one-line preview), plus
its version, namespace, topics, path, and backing-tool requirements. `show` is the
human counterpart to `search`: where `search` lists many skills compactly, `show`
drills into one (an agent gets the same data from `search ... --json | jq`).
`info` is an alias. Read-only; needs a resolvable origin but no git working tree;
the skill name is matched exactly. A name the origin does not publish is exit 1
(run `skillrig search` to list the real names).

```sh
# Show a skill's full details (alias: skillrig info <skill>).
skillrig show terraform-plan-review

# The complete record as JSON, for an agent or jq.
skillrig show terraform-plan-review --json
```

### `add`

Vendor a named skill from the configured origin into the canonical
`.agents/skills/<skill>/`, recording its identity (version, commit, tree-SHA,
path) in `.skillrig/skills-lock.json`. `add` copies the skill byte-identically,
injecting nothing. It is idempotent on identical content and refuses to overwrite
a vendored skill whose on-disk content diverges from the lock unless you pass
`--force`. Requires a git repository; commit the result, then run `skillrig
verify`.

The acquisition form is chosen automatically and reported in the result:

- **Local** — the configured origin is checked out at `<repo-root>/OWNER/REPO`;
  `add` reads that checkout (no network). Keep the checkout out of your index
  (e.g. `echo 'my-org/' >> .git/info/exclude`).
- **Remote** — no local checkout exists; `add` fetches the skill subtree over git
  from the origin, using a GitHub token from `GH_TOKEN` / `GITHUB_TOKEN` /
  `gh auth token` when one is available (public origins need none).

```sh
# Vendor a skill from your configured origin.
skillrig add terraform-plan-review

# Pin an immutable version (a bare semver expands via the origin's tag scheme;
# anything else is a literal git tag or commit SHA).
skillrig add terraform-plan-review --pin v1.4.0

# Preview what would be vendored, writing nothing.
skillrig add terraform-plan-review --dry-run

# Overwrite a locally-diverged copy with the origin's content.
skillrig add terraform-plan-review --force
```

### `verify`

Check **this** repository's vendored skills (`.agents/skills`) against the
committed lock (`.skillrig/skills-lock.json`) — label-honesty (git tree-SHA) plus
orphan/completeness — offline, deterministic, and read-only (it recomputes
tree-SHAs and writes nothing). It needs no origin and no network, which makes it a
load-bearing CI gate.

```sh
# Verify this repo's vendored skills match their recorded versions (CI gate).
skillrig verify

# Machine-readable per-skill verdicts for an agent / jq.
skillrig verify --json
```

`verify` exits `0` when everything matches, `2` on any verification failure
(mismatch / orphan / missing / dirty), and `1` on a usage problem (e.g. a
malformed lock or not a git repo).

### `index`

**Origin-maintainer command.** Run inside the origin repo (locally or in CI on
merge) to regenerate the origin's machine-readable catalog (`index.json`) by
walking its skills directory and parsing each skill's `SKILL.md` frontmatter —
the same parser `add` and `verify` use. It is a single-tip, full regeneration:
the catalog is overwritten wholesale, sorted by name with a stable key order, so
regenerating over an unchanged skill set is byte-identical.

```sh
# Regenerate index.json at the origin repo root.
skillrig index

# Write the catalog to an explicit path.
skillrig index --out catalog/index.json

# Machine-readable summary of what was generated.
skillrig index --json
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

Exit codes are part of the contract — scripts and agents branch on them, so their
meanings are fixed:

| Code | Meaning |
|------|---------|
| `0`  | Success (including an idempotent no-op) |
| `1`  | Usage or configuration error (bad flags, invalid origin, no origin configured) |
| `2`  | Verification failure (`verify` found a mismatch / orphan / missing / dirty skill) |

Code `3` (prerequisite) is reserved for a future command.

Errors go to **stderr**; data goes to **stdout**.

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

## Backing CLIs (mise backend)

A skill can require a backing CLI built in the same origin monorepo (`metadata.x-skillrig.requires`).
`skillrig` itself never installs binaries — provisioning is delegated to [mise](https://mise.jdx.dev).
For origins that ship **multiple** CLIs released on independent version streams, the
**[`skillrig/mise-skillrig`](https://github.com/skillrig/mise-skillrig)** mise backend plugin
lets you co-install them as distinct tools from one repo:

```sh
mise plugin install skillrig https://github.com/skillrig/mise-skillrig
export MISE_GITHUB_TOKEN=$(gh auth token)
mise use skillrig:my-org/our-skills/jira@1.7.0
mise use skillrig:my-org/our-skills/tfc@latest   # two tools, one repo, distinct versions
```

This works where native mise cannot — independent versioning under a strict-semver
(build-metadata) tag policy. The design rationale, the origin contract, and the planned
`skillrig` CLI integration (metadata-driven resolution + `skillrig add` auto-wiring) are in
[**RFC 0001**](docs/rfcs/0001-mise-skillrig-backend.md) and tracked in
[docs/ROADMAP.md](docs/ROADMAP.md).
