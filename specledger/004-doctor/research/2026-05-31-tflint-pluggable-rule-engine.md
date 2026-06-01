# Research: How tflint's Pluggable Rule Engine Works (and what `doctor` should borrow)

**Date**: 2026-05-31
**Context**: 004-doctor specifies `doctor` as a rules-based engine with a first-class developer-extensibility outcome (US7, FR-033/034). tflint is the canonical Go "pluggable rule engine" prior art. The user checked out the host (`/Users/vincentdesmet/specledger/skillrig/tflint`) and an example ruleset (`/Users/vincentdesmet/specledger/skillrig/tflint-ruleset-terraform`) to study its design before we lock doctor's engine architecture in `/plan`. This complements the earlier build-vs-buy spike (`2026-05-31-rule-engine-evaluation-design.md`), which concluded "hand-roll an in-process `Rule` interface."
**Time-box**: ~30 min (local source read: host ARCHITECTURE + developer-guide, plugin `main.go`, two real rules, the `tflint-plugin-sdk` `Rule`/`RuleSet` contracts from the module cache).
**Confidence**: High — read the actual SDK interfaces and shipping rule implementations, not docs alone. Versions: `tflint@v0.47.0`, `tflint-plugin-sdk@v0.17.0`, `tflint-ruleset-terraform@v0.4.0`.

## Question

How does tflint structure rules, register them, and run them as a pluggable engine — and which of those patterns should `doctor` adopt vs. deliberately reject, given doctor's rules are first-party and compiled-in (no third-party-plugin requirement)?

## Findings

### Finding 1: The `Rule` interface is tiny, and "optional methods" are handled by embedding a `DefaultRule`

From `tflint-plugin-sdk@v0.17.0/tflint/rule.go`, the effective `Rule` interface is just:

```go
type Rule interface {
    Name() string
    Enabled() bool          // enabled by default?
    Severity() Severity     // ERROR | WARNING | NOTICE
    Check(runner Runner) error
    Link() string           // optional — defaulted by DefaultRule
    Metadata() interface{}  // optional — defaulted by DefaultRule; "never referenced by the SDK"
    mustEmbedDefaultRule()  // compile-time forcing function
}

// DefaultRule supplies the optional methods; every rule embeds it.
type DefaultRule struct{}
func (r *DefaultRule) Link() string        { return "" }
func (r *DefaultRule) Metadata() interface{} { return nil }
func (r *DefaultRule) mustEmbedDefaultRule() {}
```

A real rule (`tflint-ruleset-terraform/rules/terraform_required_version.go`) is then just a struct embedding `DefaultRule` and overriding the four methods it cares about:

```go
type TerraformRequiredVersionRule struct{ tflint.DefaultRule }
func (r *TerraformRequiredVersionRule) Name() string       { return "terraform_required_version" }
func (r *TerraformRequiredVersionRule) Enabled() bool       { return true }
func (r *TerraformRequiredVersionRule) Severity() tflint.Severity { return tflint.WARNING }
func (r *TerraformRequiredVersionRule) Check(runner tflint.Runner) error { /* … */ }
```

Two idioms worth stealing:
- **Embed-for-defaults**: new rules override only what differs; optional methods (`Link`, `Metadata`) come free. Keeps the rule contract small while leaving room to grow.
- **`mustEmbedDefaultRule()`** is an unexported method on the interface that *only* `DefaultRule` can satisfy — a compile-time guarantee that every `Rule` embeds `DefaultRule`. That, in turn, means the SDK can **add new optional methods to the interface later without breaking existing rules** (they inherit the new default via the embed). This is the mechanism that makes the rule set *forward-compatible* — directly relevant to doctor's "add rules later without reworking the core" (FR-004).

`Metadata() interface{}` is an explicit, SDK-ignored escape hatch for custom rulesets to attach arbitrary per-rule data — a cheap extensibility seam.

### Finding 2: The `Runner` is the decoupling seam — rules are pure logic over an injected data provider

Rules never read files or fetch data themselves. They receive a `Runner` and ask it for everything:

```go
func (r *TerraformModuleVersionRule) Check(rr tflint.Runner) error {
    config := TerraformModuleVersionRuleConfig{}
    runner.DecodeRuleConfig(r.Name(), &config)   // per-rule typed config
    calls, _ := runner.GetModuleCalls()          // data access via Runner
    // … evaluate …
    runner.EmitIssue(r, "module … should specify a version", module.DefRange) // emit findings via Runner
}
```

The `Runner` exposes data-access (`GetModuleContent`, `GetModuleCalls`, `GetFiles`), expression evaluation, per-rule config decode (`DecodeRuleConfig`), and issue emission (`EmitIssue`). Because the rule's only contact with the outside world is this interface, a rule is **trivially unit-testable**: hand it a fake/in-memory `Runner` over fixture data and assert the emitted issues (which is exactly how `*_test.go` files in the ruleset work). This is the single most important pattern for doctor — it is the concrete realization of "rules are pure functions over a gathered fact set" that the prior spike recommended.

### Finding 3: A `RuleSet` is a named, versioned collection with built-in enable/disable/preset logic

From `tflint-plugin-sdk/tflint/ruleset.go`, `BuiltinRuleSet` is the reusable base a plugin embeds:

```go
type BuiltinRuleSet struct {
    Name, Version, Constraint string
    Rules        []Rule
    EnabledRules []Rule
}
func (r *BuiltinRuleSet) ApplyGlobalConfig(config *Config) error { /* filters Rules → EnabledRules
    by --only > per-rule config block > disabled_by_default > rule.Enabled() */ }
func (r *BuiltinRuleSet) NewRunner(runner Runner) (Runner, error) { return runner, nil } // injection hook
func (r *BuiltinRuleSet) ConfigSchema() *hclext.BodySchema { return nil }                // override for config
```

The example plugin's `main.go` is the whole wiring:

```go
plugin.Serve(&plugin.ServeOpts{RuleSet: &terraform.RuleSet{
    BuiltinRuleSet: tflint.BuiltinRuleSet{Name: "terraform", Version: project.Version},
    PresetRules: rules.PresetRules,
}})
```

And `rules/preset.go` is just a `map[string][]tflint.Rule{"all": {...}, "recommended": {...}}` — registration is a **plain slice/map of constructed rule values**, not reflection or codegen. Enable/disable, `--only`, and named presets are all resolved against `rule.Name()` + `rule.Enabled()`. doctor can mirror this exactly (a registry slice; optional preset groups) without any of the plugin machinery.

### Finding 4: The "pluggable" part is heavyweight gRPC subprocesses — and it exists for a reason doctor does NOT have

From the host `docs/developer-guide/architecture.md`: **"TFLint does not contain any rule implementations. Rules are provided as plugins, launched by TFLint as subprocesses and communicate over gRPC."** The mechanics:

- **Transport**: `github.com/hashicorp/go-plugin` launches each ruleset as a **separate binary subprocess**; host and plugin talk over **gRPC** (defined in `*.proto` in the SDK).
- **Bidirectional client/server**: the plugin runs a **"RuleSet" gRPC server** (host calls `ApplyConfig`, `Check`); the host runs a **"Runner" gRPC server** in a goroutine (the plugin calls back to fetch Terraform config, evaluate expressions, and `EmitIssue`). So the `Runner` a rule uses is, across the wire, a gRPC client to the host.
- **Discovery + install**: `plugin.Discovery` finds installed plugin binaries; `tflint --init` installs them from releases.

This architecture buys exactly one thing: **third-party, independently-authored, independently-released, separately-versioned rulesets** (`tflint-ruleset-aws`, `-google`, `-azure`, …) that the host knows nothing about at compile time, plus crash-isolation across that trust boundary. It costs a lot: a gRPC/proto contract, subprocess lifecycle management, an install/discovery system, and serialization of every data access.

**doctor has none of those requirements.** Its rules (`path-presence`, `mise-version-check`, `source-auth-reachability`, `integrity`, and future allowlist/audit/default-branch) are first-party, in-tree, and compiled into the one `skillrig` binary. There is no third-party ruleset ecosystem, no independent release cadence, no untrusted-plugin boundary. Paying for go-plugin/gRPC here would be pure ceremony — and it contradicts the repo's deliberately minimal dependency posture (3 direct deps, 15-line `go.sum`; the prior spike).

### Finding 5: tflint validates the *interface design* the prior spike already chose — at ecosystem scale

Net of Findings 1–4: tflint's value to us is **confirmation of the in-process design**, minus the transport. Everything doctor needs is in the *shape* (Rule interface + DefaultRule embed + Runner injection + RuleSet collection + Severity + preset registration); nothing doctor needs is in the *plumbing* (go-plugin, gRPC, proto, subprocess discovery/install). The prior build-vs-buy spike concluded "hand-roll an in-process `Rule` interface over a gathered fact set" — tflint is strong, real-world evidence that this interface scales to dozens of rules and an external contributor base, and it hands us a battle-tested API to copy. It also draws the **exact line** at which the heavyweight option becomes justified.

### Finding 6: doctor's engine is the natural foundation for roadmap 010 `lint` — and the closest analog to tflint's actual job

`docs/ROADMAP.md` lists **010 `lint`** — an *author-side conformance gate, a required PR check on the origin* that validates skills before they're published. That is **exactly tflint's job shape**: walk a set of artifacts (there, `*.tf`; here, `skills/*/SKILL.md` frontmatter + structure), run a set of named rules with severities, emit issues with actionable messages and (ideally) a SARIF/JSON form for CI. doctor and `lint` therefore want the **same engine**: a `Rule` over an injected fact context, a registry/preset, verdict→exit-class mapping, two-level output. Building doctor's engine with that reuse in mind (one `skillcore` rule engine; doctor and lint each supply their own rule set + fact gatherer) is the AP-04-consistent move and avoids a second parallel engine when 010 lands.

This reframes the out-of-process question (Finding 4) more concretely: **agentskills.io is an ecosystem standard** (003 adopted it for `SKILL.md`). If skillrig ever wants *third-party / org-authored, independently-released lint rules for agentskills* — the direct analog to tflint's `-aws`/`-google` rulesets — that is precisely the trigger that justifies adopting `go-plugin`/gRPC. So the layering is: **(v0) in-process rule engine shared by doctor; (010) same engine reused by `lint`; (future, on trigger) make that engine's rule set pluggable out-of-process for federated agentskills lint rules.** Designing the in-process `Rule`/`HealthContext` contract cleanly now is what keeps that future migration a transport swap rather than a rewrite — the same reason tflint can keep one `Rule` interface whether a rule is bundled or shipped as a plugin.

## Decisions

- **D1 — Adopt tflint's rule *interface design*, reject its *plugin transport*.** doctor's engine uses an in-process `Rule` interface + a `DefaultRule`-style embed for optional methods + a `Runner`/context injection seam + a registry slice (with optional preset groups). It does **not** use `hashicorp/go-plugin`, gRPC, `*.proto`, or subprocess plugins.
- **D2 — The `Runner`/context is doctor's `HealthContext`.** Rules receive an injected provider exposing gathered facts (vendored manifests + `requires`, `mise.toml` pins, PATH lookups, the GitHub-source auth prober, the `verify` integrity report) plus a findings collector (`EmitFinding` analog). Rules are pure over it — no rule does its own I/O — making each rule unit-testable against a fake context (satisfies FR-034/SC-009 and the US7 test requirement).
- **D3 — Copy the `embed-for-defaults` + forced-embed forward-compat idiom.** A `doctorrule.Base`-style embed supplies optional methods so new rules override only what they need, and (optionally) a `mustEmbed`-style marker keeps the interface extensible without breaking existing rules later. This is the concrete mechanism behind FR-004's "add rules without reworking the core."
- **D4 — Carry a Severity-like verdict + an "applicability/N-A" state.** tflint has ERROR/WARNING/NOTICE; doctor needs at least fail / warning / advisory / **N-A** (the `source-auth-reachability`-not-applicable case, FR-018) and must map verdicts → exit classes (3 prereq, 2 integrity, 0 incl. warnings) per FR-026/027. Model the verdict on a rule's emitted findings, like `EmitIssue`.
- **D5 — Defer per-rule config and named presets unless needed.** tflint's `DecodeRuleConfig` / `ApplyGlobalConfig` / `PresetRules` are valuable seams but doctor v0 runs a fixed rule set; keep the registry shape (so presets/config can be added later) but don't build the config layer now (YAGNI).
- **D6 — Design the engine as the *shared* foundation for roadmap 010 `lint`.** doctor and `lint` are the same engine over different fact sets (doctor = consumer environment/requirements; lint = origin-side `SKILL.md` conformance). Put the generic `Rule`/registry/verdict/render machinery in `skillcore` so 010 reuses it (one engine, AP-04) rather than spawning a parallel linter. The **out-of-process plugin trigger** is specifically *federated/third-party agentskills lint rules* — record it so the in-process choice is a conscious staging decision, not an oversight.

## Recommendations

1. **Plan the engine in `pkg/skillcore` as an in-process rule registry**: a `Rule` interface (`Name`/`Severity`/`Check(ctx) → findings` + embedded defaults), a `HealthContext` gathered once (deterministic local facts + online auth prober), `doctor` = gather → run `[]Rule` → collect findings → exit-class precedence → render. Mirror `verify.go`'s `Verdict`/`Report` vocabulary (AP-04: the integrity rule *calls* `Verify`).
2. **Write the US7 example/extension test against a fake `HealthContext`**, exactly as the ruleset's `*_test.go` files test rules against a fake `Runner` — this is the proof that a contributor can add + isolate-test a rule (FR-033/034, SC-009) with no CLI or network.
3. **Record the explicit out-of-process trigger** in the plan/architecture: adopt `go-plugin`/gRPC *only* if skillrig ever needs org-authored, independently-released, third-party doctor/lint-rule plugins for agentskills (the tflint situation) — analogous to the architecture's existing "MCP surface dispatches to the same `skillcore`" note. Until that trigger fires, in-process stands.
6. **Factor the engine for 010 `lint` reuse from day one** (per D6): keep the generic rule/registry/verdict/render core free of doctor-specific facts, so `lint` later supplies an origin-side fact gatherer + `SKILL.md`-conformance rule set against the *same* engine. Flag this cross-roadmap dependency in the plan so the 004 interface is reviewed with 010 in mind (and note it on the ROADMAP 010 row when docs are synced).
4. **Reuse tflint's enable/`Name()` convention** for rule identity in `--json` output and (future) selective enable/disable, so the output schema already has a stable per-rule key.
5. Keep the dependency footprint intact: this confirms **no rule-engine library and no plugin framework**; the only new dep remains the focused semver comparator from the prior spike (`Masterminds/semver/v3`).

## References

- Local: `/Users/vincentdesmet/specledger/skillrig/tflint/docs/developer-guide/architecture.md` (host/plugin gRPC flow, Runner server, Discovery)
- Local: `/Users/vincentdesmet/specledger/skillrig/tflint-ruleset-terraform/main.go` (`plugin.Serve` + `BuiltinRuleSet` wiring), `rules/preset.go` (registration map), `rules/terraform_required_version.go` + `rules/terraform_module_version.go` (real `Check`/`Runner`/`EmitIssue`/`DecodeRuleConfig` usage)
- Module cache: `tflint-plugin-sdk@v0.17.0/tflint/rule.go` (`Rule`/`DefaultRule`/`mustEmbedDefaultRule`), `…/tflint/ruleset.go` (`RuleSet`/`BuiltinRuleSet`/`ApplyGlobalConfig`)
- [terraform-linters/tflint](https://github.com/terraform-linters/tflint) · [tflint-plugin-sdk](https://github.com/terraform-linters/tflint-plugin-sdk) · [hashicorp/go-plugin](https://github.com/hashicorp/go-plugin)
- Companion spike: `specledger/004-doctor/research/2026-05-31-rule-engine-evaluation-design.md` (build-vs-buy; in-process `Rule` interface + `Masterminds/semver`)
- Spec: `004-doctor/spec.md` FR-004 (extensible engine), FR-033/034 + US7 (contributor adds a rule), FR-018 (N/A applicability), FR-026/027 (verdict → exit class)
