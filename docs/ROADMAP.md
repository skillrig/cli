# skillrig Roadmap

Consolidated from the technical architecture (`architecture.md §13`). **v0** is the
minimum coherent framework that delivers the core promise — *"the skill your agent
runs is exactly the version that was reviewed and approved"* — end-to-end. **v1**
adds governance and ergonomics once real usage justifies them. **vNext** holds
candidates that need an explicit trigger before being pulled forward.

**The discipline:** nothing moves earlier than its trigger, and nothing in a later
phase is allowed to move *truth* into a non-deterministic / inferential component
(requirement N6). Each feature ships as a single spec branch through
Specify → Clarify → Plan → Tasks → Review → Implement, with quickstart scenarios as
the acceptance contract (Constitution II) and a real-output ground-truth fixture per
integration boundary (Constitution III).

Section references (`§…`, `R…`, `N…`) point into the companion `architecture.md` /
`requirements.md`. Binding CLI behavior lives in [`docs/design/cli.md`](docs/design/cli.md).

Status legend: ✅ done · 🚧 in progress · ⬜ planned

---

## v0 — the minimum coherent framework

The smallest thing that delivers the core promise end-to-end. Suggested branch
sequencing builds the single shared primitives first (one `skillcore`, one origin
resolver — AP-04 / AP-06) and layers thin commands on top.

| # | Feature branch | Pattern | Depends on | Status |
|---|----------------|---------|------------|--------|
| 001 | **`init` + origin resolution** — `env SKILLRIG_ORIGIN > .skillrig/config.toml > ~/.config/skillrig/config.toml`; `skillrig init [--origin] [--global]` binds an existing origin (never bootstraps) | Environment | — (project skeleton) | ✅ |
| 002 | **`skillcore` + `add` (local) + `verify`** — git tree-SHA + manifest parse; **local-origin** `add` (vendor subtree + lock; `--dry-run`/`--force`); offline label-honesty + orphan check; **exit codes 0/1/2** (exit 3 → `doctor`/005) | Vendor Mutation + Verification Gate | 001 | ✅ |
| 003 | **Discover & Acquire — `search` + remote `add` + `index`** (the 003+004 merge — see note below) — `search` reads the origin's committed `index.json` (query-first token-AND over name/description/topics + `--topic` filter, deterministic order, Two-Level Output); **remote `add`** fetches the skill subtree from a GitHub-hosted origin (sparse `git` clone + token via `os.exec`) with `--pin <ref>` immutable pins (local-origin `add` shipped in 002 — `add` now serves both forms); **`index`** is the origin-side generator that produces the catalog from each skill's `SKILL.md` frontmatter. Commit 1 migrates the manifest from `skill.toml` → `SKILL.md` frontmatter (`gopkg.in/yaml.v3`). | Query + Vendor Mutation + origin-side generator | 002 | 🚧 |
| 005 | **backing-CLI prereqs** — `[[requires]]` declare + verify (`--eligible`-style readiness, auth-as-distinct-failure R18); mise consumption via per-CLI tagged releases + template-generated `mise.toml` | (extends verify/doctor) | 002 | ⬜ |
| 006 | **`doctor`** — superset health check (integrity + prereqs + auth) | Environment | 002, 005 | ⬜ |
| 007 | **`bump --pr`** — detect upstream advance, drift-aware three-way-merge, open reviewable PR (conflict markers + non-zero exit on conflict) | Vendor Mutation | 002, 004 | ⬜ |
| 008 | **`global add` / `global verify`** — fetch/restore user-scope skills against the global lock | Global Management | 002 | ⬜ |
| 009 | **multi-client materialization** — canonical `.agents/skills` + symlink views, copy-fallback (Windows/CI) | (supports add/global) | 004 | ⬜ |
| 010 | **`lint`** — author-side conformance gate, required PR check on the origin | Verification Gate | 002 | ⬜ |
| 011 | **`skills.sh`** — support Vercel's skill.sh hosted skills. External skill adoption workflow (federated skill registries, whitelisted in origin, origin policy provisions for approval/review (skills.sh are evaluated on their usage statistics and audit reports, they should vetted or flagged with warnings)) | Evolution | 002 | ⬜ |
| 012 | **`aws`** — ENTERPRISE - support Private AWS AgentRegistry hosted skills | Evolution | 002 | ⬜ |

> **003 + 004 merged into one slice (decided 2026-05-31, FR-024).** The original roadmap split `search` (003) from remote-`add` fetch (004). They ship together because they share the same new machinery — the remote-fetch layer, the auth/token resolver, and (critically) the **catalog**: `search` is meaningless against a catalog that nothing generates, and the shipped `build-index.sh` provably drifts (it drops `topics`/`requires`). So the merged slice also pulls in the origin-side **`index`** generator (reversing the original "consume-only, roadmap a generator" lean — `index` reuses the consumer's `ParseManifest`, so it's a thin walk+marshal, AP-04 by construction) and the **manifest migration** to `SKILL.md` frontmatter. The slice keeps the branch id **003**; there is no separate 004 — the numbers below (005–012) are unchanged.
>
> **Local-vs-remote reframe.** 002's `add` overloaded `OWNER/REPO` as a *local directory* (`<consumerRepoRoot>/OWNER/REPO`) and ran `git -C` on that working tree. 003 splits the two origin forms cleanly: a **path-shaped** origin operates on a local checkout (002 behavior, generalized to a real filesystem path), while a bare **`OWNER/REPO`** is fetched remotely. There is no "both present" precedence — the tool never creates or caches a local copy of a remote origin.

**Cross-cutting v0 commitments** (architecture §13):
- Two scopes only — project (vendored, verify-only) + global (fetch/restore). **No "shared" middle tier.**
- The per-skill **manifest is `SKILL.md` frontmatter** (agentskills.io standard fields + skillrig extensions under `metadata.x-skillrig.*`; `skill.toml` is dropped as of 003). The frontmatter is the single field-source for both the catalog (`index`) and `add`/`verify` (§4.1).
- Lockfile carries `commit` (provenance) + `treeSha` (label honesty) + the resolved human-readable `version`/tag; the per-skill **manifest** (not the lock) carries `requires` — the vendored manifest is the single source of truth (002 D4; reconciles §4.2). `.skillrig/config.toml` (input) split from `.skillrig/skills-lock.json` (output) (§2d).
- Origin = git; **no auth of its own** for the origin contract — a private origin is fetched with a **read-only** token resolved via `os.exec` (`GH_TOKEN` > `GITHUB_TOKEN` > `gh auth token`); still no write credential in the binary (§2d, §8b.2).
- One **batteries-included GitHub template** (skills + Go-monorepo backing-CLI pattern + index/lint/release workflows) (§2d).
- Discovery via committed `index.json`, **generated by `skillrig index` on merge to `main`** (single-tip, full-regenerate — not "on release"); **deterministic `topics` ship in the manifest** (data only; `topics` renamed from `tags`) (§9).
- Orphan protection effectively free at the `verify` gate — on-disk set must equal locked set (§9b).
- Supply-chain posture: recommend immutable releases + tag protection on the origin (§9b).

---

## v1 — governance + ergonomics (once usage justifies)

- **mise backend integration — CLI side (RFC 0001 P1 + P3).** The [`skillrig/mise-skillrig`](https://github.com/skillrig/mise-skillrig) backend plugin now ships and consumes origins **convention-driven** (it derives stream/asset/checksum names from the goreleaser convention `<bin>_<ver>_<os>_<arch>.tar.gz` + `<bin>_checksums.txt` and reads the binary from each tag's build metadata) — so it needs **no** origin-side schema to work. This roadmap item makes that resolution **metadata-driven and auto-wired** from `skillrig/cli`:
  - **P1 — origin metadata contract (optional override, not a prerequisite).** Add a convention-versioned **`[[binaries]]`** block to `.skillrig-origin.toml` and **emit it into `index.json`** from `skillrig index` (stream selector + asset template + checksum filename + per-os/arch platform map). The plugin prefers this metadata when present and falls back to the convention otherwise. One contract, two readers (plugin + CLI) — AP-04.
  - **P3 — `skillrig add` auto-wiring.** When `add` vendors a skill whose `metadata.x-skillrig.requires` names a binary sourced from the origin, write the matching `skillrig:<owner>/<repo>/<bin>` entry (+ version constraint) into the consumer's `mise.toml`, under the same dry-run/force discipline as `add`. Replaces the rejected "template stamps per-tool `tag_regex`" idea (§8b open-Q15 — `tag_regex` does not exist).
  - Requires **mise ≥ 2026.4.12** (the install-scheduler fix, PR #9093) on consumers. See [`docs/rfcs/0001-mise-skillrig-backend.md`](rfcs/0001-mise-skillrig-backend.md) §5/§7/§12 and the spike under `specledger/013-mise-backend/`.
- **Audit mode** `doctor --audit`: classify on-disk skills as OK / policy-violation / orphan (§9b).
- **External-source allowlist** with graded allowance levels (blocked / advisory / approved / pinned-only), enforced in `doctor` as a deterministic offline lookup (R27, §9b).
- **Risk-signal surfacing** (e.g. Snyk) — advisory, human-facing, online-only, behind a swappable provider interface; **never** in `verify` (R29, §9b).
- **Team→skill suggestion UX** over the deterministic tags — additive only; may prove unnecessary if `doctor`'s "tagged-for-your-team but missing" listing suffices (D1, §10).
- **Explicit patch files** (`skillrig patch`) — *only if* long-lived local divergence proves painful under three-way merge (§5b).
- **Minimal (skills-only) template variant** alongside the batteries-included one (§2d).
- **Skillset / bundle grouping** — co-version a skill with its backing CLI + shared assets as one unit (open Q11).
- Possible **human browse UI** (mkdocs/pagefind over the same `index.json`) (D3, §9).

---

## vNext — candidates needing a trigger

Each is justified only if its trigger fires; recorded here so they aren't silently assumed.

- **skillrig pulls backing binaries itself (skills + CLIs in one fetch).** *Trigger:* the mise-delegated path proves painful enough to own. *Status update:* the multi-binary-from-one-monorepo gap is now solved **out-of-process** by the **[`skillrig/mise-skillrig`](https://github.com/skillrig/mise-skillrig) backend plugin** (built & validated; RFC 0001), so this vNext item's trigger has **not** fired — mise (via the plugin) still installs. *Tension (unchanged):* owning the fetch re-absorbs the job delegated to mise (R17) — needs a real pain signal, plus cross-OS/arch asset selection, checksum/SLSA verification, and a cache.
- **Convention contract v2+** — evolving the origin contract (§2d, R5e); requires the v0 convention-version mechanism already in place.
- **MCP surface for agents** — expose `verify`/`search` as MCP tools; MUST dispatch to the same `skillcore` (§2), never a parallel implementation.
- **Client-tier differentiation** (strict/enterprise vs. lean) — a *deployment* concern layered on the same architecture (private-Pages on/off, `doctor` auth strictness, auto-merge policy, risk-score hard-gating per D6), not a requirements change (D4, §10).
- **"Shared" scope tier** — team-wide, multi-repo skills between project and global. *Trigger:* multi-project teams hit real pain with only two tiers (open Q12).
- **Additional origin bootstrap paths** beyond the GitHub template (§2d).
- **SSH-key origins (`ssh://` / `git@…`).** Today every origin is fetched over **HTTPS** with a read-only token (`GH_TOKEN` > `GITHUB_TOKEN` > `gh auth token`); `CloneURL` is hardcoded to `https://github.com/OWNER/REPO.git` and there is no SSH transport. *Trigger:* users/orgs that standardize on SSH deploy keys and have **no token available locally** — the token-only model assumes a completed `gh auth login` (or a CI secret), so a "keys-but-no-token" user has no path today. *Adds:* `ssh://`/`git@` origin parsing + `CloneURL`, an SSH-key auth path (no `http.extraHeader` injection), and an SSH leg in the true-auth e2e (a `gliderlabs/ssh` test server — a new test dependency). *Tension:* a second auth mechanism + transport to maintain; **HTTPS-token stays the primary, CI-friendly, and only tested path** until this lands.
- **GitHub Enterprise / non-`github.com` hosts.** `defaultGitHubHost` is a const seam fixed to `github.com`; GHE support threads a configurable host through origin resolution, `CloneURL`, and the token resolver's `--hostname`. *Trigger:* a GHE-hosted origin.
- **Wrap `gh skill`** for placement — *only* if `gh skill` exits preview and stabilizes, flipping the §11b decision; otherwise reimplement-core stands.

---

## Explicitly *not* built (maps to requirements §5 / architecture §10)

- **Team→skill suggestion engine (D1):** `topics` ship now (R24); suggestion UX is v1, additive, deterministic-only.
- **Onboarding wizard (D2):** docs + PR template only.
- **Browse UI (D3):** deferred to v1 over the same `index.json`.
- **Client tiers (D4):** single-track v0; a deployment concern if it ever matters.
- **No registry service, no telemetry, no bespoke auth** — GitHub is the authority plane (§2b).
