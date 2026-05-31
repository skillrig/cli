# Research: Remote Git Testing

**Date**: 2026-05-31
**Context**: 003-search-remote introduces real remote git fetch (catalog and skill subtree) for `search` and `add`. 002 had zero network boundary — all test substrate was local git working trees. This spike determines the test architecture for the new network-failure FRs (FR-017 auth, FR-018 unreachable, transient/timeout) and the happy-path fetch path.
**Time-box**: ~45 min (code archaeology + analysis)

---

## Question

What test substrate exercises the new network-failure FRs (FR-017 auth failure, FR-018 unreachable, and transient/timeout errors) for remote `add`/`search`, and what substrate covers the happy fetch path with a ground-truth tree-SHA assertion?

---

## Findings

### Finding 1: How 002 bootstraps the origin and stubs git (ground truth from code)

**Integration tests** (`test/skillcore_quickstart_test.go`, `test/quickstart_test.go`):

002 bootstraps the origin by:
1. Copying the committed fixture at `test/testdata/sample-origin` into a fresh `t.TempDir()`.
2. Running `git init -q -b main` + `git add -A` + `git commit` in that tmpDir using a **pinned identity** (fixed `GIT_AUTHOR_NAME/EMAIL/DATE` for reproducible commit SHAs — `pinnedGitEnv()` in both the integration and unit helpers).
3. Nesting the origin at `<consumerRoot>/my-org/my-skills` (the `OWNER/REPO` path the resolver maps to a local directory via `originDirRef`).
4. The CLI binary is built once in `TestMain` and exec'd via `run()`. There is **no `file://` URL, no `git clone`, no network layer** — `skillcore.Add` receives `OriginDir` as a plain filesystem path and runs `git -C <dir>` directly.

The independent oracle (`rawTreeSHA`) reads the tree-SHA with `git rev-parse HEAD:<relPath>` directly on the origin dir — never through skillcore — to prevent circular validation (Constitution III / D11).

**Unit tests** (`pkg/skillcore/helpers_test.go`, `treesha_test.go`):

`bootstrapOrigin(t)` does the same pattern in-process: `git init` + write fixture files + commit in a `t.TempDir()`. There is no mock of git itself for the happy path — the real `git` binary runs against a real (ephemeral) repo.

The **stub seam** (`stubCommandContext` + `TestHelperProcess`) is how error paths are exercised without real git:
- `gitClient.commandContext` is a pluggable field (`func(ctx, name, args) *exec.Cmd`).
- `stubCommandContext(exitCode, stderr)` returns a `commandContext` that re-execs the test binary with `GO_WANT_HELPER_PROCESS=1`, causing `TestHelperProcess` to write the given stderr and call `os.Exit(exitCode)`.
- This produces a real `*exec.ExitError` (not a mock interface), so `gitClient.run`'s error-wrapping path is exercised exactly as in production.
- Used in `TestGitClient_StubbedExit` to assert `*GitError{ExitCode, Stderr}` is populated correctly for exit 1, 128, etc.

**Key fact**: the stub seam lives entirely in `pkg/skillcore` and targets the `gitClient.commandContext` field. It does **not** stub network or HTTP — it stubs the subprocess exit.

---

### Finding 2: Can `file://` (or a local bare repo) cover the happy/integrity fetch path?

**Yes, with confidence.** Git's `file://` transport (and the `file://` URL variant of `git clone`/`git fetch`) uses the same plumbing as HTTPS but against a local bare repo. It exercises:
- The real `git clone --filter=blob:none --sparse` (partial clone) or `git fetch` code path.
- Sparse-checkout expansion of the skill subtree.
- The same `git rev-parse <ref>:<path>` tree-SHA computation that `skillcore.TreeSHA` calls — the ground-truth assertion `fetched tree-SHA == raw git tree-SHA of origin subtree` holds because both call the same git plumbing on the same object graph.

A **local bare repo** (`git init --bare`) is the cleanest substrate:
- No working tree to accidentally mutate.
- `git clone file:///path/to/bare.git` runs a real fetch handshake (not a cp/hardlink shortcut like a plain path clone without `file://`).
- Fully offline and deterministic. Push to it from the fixture working tree, then clone from it in the test.

Bootstrap pattern for 003 integration tests:
```go
// 1. Create a fixture working tree (same as 002's bootstrapOrigin)
//    and commit the sample skill + index.json.
// 2. Create a bare repo alongside it:
//    git init --bare <tmpDir>/origin.git
// 3. Push the fixture into the bare:
//    git -C <fixtureDir> push file://<tmpDir>/origin.git HEAD:main
// 4. Supply the origin URL to the CLI/skillcore as "file://<tmpDir>/origin.git"
//    (or the OWNER/REPO config pointing at a path-shaped origin).
// 5. rawTreeSHA: git -C <fixtureDir> rev-parse HEAD:<subtree>
//    (still never through skillcore — the same D11 independence).
```

The `file://` URL exercises exactly the fetch path that HTTPS will use in production — only the transport layer changes. This is how projects like `go-git`, `gh`, and `git` itself test their fetch logic offline.

**Confidence: high** — this is well-established in the Go ecosystem (see `go-git` test suite, `gh` CLI integration tests).

---

### Finding 3: Which FRs cannot be simulated with `file://`?

| FR | Failure class | `file://` / bare repo | Alternative needed |
|---|---|---|---|
| FR-017 | Auth failure (401/403, private origin, bad token) | Cannot simulate: `file://` has no auth layer | Fault injection at the exec boundary |
| FR-018 | Unreachable (network down, DNS failure, timeout) | Cannot simulate: `file://` always succeeds locally | Fault injection at the exec boundary |
| Transient | Timeout / partial failure mid-fetch | Cannot simulate reliably with `file://` | Fault injection at the exec boundary |
| FR-016 | Convention version mismatch | Fully simulable: write wrong `convention_version` in the bare origin's index.json | No HTTP needed |
| FR-015 | Skill not found / pin not found | Fully simulable with `file://` | No HTTP needed |
| FR-019 | Skill not in lock (verify after remote add) | Fully simulable | No HTTP needed |

**Auth (FR-017)**: `file://` has no authentication layer. An auth failure from a real HTTPS origin surfaces as `git clone` exiting with code 128 and stderr like `fatal: Authentication failed for 'https://github.com/...'` or `fatal: could not read Username`. These cannot be reproduced offline without either (a) a real HTTPS server that returns 401, or (b) injecting that exit code + stderr at the exec boundary.

**Unreachable (FR-018)**: A network-down/DNS failure surfaces as git exiting 128 with `fatal: unable to access 'https://...': Could not resolve host:` or `Operation timed out`. `file://` will never produce these. Same two options: real HTTP server or exec-boundary injection.

**Transient / timeout**: Similar — these are OS-level TCP failures, not reproducible with `file://`.

---

### Finding 4: Recommended test seam — exec boundary fault injection, not httptest

**The recommended seam is the same `gitClient.commandContext` field already in `pkg/skillcore/git.go`**, extended to cover the new remote fetch operations (e.g., `git clone --filter=blob:none --sparse`, `git fetch`, `git ls-remote`).

**Rationale against `httptest` with a real git-over-HTTP handshake**:

1. **Complexity / fragility**: A real git-over-HTTP server requires implementing the git smart-HTTP protocol (the `info/refs` + `upload-pack` handshake). This is non-trivial — `git http-backend` is a CGI binary, not a Go library. Wrapping it in `httptest` means shelling `git http-backend` from a Go test, which:
   - Requires `git` to be in PATH (already a requirement, but adds the CGI dep).
   - Is OS-specific in behavior (especially on macOS where `git http-backend` may not exist separately from the git bundle).
   - Produces opaque failures when the handshake version or capability negotiation changes across git releases.

2. **Exec-boundary injection proves the right thing**: What FR-017/018 tests need to assert is: "when the git subprocess fails with exit 128 and an auth-failure stderr, skillrig surfaces a distinct `AuthError` (not `UnreachableError`, not `SkillNotFoundError`) with the right what/why/fix message." That contract is fully testable by injecting the right `(exitCode, stderr)` pair via `stubCommandContext`. The CLI's error-mapping code is the thing under test, not git's HTTP stack.

3. **`file://` is the right boundary for happy-path integration**: The integration suite (`TestQuickstart_*`) should use a local bare repo over `file://` for the full stack — binary exec'd, real `git clone`, real tree-SHA. The ground-truth assertion (`fetched treeSHA == rawTreeSHA of the origin fixture`) closes the loop across the fetch boundary.

4. **Nothing genuinely requires a live HTTP layer in the test gate**: The only thing not coverable is "does git actually fail with exit 128 on a real 401 response?" — but that is a git behavior test, not a skillrig behavior test. Skillrig's contract is to correctly classify and render the error type; the classification is driven by the exit code and stderr pattern, both of which the stub can supply exactly.

**Proposed seam architecture for 003**:

```
pkg/skillcore/
  git.go         — extend gitClient with remote ops:
                     Clone(ctx, remote, dest, opts) (string, error)
                     LsRemote(ctx, remote, ref) (string, error)  // resolve commit SHA for a ref/pin
                     FetchSparse(ctx, remote, ref, subtree, dest) error
                   All dispatch through gitClient.commandContext — the single injectable point.
  errors.go      — add typed errors:
                     AuthError{Remote, Stderr string}           // exit 128 + auth-failure stderr pattern
                     UnreachableError{Remote, Stderr string}    // exit 128 + DNS/timeout stderr pattern
                     ConventionError{Got, Supported int}        // FR-016
  fetch.go       — the new remote acquisition layer:
                     Fetch(ctx, opts FetchOptions) (FetchResult, error)
                   FetchOptions carries the remote URL, ref, subtree path, dest dir.
                   Returns the fetched commit SHA + the tree-SHA (computed after fetch).
                   Called by Add when the origin is remote (not a local path).
```

**Unit test pattern (fault injection)**:
```go
func TestFetch_AuthFailure(t *testing.T) {
    c := &gitClient{commandContext: stubCommandContext(128,
        "fatal: Authentication failed for 'https://github.com/private/repo.git'")}
    _, err := c.Clone(context.Background(), "https://github.com/private/repo.git", t.TempDir(), CloneOptions{})
    var authErr *AuthError
    if !errors.As(err, &authErr) {
        t.Fatalf("want *AuthError, got %T: %v", err, err)
    }
}

func TestFetch_Unreachable(t *testing.T) {
    c := &gitClient{commandContext: stubCommandContext(128,
        "fatal: unable to access 'https://github.com/foo/bar.git/': Could not resolve host: github.com")}
    _, err := c.Clone(context.Background(), "https://github.com/foo/bar.git", t.TempDir(), CloneOptions{})
    var unreachErr *UnreachableError
    if !errors.As(err, &unreachErr) {
        t.Fatalf("want *UnreachableError, got %T: %v", err, err)
    }
}
```

**Integration test pattern (`file://` bare repo)**:
```go
func newRemoteConsumerRepo(t *testing.T) (consumerRepo, string /* originURL */) {
    // 1. Fixture working tree (as in bootstrapOrigin)
    fixtureDir := bootstrapFixtureOrigin(t)  // same as 002
    // 2. Bare repo
    bareDir := filepath.Join(t.TempDir(), "origin.git")
    runGit(t, fixtureDir, "init", "--bare", bareDir)  // or: git init --bare bareDir
    runGit(t, fixtureDir, "push", "file://"+bareDir, "HEAD:main")
    originURL := "file://" + bareDir
    // 3. Consumer repo with SKILLRIG_ORIGIN pointing at the bare repo URL
    root := t.TempDir()
    runGit(t, root, "init", "-q", "-b", "main")
    // run skillrig init --origin <path-shaped origin> ...
    return consumerRepo{root: root, originDir: fixtureDir}, originURL
}

func TestQuickstart_RemoteAdd(t *testing.T) {
    c, originURL := newRemoteConsumerRepo(t)
    wantTreeSHA := rawTreeSHA(t, c.originDir, "HEAD", "skills/"+sampleSkill)
    res := run(t, runOpts{
        args: []string{"add", sampleSkill},
        cwd:  c.root,
        env:  map[string]string{"SKILLRIG_ORIGIN": originURL},
    })
    // ... assert exit 0, treeSHA in lock == wantTreeSHA
}
```

**Error-class integration tests** (these use the stub path, not `file://`):
```go
// Auth failure: inject the stub at the skillcore boundary,
// call the CLI's add command directly (in-process, not exec-of-binary),
// or provide a fake binary wrapper that exits 128 with the right stderr.
```

For the integration (exec-of-binary) tier, the auth/unreachable tests can use a small **shim binary** (compiled once in `TestMain` alongside the real binary) that simply exits with a given code and writes a given stderr — or use `SKILLRIG_GIT_BIN` env to point the CLI at a fake git binary. However, the simpler path is to test auth/unreachable error classification as **unit tests** against the `pkg/skillcore` fetch layer directly (in-process, no binary exec), and test the CLI rendering of those errors via `internal/cli` unit tests (which call `addCmd.run` directly with a stub `gitClient`).

---

### Finding 5: What genuinely cannot be covered offline?

1. **GitHub's actual auth handshake**: Verifying that `gh auth token` or `GIT_ASKPASS` is correctly plumbed to git for a real private repo. This requires a real GitHub call and belongs in a manual/CI E2E test tier, not the offline gate.

2. **Partial-fetch abort mid-stream**: A real TCP reset mid-clone cannot be deterministically reproduced with `file://` or exec stubs. However, git's behavior on abort (non-zero exit, stderr describing the failure) is stable, and the exit-code/stderr injection covers skillrig's handling of it.

3. **Rate limiting (HTTP 429)**: Would require a real HTTP server. Not worth building for the offline gate; document as a future E2E concern.

None of these block the correctness of the offline suite. The offline suite can provide full coverage of skillrig's own error classification, rendering, and exit-code logic.

---

## Decisions

- **Decision 1: `file://` + local bare repo for the integration happy path.** The `TestQuickstart_*` suite bootstraps a bare repo in a `t.TempDir()`, pushes the fixture into it, and points the CLI at it via `file://` URL. This runs the real fetch path (sparse-checkout, commit resolution, tree-SHA computation) entirely offline and deterministically. The ground-truth assertion (`fetched treeSha == rawTreeSHA of fixture origin`) closes the integrity loop across the fetch boundary.

- **Decision 2: Exec-boundary fault injection (extended `gitClient.commandContext` stub) for FR-017/018 unit tests.** Auth, unreachable, and transient error paths are tested as `pkg/skillcore` unit tests using the existing `stubCommandContext` pattern with crafted `(exitCode=128, stderr=<git-auth-failure-text>)` pairs. This is deterministic, offline, and proves skillrig's error-type discrimination and rendering without needing an HTTP server.

- **Decision 3: No `httptest` with a real git-over-HTTP handshake in the offline gate.** The complexity and fragility (CGI binary, OS-specific `git http-backend`, protocol negotiation across git versions) is not justified. The seam at the exec boundary is sufficient for all FRs in the spec.

- **Decision 4: Typed errors for each failure class in `pkg/skillcore/errors.go`.** `AuthError`, `UnreachableError`, and `ConventionError` (FR-016) must be distinct types (not string matching on `GitError.Stderr` in the CLI). The CLI maps each to a `*UsageError` with the right what/why/fix. The stderr pattern matching (auth vs unreachable) lives inside `pkg/skillcore`'s fetch layer, not in `internal/cli`.

- **Decision 5: Stderr-pattern discrimination lives in `skillcore`, not `cli`.** `fetch.go` inspects `GitError.Stderr` to classify the failure before returning to the CLI. This keeps the CLI presentation-free (it receives a typed error, not a raw git stderr). The pattern strings (e.g., `"Authentication failed"`, `"Could not resolve host"`) are internal to skillcore and tested by the unit suite.

---

## Recommendations

1. **Extend `pkg/skillcore/git.go`** with a `Clone`/`FetchSparse` method on `gitClient`, dispatching through the existing `commandContext` field. No new seam needed — the seam already exists.

2. **Add `AuthError` and `UnreachableError` to `pkg/skillcore/errors.go`**, classified from `GitError.Stderr` inside the fetch layer. Write unit tests using `stubCommandContext` with real-world git stderr strings for each class (copy from actual git output).

3. **Bootstrap a `newRemoteConsumerRepo(t)` helper in `test/`** that creates a bare repo, pushes the fixture, and returns the `file://` URL. All `TestQuickstart_RemoteAdd*` scenarios use it. The `rawTreeSHA` oracle pattern is unchanged.

4. **Do not add `httptest`** to the test dep graph for this slice. Note it as a possible future tier for E2E auth smoke tests against a real GitHub App installation.

5. **Catalog fetch** (for `search`): the same `file://` bare repo can host an `index.json` in its root, fetched via a single `git show <ref>:index.json` or sparse-checkout. The auth/unreachable error paths for catalog fetch use the same stub pattern as skill fetch.

6. **`--pin` / ref resolution** (US3): `git ls-remote file:///path/to/origin.git <tag>` returns the commit SHA for a tag — this too runs through `gitClient.commandContext` and is stubbable for "no such ref" errors.

---

## References

- `pkg/skillcore/git.go` — the `gitClient` struct and `commandContext` seam (lines 1–99).
- `pkg/skillcore/helpers_test.go` — `stubCommandContext`, `TestHelperProcess`, `bootstrapOrigin` (the full stub + fixture pattern).
- `pkg/skillcore/treesha_test.go` — `TestGitClient_StubbedExit`, `TestTreeSHA_GroundTruth`, `TestTreeSHA_RelocationInvariance` (the existing stub-seam usage + ground-truth discipline).
- `test/skillcore_quickstart_test.go` — `newConsumerRepo`, `bootstrapOrigin`, `rawTreeSHA`, `pinnedGitEnv` (the integration bootstrap pattern, lines 139–228).
- `specledger/003-search-remote/spec-tech.md` §6 (failure taxonomy), §7 (test tier decision), §8b S4 (this spike's brief).
- `go-git` test suite (upstream reference for `file://` bare-repo integration patterns): `https://github.com/go-git/go-git/tree/main/plumbing/transport/file`
- `gh` CLI git client pattern: `https://github.com/cli/cli/blob/trunk/git/client.go` (the `commandContext`-injectable field this codebase mirrors, per research D7 in helpers_test.go).
