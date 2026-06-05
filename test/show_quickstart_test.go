// This file holds the TestQuickstart_* integration suite for feature
// 005-show-skill (the `skillrig show`/`info` command, issue #17). Each scenario
// maps 1:1 to a scenario in specledger/005-show-skill/quickstart.md.
//
// Like the 001/002/003 suites it builds the binary once (TestMain in
// quickstart_test.go) and execs it via run(). show is a Query command that reads
// the origin's index.json through the same resolver + catalog path search uses,
// so it reuses the 003 search substrate (searchConsumer / catalogFile) — a git
// consumer repo whose origin ships a hand-authored index.json read off disk and
// bound via SKILLRIG_ORIGIN. The point-lookup-specific behaviour (full
// untruncated description, info alias, skill-not-found exit 1) is what these
// scenarios pin; origin/convention/reachability failures share search's code
// path and are covered by the 003 suite.
package quickstart

import (
	"strings"
	"testing"
)

// showDescTail is a sentinel embedded near the END of the long description below,
// past the 80-char point where search truncates. show must print it; search must
// not — the observable proof that show closes issue #17's gap.
const showDescTail = "SENTINEL-TAIL-VISIBLE-ONLY-IN-SHOW"

// longDescription is a multi-line, well-over-80-char description: the exact shape
// search abbreviates to a single truncated line and show must render in full.
const longDescription = "Review a terraform plan for risk before apply: it inspects resource " +
	"deletions, replacements, and IAM changes, flags drift against the last applied state, " +
	"and summarizes the blast radius for a human approver.\n\n" +
	"It is read-only and never mutates the plan — " + showDescTail + "."

// showCatalog is a deterministic catalog whose first skill carries the long,
// multi-line description (plus topics) the show scenarios assert is rendered in
// full. A couple of extra skills keep it a realistic multi-skill origin.
func showCatalog() catalogFile {
	return catalogFile{
		SkillrigConvention: 1,
		Origin:             originRepo,
		Skills: []catalogSkill{
			{
				Name:        "terraform-plan-review",
				Version:     "1.4.0",
				Namespace:   "my-org",
				Description: longDescription,
				Topics:      []string{"platform-team", "terraform", "aws"},
				Path:        "skills/terraform-plan-review",
			},
			{
				Name:        "aws-iam-audit",
				Version:     "2.0.0",
				Namespace:   "my-org",
				Description: "Audit a terraform-managed AWS IAM policy set for drift.",
				Topics:      []string{"security", "aws"},
				Path:        "skills/aws-iam-audit",
			},
		},
	}
}

// show runs `skillrig show args...` (or any alias passed as the first arg by the
// caller) in the consumer with the origin bound via SKILLRIG_ORIGIN.
func (c searchConsumer) show(t *testing.T, args ...string) runResult {
	t.Helper()

	return run(t, runOpts{
		args: append([]string{"show"}, args...),
		cwd:  c.root,
		env:  map[string]string{"SKILLRIG_ORIGIN": originRepo},
	})
}

// ---------------------------------------------------------------------------
// US1 — Read one skill's full record
// ---------------------------------------------------------------------------

// TestQuickstart_ShowFullDescription — show prints the COMPLETE description
// (including the tail bytes search clips) plus the labelled fields + a footer
// hint, exit 0; and search over the same catalog truncates that description —
// the observable proof show closes issue #17's gap.
func TestQuickstart_ShowFullDescription(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, showCatalog())

	res := c.show(t, sampleSkill)
	if res.exit != 0 {
		t.Fatalf("show exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stderr != "" {
		t.Errorf("show success must keep stderr empty, got: %q", res.stderr)
	}

	// The defining contract of issue #17: the full, untruncated description body.
	if !strings.Contains(res.stdout, showDescTail) {
		t.Errorf("show must print the FULL description incl. the tail %q, got:\n%s", showDescTail, res.stdout)
	}

	// Labelled record: name/version, topics, path, and a runnable footer hint.
	for _, want := range []string{sampleSkill, sampleVersion, "terraform", "skills/terraform-plan-review", "skillrig add " + sampleSkill} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("show output missing %q:\n%s", want, res.stdout)
		}
	}

	// Cross-check: search over the SAME catalog truncates the same description —
	// the tail search would clip is absent (this is the gap show fills).
	sres := c.search(t, sampleSkill)
	if sres.exit != 0 {
		t.Fatalf("cross-check search exit = %d, want 0 (stderr: %s)", sres.exit, sres.stderr)
	}

	if strings.Contains(sres.stdout, showDescTail) {
		t.Errorf("search is expected to TRUNCATE the description (tail %q should be absent), got:\n%s", showDescTail, sres.stdout)
	}
}

// TestQuickstart_ShowInfoAlias — `skillrig info <skill>` is the `show` alias the
// issue named: byte-identical output to `skillrig show <skill>`.
func TestQuickstart_ShowInfoAlias(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, showCatalog())

	showRes := c.show(t, sampleSkill)
	if showRes.exit != 0 {
		t.Fatalf("show exit = %d, want 0 (stderr: %s)", showRes.exit, showRes.stderr)
	}

	infoRes := run(t, runOpts{
		args: []string{"info", sampleSkill},
		cwd:  c.root,
		env:  map[string]string{"SKILLRIG_ORIGIN": originRepo},
	})

	if infoRes.exit != 0 {
		t.Fatalf("info alias exit = %d, want 0 (stderr: %s)", infoRes.exit, infoRes.stderr)
	}

	if infoRes.stdout != showRes.stdout {
		t.Errorf("info alias output differs from show:\ninfo=%s\nshow=%s", infoRes.stdout, showRes.stdout)
	}
}

// TestQuickstart_ShowJSONComplete — --json parses and carries origin + a single
// `skill` object with the full field set, the description untruncated.
func TestQuickstart_ShowJSONComplete(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, showCatalog())

	res := c.show(t, sampleSkill, "--json")
	if res.exit != 0 {
		t.Fatalf("show --json exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	obj := decodeJSON(t, res.stdout)
	requireKeys(t, obj, "origin", "skill")

	skill, ok := obj["skill"].(map[string]any)
	if !ok {
		t.Fatalf("skill is not an object: %v", obj["skill"])
	}

	requireKeys(t, skill, "name", "version", "namespace", "description", "topics", "path")

	if skill["name"] != sampleSkill {
		t.Errorf("--json skill.name = %v, want %s", skill["name"], sampleSkill)
	}

	// The description must be the COMPLETE value (issue #17), not a clipped copy.
	desc, _ := skill["description"].(string)
	if !strings.Contains(desc, showDescTail) {
		t.Errorf("--json skill.description is truncated (missing tail %q): %q", showDescTail, desc)
	}
}

// TestQuickstart_ShowHelpExamples — show --help shows the purpose line + >=2
// runnable `skillrig show` examples (bounded shape).
func TestQuickstart_ShowHelpExamples(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"show", "--help"}})
	if res.exit != 0 {
		t.Fatalf("show --help exit = %d, want 0", res.exit)
	}

	if n := countExampleLines(res.stdout, "skillrig show"); n < 2 {
		t.Errorf("show --help shows %d 'skillrig show' example lines, want >= 2:\n%s", n, res.stdout)
	}
}

// ---------------------------------------------------------------------------
// US2 — Trustworthy failures
// ---------------------------------------------------------------------------

// TestQuickstart_ShowSkillNotFound — a NAMED skill the origin does not publish is
// exit 1 with a 3-part error (distinct from search, where an empty result is
// exit 0).
func TestQuickstart_ShowSkillNotFound(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, showCatalog())

	res := c.show(t, "no-such-skill")
	if res.exit != 1 {
		t.Fatalf("show of an unknown skill exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "no-such-skill")
	assertContains(t, "why", res.stderr, "why:")
	assertContains(t, "fix", res.stderr, "skillrig search")
}

// TestQuickstart_ShowNoOriginConfigured — with no resolvable origin, show fails
// exit 1 with the shared no-origin what/why/fix (run init first).
func TestQuickstart_ShowNoOriginConfigured(t *testing.T) {
	t.Parallel()

	// A fresh temp cwd with NO SKILLRIG_ORIGIN and no project/global config.
	res := run(t, runOpts{args: []string{"show", sampleSkill}})
	if res.exit != 1 {
		t.Fatalf("show with no origin exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "no origin")
	assertContains(t, "fix", res.stderr, "skillrig init")
}

// TestQuickstart_ShowMissingArg — show with no skill argument is exit 1 with the
// navigational "requires exactly one argument" message, not cobra's bare arg error.
func TestQuickstart_ShowMissingArg(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"show"}})
	if res.exit != 1 {
		t.Fatalf("show with no arg exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	assertContains(t, "what", res.stderr, "requires exactly one argument")
	assertContains(t, "fix", res.stderr, "skillrig show <skill>")
}
