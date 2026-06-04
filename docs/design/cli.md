# CLI Design Principles

<!-- TODO: remove duplication of progressively loaded agentic-go-cli-design skill? -->

Design guidelines for the generic `skillrig` CLI binary. The CLI is the entry point for humans, agents, and CI alike (R3) — the *same* static binary for every org (R4) — so it must be self-documenting, token-efficient, and navigable without external documentation. An agent that can only learn the tool from `--help` should still get one-shot success.

> **Source**: Principles adapted from the [Manus backend lead's CLI design post](https://www.reddit.com/r/LocalLLaMA/comments/1rrisqn/i_was_backend_lead_at_manus_after_building_agents/) and refined against skillrig's architecture (see [architecture.md](../../../../architecture.md)) and the durable contract in [requirements.md](../../../../requirements.md).

---

## Rules of Thumb

Quick reference for contributors. Every `skillrig` subcommand must satisfy these:

1. **Every subcommand must have `--help` with at least 2 usage examples**
2. **Every error must suggest a fix** — what failed, why, what to run instead
3. **Human output is compact** — truncated previews, counts, footer hints
4. **JSON output is complete** — full data, pipeable to `jq`, no truncation
5. **Errors to stderr, data to stdout** — enables clean piping
6. **Positional args for simple cases** — reserve flags for optional/complex params
7. **Origin is resolved, never baked in** — env > project config > global default (§2d of architecture.md); `--origin` overrides. The origin reference is `OWNER/REPO[@REF]`; `@REF` is optional and tracks a branch (see [Origin Reference Grammar](#origin-reference-grammar))
8. **Classify against a pattern before merging** — see [Pattern Classification](#pattern-classification) and the [pattern-gate checklist](checklist-template.md)
9. **The CLI is consume-only** — no `publish`, no `login`, no write credential in the binary (architecture §2b). GitHub is the authority plane.
10. **Standard flags everywhere** — see [Standard Flags](#standard-flags): `--json` + `--verbose` on every command; mutating commands also take `--dry-run` and refuse to clobber divergent content without `--force`

---

## Principle 1: Progressive Discovery

A well-designed CLI doesn't require reading documentation — `--help` tells you everything. The agent discovers on-demand, each level providing just enough for the next step.

The agent doesn't need to load all documentation at once, but discovers details on-demand as it goes deeper.

**Level 0: Tool description → command list injection**
The agent knows what `skillrig` subcommands exist from Cobra's auto-generated help. No need to preload all docs.

```
$ skillrig
skillrig — rig up your agents with skills (git-native skill distribution)

Commands:
  search   Query the origin's index.json for skills                    [implemented]
  add      Vendor a skill into this repo + write the lock entry        [implemented]
  verify   Offline integrity check — label-honesty (exit code; CI gate) [implemented]
  index    Generate the origin's index.json from skill frontmatter     [implemented, origin-side]
  bump     Detect upstream advance, open an upgrade PR
  global   Manage global-scope skills (fetch/restore)
  doctor   Superset health check (integrity + prereqs + auth)
  lint     Author-side conformance gate (run in the origin's PR CI)
  init     Bind this repo (or the global default) to an existing origin
```

**Level 1: `skillrig <command>` (no args) → subcommand usage**
When the agent is interested in a command, it just calls it. No arguments? The command returns its own usage:

```
$ skillrig global
Manage global-scope skills (not vendored; fetched/restored per environment)

Subcommands:
  add     Fetch and materialize a global skill
  verify  Check global-scope skills against the global lock
```

**Level 2: `skillrig <command> <subcommand>` (missing args) → specific params**

The agent decides to use `skillrig add` but isn't sure about the format? It drills down:

```
$ skillrig add
add requires exactly one argument: the skill name
why: got 0 argument(s)
fix: skillrig add <skill> (e.g. skillrig add terraform-plan-review); run skillrig add --help for flags and examples

# `skillrig add --help` then reveals the full (shipped) surface:
Usage:
  skillrig add <skill> [--pin <ref>] [--dry-run] [--force] [--json] [--verbose]

Examples:
  skillrig add terraform-plan-review
  skillrig add terraform-plan-review --pin v1.4.0
  skillrig add terraform-plan-review --dry-run
```

> The origin is **resolved**, never passed to `add` (no `--from`/`--origin` arg — clarified 2026-05-30). `add` now serves both a **local-path** origin (002) and a **remote** `OWNER/REPO` origin it fetches over `git` (003); the immutable per-skill `--pin <ref>` shipped with the remote path. The synopsis above is the shipped surface. See [Pin vs. branch ref](#origin-reference-grammar) for the `--pin` semantics.

Progressive disclosure: **overview (injected) → usage (explored) → parameters (drilled down).** The agent discovers on-demand, each level providing just enough information for the next step.

This is fundamentally different from stuffing 3,000 words of tool documentation into the system prompt. Most of that information is irrelevant most of the time — pure context waste. Progressive help lets the agent decide when it needs more.

This also imposes a requirement on command design: **every command and subcommand must have complete help output.** It's not just for humans — it's for the agent. A good help message means one-shot success. A missing one means a blind guess.

**Rule**: Every subcommand MUST have a `Long` description with at least 2 `Examples` in its Cobra definition. The agent should be able to construct correct commands from `--help` alone.

---

## Principle 2: Error Messages as Navigation

Agents will make mistakes. The key isn't preventing errors — it's **making every error point to the right direction.**

Agents can't Google. Every error must contain both "what went wrong" and "what to do instead."

```
# Bad: raw git/network error with no context
fatal: repository 'https://github.com/my-org/my-skills' not found

# Good: actionable guidance that preserves the raw error
add failed: cannot reach origin 'my-org/my-skills' (git: repository not found).
→ Check the origin is correct: 'skillrig init --origin <OWNER/REPO>' or 'cat .skillrig/config.toml'
→ If the repo is private, this is usually auth — see 'gh auth status' / GITHUB_TOKEN.
```

**The remote-fetch failure classes are distinct typed errors (R17/R18, FR-016–019).** When `add`/`search` fetch a remote origin, three confusable failures must each map to their own error class so the agent debugs the right thing — never collapse them. They are classified inside `pkg/skillcore` from the raw `git` stderr (`AuthError` / `UnreachableError` / `NotFoundError`), then rendered with what/why/fix by `internal/cli`:

```
# AuthError — private origin, no/invalid credentials. Two git-stderr families map here:
#   rejected  : "Authentication failed" / "Invalid username or token"
#   unavailable: "could not read Username/Password" / "terminal prompts disabled" / "Device not
#                configured" — the origin needed a credential we had none for, and git (run
#                non-interactively, see below) aborted instead of prompting (issue #25).
add failed: authentication to origin 'my-org/my-skills' failed (git: Authentication failed).
→ Authenticate: 'gh auth login', or export a GITHUB_TOKEN / GH_TOKEN with read access to the origin.
→ This is AUTH, not a missing/typo'd repo — the origin name resolved fine.

# UnreachableError — network failure / wrong host (git stderr: "Could not resolve host" / "Failed to connect")
search failed: origin 'my-org/my-skills' is unreachable (git: Could not resolve host github.com).
→ Check connectivity / proxy / the host in the origin reference; retry.
→ This is a network problem, not auth or a missing repo.

# NotFoundError — origin (or skill) does not exist. PRIVATE-REPO SUBTLETY: GitHub returns
# "not found" (not 403) for a private repo with no/bad token, so the fix names the auth path too.
add failed: origin 'my-org/my-skills' not found (git: repository not found).
→ Check the origin spelling: 'skillrig init --origin <OWNER/REPO>' or 'cat .skillrig/config.toml'.
→ If this is a PRIVATE origin, authenticate via 'gh auth login' or set GITHUB_TOKEN — GitHub reports a private repo you can't see as "not found".
```

Keep the **convention-version** failure distinct from all three above — it means the origin is reachable and authenticated but speaks a layout this binary doesn't support:

```
# IncompatibleConventionError — origin's convention_version is not the supported value (exact-match == 1)
add failed: origin 'my-org/my-skills' uses convention version 2, but this skillrig supports 1.
→ Update skillrig to a build that supports this origin's convention, or point at a compatible origin.

# NoSuchVersionError — a '--pin' that resolves to no git ref/tag (distinct from "skill not found")
add failed: no version 'v9.9.9' of 'terraform-plan-review' in origin 'my-org/my-skills' (no such tag).
→ List the published versions on the origin, or omit '--pin' to vendor the origin-branch tip.
```

```
# Bad: missing origin, no next step
Error: origin not configured

# Good: next step (origin discovery precedence, §2d)
No origin configured for this repo.
→ Bind it: 'skillrig init --origin my-org/my-skills' (writes .skillrig/config.toml)
→ Or set SKILLRIG_ORIGIN for a one-off / CI override.
```

```
# Bad: verify failure with no detail
verify failed

# Good: label-honesty mismatch, fully diagnosable
verify failed: 'terraform-plan-review' tree SHA mismatch.
  locked:  a83b…  (claims v1.4.0)
  on-disk: c91f…
→ The vendored content does not match the version it claims to be.
→ To restore the approved version: 'skillrig add terraform-plan-review --force'
→ If this edit is intentional, it's a local modification — commit it; 'skillrig bump' (planned) will 3-way merge it on the next upstream advance.
```

**Auth as a distinct failure (R18) — the most common footgun.** A missing backing CLI and an auth failure fetching it are different problems. Never collapse them:

```
# Bad: looks like a missing tool, but it's auth
doctor failed: oxid not found

# Good: distinguish missing-tool from can't-authenticate-to-fetch-it
doctor: backing CLI 'oxid' (>=0.4.0), required by 'terraform-plan-review', is not installed.
→ It's a private CLI from cdktn-io/oxid; install via mise: 'mise use github:cdktn-io/oxid'.
→ If install fails with 403/404, this is AUTH, not a missing tool: check 'gh auth status' / GITHUB_TOKEN reachability.
```

**Rule**: Every CLI error MUST include:
1. What failed (operation name + git/exit context)
2. Why it failed — the **actual** error, not a guess. Never swallow the raw error.
3. Suggested fix based on common causes
4. Escape hatch to see more detail (e.g., `--verbose` flag)

> **stderr is the information agents need most, precisely when commands fail. Never drop it.**

All error/warning output goes to stderr. All data output goes to stdout. This enables clean piping: `skillrig search --json 2>/dev/null | jq ...`

Never replace the actual error with a guess about the cause. The guidance should *supplement* the raw error, not *replace* it. If an agent sees "auth token expired" but the real problem was a missing backing CLI, it will waste cycles re-authenticating instead of installing the tool.

### Desire Paths

Errors are also a signal about how agents *want* to use the CLI. The concept of [desire paths](https://en.wikipedia.org/wiki/Desire_path) — trails worn by users taking shortcuts — applies to CLI design:

- If agents repeatedly try `skillrig install <skill>` instead of `skillrig add <skill>`, that failed path is a desire path. Consider paving it with an alias or shorthand.
- If agents reach for a non-existent `skillrig publish`, the error should redirect to the GitHub-native reality ("publishing = open a PR to the origin"), since the consume-only surface is a deliberate design choice (architecture §2b), not an omission.
- If an error pattern repeats across multiple agent sessions, consider whether the failed path should become a real command instead of just redirecting.

> **Future**: Systematic desire path detection requires logging agent CLI usage patterns. The architecture's no-telemetry default (N1) means this stays opt-in if ever added.

---

## Principle 3: Two-Level Output Design

### Human Output (default): Compact and Budget-Conscious

Human output is designed for quick scanning — truncated previews, counts instead of nested data, footer hints for next steps.

```
$ skillrig search terraform --topic aws
terraform-plan-review | v1.4.0 | my-org | Review a terraform plan for risk and drift. | requires: oxid, terraform
terraform-cost        | v0.9.0 | my-org | Estimate the cost delta of a plan.          | requires: terraform

2 skill(s) match 'terraform' (topic: aws)
→ Use 'skillrig add <skill>' to vendor one, or 'skillrig search <name> --json' for the full catalog entry
```

`search` is **query-first**: `search [QUERY...]` is a case-insensitive token-AND substring match over `name` + `description` + `topics`, with `--topic` as a separate exact-string filter (repeatable). The result order is deterministic (N6) — a fixed relevance bucket (exact-name > name > topic > description match) then lexicographic by name. There is **no** fuzzy / semantic / TF-IDF ranking. An empty result is a **clean exit 0** (not an error) with a footer hint.

**Truncation rules** (human output only):
- Description: first 80 chars, append `...` if truncated
- Newlines replaced with spaces
- `requires` shown as a comma-joined tool list, not the full constraint set

**Footer hints**: Every compact output MUST end with a hint line suggesting the drill-down command. This is the agent's navigation cue.

### JSON Output (`--json`): Complete and Pipeable

JSON output includes full, untruncated data — the complete catalog entry (`name`, `version`, `namespace`, `description`, `topics[]`, `path`, and any `requires` summary), full lock entries, full `SKILL.md` frontmatter. The consumer (agent or `jq` pipe) decides what to extract. No truncation, no previews.

```bash
# Agent workflow: scan the index, then selectively drill down into one entry
skillrig search terraform --topic aws --json | jq '.[].name'           # scan names
skillrig search terraform-plan-review --json | jq '.[0].requires'      # drill into prereqs
```

**Rule**: JSON output is the "execution layer" — complete, structured, pipeable. Human output is the "presentation layer" — budget-conscious, hinted. Token efficiency is achieved by the *workflow pattern* (search → inspect), not by truncating JSON.

### Exit Codes

Exit codes are **load-bearing** here, not cosmetic: `verify` and `lint` are deterministic gates run in CI (R11). The same recompute that CI trusts must surface a stable exit code. Distinguish failure classes so a CI step (and an agent) can branch on *why* the gate failed (open Q5 in architecture.md):

| Code | Meaning |
|------|---------|
| 0 | Pass (including empty results) |
| 1 | Usage / config error (bad args, no origin configured) |
| 2 | Verification failure — label-honesty mismatch, orphan (on-disk ≠ locked), or unresolved conflict markers. **Active**: emitted by the implemented `verify`. |
| 3 | Prerequisite failure — a `[[requires]]` tool missing/unsatisfied or unauthenticated (fail in CI; may warn for humans). **Reserved**: not emitted today; lands with `doctor` (`verify` is integrity-only). |

No duration metadata — `verify`/`search`/`lint` run offline against committed files; there is no per-command network cost to report.

### Binary Output

Not applicable — the `skillrig` CLI produces structured text/JSON only. Skill content and backing binaries are materialized on disk by `add`/`global add`; commands reference **paths**, never emit binary content.

---

## Standard Flags

A small set of flags carry the same meaning across every command, so an agent can transfer them without re-checking `--help`:

| Flag | Applies to | Meaning |
|------|-----------|---------|
| `--json` | all | Complete, untruncated, pipeable output (see [Two-Level Output](#principle-3-two-level-output-design)). |
| `--verbose` | all | The escape hatch: print the raw underlying git / mise / exec output behind a friendly error or summary. This is the "see more detail" promised by the error-message rule — it must actually exist, on every command, not just be referenced. |
| `--dry-run` | mutating commands (`add`, `bump`, `global add`) | Compute and print the tree + lock changes that *would* be written, then exit without touching disk or opening a PR. Preview before any destructive write. |
| `--force` | `add` / `update` | Required to overwrite on-disk content that diverges from the locked `treeSha`. Without it, a content mismatch is a refusal with guidance, not a silent clobber — mirroring the content-comparison-on-write UX validated in architecture §9b. |

`--force` and the verify-time label-honesty check are two sides of one rule: divergent content is never written or accepted silently. `--force` is the *human's* deliberate override at write time; `verify` is the *gate's* refusal at check time.

A few command-specific flags carry consistent meaning where they apply:

| Flag | Applies to | Meaning |
|------|-----------|---------|
| `--version` | root (the binary itself) | Print build provenance as a single compact line — `skillrig <version> (commit <sha>, built <date>)` — then exit `0`. Cobra supplies the flag from the root command's `Version`; there is **no** `version` subcommand. Values are injected at release time via `-ldflags -X` (GoReleaser targets `internal/cli`'s `version`/`commit`/`date` package vars); a local `go build` reports the defaults `dev`/`none`/`unknown`. Two-level-output-compliant: one human line, while the SHA + date keep it fully reproducible/traceable to a release. |
| `--pin <ref>` | `add` | Vendor a specific **immutable** version of the skill rather than the origin-branch tip. A bare `^v?SEMVER$` value expands via the origin's `tag_scheme` (e.g. `v1.4.0` → `terraform-plan-review-v1.4.0`); any other value is treated as a literal git ref/SHA. The lock records the resolved `commit` + `treeSha` + the resolved human-readable `version`/tag, so re-acquisition is byte-identical and humans can still reason about versions. A pin that resolves to no ref is a distinct `NoSuchVersionError`, **not** "not found". See [Pin vs. branch ref](#origin-reference-grammar). |
| `--topic <T>` | `search` | Repeatable exact-string filter applied **after** the free-text `[QUERY...]` match — narrows results to catalog entries carrying topic `<T>`. It is `--topic` (not `--filter`/`--tag`): the catalog field is `topics[]`, renamed from `tags` to avoid colliding with git-tag/version-pin terminology. |

---

## Origin Reference Grammar

The origin reference — the value of `--origin`, `SKILLRIG_ORIGIN`, and the `origin` key in `.skillrig/config.toml` — is `OWNER/REPO[@REF]`. The `@REF` suffix is **optional**: omitted, the origin tracks the library's default branch; supplied, it tracks a specific **branch** (e.g. a staging line of the skills library). Validation is **shape-only and offline** — like the rest of `init`, it never checks that the repo or ref actually exists (that is a future `doctor`/`verify`/`add` concern).

```
skillrig init --origin my-org/my-skills            # default branch
skillrig init --origin my-org/my-skills@staging    # track the 'staging' branch
```

This realizes the `@ref` half of the ecosystem-standard identity grammar `OWNER/REPO[/path]@ref` (architecture R26) that `gh skill` (`gh skill install github/awesome-copilot documentation-writer@v1.2.0`) and Vercel `npx skills` use. The `[/path]` portion remains future work.

**Two meanings of `@ref`, kept distinct.** For an **origin**, `@REF` is a *moving pointer* — a branch you track and re-resolve. For a **skill** vendored via `add` (`skillrig add <skill> --pin <ref>`, **shipped** with the remote-fetch path), the ref is an *immutable* pin — a tag or commit SHA, recorded in the lock so the vendored content is reproducible. Same grammar, opposite intent: the origin says "where to look (and which line of development)"; the pin says "exactly which reviewed bytes." Docs and help text must not conflate them.

#### Pin vs. branch ref

`--pin <ref>` resolves in two steps so the common case (a version) is ergonomic and the escape hatch (any git ref) still works:

- A **bare semver** (`^v?SEMVER$`, e.g. `1.4.0` or `v1.4.0`) is expanded through the origin's `tag_scheme` (`name-vSEMVER`) to the per-skill tag — `--pin v1.4.0` on `terraform-plan-review` resolves to the tag `terraform-plan-review-v1.4.0`. The fully-qualified tag is also accepted.
- **Anything else** is treated as a **literal git ref or commit SHA** and fetched verbatim.

A pin that resolves to no ref is a distinct `NoSuchVersionError` (exit 1) — deliberately *not* the same as `NotFoundError` (origin/skill missing), so the agent sees "that version doesn't exist" rather than "that skill doesn't exist". The lock records the resolved `commit` (provenance) + `treeSha` (label-honesty) + the resolved human-readable `version`/tag. The origin publishes **no per-skill tree-SHA** (the catalog is discovery-only), so label-honesty here means "the on-disk content still matches what was vendored at this commit," anchored by provenance — not "matches an origin-attested hash."

### Why a single `@ref` string, not a separate flag

The branch rides *inside* the one origin string rather than in a separate `--branch`/`--ref` flag, and is stored combined in the single `origin` config key (`origin = 'my-org/my-skills@staging'`). Rationale:

- **One key, three consumers (R26).** The same reference is the key for config, `index.json` rows, and (later) allowlist/lock entries. A single canonical string keeps those aligned; splitting owner/repo from ref into parallel fields invites drift and a "what wins if both are set?" ambiguity.
- **Ecosystem familiarity.** `@ref` matches `gh skill`, `npx skills`, npm (`pkg@version`), and Go modules (`mod@version`) — an agent transfers the form without re-reading `--help`.
- **No new flag surface.** `--origin` stays the single way to name an origin; the grammar carries the optional precision. (A `#`-separator — git/npm git-dep style — was considered and rejected for weaker ecosystem alignment with the R26 grammar already adopted.)

## Pattern Classification

Every `skillrig` subcommand MUST identify which pattern(s) it follows. This classification drives design constraints and review expectations. See the [pattern-gate checklist](checklist-template.md) for the per-command gate used at PR review time.

| Pattern | Purpose | Examples | Constraints |
|---------|---------|----------|-------------|
| **Query** | Deterministic read of the discovery artifact | `search` *(implemented)* | Reads the origin's `index.json` (fetched per call — no offline cache this slice; an unreachable origin is the `UnreachableError`). Query-first: deterministic token-AND substring over `name`+`description`+`topics` + exact `--topic` filter; fixed relevance-bucket then lexicographic order — **no inference / no fuzzy ranking** (N6). Empty result = clean exit 0. Gates the origin's `skillrigConvention` before reading. |
| **Vendor Mutation** | Write skill tree + lock entry | `add` *(implemented — local + remote)*, `bump --pr` | Writes lock via `skillcore` only. Serves a **local-path** origin (read a checkout) and a **remote** `OWNER/REPO` origin (fetch the subtree over `git`, token via `os.exec` of `gh`/`git`, never a write credential) — the two origin forms are classified, never "both-present". `--pin` vendors an immutable version. Supports `--dry-run`; refuses to clobber content that diverges from the locked `treeSha` without `--force`. `bump` *proposes* (opens a PR), never force-adopts (R13). MUST never silently discard local edits (R32). Vendors byte-identical + mode-preserving; the skill name MUST be a single path segment (no traversal); **path-traversal + symlink guards apply to remotely-fetched content too**. **Symlinks in a skill subtree are rejected this slice** — following them would break byte-identical / git-canonical vendoring (git records a symlink as a link, not its target); preserving symlinks faithfully is a future relaxation. |
| **Verification Gate** | Offline integrity / prereq / conformance | `verify` *(implemented — integrity-only)*, `lint` | MUST be offline + deterministic. Exit-code driven. **No live/online signal in this path** (R11/N1). `verify` = consumer CI gate; `lint` = author CI gate on the origin. As implemented, `verify` is **integrity-only** (label-honesty + orphan detection, exit 2); prerequisite/eligibility checks (a missing `requires` tool → exit 3) belong to the future `doctor`, so `verify` does not emit exit 3 today. |
| **Environment** | Health, auth, config, bootstrap | `doctor`, `init` | MUST be idempotent. `doctor` checks prerequisite auth (R18); works without a fully-configured project. `init` is **consumer-side only** — binds to an *existing* origin, never bootstraps one (architecture §2d). |
| **Global Management** | Fetch/restore user-scope skills | `global add`, `global verify` | Genuinely *fetches and materializes* (the restore mode project scope doesn't need, §3). Touches per-environment home dirs, never the repo's project lock (R8). |

> **Origin-side generator — `index` (not a consumer pattern).** `skillrig index` is the only command that runs **inside the origin repo** (in its `index.yml` CI on merge to `main`), not against a consumer's vendored tree. It walks `skills/*/SKILL.md`, parses each via the **same** `skillcore.ParseManifest` the consumer commands use (AP-04), and emits/marshals `index.json` — the catalog `search` consumes. It is **not** one of the five consumer patterns above: it produces the discovery artifact rather than reading or vendoring it. It is still consume-only in the credential sense — no auth, local-filesystem only — so it does not breach AP-05. Constraints: deterministic full-regenerate output (no append/aggregation/GC — single-tip catalog); the producer's output MUST equal the committed `index.json` (a ground-truth contract test); and it MUST **fail clearly** (exit 1) on a skill missing its required `x-skillrig.version` rather than silently under-emitting. Exit codes `0`/`1` only.

### Failure Mode Constraints

Each pattern has a distinct failure mode expectation:

| Pattern | Failure Mode |
|---------|-------------|
| **Query** | MUST fail with clear error + suggested fix (no origin → run `init`; unreachable/auth/incompatible-convention fetching the catalog → the matching typed error). An **empty match set is success (exit 0)**, not a failure — it prints a footer hint, not an error. |
| **Vendor Mutation** | MUST validate origin + auth before fetching. Three-way-merge conflict → non-zero exit, write git-style conflict markers, instruct resolve-and-rerun (architecture §5b). Never discard local edits. |
| **Verification Gate** | MUST be deterministic pass/fail by exit code. Label-honesty mismatch = fail (exit 2); orphan = fail (exit 2); unresolved conflict markers = fail. Prereq miss (exit 3) is reserved for the future `doctor` — the implemented `verify` is integrity-only and does not emit it. |
| **Environment** | MUST be idempotent and safe to retry. MUST distinguish "tool missing" from "tool exists but unauthenticated" (R18). |
| **Global Management** | MUST fetch/restore what's missing and report drift between the global lock and what's materialized. |

### Offline Behavior

The product promise — "the skill your agent runs is exactly the version that was reviewed and approved" — rides on `verify` being **offline and deterministic** (architecture §2c, R11). Honor the split:

- **Offline always** (`verify`, `lint`, `index`): operate on committed `skills-lock.json` + the git tree on disk (`verify`/`lint`) or the origin's local `skills/` tree (`index`). The project tree is in git, so there is no "restore from lock" — `verify` only *checks* (§3). Fully offline; `index` shells `git` only to compute tree-SHAs of a local tree, never the network.
- **Network when fetching** (`add`, `search`, `bump --pr`, `global add`): reach the origin / git to vendor content or read the catalog. `search` fetches the origin's `index.json` **per call** (no offline cache this slice — a deliberate freshness choice, D-catalog-fetch); `add` fetches the skill subtree. MUST fail with the matching typed error (`UnreachableError` / `AuthError` / `NotFoundError`) when the origin can't be reached. *(A **local-path** origin makes `add`/`search` operate on a checkout with no network — the origin-form classification, not a cache.)*
- **Auth-aware** (`add`, `search`, `doctor`): resolve a read token via `os.exec` (`GH_TOKEN` env > `GITHUB_TOKEN` env > `gh auth token --hostname <host>`), inject it into the fetch as a `http.extraHeader` Basic credential passed through the **process ENVIRON** (`GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_i`/`GIT_CONFIG_VALUE_i`, git ≥2.31) — **not** an argv `-c http.extraHeader=…` flag and **not** the clone URL — so the base64 credential never appears in `ps`/process listings or error output, and surface auth as its own actionable failure distinct from unreachable/not-found (R18). The token is a **read-only fetch** credential — there is still no write credential in the binary (AP-05). **Every `git` child runs NON-INTERACTIVELY** (`GIT_TERMINAL_PROMPT=0` + `GIT_ASKPASS=""` + `GCM_INTERACTIVE=Never`, applied unconditionally — even when no token is resolved): a missing/rejected credential must fail fast as an `AuthError`, never block on a prompt, so a no-TTY CI job cannot hang and the user never sees a raw "could not read Username" / "Device not configured" artifact (issue #25). The empty `GIT_ASKPASS` is load-bearing, not redundant: git consults the askpass program *before* the terminal, so `GIT_TERMINAL_PROMPT=0` alone does **not** stop a prompt when the environment exports an askpass GUI (e.g. **VS Code's integrated terminal**) — inheriting it would pop a dialog. A set-but-empty `GIT_ASKPASS` short-circuits the whole askpass chain (`core.askpass`/`SSH_ASKPASS` too) and overrides the inherited value. Credential *helpers* are left intact (a separate, non-interactive mechanism) — only the interactive fallbacks are removed.

---

## Anti-Patterns

Common implementation deviations to avoid.

### AP-01: Full manifest in compact list output
```go
// Wrong: dumps the full description + every requires constraint in human output
fmt.Printf("%s | %s\n", s.Name, s.Description)  // 500-char description

// Right: truncate description, summarize requires in human output
desc := truncate(s.Description, 80)
fmt.Printf("%s | %s | requires: %s\n", s.Name, desc, joinTools(s.Requires))
```

### AP-02: An online or inferential check inside the verify path
```
# Wrong: verify reaches out to compare against the origin's latest (an UPDATE check),
#        or consults a live risk score — breaks the offline, deterministic CI gate.

# Right: verify recomputes the git tree SHA of the on-disk subtree and compares it
#        to the locked treeSha — label-honesty, fully offline (R9, architecture §4.2).
#        "Did upstream move?" belongs in 'bump', not 'verify'. Risk scores are
#        advisory, human-facing, online-only — never in verify (R29).
```

### AP-03: Swallowing raw errors behind guidance
```go
// Wrong: raw error dump with no guidance
return fmt.Errorf("git error: %s", stderr)

// Also wrong: guidance that REPLACES the actual error
return fmt.Errorf("add failed: you must run 'gh auth login'")
// ↑ The real error might be a wrong origin, not auth — now the agent debugs the wrong thing

// Right: preserve the raw error + add guidance based on common causes
return fmt.Errorf("add failed: %s\n→ Check the origin: 'cat .skillrig/config.toml'\n→ If the repo is private, see 'gh auth status' / GITHUB_TOKEN", stderr)
```

### AP-04: A parallel tree-SHA / manifest-parse / fetch / catalog implementation
```go
// Wrong: bump computes the tree SHA one way, verify recomputes it another way.
//        They drift, and the value CI writes during bump no longer matches what
//        verify recomputes — the integrity guarantee silently rots.
sha := myLocalTreeHash(dir)            // in bump
sha := someOtherHash(dir)              // in verify

// Right: exactly ONE implementation, in the public pkg/skillcore package
//        (SDK-1), called by add, verify, AND future bump/doctor. Make it a
//        hard boundary (architecture §2: "the two interfaces cannot diverge").
sha := skillcore.TreeSHA(dir)
```
The same single-implementation rule extends to every shared primitive 003 adds: **one** remote-fetch impl, **one** `ParseManifest` (the `SKILL.md` frontmatter reader), **one** catalog parse/generate (`search` reads what `index` writes), and **one** search matcher — all in `pkg/skillcore`. `index` generating a catalog `search` can't parse, or `add` fetching a way `verify` can't reproduce, is the same drift this anti-pattern forbids.

### AP-05: Baking the origin into the binary, or adding a write credential
```
# Wrong: a per-org compiled binary, or 'skillrig publish' / 'skillrig login' that
#        carries a registry write credential.

# Right: one generic binary for all orgs (R4); the origin is resolved at runtime
#        from consumer config (§2d). Publishing = a PR to the origin; GitHub is the
#        authority plane (architecture §2b). The CLI surface is purely consumer-side.
```

### AP-06: Re-deriving the origin per command
```go
// Wrong: each command reads .skillrig/config.toml (or the env var) on its own,
//        so precedence drifts — one command honors SKILLRIG_ORIGIN, another forgets
//        the global default, a third hard-codes the project file.
origin := readToml(".skillrig/config.toml").Origin   // in add
origin := os.Getenv("SKILLRIG_ORIGIN")               // in search

// Right: exactly ONE resolver implements the env > project config > global default
//        precedence (§2d); every command calls it. Same single-implementation rule
//        as AP-04 — a contractor targeting client-A's origin in one repo and
//        client-B's in another must get identical resolution everywhere.
origin, err := config.ResolveOrigin(cwd)   // env > .skillrig/config.toml > global
```

---

## Architecture: Execution vs Presentation

Inside the CLI, there are two conceptual layers:

```
┌─────────────────────────────────────────────┐
│  Presentation: LLM/Human-facing output      │  ← --json (full) vs human (compact + hints)
│  Truncation | Footer hints | stderr/stdout  │
├─────────────────────────────────────────────┤
│  Execution: Go business logic               │  ← Cobra routing; skillcore (tree SHA,
│  skillcore | fetch | catalog | lock I/O     │     frontmatter parse, fetch, catalog, search);
└─────────────────────────────────────────────┘                                lock R/W
```

The execution layer handles command routing, the shared `skillcore` primitives (tree-SHA computation, `SKILL.md` frontmatter / lock parsing, remote fetch, catalog parse/generate, the search matcher), index comparison for `bump`, and lock I/O. The presentation layer formats output for the consumer (human or agent). The presentation/execution split itself is a design concern within each command's `runXxx()` function — but the integrity primitives are **not** inline: `skillcore` is a separate, importable **public package** (`pkg/skillcore`, per SDK-1), so third-party Go tools can build their own `add`/`verify` on the same primitives the CLI uses.

**Key rule**: Execution logic must not depend on output format. The same data path serves both `--json` and human output. And per AP-04, there is exactly one `skillcore` implementation of the integrity primitives — the public `pkg/skillcore` package — that `add` and `verify` (and future `bump`/`doctor`) all dispatch to, so the gate can never diverge from what CI wrote. If an MCP surface for agents is ever added, it dispatches to `pkg/skillcore` too — never a parallel implementation (architecture §2).

---

## Borrowed Ideas (vNext, additive — not yet adopted)

Harvested from the `agentic-cli-design` skill (tumf/skills). These are **additive** enhancements that this document does not currently mandate; they are recorded here so the idea isn't lost, grounded in our own authoritative contract. **This document remains authoritative.** Where the external skill conflicts with the rules above it was deliberately **not** adopted — specifically: our exit codes are `0/1/2/3` per [Exit Codes](#exit-codes) (not the skill's `2=invalid-args/3=auth/4=retryable`); **human compact output is the default** with `--json` opt-in (not JSON-primary, see [Two-Level Output](#principle-3-two-level-output-design)); and **errors are prose what/why/fix to stderr with the raw cause preserved** (not structured JSON errors, see [Principle 2](#principle-2-error-messages-as-navigation)).

- **Machine-readable introspection beyond help text.** Today Progressive Discovery ([Principle 1](#principle-1-progressive-discovery)) is help-text based (Cobra-generated). A future enhancement could let the CLI emit its own spec for agents to parse: `commands --json` (command/arg tree), `schema --command <c> --output json-schema` (per-command JSON Schema), `--help --json`, and top-level fixed fields `schemaVersion` / `type` / `ok`. Pull this forward only if agents need to parse the command tree programmatically (MCP-surface adjacency, architecture §2).
- **`install-skills` verb.** The CLI could ship *its own* usage skill from `<project>/skills` into `.agents/skills` (or `--claude` → `.claude/skills`, `--global` → home). This is a natural mechanism for **Constitution IX (Skill–CLI Co-Evolution)** — skillrig installing the agent skill that teaches its own usage. Note this is the CLI's *own* skill, distinct from skillrig's core job of vendoring *org* skills; keep the two surfaces clearly separated and consume-only (§2b).
- **Agent-friendliness scorecard (0/1/2 per principle, 14 max).** A lightweight rubric usable as an extra review gate during `/specledger.checkpoint` to score a command's agent-readiness. Advisory only — it never becomes a blocking CI gate (that role belongs to `verify`/`lint`, R11/N1).

## References

- [Technical Architecture](../../../../architecture.md)
- [Requirements (durable contract)](../../../../requirements.md)
- [Pattern-Gate Checklist](checklist-template.md)
- [External: Manus CLI design post](https://www.reddit.com/r/LocalLLaMA/comments/1rrisqn/i_was_backend_lead_at_manus_after_building_agents/)
- [External: Rewrite your CLI for AI agents](https://justin.poehnelt.com/posts/rewrite-your-cli-for-ai-agents/)
- [External: Desire Paths (Wikipedia)](https://en.wikipedia.org/wiki/Desire_path)
