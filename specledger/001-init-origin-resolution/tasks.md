# Tasks Index: CLI Initialization & Origin Resolution

Issue-graph index for this feature. Tasks live in the `sl issue` store, **not** in this file — this is a navigational anchor only.

## Feature Tracking

* **Epic**: `SL-227789`
* **User Stories Source**: `specledger/001-init-origin-resolution/spec.md`
* **Planning**: `specledger/001-init-origin-resolution/plan.md`
* **Research**: `research.md` + `research/2026-05-24-interactive-prompt-library.md` (spike S1)
* **Data Model**: `data-model.md` (config.toml fixture + precedence matrix)
* **Contracts**: `contracts/init.md`, `contracts/resolve.md`
* **Acceptance contract**: `quickstart.md` (Constitution II — scenarios map 1:1 to Go tests)

## Query Hints

```bash
sl issue list --label "spec:001-init-origin-resolution"      # all issues for this feature
sl issue list --label "phase:us1"                            # one phase
sl issue list --status open
sl issue show SL-227789                                       # epic
sl issue list --tree                                          # dependency graph
```

## Phase / Task Structure

**Epic** `SL-227789` → 6 phases (type `feature`) → tasks (type `task`).

### Phase 1 — Setup `SL-a9fb37` (blocks everything)
| Task | ID |
|------|----|
| Initialize Go module + main.go | `SL-1eabbd` |
| Cobra root command (--json/--verbose, SilenceUsage/Errors) | `SL-19f6f3` |
| Exit-code constants + error→exit mapping | `SL-3e3fa9` |
| Lint gate (.golangci.yml + gofmt/vet) | `SL-d11cfb` |
| Output helper (human-compact vs --json) | `SL-eb2528` |

### Phase 2 — Foundational `SL-b881e9` (blocks all stories)
| Task | ID |
|------|----|
| Origin type + ParseOrigin + tests | `SL-e2130a` |
| Config structs + TOML load/save (atomic) + tests | `SL-2ac8c8` |
| ResolveOrigin single resolver (AP-06) | `SL-e69c8b` |
| Ground-truth fixtures (config.toml + precedence matrix) | `SL-60a982` |

### Phase 3 — US1: Bind via `skillrig init` (P1) 🎯 MVP `SL-ec18e7`
| Task | ID |
|------|----|
| Quickstart integration test harness | `SL-4958e2` |
| Write failing TestQuickstart_* (US1) + output-shape asserts | `SL-db8e96` |
| Implement `skillrig init` | `SL-2e4214` |

### Phase 4 — US2: Precedence resolution (P2) `SL-ba7eaa`
| Task | ID |
|------|----|
| TestResolveOrigin precedence table (rows 1–7 + FromSubdir) | `SL-ca8e55` |

### Phase 5 — US3: Actionable failures (P3) `SL-d990f5`
| Task | ID |
|------|----|
| Write failing error-path tests (3-part asserts) | `SL-03ebb3` |
| Implement errors-as-navigation | `SL-05dbc5` |

### Phase 6 — Polish `SL-60678c` (blocked by all stories)
| Task | ID |
|------|----|
| Author skillrig-init agent skill (Constitution IX) | `SL-0990e2` |
| Full gate green: gofmt/vet/golangci-lint + go test ./... | `SL-1819a2` |
| Usage docs (README → docs website for config) | `SL-804dc6` |

## Dependency Graph

```
Setup(SL-a9fb37) ─▶ Foundational(SL-b881e9) ─┬▶ US1(SL-ec18e7) ─┐
                 └───────────────────────────┼▶ US2(SL-ba7eaa) ─┼▶ Polish(SL-60678c)
                                             └▶ US3(SL-d990f5) ─┘
Foundational internals:  ParseOrigin(e2130a) + Config(2ac8c8) ─▶ ResolveOrigin(e69c8b)
                         Fixtures(60a982) + ResolveOrigin ─▶ US2 precedence tests(ca8e55)
US1 (TDD):  harness(4958e2) ─▶ failing tests(db8e96) ─▶ init impl(2e4214)  [also needs e2130a, 2ac8c8]
US3 (TDD):  harness(4958e2) ─▶ failing error tests(03ebb3) ─▶ errors impl(05dbc5)  [also needs init 2e4214]
```

## Implementation Strategy

- **MVP = US1** (`skillrig init` binds an origin) — the smallest shippable slice; delivers SC-001/SC-002.
- Build order: **Setup → Foundational → US1 → US2 → US3 → Polish.** US2 and US3 can proceed in parallel once Foundational + US1 land.
- **TDD per Constitution II**: each story's quickstart-derived tests are written **failing first**, then implementation turns them green. A story is DONE only when its quickstart scenarios match the user stories AND its `TestQuickstart_*`/`TestResolveOrigin_*` tests pass.

## Definition of Done Summary (key items)

| Issue | DoD highlights |
|-------|----------------|
| SL-2e4214 (init) | quickstart scenarios match US1; BindProject/JSON/Idempotent/Rebind/Global/Help/Prompt tests pass; origin-only; git-root write + cwd fallback |
| SL-e69c8b (resolver) | single ResolveOrigin (AP-06); pure/deterministic; env>project>global |
| SL-ca8e55 (US2 tests) | TestResolveOrigin rows 1–7 + FromSubdir pass; real fixtures, no mocks |
| SL-05dbc5 (US3 errors) | MalformedOrigin + NoOriginNonInteractive pass; what/why/fix to stderr; exit 1 |
| SL-1819a2 (gate) | gofmt/vet/golangci-lint clean; go test ./... green (full quickstart suite) |
| SL-0990e2 (skill) | skillrig-init agent skill with verified trigger keywords (Constitution IX) |

---

> Index only. All task detail (design / acceptance criteria / DoD) lives in the `sl issue` store — query with the hints above.
