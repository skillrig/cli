# Quickstart — Acceptance Contract: `005-show-skill`

Each scenario is an executable `TestQuickstart_*` (Constitution §II): concrete invocations, observable output, exit codes, and **output-shape** assertions (full untruncated human body; parseable + complete `--json`; 3-part errors). Resolves [issue #17](https://github.com/skillrig/cli/issues/17): humans had no way to read a skill's *full* description — `search` truncates it to ~80 chars and only an agent could recover it via `--json | jq`.

`show` is a **Query** command (cli.md Pattern Classification): a deterministic read of the origin's `index.json` through the same resolver + catalog-load + convention-gate path `search` uses (AP-04), drilling into ONE named skill instead of listing many. `info` is an alias.

## Test substrate
- Reuses the 003 search substrate: a git **consumer** repo whose origin (`my-org/my-skills`) ships an `index.json` read straight off disk, bound via `SKILLRIG_ORIGIN` (the `searchConsumer` helper). A dedicated `showCatalog()` carries one skill with a **long, multi-line** description (> the 80-char search truncation width) so non-truncation is observable.
- Origin/convention/reachability failures share `search`'s code path, so they are covered there; `show` adds only its point-lookup-specific scenarios below.

---

## US1 — Read one skill's full record · P1

**`TestQuickstart_ShowFullDescription`** — `skillrig show <skill>` on a skill with a long multi-line description prints the description **in full** (the exact tail bytes search would clip are present) + the name/version/topics/path labels + a footer hint pointing at `add`; **exit 0**. Cross-check: `skillrig search <term>` over the same catalog truncates the same description (ellipsis, tail absent) — proving show closes the gap.
**`TestQuickstart_ShowInfoAlias`** — `skillrig info <skill>` is byte-identical to `skillrig show <skill>` (the alias the issue named).
**`TestQuickstart_ShowJSONComplete`** — `--json` parses (`json.Unmarshal` ok) and carries `origin` + a `skill` object with the full field set (`name`/`version`/`namespace`/`description`/`topics`/`path`), the `description` value untruncated (field-presence + full body, not truncation-absence).
**`TestQuickstart_ShowHelpExamples`** — `show --help` shows the purpose line + ≥2 runnable `skillrig show` examples.

## US2 — Trustworthy failures · P2

**`TestQuickstart_ShowSkillNotFound`** — `skillrig show no-such-skill` against a populated origin → **exit 1**, empty stdout, a 3-part error: what (the named skill is not in the origin), why (no skill by that exact name), fix (run `skillrig search`). Distinct from `search`, where an empty result is exit 0.
**`TestQuickstart_ShowNoOriginConfigured`** — `skillrig show <skill>` with no resolvable origin → **exit 1**, the shared `no origin configured` what/why/fix (run `skillrig init`).
**`TestQuickstart_ShowMissingArg`** — `skillrig show` (no args) → **exit 1** with the navigational "requires exactly one argument" message (not cobra's bare "accepts 1 arg(s)").

---

### Traceability
| US | Scenarios | Maps to |
|---|---|---|
| US1 read | ShowFullDescription / ShowInfoAlias / ShowJSONComplete / ShowHelpExamples | issue #17 (full human-readable description) |
| US2 failures | ShowSkillNotFound / ShowNoOriginConfigured / ShowMissingArg | cli.md Principle 2 (errors-as-navigation), exit-code contract |
