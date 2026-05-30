# Quickstart: `add` + `verify` (Executable Acceptance Contract)

**Feature**: `002-skillcore-verify` | **Date**: 2026-05-30
Per Constitution II, **each scenario below maps 1:1 to a `TestQuickstart_<name>` integration test** that builds and execs the real `skillrig` binary. Every scenario states its **output-shape assertions** (not just `Contains`): human line-count bound, `--json` parseable + structurally complete, error = what/why/fix as distinct checks + the correct exit code.

## Test harness & helpers

- Build the binary once (existing 001 harness); exec it with a controlled cwd + env.
- **`bootstrapOrigin(t) (dir, ref)`** — `git init` a `t.TempDir()`, copy `test/testdata/sample-origin/**` in, `git add -A && git commit` with **pinned** `GIT_AUTHOR_*`/`GIT_COMMITTER_*` name=`skillrig` email=`ci@skillrig.dev` date=`2026-01-01T00:00:00Z` (so the commit SHA is reproducible — D8). Returns the origin dir + ref (`HEAD`/`main`).
- **`newConsumerRepo(t) dir`** — `git init` a `t.TempDir()`; run `skillrig init --origin <originDir>` in it (the origin value is the local path — clarified 2026-05-30).
- **`commitAll(t, dir, msg)`** — stage + commit (pinned identity) so `verify` sees committed content.
- **Ground truth & oracle independence** (research D11/D12): the fixture is a *canonical, design-aligned* sample origin (`test/testdata/sample-origin/` mirroring the origin layout — `.skillrig-origin.toml` + `skills/<skill>/{SKILL.md,skill.toml}`); the sample skill (`terraform-plan-review@1.4.0`) and its tree-SHA are **illustrative real-`git` output, not a locked constant** (both the fixture and the existing `skillrig-origin` template are pre-canonical samples). Integration tests compute the **expected** tree-SHA with **raw `git`** (`git -C <origin> rev-parse <ref>:skills/<skill>`), **never** through `skillcore` — the binary under test uses `skillcore`, so the oracle must stay independent (Constitution III, no circular validation). A separate `skillcore` unit test pins `skillcore.TreeSHA == ` raw `git` output.

---

## US1 — Vendor a skill (`add`)

### TestQuickstart_AddVendorsSkill  (US1.1)
- **Given** a consumer git repo whose origin is a local checkout containing `terraform-plan-review`.
- **When** `skillrig add terraform-plan-review`.
- **Then** exit `0`; `.agents/skills/terraform-plan-review/{SKILL.md,skill.toml}` exist **byte-identical** to the origin (modes preserved); `.skillrig/skills-lock.json` has one entry `{version:"1.4.0", commit, treeSha, path:".agents/skills/terraform-plan-review"}` with **`treeSha` == the value `git rev-parse` gives for the origin subtree** (ground truth) and **no `requires` field**.
- **Shape**: human ≤ 2 lines incl. footer (`→ commit it, then run: skillrig verify`). `--json`: `json.Unmarshal` ok; keys `ok,name,version,path,commit,treeSha,action,dryRun` all present; `action=="vendored"`.

### TestQuickstart_AddIdempotent  (US1.2)
- **Given** `terraform-plan-review` already vendored.
- **When** `skillrig add terraform-plan-review` again (identical content).
- **Then** exit `0`; lock unchanged (one entry, no dup); `--json action=="unchanged"`; human prints `… already vendored (no change)`.

### TestQuickstart_AddDryRunWritesNothing  (US1.5)
- **When** `skillrig add terraform-plan-review --dry-run` in a fresh consumer.
- **Then** exit `0`; **no** `.agents/skills/` and **no** `.skillrig/skills-lock.json` created; human prefixed `would vendor …`; `--json dryRun==true, action=="vendored"`.

### TestQuickstart_AddRefusesDivergentWithoutForce  (US1.4)
- **Given** `terraform-plan-review` vendored; a byte of its `SKILL.md` then edited.
- **When** `skillrig add terraform-plan-review` (no `--force`).
- **Then** exit `1`; **error has 3 parts** — what: `refusing to overwrite .agents/skills/terraform-plan-review`; why: `on-disk content diverges from the recorded fingerprint`; fix: `re-run with --force`. Files unchanged.
- **And** `skillrig add terraform-plan-review --force` → exit `0`, `action=="overwritten"`, content restored to origin.

### TestQuickstart_AddRequiresOrigin
- **Given** a git repo with **no** origin (no `init`, no `SKILLRIG_ORIGIN`, no global).
- **When** `skillrig add terraform-plan-review`.
- **Then** exit `1`; 3-part error — what: `no origin configured`; why: `no SKILLRIG_ORIGIN / project / global origin`; fix: `skillrig init --origin OWNER/REPO`.

### TestQuickstart_AddNotGitRepo
- **When** `skillrig add …` in a non-git tmpdir (origin via `SKILLRIG_ORIGIN`).
- **Then** exit `1`; what: `not a git repository`; why: `project-scope add vendors into the repo's canonical .agents/skills and writes a lock that verify checks against git`; fix: `run inside the repo (or git init first)`. (Project-scope precondition — a future --global path is exempt; see spec Out of Scope.)

---

## US2 — Prove a skill is unmodified (`verify` label-honesty)

### TestQuickstart_VerifyPasses  (US2.1)
- **Given** `terraform-plan-review` vendored **and committed**.
- **When** `skillrig verify`.
- **Then** exit `0`; human exactly 2 lines (`verified 1 skills ✓` + `→ all match their recorded version`); `--json ok==true`, `counts.verified==1`, one verdict `status=="ok"` whose `expectedTreeSha==actualTreeSha==` the ground-truth tree-SHA.
- **And (FR-014 / SC-006)**: the vendored skill declares `[[requires]]` (oxid, terraform) for tools **absent** in the test environment, yet verify still exits `0` — proving it performs **no** prerequisite check (integrity-only).

### TestQuickstart_VerifyIsReadOnly  (FR-015)
- **Given** `terraform-plan-review` vendored + committed.
- **When** `skillrig verify` (pass) and again after a tamper (fail).
- **Then** in **both** runs the working tree is **unchanged** — assert `git status --porcelain` is identical before/after, and `.skillrig/skills-lock.json` + the skill files are byte-for-byte untouched (verify writes nothing).

### TestQuickstart_VerifyDetectsTamper  (US2.2, SC-003)
- **Given** the skill vendored + committed; then one byte of `SKILL.md` changed **and committed**.
- **When** `skillrig verify`.
- **Then** exit `2`; the failing verdict `status=="mismatch"` **names** `terraform-plan-review` with `expectedTreeSha` (recorded) ≠ `actualTreeSha` (on-disk). Human ≤ findings + K lines.

### TestQuickstart_VerifyDirtyUncommitted  (D2)
- **Given** the skill vendored but **not committed** (or committed then edited-without-commit).
- **When** `skillrig verify`.
- **Then** exit `2`; verdict `status=="dirty"`, reason names the uncommitted/locally-modified skill and says to commit it — *distinct* from `mismatch`.

### TestQuickstart_VerifyEmptyRepoPasses  (US2.4)
- **Given** a fresh git repo, no skills, no lock.
- **When** `skillrig verify`.
- **Then** exit `0` (nothing to verify), not an error; `--json ok==true, counts all zero, verdicts==[]`.

---

## US3 — Orphan / completeness (`verify`)

### TestQuickstart_VerifyDetectsOrphan  (US3.1)
- **Given** `terraform-plan-review` vendored + committed; plus an **unlocked** `.agents/skills/rogue/skill.toml` created + committed (no `add`).
- **When** `skillrig verify`.
- **Then** exit `2`; a verdict `status=="orphan"` naming `rogue` (present on disk, no lock entry).

### TestQuickstart_VerifyDetectsMissing  (US3.2)
- **Given** the skill vendored + committed; then `.agents/skills/terraform-plan-review/` removed + committed (lock still references it).
- **When** `skillrig verify`.
- **Then** exit `2`; verdict `status=="missing"` naming `terraform-plan-review`.

### TestQuickstart_VerifyAggregatesAllFailures  (US3.4, FR-012)
- **Given** one skill tampered **and** one orphan present (committed).
- **When** `skillrig verify`.
- **Then** exit `2`; **both** reported in one run — `counts.mismatch>=1 && counts.orphan>=1`, `len(verdicts)` covers all skills; the check did **not** stop at the first failure.

---

## US4 — Scriptable outcome (exit codes + `--json`)

### TestQuickstart_VerifyExitCodeMatrix  (US4.1, FR-022)
- Assert the stable mapping over the scenarios above: pass→`0`, any verification failure→`2`, malformed-lock/not-a-repo→`1`, and **never `3`**. Repeated runs on unchanged input yield the identical code (deterministic).

### TestQuickstart_VerifyJSONComplete  (US4.2)
- For both a passing and a failing run: `--json` on stdout is `json.Unmarshal`-able and **structurally complete** — top-level `ok,counts,verdicts`; `counts` has all five keys; **every** checked skill appears as a verdict with all six fields. Diagnostics go to **stderr** (stdout stays clean JSON: `verify --json 2>/dev/null | jq .` parses).

### TestQuickstart_VerifyMalformedLock  (US4.4)
- **Given** a `.skillrig/skills-lock.json` that is not valid JSON (or wrong `lockfileVersion`).
- **When** `skillrig verify`.
- **Then** exit `1` (usage/config, **distinct** from verification failure `2`); 3-part error naming the file + raw cause under `--verbose`; **not** a raw parser dump.

### TestQuickstart_AddHelpExamples / TestQuickstart_VerifyHelpExamples  (FR-018 / SC-009)
- **When** `skillrig add --help` and `skillrig verify --help`.
- **Then** exit `0`; each help output contains a one-line purpose **and ≥2 usage examples** (assert ≥2 lines beginning `skillrig add `/`skillrig verify ` in the Examples block) — sufficient to construct a correct invocation without external docs. (Output-shape, not a single `Contains`.)

---

## Round-trip (the core acceptance contract)

### TestQuickstart_AddThenVerifyRoundTrip  (SC-001, SC-005)
- `init --origin <local>` → `add terraform-plan-review` → `commitAll` → `verify` ⇒ exit `0`, in **two commands** (+commit), **zero network**, **no hand-authored lock**. Proves `add` records exactly what `verify` recomputes (same git-canonical tree-SHA, both sides — research D1). Then a one-byte tamper + commit ⇒ `verify` exit `2`. This is the headline scenario; all primitives exercised end-to-end on real git output.

---

> **Coverage check** (for `/specledger.verify`): every spec user story (US1–US4) and acceptance scenario, and every Output/Errors/Exit row in `contracts/{add,verify}.md`, has a `TestQuickstart_*` above. Deferred behaviors (prereq/exit-3, conflict markers, network, symlinks) have **no** scenarios here — by design (spec Out of Scope).
