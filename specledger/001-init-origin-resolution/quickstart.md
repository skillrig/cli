# Quickstart: CLI Initialization & Origin Resolution

> **Quickstart-as-Contract (Constitution II)**: every scenario below is an executable acceptance test. Each maps **1:1** to a Go test named `TestQuickstart_<Scenario>` (CLI scenarios exec the built binary) or `TestResolveOrigin_<Row>` (resolver scenarios run the single resolver against real fixture files on a temp filesystem — no mocks). A scenario is DONE only when its test passes. Output-shape assertions are mandatory where noted.

## Setup (test harness)

- `TestMain` builds the binary once (`go build -o $TMP/skillrig .`) and runs all CLI scenarios against it via `os/exec`.
- Each scenario runs in its own `t.TempDir()` as `cwd`, with `HOME`/`XDG_CONFIG_HOME` pointed at the temp dir so project and global config are isolated and parallel-safe.
- `SKILLRIG_ORIGIN` is set per-scenario via the exec `Env` (never the process env).
- **Git fixtures**: git-root scenarios initialise a throwaway repo in the tempdir (`git init -q`) and create nested subdirs, then run `skillrig init` from a subdir to assert the config lands at the repo root. A non-git tempdir asserts the cwd fallback. `git` is a required dependency of both the harness and skillrig itself (`git rev-parse --show-toplevel`, offline only — no fetch/clone). A helper skips git scenarios with a clear message if `git` is absent from PATH.

Conventions: **stdout** = data, **stderr** = errors/prompts, **exit** = code.

---

## Part A — `skillrig init` (CLI E2E, exec the binary)

### TestQuickstart_BindProject  (US1 / FR-005, FR-010)
```
$ skillrig init --origin my-org/my-skills
```
- **exit**: 0
- **stdout** (human, compact): line 1 contains `bound origin my-org/my-skills` and `project`; line 2 is the `→ resolve order:` footer hint.
- **file** `./.skillrig/config.toml` equals fixture (`test/fixtures/config.toml`):
  ```toml
  origin = 'my-org/my-skills'
  ```
  (TOML literal-string form — the real `go-toml/v2` output; see data-model.md ground-truth note G1.)
- **shape assert**: `len(stdoutLines) <= 2`.

### TestQuickstart_BindProjectJSON  (US1 / FR-016 — output-shape)
```
$ skillrig init --origin my-org/my-skills --json
```
- **exit**: 0
- **stdout**: single JSON object.
- **shape assert**: `json.Unmarshal` succeeds AND keys `ok, origin, scope, configPath, written` all present; `ok==true`, `origin=="my-org/my-skills"`, `scope=="project"`, `written==true`.

### TestQuickstart_IdempotentRebind  (US1 / FR-008)
```
$ skillrig init --origin my-org/my-skills
$ skillrig init --origin my-org/my-skills        # second run
```
- **exit** (both): 0
- **2nd stdout**: contains `already bound` / `no change`.
- **2nd `--json`** (variant) → `written==false`.
- **file**: unchanged between runs (byte-equal).

### TestQuickstart_RebindDifferent  (US1 / FR-009)
```
$ skillrig init --origin my-org/my-skills
$ skillrig init --origin other-org/other-skills
```
- **exit**: 0
- **file** now equals `origin = "other-org/other-skills"` (cleanly replaced, no duplicate keys).

### TestQuickstart_Global  (US1 / FR-007)
```
$ skillrig init --origin my-org/my-skills --global
```
- **exit**: 0
- **file**: `$XDG_CONFIG_HOME/skillrig/config.toml` (or `~/.config/skillrig/config.toml`) equals the origin fixture.
- **assert**: `./.skillrig/config.toml` does **not** exist (repo config untouched).
- **`--json`**: `scope=="global"`.

### TestQuickstart_BindFromGitSubdir  (US1 / FR-005, FR-010 — git-root write target)
> Initialise a git repo in the tempdir and run `init` from a nested subdir. (Skipped with a clear message if `git` is not on PATH.)
```
$ git init -q && mkdir -p a/b/c
$ cd a/b/c && skillrig init --origin my-org/my-skills
```
- **exit**: 0
- **file**: `<repo-root>/.skillrig/config.toml` equals the origin fixture — written at the **git root**, NOT at `a/b/c/.skillrig/config.toml`.
- **assert**: no `.skillrig/` dir is created under `a/b/c`.
- **resolve-symmetry**: `ResolveOrigin(cwd=a/b/c, env)` finds it via walk-up → `Source==project`, `ConfigPath==<repo-root>/.skillrig/config.toml`.

### TestQuickstart_BindNonGitCwdFallback  (US1 / FR-010 — cwd fallback)
> Non-git tempdir: write target falls back to cwd.
```
$ skillrig init --origin my-org/my-skills    # tempdir is NOT a git repo
```
- **exit**: 0
- **file**: `./.skillrig/config.toml` (in cwd) equals the origin fixture.

### TestQuickstart_MalformedOrigin  (US3 / FR-012 — error-shape)
```
$ skillrig init --origin not-a-valid-origin
```
- **exit**: 1
- **stdout**: empty.
- **stderr** asserts **three distinct parts**: (a) names the failure / echoes `not-a-valid-origin`; (b) states expected `OWNER/REPO`; (c) shows a concrete fix example.
- **assert**: no `.skillrig/config.toml` written.

### TestQuickstart_NoOriginNonInteractive  (US3 / FR-006a — error-shape)
```
$ skillrig init           # stdin is NOT a TTY (piped /dev/null)
```
- **exit**: 1
- **stderr** three parts: (a) no origin given; (b) non-interactive session (no TTY); (c) fix = pass `--origin OWNER/REPO` or set `SKILLRIG_ORIGIN`.
- **assert**: no config written.

### TestQuickstart_NonInteractiveFlag  (US3 / FR-006c — forced no-prompt, error-shape)
> stdin **is** an interactive TTY (pty), but `--non-interactive` forces fail-fast. Asserts the flag overrides TTY auto-detection — the distinct intent behind FR-006c.
```
$ skillrig init --non-interactive       # interactive TTY present, no --origin
```
- **exit**: 1
- **stderr** three parts: (a) no origin given; (b) non-interactive mode requested (`--non-interactive`); (c) fix = pass `--origin OWNER/REPO` or set `SKILLRIG_ORIGIN`.
- **assert**: the prompt string `Origin (OWNER/REPO):` is **NOT** emitted (no blocking read); no config written.

### TestQuickstart_PromptInteractive  (US1 / FR-006a — interactive path)
> Interactive TTY simulated by feeding stdin and signaling interactive mode in the harness (e.g. a pty or the harness's interactive shim).
```
$ printf 'my-org/my-skills\n' | skillrig init     # interactive
```
- **exit**: 0
- **stderr**: contains the prompt `Origin (OWNER/REPO):`.
- **file**: `./.skillrig/config.toml` equals the origin fixture.

### TestQuickstart_Help  (FR-013 — Progressive Discovery)
```
$ skillrig init --help
```
- **exit**: 0
- **shape assert**: help text contains a `Long` description AND **≥2** `skillrig init` example lines.

---

## Part B — Origin resolution precedence (resolver integration, real fixture files)

Backed by the recorded matrix in `data-model.md`. `TestResolveOrigin_Precedence` is table-driven over rows 1–7; each row materializes the indicated real files in a temp dir and calls `ResolveOrigin(cwd, env)`.

### TestResolveOrigin_Row1_None  (FR-003)
No env, no project, no global → `Source==none`, zero origin. (Caller turns this into the US3 error.)

### TestResolveOrigin_Row2_Project
project `origin=my-org/my-skills`, no env/global → `my-org/my-skills`, `Source==project`.

### TestResolveOrigin_Row3_EnvBeatsProject  (US2 — key precedence case)
`SKILLRIG_ORIGIN=ci-org/ci-skills` + project `my-org/my-skills` → `ci-org/ci-skills`, `Source==env`.

### TestResolveOrigin_Row4_Global
only global `personal/skills` → `personal/skills`, `Source==global`.

### TestResolveOrigin_Row5_ProjectBeatsGlobal  (US2 — contractor case)
project `client-a/skills` + global `personal/skills` → `client-a/skills`, `Source==project`.

### TestResolveOrigin_Row6_BlankEnvIsUnset
`SKILLRIG_ORIGIN=""` (blank) + project `my-org/my-skills` → `my-org/my-skills`, `Source==project`.

### TestResolveOrigin_Row7_MalformedProjectSkipped  (FR-004)
unparseable project `config.toml` + global `personal/skills` → `personal/skills`, `Source==global`; resolution does not error on the bad file.

### TestResolveOrigin_FromSubdir  (US2 / SC-002)
project config at `<tmp>/.skillrig/config.toml`; call `ResolveOrigin` with `cwd=<tmp>/a/b/c` → resolves `my-org/my-skills` via walk-up, `Source==project`.

---

## Coverage map (scenario → requirement)

| Scenario | Covers |
|----------|--------|
| BindProject / BindProjectJSON | US1, FR-005, FR-010, FR-016 |
| IdempotentRebind | FR-008, SC-005 |
| RebindDifferent | FR-009 |
| Global | FR-007 |
| BindFromGitSubdir | US1, FR-005, FR-010, SC-002 (git-root write target) |
| BindNonGitCwdFallback | US1, FR-010 (cwd fallback when not in a git repo) |
| MalformedOrigin | FR-012, FR-014, SC-004 |
| NoOriginNonInteractive | FR-006a, FR-014, SC-004 |
| NonInteractiveFlag | US3, FR-006c, FR-014, SC-004 |
| PromptInteractive | FR-006a |
| Help | FR-013, SC-006 |
| ResolveOrigin_Row1–7 | FR-001, FR-002, FR-003, FR-004, SC-003 |
| ResolveOrigin_FromSubdir | US2, SC-002 |

All scenarios are offline and deterministic (Constitution III / IV). The full suite passing == feature DONE (Constitution II).
