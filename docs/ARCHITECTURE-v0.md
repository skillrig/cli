# Internal Agent Skills Library — Technical Architecture (v0)

**Status:** Draft for review
**Companion to:** `requirements.md` (the durable, vendor-neutral contract)
**Scope:** v0, single-track. This document proposes *one* concrete realization of the requirements using the org's existing stack (GitHub, Go, mise). These are suggestions and are allowed to flex; the requirements are not.

> Traceability note: requirement IDs (R1, N3, …) from `requirements.md` are cited inline so each technical choice maps back to a need.

---

## 1. Shape at a glance

skillrig is a **framework**: a generic CLI plus a template for standing up a per-org **origin** (a private git repo that is the source of truth for that org's skills). Two separately-distributed pieces:

- the **generic `skillrig` CLI** — one static binary for all orgs, fetched via curl/mise (the entry point for humans, agents, and CI). This is the *only* thing fetched externally.
- the org's **origin repo** — stood up from the skillrig template; a monorepo where **skills and the private CLIs they depend on live together**, built from `cmd/` and released by the same pipeline.

The co-location is the point, not an accident: a skill's `[[requires]]` can name a backing CLI built right there in the same repo, so the skill and its tool version together, release together, and are vendored/verified as one framework-governed unit. skillrig manages that binding end-to-end — `[[requires]]` declares it, the shared pipeline builds it, `skillrig verify` confirms it's present. The CLI carries no baked-in origin; it discovers the origin at runtime from consumer config (§2d). (Realizes R4: same binary everywhere.)

> **Working name:** `skillrig` ("rig up your agents with skills"). Verified clear of dev-ecosystem collisions (distinct from `rig`, the Rust LLM framework, and from `skillctl`/`skillkit`, which are live multi-client *sync* tools occupying a different quadrant). The skill *format* stays agentskills.io-compliant regardless of the tool name, so skills remain portable across all 26+ standard-compliant clients.

Skills live in the origin in-tree. Consuming repos **vendor** a subset (tree + per-skill manifest) and pin them in a committed lockfile. A scheduled CI job proposes upgrades as PRs. Discovery is a generated, committed `index.json`. Public backing-CLI prerequisites are delegated to **mise**; *private* ones can be built and released from the origin itself (alongside the skills that need them). No daemon, no hosted service, no auth of its own. (Realizes N1, R3, R4, R5, R5b.)

The only thing fetched externally is the generic `skillrig` binary. Everything an org owns — skills *and* their backing CLIs — lives together in the origin, created from the template:

```
my-org/my-skills  (the ORIGIN — created from the skillrig origin template)
│                            #   skills + the CLIs they depend on, together,
│                            #   all managed as one skillrig-governed monorepo
├── cmd/                     # the org's private backing CLIs (built here, released here)
│   ├── oxid/                #   e.g. a skill's `requires: oxid` resolves to THIS
│   └── <other-clis>/        #   goreleaser + release-please, same pipeline as skills
├── skills/
│   ├── terraform-plan-review/
│   │   ├── SKILL.md         # agent-facing (vendors to consumer)
│   │   └── skill.toml       # machine-facing manifest; [[requires]] may name a cmd/ CLI
│   └── <more skills>/
├── index.json               # generated discovery artifact (committed); carries
│                            #   skillrig-convention version (the contract, §2d)
├── .release-please-manifest.json
└── release-please-config.json

# Fetched independently — one generic build for all orgs, never lives in the origin:
#   skillrig  (curl/mise) — operates ON the origin above
#   (its own internals — skillcore/tree-SHA/verify, lock/, index/ — live in the skillrig project)
```

(The org's repo is a single Go module, many backing-CLI binaries off `cmd/`, per-binary versioning via release-please — the preferred pattern, shipped by the template. The `skillrig` tool's *own* internals — `skillcore`, `lock`, `index` — live in the skillrig project, not the org's repo.)

---

## 2. The CLI is one tool, four callers

The generic `skillrig` binary is a single static build (goreleaser, cross-OS/arch — N4), the *same binary for every org* (R4) invoked identically by humans, agents, and CI (R3). Proposed command surface for v0:

| Command | Job | Primary caller |
|---|---|---|
| `skillrig search [--tag ...]` | Query `index.json` for skills | human, agent |
| `skillrig add <skill>` | Vendor a skill into this repo + write lock entry | human |
| `skillrig verify` | Offline: integrity + prereqs, exit code | **CI**, agent, human |
| `skillrig bump --pr` | Detect upstream advance, open upgrade PR | **CI (cron)** |
| `skillrig global add/verify <skill>` | Manage global-scope skills | human |
| `skillrig doctor` | Superset health check (integrity + prereqs + auth) | human, agent |

`verify` and `doctor` overlap deliberately: `verify` is the lean, scriptable CI gate (R11); `doctor` is the human-friendly superset that also checks prerequisite auth (R18) and global hints.

**Critical design rule:** `verify`, `bump`, and `doctor` all call the *same* `internal/skillcore` functions for hashing and manifest parsing. There must be exactly one implementation of "compute a skill's tree SHA" so the value CI writes during `bump` is identical to the value `verify` recomputes (R9, R14, N2). Make this a hard internal boundary. *(This thin-interface-over-shared-core rule is independently validated by Skilldex, whose MCP server and CLI both dispatch to the same `core/` functions "so the two interfaces cannot diverge." If you later expose an MCP surface for agents, it dispatches to `skillcore` too — never a parallel implementation.)*

---

## 2b. The CLI is consume-only — GitHub is the authority plane

A clarifying decision that *removes* a whole subsystem: **the CLI has no `publish` verb, and no auth-gated write path at all.** Tools like ClawHub/OpenClaw need `publish`/`login`/`sync`/`delete` because their backend is a separate hosted registry service that doesn't know a skill exists until you upload it. Our backend *is* the monorepo. "Publishing a skill" = opening a PR to `my-org/my-skills`. So the entire administer/publish surface collapses into GitHub primitives we already have:

| Concern | ClawHub/registry model | Our model (GitHub-native) |
|---|---|---|
| Publish a skill | `clawhub publish` (authed upload) | open a PR to the monorepo |
| Versioning / tags | registry semver + `latest` tag | GitHub releases + release-please |
| Approval / who can merge | registry approval workflow | branch protection + CODEOWNERS |
| Per-skill ownership | registry listing owner | CODEOWNERS paths |
| Integrity of a published version | registry-side | **immutable releases** + git tree SHA |
| Auth | `clawhub login`, token in config | `gh auth` / `GITHUB_TOKEN` (already needed for mise, R18) |
| Telemetry | opt-**out** usage snapshot on sync | **none** — telemetry-free by default |

Consequences:
- The CLI's surface is **purely consumer-side**: `search`, `add`, `verify`, `doctor`, `bump --pr`, `global *`, `lint`. Agents and humans share one safe surface; there is no write credential baked into the binary.
- Even `bump --pr` is GitHub-native — it's a *consumer reconciling to upstream* that happens to open a PR; it uses a scoped CI token, not a registry credential. **Convention:** treat `bump --pr` as CI-invoked; a human's agent session generally shouldn't hold PR-create rights (minor, enforce by token scoping, not by code).
- The only "write" the system does to the monorepo is **`index.json` regeneration**, and that's a merge-triggered GitHub Action running `skillrig index`, not a human/agent verb.
- **Differentiator to state explicitly:** no registry service, no telemetry, no bespoke auth flow to build or operate — strictly better on N1 (operational surface) than every registry-backed tool studied, and the no-telemetry default matters for the compliance-conscious consumer.

This is the single biggest surface reduction in the design: an entire publish/auth/registry subsystem replaced by "use git, where the org already has a permission model."

### `skillrig lint` — author-side quality gate at PR time (borrowed from Skilldex)
Add a `skillrig lint` command (author/CI-facing) that runs in the **monorepo's** PR CI, scoring each changed skill for **format conformance**: parseable frontmatter, `skill.toml` validity, allowed subdirectories, description length/specificity heuristics. Rationale (Skilldex's finding): *undertriggering* — an agent failing to invoke a skill when it should — is a documented failure mode driven by vague descriptions, and the objective parts of conformance are deterministically scorable (N6-safe) even though semantic description quality is not. Because publishing = a PR, this is exactly where quality enforcement belongs in the GitHub-authority model: the lint is a required check on PRs to `my-org/my-skills`. Keep objective checks blocking; keep any semantic-quality score advisory.

---

## 2c. The origin and the canon

Two terms, deliberately not "registry":

- **The origin** — the default backend, the org's private monorepo (`my-org/my-skills`). It's literally the git remote skills are fetched from. Trust here is implicit: internal skills are gated by PR review on the monorepo. Most orgs run exactly one origin.
- **The canon** — the origin *with a policy layer enabled*. The policy layer adds (a) an **allowlist of vetted external sources** with graded **allowance levels** (blocked / advisory / approved / pinned-version-only), and (b) optional **advisory trust signals** surfaced from external scanners (e.g. Snyk scores on a public registry). The canon is the governed, authoritative set of *approved* skills — internal plus vetted-external.

**The product promise rides on this distinction.** skillrig does not claim a skill is *safe* (unprovable — an intact skill can still instruct an agent to do harm). It claims: **"the skill your agent runs is exactly the version that was reviewed and approved."** Label honesty (git tree SHA, §4.2) proves *exact content*; the canon's allowlist + PR review prove *reviewed/approved*. Safety enforcement — scanning, human review — lives at the canon (publish/approval side), **never** in the consumer's offline verify path. This is the honest, defensible version of "verify": not "this is safe," but "this is what we vetted."

---

## 2d. Distribution: generic binary + origin template + origin discovery

skillrig is a *framework* any org can adopt, not a single-org tool. Three pieces, deliberately separated:

**1. The generic `skillrig` binary (one build for everyone, R4).** Fetched via curl/mise like `gh` or `terraform`. It is **not** compiled per-org and carries **no baked-in origin** — that was the original single-org design (Model A) and it doesn't generalize. Rejected alternative (Model B, "template generates a per-org forked CLI"): every adopting org would run its own goreleaser/release pipeline to ship a binary whose logic is identical to everyone else's, and would be stranded on whatever version they generated. That duplicates operational surface for zero benefit (violates N1) and breaks central updates. So: one generic binary, centrally maintained; the "feels like our CLI" experience comes from it being *pointed at your origin*, not from being a different binary. (A branded shell alias is fine cosmetics; a forked binary is not.)

**2. The origin template (how you stand up a library, R5d).** skillrig.dev provides a **GitHub template repo** — "Use this template → private repo → rename a few strings → dispatch the first release workflow." It ships the origin's structure (`skills/*/SKILL.md` + `skill.toml`, the `index.json` generation workflow, `lint`/CI checks, CODEOWNERS, branch-protection guidance) **and the Go-monorepo backing-CLI pattern** (`cmd/` building the org's private CLIs via goreleaser + release-please, alongside `skills/`). Standing up a library needs the git host's native features only — no CLI, no service. *(v0 ships one batteries-included template; vNext can add a minimal skills-only variant and other bootstrap paths.)*

**3. The origin as a versioned contract (R5e) — the key insight.** The origin's conventions *are the contract* the generic binary depends on: skill layout, index generation, tag/version scheme, the tree-SHA boundary. The template is the **reference implementation** of that contract. So the contract carries a **convention version** (e.g. a `skillrig-convention: 1` field in the origin's index/config); the binary checks it and fails clearly against an incompatible origin rather than silently misbehaving — the way Terraform pins provider schema versions. This lets the contract evolve deliberately in vNext without breaking deployed consumers, and makes "validating/evolving the origin conventions" a first-class, versioned activity rather than ad-hoc drift.

**Origin discovery (R5c) — git-style precedence.** Since the binary is generic, the origin is resolved at runtime from consumer-controlled config, in order:
1. **`SKILLRIG_ORIGIN` env var** — highest; for CI/ephemeral overrides.
2. **Project config** `.skillrig/config.toml` (`origin = "my-org/my-skills"`) — committed, travels with the repo, authoritative for that repo. This makes the repo self-describing (R25): any clone/agent/CI knows the origin with no extra setup.
3. **Global default** `~/.config/skillrig/config.toml` — the developer's "my org's library" default, used by `skillrig global add` and to pre-fill `init` in new repos.

Lower-priority sources fill in only what higher ones omit (like git local > global > system) — so a contractor can target client-A's origin in one repo and client-B's in another, while keeping a personal global default.

**Origin reference shape — `OWNER/REPO[@REF]`.** The origin reference accepts an optional `@REF` suffix that tracks a **branch** of the library (e.g. `my-org/my-skills@staging`); omitted, it tracks the default branch. It is stored combined in the single `origin` key and validated shape-only/offline. This realizes the `@ref` half of the R26 identity grammar (§9b) for origins specifically — note the *origin's* `@ref` is a moving branch pointer, whereas a *skill's* pin (`add --pin`) is an immutable tag/SHA. The `[/path]` portion of the grammar remains future work.

**Config vs. lock — separate files, co-located dir.** Input and tool-output have different natures and must not share a file (you'd get merge conflicts on hand-edited bits whenever the tool rewrites generated bits — the reason npm splits `package.json` from `package-lock.json`). So:
```
.skillrig/
├── config.toml        # INPUT: origin, client targets, adoption policy. Hand-edited.
└── skills-lock.json   # OUTPUT: pinned skills (version + commit + treeSha). Tool-written, never hand-edited.
```
Named `skills-lock.json` for familiarity with the Vercel lineage, but **structurally ours** — we can't match Vercel's schema because our verification differs (git tree SHA + provenance + `requires`, not their tree-SHA update-tracker). The `.skillrig/` dir gives both files one project-scope home with room to grow (cached index, optional `patches/`).

**`skillrig init` is consumer-side only.** It binds a repo (or the global default) to an *already-existing* origin — `skillrig init --origin my-org/my-skills` writes `.skillrig/config.toml`; `--global` writes the developer default; `--origin` makes it non-interactive. It does **not** bootstrap the origin — that's the template's job (GitHub-native, no CLI). Two clean, separate jobs: *stand up a library* = template; *consume from one* = `skillrig init`.

---

## 3. Two scopes, two jobs (the central asymmetry)

Realizes R6–R8. This is the most important architectural idea and the place most prior art gets muddy.

### Project scope — vendored, verify-only
- Skills are checked into the consumer repo under a canonical dir (see §6).
- A committed **`.skillrig/skills-lock.json`** records, per skill: pinned commit SHA (provenance) + git tree SHA (label honesty) + version + source.
- Because the tree is in git, there is **no "restore from lock"** — git already restored it. `skillrig verify` only *checks* the committed tree against the lock and *checks* prerequisites. (This is precisely the job Vercel's lock conflates; we keep it clean.)
- The project lock contains **zero global entries** (R8).

### Global scope — not vendored, fetch/restore
- Skills installed to the user's per-environment location, not committed anywhere.
- A separate global lock (e.g. `~/.config/skillrig/skills-lock.json`) tracks what the user has, and `skillrig global add` genuinely *fetches and materializes* missing skills (the restore mode that project scope doesn't need).
- A repo may *recommend* global skills via agent-instruction text (§7), never enforce them.

> Why this matters: it means the project lockfile is an **adoption ratchet**, not an integrity gate against corruption. The recorded tree SHA exists so the *bump PR diff is honest and reviewable* (R14) and to verify *label honesty* (content matches its claimed version), not to repair a tree git can already restore.

---

## 4. Lockfile & manifest schemas

### 4.1 `skill.toml` (per-skill manifest — vendors with the skill, R16, R25)

```toml
name        = "terraform-plan-review"
version     = "1.4.0"
namespace   = "my-org"                  # reverse-DNS-ish namespacing
description = "Review a terraform plan for risk and drift."

# Deterministic discovery tags (R24). Suggestion UX is v1; the DATA ships now.
tags = ["platform-team", "terraform", "aws"]

# Backing-CLI prerequisites (R15). Declared, not installed (R17).
[[requires]]
tool       = "oxid"
version    = ">=0.4.0"
source     = "cdktn-io/oxid"            # private repo; mise gh backend
manager    = "mise"                      # supported path; advisory

[[requires]]
tool       = "terraform"
version    = ">=1.6"
source     = "hashicorp/terraform"
manager    = "mise"
```

### 4.2 `.skillrig/skills-lock.json` (project scope — committed, R9–R11)

```jsonc
{
  "lockfileVersion": 1,
  "origin": "my-org/my-skills",       // the default backend (see §2c)
  "skills": {
    "terraform-plan-review": {
      "version": "1.4.0",
      "commit": "9f2c…",            // provenance: exact upstream commit (R10)
      "treeSha": "a83b…",           // label honesty: git tree SHA of the skill subtree (R9)
      "path": ".agents/skills/terraform-plan-review",
      "requires": [                 // mirrored for OFFLINE prereq check (R16)
        { "tool": "oxid", "version": ">=0.4.0", "source": "cdktn-io/oxid" }
      ]
    }
  }
}
```

**Fingerprint primitive — use the git tree SHA, not a bespoke hash (R9).** `treeSha` is the **git tree SHA of the skill subtree** as published at the origin for that version. The job it does is *label honesty*: when a PR vendors content claiming to be `v1.4.0`, `skillrig verify` recomputes the subtree's git tree SHA and compares it to the origin's recorded tree SHA for `v1.4.0` — catching content that was modified in transit but mislabeled as a known version (a thing no human reviewer can eyeball on a long SKILL.md). The `commit` field adds provenance ("exact upstream commit it came from").

Why git tree SHA and not a custom sha256-canonicalization (a correction from an earlier draft):
- The origin **already computes git tree SHAs for free** for every skill folder at every version — no canonicalization scheme to design, maintain, or get subtly wrong (line endings, mode bits, symlink handling all handled by git's object model).
- In a *vendored-into-git* model, integrity of the checked-in files is **already git's job** — local tampering shows up as an ordinary diff. So a bespoke content hash was largely re-deriving a guarantee git provides. The only gap git doesn't close is "does this subtree match the *version it claims to be*," and the git tree SHA answers exactly that.
- Vercel/`gh skill` use tree SHA too — their mistake wasn't the primitive, it was using it for an *online update-check* and lacking reproducible restore. Neither applies here: we use the tree SHA for offline label-honesty at vendoring time.

> Lineage credit: the two-lock split (committed project lock + global lock under `~/.agents/`) follows Vercel's `npx skills` model. We diverge by (a) using the tree SHA for *label honesty at vendoring time* rather than online update-checking, and (b) treating project scope as verify-only since our skills are vendored in git, not gitignored — git itself carries file integrity.

---

## 5. Version adoption — `skillrig bump --pr` (R12–R14)

Logic lives in the CLI (`internal/skillcore` + an `index/` compare); CI is a thin trigger (a cron workflow that runs `skillrig bump --pr`).

Flow:
1. Cron workflow in the consumer repo runs `skillrig bump --pr`.
2. CLI reads `.skillrig/skills-lock.json`, compares each pinned `commit` against the latest at the origin / in `index.json`.
3. For each advanced skill: re-vendor the new tree, record the new `treeSha` + `commit`, update lock.
4. Open a PR with the tree diff + lock diff together. Git's own diff + the recorded `treeSha` make the diff faithful (R14) — no bespoke mechanism needed.
5. Team policy (branch protection / auto-merge label) decides disposition (R13) — the tool proposes, never force-adopts.

This is Renovate/Dependabot semantics for *vendored skill trees*. Consider studying Renovate's custom-manager model as prior art, but the bump logic stays in our CLI so `bump` and `verify` share the same tree-SHA computation (no drift between "how CI computes it" and "how verify checks it").

### 5b. Local modification & conflict handling (R31–R33)
Because skills are vendored **in git**, a team that needs to modify a skill just edits the checked-in files — the change persists in their git history and is reviewed in their PR. There is **no patch-file requirement** the way `patch-package` needs one (patch-package exists only because `node_modules` is gitignored and rebuilt on every install, wiping edits; vendored-in-git has no such reinstall). So the system's only real job is reconciling local edits with upstream updates.

**v0 — drift-aware three-way merge bump.** When `skillrig bump` finds a skill whose vendored subtree no longer matches its pinned `treeSha` (i.e. the team edited it locally), it performs a **git three-way merge**: base = pinned-original tree, theirs = new-upstream tree, ours = locally-modified tree.
- Clean merge → PR as normal, now carrying both the upstream change and the preserved local edits.
- Conflict → **fail with a non-zero exit code, write standard git-style conflict markers into the affected files, and instruct the user to resolve and re-run** (R14b/R32). Don't try to be cleverer than git here; developers already know how to resolve conflict markers. `skillrig verify` recognizes "unresolved conflict markers present" as a distinct failure.
- The system **never silently discards** local modifications (R32).

**v1 (optional) — explicit patch files.** Only if real usage shows teams maintaining *long-lived* divergence against a moving upstream that 3-way merge handles painfully: a `skillrig patch <skill>` command snapshots the diff (pinned → local) into `skills-patches/<skill>.patch`; `bump` then updates the pristine skill and re-applies the patch on top, surfacing reject hunks on failure. This separates "the upstream skill" from "our delta" so each reviews independently (the patch-package value) — but it's added maintenance surface, so defer until proven necessary.

**Always preferred — upstream a flavor (R33).** For durable, generally-useful modifications, the right path is contributing back to the **canon** (a fix PR, or a named variant like `terraform-plan-review-teamx`) rather than carrying per-repo divergence. When `skillrig` detects sustained local drift on bump, it nudges: "this skill has been locally modified across N bumps — consider upstreaming it as a variant."

---

## 6. Multi-client materialization — one truth, many views (R19–R21)

Grounded in the emerging cross-client convention: `.agents/skills` is the shared project-scope directory recognized across Copilot, Cursor, Codex, Gemini CLI, OpenCode, and others; several clients also read `.claude/skills` for compatibility.

**Canonical copy:** vendor the real skill content once into `.agents/skills/<skill>/` (project) — this is the single source on disk (R20).

**Per-client views via symlink:** create `.claude/skills/<skill>` → `../../.agents/skills/<skill>` (and equivalents for other clients that need their own path). One content tree, many directory entries; no duplication (R20). Adding a new client = adding a symlink rule, no re-vendoring (R21).

Global scope mirrors this against home-dir locations (`~/.agents/skills` canonical; `~/.codex/skills`, `~/.copilot/skills`, etc. as views).

**Caveats to design for:**
- **Windows symlinks** need privilege or developer mode; provide a copy-fallback mode for clients/OSes where symlinks are unavailable, with a `doctor` note when copies (not links) are in use so drift is visible.
- Keep a small, declarative **client registry** in the CLI mapping client → path(s) → link-or-copy, so v0 ships the clients you use and new ones are config, not code.
- **Reconsider symlink-default vs. copy-default.** GitHub's first-party `gh skill` made the opposite choice: it **copies (not symlinks)** files across locations and injects tracking metadata into frontmatter, sidestepping the Windows-symlink problem entirely. The cost is content duplication and harder drift-detection (two copies can diverge). Decision for v0: prefer symlink where the OS supports it (single truth, R20), copy-fallback otherwise — but evaluate whether copy-everywhere + per-copy tree-SHA verification is simpler operationally. This is an open question (§12).

---

## 7. The agents.md / claude.md handshake (R8, R22)

Because project skills are **already vendored**, the CLI does **not** rewrite agents.md to manage them. Instead `skillrig` *prints suggested instruction text* the author pastes into the repo's `AGENTS.md` / `CLAUDE.md`. That text's active job is narrow:

- Tell the agent the `skillrig` CLI exists and how to run `skillrig verify`.
- **Remind that recommended *global* skills are not vendored** and should be ensured separately (R8) — this is the one thing the repo genuinely can't guarantee on its own.

No fenced auto-rewrite block in v0 (less surface, N1). The project tree needs no handshake; it's right there.

---

## 8. Backing-CLI provisioning — declare, build-or-delegate, verify (R15–R18)

A skill's required CLI is one of two kinds, and skillrig handles both:

- **Private (org-internal) CLIs — built in the origin itself.** A skill's `[[requires]]` can name a CLI built from the origin's own `cmd/` (e.g. `oxid`), released by the same goreleaser + release-please pipeline as the skills. Skill and tool co-live, version together, and release together — the framework manages the whole unit. mise's gh backend (private-repo releases) is the natural fetch path for these, pointed at the origin's own releases.
- **Public CLIs — delegated to mise.** For widely-used tools (`terraform`, `gh`, …), provisioning is the developer's choice; **mise** is the supported/recommended path (`mise.toml` + shell hook = good onboarding). skillrig never becomes a binary package manager (R17); it declares and verifies, mise installs.

Mechanics:
- **Declare:** `[[requires]]` in `skill.toml` (§4.1), vendored so checks run offline (R16). A `source` of the org's own origin signals a private, co-located CLI; an external source signals a public one.
- **Verify:** `skillrig verify`/`doctor` checks each `requires` entry — on PATH (or resolvable via mise)? version satisfies constraint? — deterministic pass/fail with exit code (R11, R17).
- **Auth as a distinct failure (R18):** for private-repo tools (mise gh backend pulling from the origin or e.g. `cdktn-io/oxid`), `doctor` must distinguish "tool missing" from "tool exists but you can't authenticate to fetch it" — explicitly check `gh auth` / `GITHUB_TOKEN` reachability and report it as its own actionable error. The most common onboarding/CI footgun; surface it loudly.

The CLI can *offer* to write the matching `mise.toml` stanza (helpful), but installation is mise's job, not ours. We ship no Nix package or Homebrew tap in v0; orgs wanting those contribute them.

### 8b. mise GitHub-backend realities (grounded) — and why release-please-per-binary is load-bearing

Verified against mise's GitHub backend docs (current as of early 2026). Three findings shape the template:

**(1) The one-binary-per-entry limit — the constraint that shapes the origin's release strategy.** mise's GitHub backend installs **a single binary per tool entry**; it does **not** natively fetch multiple binaries from one release (confirmed: `mise use github:org/repo` installs only the first asset and skips the rest; the community workaround is a per-tool postinstall `curl` for extra assets). This collides with the naive reading of "one monorepo origin with several backing CLIs in `cmd/`" if that monorepo cuts **one release containing all binaries**. The resolution — and it happens to be the pattern already chosen for versioning hygiene — is **per-CLI tagged release streams via release-please** (`oxid-v1.4.0`, `foo-v2.1.0`, …) rather than one monolithic release. So:
- **release-please-per-binary is not just nice versioning — it is what makes mise consumption work at all** while preserving co-location. State this in the template explicitly.
- Because mise's `github:org/repo` shorthand tracks a repo's *latest release*, a monorepo with interleaved tag streams needs **per-tool tag filtering** (mise's version/tag-regex options) so each backing CLI tracks only its own prefix (`oxid-v*`). This is fiddly enough that **the template should generate the correct `mise.toml` stanza per backing CLI** — a concrete, valuable template job, not something each adopting org should reverse-engineer.
- Rejected alternative: **separate repo per CLI** (`github:my-org/oxid`) is cleanest for mise (each tracks its own latest, no tag-regex), but it **breaks the co-location** that lets a skill and its CLI ship together — so it fights the design. Monorepo-with-tag-streams is the chosen path; see open question on exactly how the template stamps `tag_regex`.

**(2) Auth resolution is well-defined and matches R18.** Token resolution order: `github.credential_command` (highest) → `MISE_GITHUB_TOKEN` env → `github_tokens.toml` → gh CLI `hosts.yml` → git credential fill. The **`credential_command`** hook (runs a shell command per host to fetch a token — 1Password/Vault/secret-manager friendly) is the clean **enterprise auth path**, and it's host-aware for GitHub Enterprise. In CI, `jdx/mise-action` uses `${{ github.token }}` by default, so private-repo fetches in the org's own Actions need no extra plumbing. This is the same git/token auth skillrig already relies on (R5b) — no new credential surface.

**(3) mise can verify what it pulls.** mise supports optional **SLSA provenance verification** and **GPG asset verification** for github-backend tools. This complements skillrig's own posture: skillrig verifies the *skill* (tree SHA + approval), mise can verify the *backing binary* (SLSA/GPG) — layered supply-chain integrity, neither owning the other's job.

> **Net:** the co-located-monorepo origin is viable through mise, but only via per-CLI tagged releases + per-tool tag filtering, with the template generating the config. The "one big release with all binaries" shape does **not** work with mise today.

> **Reference design (from OpenClaw study):** OpenClaw's `openclaw skills list --eligible` is the closest prior art to `skillrig verify`'s prereq check — it filters to skills *actually runnable in the current environment*, treating "missing dependency or auth error" as the disqualifier. Adopt its shape: a skill is "eligible" iff every `[[requires]]` resolves (present + version satisfies) AND any private source is authenticable (R18). `verify` returns the eligible/ineligible partition with per-skill reasons. Study OpenClaw's dependency-declaration schema before finalizing the `[[requires]]` fields.

---

## 9. Discovery — generated `index.json` (R22–R24)

- On release, `internal/index` walks `skills/*/skill.toml` and emits a committed `index.json` at repo root: name, version, description, tags, requires-summary, path.
- `skillrig search [--tag platform-team]` reads `index.json` — deterministic filtering on tags (R24, N6). No standing infrastructure (R23, N1).
- **GH Pages is dropped from v0.** It added a second system for what `index.json` already provides; for a private repo it would also require Enterprise + private-Pages config purely for browse convenience. If a human browse UI is wanted later (D3), point mkdocs + a client-side search index (pagefind/lunr) at the *same* `index.json` — one source of truth, Pages becomes optional sugar.

---

## 9b. External sources, allowlist & audit (R26–R29) — v1+ governance, designed-for in v0

**Identity grammar (R26) — v0 foundation.** Adopt the ecosystem-standard reference grammar `OWNER/REPO[/path]@ref` + skill name. This is the format both Vercel `npx skills` and GitHub `gh skill` use (`gh skill install github/awesome-copilot documentation-writer@v1.2.0`, with nested-path discovery via the `skills/*/SKILL.md` convention). Use it as the **single key** for: lockfile entries (§4.2), `index.json` rows, and allowlist entries. One grammar, three consumers — an allowlist entry is literally "a source-ref pattern that lock entries are permitted to match." *(Implemented so far: the origin reference realizes `OWNER/REPO[@REF]` with `@REF` as a branch pointer — see §2d; `[/path]` and lock/allowlist consumers are still future work.)*

**Allowlist (R27) — policy data in the monorepo.** Author an `allowlist` section consumed by the `index.json` build (or a sibling `policy.toml`), listing permitted external sources at `OWNER/REPO[/path]` granularity, optionally pinned with `@ref`. Because it ships with the org-controlled source of truth, `doctor` evaluates "is this source permitted?" as a **deterministic offline lookup** (N6) — never inference.

**Audit mode (`skillrig doctor --audit`, R28) — v1+.** Walks every skill dir on disk in a consumer repo and classifies each:
- **OK** — tracked in lock, tree SHA matches, source allowlisted.
- **Policy violation** — tracked, but source not on the allowlist.
- **Integrity alarm (orphan)** — present on disk but referenced by *no* lock entry and *no* index entry. This is the higher-severity finding: the primary supply-chain vector flagged in the landscape is an *untracked* skill that social-engineers an agent into running shell commands. Orphan detection is cheap because it's the inverse of R10's "every lock entry has a matching tree" — here, "every on-disk tree has a matching lock entry."

> Sequencing: orphan detection is built directly on the v0 tree-SHA machinery (§4.2), so although the *audit UX* is v1+, v0's `verify` should already refuse to pass if the set of on-disk skills ≠ the set of locked skills. That gives orphan protection essentially for free at the CI gate without the full audit report.

**Risk-signal surfacing (R29) — v1+, advisory only.** Where an external registry exposes a risk/vulnerability score (e.g. Snyk scores on a public registry), `doctor` may fetch and display it at install/audit time. Hard constraints: this is an **external, live** signal, so it is **human-facing and online-permitted only** — it must never enter `verify` (the deterministic CI gate, R11/R5/N1). Any *blocking* on score is explicitly deferred and tier-dependent (D6): the strict/enterprise deployment may opt to gate on it; the lean one won't. Implement surfacing behind an interface so the score *provider* is swappable and absent-by-default offline.

**Supply-chain hardening to recommend (from `gh skill` study).** GitHub supports **immutable releases** — once published, release content can't be altered even by repo admins; pinning to a tag from an immutable release means a later repo compromise can't change what you already pinned. Recommend enabling immutable releases on the origin as a complement to the tree-SHA check: the tree SHA proves *the content matches the version it claims to be*, the immutable release proves *upstream can't have swapped it under a tag you trust*. Also adopt `gh skill`'s publish-time posture (tag protection, secret scanning, code scanning on the skills repo).

**Where the security scan belongs (from AWS Agent Registry + ClawHub study).** Both mature governance tools place risk/security scanning at **publish/approval time, registry-side** — not in the consumer's runtime path. AWS Agent Registry runs scanning in an *admin-controlled CI pipeline* as part of the approval workflow, emitting an audit event per state transition; ClawHub stores a *security scan summary* as registry metadata surfaced at inspect time. This independently confirms the boundary in R29: in our GitHub-authority model, the scan is a **required check on the skill PR** (publish-time, admin-controlled CI), and any score travels into `index.json` as advisory metadata. The consumer's `verify` never runs or depends on the scan — it only reads the deterministic allowlist + git tree SHA. This keeps the CI gate offline and deterministic (R11/N1) while still gating quality/security at the one place writes happen.

**Content-comparison-on-write is the validated UX (from ClawHub study).** ClawHub compares local skill contents to published versions and refuses to overwrite on mismatch (prompting, or requiring `--force` non-interactively). We get the same guarantee more cheaply via the git tree SHA (§4.2) rather than a bespoke content hash — but the "prompt/`--force` on mismatch" UX is worth mirroring in `add`/`update`.

---

## 10. What we deliberately did *not* build (maps to requirements §5)

- **Team→skill suggestion engine (D1):** tags ship in the manifest now (R24); the suggestion UX is v1. Any future suggestion layer reads tags deterministically and stays additive — truth never moves into an LLM/inferential component (N6). `doctor` can already deterministically list "skills tagged `<your-team>` not present in your global scope" without any inference — that may be enough to make the v1 "engine" unnecessary.
- **Onboarding wizard (D2):** docs + a PR template only. `skillrig init` can come later once the team→skills mapping has earned its shape from real use.
- **Browse UI (D3):** see §9.
- **Client tiers (D4):** single-track. If the strict-vs-lean split later matters, it's a *deployment* concern (e.g. private-Pages on/off, auth strictness in `doctor`, auto-merge policy) layered on this same architecture — not a change to the requirements.

---

## 11. Prior-art positioning (why build vs. adopt)

Each entry: what it is, what it solves for our contract, where it fails, and the transferable nugget. Researched entries are marked ✅; landscape-only are marked ◻️.

**✅ GitHub `gh skill` (Go, first-party, in-stack).** Thin file-placement binary. **Solves:** scope split (repo-scope = our project scope; user-scope = our global scope); multi-client placement (natively shares `.agents/skills` across Copilot/Cursor/Codex/Gemini/Antigravity/Amp/Cline/OpenCode/Warp, install-once per shared dest); portable provenance (repo/ref/tree-SHA in SKILL.md frontmatter); pinning (`@tag`/`@sha`/`--pin`). **Fails our contract:** integrity model is *tree-SHA change-detection used online* — right primitive, wrong question (it checks "did upstream move?" not offline label-honesty); and it stores provenance in frontmatter not a lockfile; **zero** backing-CLI prereq concept (R15–R18); no reviewable-PR bump (R12); discovery leans on GitHub topic conventions, not a fully org-controlled artifact. **Caveat:** public preview, "subject to change without notice." **Nugget:** the client-path matrix (borrow as data) + immutable-release hardening. Drives the §11b wrap-vs-reimplement fork.

**✅ ToolHive (`thv`, Go) — REFRAMED.** *Not* "the closest tool" as first grounded — it's an **MCP-server governance platform** (Registry + Runtime-on-Kubernetes + Gateway + Portal) that recently grew a skills feature. **Structural disqualifier:** daemon-based — the ToolHive API server must be running for every `thv skill` command (a separate `thv serve` process or the desktop app), which conflicts with N1 (no long-running service) and R4 (just a binary), and is especially bad for the CI/agent callers who'd have to stand up the daemon first. **Solves:** scope split (R6/7); multi-client auto-placement (R19–21); the `git://host/owner/repo[@ref][#path]` grammar (confirms R26 is an ecosystem convention); and notably **real content-addressed integrity via OCI digests** (identifier + digest + media type) — the integrity guarantee `gh skill`/Vercel lack. **Fails:** N1/R4 (daemon); no backing-CLI prereq concept (R15–R18); OCI/registry machinery is heavier than vendored-git wants. **Nuggets:** (1) OCI digests *validate that content-addressed identity is useful* — we achieve a sufficient git-native equivalent via the git tree SHA (§4.2), avoiding the registry/daemon dependency; (2) its **per-catalog access-control + audit-trail** model is a maturity reference for our v1 governance (R27–R29).

**✅ OpenClaw `openclaw skills` + dependency system.** The **only** ecosystem studied that solves our backing-CLI requirement. **Solves (uniquely):** skills *declare external dependencies* (e.g. `gh`, `curl`); a dependency-install flow (`installSkillDependency`) and — most relevant — `openclaw skills list --eligible` filters to skills *actually runnable in the current environment*, with success defined to include "no missing dependency or auth error." That `--eligible` semantic is precisely our `skillrig verify` prereq check incl. the auth case (R15–R18). **Also solves:** strict realpath-containment on skill discovery (only roots whose resolved realpath stays inside the configured root) — a concrete defense for our orphan/symlink-escape vector (R28); a built-in dangerous-code scanner that blocks installs on critical findings unless overridden; per-agent allowlists that *replace*, not merge with, defaults (R27). **Fails our contract:** TS/Node, not a single binary (R4); registry-backed install flow assumes ClawHub connectivity for parts. **Caveat:** public registry has had security incidents — reinforces our private-first + allowlist posture. **Nugget:** study its dependency-declaration schema + `--eligible` as the reference design for `skill.toml [[requires]]` + `verify` (§4.1, §8).

**✅ OpenClaw / ClawHub split (the Unix-philosophy question).** Confirmed: native `openclaw skills` handles day-to-day discover/install/update (embedded in the agent), while a **standalone `clawhub` CLI** owns registry-authenticated publish/sync/version/CI workflows — both hitting the same registry data. This is exactly the single-responsibility split worth mirroring: a consumption surface vs. a dedicated publish/CI binary. Validates our "root CLI + per-skill/publish CLIs off `cmd/`" instinct. *(Deeper ClawHub research pending — auth model, sync semantics, CI ergonomics.)*

**◻️ Vercel `npx skills` (lock model).** Source of our two-lock lineage (global `~/.agents/` + committed project lock) and the symlink-vs-shared-dir client pattern. But the lock is an *update-tracker* (tree-SHA change-detection), not a reproducible/offline-verifiable manifest, and it depends on a hosted check-updates endpoint (fails R5, R10). We keep the good ideas, fix the integrity model. *(Deeper research pending if selected.)*

**◻️ Vercel `find-skills` (semantic discovery).** Vector/intent-based skill search (used even inside OpenClaw). Relevant to R22 discovery and the deferred v1 suggestion question — but it's *inference-based* search, so per N6 it can only ever be an additive layer over our deterministic `index.json`, never the source of truth. *(Research pending.)*

**◻️ Skilldex (`spm`).** Academic; contributes the scope-hierarchy and "format-conformance scoring" idea (a future `skillrig lint` for authors could adopt the latter) but is npm-distributed TS (fails R4). *(Landscape.)*

**◻️ AWS Agent Registry (Bedrock AgentCore).** Governed private catalog (IAM/OAuth) — relevant given the AWS-all-in client, but heavier and more cloud-coupled than N1 wants for the vendor-neutral v0. *(Research next.)*

**◻️ TrueFoundry Skills Registry.** Commercial versioned-artifact registry; governance reference only, closed-source so internals opaque. *(Landscape.)*

Net: no existing tool satisfies "private-first, single static binary, git-native vendored skills, offline label-honesty + provenance, declared+verified backing-CLI prereqs, no daemon, multi-client" simultaneously. The closest *conceptual* pieces are scattered: `gh skill` (placement), ToolHive (governance + integrity-via-OCI), OpenClaw (prereq/eligibility). The gap is real; this architecture assembles the scattered pieces git-native with minimal surface.

### 11b. The `gh skill` fork — wrap vs. reimplement (decide before building §6)

Because `gh skill` solves your placement/scope/provenance layer first-party, there's a real build-vs-adopt fork:

- **Option A — Wrap.** `skills` shells out to `gh skill` for install/placement (the multiplexing in §6) and adds only the layers `gh skill` lacks: tree-SHA label-honesty (§4.2), backing-CLI prereq manifest + verify (§8), reviewable-PR bump (§5), and audit (§9b). **Pro:** drastically smaller build surface; you inherit GitHub's multi-client matrix and immutable-release hardening for free. **Con:** couples you to a GitHub *preview* tool ("subject to change without notice"), requires `gh` + auth everywhere (CI included), and `gh skill`'s frontmatter-provenance model partly overlaps/conflicts with your separate-lockfile model — you'd be reconciling two provenance stores.
- **Option B — Reimplement.** Own the placement layer in your CLI, learn from `gh skill`'s directory matrix but don't depend on it. **Pro:** no preview-tool coupling, single provenance store (your lockfile), full control of copy-vs-symlink. **Con:** you re-build and must maintain the client-path matrix as clients evolve.

Recommendation to pressure-test: **Option B for the core, borrow `gh skill`'s client-path matrix as data.** The coupling risk of depending on a preview tool for your load-bearing placement layer probably outweighs the build savings — but if `gh skill` exits preview and stabilizes, revisit. Either way, adopt its supply-chain posture (immutable releases, tag protection).

---

## 12. Open technical questions (for the eval round / v0.1)

1. ~~Canonical content-hash format~~ — **resolved**: use the git tree SHA (§4.2), no bespoke canonicalization needed. Removed.
2. Symlink fallback policy on Windows/CI: copy-mode default detection rules.
3. `mise.toml` stanza generation: write it, or only print it? (Leaning: offer, don't impose.)
4. Where exactly the global lock lives across clients (`~/.agents/` canonical vs. per-client) and how `global verify` reconciles multiple client home dirs.
5. Whether `skillrig verify` should hard-fail or warn on a *prerequisite* miss vs. a *label-honesty* miss (likely: label-honesty mismatch = fail; prereq = fail in CI / warn for humans; unresolved merge conflict markers = fail).
6. **Wrap vs. reimplement `gh skill`** (§11b) — the biggest architectural fork; decide before building §6.
7. **Provenance store reconciliation** — if wrapping `gh skill`, how to avoid two provenance stores (its frontmatter metadata vs. your lockfile). Likely: lockfile is canonical, frontmatter ignored/stripped.
8. **Allowlist authoring location** — `allowlist` block inside the `index.json` build inputs vs. a standalone `policy.toml`; and whether the allowlist is global-only or also per-consumer-repo (a repo tightening the org default).
9. **Risk-signal provider interface** — what the v1 advisory score source is (Snyk via the public registry? other?), and how `doctor` degrades to silent when offline.
10. **Lockfile write atomicity** (from Skilldex caution) — write to temp file + rename; consider file locking for the CI-bump-vs-human-edit race on the same lock.
11. **Skillset / bundle grouping** (from Skilldex) — should `skill.toml` support grouping a skill with its backing CLI + shared assets (vocab/templates/reference docs) as one coherently-versioned, co-vendored unit? Relevant to skills whose behavior depends on shared context. Likely v1.
12. **Third scope tier?** (from Skilldex's global/shared/project) — is a "shared" tier (team-wide, multi-repo, not user-global) needed, or do project + global suffice for v0? (Leaning: two tiers for v0.)
13. **`bump --pr` invocation policy** — enforce CI-only by token scoping so human agent sessions don't carry PR-create rights.
14. **Origin convention versioning** (§2d, R5e) — where the `skillrig-convention` version lives (in `index.json`? a top-level `.skillrig-origin.toml`?), the compatibility policy (binary supports conventions N and N-1?), and how the template's release workflow stamps it. The contract surface is small now; pin it before the first external org adopts.
15. **Template `mise.toml` generation for co-located CLIs** (§8b) — exactly how the template stamps per-tool `tag_regex`/version filters so each backing CLI in the monorepo tracks its own release stream, and whether to generate one shared `mise.toml` or per-skill stanzas keyed off each skill's `[[requires]]`.

---

## 13. Roadmap (v0 / v1 / vNext)

Consolidates the phasing scattered through the doc. **v0** is the minimum coherent framework; **v1** adds governance + ergonomics once real usage justifies them; **vNext** is candidates that need an explicit trigger before being pulled forward. The discipline: nothing moves earlier than its trigger, and nothing in a later phase is allowed to move *truth* into a non-deterministic component (N6).

### v0 — the minimum coherent framework
The smallest thing that delivers the core promise ("the skill your agent runs is the version you approved") end-to-end.
- Generic `skillrig` binary; consumer command surface: `search`, `add`, `verify`, `doctor`, `bump --pr`, `global *`, `lint`, `init` (§2).
- Two scopes: project (vendored, verify-only) + global (fetch/restore) (§3). **Two tiers only** — no "shared" middle tier.
- Lockfile with `commit` (provenance) + `treeSha` (label honesty) + `requires` (§4.2); `.skillrig/config.toml` (input) + `.skillrig/skills-lock.json` (output) (§2d).
- Origin discovery via env > project config > global default; origin = git, **no auth of its own** (§2d).
- One **batteries-included GitHub template** (skills + Go-monorepo backing-CLI pattern + index/lint/release workflows) (§2d).
- Backing-CLI declare + verify (`[[requires]]`, `--eligible`-style readiness, auth-as-distinct-failure) (§8); mise consumption via **per-CLI tagged releases + template-generated `mise.toml`** (§8b).
- Multi-client materialization: canonical `.agents/skills` + symlink views, copy-fallback (§6).
- Discovery via committed `index.json` (§9); **deterministic tags ship in the manifest** (data only).
- Drift-aware **three-way-merge bump** with conflict-markers-and-error (§5b).
- `lint` as a required PR check on the origin (§2b).
- Orphan protection effectively free at the `verify` gate (on-disk set must equal locked set) (§9b).
- Supply-chain posture: recommend immutable releases + tag protection on the origin (§9b).

### v1 — governance + ergonomics (once usage justifies)
- **Audit mode** `doctor --audit`: classify on-disk skills as OK / policy-violation / orphan (§9b).
- **External-source allowlist** with graded allowance levels, enforced in `doctor` (R27, §9b).
- **Risk-signal surfacing** (e.g. Snyk) — advisory, human-facing, online-only, behind a swappable provider interface; never in `verify` (R29, §9b).
- **Team→skill suggestion UX** over the deterministic tags — additive only; may prove unnecessary if `doctor`'s deterministic "tagged-for-your-team but missing" listing suffices (D1, §10).
- **Explicit patch files** (`skillrig patch`) — *only if* long-lived local divergence proves painful under 3-way merge (§5b).
- **Minimal (skills-only) template variant** alongside the batteries-included one (§2d).
- **Skillset / bundle grouping** — co-version a skill with its backing CLI + shared assets as one unit (open Q11).
- Possible **human browse UI** (mkdocs/pagefind over the same `index.json`) (D3, §9).

### vNext — candidates needing a trigger
Each only justified if its trigger fires; recorded so they aren't silently assumed.
- **skillrig pulls backing binaries itself (skills + CLIs in one fetch).** *Trigger:* mise's one-binary-per-release limit + `tag_regex` fiddliness (§8b) proves painful enough that skillrig orchestrating a co-located skill+CLI fetch is worth owning. *Tension:* this partially re-absorbs the binary-provisioning job deliberately delegated to mise (R17) — so it needs a real pain signal, not just "it'd be neat." Would also mean owning cross-OS/arch asset selection, checksum/SLSA verification of binaries, and a cache — non-trivial surface.
- **Convention contract v2+** — evolving the origin contract (§2d, R5e) as conventions change; requires the v0 convention-version mechanism to already be in place.
- **MCP surface for agents** — expose `verify`/`search` as MCP tools; must dispatch to the same `skillcore` (§2), never a parallel implementation.
- **Client-tier differentiation** (strict/enterprise vs. lean) — a *deployment* concern layered on the same architecture (private-Pages on/off, `doctor` auth strictness, auto-merge policy, risk-score hard-gating per D6), not a requirements change (D4, §10).
- **"Shared" scope tier** — team-wide, multi-repo skills between project and global. *Trigger:* multi-project teams hit real pain with only two tiers (open Q12).
- **Additional origin bootstrap paths** beyond the GitHub template (§2d).
- **Wrap `gh skill`** for placement — *only* if `gh skill` exits preview and stabilizes, flipping the §11b decision; otherwise reimplement-core stands.
