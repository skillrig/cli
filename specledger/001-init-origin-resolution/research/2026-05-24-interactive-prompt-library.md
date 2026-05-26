# Research: Interactive prompt library for the skillrig CLI

**Date**: 2026-05-24
**Context**: Spike S1 from [research.md](../research.md) D5. `skillrig init` needs exactly **one** interactive prompt today — ask for the origin (`OWNER/REPO`) when run interactively without `--origin`. Deciding stdlib-now vs adopt-`huh`-now before locking D5.
**Time-box**: short (~30 min). **Confidence**: high (binary sizes are real measurements on this machine).

## Question

Which interactive-prompt approach should `skillrig` use for v0 — stdlib `bufio`, `manifoldco/promptui`, `AlecAivazis/survey`, or `charmbracelet/huh` — given Constitution V (minimal deps / single static binary), VI (YAGNI), VII (shortest path to MVP), and IV (agent-first, non-interactive-safe)?

## Findings

### Finding 1: Binary-size cost (measured, Go 1.24.4 darwin/arm64, `-ldflags="-s -w"`, CGO off)

Each variant = a minimal Cobra command importing our real deps (`spf13/cobra` + `pelletier/go-toml/v2`) plus the prompt lib, built isolated and measured.

| Option | Binary size | Δ vs baseline | Transitive modules (go.sum `/go.mod` lines) |
|--------|-------------|---------------|----------------------------------------------|
| **baseline** (cobra + go-toml/v2; == stdlib `bufio`) | 2,704,354 B (2.57 MB) | — | 8 |
| **promptui** | 2,705,122 B (2.57 MB) | **+768 B (~0)** | 13 (+5) |
| **survey** v2 | 2,955,394 B (2.81 MB) | +251 KB (+0.23 MB) | 47 (+39) |
| **huh** | 3,347,714 B (3.19 MB) | **+643 KB (+0.61 MB)** | 47 (+39) |

stdlib `bufio` adds **zero** bytes and **zero** modules (it's already in the standard library). `huh` is the heaviest: +0.61 MB (~24% over baseline) and **+39 transitive modules** (bubbletea + lipgloss + bubbles + termenv + …). Surprisingly, `promptui` adds almost no binary size (+768 B) but still +5 modules.

### Finding 2: Cobra compatibility

All four run fine inside a Cobra `RunE` — they're invoked imperatively (`scanner.Scan()`, `promptui.Prompt.Run()`, `survey.AskOne()`, `huh.NewForm(...).Run()`), each blocks until the user answers, then returns control to `RunE`. `huh`/`bubbletea` take over the terminal (alt-screen / raw mode) for the duration but restore it on exit — no lifecycle conflict with Cobra. **No compatibility blocker for any option.** (Confidence: high for stdlib/promptui/survey; medium-high for huh — standard pattern, not re-verified end-to-end here.)

### Finding 3: Non-TTY / agent safety (the load-bearing dimension)

Per research D4, the prompt is reached **only** when value missing AND stdin is a TTY AND not `--non-interactive`. So in CI/agent/pipe contexts we never enter any library's prompt path — the library choice can't hang an agent **as long as the D4 gate is correct**. Defense-in-depth by option if the gate were ever bypassed:
- **stdlib `bufio.Scanner`**: on non-TTY/closed stdin, `Scan()` returns `false` immediately (EOF) — no hang, simplest failure mode. ✅ safest.
- **promptui / survey**: detect non-TTY and return an error (don't hang), but add their own TTY handling.
- **huh / bubbletea**: expects a real TTY; without one it errors rather than hangs, but it's the most terminal-machinery-heavy path. ⚠️ most moving parts in exactly the environment (agents) we care most about not breaking.

Net: stdlib is the least likely to ever block an agent — directly serves cli.md's non-interactive-default posture and architecture R3 (same binary for humans/agents/CI).

### Finding 4: Maintenance & API stability

- **stdlib**: Go team, never breaks. ✅
- **promptui** (`manifoldco`): low/with stale activity; works but quiet.
- **survey** (`AlecAivazis`): effectively **unmaintained** (community-archived sentiment); +39 deps for a stale lib — poor risk profile.
- **huh** (`charmbracelet`): **actively maintained** and at a **stable `v1.0.0`** (pulls `bubbletea v1.3.6`) — the API-churn worry is lower than assumed; heaviest dep tree, but a real v1 contract.

### Finding 6: huh API depth (from module-cache exploration, `go get` + `go doc` + source)

Read directly from `huh@v1.0.0` in the module cache. Details that matter for a *future* adoption decision and that correct earlier hedges:
- **Accessible mode** — `form.WithAccessible(true)` (and per-field `RunAccessible(w io.Writer, r io.Reader)`) **drops the Bubble Tea TUI renderer and uses basic terminal prompting**. The README gates it on an `ACCESSIBLE` env var. This means huh can degrade to plain prompts — friendlier for screen readers *and* less raw-TTY-bound, which softens (but doesn't remove) the non-TTY concern in Finding 3.
- **Injectable I/O** — `WithInput(io.Reader)` / `WithOutput(io.Writer)`, plus `RunAccessible(w, r)`, make huh forms **deterministically testable** (feed a `strings.Reader`) without a pty — compatible with our golang-testing approach.
- **Bounded execution** — `WithTimeout(d)` and `RunWithContext(ctx)` exist (note `ErrTimeout` / `ErrTimeoutUnsupported in accessible mode`, `ErrUserAborted`).
- **Rich field set** — `Input`, `Text`, `Select`, `MultiSelect`, `Confirm`, `FilePicker`, `Note`, plus `Group`/`Form` and grid `Layout`s. None of which v0's single origin prompt needs — reinforces Finding 5 (YAGNI now), but is exactly the payoff when a multi-field flow arrives.

### Finding 5: UX payoff vs YAGNI for v0

The entire v0 interactive surface is **one single-line prompt** ("Origin (OWNER/REPO): "). `huh`'s value — multi-field forms, validation UI, selects, theming — has **no payoff for one line**. It pays off when a command has a genuine multi-field interactive flow (a richer future onboarding, multi-select skill pickers, etc.). Adopting a 39-module TUI framework now to read one line is textbook YAGNI / not-shortest-path.

## Decisions

- **Decision (resolves S1)**: Use **stdlib `bufio`** for the v0 origin prompt. Confirms research.md D5's provisional baseline.
- **Defer `huh`** to a future feature that introduces a real multi-field/interactive flow. At that point +0.61 MB / +39 modules buys actual UX; today it doesn't. Record `huh` (now **`v1.0.0` stable**) as the **preferred** choice *when* a TUI form is warranted — actively maintained, best UX, `WithAccessible`/`WithInput`/`RunAccessible(w,r)` give a non-TTY-safe + testable path — over the stale `survey`.
- **Reject `survey`** outright (unmaintained, +0.23 MB, +39 deps). **Don't adopt `promptui`** either: near-zero size but +5 deps and low maintenance for no benefit over stdlib on a single line read.

## Recommendations

1. Lock D5 on **stdlib `bufio.Scanner`**, prompt to **stderr**, single read, invalid/empty → usage error (exit 1, no retry loop). Remove the ⚠️ SPIKE marker from D5 and mark S1 resolved with this data.
2. Keep the D4 gate (`--non-interactive` + TTY detection) as the real agent-safety mechanism — it's library-independent.
3. Add a forward note: when a command first needs a multi-field interactive form, re-open this spike and adopt `charmbracelet/huh` (not survey/promptui), accepting the measured ~0.6 MB / 39-module cost then.

## References

- Measured locally (Go 1.24.4, darwin/arm64, `CGO_ENABLED=0 go build -ldflags="-s -w"`); scratch modules built & discarded.
- [research.md](../research.md) D4 (interactive model), D5 (prompting), D1 (go-toml/v2)
- [docs/design/cli.md](../../../docs/design/cli.md) — Principle 2 (errors-as-navigation), non-interactive defaults, Rule 5 (stderr/stdout)
- architecture.md §2 (single static binary, R3/R4)
- Constitution v2.1.0 — V (minimal deps), VI (YAGNI), VII (shortest path), IV (agent-first)
