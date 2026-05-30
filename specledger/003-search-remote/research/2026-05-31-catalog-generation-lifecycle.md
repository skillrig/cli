# Research: Catalog Generation & Lifecycle — who builds `index.json`, single-tip vs cross-ref, and is `skillrig index` in 003?

**Date**: 2026-05-31
**Context**: Spike S2 for `003-search-remote` (spec-tech.md §8b). `search` reads a catalog (`index.json`) at the origin; this spike fixes the catalog's *data model* (single-tip vs cross-ref aggregated, which versions/fields), its *regeneration/GC policy*, and the *scope decision* for the origin-side generator `skillrig index`. Upstream S1 (`2026-05-31-skill-manifest-format.md`) is DECIDED: skill metadata moves to agentskills.io frontmatter in `SKILL.md`, skillrig fields under `metadata.x-skillrig.*`; `skill.toml` is dropped. So the catalog's field-source is now frontmatter, not `skill.toml`.
**Time-box**: ~30 min
**Confidence**: HIGH on the lifecycle/aggregation model (read directly off the real origin's workflows + release config); HIGH on the scope call (grounded in architecture §2/§9 single-impl rule + the existing fallback-script seam).

## Question

Who generates and maintains the catalog `search` reads, how, and is `skillrig index` (the origin-side generator) in 003's scope or a sibling feature? Specifically: (1) is the catalog **single-tip** (HEAD tree) or **cross-ref aggregated** (skills/versions across older tags/releases)? (2) is it **full-regenerate**, **append-only**, or does it need **GC**? (3) does building `skillrig index` belong in 003 (the catalog must actually be generatable for `search` to mean anything, and skillrig is the origin tool too), or is 003 consume-only + a contract test (FR-023) with `skillrig index` a sibling feature?

## Findings

### Finding 1: Today the catalog is single-tip, full-regenerate, merge-triggered — NOT release/tag-triggered

The real origin's index workflow (`/Users/vincentdesmet/specledger/skillrig/skillrig-origin/.github/workflows/index.yml`) is unambiguous on every lifecycle axis:

```yaml
on:
  push:
    branches: [main]
    paths: ["skills/**", ".skillrig-origin.toml", "policy.toml"]
...
- name: Regenerate index.json
  run: |
    if command -v skillrig >/dev/null 2>&1; then
      skillrig index --out index.json
    else
      ./scripts/build-index.sh > index.json
    fi
- name: Commit if changed
  run: |
    if ! git diff --quiet -- index.json; then ... git commit -m "chore: regenerate index.json"; git push; fi
```

Four load-bearing facts:
1. **Trigger = push to `main`** (paths `skills/**`), **not** tag creation, **not** `release:` events. So the catalog reflects the **HEAD tree of the default branch** at each merge.
2. **Full-regenerate** — the step rebuilds `index.json` from scratch and commits *iff the file changed* (`git diff --quiet`). There is no merge/append of a previous catalog; the prior `index.json` is overwritten wholesale. A skill deleted from the HEAD tree simply vanishes from the next build (no tombstone, no GC needed — regeneration *is* the GC).
3. **The catalog walks the working tree, not git history.** `build-index.sh` (line 18) loops `for toml in skills/*/skill.toml` — it reads the checked-out directory, never `git log`/`git tag`. (Post-S1 this loop re-points at `skills/*/SKILL.md` frontmatter; the *traversal model* — "walk the current tree" — is unchanged.)
4. **`generated` banner in the committed `index.json`** says it verbatim: *"Produced by .github/workflows/index.yml running scripts/build-index.sh on merge to the default branch."*

Architecture corroborates: §2 line 88 — *"the only write the system makes to the monorepo is `index.json` regeneration, and that's a **merge-triggered** GitHub Action running `skillrig index`."* §9 line 307 says *"On release, `internal/index` walks `skills/*/skill.toml`"* — note the *wording* "on release" is slightly stale vs. the actual merge trigger, but the **traversal is still HEAD-tree**, never tag-history. The merge trigger is the operative reality.

**Conclusion:** the v0 catalog is, by construction, **single-tip / full-regenerate / no-GC**. The infrastructure to do cross-ref aggregation does not exist and was never built.

### Finding 2: The version in the catalog comes from the manifest, decoupled from the git tag — one version per skill

The committed `index.json` carries `"version": "1.4.0"` for `terraform-plan-review`. Where does that number come from, and does it track tags?

- **Source = the manifest field**, not the git tag. `build-index.sh:21` greps `^version` out of the skill manifest (post-S1: `metadata.x-skillrig.version` in frontmatter). The skill's own `skill.toml`/frontmatter declares `version = "1.4.0"`.
- **release-please keeps that field in sync with the tag** but they are *separate stores*. `.release-please-manifest.json` records `"skills/terraform-plan-review": "1.4.0"`; `release-please-config.json` uses `include-component-in-tag: true` + `tag-separator: "-"` → merging a release PR cuts the prefixed tag `terraform-plan-review-v1.4.0` (`.skillrig-origin.toml` `tag_scheme = "name-vSEMVER"`) **and** bumps the version inside the skill's manifest in the same PR. So at HEAD, the manifest version == the latest tag's version, by release-please's own bookkeeping.
- **Therefore the catalog carries exactly ONE version per skill: the HEAD version**, which equals the newest released tag. It does **not** enumerate `1.3.0`, `1.2.0`, … — those live only as git tags (`terraform-plan-review-v1.3.0`), never in `index.json`.

This is the crux for the aggregation question: **versions are not aggregated; the catalog shows the current released version of each skill that exists at HEAD.**

### Finding 3: Cross-ref tag aggregation — the right v0 model is single-tip, and `--pin` does NOT need the catalog (comment 8e05b856)

The load-bearing question: should the catalog aggregate skills/versions across older refs/releases, so `search` can show multiple versions of a skill, or a skill removed at HEAD but present at an older tag?

**Decision: NO cross-ref aggregation for v0. The catalog reflects the current tip only.** Rationale, grounded:

1. **`search` is discovery, `add --pin` is acquisition — and they use different planes.** spec-tech.md §2 line 28 already nails the catalog as *"discovery-only"* with *"no per-skill treeSha or commit."* §5 / D-pin: a `--pin v1.4.0` resolves to the git tag `terraform-plan-review-v1.4.0` and `add` fetches **that tag's subtree directly** (S4's `git clone --sparse` at the tag), recording commit + computed treeSha. **The pin path never consults the catalog for the old version** — git tags *are* the version-history index. So "search shows v1.4.0, user pins v1.3.0" works fine: search surfaces the skill's existence + current version; the user pins any tag they know; `add` fetches the tag. The catalog does not need older versions for `--pin` to function.

2. **Cross-ref aggregation would require walking tag history at index time** — `git tag --list 'terraform-plan-review-v*'`, checking out or `git show`-ing each tag's manifest, and merging. That is a categorically different (and much heavier) generator than "walk the HEAD tree," would make `index.json` grow unbounded with release history (the GC problem from comment ef449651 becomes real), and would couple the catalog's correctness to tag-naming discipline across all of history. It buys only a *browse-old-versions* UX that git tags + `gh release list` already provide.

3. **Removed-at-HEAD skills SHOULD disappear from search.** If an org deletes a skill from the default branch, `search` *not* listing it is correct — it signals "deprecated/withdrawn, don't adopt." A consumer who already vendored it keeps working (their lock has commit+treeSha; `verify` is offline and does not need the catalog). Re-acquiring a withdrawn skill by exact `--pin <tag>` still works against the tag. So single-tip loses nothing a consumer needs, and gains "the catalog reflects what the org currently endorses."

4. **`skillrigConvention` is a single scalar at catalog root**, not per-version. A cross-ref catalog spanning refs with *different* convention versions would have no coherent root convention — another sign aggregation fights the design.

**Net:** the catalog answers *"what skills does this origin offer right now, and at what current version,"* keyed by `OWNER/REPO@ref` = a branch tip (architecture §9b identity grammar). Version *history* lives in git tags and is reached via `--pin`, not the catalog. This is the correct v0 model and it is also the *only* model the existing infrastructure implements.

### Finding 4: Append-only vs full-regenerate vs GC (comment ef449651) — full-regenerate, GC is YAGNI

Given single-tip (Finding 3), this question largely dissolves:

- **Full-regenerate is correct and is what exists.** Each merge to `main` rebuilds `index.json` from the HEAD tree and commits if changed (Finding 1). The catalog is a pure function of the HEAD tree: `index.json = f(skills/*/SKILL.md frontmatter at HEAD)`. Deterministic, reproducible, no accumulated state.
- **Append-only is wrong here.** Appending (accumulating every version ever released into the catalog) is the cross-ref model rejected in Finding 3 — it reintroduces unbounded growth and the GC problem. As release-please cuts `terraform-plan-review-v1.5.0`, the *right* behavior is: the release PR bumps the manifest version to `1.5.0`, the merge re-triggers the index workflow, and the catalog's single row for that skill flips `version: 1.4.0 → 1.5.0`. The old row is *replaced*, not appended.
- **GC is YAGNI for v0.** With full-regenerate from HEAD, there is nothing to garbage-collect: stale entries can't accumulate because the file is rebuilt wholesale each time. GC only becomes a concept under an append-only/aggregated catalog — which we're not building. Record GC as explicitly out-of-scope, revisitable only if a future feature wants a version-history catalog.

One sequencing nuance worth noting in the contract: the **release PR merge** (which bumps the manifest version) and the **index regeneration** are both `push: main` events, and the index workflow's `paths: skills/**` filter *does* fire on a release-please version bump to a skill's manifest (the bump edits a file under `skills/`). So the catalog stays consistent with released versions automatically — no separate "on release" hook needed. (This also means the §9 "on release" wording should be corrected to "on merge to main" — FR-024 doc reconciliation.)

### Finding 5: Scope — build `skillrig index` IN 003, do not roadmap it as a sibling

This is the decisive call. Weighing it against the architecture's single-implementation rule and the existing seam:

**Arguments that `skillrig index` belongs in 003:**

1. **The catalog MUST be generatable for `search` to be meaningful, and skillrig is the origin tool.** The spike's own framing flags "consume-only + roadmap a generator" as a *false economy*. `search` reads `index.json`; if `index.json` can only be produced by a documentation-grade bash script (`build-index.sh`) that **provably drifts** (it emits only `name/version/description/path`, dropping `tags`, `namespace`, `requires` — spec-tech.md §2 line 30; confirmed at `build-index.sh:25`), then `search --tag` has no trustworthy data source. FR-023 *requires* reconciling the generator with what `search` consumes. The cheapest correct way to reconcile is to make the **authoritative generator real**, not to patch the bash fallback to also emit tags (which just moves the drift risk).

2. **AP-04 single-implementation makes it nearly free.** Architecture §2 (line 88) and §9, and `index.yml`'s own comments, state `skillrig index` *"shares skillcore/manifest parsing with verify/bump so values can't diverge."* Post-S1, 003 is **already** rewriting `pkg/skillcore`'s manifest parser to read `SKILL.md` frontmatter (S1 commit 1: replace `ParseManifest`). `skillrig index` is then: *walk `skills/*/SKILL.md`, call the same `ParseManifest`, marshal to the catalog JSON shape.* The parser — the hard, shared part — is being built in 003 regardless. The generator is a thin walk+marshal on top, in the same package, satisfying the single-impl rule *by construction*. Splitting it into a sibling feature would mean either (a) the sibling re-implements/duplicates frontmatter parsing (AP-04 violation) or (b) the sibling can't start until 003's parser exists anyway — so the coupling argues for *together*, not *apart*.

3. **The seam already expects the binary.** `index.yml` (lines 36-40) already branches `if command -v skillrig … skillrig index --out index.json … else ./scripts/build-index.sh`. The CI contract is *written for* the binary; the bash script is explicitly the *"legible fallback for environments without the binary"* (`build-index.sh:1-6`). Shipping `skillrig index` lights up the path the origin template was authored against and lets FR-023 retire/demote the drifting fallback.

4. **It closes the FR-023 drift at the source.** With `skillrig index` authoritative, the committed `index.json` and the generator can't disagree — they're the same code. FR-023 becomes "ship `skillrig index` + a contract test asserting `index.json == skillrig index` over the origin fixture," which is *stronger* than "reconcile a bash script's output by hand."

**The one argument for deferral** — "003 is consumer-side (`search`+`add`), `index` is origin-side, keep slices small" — is outweighed because: the consumer (`search`) is *useless without a correct catalog*, and the generator shares 003's brand-new parser. Deferring `index` means 003 ships a `search` that reads a catalog only a drifting bash script can produce — i.e. ships the consumer half of a contract whose producer half is known-broken. That is the false economy the framing warned about.

**Scope decision: `skillrig index` lands in 003** as a thin `pkg/skillcore` generator (`GenerateCatalog(skillsDir) → Catalog`) + a `skillrig index --out` command, sharing the S1 frontmatter parser with `add`/`verify`. FR-023's origin-template work becomes: (a) re-point `build-index.sh` at frontmatter as the *fallback* (keep it legible but demoted), (b) regenerate the committed `index.json` via `skillrig index`, (c) add a contract test. This keeps 003's slice coherent (the producer and consumer of the catalog ship together, against the same parser) rather than artificially small.

*Bound the scope:* `skillrig index` ships **only** the single-tip/full-regenerate generator (Findings 1-4). No tag-history walking, no GC, no append. That keeps it small — it's a tree-walk + the parser 003 already has.

### Finding 6: Reconciliation with S1 — generator reads `metadata.x-skillrig.*` via the shared parser (AP-04)

Per S1's D-S1-catalog-source, `skillrig index` produces each catalog row by:
- `name`, `description` ← standard frontmatter top-level fields.
- `version`, `namespace`, `tags`, `requires` ← `metadata.x-skillrig.*` (S1 Option A: `tags` space-string → split to `[]`; `requires` nested list-of-maps).
- `path` ← the skill's directory (`skills/<name>`).
- Catalog root: `skillrigConvention` ← `.skillrig-origin.toml` `convention_version`; `origin` ← its `origin` field.

Critically, `skillrig index` calls the **same** `ParseManifest(skillDir)` that `add` (vendoring) and `verify` (prereq read) call — one frontmatter parse implementation in `pkg/skillcore`, three callers. This is the §9/§2 "values can't diverge" guarantee made literal and is exactly the AP-04 discipline S1 set up. No second parser, no bash-grep of YAML in production (the fallback's grep is acknowledged-lossy and demoted).

## Decisions

- **D-S2-tip — the catalog is SINGLE-TIP, not cross-ref aggregated.** `index.json` at `OWNER/REPO@ref` reflects the HEAD tree of that branch: the skills that exist there, each at its current (HEAD == latest-released-tag) version. It does **not** enumerate older versions or skills removed at HEAD. Version *history* lives in git tags and is reached by `add --pin <tag>` fetching the tag subtree directly — the catalog is never the version-history index (Findings 2, 3). This matches the only model the origin's existing `index.yml`/`build-index.sh` implement.
- **D-S2-regen — FULL-REGENERATE on merge to `main`; GC is YAGNI.** Each merge that touches `skills/**` rebuilds `index.json = f(HEAD frontmatter)` and commits iff changed. Not append-only (that's the rejected aggregated model). Nothing accumulates, so there is nothing to garbage-collect; record GC as out-of-scope, revisit only if a version-history catalog is ever wanted (Findings 1, 4). release-please version bumps land under `skills/**`, so they auto-retrigger the index and keep catalog versions == released tags with no extra hook.
- **D-S2-scope — `skillrig index` SHIPS IN 003** (not a sibling/roadmap item), as a thin `pkg/skillcore` generator + `skillrig index --out` command sharing S1's frontmatter parser with `add`/`verify` (AP-04). Rationale: `search` is useless without a non-drifting catalog; FR-023 demands reconciling the generator; the hard part (the frontmatter parser) is already being built in 003 by S1; and `index.yml` is already authored to call the binary. Deferring would ship a consumer against a known-drifting producer — the false economy the framing named. Scope-bounded to the single-tip/full-regenerate generator only (no tag-history, no GC). (Finding 5.)
- **D-S2-source — generator field-source per S1.** `name`/`description` ← standard frontmatter; `version`/`namespace`/`tags`/`requires` ← `metadata.x-skillrig.*`; `path` ← directory; root `skillrigConvention`/`origin` ← `.skillrig-origin.toml`. One shared `ParseManifest`, three callers (Finding 6).
- **D-S2-fallback — demote, don't delete, `build-index.sh`.** Re-point it at `SKILL.md` frontmatter so the contract stays legible for binary-less environments, but `skillrig index` is authoritative and the committed `index.json` is produced by it. The grep-based bash path is acknowledged-lossy (can't robustly parse nested `x-skillrig.requires`); it may legitimately emit a reduced schema as a *documented* fallback, with `skillrig index` as the full-fidelity producer.

## Recommendations

1. **Put `skillrig index` in 003's plan as a first-class command** (Query-adjacent / Environment-write pattern — it writes the origin's `index.json`; classify per cli.md and run the checklist). Implement `pkg/skillcore.GenerateCatalog(skillsDir, originCfg) (Catalog, error)` that walks `skills/*/SKILL.md`, calls the shared `ParseManifest`, and assembles the catalog struct; `internal/cli` adds `skillrig index [--out index.json] [--json] [--verbose]` rendering/writing it.
2. **Define the FR-023 catalog contract** (below) as the convention-1 catalog schema, and add a contract/ground-truth test: `skillrig index` over the origin fixture must equal the committed `index.json` (the producer==artifact guarantee), analogous to S4's `fetched treeSha == raw git tree-SHA` oracle-independence.
3. **Fix the origin template in-branch (FR-023):** re-point `build-index.sh` at frontmatter (demoted fallback), regenerate the committed `index.json` via `skillrig index`, and add the convention-1 catalog-schema note to `docs/CONVENTION.md`.
4. **Correct stale docs (FR-024):** architecture §9 line 307 says "On release, `internal/index` walks `skills/*/skill.toml`" — update to "on **merge to main**, `skillrig index` walks `skills/*/SKILL.md` frontmatter." Note explicitly: catalog is single-tip; version history is git tags, reached via `--pin`.
5. **State the single-tip model in the `search` UX:** `search` shows the current offered version; document that `add --pin <tag>` is how to get a specific/older version (it fetches the tag, not the catalog). This keeps users from expecting `search` to be a version browser.
6. **Record GC + cross-ref aggregation as explicit non-goals** in spec.md/spec-tech.md so a later reviewer doesn't reopen them; the trigger to revisit is "a feature needs a version-history/browse-all-versions catalog," which v0 does not.

## The concrete FR-023 catalog contract (convention 1)

`index.json` at origin repo root. **Single-tip, full-regenerate, no per-skill treeSha/commit (discovery-only).** Produced by `skillrig index` (authoritative) / `build-index.sh` (demoted fallback) on merge to the default branch.

```jsonc
{
  "skillrigConvention": 1,          // ← .skillrig-origin.toml convention_version (binary gates on this)
  "origin": "my-org/my-skills",     // ← .skillrig-origin.toml origin (OWNER/REPO identity, §9b)
  "skills": [
    {
      "name":        "terraform-plan-review",          // ← SKILL.md frontmatter `name` (== dir name)
      "version":     "1.4.0",                          // ← metadata.x-skillrig.version (== latest released tag at HEAD)
      "namespace":   "my-org",                         // ← metadata.x-skillrig.namespace (optional for search)
      "description": "Review a terraform plan ...",    // ← SKILL.md frontmatter `description`
      "tags":        ["platform-team","terraform","aws"], // ← metadata.x-skillrig.tags (space-string → split); REQUIRED for --tag
      "path":        "skills/terraform-plan-review",   // ← skill directory (relative to repo root)
      "requires": [                                    // ← metadata.x-skillrig.requires (carried; not required by search)
        { "tool": "oxid", "version": ">=0.4.0", "source": "my-org/my-skills" },
        { "tool": "terraform", "version": ">=1.6", "source": "hashicorp/terraform" }
      ]
    }
  ]
}
```

**Contract invariants:**
- The catalog is a **pure function of the HEAD tree's `SKILL.md` frontmatter** + `.skillrig-origin.toml`. Reproducible: `skillrig index` over the same tree yields byte-identical (modulo key order) output ⇒ the contract test compares structurally.
- **`search` consumes:** per-skill `name`, `version`, `description`, `tags[]`, `path`; root `skillrigConvention`, `origin`. `namespace`/`requires` are carried but optional for `search` this slice (spec-tech.md §2 line 33).
- **One version per skill** = the HEAD/current version. No version arrays, no historical rows (D-S2-tip).
- **`skillrigConvention` is gated by the binary** before `search`/`add` act (§4 convention gate, FR-016); a mismatch is the "update skillrig" error class.
- **No `treeSha`/`commit` per skill** — the catalog is discovery-only; integrity anchors are computed at `add` time and recorded in the consumer's lock (spec-tech.md §2 line 28, §5).

## References

- Origin index workflow (trigger=push:main, paths skills/**; full-regenerate + commit-if-changed; calls `skillrig index` else fallback) — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/.github/workflows/index.yml`.
- Origin fallback generator (walks `skills/*/skill.toml`, emits reduced `name/version/description/path` — the FR-023 drift) — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/scripts/build-index.sh:18,21,25`.
- Committed catalog (single-tip shape, `generated` banner, full schema incl. tags/requires) — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/index.json`.
- Origin convention/contract (convention_version=1, origin, skills_dir, `tag_scheme = "name-vSEMVER"`) — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/.skillrig-origin.toml`.
- release-please config (per-skill prefixed tags: `include-component-in-tag`, `tag-separator '-'` → `terraform-plan-review-v1.4.0`) — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/release-please-config.json`; manifest `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/.release-please-manifest.json`.
- Release workflow (release-please cuts prefixed tags on push:main; goreleaser only for `oxid-` CLI tags) — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/.github/workflows/release.yml`.
- Sample skill manifest (version 1.4.0 in `skill.toml`; frontmatter already has name/description — the S1 drift) — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/skills/terraform-plan-review/{skill.toml,SKILL.md}`.
- Architecture — `docs/ARCHITECTURE-v0.md` §2 line 88 (index = merge-triggered, only write, runs `skillrig index`), §9 lines 305-309 ("on release" wording stale; walks skills, discovery-only, GH-Pages dropped), §9b line 315 (`OWNER/REPO[/path]@ref` identity grammar, catalog as one of three consumers), §13 roadmap (catalog in v0).
- Upstream S1 (field-source: frontmatter + `metadata.x-skillrig.*`; drop `skill.toml`; AP-04 single parser) — `specledger/003-search-remote/research/2026-05-31-skill-manifest-format.md`.
- Framing — `specledger/003-search-remote/spec-tech.md` §2 (catalog discovery-only, build-index.sh drift = FR-023), §8b (Spike S2), §9 (FR-023/FR-024 co-evolution).
