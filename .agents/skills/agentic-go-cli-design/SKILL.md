---
name: agentic-go-cli-design
description: >-
  Design principles for Go CLIs (Cobra) that are consumed by AI agents, humans, and CI alike.
  Use this skill whenever you are adding or reviewing a Cobra command, subcommand, flag, help
  text, error message, or output format in a Go CLI.
  Apply it for anything touching `--help` text, error wording, `--json` vs human output,
  exit codes, or standard flags (`--json`/`--verbose`/`--dry-run`/`--force`), even when the
  user only says "add a command", "improve this error", or "make the CLI output nicer". The
  goal is one-shot agent success from `--help` alone.
---

# Agentic Go CLI Design

A CLI consumed by agents, humans, and CI is a contract: it must be self-documenting,
token-efficient, and navigable without external docs. An agent that can only learn the tool
from `--help` should still get **one-shot success**. These principles are adapted for Go +
[Cobra](https://github.com/spf13/cobra) CLIs.

For the full reference — worked examples, the pattern-classification table, and the complete
anti-pattern catalogue — read [`references/cli-design.md`](references/cli-design.md). Load it
when you need concrete before/after error wording or are classifying a command's design pattern.

## Rules of Thumb

Every subcommand should satisfy these:

1. **Every subcommand has a `Long` description with ≥2 `Examples`** in its Cobra definition.
   The agent must be able to construct a correct command from `--help` alone.
2. **Every error suggests a fix** — what failed, why (the *real* error), and what to run instead.
3. **Human output is compact** — truncated previews, counts, and a footer hint for next steps. Large
   or unbounded nested detail goes to per-item files on disk (+ an `index.json`), not stdout.
4. **JSON output is complete** — full data, pipeable to `jq`, never truncated.
5. **Errors to stderr, data to stdout** — so `cmd --json 2>/dev/null | jq ...` stays clean.
6. **Positional args for the simple case**; reserve flags for optional/complex params.
7. **Standard flags everywhere**: `--json` and `--verbose` on every command; mutating commands
   also take `--dry-run` and refuse to clobber divergent state without `--force`.

## Principle 1: Progressive Discovery

`--help` should reveal the tool layer by layer, so the agent loads only what it needs:

- **Level 0** — bare command prints the subcommand list (Cobra does this for free).
- **Level 1** — `cli <command>` with no args prints that command's usage.
- **Level 2** — `cli <command> <subcommand>` with missing args prints the specific params + examples.

This beats stuffing thousands of words of docs into a system prompt — most of it is irrelevant
most of the time. Progressive help lets the agent decide when it needs more. The design
requirement that falls out: **every command and subcommand must have complete help output.**

## Principle 2: Error Messages as Navigation

Agents can't Google. Every error must carry both *what went wrong* and *what to do instead*,
**without swallowing the raw error**. Guidance *supplements* the real error, it never *replaces* it
— if you replace a "wrong origin" error with "run auth login", the agent debugs the wrong thing.

Every CLI error should include:
1. What failed (operation + the underlying git/exec/exit context).
2. Why — the **actual** error, preserved verbatim. Never guess in place of it.
3. A suggested fix based on common causes.
4. An escape hatch for more detail (`--verbose`).

Distinguish failure classes that look alike — most importantly, *"tool missing"* vs *"tool present
but unauthenticated"*. Collapsing them sends the agent down the wrong path.

```go
// Wrong: raw dump, or guidance that REPLACES the cause
return fmt.Errorf("git error: %s", stderr)
return fmt.Errorf("add failed: run 'gh auth login'") // the real cause may be a bad path, not auth

// Right: preserve the raw error + add cause-based guidance
return fmt.Errorf("add failed: %s\n→ Check the config: 'cat .../config'\n→ If private, see 'gh auth status' / GITHUB_TOKEN", stderr)
```

**Desire paths**: repeated wrong guesses are signal. If agents keep typing `install` instead of
`add`, pave it with an alias. If they reach for a command that doesn't exist by design, the error
should redirect to the intended reality, not just say "unknown command".

## Principle 3: Two-Level Output

- **Human (default)** — compact and budget-conscious: truncate descriptions (~80 chars, newlines →
  spaces), show counts not nested data, and **end every compact output with a footer hint** that
  names the drill-down command. That footer is the agent's navigation cue.
- **`--json`** — complete, structured, pipeable; no truncation. The consumer (agent or `jq`)
  decides what to extract. Token efficiency comes from the *workflow* (search → inspect), not from
  truncating JSON.

**When the detail is large or unbounded, add a third tier: write it to disk.** Some commands produce
data whose volume scales with the *work*, not the command — a per-item audit, a per-file scan, a
fan-out across many targets. Streaming all of that to stdout (even as `--json`) floods the agent's context
the moment the set grows. Instead, write the full nested detail to **files on disk** — one file per
item plus an `index.json` roll-up mapping each item to its detail file — and keep stdout to the
compact summary plus a footer hint that names the directory. The agent then `grep`/`sed`/`jq`s only
the one file it needs and never pays the token cost of the rest. It's the same scan → inspect workflow
as `--json`, except the inspect step is a cheap file read instead of re-running the command. **Default
to writing the artifacts** (don't hide them behind an opt-in flag) when producing that detail is the
command's whole point, and print where they went. A single combined blob (`--out audit.json`) is for
the *audit-trail* case — it is not a substitute for per-item files, because the agent still has to load
the whole thing to find one record.

**Exit codes are load-bearing**, especially for gate commands run in CI. Distinguish failure
classes so a CI step (or agent) can branch on *why* it failed, e.g.:

| Code | Meaning |
|------|---------|
| 0 | Pass (including empty results) |
| 1 | Usage / config error (bad args, missing config) |
| 2 | Verification failure (integrity / mismatch / unresolved conflict) |
| 3 | Prerequisite failure (required tool missing or unauthenticated) |

Keep one execution path feeding both `--json` and human output — **execution logic must not depend
on output format**. And keep shared primitives (hashing, parsing, config resolution) in exactly one
internal package that every command calls, so the gate can never diverge from what produced it.

## Anti-Patterns to Avoid

- Dumping the full manifest/description in compact human output (truncate + summarize instead).
- Streaming unbounded nested data (per-item findings, scan results) to stdout when it should be
  written to per-item files on disk + an `index.json` — or collapsing the drill-down into a single
  `--out` blob the agent must load whole. Write per-item artifacts and let the agent grep/jq one.
- Swallowing the raw error behind friendly guidance (preserve it).
- Parallel implementations of a core primitive in two commands — they drift. One implementation,
  one internal package, every command dispatches to it.
- Re-deriving config/precedence per command — one resolver, called everywhere.

See [`references/cli-design.md`](references/cli-design.md) for the full anti-pattern catalogue with
Go code examples, the pattern-classification matrix, and offline-behavior rules.
