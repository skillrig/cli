# Research: Skill Manifest Format — `skill.toml` vs agentskills.io frontmatter + `x-skillrig.*`

**Date**: 2026-05-31
**Context**: Spike S1 for `003-search-remote` (spec-tech.md §8b). The catalog `search` reads is *generated from* skill metadata, so the manifest format is the field-source decision that gates the rest of 003. 002 shipped a sibling `skill.toml`; the working hypothesis is to drop it and move metadata into `SKILL.md` frontmatter with skillrig extensions.
**Time-box**: ~30 min
**Confidence**: HIGH (every load-bearing claim verified against checked-out gh-cli source and the live agentskills.io spec).

## Question

Should 003 keep the current `skill.toml` sibling manifest (002) or move skill metadata into agentskills.io **frontmatter** in `SKILL.md` with skillrig extensions under the standard's `metadata` section? And if migrating: can `allowed-tools` express skillrig's `[[requires]]`, what is the concrete `x-skillrig.*` shape, how big is the migration, and must a "manifest reframe" land before 003?

## Findings

### Finding 1: The agentskills.io standard — exact field set (authoritative)

Fetched https://agentskills.io/specification. The complete frontmatter field table:

| Field | Required | Constraint (verbatim) |
|---|---|---|
| `name` | Yes | ≤64 chars, lowercase alnum + hyphens, no leading/trailing/`--`. **Must match parent dir name.** |
| `description` | Yes | ≤1024 chars, non-empty. |
| `license` | No | License name or reference to bundled license file. |
| `compatibility` | No | ≤500 chars. Free text: "intended product, required system packages, network access, etc." |
| `metadata` | No | **"A map from string keys to string values. Clients can use this to store additional properties not defined by the Agent Skills spec... We recommend making your key names reasonably unique to avoid accidental conflicts."** |
| `allowed-tools` | No | **"A space-separated string of tools that are pre-approved to run" (Experimental).** Example: `allowed-tools: Bash(git:*) Bash(jq:*) Read`. |

There is **no `version` field and no `tags` field** in the standard. The spec's own example puts `version: "1.0"` *inside* `metadata` (`metadata: {author: ..., version: "1.0"}`) — confirming version/tags are extension territory, not standard fields.

**Key consequence:** `metadata` is the officially-sanctioned, namespaced extension mechanism. This is exactly what the hypothesis relies on, and the spec explicitly blesses it.

### Finding 2: `allowed-tools` CANNOT express skillrig's `[[requires]]` — decisive

The hypothesis floated `allowed-tools` as "the natural home for backing-CLI `requires`." **This is wrong**, on two independent confirmations:

1. **Spec semantics**: `allowed-tools` is "tools pre-approved to *run*" inside an agent session — `Bash(git:*) Read`. It is an agent-permission allowlist (which tool *invocations* the agent may make), not a backing-CLI prerequisite list. It is a flat space-separated **string**, with no slot for a version constraint (`>=0.4.0`) or a `source` repo.

2. **gh-cli enforces this**: `pkg/cmd/skills/publish/publish.go:262-270` actively **rejects** `allowed-tools` as an array — "allowed-tools must be a string (space-delimited), not an array." Tests at `publish_test.go:390,521` use `allowed-tools: git` (bare name). So even the reference client treats it as bare tool names only.

skillrig's `Require` (`pkg/skillcore/manifest.go:24-29`) carries `tool` + `version` (constraint) + `source` (private repo) + `manager`. None of that survives in `allowed-tools`. **`requires` MUST live under `x-skillrig.*`.** `compatibility` (free-text ≤500 chars) is also unsuitable as a structured source — it's human prose, not machine-parseable version constraints.

### Finding 3: How the Go gh CLI parses frontmatter (validation #1)

Parser: `/Users/vincentdesmet/specledger/skillrig/gh-cli/internal/skills/frontmatter/frontmatter.go`.

- **YAML lib**: `gopkg.in/yaml.v3` (line 9). Standard, robust.
- **Robustness**: `Parse` (lines 31-63) trims leading `\r\n`, requires a leading `---`, finds the closing `\n---`, and `yaml.Unmarshal`s the block **twice** — once into a typed `Metadata` struct (line 53-56) and once into a raw `map[string]interface{}` (line 48-51) preserved as `RawYAML`. No frontmatter → returns the whole content as `Body`, no error (graceful). Invalid YAML → hard error `"invalid frontmatter YAML: %w"`.
- **The `Metadata` struct** (lines 14-20): `Name`, `Description`, `License`, and crucially `Meta map[string]interface{} \`yaml:"metadata,omitempty"\``. So gh treats `metadata` as an **arbitrary nested map** — `interface{}` values, NOT restricted to strings (more permissive than the spec's "string→string" letter).
- **Extension pattern (the precedent that matters)**: `InjectGitHubMetadata` (lines 70-98) writes provenance into `metadata` under **flat, dotted-prefix keys**: `github-repo`, `github-ref`, `github-tree-sha`, `github-path`, `github-pinned`. Test `frontmatter_test.go:172-184` confirms the on-disk shape is flat (`metadata.github-tree-sha: tree456`), NOT a nested `metadata.github: {...}` sub-map. The comment (lines 65-68) states the intent verbatim: *"Keys are prefixed with `github-` to avoid collisions with other tools' metadata."*

This is direct, first-party validation of the hypothesis's approach: a namespaced-key-prefix under `metadata` is exactly how the in-stack reference client already extends the standard.

### Finding 4: Prior art on search-by-topic (validation #5) — gh has NO generated catalog

This reframes 003's catalog design.

- **`gh skill search`** (`pkg/cmd/skills/search/search.go`): there is **no `index.json`**. Search hits the **GitHub Code Search API** for `filename:SKILL.md <query>` (line 288), with variants for `path:` and `user:` scoping (lines 290-334). `--json` fields are `repo, skillName, namespace, description, stars, path` (lines 39-47). **No `--tag` flag, no tag filtering** — relevance ranking is name-match-first over code search results.
- **Discoverability = repo topic**, not a catalog. `gh skill publish` adds the **`agent-skills` GitHub topic** to the repo (`publish.go:483-493,692-699`); that topic is how skills become findable. Versioning = git tags + GitHub Releases (`publish.go:459-469`, immutable releases).
- **Vercel `npx skills` / `sl`**: architecture §4.2 lineage note + §11b confirm Vercel uses a lockfile + tree-SHA model (cited for the two-lock split), but no evidence of a tag-faceted search catalog; gh is the closer prior art and it does *topic + code-search*, not a faceted index.

**Implication for 003's `search`**: skillrig's design (a generated `index.json` carrying `tags[]`, gated by `skillrigConvention`) is a **deliberate divergence** from gh's "code-search + repo-topic" model — it gives skillrig the org-controlled, `--tag`-filterable, offline-coherent catalog that gh lacks (architecture §366 already flags gh's "discovery leans on GitHub topic conventions, not a fully org-controlled artifact" as a contract gap). So 003 keeps the generated catalog, but S1's manifest format becomes its field-source.

### Finding 5: Current state — frontmatter and `skill.toml` already coexist (partial migration already real)

The PoC origin skill `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/skills/terraform-plan-review/` **already has agentskills.io frontmatter** in `SKILL.md` (`name` + `description`) AND a full `skill.toml` (name, version, namespace, description, tags, two `[[requires]]`). So `name`/`description` are **duplicated** today — a latent drift bug. The `index.json` is generated from the richer `skill.toml` shape.

### Finding 6: Migration scope (validation #4) — small and self-contained

Every `skill.toml` touch-point in the CLI:

- **`pkg/skillcore/manifest.go`** — `Manifest`/`Require` structs + `ParseManifest(path)` (reads a TOML file via `pelletier/go-toml/v2`). ~47 lines. This is the *only* parse implementation (AP-04 single-impl already holds).
- **`pkg/skillcore/add.go:91`** — the sole caller: `ParseManifest(filepath.Join(srcDir, "skill.toml"))`.
- **`pkg/skillcore/verify.go:193-195`** — `isSkillDir` treats a dir as a skill if it contains `skill.toml` **OR** `SKILL.md`. (Already SKILL.md-aware; can simply drop the `skill.toml` marker.)
- **`pkg/skillcore/treesha.go:2`** — doc comment only.
- **Tests** — `manifest_test.go`, `add_test.go`, `verify_test.go`, `treesha_test.go`, `helpers_test.go` (`sampleManifest` fixture), `test/skillcore_quickstart_test.go`. All write a `skill.toml` fixture; these flip to writing frontmatter.
- **Origin template** — `skill.toml` → fold into `SKILL.md` frontmatter; regenerate `index.json` from frontmatter; update `scripts/build-index.sh` (this is FR-023, already in scope and entangled with S2).
- **Docs** — architecture §4.1 (`skill.toml` schema block), §307 (`internal/index` walks `skills/*/skill.toml`), CLAUDE.md, ROADMAP, cli.md references.

**The migration is a focused refactor of one parser + its callers + fixtures.** No new dependency: gh uses `gopkg.in/yaml.v3` and that is the right choice (the project already commits to "coupling to the standard, already incurred via R20-R21"; YAML is the standard's serialization). Adds **one dependency** (`gopkg.in/yaml.v3`) — a deliberate, justified exception to "no new deps," because frontmatter IS YAML and there is no TOML escape. `pelletier/go-toml/v2` stays (config + lock-adjacent).

### Finding 7: Concrete `x-skillrig.*` shape

The spec says `metadata` is string→string and recommends unique keys; gh's precedent is **flat dotted keys** (`github-tree-sha`). But skillrig needs *structured* data (`tags[]`, `requires[]` of objects). Two viable encodings:

**Option A — nested map under a single namespaced key (recommended):**
```yaml
---
name: terraform-plan-review
description: Review a terraform plan for risk and drift before apply...
license: Proprietary
metadata:
  x-skillrig.version: "1.4.0"
  x-skillrig.namespace: my-org
  x-skillrig.convention-version: "1"
  x-skillrig.tags: platform-team terraform aws        # space-delimited, mirrors allowed-tools convention
  x-skillrig.requires:                                 # nested list-of-maps (yaml.v3 / interface{} parse it fine)
    - tool: oxid
      version: ">=0.4.0"
      source: my-org/my-skills
      manager: mise
    - tool: terraform
      version: ">=1.6"
      source: hashicorp/terraform
      manager: mise
---
```
gh-cli's `map[string]interface{}` parses this losslessly. It *bends* the spec's "string→string" letter for `x-skillrig.requires` (nested), but: (a) the spec's own field is `map[string]interface{}` in the reference impl, (b) it's namespaced so it can't collide, (c) tags-as-space-string keeps the cheap fields spec-pure. Use dotted prefix `x-skillrig.` per the spec's "unique key" guidance and gh's `github-` precedent.

**Option B — fully flat (most spec-literal), tags/requires as encoded strings:** avoid for `requires` (encoding a list-of-objects into a string is ugly and lossy). Only adopt if a downstream agentskills.io client is found that hard-rejects non-string metadata values — none found in gh.

**Recommendation: Option A.** Map skillrig fields as: `name`/`description`/`license` → standard top-level; `tags`/`version`/`namespace`/`convention-version`/`requires` → `metadata.x-skillrig.*`. Do NOT put `requires` in `allowed-tools` (Finding 2).

## Decisions

- **D-S1-format — MIGRATE to frontmatter + `x-skillrig.*`; drop `skill.toml`.** Confirmed by every validation axis: the standard's `metadata` field exists *specifically* for this (Finding 1), gh-cli proves the namespaced-key pattern in production (Finding 3), and the sibling-file rationales ("two audiences / travels-with-skill / offline-doctor / TOML-nicer") don't survive — frontmatter travels with `SKILL.md` atomically (no name/description drift, which the PoC origin *currently has*), is offline-readable by the same parse, and aligns with 26+ ecosystem clients. Cost is YAML-vs-TOML cosmetics + one new dep (`yaml.v3`) — both already implied by standard-coupling.
- **D-S1-requires — `requires` lives under `metadata.x-skillrig.requires`, NOT `allowed-tools`.** `allowed-tools` is an agent-permission string (bare tool names), provably unable to carry version+source; gh-cli rejects arrays there. This corrects the hypothesis.
- **D-S1-catalog-source — the catalog (`index.json`) is generated FROM frontmatter `metadata.x-skillrig.*` + standard `name/description`.** `search`'s consumed schema (`name, version, description, tags[], path`, catalog-level `skillrigConvention, origin`) sources `version`/`tags` from `x-skillrig.*`, `name`/`description` from standard frontmatter, `path` from directory location. This is the field-source answer S2 depends on.
- **D-S1-sequencing — the manifest reframe is SMALL and lands IN-SLICE, as 003's first commit.** It is one parser (`manifest.go`) + one caller (`add.go:91`) + `isSkillDir` (already SKILL.md-aware) + fixtures + origin-template/`index.json`. Not big enough to warrant a separate prerequisite feature. Doing it first (before remote `add`/`search`) means the fetched-subtree read and catalog-parse are both written against the new format from the start, avoiding a double rewrite.

## Recommendations

1. **Land the manifest reframe as commit 1 of 003.** Replace `ParseManifest` to read `SKILL.md` frontmatter (gopkg.in/yaml.v3): parse standard `name/description/license`, then map `metadata` → pull `x-skillrig.version/namespace/tags/requires/convention-version`. Keep the `Manifest`/`Require` Go types nearly as-is (their fields are format-agnostic). Add `gopkg.in/yaml.v3` to go.mod with a one-line rationale comment (frontmatter is YAML; no TOML path exists for the standard).
2. **Adopt `x-skillrig.*` Option A** (nested map under `metadata`, dotted-prefix keys; `tags` as space-string, `requires` as nested list-of-maps). Document this as the convention-1 manifest contract alongside the catalog contract.
3. **Fix the origin template in the same branch (FR-023):** fold `skill.toml` into `SKILL.md` frontmatter, delete the duplicate `name`/`description` drift, and regenerate `index.json` + `build-index.sh` from frontmatter. Coordinate with S2 (catalog generation) — S2 now has its field-source nailed down.
4. **Keep skillrig's generated `index.json` catalog** (do NOT adopt gh's code-search + repo-topic discovery). It is the org-controlled, `--tag`-filterable, offline-coherent artifact gh lacks; just re-point its generator at frontmatter.
5. **Update `isSkillDir`** to key on `SKILL.md` only (drop the `skill.toml` marker) once fixtures migrate, so verify's orphan-detection stays correct.
6. **Update docs in-branch** (CLAUDE.md per the same-branch rule): architecture §4.1 (replace TOML block with frontmatter), §307 (index generator walks `SKILL.md`), cli.md/ROADMAP.

## Risks

- **Spec-letter vs. practice on nested metadata.** The spec says `metadata` is string→string; `x-skillrig.requires` (nested list) bends that. Mitigation: gh's reference impl uses `map[string]interface{}` (parses nested fine); the data is namespaced so it can't collide with other clients; only skillrig reads it. LOW risk, but note it in the contract. If a stricter validator (`skills-ref validate`) rejects non-string metadata, fall back to a JSON-encoded string for `x-skillrig.requires` only — verify against `skills-ref` before finalizing (not done in this time-box; flag for plan).
- **New dependency (`gopkg.in/yaml.v3`).** Violates the "no new deps" stance literally, but is unavoidable for a YAML standard and matches gh. Accept with rationale.
- **Coupling to a moving standard.** agentskills.io `allowed-tools` is marked "Experimental"; the standard "may change." skillrig only consumes stable fields (`name`/`description`/`metadata`) and owns its `x-skillrig.*` namespace, so exposure is limited. Already-incurred per project history.
- **`name` must match parent dir name (spec rule).** skillrig's add/verify should keep honoring this; the directory-derived skill identity already aligns with it. Minor: enforce in lint, not blocking for 003.

## References

- agentskills.io specification — https://agentskills.io/specification (frontmatter field table; `metadata`, `allowed-tools`, `compatibility` definitions quoted in Finding 1).
- gh-cli frontmatter parser — `/Users/vincentdesmet/specledger/skillrig/gh-cli/internal/skills/frontmatter/frontmatter.go` (yaml.v3; `Metadata.Meta map[string]interface{}`; `InjectGitHubMetadata` flat `github-`-prefixed keys, lines 14-20, 65-98).
- gh-cli frontmatter test — `.../internal/skills/frontmatter/frontmatter_test.go:172-184` (flat `metadata.github-tree-sha` round-trip).
- gh-cli `allowed-tools` validation — `.../pkg/cmd/skills/publish/publish.go:262-270` ("must be a string, not an array"); tests `publish_test.go:390,521`.
- gh-cli search (code-search + topic, no catalog) — `.../pkg/cmd/skills/search/search.go:288-334,39-47`; publish topic add `publish.go:483-493,692-699`.
- skillrig 002 manifest — `pkg/skillcore/manifest.go` (`ParseManifest`, `Manifest`, `Require`); sole caller `pkg/skillcore/add.go:91`; `isSkillDir` `pkg/skillcore/verify.go:193-195`.
- PoC origin manifest — `/Users/vincentdesmet/specledger/skillrig/skillrig-origin/skills/terraform-plan-review/skill.toml` + `SKILL.md` (frontmatter already present; name/description duplicated) + `index.json`.
- Architecture — `docs/ARCHITECTURE-v0.md` §4.1 (skill.toml schema, lines 161-184), §4.2 (treeSha/commit), §307 (index generator), §366 (gh discovery-via-topic gap), §370 (OpenClaw requires prior art).
- spec-tech.md §8b — `specledger/003-search-remote/spec-tech.md` (Spike S1 framing, FR-023).
