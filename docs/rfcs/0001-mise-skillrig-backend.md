# RFC 0001 — The `skillrig` mise backend plugin

**Status:** ✅ **Accepted — plugin BUILT, published, and validated end-to-end (2026-06-05).**
**Plugin repo:** [`skillrig/mise-skillrig`](https://github.com/skillrig/mise-skillrig) (public)
**Author:** generated from issue [#23](https://github.com/skillrig/cli/issues/23)
**Spike:** [`specledger/013-mise-backend/research/2026-06-02-mise-backend-plugin.md`](../../specledger/013-mise-backend/research/2026-06-02-mise-backend-plugin.md)
**Relates to:** `docs/ARCHITECTURE-v0.md` §8 (backing-CLI provisioning), §8b (mise realities), §13 vNext
**Bootstrapped:** the separate **`skillrig/mise-skillrig`** repo (the plugin is Lua, not Go; it does **not** live in `skillrig/cli`)

> **Scope note (pre-release marker, per `CLAUDE.md`).** No backward compatibility is planned.
> This RFC defines a new artifact and an (optional) convention-versioned origin contract; it
> may change freely.

> **Implementation status (2026-06-05).** The plugin specified here is **built and validated**:
> two backing CLIs co-installed from one private monorepo as distinct, independently-versioned
> tools — the capability native mise cannot provide. Verified with 54 offline unit tests,
> `stylua` + `lua-language-server` clean, CI green on ubuntu + macOS, an adversarial
> multi-lens review, and a live two-tool co-install. **§5–§8 and §12 below have been
> reconciled with what actually shipped** — the four divergences are called out inline as
> *“Shipped:”* notes. Quickstart:
> ```sh
> mise settings experimental=true
> mise plugin install skillrig https://github.com/skillrig/mise-skillrig
> export MISE_GITHUB_TOKEN=$(gh auth token)
> mise use skillrig:my-org/our-skills/jira@1.7.0
> mise use skillrig:my-org/our-skills/tfc@latest   # two tools, one repo, distinct versions
> ```

---

## 1. Summary

skillrig origins are **co-located monorepos**: one repo holds an org's agent skills *and* the
private backing CLIs those skills require (`cmd/`), released by the same pipeline so a skill
and its tool version, release, and are vendored/verified as one unit (`ARCHITECTURE-v0` §1).
A skill declares its tool via `metadata.x-skillrig.requires`; provisioning of the binary is
delegated to **mise** (`ARCHITECTURE-v0` §8 / R17 — *"skillrig declares and verifies, mise
installs"*).

This RFC specifies a **mise backend plugin** named `skillrig` so that N backing CLIs in one
origin become N **distinct** mise tools, addressed `skillrig:<owner>/<repo>/<bin>@<version>`,
each tracking its own independent release stream:

```toml
# consumer mise.toml
"skillrig:my-org/our-skills/jira" = "1.7.0"
"skillrig:my-org/our-skills/tfc"  = "latest"   # two tools, one repo, distinct versions
```

> **Shipped (divergence #2):** the GitHub coordinate is embedded **in the address**
> (`skillrig:<owner>/<repo>/<bin>`), not a bare `skillrig:<bin>` plus a separate `origin`
> option. A bare `skillrig:<bin>` + an `origin` option / `SKILLRIG_ORIGIN` env also works as a
> shorthand. This avoids per-tool `origin` config and matches the origin's
> `docs/BINARY-DISTRIBUTION.md` §4.

It is a separate, independently-released repo. This document also defines an **optional
origin-side contract** (a convention-versioned `[[binaries]]` block) and the changes to the
**origin template** and **`skillrig` CLI** that make the three pieces work together.

> **Shipped (divergence #1):** the `[[binaries]]` block is **not required**. The plugin is
> **convention-driven** — it derives stream/asset/checksum names from the goreleaser
> convention and reads the binary from each tag's build metadata, so it works against an
> origin that has **no** `[[binaries]]` block. The block (P1, §5/§12) is an *optional
> metadata-driven override*, not a prerequisite.

## 2. Motivation — why native mise is not enough *for this origin*

The naive plan ("one monorepo, consume each CLI via mise's `github` backend") fails, and the
obvious fixes don't apply under the origin's tag policy. There are **two independent layers**:

- **Layer A — install scheduler.** mise once keyed install jobs by `<backend_full>@<version>`,
  so two aliases pointing at `github:org/repo@<same version>` deduped into one job (only one
  binary installed). **Fixed in mise 2026.4.12 (PR #9093)** by re-keying to
  `<tool_short_name>@<version>`. *Necessary, and we require it — but it only fixes Layer A.*
- **Layer B — version resolution.** PR #9093 explicitly *"does not touch version listing or
  resolution."* The origin's **tag policy enforces strict semver**, which **forbids** prefix
  tags (`iii-v0.1.0` is not valid semver) but **permits** build-metadata tags (`0.1.0+iii`
  is). Per SemVer 2.0.0, *build metadata MUST be ignored for precedence* — so `0.5.0+iii` and
  `0.2.0+console` are indistinguishable to mise's resolver: the version set collapses,
  `latest` for one stream resolves to the max across **all** streams, and `version_prefix`
  (a *leading*-prefix stripper — there is **no** `tag_regex`/suffix selector) cannot pick a
  `+iii` suffix.

The full native design space (spike Finding 1b):

| Option | Independent versions | Co-location | Strict-semver tags | Native mise |
|---|:---:|:---:|:---:|:---:|
| (a) prefix streams `iii-v*` + `version_prefix` | ✅ | ✅ | ❌ forbidden by policy | ✅ |
| (b) build-metadata streams `0.5.0+iii` | ✅ | ✅ | ✅ | ❌ **broken (Layer B)** |
| (c) one release, all binaries, `asset_pattern` | ❌ | ✅ | ✅ | ✅ (post-#9093) |
| (d) separate repo per CLI | ✅ | ❌ | ✅ | ✅ |
| (e) **`skillrig` backend plugin** | ✅ | ✅ | ✅ | ✅ (plugin owns listing) |

Only **(e)** satisfies *independent versioning + co-location + strict-semver* together,
because a backend plugin's `BackendListVersions` hook **owns version listing** and can map
build-metadata tags to per-tool version streams — the one thing native mise structurally
cannot do. That is the capability justification for this plugin.

## 3. Goals / non-goals

**Goals**
- Co-install any number of an origin's backing CLIs as distinct `skillrig:<tool>` mise tools,
  each tracking its own build-metadata release stream, on a strict-semver origin.
- Drive resolution from the **origin's own metadata** (zero per-consumer `asset_pattern`
  boilerplate); the consumer writes only `skillrig:<tool> = "<version>"`.
- **Checksum-verify** every downloaded binary against the origin's published checksums.
- Resolve a GitHub token centrally (handles the private-origin keyring-404 gotcha).
- Keep skillrig's "no extra architecture" promise: a small published plugin + a metadata
  block in the origin. **No registry/index service.**

**Non-goals (v1)**
- Replacing mise. skillrig still *declares + verifies*; mise *installs* (R17).
- Binding the binary to the skill's `treeSha`/`commit` (tamper-evidence parity) — **v2**.
- SLSA/GPG attestation — **v2** (origin ships sha256 checksums today).
- Public-CLI provisioning (`terraform`, `gh`) — those stay on mise's stock backends.
- Windows-first support — best-effort; Linux/macOS are the v1 targets.

## 4. How the three pieces fit together

```
┌─────────────────────────────────────────────────────────────────────────┐
│ ORIGIN MONOREPO  my-org/my-skills   (stood up from the origin template)   │
│                                                                           │
│  cmd/iii/  cmd/console/ ...        ── goreleaser builds per-CLI assets     │
│  release-please (per-package)      ── cuts tags  v0.5.0+iii , v0.2.0+cons. │
│  releases:  iii_0.5.0_linux_amd64.tar.gz + checksums.txt   (per stream)    │
│                                                                           │
│  .skillrig-origin.toml   ─ [[binaries]] : stream selector + asset template│
│  index.json              ─ generated by `skillrig index`; mirrors binaries│
└───────────────┬───────────────────────────────────────────────────────────┘
                │ (1) plugin fetches index.json/.skillrig-origin.toml + tags + assets
                │     authenticated via MISE_GITHUB_TOKEN / GITHUB_TOKEN / gh
                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ mise  +  skillrig backend plugin   (skillrig/mise-skillrig, Lua)          │
│   BackendListVersions → list tags, filter by `+iii`, return clean semver   │
│   BackendInstall      → resolve tag, pick asset, sha256-verify, extract    │
│   BackendExecEnv       → put bin on PATH                                    │
└───────────────┬───────────────────────────────────────────────────────────┘
                │ (2)
                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ CONSUMER REPO                                                             │
│   mise.toml:   "skillrig:iii" = "latest"      ← written by `skillrig add` │
│   .skillrig/config.toml: origin = "my-org/my-skills"                       │
│   skill's SKILL.md: metadata.x-skillrig.requires: [{tool: iii, ...}]       │
└───────────────────────────────────────────────────────────────────────────┘
```

1. **Origin template** ships the release pipeline that produces per-stream build-metadata tags
   (`1.7.0+jira`) + `<bin>_<ver>_<os>_<arch>.tar.gz` assets + per-tool `<bin>_checksums.txt`;
   optionally the `[[binaries]]` metadata describing them (§5, P1).
2. **The plugin** resolves/installs each tool — convention-driven, preferring `[[binaries]]`
   metadata when present (§6).
3. **The `skillrig` CLI** auto-wires the consumer's `mise.toml` when a vendored skill requires
   a binary, and reuses its own origin resolution + token resolver (§7, RFC P3).

## 5. The origin-side contract (`[[binaries]]`) — OQ2 — **optional override (P1)**

> **Shipped (divergence #1 & #3).** The plugin does **not** need this block. It is
> **convention-driven**, deriving everything from the goreleaser release convention the origin
> template already produces:
> - **Tags** are `<core>+<bin>` — **no leading `v`**, build metadata is the **bare tool
>   name**: real tags are `1.7.0+jira`, `0.0.6+tfc` (the §-examples below previously wrote
>   `v0.5.0+iii`; corrected here). Prefix streams `<bin>-v<core>` are also supported.
> - **Assets** are `<bin>_<ver>_<os>_<arch>.tar.gz`.
> - **Checksums** are **per-tool** `<bin>_checksums.txt` (e.g. `jira_checksums.txt`), **not** a
>   single shared `checksums.txt`.
>
> The `[[binaries]]` block below is therefore the **P1 metadata-driven override** — adopt it
> when an origin diverges from the convention or wants explicit control; the plugin prefers it
> when present and falls back to the convention otherwise. One contract, two readers (plugin +
> CLI) — AP-04.

```toml
# .skillrig-origin.toml  (origin repo root) — OPTIONAL
skillrig-convention = 1
origin = "foo-org-sports/local-devops-scripts"   # identity LABEL — see divergence #4 below

[[binaries]]
name      = "jira"                # → mise tool `skillrig:<owner>/<repo>/jira`
stream    = "+jira"               # semver BUILD-METADATA suffix (bare tool name; no leading v on tags)
asset     = "jira_{version}_{os}_{arch}.tar.gz"   # {version}=semver core, {os}/{arch} mapped
checksums = "jira_checksums.txt"  # PER-TOOL checksums asset; sha256 <sp> filename lines
bin       = "jira"                # executable path inside the archive (post strip)

  # optional: map mise RUNTIME tokens → this asset's tokens (defaults: linux/darwin, amd64/arm64)
  [binaries.platforms.linux-x64]
  os = "linux"
  arch = "amd64"
  [binaries.platforms.darwin-arm64]
  os = "darwin"
  arch = "arm64"

[[binaries]]
name   = "tfc"
stream = "+tfc"
asset  = "tfc_{version}_{os}_{arch}.tar.gz"
checksums = "tfc_checksums.txt"
bin    = "tfc"
```

> **Shipped (divergence #4): `origin` is an identity *label*, not the hosting coordinate.** The
> demo origin's files say `origin = "foo-org-sports/local-devops-scripts"` while it is hosted
> at `so0k/skillrig-origin-demo`. The plugin therefore keys off the **address coordinate**
> (`skillrig:<owner>/<repo>/<bin>`) and verifies only `skillrigConvention` — it **never**
> checks the origin name.

When the override is present, `skillrig index` emits it into `index.json`:

```jsonc
{
  "skillrigConvention": 1,
  "origin": "foo-org-sports/local-devops-scripts",   // identity label only
  "binaries": [
    { "name": "jira", "stream": "+jira",
      "asset": "jira_{version}_{os}_{arch}.tar.gz",
      "checksums": "jira_checksums.txt", "bin": "jira",
      "platforms": { "linux-x64": {"os":"linux","arch":"amd64"}, "...": {} } }
  ],
  "skills": { /* unchanged */ }
}
```

**Convention versioning (R5e).** The plugin best-effort-reads `skillrigConvention` from
`index.json` (currently `1`) and fails clearly against an incompatible origin rather than
mis-resolving. It **never** checks the origin **name** (divergence #4 — that field is an
identity label that legitimately differs from the hosting coordinate).

## 6. The plugin — design (OQ1)

**Repo / naming.** Repo **[`skillrig/mise-skillrig`](https://github.com/skillrig/mise-skillrig)**
(public, shipped); mise backend name `skillrig`; tools addressed
**`skillrig:<owner>/<repo>/<bin>`** (the plugin name need not match the repo). Installed via
`mise plugin install skillrig https://github.com/skillrig/mise-skillrig` (or registered in the
mise plugin registry for the short name). **Released on its own cadence**, independent of the
`skillrig` CLI.

**Repo layout (bootstrap target).**

```
skillrig/mise-skillrig/
├── metadata.lua                  # plugin name, version, author
├── hooks/
│   ├── backend_list_versions.lua # BackendListVersions
│   ├── backend_install.lua       # BackendInstall
│   └── backend_exec_env.lua      # BackendExecEnv
├── lib/
│   ├── origin.lua                # resolve origin + fetch index.json/.skillrig-origin.toml
│   ├── github.lua                # authed GitHub API (tags, releases, asset download)
│   ├── stream.lua                # build-metadata stream parse/filter/sort
│   └── checksum.lua              # sha256 verify against checksums.txt
├── mise-tasks/                   # test/lint tasks
├── .github/workflows/            # CI + release
└── README.md
```

**Origin resolution (inside the plugin).** The origin coordinate is normally **embedded in the
address** (`skillrig:<owner>/<repo>/<bin>`), so no extra config is needed. For the bare
`skillrig:<bin>` shorthand, precedence (mirrors the CLI): `ctx.options.origin` (per-tool in
`mise.toml`) → `SKILLRIG_ORIGIN` env → error with a fix. (Recall: this `<owner>/<repo>` is the
**hosting coordinate**, distinct from the origin's `origin` identity-label field — divergence
#4.)

```toml
# preferred: coordinate in the address (no per-tool origin config)
"skillrig:my-org/our-skills/jira" = "1.7.0"

# shorthand: bare tool + explicit origin
[tools."skillrig:jira"]
version = "latest"
origin  = "my-org/our-skills"
```

### 6.1 `BackendListVersions` — the load-bearing hook

Owns version listing, which is exactly why native mise can't do this. It fetches the origin's
tags, keeps only those whose build metadata matches the tool's `stream`, strips the `+stream`
suffix, and returns clean semver cores ascending.

```lua
-- hooks/backend_list_versions.lua
function PLUGIN:BackendListVersions(ctx)
  local meta   = origin.binary_meta(ctx.tool)          -- coordinate+bin from the address; index.json optional
  local tags   = github.list_tags(meta.repo)            -- authed; real tags e.g. {"1.7.0+jira","0.0.6+tfc"}
  local versions = {}
  for _, tag in ipairs(tags) do
    local core, build = stream.parse(tag)               -- "1.7.0", "jira"  (no leading v; bare tool name)
    if build == meta.bin then                           -- build metadata == this tool's stream
      versions[#versions + 1] = core
    end
  end
  stream.sort_semver_asc(versions)                       -- ascending; mise applies no extra sort
  return { versions = versions }
end
```

`latest` then naturally resolves to the max **within the stream** — the behavior native mise
cannot produce for build-metadata tags.

### 6.2 `BackendInstall` — resolve, download, verify, extract

```lua
-- hooks/backend_install.lua
function PLUGIN:BackendInstall(ctx)
  local meta = origin.binary_meta(ctx.tool)
  local tag  = ctx.version .. "+" .. meta.bin             -- "1.7.0" + "+jira" → "1.7.0+jira" (no leading v)
  local rel  = github.release_by_tag(meta.repo, tag)      -- authed

  local plat = meta.platform()                            -- RUNTIME os/arch → asset tokens
  local name = meta.asset(ctx.version, plat)              -- "jira_1.7.0_linux_amd64.tar.gz"

  local asset = github.find_asset(rel, name)
  local file  = github.download(asset, ctx.download_path)

  -- checksum verify (mise does NOT verify for CUSTOM backends — the plugin must).
  -- checksums asset is PER-TOOL: "<bin>_checksums.txt", e.g. "jira_checksums.txt".
  local sums  = github.download(github.find_asset(rel, meta.bin .. "_checksums.txt"), ctx.download_path)
  checksum.verify_sha256(file, name, sums)                -- abort on mismatch

  archive.extract(file, ctx.install_path, { strip = "auto" })
  -- normalize the binary into install_path/bin/<bin> so BackendExecEnv stays trivial
  return {}
end
```

### 6.3 `BackendExecEnv` — PATH

```lua
-- hooks/backend_exec_env.lua
function PLUGIN:BackendExecEnv(ctx)
  return { env_vars = { { key = "PATH", value = ctx.install_path .. "/bin" } } }
end
```

(If `bin` is at the archive root rather than `bin/`, `BackendInstall` normalizes it into
`install_path/bin/<name>` so this hook stays trivial.)

## 7. Auth (the keyring-404 gotcha) and CLI integration

**Token resolution.** mise's own github-token precedence already centralizes this for the
plugin's GitHub API calls: `MISE_GITHUB_TOKEN` → `GITHUB_API_TOKEN` → `GITHUB_TOKEN` →
`settings.github.credential_command` (host via `MISE_CREDENTIAL_HOST`) → native OAuth →
`github_tokens.toml` → gh `hosts.yml` → `git credential fill`. The documented gotcha —
**mise cannot read a gh token stored in the OS keyring** (only `hosts.yml`) — is handled by
documenting either `MISE_GITHUB_TOKEN=$(gh auth token)` or a `credential_command`. The plugin
reuses this resolver via mise's HTTP/token helpers; it introduces **no new credential
surface** (consistent with `ARCHITECTURE-v0` §2b — no write credential anywhere).

**`skillrig` CLI auto-wiring (in this repo, `skillrig/cli` — RFC P3).** When `skillrig add
<skill>` vendors a skill whose `metadata.x-skillrig.requires` names a binary sourced from the
origin, the CLI writes the matching `mise.toml` stanza, embedding the origin coordinate in the
address (divergence #2):

```toml
"skillrig:my-org/our-skills/jira" = ">=0.4.0"     # from requires[].version
```

This is the one piece of plugin support that belongs in the Go CLI (it owns `add`, origin
resolution, and the lock). It is gated behind the same dry-run/force discipline as `add`. See
the roadmap entry "mise backend integration — CLI side" in `docs/ROADMAP.md`.

## 8. Verification depth (OQ3)

- **v1 — checksum-only (shipped).** sha256 against the origin's **per-tool**
  `<bin>_checksums.txt`. mise performs **no** verification for a *custom* backend (unlike the
  native `github` backend), so the plugin implements this itself — a `skillrig:` tool gets no
  mise-side SLSA/GPG today regardless of native-backend support.
- **v2 — provenance / treeSha parity.** Bind the binary's release tag/commit to the
  `treeSha`/`commit` the skill's lock entry already records, so a backing CLI is tamper-evident
  to the same standard as the skill that required it. Deferred behind a real trigger
  (`ARCHITECTURE-v0` §13 vNext); needs the lock schema to carry a binary reference.
- **SLSA/GPG** — not a current github-backend feature and not in v1; revisit with v2.

## 9. Origin template changes

The batteries-included template (`ARCHITECTURE-v0` §2d) gains:
1. **Per-package release-please** config emitting **build-metadata** tags `<semver>+<bin>`
   (e.g. `1.7.0+jira` — **no leading `v`**, build metadata is the **bare tool name**) per CLI
   in `cmd/`, plus **goreleaser** producing `<bin>_<version>_<os>_<arch>.tar.gz` archives and a
   **per-tool** `<bin>_checksums.txt` per release.
2. *(Optional, P1)* a populated **`.skillrig-origin.toml` `[[binaries]]`** block (§5) and the
   `index.yml` workflow regenerating `index.json` (binaries + skills) on merge — for
   metadata-driven resolution. The plugin works without it (convention-driven).
3. Docs: require **mise ≥ 2026.4.12** (Layer A), install the `skillrig` plugin, and set
   `MISE_GITHUB_TOKEN` / `SKILLRIG_ORIGIN`. A worked `mise.toml` example using
   `skillrig:<owner>/<repo>/<bin>` addressing.

> If an adopting org *can* relax its tag policy, the template should note that **prefix
> streams + `version_prefix`** (option (a)) work on stock mise with no plugin — the plugin is
> the answer specifically for the **strict-semver + independent-versioning** org. (The plugin
> supports prefix streams `<bin>-v<core>` too.)

## 10. Alternatives considered

- **(a) Prefix tag streams + native `version_prefix`.** Cleanest natively, but the origin's
  strict-semver tag policy forbids non-semver prefix tags. Viable only if the policy relaxes.
- **(c) One monolithic release with all binaries + `asset_pattern`.** Works on mise 2026.4.12,
  but sacrifices **independent versioning** — every CLI bumps together. Rejected.
- **(d) Separate repo per CLI.** Cleanest for mise, but breaks the **co-location** that lets a
  skill and its CLI ship/verify as one unit (`ARCHITECTURE-v0` §1). Rejected.
- **Native-stanza generator only** (`skillrig add` writes `[tool_alias]`+`asset_pattern`).
  Attractive (no new repo), but inherits Layer B — it cannot generate a working stanza for
  build-metadata streams. Useful as a *complement* for prefix-stream origins, not a substitute.
- **skillrig pulls binaries itself** (`ARCHITECTURE-v0` §13 vNext). Re-absorbs the job given to
  mise (R17) and means owning cross-OS/arch selection + cache. The plugin keeps mise as the
  installer. Rejected for now.

## 11. Open questions

1. **Plugin↔origin metadata fetch:** raw `index.json` over the GitHub API vs. a contents API
   call for `.skillrig-origin.toml`; caching policy within a mise run.
2. **`{os}/{arch}` defaults:** the default RUNTIME→asset token mapping before any
   `[binaries.platforms]` override (e.g. `x64`→`amd64`, `darwin`→`darwin`/`macos`).
3. **Archive shapes:** `strip = "auto"` heuristics for single-binary tarballs vs. nested dirs;
   raw (un-archived) asset support.
4. **mise registry submission:** short name `skillrig` vs. installing by git URL for v1.
5. **convention bump policy:** does the plugin support conventions `N` and `N-1`?
6. **v2 treeSha binding:** lock schema for a binary reference and how `verify`/`doctor` check it.

## 12. Phasing

- **P0 — RFC + spike — ✅ done.** decision recorded; §8b corrected.
- **P2 — plugin v1 — ✅ done (out of order).** [`skillrig/mise-skillrig`](https://github.com/skillrig/mise-skillrig)
  shipped: three hooks; build-metadata + prefix stream resolution; per-tool checksum verify;
  auth; 54 unit tests; CI green ubuntu+macOS; validated against a **real** origin. Built
  **convention-driven**, so it did **not** require P1 first.
- **P1 — origin metadata contract (now an *optional enhancement*, the CLI next step) — ⬜.**
  Tracked on the CLI roadmap (`docs/ROADMAP.md`, v1 "mise backend integration — CLI side"):
  add the convention-versioned **`[[binaries]]`** block to `.skillrig-origin.toml`, **emit it
  into `index.json`** from `skillrig index` (so resolution can be *metadata-driven* instead of
  convention-only), and align the origin template's release pipeline (build-metadata tags +
  per-tool `<bin>_checksums.txt`). The plugin prefers this metadata when present, else falls
  back to the convention. *(in `skillrig/cli` + origin template)*
- **P3 — CLI `add` auto-wiring — ⬜.** `skillrig add` writes `skillrig:<owner>/<repo>/<bin>`
  stanzas into the consumer's `mise.toml` from each skill's `requires`. *(in `skillrig/cli`)*
- **P4 (v2) — ⬜.** treeSha/provenance binding; SLSA; mise registry submission.

## References

See the spike: `specledger/013-mise-backend/research/2026-06-02-mise-backend-plugin.md`
(mise discussions #9074/#8266, PR #9093, mise github-backend / backend-plugin-development /
github-tokens docs, SemVer 2.0.0 build-metadata precedence).
