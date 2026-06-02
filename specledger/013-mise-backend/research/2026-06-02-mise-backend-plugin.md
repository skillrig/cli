# Research: mise backend for skillrig — multi-binary co-install from one origin monorepo

**Date**: 2026-06-02
**Context**: Issue [#23](https://github.com/skillrig/cli/issues/23) proposes a `skillrig`
mise **backend plugin** (vfox-style, Lua) so that an origin monorepo shipping N backing
CLIs in `cmd/` can be consumed as N distinct mise tools (`skillrig:jira`, `skillrig:tfc`,
…). The trigger was a validated collision: mise's stock `github` backend keyed a tool by
`owner/repo` and tracked one version per tool, so co-installing 2+ release streams from one
repo collapsed into a single tool. This spike answers the issue's four open questions and
the prior `ARCHITECTURE-v0 §8b` claims, to decide **whether** to build the plugin and, if
so, the contract it depends on.
**Time-box**: ~45 min (web + docs).

## Question

1. **OQ4 (gate):** Does a *current* mise (`asset_pattern` + aliases, ≥2026.5) cover
   multi-binary-from-one-repo **natively**, before we commit to a plugin?
2. **OQ2:** Origin-metadata contract — does `.skillrig-origin.toml` / `index.json` already
   carry per-binary stream + asset conventions, or do we add a `[[binaries]]` section?
3. **OQ3:** Verification depth for v1 — checksum-only vs. full provenance / treeSha tie-in.
4. **OQ1:** Plugin home / name & its own release cadence.

Plus: validate the backend-plugin hook surface and the auth gotcha the issue documents.

---

## Findings

### Finding 1 — PR #9093 fixes the **scheduler**, not version resolution — partial fix only — *high confidence*

> **Correction (after reviewing the PR, 2026-06-02).** An earlier draft of this finding
> claimed mise 2026.4.12 fully resolves the multi-binary case. That is **wrong** — it
> conflated two independent layers. The fix is necessary but **not sufficient** for the
> origin's actual tag scheme (see Finding 1b).

There are **two layers**, and PR #9093 touches only the first:

- **Layer A — install scheduler dedup.** The old dependency-graph node key was
  `<backend_full>@<version>`, so two aliases resolving to the same
  `github:owner/repo@<version>` collapsed into one install job. **PR #9093** re-keys nodes to
  `<tool_short_name>@<version>`, so distinct aliases get distinct scheduler slots and each
  installs with its own options (`asset_pattern`, `bin_path`, `postinstall`). The PR
  description explicitly states it **"does not touch version listing or resolution."** Shipped
  in **mise 2026.4.12** (discussion #9074, *"tested with 2026.4.12, works OK ✅"*).
- **Layer B — version listing / resolution.** *Unchanged by #9093.* This is where the
  origin's tag scheme breaks (Finding 1b).

The native multi-asset pattern (discussion #8266) — which #9093 makes work — is `[tool_alias]`
+ per-platform `asset_pattern`:

```toml
[tool_alias]
guest-init = "github:jingkaihe/matchlock"

[tools."github:jingkaihe/matchlock"]
version = "latest"
platforms.linux-x64   = { asset_pattern = "matchlock-linux-amd64" }

[tools.guest-init]
version = "latest"
platforms.linux-x64   = { asset_pattern = "guest-init-linux-amd64" }
```

This works **when both binaries ship in one release at one version** (each alias picks its
asset by pattern). It does **not** give *independent* version streams — see Finding 1b.

### Finding 1b — semver **build-metadata** streams are structurally unresolvable natively — *high confidence (by spec)*

The origin's constraint: its **tag policy enforces strict semver**, which *forbids* prefix
tags (`iii-v0.1.0` is not valid semver — leading non-numeric) but *permits* **build-metadata**
tags (`0.1.0+iii` is valid semver). So the org expresses independent CLI streams as
`v0.5.0+iii`, `v0.2.0+console`, etc.

Per **SemVer 2.0.0**: *"Build metadata MUST be ignored when determining version precedence …
two versions that differ only in the build metadata have the same precedence."* Every
semver-respecting resolver (npm, Helm, and mise's `latest`/range resolution) treats
`0.5.0+iii` and `0.5.0+console` as the **same version**. Consequences for the github backend:

- The listed version set collapses to `{0.5.0, 0.2.0}` with **no stream identity**.
- `latest` for the `console` alias resolves to `0.5.0` (the max across **all** streams), then
  looks for a `console-*` asset in the `0.5.0+iii` release — **which doesn't contain it** →
  install fails / wrong binary.
- mise's `version_prefix` only strips a **leading** prefix; there is **no** `version_suffix`
  or `tag_regex`, so a `+iii` **suffix** cannot select a stream.
- Even *exact* pins (`= "0.5.0+iii"`) are brittle: precedence-based matching treats build
  metadata as equal, and it defeats the point of tracking `latest` per stream.

**So the native design space is:**

| Option | Independent versions? | Co-location? | Strict-semver tags? | Native mise works? |
|---|---|---|---|---|
| (a) prefix streams `iii-v*` + `version_prefix` | ✅ | ✅ | ❌ (forbidden by policy) | ✅ |
| (b) build-metadata streams `0.5.0+iii` | ✅ | ✅ | ✅ | ❌ **broken (Layer B)** |
| (c) one release, all binaries, `asset_pattern` | ❌ | ✅ | ✅ | ✅ (post-#9093) |
| (d) separate repo per CLI | ✅ | ❌ | ✅ | ✅ |
| (e) **custom `skillrig:` backend plugin** | ✅ | ✅ | ✅ | ✅ (plugin owns listing) |

**Implication — this *flips* the earlier conclusion.** A dedicated `skillrig:` backend plugin
is justified on **capability**, not merely ergonomics: its `BackendListVersions` hook **owns
version listing**, so it can parse the `+iii` build metadata, filter tags to that stream, and
present a clean per-tool version list — the one thing native mise structurally cannot do for
build-metadata streams under a strict-semver tag policy. Options (a)/(c)/(d) each sacrifice
something the architecture wants (policy compliance / independent versioning / co-location).

### Finding 2 — `tag_regex` does not exist; the real knob is per-tool `version_prefix` — *high confidence*

The issue (and `ARCHITECTURE-v0 §8b`) assume a `tag_regex` option keyed per tool. The
github-backend docs describe **no** `tag_regex`. Per-stream tag selection in a monorepo is
done with **`version_prefix`**:

```toml
[tools]
"github:my-org/my-skills" = { version = "latest", version_prefix = "jira-v" }
```

So a build-metadata stream (`1.7.0+jira`) is *not* natively selectable, but a **prefix**
stream (`jira-v1.7.0`) is — via `version_prefix`. This matters: it means the **prefix tag
convention** (which release-please already produces per-package) is the natively-supported
one, and the origin template should standardize on it.

### Finding 3 — backend-plugin hook surface (vfox-style) — *high confidence*

Three Lua hooks under `hooks/`, addressed `plugin:tool`:

| Hook | ctx fields | returns |
|---|---|---|
| `BackendListVersions(ctx)` | `ctx.tool`, `ctx.options` | `{versions = {…ascending semver…}}` |
| `BackendInstall(ctx)` | `ctx.tool`, `ctx.version`, `ctx.install_path`, `ctx.download_path`, `ctx.options` | `{}` |
| `BackendExecEnv(ctx)` | `ctx.tool`, `ctx.version`, `ctx.install_path`, `ctx.options` | `{env_vars = {{key="PATH", value=install_path.."/bin"}}}` |

- `RUNTIME` injects `osType` / `archType` / `envType` (gnu|musl) for asset selection.
- **The plugin owns download + extraction + verification** into `install_path`; mise does
  **not** checksum-verify for a custom backend (unlike the first-party `github` backend,
  which does sha256 + `mise.lock`). So a plugin must *re-implement* checksum verification
  that the native `github` backend gives for free.
- A backend plugin is a **git repo** addressed `plugin:tool`; the plugin name need not match
  the repo. Installed via `mise plugin install <name> <giturl>`; optional PR to the mise
  registry for a short name. There is a `jdx/mise-backend-plugin-template` (hooks +
  `metadata.lua` + CI + lint).

### Finding 4 — auth: the gotcha is real, but mise has first-class hooks — *high confidence*

The issue's claim is confirmed: mise reads gh's token from `hosts.yml` **but cannot read a
gh token stored in an OS keyring/credential-helper** (the common macOS case), so it falls
back to anonymous and a private origin's `/releases` returns **404**. mise's documented
token precedence (github.com):

1. `MISE_GITHUB_TOKEN` → 2. `GITHUB_API_TOKEN` → 3. `GITHUB_TOKEN` →
4. `settings.github.credential_command` → 5. native GitHub OAuth →
6. `github_tokens.toml` (per-host) → 7. gh CLI `hosts.yml` → 8. `git credential fill`.

- The **enterprise-clean path is `credential_command`** (host passed via
  `MISE_CREDENTIAL_HOST`), e.g. `op read 'op://Private/GitHub Token/credential'` — works for
  the *native* github backend too, no plugin required.
- **Correction to §8b:** the doc already self-corrected that env vars precede
  `credential_command`; this spike confirms env-vars-first (1–3 above 4).

> Net: "central auth" is **not** a plugin-only value-add — `credential_command` +
> `MISE_GITHUB_TOKEN` already centralize it for the native backend. The keyring-404 is fixed
> by *documenting* `MISE_GITHUB_TOKEN=$(gh auth token)` or a `credential_command`, with or
> without a plugin.

### Finding 5 — what a plugin buys (after the Finding 1b correction) — *high confidence on #1, medium on rest*

The plugin's value is now led by **capability** (Finding 1b), with ergonomics following:

1. **Resolves build-metadata streams — the capability native mise lacks.** Under a
   strict-semver tag policy, `BackendListVersions` is the *only* place that can map
   `0.5.0+iii` / `0.2.0+console` tags to distinct per-tool version lists. Native mise cannot
   (Layer B / SemVer precedence). This is the load-bearing justification.
2. **Zero per-consumer boilerplate.** Even where native co-install *could* work (prefix
   streams), it needs the consumer to hand-author per-platform `asset_pattern` +
   `version_prefix` for *every* binary and to *know* each asset-name template. A `skillrig:`
   backend learns stream + asset template from the **origin's own metadata**, so the consumer
   writes only `skillrig:jira = "latest"`.
3. **One addressing scheme** (`plugin:tool`) instead of `[tool_alias]` indirection.
4. **Tamper-evidence parity with skills** — tie the binary's release tag/commit to the
   `treeSha`/`commit` skillrig already records in `.skillrig/skills-lock.json`.

…against real costs: a **separate Lua repo + release cadence**, and **re-implementing**
download/checksum that the native `github` backend provides for free.

### Finding 6 — verification depth options — *medium confidence*

- Native `github` backend: sha256 `checksum` + `mise.lock` lockfile; SLSA/GPG **not**
  documented for the github backend (the issue's "optionally SLSA/attestation" is
  aspirational, not a current github-backend feature).
- A custom backend must implement its own checksum verify (Finding 3). The origin already
  ships `_checksums.txt` per release (per the issue's evidence), so checksum-verify is cheap.
- **Provenance/treeSha tie-in** (bind binary tag↔skill `commit`) is the genuinely
  skillrig-specific guarantee, but it's strictly more than mise gives natively and belongs in
  a later phase.

---

## Decisions

- **OQ4 — PARTIALLY resolved; plugin still justified.** PR #9093 (mise 2026.4.12) fixes the
  install **scheduler** dedup (Layer A) — so multi-asset-from-one-release and same-version
  distinct aliases now work. It does **not** touch version **resolution** (Layer B). The
  origin's strict-semver tag policy forces **build-metadata** streams (`0.5.0+iii`), which
  SemVer precedence collapses — natively unresolvable, and no `version_prefix`/`tag_regex`
  can select a `+` suffix. So native mise covers the *easy* shapes but **not** the origin's
  actual one. **The plugin is justified on capability** (its `BackendListVersions` owns
  listing), not just ergonomics. The earlier "RESOLVED — ergonomics only" conclusion was
  **wrong** and is corrected here (Findings 1, 1b).
- **OQ1.** If built, the plugin lives in its **own git repo** (e.g. `skillrig/mise-skillrig`),
  backend name `skillrig` (used as `skillrig:<tool>`), released on its **own cadence**
  independent of the CLI, optionally registered in the mise plugin registry. It is **not**
  Go code in `skillrig/cli`.
- **OQ2.** The per-binary stream + asset conventions are **not** in `index.json`/origin
  config today. They must be added as a **convention-versioned** contract (a `[[binaries]]`
  / `tools` block in `.skillrig-origin.toml`, surfaced into `index.json`). This is
  **`skillrig/cli` + origin-template work regardless** of plugin-vs-native, because both the
  plugin *and* a native-stanza generator read the same metadata.
- **OQ3.** v1 = **checksum-only** (sha256 against the origin's `_checksums.txt`), matching
  what the native backend already does; **provenance/treeSha binding is v2** behind a real
  trigger.
- **§8b correction needed.** `tag_regex` → `version_prefix` (a *leading*-prefix stripper, no
  suffix/regex selector exists); the one-binary-per-entry framing is **partly** superseded by
  2026.4.12 (scheduler only); "central auth" is achievable natively via
  `credential_command`/`MISE_GITHUB_TOKEN`.

## Recommendations

1. **Build the dedicated `skillrig:` backend plugin** in its own repo — it is the only option
   that satisfies *independent versioning + co-location + strict-semver tag policy*
   simultaneously (Finding 1b, option (e)). The RFC commits to it as the new repo's reason to
   exist. Its `BackendListVersions` parses build-metadata streams; `BackendInstall`
   downloads + checksum-verifies the per-tool asset; `BackendExecEnv` puts it on PATH.
2. **Land the origin-metadata contract first**, in `skillrig/cli` + the origin template:
   a **convention-versioned** `[[binaries]]` block in `.skillrig-origin.toml` surfaced into
   `index.json`, carrying each binary's **stream selector** (build-metadata tag suffix) +
   **asset-name template** per os/arch + checksum source. Both the plugin and any
   `skillrig add` auto-wiring read this one contract (AP-04).
3. **Document the native fallbacks honestly** in the RFC (options (a)/(c)/(d)) so adopters who
   *can* relax the tag policy to prefix streams, or accept monolithic releases, aren't forced
   onto the plugin. The plugin is the answer **for the strict-semver + independent-versioning
   org**; it is not the only way to consume an origin's CLIs.
4. **OQ3 verification:** checksum-only for v1 (origin already ships `_checksums.txt`);
   treeSha/commit binding to `.skillrig/skills-lock.json` is v2.
5. **Correct `ARCHITECTURE-v0 §8b`** per the decisions above and pin the convention-version
   that carries the new `[[binaries]]` block; **require mise ≥ 2026.4.12** (Layer A fix) in
   the template.

## References

- Issue #23 — skillrig/cli (this repo).
- jdx/mise discussion **#9074** + **PR #9093** — multi-binary dedup fix (2026.4.12).
- jdx/mise discussion **#8266** — `tool_alias` + `asset_pattern` multi-binary pattern.
- mise docs — *GitHub Backend* (`asset_pattern`, `version_prefix`, `platforms`, `checksum`,
  `mise.lock`, `api_url`).
- mise docs — *Backend Plugin Development* (hooks, ctx fields, `plugin:tool`, distribution)
  + `jdx/mise-backend-plugin-template`.
- mise docs — *GitHub Tokens* (token precedence; keyring-404; `credential_command` /
  `MISE_CREDENTIAL_HOST`).
- vfox docs — backend/plugin authoring (`RUNTIME` object).
