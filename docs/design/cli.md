# CLI Design Principles

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
7. **Origin is resolved, never baked in** — env > project config > global default (§2d of architecture.md); `--origin` overrides
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
  search   Query the origin's index.json for skills
  add      Vendor a skill into this repo + write the lock entry
  verify   Offline integrity + prereq check (exit code; CI gate)
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
Error: requires at least 1 arg(s), only received 0

Usage:
  skillrig add <skill> [--origin OWNER/REPO] [--pin <ref>] [--json]

Examples:
  skillrig add terraform-plan-review
  skillrig add terraform-plan-review --pin v1.4.0
```

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
→ To restore the approved version: 'skillrig add terraform-plan-review --pin v1.4.0'
→ If this edit is intentional, it's a local modification — commit it; 'skillrig bump' will 3-way merge it on the next upstream advance.
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
$ skillrig search --tag terraform
terraform-plan-review | v1.4.0 | my-org | Review a terraform plan for risk and drift. | requires: oxid, terraform
terraform-cost        | v0.9.0 | my-org | Estimate the cost delta of a plan.          | requires: terraform

2 skill(s) match tag 'terraform'
→ Use 'skillrig add <skill>' to vendor one, or 'skillrig search <name> --json' for the full manifest
```

**Truncation rules** (human output only):
- Description: first 80 chars, append `...` if truncated
- Newlines replaced with spaces
- `requires` shown as a comma-joined tool list, not the full constraint set

**Footer hints**: Every compact output MUST end with a hint line suggesting the drill-down command. This is the agent's navigation cue.

### JSON Output (`--json`): Complete and Pipeable

JSON output includes full, untruncated data — the entire `skill.toml` manifest, full `requires` constraints, full lock entries. The consumer (agent or `jq` pipe) decides what to extract. No truncation, no previews.

```bash
# Agent workflow: scan the index, then selectively drill down into one manifest
skillrig search --tag terraform --json | jq '.[].name'                 # scan names
skillrig search terraform-plan-review --json | jq '.requires'          # drill into prereqs
```

**Rule**: JSON output is the "execution layer" — complete, structured, pipeable. Human output is the "presentation layer" — budget-conscious, hinted. Token efficiency is achieved by the *workflow pattern* (search → inspect), not by truncating JSON.

### Exit Codes

Exit codes are **load-bearing** here, not cosmetic: `verify` and `lint` are deterministic gates run in CI (R11). The same recompute that CI trusts must surface a stable exit code. Distinguish failure classes so a CI step (and an agent) can branch on *why* the gate failed (open Q5 in architecture.md):

| Code | Meaning |
|------|---------|
| 0 | Pass (including empty results) |
| 1 | Usage / config error (bad args, no origin configured) |
| 2 | Verification failure — label-honesty mismatch, orphan (on-disk ≠ locked), or unresolved conflict markers |
| 3 | Prerequisite failure — a `[[requires]]` tool missing/unsatisfied or unauthenticated (fail in CI; may warn for humans) |

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

---

## Pattern Classification

Every `skillrig` subcommand MUST identify which pattern(s) it follows. This classification drives design constraints and review expectations. See the [pattern-gate checklist](checklist-template.md) for the per-command gate used at PR review time.

| Pattern | Purpose | Examples | Constraints |
|---------|---------|----------|-------------|
| **Query** | Deterministic read of the discovery artifact | `search` | Offline. Reads committed `index.json`. Deterministic tag filtering — **no inference** (N6). |
| **Vendor Mutation** | Write skill tree + lock entry | `add`, `bump --pr` | Writes lock via `skillcore` only. Supports `--dry-run`; refuses to clobber content that diverges from the locked `treeSha` without `--force`. `bump` *proposes* (opens a PR), never force-adopts (R13). MUST never silently discard local edits (R32). |
| **Verification Gate** | Offline integrity / prereq / conformance | `verify`, `lint` | MUST be offline + deterministic. Exit-code driven. **No live/online signal in this path** (R11/N1). `verify` = consumer CI gate; `lint` = author CI gate on the origin. |
| **Environment** | Health, auth, config, bootstrap | `doctor`, `init` | MUST be idempotent. `doctor` checks prerequisite auth (R18); works without a fully-configured project. `init` is **consumer-side only** — binds to an *existing* origin, never bootstraps one (architecture §2d). |
| **Global Management** | Fetch/restore user-scope skills | `global add`, `global verify` | Genuinely *fetches and materializes* (the restore mode project scope doesn't need, §3). Touches per-environment home dirs, never the repo's project lock (R8). |

### Failure Mode Constraints

Each pattern has a distinct failure mode expectation:

| Pattern | Failure Mode |
|---------|-------------|
| **Query** | MUST fail with clear error + suggested fix (e.g. no origin → run `init`). |
| **Vendor Mutation** | MUST validate origin + auth before fetching. Three-way-merge conflict → non-zero exit, write git-style conflict markers, instruct resolve-and-rerun (architecture §5b). Never discard local edits. |
| **Verification Gate** | MUST be deterministic pass/fail by exit code. Label-honesty mismatch = fail; orphan = fail; unresolved conflict markers = fail; prereq miss = fail in CI / may warn for humans. |
| **Environment** | MUST be idempotent and safe to retry. MUST distinguish "tool missing" from "tool exists but unauthenticated" (R18). |
| **Global Management** | MUST fetch/restore what's missing and report drift between the global lock and what's materialized. |

### Offline Behavior

The product promise — "the skill your agent runs is exactly the version that was reviewed and approved" — rides on `verify` being **offline and deterministic** (architecture §2c, R11). Honor the split:

- **Offline always** (`search`, `verify`, `lint`): operate on committed `index.json` / `skills-lock.json` and the git tree already on disk. The project tree is in git, so there is no "restore from lock" — `verify` only *checks* (§3). Fully offline.
- **Network when fetching** (`add`, `bump --pr`, `global add`): reach the origin / git to vendor or restore content. MUST fail with a clear error when the origin is unreachable.
- **Auth-aware** (`doctor`): explicitly probes `gh auth` / `GITHUB_TOKEN` reachability for private backing-CLI sources and reports auth as its own actionable failure (R18).

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

### AP-04: A parallel tree-SHA / manifest-parse implementation
```go
// Wrong: bump computes the tree SHA one way, verify recomputes it another way.
//        They drift, and the value CI writes during bump no longer matches what
//        verify recomputes — the integrity guarantee silently rots.
sha := myLocalTreeHash(dir)            // in bump
sha := someOtherHash(dir)              // in verify

// Right: exactly ONE implementation, in internal/skillcore, called by
//        verify, bump, AND doctor. Make it a hard internal boundary
//        (architecture §2: "the two interfaces cannot diverge").
sha := skillcore.TreeSHA(dir)
```

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
│  skillcore | index compare | lock I/O       │     manifest parse); index/ compare; lock R/W
└─────────────────────────────────────────────┘
```

The execution layer handles command routing, the shared `skillcore` primitives (tree-SHA computation, `skill.toml` / lock parsing), index comparison for `bump`, and lock I/O. The presentation layer formats output for the consumer (human or agent). These are not separate packages — they're a design concern within each command's `runXxx()` function.

**Key rule**: Execution logic must not depend on output format. The same data path serves both `--json` and human output. And per AP-04, there is exactly one `skillcore` implementation of the integrity primitives — `verify`, `bump`, and `doctor` all dispatch to it so the gate can never diverge from what CI wrote. If an MCP surface for agents is ever added, it dispatches to `skillcore` too — never a parallel implementation (architecture §2).

---

## References

- [Technical Architecture](../../../../architecture.md)
- [Requirements (durable contract)](../../../../requirements.md)
- [Pattern-Gate Checklist](checklist-template.md)
- [External: Manus CLI design post](https://www.reddit.com/r/LocalLLaMA/comments/1rrisqn/i_was_backend_lead_at_manus_after_building_agents/)
- [External: Rewrite your CLI for AI agents](https://justin.poehnelt.com/posts/rewrite-your-cli-for-ai-agents/)
- [External: Desire Paths (Wikipedia)](https://en.wikipedia.org/wiki/Desire_path)
