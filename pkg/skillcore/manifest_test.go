package skillcore

import (
	"path/filepath"
	"reflect"
	"testing"
)

// writeSkillMd writes content to <tmp>/<name>/SKILL.md and returns the path. The
// parent directory is named after the skill so the parse contract (name == dir)
// can hold.
func writeSkillMd(t *testing.T, name, content string) string {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, dir, filepath.Join(name, "SKILL.md"), 0o644, content)

	return filepath.Join(dir, name, "SKILL.md")
}

// TestParseManifest_FrontmatterAndUnknownKeys asserts the parse contract: a
// SKILL.md whose YAML frontmatter carries the standard fields plus the namespaced
// metadata.x-skillrig.* extensions (including a nested requires list) AND unknown
// keys parses into the expected Manifest, ignoring the unknowns with no error
// (forward-compat).
func TestParseManifest_FrontmatterAndUnknownKeys(t *testing.T) {
	t.Parallel()

	const skillMd = `---
name: terraform-plan-review
description: Review a terraform plan
license: MIT
metadata:
  x-skillrig.namespace: dev.skillrig.samples
  x-skillrig.version: 1.4.0
  x-skillrig.convention-version: "1"
  x-skillrig.topics: [terraform, review]
  x-skillrig.experimental: true
  x-skillrig.requires:
    - tool: terraform
      version: ">=1.5"
      source: https://releases.hashicorp.com
      manager: asdf
      optional: true
    - tool: tflint
      version: 0.50.0
---

# Terraform Plan Review

Body.
`

	path := writeSkillMd(t, "terraform-plan-review", skillMd)

	got, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest: unexpected error: %v", err)
	}

	want := Manifest{
		Name:        "terraform-plan-review",
		Description: "Review a terraform plan",
		Namespace:   "dev.skillrig.samples",
		Version:     "1.4.0",
		Convention:  "1",
		Topics:      []string{"terraform", "review"},
		Requires: []Require{
			{
				Tool:    "terraform",
				Version: ">=1.5",
				Source:  "https://releases.hashicorp.com",
				Manager: "asdf",
			},
			{
				Tool:    "tflint",
				Version: "0.50.0",
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseManifest mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}

// TestParseManifest_Minimal confirms a SKILL.md with only the required standard
// name + x-skillrig.version parses to a nil/empty Requires slice (add only needs
// name + version).
func TestParseManifest_Minimal(t *testing.T) {
	t.Parallel()

	const skillMd = `---
name: solo
metadata:
  x-skillrig.version: 0.1.0
---

# solo
`

	path := writeSkillMd(t, "solo", skillMd)

	got, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	if got.Name != "solo" || got.Version != "0.1.0" {
		t.Errorf("name/version = %q/%q, want solo/0.1.0", got.Name, got.Version)
	}

	if len(got.Requires) != 0 {
		t.Errorf("Requires = %+v, want empty", got.Requires)
	}
}

// TestParseManifest_Errors covers the failure surface: a missing file, a
// SKILL.md with no frontmatter, malformed YAML, a name that does not match the
// directory, and a missing version must all return an error (the CLI renders it;
// the SDK only returns it).
func TestParseManifest_Errors(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		_, err := ParseManifest(filepath.Join(t.TempDir(), "skill", "SKILL.md"))
		if err == nil {
			t.Fatal("ParseManifest(absent): want error, got nil")
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		t.Parallel()

		path := writeSkillMd(t, "solo", "# solo\n\nNo frontmatter here.\n")

		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("ParseManifest(no frontmatter): want error, got nil")
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		t.Parallel()

		path := writeSkillMd(t, "solo", "---\nname: x\n  bad: : indent\n---\n")

		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("ParseManifest(malformed): want error, got nil")
		}
	})

	t.Run("name mismatches directory", func(t *testing.T) {
		t.Parallel()

		const skillMd = "---\nname: other\nmetadata:\n  x-skillrig.version: 0.1.0\n---\n"

		path := writeSkillMd(t, "solo", skillMd)

		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("ParseManifest(name != dir): want error, got nil")
		}
	})

	t.Run("missing version", func(t *testing.T) {
		t.Parallel()

		path := writeSkillMd(t, "solo", "---\nname: solo\n---\n")

		_, err := ParseManifest(path)
		if err == nil {
			t.Fatal("ParseManifest(no version): want error, got nil")
		}
	})
}
