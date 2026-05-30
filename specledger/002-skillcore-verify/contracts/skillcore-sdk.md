# Contract: `pkg/skillcore` (public Go SDK — SDK-1)

**Import**: `github.com/skillrig/cli/pkg/skillcore`
**Guarantee**: the **one** implementation (AP-04) of the integrity primitives — git tree-SHA, `skill.toml` parse, `skills-lock.json` I/O, and the `Add`/`Verify` operations. **Presentation-free** (returns typed values + typed errors; **never** writes to stdout/stderr or formats user-facing text — Constitution V). **Never fetches** (pure filesystem + local `git`; acquisition/auth are the caller's concern — the SDK boundary, spike §10). Consumed by the `skillrig` CLI and importable by third-party Go tools.

> Signatures below are the *intended surface* (finalized in code; PRE-RELEASE → may churn freely). Names are illustrative; the contract is the behavior + the presentation-free/typed-error discipline.

## Primitives

```go
// TreeSHA returns the git tree-object SHA of relPath at ref within the git repo
// rooted at gitDir, by shelling `git -C gitDir rev-parse <ref>:<relPath>`
// (research D1 — git-canonical, relocation-invariant). relPath must resolve to a
// directory (a skill subtree). Used by Add (on the origin, ref=resolved) and by
// Verify (on the consumer, ref="HEAD") — same function both sides.
func TreeSHA(gitDir, ref, relPath string) (string, error)

// ParseManifest parses a skill.toml. Unknown keys are ignored (forward-compat).
func ParseManifest(path string) (Manifest, error)
type Manifest struct { Name, Version, Namespace, Description string; Tags []string; Requires []Require }
type Require struct { Tool, Version, Source, Manager string } // parsed; NOT written to the lock (D4)

// Lock I/O — atomic write (temp+rename); deterministic serialization. No `requires`.
func ReadLock(repoRoot string) (LockFile, error)        // absent file → zero LockFile, nil err
func WriteLock(repoRoot string, lf LockFile) error
type LockFile  struct { LockfileVersion int; Origin string; Skills map[string]LockEntry }
type LockEntry struct { Version, Commit, TreeSha, Path string }
```

## Operations

```go
// Add vendors one skill from an already-resolved LOCAL origin into repoRoot's
// canonical .agents/skills/<name>/, mode-preserving and byte-identical (no
// injection — D6), and writes/updates the lock. It does NOT resolve origins,
// read config, or fetch — the caller supplies opts.OriginDir (a local git
// checkout) + opts.Ref. Refuses divergent overwrite unless opts.Force; opts.DryRun
// writes nothing. Idempotent on identical content.
func Add(opts AddOptions) (AddResult, error)
type AddOptions struct { OriginDir, Ref, Skill, RepoRoot string; Force, DryRun bool }
type AddResult  struct { Name, Version, Path, Commit, TreeSha string; Action Action; DryRun bool }
type Action     string // "vendored" | "unchanged" | "overwritten"

// Verify checks every vendored skill in repoRoot against the lock: label-honesty
// (recompute TreeSHA on HEAD), orphan/completeness (on-disk set = locked set),
// and dirty (uncommitted). Read-only; offline; deterministic; aggregates ALL
// findings. Returns a Report; returns a *VerifyFailure error when not ok so
// callers can branch, with the same Report attached.
func Verify(repoRoot string) (Report, error)
type Report  struct { OK bool; Counts Counts; Verdicts []Verdict }
type Counts  struct { Verified, Mismatch, Orphan, Missing, Dirty int }
type Verdict struct { Name, Path, Status, ExpectedTreeSha, ActualTreeSha, Reason string }
```

## Errors (typed, presentation-free)

```go
type VerifyFailure struct { Report Report }          // ≥1 non-ok verdict; CLI maps → exit 2
func (e *VerifyFailure) Error() string                // terse; CLI renders the Report richly
type GitError struct { ExitCode int; Stderr string }  // git invocation failure (gh pattern)
```
The CLI (`internal/cli`) maps `*VerifyFailure` → `ExitVerification(2)`, `GitError`/malformed-lock/not-a-repo → `*cli.UsageError` (1), and renders human/`--json`. The SDK itself prints nothing.

## git client (testability — gh pattern, research D7)

`skillcore` shells `git` through a small internal client with a **pluggable command constructor** (a `func(ctx, name string, args ...string) *exec.Cmd` field, default `exec.CommandContext`) and the `GitError` type, mirroring `gh/git`'s `Client`. Tests swap the constructor for a stub (unit, error paths) or run real `git` in a `t.TempDir()` (integration, ground truth). Output via injectable writers; never `os.Stdout` directly.

## Example: third-party consumer (SDK-1)

```go
import "github.com/skillrig/cli/pkg/skillcore"

rep, err := skillcore.Verify(repoRoot)
if err != nil {
    var vf *skillcore.VerifyFailure
    if errors.As(err, &vf) { renderMyOwnWay(vf.Report); os.Exit(2) } // caller owns presentation + exit policy
    log.Fatal(err)
}
fmt.Printf("%d skills verified\n", rep.Counts.Verified)
```

## Invariants (tested)

- `TreeSHA` value `Add` records (on the origin) == value `Verify` recomputes (on the consumer's committed tree) for identical content — by construction (both `git rev-parse`); proven by the `add → verify` round-trip and the relocation-invariance ground truth (data-model.md).
- No exported function writes to stdout/stderr or returns pre-formatted user text (Constitution V) — enforced by review + the CLI being the only renderer.
- Requires a git repository at `gitDir`/`repoRoot`; otherwise returns a `GitError`/typed error (the CLI renders "not a git repository").
