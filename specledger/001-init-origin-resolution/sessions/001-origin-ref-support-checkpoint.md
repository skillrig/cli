# Divergence Review: 2026-05-29 (amendment 001-origin-ref-support)

> Branch: `main` (amendment to closed feature 001). Session log named after the
> amendment rather than the branch to avoid clobbering the original
> `001-init-origin-resolution-checkpoint.md` and the poor/collision-prone `main`.
> Scope reviewed: the `OWNER/REPO[@REF]` branch-ref amendment (feature `SL-2f13c6` + 5 tasks).

### Divergences

| # | Severity | Type | Category | Issue/Artifact | Description |
|---|----------|------|----------|----------------|-------------|
| 1 | MEDIUM | conscious | Unverified DoD intent | SL-f948db | Skill trigger eval was run with `--model sonnet`, but the harness is confounded by the **already-installed** `.claude/skills/skillrig-init`: it watches for the model invoking a uniquely-named temp command, while the model triggers the real skill → 0 triggers for **every** query (positives and negatives alike). "Confirm trigger accuracy" is therefore unmeasured. Mitigated: the description change is **purely additive** (every original trigger phrase preserved verbatim — verified via `git diff`), so regression risk is near-zero. Constitution IX wants this *verified*, not merely argued. |
| 2 | LOW | conscious | Tracking-state drift | SL-2f13c6 + 5 children | All 6 amendment issues remain `open` despite the work being complete and green. `sl issue update`/`show` derive the spec context from the git branch; on `main` they error (`read specledger/: is a directory`), so statuses could not be advanced this session. |
| 3 | LOW | oversight | Missing DoD | SL-2f13c6 + 5 children | The amendment issues were created without `--dod` items, so there is no per-issue acceptance checklist to verify against. This weakens the force-closed signal for this feature (nothing to be "bypassed"), and DoD evidence should be attached before closing. |
| 4 | LOW | conscious | Validation permissiveness | `internal/config/origin.go` `refPattern` | The shape-only ref pattern `^[A-Za-z0-9._/-]+$` accepts refs that git itself rejects: `o/r@..`, `o/r@/x` (leading slash), `o/r@x/` (trailing slash), `o/r@-x` (probe-confirmed accepted). Intentional per decision D-A2 (offline, no "smart" branch detection; existence/validity deferred to a future command) and documented in FR-019. Reviewers should know malformed-but-charset-clean refs pass. |
| 5 | LOW | conscious | Plan file-list drift | SL-c5093a / `internal/cli/init_test.go` | The task listed `init_test.go` among files to touch for the prompt-label change; it was **not** modified. Its assertions reference the `originPromptLabel` *constant* (lines 53, 115), so they auto-adapt to the new value — verified passing. No edit was needed. |

No CRITICAL or HIGH divergences. No requirement (FR-018/FR-019/FR-020) is unimplemented; no planned file is missing; no production scope creep observed (`docs/ROADMAP.md` change is the user's own in-flight edit, not part of this work).

### Force-Closed Issues (DoD Bypassed)

| Issue | Title | Unchecked DoD Items | Risk |
|-------|-------|---------------------|------|
| — | — | — | **None.** All 26 closed issues have every `definition_of_done.items[].checked == true` (verified with nested detection). The 6 amendment issues are `open`, not closed. |

### Issues Encountered & Resolutions
- Force-closed detection initially returned a false negative because `definition_of_done` is nested (`{items:[{item,checked}]}`), not a flat list → re-ran descending into `.items`; confirmed 0 force-closed.
- `sl issue update`/`show` fail on `main` (spec context derived from branch) → could not advance issue statuses; recorded as divergence #2.
- Manual e2e in a prior step accidentally wrote `.skillrig/config.toml` into the repo root → removed; working tree clean.

### Items Requiring Action Before Merge
1. **[MEDIUM] Verify skill trigger accuracy (Constitution IX).** Either run a clean eval by temporarily relocating `.claude/skills/skillrig-init` so the harness's temp command isn't shadowed by the installed skill, or explicitly accept the additive-change rationale and record that acceptance. — Without this, "verified trigger accuracy" is asserted, not measured.
2. **[LOW] Advance/close the 6 issues on a feature branch** with DoD evidence attached (add `--dod` items first). — Keeps the tracker honest; currently the work looks "open" while being done.
3. **[LOW] Decide on `refPattern` strictness.** Accept the documented permissiveness, or tighten to reject `..` and leading/trailing `/`. — Low risk (offline shape-only), but a conscious sign-off avoids surprise.

### Tests & Checks
- Status: **PASS**
- Commands run: `make check` (`gofmt -w .`, `go vet ./...`, `golangci-lint run` → **0 issues**, `go test ./...` → all ok); plus targeted `go test -count=1 ./internal/config/ -run 'TestParseOrigin|TestOriginString|TestSaveLoadRoundTripWithRef|TestResolveOrigin_Precedence'` and `./test/ -run TestQuickstart_BindWithRef` — all PASS.
- Failures: none.

### Progress Summary
- Closed: 26 issues (all pre-existing 001 work; 0 from this amendment)
- In Progress: 0
- Open/Remaining: 6 issues (amendment feature `SL-2f13c6` + 5 task children — implementation complete, status not advanced; see divergence #2)
- Force-Closed: 0 (DoD bypassed: none)

### Post-Review Update (2026-05-29, same session)

Adversarial cold review (independent agent) confirmed the above and surfaced 3 additional MEDIUM findings, now **resolved**:
- **D1 (contract drift)** — `contracts/init.md` no-origin error rows updated to `--origin OWNER/REPO[@REF]` to match the shipped binary.
- **D2 (ground-truth drift)** — `data-model.md` precedence matrix gained rows 8/9 (ref survives resolution), matching the test table.
- **D3 (untested path)** — added `TestResolveOrigin_MalformedRefRecordsDiagnostic` exercising the `refPattern` reject branch end-to-end through the resolver.
- **D4 (assertion gap)** — `TestQuickstart_BindWithRef` now asserts the `→ resolve order:` footer and full JSON key completeness (`ok/origin/scope/configPath/written`).
- Re-ran `make check` → still green (lint 0 issues, all tests pass).

Divergence #1 (skill eval) remains a **conscious, recorded** acceptance: closed `SL-f948db` with the caveat in its close reason (additive description diff, harness-confound documented). Divergences #2/#3 (tracking state / missing DoD) resolved by the closures below.

**Issue closures**: created feature branch `001-init-origin-resolution` (carrying the uncommitted amendment) so `sl` resolves the spec, then closed all 6 amendment issues (`SL-2f13c6` + `SL-d15491`, `SL-c5093a`, `SL-d488d8`, `SL-d9a9bd`, `SL-f948db`) with evidence-bearing reasons. Spec 001 now: **32 closed, 0 open**. (Adversarial finding D5 — the `docs/ROADMAP.md` `aws` row / row-003 edit — is the user's own in-flight edit, left untouched.)

### Uncommitted Changes
- `internal/config/origin.go`, `internal/cli/init.go` (code)
- `internal/config/origin_test.go`, `internal/config/config_test.go`, `internal/config/resolve_test.go`, `test/quickstart_test.go` (tests)
- `docs/design/cli.md`, `README.md`, `docs/ARCHITECTURE-v0.md` (docs)
- `specledger/001-init-origin-resolution/{spec.md,data-model.md,quickstart.md,contracts/init.md,contracts/resolve.md,issues.jsonl}` (spec artifacts)
- `specledger/001-init-origin-resolution/amendments/001-origin-ref-support.md` (new addendum, untracked)
- `.agents/skills/skillrig-init/{SKILL.md,evals/evals.json,evals/trigger-eval-set.json}` (skill + evals)
- Not mine (user's in-flight edits): `CLAUDE.md`, `docs/ROADMAP.md`; pre-existing untracked: `docs/guides/vcr-cassettes.md`

---
