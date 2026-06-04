package skillcore

import (
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// writeOriginFixture lays down a minimal origin checkout under a fresh tmpDir: a
// .skillrig-origin.toml carrying the convention/origin/skills_dir, plus a
// skills/<name>/SKILL.md for each (name, frontmatter) pair. It does NOT git-init
// — GenerateCatalog reads the filesystem, not git, so a working tree is enough
// (the tree-SHA oracle elsewhere uses raw git; the catalog never hashes).
func writeOriginFixture(t *testing.T, originToml string, skills map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, dir, ".skillrig-origin.toml", 0o644, originToml)

	for name, skillMd := range skills {
		writeFile(t, dir, filepath.Join("skills", name, "SKILL.md"), 0o644, skillMd)
	}

	return dir
}

// goldenOriginToml is the origin contract the golden-fixture test builds against.
const goldenOriginToml = `convention_version = 1
origin = "my-org/my-skills"
skills_dir = "skills"
`

// goldenAlphaSkillMd / goldenBetaSkillMd are the two skills the golden catalog is
// generated from: alpha carries topics + a single requires entry, beta omits
// requires (so the golden pins the requires: null projection too).
const goldenAlphaSkillMd = `---
name: alpha
description: Alpha skill.
metadata:
  x-skillrig.namespace: my-org
  x-skillrig.version: 1.0.0
  x-skillrig.topics: [aws, platform]
  x-skillrig.requires:
    - tool: terraform
      version: ">=1.6"
      source: hashicorp/terraform
      manager: mise
---
# alpha
`

const goldenBetaSkillMd = `---
name: beta
description: Beta skill for review.
metadata:
  x-skillrig.namespace: my-org
  x-skillrig.version: 2.1.0
  x-skillrig.topics: [review]
---
# beta
`

// goldenIndexJSON is the committed ground-truth catalog (SC-009 / D2): the exact
// bytes GenerateCatalog must emit over goldenAlphaSkillMd + goldenBetaSkillMd.
// Entries are sorted by name; a trailing newline is appended; Require fields
// serialize with LOWERCASE keys (tool/version/source/manager) from the type's
// json tags (FIX-5, data-model §2) — the golden documents the producer's actual
// on-disk artifact, and its lowercase keys keep the bug from re-hiding.
const goldenIndexJSON = `{
  "skillrigConvention": 1,
  "origin": "my-org/my-skills",
  "skills": [
    {
      "name": "alpha",
      "version": "1.0.0",
      "namespace": "my-org",
      "description": "Alpha skill.",
      "topics": [
        "aws",
        "platform"
      ],
      "path": "skills/alpha",
      "requires": [
        {
          "tool": "terraform",
          "version": ">=1.6",
          "source": "hashicorp/terraform",
          "manager": "mise"
        }
      ]
    },
    {
      "name": "beta",
      "version": "2.1.0",
      "namespace": "my-org",
      "description": "Beta skill for review.",
      "topics": [
        "review"
      ],
      "path": "skills/beta",
      "requires": null
    }
  ]
}
`

// TestGenerateCatalog_EqualsGoldenFixture is the producer==artifact ground-truth
// oracle (D2 contract test, SC-009): GenerateCatalog over a fixed skill set must
// be BYTE-identical to the committed golden index.json — sorted by name, trailing
// newline, convention carried from .skillrig-origin.toml (not hardcoded — C7). A
// diff here means the catalog format drifted under search/add's feet.
func TestGenerateCatalog_EqualsGoldenFixture(t *testing.T) {
	t.Parallel()

	dir := writeOriginFixture(t, goldenOriginToml, map[string]string{
		// Insertion order is intentionally NOT lexicographic so the test also
		// proves GenerateCatalog sorts rather than echoing directory order. (Go
		// map ranging is randomized anyway; ReadDir returns sorted, but writing
		// beta-then-alpha here documents the intent.)
		"beta":  goldenBetaSkillMd,
		"alpha": goldenAlphaSkillMd,
	})

	got, err := GenerateCatalog(dir)
	if err != nil {
		t.Fatalf("GenerateCatalog: unexpected error: %v", err)
	}

	if string(got) != goldenIndexJSON {
		t.Errorf("GenerateCatalog output != golden fixture\n--- got ---\n%s\n--- want ---\n%s",
			got, goldenIndexJSON)
	}
}

// TestGenerateCatalog_Deterministic asserts the determinism contract (SC-009):
// two runs over an unchanged skill set yield byte-identical output (no map-order
// or walk-order nondeterminism leaks into the artifact).
func TestGenerateCatalog_Deterministic(t *testing.T) {
	t.Parallel()

	dir := writeOriginFixture(t, goldenOriginToml, map[string]string{
		"alpha": goldenAlphaSkillMd,
		"beta":  goldenBetaSkillMd,
	})

	first, err := GenerateCatalog(dir)
	if err != nil {
		t.Fatalf("GenerateCatalog (first): %v", err)
	}

	second, err := GenerateCatalog(dir)
	if err != nil {
		t.Fatalf("GenerateCatalog (second): %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("GenerateCatalog not deterministic across runs:\nfirst:\n%s\nsecond:\n%s",
			first, second)
	}
}

// TestCheckConvention_Boundary pins the exact-match convention gate (C1): only the
// single supported value passes; the immediately-adjacent values (one below, one
// above) and the absent/zero field all fail with *IncompatibleConventionError
// carrying the read value. This is the boundary the brief calls out (0, absent,
// and 2 give the error, 1 is ok) — encoded as 0/1/2 plus the explicit absent
// case (a missing skillrigConvention decodes to 0).
func TestCheckConvention_Boundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		n       int
		wantErr bool
	}{
		{name: "zero (absent field) fails", n: 0, wantErr: true},
		{name: "exactly one is ok", n: 1, wantErr: false},
		{name: "two (one above) fails", n: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := CheckConvention(tt.n)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("CheckConvention(%d) = %v, want nil", tt.n, err)
				}

				return
			}

			var convErr *IncompatibleConventionError
			if !errors.As(err, &convErr) {
				t.Fatalf("CheckConvention(%d) error = %T (%v), want *IncompatibleConventionError",
					tt.n, err, err)
			}

			if convErr.Found != tt.n {
				t.Errorf("IncompatibleConventionError.Found = %d, want %d", convErr.Found, tt.n)
			}

			if convErr.Supported != supportedConvention {
				t.Errorf("IncompatibleConventionError.Supported = %d, want %d",
					convErr.Supported, supportedConvention)
			}
		})
	}
}

// TestSearch_OrderingAndDeterminism is the table-driven matcher contract (D8):
// query → the EXACT ordered list of matching names. It pins the relevance
// buckets (exact-name > name-substring > topic-hit > description-only) with the
// lexicographic-by-name tiebreak, the token-AND substring rule, the case
// insensitivity, the --topic membership filter, and the empty-query "list all"
// and no-match "empty" edges. Exact order is asserted — not set membership — so a
// ranking regression is caught.
func TestSearch_OrderingAndDeterminism(t *testing.T) {
	t.Parallel()

	// A fixed catalog exercising every bucket. Names chosen so the lexicographic
	// tiebreak is observable within a bucket (e.g. two name-substring hits).
	catalog := Catalog{
		SkillrigConvention: 1,
		Origin:             "my-org/my-skills",
		Skills: []CatalogEntry{
			{
				Name:        "terraform",
				Description: "infra as code",
				Topics:      []string{"infra"},
			},
			{
				Name:        "terraform-plan-review",
				Description: "Review a terraform plan",
				Topics:      []string{"platform", "aws"},
			},
			{
				Name:        "drift-detector",
				Description: "detect terraform drift across stacks",
				Topics:      []string{"terraform-tooling"},
			},
			{
				Name:        "cost-estimator",
				Description: "estimate spend before apply",
				Topics:      []string{"finops", "aws"},
			},
			{
				Name:        "terraform-fmt",
				Description: "format HCL",
				Topics:      []string{"style"},
			},
		},
	}

	tests := []struct {
		name   string
		query  []string
		topics []string
		want   []string
	}{
		{
			name:  "empty query lists all by name",
			query: nil,
			want: []string{
				"cost-estimator",
				"drift-detector",
				"terraform",
				"terraform-fmt",
				"terraform-plan-review",
			},
		},
		{
			name:  "relevance buckets then lexicographic name",
			query: []string{"terraform"},
			// exact-name(3): terraform
			// name-substring(2): terraform-fmt, terraform-plan-review (lex)
			// topic-hit(1): drift-detector (topic "terraform-tooling")
			// description-only(0): cost-estimator does NOT match (no "terraform")
			want: []string{
				"terraform",
				"terraform-fmt",
				"terraform-plan-review",
				"drift-detector",
			},
		},
		{
			name:  "case-insensitive substring match",
			query: []string{"TERRAFORM"},
			want: []string{
				"terraform",
				"terraform-fmt",
				"terraform-plan-review",
				"drift-detector",
			},
		},
		{
			name:  "token-AND requires every term to match",
			query: []string{"terraform", "review"},
			// only terraform-plan-review has both "terraform" and "review".
			want: []string{"terraform-plan-review"},
		},
		{
			name:   "topic filter (case-insensitive membership)",
			query:  nil,
			topics: []string{"AWS"},
			want:   []string{"cost-estimator", "terraform-plan-review"},
		},
		{
			name:   "query and topic combine (AND)",
			query:  []string{"terraform"},
			topics: []string{"aws"},
			want:   []string{"terraform-plan-review"},
		},
		{
			name:  "no match yields empty",
			query: []string{"kubernetes"},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := namesOf(Search(catalog, tt.query, tt.topics))

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Search(%v, topics=%v) order =\n  %v\nwant\n  %v",
					tt.query, tt.topics, got, tt.want)
			}

			// Determinism: a second call over the same inputs is identical.
			again := namesOf(Search(catalog, tt.query, tt.topics))
			if !reflect.DeepEqual(again, got) {
				t.Errorf("Search not deterministic: first %v, second %v", got, again)
			}
		})
	}
}

// namesOf projects entries to their names, preserving order. It returns a
// non-nil empty slice for an empty input so reflect.DeepEqual against a
// []string{} literal (the no-match expectation) holds.
func namesOf(entries []CatalogEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}

	return out
}

// TestClassifyGitError_StderrToTyped is the stderr→typed-error classification
// table (D4/D6): each representative git/gh stderr (all exit 128) must map to its
// network-class typed error, an unrecognized stderr must pass the raw *GitError
// through unchanged, and a nil input must stay nil. The classifier is the single
// place this mapping lives (skillcore, never cli); the typed errors wrap the
// original so --verbose still reaches the raw git output.
func TestClassifyGitError_StderrToTyped(t *testing.T) {
	t.Parallel()

	t.Run("nil passes through as nil", func(t *testing.T) {
		t.Parallel()

		if err := ClassifyGitError(nil); err != nil {
			t.Fatalf("ClassifyGitError(nil) = %v, want nil", err)
		}
	})

	tests := []struct {
		name   string
		stderr string
		// want is the typed class the stderr must classify into. classRaw means
		// "left as the bare *GitError" (no network class matched).
		want errClass
	}{
		{
			name:   "authentication failed -> AuthError",
			stderr: "remote: Authentication failed for 'https://github.com/my-org/my-skills/'",
			want:   classAuth,
		},
		{
			name:   "invalid username or token -> AuthError",
			stderr: "fatal: Invalid username or token. Password authentication is not supported.",
			want:   classAuth,
		},
		{
			// Issue #25's exact macOS symptom: a private origin reached with no
			// usable credential, git unable to prompt → must NOT fall through to a
			// raw *GitError. Carries no host/connect or "not found" anchor.
			name:   "could not read Username / Device not configured -> AuthError (issue #25)",
			stderr: "fatal: could not read Username for 'https://github.com': Device not configured",
			want:   classAuth,
		},
		{
			// The same situation under GIT_TERMINAL_PROMPT=0 (CI / no TTY): git's
			// own "terminal prompts disabled" message.
			name:   "could not read Username / terminal prompts disabled -> AuthError (issue #25)",
			stderr: "fatal: could not read Username for 'https://github.com': terminal prompts disabled",
			want:   classAuth,
		},
		{
			name:   "could not read Password -> AuthError",
			stderr: "fatal: could not read Password for 'https://x@github.com': terminal prompts disabled",
			want:   classAuth,
		},
		{
			// Scope guard: the bare "Device not configured" errno, WITHOUT a
			// credential-read prefix, must NOT be force-classified as auth — it is a
			// generic ENXIO that can arise outside a credential read. Only the
			// "could not read Username/Password" prefix (above) makes it auth.
			name:   "bare 'Device not configured' without a credential read stays raw",
			stderr: "fatal: write error: Device not configured",
			want:   classRaw,
		},
		{
			// Same scope guard for the prompt-disabled tail: we anchor on the
			// credential-read PREFIX, not this variable tail, so "terminal prompts
			// disabled" on its own (no "could not read Username/Password") is NOT auth.
			name:   "bare 'terminal prompts disabled' without a credential read stays raw",
			stderr: "fatal: terminal prompts disabled",
			want:   classRaw,
		},
		{
			name:   "could not resolve host -> UnreachableError",
			stderr: "fatal: unable to access 'https://github.com/...': Could not resolve host: github.com",
			want:   classUnreachable,
		},
		{
			name:   "failed to connect -> UnreachableError",
			stderr: "fatal: unable to access '...': Failed to connect to github.com port 443",
			want:   classUnreachable,
		},
		{
			name:   "repository not found (URL form) -> NotFoundError",
			stderr: "fatal: repository 'https://github.com/my-org/missing/' not found",
			want:   classNotFound,
		},
		{
			name:   "Remote: Repository not found (capitalized) -> NotFoundError",
			stderr: "remote: Repository not found.\nfatal: repository '...' not found",
			want:   classNotFound,
		},
		{
			name:   "unrecognized stderr passes the raw *GitError through",
			stderr: "fatal: early EOF",
			want:   classRaw,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := &GitError{ExitCode: 128, Stderr: tt.stderr}

			out := ClassifyGitError(raw)

			if got := classOf(out); got != tt.want {
				t.Errorf("ClassifyGitError(%q) class = %v, want %v", tt.stderr, got, tt.want)
			}

			// Every classified error must still unwrap to the raw *GitError so
			// --verbose can reach the original stderr (the raw case IS the
			// *GitError; the network cases wrap it).
			var ge *GitError
			if !errors.As(out, &ge) {
				t.Fatalf("classified error %v does not unwrap to *GitError", out)
			}

			if ge.Stderr != tt.stderr {
				t.Errorf("unwrapped stderr = %q, want %q", ge.Stderr, tt.stderr)
			}
		})
	}
}

// errClass names the typed failure ClassifyGitError can produce, so the table can
// assert the matched class declaratively (one classOf call) instead of a per-case
// errors.As closure — keeping the test's branching flat.
type errClass int

const (
	classRaw errClass = iota // left as the bare *GitError (no network class)
	classAuth
	classUnreachable
	classNotFound
)

func (c errClass) String() string {
	switch c {
	case classAuth:
		return "AuthError"
	case classUnreachable:
		return "UnreachableError"
	case classNotFound:
		return "NotFoundError"
	case classRaw:
		return "raw *GitError"
	default:
		return "unknown"
	}
}

// classOf reports which network class err was classified into; classRaw means no
// network class matched (it is still the bare *GitError). The most specific match
// wins — the network types are checked before falling through to raw.
func classOf(err error) errClass {
	var (
		a *AuthError
		u *UnreachableError
		n *NotFoundError
	)

	switch {
	case errors.As(err, &a):
		return classAuth
	case errors.As(err, &u):
		return classUnreachable
	case errors.As(err, &n):
		return classNotFound
	default:
		return classRaw
	}
}

// ghStub is the behavior a fake `gh` binary impersonates for one case: print
// stdout, then exit with exitCode. absent==true installs NO gh on PATH (the
// fallback tier is skipped entirely).
type ghStub struct {
	absent   bool
	stdout   string
	exitCode int
}

// installGhStub points PATH at a fresh dir for this case. When stub.absent it
// leaves the dir empty (exec.LookPath("gh") fails → fallback skipped); otherwise
// it writes an executable `gh` shell script that emits stub.stdout and exits with
// stub.exitCode. Returning a fresh dir per case (no inherited PATH) keeps the
// fallback hermetic — a real gh on the developer's PATH can never leak in.
func installGhStub(t *testing.T, stub ghStub) {
	t.Helper()

	binDir := t.TempDir()

	if !stub.absent {
		// A leading newline in stdout proves the resolver TrimSpaces the output.
		script := "#!/bin/sh\nprintf '%s' \"" + stub.stdout + "\"\nexit " +
			strconv.Itoa(stub.exitCode) + "\n"
		writeFile(t, binDir, "gh", 0o755, script)
	}

	// Replace PATH entirely (not prepend) so no ambient gh is reachable.
	t.Setenv("PATH", binDir)
}

// TestResolveGitHubToken_Precedence pins the resolver order (D4): GH_TOKEN env >
// GITHUB_TOKEN env > `gh auth token`. The env tiers are exercised directly
// (whitespace-only is "unset"); the gh-fallback tier is exercised by impersonating
// the gh binary on a scrubbed PATH (a stub shell script), so the test never
// touches a real gh session. Each case clears both env vars first so the
// precedence is unambiguous.
//
// This test mutates process env (GH_TOKEN/GITHUB_TOKEN/PATH), so it does NOT run
// in parallel — it would race other tests reading those globals.
//
//nolint:tparallel // mutates os.Environ (GH_TOKEN/GITHUB_TOKEN/PATH); must run serially.
func TestResolveGitHubToken_Precedence(t *testing.T) {
	tests := []struct {
		name string
		// env tiers ("" + ok=false means the var is left unset)
		ghToken   string
		ghTokenOK bool
		ghubToken string
		ghubOK    bool
		// gh fallback impersonation
		gh ghStub

		wantToken string
		wantOK    bool
	}{
		{
			name:      "GH_TOKEN wins over GITHUB_TOKEN and gh",
			ghToken:   "gh-env-token",
			ghTokenOK: true,
			ghubToken: "github-env-token",
			ghubOK:    true,
			gh:        ghStub{stdout: "gh-cli-token"},
			wantToken: "gh-env-token",
			wantOK:    true,
		},
		{
			name:      "GITHUB_TOKEN used when GH_TOKEN unset",
			ghubToken: "github-env-token",
			ghubOK:    true,
			gh:        ghStub{stdout: "gh-cli-token"},
			wantToken: "github-env-token",
			wantOK:    true,
		},
		{
			name:      "whitespace-only GH_TOKEN is treated as unset",
			ghToken:   "   ",
			ghTokenOK: true,
			ghubToken: "github-env-token",
			ghubOK:    true,
			wantToken: "github-env-token",
			wantOK:    true,
		},
		{
			name:      "falls back to gh auth token when both env unset",
			gh:        ghStub{stdout: "\ngh-cli-token\n"}, // leading/trailing ws trimmed
			wantToken: "gh-cli-token",
			wantOK:    true,
		},
		{
			name:      "gh absent from PATH -> (\"\", false)",
			gh:        ghStub{absent: true},
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "gh present but non-zero exit -> (\"\", false)",
			gh:        ghStub{stdout: "stale-token", exitCode: 1},
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "gh exit 0 but empty/whitespace stdout -> (\"\", false)",
			gh:        ghStub{stdout: "  \n"},
			wantToken: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv auto-restores. Setting an unwanted tier to "" is
			// equivalent to unsetting it: the resolver treats an empty (or
			// whitespace-only) value as "unset", so the precedence under test is
			// unambiguous without touching os.Unsetenv.
			ghEnv := ""
			if tt.ghTokenOK {
				ghEnv = tt.ghToken
			}

			t.Setenv("GH_TOKEN", ghEnv)

			ghubEnv := ""
			if tt.ghubOK {
				ghubEnv = tt.ghubToken
			}

			t.Setenv("GITHUB_TOKEN", ghubEnv)

			installGhStub(t, tt.gh)

			gotToken, gotOK := ResolveGitHubToken("github.com")

			if gotToken != tt.wantToken || gotOK != tt.wantOK {
				t.Errorf("ResolveGitHubToken() = (%q, %v), want (%q, %v)",
					gotToken, gotOK, tt.wantToken, tt.wantOK)
			}
		})
	}
}

// TestManifest_MetadataXSkillrigKeys focuses the parse contract on the
// metadata.x-skillrig.* extension lift (D1): every namespaced key — namespace,
// version, convention-version, topics, and the nested requires list — is hoisted
// onto the flat Manifest, while standard frontmatter (name, description) is read
// verbatim and unknown keys (top-level and inside metadata) are ignored. This is
// the catalog/add data source, so the lift is asserted field-by-field.
func TestManifest_MetadataXSkillrigKeys(t *testing.T) {
	t.Parallel()

	const skillMd = `---
name: alpha
description: An alpha skill.
license: MIT
allowed-tools: Bash(git:*) Read
metadata:
  x-skillrig.namespace: my-org
  x-skillrig.version: 1.4.0
  x-skillrig.convention-version: "1"
  x-skillrig.topics: [platform-team, terraform, aws]
  x-skillrig.unknown-extension: ignored
  x-skillrig.requires:
    - tool: oxid
      version: ">=0.4.0"
      source: my-org/my-skills
      manager: mise
---

# alpha
`

	path := writeSkillMd(t, "alpha", skillMd)

	got, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	want := Manifest{
		Name:        "alpha",
		Description: "An alpha skill.",
		Namespace:   "my-org",
		Version:     "1.4.0",
		Convention:  "1",
		Topics:      []string{"platform-team", "terraform", "aws"},
		Requires: []Require{
			{
				Tool:    "oxid",
				Version: ">=0.4.0",
				Source:  "my-org/my-skills",
				Manager: "mise",
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseManifest metadata.x-skillrig lift mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}
