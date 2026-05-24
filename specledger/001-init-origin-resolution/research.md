# Phase 0 Research: CLI Initialization & Origin Resolution

## Prior Work

`sl issue list --all` → no issues; this is the first feature in the project. No prior plan/tasks to reconcile. Authoritative references: `architecture.md` §2 (command surface), §2b (consume-only), §2d (origin discovery + config/lock split + `init` semantics); `docs/design/cli.md` (binding CLI contract); constitution v2.1.0.

---

## D1 — TOML library

**Decision**: `github.com/pelletier/go-toml/v2`.
**Rationale**: Actively maintained, fast, `encoding/json`-style `Marshal`/`Unmarshal` with struct tags, good error messages (helps FR-004 "clear diagnostic, not raw parser dump"), no cgo (keeps the static-binary goal). Matches architecture's `config.toml` decision.
**Alternatives**: `BurntSushi/toml` (mature but slower, less ergonomic v2 API); hand-rolled parser (rejected — YAGNI, error-prone on edge cases).

## D2 — Global config location

**Decision**: `$XDG_CONFIG_HOME/skillrig/config.toml`, falling back to `~/.config/skillrig/config.toml` on **all** platforms. Do NOT use `os.UserConfigDir()`.
**Rationale**: Architecture §2d names `~/.config/skillrig/config.toml` explicitly, git-style. `os.UserConfigDir()` returns `~/Library/Application Support` on macOS and `%AppData%` on Windows, which would diverge from the documented path. Consistency with the documented contract beats per-OS idiom here.
**Alternatives**: `os.UserConfigDir()` (rejected — diverges from the architecture path on macOS/Windows).

## D3 — Project config discovery (resolution) vs write location (init)

**Decision**:
- **Resolution** walks up from `cwd` to the nearest ancestor containing `.skillrig/config.toml` (git-style discovery), so any subdirectory of a bound repo resolves the same origin (supports SC-002 "self-describing repo").
- **`init` (write)** writes to `./.skillrig/config.toml` in the current working directory by default (creating `.skillrig/` if needed, FR-010). It does not attempt to locate a VCS root (no git dependency; offline, simple).
**Rationale**: Walk-up resolution is what makes the committed config authoritative from anywhere in the tree; writing at cwd keeps `init` dependency-free and predictable. A user running `init` at the repo root (the normal case) gets the expected `<repo>/.skillrig/config.toml`.
**Alternatives**: cwd-only resolution (rejected — breaks resolution from subdirectories); git-root detection for write (rejected — adds a git dependency for marginal benefit, YAGNI).

## D4 — Interactive vs non-interactive detection

**Decision**: Treat the session as interactive iff **stdin is a character device** — `fileInfo, _ := os.Stdin.Stat(); interactive := fileInfo.Mode()&os.ModeCharDevice != 0`. Stdlib only.
**Rationale**: No extra dependency; correctly returns false in CI/pipes/agent runners (TTY-less), satisfying FR-006a (prompt only when interactive). Honors agentic-cli-design P2 and cli.md.
**Alternatives**: `golang.org/x/term.IsTerminal` (works but adds a dependency for what stdlib covers); always-non-interactive (rejected — FR-006a wants a prompt for humans).

## D5 — Prompting mechanism

**Decision**: When interactive and `--origin` omitted, prompt once on stderr ("Origin (OWNER/REPO): ") and read one line from stdin via `bufio.Scanner`. Stdlib only. Empty/invalid input → usage/config error (exit 1), not a re-prompt loop.
**Rationale**: Minimal, dependency-free, predictable. The prompt goes to **stderr** so stdout stays clean for any machine consumer (cli.md Rule 5). YAGNI — no `survey`/`promptui`.
**Alternatives**: `AlecAivazis/survey`, `manifoldco/promptui` (rejected — heavy deps for a single prompt); retry loop (rejected — keep it deterministic and scriptable).

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

**Decision**: Write to a temp file in the target dir then `os.Rename` over the destination (rename is atomic on the same filesystem). Preserve a trailing newline; stable key ordering via the struct field order.
**Rationale**: Prevents a torn/corrupt `config.toml` on crash and avoids partial writes the resolver might later read (FR-004). Mirrors architecture open-Q10's lockfile-atomicity guidance, applied to config.
**Alternatives**: Direct truncating write (rejected — torn-write risk); file locking (rejected — single-writer `init`, not needed; YAGNI).

## D10 — Cobra command structure

**Decision**: One root command `skillrig` (Long description + Example block; `RunE` prints help when called bare per cli.md Level-0). Persistent flags `--json` and `--verbose` on root. One subcommand `init` with local flags `--origin` and `--global`, `Args: cobra.NoArgs` (origin is a flag, not positional, since it's optional/interactive), a `Long` description, and ≥2 `Example` lines (cli.md Rule 1 / Principle 1). Unit tests drive commands in-process via `SetArgs`/`SetOut`/`SetErr` (golang-spf13-cobra skill).
**Rationale**: Establishes the progressive-discovery skeleton future commands extend. `--origin` as a flag (not positional) reads naturally with `--global` and the prompt fallback.
**Alternatives**: positional `skillrig init <origin>` (viable, but a flag composes better with `--global` and optional/interactive entry; revisit if usage shows positional is preferred — a desire path per cli.md).

---

## Resolved unknowns

All Technical Context items are resolved; no remaining `NEEDS CLARIFICATION`. No external network/integration boundaries in this feature, so no httptest/go-vcr (Constitution III applies to the **config.toml** ground-truth fixture instead).
