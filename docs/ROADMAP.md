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
| 001 | **`init` + origin resolution** — `env SKILLRIG_ORIGIN > .skillrig/config.toml > ~/.config/skillrig/config.toml`; `skillrig init [--origin] [--global]` binds an existing origin (never bootstraps) | Environment | — (project skeleton) | 🚧 |
| 002 | **`skillcore` + `verify`** — git tree-SHA + `skill.toml` manifest parse; offline label-honesty + orphan check; exit codes 0/2/3 | Verification Gate | 001 | ⬜ |
| 003 | **`search`** — read origin (branch aware) committed `index.json`, deterministic tag filter, Two-Level Output | Query | 001 | ⬜ |
| 004 | **`add`** — vendor a skill subtree + write lock entry; `--dry-run`, refuse-clobber-without-`--force` | Vendor Mutation | 002 | ⬜ |
| 005 | **backing-CLI prereqs** — `[[requires]]` declare + verify (`--eligible`-style readiness, auth-as-distinct-failure R18); mise consumption via per-CLI tagged releases + template-generated `mise.toml` | (extends verify/doctor) | 002 | ⬜ |
| 006 | **`doctor`** — superset health check (integrity + prereqs + auth) | Environment | 002, 005 | ⬜ |
| 007 | **`bump --pr`** — detect upstream advance, drift-aware three-way-merge, open reviewable PR (conflict markers + non-zero exit on conflict) | Vendor Mutation | 002, 004 | ⬜ |
| 008 | **`global add` / `global verify`** — fetch/restore user-scope skills against the global lock | Global Management | 002 | ⬜ |
| 009 | **multi-client materialization** — canonical `.agents/skills` + symlink views, copy-fallback (Windows/CI) | (supports add/global) | 004 | ⬜ |
| 010 | **`lint`** — author-side conformance gate, required PR check on the origin | Verification Gate | 002 | ⬜ |
| 011 | **`aws`** — support AWS AgentRegistry hosted skills | Evolution | 002 | ⬜ |

**Cross-cutting v0 commitments** (architecture §13):
- Two scopes only — project (vendored, verify-only) + global (fetch/restore). **No "shared" middle tier.**
- Lockfile carries `commit` (provenance) + `treeSha` (label honesty) + `requires` (§4.2); `.skillrig/config.toml` (input) split from `.skillrig/skills-lock.json` (output) (§2d).
- Origin = git; **no auth of its own** (§2d).
- One **batteries-included GitHub template** (skills + Go-monorepo backing-CLI pattern + index/lint/release workflows) (§2d).
- Discovery via committed `index.json`; **deterministic tags ship in the manifest** (data only) (§9).
- Orphan protection effectively free at the `verify` gate — on-disk set must equal locked set (§9b).
- Supply-chain posture: recommend immutable releases + tag protection on the origin (§9b).

---

## v1 — governance + ergonomics (once usage justifies)

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

- **skillrig pulls backing binaries itself (skills + CLIs in one fetch).** *Trigger:* mise's one-binary-per-release limit + `tag_regex` fiddliness (§8b) proves painful enough to own. *Tension:* re-absorbs the binary-provisioning job deliberately delegated to mise (R17) — needs a real pain signal, plus cross-OS/arch asset selection, checksum/SLSA verification, and a cache.
- **Convention contract v2+** — evolving the origin contract (§2d, R5e); requires the v0 convention-version mechanism already in place.
- **MCP surface for agents** — expose `verify`/`search` as MCP tools; MUST dispatch to the same `skillcore` (§2), never a parallel implementation.
- **Client-tier differentiation** (strict/enterprise vs. lean) — a *deployment* concern layered on the same architecture (private-Pages on/off, `doctor` auth strictness, auto-merge policy, risk-score hard-gating per D6), not a requirements change (D4, §10).
- **"Shared" scope tier** — team-wide, multi-repo skills between project and global. *Trigger:* multi-project teams hit real pain with only two tiers (open Q12).
- **Additional origin bootstrap paths** beyond the GitHub template (§2d).
- **Wrap `gh skill`** for placement — *only* if `gh skill` exits preview and stabilizes, flipping the §11b decision; otherwise reimplement-core stands.

---

## Explicitly *not* built (maps to requirements §5 / architecture §10)

- **Team→skill suggestion engine (D1):** tags ship now (R24); suggestion UX is v1, additive, deterministic-only.
- **Onboarding wizard (D2):** docs + PR template only.
- **Browse UI (D3):** deferred to v1 over the same `index.json`.
- **Client tiers (D4):** single-track v0; a deployment concern if it ever matters.
- **No registry service, no telemetry, no bespoke auth** — GitHub is the authority plane (§2b).
