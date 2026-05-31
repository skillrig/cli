// This file holds the TestQuickstart_* integration suite for feature
// 003-search-remote, the scenarios that are exercisable end-to-end against the
// real binary today: the discover (search, US1) and catalog-generation (index,
// US5) groups, plus the add --help shape (US2 SC-008). Each maps 1:1 to a
// scenario in specledger/003-search-remote/quickstart.md.
//
// Like the 001/002 suites it builds the binary once (TestMain in
// quickstart_test.go) and execs it via run(). It reuses the 002 fixture
// helpers (git, copyTree, sampleOriginDir, pinnedGitEnv, originRepo,
// decodeJSON, requireKeys, countExampleLines) and the RAW-git oracle
// discipline: every fixture is bootstrapped and every expected value computed
// with raw git, NEVER through skillcore (Constitution III / research D11).
//
// SUBSTRATE NOTE (S4 / D6). The remote-acquisition group (US2 remote add, US3
// --pin, US4 auth/unreachable failures) runs against a real file:// bare repo
// for the CLI's origin (FIX-1 gave config.ParseOrigin a local/file:// form and
// pkg/skillcore.cloneURL a file:// seam, so `add` with no local checkout clones
// a t.TempDir bare repo over a real git transport — offline, no github.com).
// newRemoteOrigin git-inits a working tree (committed + a v-tag), clones it
// --bare, and binds SKILLRIG_ORIGIN=file://<bare>; the RAW-git oracle reads the
// expected treeSha straight from that bare repo (never skillcore, D11).
//
// Injected git failures (US4 auth/unreachable/private-not-found) are produced
// at the integration tier by a fake `git` on the binary's PATH (fakeGitBin) that
// passes every command through to the real git EXCEPT `clone`, which it fails
// with a crafted (exit 128, stderr) — the integration analog of the
// pkg/skillcore commandContext stub seam (which, being an unexported field, is
// only reachable from a pkg/skillcore unit test). The clone-phase failure trips
// the catalog gate before any subtree is fetched, so the CLI renders the
// auth/unreachable/not-found class distinctly. The typed-class assertions for
// those classes live as unit tests in pkg/skillcore (TestClassifyFetchError),
// per the quickstart's own "unit-level via the stub seam" note.
package quickstart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Search/index fixture helpers (local-path origin form — the testable path).
// ---------------------------------------------------------------------------

// catalogSkill is one entry in a hand-authored index.json. The fields mirror
// skillcore.CatalogEntry (the search --json projection), so an entry written
// here round-trips through the binary's search reader.
type catalogSkill struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Namespace   string   `json:"namespace"`
	Description string   `json:"description"`
	Topics      []string `json:"topics"`
	Path        string   `json:"path"`
}

// catalogFile is a hand-authored index.json: the convention version search
// gates on, the origin identity, and the skills it lists.
type catalogFile struct {
	SkillrigConvention int            `json:"skillrigConvention"`
	Origin             string         `json:"origin"`
	Skills             []catalogSkill `json:"skills"`
}

// searchCatalog is a small, deterministic multi-skill catalog used by the
// search scenarios that need more than the single-skill fixture: ordering,
// token-AND query, and topic filtering. The names are chosen so the relevance
// buckets are unambiguous (an exact-name and a name-substring hit for the
// "terraform" query, plus a description-only and an unrelated skill).
func searchCatalog() catalogFile {
	return catalogFile{
		SkillrigConvention: 1,
		Origin:             originRepo,
		Skills: []catalogSkill{
			{
				Name:        "terraform-plan-review",
				Version:     "1.4.0",
				Namespace:   "my-org",
				Description: "Review a terraform plan for risk before apply.",
				Topics:      []string{"platform-team", "terraform", "aws"},
				Path:        "skills/terraform-plan-review",
			},
			{
				Name:        "terraform-module-lint",
				Version:     "0.9.0",
				Namespace:   "my-org",
				Description: "Lint a terraform module for style and structure.",
				Topics:      []string{"terraform"},
				Path:        "skills/terraform-module-lint",
			},
			{
				Name:        "aws-iam-audit",
				Version:     "2.0.0",
				Namespace:   "my-org",
				Description: "Audit a terraform-managed AWS IAM policy set for drift.",
				Topics:      []string{"security", "aws"},
				Path:        "skills/aws-iam-audit",
			},
			{
				Name:        "k8s-manifest-check",
				Version:     "1.0.0",
				Namespace:   "my-org",
				Description: "Validate kubernetes manifests before rollout.",
				Topics:      []string{"kubernetes"},
				Path:        "skills/k8s-manifest-check",
			},
		},
	}
}

// searchConsumer is a consumer repo whose origin (a local checkout at
// <root>/my-org/my-skills) ships only an index.json — search reads the catalog
// straight off disk (it does not need the origin to be a committed git repo,
// only the consumer to be one), so a catalog fixture is all the substrate the
// search scenarios require.
type searchConsumer struct {
	root string
}

// newSearchConsumer builds a git consumer repo, writes the given catalog as the
// origin's index.json at <root>/my-org/my-skills/index.json, and binds the
// origin via SKILLRIG_ORIGIN at call sites (search resolves it like every
// command). The origin checkout is kept out of the consumer index so it never
// pollutes the working tree the way 002 arranges it.
func newSearchConsumer(t *testing.T, cat catalogFile) searchConsumer {
	t.Helper()
	requireGit(t)

	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")

	originDir := filepath.Join(root, filepath.FromSlash(originRepo))
	if err := os.MkdirAll(originDir, 0o755); err != nil {
		t.Fatalf("mkdir origin %s: %v", originDir, err)
	}

	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}

	if err := os.WriteFile(filepath.Join(originDir, "index.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write index.json: %v", err)
	}

	return searchConsumer{root: root}
}

// search runs `skillrig search args...` in the consumer with the origin bound
// via SKILLRIG_ORIGIN (the env precedence the resolver honors).
func (c searchConsumer) search(t *testing.T, args ...string) runResult {
	t.Helper()

	return run(t, runOpts{
		args: append([]string{"search"}, args...),
		cwd:  c.root,
		env:  map[string]string{"SKILLRIG_ORIGIN": originRepo},
	})
}

// originRepoBootstrap bootstraps a committed local origin git repo from a
// source tree (the committed fixture, or a multi-skill tree built in a
// t.TempDir) so `index` — which runs INSIDE the origin repo and finds its root
// via git — has a real work tree. It returns the origin repo root.
func bootstrapOriginRepo(t *testing.T, src string) string {
	t.Helper()
	requireGit(t)

	dir := t.TempDir()
	copyTree(t, src, dir)

	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "origin fixture")

	return dir
}

// indexIn runs `skillrig index args...` with cwd inside the origin repo (index
// is an origin-side generator: it locates the origin root from the cwd).
func indexIn(t *testing.T, originRoot string, args ...string) runResult {
	t.Helper()

	return run(t, runOpts{args: append([]string{"index"}, args...), cwd: originRoot})
}

// writeSkillMD writes a SKILL.md with the given frontmatter under
// originRoot/skills/<name>/. raw is the full frontmatter body (between the
// fences) so a scenario can author a malformed or version-less manifest.
func writeSkillMD(t *testing.T, originRoot, name, frontmatter, body string) {
	t.Helper()

	dir := filepath.Join(originRoot, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill %s: %v", name, err)
	}

	content := "---\n" + frontmatter + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md for %s: %v", name, err)
	}
}

// writeOriginConfig writes a minimal .skillrig-origin.toml (convention 1) at
// originRoot so index can read the convention version and skills dir.
func writeOriginConfig(t *testing.T, originRoot string) {
	t.Helper()

	cfg := "convention_version = 1\norigin = \"" + originRepo + "\"\nskills_dir = \"skills\"\n"
	if err := os.WriteFile(filepath.Join(originRoot, ".skillrig-origin.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write origin config: %v", err)
	}
}

// searchEntry mirrors one search --json skill entry for completeness assertions.
type searchEntry struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Namespace   string   `json:"namespace"`
	Description string   `json:"description"`
	Topics      []string `json:"topics"`
	Path        string   `json:"path"`
}

// searchPayload mirrors the search --json top-level object.
type searchPayload struct {
	Origin string        `json:"origin"`
	Skills []searchEntry `json:"skills"`
}

// decodeSearch strictly decodes a search --json payload.
func decodeSearch(t *testing.T, stdout string) searchPayload {
	t.Helper()

	var p searchPayload
	if err := json.Unmarshal([]byte(stdout), &p); err != nil {
		t.Fatalf("search --json is not parseable: %v\n%s", err, stdout)
	}

	return p
}

// searchNames returns the matched skill names in result order (search --json
// preserves the binary's relevance+name ordering).
func searchNames(p searchPayload) []string {
	out := make([]string, len(p.Skills))
	for i, s := range p.Skills {
		out[i] = s.Name
	}

	return out
}

// ---------------------------------------------------------------------------
// US1 — Discover (search)
// ---------------------------------------------------------------------------

// TestQuickstart_SearchListsSkills — search with no query lists every skill the
// origin publishes (name/version/desc) + a footer hint; bounded human shape.
func TestQuickstart_SearchListsSkills(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, searchCatalog())

	res := c.search(t)
	if res.exit != 0 {
		t.Fatalf("search exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	matches := len(searchCatalog().Skills)

	lines := nonEmptyLines(res.stdout)
	if len(lines) > matches+5 {
		t.Errorf("human stdout has %d lines, want <= matches+5 (%d):\n%s", len(lines), matches+5, res.stdout)
	}

	for _, name := range []string{"terraform-plan-review", "aws-iam-audit", "k8s-manifest-check"} {
		if !strings.Contains(res.stdout, name) {
			t.Errorf("listing omits %q:\n%s", name, res.stdout)
		}
	}

	if !strings.Contains(res.stdout, "skillrig add") {
		t.Errorf("listing missing the add footer hint:\n%s", res.stdout)
	}
}

// TestQuickstart_SearchQueryMatchesNameDesc — `search terraform plan` keeps only
// skills whose name+description+topics contain BOTH terms (token-AND); a skill
// matching one term but not the other is excluded (FR-002).
func TestQuickstart_SearchQueryMatchesNameDesc(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, searchCatalog())

	res := c.search(t, "--json", "terraform", "plan")
	if res.exit != 0 {
		t.Fatalf("search exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	names := searchNames(decodeSearch(t, res.stdout))

	// Only terraform-plan-review carries both "terraform" AND "plan". The other
	// terraform skills lack "plan"; aws-iam-audit has "terraform" (description)
	// but not "plan"; k8s has neither.
	if len(names) != 1 || names[0] != "terraform-plan-review" {
		t.Errorf("token-AND query 'terraform plan' = %v, want exactly [terraform-plan-review]", names)
	}
}

// TestQuickstart_SearchOrderingDeterministic — a query hitting several skills is
// ordered by the fixed relevance bucket then name, and is byte-identical across
// two runs (D8/N6, SC-002).
func TestQuickstart_SearchOrderingDeterministic(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, searchCatalog())

	first := c.search(t, "--json", "terraform")
	second := c.search(t, "--json", "terraform")

	if first.exit != 0 || second.exit != 0 {
		t.Fatalf("search exits = %d/%d, want 0/0 (stderr: %s)", first.exit, second.exit, first.stderr)
	}

	if first.stdout != second.stdout {
		t.Errorf("search ordering not byte-identical across runs:\nA=%s\nB=%s", first.stdout, second.stdout)
	}

	names := searchNames(decodeSearch(t, first.stdout))

	// "terraform" hits all three terraform-ish skills. Bucket order: the two
	// name-substring hits (terraform-module-lint, terraform-plan-review) outrank
	// the description-only hit (aws-iam-audit); within the name bucket, ties
	// break lexicographically by name (module-lint < plan-review). k8s does not
	// match at all.
	want := []string{"terraform-module-lint", "terraform-plan-review", "aws-iam-audit"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("ordering = %v, want %v (relevance bucket then name)", names, want)
	}
}

// TestQuickstart_SearchFilterByTopic — `search --topic aws` lists only aws-topic
// skills and is identical across two runs.
func TestQuickstart_SearchFilterByTopic(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, searchCatalog())

	first := c.search(t, "--json", "--topic", "aws")
	second := c.search(t, "--json", "--topic", "aws")

	if first.exit != 0 {
		t.Fatalf("search exit = %d, want 0 (stderr: %s)", first.exit, first.stderr)
	}

	if first.stdout != second.stdout {
		t.Errorf("topic filter not deterministic:\nA=%s\nB=%s", first.stdout, second.stdout)
	}

	names := searchNames(decodeSearch(t, first.stdout))

	// Exactly the two aws-topic skills (ordered by name: aws-iam-audit then
	// terraform-plan-review); the two non-aws skills are excluded.
	want := []string{"aws-iam-audit", "terraform-plan-review"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("--topic aws = %v, want %v", names, want)
	}
}

// TestQuickstart_SearchEmptyResult — `search --topic nonesuch` reports no match
// and is still success (exit 0, FR-004).
func TestQuickstart_SearchEmptyResult(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, searchCatalog())

	res := c.search(t, "--topic", "nonesuch")
	if res.exit != 0 {
		t.Fatalf("empty-result search exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if !strings.Contains(res.stdout, "no skills matched") {
		t.Errorf("human output = %q, want it to say 'no skills matched'", res.stdout)
	}

	// --json variant: an empty result is the [] skills array, not null.
	jsonRes := c.search(t, "--json", "--topic", "nonesuch")
	if jsonRes.exit != 0 {
		t.Fatalf("empty-result search --json exit = %d, want 0", jsonRes.exit)
	}

	p := decodeSearch(t, jsonRes.stdout)
	if len(p.Skills) != 0 {
		t.Errorf("empty result --json skills = %v, want []", p.Skills)
	}

	if !strings.Contains(jsonRes.stdout, "\"skills\":[]") {
		t.Errorf("empty result should serialize skills as [], got:\n%s", jsonRes.stdout)
	}
}

// TestQuickstart_SearchJSONComplete — --json parses and every entry carries the
// full field set add needs (field-presence, not truncation).
func TestQuickstart_SearchJSONComplete(t *testing.T) {
	t.Parallel()

	c := newSearchConsumer(t, searchCatalog())

	res := c.search(t, "--json")
	if res.exit != 0 {
		t.Fatalf("search --json exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	// Top-level structural completeness.
	obj := decodeJSON(t, res.stdout)
	requireKeys(t, obj, "origin", "skills")

	rawSkills, ok := obj["skills"].([]any)
	if !ok {
		t.Fatalf("skills is not an array: %v", obj["skills"])
	}

	if len(rawSkills) == 0 {
		t.Fatal("expected at least one skill to assert per-entry completeness")
	}

	// Every entry carries name/version/namespace/description/topics/path.
	for i, raw := range rawSkills {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("skills[%d] is not an object: %v", i, raw)
		}

		requireKeys(t, entry, "name", "version", "namespace", "description", "topics", "path")
	}
}

// TestQuickstart_SearchConventionMismatch — a catalog declaring
// skillrigConvention 2 fails with exit 1 and a 3-part compatibility message
// ("update skillrig").
func TestQuickstart_SearchConventionMismatch(t *testing.T) {
	t.Parallel()

	cat := searchCatalog()
	cat.SkillrigConvention = 2
	c := newSearchConsumer(t, cat)

	res := c.search(t)
	if res.exit != 1 {
		t.Fatalf("convention-2 search exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	// Three distinct parts: what (a version mismatch), why, fix (update skillrig).
	assertContains(t, "what", res.stderr, "convention")
	assertContains(t, "why", res.stderr, "why:")
	assertContains(t, "fix", res.stderr, "update skillrig")
}

// TestQuickstart_SearchConventionBoundary (C1) — the exact-match gate: a catalog
// declaring convention 0 AND one omitting the field each fail (a lower/missing
// convention does NOT silently pass), while convention 1 passes. Pins the
// non-">" boundary so FR-016/SC-005 is unambiguous.
func TestQuickstart_SearchConventionBoundary(t *testing.T) {
	t.Parallel()

	// convention == 1 passes.
	pass := newSearchConsumer(t, searchCatalog())
	if res := pass.search(t); res.exit != 0 {
		t.Fatalf("convention-1 search exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	// convention == 0 (explicit) fails exit 1.
	zeroCat := searchCatalog()
	zeroCat.SkillrigConvention = 0
	zero := newSearchConsumer(t, zeroCat)

	if res := zero.search(t); res.exit != 1 {
		t.Errorf("convention-0 search exit = %d, want 1 (a lower/zero convention must not pass)", res.exit)
	}

	// convention absent (field omitted → JSON default 0) also fails exit 1. Write
	// a catalog object WITHOUT the skillrigConvention key.
	c := newSearchConsumer(t, searchCatalog())
	absent := map[string]any{"origin": originRepo, "skills": []any{}}

	data, err := json.MarshalIndent(absent, "", "  ")
	if err != nil {
		t.Fatalf("marshal absent-convention catalog: %v", err)
	}

	path := filepath.Join(c.root, filepath.FromSlash(originRepo), "index.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("rewrite catalog without convention: %v", err)
	}

	if res := c.search(t); res.exit != 1 {
		t.Errorf("absent-convention search exit = %d, want 1 (an absent convention must not pass)", res.exit)
	}
}

// TestQuickstart_SearchHelpExamples — search --help shows the purpose line + >=2
// runnable examples (bounded shape).
func TestQuickstart_SearchHelpExamples(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"search", "--help"}})
	if res.exit != 0 {
		t.Fatalf("search --help exit = %d, want 0", res.exit)
	}

	if n := countExampleLines(res.stdout, "skillrig search"); n < 2 {
		t.Errorf("search --help shows %d 'skillrig search' example lines, want >= 2:\n%s", n, res.stdout)
	}
}

// ---------------------------------------------------------------------------
// US2 — add --help (the testable slice of the remote-acquisition group)
// ---------------------------------------------------------------------------

// TestQuickstart_AddHelpShowsPinExample (C5/SC-008) — add --help shows >=2
// runnable examples, one of which is the --pin form. (The base add --help shape
// is asserted by the 002 suite's TestQuickstart_AddHelpExamples; this pins the
// 003-specific --pin example requirement.)
func TestQuickstart_AddHelpShowsPinExample(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"add", "--help"}})
	if res.exit != 0 {
		t.Fatalf("add --help exit = %d, want 0", res.exit)
	}

	if n := countExampleLines(res.stdout, "skillrig add"); n < 2 {
		t.Errorf("add --help shows %d 'skillrig add' example lines, want >= 2:\n%s", n, res.stdout)
	}

	if !strings.Contains(res.stdout, "--pin") {
		t.Errorf("add --help must document a --pin example (SC-008), got:\n%s", res.stdout)
	}
}

// ---------------------------------------------------------------------------
// US5 — Catalog generation (index)
// ---------------------------------------------------------------------------

// committedIndex reads the committed fixture index.json (the producer==artifact
// oracle for IndexMatchesCommitted).
func committedIndex(t *testing.T) string {
	t.Helper()

	return readFile(t, filepath.Join("testdata", "sample-origin", "index.json"))
}

// TestQuickstart_IndexGenerates — `index` over the origin fixture writes an
// index.json whose entries match the skills' frontmatter, INCLUDING topics.
func TestQuickstart_IndexGenerates(t *testing.T) {
	t.Parallel()

	originRoot := bootstrapOriginRepo(t, sampleOriginDir(t))

	res := indexIn(t, originRoot)
	if res.exit != 0 {
		t.Fatalf("index exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if !strings.Contains(res.stdout, "indexed 1 skill") {
		t.Errorf("index human output = %q, want it to report 1 indexed skill", res.stdout)
	}

	var cat catalogFile
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(originRoot, "index.json"))), &cat); err != nil {
		t.Fatalf("generated index.json not parseable: %v", err)
	}

	if cat.SkillrigConvention != 1 {
		t.Errorf("generated convention = %d, want 1 (read from .skillrig-origin.toml)", cat.SkillrigConvention)
	}

	if len(cat.Skills) != 1 {
		t.Fatalf("generated skills = %d, want 1", len(cat.Skills))
	}

	entry := cat.Skills[0]
	if entry.Name != sampleSkill || entry.Version != sampleVersion {
		t.Errorf("entry name/version = %q/%q, want %q/%q", entry.Name, entry.Version, sampleSkill, sampleVersion)
	}

	// Topics are the field the old build-index.sh dropped — assert they survive.
	wantTopics := []string{"platform-team", "terraform", "aws"}
	if strings.Join(entry.Topics, ",") != strings.Join(wantTopics, ",") {
		t.Errorf("entry topics = %v, want %v (topics must be carried)", entry.Topics, wantTopics)
	}

	// --json summary is structurally complete.
	jsonRes := indexIn(t, originRoot, "--json")
	obj := decodeJSON(t, jsonRes.stdout)
	requireKeys(t, obj, "out", "skills", "convention")
}

// TestQuickstart_IndexDeterministic — running index twice over an unchanged
// skill set yields byte-identical output (SC-009).
func TestQuickstart_IndexDeterministic(t *testing.T) {
	t.Parallel()

	originRoot := bootstrapOriginRepo(t, sampleOriginDir(t))

	if res := indexIn(t, originRoot); res.exit != 0 {
		t.Fatalf("first index exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	firstBytes := readFile(t, filepath.Join(originRoot, "index.json"))

	if res := indexIn(t, originRoot); res.exit != 0 {
		t.Fatalf("second index exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	secondBytes := readFile(t, filepath.Join(originRoot, "index.json"))
	if firstBytes != secondBytes {
		t.Errorf("index is not deterministic:\nfirst=%s\nsecond=%s", firstBytes, secondBytes)
	}
}

// TestQuickstart_IndexMatchesCommitted — `index` output equals the committed
// PoC index.json (producer == artifact oracle). The committed fixture
// index.json is the ground truth; regenerating it MUST reproduce it byte for
// byte.
//
// De-circularization (FIX-5 / M1): byte-equality alone cannot catch the
// PascalCase-requires-keys bug, because the producer and the committed fixture
// were generated the same (formerly buggy) way — they would agree on "Tool"
// just as readily as on "tool". So this also decodes the committed fixture's
// requires through a struct with `json:"tool"` tags and asserts the field is
// populated: that fails iff the JSON emits "Tool"/PascalCase (data-model §2),
// pinning the lowercase-key contract independently of the producer==artifact
// comparison.
func TestQuickstart_IndexMatchesCommitted(t *testing.T) {
	t.Parallel()

	originRoot := bootstrapOriginRepo(t, sampleOriginDir(t))

	if res := indexIn(t, originRoot); res.exit != 0 {
		t.Fatalf("index exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	got := readFile(t, filepath.Join(originRoot, "index.json"))
	want := committedIndex(t)

	if got != want {
		t.Errorf("regenerated index.json != committed fixture (producer/artifact drift):\ngot=%s\nwant=%s", got, want)
	}

	assertRequiresKeysLowercase(t, []byte(want))
	assertVersionConstraintsUnescaped(t, []byte(want))
}

// assertVersionConstraintsUnescaped asserts the catalog's raw bytes carry the
// readable ">=" version constraint, NOT Go's default HTML-escaped ">="
// (FIX-5/M1). The catalog MUST be marshaled with SetEscapeHTML(false); this
// raw-byte check is the other half of de-circularizing the producer==artifact
// oracle so the escaping regression cannot hide behind a matching fixture.
func assertVersionConstraintsUnescaped(t *testing.T, indexJSON []byte) {
	t.Helper()

	// "\\u003e" is the 6 ASCII bytes backslash-u-0-0-3-e — Go's HTML-escaped
	// form of '>'. An interpreted literal (not a raw `...`) is required so the
	// sequence is those bytes, not a literal '>' rune.
	if bytes.Contains(indexJSON, []byte("\\u003e")) {
		t.Errorf("index.json HTML-escapes version constraints (\\u003e) — marshal the catalog "+
			"with SetEscapeHTML(false) so \">=\" stays readable; got:\n%s", indexJSON)
	}

	if !bytes.Contains(indexJSON, []byte(">=")) {
		t.Errorf("index.json has no readable \">=\" constraint — the unescaped-constraint "+
			"assertion needs at least one to be meaningful; got:\n%s", indexJSON)
	}
}

// requiresProbe decodes just the requires list of the first catalog skill via
// lowercase `json` tags. If the catalog emitted PascalCase keys ("Tool"), Tool
// would unmarshal to "" — the discriminator the lowercase-key assertion checks.
type requiresProbe struct {
	Skills []struct {
		Requires []struct {
			Tool    string `json:"tool"`
			Version string `json:"version"`
		} `json:"requires"`
	} `json:"skills"`
}

// assertRequiresKeysLowercase decodes the catalog and asserts every requires
// entry's keys are lowercase (data-model §2): a non-empty .tool proves the JSON
// used "tool", not "Tool". It breaks the circular producer==artifact oracle in
// IndexMatchesCommitted so the PascalCase-requires bug (FIX-5/M1) cannot hide.
func assertRequiresKeysLowercase(t *testing.T, indexJSON []byte) {
	t.Helper()

	var probe requiresProbe
	if err := json.Unmarshal(indexJSON, &probe); err != nil {
		t.Fatalf("index.json is not parseable: %v\n%s", err, indexJSON)
	}

	if len(probe.Skills) == 0 {
		t.Fatal("index.json has no skills to assert requires-key casing on")
	}

	sawRequire := false

	for i, s := range probe.Skills {
		for j, r := range s.Requires {
			sawRequire = true

			if r.Tool == "" {
				t.Errorf("skills[%d].requires[%d].tool is empty — requires keys must be lowercase "+
					"\"tool\"/\"version\" (data-model §2), not PascalCase; got:\n%s", i, j, indexJSON)
			}
		}
	}

	if !sawRequire {
		t.Fatal("no requires entries found — the lowercase-key assertion needs at least one to be meaningful")
	}
}

// TestQuickstart_IndexMalformedFrontmatter — a skill with broken frontmatter
// fails index with exit 1 naming the offending SKILL.md.
func TestQuickstart_IndexMalformedFrontmatter(t *testing.T) {
	t.Parallel()
	requireGit(t)

	originRoot := t.TempDir()
	writeOriginConfig(t, originRoot)
	// Unterminated YAML flow sequence — a parse error, not a schema error.
	writeSkillMD(t, originRoot, "broken", "name: [unterminated", "# Broken")

	git(t, originRoot, "init", "-q", "-b", "main")
	git(t, originRoot, "add", "-A")
	git(t, originRoot, "commit", "-q", "-m", "broken skill")

	res := indexIn(t, originRoot)
	if res.exit != 1 {
		t.Fatalf("malformed-frontmatter index exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	// The error must name the offending file so the maintainer can fix it.
	if !strings.Contains(res.stderr, filepath.Join("skills", "broken", "SKILL.md")) {
		t.Errorf("error must name the offending SKILL.md, got:\n%s", res.stderr)
	}
}

// TestQuickstart_IndexNotInOrigin (C8) — running index outside an origin repo
// (a git repo with no .skillrig-origin.toml) fails exit 1 with the what/why/fix
// "run inside the origin repo" navigation.
func TestQuickstart_IndexNotInOrigin(t *testing.T) {
	t.Parallel()
	requireGit(t)

	// A git repo, but with no .skillrig-origin.toml → not an origin repo.
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")

	res := indexIn(t, root)
	if res.exit != 1 {
		t.Fatalf("index outside an origin exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "not in an origin repository")
	assertContains(t, "why", res.stderr, "why:")
	assertContains(t, "fix", res.stderr, "inside the origin repo")
}

// TestQuickstart_IndexMissingVersion (C9) — a skill whose frontmatter omits the
// required x-skillrig.version fails index with exit 1 naming the offending
// SKILL.md (the catalog-entry validation rule; guards the seed-enrichment
// precondition of IndexMatchesCommitted).
func TestQuickstart_IndexMissingVersion(t *testing.T) {
	t.Parallel()
	requireGit(t)

	originRoot := t.TempDir()
	writeOriginConfig(t, originRoot)

	// Valid YAML, name matches the directory, but no x-skillrig.version.
	fm := "name: novers\n" +
		"description: a skill missing its version\n" +
		"metadata:\n" +
		"  x-skillrig.namespace: my-org"
	writeSkillMD(t, originRoot, "novers", fm, "# Novers")

	git(t, originRoot, "init", "-q", "-b", "main")
	git(t, originRoot, "add", "-A")
	git(t, originRoot, "commit", "-q", "-m", "versionless skill")

	res := indexIn(t, originRoot)
	if res.exit != 1 {
		t.Fatalf("versionless-skill index exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	if !strings.Contains(res.stderr, filepath.Join("skills", "novers", "SKILL.md")) {
		t.Errorf("error must name the offending SKILL.md, got:\n%s", res.stderr)
	}

	// And it must point at the missing version specifically (errors-as-navigation).
	if !strings.Contains(res.stderr, "x-skillrig.version") {
		t.Errorf("error must cite the missing x-skillrig.version, got:\n%s", res.stderr)
	}
}

// ---------------------------------------------------------------------------
// US2/US3/US4 — remote acquisition, --pin, injected failures (file:// substrate)
//
// The CLI's origin is a real file:// bare repo built in t.TempDir(); add with no
// local checkout clones it over a real git transport (FIX-1's file:// seam),
// offline. Every expected treeSha is the RAW-git oracle read straight from that
// bare repo (rawTreeSHA → `git rev-parse <ref>:<path>`), NEVER skillcore (D11).
// ---------------------------------------------------------------------------

// pinTag is the immutable release tag the remote-origin fixture publishes for
// the sample skill: the full-tag form of the bare semver sampleVersion under the
// origin's name-vSEMVER scheme. `--pin v1.4.0` expands to exactly this.
const pinTag = sampleSkill + "-v" + sampleVersion

// remoteOrigin is a file:// bare-repo origin: a committed working tree (with the
// sample skill + an index.json + a release tag) pushed into a bare repo. The CLI
// is pointed at it via SKILLRIG_ORIGIN=file://<bareDir>, so the remote-fetch path
// clones it without any local checkout.
type remoteOrigin struct {
	// bareDir is the bare git repo the CLI clones from (the file:// target).
	bareDir string
	// cloneURL is file://<bareDir>, the SKILLRIG_ORIGIN value the CLI resolves.
	cloneURL string
}

// newRemoteOrigin builds the file:// bare-repo substrate from the committed
// sample-origin fixture: it copies the fixture into a work tree, writes its
// index.json (so the convention gate the remote add runs sees skillrigConvention
// 1), commits with the pinned identity, tags the release (pinTag), then clones
// it --bare. The bare repo's default branch is main so an unpinned add resolves
// the origin @ref (HEAD/main); the tag makes a pinned add reproducible.
func newRemoteOrigin(t *testing.T) remoteOrigin {
	t.Helper()
	requireGit(t)

	work := t.TempDir()
	copyTree(t, sampleOriginDir(t), work)

	// The committed fixture already ships an index.json; copyTree carried it, so
	// the convention gate reads skillrigConvention 1 straight from the fixture.
	git(t, work, "init", "-q", "-b", "main")
	git(t, work, "add", "-A")
	git(t, work, "commit", "-q", "-m", "origin fixture")
	git(t, work, "tag", pinTag)

	bareDir := filepath.Join(t.TempDir(), "origin.git")
	git(t, work, "clone", "-q", "--bare", work, bareDir)

	return remoteOrigin{bareDir: bareDir, cloneURL: "file://" + bareDir}
}

// newRemoteOriginConvention mirrors newRemoteOrigin but rewrites the work-tree
// index.json so its declared skillrigConvention is `conv` instead of the
// fixture's 1, before committing and cloning --bare. It is a localized byte
// rewrite of the single convention token (not a full JSON re-encode), so the rest
// of the catalog — including the `requires` blocks catalogFile does not model —
// survives untouched. It is the substrate for the remote convention-gate test.
func newRemoteOriginConvention(t *testing.T, conv int) remoteOrigin {
	t.Helper()
	requireGit(t)

	work := t.TempDir()
	copyTree(t, sampleOriginDir(t), work)

	// Localized rewrite: flip the declared convention from the fixture's 1 to conv.
	indexPath := filepath.Join(work, "index.json")
	before := readFile(t, indexPath)
	after := strings.Replace(before, `"skillrigConvention": 1`, fmt.Sprintf(`"skillrigConvention": %d`, conv), 1)

	if after == before {
		t.Fatalf("index.json did not contain the expected convention token to rewrite:\n%s", before)
	}

	if err := os.WriteFile(indexPath, []byte(after), 0o644); err != nil {
		t.Fatalf("rewrite index.json convention: %v", err)
	}

	git(t, work, "init", "-q", "-b", "main")
	git(t, work, "add", "-A")
	git(t, work, "commit", "-q", "-m", "origin fixture (convention "+strconv.Itoa(conv)+")")
	git(t, work, "tag", pinTag)

	bareDir := filepath.Join(t.TempDir(), "origin.git")
	git(t, work, "clone", "-q", "--bare", work, bareDir)

	return remoteOrigin{bareDir: bareDir, cloneURL: "file://" + bareDir}
}

// rawTree returns the RAW-git tree-SHA of the sample skill subtree at ref in the
// bare origin (the independent oracle, D11). The bare repo carries the full
// history, so `git rev-parse <ref>:<path>` resolves the same tree object the CLI
// will fetch and fingerprint.
func (o remoteOrigin) rawTree(t *testing.T, ref string) string {
	t.Helper()

	return rawTreeSHA(t, o.bareDir, ref, originSubtree)
}

// remoteConsumer is a fresh git repo (no origin checkout) that vendors from a
// file:// origin via SKILLRIG_ORIGIN.
type remoteConsumer struct {
	root     string
	cloneURL string
}

// newRemoteConsumer git-inits a consumer repo bound (via env at call sites) to
// the remote origin. There is NO local OWNER/REPO checkout under it, so add must
// take the remote-fetch path against the file:// origin.
func newRemoteConsumer(t *testing.T, o remoteOrigin) remoteConsumer {
	t.Helper()
	requireGit(t)

	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")

	return remoteConsumer{root: root, cloneURL: o.cloneURL}
}

// add runs `skillrig add args...` in the consumer with the origin bound via
// SKILLRIG_ORIGIN=file://<bare> (the env precedence the resolver honors).
func (c remoteConsumer) add(t *testing.T, args ...string) runResult {
	t.Helper()

	return run(t, runOpts{
		args: append([]string{"add"}, args...),
		cwd:  c.root,
		env:  map[string]string{"SKILLRIG_ORIGIN": c.cloneURL},
	})
}

// verify runs `skillrig verify` in the consumer. verify reads the lock + git and
// needs no origin, so no SKILLRIG_ORIGIN is bound — proving the vendored result
// stands on its own after a remote add.
func (c remoteConsumer) verify(t *testing.T, args ...string) runResult {
	t.Helper()

	return run(t, runOpts{args: append([]string{"verify"}, args...), cwd: c.root})
}

// fakeGitBin writes a `git` shim into a fresh dir and returns the dir, for
// prepending to the binary's PATH. The shim passes EVERY git invocation through
// to the real git EXCEPT `clone`, which it fails with the crafted (exit 128,
// stderr) — the integration analog of pkg/skillcore's commandContext stub seam.
// gitToplevel (rev-parse) still succeeds, so the failure surfaces precisely at
// the remote fetch's clone phase (the catalog gate), letting the CLI render the
// auth/unreachable/not-found class distinctly.
func fakeGitBin(t *testing.T, stderr string) string {
	t.Helper()

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH; skipping injected-failure scenario")
	}

	dir := t.TempDir()

	// For `clone` (the first positional arg), emit the crafted stderr and exit
	// 128; otherwise exec the real git with all args. The crafted stderr is
	// single-quoted in the heredoc-free script, so it must contain no single quote.
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = clone ]; then\n" +
		"  printf '%s\\n' " + shellQuote(stderr) + " 1>&2\n" +
		"  exit 128\n" +
		"fi\n" +
		"exec " + shellQuote(realGit) + " \"$@\"\n"

	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	return dir
}

// shellQuote single-quotes s for POSIX sh, escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// addWithFakeGit runs `skillrig add <skill>` against the remote origin with the
// fake git (failing clones with stderr) prepended to the binary's PATH. The PATH
// override is placed in opts.env, which os/exec dedupes keeping the last value,
// so the shim shadows the real git for the child process only.
func (c remoteConsumer) addWithFakeGit(t *testing.T, stderr string, args ...string) runResult {
	t.Helper()

	binDir := fakeGitBin(t, stderr)

	return run(t, runOpts{
		args: append([]string{"add"}, args...),
		cwd:  c.root,
		env: map[string]string{
			"SKILLRIG_ORIGIN": c.cloneURL,
			"PATH":            binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		},
	})
}

// TestQuickstart_AddRemoteNoLocalCopy (US2) — given a file:// origin and NO local
// checkout, add vendors the subtree byte-identical to the origin, records a lock
// entry whose treeSha == the RAW-git ground truth, and a subsequent verify (no
// origin needed) exits 0.
func TestQuickstart_AddRemoteNoLocalCopy(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)
	wantTree := o.rawTree(t, "HEAD")

	res := c.add(t, sampleSkill)
	if res.exit != 0 {
		t.Fatalf("remote add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	// Human shape: bounded (≤ 2 lines) with the verify footer hint.
	lines := nonEmptyLines(res.stdout)
	if len(lines) > 2 {
		t.Errorf("human stdout has %d lines, want <= 2:\n%s", len(lines), res.stdout)
	}

	if !strings.Contains(res.stdout, "skillrig verify") {
		t.Errorf("remote add missing the verify footer hint:\n%s", res.stdout)
	}

	// Vendored byte-identical to the origin fixture, including the exec bit.
	assertVendoredMatchesFixture(t, c.root)

	// Lock: treeSha == the raw-git ground truth; version is the manifest version.
	entry := lockEntry(t, c.root, sampleSkill)
	if entry["treeSha"] != wantTree {
		t.Errorf("lock treeSha = %v, want raw-git ground truth %s", entry["treeSha"], wantTree)
	}

	if entry["version"] != sampleVersion {
		t.Errorf("lock version = %v, want %s", entry["version"], sampleVersion)
	}

	if entry["path"] != vendoredPath {
		t.Errorf("lock path = %v, want %s", entry["path"], vendoredPath)
	}

	if commit, _ := entry["commit"].(string); len(commit) != 40 {
		t.Errorf("lock commit = %q, want a 40-hex commit SHA", commit)
	}

	// Commit the vendored result, then verify must exit 0 (the round-trip).
	commitAll(t, c.root, "vendor remote skill")

	if v := c.verify(t); v.exit != 0 {
		t.Fatalf("verify after remote add exit = %d, want 0 (stderr: %s)", v.exit, v.stderr)
	}
}

// TestQuickstart_AddRemoteIdempotent (US2, SC-006) — re-running a remote add on
// the unchanged vendored skill reports unchanged, exits 0, and leaves the lock
// byte-unchanged.
func TestQuickstart_AddRemoteIdempotent(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	if res := c.add(t, sampleSkill); res.exit != 0 {
		t.Fatalf("first remote add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	lockBefore := readFile(t, filepath.Join(c.root, ".skillrig", "skills-lock.json"))

	second := c.add(t, sampleSkill)
	if second.exit != 0 {
		t.Fatalf("second remote add exit = %d, want 0 (stderr: %s)", second.exit, second.stderr)
	}

	if !strings.Contains(second.stdout, "already vendored") && !strings.Contains(second.stdout, "no change") {
		t.Errorf("idempotent remote re-add should note no change, got:\n%s", second.stdout)
	}

	if after := readFile(t, filepath.Join(c.root, ".skillrig", "skills-lock.json")); after != lockBefore {
		t.Errorf("lock changed on idempotent remote re-add:\nbefore=%s\nafter=%s", lockBefore, after)
	}

	jsonRes := c.add(t, sampleSkill, "--json")

	obj := decodeJSON(t, jsonRes.stdout)
	if obj["action"] != "unchanged" {
		t.Errorf("--json action = %v on idempotent remote re-add, want unchanged", obj["action"])
	}
}

// TestQuickstart_AddRemoteForceOnDivergence (US2) — a locally-modified vendored
// skill makes a plain re-add refuse with a --force hint (002 parity over the
// remote path); --force overwrites it back to the origin content.
func TestQuickstart_AddRemoteForceOnDivergence(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	if res := c.add(t, sampleSkill); res.exit != 0 {
		t.Fatalf("initial remote add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	skillMD := filepath.Join(c.root, vendoredPath, "SKILL.md")
	appendByte(t, skillMD)

	refused := c.add(t, sampleSkill)
	if refused.exit != 1 {
		t.Fatalf("divergent remote re-add (no --force) exit = %d, want 1 (stderr: %s)", refused.exit, refused.stderr)
	}

	if refused.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", refused.stdout)
	}

	assertContains(t, "fix", refused.stderr, "--force")

	// --force restores the origin content and reports overwritten.
	forced := c.add(t, sampleSkill, "--force", "--json")
	if forced.exit != 0 {
		t.Fatalf("forced remote add exit = %d, want 0 (stderr: %s)", forced.exit, forced.stderr)
	}

	if obj := decodeJSON(t, forced.stdout); obj["action"] != "overwritten" {
		t.Errorf("--force --json action = %v, want overwritten", obj["action"])
	}

	originMD := readFile(t, filepath.Join(sampleOriginDir(t), "skills", sampleSkill, "SKILL.md"))
	if readFile(t, skillMD) != originMD {
		t.Errorf("--force should restore the remote skill to the origin's content")
	}
}

// TestQuickstart_AddRemoteDryRun (US2, C6/FR-020) — a remote add --dry-run prints
// a bounded preview, exits 0, and leaves the working tree + lock byte-unchanged.
func TestQuickstart_AddRemoteDryRun(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	res := c.add(t, sampleSkill, "--dry-run")
	if res.exit != 0 {
		t.Fatalf("remote add --dry-run exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if !strings.Contains(res.stdout, "would vendor") {
		t.Errorf("dry-run human output should be prefixed 'would vendor …', got:\n%s", res.stdout)
	}

	// Bounded preview.
	if lines := nonEmptyLines(res.stdout); len(lines) > 2 {
		t.Errorf("dry-run preview has %d lines, want <= 2:\n%s", len(lines), res.stdout)
	}

	// Nothing written: no .agents tree, no lock, and a clean working tree.
	if _, err := os.Stat(filepath.Join(c.root, ".agents")); !os.IsNotExist(err) {
		t.Errorf(".agents/ must not exist after remote --dry-run, stat err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(c.root, ".skillrig", "skills-lock.json")); !os.IsNotExist(err) {
		t.Errorf("lock must not exist after remote --dry-run, stat err = %v", err)
	}

	if porcelain := statusPorcelain(t, c.root); porcelain != "" {
		t.Errorf("working tree not clean after remote --dry-run:\n%s", porcelain)
	}

	jsonRes := c.add(t, sampleSkill, "--dry-run", "--json")

	obj := decodeJSON(t, jsonRes.stdout)
	requireKeys(t, obj, addResultKeys...)

	if obj["dryRun"] != true {
		t.Errorf("--json dryRun = %v, want true", obj["dryRun"])
	}
}

// TestQuickstart_AddPinnedReproducible (US3, SC-004) — pinning the release tag on
// TWO clean consumers yields byte-identical content and identical locks (same
// version/commit/treeSha), and that treeSha is the RAW-git ground truth of the
// tagged commit.
func TestQuickstart_AddPinnedReproducible(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	wantTree := o.rawTree(t, pinTag)

	first := newRemoteConsumer(t, o)
	second := newRemoteConsumer(t, o)

	if res := first.add(t, sampleSkill, "--pin", "v"+sampleVersion); res.exit != 0 {
		t.Fatalf("first pinned add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if res := second.add(t, sampleSkill, "--pin", "v"+sampleVersion); res.exit != 0 {
		t.Fatalf("second pinned add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	e1 := lockEntry(t, first.root, sampleSkill)
	e2 := lockEntry(t, second.root, sampleSkill)

	// treeSha == raw-git ground truth AND identical across the two repos.
	if e1["treeSha"] != wantTree || e2["treeSha"] != wantTree {
		t.Errorf("pinned treeSha = %v / %v, want raw-git ground truth %s", e1["treeSha"], e2["treeSha"], wantTree)
	}

	if e1["commit"] != e2["commit"] {
		t.Errorf("pinned commit drifted across clean repos: %v vs %v", e1["commit"], e2["commit"])
	}

	// The recorded version is the resolved tag (pin honesty, data-model §3).
	if e1["version"] != pinTag {
		t.Errorf("pinned version = %v, want the resolved tag %s", e1["version"], pinTag)
	}

	// Byte-identical vendored content across the two pinned consumers.
	for _, f := range []string{"SKILL.md", "check.sh"} {
		if readSkillFile(t, first.root, f) != readSkillFile(t, second.root, f) {
			t.Errorf("pinned vendored %s differs across clean repos", f)
		}
	}
}

// TestQuickstart_AddPinTagFormEquivalent (US3, C3/SC-004) — `--pin v1.4.0`
// (bare-semver expansion) and `--pin <skill>-v1.4.0` (full-tag literal) resolve
// to the SAME commit and treeSha, confirming the deterministic --pin rule
// end-to-end over the file:// origin.
func TestQuickstart_AddPinTagFormEquivalent(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)

	bare := newRemoteConsumer(t, o)
	full := newRemoteConsumer(t, o)

	if res := bare.add(t, sampleSkill, "--pin", "v"+sampleVersion); res.exit != 0 {
		t.Fatalf("bare-semver pin add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if res := full.add(t, sampleSkill, "--pin", pinTag); res.exit != 0 {
		t.Fatalf("full-tag pin add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	bareEntry := lockEntry(t, bare.root, sampleSkill)
	fullEntry := lockEntry(t, full.root, sampleSkill)

	if bareEntry["commit"] != fullEntry["commit"] {
		t.Errorf("bare-semver vs full-tag commit differ: %v vs %v (must be the same tag)", bareEntry["commit"], fullEntry["commit"])
	}

	if bareEntry["treeSha"] != fullEntry["treeSha"] {
		t.Errorf("bare-semver vs full-tag treeSha differ: %v vs %v (SC-004)", bareEntry["treeSha"], fullEntry["treeSha"])
	}
}

// TestQuickstart_AddPinNotFound (US3, C2/FR-015) — pinning a non-existent version
// fails exit 1 with the distinct NoSuchVersionError rendering ("has no version" +
// the pin-does-not-resolve why), NOT the skill-not-found message. The skill and
// the repo exist; only the requested tag does not.
func TestQuickstart_AddPinNotFound(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	res := c.add(t, sampleSkill, "--pin", "v9.9.9")
	if res.exit != 1 {
		t.Fatalf("pin-not-found add exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	// The NoSuchVersionError rendering is the structured discriminator: it names
	// the missing version and cites the unresolved pin — distinct from a
	// skill-not-found ("not found in the origin") class (C2).
	assertContains(t, "what", res.stderr, "has no version")
	assertContains(t, "why", res.stderr, "the pin does not resolve")

	if strings.Contains(res.stderr, "not found in the origin") {
		t.Errorf("pin-not-found must NOT render as skill-not-found (C2: distinct classes), got:\n%s", res.stderr)
	}

	// The raw git cause is surfaced under --verbose, never swallowed.
	verbose := c.add(t, sampleSkill, "--pin", "v9.9.9", "--verbose")
	if verbose.exit != 1 {
		t.Errorf("--verbose pin-not-found exit = %d, want 1", verbose.exit)
	}
}

// TestQuickstart_AddAuthFailureDistinct (US4) — an injected clone auth failure
// renders as an AUTHENTICATION failure (distinct from not-found/unreachable),
// pointing at gh auth login / GITHUB_TOKEN; exit 1.
func TestQuickstart_AddAuthFailureDistinct(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	res := c.addWithFakeGit(t,
		"remote: Authentication failed for 'https://github.com/my-org/my-skills/'",
		sampleSkill)

	if res.exit != 1 {
		t.Fatalf("injected auth-failure add exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "authentication failed")
	// Distinct from unreachable / not-found, and points at the credential fix.
	if strings.Contains(res.stderr, "could not reach") || strings.Contains(res.stderr, "not found") {
		t.Errorf("auth failure must be distinct from unreachable/not-found, got:\n%s", res.stderr)
	}

	if !strings.Contains(res.stderr, "gh auth login") && !strings.Contains(res.stderr, "GITHUB_TOKEN") {
		t.Errorf("auth failure fix should point at gh auth login / GITHUB_TOKEN, got:\n%s", res.stderr)
	}
}

// TestQuickstart_AddUnreachableDistinct (US4) — an injected clone "could not
// resolve host" failure renders as an UNREACHABLE failure, distinct from
// auth/not-found; exit 1.
func TestQuickstart_AddUnreachableDistinct(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	res := c.addWithFakeGit(t,
		"fatal: unable to access origin: Could not resolve host: github.com",
		sampleSkill)

	if res.exit != 1 {
		t.Fatalf("injected unreachable add exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "could not reach")

	if strings.Contains(res.stderr, "authentication failed") {
		t.Errorf("unreachable must be distinct from auth, got:\n%s", res.stderr)
	}
}

// TestQuickstart_AddPrivateNotFoundHintsAuth (US4, D4) — an injected clone
// not-found with no resolved token renders a not-found that ALSO adds the "if
// private, authenticate" hint, so the agent is not sent to re-check a skill name
// when the real problem is a missing credential; exit 1.
func TestQuickstart_AddPrivateNotFoundHintsAuth(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	res := c.addWithFakeGit(t,
		"fatal: repository 'https://github.com/my-org/my-skills/' not found",
		sampleSkill)

	if res.exit != 1 {
		t.Fatalf("injected private-not-found add exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	assertContains(t, "what", res.stderr, "not found")
	// The D4 subtlety: an unauthenticated not-found adds the authenticate hint.
	if !strings.Contains(res.stderr, "authenticate") {
		t.Errorf("private-not-found should add the 'if private, authenticate' hint, got:\n%s", res.stderr)
	}
}

// TestQuickstart_AddRemoteConventionMismatch (FIX-4/H1) — a file:// origin whose
// index.json declares skillrigConvention 2 is rejected by the remote add's
// convention gate end-to-end: exit 1 with the IncompatibleConvention what/why/fix,
// and NOTHING written (no .agents/, no .skillrig/ in the consumer). This proves
// gateRemoteConvention runs over the real remote-fetch path before any vendoring.
func TestQuickstart_AddRemoteConventionMismatch(t *testing.T) {
	t.Parallel()

	o := newRemoteOriginConvention(t, 2)
	c := newRemoteConsumer(t, o)

	res := c.add(t, sampleSkill)
	if res.exit != 1 {
		t.Fatalf("convention-2 remote add exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	// The IncompatibleConvention rendering: what (a convention mismatch), why, and
	// the fix that points at updating skillrig.
	assertContains(t, "what", res.stderr, "convention")
	assertContains(t, "why", res.stderr, "why:")
	assertContains(t, "fix", res.stderr, "update skillrig")

	// The gate runs BEFORE any write: no vendored tree and no lock must exist.
	if _, err := os.Stat(filepath.Join(c.root, ".agents")); !os.IsNotExist(err) {
		t.Errorf(".agents/ must not exist after a gated remote add, stat err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(c.root, ".skillrig")); !os.IsNotExist(err) {
		t.Errorf(".skillrig/ must not exist after a gated remote add, stat err = %v", err)
	}
}

// assertVendoredMatchesFixture checks every vendored skill file under root is
// byte-identical to the committed sample-origin fixture, with modes preserved
// (the exec bit is part of the tree-SHA). The remote-fetched copy must match the
// origin source exactly.
func assertVendoredMatchesFixture(t *testing.T, root string) {
	t.Helper()

	for _, f := range []string{"SKILL.md", "check.sh"} {
		got := readSkillFile(t, root, f)
		want := readFile(t, filepath.Join(sampleOriginDir(t), "skills", sampleSkill, f))

		if got != want {
			t.Errorf("vendored %s differs from the origin fixture", f)
		}

		gotMode := fileMode(t, filepath.Join(root, vendoredPath, f))
		wantMode := fileMode(t, filepath.Join(sampleOriginDir(t), "skills", sampleSkill, f))

		if gotMode != wantMode {
			t.Errorf("vendored %s mode = %v, want %v", f, gotMode, wantMode)
		}
	}

	if execMode := fileMode(t, filepath.Join(root, vendoredPath, "check.sh")); execMode&0o111 == 0 {
		t.Errorf("vendored check.sh lost its executable bit: mode = %v", execMode)
	}
}

// TestQuickstart_SearchRemoteFileOrigin (US1 over the remote substrate) — search
// against a file:// origin with NO local checkout fetches index.json over the
// real git transport (FIX-2's per-call catalog fetch) and lists the skill the
// origin publishes; exit 0 with the complete --json record. This proves search
// works end-to-end against a remote origin, not just a local catalog on disk.
func TestQuickstart_SearchRemoteFileOrigin(t *testing.T) {
	t.Parallel()

	o := newRemoteOrigin(t)
	c := newRemoteConsumer(t, o)

	res := run(t, runOpts{
		args: []string{"search", "--json"},
		cwd:  c.root,
		env:  map[string]string{"SKILLRIG_ORIGIN": c.cloneURL},
	})
	if res.exit != 0 {
		t.Fatalf("remote search exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	p := decodeSearch(t, res.stdout)
	if names := searchNames(p); len(names) != 1 || names[0] != sampleSkill {
		t.Errorf("remote search names = %v, want exactly [%s] (fetched from the file:// origin)", names, sampleSkill)
	}

	// --json record is structurally complete (every field add needs).
	obj := decodeJSON(t, res.stdout)
	requireKeys(t, obj, "origin", "skills")

	rawSkills, ok := obj["skills"].([]any)
	if !ok || len(rawSkills) == 0 {
		t.Fatalf("remote search skills not a non-empty array: %v", obj["skills"])
	}

	entry, ok := rawSkills[0].(map[string]any)
	if !ok {
		t.Fatalf("remote search skills[0] not an object: %v", rawSkills[0])
	}

	requireKeys(t, entry, "name", "version", "namespace", "description", "topics", "path")
}
