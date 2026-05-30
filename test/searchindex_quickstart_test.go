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
// --pin, US4 auth/unreachable failures) is specified against a file:// bare
// repo for the CLI's origin and the pkg/skillcore commandContext stub seam for
// injected git failures. Neither substrate is reachable from this integration
// tier against the current binary: the origin resolver accepts only the
// OWNER/REPO[@REF] shape (a file:// URL is rejected by config.originPattern) and
// pkg/skillcore.cloneURL hard-codes https://github.com/OWNER/REPO.git with no
// local/file:// seam, so `add` with no local checkout can only reach github.com
// over the network — it never clones a t.TempDir bare repo. The commandContext
// stub is an unexported pkg/skillcore field, reachable only from a
// pkg/skillcore unit test, not from a built binary. Those scenarios are
// therefore registered here as t.Skip placeholders (so the contract stays
// visible and a future un-skip is one edit) and belong, per the quickstart's
// own "unit-level via the stub seam" note, in pkg/skillcore once a file:// /
// local-clone fetch seam exists.
package quickstart

import (
	"encoding/json"
	"os"
	"path/filepath"
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
// US2/US3/US4 — remote acquisition, --pin, injected failures.
//
// These are specified against a file:// bare repo (the CLI's origin) and the
// pkg/skillcore commandContext stub seam. As documented in the package header,
// neither substrate is reachable from a BUILT binary against the current
// implementation: the origin resolver rejects a file:// URL (config.originPattern
// is OWNER/REPO[@REF]-only) and pkg/skillcore.cloneURL hard-codes
// https://github.com/OWNER/REPO.git with no local/file:// seam, so a no-local-copy
// add can only reach github.com over the network. The stub seam is an unexported
// pkg/skillcore field, reachable only from a pkg/skillcore unit test.
//
// They are registered as skips so the contract stays visible (and a future
// un-skip is a one-line edit once a file:// / local-clone fetch seam lands in
// pkg/skillcore/fetch.go). Per the quickstart's own "unit-level via the stub
// seam" note, the auth/unreachable/not-found classifications belong in
// pkg/skillcore_test, and the remote add/pin round-trips belong here once the
// binary can be pointed at a t.TempDir bare repo.
// ---------------------------------------------------------------------------

const remoteSubstrateBlocked = "BLOCKED: needs a file:// / local-clone fetch seam in pkg/skillcore " +
	"(cloneURL hard-codes https://github.com and the origin resolver rejects file:// URLs); " +
	"un-skip once add can be pointed at a t.TempDir bare repo — touches non-test code, out of this task's scope"

// Each blocked scenario maps to a quickstart contract so the suite roster
// reflects the full 003 surface even while the file:// / stub-seam substrate is
// missing. t.Parallel() then t.Skip keeps them parallel-safe (paralleltest) and
// turns "un-skip" into a one-line edit once the fetch seam lands.

func TestQuickstart_AddRemoteNoLocalCopy(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddRemoteIdempotent(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddRemoteForceOnDivergence(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddRemoteDryRun(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddPinnedReproducible(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddPinTagFormEquivalent(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddPinNotFound(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddAuthFailureDistinct(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddUnreachableDistinct(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}

func TestQuickstart_AddPrivateNotFoundHintsAuth(t *testing.T) {
	t.Parallel()
	t.Skip(remoteSubstrateBlocked)
}
