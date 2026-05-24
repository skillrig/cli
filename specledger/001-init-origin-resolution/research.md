# Phase 0 Research: CLI Initialization & Origin Resolution

## Prior Work

`sl issue list --all` → no issues; this is the first feature in the project. No prior plan/tasks to reconcile. Authoritative references: `architecture.md` §2 (command surface), §2b (consume-only), §2d (origin discovery + config/lock split + `init` semantics); `docs/design/cli.md` (binding CLI contract); constitution v2.1.0.

---

## D1 — TOML library

**Decision**: `github.com/pelletier/go-toml/v2`.
**Rationale**: Actively maintained, fast, `encoding/json`-style `Marshal`/`Unmarshal` with struct tags, good error messages (helps FR-004 "clear diagnostic, not raw parser dump"), no cgo (keeps the static-binary goal). Matches architecture's `config.toml` decision.
**Alternatives**: `BurntSushi/toml` (mature but slower, less ergonomic v2 API); hand-rolled parser (rejected — YAGNI, error-prone on edge cases).

## D2 — Global config location

**Decision**: `$XDG_CONFIG_HOME/skillrig/config.toml`, falling back to `~/.config/skillrig/config.toml` on **all** platforms. Resolve `~` via stdlib `os.UserHomeDir()` (cross-platform). Do NOT use `os.UserConfigDir()`.
**Rationale**: Architecture §2d names `~/.config/skillrig/config.toml` explicitly, git-style. `os.UserConfigDir()` returns `~/Library/Application Support` on macOS and `%AppData%` on Windows, which would diverge from the documented path. Consistency with the documented contract beats per-OS idiom here.

**OS / shell support (v0 note — review comment ec7c2bdb).** `XDG_CONFIG_HOME` is an environment variable from the freedesktop XDG Base Directory spec — it is **not shell-specific** (any process inherits it if the environment sets it; shells/login managers commonly export it on Linux):
- **Linux/BSD**: native convention. Honored when set, else `~/.config`. ✅ idiomatic.
- **macOS**: XDG is not an OS standard (Apple uses `~/Library/Application Support`), but `XDG_CONFIG_HOME` is usually unset so we land on `~/.config`, which `git`/`gh` also use. ✅ works, widely accepted for CLI tools.
- **Windows**: `~/.config` is non-idiomatic (the convention is `%AppData%`). `os.UserHomeDir()` returns `%USERPROFILE%`, so we'd write `%USERPROFILE%\.config\skillrig\config.toml`. ⚠️ functional but unconventional — **explicit v0 caveat**; revisit Windows path idiom if/when Windows is a first-class target.
**Alternatives**: `os.UserConfigDir()` (rejected — diverges from the architecture path on macOS/Windows). `adrg/xdg` library (handles per-OS XDG resolution incl. Windows Known Folders) — **deferred**: adds a dependency for what `os.UserHomeDir()` + the `XDG_CONFIG_HOME` env check cover in v0; reconsider when Windows idiomatic paths matter.

## D3 — Project config discovery (resolution) vs write location (init)

**Decision** (revised per review comment 0aec14ed):
- **Resolution** walks up from `cwd` to the nearest ancestor containing `.skillrig/config.toml` (git-style discovery), so any subdirectory of a bound repo resolves the same origin (supports SC-002 "self-describing repo"). This is a **pure-filesystem** walk — no subprocess, deterministic, and it still works in a not-yet-`git init`'d directory or an agent sandbox.
- **`init` (write)** writes to `.skillrig/config.toml` at the **git repository root** when the cwd is inside a git work tree (`git rev-parse --show-toplevel`), otherwise falls back to `./.skillrig/config.toml` at cwd (creating `.skillrig/` if needed, FR-010).
**Rationale**: The earlier "no git dependency" framing was wrong for this framework — skillrig is **git-native** (it vendors skills from git origins, architecture §2/§5), so git is a safe baseline assumption and using it to anchor the write at the repo root avoids the foot-gun of writing `.skillrig/` into a random subdirectory. Resolution stays git-*independent* on purpose (faster, no subprocess, robust pre-`git init`), so the two jobs use the right tool each: write = locate the repo root (git when available); resolve = filesystem walk-up.
**Alternatives**: cwd-only write (rejected — writes config in a subdir if run from one); cwd-only resolution (rejected — breaks resolution from subdirectories); requiring git for *resolution* (rejected — resolution must work offline/subprocess-free and in non-repo dirs).
**Open**: if `git` is genuinely absent and cwd is not a repo, `init` falls back to cwd — confirm this fallback is acceptable vs. erroring (leaning: fall back, since the resolver finds it by walk-up regardless).

## D4 — Interactive vs non-interactive model (flags + detection)

**Decision** (revised per review comment 8019052e):
- **Flags carry the values**; missing required values (the origin) are *prompted for* when interactive.
- **Auto-detect** interactivity: a session is interactive iff **stdin is a character device** — `fileInfo, _ := os.Stdin.Stat(); interactive := fileInfo.Mode()&os.ModeCharDevice != 0` (stdlib).
- **Explicit `--non-interactive` flag** overrides detection: it forces non-interactive even on a TTY, and when a required value is missing it **errors describing exactly which flag(s) to pass** (e.g. "missing origin: pass `--origin OWNER/REPO`"). This gives agents/CI a deterministic "never prompt" switch independent of TTY heuristics.
- Resolution of "should I prompt?": prompt **iff** value missing AND interactive (TTY) AND not `--non-interactive`; otherwise error with the missing-flag guidance (FR-006a).
**Rationale**: TTY detection alone is a heuristic; an explicit `--non-interactive` flag is the contract agents rely on (agentic-cli-design P2) and makes the missing-input error path testable. Stdlib detection avoids a dependency.
**Alternatives**: `golang.org/x/term.IsTerminal` (adds a dep for what stdlib covers); detection-only with no explicit flag (rejected — agents want an explicit switch).

## D5 — Prompting mechanism  ✅ (S1 spike resolved)

**Decision**: When prompting is warranted (D4), prompt once on **stderr** (`Origin (OWNER/REPO): `) and read one line via stdlib **`bufio.Scanner`**. Empty/invalid → usage/config error (exit 1), no re-prompt loop. Prompt on stderr keeps stdout clean for machine consumers (cli.md Rule 5).
**Rationale (resolved by spike S1 — measured)**: see [research/2026-05-24-interactive-prompt-library.md](research/2026-05-24-interactive-prompt-library.md). Measured stripped-binary deltas over our cobra+go-toml/v2 baseline (2.57 MB): stdlib **+0**, promptui **+768 B**, survey **+0.23 MB / +39 modules**, huh **+0.61 MB / +39 modules**. v0 has exactly **one** single-line prompt — a TUI form lib has no UX payoff yet (YAGNI/VI, shortest-path/VII), and stdlib `bufio` is the least likely to ever block an agent on non-TTY stdin (returns EOF immediately) — best fit for IV/agent-safety + V/minimal-deps.
**Deferred (not now)**: when a command first needs a genuine multi-field interactive form, adopt **`charmbracelet/huh`** (now `v1.0.0` stable; `WithAccessible`/`WithInput`/`RunAccessible(w,r)` give a non-TTY-safe, testable path) — not the unmaintained `survey`. Re-open the spike then to re-confirm the ~0.6 MB / 39-module cost is justified.
**Alternatives rejected for v0**: `survey` (unmaintained, +0.23 MB, +39 deps); `promptui` (near-zero size but +5 deps and low maintenance for no benefit over stdlib on one line); `huh` (deferred per above).

## D6 — OWNER/REPO validation

**Decision**: Trim surrounding whitespace, then require the pattern `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$` — exactly two non-empty, slash-separated segments. Reject anything else with a usage/config error (exit 1) that names the expected format and echoes the offending value (FR-012). An empty/blank `SKILLRIG_ORIGIN` is treated as **unset**, not invalid (spec Edge Cases).
**Rationale**: Matches GitHub `owner/repo` charset; offline shape-only check (no existence/reachability — deferred per spec Assumptions). Distinguishing "blank env = unset" preserves precedence fall-through.
**Alternatives**: Full GitHub-name RFC validation (rejected — over-strict, not needed offline); accepting `owner/repo/path` (rejected — that grammar belongs to skill refs, not origin config).

## D7 — Exit-code & error mapping (cli.md is authoritative)

**Decision**: `ExitOK=0`, `ExitUsage=1`. Codes `2` (verification) and `3` (prerequisite) are declared as reserved constants with comments but unused here. Commands use Cobra `RunE`; the root sets `SilenceUsage=true` and `SilenceErrors=true` so we render errors ourselves (errors-as-navigation), then `main.go` maps the returned error to an exit code. A typed `UsageError` (wrapping the raw cause) carries `ExitUsage`; anything else also exits `1` for this feature's surface.
**Rationale**: cli.md's exit codes are load-bearing and override agentic-cli-design's `2=invalid-args/3=auth/4=retryable` scheme (documented conflict — defer to cli.md). Silencing Cobra's built-in usage/error noise lets us honor the what/why/fix contract and the stdout/stderr split.
**Alternatives**: Cobra default error printing (rejected — dumps usage to stdout and a bare error, violating cli.md Principle 2 and Rule 5); adopting the skill's code numbers (rejected — collides with cli.md's meaning of 2/3).

## D8 — Output shape: human (default) vs `--json`

**Decision**:
- **Human (default)**: a compact confirmation to stdout, e.g. `bound origin my-org/my-skills (project: ./.skillrig/config.toml)`, plus a single footer-hint line (cli.md Principle 3). Line count is bounded (≤ ~2 lines) — asserted in tests.
- **`--json`**: a single complete object to stdout: `{ "ok": true, "origin": "my-org/my-skills", "scope": "project|global", "configPath": "…", "written": true|false }`. Parseable + structurally complete (all keys present).
- **Errors**: prose (what/why/fix) to **stderr** regardless of `--json`, with the raw cause preserved. We do NOT emit JSON errors (defer to cli.md over agentic-cli-design P1).
**Rationale**: Matches cli.md two-level output and the spec's FR-016. `written:false` distinguishes an idempotent no-op re-bind from a fresh write (FR-008) for machine consumers.
**Alternatives**: JSON-by-default (rejected — cli.md keeps human compact as default); structured JSON errors (rejected — cli.md mandates prose errors-as-navigation).

## D9 — Atomic config write

**Decision**: Write to a temp file **in the same target directory** (so it's on the same filesystem/volume) then `os.Rename` over the destination. Preserve a trailing newline; stable key ordering via the struct field order. File mode `0o644`, dir `0o755`.
**Rationale**: Prevents a torn/corrupt `config.toml` on crash and avoids partial writes the resolver might later read (FR-004). Mirrors architecture open-Q10's lockfile-atomicity guidance, applied to config.
**OS requirements (review comment 2bf31175)**:
- **POSIX (Linux/macOS)**: `rename(2)` is atomic when source and dest are on the same filesystem — hence the temp file goes in the **target dir**, never `os.TempDir()` (which may be a different mount, turning `os.Rename` into a cross-device `EXDEV` error). ✅
- **Windows**: `os.Rename` maps to `MoveFileEx` with `MOVEFILE_REPLACE_EXISTING` in modern Go, so replace-over-existing works; but a concurrent open handle on the dest (AV scanners, the file open in an editor) can cause a sharing-violation. Acceptable for v0's single-writer `init`; note as a Windows caveat.
- Create parent dirs (`os.MkdirAll`) before writing; `fsync` the temp file before rename for durability if we later care (deferred — YAGNI for a tiny config).
**Alternatives**: Direct truncating write (rejected — torn-write risk); temp in `os.TempDir()` (rejected — cross-device rename failure); file locking (rejected — single-writer `init`, not needed; YAGNI).

## D10 — Cobra command structure

**Decision**: One root command `skillrig` (Long description + Example block; `RunE` prints help when called bare per cli.md Level-0). Persistent flags `--json` and `--verbose` on root. One subcommand `init` with local flags `--origin` and `--global`, `Args: cobra.NoArgs` (origin is a flag, not positional, since it's optional/interactive), a `Long` description, and ≥2 `Example` lines (cli.md Rule 1 / Principle 1). Unit tests drive commands in-process via `SetArgs`/`SetOut`/`SetErr` (golang-spf13-cobra skill).
**Rationale**: Establishes the progressive-discovery skeleton future commands extend. `--origin` as a flag (not positional) reads naturally with `--global` and the prompt fallback.
**Alternatives**: positional `skillrig init <origin>` (viable, but a flag composes better with `--global` and optional/interactive entry; revisit if usage shows positional is preferred — a desire path per cli.md).

**Config library — viper? (review comment be52d37c)**: **Decision: no viper for v0.** Viper *does* support TOML (it reads via `pelletier/go-toml`), and `cobra`+`viper`+`huh` are mutually compatible (cobra = commands, viper = config layering, huh = interactive forms — three orthogonal libs). But viper's value is generic multi-source config merging, whereas our origin resolution has **specific, contract-bound semantics** (architecture §2d): a named env var (`SKILLRIG_ORIGIN`), filesystem **walk-up** to the nearest project `.skillrig/config.toml`, a fixed global path, blank-env-as-unset, malformed-source-skip, and a single `ResolveOrigin` per AP-06. Viper's `AutomaticEnv`/config-merge does not model walk-up or our precedence cleanly and would fight the contract. Keep `go-toml/v2` (D1) + the hand-rolled resolver (D-contract `resolve.md`) — exact control, fewer deps, trivially table-testable. Revisit viper only if config grows many keys/sources.

---

## Open evaluations / spikes

- **S1 — Interactive prompt library (D5)**: ✅ **resolved** by [research/2026-05-24-interactive-prompt-library.md](research/2026-05-24-interactive-prompt-library.md). Outcome: stdlib `bufio` for v0; defer `charmbracelet/huh` (v1.0.0) to a future multi-field interactive flow. Measured: huh +0.61 MB / +39 modules vs a +0 stdlib baseline.

Everything is resolved; no remaining `NEEDS CLARIFICATION`. No external network/integration boundaries in this feature, so no httptest/go-vcr (Constitution III applies to the **config.toml** ground-truth fixture instead).

## Review trail

Revised per crit review of `research.md` (Session 2026-05-24): D2 (XDG OS/shell support + `os.UserHomeDir`/`adrg/xdg` note), D3 (git is a fair assumption — git-root write, fs walk-up resolve), D4 (explicit `--non-interactive` flag + flag/prompt model), D5 (huh spike S1), D9 (POSIX same-fs + Windows `MoveFileEx` caveats), D10 (viper evaluated, rejected for v0; TOML supported; cobra/viper/huh compatible).
