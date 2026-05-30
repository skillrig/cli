---
name: skillrig
description: >-
  Point a repository at your org's agent-skills library and manage vendored skills with the
  `skillrig` CLI — bind/choose the origin (`init`), vendor/add a skill (`add`), and
  verify/check that committed skills are exactly what was approved (`verify`). Use whenever
  the user wants to find, install, add, vendor, pull in, lock, or pin an agent skill from a
  skills library; set/configure where skills come from or fix a "no origin configured" error;
  point a repo at a skills repo (OWNER/REPO[@branch]) or use SKILLRIG_ORIGIN; or verify /
  check / audit that vendored skills haven't been tampered with (a CI gate) and debug
  add/verify errors or `mismatch`/`orphan`/`missing`/`dirty` verdicts. Vendoring requires an
  origin, so `skillrig init` comes first. Trigger even when the command isn't named — e.g.
  "point this repo at our skills", "pull in the terraform-review skill", "make sure nobody
  changed our skills", "why did the skills check fail in CI", "our agent skill got edited".
license: MIT
metadata:
  author: skillrig
  cli: skillrig
  user-invocable: true
---

# skillrig

`skillrig` is a single, generic, **consume-only** CLI for pointing a repo at an **origin** —
the `OWNER/REPO` that hosts your org's agent skills — and managing the skills vendored from
it. The same binary serves humans, agents, and CI. There is no publish/login: GitHub is the
authority plane ("publishing" = a PR to the origin).

**The promise:** *the skill your agent runs is exactly the version that was reviewed and
approved.* `add` records a tamper-evident git tree-SHA when a skill is vendored; `verify`
recomputes it and fails if it drifted — same primitive on both sides, so the gate cannot lie.

## When to use this skill

Use it whenever the user wants to **find / install / add / vendor / pull in** an agent skill
from a library, **set or fix the origin** ("point this repo at our skills", "no origin
configured"), or **verify / check / audit** that vendored skills are unmodified (a CI gate),
including debugging `add`/`verify` output. Three activities, three commands:

| Activity | Command | Read |
|---|---|---|
| Choose where skills come from (bind the origin) | `skillrig init` | [references/init.md](references/init.md) |
| Vendor a skill into the repo (+ lock its identity) | `skillrig add <skill>` | [references/add.md](references/add.md) |
| Prove vendored skills match what was approved | `skillrig verify` | [references/verify.md](references/verify.md) |

Load only the reference for the activity at hand.

## Prerequisite: an origin must be configured (run `init` first)

`add` needs to know **where** skills come from, so a configured origin is a precondition.
**Smoketest before vendoring:** is an origin resolvable?

- project: `.skillrig/config.toml` exists (at the git repo root), **or**
- env: `$SKILLRIG_ORIGIN` is set, **or**
- global: `~/.config/skillrig/config.toml` exists.

Precedence (highest wins): `SKILLRIG_ORIGIN` > project config > global. If none resolve,
`add` fails with `no origin configured` — run `skillrig init --origin OWNER/REPO` first
(see [references/init.md](references/init.md)). **`verify` needs no origin** (it reads the
committed lock + tree, offline).

## The typical workflow

```
skillrig init --origin my-org/my-skills      # 1. bind the origin (once per repo)
skillrig add terraform-plan-review           # 2. vendor a skill into .agents/skills/
git add -A && git commit -m "vendor skill"   # 3. commit (verify checks committed content)
skillrig verify                              # 4. prove it matches the recorded version (CI gate)
```

Exit codes are load-bearing for CI/agents: `0` ok · `1` usage/config · `2` verification
failure · `3` reserved (never emitted). Errors are what/why/fix on stderr; `--json` is the
complete machine view on stdout; `--verbose` shows the raw cause. Details per command in the
references.

## Not here yet

**Discovery / search is the next planned feature.** Listing or searching the *approved*
skills available in your configured origin (a `search`/index command) does **not exist yet** —
until it lands, vendor a skill by its **known name** with `add`. Note the scope: `skillrig`'s
"find" means *find an approved skill in **your origin***, which is distinct from the generic
`find-skills` skill (discovering skills from anywhere). So a request to "find/install a skill
from our library" is `skillrig`; "what skills exist out there for X?" is `find-skills`.

Also designed but **not implemented** (don't assume they exist): remote/network fetch + auth,
immutable per-skill `--pin`, multi-client symlink views, and a prerequisite/health `doctor`
(the reserved exit `3`). This slice is `init` + `add` (from a local origin checkout) +
`verify`.
